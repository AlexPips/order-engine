package matching

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var ErrInsufficientLiquidity = errors.New("no matching orders available")

type Engine struct {
	books map[string]*OrderBook
}

func New() *Engine {
	return &Engine{books: make(map[string]*OrderBook)}
}

func (e *Engine) SubmitOrder(ctx context.Context, o *domain.Order) ([]domain.Trade, error) {
	book := e.getOrCreateBook(o.Symbol)
	switch o.Type {
	case domain.OrderTypeMarket:
		return e.matchMarket(ctx, book, o)
	case domain.OrderTypeLimit:
		return e.matchLimit(ctx, book, o)
	default:
		return nil, errors.New("unknown order type")
	}
}

func (e *Engine) getOrCreateBook(symbol string) *OrderBook {
	if b, ok := e.books[symbol]; ok {
		return b
	}
	e.books[symbol] = &OrderBook{}
	return e.books[symbol]
}

func (e *Engine) matchLimit(ctx context.Context, book *OrderBook, incoming *domain.Order) ([]domain.Trade, error) {
	var trades []domain.Trade
	remaining := incoming.Quantity

	for remaining.GreaterThan(decimal.Zero) {
		var cp *domain.Order
		if incoming.Side == domain.SideBuy {
			cp = book.bestAsk()
			if cp == nil || cp.Price.GreaterThan(incoming.Price) {
				break
			}
		} else {
			cp = book.bestBid()
			if cp == nil || cp.Price.LessThan(incoming.Price) {
				break
			}
		}
		fillQty := decimal.Min(remaining, cp.Quantity.Sub(cp.FilledQty))
		trades = append(trades, domain.Trade{
			ID:          domain.TradeID(uuid.NewString()),
			Symbol:      incoming.Symbol,
			BuyOrderID:  orderIDForSide(domain.SideBuy, incoming, cp),
			SellOrderID: orderIDForSide(domain.SideSell, incoming, cp),
			Price:       cp.Price,
			Quantity:    fillQty,
			ExecutedAt:  time.Now().UnixNano(),
		})
		remaining = remaining.Sub(fillQty)
		cp.FilledQty = cp.FilledQty.Add(fillQty)
		if cp.FilledQty.Equal(cp.Quantity) {
			if err := book.removeOrder(cp.ID); err != nil {
				return nil, fmt.Errorf("remove filled order: %w", err)
			}
		}
	}

	if remaining.GreaterThan(decimal.Zero) {
		incoming.FilledQty = incoming.Quantity.Sub(remaining)
		incoming.Quantity = remaining
		switch {
		case incoming.FilledQty.IsZero():
			incoming.Status = domain.OrderStatusNew
		default:
			incoming.Status = domain.OrderStatusPartial
		}
		if err := book.insertOrder(incoming); err != nil {
			return nil, fmt.Errorf("insert resting order: %w", err)
		}
	} else {
		incoming.FilledQty = incoming.Quantity
		incoming.Status = domain.OrderStatusFilled
	}
	return trades, nil
}

func (e *Engine) matchMarket(ctx context.Context, book *OrderBook, incoming *domain.Order) ([]domain.Trade, error) {
	if incoming.Side == domain.SideBuy {
		incoming.Price = decimal.NewFromInt(1_000_000_000)
	} else {
		incoming.Price = decimal.Zero
	}
	return e.matchLimit(ctx, book, incoming)
}

func orderIDForSide(side domain.Side, incoming, resting *domain.Order) domain.OrderID {
	if incoming.Side == side {
		return incoming.ID
	}
	return resting.ID
}
