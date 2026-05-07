# 詳細設計書: コマンド Record 完全自己完結化

## 1. スキーマ変更 (Phase 1)

### 1.1 `internal/fileanalysis/schema.go`

`CurrentSchemaVersion` を 21 → **22** に更新する。

#### 1.1.1 新規型定義

```go
// DepEntry represents one entry in the unified analysis list.
// Covers both shared libraries (SOName non-empty) and interpreter executables
// from the shebang chain (SOName empty). Path is the dedup primary key.
type DepEntry struct {
    SOName          string               `json:"soname,omitempty"`
    Path            string               `json:"path"`
    Hash            string               `json:"hash"`
    SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
    SymbolAnalysis  *SymbolAnalysisData  `json:"symbol_analysis,omitempty"`
    Warnings        []string             `json:"warnings,omitempty"`
}

// ShebangBinaryInfo holds ordered naming metadata for one binary in the shebang chain.
// Hash and analysis results are stored in the corresponding Deps entry (keyed by Path).
type ShebangBinaryInfo struct {
    RawPath     string `json:"raw_path,omitempty"`
    Path        string `json:"path"`
    CommandName string `json:"command_name,omitempty"`
    // No ContentHash or analysis fields: all binary data lives in Deps.
}

// DebugInfo holds optional debugging information recorded with -debug-info.
type DebugInfo struct {
    // DepSources maps each dep's absolute path to the list of binary absolute
    // paths that declare it as a dependency.
    DepSources map[string][]string `json:"dep_sources,omitempty"`
}
```

#### 1.1.2 `Record` 構造体の変更

追加フィールド:
```go
Deps         []DepEntry          `json:"deps,omitempty"`
ShebangChain []ShebangBinaryInfo `json:"shebang_chain,omitempty"`
Debug        *DebugInfo          `json:"debug,omitempty"`
```

削除フィールド（スキーマバージョン 22 時点で不要）:
```go
// 削除: DynLibDeps []LibEntry
// 削除: ShebangInterpreter *ShebangInterpreterInfo
// 削除: AnalysisWarnings []string
```

`LibEntry` および `ShebangInterpreterInfo` 型も削除する。

### 1.2 後方互換性

`fileanalysis.Store.Load` は `schema_version != 22` の場合に `SchemaVersionMismatchError` を返す（既存動作）。`Store.Update` は旧スキーマ（22 未満）を新規 Record として扱い、`record` 再実行で上書きする（既存動作）。

---

## 2. `record` コマンド変更 (Phase 2)

### 2.1 変更対象ファイル

- `internal/filevalidator/validator.go` — 主実装

### 2.2 `SaveRecord` 全体フロー（変更後）

```
SaveRecord(filePath, force)
  → analyzeTarget(filePath)            # hash + syscall + symbol + dynlib deps
  → resolveShebangChain(filePath)      # shebang チェーン各バイナリの同様解析
  → collectAndDedupDeps(...)           # 全 dyn_lib_deps を path 主キーで dedup
  → analyzeDepEntries(...)             # 各 dep の解析（キャッシュ優先）
  → buildRecord(...)                   # Record 構築
  → store.Save(...)                    # アトミック書き出し
```

**変更点:**
- `saveInterpreterRecord` を廃止する（インタープリターの Record ファイルを別途生成しない）
- インタープリターの情報は `ShebangChain` に埋め込む
- `AnalysisWarnings` は `DepEntry.Warnings` に移動する

### 2.3 `resolveShebangChain` の仕様

`shebang.Parse` で解析後、各バイナリ（`InterpreterPath`、`ResolvedPath`）について:

1. `calculateHash` でコンテンツハッシュを算出（`ShebangBinaryInfo.ContentHash` に設定）
2. ELF dynlib analyzer で `DynLibDeps`（共有ライブラリ）を収集（`elfDynlibAnalyzer.Analyze(filePath)`）
3. 一時 `Record` を作成し `DynLibDeps` を設定した上で `analyzeELFSyscalls(&tempRecord, filePath)` を呼ぶ → `tempRecord.SyscallAnalysis` を取得
4. `binaryAnalyzer.AnalyzeNetworkSymbols` で `SymbolAnalysis` を取得

注: `analyzeELFSyscalls` は libc エントリを探すため（`findLibcEntry(record.DynLibDeps)`）、手順 2 で `DynLibDeps` を設定してから呼ぶ必要がある。

返却型: `([]ShebangBinaryInfo, []binaryDynLibDeps)`
— `ShebangBinaryInfo` は命名メタデータのみ（`RawPath`、`Path`、`CommandName`）。`ContentHash` は不要（hash は `deps` に保持される）
— `binaryDynLibDeps` はインタープリターバイナリ自体の hash + 解析結果 + 共有ライブラリ一覧を dedup 処理に渡すために返す

**`ShebangBinaryInfo` フィールドマッピング:**
| `shebang.Info` フィールド | `ShebangBinaryInfo` フィールド |
|---|---|
| `RawInterpreterPath` | 先頭エントリの `RawPath` |
| `InterpreterPath`（env バイナリのパス） | 先頭エントリの `Path` |
| `CommandName` | 先頭エントリの `CommandName` |
| `ResolvedPath`（解決済みコマンドのパス） | 2番目エントリの `Path` |

**インタープリターバイナリの `DepEntry` 生成:**
各インタープリターバイナリは `DepEntry{SOName: "", Path: ..., Hash: ..., SyscallAnalysis: ..., SymbolAnalysis: ...}` として `collectAndDedupDeps` に渡す。VDSO/wrapper 判定は `SOName` が空の場合スキップ（実行バイナリは wrapper でない）。

### 2.4 `collectAndDedupDeps` の仕様

入力型（内部ヘルパー構造体）:
```go
// binaryDynLibDeps pairs a source binary path with its dynamic library dependencies.
type binaryDynLibDeps struct {
    sourcePath string
    deps       []fileanalysis.LibEntry
}
```

入力: コマンド本体と shebang チェーン各バイナリの `binaryDynLibDeps` のスライス

アルゴリズム（コード中のコメントはすべて英語で記述）:

```
seen := map[string]fileanalysis.LibEntry{}  // key: path
sources := map[string][]string{}            // key: dep path → source binary paths (for DebugInfo)

for each bdd in input:
    for each dep in bdd.deps:
        if prev, ok := seen[dep.Path]; ok:
            if prev.Hash != dep.Hash:
                return nil, nil, fmt.Errorf("dep hash mismatch for path %q: ...")
            // same path + same hash: only record source
        else:
            seen[dep.Path] = dep
        sources[dep.Path] = append(sources[dep.Path], bdd.sourcePath)

return dedupedLibEntries, sources, nil
```

**エラー:** 同一 `path` で `hash` が異なる場合は即時エラー返却。Record は書き出さない。

### 2.5 `analyzeDepEntries` の仕様

各 `path` ユニークな dep について:

```
if dep.SOName == "":
    // interpreter executable: analysis already computed in resolveShebangChain
    entry = DepEntry{SOName: "", Path, Hash, precomputedSyscallAnalysis, precomputedSymbolAnalysis, warnings}
else if isKnownVDSO(dep.SOName) || binaryanalyzer.IsSyscallWrapperLibrary(dep.SOName):
    entry = DepEntry{SOName, Path, Hash, nil, nil, nil}
else:
    result = v.processedLibAnalysis[{dep.Path, dep.Hash}]  // in-session cache
    if not cached:
        result = v.dynamicLibAnalysisStore.LoadOrAnalyzeAndStore(dep.Path, dep.Hash)
        cache result
    entry = DepEntry{SOName, Path, Hash, result.SyscallAnalysis, result.SymbolAnalysis, result.Warnings}

append entry to deps
```

インタープリターバイナリのエントリは `resolveShebangChain` で解析済みのため、dynlib キャッシュへのアクセスは不要。

**既存関数の再利用:**
- `isKnownVDSO(soname string) bool` (validator.go:690) — そのまま使用
- `binaryanalyzer.IsSyscallWrapperLibrary(soname string) bool` — そのまま使用
- `v.analyzeOneLibrary(lib LibEntry)` — 既存ロジックを `LoadOrAnalyzeAndStore` の内部で使用

`DepEntry.Warnings` はソート後 dedup する（`slices.Sort` + `slices.Compact`）。

### 2.6 `DebugInfo` 生成

`v.includeDebugInfo == true` の場合のみ `Record.Debug` を設定する:
```go
record.Debug = &DebugInfo{DepSources: sources}
```
`v.includeDebugInfo == false` の場合は `record.Debug = nil`（`omitempty` により JSON に含まれない）。

### 2.7 `Record.SymbolAnalysis` の `DynamicLoadSymbols` 処理

現行の `analyzeLibraries` は dep の `DynamicLoadSymbols` をコマンドの `record.SymbolAnalysis.DynamicLoadSymbols` にマージしている。新設計では runner が各 `DepEntry.SymbolAnalysis.DynamicLoadSymbols` を参照するため、**このマージを `record` では行わない**。ただし `record.SymbolAnalysis` にはコマンド本体の分析結果（shebang チェーン由来を除く）はそのまま記録する。

---

## 3. `runner` 変更 (Phase 3)

### 3.1 `internal/runner/base/security/network_analyzer.go`

#### 3.1.1 `AnalysisDeps` 構造体の変更

```go
// 変更前
type AnalysisDeps struct {
    NetworkSymbolStore fileanalysis.NetworkSymbolStore
    SyscallStore       fileanalysis.SyscallAnalysisStore
    DynLibDepsStore    fileanalysis.DynLibDepsStore
    LibAnalysisStore   dynamicanalysis.Store
    ShebangStore       fileanalysis.ShebangInterpreterStore
}

// 変更後
type AnalysisDeps struct {
    RecordStore RecordLoader
}
```

`RecordLoader` インターフェース（同ファイルに定義）:
```go
// RecordLoader loads an analysis record for the given file path.
type RecordLoader interface {
    LoadRecord(filePath string) (*fileanalysis.Record, error)
}
```

`filevalidator.Validator` は既に `LoadRecord(filePath string) (*fileanalysis.Record, error)` を実装しているため、アダプター不要で直接渡せる。

`RecordStore == nil` の場合、binary analysis は無効（既存の nil-store 動作を維持）。

**除去するフィールドと理由:**
- `NetworkSymbolStore`, `SyscallStore`: Record の `SyscallAnalysis`、`SymbolAnalysis` フィールドを直接参照するため不要
- `DynLibDepsStore`, `LibAnalysisStore`: Record の `Deps` フィールドを直接参照するため不要
- `ShebangStore`: Record の `ShebangChain` フィールドを直接参照するため不要

ContentHash の検証は `record.ContentHash != contentHash` の比較で行う（`NetworkSymbolStore.LoadNetworkSymbolAnalysis` の hash 検証を置き換える）。

#### 3.1.2 `analyzeBinarySignals` の変更

`record.Deps` には共有ライブラリとインタープリターバイナリが統合されているため、`followShebangChain`（解析目的）は廃止し、`checkDepsSignals` のみで全シグナルを収集する。

```
analyzeBinarySignals(cmdPath, contentHash):
  1. if RecordStore == nil: return false, false, nil  // binary analysis disabled
  2. RecordStore.LoadRecord(cmdPath) → record, err
     if err (not found / schema mismatch / corrupted): treat as high risk
  3. if record.ContentHash != contentHash → treat as high risk (hash mismatch)
  4. checkSyscallSignals(record.SyscallAnalysis) → isNetwork, isHighRisk
  5. checkSymbolSignals(record.SymbolAnalysis) → isNetwork, isHighRisk (OR-merge)
  6. checkDepsSignals(record.Deps) → isNetwork, isHighRisk, err
     if err: return false, false, err  // fail-closed
  7. return isNetwork, isHighRisk, nil
```

#### 3.1.3 `checkDepsSignals` の仕様（新関数）

共有ライブラリとインタープリターバイナリを同一ループで処理する。

```
checkDepsSignals(deps []DepEntry) (isNetwork, isHighRisk bool, err error):
  for each dep in deps:
    if dep.SOName != "" && (isVDSOEntry(dep.SOName) || IsSyscallWrapperLibrary(dep.SOName)):
        continue  // known wrappers and VDSO: skip
    if dep.SyscallAnalysis == nil && dep.SymbolAnalysis == nil:
        // analysis not embedded: fail-closed (F-003 AC-6)
        return false, false, fmt.Errorf("%w: %s", ErrDepAnalysisNotEmbedded, dep.Path)
    sigs = extractDepSignals(dep.SyscallAnalysis, dep.SymbolAnalysis)
    isNetwork ||= sigs.networkSignal
    isHighRisk ||= sigs.highRiskSignal
  return isNetwork, isHighRisk, nil
```

VDSO/wrapper のスキップ条件を `dep.SOName != ""` で絞る（インタープリターバイナリは `SOName == ""`）。

**既存関数の再利用:**
- `isVDSOEntry(soname)` — そのまま使用
- `binaryanalyzer.IsSyscallWrapperLibrary(soname)` — そのまま使用
- `analyzeDepSignals(result *dynamicanalysis.Result)` → `extractDepSignals(*SyscallAnalysisData, *SymbolAnalysisData)` に変更して再利用（`*dynamicanalysis.Result` への依存を除去）

#### 3.1.4 `followShebangChain`（解析目的）の廃止

`record.Deps` にインタープリターバイナリの解析結果が含まれるため、`followShebangChain` による解析目的の処理は不要となる。`ShebangStore.LoadInterpreterAnalysisPath` への依存も除去する。

`ShebangChain` はランタイム検証（シンボリックリンクリダイレクト検出 + PATH 再解決）のために後述の `verifyShebangChain` が参照するが、`NetworkAnalyzer` からは参照しない。

#### 3.1.5 `verifyShebangChain` の新規実装

`NetworkAnalyzer` とは独立した検証ロジック。コマンド実行直前に呼ばれ、`shebang_chain` エントリの整合性を確認する。

```
verifyShebangChain(chain []ShebangBinaryInfo) error:
  for each entry in chain:
    // Symlink redirect detection (existing behavior, now reads from ShebangChain)
    if entry.RawPath != "":
        actual = filepath.EvalSymlinks(entry.RawPath)
        if actual != entry.Path:
            return fmt.Errorf("shebang symlink redirected: %q → %q, expected %q",
                entry.RawPath, actual, entry.Path)

    // PATH re-resolution for env-form shebangs (new)
    if entry.CommandName != "":
        lookupPath, err = exec.LookPath(entry.CommandName)
        if err:
            return fmt.Errorf("command %q not found in PATH: %w", entry.CommandName, err)
        resolved = filepath.EvalSymlinks(lookupPath)
        if resolved != entry.Path:
            return fmt.Errorf("PATH resolution mismatch for %q: got %q, recorded %q",
                entry.CommandName, resolved, entry.Path)

  return nil
```

**適用箇所:** `verification.Manager` の検証フロー内、コマンドグループ実行前。`filevalidator.Verify` のスクリプトハッシュ検証とは独立して呼ばれる。

**エラー時の動作:** いずれかのチェックが失敗した場合、コマンドグループ全体を abort（fail-closed）。ロギングは `slog.Error` で記録する。

**既存関数の再利用:**
- `filepath.EvalSymlinks` (標準ライブラリ) — そのまま使用
- `exec.LookPath` (標準ライブラリ) — そのまま使用

### 3.2 `internal/verification/manager.go` の変更

`GetAnalysisDeps` から `DynLibDepsStore`、`LibAnalysisStore`、`ShebangStore` を除去し、`RecordStore` を追加する:

```go
func (m *Manager) GetAnalysisDeps() security.AnalysisDeps {
    return security.AnalysisDeps{
        RecordStore: m.fileValidator,  // filevalidator.Validator implements RecordLoader
    }
}
```

`Manager` から `networkSymbolStore`、`syscallAnalysisStore`、`dynlibAnalysisStore`、`dynLibDepsStore`、`shebangStore` フィールドをすべて削除する。関連する初期化コードも除去する。

`verifyShebangChain` は `Manager` のコマンドグループ実行前フローに組み込む（Record のロードは `fileValidator.LoadRecord` 経由）。

---

## 4. エラー設計

### 4.1 新規エラー: `ErrDepAnalysisNotEmbedded`

```go
// ErrDepAnalysisNotEmbedded is returned when a dep entry in Record.Deps has nil
// SyscallAnalysis and SymbolAnalysis but is not a known syscall wrapper or VDSO.
// The runner must abort rather than fall back to a permissive default.
var ErrDepAnalysisNotEmbedded = errors.New("dep analysis not embedded in record")
```

`checkDepsSignals` はこのエラーをラップして返す。

### 4.2 PATH 再解決不一致（verifyShebangChain）

`entry.CommandName` を `exec.LookPath` + `filepath.EvalSymlinks` で解決した結果が `entry.Path` と異なる場合:
- `slog.Error` でログ記録
- コマンドグループ全体を abort（fail-closed）
- 記述的なエラーを返す: `"PATH resolution mismatch for %q: got %q, recorded %q"`

`exec.LookPath` が失敗（コマンドが PATH に存在しない）場合も同様に abort する。

### 4.3 hash 不一致（dedup 時）

既存の error sentinel は使用せず、記述的なエラーを `fmt.Errorf` で返す:
```
"dep hash mismatch for path %q: recorded %q, found %q"
```

---

## 5. テスト設計

### 5.1 ユニットテスト（新規・変更）

#### `internal/fileanalysis/schema_test.go` （新規）

| テスト | 対応 AC |
|---|---|
| `DepEntry` JSON round-trip (soname あり/なし、syscall_analysis null / 非 null) | F-001 AC1 |
| `DepEntry` JSON round-trip (warnings あり / なし) | F-001 AC4 |
| `ShebangBinaryInfo` JSON round-trip (direct form、`content_hash` フィールドなし) | F-002 AC1, AC2 |
| `ShebangBinaryInfo` JSON round-trip (env form) | F-002 AC1, AC3 |
| `DebugInfo` omitempty (debug=nil → JSON に "debug" キーなし) | F-004 AC1 |
| `CurrentSchemaVersion == 22` | F-005 AC1 |

#### `internal/filevalidator/validator_dedup_test.go` （新規）

| テスト | 対応 AC |
|---|---|
| 同一 path + 同一 hash → 1 エントリに統合 | F-001 AC2 |
| 同一 path + 異なる hash → エラー返却、Record 書き出しなし | F-001 AC2, 4.3 |
| syscall wrapper dep → `SyscallAnalysis == nil && SymbolAnalysis == nil` | F-001 AC3 |
| VDSO dep → `SyscallAnalysis == nil && SymbolAnalysis == nil` | F-001 AC3 |
| dep warnings → `DepEntry.Warnings` に記録 | F-001 AC4 |

#### `internal/verification/shebang_chain_verifier_test.go` （新規）

| テスト | 対応 AC |
|---|---|
| `command_name` あり + PATH 解決が `path` と一致 → エラーなし | F-002 AC6 |
| `command_name` あり + PATH 解決が `path` と不一致 → エラー返却 | F-002 AC6 |
| `command_name` あり + PATH に存在しない → エラー返却 | F-002 AC6 |
| `command_name` なし（direct form）→ PATH 再解決をスキップ | F-002 AC6 |
| `raw_path` のシンボリックリンク先が `path` と一致 → エラーなし | 既存動作の維持 |
| `raw_path` のシンボリックリンク先が `path` と不一致 → エラー返却 | 既存動作の維持 |

#### `internal/runner/base/security/network_analyzer_test.go` （変更）

| テスト | 対応 AC |
|---|---|
| `Record.Deps` から共有ライブラリの network signal を検出する | F-003 AC2 |
| `Record.Deps` からインタープリターバイナリの network signal を検出する（`soname` なしエントリ） | F-003 AC5 |
| 非 wrapper dep の `SyscallAnalysis == nil` → `ErrDepAnalysisNotEmbedded` | F-003 AC6 |
| VDSO dep の `SyscallAnalysis == nil` → エラーなし | F-003 AC6 |
| `DynLibDepsStore`、`LibAnalysisStore`、`ShebangStore` への依存がない（コンパイル確認） | F-003 AC1 |

### 5.2 統合テスト（変更）

| テスト | 対応 AC |
|---|---|
| ELF バイナリの `record` が `deps` を埋め込んだ Record を生成 | F-001 AC1〜3 |
| 直接形式 shebang の `shebang_chain`（1エントリ）生成 | F-002 AC2 |
| env 形式 shebang の `shebang_chain`（2エントリ）生成 | F-002 AC3 |
| `dynlib-analysis/` ディレクトリなしで `runner` が正常動作 | F-003 AC3 |
| `-debug-info` あり → `debug.dep_sources` 記録 | F-004 AC2 |
| `-debug-info` なし → `debug` フィールドなし | F-004 AC1 |
| v21 Record 読み込み → `SchemaVersionMismatchError` | F-005 AC2 |
| `record` 再実行で旧 Record を上書き | F-005 AC3 |

### 5.3 既存テストの変更

- `validator_shebang_test.go`: インタープリターの別 Record ファイル生成を前提とするテストを削除し、インタープリターバイナリが `Record.Deps` に含まれること（`soname` なし）を検証する形に書き換え
- `validator_library_analysis_test.go`: `AnalysisWarnings` の期待値を `DepEntry.Warnings` に変更
- `network_analyzer_test.go`: `AnalysisDeps` から全ストアを使うテストを削除し、`RecordStore` を使う形に書き換え

### 5.4 削除するテスト（不要となるもの）

- `ErrAnalysisNotFound` → 高リスクフォールバックのテスト（`checkDynLibDepsNetwork` の `ErrAnalysisNotFound` パス）
- `ShebangStore.LoadInterpreterAnalysisPath` を直接使うテスト
- `DynLibDepsStore` を使うテスト（`LoadDynLibDeps` 経由のパス）
- `ShebangBinaryInfo.SyscallAnalysis`・`SymbolAnalysis` フィールドを参照するテスト（フィールド削除に伴い）

---

## 6. 実装上の注意事項

### 6.1 コードの日本語禁止

コメント、変数名、定数名、エラーメッセージすべて英語で記述する。

### 6.2 既存関数の再利用

| 用途 | 使用する既存関数 |
|---|---|
| VDSO 判定 | `isKnownVDSO(soname)` in `validator.go` と `isVDSOEntry(soname)` in `network_analyzer.go` |
| syscall wrapper 判定 | `binaryanalyzer.IsSyscallWrapperLibrary(soname)` |
| ライブラリ解析 | `v.analyzeOneLibrary(lib LibEntry)` |
| ELF syscall 解析 | `v.analyzeELFSyscalls(record, filePath)` |
| symbol 解析 | `v.binaryAnalyzer.AnalyzeNetworkSymbols(...)` |
| hash 計算 | `v.calculateHash(targetPath)` |

注: `isKnownVDSO`（`validator.go`）と `isVDSOEntry`（`network_analyzer.go`）は同等ロジックの重複。今回の実装ではそのまま維持し、将来の DRY化タスクに委ねる。

### 6.3 `LibEntry` 型の扱い

`LibEntry` 型は `record` コマンドの内部処理（dynlib 解析結果の受け渡し）には引き続き使用できる（dynlib-analysis キャッシュは `record` 内部最適化として存続）。ただし `Record` の JSON フィールドには露出しない。`LibEntry` 型の削除は `DepEntry` への移行が完了した後に行う。

### 6.4 `fileanalysis.DynLibDepsStore`, `ShebangInterpreterStore` インターフェース

`runner` がこれらを参照しなくなるため、インターフェース定義は残しつつ `AnalysisDeps` から除去する。型自体の削除は別タスクとし、今回のスコープ外とする。
