package logging

import (
	"log/slog"
	"testing"
	"time"
)

func TestSlackHandler_WithAttrs(t *testing.T) {
	// Create a SlackHandler (we don't need a real webhook URL for this test)
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
		attrs:      nil,
		groups:     nil,
	}

	// Test WithAttrs
	attrs := []slog.Attr{
		slog.String("key1", "value1"),
		slog.String("key2", "value2"),
	}

	newHandler := handler.WithAttrs(attrs).(*SlackHandler)

	// Verify the new handler has the attributes
	if len(newHandler.attrs) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(newHandler.attrs))
	}

	// Verify the original handler is unchanged
	if len(handler.attrs) != 0 {
		t.Errorf("Original handler should not be modified")
	}

	// Test chaining WithAttrs
	moreAttrs := []slog.Attr{
		slog.String("key3", "value3"),
	}

	chainedHandler := newHandler.WithAttrs(moreAttrs).(*SlackHandler)

	if len(chainedHandler.attrs) != 3 {
		t.Errorf("Expected 3 attributes after chaining, got %d", len(chainedHandler.attrs))
	}
}

func TestSlackHandler_WithGroup(t *testing.T) {
	// Create a SlackHandler
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
		attrs:      nil,
		groups:     nil,
	}

	// Test WithGroup
	newHandler := handler.WithGroup("group1").(*SlackHandler)

	// Verify the new handler has the group
	if len(newHandler.groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(newHandler.groups))
	}

	if newHandler.groups[0] != "group1" {
		t.Errorf("Expected group name 'group1', got '%s'", newHandler.groups[0])
	}

	// Verify the original handler is unchanged
	if len(handler.groups) != 0 {
		t.Errorf("Original handler should not be modified")
	}

	// Test chaining WithGroup
	chainedHandler := newHandler.WithGroup("group2").(*SlackHandler)

	if len(chainedHandler.groups) != 2 {
		t.Errorf("Expected 2 groups after chaining, got %d", len(chainedHandler.groups))
	}

	if chainedHandler.groups[1] != "group2" {
		t.Errorf("Expected second group name 'group2', got '%s'", chainedHandler.groups[1])
	}
}

func TestSlackHandler_WithAttrsAndGroups(t *testing.T) {
	// Create a SlackHandler
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
		attrs:      nil,
		groups:     nil,
	}

	// Test combining WithAttrs and WithGroup
	attrs := []slog.Attr{
		slog.String("key1", "value1"),
	}

	combined := handler.WithAttrs(attrs).WithGroup("testgroup").(*SlackHandler)

	if len(combined.attrs) != 1 {
		t.Errorf("Expected 1 attribute, got %d", len(combined.attrs))
	}

	if len(combined.groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(combined.groups))
	}

	if combined.groups[0] != "testgroup" {
		t.Errorf("Expected group name 'testgroup', got '%s'", combined.groups[0])
	}
}

func TestSlackHandler_ApplyAccumulatedContext(t *testing.T) {
	// Create a SlackHandler with some accumulated context
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
		attrs: []slog.Attr{
			slog.String("accumulated_key", "accumulated_value"),
		},
		groups: []string{"testgroup"},
	}

	// Create a test record
	originalRecord := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	originalRecord.AddAttrs(slog.String("original_key", "original_value"))

	// Apply accumulated context
	newRecord := handler.applyAccumulatedContext(originalRecord)

	// Verify the new record has both accumulated and original attributes
	attrCount := 0
	var hasAccumulated, hasOriginal, hasGroup bool

	newRecord.Attrs(func(attr slog.Attr) bool {
		attrCount++
		switch attr.Key {
		case "original_key":
			hasOriginal = true
		case "testgroup":
			hasGroup = true
			// Check if the group contains the accumulated attribute
			if attr.Value.Kind() == slog.KindGroup {
				groupAttrs := attr.Value.Group()
				for _, groupAttr := range groupAttrs {
					if groupAttr.Key == "accumulated_key" {
						hasAccumulated = true
					}
				}
			}
		}
		return true
	})

	if !hasOriginal {
		t.Error("Expected original attribute to be present")
	}

	if !hasGroup {
		t.Error("Expected group to be present")
	}

	if !hasAccumulated {
		t.Error("Expected accumulated attribute to be present in group")
	}
}

func TestSlackHandler_WithAttrsEmptySlice(t *testing.T) {
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
	}

	// WithAttrs with empty slice should return the same handler
	newHandler := handler.WithAttrs([]slog.Attr{})

	if newHandler != handler {
		t.Error("WithAttrs with empty slice should return the same handler")
	}
}

func TestSlackHandler_WithGroupEmptyString(t *testing.T) {
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
	}

	// WithGroup with empty string should return the same handler
	newHandler := handler.WithGroup("")

	if newHandler != handler {
		t.Error("WithGroup with empty string should return the same handler")
	}
}
