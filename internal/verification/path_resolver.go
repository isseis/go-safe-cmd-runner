package verification

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// PathResolver provides secure path resolution with caching
type PathResolver struct {
	pathEnv           string
	security          *security.Validator
	cache             map[string]string
	mu                sync.RWMutex
	skipStandardPaths bool
	standardPaths     []string
}

// NewPathResolver creates a new PathResolver with the specified configuration
func NewPathResolver(pathEnv string, security *security.Validator, skipStandardPaths bool) *PathResolver {
	return &PathResolver{
		pathEnv:           pathEnv,
		security:          security,
		cache:             make(map[string]string),
		skipStandardPaths: skipStandardPaths,
		standardPaths:     []string{"/bin/", "/sbin/", "/usr/bin/", "/usr/sbin/"},
	}
}

// canAccessDirectory checks if a directory can be accessed
func (pr *PathResolver) canAccessDirectory(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false // Directory doesn't exist or can't be accessed
	}

	return info.IsDir() // Just check if it's a directory
}

// ShouldSkipVerification checks if a path should be skipped based on configuration
func (pr *PathResolver) ShouldSkipVerification(path string) bool {
	if !pr.skipStandardPaths {
		return false
	}

	for _, standardPath := range pr.standardPaths {
		if strings.HasPrefix(path, standardPath) {
			return true
		}
	}
	return false
}

// validateCommandSafety validates command safety using the security validator
func (pr *PathResolver) validateCommandSafety(command string) error {
	if pr.security == nil {
		return nil // No security validation if validator is not available
	}

	// Use security validator to check for path traversal and other attacks
	return pr.security.ValidateCommand(command)
}

// validateAndCacheCommand validates that a path points to an executable file and caches it
func (pr *PathResolver) validateAndCacheCommand(path, cacheKey string) (string, error) {
	if info, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("%w: %s", ErrCommandNotFound, cacheKey)
	} else if info.IsDir() {
		return "", fmt.Errorf("%w: %s is a directory", ErrCommandNotFound, cacheKey)
	}

	pr.mu.Lock()
	pr.cache[cacheKey] = path
	pr.mu.Unlock()
	return path, nil
}

// ResolvePath resolves a command to its full path without security validation
func (pr *PathResolver) ResolvePath(command string) (string, error) {
	// Check cache first
	pr.mu.RLock()
	if cached, exists := pr.cache[command]; exists {
		pr.mu.RUnlock()
		return cached, nil
	}
	pr.mu.RUnlock()

	var resolvedPath string
	var err error

	// If absolute path, verify it exists and is not a directory
	if filepath.IsAbs(command) {
		resolvedPath, err = pr.validateAndCacheCommand(command, command)
		if err != nil {
			return "", err
		}
		return resolvedPath, nil
	}

	// Resolve from PATH environment variable
	var lastErr error
	for _, dir := range strings.Split(pr.pathEnv, string(os.PathListSeparator)) {
		// Check if directory can be accessed
		if !pr.canAccessDirectory(dir) {
			continue // Skip inaccessible directories
		}

		// Try to validate the command at this path
		fullPath := filepath.Join(dir, command)
		resolved, err := pr.validateAndCacheCommand(fullPath, command)
		if err == nil {
			// Found a valid executable
			return resolved, nil
		}
		// Save the last error in case we don't find any valid command
		lastErr = err
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("%w: %s", ErrCommandNotFound, command)
}

// ValidateCommand performs security validation on a resolved command path
func (pr *PathResolver) ValidateCommand(resolvedPath string) error {
	return pr.validateCommandSafety(resolvedPath)
}
