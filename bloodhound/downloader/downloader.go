package downloader

import (
	"context"
	"path/filepath"

	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
	"github.com/Alturino/bloodhound/internal/db"
	"github.com/Alturino/bloodhound/internal/logging"
	"github.com/Alturino/bloodhound/internal/nats"
)

func StartDownloader(ctx context.Context) {
	configPath := filepath.Join(".", "downloader.yaml")

	cfg := config.Get(ctx, configPath)

	logger := logging.Get().With().
		Str(constants.KEY_TAG, "downloader StartDownloader").
		Str(constants.KEY_APP, "downloader").
		Any(constants.KEY_CONFIG, cfg).
		Logger()

	logger.Debug().Msg("initializing db")
	pool := db.Get(ctx, cfg.Database)
	defer func() {
		logger.Debug().Msg("closing db")
		pool.Close()
		logger.Info().Msg("closed db")
	}()
	logger.Info().Msg("initialized db")

	logger.Debug().Msg("initializing nats")
	nats := nats.Get(ctx, cfg.Nats)
	defer func() {
		logger.Debug().Msg("closing nats")
		nats.Close()
		logger.Info().Msg("closed nats")
	}()
	logger.Info().Msg("initialized nats")
}
