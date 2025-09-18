package output

import (
	"os"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Config represents configuration for output capture
type Config struct {
	Path    string // Output file path
	MaxSize int64  // Maximum output size in bytes
}

// Capture represents an active output capture session
type Capture struct {
	OutputPath  string    // Final output file path
	TempPath    string    // Temporary file path during capture
	TempFile    *os.File  // File handle to temporary file
	MaxSize     int64     // Maximum allowed output size
	CurrentSize int64     // Current accumulated output size
	StartTime   time.Time // Start time of capture session
}

// Analysis represents the analysis result for Dry-Run mode
type Analysis struct {
	OutputPath      string                // Configured output path
	ResolvedPath    string                // Resolved absolute path
	DirectoryExists bool                  // Whether target directory exists
	WritePermission bool                  // Whether we have write permission
	SecurityRisk    runnertypes.RiskLevel // Assessed security risk level
	MaxSizeLimit    int64                 // Effective size limit
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
