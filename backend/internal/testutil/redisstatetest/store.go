package redisstatetest

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/redisstate"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// New runs a local protocol-compatible Redis simulator and closes its client.
func New(t *testing.T) (*redisstate.Store, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), PoolSize: 32, MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	return redisstate.New(client), server
}
