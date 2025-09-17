//go:build performance
// +build performance

package encoding

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
			name: "encode_with_fallback_normal",
			operation: func() error {
				_, err := encoder.EncodeWithFallback(testApplicationPath)
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
