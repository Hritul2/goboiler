package asyncprocessor

import (
	"context"

	"github.com/hibiken/asynq"
)

// HandlerFunc processes one task type. Named to parallel http.HandlerFunc.
type HandlerFunc func(ctx context.Context, task *asynq.Task) error

// Router maps task types to handlers. asyncprocessor has no idea what any
// task does — every domain (email, and whatever comes later) registers its
// own handlers here; this is the single place dispatch is decided.
type Router struct {
	mux *asynq.ServeMux
}

// NewRouter returns an empty Router.
func NewRouter() *Router {
	return &Router{mux: asynq.NewServeMux()}
}

// Handle registers h as the processor for taskType.
func (r *Router) Handle(taskType string, h HandlerFunc) {
	r.mux.HandleFunc(taskType, h)
}
