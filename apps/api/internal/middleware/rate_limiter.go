package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type clientInfo struct {
	count       int
	windowStart time.Time
}

type MemoryRateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientInfo
	limit   int
	window  time.Duration
	prefix  string
}

func newMemoryRateLimiter(prefix string, limit int, window time.Duration) *MemoryRateLimiter {
	rl := &MemoryRateLimiter{
		clients: make(map[string]*clientInfo),
		limit:   limit,
		window:  window,
		prefix:  prefix,
	}

	go rl.cleanup()
	return rl
}

func (rl *MemoryRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, info := range rl.clients {
			if now.Sub(info.windowStart) >= rl.window {
				delete(rl.clients, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *MemoryRateLimiter) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		key := rl.prefix + ":" + ip

		rl.mu.Lock()
		info, exists := rl.clients[key]

		if !exists || now.Sub(info.windowStart) >= rl.window {
			rl.clients[key] = &clientInfo{
				count:       1,
				windowStart: now,
			}
			rl.mu.Unlock()
			c.Next()
			return
		} else {
			info.count++
			if info.count > rl.limit {
				rl.mu.Unlock()
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": "Too many requests",
				})
				return
			}
		}
		rl.mu.Unlock()

		c.Next()
	}
}

type RedisRateLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
	prefix string
}

func (rl *RedisRateLimiter) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("rate_limit:%s:%s", rl.prefix, ip)

		ctx := context.Background()

		// Increment the counter
		incr := rl.client.Incr(ctx, key)
		if err := incr.Err(); err != nil {
			log.Printf("Redis rate limiter error: %v", err)
			c.Next()
			return
		}

		if incr.Val() == 1 {
			rl.client.Expire(ctx, key, rl.window)
		}

		if incr.Val() > int64(rl.limit) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests",
			})
			return
		}

		c.Next()
	}
}

func NewRateLimiter(redisURL string, prefix string, limit int, window time.Duration) gin.HandlerFunc {
	if redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err == nil {
			client := redis.NewClient(opts)
			if err := client.Ping(context.Background()).Err(); err == nil {
				log.Printf("Using Redis rate limiter [%s] (limit: %d, window: %v)", prefix, limit, window)
				return (&RedisRateLimiter{
					client: client,
					limit:  limit,
					window: window,
					prefix: prefix,
				}).Handle()
			} else {
				log.Printf("Redis connection failed, falling back to memory rate limiter: %v", err)
			}
		} else {
			log.Printf("Invalid Redis URL, falling back to memory rate limiter: %v", err)
		}
	}

	log.Printf("Using Memory rate limiter [%s] (limit: %d, window: %v)", prefix, limit, window)
	return newMemoryRateLimiter(prefix, limit, window).Handle()
}
