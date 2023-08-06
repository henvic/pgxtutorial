package telemetry

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
)

func meterProvider(t testing.TB) metric.MeterProvider {
	metricsExp, err := stdoutmetric.New(
		stdoutmetric.WithEncoder(json.NewEncoder(os.Stdout)),
	)
	if err != nil {
		t.Errorf("Error creating stdout exporter: %v", err)
	}
	return sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricsExp)))
}

func TestNewProvider(t *testing.T) {
	logger := slog.Default()
	tracer := trace.NewNoopTracerProvider()
	meter := meterProvider(t)
	propagator := propagation.NewCompositeTextMapPropagator()

	// Test creating a new provider
	provider := NewProvider(logger, tracer.Tracer("example"), meter.Meter("example"), propagator)

	// Test Logger method
	if provider.Logger() != logger {
		t.Errorf("Expected Logger() to return the correct logger")
	}

	// Test Tracer method
	if provider.Tracer() != tracer.Tracer("example") {
		t.Errorf("Expected Tracer() to return the correct tracer")
	}

	// Test Meter method
	if provider.Meter() != meter.Meter("example") {
		t.Errorf("Expected Meter() to return the correct meter")
	}

	// Test Propagator method
	if provider.Propagator() == nil {
		t.Errorf("Expected Propagator() to be set correctly")
	}
}
