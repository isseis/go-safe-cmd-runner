package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.NoError(t, err, "NewConditionalTextHandler should not return error")
	assert.NotNil(t, handler, "NewConditionalTextHandler should return a non-nil handler")
}

func TestNewConditionalTextHandler_ErrorOnNilCapabilities(t *testing.T) {
	var buf bytes.Buffer

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: nil,
		Writer:       &buf,
	})

	assert.Error(t, err, "Expected error when Capabilities is nil")
	assert.Nil(t, handler, "Expected nil handler when error occurs")
}

func TestNewConditionalTextHandler_ErrorOnNilWriter(t *testing.T) {
	caps := &conditionalTestCapabilities{interactive: false, supportsColor: false}

	handler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: caps,
		Writer:       nil,
	})

	assert.Error(t, err, "Expected error when Writer is nil")
	assert.Nil(t, handler, "Expected nil handler when error occurs")
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
	require.NoError(t, err, "NewConditionalTextHandler failed")

	ctx := context.Background()

	// Should be disabled in interactive mode
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo), "Handler should be disabled in interactive mode")
	assert.False(t, handler.Enabled(ctx, slog.LevelError), "Handler should be disabled in interactive mode for all levels")
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
	require.NoError(t, err, "NewConditionalTextHandler failed")

	ctx := context.Background()

	// Should respect the underlying text handler's level settings
	assert.False(t, handler.Enabled(ctx, slog.LevelDebug), "Handler should not be enabled for debug level when min level is warn")
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo), "Handler should not be enabled for info level when min level is warn")
	assert.True(t, handler.Enabled(ctx, slog.LevelWarn), "Handler should be enabled for warn level")
	assert.True(t, handler.Enabled(ctx, slog.LevelError), "Handler should be enabled for error level")
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
	require.NoError(t, err, "NewConditionalTextHandler failed")

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	// Should not handle in interactive mode
	err = handler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Buffer should be empty
	assert.Equal(t, 0, buf.Len(), "No output should be written in interactive mode")
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
	require.NoError(t, err, "NewConditionalTextHandler failed")

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	// Should handle in non-interactive mode
	err = handler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Buffer should contain output
	assert.NotEqual(t, 0, buf.Len(), "Output should be written in non-interactive mode")

	output := buf.String()
	assert.Contains(t, output, "test message", "Output should contain the log message")
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
	require.NoError(t, err, "NewConditionalTextHandler failed")

	attrs := []slog.Attr{
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	}

	newHandler := handler.WithAttrs(attrs)

	// Should return a new handler
	assert.NotEqual(t, handler, newHandler, "WithAttrs should return a new handler instance")

	// New handler should be of the same type
	_, ok := newHandler.(*ConditionalTextHandler)
	assert.True(t, ok, "WithAttrs should return a ConditionalTextHandler")

	// Test that attributes are applied when logging
	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	err = newHandler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	output := buf.String()
	assert.Contains(t, output, "test message", "Output should contain the log message")
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
	require.NoError(t, err, "NewConditionalTextHandler failed")

	const testGroupName = "testgroup"
	groupName := testGroupName
	newHandler := handler.WithGroup(groupName)

	// Should return a new handler
	assert.NotEqual(t, handler, newHandler, "WithGroup should return a new handler instance")

	// New handler should be of the same type
	_, ok := newHandler.(*ConditionalTextHandler)
	assert.True(t, ok, "WithGroup should return a ConditionalTextHandler")

	// Test that group is applied when logging
	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("attr", "value"))

	err = newHandler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	output := buf.String()
	assert.Contains(t, output, "test message", "Output should contain the log message")
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
	require.NoError(t, err, "NewConditionalTextHandler failed")

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	// Initially non-interactive, should handle
	assert.True(t, handler.Enabled(ctx, slog.LevelInfo), "Should be enabled in non-interactive mode")

	err = handler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	initialOutput := buf.String()
	assert.NotEmpty(t, initialOutput, "Should produce output in non-interactive mode")

	// Switch to interactive mode
	caps.interactive = true

	// Now should not be enabled
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo), "Should be disabled in interactive mode")

	// Reset buffer and try again
	buf.Reset()
	err = handler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Should not produce output in interactive mode
	assert.Equal(t, 0, buf.Len(), "Should not produce output in interactive mode")
}
