package irc

import "sync"

type errorChan struct {
	ch   chan error
	once *sync.Once
}

func (e *errorChan) Error(err error) {
	e.once.Do(func() {
		e.ch <- err
		close(e.ch)
	})
}

func (e *errorChan) Reset() *errorChan {
	e.ch = make(chan error, 1)
	e.once = &sync.Once{}
	return e
}
