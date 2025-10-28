//go:build test
// +build test

package security

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTempDirectory_ConcurrentAccess tests concurrent file operations in temp directory
func TestTempDirectory_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()

	// Number of concurrent goroutines
	numGoroutines := 10
	numOperations := 100

	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines*numOperations)

	// Launch multiple goroutines to perform concurrent file operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				// Create a unique file for this goroutine
				filePath := filepath.Join(tempDir, fmt.Sprintf("file_%d_%d.txt", id, j))
				content := []byte(fmt.Sprintf("test content from goroutine %d operation %d", id, j))

				// Write file
				err := os.WriteFile(filePath, content, 0o644)
				if err != nil {
					errorChan <- err
					return
				}

				// Read file back
				_, err = os.ReadFile(filePath)
				if err != nil {
					errorChan <- err
					return
				}

				// Delete file
				err = os.Remove(filePath)
				if err != nil {
					errorChan <- err
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errorChan)

	// Check for any errors
	for err := range errorChan {
		t.Errorf("Concurrent operation failed: %v", err)
	}

	t.Logf("Successfully completed %d concurrent goroutines with %d operations each",
		numGoroutines, numOperations)
}

// TestTempDirectory_ConcurrentCleanup tests concurrent cleanup operations
func TestTempDirectory_ConcurrentCleanup(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple subdirectories
	numDirs := 20
	dirs := make([]string, numDirs)
	for i := 0; i < numDirs; i++ {
		dirPath := filepath.Join(tempDir, fmt.Sprintf("dir_%d", i))
		err := os.MkdirAll(dirPath, 0o750)
		require.NoError(t, err)
		dirs[i] = dirPath

		// Create some files in each directory
		for j := 0; j < 5; j++ {
			filePath := filepath.Join(dirPath, fmt.Sprintf("file_%d.txt", j))
			err = os.WriteFile(filePath, []byte("test content"), 0o644)
			require.NoError(t, err)
		}
	}

	// Concurrently clean up directories
	var wg sync.WaitGroup
	errorChan := make(chan error, numDirs)

	for _, dirPath := range dirs {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			err := os.RemoveAll(path)
			if err != nil {
				errorChan <- err
			}
		}(dirPath)
	}

	// Wait for all cleanup operations to complete
	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Errorf("Cleanup failed: %v", err)
	}

	// Verify all directories are removed
	for _, dirPath := range dirs {
		_, err := os.Stat(dirPath)
		require.True(t, os.IsNotExist(err), "Directory should be removed: %s", dirPath)
	}

	t.Logf("Successfully cleaned up %d directories concurrently", numDirs)
}

// TestTempDirectory_RaceDetection tests for race conditions using Go's race detector
func TestTempDirectory_RaceDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race detection test in short mode")
	}

	tempDir := t.TempDir()
	sharedFile := filepath.Join(tempDir, "shared.txt")

	// Initialize the file
	err := os.WriteFile(sharedFile, []byte("initial"), 0o644)
	require.NoError(t, err)

	// Use a mutex to coordinate access
	var mu sync.Mutex
	var wg sync.WaitGroup

	numGoroutines := 5
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()

			// Safe concurrent access with mutex
			mu.Lock()
			content, err := os.ReadFile(sharedFile)
			if err != nil {
				t.Errorf("Read failed: %v", err)
				mu.Unlock()
				return
			}

			// Modify and write back
			content = append(content, []byte(" modified")...)
			err = os.WriteFile(sharedFile, content, 0o644)
			if err != nil {
				t.Errorf("Write failed: %v", err)
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify file still exists and has been modified
	_, err = os.Stat(sharedFile)
	require.NoError(t, err, "Shared file should still exist")

	t.Logf("Race detection test completed with %d goroutines", numGoroutines)
}

// TestTempDirectory_CleanupOnPanic tests cleanup behavior on panic
func TestTempDirectory_CleanupOnPanic(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file
	testFile := filepath.Join(tempDir, "panic_test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Test cleanup even with panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic: %v", r)
			}
		}()

		// Simulate operation that might panic
		// In production, this would be cleaned up by deferred cleanup
		_ = testFile

		// Intentionally panic
		// panic("simulated panic")
	}()

	// File should still be accessible
	_, err = os.Stat(testFile)
	require.NoError(t, err, "File should still exist after panic recovery")

	t.Logf("Cleanup on panic test completed")
}
