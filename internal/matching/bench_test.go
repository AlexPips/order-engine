package matching

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/shopspring/decimal"
)

func BenchmarkSubmitOrder(b *testing.B) {
	engine := New()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o := &domain.Order{
			ID:       domain.OrderID(fmt.Sprintf("order-%d", i)),
			UserID:   "user-1",
			Symbol:   "BTCUSD",
			Side:     domain.SideBuy,
			Type:     domain.OrderTypeLimit,
			Price:    decimal.NewFromInt(50000),
			Quantity: decimal.NewFromInt(1),
			Status:   domain.OrderStatusNew,
		}
		engine.SubmitOrder(ctx, o)
	}
}

func BenchmarkSubmitOrderParallel(b *testing.B) {
	engine := New()
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			o := &domain.Order{
				ID:       domain.OrderID(fmt.Sprintf("order-%d", i)),
				UserID:   "user-1",
				Symbol:   "BTCUSD",
				Side:     domain.SideBuy,
				Type:     domain.OrderTypeLimit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromInt(1),
				Status:   domain.OrderStatusNew,
			}
			engine.SubmitOrder(ctx, o)
			i++
		}
	})
}

func BenchmarkMatchingWithTrades(b *testing.B) {
	engine := New()
	ctx := context.Background()

	resting := &domain.Order{
		ID:       "resting-1",
		UserID:   "user-1",
		Symbol:   "BTCUSD",
		Side:     domain.SideSell,
		Type:     domain.OrderTypeLimit,
		Price:    decimal.NewFromInt(50000),
		Quantity: decimal.NewFromInt(1000),
		Status:   domain.OrderStatusNew,
	}
	engine.SubmitOrder(ctx, resting)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o := &domain.Order{
			ID:       domain.OrderID(fmt.Sprintf("order-%d", i)),
			UserID:   "user-2",
			Symbol:   "BTCUSD",
			Side:     domain.SideBuy,
			Type:     domain.OrderTypeLimit,
			Price:    decimal.NewFromInt(50000),
			Quantity: decimal.NewFromInt(1),
			Status:   domain.OrderStatusNew,
		}
		engine.SubmitOrder(ctx, o)
	}
}

func BenchmarkOrderBookSnapshot(b *testing.B) {
	engine := New()
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		o := &domain.Order{
			ID:       domain.OrderID(fmt.Sprintf("order-%d", i)),
			UserID:   "user-1",
			Symbol:   "BTCUSD",
			Side:     domain.SideBuy,
			Type:     domain.OrderTypeLimit,
			Price:    decimal.NewFromInt(int64(50000 - i)),
			Quantity: decimal.NewFromInt(1),
			Status:   domain.OrderStatusNew,
		}
		engine.SubmitOrder(ctx, o)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.GetOrderBook("BTCUSD")
	}
}

func BenchmarkConcurrentSubmit(b *testing.B) {
	engine := New()
	ctx := context.Background()
	var mu sync.Mutex
	counter := 0

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			id := counter
			counter++
			mu.Unlock()

			o := &domain.Order{
				ID:       domain.OrderID(fmt.Sprintf("order-%d", id)),
				UserID:   "user-1",
				Symbol:   "BTCUSD",
				Side:     domain.SideBuy,
				Type:     domain.OrderTypeLimit,
				Price:    decimal.NewFromInt(50000),
				Quantity: decimal.NewFromInt(1),
				Status:   domain.OrderStatusNew,
			}
			engine.SubmitOrder(ctx, o)
		}
	})
}
