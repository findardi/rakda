package ratelimit

import (
	"maps"
	"sync"
	"time"
)

type Memory struct {
	mu      sync.Mutex
	buckets map[string]*window
}

type window struct {
	count   int
	resetAt time.Time
}

func NewMemory() *Memory {
	m := &Memory{buckets: make(map[string]*window)}

	go m.janitor()

	return m
}

func (m *Memory) Allow(key string, limit int, win time.Duration) (allowed bool, retryAfter time.Duration) {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.buckets[key]
	if !ok || now.After(w.resetAt) {
		m.buckets[key] = &window{
			count:   1,
			resetAt: now.Add(win),
		}
		return true, 0
	}

	if w.count >= limit {
		return false, w.resetAt.Sub(now)
	}

	w.count++
	return true, 0
}

func (m *Memory) janitor() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()

	for now := range t.C {
		m.mu.Lock()
		maps.DeleteFunc(m.buckets, func(_ string, w *window) bool { return now.After(w.resetAt) })
		m.mu.Unlock()
	}
}
