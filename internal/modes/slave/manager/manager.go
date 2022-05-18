package manager

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SevenTV/Common/sync_map"
	"github.com/SevenTV/Common/utils"
	"github.com/seventv/waterhose/internal/global"
	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
)

/*

The 0th connection in each manager will be a connection that does not have auth, anonymous TwitchIRC user.

*/

var anonCreds = ConnectionOptions{
	Username: "justinfan123123",
	OAuth:    "oauth:59301",
}

type RateLimiter func(context.Context, *pb.Channel) error

type EventPublish func(context.Context, *pb.PublishSlaveChannelEventRequest) error

type channelAllocation struct {
	Id   uint32
	Anon bool
}

type Manager struct {
	gCtx global.Context

	channels sync_map.Map[string, channelAllocation]

	ircConns sync_map.Map[uint32, *Connection]
	ircIdx   *uint32

	ircAnonConns sync_map.Map[uint32, *Connection]
	ircAnonIdx   *uint32

	mtx sync.Mutex

	openConnMtx sync.Mutex

	rl RateLimiter
	ep EventPublish

	connectionOptions ConnectionOptions
}

func New(gCtx global.Context, rl RateLimiter, ep EventPublish) *Manager {
	manager := &Manager{
		gCtx:       gCtx,
		ircIdx:     utils.PointerOf(uint32(0)),
		ircAnonIdx: utils.PointerOf(uint32(0)),
		rl:         rl,
		ep:         ep,
	}

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
	if allocation, ok := m.channels.Load(channel.GetId()); ok {
		var conn *Connection
		if allocation.Anon {
			conn, _ = m.ircAnonConns.Load(allocation.Id)
		} else {
			conn, _ = m.ircConns.Load(allocation.Id)
		}
		if allocation.Anon == channel.UseAnonymous {
			conn.JoinChannel(channel)
			return
		}
		conn.PartChannel(channel.GetLogin())
		m.channels.Delete(channel.GetId())
	}

	var conn *Connection
	var allocation channelAllocation
	var ptr *uint32

	m.mtx.Lock()
	if channel.UseAnonymous {
		ptr = m.ircAnonIdx
	} else {
		ptr = m.ircIdx
	}

	allocation.Anon = channel.UseAnonymous
	idx := atomic.LoadUint32(ptr)
	if idx == 0 {
		conn = m.newConn(channel.UseAnonymous)
	} else {
		if channel.UseAnonymous {
			conn, _ = m.ircAnonConns.Load(idx - 1)
		} else {
			conn, _ = m.ircConns.Load(idx - 1)
		}
		if conn.ConnLength() >= m.gCtx.Config().Slave.IRC.ChannelLimitPerConn {
			conn = m.newConn(channel.UseAnonymous)
		}
	}

	allocation.Id = conn.idx
	m.channels.Store(channel.GetId(), allocation)
	m.mtx.Unlock()

	conn.JoinChannel(channel)
}

func (m *Manager) PartChat(channel *pb.Channel) {
	if allocation, ok := m.channels.Load(channel.GetId()); ok {
		var conn *Connection
		if allocation.Anon {
			conn, _ = m.ircConns.Load(allocation.Id)
		} else {
			conn, _ = m.ircAnonConns.Load(allocation.Id)
		}
		conn.PartChannel(channel.GetLogin())
	}
}

func (m *Manager) newConn(anon bool) *Connection {
	options := m.connectionOptions
	var p *uint32
	if anon {
		p = m.ircAnonIdx
		options = anonCreds
	} else {
		p = m.ircIdx
	}

	conn := newConnection(m.gCtx, m, options, m.openConnLimiter)
	idx := atomic.AddUint32(p, 1) - 1
	conn.idx = idx
	conn.anon = anon
	if anon {
		m.ircAnonConns.Store(idx, conn)
	} else {
		m.ircConns.Store(idx, conn)
	}

	return conn
}

func (m *Manager) openConnLimiter() {
	m.openConnMtx.Lock()
	time.Sleep(time.Millisecond * 250)
	m.openConnMtx.Unlock()
}
