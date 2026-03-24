# 実装計画書: `IsHighRisk` 廃止・`HighRiskReasons` リネーム

## 進捗状況

- [ ] フェーズ 1: 型定義の変更
- [ ] フェーズ 2: 本体コードの変更
- [ ] フェーズ 3: テストコードの変更
- [ ] フェーズ 4: 受け入れ条件検証

---

## フェーズ 1: 型定義の変更

### タスク 1-1: `SyscallSummary.IsHighRisk` を削除

**ファイル**: `internal/common/syscall_types.go`

- [ ] `SyscallSummary` 構造体から `IsHighRisk bool` フィールドを削除する
- [ ] 対応する受け入れ条件: AC-1

### タスク 1-2: `HighRiskReasons` → `AnalysisWarnings` にリネーム

**ファイル**: `internal/common/syscall_types.go`

- [ ] `SyscallAnalysisResultCore` の `HighRiskReasons []string` フィールドを `AnalysisWarnings []string` にリネーム
- [ ] JSON タグを `json:"high_risk_reasons,omitempty"` → `json:"analysis_warnings,omitempty"` に変更
- [ ] フィールドのドキュメントコメントを更新
- [ ] 対応する受け入れ条件: AC-2

### タスク 1-3: スキーマバージョンを 5 → 6 に更新

**ファイル**: `internal/fileanalysis/schema.go`

- [ ] `CurrentSchemaVersion` を `5` から `6` に変更
- [ ] バージョン履歴コメントに Version 6 の説明を追加
- [ ] `Store.Update` のコメント行を更新: `(enables 'record --force' migration)` → `re-running 'record' migrates old-schema records automatically (--force not required)`
- [ ] 対応する受け入れ条件: AC-6

### フェーズ 1 完了確認

- [ ] `make build` でコンパイルエラーの場所を確認（このフェーズ完了後はエラーが出ることが正常）

---

## フェーズ 2: 本体コードの変更

### タスク 2-1: `filevalidator` での `IsHighRisk` 代入削除

**ファイル**: `internal/filevalidator/validator.go`

- [ ] `SyscallSummary` の初期化から `IsHighRisk: hasUnknown` の行を削除
- [ ] 対応する受け入れ条件: AC-3

### タスク 2-2: `syscall_analyzer.go` での変更

**ファイル**: `internal/runner/security/elfanalyzer/syscall_analyzer.go`

- [ ] `HighRiskReasons` → `AnalysisWarnings` にリネーム（4 箇所: L313, L339, L374, L378 付近）
- [ ] `result.Summary.IsHighRisk = true`（L368 付近）の行を削除
- [ ] `result.Summary.IsHighRisk = result.Summary.IsHighRisk || result.HasUnknownSyscalls`（L386 付近）の行を削除
- [ ] L355 付近のビルドサマリーコメントから `IsHighRisk` への言及を削除し、リスク導出は `convertSyscallResult` が行う旨を記述
- [ ] 対応する受け入れ条件: AC-4

### タスク 2-3: `standard_analyzer.go` での変更

**ファイル**: `internal/runner/security/elfanalyzer/standard_analyzer.go`

- [ ] `convertSyscallResult` のリスク判定を `result.Summary.IsHighRisk` から `result.HasUnknownSyscalls || EvalMprotectRisk(result.ArgEvalResults)` に置き換え
- [ ] `result.HighRiskReasons` → `result.AnalysisWarnings` に更新
- [ ] `convertSyscallResult` のドキュメントコメント（L347–352 付近）から `IsHighRisk` の説明行を削除し、新しい導出条件を記述
- [ ] 対応する受け入れ条件: AC-4, AC-5

### タスク 2-4: `mprotect_risk.go` のコメント更新

**ファイル**: `internal/runner/security/elfanalyzer/mprotect_risk.go`

- [ ] L6 のコメントを「`IsHighRisk` に設定すべきか」から「mprotect 由来のリスクが存在するか（`AnalysisWarnings` への追加判断および `convertSyscallResult` でのリスク判定に使用）」に更新
- [ ] 対応する受け入れ条件: AC-2（`AnalysisWarnings` リネームに伴うコードの実態合わせ）

### フェーズ 2 完了確認

- [ ] `make build` がエラーなく完了すること

---

## フェーズ 3: テストコードの変更

### タスク 3-1: `syscall_types_test.go`

**ファイル**: `internal/common/syscall_types_test.go`

- [ ] `TestSyscallSummary_JSONRoundTrip`（L87–101）から `IsHighRisk` フィールドの設定・アサーションを削除
- [ ] `TestSyscallAnalysisResultCore_JSONRoundTrip`（L106–135）から `IsHighRisk` フィールドの設定・アサーションを削除
- [ ] `HighRiskReasons` → `AnalysisWarnings`、JSON キー `high_risk_reasons` → `analysis_warnings` に更新
- [ ] 対応する受け入れ条件: AC-1, AC-2

### タスク 3-2: `validator_test.go`

**ファイル**: `internal/filevalidator/validator_test.go`

- [ ] `TestBuildSyscallAnalysisData` 内の `IsHighRisk` アサーション（L1314, L1326 付近）を削除
- [ ] `TestSaveRecord_PreservesSyscallAnalysis`（L855–895 付近）の `HighRiskReasons` → `AnalysisWarnings` にリネーム（フィールド設定・アサーション・エラーメッセージ文字列すべて）
- [ ] 対応する受け入れ条件: AC-2, AC-3

### タスク 3-3: `syscall_store_test.go`

**ファイル**: `internal/fileanalysis/syscall_store_test.go`

- [ ] 基本ラウンドトリップテスト（L28–64）から `IsHighRisk` フィールドの設定・アサーションを削除
- [ ] `TestSyscallAnalysisStore_HighRiskReasons` → `TestSyscallAnalysisStore_AnalysisWarnings` に関数名をリネーム（L146 付近）
- [ ] `AnalysisWarnings` ラウンドトリップテスト（L151–193）の `HighRiskReasons` → `AnalysisWarnings`、JSON キー更新、`IsHighRisk` フィールドの設定・`assert.True(t, loadedResult.Summary.IsHighRisk)` アサーションを削除
- [ ] ArgEvalResults ラウンドトリップテスト（L328–358）から `IsHighRisk` フィールドの設定・`assert.True(t, loaded.Summary.IsHighRisk)` アサーションを削除
- [ ] 対応する受け入れ条件: AC-6

### タスク 3-4: `syscall_analyzer_test.go`

**ファイル**: `internal/runner/security/elfanalyzer/syscall_analyzer_test.go`

- [ ] 未知 syscall 検出テスト（L155–170 付近）: `assert.True(t, result.Summary.IsHighRisk)` を削除（直前行で `HasUnknownSyscalls` 確認済み）。`HighRiskReasons` → `AnalysisWarnings`
- [ ] 複数 syscall 検出テスト（L240–270 付近）: `assert.False(t, result.Summary.IsHighRisk)` を削除
- [ ] syscall 未検出テスト（L254–270 付近）: `assert.False(t, result.Summary.IsHighRisk)` を削除
- [ ] ネットワーク+未知 syscall 混在テスト（L320–330 付近）: `assert.True(t, result.Summary.IsHighRisk)` を削除
- [ ] スキャンリミット超過テスト（L500–535 付近）: `assert.True(t, result.Summary.IsHighRisk)` を `assert.True(t, result.HasUnknownSyscalls)` に置き換え
- [ ] ウィンドウ枯渇テスト（L525–535 付近）: `assert.True(t, result.Summary.IsHighRisk)` を `assert.True(t, result.HasUnknownSyscalls)` に置き換え
- [ ] mprotect テスト群（L830–895 付近）: `assert.True/False(t, result.Summary.IsHighRisk)` を `EvalMprotectRisk(result.ArgEvalResults)` による確認に置き換え
- [ ] `exec_not_set does not overwrite pre-existing IsHighRisk=true` テスト（L871–890）: テスト名を `exec_not_set with HasUnknownSyscalls remains high risk` に変更し、`assert.True(t, result.Summary.IsHighRisk)` 行を削除
- [ ] ARM64 mprotect テーブル駆動テスト（L905–1010 付近）: `wantIsHighRisk bool` フィールドを `wantHighRisk bool` にリネームし、検証式を `result.HasUnknownSyscalls || EvalMprotectRisk(result.ArgEvalResults)` に変更
- [ ] `HighRiskReasons` → `AnalysisWarnings` に更新
- [ ] 対応する受け入れ条件: AC-4

### タスク 3-5: `analyzer_test.go`

**ファイル**: `internal/runner/security/elfanalyzer/analyzer_test.go`

- [ ] `TestStandardELFAnalyzer_SyscallLookup_HighRisk`（L355–389）: `Summary.IsHighRisk: true` 削除、`HighRiskReasons` → `AnalysisWarnings`
- [ ] `TestStandardELFAnalyzer_SyscallLookup_HighRiskTakesPrecedenceOverNetwork`（L391–436）: 同上。コメント `// IsHighRisk must win` → `// Risk must win`、`// IsHighRisk must take precedence` → `// Risk must take precedence`
- [ ] `TestAC3_DynamicELF_SyscallFallback_HighRisk`（L580–610 付近）: `Summary.IsHighRisk: true` 削除、`HighRiskReasons` → `AnalysisWarnings`、関数コメント内の `SyscallAnalysis returns IsHighRisk=true` → `SyscallAnalysis returns AnalysisError`
- [ ] その他の全モックデータ: `grep -n 'IsHighRisk\|HighRiskReasons' analyzer_test.go` で残存箇所を確認し、全削除・リネーム
- [ ] 対応する受け入れ条件: AC-5

### タスク 3-6: `file_analysis_store_test.go`

**ファイル**: `internal/fileanalysis/file_analysis_store_test.go`

- [ ] L143 付近の `HighRiskReasons` フィールドを `AnalysisWarnings` に更新
- [ ] 対応する受け入れ条件: AC-2

### タスク 3-7: `syscall_analyzer_integration_test.go`

**ファイル**: `internal/runner/security/elfanalyzer/syscall_analyzer_integration_test.go`

- [ ] L398 付近の `result.Summary.IsHighRisk` 参照を `HasUnknownSyscalls` への置き換えまたは削除
- [ ] 対応する受け入れ条件: AC-4

### フェーズ 3 完了確認

- [ ] `make test` がすべてパスすること

---

## フェーズ 4: 受け入れ条件検証

### タスク 4-1: AC-2 最終確認

- [ ] `grep -r HighRiskReasons --include='*.go' .` でヒットなしを確認
- [ ] `grep -r HighRiskReasons docs/development docs/user` でヒットなしを確認（該当箇所があれば更新）

### タスク 4-2: AC-1 最終確認

- [ ] `internal/common/syscall_types.go` の `SyscallSummary` 構造体に `IsHighRisk` フィールドが存在しないことを目視確認

### タスク 4-3: AC-6 最終確認

- [ ] `TestStore_SchemaVersionMismatch` が引き続きパスすること（旧バージョンの JSON ロード時に `SchemaVersionMismatchError` が返されること）
- [ ] `internal/fileanalysis/network_symbol_store_test.go` のスキーマ不一致伝播テストがパスすること（スキーマバージョンはファイル全体に適用されるため）
- [ ] `go test -tags test -v ./internal/fileanalysis/` でパッケージ全体が通過すること

### タスク 4-4: 全品質チェック

- [ ] `make build` がエラーなく完了すること（AC-1）
- [ ] `make test` がすべてパスすること（AC-7）
- [ ] `make lint` がエラーなく完了すること（AC-7）

---

## 変更ファイル一覧

### 本体ファイル

| ファイル | 変更内容 |
|----------|----------|
| `internal/common/syscall_types.go` | `SyscallSummary.IsHighRisk` 削除。`HighRiskReasons` → `AnalysisWarnings` リネーム（JSON タグも変更） |
| `internal/filevalidator/validator.go` | `IsHighRisk` への代入を削除 |
| `internal/runner/security/elfanalyzer/syscall_analyzer.go` | `IsHighRisk` 代入を全削除。`HighRiskReasons` → `AnalysisWarnings`。コメント更新 |
| `internal/runner/security/elfanalyzer/standard_analyzer.go` | リスク判定を `HasUnknownSyscalls || EvalMprotectRisk` に置き換え。`HighRiskReasons` → `AnalysisWarnings`。コメント更新 |
| `internal/runner/security/elfanalyzer/mprotect_risk.go` | `EvalMprotectRisk` 関数のコメント更新 |
| `internal/fileanalysis/schema.go` | `CurrentSchemaVersion` を 5 → 6 に更新 |

### テストファイル

| ファイル | 変更内容 |
|----------|----------|
| `internal/common/syscall_types_test.go` | `IsHighRisk` の設定・アサーション削除。`HighRiskReasons` → `AnalysisWarnings` |
| `internal/filevalidator/validator_test.go` | `IsHighRisk` アサーション削除。`HighRiskReasons` → `AnalysisWarnings` |
| `internal/fileanalysis/syscall_store_test.go` | `IsHighRisk` 削除。`HighRiskReasons` → `AnalysisWarnings`、JSON キー更新 |
| `internal/runner/security/elfanalyzer/syscall_analyzer_test.go` | `IsHighRisk` 参照を等価な確認に置き換え。`HighRiskReasons` → `AnalysisWarnings` |
| `internal/runner/security/elfanalyzer/analyzer_test.go` | モックデータの `IsHighRisk` 設定削除。キャッシュ経路テスト更新。`HighRiskReasons` → `AnalysisWarnings` |
| `internal/fileanalysis/file_analysis_store_test.go` | `HighRiskReasons` → `AnalysisWarnings` |
| `internal/runner/security/elfanalyzer/syscall_analyzer_integration_test.go` | `IsHighRisk` 参照を削除または置き換え |
