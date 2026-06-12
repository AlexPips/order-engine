package events

import (
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	bus := New()
	received := make(chan any, 1)

	bus.Subscribe("test", func(msg any) {
		received <- msg
	})

	bus.Publish("test", "hello")

	select {
	case msg := <-received:
		if msg != "hello" {
			t.Errorf("expected hello, got %v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestChannelSubscription(t *testing.T) {
	bus := New()
	ch := bus.SubscribeChannel("test", 16)

	bus.Publish("test", "hello")

	select {
	case msg := <-ch:
		if msg != "hello" {
			t.Errorf("expected hello, got %v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestChannelNonBlocking(t *testing.T) {
	bus := New()
	ch := bus.SubscribeChannel("test", 1)

	bus.Publish("test", "msg1")
	bus.Publish("test", "msg2")

	select {
	case msg := <-ch:
		if msg != "msg1" {
			t.Errorf("expected msg1, got %v", msg)
		}
	default:
		t.Fatal("expected message in channel")
	}
}

func TestUnsubscribeChannel(t *testing.T) {
	bus := New()
	ch := bus.SubscribeChannel("test", 16)

	bus.UnsubscribeChannel("test", ch)

	bus.Publish("test", "hello")

	select {
	case msg, ok := <-ch:
		if ok && msg != nil {
			t.Fatal("should not receive non-nil after unsubscribe")
		}
	default:
	}
}
