package pubengine

import (
	"sync"
	"time"
)

// LoginLimiter rate-limits login attempts per IP address.
type LoginLimiter struct {
	done     chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
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
		done:     make(chan struct{}), stopped: make(chan struct{}),
	}
	go l.cleanup()
	return l
}

func (l *LoginLimiter) cleanup() {
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()
	defer close(l.stopped)
	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
		}
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

// Allow atomically checks and records an attempt, including successful logins.
func (l *LoginLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-l.window)
	hits := l.attempts[ip]
	kept := hits[:0]
	for _, hit := range hits {
		if hit.After(cutoff) {
			kept = append(kept, hit)
		}
	}
	if len(kept) >= l.max {
		l.attempts[ip] = kept
		return false
	}
	l.attempts[ip] = append(kept, time.Now())
	return true
}

// Check returns true if the IP has not exceeded the rate limit.
// This is informational only. Use Allow for atomic admission.
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

// Close stops the cleanup worker and waits for it to exit. It is safe to repeat.
func (l *LoginLimiter) Close() {
	l.stopOnce.Do(func() { close(l.done) })
	<-l.stopped
}
