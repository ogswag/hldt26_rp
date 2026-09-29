package httpserver

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
)

type memberJSON struct {
	UserID    string     `json:"user_id"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	CreatedAt *time.Time `json:"created_at"`
}

type addMemberBody struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type memberRoleBody struct {
	Role string `json:"role"`
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	ctx := r.Context()
	items := []memberJSON{}
	if row.UserID != nil {
		owner, err := s.q.GetUserByID(ctx, *row.UserID)
		if err != nil && !isNoRows(err) {
			s.internal(w, r, "members.owner", err, "Не удалось загрузить участников. Повторите запрос.")
			return
		}
		if err == nil {
			items = append(items, memberJSON{UserID: owner.ID.String(), Email: owner.Email, Role: RoleOwner, CreatedAt: timestamptzPtr(row.CreatedAt)})
		}
	}
	rows, err := s.q.ListProjectMembers(ctx, row.ID)
	if err != nil {
		s.internal(w, r, "members.list", err, "Не удалось загрузить участников. Повторите запрос.")
		return
	}
	for _, m := range rows {
		items = append(items, memberJSON{UserID: m.UserID.String(), Email: m.Email, Role: m.Role, CreatedAt: timestamptzPtr(m.CreatedAt)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	var body addMemberBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if !ValidMemberRole(body.Role) {
		writeError(w, r, http.StatusBadRequest, "Роль участника должна быть editor или viewer.")
		return
	}
	email := normalizeEmail(body.Email)
	if !validEmail(email) {
		writeError(w, r, http.StatusBadRequest, "Укажите корректный email.")
		return
	}
	ctx := r.Context()
	actor := auth.FromRequest(r).UserID
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if isNoRows(err) {
			s.inviteMember(w, r, row, email, body.Role, actor)
			return
		}
		s.internal(w, r, "members.add.user", err, "Не удалось добавить участника. Повторите запрос.")
		return
	}
	if row.UserID != nil && *row.UserID == user.ID {
		writeError(w, r, http.StatusBadRequest, "Владелец уже имеет полный доступ к проекту.")
		return
	}
	err = s.inTx(ctx, func(tx *Server) error {
		n, err := tx.q.AddProjectMember(ctx, db.AddProjectMemberParams{ProjectID: row.ID, UserID: user.ID, Role: body.Role, AddedBy: &actor})
		if err != nil {
			return err
		}
		if n == 0 {
			return &httpError{status: http.StatusConflict, msg: "Этот пользователь уже участник проекта. Измените его роль в списке."}
		}
		return tx.recordAudit(ctx, requestID(r), audit.Event{
			Actor: &actor, Action: audit.ActionMemberAdd, TargetType: audit.TargetProject, TargetID: row.ID.String(),
			Meta: map[string]any{"user_id": user.ID.String(), "role": body.Role},
		})
	})
	if err != nil {
		if e, ok := err.(*httpError); ok {
			writeError(w, r, e.status, e.msg)
			return
		}
		s.internal(w, r, "members.add", err, "Не удалось добавить участника. Повторите запрос.")
		return
	}
	now := time.Now().UTC()
	writeJSON(w, http.StatusCreated, memberJSON{UserID: user.ID.String(), Email: user.Email, Role: body.Role, CreatedAt: &now})
}

// inviteMember invites an address that has no account yet; accepting it makes the person a member at once.
func (s *Server) inviteMember(w http.ResponseWriter, r *http.Request, row db.GetProjectRow, email, role string, actor uuid.UUID) {
	var invite db.CreateInvitationRow
	err := s.inTx(r.Context(), func(tx *Server) error {
		var err error
		invite, err = tx.invite(r.Context(), email, actor, &row.ID, &role, row.Name)
		if err != nil {
			return err
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &actor, Action: audit.ActionInviteCreate, TargetType: audit.TargetInvite, TargetID: invite.ID.String(),
			Meta: map[string]any{"email": audit.Mask(email), "project_id": row.ID.String(), "role": role},
		})
	})
	if err != nil {
		s.internal(w, r, "members.invite", err, "Не удалось пригласить участника. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":     "invited",
		"invitation": inviteJSONFromRow(invite, row.Name),
		"message":    "Аккаунта с таким email нет. Мы отправили приглашение: после регистрации человек станет участником проекта.",
	})
}

func (s *Server) setMemberRole(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	uid, ok := parseChildID(w, r, "userId", "Некорректный идентификатор пользователя.")
	if !ok {
		return
	}
	var body memberRoleBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if !ValidMemberRole(body.Role) {
		writeError(w, r, http.StatusBadRequest, "Роль участника должна быть editor или viewer.")
		return
	}
	actor := auth.FromRequest(r).UserID
	err := s.inTx(r.Context(), func(tx *Server) error {
		n, err := tx.q.SetProjectMemberRole(r.Context(), db.SetProjectMemberRoleParams{ProjectID: row.ID, UserID: uid, Role: body.Role})
		if err != nil {
			return err
		}
		if n == 0 {
			return &httpError{status: http.StatusNotFound, msg: "Участник не найден."}
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &actor, Action: audit.ActionMemberRole, TargetType: audit.TargetProject, TargetID: row.ID.String(),
			Meta: map[string]any{"user_id": uid.String(), "role": body.Role},
		})
	})
	if err != nil {
		if e, ok := err.(*httpError); ok {
			writeError(w, r, e.status, e.msg)
			return
		}
		s.internal(w, r, "members.role", err, "Не удалось изменить роль. Повторите запрос.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeMember lets the owner remove anyone and a member remove only themselves.
func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	uid, ok := parseChildID(w, r, "userId", "Некорректный идентификатор пользователя.")
	if !ok {
		return
	}
	if roleFrom(r) != RoleOwner && uid != auth.FromRequest(r).UserID {
		writeJSON(w, http.StatusForbidden, errorBody{Error: forbiddenText(accessOwner), Code: "forbidden", RequestID: requestID(r)})
		return
	}
	actor := auth.FromRequest(r).UserID
	err := s.inTx(r.Context(), func(tx *Server) error {
		n, err := tx.q.RemoveProjectMember(r.Context(), db.RemoveProjectMemberParams{ProjectID: row.ID, UserID: uid})
		if err != nil {
			return err
		}
		if n == 0 {
			return &httpError{status: http.StatusNotFound, msg: "Участник не найден."}
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &actor, Action: audit.ActionMemberRemove, TargetType: audit.TargetProject, TargetID: row.ID.String(),
			Meta: map[string]any{"user_id": uid.String()},
		})
	})
	if err != nil {
		if e, ok := err.(*httpError); ok {
			writeError(w, r, e.status, e.msg)
			return
		}
		s.internal(w, r, "members.remove", err, "Не удалось удалить участника. Повторите запрос.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
