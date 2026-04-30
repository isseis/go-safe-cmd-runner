package filevalidator

import (
	"bytes"
	"cmp"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/security/machoanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/shebang"
)

// SyscallNumberTable provides syscall name and network classification by number.
// This interface is structurally identical to libccache.SyscallNumberTable and
// elfanalyzer.SyscallNumberTable; defining it here avoids a direct import of those packages.
type SyscallNumberTable interface {
	GetSyscallName(number int) string
	IsNetworkSyscall(number int) bool
}

// LibcSyscallInfo holds a syscall detected via libc import symbol matching.
// Mirrors common.SyscallInfo fields needed by Validator without importing libccache.
type LibcSyscallInfo = common.SyscallInfo

// LibcCacheInterface abstracts libc wrapper cache operations.
// Implemented by a concrete adapter wrapping libccache.LibcCacheManager.
// This avoids a direct import of libccache which would create a cycle:
// filevalidator → libccache → elfanalyzer → filevalidator.
type LibcCacheInterface interface {
	// GetOrCreateSyscalls returns the syscall infos for the given libc file.
	// It handles cache lookup and, on miss, libc ELF analysis.
	// importSymbols is the list of UND symbol names from the target binary.
	// machine is the ELF machine type of the target binary, used to select the syscall table.
	// Returns an error wrapping ErrUnsupportedArch for unsupported architectures.
	GetOrCreateSyscalls(libcPath, libcHash string, importSymbols []string, machine elf.Machine) ([]common.SyscallInfo, error)
}

// LibSystemCacheInterface abstracts libSystem wrapper cache operations for Mach-O.
type LibSystemCacheInterface interface {
	// GetSyscallInfos resolves the libsystem_kernel.dylib source from dynLibDeps,
	// matches importSymbols against the cache, and returns the detected syscalls.
	// hasLibSystemLoadCmd must be true when the binary's LC_LOAD_DYLIB entries
	// name a libSystem-family library; this allows the resolver to proceed to
	// dyld cache extraction on macOS 11+ where system libraries are absent from
	// DynLibDeps because they live only in the dyld shared cache.
	// Returns nil, nil when libSystem is not in dynLibDeps or all fallback paths failed.
	GetSyscallInfos(
		dynLibDeps []fileanalysis.LibEntry,
		importSymbols []string,
		hasLibSystemLoadCmd bool,
	) ([]common.SyscallInfo, error)
}

// ErrUnsupportedGOARCH is returned by nativeMachoCPU when runtime.GOARCH is not a
// recognised macOS architecture. Callers must abort analysis for the affected binary
// rather than silently falling back to the wrong Fat Mach-O slice.
var ErrUnsupportedGOARCH = errors.New("unsupported GOARCH for Fat Mach-O slice selection")

// ErrUnsupportedArch is returned by SyscallAnalyzerInterface.AnalyzeSyscallsFromELF
// and SyscallAnalyzerInterface.GetOrCreate when the ELF architecture is not supported.
// Adapters wrapping concrete elfanalyzer types must convert UnsupportedArchitectureError
// to this sentinel so that filevalidator can detect it without importing elfanalyzer.
var ErrUnsupportedArch = errors.New("unsupported ELF architecture")

// SyscallAnalyzerInterface defines the subset of SyscallAnalyzer methods used by Validator.
// This interface avoids a circular import: elfanalyzer already imports filevalidator
// (via standard_analyzer.go), so filevalidator cannot import elfanalyzer directly.
// Implementations must convert elfanalyzer.UnsupportedArchitectureError to ErrUnsupportedArch.
type SyscallAnalyzerInterface interface {
	// AnalyzeSyscallsFromELF analyzes the ELF file for direct syscall instructions.
	// Returns detected syscalls, argument evaluation results (e.g., mprotect PROT_EXEC),
	// and determination stats for debug use.
	// Returns an error wrapping ErrUnsupportedArch (detectable via errors.Is) for
	// unsupported architectures.
	AnalyzeSyscallsFromELF(elfFile *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error)
	// EvaluatePLTCallArgs scans .text for CALL/BL instructions targeting funcName's
	// PLT stub and backward-scans the third argument register at each call site.
	// Returns (nil, nil) if funcName has no PLT entry or no call sites are found.
	// Returns an error wrapping ErrUnsupportedArch for unsupported architectures.
	EvaluatePLTCallArgs(elfFile *elf.File, funcName string) (*common.SyscallArgEvalResult, error)
	// GetSyscallTable returns the SyscallNumberTable for the given machine type.
	// Returns (table, true) for supported architectures, (nil, false) for unsupported ones.
	GetSyscallTable(machine elf.Machine) (SyscallNumberTable, bool)
}

// Error definitions for static error handling
var (
	// errNotELF is returned by openELFFile when the file is not an ELF binary.
	errNotELF = errors.New("file is not an ELF binary")
)

// FileValidator interface defines the basic file validation methods
type FileValidator interface {
	SaveRecord(filePath string, force bool) (string, string, error)
	Verify(filePath string) error
	// VerifyWithHash verifies the file and returns the prefixed content hash ("algo:hex")
	// so callers can forward it to downstream consumers without a redundant file read.
	VerifyWithHash(filePath string) (string, error)
	VerifyAndRead(filePath string) ([]byte, error)
	// LoadRecord returns the full analysis record for the given file path.
	// Used by verification.Manager to access DynLibDeps without exposing the store directly.
	LoadRecord(filePath string) (*fileanalysis.Record, error)
}

// HashFilePath returns the path where the hash for the given file would be stored.
func (v *Validator) HashFilePath(filePath common.ResolvedPath) (string, error) {
	return v.hashFilePathGetter.GetHashFilePath(v.hashDir, filePath)
}

// Store returns the underlying fileanalysis.Store.
// This is useful for accessing syscall analysis results stored alongside hashes.
func (v *Validator) Store() *fileanalysis.Store {
	return v.store
}

// Validator provides functionality to record and verify file hashes.
// It should be instantiated using the New function.
type Validator struct {
	algorithm          HashAlgorithm
	hashDir            common.ResolvedPath
	hashFilePathGetter common.HashFilePathGetter

	// store is the unified analysis store for FileAnalysisRecord format.
	store *fileanalysis.Store

	fileSystem          safefileio.FileSystem           // used by openELFFile in analyzeELFSyscalls
	elfDynlibAnalyzer   *elfdynlib.DynLibAnalyzer       // nil if dynlib analysis is disabled
	machoDynlibAnalyzer *machodylib.MachODynLibAnalyzer // nil if Mach-O dynlib analysis is disabled
	binaryAnalyzer      binaryanalyzer.BinaryAnalyzer   // nil if binary analysis is disabled
	libcCache           LibcCacheInterface              // nil if libc cache is disabled
	libSystemCache      LibSystemCacheInterface         // nil if Mach-O libSystem cache is disabled
	syscallAnalyzer     SyscallAnalyzerInterface        // nil if syscall analysis is disabled
	machoSyscallTable   SyscallNumberTable              // nil falls back to noop table in ScanSyscallInfos
	includeDebugInfo    bool
}

// New initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
// The hash directory is created automatically if it does not exist.
// This constructor uses the FileAnalysisRecord format for storing hash and analysis results.
// The analysis store preserves existing fields (e.g., SyscallAnalysis) when updating hashes.
func New(algorithm HashAlgorithm, hashDir string) (*Validator, error) {
	hashFilePathGetter := NewHybridHashFilePathGetter()

	// Create analysis store first — this creates the directory if it doesn't exist.
	store, err := fileanalysis.NewStore(hashDir, hashFilePathGetter)
	if err != nil {
		return nil, fmt.Errorf("failed to create analysis store: %w", err)
	}

	// The directory now exists; resolve it to an absolute, symlink-free path.
	resolvedHashDir, err := common.NewResolvedPath(hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve hash directory path %q: %w", hashDir, err)
	}

	// Now create the validator — the directory is guaranteed to exist.
	v, err := newValidator(algorithm, resolvedHashDir, hashFilePathGetter)
	if err != nil {
		return nil, err
	}
	v.store = store

	return v, nil
}

// newValidator initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
func newValidator(algorithm HashAlgorithm, hashDir common.ResolvedPath, hashFilePathGetter common.HashFilePathGetter) (*Validator, error) {
	if algorithm == nil {
		return nil, ErrNilAlgorithm
	}

	// Ensure the hash directory exists and is a directory
	info, err := os.Lstat(hashDir.String())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrHashDirNotExist, hashDir)
		}
		return nil, fmt.Errorf("failed to access hash directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrHashPathNotDir, hashDir)
	}

	return &Validator{
		algorithm:          algorithm,
		hashDir:            hashDir,
		hashFilePathGetter: hashFilePathGetter,
		fileSystem:         safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
	}, nil
}

// SaveRecord calculates the hash of the file at filePath and saves it to the hash directory.
// The hash file is named using a URL-safe Base64 encoding of the file path.
// If force is true, existing hash files for the same file path will be overwritten.
// Returns ErrHashFilePathCollision if a different file's record occupies the same
// hash file path (possible with SHA256 fallback encoding for very long paths).
// Existing fields (e.g., SyscallAnalysis) in the record are preserved when updating.
func (v *Validator) SaveRecord(filePath string, force bool) (string, string, error) {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return "", "", err
	}

	// Analyze shebang and record interpreter binaries before persisting this record.
	// This ensures atomic failure: if interpreter recording fails, the script record
	// is not written either.
	shebangInfo, err := v.resolveShebangInfo(targetPath.String())
	if err != nil {
		return "", "", err
	}
	if shebangInfo != nil {
		// Use saveRecordCore to skip redundant shebang analysis: resolveShebangInfo
		// already confirmed the interpreters are not shebang scripts themselves.
		if err := v.saveInterpreterRecord(shebangInfo.InterpreterPath, force); err != nil {
			return "", "", fmt.Errorf("failed to record interpreter %s: %w",
				shebangInfo.InterpreterPath, err)
		}
		if shebangInfo.ResolvedPath != "" {
			if err := v.saveInterpreterRecord(shebangInfo.ResolvedPath, force); err != nil {
				return "", "", fmt.Errorf("failed to record resolved command %s: %w",
					shebangInfo.ResolvedPath, err)
			}
		}
	}

	return v.saveRecordCore(targetPath.String(), force, shebangInfo)
}

// saveInterpreterRecord records an interpreter binary discovered via shebang analysis.
// When force is true, an existing record is overwritten.
// When force is false, an existing record is left unchanged (ErrHashFileExists is silently
// ignored), because multiple scripts may share the same interpreter and the caller's
// non-destructive intent should not prevent the script itself from being recorded.
func (v *Validator) saveInterpreterRecord(path string, force bool) error {
	_, _, err := v.saveRecordCore(path, force, nil)
	if !force && errors.Is(err, ErrHashFileExists) {
		return nil
	}
	return err
}

// saveRecordCore calculates the hash and persists the analysis record for filePath.
// shebangInfo must be pre-resolved by the caller; nil means non-script file.
// Unlike SaveRecord, this method does NOT perform shebang analysis itself.
// Use SaveRecord for files whose shebang status is unknown.
func (v *Validator) saveRecordCore(filePath string, force bool, shebangInfo *shebang.Info) (string, string, error) {
	targetPath, err := validatePath(filePath)
	if err != nil {
		return "", "", err
	}

	// Calculate the hash of the file
	hash, err := v.calculateHash(targetPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Get the path for the hash file
	hashFilePath, err := v.HashFilePath(targetPath)
	if err != nil {
		return "", "", err
	}

	contentHash, err := v.updateAnalysisRecord(targetPath, hash, force, shebangInfo)
	if err != nil {
		return "", "", err
	}
	return hashFilePath, contentHash, nil
}

// updateAnalysisRecord saves the hash using FileAnalysisRecord format.
// This format preserves existing fields (e.g., SyscallAnalysis) when updating.
//
// Collision and duplicate detection are performed inside the Update callback,
// which avoids a redundant Load() call and keeps error handling in sync with
// Store.Update()'s own semantics (e.g., SchemaVersionMismatchError is rejected
// by Update before the callback runs).
func (v *Validator) updateAnalysisRecord(filePath common.ResolvedPath, hash string, force bool, shebangInfo *shebang.Info) (string, error) {
	contentHash := fmt.Sprintf("%s:%s", v.algorithm.Name(), hash)
	err := v.store.Update(filePath, func(record *fileanalysis.Record) error {
		// record.FilePath is non-empty when a valid existing record was loaded.
		// An empty FilePath means the record is new (not found or was corrupted).
		if record.FilePath != "" && record.FilePath != filePath.String() {
			return fmt.Errorf("%w: %s and %s map to the same record file",
				ErrHashFilePathCollision, filePath, record.FilePath)
		}
		if record.FilePath == filePath.String() && !force {
			return fmt.Errorf("hash file already exists for %s: %w", filePath, ErrHashFileExists)
		}
		record.ContentHash = contentHash

		// Analyze dynamic library dependencies (ELF + Mach-O).
		if err := v.analyzeDynLibDeps(filePath.String(), record); err != nil {
			return err
		}

		// Analyze binary symbols if analyzer is available.
		// Stores the result as SymbolAnalysis in the record.
		if v.binaryAnalyzer != nil {
			output := v.binaryAnalyzer.AnalyzeNetworkSymbols(filePath.String(), contentHash)
			switch output.Result {
			case binaryanalyzer.NetworkDetected, binaryanalyzer.NoNetworkSymbols:
				record.SymbolAnalysis = &fileanalysis.SymbolAnalysisData{
					DetectedSymbols:    convertDetectedSymbols(output.DetectedSymbols),
					DynamicLoadSymbols: convertDetectedSymbols(output.DynamicLoadSymbols),
				}
			case binaryanalyzer.StaticBinary, binaryanalyzer.NotSupportedBinary:
				// SymbolAnalysis does not apply to static or unsupported binaries.
				// Explicitly nil it out so that a force-re-record of a binary that was
				// previously dynamic does not carry over the old SymbolAnalysis value
				// (store.Update passes the existing record to this callback unchanged).
				record.SymbolAnalysis = nil
			case binaryanalyzer.AnalysisError:
				return fmt.Errorf("network symbol analysis failed: %w", output.Error)
			}
		}

		// Detect known network libraries based on SOName.
		// Run only when DynLibDeps is recorded and SymbolAnalysis is set.
		if len(record.DynLibDeps) > 0 && record.SymbolAnalysis != nil {
			var matched []string
			for _, lib := range record.DynLibDeps {
				base := filepath.Base(lib.SOName)
				if binaryanalyzer.IsKnownNetworkLibrary(base) {
					matched = append(matched, lib.SOName)
				}
			}
			slices.Sort(matched)
			record.SymbolAnalysis.KnownNetworkLibDeps = matched
		}

		// Analyze ELF syscalls via libc import symbol matching and direct instruction scan.
		if err := v.analyzeELFSyscalls(record, filePath.String()); err != nil {
			return err
		}

		// Mach-O arm64 svc #0x80 scan and libSystem import-symbol matching.
		// Merge both results and store them in record.SyscallAnalysis (task 0100).
		// ScanSVCAddrs checks magic bytes and returns nil for non-Mach-O
		// files, so this is safe to call on all platforms and binary formats.
		if err := v.analyzeMachoSyscalls(record, filePath.String()); err != nil {
			return err
		}

		// Record shebang interpreter info.
		if shebangInfo != nil {
			record.ShebangInterpreter = &fileanalysis.ShebangInterpreterInfo{
				RawInterpreterPath: shebangInfo.RawInterpreterPath,
				InterpreterPath:    shebangInfo.InterpreterPath,
				CommandName:        shebangInfo.CommandName,
				ResolvedPath:       shebangInfo.ResolvedPath,
			}
		} else {
			record.ShebangInterpreter = nil
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to update analysis record: %w", err)
	}

	return contentHash, nil
}

// LoadRecord returns the full analysis record for the given file path.
func (v *Validator) LoadRecord(filePath string) (*fileanalysis.Record, error) {
	targetPath, err := validatePath(filePath)
	if err != nil {
		return nil, err
	}
	record, err := v.store.Load(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load analysis record: %w", err)
	}
	return record, nil
}

// resolveShebangInfo parses the shebang line of the file at filePath and
// returns the interpreter info. Returns (nil, nil) for non-shebang files.
// Returns an error wrapping ErrRecursiveShebang if the interpreter is itself
// a shebang script.
func (v *Validator) resolveShebangInfo(filePath string) (*shebang.Info, error) {
	shebangInfo, err := shebang.Parse(filePath, v.fileSystem)
	if err != nil {
		return nil, fmt.Errorf("shebang analysis failed for %s: %w", filePath, err)
	}
	if shebangInfo == nil {
		return nil, nil
	}

	if err := v.checkNotShebang(shebangInfo.InterpreterPath, "interpreter"); err != nil {
		return nil, err
	}

	if shebangInfo.ResolvedPath != "" {
		if err := v.checkNotShebang(shebangInfo.ResolvedPath, "resolved command"); err != nil {
			return nil, err
		}
	}

	return shebangInfo, nil
}

// checkNotShebang returns ErrRecursiveShebang if path is itself a shebang script.
// role is a human-readable label ("interpreter", "resolved command") used in error messages.
func (v *Validator) checkNotShebang(path, role string) error {
	isScript, err := shebang.IsShebangScript(path, v.fileSystem)
	if err != nil {
		return fmt.Errorf("failed to check %s %s: %w", role, path, err)
	}
	if isScript {
		return fmt.Errorf("%s %s is itself a shebang script: %w", role, path, ErrRecursiveShebang)
	}
	return nil
}

// SetLibSystemCache injects the LibSystemCacheInterface used during record operations.
func (v *Validator) SetLibSystemCache(m LibSystemCacheInterface) {
	v.libSystemCache = m
}

// SetMachoSyscallTable injects the SyscallNumberTable used for macOS BSD syscall
// number resolution during Pass 1 and Pass 2 analysis. When nil, syscall names
// and network flags are left empty but numbers are still resolved where possible.
func (v *Validator) SetMachoSyscallTable(t SyscallNumberTable) {
	v.machoSyscallTable = t
}

// SetELFDynLibAnalyzer injects the DynLibAnalyzer used during record operations.
// Call before the first SaveRecord() invocation. Safe to call with nil (disables dynlib analysis).
func (v *Validator) SetELFDynLibAnalyzer(a *elfdynlib.DynLibAnalyzer) {
	v.elfDynlibAnalyzer = a
}

// SetMachODynLibAnalyzer injects the MachODynLibAnalyzer used during record operations.
// Call before the first SaveRecord() invocation. Safe to call with nil (disables Mach-O dynlib analysis).
func (v *Validator) SetMachODynLibAnalyzer(a *machodylib.MachODynLibAnalyzer) {
	v.machoDynlibAnalyzer = a
}

// analyzeDynLibDeps analyzes dynamic library dependencies for the given file and
// updates the record. ELF analysis runs first; Mach-O analysis runs only when ELF
// returns no results. Both fields are cleared before analysis when at least one
// analyzer is set, to prevent stale data from a previous record.
func (v *Validator) analyzeDynLibDeps(filePath string, record *fileanalysis.Record) error {
	if v.elfDynlibAnalyzer == nil && v.machoDynlibAnalyzer == nil {
		return nil
	}

	// Stale data prevention: reset before re-analysis.
	record.DynLibDeps = nil
	record.AnalysisWarnings = nil

	if v.elfDynlibAnalyzer != nil {
		dynLibDeps, err := v.elfDynlibAnalyzer.Analyze(filePath)
		if err != nil {
			return fmt.Errorf("dynamic library analysis failed: %w", err)
		}

		record.DynLibDeps = dynLibDeps // nil for non-ELF or static ELF (omitted in JSON)
	}

	// Mach-O analysis: only when ELF analysis returned no results.
	if v.machoDynlibAnalyzer != nil && len(record.DynLibDeps) == 0 {
		libs, warns, err := v.machoDynlibAnalyzer.Analyze(filePath)
		if err != nil {
			return fmt.Errorf("Mach-O dynamic library analysis failed: %w", err)
		}

		record.DynLibDeps = libs
		for _, w := range warns {
			record.AnalysisWarnings = append(record.AnalysisWarnings, w.String())
		}
	}

	slices.SortFunc(record.DynLibDeps, func(a, b fileanalysis.LibEntry) int {
		if c := cmp.Compare(a.SOName, b.SOName); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return cmp.Compare(a.Hash, b.Hash)
	})
	slices.Sort(record.AnalysisWarnings)

	return nil
}

// SetBinaryAnalyzer injects the BinaryAnalyzer used during record operations.
// Call before the first SaveRecord() invocation. Safe to call with nil (disables binary analysis).
func (v *Validator) SetBinaryAnalyzer(a binaryanalyzer.BinaryAnalyzer) {
	v.binaryAnalyzer = a
}

// SetLibcCache injects the LibcCacheInterface used during record operations.
func (v *Validator) SetLibcCache(m LibcCacheInterface) {
	v.libcCache = m
}

// SetSyscallAnalyzer injects the SyscallAnalyzer used during record operations.
func (v *Validator) SetSyscallAnalyzer(a SyscallAnalyzerInterface) {
	v.syscallAnalyzer = a
}

// SetIncludeDebugInfo controls whether debug information (Occurrences,
// DeterminationStats) is included in saved JSON output.
func (v *Validator) SetIncludeDebugInfo(b bool) {
	v.includeDebugInfo = b
}

// Verify checks if the file at filePath matches its recorded hash.
// Returns ErrMismatch if the hashes don't match, or ErrHashFileNotFound if no hash is recorded.
func (v *Validator) Verify(filePath string) error {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return err
	}

	// Calculate the current hash
	actualHash, err := v.calculateHash(targetPath)
	if os.IsNotExist(err) {
		return err
	}
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	return v.verifyHash(targetPath, actualHash)
}

// VerifyWithHash checks if the file at filePath matches its recorded hash and
// returns the prefixed content hash ("algo:hex") on success.
// It behaves identically to Verify but also returns the computed hash so that
// callers can forward it to downstream consumers (e.g. ELF analysis) without
// a redundant read of the file.
func (v *Validator) VerifyWithHash(filePath string) (string, error) {
	targetPath, err := validatePath(filePath)
	if err != nil {
		return "", err
	}

	actualHash, err := v.calculateHash(targetPath)
	if os.IsNotExist(err) {
		return "", err
	}
	if err != nil {
		return "", fmt.Errorf("failed to calculate file hash: %w", err)
	}

	if err := v.verifyHash(targetPath, actualHash); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%s", v.algorithm.Name(), actualHash), nil
}

// verifyHash verifies the hash using FileAnalysisRecord format.
func (v *Validator) verifyHash(filePath common.ResolvedPath, actualHash string) error {
	record, err := v.store.Load(filePath)
	if err != nil {
		if errors.Is(err, fileanalysis.ErrRecordNotFound) {
			return ErrHashFileNotFound
		}
		return fmt.Errorf("failed to load analysis record: %w", err)
	}

	// Check for hash file path collision
	if record.FilePath != filePath.String() {
		return fmt.Errorf("%w: record belongs to %s, not %s",
			ErrHashFilePathCollision, record.FilePath, filePath)
	}

	// ContentHash is in prefixed format "sha256:<hex>"
	expectedHash := fmt.Sprintf("%s:%s", v.algorithm.Name(), actualHash)
	if record.ContentHash != expectedHash {
		return ErrMismatch
	}

	return nil
}

// validatePath validates and normalizes the given file path.
func validatePath(filePath string) (common.ResolvedPath, error) {
	rp, err := common.NewResolvedPath(filePath)
	if err != nil {
		return common.ResolvedPath{}, err
	}

	// check if resolvedPath is a regular file
	fileInfo, err := os.Lstat(rp.String())
	if err != nil {
		return common.ResolvedPath{}, err
	}
	if !fileInfo.Mode().IsRegular() {
		return common.ResolvedPath{}, fmt.Errorf("%w: not a regular file: %v", safefileio.ErrInvalidFilePath, rp)
	}

	return rp, nil
}

// calculateHash calculates the hash of the file at the given path.
// filePath must be validated by validatePath before calling this function.
func (v *Validator) calculateHash(filePath common.ResolvedPath) (string, error) {
	content, err := safefileio.SafeReadFile(filePath)
	if err != nil {
		return "", err
	}
	return v.algorithm.Sum(bytes.NewReader(content))
}

// verifyAndReadContent performs the common verification and reading logic
// readContent should return the file content and any read error
func (v *Validator) verifyAndReadContent(targetPath common.ResolvedPath, readContent func() ([]byte, error)) ([]byte, error) {
	// Read file content
	content, err := readContent()
	if err != nil {
		return nil, err
	}

	// Calculate hash of the content we just read
	actualHash, err := v.algorithm.Sum(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}

	if verifyErr := v.verifyHash(targetPath, actualHash); verifyErr != nil {
		return nil, verifyErr
	}
	return content, nil
}

// VerifyAndRead atomically verifies file integrity and returns its content to prevent TOCTOU attacks
func (v *Validator) VerifyAndRead(filePath string) ([]byte, error) {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return nil, err
	}

	// Use common verification logic with normal file reading
	return v.verifyAndReadContent(targetPath, func() ([]byte, error) {
		content, err := safefileio.SafeReadFile(targetPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return content, nil
	})
}

// convertDetectedSymbols converts binaryanalyzer.DetectedSymbol slice to []string.
// Returns nil for empty input to keep JSON output clean with omitempty.
//
// NOTE: This is the inverse of convertNetworkSymbolEntries in
// internal/runner/security/network_analyzer.go. fileanalysis stores symbol
// names as plain strings.
func convertDetectedSymbols(syms []binaryanalyzer.DetectedSymbol) []string {
	if len(syms) == 0 {
		return nil
	}
	entries := make([]string, len(syms))
	for i, s := range syms {
		entries[i] = s.Name
	}
	slices.Sort(entries)
	return entries
}

// buildMachoSyscallData merges svc and libSystem entries and constructs
// SyscallAnalysisData.
// AnalysisWarnings is populated only when unresolved svc #0x80 entries exist
// (i.e., entries with an Occurrence where DeterminationMethod="direct_svc_0x80" AND Number == -1).
// When all svc entries are resolved (Number != -1), no warning is emitted.
// DetectedSyscalls contains all entries without filtering.
func buildMachoSyscallData(
	svcEntries []common.SyscallInfo,
	libsysEntries []common.SyscallInfo,
	arch string,
	includeDebugInfo bool,
) *fileanalysis.SyscallAnalysisData {
	merged := mergeMachoSyscallInfos(svcEntries, libsysEntries)

	var warnings []string
	for _, s := range merged {
		if s.Number == -1 {
			// Check if any Occurrence has DeterminationMethod="direct_svc_0x80"
			for _, occ := range s.Occurrences {
				if occ.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 {
					warnings = []string{"svc #0x80 detected: syscall number unresolved, direct kernel call bypassing libSystem.dylib"}
					break
				}
			}
			if len(warnings) > 0 {
				break
			}
		}
	}

	syscalls := merged
	if !includeDebugInfo {
		syscalls = stripOccurrences(merged)
	}

	return &fileanalysis.SyscallAnalysisData{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture:     arch,
			AnalysisWarnings: warnings,
			DetectedSyscalls: syscalls,
		},
	}
}

// mergeMachoSyscallInfos combines svc entries and libSystem entries into a
// deterministically sorted slice grouped by syscall number.
// Entries with the same Number are merged into a single SyscallInfo with
// multiple Occurrences, sorted by Location. When merging, a non-empty Name is
// preferred over an empty one.
// Groups are sorted by Number (ascending), with Number=-1 at the end.
func mergeMachoSyscallInfos(svcEntries, libsysEntries []common.SyscallInfo) []common.SyscallInfo {
	if len(svcEntries) == 0 && len(libsysEntries) == 0 {
		return nil
	}
	merged := make([]common.SyscallInfo, 0, len(svcEntries)+len(libsysEntries))
	merged = append(merged, svcEntries...)
	merged = append(merged, libsysEntries...)
	return common.GroupAndSortSyscalls(merged)
}

// analyzeMachoSyscalls runs the Mach-O Pass 1 / Pass 2 syscall scan and
// libSystem import-symbol matching, then stores the merged result in
// record.SyscallAnalysis.
//
// Pass 1 (direct svc #0x80): resolves syscall numbers via X16 backward scan.
// Pass 2 (Go wrapper calls): resolves syscall numbers via X0 backward scan at
// BL call sites targeting known Go syscall stubs.
//
// It is a no-op (leaves SyscallAnalysis unchanged) when no entries are found.
// ScanSyscallInfos checks magic bytes and returns nil for non-Mach-O files, so
// this is safe to call on all platforms and binary formats.
func (v *Validator) analyzeMachoSyscalls(record *fileanalysis.Record, filePath string) error {
	svcEntries, wrapperEntries, err := machoanalyzer.ScanSyscallInfos(filePath, v.fileSystem, v.machoSyscallTable)
	if err != nil {
		return fmt.Errorf("mach-o syscall scan failed: %w", err)
	}

	libsysEntries, libsysArch, err := v.analyzeLibSystem(record, filePath)
	if err != nil {
		return fmt.Errorf("libSystem import analysis failed: %w", err)
	}

	// Combine Go wrapper call results with libSystem entries: both are
	// non-direct-svc detections and do not trigger the high-risk svc warning.
	wrapperEntries = append(wrapperEntries, libsysEntries...)
	combinedLibEntries := wrapperEntries

	if len(svcEntries)+len(combinedLibEntries) > 0 {
		// Use the architecture from the Mach-O slice used for libSystem analysis.
		// Fall back to archNameArm64 when no libSystem info was available: svc scan
		// only processes arm64 slices, so entries always originate from arm64.
		arch := libsysArch
		if arch == "" {
			arch = archNameArm64
		}
		record.SyscallAnalysis = buildMachoSyscallData(svcEntries, combinedLibEntries, arch, v.includeDebugInfo)
	}
	return nil
}

// analyzeLibSystem obtains imported symbols from the target Mach-O binary
// and matches them against the libSystem cache to identify syscall wrappers.
// Returns nil, nil when v.libSystemCache is nil or the file is not Mach-O.
// Note: DynLibDeps may be empty on macOS 11+ because all system libraries
// (including libSystem.B.dylib) live in the dyld shared cache and are not
// hash-verified by MachODynLibAnalyzer. The adapter's fallback symbol-name
// matching handles detection in that case.
func (v *Validator) analyzeLibSystem(
	record *fileanalysis.Record,
	filePath string,
) ([]common.SyscallInfo, string, error) {
	if v.libSystemCache == nil {
		return nil, "", nil
	}

	info, err := getMachoAnalysisInfo(v.fileSystem, filePath)
	if err != nil || info == nil {
		return nil, "", err
	}

	// Strip the Mach-O underscore prefix (e.g. "_socket" → "socket") before matching.
	normalized := make([]string, len(info.importSymbols))
	for i, sym := range info.importSymbols {
		normalized[i] = machoanalyzer.NormalizeSymbolName(sym)
	}

	syscalls, err := v.libSystemCache.GetSyscallInfos(record.DynLibDeps, normalized, info.hasLibSystemLoadCmd)
	return syscalls, info.architecture, err
}

// machoAnalysisInfo holds the results of a Mach-O load-command inspection.
type machoAnalysisInfo struct {
	// importSymbols is the list of UND symbols from the Mach-O symbol table.
	importSymbols []string
	// hasLibSystemLoadCmd is true when any LC_LOAD_DYLIB entry names a
	// libSystem-family library (/usr/lib/libSystem.B.dylib or libsystem_kernel.dylib).
	hasLibSystemLoadCmd bool
	// architecture is the arch string derived from the Mach-O CPU type (e.g. "arm64", "x86_64").
	architecture string
}

// machoCPUToArchName converts a macho.Cpu constant to the architecture name
// used in SyscallAnalysisResultCore.Architecture (matching the ELF convention).
func machoCPUToArchName(cpu macho.Cpu) string {
	switch cpu {
	case macho.CpuArm64:
		return archNameArm64
	case macho.CpuAmd64:
		return "x86_64"
	default:
		return cpu.String()
	}
}

// machoFatMagicLE and machoFatCigamLE are the Mach-O fat-header magic values
// as seen after decoding the first 4 bytes with binary.LittleEndian.Uint32.
// A real fat header written on disk as big-endian 0xCAFEBABE decodes to
// 0xBEBAFECA, while the byte-swapped form 0xBEBAFECA decodes to 0xCAFEBABE.
const (
	machoFatMagicLE = uint32(0xBEBAFECA)
	machoFatCigamLE = uint32(0xCAFEBABE)
)

// archNameArm64 is the canonical architecture string for Apple Silicon (arm64).
const archNameArm64 = "arm64"

// nativeMachoCPU returns the macho.Cpu constant for the current runtime.GOARCH.
// Returns an error for unrecognised architectures to prevent silent wrong-slice
// selection in fat binaries — a security-critical mismatch.
// Add a new case here whenever a new macOS/Go architecture is supported.
func nativeMachoCPU() (macho.Cpu, error) {
	switch runtime.GOARCH {
	case "arm64":
		return macho.CpuArm64, nil
	case "amd64":
		return macho.CpuAmd64, nil
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedGOARCH, runtime.GOARCH)
	}
}

// nativeOrArm64Slice returns the Fat arch slice that matches nativeCPU,
// falling back to arm64 if the native arch is not present.
// Returns nil when neither nativeCPU nor arm64 is present in the fat binary.
func nativeOrArm64Slice(fat *macho.FatFile, nativeCPU macho.Cpu) *macho.File {
	var arm64Slice *macho.File
	for i := range fat.Arches {
		cpu := fat.Arches[i].Cpu
		if cpu == nativeCPU {
			return fat.Arches[i].File
		}
		if cpu == macho.CpuArm64 {
			arm64Slice = fat.Arches[i].File
		}
	}
	return arm64Slice
}

// extractMachoSliceInfo extracts imported symbols and libSystem load-command
// presence from an already-parsed *macho.File slice.
func extractMachoSliceInfo(mf *macho.File) *machoAnalysisInfo {
	syms, err := mf.ImportedSymbols()
	if err != nil {
		// debug/macho returns FormatError when Symtab is nil (e.g. stripped binaries).
		// Return an empty slice so the caller can distinguish "is a Mach-O but has no
		// imports" from "not a Mach-O" (which returns nil).
		syms = []string{} //nolint:nilerr // FormatError for missing Symtab is not fatal
	}

	hasLibSystem := false
	if libs, libErr := mf.ImportedLibraries(); libErr == nil {
		for _, lib := range libs {
			if lib == "/usr/lib/libSystem.B.dylib" || filepath.Base(lib) == "libsystem_kernel.dylib" {
				hasLibSystem = true
				break
			}
		}
	}

	return &machoAnalysisInfo{
		importSymbols:       syms,
		hasLibSystemLoadCmd: hasLibSystem,
		architecture:        machoCPUToArchName(mf.Cpu),
	}
}

// getMachoAnalysisInfo opens filePath as a Mach-O file, extracts imported symbols,
// and checks whether any LC_LOAD_DYLIB entry names a libSystem-family library.
// Handles both single-arch and Fat/universal binaries; for Fat binaries the native
// GOARCH slice is used (arm64 as fallback). Returns nil, nil for non-Mach-O files.
func getMachoAnalysisInfo(fs safefileio.FileSystem, filePath string) (*machoAnalysisInfo, error) {
	f, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Read the first 4 bytes to distinguish Fat binaries from single-arch Mach-O.
	// macho.NewFile and macho.NewFatFile both use io.ReaderAt (absolute offsets),
	// so sequential read position does not affect them.
	var magicBuf [4]byte
	if _, err := io.ReadFull(f, magicBuf[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, nil // file shorter than 4 bytes — not Mach-O
		}
		return nil, err
	}
	magic := binary.LittleEndian.Uint32(magicBuf[:])

	if magic == machoFatMagicLE || magic == machoFatCigamLE {
		fat, err := macho.NewFatFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Fat Mach-O: %w", err)
		}
		defer func() { _ = fat.Close() }()

		nativeCPU, err := nativeMachoCPU()
		if err != nil {
			return nil, err
		}
		slice := nativeOrArm64Slice(fat, nativeCPU)
		if slice == nil {
			// No matching slice found; treat as non-Mach-O for analysis purposes.
			return nil, nil
		}
		return extractMachoSliceInfo(slice), nil
	}

	mf, err := macho.NewFile(f)
	if err != nil {
		// Non-Mach-O file such as ELF: skip it.
		return nil, nil //nolint:nilerr // Mach-O parse failure means a non-Mach-O file
	}
	defer func() { _ = mf.Close() }()

	return extractMachoSliceInfo(mf), nil
}

// analyzeELFSyscalls performs ELF syscall analysis on the given file path and sets
// record.SyscallAnalysis. It is called from the store.Update() callback in
// updateAnalysisRecord. Always writes record.SyscallAnalysis (nil for non-ELF
// files or ELF with no detected syscalls) to clear stale values from prior runs.
// Fatal errors are returned to prevent the record from being saved.
func (v *Validator) analyzeELFSyscalls(record *fileanalysis.Record, filePath string) error {
	if v.syscallAnalyzer == nil && v.libcCache == nil {
		return nil
	}

	// Open the target binary as an ELF file; skip non-ELF files silently.
	elfFile, elfErr := openELFFile(v.fileSystem, filePath)
	if elfErr != nil {
		if errors.Is(elfErr, errNotELF) {
			record.SyscallAnalysis = nil // Non-ELF: clear any stale analysis from a previous record run.
			return nil
		}
		return fmt.Errorf("failed to open ELF file: %w", elfErr)
	}
	defer func() { _ = elfFile.Close() }()

	// Match imported symbols against the libc syscall cache.
	var libcSyscalls []common.SyscallInfo
	if v.libcCache != nil && len(record.DynLibDeps) > 0 {
		if libcEntry := findLibcEntry(record.DynLibDeps); libcEntry != nil {
			importSymbols, symErr := extractUNDSymbols(elfFile)
			if symErr != nil {
				return fmt.Errorf("failed to extract UND symbols: %w", symErr)
			}
			infos, cacheErr := v.libcCache.GetOrCreateSyscalls(libcEntry.Path, libcEntry.Hash, importSymbols, elfFile.Machine)
			if cacheErr != nil {
				if !errors.Is(cacheErr, ErrUnsupportedArch) {
					return fmt.Errorf("libc cache error: %w", cacheErr)
				}
				// ErrUnsupportedArch: skip libc cache and continue.
			} else {
				libcSyscalls = infos
			}
		}
	}

	// Scan ELF instructions directly for syscall invocations.
	var directSyscalls []common.SyscallInfo
	var directArgEvalResults []common.SyscallArgEvalResult
	var directStats *common.SyscallDeterminationStats
	if v.syscallAnalyzer != nil {
		detected, evalResults, stats, analyzeErr := v.syscallAnalyzer.AnalyzeSyscallsFromELF(elfFile)
		if analyzeErr != nil {
			if !errors.Is(analyzeErr, ErrUnsupportedArch) {
				return fmt.Errorf("syscall analysis failed: %w", analyzeErr)
			}
		} else {
			directSyscalls = detected
			directArgEvalResults = evalResults
			directStats = stats
		}
	}

	// Merge results and write SyscallAnalysis; always assign to overwrite any stale value.
	allSyscalls := mergeSyscallInfos(libcSyscalls, directSyscalls)
	argEvalResults := buildArgEvalResults(libcSyscalls, directArgEvalResults, elfFile, v.syscallAnalyzer)
	slices.SortFunc(argEvalResults, func(a, b common.SyscallArgEvalResult) int {
		if c := cmp.Compare(a.SyscallName, b.SyscallName); c != 0 {
			return c
		}
		return cmp.Compare(a.Status, b.Status)
	})
	if len(allSyscalls) > 0 || len(argEvalResults) > 0 {
		record.SyscallAnalysis = buildSyscallData(allSyscalls, argEvalResults, elfFile.Machine, directStats, v.includeDebugInfo)
	} else {
		record.SyscallAnalysis = nil
	}
	return nil
}

// elfMagicStr is the ELF magic number string literal.
const elfMagicStr = "\x7fELF"

// elfMagic is the ELF magic number bytes.
var elfMagic = []byte(elfMagicStr)

// openELFFile opens filePath via SafeOpenFile and parses it as an ELF binary.
// Returns errNotELF if the file is not an ELF binary (bad magic number or unsupported format).
// Returns other errors for I/O failures or unexpected parse errors.
// The caller is responsible for calling Close() on the returned *elf.File.
func openELFFile(fs safefileio.FileSystem, filePath string) (*elf.File, error) {
	f, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Pre-check magic bytes to detect non-ELF files without relying on elf.NewFile
	// error classification, which may change across Go versions.
	magic := make([]byte, len(elfMagic))
	if _, err := io.ReadFull(f, magic); err != nil {
		_ = f.Close()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, errNotELF
		}
		return nil, fmt.Errorf("failed to read magic bytes: %w", err)
	}
	if !bytes.Equal(magic, elfMagic) {
		_ = f.Close()
		return nil, errNotELF
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to seek file: %w", err)
	}

	elfFile, err := elf.NewFile(f)
	if err != nil {
		_ = f.Close()
		if _, ok := errors.AsType[*elf.FormatError](err); ok {
			return nil, errNotELF
		}
		return nil, fmt.Errorf("failed to parse ELF file: %w", err)
	}
	return elfFile, nil
}

// extractUNDSymbols returns the names of undefined (UND) symbols from elfFile's .dynsym section.
// If .dynsym does not exist (elf.ErrNoSymbols), returns an empty slice with no error.
// Other errors are returned as-is.
func extractUNDSymbols(elfFile *elf.File) ([]string, error) {
	syms, err := elfFile.DynamicSymbols()
	if err != nil {
		if errors.Is(err, elf.ErrNoSymbols) {
			return nil, nil
		}
		return nil, err
	}
	var result []string
	for _, s := range syms {
		if elf.ST_BIND(s.Info) == elf.STB_LOCAL {
			continue
		}
		if s.Section != elf.SHN_UNDEF {
			continue
		}
		if elf.ST_TYPE(s.Info) != elf.STT_FUNC {
			continue
		}
		result = append(result, s.Name)
	}
	return result, nil
}

// findLibcEntry returns the first LibEntry from deps whose SOName starts with "libc.so.".
// Returns nil if no such entry is found.
func findLibcEntry(deps []fileanalysis.LibEntry) *fileanalysis.LibEntry {
	for i := range deps {
		if strings.HasPrefix(deps[i].SOName, "libc.so.") {
			return &deps[i]
		}
	}
	return nil
}

// mergeSyscallInfos merges libc-derived and direct syscall infos into a single slice.
// Entries with the same Number are grouped together and their Occurrences are merged.
// A non-empty Name is preferred over an empty one; IsNetwork is true if any entry has it set.
func mergeSyscallInfos(libc, direct []common.SyscallInfo) []common.SyscallInfo {
	combined := make([]common.SyscallInfo, 0, len(libc)+len(direct))
	combined = append(combined, libc...)
	combined = append(combined, direct...)
	return common.GroupAndSortSyscalls(combined)
}

// elfArchName converts an elf.Machine to the architecture name string used in records.
// Returns the elf.Machine's String() representation if the machine is not recognized.
func elfArchName(machine elf.Machine) string {
	switch machine {
	case elf.EM_X86_64:
		return "x86_64"
	case elf.EM_AARCH64:
		return archNameArm64
	default:
		return machine.String()
	}
}

// syscallNameMprotect is the canonical syscall name used as a key throughout
// libc-import and PLT analysis for the mprotect syscall.
const syscallNameMprotect = "mprotect"

// mprotectStatusPriorityExecConfirmed is the highest risk priority for mprotect status evaluation.
const mprotectStatusPriorityExecConfirmed = 2

// buildArgEvalResults merges libc-import mprotect detection with direct analysis ArgEvalResults.
// When mprotect appears in libcSyscalls, it evaluates the PLT call sites and picks the
// highest-risk result between the direct-syscall result (if any) and the PLT result.
// Falls back to exec_unknown when PLT analysis finds no call sites or is unavailable.
func buildArgEvalResults(
	libcSyscalls []common.SyscallInfo,
	directArgEvalResults []common.SyscallArgEvalResult,
	elfFile *elf.File,
	analyzer SyscallAnalyzerInterface,
) []common.SyscallArgEvalResult {
	// Check if mprotect is present in libc import syscalls.
	hasMprotect := false
	for _, s := range libcSyscalls {
		if s.Name == syscallNameMprotect {
			hasMprotect = true
			break
		}
	}
	if !hasMprotect {
		return directArgEvalResults
	}

	// Find direct mprotect result (if any).
	var directMprotect *common.SyscallArgEvalResult
	for i := range directArgEvalResults {
		if directArgEvalResults[i].SyscallName == syscallNameMprotect {
			directMprotect = &directArgEvalResults[i]
			break
		}
	}

	// mprotect is imported via libc. Try to determine the prot argument by
	// backward-scanning each PLT call site in the binary.
	pltResult := common.SyscallArgEvalResult{
		SyscallName: syscallNameMprotect,
		Status:      common.SyscallArgEvalExecUnknown,
		Details:     "called via libc wrapper (prot argument not statically determinable)",
	}
	if analyzer != nil && elfFile != nil {
		result, err := analyzer.EvaluatePLTCallArgs(elfFile, syscallNameMprotect)
		if err != nil {
			if errors.Is(err, ErrUnsupportedArch) {
				pltResult.Details = fmt.Sprintf("%s (PLT analysis unsupported for this architecture)", pltResult.Details)
			} else {
				pltResult.Details = fmt.Sprintf("%s (PLT analysis failed: %v)", pltResult.Details, err)
			}
		} else if result != nil {
			pltResult = *result
		}
	}

	// Pick the highest-risk mprotect result between direct and PLT.
	bestMprotect := pltResult
	if directMprotect != nil && mprotectStatusPriority(directMprotect.Status) > mprotectStatusPriority(pltResult.Status) {
		bestMprotect = *directMprotect
	}

	// Return non-mprotect direct results plus the best mprotect result.
	result := make([]common.SyscallArgEvalResult, 0, len(directArgEvalResults))
	for _, r := range directArgEvalResults {
		if r.SyscallName != syscallNameMprotect {
			result = append(result, r)
		}
	}
	return append(result, bestMprotect)
}

// mprotectStatusPriority returns the risk priority of a SyscallArgEvalStatus.
// Higher value means higher risk.
func mprotectStatusPriority(status common.SyscallArgEvalStatus) int {
	switch status {
	case common.SyscallArgEvalExecConfirmed:
		return mprotectStatusPriorityExecConfirmed
	case common.SyscallArgEvalExecUnknown:
		return mprotectStatusPriorityExecConfirmed - 1
	default:
		return 0
	}
}

// stripOccurrences returns a copy of syscalls with Occurrences removed from each entry.
func stripOccurrences(syscalls []common.SyscallInfo) []common.SyscallInfo {
	result := make([]common.SyscallInfo, len(syscalls))
	for i, s := range syscalls {
		result[i] = s
		result[i].Occurrences = nil
	}
	return result
}

// buildSyscallData constructs a SyscallAnalysisData from the merged syscall infos.
func buildSyscallData(
	all []common.SyscallInfo,
	argEvalResults []common.SyscallArgEvalResult,
	machine elf.Machine,
	stats *common.SyscallDeterminationStats,
	includeDebugInfo bool,
) *fileanalysis.SyscallAnalysisData {
	syscalls := all
	if !includeDebugInfo {
		syscalls = stripOccurrences(all)
		stats = nil
	}

	return &fileanalysis.SyscallAnalysisData{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture:       elfArchName(machine),
			DetectedSyscalls:   syscalls,
			ArgEvalResults:     argEvalResults,
			DeterminationStats: stats,
		},
	}
}
