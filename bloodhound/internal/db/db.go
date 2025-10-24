package db

import (
	"context"
	"fmt"
	"sync"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	pgxUUID "github.com/vgarvardt/pgx-google-uuid/v5"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
	"github.com/Alturino/bloodhound/internal/otel/otelutil"
)

var pool *pgxpool.Pool

func Get(
	ctx context.Context,
	config config.Database,
) (*pgxpool.Pool, error) {
	ctx, span := otelutil.Tracer.Start(ctx, "main NewDatabaseClient")
	defer span.End()

	logger := zerolog.Ctx(ctx).
		With().
		Str(constants.KEY_PROCESS, "connecting to database").
		Str(constants.KEY_TAG, "db Get").
		Logger()

	return sync.OnceValues(func() (*pgxpool.Pool, error) {
		logger.Debug().Msg("connecting to database")

		logger.Debug().Msg("parsing postgres url")
		postgresURL := postgresURL(config)

		logger = logger.With().Str(constants.KEY_PROCESS, "initializing pgx config").Logger()
		logger.Debug().Msg("initializing pgx config")
		pgxConfig, err := pgxpool.ParseConfig(postgresURL)
		if err != nil {
			err = fmt.Errorf("failed creating pgx config with error: %w", err)
			return nil, err
		}
		logger.Debug().Msg("initialized pgx config")

		logger = logger.With().Str(constants.KEY_PROCESS, "attaching otel to pgx").Logger()
		logger.Debug().Msg("attaching otel to pgx")
		pgxConfig.AfterConnect = func(ctx context.Context, pgxConn *pgx.Conn) error {
			pgxUUID.Register(pgxConn.TypeMap())
			return nil
		}
		logger.Debug().Msg("attached otel tracer to pgx config")

		logger = logger.With().Str(constants.KEY_PROCESS, "attaching otel tracer to pgx").Logger()
		logger.Trace().Msg("attaching otel tracer to pgx")
		pgxConfig.ConnConfig.Tracer = otelpgx.NewTracer(
			otelpgx.WithTracerAttributes(semconv.DBSystemPostgreSQL),
		)
		logger.Info().Msg("attached otel tracer to pgx")

		logger = logger.With().Str(constants.KEY_PROCESS, "creating connection pool").Logger()
		logger.Trace().Msg("creating connection pool")
		pool, err = pgxpool.NewWithConfig(ctx, pgxConfig)
		if err != nil {
			err = fmt.Errorf("failed creating connection pool with error: %w", err)
			return nil, err
		}
		logger.Info().Msg("created connection pool")

		logger.Debug().Msg("pinging database")
		err = pool.Ping(ctx)
		if err != nil {
			err = fmt.Errorf("failed ping db with error: %w", err)
			return nil, err
		}
		logger.Info().Msg("pinged database")

		logger = logger.With().Str(constants.KEY_PROCESS, "running migrations").Logger()
		logger.Debug().Msg("running migrations")
		if err = MigrateUp(ctx, config, pool, postgresURL); err != nil {
			err = fmt.Errorf("failed migration up with error: %w", err)
			return nil, err
		}
		logger.Debug().Msg("ran migrations")
		return pool, nil
	})()
}

func postgresURL(config config.Database) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		config.Username,
		config.Password,
		config.Host,
		config.Port,
		config.Name,
	)
}
