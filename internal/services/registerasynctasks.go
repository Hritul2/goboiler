package services

import (
	"fmt"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/lib/email"
	"github.com/hritul2/goboiler/internal/services/asyncprocessor"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// NewJobRouter builds every dependency a task handler needs and registers
// each domain's handlers against a fresh asyncprocessor.Router. This is the
// single place "which job kinds exist" is decided — adding a new one means
// adding its dependency setup and one router.Handle call here, nothing in
// cmd/main.go or internal/services/asyncprocessor itself.
func NewJobRouter(cfg *config.Config, log zerolog.Logger) (*asyncprocessor.Router, error) {
	sender, err := email.New(&cfg.EmailConfig, log)
	if err != nil {
		return nil, fmt.Errorf("job router: email sender: %w", err)
	}

	router := asyncprocessor.NewRouter()
	router.Handle(email.TaskWelcomeEmail, email.NewWelcomeEmailHandler(sender, log))
	// router.Handle(<domain>.TaskX, <domain>.NewXHandler(...))

	return router, nil
}
