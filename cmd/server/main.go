package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108: pprof intentionally exposed for dev profiling
	"os"
	"os/signal"
	"syscall"
	"time"

	orderpb "github.com/AlexPips/order-engine/gen/proto/order/v1"
	"github.com/AlexPips/order-engine/internal/config"
	"github.com/AlexPips/order-engine/internal/db"
	"github.com/AlexPips/order-engine/internal/events"
	"github.com/AlexPips/order-engine/internal/interceptors"
	"github.com/AlexPips/order-engine/internal/matching"
	"github.com/AlexPips/order-engine/internal/repository"
	"github.com/AlexPips/order-engine/internal/server"
	"github.com/AlexPips/order-engine/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to database")

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("failed to listen", "port", cfg.GRPCPort, "error", err)
		pool.Close()
		os.Exit(1)
	}

	tp, err := telemetry.InitTracerProvider(ctx, "order-engine", "0.1.0")
	if err != nil {
		slog.Warn("OTel tracer init failed (tracing disabled)", "error", err)
	} else {
		slog.Info("OTel tracer provider initialized")
	}

	engine := matching.New()
	bus := events.New()
	queries := repository.New(pool)
	metrics := telemetry.NewMetrics()
	health := telemetry.NewHealth(pool)
	srv := server.NewOrderService(engine, bus, queries, pool, metrics)

	if err := srv.RecoverState(ctx); err != nil {
		slog.Error("state recovery failed", "error", err)
	} else {
		slog.Info("state recovery complete")
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			interceptors.RecoveryUnary(),
			interceptors.RequestIDUnary(),
			interceptors.LoggingUnary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.RecoveryStream(),
			interceptors.RequestIDStream(),
			interceptors.LoggingStream(),
		),
	)
	orderpb.RegisterOrderServiceServer(grpcServer, srv)
	reflection.Register(grpcServer)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Metrics + health + pprof on :6060
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", health.LivenessHandler)
	mux.HandleFunc("/ready", health.ReadinessHandler)

	metricsServer := &http.Server{Addr: ":6060", Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		slog.Info("metrics+health listening", "addr", ":6060")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	// Update book depth metrics every 5 seconds
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for _, sym := range engine.BookSymbols() {
					metrics.BookDepth.WithLabelValues(sym).Set(float64(engine.BookDepth(sym)))
				}
			case <-stop:
				return
			}
		}
	}()

	go func() {
		slog.Info("server listening", "addr", lis.Addr())
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("server error", "error", err)
		}
	}()

	<-stop
	slog.Info("shutting down")
	grpcServer.GracefulStop()
	metricsServer.Close()
	pool.Close()
	if tp != nil {
		telemetry.ShutdownTracerProvider(context.Background(), tp)
	}
}
