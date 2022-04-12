package app

import (
	"net"
	"sync"

	pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_chat/v1"
	"github.com/seventv/twitch-chat-controller/src/app/server"
	"github.com/seventv/twitch-chat-controller/src/global"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"google.golang.org/grpc"
)

func New(gCtx global.Context) <-chan struct{} {
	wg := sync.WaitGroup{}
	wg.Add(2)

	done := make(chan struct{})

	var (
		httpSrv *HttpServer
		grpcSrv *grpc.Server
	)

	go func() {
		defer wg.Done()
		ln, err := net.Listen("tcp", gCtx.Config().API.Bind)
		if err != nil {
			logrus.Fatal("failed to listen to addresss: ", err)
		}

		grpcSrv = grpc.NewServer()
		pb.RegisterTwitchChatServiceServer(grpcSrv, server.New(gCtx))

		if err := grpcSrv.Serve(ln); err != nil {
			logrus.Fatal("failed to listen to addresss: ", err)
		}
	}()

	go func() {
		defer wg.Done()

		httpSrv = &HttpServer{
			gCtx:   gCtx,
			Server: &fasthttp.Server{},
		}

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
