package main

import (
	"testing"
	"time"
)

func TestLoginLimiterBlocksAfterMax(t *testing.T) {
	limiter := newLoginLimiter(2, 200*time.Millisecond)
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
	limiter := newLoginLimiter(1, 150*time.Millisecond)
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
	limiter := newLoginLimiter(1, 200*time.Millisecond)

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
