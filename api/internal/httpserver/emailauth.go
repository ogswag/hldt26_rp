package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/mail"
)

const (
	verifyTTL = 48 * time.Hour
	resetTTL  = time.Hour
	inviteTTL = 7 * 24 * time.Hour
)

// resetSame is the one answer a password request gives, so it never says which addresses exist.
const resetSame = "Если такой аккаунт есть, письмо со ссылкой уже отправлено. Проверьте почту."

type tokenBody struct {
	Token string `json:"token"`
}

type emailBody struct {
	Email string `json:"email"`
}

type resetConfirmBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// link builds the address a letter carries. The token sits in the fragment, so proxies and logs never see it.
func (s *Server) link(path, token string) string {
	origin := strings.TrimRight(s.cfg.PublicOrigin, "/")
	if origin == "" {
		origin = "http://localhost"
	}
	return origin + path + "#token=" + token
}

// issueEmailToken stores a one-time token for the user and queues the letter that carries it.
func (s *Server) issueEmailToken(ctx context.Context, userID uuid.UUID, email, purpose string) error {
	ttl, path, template := verifyTTL, "/verify-email", mail.TemplateVerifyEmail
	if purpose == "reset_password" {
		ttl, path, template = resetTTL, "/reset-password", mail.TemplateResetPassord
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"link": s.link(path, token)})
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *Server) error {
		if err := tx.q.DeleteUserEmailTokens(ctx, db.DeleteUserEmailTokensParams{UserID: userID, Purpose: purpose}); err != nil {
			return err
		}
		if _, err := tx.q.CreateEmailToken(ctx, db.CreateEmailTokenParams{
			UserID: userID, Purpose: purpose, TokenHash: hash,
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(ttl), Valid: true},
		}); err != nil {
			return err
		}
		_, err := tx.q.EnqueueEmail(ctx, db.EnqueueEmailParams{ToEmail: email, Template: template, Payload: payload})
		return err
	})
}

// sendVerification queues the confirmation letter for a fresh account; a failure must not fail registration.
func (s *Server) sendVerification(r *http.Request, userID uuid.UUID, email string) {
	if err := s.issueEmailToken(r.Context(), userID, email, "verify_email"); err != nil {
		s.log.Error("auth.verify.queue", "op", "auth.verify", "request_id", requestID(r), "err", err)
	}
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var body tokenBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if body.Token == "" {
		writeError(w, r, http.StatusBadRequest, "Ссылка неполная. Откройте её из письма целиком.")
		return
	}
	var userID uuid.UUID
	err := s.inTx(r.Context(), func(tx *Server) error {
		var err error
		userID, err = tx.q.UseEmailToken(r.Context(), db.UseEmailTokenParams{
			TokenHash: auth.HashToken(body.Token), Purpose: "verify_email",
		})
		if err != nil {
			return err
		}
		if _, err := tx.q.MarkEmailVerified(r.Context(), userID); err != nil {
			return err
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &userID, Action: audit.ActionEmailVerified, TargetType: audit.TargetUser, TargetID: userID.String(),
		})
	})
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusGone, "Ссылка уже использована или истекла. Запросите письмо снова.")
			return
		}
		s.internal(w, r, "auth.verify", err, "Не удалось подтвердить email. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	id, ok := signedIn(w, r)
	if !ok {
		return
	}
	row, err := s.q.GetUserByID(r.Context(), id.UserID)
	if err != nil {
		s.internal(w, r, "auth.verify.resend", err, "Не удалось отправить письмо. Повторите запрос.")
		return
	}
	if row.EmailVerifiedAt.Valid {
		writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
		return
	}
	if err := s.issueEmailToken(r.Context(), id.UserID, row.Email, "verify_email"); err != nil {
		s.internal(w, r, "auth.verify.resend", err, "Не удалось отправить письмо. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
}

func (s *Server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body emailBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	email := normalizeEmail(body.Email)
	row, err := s.q.GetUserByEmail(r.Context(), email)
	switch {
	case isNoRows(err):
	case err != nil:
		s.internal(w, r, "auth.reset.request", err, "Не удалось отправить письмо. Повторите запрос.")
		return
	default:
		if err := s.issueEmailToken(r.Context(), row.ID, row.Email, "reset_password"); err != nil {
			s.internal(w, r, "auth.reset.request", err, "Не удалось отправить письмо. Повторите запрос.")
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sent", "message": resetSame})
}

func (s *Server) confirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body resetConfirmBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if body.Token == "" {
		writeError(w, r, http.StatusBadRequest, "Ссылка неполная. Откройте её из письма целиком.")
		return
	}
	if len(body.Password) < 8 || len(body.Password) > 72 {
		writeError(w, r, http.StatusBadRequest, "Пароль должен быть от 8 до 72 символов.")
		return
	}
	hash, err := auth.HashPassword(body.Password, s.cfg.BcryptCost)
	if err != nil {
		s.internal(w, r, "auth.reset.hash", err, "Не удалось задать пароль. Повторите запрос.")
		return
	}
	var userID uuid.UUID
	err = s.inTx(r.Context(), func(tx *Server) error {
		var err error
		userID, err = tx.q.UseEmailToken(r.Context(), db.UseEmailTokenParams{
			TokenHash: auth.HashToken(body.Token), Purpose: "reset_password",
		})
		if err != nil {
			return err
		}
		if _, err := tx.q.SetUserPassword(r.Context(), db.SetUserPasswordParams{ID: userID, PasswordHash: hash}); err != nil {
			return err
		}
		if _, err := tx.q.RevokeUserSessions(r.Context(), userID); err != nil {
			return err
		}
		if _, err := tx.q.MarkEmailVerified(r.Context(), userID); err != nil {
			return err
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &userID, Action: audit.ActionPasswordReset, TargetType: audit.TargetUser, TargetID: userID.String(),
		})
	})
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusGone, "Ссылка уже использована или истекла. Запросите письмо снова.")
			return
		}
		s.internal(w, r, "auth.reset", err, "Не удалось задать пароль. Повторите запрос.")
		return
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "changed"})
}

type outboxItem struct {
	ID        string            `json:"id"`
	ToEmail   string            `json:"to_email"`
	Template  string            `json:"template"`
	Payload   map[string]string `json:"payload"`
	Status    string            `json:"status"`
	Attempts  int32             `json:"attempts"`
	CreatedAt *time.Time        `json:"created_at"`
}

// devOutbox shows the letters of one address. It is served only when the API runs locally.
func (s *Server) devOutbox(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AppEnv != "local" {
		writeError(w, r, http.StatusNotFound, "Не найдено.")
		return
	}
	email := normalizeEmail(r.URL.Query().Get("to"))
	if email == "" {
		writeError(w, r, http.StatusBadRequest, "Укажите to.")
		return
	}
	rows, err := s.q.ListOutboxByEmail(r.Context(), db.ListOutboxByEmailParams{ToEmail: email, MaxRows: 20})
	if err != nil {
		s.internal(w, r, "mail.outbox", err, "Не удалось прочитать письма. Повторите запрос.")
		return
	}
	items := make([]outboxItem, 0, len(rows))
	for _, row := range rows {
		payload := map[string]string{}
		_ = json.Unmarshal(row.Payload, &payload)
		items = append(items, outboxItem{
			ID:        row.ID.String(),
			ToEmail:   row.ToEmail,
			Template:  row.Template,
			Payload:   payload,
			Status:    row.Status,
			Attempts:  row.Attempts,
			CreatedAt: timestamptzPtr(row.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
