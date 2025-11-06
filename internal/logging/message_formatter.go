package logging

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/color"
)

// MessageFormatter handles formatting log messages with optional color support.
type MessageFormatter interface {
	// FormatRecordWithColor formats a log record with optional color support
	FormatRecordWithColor(record slog.Record, useColor bool) string

	// FormatRecordInteractive formats a log record for interactive display
	// with simplified formatting optimized for human readability
	FormatRecordInteractive(record slog.Record, useColor bool) string

	// FormatLogFileHint formats a log file hint message for error-level logs
	FormatLogFileHint(lineNumber int, useColor bool) string
}

// DefaultMessageFormatter provides a simple implementation of MessageFormatter
// without ANSI escape sequences, using symbols and prefixes for visual distinction.
type DefaultMessageFormatter struct{}

// NewDefaultMessageFormatter creates a new DefaultMessageFormatter.
func NewDefaultMessageFormatter() *DefaultMessageFormatter {
	return &DefaultMessageFormatter{}
}

// FormatRecordWithColor formats a log record with optional color support.
// This implementation uses simple symbols and formatting for visual distinction
// without ANSI escape sequences.
func (f *DefaultMessageFormatter) FormatRecordWithColor(record slog.Record, useColor bool) string {
	var sb strings.Builder

	// Add timestamp
	timestamp := record.Time.Format("2006-01-02 15:04:05")
	sb.WriteString(timestamp)
	sb.WriteString(" ")

	// Add level with symbol/prefix for visual distinction
	levelStr := f.formatLevel(record.Level, useColor)
	sb.WriteString(levelStr)
	sb.WriteString(" ")

	// Add message
	sb.WriteString(record.Message)

	// Add attributes
	if record.NumAttrs() > 0 {
		sb.WriteString(" ")
		f.appendAttrs(&sb, record)
	}

	return sb.String()
}

// FormatRecordInteractive formats a log record for interactive display with simplified formatting.
// This version prioritizes readability over completeness, showing only essential information.
func (f *DefaultMessageFormatter) FormatRecordInteractive(record slog.Record, useColor bool) string {
	var sb strings.Builder

	// Add level with symbol/prefix for visual distinction
	levelStr := f.formatLevel(record.Level, useColor)
	sb.WriteString(levelStr)
	sb.WriteString(" ")

	// Add message
	sb.WriteString(record.Message)

	// For interactive display, show only key attributes based on message importance
	if record.NumAttrs() > 0 {
		f.appendInteractiveAttrs(&sb, record)
	}

	return sb.String()
}

// getPriorityKeys returns priority attributes based on log level
// - stderr: shown only at WARN level and above
// - stdout: shown only at DEBUG level
func (f *DefaultMessageFormatter) getPriorityKeys(level slog.Level) []string {
	// Base priority keys (always included)
	baseKeys := []string{"error", "group", "command", "file", "component", "variable"}

	// Add stderr for WARN and above (WARN=-4, ERROR=0, higher values are more severe)
	if level >= slog.LevelWarn {
		// Insert stderr after error
		return append([]string{"error", "stderr"}, baseKeys[1:]...)
	}

	// Add stdout for DEBUG level
	if level == slog.LevelDebug {
		// Add stdout at the end
		return append(baseKeys, "stdout")
	}

	// For INFO and other levels, return base keys without stderr or stdout
	return baseKeys
}

// appendInteractiveAttrs appends selected attributes for interactive display
func (f *DefaultMessageFormatter) appendInteractiveAttrs(sb *strings.Builder, record slog.Record) {
	// Priority attributes to show in interactive mode (in order of preference)
	// Note: stderr and stdout are conditionally included based on log level
	priorityKeys := f.getPriorityKeys(record.Level)

	var foundAttrs []slog.Attr

	// First pass: collect priority attributes in priority order
	for _, priorityKey := range priorityKeys {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == priorityKey || strings.HasSuffix(attr.Key, "."+priorityKey) {
				foundAttrs = append(foundAttrs, attr)
				return false // Stop after finding the first match for this priority key
			}
			return true
		})
	}

	// If no priority attributes found, collect up to maxInteractiveAttrs most relevant ones
	const maxInteractiveAttrs = 3
	if len(foundAttrs) == 0 {
		count := 0
		record.Attrs(func(attr slog.Attr) bool {
			if count >= maxInteractiveAttrs {
				return false
			}
			// Skip internal/noisy attributes
			if !f.shouldSkipInteractiveAttr(attr.Key) {
				foundAttrs = append(foundAttrs, attr)
				count++
			}
			return true
		})
	}

	// Format found attributes
	if len(foundAttrs) > 0 {
		sb.WriteString(" ")
		for i, attr := range foundAttrs {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(attr.Key)
			sb.WriteString("=")
			sb.WriteString(f.formatValue(attr.Value))
		}
	}
}

// shouldSkipInteractiveAttr determines if an attribute should be skipped in interactive mode
func (f *DefaultMessageFormatter) shouldSkipInteractiveAttr(key string) bool {
	skipKeys := []string{
		"time", "level", "msg", "run_id", "hostname", "pid", "schema_version",
		"duration_ms", "verified_files", "skipped_files", "total_files",
		"interactive_mode", "color_support", "slack_enabled",
	}

	for _, skipKey := range skipKeys {
		if key == skipKey {
			return true
		}
	}
	return false
}

// FormatLogFileHint formats a log file hint message for error-level logs.
func (f *DefaultMessageFormatter) FormatLogFileHint(lineNumber int, useColor bool) string {
	if lineNumber <= 0 {
		return ""
	}

	var sb strings.Builder

	if useColor {
		sb.WriteString(color.Cyan("* "))
	} else {
		sb.WriteString("HINT: ")
	}

	sb.WriteString("Check log file around line ")
	sb.WriteString(strconv.Itoa(lineNumber))
	sb.WriteString(" for more details")

	return sb.String()
}

// formatLevel formats the log level with visual distinction
func (f *DefaultMessageFormatter) formatLevel(level slog.Level, useColor bool) string {
	if useColor {
		switch level {
		case slog.LevelDebug:
			return color.Gray("* DEBUG")
		case slog.LevelInfo:
			return color.Green("+ INFO ")
		case slog.LevelWarn:
			return color.Yellow("! WARN ")
		case slog.LevelError:
			return color.Red("X ERROR")
		default:
			return color.Gray("> " + level.String())
		}
	} else {
		switch level {
		case slog.LevelDebug:
			return "[DEBUG]"
		case slog.LevelInfo:
			return "[INFO ]"
		case slog.LevelWarn:
			return "[WARN ]"
		case slog.LevelError:
			return "[ERROR]"
		default:
			return "[" + strings.ToUpper(level.String()) + "]"
		}
	}
}

// appendAttrs appends log record attributes to the string builder
func (f *DefaultMessageFormatter) appendAttrs(sb *strings.Builder, record slog.Record) {
	attrs := make([]slog.Attr, 0, record.NumAttrs())
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})

	for i, attr := range attrs {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(attr.Key)
		sb.WriteString("=")
		sb.WriteString(f.formatValue(attr.Value))
	}
}

// formatValue formats a slog.Value for display
func (f *DefaultMessageFormatter) formatValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindGroup:
		// For group values, format as comma-separated key=value pairs
		attrs := value.Group()
		if len(attrs) == 0 {
			return "{}"
		}
		var parts []string
		for _, attr := range attrs {
			parts = append(parts, attr.Key+"="+f.formatValue(attr.Value))
		}
		return "{" + strings.Join(parts, ",") + "}"
	default:
		return value.String()
	}
}
