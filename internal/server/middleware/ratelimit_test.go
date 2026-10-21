package middleware

import (
	"testing"
	"time"
)

func TestKeyedLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewKeyedLimiter(60, 2) // 1 token/sec, burst 2
	l.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d should be allowed (burst)", i)
		}
	}
	ok, wait := l.Allow("a")
	if ok {
		t.Fatal("third request should be limited")
	}
	if wait <= 0 || wait > time.Second {
		t.Fatalf("wait = %v; want (0, 1s]", wait)
	}

	// Other keys are independent.
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("key b should have its own bucket")
	}

	// A rejected request must not consume a token: after 1s exactly one
	// more request is allowed.
	now = now.Add(time.Second)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("should be allowed after refill")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("should be limited again")
	}
}

func TestKeyedLimiterEvictsIdle(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewKeyedLimiter(60, 1)
	l.now = func() time.Time { return now }
	l.Allow("a")
	l.Allow("b")
	now = now.Add(11 * time.Minute)
	l.Allow("c")
	if n := len(l.buckets); n != 1 {
		t.Fatalf("buckets = %d; want 1 after eviction", n)
	}
}
