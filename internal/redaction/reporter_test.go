//go:build test

package redaction

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShutdownReporter_NoFailures(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)
	var buf bytes.Buffer
	reporter := NewShutdownReporter(collector, &buf, nil)

	err := reporter.Report()
	require.NoError(t, err)

	// Should not write anything if no failures
	assert.Empty(t, buf.String())
}

func TestShutdownReporter_WithFailures(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)

	// Record some failures
	collector.RecordFailure("key1", errors.New("error1"))
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	collector.RecordFailure("key2", errors.New("error2"))
	time.Sleep(10 * time.Millisecond)
	collector.RecordFailure("key1", errors.New("error1")) // Same key again

	var buf bytes.Buffer
	reporter := NewShutdownReporter(collector, &buf, nil)

	err := reporter.Report()
	require.NoError(t, err)

	output := buf.String()

	// Check for expected content
	assert.Contains(t, output, "REDACTION FAILURES DETECTED")
	assert.Contains(t, output, "Total failures: 3")
	assert.Contains(t, output, "Affected attributes: 2")
	assert.Contains(t, output, "Attribute: key1")
	assert.Contains(t, output, "Attribute: key2")
	assert.Contains(t, output, "Count: 2") // key1 appears twice
	assert.Contains(t, output, "Count: 1") // key2 appears once
	assert.Contains(t, output, "error1")
	assert.Contains(t, output, "error2")
}

func TestShutdownReporter_WithLogger(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)

	// Record a failure
	collector.RecordFailure("test_key", errors.New("test error"))

	var buf bytes.Buffer
	var logBuf bytes.Buffer

	// Create a logger that writes to a buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	reporter := NewShutdownReporter(collector, &buf, logger)

	err := reporter.Report()
	require.NoError(t, err)

	// Check that both output and log were written
	assert.NotEmpty(t, buf.String())
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "Redaction failures summary")
	assert.Contains(t, logOutput, "total_failures=1")
}

func TestShutdownReporter_GroupsByKey(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)

	// Record multiple failures for the same key
	for i := 0; i < 5; i++ {
		collector.RecordFailure("repeated_key", errors.New("same error"))
		time.Sleep(5 * time.Millisecond)
	}

	var buf bytes.Buffer
	reporter := NewShutdownReporter(collector, &buf, nil)

	err := reporter.Report()
	require.NoError(t, err)

	output := buf.String()

	// Should group all failures under the same key
	assert.Contains(t, output, "Total failures: 5")
	assert.Contains(t, output, "Affected attributes: 1")
	assert.Contains(t, output, "Attribute: repeated_key")
	assert.Contains(t, output, "Count: 5")

	// Should show first and last occurrence
	assert.Contains(t, output, "First occurrence:")
	assert.Contains(t, output, "Last occurrence:")
}

func TestShutdownReporter_SortsKeysByName(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)

	// Record failures in non-alphabetical order
	collector.RecordFailure("zebra", errors.New("error"))
	collector.RecordFailure("apple", errors.New("error"))
	collector.RecordFailure("banana", errors.New("error"))

	var buf bytes.Buffer
	reporter := NewShutdownReporter(collector, &buf, nil)

	err := reporter.Report()
	require.NoError(t, err)

	output := buf.String()

	// Keys should appear in alphabetical order
	applePos := strings.Index(output, "Attribute: apple")
	bananaPos := strings.Index(output, "Attribute: banana")
	zebraPos := strings.Index(output, "Attribute: zebra")

	require.NotEqual(t, -1, applePos)
	require.NotEqual(t, -1, bananaPos)
	require.NotEqual(t, -1, zebraPos)

	assert.Less(t, applePos, bananaPos, "apple should come before banana")
	assert.Less(t, bananaPos, zebraPos, "banana should come before zebra")
}

func TestShutdownReporter_FormattingConsistency(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)

	collector.RecordFailure("test_attr", errors.New("test error message"))

	var buf bytes.Buffer
	reporter := NewShutdownReporter(collector, &buf, nil)

	err := reporter.Report()
	require.NoError(t, err)

	output := buf.String()

	// Check for expected formatting elements
	assert.Contains(t, output, "REDACTION FAILURES DETECTED")
	assert.Contains(t, output, "Details:")
	assert.Contains(t, output, "Note:")
}

func TestShutdownReporter_NilWriter(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)
	collector.RecordFailure("test", errors.New("error"))

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	// Create reporter with nil writer
	reporter := NewShutdownReporter(collector, nil, logger)

	err := reporter.Report()
	require.NoError(t, err)

	// Should still log to logger
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "Redaction failures summary")
}

func TestShutdownReporter_NilLogger(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)
	collector.RecordFailure("test", errors.New("error"))

	var buf bytes.Buffer

	// Create reporter with nil logger
	reporter := NewShutdownReporter(collector, &buf, nil)

	err := reporter.Report()
	require.NoError(t, err)

	// Should still write to writer
	assert.NotEmpty(t, buf.String())
	assert.Contains(t, buf.String(), "REDACTION FAILURES DETECTED")
}

func TestShutdownReporter_NonMemoryCollector(t *testing.T) {
	// Create a mock collector that doesn't implement the full interface
	mockCollector := &mockErrorCollector{}

	var buf bytes.Buffer
	reporter := NewShutdownReporter(mockCollector, &buf, nil)

	err := reporter.Report()
	require.NoError(t, err)

	// Should not write anything for non-memory collectors
	assert.Empty(t, buf.String())
}

// mockErrorCollector is a minimal implementation for testing
type mockErrorCollector struct{}

func (m *mockErrorCollector) RecordFailure(_ string, _ error) {
	// No-op
}
