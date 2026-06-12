package matching

import (
	"context"
	"fmt"
	"testing"

	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/shopspring/decimal"
)

func prepopulatedBook(b *testing.B, numOrders int) *Engine {
	b.Helper()
	eng := New()
	ctx := context.Background()

	for i := 0; i < numOrders; i++ {
		side := domain.SideSell
		if i%2 == 0 {
			side = domain.SideBuy
		}
		price := 50000.0 + float64(i%100)
		o := &domain.Order{
			ID:       domain.OrderID(fmt.Sprintf("rest-%d", i)),
			UserID:   "bench",
			Symbol:   "BTCUSD",
			Side:     side,
			Type:     domain.OrderTypeLimit,
			Price:    decimal.NewFromFloat(price),
			Quantity: decimal.NewFromFloat(1),
			Status:   domain.OrderStatusNew,
		}
		if _, err := eng.SubmitOrder(ctx, o); err != nil {
			b.Fatal(err)
		}
	}
	return eng
}

func BenchmarkSubmitOrderLimitRest(b *testing.B) {
	eng := prepopulatedBook(b, 1000)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o := &domain.Order{
			ID:       domain.OrderID(fmt.Sprintf("bench-%d", i)),
			UserID:   "bench",
			Symbol:   "BTCUSD",
			Side:     domain.SideBuy,
			Type:     domain.OrderTypeLimit,
			Price:    decimal.NewFromFloat(40000),
			Quantity: decimal.NewFromFloat(0.1),
			Status:   domain.OrderStatusNew,
		}
		eng.SubmitOrder(ctx, o) //nolint:errcheck
	}
}

func BenchmarkSubmitOrderLimitMatch(b *testing.B) {
	eng := prepopulatedBook(b, 1000)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o := &domain.Order{
			ID:       domain.OrderID(fmt.Sprintf("bench-match-%d", i)),
			UserID:   "bench",
			Symbol:   "BTCUSD",
			Side:     domain.SideBuy,
			Type:     domain.OrderTypeLimit,
			Price:    decimal.NewFromFloat(60000),
			Quantity: decimal.NewFromFloat(0.1),
			Status:   domain.OrderStatusNew,
		}
		eng.SubmitOrder(ctx, o) //nolint:errcheck
	}
}

func BenchmarkSubmitOrderMarket(b *testing.B) {
	eng := prepopulatedBook(b, 1000)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o := &domain.Order{
			ID:       domain.OrderID(fmt.Sprintf("bench-mkt-%d", i)),
			UserID:   "bench",
			Symbol:   "BTCUSD",
			Side:     domain.SideBuy,
			Type:     domain.OrderTypeMarket,
			Quantity: decimal.NewFromFloat(0.1),
			Status:   domain.OrderStatusNew,
		}
		eng.SubmitOrder(ctx, o) //nolint:errcheck
	}
}

func BenchmarkGetOrderBook(b *testing.B) {
	eng := prepopulatedBook(b, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.GetOrderBook("BTCUSD")
	}
}
