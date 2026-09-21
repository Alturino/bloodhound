package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/otelconf"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Telemetry *otelconf.OpenTelemetryConfiguration `mapstructure:"telemetry" json:"telemetry"`
	Storage   *Storage                             `mapstructure:"storage"   json:"storage"`
	Database  *DB                                  `mapstructure:"database"  json:"database"`
	App       *App                                 `mapstructure:"app"       json:"app"`
}

type App struct {
	Enabled     bool           `mapstructure:"enabled"     json:"enabled"`
	LogLevel    slog.Level     `mapstructure:"log_level"   json:"log_level"`
	LogLevelVar *slog.LevelVar `mapstructure:"-"           json:"-"`
	Name        string         `mapstructure:"name"        json:"name"`
	Hostname    string         `mapstructure:"hostname"    json:"hostname"`
	Environment string         `mapstructure:"environment" json:"environment"`
	LogDir      string         `mapstructure:"log_dir"     json:"log_dir"`
	IDX         IDX            `mapstructure:"idx"         json:"idx"`
	Stockbit    Stockbit       `mapstructure:"stockbit"    json:"stockbit"`
}

func (a *App) ServiceName() string {
	return fmt.Sprintf("%s_%s_%s", a.Name, a.Hostname, a.Environment)
}

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.GetViper()
	v.SetEnvPrefix("bloodhound")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.WatchConfig()

	// Set defaults
	v.SetDefault("app.name", "bloodhound")
	v.MustBindEnv("app.hostname", "HOSTNAME")
	v.SetDefault("app.environment", "development")
	v.SetDefault("app.log_level", "info")
	v.SetDefault("app.enabled", true)

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "")
	v.SetDefault("database.password", "")
	v.SetDefault("database.dbname", "idx_fetcher")
	v.SetDefault("database.sslmode", "require")

	v.SetDefault("minio.enabled", true)
	v.SetDefault("minio.endpoint", "localhost:9000")
	v.SetDefault("minio.access_key", "")
	v.SetDefault("minio.secret_key", "")
	v.SetDefault("minio.bucket", "idx-announcements")
	v.SetDefault("minio.use_ssl", true)

	v.SetDefault("app.idx.base_url", "https://idx.co.id")
	v.SetDefault("app.idx.page_size", 10)
	v.SetDefault("app.idx.worker_count", 3)
	v.SetDefault("app.idx.worker_pool.announcement_workers", 4)
	v.SetDefault("app.idx.worker_pool.attachment_workers", 4)

	v.SetDefault("app.stockbit.base_url", "https://exodus.stockbit.com")
	v.SetDefault("app.stockbit.token", "")

	v.SetDefault("scheduler.interval", 15*time.Minute)
	v.SetDefault("scheduler.cron_expr", "*/15 * * * *")

	// Config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("bloodhound")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./env")
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Unmarshal non-telemetry fields via Viper (handles time.Duration etc.).
	// We decode into a struct without the Telemetry field because mapstructure
	// cannot handle otelconf's discriminated union types.
	var raw struct {
		Storage *Storage `mapstructure:"storage"`
		Database  *DB    `mapstructure:"database"`
		App       *App   `mapstructure:"app"`
	}
	raw.App = &App{}
	raw.App.LogLevelVar = &slog.LevelVar{}

	if err := v.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	raw.App.LogLevelVar.Set(raw.App.LogLevel)

	cfg := &Config{
		Storage:  raw.Storage,
		Database: raw.Database,
		App:      raw.App,
	}

	// Parse telemetry with otelconf (mapstructure cannot handle otelconf types)
	if t := v.AllSettings()["telemetry"]; t != nil {
		if telemetryMap, ok := t.(map[string]any); ok {
			if err := parseTelemetry(telemetryMap, cfg); err != nil {
				return nil, fmt.Errorf("load telemetry config: %w", err)
			}
		}
	}

	return cfg, nil
}

// parseTelemetry takes the raw telemetry subtree and parses it with
// otelconf.ParseYAML().
func parseTelemetry(raw map[string]any, cfg *Config) error {
	// Extract enabled flag
	if enabled, ok := raw["enabled"]; ok {
		if e, ok := enabled.(bool); ok {
			cfg.App.Enabled = e
		}
	}

	// Remove non-otel fields
	delete(raw, "enabled")

	// Marshal to YAML for otelconf.ParseYAML()
	otelYAML, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal telemetry to yaml: %w", err)
	}

	// Parse with otelconf (handles ${VAR} substitution internally)
	otelCfg, err := otelconf.ParseYAML(otelYAML)
	if err != nil {
		return fmt.Errorf("parse otel config: %w", err)
	}

	cfg.Telemetry = otelCfg
	return nil
}
