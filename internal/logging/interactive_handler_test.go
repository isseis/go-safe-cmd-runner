package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
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
}

func (m *interactiveTestMessageFormatter) FormatRecordWithColor(record slog.Record, useColor bool) string {
	m.formatRecordCalled = true
	m.recordMessage = record.Message
	if useColor {
		return "@ " + record.Message
	}
	return "[FORMATTED] " + record.Message
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
	if err != nil {
		t.Errorf("NewInteractiveHandler should not return error: %v", err)
	}
	if handler == nil {
		t.Error("NewInteractiveHandler should return a non-nil handler")
	}
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
			if err == nil {
				t.Errorf("Expected error for %s", tc.name)
			}
			if handler != nil {
				t.Errorf("Expected nil handler for %s, got %v", tc.name, handler)
			}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
	}

	ctx := context.Background()

	// Should be enabled for levels >= configured level in interactive mode
	if handler.Enabled(ctx, slog.LevelDebug) {
		t.Error("Should not be enabled for debug level when min level is warn")
	}
	if handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Should not be enabled for info level when min level is warn")
	}
	if !handler.Enabled(ctx, slog.LevelWarn) {
		t.Error("Should be enabled for warn level")
	}
	if !handler.Enabled(ctx, slog.LevelError) {
		t.Error("Should be enabled for error level")
	}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
	}

	ctx := context.Background()

	// Should be disabled for all levels in non-interactive mode
	if handler.Enabled(ctx, slog.LevelDebug) {
		t.Error("Should be disabled in non-interactive mode")
	}
	if handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Should be disabled in non-interactive mode")
	}
	if handler.Enabled(ctx, slog.LevelWarn) {
		t.Error("Should be disabled in non-interactive mode")
	}
	if handler.Enabled(ctx, slog.LevelError) {
		t.Error("Should be disabled in non-interactive mode")
	}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	err = handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Should call formatter with color support
	if !formatter.formatRecordCalled {
		t.Error("Formatter should have been called")
	}
	if formatter.recordMessage != "test message" {
		t.Errorf("Formatter received wrong message: %s", formatter.recordMessage)
	}

	// Should write formatted output
	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Error("Output should contain formatted message")
	}

	// Should not add hint for non-error levels
	if formatter.formatHintCalled {
		t.Error("Hint formatter should not be called for non-error levels")
	}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	err = handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Should not call formatter in non-interactive mode
	if formatter.formatRecordCalled {
		t.Error("Formatter should not be called in non-interactive mode")
	}

	// Should not write any output
	if buf.Len() > 0 {
		t.Error("No output should be written in non-interactive mode")
	}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelError, "error message", 0)

	err = handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Should call both formatters
	if !formatter.formatRecordCalled {
		t.Error("Record formatter should have been called")
	}
	if !formatter.formatHintCalled {
		t.Error("Hint formatter should have been called for error level")
	}

	// Should call line tracker
	if !tracker.getCalled {
		t.Error("Line tracker should have been called")
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 1 {
		t.Error("Should have at least one line of output")
	}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
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
	if _, ok := newHandler.(*InteractiveHandler); !ok {
		t.Error("WithAttrs should return an InteractiveHandler")
	}

	// Test with empty attrs
	sameHandler := handler.WithAttrs(nil)
	if sameHandler != handler {
		t.Error("WithAttrs with empty attrs should return same handler")
	}

	sameHandler = handler.WithAttrs([]slog.Attr{})
	if sameHandler != handler {
		t.Error("WithAttrs with empty slice should return same handler")
	}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
	}

	const testGroupName = "testgroup"
	groupName := testGroupName
	newHandler := handler.WithGroup(groupName)

	// Should return a new handler
	if newHandler == handler {
		t.Error("WithGroup should return a new handler instance")
	}

	// New handler should be of the same type
	if _, ok := newHandler.(*InteractiveHandler); !ok {
		t.Error("WithGroup should return an InteractiveHandler")
	}

	// Test with empty group name
	sameHandler := handler.WithGroup("")
	if sameHandler != handler {
		t.Error("WithGroup with empty name should return same handler")
	}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
	}

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
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Formatter should have been called
	if !formatter.formatRecordCalled {
		t.Error("Formatter should have been called")
	}
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
	if err != nil {
		t.Fatalf("NewInteractiveHandler failed: %v", err)
	}

	// Add group to handler
	const testGroupName2 = "testgroup"
	handlerWithGroup := handler.WithGroup(testGroupName2)

	ctx := context.Background()
	now := time.Now()
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	err = handlerWithGroup.Handle(ctx, record)
	if err != nil {
		t.Errorf("Handle should not return error: %v", err)
	}

	// Formatter should have been called
	if !formatter.formatRecordCalled {
		t.Error("Formatter should have been called")
	}
}
