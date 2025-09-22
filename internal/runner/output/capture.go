package output

import (
	"os"
	"sync"
	"time"
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

// Close closes the capture (implements CaptureWriter interface)
func (c *Capture) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.FileHandle != nil {
		return c.FileHandle.Close()
	}
	return nil
}
