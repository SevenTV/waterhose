package slave

import (
	"github.com/seventv/waterhose/src/global"
	"github.com/seventv/waterhose/src/modes/slave/client"
)

func New(gCtx global.Context) <-chan struct{} {
	done := make(chan struct{})

	clientDone := client.New(gCtx)

	go func() {
		<-gCtx.Done()

		<-clientDone

		close(done)
	}()

	return done
}
