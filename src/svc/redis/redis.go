package redis

import (
	"context"
	"time"

	"github.com/SevenTV/Common/redis"
	"github.com/seventv/twitch-edge/src/instance"

	rawRedis "github.com/go-redis/redis/v8"
)

type redisInst struct {
	redis.Instance
}

func (r *redisInst) Pipeline(ctx context.Context) rawRedis.Pipeliner {
	return r.Instance.RawClient().Pipeline()
}

func (r *redisInst) Publish(ctx context.Context, channel string, value string) error {
	return r.Instance.RawClient().Publish(ctx, channel, value).Err()
}

func (r *redisInst) RateLimitNewConnection(ctx context.Context) (bool, time.Duration, error) {
	pipe := r.Instance.RawClient().Pipeline()
	incrCmd := pipe.Incr(ctx, RedisPrefix+"rates:connection")
	ttlCmd := pipe.TTL(ctx, RedisPrefix+"rates:connection")
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, err
	}

	ttl := ttlCmd.Val()

	if ttl == -1 {
		err := r.Instance.RawClient().Expire(ctx, RedisPrefix+"rates:connection", 10*time.Second).Err()
		if err != nil {
			return false, 0, err
		}
		ttl = time.Second * 10
	}

	return incrCmd.Val() < 150, ttl, nil
}

func (r *redisInst) RateLimitJoin(ctx context.Context, count int) (int, time.Duration, error) {
	pipe := r.Instance.RawClient().Pipeline()
	incrCmd := pipe.IncrBy(ctx, RedisPrefix+"rates:join", int64(count))
	ttlCmd := pipe.TTL(ctx, RedisPrefix+"rates:join")
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, 0, err
	}

	ttl := ttlCmd.Val()

	if ttl == -1 {
		err := r.Instance.RawClient().Expire(ctx, RedisPrefix+"rates:join", 10*time.Second).Err()
		if err != nil {
			return 0, 0, err
		}
		ttl = time.Second * 10
	}

	v := int(incrCmd.Val()) - 1500
	if v < 0 {
		return count, ttl, nil
	}

	if v > count {
		return 0, ttl, nil
	}

	return count - v, ttl, nil
}

const RedisPrefix = "twitch-chat:"

func WrapRedis(redis redis.Instance) instance.Redis {
	return &redisInst{Instance: redis}
}
