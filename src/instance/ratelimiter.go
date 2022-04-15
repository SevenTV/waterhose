package instance

import (
	"context"

	pb "github.com/seventv/twitch-edge/protobuf/twitch_edge/v1"
)

type RateLimiter interface {
	RequestJoin(ctx context.Context, channel *pb.Channel) error
	Start()
}
