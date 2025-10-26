# テストインフラ整備 - 詳細仕様書

**タスクID**: 0044-06
**作成日**: 2025-10-26
**ステータス**: 詳細設計
**関連文書**: [05_test_infrastructure_requirements.md](05_test_infrastructure_requirements.md)

## 1. モック設計

### 1.1 security.Validatorモック

#### 1.1.1 インターフェース定義

既存の`security.Validator`は構造体であるため、テスト用のインターフェースを定義する。

**ファイル**: `internal/runner/security/interfaces.go`

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

**設計根拠**:
- 既存の`Validator`構造体を変更せず、後方互換性を保つ
- テストでのモック使用を可能にする
- 将来の拡張に対応

#### 1.1.2 モック実装

**ファイル**: `internal/runner/security/testing/testify_mocks.go`

```go
package testing

import (
    "github.com/stretchr/testify/mock"
)

// MockValidator is a mock implementation of ValidatorInterface
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

#### 1.1.3 使用例

```go
func TestExample_WithMockValidator(t *testing.T) {
    // Setup mock
    mockValidator := new(testing.MockValidator)
    mockValidator.On("ValidateAllEnvironmentVars",
        mock.MatchedBy(func(envVars map[string]string) bool {
            _, hasDangerous := envVars["DANGEROUS_VAR"]
            return hasDangerous
        })).Return(errors.New("dangerous environment variable detected"))

    // Use in test
    err := mockValidator.ValidateAllEnvironmentVars(map[string]string{
        "DANGEROUS_VAR": "rm -rf /",
    })

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "dangerous")
    mockValidator.AssertExpectations(t)
}
```

### 1.2 verification.Managerモック

#### 1.2.1 インターフェース定義

**ファイル**: `internal/verification/interfaces.go`

```go
package verification

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ManagerInterface defines the interface for verification management
// This interface is introduced for testing purposes
type ManagerInterface interface {
    ResolvePath(path string) (string, error)
    VerifyGroupFiles(group *runnertypes.GroupSpec) (*Result, error)
}

// Ensure Manager implements ManagerInterface
var _ ManagerInterface = (*Manager)(nil)
```

#### 1.2.2 モック実装

**ファイル**: `internal/verification/testing/testify_mocks.go`

```go
package testing

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
    "github.com/isseis/go-safe-cmd-runner/internal/verification"
    "github.com/stretchr/testify/mock"
)

// MockManager is a mock implementation of verification.ManagerInterface
type MockManager struct {
    mock.Mock
}

// ResolvePath mocks the ResolvePath method
func (m *MockManager) ResolvePath(path string) (string, error) {
    args := m.Called(path)
    return args.String(0), args.Error(1)
}

// VerifyGroupFiles mocks the VerifyGroupFiles method
func (m *MockManager) VerifyGroupFiles(group *runnertypes.GroupSpec) (*verification.Result, error) {
    args := m.Called(group)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*verification.Result), args.Error(1)
}
```

#### 1.2.3 使用例

```go
func TestExample_WithMockVerificationManager(t *testing.T) {
    // Setup mock
    mockVM := new(testing.MockManager)
    mockVM.On("ResolvePath", "/nonexistent/command").
        Return("", errors.New("command not found in PATH"))

    // Use in test
    resolvedPath, err := mockVM.ResolvePath("/nonexistent/command")

    assert.Error(t, err)
    assert.Empty(t, resolvedPath)
    assert.Contains(t, err.Error(), "not found")
    mockVM.AssertExpectations(t)
}
```

### 1.3 GroupExecutorへの統合

#### 1.3.1 インターフェース化の影響

`DefaultGroupExecutor`構造体のフィールドを更新：

**変更前**:
```go
type DefaultGroupExecutor struct {
    validator           *security.Validator
    verificationManager *verification.Manager
    // ... other fields
}
```

**変更後**:
```go
type DefaultGroupExecutor struct {
    validator           security.ValidatorInterface
    verificationManager verification.ManagerInterface
    // ... other fields
}
```

**影響範囲**:
- `NewDefaultGroupExecutor`の型シグネチャ
- テストコードでのモック注入

**後方互換性**:
- 既存の`*security.Validator`と`*verification.Manager`は自動的にインターフェースを満たす
- 既存コードの変更は不要

## 2. テストケース詳細仕様

### 2.1 T1.2: 環境変数検証エラーテスト

#### 2.1.1 テスト目的

環境変数のセキュリティ検証が失敗した場合、適切にエラーが伝播されることを確認する。

#### 2.1.2 テストシナリオ

```go
func TestExecuteCommandInGroup_ValidateEnvironmentVarsFailure(t *testing.T) {
    // Arrange
    mockValidator := new(securitytesting.MockValidator)
    mockRM := new(runnertesting.MockResourceManager)

    // Setup: validator rejects dangerous environment variable
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
        ExpandedVars: map[string]string{
            "DANGEROUS_VAR": "rm -rf /",
        },
    }

    // ... group and global setup

    // Act
    result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

    // Assert
    require.Error(t, err)
    assert.Nil(t, result)
    assert.Contains(t, err.Error(), "environment variables security validation failed")
    assert.ErrorIs(t, err, expectedErr)

    // Verify ExecuteCommand was NOT called
    mockRM.AssertNotCalled(t, "ExecuteCommand")
    mockValidator.AssertExpectations(t)
}
```

**期待される結果**:
- エラーが返される
- resultはnil
- エラーメッセージに"environment variables security validation failed"が含まれる
- 元のエラーが適切にラップされている
- ExecuteCommandが呼ばれない

### 2.2 T1.3: パス解決エラーテスト

#### 2.2.1 テスト目的

コマンドパスの解決が失敗した場合、適切にエラーが伝播されることを確認する。

#### 2.2.2 テストシナリオ

```go
func TestExecuteCommandInGroup_ResolvePathFailure(t *testing.T) {
    // Arrange
    mockVM := new(verificationtesting.MockManager)
    mockRM := new(runnertesting.MockResourceManager)

    // Setup: path resolution fails
    expectedErr := errors.New("command not found in PATH")
    mockVM.On("ResolvePath", "/nonexistent/command").
        Return("", expectedErr)

    ge := NewDefaultGroupExecutor(
        nil,
        config,
        nil, // skip validator for this test
        mockVM,
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
            Name: "test-cmd",
        },
        ExpandedCmd: "/nonexistent/command",
    }

    // ... group and global setup

    // Act
    result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

    // Assert
    require.Error(t, err)
    assert.Nil(t, result)
    assert.Contains(t, err.Error(), "command path resolution failed")
    assert.ErrorIs(t, err, expectedErr)

    // Verify mocks
    mockRM.AssertNotCalled(t, "ExecuteCommand")
    mockVM.AssertCalled(t, "ResolvePath", "/nonexistent/command")
    mockVM.AssertExpectations(t)
}
```

**期待される結果**:
- エラーが返される
- resultはnil
- エラーメッセージに"command path resolution failed"が含まれる
- ResolvePathが正しい引数で呼ばれる
- ExecuteCommandが呼ばれない

### 2.3 T2.1: dry-run DetailLevelFull テスト

#### 2.3.1 テスト目的

dry-runモードでDetailLevelFullが指定された場合、最終環境変数が出力されることを確認する。

#### 2.3.2 テストシナリオ

```go
func TestExecuteCommandInGroup_DryRunDetailLevelFull(t *testing.T) {
    // Arrange
    mockRM := new(runnertesting.MockResourceManager)

    // Capture stdout
    oldStdout := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w
    defer func() {
        os.Stdout = oldStdout
    }()

    ge := NewDefaultGroupExecutor(
        nil,
        config,
        nil,
        nil,
        mockRM,
        "test-run-123",
        nil,
        true,                     // isDryRun = true
        resource.DetailLevelFull, // DetailLevelFull
        false,                    // dryRunShowSensitive = false
        false,
    )

    cmd := &runnertypes.RuntimeCommand{
        Spec: &runnertypes.CommandSpec{
            Name: "test-cmd",
            Environment: map[string]string{
                "TEST_VAR": "test_value",
                "SECRET":   "secret_value",
            },
        },
        ExpandedCmd:  "/bin/echo",
        ExpandedVars: map[string]string{
            "TEST_VAR": "test_value",
            "SECRET":   "secret_value",
        },
    }

    mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
        Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "[DRY-RUN] output"}, nil)

    // ... group and global setup

    // Act
    result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

    // Capture output
    w.Close()
    var buf bytes.Buffer
    io.Copy(&buf, r)
    output := buf.String()

    // Assert
    require.NoError(t, err)
    assert.NotNil(t, result)

    // Verify environment output
    assert.Contains(t, output, "Final Environment Variables")
    assert.Contains(t, output, "TEST_VAR")
    assert.Contains(t, output, "test_value")
    assert.Contains(t, output, "SECRET")
    assert.Contains(t, output, "***") // SECRET should be masked

    mockRM.AssertExpectations(t)
}
```

**期待される結果**:
- 正常に実行される
- 標準出力に"Final Environment Variables"が含まれる
- 通常の変数値は表示される
- センシティブな変数値はマスクされる

### 2.4 T2.2: dry-run 変数展開デバッグテスト

#### 2.4.1 テスト目的

dry-runモードで変数展開のデバッグ情報が出力されることを確認する。

#### 2.4.2 テストシナリオ

```go
func TestExecuteGroup_DryRunVariableExpansion(t *testing.T) {
    // Arrange
    mockRM := new(runnertesting.MockResourceManager)

    // Capture stdout
    oldStdout := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w
    defer func() {
        os.Stdout = oldStdout
    }()

    ge := NewDefaultGroupExecutor(
        nil,
        config,
        nil,
        nil,
        mockRM,
        "test-run-123",
        nil,
        true,                        // isDryRun = true
        resource.DetailLevelSummary,
        false,
        false,
    )

    group := &runnertypes.GroupSpec{
        Name: "test-group",
        FromEnv: []string{"TEST_VAR"},
        Commands: []runnertypes.CommandSpec{
            {Name: "test-cmd", Cmd: "/bin/echo"},
        },
    }

    runtimeGlobal := &runnertypes.RuntimeGlobal{
        Spec: &runnertypes.GlobalSpec{
            FromEnv: []string{"GLOBAL_VAR"},
        },
        ExpandedVars: map[string]string{},
    }

    mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
        Return(&resource.ExecutionResult{ExitCode: 0}, nil)

    // Act
    err := ge.ExecuteGroup(context.Background(), group, runtimeGlobal)

    // Capture output
    w.Close()
    var buf bytes.Buffer
    io.Copy(&buf, r)
    output := buf.String()

    // Assert
    require.NoError(t, err)
    assert.Contains(t, output, "Variable Expansion Debug Information")
    assert.Contains(t, output, "from_env")

    mockRM.AssertExpectations(t)
}
```

**期待される結果**:
- 正常に実行される
- 標準出力に"Variable Expansion Debug Information"が含まれる
- from_envの継承情報が表示される

### 2.5 T3系テストケース

優先度3のテストケースは、より単純な条件分岐のテストである：

- **T3.1**: verificationManager=nilの場合のスキップ動作
- **T3.2**: keepTempDirs=trueの場合のクリーンアップスキップ
- **T3.3**: notificationFunc=nilの場合の通知スキップ
- **T3.4**: Description=""の場合のログ出力
- **T3.5**: 変数展開エラーの伝播
- **T3.6**: ファイル検証結果のログ出力

これらは既存のテストパターンに従って実装する。詳細は実装計画書で定義する。

## 3. エラーハンドリング仕様

### 3.1 エラーラッピング

すべてのエラーは適切にラップされ、元のエラーが保持される：

```go
if err := validator.ValidateAllEnvironmentVars(envVars); err != nil {
    return nil, fmt.Errorf("environment variables security validation failed: %w", err)
}
```

### 3.2 エラー検証

テストでは`errors.Is()`を使用してエラー型を検証する：

```go
assert.ErrorIs(t, err, expectedErr)
```

## 4. パフォーマンス考慮事項

### 4.1 モックのオーバーヘッド

- testify/mockは軽量で高速
- 各テストケースの実行時間目標: 50ms以内
- 全テスト実行時間の増加: 1秒以内

### 4.2 並列実行

テストは並列実行可能な設計とする：

```go
func TestExample(t *testing.T) {
    t.Parallel() // 可能な場合
    // test code
}
```

## 5. ドキュメント要件

### 5.1 コード内ドキュメント

すべてのモックメソッドにGoDocコメントを追加：

```go
// MockValidator is a mock implementation of ValidatorInterface for testing purposes.
// It uses testify/mock to provide flexible test behavior configuration.
//
// Example usage:
//   mockValidator := new(MockValidator)
//   mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)
type MockValidator struct {
    mock.Mock
}
```

### 5.2 使用例ドキュメント

各モックの使用例を`*_test.go`ファイルに含める。

## 6. 実装順序

### Phase 1: インターフェース定義とモック実装

1. `security.ValidatorInterface`の定義
2. `verification.ManagerInterface`の定義
3. モック実装
4. 単体テスト

### Phase 2: GroupExecutorの更新

1. フィールド型の変更
2. 既存テストの動作確認
3. リグレッションテスト

### Phase 3: 新規テストケースの実装

1. T1.2, T1.3（優先度1延期分）
2. T2.1, T2.2（優先度2）
3. T3.1～T3.6（優先度3）

### Phase 4: 検証とドキュメント

1. カバレッジ測定
2. 実装計画書の更新
3. レビューとマージ
