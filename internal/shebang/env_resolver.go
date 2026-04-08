package shebang

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveEnvCommand resolves name through pathEnv and returns the canonical
// (absolute, symlink-resolved) path of the executable.
//
// This is the single authoritative implementation of the env-form resolution
// pipeline, shared by both record time (parseEnvForm in parser.go) and verify
// time (verifyEnvPathResolution in verification/manager.go).  Keeping both
// callers on the same code path prevents subtle divergence that could produce
// false positives or negatives when comparing recorded vs current paths.
//
// pathEnv is a colon-separated (UNIX) or semicolon-separated (Windows)
// directory list, typically the value of the PATH environment variable.
func ResolveEnvCommand(name, pathEnv string) (string, error) {
	found, err := LookPathInEnv(name, pathEnv)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrCommandNotFound, name)
	}

	// LookPathInEnv skips non-absolute PATH entries so found is always absolute.
	// Reject the path defensively if it is still relative (e.g. name itself
	// contained a path separator and was not absolute).
	if !filepath.IsAbs(found) {
		return "", fmt.Errorf("%w: relative path %q is not supported", ErrCommandNotFound, found)
	}

	resolved, err := filepath.EvalSymlinks(found)
	if err != nil {
		return "", fmt.Errorf("failed to resolve command path %s: %w", found, err)
	}
	return resolved, nil
}

// LookPathInEnv searches for an executable named name in the directories listed
// in pathEnv (colon-separated on UNIX, semicolon-separated on Windows).
// Returns the first matching executable path or ErrCommandNotFound.
//
// Unlike exec.LookPath, this function accepts an explicit PATH string rather
// than using the current process environment, making it suitable for resolving
// commands in a runtime environment whose PATH may differ from the process PATH.
func LookPathInEnv(name, pathEnv string) (string, error) {
	if containsPathSeparator(name) {
		// name is a relative or absolute path — use directly.
		if isExecutableFile(name) {
			return name, nil
		}
		return "", ErrCommandNotFound
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if !filepath.IsAbs(dir) {
			// Skip empty and relative PATH entries; they are cwd-dependent and
			// produce non-deterministic results when invoked from different locations.
			continue
		}
		candidate := filepath.Join(dir, name)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", ErrCommandNotFound
}

// containsPathSeparator reports whether name contains a filepath separator.
func containsPathSeparator(name string) bool {
	for _, c := range name {
		if c == '/' || c == filepath.Separator {
			return true
		}
	}
	return false
}

// isExecutableFile reports whether path names a regular file with at least
// one execute bit set.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path) // #nosec G703 -- path is constructed from a trusted PATH env value
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
