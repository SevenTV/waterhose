package ratelimiter

import (
	"context"
	"sync"
	"time"

	"github.com/SevenTV/Common/datastructures/priority_queue"
	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-chat-controller/src/instance"
)

const JoinLimitPerTenSeconds = 2000

type pqJoinChannel struct {
	once    sync.Once
	done    chan struct{}
	Channel *pb.Channel
}

type RateLimiter struct {
	mtx sync.Mutex
	pq  priority_queue.PriorityQueue[*pqJoinChannel]
}

func New() instance.RateLimiter {
	return &RateLimiter{
		pq: priority_queue.PriorityQueue[*pqJoinChannel]{},
	}
}

func (r *RateLimiter) Start() {
	tick := time.NewTicker(time.Second*10 + utils.JitterTime(time.Second, time.Second*5))
	for range tick.C {
		r.mtx.Lock()
		for i := 0; i < JoinLimitPerTenSeconds; i++ {
			if r.pq.Len() != 0 {
				item := r.pq.Pop().Value()
				item.once.Do(func() {
					close(item.done)
				})
			} else {
				break
			}
		}
		r.mtx.Unlock()
	}
}

func (r *RateLimiter) RequestJoin(ctx context.Context, channel *pb.Channel) error {
	r.mtx.Lock()
	item := r.pq.Push(&pqJoinChannel{
		done:    make(chan struct{}),
		Channel: channel,
	}, int(channel.Priority))
	r.mtx.Unlock()

	defer item.Value().once.Do(func() {
		close(item.Value().done)
	})

	select {
	case <-item.Value().done:
	case <-ctx.Done():
	}

	r.mtx.Lock()
	if item.Index() != -1 {
		r.pq.Remove(item.Index())
	}
	r.mtx.Unlock()

	return ctx.Err()
}