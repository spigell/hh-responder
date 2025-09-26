package logger

import (
	"strings"

	"go.uber.org/zap"
)

const (
	// FieldProvider is the structured log field key for the AI provider name.
	FieldProvider = "ai_provider"
	// FieldModel is the structured log field key for the AI model identifier.
	FieldModel = "ai_model"
)

// StringField describes a string-valued structured logging field.
type StringField struct {
	Key   string
	Value string
}

// StringFields converts the provided key/value pairs into zap fields, trimming
// whitespace and omitting entries with empty keys or values.
func StringFields(fields ...StringField) []zap.Field {
	result := make([]zap.Field, 0, len(fields))
	for _, field := range fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			continue
		}

		value := strings.TrimSpace(field.Value)
		if value == "" {
			continue
		}

		result = append(result, zap.String(key, value))
	}

	return result
}

// WithFields safely attaches the provided fields to the logger.
// If the logger is nil or no fields are supplied, the input logger is returned
// unchanged, defaulting to a no-op logger when nil.
func WithFields(logger *zap.Logger, fields ...zap.Field) *zap.Logger {
	if logger == nil {
		logger = zap.NewNop()
	}

	if len(fields) == 0 {
		return logger
	}

	return logger.With(fields...)
}

// CommonFields returns standard zap fields that describe the AI provider and model.
// Empty values are ignored to keep log entries compact when information is missing.
func CommonFields(provider, model string) []zap.Field {
	return StringFields(
		StringField{Key: FieldProvider, Value: provider},
		StringField{Key: FieldModel, Value: model},
	)
}

// WithCommonFields attaches the common AI fields to the provided logger.
// If the logger is nil, a no-op logger is created to avoid panics.
func WithCommonFields(logger *zap.Logger, provider, model string) *zap.Logger {
	fields := CommonFields(provider, model)
	return WithFields(logger, fields...)
}
