package output

import (
	"os"
)

// CaptureManager manages the complete lifecycle of output capture
type CaptureManager interface {
	// PrepareOutput validates paths and prepares for output capture
	PrepareOutput(outputPath string, workDir string, maxSize int64) (*Capture, error)

	// ValidateOutputPath validates an output path without preparing capture
	// This allows early validation before command execution
	ValidateOutputPath(outputPath string, workDir string) error

	// WriteOutput writes data to the output capture session
	WriteOutput(capture *Capture, data []byte) error

	// FinalizeOutput completes the capture session and moves temp file to final location
	FinalizeOutput(capture *Capture) error

	// CleanupOutput cleans up resources in case of error
	CleanupOutput(capture *Capture) error

	// AnalyzeOutput performs dry-run analysis of output configuration
	AnalyzeOutput(outputPath string, workDir string) (*Analysis, error)
}

// PathValidator validates and resolves file paths for security
type PathValidator interface {
	// ValidateAndResolvePath validates and resolves an output path
	// This performs basic path validation and path traversal prevention
	ValidateAndResolvePath(outputPath, workDir string) (string, error)
}

// FileManager handles safe file system operations using safefileio
type FileManager interface {
	// CreateTempFile creates a temporary file for output capture
	CreateTempFile(dir string, pattern string) (*os.File, error)

	// WriteToTemp writes data to temporary file
	WriteToTemp(file *os.File, data []byte) (int, error)

	// MoveToFinal atomically moves temp file to final location
	MoveToFinal(tempPath, finalPath string) error

	// EnsureDirectory ensures directory exists with proper permissions
	EnsureDirectory(path string) error

	// RemoveTemp removes temporary file
	RemoveTemp(path string) error
}
