package server

import (
	"context"
	"io"
	"sync"
	"time"

	orderpb "github.com/AlexPips/order-engine/gen/proto/order/v1"
	"github.com/AlexPips/order-engine/internal/domain"
	"github.com/AlexPips/order-engine/internal/events"
	"github.com/AlexPips/order-engine/internal/matching"
	"github.com/AlexPips/order-engine/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type OrderService struct {
	orderpb.UnimplementedOrderServiceServer
	engine *matching.Engine
	bus    *events.Bus
	repo   *repository.Queries
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	orders map[domain.OrderID]*domain.Order
}

func NewOrderService(engine *matching.Engine, bus *events.Bus, repo *repository.Queries, pool *pgxpool.Pool) *OrderService {
	return &OrderService{
		engine: engine,
		bus:    bus,
		repo:   repo,
		pool:   pool,
		orders: make(map[domain.OrderID]*domain.Order),
	}
}

func (s *OrderService) RecoverState(ctx context.Context) error {
	dbOrders, err := s.repo.GetAllOpenOrders(ctx)
	if err != nil {
		return err
	}

	domainOrders := make([]domain.Order, 0, len(dbOrders))
	for i := range dbOrders {
		o := domain.Order{
			ID:        domain.OrderID(dbOrders[i].ID),
			UserID:    domain.UserID(dbOrders[i].UserID),
			Symbol:    dbOrders[i].Symbol,
			Side:      stringToDomainSide(dbOrders[i].Side),
			Type:      stringToDomainType(dbOrders[i].Type),
			Price:     dbOrders[i].Price,
			Quantity:  dbOrders[i].Quantity,
			FilledQty: dbOrders[i].FilledQty,
			Status:    stringToDomainStatus(dbOrders[i].Status),
			CreatedAt: dbOrders[i].CreatedAt,
			UpdatedAt: dbOrders[i].UpdatedAt,
		}
		s.orders[o.ID] = &o
		domainOrders = append(domainOrders, o)
	}

	s.engine.ReplayOrders(domainOrders)
	return nil
}

func (s *OrderService) persistOrderTx(ctx context.Context, o *domain.Order, trades []domain.Trade) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	txRepo := s.repo.WithTx(tx)
	if _, err := txRepo.CreateOrder(ctx, domainToCreateParams(o)); err != nil {
		return err
	}

	for _, t := range trades {
		s.bus.Publish("trade."+t.Symbol, events.TradeEvent{
			Symbol: t.Symbol, BuyID: string(t.BuyOrderID),
			SellID: string(t.SellOrderID), Price: t.Price.String(), Qty: t.Quantity.String(),
		})
		if _, err := txRepo.CreateTrade(ctx, domainToTradeParams(&t)); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *OrderService) CreateOrder(ctx context.Context, req *orderpb.CreateOrderRequest) (*orderpb.CreateOrderResponse, error) {
	oid := domain.OrderID(req.GetIdempotencyKey())
	if oid == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}

	// Idempotency check: reject duplicate order IDs
	s.mu.RLock()
	if _, exists := s.orders[oid]; exists {
		s.mu.RUnlock()
		return nil, status.Errorf(codes.AlreadyExists, "order %s already exists", oid)
	}
	s.mu.RUnlock()

	o := domain.Order{
		ID:             oid,
		UserID:         domain.UserID(req.GetUserId()),
		Symbol:         req.GetSymbol(),
		Side:           protoToDomainSide(req.GetSide()),
		Type:           protoToDomainType(req.GetType()),
		Price:          pbDecimalToDecimal(req.GetPrice()),
		Quantity:       pbDecimalToDecimal(req.GetQuantity()),
		Status:         domain.OrderStatusNew,
		CreatedAt:      time.Now().UnixNano(),
		UpdatedAt:      time.Now().UnixNano(),
		MaxSlippageBPS: req.GetMaxSlippageBps(),
	}

	trades, err := s.engine.SubmitOrder(ctx, &o)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	s.mu.Lock()
	s.orders[o.ID] = &o
	s.mu.Unlock()

	// Transactional DB writes: wrap order + trades in a single transaction
	if err := s.persistOrderTx(ctx, &o, trades); err != nil {
		return nil, status.Errorf(codes.Internal, "persist order: %v", err)
	}

	return &orderpb.CreateOrderResponse{Order: domainToProto(&o)}, nil
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
	for i := range snap.Bids {
		bids[i] = &orderpb.PriceLevel{
			Price:      decimalToProto(snap.Bids[i].Price),
			Quantity:   decimalToProto(snap.Bids[i].Quantity),
			OrderCount: int32(snap.Bids[i].OrderCount), //nolint:gosec // G115: order book depth fits int32
		}
	}

	asks := make([]*orderpb.PriceLevel, len(snap.Asks))
	for i := range snap.Asks {
		asks[i] = &orderpb.PriceLevel{
			Price:      decimalToProto(snap.Asks[i].Price),
			Quantity:   decimalToProto(snap.Asks[i].Quantity),
			OrderCount: int32(snap.Asks[i].OrderCount), //nolint:gosec // G115: order book depth fits int32
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

func (s *OrderService) StreamOrderUpdates(req *orderpb.StreamOrderUpdatesRequest, stream grpc.ServerStreamingServer[orderpb.StreamOrderUpdatesResponse]) error {
	topic := "order.update"
	ch := s.bus.SubscribeChannel(topic, 64)
	defer s.bus.UnsubscribeChannel(topic, ch)

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			ev, ok := msg.(events.OrderUpdateEvent)
			if !ok {
				continue
			}
			if !s.matchesFilter(req, ev) {
				continue
			}
			row, err := s.repo.GetOrder(stream.Context(), ev.OrderID)
			if err != nil {
				continue
			}
			if err := stream.Send(&orderpb.StreamOrderUpdatesResponse{
				Order: repoToOrderProto(&row),
			}); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

func (s *OrderService) matchesFilter(req *orderpb.StreamOrderUpdatesRequest, ev events.OrderUpdateEvent) bool {
	switch f := req.GetFilter().(type) {
	case *orderpb.StreamOrderUpdatesRequest_Symbol:
		return ev.Symbol == f.Symbol
	case *orderpb.StreamOrderUpdatesRequest_UserId:
		row, err := s.repo.GetOrder(context.Background(), ev.OrderID)
		if err != nil {
			return false
		}
		return row.UserID == f.UserId
	default:
		return true
	}
}

func (s *OrderService) BatchCreateOrders(stream grpc.ClientStreamingServer[orderpb.BatchCreateOrdersRequest, orderpb.BatchCreateOrdersResponse]) error {
	var created []*orderpb.Order

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&orderpb.BatchCreateOrdersResponse{Orders: created})
		}
		if err != nil {
			return err
		}

		oid := domain.OrderID(req.GetOrder().GetIdempotencyKey())
		if oid == "" {
			continue
		}

		// Idempotency check: skip duplicate order IDs
		s.mu.RLock()
		if _, exists := s.orders[oid]; exists {
			s.mu.RUnlock()
			continue
		}
		s.mu.RUnlock()

		o := domain.Order{
			ID:             oid,
			UserID:         domain.UserID(req.GetOrder().GetUserId()),
			Symbol:         req.GetOrder().GetSymbol(),
			Side:           protoToDomainSide(req.GetOrder().GetSide()),
			Type:           protoToDomainType(req.GetOrder().GetType()),
			Price:          pbDecimalToDecimal(req.GetOrder().GetPrice()),
			Quantity:       pbDecimalToDecimal(req.GetOrder().GetQuantity()),
			Status:         domain.OrderStatusNew,
			CreatedAt:      time.Now().UnixNano(),
			UpdatedAt:      time.Now().UnixNano(),
			MaxSlippageBPS: req.GetOrder().GetMaxSlippageBps(),
		}

		trades, err := s.engine.SubmitOrder(stream.Context(), &o)
		if err != nil {
			continue
		}

		s.mu.Lock()
		s.orders[o.ID] = &o
		s.mu.Unlock()

		if err := s.persistOrderTx(context.Background(), &o, trades); err != nil {
			continue
		}

		created = append(created, domainToProto(&o))
	}
}

func (s *OrderService) TradeFeed(stream grpc.BidiStreamingServer[orderpb.TradeFeedRequest, orderpb.TradeFeedResponse]) error {
	errc := make(chan error, 2)
	subscriptions := make(map[string]chan any)
	var subMu sync.Mutex

	go func() {
		for {
			req, err := stream.Recv()
			if err != nil {
				errc <- err
				return
			}
			switch m := req.GetMsg().(type) {
			case *orderpb.TradeFeedRequest_SubscribeSymbol:
				symbol := m.SubscribeSymbol
				topic := "trade." + symbol

				subMu.Lock()
				if _, ok := subscriptions[symbol]; ok {
					subMu.Unlock()
					continue
				}
				ch := s.bus.SubscribeChannel(topic, 256)
				subscriptions[symbol] = ch
				subMu.Unlock()

				go func(sym string, tradeCh chan any) {
					for msg := range tradeCh {
						ev, ok := msg.(events.TradeEvent)
						if !ok {
							continue
						}
						if err := stream.Send(&orderpb.TradeFeedResponse{
							Msg: &orderpb.TradeFeedResponse_Trade{
								Trade: &orderpb.Trade{
									Symbol:      ev.Symbol,
									BuyOrderId:  ev.BuyID,
									SellOrderId: ev.SellID,
									Price:       stringToDecimalProto(ev.Price),
									Quantity:    stringToDecimalProto(ev.Qty),
								},
							},
						}); err != nil {
							errc <- err
							return
						}
					}
				}(symbol, ch)

			case *orderpb.TradeFeedRequest_NewOrder:
				o := domain.Order{
					ID:             domain.OrderID(m.NewOrder.GetIdempotencyKey()),
					UserID:         domain.UserID(m.NewOrder.GetUserId()),
					Symbol:         m.NewOrder.GetSymbol(),
					Side:           protoToDomainSide(m.NewOrder.GetSide()),
					Type:           protoToDomainType(m.NewOrder.GetType()),
					Price:          pbDecimalToDecimal(m.NewOrder.GetPrice()),
					Quantity:       pbDecimalToDecimal(m.NewOrder.GetQuantity()),
					Status:         domain.OrderStatusNew,
					CreatedAt:      time.Now().UnixNano(),
					UpdatedAt:      time.Now().UnixNano(),
					MaxSlippageBPS: m.NewOrder.GetMaxSlippageBps(),
				}

				s.mu.Lock()
				if _, exists := s.orders[o.ID]; exists {
					s.mu.Unlock()
					continue
				}
				trades, err := s.engine.SubmitOrder(stream.Context(), &o)
				if err != nil {
					s.mu.Unlock()
					continue
				}
				s.orders[o.ID] = &o
				s.mu.Unlock()

				_ = s.persistOrderTx(context.Background(), &o, trades) //nolint:errcheck
			}
		}
	}()

	err := <-errc

	subMu.Lock()
	for symbol, ch := range subscriptions {
		topic := "trade." + symbol
		s.bus.UnsubscribeChannel(topic, ch)
	}
	subMu.Unlock()

	return err
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

func stringToDecimalProto(s string) *orderpb.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimalToProto(decimal.Zero)
	}
	return decimalToProto(d)
}

func stringToDomainSide(s string) domain.Side {
	if s == "SELL" {
		return domain.SideSell
	}
	return domain.SideBuy
}

func stringToDomainType(s string) domain.OrderType {
	if s == "MARKET" {
		return domain.OrderTypeMarket
	}
	return domain.OrderTypeLimit
}

func stringToDomainStatus(s string) domain.OrderStatus {
	switch s {
	case "PARTIAL":
		return domain.OrderStatusPartial
	case "FILLED":
		return domain.OrderStatusFilled
	case "CANCELED":
		return domain.OrderStatusCanceled
	case "REJECTED":
		return domain.OrderStatusRejected
	default:
		return domain.OrderStatusNew
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
