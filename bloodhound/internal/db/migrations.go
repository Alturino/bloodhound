package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
)

func MigrateUp(
	ctx context.Context,
	config config.Database,
	pool *pgxpool.Pool,
	postgresURL string,
) error {
	logger := zerolog.Ctx(ctx).With().Str(constants.KEY_TAG, "db MigrateUp").Logger()
	logger.Debug().Msg("creating sql.DB from pool")
	db := stdlib.OpenDBFromPool(pool)
	logger.Debug().Msg("created sql.DB from pool")

	logger.Debug().Msg("creating postgres driver for migration")
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		err = fmt.Errorf("failed creating postgres driver for migration with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Info().Msg("created postgres driver for migration")

	logger.Debug().Msg("creating migration instance")
	migration, err := migrate.NewWithDatabaseInstance(config.MigrationPath, postgresURL, driver)
	if err != nil {
		err = fmt.Errorf("failed migration postgres with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Debug().Msg("created migration instance")

	err = migration.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		err = fmt.Errorf("failed migration up with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Info().Msg("migrations up done")

	return nil
}
