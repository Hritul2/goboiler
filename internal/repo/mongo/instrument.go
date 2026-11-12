package mongo

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// meter is the package-level instrumentation handle, following
// internal/observability's documented convention (one per package, keyed by
// import path). The instrument names (repo.operation,
// repo.operation.duration_ms) are driver-neutral so a future postgres/sqlite
// repo can emit the same series.
var (
	meter = otel.Meter("github.com/hritul2/goboiler/internal/repo/mongo")

	repoOperationCounter, _ = meter.Int64Counter(
		"repo.operation",
		metric.WithDescription("Total repository operations, by operation and status"),
	)
	repoOperationDuration, _ = meter.Float64Histogram(
		"repo.operation.duration_ms",
		metric.WithDescription("Repository operation duration"),
		metric.WithUnit("ms"),
	)
)

// instrumentOperation records repo.operation/repo.operation.duration_ms
// around fn and forwards fn's return value unchanged. This is the one
// shared place every CRUD method routes through for metrics — kept
// independent of *mongo.Collection so it's unit-testable without a live
// Mongo connection.
func instrumentOperation(ctx context.Context, operation string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := float64(time.Since(start).Milliseconds())

	status := "ok"
	if err != nil {
		status = "error"
	}

	repoOperationCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("status", status),
	))
	repoOperationDuration.Record(ctx, duration, metric.WithAttributes(
		attribute.String("operation", operation),
	))

	return err
}
