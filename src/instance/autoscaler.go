package instance

import (
	pb "github.com/seventv/twitch-edge/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-edge/src/structures"
)

type AutoScaler interface {
	AllocateChannels(channels []*pb.Channel) error
	GetChannelsForEdge(idx int) []structures.Channel
	Load() error
}
