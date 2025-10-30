package otelutil

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var Tracer trace.Tracer = otel.Tracer("bloodhound")

func RecordError(err error, span trace.Span) {
	if err != nil {
		span.AddEvent(err.Error())
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
}
