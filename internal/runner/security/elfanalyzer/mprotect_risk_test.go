//go:build test

package elfanalyzer

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestEvalMprotectRisk(t *testing.T) {
	tests := []struct {
		name    string
		results []common.SyscallArgEvalResult
		want    bool
	}{
		{
			name: "exec_confirmed returns true",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecConfirmed},
			},
			want: true,
		},
		{
			name: "exec_unknown returns true",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecUnknown},
			},
			want: true,
		},
		{
			name: "exec_not_set returns false",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet},
			},
			want: false,
		},
		{
			name:    "empty list returns false",
			results: []common.SyscallArgEvalResult{},
			want:    false,
		},
		{
			name:    "nil list returns false",
			results: nil,
			want:    false,
		},
		{
			name: "non-mprotect entry is ignored",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "mmap", Status: common.SyscallArgEvalExecConfirmed},
			},
			want: false,
		},
		{
			name: "non-mprotect entry with mprotect exec_not_set",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "mmap", Status: common.SyscallArgEvalExecConfirmed},
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet},
			},
			want: false,
		},
		// pkey_mprotect cases
		{
			name: "pkey_mprotect exec_confirmed returns true",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "pkey_mprotect", Status: common.SyscallArgEvalExecConfirmed},
			},
			want: true,
		},
		{
			name: "pkey_mprotect exec_unknown returns true",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "pkey_mprotect", Status: common.SyscallArgEvalExecUnknown},
			},
			want: true,
		},
		{
			name: "pkey_mprotect exec_not_set returns false",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "pkey_mprotect", Status: common.SyscallArgEvalExecNotSet},
			},
			want: false,
		},
		{
			name: "mprotect exec_not_set + pkey_mprotect exec_unknown returns true",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet},
				{SyscallName: "pkey_mprotect", Status: common.SyscallArgEvalExecUnknown},
			},
			want: true,
		},
		{
			name: "both exec_not_set returns false",
			results: []common.SyscallArgEvalResult{
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet},
				{SyscallName: "pkey_mprotect", Status: common.SyscallArgEvalExecNotSet},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvalMprotectRisk(tt.results)
			assert.Equal(t, tt.want, got)
		})
	}
}
