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

func (ob *OrderBook) insertOrder(o *domain.Order) error {
	if o.Type != domain.OrderTypeLimit {
		return nil
	}
	ob.mu.Lock()
	defer ob.mu.Unlock()
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

func insertIntoLevel(levels []PriceLevel, o *domain.Order) []PriceLevel {
	for i, lvl := range levels {
		if o.Price.Equal(lvl.Price) {
			levels[i].Orders = append(levels[i].Orders, *o)
			return levels
		}
	}

	insertIdx := sort.Search(len(levels), func(i int) bool {
		if o.Side == domain.SideBuy {
			return levels[i].Price.LessThan(o.Price)
		}
		return levels[i].Price.GreaterThan(o.Price)
	})

	newLevel := PriceLevel{
		Price:  o.Price,
		Orders: []domain.Order{*o},
	}
	levels = append(levels, PriceLevel{})
	copy(levels[insertIdx+1:], levels[insertIdx:])
	levels[insertIdx] = newLevel

	return levels
}

func (ob *OrderBook) bestBid() *domain.Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	if len(ob.bids) == 0 || len(ob.bids[0].Orders) == 0 {
		return nil
	}
	return &ob.bids[0].Orders[0]
}

func (ob *OrderBook) bestAsk() *domain.Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	if len(ob.asks) == 0 || len(ob.asks[0].Orders) == 0 {
		return nil
	}
	return &ob.asks[0].Orders[0]
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

func (ob *OrderBook) pruneEmptyLevels() {
	ob.bids = pruneLevels(ob.bids)
	ob.asks = pruneLevels(ob.asks)
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
