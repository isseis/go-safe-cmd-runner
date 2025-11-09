package output

import (
	"errors"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
)

// CaptureWriter defines the interface for writing to output capture
type CaptureWriter interface {
	// WriteOutput writes data to the capture
	WriteOutput(data []byte) error
	// Close closes the capture writer
	Close() error
}

// TeeOutputWriter implements executor.OutputWriter and writes to both
// a CaptureWriter (for file output) and another OutputWriter (for console output)
type TeeOutputWriter struct {
	capture CaptureWriter
	writer  executor.OutputWriter
}

// NewTeeOutputWriter creates a new TeeOutputWriter that writes to both
// the capture writer and the output writer
func NewTeeOutputWriter(capture CaptureWriter, writer executor.OutputWriter) executor.OutputWriter {
	return &TeeOutputWriter{
		capture: capture,
		writer:  writer,
	}
}

// Write implements executor.OutputWriter.Write
// It writes data to both the capture writer and the output writer
func (t *TeeOutputWriter) Write(stream executor.OutputStream, data []byte) error {
	// Write to capture first (file output)
	if t.capture != nil {
		if err := t.capture.WriteOutput(data); err != nil {
			return err
		}
	}

	// Write to output writer (console output)
	if t.writer != nil {
		if err := t.writer.Write(stream, data); err != nil {
			return err
		}
	}

	return nil
}

// Close implements executor.OutputWriter.Close
// It closes both the capture writer and the output writer
func (t *TeeOutputWriter) Close() error {
	var errs []error

	// Close capture first
	if t.capture != nil {
		if err := t.capture.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	// Close output writer
	if t.writer != nil {
		if err := t.writer.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
