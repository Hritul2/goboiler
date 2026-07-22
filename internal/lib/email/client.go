// Package email provides outbound email over standard SMTP (Zoho, Gmail app
// password, AWS SES SMTP — any provider). It is a plain constructor, not a
// singleton: go-mail dials per send, so there is no connection state worth
// boot-time management. Services receive a Sender via dependency injection.
package email

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// Email is a single outbound message.
type Email struct {
	To      []string
	Subject string
	HTML    string
	Text    string // optional plain-text alternative
}

// Sender delivers emails. Implementations: smtpSender (real), noopSender
// (Enabled=false — logs and drops).
type Sender interface {
	Send(ctx context.Context, e Email) error
}

// New returns the configured Sender. With Enabled=false it returns a logging
// no-op so local dev boots without SMTP credentials and "sends" stay visible
// in logs.
func New(cfg *config.EmailConfig, log zerolog.Logger) (Sender, error) {
	if !cfg.Enabled {
		return &noopSender{log: log}, nil
	}
	return newSMTP(cfg, log)
}

type noopSender struct {
	log zerolog.Logger
}

func (n *noopSender) Send(_ context.Context, e Email) error {
	n.log.Info().
		Strs("to", e.To).
		Str("subject", e.Subject).
		Msg("email sending disabled; skipping send")
	return nil
}
