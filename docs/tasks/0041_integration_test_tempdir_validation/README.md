# Task 0041: 統合テストにおける一時ディレクトリ検証の強化

## 概要

`TestIntegration_TempDirHandling` 統合テストを改善し、一時ディレクトリの作成・削除動作を実際のファイルシステムで検証する機能を追加します。

## 背景

現在の `cmd/runner/integration_workdir_test.go` の `TestIntegration_TempDirHandling` テストには以下の問題があります：

1. **未使用フィールド**: `expectTempDir` フィールドが定義されているが使用されていない
2. **不完全な検証**: 一時ディレクトリの作成・削除が実際に行われているかを確認していない
3. **副作用の未検証**: `keepTempDirs` フラグの効果が検証されていない

これらの問題により、統合テストとしての価値が低下しており、実際のバグを見逃す可能性があります。

## 目的

- 一時ディレクトリの作成・削除が正しく行われることを統合テストで検証する
- `keepTempDirs` フラグが正しく動作することを確認する
- コマンド出力から `__runner_workdir` の値を抽出し、実際のディレクトリ存在を確認する

## 実装方針

### アプローチ

**メモリバッファ方式の出力キャプチャ**を採用します：

- テスト専用の簡易的な出力バッファ (`testOutputBuffer`) を実装
- Executor をラップして出力をキャプチャ (`executorWithOutput`)
- ヘルパー関数で検証ロジックを構造化

**理由:**
- シンプルで理解しやすい
- テストの意図が明確
- プロダクションコードを変更しない

### 実装スコープ

**対象（In Scope）:**
- `testOutputBuffer` 構造体の実装
- `executorWithOutput` 構造体の実装（Executor ラッパー）
- `createRunnerWithOutputCapture()` ヘルパー関数
- `extractWorkdirFromOutput()` ヘルパー関数
- `validateTempDirBehavior()` ヘルパー関数
- `TestIntegration_TempDirHandling` の改善

**対象外（Out of Scope）:**
- プロダクションコードの変更（`internal/runner/*` は変更しない）
- 他の統合テストの変更
- ユニットテストの追加

## ドキュメント

| ドキュメント | 説明 |
|-----------|------|
| [01_requirements.md](01_requirements.md) | 機能要件、非機能要件、テスト要件 |
| [02_architecture.md](02_architecture.md) | アーキテクチャ設計、コンポーネント設計 |
| [03_specification.md](03_specification.md) | 詳細仕様、データ構造、API仕様 |
| [04_implementation_plan.md](04_implementation_plan.md) | 実装計画、フェーズ別作業項目 |

## 実装フェーズ

### Phase 1: テストインフラの実装（2-3時間）

- `testOutputBuffer` 構造体の実装
- `executorWithOutput` 構造体の実装
- `buildEnvSlice()` ヘルパー関数の実装
- 動作確認テスト

### Phase 2: ヘルパー関数の実装（2-3時間）

- `createRunnerWithOutputCapture()` の実装
- `extractWorkdirFromOutput()` の実装
- `validateTempDirBehavior()` の実装

### Phase 3: 統合テストの改善（1-2時間）

- `TestIntegration_TempDirHandling` の書き換え
- テスト実行と検証
- 最終調整

## テストケース

| テストケース | 説明 | 期待される動作 |
|------------|------|--------------|
| TC-001 | 自動一時ディレクトリ（クリーンアップあり） | 一時ディレクトリが作成され、クリーンアップ後に削除される |
| TC-002 | 自動一時ディレクトリ（保持） | 一時ディレクトリが作成され、クリーンアップ後も保持される |
| TC-003 | 固定ワークディレクトリ | テスト内で作成した固定ワークディレクトリが使用され、Runner の一時ディレクトリは作成されない |

**注:** TC-003 では、`/tmp` のようなシステムのグローバルなディレクトリを直接使用せず、テスト内で `os.MkdirTemp()` により作成した一時ディレクトリを固定ワークディレクトリとして使用します。これにより、テストの独立性と堅牢性が向上します。

## 期待される成果

1. **完全な検証**: 一時ディレクトリの作成・削除が実際のファイルシステムで確認される
2. **`expectTempDir` の活用**: 未使用フィールドが適切に使用される
3. **`keepTempDirs` の検証**: フラグが正しく動作することが確認される
4. **統合テストの価値向上**: バグを早期に発見できるテストになる

## 影響範囲

### 変更されるファイル

- `cmd/runner/integration_workdir_test.go`

### 変更されないファイル

- プロダクションコード（`internal/runner/*`）
- 他の統合テスト
- ユニットテスト

## 関連タスク

- **Task 0034**: workdir redesign（親タスク）
- **Phase 4**: テスト実装フェーズの一部

## 推定工数

**合計: 5-8 時間（1-2日）**

- Phase 1: 2-3 時間
- Phase 2: 2-3 時間
- Phase 3: 1-2 時間

## 完了基準

- [ ] Phase 1-3 のすべてのタスクが完了している
- [ ] `TestIntegration_TempDirHandling` のすべてのサブテストが成功する
- [ ] `make test` が成功する
- [ ] `make lint` が成功する
- [ ] コードレビューが完了している

## 開始方法

1. このREADMEを読む
2. [01_requirements.md](01_requirements.md) で要件を確認
3. [02_architecture.md](02_architecture.md) で設計を理解
4. [03_specification.md](03_specification.md) で詳細仕様を確認
5. [04_implementation_plan.md](04_implementation_plan.md) に従って実装を開始

## 参考資料

### コードベース

- `cmd/runner/integration_workdir_test.go`: 改善対象のテストコード
- `internal/runner/executor/executor.go`: Executor インターフェース
- `internal/runner/resource/temp_dir_manager.go`: 一時ディレクトリ管理

### 外部ドキュメント

- [stretchr/testify](https://github.com/stretchr/testify): テストフレームワーク
- [Go testing package](https://pkg.go.dev/testing): Go標準のテストパッケージ

---

**作成日**: 2025-10-22
**最終更新日**: 2025-10-22
**ステータス**: 計画完了、実装準備完了
