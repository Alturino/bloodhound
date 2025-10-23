package otelutil

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var Tracer trace.Tracer = otel.Tracer("bloodhound")
