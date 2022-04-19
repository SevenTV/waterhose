package slave

import (
	"sync"

	"github.com/seventv/waterhose/src/global"
	"github.com/seventv/waterhose/src/health"
	"github.com/seventv/waterhose/src/modes/slave/client"
	"github.com/seventv/waterhose/src/monitoring"
)

func New(gCtx global.Context) <-chan struct{} {
	done := make(chan struct{})

	clientDone := client.New(gCtx)
	wg := sync.WaitGroup{}

	if gCtx.Config().Health.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-health.New(gCtx)
		}()
	}

	if gCtx.Config().Monitoring.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-monitoring.New(gCtx)
		}()
	}

	go func() {
		<-gCtx.Done()

		<-clientDone

		wg.Done()

		close(done)
	}()

	return done
}
