package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/seventv/twitch-edge/src/global"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"
)

func New(gCtx global.Context) <-chan struct{} {
	r := prometheus.NewRegistry()
	gCtx.Inst().Monitoring.Register(r)

	server := fasthttp.Server{
		Handler: fasthttpadaptor.NewFastHTTPHandler(promhttp.HandlerFor(r, promhttp.HandlerOpts{
			Registry:          r,
			EnableOpenMetrics: true,
		})),
		GetOnly:          true,
		DisableKeepalive: true,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.ListenAndServe(gCtx.Config().Monitoring.Bind); err != nil {
			zap.S().Fatalw("failed to start monitoring bind",
				"error", err,
			)
		}
	}()

	go func() {
		<-gCtx.Done()
		_ = server.Shutdown()
	}()
	return done
}
