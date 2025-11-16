//go:build test

// Package logging provides benchmark tests for slack_handler.go command result extraction functions.
// These benchmarks measure the performance of extractCommandResults across various input formats.
package logging

import (
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// createBenchmarkCommandResults creates a standard set of CommandResult test data.
// Returns 5 command results with various exit codes and output patterns.
func createBenchmarkCommandResults() []common.CommandResult {
	return []common.CommandResult{
		{CommandResultFields: common.CommandResultFields{Name: "cmd1", ExitCode: 0, Output: "output1", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd2", ExitCode: 1, Output: "output2", Stderr: "error2"}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd3", ExitCode: 0, Output: "output3", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd4", ExitCode: 0, Output: "output4", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd5", ExitCode: 1, Output: "", Stderr: "error5"}},
	}
}

// createBenchmarkAttrs creates slog.Attr representation for a single command result.
func createBenchmarkAttrs(name string, exitCode int, output, stderr string) []slog.Attr {
	return []slog.Attr{
		slog.String(common.LogFieldName, name),
		slog.Int(common.LogFieldExitCode, exitCode),
		slog.String(common.LogFieldOutput, output),
		slog.String(common.LogFieldStderr, stderr),
	}
}

// BenchmarkExtractCommandResults_Current measures the current implementation
func BenchmarkExtractCommandResults_Current(b *testing.B) {
	// Create test data with []common.CommandResult (direct format)
	commands := createBenchmarkCommandResults()
	value := slog.AnyValue(commands)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractCommandResults_AfterRedaction measures performance with []any (after RedactingHandler)
func BenchmarkExtractCommandResults_AfterRedaction(b *testing.B) {
	// Create test data with []any (simulating RedactingHandler output)
	cmdResults := createBenchmarkCommandResults()
	commands := make([]any, len(cmdResults))
	for i, cmd := range cmdResults {
		commands[i] = cmd
	}
	value := slog.AnyValue(commands)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractCommandResults_WithSlogValue measures performance with slog.Value elements
func BenchmarkExtractCommandResults_WithSlogValue(b *testing.B) {
	// Create test data with []any containing slog.Value elements
	cmdResults := createBenchmarkCommandResults()
	commands := make([]any, len(cmdResults))
	for i, cmd := range cmdResults {
		commands[i] = cmd.LogValue()
	}
	value := slog.AnyValue(commands)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractCommandResults_WithAttrSlice measures performance with []slog.Attr elements
func BenchmarkExtractCommandResults_WithAttrSlice(b *testing.B) {
	// Create test data with []any containing []slog.Attr elements
	cmdResults := createBenchmarkCommandResults()
	commands := make([]any, len(cmdResults))
	for i, cmd := range cmdResults {
		commands[i] = createBenchmarkAttrs(
			cmd.Name,
			cmd.ExitCode,
			cmd.Output,
			cmd.Stderr,
		)
	}
	value := slog.AnyValue(commands)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractFromAttrs measures the performance of extractFromAttrs
func BenchmarkExtractFromAttrs(b *testing.B) {
	attrs := []slog.Attr{
		slog.String(common.LogFieldName, "test_command"),
		slog.Int(common.LogFieldExitCode, 0),
		slog.String(common.LogFieldOutput, "test output"),
		slog.String(common.LogFieldStderr, ""),
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractFromAttrs(attrs)
	}
}
