package output

import (
	"testing"
	"time"
)

// Test for Capture struct
func TestCapture(t *testing.T) {
	tests := []*struct {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.testFunc != nil {
				tt.testFunc(t, &tt.capture)
			}
		})
	}
}
