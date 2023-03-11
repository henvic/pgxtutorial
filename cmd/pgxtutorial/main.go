package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	httpAddr = flag.String("http", "localhost:8080", "HTTP service address to listen for incoming requests on")
	grpcAddr = flag.String("grpc", "localhost:8082", "gRPC service address to listen for incoming requests on")
	version  = flag.Bool("version", false, "Print build info")
)

func main() {
	flag.Parse()
	if *version {
		info, _ := debug.ReadBuildInfo()
		fmt.Println(info)
		os.Exit(2)
	}

	pgxLogLevel, err := database.LogLevelFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	pgPool, err := database.NewPGXPool(context.Background(), "", &database.PGXStdLogger{}, pgxLogLevel)
	if err != nil {
		log.Fatal(err)
	}
	defer pgPool.Close()
	s := &api.Server{
		Inventory:   inventory.NewService(postgres.NewDB(pgPool)),
		HTTPAddress: *httpAddr,
		GRPCAddress: *grpcAddr,
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
		log.Fatal(err)
	}
}
