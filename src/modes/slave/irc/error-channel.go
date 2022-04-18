package irc

import "sync"

type errorChan struct {
	ch   chan error
	err  error
	once *sync.Once
}

func (e *errorChan) Error(err error) {
	e.once.Do(func() {
		e.ch <- err
		e.err = err
		close(e.ch)
	})
}

func (e *errorChan) Reset() *errorChan {
	e.ch = make(chan error, 1)
	e.once = &sync.Once{}
	e.err = nil
	return e
}
