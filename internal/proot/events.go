package proot

import "sync"

// EventType is the type of container event.
type EventType string

const (
	EventStdout EventType = "stdout"
	EventStderr EventType = "stderr"
	EventExit   EventType = "exit"
	EventError  EventType = "error"
	EventReady  EventType = "ready"
)

// ContainerEvent represents an event from a container.
type ContainerEvent struct {
	Type     EventType
	ID       string
	Data     string
	ExitCode int
}

// EventSubscriber receives container events.
type EventSubscriber func(event ContainerEvent)

// EventBus manages event subscriptions for containers.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]EventSubscriber
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]EventSubscriber),
	}
}

// Subscribe registers a subscriber for a container's events.
func (b *EventBus) Subscribe(containerID string, fn EventSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[containerID] = append(b.subscribers[containerID], fn)
}

// Unsubscribe removes all subscribers for a container.
func (b *EventBus) Unsubscribe(containerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, containerID)
}

// Publish sends an event to all subscribers of the container.
func (b *EventBus) Publish(event ContainerEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, fn := range b.subscribers[event.ID] {
		go func(f EventSubscriber) {
			defer func() { recover() }()
			f(event)
		}(fn)
	}
}

// SubscriberCount returns the number of subscribers for a container.
func (b *EventBus) SubscriberCount(containerID string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[containerID])
}
