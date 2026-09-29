// Package mail renders the account letters and sends them, over SMTP in a deployment and into memory locally.
package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

const (
	TemplateVerifyEmail  = "verify_email"
	TemplateResetPassord = "reset_password"
	TemplateInvite       = "invite"
)

// Message is one letter, plain text only.
type Message struct {
	To      string
	Subject string
	Body    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
	// KeepsLink reports whether sent letters stay readable locally, so the link survives in the outbox row.
	KeepsLink() bool
}

// Render builds the letter for a template and its payload.
func Render(template string, payload map[string]string) (Message, error) {
	link := payload["link"]
	switch template {
	case TemplateVerifyEmail:
		return Message{
			Subject: "Подтвердите email",
			Body: "Здравствуйте.\n\nЧтобы подтвердить адрес, откройте ссылку:\n" + link +
				"\n\nСсылка действует 48 часов. Если вы не создавали аккаунт, письмо можно не читать.\n",
		}, nil
	case TemplateResetPassord:
		return Message{
			Subject: "Восстановление пароля",
			Body: "Здравствуйте.\n\nЧтобы задать новый пароль, откройте ссылку:\n" + link +
				"\n\nСсылка действует 1 час и работает один раз. Если вы не просили новый пароль, ничего делать не нужно.\n",
		}, nil
	case TemplateInvite:
		who := payload["project"]
		what := "Вас пригласили в приложение."
		if who != "" {
			what = "Вас пригласили в проект «" + who + "»."
		}
		return Message{
			Subject: "Приглашение",
			Body: "Здравствуйте.\n\n" + what + "\nЧтобы принять приглашение, откройте ссылку:\n" + link +
				"\n\nСсылка действует 7 дней.\n",
		}, nil
	}
	return Message{}, fmt.Errorf("mail.render: unknown template %q", template)
}

// Capture keeps letters in memory. Local runs and tests read them instead of an inbox.
type Capture struct {
	mu   sync.Mutex
	sent []Message
}

func (c *Capture) Send(_ context.Context, m Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, m)
	return nil
}

func (c *Capture) KeepsLink() bool { return true }

// Sent returns the letters for one address, newest last.
func (c *Capture) Sent(to string) []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []Message
	for _, m := range c.sent {
		if strings.EqualFold(m.To, to) {
			out = append(out, m)
		}
	}
	return out
}

// SMTP sends over STARTTLS on the submission port.
type SMTP struct {
	Host     string
	Port     int
	From     string
	User     string
	Password string
	Timeout  time.Duration
}

func (s SMTP) KeepsLink() bool { return false }

func (s SMTP) Send(ctx context.Context, m Message) error {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	addr := net.JoinHostPort(s.Host, fmt.Sprint(s.Port))
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("mail.dial: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("mail.deadline: %w", err)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("mail.client: %w", err)
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return fmt.Errorf("mail.starttls: server does not support STARTTLS")
	}
	if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("mail.starttls: %w", err)
	}
	if s.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.User, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail.auth: %w", err)
		}
	}
	if err := c.Mail(s.From); err != nil {
		return fmt.Errorf("mail.from: %w", err)
	}
	if err := c.Rcpt(m.To); err != nil {
		return fmt.Errorf("mail.rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail.data: %w", err)
	}
	if _, err := w.Write([]byte(wire(s.From, m))); err != nil {
		return fmt.Errorf("mail.write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail.close: %w", err)
	}
	return c.Quit()
}

func wire(from string, m Message) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + m.To + "\r\n")
	b.WriteString("Subject: " + encodeHeader(m.Subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(m.Body, "\n", "\r\n"))
	return b.String()
}

func encodeHeader(s string) string {
	return mime.BEncoding.Encode("utf-8", s)
}
