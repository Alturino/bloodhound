package nats

import (
	"context"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
)

var (
	once     sync.Once
	natsConn *nats.Conn
)

func disconnectErrHandler(ctx context.Context) nats.ConnErrHandler {
	logger := zerolog.Ctx(ctx).
		With().
		Ctx(ctx).
		Str(constants.KEY_TAG, "nats disconnectErrHandler").
		Logger()
	return func(c *nats.Conn, err error) {
		err = fmt.Errorf("nats disconnected with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
	}
}

func reconnectErrHandler(ctx context.Context) nats.ConnErrHandler {
	logger := zerolog.Ctx(ctx).
		With().
		Ctx(ctx).
		Str(constants.KEY_TAG, "nats reconnectErrHandler").
		Logger()
	return func(_ *nats.Conn, err error) {
		err = fmt.Errorf("nats failed to reconnect with err: %w", err)
		logger.Error().Err(err).Msg(err.Error())
	}
}

func reconnectHandler(ctx context.Context) nats.ConnHandler {
	logger := zerolog.Ctx(ctx).
		With().
		Ctx(ctx).
		Str(constants.KEY_TAG, "nats reconnectHandler").
		Logger()
	return func(c *nats.Conn) {
		logger.Debug().Msg("nats reconnected")
	}
}

func closedHandler(ctx context.Context) nats.ConnHandler {
	logger := zerolog.Ctx(ctx).
		With().
		Ctx(ctx).
		Str(constants.KEY_TAG, "nats closedHandler").
		Logger()
	return func(c *nats.Conn) {
		logger.Debug().Msg("nats closed")
	}
}

func discoverHandler(ctx context.Context) nats.ConnHandler {
	logger := zerolog.Ctx(ctx).
		With().
		Ctx(ctx).
		Str(constants.KEY_TAG, "nats discoverHandler").
		Logger()
	return func(c *nats.Conn) {
		logger.Debug().
			Strs("known_nats_servers", c.Servers()).
			Strs("discovered_nats_servers", c.DiscoveredServers()).
			Msg("nats discovered new server")
	}
}

func errHandler(ctx context.Context) nats.ErrHandler {
	logger := zerolog.Ctx(ctx).
		With().
		Ctx(ctx).
		Str(constants.KEY_TAG, "nats errHandler").
		Logger()
	return func(c *nats.Conn, s *nats.Subscription, err error) {
		err = fmt.Errorf("nats received err: %w", err)
		logger.Error().Err(err).Msg(err.Error())
	}
}

func Get(ctx context.Context, config config.Nats) (*nats.Conn, error) {
	endpoint := fmt.Sprintf("%s:%d", config.Host, config.Port)
	var err error
	once.Do(func() {
		natsConn, err = nats.Connect(
			endpoint,
			nats.DisconnectErrHandler(disconnectErrHandler(ctx)),
			nats.ReconnectErrHandler(reconnectErrHandler(ctx)),
			nats.ReconnectHandler(reconnectHandler(ctx)),
			nats.ClosedHandler(closedHandler(ctx)),
			nats.DiscoveredServersHandler(discoverHandler(ctx)),
			nats.ErrorHandler(errHandler(ctx)),
		)
		if err != nil {
			err = fmt.Errorf("failed connecting to nats with error: %w", err)
			return
		}
	})
	return natsConn, err
}
