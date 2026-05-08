# 実装計画書: デバッグ情報へのシンボル・syscall 呼び出し元帰属

## 受け入れ条件とタスクの対応表

| AC | 内容 | 対応タスク |
|---|---|---|
| AC-001 | `--debug-info` 時、各 `SyscallOccurrence.source_path` に呼び出し元パスが格納される | 3.4, 3.6 |
| AC-002 | `--debug-info` 非指定時、`source_path` は JSON に出力されない | 1.1, 3.6 |
| AC-003 | libc インポート経由 (`location=0`) の occurrence でも `source_path` が正しく設定される | 3.4, 3.6 |
| AC-004 | `--debug-info` 時、各 `DetectedSymbol.source_path` が格納される | 3.4, 3.5 |
| AC-005 | `--debug-info` 非指定時、`source_path` は JSON に出力されない | 1.2, 3.5 |
| AC-006 | `--debug-info` 時、同一シンボル名でも呼び出し元が異なれば別エントリ | 3.1, 3.2, 3.5 |
| AC-007 | `--debug-info` 非指定時、同一シンボル名は 1 エントリに集約（従来動作維持） | 3.2, 3.5 |
| AC-008 | `addRecord` は `SourcePath` 未設定の occurrence にのみ `sourcePath` をセット | 3.6 |
| AC-009 | `addRecord` は main / shebang_interpreter / dynlib の 3 種の role を区別する | 3.1, 3.4 |
| AC-010 | `addSymbolAnalysis` は debug 時 `(name, source_path)` 単位で重複除去、非 debug 時 `name` 単位 | 3.5 |
| AC-011 | `CurrentSchemaVersion` を 22 → 23 に更新 | 1.3 |
| AC-012 | v22 以下の Record ロード時は `SchemaVersionMismatchError` を返す | 1.3, 7.6 |
| AC-013 | v22 以下の Record に `record` 再実行で v23 形式に上書き再生成できる | 1.3（既存テストで自動カバー） |

---

## フェーズ 1: スキーマ変更 (FR-001, FR-002, FR-004)

### タスク 1.1: `SyscallOccurrence` への `SourcePath` 追加

対象: `internal/common/syscall_types.go`

- [x] `SyscallOccurrence` 構造体に `SourcePath string \`json:"source_path,omitempty"\`` フィールドを追加する

AC カバレッジ: AC-001, AC-002, AC-003

### タスク 1.2: `DetectedSymbol` 型の新設と `SymbolAnalysisData` の更新

対象: `internal/fileanalysis/schema.go`

- [x] `DetectedSymbol` 構造体を新設する
  ```go
  type DetectedSymbol struct {
      Name       string `json:"name"`
      SourcePath string `json:"source_path,omitempty"`
  }
  ```
- [x] `SymbolAnalysisData.DetectedSymbols` の型を `[]string` → `[]DetectedSymbol` に変更する
- [x] `SymbolAnalysisData.DynamicLoadSymbols` の型を `[]string` → `[]DetectedSymbol` に変更する

AC カバレッジ: AC-004, AC-005, AC-006, AC-007

### タスク 1.3: スキーマバージョン更新

対象: `internal/fileanalysis/schema.go`

- [x] `CurrentSchemaVersion` を `22` → `23` に変更する
- [x] バージョン履歴コメントに v23 の説明（source attribution 追加）を追記する

AC カバレッジ: AC-011, AC-012, AC-013

---

## フェーズ 2: `convertDetectedSymbols` の更新

### タスク 2.1: 戻り値型の変更

対象: `internal/filevalidator/validator.go`

- [x] `convertDetectedSymbols` の戻り値型を `[]string` → `[]fileanalysis.DetectedSymbol` に変更する
  - `SourcePath` は空文字列のまま（呼び出し元帰属は集約時に設定）
  - ソートは `Name` フィールドで行う（従来動作維持）
- [x] `analyzeRecordTarget` および `analyzeOneLibrary` の `record.SymbolAnalysis` 設定箇所がコンパイルエラーにならないことを確認する

---

## フェーズ 3: `analysisAggregate` の変更 (FR-003)

### タスク 3.1: 内部型の定義

対象: `internal/filevalidator/validator.go`

- [x] `detectedSymbolKey` 型を定義する
  ```go
  type detectedSymbolKey struct {
      name       string
      sourcePath string
  }
  ```
- [x] `sourceRole` 型および定数を定義する
  ```go
  type sourceRole string
  const (
      roleMain               sourceRole = "main"
      roleShebangInterpreter sourceRole = "shebang_interpreter"
      roleDynLib             sourceRole = "dynlib"
  )
  ```

AC カバレッジ: AC-009

### タスク 3.2: `analysisAggregate` 構造体の変更

対象: `internal/filevalidator/validator.go`

- [x] `includeDebugInfo bool` フィールドを追加する
- [x] `symbols map[string]struct{}` → `symbols map[detectedSymbolKey]struct{}` に変更する
- [x] `dynLoads map[string]struct{}` → `dynLoads map[detectedSymbolKey]struct{}` に変更する

AC カバレッジ: AC-006, AC-007

### タスク 3.3: `newAnalysisAggregate` の変更

対象: `internal/filevalidator/validator.go`

- [x] 引数に `includeDebugInfo bool` を追加する
- [x] 初期化時に `includeDebugInfo` を構造体フィールドに設定する

### タスク 3.4: `addRecord` / `addDynamicResult` のシグネチャ変更

対象: `internal/filevalidator/validator.go`

- [x] `addRecord(record *fileanalysis.Record, sourcePath string, role sourceRole)` に変更する
- [x] `addDynamicResult(result *dynamicanalysis.Result, sourcePath string, role sourceRole)` に変更する
- [x] 各メソッド内で `addSymbolAnalysis(data, sourcePath)` を呼び出す

AC カバレッジ: AC-001, AC-003, AC-004, AC-009

### タスク 3.5: `addSymbolAnalysis` の変更

対象: `internal/filevalidator/validator.go`

- [x] 引数に `sourcePath string` を追加する
- [x] dedup キーの構築ロジックを変更する
  - `includeDebugInfo = false` の場合: `detectedSymbolKey{name: sym.Name, sourcePath: ""}` を使用（name 単位 dedup）
  - `includeDebugInfo = true` の場合: `sym.SourcePath` が非空なら `sym.SourcePath` を使用、空なら引数の `sourcePath` を使用
- [x] `a.symbols` および `a.dynLoads` マップへの追加をキー型変更に合わせて更新する

AC カバレッジ: AC-004, AC-005, AC-006, AC-007, AC-010

### タスク 3.6: `SyscallOccurrence` への `SourcePath` スタンプ処理

対象: `internal/filevalidator/validator.go`

- [x] `addRecord` 内に以下の処理を `addSyscallAnalysis` の呼び出し前に追加する
  - `a.includeDebugInfo = true` かつ `record.SyscallAnalysis != nil` のときのみ実行
  - `record.SyscallAnalysis.DetectedSyscalls` の各 `SyscallOccurrence.SourcePath` が空文字列の場合のみ `sourcePath` をセット（AC-008）
- [x] `addDynamicResult` 内に同様の処理を追加する（`result.SyscallAnalysis` 対象）

AC カバレッジ: AC-001, AC-002, AC-003, AC-008

### タスク 3.7: `symbolAnalysis()` の変更

対象: `internal/filevalidator/validator.go`

- [x] 戻り値の `result.DetectedSymbols` を `[]fileanalysis.DetectedSymbol` として構築する
  - `detectedSymbolKey` の `{name, sourcePath}` から `DetectedSymbol{Name: key.name, SourcePath: key.sourcePath}` を生成
  - 非 debug 時は `key.sourcePath = ""` のため `omitempty` により JSON で省略される
- [x] ソート順を `Name` 昇順、同一の場合は `SourcePath` 昇順に変更する
- [x] 同様に `result.DynamicLoadSymbols` も `[]DetectedSymbol` として構築する

AC カバレッジ: AC-004, AC-005, AC-006, AC-007

---

## フェーズ 4: 呼び出し元の更新

### タスク 4.1: `populateAnalysisRecord` の更新

対象: `internal/filevalidator/validator.go`

- [x] `aggregate := newAnalysisAggregate()` → `newAnalysisAggregate(v.includeDebugInfo)` に変更する
- [x] `aggregate.addRecord(targetAnalysis)` → `aggregate.addRecord(targetAnalysis, filePath, roleMain)` に変更する

AC カバレッジ: AC-001, AC-004, AC-009

### タスク 4.2: `populateShebangData` の更新

対象: `internal/filevalidator/validator.go`

- [x] `aggregate.addRecord(chainAnalysis)` → `aggregate.addRecord(chainAnalysis, entry.Path, roleShebangInterpreter)` に変更する

AC カバレッジ: AC-001, AC-004, AC-009

### タスク 4.3: `analyzeLibraries` の更新

対象: `internal/filevalidator/validator.go`

- [x] `aggregate := newAnalysisAggregate()` → `newAnalysisAggregate(v.includeDebugInfo)` に変更する
- [x] `aggregate.addRecord(record)` → `aggregate.addRecord(record, record.FilePath, roleMain)` に変更する
  - record には既にソースパスが設定された occurrence / symbol が含まれるため「空のみ上書き」ロジックにより既存値は保護される
- [x] `aggregate.addDynamicResult(result)` → `aggregate.addDynamicResult(result, lib.Path, roleDynLib)` に変更する

AC カバレッジ: AC-001, AC-004, AC-008, AC-009

---

## フェーズ 5: `network_analyzer.go` の更新 (NFR-003, NFR-004)

### タスク 5.1: `convertNetworkSymbolEntries` の更新

対象: `internal/runner/base/security/network_analyzer.go`

- [x] 引数型を `[]string` → `[]fileanalysis.DetectedSymbol` に変更する
- [x] ループ内の `e` を `e.Name` に変更する（判定ロジック自体は変更しない）

NFR カバレッジ: NFR-003, NFR-004

### タスク 5.2: `buildAnalysisOutputFromSymbolData` の述語更新

対象: `internal/runner/base/security/network_analyzer.go`

- [x] `slices.ContainsFunc(data.DetectedSymbols, binaryanalyzer.IsNetworkSymbolName)` の述語を以下に変更する
  ```go
  func(s fileanalysis.DetectedSymbol) bool { return binaryanalyzer.IsNetworkSymbolName(s.Name) }
  ```

NFR カバレッジ: NFR-003, NFR-004

---

## フェーズ 6: 既存テストの更新

### タスク 6.1: `internal/fileanalysis/file_analysis_store_test.go`

- [x] `TestStore_Load_V21RejectedWithSchemaVersionMismatch` の `assert.Equal(t, 22, schemaErr.Expected)` を `assert.Equal(t, CurrentSchemaVersion, schemaErr.Expected)` に変更する（定数名参照に統一）

### タスク 6.2: `internal/filevalidator/validator_test.go`

- [x] `record.SymbolAnalysis.DetectedSymbols` の期待値を `[]string` から `[]fileanalysis.DetectedSymbol` に変更する（全箇所）
  - 例: `assert.Equal(t, "socket", record.SymbolAnalysis.DetectedSymbols[0])` → `assert.Equal(t, fileanalysis.DetectedSymbol{Name: "socket"}, record.SymbolAnalysis.DetectedSymbols[0])`
- [x] `record.SymbolAnalysis.DynamicLoadSymbols` の期待値を同様に変更する

### タスク 6.3: `internal/filevalidator/validator_library_analysis_test.go`

- [x] `assert.Contains(t, result.SymbolAnalysis.DetectedSymbols, "socket")` を `DetectedSymbol` 型で比較するよう変更する
- [x] `assert.Contains(t, record.SymbolAnalysis.DynamicLoadSymbols, "dlopen")` を更新する

### タスク 6.4: `internal/filevalidator/validator_sort_test.go`

- [x] `TestRecord_DetectedSymbols_SortedAlphabetically` の期待値を `[]fileanalysis.DetectedSymbol{{Name: "bind"}, {Name: "connect"}, {Name: "socket"}}` に変更する
- [x] `TestRecord_DynamicLoadSymbols_SortedAlphabetically` の期待値を同様に変更する

### タスク 6.5: `internal/runner/base/security/network_analyzer_test.go`

- [x] `DetectedSymbols: []string{...}` の箇所をすべて `[]fileanalysis.DetectedSymbol{{Name: "..."}}` に変更する（`nil` はそのまま）

### タスク 6.6: `internal/dynamicanalysis/store_test.go`

- [x] `DetectedSymbols: []string{"socket"}` を `[]fileanalysis.DetectedSymbol{{Name: "socket"}}` に変更する
- [x] `assert.Equal(t, []string{"socket"}, result2.SymbolAnalysis.DetectedSymbols)` を更新する

---

## フェーズ 7: 新規テストの追加

### タスク 7.1: AC-001 + AC-003 — `SyscallOccurrence.SourcePath` 設定

対象: `internal/filevalidator/validator_test.go`

- [ ] `TestRecord_SyscallOccurrence_SourcePathSetWhenDebugInfo` を追加する
  - `SetIncludeDebugInfo(true)` の Validator で SaveRecord
  - `SyscallOccurrence.SourcePath` がターゲットバイナリのパスと一致することを確認
  - `Source: "libc_symbol_import"` の occurrence（location=0）でも `SourcePath` が正しく設定されることを確認（AC-003）

### タスク 7.2: AC-002 — `SyscallOccurrence.SourcePath` 非 debug 時省略

対象: `internal/filevalidator/validator_test.go`

- [ ] `TestRecord_SyscallOccurrence_SourcePathOmittedWithoutDebugInfo` を追加する
  - `SetIncludeDebugInfo(false)` の Validator で SaveRecord
  - `SyscallAnalysis.DetectedSyscalls[*].Occurrences` が `nil` であることを確認（`stripOccurrences` 動作）

### タスク 7.3: AC-004 — `DetectedSymbol.SourcePath` debug 時設定

対象: `internal/filevalidator/validator_test.go`

- [ ] `TestRecord_DetectedSymbol_SourcePathSetWhenDebugInfo` を追加する
  - `SetIncludeDebugInfo(true)` の Validator で SaveRecord
  - `DetectedSymbols[i].SourcePath` がターゲットバイナリのパスと一致することを確認

### タスク 7.4: AC-005 — `DetectedSymbol.SourcePath` 非 debug 時省略

対象: `internal/filevalidator/validator_test.go`

- [ ] `TestRecord_DetectedSymbol_SourcePathOmittedWithoutDebugInfo` を追加する
  - `SetIncludeDebugInfo(false)` の Validator で SaveRecord
  - `DetectedSymbols[i].SourcePath` が空文字列であることを確認

### タスク 7.5: AC-006 — debug 時 `(name, source_path)` 単位 dedup

対象: `internal/filevalidator/validator_dedup_test.go`

- [ ] `TestAnalysisAggregate_DetectedSymbol_DedupByNameAndSourcePath_DebugMode` を追加する
  - `newAnalysisAggregate(true)` で aggregate を作成
  - 異なる `sourcePath` を持つ 2 つの record に同一シンボル名を設定し `addRecord` を 2 回呼び出す
  - `symbolAnalysis().DetectedSymbols` が 2 エントリであることを確認

### タスク 7.6: AC-007 — 非 debug 時 `name` 単位 dedup

対象: `internal/filevalidator/validator_dedup_test.go`

- [ ] `TestAnalysisAggregate_DetectedSymbol_DedupByNameOnly_NonDebugMode` を追加する
  - `newAnalysisAggregate(false)` で aggregate を作成
  - 同一シンボル名を異なるソースパスで 2 回 `addRecord` する
  - `symbolAnalysis().DetectedSymbols` が 1 エントリであることを確認

### タスク 7.7: AC-008 — `SourcePath` 設定済み occurrence は上書きしない

対象: `internal/filevalidator/validator_dedup_test.go`

- [ ] `TestAnalysisAggregate_AddRecord_SourcePathNotOverwrittenIfAlreadySet` を追加する
  - `SyscallOccurrence.SourcePath` を事前に設定した record を `addRecord(record, "other_path", roleMain)` で追加する
  - `syscallAnalysis()` の occurrence の `SourcePath` が元の値のままであることを確認

### タスク 7.8: AC-009 — 3 種のロールが区別される

対象: `internal/filevalidator/validator_dedup_test.go`

- [ ] `TestAnalysisAggregate_AllRolesDistinct` を追加する
  - `newAnalysisAggregate(true)` で aggregate を作成
  - `addRecord` で `roleMain` / `roleShebangInterpreter`、`addDynamicResult` で `roleDynLib` を指定
  - 各 occurrence の `SourcePath` が対応するソースパスに設定されていることを確認

### タスク 7.9: AC-012 — v22 レコードロード時の `SchemaVersionMismatchError`

対象: `internal/fileanalysis/file_analysis_store_test.go`

- [ ] `TestStore_Load_V22RejectedWithSchemaVersionMismatch` を追加する
  - `schema_version: 22` のレコードを直接書き込む
  - `store.Load` が `SchemaVersionMismatchError` を返し、`Expected = CurrentSchemaVersion (23)`、`Actual = 22` であることを確認

---

## テストカバレッジと非機能要件の確認

| 要件 | 対応方法 |
|---|---|
| NFR-001: 非 debug 時 JSON サイズ増加なし | `omitempty` タグによる省略 + タスク 7.2/7.4 のテストで確認 |
| NFR-002: debug 時追加データはソース attribution 文字列のみ | 設計通り `SourcePath` 文字列のみ追加、データ重複なし |
| NFR-003: セキュリティポリシー評価の動作変更なし | タスク 5.1/5.2 + 既存 network_analyzer テストで確認 |
| NFR-004: `DetectedSymbol` 型変更後もリスク判定維持 | タスク 5.2 で `.Name` 参照に更新 + 既存テストで確認 |

## 実施上の注意事項

1. フェーズ 1 のスキーマ変更後、コンパイルエラーが多数発生する。フェーズ 2→6 を順に修正してコンパイルを通す。
2. `analysisAggregate` は `filevalidator` パッケージ内部型のため、タスク 7.5〜7.8 の新規テストは `package filevalidator`（非公開アクセス可）で記述する。
3. 実装コード内にコメントを含め日本語を使用しない（要件 6.1 に対応）。
4. 各フェーズ完了後に `make fmt && make test && make lint` を実行してエラーがないことを確認する。
