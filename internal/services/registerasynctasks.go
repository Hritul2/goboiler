package services

import (
	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/services/asyncprocessor"
	"github.com/hritul2/goboiler/internal/utils/config"
)

// NewJobRouter builds every dependency a task handler needs and registers
// each domain's handlers against a fresh asyncprocessor.Router. This is the
// single place "which job kinds exist" is decided — adding a new one means
// adding its dependency setup and one router.Handle call here, nothing in
// cmd/main.go or internal/services/asyncprocessor itself.
//
// The skeleton registers no jobs: the worker still starts, and a task of an
// unregistered type simply fails with "handler not found" (asynq retries or
// archives it) — nothing enqueues one until you add a producer.
//
// ADD YOUR DOMAIN: build the dependencies your handlers need (for email:
// `sender, err := email.New(&cfg.Email, log)`) and register each consumer
// (`router.Handle(email.TaskWelcomeEmail, email.NewWelcomeEmailHandler(sender, log))`)
// — see docs/EXAMPLE.md step 5.
func NewJobRouter(cfg *config.Config, log zerolog.Logger) (*asyncprocessor.Router, error) {
	_, _ = cfg, log // used once you build handler dependencies

	router := asyncprocessor.NewRouter()
	// router.Handle(<domain>.TaskX, <domain>.NewXHandler(...))

	return router, nil
}
