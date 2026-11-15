package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Sign-in attempts allowed per client IP. Generous enough for a studio's
// shared wifi, low enough that one client can't flood oauth_states.
const (
	authRateLimit  = 30
	authRateWindow = time.Minute
)

// rateLimiter is a fixed-window counter per key, kept in memory (one process).
type rateLimiter struct {
	limit  int
	window time.Duration

	mu      sync.Mutex
	buckets map[string]*rateBucket
}

type rateBucket struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, buckets: map[string]*rateBucket{}}
}

// maxRateBuckets bounds memory: past this, finished windows are dropped.
const maxRateBuckets = 10000

// allow counts one request for key and reports whether it is within the
// limit, plus how long until the window resets when it isn't.
func (l *rateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[key]
	if b == nil || now.Sub(b.start) >= l.window {
		if len(l.buckets) >= maxRateBuckets {
			for k, old := range l.buckets {
				if now.Sub(old.start) >= l.window {
					delete(l.buckets, k)
				}
			}
			if len(l.buckets) >= maxRateBuckets {
				// Still full of live windows (a very wide flood): refuse new clients briefly.
				return false, l.window
			}
		}
		b = &rateBucket{start: now}
		l.buckets[key] = b
	}
	b.count++
	if b.count > l.limit {
		return false, l.window - now.Sub(b.start)
	}
	return true, 0
}

func (l *rateLimiter) middleware(s *Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, wait := l.allow(s.clientIP(r), s.now())
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
				s.writeError(w, r, http.StatusTooManyRequests, "too many sign-in attempts, try again in a minute", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
