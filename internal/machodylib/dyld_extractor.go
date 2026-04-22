//go:build !darwin

package machodylib

// ExtractLibSystemKernelFromDyldCache extracts libsystem_kernel.dylib from the
// dyld shared cache. On non-Darwin platforms, always returns nil, nil.
func ExtractLibSystemKernelFromDyldCache() (*LibSystemKernelBytes, error) {
	return nil, nil
}
