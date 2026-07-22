package email

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// fakeSender captures the last Email for assertion in emails_test.go.
type fakeSender struct {
	last Email
}

func (f *fakeSender) Send(_ context.Context, e Email) error {
	f.last = e
	return nil
}

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
