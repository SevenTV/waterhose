package instance

import (
	"context"
	"time"

	"github.com/SevenTV/Common/redis"
	rawRedis "github.com/go-redis/redis/v8"
)

type Redis interface {
	Pipeline(ctx context.Context) rawRedis.Pipeliner
	Publish(ctx context.Context, channel string, value string) error

	RateLimitNewConnection(ctx context.Context) (bool, time.Duration, error)
	RateLimitJoin(ctx context.Context, count int) (int, time.Duration, error)

	redis.Instance
}
