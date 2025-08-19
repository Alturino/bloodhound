package logging

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var once sync.Once

func Get(filepath string) zerolog.Logger {
	once.Do(func() {
		zerolog.DurationFieldUnit = time.Microsecond
		zerolog.ErrorFieldName = "error"
		zerolog.ErrorStackFieldName = "stack-trace"
		zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
		zerolog.LevelFieldName = "level"
		zerolog.MessageFieldName = "message"
		zerolog.TimestampFieldName = "timestamp"

		logLevel := zerolog.InfoLevel
		if config.Env == "development" {
			logLevel = zerolog.TraceLevel
		}

		fileWriter := &lumberjack.Logger{
			Filename: filepath,
			Compress: true,
		}
		output := zerolog.MultiLevelWriter(os.Stdout, fileWriter)

		logger = zerolog.New(output).
			Level(logLevel).
			With().
			Timestamp().
			Caller().
			Stack().
			Int("pid", os.Getpid()).
			Int("gid", os.Getgid()).
			Int("uid", os.Getuid()).
			Logger()

		logger.Info().
			Str(constants.KEY_TAG, "InitLogger").
			Str(constants.KEY_PROCESS, "InitLogger").
			Msg("finish initiating logging")
	})
	return logger
}
