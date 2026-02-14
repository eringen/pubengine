package pubengine

import (
	"sync"
	"time"
)

// LoginLimiter rate-limits login attempts per IP address.
type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

// NewLoginLimiter creates a LoginLimiter that allows max attempts per window.
func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	l := &LoginLimiter{
		attempts: make(map[string][]time.Time),
		max:      max,
		window:   window,
	}
	go l.cleanup()
	return l
}

func (l *LoginLimiter) cleanup() {
	ticker := time.NewTicker(l.window)
	for range ticker.C {
		cutoff := time.Now().Add(-l.window)
		l.mu.Lock()
		for ip, hits := range l.attempts {
			kept := hits[:0]
			for _, t := range hits {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(l.attempts, ip)
			} else {
				l.attempts[ip] = kept
			}
		}
		l.mu.Unlock()
	}
}

// Allow checks if the IP has not exceeded the rate limit and records the attempt.
// Kept for backwards compatibility; prefer Check + Record for login flows.
func (l *LoginLimiter) Allow(ip string) bool {
	if !l.Check(ip) {
		return false
	}
	l.Record(ip)
	return true
}

// Check returns true if the IP has not exceeded the rate limit.
// It does not record an attempt — call Record separately on failure.
func (l *LoginLimiter) Check(ip string) bool {
	cutoff := time.Now().Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	hits := l.attempts[ip]
	kept := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.attempts[ip] = kept
	return len(kept) < l.max
}

// Record registers a failed login attempt for the given IP.
func (l *LoginLimiter) Record(ip string) {
	l.mu.Lock()
	l.attempts[ip] = append(l.attempts[ip], time.Now())
	l.mu.Unlock()
}
