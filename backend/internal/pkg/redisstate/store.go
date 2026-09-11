// Package redisstate provides an atomic compare-and-swap store for account state.
package redisstate

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

type Store struct{ client redis.UniversalClient }

func New(client redis.UniversalClient) *Store { return &Store{client: client} }

func (s *Store) Get(ctx context.Context, key string) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("account state requires Redis")
	}
	value, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return value, err
}

// State expiration belongs to the coordinator. In particular a stopped account
// must never become available merely because a cache key expired.
var compareAndSwap = redis.NewScript(`
local old = redis.call('GET', KEYS[1]) or ''
if old ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[1], ARGV[2])
return 1
`)

func (s *Store) CompareAndSwap(ctx context.Context, key, old, next string) (bool, error) {
	if s == nil || s.client == nil {
		return false, errors.New("account state requires Redis")
	}
	result, err := compareAndSwap.Run(ctx, s.client, []string{key}, old, next).Int()
	return result == 1, err
}
