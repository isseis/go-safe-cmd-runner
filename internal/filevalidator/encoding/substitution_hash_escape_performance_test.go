//go:build performance
// +build performance

package encoding

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testApplicationPath = "/usr/local/bin/application/module/file.txt"

func TestSubstitutionHashEscape_PerformanceRegression(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	// Baseline performance expectations (adjusted based on actual measurements)
	benchmarks := []struct {
		name                string
		operation           func() error
		maxNsPerOp          int64 // Maximum nanoseconds per operation
		maxAllocsBytesPerOp int64 // Maximum allocated bytes per operation
	}{
		{
			name: "encode_simple_path",
			operation: func() error {
				_, err := encoder.Encode("/usr/bin/python3")
				return err
			},
			maxNsPerOp:          5000, // 5μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 200,  // 200 bytes
		},
		{
			name: "encode_application_path",
			operation: func() error {
				_, err := encoder.Encode(testApplicationPath)
				return err
			},
			maxNsPerOp:          8000, // 8μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 400,  // 400 bytes
		},
		{
			name: "encode_special_chars",
			operation: func() error {
				_, err := encoder.Encode("/path/with#many~special#chars/file")
				return err
			},
			maxNsPerOp:          6000, // 6μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 300,  // 300 bytes
		},
		{
			name: "decode_normal",
			operation: func() error {
				_, err := encoder.Decode("~usr~bin~python3")
				return err
			},
			maxNsPerOp:          4000, // 4μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 150,  // 150 bytes
		},
		{
			name: "decode_complex",
			operation: func() error {
				_, err := encoder.Decode("~path~with#1many##special#1chars~file")
				return err
			},
			maxNsPerOp:          5000, // 5μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 200,  // 200 bytes
		},
	}

	for _, bench := range benchmarks {
		t.Run(bench.name, func(t *testing.T) {
			// Warmup
			for range 100 {
				_ = bench.operation()
			}

			// Measure performance
			iterations := 1000
			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			start := time.Now()
			for range iterations {
				err := bench.operation()
				require.NoError(t, err)
			}
			duration := time.Since(start)

			runtime.ReadMemStats(&m2)
			allocatedBytes := m2.TotalAlloc - m1.TotalAlloc

			nsPerOp := duration.Nanoseconds() / int64(iterations)
			bytesPerOp := int64(allocatedBytes) / int64(iterations)

			t.Logf("Performance regression test: %s", bench.name)
			t.Logf("  Nanoseconds per operation: %d (max: %d)", nsPerOp, bench.maxNsPerOp)
			t.Logf("  Bytes allocated per operation: %d (max: %d)", bytesPerOp, bench.maxAllocsBytesPerOp)

			// Check against regression thresholds
			if nsPerOp > bench.maxNsPerOp {
				t.Errorf("Performance regression detected: %d ns/op > %d ns/op", nsPerOp, bench.maxNsPerOp)
			}

			if bytesPerOp > bench.maxAllocsBytesPerOp {
				t.Errorf("Memory regression detected: %d bytes/op > %d bytes/op", bytesPerOp, bench.maxAllocsBytesPerOp)
			}
		})
	}
}

// BenchmarkSubstitutionHashEscape_Encode benchmarks the Encode method.
func BenchmarkSubstitutionHashEscape_Encode(b *testing.B) {
	encoder := NewSubstitutionHashEscape()

	benchmarks := []struct {
		name string
		path string
	}{
		{"simple_path", "/usr/bin/python3"},
		{"application_path", testApplicationPath},
		{"special_chars", "/path/with#hash~tilde/chars"},
		{"long_path", "/very/long/path/with/many/components/and/directories/file.extension"},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := encoder.Encode(bench.path)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSubstitutionHashEscape_Decode benchmarks the Decode method.
func BenchmarkSubstitutionHashEscape_Decode(b *testing.B) {
	encoder := NewSubstitutionHashEscape()

	// Pre-encode test paths
	testPaths := []string{
		"/usr/bin/python3",
		testApplicationPath,
		"/path/with#hash~tilde/chars",
		"/very/long/path/with/many/components/and/directories/file.extension",
	}

	encodedPaths := make([]string, len(testPaths))
	for i, path := range testPaths {
		encoded, err := encoder.Encode(path)
		if err != nil {
			b.Fatal(err)
		}
		encodedPaths[i] = encoded
	}

	benchmarks := []struct {
		name    string
		encoded string
	}{
		{"simple_path", encodedPaths[0]},
		{"application_path", encodedPaths[1]},
		{"special_chars", encodedPaths[2]},
		{"long_path", encodedPaths[3]},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := encoder.Decode(bench.encoded)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSubstitutionHashEscape_RoundTrip benchmarks encode + decode operations.
func BenchmarkSubstitutionHashEscape_RoundTrip(b *testing.B) {
	encoder := NewSubstitutionHashEscape()

	benchmarks := []struct {
		name string
		path string
	}{
		{"simple_path", "/usr/bin/python3"},
		{"application_path", testApplicationPath},
		{"special_chars", "/path/with#hash~tilde/chars"},
		{"long_path", "/very/long/path/with/many/components/and/directories/file.extension"},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				encoded, err := encoder.Encode(bench.path)
				if err != nil {
					b.Fatal(err)
				}
				decoded, err := encoder.Decode(encoded)
				if err != nil {
					b.Fatal(err)
				}
				if decoded != bench.path {
					b.Fatalf("Round trip failed: expected %s, got %s", bench.path, decoded)
				}
			}
		})
	}
}
