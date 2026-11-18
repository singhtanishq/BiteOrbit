package httpx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter implements RateLimiter with a fixed-window INCR+EXPIRE.
// Used when REDIS_URL is configured so multiple API instances share limits;
// falls back to the in-memory limiter otherwise.
type RedisRateLimiter struct {
	client *redis.Client
}

// NewRedisRateLimiter verifies connectivity and returns a limiter, or an
// error so the caller can fall back to in-memory.
func NewRedisRateLimiter(ctx context.Context, redisURL string) (*RedisRateLimiter, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	opts.DialTimeout = 3 * time.Second
	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisRateLimiter{client: client}, nil
}

// Allow implements RateLimiter.
func (r *RedisRateLimiter) Allow(key string, limit int, window time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	windowKey := fmt.Sprintf("rl:%s:%d", key, time.Now().Unix()/int64(window.Seconds()))
	count, err := r.client.Incr(ctx, windowKey).Result()
	if err != nil {
		// Redis unavailable: fail open for availability, log-free by design
		// (the API logger wraps calls). Callers treat true as allowed.
		return true
	}
	if count == 1 {
		r.client.Expire(ctx, windowKey, window+time.Second)
	}
	return count <= int64(limit)
}

func (r *RedisRateLimiter) Close() error { return r.client.Close() }
