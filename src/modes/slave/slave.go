package slave

import "github.com/seventv/twitch-edge/src/global"

func New(gCtx global.Context) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		<-gCtx.Done()

		close(done)
	}()

	return done
}
