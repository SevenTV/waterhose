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

	"github.com/bugsnag/panicwrap"
	"github.com/seventv/waterhose/src/configure"
	"github.com/seventv/waterhose/src/global"
	"github.com/seventv/waterhose/src/modes"
	"go.uber.org/zap"
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

	if !config.IsMaster() && !config.IsSlave() {
		zap.S().Fatalw("invalid startup mode specified: ",
			"mode", config.Mode,
		)
	}

	exitStatus, err := panicwrap.BasicWrap(func(s string) {
		zap.S().Error("panic: ", s)
	})
	if err != nil {
		zap.S().Errorw("failed to setup panic handler: ",
			"error", err,
		)
		os.Exit(2)
	}

	if exitStatus >= 0 {
		os.Exit(exitStatus)
	}

	if !config.NoHeader {
		if config.IsMaster() {
			zap.S().Info("7TV WaterHose Master")
		} else if config.IsSlave() {
			zap.S().Info("7TV WaterHose Slave")
		}
		zap.S().Infof("Version: %s", Version)
		zap.S().Infof("build.Time: %s", Time)
		zap.S().Infof("build.User: %s", User)
	}

	zap.S().Debug("MaxProcs: ", runtime.GOMAXPROCS(0))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	c, cancel := context.WithCancel(context.Background())

	gCtx := global.New(c, config)

	var appDone <-chan struct{}
	done := make(chan struct{})
	go func() {
		<-sig
		cancel()
		go func() {
			select {
			case <-time.After(time.Minute):
			case <-sig:
			}
			zap.S().Fatal("force shutdown")
		}()

		zap.S().Info("shutting down")

		if appDone != nil {
			<-appDone
		}

		close(done)
	}()

	appDone = modes.New(gCtx)

	zap.S().Info("running")

	<-done

	zap.S().Info("shutdown")
	os.Exit(0)
}
