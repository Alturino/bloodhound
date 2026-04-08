package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/natefinch/lumberjack"
	slogmulti "github.com/samber/slog-multi"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
)

func Get(config *config.App) *slog.Logger {
	logfile := &lumberjack.Logger{
		Filename:  filepath.Join(config.LogDir, "bloodhound.log"),
		MaxSize:   1000, // megabytes
		MaxAge:    30,
		Compress:  true,
		LocalTime: true,
	}
	logDestination := io.MultiWriter(os.Stdout, logfile)

	sinkHandlerOption := &slog.HandlerOptions{
		Level:     config.LogLevelVar,
		AddSource: true,
	}
	var sinkHandler slog.Handler = slog.NewJSONHandler(logDestination, sinkHandlerOption)
	if config.Environment == "development" {
		config.LogLevelVar.Set(slog.LevelDebug)
		logfile.Filename = filepath.Join(config.LogDir, "bloodhound-dev.log")
		sinkHandler = slog.NewTextHandler(logDestination, sinkHandlerOption)
		slog.Debug(
			"logger has been setup for debug",
			slog.String("log_filename", logfile.Filename),
		)
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

	// otelSlog := otelslog.NewHandler(common.APPLICATION_NAME, otelslog.WithSource(true)).
	// 	WithAttrs([]slog.Attr{slog.String("app", "leaderboard")})

	// logger = slog.New(slogmulti.Fanout(otelSlog, pipe))

	logger := slog.New(pipe)
	slog.SetDefault(logger)

	return logger
}
