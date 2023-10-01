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
	"github.com/henvic/pgxtutorial/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server for the API.
type Server struct {
	HTTPAddress  string
	GRPCAddress  string
	ProbeAddress string
	Log          *slog.Logger
	Inventory    *inventory.Service
	grpc         *grpcServer
	http         *httpServer
	probe        *probeServer
	Telemetry    *telemetry.Provider
	stopFn       sync.Once
}

// Run starts the HTTP and gRPC servers.
func (s *Server) Run(ctx context.Context) (err error) {
	var ec = make(chan error, 3) // gRPC, HTTP, debug servers
	ctx, cancel := context.WithCancel(ctx)

	// s.Telemetry.Tracer("api")
	// s.Telemetry.Meter("api")

	s.grpc = &grpcServer{
		inventory: s.Inventory,
		tel:       s.Telemetry,
	}
	s.http = &httpServer{
		inventory: s.Inventory,
		tel:       s.Telemetry,
	}
	s.probe = &probeServer{
		tel: s.Telemetry,
	}

	go func() {
		err := s.grpc.Run(
			ctx,
			s.GRPCAddress,
			otelgrpc.WithMeterProvider(
				s.Telemetry.MeterProvider,
			),
			otelgrpc.WithTracerProvider(s.Telemetry.TraceProvider),
			otelgrpc.WithPropagators(s.Telemetry.Propagator()),
		)

		if err != nil {
			err = fmt.Errorf("gRPC server error: %w", err)
		}

		ec <- err
	}()
	go func() {
		err := s.http.Run(
			ctx,
			s.HTTPAddress,
			otelhttp.WithMeterProvider(s.Telemetry.MeterProvider),
			otelhttp.WithTracerProvider(s.Telemetry.TraceProvider),
			otelhttp.WithPropagators(s.Telemetry.Propagator()),
		)

		if err != nil {
			err = fmt.Errorf("HTTP server error: %w", err)
		}

		ec <- err
	}()
	go func() {
		err := s.probe.Run(ctx, s.ProbeAddress)
		if err != nil {
			err = fmt.Errorf("probe server error: %w", err)
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
		s.probe.Shutdown(ctx)
	})
}

type httpServer struct {
	inventory *inventory.Service
	tel       *telemetry.Provider

	middleware func(http.Handler) http.Handler
	http       *http.Server
}

// Run HTTP server.
func (s *httpServer) Run(ctx context.Context, address string, otelOptions ...otelhttp.Option) error {
	handler := NewHTTPServerAPI(s.inventory, s.tel)

	// Inject middleware, if the middleware field is set.
	if s.middleware != nil {
		handler = s.middleware(handler)
	}

	s.http = &http.Server{
		Addr:    address,
		Handler: otelhttp.NewHandler(handler, "api", otelOptions...),

		ReadHeaderTimeout: 5 * time.Second, // mitigate risk of Slowloris Attack
	}
	s.tel.Logger().Info("HTTP server listening", slog.Any("address", address))
	if err := s.http.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown HTTP server.
func (s *httpServer) Shutdown(ctx context.Context) {
	s.tel.Logger().Info("shutting down HTTP server")
	if s.http != nil {
		if err := s.http.Shutdown(ctx); err != nil {
			s.tel.Logger().Error("graceful shutdown of HTTP server failed", slog.Any("error", err))
		}
	}
}

type grpcServer struct {
	inventory *inventory.Service
	grpc      *grpc.Server
	tel       *telemetry.Provider
}

// Run gRPC server.
func (s *grpcServer) Run(ctx context.Context, address string, oo ...otelgrpc.Option) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.grpc = grpc.NewServer(
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor(oo...)),
		grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor(oo...)),
	)
	reflection.Register(s.grpc)
	RegisterInventoryServer(s.grpc, &InventoryGRPC{
		Inventory: s.inventory,
	})
	s.tel.Logger().Info("gRPC server listening", slog.Any("address", lis.Addr()))
	if err := s.grpc.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}

// Shutdown gRPC server.
func (s *grpcServer) Shutdown(ctx context.Context) {
	s.tel.Logger().Info("shutting down gRPC server")
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
		s.tel.Logger().Error("graceful shutdown of gRPC server failed")
	}
}

// probeServer runs an HTTP server exposing pprof endpoints.
type probeServer struct {
	http *http.Server
	tel  *telemetry.Provider
}

// Run HTTP pprof server.
func (s *probeServer) Run(ctx context.Context, address string) error {
	// Use http.DefaultServeMux, rather than defining a custom mux.
	s.http = &http.Server{
		Addr: address,

		ReadHeaderTimeout: 5 * time.Second, // mitigate risk of Slowloris Attack
	}
	s.tel.Logger().Info("Probe server listening", slog.Any("address", address))
	if err := s.http.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown HTTP server.
func (s *probeServer) Shutdown(ctx context.Context) {
	s.tel.Logger().Info("shutting down pprof server")
	if s.http != nil {
		if err := s.http.Shutdown(ctx); err != nil {
			s.tel.Logger().Error("graceful shutdown of pprof server failed", slog.Any("error", err))
		}
	}
}
