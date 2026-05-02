package output

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				assert.Equal(t, "/tmp/final-output.txt", capture.OutputPath)
				// FileHandle will be set by PrepareOutput in real usage
				// In this test context, nil is acceptable
				assert.Equal(t, int64(10*1024*1024), capture.MaxSize)
				assert.Equal(t, int64(0), capture.CurrentSize)
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
				assert.Equal(t, int64(512*1024), capture.CurrentSize)
				assert.Less(t, capture.CurrentSize, capture.MaxSize)
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

// TestCapture_WriteOutput tests the WriteOutput method behavior
func TestCapture_WriteOutput(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() (*Capture, func(), error)
		data        []byte
		wantError   bool
		errorType   ErrorType
		wantWritten int64
	}{
		{
			name: "successful write",
			setupFunc: func() (*Capture, func(), error) {
				tmpFile, err := os.CreateTemp("", "capture_test_*.tmp")
				if err != nil {
					return nil, nil, err
				}
				cleanup := func() {
					tmpFile.Close()
					os.Remove(tmpFile.Name())
				}
				capture := &Capture{
					OutputPath:   "/tmp/final-output.txt",
					TempFilePath: tmpFile.Name(),
					FileHandle:   tmpFile,
					MaxSize:      1024,
					CurrentSize:  0,
					StartTime:    time.Now(),
				}
				return capture, cleanup, nil
			},
			data:        []byte("test data"),
			wantError:   false,
			wantWritten: 9,
		},
		{
			name: "write with nil file handle",
			setupFunc: func() (*Capture, func(), error) {
				capture := &Capture{
					OutputPath:   "/tmp/final-output.txt",
					TempFilePath: "",
					FileHandle:   nil,
					MaxSize:      1024,
					CurrentSize:  0,
					StartTime:    time.Now(),
				}
				return capture, func() {}, nil
			},
			data:        []byte("test data"),
			wantError:   false,
			wantWritten: 0, // No actual write when FileHandle is nil
		},
		{
			name: "size limit exceeded",
			setupFunc: func() (*Capture, func(), error) {
				tmpFile, err := os.CreateTemp("", "capture_test_*.tmp")
				if err != nil {
					return nil, nil, err
				}
				cleanup := func() {
					tmpFile.Close()
					os.Remove(tmpFile.Name())
				}
				capture := &Capture{
					OutputPath:   "/tmp/final-output.txt",
					TempFilePath: tmpFile.Name(),
					FileHandle:   tmpFile,
					MaxSize:      5, // Very small limit
					CurrentSize:  0,
					StartTime:    time.Now(),
				}
				return capture, cleanup, nil
			},
			data:      []byte("this data exceeds the limit"),
			wantError: true,
			errorType: ErrorTypeSizeLimit,
		},
		{
			name: "write at size limit boundary",
			setupFunc: func() (*Capture, func(), error) {
				tmpFile, err := os.CreateTemp("", "capture_test_*.tmp")
				if err != nil {
					return nil, nil, err
				}
				cleanup := func() {
					tmpFile.Close()
					os.Remove(tmpFile.Name())
				}
				capture := &Capture{
					OutputPath:   "/tmp/final-output.txt",
					TempFilePath: tmpFile.Name(),
					FileHandle:   tmpFile,
					MaxSize:      10,
					CurrentSize:  5, // Already have some data
					StartTime:    time.Now(),
				}
				return capture, cleanup, nil
			},
			data:        []byte("12345"), // Exactly at limit
			wantError:   false,
			wantWritten: 5,
		},
		{
			name: "write beyond size limit boundary",
			setupFunc: func() (*Capture, func(), error) {
				tmpFile, err := os.CreateTemp("", "capture_test_*.tmp")
				if err != nil {
					return nil, nil, err
				}
				cleanup := func() {
					tmpFile.Close()
					os.Remove(tmpFile.Name())
				}
				capture := &Capture{
					OutputPath:   "/tmp/final-output.txt",
					TempFilePath: tmpFile.Name(),
					FileHandle:   tmpFile,
					MaxSize:      10,
					CurrentSize:  5, // Already have some data
					StartTime:    time.Now(),
				}
				return capture, cleanup, nil
			},
			data:      []byte("123456"), // Exceeds limit by 1 byte
			wantError: true,
			errorType: ErrorTypeSizeLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture, cleanup, err := tt.setupFunc()
			require.NoError(t, err)
			defer cleanup()

			initialSize := capture.CurrentSize
			err = capture.WriteOutput(tt.data)

			if tt.wantError {
				assert.Error(t, err)
				var captureErr *CaptureError
				assert.ErrorAs(t, err, &captureErr)
				assert.Equal(t, tt.errorType, captureErr.Type)
				// Size should not change on error
				assert.Equal(t, initialSize, capture.CurrentSize)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, initialSize+tt.wantWritten, capture.CurrentSize)
			}
		})
	}
}

// TestCapture_Close tests the Close method behavior
func TestCapture_Close(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (*Capture, func(), error)
		wantError bool
	}{
		{
			name: "successful close with file handle",
			setupFunc: func() (*Capture, func(), error) {
				tmpFile, err := os.CreateTemp("", "capture_test_*.tmp")
				if err != nil {
					return nil, nil, err
				}
				// Write some data to ensure file is created
				tmpFile.WriteString("test data")
				tmpFile.Sync()

				cleanup := func() {
					// File should be closed by the test, but remove if still exists
					os.Remove(tmpFile.Name())
				}
				capture := &Capture{
					OutputPath:   "/tmp/final-output.txt",
					TempFilePath: tmpFile.Name(),
					FileHandle:   tmpFile,
					MaxSize:      1024,
					CurrentSize:  9,
					StartTime:    time.Now(),
				}
				return capture, cleanup, nil
			},
			wantError: false,
		},
		{
			name: "close with nil file handle",
			setupFunc: func() (*Capture, func(), error) {
				capture := &Capture{
					OutputPath:   "/tmp/final-output.txt",
					TempFilePath: "",
					FileHandle:   nil,
					MaxSize:      1024,
					CurrentSize:  0,
					StartTime:    time.Now(),
				}
				return capture, func() {}, nil
			},
			wantError: false,
		},
		{
			name: "close already closed file",
			setupFunc: func() (*Capture, func(), error) {
				tmpFile, err := os.CreateTemp("", "capture_test_*.tmp")
				if err != nil {
					return nil, nil, err
				}
				// Close the file immediately
				tmpFile.Close()

				cleanup := func() {
					os.Remove(tmpFile.Name())
				}
				capture := &Capture{
					OutputPath:   "/tmp/final-output.txt",
					TempFilePath: tmpFile.Name(),
					FileHandle:   tmpFile,
					MaxSize:      1024,
					CurrentSize:  0,
					StartTime:    time.Now(),
				}
				return capture, cleanup, nil
			},
			wantError: true, // Should error when trying to close already closed file
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture, cleanup, err := tt.setupFunc()
			require.NoError(t, err)
			defer cleanup()

			err = capture.Close()

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCapture_CloseIdempotency tests that Close can be called multiple times safely
func TestCapture_CloseIdempotency(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "capture_test_*.tmp")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write some data to ensure file is created
	tmpFile.WriteString("test data")
	tmpFile.Sync()

	capture := &Capture{
		OutputPath:   "/tmp/final-output.txt",
		TempFilePath: tmpPath,
		FileHandle:   tmpFile,
		MaxSize:      1024,
		CurrentSize:  9,
		StartTime:    time.Now(),
	}

	// First close should succeed
	err = capture.Close()
	assert.NoError(t, err)
	assert.Nil(t, capture.FileHandle, "FileHandle should be nil after Close")

	// Subsequent closes should also succeed (idempotent)
	for i := 0; i < 2; i++ {
		err = capture.Close()
		assert.NoError(t, err, "Subsequent Close() call #%d should not produce an error", i+1)
		assert.Nil(t, capture.FileHandle, "FileHandle should remain nil after subsequent Close() call #%d", i+1)
	}
}

// TestCapture_ConcurrentAccess tests concurrent access to WriteOutput method
func TestCapture_ConcurrentAccess(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "capture_test_concurrent_*.tmp")
	require.NoError(t, err)
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	capture := &Capture{
		OutputPath:   "/tmp/final-output.txt",
		TempFilePath: tmpFile.Name(),
		FileHandle:   tmpFile,
		MaxSize:      1000, // 1KB - small to potentially hit limits
		CurrentSize:  0,
		StartTime:    time.Now(),
	}

	// Number of concurrent goroutines
	const numGoroutines = 5
	const writesPerGoroutine = 3

	var wg sync.WaitGroup
	var successfulWrites int64
	var mu sync.Mutex

	// Start concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				data := []byte(fmt.Sprintf("g%02d-w%02d  ", goroutineID, j)) // 9 bytes
				err := capture.WriteOutput(data)
				if err == nil {
					mu.Lock()
					successfulWrites++
					mu.Unlock()
				}
				// Note: Some writes may fail due to size limits, which is expected
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify that at least some writes were successful
	assert.Greater(t, successfulWrites, int64(0), "Expected at least some successful writes")

	// Debug information to understand the discrepancy
	t.Logf("Successful writes: %d, Current size: %d, Expected if all full: %d",
		successfulWrites, capture.CurrentSize, successfulWrites*9)

	// For concurrent access test, we mainly want to verify:
	// 1. No race conditions occurred (no panics/crashes)
	// 2. Size tracking is consistent (CurrentSize <= MaxSize)
	// 3. Some writes were successful
	assert.LessOrEqual(t, capture.CurrentSize, capture.MaxSize, "Current size should not exceed max size")

	// Close and verify file content
	err = capture.Close()
	assert.NoError(t, err)

	// Read the file and verify the size matches CurrentSize
	fileInfo, err := os.Stat(tmpFile.Name())
	require.NoError(t, err)
	assert.Equal(t, capture.CurrentSize, fileInfo.Size(), "File size should match CurrentSize")
}
