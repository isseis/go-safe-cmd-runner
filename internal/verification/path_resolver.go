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

// validateAndCacheCommand validates that a path points to an executable file,
// resolves symlinks, and caches the result.
// Returns the symlink-resolved absolute path.
func (pr *PathResolver) validateAndCacheCommand(path, cacheKey string) (string, error) {
	if info, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("%w: %s", ErrCommandNotFound, cacheKey)
	} else if info.IsDir() {
		return "", fmt.Errorf("%w: %s is a directory", ErrCommandNotFound, cacheKey)
	}

	// Resolve symlinks to get the canonical path.
	// This ensures consistency across all subsequent security checks and execution.
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks for %s: %w", cacheKey, err)
	}

	pr.mu.Lock()
	pr.cache[cacheKey] = resolvedPath
	pr.mu.Unlock()
	return resolvedPath, nil
}

// ResolvePath resolves a command to its full, symlink-resolved path without security validation.
//
// This method performs path resolution, symlink resolution, and basic file existence checks.
// The returned path is:
//   - An absolute path
//   - Symlink-resolved (canonical path)
//   - Verified to exist and not be a directory
//
// Command allowlist validation is intentionally NOT performed here - it is the
// responsibility of the caller (GroupExecutor) to validate the resolved path
// using security.ValidateCommandAllowed(), which checks both global patterns
// and group-level cmd_allowed configuration.
//
// This separation of concerns ensures that:
//  1. Path resolution remains independent of group context
//  2. Validation can properly consider group-specific allowlists
//  3. The same resolved path can be validated differently in different contexts
//  4. TOCTOU vulnerabilities are mitigated by resolving symlinks once at the start
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
