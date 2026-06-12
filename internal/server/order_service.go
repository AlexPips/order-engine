package server

import (
	"context"
	"sync"
	"time"

	orderpb "github.com/AlexPips/order-engine/gen/order/v1"
	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/AlexPips/order-engine/internal/events"
	"github.com/AlexPips/order-engine/internal/matching"
	"github.com/AlexPips/order-engine/internal/repository"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type OrderService struct {
	orderpb.UnimplementedOrderServiceServer
	engine *matching.Engine
	bus    *events.Bus
	repo   *repository.Queries
	mu     sync.RWMutex
	orders map[domain.OrderID]*domain.Order
}

func NewOrderService(engine *matching.Engine, bus *events.Bus, repo *repository.Queries) *OrderService {
	return &OrderService{
		engine: engine,
		bus:    bus,
		repo:   repo,
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
	s.mu.Lock()
	s.orders[o.ID] = &o
	s.mu.Unlock()

	trades, err := s.engine.SubmitOrder(ctx, &o)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.mu.RLock()
	stored := s.orders[o.ID]
	s.mu.RUnlock()

	params := domainToCreateParams(stored)
	if _, err := s.repo.CreateOrder(ctx, params); err != nil {
		return nil, status.Errorf(codes.Internal, "persist order: %v", err)
	}

	for _, t := range trades {
		s.bus.Publish("trade."+t.Symbol, events.TradeEvent{
			Symbol: t.Symbol, BuyID: string(t.BuyOrderID),
			SellID: string(t.SellOrderID), Price: t.Price.String(), Qty: t.Quantity.String(),
		})
		if _, err := s.repo.CreateTrade(ctx, domainToTradeParams(&t)); err != nil {
			return nil, status.Errorf(codes.Internal, "persist trade: %v", err)
		}
	}

	return &orderpb.CreateOrderResponse{Order: domainToProto(stored)}, nil
}

func (s *OrderService) GetOrder(ctx context.Context, req *orderpb.GetOrderRequest) (*orderpb.GetOrderResponse, error) {
	row, err := s.repo.GetOrder(ctx, req.GetId())
	if err != nil {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	return &orderpb.GetOrderResponse{Order: repoToOrderProto(&row)}, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, req *orderpb.CancelOrderRequest) (*orderpb.CancelOrderResponse, error) {
	row, err := s.repo.CancelOrder(ctx, repository.CancelOrderParams{
		ID:        req.GetId(),
		UpdatedAt: time.Now().UnixNano(),
	})
	if err != nil {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	s.mu.Lock()
	if o, ok := s.orders[domain.OrderID(row.ID)]; ok {
		o.Status = domain.OrderStatusCanceled
		o.UpdatedAt = row.UpdatedAt
	}
	s.mu.Unlock()

	s.bus.Publish("order.cancel", events.OrderUpdateEvent{
		OrderID: row.ID, Symbol: row.Symbol, Status: "CANCELED",
	})
	return &orderpb.CancelOrderResponse{Order: repoToOrderProto(&row)}, nil
}

func (s *OrderService) GetOrderBook(ctx context.Context, req *orderpb.GetOrderBookRequest) (*orderpb.GetOrderBookResponse, error) {
	symbol := req.GetSymbol()
	if symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}

	snap := s.engine.GetOrderBook(symbol)

	bids := make([]*orderpb.PriceLevel, len(snap.Bids))
	for i, b := range snap.Bids {
		bids[i] = &orderpb.PriceLevel{
			Price:      decimalToProto(b.Price),
			Quantity:   decimalToProto(b.Quantity),
			OrderCount: int32(b.OrderCount),
		}
	}

	asks := make([]*orderpb.PriceLevel, len(snap.Asks))
	for i, a := range snap.Asks {
		asks[i] = &orderpb.PriceLevel{
			Price:      decimalToProto(a.Price),
			Quantity:   decimalToProto(a.Quantity),
			OrderCount: int32(a.OrderCount),
		}
	}

	return &orderpb.GetOrderBookResponse{
		OrderBook: &orderpb.OrderBookSnapshot{
			Symbol: symbol,
			Bids:   bids,
			Asks:   asks,
		},
	}, nil
}

func domainToCreateParams(o *domain.Order) repository.CreateOrderParams {
	return repository.CreateOrderParams{
		ID:        string(o.ID),
		UserID:    string(o.UserID),
		Symbol:    o.Symbol,
		Side:      sideToString(o.Side),
		Type:      orderTypeToString(o.Type),
		Price:     o.Price,
		Quantity:  o.Quantity,
		FilledQty: o.FilledQty,
		Status:    statusToString(o.Status),
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
}

func domainToTradeParams(t *domain.Trade) repository.CreateTradeParams {
	return repository.CreateTradeParams{
		ID:          string(t.ID),
		Symbol:      t.Symbol,
		BuyOrderID:  string(t.BuyOrderID),
		SellOrderID: string(t.SellOrderID),
		Price:       t.Price,
		Quantity:    t.Quantity,
		ExecutedAt:  t.ExecutedAt,
	}
}

func repoToOrderProto(o *repository.Order) *orderpb.Order {
	return &orderpb.Order{
		Id:                o.ID,
		UserId:            o.UserID,
		Symbol:            o.Symbol,
		Side:              stringToSideProto(o.Side),
		Type:              stringToTypeProto(o.Type),
		Price:             decimalToProto(o.Price),
		Quantity:          decimalToProto(o.Quantity),
		FilledQuantity:    decimalToProto(o.FilledQty),
		Status:            stringToStatusProto(o.Status),
		CreatedAtUnixNano: o.CreatedAt,
		UpdatedAtUnixNano: o.UpdatedAt,
	}
}

func sideToString(s domain.Side) string {
	if s == domain.SideSell {
		return "SELL"
	}
	return "BUY"
}

func orderTypeToString(t domain.OrderType) string {
	if t == domain.OrderTypeMarket {
		return "MARKET"
	}
	return "LIMIT"
}

func statusToString(s domain.OrderStatus) string {
	switch s {
	case domain.OrderStatusPartial:
		return "PARTIAL"
	case domain.OrderStatusFilled:
		return "FILLED"
	case domain.OrderStatusCanceled:
		return "CANCELED"
	case domain.OrderStatusRejected:
		return "REJECTED"
	default:
		return "NEW"
	}
}

func stringToSideProto(s string) orderpb.Side {
	if s == "SELL" {
		return orderpb.Side_SIDE_SELL
	}
	return orderpb.Side_SIDE_BUY
}

func stringToTypeProto(t string) orderpb.OrderType {
	if t == "MARKET" {
		return orderpb.OrderType_ORDER_TYPE_MARKET
	}
	return orderpb.OrderType_ORDER_TYPE_LIMIT
}

func stringToStatusProto(s string) orderpb.OrderStatus {
	switch s {
	case "PARTIAL":
		return orderpb.OrderStatus_ORDER_STATUS_PARTIAL
	case "FILLED":
		return orderpb.OrderStatus_ORDER_STATUS_FILLED
	case "CANCELED":
		return orderpb.OrderStatus_ORDER_STATUS_CANCELED
	case "REJECTED":
		return orderpb.OrderStatus_ORDER_STATUS_REJECTED
	default:
		return orderpb.OrderStatus_ORDER_STATUS_NEW
	}
}

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

func domainToProto(o *domain.Order) *orderpb.Order {
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
