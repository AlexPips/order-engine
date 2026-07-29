package interceptors

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func LoggingUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		slog.Info("grpc.unary",
			"method", info.FullMethod,
			"code", status.Code(err),
			"duration", time.Since(start).String(),
		)
		return resp, err
	}
}

func LoggingStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		slog.Info("grpc.stream",
			"method", info.FullMethod,
			"code", status.Code(err),
			"duration", time.Since(start).String(),
		)
		return err
	}
}

func RecoveryUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc.panic",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

func RecoveryStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc.panic",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}

type requestIDKey struct{}

// RequestIDUnary injects a UUID into the context and logs it.
func RequestIDUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		rid := uuid.NewString()
		ctx = context.WithValue(ctx, requestIDKey{}, rid)
		slog.Info("grpc.request.start",
			"method", info.FullMethod,
			"request_id", rid,
		)
		resp, err := handler(ctx, req)
		slog.Info("grpc.request.end",
			"method", info.FullMethod,
			"request_id", rid,
			"code", status.Code(err),
		)
		return resp, err
	}
}

// RequestIDStream injects a UUID into the stream context.
func RequestIDStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		rid := uuid.NewString()
		wrapped := &requestIDStream{ServerStream: ss, rid: rid}
		slog.Info("grpc.stream.start",
			"method", info.FullMethod,
			"request_id", rid,
		)
		err := handler(srv, wrapped)
		slog.Info("grpc.stream.end",
			"method", info.FullMethod,
			"request_id", rid,
			"code", status.Code(err),
		)
		return err
	}
}

// RequestIDFromContext extracts the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	if rid, ok := ctx.Value(requestIDKey{}).(string); ok {
		return rid
	}
	return ""
}

type requestIDStream struct {
	grpc.ServerStream
	rid string
}

func (s *requestIDStream) Context() context.Context {
	return context.WithValue(s.ServerStream.Context(), requestIDKey{}, s.rid)
}
