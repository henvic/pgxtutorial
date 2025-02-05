package main

import (
	"context"
	"encoding/json"
	_ "expvar" // #nosec G108
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // #nosec G108
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/felixge/fgprof"
	"github.com/henvic/pgxtutorial/internal/api"
	"github.com/henvic/pgxtutorial/internal/database"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"github.com/henvic/pgxtutorial/internal/postgres"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/automaxprocs/maxprocs"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	httpAddr  = flag.String("http", "localhost:8080", "HTTP service address to listen for incoming requests on")
	grpcAddr  = flag.String("grpc", "localhost:8082", "gRPC service address to listen for incoming requests on")
	probeAddr = flag.String("probe", "localhost:6060", "probe (inspection) HTTP service address")
	version   = flag.Bool("version", false, "Print build info")

	buildInfo, _ = debug.ReadBuildInfo()
)

// buildInfoTelemetry for OpenTelemetry.
func buildInfoTelemetry() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.ServiceName("api"),
		semconv.ServiceVersion("1.0.0"),
		attribute.Key("build.go").String(runtime.Version()),
	}
	for _, s := range buildInfo.Settings {
		switch s.Key {
		case "vcs.revision", "vcs.time":
			attrs = append(attrs, attribute.Key("build."+s.Key).String(s.Value))
		case "vcs.modified":
			attrs = append(attrs, attribute.Key("build.vcs.modified").Bool(s.Value == "true"))
		}
	}
	return attrs
}

func main() {
	flag.Parse()
	if *version {
		fmt.Println(buildInfo)
		os.Exit(2)
	}

	p := program{
		log: slog.Default(),
	}

	haltTelemetry, err := p.telemetry()
	if err != nil {
		p.log.Error("cannot initialize telemetry", slog.Any("error", err))
		os.Exit(1)
	}
	// Setting catch-all global OpenTelemetry providers.
	otel.SetTracerProvider(p.tracer)
	otel.SetTextMapPropagator(p.propagator)
	otel.SetMeterProvider(p.meter)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		p.log.Error("irremediable OpenTelemetry event", slog.Any("error", err))
	}))

	defer func() {
		if err != nil {
			os.Exit(1)
		}
	}()
	defer haltTelemetry()

	_, span := otel.Tracer("main").Start(context.Background(), "main")
	defer func() {
		if r := recover(); r != nil {
			span.RecordError(fmt.Errorf("%v", r),
				trace.WithAttributes(attribute.String("stack_trace", string(debug.Stack()))))
			span.SetStatus(codes.Error, "program killed by a panic")
			span.End()
			panic(r)
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "program exited with error")
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}()

	if err = p.run(); err != nil {
		p.log.Error("application terminated by error", slog.Any("error", err))
	}
}

type program struct {
	log        *slog.Logger
	tracer     trace.TracerProvider
	propagator propagation.TextMapPropagator
	meter      metric.MeterProvider
}

func (p *program) run() error {
	// Set GOMAXPROCS to match Linux container CPU quota on Linux.
	if runtime.GOOS == "linux" {
		if _, err := maxprocs.Set(maxprocs.Logger(p.log.Info)); err != nil {
			p.log.Error("cannot set GOMAXPROCS", slog.Any("error", err))
		}
	}

	// Register fgprof HTTP handler, a sampling Go profiler.
	http.DefaultServeMux.Handle("/debug/fgprof", fgprof.Handler())

	pgxLogLevel, err := database.LogLevelFromEnv()
	if err != nil {
		return fmt.Errorf("cannot get pgx logging level: %w", err)
	}
	pgPool, err := database.NewPGXPool(context.Background(), "", &database.PGXStdLogger{
		Logger: p.log,
	}, pgxLogLevel, p.tracer)
	if err != nil {
		return fmt.Errorf("cannot create pgx pool: %w", err)
	}
	defer pgPool.Close()

	s := &api.Server{
		Inventory:    inventory.NewService(postgres.NewDB(pgPool, p.log)),
		Log:          p.log,
		Tracer:       p.tracer,
		Meter:        p.meter,
		Propagator:   p.propagator,
		HTTPAddress:  *httpAddr,
		GRPCAddress:  *grpcAddr,
		ProbeAddress: *probeAddr,
	}
	ec := make(chan error, 1)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		ec <- s.Run(context.Background())
	}()

	// Waits for an internal error that shutdowns the server.
	// Otherwise, wait for a SIGINT or SIGTERM and tries to shutdown the server gracefully.
	// After a shutdown signal, HTTP requests taking longer than the specified grace period are forcibly closed.
	select {
	case err = <-ec:
	case <-ctx.Done():
		fmt.Println()
		haltCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s.Shutdown(haltCtx)
		stop()
		err = <-ec
	}
	if err != nil {
		return fmt.Errorf("application terminated by error: %w", err)
	}
	return nil
}

// telemetry initializes OpenTelemetry tracing and metrics providers.
func (p *program) telemetry() (halt func(), err error) {
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
		p.tracer = tracenoop.NewTracerProvider()

		p.meter = noop.NewMeterProvider()
		return func() {}, nil
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(buildInfoTelemetry()...))
	if err != nil {
		return nil, fmt.Errorf("cannot initialize tracer resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithResource(res), sdktrace.WithBatcher(tr))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mt)))
	p.tracer = tp
	p.meter = mp

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
