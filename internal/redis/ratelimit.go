package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RateLimiter implements a sliding-window rate limiter backed by Redis sorted sets.
// It is safe for multi-instance deployments — all instances share the same counters.
type RateLimiter struct {
	rdb *goredis.Client
}

// NewRateLimiter creates a Redis-backed rate limiter.
func NewRateLimiter(rdb *goredis.Client) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

// Allow checks whether a request identified by key is allowed under the given
// rate (maxRequests per window). Uses a sliding window with Redis sorted sets.
//
// Algorithm:
//  1. Remove entries older than the window
//  2. Count entries remaining in the window
//  3. If count >= maxRequests, deny
//  4. Otherwise, add current request and allow
func (rl *RateLimiter) Allow(ctx context.Context, keyPrefix string, key string, maxRequests int, window time.Duration) (bool, error) {
	redisKey := fmt.Sprintf("ratelimit:%s:%s", keyPrefix, key)
	now := time.Now().UnixNano()
	windowStart := now - window.Nanoseconds()

	pipe := rl.rdb.Pipeline()

	// Remove expired entries (older than window)
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart))

	// Count current entries in window
	countCmd := pipe.ZCard(ctx, redisKey)

	// Add current request timestamp as score
	member := goredis.Z{
		Score:  float64(now),
		Member: fmt.Sprintf("%d", now),
	}
	pipe.ZAdd(ctx, redisKey, member)

	// Set expiry to prevent memory leak (window * 2 for safety margin)
	pipe.Expire(ctx, redisKey, window*2)

	_, err := pipe.Exec(ctx)
	if err != nil {
		// On Redis error, allow the request (fail-open for availability)
		return true, fmt.Errorf("redis rate limit: %w", err)
	}

	count, _ := countCmd.Val(), countCmd.Err()
	return count < int64(maxRequests), nil
}
