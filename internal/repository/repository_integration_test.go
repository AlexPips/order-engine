//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/AlexPips/order-engine/internal/db"
	"github.com/shopspring/decimal"
)

func setupRepo(t *testing.T) (*Queries, func()) {
	t.Helper()

	dbURL, cleanup := db.SetupTestDB(t)

	pool, err := db.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}

	return New(pool), func() {
		pool.Close()
		cleanup()
	}
}

func TestCreateAndGetOrder(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixNano()

	order, err := repo.CreateOrder(ctx, CreateOrderParams{
		ID:        "order-1",
		UserID:    "user-1",
		Symbol:    "BTCUSD",
		Side:      "BUY",
		Type:      "LIMIT",
		Price:     decimal.NewFromFloat(50000),
		Quantity:  decimal.NewFromFloat(1),
		FilledQty: decimal.Zero,
		Status:    "NEW",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	if order.ID != "order-1" {
		t.Errorf("expected order-1, got %s", order.ID)
	}

	fetched, err := repo.GetOrder(ctx, "order-1")
	if err != nil {
		t.Fatal(err)
	}

	if fetched.ID != order.ID {
		t.Errorf("expected %s, got %s", order.ID, fetched.ID)
	}
}

func TestCancelOrder(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixNano()

	_, err := repo.CreateOrder(ctx, CreateOrderParams{
		ID:        "order-2",
		UserID:    "user-1",
		Symbol:    "BTCUSD",
		Side:      "BUY",
		Type:      "LIMIT",
		Price:     decimal.NewFromFloat(50000),
		Quantity:  decimal.NewFromFloat(1),
		FilledQty: decimal.Zero,
		Status:    "NEW",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := repo.CancelOrder(ctx, CancelOrderParams{
		ID:        "order-2",
		UpdatedAt: time.Now().UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if cancelled.Status != "CANCELED" {
		t.Errorf("expected CANCELED, got %s", cancelled.Status)
	}
}

func TestCreateAndGetTrade(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixNano()

	_, err := repo.CreateOrder(ctx, CreateOrderParams{
		ID: "buy-1", UserID: "u1", Symbol: "BTCUSD",
		Side: "BUY", Type: "LIMIT", Price: decimal.NewFromFloat(50000),
		Quantity: decimal.NewFromFloat(1), Status: "FILLED",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.CreateOrder(ctx, CreateOrderParams{
		ID: "sell-1", UserID: "u2", Symbol: "BTCUSD",
		Side: "SELL", Type: "LIMIT", Price: decimal.NewFromFloat(50000),
		Quantity: decimal.NewFromFloat(1), Status: "FILLED",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	trade, err := repo.CreateTrade(ctx, CreateTradeParams{
		ID:          "trade-1",
		Symbol:      "BTCUSD",
		BuyOrderID:  "buy-1",
		SellOrderID: "sell-1",
		Price:       decimal.NewFromFloat(50000),
		Quantity:    decimal.NewFromFloat(1),
		ExecutedAt:  now,
	})
	if err != nil {
		t.Fatal(err)
	}

	if trade.ID != "trade-1" {
		t.Errorf("expected trade-1, got %s", trade.ID)
	}

	trades, err := repo.GetTradesBySymbol(ctx, "BTCUSD")
	if err != nil {
		t.Fatal(err)
	}

	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
}

func TestGetOpenOrdersBySymbol(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixNano()

	for i, status := range []string{"NEW", "PARTIAL", "FILLED"} {
		_, err := repo.CreateOrder(ctx, CreateOrderParams{
			ID:     "order-" + string(rune('a'+i)),
			UserID: "u1", Symbol: "BTCUSD",
			Side: "BUY", Type: "LIMIT", Price: decimal.NewFromFloat(50000),
			Quantity: decimal.NewFromFloat(1), Status: status,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	orders, err := repo.GetOpenOrdersBySymbol(ctx, "BTCUSD")
	if err != nil {
		t.Fatal(err)
	}

	if len(orders) != 2 {
		t.Fatalf("expected 2 open orders, got %d", len(orders))
	}
}
