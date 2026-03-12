# ELF 動的リンクライブラリ整合性検証 詳細仕様書

## 1. 概要

### 1.1 目的

本仕様書は、ELF バイナリの動的リンクライブラリの整合性検証機能（方策 A）および `dlopen` シンボル検出機能（方策 B）の詳細実装設計を定義する。

### 1.2 設計範囲

- `fileanalysis` パッケージのスキーマ拡張（`DynLibDeps`, `HasDynamicLoad`, `CurrentSchemaVersion` 変更）
- `dynlibanalysis` パッケージの新規実装（ライブラリ解決エンジン、`ld.so.cache` パーサー、解析・検証コンポーネント）
- `binaryanalyzer` パッケージの拡張（`dynamicLoadSymbolRegistry`, `HasDynamicLoad` フラグ）
- `filevalidator` パッケージの拡張（`DynLibAnalyzer` 注入、`LoadRecord` メソッド追加）
- `verification` パッケージの拡張（`verifyDynLibDeps` 追加）
- `runner/security` パッケージの拡張（`HasDynamicLoad` による高リスク判定）
- `record` コマンドの拡張

## 2. パッケージ構成

### 2.1 ディレクトリ構造

```
cmd/
└── record/
    └── main.go                            # DynLibAnalyzer 統合

internal/
├── fileanalysis/
│   └── schema.go                          # Record 拡張、型定義、CurrentSchemaVersion 2
│
├── dynlibanalysis/                        # NEW: 動的ライブラリ解析パッケージ
│   ├── analyzer.go                        # DynLibAnalyzer 構造体・コンストラクタ
│   ├── resolver.go                        # LibraryResolver: DT_NEEDED → フルパス解決
│   ├── ldcache.go                         # ld.so.cache パーサー
│   ├── default_paths.go                   # アーキテクチャ別デフォルト検索パス
│   ├── verifier.go                        # DynLibVerifier: runner 実行時の検証
│   ├── errors.go                          # エラー型定義
│   ├── doc.go                             # パッケージドキュメント
│   ├── analyzer_test.go                   # DynLibAnalyzer テスト
│   ├── resolver_test.go                   # LibraryResolver テスト
│   ├── ldcache_test.go                    # ld.so.cache パーサーテスト
│   ├── default_paths_test.go              # デフォルトパステスト
│   ├── verifier_test.go                   # DynLibVerifier テスト
│   └── testdata/
│       ├── README.md                      # テストデータの説明と生成方法
│       └── ldcache_new_format.bin         # 最小構成の ld.so.cache テストデータ
│
├── filevalidator/
│   └── validator.go                       # DynLibAnalyzer 注入、LoadRecord 追加
│
├── verification/
│   └── manager.go                         # verifyDynLibDeps 追加
│
├── runner/
│   └── security/
│       ├── network_analyzer.go            # HasDynamicLoad による高リスク判定追加
│       ├── binaryanalyzer/
│       │   ├── network_symbols.go         # dynamicLoadSymbolRegistry 追加
│       │   └── analyzer.go                # AnalysisOutput.HasDynamicLoad 追加
│       ├── elfanalyzer/
│       │   └── standard_analyzer.go       # HasDynamicLoad 検出ロジック追加
│       └── machoanalyzer/
│           └── standard_analyzer.go       # HasDynamicLoad 検出ロジック追加
```

## 3. 型定義

### 3.1 `fileanalysis/schema.go` — スキーマ拡張

```go
// internal/fileanalysis/schema.go

const (
    // CurrentSchemaVersion is the expected schema version for analysis records.
    // Version 2 adds DynLibDeps and HasDynamicLoad fields. DT_RPATH は非サポートのため
    // DT_RPATH を持つ ELF を record すると ErrDTRPATHNotSupported が返る。
    // Load returns SchemaVersionMismatchError for records with schema_version != 2.
    // Store.Update treats older schemas (Actual < Expected) as overwritable (enables record --force migration).
    // Store.Update rejects newer schemas (Actual > Expected) to preserve forward compatibility.
    CurrentSchemaVersion = 2 // Changed from 1 to 2
)

// Record contains the analysis record for a file.
type Record struct {
    SchemaVersion   int                  `json:"schema_version"`
    FilePath        string               `json:"file_path"`
    ContentHash     string               `json:"content_hash"`
    UpdatedAt       time.Time            `json:"updated_at"`
    SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
    // DynLibDeps contains the dynamic library dependency snapshot recorded at record time.
    // Only present for ELF binaries with DT_NEEDED entries.
    DynLibDeps *DynLibDepsData `json:"dyn_lib_deps,omitempty"`
    // HasDynamicLoad indicates that dlopen/dlsym/dlvsym symbols were found in the binary
    // at record time. Stored as an informational snapshot only; the runner does NOT read
    // this field for its high-risk determination. Instead, isNetworkViaBinaryAnalysis()
    // re-runs AnalyzeNetworkSymbols() on the binary at runtime. This is consistent with
    // the existing design of isNetworkViaBinaryAnalysis(), which is a fallback path for
    // commands not found in security profiles and always performs live binary analysis
    // (not record-time cached results). Changing HasDynamicLoad to use the stored value
    // would create an inconsistency within that flow and is out of scope for this task.
    HasDynamicLoad bool `json:"has_dynamic_load,omitempty"`
}

// DynLibDepsData contains the dynamic library dependency snapshot.
type DynLibDepsData struct {
    RecordedAt time.Time  `json:"recorded_at"`
    Libs       []LibEntry `json:"libs"`
}

// LibEntry represents a single resolved dynamic library dependency.
type LibEntry struct {
    // SOName is the DT_NEEDED library name (e.g., "libssl.so.3").
    SOName string `json:"soname"`

    // Path is the resolved full path to the library file, normalized via
    // filepath.EvalSymlinks + filepath.Clean.
    Path string `json:"path"`

    // Hash is the SHA256 hash of the library file in "sha256:<hex>" format.
    Hash string `json:"hash"`
}
```

### 3.1b `fileanalysis/file_analysis_store.go` — `Store.Update` 旧スキーマ上書き許可

`CurrentSchemaVersion` を旧バージョンから 2 に変更すると、`Store.Update` が既存の旧スキーマレコードに対して `SchemaVersionMismatchError` を返し上書きを拒否するため、`record --force` による移行が不可能になる。

`SchemaVersionMismatchError` の `Actual` と `Expected` を比較して処理を分岐する:

```go
// internal/fileanalysis/file_analysis_store.go

// Update performs a read-modify-write operation on the analysis record.
//
// Error Handling:
//   - ErrRecordNotFound: creates a new record
//   - RecordCorruptedError: creates a new record (overwriting corrupted data)
//   - SchemaVersionMismatchError (Actual < Expected): old schema; treat as not found,
//     allow overwrite so that `record --force` can migrate records to the current version
//   - SchemaVersionMismatchError (Actual > Expected): future schema written by a newer
//     binary; refuse to overwrite to preserve forward compatibility
func (s *Store) Update(filePath common.ResolvedPath, updateFn func(*Record) error) error {
    record, err := s.Load(filePath)
    if err != nil {
        var schemaErr *SchemaVersionMismatchError
        if errors.As(err, &schemaErr) {
            if schemaErr.Actual > schemaErr.Expected {
                // Future schema: do NOT overwrite (forward compatibility protection)
                return fmt.Errorf("cannot update record: %w", err)
            }
            // Older schema (e.g. v1 record, current binary expects v2):
            // allow --force to overwrite by treating it as a fresh record.
            record = &Record{}
        } else if errors.Is(err, ErrRecordNotFound) || errors.As(err, new(*RecordCorruptedError)) {
            record = &Record{}
        } else {
            return fmt.Errorf("failed to load existing record: %w", err)
        }
    }
    // ... rest unchanged
}
```

### 3.2 `dynlibanalysis/errors.go` — エラー型定義

```go
// Package dynlibanalysis provides dynamic library analysis for ELF binaries.
package dynlibanalysis

import (
    "fmt"
    "strings"
)

// ErrLibraryNotResolved indicates that a DT_NEEDED library could not be resolved
// to a filesystem path through any of the available search methods.
type ErrLibraryNotResolved struct {
    SOName      string
    ParentPath  string
    SearchPaths []string // all paths that were tried
}

func (e *ErrLibraryNotResolved) Error() string {
    var sb strings.Builder
    fmt.Fprintf(&sb, "failed to resolve dynamic library: %s\n", e.SOName)
    fmt.Fprintf(&sb, "  parent: %s\n", e.ParentPath)
    sb.WriteString("  searched paths:\n")
    for _, p := range e.SearchPaths {
        fmt.Fprintf(&sb, "    - %s\n", p)
    }
    return sb.String()
}

// ErrRecursionDepthExceeded indicates that dependency resolution exceeded the
// maximum allowed depth. This typically indicates an abnormal library configuration
// or a missed circular dependency.
type ErrRecursionDepthExceeded struct {
    Depth    int
    MaxDepth int
    SOName   string
}

func (e *ErrRecursionDepthExceeded) Error() string {
    return fmt.Sprintf("dependency resolution depth exceeded: %s at depth %d (max %d)",
        e.SOName, e.Depth, e.MaxDepth)
}

// ErrLibraryHashMismatch indicates that a library's hash does not match the recorded value.
type ErrLibraryHashMismatch struct {
    SOName       string
    Path         string
    ExpectedHash string
    ActualHash   string
}

func (e *ErrLibraryHashMismatch) Error() string {
    return fmt.Sprintf("dynamic library hash mismatch: %s\n"+
        "  path: %s\n"+
        "  expected hash: %s\n"+
        "  actual hash: %s\n"+
        "  please re-run 'record' command",
        e.SOName, e.Path, e.ExpectedHash, e.ActualHash)
}

// ErrEmptyLibraryPath indicates that a LibEntry has an empty path,
// which should never happen in valid records (defensive check).
type ErrEmptyLibraryPath struct {
    SOName string
}

func (e *ErrEmptyLibraryPath) Error() string {
    return fmt.Sprintf("incomplete record: empty path for library %s\n"+
        "  please re-run 'record' command",
        e.SOName)
}

// ErrDynLibDepsRequired indicates that a DynLibDeps record is required
// but not present for an ELF binary.
type ErrDynLibDepsRequired struct {
    BinaryPath string
}

func (e *ErrDynLibDepsRequired) Error() string {
    return fmt.Sprintf("dynamic library dependencies not recorded for ELF binary: %s\n"+
        "  please re-run 'record' command",
        e.BinaryPath)
}

// ErrDTRPATHNotSupported indicates that a DT_RPATH entry was found in the ELF binary
// or one of its dependencies. DT_RPATH is not supported because its inheritance
// semantics are complex and the feature is deprecated in modern linkers.
// Use DT_RUNPATH instead.
type ErrDTRPATHNotSupported struct {
    Path  string
    RPATH string
}

func (e *ErrDTRPATHNotSupported) Error() string {
    return fmt.Sprintf("DT_RPATH is not supported: %s\n"+
        "  rpath: %s\n"+
        "  use DT_RUNPATH (--enable-new-dtags linker flag) instead",
        e.Path, e.RPATH)
}
```

### 3.3 RUNPATH コンテキスト管理（実装メモ）

当初は `resolver_context.go` に `ResolveContext` 構造体を設ける設計だったが、実装時にインライン化された。`resolver_context.go` は存在せず、`resolver.go` の `Resolve` メソッドが `parentPath string` と `runpath []string` を直接引数として受け取る。`LD_LIBRARY_PATH` は `record`・`runner` ともに使用しないため `IncludeLDLibraryPath` フラグも不要となった。

RUNPATH の非継承は BFS キュー内の各 `resolveItem` が `parentPath` と `runpath`（= その親自身の `DT_RUNPATH`）のみを保持することで自然に実現されている（§3.7 参照）。

### 3.4 `dynlibanalysis/resolver.go` — ライブラリパス解決エンジン

`ResolveContext` はインライン化され、`Resolve` は `parentPath string` と `runpath []string` を直接受け取る。`LD_LIBRARY_PATH` は `record`・`runner` ともに不使用のため除去済み。解決順序:

1. `runpath`（`$ORIGIN` → `filepath.Dir(parentPath)`）
2. `/etc/ld.so.cache`
3. デフォルトパス（アーキテクチャ依存）

```go
// Resolve resolves a DT_NEEDED soname to a filesystem path.
// DT_RPATH は非サポート（ErrDTRPATHNotSupported を参照）。
// LD_LIBRARY_PATH は record・runner ともに使用しない。
func (r *LibraryResolver) Resolve(soname string, parentPath string, runpath []string) (string, error) {
    // Step 1: RUNPATH ($ORIGIN -> filepath.Dir(parentPath))
    // Step 2: ld.so.cache
    // Step 3: Default paths
}
```

### 3.5 `dynlibanalysis/ldcache.go` — `/etc/ld.so.cache` パーサー

```go
package dynlibanalysis

import (
    "bytes"
    "encoding/binary"
    "fmt"
    "log/slog"
    "os"
)

const (
    defaultLDCachePath = "/etc/ld.so.cache"

    // cachemagicNew is the magic string for the new format of ld.so.cache.
    // Only this format is supported; the old "ld.so-1.7.0" format is not.
    cachemagicNew    = "glibc-ld.so.cache1.1"
    cachemagicNewLen = 19 // length of cachemagicNew without null terminator

    // newEntrySize is the size of a single cache entry in the new format.
    newEntrySize = 24 // flags(4) + key_offset(4) + value_offset(4) + osversion(4) + hwcap(8)

    // headerPadding is the number of unused uint32 fields in the header.
    headerPadding = 5
)

// LDCache represents a parsed /etc/ld.so.cache file.
type LDCache struct {
    entries map[string]string // soname -> resolved path
}

// newCacheHeader is the header structure of the new ld.so.cache format.
type newCacheHeader struct {
    // NLibs is the number of library entries.
    NLibs uint32
    // LenStrings is the total size of the string table in bytes.
    LenStrings uint32
    // Unused contains reserved fields.
    Unused [headerPadding]uint32
}

// newCacheEntry is a single entry in the new ld.so.cache format.
type newCacheEntry struct {
    Flags       int32
    KeyOffset   uint32
    ValueOffset uint32
    OSVersion   uint32
    HWCap       uint64
}

// ParseLDCache parses the /etc/ld.so.cache binary file.
// Only the new format ("glibc-ld.so.cache1.1") is supported.
// Returns (nil, error) if the cache is unavailable or uses an unsupported format.
// The caller should treat a nil cache as "cache unavailable" and proceed with
// default path fallback.
func ParseLDCache(path string) (*LDCache, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            slog.Warn("ld.so.cache not found, falling back to default paths",
                "path", path)
            return nil, fmt.Errorf("ld.so.cache not found: %w", err)
        }
        slog.Warn("failed to read ld.so.cache, falling back to default paths",
            "path", path, "error", err)
        return nil, fmt.Errorf("failed to read ld.so.cache: %w", err)
    }

    return parseLDCacheData(data)
}

// parseLDCacheData parses the raw bytes of an ld.so.cache file.
// Unexported; tests within the same package access it directly with synthetic cache data.
func parseLDCacheData(data []byte) (*LDCache, error) {
    // Check for new format magic
    if len(data) < cachemagicNewLen {
        return nil, fmt.Errorf("ld.so.cache too small: %d bytes", len(data))
    }

    // The new format may appear either at the beginning of the file or
    // after the old format header. Search for the magic string.
    newStart := bytes.Index(data, []byte(cachemagicNew))
    if newStart < 0 {
        slog.Warn("unsupported ld.so.cache format, falling back to default paths",
            "format", fmt.Sprintf("%q", data[:min(20, len(data))]))
        return nil, fmt.Errorf("unsupported ld.so.cache format")
    }

    // Skip the magic string (padded to align with the header)
    headerStart := newStart + cachemagicNewLen
    // Align to next uint32 boundary
    if headerStart%4 != 0 {
        headerStart += 4 - (headerStart % 4)
    }

    // Parse header
    if len(data) < headerStart+int(binary.Size(newCacheHeader{})) {
        return nil, fmt.Errorf("ld.so.cache header truncated")
    }

    var header newCacheHeader
    reader := bytes.NewReader(data[headerStart:])
    if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
        return nil, fmt.Errorf("failed to read ld.so.cache header: %w", err)
    }

    // Calculate offsets
    entryStart := headerStart + int(binary.Size(header))
    stringTableStart := entryStart + int(header.NLibs)*newEntrySize

    if len(data) < stringTableStart+int(header.LenStrings) {
        return nil, fmt.Errorf("ld.so.cache data truncated: need %d bytes, have %d",
            stringTableStart+int(header.LenStrings), len(data))
    }

    // Parse entries
    cache := &LDCache{
        entries: make(map[string]string, header.NLibs),
    }

    for i := uint32(0); i < header.NLibs; i++ {
        offset := entryStart + int(i)*newEntrySize
        entryReader := bytes.NewReader(data[offset:])

        var entry newCacheEntry
        if err := binary.Read(entryReader, binary.LittleEndian, &entry); err != nil {
            return nil, fmt.Errorf("failed to read cache entry %d: %w", i, err)
        }

        // Extract strings from string table
        keyStart := stringTableStart + int(entry.KeyOffset)
        valueStart := stringTableStart + int(entry.ValueOffset)

        key := extractCString(data, keyStart)
        value := extractCString(data, valueStart)

        if key != "" && value != "" {
            // First entry wins (consistent with ld.so behavior)
            if _, exists := cache.entries[key]; !exists {
                cache.entries[key] = value
            }
        }
    }

    return cache, nil
}

// Lookup returns the resolved path for the given soname.
// Returns empty string if not found.
func (c *LDCache) Lookup(soname string) string {
    if c == nil {
        return ""
    }
    return c.entries[soname]
}

// extractCString extracts a null-terminated C string from data starting at offset.
func extractCString(data []byte, offset int) string {
    if offset < 0 || offset >= len(data) {
        return ""
    }
    end := bytes.IndexByte(data[offset:], 0)
    if end < 0 {
        return ""
    }
    return string(data[offset : offset+end])
}
```

### 3.6 `dynlibanalysis/default_paths.go` — デフォルト検索パス

```go
package dynlibanalysis

import "debug/elf"

// DefaultSearchPaths returns the architecture-specific default library search paths.
// These are used as the last resort when RPATH/RUNPATH and ld.so.cache fail to resolve.
// The order is: multiarch paths (Debian/Ubuntu) -> /lib64, /usr/lib64 (RHEL) -> generic.
func DefaultSearchPaths(machine elf.Machine) []string {
    switch machine {
    case elf.EM_X86_64:
        return []string{
            "/lib/x86_64-linux-gnu",
            "/usr/lib/x86_64-linux-gnu",
            "/lib64",
            "/usr/lib64",
            "/lib",
            "/usr/lib",
        }
    case elf.EM_AARCH64:
        return []string{
            "/lib/aarch64-linux-gnu",
            "/usr/lib/aarch64-linux-gnu",
            "/lib64",
            "/usr/lib64",
            "/lib",
            "/usr/lib",
        }
    default:
        return []string{
            "/lib64",
            "/usr/lib64",
            "/lib",
            "/usr/lib",
        }
    }
}
```

### 3.7 `dynlibanalysis/analyzer.go` — `record` 時の解析

```go
package dynlibanalysis

import (
    "bytes"
    "crypto/sha256"
    "debug/elf"
    "encoding/hex"
    "fmt"
    "log/slog"
    "os"
    "path/filepath"
    "time"

    "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

const (
    // MaxRecursionDepth is the maximum depth for recursive dependency resolution.
    // Normal Linux binaries have 3-5 levels of dependencies; exceeding this limit
    // indicates an abnormal configuration or missed circular dependency.
    MaxRecursionDepth = 20

    hashPrefix = "sha256"
)

// knownVDSOs contains virtual shared objects that exist only in kernel memory.
// These should be skipped during dependency resolution as they have no filesystem path.
var knownVDSOs = map[string]struct{}{
    "linux-vdso.so.1":   {},
    "linux-gate.so.1":   {},
    "linux-vdso64.so.1": {},
}

// DynLibAnalyzer resolves and records dynamic library dependencies for ELF binaries.
type DynLibAnalyzer struct {
    fs    safefileio.FileSystem
    cache *LDCache // parsed once at construction time; nil if ld.so.cache is unavailable
}

// NewDynLibAnalyzer creates a new analyzer. It parses /etc/ld.so.cache once at
// construction time and reuses the result for every Analyze() call.
// If the cache is unavailable, resolution falls back to default paths.
// A LibraryResolver is created per Analyze() call (not per DynLibAnalyzer) because
// the resolver holds architecture-specific search paths that vary by binary.
func NewDynLibAnalyzer(fs safefileio.FileSystem) *DynLibAnalyzer {
    cache, err := ParseLDCache(defaultLDCachePath)
    if err != nil {
        slog.Warn("ld.so.cache unavailable, falling back to default paths",
            "error", err)
    }
    return &DynLibAnalyzer{
        fs:    fs,
        cache: cache,
    }
}

// resolveItem represents a pending library to resolve in the BFS queue.
type resolveItem struct {
    soname     string
    parentPath string   // path of the ELF that has this soname as DT_NEEDED
    runpath    []string // DT_RUNPATH of parentPath
    depth      int
}

// Analyze resolves all direct and transitive DT_NEEDED dependencies of the given
// ELF binary, computes their hashes, and returns a DynLibDepsData snapshot.
//
// Returns nil (not an error) if the file is not ELF or has no DT_NEEDED entries.
// Returns an error if any library cannot be resolved (FR-3.1.7).
func (a *DynLibAnalyzer) Analyze(binaryPath string) (*fileanalysis.DynLibDepsData, error) {
    // Open file safely
    file, err := a.fs.SafeOpenFile(binaryPath, os.O_RDONLY, 0)
    if err != nil {
        return nil, fmt.Errorf("failed to open file: %w", err)
    }
    defer func() { _ = file.Close() }()

    // Try to parse as ELF
    elfFile, err := elf.NewFile(file)
    if err != nil {
        // Not an ELF file - this is normal for scripts etc.
        return nil, nil
    }
    defer func() { _ = elfFile.Close() }()

    // Get DT_NEEDED entries
    needed, err := elfFile.DynString(elf.DT_NEEDED)
    if err != nil || len(needed) == 0 {
        // No DT_NEEDED entries (static binary or no dependencies)
        return nil, nil
    }

    // Create resolver for this binary's architecture.
    // a.cache was parsed once at NewDynLibAnalyzer() time and is reused here.
    resolver := NewLibraryResolver(a.cache, elfFile.Machine)

    // DT_RPATH チェック: DT_RPATH が存在する場合は非サポートとして処理中断
    rpath, _ := elfFile.DynString(elf.DT_RPATH)
    if len(rpath) > 0 {
        return nil, &ErrDTRPATHNotSupported{Path: binaryPath, RPATH: strings.Join(rpath, ":")}
    }

    // Get RUNPATH
    runpath, _ := elfFile.DynString(elf.DT_RUNPATH)
    runpathEntries := splitPathList(runpath)

    // BFS queue and visited set
    var queue []resolveItem
    recorded := make(map[string]struct{})
    var libs []fileanalysis.LibEntry

    // Seed queue with direct dependencies
    for _, soname := range needed {
        queue = append(queue, resolveItem{
            soname:     soname,
            parentPath: binaryPath,
            runpath:    runpathEntries,
            depth:      1,
        })
    }

    // Process queue
    for len(queue) > 0 {
        item := queue[0]
        queue = queue[1:]

        // Skip known vDSOs
        if _, isVDSO := knownVDSOs[item.soname]; isVDSO {
            continue
        }

        // Check depth limit
        if item.depth > MaxRecursionDepth {
            return nil, &ErrRecursionDepthExceeded{
                Depth:    item.depth,
                MaxDepth: MaxRecursionDepth,
                SOName:   item.soname,
            }
        }

        // Resolve library path
        resolvedPath, err := resolver.Resolve(item.soname, item.parentPath, item.runpath)
        if err != nil {
            return nil, err
        }

        // Each physical library file is recorded at most once.
        if _, ok := recorded[resolvedPath]; ok {
            continue
        }
        recorded[resolvedPath] = struct{}{}

        // Compute hash using safefileio
        hash, err := computeFileHash(a.fs, resolvedPath)
        if err != nil {
            return nil, fmt.Errorf("failed to compute hash for %s: %w", resolvedPath, err)
        }

        // Record the library entry
        libs = append(libs, fileanalysis.LibEntry{
            SOName: item.soname,
            Path:   resolvedPath,
            Hash:   hash,
        })

        // Parse child dependencies; each child uses its own parent's RUNPATH only
        childNeeded, childRUNPATH, err := a.parseELFDeps(resolvedPath)
        if err != nil {
            slog.Debug("Failed to parse child ELF dependencies",
                "path", resolvedPath, "error", err)
            continue
        }

        for _, childSoname := range childNeeded {
            queue = append(queue, resolveItem{
                soname:     childSoname,
                parentPath: resolvedPath,
                runpath:    childRUNPATH,
                depth:      item.depth + 1,
            })
        }
    }

    if len(libs) == 0 {
        return nil, nil
    }

    return &fileanalysis.DynLibDepsData{
        RecordedAt: time.Now(),
        Libs:       libs,
    }, nil
}

// computeFileHash computes the SHA256 hash of the file at the given path
// using safefileio for symlink attack prevention.
// Shared by DynLibAnalyzer and DynLibVerifier to avoid duplication.
// Uses streaming (SafeOpenFile + io.Copy) to avoid loading large libraries
// (e.g. libLLVM.so ~50MB) entirely into memory.
func computeFileHash(fs safefileio.FileSystem, path string) (string, error) {
    file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return "", err
    }
    defer closeFile(file, path)

    h := sha256.New()
    if _, err := io.Copy(h, file); err != nil {
        return "", fmt.Errorf("failed to hash %s: %w", path, err)
    }
    return fmt.Sprintf("%s:%s", hashPrefix, hex.EncodeToString(h.Sum(nil))), nil
}

// parseELFDeps opens the given path as ELF and extracts DT_NEEDED and DT_RUNPATH.
// DT_RPATH が存在する場合は ErrDTRPATHNotSupported を返す。
// Returns nil slices (not an error) if parsing fails for other reasons.
func (a *DynLibAnalyzer) parseELFDeps(path string) (needed, runpath []string, err error) {
    file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return nil, nil, err
    }
    defer func() { _ = file.Close() }()

    elfFile, err := elf.NewFile(file)
    if err != nil {
        return nil, nil, err
    }
    defer func() { _ = elfFile.Close() }()

    neededRaw, _ := elfFile.DynString(elf.DT_NEEDED)
    rpathRaw, _ := elfFile.DynString(elf.DT_RPATH)
    if len(rpathRaw) > 0 {
        return nil, nil, &ErrDTRPATHNotSupported{Path: path, RPATH: strings.Join(rpathRaw, ":")}
    }
    runpathRaw, _ := elfFile.DynString(elf.DT_RUNPATH)

    return neededRaw, splitPathList(runpathRaw), nil
}

// splitPathList splits colon-separated path lists (as returned by DynString)
// into individual paths. Returns nil for empty input.
func splitPathList(pathLists []string) []string {
    if len(pathLists) == 0 {
        return nil
    }
    var result []string
    for _, pl := range pathLists {
        for _, p := range filepath.SplitList(pl) {
            if p != "" {
                result = append(result, p)
            }
        }
    }
    if len(result) == 0 {
        return nil
    }
    return result
}
```

### 3.8 `dynlibanalysis/verifier.go` — `runner` 時のハッシュ検証

`LD_LIBRARY_PATH` は runner 実行前に常にクリアされるため、パス再解決は不要。
`Verify` はハッシュ照合のみを行う。

```go
package dynlibanalysis

import (
    "fmt"

    "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// DynLibVerifier performs hash verification of recorded library dependencies.
type DynLibVerifier struct {
    fs safefileio.FileSystem
}

// NewDynLibVerifier creates a new verifier.
func NewDynLibVerifier(fs safefileio.FileSystem) *DynLibVerifier {
    return &DynLibVerifier{fs: fs}
}

// Verify performs hash verification of dynamic library dependencies.
//
// For each LibEntry, compute the hash of the file at entry.Path and compare with entry.Hash.
//
// Returns nil if all checks pass.
// Returns a descriptive error if any check fails.
func (v *DynLibVerifier) Verify(binaryPath string, deps *fileanalysis.DynLibDepsData) error {
    if deps == nil || len(deps.Libs) == 0 {
        return nil
    }

    // Hash verification
    for _, entry := range deps.Libs {
        if entry.Path == "" {
            return &ErrEmptyLibraryPath{
                SOName: entry.SOName,
            }
        }

        actualHash, err := computeFileHash(entry.Path)
        if err != nil {
            return fmt.Errorf("failed to read library %s at %s: %w",
                entry.SOName, entry.Path, err)
        }

        if actualHash != entry.Hash {
            return &ErrLibraryHashMismatch{
                SOName:       entry.SOName,
                Path:         entry.Path,
                ExpectedHash: entry.Hash,
                ActualHash:   actualHash,
            }
        }
    }

    return nil
}

// computeFileHash is defined in analyzer.go and shared by DynLibVerifier.
```

### 3.9 `binaryanalyzer/network_symbols.go` — `dynamicLoadSymbolRegistry` 追加

```go
// internal/runner/security/binaryanalyzer/network_symbols.go

// CategoryDynamicLoad represents dynamic library loading functions.
const CategoryDynamicLoad SymbolCategory = "dynamic_load"

// dynamicLoadSymbolRegistry contains symbols indicating runtime library loading.
// Kept separate from networkSymbolRegistry to avoid conflating dynamic load
// detection with network capability detection.
var dynamicLoadSymbolRegistry = map[string]struct{}{
    "dlopen":  {}, // Runtime library loading
    "dlsym":   {}, // Symbol resolution from loaded library
    "dlvsym":  {}, // Versioned symbol resolution
}

// IsDynamicLoadSymbol checks if the given symbol name is a dynamic load function.
func IsDynamicLoadSymbol(name string) bool {
    _, found := dynamicLoadSymbolRegistry[name]
    return found
}
```

### 3.10 `binaryanalyzer/analyzer.go` — `AnalysisOutput` 拡張

```go
// internal/runner/security/binaryanalyzer/analyzer.go

// AnalysisOutput contains the complete result of binary analysis.
type AnalysisOutput struct {
    Result          AnalysisResult
    DetectedSymbols []DetectedSymbol
    Error           error
    // HasDynamicLoad indicates that dlopen/dlsym/dlvsym symbols were found.
    // When true, dynamic library integrity cannot be statically verified.
    // This field is independent of Result (a binary can have both network
    // symbols and dynamic load symbols).
    HasDynamicLoad bool
}
```

### 3.11 ELF/Mach-O アナライザーへの `HasDynamicLoad` 検出追加

ELF アナライザーの `AnalyzeNetworkSymbols` のシンボルチェックループに `IsDynamicLoadSymbol` を追加する:

```go
// internal/runner/security/elfanalyzer/standard_analyzer.go
// (既存のシンボルチェックループ内に追加)

for _, sym := range symbols {
    if sym.Section == elf.SHN_UNDEF {
        symName := sym.Name
        // Existing: check network symbols
        if cat, found := a.networkSymbols[symName]; found {
            detected = append(detected, binaryanalyzer.DetectedSymbol{
                Name:     symName,
                Category: string(cat),
            })
        }
        // NEW: check dynamic load symbols
        if binaryanalyzer.IsDynamicLoadSymbol(symName) {
            hasDynamicLoad = true
        }
    }
}
```

Mach-O アナライザーにも同様の変更を追加する:

```go
// internal/runner/security/machoanalyzer/standard_analyzer.go
// (既存のシンボルチェックループ内に追加)

for _, sym := range importedSymbols {
    normalized := normalizeSymbolName(sym)
    if cat, found := a.networkSymbols[normalized]; found {
        detected = append(detected, binaryanalyzer.DetectedSymbol{
            Name:     normalized,
            Category: string(cat),
        })
    }
    // NEW: check dynamic load symbols
    if binaryanalyzer.IsDynamicLoadSymbol(normalized) {
        hasDynamicLoad = true
    }
}
```

### 3.12 `filevalidator/validator.go` — `DynLibAnalyzer` 注入と `LoadRecord` 追加

```go
// internal/filevalidator/validator.go

// FileValidator interface defines the basic file validation methods
type FileValidator interface {
    Record(filePath string, force bool) (string, string, error)
    Verify(filePath string) error
    VerifyWithHash(filePath string) (string, error)
    VerifyWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) error
    VerifyAndRead(filePath string) ([]byte, error)
    VerifyAndReadWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) ([]byte, error)
    // LoadRecord returns the full analysis record for the given file path.
    // Used by verification.Manager to access DynLibDeps without exposing the store directly.
    LoadRecord(filePath string) (*fileanalysis.Record, error)
}

// Validator provides functionality to record and verify file hashes.
type Validator struct {
    algorithm               HashAlgorithm
    hashDir                 string
    hashFilePathGetter      common.HashFilePathGetter
    privilegedFileValidator *PrivilegedFileValidator
    store                   *fileanalysis.Store
    dynlibAnalyzer          *dynlibanalysis.DynLibAnalyzer    // nil if dynlib analysis is disabled
    binaryAnalyzer          binaryanalyzer.BinaryAnalyzer     // nil if HasDynamicLoad recording is disabled
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
// Call before the first Record() invocation. Safe to call with nil (disables dynlib analysis).
func (v *Validator) SetDynLibAnalyzer(a *dynlibanalysis.DynLibAnalyzer) {
    v.dynlibAnalyzer = a
}

// SetBinaryAnalyzer injects the BinaryAnalyzer used to record HasDynamicLoad during record operations.
// Call before the first Record() invocation. Safe to call with nil (disables HasDynamicLoad recording).
func (v *Validator) SetBinaryAnalyzer(a binaryanalyzer.BinaryAnalyzer) {
    v.binaryAnalyzer = a
}
```

`Record` メソッドの拡張（`updateAnalysisRecord` 内の `store.Update` コールバックに DynLibDeps / HasDynamicLoad 解析を統合）:

```go
// updateAnalysisRecord saves the hash using FileAnalysisRecord format.
// When analyzers are set, DynLibDeps and HasDynamicLoad are also analyzed and
// recorded in the same Update callback (atomic with hash update).
// HasDynamicLoad is always reset to false before the analyzer is consulted, so
// disabling the analyzer clears any stale true from a prior run. The json tag
// uses omitempty, so false is omitted from the JSON file; this is safe because
// every record run recomputes the value from scratch.
func (v *Validator) updateAnalysisRecord(filePath common.ResolvedPath, hash, hashFilePath string, force bool) (string, string, error) {
    contentHash := fmt.Sprintf("%s:%s", v.algorithm.Name(), hash)
    err := v.store.Update(filePath, func(record *fileanalysis.Record) error {
        // Existing collision/duplicate checks
        if record.FilePath != "" && record.FilePath != filePath.String() {
            return fmt.Errorf("%w: %s and %s map to the same record file",
                ErrHashFilePathCollision, filePath, record.FilePath)
        }
        if record.FilePath == filePath.String() && !force {
            return fmt.Errorf("hash file already exists for %s: %w", filePath, ErrHashFileExists)
        }
        record.ContentHash = contentHash

        // NEW: Analyze dynamic library dependencies if analyzer is available
        if v.dynlibAnalyzer != nil {
            dynLibDeps, err := v.dynlibAnalyzer.Analyze(filePath.String())
            if err != nil {
                // Analysis failure -> callback returns error -> nothing is persisted
                return fmt.Errorf("dynamic library analysis failed: %w", err)
            }
            record.DynLibDeps = dynLibDeps // nil for non-ELF or static ELF (omitted in JSON)
        }

        // NEW: Detect dlopen/dlsym/dlvsym symbols via BinaryAnalyzer interface.
        // Always reset to false first so that disabling the analyzer clears any stale true
        // from a prior record run. omitempty means false is omitted from JSON, which is safe:
        // every record run recomputes this value from scratch.
        record.HasDynamicLoad = false
        if v.binaryAnalyzer != nil {
            output := v.binaryAnalyzer.AnalyzeNetworkSymbols(filePath.String(), contentHash)
            record.HasDynamicLoad = output.HasDynamicLoad
        }

        return nil
    })
    if err != nil {
        return "", "", fmt.Errorf("failed to update analysis record: %w", err)
    }

    return hashFilePath, contentHash, nil
}
```

### 3.13 `verification/manager.go` — `verifyDynLibDeps` の実装

`Manager` 構造体に `dynlibVerifier` フィールドを追加し、`newManagerInternal` で 1 回だけ `NewDynLibVerifier` を呼ぶ。`NewManager`・`NewManagerForDryRun`・`NewManagerForTest` はいずれも `newManagerInternal` 経由で構築されるため、すべてのコンストラクタで一貫して初期化される。

```go
// internal/verification/manager.go
// 追加 import: "debug/elf"（hasDynamicLibraryDeps で使用; "os" は既存）

type Manager struct {
    // ... existing fields ...
    dynlibVerifier *dynlibanalysis.DynLibVerifier  // NEW: initialized once at construction
}

// In NewManager (or equivalent constructor):
//   m.dynlibVerifier = dynlibanalysis.NewDynLibVerifier(fs)

// verifyDynLibDeps performs dynamic library integrity verification
// when a DynLibDeps snapshot is present in the analysis record.
func (m *Manager) verifyDynLibDeps(cmdPath string) error {
    record, err := m.fileValidator.LoadRecord(cmdPath)
    if err != nil {
        return fmt.Errorf("failed to load record for dynlib verification: %w", err)
    }

    if record.DynLibDeps != nil {
        // DynLibDeps is recorded: perform hash verification
        return m.dynlibVerifier.Verify(cmdPath, record.DynLibDeps)
    }

    // DynLibDeps is not recorded: check if this is a dynamically linked ELF binary
    hasDynDeps, err := hasDynamicLibraryDeps(cmdPath)
    if err != nil {
        return fmt.Errorf("failed to check dynamic library dependencies: %w", err)
    }

    if hasDynDeps {
        // ELF binary without DynLibDeps record -> requires re-recording
        return &dynlibanalysis.ErrDynLibDepsRequired{BinaryPath: cmdPath}
    }

    // Non-ELF binary (or static/no-dependency ELF) without DynLibDeps -> normal
    return nil
}

// hasDynamicLibraryDeps checks if the file at the given path is an ELF binary
// that has at least one DT_NEEDED entry (i.e., dynamically linked).
// Static ELFs and ELFs with no DT_NEEDED entries return (false, nil).
//
// Errors are classified as follows:
//   - os.Open failure (I/O error, permission denied, file not found) → (false, err): propagated to caller
//   - elf.NewFile failure (not an ELF, bad magic)                    → (false, nil): file is not an ELF binary
func hasDynamicLibraryDeps(path string) (bool, error) {
    // Open file first: distinguish I/O failures from format errors.
    // The path is already symlink-resolved by PathResolver, so os.Open is safe here.
    osFile, err := os.Open(path)
    if err != nil {
        // I/O error or permission denied: propagate, do not silently skip dynlib check
        return false, fmt.Errorf("failed to open binary for ELF inspection: %w", err)
    }
    defer func() { _ = osFile.Close() }()

    elfFile, err := elf.NewFile(osFile)
    if err != nil {
        // Not an ELF binary (bad magic, unsupported format, etc.)
        return false, nil
    }
    defer func() { _ = elfFile.Close() }()

    needed, err := elfFile.DynString(elf.DT_NEEDED)
    if err != nil || len(needed) == 0 {
        return false, nil
    }
    return true, nil
}
```

#### 統合ポイントの設計

**`VerifyGroupFiles` 内に `isCommandFile` 判定を埋め込まない理由**

`collectVerificationFiles` は重複排除のために `map[string]struct{}` を返す。この時点でファイルが「`verify_files` に明示されたファイル」か「コマンドファイル（`commands[*].cmd` から解決されたパス）」かという由来情報が失われる。

`VerifyGroupFiles` のループで `isCommandFile` を判定するには `collectVerificationFiles` の戻り値型を変更する必要があり、既存の `ManagerInterface` の契約変更が波及する。

**採用する設計: `Manager.VerifyCommandDynLibDeps` を公開メソッドとして追加**

`verifyDynLibDeps` を内部ヘルパーとして保持しつつ、コマンドパスを受け取る公開メソッド `VerifyCommandDynLibDeps` を追加する。呼び出しは `group_executor.go` の `verifyGroupFiles` 内で行い、`VerifyGroupFiles` の実装は変更しない。

```go
// VerifyCommandDynLibDeps performs dynamic library integrity verification for a command binary.
// It is called separately from VerifyGroupFiles to avoid the need to track
// which files in the verification set are command files vs explicit verify_files entries.
func (m *Manager) VerifyCommandDynLibDeps(cmdPath string) error {
    return m.verifyDynLibDeps(cmdPath)
}
```

`ManagerInterface` への追加:

```go
type ManagerInterface interface {
    ResolvePath(path string) (string, error)
    VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*Result, error)
    VerifyCommandDynLibDeps(cmdPath string) error  // NEW
}
```

`group_executor.go` の `verifyGroupFiles` 内での呼び出し:

```go
// verifyGroupFiles 内（VerifyGroupFiles 成功後）
result, err := ge.verificationManager.VerifyGroupFiles(runtimeGroup)
if err != nil {
    return err
}

// NEW: Verify dynamic library dependencies for each command binary
for _, cmd := range runtimeGroup.Commands {
    resolvedPath, err := ge.verificationManager.ResolvePath(cmd.ExpandedCmd)
    if err != nil {
        continue // path resolution failure already logged above
    }
    if dlErr := ge.verificationManager.VerifyCommandDynLibDeps(resolvedPath); dlErr != nil {
        slog.Error("Dynamic library verification failed",
            "group", runnertypes.ExtractGroupName(runtimeGroup),
            "command", resolvedPath,
            "error", dlErr)
        return dlErr
    }
}
```

この設計により:
- `VerifyGroupFiles` の実装・インターフェース契約は変更なし
- コマンドファイルの識別は `runtimeGroup.Commands` から直接行う（メタデータを失わない）
- `VerifyCommandDynLibDeps` はテストでモック可能

### 3.14 `runner/security/network_analyzer.go` — `HasDynamicLoad` 高リスク判定

> **設計注記（二重検出について）**: `HasDynamicLoad` は `record` 時に `Record.HasDynamicLoad` として保存されるが、runner の高リスク判定（本セクション）では `Record.HasDynamicLoad` を読まず、`AnalyzeNetworkSymbols()` を**再度**呼び出す。
> `isNetworkViaBinaryAnalysis` はセキュリティプロファイルに未登録のコマンドに対するフォールバック判定フローであり、既存の設計として record 時の保存値ではなくバイナリのライブ解析を行う。`HasDynamicLoad` だけ保存値を読む設計にするとフロー内で不整合が生じる。
> ハッシュ検証後のバイナリは改ざんされていないことが保証されるため再解析は冗長だが、このフロー全体の改善は本タスクのスコープ外とする。`Record.HasDynamicLoad` は将来の診断・監査用途のスナップショットとして保存される。

現行の `isNetworkViaBinaryAnalysis` は `bool` を返す。`HasDynamicLoad` を高リスク（`RiskLevelHigh`）として伝播するには、`EvaluateRisk` の `isHighRisk=true` パスを通す必要がある（`evaluator.go:45-46` 参照）。単に `true` を返すと `isNetwork=true`（中リスク）になるため、戻り値シグネチャを `(isNetwork, isHighRisk bool)` に拡張する。

**`HasDynamicLoad` と `NetworkDetected` の同時検出について**

`HasDynamicLoad` と `NetworkDetected` は独立したシグナルであり、同一バイナリで両方が検出される場合がある（例: `dlopen` と `socket` の両方を使うバイナリ）。この場合、`(true, true)` を返す:

- `isNetwork=true`: バイナリが直接ネットワーク操作シンボルを持つ（`evaluator.go` の `isNetwork` パスで `RiskLevelMedium` に到達するが、`isHighRisk=true` により上書きされる）
- `isHighRisk=true`: `dlopen` 等により実行時に追加ライブラリを動的ロードする（静的解析が不完全であることを示す）

`(false, true)` を返すと「ネットワーク操作ではないが高リスク」という意味になり、実際には `NetworkDetected` であるにもかかわらず `isNetwork=false` として呼び出し元（ログ等）に伝わるため不正確となる。リスクレベル自体は `max(Medium, High)=High` で変わらないが、セマンティクスの正確性のために両フラグを独立して設定する。

```go
// internal/runner/security/network_analyzer.go

// isNetworkViaBinaryAnalysis performs binary analysis on the command binary.
// Returns (isNetwork, isHighRisk).
//
// isHighRisk=true is returned when HasDynamicLoad is detected (dlopen/dlsym/dlvsym),
// indicating the binary loads libraries at runtime and cannot be statically analyzed.
//
// Note: HasDynamicLoad and NetworkDetected are independent signals. A binary may
// have both (e.g., uses dlopen AND has socket symbols), in which case (true, true)
// is returned. The two flags are set independently to preserve semantic accuracy:
// isNetwork reflects static symbol detection; isHighRisk reflects analysis incompleteness.
func (a *NetworkAnalyzer) isNetworkViaBinaryAnalysis(cmdPath string, contentHash string) (isNetwork, isHighRisk bool) {
    output := a.binaryAnalyzer.AnalyzeNetworkSymbols(cmdPath, contentHash)

    // NEW: HasDynamicLoad -> high risk (RiskLevelHigh via EvaluateRisk isHighRisk path)
    // dlopen/dlsym/dlvsym indicate runtime library loading; static analysis is incomplete.
    // isHighRisk is set independently of isNetwork so both signals are preserved.
    if output.HasDynamicLoad {
        slog.Debug("Binary analysis detected dynamic load symbols (dlopen/dlsym/dlvsym)",
            "path", cmdPath)
        isHighRisk = true
    }

    // Existing logic follows (unchanged)...
    switch output.Result {
    case binaryanalyzer.NetworkDetected:
        return true, isHighRisk
    case binaryanalyzer.NoNetworkSymbols:
        return false, isHighRisk
    case binaryanalyzer.NotSupportedBinary:
        return false, isHighRisk
    case binaryanalyzer.StaticBinary:
        return false, isHighRisk
    case binaryanalyzer.AnalysisError:
        return true, isHighRisk // Fail safe: treat as middle risk (network detected)
    default:
        return true, isHighRisk
    }
}
```

`IsNetworkOperation` 内の呼び出しも変更する:

```go
// IsNetworkOperation 内（既存コード）
if !foundInProfiles && filepath.IsAbs(cmdName) {
    // CHANGED: isNetworkViaBinaryAnalysis now returns (isNetwork, isHighRisk)
    if isNet, isHigh := a.isNetworkViaBinaryAnalysis(cmdName, contentHash); isNet || isHigh {
        return isNet, isHigh
    }
}
```

`cmd/record/main.go` から `BinaryAnalyzer` を取得するために、プラットフォーム選択ロジックを `NewNetworkAnalyzer` から分離した公開ファクトリ関数を追加する:

```go
// internal/runner/security/network_analyzer.go

// NewBinaryAnalyzer returns the platform-appropriate BinaryAnalyzer implementation.
// On macOS, returns a Mach-O analyzer; on Linux and other platforms, returns an ELF analyzer.
// This function exposes the same platform selection logic used internally by NewNetworkAnalyzer,
// allowing callers outside the runner (e.g., cmd/record) to obtain a BinaryAnalyzer without
// constructing a full NetworkAnalyzer.
func NewBinaryAnalyzer() binaryanalyzer.BinaryAnalyzer {
    switch runtime.GOOS {
    case gosDarwin:
        return machoanalyzer.NewStandardMachOAnalyzer(nil)
    default: // "linux", etc.
        return elfanalyzer.NewStandardELFAnalyzer(nil, nil)
    }
}
```

> **設計注記**: `NewNetworkAnalyzer` の内部実装を `NewBinaryAnalyzer` に委譲するようリファクタリングすることで、重複を排除できる。ただし本タスクのスコープでは単純に同等の関数を追加するだけでも差し支えない。

### 3.15 `cmd/record/main.go` — `DynLibAnalyzer` 統合

`deps.validatorFactory` の戻り値型を `*filevalidator.Validator` に変更し、`processFiles` の前にセッターで注入する:

```go
// cmd/record/main.go

func run(args []string, stdout, stderr io.Writer, d dirCreator) int {
    // ... existing config setup ...

    // validatorFactory now returns *filevalidator.Validator (concrete type)
    recorder, err := d.validatorFactory(cfg.hashDir)
    if err != nil {
        // ... error handling ...
    }

    // NEW: Create and inject DynLibAnalyzer for dynamic library dependency recording
    dynlibAnalyzer := dynlibanalysis.NewDynLibAnalyzer(
        safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
    )
    recorder.SetDynLibAnalyzer(dynlibAnalyzer)

    // processFiles receives recorder as hashRecorder interface (no signature change needed)
    // ... existing processFiles call ...
}
```

> **設計注記**: `deps.validatorFactory` の戻り値型を `hashRecorder` インターフェースから `*filevalidator.Validator` 具象型に変更することで、`run()` 内でセッターメソッドを呼び出せる。`processFiles` には引き続き `hashRecorder` インターフェースとして渡す。`filevalidator` は別パッケージのため、フィールドへの直接代入（`recorder.dynlibAnalyzer = ...`）はパッケージ外から不可であり、セッターメソッドを使用する。既存の `syscallAnalysisContext` が同様のコンストラクタ注入パターンを採用しており、一貫性がある。

### 3.16 `HasDynamicLoad` の `record` 時の記録

`HasDynamicLoad` の記録は §3.12 の `updateAnalysisRecord` の `store.Update` コールバック内に統合する（案 A）。
`processFiles` 内で別途 `store.Update` を呼ぶ案（案 B）は採用しない。

**案 B を採用しない理由:**
- `AnalyzeNetworkSymbols` をパッケージレベル関数として呼ぶことはできない（`BinaryAnalyzer` はインターフェース）
- `HasDynamicLoad == true` の時のみ書き込むと、再 record 後に古い `true` 値が残る stale 問題が生じる
- `store.Update` を 2 回呼ぶと 2 回目の更新が最初の更新を上書き（`DynLibDeps` を消去）するリスクがある

**案 A の統合方法:**

`Validator.binaryAnalyzer` フィールドに `BinaryAnalyzer` インスタンスを注入し、`updateAnalysisRecord` コールバック内で `record.HasDynamicLoad` を常に書き込む（§3.12 参照）。

`cmd/record/main.go` での注入:

```go
// cmd/record/main.go

func run(args []string, stdout, stderr io.Writer, d dirCreator) int {
    // ... existing config, validator setup ...

    // NEW: Inject analyzers for dynamic library dependency recording and
    //      HasDynamicLoad detection. Both are integrated into updateAnalysisRecord's
    //      store.Update callback for atomicity.
    dynlibAnalyzer := dynlibanalysis.NewDynLibAnalyzer(
        safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
    )
    recorder.SetDynLibAnalyzer(dynlibAnalyzer)
    recorder.SetBinaryAnalyzer(security.NewBinaryAnalyzer())

    // ... existing processFiles call ...
}
```

## 4. テスト実装詳細

### 4.1 受入基準とテストの対応表

| 受入基準 (AC) | テストケース | テストファイル |
|-------------|------------|-------------|
| AC-1: ライブラリパス解決 | `TestResolve_RUNPATH`, `TestResolve_Origin`, `TestResolve_LDCache`, `TestResolve_DefaultPaths`, `TestResolve_Failure` | `resolver_test.go` |
| AC-1: 再帰解決・循環防止 | `TestAnalyze_TransitiveDeps`, `TestAnalyze_CircularDeps`, `TestAnalyze_MaxDepth`, `TestAnalyze_ResolutionFailure` | `analyzer_test.go` |
| AC-1: DT_RPATH 非サポート | `TestAnalyze_DTRPATHNotSupported` | `analyzer_test.go` |
| AC-2: record 拡張 | `TestAnalyze_DynamicELF`, `TestAnalyze_StaticELF`, `TestAnalyze_NonELF`, `TestAnalyze_LibEntryFields`, `TestAnalyze_Force` | `analyzer_test.go` |
| AC-3: runner 検証拡張 | `TestVerify_HashMatch`, `TestVerify_HashMismatch`, `TestVerify_EmptyPath`, `TestVerify_SchemaVersion`, `TestVerify_ELFNoDynLibDeps`, `TestVerify_NonELFNoDynLibDeps` | `verifier_test.go` |
| AC-3: 統合ポイント（コマンドのみ適用）[コンポーネントテスト] | `TestVerifyGroupFiles_DynLibNotCalledForVerifyFiles`（MockManager 使用・呼び出しパターン検証）, `TestVerifyCommandDynLibDeps_DynamicELF`, `TestVerifyCommandDynLibDeps_NonELF` | `manager_test.go`, `group_executor_test.go` |
| AC-4: dlopen シンボル検出 | `TestIsDynamicLoadSymbol`, `TestHasDynamicLoad_ELF`, `TestHasDynamicLoad_Independent` | `network_symbols_test.go`, `standard_analyzer_test.go` |
| AC-4: HasDynamicLoad 記録（stale 防止） | `TestRecord_HasDynamicLoad_True`, `TestRecord_HasDynamicLoad_WrittenWhenFalse`, `TestRecord_BinaryAnalyzerNil_NoError` | `validator_test.go` |
| AC-5: 既存機能非影響 | `TestExistingContentHashPreserved`, `TestExistingSyscallAnalysisPreserved`, `TestExistingTests` | 既存テストスイート |

### 4.2 ユニットテスト

#### 4.2.1 `resolver_test.go`

```go
func TestResolve_RUNPATH(t *testing.T) {
    // Setup: create temp directory with dummy .so
    // Create ResolveContext with OwnRUNPATH pointing to temp dir
    // Verify: library resolves to the expected path
}

func TestResolve_Origin(t *testing.T) {
    // Setup: create temp directory simulating binary's dir
    // Use $ORIGIN in RUNPATH entries
    // Verify: $ORIGIN is expanded to ParentDir
}

func TestResolve_LDCache(t *testing.T) {
    // Setup: inject mock LDCache with known soname->path mapping
    // Verify: library resolves via cache
}

func TestResolve_DefaultPaths(t *testing.T) {
    // Test with elf.EM_X86_64 -> verify multiarch paths included
    // Test with elf.EM_AARCH64 -> verify aarch64 multiarch paths
    // Test with other machine -> verify generic paths only
}

func TestResolve_Failure(t *testing.T) {
    // Setup: no library exists in any search path
    // Verify: ErrLibraryNotResolved is returned with correct fields
    // Verify: SearchPaths contains all attempted paths
}
```

#### 4.2.2 `ldcache_test.go`

```go
func TestParseLDCache_NewFormat(t *testing.T) {
    // Use testdata/ldcache_new_format.bin
    // Verify: entries are correctly parsed
}

func TestParseLDCache_NotFound(t *testing.T) {
    // Path does not exist
    // Verify: returns nil, error
}

func TestParseLDCache_UnsupportedFormat(t *testing.T) {
    // Provide data without the magic string
    // Verify: returns nil, error
}

func TestParseLDCache_Truncated(t *testing.T) {
    // Provide truncated data
    // Verify: returns nil, error
}

func TestLDCache_Lookup(t *testing.T) {
    // Verify: known soname returns correct path
    // Verify: unknown soname returns ""
    // Verify: nil cache returns ""
}
```

#### 4.2.3 RUNPATH 非継承テスト

`resolver_context.go` は存在しない。RUNPATH 非継承は `analyzer_test.go` の `TestAnalyze_TransitiveDeps` 等で検証する（各 resolveItem が親自身の runpath のみを保持することを確認）。

#### 4.2.4 `verifier_test.go`

```go
func TestVerify_HashMatch(t *testing.T) {
    // Setup: create temp .so files and compute their hashes
    // Create DynLibDepsData with matching hashes
    // Verify: Verify() returns nil
}

func TestVerify_HashMismatch(t *testing.T) {
    // Setup: create temp .so, record wrong hash
    // Verify: ErrLibraryHashMismatch is returned
    // Verify: error contains soname, expected hash, actual hash
}

func TestVerify_EmptyPath(t *testing.T) {
    // Setup: create DynLibDepsData with entry.Path = ""
    // Verify: ErrEmptyLibraryPath is returned
}
```

#### 4.2.5 `network_symbols_test.go`

```go
func TestIsDynamicLoadSymbol(t *testing.T) {
    tests := []struct {
        name     string
        symbol   string
        expected bool
    }{
        {"dlopen detected", "dlopen", true},
        {"dlsym detected", "dlsym", true},
        {"dlvsym detected", "dlvsym", true},
        {"network symbol not detected as dynload", "socket", false},
        {"unknown symbol", "unknown_func", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.expected, binaryanalyzer.IsDynamicLoadSymbol(tt.symbol))
        })
    }
}
```

#### 4.2.6 `group_executor_test.go`（コンポーネントテスト）

`group_executor.go` の呼び出しパターンを `MockManager` を使ってテストする。実ファイルシステムや ELF 解析は使わず、`VerifyCommandDynLibDeps` の呼び出し有無のみを検証するため、統合テストではなくコンポーネントテスト（単体テストに準じる）に分類する。

```go
func TestVerifyGroupFiles_DynLibNotCalledForVerifyFiles(t *testing.T) {
    // Setup: MockManager with call recorder
    // Execute: verifyGroupFiles with a mix of command and verify_files entries
    // Verify: VerifyCommandDynLibDeps was called only for command files, not for verify_files
}
```

### 4.3 統合テスト

#### 4.3.1 `record` → `runner` 正常フロー

```go
func TestIntegration_RecordAndVerify_Normal(t *testing.T) {
    // 1. Build or use a test binary with known DT_NEEDED
    // 2. Run record -> verify DynLibDeps is created in record file
    // 3. Run runner verification -> verify success
}
```

#### 4.3.2 ライブラリ改ざん検出

```go
func TestIntegration_LibraryTamperingDetection(t *testing.T) {
    // 1. Record a binary
    // 2. Modify a dependent library's content
    // 3. Run runner verification -> verify hash mismatch
}
```

#### 4.3.3 ライブラリ改ざん検出（追加ケース）

LD_LIBRARY_PATH は runner 実行前にクリアされるため、LD_LIBRARY_PATH ハイジャックの統合テストは不要。
ライブラリ内容の改ざんはハッシュ検証で検出される（4.3.2 参照）。

#### 4.3.4 旧スキーマ拒否と移行

```go
func TestIntegration_OldSchemaRejection(t *testing.T) {
    // 1. Create a record file with schema_version: 1 or 2
    // 2. Run runner verification (Store.Load path) -> verify SchemaVersionMismatchError
    //    (runner rejects old schema records)
    // 3. Run record --force on the same file -> verify success
    //    (Store.Update allows overwriting old schema when Actual < Expected)
    // 4. Run runner verification again -> verify success (record now has schema_version: 2)
}
```

### 4.4 テストデータ

- `testdata/ldcache_new_format.bin`: 最小構成の `ld.so.cache`（新形式）バイナリファイル。テスト生成スクリプトで作成
- ライブラリ解決テスト: テンポラリディレクトリにダミー `.so` ファイルを配置して解決パスを検証
- ELF テストバイナリ: `go test` 内で `exec.Command("gcc", ...)` 等でテスト用の小さな C プログラムをコンパイルするか、既存のシステムバイナリ（`/bin/ls` 等）を使用

## 5. 実装チェックリスト

### 5.1 Phase 1: 基盤型とスキーマ拡張
- [x] `fileanalysis/schema.go`: `CurrentSchemaVersion` を 2 に変更
- [x] `fileanalysis/schema.go`: `DynLibDepsData`, `LibEntry` 型定義追加（`soname`, `path`, `hash` のみ）
- [x] `fileanalysis/schema.go`: `Record` に `DynLibDeps`, `HasDynamicLoad` フィールド追加
- [x] `fileanalysis/file_analysis_store.go`: `Store.Update` を修正し、旧スキーマ（`Actual < Expected`）の `SchemaVersionMismatchError` を `Record{}` で上書き許可、新スキーマ（`Actual > Expected`）は引き続き拒否
- [x] 既存テストの `CurrentSchemaVersion` 依存箇所を更新
- [x] `Store.Update` の新しい分岐をユニットテストで検証

### 5.2 Phase 2: ライブラリ解決エンジン
- [x] `dynlibanalysis/` パッケージ作成
- [x] `dynlibanalysis/errors.go`: エラー型定義（`ErrDTRPATHNotSupported` を含む）
- [x] `dynlibanalysis/default_paths.go`: アーキテクチャ別デフォルト検索パス
- [x] `dynlibanalysis/ldcache.go`: `/etc/ld.so.cache` パーサー
- [x] `dynlibanalysis/resolver.go`: `LibraryResolver`, `Resolve`（3 段階：RUNPATH → cache → default、LD_LIBRARY_PATH は除外）
- [x] 上記の全ユニットテスト

### 5.3 Phase 3: DynLibAnalyzer（record 拡張）
- [x] `dynlibanalysis/analyzer.go`: `DynLibAnalyzer`, `Analyze`（再帰解決 + ハッシュ計算）
  - DT_RPATH 検出時に `ErrDTRPATHNotSupported` を返す
  - `traversalKey` は `resolvedPath` のみ
  - `LibEntry` には `soname`, `path`, `hash` のみ記録
- [x] vDSO スキップリスト
- [x] visited セット（循環依存防止）
- [x] 再帰深度制限
- [x] `filevalidator/validator.go`: `dynlibAnalyzer` フィールド, `SetDynLibAnalyzer` セッター, `LoadRecord` 追加
- [x] `filevalidator/validator.go`: `updateAnalysisRecord` コールバックに DynLibDeps 解析統合
- [x] `cmd/record/main.go`: `deps.validatorFactory` を `*filevalidator.Validator` 返しに変更し `SetDynLibAnalyzer` で注入
- [x] 上記の全ユニットテスト・統合テスト

### 5.4 Phase 4: DynLibVerifier（runner 拡張）
- [x] `dynlibanalysis/verifier.go`: `DynLibVerifier`, `Verify`（ハッシュ検証のみ）
- [x] `FileValidator` インターフェースに `LoadRecord` 追加
- [x] `verification/manager.go`: `verifyDynLibDeps` 実装
- [x] `verification/manager.go`: `VerifyGroupFiles` への統合
- [x] ELF バイナリの `DynLibDeps` 未記録検出
- [x] `LD_LIBRARY_PATH` は record・runner ともに不使用（runner 実行前にクリア済み）
- [x] 上記の全ユニットテスト・統合テスト

### 5.5 Phase 5: dlopen シンボル検出 + 仕上げ
- [x] `binaryanalyzer/network_symbols.go`: `dynamicLoadSymbolRegistry`, `IsDynamicLoadSymbol`
- [x] `binaryanalyzer/analyzer.go`: `AnalysisOutput.HasDynamicLoad` フィールド追加
- [x] `elfanalyzer/standard_analyzer.go`: `HasDynamicLoad` 検出ロジック追加
- [x] `machoanalyzer/standard_analyzer.go`: `HasDynamicLoad` 検出ロジック追加
- [x] `fileanalysis/schema.go`: `Record.HasDynamicLoad` フィールド（Phase 1 で追加済み）
- [x] `filevalidator/validator.go`: `binaryAnalyzer` フィールド, `SetBinaryAnalyzer` セッター追加
- [x] `filevalidator/validator.go`: `updateAnalysisRecord` コールバックに `HasDynamicLoad` 記録を統合（常に true/false を書き込み）
- [x] `cmd/record/main.go`: `SetBinaryAnalyzer` で `BinaryAnalyzer` を注入
- [x] `runner/security/network_analyzer.go`: `isNetworkViaBinaryAnalysis` の戻り値を `(isNetwork, isHighRisk bool)` に変更し、`HasDynamicLoad` 検出時に `isHighRisk=true` を設定する高リスク判定を追加（`isNetwork` は独立して判定）
- [x] `runner/security/network_analyzer.go`: `IsNetworkOperation` 内の呼び出しを `isNet, isHigh := a.isNetworkViaBinaryAnalysis(...)` に変更
- [x] 上記の全ユニットテスト（`TestRecord_HasDynamicLoad_WrittenWhenFalse` を含む）
- [x] 全既存テストのパス確認
- [x] `make lint` / `make fmt` パス確認

## 6. 参照

- タスク 0069: ELF 動的シンボル解析によるネットワーク操作検出
- タスク 0070/0072: ELF syscall 解析
- タスク 0073: Mach-O ネットワーク操作検出
- [01_requirements.md](01_requirements.md): 要件定義書
- [02_architecture.md](02_architecture.md): アーキテクチャ設計書
