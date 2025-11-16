//go:build test

package logging

import (
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// BenchmarkExtractCommandResults_Current measures the current implementation
func BenchmarkExtractCommandResults_Current(b *testing.B) {
	// Create test data with []common.CommandResult (direct format)
	commands := []common.CommandResult{
		{CommandResultFields: common.CommandResultFields{Name: "cmd1", ExitCode: 0, Output: "output1", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd2", ExitCode: 1, Output: "output2", Stderr: "error2"}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd3", ExitCode: 0, Output: "output3", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd4", ExitCode: 0, Output: "output4", Stderr: ""}},
		{CommandResultFields: common.CommandResultFields{Name: "cmd5", ExitCode: 1, Output: "", Stderr: "error5"}},
	}
	value := slog.AnyValue(commands)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractCommandResults_AfterRedaction measures performance with []any (after RedactingHandler)
func BenchmarkExtractCommandResults_AfterRedaction(b *testing.B) {
	// Create test data with []any (simulating RedactingHandler output)
	commands := []any{
		common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd1", ExitCode: 0, Output: "output1", Stderr: ""}},
		common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd2", ExitCode: 1, Output: "output2", Stderr: "error2"}},
		common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd3", ExitCode: 0, Output: "output3", Stderr: ""}},
		common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd4", ExitCode: 0, Output: "output4", Stderr: ""}},
		common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd5", ExitCode: 1, Output: "", Stderr: "error5"}},
	}
	value := slog.AnyValue(commands)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractCommandResults_WithSlogValue measures performance with slog.Value elements
func BenchmarkExtractCommandResults_WithSlogValue(b *testing.B) {
	// Create test data with []any containing slog.Value elements
	cmd1 := common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd1", ExitCode: 0, Output: "output1", Stderr: ""}}
	cmd2 := common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd2", ExitCode: 1, Output: "output2", Stderr: "error2"}}
	cmd3 := common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd3", ExitCode: 0, Output: "output3", Stderr: ""}}
	cmd4 := common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd4", ExitCode: 0, Output: "output4", Stderr: ""}}
	cmd5 := common.CommandResult{CommandResultFields: common.CommandResultFields{Name: "cmd5", ExitCode: 1, Output: "", Stderr: "error5"}}

	commands := []any{
		cmd1.LogValue(),
		cmd2.LogValue(),
		cmd3.LogValue(),
		cmd4.LogValue(),
		cmd5.LogValue(),
	}
	value := slog.AnyValue(commands)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractCommandResults(value)
	}
}

// BenchmarkExtractCommandResults_WithAttrSlice measures performance with []slog.Attr elements
func BenchmarkExtractCommandResults_WithAttrSlice(b *testing.B) {
	// Create test data with []any containing []slog.Attr elements
	commands := []any{
		[]slog.Attr{
			slog.String(common.LogFieldName, "cmd1"),
			slog.Int(common.LogFieldExitCode, 0),
			slog.String(common.LogFieldOutput, "output1"),
			slog.String(common.LogFieldStderr, ""),
		},
		[]slog.Attr{
			slog.String(common.LogFieldName, "cmd2"),
			slog.Int(common.LogFieldExitCode, 1),
			slog.String(common.LogFieldOutput, "output2"),
			slog.String(common.LogFieldStderr, "error2"),
		},
		[]slog.Attr{
			slog.String(common.LogFieldName, "cmd3"),
			slog.Int(common.LogFieldExitCode, 0),
			slog.String(common.LogFieldOutput, "output3"),
			slog.String(common.LogFieldStderr, ""),
		},
		[]slog.Attr{
			slog.String(common.LogFieldName, "cmd4"),
			slog.Int(common.LogFieldExitCode, 0),
			slog.String(common.LogFieldOutput, "output4"),
			slog.String(common.LogFieldStderr, ""),
		},
		[]slog.Attr{
			slog.String(common.LogFieldName, "cmd5"),
			slog.Int(common.LogFieldExitCode, 1),
			slog.String(common.LogFieldOutput, ""),
			slog.String(common.LogFieldStderr, "error5"),
		},
	}
	value := slog.AnyValue(commands)

	b.ResetTimer()
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
	for i := 0; i < b.N; i++ {
		_ = extractFromAttrs(attrs)
	}
}
