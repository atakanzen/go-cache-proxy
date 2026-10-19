package cache

import (
	"context"
	"sync"
)

type Memory struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

func NewMemory() *Memory {
	return &Memory{
		entries: make(map[string]Entry),
	}
}

func (m *Memory) Get(ctx context.Context, key string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[key]
	return entry, ok
}

func (m *Memory) Set(ctx context.Context, key string, entry Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries[key] = entry
	return nil
}

func (m *Memory) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make(map[string]Entry)
	return nil
}
