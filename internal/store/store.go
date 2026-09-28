// Package store provides document persistence for the learning plane: an
// embedded JSON-file store for development/single node and a PostgreSQL
// (JSONB) store for production.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ErrNotFound is returned when a document does not exist.
var ErrNotFound = errors.New("not found")

// Store is a minimal document store keyed by collection and id.
type Store interface {
	Put(coll, id string, v any) error
	Get(coll, id string, v any) error
	Delete(coll, id string) error
	// List returns raw documents of a collection ordered by id.
	List(coll string) ([]json.RawMessage, error)
	Close() error
}

// ListAs decodes a collection into a typed slice.
func ListAs[T any](s Store, coll string) ([]T, error) {
	raw, err := s.List(coll)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raw))
	for _, r := range raw {
		var v T
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Memory is an in-memory store with optional JSON file persistence.
type Memory struct {
	mu    sync.RWMutex
	data  map[string]map[string]json.RawMessage
	path  string
	dirty bool
}

// NewMemory creates a store; if path is non-empty data is loaded from and
// flushed to that file.
func NewMemory(path string) (*Memory, error) {
	m := &Memory{data: map[string]map[string]json.RawMessage{}, path: path}
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(b, &m.data); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

func (m *Memory) Put(coll, id string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[coll] == nil {
		m.data[coll] = map[string]json.RawMessage{}
	}
	m.data[coll][id] = b
	m.dirty = true
	return m.flushLocked()
}

func (m *Memory) Get(coll, id string, v any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.data[coll][id]
	if !ok {
		return ErrNotFound
	}
	return json.Unmarshal(b, v)
}

func (m *Memory) Delete(coll, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data[coll], id)
	m.dirty = true
	return m.flushLocked()
}

func (m *Memory) List(coll string) ([]json.RawMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.data[coll]))
	for id := range m.data[coll] {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]json.RawMessage, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.data[coll][id])
	}
	return out, nil
}

func (m *Memory) flushLocked() error {
	if m.path == "" || !m.dirty {
		return nil
	}
	b, err := json.Marshal(m.data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	m.dirty = false
	return os.Rename(tmp, m.path)
}

func (m *Memory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flushLocked()
}
