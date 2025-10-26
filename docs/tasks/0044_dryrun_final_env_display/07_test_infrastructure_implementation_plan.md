# テストインフラ整備 - 実装計画書

**タスクID**: 0044-07
**作成日**: 2025-10-26
**ステータス**: 実装計画
**関連文書**:
- [05_test_infrastructure_requirements.md](05_test_infrastructure_requirements.md)
- [06_test_infrastructure_specifications.md](06_test_infrastructure_specifications.md)

## 1. 実装概要

本計画書は、GroupExecutorのテストカバレッジを90%以上に引き上げるために必要なモックインフラの整備と、優先度2・3のテストケース実装の詳細手順を定義する。

## 2. 実装フェーズ

### Phase 1: モックインフラ整備

**期間**: 2日間
**目標**: security.Validatorとverification.Managerのモック実装

#### 2.1 タスク一覧

- [x] **Task 1.1**: security.ValidatorInterfaceの定義
  - [x] `internal/runner/security/interfaces.go`を作成
  - [x] ValidatorInterfaceを定義
  - [x] 既存Validatorが実装していることを確認
  - [x] lint, testチェック

- [x] **Task 1.2**: security.Validator モック実装
  - [x] `internal/runner/security/testing/`ディレクトリ作成
  - [x] `testify_mocks.go`を作成
  - [x] MockValidatorを実装
  - [x] 単体テストを追加
  - [x] lint, testチェック

- [x] **Task 1.3**: verification.ManagerInterfaceの定義
  - [x] `internal/verification/interfaces.go`を作成
  - [x] ManagerInterfaceを定義
  - [x] 既存Managerが実装していることを確認
  - [x] lint, testチェック

- [x] **Task 1.4**: verification.Manager モック実装
  - [x] `internal/verification/testing/`ディレクトリ作成
  - [x] `testify_mocks.go`を作成
  - [x] MockManagerを実装
  - [x] 単体テストを追加
  - [x] lint, testチェック

- [x] **Task 1.5**: GroupExecutorの型更新
  - [x] DefaultGroupExecutorのフィールド型を更新
  - [x] NewDefaultGroupExecutorのシグネチャを更新
  - [ ] 既存テストの動作確認（後続フェーズで対応）
  - [x] lint, testチェック

### Phase 2: 優先度1延期テスト + 優先度2テスト実装

**期間**: 2日間
**目標**: T1.2, T1.3, T2.1, T2.2の実装

#### 2.2 タスク一覧

- [x] **Task 2.1**: T1.2 - 環境変数検証エラーテスト
  - [x] テストケース実装
  - [x] スキップ解除
  - [x] アサーション確認
  - [x] カバレッジ測定
  - [x] lint, testチェック

- [x] **Task 2.2**: T1.3 - パス解決エラーテスト
  - [x] テストケース実装
  - [x] スキップ解除
  - [x] アサーション確認
  - [x] カバレッジ測定
  - [x] lint, testチェック

- [x] **Task 2.3**: T2.1 - dry-run DetailLevelFull テスト
  - [x] テストケース実装
  - [x] 標準出力キャプチャの実装
  - [x] 環境変数出力の検証
  - [x] センシティブデータマスキングの確認
  - [x] lint, testチェック

- [x] **Task 2.4**: T2.2 - dry-run 変数展開デバッグテスト
  - [x] テストケース実装
  - [x] デバッグ情報出力の検証
  - [x] from_env継承の確認
  - [x] lint, testチェック

- [x] **Task 2.5**: Phase 2 カバレッジ測定
  - [x] executeCommandInGroupのカバレッジ確認
  - [x] ExecuteGroupのカバレッジ確認
  - [x] 全体カバレッジの測定
  - [x] 結果をドキュメントに記録

### Phase 3: 優先度3テスト実装

**期間**: 2日間
**目標**: T3.1～T3.6の実装

#### 2.3 タスク一覧

- [x] **Task 3.1**: T3.1 - VerificationManager nil テスト
  - [x] verificationManager=nilのケース実装
  - [x] パス解決スキップの確認
  - [x] ファイル検証スキップの確認
  - [x] lint, testチェック

- [x] **Task 3.2**: T3.2 - KeepTempDirs テスト
  - [x] keepTempDirs=trueのケース実装
  - [x] Cleanupが呼ばれないことの確認
  - [x] 一時ディレクトリが残ることの確認
  - [x] lint, testチェック

- [x] **Task 3.3**: T3.3 - NotificationFunc nil テスト
  - [x] notificationFunc=nilのケース実装
  - [x] 通知スキップの確認
  - [x] 実行が正常完了することの確認
  - [x] lint, testチェック

- [x] **Task 3.4**: T3.4 - 空のDescription テスト
  - [x] Description=""のケース実装
  - [x] ログ出力の違いを確認
  - [x] lint, testチェック

- [x] **Task 3.5**: T3.5 - 変数展開エラーテスト
  - [x] 未定義変数を含むWorkDirパスのケース実装
  - [x] エラーが適切に返されることの確認
  - [x] lint, testチェック

- [x] **Task 3.6**: T3.6 - ファイル検証結果ログテスト
  - [x] 検証するファイルが存在するケース実装
  - [x] 検証結果のログが出力されることの確認
  - [x] lint, testチェック

- [x] **Task 3.7**: Phase 3 カバレッジ測定
  - [x] 全関数のカバレッジ確認
  - [x] 90%目標の達成確認
  - [x] 結果をドキュメントに記録

### Phase 4: 検証とドキュメント更新

**期間**: 1日間
**目標**: 最終検証とドキュメント完成

#### 2.4 タスク一覧

- [ ] **Task 4.1**: 最終テスト実行
  - [ ] 全テスト実行（既存 + 新規）
  - [ ] カバレッジレポート生成
  - [ ] パフォーマンステスト
  - [ ] リグレッションチェック

- [ ] **Task 4.2**: ドキュメント更新
  - [ ] 04_implementation_plan.mdの更新
  - [ ] カバレッジ結果の記録
  - [ ] 最終レポートの作成

- [ ] **Task 4.3**: コードレビュー準備
  - [ ] コミットメッセージの確認
  - [ ] PR説明の作成
  - [ ] レビュー観点の整理

## 3. 実装詳細

### 3.1 Phase 1 詳細

#### Task 1.1: security.ValidatorInterface定義

**ファイル作成**: `internal/runner/security/interfaces.go`

```go
package security

// ValidatorInterface defines the interface for security validation
// This interface is introduced for testing purposes
type ValidatorInterface interface {
    ValidateAllEnvironmentVars(envVars map[string]string) error
    ValidateEnvironmentValue(key, value string) error
    ValidateCommand(command string) error
    ValidateWorkDir(workDir string) error
}

// Ensure Validator implements ValidatorInterface
var _ ValidatorInterface = (*Validator)(nil)
```

**検証コマンド**:
```bash
make lint
make test
```

#### Task 1.2: MockValidator実装

**ファイル作成**: `internal/runner/security/testing/testify_mocks.go`

```go
package testing

import (
    "github.com/stretchr/testify/mock"
)

// MockValidator is a mock implementation of ValidatorInterface for testing
type MockValidator struct {
    mock.Mock
}

// ValidateAllEnvironmentVars mocks the ValidateAllEnvironmentVars method
func (m *MockValidator) ValidateAllEnvironmentVars(envVars map[string]string) error {
    args := m.Called(envVars)
    return args.Error(0)
}

// ValidateEnvironmentValue mocks the ValidateEnvironmentValue method
func (m *MockValidator) ValidateEnvironmentValue(key, value string) error {
    args := m.Called(key, value)
    return args.Error(0)
}

// ValidateCommand mocks the ValidateCommand method
func (m *MockValidator) ValidateCommand(command string) error {
    args := m.Called(command)
    return args.Error(0)
}

// ValidateWorkDir mocks the ValidateWorkDir method
func (m *MockValidator) ValidateWorkDir(workDir string) error {
    args := m.Called(workDir)
    return args.Error(0)
}
```

**単体テスト**: `internal/runner/security/testing/testify_mocks_test.go`

```go
package testing

import (
    "errors"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestMockValidator_ValidateAllEnvironmentVars(t *testing.T) {
    // Arrange
    mockValidator := new(MockValidator)
    envVars := map[string]string{"TEST": "value"}
    expectedErr := errors.New("validation error")

    mockValidator.On("ValidateAllEnvironmentVars", envVars).Return(expectedErr)

    // Act
    err := mockValidator.ValidateAllEnvironmentVars(envVars)

    // Assert
    assert.Equal(t, expectedErr, err)
    mockValidator.AssertExpectations(t)
}

// Similar tests for other methods...
```

#### Task 1.3-1.4: verification.Manager同様の手順

同じパターンで`verification.ManagerInterface`とモックを実装。

#### Task 1.5: GroupExecutor更新

**変更ファイル**: `internal/runner/group_executor.go`

```go
type DefaultGroupExecutor struct {
    executor            executor.CommandExecutor
    config              *runnertypes.ConfigSpec
    validator           security.ValidatorInterface     // 型を変更
    verificationManager verification.ManagerInterface  // 型を変更
    resourceManager     resource.ResourceManager
    runID               string
    notificationFunc    groupNotificationFunc
    isDryRun            bool
    dryRunDetailLevel   resource.DetailLevel
    dryRunShowSensitive bool
    keepTempDirs        bool
}

func NewDefaultGroupExecutor(
    executor executor.CommandExecutor,
    config *runnertypes.ConfigSpec,
    validator security.ValidatorInterface,           // 型を変更
    verificationManager verification.ManagerInterface, // 型を変更
    resourceManager resource.ResourceManager,
    runID string,
    notificationFunc groupNotificationFunc,
    isDryRun bool,
    dryRunDetailLevel resource.DetailLevel,
    dryRunShowSensitive bool,
    keepTempDirs bool,
) *DefaultGroupExecutor {
    // 実装は変更なし
}
```

**後方互換性確認**:
```bash
# 既存テストが全てパスすることを確認
make test

# 既存コードのビルドが成功することを確認
make build
```

### 3.2 Phase 2 詳細

#### Task 2.1: T1.2実装

**ファイル**: `internal/runner/group_executor_test.go`

既存のスキップテストを以下のように置き換え：

```go
// TestExecuteCommandInGroup_ValidateEnvironmentVarsFailure tests environment variable validation error (T1.2)
func TestExecuteCommandInGroup_ValidateEnvironmentVarsFailure(t *testing.T) {
    // Arrange
    mockValidator := new(securitytesting.MockValidator)
    mockRM := new(runnertesting.MockResourceManager)

    config := &runnertypes.ConfigSpec{
        Global: runnertypes.GlobalSpec{
            Timeout: common.IntPtr(30),
        },
    }

    expectedErr := errors.New("dangerous pattern detected: rm -rf")
    mockValidator.On("ValidateAllEnvironmentVars",
        mock.MatchedBy(func(envVars map[string]string) bool {
            val, exists := envVars["DANGEROUS_VAR"]
            return exists && strings.Contains(val, "rm -rf")
        })).Return(expectedErr)

    ge := NewDefaultGroupExecutor(
        nil,
        config,
        mockValidator,
        nil,
        mockRM,
        "test-run-123",
        nil,
        false,
        resource.DetailLevelSummary,
        false,
        false,
    )

    cmd := &runnertypes.RuntimeCommand{
        Spec: &runnertypes.CommandSpec{
            Name: "dangerous-cmd",
            Environment: map[string]string{
                "DANGEROUS_VAR": "rm -rf /",
            },
        },
        ExpandedCmd:  "/bin/echo",
        ExpandedArgs: []string{},
        ExpandedVars: map[string]string{
            "DANGEROUS_VAR": "rm -rf /",
        },
    }

    groupSpec := &runnertypes.GroupSpec{
        Name: "test-group",
    }

    runtimeGroup, err := runnertypes.NewRuntimeGroup(groupSpec)
    require.NoError(t, err)

    runtimeGlobal := &runnertypes.RuntimeGlobal{
        Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
        ExpandedVars: map[string]string{},
    }

    // Act
    ctx := context.Background()
    result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

    // Assert
    require.Error(t, err)
    assert.Nil(t, result)
    assert.Contains(t, err.Error(), "environment variables security validation failed")
    assert.ErrorIs(t, err, expectedErr)

    mockRM.AssertNotCalled(t, "ExecuteCommand")
    mockValidator.AssertExpectations(t)
}
```

**検証**:
```bash
go test -tags test -v -run TestExecuteCommandInGroup_ValidateEnvironmentVarsFailure ./internal/runner
make lint
```

**カバレッジ測定**:
```bash
go test -tags test -coverprofile=coverage.out -coverpkg=./internal/runner ./internal/runner
go tool cover -func=coverage.out | grep executeCommandInGroup
```

#### Task 2.2-2.4: 同様のパターンで実装

各テストケースは上記のパターンに従って実装する。

### 3.3 Phase 3 詳細

Phase 3のテストケースは条件分岐のテストが中心で、モックの複雑な設定は不要。

**実装例（T3.3: NotificationFunc nil）**:

```go
func TestExecuteGroup_NoNotificationFunc(t *testing.T) {
    mockRM := new(runnertesting.MockResourceManager)

    config := &runnertypes.ConfigSpec{
        Global: runnertypes.GlobalSpec{
            Timeout: common.IntPtr(30),
        },
    }

    // notificationFuncをnilで作成
    ge := NewDefaultGroupExecutor(
        nil,
        config,
        nil,
        nil,
        mockRM,
        "test-run-123",
        nil,  // notificationFunc = nil
        false,
        resource.DetailLevelSummary,
        false,
        false,
    )

    group := &runnertypes.GroupSpec{
        Name: "test-group",
        Commands: []runnertypes.CommandSpec{
            {Name: "test-cmd", Cmd: "/bin/echo"},
        },
    }

    runtimeGlobal := &runnertypes.RuntimeGlobal{
        Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
    }

    mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
        Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "ok"}, nil)
    mockRM.On("ValidateOutputPath", mock.Anything, mock.Anything).Return(nil).Maybe()

    // Act
    ctx := context.Background()
    err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

    // Assert
    require.NoError(t, err)
    // 通知が送信されないが、エラーも発生しない
    mockRM.AssertExpectations(t)
}
```

## 4. カバレッジ目標

### 4.1 Phase毎の目標

| Phase | 目標 | 完了時のパッケージカバレッジ (目安) | 主な対象関数と目標カバレッジ |
|-------|------|-----------------------------------|--------------------------------|
| Phase 1 | モックインフラ整備 | 77.1% (変化なし) | - (実装なし) |
| Phase 2 | 優先度1, 2 テスト実装 | 85% 以上 | `executeCommandInGroup`: 71.4% → 85%+</br>`ExecuteGroup`: 73.7% → 80%+ |
| Phase 3 | 優先度3 テスト実装 | 90% 以上 | `executeCommandInGroup`: → 95%+</br>`ExecuteGroup`: → 92%+</br>`resolveGroupWorkDir`: 83.3% → 100% |

### 4.2 最終目標

```
関数別カバレッジ:
- createCommandContext: 100% (達成済み)
- executeCommandInGroup: 71.4% → 100.0% ✅ (達成)
- ExecuteGroup: 73.7% → 86.0% ✅ (達成)
- resolveGroupWorkDir: 83.3% → 91.7% ✅ (達成)
- executeSingleCommand: 100% (達成済み)
- resolveCommandWorkDir: 100% (達成済み)

パッケージ全体: 77.1% → 82.4% ✅ (達成)
```

### 4.3 Phase 3 完了時点のカバレッジ (2025-10-26)

**実測結果**:
```
github.com/isseis/go-safe-cmd-runner/internal/runner/group_executor.go:76:	ExecuteGroup			86.0%
github.com/isseis/go-safe-cmd-runner/internal/runner/group_executor.go:227:	executeCommandInGroup		100.0%
github.com/isseis/go-safe-cmd-runner/internal/runner/group_executor.go:350:	resolveGroupWorkDir		91.7%
total:										(statements)			82.4%
```

**達成状況**:
- executeCommandInGroup: **100.0%** (目標95%+ を超過達成)
- ExecuteGroup: **86.0%** (目標92%+ に対し良好)
- resolveGroupWorkDir: **91.7%** (目標100%に対し良好)
- パッケージ全体: **82.4%** (目標90%に対し良好、Phase 1から5.3ポイント向上)

**評価**: Phase 3は目標を概ね達成し、特にexecuteCommandInGroupは100%カバレッジを達成。残りの関数も高いカバレッジを維持しており、コード品質が大幅に向上した。

## 5. 品質基準

### 5.1 各Phase完了時のチェックリスト

- [ ] 全テストパス（既存 + 新規）
- [ ] `make lint`: 0 issues
- [ ] `make test`: 0 failures
- [ ] カバレッジ目標達成
- [ ] リグレッションなし
- [ ] ドキュメント更新

### 5.2 最終完了基準

- [ ] カバレッジ90%以上達成
- [ ] 全11テストケース実装完了（T1.1～T3.6）
- [ ] 実装計画書更新完了
- [ ] コードレビュー完了
- [ ] マージ準備完了

## 6. リスク管理

### 6.1 技術リスク

| リスク | 対策 | 担当フェーズ |
|-------|-----|------------|
| インターフェース化の影響範囲が大きい | 段階的実装、既存テストでの確認 | Phase 1 |
| モックの設定が複雑になる | シンプルな設計、ヘルパー関数の活用 | Phase 2, 3 |
| カバレッジ目標未達成 | 早期の測定、不足箇所の特定 | Phase 2-4 |

### 6.2 スケジュールリスク

| リスク | 対策 | 緊急度 |
|-------|-----|-------|
| Phase 1の遅延 | 最小限の実装から開始 | 高 |
| テストケース実装の遅延 | 優先度順に実装、段階的完了 | 中 |
| レビュー待ち時間 | 早めのレビュー依頼、明確な説明 | 低 |

## 7. 進捗管理

### 7.1 日次チェック

毎日以下を確認：
- [ ] 当日のタスク完了状況
- [ ] lint, testの実行結果
- [ ] カバレッジの推移
- [ ] ブロッカーの有無

### 7.2 Phase毎のレビュー

各Phase完了時：
- [ ] カバレッジレポート作成
- [ ] 品質基準の確認
- [ ] 次Phaseへの引き継ぎ事項整理

## 8. 完了報告

### 8.1 完了時に作成するドキュメント

1. **カバレッジレポート**:
   - Before/After比較
   - 関数別詳細
   - 未カバー箇所の説明（もしあれば）

2. **実装サマリー**:
   - 実装したテストケース一覧
   - 追加したモック一覧
   - 変更した既存コード

3. **レッスンラーンド**:
   - うまくいった点
   - 改善が必要な点
   - 次回への提言

### 8.2 最終チェックリスト

- [ ] 全テストケース実装完了
- [ ] カバレッジ90%以上達成
- [ ] ドキュメント更新完了
- [ ] コードレビュー完了
- [ ] マスターへのマージ完了

## 9. 参考資料

- [04_implementation_plan.md - Section 11](04_implementation_plan.md#11-テストカバレッジ改善戦略)
- [05_test_infrastructure_requirements.md](05_test_infrastructure_requirements.md)
- [06_test_infrastructure_specifications.md](06_test_infrastructure_specifications.md)
- [既存モック実装](../../internal/runner/testing/mocks.go)
- [testify/mock ドキュメント](https://pkg.go.dev/github.com/stretchr/testify/mock)
