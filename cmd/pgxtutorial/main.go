package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof" // #nosec G108
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/felixge/fgprof"
	"github.com/henvic/pgxtutorial/internal/api"
	"github.com/henvic/pgxtutorial/internal/database"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"github.com/henvic/pgxtutorial/internal/postgres"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/exp/slog"
)

var (
	httpAddr  = flag.String("http", "localhost:8080", "HTTP service address to listen for incoming requests on")
	grpcAddr  = flag.String("grpc", "localhost:8082", "gRPC service address to listen for incoming requests on")
	pprofAddr = flag.String("pprof", "localhost:6061", "pprof address")
	version   = flag.Bool("version", false, "Print build info")
)

func main() {
	flag.Parse()
	if *version {
		info, _ := debug.ReadBuildInfo()
		fmt.Println(info)
		os.Exit(2)
	}

	logger := slog.Default()

	// Set GOMAXPROCS to match Linux container CPU quota.
	if _, err := maxprocs.Set(maxprocs.Logger(logger.Info)); err != nil {
		logger.Error("cannot set GOMAXPROCS", slog.Any("error", err))
	}

	// Register fgprof HTTP handler, a sampling Go profiler.
	http.DefaultServeMux.Handle("/debug/fgprof", fgprof.Handler())

	pgxLogLevel, err := database.LogLevelFromEnv()
	if err != nil {
		logger.Error("cannot get PGX logging level", slog.Any("error", err))
		os.Exit(1)
	}
	pgPool, err := database.NewPGXPool(context.Background(), "", &database.PGXStdLogger{
		Logger: logger,
	}, pgxLogLevel)
	if err != nil {
		logger.Error("cannot set pgx pool", slog.Any("error", err))
		os.Exit(1)
	}
	defer pgPool.Close()
	s := &api.Server{
		Inventory:    inventory.NewService(postgres.NewDB(pgPool, logger)),
		Logger:       logger,
		HTTPAddress:  *httpAddr,
		GRPCAddress:  *grpcAddr,
		PprofAddress: *pprofAddr,
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
		logger.Error("application terminated by error", slog.Any("error", err))
		os.Exit(1)
	}
}
