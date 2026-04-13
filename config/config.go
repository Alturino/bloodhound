package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

var cfg Config

func init() {
	cfg.App.LogLevelVar = &slog.LevelVar{}
}

// Config holds all application configuration
type Config struct {
	Scheduler Scheduler `mapstructure:"scheduler"`
	Telemetry Telemetry `mapstructure:"telemetry"`
	MinIO     MinIO     `mapstructure:"minio"`
	Database  Database  `mapstructure:"database"`
	App       App       `mapstructure:"app"`
}

type App struct {
	LogLevel    slog.Level `mapstructure:"log_level"` // debug, info, warn, error
	LogLevelVar *slog.LevelVar
	Name        string   `mapstructure:"name"`
	Environment string   `mapstructure:"environment"` // development, production
	LogDir      string   `mapstructure:"log_dir"`
	IDX         IDX      `mapstructure:"idx"`
	Stockbit    Stockbit `mapstructure:"stockbit"`
}

type Stockbit struct {
	Token   string `mapstructure:"token"`
	BaseURL string `mapstructure:"base_url"`
}

type Database struct {
	MaxConnections int    `mapstructure:"max_connections" json:"max_connections"`
	MinConnections int    `mapstructure:"min_connections" json:"min_connections"`
	Port           int    `mapstructure:"port"`
	Host           string `mapstructure:"host"`
	MigrationPath  string `mapstructure:"migration_path"  json:"migration_path"`
	User           string `mapstructure:"user"`
	Password       string `mapstructure:"password"`
	DBName         string `mapstructure:"dbname"`
	SSLMode        string `mapstructure:"sslmode"`
}

func (d Database) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User,
		d.Password,
		d.Host,
		d.Port,
		d.DBName,
		d.SSLMode,
	)
}

type MinIO struct {
	UseSSL    bool   `mapstructure:"use_ssl"`
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
}

type IDX struct {
	PageSize int    `mapstructure:"page_size"`
	BaseURL  string `mapstructure:"base_url"`
	Token    string `mapstructure:"token"`
	MockMode bool   `mapstructure:"mock_mode"`
}

type Scheduler struct {
	Interval time.Duration `mapstructure:"interval"`
	CronExpr string        `mapstructure:"cron_expr"` // e.g., "*/15 * * * *"
}

type Telemetry struct {
	Enabled      bool   `mapstructure:"enabled"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
	ServiceName  string `mapstructure:"service_name"`
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
	v.SetDefault("app.environment", "development")
	v.SetDefault("app.log_level", "info")

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "postgres")
	v.SetDefault("database.dbname", "idx_fetcher")
	v.SetDefault("database.sslmode", "disable")

	v.SetDefault("minio.endpoint", "localhost:9000")
	v.SetDefault("minio.access_key", "minioadmin")
	v.SetDefault("minio.secret_key", "minioadmin")
	v.SetDefault("minio.bucket", "idx-announcements")
	v.SetDefault("minio.use_ssl", false)

	v.SetDefault("app.idx.base_url", "https://idx.co.id")
	v.SetDefault("app.idx.page_size", 10)

	v.SetDefault("app.stockbit.base_url", "https://exodus.stockbit.com")
	v.SetDefault("app.stockbit.token", "")

	v.SetDefault("scheduler.interval", 15*time.Minute)
	v.SetDefault("scheduler.cron_expr", "*/15 * * * *")

	v.SetDefault("telemetry.enabled", true)
	v.SetDefault("telemetry.service_name", "bloodhound")

	// Config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("bloodhound")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./env")
	}
	v.OnConfigChange(func(in fsnotify.Event) {
		if err := v.MergeInConfig(); err != nil {
			err = fmt.Errorf("merge config file: %w", err)
			slog.Error(err.Error())
		}

		if err := v.Unmarshal(&cfg); err != nil {
			err = fmt.Errorf("unmarshal config: %w", err)
			slog.Error(err.Error())
		}
	})

	if err := v.ReadInConfig(); err != nil {
		err = fmt.Errorf("read config file: %w", err)
		return &cfg, err
	}

	if err := v.Unmarshal(&cfg); err != nil {
		err = fmt.Errorf("unmarshal config: %w", err)
		return &cfg, err
	}

	return &cfg, nil
}
