package client

import (
	"context"
	"fmt"
	"time"

	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/twitch-edge/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-edge/src/global"
	"github.com/seventv/twitch-edge/src/modes/slave/manager"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	gCtx global.Context

	manager *manager.Manager

	grpc pb.TwitchEdgeServiceClient
}

func New(gCtx global.Context) <-chan struct{} {
	done := make(chan struct{})

	var cl *Client
	cl = &Client{
		gCtx: gCtx,
		manager: manager.New(gCtx, func(ctx context.Context, channel *pb.Channel) error {
			_, err := cl.grpc.JoinChannelEdge(ctx, &pb.JoinChannelEdgeRequest{
				Channel: channel,
			})
			return err
		}, func(ctx context.Context, evt *pb.PublishEdgeChannelEventRequest) error {
			_, err := cl.grpc.PublishEdgeChannelEvent(ctx, evt)
			return err
		}),
	}

	go func() {
		<-gCtx.Done()

		close(done)
	}()

	go func() {
		failedN := 0
		for {
			logrus.Info("starting grpc")
			ctx, cancel := context.WithCancel(gCtx)
			err := cl.initGrpc(ctx)
			cancel()
			failedN++
			if err != nil {
				if gCtx.Err() != nil {
					return
				}
				logrus.Error("grpc disconnect with error: ", err)
			} else {
				logrus.Warn("disconnected from grpc")
			}

			logrus.Info(utils.JitterTime(time.Second, time.Second*10))

			time.Sleep(time.Second*5 + utils.JitterTime(time.Second, time.Second*10))
		}
	}()

	return done
}

func (c *Client) initGrpc(ctx context.Context) error {
	ctxT, cancelT := context.WithTimeout(ctx, time.Second*5)
	conn, err := grpc.DialContext(ctxT, c.gCtx.Config().Slave.API.GrpcDial, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	cancelT()
	if err != nil {
		return fmt.Errorf("failed to connect to grpc_dial: %v", err)
	}
	defer conn.Close()

	c.grpc = pb.NewTwitchEdgeServiceClient(conn)
	events, err := c.grpc.RegisterEdge(ctx, &pb.RegisterEdgeRequest{
		NodeName: c.gCtx.Config().K8S.NodeName,
	})
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to register node: %v", err)
	}

	logrus.Info("connected to grpc")

	for {
		msg, err := events.Recv()
		if err != nil {
			return err
		}

		switch payload := msg.Payload.(type) {
		case *pb.RegisterEdgeResponse_JoinChannelPayload_:
			channels := payload.JoinChannelPayload.GetChannels()
			for _, v := range channels {
				c.manager.JoinChat(v)
			}
		case *pb.RegisterEdgeResponse_PartChannelPayload_:
			channels := payload.PartChannelPayload.GetChannels()
			for _, v := range channels {
				c.manager.PartChat(v)
			}
		case *pb.RegisterEdgeResponse_LoginPayload_:
			c.manager.SetLoginCreds(manager.ConnectionOptions{
				Username: payload.LoginPayload.GetChannel().GetLogin(),
				OAuth:    "oauth:" + payload.LoginPayload.GetOauth(),
			})
		}
	}
}
