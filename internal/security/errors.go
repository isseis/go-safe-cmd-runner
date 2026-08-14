package security

import "errors"

var (
	// ErrInvalidDirPermissions is returned when a directory has inappropriate permissions.
	ErrInvalidDirPermissions = errors.New("invalid directory permissions")

	// ErrInsecurePathComponent is returned for insecure path component issues.
	ErrInsecurePathComponent = errors.New("insecure path component")

	// ErrInvalidPath is returned for path structural issues.
	ErrInvalidPath = errors.New("invalid path")

	// ErrPathResolution is returned when a path could not be fully resolved for a
	// permission check. A checkable path is still returned alongside it, so callers
	// use this to record the failure rather than to skip the check.
	ErrPathResolution = errors.New("failed to resolve path for permission check")
)
