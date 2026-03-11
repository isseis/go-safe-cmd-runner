# ELF 動的リンクライブラリ整合性検証 実装計画書

## 1. 実装概要

### 1.1 実装目標

- ELF バイナリの `DT_NEEDED` から依存ライブラリの完全な依存ツリーを解決し、`record` 時にスナップショットを記録する
- `runner` 実行時にハッシュ照合でライブラリの改ざんを検出する（LD_LIBRARY_PATH は runner 実行前にクリアされるため第 2 段階は不要）
- DT_RPATH を持つ ELF バイナリを検出した場合は `ErrDTRPATHNotSupported` を返して処理中断する
- `dlopen` / `dlsym` / `dlvsym` を検出し、実行時ロードを使用するバイナリを高リスクと判定する
- `CurrentSchemaVersion` を 1 → 2 に上げ、旧記録を一律拒否する

### 1.2 実装スコープ

| 区分 | 内容 |
|------|------|
| 新規パッケージ | `internal/dynlibanalysis/`（7 ファイル + テスト + testdata） |
| 拡張対象 | `fileanalysis`, `filevalidator`, `verification`, `binaryanalyzer`, `elfanalyzer`, `machoanalyzer`, `network_analyzer`, `cmd/record` |
| 新規ファイル数 | 約 15 ファイル（実装 + テスト + testdata） |
| テストケース | 単体 36 件、コンポーネント 1 件、統合 8 件 |

### 1.3 想定工数

| Phase | 工数 | 内容 |
|-------|------|------|
| Phase 1 | 0.5 日 | スキーマ拡張・型定義 |
| Phase 2 | 3 日 | ライブラリ解決エンジン（ld.so.cache パーサー含む） |
| Phase 3 | 2 日 | DynLibAnalyzer（record 拡張） |
| Phase 4 | 2 日 | DynLibVerifier（runner 拡張） |
| Phase 5 | 1.5 日 | dlopen シンボル検出 + 仕上げ |
| 合計 | 9 日 | |

## 2. 実装フェーズ計画

### 2.1 Phase 1: スキーマ拡張・型定義 (0.5 日)

**目標**: `fileanalysis.Record` を拡張し、`DynLibDeps` と `HasDynamicLoad` を格納できるようにする。スキーマバージョンを 2 に上げる。

#### 実装対象

```
internal/fileanalysis/schema.go
```

#### 実装内容

##### 2.1.1 `CurrentSchemaVersion` の変更

**ファイル**: `internal/fileanalysis/schema.go`

```go
const (
    CurrentSchemaVersion = 2  // 1 → 2 に変更
)
```

##### 2.1.2 `DynLibDepsData` / `LibEntry` 型定義

**ファイル**: `internal/fileanalysis/schema.go`

```go
type DynLibDepsData struct {
    RecordedAt time.Time  `json:"recorded_at"`
    Libs       []LibEntry `json:"libs"`
}

type LibEntry struct {
    SOName string `json:"soname"`
    Path   string `json:"path"`
    Hash   string `json:"hash"`
}
```

`parent_path` および `inherited_rpath` フィールドは削除済み（スコープ外: 第 2 段階検証廃止・DT_RPATH 非サポート化による）。

##### 2.1.3 `Record` 構造体の拡張

**ファイル**: `internal/fileanalysis/schema.go`

- `DynLibDeps *DynLibDepsData` フィールド追加
- `HasDynamicLoad bool` フィールド追加

##### 2.1.4 `Store.Update` — 旧スキーマレコードの上書き許可

**ファイル**: `internal/fileanalysis/file_analysis_store.go`

`CurrentSchemaVersion` を 1 → 2 に変更すると、既存の v1 レコードを持つ全ファイルで `record --force` が失敗する問題がある。`Store.Update` が `SchemaVersionMismatchError` を一律に拒否するためである。

移行手順（「全管理対象バイナリに `record --force` を再実行」）を実行可能にするため、`Update` の `SchemaVersionMismatchError` 処理を以下のように変更する:

- `Actual < Expected`（旧スキーマ、例: v1 レコードを v2 バイナリで上書き）→ `ErrRecordNotFound` と同等に扱い、`Record{}` で上書きを許可
- `Actual > Expected`（新スキーマ、例: v3 レコードを v2 バイナリが読む）→ 現行通りエラーを返す（前方互換性保護）

```go
// internal/fileanalysis/file_analysis_store.go

var schemaErr *SchemaVersionMismatchError
if errors.As(err, &schemaErr) {
    if schemaErr.Actual > schemaErr.Expected {
        // Future schema: do NOT overwrite (forward compatibility protection)
        return fmt.Errorf("cannot update record: %w", err)
    }
    // Older schema (e.g. v1 record, current binary is v2):
    // treat as if not found and allow --force to overwrite.
    record = &Record{}
} else if ...
```

##### 2.1.5 既存テストの更新

`CurrentSchemaVersion` に依存するテストケース（`Store.Load` の `SchemaVersionMismatchError` テスト等）を新しいバージョン値に更新する。

#### 完了条件

- [x] `CurrentSchemaVersion` が 2 に変更されていること
- [x] `DynLibDepsData`, `LibEntry` 型が定義されていること（`soname`, `path`, `hash` のみ）
- [x] `Record` に `DynLibDeps`, `HasDynamicLoad` フィールドが追加されていること
- [x] `Store.Update` が旧スキーマ（`Actual < Expected`）の `SchemaVersionMismatchError` 時に上書きを許可すること
- [x] `Store.Update` が新スキーマ（`Actual > Expected`）の `SchemaVersionMismatchError` 時にエラーを返すこと
- [x] 既存テストが全てパスすること
- [x] `make lint` / `make fmt` がパスすること

---

### 2.2 Phase 2: ライブラリ解決エンジン (3 日)

**目標**: `DT_NEEDED` のライブラリ名からファイルシステム上のフルパスを解決するエンジンを実装する。RPATH/RUNPATH 継承や `/etc/ld.so.cache` の解析を含む。

#### 実装対象

```
internal/dynlibanalysis/           # NEW パッケージ
├── doc.go
├── errors.go
├── default_paths.go
├── ldcache.go
├── resolver_context.go
├── resolver.go
├── default_paths_test.go
├── ldcache_test.go
├── resolver_context_test.go
├── resolver_test.go
└── testdata/
    ├── README.md
    └── ldcache_new_format.bin
```

#### 実装内容

##### 2.2.1 パッケージ作成とエラー型

**ファイル**: `internal/dynlibanalysis/doc.go`

```go
// Package dynlibanalysis provides dynamic library dependency analysis
// and verification for ELF binaries.
package dynlibanalysis
```

**ファイル**: `internal/dynlibanalysis/errors.go`

- `ErrLibraryNotResolved`: ライブラリ解決失敗（soname, parentPath, searchPaths を含む）
- `ErrRecursionDepthExceeded`: 再帰深度超過
- `ErrLibraryHashMismatch`: ハッシュ不一致（検証失敗）
- `ErrEmptyLibraryPath`: 空パス（防御的チェック）
- `ErrDynLibDepsRequired`: ELF バイナリに DynLibDeps がない
- `ErrDTRPATHNotSupported`: DT_RPATH を持つ ELF が検出された（DT_RPATH は非サポート）
- ~~`ErrLibraryPathMismatch`~~: 削除済み（第 2 段階検証廃止のため）

##### 2.2.2 デフォルト検索パス

**ファイル**: `internal/dynlibanalysis/default_paths.go`

- `DefaultSearchPaths(machine elf.Machine) []string`
- x86_64: multiarch → lib64 → generic
- aarch64: multiarch → lib64 → generic
- その他: lib64 → generic

##### 2.2.3 `/etc/ld.so.cache` パーサー

**ファイル**: `internal/dynlibanalysis/ldcache.go`

- `LDCache` 構造体（`entries map[string]string`）
- `ParseLDCache(path string) (*LDCache, error)`: 新形式（`glibc-ld.so.cache1.1`）のみサポート
- `parseLDCacheData(data []byte) (*LDCache, error)`: unexported ヘルパー関数。テスト用に合成キャッシュデータから直接パース可能（同一パッケージ内からアクセス）
- `Lookup(soname string) string`: soname → パス検索
- ヘッダー・エントリ構造体の定義
- `extractCString` ヘルパー

##### 2.2.4 `ResolveContext`: RUNPATH コンテキスト管理

DT_RPATH は非サポート。DT_RPATH を持つ ELF を検出した場合は `ErrDTRPATHNotSupported` を返す。
`ResolveContext` は `OwnRUNPATH` のみを保持する（`OwnRPATH`, `InheritedRPATH`, `ExpandedRPATHEntry` は削除済み）。

**ファイル**: `internal/dynlibanalysis/resolver_context.go`

- `ResolveContext` 構造体（`ParentPath`, `ParentDir`, `OwnRUNPATH`, `IncludeLDLibraryPath`）
- `NewRootContext(binaryPath string, runpath []string, includeLDLibraryPath bool)`: ルートバイナリ用の初期コンテキスト
- `NewChildContext(childPath string, childRUNPATH []string)`: 子依存用のコンテキスト（RPATH 継承なし）
- `expandOrigin()`: `$ORIGIN` 展開ヘルパー

##### 2.2.5 `LibraryResolver`: パス解決

**ファイル**: `internal/dynlibanalysis/resolver.go`

- `LibraryResolver` 構造体（cache, archPaths）— `fs` フィールドなし
- `NewLibraryResolver(cache *LDCache, elfMachine elf.Machine) *LibraryResolver`
  - `cache` は呼び出し元（`DynLibAnalyzer` / `DynLibVerifier`）が構築時に 1 回パースして渡す
  - ld.so.cache のパースは `NewLibraryResolver` の責務**ではない**
  - `safefileio.FileSystem` は不要: `tryResolve` は `os.Lstat` + `filepath.EvalSymlinks` を直接使用（`safefileio` はコンテンツ読み取り向け、パス存在確認向けではない）
- `Resolve(soname string, ctx *ResolveContext) (string, error)`: 4 段階の優先順位で解決
  1. LD_LIBRARY_PATH（`IncludeLDLibraryPath=true` 時のみ）
  2. OwnRUNPATH
  3. /etc/ld.so.cache
  4. デフォルトパス
  （DT_RPATH は非サポートのため検索ステップなし）
- `tryResolve(candidate string) (string, error)`: 存在確認 + `EvalSymlinks` + `Clean`

##### 2.2.6 テストデータ作成

**ファイル**: `internal/dynlibanalysis/testdata/README.md`

- テストデータの説明と生成方法
- `ldcache_new_format.bin`: Go のテストコード内で最小構成のバイナリデータを生成

#### 完了条件

- [x] `dynlibanalysis` パッケージが作成されていること
- [x] エラー型が全て定義されていること（`ErrDTRPATHNotSupported` を含む）
- [x] `DefaultSearchPaths` がアーキテクチャ別に正しいパスを返すこと
- [x] `ParseLDCache` が新形式のキャッシュを正しくパースすること
- [x] `ParseLDCache` がキャッシュ不在・フォーマット不正時にエラーを返すこと
- [x] `NewLibraryResolver(cache, machine)` が `cache` を引数で受け取り、自身で `ParseLDCache` を呼ばないこと（`fs` 引数なし）
- [x] `ResolveContext` が `OwnRUNPATH` のみを保持すること（`OwnRPATH`, `InheritedRPATH` は削除済み）
- [x] `ResolveContext.NewChildContext` が RUNPATH を継承せず子自身の RUNPATH のみを設定すること
- [x] `LibraryResolver.Resolve` が 4 段階の優先順位（LD_LIBRARY_PATH → RUNPATH → cache → default）で解決すること
- [x] `$ORIGIN` が RUNPATH で正しく展開されること
- [x] 全ユニットテストがパスすること
- [x] `make lint` / `make fmt` がパスすること

---

### 2.3 Phase 3: DynLibAnalyzer — record 拡張 (2 日)

**目標**: `record` コマンドで ELF バイナリの動的ライブラリ依存関係を再帰的に解決・記録する。

#### 実装対象

```
internal/dynlibanalysis/analyzer.go        # NEW
internal/dynlibanalysis/analyzer_test.go   # NEW
internal/filevalidator/validator.go        # 拡張
cmd/record/main.go                         # 拡張
```

#### 実装内容

##### 2.3.1 `DynLibAnalyzer` 実装

**ファイル**: `internal/dynlibanalysis/analyzer.go`

- `DynLibAnalyzer` 構造体（fs, cache）— `resolver` フィールドは持たない
- `NewDynLibAnalyzer(fs) *DynLibAnalyzer`: 構築時に `ParseLDCache` を 1 回呼び `cache` を保持
- `Analyze()` 内でバイナリのアーキテクチャが確定してから `NewLibraryResolver(cache, machine)` を呼ぶ
- `Analyze(binaryPath string) (*DynLibDepsData, error)`:
  - ELF パース → `DT_NEEDED`, `DT_RPATH`, `DT_RUNPATH` 取得
  - 非 ELF → `nil, nil`（正常）
  - `DT_NEEDED` なし → `nil, nil`（正常）
  - BFS キューで再帰的依存解決
  - `knownVDSOs` スキップ
  - `visitedKey`（resolvedPath + inheritedRPATHKey）で重複防止
  - `MaxRecursionDepth`（20 段）超過時はエラー
  - ライブラリ解決失敗時はエラー（何も永続化しない）
- `computeFileHash(path string) (string, error)`: パッケージレベル関数。`safefileio.SafeReadFile` + SHA256。`DynLibAnalyzer` と `DynLibVerifier` の両方から呼び出す（DRY）
- `parseELFDeps(path string) (needed, rpath, runpath []string, err error)`
- `splitPathList(pathLists []string) []string`

##### 2.3.2 `Validator` への `DynLibAnalyzer` 注入

**ファイル**: `internal/filevalidator/validator.go`

- `Validator.dynlibAnalyzer` フィールド追加（`*dynlibanalysis.DynLibAnalyzer`、nil 可）
- `LoadRecord(filePath string) (*fileanalysis.Record, error)` メソッド追加
- `FileValidator` インターフェースに `LoadRecord` 追加
- `updateAnalysisRecord` の `store.Update` コールバック内に DynLibDeps 解析を統合

> **設計注記（I/O と store.Update の関係）**: `store.Update` は現時点でファイルロック機構を持たない（Load → インメモリ修正 → Save の単純シーケンス）。そのため、コールバック内で重いI/O（再帰 ELF 探索・SHA256 計算等）を行っても「ロック保持期間の肥大化」は発生しない。並行 `record` 実行時の競合ウィンドウはコールバック外に出しても縮まるだけでゼロにはならず、YAGNI 観点からも今の設計（コールバック内で解析）で問題ない。将来 `store.Update` にファイルロックが導入される場合は、解析を事前に完了させてから `store.Update` を呼ぶ形にリファクタリングすること。

> **設計注記（Setter メソッド）**: `Validator` は `filevalidator` パッケージに属し、フィールドはパッケージ外から直接アクセス不可である。`cmd/record/main.go` から注入するため、**公開セッターメソッド**（`SetDynLibAnalyzer`, `SetBinaryAnalyzer`）を追加する。

##### 2.3.3 `record` コマンドへの統合

**ファイル**: `cmd/record/main.go`

- `deps.validatorFactory` のシグネチャを `func(hashDir string) (*filevalidator.Validator, error)` に変更する
  （戻り値をインターフェース型 `hashRecorder` から具象型 `*filevalidator.Validator` に変更）
- `run()` 関数内で `DynLibAnalyzer` を作成し、`validatorFactory` が返した `*Validator` の `SetDynLibAnalyzer` セッターで注入する
- `processFiles` には引き続き `hashRecorder` インターフェースとして渡す（シグネチャ変更不要）
- `processFiles` のフロー変更なし（`Record()` 内部で自動的に解析される）

> **根拠**: `syscallAnalysisContext` が既に「`run()` で構築し依存を構築時に解決する」パターンを採用している。同じパターンに揃えることで一貫性を保ち、インターフェースを汚染しない。

#### 完了条件

- [x] `DynLibAnalyzer.Analyze` が動的 ELF から `DynLibDepsData` を返すこと
- [x] `DynLibAnalyzer.Analyze` が非 ELF / 静的 ELF で `nil` を返すこと
- [x] `DynLibAnalyzer.Analyze` が DT_RPATH を持つ ELF で `ErrDTRPATHNotSupported` を返すこと
- [x] `LibEntry` に `soname`, `path`, `hash` が正しく記録されること（`parent_path`, `inherited_rpath` は削除済み）
- [x] 間接依存が再帰的に解決・記録されること
- [x] `traversalKey` が `resolvedPath` のみであること
- [x] vDSO がスキップされること
- [x] 循環依存で無限ループしないこと
- [x] 再帰深度超過時にエラーで `record` が失敗すること
- [x] ライブラリ解決失敗時にエラーで `record` が失敗し、何も永続化されないこと
- [x] `record --force` で `DynLibDeps` が更新されること
- [x] `FileValidator.LoadRecord` が正しくレコードを返すこと
- [x] 既存テストが全てパスすること
- [x] `make lint` / `make fmt` がパスすること

---

### 2.4 Phase 4: DynLibVerifier — runner 拡張 (2 日)

**目標**: `runner` 実行時にハッシュ検証でライブラリの整合性を検証する。LD_LIBRARY_PATH は runner 実行前にクリアされるため、第 2 段階（パス再解決）は不要。

#### 実装対象

```
internal/dynlibanalysis/verifier.go        # NEW
internal/dynlibanalysis/verifier_test.go   # NEW
internal/verification/manager.go           # 拡張
internal/verification/interfaces.go        # 拡張（ManagerInterface への VerifyCommandDynLibDeps 追加）
internal/runner/group_executor.go          # 拡張（コマンドごとの VerifyCommandDynLibDeps 呼び出し）
internal/verification/testing/testify_mocks.go  # 更新（MockManager, MockFileValidator への追加）
```

#### 実装内容

##### 2.4.1 `DynLibVerifier` 実装

**ファイル**: `internal/dynlibanalysis/verifier.go`

- `DynLibVerifier` 構造体（fs のみ）— `cache` は不要（パス再解決は行わないため）
- `NewDynLibVerifier(fs) *DynLibVerifier`: 単純な構造体初期化のみ
- `Verify(binaryPath string, deps *DynLibDepsData) error`:
  - 各 `LibEntry.Path` のハッシュ計算 → `LibEntry.Hash` と比較
  - 第 2 段階（パス再解決）は削除済み（LD_LIBRARY_PATH は runner 実行前にクリア済み）
- `computeFileHash` は `analyzer.go` で定義されたパッケージ共有関数を使用（DRY）

##### 2.4.2 `verification.Manager` への統合

**ファイル**: `internal/verification/manager.go`

- `Manager.dynlibVerifier *dynlibanalysis.DynLibVerifier` フィールド追加
- `newManagerInternal` 関数内で `NewDynLibVerifier(fs)` を呼び、`m.dynlibVerifier` に保持する
  - 既存コードは `newManagerInternal` + functional options パターンを採用しているため、このコンストラクタ内でフィールドを直接初期化する
  - `InternalOption` による注入は不要（`fs` は常に存在するため条件分岐不要）
- `verifyDynLibDeps(cmdPath string) error` 非公開ヘルパー追加:
  1. `m.fileValidator.LoadRecord(cmdPath)` でレコード取得
  2. `DynLibDeps` あり → `m.dynlibVerifier.Verify()`（ハッシュ検証のみ）
  3. `DynLibDeps` なし & 動的リンク ELF（`DT_NEEDED` あり）→ `ErrDynLibDepsRequired`
  4. `DynLibDeps` なし & 静的 ELF / `DT_NEEDED` なし ELF / 非 ELF → `nil`（正常）
- `hasDynamicLibraryDeps(path string) (bool, error)` ヘルパー追加（`elf.Open` で `DT_NEEDED` の有無を確認）
- `VerifyCommandDynLibDeps(cmdPath string) error` **公開メソッド**追加（内部で `verifyDynLibDeps` を呼ぶ）
- `ManagerInterface` に `VerifyCommandDynLibDeps` を追加（`internal/verification/interfaces.go`）

> **注意**: `VerifyGroupFiles` 内での呼び出しは採用しない。`collectVerificationFiles` が返す `map[string]struct{}` の時点でファイルの由来（コマンドファイルか `verify_files` か）が失われており、`isCommandFile` を判定できないためである。

**呼び出し元**: `internal/runner/group_executor.go` の `verifyGroupFiles` 内で、`VerifyGroupFiles` 成功後に `runtimeGroup.Commands` をループして各コマンドパスに `VerifyCommandDynLibDeps` を呼ぶ。

##### 2.4.3 `FileValidator` インターフェースのモック更新

既存のテスト用モック（`MockFileValidator` 等）に `LoadRecord` メソッドを追加する。

##### 2.4.4 `MockManager` の更新

`internal/verification/testing/testify_mocks.go` の `MockManager` に `VerifyCommandDynLibDeps` を追加する。

#### 完了条件

- [x] ハッシュ一致で検証成功すること
- [x] ハッシュ不一致で `ErrLibraryHashMismatch` が返ること
- [x] 第 2 段階（パス再解決）は削除済み（`ErrLibraryPathMismatch` は使用しない）
- [x] 空パスのエントリで `ErrEmptyLibraryPath` が返ること
- [x] 動的リンク ELF（`DT_NEEDED` あり）に `DynLibDeps` がない場合に `ErrDynLibDepsRequired` が返ること
- [x] 静的 ELF / `DT_NEEDED` なし ELF に `DynLibDeps` がない場合は正常動作すること
- [x] 非 ELF バイナリに `DynLibDeps` がない場合は正常動作すること
- [x] `schema_version: 1` の記録で `SchemaVersionMismatchError` が返ること
- [x] エラーメッセージにライブラリ名・パス・ハッシュ等の必要情報が含まれること
- [x] `VerifyCommandDynLibDeps` が `ManagerInterface` に追加されていること
- [x] `group_executor.go` の `verifyGroupFiles` が `VerifyGroupFiles` 成功後にコマンドごとに `VerifyCommandDynLibDeps` を呼び出すこと
- [x] `MockManager` に `VerifyCommandDynLibDeps` が追加されていること
- [x] 既存テストが全てパスすること
- [x] `make lint` / `make fmt` がパスすること

---

### 2.5 Phase 5: dlopen シンボル検出 + 仕上げ (1.5 日)

**目標**: `dlopen` / `dlsym` / `dlvsym` の使用を検出して高リスク判定する。全テストのパスを確認し、ドキュメントを更新する。

#### 実装対象

```
internal/runner/security/binaryanalyzer/network_symbols.go   # 拡張
internal/runner/security/binaryanalyzer/analyzer.go           # 拡張
internal/runner/security/elfanalyzer/standard_analyzer.go     # 拡張
internal/runner/security/machoanalyzer/standard_analyzer.go   # 拡張
internal/runner/security/network_analyzer.go                  # 拡張
cmd/record/main.go                                            # 拡張（HasDynamicLoad 記録）
```

#### 実装内容

##### 2.5.1 `dynamicLoadSymbolRegistry` 追加

**ファイル**: `internal/runner/security/binaryanalyzer/network_symbols.go`

- `CategoryDynamicLoad SymbolCategory = "dynamic_load"`
- `dynamicLoadSymbolRegistry map[string]struct{}`: `dlopen`, `dlsym`, `dlvsym`
- `IsDynamicLoadSymbol(name string) bool`

##### 2.5.2 `AnalysisOutput.HasDynamicLoad` 追加

**ファイル**: `internal/runner/security/binaryanalyzer/analyzer.go`

- `AnalysisOutput` に `HasDynamicLoad bool` フィールド追加

##### 2.5.3 ELF/Mach-O アナライザーの拡張

**ファイル**: `internal/runner/security/elfanalyzer/standard_analyzer.go`

- シンボルチェックループに `IsDynamicLoadSymbol` 判定追加
- `hasDynamicLoad` フラグを `AnalysisOutput.HasDynamicLoad` にセット

**ファイル**: `internal/runner/security/machoanalyzer/standard_analyzer.go`

- インポートシンボルチェックループに `IsDynamicLoadSymbol` 判定追加
- `hasDynamicLoad` フラグを `AnalysisOutput.HasDynamicLoad` にセット

##### 2.5.4 `network_analyzer.go` の拡張

**ファイル**: `internal/runner/security/network_analyzer.go`

**`NewBinaryAnalyzer` の追加:**
- `NewBinaryAnalyzer() binaryanalyzer.BinaryAnalyzer` を新規公開関数として追加
- `NewNetworkAnalyzer` のプラットフォーム選択ロジック（`runtime.GOOS` switch）を本関数に委譲
- `cmd/record/main.go` はこの関数を呼び出して `BinaryAnalyzer` を取得する

**`isNetworkViaBinaryAnalysis` の拡張:**
- 戻り値を `bool` から `(isNetwork, isHighRisk bool)` に変更する
  - `HasDynamicLoad` と `output.Result` は独立したシグナルとして処理する
  - `HasDynamicLoad=true` の場合は `isHighRisk=true` をセット（既存ケースの `isNetwork` 判定には影響しない）
  - `NetworkDetected` / `AnalysisError` 等の既存ケースは `(true, isHighRisk)` を返す（`isHighRisk` は `HasDynamicLoad` の値による）
  - `dlopen+socket` を両方持つバイナリは `(true, true)` を返す（ネットワーク操作かつ高リスク）
- `IsNetworkOperation` 内の呼び出しを `isNet, isHigh := a.isNetworkViaBinaryAnalysis(...)` に変更
- ログ出力: `"Binary analysis detected dynamic load symbols (dlopen/dlsym/dlvsym)"`

> **注意**: `isNetworkViaBinaryAnalysis` が `true` を返すだけでは `IsNetworkOperation` が `(true, false)` を返し `RiskLevelMedium`（中リスク）になる。`RiskLevelHigh` にするには `isHighRisk=true` を `EvaluateRisk` まで伝播する必要がある。
>
> **セマンティクスの原則**: `isNetwork` と `isHighRisk` は独立して設定する。`HasDynamicLoad=true` かつ `NetworkDetected=true` の場合に `isNetwork=false` を返すと呼び出し元でのログ・監査時に情報が失われる。`(true, true)` を返すことで両シグナルを正確に伝播する。

##### 2.5.5 `HasDynamicLoad` の record 時記録

**ファイル**: `internal/filevalidator/validator.go`, `cmd/record/main.go`

案 A（`updateAnalysisRecord` の `store.Update` コールバック内に統合）を採用する。

案 B（`processFiles` 内で別途 `store.Update`）は採用しない。理由:
- `AnalyzeNetworkSymbols` はパッケージレベル関数ではなく `BinaryAnalyzer` インターフェースのメソッドであり、`processFiles` から直接呼べない
- `true` の時のみ書き込むと再 record 後に stale な `true` が残る
- `store.Update` を 2 回呼ぶと 2 回目が `DynLibDeps` を消去するリスクがある

実装内容:
- `Validator.binaryAnalyzer binaryanalyzer.BinaryAnalyzer` フィールド追加（nil 可）, `SetBinaryAnalyzer` セッター追加
- `updateAnalysisRecord` コールバック内で `record.HasDynamicLoad = output.HasDynamicLoad` を常に代入（true/false 両方）
- `cmd/record/main.go` の `run()` で `security.NewBinaryAnalyzer()` を呼び、`SetBinaryAnalyzer` セッターで注入する
  （§2.3.3 と同様に `deps.validatorFactory` が `*filevalidator.Validator` を返す設計を利用する）

##### 2.5.6 全体テスト・仕上げ

- `dlopen` シンボル検出のユニットテスト
- `HasDynamicLoad: true` のバイナリで `runner` が高リスク扱いになることの統合テスト
- 既存テストの全パス確認
- `make test` / `make lint` / `make fmt` 全パス確認
- CHANGELOG 更新

#### 完了条件

- [x] `IsDynamicLoadSymbol` が `dlopen/dlsym/dlvsym` を認識すること
- [x] `HasDynamicLoad` が `NetworkDetected` とは独立して設定されること
- [x] ELF アナライザーで `dlopen` 使用バイナリが `HasDynamicLoad: true` と判定されること
- [x] Mach-O アナライザーで `dlopen` 使用バイナリが `HasDynamicLoad: true` と判定されること（コード実装済み、macOS 環境でのみ実行可能）
- [x] `isNetworkViaBinaryAnalysis` が `HasDynamicLoad: true` かつ `NetworkDetected: false` 時に `(false, true)` を返し、`EvaluateRisk` が `RiskLevelHigh` を返すこと
- [x] `isNetworkViaBinaryAnalysis` が `HasDynamicLoad: true` かつ `NetworkDetected: true` 時に `(true, true)` を返し、`EvaluateRisk` が `RiskLevelHigh` を返すこと
- [x] `record` で `HasDynamicLoad: true` のバイナリに `true` が保存されること
- [x] `record` で `HasDynamicLoad: false` のバイナリに `false` が保存されること（stale 値上書き確認）
- [x] `Validator.binaryAnalyzer` が未設定の場合でも `Record()` が正常動作すること
- [x] 既存の `ContentHash` 検証が正常に動作すること
- [x] `SyscallAnalysis` フィールドが保持されること
- [x] 既存のテストがすべてパスすること
- [x] `make test` / `make lint` / `make fmt` が全てパスすること

## 3. タスク依存関係

### 3.1 前提条件

- タスク 0069（ELF .dynsym 解析）が完了済みであること
- タスク 0070/0072（ELF syscall 解析）が完了済みであること（`SyscallAnalysis` フィールド共存のため）
- タスク 0073（Mach-O 解析）が完了済みであること（`dlopen` 検出の macOS 波及のため）

### 3.2 実装順序の依存関係

```mermaid
gantt
    title 実装フェーズ依存関係
    dateFormat YYYY-MM-DD
    axisFormat %m/%d

    section Phase 1
    スキーマ拡張・型定義          :p1, 2026-03-07, 1d

    section Phase 2
    エラー型定義                  :p2a, after p1, 1d
    ld.so.cache パーサー          :p2b, after p2a, 1d
    ResolveContext                :p2c, after p2a, 1d
    LibraryResolver               :p2d, after p2b p2c, 1d
    デフォルトパス                :p2e, after p2a, 1d

    section Phase 3
    DynLibAnalyzer                :p3a, after p2d, 1d
    Validator 拡張                :p3b, after p3a, 1d
    record コマンド拡張           :p3c, after p3b, 1d

    section Phase 4
    DynLibVerifier                :p4a, after p2d, 1d
    Manager 統合                  :p4b, after p4a, 1d

    section Phase 5
    dlopen シンボル検出           :p5a, after p1, 1d
    仕上げ・全体テスト            :p5b, after p3c, 1d
```

### 3.3 並行実装可能なタスク

| タスクグループ | 含まれるタスク | 前提条件 |
|-------------|------------|---------|
| A: 型定義基盤 | Phase 1 | なし |
| B: 解決エンジン | Phase 2 の全項目 | Phase 1 完了 |
| C: record 拡張 | Phase 3 | Phase 2 完了 |
| D: runner 拡張 | Phase 4 | Phase 2 完了 |
| E: dlopen 検出 | Phase 5 の 2.5.1〜2.5.4 | Phase 1 完了 |

Phase 3（record 拡張）と Phase 4（runner 拡張）は Phase 2 完了後に並行実装可能。
Phase 5 の dlopen 検出部分は Phase 1 完了後に独立して実装可能。

## 4. リスク分析と対策

### 4.1 技術的リスク

#### 4.1.1 HIGH: `ld.so.cache` のフォーマット差異

**リスク**: 異なる glibc バージョンや Linux ディストリビューションで `ld.so.cache` のバイナリフォーマットが異なる可能性がある。

**対策**: 新形式（`glibc-ld.so.cache1.1`）のみをサポート。パース失敗時はデフォルトパスにフォールバックする。テストデータとして最小構成のキャッシュバイナリを用意し、フォーマット差異の早期検出を可能にする。

**検出方法**: `ParseLDCache` が `nil` を返した場合にログ出力。CI 環境でのテスト実行で検出。

#### 4.1.2 MEDIUM: RPATH 継承ルールの複雑さ

**リスク**: `ld.so` の RPATH/RUNPATH 継承ルールを正確に実装しないと、偽陽性（正規ライブラリの解決失敗）が発生する。

**対策**: `ResolveContext.NewChildContext` のユニットテストで継承チェーンの正確性を検証。`ld.so(8)` のマニュアルに基づく実装。

**検出方法**: システムバイナリ（`/bin/ls` 等）を使った統合テストで実環境での動作を確認。

#### 4.1.3 LOW: パフォーマンス（大量依存ライブラリ）

**リスク**: 依存ライブラリ数が非常に多いバイナリで `record` / `runner` の処理時間が長くなる。

**対策**: `visited` セットで重複解析を防止。`ld.so.cache` は `DynLibAnalyzer` の構築時に 1 回のみパースし、`cache *LDCache` として保持する。`NewLibraryResolver` はキャッシュを引数で受け取るためパースを行わない。`DynLibVerifier` はハッシュ検証のみを行うため `ld.so.cache` は不要。典型バイナリは 10〜30 依存であり、上限 20 段で制御。

### 4.2 セキュリティリスク

#### 4.2.1 HIGH: `CurrentSchemaVersion` 変更の影響

**リスク**: バージョン 1 から 2 への変更後、全管理対象バイナリの `record --force` 再実行が必要。失念すると `runner` が全コマンドをブロックする。

**対策**: README に移行手順を明記。`SchemaVersionMismatchError` のエラーメッセージに `record --force` の再実行を促す文言を含める。

#### 4.2.2 MEDIUM: `dlopen` 検出の false positive 増加

**リスク**: `python3`, `bash`, `git` 等の一般的なコマンドが `HasDynamicLoad: true` となり、高リスク扱いになる。

**対策**: 仕様通りの動作として文書化。`dlopen` を使うバイナリが多数存在することは事前に想定済み。

### 4.3 運用リスク

#### 4.3.1 MEDIUM: 依存ライブラリのセキュリティアップデート

**リスク**: OS パッケージアップデートでライブラリが更新されると、`runner` が全コマンドをブロックする。

**対策**: エラーメッセージに `record --force` の再実行を促す文言を含める。運用ガイドでアップデート後の `record` 再実行手順を文書化する。

## 5. 品質保証計画

### 5.1 テストピラミッド

```mermaid
graph TB
    classDef tier1 fill:#ffb86b,stroke:#333,color:#000;
    classDef tier2 fill:#ffd59a,stroke:#333,color:#000;
    classDef tier3 fill:#c3f08a,stroke:#333,color:#000;

    T1["統合テスト (8 ケース)"]:::tier1
    T2["コンポーネントテスト (1 ケース)"]:::tier2
    T3["単体テスト (44 ケース)"]:::tier3

    T3 --> T2 --> T1
```

### 5.2 テストカバレッジ目標

| パッケージ | 目標カバレッジ | 重点テスト領域 |
|----------|-------------|-------------|
| `dynlibanalysis` | 80% | 解決優先順位、RPATH 継承、循環防止 |
| `fileanalysis` | 既存維持 | スキーマバージョン変更の影響 |
| `filevalidator` | 既存維持 | `LoadRecord`, DynLibDeps 解析統合 |
| `verification` | 既存維持 | `verifyDynLibDeps` 統合 |
| `binaryanalyzer` | 既存維持 | `IsDynamicLoadSymbol`, `HasDynamicLoad` |

### 5.3 テストケース一覧

#### 単体テスト

| # | テストケース | パッケージ | 検証内容 |
|---|-------------|----------|---------|
| 1 | `TestResolve_RUNPATH` | `dynlibanalysis` | RUNPATH ディレクトリからの解決 |
| 2 | `TestResolve_Origin` | `dynlibanalysis` | `$ORIGIN` → ParentDir 展開 |
| 3 | `TestResolve_LDCache` | `dynlibanalysis` | ld.so.cache 経由の解決 |
| 4 | `TestResolve_DefaultPaths` | `dynlibanalysis` | アーキテクチャ別デフォルトパス |
| 5 | `TestResolve_Failure` | `dynlibanalysis` | 解決失敗エラーの内容検証 |
| 6 | `TestParseLDCache_NewFormat` | `dynlibanalysis` | 新形式キャッシュのパース |
| 7 | `TestParseLDCache_NotFound` | `dynlibanalysis` | キャッシュ不在時の動作 |
| 8 | `TestParseLDCache_UnsupportedFormat` | `dynlibanalysis` | 非対応フォーマット |
| 9 | `TestParseLDCache_Truncated` | `dynlibanalysis` | データ切れ |
| 10 | `TestLDCache_Lookup` | `dynlibanalysis` | soname → パス検索 |
| 11 | `TestAnalyze_DynamicELF` | `dynlibanalysis` | 動的 ELF の解析 |
| 15 | `TestAnalyze_NonELF` | `dynlibanalysis` | 非 ELF → nil |
| 16 | `TestAnalyze_StaticELF` | `dynlibanalysis` | 静的 ELF → nil |
| 17 | `TestAnalyze_DTRPATHNotSupported` | `dynlibanalysis` | DT_RPATH を持つ ELF → ErrDTRPATHNotSupported |
| 18 | `TestAnalyze_LibEntryFields` | `dynlibanalysis` | LibEntry のフィールド（soname, path, hash）が正しく記録されること |
| 19 | `TestAnalyze_Force` | `filevalidator` | `record --force` で DynLibDeps が更新されること |
| 20 | `TestAnalyze_TransitiveDeps` | `dynlibanalysis` | 間接依存の再帰解決 |
| 21 | `TestAnalyze_CircularDeps` | `dynlibanalysis` | 循環依存防止 |
| 22 | `TestAnalyze_MaxDepth` | `dynlibanalysis` | 深度超過エラー |
| 23 | `TestAnalyze_ResolutionFailure` | `dynlibanalysis` | 解決失敗 → record 失敗 |
| 24 | `TestVerify_HashMatch` | `dynlibanalysis` | ハッシュ一致 → 成功 |
| 25 | `TestVerify_HashMismatch` | `dynlibanalysis` | ハッシュ不一致 → エラー |
| 26 | `TestVerify_EmptyPath` | `dynlibanalysis` | 空パス → エラー |
| 27 | `TestVerify_SchemaVersion` | `verification` | schema_version 1 → SchemaVersionMismatchError |
| 28 | `TestVerify_ELFNoDynLibDeps` | `verification` | 動的 ELF（DT_NEEDED あり）に DynLibDeps なし → ErrDynLibDepsRequired |
| 29 | `TestVerify_NonELFNoDynLibDeps` | `verification` | 非 ELF に DynLibDeps なし → 正常動作 |
| 30 | `TestVerify_FutureSchemaVersion` | `verification` | schema_version が現行より新しい → SchemaVersionMismatchError |
| 31 | `TestIsDynamicLoadSymbol` | `binaryanalyzer` | dlopen/dlsym/dlvsym 認識 |
| 32 | `TestHasDynamicLoad_ELF` | `elfanalyzer` | ELF で dlopen 使用 → HasDynamicLoad=true |
| 33 | `TestHasDynamicLoad_Independent` | `binaryanalyzer` | `NetworkDetected` と独立して設定されること（dlopen+socket 両方 → `HasDynamicLoad=true` かつ `NetworkDetected=true`） |
| 34 | `TestRecord_HasDynamicLoad_True` | `filevalidator` | dlopen 使用バイナリで HasDynamicLoad=true が記録されること |
| 35 | `TestRecord_HasDynamicLoad_WrittenWhenFalse` | `filevalidator` | dlopen 不使用バイナリで HasDynamicLoad=false が記録されること（stale 値上書き確認） |
| 36 | `TestRecord_BinaryAnalyzerNil_NoError` | `filevalidator` | binaryAnalyzer 未設定時でも Record() が正常動作すること |

#### コンポーネントテスト

| # | テストケース | パッケージ | 検証内容 |
|---|-------------|----------|---------|
| 1 | `TestVerifyGroupFiles_DynLibNotCalledForVerifyFiles` | `runner`（`group_executor_test.go`） | `verify_files` に含まれる非コマンドファイルには `VerifyCommandDynLibDeps` が呼ばれないことを `MockManager` で検証（モックを使った呼び出しパターンの単体テスト） |

#### 統合テスト

| # | テストケース | 検証内容 |
|---|-------------|---------|
| 1 | `TestIntegration_RecordAndVerify_Normal` | record → runner ハッシュ検証成功 |
| 2 | `TestIntegration_LibraryTamperingDetection` | ライブラリ改ざん → ブロック |
| 3 | `TestIntegration_OldSchemaRejection` | schema_version 1 or 2 → ブロック |
| 4 | `TestIntegration_NonELFNormal` | 非 ELF → 従来検証のみ |
| 5 | `TestIntegration_EmptyPathDefense` | path="" → ブロック |
| 6 | `TestIntegration_HasDynamicLoad` | dlopen 使用バイナリ → 高リスク判定 |
| 7 | `TestIntegration_RecordFailure` | ライブラリ解決失敗 → record 失敗 |
| 8 | `TestIntegration_DTRPATHRejection` | DT_RPATH を持つバイナリの record → 失敗（ErrDTRPATHNotSupported） |

## 6. 成功基準

### 6.1 機能要件

- [x] AC-1: ライブラリパス解決が全ケースで正しく動作すること（DT_RPATH は ErrDTRPATHNotSupported で拒否）
- [x] AC-2: `record` で `DynLibDeps` が正しく記録されること（LibEntry は soname, path, hash のみ）
- [x] AC-3: `runner` のハッシュ検証が全ケースで正しく動作すること（第 2 段階は削除済み）
- [x] AC-4: `dlopen` シンボルが正しく検出・判定されること
- [x] AC-5: 既存機能への非影響が確認されていること

### 6.2 非機能要件

- [x] `record` の解析時間が一般的なバイナリで実用的な範囲内であること
- [x] ライブラリファイル読み取りに `safefileio` が使用されていること
- [x] 全パスが `filepath.EvalSymlinks` + `filepath.Clean` で正規化されていること
- [x] `ldd` / `ldconfig` 等の外部コマンドに依存していないこと

### 6.3 品質要件

- [x] `make test` が全てパスすること
- [x] `make lint` が全てパスすること
- [x] `make fmt` が全てパスすること
- [x] 新規コードのテストカバレッジが 80% 以上であること

## 7. 実装チェックリスト

### Phase 1: スキーマ拡張
- [x] `fileanalysis/schema.go`: `CurrentSchemaVersion = 2`（1 → 2 に変更）
- [x] `fileanalysis/schema.go`: `DynLibDepsData`, `LibEntry` 型定義（soname, path, hash のみ）
- [x] `fileanalysis/schema.go`: `Record.DynLibDeps`, `Record.HasDynamicLoad` 追加
- [x] 既存テストの `CurrentSchemaVersion` 依存箇所を更新

### Phase 2: ライブラリ解決エンジン
- [x] `dynlibanalysis/doc.go`: パッケージドキュメント
- [x] `dynlibanalysis/errors.go`: 全エラー型定義（`ErrDTRPATHNotSupported` を含む）
- [x] `dynlibanalysis/default_paths.go`: `DefaultSearchPaths`
- [x] `dynlibanalysis/ldcache.go`: `ParseLDCache`, `Lookup`
- [x] `dynlibanalysis/resolver.go`: `LibraryResolver`, `Resolve`（3 段階: RUNPATH → cache → default）, `tryResolve`, `expandOrigin`
- [x] `dynlibanalysis/testdata/README.md`
- [x] `dynlibanalysis/default_paths_test.go`
- [x] `dynlibanalysis/ldcache_test.go`
- [x] `dynlibanalysis/resolver_test.go`

### Phase 3: DynLibAnalyzer（record 拡張）
- [x] `dynlibanalysis/analyzer.go`: `DynLibAnalyzer`, `Analyze`（DT_RPATH 検出→エラー, `resolveItem` は parentPath/runpath を直接保持）, ヘルパー関数
- [x] `dynlibanalysis/analyzer_test.go`
- [x] `filevalidator/validator.go`: `dynlibAnalyzer` フィールド, `SetDynLibAnalyzer` セッター, `LoadRecord`
- [x] `filevalidator/validator.go`: `FileValidator` IF に `LoadRecord` 追加
- [x] `filevalidator/validator.go`: `updateAnalysisRecord` コールバック拡張
- [x] `cmd/record/main.go`: `DynLibAnalyzer` 統合

### Phase 4: DynLibVerifier（runner 拡張）
- [x] `dynlibanalysis/verifier.go`: `DynLibVerifier`, `Verify`（ハッシュ検証のみ、第 2 段階は削除済み）
- [x] `dynlibanalysis/verifier_test.go`
- [x] `verification/manager.go`: `dynlibVerifier` フィールド追加・コンストラクタで 1 回生成
- [x] `verification/manager.go`: `verifyDynLibDeps`, `hasDynamicLibraryDeps`
- [x] `verification/interfaces.go`: `ManagerInterface` に `VerifyCommandDynLibDeps` 追加
- [x] `internal/runner/group_executor.go`: `verifyGroupFiles` からの `VerifyCommandDynLibDeps` 呼び出し統合
- [x] `verification/testing/testify_mocks.go`: `MockManager` に `VerifyCommandDynLibDeps` 追加
- [x] `verification/testing/testify_mocks.go`: `MockFileValidator` に `LoadRecord` 追加

### Phase 5: dlopen シンボル検出 + 仕上げ
- [x] `binaryanalyzer/network_symbols.go`: `CategoryDynamicLoad`, `dynamicLoadSymbolRegistry`, `IsDynamicLoadSymbol`
- [x] `binaryanalyzer/analyzer.go`: `AnalysisOutput.HasDynamicLoad`
- [x] `elfanalyzer/standard_analyzer.go`: `HasDynamicLoad` 検出
- [x] `machoanalyzer/standard_analyzer.go`: `HasDynamicLoad` 検出
- [x] `filevalidator/validator.go`: `binaryAnalyzer` フィールド, `SetBinaryAnalyzer` セッター追加・`updateAnalysisRecord` コールバック拡張
- [x] `cmd/record/main.go`: `SetBinaryAnalyzer` / `SetDynLibAnalyzer` で各アナライザーを注入
- [x] `network_analyzer.go`: `NewBinaryAnalyzer() binaryanalyzer.BinaryAnalyzer` 公開ファクトリ関数を追加（プラットフォーム選択ロジックを `NewNetworkAnalyzer` から分離）
- [x] `network_analyzer.go`: `isNetworkViaBinaryAnalysis` 戻り値を `(isNetwork, isHighRisk bool)` に変更し、`HasDynamicLoad` と `output.Result` を独立して処理する拡張（`dlopen+socket` 同時検出時は `(true, true)` を返す）
- [x] `network_analyzer.go`: `IsNetworkOperation` の呼び出し側を `isNet, isHigh := a.isNetworkViaBinaryAnalysis(...)` に更新
- [x] 全テストパス確認
- [x] `make lint` / `make fmt` パス確認

## 8. 参照

- [01_requirements.md](01_requirements.md): 要件定義書
- [02_architecture.md](02_architecture.md): アーキテクチャ設計書
- [03_detailed_specification.md](03_detailed_specification.md): 詳細仕様書
