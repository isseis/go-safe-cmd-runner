package output

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Manager operation errors
var (
	ErrOutputSizeLimitExceeded = errors.New("output size limit exceeded")
)

// Pre-compiled regular expressions for security risk evaluation
var (
	// Critical: Exact system critical files (case-insensitive)
	criticalFilesRegex = regexp.MustCompile(`^(?i)(/etc/passwd|/etc/shadow|/etc/sudoers)$`)

	// Critical: System critical directories (case-insensitive)
	criticalDirsRegex = regexp.MustCompile(`^(?i)(/boot/|/sys/|/proc/)`)

	// Critical: Sensitive key files (basename only, case-insensitive)
	criticalKeyFilesRegex = regexp.MustCompile(`^(?i)(authorized_keys|id_rsa|id_ed25519|id_ecdsa|id_dsa)$`)

	// High: System directories (case-insensitive)
	highDirsRegex = regexp.MustCompile(`^(?i)(/etc/|/var/log/|/usr/bin/|/usr/sbin/)`)

	// High: Sensitive config directories (case-insensitive, can be anywhere in path)
	highConfigDirsRegex = regexp.MustCompile(`(?i)(^|/)\.ssh/|(?i)(^|/)\.gnupg/`)
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

// ValidateOutputPath validates an output path without preparing capture
func (m *DefaultOutputCaptureManager) ValidateOutputPath(outputPath string, workDir string) error {
	if outputPath == "" {
		return nil // No output path to validate
	}

	// 1. Path validation and resolution
	resolvedPath, err := m.pathValidator.ValidateAndResolvePath(outputPath, workDir)
	if err != nil {
		return fmt.Errorf("path validation failed: %w", err)
	}

	// 2. Security permission check
	if err := m.securityValidator.ValidateOutputWritePermission(resolvedPath, os.Getuid()); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}

	return nil
}

// PrepareOutput validates paths and prepares for output capture using temporary file
func (m *DefaultOutputCaptureManager) PrepareOutput(outputPath string, workDir string, maxSize int64) (*Capture, error) {
	// 1. Path validation and resolution (reuse validation logic)
	if err := m.ValidateOutputPath(outputPath, workDir); err != nil {
		return nil, err
	}

	// Re-resolve the path since ValidateOutputPath only validates
	resolvedPath, err := m.pathValidator.ValidateAndResolvePath(outputPath, workDir)
	if err != nil {
		return nil, fmt.Errorf("path validation failed: %w", err)
	}

	// 3. Ensure directory exists
	dir := filepath.Dir(resolvedPath)
	if err := m.fileManager.EnsureDirectory(dir); err != nil {
		return nil, fmt.Errorf("failed to ensure directory: %w", err)
	}

	// 4. Create temporary file
	tempFile, tempPath, err := m.createTempFile(resolvedPath)
	if err != nil {
		return nil, err
	}

	// 5. Create capture session with temporary file
	capture := &Capture{
		OutputPath:   resolvedPath,
		TempFilePath: tempPath,
		FileHandle:   tempFile,
		MaxSize:      maxSize,
		CurrentSize:  0,
		StartTime:    time.Now(),
	}

	return capture, nil
}

// WriteOutput writes data to the temporary file with size limit checking
func (m *DefaultOutputCaptureManager) WriteOutput(capture *Capture, data []byte) error {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()

	// Check size limits
	newSize := capture.CurrentSize + int64(len(data))
	if capture.MaxSize > 0 && newSize > capture.MaxSize {
		return fmt.Errorf("%w: %d bytes (limit: %d)", ErrOutputSizeLimitExceeded, newSize, capture.MaxSize)
	}

	// Write to temporary file
	n, err := capture.FileHandle.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	capture.CurrentSize += int64(n)
	return nil
}

// FinalizeOutput closes the temporary file and moves it to the final location
func (m *DefaultOutputCaptureManager) FinalizeOutput(capture *Capture) error {
	// Close the temporary file before moving it to the final location
	if err := capture.FileHandle.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Move the temporary file to the final output location
	return m.moveToFinalLocation(capture.TempFilePath, capture.OutputPath)
}

// createTempFile creates a temporary file in the same directory as the output
func (m *DefaultOutputCaptureManager) createTempFile(outputPath string) (*os.File, string, error) {
	tempDir := filepath.Dir(outputPath)
	tempFile, err := m.fileManager.CreateTempFile(tempDir, "output_*.tmp")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	return tempFile, tempFile.Name(), nil
}

// moveToFinalLocation moves the temporary file to the final output location
func (m *DefaultOutputCaptureManager) moveToFinalLocation(tempPath, outputPath string) error {
	if err := m.fileManager.MoveToFinal(tempPath, outputPath); err != nil {
		return fmt.Errorf("failed to move temp file to final location: %w", err)
	}
	return nil
}

// cleanupTempFile handles cleanup of temporary files with proper error logging
// This function is called when an error occurs during processing
func (m *DefaultOutputCaptureManager) cleanupTempFile(tempFile *os.File, tempPath string) {
	if tempFile == nil {
		return
	}
	// Try to close file if it's still open
	if err := tempFile.Close(); err != nil {
		slog.Warn("failed to close temporary file during cleanup", "path", tempPath, "error", err)
	}

	// Remove temporary file if it still exists
	// Log removal errors as warnings since they don't affect the main operation
	if removeErr := m.fileManager.RemoveTemp(tempPath); removeErr != nil {
		slog.Warn("failed to remove temporary file", "path", tempPath, "error", removeErr)
	}
}

// CleanupOutput cleans up the temporary file and resets capture state
func (m *DefaultOutputCaptureManager) CleanupOutput(capture *Capture) error {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()

	// Close and remove temporary file if it exists
	if capture.FileHandle != nil {
		m.cleanupTempFile(capture.FileHandle, capture.TempFilePath)
		capture.FileHandle = nil
		capture.TempFilePath = ""
	}

	// Reset size
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

// isPathWithinDirectory checks if a path is within a given directory
// using proper path boundary checking to prevent false positives
func isPathWithinDirectory(targetPath, dirPath string) bool {
	if dirPath == "" {
		return false
	}

	cleanTarget := filepath.Clean(targetPath)
	cleanDir := filepath.Clean(dirPath)

	// Ensure directory path ends with separator for proper boundary checking
	if !strings.HasSuffix(cleanDir, string(filepath.Separator)) {
		cleanDir += string(filepath.Separator)
	}

	// Check if target starts with directory and is not equal to directory
	return strings.HasPrefix(cleanTarget+string(filepath.Separator), cleanDir) && cleanTarget != strings.TrimSuffix(cleanDir, string(filepath.Separator))
}

// evaluateSecurityRisk assesses the security risk of writing to the given path
func (m *DefaultOutputCaptureManager) evaluateSecurityRisk(path, workDir string) runnertypes.RiskLevel {
	cleanPath := filepath.Clean(path)
	baseName := filepath.Base(cleanPath)

	// Critical: Exact system critical files
	if criticalFilesRegex.MatchString(cleanPath) {
		return runnertypes.RiskLevelCritical
	}

	// Critical: System critical directories
	if criticalDirsRegex.MatchString(cleanPath) {
		return runnertypes.RiskLevelCritical
	}

	// Critical: Sensitive key files (check basename)
	if criticalKeyFilesRegex.MatchString(baseName) {
		return runnertypes.RiskLevelCritical
	}

	// High: System directories
	if highDirsRegex.MatchString(cleanPath) {
		return runnertypes.RiskLevelHigh
	}

	// High: Sensitive config directories
	if highConfigDirsRegex.MatchString(cleanPath) {
		return runnertypes.RiskLevelHigh
	}

	// Low: Work directory or user home
	if workDir != "" && isPathWithinDirectory(cleanPath, workDir) {
		return runnertypes.RiskLevelLow
	}

	// Check if in user's home directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		if isPathWithinDirectory(cleanPath, homeDir) {
			return runnertypes.RiskLevelLow
		}
	}

	// Medium: Everything else
	return runnertypes.RiskLevelMedium
}
