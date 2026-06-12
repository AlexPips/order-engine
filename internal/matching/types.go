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

type SnapshotPriceLevel struct {
	Price      decimal.Decimal
	Quantity   decimal.Decimal
	OrderCount int
}

type OrderBookSnapshot struct {
	Symbol string
	Bids   []SnapshotPriceLevel // sorted high→low
	Asks   []SnapshotPriceLevel // sorted low→high
}

type OrderBook struct {
	mu   sync.RWMutex
	bids []PriceLevel
	asks []PriceLevel
}
