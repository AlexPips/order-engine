package matching

import (
	"context"
	"testing"

	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/shopspring/decimal"
)

func TestLimitBuyMatchesSell(t *testing.T) {
	eng := New()
	ctx := context.Background()

	sell := domain.Order{
		ID: "sell-1", UserID: "u1", Symbol: "BTCUSD",
		Side: domain.SideSell, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromFloat(50000), Quantity: decimal.NewFromFloat(1),
		Status: domain.OrderStatusNew,
	}
	buy := domain.Order{
		ID: "buy-1", UserID: "u2", Symbol: "BTCUSD",
		Side: domain.SideBuy, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromFloat(50000), Quantity: decimal.NewFromFloat(1),
		Status: domain.OrderStatusNew,
	}

	// Submit sell first (rests), then buy (matches).
	if _, err := eng.SubmitOrder(ctx, &sell); err != nil {
		t.Fatal(err)
	}
	trades, err := eng.SubmitOrder(ctx, &buy)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Price.String() != "50000" {
		t.Errorf("expected price 50000, got %s", trades[0].Price)
	}
}

func TestPartialFill(t *testing.T) {
	eng := New()
	ctx := context.Background()

	sell := domain.Order{
		ID: "sell-1", UserID: "u1", Symbol: "ETHUSD",
		Side: domain.SideSell, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromFloat(3000), Quantity: decimal.NewFromFloat(2),
		Status: domain.OrderStatusNew,
	}
	buy := domain.Order{
		ID: "buy-1", UserID: "u2", Symbol: "ETHUSD",
		Side: domain.SideBuy, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromFloat(3000), Quantity: decimal.NewFromFloat(1),
		Status: domain.OrderStatusNew,
	}
	if _, err := eng.SubmitOrder(ctx, &sell); err != nil {
		t.Fatal(err)
	}
	trades, err := eng.SubmitOrder(ctx, &buy)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Quantity.String() != "1" {
		t.Errorf("expected fill qty 1, got %s", trades[0].Quantity)
	}
}

func TestNoCrossLimit(t *testing.T) {
	eng := New()
	ctx := context.Background()

	// Sell at 60000, buy at 50000 — prices don't cross.
	sell := domain.Order{
		ID: "sell-1", UserID: "u1", Symbol: "BTCUSD",
		Side: domain.SideSell, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromFloat(60000), Quantity: decimal.NewFromFloat(1),
		Status: domain.OrderStatusNew,
	}
	buy := domain.Order{
		ID: "buy-1", UserID: "u2", Symbol: "BTCUSD",
		Side: domain.SideBuy, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromFloat(50000), Quantity: decimal.NewFromFloat(1),
		Status: domain.OrderStatusNew,
	}
	if _, err := eng.SubmitOrder(ctx, &sell); err != nil {
		t.Fatal(err)
	}
	trades, err := eng.SubmitOrder(ctx, &buy)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 0 {
		t.Fatalf("expected 0 trades (prices don't cross), got %d", len(trades))
	}
}
