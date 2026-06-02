package main

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	orderpb "github.com/AlexPips/order-engine/gen/order/v1"
	"github.com/AlexPips/order-engine/internal/events"
	"github.com/AlexPips/order-engine/internal/matching"
	"github.com/AlexPips/order-engine/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	port := "50051"
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		slog.Error("failed to listen", "port", port, "error", err)
		os.Exit(1)
	}

	engine := matching.New()
	bus := events.New()
	srv := server.NewOrderService(engine, bus)

	grpcServer := grpc.NewServer()
	orderpb.RegisterOrderServiceServer(grpcServer, srv)
	reflection.Register(grpcServer)

	// Graceful shutdown on SIGINT/SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server listening", "addr", lis.Addr())
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-stop
	slog.Info("shutting down")
	grpcServer.GracefulStop()
}
