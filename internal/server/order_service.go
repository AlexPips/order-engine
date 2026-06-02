package server

import (
	"context"
	"sync"
	"time"

	orderpb "github.com/AlexPips/order-engine/gen/order/v1"
	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/AlexPips/order-engine/internal/events"
	"github.com/AlexPips/order-engine/internal/matching"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type OrderService struct {
	orderpb.UnimplementedOrderServiceServer
	engine *matching.Engine
	bus    *events.Bus
	mu     sync.RWMutex
	orders map[domain.OrderID]*domain.Order
}

func NewOrderService(engine *matching.Engine, bus *events.Bus) *OrderService {
	return &OrderService{
		engine: engine,
		bus:    bus,
		orders: make(map[domain.OrderID]*domain.Order),
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, req *orderpb.CreateOrderRequest) (*orderpb.CreateOrderResponse, error) {
	oid := domain.OrderID(req.GetIdempotencyKey())
	if oid == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	o := domain.Order{
		ID:        oid,
		UserID:    domain.UserID(req.GetUserId()),
		Symbol:    req.GetSymbol(),
		Side:      protoToDomainSide(req.GetSide()),
		Type:      protoToDomainType(req.GetType()),
		Price:     pbDecimalToDecimal(req.GetPrice()),
		Quantity:  pbDecimalToDecimal(req.GetQuantity()),
		Status:    domain.OrderStatusNew,
		CreatedAt: time.Now().UnixNano(),
		UpdatedAt: time.Now().UnixNano(),
	}
	// store before submit so the engine can reference it
	s.mu.Lock()
	s.orders[o.ID] = &o
	s.mu.Unlock()

	trades, err := s.engine.SubmitOrder(ctx, o)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	for _, t := range trades {
		s.bus.Publish("trade."+t.Symbol, events.TradeEvent{
			Symbol: t.Symbol, BuyID: string(t.BuyOrderID),
			SellID: string(t.SellOrderID), Price: t.Price.String(), Qty: t.Quantity.String(),
		})
	}
	// refresh from map after engine mutated it
	s.mu.RLock()
	stored := s.orders[o.ID]
	s.mu.RUnlock()
	return &orderpb.CreateOrderResponse{Order: domainToProto(*stored)}, nil
}

func (s *OrderService) GetOrder(ctx context.Context, req *orderpb.GetOrderRequest) (*orderpb.GetOrderResponse, error) {
	s.mu.RLock()
	o, ok := s.orders[domain.OrderID(req.GetId())]
	s.mu.RUnlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	return &orderpb.GetOrderResponse{Order: domainToProto(*o)}, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, req *orderpb.CancelOrderRequest) (*orderpb.CancelOrderResponse, error) {
	s.mu.Lock()
	o, ok := s.orders[domain.OrderID(req.GetId())]
	if !ok {
		s.mu.Unlock()
		return nil, status.Error(codes.NotFound, "order not found")
	}
	o.Status = domain.OrderStatusCanceled
	o.UpdatedAt = time.Now().UnixNano()
	s.mu.Unlock()
	// remove from the order book
	s.bus.Publish("order.cancel", events.OrderUpdateEvent{
		OrderID: string(o.ID), Symbol: o.Symbol, Status: "CANCELED",
	})
	return &orderpb.CancelOrderResponse{Order: domainToProto(*o)}, nil
}

// helpers remain the same as before
func protoToDomainSide(s orderpb.Side) domain.Side {
	if s == orderpb.Side_SIDE_SELL {
		return domain.SideSell
	}
	return domain.SideBuy
}

func protoToDomainType(t orderpb.OrderType) domain.OrderType {
	if t == orderpb.OrderType_ORDER_TYPE_MARKET {
		return domain.OrderTypeMarket
	}
	return domain.OrderTypeLimit
}

func pbDecimalToDecimal(d *orderpb.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return decimal.New(d.GetValue(), d.GetPrecision())
}

func decimalToProto(d decimal.Decimal) *orderpb.Decimal {
	return &orderpb.Decimal{
		Value:     d.Coefficient().Int64(),
		Precision: -d.Exponent(),
	}
}

func domainToProto(o domain.Order) *orderpb.Order {
	return &orderpb.Order{
		Id:                string(o.ID),
		UserId:            string(o.UserID),
		Symbol:            o.Symbol,
		Side:              domainSideToProto(o.Side),
		Type:              domainTypeToProto(o.Type),
		Price:             decimalToProto(o.Price),
		Quantity:          decimalToProto(o.Quantity),
		FilledQuantity:    decimalToProto(o.FilledQty),
		Status:            domainStatusToProto(o.Status),
		CreatedAtUnixNano: o.CreatedAt,
		UpdatedAtUnixNano: o.UpdatedAt,
	}
}

func domainSideToProto(s domain.Side) orderpb.Side {
	if s == domain.SideSell {
		return orderpb.Side_SIDE_SELL
	}
	return orderpb.Side_SIDE_BUY
}

func domainTypeToProto(t domain.OrderType) orderpb.OrderType {
	if t == domain.OrderTypeMarket {
		return orderpb.OrderType_ORDER_TYPE_MARKET
	}
	return orderpb.OrderType_ORDER_TYPE_LIMIT
}

func domainStatusToProto(s domain.OrderStatus) orderpb.OrderStatus {
	switch s {
	case domain.OrderStatusPartial:
		return orderpb.OrderStatus_ORDER_STATUS_PARTIAL
	case domain.OrderStatusFilled:
		return orderpb.OrderStatus_ORDER_STATUS_FILLED
	case domain.OrderStatusCanceled:
		return orderpb.OrderStatus_ORDER_STATUS_CANCELED
	case domain.OrderStatusRejected:
		return orderpb.OrderStatus_ORDER_STATUS_REJECTED
	default:
		return orderpb.OrderStatus_ORDER_STATUS_NEW
	}
}

func pbStatusToDomain(s orderpb.OrderStatus) domain.OrderStatus {
	switch s {
	case orderpb.OrderStatus_ORDER_STATUS_PARTIAL:
		return domain.OrderStatusPartial
	case orderpb.OrderStatus_ORDER_STATUS_FILLED:
		return domain.OrderStatusFilled
	case orderpb.OrderStatus_ORDER_STATUS_CANCELED:
		return domain.OrderStatusCanceled
	case orderpb.OrderStatus_ORDER_STATUS_REJECTED:
		return domain.OrderStatusRejected
	default:
		return domain.OrderStatusNew
	}
}
