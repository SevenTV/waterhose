package instance

import (
	"github.com/seventv/waterhose/internal/structures"
	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
)

type AutoScaler interface {
	AllocateChannels(channels []*pb.Channel) error
	GetChannelsForSlave(idx int) []structures.Channel
	Load() error
}
