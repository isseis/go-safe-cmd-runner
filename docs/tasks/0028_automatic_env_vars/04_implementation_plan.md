# 実装計画書: 自動環境変数設定機能

## 1. 概要

本ドキュメントは、自動環境変数設定機能の実装計画を定義する。TDD（テスト駆動開発）アプローチに従い、テストファースト、実装、リファクタリングの順で進める。

## 2. 実装フェーズ

### Phase 1: エラー型定義

- [x] **1.1 エラー型定義**
  - ファイル: `internal/runner/runnertypes/errors.go` (実際の実装場所)
  - タスク: `ReservedEnvPrefixError` 構造体とコンストラクタを追加
  - テスト: エラーメッセージのフォーマット確認
  - 注: エラー型は `AutoEnvPrefix` 定数を参照する

### Phase 2: 自動環境変数生成（AutoEnvProvider）

- [x] **2.1 定数とClock関数型定義**
  - ファイル: `internal/runner/environment/autoenv.go`（新規作成）
  - タスク: 型定義と定数を追加
    - `Clock` 関数型定義（`type Clock func() time.Time`）
    - 定数 `AutoEnvPrefix = "__RUNNER_"` （他のファイルからも参照可能なパッケージレベル定数）
    - 定数 `AutoEnvKeyDatetime = "DATETIME"`
    - 定数 `AutoEnvKeyPID = "PID"`
    - 定数 `DatetimeLayout = "200601021504"`

- [x] **2.2 AutoEnvProviderインターフェース定義**
  - ファイル: `internal/runner/environment/autoenv.go`
  - タスク: インターフェースを定義
    - `AutoEnvProvider` インターフェース
    - `Generate() map[string]string` メソッド

- [x] **2.3 AutoEnvProvider実装（テスト作成）**
  - ファイル: `internal/runner/environment/autoenv_test.go`（新規作成）
  - タスク: `AutoEnvProvider` のテストケース作成
    - `Generate()` のテスト（DATETIME, PID が含まれることを確認）
    - Clock関数を注入した固定時刻テスト
    - 日時フォーマットテスト（ミリ秒ゼロパディング、UTC確認）
    - エッジケース: 年末年始、ナノ秒精度
  - 状態: テスト失敗を確認（実装前）

- [x] **2.4 AutoEnvProvider実装**
  - ファイル: `internal/runner/environment/autoenv.go`
  - タスク: `autoEnvProvider` 構造体と実装
    - `autoEnvProvider struct` 定義（logger, clock フィールド）
    - `NewAutoEnvProvider(clock Clock)` コンストラクタ（clockがnilの場合は `time.Now` をデフォルトとする）
    - `Generate()` メソッド実装（mapを返す）
    - `generateDateTime` プライベートメソッド実装（`p.clock().UTC()` 使用）
    - `generatePID` プライベートメソッド実装（`os.Getpid()` 使用）
  - 状態: テスト成功を確認

### Phase 3: EnvironmentManager実装

- [x] **3.1 EnvironmentManagerインターフェース定義**
  - ファイル: `internal/runner/environment/manager.go`（新規作成）
  - タスク: インターフェースを定義
    - `Manager` インターフェース (lint対応のため`EnvironmentManager`から変更)
    - `ValidateUserEnv(userEnv map[string]string) error` メソッド
    - `BuildEnv(userEnv map[string]string) (map[string]string, error)` メソッド

- [x] **3.2 EnvironmentManager実装（テスト作成）**
  - ファイル: `internal/runner/environment/manager_test.go`（新規作成）
  - タスク: `Manager` のテストケース作成
    - `ValidateUserEnv` のテスト（予約プレフィックス `AutoEnvPrefix` を使用した検証）
    - `BuildEnv` のテスト（自動環境変数とユーザー環境変数のマージ）
    - Clock関数を注入したテスト
  - 状態: テスト失敗を確認（実装前）

- [x] **3.3 EnvironmentManager実装**
  - ファイル: `internal/runner/environment/manager.go`
  - タスク: `manager` 構造体と実装
    - `manager struct` 定義（autoProvider フィールド）
    - `NewManager(clock Clock)` コンストラクタ（clockがnilの場合は内部で `time.Now` を使用）
    - `ValidateUserEnv` メソッド実装（`AutoEnvPrefix` を使用した予約プレフィックスチェック、`manager.go` 内に実装）
    - `BuildEnv` メソッド実装（`autoProvider.Generate()` 呼び出しとマージ）
  - 状態: テスト成功を確認

### Phase 4: 統合テスト

- [ ] **4.1 環境変数統合テスト（テスト作成）**
  - ファイル: `internal/runner/environment/integration_test.go`（新規作成）
  - タスク: EnvironmentManager と VariableExpander の統合テスト
    - `BuildEnv()` で構築した環境変数マップを使って `VariableExpander` が `${__RUNNER_DATETIME}` を展開できることを確認
    - `${__RUNNER_PID}` の展開テスト
  - 状態: テスト失敗を確認（統合前）

- [ ] **4.2 統合確認**
  - タスク: 既存の `VariableExpander` が `EnvironmentManager.BuildEnv()` の結果を使用できることを確認
    - 変更不要であることを確認
  - 状態: テスト成功を確認

### Phase 5: 設定ロード時の検証統合

- [ ] **5.1 Config検証統合（テスト作成）**
  - ファイル: `internal/runner/config/loader_test.go`
  - タスク: 設定ロード時の予約プレフィックス検証テスト
    - コマンドレベル環境変数の検証
    - グループレベル環境変数の検証（存在する場合）
    - エラーケース: 予約プレフィックス使用
  - 状態: テスト失敗を確認（実装前）

- [ ] **5.2 Config検証統合（実装）**
  - ファイル: `internal/runner/config/loader.go`
  - タスク: 設定ロード時に環境変数を検証
    - EnvironmentManagerを使用して各コマンドの環境変数を検証
    - エラー時は設定ロードを中止
  - 状態: テスト成功を確認

- [ ] **5.3 依存性注入の更新（実装）**
  - ファイル: `cmd/runner/main.go`
  - タスク: EnvironmentManagerを各コンポーネントに注入
    - EnvironmentManagerの生成（`NewEnvironmentManager(nil)` - clockはnilでデフォルトの `time.Now` を使用）
    - VariableExpanderへの注入
    - Config Loaderへの注入
  - 状態: ビルド成功を確認

### Phase 6: 統合テストとサンプル

- [ ] **6.1 E2Eテスト作成**
  - ファイル: `sample/auto_env_test.toml`
  - タスク: 実際のTOMLファイルでのテスト
    - 自動環境変数を使用したコマンド
    - 変数展開との組み合わせ

- [ ] **6.2 E2Eテスト実行**
  - タスク: 実際にrunnerを実行してテスト
    - 環境変数が正しく設定されることを確認
    - dry-runモードでの確認

- [ ] **6.3 エラーケースのE2Eテスト**
  - ファイル: `sample/auto_env_error.toml`
  - タスク: 予約プレフィックスエラーのテスト
    - 予約プレフィックスを使用したTOML
    - エラーメッセージの確認

- [ ] **6.4 グループ実行テスト**
  - ファイル: `sample/auto_env_group.toml`
  - タスク: グループ実行での環境変数テスト
    - 各コマンドで環境変数が設定されることを確認

### Phase 7: ドキュメント整備

- [ ] **7.1 README.md更新**
  - ファイル: `README.md`
  - タスク: 自動環境変数機能の説明追加
    - 機能概要
    - 使用例
    - 予約プレフィックスの説明

- [ ] **7.2 TOMLドキュメント更新**
  - ファイル: `docs/toml_config_*.md`（該当ファイル）
  - タスク: 設定ファイルガイドの更新
    - 自動環境変数のリスト
    - 予約プレフィックスの制約
    - サンプルコード

- [ ] **7.3 サンプルファイル作成**
  - ファイル: `sample/auto_env_example.toml`
  - タスク: 実用的なサンプルの作成
    - タイムスタンプ付きバックアップ
    - PIDを使用したロックファイル

### Phase 8: 最終確認とクリーンアップ

- [ ] **8.1 全テスト実行**
  - タスク: `make test` で全テスト実行
    - 既存テストの成功確認
    - 新規テストの成功確認

- [ ] **8.2 Linter実行**
  - タスク: `make lint` でコード品質確認
    - 警告の修正
    - コードスタイルの統一

- [ ] **8.3 フォーマット実行**
  - タスク: `make fmt` でコード整形

- [ ] **8.4 ビルド確認**
  - タスク: `make build` でビルド成功確認

- [ ] **8.5 コードレビュー**
  - タスク: 実装全体のレビュー
    - セキュリティ確認
    - パフォーマンス確認
    - エラーハンドリング確認

- [ ] **8.6 コミット準備**
  - タスク: 変更ファイルの整理
    - 不要なデバッグコード削除
    - コメントの確認

## 3. テストファイル一覧

### 新規作成

1. `internal/runner/environment/autoenv_test.go`
   - AutoEnvProviderのテスト（日時フォーマット、PID、Clock関数など）

2. `internal/runner/environment/manager_test.go`
   - EnvironmentManagerのテスト（検証、環境変数マージなど）

3. `internal/runner/environment/integration_test.go`
   - EnvironmentManager と VariableExpander の統合テスト

4. `sample/auto_env_test.toml`
   - E2Eテスト用サンプル

5. `sample/auto_env_error.toml`
   - エラーケーステスト用サンプル

6. `sample/auto_env_group.toml`
   - グループ実行テスト用サンプル

### 変更

1. `internal/runner/config/loader_test.go`
   - 設定ロード時の検証テスト追加

## 4. 実装ファイル一覧

### 新規作成

1. `internal/runner/environment/autoenv.go`
   - `Clock` 関数型定義
   - 定数定義（`AutoEnvPrefix`, `AutoEnvKeyDatetime`, `AutoEnvKeyPID`, `DatetimeLayout`）
   - `AutoEnvProvider` インターフェース定義
   - `autoEnvProvider` 構造体と実装

2. `internal/runner/environment/manager.go`
   - `EnvironmentManager` インターフェース定義
   - `environmentManager` 構造体と実装
   - 予約プレフィックス検証ロジック（`ValidateUserEnv` メソッド内で実装）

3. `sample/auto_env_example.toml`
   - 実用サンプル

### 変更

1. `internal/runner/errors/errors.go`
   - `ReservedEnvPrefixError` 追加

2. `internal/runner/config/loader.go`
   - 設定ロード時の検証追加（EnvironmentManager経由）

3. `internal/runner/executor/executor.go`
   - EnvironmentManagerの利用を追加

4. `cmd/runner/main.go`
   - EnvironmentManagerの生成と依存性注入

5. `README.md`
   - 機能説明追加

6. `docs/toml_config_*.md`
   - 設定ガイド更新

## 5. 依存関係

### パッケージ間依存

```
errors
  ↑
environment (autoenv.go, manager.go, processor.go)
  ↑
config (loader.go)
  ↑
cmd/runner (main.go)
```

### 実装順序の制約

1. エラー型 → 定数定義
2. Clock関数型・定数定義 → AutoEnvProvider実装
3. AutoEnvProvider → EnvironmentManager実装（検証ロジックを含む）
4. EnvironmentManager → VariableExpander統合
5. EnvironmentManager → Config Loader拡張
6. 全コンポーネント → main.goで依存性注入

## 6. リスクと対策

### リスク1: 既存コードへの影響

- **リスク**: 環境変数マネージャーの変更が既存機能を破壊
- **対策**:
  - 既存テストの継続的実行
  - 変更は additive（既存の動作を変更しない）

### リスク2: タイムゾーン処理の誤り

- **リスク**: UTC以外のタイムゾーンが使用される
- **対策**:
  - `time.Now().UTC()` の明示的使用
  - テストでUTC確認

### リスク3: ミリ秒フォーマットの誤り

- **リスク**: ゼロパディングや桁数の誤り（3桁フォーマット）
- **対策**:
  - 包括的なテストケース
  - エッジケースのテスト（0, 1, 123, 999など）
  - Clock関数を使用した固定時刻テストで正確性確認

### リスク4: 予約プレフィックス検証漏れ

- **リスク**: 検証が一部の設定項目で実行されない
- **対策**:
  - 全ての環境変数定義箇所で検証
  - E2Eテストで確認

## 7. パフォーマンス目標

- 環境変数生成オーバーヘッド: < 10μs
- 予約プレフィックス検証: O(n)、n < 100で < 100μs
- 総合的な実行時間増加: < 1%

## 8. 完了条件

以下の全ての条件を満たした時点で実装完了とする:

- [ ] 全てのユニットテストが成功
- [ ] 全ての統合テストが成功
- [ ] E2Eテストが成功
- [ ] `make test` が成功
- [ ] `make lint` がエラーなし
- [ ] `make build` が成功
- [ ] ドキュメントが更新済み
- [ ] サンプルファイルが作成済み
- [ ] コードレビュー完了

## 9. スケジュール目安

| フェーズ | 推定工数 | 累積工数 |
|---------|---------|---------|
| Phase 1: エラー型定義 | 0.5時間 | 0.5時間 |
| Phase 2: 自動環境変数生成（AutoEnvProvider） | 3時間 | 3.5時間 |
| Phase 3: EnvironmentManager実装 | 3時間 | 6.5時間 |
| Phase 4: 統合テスト | 2時間 | 8.5時間 |
| Phase 5: Config検証統合 | 1.5時間 | 10時間 |
| Phase 6: E2Eテストとサンプル | 2時間 | 12時間 |
| Phase 7: ドキュメント | 1時間 | 13時間 |
| Phase 8: 最終確認 | 1時間 | 14時間 |

**総推定工数**: 14時間

## 10. 次のステップ

実装開始時は以下の順序で進める:

1. Phase 1から順番に実装
2. 各フェーズでTDDサイクルを厳守（テスト→実装→確認）
3. 各フェーズ完了後、チェックボックスをマーク
4. 問題発生時は本ドキュメントを更新
5. 全フェーズ完了後、最終レビュー

## 11. 参考資料

- 要件定義書: `docs/tasks/0028_automatic_env_vars/01_requirements.md`
- アーキテクチャ設計書: `docs/tasks/0028_automatic_env_vars/02_architecture.md`
- 詳細仕様書: `docs/tasks/0028_automatic_env_vars/03_specification.md`
- 既存の変数展開機能: `docs/tasks/0026_variable_expansion/`
