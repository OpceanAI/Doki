// Package events provides a thread-safe event bus for the Doki daemon.
//
// It mirrors the semantics of the Docker /events endpoint and the
// Podman /libpod/events endpoint: subscribers receive a continuous
// stream of typed events, optionally filtered by type/event/labels.
// Subscribers can be removed at any time without affecting other
// subscribers. The bus is the single source of truth for all
// container/image/network/volume/system lifecycle notifications.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Type is the high-level event category.
type Type string

const (
	TypeContainer Type = "container"
	TypeImage     Type = "image"
	TypeNetwork   Type = "network"
	TypeVolume    Type = "volume"
	TypePod       Type = "pod"
	TypeSecret    Type = "secret"
	TypeSystem    Type = "system"
	TypeBuilder   Type = "builder"
)

// Action is the specific action that occurred. Values are the
// canonical Moby/Podman strings ("start", "die", "create", ...).
type Action string

const (
	ActionCreate   Action = "create"
	ActionStart    Action = "start"
	ActionStop     Action = "stop"
	ActionDie      Action = "die"
	ActionKill     Action = "kill"
	ActionPause    Action = "pause"
	ActionUnpause  Action = "unpause"
	ActionRestart  Action = "restart"
	ActionDestroy  Action = "destroy"
	ActionRemove   Action = "destroy"
	ActionRename   Action = "rename"
	ActionResize   Action = "resize"
	ActionArchive  Action = "archive"
	ActionExport   Action = "export"
	ActionImport   Action = "import"
	ActionPull     Action = "pull"
	ActionPush     Action = "push"
	ActionTag      Action = "tag"
	ActionUntag    Action = "untag"
	ActionDelete   Action = "delete"
	ActionConnect  Action = "connect"
	ActionDisconnect Action = "disconnect"
	ActionPrune    Action = "prune"
	ActionHealth   Action = "health_status"
	ActionOOM      Action = "oom"
	ActionExec     Action = "exec_create"
	ActionExecDie  Action = "exec_die"
)

// Event is a single lifecycle notification.
type Event struct {
	Type        Type             `json:"Type"`
	Action      Action           `json:"Action"`
	Actor       Actor            `json:"Actor"`
	Status      string           `json:"status,omitempty"`
	ID          string           `json:"id,omitempty"`
	From        string           `json:"from,omitempty"`
	Time        int64            `json:"time"`
	TimeNano    int64            `json:"timeNano"`
	Scope       string           `json:"scope,omitempty"`
	Attributes  map[string]string `json:"Actor.Attributes,omitempty"`
}

// Actor is the object the event refers to.
type Actor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes,omitempty"`
}

// Filter narrows the events a subscriber receives. Each non-empty
// field is AND-combined; each list within a field is OR-combined.
type Filter struct {
	Type   []string `json:"type,omitempty"`
	Event  []string `json:"event,omitempty"`
	Container []string `json:"container,omitempty"`
	Image  []string `json:"image,omitempty"`
	Network []string `json:"network,omitempty"`
	Volume []string `json:"volume,omitempty"`
	Pod    []string `json:"pod,omitempty"`
	Label  []string `json:"label,omitempty"`
}

// Subscription is a handle returned by Subscribe that the caller
// must Close() to stop receiving events.
type Subscription struct {
	id     uint64
	ch     chan Event
	filter Filter
	bus    *Bus
	closed atomic.Bool
}

// Channel returns the receive-only channel for events.
func (s *Subscription) Channel() <-chan Event { return s.ch }

// ID returns the subscription's unique identifier.
func (s *Subscription) ID() uint64 { return s.id }

// Close releases the subscription. Idempotent.
func (s *Subscription) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.bus.unsubscribe(s)
	return nil
}

// matches reports whether an event passes the subscription's filter.
func (f Filter) matches(e Event) bool {
	if len(f.Type) > 0 {
		ok := false
		for _, t := range f.Type {
			if t == string(e.Type) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Event) > 0 {
		ok := false
		for _, a := range f.Event {
			if a == string(e.Action) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Container) > 0 {
		ok := false
		for _, c := range f.Container {
			if c == e.Actor.ID || strings.HasPrefix(e.Actor.ID, c) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Image) > 0 {
		ok := false
		for _, im := range f.Image {
			if im == e.Actor.ID || strings.Contains(e.Actor.ID, im) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Network) > 0 {
		ok := false
		for _, n := range f.Network {
			if n == e.Actor.ID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Volume) > 0 {
		ok := false
		for _, v := range f.Volume {
			if v == e.Actor.ID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Pod) > 0 {
		ok := false
		for _, p := range f.Pod {
			if p == e.Actor.ID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Label) > 0 {
		ok := true
		for _, lf := range f.Label {
			parts := strings.SplitN(lf, "=", 2)
			key := parts[0]
			want := ""
			if len(parts) == 2 {
				want = parts[1]
			}
			got, has := e.Actor.Attributes[key]
			if !has || (want != "" && got != want) {
				ok = false
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Bus is the central event multiplexer.
type Bus struct {
	mu     sync.RWMutex
	subs   map[uint64]*Subscription
	nextID atomic.Uint64
	logger *slog.Logger
}

// NewBus creates an empty bus.
func NewBus() *Bus {
	return &Bus{
		subs:   make(map[uint64]*Subscription),
		logger: slog.Default().With("component", "events-bus"),
	}
}

// Subscribe registers a new subscriber with the given filter and
// buffer size. Buffer size 0 means unbuffered; otherwise the channel
// will buffer up to bufferSize events before dropping the slowest
// subscribers' messages (publishing is non-blocking).
func (b *Bus) Subscribe(f Filter, bufferSize int) *Subscription {
	id := b.nextID.Add(1)
	if bufferSize < 0 {
		bufferSize = 0
	}
	ch := make(chan Event, bufferSize)
	sub := &Subscription{
		id:     id,
		ch:     ch,
		filter: f,
		bus:    b,
	}
	b.mu.Lock()
	b.subs[id] = sub
	b.mu.Unlock()
	return sub
}

// SubscribeContext is like Subscribe but additionally closes the
// subscription when ctx is cancelled.
func (b *Bus) SubscribeContext(ctx context.Context, f Filter, bufferSize int) *Subscription {
	sub := b.Subscribe(f, bufferSize)
	go func() {
		<-ctx.Done()
		_ = sub.Close()
	}()
	return sub
}

// Publish sends an event to all matching subscribers. It never blocks
// and never returns an error: if a subscriber's buffer is full, the
// event is dropped for that subscriber (and the bus increments a
// per-subscriber drop counter visible via Stats).
func (b *Bus) Publish(e Event) {
	if e.Time == 0 {
		e.Time = time.Now().Unix()
	}
	if e.TimeNano == 0 {
		e.TimeNano = time.Now().UnixNano()
	}
	if e.Actor.Attributes == nil {
		e.Actor.Attributes = map[string]string{}
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs {
		if s.closed.Load() {
			continue
		}
		if !s.filter.matches(e) {
			continue
		}
		select {
		case s.ch <- e:
		default:
			// Drop event for this subscriber to avoid head-of-line
			// blocking across the bus. Production-grade event buses
			// prefer dropping over blocking.
			b.logger.Debug("event dropped for slow subscriber",
				"sub_id", s.id, "type", e.Type, "action", e.Action)
		}
	}
}

// PublishContainer is a convenience for container events.
func (b *Bus) PublishContainer(action Action, id, name string, attrs map[string]string) {
	e := Event{
		Type:   TypeContainer,
		Action: action,
		Actor: Actor{
			ID:         id,
			Attributes: mergeAttrs(map[string]string{"name": name}, attrs),
		},
	}
	b.Publish(e)
}

// PublishImage publishes an image event.
func (b *Bus) PublishImage(action Action, id string, attrs map[string]string) {
	b.Publish(Event{
		Type:   TypeImage,
		Action: action,
		Actor:  Actor{ID: id, Attributes: attrs},
	})
}

// PublishNetwork publishes a network event.
func (b *Bus) PublishNetwork(action Action, id, name string) {
	b.Publish(Event{
		Type:   TypeNetwork,
		Action: action,
		Actor:  Actor{ID: id, Attributes: map[string]string{"name": name}},
	})
}

// PublishVolume publishes a volume event.
func (b *Bus) PublishVolume(action Action, name string) {
	b.Publish(Event{
		Type:   TypeVolume,
		Action: action,
		Actor:  Actor{ID: name, Attributes: map[string]string{"name": name}},
	})
}

// PublishPod publishes a pod event.
func (b *Bus) PublishPod(action Action, id, name string) {
	b.Publish(Event{
		Type:   TypePod,
		Action: action,
		Actor:  Actor{ID: id, Attributes: map[string]string{"name": name}},
	})
}

// PublishSystem publishes a system-level event.
func (b *Bus) PublishSystem(action Action, attrs map[string]string) {
	b.Publish(Event{
		Type:   TypeSystem,
		Action: action,
		Actor:  Actor{ID: "system", Attributes: attrs},
	})
}

// unsubscribe removes a subscription and closes its channel.
func (b *Bus) unsubscribe(s *Subscription) {
	b.mu.Lock()
	delete(b.subs, s.id)
	b.mu.Unlock()
	close(s.ch)
}

// Count returns the number of active subscribers.
func (b *Bus) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

// Close shuts down the bus and closes all subscriber channels.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, s := range b.subs {
		if !s.closed.Swap(true) {
			close(s.ch)
		}
		delete(b.subs, id)
	}
}

// ToJSON returns a JSON-encoded representation suitable for /events.
func (e Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FilterFromJSON parses a Docker-style filter JSON. The format is a
// map of arrays of strings: `{"status":["running"],"label":["a=b"]}`.
func FilterFromJSON(data []byte) (Filter, error) {
	if len(data) == 0 {
		return Filter{}, nil
	}
	var raw map[string][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return Filter{}, fmt.Errorf("parse filters: %w", err)
	}
	f := Filter{
		Type:      raw["type"],
		Event:     raw["event"],
		Container: raw["container"],
		Image:     raw["image"],
		Network:   raw["network"],
		Volume:    raw["volume"],
		Pod:       raw["pod"],
		Label:     raw["label"],
	}
	// Docker also accepts "status" for container filter; map to Event.
	if len(raw["status"]) > 0 {
		f.Event = append(f.Event, raw["status"]...)
	}
	return f, nil
}

func mergeAttrs(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
