package runnertypes

import (
	"log/slog"
	"testing"
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
			if err != nil {
				t.Errorf("UnmarshalText() error = %v, want nil", err)
			}
			if level != tt.expected {
				t.Errorf("UnmarshalText() = %v, want %v", level, tt.expected)
			}
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
			if err == nil {
				t.Errorf("UnmarshalText() error = nil, want error for input %q", tt.input)
			}
		})
	}
}

// Test ToSlogLevel conversion with valid levels
func TestLogLevel_ToSlogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected slog.Level
		wantErr  bool
	}{
		{"debug", LogLevelDebug, slog.LevelDebug, false},
		{"info", LogLevelInfo, slog.LevelInfo, false},
		{"warn", LogLevelWarn, slog.LevelWarn, false},
		{"error", LogLevelError, slog.LevelError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slogLevel, err := tt.level.ToSlogLevel()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToSlogLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if slogLevel != tt.expected {
				t.Errorf("ToSlogLevel() = %v, want %v", slogLevel, tt.expected)
			}
		})
	}
}

// Test ToSlogLevel conversion with invalid levels
// LogLevel is a string alias, so invalid values can be set directly without going through UnmarshalText
func TestLogLevel_ToSlogLevel_InvalidLevels(t *testing.T) {
	tests := []struct {
		name    string
		level   LogLevel
		wantErr bool
	}{
		{"typo dbg", LogLevel("dbg"), true},
		{"typo debg", LogLevel("debg"), true},
		{"unknown value", LogLevel("unknown"), true},
		{"numeric string", LogLevel("1"), true},
		{"uppercase DEBUG", LogLevel("DEBUG"), false},
		{"mixed case Debug", LogLevel("Debug"), false},
		{"whitespace", LogLevel(" debug"), true},
		{"empty string", LogLevel(""), false}, // empty string should work (defaults to info)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slogLevel, err := tt.level.ToSlogLevel()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToSlogLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// For invalid levels, we just check that an error was returned
			if tt.wantErr && err == nil {
				t.Errorf("ToSlogLevel() expected error for invalid level %q, got nil", tt.level)
			}
			// For empty string, it should default to info level
			if tt.level == LogLevel("") && !tt.wantErr && slogLevel != slog.LevelInfo {
				t.Errorf("ToSlogLevel() empty string should default to info, got %v", slogLevel)
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
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
