package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Database  DatabaseConfig  `mapstructure:"database"`
	MinIO     MinIOConfig     `mapstructure:"minio"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
}

type AppConfig struct {
	Name        string     `mapstructure:"name"`
	Environment string     `mapstructure:"environment"` // development, production
	LogDir      string     `mapstructure:"log_dir"`
	LogLevel    slog.Level `mapstructure:"log_level"` // debug, info, warn, error
	Name        string `mapstructure:"name"`
	Environment string `mapstructure:"environment"` // development, production
	LogLevel    string `mapstructure:"log_level"`   // debug, info, warn, error
	Name        string    `mapstructure:"name"`
	Environment string    `mapstructure:"environment"` // development, production
	LogLevel    string    `mapstructure:"log_level"`   // debug, info, warn, error
	IDX         IDXConfig `mapstructure:"idx"`
	Name        string     `mapstructure:"name"`
	Environment string     `mapstructure:"environment"` // development, production
	LogDir      string     `mapstructure:"log_dir"`
	LogLevel    slog.Level `mapstructure:"log_level"` // debug, info, warn, error
	Name        string `mapstructure:"name"`
	Environment string `mapstructure:"environment"` // development, production
	LogLevel    string `mapstructure:"log_level"`   // debug, info, warn, error
	Name        string    `mapstructure:"name"`
	Environment string    `mapstructure:"environment"` // development, production
	LogLevel    string    `mapstructure:"log_level"`   // debug, info, warn, error
	IDX         IDXConfig `mapstructure:"idx"`
	Name        string    `mapstructure:"name"`
	Environment string    `mapstructure:"environment"` // development, production
	LogLevel    string    `mapstructure:"log_level"`   // debug, info, warn, error
	IDX         IDXConfig `mapstructure:"idx"`
	Name        string         `mapstructure:"name"`
	Environment string         `mapstructure:"environment"` // development, production
	LogLevel    slog.Level     `mapstructure:"log_level"`   // debug, info, warn, error
	IDX         IDXConfig      `mapstructure:"idx"`
	Stockbit    StockbitConfig `mapstructure:"stockbit"`
}

type StockbitConfig struct {
	Token   string `mapstructure:"token"`
	BaseURL string `mapstructure:"base_url"`
	Name        string `mapstructure:"name"`
	Environment string `mapstructure:"environment"` // development, production
	LogLevel    string `mapstructure:"log_level"`   // debug, info, warn, error
	Name        string     `mapstructure:"name"`
	Environment string     `mapstructure:"environment"` // development, production
	LogDir      string     `mapstructure:"log_dir"`
	LogLevel    slog.Level `mapstructure:"log_level"` // debug, info, warn, error
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (d DatabaseConfig) DSN() string {
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

type MinIOConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	UseSSL    bool   `mapstructure:"use_ssl"`
}

type IDXConfig struct {
	BaseURL  string `mapstructure:"base_url"`
	PageSize int    `mapstructure:"page_size"`
}

type SchedulerConfig struct {
	Interval time.Duration `mapstructure:"interval"`
	CronExpr string        `mapstructure:"cron_expr"` // e.g., "*/15 * * * *"
}

type TelemetryConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
	ServiceName  string `mapstructure:"service_name"`
}

// Load reads configuration from file and environment variables
func Load(configPath string) (Config, error) {
	v := viper.New()

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

	v.SetDefault("app.idx.base_url", "https://idx.co.id/primary/ListedCompany/GetAnnouncement")
	v.SetDefault("app.idx.page_size", 10)
	v.SetDefault("app.stockbit.token", "")
	v.SetDefault("idx.base_url", "https://idx.co.id/primary/ListedCompany/GetAnnouncement")
	v.SetDefault("idx.page_size", 10)

	v.SetDefault("scheduler.interval", 15*time.Minute)
	v.SetDefault("scheduler.cron_expr", "*/15 * * * *")

	v.SetDefault("telemetry.enabled", true)
	v.SetDefault("telemetry.service_name", "bloodhound")

	// Config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
	}

	// Environment variables
	v.SetEnvPrefix("IDX")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return Config{}, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found is OK, we use defaults + env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}
