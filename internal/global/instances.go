package global

import (
	"github.com/SevenTV/Common/mongo"
	"github.com/seventv/waterhose/internal/instance"
	"github.com/seventv/waterhose/internal/svc/events"
)

type Instances struct {
	Redis        instance.Redis
	K8S          instance.K8S
	EventEmitter *events.EventEmitter
	Twitch       instance.Twitch
	Mongo        mongo.Instance
	AutoScaler   instance.AutoScaler
	RateLimit    instance.RateLimiter
	Monitoring   instance.Monitoring
}
