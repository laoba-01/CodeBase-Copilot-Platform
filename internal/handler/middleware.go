package handler

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codebase-copilot/core/internal/redis"
)

// ── Body Limit ──

// BodyLimit returns middleware that limits request body size.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// ── Rate Limiting ──

// redisLimiter is the optional Redis-backed rate limiter for distributed deployments.
// When set, rate limits are shared across all replicas. When nil (default), the
// in-memory token-bucket fallback is used.
var redisLimiter *redis.RateLimiter

// SetRedisRateLimiter configures the Redis-backed rate limiter.
// Call once at startup before serving requests.
func SetRedisRateLimiter(rl *redis.RateLimiter) {
	redisLimiter = rl
}

// RateLimit returns middleware that rate-limits requests.
// Uses Redis sliding-window when available, otherwise falls back to in-memory token-bucket.
func RateLimit(rate, burst int) gin.HandlerFunc {
	if redisLimiter != nil {
		return redisRateLimit(rate, burst)
	}
	return memoryRateLimit(rate, burst)
}

// StrictRateLimit returns a rate limiter with lower burst for expensive endpoints.
func StrictRateLimit(rate, burst int) gin.HandlerFunc {
	if redisLimiter != nil {
		return redisRateLimit(rate, burst)
	}
	return memoryRateLimit(rate, burst)
}

// ── Redis-backed sliding-window rate limiter ──

func redisRateLimit(rate, burst int) gin.HandlerFunc {
	// rate = max requests, burst = not used in sliding-window (stays compatible)
	// Window is 1 second — so rate = max requests per second
	window := 1 * time.Second
	// Use burst as the max requests in the window
	maxRequests := burst

	return func(c *gin.Context) {
		allowed, err := redisLimiter.Allow(
			c.Request.Context(),
			"api",
			c.ClientIP(),
			maxRequests,
			window,
		)
		if err != nil {
			// Log and allow on Redis errors (fail-open)
			log.Printf("WARNING: redis rate limit error: %v", err)
			c.Next()
			return
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

// ── In-memory token-bucket rate limiter (fallback / single-instance) ──

// memoryRateLimiter is a simple per-IP token-bucket rate limiter.
type memoryRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    int           // tokens per second
	burst   int           // max burst
	cleanup time.Duration // cleanup interval
}

type bucket struct {
	tokens   float64
	lastTime time.Time
}

func newMemoryRateLimiter(rate, burst int) *memoryRateLimiter {
	rl := &memoryRateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		cleanup: 5 * time.Minute,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *memoryRateLimiter) cleanupLoop() {
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

// allow checks if a request from the given key is allowed.
func (rl *memoryRateLimiter) allow(key string) bool {
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

// memoryRateLimit returns middleware that uses in-memory token-bucket rate limiting.
func memoryRateLimit(rate, burst int) gin.HandlerFunc {
	rl := newMemoryRateLimiter(rate, burst)
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !rl.allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
