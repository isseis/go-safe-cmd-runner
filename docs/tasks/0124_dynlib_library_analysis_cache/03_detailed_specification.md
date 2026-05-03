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

1. `pathEnc.Encode(libPath + "#" + libHash)` でキャッシュファイルパスを生成
2. キャッシュファイルを読み込み JSON をパース（失敗 → Cache Miss）
3. `cache.SchemaVersion != CacheSchemaVersion` → Cache Miss
4. `cache.LibHash != libHash` → Cache Miss
5. Cache Hit: `LibAnalysisResult{HasNetworkSignal, HasDynamicLoadSignal}` を構築して返す（`Warnings` は空）
6. Cache Miss: `analyzer.Analyze(libPath)` → `(cacheFile, warnings, err)` を取得し、ファイルを書き込み、`LibAnalysisResult{..., Warnings: warnings}` を返す

**ファイル命名:**

```
<cacheDir>/<pathencoding.SubstitutionHashEscape(libPath + "#" + libHash)>
```

1 ライブラリバージョン（`lib_path` + `lib_hash`）につき 1 ファイル。
同一 `lib_path` でも `lib_hash` が異なるエントリを同時保持できる。

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
    4. メモリキャッシュ Hit → キャッシュ済み warnings を record.AnalysisWarnings に追記してスキップ
  5. libAnalysisCacheManager.GetOrCreate(lib.Path, lib.Hash)
         - 成功（warnings あり）: result.Warnings を record.AnalysisWarnings に追記してメモリキャッシュに格納
     - 失敗（ファイル不在等の error）: error をそのまま返す（FR-3.6.2）
  6. record.LibraryAnalysis および DetectedLibraryNetworkDeps への書き込みは行わない
```

メモリキャッシュ（セッション内重複解析防止）は
`map[string]*dynlibcache.LibAnalysisResult` を使用する。

- キー: `libPath + "#" + libHash`
- 目的: 同一 `record` セッション内で再遭遇したライブラリに対して、`warnings` 伝播を失わない

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
      ErrRecordNotFound → (true, true) 高リスク
      ErrHashMismatch   → (true, true) 高リスク
      SchemaVersionMismatchError → (true, true) 高リスク
      その他エラー → (true, true) 高リスク

    for _, dep := range deps:
    if dep が VDSO / syscall wrapper に該当:
        continue
        result, err := libCache.Get(dep.Path, dep.Hash)
        err != nil → エラー停止（ErrCacheMiss 含む）
        result.HasNetworkSignal     → NetworkDetected
        result.HasDynamicLoadSignal → 高リスクフラグを true に
```

`runner` は実行時ライブ解析へのフォールバックを行わない。
`DynLibDeps` またはライブラリキャッシュが読めない場合は fail-closed（高リスクまたは停止）とする。

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

| AC | テスト種別 | テスト名 / テスト内容 |
|----|---------|---------|
| AC-1 | Unit | `TestCacheManager_GetOrCreate_Hit`: 同一ライブラリを 2 回 GetOrCreate → 2 回目はファイルを読まない（モック確認） |
| AC-2 | Unit | `TestCacheManager_GetOrCreate_HashMismatch`: `lib_hash` 変更後の GetOrCreate → 再解析され新しい `path+hash` キーで保存される |
| AC-3 | Unit | `TestAnalyzeLibraries_dynLibDepsPreservedOnCacheHit`: キャッシュヒット時でも `record.DynLibDeps` に正しい soname/path/hash が記録される |
| AC-4 | Integration | dynlibcache 経由のネットワーク判定が 0123 ベースラインと同等（runner 側で wrapper/VDSO 除外を維持し、`LoadDynLibDeps` の `ErrRecordNotFound` は fail-closed で高リスク扱い） |
| AC-5 | Unit | `TestCacheManager_GetOrCreate_CorruptFile`: 破損キャッシュファイル → 再解析して `AnalysisWarnings` に記録 |
| AC-6 | Unit | `TestAnalyzeLibraries_recordHasNoDynLibAnalysisField`: record JSON サイズの削減を確認（`LibraryAnalysis` フィールドが含まれない） |
| AC-7 | Unit | `TestAnalyzeLibraries_excludesWrapperAndVDSO`: wrapper / VDSO がキャッシュ対象から除外される |
| AC-8 | CI | `make fmt` / `go test -tags test -v ./...` / `make lint` |
| AC-9 | Integration | `has_dynamic_load_signal = true` のライブラリ → runner が高リスク判定 |
| AC-10 | Unit | `TestValidatorLibraryAnalyzer_Analyze_fileTooLarge`: ファイルサイズ 1 GB 超のライブラリを `Analyze()` に渡すと警告が返り `SyscallAnalysis` が nil（FR-3.6.1） |
| AC-11 | Unit | `TestAnalyzeLibraries_fileTooLargeWarningPropagated`: ファイルサイズ超過ライブラリの警告が `record.AnalysisWarnings` に追記され処理が継続される（FR-3.6.1） |
| AC-12 | Unit | `TestAnalyzeLibraries_missingLibFileReturnsError`: 存在しないライブラリパスが含まれる DynLibDeps で `analyzeLibraries()` がエラーを返すことを確認。また `record` セッションレベルのテストで次ファイルの処理継続を確認（FR-3.6.2） |
| AC-13 | Unit | `TestAnalyzeLibraries_vdsoExcluded`: `linux-vdso.so.1` のみを含む DynLibDeps で `GetOrCreate` が呼ばれないことを確認（FR-3.6.3） |

### 10.1 AC-10/11 の実装方法

ファイルサイズ超過は `validatorLibraryAnalyzer.Analyze()` 内で検出するが、
テスト時に 1 GB 超の実ファイルを作成するのはコストが高い。
**モック `FileSystem`** の `SafeOpenFile().Stat()` が 1 GB 超のサイズを返すようスタブすることで、
実ファイルなしにサイズ超過パスをテストする。

```
// テスト用スタブ例
type oversizedStatFile struct { safefileio.File }
func (f *oversizedStatFile) Stat() (os.FileInfo, error) {
    return &fakeFileInfo{size: maxFileSize + 1}, nil
}
```

AC-11 の `TestAnalyzeLibraries_fileTooLargeWarningPropagated` では、
`GetOrCreate` を呼んだ結果の `LibAnalysisResult.Warnings` が
`record.AnalysisWarnings` に追記されることを `analyzeLibraries()` ごと呼び出して確認する。

### 10.2 AC-12 の実装方法

`TestAnalyzeLibraries_missingLibFileReturnsError` は
`analyzeLibraries()` を直接呼び出し、`DynLibDeps` に存在しないパスを含むエントリを渡す。
`Analyze()` が error を返し、`analyzeLibraries()` がその error を上位へ伝播することを確認する。

セッション継続の確認は統合テストレベルで行う（`record` が複数ファイルを処理する際に、
1 ファイルの `analyzeLibraries` エラーが次ファイルの処理を妨げないこと）。

**タスク 0123 AC-7 との関係:**
タスク 0123 の `analyzeOneLibrary` は warning を返していたが、本タスクで `Analyze()` が
error を返す動作に変更する。`TestAnalyzeOneLibrary_missingFileAddsWarning`（旧テスト）は
`TestValidatorLibraryAnalyzer_Analyze_missingFileReturnsError` に置き換える。

### 10.3 AC-13 の実装方法

`TestAnalyzeLibraries_vdsoExcluded` は VDSO エントリのみを持つ `DynLibDeps` を渡し、
`GetOrCreate` が一度も呼ばれないことをモック `CacheManagerInterface` の呼び出し回数で確認する。
（既存の `TestAnalyzeLibraries_excludesWrapperAndVDSO` は wrapper と混在テストのため、
VDSO 除外を単独で保証しない。）
