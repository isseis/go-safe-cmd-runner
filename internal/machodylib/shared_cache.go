//go:build !darwin

package machodylib

// IsDyldSharedCacheLib always returns false on non-Darwin platforms;
// the dyld shared cache is a macOS/iOS-specific concept.
func IsDyldSharedCacheLib(_ string) bool {
	return false
}
