package domain

import "github.com/shopspring/decimal"

type Side int

const (
	SideBuy Side = iota
	SideSell
)

type OrderType int

const (
	OrderTypeLimit OrderType = iota
	OrderTypeMarket
)

type OrderStatus int

const (
	OrderStatusNew OrderStatus = iota
	OrderStatusPartial
	OrderStatusFilled
	OrderStatusCanceled
	OrderStatusRejected
)

type Order struct {
	ID        OrderID
	UserID    UserID
	Symbol    string
	Side      Side
	Type      OrderType
	Price     decimal.Decimal
	Quantity  decimal.Decimal
	FilledQty decimal.Decimal
	Status    OrderStatus
	CreatedAt int64
	UpdatedAt int64
}
