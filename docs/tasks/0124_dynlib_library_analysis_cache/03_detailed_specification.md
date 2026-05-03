# ライブラリ解析結果の共通キャッシュ化 詳細仕様書

## 1. 変更ファイル一覧

| ファイル | 変更種別 | 概要 |
|---------|----------|------|
| `internal/dynlibcache/schema.go` | 新規 | `LibAnalysisCacheFile` 型、スキーマ定数 |
| `internal/dynlibcache/cache.go` | 新規 | `CacheManager`（GetOrCreate） |
| `internal/dynlibcache/errors.go` | 新規 | エラー型 |
| `internal/fileanalysis/schema.go` | 変更 | `LibraryAnalysis` / `DetectedLibraryNetworkDeps` 削除、`DynLibDepsStore` 追加、バージョン 20→21 |
| `internal/fileanalysis/dyn_lib_deps_store.go` | 新規 | `DynLibDepsStore` インタフェースと実装 |
| `internal/filevalidator/validator.go` | 変更 | `CacheManager` 注入、`analyzeLibraries()` をキャッシュ書き込み方式に変更 |
| `internal/runner/base/security/network_analyzer.go` | 変更 | `DynLibDepsStore` / `CacheManager` 注入、ライブラリシグナル集計追加 |
| `cmd/record/main.go` | 変更 | `dynlibcache.NewCacheManager()` 生成と `SetLibraryAnalysisCacheManager()` 呼び出し |
| `cmd/runner/main.go` | 変更 | `dynlibcache.NewCacheManager()` 生成と `NetworkAnalyzer` への注入 |

---

## 2. `internal/dynlibcache/schema.go`（新規）

```go
package dynlibcache

// CacheSchemaVersion is the current schema version for library analysis cache files.
// Version history:
//
//	1 - initial schema
const CacheSchemaVersion = 1

// LibAnalysisCacheFile is the JSON schema for a library analysis cache file.
type LibAnalysisCacheFile struct {
    SchemaVersion        int    `json:"schema_version"`
    LibPath              string `json:"lib_path"`
    LibHash              string `json:"lib_hash"`          // "sha256:<hex>"
    AnalyzedAt           string `json:"analyzed_at"`       // RFC3339
    HasNetworkSignal     bool   `json:"has_network_signal"`
    HasDynamicLoadSignal bool   `json:"has_dynamic_load_signal"`

    // SyscallAnalysis is retained for audit/debug purposes; not used by the runner.
    SyscallAnalysis *fileanalysis.SyscallAnalysisData `json:"syscall_analysis,omitempty"`

    // SymbolAnalysis is retained for audit/debug purposes; not used by the runner.
    SymbolAnalysis *fileanalysis.SymbolAnalysisData `json:"symbol_analysis,omitempty"`
}

// LibAnalysisResult is the in-memory result returned by CacheManager.GetOrCreate.
// Warnings are transient analysis warnings not persisted to the cache file.
type LibAnalysisResult struct {
    HasNetworkSignal     bool
    HasDynamicLoadSignal bool
    Warnings             []string
}
```

---

## 3. `internal/dynlibcache/cache.go`（新規）

### 3.1 CacheManager の構造

```go
type CacheManager struct {
    cacheDir string
    fs       safefileio.FileSystem
    analyzer LibraryAnalyzer
    pathEnc  *pathencoding.SubstitutionHashEscape
}

func NewCacheManager(
    cacheDir string,
    fs safefileio.FileSystem,
    analyzer LibraryAnalyzer,
) (*CacheManager, error)
```

`cacheDir` は存在しない場合、自動作成する（`os.MkdirAll`、パーミッション `0o755`）。

### 3.2 GetOrCreate

```go
// GetOrCreate returns cached analysis or analyzes the library on cache miss.
// libPath must be a normalized absolute path. libHash must be in "sha256:<hex>" format.
// Warnings from analysis on cache miss are included in LibAnalysisResult.Warnings.
func (m *CacheManager) GetOrCreate(libPath, libHash string) (*LibAnalysisResult, error)
```

**処理フロー:**

1. `pathEnc.Encode(libPath)` でキャッシュファイルパスを生成
2. キャッシュファイルを読み込み JSON をパース（失敗 → Cache Miss）
3. `cache.SchemaVersion != CacheSchemaVersion` → Cache Miss
4. `cache.LibHash != libHash` → Cache Miss
5. Cache Hit: `LibAnalysisResult{HasNetworkSignal, HasDynamicLoadSignal}` を構築して返す（`Warnings` は空）
6. Cache Miss: `analyzer.Analyze(libPath)` → `(cacheFile, warnings, err)` を取得し、ファイルを書き込み、`LibAnalysisResult{..., Warnings: warnings}` を返す

**ファイル命名:**

```
<cacheDir>/<pathencoding.SubstitutionHashEscape(libPath)>
```

libc-cache と同じ命名方式。1 ライブラリにつき 1 ファイル。`lib_hash` 変化時は上書き。

**アトミック書き込み:**

`libccache` の `writeFileAtomic`（非公開関数）と同じパターン（一時ファイルへ書き込み → `os.Rename`）を `dynlibcache/cache.go` 内に実装する。`libccache.writeFileAtomic` は非公開のため直接再利用できないが、実装は同一である。

### 3.3 LibraryAnalyzer インタフェース

```go
// LibraryAnalyzer performs analysis of a single dynamic library.
// The concrete implementation lives in filevalidator and wraps the existing
// BinaryAnalyzer / SyscallAnalyzerInterface engines.
type LibraryAnalyzer interface {
    // Analyze returns the cache file data, any non-fatal warnings, and an error.
    // Warnings are transient (e.g., file open failures) and must not be cached.
    Analyze(libPath string) (*LibAnalysisCacheFile, []string, error)
}
```

---

## 4. `internal/dynlibcache/errors.go`（新規）

```go
var (
    // ErrCacheMiss is returned by Get when the cache file does not exist,
    // the lib_hash does not match, or the schema version does not match.
    ErrCacheMiss = errors.New("dynlibcache: cache miss")
)
```

---

## 5. `internal/fileanalysis/schema.go`（変更）

### 5.1 スキーマバージョン

```go
// Version 21 removes LibraryAnalysis from Record and DetectedLibraryNetworkDeps
// from SymbolAnalysisData. Library analysis results are now stored in separate
// per-library cache files (internal/dynlibcache) and read by the runner at runtime.
CurrentSchemaVersion = 21
```

### 5.2 `Record` からの削除

`LibraryAnalysis []LibraryAnalysisEntry` フィールドを削除する。
`DynLibDeps []LibEntry` は変更なし（DynLibDep として記録する）。

### 5.3 `SymbolAnalysisData` からの削除

`DetectedLibraryNetworkDeps []string` フィールドを削除する。

### 5.4 `LibraryAnalysisEntry` 型の削除

`Record.LibraryAnalysis` の削除に伴い、`LibraryAnalysisEntry` 型も削除する。

---

## 6. `internal/fileanalysis/dyn_lib_deps_store.go`（新規）

```go
package fileanalysis

// DynLibDepsStore loads DynLibDeps from a file analysis record.
// Injected into NetworkAnalyzer for library signal aggregation.
type DynLibDepsStore interface {
    // LoadDynLibDeps returns the recorded DynLibDeps for the given file.
    // Returns ErrHashMismatch when contentHash differs from record.ContentHash.
    // Returns (nil, nil) when DynLibDeps is not recorded (e.g. static binary).
    LoadDynLibDeps(filePath string, contentHash string) ([]LibEntry, error)
}

// dynLibDepsStore is the Store-backed implementation of DynLibDepsStore.
type dynLibDepsStore struct {
    store *Store
}

func NewDynLibDepsStore(store *Store) DynLibDepsStore {
    return &dynLibDepsStore{store: store}
}

func (s *dynLibDepsStore) LoadDynLibDeps(filePath string, contentHash string) ([]LibEntry, error) {
    // store.Load → hash 照合 → record.DynLibDeps を返す
}
```

---

## 7. `internal/filevalidator/validator.go`（変更）

### 7.1 フィールド変更

```go
// 削除: libraryAnalysisCache map[string]*fileanalysis.LibraryAnalysisEntry
// 追加:
libAnalysisCacheManager dynlibcache.CacheManagerInterface  // nil = 無効
```

### 7.2 SetLibraryAnalysisCacheManager

```go
func (v *Validator) SetLibraryAnalysisCacheManager(m dynlibcache.CacheManagerInterface)
```

`cmd/record/main.go` から呼び出す。

### 7.3 analyzeLibraries() の変更

**変更前（0123）:** `LibraryAnalysisEntry` を `record.LibraryAnalysis` に追加し、`DetectedLibraryNetworkDeps` を集計

**変更後（0124）:**

```
func (v *Validator) analyzeLibraries(record *fileanalysis.Record):
  1. libAnalysisCacheManager が nil → 早期リターン
  2. record.DynLibDeps をイテレート
  3. VDSO / syscall wrapper → スキップ
  4. メモリキャッシュ Hit → スキップ（GetOrCreate は冪等だが二重解析を避ける）
  5. libAnalysisCacheManager.GetOrCreate(lib.Path, lib.Hash)
     - 成功: メモリキャッシュに格納
     - 失敗: AnalysisWarnings に追記し継続
  6. record.LibraryAnalysis および DetectedLibraryNetworkDeps への書き込みは行わない
```

メモリキャッシュ（セッション内重複解析防止）は `map[string]struct{}` で済む
（「解析済み」フラグのみ必要で、結果は dynlibcache が保持する）。

---

## 8. `internal/runner/base/security/network_analyzer.go`（変更）

### 8.1 フィールド追加

```go
type NetworkAnalyzer struct {
    goos         string
    store        fileanalysis.NetworkSymbolStore
    syscallStore fileanalysis.SyscallAnalysisStore
    depsStore    fileanalysis.DynLibDepsStore        // 新規（nil = 無効）
    libCache     dynlibcache.CacheManagerInterface   // 新規（nil = 無効）
}
```

### 8.2 コンストラクタ追加

```go
func NewNetworkAnalyzerWithLibCache(
    goos string,
    symStore fileanalysis.NetworkSymbolStore,
    svcStore fileanalysis.SyscallAnalysisStore,
    depsStore fileanalysis.DynLibDepsStore,
    libCache dynlibcache.CacheManagerInterface,
) *NetworkAnalyzer
```

### 8.3 isNetworkViaBinaryAnalysis() の拡張

既存の `SymbolAnalysisData` 判定後、以下を追加する:

```
if depsStore != nil && libCache != nil:
    deps, err := depsStore.LoadDynLibDeps(cmdPath, contentHash)
    err 処理:
      ErrRecordNotFound → スキップ（DynLibDeps 未記録）
      ErrHashMismatch   → (true, true) 高リスク
      SchemaVersionMismatchError → (true, true) 高リスク
      その他エラー → (true, true) 高リスク

    for _, dep := range deps:
        result, err := libCache.Get(dep.Path, dep.Hash)
        err != nil → エラー停止（ErrCacheMiss 含む）
        result.HasNetworkSignal     → NetworkDetected
        result.HasDynamicLoadSignal → 高リスクフラグを true に
```

`libCache.Get(path, hash)` は **読み取り専用**（runner は解析を行わない）。
キャッシュファイルが存在しない場合はエラー停止する。

### 8.4 libCache の Get（読み取り専用）

`CacheManagerInterface` に読み取り専用メソッドを追加する:

```go
// Get はキャッシュを読み取る。存在しない・ハッシュ不一致の場合は ErrCacheMiss を返す。
// runner は解析を行わないため、GetOrCreate ではなく Get を使用する。
Get(libPath, libHash string) (*LibAnalysisResult, error)
```

---

## 9. スキーマバージョン移行

### 9.1 旧スキーマ（v20）レコードの扱い

`SchemaVersionMismatchError` は `LoadNetworkSymbolAnalysis` / `LoadDynLibDeps` が返す。
`NetworkAnalyzer` は既存の動作（高リスク扱い + 警告ログ）を維持する。
ユーザーは `record` を再実行する必要がある。

### 9.2 移行に伴う削除対象

| 削除対象 | 場所 |
|---------|------|
| `LibraryAnalysisEntry` 型 | `internal/fileanalysis/schema.go` |
| `Record.LibraryAnalysis` フィールド | `internal/fileanalysis/schema.go` |
| `SymbolAnalysisData.DetectedLibraryNetworkDeps` | `internal/fileanalysis/schema.go` |
| `buildAnalysisOutputFromSymbolData` の `DetectedLibraryNetworkDeps` 参照 | `network_analyzer.go` |
| `Validator.libraryAnalysisCache` フィールド（`map[string]*LibraryAnalysisEntry`） | `validator.go` |

---

## 10. テスト方針

各 AC に対するテストの対応:

| AC | テスト種別 | テスト内容 |
|----|---------|---------|
| AC-1 | Unit | 同一ライブラリを 2 回 GetOrCreate → 2 回目はファイルを読まない（モック確認） |
| AC-2 | Unit | `lib_hash` 変更後の GetOrCreate → 再解析されキャッシュが上書きされる |
| AC-3 | Unit | キャッシュヒット時でも `record.DynLibDeps` に正しい soname/path/hash が記録される |
| AC-4 | Integration | dynlibcache 経由のネットワーク判定が 0123 ベースラインと同等 |
| AC-5 | Unit | 破損キャッシュファイル → 再解析して `AnalysisWarnings` に記録 |
| AC-6 | Unit | record JSON サイズの削減を確認（`LibraryAnalysis` フィールドが含まれない） |
| AC-7 | Unit | wrapper / VDSO がキャッシュ対象から除外される |
| AC-8 | CI | `make fmt` / `go test -tags test -v ./...` / `make lint` |
| AC-9 | Integration | `has_dynamic_load_signal = true` のライブラリ → runner が高リスク判定 |
