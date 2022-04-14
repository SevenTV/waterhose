package server

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/SevenTV/Common/eventemitter"
	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-chat-controller/src/global"
	"github.com/seventv/twitch-chat-controller/src/structures"
	"github.com/seventv/twitch-chat-controller/src/svc/events"
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

	edgeIdx, err := strconv.Atoi(strings.TrimPrefix(req.NodeName, s.gCtx.Config().K8S.SatefulsetName+"-"))
	if err != nil {
		return ErrBadNodeName
	}

	accountID := s.gCtx.Config().Irc.BotAccountID

	loginEventCh := make(chan struct{}, 5)
	defer close(loginEventCh)
	loginEventCh <- struct{}{}

	loginTick := time.NewTicker(time.Hour + utils.JitterTime(time.Minute, time.Minute*10))
	defer loginTick.Stop()
	go func() {
		for range loginTick.C {
			loginEventCh <- struct{}{}
		}
	}()

	joinEventCh := make(chan []structures.Channel, 5)
	defer close(joinEventCh)
	joinEventCh <- s.gCtx.Inst().AutoScaler.GetChannelsForEdge(edgeIdx)

	defer s.gCtx.Inst().EventEmitter.Listen(eventemitter.NewEventListener(map[string]reflect.Value{
		events.TwitchChatLoginFormat(accountID): reflect.ValueOf(loginEventCh),
		events.EdgeChannelUpdateFormat(edgeIdx): reflect.ValueOf(joinEventCh),
	}))()

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
		case channels := <-joinEventCh:
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
