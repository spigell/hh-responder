package logger

import (
	"strings"

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

// TruncateForLog shortens the provided string to the specified limit, appending an ellipsis when truncated.
func TruncateForLog(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}
