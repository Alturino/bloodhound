package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/otelconf"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/alturino/bloodhound/internal/config"
)

// App holds the OpenTelemetry providers
type App struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Tracer         trace.Tracer
	Metrics        *MetricsProvider
	Logger         *slog.Logger
	shutdown       func(ctx context.Context) error
}

// New creates a new App instance with configured providers
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	if !cfg.Telemetry.Enabled {
		tp, mp := tracenoop.NewTracerProvider(), metricnoop.NewMeterProvider()
		otel.SetTracerProvider(tp)
		otel.SetMeterProvider(mp)

		mtr := mp.Meter(cfg.App.ServiceName())
		metrics, err := NewMetrics(mtr)
		if err != nil {
			return nil, err
		}

		return &App{
			TracerProvider: tp,
			MeterProvider:  mp,
			Tracer:         tp.Tracer(cfg.App.ServiceName()),
			Metrics:        metrics,
			Logger:         slog.Default(),
		}, nil
	}

	// Marshal the otel config subtree to YAML bytes
	otelYAML := cfg.Telemetry.OTelRaw
	if len(otelYAML) == 0 {
		return nil, fmt.Errorf("no otel config found in telemetry.otel")
	}

	// Set env vars that otelconf ParseYAML will substitute
	os.Setenv("OTEL_SERVICE_NAME", cfg.App.ServiceName())
	os.Setenv("OTEL_ENVIRONMENT", cfg.App.Environment)

	// Parse otelconf YAML (handles ${VAR} substitution internally)
	otelCfg, err := otelconf.ParseYAML(otelYAML)
	if err != nil {
		return nil, fmt.Errorf("parse otel config: %w", err)
	}

	// Create SDK from declarative config
	sdk, err := otelconf.NewSDK(
		otelconf.WithOpenTelemetryConfiguration(*otelCfg),
		otelconf.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel sdk: %w", err)
	}

	// Set global providers
	otel.SetTracerProvider(sdk.TracerProvider())
	otel.SetMeterProvider(sdk.MeterProvider())
	otel.SetTextMapPropagator(sdk.Propagator())

	// Create custom metrics from meter provider
	mtr := sdk.MeterProvider().Meter(cfg.App.ServiceName())
	metrics, err := NewMetrics(mtr)
	if err != nil {
		return nil, err
	}

	// Initialize structured logger
	logger := initLogger(cfg)

	return &App{
		TracerProvider: sdk.TracerProvider(),
		MeterProvider:  sdk.MeterProvider(),
		Tracer:         sdk.TracerProvider().Tracer(cfg.App.ServiceName()),
		Metrics:        metrics,
		Logger:         logger,
		shutdown:       sdk.Shutdown,
	}, nil
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
func (t *App) Shutdown(ctx context.Context) error {
	if t.shutdown != nil {
		return t.shutdown(ctx)
	}
	return nil
}

func RecordError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
}
