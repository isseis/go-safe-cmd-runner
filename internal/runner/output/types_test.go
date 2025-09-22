package output

import (
	"testing"
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
				if config.Path != "/tmp/test-output.txt" {
					t.Errorf("Expected Path '/tmp/test-output.txt', got '%s'", config.Path)
				}
				if config.MaxSize != 1024*1024 {
					t.Errorf("Expected MaxSize 1048576, got %d", config.MaxSize)
				}
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
				if config.Path != "" {
					t.Errorf("Expected empty Path, got '%s'", config.Path)
				}
				if config.MaxSize != 0 {
					t.Errorf("Expected MaxSize 0, got %d", config.MaxSize)
				}
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
				if !analysis.DirectoryExists {
					t.Error("Expected DirectoryExists to be true")
				}
				if !analysis.WritePermission {
					t.Error("Expected WritePermission to be true")
				}
				if analysis.SecurityRisk != RiskLevelLow {
					t.Errorf("Expected SecurityRisk Low, got %v", analysis.SecurityRisk)
				}
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
				if analysis.DirectoryExists {
					t.Error("Expected DirectoryExists to be false")
				}
				if analysis.WritePermission {
					t.Error("Expected WritePermission to be false")
				}
				if analysis.SecurityRisk != RiskLevelHigh {
					t.Errorf("Expected SecurityRisk High, got %v", analysis.SecurityRisk)
				}
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
