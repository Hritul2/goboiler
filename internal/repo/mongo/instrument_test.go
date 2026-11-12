package mongo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestInstrumentOperation_RecordsMetrics covers both the success and failure
// paths against a SINGLE registered MeterProvider/reader, matching the
// asyncprocessor/email pattern: the package-level instruments bind to the
// first real MeterProvider ever set via otel.SetMeterProvider, so a second,
// separate test setting its own provider would not observe them.
func TestInstrumentOperation_RecordsMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	require.NoError(t, instrumentOperation(context.Background(), "create", func() error {
		return nil
	}))

	wantErr := errors.New("boom")
	err := instrumentOperation(context.Background(), "get", func() error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr, "instrumentOperation must forward the wrapped fn's error unchanged")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var foundCounter, foundHistogram bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "repo.operation":
				foundCounter = true
			case "repo.operation.duration_ms":
				foundHistogram = true
			}
		}
	}
	assert.True(t, foundCounter, "repo.operation counter must be recorded")
	assert.True(t, foundHistogram, "repo.operation.duration_ms histogram must be recorded")
}
