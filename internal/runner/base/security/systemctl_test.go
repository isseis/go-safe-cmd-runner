//go:build test

package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
)

// TestFirstSystemctlSubcommand exercises the hand-written argv parser, in
// particular the option handling that must not let a change verb slip through as
// a read-only subcommand.
func TestFirstSystemctlSubcommand(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantVerb      string
		wantForceHigh bool
	}{
		{"plain verb", []string{"status", "nginx"}, "status", false},
		{"no args", nil, "", false},
		{"value option consumes next token", []string{"-t", "service", "status"}, "status", false},
		{"long value option consumes next token", []string{"--host", "h1", "restart"}, "restart", false},
		{"signal value option consumes next token", []string{"-s", "SIGKILL", "kill", "nginx"}, "kill", false},
		{"lines value option consumes next token", []string{"-n", "10", "status"}, "status", false},
		{"combined option is self-contained", []string{"--type=service", "list-units"}, "list-units", false},
		{"boolean option is skipped", []string{"--no-pager", "status"}, "status", false},
		{"unknown combined option is skipped safely", []string{"--legend=false", "status"}, "status", false},
		{"unknown separate option forces high", []string{"--mystery", "status"}, "", true},
		{"option terminator takes next token", []string{"--", "restart"}, "restart", false},
		{"value option without following token", []string{"-t"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verb, forceHigh := firstSystemctlSubcommand(tt.args)
			assert.Equal(t, tt.wantVerb, verb, "verb")
			assert.Equal(t, tt.wantForceHigh, forceHigh, "forceHigh")
		})
	}
}

// TestSystemctlSubcommandRisk verifies the verb-to-risk mapping: change verbs and
// unknown/unidentifiable verbs are High, read-only verbs and an omitted
// subcommand are a Medium floor (never Low).
func TestSystemctlSubcommandRisk(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want runnertypes.RiskLevel
	}{
		{"change verb restart", []string{"restart", "nginx"}, runnertypes.RiskLevelHigh},
		{"change verb daemon-reload", []string{"daemon-reload"}, runnertypes.RiskLevelHigh},
		{"change verb isolate", []string{"isolate", "rescue.target"}, runnertypes.RiskLevelHigh},
		{"read-only status is medium floor", []string{"status", "nginx"}, runnertypes.RiskLevelMedium},
		{"read-only show is medium floor", []string{"show", "nginx"}, runnertypes.RiskLevelMedium},
		{"omitted subcommand is medium floor", nil, runnertypes.RiskLevelMedium},
		{"unknown verb is high", []string{"frobnicate"}, runnertypes.RiskLevelHigh},
		{"hidden change verb behind value option is detected", []string{"-t", "service", "stop", "nginx"}, runnertypes.RiskLevelHigh},
		{"unknown separate option forces high", []string{"--mystery", "status"}, runnertypes.RiskLevelHigh},
		{"read-only verb after boolean option stays medium", []string{"--no-pager", "list-units"}, runnertypes.RiskLevelMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SystemctlSubcommandRisk(tt.args))
		})
	}
}
