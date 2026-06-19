package store

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Store interface {
	Get(key string) (*StoredObject, error)
	Put(key string, obj *StoredObject) error
	Delete(key string) error
	List(prefix string) ([]*StoredObject, error)
	Watch(prefix string, sinceRevision int64) (<-chan WatchEvent, error)
	Unwatch(ch <-chan WatchEvent)
	CurrentRevision() int64
	Compact(beforeRevision int64) error
	Close() error
}

type StoredObject struct {
	Key            string          `json:"key"`
	Value          json.RawMessage `json:"value"`
	Revision       int64           `json:"revision"`
	ModRevision    int64           `json:"mod_revision"`
	CreateRevision int64           `json:"create_revision"`
	Version        int64           `json:"version"`
	Lease          int64           `json:"lease,omitempty"`
	Deleted        bool            `json:"deleted,omitempty"`
}

type WatchEvent struct {
	Type   string        `json:"type"`
	Object *StoredObject `json:"object"`
}

const (
	EventAdded    = "ADDED"
	EventModified = "MODIFIED"
	EventDeleted  = "DELETED"
	EventBookmark = "BOOKMARK"
	EventError    = "ERROR"
)

type MemoryStore struct {
	mu        sync.RWMutex
	objects   map[string]*StoredObject
	revision  atomic.Int64
	watchers  map[int]*watcher
	watcherID int
	closed    bool
}

type watcher struct {
	prefix  string
	since   int64
	ch      chan WatchEvent
	store   *MemoryStore
	id      int
}

func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		objects:  make(map[string]*StoredObject),
		watchers: make(map[int]*watcher),
	}
	s.revision.Store(0)
	return s
}

func (s *MemoryStore) Get(key string) (*StoredObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj, ok := s.objects[key]
	if !ok || obj.Deleted {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	copy := *obj
	return &copy, nil
}

func (s *MemoryStore) Put(key string, obj *StoredObject) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rev := s.revision.Add(1)

	existing, exists := s.objects[key]
	if exists && !existing.Deleted {
		obj.CreateRevision = existing.CreateRevision
		obj.Version = existing.Version + 1
		obj.ModRevision = rev
		obj.Revision = rev
		obj.Deleted = false
		s.objects[key] = obj
		s.notifyWatchers(WatchEvent{Type: EventModified, Object: obj})
	} else {
		obj.CreateRevision = rev
		obj.ModRevision = rev
		obj.Revision = rev
		obj.Version = 1
		obj.Deleted = false
		s.objects[key] = obj
		s.notifyWatchers(WatchEvent{Type: EventAdded, Object: obj})
	}

	return nil
}

func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj, ok := s.objects[key]
	if !ok || obj.Deleted {
		return fmt.Errorf("key not found: %s", key)
	}

	rev := s.revision.Add(1)
	obj.Deleted = true
	obj.ModRevision = rev
	obj.Revision = rev

	s.notifyWatchers(WatchEvent{Type: EventDeleted, Object: obj})
	return nil
}

func (s *MemoryStore) List(prefix string) ([]*StoredObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*StoredObject
	for key, obj := range s.objects {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix && !obj.Deleted {
			copy := *obj
			result = append(result, &copy)
		}
	}
	return result, nil
}

func (s *MemoryStore) Watch(prefix string, sinceRevision int64) (<-chan WatchEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("store closed")
	}

	s.watcherID++
	id := s.watcherID

	ch := make(chan WatchEvent, 256)
	w := &watcher{
		prefix: prefix,
		since:  sinceRevision,
		ch:     ch,
		store:  s,
		id:     id,
	}
	s.watchers[id] = w

	for _, obj := range s.objects {
		if obj.Revision > sinceRevision && len(obj.Key) >= len(prefix) && obj.Key[:len(prefix)] == prefix {
			eventType := EventAdded
			if obj.Deleted {
				eventType = EventDeleted
			} else if obj.Version > 1 {
				eventType = EventModified
			}
			copy := *obj
			select {
			case ch <- WatchEvent{Type: eventType, Object: &copy}:
			default:
			}
		}
	}

	return ch, nil
}

func (s *MemoryStore) Unwatch(ch <-chan WatchEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, w := range s.watchers {
		if w.ch == ch {
			close(w.ch)
			delete(s.watchers, id)
			return
		}
	}
}

func (s *MemoryStore) notifyWatchers(event WatchEvent) {
	for _, w := range s.watchers {
		if len(event.Object.Key) >= len(w.prefix) && event.Object.Key[:len(w.prefix)] == w.prefix {
			if event.Object.Revision > w.since {
				select {
				case w.ch <- event:
				default:
				}
			}
		}
	}
}

func (s *MemoryStore) CurrentRevision() int64 {
	return s.revision.Load()
}

func (s *MemoryStore) Compact(beforeRevision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, obj := range s.objects {
		if obj.Deleted && obj.ModRevision < beforeRevision {
			delete(s.objects, key)
		}
	}
	return nil
}

func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	for id, w := range s.watchers {
		close(w.ch)
		delete(s.watchers, id)
	}
	return nil
}

func KeyFor(group, resource, namespace, name string) string {
	if namespace == "" {
		return fmt.Sprintf("/registry/%s/%s/%s", group, resource, name)
	}
	return fmt.Sprintf("/registry/%s/%s/%s/%s", group, resource, namespace, name)
}

func ParseKey(key string) (group, resource, namespace, name string, err error) {
	parts := splitKey(key)
	switch len(parts) {
	case 4:
		return parts[1], parts[2], "", parts[3], nil
	case 5:
		return parts[1], parts[2], parts[3], parts[4], nil
	default:
		return "", "", "", "", fmt.Errorf("invalid key: %s", key)
	}
}

func splitKey(key string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			if i > start {
				parts = append(parts, key[start:i])
			}
			start = i + 1
		}
	}
	if start < len(key) {
		parts = append(parts, key[start:])
	}
	return parts
}

type Lease struct {
	ID        int64
	TTL       time.Duration
	Keys      []string
	ExpiresAt time.Time
}
