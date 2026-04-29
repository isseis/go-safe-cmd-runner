package filevalidator

import (
	"os"
	"testing"
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

	// Save record
	_, _, err = validator.SaveRecord(testFile, false)
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
