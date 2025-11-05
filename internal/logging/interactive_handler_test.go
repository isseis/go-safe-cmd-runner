package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// interactiveTestCapabilities implements terminal.Capabilities for testing
type interactiveTestCapabilities struct {
	interactive           bool
	supportsColor         bool
	hasExplicitPreference bool
}

func (m *interactiveTestCapabilities) IsInteractive() bool {
	return m.interactive
}

func (m *interactiveTestCapabilities) SupportsColor() bool {
	return m.supportsColor
}

func (m *interactiveTestCapabilities) HasExplicitUserPreference() bool {
	return m.hasExplicitPreference
}

// interactiveTestMessageFormatter implements MessageFormatter for testing
type interactiveTestMessageFormatter struct {
	formatRecordCalled bool
	formatHintCalled   bool
	recordMessage      string
	capturedRecord     *slog.Record
}

func (m *interactiveTestMessageFormatter) FormatRecordWithColor(record slog.Record, useColor bool) string {
	m.formatRecordCalled = true
	m.recordMessage = record.Message
	// Capture the record for attribute inspection
	recordCopy := record.Clone()
	m.capturedRecord = &recordCopy
	if useColor {
		return "@ " + record.Message
	}
	return "[FORMATTED] " + record.Message
}

func (m *interactiveTestMessageFormatter) FormatRecordInteractive(record slog.Record, useColor bool) string {
	m.formatRecordCalled = true
	m.recordMessage = record.Message
	// Capture the record for attribute inspection
	recordCopy := record.Clone()
	m.capturedRecord = &recordCopy
	if useColor {
		return ">> " + record.Message
	}
	return "[INTERACTIVE] " + record.Message
}

func (m *interactiveTestMessageFormatter) FormatLogFileHint(lineNumber int, useColor bool) string {
	m.formatHintCalled = true
	if lineNumber <= 0 {
		return ""
	}
	if useColor {
		return "* Line " + string(rune('0'+lineNumber))
	}
	return "HINT: Line " + string(rune('0'+lineNumber))
}

// GetAttribute returns the value of an attribute by key from the captured record
func (m *interactiveTestMessageFormatter) GetAttribute(key string) (slog.Value, bool) {
	if m.capturedRecord == nil {
		return slog.Value{}, false
	}

	var found bool
	var result slog.Value
	m.capturedRecord.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			result = attr.Value
			found = true
			return false // Stop iteration
		}
		return true // Continue iteration
	})
	return result, found
}

// interactiveTestLogLineTracker implements LogLineTracker for testing
type interactiveTestLogLineTracker struct {
	currentLine int
	getCalled   bool
}

func (m *interactiveTestLogLineTracker) GetCurrentLine() int {
	m.getCalled = true
	return m.currentLine
}

func (m *interactiveTestLogLineTracker) IncrementLine() int {
	m.currentLine++
	return m.currentLine
}

func (m *interactiveTestLogLineTracker) Reset() {
	m.currentLine = 0
}

func TestNewInteractiveHandler(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: true}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	assert.NoError(t, err, "NewInteractiveHandler should not return error")
	assert.NotNil(t, handler, "NewInteractiveHandler should return a non-nil handler")
}

func TestNewInteractiveHandler_ErrorOnMissingDependencies(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: true}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	testCases := []struct {
		name string
		opts InteractiveHandlerOptions
	}{
		{
			name: "nil writer",
			opts: InteractiveHandlerOptions{
				Writer:       nil,
				Capabilities: caps,
				Formatter:    formatter,
				LineTracker:  tracker,
			},
		},
		{
			name: "nil capabilities",
			opts: InteractiveHandlerOptions{
				Writer:       &buf,
				Capabilities: nil,
				Formatter:    formatter,
				LineTracker:  tracker,
			},
		},
		{
			name: "nil formatter",
			opts: InteractiveHandlerOptions{
				Writer:       &buf,
				Capabilities: caps,
				Formatter:    nil,
				LineTracker:  tracker,
			},
		},
		{
			name: "nil line tracker",
			opts: InteractiveHandlerOptions{
				Writer:       &buf,
				Capabilities: caps,
				Formatter:    formatter,
				LineTracker:  nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewInteractiveHandler(tc.opts)
			assert.Error(t, err, "Expected error for %s", tc.name)
			assert.Nil(t, handler, "Expected nil handler for %s", tc.name)
		})
	}
}

func TestInteractiveHandler_Enabled_Interactive(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: true}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelWarn,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	ctx := context.Background()

	// Should be enabled for levels >= configured level in interactive mode
	assert.False(t, handler.Enabled(ctx, slog.LevelDebug), "Should not be enabled for debug level when min level is warn")
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo), "Should not be enabled for info level when min level is warn")
	assert.True(t, handler.Enabled(ctx, slog.LevelWarn), "Should be enabled for warn level")
	assert.True(t, handler.Enabled(ctx, slog.LevelError), "Should be enabled for error level")
}

func TestInteractiveHandler_Enabled_NonInteractive(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: false, supportsColor: false}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	ctx := context.Background()

	// Should be disabled for all levels in non-interactive mode
	assert.False(t, handler.Enabled(ctx, slog.LevelDebug), "Should be disabled in non-interactive mode")
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo), "Should be disabled in non-interactive mode")
	assert.False(t, handler.Enabled(ctx, slog.LevelWarn), "Should be disabled in non-interactive mode")
	assert.False(t, handler.Enabled(ctx, slog.LevelError), "Should be disabled in non-interactive mode")
}

func TestInteractiveHandler_Handle_Interactive(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: true}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{currentLine: 42}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	err = handler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Should call formatter with color support
	assert.True(t, formatter.formatRecordCalled, "Formatter should have been called")
	assert.Equal(t, "test message", formatter.recordMessage, "Formatter received wrong message")

	// Should write formatted output
	output := buf.String()
	assert.Contains(t, output, "test message", "Output should contain formatted message")

	// Should not add hint for non-error levels
	assert.False(t, formatter.formatHintCalled, "Hint formatter should not be called for non-error levels")
}

func TestInteractiveHandler_Handle_NonInteractive(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: false, supportsColor: false}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	err = handler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Should not call formatter in non-interactive mode
	assert.False(t, formatter.formatRecordCalled, "Formatter should not be called in non-interactive mode")

	// Should not write any output
	assert.Zero(t, buf.Len(), "No output should be written in non-interactive mode")
}

func TestInteractiveHandler_Handle_ErrorLevelWithHint(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: false}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{currentLine: 123}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelError, "error message", 0)

	err = handler.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Should call both formatters
	assert.True(t, formatter.formatRecordCalled, "Record formatter should have been called")
	assert.True(t, formatter.formatHintCalled, "Hint formatter should have been called for error level")

	// Should call line tracker
	assert.True(t, tracker.getCalled, "Line tracker should have been called")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.GreaterOrEqual(t, len(lines), 1, "Should have at least one line of output")
}

func TestInteractiveHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: true}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	attrs := []slog.Attr{
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	}

	newHandler := handler.WithAttrs(attrs)

	// Should return a new handler
	assert.NotEqual(t, handler, newHandler, "WithAttrs should return a new handler instance")

	// New handler should be of the same type
	_, ok := newHandler.(*InteractiveHandler)
	assert.True(t, ok, "WithAttrs should return an InteractiveHandler")

	// Test with empty attrs
	sameHandler := handler.WithAttrs(nil)
	assert.Equal(t, handler, sameHandler, "WithAttrs with empty attrs should return same handler")

	sameHandler = handler.WithAttrs([]slog.Attr{})
	assert.Equal(t, handler, sameHandler, "WithAttrs with empty slice should return same handler")
}

func TestInteractiveHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: true}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	const testGroupName = "testgroup"
	groupName := testGroupName
	newHandler := handler.WithGroup(groupName)

	// Should return a new handler
	assert.NotEqual(t, handler, newHandler, "WithGroup should return a new handler instance")

	// New handler should be of the same type
	_, ok := newHandler.(*InteractiveHandler)
	assert.True(t, ok, "WithGroup should return an InteractiveHandler")

	// Test with empty group name
	sameHandler := handler.WithGroup("")
	assert.Equal(t, handler, sameHandler, "WithGroup with empty name should return same handler")
}

func TestInteractiveHandler_Handle_WithAttributes(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: false}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	// Add attributes to handler
	attrs := []slog.Attr{
		slog.String("component", "test"),
	}
	handlerWithAttrs := handler.WithAttrs(attrs)

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("extra", "value"))

	err = handlerWithAttrs.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Formatter should have been called
	assert.True(t, formatter.formatRecordCalled, "Formatter should have been called")
}

func TestInteractiveHandler_Handle_WithGroups(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: false}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	// Add attributes and groups to handler
	handlerWithAttrs := handler.WithAttrs([]slog.Attr{
		slog.String("component", "database"),
		slog.String("operation", "query"),
	})
	handlerWithGroup := handlerWithAttrs.WithGroup("auth").WithGroup("session")

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("user_id", "12345"))

	err = handlerWithGroup.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Formatter should have been called
	assert.True(t, formatter.formatRecordCalled, "Formatter should have been called")

	// Verify that group prefixes were applied correctly (standard slog behavior)
	testCases := []struct {
		key      string
		expected string
		desc     string
	}{
		{"component", "database", "WithAttrs attributes should not be prefixed (added before WithGroup)"},
		{"operation", "query", "WithAttrs attributes should not be prefixed (added before WithGroup)"},
		{"auth.session.user_id", "12345", "record-level attributes should be prefixed with group hierarchy"},
	}

	for _, tc := range testCases {
		value, found := formatter.GetAttribute(tc.key)
		assert.True(t, found, "Expected to find attribute %q, but it was not found. %s", tc.key, tc.desc)
		if !found {
			continue
		}
		assert.Equal(t, tc.expected, value.String(), "For attribute %q. %s", tc.key, tc.desc)
	}

	// Note: In standard slog behavior, WithAttrs attributes exist without prefix
	// since they were added before WithGroup was called
}

func TestInteractiveHandler_Handle_WithAttrsAfterGroup(t *testing.T) {
	var buf bytes.Buffer
	caps := &interactiveTestCapabilities{interactive: true, supportsColor: false}
	formatter := &interactiveTestMessageFormatter{}
	tracker := &interactiveTestLogLineTracker{}

	handler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Level:        slog.LevelInfo,
		Writer:       &buf,
		Capabilities: caps,
		Formatter:    formatter,
		LineTracker:  tracker,
	})
	require.NoError(t, err, "NewInteractiveHandler failed")

	// Create a handler with groups first, then add attributes
	handlerWithGroup := handler.WithGroup("auth").WithGroup("session")
	handlerWithAttrs := handlerWithGroup.WithAttrs([]slog.Attr{
		slog.String("component", "database"),
		slog.String("operation", "query"),
	})

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("user_id", "12345"))

	err = handlerWithAttrs.Handle(ctx, record)
	assert.NoError(t, err, "Handle should not return error")

	// Formatter should have been called
	assert.True(t, formatter.formatRecordCalled, "Formatter should have been called")

	// Verify that group prefixes were applied correctly
	// When WithAttrs is called after WithGroup, the attributes should be prefixed
	testCases := []struct {
		key      string
		expected string
		desc     string
	}{
		{"auth.session.component", "database", "WithAttrs attributes should be prefixed when added after WithGroup"},
		{"auth.session.operation", "query", "WithAttrs attributes should be prefixed when added after WithGroup"},
		{"auth.session.user_id", "12345", "record-level attributes should be prefixed with group hierarchy"},
	}

	for _, tc := range testCases {
		value, found := formatter.GetAttribute(tc.key)
		assert.True(t, found, "Expected to find attribute %q, but it was not found. %s", tc.key, tc.desc)
		if !found {
			continue
		}
		assert.Equal(t, tc.expected, value.String(), "For attribute %q. %s", tc.key, tc.desc)
	}

	// Note: This test verifies the critical case where WithAttrs is called after WithGroup
	// The attributes should be prefixed with the group hierarchy
}
