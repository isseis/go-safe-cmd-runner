package logging

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewDefaultMessageFormatter(t *testing.T) {
	formatter := NewDefaultMessageFormatter()
	assert.NotNil(t, formatter, "NewDefaultMessageFormatter should return a non-nil instance")
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
			expecteds: []string{"2024-01-01 12:00:00 \033[90m* DEBUG\033[0m debug message"},
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
			expecteds: []string{"2024-01-01 12:00:00 \033[32m+ INFO \033[0m info message"},
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
			expecteds: []string{"2024-01-01 12:00:00 \033[33m! WARN \033[0m warn message"},
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
			expecteds: []string{"2024-01-01 12:00:00 \033[31mX ERROR\033[0m error message"},
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

			assert.True(t, found, "FormatRecordWithColor() = %q, expected one of %v", result, tt.expecteds)
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
	assert.Contains(t, result, "2024-01-01 12:00:00", "Result should contain timestamp")
	assert.Contains(t, result, "[INFO ]", "Result should contain level")
	assert.Contains(t, result, "test message", "Result should contain message")
	assert.Contains(t, result, "key1=value1", "Result should contain first attribute")
	assert.Contains(t, result, "key2=42", "Result should contain second attribute")
}

func TestDefaultMessageFormatter_FormatRecordInteractive(t *testing.T) {
	formatter := NewDefaultMessageFormatter()
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		record    slog.Record
		useColor  bool
		expecteds []string
	}{
		{
			name:      "info level with color - no timestamp",
			record:    slog.NewRecord(now, slog.LevelInfo, "test message", 0),
			useColor:  true,
			expecteds: []string{"\033[32m+ INFO \033[0m test message"},
		},
		{
			name: "warn level with priority attribute",
			record: func() slog.Record {
				r := slog.NewRecord(now, slog.LevelWarn, "access denied", 0)
				r.AddAttrs(slog.String("variable", "SHELL"), slog.String("run_id", "123"), slog.String("hostname", "test"))
				return r
			}(),
			useColor:  true,
			expecteds: []string{"\033[33m! WARN \033[0m access denied variable=SHELL"},
		},
		{
			name: "error level with multiple priority attributes",
			record: func() slog.Record {
				r := slog.NewRecord(now, slog.LevelError, "operation failed", 0)
				r.AddAttrs(slog.String("component", "auth"), slog.String("error", "timeout"), slog.String("duration_ms", "5000"))
				return r
			}(),
			useColor:  true,
			expecteds: []string{"\033[31mX ERROR\033[0m operation failed error=timeout component=auth"},
		},
		{
			name:      "debug level without color",
			record:    slog.NewRecord(now, slog.LevelDebug, "debug info", 0),
			useColor:  false,
			expecteds: []string{"[DEBUG] debug info"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatRecordInteractive(tt.record, tt.useColor)
			found := false
			for _, expected := range tt.expecteds {
				if result == expected {
					found = true
					break
				}
			}
			assert.True(t, found, "FormatRecordInteractive() = %q, expected one of %v", result, tt.expecteds)
		})
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
			expected:   "\033[36m* \033[0mCheck log file around line 42 for more details",
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
			assert.Equal(t, tt.expected, result, "FormatLogFileHint() should match expected value")
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
		{"debug with color", slog.LevelDebug, true, "\033[90m* DEBUG\033[0m"},
		{"debug without color", slog.LevelDebug, false, "[DEBUG]"},
		{"info with color", slog.LevelInfo, true, "\033[32m+ INFO \033[0m"},
		{"info without color", slog.LevelInfo, false, "[INFO ]"},
		{"warn with color", slog.LevelWarn, true, "\033[33m! WARN \033[0m"},
		{"warn without color", slog.LevelWarn, false, "[WARN ]"},
		{"error with color", slog.LevelError, true, "\033[31mX ERROR\033[0m"},
		{"error without color", slog.LevelError, false, "[ERROR]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.formatLevel(tt.level, tt.useColor)
			assert.Equal(t, tt.expected, result, "formatLevel() should match expected value")
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
			assert.Equal(t, tt.expected, result, "formatValue() should match expected value")
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
	assert.NotEmpty(t, result, "FormatRecordWithColor should return non-empty string")

	hint := formatter.FormatLogFileHint(10, false)
	expected := "HINT: Check log file around line 10 for more details"
	assert.Equal(t, expected, hint, "FormatLogFileHint() should match expected value")
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
	assert.Contains(t, resultWithColor, "custom message", "Should contain the message for custom levels")
	assert.Contains(t, resultWithoutColor, "custom message", "Should contain the message for custom levels")
}

func TestShouldSkipInteractiveAttr_True(t *testing.T) {
	formatter := NewDefaultMessageFormatter()

	// Attributes that should be skipped in interactive mode
	skipAttrs := []string{
		"time",
		"level",
		"msg",
		"run_id",
		"hostname",
		"pid",
		"schema_version",
		"duration_ms",
		"verified_files",
		"skipped_files",
		"total_files",
		"interactive_mode",
		"color_support",
		"slack_enabled",
	}

	for _, attr := range skipAttrs {
		t.Run(attr, func(t *testing.T) {
			assert.True(t, formatter.shouldSkipInteractiveAttr(attr), "shouldSkipInteractiveAttr(%q) should return true", attr)
		})
	}
}

func TestShouldSkipInteractiveAttr_False(t *testing.T) {
	formatter := NewDefaultMessageFormatter()

	// Attributes that should NOT be skipped in interactive mode
	keepAttrs := []string{
		"error",
		"group",
		"command",
		"file",
		"component",
		"variable",
		"user_defined_key",
		"custom_attr",
	}

	for _, attr := range keepAttrs {
		t.Run(attr, func(t *testing.T) {
			assert.False(t, formatter.shouldSkipInteractiveAttr(attr), "shouldSkipInteractiveAttr(%q) should return false", attr)
		})
	}
}

func TestAppendInteractiveAttrs_EdgeCases(t *testing.T) {
	formatter := NewDefaultMessageFormatter()

	tests := []struct {
		name         string
		record       func() slog.Record
		checkContent func(t *testing.T, result string)
	}{
		{
			name: "empty attributes",
			record: func() slog.Record {
				return slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
			},
			checkContent: func(t *testing.T, result string) {
				if !strings.Contains(result, "test") {
					t.Error("Should contain message text")
				}
			},
		},
		{
			name: "many attributes",
			record: func() slog.Record {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
				for i := 0; i < 20; i++ {
					r.AddAttrs(slog.String("key"+string(rune('A'+i)), "value"))
				}
				return r
			},
			checkContent: func(t *testing.T, result string) {
				if !strings.Contains(result, "keyA=value") {
					t.Error("Should contain at least the first attribute")
				}
			},
		},
		{
			name: "special characters in attribute values",
			record: func() slog.Record {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
				r.AddAttrs(
					slog.String("path", "/tmp/test file.txt"),
					slog.String("command", "echo \"hello world\""),
					slog.String("error", "failed: permission denied"),
				)
				return r
			},
			checkContent: func(t *testing.T, result string) {
				// Should contain command and error attributes (path might be skipped)
				if !strings.Contains(result, "error=") && !strings.Contains(result, "command=") {
					t.Errorf("Should contain some attributes, got: %s", result)
				}
			},
		},
		{
			name: "mixed attribute types",
			record: func() slog.Record {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
				r.AddAttrs(
					slog.String("string_key", "string_value"),
					slog.Int("int_key", 42),
					slog.Bool("bool_key", true),
					slog.Float64("float_key", 3.14),
					slog.Duration("duration_key", 5*time.Second),
				)
				return r
			},
			checkContent: func(t *testing.T, result string) {
				if !strings.Contains(result, "string_key=string_value") {
					t.Error("Should contain string attribute")
				}
			},
		},
		{
			name: "nested groups",
			record: func() slog.Record {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
				r.AddAttrs(
					slog.Group("outer",
						slog.String("inner_key", "inner_value"),
						slog.Int("count", 10),
					),
				)
				return r
			},
			checkContent: func(t *testing.T, result string) {
				// Should format the group somehow
				if result == "" {
					t.Error("Result should not be empty")
				}
			},
		},
		{
			name: "skipped attributes mixed with kept attributes",
			record: func() slog.Record {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
				r.AddAttrs(
					slog.String("time", "2024-01-01"),
					slog.String("error", "test error"),
					slog.String("run_id", "test-run"),
					slog.String("component", "test-component"),
				)
				return r
			},
			checkContent: func(t *testing.T, result string) {
				if strings.Contains(result, "run_id=") {
					t.Error("Should skip run_id attribute")
				}
				if !strings.Contains(result, "error=test error") {
					t.Error("Should keep error attribute")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatRecordInteractive(tt.record(), false)
			tt.checkContent(t, result)
		})
	}
}

func TestAppendInteractiveAttrs_EmptyValues(t *testing.T) {
	formatter := NewDefaultMessageFormatter()

	tests := []struct {
		name   string
		record func() slog.Record
	}{
		{
			name: "empty string value",
			record: func() slog.Record {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
				r.AddAttrs(slog.String("key", ""))
				return r
			},
		},
		{
			name: "zero int value",
			record: func() slog.Record {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
				r.AddAttrs(slog.Int("count", 0))
				return r
			},
		},
		{
			name: "false bool value",
			record: func() slog.Record {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
				r.AddAttrs(slog.Bool("flag", false))
				return r
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic with empty or zero values
			result := formatter.FormatRecordInteractive(tt.record(), false)
			if result == "" {
				t.Error("FormatRecordInteractive should return non-empty string")
			}
		})
	}
}
