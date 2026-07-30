package handler

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// BodyLimit returns middleware that limits request body size.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// RateLimiter is a simple per-IP token-bucket rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     int           // tokens per second
	burst    int           // max burst
	cleanup  time.Duration // cleanup interval
}

type bucket struct {
	tokens   float64
	lastTime time.Time
}

// NewRateLimiter creates a rate limiter with the given rate and burst.
func NewRateLimiter(rate, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		cleanup: 5 * time.Minute,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	for {
		time.Sleep(rl.cleanup)
		rl.mu.Lock()
		now := time.Now()
		for k, b := range rl.buckets {
			if now.Sub(b.lastTime) > rl.cleanup {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{tokens: float64(rl.burst), lastTime: now}
		rl.buckets[key] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * float64(rl.rate)
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastTime = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RateLimit returns middleware that rate-limits requests.
func RateLimit(rate, burst int) gin.HandlerFunc {
	rl := NewRateLimiter(rate, burst)
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !rl.Allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

// StrictRateLimit returns a rate limiter with lower burst for expensive endpoints.
func StrictRateLimit(rate, burst int) gin.HandlerFunc {
	return RateLimit(rate, burst)
}
