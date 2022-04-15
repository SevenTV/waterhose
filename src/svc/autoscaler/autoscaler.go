package autoscaler

import (
	"context"
	"sync"
	"time"

	"github.com/SevenTV/Common/utils"
	"github.com/seventv/twitch-edge/loaders"
	pb "github.com/seventv/twitch-edge/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-edge/src/global"
	"github.com/seventv/twitch-edge/src/instance"
	"github.com/seventv/twitch-edge/src/structures"
	"github.com/seventv/twitch-edge/src/svc/mongo"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"k8s.io/client-go/util/retry"
)

type autoScaler struct {
	gCtx global.Context
	mtx  sync.Mutex

	// a map of edge ids to a list of chnanels in said edge
	edges   []map[string]structures.Channel
	edgeIdx int

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

			allocations := map[int][]structures.Channel{}
			newChannels := []mongo.WriteModel{}

		users:
			for userIdx < len(users) {
				if users[userIdx].ID == "" {
					userIdx++
					continue
				}

				for a.edgeIdx < len(a.edges) {
					usr := users[userIdx]

					preset := false
					presetEdgeIdx := a.edgeIdx
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
								"last_updated": time.Now(),
							},
						})
						operation.SetUpsert(true)

						newChannels = append(newChannels, operation)

						userIdx++
						continue users
					}

					if presetEdgeIdx == a.edgeIdx {
						a.edgeIdx++
					}
				}

				if a.edgeIdx == len(a.edges) {
					// we need a new edge node
					// we at this point allocate the channels to an edge node that does not exist.
					// and then after everything is allocated we do a rescale.
					a.edges = append(a.edges, map[string]structures.Channel{})
				}
			}

			_, err := a.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).BulkWrite(ctx, newChannels)
			if err != nil {
				// TODO
				logrus.Fatal(err)
			}

			a.rescaleUnsafe(ctx, int32(len(a.edges)))

			for idx, channels := range allocations {
				a.gCtx.Inst().EventEmitter.PublishEdgeChannelUpdate(idx, channels)
			}

			return ret, errs
		},
		Wait: time.Second,
	})

	return a
}

func (a *autoScaler) Load() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	cur, err := a.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).Find(ctx, bson.M{})
	if err != nil {
		return err
	}

	channels := []structures.Channel{}
	{
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		if err = cur.All(ctx, &channels); err != nil {
			return err
		}
	}

	a.mtx.Lock()
	defer a.mtx.Unlock()
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

	a.rescaleUnsafe(ctx, int32(len(a.edges)))

	for idx, channelsMp := range a.edges {
		_, channels := utils.DestructureMap(channelsMp)
		a.gCtx.Inst().EventEmitter.PublishEdgeChannelUpdate(idx, channels)
	}

	return nil
}

func (a *autoScaler) AllocateChannels(channels []*pb.Channel) error {
	_, errs := a.loader.LoadAll(channels)
	return errs[0]
}

func (a *autoScaler) GetChannelsForEdge(idx int) []structures.Channel {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if len(a.edges) <= idx {
		return nil
	}

	channels := make([]structures.Channel, len(a.edges[idx]))
	i := 0
	for _, v := range a.edges[idx] {
		channels[i] = v
		i++
	}

	return channels
}

func (a *autoScaler) rescaleUnsafe(ctx context.Context, size int32) {
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		statefulSet, err := a.gCtx.Inst().K8S.GetStatefulSet(ctx, a.gCtx.Config().K8S.SatefulsetName)
		if err != nil {
			// unknown as to why this error occured.
			// TODO fix this error
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
		// TODO
		logrus.Fatal(err)
	}
}
