package config

import (
	"fmt"
	"log/slog"
	"os"
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

var config Config

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

	config := Config{App: &App{LogLevelVar: &slog.LevelVar{}}}

	if err := v.ReadInConfig(); err != nil {
		err = fmt.Errorf("read config file: %w", err)
		return &config, err
	}

	if err := v.Unmarshal(&config); err != nil {
		err = fmt.Errorf("unmarshal config: %w", err)
		return &config, err
	}
	cfg.App.LogLevelVar.Set(cfg.App.LogLevel)

	config.Storage.Local.BloodhoundDir = expandPath(config.Storage.Local.BloodhoundDir)
	config.App.LogLevelVar.Set(config.App.LogLevel)

	return &config, nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		path = strings.Replace(path, "~", "$HOME", 1)
	}

	path = os.ExpandEnv(path)

	return path
}
