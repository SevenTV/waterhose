package events

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/SevenTV/Common/utils"
)

type Events struct {
	mtx sync.Mutex
	mp  map[string]*eventContainer
}

type eventContainer struct {
	i   *int32
	mtx sync.Mutex
	mp  map[int32]chan interface{}
}

func New() *Events {
	evt := &Events{
		mp: map[string]*eventContainer{},
	}

	go evt.clean()

	return evt
}

func (e *Events) Subscribe(evt string, ch chan interface{}) func() {
	e.mtx.Lock()

	evtContainer, ok := e.mp[evt]
	if !ok {
		e.mp[evt] = &eventContainer{
			i:  utils.PointerOf(int32(0)),
			mp: map[int32]chan interface{}{},
		}
		evtContainer = e.mp[evt]
	}

	e.mtx.Unlock()
	evtContainer.mtx.Lock()

	i := atomic.AddInt32(evtContainer.i, 1)
	evtContainer.mp[i] = ch

	evtContainer.mtx.Unlock()

	return func() {
		evtContainer.mtx.Lock()
		defer evtContainer.mtx.Unlock()

		delete(evtContainer.mp, i)
	}
}

func (e *Events) Publish(evt string, payload interface{}) int {
	e.mtx.Lock()

	i := 0
	if evtContainer, ok := e.mp[evt]; ok {
		evtContainer.mtx.Lock()
		i = len(evtContainer.mp)
		for _, v := range evtContainer.mp {
			select {
			case v <- payload:
			default:
			}
		}
		evtContainer.mtx.Unlock()
	}
	e.mtx.Unlock()

	return i
}

func (e *Events) clean() {
	tick := time.NewTicker(time.Minute*30 + utils.JitterTime(time.Second, time.Minute*5))
	for range tick.C {
		e.mtx.Lock()

		for k, v := range e.mp {
			v.mtx.Lock()
			if len(v.mp) == 0 {
				delete(e.mp, k)
			}
			v.mtx.Unlock()
		}

		e.mtx.Unlock()
	}
}
