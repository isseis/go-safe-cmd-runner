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
func createBenchmarkCommandResults() common.CommandResults {
	return common.CommandResults{
		{CommandResultFields: common.CommandResultFields{Name: "cmd1", ExitCode: 0, Output: "output1", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd2", ExitCode: 1, Output: "output2", Stderr: "error2"}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd3", ExitCode: 0, Output: "output3", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd4", ExitCode: 0, Output: "output4", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd5", ExitCode: 1, Output: "", Stderr: "error5"}},
	}
}

// BenchmarkExtractCommandResults_CommandResults measures extraction when the
// runner emits CommandResults (the only supported path after Task0056).
func BenchmarkExtractCommandResults_CommandResults(b *testing.B) {
	value := createBenchmarkCommandResults().LogValue()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractCommandResults_FromGroupValue measures the inner helper directly.
func BenchmarkExtractCommandResults_FromGroupValue(b *testing.B) {
	groupValue := createBenchmarkCommandResults().LogValue()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResultsFromGroup(groupValue)
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
