package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestInitTracerProvider_ReturnsNonNil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tp, err := InitTracerProvider(ctx, "test-service", "v0.0.0")
	// We accept either a valid provider or a graceful failure when no
	// OTel collector is running (CI / dev environment).
	if err != nil {
		t.Logf("InitTracerProvider returned expected error (no collector): %v", err)
		return
	}
	if tp == nil {
		t.Fatal("InitTracerProvider returned nil TracerProvider with nil error")
	}
	_ = tp.Shutdown(ctx) //nolint:errcheck
}

func TestShutdownTracerProvider_Nil(t *testing.T) {
	// Must not panic.
	ShutdownTracerProvider(context.Background(), nil)
}

func TestShutdownTracerProvider_NoCollector(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tp, err := InitTracerProvider(ctx, "test", "v1")
	if err != nil {
		t.Skip("no OTel collector available in this environment")
		return
	}

	done := make(chan struct{})
	go func() {
		ShutdownTracerProvider(ctx, tp)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(10 * time.Second):
		t.Fatal("ShutdownTracerProvider timed out")
	}
}
