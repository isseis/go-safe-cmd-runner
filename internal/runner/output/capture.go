package output

import (
	"os"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
)

// Capture represents an active output capture session using temporary file
type Capture struct {
	OutputPath   string     // Final output file path
	TempFilePath string     // Temporary file path
	FileHandle   *os.File   // File handle for temporary file
	MaxSize      int64      // Maximum allowed output size
	CurrentSize  int64      // Current accumulated output size
	StartTime    time.Time  // Start time of capture session
	mutex        sync.Mutex // Protects concurrent access to file and size
}

// Write implements executor.OutputWriter interface
// The stream parameter is ignored since Capture does not distinguish between stdout/stderr
func (c *Capture) Write(_ executor.OutputStream, data []byte) error {
	return c.WriteOutput(data)
}

// WriteOutput writes data to the capture (implements CaptureWriter interface)
func (c *Capture) WriteOutput(data []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check size limit
	if c.CurrentSize+int64(len(data)) > c.MaxSize {
		return &CaptureError{
			Type:  ErrorTypeSizeLimit,
			Path:  c.OutputPath,
			Phase: PhaseExecution,
			Cause: ErrOutputSizeExceeded,
		}
	}

	// Write to file
	if c.FileHandle != nil {
		n, err := c.FileHandle.Write(data)
		if err != nil {
			return &CaptureError{
				Type:  ErrorTypeFileSystem,
				Path:  c.OutputPath,
				Phase: PhaseExecution,
				Cause: err,
			}
		}
		c.CurrentSize += int64(n)
	}

	return nil
}

// Close closes the capture (implements both CaptureWriter and executor.OutputWriter interfaces)
// This method is idempotent and can be safely called multiple times.
func (c *Capture) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.FileHandle != nil {
		err := c.FileHandle.Close()
		c.FileHandle = nil // Set to nil after closing to make Close idempotent
		return err
	}
	return nil
}
