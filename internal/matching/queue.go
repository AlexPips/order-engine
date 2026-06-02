package matching

import (
	"errors"
	"sort"

	"github.com/AlexPips/order-engine/internal/domain"
)

var (
	ErrDuplicateOrder = errors.New("order already exists")
	ErrOrderNotFound  = errors.New("order not found")
)

func (ob *OrderBook) insertOrder(o domain.Order) error {
	if o.Type != domain.OrderTypeLimit {
		return nil
	}
	ob.mu.Lock()
	defer ob.mu.Unlock()
	if ob.findOrder(o.ID) != nil {
		return ErrDuplicateOrder
	}
	if o.Side == domain.SideBuy {
		ob.bids = insertIntoLevel(ob.bids, o)
	} else {
		ob.asks = insertIntoLevel(ob.asks, o)
	}
	return nil
}

func insertIntoLevel(levels []PriceLevel, o domain.Order) []PriceLevel {
	for i, lvl := range levels {
		if o.Price.Equal(lvl.Price) {
			levels[i].Orders = append(levels[i].Orders, o)
			return levels
		}
	}
	levels = append(levels, PriceLevel{
		Price:  o.Price,
		Orders: []domain.Order{o},
	})
	sort.Slice(levels, func(i, j int) bool {
		if o.Side == domain.SideBuy {
			return levels[i].Price.GreaterThan(levels[j].Price)
		}
		return levels[i].Price.LessThan(levels[j].Price)
	})
	return levels
}

func (ob *OrderBook) findOrder(id domain.OrderID) *domain.Order {
	for _, levels := range [][]PriceLevel{ob.bids, ob.asks} {
		for _, lvl := range levels {
			for i := range lvl.Orders {
				if lvl.Orders[i].ID == id {
					return &lvl.Orders[i]
				}
			}
		}
	}
	return nil
}

// removeOrder removes an order from the book. Called on cancel or full fill.
func (ob *OrderBook) removeOrder(id domain.OrderID) error {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	for sideIdx, levels := range [][]PriceLevel{ob.bids, ob.asks} {
		for lvlIdx := range levels {
			for ordIdx, o := range levels[lvlIdx].Orders {
				if o.ID == id {
					lvl := &levels[lvlIdx]
					lvl.Orders = append(lvl.Orders[:ordIdx], lvl.Orders[ordIdx+1:]...)
					if len(lvl.Orders) == 0 {
						if sideIdx == 0 {
							ob.bids = append(ob.bids[:lvlIdx], ob.bids[lvlIdx+1:]...)
						} else {
							ob.asks = append(ob.asks[:lvlIdx], ob.asks[lvlIdx+1:]...)
						}
					}
					return nil
				}
			}
		}
	}
	return ErrOrderNotFound
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
