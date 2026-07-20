package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	asyncImageReadyKey       = "async_image:queue:ready"
	asyncImageDelayedKey     = "async_image:queue:delayed"
	asyncImageActiveKey      = "async_image:queue:active"
	asyncImageInflightPrefix = "async_image:queue:inflight:"
	asyncImageInflightTTL    = 48 * time.Hour
)

var asyncImageEnqueueScript = redis.NewScript(`
if redis.call("SET", KEYS[1], ARGV[1], "NX", "PX", ARGV[2]) then
  redis.call("LPUSH", KEYS[2], ARGV[1])
  return 1
end
return 0
`)

var asyncImageReserveScript = redis.NewScript(`
local task = redis.call("RPOP", KEYS[1])
if not task then return nil end
redis.call("ZADD", KEYS[2], ARGV[1], task)
return task
`)

var asyncImageAckScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
redis.call("DEL", KEYS[2])
return 1
`)

var asyncImageRequeueScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
redis.call("ZADD", KEYS[2], ARGV[2], ARGV[1])
redis.call("PEXPIRE", KEYS[3], ARGV[3])
return 1
`)

var asyncImageMoveDelayedScript = redis.NewScript(`
local tasks = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", ARGV[1], "LIMIT", 0, ARGV[2])
for _, task in ipairs(tasks) do
  redis.call("ZREM", KEYS[1], task)
  redis.call("LPUSH", KEYS[2], task)
end
return #tasks
`)

var asyncImageRecoverActiveScript = redis.NewScript(`
local tasks = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", ARGV[1], "LIMIT", 0, ARGV[2])
for _, task in ipairs(tasks) do
  redis.call("ZREM", KEYS[1], task)
  redis.call("LPUSH", KEYS[2], task)
end
return #tasks
`)

type asyncImageQueue struct {
	rdb *redis.Client
}

func NewAsyncImageQueue(rdb *redis.Client) service.AsyncImageQueue {
	return &asyncImageQueue{rdb: rdb}
}

func (q *asyncImageQueue) Enqueue(ctx context.Context, taskID string) error {
	if q == nil || q.rdb == nil || !service.IsValidAsyncImageTaskID(taskID) {
		return service.ErrAsyncImageQueueBadPayload
	}
	applied, err := asyncImageEnqueueScript.Run(ctx, q.rdb,
		[]string{asyncImageInflightPrefix + taskID, asyncImageReadyKey},
		taskID, asyncImageInflightTTL.Milliseconds(),
	).Int()
	if err != nil {
		return err
	}
	if applied == 0 {
		return service.ErrAsyncImageAlreadyQueued
	}
	return nil
}

func (q *asyncImageQueue) Reserve(ctx context.Context, blockTimeout time.Duration) (string, error) {
	if q == nil || q.rdb == nil {
		return "", service.ErrAsyncImageQueueBadPayload
	}
	deadline := time.Now().Add(blockTimeout)
	for {
		raw, err := asyncImageReserveScript.Run(ctx, q.rdb,
			[]string{asyncImageReadyKey, asyncImageActiveKey}, time.Now().UnixMilli(),
		).Result()
		if err == nil {
			taskID, ok := raw.(string)
			if !ok || !service.IsValidAsyncImageTaskID(taskID) {
				return "", service.ErrAsyncImageQueueBadPayload
			}
			return taskID, nil
		}
		if !errors.Is(err, redis.Nil) {
			return "", err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", service.ErrAsyncImageQueueEmpty
		}
		wait := time.Second
		if remaining < wait {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func (q *asyncImageQueue) Ack(ctx context.Context, taskID string) error {
	if q == nil || q.rdb == nil || !service.IsValidAsyncImageTaskID(taskID) {
		return service.ErrAsyncImageQueueBadPayload
	}
	return asyncImageAckScript.Run(ctx, q.rdb,
		[]string{asyncImageActiveKey, asyncImageInflightPrefix + taskID}, taskID,
	).Err()
}

func (q *asyncImageQueue) Heartbeat(ctx context.Context, taskID string) error {
	if q == nil || q.rdb == nil || !service.IsValidAsyncImageTaskID(taskID) {
		return service.ErrAsyncImageQueueBadPayload
	}
	return q.rdb.ZAdd(ctx, asyncImageActiveKey, redis.Z{Score: float64(time.Now().UnixMilli()), Member: taskID}).Err()
}

func (q *asyncImageQueue) RequeueAfter(ctx context.Context, taskID string, delay time.Duration) error {
	if q == nil || q.rdb == nil || !service.IsValidAsyncImageTaskID(taskID) {
		return service.ErrAsyncImageQueueBadPayload
	}
	if delay < 0 {
		delay = 0
	}
	return asyncImageRequeueScript.Run(ctx, q.rdb,
		[]string{asyncImageActiveKey, asyncImageDelayedKey, asyncImageInflightPrefix + taskID},
		taskID, time.Now().Add(delay).UnixMilli(), asyncImageInflightTTL.Milliseconds(),
	).Err()
}

func (q *asyncImageQueue) MoveDueDelayedToReady(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	return asyncImageMoveDelayedScript.Run(ctx, q.rdb,
		[]string{asyncImageDelayedKey, asyncImageReadyKey}, time.Now().UnixMilli(), limit,
	).Int()
}

func (q *asyncImageQueue) RecoverStaleActive(ctx context.Context, staleAfter time.Duration, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	if staleAfter <= 0 {
		staleAfter = 2 * time.Minute
	}
	return asyncImageRecoverActiveScript.Run(ctx, q.rdb,
		[]string{asyncImageActiveKey, asyncImageReadyKey}, time.Now().Add(-staleAfter).UnixMilli(), limit,
	).Int()
}

var _ service.AsyncImageQueue = (*asyncImageQueue)(nil)
