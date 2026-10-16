package cache

import "sync"

type Memory struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

func NewMemory() *Memory {
	return &Memory{
		entries: make(map[string]Entry),
	}
}

func (m *Memory) Get(key string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[key]
	return entry, ok
}

func (m *Memory) Set(key string, entry Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries[key] = entry
}

func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make(map[string]Entry)
}
