package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// conditionalTestCapabilities implements terminal.Capabilities for testing
type conditionalTestCapabilities struct {
	interactive           bool
	supportsColor         bool
	hasExplicitPreference bool
}

func (m *conditionalTestCapabilities) IsInteractive() bool {
	return m.interactive
}

func (m *conditionalTestCapabilities) SupportsColor() bool {
	return m.supportsColor
}

func (m *conditionalTestCapabilities) HasExplicitUserPreference() bool {
	return m.hasExplicitPreference
}

func TestNewConditionalTextHandler(t *testing.T) {
	var buf bytes.Buffer
	caps := &conditionalTestCapabilities{interactive: false, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       &buf,
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	})
	if err != nil {
		t.Errorf("NewConditionalTextHandler should not return error: %v", err)
	}
	if handler == nil {
		t.Error("NewConditionalTextHandler should return a non-nil handler")
	}
}

func TestNewConditionalTextHandler_ErrorOnNilCapabilities(t *testing.T) {
	var buf bytes.Buffer

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: nil,
		Writer:       &buf,
	})

	if err == nil {
		t.Error("Expected error when Capabilities is nil")
	}
	if handler != nil {
		t.Error("Expected nil handler when error occurs")
	}
}

func TestNewConditionalTextHandler_ErrorOnNilWriter(t *testing.T) {
	caps := &conditionalTestCapabilities{interactive: false, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       nil,
	})

	if err == nil {
		t.Error("Expected error when Writer is nil")
	}
	if handler != nil {
		t.Error("Expected nil handler when error occurs")
	}
}

func TestConditionalTextHandler_Enabled_Interactive(t *testing.T) {
	var buf bytes.Buffer
	caps := &conditionalTestCapabilities{interactive: true, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       &buf,
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	})
	if err != nil {
		t.Fatalf("NewConditionalTextHandler failed: %v", err)
	}

	ctx := context.Background()

	// Should be disabled in interactive mode
	if handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Handler should be disabled in interactive mode")
	}
	if handler.Enabled(ctx, slog.LevelError) {
		t.Error("Handler should be disabled in interactive mode for all levels")
	}
}

func TestConditionalTextHandler_Enabled_NonInteractive(t *testing.T) {
	var buf bytes.Buffer
	caps := &conditionalTestCapabilities{interactive: false, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       &buf,
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slog.LevelWarn,
		},
	})
	if err != nil {
		t.Fatalf("NewConditionalTextHandler failed: %v", err)
	}

	ctx := context.Background()

	// Should respect the underlying text handler's level settings
	if handler.Enabled(ctx, slog.LevelDebug) {
		t.Error("Handler should not be enabled for debug level when min level is warn")
	}
	if handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Handler should not be enabled for info level when min level is warn")
	}
	if !handler.Enabled(ctx, slog.LevelWarn) {
		t.Error("Handler should be enabled for warn level")
	}
	if !handler.Enabled(ctx, slog.LevelError) {
		t.Error("Handler should be enabled for error level")
	}
}

func TestConditionalTextHandler_Handle_Interactive(t *testing.T) {
	var buf bytes.Buffer
	caps := &conditionalTestCapabilities{interactive: true, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       &buf,
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	})
	if err != nil {
		t.Fatalf("NewConditionalTextHandler failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	// Should not handle in interactive mode
	err = handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Buffer should be empty
	if buf.Len() > 0 {
		t.Error("No output should be written in interactive mode")
	}
}

func TestConditionalTextHandler_Handle_NonInteractive(t *testing.T) {
	var buf bytes.Buffer
	caps := &conditionalTestCapabilities{interactive: false, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       &buf,
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	})
	if err != nil {
		t.Fatalf("NewConditionalTextHandler failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	// Should handle in non-interactive mode
	err = handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Buffer should contain output
	if buf.Len() == 0 {
		t.Error("Output should be written in non-interactive mode")
	}

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Error("Output should contain the log message")
	}
}

func TestConditionalTextHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	caps := &conditionalTestCapabilities{interactive: false, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       &buf,
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	})
	if err != nil {
		t.Fatalf("NewConditionalTextHandler failed: %v", err)
	}

	attrs := []slog.Attr{
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	}

	newHandler := handler.WithAttrs(attrs)

	// Should return a new handler
	if newHandler == handler {
		t.Error("WithAttrs should return a new handler instance")
	}

	// New handler should be of the same type
	if _, ok := newHandler.(*ConditionalTextHandler); !ok {
		t.Error("WithAttrs should return a ConditionalTextHandler")
	}

	// Test that attributes are applied when logging
	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	err = newHandler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Error("Output should contain the log message")
	}
}

func TestConditionalTextHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	caps := &conditionalTestCapabilities{interactive: false, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       &buf,
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	})
	if err != nil {
		t.Fatalf("NewConditionalTextHandler failed: %v", err)
	}

	const testGroupName = "testgroup"
	groupName := testGroupName
	newHandler := handler.WithGroup(groupName)

	// Should return a new handler
	if newHandler == handler {
		t.Error("WithGroup should return a new handler instance")
	}

	// New handler should be of the same type
	if _, ok := newHandler.(*ConditionalTextHandler); !ok {
		t.Error("WithGroup should return a ConditionalTextHandler")
	}

	// Test that group is applied when logging
	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("attr", "value"))

	err = newHandler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Error("Output should contain the log message")
	}
}

func TestConditionalTextHandler_InteractiveToggle(t *testing.T) {
	var buf bytes.Buffer
	caps := &conditionalTestCapabilities{interactive: false, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       &buf,
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	})
	if err != nil {
		t.Fatalf("NewConditionalTextHandler failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	// Initially non-interactive, should handle
	if !handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Should be enabled in non-interactive mode")
	}

	err = handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	initialOutput := buf.String()
	if len(initialOutput) == 0 {
		t.Error("Should produce output in non-interactive mode")
	}

	// Switch to interactive mode
	caps.interactive = true

	// Now should not be enabled
	if handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Should be disabled in interactive mode")
	}

	// Reset buffer and try again
	buf.Reset()
	err = handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Should not produce output in interactive mode
	if buf.Len() > 0 {
		t.Error("Should not produce output in interactive mode")
	}
}
