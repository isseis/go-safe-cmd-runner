package config

import (
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyGlobalDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    *runnertypes.GlobalSpec
		expected *runnertypes.GlobalSpec
	}{
		{
			name: "VerifyStandardPaths nil -> default true",
			input: &runnertypes.GlobalSpec{
				VerifyStandardPaths: nil,
			},
			expected: &runnertypes.GlobalSpec{
				VerifyStandardPaths: commontesting.BoolPtr(true),
			},
		},
		{
			name: "VerifyStandardPaths true -> unchanged",
			input: &runnertypes.GlobalSpec{
				VerifyStandardPaths: commontesting.BoolPtr(true),
			},
			expected: &runnertypes.GlobalSpec{
				VerifyStandardPaths: commontesting.BoolPtr(true),
			},
		},
		{
			name: "VerifyStandardPaths false -> unchanged",
			input: &runnertypes.GlobalSpec{
				VerifyStandardPaths: commontesting.BoolPtr(false),
			},
			expected: &runnertypes.GlobalSpec{
				VerifyStandardPaths: commontesting.BoolPtr(false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ApplyGlobalDefaults(tt.input)

			// Check VerifyStandardPaths
			switch {
			case tt.input.VerifyStandardPaths == nil && tt.expected.VerifyStandardPaths != nil:
				assert.Fail(t, "VerifyStandardPaths mismatch", "got nil, want %v", *tt.expected.VerifyStandardPaths)
			case tt.input.VerifyStandardPaths != nil && tt.expected.VerifyStandardPaths == nil:
				assert.Fail(t, "VerifyStandardPaths mismatch", "got %v, want nil", *tt.input.VerifyStandardPaths)
			case tt.input.VerifyStandardPaths != nil && tt.expected.VerifyStandardPaths != nil:
				assert.Equal(t, *tt.expected.VerifyStandardPaths, *tt.input.VerifyStandardPaths, "VerifyStandardPaths value mismatch")
			}
		})
	}
}

func TestApplyCommandDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    *runnertypes.CommandSpec
		expected *runnertypes.CommandSpec
	}{
		{
			name: "RiskLevel nil -> stays nil (no default applied)",
			input: &runnertypes.CommandSpec{
				RiskLevel: nil,
			},
			expected: &runnertypes.CommandSpec{
				RiskLevel: nil,
			},
		},
		{
			name: "RiskLevel medium -> unchanged",
			input: &runnertypes.CommandSpec{
				RiskLevel: func() *string { s := "medium"; return &s }(),
			},
			expected: &runnertypes.CommandSpec{
				RiskLevel: func() *string { s := "medium"; return &s }(),
			},
		},
		{
			name: "RiskLevel high -> unchanged",
			input: &runnertypes.CommandSpec{
				RiskLevel: func() *string { s := "high"; return &s }(),
			},
			expected: &runnertypes.CommandSpec{
				RiskLevel: func() *string { s := "high"; return &s }(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ApplyCommandDefaults(tt.input)

			if tt.expected.RiskLevel == nil {
				assert.Nil(t, tt.input.RiskLevel, "RiskLevel should be nil")
			} else {
				require.NotNil(t, tt.input.RiskLevel, "RiskLevel should not be nil")
				assert.Equal(t, *tt.expected.RiskLevel, *tt.input.RiskLevel, "RiskLevel mismatch")
			}
		})
	}
}
