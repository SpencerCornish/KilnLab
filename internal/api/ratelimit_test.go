package api

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(3, time.Minute)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for i := range 3 {
		if ok, _ := l.allow("a", now); !ok {
			t.Fatalf("request %d refused", i+1)
		}
	}
	ok, wait := l.allow("a", now.Add(20*time.Second))
	if ok || wait != 40*time.Second {
		t.Fatalf("4th request: ok=%v wait=%v, want refused for 40s", ok, wait)
	}
	if ok, _ := l.allow("b", now); !ok {
		t.Fatal("another client was refused")
	}
	if ok, _ := l.allow("a", now.Add(time.Minute)); !ok {
		t.Fatal("still refused after the window reset")
	}
}
