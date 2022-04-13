package server

import (
	"context"
	"time"

	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-chat-controller/src/global"
	"github.com/sirupsen/logrus"
)

type Server struct {
	gCtx global.Context
	pb.UnimplementedTwitchEdgeServiceServer
}

func New(gCtx global.Context) pb.TwitchEdgeServiceServer {
	return &Server{
		gCtx: gCtx,
	}
}

func (s *Server) RegisterEdge(req *pb.RegisterEdgeRequest, srv pb.TwitchEdgeService_RegisterEdgeServer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	accountID := s.gCtx.Config().Irc.Accounts.MainAccountID

	loginEventCh := make(chan interface{}, 5)
	defer close(loginEventCh)
	defer s.gCtx.Inst().Events.Subscribe("twitch-chat:login:"+accountID, loginEventCh)()

	loginTick := time.NewTicker(time.Hour + utils.JitterTime(time.Minute, time.Minute*10))
	defer loginTick.Stop()
	go func() {
		for range loginTick.C {
			loginEventCh <- nil
		}
	}()

	loginEventCh <- nil

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-loginEventCh:
			loginTick.Reset(time.Hour + utils.JitterTime(time.Minute, time.Minute*10))
			utils.EmptyChannel(loginEventCh)

			tCtx, tCancel := context.WithTimeout(ctx, time.Second*5)
			auth, err := s.gCtx.Inst().Twitch.GetOAuth(tCtx, accountID)
			tCancel()
			if err != nil {
				logrus.Error("failed to get twitch login oauth: ", err)
				return ErrNoLoginData
			}

			user, err := s.gCtx.Inst().Twitch.GetUser(accountID)
			if err != nil {
				logrus.Error("failed to get twitch user: ", err)
				return ErrNoLoginData
			}

			if err = srv.Send(&pb.RegisterEdgeResponse{
				Type: pb.EventType_EVENT_TYPE_LOGIN,
				Payload: &pb.RegisterEdgeResponse_LoginPayload_{
					LoginPayload: &pb.RegisterEdgeResponse_LoginPayload{
						Channel: &pb.Channel{
							Id:    accountID,
							Login: user.Login,
						},
						Oauth: auth.AccessToken,
					},
				},
			}); err != nil {
				return err
			}
		}
	}
}

func (s *Server) PublishEdgeEvent(ctx context.Context, req *pb.PublishEdgeEventRequest) (*pb.PublishEdgeEventResponse, error) {
	return nil, nil
}

func (s *Server) JoinChannel(ctx context.Context, req *pb.JoinChannelRequest) (*pb.JoinChannelResponse, error) {
	if err := s.gCtx.Inst().AutoScaler.AllocateChannels(req.Channels); err != nil {
		return nil, err
	}

	return &pb.JoinChannelResponse{
		Success: true,
	}, nil
}
