package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/telemetry"
)

func Get(ctx context.Context, config *config.Database) (*sql.DB, error) {
	ctx, span := telemetry.AppTelemetry.Tracer.Start(ctx, "db.Get")
	defer span.End()

	logger := slog.Default().With(
		slog.String("tag", "db.Get"),
	)
	logger.DebugContext(ctx, "connecting to database")

	logger.DebugContext(ctx, "creating connection pool")
	postgresDSN := config.DSN()
	db, err := otelsql.Open(
		"postgres",
		postgresDSN,
		otelsql.WithAttributes(semconv.DBSystemNamePostgreSQL),
	)
	if err != nil {
		err = fmt.Errorf("parsing postgres url: %w", err)
		logger.ErrorContext(ctx, err.Error())
		return nil, err
	}
	logger.InfoContext(ctx, "open connection to database")
	span.AddEvent("open connection to database")

	logger.DebugContext(ctx, "pinging database")
	if err := db.PingContext(ctx); err != nil {
		err = fmt.Errorf("pinging db: %w", err)
		return nil, err
	}
	logger.InfoContext(ctx, "connected to database")
	span.AddEvent("connected to database")

	if err := migrateUp(config, db, postgresDSN); err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(time.Minute * 15)
	db.SetConnMaxIdleTime(time.Minute * 5)
	db.SetMaxOpenConns(config.MaxConnections)
	db.SetMaxIdleConns(config.MinConnections)

	logger.InfoContext(ctx, "created connection to database")
	span.AddEvent("created connection to database")
	return db, nil
}

func migrateUp(config *config.Database, db *sql.DB, postgresURL string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		err = fmt.Errorf("creating postgres driver for migration: %w", err)
		return err
	}

	migration, err := migrate.NewWithDatabaseInstance(config.MigrationPath, postgresURL, driver)
	if err != nil {
		err = fmt.Errorf("migration postgres: %w", err)
		return err
	}

	if err := migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		err = fmt.Errorf("migration up: %w", err)
		return err
	}
	return nil
}
