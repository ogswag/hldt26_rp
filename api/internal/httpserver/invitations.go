package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/config"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/mail"
)

const inviteListCap = 200

type invitationJSON struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	ProjectID   *string    `json:"project_id"`
	ProjectName *string    `json:"project_name"`
	ProjectRole *string    `json:"project_role"`
	CreatedAt   *time.Time `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	AcceptedAt  *time.Time `json:"accepted_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	InvitedBy   *string    `json:"invited_by"`
}

type createInviteBody struct {
	Email       string  `json:"email"`
	ProjectID   *string `json:"project_id"`
	ProjectRole *string `json:"project_role"`
}

// invite stores an invitation and queues its letter. project and role are empty for an invitation to the app.
func (s *Server) invite(ctx context.Context, email string, by uuid.UUID, project *uuid.UUID, role *string, projectName string) (db.CreateInvitationRow, error) {
	token, hash, err := auth.NewToken()
	if err != nil {
		return db.CreateInvitationRow{}, err
	}
	row, err := s.q.CreateInvitation(ctx, db.CreateInvitationParams{
		Email:       email,
		TokenHash:   hash,
		InvitedBy:   &by,
		ProjectID:   project,
		ProjectRole: role,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(inviteTTL), Valid: true},
	})
	if err != nil {
		return db.CreateInvitationRow{}, err
	}
	payload, err := json.Marshal(map[string]string{"link": s.link("/register", token), "project": projectName})
	if err != nil {
		return db.CreateInvitationRow{}, err
	}
	if _, err := s.q.EnqueueEmail(ctx, db.EnqueueEmailParams{ToEmail: email, Template: mail.TemplateInvite, Payload: payload}); err != nil {
		return db.CreateInvitationRow{}, err
	}
	return row, nil
}

func (s *Server) adminCreateInvitation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	var body createInviteBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	email := normalizeEmail(body.Email)
	if !validEmail(email) {
		writeError(w, r, http.StatusBadRequest, "Укажите корректный email.")
		return
	}
	var project *uuid.UUID
	var role *string
	name := ""
	if body.ProjectID != nil {
		pid, err := uuid.Parse(*body.ProjectID)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор проекта.")
			return
		}
		if body.ProjectRole == nil || !ValidMemberRole(*body.ProjectRole) {
			writeError(w, r, http.StatusBadRequest, "Роль участника должна быть editor или viewer.")
			return
		}
		row, err := s.q.GetProject(r.Context(), pid)
		if err != nil {
			if isNoRows(err) {
				writeError(w, r, http.StatusNotFound, "Проект не найден.")
				return
			}
			s.internal(w, r, "invite.project", err, "Не удалось создать приглашение. Повторите запрос.")
			return
		}
		project, role, name = &pid, body.ProjectRole, row.Name
	} else if body.ProjectRole != nil {
		writeError(w, r, http.StatusBadRequest, "Роль указывают вместе с проектом.")
		return
	}
	var row db.CreateInvitationRow
	err := s.inTx(r.Context(), func(tx *Server) error {
		var err error
		row, err = tx.invite(r.Context(), email, id.UserID, project, role, name)
		if err != nil {
			return err
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &id.UserID, Action: audit.ActionInviteCreate, TargetType: audit.TargetInvite, TargetID: row.ID.String(),
			Meta: map[string]any{"email": audit.Mask(email), "project_id": idOrEmpty(project)},
		})
	})
	if err != nil {
		s.internal(w, r, "invite.create", err, "Не удалось создать приглашение. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusCreated, inviteJSONFromRow(row, name))
}

func inviteJSONFromRow(row db.CreateInvitationRow, name string) invitationJSON {
	out := invitationJSON{
		ID:          row.ID.String(),
		Email:       row.Email,
		ProjectRole: row.ProjectRole,
		CreatedAt:   timestamptzPtr(row.CreatedAt),
		ExpiresAt:   timestamptzPtr(row.ExpiresAt),
	}
	if row.ProjectID != nil {
		id := row.ProjectID.String()
		out.ProjectID = &id
		out.ProjectName = &name
	}
	return out
}

func idOrEmpty(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func (s *Server) adminListInvitations(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	var project *uuid.UUID
	if raw := r.URL.Query().Get("project_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор проекта.")
			return
		}
		project = &id
	}
	limit := int32(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > inviteListCap {
			writeError(w, r, http.StatusBadRequest, "limit должен быть числом от 1 до 200.")
			return
		}
		limit = int32(n)
	}
	rows, err := s.q.ListInvitations(r.Context(), db.ListInvitationsParams{ProjectID: project, MaxRows: limit})
	if err != nil {
		s.internal(w, r, "invite.list", err, "Не удалось загрузить приглашения. Повторите запрос.")
		return
	}
	items := make([]invitationJSON, 0, len(rows))
	for _, row := range rows {
		item := invitationJSON{
			ID:          row.ID.String(),
			Email:       row.Email,
			ProjectName: row.ProjectName,
			ProjectRole: row.ProjectRole,
			CreatedAt:   timestamptzPtr(row.CreatedAt),
			ExpiresAt:   timestamptzPtr(row.ExpiresAt),
			AcceptedAt:  timestamptzPtr(row.AcceptedAt),
			RevokedAt:   timestamptzPtr(row.RevokedAt),
			InvitedBy:   row.InvitedByEmail,
		}
		if row.ProjectID != nil {
			id := row.ProjectID.String()
			item.ProjectID = &id
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) adminRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	inviteID, ok := parseChildID(w, r, "inviteId", "Некорректный идентификатор приглашения.")
	if !ok {
		return
	}
	err := s.inTx(r.Context(), func(tx *Server) error {
		n, err := tx.q.RevokeInvitation(r.Context(), inviteID)
		if err != nil {
			return err
		}
		if n == 0 {
			return &httpError{status: http.StatusNotFound, msg: "Открытое приглашение не найдено."}
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &id.UserID, Action: audit.ActionInviteRevoke, TargetType: audit.TargetInvite, TargetID: inviteID.String(),
		})
	})
	if err != nil {
		if e, ok := err.(*httpError); ok {
			writeError(w, r, e.status, e.msg)
			return
		}
		s.internal(w, r, "invite.revoke", err, "Не удалось отозвать приглашение. Повторите запрос.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type inspectJSON struct {
	Valid       bool    `json:"valid"`
	Email       string  `json:"email,omitempty"`
	ProjectName *string `json:"project_name,omitempty"`
	ProjectRole *string `json:"project_role,omitempty"`
	Reason      string  `json:"reason,omitempty"`
}

// inspectInvitation tells the registration page whose invitation the token is, without signing anyone in.
func (s *Server) inspectInvitation(w http.ResponseWriter, r *http.Request) {
	var body tokenBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if body.Token == "" {
		writeError(w, r, http.StatusBadRequest, "Ссылка неполная. Откройте её из письма целиком.")
		return
	}
	row, err := s.q.GetInvitationByTokenHash(r.Context(), auth.HashToken(body.Token))
	if err != nil {
		if isNoRows(err) {
			writeJSON(w, http.StatusOK, inspectJSON{Reason: "Приглашение не найдено. Проверьте ссылку из письма."})
			return
		}
		s.internal(w, r, "invite.inspect", err, "Не удалось проверить приглашение. Повторите запрос.")
		return
	}
	switch {
	case row.RevokedAt.Valid:
		writeJSON(w, http.StatusOK, inspectJSON{Reason: "Приглашение отозвано. Попросите новое."})
	case row.AcceptedAt.Valid:
		writeJSON(w, http.StatusOK, inspectJSON{Reason: "Приглашение уже использовано. Войдите в аккаунт."})
	case !row.ExpiresAt.Valid || time.Now().After(row.ExpiresAt.Time):
		writeJSON(w, http.StatusOK, inspectJSON{Reason: "Срок приглашения истёк. Попросите новое."})
	default:
		writeJSON(w, http.StatusOK, inspectJSON{
			Valid: true, Email: row.Email, ProjectName: row.ProjectName, ProjectRole: row.ProjectRole,
		})
	}
}

// acceptInvitations turns every open invitation for the address into membership, right after registration.
func (s *Server) acceptInvitations(ctx context.Context, reqID string, userID uuid.UUID, email string) error {
	rows, err := s.q.ListOpenInvitationsForEmail(ctx, email)
	if err != nil {
		return err
	}
	for _, row := range rows {
		accepted, err := s.q.AcceptInvitation(ctx, db.AcceptInvitationParams{ID: row.ID, UserID: userID})
		if err != nil {
			if isNoRows(err) {
				continue
			}
			return err
		}
		if accepted.ProjectID != nil && accepted.ProjectRole != nil {
			if _, err := s.q.AddProjectMember(ctx, db.AddProjectMemberParams{
				ProjectID: *accepted.ProjectID, UserID: userID, Role: *accepted.ProjectRole, AddedBy: row.InvitedBy,
			}); err != nil {
				return err
			}
		}
		if err := s.recordAudit(ctx, reqID, audit.Event{
			Actor: &userID, Action: audit.ActionInviteAccept, TargetType: audit.TargetInvite, TargetID: row.ID.String(),
			Meta: map[string]any{"project_id": idOrEmpty(accepted.ProjectID)},
		}); err != nil {
			return err
		}
	}
	return nil
}

// inviteFor checks the token a registration carries and returns the invitation it belongs to.
func (s *Server) inviteFor(ctx context.Context, token, email string) (db.GetInvitationByTokenHashRow, bool) {
	if token == "" {
		return db.GetInvitationByTokenHashRow{}, false
	}
	row, err := s.q.GetInvitationByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return db.GetInvitationByTokenHashRow{}, false
	}
	if row.RevokedAt.Valid || row.AcceptedAt.Valid || !row.ExpiresAt.Valid || time.Now().After(row.ExpiresAt.Time) {
		return db.GetInvitationByTokenHashRow{}, false
	}
	if normalizeEmail(row.Email) != email {
		return db.GetInvitationByTokenHashRow{}, false
	}
	return row, true
}

func (s *Server) registrationMode() string {
	if s.cfg.RegistrationMode == config.RegistrationInvite {
		return config.RegistrationInvite
	}
	return config.RegistrationOpen
}

func (s *Server) adminOnly(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	if !s.requireAdmin(w, r) {
		return auth.Identity{}, false
	}
	return auth.FromRequest(r), true
}
