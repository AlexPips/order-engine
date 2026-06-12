package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	orderpb "github.com/AlexPips/order-engine/gen/order/v1"
	"github.com/AlexPips/order-engine/internal/config"
	"github.com/AlexPips/order-engine/internal/db"
	"github.com/AlexPips/order-engine/internal/events"
	"github.com/AlexPips/order-engine/internal/matching"
	"github.com/AlexPips/order-engine/internal/repository"
	"github.com/AlexPips/order-engine/internal/server"
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

	engine := matching.New()
	bus := events.New()
	queries := repository.New(pool)
	srv := server.NewOrderService(engine, bus, queries)

	grpcServer := grpc.NewServer()
	orderpb.RegisterOrderServiceServer(grpcServer, srv)
	reflection.Register(grpcServer)

	pprofServer := &http.Server{Addr: ":6060", Handler: nil}
	go func() {
		slog.Info("pprof listening", "addr", ":6060")
		if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("pprof error", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server listening", "addr", lis.Addr())
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("server error", "error", err)
		}
	}()

	<-stop
	slog.Info("shutting down")
	grpcServer.GracefulStop()
	pprofServer.Close()
	pool.Close()
}
