package instance

import (
	"context"

	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
)

type RateLimiter interface {
	RequestJoin(ctx context.Context, channel *pb.Channel) error
	Start()
}
