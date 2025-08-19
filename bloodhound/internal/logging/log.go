package logging

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var (
	once   sync.Once
	logger zerolog.Logger
)

func Get() *zerolog.Logger {
	once.Do(func() {
		zerolog.DurationFieldUnit = time.Microsecond
		zerolog.ErrorFieldName = "error"
		zerolog.ErrorStackFieldName = "stack-trace"
		zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
		zerolog.LevelFieldName = "level"
		zerolog.MessageFieldName = "message"
		zerolog.TimestampFieldName = "timestamp"

		logLevel := zerolog.InfoLevel
		// if config.Env == "development" {
		// 	logLevel = zerolog.TraceLevel
		// }

		filename := filepath.Join("./", "bloodhound.log")
		fileWriter := &lumberjack.Logger{
			Filename: filename,
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
			Str(KEY_TAG, "logging Get").
			Str(KEY_PROCESS, "initiating logging").
			Msg("finish initiating logging")

		zerolog.DefaultContextLogger = &logger
	})
	return &logger
}
