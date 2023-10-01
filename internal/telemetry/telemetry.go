package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc/credentials/insecure"
)

// Provider for telemetry services.
type Provider struct {
	log           *slog.Logger
	TraceProvider trace.TracerProvider
	MeterProvider metric.MeterProvider
	propagator    propagation.TextMapPropagator
	Shutdown      func()
}

// NewProvider creates a new telemetry provider.
func NewProvider(log *slog.Logger, attrs ...attribute.KeyValue) (*Provider, error) {

	provider := &Provider{
		log: log,
	}

	fn, err := provider.telemetry(attrs...)
	if err != nil {
		return nil, err
	}

	provider.Shutdown = fn

	return provider, nil
}

// Logger returns the slog logger.
func (p Provider) Logger() *slog.Logger {
	return p.log
}

// Tracer returns the OpenTelemetry tracer.
func (p Provider) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	return p.TraceProvider.Tracer(name, options...)
}

// Meter returns the OpenTelemetry meter.
func (p Provider) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return p.MeterProvider.Meter(name, opts...)
}

// Propagator returns the OpenTelemetry propagator.
func (p Provider) Propagator() propagation.TextMapPropagator {
	return p.propagator
}

// telemetry initializes OpenTelemetry tracing and metrics providers.
func (p *Provider) telemetry(attributes ...attribute.KeyValue) (halt func(), err error) {
	p.propagator = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	var (
		tr sdktrace.SpanExporter
		mt sdkmetric.Exporter
	)

	// OTEL_EXPORTER can be used to configure whether to use the OpenTelemetry gRPC exporter protocol, stdout, or noop.
	switch exporter, ok := os.LookupEnv("OTEL_EXPORTER"); {
	case exporter == "stdout":
		// Tip: Use stdouttrace.WithPrettyPrint() to print spans in human readable format.
		if tr, err = stdouttrace.New(); err != nil {
			return nil, fmt.Errorf("stdouttrace: %w", err)
		}
		if mt, err = stdoutmetric.New(stdoutmetric.WithEncoder(json.NewEncoder(os.Stdout))); err != nil {
			return nil, fmt.Errorf("stdoutmetric: %w", err)
		}
	case exporter == "otlp":
		if tr, err = otlptracegrpc.New(context.Background(), otlptracegrpc.WithTLSCredentials(insecure.NewCredentials())); err != nil {
			return nil, fmt.Errorf("otlptracegrpc: %w", err)
		}

		if mt, err = otlpmetricgrpc.New(context.Background(), otlpmetricgrpc.WithTLSCredentials(insecure.NewCredentials())); err != nil {
			return nil, fmt.Errorf("otlpmetricgrpc: %w", err)
		}
	case ok:
		p.log.Warn("unknown OTEL_EXPORTER value")
		fallthrough
	default:
		p.TraceProvider = trace.NewNoopTracerProvider()
		p.MeterProvider = noop.NewMeterProvider()
		return func() {}, nil
	}

	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(attributes...),
	)

	if err != nil {
		return nil, fmt.Errorf("cannot initialize tracer resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithResource(res), sdktrace.WithBatcher(tr))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mt)))
	p.TraceProvider = tp
	p.MeterProvider = mp

	// The following function will be called when the graceful shutdown starts.
	return func() {
		haltCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var w sync.WaitGroup
		w.Add(2)
		go func() {
			defer w.Done()
			if err := tp.Shutdown(haltCtx); err != nil {
				p.log.Error("telemetry tracer shutdown", slog.Any("error", err))
			}
		}()
		go func() {
			defer w.Done()
			if err := mp.Shutdown(haltCtx); err != nil {
				p.log.Error("telemetry meter shutdown", slog.Any("error", err))
			}
		}()
		w.Wait()
	}, nil
}
