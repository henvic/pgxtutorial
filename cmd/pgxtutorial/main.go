package main

import (
	"context"
	_ "expvar" // #nosec G108
	"flag"
	"fmt"
	"log/slog"
	_ "net/http/pprof" // #nosec G108
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/henvic/pgxtutorial/internal/api"
	"github.com/henvic/pgxtutorial/internal/database"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"github.com/henvic/pgxtutorial/internal/postgres"
)

var (
	httpAddr  = flag.String("http", "localhost:8080", "HTTP service address to listen for incoming requests on")
	grpcAddr  = flag.String("grpc", "localhost:8082", "gRPC service address to listen for incoming requests on")
	probeAddr = flag.String("probe", "localhost:6060", "probe (inspection) HTTP service address")
	version   = flag.Bool("version", false, "Print build info")

	buildInfo, _ = debug.ReadBuildInfo()
)

func main() {
	flag.Parse()
	if *version {
		fmt.Println(buildInfo)
		os.Exit(2)
	}

	p := program{
		log: slog.Default(),
	}

	if err := p.run(); err != nil {
		p.log.Error("application terminated by error", slog.Any("error", err))
		os.Exit(1)
	}
}

type program struct {
	log *slog.Logger
}

func (p *program) run() error {
	pgxLogLevel, err := database.LogLevelFromEnv()
	if err != nil {
		return fmt.Errorf("cannot get pgx logging level: %w", err)
	}
	pgPool, err := database.NewPGXPool(context.Background(), "", &database.PGXStdLogger{
		Logger: p.log,
	}, pgxLogLevel)
	if err != nil {
		return fmt.Errorf("cannot create pgx pool: %w", err)
	}
	defer pgPool.Close()

	s := &api.Server{
		Inventory:    inventory.NewService(postgres.NewDB(pgPool, p.log)),
		Log:          p.log,
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
