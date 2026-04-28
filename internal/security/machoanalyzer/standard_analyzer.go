package machoanalyzer

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
)

// ErrDirectSyscall is retained for backward compatibility.
// It is no longer returned by AnalyzeNetworkSymbols; svc #0x80 risk
// is evaluated separately via SyscallAnalysis (see svc_scanner.go).
var ErrDirectSyscall = errors.New("direct syscall instruction detected (svc #0x80)")

// ErrNotRegularFile indicates the target is not a regular file.
var ErrNotRegularFile = errors.New("not a regular file")

// ErrFileTooLarge indicates the file exceeds the maximum size for analysis.
var ErrFileTooLarge = errors.New("file too large")

// maxFileSize is the maximum file size for Mach-O analysis (1 GB).
const maxFileSize = 1 << 30

// magicNumberSize is the number of bytes in a Mach-O magic number.
const magicNumberSize = 4

// Mach-O magic numbers (see <mach-o/loader.h>)
const (
	machoMagic64 = 0xFEEDFACF // 64-bit Mach-O (native endian)
	machoCigam64 = 0xCFFAEDFE // 64-bit Mach-O (byte-swapped)
	machoMagic32 = 0xFEEDFACE // 32-bit Mach-O (native endian)
	machoCigam32 = 0xCEFAEDFE // 32-bit Mach-O (byte-swapped)
	fatMagic     = 0xCAFEBABE // Fat binary
	fatCigam     = 0xBEBAFECA // Fat binary (byte-swapped)
)

// libOrdinalMask and libOrdinalShift extract the library ordinal from a symbol's Desc field.
// The ordinal occupies bits 15:8 of Desc (see <mach-o/nlist.h> GET_LIBRARY_ORDINAL).
const (
	libOrdinalMask  = 0xFF
	libOrdinalShift = 8
)

// isMachOMagic returns true if the first 4 bytes match any Mach-O or Fat binary magic.
func isMachOMagic(b []byte) bool {
	if len(b) < magicNumberSize {
		return false
	}
	magic := binary.LittleEndian.Uint32(b[:magicNumberSize])
	switch magic {
	case machoMagic64, machoCigam64, fatMagic, fatCigam:
		return true
	}
	return false
}

// analyzeSlice performs symbol analysis on a single *macho.File with libSystem filtering.
// Records all libSystem-derived symbols (both network and non-network categories).
// Returns the AnalysisOutput for that slice.
func (a *StandardMachOAnalyzer) analyzeSlice(f *macho.File) binaryanalyzer.AnalysisOutput {
	// Get imported libraries for library ordinal resolution
	libs, err := f.ImportedLibraries()
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to get imported libraries: %w", err),
		}
	}

	var detected []binaryanalyzer.DetectedSymbol
	var dynamicLoadSyms []binaryanalyzer.DetectedSymbol

	if f.Symtab != nil {
		// Symtab available: extract undefined symbols and check library ordinal
		symbols := machoUndefinedSymbols(f)
		flatNamespace := isFlatNamespace(symbols)
		hasLibSystem := flatNamespace && slices.ContainsFunc(libs, isLibSystemLibrary)

		for _, sym := range symbols {
			normalized := NormalizeSymbolName(sym.Name)

			if isLibSystemSymbol(sym, libs) || (flatNamespace && hasLibSystem) {
				cat := categorizeMachoSymbol(normalized, a.networkSymbols)
				detected = append(detected, binaryanalyzer.DetectedSymbol{
					Name:     normalized,
					Category: cat,
				})
			}

			if binaryanalyzer.IsDynamicLoadSymbol(normalized) {
				dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
					Name:     normalized,
					Category: "dynamic_load",
				})
			}
		}
	} else {
		// Symtab unavailable: fall back to ImportedSymbols() if libSystem is in ImportedLibraries
		var err error
		detected, dynamicLoadSyms, err = a.analyzeSliceFallback(f, libs)
		if err != nil {
			return binaryanalyzer.AnalysisOutput{
				Result: binaryanalyzer.AnalysisError,
				Error:  fmt.Errorf("failed to get imported symbols: %w", err),
			}
		}
	}

	// Determine Result based on network-category symbols in detected list
	hasNetwork := slices.ContainsFunc(detected, func(s binaryanalyzer.DetectedSymbol) bool {
		return binaryanalyzer.IsNetworkCategory(s.Category)
	})

	result := binaryanalyzer.NoNetworkSymbols
	if hasNetwork {
		result = binaryanalyzer.NetworkDetected
	}

	return binaryanalyzer.AnalysisOutput{
		Result:             result,
		DetectedSymbols:    detected,
		DynamicLoadSymbols: dynamicLoadSyms,
	}
}

// analyzeAllFatSlices analyzes every slice in a Fat binary and returns the most
// severe result found across all slices. This prevents an attacker from hiding a
// malicious slice behind a benign one (e.g., a clean arm64 slice concealing a
// network-capable x86_64 slice).
//
// Severity order (highest to lowest): NetworkDetected > AnalysisError > NoNetworkSymbols.
// NotSupportedBinary is returned only when no slice could be analyzed.
func (a *StandardMachOAnalyzer) analyzeAllFatSlices(fat *macho.FatFile) binaryanalyzer.AnalysisOutput {
	var worstError binaryanalyzer.AnalysisOutput
	analyzedAny := false
	var dynamicLoadSyms []binaryanalyzer.DetectedSymbol
	seenDynLoadSyms := make(map[string]struct{})

	for i := range fat.Arches {
		slice := fat.Arches[i].File
		result := a.analyzeSlice(slice)

		for _, sym := range result.DynamicLoadSymbols {
			if _, seen := seenDynLoadSyms[sym.Name]; !seen {
				dynamicLoadSyms = append(dynamicLoadSyms, sym)
				seenDynLoadSyms[sym.Name] = struct{}{}
			}
		}

		switch result.Result {
		case binaryanalyzer.NetworkDetected:
			// Highest severity — return immediately (preserve DynamicLoadSymbols).
			result.DynamicLoadSymbols = dynamicLoadSyms
			return result
		case binaryanalyzer.AnalysisError:
			// Record but keep scanning; a later slice might be NetworkDetected.
			worstError = result
			analyzedAny = true
		case binaryanalyzer.NoNetworkSymbols:
			analyzedAny = true
		}
		// NotSupportedBinary or other: skip (don't count as analyzed)
	}

	if !analyzedAny {
		return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NotSupportedBinary}
	}

	if worstError.Result == binaryanalyzer.AnalysisError {
		worstError.DynamicLoadSymbols = dynamicLoadSyms
		return worstError
	}

	return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols, DynamicLoadSymbols: dynamicLoadSyms}
}

// AnalyzeNetworkSymbols implements binaryanalyzer.BinaryAnalyzer.
//
// For Fat binaries, all slices are analyzed. The binary is flagged if any slice
// contains network symbols or direct syscalls, preventing cross-architecture
// security bypasses.
//
// Returns:
//   - NetworkDetected: any slice imports network-related symbols
//   - NoNetworkSymbols: all slices are clean (no network symbols, no svc #0x80)
//   - NotSupportedBinary: not a Mach-O file, or Fat binary with no analyzable slices
//   - AnalysisError: parse error, or svc #0x80 detected in any slice (high risk)
func (a *StandardMachOAnalyzer) AnalyzeNetworkSymbols(path string, _ string) binaryanalyzer.AnalysisOutput {
	// Step 1: open file safely via safefileio
	file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to open file: %w", err),
		}
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.Warn("error closing file during Mach-O analysis", slog.Any("error", closeErr))
		}
	}()

	// Step 2: verify regular file (reject directories, symlinks, etc.)
	fileInfo, err := file.Stat()
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to stat file: %w", err),
		}
	}
	if !fileInfo.Mode().IsRegular() {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.NotSupportedBinary,
			Error:  fmt.Errorf("%w: %s", ErrNotRegularFile, fileInfo.Mode()),
		}
	}
	if fileInfo.Size() > maxFileSize {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("%w: %d bytes (max %d)", ErrFileTooLarge, fileInfo.Size(), maxFileSize),
		}
	}

	// Step 3: read first 4 bytes and check Mach-O / Fat magic
	magic := make([]byte, magicNumberSize)
	if _, err := io.ReadFull(file, magic); err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to read magic: %w", err),
		}
	}
	if !isMachOMagic(magic) {
		return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NotSupportedBinary}
	}

	// Step 4: seek back to start for macho.NewFile / macho.NewFatFile
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to seek: %w", err),
		}
	}

	// Step 5: dispatch on binary type
	m := binary.LittleEndian.Uint32(magic)
	if m == fatMagic || m == fatCigam {
		fat, err := macho.NewFatFile(file)
		if err != nil {
			return binaryanalyzer.AnalysisOutput{
				Result: binaryanalyzer.AnalysisError,
				Error:  fmt.Errorf("failed to parse Fat binary: %w", err),
			}
		}
		defer func() {
			if closeErr := fat.Close(); closeErr != nil {
				slog.Warn("error closing Fat Mach-O file", slog.Any("error", closeErr))
			}
		}()
		return a.analyzeAllFatSlices(fat)
	}

	// Single-arch Mach-O
	machOFile, err := macho.NewFile(file)
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to parse Mach-O: %w", err),
		}
	}
	defer func() {
		if closeErr := machOFile.Close(); closeErr != nil {
			slog.Warn("error closing Mach-O file", slog.Any("error", closeErr))
		}
	}()

	return a.analyzeSlice(machOFile)
}

// machoUndefinedSymbols extracts undefined external symbols from a Mach-O file.
// Uses Dysymtab if available for efficiency; falls back to scanning all symtab entries.
func machoUndefinedSymbols(f *macho.File) []macho.Symbol {
	const (
		nType = 0x0e
		nUndf = 0x0
		nExt  = 0x01
		nStab = 0xe0
	)
	if f.Symtab == nil {
		return nil
	}
	if f.Dysymtab != nil {
		dt := f.Dysymtab
		symCount := uint32(len(f.Symtab.Syms)) //nolint:gosec // len() is always non-negative
		if dt.Iundefsym > symCount {
			return nil
		}
		end := dt.Iundefsym + dt.Nundefsym
		if end < dt.Iundefsym || end > symCount {
			end = symCount
		}
		return f.Symtab.Syms[dt.Iundefsym:end]
	}
	var result []macho.Symbol
	for _, s := range f.Symtab.Syms {
		if s.Type&nStab != 0 {
			continue
		}
		if s.Type&nType == nUndf && s.Type&nExt != 0 {
			result = append(result, s)
		}
	}
	return result
}

// isLibSystemSymbol checks if a Mach-O symbol is from libSystem.
// Uses the library ordinal in Desc field (two-level namespace).
// Returns false for out-of-range ordinals (SELF, DYNAMIC_LOOKUP, EXECUTABLE, etc.).
func isLibSystemSymbol(sym macho.Symbol, libs []string) bool {
	ordinal := int((sym.Desc >> libOrdinalShift) & libOrdinalMask)
	if ordinal < 1 || ordinal > len(libs) {
		return false
	}
	return isLibSystemLibrary(libs[ordinal-1])
}

// isFlatNamespace reports whether all symbols use ordinal 0 (flat namespace).
// Empty input returns false.
func isFlatNamespace(symbols []macho.Symbol) bool {
	return len(symbols) > 0 && !slices.ContainsFunc(symbols, func(sym macho.Symbol) bool {
		return ((sym.Desc >> libOrdinalShift) & libOrdinalMask) != 0
	})
}

// isLibSystemLibrary checks if a library path is a libSystem variant.
func isLibSystemLibrary(path string) bool {
	if path == "/usr/lib/libSystem.B.dylib" {
		return true
	}
	base := filepath.Base(path)
	return strings.HasPrefix(base, "libsystem_") &&
		strings.HasSuffix(base, ".dylib")
}

// categorizeMachoSymbol returns the category of a symbol using networkSymbols,
// or "syscall_wrapper" if not found.
func categorizeMachoSymbol(name string, networkSymbols map[string]binaryanalyzer.SymbolCategory) string {
	if cat, found := networkSymbols[name]; found {
		return string(cat)
	}
	return string(binaryanalyzer.CategorySyscallWrapper)
}

// analyzeSliceFallback handles Mach-O analysis when Symtab is unavailable.
// If libSystem is in ImportedLibraries, all imported symbols are treated as libSystem-derived.
// If libSystem is absent, no symbols are recorded (maintains library filtering intent).
func (a *StandardMachOAnalyzer) analyzeSliceFallback(
	f *macho.File,
	libs []string,
) (detected, dynamicLoadSyms []binaryanalyzer.DetectedSymbol, err error) {
	hasLibSystem := slices.ContainsFunc(libs, isLibSystemLibrary)

	// If libSystem is not imported, don't record symbols
	if !hasLibSystem {
		return nil, nil, nil
	}

	symbols, err := f.ImportedSymbols()
	if err != nil {
		return nil, nil, err
	}

	for _, sym := range symbols {
		normalized := NormalizeSymbolName(sym)
		cat := categorizeMachoSymbol(normalized, a.networkSymbols)
		detected = append(detected, binaryanalyzer.DetectedSymbol{
			Name:     normalized,
			Category: cat,
		})
		if binaryanalyzer.IsDynamicLoadSymbol(normalized) {
			dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
				Name:     normalized,
				Category: "dynamic_load",
			})
		}
	}
	return detected, dynamicLoadSyms, nil
}
