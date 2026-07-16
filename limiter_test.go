package pubengine

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoginLimiterBlocksAfterMax(t *testing.T) {
	limiter := NewLoginLimiter(2, 200*time.Millisecond)
	t.Cleanup(limiter.Close)
	ip := "203.0.113.10"

	if !limiter.Allow(ip) {
		t.Fatalf("expected first attempt to be allowed")
	}
	if !limiter.Allow(ip) {
		t.Fatalf("expected second attempt to be allowed")
	}
	if limiter.Allow(ip) {
		t.Fatalf("expected third attempt to be blocked")
	}
}

func TestLoginLimiterResetsAfterWindow(t *testing.T) {
	limiter := NewLoginLimiter(1, 150*time.Millisecond)
	t.Cleanup(limiter.Close)
	ip := "203.0.113.20"

	if !limiter.Allow(ip) {
		t.Fatalf("expected first attempt to be allowed")
	}
	if limiter.Allow(ip) {
		t.Fatalf("expected second attempt to be blocked")
	}

	time.Sleep(200 * time.Millisecond)
	if !limiter.Allow(ip) {
		t.Fatalf("expected attempt after window to be allowed")
	}
}

func TestLoginLimiterIsPerIP(t *testing.T) {
	limiter := NewLoginLimiter(1, 200*time.Millisecond)
	t.Cleanup(limiter.Close)

	if !limiter.Allow("203.0.113.30") {
		t.Fatalf("expected first ip to be allowed")
	}
	if !limiter.Allow("203.0.113.31") {
		t.Fatalf("expected second ip to be allowed independently")
	}
	if limiter.Allow("203.0.113.30") {
		t.Fatalf("expected first ip to be blocked after max")
	}
}

func TestLoginLimiterConcurrentAdmission(t *testing.T) {
	l := NewLoginLimiter(5, time.Minute)
	t.Cleanup(l.Close)
	var allowed atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if l.Allow("same-ip") {
				allowed.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if allowed.Load() != 5 {
		t.Fatalf("admitted %d attempts", allowed.Load())
	}
}
