package domain

import "github.com/shopspring/decimal"

type Trade struct {
	ID          TradeID
	Symbol      string
	BuyOrderID  OrderID
	SellOrderID OrderID
	Price       decimal.Decimal
	Quantity    decimal.Decimal
	ExecutedAt  int64
}
