package output

import (
	"os"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Config represents configuration for output capture
type Config struct {
	Path    string // Output file path
	MaxSize int64  // Maximum output size in bytes
}

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

// Analysis represents the analysis result for Dry-Run mode
type Analysis struct {
	OutputPath      string                // Configured output path
	ResolvedPath    string                // Resolved absolute path
	DirectoryExists bool                  // Whether target directory exists
	WritePermission bool                  // Whether we have write permission
	SecurityRisk    runnertypes.RiskLevel // Assessed security risk level
	MaxSizeLimit    int64                 // Effective size limit
	ErrorMessage    string                // Error message if any issues found
}

// RiskLevel is an alias to the existing runnertypes.RiskLevel for convenience
type RiskLevel = runnertypes.RiskLevel

// Risk level constants for convenience
const (
	RiskLevelUnknown  = runnertypes.RiskLevelUnknown
	RiskLevelLow      = runnertypes.RiskLevelLow
	RiskLevelMedium   = runnertypes.RiskLevelMedium
	RiskLevelHigh     = runnertypes.RiskLevelHigh
	RiskLevelCritical = runnertypes.RiskLevelCritical
)

// Output size constants
const (
	// DefaultMaxOutputSize is the default maximum output size (10MB)
	DefaultMaxOutputSize = 10 * 1024 * 1024
	// AbsoluteMaxOutputSize is the absolute maximum output size (100MB)
	AbsoluteMaxOutputSize = 100 * 1024 * 1024
)
