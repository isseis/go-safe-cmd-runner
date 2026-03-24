package filevalidator

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlibanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
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
	// Returns detected syscalls and argument evaluation results (e.g., mprotect PROT_EXEC).
	// Returns an error wrapping ErrUnsupportedArch (detectable via errors.Is) for
	// unsupported architectures.
	AnalyzeSyscallsFromELF(elfFile *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, error)
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
	ErrPrivilegeManagerNotAvailable    = errors.New("privilege manager not available")
	ErrPrivilegedExecutionNotSupported = errors.New("privileged execution not supported")

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
	VerifyWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) error
	VerifyAndRead(filePath string) ([]byte, error)
	VerifyAndReadWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) ([]byte, error)
	// LoadRecord returns the full analysis record for the given file path.
	// Used by verification.Manager to access DynLibDeps without exposing the store directly.
	LoadRecord(filePath string) (*fileanalysis.Record, error)
}

// GetHashFilePath returns the path where the hash for the given file would be stored.
func (v *Validator) GetHashFilePath(filePath common.ResolvedPath) (string, error) {
	return v.hashFilePathGetter.GetHashFilePath(v.hashDir, filePath)
}

// GetStore returns the underlying fileanalysis.Store.
// This is useful for accessing syscall analysis results stored alongside hashes.
func (v *Validator) GetStore() *fileanalysis.Store {
	return v.store
}

// Validator provides functionality to record and verify file hashes.
// It should be instantiated using the New function.
type Validator struct {
	algorithm               HashAlgorithm
	hashDir                 string
	hashFilePathGetter      common.HashFilePathGetter
	privilegedFileValidator *PrivilegedFileValidator

	// store is the unified analysis store for FileAnalysisRecord format.
	store *fileanalysis.Store

	fileSystem      safefileio.FileSystem          // used by openELFFile in analyzeSyscalls
	dynlibAnalyzer  *dynlibanalysis.DynLibAnalyzer // nil if dynlib analysis is disabled
	binaryAnalyzer  binaryanalyzer.BinaryAnalyzer  // nil if binary analysis is disabled
	libcCache       LibcCacheInterface             // nil if libc cache is disabled
	syscallAnalyzer SyscallAnalyzerInterface       // nil if syscall analysis is disabled
}

// New initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
// The hash directory is created automatically if it does not exist.
// This constructor uses the FileAnalysisRecord format for storing hash and analysis results.
// The analysis store preserves existing fields (e.g., SyscallAnalysis) when updating hashes.
func New(algorithm HashAlgorithm, hashDir string) (*Validator, error) {
	// Resolve to absolute path early, consistent with newValidator behavior.
	var err error
	hashDir, err = filepath.Abs(hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for hash directory: %w", err)
	}

	hashFilePathGetter := NewHybridHashFilePathGetter()

	// Create analysis store first — this creates the directory if it doesn't exist.
	store, err := fileanalysis.NewStore(hashDir, hashFilePathGetter)
	if err != nil {
		return nil, fmt.Errorf("failed to create analysis store: %w", err)
	}

	// Now create the validator — the directory is guaranteed to exist.
	v, err := newValidator(algorithm, hashDir, hashFilePathGetter)
	if err != nil {
		return nil, err
	}
	v.store = store

	return v, nil
}

// newValidator initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
func newValidator(algorithm HashAlgorithm, hashDir string, hashFilePathGetter common.HashFilePathGetter) (*Validator, error) {
	if algorithm == nil {
		return nil, ErrNilAlgorithm
	}

	// Ensure the hash directory exists and is a directory
	info, err := os.Lstat(hashDir)
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
		algorithm:               algorithm,
		hashDir:                 hashDir,
		hashFilePathGetter:      hashFilePathGetter,
		privilegedFileValidator: DefaultPrivilegedFileValidator(),
		fileSystem:              safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
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

	// Calculate the hash of the file
	hash, err := v.calculateHash(targetPath.String())
	if err != nil {
		return "", "", fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Get the path for the hash file
	hashFilePath, err := v.GetHashFilePath(targetPath)
	if err != nil {
		return "", "", err
	}

	contentHash, err := v.updateAnalysisRecord(targetPath, hash, force)
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
func (v *Validator) updateAnalysisRecord(filePath common.ResolvedPath, hash string, force bool) (string, error) {
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

		// Analyze dynamic library dependencies if analyzer is available.
		// Analysis failure causes the callback to return an error, which
		// prevents the record from being persisted (atomicity).
		if v.dynlibAnalyzer != nil {
			dynLibDeps, analyzeErr := v.dynlibAnalyzer.Analyze(filePath.String())
			if analyzeErr != nil {
				return fmt.Errorf("dynamic library analysis failed: %w", analyzeErr)
			}
			record.DynLibDeps = dynLibDeps // nil for non-ELF or static ELF (omitted in JSON)
		}

		// Analyze binary symbols if analyzer is available.
		// Stores the result as SymbolAnalysis in the record.
		if v.binaryAnalyzer != nil {
			output := v.binaryAnalyzer.AnalyzeNetworkSymbols(filePath.String(), contentHash)
			switch output.Result {
			case binaryanalyzer.NetworkDetected, binaryanalyzer.NoNetworkSymbols:
				record.SymbolAnalysis = &fileanalysis.SymbolAnalysisData{
					AnalyzedAt:         time.Now().UTC(),
					DetectedSymbols:    convertDetectedSymbols(output.DetectedSymbols),
					DynamicLoadSymbols: convertDetectedSymbols(output.DynamicLoadSymbols),
				}
			case binaryanalyzer.StaticBinary, binaryanalyzer.NotSupportedBinary:
				// Static binary or unsupported format: clear any previously stored
				// SymbolAnalysis to prevent stale data from an earlier record run.
				record.SymbolAnalysis = nil
			case binaryanalyzer.AnalysisError:
				return fmt.Errorf("network symbol analysis failed: %w", output.Error)
			}
		}

		// Steps A-D: ELF syscall analysis (libc import + direct instruction).
		if err := v.analyzeSyscalls(record, filePath.String()); err != nil {
			return err
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

// SetDynLibAnalyzer injects the DynLibAnalyzer used during record operations.
// Call before the first SaveRecord() invocation. Safe to call with nil (disables dynlib analysis).
func (v *Validator) SetDynLibAnalyzer(a *dynlibanalysis.DynLibAnalyzer) {
	v.dynlibAnalyzer = a
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

// Verify checks if the file at filePath matches its recorded hash.
// Returns ErrMismatch if the hashes don't match, or ErrHashFileNotFound if no hash is recorded.
func (v *Validator) Verify(filePath string) error {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return err
	}

	// Calculate the current hash
	actualHash, err := v.calculateHash(targetPath.String())
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

	actualHash, err := v.calculateHash(targetPath.String())
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
	if filePath == "" {
		return "", safefileio.ErrInvalidFilePath
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", err
	}
	// check if resolvedPath is a regular file
	fileInfo, err := os.Lstat(resolvedPath)
	if err != nil {
		return "", err
	}
	if !fileInfo.Mode().IsRegular() {
		return "", fmt.Errorf("%w: not a regular file: %s", safefileio.ErrInvalidFilePath, resolvedPath)
	}

	return common.NewResolvedPath(resolvedPath)
}

// calculateHash calculates the hash of the file at the given path.
// filePath must be validated by validatePath before calling this function.
func (v *Validator) calculateHash(filePath string) (string, error) {
	content, err := safefileio.SafeReadFile(filePath)
	if err != nil {
		return "", err
	}
	return v.algorithm.Sum(bytes.NewReader(content))
}

// VerifyFromHandle verifies a file's hash using an already opened file handle.
// The file parameter must implement io.ReadSeeker (satisfied by *os.File and safefileio.File).
func (v *Validator) VerifyFromHandle(file io.ReadSeeker, targetPath common.ResolvedPath) error {
	// Calculate hash directly from file handle (normal privilege)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek file to start: %w", err)
	}
	actualHash, err := v.algorithm.Sum(file)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	return v.verifyHash(targetPath, actualHash)
}

// VerifyWithPrivileges verifies a file's integrity using privilege escalation
// This method assumes that normal verification has already failed with a permission error
func (v *Validator) VerifyWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) error {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return err
	}

	// Check if privilege manager is available
	if privManager == nil {
		return fmt.Errorf("failed to verify file %s: %w", targetPath, ErrPrivilegeManagerNotAvailable)
	}

	// Check if privilege escalation is supported
	if !privManager.IsPrivilegedExecutionSupported() {
		return fmt.Errorf("failed to verify file %s: %w", targetPath, ErrPrivilegedExecutionNotSupported)
	}

	// Open file with privileges
	file, openErr := v.privilegedFileValidator.OpenFileWithPrivileges(targetPath.String(), privManager)
	if openErr != nil {
		return fmt.Errorf("failed to open file with privileges: %w", openErr)
	}
	defer func() {
		_ = file.Close() // Ignore close error
	}()

	// Verify using the opened file handle
	return v.VerifyFromHandle(file, targetPath)
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
		content, err := safefileio.SafeReadFile(targetPath.String())
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return content, nil
	})
}

// VerifyAndReadWithPrivileges atomically verifies file integrity and returns its content using privileged access
func (v *Validator) VerifyAndReadWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) ([]byte, error) {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return nil, err
	}

	// Check if privilege manager is available
	if privManager == nil {
		return nil, fmt.Errorf("failed to verify and read file %s: %w", targetPath, ErrPrivilegeManagerNotAvailable)
	}

	// Check if privilege escalation is supported
	if !privManager.IsPrivilegedExecutionSupported() {
		return nil, fmt.Errorf("failed to verify and read file %s: %w", targetPath, ErrPrivilegedExecutionNotSupported)
	}

	// Use common verification logic with privileged file reading
	return v.verifyAndReadContent(targetPath, func() ([]byte, error) {
		// Open file with privileges
		file, openErr := v.privilegedFileValidator.OpenFileWithPrivileges(targetPath.String(), privManager)
		if openErr != nil {
			return nil, fmt.Errorf("failed to open file with privileges: %w", openErr)
		}
		defer func() {
			_ = file.Close() // Ignore close error
		}()

		// Read content from the opened file handle
		content, err := io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file content: %w", err)
		}
		return content, nil
	})
}

// convertDetectedSymbols converts binaryanalyzer.DetectedSymbol slice to fileanalysis.DetectedSymbolEntry slice.
// Returns nil for empty input to keep JSON output clean with omitempty.
//
// NOTE: This is the inverse of convertNetworkSymbolEntries in
// internal/runner/security/network_analyzer.go. Both functions map the same
// two fields (Name, Category) between binaryanalyzer and fileanalysis types.
// If either type gains or loses fields, update both functions together.
func convertDetectedSymbols(syms []binaryanalyzer.DetectedSymbol) []fileanalysis.DetectedSymbolEntry {
	if len(syms) == 0 {
		return nil
	}
	entries := make([]fileanalysis.DetectedSymbolEntry, len(syms))
	for i, s := range syms {
		entries[i] = fileanalysis.DetectedSymbolEntry{Name: s.Name, Category: s.Category}
	}
	return entries
}

// analyzeSyscalls performs ELF syscall analysis on the given file path and sets
// record.SyscallAnalysis. It is called from the store.Update() callback in
// updateAnalysisRecord. Always writes record.SyscallAnalysis (nil for non-ELF
// files or ELF with no detected syscalls) to clear stale values from prior runs.
// Fatal errors are returned to prevent the record from being saved.
func (v *Validator) analyzeSyscalls(record *fileanalysis.Record, filePath string) error {
	if v.syscallAnalyzer == nil && v.libcCache == nil {
		return nil
	}

	// Step A: Open the target binary as an ELF file.
	elfFile, elfErr := openELFFile(v.fileSystem, filePath)
	if elfErr != nil {
		if errors.Is(elfErr, errNotELF) {
			record.SyscallAnalysis = nil // Non-ELF: clear any stale analysis from a previous record run.
			return nil
		}
		return fmt.Errorf("failed to open ELF file: %w", elfErr)
	}
	defer func() { _ = elfFile.Close() }()

	// Step B: libc import symbol matching via cache.
	var libcSyscalls []common.SyscallInfo
	if v.libcCache != nil && record.DynLibDeps != nil {
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

	// Step C: Direct syscall instruction analysis.
	var directSyscalls []common.SyscallInfo
	var directArgEvalResults []common.SyscallArgEvalResult
	if v.syscallAnalyzer != nil {
		detected, evalResults, analyzeErr := v.syscallAnalyzer.AnalyzeSyscallsFromELF(elfFile)
		if analyzeErr != nil {
			if !errors.Is(analyzeErr, ErrUnsupportedArch) {
				return fmt.Errorf("syscall analysis failed: %w", analyzeErr)
			}
		} else {
			directSyscalls = detected
			directArgEvalResults = evalResults
		}
	}

	// Step D: Merge and set SyscallAnalysis.
	// Always assign (including nil) to overwrite any stale value from a previous record run.
	allSyscalls := mergeSyscallInfos(libcSyscalls, directSyscalls)
	argEvalResults := buildArgEvalResults(libcSyscalls, directArgEvalResults, elfFile, v.syscallAnalyzer)
	if len(allSyscalls) > 0 || len(argEvalResults) > 0 {
		record.SyscallAnalysis = buildSyscallAnalysisData(allSyscalls, directSyscalls, argEvalResults, elfFile.Machine)
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
func findLibcEntry(deps *fileanalysis.DynLibDepsData) *fileanalysis.LibEntry {
	for i := range deps.Libs {
		if strings.HasPrefix(deps.Libs[i].SOName, "libc.so.") {
			return &deps.Libs[i]
		}
	}
	return nil
}

// mergeSyscallInfos merges libc-derived and direct syscall infos into a single slice.
// When the same Number appears in both, the direct entry (Source == "") takes priority.
func mergeSyscallInfos(libc, direct []common.SyscallInfo) []common.SyscallInfo {
	// Build a map keyed by Number, direct entries override libc entries.
	merged := make(map[int]common.SyscallInfo)
	for _, info := range libc {
		merged[info.Number] = info
	}
	for _, info := range direct {
		merged[info.Number] = info
	}
	result := make([]common.SyscallInfo, 0, len(merged))
	for _, info := range merged {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Number < result[j].Number })
	return result
}

// elfMachineToArchName converts an elf.Machine to the architecture name string used in records.
// Returns the elf.Machine's String() representation if the machine is not recognized.
func elfMachineToArchName(machine elf.Machine) string {
	switch machine {
	case elf.EM_X86_64:
		return "x86_64"
	case elf.EM_AARCH64:
		return "arm64"
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
		if err == nil && result != nil {
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

// buildSyscallAnalysisData constructs a SyscallAnalysisData from the merged syscall infos.
// HasUnknownSyscalls is determined by whether any direct (Source == "") entry has Number < 0.
func buildSyscallAnalysisData(all []common.SyscallInfo, direct []common.SyscallInfo, argEvalResults []common.SyscallArgEvalResult, machine elf.Machine) *fileanalysis.SyscallAnalysisData {
	hasUnknown := false
	for _, info := range direct {
		if info.Number < 0 {
			hasUnknown = true
			break
		}
	}

	var hasNetwork bool
	var networkCount int
	for _, info := range all {
		if info.IsNetwork {
			hasNetwork = true
			networkCount++
		}
	}

	retained := fileanalysis.FilterSyscallsForStorage(all)

	return &fileanalysis.SyscallAnalysisData{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture:       elfMachineToArchName(machine),
			DetectedSyscalls:   retained,
			HasUnknownSyscalls: hasUnknown,
			ArgEvalResults:     argEvalResults,
			Summary: common.SyscallSummary{
				HasNetworkSyscalls:  hasNetwork,
				TotalDetectedEvents: len(retained),
				NetworkSyscallCount: networkCount,
			},
		},
		AnalyzedAt: time.Now().UTC(),
	}
}
