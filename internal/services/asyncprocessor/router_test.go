package asyncprocessor

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRouter_DispatchesToRegisteredHandler(t *testing.T) {
	r := NewRouter()

	var gotTask *asynq.Task
	r.Handle("test:ping", func(_ context.Context, task *asynq.Task) error {
		gotTask = task
		return nil
	})

	task := asynq.NewTask("test:ping", []byte(`{"ok":true}`))
	err := r.mux.ProcessTask(context.Background(), task)

	require.NoError(t, err)
	require.NotNil(t, gotTask)
	assert.Equal(t, "test:ping", gotTask.Type())
}

func TestRouter_UnregisteredType(t *testing.T) {
	r := NewRouter()

	task := asynq.NewTask("test:unregistered", nil)
	err := r.mux.ProcessTask(context.Background(), task)

	require.Error(t, err)
}

func TestRouter_Handle_RecordsSpanOnError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	r := NewRouter()
	wantErr := errors.New("boom")
	r.Handle("test:ping", func(_ context.Context, _ *asynq.Task) error {
		return wantErr
	})

	task := asynq.NewTask("test:ping", nil)
	err := r.mux.ProcessTask(context.Background(), task)
	require.ErrorIs(t, err, wantErr, "wrapping must forward the handler's return value unchanged")

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "task.test:ping", spans[0].Name)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
}

func TestRouter_Handle_RecordsTaskMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	r := NewRouter()
	r.Handle("test:ping", func(_ context.Context, _ *asynq.Task) error {
		return nil
	})

	task := asynq.NewTask("test:ping", nil)
	require.NoError(t, r.mux.ProcessTask(context.Background(), task))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var foundCounter, foundHistogram bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "task.processed":
				foundCounter = true
			case "task.duration_ms":
				foundHistogram = true
			}
		}
	}
	assert.True(t, foundCounter, "task.processed counter must be recorded")
	assert.True(t, foundHistogram, "task.duration_ms histogram must be recorded")
}

// A project with no jobs yet registers nothing. Init hands router.mux to
// asynq.Server.Start, whose only precondition is a non-nil handler (checked
// against the asynq source), so an empty Router must still carry a usable
// mux, and a task of any type must fail cleanly rather than panic.
func TestRouter_EmptyRouterIsUsableByStart(t *testing.T) {
	r := NewRouter()

	var h asynq.Handler = r.mux
	require.NotNil(t, h)

	err := r.mux.ProcessTask(context.Background(), asynq.NewTask("anything", nil))
	require.Error(t, err)
}
