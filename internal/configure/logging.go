package configure

import (
	"io"
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	log.SetOutput(io.Discard)
}

func initLogging(level string) {
	var lvl zapcore.Level
	switch level {
	case "debug":
		lvl = zap.DebugLevel
	case "info":
		lvl = zap.InfoLevel
	case "warn":
		lvl = zap.WarnLevel
	case "error":
		lvl = zap.ErrorLevel
	case "panic":
		lvl = zap.PanicLevel
	case "fatal":
		lvl = zap.FatalLevel
	default:
		lvl = zap.InfoLevel
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	logger, _ := cfg.Build()

	zap.ReplaceGlobals(logger)
}
