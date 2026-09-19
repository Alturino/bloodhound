package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/blobstorage"
	"github.com/alturino/bloodhound/internal/db"
	"github.com/alturino/bloodhound/internal/httpclient"
	"github.com/alturino/bloodhound/internal/idx"
	"github.com/alturino/bloodhound/internal/log"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type idxDependencies struct {
	db       *sql.DB
	logger   *slog.Logger
	stg      blobstorage.Storage
	tmt      *telemetry.App
	client   idx.Client
	annStore idx.AnnouncementStore
	attStore idx.AttachmentStore
}

func initIDXDeps(ctx context.Context, cfg *config.Config) (*idxDependencies, error) {
	logger, err := log.Get(cfg.App)
	if err != nil {
		return nil, fmt.Errorf("initialize logger: %w", err)
	}

	logger.DebugContext(ctx, "initializing telemetry")
	tmt, err := telemetry.New(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize telemetry: %w", err)
	}
	logger.InfoContext(ctx, "initialized telemetry")

	logger.DebugContext(ctx, "initializing blobstorage")
	stg, err := blobstorage.NewStorage(cfg.Storage, logger, telemetry.AppTelemetry.Tracer)
	if err != nil {
		return nil, fmt.Errorf("initialize blobstorage: %w", err)
	}
	logger.InfoContext(ctx, "initialized blobstorage")

	logger.DebugContext(ctx, "initializing database")
	database, err := db.Get(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}

	logger.DebugContext(ctx, "initializing AnnouncementStore")
	annStore := idx.NewAnnouncementStore(database, logger, telemetry.AppTelemetry.Tracer)
	logger.DebugContext(ctx, "initialized AnnouncementStore")

	logger.DebugContext(ctx, "initializing AttachmentStore")
	attStore := idx.NewAttachmentStore(database, logger, telemetry.AppTelemetry.Tracer)
	logger.DebugContext(ctx, "initialized AttachmentStore")

	httpClient := httpclient.NewClient(cfg, telemetry.AppTelemetry.Tracer)
	client := idx.NewClient(
		httpClient,
		&cfg.App.IDX,
		logger.With(slog.String("tag", "idx.Client")),
		telemetry.AppTelemetry.Tracer,
	)

	return &idxDependencies{
		db:       database,
		logger:   logger,
		stg:      stg,
		tmt:      tmt,
		client:   client,
		annStore: annStore,
		attStore: attStore,
	}, nil
}
