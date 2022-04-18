package modes

import (
	"context"
	"time"

	cMongo "github.com/SevenTV/Common/mongo"
	cRedis "github.com/SevenTV/Common/redis"
	"go.uber.org/zap"

	"github.com/seventv/waterhose/src/global"
	"github.com/seventv/waterhose/src/modes/master"
	"github.com/seventv/waterhose/src/modes/slave"
	"github.com/seventv/waterhose/src/monitoring"
	"github.com/seventv/waterhose/src/svc/autoscaler"
	"github.com/seventv/waterhose/src/svc/events"
	"github.com/seventv/waterhose/src/svc/k8s"
	"github.com/seventv/waterhose/src/svc/ratelimiter"
	"github.com/seventv/waterhose/src/svc/redis"
	"github.com/seventv/waterhose/src/svc/twitch"
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
		gCtx.Inst().Monitoring = monitoring.NewPrometheus(gCtx)
	}

	if isMaster || isSlave {
		ctx, cancel := context.WithTimeout(gCtx, time.Second*15)
		redisInst, err := cRedis.Setup(ctx, cRedis.SetupOptions{
			MasterName: gCtx.Config().Redis.MasterName,
			Username:   gCtx.Config().Redis.Username,
			Password:   gCtx.Config().Redis.Password,
			Database:   gCtx.Config().Redis.Database,
			Addresses:  gCtx.Config().Redis.Addresses,
			Sentinel:   gCtx.Config().Redis.Sentinel,
		})
		cancel()
		if err != nil {
			zap.S().Fatalw("failed to connect to redis",
				"error", err,
			)
		}

		gCtx.Inst().Redis = redis.WrapRedis(redisInst)
	}

	if isMaster {
		ctx, cancel := context.WithTimeout(gCtx, time.Second*15)
		mongoInst, err := cMongo.Setup(ctx, cMongo.SetupOptions{
			URI:      gCtx.Config().Master.Mongo.URI,
			DB:       gCtx.Config().Master.Mongo.Database,
			Direct:   gCtx.Config().Master.Mongo.Direct,
			CollSync: false,
		})
		cancel()
		if err != nil {
			zap.S().Fatal("failed to connect to mongo",
				"error", err,
			)
		}

		gCtx.Inst().Mongo = mongoInst
	}

	if isMaster && gCtx.Config().Master.K8S.Enabled {
		gCtx.Inst().K8S = k8s.New(gCtx)
		zap.S().Info("k8s, ok")
	}

	if isMaster || isSlave {
		gCtx.Inst().EventEmitter = events.New()
		zap.S().Info("eventemitter, ok")
	}

	if isMaster {
		gCtx.Inst().Twitch = twitch.New(gCtx)
		zap.S().Info("twitch, ok")
	}

	if isMaster {
		gCtx.Inst().AutoScaler = autoscaler.New(gCtx)
		if err := gCtx.Inst().AutoScaler.Load(); err != nil {
			zap.S().Fatalw("failed to load autoscaler: ",
				"error", err,
			)
		}

		zap.S().Info("autoscaler, ok")
	}

	if isMaster {
		gCtx.Inst().RateLimit = ratelimiter.New()
		go gCtx.Inst().RateLimit.Start()
		zap.S().Info("ratelimiter, ok")
	}
}
