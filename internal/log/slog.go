package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/natefinch/lumberjack"
	slogmulti "github.com/samber/slog-multi"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
)

func Get(config config.AppConfig) *slog.Logger {
	return initLogger(config)()
}

func initLogger(config config.AppConfig) func() *slog.Logger {
	return sync.OnceValue(func() *slog.Logger {
		logfile := &lumberjack.Logger{
			Filename:  filepath.Join(config.LogDir, "bloodhound.log"),
			MaxSize:   1000, // megabytes
			Compress:  true,
			LocalTime: true,
		}
		logDestination := io.MultiWriter(os.Stdout, logfile)

		loglevel := config.LogLevel
		sinkHandlerOption := &slog.HandlerOptions{Level: loglevel, AddSource: true}
		var sinkHandler slog.Handler = slog.NewJSONHandler(logDestination, sinkHandlerOption)
		if config.Environment != "production" {
			loglevel = slog.LevelDebug
		}

		pipe := slogmulti.Pipe(
			slogmulti.NewHandleInlineMiddleware(
				func(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
					span := trace.SpanContextFromContext(ctx)
					if span.IsValid() {
						record.AddAttrs(
							slog.String("trace_id", span.TraceID().String()),
							slog.String("span_id", span.SpanID().String()),
						)
					}
					return next(ctx, record)
				},
			),
		).Handler(sinkHandler).
			WithAttrs([]slog.Attr{slog.String("app", "bloodhound")})

		logger := slog.New(slogmulti.Fanout(pipe))
		slog.SetDefault(logger)

		return logger
	})
}
