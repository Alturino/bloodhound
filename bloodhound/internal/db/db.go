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
)

var (
	once sync.Once
	pool *pgxpool.Pool
)

func Get(
	ctx context.Context,
	config config.Database,
) *pgxpool.Pool {
	once.Do(func() {
		// ctx, span := trace.Tracer.Start(ctx, "main NewDatabaseClient")
		// defer span.End()

		logger := zerolog.Ctx(ctx).
			With().
			Str(constants.KEY_PROCESS, "connecting to database").
			Logger()

		logger.Debug().Msg("connecting to database")

		logger.Debug().Msg("parsing postgres url")
		postgresUrl := postgresUrl(config)

		logger = logger.With().Str(constants.KEY_PROCESS, "initializing pgx config").Logger()
		logger.Trace().Msg("initializing pgx config")
		pgxConfig, err := pgxpool.ParseConfig(postgresUrl)
		if err != nil {
			err = fmt.Errorf("failed creating pgx config with error: %w", err)
			logger.Fatal().Err(err).Msg(err.Error())
		}
		logger.Info().Msg("initialized pgx config")

		pgxConfig.AfterConnect = func(ctx context.Context, pgxConn *pgx.Conn) error {
			pgxUUID.Register(pgxConn.TypeMap())
			return nil
		}

		logger = logger.With().Str(constants.KEY_PROCESS, "attaching otel tracer to pgx").Logger()
		logger.Trace().Msg("attaching otel tracer to pgx")
		pgxConfig.ConnConfig.Tracer = otelpgx.NewTracer(
			otelpgx.WithTracerAttributes(semconv.DBSystemPostgreSQL),
		)
		logger.Info().Msgf("attached otel tracer to pgx")

		logger = logger.With().Str(constants.KEY_PROCESS, "creating connection pool").Logger()
		logger.Trace().Msg("creating connection pool")
		pool, err = pgxpool.NewWithConfig(ctx, pgxConfig)
		if err != nil {
			err = fmt.Errorf("failed creating connection pool with error: %w", err)
			logger.Fatal().Err(err).Msg(err.Error())
		}
		logger.Info().Msg("created connection pool")

		logger.Debug().Msg("pinging database")
		err = pool.Ping(ctx)
		if err != nil {
			err = fmt.Errorf("failed ping db with error: %w", err)
			logger.Error().Err(err).Msg(err.Error())
			return
		}
		logger.Info().Msg("pinged database")

		if err = MigrateUp(ctx, config, pool, postgresUrl); err != nil {
			err = fmt.Errorf("failed migration up with error: %w", err)
			logger.Fatal().Err(err).Msg(err.Error())
		}

		logger.Info().Msg("created connection to database")
	})

	return pool
}

func postgresUrl(config config.Database) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		config.Username,
		config.Password,
		config.Host,
		config.Port,
		config.Name,
	)
}
