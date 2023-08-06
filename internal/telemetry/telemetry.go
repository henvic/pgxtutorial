package telemetry

import (
	"log/slog"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Provider for telemetry services.
type Provider struct {
	log        *slog.Logger
	tracer     trace.Tracer
	meter      metric.Meter
	propagator propagation.TextMapPropagator
}

// NewProvider creates a new telemetry provider.
func NewProvider(log *slog.Logger, tracer trace.Tracer, meter metric.Meter, propagator propagation.TextMapPropagator) *Provider {
	return &Provider{
		log:        log,
		tracer:     tracer,
		meter:      meter,
		propagator: propagator,
	}
}

// Logger returns the slog logger.
func (p Provider) Logger() *slog.Logger {
	return p.log
}

// Tracer returns the OpenTelemetry tracer.
func (p Provider) Tracer() trace.Tracer {
	return p.tracer
}

// Meter returns the OpenTelemetry meter.
func (p Provider) Meter() metric.Meter {
	return p.meter
}

// Propagator returns the OpenTelemetry propagator.
func (p Provider) Propagator() propagation.TextMapPropagator {
	return p.propagator
}
