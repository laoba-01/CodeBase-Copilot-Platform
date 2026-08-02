package db

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 1 * time.Hour
	cfg.MaxConnIdleTime = 10 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	// Retry connection with exponential backoff (up to 5 attempts)
	var pool *pgxpool.Pool
	maxRetries := 5
	baseDelay := 500 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("connect db after %d attempts: %w", maxRetries, err)
			}
			delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
			fmt.Printf("WARNING: db connection attempt %d/%d failed: %v (retrying in %v)\n",
				attempt+1, maxRetries, err, delay)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled while connecting to db: %w", ctx.Err())
			case <-time.After(delay):
			}
			continue
		}

		// Verify connection works
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("ping db after %d attempts: %w", maxRetries, err)
			}
			delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
			fmt.Printf("WARNING: db ping attempt %d/%d failed: %v (retrying in %v)\n",
				attempt+1, maxRetries, err, delay)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled while pinging db: %w", ctx.Err())
			case <-time.After(delay):
			}
			continue
		}

		// Success
		return pool, nil
	}

	return nil, fmt.Errorf("connect db: exhausted all %d retries", maxRetries)
}

// RecoverStuckRepos resets repos stuck in transient states back to error on startup.
func RecoverStuckRepos(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		UPDATE repos SET status = 'error'
		WHERE status IN ('pending', 'cloning', 'indexing')
	`)
	if err != nil {
		return fmt.Errorf("recover stuck repos: %w", err)
	}
	return nil
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}

	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("exec %s: %w", f, err)
		}
		fmt.Printf("migration applied: %s\n", filepath.Base(f))
	}
	return nil
}
