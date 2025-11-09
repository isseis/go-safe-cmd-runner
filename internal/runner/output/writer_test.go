package output

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test errors
var (
	errMockCapture      = errors.New("capture error")
	errMockWriter       = errors.New("writer error")
	errMockCaptureClose = errors.New("capture close error")
	errMockWriterClose  = errors.New("writer close error")
)

// Mock OutputWriter for testing
type mockOutputWriter struct {
	writtenData [][]byte
	writeError  error
	closeError  error
	closed      bool
}

func (m *mockOutputWriter) Write(_ executor.OutputStream, data []byte) error {
	if m.writeError != nil {
		return m.writeError
	}
	m.writtenData = append(m.writtenData, data)
	return nil
}

func (m *mockOutputWriter) Close() error {
	m.closed = true
	return m.closeError
}

func TestTeeOutputWriter_Write(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		stream        executor.OutputStream
		bufferWritten bool
		captureError  error
		writerError   error
		wantError     bool
	}{
		{
			name:          "successful write to both",
			data:          []byte("test output"),
			stream:        executor.StdoutStream,
			bufferWritten: true,
			captureError:  nil,
			writerError:   nil,
			wantError:     false,
		},
		{
			name:          "capture error",
			data:          []byte("test output"),
			stream:        executor.StdoutStream,
			bufferWritten: false,
			captureError:  errMockCapture,
			writerError:   nil,
			wantError:     true,
		},
		{
			name:          "writer error",
			data:          []byte("test output"),
			stream:        executor.StdoutStream,
			bufferWritten: true,
			captureError:  nil,
			writerError:   errMockWriter,
			wantError:     true,
		},
		{
			name:          "empty data",
			data:          []byte{},
			stream:        executor.StdoutStream,
			bufferWritten: true,
			captureError:  nil,
			writerError:   nil,
			wantError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock capture
			mockCapture := &mockCapture{
				writeError: tt.captureError,
			}

			// Create mock writer
			mockWriter := &mockOutputWriter{
				writeError: tt.writerError,
			}

			// Create TeeOutputWriter
			teeWriter := NewTeeOutputWriter(mockCapture, mockWriter)

			// Execute Write
			err := teeWriter.Write(tt.stream, tt.data)

			// Check error
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check if data was written to capture
			if tt.bufferWritten && tt.captureError == nil {
				assert.Equal(t, 1, len(mockCapture.writeCalls))
				assert.Equal(t, tt.data, mockCapture.writeCalls[0])
			}

			// Check if data was written to writer
			// Writer is only called if capture succeeds (or capture is nil)
			if tt.writerError == nil && tt.captureError == nil {
				assert.Equal(t, 1, len(mockWriter.writtenData))
				assert.Equal(t, tt.data, mockWriter.writtenData[0])
			} else if tt.captureError != nil {
				// If capture failed, writer should not be called
				assert.Equal(t, 0, len(mockWriter.writtenData))
			}
		})
	}
}

func TestTeeOutputWriter_Close(t *testing.T) {
	tests := []struct {
		name         string
		captureError error
		writerError  error
		wantError    bool
	}{
		{
			name:         "successful close",
			captureError: nil,
			writerError:  nil,
			wantError:    false,
		},
		{
			name:         "capture close error",
			captureError: errMockCaptureClose,
			writerError:  nil,
			wantError:    true,
		},
		{
			name:         "writer close error",
			captureError: nil,
			writerError:  errMockWriterClose,
			wantError:    true,
		},
		{
			name:         "both close errors",
			captureError: errMockCaptureClose,
			writerError:  errMockWriterClose,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock capture
			mockCapture := &mockCapture{
				closeError: tt.captureError,
			}

			// Create mock writer
			mockWriter := &mockOutputWriter{
				closeError: tt.writerError,
			}

			// Create TeeOutputWriter
			teeWriter := NewTeeOutputWriter(mockCapture, mockWriter)

			// Execute Close
			err := teeWriter.Close()

			// Check error
			if tt.wantError {
				assert.Error(t, err)

				// Special handling for both close errors test case
				if tt.name == "both close errors" {
					// Verify that errors.Join properly combines both errors
					assert.Contains(t, err.Error(), errMockCaptureClose.Error())
					assert.Contains(t, err.Error(), errMockWriterClose.Error())

					// Verify that errors.Is works correctly with joined errors
					assert.True(t, errors.Is(err, errMockCaptureClose))
					assert.True(t, errors.Is(err, errMockWriterClose))
				}
			} else {
				assert.NoError(t, err)
			}

			// Check if capture was closed
			assert.True(t, mockCapture.closed)

			// Check if writer was closed
			assert.True(t, mockWriter.closed)
		})
	}
}

func TestTeeOutputWriter_NilCapture(t *testing.T) {
	// Create mock writer
	mockWriter := &mockOutputWriter{}

	// Create TeeOutputWriter with nil capture
	teeWriter := NewTeeOutputWriter(nil, mockWriter)

	// Write should only write to writer
	data := []byte("test output")
	err := teeWriter.Write(executor.StdoutStream, data)
	require.NoError(t, err)

	// Check that data was written only to writer
	assert.Equal(t, 1, len(mockWriter.writtenData))
	assert.Equal(t, data, mockWriter.writtenData[0])

	// Close should only close writer
	err = teeWriter.Close()
	require.NoError(t, err)
	assert.True(t, mockWriter.closed)
}

func TestTeeOutputWriter_NilWriter(t *testing.T) {
	// Create mock capture
	mockCapture := &mockCapture{}

	// Create TeeOutputWriter with nil writer
	teeWriter := NewTeeOutputWriter(mockCapture, nil)

	// Write should only write to capture
	data := []byte("test output")
	err := teeWriter.Write(executor.StdoutStream, data)
	require.NoError(t, err)

	// Check that data was written only to capture
	assert.Equal(t, 1, len(mockCapture.writeCalls))
	assert.Equal(t, data, mockCapture.writeCalls[0])

	// Close should only close capture
	err = teeWriter.Close()
	require.NoError(t, err)
	assert.True(t, mockCapture.closed)
}

// Mock capture for testing
type mockCapture struct {
	writeCalls [][]byte
	writeError error
	closed     bool
	closeError error
}

func (m *mockCapture) WriteOutput(data []byte) error {
	if m.writeError != nil {
		return m.writeError
	}
	m.writeCalls = append(m.writeCalls, data)
	return nil
}

func (m *mockCapture) Close() error {
	m.closed = true
	return m.closeError
}
