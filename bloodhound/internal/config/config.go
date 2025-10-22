package config

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	"github.com/Alturino/bloodhound/internal/common/constants"
)

type Application struct {
	Env  string `mapstructure:"env"  json:"env"`
	Host string `mapstructure:"host" json:"host"`
	Port int    `mapstructure:"port" json:"port"`
}

type Config struct {
	Database    `mapstructure:"db"          json:"db"`
	Application `mapstructure:"application" json:"application"`
	Otel        `mapstructure:"otel"        json:"otel"`
	Nats        `mapstructure:"nats"        json:"nats"`
}

var (
	config Config
	once   sync.Once
)

func Get(c context.Context, filename string) Config {
	logger := zerolog.Ctx(c).With().
		Str(constants.KEY_TAG, "config Get").
		Str("filename", filename).
		Logger()

	dir := filepath.Dir(filename)
	filename = filepath.Base(filename)
	viper.AddConfigPath(dir)
	viper.AddConfigPath("./env/")
	viper.SetConfigName(filename)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	once.Do(func() {
		viper.OnConfigChange(func(in fsnotify.Event) {
			logger.Info().
				Str("filename", in.Name).
				Str("operation", in.Op.String()).
				Msg("config file changed")
			Get(c, in.Name)
			if err := viper.MergeInConfig(); err != nil {
				err = fmt.Errorf("error merging config with error: %w", err)
				logger.Warn().Err(err).Msg(err.Error())
			}
		})
		viper.WatchConfig()
	})

	logger = logger.With().Str(constants.KEY_PROCESS, "reading config").Logger()
	logger.Trace().Msg("reading config")
	err := viper.ReadInConfig()
	if err != nil {
		err = fmt.Errorf("error when reading config with error: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}
	logger.Info().Msg("read config")

	logger = logger.With().Str(constants.KEY_PROCESS, "unmarshaling config").Logger()
	logger.Trace().Msg("unmarshaling config")
	err = viper.Unmarshal(&config)
	if err != nil {
		err = fmt.Errorf("error unmarshaling config with error: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}
	logger.Info().Any(constants.KEY_PROCESS, config).Msg("marshalled config")
	return config
}
