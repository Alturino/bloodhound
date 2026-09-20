package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/go-jet/jet/v2/generator/metadata"
	jetgenpostgres "github.com/go-jet/jet/v2/generator/postgres"
	"github.com/go-jet/jet/v2/generator/template"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/iancoleman/strcase"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/config"
)

func Get(ctx context.Context, config *config.DB, tracer trace.Tracer) (*sql.DB, error) {
	ctx, span := tracer.Start(ctx, "db.Get")
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

	if err := migrateUp(ctx, config, db, postgresDSN, tracer); err != nil {
		return nil, err
	}

	if err := generateJet(ctx, config, tracer); err != nil {
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

func migrateUp(ctx context.Context, config *config.DB, db *sql.DB, postgresURL string, tracer trace.Tracer) error {
	ctx, span := tracer.Start(
		ctx,
		"db.migrateUp",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := slog.Default().With(slog.String("tag", "db.migrateUp"))

	logger.DebugContext(ctx, "creating postgres driver for migration")
	span.AddEvent("creating postgres driver for migration")
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		err = fmt.Errorf("creating postgres driver for migration: %w", err)
		return err
	}
	logger.DebugContext(ctx, "created postgres driver for migration")
	span.AddEvent("created postgres driver for migration")

	logger.DebugContext(ctx, "creating migration instance")
	span.AddEvent("creating migration instance")
	migration, err := migrate.NewWithDatabaseInstance(config.MigrationPath, postgresURL, driver)
	if err != nil {
		err = fmt.Errorf("migration postgres: %w", err)
		return err
	}
	logger.DebugContext(ctx, "created migration instance")
	span.AddEvent("created migration instance")

	logger.DebugContext(ctx, "running database migration")
	span.AddEvent("running database migration")
	if err := migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		err = fmt.Errorf("migration up: %w", err)
		return err
	}
	logger.DebugContext(ctx, "ran database migration")
	span.AddEvent("ran database migration")

	logger.InfoContext(ctx, "migrated database")

	return nil
}

func generateJet(ctx context.Context, config *config.DB, tracer trace.Tracer) error {
	_, span := tracer.Start(
		ctx,
		"db.generateJet",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(),
	)
	defer span.End()

	dsn := config.DSN()
	destinationdir := filepath.Join(".", "internal", "db", ".gen")
	template := template.Default(postgres.Dialect).
		UseSchema(func(schemaMetaData metadata.Schema) template.Schema {
			return template.DefaultSchema(schemaMetaData).
				UseModel(template.DefaultModel().
					UseTable(func(table metadata.Table) template.TableModel {
						return template.DefaultTableModel(table).
							UseField(func(column metadata.Column) template.TableModelField {
								return template.DefaultTableModelField(column).UseTags(
									fmt.Sprintf("json:\"%s\"", strcase.ToSnake(column.Name)),
								)
							})
					}),
				)
		})

	logger := slog.Default().With(slog.String("tag", "db.generateJet"))

	logger.DebugContext(ctx, "generating jet files")
	span.AddEvent("generating jet files")
	if err := jetgenpostgres.GenerateDSN(dsn, "public", destinationdir, template); err != nil {
		err = fmt.Errorf("generating jet files: %w", err)
		return err
	}
	logger.DebugContext(ctx, "generated jet files")
	span.AddEvent("generated jet files")

	return nil
}
