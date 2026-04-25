# 実装計画書: detected_syscalls のシステムコール番号単位グループ化

## 進捗サマリー

| フェーズ | 内容 | 状態 |
|---------|------|------|
| 1 | 型定義の変更（FR-1, FR-2） | [x] 完了 |
| 2 | elfanalyzer の修正（FR-6） | [x] 完了 |
| 3 | machoanalyzer の修正 | [x] 完了 |
| 4 | validator.go の修正（FR-4） | [x] 完了 |
| 5 | network_analyzer.go の修正（FR-5） | [x] 完了 |
| 6 | syscall_store.go の修正（FR-3） | [x] 完了 |
| 7 | スキーマバージョンの更新（FR-7） | [x] 完了 |
| 8 | テストの更新（AC-7） | [x] 完了 |

## 実装ステップ

### フェーズ 1: 型定義の変更

FR-1, FR-2 に対応する。他フェーズのすべてが本フェーズに依存するため最初に実施する。
FR-2 適用後はコンパイルエラーが多数発生するため、フェーズ 2〜6 を続けて実施してコンパイルを通す。

- [ ] **1-1**: `internal/common/syscall_types.go` に `SyscallOccurrence` 型を追加する（FR-1）
- [ ] **1-2**: `internal/common/syscall_types.go` の `SyscallInfo` から `Location`・`DeterminationMethod`・`Source` を削除し、`Occurrences []SyscallOccurrence` を追加する（FR-2）

### フェーズ 2: elfanalyzer の修正

FR-6 に対応する。`SyscallInfo` の top-level フィールド削除により発生するコンパイルエラーを解消する。

- [ ] **2-1**: `syscall_analyzer.go` の `extractSyscallInfo` が生成する `SyscallInfo` を修正する。`Location`・`DeterminationMethod` を `Occurrences[0]` へ移動する
- [ ] **2-2**: `syscall_analyzer.go` の Pass 2（Go wrapper call）が生成する `SyscallInfo` を修正する。`Location`・`DeterminationMethod` を `Occurrences[0]` へ移動する
- [ ] **2-3**: `syscall_analyzer.go` の `evaluateMprotectFamilyArgs` を修正する。`info.DeterminationMethod` を `info.Occurrences[0].DeterminationMethod` へ、`entry.Location` を `entry.Occurrences[0].Location` へ変更する（FR-6.1）
- [ ] **2-4**: `syscall_analyzer.go` の `evalSingleMprotect` を修正する。`entry.Location` を `entry.Occurrences[0].Location` へ変更する（FR-6.2）
- [ ] **2-5**: `plt_analyzer.go` の `EvaluatePLTCallArgs` を修正する。synthetic `SyscallInfo{Location: inst.Offset}` を `Occurrences[0]` に `Location` を格納する形式へ変更する（FR-6.3）

### フェーズ 3: machoanalyzer の修正

`SyscallInfo` の top-level フィールド削除により発生するコンパイルエラーを解消する。

- [ ] **3-1**: `pass1_scanner.go` の `scanSVCWithX16`（`unknownSyscallInfo` 含む）が生成する `SyscallInfo` を修正する。`Location`・`DeterminationMethod` を `Occurrences[0]` へ移動する
- [ ] **3-2**: `pass2_scanner.go` の `scanGoWrapperCalls` が生成する `SyscallInfo` を修正する。`Location`・`DeterminationMethod` を `Occurrences[0]` へ移動する
- [ ] **3-3**: `svc_scanner.go` の `analyzeArm64Slice` を修正する。`info.DeterminationMethod = ...`・`info.Source = ...` の代入を `Occurrences[0]` への設定へ変更する

### フェーズ 4: validator.go の修正

FR-4 に対応する。

- [ ] **4-1**: `buildSVCInfos` を修正する。`SyscallInfo{Location: addr, DeterminationMethod: ..., Source: ...}` を `Occurrences[0]` に出現情報を格納する形式へ変更する（FR-4.3）
- [ ] **4-2**: `mergeMachoSyscallInfos` を修正する。同一 `Number` の複数エントリを `Occurrences` へマージし、グループエントリを `Number` 昇順（`-1` は末尾）でソートし、各グループ内の `Occurrences` を `Location` 昇順でソートする（FR-4.2, FR-3, AC-1, AC-2）
- [ ] **4-3**: `buildMachoSyscallData` の `AnalysisWarnings` 生成ロジックを修正する。`s.DeterminationMethod` の直接参照を `Occurrences` 走査へ変更する（FR-4.1）
- [ ] **4-4**: `mergeSyscallInfos`（ELF パス）を修正する。同一 `Number` の複数エントリを `Occurrences` へマージし、グループエントリを `Number` 昇順（`-1` は末尾）でソートし、各グループ内の `Occurrences` を `Location` 昇順でソートする（FR-4.4, FR-3, AC-1, AC-2）

### フェーズ 5: network_analyzer.go の修正

FR-5 に対応する。

- [ ] **5-1**: `syscallAnalysisHasSVCSignal` を修正する。`s.DeterminationMethod` の直接参照を `Occurrences` 走査へ変更する（FR-5）

### フェーズ 6: syscall_store.go の修正

FR-3 に対応する。統合テストパスのグループ化ゲート。

- [ ] **6-1**: `SaveSyscallAnalysis` を修正する。現行のソート処理を、番号単位グループ化（`Occurrences` のマージ）＋ `Number` 昇順ソート（`-1` は末尾）＋ `Occurrences` 内 `Location` 昇順ソートへ変更する（FR-3）

### フェーズ 7: スキーマバージョンの更新

FR-7 に対応する。

- [ ] **7-1**: `internal/fileanalysis/schema.go` の `CurrentSchemaVersion` を 16 から 17 に更新する（FR-7）
- [ ] **7-2**: バージョン履歴コメントに v17 の説明を追記する（FR-7）
- [ ] **7-3**: v16 形式レコードのロードが `SchemaVersionMismatchError` を返す既存テストを確認し、必要に応じて期待値を schema version 17 前提へ更新する（AC-6）

### フェーズ 8: テストの更新

AC-7 に対応する。各フェーズの実装完了後、テストをまとめて更新する。

- [ ] **8-1**: `internal/common/syscall_types_test.go`：`SyscallOccurrence` の構造テストを追加し、`SyscallInfo` の構造テストを v17 形式へ更新する（AC-7）
- [ ] **8-2**: `internal/runner/security/elfanalyzer/` 配下のテスト：`SyscallInfo` 生成箇所を `Occurrences[0]` を持つ形式へ更新する（AC-7）
- [ ] **8-3**: `internal/runner/security/machoanalyzer/` 配下のテスト：`SyscallInfo` 生成箇所を `Occurrences[0]` を持つ形式へ更新する（AC-7）
- [ ] **8-4**: `internal/filevalidator/validator_macho_test.go`：`buildMachoSyscallData` のテストを `Occurrences` 参照前提へ更新する（AC-7）
- [ ] **8-5**: `internal/runner/security/network_analyzer_test.go`：`syscallAnalysisHasSVCSignal` のテストを `Occurrences` を持つ構造前提へ更新する（AC-7）
- [ ] **8-6**: `internal/fileanalysis/syscall_store_test.go`：グループ化ロジックのテストを追加する（同一番号の複数エントリが 1 グループに集約されること）（AC-7）
- [ ] **8-7**: `make test` を実行してすべてのテストがパスすることを確認する（AC-6）
- [ ] **8-8**: `make lint` を実行してエラーがないことを確認する（AC-6）

## 受け入れ基準との対応

| AC | 対応ステップ |
|----|------------|
| AC-1: JSON 構造のグループ化 | 6-1（SaveSyscallAnalysis）、4-2（mergeMachoSyscallInfos）、4-4（mergeSyscallInfos） |
| AC-2: ソート順の維持 | 6-1、4-2（mergeMachoSyscallInfos）、4-4（mergeSyscallInfos） |
| AC-3: AnalysisWarnings の正確な発出（Mach-O） | 4-3（buildMachoSyscallData） |
| AC-4: runner の高リスク判定（未解決 svc） | 5-1（syscallAnalysisHasSVCSignal） |
| AC-5: runner のネットワーク判定 | 変更不要（IsNetwork フィールドはグループエントリに残る） |
| AC-6: スキーマバージョン 17 の強制 | 7-1、7-2、7-3、8-7、8-8 |
| AC-7: 既存テストの更新 | 8-1〜8-6 |

## コミット方針

| コミット | 対象フェーズ | 内容 |
|---------|------------|------|
| 1 | フェーズ 1〜5 | 型定義変更とコンパイルエラー解消をまとめて実施。フェーズ 1 で `SyscallInfo` の top-level フィールドが削除されてコンパイルが壊れるため、フェーズ 5 まで続けて完了させてからコミットする |
| 2 | フェーズ 6 | `SaveSyscallAnalysis` のグループ化ロジック |
| 3 | フェーズ 7 | スキーマバージョン更新 |
| 4 | フェーズ 8 | テストの更新 |

## 注意事項

- `mergeSyscallInfos`（4-4）は ELF パスの本番コードに影響するため、変更後に ELF バイナリを対象とした統合テストを重点的に確認すること
- `mergeMachoSyscallInfos`（4-2）は Mach-O パスの本番コードに影響するため、同一番号の複数 syscall を含む Mach-O バイナリでグループ化と並び順を重点的に確認すること
- スキーマバージョン更新（フェーズ 7）は最後に行い、既存 v16 レコードが `SchemaVersionMismatchError` で拒否されることを確認すること
