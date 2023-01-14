package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(json bool, debug bool) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	encoding := "console"

	if json {
		encoding = "json"
	}

	if debug {
		level = zapcore.DebugLevel
	}

	cfg := zap.Config{
		Encoding:         encoding,
		Level:            zap.NewAtomicLevelAt(level),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey: "step",

			LevelKey:    "level",
			EncodeLevel: zapcore.LowercaseLevelEncoder,

			TimeKey:    "time",
			EncodeTime: zapcore.RFC3339TimeEncoder,

			CallerKey:    "caller",
			EncodeCaller: zapcore.ShortCallerEncoder,
		},
	}
	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	defer logger.Sync()

	return logger, nil
}
