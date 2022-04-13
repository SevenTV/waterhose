package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	cMongo "github.com/SevenTV/Common/mongo"
	cRedis "github.com/SevenTV/Common/redis"
	"github.com/bugsnag/panicwrap"
	"github.com/seventv/twitch-chat-controller/src/app"
	"github.com/seventv/twitch-chat-controller/src/configure"
	"github.com/seventv/twitch-chat-controller/src/global"
	"github.com/seventv/twitch-chat-controller/src/svc/autoscaler"
	"github.com/seventv/twitch-chat-controller/src/svc/events"
	"github.com/seventv/twitch-chat-controller/src/svc/k8s"
	"github.com/seventv/twitch-chat-controller/src/svc/ratelimiter"
	"github.com/seventv/twitch-chat-controller/src/svc/redis"
	"github.com/seventv/twitch-chat-controller/src/svc/twitch"
	"github.com/sirupsen/logrus"
)

var (
	Version = "development"
	Unix    = ""
	Time    = "unknown"
	User    = "unknown"
)

func init() {
	debug.SetGCPercent(2000)
	if i, err := strconv.Atoi(Unix); err == nil {
		Time = time.Unix(int64(i), 0).Format(time.RFC3339)
	}
}

func main() {
	config := configure.New()

	exitStatus, err := panicwrap.BasicWrap(func(s string) {
		logrus.Error(s)
	})
	if err != nil {
		logrus.Error("failed to setup panic handler: ", err)
		os.Exit(2)
	}

	if exitStatus >= 0 {
		os.Exit(exitStatus)
	}

	if !config.NoHeader {
		logrus.Info("7TV Twitch-Chat-Controller")
		logrus.Infof("Version: %s", Version)
		logrus.Infof("build.Time: %s", Time)
		logrus.Infof("build.User: %s", User)
	}

	logrus.Debug("MaxProcs: ", runtime.GOMAXPROCS(0))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	c, cancel := context.WithCancel(context.Background())

	gCtx := global.New(c, config)

	{
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

	{
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

	{
		gCtx.Inst().K8S = k8s.New(gCtx)
	}

	{
		gCtx.Inst().Events = events.New()
	}

	{
		gCtx.Inst().Twitch = twitch.New(gCtx)
	}

	{
		gCtx.Inst().AutoScaler = autoscaler.New(gCtx)
		if err := gCtx.Inst().AutoScaler.Load(); err != nil {
			logrus.Fatal("failed to load autoscaler: ", err)
		}
	}

	{
		gCtx.Inst().RateLimit = ratelimiter.New()
		go gCtx.Inst().RateLimit.Start()
	}

	appDone := app.New(gCtx)

	logrus.Info("running")

	done := make(chan struct{})
	go func() {
		<-sig
		cancel()
		go func() {
			select {
			case <-time.After(time.Minute):
			case <-sig:
			}
			logrus.Fatal("force shutdown")
		}()

		logrus.Info("shutting down")

		<-appDone

		close(done)
	}()

	<-done

	logrus.Info("shutdown")
	os.Exit(0)
}
