package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// Config holds all application configuration
type Config struct {
	Telemetry *Telemetry `mapstructure:"telemetry" json:"telemetry"`
	Storage   *Storage   `mapstructure:"storage"   json:"storage"`
	Database  *DB        `mapstructure:"database"  json:"database"`
	App       *App       `mapstructure:"app"       json:"app"`
}

type App struct {
	LogLevel    slog.Level     `mapstructure:"log_level"   json:"log_level"` // debug, info, warn, error
	LogLevelVar *slog.LevelVar `mapstructure:"-"           json:"-"`
	Name        string         `mapstructure:"name"        json:"name"`
	Hostname    string         `mapstructure:"hostname"    json:"hostname"`
	Environment string         `mapstructure:"environment" json:"environment"` // development, production
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

	v.SetDefault("telemetry.enabled", true)

	// Config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("bloodhound")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./env")
	}

	var cfg Config
	cfg.App = &App{}
	cfg.App.LogLevelVar = &slog.LevelVar{}

	if err := v.ReadInConfig(); err != nil {
		err = fmt.Errorf("read config file: %w", err)
		return &cfg, err
	}

	if err := v.Unmarshal(&cfg); err != nil {
		err = fmt.Errorf("unmarshal config: %w", err)
		return &cfg, err
	}
	cfg.App.LogLevelVar.Set(cfg.App.LogLevel)

	// Extract raw OTel config YAML for otelconf.ParseYAML()
	if otelRaw := v.Get("telemetry.otel"); otelRaw != nil {
		otelYAML, err := yaml.Marshal(otelRaw)
		if err != nil {
			return &cfg, fmt.Errorf("marshal otel config: %w", err)
		}
		cfg.Telemetry.OTelRaw = otelYAML
	}

	return &cfg, nil
}
