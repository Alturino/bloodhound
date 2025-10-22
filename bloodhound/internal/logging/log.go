package logging

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"

	"github.com/Alturino/bloodhound/internal/common/constants"
)

var (
	once   sync.Once
	logger zerolog.Logger
)

func Get() zerolog.Logger {
	once.Do(func() {
		zerolog.DurationFieldUnit = time.Microsecond
		zerolog.ErrorFieldName = "error"
		zerolog.ErrorStackFieldName = "stack-trace"
		zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
		zerolog.LevelFieldName = "level"
		zerolog.MessageFieldName = "message"
		zerolog.TimestampFieldName = "timestamp"

		logLevel := zerolog.DebugLevel
		// if config.Env == "development" {
		// 	logLevel = zerolog.TraceLevel
		// }

		dir := filepath.Dir(".")
		// if config.Env == "development" {
		// 	dir = filepath.Dir("/var/log")
		// }
		filename := filepath.Join(dir, "bloodhound.log")
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
			Logger().
			Hook(TraceHook(), BaggageHook())

		logger.Info().
			Str(constants.KEY_TAG, "logging Get").
			Str(constants.KEY_PROCESS, "initiating logging").
			Msg("finish initiating logging")

		zerolog.DefaultContextLogger = &logger
		log.Logger = logger
	})
	return logger
}
