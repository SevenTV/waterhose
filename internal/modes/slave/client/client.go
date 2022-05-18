package client

import (
	"context"
	"fmt"
	"time"

	"github.com/SevenTV/Common/utils"
	"github.com/seventv/waterhose/internal/global"
	"github.com/seventv/waterhose/internal/modes/slave/manager"
	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	gCtx global.Context

	manager *manager.Manager

	grpc pb.WaterHoseServiceClient
}

func New(gCtx global.Context) <-chan struct{} {
	done := make(chan struct{})

	var cl *Client
	cl = &Client{
		gCtx: gCtx,
		manager: manager.New(gCtx, func(ctx context.Context, channel *pb.Channel) error {
			_, err := cl.grpc.SlaveJoinLimit(ctx, &pb.SlaveJoinLimitRequest{
				Channel: channel,
			})
			return err
		}, func(ctx context.Context, evt *pb.PublishSlaveChannelEventRequest) error {
			_, err := cl.grpc.PublishSlaveChannelEvent(ctx, evt)
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
			zap.S().Info("starting grpc")
			ctx, cancel := context.WithCancel(gCtx)
			err := cl.initGrpc(ctx)
			cancel()
			failedN++
			if err != nil {
				if gCtx.Err() != nil {
					return
				}
				zap.S().Errorw("grpc disconnect with error",
					"error", err,
				)
			} else {
				zap.S().Warn("disconnected from grpc")
			}

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

	c.grpc = pb.NewWaterHoseServiceClient(conn)
	events, err := c.grpc.RegisterSlave(ctx, &pb.RegisterSlaveRequest{
		SlaveName: c.gCtx.Config().K8S.NodeName,
	})
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to register node: %v", err)
	}

	zap.S().Info("connected to grpc")

	for {
		msg, err := events.Recv()
		if err != nil {
			return err
		}

		switch payload := msg.Payload.(type) {
		case *pb.RegisterSlaveResponse_JoinChannelPayload_:
			channels := payload.JoinChannelPayload.GetChannels()
			zap.S().Infow("recieved channels to join",
				"length", len(channels),
			)
			for _, v := range channels {
				c.manager.JoinChat(v)
			}
		case *pb.RegisterSlaveResponse_PartChannelPayload_:
			channels := payload.PartChannelPayload.GetChannels()
			zap.S().Infow("recieved channels to part",
				"length", len(channels),
			)
			for _, v := range channels {
				c.manager.PartChat(v)
			}
		case *pb.RegisterSlaveResponse_LoginPayload_:
			zap.S().Info("recieved login info")
			c.manager.SetLoginCreds(manager.ConnectionOptions{
				Username: payload.LoginPayload.GetChannel().GetLogin(),
				OAuth:    "oauth:" + payload.LoginPayload.GetOauth(),
			})
		}
	}
}
