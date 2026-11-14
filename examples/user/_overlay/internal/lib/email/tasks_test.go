package email

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestWelcomeEmailTaskOptions(t *testing.T) {
	opts := welcomeEmailTaskOptions()

	got := make(map[asynq.OptionType]any, len(opts))
	for _, o := range opts {
		got[o.Type()] = o.Value()
	}

	require.Contains(t, got, asynq.MaxRetryOpt)
	assert.Equal(t, 3, got[asynq.MaxRetryOpt])

	require.Contains(t, got, asynq.QueueOpt)
	assert.Equal(t, "default", got[asynq.QueueOpt])

	require.Contains(t, got, asynq.TimeoutOpt)
	assert.Equal(t, 30*time.Second, got[asynq.TimeoutOpt])
}

func TestNewWelcomeEmailTask(t *testing.T) {
	task, err := NewWelcomeEmailTask("ada@example.com", "Ada")
	require.NoError(t, err)
	assert.Equal(t, TaskWelcomeEmail, task.Type())

	var p WelcomeEmailPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &p))
	assert.Equal(t, "ada@example.com", p.To)
	assert.Equal(t, "Ada", p.Name)
}

func TestNewWelcomeEmailHandler(t *testing.T) {
	sender := &fakeSender{}
	handler := NewWelcomeEmailHandler(sender, zerolog.Nop())

	task, err := NewWelcomeEmailTask("ada@example.com", "Ada")
	require.NoError(t, err)

	err = handler(context.Background(), task)
	require.NoError(t, err)

	require.True(t, sender.sent)
	assert.Equal(t, []string{"ada@example.com"}, sender.last.To)
	assert.Contains(t, sender.last.HTML, "Ada")
}

func TestNewWelcomeEmailHandler_BadPayload(t *testing.T) {
	sender := &fakeSender{}
	handler := NewWelcomeEmailHandler(sender, zerolog.Nop())

	task := asynq.NewTask(TaskWelcomeEmail, []byte("not json"))

	err := handler(context.Background(), task)
	require.Error(t, err)
	assert.False(t, sender.sent, "sender must not be called on a bad payload")
}

func collectMetricNames(t *testing.T, reader *sdkmetric.ManualReader) map[string]bool {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	names := make(map[string]bool)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names[m.Name] = true
		}
	}
	return names
}

// TestNewWelcomeEmailHandler_RecordsMetrics exercises both the success and
// failure paths against a SINGLE registered MeterProvider/reader. The OTel
// Go SDK's global package-level instruments (created once at package init)
// bind to the first real MeterProvider ever set via otel.SetMeterProvider;
// a later, separate SetMeterProvider call in a different test does not
// rebind them — so this must be one test, not two, to reliably observe both
// counters.
func TestNewWelcomeEmailHandler_RecordsMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	sender := &fakeSender{}
	handler := NewWelcomeEmailHandler(sender, zerolog.Nop())

	task, err := NewWelcomeEmailTask("ada@example.com", "Ada")
	require.NoError(t, err)
	require.NoError(t, handler(context.Background(), task))

	badTask := asynq.NewTask(TaskWelcomeEmail, []byte("not json"))
	require.Error(t, handler(context.Background(), badTask))

	names := collectMetricNames(t, reader)
	assert.True(t, names["email.sent"], "email.sent counter must be recorded")
	assert.True(t, names["email.failed"], "email.failed counter must be recorded")
}
