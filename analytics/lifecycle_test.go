package analytics

import (
	"testing"
	"time"
)

func TestWorkersCanBeStoppedRepeatedly(t *testing.T) {
	s := testStore(t)
	stop := s.StartCleanupScheduler(365, time.Millisecond)
	stop()
	stop()
	h := NewHandler(s)
	h.Close()
	h.Close()
	select {
	case <-h.collectLimiter.stopped:
	default:
		t.Fatal("limiter worker did not exit")
	}
}
