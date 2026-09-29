package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/testdb"
)

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return e.Code
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", name, rec.Header().Values("Set-Cookie"))
	return nil
}

func TestSessionCookieAttributes(t *testing.T) {
	_, q := testdb.New(t, "../../../data")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := testConfig()
	cfg.CookieSecure = true
	h := New(cfg, log, q, nil, nil, nil)

	body := strings.NewReader(`{"email":"cookie@example.com","password":"secret-password-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	c := sessionCookie(t, rec, cookieSecureName)
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
		t.Fatalf("cookie attributes: %+v", c)
	}
	if left := time.Until(c.Expires); left < 29*24*time.Hour || left > 31*24*time.Hour {
		t.Fatalf("cookie expires in %s, want 30 days", left)
	}
	if strings.Contains(rec.Body.String(), c.Value) {
		t.Fatal("the session token must not be in the response body")
	}
	var auth authJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &auth); err != nil || auth.CSRFToken == "" || auth.User.Email != "cookie@example.com" {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestSessionLifecycle(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	alice := register(t, env.h, "alice@example.com")

	var me authJSON
	alice.json(http.MethodGet, "/api/auth/me", nil, http.StatusOK, &me)
	if me.User.Email != "alice@example.com" || me.CSRFToken != alice.csrf {
		t.Fatalf("me: %+v", me)
	}

	// A second device of the same user.
	phone := &itClient{t: t, h: env.h}
	var login authJSON
	phone.json(http.MethodPost, "/api/auth/login", map[string]string{"email": "ALICE@example.com ", "password": "secret-password-1"}, http.StatusOK, &login)
	phone.csrf = login.CSRFToken
	if phone.cookie.Value == alice.cookie.Value || phone.csrf == alice.csrf {
		t.Fatal("each login must get its own session")
	}

	var list []sessionJSON
	alice.json(http.MethodGet, "/api/auth/sessions", nil, http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("sessions: %+v", list)
	}
	current := 0
	for _, s := range list {
		if s.Current {
			current++
		}
		if s.IPPrefix != "192.0.2.0/24" {
			t.Fatalf("ip prefix %q", s.IPPrefix)
		}
	}
	if current != 1 {
		t.Fatalf("exactly one current session: %+v", list)
	}

	// Another user cannot revoke Alice's session.
	bob := register(t, env.h, "bob@example.com")
	var phoneSession string
	for _, s := range list {
		if !s.Current {
			phoneSession = s.ID
		}
	}
	bob.json(http.MethodDelete, "/api/auth/sessions/"+phoneSession, nil, http.StatusNotFound, nil)
	phone.json(http.MethodGet, "/api/auth/me", nil, http.StatusOK, nil)

	var revoked struct {
		Revoked int64 `json:"revoked"`
	}
	alice.json(http.MethodPost, "/api/auth/sessions/revoke-others", nil, http.StatusOK, &revoked)
	if revoked.Revoked != 1 {
		t.Fatalf("revoked %d", revoked.Revoked)
	}
	stale := phone.cookie
	rec := phone.do(http.MethodGet, "/api/projects", nil, false)
	if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "session_expired" {
		t.Fatalf("revoked session: %d %s", rec.Code, rec.Body.String())
	}
	if c := sessionCookie(t, rec, cookiePlainName); c.MaxAge >= 0 {
		t.Fatalf("stale cookie must be cleared: %+v", c)
	}
	if phone.cookie != nil {
		t.Fatal("client must drop the cleared cookie")
	}

	// Login with a stale cookie still works and replaces it.
	phone.cookie = stale
	phone.json(http.MethodPost, "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "secret-password-1"}, http.StatusOK, &login)
	phone.csrf = login.CSRFToken
	if phone.cookie == nil || phone.cookie.Value == stale.Value {
		t.Fatal("login must set a new cookie")
	}

	// Logging in again from a device ends that device's previous session.
	before := phone.cookie
	phone.json(http.MethodPost, "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "secret-password-1"}, http.StatusOK, &login)
	phone.csrf = login.CSRFToken
	old := &itClient{t: t, h: env.h, cookie: before}
	if rec := old.do(http.MethodGet, "/api/auth/me", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("replaced session still works: %d", rec.Code)
	}

	phone.json(http.MethodPost, "/api/auth/logout", nil, http.StatusNoContent, nil)
	if phone.cookie != nil {
		t.Fatal("logout must clear the cookie")
	}
	// Logout without a session is harmless.
	phone.json(http.MethodPost, "/api/auth/logout", nil, http.StatusNoContent, nil)

	// Expiry: absolute and idle.
	if _, err := env.db.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 second' WHERE user_id = (SELECT id FROM users WHERE email = 'bob@example.com')`); err != nil {
		t.Fatal(err)
	}
	if rec := bob.do(http.MethodGet, "/api/auth/me", nil, false); rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "session_expired" {
		t.Fatalf("expired session: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := env.db.Exec(ctx, `UPDATE sessions SET last_seen_at = now() - interval '169 hours' WHERE revoked_at IS NULL AND user_id = (SELECT id FROM users WHERE email = 'alice@example.com')`); err != nil {
		t.Fatal(err)
	}
	if rec := alice.do(http.MethodGet, "/api/auth/me", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("idle session: %d %s", rec.Code, rec.Body.String())
	}

	// last_seen_at moves after five minutes of silence.
	carol := register(t, env.h, "carol@example.com")
	if _, err := env.db.Exec(ctx, `UPDATE sessions SET last_seen_at = now() - interval '10 minutes' WHERE user_id = (SELECT id FROM users WHERE email = 'carol@example.com')`); err != nil {
		t.Fatal(err)
	}
	carol.json(http.MethodGet, "/api/auth/me", nil, http.StatusOK, nil)
	var fresh bool
	if err := env.db.QueryRow(ctx, `SELECT last_seen_at > now() - interval '1 minute' FROM sessions WHERE user_id = (SELECT id FROM users WHERE email = 'carol@example.com')`).Scan(&fresh); err != nil || !fresh {
		t.Fatalf("last_seen_at not touched: %v", err)
	}

	// Unknown email and wrong password look the same.
	unknown := phone.do(http.MethodPost, "/api/auth/login", map[string]string{"email": "nobody@example.com", "password": "secret-password-1"}, false)
	wrong := phone.do(http.MethodPost, "/api/auth/login", map[string]string{"email": "carol@example.com", "password": "wrong-password"}, false)
	if unknown.Code != http.StatusUnauthorized || wrong.Code != http.StatusUnauthorized || errorText(t, unknown) != errorText(t, wrong) {
		t.Fatalf("login failures differ: %s / %s", unknown.Body.String(), wrong.Body.String())
	}
}

func errorText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e errorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	return e.Error
}

func TestCSRF(t *testing.T) {
	env := newEnv(t)
	user := register(t, env.h, "csrf@example.com")
	create := map[string]any{"name": "Склад", "object_type": "warehouse"}

	send := func(c *itClient, token string, headers map[string]string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(create)
		req := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		if c.cookie != nil {
			req.AddCookie(c.cookie)
		}
		if token != "" {
			req.Header.Set(csrfHeader, token)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		env.h.ServeHTTP(rec, req)
		return rec
	}
	for name, tc := range map[string]struct {
		token   string
		headers map[string]string
		want    int
	}{
		"no token":       {"", map[string]string{"Origin": "http://localhost"}, http.StatusForbidden},
		"wrong token":    {"not-the-token", map[string]string{"Origin": "http://localhost"}, http.StatusForbidden},
		"no origin":      {user.csrf, nil, http.StatusForbidden},
		"same origin":    {user.csrf, map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://evil.example"}, http.StatusCreated},
		"cross site":     {user.csrf, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		"same site":      {user.csrf, map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		"public origin":  {user.csrf, map[string]string{"Origin": "http://localhost"}, http.StatusCreated},
		"foreign origin": {user.csrf, map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		"null origin":    {user.csrf, map[string]string{"Origin": "null"}, http.StatusForbidden},
	} {
		rec := send(user, tc.token, tc.headers)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d: %s", name, rec.Code, tc.want, rec.Body.String())
			continue
		}
		if tc.want == http.StatusForbidden && errorCode(t, rec) != "csrf" {
			t.Errorf("%s: code %q", name, errorCode(t, rec))
		}
	}
	other := register(t, env.h, "csrf-other@example.com")
	if rec := send(user, other.csrf, map[string]string{"Origin": "http://localhost"}); rec.Code != http.StatusForbidden {
		t.Fatalf("another session's token: %d", rec.Code)
	}

	// Guests have no token, but a cross-site POST is still refused.
	guest := &itClient{t: t, h: env.h}
	if rec := send(guest, "", map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusForbidden || errorCode(t, rec) != "csrf" {
		t.Fatalf("cross-site guest: %d %s", rec.Code, rec.Body.String())
	}
	// Login from another site is refused too (login CSRF).
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"csrf@example.com","password":"secret-password-1"}`))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	env.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site login: %d", rec.Code)
	}
	// Reads need no token.
	user.csrf = ""
	user.json(http.MethodGet, "/api/projects", nil, http.StatusOK, nil)
}

func TestRoleChangeAppliesImmediately(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	owner := register(t, env.h, "rc-owner@example.com")
	member := register(t, env.h, "rc-member@example.com")

	member.json(http.MethodPost, "/api/admin/solutions", map[string]any{}, http.StatusForbidden, nil)
	if _, err := env.db.Exec(ctx, `UPDATE users SET role = 'admin' WHERE email = 'rc-member@example.com'`); err != nil {
		t.Fatal(err)
	}
	var me authJSON
	member.json(http.MethodGet, "/api/auth/me", nil, http.StatusOK, &me)
	if me.User.Role != "admin" {
		t.Fatalf("role %q", me.User.Role)
	}
	if rec := member.do(http.MethodPost, "/api/admin/solutions", map[string]any{}, false); rec.Code == http.StatusForbidden {
		t.Fatalf("admin still forbidden: %s", rec.Body.String())
	}

	p, _ := warehouseProject(t, owner, "Роли")
	addMember(t, owner, p.ID, "rc-member@example.com", RoleEditor)
	var list struct {
		Items []struct {
			UserID string `json:"user_id"`
			Email  string `json:"email"`
		} `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/members", nil, http.StatusOK, &list)
	var memberID string
	for _, m := range list.Items {
		if m.Email == "rc-member@example.com" {
			memberID = m.UserID
		}
	}
	if memberID == "" {
		t.Fatalf("member not listed: %+v", list)
	}
	postTxs(t, member, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "name", "Роли 2")))
	owner.json(http.MethodPut, "/api/projects/"+p.ID+"/members/"+memberID, map[string]any{"role": RoleViewer}, http.StatusNoContent, nil)
	rec := member.do(http.MethodPost, "/api/projects/"+p.ID+"/transactions", map[string]any{
		"client_id": "role-change", "schema_version": 1,
		"txs": []testTx{newTx(opSet(ops.CollProject, ops.CollProject, "name", "Роли 3"))},
	}, false)
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
		t.Fatalf("viewer can still edit: %d %s", rec.Code, rec.Body.String())
	}
}
