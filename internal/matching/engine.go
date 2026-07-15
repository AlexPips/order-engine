package matching

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var ErrInsufficientLiquidity = errors.New("no matching orders available")

type Engine struct {
	mu    sync.RWMutex
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
		return e.matchLimit(ctx, book, o, decimal.Zero)
	default:
		return nil, errors.New("unknown order type")
	}
}

func (e *Engine) getOrCreateBook(symbol string) *OrderBook {
	// Fast path: read lock
	e.mu.RLock()
	if b, ok := e.books[symbol]; ok {
		e.mu.RUnlock()
		return b
	}
	e.mu.RUnlock()

	// Slow path: write lock with double-check
	e.mu.Lock()
	defer e.mu.Unlock()
	if b, ok := e.books[symbol]; ok {
		return b
	}
	e.books[symbol] = newOrderBook()
	return e.books[symbol]
}

func (e *Engine) GetOrderBook(symbol string) OrderBookSnapshot {
	book := e.getOrCreateBook(symbol)
	snap := book.snapshot()
	snap.Symbol = symbol
	return snap
}

func (e *Engine) ReplayOrders(orders []domain.Order) {
	bySymbol := make(map[string][]domain.Order)
	for i := range orders {
		bySymbol[orders[i].Symbol] = append(bySymbol[orders[i].Symbol], orders[i])
	}
	for symbol, symOrders := range bySymbol {
		book := e.getOrCreateBook(symbol)
		for i := range symOrders {
			_ = book.insertOrder(&symOrders[i]) //nolint:errcheck
		}
	}
}

func (e *Engine) matchLimit(ctx context.Context, book *OrderBook, incoming *domain.Order, priceLimit decimal.Decimal) ([]domain.Trade, error) {
	var trades []domain.Trade
	remaining := incoming.Quantity

	if incoming.Side == domain.SideBuy {
		for i := 0; i < len(book.asks) && remaining.GreaterThan(decimal.Zero); {
			lvl := &book.asks[i]
			if lvl.Price.GreaterThan(incoming.Price) {
				break
			}
			if !priceLimit.IsZero() && lvl.Price.GreaterThan(priceLimit) {
				break
			}
			before := len(lvl.Orders)
			lvl.Orders = fillOrdersAtLevel(lvl.Orders, incoming, &remaining, &trades)
			emptied := len(lvl.Orders) == 0 && before > 0
			book.pruneEmptyLevels()
			if emptied {
				continue
			}
			i++
		}
	} else {
		for i := 0; i < len(book.bids) && remaining.GreaterThan(decimal.Zero); {
			lvl := &book.bids[i]
			if lvl.Price.LessThan(incoming.Price) {
				break
			}
			if !priceLimit.IsZero() && lvl.Price.LessThan(priceLimit) {
				break
			}
			before := len(lvl.Orders)
			lvl.Orders = fillOrdersAtLevel(lvl.Orders, incoming, &remaining, &trades)
			emptied := len(lvl.Orders) == 0 && before > 0
			book.pruneEmptyLevels()
			if emptied {
				continue
			}
			i++
		}
	}

	filledQty := incoming.Quantity.Sub(remaining)
	if remaining.GreaterThan(decimal.Zero) {
		incoming.FilledQty = filledQty
		switch {
		case filledQty.IsZero():
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

func fillOrdersAtLevel(
	orders []domain.Order,
	incoming *domain.Order,
	remaining *decimal.Decimal,
	trades *[]domain.Trade,
) []domain.Order {
	for j := 0; j < len(orders) && remaining.GreaterThan(decimal.Zero); {
		ro := &orders[j]
		if incoming.UserID == ro.UserID {
			j++
			continue
		}
		fillQty := decimal.Min(*remaining, ro.Quantity.Sub(ro.FilledQty))
		*trades = append(*trades, domain.Trade{
			ID:          domain.TradeID(uuid.NewString()),
			Symbol:      incoming.Symbol,
			BuyOrderID:  orderIDForSide(domain.SideBuy, incoming, ro),
			SellOrderID: orderIDForSide(domain.SideSell, incoming, ro),
			Price:       ro.Price,
			Quantity:    fillQty,
			ExecutedAt:  time.Now().UnixNano(),
		})
		*remaining = remaining.Sub(fillQty)
		ro.FilledQty = ro.FilledQty.Add(fillQty)
		if ro.FilledQty.Equal(ro.Quantity) {
			orders = append(orders[:j], orders[j+1:]...)
			continue
		}
		j++
	}
	return orders
}

func (e *Engine) matchMarket(ctx context.Context, book *OrderBook, incoming *domain.Order) ([]domain.Trade, error) {
	var priceLimit decimal.Decimal
	if incoming.MaxSlippageBPS > 0 {
		bps := decimal.NewFromInt(int64(incoming.MaxSlippageBPS))
		factor := bps.Div(decimal.NewFromInt(10000))
		if incoming.Side == domain.SideBuy {
			best := book.bestAsk()
			if best == nil {
				return nil, ErrInsufficientLiquidity
			}
			priceLimit = best.Price.Mul(decimal.NewFromInt(1).Add(factor))
			incoming.Price = priceLimit
		} else {
			best := book.bestBid()
			if best == nil {
				return nil, ErrInsufficientLiquidity
			}
			priceLimit = best.Price.Mul(decimal.NewFromInt(1).Sub(factor))
			incoming.Price = priceLimit
		}
	} else {
		if incoming.Side == domain.SideBuy {
			incoming.Price = decimal.NewFromInt(1_000_000_000)
		} else {
			incoming.Price = decimal.Zero
		}
	}
	return e.matchLimit(ctx, book, incoming, priceLimit)
}

func orderIDForSide(side domain.Side, incoming, resting *domain.Order) domain.OrderID {
	if incoming.Side == side {
		return incoming.ID
	}
	return resting.ID
}
