package master

import (
	"net"
	"sync"

	"github.com/seventv/waterhose/internal/global"
	"github.com/seventv/waterhose/internal/health"
	"github.com/seventv/waterhose/internal/modes/master/http"
	"github.com/seventv/waterhose/internal/modes/master/server"
	"github.com/seventv/waterhose/internal/monitoring"
	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func New(gCtx global.Context) <-chan struct{} {
	wg := sync.WaitGroup{}
	wg.Add(2)

	done := make(chan struct{})

	var (
		httpSrv *http.HttpServer
		grpcSrv *grpc.Server
	)

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
		defer wg.Done()

		ln, err := net.Listen("tcp", gCtx.Config().Master.API.Bind)
		if err != nil {
			zap.S().Fatalw("failed to listen to addresss: ",
				"error", err,
			)
		}

		grpcSrv = grpc.NewServer()
		pb.RegisterWaterHoseServiceServer(grpcSrv, server.New(gCtx))

		if err := grpcSrv.Serve(ln); err != nil {
			zap.S().Fatalw("failed to listen to addresss: ",
				"error", err,
			)
		}
	}()

	go func() {
		defer wg.Done()

		httpSrv = http.New(gCtx)
		if err := httpSrv.Start(gCtx.Config().Master.API.HttpBind); err != nil {
			zap.S().Fatalw("failed to listen to addresss: ",
				"error", err,
			)
		}
	}()

	go func() {
		<-gCtx.Done()

		go grpcSrv.Stop()
		go func() {
			_ = httpSrv.Shutdown()
		}()

		wg.Wait()
		close(done)
	}()

	return done
}
