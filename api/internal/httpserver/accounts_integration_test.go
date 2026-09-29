package httpserver

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"moscow_hackathon_2026/api/internal/config"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/mail"
)

type outboxResp struct {
	Items []outboxItem `json:"items"`
}

// letter reads the newest letter of one template from the local outbox and returns the link it carries.
func letter(t *testing.T, c *itClient, to, template string) string {
	t.Helper()
	var out outboxResp
	c.json(http.MethodGet, "/api/dev/outbox?to="+to, nil, http.StatusOK, &out)
	for _, item := range out.Items {
		if item.Template == template {
			link := item.Payload["link"]
			_, token, ok := strings.Cut(link, "#token=")
			if !ok {
				t.Fatalf("letter %s to %s has no token: %q", template, to, link)
			}
			return token
		}
	}
	t.Fatalf("no %s letter for %s: %+v", template, to, out.Items)
	return ""
}

func TestEmailVerificationAndPasswordReset(t *testing.T) {
	env := newEnv(t)
	c := register(t, env.h, "verify@example.com")

	token := letter(t, c, "verify@example.com", mail.TemplateVerifyEmail)
	c.json(http.MethodPost, "/api/auth/verify-email", map[string]string{"token": token}, http.StatusOK, nil)
	c.json(http.MethodPost, "/api/auth/verify-email", map[string]string{"token": token}, http.StatusGone, nil)

	anon := &itClient{t: t, h: env.h}
	anon.json(http.MethodPost, "/api/auth/password-reset/request", map[string]string{"email": "nobody@example.com"}, http.StatusAccepted, nil)
	var out outboxResp
	anon.json(http.MethodGet, "/api/dev/outbox?to=nobody@example.com", nil, http.StatusOK, &out)
	if len(out.Items) != 0 {
		t.Fatalf("an unknown address must not get a letter: %+v", out.Items)
	}

	anon.json(http.MethodPost, "/api/auth/password-reset/request", map[string]string{"email": "verify@example.com"}, http.StatusAccepted, nil)
	reset := letter(t, c, "verify@example.com", mail.TemplateResetPassord)
	anon.json(http.MethodPost, "/api/auth/password-reset/confirm", map[string]string{"token": reset, "password": "short"}, http.StatusBadRequest, nil)
	anon.json(http.MethodPost, "/api/auth/password-reset/confirm", map[string]string{"token": reset, "password": "new-password-2"}, http.StatusOK, nil)
	// The old session ended with the password change.
	c.json(http.MethodGet, "/api/auth/me", nil, http.StatusUnauthorized, nil)
	anon.json(http.MethodPost, "/api/auth/password-reset/confirm", map[string]string{"token": reset, "password": "new-password-3"}, http.StatusGone, nil)

	fresh := &itClient{t: t, h: env.h}
	var auth authJSON
	fresh.json(http.MethodPost, "/api/auth/login", map[string]string{"email": "verify@example.com", "password": "new-password-2"}, http.StatusOK, &auth)
	if auth.User.Email != "verify@example.com" {
		t.Fatalf("login with the new password: %+v", auth)
	}
}

func TestInvitationsGiveProjectAccess(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "inviter@example.com")
	p, _ := warehouseProject(t, owner, "Проект с приглашением")

	var invited struct {
		Status     string         `json:"status"`
		Invitation invitationJSON `json:"invitation"`
	}
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/members",
		map[string]any{"email": "newcomer@example.com", "role": RoleEditor}, http.StatusAccepted, &invited)
	if invited.Invitation.ProjectRole == nil || *invited.Invitation.ProjectRole != RoleEditor {
		t.Fatalf("invitation: %+v", invited)
	}

	token := letter(t, owner, "newcomer@example.com", mail.TemplateInvite)
	anon := &itClient{t: t, h: env.h}
	var inspect inspectJSON
	anon.json(http.MethodPost, "/api/auth/invitations/inspect", map[string]string{"token": token}, http.StatusOK, &inspect)
	if !inspect.Valid || inspect.Email != "newcomer@example.com" || inspect.ProjectName == nil {
		t.Fatalf("inspect: %+v", inspect)
	}

	newcomer := &itClient{t: t, h: env.h}
	var auth authJSON
	newcomer.json(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "newcomer@example.com", "password": "secret-password-1", "invite_token": token}, http.StatusOK, &auth)
	newcomer.csrf = auth.CSRFToken
	var listed struct {
		Items []struct {
			ID     string `json:"id"`
			Access string `json:"access"`
		} `json:"items"`
	}
	newcomer.json(http.MethodGet, "/api/projects", nil, http.StatusOK, &listed)
	if len(listed.Items) != 1 || listed.Items[0].ID != p.ID || listed.Items[0].Access != RoleEditor {
		t.Fatalf("an accepted invitation must make the person an editor: %+v", listed.Items)
	}
	// The invitation is spent; the same token opens nothing more.
	anon.json(http.MethodPost, "/api/auth/invitations/inspect", map[string]string{"token": token}, http.StatusOK, &inspect)
	if inspect.Valid {
		t.Fatalf("a used invitation must not be valid: %+v", inspect)
	}
}

func TestRegistrationByInviteOnly(t *testing.T) {
	env := newEnv(t)
	admin := register(t, env.h, "admin@example.com")
	if _, err := env.db.Exec(t.Context(), "UPDATE users SET role = 'admin' WHERE email = $1", "admin@example.com"); err != nil {
		t.Fatal(err)
	}
	env.srv.cfg.RegistrationMode = config.RegistrationInvite

	stranger := &itClient{t: t, h: env.h}
	var refused errorBody
	stranger.json(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "stranger@example.com", "password": "secret-password-1"}, http.StatusForbidden, &refused)
	if refused.Code != "invite_required" {
		t.Fatalf("closed registration: %+v", refused)
	}

	var created invitationJSON
	admin.json(http.MethodPost, "/api/admin/invitations", map[string]any{"email": "guest@example.com"}, http.StatusCreated, &created)
	token := letter(t, admin, "guest@example.com", mail.TemplateInvite)
	guest := &itClient{t: t, h: env.h}
	guest.json(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "guest@example.com", "password": "secret-password-1", "invite_token": token}, http.StatusOK, nil)

	var second invitationJSON
	admin.json(http.MethodPost, "/api/admin/invitations", map[string]any{"email": "later@example.com"}, http.StatusCreated, &second)
	admin.json(http.MethodDelete, "/api/admin/invitations/"+second.ID, nil, http.StatusNoContent, nil)
	admin.json(http.MethodDelete, "/api/admin/invitations/"+second.ID, nil, http.StatusNotFound, nil)

	var list struct {
		Items []invitationJSON `json:"items"`
	}
	admin.json(http.MethodGet, "/api/admin/invitations", nil, http.StatusOK, &list)
	if len(list.Items) != 2 {
		t.Fatalf("invitations: %+v", list.Items)
	}
}

func TestTrashKeepsProjectForThirtyDays(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "trash@example.com")
	stranger := register(t, env.h, "stranger@example.com")
	p, _ := warehouseProject(t, owner, "Проект в корзину")

	var deleted deletedJSON
	owner.json(http.MethodDelete, "/api/projects/"+p.ID, nil, http.StatusOK, &deleted)
	if deleted.DeletedAt == nil || deleted.PurgeAt == nil || !deleted.PurgeAt.After(*deleted.DeletedAt) {
		t.Fatalf("delete answer: %+v", deleted)
	}

	var gone errorBody
	owner.json(http.MethodGet, "/api/projects/"+p.ID, nil, http.StatusGone, &gone)
	if gone.Code != "project_deleted" {
		t.Fatalf("a trashed project answers: %+v", gone)
	}
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/transactions",
		map[string]any{"tx_id": "00000000-0000-4000-8000-000000000001", "ops": []any{}}, http.StatusGone, nil)

	var trash struct {
		Items []trashItem `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/trash", nil, http.StatusOK, &trash)
	if len(trash.Items) != 1 || trash.Items[0].ID != p.ID {
		t.Fatalf("trash: %+v", trash.Items)
	}
	var otherTrash struct {
		Items []trashItem `json:"items"`
	}
	stranger.json(http.MethodGet, "/api/projects/trash", nil, http.StatusOK, &otherTrash)
	if len(otherTrash.Items) != 0 {
		t.Fatalf("someone else's trash must stay hidden: %+v", otherTrash.Items)
	}
	stranger.json(http.MethodPost, "/api/projects/"+p.ID+"/restore", nil, http.StatusNotFound, nil)

	var back projectResp
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/restore", nil, http.StatusOK, &back)
	if back.ID != p.ID {
		t.Fatalf("restored project: %+v", back)
	}
	owner.json(http.MethodGet, "/api/projects/"+p.ID, nil, http.StatusOK, nil)

	owner.json(http.MethodDelete, "/api/projects/"+p.ID, nil, http.StatusOK, nil)
	old := time.Now().Add(-40 * 24 * time.Hour)
	if _, err := env.db.Exec(t.Context(), "UPDATE projects SET deleted_at = $2 WHERE id = $1", p.ID, old); err != nil {
		t.Fatal(err)
	}
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/restore", nil, http.StatusGone, nil)

	n, err := PurgeTrash(t.Context(), db.New(env.db), time.Now().Add(-30*24*time.Hour), 100)
	if err != nil || n != 1 {
		t.Fatalf("purge: %d, %v", n, err)
	}
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/restore", nil, http.StatusNotFound, nil)
}

func TestAuditRecordsAccountActions(t *testing.T) {
	env := newEnv(t)
	admin := register(t, env.h, "auditor@example.com")
	if _, err := env.db.Exec(t.Context(), "UPDATE users SET role = 'admin' WHERE email = $1", "auditor@example.com"); err != nil {
		t.Fatal(err)
	}
	p, _ := warehouseProject(t, admin, "Проект с журналом")
	colleague := register(t, env.h, "colleague@example.com")
	colleague.json(http.MethodPost, "/api/auth/logout", nil, http.StatusNoContent, nil)

	var member memberJSON
	admin.json(http.MethodPost, "/api/projects/"+p.ID+"/members",
		map[string]any{"email": "colleague@example.com", "role": RoleViewer}, http.StatusCreated, &member)
	admin.json(http.MethodPut, "/api/projects/"+p.ID+"/members/"+member.UserID, map[string]any{"role": RoleEditor}, http.StatusNoContent, nil)
	admin.json(http.MethodDelete, "/api/projects/"+p.ID+"/members/"+member.UserID, nil, http.StatusNoContent, nil)
	admin.json(http.MethodDelete, "/api/projects/"+p.ID, nil, http.StatusOK, nil)

	var log struct {
		Items []auditItem `json:"items"`
	}
	admin.json(http.MethodGet, "/api/admin/audit?target_type=project&target_id="+p.ID, nil, http.StatusOK, &log)
	got := map[string]bool{}
	for _, item := range log.Items {
		got[item.Action] = true
	}
	for _, want := range []string{"member.add", "member.role", "member.remove", "project.delete"} {
		if !got[want] {
			t.Errorf("audit has no %s: %+v", want, log.Items)
		}
	}
	if n := log.Items[0].TargetName; n == nil || *n != p.Name {
		t.Errorf("audit names the project %v, want %q", n, p.Name)
	}
	colleague.json(http.MethodGet, "/api/admin/audit", nil, http.StatusForbidden, nil)
}
