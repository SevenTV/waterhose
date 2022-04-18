package manager

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SevenTV/Common/sync_map"
	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
	"github.com/seventv/waterhose/src/global"
)

/*

The 0th connection in each manager will be a connection that does not have auth, anonymous TwitchIRC user.

*/

type RateLimiter func(context.Context, *pb.Channel) error

type EventPublish func(context.Context, *pb.PublishEdgeChannelEventRequest) error

type Manager struct {
	gCtx global.Context

	channels sync_map.Map[string, uint32]
	ircConns sync_map.Map[uint32, *Connection]
	ircIdx   *uint32
	mtx      sync.Mutex

	openConnMtx sync.Mutex

	rl RateLimiter
	ep EventPublish

	connectionOptions ConnectionOptions
}

func New(gCtx global.Context, rl RateLimiter, ep EventPublish) *Manager {
	manager := &Manager{
		gCtx:   gCtx,
		ircIdx: utils.PointerOf(uint32(0)),
		rl:     rl,
		ep:     ep,
	}

	manager.init()

	return manager
}

func (m *Manager) SetLoginCreds(options ConnectionOptions) {
	m.connectionOptions = options
	m.ircConns.Range(func(key uint32, value *Connection) bool {
		if key != 0 {
			value.client.SetOAuth(options.OAuth)
		}
		return true
	})
}

func (m *Manager) JoinChat(channel *pb.Channel) {
	if idx, ok := m.channels.Load(channel.GetId()); ok {
		conn, _ := m.ircConns.Load(idx)
		conn.JoinChannel(channel)
		return
	}

	var conn *Connection
	m.mtx.Lock()
	idx := atomic.LoadUint32(m.ircIdx)
	if idx == 1 {
		conn = m.newConn(m.connectionOptions)
	} else {
		conn, _ = m.ircConns.Load(idx - 1)
		if conn.ConnLength() >= m.gCtx.Config().Slave.IRC.ChannelLimitPerConn {
			conn = m.newConn(m.connectionOptions)
		}
	}
	m.mtx.Unlock()

	conn.JoinChannel(channel)
	m.channels.Store(channel.GetId(), conn.idx)
}

func (m *Manager) PartChat(channel *pb.Channel) {
	if idx, ok := m.channels.Load(channel.GetId()); ok {
		conn, _ := m.ircConns.Load(idx)
		conn.PartChannel(channel.GetLogin())
	}
}

func (m *Manager) joinAnon(channel *pb.Channel) {
	if idx, ok := m.channels.Load(channel.GetId()); ok {
		conn, _ := m.ircConns.Load(idx)
		conn.PartChannel(channel.GetLogin())
	}

	m.channels.Store(channel.GetId(), 0)
	conn, _ := m.ircConns.Load(0)
	conn.JoinChannel(channel)
}

func (m *Manager) init() {
	m.newConn(ConnectionOptions{
		Username: "justinfan123123",
		OAuth:    "oauth:59301",
	})
}

func (m *Manager) newConn(options ConnectionOptions) *Connection {
	conn := newConnection(m.gCtx, m, options, m.openConnLimiter)
	idx := atomic.AddUint32(m.ircIdx, 1) - 1
	conn.idx = idx
	m.ircConns.Store(idx, conn)
	return conn
}

func (m *Manager) openConnLimiter() {
	m.openConnMtx.Lock()
	time.Sleep(time.Millisecond * 250)
	m.openConnMtx.Unlock()
}
