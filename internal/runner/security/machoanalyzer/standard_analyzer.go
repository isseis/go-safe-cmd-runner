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
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
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

// analyzeSlice performs libSystem-filtered symbol analysis on a single *macho.File.
// Returns the AnalysisOutput for that slice.
func (a *StandardMachOAnalyzer) analyzeSlice(f *macho.File) binaryanalyzer.AnalysisOutput {
	// Retrieve referenced library list for ordinal resolution.
	libs, _ := f.ImportedLibraries()

	var detected []binaryanalyzer.DetectedSymbol
	var dynamicLoadSyms []binaryanalyzer.DetectedSymbol

	if f.Symtab != nil {
		symbols := machoUndefinedSymbols(f)
		flatNamespace := isFlatNamespace(symbols)

		for _, sym := range symbols {
			normalized := NormalizeSymbolName(sym.Name)

			// In flat namespace (ordinal==0), attribute to libSystem when libSystem
			// is present in the imported libraries list — we cannot determine the
			// exact source library, so we use the conservative heuristic that all
			// undefined symbols originate from a system library.
			fromLibSystem := isLibSystemSymbol(sym, libs) ||
				(flatNamespace && hasLibSystem(libs))
			if fromLibSystem {
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
		// Symtab absent: fall back to ImportedLibraries / ImportedSymbols.
		var err error
		detected, dynamicLoadSyms, err = a.analyzeSliceFallback(f, libs)
		if err != nil {
			return binaryanalyzer.AnalysisOutput{
				Result: binaryanalyzer.AnalysisError,
				Error:  fmt.Errorf("failed to get imported symbols: %w", err),
			}
		}
	}

	// Result is determined by whether any network-category symbol was found.
	hasNetwork := false
	for _, sym := range detected {
		if binaryanalyzer.IsNetworkCategory(sym.Category) {
			hasNetwork = true
			break
		}
	}

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

// machoUndefinedSymbols returns the undefined external symbols from a Mach-O file.
// When Dysymtab is present, only the undef section range is used.
// Otherwise, all Symtab entries with N_TYPE==N_UNDF and N_EXT are returned.
func machoUndefinedSymbols(f *macho.File) []macho.Symbol {
	const (
		nTypeMask = 0x0e // N_TYPE mask
		nUndf     = 0x0  // N_UNDF
		nExt      = 0x01 // N_EXT (external)
		nStab     = 0xe0 // stab mask
	)
	if f.Symtab == nil {
		return nil
	}
	if f.Dysymtab != nil {
		dt := f.Dysymtab
		total := uint32(len(f.Symtab.Syms)) //#nosec G115 -- len() is non-negative and slice lengths fit in uint32
		if dt.Iundefsym > total {
			return nil
		}
		end := dt.Iundefsym + dt.Nundefsym
		if end > total {
			end = total
		}
		return f.Symtab.Syms[dt.Iundefsym:end]
	}
	var result []macho.Symbol
	for _, s := range f.Symtab.Syms {
		if s.Type&nStab != 0 {
			continue
		}
		if s.Type&nTypeMask == nUndf && s.Type&nExt != 0 {
			result = append(result, s)
		}
	}
	return result
}

// libOrdinalShift is the bit shift to extract the library ordinal from Desc.
const libOrdinalShift = 8

// libOrdinalMask masks the 8-bit library ordinal field after shifting.
const libOrdinalMask = 0xFF

// isLibSystemSymbol returns true if the Mach-O symbol originates from libSystem.
// In two-level namespace binaries, the library ordinal is encoded in the upper
// byte of Desc (bits 15:8). Ordinal 1 refers to libs[0], etc.
// Special ordinals (0=SELF, 254=DYNAMIC_LOOKUP, 255=EXECUTABLE) return false.
func isLibSystemSymbol(sym macho.Symbol, libs []string) bool {
	ordinal := int((sym.Desc >> libOrdinalShift) & libOrdinalMask)
	if ordinal < 1 || ordinal > len(libs) {
		return false
	}
	return isLibSystemLibrary(libs[ordinal-1])
}

// isLibSystemLibrary returns true if the library path corresponds to libSystem
// or one of its sub-libraries (libsystem_*.dylib).
func isLibSystemLibrary(path string) bool {
	if path == "/usr/lib/libSystem.B.dylib" {
		return true
	}
	base := filepath.Base(path)
	return strings.HasPrefix(base, "libsystem_") && strings.HasSuffix(base, ".dylib")
}

// hasLibSystem returns true if any entry in libs is a libSystem library.
func hasLibSystem(libs []string) bool {
	for _, lib := range libs {
		if isLibSystemLibrary(lib) {
			return true
		}
	}
	return false
}

// isFlatNamespace returns true when all symbols use ordinal 0, which indicates
// the binary was linked with -flat_namespace (e.g., Go CGO binaries on macOS).
// Returns false for an empty slice so the fast path is taken for binaries with
// no undefined symbols.
func isFlatNamespace(syms []macho.Symbol) bool {
	if len(syms) == 0 {
		return false
	}
	for _, s := range syms {
		if (s.Desc>>libOrdinalShift)&libOrdinalMask != 0 {
			return false
		}
	}
	return true
}

// categorizeMachoSymbol categorizes a Mach-O symbol using the networkSymbols registry.
// Returns the matching category or "syscall_wrapper" for non-network libc symbols.
func categorizeMachoSymbol(name string, networkSymbols map[string]binaryanalyzer.SymbolCategory) string {
	if cat, found := networkSymbols[name]; found {
		return string(cat)
	}
	return string(binaryanalyzer.CategorySyscallWrapper)
}

// analyzeSliceFallback handles Symtab-absent Mach-O slices.
// When libSystem is listed in ImportedLibraries, all ImportedSymbols are treated
// as libSystem-derived. When libSystem is absent, no symbols are recorded.
func (a *StandardMachOAnalyzer) analyzeSliceFallback(f *macho.File, libs []string) (detected, dynamicLoadSyms []binaryanalyzer.DetectedSymbol, err error) {
	hasLibSystem := false
	for _, lib := range libs {
		if isLibSystemLibrary(lib) {
			hasLibSystem = true
			break
		}
	}

	// Without libSystem the binary does not use standard syscall interfaces.
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
