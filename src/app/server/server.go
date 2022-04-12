package server

import (
	"context"

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
	accountID := s.gCtx.Config().Irc.Accounts.MainAccountID
	auth, err := s.gCtx.Inst().Twitch.GetOAuth(context.TODO(), accountID)
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

	return s.UnimplementedTwitchChatServiceServer.Register(req, srv)
}
