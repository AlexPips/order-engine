package matching

import (
	"errors"
	"sort"

	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/shopspring/decimal"
)

var (
	ErrDuplicateOrder = errors.New("order already exists")
)

func insertIntoLevel(levels []PriceLevel, o *domain.Order) []PriceLevel {
	// Binary search finds insertion index in one pass (O(log n)).
	// Bids are sorted high→low, asks low→high.
	insertIdx := sort.Search(len(levels), func(i int) bool {
		if o.Side == domain.SideBuy {
			return levels[i].Price.LessThan(o.Price)
		}
		return levels[i].Price.GreaterThan(o.Price)
	})

	// Price level exists at insertIdx-1 (sort.Search returns the first index
	// where the predicate is true, which is past any equal-price level since
	// LessThan/GreaterThan are both false for equal decimals).
	if insertIdx > 0 && levels[insertIdx-1].Price.Equal(o.Price) {
		levels[insertIdx-1].Orders = append(levels[insertIdx-1].Orders, *o)
		return levels
	}

	// New price level — insert at insertIdx (shifts elements, O(n)).
	newLevel := PriceLevel{
		Price:  o.Price,
		Orders: []domain.Order{*o},
	}
	levels = append(levels, PriceLevel{})
	copy(levels[insertIdx+1:], levels[insertIdx:])
	levels[insertIdx] = newLevel

	return levels
}

// bestBid returns the best bid. Caller must hold book.mu.
func (ob *OrderBook) bestBid() *domain.Order {
	if len(ob.bids) == 0 || len(ob.bids[0].Orders) == 0 {
		return nil
	}
	return &ob.bids[0].Orders[0]
}

// bestAsk returns the best ask. Caller must hold book.mu.
func (ob *OrderBook) bestAsk() *domain.Order {
	if len(ob.asks) == 0 || len(ob.asks[0].Orders) == 0 {
		return nil
	}
	return &ob.asks[0].Orders[0]
}

// insertOrderLocked inserts an order. Caller must hold book.mu.
// Market orders are not resting orders, so they are not inserted.
func (ob *OrderBook) insertOrderLocked(o *domain.Order) error {
	if o.Type != domain.OrderTypeLimit {
		return nil
	}
	if _, exists := ob.orders[o.ID]; exists {
		return ErrDuplicateOrder
	}
	ob.orders[o.ID] = o
	if o.Side == domain.SideBuy {
		ob.bids = insertIntoLevel(ob.bids, o)
	} else {
		ob.asks = insertIntoLevel(ob.asks, o)
	}
	return nil
}

// pruneEmptyLevelsLocked removes empty price levels. Caller must hold book.mu.
func (ob *OrderBook) pruneEmptyLevelsLocked() {
	ob.bids = pruneLevels(ob.bids)
	ob.asks = pruneLevels(ob.asks)
}

func (ob *OrderBook) snapshot() OrderBookSnapshot {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids := make([]SnapshotPriceLevel, len(ob.bids))
	for i, lvl := range ob.bids {
		var qty decimal.Decimal
		for j := range lvl.Orders {
			qty = qty.Add(lvl.Orders[j].Quantity.Sub(lvl.Orders[j].FilledQty))
		}
		bids[i] = SnapshotPriceLevel{
			Price:      lvl.Price,
			Quantity:   qty,
			OrderCount: len(lvl.Orders),
		}
	}

	asks := make([]SnapshotPriceLevel, len(ob.asks))
	for i, lvl := range ob.asks {
		var qty decimal.Decimal
		for j := range lvl.Orders {
			qty = qty.Add(lvl.Orders[j].Quantity.Sub(lvl.Orders[j].FilledQty))
		}
		asks[i] = SnapshotPriceLevel{
			Price:      lvl.Price,
			Quantity:   qty,
			OrderCount: len(lvl.Orders),
		}
	}

	return OrderBookSnapshot{Bids: bids, Asks: asks}
}

func pruneLevels(levels []PriceLevel) []PriceLevel {
	n := 0
	for _, lvl := range levels {
		if len(lvl.Orders) > 0 {
			levels[n] = lvl
			n++
		}
	}
	return levels[:n]
}
