package elfanalyzer

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// SyscallAnalysisStore defines the interface for syscall analysis result storage.
// This decouples the analyzer from the concrete storage implementation to avoid
// circular dependencies with the internal/fileanalysis package.
// The concrete implementation is provided by an adapter that wraps fileanalysis.SyscallAnalysisStore.
type SyscallAnalysisStore interface {
	// LoadSyscallAnalysis loads syscall analysis from storage.
	// `expectedHash` contains both the hash algorithm and the expected hash value.
	// Format: "sha256:<hex>" (e.g., "sha256:abc123...def789")
	// Returns (result, true, nil) if found and hash matches.
	// Returns (nil, false, nil) if not found or hash mismatch.
	// Returns (nil, false, error) on other errors.
	LoadSyscallAnalysis(filePath string, expectedHash string) (*SyscallAnalysisResult, bool, error)
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

	// ErrSyscallAnalysisHighRisk indicates syscall analysis found unknown syscalls.
	ErrSyscallAnalysisHighRisk = errors.New("syscall analysis high risk")
)

// StandardELFAnalyzer implements ELFAnalyzer using Go's debug/elf package.
type StandardELFAnalyzer struct {
	fs             safefileio.FileSystem
	networkSymbols map[string]SymbolCategory
	privManager    runnertypes.PrivilegeManager           // optional, for execute-only binaries
	pfv            *filevalidator.PrivilegedFileValidator // used for privileged file access

	// syscallStore is the optional syscall analysis store for static binary analysis.
	// When set, the analyzer will lookup pre-computed syscall analysis results
	// for static binaries instead of returning StaticBinary directly.
	syscallStore SyscallAnalysisStore
	// hashAlgo is the hash algorithm used for file hash calculation.
	// Required when syscallStore is set.
	hashAlgo filevalidator.HashAlgorithm
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
		networkSymbols: GetNetworkSymbols(),
		privManager:    privManager,
		pfv:            filevalidator.NewPrivilegedFileValidator(fs),
	}
}

// NewStandardELFAnalyzerWithSymbols creates an analyzer with custom network symbols.
// This is primarily for testing purposes.
func NewStandardELFAnalyzerWithSymbols(fs safefileio.FileSystem, privManager runnertypes.PrivilegeManager, symbols map[string]SymbolCategory) *StandardELFAnalyzer {
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
		analyzer.hashAlgo = &filevalidator.SHA256{}
	}

	return analyzer
}

// AnalyzeNetworkSymbols implements ELFAnalyzer interface.
func (a *StandardELFAnalyzer) AnalyzeNetworkSymbols(path string) AnalysisOutput {
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
				return AnalysisOutput{
					Result: AnalysisError,
					Error:  fmt.Errorf("failed to open file with privileges: %w", err),
				}
			}
		} else {
			return AnalysisOutput{
				Result: AnalysisError,
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
		return AnalysisOutput{
			Result: AnalysisError,
			Error:  fmt.Errorf("failed to stat file: %w", err),
		}
	}

	// Ensure it's a regular file, not a device, FIFO, socket, or directory
	if !fileInfo.Mode().IsRegular() {
		return AnalysisOutput{
			Result: NotELFBinary,
			Error:  fmt.Errorf("%w: %s", ErrNotRegularFile, fileInfo.Mode()),
		}
	}

	// Check file size is reasonable
	if fileInfo.Size() > maxFileSize {
		return AnalysisOutput{
			Result: AnalysisError,
			Error:  fmt.Errorf("%w: %d bytes (max %d)", ErrFileTooLarge, fileInfo.Size(), maxFileSize),
		}
	}

	// Step 2: Check ELF magic number
	magic := make([]byte, elfMagicLen)
	if _, err := io.ReadFull(file, magic); err != nil {
		return AnalysisOutput{
			Result: AnalysisError,
			Error:  fmt.Errorf("failed to read magic number: %w", err),
		}
	}

	if !isELFMagic(magic) {
		return AnalysisOutput{
			Result: NotELFBinary,
		}
	}

	// Step 3: Parse ELF using debug/elf.NewFile
	// The safefileio.File interface implements io.ReaderAt, so we can
	// pass it directly to elf.NewFile without re-opening the file.
	// This eliminates potential TOCTOU race conditions.
	elfFile, err := elf.NewFile(file)
	if err != nil {
		return AnalysisOutput{
			Result: AnalysisError,
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
			return a.handleStaticBinary(path, file)
		}
		return AnalysisOutput{
			Result: AnalysisError,
			Error:  fmt.Errorf("failed to read dynamic symbols: %w", err),
		}
	}

	// Empty .dynsym is treated as static binary
	if len(dynsyms) == 0 {
		return a.handleStaticBinary(path, file)
	}

	// Step 5: Check for network symbols
	var detected []DetectedSymbol
	for _, sym := range dynsyms {
		// Only check undefined symbols (imported from shared libraries)
		// Defined symbols are exported, not imported
		if sym.Section == elf.SHN_UNDEF {
			if cat, found := a.networkSymbols[sym.Name]; found {
				detected = append(detected, DetectedSymbol{
					Name:     sym.Name,
					Category: string(cat),
				})
			}
		}
	}

	if len(detected) > 0 {
		return AnalysisOutput{
			Result:          NetworkDetected,
			DetectedSymbols: detected,
		}
	}

	return AnalysisOutput{
		Result: NoNetworkSymbols,
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
func (a *StandardELFAnalyzer) handleStaticBinary(path string, file safefileio.File) AnalysisOutput {
	// If syscall store is not configured, return StaticBinary directly
	if a.syscallStore == nil {
		return AnalysisOutput{
			Result: StaticBinary,
		}
	}

	// Attempt syscall analysis lookup
	result := a.lookupSyscallAnalysis(path, file)
	if result.Result != StaticBinary {
		return result
	}

	// Fallback to StaticBinary if no analysis found
	return AnalysisOutput{
		Result: StaticBinary,
	}
}

// lookupSyscallAnalysis checks the syscall analysis store for analysis results.
// Uses the already-opened file handle to calculate hash (TOCTOU safe).
func (a *StandardELFAnalyzer) lookupSyscallAnalysis(path string, file safefileio.File) AnalysisOutput {
	// Calculate file hash using the already-opened file handle
	hash, err := a.calculateFileHash(file)
	if err != nil {
		slog.Debug("Failed to calculate hash for syscall analysis lookup",
			"path", path,
			"error", err)
		return AnalysisOutput{Result: StaticBinary}
	}

	// Load analysis result
	result, found, err := a.syscallStore.LoadSyscallAnalysis(path, hash)
	if err != nil {
		slog.Debug("Syscall analysis lookup error",
			"path", path,
			"error", err)
		return AnalysisOutput{Result: StaticBinary}
	}

	if !found {
		// Result not found or hash mismatch
		return AnalysisOutput{Result: StaticBinary}
	}

	// Convert syscall analysis result to AnalysisOutput
	return a.convertSyscallResult(result)
}

// convertSyscallResult converts SyscallAnalysisResult to AnalysisOutput.
// This method relies on Summary fields set by analyzeSyscallsInCode():
//   - HasNetworkSyscalls: true if any network-related syscall was detected
//   - IsHighRisk: true if any syscall number could not be determined
//
// These fields are guaranteed to be set according to the rules in the detailed specification.
func (a *StandardELFAnalyzer) convertSyscallResult(result *SyscallAnalysisResult) AnalysisOutput {
	// Check HasNetworkSyscalls first (set when NetworkSyscallCount > 0)
	if result.Summary.HasNetworkSyscalls {
		// Build detected symbols from syscall info
		var symbols []DetectedSymbol
		if result.Summary.NetworkSyscallCount > 0 {
			symbols = make([]DetectedSymbol, 0, result.Summary.NetworkSyscallCount)
		}
		for _, info := range result.DetectedSyscalls {
			if info.IsNetwork {
				symbols = append(symbols, DetectedSymbol{
					Name:     info.Name,
					Category: "syscall",
				})
			}
		}
		return AnalysisOutput{
			Result:          NetworkDetected,
			DetectedSymbols: symbols,
		}
	}

	// Check IsHighRisk (set when HasUnknownSyscalls is true)
	if result.Summary.IsHighRisk {
		// High risk: treat as potential network operation
		return AnalysisOutput{
			Result: AnalysisError,
			Error:  fmt.Errorf("%w: %v", ErrSyscallAnalysisHighRisk, result.HighRiskReasons),
		}
	}

	return AnalysisOutput{Result: NoNetworkSymbols}
}

// calculateFileHash calculates SHA256 hash of the file.
// Returns hash in prefixed format: "sha256:<hex>" for consistency with
// FileAnalysisRecord.ContentHash schema.
// Uses the already-opened file handle to avoid TOCTOU vulnerabilities.
func (a *StandardELFAnalyzer) calculateFileHash(file safefileio.File) (string, error) {
	// Seek to beginning to ensure we hash the entire file
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to seek to file start: %w", err)
	}

	rawHash, err := a.hashAlgo.Sum(file)
	if err != nil {
		return "", err
	}

	// Return prefixed format: "sha256:<hex>"
	return fmt.Sprintf("%s:%s", a.hashAlgo.Name(), rawHash), nil
}
