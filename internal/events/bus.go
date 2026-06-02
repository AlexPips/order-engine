package events

import "sync"

type TradeEvent struct {
	Symbol string
	BuyID  string
	SellID string
	Price  string
	Qty    string
}

type OrderUpdateEvent struct {
	OrderID string
	Symbol  string
	Status  string
}

type Handler func(any)

type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

func New() *Bus {
	return &Bus{handlers: make(map[string][]Handler)}
}

func (b *Bus) Subscribe(topic string, fn Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], fn)
}

func (b *Bus) Publish(topic string, msg any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, fn := range b.handlers[topic] {
		fn(msg)
	}
}
