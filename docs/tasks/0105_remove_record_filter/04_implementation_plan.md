# 実装計画書: record コマンドのシステムコールフィルタリング削除

## 1. 実装の進め方

本実装計画書は 03_detailed_specification.md に基づき、record 側の責務整理、runner 側の判定修正、macOS syscall テーブル自動生成、既存テストの更新を段階的に進めるためのチェックリストを定義する。

### 実装ステップ概要

1. Step 1: record 側の syscall 保存経路からフィルタリングを除去する
2. Step 2: runner 側の SVC / network 判定を未解決 svc 前提に修正する
3. Step 3: macOS BSD syscall テーブルの自動生成を追加する
4. Step 4: 既存テストを新しい保存・判定前提に更新する
5. Step 5: 最終検証と関連ドキュメント整合を確認する

## 2. 事前確認事項

### 2.1 依存方向の確認

- [x] internal/fileanalysis は syscall 保存対象の選別を持たず、解析データ構造の責務に留める
- [x] internal/filevalidator は fileanalysis のデータ型を利用するが、runner の判定ロジックを持ち込まない
- [x] internal/runner/security は保存済み record を評価する責務に留まり、record 側の保存方針へ逆依存しない
- [x] internal/libccache の macOS syscall テーブル拡張は既存 API を維持し、呼び出し側の変更を不要にする

### 2.2 既存実装の確認

- [x] internal/fileanalysis/syscall_store.go に FilterSyscallsForStorage が存在し、削除対象が明確である
- [x] internal/filevalidator/validator.go に buildSyscallData と buildMachoSyscallData の 2 箇所の呼び出しが存在する
- [x] internal/runner/security/network_analyzer.go に syscallAnalysisHasSVCSignal と syscallAnalysisHasNetworkSignal の現行判定が存在する
- [x] scripts/generate_syscall_table.py と Makefile に Linux 用自動生成フローがあり、macOS 向け追加先が明確である

### 2.3 事前に完了済みのドキュメント整合

- [x] docs/tasks/0105_remove_record_filter/02_architecture.md と 03_detailed_specification.md のテストカバー観点を見直した
- [x] docs/tasks/0104_macho_syscall_number_analysis/03_detailed_specification.md の syscallAnalysisHasSVCSignal 削除前提を superseded として明示した
- [x] docs/tasks/0104_macho_syscall_number_analysis/04_implementation_plan.md の旧方針を 0105 の方針に合わせて注記した

## 3. Step 1: record 側の保存フィルタ削除

**対象ファイル:**
- internal/fileanalysis/syscall_store.go
- internal/fileanalysis/syscall_store_test.go
- internal/filevalidator/validator.go
- internal/filevalidator/validator_test.go
- internal/filevalidator/validator_macho_test.go

### 3.1 実装チェックリスト

- [x] FilterSyscallsForStorage を削除する
- [x] buildSyscallData で all をそのまま DetectedSyscalls に格納する
- [x] buildMachoSyscallData で merge 結果をそのまま DetectedSyscalls に格納する
- [x] buildMachoSyscallData の warnings 判定を merged に対して実行する
- [x] buildMachoSyscallData の warnings 条件を direct_svc_0x80 かつ Number == -1 に限定する
- [x] 関数コメントをフィルタなし前提へ更新する

### 3.2 テストチェックリスト

- [x] internal/fileanalysis/syscall_store_test.go の FilterSyscallsForStorage 前提テストを削除または置換する
- [x] internal/filevalidator/validator_test.go で非ネットワーク・解決済み syscall が DetectedSyscalls に残ることを確認する
- [x] internal/filevalidator/validator_macho_test.go で解決済み非ネットワーク svc が DetectedSyscalls に残ることを確認する
- [x] internal/filevalidator/validator_macho_test.go で libSystem と svc の全エントリが保持されることを確認する
- [x] internal/filevalidator/validator_macho_test.go で未解決 svc のみ AnalysisWarnings を発火させることを確認する

## 4. Step 2: runner 側の判定修正

**対象ファイル:**
- internal/runner/security/network_analyzer.go
- internal/runner/security/network_analyzer_test.go

### 4.1 実装チェックリスト

- [x] syscallAnalysisHasSVCSignal を削除せずに保持する
- [x] syscallAnalysisHasSVCSignal の高リスク条件を direct_svc_0x80 かつ Number == -1 のみに修正する
- [x] syscallAnalysisHasNetworkSignal の direct_svc_0x80 除外条件を削除する
- [x] syscallAnalysisHasNetworkSignal を IsNetwork のみで判定する実装に更新する
- [x] 関数コメントを未解決 svc と解決済ぽネットワーク svc の責務分担に合わせて更新する

### 4.2 テストチェックリスト

- [x] 未解決 svc を high risk と判定するテストを維持または追加する
- [x] 解決済み非ネットワーク svc を high risk と判定しないテストを追加する
- [x] 解決済みネットワーク svc を network signal として検出するテストを追加する
- [x] libSystem 由来ネットワーク syscall を network signal として検出する既存テストを維持する
- [x] 旧レコード相当のフィルタ済み DetectedSyscalls でも判定が変わらないことを確認する

## 5. Step 3: macOS BSD syscall テーブル自動生成

**対象ファイル:**
- internal/libccache/macos_syscall_table.go
- internal/libccache/macos_syscall_numbers.go
- scripts/generate_syscall_table.py
- Makefile
- internal/libccache 配下の関連テスト

### 5.1 実装チェックリスト

- [ ] macos_syscall_table.go から手動定義の macOSSyscallEntries を削除する
- [ ] macos_syscall_numbers.go を自動生成ファイルとして追加する
- [ ] macos_syscall_numbers.go をコミット対象に含め、macOS ヘッダーがない環境でも既存生成物を利用できる状態にする
- [ ] generate_syscall_table.py に MACOS_NETWORK_SYSCALL_NAMES を追加する
- [ ] generate_syscall_table.py に parse_macos_header を追加する
- [ ] generate_syscall_table.py に macOS 用生成関数を追加する
- [ ] generate_syscall_table.py に --macos-header オプションを追加する
- [ ] Makefile に MACOS_SYSCALL_HEADER を追加する
- [ ] generate-syscall-tables ターゲットで macOS ヘッダーがある場合のみ生成する
- [ ] gofumpt 対象に macos_syscall_numbers.go を追加する

### 5.2 テストチェックリスト

- [ ] MacOSSyscallTable.GetSyscallName(3) == read を確認する
- [ ] MacOSSyscallTable.IsNetworkSyscall(97) == true を確認する
- [ ] MacOSSyscallTable.IsNetworkSyscall(3) == false を確認する
- [ ] make generate-syscall-tables で macOS 環境の再生成が成立することを確認する
- [ ] macOS ヘッダー非存在環境でも既存ファイル利用でビルドが維持されることを確認する

## 6. Step 4: テスト更新と回帰確認

**対象ファイル:**
- internal/filevalidator/validator_test.go
- internal/filevalidator/validator_macho_test.go
- internal/runner/security/network_analyzer_test.go
- internal/fileanalysis/syscall_store_test.go
- internal/libccache 配下の関連テスト

### 6.1 テスト更新チェックリスト

- [ ] AC-1 を validator_test.go の保持テストでカバーする
- [ ] AC-2 と AC-3 を validator_macho_test.go の全件保持と warning 条件でカバーする
- [ ] AC-4 と AC-5 を network_analyzer_test.go の分岐テストでカバーする
- [ ] AC-6 を libccache テストでカバーする
- [ ] NFR-1 を network_analyzer_test.go の旧レコード互換ケースでカバーする
- [ ] 不要になった旧前提テスト名やコメントを更新する

## 7. Step 5: 最終検証

### 7.1 実行チェックリスト

- [ ] make fmt
- [ ] make build
- [ ] make test
- [ ] make lint
- [ ] 必要に応じて make generate-syscall-tables を実行し、生成物に差分がないことを確認する

### 7.2 ドキュメント整合チェックリスト

- [ ] docs/tasks/0105_remove_record_filter/01_requirements.md の AC-1 から AC-8 が実装・テスト・文書確認のいずれかに対応付いていることを確認する
- [ ] docs/tasks/0105_remove_record_filter/02_architecture.md と 03_detailed_specification.md の記述が最終実装と一致することを確認する
- [ ] docs/tasks/0104_macho_syscall_number_analysis 配下に 0105 と矛盾する記述が残っていないことを確認する

## 8. 完了条件

- [ ] record は全 syscall をフィルタなしで保存する
- [ ] runner は未解決 svc のみを high risk とし、解決済みネットワーク syscall を見逃さない
- [ ] macOS BSD syscall テーブルが自動生成に移行している
- [ ] 既存テストと追加テストで AC-1 から AC-8、および NFR-1 を確認できる
- [ ] make build が通過する
- [ ] make test と make lint が通過する
