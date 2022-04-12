package server

import (
	"context"
	"time"

	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_chat/v1"
	"github.com/seventv/twitch-chat-controller/src/global"
	"github.com/sirupsen/logrus"
)

type Server struct {
	gCtx global.Context
	pb.UnimplementedTwitchChatServiceServer
}

func New(gCtx global.Context) pb.TwitchChatServiceServer {
	return &Server{
		gCtx: gCtx,
	}
}

func (s *Server) Register(req *pb.RegisterRequest, srv pb.TwitchChatService_RegisterServer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	accountID := s.gCtx.Config().Irc.Accounts.MainAccountID

	loginEventCh := make(chan interface{}, 5)
	defer close(loginEventCh)
	defer s.gCtx.Inst().Events.Subscribe("twitch-chat:login:"+accountID, loginEventCh)()
	loginTick := time.NewTicker(time.Hour)
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
			loginTick.Reset(time.Hour)
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

			_ = srv.Send(&pb.RegisterResponse{
				Type: pb.EventType_EVENT_TYPE_LOGIN,
				Payload: &pb.RegisterResponse_LoginPayload_{
					LoginPayload: &pb.RegisterResponse_LoginPayload{
						Channel: &pb.Channel{
							Id:    accountID,
							Login: user.Login,
						},
						Oauth: auth.AccessToken,
					},
				},
			})
		}
	}
}
