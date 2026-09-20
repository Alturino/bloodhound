package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"

	"github.com/alturino/bloodhound/internal/config"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// ResolveConfigPath returns the config path from viper flag, falling back to
// "bloodhound.yaml" if unset.
func ResolveConfigPath() string {
	configPath := viper.GetString("config")
	if configPath == "" {
		configPath = "bloodhound.yaml"
	}
	return configPath
}

// LoadConfig loads the application configuration from the given path and
// appends the config_path to the returned context for structured logging.
func LoadConfig(ctx context.Context, configPath string) (*config.Config, context.Context, error) {
	ctx = slogctx.Append(ctx, slog.String("config_path", configPath))

	slog.InfoContext(ctx, "load config")
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, ctx, fmt.Errorf("load config: %w", err)
	}

	return cfg, ctx, nil
}

// StartPprofServer starts the pprof HTTP server on :9999 in a background
// goroutine. The server uses the default mux which registers pprof handlers
// via the net/http/pprof blank import.
func StartPprofServer(ctx context.Context) {
	go func() {
		if err := http.ListenAndServe(":9999", nil); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return
		}
	}()
}

// WatchConfigChange registers a viper OnConfigChange callback that merges
// config file changes and updates the log level.
func WatchConfigChange(ctx context.Context, cfg *config.Config, logger *slog.Logger) {
	viper.OnConfigChange(func(in fsnotify.Event) {
		if !in.Has(fsnotify.Write) {
			return
		}
		if err := viper.MergeInConfig(); err != nil {
			logger.ErrorContext(ctx, fmt.Errorf("merge config file: %w", err).Error())
			return
		}
		if err := viper.Unmarshal(cfg); err != nil {
			logger.ErrorContext(ctx, fmt.Errorf("unmarshal config: %w", err).Error())
			return
		}
		cfg.App.LogLevelVar.Set(cfg.App.LogLevel)
	})
}

// RecoverPanic returns a deferred function that recovers from panics and logs
// them using the provided logger.
func RecoverPanic(logger *slog.Logger) {
	if r := recover(); r != nil {
		logger.Error("panic", slog.Any("panic", r))
	}
}

// ShutdownTelemetry returns a deferred function that shuts down the telemetry
// app and logs any errors.
func ShutdownTelemetry(ctx context.Context, tmt *telemetry.App, logger *slog.Logger) {
	if err := tmt.Shutdown(ctx); err != nil {
		logger.ErrorContext(ctx, fmt.Errorf("shutdown telemetry: %w", err).Error())
	}
}

// CloseDatabase returns a deferred function that closes the database connection
// and logs any errors.
func CloseDatabase(ctx context.Context, db *sql.DB, logger *slog.Logger) {
	if err := db.Close(); err != nil {
		logger.ErrorContext(ctx, fmt.Errorf("close database: %w", err).Error())
	}
}
