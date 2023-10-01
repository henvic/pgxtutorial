package main

import (
	"context"
	_ "expvar" // #nosec G108
	"flag"
	"fmt"

	"net/http"
	_ "net/http/pprof" // #nosec G108
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/felixge/fgprof"
	"github.com/henvic/pgxtutorial/internal/api"
	"github.com/henvic/pgxtutorial/internal/database"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"github.com/henvic/pgxtutorial/internal/postgres"
	"github.com/henvic/pgxtutorial/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/exp/slog"
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

	provider, err := telemetry.NewProvider(p.log, buildInfoTelemetry()...)
	if err != nil {
		p.log.Error("cannot initialize telemetry", slog.Any("error", err))
		os.Exit(1)
	}
	// Setting catch-all global OpenTelemetry providers.
	otel.SetTracerProvider(p.tel.TraceProvider)
	otel.SetMeterProvider(p.tel.MeterProvider)
	otel.SetTextMapPropagator(p.tel.Propagator())

	defer func() {
		if err != nil {
			os.Exit(1)
		}
	}()

	defer provider.Shutdown()

	_, span := otel.Tracer("main").Start(context.Background(), "main")
	defer func() {
		if r := recover(); r != nil {
			span.RecordError(fmt.Errorf("%v", r))
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
	log *slog.Logger
	tel *telemetry.Provider
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
		p.log.Error("cannot get pgx logging level", slog.Any("error", err))
		os.Exit(1)
	}
	pgPool, err := database.NewPGXPool(context.Background(), "", &database.PGXStdLogger{
		Logger: p.log,
	}, pgxLogLevel, p.tel.TraceProvider)
	if err != nil {
		p.log.Error("cannot set pgx pool", slog.Any("error", err))
		os.Exit(1)
	}
	defer pgPool.Close()

	s := &api.Server{
		Inventory:    inventory.NewService(postgres.NewDB(pgPool, p.log)),
		Log:          p.log,
		Telemetry:    p.tel,
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
