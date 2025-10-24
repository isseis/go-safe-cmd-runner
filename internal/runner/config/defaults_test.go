package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
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
				VerifyStandardPaths: common.BoolPtr(true),
			},
		},
		{
			name: "VerifyStandardPaths true -> unchanged",
			input: &runnertypes.GlobalSpec{
				VerifyStandardPaths: common.BoolPtr(true),
			},
			expected: &runnertypes.GlobalSpec{
				VerifyStandardPaths: common.BoolPtr(true),
			},
		},
		{
			name: "VerifyStandardPaths false -> unchanged",
			input: &runnertypes.GlobalSpec{
				VerifyStandardPaths: common.BoolPtr(false),
			},
			expected: &runnertypes.GlobalSpec{
				VerifyStandardPaths: common.BoolPtr(false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ApplyGlobalDefaults(tt.input)

			// Check VerifyStandardPaths
			switch {
			case tt.input.VerifyStandardPaths == nil && tt.expected.VerifyStandardPaths != nil:
				t.Errorf("VerifyStandardPaths: got nil, want %v", *tt.expected.VerifyStandardPaths)
			case tt.input.VerifyStandardPaths != nil && tt.expected.VerifyStandardPaths == nil:
				t.Errorf("VerifyStandardPaths: got %v, want nil", *tt.input.VerifyStandardPaths)
			case tt.input.VerifyStandardPaths != nil && tt.expected.VerifyStandardPaths != nil:
				if *tt.input.VerifyStandardPaths != *tt.expected.VerifyStandardPaths {
					t.Errorf("VerifyStandardPaths: got %v, want %v", *tt.input.VerifyStandardPaths, *tt.expected.VerifyStandardPaths)
				}
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
			name: "RiskLevel empty -> default low",
			input: &runnertypes.CommandSpec{
				RiskLevel: "",
			},
			expected: &runnertypes.CommandSpec{
				RiskLevel: "low",
			},
		},
		{
			name: "RiskLevel medium -> unchanged",
			input: &runnertypes.CommandSpec{
				RiskLevel: "medium",
			},
			expected: &runnertypes.CommandSpec{
				RiskLevel: "medium",
			},
		},
		{
			name: "RiskLevel high -> unchanged",
			input: &runnertypes.CommandSpec{
				RiskLevel: "high",
			},
			expected: &runnertypes.CommandSpec{
				RiskLevel: "high",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ApplyCommandDefaults(tt.input)

			if tt.input.RiskLevel != tt.expected.RiskLevel {
				t.Errorf("RiskLevel: got %v, want %v", tt.input.RiskLevel, tt.expected.RiskLevel)
			}
		})
	}
}
