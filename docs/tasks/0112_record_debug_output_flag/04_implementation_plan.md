# 実装計画書: record コマンドへのデバッグ情報出力フラグ追加

## 1. 目的

本計画書は、要件定義書の受け入れ条件を満たすための実装順序と確認観点を明確化し、進捗をチェックボックスで追跡できるようにする。

## 2. 対象ドキュメントと参照

- 要件定義書: `01_requirements.md`
- アーキテクチャ設計書: `02_architecture.md`
- 詳細仕様書: `03_detailed_specification.md`

## 3. 実装方針

- 解析ロジックは変更せず、保存時の出力制御のみ追加する。
- `record` から `filevalidator.Validator` へフラグをセッターで伝搬する。
- `Occurrences` と `DeterminationStats` は同一フラグで制御し、デフォルトでは非出力とする。
- 既存コードの再利用を優先し、新規処理は最小限のヘルパー関数に限定する。

## 4. 進捗管理チェックリスト

### 4.1 仕様同期タスク

- [x] P-01: 変更対象ファイル一覧を最終確定する
  - 完了条件: 実装対象とテスト対象が 01/02/03 で矛盾しない
- [x] P-02: 受け入れ条件 AC-1 から AC-9 のトレーサビリティ表を更新する
  - 完了条件: すべての AC に対して実装箇所とテスト名が紐づく

### 4.2 実装タスク（CLI 層）

- [x] I-01: `cmd/record/main.go` の `recordConfig` に `debugInfo` を追加する
  - 完了条件: 構造体でフラグ値を保持できる
- [x] I-02: `parseArgs` に `--debug-info` を追加する
  - 完了条件: ヘルプにフラグが表示され、引数解析で値を受け取れる
- [x] I-03: `run` で `SetIncludeDebugInfo` を呼び出す
  - 完了条件: `record` 実行時に Validator にフラグ値が伝搬する

### 4.3 実装タスク（Validator 層）

- [x] I-04: `SyscallAnalyzerInterface` の `AnalyzeSyscallsFromELF` 戻り値を拡張する
  - 完了条件: `*common.SyscallDeterminationStats` を受け渡しできる
- [x] I-05: `Validator` に `includeDebugInfo` フィールドと `SetIncludeDebugInfo` を追加する
  - 完了条件: 出力制御の状態を保持できる
- [x] I-06: `stripOccurrences` ヘルパーを追加する
  - 完了条件: `DetectedSyscalls` のコピーを作り `Occurrences` を除去できる
- [x] I-07: `buildSyscallData` を拡張する
  - 完了条件: `includeDebugInfo=false` で `Occurrences` と `DeterminationStats` が出力されない
  - 完了条件: `includeDebugInfo=true` で両方が保持される
- [x] I-08: `buildMachoSyscallData` を拡張する
  - 完了条件: `includeDebugInfo=false` で `Occurrences` が出力されない
  - 完了条件: 警告判定は `Occurrences` 除去前の情報で維持される
- [x] I-09: ELF 解析経路で `stats` を `buildSyscallData` へ連携する
  - 完了条件: direct syscall 解析時の `DeterminationStats` が debug 有効時に出力される
- [x] I-10: Mach-O 解析経路で `includeDebugInfo` を `buildMachoSyscallData` へ連携する
  - 完了条件: Mach-O でも同じ制御方針が適用される

### 4.4 実装タスク（Adapter 層）

- [x] I-11: `internal/libccache/adapters.go` の戻り値を拡張する
  - 完了条件: analyzer の `DeterminationStats` を validator へ pass-through できる
  - 完了条件: Unsupported Architecture 変換時のエラーハンドリングは維持される

### 4.5 テストタスク

- [x] T-01: `validator_test.go` の既存 stub を新シグネチャへ更新する
  - 完了条件: 既存テストがコンパイルエラーなく実行可能
- [x] T-02: `validator_test.go` に debug 情報付き stub を追加する
  - 完了条件: `Occurrences` と `DeterminationStats` を含む入力を再現できる
- [x] T-03: `TestRecord_DebugInfo_ELF` を追加する
  - 完了条件: `includeDebugInfo=false/true` の `Occurrences` 制御差分を検証できる
  - 対応 AC: AC-1, AC-3, AC-5, AC-7
- [x] T-04: `TestRecord_DebugInfo_DeterminationStats` を追加する
  - 完了条件: `includeDebugInfo=false/true` の `DeterminationStats` 制御差分を検証できる
  - 対応 AC: AC-2, AC-4, AC-5, AC-7
- [x] T-05: `TestBuildSyscallData_DebugInfo` を追加する
  - 完了条件: ヘルパー関数単体で出力差分を検証できる
  - 対応 AC: AC-1, AC-2, AC-3, AC-4
- [x] T-06: `validator_macho_test.go` に `TestBuildMachoSyscallData_DebugInfo` を追加する
  - 完了条件: Mach-O パスで `Occurrences` 制御差分を検証できる
  - 対応 AC: AC-6
- [x] T-07: 既存の `buildSyscallData` / `buildMachoSyscallData` 呼び出しテストを新シグネチャへ更新する
  - 完了条件: 既存テスト意図を維持しつつ全件パスする
- [x] T-08: `internal/libccache/adapters_test.go` を新シグネチャに追従させる
  - 完了条件: Adapter 層の回帰がないことを確認できる
- [x] T-09: `cmd/record` 側の引数解析テストを必要に応じて追加または更新する
  - 完了条件: `--debug-info` が正しく解釈されることを確認できる

### 4.6 検証タスク

- [x] V-01: `make fmt` を実行する
  - 完了条件: 差分がフォーマット規約に一致する
- [x] V-02: `make test` を実行する
  - 完了条件: 全テストが通過する
- [x] V-03: `make lint` を実行する
  - 完了条件: 新規 lint エラーがない
- [x] V-04: 受け入れ条件チェックを実施する
  - 完了条件: AC-1 から AC-9 まで満足を確認できる

### 4.7 レビュータスク（重点観点）

- [x] R-01: 文書間整合性レビューを実施する
  - 完了条件: 01/02/03/04 間で変更対象・テスト方針・AC 対応に矛盾がない
- [x] R-02: 非機能要件レビューを実施する
  - 完了条件: テストカバレッジ不足がない
- [x] R-03: テスト重複レビューを実施する
  - 完了条件: 無意味な重複テストや既存テストとの重複が除去される
- [x] R-04: 再利用性レビューを実施する
  - 完了条件: 既存関数を再利用せずに自前実装している箇所がない
- [x] R-05: 言語ポリシーレビューを実施する
  - 完了条件: 本タスクの Go コード上に日本語コメントや日本語文字列を追加していない

## 5. マイルストーン

- M1: CLI と Validator の実装完了（I-01 から I-10）
- M2: Adapter とテスト更新完了（I-11, T-01 から T-09）
- M3: 検証とレビュー完了（V-01 から V-04, R-01 から R-05）

## 6. 完了判定

以下をすべて満たした時点で本タスクを完了とする。

- 進捗管理チェックリストの未完了項目がない
- 受け入れ条件 AC-1 から AC-9 の証跡がテストまたは実行結果で示せる
- `make test` と `make lint` が成功する
- ドキュメント間整合性レビューで重大指摘が残っていない
