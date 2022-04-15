package modes

import (
	"context"
	"time"

	cMongo "github.com/SevenTV/Common/mongo"
	cRedis "github.com/SevenTV/Common/redis"

	"github.com/seventv/twitch-edge/src/global"
	"github.com/seventv/twitch-edge/src/modes/master"
	"github.com/seventv/twitch-edge/src/modes/slave"
	"github.com/seventv/twitch-edge/src/svc/autoscaler"
	"github.com/seventv/twitch-edge/src/svc/events"
	"github.com/seventv/twitch-edge/src/svc/k8s"
	"github.com/seventv/twitch-edge/src/svc/ratelimiter"
	"github.com/seventv/twitch-edge/src/svc/redis"
	"github.com/seventv/twitch-edge/src/svc/twitch"

	"github.com/sirupsen/logrus"
)

func New(gCtx global.Context) <-chan struct{} {
	setupGlobal(gCtx)

	if gCtx.Config().IsMaster() {
		return master.New(gCtx)
	} else {
		return slave.New(gCtx)
	}
}

func setupGlobal(gCtx global.Context) {
	isMaster := gCtx.Config().IsMaster()
	isSlave := gCtx.Config().IsSlave()

	if isMaster || isSlave {
		ctx, cancel := context.WithTimeout(gCtx, time.Second*15)
		redisInst, err := cRedis.Setup(ctx, cRedis.SetupOptions{
			Username:  gCtx.Config().Redis.Username,
			Password:  gCtx.Config().Redis.Password,
			Database:  gCtx.Config().Redis.Database,
			Addresses: gCtx.Config().Redis.Addresses,
			Sentinel:  gCtx.Config().Redis.Sentinel,
		})
		cancel()
		if err != nil {
			logrus.WithError(err).Fatal("failed to connect to redis")
		}

		gCtx.Inst().Redis = redis.WrapRedis(redisInst)
	}

	if isMaster {
		ctx, cancel := context.WithTimeout(gCtx, time.Second*15)
		mongoInst, err := cMongo.Setup(ctx, cMongo.SetupOptions{
			URI:      gCtx.Config().Mongo.URI,
			DB:       gCtx.Config().Mongo.Database,
			Direct:   gCtx.Config().Mongo.Direct,
			CollSync: false,
		})
		cancel()
		if err != nil {
			logrus.WithError(err).Fatal("failed to connect to mongo")
		}

		gCtx.Inst().Mongo = mongoInst
	}

	if isMaster {
		gCtx.Inst().K8S = k8s.New(gCtx)
		logrus.Info("k8s, ok")
	}

	if isMaster || isSlave {
		gCtx.Inst().EventEmitter = events.New()
		logrus.Info("eventemitter, ok")
	}

	if isMaster {
		gCtx.Inst().Twitch = twitch.New(gCtx)
		logrus.Info("twitch, ok")
	}

	if isMaster {
		gCtx.Inst().AutoScaler = autoscaler.New(gCtx)
		if err := gCtx.Inst().AutoScaler.Load(); err != nil {
			logrus.Fatal("failed to load autoscaler: ", err)
		}

		logrus.Info("autoscaler, ok")
	}

	if isMaster {
		gCtx.Inst().RateLimit = ratelimiter.New()
		go gCtx.Inst().RateLimit.Start()
		logrus.Info("ratelimiter, ok")
	}
}
