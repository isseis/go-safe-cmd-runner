//go:build test

package elfanalyzer

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkAnalyzeNetworkSymbols measures the performance of ELF analysis
func BenchmarkAnalyzeNetworkSymbols(b *testing.B) {
	// Find real system binaries for benchmarking
	testBinaries := []struct {
		name string
		path string
	}{
		{"ls", "/usr/bin/ls"},
		{"cat", "/usr/bin/cat"},
		{"curl", "/usr/bin/curl"},
	}

	for _, bin := range testBinaries {
		if _, err := os.Stat(bin.path); os.IsNotExist(err) {
			continue
		}

		absPath, err := filepath.Abs(bin.path)
		if err != nil {
			continue
		}

		b.Run(bin.name, func(b *testing.B) {
			analyzer := NewStandardELFAnalyzer(nil, nil)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = analyzer.AnalyzeNetworkSymbols(absPath, "")
			}
		})
	}
}

// BenchmarkAnalyzeNetworkSymbols_TestdataFixtures benchmarks against test fixtures
func BenchmarkAnalyzeNetworkSymbols_TestdataFixtures(b *testing.B) {
	testdataDir := "testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		b.Skip("testdata directory not found")
	}

	fixtures := []string{
		"with_socket.elf",
		"with_ssl.elf",
		"no_network.elf",
		"static.elf",
		"script.sh",
		"corrupted.elf",
	}

	for _, fixture := range fixtures {
		path := filepath.Join(testdataDir, fixture)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}

		b.Run(fixture, func(b *testing.B) {
			analyzer := NewStandardELFAnalyzer(nil, nil)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = analyzer.AnalyzeNetworkSymbols(absPath, "")
			}
		})
	}
}
