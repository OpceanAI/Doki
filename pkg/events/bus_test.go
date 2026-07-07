package events

import (
	"context"
	"testing"
	"time"
)

func TestBusPublishSubscribe(t *testing.T) {
	b := NewBus()
	defer b.Close()

	sub := b.Subscribe(Filter{}, 4)
	b.PublishContainer(ActionStart, "c1", "myapp", nil)

	select {
	case ev := <-sub.Channel():
		if ev.Type != TypeContainer {
			t.Fatalf("type=%s", ev.Type)
		}
		if ev.Action != ActionStart {
			t.Fatalf("action=%s", ev.Action)
		}
		if ev.Actor.ID != "c1" {
			t.Fatalf("actor id=%s", ev.Actor.ID)
		}
		if ev.Actor.Attributes["name"] != "myapp" {
			t.Fatalf("name attr=%v", ev.Actor.Attributes)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBusFilter(t *testing.T) {
	b := NewBus()
	defer b.Close()

	sub := b.Subscribe(Filter{Type: []string{"image"}}, 4)
	b.PublishContainer(ActionStart, "c1", "myapp", nil)
	b.PublishImage(ActionPull, "alpine:3", nil)

	select {
	case ev := <-sub.Channel():
		if ev.Type != TypeImage {
			t.Fatalf("got type %s, want image", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no image event received")
	}
}

func TestBusSubscribeContext(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sub := b.SubscribeContext(ctx, Filter{}, 4)

	b.PublishContainer(ActionStart, "c1", "myapp", nil)
	<-sub.Channel()

	cancel()
	// After cancel, channel should be closed.
	select {
	case _, ok := <-sub.Channel():
		if ok {
			t.Fatal("expected channel to be closed after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for close")
	}
}

func TestBusDropOnFull(t *testing.T) {
	b := NewBus()
	defer b.Close()

	// Subscriber with tiny buffer that never reads.
	_ = b.Subscribe(Filter{}, 1)

	// Publish many events; none should block the publisher.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			b.PublishContainer(ActionStart, "c1", "myapp", nil)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publisher blocked")
	}
}

func TestFilterMatches(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
		event  Event
		want   bool
	}{
		{"empty", Filter{}, Event{Type: TypeContainer, Action: ActionStart}, true},
		{"type match", Filter{Type: []string{"container"}}, Event{Type: TypeContainer, Action: ActionStart}, true},
		{"type mismatch", Filter{Type: []string{"image"}}, Event{Type: TypeContainer, Action: ActionStart}, false},
		{"event match", Filter{Event: []string{"start"}}, Event{Type: TypeContainer, Action: ActionStart}, true},
		{"label match", Filter{Label: []string{"a=b"}}, Event{Type: TypeContainer, Actor: Actor{Attributes: map[string]string{"a": "b"}}}, true},
		{"label mismatch", Filter{Label: []string{"a=c"}}, Event{Type: TypeContainer, Actor: Actor{Attributes: map[string]string{"a": "b"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.matches(tt.event); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterFromJSON(t *testing.T) {
	data := []byte(`{"type":["container"],"event":["start","die"],"label":["a=b"]}`)
	f, err := FilterFromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Type) != 1 || f.Type[0] != "container" {
		t.Errorf("type=%v", f.Type)
	}
	if len(f.Event) != 2 {
		t.Errorf("event=%v", f.Event)
	}
	if len(f.Label) != 1 {
		t.Errorf("label=%v", f.Label)
	}
}
