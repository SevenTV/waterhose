package events

import (
	"fmt"

	"github.com/SevenTV/Common/eventemitter"
	"github.com/seventv/twitch-chat-controller/src/structures"
)

type EventEmitter struct {
	*eventemitter.RawEventEmitter
}

func New() *EventEmitter {
	return &EventEmitter{
		RawEventEmitter: eventemitter.New(),
	}
}

func (ev *EventEmitter) PublishEdgeChannelUpdate(idx int, channels []structures.Channel) {
	ev.PublishRaw(EdgeChannelUpdateFormat(idx), channels)
}

func (ev *EventEmitter) PublishTwitchChatLogin(accountID string) {
	ev.PublishRaw(TwitchChatLoginFormat(accountID), struct{}{})
}

func EdgeChannelUpdateFormat(idx int) string {
	return fmt.Sprintf("edge-channel-update:%d", idx)
}

func TwitchChatLoginFormat(accountID string) string {
	return fmt.Sprintf("twitch-chat-login:%s", accountID)
}
