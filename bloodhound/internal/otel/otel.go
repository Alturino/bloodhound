package otel

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"golang.org/x/sync/errgroup"

	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
)

type ShutdownFunc func(context.Context) error

func InitOtelSdk(
	ctx context.Context,
	serviceName string,
	config config.Otel,
) (shutdownFuncs []ShutdownFunc, err error) {
	logger := zerolog.Ctx(ctx).
		With().
		Ctx(ctx).
		Str(constants.KEY_TAG, "main InitOtelSdk").
		Logger()

	logger = logger.With().Str(constants.KEY_PROCESS, "initializing otel propagator").Logger()
	logger.Info().Msg("initializing otel propagator")
	propagator := propagation.NewCompositeTextMapPropagator(
		jaeger.Jaeger{},
		ot.OT{},
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(propagator)
	logger.Info().Msg("initialized otel propagator")

	res, err := resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithContainer(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithHostID(),
		resource.WithOS(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		err = fmt.Errorf("failed initializing otel tracerProvider with error=%w", err)
		logger.Error().Err(err).Msg(err.Error())
		return nil, err
	}

	logger = logger.With().Str(constants.KEY_PROCESS, "initializing otel tracerProvider").Logger()
	logger.Info().Msg("initializing otel tracerProvider")
	ctx = logger.WithContext(ctx)
	tracerProvider, err := InitTracerProvider(
		ctx,
		fmt.Sprintf("%s:%d", config.Host, config.Port),
		serviceName,
		res,
	)
	if err != nil {
		err = fmt.Errorf("failed initializing otel tracerProvider with error=%w", err)
		logger.Error().Err(err).Msg(err.Error())
		return nil, err
	}
	otel.SetTracerProvider(tracerProvider)
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	logger.Info().Msg("initialized otel tracerProvider")

	logger = logger.With().Str(constants.KEY_PROCESS, "initializing meterProvider").Logger()
	logger.Info().Msg("initializing meterProvider")
	ctx = logger.WithContext(ctx)
	metricEndpoint := fmt.Sprintf("%s:%d", config.Host, config.Port)
	meterProvider, err := InitMetricProvider(ctx, metricEndpoint, res)
	if err != nil {
		err = fmt.Errorf("failed initializing otel meterProvider with error=%w", err)
		logger.Error().Err(err).Msg(err.Error())
		return shutdownFuncs, err
	}
	otel.SetMeterProvider(meterProvider)
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	logger.Info().Msg("initialized meterProvider")

	return shutdownFuncs, nil
}

func ShutdownOtel(ctx context.Context, shutdownFuncs []ShutdownFunc) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	erg, ctx := errgroup.WithContext(ctx)
	for _, shutdown := range shutdownFuncs {
		erg.Go(func() error { return shutdown(ctx) })
	}
	return erg.Wait()
}
