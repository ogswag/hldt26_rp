package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/config"
	"moscow_hackathon_2026/api/internal/db"
)

type authBody struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	InviteToken string `json:"invite_token,omitempty"`
}

type userJSON struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func validEmail(s string) bool {
	if len(s) < 3 || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at < 1 || at == len(s)-1 {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	return true
}

func userFromRow(id, email, role string) userJSON {
	return userJSON{ID: id, Email: email, Role: role}
}

// sessionUser is what a new session needs from whichever query loaded the account.
type sessionUser struct {
	ID    uuid.UUID
	Email string
	Role  string
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body authBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	email := normalizeEmail(body.Email)
	if !validEmail(email) {
		writeError(w, r, http.StatusBadRequest, "Укажите корректный email.")
		return
	}
	if len(body.Password) < 8 || len(body.Password) > 72 {
		writeError(w, r, http.StatusBadRequest, "Пароль должен быть от 8 до 72 символов.")
		return
	}
	_, invited := s.inviteFor(r.Context(), body.InviteToken, email)
	if s.registrationMode() == config.RegistrationInvite && !invited {
		writeCode(w, r, http.StatusForbidden, "invite_required",
			"Регистрация только по приглашению. Откройте ссылку из письма или попросите приглашение.")
		return
	}
	hash, err := auth.HashPassword(body.Password, s.cfg.BcryptCost)
	if err != nil {
		s.log.Error("auth.register.hash", "op", "auth.register", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось создать аккаунт. Повторите запрос.")
		return
	}
	// An invited address is already proven: the letter reached it.
	var verified pgtype.Timestamptz
	if invited {
		verified = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	var row db.CreateUserRow
	err = s.inTx(r.Context(), func(tx *Server) error {
		var err error
		row, err = tx.q.CreateUser(r.Context(), db.CreateUserParams{
			Email:           email,
			PasswordHash:    hash,
			Role:            auth.RoleUser,
			EmailVerifiedAt: verified,
		})
		if err != nil {
			return err
		}
		return tx.acceptInvitations(r.Context(), requestID(r), row.ID, email)
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, r, http.StatusConflict, "Этот email уже зарегистрирован. Войдите или укажите другой.")
			return
		}
		s.log.Error("auth.register", "op", "auth.register", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось создать аккаунт. Повторите запрос.")
		return
	}
	if !invited {
		s.sendVerification(r, row.ID, email)
	}
	s.startSession(w, r, sessionUser{ID: row.ID, Email: row.Email, Role: row.Role}, http.StatusOK)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body authBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	email := normalizeEmail(body.Email)
	row, err := s.q.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Same bcrypt work as a real account, so the response time does not reveal which emails exist.
			auth.CheckPassword(s.dummyHash(), body.Password)
			writeError(w, r, http.StatusUnauthorized, "Неверный email или пароль.")
			return
		}
		s.log.Error("auth.login", "op", "auth.login", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось войти. Повторите запрос.")
		return
	}
	if !auth.CheckPassword(row.PasswordHash, body.Password) {
		writeError(w, r, http.StatusUnauthorized, "Неверный email или пароль.")
		return
	}
	s.startSession(w, r, sessionUser{ID: row.ID, Email: row.Email, Role: row.Role}, http.StatusOK)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	id, ok := signedIn(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, authJSON{User: userFromRow(id.UserID.String(), id.Email, id.Role), CSRFToken: id.CSRF})
}
