package elfanalyzer

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// SyscallAnalysisStore defines the interface for syscall analysis result storage.
type SyscallAnalysisStore interface {
	// LoadSyscallAnalysis loads syscall analysis from storage.
	// `expectedHash` contains both the hash algorithm and the expected hash value.
	// Format: "sha256:<hex>" (e.g., "sha256:abc123...def789")
	// Returns (result, nil) if found and hash matches.
	// Returns (nil, fileanalysis.ErrRecordNotFound) if not found.
	// Returns (nil, fileanalysis.ErrHashMismatch) if hash mismatch.
	// Returns (nil, nil) if no syscall analysis result exists in storage
	// (e.g., analysis was not applicable, skipped, or completed without stored results).
	// Returns (nil, error) on other errors.
	LoadSyscallAnalysis(filePath string, expectedHash string) (*SyscallAnalysisResult, error)
}

// elfMagicStr is the ELF magic number string literal.
const elfMagicStr = "\x7fELF"

// elfMagic is the ELF magic number bytes.
var elfMagic = []byte(elfMagicStr)

// elfMagicLen is the number of bytes in the ELF magic number.
const elfMagicLen = len(elfMagicStr)

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
)

// StandardELFAnalyzer implements ELFAnalyzer using Go's debug/elf package.
type StandardELFAnalyzer struct {
	fs             safefileio.FileSystem
	networkSymbols map[string]binaryanalyzer.SymbolCategory
	privManager    runnertypes.PrivilegeManager           // optional, for execute-only binaries
	pfv            *filevalidator.PrivilegedFileValidator // used for privileged file access

	// syscallStore is the optional syscall analysis store for static binary analysis.
	// When set, the analyzer will lookup pre-computed syscall analysis results
	// for static binaries instead of returning StaticBinary directly.
	syscallStore SyscallAnalysisStore
}

// NewStandardELFAnalyzer creates a new StandardELFAnalyzer with the given file system.
// If fs is nil, the default safefileio.FileSystem is used.
// privManager is optional (nil = no privilege escalation for execute-only binaries).
func NewStandardELFAnalyzer(fs safefileio.FileSystem, privManager runnertypes.PrivilegeManager) *StandardELFAnalyzer {
	if fs == nil {
		fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	}
	return &StandardELFAnalyzer{
		fs:             fs,
		networkSymbols: binaryanalyzer.GetNetworkSymbols(),
		privManager:    privManager,
		pfv:            filevalidator.NewPrivilegedFileValidator(fs),
	}
}

// NewStandardELFAnalyzerWithSymbols creates an analyzer with custom network symbols.
// This is primarily for testing purposes.
func NewStandardELFAnalyzerWithSymbols(fs safefileio.FileSystem, privManager runnertypes.PrivilegeManager, symbols map[string]binaryanalyzer.SymbolCategory) *StandardELFAnalyzer {
	if fs == nil {
		fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	}
	return &StandardELFAnalyzer{
		fs:             fs,
		networkSymbols: symbols,
		privManager:    privManager,
		pfv:            filevalidator.NewPrivilegedFileValidator(fs),
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
	privManager runnertypes.PrivilegeManager,
	store SyscallAnalysisStore,
) *StandardELFAnalyzer {
	analyzer := NewStandardELFAnalyzer(fs, privManager)

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
		// If it's a permission error and we have privilege manager, try privileged access.
		// OpenFileWithPrivileges now uses safefileio internally, providing full
		// symlink/TOCTOU protection even during privilege escalation.
		if errors.Is(err, os.ErrPermission) && a.privManager != nil {
			file, err = a.pfv.OpenFileWithPrivileges(path, a.privManager)
			if err != nil {
				return binaryanalyzer.AnalysisOutput{
					Result: binaryanalyzer.AnalysisError,
					Error:  fmt.Errorf("failed to open file with privileges: %w", err),
				}
			}
		} else {
			return binaryanalyzer.AnalysisOutput{
				Result: binaryanalyzer.AnalysisError,
				Error:  fmt.Errorf("failed to open file: %w", err),
			}
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
	magic := make([]byte, elfMagicLen)
	if _, err := io.ReadFull(file, magic); err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to read magic number: %w", err),
		}
	}

	if !isELFMagic(magic) {
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

	// Step 4: Get dynamic symbols
	dynsyms, err := elfFile.DynamicSymbols()
	if err != nil {
		// ErrNoSymbols indicates no .dynsym section exists (static binary)
		if errors.Is(err, elf.ErrNoSymbols) {
			return a.handleStaticBinary(path, file, contentHash)
		}
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to read dynamic symbols: %w", err),
		}
	}

	// Empty .dynsym is treated as static binary
	if len(dynsyms) == 0 {
		return a.handleStaticBinary(path, file, contentHash)
	}

	// Step 5: Check for network symbols and dynamic load symbols
	dynOutput := a.checkDynamicSymbols(dynsyms)
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

// checkDynamicSymbols scans the given ELF symbol list for network-related and
// dynamic-load symbols. It only considers symbols in the undefined section
// (i.e. imported from shared libraries).
func (a *StandardELFAnalyzer) checkDynamicSymbols(dynsyms []elf.Symbol) binaryanalyzer.AnalysisOutput {
	var detected []binaryanalyzer.DetectedSymbol
	var dynamicLoadSyms []binaryanalyzer.DetectedSymbol
	for _, sym := range dynsyms {
		// Only check undefined symbols (imported from shared libraries)
		// Defined symbols are exported, not imported
		if sym.Section == elf.SHN_UNDEF {
			if cat, found := a.networkSymbols[sym.Name]; found {
				detected = append(detected, binaryanalyzer.DetectedSymbol{
					Name:     sym.Name,
					Category: string(cat),
				})
			}
			if binaryanalyzer.IsDynamicLoadSymbol(sym.Name) {
				dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
					Name:     sym.Name,
					Category: "dynamic_load",
				})
			}
		}
	}

	if len(detected) > 0 {
		return binaryanalyzer.AnalysisOutput{
			Result:             binaryanalyzer.NetworkDetected,
			DetectedSymbols:    detected,
			DynamicLoadSymbols: dynamicLoadSyms,
		}
	}

	return binaryanalyzer.AnalysisOutput{
		Result:             binaryanalyzer.NoNetworkSymbols,
		DynamicLoadSymbols: dynamicLoadSyms,
	}
}

// isELFMagic checks if the given bytes match the ELF magic number.
func isELFMagic(magic []byte) bool {
	if len(magic) < elfMagicLen {
		return false
	}
	return bytes.Equal(magic[:elfMagicLen], elfMagic)
}

// handleStaticBinary handles static binary detection and syscall analysis lookup.
// If syscallStore is configured, it attempts to lookup pre-computed syscall analysis.
// Otherwise, it returns StaticBinary directly.
// contentHash must be non-empty (see BinaryAnalyzer.AnalyzeNetworkSymbols contract).
func (a *StandardELFAnalyzer) handleStaticBinary(path string, file safefileio.File, contentHash string) binaryanalyzer.AnalysisOutput {
	if a.syscallStore == nil {
		return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.StaticBinary}
	}

	result := a.lookupSyscallAnalysis(path, file, contentHash)
	if result.Result != binaryanalyzer.StaticBinary {
		return result
	}

	return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.StaticBinary}
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
			// Unexpected error, log it before falling back.
			slog.Debug("Syscall analysis lookup error",
				"path", path,
				"error", err)
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
	hasUnknown := false
	for _, info := range result.DetectedSyscalls {
		if info.Number == -1 {
			hasUnknown = true
			break
		}
	}
	if hasUnknown || EvalMprotectRisk(result.ArgEvalResults) {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("%w: %v", ErrSyscallAnalysisHighRisk, result.AnalysisWarnings),
		}
	}

	var symbols []binaryanalyzer.DetectedSymbol
	for _, info := range result.DetectedSyscalls {
		if info.IsNetwork {
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
