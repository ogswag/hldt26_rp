// Package audit records who did what to accounts, members, projects and the robot catalog. Project content changes
// are not recorded here: they live in project_operations.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/db"
)

const (
	ActionLogout        = "auth.logout"
	ActionSessionRevoke = "auth.session_revoke"
	ActionPasswordReset = "auth.password_reset"
	ActionEmailVerified = "auth.email_verified"
	ActionInviteCreate  = "invite.create"
	ActionInviteRevoke  = "invite.revoke"
	ActionInviteAccept  = "invite.accept"
	ActionMemberAdd     = "member.add"
	ActionMemberRole    = "member.role"
	ActionMemberRemove  = "member.remove"
	ActionProjectDelete = "project.delete"
	ActionProjectRestor = "project.restore"
	ActionProjectPurge  = "project.purge"

	ActionSolutionCreate    = "solution.create"
	ActionSolutionUpdate    = "solution.update"
	ActionSolutionArchive   = "solution.archive"
	ActionSolutionRestore   = "solution.restore"
	ActionSolutionDuplicate = "solution.duplicate"
	ActionSolutionImage     = "solution.image"
	ActionCatalogImport     = "catalog.import"

	TargetUser    = "user"
	TargetProject = "project"
	TargetInvite  = "invitation"
	TargetSession = "session"

	TargetSolution      = "solution"
	TargetCatalogImport = "catalog_import"
)

// Event is one recorded action. Meta carries identifiers and catalog field changes only: no tokens, no numbers
// from projects.
type Event struct {
	Actor      *uuid.UUID
	Action     string
	TargetType string
	TargetID   string
	RequestID  string
	Meta       map[string]any
}

// Mask hides the local part of an email, so the log says which account without repeating the address.
func Mask(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at < 1 {
		return "***"
	}
	local := email[:at]
	if len(local) <= 2 {
		return "*" + email[at:]
	}
	return local[:1] + "***" + email[at:]
}

// Record writes the event with the querier of the action's own transaction.
func Record(ctx context.Context, q *db.Queries, e Event) error {
	meta := json.RawMessage(`{}`)
	if len(e.Meta) > 0 {
		b, err := json.Marshal(e.Meta)
		if err != nil {
			return fmt.Errorf("audit.meta: %w", err)
		}
		meta = b
	}
	var target, request *string
	if e.TargetID != "" {
		target = &e.TargetID
	}
	if e.RequestID != "" {
		request = &e.RequestID
	}
	if err := q.RecordAuditEvent(ctx, db.RecordAuditEventParams{
		ActorUserID: e.Actor,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    target,
		RequestID:   request,
		Meta:        meta,
	}); err != nil {
		return fmt.Errorf("audit.record: %w", err)
	}
	return nil
}
