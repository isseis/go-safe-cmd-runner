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

// ResolvePath resolves a command to its full path
func (pr *PathResolver) ResolvePath(command string) (string, error) {
	// Check cache first
	pr.mu.RLock()
	if cached, exists := pr.cache[command]; exists {
		pr.mu.RUnlock()
		return cached, nil
	}
	pr.mu.RUnlock()

	// Validate command safety
	if err := pr.validateCommandSafety(command); err != nil {
		return "", fmt.Errorf("unsafe command rejected: %w", err)
	}

	// If absolute path, verify it exists and is not a directory
	if filepath.IsAbs(command) {
		return pr.validateAndCacheCommand(command, command)
	}

	// Resolve from PATH environment variable
	for _, dir := range strings.Split(pr.pathEnv, string(os.PathListSeparator)) {
		// Check if directory can be accessed
		if !pr.canAccessDirectory(dir) {
			continue // Skip inaccessible directories
		}

		// Check if command file exists
		fullPath := filepath.Join(dir, command)
		if _, err := os.Stat(fullPath); err == nil {
			return pr.validateAndCacheCommand(fullPath, command)
		}
	}

	return "", fmt.Errorf("%w: %s", ErrCommandNotFound, command)
}

// removeDuplicates removes duplicate strings from a slice
func removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}
