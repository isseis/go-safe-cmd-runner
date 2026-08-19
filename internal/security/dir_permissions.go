package security

const (
	// DefaultMaxPathLength defines the maximum allowed path length for checks.
	DefaultMaxPathLength = 4096
	// UIDRoot is the root user ID.
	UIDRoot = 0
	// GIDRoot is the root group ID.
	GIDRoot = 0
)

// DirectoryPermChecker validates that a directory's permissions, ownership, and
// path components are safe.
type DirectoryPermChecker interface {
	ValidateDirectoryPermissions(path string) error
}
