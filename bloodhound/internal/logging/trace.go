package logging

import (
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

func TraceHook() zerolog.HookFunc {
	return func(e *zerolog.Event, level zerolog.Level, message string) {
		spanContext := trace.SpanContextFromContext(e.GetCtx())
		if spanContext.IsValid() {
			e.Str("trace_id", spanContext.TraceID().String()).
				Str("span_id", spanContext.SpanID().String())
		}
	}
}
