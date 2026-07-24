package email

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	mail "github.com/wneessen/go-mail"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// smtpSender delivers email over standard SMTP via go-mail.
type smtpSender struct {
	client   *mail.Client
	from     string
	fromName string
	log      zerolog.Logger
}

func newSMTP(cfg *config.EmailConfig, log zerolog.Logger) (*smtpSender, error) {
	// TLSOpportunistic upgrades to STARTTLS whenever the server offers it —
	// effectively mandatory on real providers' port 587 — while still
	// allowing TLS-less local catchers like Mailpit on localhost:1025.
	opts := []mail.Option{
		mail.WithPort(cfg.Port),
		mail.WithTLSPortPolicy(mail.TLSOpportunistic),
	}
	if cfg.Username != "" {
		opts = append(opts,
			// AutoDiscover negotiates the strongest mechanism the server
			// advertises (falls back to PLAIN/LOGIN) — covers Zoho, Gmail,
			// and SES without provider-specific configuration.
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
			mail.WithUsername(cfg.Username),
			mail.WithPassword(cfg.Password),
		)
	}

	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("email: smtp client: %w", err)
	}

	return &smtpSender{
		client:   client,
		from:     cfg.From,
		fromName: cfg.FromName,
		log:      log,
	}, nil
}

// buildMsg constructs the wire message (pure, unit-tested).
func buildMsg(from, fromName string, e Email) (*mail.Msg, error) {
	m := mail.NewMsg()

	if err := m.FromFormat(fromName, from); err != nil {
		return nil, fmt.Errorf("email: from address: %w", err)
	}
	if err := m.To(e.To...); err != nil {
		return nil, fmt.Errorf("email: to address: %w", err)
	}

	m.Subject(e.Subject)
	m.SetBodyString(mail.TypeTextHTML, e.HTML)
	if e.Text != "" {
		m.AddAlternativeString(mail.TypeTextPlain, e.Text)
	}
	return m, nil
}

func (s *smtpSender) Send(ctx context.Context, e Email) error {
	msg, err := buildMsg(s.from, s.fromName, e)
	if err != nil {
		s.log.Error().Err(err).Strs("to", e.To).Msg("failed to build email message")
		return err
	}

	// Error classification is the service layer's job; keep the raw cause.
	if err := s.client.DialAndSendWithContext(ctx, msg); err != nil {
		wrapped := fmt.Errorf("email: send: %w", err)
		s.log.Error().Err(wrapped).Strs("to", e.To).Str("subject", e.Subject).Msg("failed to send email")
		return wrapped
	}

	s.log.Info().Strs("to", e.To).Str("subject", e.Subject).Msg("email sent")
	return nil
}
