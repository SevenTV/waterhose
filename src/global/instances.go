package global

import (
	"github.com/SevenTV/Common/mongo"
	"github.com/seventv/twitch-chat-controller/src/instance"
	"github.com/seventv/twitch-chat-controller/src/svc/events"
)

type Instances struct {
	Redis      instance.Redis
	K8S        instance.K8S
	Events     *events.Events
	Twitch     instance.Twitch
	Mongo      mongo.Instance
	AutoScaler instance.AutoScaler
	RateLimit  instance.RateLimiter
}
