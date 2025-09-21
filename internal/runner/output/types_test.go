package output

import (
	"testing"
	"time"
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

// Test for Capture struct
func TestCapture(t *testing.T) {
	tests := []struct {
		name     string
		capture  Capture
		testFunc func(t *testing.T, capture *Capture)
	}{
		{
			name: "new output capture with memory buffer",
			capture: Capture{
				OutputPath:  "/tmp/final-output.txt",
				FileHandle:  nil,              // Will be set by PrepareOutput in real usage
				MaxSize:     10 * 1024 * 1024, // 10MB
				CurrentSize: 0,
				StartTime:   time.Now(),
			},
			testFunc: func(t *testing.T, capture *Capture) {
				if capture.OutputPath != "/tmp/final-output.txt" {
					t.Errorf("Expected OutputPath '/tmp/final-output.txt', got '%s'", capture.OutputPath)
				}
				// FileHandle will be set by PrepareOutput in real usage
				// In this test context, nil is acceptable
				if capture.MaxSize != 10*1024*1024 {
					t.Errorf("Expected MaxSize 10485760, got %d", capture.MaxSize)
				}
				if capture.CurrentSize != 0 {
					t.Errorf("Expected CurrentSize 0, got %d", capture.CurrentSize)
				}
			},
		},
		{
			name: "capture with accumulated size",
			capture: Capture{
				OutputPath:  "/var/log/command.log",
				FileHandle:  nil,         // Will be set by PrepareOutput in real usage
				MaxSize:     1024 * 1024, // 1MB
				CurrentSize: 512 * 1024,  // 512KB
				StartTime:   time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			testFunc: func(t *testing.T, capture *Capture) {
				if capture.CurrentSize != 512*1024 {
					t.Errorf("Expected CurrentSize 524288, got %d", capture.CurrentSize)
				}
				if capture.MaxSize <= capture.CurrentSize {
					t.Errorf("CurrentSize (%d) should be less than MaxSize (%d)", capture.CurrentSize, capture.MaxSize)
				}
			},
		},
	}

	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if tt.testFunc != nil {
				tt.testFunc(t, &tt.capture)
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
