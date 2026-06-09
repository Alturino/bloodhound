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
	tmt      *telemetry.Telemetry
	client   idx.Client
	annStore idx.AnnouncementStore
	attStore idx.AttachmentStore
}

func initIDXDeps(ctx context.Context, cfg *config.Config) (*idxDependencies, error) {
	logger := log.Get(cfg.App)

	tmt, err := telemetry.New(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize telemetry: %w", err)
	}

	stg, err := blobstorage.NewStorage(cfg.Storage, logger, telemetry.AppTelemetry.Tracer)
	if err != nil {
		return nil, fmt.Errorf("initialize blobstorage: %w", err)
	}

	database, err := db.Get(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}

	annStore := idx.NewAnnouncementStore(database, logger, telemetry.AppTelemetry.Tracer)
	attStore := idx.NewAttachmentStore(database, logger, telemetry.AppTelemetry.Tracer)

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
