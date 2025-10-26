# テストインフラ整備 - 要件定義書

**タスクID**: 0044-05
**作成日**: 2025-10-26
**ステータス**: 要件定義
**関連文書**: [04_implementation_plan.md - Section 11](04_implementation_plan.md#11-テストカバレッジ改善戦略)

## 1. 背景と目的

### 1.1 背景

[04_implementation_plan.md](04_implementation_plan.md)のセクション11「テストカバレッジ改善戦略」において、GroupExecutorのテストカバレッジを90%以上に引き上げる計画が策定された。Phase 1（優先度1）の実装により、`createCommandContext`のカバレッジは100%に到達したが、残りのテストケース（優先度2, 3）の実装には以下のモックインフラが不足している：

1. **security.Validator**のモック実装
2. **verification.Manager**のモック実装

これらのモックなしでは、以下のテストケースが実装できない：

**優先度1（延期中）**:
- T1.2: 環境変数検証エラーテスト
- T1.3: パス解決エラーテスト

**優先度2**:
- T2.1: dry-run DetailLevelFull テスト
- T2.2: dry-run 変数展開デバッグテスト

**優先度3**:
- T3.1: VerificationManager nil テスト
- T3.2: KeepTempDirs テスト
- T3.3: NotificationFunc nil テスト
- T3.4: 空のDescription テスト
- T3.5: 変数展開エラーテスト
- T3.6: ファイル検証結果ログテスト

### 1.2 目的

本タスクの目的は、GroupExecutorのテストカバレッジ改善に必要なモックインフラを整備し、優先度2と3のテストケース実装を可能にすることである。

## 2. スコープ

### 2.1 対象範囲（In Scope）

1. **モック実装**:
   - `security.Validator`のモック実装
   - `verification.Manager`のモック実装
   - 既存のモックパターンに準拠した設計

2. **テストケース実装**:
   - 優先度1の延期テストケース（T1.2, T1.3）
   - 優先度2のテストケース（T2.1, T2.2）
   - 優先度3のテストケース（T3.1～T3.6）

3. **ドキュメント更新**:
   - 実装計画書の更新（カバレッジ結果の反映）
   - モック実装ガイドラインの作成

### 2.2 対象外（Out of Scope）

- 他のパッケージのテストカバレッジ改善
- 既存のモック実装の大幅なリファクタリング
- パフォーマンステストの追加
- 統合テストの追加

## 3. 機能要件

### 3.1 security.Validatorモック

#### FR-1.1: 基本インターフェース

**要件**: security.Validatorの主要メソッドをモック可能にする

**補足**: `security.Validator`は構造体であるため、テスト容易性向上のために`ValidatorInterface`を定義し、それをモックの対象とする。

**対象メソッド**:
```go
ValidateAllEnvironmentVars(envVars map[string]string) error
ValidateEnvironmentValue(key, value string) error
ValidateCommand(command string) error
ValidateWorkDir(workDir string) error
```

**実装場所**:
- **インターフェース**: `internal/runner/security/interfaces.go`
- **モック**: `internal/runner/security/testing/testify_mocks.go`

#### FR-1.2: エラーシミュレーション

**要件**: テストケースでエラーケースを再現できること

**必須機能**:
- 特定の環境変数名・値パターンでエラーを返す
- カスタムエラーメッセージの設定
- 成功/失敗の切り替え

### 3.2 verification.Managerモック

#### FR-2.1: 基本インターフェース

**要件**: verification.Managerの主要メソッドをモック可能にする

**補足**: `verification.Manager`は構造体であるため、テスト容易性向上のために`ManagerInterface`を定義し、それをモックの対象とする。

**対象メソッド**:
```go
ResolvePath(path string) (string, error)
VerifyGroupFiles(group *runnertypes.GroupSpec) (*verification.Result, error)
```

**実装場所**:
- **インターフェース**: `internal/verification/interfaces.go`
- **モック**: `internal/verification/testing/testify_mocks.go`

#### FR-2.2: パス解決シミュレーション

**要件**: コマンドパスの解決動作をシミュレートできること

**必須機能**:
- 存在するパスと存在しないパスのシミュレーション
- カスタムエラーの返却
- 複数パターンの事前設定

### 3.3 テストケース実装

#### FR-3.1: 優先度1延期テストの完了

**要件**: T1.2とT1.3を実装し、スキップを解除する

**対象**:
- `TestExecuteCommandInGroup_ValidateEnvironmentVarsFailure` (T1.2)
- `TestExecuteCommandInGroup_ResolvePathFailure` (T1.3)

#### FR-3.2: 優先度2テストの実装

**要件**: dry-run関連のテストケースを実装する

**対象**:
- `TestExecuteCommandInGroup_DryRunDetailLevelFull` (T2.1)
- `TestExecuteGroup_DryRunVariableExpansion` (T2.2)

#### FR-3.3: 優先度3テストの実装

**要件**: エッジケースとオプショナル機能のテストを実装する

**対象**: T3.1～T3.6（詳細は[04_implementation_plan.md](04_implementation_plan.md#優先度3-オプショナル機能とエッジケース-目標完了-第3週)参照）

## 4. 非機能要件

### 4.1 保守性

**NFR-1.1**: モックコードの可読性
- 既存のモックパターン（`internal/runner/testing/mocks.go`）に準拠
- 明確な命名規則とコメント
- テスト可能なモック実装

**NFR-1.2**: ドキュメント整備
- モック使用方法のサンプルコード
- トラブルシューティングガイド

### 4.2 拡張性

**NFR-2.1**: 将来のメソッド追加への対応
- testify/mockを使用したジェネリックなモック実装
- 新規メソッドの追加が容易な設計

### 4.3 パフォーマンス

**NFR-3.1**: テスト実行時間
- 新規テストケースの実行時間: 各50ms以内
- 全テストの実行時間増加: 1秒以内

### 4.4 互換性

**NFR-4.1**: 既存テストへの影響なし
- 既存テストが全てパス
- lint チェックが0 issues
- ビルドエラーなし

## 5. 制約条件

### 5.1 技術的制約

1. **既存アーキテクチャの尊重**:
   - security.Validatorは構造体（インターフェースではない）
   - verification.Managerも構造体（インターフェースではない）
   - モックはtestify/mockを使用

2. **テストタグの使用**:
   - テストコードは`-tags test`でビルド
   - 本番コードへの依存なし

3. **ディレクトリ構造**:
   - モックは`<package>/testing/`ディレクトリに配置
   - `testify_mocks.go`ファイルに実装

### 5.2 プロジェクト制約

1. **タイムボックス**:
   - Phase 1: モック実装（2日間）
   - Phase 2: 優先度1,2テスト実装（2日間）
   - Phase 3: 優先度3テスト実装（2日間）
   - 合計: 6営業日

2. **リソース**:
   - 開発者: 1名
   - レビュアー: 必要に応じて

## 6. 成功基準

### 6.1 カバレッジ目標

- **GroupExecutor全体**: 71.4% → 90%以上
- **executeCommandInGroup**: 71.4% → 95%以上
- **ExecuteGroup**: 73.7% → 92%以上
- **resolveGroupWorkDir**: 83.3% → 100%

### 6.2 品質基準

1. ✅ 全テストパス（既存 + 新規）
2. ✅ `make lint`: 0 issues
3. ✅ `make test`: 0 failures
4. ✅ リグレッションなし

### 6.3 ドキュメント基準

1. ✅ 実装計画書の更新
2. ✅ モック使用例の追加
3. ✅ カバレッジレポートの作成

## 7. リスク分析

### 7.1 技術リスク

| リスク | 影響度 | 発生確率 | 対策 |
|-------|--------|---------|-----|
| モック実装が複雑化 | 高 | 中 | シンプルな設計を優先、段階的実装 |
| 既存テストへの影響 | 高 | 低 | 段階的マージ、CI/CDでの検証 |
| パフォーマンス劣化 | 中 | 低 | ベンチマークテストの実施 |

### 7.2 スケジュールリスク

| リスク | 影響度 | 発生確率 | 対策 |
|-------|--------|---------|-----|
| モック実装の遅延 | 高 | 中 | 最小限の機能から実装開始 |
| テストケース実装の遅延 | 中 | 低 | 優先度順に実装、段階的完了 |

## 8. 前提条件と依存関係

### 8.1 前提条件

1. Phase 1（T1.1）が完了していること
2. 既存のモックパターンが理解されていること
3. testify/mockライブラリが利用可能であること

### 8.2 依存関係

**ブロッカー**（本タスク開始前に必要）:
- なし（即座に開始可能）

**ブロックされるタスク**（本タスク完了後に開始可能）:
- 他のパッケージのテストカバレッジ改善
- エンドツーエンドテストの追加

## 9. 承認

本要件定義書は、以下の条件を満たした時点で承認されたものとする：

- [ ] テストカバレッジ改善戦略（Section 11）との整合性確認
- [ ] 技術的実現可能性の確認
- [ ] スケジュールの妥当性確認

## 10. 付録

### 10.1 参考資料

1. [04_implementation_plan.md - Section 11](04_implementation_plan.md#11-テストカバレッジ改善戦略)
2. [既存モック実装](../../internal/runner/testing/mocks.go)
3. [testify/mock ドキュメント](https://pkg.go.dev/github.com/stretchr/testify/mock)

### 10.2 用語集

| 用語 | 定義 |
|-----|------|
| モック | テスト用に実装の代替を提供するオブジェクト |
| testify | Go言語のテストフレームワーク |
| カバレッジ | テストでカバーされているコードの割合 |
| スキップテスト | 条件により実行されないテスト |
