package elfanalyzer

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

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
)

// StandardELFAnalyzer implements ELFAnalyzer using Go's debug/elf package.
type StandardELFAnalyzer struct {
	fs             safefileio.FileSystem
	networkSymbols map[string]SymbolCategory
	privManager    runnertypes.PrivilegeManager           // optional, for execute-only binaries
	pfv            *filevalidator.PrivilegedFileValidator // used for privileged file access
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
		// Check if error indicates no .dynsym section (static binary)
		if isNoDynsymError(err) {
			return AnalysisOutput{
				Result: StaticBinary,
			}
		}
		return AnalysisOutput{
			Result: AnalysisError,
			Error:  fmt.Errorf("failed to read dynamic symbols: %w", err),
		}
	}

	// Empty .dynsym is treated as static binary
	if len(dynsyms) == 0 {
		return AnalysisOutput{
			Result: StaticBinary,
		}
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

// isNoDynsymError checks if the error indicates no .dynsym section exists.
func isNoDynsymError(err error) bool {
	if err == nil {
		return false
	}
	// debug/elf returns specific errors for missing sections.
	// We only check for explicit "no symbol" patterns to avoid misclassifying
	// corruption/parsing errors as StaticBinary (which would lower safety).
	// Any malformed .dynsym should remain AnalysisError for fail-safe behavior.
	errStr := err.Error()
	return errors.Is(err, elf.ErrNoSymbols) ||
		containsAny(errStr, "no symbol", "no dynamic symbol", ".dynsym not found")
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
