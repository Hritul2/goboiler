package asyncprocessor

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

// tracer/meter are the package-level instrumentation handles, following
// internal/observability's documented convention (one per package, keyed by
// import path).
var (
	tracer = otel.Tracer("github.com/hritul2/goboiler/internal/services/asyncprocessor")
	meter  = otel.Meter("github.com/hritul2/goboiler/internal/services/asyncprocessor")

	taskProcessedCounter, _ = meter.Int64Counter(
		"task.processed",
		metric.WithDescription("Total async tasks processed, by task_type and status"),
	)
	taskDurationHistogram, _ = meter.Float64Histogram(
		"task.duration_ms",
		metric.WithDescription("Async task processing duration"),
		metric.WithUnit("ms"),
	)
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

// Handle registers h as the processor for taskType, wrapped in a span and
// metrics both named/tagged after the task type. This is the one generic
// place that sees every task type, so it's the right layer for tracing and
// metrics — domain-specific log fields (e.g. email's `to`) stay at the
// domain level; only generic timing/pass-fail is recorded here.
func (r *Router) Handle(taskType string, h HandlerFunc) {
	r.mux.HandleFunc(taskType, instrumentedHandler(taskType, h))
}

func instrumentedHandler(taskType string, h HandlerFunc) HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		ctx, span := tracer.Start(ctx, "task."+taskType)
		defer span.End()

		start := time.Now()
		err := h(ctx, task)
		duration := float64(time.Since(start).Milliseconds())

		status := "ok"
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			status = "error"
		}

		attrs := metric.WithAttributes(
			attribute.String("task_type", taskType),
			attribute.String("status", status),
		)
		taskProcessedCounter.Add(ctx, 1, attrs)
		taskDurationHistogram.Record(ctx, duration, metric.WithAttributes(attribute.String("task_type", taskType)))

		return err
	}
}
