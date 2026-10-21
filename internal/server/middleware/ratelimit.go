package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"todoapp/internal/models"
	"todoapp/internal/server/httpx"
)

// KeyedLimiter is a set of token-bucket limiters keyed by an arbitrary
// string (client IP, username, ...). Idle buckets are evicted lazily.
type KeyedLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*bucket
	limit       rate.Limit
	burst       int
	idleTTL     time.Duration
	lastCleanup time.Time
	now         func() time.Time
}

type bucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// NewKeyedLimiter allows perMinute events per key with the given burst.
func NewKeyedLimiter(perMinute float64, burst int) *KeyedLimiter {
	return &KeyedLimiter{
		buckets: map[string]*bucket{},
		limit:   rate.Limit(perMinute / 60),
		burst:   burst,
		idleTTL: 10 * time.Minute,
		now:     time.Now,
	}
}

// Allow consumes one token for key. When the bucket is empty it returns
// false and how long the caller should wait.
func (k *KeyedLimiter) Allow(key string) (bool, time.Duration) {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()

	if now.Sub(k.lastCleanup) > k.idleTTL {
		for key, b := range k.buckets {
			if now.Sub(b.lastSeen) > k.idleTTL {
				delete(k.buckets, key)
			}
		}
		k.lastCleanup = now
	}

	b, ok := k.buckets[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(k.limit, k.burst)}
		k.buckets[key] = b
	}
	b.lastSeen = now

	res := b.lim.ReserveN(now, 1)
	if !res.OK() {
		return false, time.Minute
	}
	if d := res.DelayFrom(now); d > 0 {
		res.CancelAt(now)
		return false, d
	}
	return true, 0
}

// WriteRateLimited writes a 429 with a Retry-After header.
func WriteRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	secs := int(math.Ceil(retryAfter.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	httpx.WriteError(w, http.StatusTooManyRequests, models.CodeRateLimited,
		"too many attempts, retry in "+strconv.Itoa(secs)+"s")
}

// RateLimitByIP limits requests per client IP. When the server runs behind
// a trusted proxy, chi's RealIP middleware must run first so RemoteAddr is
// the real client address.
func RateLimitByIP(l *KeyedLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ok, wait := l.Allow(ClientIP(r)); !ok {
				WriteRateLimited(w, wait)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP returns the host part of r.RemoteAddr.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // RealIP may set a bare IP
	}
	return host
}
