package instance

import pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_edge/v1"

type AutoScaler interface {
	AllocateChannels(chnanels []*pb.Channel) error
	Load() error
}
