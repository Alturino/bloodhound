package config

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
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
	Minio       `mapstructure:"minio"       json:"minio"`
}

var (
	config Config
	once   sync.Once
)

func Get(c context.Context, filename string) (Config, error) {
	logger := log.Logger.With().
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
			lg := logger.With().
				Str(constants.KEY_TAG, "config viper.OnConfigChange").
				Str("filename", in.Name).
				Str("operation", in.Op.String()).
				Logger()

			lg.Info().Msg("config file changed")

			lg.Debug().Msg("merging config")
			if err := viper.MergeInConfig(); err != nil {
				err = fmt.Errorf("failed merging config with error: %w", err)
				lg.Warn().Err(err).Msg(err.Error())
				return
			}
			lg.Debug().Msg("merged config")

			lg.Debug().Msg("unmarshaling config")
			if err := viper.Unmarshal(&config); err != nil {
				err = fmt.Errorf("failed unmarshaling config with error: %w", err)
				lg.Warn().Err(err).Msg(err.Error())
				return
			}
			lg.Debug().Msg("marshalled config")

			lg.Info().Msg("config reloaded successfully")
		})
		viper.WatchConfig()
	})

	logger = logger.With().Str(constants.KEY_PROCESS, "reading config").Logger()
	logger.Debug().Msg("reading config")
	err := viper.ReadInConfig()
	if err != nil {
		err = fmt.Errorf("error when reading config with error: %w", err)
		return Config{}, err
	}
	logger.Debug().Msg("read config")

	logger = logger.With().Str(constants.KEY_PROCESS, "unmarshaling config").Logger()
	logger.Debug().Msg("unmarshaling config")
	err = viper.Unmarshal(&config)
	if err != nil {
		err = fmt.Errorf("error unmarshaling config with error: %w", err)
		return Config{}, err
	}
	logger.Debug().Any(constants.KEY_PROCESS, config).Msg("marshalled config")

	return config, nil
}
