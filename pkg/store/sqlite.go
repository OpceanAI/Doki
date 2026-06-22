package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/ncruces/go-sqlite3"
)

var _ Store = (*SQLiteStore)(nil)

type SQLiteStore struct {
	db *sqlite3.Conn

	mu       sync.Mutex
	watchMu  sync.RWMutex
	watchers map[int]*sqliteWatcher
	nextID   int
	closed   bool
}

type sqliteWatcher struct {
	id     int
	prefix string
	since  int64
	ch     chan WatchEvent
}

const (
	revisionKey     = "current_revision"
	watchBufferSize = 256
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS objects (
	key TEXT PRIMARY KEY,
	value BLOB,
	revision INTEGER,
	mod_revision INTEGER,
	create_revision INTEGER,
	version INTEGER,
	lease INTEGER DEFAULT 0,
	deleted INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT
);
`

const selectObjectColumns = "key, value, revision, mod_revision, create_revision, version, lease, deleted"

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if path != "" && path != ":memory:" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, fmt.Errorf("create db directory: %w", err)
			}
		}
	}

	db, err := sqlite3.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	s := &SQLiteStore{
		db:       db,
		watchers: make(map[int]*sqliteWatcher),
	}

	if err := db.BusyTimeout(5 * time.Second); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	if err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	if err := db.Exec("INSERT OR IGNORE INTO meta (key, value) VALUES ('" + revisionKey + "', '0')"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init meta: %w", err)
	}

	if path != "" && path != ":memory:" {
		_ = os.Chmod(path, 0o600)
	}

	return s, nil
}

func (s *SQLiteStore) Get(key string) (*StoredObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("store closed")
	}

	obj, err := s.peekLocked(key)
	if err != nil {
		return nil, fmt.Errorf("get %q: %w", key, err)
	}
	if obj == nil || obj.Deleted {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return obj, nil
}

func (s *SQLiteStore) Put(key string, obj *StoredObject) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store closed")
	}
	if obj == nil {
		return fmt.Errorf("nil object")
	}

	obj.Key = key
	obj.Deleted = false

	tx, err := s.db.BeginImmediate()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	rev, err := s.readRevisionLocked()
	if err != nil {
		return err
	}
	newRev := rev + 1
	if err := s.setRevisionLocked(newRev); err != nil {
		return err
	}

	existing, err := s.peekLocked(key)
	if err != nil {
		return fmt.Errorf("load existing: %w", err)
	}

	eventType := EventAdded
	if existing != nil && !existing.Deleted {
		obj.CreateRevision = existing.CreateRevision
		obj.Version = existing.Version + 1
		eventType = EventModified
	} else {
		obj.CreateRevision = newRev
		obj.Version = 1
	}
	obj.Revision = newRev
	obj.ModRevision = newRev

	if err := s.upsertObjectLocked(obj, false); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true

	s.notifyWatchers(WatchEvent{Type: eventType, Object: cloneObject(obj)})
	return nil
}

func (s *SQLiteStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store closed")
	}

	existing, err := s.peekLocked(key)
	if err != nil {
		return fmt.Errorf("delete %q: %w", key, err)
	}
	if existing == nil || existing.Deleted {
		return fmt.Errorf("key not found: %s", key)
	}

	tx, err := s.db.BeginImmediate()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	rev, err := s.readRevisionLocked()
	if err != nil {
		return err
	}
	newRev := rev + 1
	if err := s.setRevisionLocked(newRev); err != nil {
		return err
	}

	if err := s.markDeletedLocked(key, newRev); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true

	existing.Deleted = true
	existing.Revision = newRev
	existing.ModRevision = newRev
	s.notifyWatchers(WatchEvent{Type: EventDeleted, Object: cloneObject(existing)})
	return nil
}

func (s *SQLiteStore) List(prefix string) ([]*StoredObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("store closed")
	}

	var query string
	if prefix == "" {
		query = "SELECT " + selectObjectColumns + " FROM objects WHERE deleted = 0"
	} else {
		query = "SELECT " + selectObjectColumns + " FROM objects WHERE deleted = 0 AND substr(key, 1, ?) = ?"
	}

	stmt, err := s.prepareLocked(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if prefix != "" {
		if err := stmt.BindInt64(1, int64(len(prefix))); err != nil {
			return nil, fmt.Errorf("bind prefix length: %w", err)
		}
		if err := stmt.BindText(2, prefix); err != nil {
			return nil, fmt.Errorf("bind prefix: %w", err)
		}
	}

	var result []*StoredObject
	for stmt.Step() {
		result = append(result, scanObject(stmt))
	}
	if err := stmt.Err(); err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	return result, nil
}

func (s *SQLiteStore) Watch(prefix string, sinceRevision int64) (<-chan WatchEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("store closed")
	}

	ch := make(chan WatchEvent, watchBufferSize)

	s.watchMu.Lock()
	s.nextID++
	id := s.nextID
	s.watchers[id] = &sqliteWatcher{
		id:     id,
		prefix: prefix,
		since:  sinceRevision,
		ch:     ch,
	}
	s.watchMu.Unlock()

	var err error
	defer func() {
		if err != nil {
			s.watchMu.Lock()
			delete(s.watchers, id)
			s.watchMu.Unlock()
			close(ch)
		}
	}()

	var query string
	if prefix == "" {
		query = "SELECT " + selectObjectColumns + " FROM objects WHERE revision > ?"
	} else {
		query = "SELECT " + selectObjectColumns + " FROM objects WHERE revision > ? AND substr(key, 1, ?) = ?"
	}

	stmt, perr := s.prepareLocked(query)
	if perr != nil {
		err = perr
		return nil, err
	}
	defer stmt.Close()

	if berr := stmt.BindInt64(1, sinceRevision); berr != nil {
		err = berr
		return nil, fmt.Errorf("bind since revision: %w", berr)
	}
	if prefix != "" {
		if berr := stmt.BindInt64(2, int64(len(prefix))); berr != nil {
			err = berr
			return nil, fmt.Errorf("bind prefix length: %w", berr)
		}
		if berr := stmt.BindText(3, prefix); berr != nil {
			err = berr
			return nil, fmt.Errorf("bind prefix: %w", berr)
		}
	}

	for stmt.Step() {
		obj := scanObject(stmt)
		eventType := EventAdded
		if obj.Deleted {
			eventType = EventDeleted
		} else if obj.Version > 1 {
			eventType = EventModified
		}
		select {
		case ch <- WatchEvent{Type: eventType, Object: obj}:
		default:
		}
	}
	if serr := stmt.Err(); serr != nil {
		err = serr
		return nil, fmt.Errorf("watch replay: %w", serr)
	}

	return ch, nil
}

func (s *SQLiteStore) Unwatch(ch <-chan WatchEvent) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()

	for id, w := range s.watchers {
		if w.ch == ch {
			close(w.ch)
			delete(s.watchers, id)
			return
		}
	}
}

func (s *SQLiteStore) CurrentRevision() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0
	}
	rev, err := s.readRevisionLocked()
	if err != nil {
		return 0
	}
	return rev
}

func (s *SQLiteStore) Compact(beforeRevision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked("DELETE FROM objects WHERE deleted = 1 AND mod_revision < ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	if err := stmt.BindInt64(1, beforeRevision); err != nil {
		return fmt.Errorf("bind before revision: %w", err)
	}
	if err := stmt.Exec(); err != nil {
		return fmt.Errorf("compact: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	s.watchMu.Lock()
	for id, w := range s.watchers {
		close(w.ch)
		delete(s.watchers, id)
	}
	s.watchMu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *SQLiteStore) notifyWatchers(event WatchEvent) {
	s.watchMu.RLock()
	defer s.watchMu.RUnlock()

	for _, w := range s.watchers {
		if hasPrefix(event.Object.Key, w.prefix) && event.Object.Revision > w.since {
			select {
			case w.ch <- event:
			default:
			}
		}
	}
}

func (s *SQLiteStore) prepareLocked(query string) (*sqlite3.Stmt, error) {
	stmt, _, err := s.db.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}
	return stmt, nil
}

func (s *SQLiteStore) readRevisionLocked() (int64, error) {
	stmt, err := s.prepareLocked("SELECT value FROM meta WHERE key = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	if err := stmt.BindText(1, revisionKey); err != nil {
		return 0, fmt.Errorf("bind revision key: %w", err)
	}

	if !stmt.Step() {
		if err := stmt.Err(); err != nil {
			return 0, fmt.Errorf("read revision: %w", err)
		}
		return 0, nil
	}

	value := stmt.ColumnText(0)
	if err := stmt.Err(); err != nil {
		return 0, fmt.Errorf("read revision: %w", err)
	}
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse revision: %w", err)
	}
	return n, nil
}

func (s *SQLiteStore) setRevisionLocked(rev int64) error {
	stmt, err := s.prepareLocked("UPDATE meta SET value = ? WHERE key = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	if err := stmt.BindText(1, strconv.FormatInt(rev, 10)); err != nil {
		return fmt.Errorf("bind revision value: %w", err)
	}
	if err := stmt.BindText(2, revisionKey); err != nil {
		return fmt.Errorf("bind revision key: %w", err)
	}
	if err := stmt.Exec(); err != nil {
		return fmt.Errorf("update revision: %w", err)
	}
	return nil
}

func (s *SQLiteStore) peekLocked(key string) (*StoredObject, error) {
	stmt, err := s.prepareLocked("SELECT " + selectObjectColumns + " FROM objects WHERE key = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if err := stmt.BindText(1, key); err != nil {
		return nil, fmt.Errorf("bind key: %w", err)
	}

	if !stmt.Step() {
		if err := stmt.Err(); err != nil {
			return nil, fmt.Errorf("peek: %w", err)
		}
		return nil, nil
	}
	obj := scanObject(stmt)
	if err := stmt.Err(); err != nil {
		return nil, fmt.Errorf("peek: %w", err)
	}
	return obj, nil
}

func (s *SQLiteStore) upsertObjectLocked(obj *StoredObject, deleted bool) error {
	stmt, err := s.prepareLocked("INSERT OR REPLACE INTO objects (key, value, revision, mod_revision, create_revision, version, lease, deleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	if err := stmt.BindText(1, obj.Key); err != nil {
		return fmt.Errorf("bind key: %w", err)
	}
	if len(obj.Value) == 0 {
		if err := stmt.BindNull(2); err != nil {
			return fmt.Errorf("bind value: %w", err)
		}
	} else {
		if err := stmt.BindBlob(2, []byte(obj.Value)); err != nil {
			return fmt.Errorf("bind value: %w", err)
		}
	}
	if err := stmt.BindInt64(3, obj.Revision); err != nil {
		return fmt.Errorf("bind revision: %w", err)
	}
	if err := stmt.BindInt64(4, obj.ModRevision); err != nil {
		return fmt.Errorf("bind mod_revision: %w", err)
	}
	if err := stmt.BindInt64(5, obj.CreateRevision); err != nil {
		return fmt.Errorf("bind create_revision: %w", err)
	}
	if err := stmt.BindInt64(6, obj.Version); err != nil {
		return fmt.Errorf("bind version: %w", err)
	}
	if err := stmt.BindInt64(7, obj.Lease); err != nil {
		return fmt.Errorf("bind lease: %w", err)
	}
	deletedFlag := int64(0)
	if deleted {
		deletedFlag = 1
	}
	if err := stmt.BindInt64(8, deletedFlag); err != nil {
		return fmt.Errorf("bind deleted: %w", err)
	}
	if err := stmt.Exec(); err != nil {
		return fmt.Errorf("upsert object: %w", err)
	}
	return nil
}

func (s *SQLiteStore) markDeletedLocked(key string, newRev int64) error {
	stmt, err := s.prepareLocked("UPDATE objects SET deleted = 1, mod_revision = ?, revision = ? WHERE key = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	if err := stmt.BindInt64(1, newRev); err != nil {
		return fmt.Errorf("bind mod_revision: %w", err)
	}
	if err := stmt.BindInt64(2, newRev); err != nil {
		return fmt.Errorf("bind revision: %w", err)
	}
	if err := stmt.BindText(3, key); err != nil {
		return fmt.Errorf("bind key: %w", err)
	}
	if err := stmt.Exec(); err != nil {
		return fmt.Errorf("mark deleted: %w", err)
	}
	return nil
}

func scanObject(stmt *sqlite3.Stmt) *StoredObject {
	obj := &StoredObject{
		Key:            stmt.ColumnText(0),
		Revision:       stmt.ColumnInt64(2),
		ModRevision:    stmt.ColumnInt64(3),
		CreateRevision: stmt.ColumnInt64(4),
		Version:        stmt.ColumnInt64(5),
		Lease:          stmt.ColumnInt64(6),
		Deleted:        stmt.ColumnInt64(7) != 0,
	}
	if stmt.ColumnType(1) != sqlite3.NULL {
		obj.Value = append(json.RawMessage(nil), stmt.ColumnRawBlob(1)...)
	}
	return obj
}

func cloneObject(obj *StoredObject) *StoredObject {
	cp := *obj
	if obj.Value != nil {
		cp.Value = append(json.RawMessage(nil), obj.Value...)
	}
	return &cp
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
