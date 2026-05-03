# 動的ライブラリ解析結果ストア導入 詳細仕様書

## 1. 変更ファイル一覧（最終形）

| ファイル | 変更種別 | 概要 |
|---------|----------|------|
| internal/dynlibanalysisstore/schema.go | 新規 | DynamicLibAnalysisFile 型、スキーマ定数 |
| internal/dynlibanalysisstore/store.go | 新規 | DynamicLibAnalysisStore（LoadOrAnalyzeAndStore / LoadAnalysis） |
| internal/dynlibanalysisstore/errors.go | 新規 | 解析結果未取得エラー型 |
| internal/fileanalysis/schema.go | 変更 | LibraryAnalysis/DetectedLibraryNetworkDeps 削除、バージョン更新 |
| internal/fileanalysis/dyn_lib_deps_store.go | 新規 | DynLibDepsStore インタフェースと実装 |
| internal/filevalidator/validator.go | 変更 | DynamicLibAnalysisStore 注入、analyzeLibraries で保存 |
| internal/runner/base/security/network_analyzer.go | 変更 | DynLibDepsStore / DynamicLibAnalysisStore 注入、解析結果集計 |
| cmd/record/main.go | 変更 | ストア生成と validator への注入 |
| cmd/runner/main.go | 変更 | ストア生成と network analyzer への注入 |

---

## 2. 用語と責務

### 2.1 用語

- 動的ライブラリ解析結果: runner の判定入力データ
- 解析結果ストア: 上記データを path/hash/schema で保存・取得する機構
- 再解析回避キャッシュ: record 実行中の重複回避戦略

### 2.2 record と runner の責務分離

- record: 解析結果を作る、保存する、再利用する
- runner: 解析結果を読む、導出する、判定する

runner 仕様文では cache hit/miss という語を使わず、解析結果取得成功/未取得で記載する。

---

## 3. データモデル

### 3.1 DynamicLibAnalysisFile

```go
type DynamicLibAnalysisFile struct {
    SchemaVersion      int    `json:"schema_version"`
    LibPath            string `json:"lib_path"`
    LibHash            string `json:"lib_hash"`
    SyscallAnalysis    *fileanalysis.SyscallAnalysisData `json:"syscall_analysis,omitempty"`
    SymbolAnalysis     *fileanalysis.SymbolAnalysisData  `json:"symbol_analysis,omitempty"`
    DynamicLoadSymbols []string                          `json:"dynamic_load_symbols,omitempty"`
}
```

### 3.2 DynamicLibAnalysisResult

```go
type DynamicLibAnalysisResult struct {
    SyscallAnalysis    *fileanalysis.SyscallAnalysisData
    SymbolAnalysis     *fileanalysis.SymbolAnalysisData
    DynamicLoadSymbols []string
    Warnings           []string
}
```

Warnings は record の解析時にのみ利用し、永続ファイルへ保存しない。

---

## 4. ストア API 仕様

### 4.1 DynamicLibAnalysisStore

```go
type DynamicLibAnalysisStore interface {
    // record 専用: 取得できなければ解析して保存する
    LoadOrAnalyzeAndStore(libPath, libHash string) (*DynamicLibAnalysisResult, error)

    // runner 専用: 取得のみ。解析は行わない
    LoadAnalysis(libPath, libHash string) (*DynamicLibAnalysisResult, error)
}
```

### 4.2 エラー型

```go
var ErrAnalysisNotFound = errors.New("dynlibanalysisstore: analysis not found")
```

ErrAnalysisNotFound は以下を含む。

- ファイル不存在
- schema_version 不一致
- lib_hash 不一致

---

## 5. record 側詳細

### 5.1 Validator フィールド

```go
type Validator struct {
    // 旧: libraryAnalysisCache
    // 新: processedLibAnalysis map[string]*DynamicLibAnalysisResult
    dynamicLibAnalysisStore DynamicLibAnalysisStore // nil = 無効
}
```

### 5.2 analyzeLibraries フロー

```text
1. dynamicLibAnalysisStore が nil なら return
2. record.DynLibDeps を走査
3. wrapper / VDSO を除外
4. processedLibAnalysis にあれば warnings を再伝播して continue
5. dynamicLibAnalysisStore.LoadOrAnalyzeAndStore(lib.Path, lib.Hash)
6. 成功時: warnings を record.AnalysisWarnings に追記
7. 失敗時: error を上位へ返却（ファイル不在、サイズ超過含む）
```

### 5.3 エラー処理

- ファイル不在と 1 GB 超過はともに error を返し、当該レコードを不出力とする
- セッション継続は record のファイル単位エラー制御で担保する

---

## 6. runner 側詳細

### 6.1 NetworkAnalyzer フィールド

```go
type NetworkAnalyzer struct {
    goos             string
    store            fileanalysis.NetworkSymbolStore
    syscallStore     fileanalysis.SyscallAnalysisStore
    depsStore        fileanalysis.DynLibDepsStore
    libAnalysisStore DynamicLibAnalysisStore
}
```

### 6.2 コンストラクタ

```go
func NewNetworkAnalyzerWithLibAnalysisStore(
    goos string,
    symStore fileanalysis.NetworkSymbolStore,
    svcStore fileanalysis.SyscallAnalysisStore,
    depsStore fileanalysis.DynLibDepsStore,
    libAnalysisStore DynamicLibAnalysisStore,
) *NetworkAnalyzer
```

### 6.3 isNetworkViaBinaryAnalysis 拡張

```text
1. 既存のバイナリ本体判定を実行
2. depsStore.LoadDynLibDeps(cmdPath, contentHash) を実行
3. 各 dep で wrapper / VDSO を除外
4. libAnalysisStore.LoadAnalysis(dep.Path, dep.Hash) を実行
5. 読込結果から has_network_signal / has_dynamic_load_signal を導出
6. 解析結果未取得や store 読込失敗は fail-closed で停止
```

---

## 7. スキーマ移行

### 7.1 fileanalysis.Record

- Record.LibraryAnalysis を削除
- SymbolAnalysisData.DetectedLibraryNetworkDeps を削除
- CurrentSchemaVersion を更新

### 7.2 旧レコード

- LoadNetworkSymbolAnalysis / LoadDynLibDeps が SchemaVersionMismatchError を返す
- runner は fail-closed を維持する
- ユーザーは record 再実行が必要

---

## 8. シンボルリネーム仕様（最終形）

| 種別 | 旧シンボル | 新シンボル |
|---|---|---|
| package | internal/dynlibcache | internal/dynlibanalysisstore |
| type | CacheManager | DynamicLibAnalysisStoreImpl |
| interface | CacheManagerInterface | DynamicLibAnalysisStore |
| method | GetOrCreate | LoadOrAnalyzeAndStore |
| method | Get | LoadAnalysis |
| error | ErrCacheMiss | ErrAnalysisNotFound |
| validator field | libAnalysisCacheManager | dynamicLibAnalysisStore |
| runner field | libCache | libAnalysisStore |
| constructor | NewNetworkAnalyzerWithLibCache | NewNetworkAnalyzerWithLibAnalysisStore |

注記: 最終マージ時点では旧シンボルを残さず、新シンボルへ統一する。

---

## 9. テスト方針

| AC | テスト種別 | テスト内容（新命名） |
|----|---------|---------|
| AC-1 | Unit | TestDynamicLibAnalysisStore_LoadOrAnalyzeAndStore_Reuse |
| AC-2 | Unit | TestDynamicLibAnalysisStore_LoadOrAnalyzeAndStore_HashChanged |
| AC-3 | Unit | TestAnalyzeLibraries_DynLibDepsPreservedOnReuse |
| AC-4 | Integration | TestNetworkAnalyzer_UsesDynamicLibAnalysisResults |
| AC-5 | Unit | TestDynamicLibAnalysisStore_CorruptFile_Reanalyze |
| AC-6 | Unit | TestAnalyzeLibraries_RecordHasNoLibraryAnalysisField |
| AC-7 | Unit | TestAnalyzeLibraries_ExcludesWrapperAndVDSO |
| AC-8 | CI | make fmt / go test -tags test -v ./... / make lint |
| AC-9 | Integration | TestNetworkAnalyzer_DynamicLoadSymbolsHighRisk |
| AC-10 | Unit | TestValidatorLibraryAnalyzer_Analyze_FileTooLargeReturnsError |
| AC-12 | Unit | TestAnalyzeLibraries_MissingLibFileReturnsError |
| AC-13 | Unit | TestAnalyzeLibraries_VDSOExcluded |

### 9.1 AC-10 の実装補足

SafeOpenFile().Stat() をモックし、maxFileSize + 1 を返すことで
1 GB 超過パスを実ファイルなしで検証する。

### 9.2 AC-12 の実装補足

analyzeLibraries を直接呼び、存在しないパスを含む DynLibDeps を渡し、
error 伝播とセッション継続を確認する。
