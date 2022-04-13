package autoscaler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/SevenTV/Common/utils"
	"github.com/seventv/twitch-chat-controller/loaders"
	pb "github.com/seventv/twitch-chat-controller/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-chat-controller/src/global"
	"github.com/seventv/twitch-chat-controller/src/instance"
	"github.com/seventv/twitch-chat-controller/src/structures"
	"github.com/seventv/twitch-chat-controller/src/svc/mongo"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"k8s.io/client-go/util/retry"
)

type autoScaler struct {
	gCtx global.Context
	mtx  sync.Mutex

	// a map of edge ids to a list of chnanels in said edge
	edges []map[string]structures.Channel
	// a map of channel id to edge id
	channels map[string]structures.Channel

	ready     chan struct{}
	readyOnce sync.Once

	loader *loaders.AllocateChannel
}

func New(gCtx global.Context) instance.AutoScaler {
	a := &autoScaler{
		gCtx:     gCtx,
		edges:    []map[string]structures.Channel{},
		channels: map[string]structures.Channel{},
		ready:    make(chan struct{}),
	}
	a.loader = loaders.NewAllocateChannel(loaders.AllocateChannelConfig{
		Fetch: func(channels []*pb.Channel) ([]string, []error) {
			ctx := context.TODO()

			a.mtx.Lock()
			defer a.mtx.Unlock()

			ret, errs := make([]string, len(channels)), make([]error, len(channels))

			mp := map[string]*pb.Channel{}
			filteredIDs := []string{}
			for _, v := range channels {
				if _, ok := mp[v.Id]; !ok {
					filteredIDs = append(filteredIDs, v.Id)
				}

				mp[v.Id] = v
			}
			if len(filteredIDs) == 0 {
				return ret, errs
			}

			users, _ := a.gCtx.Inst().Twitch.GetUsers(filteredIDs)
			userIdx := 0
			edgeIdx := 0

			allocations := map[int][]structures.Channel{}
			newChannels := []mongo.WriteModel{}

		users:
			for userIdx < len(users) {
				if users[userIdx].ID == "" {
					userIdx++
					continue
				}

				for edgeIdx < len(a.edges) {
					usr := users[userIdx]

					preset := false
					presetEdgeIdx := edgeIdx
					if channel, ok := a.channels[usr.ID]; ok {
						presetEdgeIdx = int(channel.EdgeNode)
						preset = true
					}

					channels := a.edges[presetEdgeIdx]

					if len(channels) < gCtx.Config().Irc.ChannelLimit || preset {
						channel := structures.Channel{
							TwitchID:    usr.ID,
							TwitchLogin: usr.Login,
							Priority:    mp[usr.ID].Priority,
							EdgeNode:    int32(presetEdgeIdx),
						}

						channels[usr.ID] = channel
						a.channels[usr.ID] = channel
						allocations[presetEdgeIdx] = append(allocations[presetEdgeIdx], channel)

						operation := mongo.NewUpdateOneModel()
						operation.SetFilter(bson.M{
							"twitch_id": channel.TwitchID,
						})
						operation.SetUpdate(bson.M{
							"$set": bson.M{
								"twitch_id":    channel.TwitchID,
								"twitch_login": channel.TwitchLogin,
								"priority":     channel.Priority,
								"edge_node":    channel.EdgeNode,
							},
						})
						operation.SetUpsert(true)

						newChannels = append(newChannels, operation)

						userIdx++
						continue users
					}

					if presetEdgeIdx == edgeIdx {
						edgeIdx++
					}
				}

				if edgeIdx == len(a.edges) {
					// we need a new edge node
					// we at this point allocate the channels to an edge node that does not exist.
					// and then after everything is allocated we do a rescale.
					a.edges = append(a.edges, map[string]structures.Channel{})
				}
			}

			_, err := a.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).BulkWrite(ctx, newChannels)
			if err != nil {
				logrus.Fatal(err)
			}

			size := int32(len(a.edges))
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				statefulSet, err := a.gCtx.Inst().K8S.GetStatefulSet(ctx, a.gCtx.Config().K8S.SatefulsetName)
				if err != nil {
					// unknown as to why this error occured.
					// todo fix this error
					logrus.Fatal(err)
				}

				if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas < size {
					// rescale
					statefulSet.Spec.Replicas = utils.PointerOf(int32(size))
					_, err := a.gCtx.Inst().K8S.UpdateStatefulSet(context.TODO(), statefulSet)
					return err
				}

				return nil
			}); err != nil {
				logrus.Fatal(err)
			}

			for i, users := range allocations {
				a.gCtx.Inst().Events.Publish(fmt.Sprintf("edge-update:%d", i), users)
			}

			return ret, errs
		},
		Wait: time.Second,
	})

	return a
}

func (a *autoScaler) Load() error {
	ctx := context.TODO()

	cur, err := a.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).Find(ctx, bson.M{})
	if err != nil {
		return err
	}

	channels := []structures.Channel{}
	if err = cur.All(ctx, &channels); err != nil {
		return err
	}

	for _, v := range channels {
		a.channels[v.TwitchID] = v
		if len(a.edges) < int(v.EdgeNode)+1 {
			edges := make([]map[string]structures.Channel, int(v.EdgeNode)+1)
			copy(edges, a.edges)
			for i, v := range edges {
				if v == nil {
					edges[i] = map[string]structures.Channel{}
				}
			}
			a.edges = edges
		}

		a.edges[v.EdgeNode][v.TwitchID] = v
	}

	a.readyOnce.Do(func() {
		close(a.ready)
	})

	size := int32(len(a.edges))
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		statefulSet, err := a.gCtx.Inst().K8S.GetStatefulSet(ctx, a.gCtx.Config().K8S.SatefulsetName)
		if err != nil {
			// unknown as to why this error occured.
			// todo fix this error
			logrus.Fatal(err)
		}

		if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas != size {
			// rescale
			statefulSet.Spec.Replicas = utils.PointerOf(int32(size))
			_, err := a.gCtx.Inst().K8S.UpdateStatefulSet(context.TODO(), statefulSet)
			return err
		}

		return nil
	}); err != nil {
		logrus.Fatal(err)
	}

	for i, users := range a.edges {
		a.gCtx.Inst().Events.Publish(fmt.Sprintf("edge-update:%d", i), users)
	}

	return nil
}

func (a *autoScaler) AllocateChannels(chnanels []*pb.Channel) error {
	_, errs := a.loader.LoadAll(chnanels)
	return errs[0]
}
