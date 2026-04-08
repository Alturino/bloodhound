package telemetry

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/alturino/bloodhound/config"
)

// Telemetry holds the OpenTelemetry providers
type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Tracer         trace.Tracer
	Meter          metric.Meter
	Logger         *slog.Logger
}

var AppTelemetry Telemetry

// New creates a new Telemetry instance with configured providers
// TODO: refactor use otelconf package to simplify configuration and initialization
func New(ctx context.Context, cfg *config.Config) (*Telemetry, error) {
	if !cfg.Telemetry.Enabled {
		tp, mp := tracenoop.NewTracerProvider(), metricnoop.NewMeterProvider()
		otel.SetTracerProvider(tp)
		otel.SetMeterProvider(mp)
		return &Telemetry{
			TracerProvider: nil,
			MeterProvider:  nil,
			Tracer:         tp.Tracer(cfg.Telemetry.ServiceName),
			Meter:          mp.Meter(cfg.Telemetry.ServiceName),
			Logger:         slog.Default(),
		}, nil
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.Telemetry.ServiceName),
			semconv.DeploymentEnvironment(cfg.App.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	// 1. Initialize Traces
	tp, err := initTracer(ctx, cfg, res)
	if err != nil {
		return nil, err
	}

	// 2. Initialize Metrics
	mp, err := initMeter(ctx, cfg, res)
	if err != nil {
		return nil, err
	}

	// 3. Initialize Logger
	logger := initLogger(cfg)

	AppTelemetry = Telemetry{
		TracerProvider: tp,
		MeterProvider:  mp,
		Tracer:         tp.Tracer(cfg.Telemetry.ServiceName),
		Meter:          mp.Meter(cfg.Telemetry.ServiceName),
		Logger:         logger,
	}
	return &AppTelemetry, nil
}

func initTracer(
	ctx context.Context,
	cfg *config.Config,
	res *resource.Resource,
) (*sdktrace.TracerProvider, error) {
	var exporter sdktrace.SpanExporter
	var err error

	exporter, err = otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(cfg.Telemetry.OTLPEndpoint))
	if cfg.App.Environment != "production" {
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	}

	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

func initMeter(
	ctx context.Context,
	cfg *config.Config,
	res *resource.Resource,
) (*sdkmetric.MeterProvider, error) {
	var exporter sdkmetric.Exporter
	var err error

	exporter, err = otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpoint(cfg.Telemetry.OTLPEndpoint))
	if cfg.App.Environment != "production" {
		exporter, err = stdoutmetric.New(stdoutmetric.WithPrettyPrint())
	}

	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return mp, nil
}

func initLogger(cfg *config.Config) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	handler = slog.NewJSONHandler(os.Stdout, opts)
	if cfg.App.Environment != "production" {
		opts.Level = slog.LevelDebug
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

// Shutdown gracefully shuts down the telemetry providers
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t.TracerProvider != nil {
		if err := t.TracerProvider.Shutdown(ctx); err != nil {
			return err
		}
	}
	if t.MeterProvider != nil {
		if err := t.MeterProvider.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

func RecordError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.AddEvent(err.Error())
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
}
