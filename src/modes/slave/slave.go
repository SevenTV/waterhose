package slave

import (
	"github.com/seventv/twitch-edge/src/global"
	"github.com/seventv/twitch-edge/src/modes/slave/client"
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
