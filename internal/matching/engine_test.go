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

func TestGetOrderBookEmpty(t *testing.T) {
	eng := New()
	snap := eng.GetOrderBook("BTCUSD")

	if len(snap.Bids) != 0 {
		t.Fatalf("expected 0 bids, got %d", len(snap.Bids))
	}
	if len(snap.Asks) != 0 {
		t.Fatalf("expected 0 asks, got %d", len(snap.Asks))
	}
}

func TestGetOrderBookSorted(t *testing.T) {
	eng := New()
	ctx := context.Background()

	for i, price := range []float64{52000, 50000, 51000} {
		o := domain.Order{
			ID: domain.OrderID("sell-" + string(rune('a'+i))), UserID: "u1",
			Symbol: "BTCUSD", Side: domain.SideSell, Type: domain.OrderTypeLimit,
			Price:    decimal.NewFromFloat(price),
			Quantity: decimal.NewFromFloat(1),
			Status:   domain.OrderStatusNew,
		}
		if _, err := eng.SubmitOrder(ctx, &o); err != nil {
			t.Fatal(err)
		}
	}

	for i, price := range []float64{48000, 49000, 47000} {
		o := domain.Order{
			ID: domain.OrderID("buy-" + string(rune('a'+i))), UserID: "u2",
			Symbol: "BTCUSD", Side: domain.SideBuy, Type: domain.OrderTypeLimit,
			Price:    decimal.NewFromFloat(price),
			Quantity: decimal.NewFromFloat(1),
			Status:   domain.OrderStatusNew,
		}
		if _, err := eng.SubmitOrder(ctx, &o); err != nil {
			t.Fatal(err)
		}
	}

	snap := eng.GetOrderBook("BTCUSD")

	if len(snap.Bids) != 3 {
		t.Fatalf("expected 3 bid levels, got %d", len(snap.Bids))
	}
	for i := 0; i < len(snap.Bids)-1; i++ {
		if snap.Bids[i].Price.LessThan(snap.Bids[i+1].Price) {
			t.Errorf("bids not sorted high→low: %s < %s", snap.Bids[i].Price, snap.Bids[i+1].Price)
		}
	}

	if len(snap.Asks) != 3 {
		t.Fatalf("expected 3 ask levels, got %d", len(snap.Asks))
	}
	for i := 0; i < len(snap.Asks)-1; i++ {
		if snap.Asks[i].Price.GreaterThan(snap.Asks[i+1].Price) {
			t.Errorf("asks not sorted low→high: %s > %s", snap.Asks[i].Price, snap.Asks[i+1].Price)
		}
	}
}

func TestGetOrderBookMultipleOrdersSameLevel(t *testing.T) {
	eng := New()
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		o := domain.Order{
			ID: domain.OrderID("buy-" + string(rune('a'+i))), UserID: "u2",
			Symbol: "BTCUSD", Side: domain.SideBuy, Type: domain.OrderTypeLimit,
			Price:    decimal.NewFromFloat(50000),
			Quantity: decimal.NewFromFloat(1),
			Status:   domain.OrderStatusNew,
		}
		if _, err := eng.SubmitOrder(ctx, &o); err != nil {
			t.Fatal(err)
		}
	}

	snap := eng.GetOrderBook("BTCUSD")

	if len(snap.Bids) != 1 {
		t.Fatalf("expected 1 bid level, got %d", len(snap.Bids))
	}
	if snap.Bids[0].OrderCount != 2 {
		t.Errorf("expected order_count 2, got %d", snap.Bids[0].OrderCount)
	}
	if snap.Bids[0].Quantity.String() != "2" {
		t.Errorf("expected quantity 2, got %s", snap.Bids[0].Quantity)
	}
}

func TestSelfTradePrevention(t *testing.T) {
	eng := New()
	ctx := context.Background()

	sell := domain.Order{
		ID: "sell-1", UserID: "trader-1", Symbol: "BTCUSD",
		Side: domain.SideSell, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromInt(50000), Quantity: decimal.NewFromInt(1),
		Status: domain.OrderStatusNew,
	}
	if _, err := eng.SubmitOrder(ctx, &sell); err != nil {
		t.Fatal(err)
	}

	buy := domain.Order{
		ID: "buy-1", UserID: "trader-1", Symbol: "BTCUSD",
		Side: domain.SideBuy, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromInt(50000), Quantity: decimal.NewFromInt(1),
		Status: domain.OrderStatusNew,
	}
	trades, err := eng.SubmitOrder(ctx, &buy)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 0 {
		t.Fatalf("expected 0 trades (self-trade prevention), got %d", len(trades))
	}

	snap := eng.GetOrderBook("BTCUSD")
	if len(snap.Bids) != 1 {
		t.Fatalf("expected 1 bid level (buy order rested), got %d", len(snap.Bids))
	}
	if len(snap.Asks) != 1 {
		t.Fatalf("expected 1 ask level (sell order still resting), got %d", len(snap.Asks))
	}
}

func TestSelfTradePreventionMixedUsers(t *testing.T) {
	eng := New()
	ctx := context.Background()

	sell := domain.Order{
		ID: "sell-1", UserID: "trader-1", Symbol: "ETHUSD",
		Side: domain.SideSell, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromInt(3000), Quantity: decimal.NewFromInt(2),
		Status: domain.OrderStatusNew,
	}
	if _, err := eng.SubmitOrder(ctx, &sell); err != nil {
		t.Fatal(err)
	}

	buy := domain.Order{
		ID: "buy-1", UserID: "trader-2", Symbol: "ETHUSD",
		Side: domain.SideBuy, Type: domain.OrderTypeLimit,
		Price: decimal.NewFromInt(3000), Quantity: decimal.NewFromInt(1),
		Status: domain.OrderStatusNew,
	}
	trades, err := eng.SubmitOrder(ctx, &buy)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade (different users), got %d", len(trades))
	}
	if trades[0].Quantity.String() != "1" {
		t.Errorf("expected fill qty 1, got %s", trades[0].Quantity)
	}
}

func TestMarketOrderSlippageProtection(t *testing.T) {
	eng := New()
	ctx := context.Background()

	prices := []int64{100, 200, 300, 400, 500}
	for i, p := range prices {
		o := domain.Order{
			ID: domain.OrderID("sell-" + string(rune('a'+i))), UserID: "maker",
			Symbol: "BTCUSD", Side: domain.SideSell, Type: domain.OrderTypeLimit,
			Price: decimal.NewFromInt(p), Quantity: decimal.NewFromInt(1),
			Status: domain.OrderStatusNew,
		}
		if _, err := eng.SubmitOrder(ctx, &o); err != nil {
			t.Fatal(err)
		}
	}

	marketBuy := domain.Order{
		ID: "market-buy-1", UserID: "taker", Symbol: "BTCUSD",
		Side: domain.SideBuy, Type: domain.OrderTypeMarket,
		Quantity: decimal.NewFromInt(10), Status: domain.OrderStatusNew,
		MaxSlippageBPS: 1500,
	}
	trades, err := eng.SubmitOrder(ctx, &marketBuy)
	if err != nil {
		t.Fatal(err)
	}

	totalFilled := decimal.Zero
	for _, tr := range trades {
		totalFilled = totalFilled.Add(tr.Quantity)
	}

	if totalFilled.LessThanOrEqual(decimal.Zero) {
		t.Fatal("expected some fills from market order")
	}
	if totalFilled.GreaterThan(decimal.NewFromInt(1)) {
		t.Fatalf("expected slippage to limit fills to <= 1 level, total filled: %s", totalFilled)
	}

	for _, tr := range trades {
		if tr.Price.GreaterThan(decimal.NewFromInt(115)) {
			t.Errorf("trade price %s exceeds slippage limit (100 * 1.15 = 115)", tr.Price)
		}
	}
}

func TestMarketOrderNoSlippageLimit(t *testing.T) {
	eng := New()
	ctx := context.Background()

	prices := []int64{100, 200, 300}
	for i, p := range prices {
		o := domain.Order{
			ID: domain.OrderID("sell-" + string(rune('a'+i))), UserID: "maker",
			Symbol: "BTCUSD", Side: domain.SideSell, Type: domain.OrderTypeLimit,
			Price: decimal.NewFromInt(p), Quantity: decimal.NewFromInt(1),
			Status: domain.OrderStatusNew,
		}
		if _, err := eng.SubmitOrder(ctx, &o); err != nil {
			t.Fatal(err)
		}
	}

	marketBuy := domain.Order{
		ID: "market-buy-1", UserID: "taker", Symbol: "BTCUSD",
		Side: domain.SideBuy, Type: domain.OrderTypeMarket,
		Quantity: decimal.NewFromInt(10), Status: domain.OrderStatusNew,
	}
	trades, err := eng.SubmitOrder(ctx, &marketBuy)
	if err != nil {
		t.Fatal(err)
	}

	totalFilled := decimal.Zero
	for _, tr := range trades {
		totalFilled = totalFilled.Add(tr.Quantity)
	}
	if totalFilled.String() != "3" {
		t.Fatalf("expected all 3 levels filled (no slippage limit), got %s", totalFilled)
	}
}
