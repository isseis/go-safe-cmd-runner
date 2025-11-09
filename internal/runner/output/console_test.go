package output

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConsoleOutputWriter(t *testing.T) {
	writer := NewConsoleOutputWriter()
	require.NotNil(t, writer)

	// Verify it implements the interface
	_ = writer
}

func TestConsoleOutputWriter_Write(t *testing.T) {
	tests := []struct {
		name   string
		stream executor.OutputStream
	}{
		{
			name:   "stdout stream",
			stream: executor.StdoutStream,
		},
		{
			name:   "stderr stream",
			stream: executor.StderrStream,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture original stdout/stderr
			originalStdout := os.Stdout
			originalStderr := os.Stderr
			defer func() {
				os.Stdout = originalStdout
				os.Stderr = originalStderr
			}()

			// Create pipes to capture output
			r, w, err := os.Pipe()
			require.NoError(t, err)
			defer r.Close()
			defer w.Close()

			// Redirect stdout or stderr
			if tt.stream == executor.StdoutStream {
				os.Stdout = w
			} else {
				os.Stderr = w
			}

			writer := NewConsoleOutputWriter()
			testData := []byte("test output")

			// Write data
			err = writer.Write(tt.stream, testData)
			assert.NoError(t, err)

			// Close writer side to flush
			w.Close()

			// Read captured output
			var buf bytes.Buffer
			_, err = io.Copy(&buf, r)
			require.NoError(t, err)

			assert.Equal(t, "test output", buf.String())
		})
	}
}

func TestConsoleOutputWriter_Write_Concurrent(t *testing.T) {
	// Capture original stdout/stderr
	originalStdout := os.Stdout
	originalStderr := os.Stderr
	defer func() {
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	}()

	// Create pipes to capture output
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	defer rOut.Close()
	defer wOut.Close()

	rErr, wErr, err := os.Pipe()
	require.NoError(t, err)
	defer rErr.Close()
	defer wErr.Close()

	os.Stdout = wOut
	os.Stderr = wErr

	writer := NewConsoleOutputWriter()

	// Test concurrent writes
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Alternate between stdout and stderr
			stream := executor.StdoutStream
			if id%2 == 1 {
				stream = executor.StderrStream
			}

			err := writer.Write(stream, []byte("test"))
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Close writers to flush
	wOut.Close()
	wErr.Close()

	// Verify no errors occurred (actual output content is not deterministic due to concurrency)
}

func TestConsoleOutputWriter_Close(t *testing.T) {
	writer := NewConsoleOutputWriter()

	err := writer.Close()
	assert.NoError(t, err)

	// Should be idempotent
	err = writer.Close()
	assert.NoError(t, err)
}
