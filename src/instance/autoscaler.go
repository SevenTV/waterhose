package instance

import (
	pb "github.com/seventv/waterhose/protobuf/twitch_edge/v1"
	"github.com/seventv/waterhose/src/structures"
)

type AutoScaler interface {
	AllocateChannels(channels []*pb.Channel) error
	GetChannelsForEdge(idx int) []structures.Channel
	Load() error
}
