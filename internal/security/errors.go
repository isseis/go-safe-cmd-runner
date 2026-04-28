package security

import "errors"

var (
	// ErrInvalidDirPermissions is returned when a directory has inappropriate permissions.
	ErrInvalidDirPermissions = errors.New("invalid directory permissions")

	// ErrInsecurePathComponent is returned for insecure path component issues.
	ErrInsecurePathComponent = errors.New("insecure path component")

	// ErrInvalidPath is returned for path structural issues.
	ErrInvalidPath = errors.New("invalid path")
)
