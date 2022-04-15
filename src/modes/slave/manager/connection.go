package manager

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/twitch-edge/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-edge/src/global"
	"github.com/seventv/twitch-edge/src/modes/slave/irc"
	"github.com/sirupsen/logrus"
)

type ChannelState string

const (
	ChannelStateJoinRequested ChannelState = "JOIN_REQUESTED"
	ChannelStateJoined        ChannelState = "JOINED"
	ChannelStateParted        ChannelState = "PARTED"
	ChannelStateSuspended     ChannelState = "SUSPENDED"
	ChannelStateBanned        ChannelState = "BANNED"
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

	mtx      sync.Mutex
	channels map[string]*Channel
	idx      int
	username string

	client *irc.Client
}

type ConnectionOptions struct {
	Username string
	OAuth    string
}

func newConnection(gCtx global.Context, manager *Manager, options ConnectionOptions) *Connection {
	conn := &Connection{
		gCtx:     gCtx,
		manager:  manager,
		username: options.Username,
		client:   irc.New(options.Username, options.OAuth),
		channels: map[string]*Channel{},
	}
	conn.client.SetOnConnect(conn.onConnect)
	conn.client.SetOnReconnect(conn.onReconnect)
	conn.client.SetOnMessage(conn.onMessage)

	go func() {
		for {
			logrus.Infof("twitch client %d connecting", conn.idx)

			if err := conn.getNewConnection(gCtx); err != nil {
				if gCtx.Err() != nil {
					return
				}

				logrus.Errorf("redis: %e", conn.idx, err)
				time.Sleep(utils.JitterTime(time.Second, time.Second*5))
			}

			err := conn.client.Connect(gCtx)
			if err != nil {
				if gCtx.Err() != nil {
					return
				}

				logrus.Errorf("twitch client %d disconnected: %e", conn.idx, err)
				time.Sleep(utils.JitterTime(time.Second, time.Second*5))
			}
		}
	}()

	return conn
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
	logrus.WithField("idx", c.idx).Info("twitch client connected")
	// todo connect to channels :)
}

func (c *Connection) onReconnect() {
	logrus.WithField("idx", c.idx).Info("twitch client reconnect")
	time.Sleep(utils.JitterTime(time.Second*5, time.Second*10))
}

func (c *Connection) onMessage(m irc.Message) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	channel := strings.TrimLeft(m.Channel, "#")

	fmt.Println(m.Raw)

	if _, ok := c.channels[channel]; !ok {
		return
	}

	c.channels[channel].Update()
	switch m.Type {
	case irc.MessageTypeJoin:
		if m.User == c.username {
			c.channels[channel].Update(ChannelStateJoined)
		}
	case irc.MessageTypePart:
		if m.User == c.username {
			c.channels[channel].Update(ChannelStateParted)
			go c.JoinChannel(c.channels[channel].Raw)
		}
	case irc.MessageTypeNotice:
		if m.Tags.ChannelSuspended() {
			c.channels[channel].Update(ChannelStateSuspended)
			return
		}

		if m.Tags.Banned() {
			logrus.WithField("idx", c.idx).Warn("banned from channel: ", channel)
			c.channels[channel].Update(ChannelStateBanned)
			go c.manager.joinAnon(c.channels[channel].Raw)
			return
		}
	}

	pipe := c.gCtx.Inst().Redis.Pipeline(c.gCtx)
	pipe.Publish(c.gCtx, "twitch-irc-chat-messages:ALL:GLOBAL", m.Raw)
	pipe.Publish(c.gCtx, "twitch-irc-chat-messages:ALL:"+c.channels[channel].Raw.Id, m.Raw)
	pipe.Publish(c.gCtx, fmt.Sprintf("twitch-irc-chat-messages:%s:GLOBAL", m.Type), m.Raw)
	pipe.Publish(c.gCtx, fmt.Sprintf("twitch-irc-chat-messages:%s:%s", m.Type, c.channels[channel].Raw.Id), m.Raw)
	if _, err := pipe.Exec(c.gCtx); err != nil {
		logrus.Error("failed to publish twitch message: ", err)
	}
}

func (c *Connection) JoinChannel(channel *pb.Channel) {
	if c.idx != 0 {
		logrus.WithField("idx", c.idx).Debug("waiting limits: ", channel)
		if err := c.manager.rl(c.gCtx, channel); err != nil {
			logrus.Error("failed to get rates on channel: ", err)
			return
		}
		logrus.WithField("idx", c.idx).Debug("joining channel: ", channel)
	}

	c.mtx.Lock()
	c.channels[channel.GetLogin()] = (&Channel{
		Raw: channel,
	}).Update()
	c.mtx.Unlock()

	c.channels[channel.Login].Update(ChannelStateJoinRequested)
	c.client.Write("JOIN #" + channel.GetLogin())
	logrus.WithField("idx", c.idx).Debug("issued join for channel: ", channel)
}

func (c *Connection) PartChannel(channel string) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	logrus.WithField("idx", c.idx).Debug("issued part for channel: ", channel)

	delete(c.channels, channel)
	c.client.Write("PART #" + channel)
}

type Connections []*Connection

func (c *Connections) New(conn *Connection) {
	i := len(*c)
	*c = append(*c, conn)
	conn.idx = i
}

func (c Connections) First() *Connection {
	return c[0]
}

func (c Connections) Last() *Connection {
	if len(c) == 0 {
		return nil
	}

	return c[len(c)-1]
}
