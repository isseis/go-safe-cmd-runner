# Mach-O `LC_LOAD_DYLIB` 整合性検証 詳細仕様書

## 1. 概要

### 1.1 目的

本仕様書は、Mach-O バイナリの `LC_LOAD_DYLIB` / `LC_LOAD_WEAK_DYLIB` エントリから依存ライブラリを解決・記録し、`runner` 実行時にハッシュ照合でライブラリの改ざん・差し替えを検出する機能の詳細実装設計を定義する。

### 1.2 設計範囲

- `machodylib` パッケージの新規実装（Mach-O ライブラリ解決エンジン、dyld shared cache 判定、`MachODynLibAnalyzer`）
- `fileanalysis` パッケージのスキーマ拡張（`AnalysisWarnings`、`CurrentSchemaVersion` 変更）
- `filevalidator` パッケージの拡張（`machoDynlibAnalyzer` 注入、`SaveRecord` 内の Mach-O 解析呼び出し）
- `verification` パッケージの拡張（`hasMachODynamicLibraryDeps` 追加）
- `dynlibanalysis` パッケージの拡張（`ErrDynLibDepsRequired` のエラーメッセージ汎用化）
- `cmd/record` コマンドの拡張（`MachODynLibAnalyzer` 注入）

### 1.3 前提

- ELF 版タスク 0074 で導入された以下の基盤を再利用する：
  - `fileanalysis.LibEntry` 型（`SOName`, `Path`, `Hash`）
  - `dynlibanalysis.DynLibVerifier`（形式非依存のハッシュ照合）
  - `dynlibanalysis` パッケージの共有エラー型（`ErrLibraryHashMismatch`, `ErrEmptyLibraryPath`, `ErrDynLibDepsRequired`）
  - `computeFileHash` ヘルパー関数
- `DynLibDeps` フィールドは `[]fileanalysis.LibEntry` 型（スキーマバージョン 10 でフラット化済み）
- 現行 `CurrentSchemaVersion` は 13

## 2. パッケージ構成

### 2.1 ディレクトリ構造

```
internal/
├── machodylib/                          # NEW: Mach-O 動的ライブラリ解析パッケージ
│   ├── doc.go                           # パッケージドキュメント
│   ├── analyzer.go                      # MachODynLibAnalyzer: record 時の BFS 依存解決 + ハッシュ計算
│   ├── resolver.go                      # LibraryResolver: インストール名 → ファイルシステムパス解決
│   ├── shared_cache.go                  # IsDyldSharedCacheLib: dyld shared cache 判定
│   ├── errors.go                        # エラー型定義
│   ├── analyzer_test.go                 # MachODynLibAnalyzer テスト
│   ├── resolver_test.go                 # LibraryResolver テスト
│   ├── shared_cache_test.go             # dyld shared cache 判定テスト
│   └── testdata/
│       └── README.md                    # テストデータの説明と生成方法
│
├── fileanalysis/
│   └── schema.go                        # CurrentSchemaVersion: 13 → 14
│                                        # Record.AnalysisWarnings 追加
│
├── filevalidator/
│   └── validator.go                     # machoDynlibAnalyzer フィールド追加
│                                        # SetMachODynLibAnalyzer セッター追加
│                                        # updateAnalysisRecord 内に Mach-O 解析統合
│
├── verification/
│   └── manager.go                       # hasMachODynamicLibraryDeps 追加
│                                        # verifyDynLibDeps 内の Mach-O 判定追加
│
├── dynlibanalysis/
│   └── errors.go                        # ErrDynLibDepsRequired メッセージ汎用化
│
cmd/
└── record/
    └── main.go                          # MachODynLibAnalyzer 注入
```

> **NOTE（`elfDynlibAnalyzer` リネームについて）**: 02_architecture.md では `dynlibAnalyzer` → `elfDynlibAnalyzer` へのフィールドリネーム、および `dynlibanalysis` → `elfdynlib` へのパッケージリネームが記述されている。しかし、本タスクのスコープではリネームを実施しない。理由は、リネームは広範なテスト更新を伴い、Mach-O 機能追加の本質とは無関係であるため。リネームが必要な場合は別タスクで対応する。本仕様書では現行のフィールド名 `dynlibAnalyzer`・パッケージ名 `dynlibanalysis` をそのまま使用する。

## 3. 型定義

### 3.1 `fileanalysis/schema.go` — スキーマ拡張

#### 3.1.1 `CurrentSchemaVersion` の変更

```go
// internal/fileanalysis/schema.go

const (
    // CurrentSchemaVersion is the current analysis record schema version.
    // ...（既存のバージョン履歴）...
    // Version 14: Mach-O バイナリについても DynLibDeps を記録する（LC_LOAD_DYLIB 整合性検証）。
    //             Record.AnalysisWarnings フィールドを追加（dynlib 解析の警告格納用）。
    // Load returns SchemaVersionMismatchError for records with schema_version != 14.
    // Store.Update treats older schemas (Actual < Expected) as overwritable;
    // re-running `record` migrates old-schema records automatically (--force not required).
    // Store.Update rejects newer schemas (Actual > Expected) to preserve forward compatibility.
    CurrentSchemaVersion = 14
)
```

#### 3.1.2 `Record` 構造体の拡張

```go
// Record（追加フィールドのみ記載）
type Record struct {
    // ... 既存フィールド省略 ...

    // AnalysisWarnings contains warning messages generated during analysis.
    // Used when dynlib dependencies contain unresolvable entries (e.g., unknown @ tokens)
    // that prevent hash verification but do not block recording.
    // nil/empty is omitted from JSON (omitempty).
    AnalysisWarnings []string `json:"analysis_warnings,omitempty"`
}
```

> **NOTE**: 既存の `SyscallAnalysis` 内の `AnalysisWarnings`（`common.SyscallAnalysisResultCore` 内）は syscall 解析固有。dynlib 解析の警告は性質が異なるため `Record` 直下に独立フィールドを追加する。

#### 3.1.3 `LibEntry` の Mach-O での解釈

`LibEntry` は ELF 版で定義済み。Mach-O では以下の意味を持つ：

| フィールド | Mach-O での意味 |
|-----------|----------------|
| `SOName`  | インストール名（例: `@rpath/libFoo.dylib`、`/usr/local/lib/libbar.dylib`） |
| `Path`    | 解決・正規化されたフルパス（`filepath.EvalSymlinks` + `filepath.Clean` 適用後） |
| `Hash`    | `"sha256:<hex>"` 形式のハッシュ値 |

### 3.2 `machodylib/errors.go` — エラー型定義

```go
// Package machodylib provides Mach-O dynamic library dependency analysis
// for LC_LOAD_DYLIB integrity verification.
package machodylib

import (
    "errors"
    "fmt"
    "strings"
)

// ErrNotMachO is returned by openMachO when the file is not a valid Mach-O or
// Fat binary. Callers use errors.Is to distinguish "not Mach-O" (skip silently)
// from real failures such as I/O errors or ErrNoMatchingSlice.
var ErrNotMachO = errors.New("not a Mach-O file")

// ErrLibraryNotResolved indicates that an LC_LOAD_DYLIB install name could not
// be resolved to a filesystem path through any of the available search methods.
type ErrLibraryNotResolved struct {
    InstallName string
    LoaderPath  string
    Tried       []string // all paths that were tried
}

func (e *ErrLibraryNotResolved) Error() string {
    var sb strings.Builder
    fmt.Fprintf(&sb, "failed to resolve dynamic library: %s\n", e.InstallName)
    fmt.Fprintf(&sb, "  loader: %s\n", e.LoaderPath)
    if len(e.Tried) > 0 {
        sb.WriteString("  tried:\n")
        for _, p := range e.Tried {
            fmt.Fprintf(&sb, "    - %s (not found)\n", p)
        }
    }
    return sb.String()
}

// ErrUnknownAtToken indicates that an unrecognized @ prefix token was found
// in an install name. Only @executable_path, @loader_path, and @rpath are supported.
type ErrUnknownAtToken struct {
    InstallName string
    Token       string
}

func (e *ErrUnknownAtToken) Error() string {
    return fmt.Sprintf("unknown @ token in install name: %s (token: %s)",
        e.InstallName, e.Token)
}

// ErrRecursionDepthExceeded indicates that dependency resolution exceeded the
// maximum allowed depth. This typically indicates an abnormal library configuration.
type ErrRecursionDepthExceeded struct {
    Depth    int
    MaxDepth int
    SOName   string
}

func (e *ErrRecursionDepthExceeded) Error() string {
    return fmt.Sprintf("dependency resolution depth exceeded: %s at depth %d (max %d)",
        e.SOName, e.Depth, e.MaxDepth)
}

// ErrNoMatchingSlice indicates that a Fat binary does not contain a slice
// matching the native architecture.
type ErrNoMatchingSlice struct {
    BinaryPath string
    GOARCH     string
}

func (e *ErrNoMatchingSlice) Error() string {
    return fmt.Sprintf("no matching architecture slice in Fat binary: %s (GOARCH=%s)",
        e.BinaryPath, e.GOARCH)
}
```

`ErrLibraryHashMismatch`・`ErrEmptyLibraryPath`・`ErrDynLibDepsRequired` は `dynlibanalysis` パッケージの既存型を再利用する。

### 3.3 `machodylib/shared_cache.go` — dyld shared cache 判定

```go
package machodylib

import "strings"

// systemLibPrefixes are install name prefixes for libraries typically found
// in the dyld shared cache. When a library with one of these prefixes cannot
// be found on the filesystem, it is assumed to reside in the dyld shared cache
// and is skipped (hash verification delegated to code signing).
var systemLibPrefixes = []string{
    "/usr/lib/",
    "/usr/libexec/",
    "/System/Library/",
    "/Library/Apple/",
}

// IsDyldSharedCacheLib reports whether the given install name matches a
// system library prefix that indicates the library is likely part of the
// dyld shared cache.
//
// This function should only be called AFTER Resolve has failed (file not found
// on the filesystem). The combination of "system prefix + file not found"
// satisfies FR-3.1.5's two-condition test for dyld shared cache membership.
func IsDyldSharedCacheLib(installName string) bool {
    for _, prefix := range systemLibPrefixes {
        if strings.HasPrefix(installName, prefix) {
            return true
        }
    }
    return false
}
```

### 3.4 `machodylib/resolver.go` — ライブラリパス解決エンジン

```go
package machodylib

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// defaultSearchPaths are the fallback search paths used when an install name
// has no @ token and is not an absolute path. Corresponds to
// DYLD_FALLBACK_LIBRARY_PATH defaults: /usr/local/lib, /usr/lib.
var defaultSearchPaths = []string{
    "/usr/local/lib",
    "/usr/lib",
}

// LibraryResolver resolves Mach-O install names to filesystem paths.
// It implements the subset of dyld's path resolution algorithm needed for
// security verification (FR-3.1.2).
type LibraryResolver struct {
    executableDir string // directory of the main binary (@executable_path expansion)
}

// NewLibraryResolver creates a new resolver.
// executableDir is the directory of the main binary (used for @executable_path).
func NewLibraryResolver(executableDir string) *LibraryResolver {
    return &LibraryResolver{executableDir: executableDir}
}

// Resolve resolves an install name to a filesystem path.
//
// Resolution order (FR-3.1.2):
//  1. Absolute path (starts with /, no @ token): use directly
//  2. @executable_path token: expand to executableDir
//  3. @loader_path token: expand to directory of loaderPath
//  4. @rpath token: try each rpath entry, first existing path wins
//  5. Default search paths: /usr/local/lib, /usr/lib
//
// Returns ErrUnknownAtToken for unrecognized @ prefix tokens.
// Returns ErrLibraryNotResolved if no existing file is found.
//
// The returned path is normalized via filepath.EvalSymlinks + filepath.Clean.
func (r *LibraryResolver) Resolve(installName, loaderPath string, rpaths []string) (string, error) {
    var tried []string

    // Check for @ token
    if strings.HasPrefix(installName, "@") {
        token, suffix := splitAtToken(installName)

        switch token {
        case "@executable_path":
            candidate := filepath.Join(r.executableDir, suffix)
            resolved, err := r.tryResolve(candidate)
            if err == nil {
                return resolved, nil
            }
            tried = append(tried, candidate)
            return "", &ErrLibraryNotResolved{
                InstallName: installName,
                LoaderPath:  loaderPath,
                Tried:       tried,
            }

        case "@loader_path":
            loaderDir := filepath.Dir(loaderPath)
            candidate := filepath.Join(loaderDir, suffix)
            resolved, err := r.tryResolve(candidate)
            if err == nil {
                return resolved, nil
            }
            tried = append(tried, candidate)
            return "", &ErrLibraryNotResolved{
                InstallName: installName,
                LoaderPath:  loaderPath,
                Tried:       tried,
            }

        case "@rpath":
            for _, rp := range rpaths {
                // LC_RPATH entries may contain @executable_path or @loader_path
                expandedRpath := r.expandRpathEntry(rp, loaderPath)
                candidate := filepath.Join(expandedRpath, suffix)
                resolved, err := r.tryResolve(candidate)
                if err == nil {
                    return resolved, nil
                }
                tried = append(tried, candidate)
            }
            return "", &ErrLibraryNotResolved{
                InstallName: installName,
                LoaderPath:  loaderPath,
                Tried:       tried,
            }

        default:
            // Unknown @ token (e.g., @loader_rpath, @rpath_fallback)
            return "", &ErrUnknownAtToken{
                InstallName: installName,
                Token:       token,
            }
        }
    }

    // Absolute path (no @ token, starts with /)
    if filepath.IsAbs(installName) {
        resolved, err := r.tryResolve(installName)
        if err == nil {
            return resolved, nil
        }
        return "", &ErrLibraryNotResolved{
            InstallName: installName,
            LoaderPath:  loaderPath,
            Tried:       []string{installName},
        }
    }

    // Relative name without @ token: search default paths
    basename := filepath.Base(installName)
    for _, dir := range defaultSearchPaths {
        candidate := filepath.Join(dir, basename)
        resolved, err := r.tryResolve(candidate)
        if err == nil {
            return resolved, nil
        }
        tried = append(tried, candidate)
    }

    return "", &ErrLibraryNotResolved{
        InstallName: installName,
        LoaderPath:  loaderPath,
        Tried:       tried,
    }
}

// expandRpathEntry expands @executable_path and @loader_path tokens
// within an LC_RPATH entry.
func (r *LibraryResolver) expandRpathEntry(rpathEntry, loaderPath string) string {
    if strings.HasPrefix(rpathEntry, "@executable_path") {
        suffix := strings.TrimPrefix(rpathEntry, "@executable_path")
        suffix = strings.TrimPrefix(suffix, "/")
        return filepath.Clean(filepath.Join(r.executableDir, suffix))
    }
    if strings.HasPrefix(rpathEntry, "@loader_path") {
        suffix := strings.TrimPrefix(rpathEntry, "@loader_path")
        suffix = strings.TrimPrefix(suffix, "/")
        loaderDir := filepath.Dir(loaderPath)
        return filepath.Clean(filepath.Join(loaderDir, suffix))
    }
    return rpathEntry
}

// splitAtToken splits an install name like "@rpath/libFoo.dylib" into
// ("@rpath", "libFoo.dylib"). The suffix does not include the leading separator.
func splitAtToken(installName string) (token, suffix string) {
    idx := strings.Index(installName, "/")
    if idx < 0 {
        return installName, ""
    }
    return installName[:idx], installName[idx+1:]
}

// tryResolve checks if the candidate path exists and resolves it via
// filepath.EvalSymlinks + filepath.Clean for normalization.
func (r *LibraryResolver) tryResolve(candidate string) (string, error) {
    // Check if the file exists; distinguishes not-found from other errors
    // (e.g., permission denied) consistent with the ELF resolver.
    if _, err := os.Lstat(candidate); err != nil {
        return "", err
    }
    resolved, err := filepath.EvalSymlinks(candidate)
    if err != nil {
        return "", fmt.Errorf("failed to resolve symlinks for %s: %w", candidate, err)
    }
    return filepath.Clean(resolved), nil
}
```

### 3.5 `machodylib/analyzer.go` — `record` 時の BFS 依存解決

```go
package machodylib

import (
    "bytes"
    "crypto/sha256"
    "debug/macho"
    "encoding/binary"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "log/slog"
    "os"
    "path/filepath"
    "runtime"

    "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

const (
    // MaxRecursionDepth is the maximum depth for recursive dependency resolution.
    // Normal macOS binaries have 3-5 levels of dependencies; exceeding this limit
    // indicates an abnormal configuration or missed circular dependency.
    MaxRecursionDepth = 20
)

// AnalysisWarning holds information about an unresolvable dependency
// (e.g., unknown @ token) that does not block recording but prevents
// hash verification for that particular library.
type AnalysisWarning struct {
    InstallName string // the unresolved install name
    Reason      string // human-readable reason (e.g., "unknown @ token: @loader_rpath")
}

// String returns a formatted warning string suitable for Record.AnalysisWarnings.
func (w AnalysisWarning) String() string {
    return fmt.Sprintf("dynlib warning: %s: %s", w.Reason, w.InstallName)
}

// MachODynLibAnalyzer resolves and records dynamic library dependencies
// for Mach-O binaries (LC_LOAD_DYLIB / LC_LOAD_WEAK_DYLIB).
type MachODynLibAnalyzer struct {
    fs safefileio.FileSystem
}

// NewMachODynLibAnalyzer creates a new analyzer.
func NewMachODynLibAnalyzer(fs safefileio.FileSystem) *MachODynLibAnalyzer {
    return &MachODynLibAnalyzer{fs: fs}
}

// bfsItem represents a pending library to resolve in the BFS queue.
type bfsItem struct {
    installName string   // LC_LOAD_DYLIB install name (stored as SOName in LibEntry)
    loaderPath  string   // path of the Mach-O that has this dependency
    rpaths      []string // LC_RPATH entries of the loader
    isWeak      bool     // true for LC_LOAD_WEAK_DYLIB
    depth       int      // current recursion depth
}

// Analyze resolves all direct and transitive LC_LOAD_DYLIB / LC_LOAD_WEAK_DYLIB
// dependencies of the given Mach-O binary, computes their hashes, and returns
// a []LibEntry snapshot along with any analysis warnings.
//
// Returns (nil, nil, nil) if the file is not Mach-O or has no LC_LOAD_DYLIB entries.
// Returns an error if any LC_LOAD_DYLIB (strong) dependency cannot be resolved.
// LC_LOAD_WEAK_DYLIB resolution failures are skipped with an info log.
// dyld shared cache libraries are skipped (not included in DynLibDeps).
// Unknown @ tokens generate warnings and skip the library.
func (a *MachODynLibAnalyzer) Analyze(binaryPath string) ([]fileanalysis.LibEntry, []AnalysisWarning, error) {
    // Open and parse Mach-O (or Fat binary)
    machoFile, closer, err := a.openMachO(binaryPath)
    if err != nil {
        if errors.Is(err, ErrNotMachO) {
            return nil, nil, nil // not a Mach-O file; skip silently
        }
        return nil, nil, err // I/O error, ErrNoMatchingSlice, etc.
    }
    defer func() { _ = closer.Close() }()
    defer func() { _ = machoFile.Close() }()

    // Extract load commands: LC_LOAD_DYLIB, LC_LOAD_WEAK_DYLIB, LC_RPATH
    deps, rpaths := extractLoadCommands(machoFile)
    if len(deps) == 0 {
        return nil, nil, nil // no dynamic dependencies
    }

    executableDir := filepath.Dir(binaryPath)
    resolver := NewLibraryResolver(executableDir)

    // BFS queue and visited set
    var queue []bfsItem
    visited := make(map[string]struct{})
    var libs []fileanalysis.LibEntry
    var warnings []AnalysisWarning

    // Seed queue with direct dependencies
    for _, dep := range deps {
        queue = append(queue, bfsItem{
            installName: dep.installName,
            loaderPath:  binaryPath,
            rpaths:      rpaths,
            isWeak:      dep.isWeak,
            depth:       1,
        })
    }

    // Process BFS queue
    for len(queue) > 0 {
        item := queue[0]
        queue = queue[1:]

        // Check depth limit
        if item.depth > MaxRecursionDepth {
            return nil, nil, &ErrRecursionDepthExceeded{
                Depth:    item.depth,
                MaxDepth: MaxRecursionDepth,
                SOName:   item.installName,
            }
        }

        // Resolve install name to filesystem path
        resolvedPath, err := resolver.Resolve(item.installName, item.loaderPath, item.rpaths)
        if err != nil {
            // Check for unknown @ token
            if unknownErr, ok := err.(*ErrUnknownAtToken); ok {
                warnings = append(warnings, AnalysisWarning{
                    InstallName: item.installName,
                    Reason:      unknownErr.Error(),
                })
                continue
            }

            // Resolution failed: check if dyld shared cache library
            if IsDyldSharedCacheLib(item.installName) {
                slog.Info("dynlib: skipping dyld shared cache library (delegating to code signing)",
                    "install_name", item.installName)
                continue
            }

            // Weak dependency: skip with info log
            if item.isWeak {
                slog.Info("dynlib: skipping unresolved weak dependency",
                    "install_name", item.installName,
                    "loader", item.loaderPath)
                continue
            }

            // Strong dependency resolution failure: abort recording
            return nil, nil, fmt.Errorf("failed to resolve LC_LOAD_DYLIB dependency: %w", err)
        }

        // Skip already-visited libraries (cycle prevention)
        if _, ok := visited[resolvedPath]; ok {
            continue
        }
        visited[resolvedPath] = struct{}{}

        // Compute hash using safefileio
        hash, err := computeFileHash(a.fs, resolvedPath)
        if err != nil {
            return nil, nil, fmt.Errorf("failed to compute hash for %s: %w", resolvedPath, err)
        }

        // Record the library entry
        libs = append(libs, fileanalysis.LibEntry{
            SOName: item.installName,
            Path:   resolvedPath,
            Hash:   hash,
        })

        // Parse child dependencies for BFS continuation
        childDeps, childRpaths, parseErr := a.parseMachODeps(resolvedPath)
        if parseErr != nil {
            slog.Debug("Failed to parse child Mach-O dependencies",
                "path", resolvedPath, "error", parseErr)
            continue
        }

        for _, childDep := range childDeps {
            queue = append(queue, bfsItem{
                installName: childDep.installName,
                loaderPath:  resolvedPath,
                rpaths:      childRpaths,
                isWeak:      childDep.isWeak,
                depth:       item.depth + 1,
            })
        }
    }

    if len(libs) == 0 {
        return nil, warnings, nil
    }

    return libs, warnings, nil
}

// depEntry holds a dependency's install name and its weak/strong classification.
type depEntry struct {
    installName string
    isWeak      bool
}

// openMachO opens the file at binaryPath and returns a *macho.File along with
// an io.Closer that must be called to release all underlying file descriptors.
// The caller must call closer.Close() when done, regardless of whether
// machoFile.Close() is also called (macho.File.Close does not close the
// underlying os.File / SectionReader source).
//
// For Fat binaries, selects the slice matching runtime.GOARCH.
// Returns ErrNotMachO (via errors.Is) if the file is not a Mach-O or Fat binary.
// Returns ErrNoMatchingSlice if the Fat binary has no slice for the native arch.
// Returns other errors for I/O or permission failures.
func (a *MachODynLibAnalyzer) openMachO(binaryPath string) (*macho.File, io.Closer, error) {
    file, err := a.fs.SafeOpenFile(binaryPath, os.O_RDONLY, 0)
    if err != nil {
        return nil, nil, err
    }

    // Try as Fat binary first
    fatFile, fatErr := macho.NewFatFile(file)
    if fatErr == nil {
        // fatFile.Close() does NOT close the underlying io.ReaderAt (file),
        // so we must close file explicitly in all paths.
        // Defer close until after Arches is fully read.
        cpuType := goarchToCPUType(runtime.GOARCH)
        if cpuType == 0 {
            _ = fatFile.Close()
            _ = file.Close()
            return nil, nil, &ErrNoMatchingSlice{BinaryPath: binaryPath, GOARCH: runtime.GOARCH}
        }
        for _, arch := range fatFile.Arches {
            if arch.Cpu == cpuType {
                _ = fatFile.Close()
                // Close the original file handle and re-open for the slice.
                _ = file.Close()
                sliceFile, err := a.fs.SafeOpenFile(binaryPath, os.O_RDONLY, 0)
                if err != nil {
                    return nil, nil, err
                }
                machoFile, err := macho.NewFile(
                    io.NewSectionReader(sliceFile, int64(arch.Offset), int64(arch.Size)))
                if err != nil {
                    _ = sliceFile.Close()
                    return nil, nil, fmt.Errorf("%w: %w", ErrNotMachO, err)
                }
                // Return sliceFile as the closer: macho.File.Close does not
                // close the SectionReader's underlying source.
                return machoFile, sliceFile, nil
            }
        }
        _ = fatFile.Close()
        _ = file.Close()
        return nil, nil, &ErrNoMatchingSlice{BinaryPath: binaryPath, GOARCH: runtime.GOARCH}
    }

    // Try as single-architecture Mach-O
    // Reset file position (NewFatFile may have consumed bytes)
    if seeker, ok := file.(io.Seeker); ok {
        if _, err := seeker.Seek(0, io.SeekStart); err != nil {
            _ = file.Close()
            return nil, nil, err
        }
    }

    machoFile, err := macho.NewFile(file)
    if err != nil {
        _ = file.Close()
        return nil, nil, fmt.Errorf("%w: %w", ErrNotMachO, err)
    }

    // file is the underlying source; return it as the closer.
    return machoFile, file, nil
}

// goarchToCPUType maps runtime.GOARCH to macho.Cpu type.
func goarchToCPUType(goarch string) macho.Cpu {
    switch goarch {
    case "arm64":
        return macho.CpuArm64
    case "amd64":
        return macho.CpuAmd64
    default:
        return 0
    }
}

// loadCmdWeakDylib is LC_LOAD_WEAK_DYLIB (0x80000018).
// Go's debug/macho package does not define this constant as of Go 1.23.
const loadCmdWeakDylib macho.LoadCmd = 0x80000018

// extractLoadCommands extracts LC_LOAD_DYLIB, LC_LOAD_WEAK_DYLIB, and LC_RPATH
// entries from the Mach-O file's load commands.
//
// Implementation note: macho.File.ImportedLibraries() returns all dependency names
// but does not distinguish LC_LOAD_DYLIB from LC_LOAD_WEAK_DYLIB. This function
// walks macho.File.Loads directly to extract both the install name and the command
// type (FR-3.1.1).
func extractLoadCommands(f *macho.File) (deps []depEntry, rpaths []string) {
    for _, load := range f.Loads {
        raw := load.Raw()
        if len(raw) < 8 {
            continue
        }
        cmd := f.ByteOrder.Uint32(raw[0:4])
        switch macho.LoadCmd(cmd) {
        case macho.LoadCmdDylib: // LC_LOAD_DYLIB = 0xC
            name := extractDylibName(raw, f.ByteOrder)
            if name != "" {
                deps = append(deps, depEntry{installName: name, isWeak: false})
            }
        case loadCmdWeakDylib: // LC_LOAD_WEAK_DYLIB = 0x80000018
            name := extractDylibName(raw, f.ByteOrder)
            if name != "" {
                deps = append(deps, depEntry{installName: name, isWeak: true})
            }
        case macho.LoadCmdRpath: // LC_RPATH = 0x8000001C
            path := extractRpathName(raw, f.ByteOrder)
            if path != "" {
                rpaths = append(rpaths, path)
            }
        }
    }
    return
}

// extractDylibName extracts the library name from an LC_LOAD_DYLIB or
// LC_LOAD_WEAK_DYLIB load command's raw bytes.
// Layout: cmd(4) + cmdsize(4) + name_offset(4) + timestamp(4) + current_version(4)
// + compat_version(4) + name string (null-terminated).
func extractDylibName(raw []byte, bo binary.ByteOrder) string {
    if len(raw) < 12 {
        return ""
    }
    nameOffset := bo.Uint32(raw[8:12])
    if int(nameOffset) >= len(raw) {
        return ""
    }
    name := raw[nameOffset:]
    if idx := bytes.IndexByte(name, 0); idx >= 0 {
        name = name[:idx]
    }
    return string(name)
}

// extractRpathName extracts the path from an LC_RPATH load command's raw bytes.
// Layout: cmd(4) + cmdsize(4) + path_offset(4) + path string (null-terminated).
func extractRpathName(raw []byte, bo binary.ByteOrder) string {
    if len(raw) < 12 {
        return ""
    }
    pathOffset := bo.Uint32(raw[8:12])
    if int(pathOffset) >= len(raw) {
        return ""
    }
    path := raw[pathOffset:]
    if idx := bytes.IndexByte(path, 0); idx >= 0 {
        path = path[:idx]
    }
    return string(path)
}

// parseMachODeps opens a resolved .dylib and extracts its LC_LOAD_DYLIB /
// LC_LOAD_WEAK_DYLIB / LC_RPATH entries for BFS continuation.
// Returns nil slices (not an error) if parsing fails.
func (a *MachODynLibAnalyzer) parseMachODeps(path string) ([]depEntry, []string, error) {
    file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return nil, nil, err
    }
    defer func() { _ = file.Close() }()

    machoFile, err := macho.NewFile(file)
    if err != nil {
        return nil, nil, err
    }
    defer func() { _ = machoFile.Close() }()

    deps, rpaths := extractLoadCommands(machoFile)
    return deps, rpaths, nil
}

// computeFileHash computes the SHA256 hash of the file at the given path
// using safefileio for symlink attack prevention.
// Uses streaming (SafeOpenFile + io.Copy) to avoid loading large libraries into memory.
//
// Note: This is functionally identical to dynlibanalysis.computeFileHash but
// is defined separately to avoid a circular import between machodylib and
// dynlibanalysis. Both implementations use the same algorithm (SHA256) and
// format ("sha256:<hex>").
func computeFileHash(fs safefileio.FileSystem, path string) (string, error) {
    file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return "", err
    }
    defer func() { _ = file.Close() }()

    h := sha256.New()
    if _, err := io.Copy(h, file); err != nil {
        return "", fmt.Errorf("failed to hash %s: %w", path, err)
    }
    return fmt.Sprintf("sha256:%s", hex.EncodeToString(h.Sum(nil))), nil
}

// HasDynamicLibDeps checks if the file at the given path is a Mach-O binary
// that has at least one LC_LOAD_DYLIB or LC_LOAD_WEAK_DYLIB entry pointing to
// a non-dyld-shared-cache library.
//
// Used by runner to detect Mach-O binaries that should have DynLibDeps recorded
// but don't (e.g., recorded before this feature was added).
//
// Returns (false, nil) for non-Mach-O files, Mach-O files with no dependencies,
// or Mach-O files whose dependencies are all dyld shared cache libraries.
func HasDynamicLibDeps(path string, fs safefileio.FileSystem) (bool, error) {
    file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return false, fmt.Errorf("failed to open binary for Mach-O inspection: %w", err)
    }
    defer func() { _ = file.Close() }()

    // Try as Fat binary first
    fatFile, fatErr := macho.NewFatFile(file)
    if fatErr == nil {
        // Defer close until after Arches is fully read.
        cpuType := goarchToCPUType(runtime.GOARCH)
        if cpuType == 0 {
            _ = fatFile.Close()
            return false, nil
        }
        for _, arch := range fatFile.Arches {
            if arch.Cpu == cpuType {
                _ = fatFile.Close()
                // Re-open for the matching slice
                sliceFile, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
                if err != nil {
                    return false, err
                }
                defer func() { _ = sliceFile.Close() }()
                machoFile, err := macho.NewFile(
                    io.NewSectionReader(sliceFile, int64(arch.Offset), int64(arch.Size)))
                if err != nil {
                    return false, nil
                }
                defer func() { _ = machoFile.Close() }()
                deps, _ := extractLoadCommands(machoFile)
                for _, dep := range deps {
                    // Treat as dyld shared cache only when the install name is
                    // system-prefixed AND the file is absent from disk.
                    if IsDyldSharedCacheLib(dep.installName) {
                        if _, err := os.Stat(dep.installName); os.IsNotExist(err) {
                            continue
                        }
                    }
                    return true, nil
                }
                return false, nil
            }
        }
        _ = fatFile.Close()
        return false, nil // no matching architecture
    }

    // Try as single-architecture Mach-O
    if seeker, ok := file.(io.Seeker); ok {
        if _, err := seeker.Seek(0, io.SeekStart); err != nil {
            return false, nil
        }
    }

    machoFile, err := macho.NewFile(file)
    if err != nil {
        // Not a Mach-O binary
        return false, nil
    }
    defer func() { _ = machoFile.Close() }()

    deps, _ := extractLoadCommands(machoFile)
    for _, dep := range deps {
        // Treat as dyld shared cache only when the install name is
        // system-prefixed AND the file is absent from disk.
        if IsDyldSharedCacheLib(dep.installName) {
            if _, err := os.Stat(dep.installName); os.IsNotExist(err) {
                continue
            }
        }
        return true, nil
    }
    return false, nil
}
```

> **`computeFileHash` の重複について**: `dynlibanalysis.computeFileHash` と同一ロジックだが、`machodylib` → `dynlibanalysis` の import は循環依存を引き起こさないものの、将来 `dynlibanalysis` を `elfdynlib` にリネームした場合に不自然な依存となる。実装時に共通ユーティリティパッケージへの切り出しを検討してもよいが、YAGNI の観点で本タスクでは重複を許容する。

### 3.6 `dynlibanalysis/errors.go` — エラーメッセージの汎用化

`ErrDynLibDepsRequired` のエラーメッセージが "ELF binary" とハードコーディングされているため、Mach-O バイナリに対しても適切なメッセージを返すよう汎用化する。

```go
// internal/dynlibanalysis/errors.go（変更箇所のみ）

func (e *ErrDynLibDepsRequired) Error() string {
    return fmt.Sprintf("dynamic library dependencies not recorded for binary: %s\n"+
        "  please re-run 'record' command",
        e.BinaryPath)
}
```

変更前: `"dynamic library dependencies not recorded for ELF binary: %s\n"...`
変更後: `"dynamic library dependencies not recorded for binary: %s\n"...`

doc comment も合わせて更新する:

変更前: `// ErrDynLibDepsRequired indicates that a DynLibDeps record is required but not present for an ELF binary.`
変更後: `// ErrDynLibDepsRequired indicates that a DynLibDeps record is required but not present for a binary.`

### 3.7 `filevalidator/validator.go` — Mach-O 解析の統合

#### 3.7.1 フィールドとセッターの追加

```go
// internal/filevalidator/validator.go

import (
    // ... 既存 imports ...
    "github.com/isseis/go-safe-cmd-runner/internal/machodylib"
)

type Validator struct {
    // ... 既存フィールド ...
    dynlibAnalyzer      *dynlibanalysis.DynLibAnalyzer    // ELF 用（既存）
    machoDynlibAnalyzer *machodylib.MachODynLibAnalyzer   // Mach-O 用（新規）
    // ... 他の既存フィールド ...
}

// SetMachODynLibAnalyzer injects the MachODynLibAnalyzer used during record operations.
// Call before the first Record() invocation. Safe to call with nil (disables Mach-O dynlib analysis).
func (v *Validator) SetMachODynLibAnalyzer(a *machodylib.MachODynLibAnalyzer) {
    v.machoDynlibAnalyzer = a
}
```

#### 3.7.2 `updateAnalysisRecord` コールバックの拡張

```go
// updateAnalysisRecord 内の store.Update コールバック（DynLibDeps 解析部分）:

// Analyze dynamic library dependencies if analyzer is available.
// Analysis failure causes the callback to return an error, which
// prevents the record from being persisted (atomicity).
//
// DynLibDeps and AnalysisWarnings are always reset before analysis to prevent
// stale data when a file type changes (e.g., ELF -> Mach-O on --force re-record).
record.DynLibDeps = nil
record.AnalysisWarnings = nil

// ELF analysis (existing)
if v.dynlibAnalyzer != nil {
    dynLibDeps, analyzeErr := v.dynlibAnalyzer.Analyze(filePath.String())
    if analyzeErr != nil {
        return fmt.Errorf("dynamic library analysis failed: %w", analyzeErr)
    }
    record.DynLibDeps = dynLibDeps // nil for non-ELF or static ELF
}

// Mach-O analysis (new): only run if ELF analysis did not produce results.
// ELF and Mach-O are mutually exclusive formats, but the guard ensures
// correct behavior even if a file is replaced between analyses.
if v.machoDynlibAnalyzer != nil && len(record.DynLibDeps) == 0 {
    libs, warns, analyzeErr := v.machoDynlibAnalyzer.Analyze(filePath.String())
    if analyzeErr != nil {
        return fmt.Errorf("Mach-O dynamic library analysis failed: %w", analyzeErr)
    }
    record.DynLibDeps = libs // nil for non-Mach-O or Mach-O without LC_LOAD_DYLIB
    if len(warns) > 0 {
        for _, w := range warns {
            record.AnalysisWarnings = append(record.AnalysisWarnings, w.String())
        }
    }
}
```

> **NOTE**: `record.DynLibDeps = nil` と `record.AnalysisWarnings = nil` の初期化により、`--force` での再記録時に ELF → Mach-O（またはその逆）へファイルが差し替わった場合でも stale データが残らない。この初期化は本タスクで新たに追加する変更であり、現行の ELF 解析コードには含まれていない。既存の ELF テストへの影響を確認すること。

### 3.8 `verification/manager.go` — Mach-O 動的依存チェックの追加

```go
// internal/verification/manager.go（拡張）

import (
    // ... 既存 imports ...
    "github.com/isseis/go-safe-cmd-runner/internal/machodylib"
)

// verifyDynLibDeps 内の「DynLibDeps なし」分岐に Mach-O チェックを追加する。
// 既存の ELF チェック後に実行される。

func (m *Manager) verifyDynLibDeps(cmdPath string) error {
    // ... 既存: record ロード、スキーマバージョン処理（変更なし） ...

    if len(record.DynLibDeps) > 0 {
        return m.dynlibVerifier.Verify(record.DynLibDeps)
    }

    // DynLibDeps is not recorded: check if this is a dynamically linked binary.
    // ELF check (existing)
    hasDynDeps, err := m.hasDynamicLibraryDeps(cmdPath)
    if err != nil {
        return fmt.Errorf("failed to check dynamic library dependencies: %w", err)
    }
    if hasDynDeps {
        return &dynlibanalysis.ErrDynLibDepsRequired{BinaryPath: cmdPath}
    }

    // Mach-O check (new)
    hasMachoDynDeps, err := m.hasMachODynamicLibraryDeps(cmdPath)
    if err != nil {
        return fmt.Errorf("failed to check Mach-O dynamic library dependencies: %w", err)
    }
    if hasMachoDynDeps {
        return &dynlibanalysis.ErrDynLibDepsRequired{BinaryPath: cmdPath}
    }

    return nil
}

// hasMachODynamicLibraryDeps checks if the file at the given path is a Mach-O binary
// that has at least one non-dyld-shared-cache dynamic library dependency.
func (m *Manager) hasMachODynamicLibraryDeps(path string) (bool, error) {
    return machodylib.HasDynamicLibDeps(path, m.safeFS)
}
```

### 3.9 `cmd/record/main.go` — `MachODynLibAnalyzer` 注入

```go
// cmd/record/main.go

import (
    // ... 既存 imports ...
    "github.com/isseis/go-safe-cmd-runner/internal/machodylib"
)

type deps struct {
    // ... 既存フィールド ...
    dynlibAnalyzerFactory      func() *dynlibanalysis.DynLibAnalyzer      // 既存
    machoDynlibAnalyzerFactory func() *machodylib.MachODynLibAnalyzer     // 新規
    // ...
}

// defaultDeps (or equivalent initialization):
var defaultDeps = deps{
    // ... 既存 ...
    dynlibAnalyzerFactory: func() *dynlibanalysis.DynLibAnalyzer {
        return dynlibanalysis.NewDynLibAnalyzer(safefileio.NewFileSystem(safefileio.FileSystemConfig{}))
    },
    machoDynlibAnalyzerFactory: func() *machodylib.MachODynLibAnalyzer {
        return machodylib.NewMachODynLibAnalyzer(safefileio.NewFileSystem(safefileio.FileSystemConfig{}))
    },
    // ...
}

// run() 内の analyzer 注入部分:
if fv, ok := validator.(*filevalidator.Validator); ok {
    if d.dynlibAnalyzerFactory != nil {
        fv.SetDynLibAnalyzer(d.dynlibAnalyzerFactory())
    }
    // NEW: Inject Mach-O dynamic library analyzer
    if d.machoDynlibAnalyzerFactory != nil {
        fv.SetMachODynLibAnalyzer(d.machoDynlibAnalyzerFactory())
    }
    fv.SetBinaryAnalyzer(security.NewBinaryAnalyzer())
    // ... 既存の syscall analyzer 注入 ...
}
```

## 4. テスト実装詳細

### 4.1 受入基準とテストの対応表

| 受入基準 (AC) | テストケース | テストファイル |
|-------------|------------|-------------|
| AC-1: パス解決（絶対パス） | `TestResolve_AbsolutePath` | `resolver_test.go` |
| AC-1: `@executable_path` 展開 | `TestResolve_ExecutablePath` | `resolver_test.go` |
| AC-1: `@loader_path` 展開 | `TestResolve_LoaderPath` | `resolver_test.go` |
| AC-1: `@rpath` 展開（単一） | `TestResolve_Rpath_Single` | `resolver_test.go` |
| AC-1: `@rpath` 展開（複数） | `TestResolve_Rpath_Multiple` | `resolver_test.go` |
| AC-1: `LC_RPATH` 内の `@executable_path` | `TestResolve_RpathWithExecutablePath` | `resolver_test.go` |
| AC-1: デフォルト検索パス | `TestResolve_DefaultPaths` | `resolver_test.go` |
| AC-1: dyld shared cache スキップ | `TestIsDyldSharedCacheLib` | `shared_cache_test.go` |
| AC-1: 未知 `@` トークン | `TestResolve_UnknownAtToken` | `resolver_test.go` |
| AC-1: 解決失敗（`LC_LOAD_DYLIB`） | `TestAnalyze_StrongDependencyResolutionFailure` | `analyzer_test.go` |
| AC-1: 解決失敗（`LC_LOAD_WEAK_DYLIB`） | `TestAnalyze_WeakDependencyResolutionFailure` | `analyzer_test.go` |
| AC-1: `LC_LOAD_DYLIB` エントリなし | `TestAnalyze_StaticMachO` | `analyzer_test.go` |
| AC-1: エラーメッセージにインストール名 | `TestAnalyze_ErrorMessageContainsInstallName` | `analyzer_test.go` |
| AC-1: 間接依存の再帰解決 | `TestAnalyze_TransitiveDeps` | `analyzer_test.go` |
| AC-1: 間接依存の `@rpath` 解決 | `TestAnalyze_TransitiveRpathResolution` | `analyzer_test.go` |
| AC-1: 循環依存防止 | `TestAnalyze_CircularDeps` | `analyzer_test.go` |
| AC-1: 再帰深度超過 | `TestAnalyze_MaxDepthExceeded` | `analyzer_test.go` |
| AC-1: Fat バイナリのスライス選択 | `TestOpenMachO_FatBinary` | `analyzer_test.go` |
| AC-2: `DynLibDeps` 記録 | `TestAnalyze_DynamicMachO` | `analyzer_test.go` |
| AC-2: `LibEntry` フィールド内容 | `TestAnalyze_LibEntryFields` | `analyzer_test.go` |
| AC-2: 非 Mach-O ファイル | `TestAnalyze_NonMachO` | `analyzer_test.go` |
| AC-2: dyld shared cache のみ | `TestAnalyze_DyldSharedCacheOnly` | `analyzer_test.go` |
| AC-2: 未知トークン警告 | `TestAnalyze_UnknownTokenWarning` | `analyzer_test.go` |
| AC-2: `--force` での再記録 | `TestRecord_Force_MachO` | `validator_test.go` |
| AC-3: ハッシュ一致で実行許可 | `TestVerifyDynLibDeps_MachO_HashMatch` | `manager_test.go` |
| AC-3: ハッシュ不一致でブロック | `TestVerifyDynLibDeps_MachO_HashMismatch` | `manager_test.go` |
| AC-3: ハッシュ不一致エラーメッセージ | `TestVerifyDynLibDeps_MachO_HashMismatch_ErrorMessage` | `manager_test.go` |
| AC-3: `path: ""` でブロック | `TestVerifyDynLibDeps_MachO_EmptyPath` | `manager_test.go` |
| AC-3: 旧スキーマ（< 14）でブロック | `TestVerifyDynLibDeps_OldSchema_MachO_Blocked` | `manager_test.go` |
| AC-3: 新スキーマ（>= 14）`DynLibDeps` なしでブロック | `TestVerifyDynLibDeps_NewSchema_MachO_NoDynLibDeps` | `manager_test.go` |
| AC-3: 非 Mach-O で `DynLibDeps` なし | `TestVerifyDynLibDeps_NonMachO_NoDynLibDeps` | `manager_test.go` |
| AC-3: `HasDynamicLibDeps` 判定 | `TestHasDynamicLibDeps_MachO`, `TestHasDynamicLibDeps_NonMachO`, `TestHasDynamicLibDeps_FatBinary` | `analyzer_test.go` |
| AC-4: 既存 ELF 検証の非影響 | 既存テストスイートの全パス | 既存テストファイル |
| AC-4: 既存 `ContentHash` 検証の非影響 | 既存テストスイートの全パス | 既存テストファイル |

### 4.2 ユニットテスト

#### 4.2.1 `resolver_test.go`

```go
func TestResolve_AbsolutePath(t *testing.T) {
    // Setup: create temp .dylib at a known absolute path
    // Verify: absolute install name resolves to that path
}

func TestResolve_ExecutablePath(t *testing.T) {
    // Setup: create temp dir simulating executable's directory, place .dylib there
    // Use @executable_path/libFoo.dylib as install name
    // Verify: resolved to executableDir/libFoo.dylib
}

func TestResolve_LoaderPath(t *testing.T) {
    // Setup: create temp dir simulating loader's directory, place .dylib there
    // Use @loader_path/libFoo.dylib as install name
    // Verify: resolved to dir(loaderPath)/libFoo.dylib
}

func TestResolve_Rpath_Single(t *testing.T) {
    // Setup: create temp dir, place .dylib in it
    // Provide single LC_RPATH pointing to temp dir
    // Use @rpath/libFoo.dylib as install name
    // Verify: resolved to tempDir/libFoo.dylib
}

func TestResolve_Rpath_Multiple(t *testing.T) {
    // Setup: create two temp dirs, place .dylib in second
    // Provide two LC_RPATH entries
    // Verify: first dir misses, second dir resolves correctly
}

func TestResolve_RpathWithExecutablePath(t *testing.T) {
    // Setup: LC_RPATH = @executable_path/../lib
    // Place .dylib in executableDir/../lib/
    // Verify: double expansion works correctly
}

func TestResolve_DefaultPaths(t *testing.T) {
    // Note: May require filesystem mocking or skip on CI
    // Verify: /usr/local/lib is tried before /usr/lib
}

func TestResolve_UnknownAtToken(t *testing.T) {
    // Use @loader_rpath/libFoo.dylib as install name
    // Verify: ErrUnknownAtToken with correct Token and InstallName
}

func TestResolve_Failure(t *testing.T) {
    // Setup: no library exists in any search path
    // Verify: ErrLibraryNotResolved with InstallName and Tried paths
}
```

#### 4.2.2 `shared_cache_test.go`

```go
func TestIsDyldSharedCacheLib(t *testing.T) {
    tests := []struct {
        name        string
        installName string
        expected    bool
    }{
        {"usr/lib system lib", "/usr/lib/libSystem.B.dylib", true},
        {"usr/libexec", "/usr/libexec/something.dylib", true},
        {"System/Library", "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", true},
        {"Library/Apple", "/Library/Apple/usr/lib/libFoo.dylib", true},
        {"non-system path", "/usr/local/lib/libFoo.dylib", false},
        {"homebrew path", "/opt/homebrew/lib/libFoo.dylib", false},
        {"relative path", "libFoo.dylib", false},
        {"rpath token", "@rpath/libFoo.dylib", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.expected, machodylib.IsDyldSharedCacheLib(tt.installName))
        })
    }
}
```

#### 4.2.3 `analyzer_test.go`（コンポーネントテスト）

```go
func TestAnalyze_NonMachO(t *testing.T) {
    // Setup: create a non-Mach-O file (e.g., text file)
    // Verify: returns (nil, nil, nil)
}

func TestAnalyze_DyldSharedCacheOnly(t *testing.T) {
    // Setup: Mach-O with only /usr/lib/... dependencies (all dyld shared cache)
    // Verify: returns (nil, nil/warnings, nil)
}

func TestAnalyze_UnknownTokenWarning(t *testing.T) {
    // Setup: Mach-O with @loader_rpath/libFoo.dylib dependency
    // Verify: returns (nil/libs, warnings, nil); warnings contains the unknown token
}

func TestAnalyze_StrongDependencyResolutionFailure(t *testing.T) {
    // Setup: Mach-O with LC_LOAD_DYLIB pointing to non-existent non-system library
    // Verify: returns error (ErrLibraryNotResolved wrapped)
}

func TestAnalyze_WeakDependencyResolutionFailure(t *testing.T) {
    // Setup: Mach-O with LC_LOAD_WEAK_DYLIB pointing to non-existent non-system library
    // Verify: returns (libs, nil, nil); weak dependency is skipped
}

func TestAnalyze_CircularDeps(t *testing.T) {
    // Setup: two .dylib files that depend on each other
    // Verify: BFS terminates (visited set prevents re-analysis)
}

func TestAnalyze_MaxDepthExceeded(t *testing.T) {
    // Setup: chain of .dylib dependencies exceeding MaxRecursionDepth
    // Verify: ErrRecursionDepthExceeded
}

func TestHasDynamicLibDeps_MachO(t *testing.T) {
    // Setup: Mach-O with non-system LC_LOAD_DYLIB
    // Verify: (true, nil)
}

func TestHasDynamicLibDeps_NonMachO(t *testing.T) {
    // Setup: non-Mach-O file
    // Verify: (false, nil)
}
```

### 4.3 統合テスト

#### 4.3.1 `record` → `runner` 正常フロー

```go
func TestIntegration_MachO_RecordAndVerify_Normal(t *testing.T) {
    // 1. Use a test Mach-O binary with known LC_LOAD_DYLIB dependencies
    // 2. Run record -> verify DynLibDeps is created with correct entries
    // 3. Run runner verification -> verify success (hash match)
}
```

#### 4.3.2 ライブラリ改ざん検出

```go
func TestIntegration_MachO_LibraryTampering(t *testing.T) {
    // 1. Record a Mach-O binary
    // 2. Modify a dependent .dylib's content (change file bytes)
    // 3. Run runner verification -> verify ErrLibraryHashMismatch
}
```

#### 4.3.3 旧スキーマブロック

```go
func TestIntegration_MachO_OldSchemaBlocked(t *testing.T) {
    // 1. Create a record with schema_version: 13 (pre-Mach-O dynlib support)
    // 2. Run runner verification for a Mach-O binary
    // 3. Verify: execution is blocked with SchemaVersionMismatchError (old schema must re-record)
}
```

#### 4.3.4 `DynLibDeps` 未記録検出（新スキーマ）

```go
func TestIntegration_MachO_NewSchema_NoDynLibDeps(t *testing.T) {
    // 1. Create a record with schema_version: 14 but no DynLibDeps
    //    (simulate a Mach-O binary recorded without the analyzer enabled)
    // 2. Run runner verification for a Mach-O binary with non-system deps
    // 3. Verify: ErrDynLibDepsRequired
}
```

#### 4.3.5 dyld shared cache のみの Mach-O バイナリ

```go
func TestIntegration_MachO_DyldSharedCacheOnly_Allowed(t *testing.T) {
    // 1. Record a Mach-O binary whose dependencies are all in dyld shared cache
    // 2. Verify: DynLibDeps is nil (no entries recorded)
    // 3. Run runner verification -> verify success (HasDynamicLibDeps returns false)
}
```

#### 4.3.6 既存 ELF 検証の非影響

```go
func TestIntegration_ELF_DynLibDeps_StillWorks(t *testing.T) {
    // 1. Record an ELF binary with DT_NEEDED dependencies
    // 2. Run runner verification -> verify success
    // Ensures Mach-O changes don't break existing ELF dynlib verification
}
```

### 4.4 テストフィクスチャ

- テスト用 Mach-O バイナリとダミー `.dylib` をテンポラリディレクトリに配置する
- テスト用バイナリは `testdata/` 配下に配置し、生成スクリプトも同梱する
- macOS CI でのみ実行される Mach-O 固有テストは `//go:build darwin` ビルドタグで制限する
- `LC_RPATH`、`@rpath` インストール名を持つ最小 Mach-O バイナリの生成には以下のアプローチを使用する：
  - C コンパイラ（`clang`）による小さなテスト `.dylib` とバイナリのコンパイル
  - `install_name_tool` による `LC_RPATH` の設定
  - 生成スクリプトを `testdata/` に配置

## 5. 実装チェックリスト

（04_implementation_plan.md に記載）

## 6. 参照

- タスク 0074: ELF DT_NEEDED 整合性検証（ELF 版の詳細仕様）
- タスク 0073: Mach-O ネットワーク操作検出
- タスク 0095: Mach-O 機能パリティ（親タスク）
- [01_requirements.md](01_requirements.md): 要件定義書
- [02_architecture.md](02_architecture.md): アーキテクチャ設計書
