package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/db"
)

const (
	// MaxAttempts is how often one letter is tried before the row is marked failed.
	MaxAttempts = 5
	batch       = 20
)

// Outbox takes queued letters from the database and hands them to the sender.
type Outbox struct {
	q      *db.Queries
	sender Sender
	log    *slog.Logger
}

func NewOutbox(q *db.Queries, sender Sender, log *slog.Logger) *Outbox {
	return &Outbox{q: q, sender: sender, log: log}
}

// Send delivers the letters that are due. One row is claimed by one process, so several API instances share it.
func (o *Outbox) Send(ctx context.Context) error {
	rows, err := o.q.ClaimEmailOutbox(ctx, batch)
	if err != nil {
		return fmt.Errorf("mail.claim: %w", err)
	}
	for _, row := range rows {
		var payload map[string]string
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			o.fail(ctx, row.ID, fmt.Sprintf("payload: %v", err))
			continue
		}
		m, err := Render(row.Template, payload)
		if err != nil {
			o.fail(ctx, row.ID, err.Error())
			continue
		}
		m.To = row.ToEmail
		if err := o.sender.Send(ctx, m); err != nil {
			o.log.Warn("mail.send", "op", "mail.send", "email_id", row.ID.String(), "attempt", row.Attempts, "err", err)
			o.fail(ctx, row.ID, err.Error())
			continue
		}
		if err := o.q.MarkEmailSent(ctx, db.MarkEmailSentParams{ID: row.ID, KeepLink: o.sender.KeepsLink()}); err != nil {
			return fmt.Errorf("mail.sent: %w", err)
		}
	}
	return nil
}

func (o *Outbox) fail(ctx context.Context, id uuid.UUID, text string) {
	if err := o.q.MarkEmailFailed(ctx, db.MarkEmailFailedParams{
		ID:          id,
		MaxAttempts: MaxAttempts,
		ErrorText:   &text,
	}); err != nil {
		o.log.Error("mail.fail", "op", "mail.send", "email_id", id.String(), "err", err)
	}
}
