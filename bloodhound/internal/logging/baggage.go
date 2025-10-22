package logging

import (
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/baggage"
)

func BaggageHook() zerolog.HookFunc {
	return func(e *zerolog.Event, level zerolog.Level, message string) {
		bg := baggage.FromContext(e.GetCtx())
		for _, member := range bg.Members() {
			e.Str(member.Key(), member.Value())
		}
	}
}
