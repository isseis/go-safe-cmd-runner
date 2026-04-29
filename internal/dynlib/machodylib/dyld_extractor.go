//go:build !darwin

package machodylib

// ExtractLibSystemKernel extracts libsystem_kernel.dylib from the
// dyld shared cache. On non-Darwin platforms, always returns nil, nil.
func ExtractLibSystemKernel() (*LibSystemKernelBytes, error) {
	return nil, nil
}
