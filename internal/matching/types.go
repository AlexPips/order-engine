package matching

import (
	"sync"

	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/shopspring/decimal"
)

type PriceLevel struct {
	Price  decimal.Decimal
	Orders []domain.Order
}

// OrderBook holds resting orders for one symbol.
// Bids sorted high→low, asks sorted low→high.
type OrderBook struct {
	mu   sync.RWMutex
	bids []PriceLevel
	asks []PriceLevel
}
