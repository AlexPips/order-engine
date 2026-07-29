package domain

import "github.com/shopspring/decimal"

type Side int

const (
	SideBuy Side = iota
	SideSell
)

func (s Side) String() string {
	switch s {
	case SideBuy:
		return "BUY"
	case SideSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

type OrderType int

const (
	OrderTypeLimit OrderType = iota
	OrderTypeMarket
)

func (t OrderType) String() string {
	switch t {
	case OrderTypeLimit:
		return "LIMIT"
	case OrderTypeMarket:
		return "MARKET"
	default:
		return "UNKNOWN"
	}
}

type OrderStatus int

const (
	OrderStatusNew OrderStatus = iota
	OrderStatusPartial
	OrderStatusFilled
	OrderStatusCanceled
	OrderStatusRejected
)

func (s OrderStatus) String() string {
	switch s {
	case OrderStatusNew:
		return "NEW"
	case OrderStatusPartial:
		return "PARTIAL"
	case OrderStatusFilled:
		return "FILLED"
	case OrderStatusCanceled:
		return "CANCELED"
	case OrderStatusRejected:
		return "REJECTED"
	default:
		return "UNKNOWN"
	}
}

type Order struct {
	ID             OrderID
	UserID         UserID
	Symbol         string
	Side           Side
	Type           OrderType
	Price          decimal.Decimal
	Quantity       decimal.Decimal
	FilledQty      decimal.Decimal
	Status         OrderStatus
	CreatedAt      int64
	UpdatedAt      int64
	MaxSlippageBPS uint32
}
