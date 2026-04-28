package security

const (
	// DefaultMaxPathLength defines the maximum allowed path length for checks.
	DefaultMaxPathLength = 4096
)

// DirectoryPermChecker validates directory permissions for TOCTOU safety.
type DirectoryPermChecker interface {
	ValidateDirectoryPermissions(path string) error
}
