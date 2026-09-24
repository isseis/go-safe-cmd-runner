//go:build test

// Package logging provides benchmark tests for slack_handler.go command result extraction functions.
// These benchmarks measure the performance of extractCommandResults across various input formats.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
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
	for range b.N {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractCommandResults_FromGroupValue measures the inner helper directly.
func BenchmarkExtractCommandResults_FromGroupValue(b *testing.B) {
	groupValue := createBenchmarkCommandResults().LogValue()

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
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
	for range b.N {
		_ = extractFromAttrs(attrs)
	}
}

// BenchmarkBuildPreExecutionError_FailedFilePaths measures rendering a
// failed-file list end to end, as production does it: the RedactingHandler
// pass (which redacts every element and turns the []string into []any)
// followed by the builder's budgeted selection. The budget is absolute: a
// 10,000-path list should render in under 100 ms, small next to the hashing
// and I/O of the verification that produced it, and the per-path cost at
// 10,000 should stay within 3x of that at 1,000 so quadratic growth shows up.
// Most of the end-to-end cost is the existing per-element redaction, which
// every per-file verification log line already pays; the builder alone is a
// small fraction. Timing depends on the machine, so no test asserts it.
func BenchmarkBuildPreExecutionError_FailedFilePaths(b *testing.B) {
	manyPaths := func(n int) []string {
		paths := make([]string, n)
		for i := range paths {
			paths[i] = fmt.Sprintf("/var/lib/backup/%05d/snapshot-with-a-long-name.tar", i)
		}
		return paths
	}
	// A few 4 KiB paths: none fits whole, so the builder falls through to
	// the prefix search on the first one after judging every path.
	longPaths := make([]string, 8)
	for i := range longPaths {
		longPaths[i] = fmt.Sprintf("/%d", i) + strings.Repeat("d", 4*1024-2)
	}

	for _, bc := range []struct {
		name  string
		paths []string
	}{
		{"n=1000", manyPaths(1000)},
		{"n=10000", manyPaths(10000)},
		{"4KiB_paths", longPaths},
	} {
		b.Run(bc.name, func(b *testing.B) {
			record := slog.NewRecord(time.Now(), slog.LevelError, "Pre-execution error occurred", 0)
			record.AddAttrs(NotificationAttrs(PreExecutionErrorNotification(), common.GroupScope("backup"))...)
			record.AddAttrs(
				slog.String(common.PreExecErrorAttrs.ErrorType, string(ErrorTypeGroupFileVerification)),
				slog.String(common.PreExecErrorAttrs.ErrorMessage, "Total: 1, Verified: 0, Failed: 1, Error: group file verification failed"),
				slog.String(common.PreExecErrorAttrs.Component, "verification"),
				slog.Any(common.PreExecErrorAttrs.FailedFilePaths, bc.paths),
			)
			handler := redaction.NewRedactingHandler(tu.NewCallbackHandler(func(r slog.Record) {
				_ = buildPreExecutionError(r)
			}), nil, nil)
			ctx := context.Background()

			b.ReportAllocs()
			for b.Loop() {
				if err := handler.Handle(ctx, record.Clone()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
