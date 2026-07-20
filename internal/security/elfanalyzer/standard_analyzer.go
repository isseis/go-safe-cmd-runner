package elfanalyzer

import (
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/elfmagic"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
)

// maxFileSize is the maximum file size for ELF analysis (1 GB).
const maxFileSize = 1 << 30

// Static errors for linter compliance (err113).
var (
	// ErrNotRegularFile indicates the file is not a regular file.
	ErrNotRegularFile = errors.New("not a regular file")

	// ErrFileTooLarge indicates the file exceeds the maximum size for analysis.
	ErrFileTooLarge = errors.New("file too large")

	// ErrSyscallAnalysisHighRisk indicates syscall analysis found high-risk results.
	ErrSyscallAnalysisHighRisk = errors.New("syscall analysis high risk")

	// ErrSyscallStoreIOError indicates a syscall analysis store I/O error.
	ErrSyscallStoreIOError = errors.New("syscall analysis store I/O error")
)

// Compile-time check: StandardELFAnalyzer implements binaryanalyzer.BinaryAnalyzer.
var _ binaryanalyzer.BinaryAnalyzer = (*StandardELFAnalyzer)(nil)

// StandardELFAnalyzer implements ELFAnalyzer using Go's debug/elf package.
type StandardELFAnalyzer struct {
	fs             safefileio.FileSystem
	networkSymbols map[string]binaryanalyzer.SymbolCategory

	// syscallStore is the optional syscall analysis store for static binary analysis.
	// When set, the analyzer will lookup pre-computed syscall analysis results
	// for static binaries instead of returning StaticBinary directly.
	syscallStore SyscallAnalysisStore
}

// NewStandardELFAnalyzer creates a new StandardELFAnalyzer with the given file system.
// If fs is nil, the default safefileio.FileSystem is used.
func NewStandardELFAnalyzer(fs safefileio.FileSystem) *StandardELFAnalyzer {
	if fs == nil {
		fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	}
	return &StandardELFAnalyzer{
		fs:             fs,
		networkSymbols: binaryanalyzer.GetNetworkSymbols(),
	}
}

// NewStandardELFAnalyzerWithSyscallStore creates an analyzer with syscall analysis store support.
// Uses dependency injection for SyscallAnalysisStore to avoid circular dependencies.
//
// When a static binary is detected, the analyzer will lookup pre-computed syscall
// analysis results from the store using the file's hash for validation.
// If store is nil, the analyzer behaves like NewStandardELFAnalyzer.
func NewStandardELFAnalyzerWithSyscallStore(
	fs safefileio.FileSystem,
	store SyscallAnalysisStore,
) *StandardELFAnalyzer {
	analyzer := NewStandardELFAnalyzer(fs)

	if store != nil {
		analyzer.syscallStore = store
	}

	return analyzer
}

// AnalyzeNetworkSymbols implements binaryanalyzer.BinaryAnalyzer.
func (a *StandardELFAnalyzer) AnalyzeNetworkSymbols(path string, contentHash string) binaryanalyzer.AnalysisOutput {
	// Step 1: Open file safely using safefileio
	// This prevents symlink attacks and TOCTOU race conditions.
	file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to open file: %w", err),
		}
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.Warn("error closing file during ELF analysis", slog.Any("error", closeErr))
		}
	}()

	// Step 1b: Validate the file is a regular file and check size
	// This prevents resource exhaustion from devices, FIFOs, or extremely large files
	fileInfo, err := file.Stat()
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to stat file: %w", err),
		}
	}

	// Ensure it's a regular file, not a device, FIFO, socket, or directory
	if !fileInfo.Mode().IsRegular() {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.NotSupportedBinary,
			Error:  fmt.Errorf("%w: %s", ErrNotRegularFile, fileInfo.Mode()),
		}
	}

	// Check file size is reasonable
	if fileInfo.Size() > maxFileSize {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("%w: %d bytes (max %d)", ErrFileTooLarge, fileInfo.Size(), maxFileSize),
		}
	}

	// Step 2: Check ELF magic number
	magic := make([]byte, elfmagic.Len)
	if _, err := io.ReadFull(file, magic); err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to read magic number: %w", err),
		}
	}

	if !elfmagic.Is(magic) {
		// File is not in ELF format (e.g., Mach-O on macOS, PE on Windows,
		// or a script). The ELF analyzer cannot inspect it further.
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.NotSupportedBinary,
		}
	}

	// Step 3: Parse ELF using debug/elf.NewFile
	// The safefileio.File interface implements io.ReaderAt, so we can
	// pass it directly to elf.NewFile without re-opening the file.
	// This eliminates potential TOCTOU race conditions.
	elfFile, err := elf.NewFile(file)
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to parse ELF: %w", err),
		}
	}
	defer func() {
		if closeErr := elfFile.Close(); closeErr != nil {
			slog.Warn("error closing ELF file during analysis", slog.Any("error", closeErr))
		}
	}()

	// Step 4+5: libc symbol filtering and dynamic load symbol checking
	dynOutput := a.checkDynamicSymbols(elfFile)
	if dynOutput.Result == binaryanalyzer.StaticBinary {
		if a.syscallStore != nil {
			syscallOutput := a.lookupSyscallAnalysis(path, file, contentHash)
			if syscallOutput.Result != binaryanalyzer.StaticBinary {
				return syscallOutput
			}
		}
		return dynOutput
	}

	if dynOutput.Result != binaryanalyzer.NoNetworkSymbols {
		return dynOutput
	}

	// CGO binary fallback: when .dynsym contains no network symbols, check the
	// syscall analysis store. CGO binaries call socket() via Go runtime syscall
	// wrappers without importing libc's socket symbol, so .dynsym analysis alone
	// misses the network usage.
	if a.syscallStore != nil {
		syscallOutput := a.lookupSyscallAnalysis(path, file, contentHash)
		if syscallOutput.Result != binaryanalyzer.StaticBinary {
			// Store has data (NetworkDetected, AnalysisError, or NoNetworkSymbols).
			return syscallOutput
		}
		// No entry in store — fall through to return the dynsym result.
	}

	return dynOutput
}

// checkDynamicSymbols extracts network-related and libc symbols from the given ELF file.
// For each SHN_UNDEF symbol, applies a two-step filter:
//  1. If the symbol name is in networkSymbols, record it with the corresponding category.
//  2. Otherwise, if the symbol is from libc, record it as "syscall_wrapper".
//
// Note: For VERNEED-present (glibc) binaries, Step 2 successfully identifies non-network
// libc symbols. However, for VERNEED-absent (musl) binaries, sym.Library is always empty,
// so Step 2 never triggers and non-network libc symbols are not recorded.
func (a *StandardELFAnalyzer) checkDynamicSymbols(elfFile *elf.File) binaryanalyzer.AnalysisOutput {
	dynsyms, err := elfFile.DynamicSymbols()
	if err != nil {
		if errors.Is(err, elf.ErrNoSymbols) {
			// Static binary
			return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.StaticBinary}
		}
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to read dynamic symbols: %w", err),
		}
	}

	hasAnyUndef := slices.ContainsFunc(dynsyms, func(s elf.Symbol) bool {
		return s.Section == elf.SHN_UNDEF
	})

	// If no undefined symbols exist, this is a statically linked or import-free binary
	if !hasAnyUndef {
		return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}
	}

	var detected []binaryanalyzer.DetectedSymbol
	var dynamicLoadSyms []binaryanalyzer.DetectedSymbol

	for _, sym := range dynsyms {
		if sym.Section != elf.SHN_UNDEF {
			continue
		}

		// Step 1: name-based detection — applies regardless of sym.Library or VERNEED presence.
		if cat, found := a.networkSymbols[sym.Name]; found {
			detected = append(detected, binaryanalyzer.DetectedSymbol{
				Name:     sym.Name,
				Category: string(cat),
			})
		} else if isLibcLibrary(sym.Library) {
			// Step 2: libc symbols not in networkSymbols are syscall wrappers.
			// On VERNEED-less binaries (musl) sym.Library is always empty,
			// so isLibcLibrary returns false and syscall_wrapper is never assigned.
			detected = append(detected, binaryanalyzer.DetectedSymbol{
				Name:     sym.Name,
				Category: string(binaryanalyzer.CategorySyscallWrapper),
			})
		}

		if binaryanalyzer.IsDynamicLoadSymbol(sym.Name) {
			dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
				Name:     sym.Name,
				Category: "dynamic_load",
			})
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

// isLibcLibrary checks if the library name matches libc patterns.
func isLibcLibrary(lib string) bool {
	if lib == "" {
		return false
	}
	base := filepath.Base(lib)
	return strings.HasPrefix(base, "libc.so.") ||
		strings.HasPrefix(base, "libc.musl-")
}

// lookupSyscallAnalysis checks the syscall analysis store for analysis results.
// contentHash must be non-empty (see BinaryAnalyzer.AnalyzeNetworkSymbols contract).
func (a *StandardELFAnalyzer) lookupSyscallAnalysis(path string, _ safefileio.File, contentHash string) binaryanalyzer.AnalysisOutput {
	if contentHash == "" {
		panic("lookupSyscallAnalysis: contentHash must not be empty")
	}

	result, err := a.syscallStore.LoadSyscallAnalysis(path, contentHash)
	if err != nil {
		switch {
		case errors.Is(err, fileanalysis.ErrRecordNotFound):
			// Cache miss: no record stored yet. Fall back silently.
		case errors.Is(err, fileanalysis.ErrHashMismatch):
			// The stored record was created for a different binary. The binary has been
			// replaced since record time, which is a security-relevant condition.
			// Return AnalysisError so the caller treats this as high-risk rather than
			// silently assuming no network capability.
			return binaryanalyzer.AnalysisOutput{
				Result: binaryanalyzer.AnalysisError,
				Error:  fmt.Errorf("%w: %s", ErrSyscallHashMismatch, path),
			}
		default:
			slog.Warn("Syscall analysis lookup error",
				"path", path,
				"error", err,
				"reason", "store_io_error")
			return binaryanalyzer.AnalysisOutput{
				Result: binaryanalyzer.AnalysisError,
				Error:  fmt.Errorf("%w: %s: %w", ErrSyscallStoreIOError, path, err),
			}
		}
		return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.StaticBinary}
	}

	if result == nil {
		// A matching record exists, but the syscall analysis payload is absent
		// (commonly interpreted as analyzed but no relevant syscalls detected).
		// Treat it as StaticBinary and fall back silently.
		return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.StaticBinary}
	}
	return a.convertSyscallResult(result)
}

// convertSyscallResult converts SyscallAnalysisResult to AnalysisOutput.
// Risk is derived by scanning DetectedSyscalls for unknown syscall numbers and
// checking ArgEvalResults for mprotect PROT_EXEC risk.
func (a *StandardELFAnalyzer) convertSyscallResult(result *SyscallAnalysisResult) binaryanalyzer.AnalysisOutput {
	// Risk takes precedence over NetworkDetected: when unknown syscalls are present
	// or mprotect PROT_EXEC risk is detected, the analysis is incomplete and unreliable,
	// so we must treat the result as an error even if network syscalls were also detected.
	//
	// Number == -1 is the sentinel for "could not determine syscall number" and only
	// appears in direct-syscall entries (Source == ""). libc_symbol_import entries
	// always have Number >= 0 (enforced by validateInfos at cache-build time), so
	// they are never mistaken for unknown syscalls here.
	hasUnknown := slices.ContainsFunc(result.DetectedSyscalls, func(info SyscallInfo) bool {
		return info.Number == -1
	})
	if hasUnknown || EvalMprotectRisk(result.ArgEvalResults) {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("%w: %v", ErrSyscallAnalysisHighRisk, result.AnalysisWarnings),
		}
	}

	var symbols []binaryanalyzer.DetectedSymbol
	table := SyscallTableForArchitecture(result.Architecture)
	for _, info := range result.DetectedSyscalls {
		if table != nil && info.Number >= 0 && table.IsNetworkSyscall(info.Number) {
			symbols = append(symbols, binaryanalyzer.DetectedSymbol{
				Name:     info.Name,
				Category: "syscall",
			})
		}
	}
	if len(symbols) > 0 {
		return binaryanalyzer.AnalysisOutput{
			Result:          binaryanalyzer.NetworkDetected,
			DetectedSymbols: symbols,
		}
	}

	return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}
}
