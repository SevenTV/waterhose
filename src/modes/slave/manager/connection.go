package manager

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/SevenTV/Common/sync_map"
	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
	"github.com/seventv/waterhose/src/global"
	"github.com/seventv/waterhose/src/modes/slave/irc"
	"go.uber.org/zap"
)

type ChannelState string

const (
	ChannelStateWaitingLimits ChannelState = "WAITING_LIMITS"
	ChannelStateJoinRequested ChannelState = "JOIN_REQUESTED"
	ChannelStateJoined        ChannelState = "JOINED"
	ChannelStateParted        ChannelState = "PARTED"
	ChannelStateSuspended     ChannelState = "SUSPENDED"
	ChannelStateBanned        ChannelState = "BANNED"
	ChannelStateUnknown       ChannelState = "UNKNOWN"
)

type Channel struct {
	State     ChannelState
	LastEvent time.Time
	Raw       *pb.Channel
}

func (c *Channel) Update(state ...ChannelState) *Channel {
	if len(state) == 1 {
		c.State = state[0]
	}
	c.LastEvent = time.Now()
	return c
}

type Connection struct {
	gCtx global.Context

	manager *Manager

	channels sync_map.Map[string, *Channel]
	length   *int64

	idx      uint32
	anon     bool
	username string

	client *irc.Client
}

type ConnectionOptions struct {
	Username string
	OAuth    string
}

func newConnection(gCtx global.Context, manager *Manager, options ConnectionOptions, openConnLimiter func()) *Connection {
	conn := &Connection{
		gCtx:     gCtx,
		manager:  manager,
		username: options.Username,
		client:   irc.New(options.Username, options.OAuth, openConnLimiter),
		length:   utils.PointerOf(int64(0)),
	}
	conn.client.SetOnConnect(conn.onConnect)
	conn.client.SetOnReconnect(conn.onReconnect)
	conn.client.SetOnMessage(conn.onMessage)

	go func() {
		tick := time.NewTicker(time.Minute + utils.JitterTime(time.Minute, time.Minute*5))
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
			case <-gCtx.Done():
				return
			}

			conn.channels.Range(func(key string, v *Channel) bool {
				switch v.State {
				case ChannelStateJoined, ChannelStateSuspended:
					if v.LastEvent.Before(time.Now().Add(-time.Hour * 24)) {
						zap.S().Debugw("joined channel dead",
							"channel_id", v.Raw.Id,
							"channel_login", v.Raw.Login,
							"last_event", v.LastEvent,
							"conn_id", conn.idx,
							"anon", conn.anon,
						)
						v.Update(ChannelStateUnknown)
						conn.JoinChannel(v.Raw)
					}
				case ChannelStateJoinRequested:
					if v.LastEvent.Before(time.Now().Add(-time.Minute * 20)) {
						zap.S().Debugw("join request timed out",
							"channel_id", v.Raw.Id,
							"channel_login", v.Raw.Login,
							"last_event", v.LastEvent,
							"conn_id", conn.idx,
							"anon", conn.anon,
						)
						if err := conn.manager.ep(gCtx, &pb.PublishSlaveChannelEventRequest{
							Channel: v.Raw,
							Type:    pb.PublishSlaveChannelEventRequest_EVENT_TYPE_UNKNOWN_CHANNEL,
						}); err != nil {
							zap.S().Errorw("failed to publish event to master",
								"error", err,
							)
						}
						conn.channels.Delete(key)
					}
				}
				return true
			})
		}
	}()

	go func() {
		for {
			if err := conn.getNewConnection(gCtx); err != nil {
				if gCtx.Err() != nil {
					return
				}

				zap.S().Errorw("redis",
					"error", err,
					"conn_id", conn.idx,
					"anon", conn.anon,
				)
				time.Sleep(utils.JitterTime(time.Second, time.Second*5))
			}

			err := conn.client.Connect(gCtx)
			if err != nil {
				if gCtx.Err() != nil {
					return
				}

				zap.S().Errorw("twitch client disconnected",
					"erorr", err,
					"conn_id", conn.idx,
					"anon", conn.anon,
				)
				time.Sleep(utils.JitterTime(time.Second, time.Second*5))
			}
		}
	}()

	return conn
}

func (c *Connection) ConnLength() int {
	return int(atomic.LoadInt64(c.length))
}

func (c *Connection) getNewConnection(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		allowed, ttl, err := c.gCtx.Inst().Redis.RateLimitNewConnection(ctx)
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
		time.Sleep(ttl + utils.JitterTime(time.Millisecond*100, time.Second))
	}
}

func (c *Connection) onConnect() {
	zap.S().Debugw("twitch client connected",
		"idx", c.idx,
		"anon", c.anon,
	)
	c.channels.Range(func(key string, value *Channel) bool {
		value.Update(ChannelStateUnknown)
		c.JoinChannel(value.Raw)
		return true
	})
}

func (c *Connection) onReconnect() {
	zap.S().Debugw("twitch client reconnected",
		"idx", c.idx,
		"anon", c.anon,
	)

	for {
		if err := c.getNewConnection(c.gCtx); err != nil {
			if c.gCtx.Err() != nil {
				return
			}

			zap.S().Errorw("redis",
				"error", err,
				"conn_id", c.idx,
			)
			time.Sleep(utils.JitterTime(time.Second, time.Second*5))
			continue
		}

		return
	}
}

func (c *Connection) onMessage(m irc.Message) {
	channel, ok := c.channels.Load(strings.TrimLeft(m.Channel, "#"))
	if !ok {
		return
	}

	channel.Update()
	switch m.Type {
	case irc.MessageTypeRoomState:
		if channel.State != ChannelStateJoined {
			if channel.Raw.BotBanned && !c.anon {
				channel.Raw.BotBanned = false
			}

			if err := c.manager.ep(c.gCtx, &pb.PublishSlaveChannelEventRequest{
				Channel: channel.Raw,
				Type:    pb.PublishSlaveChannelEventRequest_EVENT_TYPE_JOINED,
			}); err != nil {
				zap.S().Errorw("failed to publish event to master",
					"error", err,
				)
			}
		}
		channel.Update(ChannelStateJoined)
	case irc.MessageTypeJoin:
		if m.User == c.username || m.User == "justinfan123123" {
			if channel.State != ChannelStateJoined {
				if channel.Raw.BotBanned && !c.anon {
					channel.Raw.BotBanned = false
				}

				if err := c.manager.ep(c.gCtx, &pb.PublishSlaveChannelEventRequest{
					Channel: channel.Raw,
					Type:    pb.PublishSlaveChannelEventRequest_EVENT_TYPE_JOINED,
				}); err != nil {
					zap.S().Errorw("failed to publish event to master",
						"error", err,
					)
				}
			}
			channel.Update(ChannelStateJoined)
		}
	case irc.MessageTypePart:
		if m.User == c.username {
			channel.Update(ChannelStateParted)
			c.JoinChannel(channel.Raw)
		}
	case irc.MessageTypeNotice:
		if m.Tags.ChannelSuspended() {
			channel.Update(ChannelStateSuspended)
			if err := c.manager.ep(c.gCtx, &pb.PublishSlaveChannelEventRequest{
				Channel: channel.Raw,
				Type:    pb.PublishSlaveChannelEventRequest_EVENT_TYPE_SUSPENDED_CHANNEL,
			}); err != nil {
				zap.S().Errorw("failed to publish event to master",
					"error", err,
				)
			}
			return
		}

		if m.Tags.Banned() {
			if err := c.manager.ep(c.gCtx, &pb.PublishSlaveChannelEventRequest{
				Channel: channel.Raw,
				Type:    pb.PublishSlaveChannelEventRequest_EVENT_TYPE_BOT_BANNED,
			}); err != nil {
				zap.S().Errorw("failed to publish event to master",
					"error", err,
				)
			}
			channel.Raw.BotBanned = true
			channel.Raw.UseAnonymous = true
			channel.Update(ChannelStateBanned)
			go c.manager.JoinChat(channel.Raw)
			return
		}
	}

	pipe := c.gCtx.Inst().Redis.Pipeline(c.gCtx)
	pipe.Publish(c.gCtx, "twitch-irc-chat-messages:ALL:GLOBAL", m.Raw)
	pipe.Publish(c.gCtx, "twitch-irc-chat-messages:ALL:"+channel.Raw.Id, m.Raw)
	pipe.Publish(c.gCtx, fmt.Sprintf("twitch-irc-chat-messages:%s:GLOBAL", m.Type), m.Raw)
	pipe.Publish(c.gCtx, fmt.Sprintf("twitch-irc-chat-messages:%s:%s", m.Type, channel.Raw.Id), m.Raw)
	if _, err := pipe.Exec(c.gCtx); err != nil {
		zap.S().Errorw("failed to publish twitch message",
			"error", err,
		)
	}
}

func (c *Connection) JoinChannel(channel *pb.Channel) {
	ch, ok := c.channels.LoadOrStore(channel.GetLogin(), &Channel{
		Raw:       channel,
		State:     ChannelStateWaitingLimits,
		LastEvent: time.Now(),
	})
	if ok {
		ch.Raw = channel
		if ch.State == ChannelStateJoined {
			if err := c.manager.ep(c.gCtx, &pb.PublishSlaveChannelEventRequest{
				Channel: channel,
				Type:    pb.PublishSlaveChannelEventRequest_EVENT_TYPE_JOINED,
			}); err != nil {
				zap.S().Errorw("failed to publish event to master",
					"error", err,
				)
			}
			return
		}
	} else {
		atomic.AddInt64(c.length, 1)
	}

	ch.Update()
	if !c.client.IsConnected() {
		return
	}

	go func() {
		if !c.anon {
			ch.Update(ChannelStateWaitingLimits)
			if err := c.manager.rl(c.gCtx, channel); err != nil {
				zap.S().Errorw("failed to get rates on channel",
					"error", err,
				)
				return
			}
		}

		ch.Update(ChannelStateJoinRequested)
		if err := c.client.Write("JOIN #" + channel.GetLogin()); err != nil {
			zap.S().Warn("client not connected")
			ch.Update(ChannelStateParted)
		}
	}()
}

func (c *Connection) PartChannel(channel string) {
	atomic.AddInt64(c.length, -1)
	c.channels.Delete(channel)

	if !c.client.IsConnected() {
		return
	}

	if err := c.client.Write("PART #" + channel); err != nil {
		zap.S().Warn("client not connected")
	}
}
