package events

import (
	"fmt"

	"github.com/SevenTV/Common/eventemitter"
	"github.com/seventv/waterhose/src/structures"
)

type EventEmitter struct {
	*eventemitter.RawEventEmitter
}

func New() *EventEmitter {
	return &EventEmitter{
		RawEventEmitter: eventemitter.New(),
	}
}

func (ev *EventEmitter) PublishSlaveChannelUpdate(idx int, channels []structures.Channel) {
	ev.PublishRaw(SlaveChannelUpdateFormat(idx), channels)
}

func (ev *EventEmitter) PublishTwitchChatLogin(accountID string) {
	ev.PublishRaw(TwitchChatLoginFormat(accountID), struct{}{})
}

func SlaveChannelUpdateFormat(idx int) string {
	return fmt.Sprintf("slave-channel-update:%d", idx)
}

func TwitchChatLoginFormat(accountID string) string {
	return fmt.Sprintf("twitch-chat-login:%s", accountID)
}
