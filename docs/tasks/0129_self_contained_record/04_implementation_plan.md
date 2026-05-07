# 実装計画書: コマンド Record 完全自己完結化

## 進捗サマリー

- [ ] Phase 1: スキーマ定義
- [ ] Phase 2: record コマンド（filevalidator）
- [ ] Phase 3: runner 更新
- [ ] Phase 4: 検証・仕上げ

---

## Phase 1: スキーマ定義

### 1-1. `CurrentSchemaVersion` を 22 に更新

- [ ] `internal/fileanalysis/schema.go`: `CurrentSchemaVersion = 21` → `22` に変更し、バージョン履歴コメントを追記する

### 1-2. 新規型の追加

- [ ] `internal/fileanalysis/schema.go`: `DepEntry` 型を追加する
  - フィールド: `SOName` (omitempty), `Path`, `Hash`, `SyscallAnalysis *SyscallAnalysisData`, `SymbolAnalysis *SymbolAnalysisData`, `Warnings []string`
  - `SOName` は共有ライブラリに設定、インタープリターバイナリエントリでは省略（空文字）
- [ ] `internal/fileanalysis/schema.go`: `ShebangBinaryInfo` 型を追加する
  - フィールド: `RawPath` (omitempty), `Path`, `CommandName` (omitempty), `ContentHash`
  - 解析フィールド（`SyscallAnalysis`、`SymbolAnalysis`）は含めない（`Deps` に統合）
- [ ] `internal/fileanalysis/schema.go`: `DebugInfo` 型を追加する
  - フィールド: `DepSources map[string][]string`

### 1-3. `Record` 構造体の変更

- [ ] `internal/fileanalysis/schema.go`: `Record` に `Deps []DepEntry`、`ShebangChain []ShebangBinaryInfo`、`Debug *DebugInfo` を追加する（`omitempty`）
- [ ] `internal/fileanalysis/schema.go`: `Record` から `DynLibDeps []LibEntry`、`ShebangInterpreter *ShebangInterpreterInfo`、`AnalysisWarnings []string` を削除する
- [ ] `internal/fileanalysis/schema.go`: `LibEntry` 型を削除する（`DepEntry` に置き換え）
- [ ] `internal/fileanalysis/schema.go`: `ShebangInterpreterInfo` 型を削除する

### 1-4. スキーマ変更に伴う既存コードの修正（コンパイルエラー解消）

- [ ] `Record.DynLibDeps` を参照しているすべての箇所を `Record.Deps` に置き換える（grep で特定）
- [ ] `Record.ShebangInterpreter` を参照しているすべての箇所を `Record.ShebangChain` に置き換える
- [ ] `Record.AnalysisWarnings` を参照しているすべての箇所を修正する
- [ ] `LibEntry` 型を参照しているすべての箇所を `DepEntry` に置き換えるか、内部用途のみ残す

### 1-5. スキーマ変更のテスト

- [ ] `internal/fileanalysis/schema_test.go` を新規作成し、以下のテストを追加する:
  - `DepEntry` JSON round-trip（`syscall_analysis` null / 非 null、`warnings` あり / なし）
  - `ShebangBinaryInfo` JSON round-trip（direct form / env form）
  - `DebugInfo` omitempty（`debug == nil` → JSON に `"debug"` キーなし）
  - `CurrentSchemaVersion == 22` の確認
- [ ] `fileanalysis.Store.Load` が `schema_version != 22` の Record に対して `SchemaVersionMismatchError` を返すことを確認する（既存テストを v21 フィクスチャで実行確認、または新規テスト追加）

---

## Phase 2: record コマンド（filevalidator）

### 2-1. `resolveShebangChain` の実装

- [ ] `internal/filevalidator/validator.go`: `resolveShebangChain(filePath string) ([]ShebangBinaryInfo, []binaryDynLibDeps, error)` を実装する
  - `shebang.Parse` でインタープリター情報を取得
  - 各バイナリ（`InterpreterPath`、`ResolvedPath`）について:
    1. `calculateHash` でハッシュ算出（`ShebangBinaryInfo.ContentHash` に設定）
    2. `elfDynlibAnalyzer.Analyze` で `DynLibDeps`（共有ライブラリ）を収集
    3. 一時 `fileanalysis.Record` に `DynLibDeps` を設定し `analyzeELFSyscalls(&tmpRecord, filePath)` を呼ぶ（`findLibcEntry` が `DynLibDeps` を必要とするため）
    4. `binaryAnalyzer.AnalyzeNetworkSymbols` で `SymbolAnalysis` を取得
  - `ShebangBinaryInfo` に識別情報（`RawPath`、`Path`、`CommandName`、`ContentHash`）のみを設定する（解析結果は `Deps` に含めるため `ShebangBinaryInfo` には含めない）
  - インタープリターバイナリ自体の解析結果（hash + syscall + symbol）は `binaryDynLibDeps` の形式で返し、`collectAndDedupDeps` で `DepEntry{SOName: ""}` として `deps` に追加する
  - インタープリターが shebang スクリプトの場合は `ErrRecursiveShebang` を返す（既存ロジックを維持）

### 2-2. `collectAndDedupDeps` の実装

- [ ] `internal/filevalidator/validator.go`: `collectAndDedupDeps(binaries []binaryWithDeps) ([]DepEntry, map[string][]string, error)` を実装する
  - 同一 `path` + 同一 `hash` → 統合（1エントリ）
  - 同一 `path` + 異なる `hash` → 即時エラー返却
  - `sources` マップ（ `DepSources` 用）を同時に構築する

### 2-3. `analyzeDepEntries` の実装

- [ ] `internal/filevalidator/validator.go`: `analyzeDepEntries(rawDeps []depRaw) ([]DepEntry, error)` を実装する
  - VDSO / syscall wrapper → `SyscallAnalysis == nil`、`SymbolAnalysis == nil`
  - それ以外 → in-session キャッシュまたは `dynamicLibAnalysisStore.LoadOrAnalyzeAndStore` で解析
  - 既存の `v.processedLibAnalysis` キャッシュを再利用する
  - `Warnings` をソート後 dedup して `DepEntry.Warnings` に設定する

### 2-4. `updateAnalysisRecord` の大幅変更

- [ ] `internal/filevalidator/validator.go`: `updateAnalysisRecord` を変更して以下を行う:
  - `resolveShebangChain` を呼ぶ（旧 `saveInterpreterRecord` 呼び出しは不要）
  - shebang チェーン各バイナリの `DynLibDeps` も収集する
  - `collectAndDedupDeps` で全 deps を dedup する
  - `analyzeDepEntries` で各 dep を解析する
  - `record.Deps = deps` を設定する
  - `record.ShebangChain = chain` を設定する
  - `v.includeDebugInfo` が true なら `record.Debug = &DebugInfo{DepSources: sources}` を設定する
  - `record.AnalysisWarnings`、`record.DynLibDeps`、`record.ShebangInterpreter` の設定を削除する（フィールド削除に伴い自動的に不要）

### 2-5. `SaveRecord` から `saveInterpreterRecord` 呼び出しを削除

- [ ] `internal/filevalidator/validator.go`: `SaveRecord` 内の `saveInterpreterRecord` 呼び出しブロックを削除する
- [ ] `internal/filevalidator/validator.go`: `saveInterpreterRecord` 関数を削除する

### 2-6. `analyzeLibraries` の変更

- [ ] `internal/filevalidator/validator.go`: `analyzeLibraries` を変更してデータを `DepEntry` スライスに書き込むようにする（または `analyzeDepEntries` に統合して `analyzeLibraries` を削除する）
- [ ] dep からの `DynamicLoadSymbols` を `record.SymbolAnalysis.DynamicLoadSymbols` にマージする処理を削除する（runner がランタイムに参照するため）

### 2-7. record コマンドのテスト

- [ ] `internal/filevalidator/validator_dedup_test.go` を新規作成し、以下のテストを追加する:
  - 同一 path + 同一 hash → 1 エントリ統合
  - 同一 path + 異なる hash → エラー、Record 書き出しなし（F-001 AC2 / 4.3）
  - syscall wrapper dep → `SyscallAnalysis == nil && SymbolAnalysis == nil`（F-001 AC3）
  - VDSO dep → `SyscallAnalysis == nil && SymbolAnalysis == nil`（F-001 AC3）
  - dep warnings → `DepEntry.Warnings` に記録（F-001 AC4）
- [ ] `internal/filevalidator/validator_shebang_test.go`: 別 Record ファイル生成を前提とするテストを削除し、`ShebangChain` エントリの検証に書き換える
- [ ] `internal/filevalidator/validator_library_analysis_test.go`: `AnalysisWarnings` の期待値を `DepEntry.Warnings` に変更する
- [ ] `cmd/record/main_test.go`: `-debug-info` あり / なしで `Debug` フィールドの有無を確認するテストを追加する（F-004 AC1, AC2）

---

## Phase 3: runner 更新

### 3-1. `RecordLoader` インターフェースの定義

- [ ] `internal/runner/base/security/network_analyzer.go`: `RecordLoader` インターフェースを定義する
  ```go
  type RecordLoader interface {
      LoadRecord(filePath string) (*fileanalysis.Record, error)
  }
  ```

### 3-2. `AnalysisDeps` の変更

- [ ] `internal/runner/base/security/network_analyzer.go`: `AnalysisDeps` から全既存フィールド（`NetworkSymbolStore`、`SyscallStore`、`DynLibDepsStore`、`LibAnalysisStore`、`ShebangStore`）を削除し、`RecordStore RecordLoader` のみにする

### 3-3. `ErrDepAnalysisNotEmbedded` の定義

- [ ] `internal/runner/base/security/network_analyzer.go` または `internal/fileanalysis/errors.go`: `ErrDepAnalysisNotEmbedded` を定義する

### 3-4. `analyzeBinarySignals` の変更

- [ ] `internal/runner/base/security/network_analyzer.go`: `analyzeBinarySignals` を変更して以下を行う:
  - `RecordStore.LoadRecord(cmdPath)` でレコードを読み込む
  - `record.ContentHash != contentHash` の場合はハッシュ不一致として高リスク扱い
  - コマンド本体の `SyscallAnalysis`、`SymbolAnalysis` から既存の `checkSyscallCache`、`checkAnalysisCache` 相当のシグナルを抽出する
  - `checkDepsSignals(record.Deps)` を呼ぶ
  - `followShebangChain(record.ShebangChain)` を呼ぶ

### 3-5. `checkDepsSignals` の実装

- [ ] `internal/runner/base/security/network_analyzer.go`: `checkDepsSignals(deps []fileanalysis.DepEntry) (isNetwork, isHighRisk bool, err error)` を実装する
  - VDSO / wrapper はスキップ
  - `SyscallAnalysis == nil && SymbolAnalysis == nil` かつ非 wrapper/VDSO → `ErrDepAnalysisNotEmbedded` を返す（fail-closed）
  - 既存の `analyzeDepSignals` ロジックを `*SyscallAnalysisData`、`*SymbolAnalysisData` を受け取る形に変更して再利用する

### 3-6. `followShebangChain`（解析目的）の廃止

- [ ] `internal/runner/base/security/network_analyzer.go`: `followShebangChain` 関数を削除する（インタープリターバイナリの解析結果は `Record.Deps` に含まれるため不要）
- [ ] `internal/runner/base/security/network_analyzer.go`: `ShebangStore.LoadInterpreterAnalysisPath` への依存を除去する（`AnalysisDeps` から `ShebangStore` を削除した時点で自動的に除去される）

### 3-7. `checkDynLibDepsNetwork` の削除

- [ ] `internal/runner/base/security/network_analyzer.go`: `checkDynLibDepsNetwork` 関数を削除する（`checkDepsSignals` に置き換え）

### 3-8. `verification.Manager` の変更

- [ ] `internal/verification/manager.go`: `GetAnalysisDeps` を `AnalysisDeps{RecordStore: m.fileValidator}` のみを返すよう変更する（`NetworkSymbolStore`、`SyscallStore`、`DynLibDepsStore`、`LibAnalysisStore`、`ShebangStore` を除去）
- [ ] `internal/verification/manager.go`: `Manager` から `networkSymbolStore`、`syscallAnalysisStore`、`dynLibDepsStore`、`dynlibAnalysisStore`、`shebangStore` フィールドを削除する
- [ ] `internal/verification/manager.go`: 各 store を初期化するブロック（`NewNetworkSymbolStore`、`NewSyscallAnalysisStore`、`NewDynLibDepsStore`、`dynamicanalysis.New`、`NewShebangInterpreterStore`）を削除する

### 3-9. runner のテスト

- [ ] `internal/runner/base/security/network_analyzer_test.go`: 以下のテストを追加・変更する:
  - `Record.Deps` 内の共有ライブラリエントリから network signal を検出する（F-003 AC2）
  - `Record.Deps` 内のインタープリターバイナリエントリ（`soname` なし）から network signal を検出する（F-003 AC5）
  - 非 wrapper dep の `SyscallAnalysis == nil` → `ErrDepAnalysisNotEmbedded` を返す（F-003 AC6）
  - VDSO dep の `SyscallAnalysis == nil` → エラーなし（F-003 AC6）
  - `record.ContentHash != contentHash` → 高リスク扱いになること
  - `DynLibDepsStore`、`LibAnalysisStore`、`ShebangStore` を使うテストを削除する
  - `ErrAnalysisNotFound` → 高リスクフォールバックのテストを削除する
  - `checkDynLibDepsNetwork`、`followShebangChain` を使うテストを削除する
- [ ] `internal/verification/manager_test.go`: `GetAnalysisDeps` 関連テストを新 `AnalysisDeps` 構造に合わせて更新する

---

## Phase 4: 検証・仕上げ

### 4-1. 統合テスト

- [ ] `cmd/record/main_test.go` または専用の統合テストファイル:
  - ELF バイナリの `record` 実行 → `deps` に解析結果が埋め込まれることを確認（F-001 AC1〜3）
  - 直接形式 shebang の `shebang_chain`（1エントリ）生成確認（F-002 AC2）
  - env 形式 shebang の `shebang_chain`（2エントリ）生成確認（F-002 AC3）
  - `dynlib-analysis/` ディレクトリを削除した状態で `runner` が正常動作することを確認（F-003 AC3）
  - v21 Record を読み込むと `SchemaVersionMismatchError` が返ることを確認（F-005 AC2）
  - `record` 再実行で旧 Record を新フォーマットで上書きできることを確認（F-005 AC3）

### 4-2. ビルド・テスト・Lint

- [ ] `make build` でビルドが通ることを確認する
- [ ] `make test` で全テストが通ることを確認する（`go test -tags test -v ./...`）
- [ ] `make lint` でリンターが通ることを確認する
- [ ] `make fmt` でフォーマットを適用する

### 4-3. 最終確認

- [ ] 要件定義書の全 Acceptance Criteria（F-001〜F-005）に対応するテストが存在することを確認する
- [ ] コード中に日本語が含まれていないことを確認する（`grep -rn '[^\x00-\x7F]' --include="*.go"` で検索）
- [ ] `dynamicanalysis.ErrAnalysisNotFound` が runner から参照されていないことを確認する
- [ ] `ShebangStore`、`DynLibDepsStore`、`LibAnalysisStore` が `AnalysisDeps` から除去されていることを確認する
