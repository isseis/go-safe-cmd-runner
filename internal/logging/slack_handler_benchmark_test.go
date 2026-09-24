//go:build test

// Package logging provides benchmark tests for slack_handler.go: command
// result extraction and failed-file list rendering.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
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

// benchmarkFailedPaths returns n distinct paths of equal length.
func benchmarkFailedPaths(n int) []string {
	paths := make([]string, n)
	for i := range paths {
		paths[i] = fmt.Sprintf("/var/lib/backup/%05d/snapshot-with-a-long-name.tar", i)
	}
	return paths
}

// benchmarkFailedFilesDetail is the error_message the failed-file benchmarks
// render their lists after.
const benchmarkFailedFilesDetail = "Total: 1, Verified: 0, Failed: 1, Error: group file verification failed"

// BenchmarkBuildPreExecutionError_FailedFilePaths measures rendering a
// failed-file list end to end, as production does it: the RedactingHandler
// pass (which redacts every element and turns the []string into []any)
// followed by the builder's budgeted selection. Most of this cost is the
// existing per-element redaction, which grows linearly with the list; see
// BenchmarkRenderFailedFiles for the builder's own share and scaling. Timing
// depends on the machine, so no test asserts it.
func BenchmarkBuildPreExecutionError_FailedFilePaths(b *testing.B) {
	// A few 4 KiB paths: none fits whole, so the builder falls through to
	// the prefix search on the first one after judging every path.
	longPaths := make([]string, 8)
	for i := range longPaths {
		longPaths[i] = fmt.Sprintf("/%d", i) + strings.Repeat("d", 4*1024-2)
	}

	// The production handler adds the webhook host to the value detector.
	config, err := redaction.NewConfig(redaction.WithWebhookHost("hooks.slack.com"))
	if err != nil {
		b.Fatal(err)
	}

	for _, bc := range []struct {
		name  string
		paths []string
	}{
		{"n=1000", benchmarkFailedPaths(1000)},
		{"n=10000", benchmarkFailedPaths(10000)},
		{"4KiB_paths", longPaths},
	} {
		b.Run(bc.name, func(b *testing.B) {
			record := slog.NewRecord(time.Now(), slog.LevelError, "Pre-execution error occurred", 0)
			record.AddAttrs(NotificationAttrs(PreExecutionErrorNotification(), common.GroupScope("backup"))...)
			record.AddAttrs(
				slog.String(common.PreExecErrorAttrs.ErrorType, string(ErrorTypeGroupFileVerification)),
				slog.String(common.PreExecErrorAttrs.ErrorMessage, benchmarkFailedFilesDetail),
				slog.String(common.PreExecErrorAttrs.Component, "verification"),
				slog.Any(common.PreExecErrorAttrs.FailedFilePaths, bc.paths),
			)
			var details messageDetails
			handler := redaction.NewRedactingHandler(tu.NewCallbackHandler(func(r slog.Record) {
				details = buildPreExecutionError(r)
			}), config, nil)
			ctx := context.Background()

			// Fail fast if the list is not rendered, so the numbers cannot
			// silently measure the no-list path.
			if err := handler.Handle(ctx, record.Clone()); err != nil {
				b.Fatal(err)
			}
			if !slices.ContainsFunc(details.fields, func(f SlackAttachmentField) bool {
				return f.Title == "Error Message" && strings.Contains(f.Value, ", Files: ")
			}) {
				b.Fatalf("the Error Message carries no Files section: %v", details.fields)
			}

			b.ReportAllocs()
			for b.Loop() {
				if err := handler.Handle(ctx, record.Clone()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRenderFailedFiles measures the builder's own share of rendering a
// failed-file list, without the redaction pass that dominates the end-to-end
// figure. This is the stage the absolute budget targets: a 10,000-path list
// under 100 ms, with a per-path cost at 10,000 within 3x of that at 1,000 so
// quadratic growth in the selection shows up.
func BenchmarkRenderFailedFiles(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		paths := benchmarkFailedPaths(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = renderFailedFiles(benchmarkFailedFilesDetail, paths)
			}
		})
	}
}
