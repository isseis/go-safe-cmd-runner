package filevalidator

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// BenchmarkValidator_Verify benchmarks the standard Verify method
func BenchmarkValidator_Verify(b *testing.B) {
	tempDir := b.TempDir()
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		b.Fatalf("Failed to create validator: %v", err)
	}

	// Create test file
	testFile := createBenchmarkTestFile(b, "benchmark test content")

	// Record hash
	_, _, err = validator.Record(testFile, false)
	if err != nil {
		b.Fatalf("Failed to record hash: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := validator.Verify(testFile)
		if err != nil {
			b.Fatalf("Verify failed: %v", err)
		}
	}
}

// BenchmarkValidator_VerifyFromHandle benchmarks the new VerifyFromHandle method
func BenchmarkValidator_VerifyFromHandle(b *testing.B) {
	tempDir := b.TempDir()
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		b.Fatalf("Failed to create validator: %v", err)
	}

	// Create test file
	testFile := createBenchmarkTestFile(b, "benchmark test content")

	// Record hash
	_, _, err = validator.Record(testFile, false)
	if err != nil {
		b.Fatalf("Failed to record hash: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Open file manually (simulating the privilege-separated approach)
		file, err := openTestFile(testFile)
		if err != nil {
			b.Fatalf("Failed to open file: %v", err)
		}

		err = validator.VerifyFromHandle(file, common.ResolvedPath(testFile))
		file.Close()

		if err != nil {
			b.Fatalf("VerifyFromHandle failed: %v", err)
		}
	}
}

// Helper function for benchmarks
func openTestFile(path string) (*os.File, error) {
	return os.Open(path)
}

// createBenchmarkTestFile creates a temporary test file for benchmarks
func createBenchmarkTestFile(b *testing.B, content string) string {
	b.Helper()
	tmpFile, err := os.CreateTemp(b.TempDir(), "bench_file_*.txt")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}

	_, err = tmpFile.WriteString(content)
	if err != nil {
		b.Fatalf("Failed to write content: %v", err)
	}

	err = tmpFile.Close()
	if err != nil {
		b.Fatalf("Failed to close temp file: %v", err)
	}

	return tmpFile.Name()
}

// BenchmarkOpenFileWithPrivileges benchmarks the privilege file opening
func BenchmarkOpenFileWithPrivileges(b *testing.B) {
	testFile := createBenchmarkTestFile(b, "benchmark test content")
	pfv := NewPrivilegedFileValidator(nil) // Use default FileSystem

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Test normal file access (fast path)
		file, err := pfv.OpenFileWithPrivileges(testFile, nil)
		if err != nil {
			b.Fatalf("OpenFileWithPrivileges failed: %v", err)
		}
		file.Close()
	}
}
