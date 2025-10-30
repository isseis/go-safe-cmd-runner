# 実装計画書: 環境変数継承モードの明示的追跡

## 1. 概要

### 1.1 文書の目的
この文書は、環境変数allowlistの継承モード（`InheritanceMode`）を明示的に追跡する機能の実装計画を定義し、進捗を管理するための実装チェックリストを提供する。

### 1.2 対象読者
- 実装担当者
- プロジェクトマネージャー
- レビュアー

### 1.3 関連文書
- [01_requirements.ja.md](./01_requirements.ja.md) - 要件定義書
- [02_architecture.ja.md](./02_architecture.ja.md) - アーキテクチャ設計書
- [03_detailed_design.ja.md](./03_detailed_design.ja.md) - 詳細設計書

## 2. 実装スケジュール

### 2.1 全体スケジュール

| フェーズ | 推定時間 | 依存関係 |
|----------|----------|----------|
| フェーズ1: ヘルパー関数実装 | 1時間 | なし |
| フェーズ2: RuntimeGroup拡張 | 30分 | フェーズ1 |
| フェーズ3: Validator更新 | 1時間 | フェーズ1 |
| フェーズ4: Expansion更新 | 1時間 | フェーズ1, フェーズ2 |
| フェーズ5: Debug更新 | 1.5時間 | フェーズ2, フェーズ4 |
| フェーズ6: 統合テスト | 1時間 | すべて |
| **合計** | **6時間** | - |

### 2.2 マイルストーン

- **M1**: フェーズ1完了 - 継承モード判定ロジック確立
- **M2**: フェーズ2完了 - データ構造の拡張完了
- **M3**: フェーズ3-5完了 - すべてのモジュールで継承モード使用
- **M4**: フェーズ6完了 - 統合テスト完了、リリース準備完了

## 3. 実装タスクリスト

### 3.1 フェーズ1: ヘルパー関数実装

**目標**: 継承モード判定ロジックを単一の関数に実装し、完全にテストする

**推定時間**: 1時間

#### 3.1.1 ファイル作成

- [x] `internal/runner/runnertypes/env_inheritance.go` ファイルを作成
- [x] パッケージ宣言とインポートを追加

#### 3.1.2 関数実装

- [x] `DetermineEnvAllowlistInheritanceMode(envAllowed []string) InheritanceMode` 関数を実装
  - [x] nilチェック処理を実装 → `InheritanceModeInherit`
  - [x] 空スライスチェック処理を実装 → `InheritanceModeReject`
  - [x] デフォルトケース（長さ>0）を実装 → `InheritanceModeExplicit`
- [x] 完全なgodocコメントを追加
  - [x] 関数の目的を記述
  - [x] 判定ルールを記述（3つのケース）
  - [x] Go言語のnil/空スライスの仕様を記述
  - [x] パラメータを記述
  - [x] 戻り値を記述
  - [x] 使用例を記述

#### 3.1.3 テスト実装

- [x] `internal/runner/runnertypes/env_inheritance_test.go` ファイルを作成
- [x] テーブル駆動テストを実装
  - [x] nilスライスのテストケース
  - [x] 空スライスのテストケース
  - [x] 1要素スライスのテストケース
  - [x] 複数要素スライスのテストケース
  - [x] 多数要素スライスのテストケース
- [x] 個別テスト関数を実装（オプション）
  - [x] `TestDetermineEnvAllowlistInheritanceMode_Inherit_NilSlice`
  - [x] `TestDetermineEnvAllowlistInheritanceMode_Reject_EmptySlice`
  - [x] `TestDetermineEnvAllowlistInheritanceMode_Explicit_SingleElement`
  - [x] `TestDetermineEnvAllowlistInheritanceMode_Explicit_MultipleElements`

#### 3.1.4 検証

- [x] テストを実行: `go test ./internal/runner/runnertypes/...`
- [x] すべてのテストが成功することを確認
- [x] カバレッジを確認: `go test -cover ./internal/runner/runnertypes/...`
- [x] カバレッジ100%を確認
- [x] lintを実行: `golangci-lint run ./internal/runner/runnertypes/...`
- [x] lint警告がないことを確認

#### 3.1.5 コミット

- [x] 変更をステージング: `git add internal/runner/runnertypes/env_inheritance*.go`
- [x] コミット: `git commit -m "feat: add DetermineEnvAllowlistInheritanceMode helper"`

---

### 3.2 フェーズ2: RuntimeGroup拡張

**目標**: RuntimeGroup構造体に継承モードフィールドを追加

**推定時間**: 30分

#### 3.2.1 フィールド追加

- [x] `internal/runner/runnertypes/runtime.go` を開く
- [x] `RuntimeGroup` 構造体に `EnvAllowlistInheritanceMode InheritanceMode` フィールドを追加
- [x] 完全なgodocコメントを追加
  - [x] フィールドの目的を記述
  - [x] ライフサイクルを記述（作成時→設定時→使用時）
  - [x] 値の意味を記述（3つのモード）
  - [x] 使用箇所を記述
  - [x] 不変条件を記述

#### 3.2.2 検証

- [x] ビルドを実行: `go build ./...`
- [x] エラーがないことを確認
- [x] 既存テストを実行: `go test ./internal/runner/runnertypes/...`
- [x] すべてのテストが成功することを確認

#### 3.2.3 コミット

- [ ] 変更をステージング: `git add internal/runner/runnertypes/runtime.go`
- [ ] コミット: `git commit -m "feat: add EnvAllowlistInheritanceMode field to RuntimeGroup"`

---

### 3.3 フェーズ3: Validator更新

**目標**: Validatorで継承モード判定ロジックを使用するよう変更

**推定時間**: 1時間

#### 3.3.1 コード変更

- [ ] `internal/runner/config/validator.go` を開く
- [ ] `analyzeInheritanceMode` 関数を特定
- [ ] 既存の判定ロジック（if-else）を削除
- [ ] `DetermineEnvAllowlistInheritanceMode` を呼び出すコードを追加
- [ ] switch文を実装
  - [ ] `InheritanceModeInherit` ケースを実装（既存ロジックを移行）
  - [ ] `InheritanceModeReject` ケースを実装（既存ロジックを移行）
  - [ ] `InheritanceModeExplicit` ケースを実装（コメントのみ）

#### 3.3.2 テスト確認

- [ ] 既存テストを実行: `go test ./internal/runner/config/... -run TestValidator`
- [ ] すべてのテストが成功することを確認
- [ ] 必要に応じて新規テストを追加
  - [ ] `TestValidator_AnalyzeInheritanceMode_Inherit_EmptyGlobal`
  - [ ] `TestValidator_AnalyzeInheritanceMode_Reject_CommandsUseEnv`
  - [ ] `TestValidator_AnalyzeInheritanceMode_Explicit_NoWarning`

#### 3.3.3 検証

- [ ] テストを実行: `go test ./internal/runner/config/...`
- [ ] すべてのテストが成功することを確認
- [ ] lintを実行: `golangci-lint run ./internal/runner/config/...`
- [ ] lint警告がないことを確認

#### 3.3.4 コミット

- [ ] 変更をステージング: `git add internal/runner/config/validator*.go`
- [ ] コミット: `git commit -m "refactor: use DetermineEnvAllowlistInheritanceMode in Validator"`

---

### 3.4 フェーズ4: Expansion更新

**目標**: Expanderで継承モードを設定するよう変更

**推定時間**: 1時間

#### 3.4.1 コード変更

- [ ] `internal/runner/config/expansion.go` を開く
- [ ] `ExpandGroup` 関数を特定
- [ ] `RuntimeGroup` 作成箇所を特定
- [ ] `RuntimeGroup` 作成直後に継承モード設定コードを追加
  ```go
  runtimeGroup.EnvAllowlistInheritanceMode =
      runnertypes.DetermineEnvAllowlistInheritanceMode(group.EnvAllowed)
  ```

#### 3.4.2 テスト実装

- [ ] `internal/runner/config/expansion_test.go` にテストを追加
  - [ ] `TestExpander_ExpandGroup_SetsEnvAllowlistInheritanceMode_Inherit`
  - [ ] `TestExpander_ExpandGroup_SetsEnvAllowlistInheritanceMode_Reject`
  - [ ] `TestExpander_ExpandGroup_SetsEnvAllowlistInheritanceMode_Explicit`

#### 3.4.3 検証

- [ ] テストを実行: `go test ./internal/runner/config/... -run TestExpander`
- [ ] 新規テストが成功することを確認
- [ ] 既存テストが成功することを確認
- [ ] カバレッジを確認

#### 3.4.4 コミット

- [ ] 変更をステージング: `git add internal/runner/config/expansion*.go`
- [ ] コミット: `git commit -m "feat: set EnvAllowlistInheritanceMode in Expander"`

---

### 3.5 フェーズ5: Debug更新

**目標**: デバッグ出力で継承モードフィールドを使用し、Rejectモードを明示化

**推定時間**: 1.5時間

#### 3.5.1 コード変更

- [ ] `internal/runner/debug/inheritance.go` を開く
- [ ] `PrintFromEnvInheritance` 関数を特定
- [ ] 既存の判定ロジック（if-else）を削除
- [ ] `runtimeGroup.EnvAllowlistInheritanceMode` を取得
- [ ] switch文を実装
  - [ ] `InheritanceModeInherit` ケースを実装
    - [ ] "Inheriting Global env_allowlist" メッセージ
    - [ ] Global allowlistが空の場合の処理
    - [ ] Global allowlistがある場合の処理（変数数を表示）
  - [ ] `InheritanceModeExplicit` ケースを実装
    - [ ] "Using group-specific env_allowlist" メッセージ
    - [ ] Group allowlistを表示（変数数を表示）
  - [ ] `InheritanceModeReject` ケースを実装（新規）
    - [ ] "Rejecting all environment variables" メッセージ
    - [ ] 説明メッセージ
  - [ ] defaultケースを実装（エラーハンドリング）

#### 3.5.2 テスト実装

- [ ] `internal/runner/debug/inheritance_test.go` にテストを追加/更新
  - [ ] `TestPrintFromEnvInheritance_Inherit_WithGlobalAllowlist`
  - [ ] `TestPrintFromEnvInheritance_Inherit_EmptyGlobalAllowlist`
  - [ ] `TestPrintFromEnvInheritance_Explicit`
  - [ ] `TestPrintFromEnvInheritance_Reject` (新規)

#### 3.5.3 手動テスト

- [ ] テスト用TOML設定ファイルを作成
  - [ ] Inheritモードのグループ
  - [ ] Explicitモードのグループ
  - [ ] Rejectモードのグループ
- [ ] runnerをビルド: `make build`
- [ ] デバッグモードで実行: `./build/runner --config test.toml --debug`
- [ ] 出力を確認
  - [ ] Inheritモードが正しく表示されることを確認
  - [ ] Explicitモードが正しく表示されることを確認
  - [ ] Rejectモードが明示的に表示されることを確認

#### 3.5.4 検証

- [ ] テストを実行: `go test ./internal/runner/debug/...`
- [ ] すべてのテストが成功することを確認
- [ ] lintを実行: `golangci-lint run ./internal/runner/debug/...`
- [ ] lint警告がないことを確認

#### 3.5.5 コミット

- [ ] 変更をステージング: `git add internal/runner/debug/inheritance*.go`
- [ ] テスト用TOMLを削除（必要に応じて）
- [ ] コミット: `git commit -m "refactor: simplify PrintFromEnvInheritance using InheritanceMode"`

---

### 3.6 フェーズ6: 統合テスト

**目標**: エンドツーエンドのテストを実装し、全体の動作を確認

**推定時間**: 1時間

#### 3.6.1 統合テスト実装

- [ ] 統合テストファイルを作成または更新
  - [ ] `internal/runner/integration_inheritance_mode_test.go` (新規推奨)
- [ ] Inheritモードの統合テストを実装
  - [ ] TOML設定を準備
  - [ ] Loader → Validator → Expander → Debug の流れをテスト
  - [ ] 継承モードが正しく設定されることを確認
  - [ ] デバッグ出力が正しいことを確認
- [ ] Explicitモードの統合テストを実装
- [ ] Rejectモードの統合テストを実装

#### 3.6.2 全体テスト

- [ ] すべてのユニットテストを実行: `make test` または `go test ./...`
- [ ] すべてのテストが成功することを確認
- [ ] 統合テストを実行
- [ ] すべての統合テストが成功することを確認

#### 3.6.3 カバレッジ確認

- [ ] カバレッジレポートを生成: `go test -coverprofile=coverage.out ./...`
- [ ] カバレッジを表示: `go tool cover -html=coverage.out`
- [ ] 追加したコードのカバレッジを確認
  - [ ] `DetermineEnvAllowlistInheritanceMode`: 100%
  - [ ] Validator変更箇所: 既存カバレッジ維持
  - [ ] Expansion変更箇所: 既存カバレッジ維持
  - [ ] Debug変更箇所: 既存カバレッジ維持

#### 3.6.4 Lint確認

- [ ] 全体のlintを実行: `make lint` または `golangci-lint run ./...`
- [ ] lint警告がないことを確認
- [ ] 必要に応じて警告を修正

#### 3.6.5 コミット

- [ ] 変更をステージング: `git add internal/runner/*_test.go`
- [ ] コミット: `git commit -m "test: add integration tests for inheritance mode tracking"`

---

## 4. コードレビューチェックリスト

### 4.1 機能レビュー

- [ ] `DetermineEnvAllowlistInheritanceMode` の判定ロジックが正しい
  - [ ] nilチェックが最初に実行される
  - [ ] 空スライスチェックが2番目に実行される
  - [ ] デフォルトケースがExplicitモード
- [ ] `RuntimeGroup.EnvAllowlistInheritanceMode` が適切に設定される
  - [ ] `ExpandGroup` 内で必ず設定される
  - [ ] 設定タイミングが適切（RuntimeGroup作成直後）
- [ ] Validatorの変更が正しい
  - [ ] 3つのモードすべてが処理される
  - [ ] 既存の警告が維持される
- [ ] Debugの変更が正しい
  - [ ] 3つのモードすべてが出力される
  - [ ] Rejectモードが明示的に表示される

### 4.2 テストレビュー

- [ ] ユニットテストがすべてのケースをカバー
  - [ ] nil、空スライス、1要素、複数要素のテストがある
  - [ ] 各モジュールの変更に対応するテストがある
- [ ] テストが明確で理解しやすい
  - [ ] Arrange-Act-Assertパターンに従っている
  - [ ] テスト名が内容を明確に表している
- [ ] エッジケースがテストされている
  - [ ] 空のグローバルallowlist
  - [ ] 環境変数を使用するコマンドがある場合のRejectモード

### 4.3 コーディング規約レビュー

- [ ] godocスタイルのコメントがある
- [ ] コメントが英語で書かれている（コード内）
- [ ] 変数名が明確で一貫している
- [ ] エラーハンドリングが適切（該当する場合）

### 4.4 性能レビュー

- [ ] 計算量が増加していない
- [ ] メモリ使用量が許容範囲内
- [ ] 不要な計算やコピーがない

### 4.5 セキュリティレビュー

- [ ] 既存のセキュリティポリシーを変更していない
- [ ] 新たな攻撃ベクトルを導入していない
- [ ] 型安全性が維持・向上している

---

## 5. ドキュメント更新チェックリスト

### 5.1 コード内ドキュメント

- [ ] `env_inheritance.go` にgodocコメントを追加
- [ ] `runtime.go` のフィールドにgodocコメントを追加
- [ ] 必要に応じて他のファイルのコメントを更新

### 5.2 開発者向けドキュメント

- [ ] `docs/dev/config-inheritance-behavior.ja.md` を更新
  - [ ] 継承モード追跡の説明を追加
  - [ ] DetermineEnvAllowlistInheritanceMode関数の説明を追加
- [ ] `docs/dev/design-implementation-overview.ja.md` を更新
  - [ ] 新機能の概要を追加
  - [ ] アーキテクチャ図を更新（必要に応じて）

### 5.3 ユーザー向けドキュメント

- [ ] リリースノートを準備
  - [ ] 新機能の説明
  - [ ] デバッグ出力の変更点
  - [ ] 後方互換性の説明
- [ ] 必要に応じてユーザーガイドを更新

---

## 6. リリース準備チェックリスト

### 6.1 テスト実行

- [ ] すべてのユニットテストが成功: `make test`
- [ ] すべての統合テストが成功
- [ ] カバレッジが目標を達成
  - [ ] `DetermineEnvAllowlistInheritanceMode`: 100%
  - [ ] 他のモジュール: 既存カバレッジ維持
- [ ] lintが成功: `make lint`
- [ ] セキュリティチェックが成功: `make security-check`

### 6.2 ビルド確認

- [ ] ビルドが成功: `make build`
- [ ] 生成されたバイナリが動作することを確認
- [ ] クロスコンパイルが成功（必要に応じて）

### 6.3 手動テスト

- [ ] 実際のTOML設定で動作確認
  - [ ] Inheritモードのグループ
  - [ ] Explicitモードのグループ
  - [ ] Rejectモードのグループ
- [ ] デバッグ出力が期待通りであることを確認
- [ ] 既存機能に影響がないことを確認

### 6.4 ドキュメント確認

- [ ] すべてのgodocコメントが適切
- [ ] 開発者向けドキュメントが更新済み
- [ ] リリースノートが準備済み

### 6.5 バージョン管理

- [ ] バージョン番号を決定（SemVer）
  - [ ] 提案: v1.(x+1).0（マイナーバージョンアップ）
- [ ] CHANGELOGを更新
- [ ] バージョンタグを作成準備

---

## 7. デプロイチェックリスト

### 7.1 最終確認

- [ ] すべての変更がコミット済み
- [ ] すべてのテストが成功
- [ ] lintが成功
- [ ] ドキュメントが更新済み

### 7.2 タグとリリース

- [ ] バージョンタグを作成: `git tag v1.x.0`
- [ ] タグをプッシュ: `git push origin v1.x.0`
- [ ] GitHubでリリースを作成
  - [ ] リリースノートを貼り付け
  - [ ] バイナリを添付（必要に応じて）

### 7.3 リリース後

- [ ] リリースアナウンス（必要に応じて）
- [ ] ドキュメントサイトを更新（該当する場合）
- [ ] 次のマイルストーンを計画

---

## 8. トラブルシューティング

### 8.1 一般的な問題と対処法

#### 問題: テストが失敗する

**対処法:**
1. エラーメッセージを確認
2. 該当するテストを個別に実行: `go test -v -run TestName`
3. デバッグ出力を追加して原因を特定
4. 必要に応じてテストまたは実装を修正

#### 問題: lint警告が出る

**対処法:**
1. 警告メッセージを確認
2. godocコメントの不足: コメントを追加
3. 未使用の変数/インポート: 削除または`_`を使用
4. コーディング規約違反: gofmtを実行

#### 問題: カバレッジが低い

**対処法:**
1. カバレッジレポートで未カバーの箇所を特定
2. 該当するテストケースを追加
3. エッジケースがカバーされているか確認

#### 問題: 既存テストが失敗する

**対処法:**
1. 変更内容を確認
2. 既存の動作を変更していないか確認
3. テストの期待値を更新（動作変更が意図的な場合）
4. 実装を修正（動作変更が意図的でない場合）

### 8.2 ロールバック手順

フェーズごとに独立したコミットを作成しているため、問題が発生した場合は該当フェーズのコミットをrevertできる。

```bash
# 特定のコミットをrevert
git revert <commit-hash>

# 直前のコミットをrevert
git revert HEAD

# revertをプッシュ
git push origin <branch-name>
```

---

## 9. 進捗トラッキング

### 9.1 進捗サマリー

実装の進捗を以下のフォーマットで記録する:

```
日付: 2025-10-30
実装者: [名前]

完了したフェーズ:
- [x] フェーズ1: ヘルパー関数実装
- [ ] フェーズ2: RuntimeGroup拡張
- [ ] フェーズ3: Validator更新
- [ ] フェーズ4: Expansion更新
- [ ] フェーズ5: Debug更新
- [ ] フェーズ6: 統合テスト

現在の状態: フェーズ1完了、フェーズ2実装中

ブロッカー: なし

次のステップ: フェーズ2を完了する
```

### 9.2 日次レポート（オプション）

実装期間中、日次で以下を記録することを推奨:

- 完了したタスク
- 発見した問題
- 解決した問題
- 次の作業予定

---

## 10. 付録

### 10.1 コミットメッセージテンプレート

各フェーズで推奨されるコミットメッセージ:

```
フェーズ1:
feat: add DetermineEnvAllowlistInheritanceMode helper

- Add DetermineEnvAllowlistInheritanceMode function to determine inheritance
  mode from env_allowlist configuration
- Implement logic to distinguish Inherit, Explicit, and Reject modes
- Add comprehensive unit tests with table-driven approach
- Achieve 100% test coverage for the new function

フェーズ2:
feat: add EnvAllowlistInheritanceMode field to RuntimeGroup

- Add EnvAllowlistInheritanceMode field to track inheritance mode at runtime
- Add detailed godoc comments explaining field purpose and lifecycle
- No behavior change in this commit

フェーズ3:
refactor: use DetermineEnvAllowlistInheritanceMode in Validator

- Replace inline inheritance mode determination logic with helper function
- Use switch statement for clearer mode handling
- Maintain existing validation warnings
- No behavior change, only code structure improvement

フェーズ4:
feat: set EnvAllowlistInheritanceMode in Expander

- Set EnvAllowlistInheritanceMode field during group expansion
- Add unit tests to verify mode is set correctly for all three modes
- Inheritance mode is now available in RuntimeGroup for downstream usage

フェーズ5:
refactor: simplify PrintFromEnvInheritance using InheritanceMode

- Use RuntimeGroup.EnvAllowlistInheritanceMode instead of inline logic
- Explicitly handle Reject mode in debug output
- Improve output readability with variable counts
- Add default case for defensive programming

フェーズ6:
test: add integration tests for inheritance mode tracking

- Add end-to-end tests for Inherit, Explicit, and Reject modes
- Verify mode is correctly set through Loader -> Validator -> Expander flow
- Verify debug output matches expected format for each mode
```

### 10.2 参考リンク

- [詳細設計書](./03_detailed_design.ja.md)
- [アーキテクチャ設計書](./02_architecture.ja.md)
- [要件定義書](./01_requirements.ja.md)

---

**文書バージョン**: 1.0
**作成日**: 2025-10-30
**最終更新**: 2025-10-30
**ステータス**: 準備完了
