package common

// HashFilePathGetter is an interface for getting the path where the hash for a file would be stored.
// This interface is defined in the common package to avoid import cycles between filevalidator
// and fileanalysis, both of which need to reference this interface.
type HashFilePathGetter interface {
	// GetHashFilePath returns the path where the given file's hash would be stored.
	GetHashFilePath(hashDir string, filePath ResolvedPath) (string, error)
}
