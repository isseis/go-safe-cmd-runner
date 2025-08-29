package logging

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestNewDefaultMessageFormatter(t *testing.T) {
	formatter := NewDefaultMessageFormatter()
	if formatter == nil {
		t.Error("NewDefaultMessageFormatter should return a non-nil instance")
	}
}

func TestDefaultMessageFormatter_FormatRecordWithColor(t *testing.T) {
	formatter := NewDefaultMessageFormatter()
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		record    slog.Record
		useColor  bool
		expecteds []string // Multiple possible valid formats
	}{
		{
			name:      "debug level with color",
			record:    slog.NewRecord(now, slog.LevelDebug, "debug message", 0),
			useColor:  true,
			expecteds: []string{"2024-01-01 12:00:00 * DEBUG debug message"},
		},
		{
			name:      "debug level without color",
			record:    slog.NewRecord(now, slog.LevelDebug, "debug message", 0),
			useColor:  false,
			expecteds: []string{"2024-01-01 12:00:00 [DEBUG] debug message"},
		},
		{
			name:      "info level with color",
			record:    slog.NewRecord(now, slog.LevelInfo, "info message", 0),
			useColor:  true,
			expecteds: []string{"2024-01-01 12:00:00 + INFO  info message"},
		},
		{
			name:      "info level without color",
			record:    slog.NewRecord(now, slog.LevelInfo, "info message", 0),
			useColor:  false,
			expecteds: []string{"2024-01-01 12:00:00 [INFO ] info message"},
		},
		{
			name:      "warn level with color",
			record:    slog.NewRecord(now, slog.LevelWarn, "warn message", 0),
			useColor:  true,
			expecteds: []string{"2024-01-01 12:00:00 ! WARN  warn message"},
		},
		{
			name:      "warn level without color",
			record:    slog.NewRecord(now, slog.LevelWarn, "warn message", 0),
			useColor:  false,
			expecteds: []string{"2024-01-01 12:00:00 [WARN ] warn message"},
		},
		{
			name:      "error level with color",
			record:    slog.NewRecord(now, slog.LevelError, "error message", 0),
			useColor:  true,
			expecteds: []string{"2024-01-01 12:00:00 X ERROR error message"},
		},
		{
			name:      "error level without color",
			record:    slog.NewRecord(now, slog.LevelError, "error message", 0),
			useColor:  false,
			expecteds: []string{"2024-01-01 12:00:00 [ERROR] error message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatRecordWithColor(tt.record, tt.useColor)

			// Check if result matches any of the expected formats
			found := false
			for _, expected := range tt.expecteds {
				if result == expected {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("FormatRecordWithColor() = %q, expected one of %v", result, tt.expecteds)
			}
		})
	}
}

func TestDefaultMessageFormatter_FormatRecordWithAttributes(t *testing.T) {
	formatter := NewDefaultMessageFormatter()
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)
	record.AddAttrs(
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	)

	result := formatter.FormatRecordWithColor(record, false)

	// Check that the result contains the timestamp, level, message, and attributes
	if !strings.Contains(result, "2024-01-01 12:00:00") {
		t.Error("Result should contain timestamp")
	}
	if !strings.Contains(result, "[INFO ]") {
		t.Error("Result should contain level")
	}
	if !strings.Contains(result, "test message") {
		t.Error("Result should contain message")
	}
	if !strings.Contains(result, "key1=value1") {
		t.Error("Result should contain first attribute")
	}
	if !strings.Contains(result, "key2=42") {
		t.Error("Result should contain second attribute")
	}
}

func TestDefaultMessageFormatter_FormatLogFileHint(t *testing.T) {
	formatter := NewDefaultMessageFormatter()

	tests := []struct {
		name       string
		lineNumber int
		useColor   bool
		expected   string
	}{
		{
			name:       "valid line with color",
			lineNumber: 42,
			useColor:   true,
			expected:   "* Check log file around line 42 for more details",
		},
		{
			name:       "valid line without color",
			lineNumber: 100,
			useColor:   false,
			expected:   "HINT: Check log file around line 100 for more details",
		},
		{
			name:       "zero line number",
			lineNumber: 0,
			useColor:   false,
			expected:   "",
		},
		{
			name:       "negative line number",
			lineNumber: -5,
			useColor:   true,
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatLogFileHint(tt.lineNumber, tt.useColor)
			if result != tt.expected {
				t.Errorf("FormatLogFileHint() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestDefaultMessageFormatter_FormatLevel(t *testing.T) {
	formatter := NewDefaultMessageFormatter()

	tests := []struct {
		name     string
		level    slog.Level
		useColor bool
		expected string
	}{
		{"debug with color", slog.LevelDebug, true, "* DEBUG"},
		{"debug without color", slog.LevelDebug, false, "[DEBUG]"},
		{"info with color", slog.LevelInfo, true, "+ INFO "},
		{"info without color", slog.LevelInfo, false, "[INFO ]"},
		{"warn with color", slog.LevelWarn, true, "! WARN "},
		{"warn without color", slog.LevelWarn, false, "[WARN ]"},
		{"error with color", slog.LevelError, true, "X ERROR"},
		{"error without color", slog.LevelError, false, "[ERROR]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.formatLevel(tt.level, tt.useColor)
			if result != tt.expected {
				t.Errorf("formatLevel() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestDefaultMessageFormatter_FormatValue(t *testing.T) {
	formatter := NewDefaultMessageFormatter()
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		value    slog.Value
		expected string
	}{
		{
			name:     "string value",
			value:    slog.StringValue("test string"),
			expected: "test string",
		},
		{
			name:     "int value",
			value:    slog.IntValue(42),
			expected: "42",
		},
		{
			name:     "time value",
			value:    slog.TimeValue(testTime),
			expected: "2024-01-01T12:00:00Z",
		},
		{
			name:     "duration value",
			value:    slog.DurationValue(5 * time.Second),
			expected: "5s",
		},
		{
			name:     "empty group",
			value:    slog.GroupValue(),
			expected: "{}",
		},
		{
			name: "group with attributes",
			value: slog.GroupValue(
				slog.String("key1", "value1"),
				slog.Int("key2", 42),
			),
			expected: "{key1=value1,key2=42}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.formatValue(tt.value)
			if result != tt.expected {
				t.Errorf("formatValue() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestMessageFormatter_Interface(t *testing.T) {
	// Test that DefaultMessageFormatter implements MessageFormatter interface
	var formatter MessageFormatter = NewDefaultMessageFormatter()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	record := slog.NewRecord(now, slog.LevelInfo, "test message", 0)

	// Test interface methods
	result := formatter.FormatRecordWithColor(record, false)
	if result == "" {
		t.Error("FormatRecordWithColor should return non-empty string")
	}

	hint := formatter.FormatLogFileHint(10, false)
	expected := "HINT: Check log file around line 10 for more details"
	if hint != expected {
		t.Errorf("FormatLogFileHint() = %q, expected %q", hint, expected)
	}
}

func TestDefaultMessageFormatter_CustomLevel(t *testing.T) {
	formatter := NewDefaultMessageFormatter()
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Test custom level higher than ERROR
	customLevel := slog.Level(12) // Higher than ERROR (8)
	record := slog.NewRecord(now, customLevel, "custom message", 0)

	resultWithColor := formatter.FormatRecordWithColor(record, true)
	resultWithoutColor := formatter.FormatRecordWithColor(record, false)

	// Should handle custom levels gracefully
	if !strings.Contains(resultWithColor, "custom message") {
		t.Error("Should contain the message for custom levels")
	}
	if !strings.Contains(resultWithoutColor, "custom message") {
		t.Error("Should contain the message for custom levels")
	}
}
