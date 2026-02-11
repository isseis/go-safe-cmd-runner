//go:build test

//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandResults_LogValue(t *testing.T) {
	tests := []struct {
		name     string
		results  CommandResults
		validate func(t *testing.T, value slog.Value)
	}{
		{
			name:    "nil slice",
			results: nil,
			validate: func(t *testing.T, value slog.Value) {
				assert.Equal(t, slog.KindGroup, value.Kind())
				attrs := value.Group()
				assert.Len(t, attrs, 2)
				assert.Equal(t, "total_count", attrs[0].Key)
				assert.Equal(t, int64(0), attrs[0].Value.Int64())
				assert.Equal(t, "truncated", attrs[1].Key)
				assert.False(t, attrs[1].Value.Bool())
			},
		},
		{
			name:    "empty slice",
			results: CommandResults{},
			validate: func(t *testing.T, value slog.Value) {
				assert.Equal(t, slog.KindGroup, value.Kind())
				attrs := value.Group()
				assert.Len(t, attrs, 2)
				assert.Equal(t, "total_count", attrs[0].Key)
				assert.Equal(t, int64(0), attrs[0].Value.Int64())
				assert.Equal(t, "truncated", attrs[1].Key)
				assert.False(t, attrs[1].Value.Bool())
			},
		},
		{
			name: "single command",
			results: CommandResults{
				{CommandResultFields: CommandResultFields{
					Name:     "test1",
					ExitCode: 0,
					Output:   "ok",
					Stderr:   "",
				}},
			},
			validate: func(t *testing.T, value slog.Value) {
				assert.Equal(t, slog.KindGroup, value.Kind())
				attrs := value.Group()
				assert.Len(t, attrs, 3) // total_count, truncated, cmd_0

				assert.Equal(t, "total_count", attrs[0].Key)
				assert.Equal(t, int64(1), attrs[0].Value.Int64())

				assert.Equal(t, "truncated", attrs[1].Key)
				assert.False(t, attrs[1].Value.Bool())

				assert.Equal(t, "cmd_0", attrs[2].Key)
				assert.Equal(t, slog.KindGroup, attrs[2].Value.Kind())

				cmdAttrs := attrs[2].Value.Group()
				assert.Len(t, cmdAttrs, 4)
				assert.Equal(t, "name", cmdAttrs[0].Key)
				assert.Equal(t, "test1", cmdAttrs[0].Value.String())
				assert.Equal(t, "exit_code", cmdAttrs[1].Key)
				assert.Equal(t, int64(0), cmdAttrs[1].Value.Int64())
			},
		},
		{
			name: "multiple commands",
			results: CommandResults{
				{CommandResultFields: CommandResultFields{Name: "test1", ExitCode: 0, Output: "out1", Stderr: ""}},
				{CommandResultFields: CommandResultFields{Name: "test2", ExitCode: 1, Output: "", Stderr: "err2"}},
				{CommandResultFields: CommandResultFields{Name: "test3", ExitCode: 0, Output: "out3", Stderr: "warn3"}},
			},
			validate: func(t *testing.T, value slog.Value) {
				attrs := value.Group()
				assert.Len(t, attrs, 5) // total_count, truncated, cmd_0..cmd_2
				assert.Equal(t, int64(3), attrs[0].Value.Int64())
				assert.False(t, attrs[1].Value.Bool())
			},
		},
		{
			name:    "exactly max commands",
			results: createTestCommandResults(100),
			validate: func(t *testing.T, value slog.Value) {
				attrs := value.Group()
				assert.Len(t, attrs, 102) // total_count, truncated, cmd_0..cmd_99
				assert.Equal(t, int64(100), attrs[0].Value.Int64())
				assert.False(t, attrs[1].Value.Bool())

				for i := 0; i < 100; i++ {
					assert.Equal(t, fmt.Sprintf("cmd_%d", i), attrs[i+2].Key)
				}
			},
		},
		{
			name:    "one over limit is truncated",
			results: createTestCommandResults(101),
			validate: func(t *testing.T, value slog.Value) {
				attrs := value.Group()
				assert.Len(t, attrs, 102)
				assert.Equal(t, int64(101), attrs[0].Value.Int64())
				assert.True(t, attrs[1].Value.Bool())
			},
		},
		{
			name:    "large truncation",
			results: createTestCommandResults(150),
			validate: func(t *testing.T, value slog.Value) {
				attrs := value.Group()
				assert.Len(t, attrs, 102)
				assert.Equal(t, int64(150), attrs[0].Value.Int64())
				assert.True(t, attrs[1].Value.Bool())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.results.LogValue()
			tt.validate(t, value)
		})
	}
}

func createTestCommandResults(count int) CommandResults {
	results := make(CommandResults, count)
	for i := 0; i < count; i++ {
		results[i] = CommandResult{
			CommandResultFields: CommandResultFields{
				Name:     fmt.Sprintf("cmd%d", i),
				ExitCode: i % 3,
				Output:   fmt.Sprintf("output %d", i),
				Stderr:   fmt.Sprintf("stderr %d", i),
			},
		}
	}

	return results
}
