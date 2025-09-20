package output

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Manager operation errors
var (
	ErrOutputSizeLimitExceeded = errors.New("output size limit exceeded")
)

// SecurityValidator defines the interface for security validation
type SecurityValidator interface {
	ValidateOutputWritePermission(outputPath string, realUID int) error
}

// DefaultOutputCaptureManager implements CaptureManager interface
type DefaultOutputCaptureManager struct {
	pathValidator     PathValidator
	fileManager       FileManager
	securityValidator SecurityValidator
}

// NewDefaultOutputCaptureManager creates a new DefaultOutputCaptureManager
func NewDefaultOutputCaptureManager(securityValidator SecurityValidator) *DefaultOutputCaptureManager {
	return &DefaultOutputCaptureManager{
		pathValidator:     NewDefaultPathValidator(),
		fileManager:       NewSafeFileManager(),
		securityValidator: securityValidator,
	}
}

// PrepareOutput validates paths and prepares for output capture using memory buffer
func (m *DefaultOutputCaptureManager) PrepareOutput(outputPath string, workDir string, maxSize int64) (*Capture, error) {
	// 1. Path validation and resolution
	resolvedPath, err := m.pathValidator.ValidateAndResolvePath(outputPath, workDir)
	if err != nil {
		return nil, fmt.Errorf("path validation failed: %w", err)
	}

	// 2. Security permission check
	if err := m.securityValidator.ValidateOutputWritePermission(resolvedPath, os.Getuid()); err != nil {
		return nil, fmt.Errorf("security validation failed: %w", err)
	}

	// 3. Ensure directory exists
	dir := filepath.Dir(resolvedPath)
	if err := m.fileManager.EnsureDirectory(dir); err != nil {
		return nil, fmt.Errorf("failed to ensure directory: %w", err)
	}

	// 4. Create capture session with memory buffer
	capture := &Capture{
		OutputPath:  resolvedPath,
		Buffer:      &bytes.Buffer{},
		MaxSize:     maxSize,
		CurrentSize: 0,
		StartTime:   time.Now(),
	}

	return capture, nil
}

// WriteOutput writes data to the memory buffer with size limit checking
func (m *DefaultOutputCaptureManager) WriteOutput(capture *Capture, data []byte) error {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()

	// Check size limits
	newSize := capture.CurrentSize + int64(len(data))
	if capture.MaxSize > 0 && newSize > capture.MaxSize {
		return fmt.Errorf("%w: %d bytes (limit: %d)", ErrOutputSizeLimitExceeded, newSize, capture.MaxSize)
	}

	// Write to memory buffer
	n, err := capture.Buffer.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to buffer: %w", err)
	}

	capture.CurrentSize += int64(n)
	return nil
}

// FinalizeOutput writes the buffer content to the final file location
func (m *DefaultOutputCaptureManager) FinalizeOutput(capture *Capture) error {
	// Create a temporary file with the buffer content
	tempDir := filepath.Dir(capture.OutputPath)
	tempFile, err := m.fileManager.CreateTempFile(tempDir, "output_*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	tempPath := tempFile.Name()
	defer func() {
		if closeErr := tempFile.Close(); closeErr != nil {
			// Log the error but don't override the main error
			fmt.Printf("Warning: failed to close temporary file %s: %v\n", tempPath, closeErr)
		}
		// Clean up temp file if it still exists (in case of error)
		if removeErr := m.fileManager.RemoveTemp(tempPath); removeErr != nil {
			// Log the error but don't override the main error
			fmt.Printf("Warning: failed to remove temporary file %s: %v\n", tempPath, removeErr)
		}
	}()

	// Write buffer content to temp file
	bufferContent := capture.Buffer.Bytes()
	if _, err := m.fileManager.WriteToTemp(tempFile, bufferContent); err != nil {
		return fmt.Errorf("failed to write buffer to temp file: %w", err)
	}

	// Close temp file before moving
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Move temp file to final location
	if err := m.fileManager.MoveToFinal(tempPath, capture.OutputPath); err != nil {
		return fmt.Errorf("failed to move temp file to final location: %w", err)
	}

	return nil
}

// CleanupOutput cleans up the memory buffer
func (m *DefaultOutputCaptureManager) CleanupOutput(capture *Capture) error {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()

	// Reset buffer and size
	capture.Buffer.Reset()
	capture.CurrentSize = 0

	return nil
}

// AnalyzeOutput performs dry-run analysis of output configuration
func (m *DefaultOutputCaptureManager) AnalyzeOutput(outputPath string, workDir string) (*Analysis, error) {
	analysis := &Analysis{
		OutputPath:      outputPath,
		DirectoryExists: false,
		WritePermission: false,
		SecurityRisk:    RiskLevelCritical, // Default to critical until proven otherwise
	}

	// 1. Path validation and resolution
	resolvedPath, err := m.pathValidator.ValidateAndResolvePath(outputPath, workDir)
	if err != nil {
		analysis.ErrorMessage = fmt.Sprintf("Path validation failed: %v", err)
		return analysis, nil
	}
	analysis.ResolvedPath = resolvedPath

	// 2. Check directory existence
	dir := filepath.Dir(resolvedPath)
	if stat, err := os.Lstat(dir); err == nil && stat.IsDir() && stat.Mode()&os.ModeSymlink == 0 {
		analysis.DirectoryExists = true
	}

	// 3. Permission validation
	if err := m.securityValidator.ValidateOutputWritePermission(resolvedPath, os.Getuid()); err != nil {
		if analysis.ErrorMessage == "" {
			analysis.ErrorMessage = fmt.Sprintf("Permission check failed: %v", err)
		}
	} else {
		analysis.WritePermission = true
	}

	// 4. Security risk evaluation
	analysis.SecurityRisk = m.evaluateSecurityRisk(resolvedPath, workDir)

	return analysis, nil
}

// evaluateSecurityRisk assesses the security risk of writing to the given path
func (m *DefaultOutputCaptureManager) evaluateSecurityRisk(path, workDir string) runnertypes.RiskLevel {
	pathLower := strings.ToLower(path)

	// Critical: System critical files
	criticalPatterns := []string{
		"/etc/passwd", "/etc/shadow", "/etc/sudoers",
		"/boot/", "/sys/", "/proc/",
		"authorized_keys", "id_rsa", "id_ed25519",
	}

	for _, pattern := range criticalPatterns {
		if strings.Contains(pathLower, pattern) {
			return runnertypes.RiskLevelCritical
		}
	}

	// High: System directories
	highPatterns := []string{
		"/etc/", "/var/log/", "/usr/bin/", "/usr/sbin/",
		".ssh/", ".gnupg/",
	}

	for _, pattern := range highPatterns {
		if strings.Contains(pathLower, pattern) {
			return runnertypes.RiskLevelHigh
		}
	}

	// Low: Work directory or user home
	if workDir != "" {
		cleanWorkDir := filepath.Clean(workDir)
		cleanPath := filepath.Clean(path)
		if strings.HasPrefix(cleanPath, cleanWorkDir) {
			return runnertypes.RiskLevelLow
		}
	}

	// Check if in user's home directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		cleanHomePath := filepath.Clean(homeDir)
		cleanPath := filepath.Clean(path)
		if strings.HasPrefix(cleanPath, cleanHomePath) {
			return runnertypes.RiskLevelLow
		}
	}

	// Medium: Everything else
	return runnertypes.RiskLevelMedium
}
