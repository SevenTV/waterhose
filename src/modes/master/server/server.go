package server

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/SevenTV/Common/eventemitter"
	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
	"github.com/seventv/waterhose/src/global"
	"github.com/seventv/waterhose/src/structures"
	"github.com/seventv/waterhose/src/svc/events"
	"go.uber.org/zap"
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

	edgeIdx, err := strconv.Atoi(strings.TrimPrefix(req.NodeName, s.gCtx.Config().Master.K8S.SatefulsetName+"-"))
	if err != nil {
		return ErrBadNodeName
	}

	accountID := s.gCtx.Config().Master.Irc.BotAccountID

	loginEventCh := make(chan struct{}, 50)
	defer close(loginEventCh)
	loginEventCh <- struct{}{}

	loginTick := time.NewTicker(time.Hour + utils.JitterTime(time.Minute, time.Minute*10))
	defer loginTick.Stop()
	go func() {
		for range loginTick.C {
			loginEventCh <- struct{}{}
		}
	}()

	joinEventCh := make(chan []structures.Channel, 50)
	defer close(joinEventCh)

	first := false

	defer s.gCtx.Inst().EventEmitter.Listen(eventemitter.NewEventListener(map[string]reflect.Value{
		events.TwitchChatLoginFormat(accountID): reflect.ValueOf(loginEventCh),
		events.EdgeChannelUpdateFormat(edgeIdx): reflect.ValueOf(joinEventCh),
	}))()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-loginEventCh:
			if !first {
				go func() {
					joinEventCh <- s.gCtx.Inst().AutoScaler.GetChannelsForEdge(edgeIdx)
				}()
				first = false
			}
			loginTick.Reset(time.Hour + utils.JitterTime(time.Minute, time.Minute*10))
			utils.EmptyChannel(loginEventCh)

			tCtx, tCancel := context.WithTimeout(ctx, time.Second*5)
			auth, err := s.gCtx.Inst().Twitch.GetOAuth(tCtx, accountID)
			tCancel()
			if err != nil {
				zap.S().Errorw("failed to get twitch login oauth",
					"error", err,
				)
				return ErrNoLoginData
			}

			user, err := s.gCtx.Inst().Twitch.GetUser(accountID)
			if err != nil {
				zap.S().Errorw("failed to get twitch user",
					"error", err,
				)
				return ErrNoLoginData
			}

			if err = srv.Send(&pb.RegisterEdgeResponse{
				Type: pb.RegisterEdgeResponse_EVENT_TYPE_LOGIN,
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
				Type: pb.RegisterEdgeResponse_EVENT_TYPE_JOIN_CHANNEL,
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

func (s *Server) PublishEdgeChannelEvent(ctx context.Context, req *pb.PublishEdgeChannelEventRequest) (*pb.PublishEdgeChannelEventResponse, error) {
	switch req.Type {
	case pb.PublishEdgeChannelEventRequest_EVENT_TYPE_BANNED:
		zap.S().Debugw("channel banned bot",
			"channel_id", req.Channel.Id,
			"channel_login", req.Channel.Login,
		)
		// TODO handle this
	case pb.PublishEdgeChannelEventRequest_EVENT_TYPE_SUSPENDED_CHANNEL:
		zap.S().Debugw("suspended channel",
			"channel_id", req.Channel.Id,
			"channel_login", req.Channel.Login,
		)
		// TODO handle this
	case pb.PublishEdgeChannelEventRequest_EVENT_TYPE_UNKNOWN_CHANNEL:
		zap.S().Debugw("unknown channel",
			"channel_id", req.Channel.Id,
			"channel_login", req.Channel.Login,
		)
		// we need to issue a rejoin on this channel
		if err := s.gCtx.Inst().AutoScaler.AllocateChannels([]*pb.Channel{
			req.Channel,
		}); err != nil {
			return nil, err
		}
	}
	return &pb.PublishEdgeChannelEventResponse{
		Success: true,
	}, nil
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
