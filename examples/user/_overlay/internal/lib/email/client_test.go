package email

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/utils/config"
)

func TestNew_DisabledReturnsNoop(t *testing.T) {
	s, err := New(&config.EmailConfig{Enabled: false}, zerolog.Nop())
	require.NoError(t, err)
	require.NotNil(t, s)

	// A no-op send must succeed without any SMTP configuration.
	err = s.Send(context.Background(), Email{To: []string{"a@b.c"}, Subject: "hi", HTML: "<p>hi</p>"})
	require.NoError(t, err)
}

func TestNew_EnabledInvalidHost(t *testing.T) {
	_, err := New(&config.EmailConfig{Enabled: true, Host: "", Port: 587, From: "a@b.c"}, zerolog.Nop())
	require.Error(t, err)
}

// example:region fake-sender

// fakeSender captures the last Email for assertion in emails_test.go and
// tasks_test.go.
type fakeSender struct {
	last Email
	sent bool
}

func (f *fakeSender) Send(_ context.Context, e Email) error {
	f.last = e
	f.sent = true
	return nil
}

// example:endregion fake-sender
