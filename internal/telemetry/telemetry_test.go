package telemetry

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
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

	// Test creating a new provider
	provider, err := NewProvider(logger)
	require.NoError(t, err)
	require.NotNil(t, provider)

	// Test Logger method
	if provider.Logger() != logger {
		t.Errorf("Expected Logger() to return the correct logger")
	}

	// Test Tracer method
	if provider.Tracer("example") != tracer.Tracer("example") {
		t.Errorf("Expected Tracer() to return the correct tracer")
	}

	// Test Meter method
	if provider.Meter("example") != meter.Meter("example") {
		t.Errorf("Expected Meter() to return the correct meter")
	}

	// Test Propagator method
	if provider.Propagator() == nil {
		t.Errorf("Expected Propagator() to be set correctly")
	}
}
