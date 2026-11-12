package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/bridges/otelzerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// Provider holds the three OTel signal providers and shuts them down together.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider
	serviceName    string
}

// New initialises all three OTel signals (traces, metrics, logs) and registers
// them as global providers. If cfg.OTel.Enabled is false, no-op
// providers are registered so callers need no conditional logic.
func New(ctx context.Context, cfg *config.Config, log zerolog.Logger) (*Provider, error) {
	if !cfg.OTel.Enabled {
		log.Info().Msg("OpenTelemetry disabled — using no-op providers")
		return &Provider{serviceName: cfg.App.Name}, nil
	}

	endpoint := cfg.OTel.Endpoint

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp, err := buildTracerProvider(ctx, endpoint, cfg.OTel.SampleRate, res)
	if err != nil {
		return nil, fmt.Errorf("otel tracer provider: %w", err)
	}

	mp, err := buildMeterProvider(ctx, endpoint, res)
	if err != nil {
		return nil, fmt.Errorf("otel meter provider: %w", err)
	}

	lp, err := buildLoggerProvider(ctx, endpoint, res)
	if err != nil {
		return nil, fmt.Errorf("otel logger provider: %w", err)
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	global.SetLoggerProvider(lp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Info().
		Str("endpoint", endpoint).
		Float64("sample_rate", cfg.OTel.SampleRate).
		Msg("OpenTelemetry initialised")

	return &Provider{
		tracerProvider: tp,
		meterProvider:  mp,
		loggerProvider: lp,
		serviceName:    cfg.App.Name,
	}, nil
}

// ZerologHook returns a zerolog.Hook that ships every log record to the OTel
// Logs signal. Wire it into the logger after calling New:
//
//	log = log.Hook(obs.ZerologHook())
func (p *Provider) ZerologHook() *otelzerolog.Hook {
	if p.loggerProvider == nil {
		return otelzerolog.NewHook(p.serviceName)
	}
	return otelzerolog.NewHook(p.serviceName, otelzerolog.WithLoggerProvider(p.loggerProvider))
}

// Shutdown flushes and stops all three providers. Call this in main via defer.
func (p *Provider) Shutdown(ctx context.Context) error {
	var firstErr error

	set := func(err error) {
		if firstErr == nil && err != nil {
			firstErr = err
		}
	}

	if p.tracerProvider != nil {
		set(p.tracerProvider.Shutdown(ctx))
	}
	if p.meterProvider != nil {
		set(p.meterProvider.Shutdown(ctx))
	}
	if p.loggerProvider != nil {
		set(p.loggerProvider.Shutdown(ctx))
	}

	return firstErr
}

func buildResource(ctx context.Context, cfg *config.Config) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.App.Name),
			semconv.ServiceVersion(cfg.App.Version),
			semconv.DeploymentEnvironmentName(cfg.App.Env),
		),
	)
}

func buildTracerProvider(ctx context.Context, endpoint string, sampleRate float64, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRate)),
	), nil
}

func buildMeterProvider(ctx context.Context, endpoint string, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	exp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(15*time.Second))),
		sdkmetric.WithResource(res),
	), nil
}

func buildLoggerProvider(ctx context.Context, endpoint string, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	exp, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	return sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		sdklog.WithResource(res),
	), nil
}

// ── Usage examples ────────────────────────────────────────────────────────────
//
// All three signals are available globally via the otel package after New()
// returns. Import only what each package needs — no Provider reference required
// after initialisation.
//
// ── LOGGING (via zerolog — already bridged to OTel Logs) ─────────────────────
//
//	// In any handler or service, receive the logger from context or inject it.
//	// Every zerolog call is automatically forwarded to the OTel collector.
//
//	log.Info().Str("user_id", uid).Msg("user signed in")
//	log.Warn().Err(err).Str("path", r.URL.Path).Msg("upstream timeout, retrying")
//	log.Error().Err(err).Msg("payment failed")
//
//	// Attach the active trace-id so logs and traces correlate in the collector:
//	span := trace.SpanFromContext(ctx)
//	log.Info().
//	    Str("trace_id", span.SpanContext().TraceID().String()).
//	    Str("span_id", span.SpanContext().SpanID().String()).
//	    Msg("processing order")
//
// ── TRACING ──────────────────────────────────────────────────────────────────
//
//	import "go.opentelemetry.io/otel"
//	import "go.opentelemetry.io/otel/attribute"
//
//	// Get a tracer once per package (package name is the instrumentation scope).
//	var tracer = otel.Tracer("github.com/hritul2/goboiler/internal/service/order")
//
//	func (s *OrderService) Create(ctx context.Context, req CreateOrderRequest) error {
//	    ctx, span := tracer.Start(ctx, "OrderService.Create")
//	    defer span.End()
//
//	    span.SetAttributes(
//	        attribute.String("order.user_id", req.UserID),
//	        attribute.Int("order.item_count", len(req.Items)),
//	    )
//
//	    if err := s.repo.Insert(ctx, req); err != nil {
//	        span.RecordError(err)
//	        span.SetStatus(codes.Error, err.Error())
//	        return err
//	    }
//	    return nil
//	}
//
// ── METRICS ──────────────────────────────────────────────────────────────────
//
//	import "go.opentelemetry.io/otel"
//	import "go.opentelemetry.io/otel/metric"
//
//	// Get a meter once per package.
//	var meter = otel.Meter("github.com/hritul2/goboiler/internal/handler/order")
//
//	// Declare instruments at package init (counters, histograms, gauges).
//	var (
//	    orderCounter, _ = meter.Int64Counter(
//	        "orders.created",
//	        metric.WithDescription("Total orders successfully created"),
//	    )
//	    orderLatency, _ = meter.Float64Histogram(
//	        "orders.duration_ms",
//	        metric.WithDescription("End-to-end order creation latency"),
//	        metric.WithUnit("ms"),
//	    )
//	)
//
//	func (h *OrderHandler) Create(c echo.Context) error {
//	    start := time.Now()
//	    // ... process ...
//	    orderCounter.Add(c.Request().Context(), 1,
//	        metric.WithAttributes(attribute.String("status", "ok")),
//	    )
//	    orderLatency.Record(c.Request().Context(),
//	        float64(time.Since(start).Milliseconds()),
//	    )
//	    return c.JSON(http.StatusCreated, resp)
//	}
