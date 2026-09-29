package httpserver

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
)

const (
	cookieSecureName = "__Host-session"
	cookiePlainName  = "session"
	csrfHeader       = "X-CSRF-Token"
	touchEvery       = 5 * time.Minute
	userAgentMax     = 512
)

func (s *Server) cookieName() string {
	if s.cfg.CookieSecure {
		return cookieSecureName
	}
	return cookiePlainName
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName(),
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		Secure:   s.cfg.CookieSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   s.cfg.CookieSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// authEntry routes treat a stale cookie as no cookie: they are how the caller gets a new session.
func authEntry(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/auth/login", "/api/auth/register", "/api/auth/logout":
		return true
	}
	return false
}

func csrfExempt(r *http.Request) bool {
	return r.Method == http.MethodPost && (r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/register")
}

// identity resolves the session cookie. The role comes from the database on every request, so a role change applies at once.
func (s *Server) identity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.cookieName())
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r.WithContext(auth.ContextWithIdentity(r.Context(), auth.Guest())))
			return
		}
		id, ok, err := s.lookupSession(r, c.Value)
		if err != nil {
			s.log.Error("auth.session", "op", "auth.session", "request_id", requestID(r), "err", err)
			writeError(w, r, http.StatusInternalServerError, "Не удалось проверить сессию. Повторите запрос.")
			return
		}
		if !ok {
			s.clearSessionCookie(w)
			if authEntry(r) {
				next.ServeHTTP(w, r.WithContext(auth.ContextWithIdentity(r.Context(), auth.Guest())))
				return
			}
			writeCode(w, r, http.StatusUnauthorized, "session_expired", "Сессия истекла. Войдите снова.")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.ContextWithIdentity(r.Context(), id)))
	})
}

func (s *Server) lookupSession(r *http.Request, token string) (auth.Identity, bool, error) {
	row, err := s.q.GetSessionByTokenHash(r.Context(), auth.HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Identity{}, false, nil
	}
	if err != nil {
		return auth.Identity{}, false, err
	}
	now := time.Now()
	if !now.Before(row.ExpiresAt.Time) || !now.Before(row.LastSeenAt.Time.Add(s.cfg.SessionIdle)) {
		return auth.Identity{}, false, nil
	}
	if now.Sub(row.LastSeenAt.Time) >= touchEvery {
		if err := s.q.TouchSession(r.Context(), row.ID); err != nil {
			s.log.Warn("auth.touch", "op", "auth.touch", "request_id", requestID(r), "err", err)
		}
	}
	return auth.Identity{
		UserID:    row.UserID,
		Email:     row.Email,
		Role:      row.Role,
		SessionID: row.ID,
		CSRF:      row.CsrfToken,
	}, true, nil
}

func unsafeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// csrf rejects state-changing API requests from other sites and, with a session, without the session's token.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !unsafeMethod(r.Method) || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		id := auth.FromRequest(r)
		hasOriginSignal := r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Origin") != ""
		if (hasOriginSignal || !id.IsGuest()) && !s.sameOrigin(r) {
			writeCode(w, r, http.StatusForbidden, "csrf", "Запрос отклонён: он пришёл не со страницы приложения.")
			return
		}
		if !id.IsGuest() && !csrfExempt(r) && subtle.ConstantTimeCompare([]byte(r.Header.Get(csrfHeader)), []byte(id.CSRF)) != 1 {
			writeCode(w, r, http.StatusForbidden, "csrf", "Страница устарела. Обновите её и повторите.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin trusts Sec-Fetch-Site, then Origin. Authenticated requests without either signal are rejected.
func (s *Server) sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin == s.cfg.PublicOrigin
	}
	return false
}

// clientIP reads X-Real-IP only from a proxy on a private network, such as nginx in compose.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer != nil && (peer.IsLoopback() || peer.IsPrivate()) {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
	}
	return host
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

type authJSON struct {
	User      userJSON `json:"user"`
	CSRFToken string   `json:"csrf_token"`
}

// startSession replaces the caller's session, if any, with a new one for user.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user sessionUser, status int) {
	if prev := auth.FromRequest(r); !prev.IsGuest() {
		if _, err := s.q.RevokeSession(r.Context(), db.RevokeSessionParams{ID: prev.SessionID, UserID: prev.UserID}); err != nil {
			s.log.Warn("auth.revoke", "op", "auth.login", "request_id", requestID(r), "err", err)
		}
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		s.authFailed(w, r, err)
		return
	}
	csrf, err := auth.NewCSRF()
	if err != nil {
		s.authFailed(w, r, err)
		return
	}
	expires := time.Now().Add(s.cfg.SessionMax).UTC()
	if _, err := s.q.CreateSession(r.Context(), db.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: hash,
		CsrfToken: csrf,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		UserAgent: truncateRunes(r.UserAgent(), userAgentMax),
		IpPrefix:  auth.IPPrefix(clientIP(r)),
	}); err != nil {
		s.authFailed(w, r, err)
		return
	}
	s.setSessionCookie(w, token, expires)
	writeJSON(w, status, authJSON{User: userFromRow(user.ID.String(), user.Email, user.Role), CSRFToken: csrf})
}

func (s *Server) authFailed(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("auth.session.create", "op", "auth.session", "request_id", requestID(r), "err", err)
	writeError(w, r, http.StatusInternalServerError, "Не удалось войти. Повторите запрос.")
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if id := auth.FromRequest(r); !id.IsGuest() {
		if err := s.inTx(r.Context(), func(tx *Server) error {
			if _, err := tx.q.RevokeSession(r.Context(), db.RevokeSessionParams{ID: id.SessionID, UserID: id.UserID}); err != nil {
				return err
			}
			return tx.recordAudit(r.Context(), requestID(r), audit.Event{
				Actor: &id.UserID, Action: audit.ActionLogout, TargetType: audit.TargetSession, TargetID: id.SessionID.String(),
			})
		}); err != nil {
			s.log.Error("auth.logout", "op", "auth.logout", "request_id", requestID(r), "err", err)
			writeError(w, r, http.StatusInternalServerError, "Не удалось выйти. Повторите запрос.")
			return
		}
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

type sessionJSON struct {
	ID         string    `json:"id"`
	Current    bool      `json:"current"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	UserAgent  string    `json:"user_agent"`
	IPPrefix   string    `json:"ip_prefix"`
}

func signedIn(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	id := auth.FromRequest(r)
	if id.IsGuest() {
		writeError(w, r, http.StatusUnauthorized, "Войдите в аккаунт.")
		return id, false
	}
	return id, true
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := signedIn(w, r)
	if !ok {
		return
	}
	rows, err := s.q.ListUserSessions(r.Context(), id.UserID)
	if err != nil {
		s.log.Error("auth.sessions", "op", "auth.sessions", "request_id", requestID(r), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить сессии. Повторите запрос.")
		return
	}
	now := time.Now()
	out := make([]sessionJSON, 0, len(rows))
	for _, row := range rows {
		if !now.Before(row.LastSeenAt.Time.Add(s.cfg.SessionIdle)) {
			continue
		}
		out = append(out, sessionJSON{
			ID:         row.ID.String(),
			Current:    row.ID == id.SessionID,
			CreatedAt:  row.CreatedAt.Time,
			LastSeenAt: row.LastSeenAt.Time,
			ExpiresAt:  row.ExpiresAt.Time,
			UserAgent:  row.UserAgent,
			IPPrefix:   row.IpPrefix,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	id, ok := signedIn(w, r)
	if !ok {
		return
	}
	sid, err := uuid.Parse(chi.URLParam(r, "sessionId"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "Сессия не найдена.")
		return
	}
	err = s.inTx(r.Context(), func(tx *Server) error {
		n, err := tx.q.RevokeSession(r.Context(), db.RevokeSessionParams{ID: sid, UserID: id.UserID})
		if err != nil {
			return err
		}
		if n == 0 {
			return &httpError{status: http.StatusNotFound, msg: "Сессия не найдена."}
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &id.UserID, Action: audit.ActionSessionRevoke, TargetType: audit.TargetSession, TargetID: sid.String(),
		})
	})
	if err != nil {
		if e, ok := err.(*httpError); ok {
			writeError(w, r, e.status, e.msg)
			return
		}
		s.log.Error("auth.revoke", "op", "auth.revoke", "request_id", requestID(r), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось завершить сессию. Повторите запрос.")
		return
	}
	if sid == id.SessionID {
		s.clearSessionCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := signedIn(w, r)
	if !ok {
		return
	}
	n, err := s.q.RevokeOtherSessions(r.Context(), db.RevokeOtherSessionsParams{UserID: id.UserID, ID: id.SessionID})
	if err != nil {
		s.log.Error("auth.revoke_others", "op", "auth.revoke_others", "request_id", requestID(r), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось завершить сессии. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"revoked": n})
}
