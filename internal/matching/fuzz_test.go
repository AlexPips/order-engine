package matching

import (
	"context"
	"testing"

	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/shopspring/decimal"
)

func FuzzMatchLimit(f *testing.F) {
	f.Add("BTCUSD", "BUY", "50000.00", "1.0", "SELL", "50000.00", "0.5")
	f.Add("BTCUSD", "BUY", "50000.00", "1.0", "SELL", "49999.99", "0.5")
	f.Add("BTCUSD", "SELL", "50000.00", "1.0", "BUY", "50000.00", "0.5")
	f.Add("ETHUSD", "BUY", "3000.00", "10.0", "SELL", "3000.00", "5.0")

	f.Fuzz(func(t *testing.T, symbol, sideStr, priceStr, qtyStr, cSideStr, cPriceStr, cQtyStr string) {
		side := domain.SideBuy
		if sideStr == "SELL" {
			side = domain.SideSell
		}

		cSide := domain.SideSell
		if cSideStr == "BUY" {
			cSide = domain.SideBuy
		}

		price, err := decimal.NewFromString(priceStr)
		if err != nil || !price.IsPositive() {
			return
		}
		qty, err := decimal.NewFromString(qtyStr)
		if err != nil || !qty.IsPositive() {
			return
		}
		cPrice, err := decimal.NewFromString(cPriceStr)
		if err != nil || !cPrice.IsPositive() {
			return
		}
		cQty, err := decimal.NewFromString(cQtyStr)
		if err != nil || !cQty.IsPositive() {
			return
		}

		engine := New()
		ctx := context.Background()

		resting := &domain.Order{
			ID:       "resting-1",
			UserID:   "user-1",
			Symbol:   symbol,
			Side:     cSide,
			Type:     domain.OrderTypeLimit,
			Price:    cPrice,
			Quantity: cQty,
			Status:   domain.OrderStatusNew,
		}
		if _, err := engine.SubmitOrder(ctx, resting); err != nil {
			return
		}

		incoming := &domain.Order{
			ID:       "incoming-1",
			UserID:   "user-2",
			Symbol:   symbol,
			Side:     side,
			Type:     domain.OrderTypeLimit,
			Price:    price,
			Quantity: qty,
			Status:   domain.OrderStatusNew,
		}
		trades, err := engine.SubmitOrder(ctx, incoming)
		if err != nil {
			return
		}

		for _, tr := range trades {
			if tr.Quantity.LessThanOrEqual(decimal.Zero) {
				t.Fatalf("trade quantity must be positive: %s", tr.Quantity)
			}
			if tr.Price.LessThanOrEqual(decimal.Zero) {
				t.Fatalf("trade price must be positive: %s", tr.Price)
			}
			if tr.BuyOrderID == "" || tr.SellOrderID == "" {
				t.Fatalf("trade must have both order IDs")
			}
		}
	})
}

func FuzzMatchMarket(f *testing.F) {
	f.Add("BTCUSD", "BUY", "1.0", "SELL", "50000.00", "0.5")
	f.Add("BTCUSD", "SELL", "1.0", "BUY", "50000.00", "0.5")

	f.Fuzz(func(t *testing.T, symbol, sideStr, qtyStr, cSideStr, cPriceStr, cQtyStr string) {
		side := domain.SideBuy
		if sideStr == "SELL" {
			side = domain.SideSell
		}

		cSide := domain.SideSell
		if cSideStr == "BUY" {
			cSide = domain.SideBuy
		}

		qty, err := decimal.NewFromString(qtyStr)
		if err != nil || !qty.IsPositive() {
			return
		}
		cPrice, err := decimal.NewFromString(cPriceStr)
		if err != nil || !cPrice.IsPositive() {
			return
		}
		cQty, err := decimal.NewFromString(cQtyStr)
		if err != nil || !cQty.IsPositive() {
			return
		}

		engine := New()
		ctx := context.Background()

		resting := &domain.Order{
			ID:       "resting-1",
			UserID:   "user-1",
			Symbol:   symbol,
			Side:     cSide,
			Type:     domain.OrderTypeLimit,
			Price:    cPrice,
			Quantity: cQty,
			Status:   domain.OrderStatusNew,
		}
		if _, err := engine.SubmitOrder(ctx, resting); err != nil {
			return
		}

		incoming := &domain.Order{
			ID:       "incoming-1",
			UserID:   "user-2",
			Symbol:   symbol,
			Side:     side,
			Type:     domain.OrderTypeMarket,
			Quantity: qty,
			Status:   domain.OrderStatusNew,
		}
		trades, err := engine.SubmitOrder(ctx, incoming)
		if err != nil {
			return
		}

		for _, tr := range trades {
			if tr.Quantity.LessThanOrEqual(decimal.Zero) {
				t.Fatalf("trade quantity must be positive: %s", tr.Quantity)
			}
			if tr.Price.LessThanOrEqual(decimal.Zero) {
				t.Fatalf("trade price must be positive: %s", tr.Price)
			}
		}
	})
}
