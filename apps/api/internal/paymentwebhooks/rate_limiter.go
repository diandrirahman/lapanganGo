package paymentwebhooks

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type tokenBucket struct {
	tokens  float64
	updated time.Time
}

const maxRateLimiterBuckets = 10_000

type RateLimiter struct {
	mu        sync.Mutex
	perMinute int
	burst     int
	window    time.Duration
	buckets   map[string]tokenBucket
	now       Clock
	lastSweep time.Time
}

func NewRateLimiter(perMinute, burst int, window time.Duration) *RateLimiter {
	return &RateLimiter{perMinute: perMinute, burst: burst, window: window, buckets: make(map[string]tokenBucket), now: time.Now}
}

func (l *RateLimiter) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.allow(c.FullPath()) {
			respond(c, http.StatusTooManyRequests, RateLimitedCategory, c.GetString("payment_webhook_correlation_id"))
			c.Abort()
			return
		}
		c.Next()
	}
}

func (l *RateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= l.window {
		for candidate, candidateBucket := range l.buckets {
			if now.Sub(candidateBucket.updated) >= l.window {
				delete(l.buckets, candidate)
			}
		}
		l.lastSweep = now
	}
	bucket, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxRateLimiterBuckets {
			return false
		}
		bucket = tokenBucket{tokens: float64(l.burst), updated: now}
	}
	if elapsed := now.Sub(bucket.updated); elapsed > 0 {
		bucket.tokens = minFloat(float64(l.burst), bucket.tokens+elapsed.Seconds()*float64(l.perMinute)/l.window.Seconds())
		bucket.updated = now
	}
	if bucket.tokens < 1 {
		l.buckets[key] = bucket
		return false
	}
	bucket.tokens--
	l.buckets[key] = bucket
	return true
}
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
