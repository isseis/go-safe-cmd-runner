package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestLoader_ApplyDefaults_Integration(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		expected func(*runnertypes.ConfigSpec) bool
	}{
		{
			name: "verify_standard_paths omitted -> default true",
			toml: `
version = "1.0"

[global]
timeout = 300

[[groups]]
name = "test"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
`,
			expected: func(cfg *runnertypes.ConfigSpec) bool {
				return cfg.Global.VerifyStandardPaths != nil && *cfg.Global.VerifyStandardPaths == true
			},
		},
		{
			name: "verify_standard_paths = false -> unchanged",
			toml: `
version = "1.0"

[global]
verify_standard_paths = false
timeout = 300

[[groups]]
name = "test"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
`,
			expected: func(cfg *runnertypes.ConfigSpec) bool {
				return cfg.Global.VerifyStandardPaths != nil && *cfg.Global.VerifyStandardPaths == false
			},
		},
		{
			name: "risk_level omitted -> default low",
			toml: `
version = "1.0"

[global]
timeout = 300

[[groups]]
name = "test"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
`,
			expected: func(cfg *runnertypes.ConfigSpec) bool {
				return len(cfg.Groups) > 0 && len(cfg.Groups[0].Commands) > 0 &&
					cfg.Groups[0].Commands[0].RiskLevel == "low"
			},
		},
		{
			name: "risk_level = medium -> unchanged",
			toml: `
version = "1.0"

[global]
timeout = 300

[[groups]]
name = "test"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
risk_level = "medium"
`,
			expected: func(cfg *runnertypes.ConfigSpec) bool {
				return len(cfg.Groups) > 0 && len(cfg.Groups[0].Commands) > 0 &&
					cfg.Groups[0].Commands[0].RiskLevel == "medium"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.toml))
			if err != nil {
				assert.NoError(t, err, "LoadConfig() should not error")
				return
			}

			if !tt.expected(cfg) {
				assert.Fail(t, "Default values not applied correctly")
				t.Logf("Global.VerifyStandardPaths: %v", cfg.Global.VerifyStandardPaths)
				if len(cfg.Groups) > 0 && len(cfg.Groups[0].Commands) > 0 {
					t.Logf("Commands[0].RiskLevel: %v", cfg.Groups[0].Commands[0].RiskLevel)
				}
			}
		})
	}
}
