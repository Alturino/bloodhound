package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/natefinch/lumberjack"
	slogmulti "github.com/samber/slog-multi"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/config"
)

func Get(config *config.App) (*slog.Logger, error) {
	serviceName := config.ServiceName()
	logPath := filepath.Join(config.LogDir, serviceName+".log")
	logfile := &lumberjack.Logger{
		Filename:  logPath,
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
	sinkHandler := slog.NewJSONHandler(logDestination, sinkHandlerOption)
	slogctxHandler := slogctx.NewHandler(sinkHandler, &slogctx.HandlerOptions{})
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
	).Handler(slogctxHandler).
		WithAttrs([]slog.Attr{slog.String("svc_name", serviceName)})

	// otelSlog := otelslog.NewHandler(common.APPLICATION_NAME, otelslog.WithSource(true)).
	// 	WithAttrs([]slog.Attr{slog.String("app", "leaderboard")})

	// logger = slog.New(slogmulti.Fanout(otelSlog, pipe))

	logger := slog.New(pipe)
	slog.SetDefault(logger)

	return logger, nil
}
