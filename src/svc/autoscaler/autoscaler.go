package autoscaler

import (
	"context"
	"sync"
	"time"

	"github.com/SevenTV/Common/dataloader"
	"github.com/SevenTV/Common/sync_map"
	"github.com/SevenTV/Common/utils"
	pb "github.com/seventv/twitch-edge/protobuf/twitch_edge/v1"
	"github.com/seventv/twitch-edge/src/global"
	"github.com/seventv/twitch-edge/src/instance"
	"github.com/seventv/twitch-edge/src/structures"
	"github.com/seventv/twitch-edge/src/svc/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

type autoScaler struct {
	gCtx global.Context

	// a map of edge ids to a list of chnanels in said edge
	edges   []map[string]structures.Channel
	edgeIdx int
	mtx     sync.Mutex

	// a map of channel id to edge id
	channels sync_map.Map[string, structures.Channel]

	ready     chan struct{}
	readyOnce sync.Once

	loader *dataloader.DataLoader[*pb.Channel, string]
}

func New(gCtx global.Context) instance.AutoScaler {
	a := &autoScaler{
		gCtx:  gCtx,
		ready: make(chan struct{}),
	}
	a.loader = dataloader.New(dataloader.Config[*pb.Channel, string]{
		Fetch: func(channels []*pb.Channel) ([]string, []error) {
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

			if len(users) == 0 {
				return ret, errs
			}

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
					if channel, ok := a.channels.Load(usr.ID); ok {
						presetEdgeIdx = int(channel.EdgeNode)
						preset = true
					}

					channels := a.edges[presetEdgeIdx]

					if len(channels) < gCtx.Config().Master.Irc.ChannelLimitPerSlave || preset {
						channel := structures.Channel{
							TwitchID:    usr.ID,
							TwitchLogin: usr.Login,
							Priority:    mp[usr.ID].Priority,
							EdgeNode:    int32(presetEdgeIdx),
						}

						channels[usr.ID] = channel
						a.channels.Store(usr.ID, channel)
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

			attempts := 0
			for {
				if attempts != 0 {
					time.Sleep(time.Millisecond * 500)
					if attempts > 10 {
						zap.S().Fatal("failed to update mongo")
					}
				}
				ctx, cancel := context.WithTimeout(gCtx, time.Second*30)
				_, err := a.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).BulkWrite(ctx, newChannels)
				cancel()
				if err != nil {
					zap.S().Errorw("Update mongo failed",
						"error", err,
					)
					attempts++
					continue
				}
				break
			}

			ctx, cancel := context.WithTimeout(gCtx, time.Second*15)
			a.rescaleUnsafe(ctx, int32(len(a.edges)))
			cancel()

			for idx, channels := range allocations {
				zap.S().Infow("allocated channels to node",
					"length", len(channels),
					"node_idx", idx,
				)
				go a.gCtx.Inst().EventEmitter.PublishEdgeChannelUpdate(idx, channels)
			}

			return ret, errs
		},
		MaxBatch: 2000,
		Wait:     time.Millisecond * 500,
	})

	return a
}

func (a *autoScaler) Load() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	cur, err := a.gCtx.Inst().Mongo.Collection(mongo.CollectionNameChannels).Find(ctx, bson.M{})
	cancel()
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
	for _, v := range channels {
		a.channels.Store(v.TwitchID, v)
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

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	a.rescaleUnsafe(ctx, int32(len(a.edges)))
	cancel()

	a.readyOnce.Do(func() {
		close(a.ready)
	})

	for idx, channelsMp := range a.edges {
		_, channels := utils.DestructureMap(channelsMp)
		zap.S().Infow("loaded channels for node",
			"length", len(channels),
			"node_idx", idx,
		)
		go a.gCtx.Inst().EventEmitter.PublishEdgeChannelUpdate(idx, channels)
	}
	a.mtx.Unlock()

	return nil
}

func (a *autoScaler) AllocateChannels(channels []*pb.Channel) error {
	_, errs := a.loader.LoadAll(channels)
	return errs[0]
}

func (a *autoScaler) GetChannelsForEdge(idx int) []structures.Channel {
	a.mtx.Lock()
	if len(a.edges) <= idx {
		a.mtx.Unlock()
		return nil
	}

	channels := make([]structures.Channel, len(a.edges[idx]))
	i := 0
	for _, v := range a.edges[idx] {
		channels[i] = v
		i++
	}

	a.mtx.Unlock()

	return channels
}

func (a *autoScaler) rescaleUnsafe(ctx context.Context, size int32) {
	if !a.gCtx.Config().Master.K8S.Enabled {
		return
	}
	attempts := 0
	for {
		if attempts > 1 {
			time.Sleep(time.Millisecond * 800)
			if attempts > 20 {
				zap.S().Fatal("failed k8s too many times")
			}
		}
		statefulSet, err := a.gCtx.Inst().K8S.GetStatefulSet(ctx, a.gCtx.Config().Master.K8S.SatefulsetName)
		if err != nil {
			zap.S().Errorw("failed to get k8s api",
				"error", err,
				"attempts", attempts,
			)
			attempts++
			continue
		}

		if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas != size {
			// rescale
			statefulSet.Spec.Replicas = utils.PointerOf(int32(size))
			_, err := a.gCtx.Inst().K8S.UpdateStatefulSet(ctx, statefulSet)
			if err != nil {
				zap.S().Errorw("failed to get k8s api",
					"error", err,
					"attempts", attempts,
				)
				attempts++
				continue
			}
		}

		return
	}

}
