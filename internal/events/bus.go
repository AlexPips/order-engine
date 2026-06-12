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
	subs     map[string][]chan any
}

func New() *Bus {
	return &Bus{
		handlers: make(map[string][]Handler),
		subs:     make(map[string][]chan any),
	}
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
	for _, ch := range b.subs[topic] {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *Bus) SubscribeChannel(topic string, bufSize int) chan any {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan any, bufSize)
	b.subs[topic] = append(b.subs[topic], ch)
	return ch
}

func (b *Bus) UnsubscribeChannel(topic string, ch chan any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subs[topic]
	for i, s := range subs {
		if s == ch {
			b.subs[topic] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}
