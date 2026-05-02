package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test for Config struct
func TestConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		wantErr  bool
		testFunc func(t *testing.T, config Config)
	}{
		{
			name: "valid config with file output",
			config: Config{
				Path:    "/tmp/test-output.txt",
				MaxSize: 1024 * 1024, // 1MB
			},
			wantErr: false,
			testFunc: func(t *testing.T, config Config) {
				assert.Equal(t, "/tmp/test-output.txt", config.Path)
				assert.Equal(t, int64(1024*1024), config.MaxSize)
			},
		},
		{
			name: "default config values",
			config: Config{
				Path:    "",
				MaxSize: 0,
			},
			wantErr: false,
			testFunc: func(t *testing.T, config Config) {
				assert.Equal(t, "", config.Path)
				assert.Equal(t, int64(0), config.MaxSize)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.testFunc != nil {
				tt.testFunc(t, tt.config)
			}
		})
	}
}

// Test for Analysis struct (for Dry-Run)
func TestAnalysis(t *testing.T) {
	tests := []struct {
		name     string
		analysis Analysis
		testFunc func(t *testing.T, analysis Analysis)
	}{
		{
			name: "safe output analysis",
			analysis: Analysis{
				OutputPath:      "/home/user/output.txt",
				ResolvedPath:    "/home/user/output.txt",
				DirectoryExists: true,
				WritePermission: true,
				SecurityRisk:    RiskLevelLow,
				MaxSizeLimit:    10 * 1024 * 1024, // 10MB
			},
			testFunc: func(t *testing.T, analysis Analysis) {
				assert.True(t, analysis.DirectoryExists, "Expected DirectoryExists to be true")
				assert.True(t, analysis.WritePermission, "Expected WritePermission to be true")
				assert.Equal(t, RiskLevelLow, analysis.SecurityRisk, "Expected SecurityRisk Low")
			},
		},
		{
			name: "risky output analysis",
			analysis: Analysis{
				OutputPath:      "../../../etc/passwd",
				ResolvedPath:    "/etc/passwd",
				DirectoryExists: false,
				WritePermission: false,
				SecurityRisk:    RiskLevelHigh,
				MaxSizeLimit:    1024 * 1024, // 1MB
			},
			testFunc: func(t *testing.T, analysis Analysis) {
				assert.False(t, analysis.DirectoryExists, "Expected DirectoryExists to be false")
				assert.False(t, analysis.WritePermission, "Expected WritePermission to be false")
				assert.Equal(t, RiskLevelHigh, analysis.SecurityRisk, "Expected SecurityRisk High")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.testFunc != nil {
				tt.testFunc(t, tt.analysis)
			}
		})
	}
}
