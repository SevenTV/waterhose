package manager

import (
	"context"
	"sync"

	pb "github.com/seventv/twitch-edge/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-edge/src/global"
)

/*

The 0th connection in each manager will be a connection that does not have auth, anonymous TwitchIRC user.

*/

type RateLimiter func(context.Context, *pb.Channel) error

type EventPublish func(context.Context, *pb.PublishEdgeChannelEventRequest) error

type Manager struct {
	gCtx global.Context

	channels map[string]int
	ircConns Connections
	mtx      sync.Mutex

	rl RateLimiter
	ep EventPublish

	connectionOptions ConnectionOptions
}

func New(gCtx global.Context, rl RateLimiter, ep EventPublish) *Manager {
	manager := &Manager{
		gCtx:     gCtx,
		channels: map[string]int{},
		ircConns: []*Connection{},
		rl:       rl,
	}

	manager.init()

	return manager
}

func (m *Manager) SetLoginCreds(options ConnectionOptions) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.connectionOptions = options
	for _, v := range m.ircConns[1:] {
		v.client.SetOAuth(options.OAuth)
	}
}

func (m *Manager) JoinChat(channel *pb.Channel) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if idx, ok := m.channels[channel.GetId()]; ok {
		go m.ircConns[idx].JoinChannel(channel)
		return
	}

	if len(m.ircConns) == 1 || len(m.ircConns.Last().channels) >= m.gCtx.Config().Slave.IRC.ChannelLimitPerConn {
		m.newConn(m.connectionOptions)
	}

	conn := m.ircConns.Last()
	go conn.JoinChannel(channel)
	m.channels[channel.GetId()] = conn.idx
}

func (m *Manager) PartChat(channel *pb.Channel) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if idx, ok := m.channels[channel.GetId()]; ok {
		m.ircConns[idx].PartChannel(channel.GetId())
	}
}

func (m *Manager) joinAnon(channel *pb.Channel) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if idx, ok := m.channels[channel.GetId()]; ok {
		m.ircConns[idx].PartChannel(channel.GetId())
	}

	m.channels[channel.GetId()] = 0
	m.ircConns.First().JoinChannel(channel)
}

func (m *Manager) init() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.newConn(ConnectionOptions{
		Username: "justinfan123123",
		OAuth:    "oauth:59301",
	})
}

func (m *Manager) newConn(options ConnectionOptions) {
	m.ircConns.New(newConnection(m.gCtx, m, options))
}
