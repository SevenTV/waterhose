package instance

import (
	pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-chat-controller/src/structures"
)

type AutoScaler interface {
	AllocateChannels(channels []*pb.Channel) error
	GetChannelsForEdge(idx int) []structures.Channel
	Load() error
}
