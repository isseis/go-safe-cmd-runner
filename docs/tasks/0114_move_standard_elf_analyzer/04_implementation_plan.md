# 実装計画書: StandardELFAnalyzer の internal/security/elfanalyzer への移動

## 1. 目的

- `StandardELFAnalyzer` を `internal/security/elfanalyzer` へ移動し、`cmd/record` から
  `internal/runner/security/elfanalyzer` への推移的依存を除去する。
- execute-only バイナリの特権オープン挙動を維持するため、runner 側に実装を残しつつ、
  依存をインターフェースで分離する。

## 2. 実装タスク（進捗管理）

### 2.1 事前確認

- [ ] `go list -deps ./cmd/record | grep internal/runner/security/elfanalyzer` の現状値を記録
- [ ] `go list -deps ./cmd/verify | grep internal/runner/security` の現状値を記録
- [ ] `internal/runner/security/elfanalyzer/testdata` と
      `internal/security/elfanalyzer/testdata` の差分を確認（特に `README.md`）

### 2.2 コア実装の移動

- [ ] `internal/security/elfanalyzer/privileged_opener.go` を新規作成
- [ ] `internal/runner/security/elfanalyzer/standard_analyzer.go` を
      `internal/security/elfanalyzer/standard_analyzer.go` へ移動
- [ ] `StandardELFAnalyzer` の依存を `PrivilegeManager` から
      `PrivilegedFileOpener` へ置換
- [ ] `AnalyzeNetworkSymbols` の権限フォールバックを
      `opener.OpenWithPrivileges(path)` 呼び出しに置換
- [ ] `binaryanalyzer.BinaryAnalyzer` のコンパイル時チェックを移動先へ配置

### 2.3 runner 側アダプタの実装

- [ ] `internal/runner/security/elfanalyzer/privileged_opener_impl.go` を新規作成
- [ ] `NewPrivilegedFileOpener(fs, privManager)` を実装
- [ ] `OpenWithPrivileges(path)` が `PrivilegedFileValidator` を正しく委譲することを確認

### 2.4 呼び出し側の切り替え

- [ ] `internal/runner/security/binary_analyzer.go` の import を
      `internal/security/elfanalyzer` に変更
- [ ] `NewStandardELFAnalyzer(nil, nil)` の呼び出し先を移動先へ切り替え
- [ ] `internal/runner/security/syscall_store_adapter.go` のコメント参照を更新

### 2.5 不要ファイルの整理

- [ ] `internal/runner/security/elfanalyzer/analyzer.go` を削除
- [ ] `internal/runner/security/elfanalyzer/standard_analyzer.go` を削除
- [ ] `internal/runner/security/elfanalyzer/testdata/` を削除

### 2.6 テスト移行

- [ ] `analyzer_test.go` を `internal/security/elfanalyzer/` へ移動
- [ ] `analyzer_benchmark_test.go` を `internal/security/elfanalyzer/` へ移動
- [ ] `standard_analyzer_fallback_test.go` を `internal/security/elfanalyzer/` へ移動
- [ ] `analyzer_test.go` から `secelfanalyzer` import を削除し、
      型参照を自パッケージ参照へ置換

### 2.7 受け入れ基準の検証

- [ ] AC-1: `go list -deps ./cmd/record | grep internal/runner/security/elfanalyzer` が 0 件
- [ ] AC-2: `go list -deps ./cmd/verify | grep internal/runner/security` が 0 件
- [ ] AC-3: `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功
- [ ] AC-4: `make test` が成功
- [ ] AC-5: `cmd/runner` の統合テスト（`integration_cmd_allowed_security_test.go` など）が成功

## 3. 品質レビュー観点（本タスク重点）

### 3.1 文書間整合性

- [ ] `01_requirements.md` / `02_architecture.md` / `03_detailed_specification.md` /
      本計画書の用語・ファイルパス・責務分割が一致
- [ ] AC と検証コマンドの対応が 1 対 1 で追跡可能

### 3.2 非機能要件・テスト妥当性

- [ ] 既存テストと重複した無意味テストが追加されていない
- [ ] 移動で失われるテスト観点がない
- [ ] 失敗時の安全側挙動（`AnalysisError` への倒し込み）が維持される

### 3.3 実装品質

- [ ] 既存の再利用可能関数（`PrivilegedFileValidator`、既存型）を再利用し、
      不要な再実装をしていない
- [ ] 新規・変更コードのコメントは英語で記述されている
- [ ] 対象コードに日本語文字が混入していない（`[ぁ-んァ-ン一-龯]` 検索）

## 4. 実行順序（推奨）

1. 事前確認（2.1）
2. コア実装の移動（2.2）
3. runner 側アダプタ実装（2.3）
4. 呼び出し側切り替え（2.4）
5. 不要ファイル整理（2.5）
6. テスト移行（2.6）
7. 受け入れ基準検証（2.7）
8. 品質レビュー観点（3.x）の最終確認

## 5. 完了条件

- 2.1〜2.7 と 3.1〜3.3 のチェックボックスがすべて完了していること。
- AC-1〜AC-5 を満たす検証ログを提示できること。
