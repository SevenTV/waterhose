all: build_deps build

deps: build_deps

build_deps:
	go install github.com/seventv/dataloaden@cc5ac4900

compile:
	dataloaden TwitchUserLoader "string" "github.com/nicklaw5/helix.User"
	dataloaden AllocateChannel "*github.com/seventv/twitch-edge/protobuf/twitch_edge/v1.Channel" "string"
