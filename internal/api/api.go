package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	sync "sync"
	"time"

	"github.com/henvic/pgxtutorial/internal/inventory"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server for the API.
type Server struct {
	HTTPAddress  string
	GRPCAddress  string
	PprofAddress string

	Inventory *inventory.Service
	Logger    *slog.Logger

	grpc  *grpcServer
	http  *httpServer
	pprof *pprofServer

	stopFn sync.Once
}

// Run starts the HTTP and gRPC servers.
func (s *Server) Run(ctx context.Context) (err error) {
	var ec = make(chan error, 3) // gRPC, HTTP, debug servers
	ctx, cancel := context.WithCancel(ctx)
	s.grpc = &grpcServer{
		inventory: s.Inventory,
		logger:    s.Logger,
	}
	s.http = &httpServer{
		inventory: s.Inventory,
		logger:    s.Logger,
	}
	s.pprof = &pprofServer{
		logger: s.Logger,
	}

	go func() {
		err := s.grpc.Run(ctx, s.GRPCAddress)
		if err != nil {
			err = fmt.Errorf("gRPC server error: %w", err)
		}
		ec <- err
	}()
	go func() {
		err := s.http.Run(ctx, s.HTTPAddress)
		if err != nil {
			err = fmt.Errorf("HTTP server error: %w", err)
		}
		ec <- err
	}()
	go func() {
		err := s.pprof.Run(ctx, s.PprofAddress)
		if err != nil {
			err = fmt.Errorf("pprof server error: %w", err)
		}
		ec <- err
	}()

	// Wait for the services to exit.
	var es []string
	for i := 0; i < cap(ec); i++ {
		if err := <-ec; err != nil {
			es = append(es, err.Error())
			// If one of the services returns by a reason other than parent context canceled,
			// try to gracefully shutdown the other services to shutdown everything,
			// with the goal of replacing this service with a new healthy one.
			// NOTE: It might be a slightly better strategy to announce it as unfit for handling traffic,
			// while leaving the program running for debugging.
			if ctx.Err() == nil {
				s.Shutdown(context.Background())
			}
		}
	}
	if len(es) > 0 {
		err = errors.New(strings.Join(es, ", "))
	}
	cancel()
	return err
}

// Shutdown HTTP and gRPC servers.
func (s *Server) Shutdown(ctx context.Context) {
	// Don't try to start a graceful shutdown multiple times.
	s.stopFn.Do(func() {
		s.http.Shutdown(ctx)
		s.grpc.Shutdown(ctx)
		s.pprof.Shutdown(ctx)
	})
}

type httpServer struct {
	inventory *inventory.Service
	logger    *slog.Logger

	middleware func(http.Handler) http.Handler
	http       *http.Server
}

// Run HTTP server.
func (s *httpServer) Run(ctx context.Context, address string) error {
	handler := NewHTTPServer(s.inventory, s.logger)

	// Inject middleware, if the middleware field is set.
	if s.middleware != nil {
		handler = s.middleware(handler)
	}

	s.http = &http.Server{
		Addr:    address,
		Handler: handler,

		ReadHeaderTimeout: 5 * time.Second, // mitigate risk of Slowloris Attack
	}
	s.logger.Info("HTTP server listening", slog.Any("address", address))
	if err := s.http.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown HTTP server.
func (s *httpServer) Shutdown(ctx context.Context) {
	s.logger.Info("shutting down HTTP server")
	if s.http != nil {
		if err := s.http.Shutdown(ctx); err != nil {
			s.logger.Error("graceful shutdown of HTTP server failed", slog.Any("error", err))
		}
	}
}

type grpcServer struct {
	inventory *inventory.Service
	grpc      *grpc.Server
	logger    *slog.Logger
}

// Run gRPC server.
func (s *grpcServer) Run(ctx context.Context, address string) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.grpc = grpc.NewServer()
	reflection.Register(s.grpc)
	RegisterInventoryServer(s.grpc, &InventoryGRPC{
		Inventory: s.inventory,
	})
	s.logger.Info("gRPC server listening", slog.Any("address", lis.Addr()))
	if err := s.grpc.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}

// Shutdown gRPC server.
func (s *grpcServer) Shutdown(ctx context.Context) {
	s.logger.Info("shutting down gRPC server")
	done := make(chan struct{}, 1)
	go func() {
		if s.grpc != nil {
			s.grpc.GracefulStop()
		}
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-ctx.Done():
		if s.grpc != nil {
			s.grpc.Stop()
		}
		s.logger.Error("graceful shutdown of gRPC server failed")
	}
}

// pprofServer runs an HTTP server exposing pprof endpoints.
type pprofServer struct {
	http   *http.Server
	logger *slog.Logger
}

// Run HTTP pprof server.
func (s *pprofServer) Run(ctx context.Context, address string) error {
	// Use http.DefaultServeMux, rather than defining a custom mux.
	s.http = &http.Server{
		Addr: address,

		ReadHeaderTimeout: 5 * time.Second, // mitigate risk of Slowloris Attack
	}
	s.logger.Info("pprof server listening", slog.Any("address", address))
	if err := s.http.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown HTTP server.
func (s *pprofServer) Shutdown(ctx context.Context) {
	s.logger.Info("shutting down pprof server")
	if s.http != nil {
		if err := s.http.Shutdown(ctx); err != nil {
			s.logger.Error("graceful shutdown of pprof server failed", slog.Any("error", err))
		}
	}
}
