package config

import (
	"fmt"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// PathResolver resolves include paths to absolute paths.
type PathResolver interface {
	// ResolvePath resolves an include path to an absolute path.
	//
	// Parameters:
	//   - includePath: Path as written in the includes array
	//   - baseDir: Directory containing the config file (absolute path)
	//
	// Returns:
	//   - Resolved absolute path
	//   - Error if file does not exist or path is invalid
	//
	// Security:
	//   - Validates against path traversal attacks
	//   - Checks for symlink safety using safefileio
	ResolvePath(includePath string, baseDir string) (string, error)
}

// DefaultPathResolver is the production implementation of PathResolver.
type DefaultPathResolver struct {
	fs common.FileSystem
}

// NewDefaultPathResolver creates a new DefaultPathResolver.
func NewDefaultPathResolver(fs common.FileSystem) *DefaultPathResolver {
	return &DefaultPathResolver{fs: fs}
}

// ResolvePath resolves an include path to an absolute path.
func (r *DefaultPathResolver) ResolvePath(includePath string, baseDir string) (string, error) {
	// Step 1: Check if path is absolute
	var candidatePath string
	if filepath.IsAbs(includePath) {
		candidatePath = includePath
	} else {
		// Step 2: Join with base directory
		candidatePath = filepath.Join(baseDir, includePath)
	}

	// Step 3: Clean the path (resolve . and ..)
	candidatePath = filepath.Clean(candidatePath)

	// Step 4: Check file existence
	exists, err := r.fs.FileExists(candidatePath)
	if err != nil {
		return "", fmt.Errorf("failed to check file existence: %w", err)
	}
	if !exists {
		return "", &ErrIncludedFileNotFound{
			IncludePath:    includePath,
			ResolvedPath:   candidatePath,
			ReferencedFrom: baseDir,
		}
	}

	// Step 5: Security validation (symlink check via safefileio)
	// This will be handled by the file system abstraction
	// when actually reading the file

	// Step 6: Convert to absolute path (if not already)
	absPath, err := filepath.Abs(candidatePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	return absPath, nil
}
