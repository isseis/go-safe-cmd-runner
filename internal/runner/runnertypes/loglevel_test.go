package runnertypes

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test valid log levels
func TestLogLevel_UnmarshalText_ValidLevels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected LogLevel
	}{
		{"debug", "debug", LogLevelDebug},
		{"info", "info", LogLevelInfo},
		{"warn", "warn", LogLevelWarn},
		{"error", "error", LogLevelError},
		{"empty defaults to info", "", LogLevelInfo},
		{"uppercase DEBUG", "DEBUG", LogLevelDebug},
		{"uppercase INFO", "INFO", LogLevelInfo},
		{"uppercase WARN", "WARN", LogLevelWarn},
		{"uppercase ERROR", "ERROR", LogLevelError},
		{"mixed case Debug", "Debug", LogLevelDebug},
		{"mixed case Info", "Info", LogLevelInfo},
		{"mixed case Warn", "Warn", LogLevelWarn},
		{"mixed case Error", "Error", LogLevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var level LogLevel
			err := level.UnmarshalText([]byte(tt.input))
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, level)
		})
	}
}

// Test invalid log levels
func TestLogLevel_UnmarshalText_InvalidLevels(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"typo", "debg"},
		{"unknown", "unknown"},
		{"number", "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var level LogLevel
			err := level.UnmarshalText([]byte(tt.input))
			assert.Error(t, err)
		})
	}
}

// Test ToSlogLevel conversion
// Verifies conversion of valid and invalid log levels to slog.Level and error handling
func TestLogLevel_ToSlogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected slog.Level
		wantErr  bool
	}{
		// Valid constant levels
		{"debug constant", LogLevelDebug, slog.LevelDebug, false},
		{"info constant", LogLevelInfo, slog.LevelInfo, false},
		{"warn constant", LogLevelWarn, slog.LevelWarn, false},
		{"error constant", LogLevelError, slog.LevelError, false},

		// Valid string variations (case-insensitive)
		{"uppercase DEBUG", LogLevel("DEBUG"), slog.LevelDebug, false},
		{"uppercase INFO", LogLevel("INFO"), slog.LevelInfo, false},
		{"uppercase WARN", LogLevel("WARN"), slog.LevelWarn, false},
		{"uppercase ERROR", LogLevel("ERROR"), slog.LevelError, false},
		{"mixed case Debug", LogLevel("Debug"), slog.LevelDebug, false},
		{"mixed case Info", LogLevel("Info"), slog.LevelInfo, false},
		{"mixed case Warn", LogLevel("Warn"), slog.LevelWarn, false},
		{"mixed case Error", LogLevel("Error"), slog.LevelError, false},

		// Empty string defaults to info
		{"empty string", LogLevel(""), slog.LevelInfo, false},

		// Invalid levels
		{"typo dbg", LogLevel("dbg"), slog.Level(0), true},
		{"typo debg", LogLevel("debg"), slog.Level(0), true},
		{"unknown value", LogLevel("unknown"), slog.Level(0), true},
		{"numeric string", LogLevel("1"), slog.Level(0), true},
		{"whitespace prefix", LogLevel(" debug"), slog.Level(0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slogLevel, err := tt.level.ToSlogLevel()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, slogLevel)
			}
		})
	}
}

// Test String method
func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected string
	}{
		{"debug", LogLevelDebug, "debug"},
		{"info", LogLevelInfo, "info"},
		{"warn", LogLevelWarn, "warn"},
		{"error", LogLevelError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.level.String()
			assert.Equal(t, tt.expected, got)
		})
	}
}
