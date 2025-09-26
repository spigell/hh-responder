package logger

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestStringFields(t *testing.T) {
	fields := StringFields(
		StringField{Key: "  provider  ", Value: "  Gemini  "},
		StringField{Key: "ignored", Value: "   "},
		StringField{Key: "   ", Value: "empty key"},
	)

	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}

	if fields[0].Key != "provider" || fields[0].String != "Gemini" {
		t.Fatalf("unexpected provider field: %+v", fields[0])
	}

	empty := StringFields()
	if len(empty) != 0 {
		t.Fatalf("expected empty fields, got %d", len(empty))
	}
}

func TestWithFields(t *testing.T) {
	core, observed := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	enriched := WithFields(logger, zap.String("foo", "bar"))
	enriched.Info("test log")

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	ctx := entries[0].ContextMap()
	if ctx["foo"] != "bar" {
		t.Fatalf("expected field to be bar, got %q", ctx["foo"])
	}

	enriched = WithFields(nil, zap.String("baz", "qux"))
	if enriched == nil {
		t.Fatalf("expected fallback logger when nil provided")
	}

	// Ensure logging with the fallback logger does not panic.
	enriched.Info("another log")
}

func TestCommonFields(t *testing.T) {
	fields := CommonFields("  Gemini  ", "model-v1")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	if fields[0].Key != FieldProvider || fields[0].String != "Gemini" {
		t.Fatalf("unexpected provider field: %+v", fields[0])
	}

	if fields[1].Key != FieldModel || fields[1].String != "model-v1" {
		t.Fatalf("unexpected model field: %+v", fields[1])
	}

	empty := CommonFields("", "")
	if len(empty) != 0 {
		t.Fatalf("expected empty fields, got %d", len(empty))
	}
}

func TestWithCommonFields(t *testing.T) {
	core, observed := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	enriched := WithCommonFields(logger, "gemini", "model-x")
	enriched.Info("test log")

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	ctx := entries[0].ContextMap()
	if ctx[FieldProvider] != "gemini" {
		t.Fatalf("expected provider field to be gemini, got %q", ctx[FieldProvider])
	}

	if ctx[FieldModel] != "model-x" {
		t.Fatalf("expected model field to be model-x, got %q", ctx[FieldModel])
	}

	enriched = WithCommonFields(nil, "gemini", "model-x")
	if enriched == nil {
		t.Fatalf("expected fallback logger when nil provided")
	}

	// Ensure logging with the fallback logger does not panic.
	enriched.Info("another log")
}
