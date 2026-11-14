package email

import "context"

// Typed send functions — one per email the application sends. Services call
// these rather than constructing Email values themselves.

// SendWelcome sends the post-signup welcome email.
func SendWelcome(ctx context.Context, s Sender, to, name string) error {
	html, err := renderTemplate("welcome.html", welcomeData{Name: name})
	if err != nil {
		return err
	}

	return s.Send(ctx, Email{
		To:      []string{to},
		Subject: "Welcome aboard, " + name + "!",
		HTML:    html,
	})
}
