package master

import (
	"net"
	"sync"

	pb "github.com/seventv/twitch-edge/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-edge/src/global"
	"github.com/seventv/twitch-edge/src/modes/master/http"
	"github.com/seventv/twitch-edge/src/modes/master/server"
	"github.com/sirupsen/logrus"
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

	go func() {
		defer wg.Done()
		ln, err := net.Listen("tcp", gCtx.Config().API.Bind)
		if err != nil {
			logrus.Fatal("failed to listen to addresss: ", err)
		}

		grpcSrv = grpc.NewServer()
		pb.RegisterTwitchEdgeServiceServer(grpcSrv, server.New(gCtx))

		if err := grpcSrv.Serve(ln); err != nil {
			logrus.Fatal("failed to listen to addresss: ", err)
		}
	}()

	go func() {
		defer wg.Done()

		httpSrv = http.New(gCtx)

		if err := httpSrv.Start(gCtx.Config().API.HttpBind); err != nil {
			logrus.Fatal("failed to listen to addresss: ", err)
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
