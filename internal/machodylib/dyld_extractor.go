//go:build !darwin

package machodylib

// LibSystemKernelBytes holds the in-memory bytes of libsystem_kernel.dylib
// extracted from the dyld shared cache, together with its SHA-256 hash.
type LibSystemKernelBytes struct {
	Data []byte // Mach-O bytes.
	Hash string // "sha256:<hex>" (SHA-256 of Data).
}

// ExtractLibSystemKernelFromDyldCache extracts libsystem_kernel.dylib from the
// dyld shared cache. On non-Darwin platforms, always returns nil, nil.
func ExtractLibSystemKernelFromDyldCache() (*LibSystemKernelBytes, error) {
	return nil, nil
}
