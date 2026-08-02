package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps a Redis connection for rate limiting and caching.
type Client struct {
	rdb *goredis.Client
}

// NewClient creates a new Redis client and verifies connectivity.
func NewClient(ctx context.Context, redisURL string) (*Client, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	rdb := goredis.NewClient(opts)

	// Verify connectivity with retry
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := rdb.Ping(ctx).Err(); err != nil {
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("redis ping after %d attempts: %w", maxRetries, err)
			}
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
			case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			}
			continue
		}
		break
	}

	return &Client{rdb: rdb}, nil
}

// Close releases the Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// Ping checks Redis connectivity for health checks.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Conn returns the underlying go-redis client for use with other Redis utilities.
func (c *Client) Conn() *goredis.Client {
	return c.rdb
}
