package main

import (
	"sync"
	"time"
)

// rateLimiter is a simple in-memory sliding-window limiter. Cloud Run may run
// several instances, each with its own limiter; that's fine for abuse
// prevention at our scale (and max-instances is kept small).
type rateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newRateLimiter() *rateLimiter { return &rateLimiter{hits: map[string][]time.Time{}} }

func (l *rateLimiter) allow(key string, max int, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	if len(l.hits) > 10000 { // bound memory; drop everything if we're being flooded
		l.hits = map[string][]time.Time{key: l.hits[key]}
	}
	return true
}
