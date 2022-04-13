package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-chat-controller/src/global"
	"github.com/seventv/twitch-chat-controller/src/structures"
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
	ctx, cancel := context.WithCancel(srv.Context())
	defer cancel()

	accountID := s.gCtx.Config().Irc.Accounts.MainAccountID

	loginEventCh := make(chan any, 5)
	loginEventCh <- nil
	defer close(loginEventCh)
	defer s.gCtx.Inst().Events.Subscribe("twitch-chat:login:"+accountID, loginEventCh)()

	name, err := strconv.Atoi(strings.TrimPrefix(req.NodeName, s.gCtx.Config().K8S.SatefulsetName+"-"))
	if err != nil {
		return ErrBadNodeName
	}

	joinEventCh := make(chan any, 5)
	joinEventCh <- s.gCtx.Inst().AutoScaler.GetChannelsForEdge(name)
	defer close(joinEventCh)
	defer s.gCtx.Inst().Events.Subscribe(fmt.Sprintf("edge-update:%d", name), joinEventCh)()

	loginTick := time.NewTicker(time.Hour + utils.JitterTime(time.Minute, time.Minute*10))
	defer loginTick.Stop()
	go func() {
		for range loginTick.C {
			loginEventCh <- nil
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
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
		case ch := <-joinEventCh:
			channels := ch.([]structures.Channel)

			pbChannels := make([]*pb.Channel, len(channels))
			for i, channel := range channels {
				pbChannels[i] = &pb.Channel{
					Id:       channel.TwitchID,
					Login:    channel.TwitchLogin,
					Priority: channel.Priority,
				}
			}

			if err = srv.Send(&pb.RegisterEdgeResponse{
				Type: pb.EventType_EVENT_TYPE_JOIN_CHANNEL,
				Payload: &pb.RegisterEdgeResponse_JoinChannelPayload_{
					JoinChannelPayload: &pb.RegisterEdgeResponse_JoinChannelPayload{
						Channels: pbChannels,
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

func (s *Server) JoinChannelEdge(ctx context.Context, req *pb.JoinChannelEdgeRequest) (*pb.JoinChannelEdgeResponse, error) {
	err := s.gCtx.Inst().RateLimit.RequestJoin(ctx, req.Channel)
	if err != nil {
		return nil, err
	}

	return &pb.JoinChannelEdgeResponse{
		Allowed: true,
	}, nil
}
