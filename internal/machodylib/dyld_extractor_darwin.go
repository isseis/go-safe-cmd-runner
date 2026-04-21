//go:build darwin

package machodylib

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"

	"github.com/blacktop/ipsw/pkg/dyld"
)

// dyldSharedCachePaths is the ordered list of dyld shared cache paths to try.
// arm64e is preferred; arm64 is the fallback for older hardware.
var dyldSharedCachePaths = []string{
	"/System/Library/dyld/dyld_shared_cache_arm64e",
	"/System/Library/dyld/dyld_shared_cache_arm64",
}

// ExtractLibSystemKernelFromDyldCache extracts libsystem_kernel.dylib from the
// dyld shared cache.
//
// On failure (cache not found, image not found, or extraction failure),
// returns nil, nil so the caller can fall back to symbol-name matching.
// Logs at slog.Info level for all non-error fallback conditions.
func ExtractLibSystemKernelFromDyldCache() (*LibSystemKernelBytes, error) {
	// Try each configured dyld shared cache path in order.
	var cachePath string
	for _, p := range dyldSharedCachePaths {
		if _, err := os.Stat(p); err == nil {
			cachePath = p
			break
		}
	}
	if cachePath == "" {
		slog.Info("dyld shared cache not found; applying fallback",
			"tried", dyldSharedCachePaths)
		return nil, nil
	}

	// Parse the shared cache using blacktop/ipsw/pkg/dyld.
	f, err := dyld.Open(cachePath)
	if err != nil {
		slog.Info("Failed to open dyld shared cache; applying fallback",
			"path", cachePath, "error", err)
		return nil, nil
	}
	defer func() { _ = f.Close() }()

	// Locate the libsystem_kernel.dylib image.
	image, err := f.Image(libsystemKernelInstallName)
	if err != nil || image == nil {
		slog.Info("libsystem_kernel.dylib was not found in the dyld shared cache; applying fallback",
			"cache_path", cachePath,
			"install_name", libsystemKernelInstallName)
		return nil, nil
	}

	// Materialize the image as standalone Mach-O bytes.
	// Concrete pkg/dyld API details are isolated in extractMachOImageBytes so that
	// future API changes do not affect resolver logic or tests.
	machoBytes, err := extractMachOImageBytes(f, image)
	if err != nil {
		slog.Info("Failed to obtain libsystem_kernel.dylib bytes; applying fallback",
			"error", err)
		return nil, nil
	}

	// Compute the SHA-256 hash used as the cache validity key.
	h := sha256.Sum256(machoBytes)
	hash := fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))

	return &LibSystemKernelBytes{
		Data: machoBytes,
		Hash: hash,
	}, nil
}

// extractMachOImageBytes exports a dyld cache image as a standalone Mach-O byte slice.
// This helper is the only place that depends on concrete blacktop/go-macho method names,
// so that future API changes do not affect resolver logic or tests.
func extractMachOImageBytes(_ *dyld.File, image *dyld.CacheImage) ([]byte, error) {
	m, err := image.GetMacho()
	if err != nil {
		return nil, fmt.Errorf("failed to get Mach-O from cache image: %w", err)
	}

	// Export to a temporary file; blacktop/go-macho's Export rewrites segment offsets
	// so that the result is a valid standalone Mach-O parseable by debug/macho.
	tmpFile, err := os.CreateTemp("", "libsystem_kernel_*.dylib")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	// Include local symbols so the Symtab is populated for our syscall wrapper analysis.
	// Pass nil for DyldChainedFixups — we need only the __TEXT section and SYMTAB.
	syms := image.GetLocalSymbolsAsMachoSymbols()
	if err := m.Export(tmpPath, nil, m.GetBaseAddress(), syms); err != nil {
		return nil, fmt.Errorf("failed to export Mach-O: %w", err)
	}

	data, err := os.ReadFile(tmpPath) //nolint:gosec // G304: tmpPath is from os.CreateTemp
	if err != nil {
		return nil, fmt.Errorf("failed to read exported Mach-O: %w", err)
	}

	return data, nil
}
