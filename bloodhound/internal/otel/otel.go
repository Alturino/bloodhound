package otel

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
)

type ShutdownFunc func(context.Context) error

func InitOtelSdk(
	c context.Context,
	serviceName string,
	config config.Otel,
) (shutdownFuncs []ShutdownFunc, err error) {
	logger := zerolog.Ctx(c).
		With().
		Ctx(c).
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
		c,
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
	c = logger.WithContext(c)
	tracerProvider, err := InitTracerProvider(
		c,
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
	c = logger.WithContext(c)
	metricEndpoint := fmt.Sprintf("%s:%d", config.Host, config.Port)
	meterProvider, err := InitMetricProvider(c, metricEndpoint, res)
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

func ShutdownOtel(c context.Context, shutdownFuncs []ShutdownFunc) error {
	var wg sync.WaitGroup
	var err error
	for _, shutdown := range shutdownFuncs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if newErr := shutdown(c); newErr != nil {
				err = errors.Join(newErr)
			}
		}()
	}
	wg.Wait()
	return err
}
