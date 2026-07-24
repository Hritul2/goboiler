package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/hritul2/goboiler/internal/models"
)

// Compile-time interface compliance.
var _ UserRepository = (*mongoUserRepo)(nil)

func TestUserIndexModels(t *testing.T) {
	idx := userIndexModels()
	require.Len(t, idx, 1, "users collection defines exactly one index")

	keys, ok := idx[0].Keys.(bson.D)
	require.True(t, ok, "Keys must be an order-preserving bson.D")
	require.Len(t, keys, 1)
	assert.Equal(t, "email", keys[0].Key)
	assert.EqualValues(t, 1, keys[0].Value)

	require.NotNil(t, idx[0].Options)
	var opts options.IndexOptions
	for _, set := range idx[0].Options.List() {
		require.NoError(t, set(&opts))
	}
	require.NotNil(t, opts.Unique, "email index must set Unique")
	assert.True(t, *opts.Unique)
}

func TestListFindOptions(t *testing.T) {
	p := models.Pagination{Page: 3, Limit: 10}
	p.Normalize()

	var opts options.FindOptions
	for _, set := range listFindOptions(p).List() {
		require.NoError(t, set(&opts))
	}

	require.NotNil(t, opts.Skip)
	assert.EqualValues(t, 20, *opts.Skip)
	require.NotNil(t, opts.Limit)
	assert.EqualValues(t, 10, *opts.Limit)

	sort, ok := opts.Sort.(bson.D)
	require.True(t, ok, "Sort must be an order-preserving bson.D")
	require.Len(t, sort, 1)
	assert.Equal(t, "created_at", sort[0].Key)
	assert.EqualValues(t, -1, sort[0].Value, "newest first")
}

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
