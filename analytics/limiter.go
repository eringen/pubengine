package analytics

import (
	"sync"
	"time"
)

// rateLimiter is a per-key sliding-window rate limiter.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		hits:   make(map[string][]time.Time),
		max:    max,
		window: window,
	}
	go rl.cleanup()
	return rl
}

// allow checks if key has not exceeded the limit and records the request.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	hits := rl.hits[key]
	kept := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.max {
		rl.hits[key] = kept
		return false
	}
	rl.hits[key] = append(kept, now)
	return true
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	for range ticker.C {
		cutoff := time.Now().Add(-rl.window)
		rl.mu.Lock()
		for key, hits := range rl.hits {
			kept := hits[:0]
			for _, t := range hits {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(rl.hits, key)
			} else {
				rl.hits[key] = kept
			}
		}
		rl.mu.Unlock()
	}
}
