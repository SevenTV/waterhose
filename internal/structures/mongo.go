package structures

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ChannelState string

type Channel struct {
	ID           primitive.ObjectID `bson:"_id"`
	TwitchID     string             `bson:"twitch_id"`
	TwitchLogin  string             `bson:"twitch_login"`
	Priority     int32              `bson:"priority"`
	EdgeNode     int32              `bson:"edge_node"`
	LastUpdated  time.Time          `bson:"last_updated"`
	State        ChannelState       `bson:"state"`
	UseAnonymous bool               `bson:"use_anonymous"`
	BotBanned    bool               `bson:"bot_banned"`
}

const (
	ChannelStateSuspended ChannelState = "SUSPENDED"
	ChannelStateJoined    ChannelState = "JOINED"
	ChannelStateUnknown   ChannelState = "UNKNOWN"
)
