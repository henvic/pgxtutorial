package telemetrytest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"

	"github.com/henvic/pgxtutorial/internal/telemetry"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Discard all telemetry.
func Discard() *telemetry.Provider {
	tracer := tracenoop.NewTracerProvider().Tracer("discard")
	meter := metricnoop.NewMeterProvider().Meter("discard")
	propagator := propagation.NewCompositeTextMapPropagator()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return telemetry.NewProvider(logger, tracer, meter, propagator)
}

// Record of the telemetry.
type Memory struct {
	trace *tracetest.InMemoryExporter
	meter bytes.Buffer
	log   bytes.Buffer
	mr    *sdkmetric.PeriodicReader
}

// Reset the recorded telemetry.
func (mem *Memory) Reset() {
	mem.trace.Reset()
	mem.meter.Reset()
	mem.log.Reset()
}

// Trace returns the recorded spans.
func (mem *Memory) Trace() []sdktrace.ReadOnlySpan {
	return mem.trace.GetSpans().Snapshots()
}

// Meter of the telemetry.
func (mem *Memory) Meter() string {
	if err := mem.mr.ForceFlush(context.Background()); err != nil {
		panic(err)
	}
	return mem.meter.String()
}

// Log of the telemetry.
func (mem *Memory) Log() string {
	return mem.log.String()
}

// Provider for telemetrytest.
func Provider() (provider *telemetry.Provider, mem *Memory) {
	mem = &Memory{}
	mem.trace = tracetest.NewInMemoryExporter()
	logger := slog.New(slog.NewJSONHandler(&mem.log, nil))
	mt, err := stdoutmetric.New(stdoutmetric.WithEncoder(json.NewEncoder(&mem.meter)))
	if err != nil {
		panic(err)
	}
	mem.mr = sdkmetric.NewPeriodicReader(mt)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.Default()),
		sdktrace.WithSyncer(mem.trace),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(resource.Default()),
		sdkmetric.WithReader(mem.mr),
	)
	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	return telemetry.NewProvider(logger, tp.Tracer("tracer"), mp.Meter("meter"), propagator), mem
}
