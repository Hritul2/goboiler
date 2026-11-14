package user

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hritul2/goboiler/internal/validation"
)

func TestUserService_Create_RecordsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	svc := NewUserService(newFakeUserRepo(), &fakeEnqueuer{}, zerolog.Nop())

	_, err := svc.Create(context.Background(), validation.UserCreateInput{
		Email: "ada@example.com", Name: "Ada", Password: "password123",
	})
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "UserService.Create", spans[0].Name)
}
