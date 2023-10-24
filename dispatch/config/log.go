package config

import (
	_ "github.com/heyvito/zap-human"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitializeLogging(ctx *Context) error {
	var (
		logger *zap.Logger
		err    error
	)
	if ctx.Debug {
		config := zap.NewDevelopmentConfig()
		config.Encoding = "human"
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.DisableCaller = true
		logger, err = config.Build()
	} else {
		logger, err = zap.NewProductionConfig().Build()
	}
	if err != nil {
		return err
	}
	_ = zap.ReplaceGlobals(logger)
	return nil
}
