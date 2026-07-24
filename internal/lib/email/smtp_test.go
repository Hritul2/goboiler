package email

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	th "github.com/hritul2/goboiler/internal/testing"
	"github.com/hritul2/goboiler/internal/utils/config"
)

func TestBuildMsg(t *testing.T) {
	msg, err := buildMsg("noreply@example.com", "Example App", Email{
		To:      []string{"ada@example.com"},
		Subject: "Hello Ada",
		HTML:    "<p>Hi there</p>",
		Text:    "Hi there",
	})
	require.NoError(t, err)

	// *mail.Msg implements io.WriterTo — assert on the rendered wire format,
	// robust regardless of which getters go-mail exposes.
	var buf bytes.Buffer
	_, err = msg.WriteTo(&buf)
	require.NoError(t, err)

	out := buf.String()
	th.AssertStringContains(t, out,
		"Subject: Hello Ada",
		"ada@example.com",
		"noreply@example.com",
		"Example App",
		"<p>Hi there</p>",
	)
}

func TestBuildMsg_InvalidRecipient(t *testing.T) {
	_, err := buildMsg("noreply@example.com", "", Email{
		To:      []string{"not-an-address"},
		Subject: "x",
		HTML:    "<p>x</p>",
	})
	require.Error(t, err)
}

func TestNewSMTP_EmptyHost(t *testing.T) {
	_, err := newSMTP(&config.EmailConfig{Enabled: true, Host: "", Port: 587, From: "a@b.c"}, zerolog.Nop())
	require.Error(t, err)
}
