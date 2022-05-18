package server

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/SevenTV/Common/eventemitter"
	"github.com/SevenTV/Common/utils"
	"github.com/seventv/waterhose/internal/global"
	"github.com/seventv/waterhose/internal/structures"
	"github.com/seventv/waterhose/internal/svc/events"
	"github.com/seventv/waterhose/internal/svc/mongo"
	pb "github.com/seventv/waterhose/protobuf/waterhose/v1"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

type Server struct {
	gCtx global.Context
	pb.UnimplementedWaterHoseServiceServer
}

func New(gCtx global.Context) pb.WaterHoseServiceServer {
	return &Server{
		gCtx: gCtx,
	}
}

func (s *Server) RegisterSlave(req *pb.RegisterSlaveRequest, srv pb.WaterHoseService_RegisterSlaveServer) error {
	ctx, cancel := context.WithCancel(srv.Context())
	defer cancel()

	slaveIdx, err := strconv.Atoi(strings.TrimPrefix(req.SlaveName, s.gCtx.Config().Master.K8S.SatefulsetName+"-"))
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
		events.TwitchChatLoginFormat(accountID):   reflect.ValueOf(loginEventCh),
		events.SlaveChannelUpdateFormat(slaveIdx): reflect.ValueOf(joinEventCh),
	}))()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-loginEventCh:
			if !first {
				go func() {
					joinEventCh <- s.gCtx.Inst().AutoScaler.GetChannelsForSlave(slaveIdx)
				}()
				first = true
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

			if err = srv.Send(&pb.RegisterSlaveResponse{
				Type: pb.RegisterSlaveResponse_EVENT_TYPE_LOGIN,
				Payload: &pb.RegisterSlaveResponse_LoginPayload_{
					LoginPayload: &pb.RegisterSlaveResponse_LoginPayload{
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
					Id:           channel.TwitchID,
					Login:        channel.TwitchLogin,
					Priority:     channel.Priority,
					UseAnonymous: channel.UseAnonymous,
					BotBanned:    channel.BotBanned,
				}
			}

			if err = srv.Send(&pb.RegisterSlaveResponse{
				Type: pb.RegisterSlaveResponse_EVENT_TYPE_JOIN_CHANNEL,
				Payload: &pb.RegisterSlaveResponse_JoinChannelPayload_{
					JoinChannelPayload: &pb.RegisterSlaveResponse_JoinChannelPayload{
						Channels: pbChannels,
					},
				},
			}); err != nil {
				return err
			}
		}
	}
}

func (s *Server) PublishSlaveChannelEvent(ctx context.Context, req *pb.PublishSlaveChannelEventRequest) (*pb.PublishSlaveChannelEventResponse, error) {
	switch req.Type {
	case pb.PublishSlaveChannelEventRequest_EVENT_TYPE_BOT_BANNED:
		zap.S().Debugw("channel banned bot",
			"channel_id", req.Channel.Id,
			"channel_login", req.Channel.Login,
		)
		if _, err := s.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).UpdateOne(ctx, bson.M{
			"twitch_id": req.Channel.Id,
		}, bson.M{
			"$set": bson.M{
				"bot_banned": true,
			},
		}); err != nil {
			zap.S().Errorw("failed to update mongo",
				"error", err,
			)
		}
	case pb.PublishSlaveChannelEventRequest_EVENT_TYPE_SUSPENDED_CHANNEL:
		zap.S().Debugw("suspended channel",
			"channel_id", req.Channel.Id,
			"channel_login", req.Channel.Login,
		)
		if _, err := s.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).UpdateOne(ctx, bson.M{
			"twitch_id": req.Channel.Id,
		}, bson.M{
			"$set": bson.M{
				"state": structures.ChannelStateSuspended,
			},
		}); err != nil {
			zap.S().Errorw("failed to update mongo",
				"error", err,
			)
		}
	case pb.PublishSlaveChannelEventRequest_EVENT_TYPE_JOINED:
		zap.S().Debugw("joined channel",
			"channel_id", req.Channel.Id,
			"channel_login", req.Channel.Login,
		)
		if _, err := s.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).UpdateOne(ctx, bson.M{
			"twitch_id": req.Channel.Id,
		},
			bson.M{
				"$set": bson.M{
					"state":      structures.ChannelStateJoined,
					"bot_banned": req.Channel.BotBanned,
				},
			}); err != nil {
			zap.S().Errorw("failed to update mongo",
				"error", err,
			)
		}
	case pb.PublishSlaveChannelEventRequest_EVENT_TYPE_UNKNOWN_CHANNEL:
		zap.S().Debugw("unknown channel",
			"channel_id", req.Channel.Id,
			"channel_login", req.Channel.Login,
		)
		if _, err := s.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).UpdateOne(ctx, bson.M{
			"twitch_id": req.Channel.Id,
		}, bson.M{
			"$set": bson.M{
				"state": structures.ChannelStateUnknown,
			},
		}); err != nil {
			zap.S().Errorw("failed to update mongo",
				"error", err,
			)
		}
		// we need to issue a rejoin on this channel
		if err := s.gCtx.Inst().AutoScaler.AllocateChannels([]*pb.Channel{
			req.Channel,
		}); err != nil {
			return nil, err
		}
	}
	return &pb.PublishSlaveChannelEventResponse{
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

func (s *Server) SlaveJoinLimit(ctx context.Context, req *pb.SlaveJoinLimitRequest) (*pb.SlaveJoinLimitResponse, error) {
	err := s.gCtx.Inst().RateLimit.RequestJoin(ctx, req.Channel)
	if err != nil {
		return nil, err
	}

	return &pb.SlaveJoinLimitResponse{
		Allowed: true,
	}, nil
}
