# Task 0036: 実装計画書

## タスク概要

**タスクID**: 0036
**タイトル**: `internal/runner/runner_test.go`の型移行と再有効化
**優先度**: 高（最優先）
**推定工数**: 2-3日
**前提条件**: Task 0035 Phase 8 完了

## 目標

`internal/runner/runner_test.go`（2569行、21個のテスト関数）を古い型システムから新しいSpec/Runtime分離型システムに完全移行し、`skip_integration_tests`タグを削除して統合テストを再有効化する。

## 実装手順

### Step 1: ヘルパーメソッドの実装（1-2時間）

#### 1.1 MockResourceManager ヘルパーの追加

`internal/runner/runner_test.go` の `MockResourceManager` 型エイリアス定義（現在約60行目）の直後に、以下のメソッドを追加：

```go
// SetupDefaultMockBehavior sets up common default mock expectations for basic test scenarios
func (m *MockResourceManager) SetupDefaultMockBehavior() {
	// Default ValidateOutputPath behavior - allows any output path
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Maybe()

	// Default ExecuteCommand behavior - returns successful execution
	defaultResult := &resource.ExecutionResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
	}
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(defaultResult, nil).Maybe()
}

// SetupSuccessfulMockExecution sets up mock for successful command execution with custom output
func (m *MockResourceManager) SetupSuccessfulMockExecution(stdout, stderr string) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	result := &resource.ExecutionResult{
		ExitCode: 0,
		Stdout:   stdout,
		Stderr:   stderr,
	}
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(result, nil)
}

// SetupFailedMockExecution sets up mock for failed command execution with custom error
func (m *MockResourceManager) SetupFailedMockExecution(err error) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, err)
}

// NewMockResourceManagerWithDefaults creates a new MockResourceManager with default behavior setup
func NewMockResourceManagerWithDefaults() *MockResourceManager {
	mockRM := &MockResourceManager{}
	mockRM.SetupDefaultMockBehavior()
	return mockRM
}
```

#### 1.2 型変換ヘルパー関数の追加

ヘルパーメソッドの後に、型変換を簡単にするヘルパー関数を追加：

```go
// createConfigSpec creates a ConfigSpec from test parameters for easy migration
func createConfigSpec(timeout int, workDir string, groups []runnertypes.GroupSpec) *runnertypes.ConfigSpec {
	return &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: timeout,
			WorkDir: workDir,
		},
		Groups: groups,
	}
}

// createSimpleGroupSpec creates a simple GroupSpec for testing
func createSimpleGroupSpec(name string, commands []runnertypes.CommandSpec) runnertypes.GroupSpec {
	return runnertypes.GroupSpec{
		Name:     name,
		Commands: commands,
	}
}

// createSimpleCommandSpec creates a simple CommandSpec for testing
func createSimpleCommandSpec(name, cmd string, args []string) runnertypes.CommandSpec {
	return runnertypes.CommandSpec{
		Name: name,
		Cmd:  cmd,
		Args: args,
	}
}
```

#### 1.3 検証

```bash
# コンパイルエラーがないことを確認
go build -tags test ./internal/runner/...
```

**コミットメッセージ**: `test: add helper methods for runner_test.go migration`
**ステータス**: ✅ 完了 (2025-10-20)

### Step 2: TestNewRunner の移行（30分）

#### 2.1 変更内容

`TestNewRunner`関数（行114-178）を修正：

```go
func TestNewRunner(t *testing.T) {
	// Before: config := &runnertypes.Config{
	// After:
	config := &runnertypes.ConfigSpec{
		Version: "1.0",  // 追加
		Global: runnertypes.GlobalSpec{  // GlobalConfig → GlobalSpec
			Timeout:  3600,
			WorkDir:  "/tmp",
			LogLevel: "info",
		},
	}

	// 以降のサブテストはそのまま（configの型が変わるだけ）
	// ...
}
```

#### 2.2 検証

```bash
go test -tags test -v ./internal/runner -run TestNewRunner
```

**コミットメッセージ**: `test: migrate TestNewRunner to ConfigSpec`
**ステータス**: ✅ 完了 (2025-10-20)

### Step 3: TestNewRunnerWithSecurity の移行（30分）

同様に`TestNewRunnerWithSecurity`関数（行180-221）を修正。

**コミットメッセージ**: `test: migrate TestNewRunnerWithSecurity to ConfigSpec`
**ステータス**: ✅ 完了 (2025-10-20)

### Step 4: TestRunner_ExecuteGroup の移行（2-3時間）⚠️ 複雑

#### 4.1 変更内容

この関数は`CommandGroup`と`Command`を使用しており、複雑な変更が必要：

**Before** (行223-331の一部):
```go
config := &runnertypes.Config{
	Global: runnertypes.GlobalConfig{
		Timeout:  30,
		WorkDir:  tempDir,
		LogLevel: "info",
	},
	Groups: []runnertypes.CommandGroup{
		{
			Name: "test-group",
			Commands: []runnertypes.Command{
				{
					Name: "cmd1",
					Cmd:  "/bin/echo",
					Args: []string{"hello"},
				},
			},
		},
	},
}

// ...

err := runner.ExecuteGroup(ctx, &config.Groups[0])
```

**After**:
```go
config := &runnertypes.ConfigSpec{
	Version: "1.0",
	Global: runnertypes.GlobalSpec{
		Timeout:  30,
		WorkDir:  tempDir,
		LogLevel: "info",
	},
	Groups: []runnertypes.GroupSpec{  // CommandGroup → GroupSpec
		{
			Name: "test-group",
			Commands: []runnertypes.CommandSpec{  // Command → CommandSpec
				{
					Name: "cmd1",
					Cmd:  "/bin/echo",
					Args: []string{"hello"},
				},
			},
		},
	},
}

// RuntimeGlobal の取得が必要
runtimeGlobal, err := config.ExpandGlobal()
require.NoError(t, err)

// ExecuteGroup の呼び出し方法が変わる
err = runner.groupExecutor.ExecuteGroup(ctx, &config.Groups[0], runtimeGlobal)
```

#### 4.2 注意点

- `Runner`は内部的に`groupExecutor`を持つため、直接`ExecuteGroup`を呼べない場合がある
- `Runner.ExecuteGroup()`メソッドが存在するか確認し、存在する場合はそれを使用
- `RuntimeGlobal`の生成を忘れずに

#### 4.3 検証

```bash
go test -tags test -v ./internal/runner -run TestRunner_ExecuteGroup
```

**コミットメッセージ**: `test: migrate TestRunner_ExecuteGroup to GroupSpec/CommandSpec`
**ステータス**: ✅ 完了 (2025-10-20)

### Step 5: TestRunner_ExecuteGroup_ComplexErrorScenarios の移行（1-2時間）

**コミットメッセージ**: `test: migrate TestRunner_ExecuteGroup_ComplexErrorScenarios to new type system`
**ステータス**: ✅ 完了 (2025-10-20)

### Step 6: TestRunner_ExecuteAll の移行（1-2時間）

**コミットメッセージ**: `test: migrate TestRunner_ExecuteAll to new type system`
**ステータス**: ✅ 完了 (2025-10-20)

### Step 7: TestRunner_ExecuteAll_ComplexErrorScenarios の移行（2-3時間）

**コミットメッセージ**: `test: migrate TestRunner_ExecuteAll_ComplexErrorScenarios to new type system`
**ステータス**: ✅ 完了 (2025-10-20)

### Step 8: TestRunner_CommandTimeoutBehavior の移行（1時間）

**コミットメッセージ**: `test: migrate TestRunner_CommandTimeoutBehavior to new type system`
**ステータス**: ✅ 完了 (2025-10-20)

### Step 9-24: 残りのテスト関数の移行（8-12時間）

各テスト関数を順次移行。複雑度に応じて1-2時間/関数。

#### 進捗状況（7/21 完了、33%）
**完了**:
- ✅ TestNewRunner
- ✅ TestNewRunnerWithSecurity
- ✅ TestRunner_ExecuteGroup
- ✅ TestRunner_ExecuteGroup_ComplexErrorScenarios
- ✅ TestRunner_ExecuteAll
- ✅ TestRunner_ExecuteAll_ComplexErrorScenarios
- ✅ TestRunner_CommandTimeoutBehavior

**未完了**:
- ⏳ TestRunner_ExecuteCommand (次のターゲット)
- ⏳ TestRunner_ExecuteCommand_WithEnvironment
- ⏳ TestRunner_OutputCapture
- ⏳ TestRunner_OutputCaptureEdgeCases
- ⏳ TestRunner_OutputSizeLimit
- ⏳ TestRunner_EnvironmentVariables
- ⏳ TestRunner_ExecuteAllWithPriority
- ⏳ TestRunner_GroupPriority
- ⏳ TestRunner_DependencyHandling ⚠️ 最も複雑
- ⏳ TestRunner_PrivilegedCommand
- ⏳ TestRunner_SecurityValidation
- ⏳ TestRunner_SecurityIntegration
- ⏳ その他2テスト

#### 優先順位
1. **簡単**（30分-1時間）:
   - TestRunner_ExecuteCommand
   - TestRunner_CommandTimeout
   - TestRunner_GroupTimeout
   - TestRunner_GlobalTimeout

2. **中程度**（1-2時間）:
   - TestRunner_ExecuteAll
   - TestRunner_OutputCapture
   - TestRunner_OutputCaptureEdgeCases
   - TestRunner_OutputSizeLimit
   - TestRunner_EnvironmentVariables

3. **複雑**（2-3時間）:
   - TestRunner_ExecuteAllWithPriority
   - TestRunner_GroupPriority
   - TestRunner_DependencyHandling ⚠️ 最も複雑
   - TestRunner_PrivilegedCommand
   - TestRunner_SecurityValidation
   - TestRunner_SecurityIntegration

### Step 24: skip_integration_testsタグの削除（15分）

すべてのテストが成功したら：

```go
// 削除: //go:build skip_integration_tests

package runner

import (
	// ...
)
```

**検証**:
```bash
make test
make lint
```

**コミットメッセージ**: `test: remove skip_integration_tests tag from runner_test.go`

### Step 25: 古い型定義の削除（30分-1時間）

`internal/runner/runnertypes/config.go`から古い型定義を削除：

```go
// 削除対象:
// type Config struct { ... }
// type GlobalConfig struct { ... }
// type CommandGroup struct { ... }
// type Command struct { ... }
```

**注意**: 他のファイルが依存していないことを事前に確認：

```bash
# Config の使用箇所を確認
grep -r "runnertypes\.Config[^S]" --include="*.go" --exclude-dir=".git"

# GlobalConfig の使用箇所を確認
grep -r "runnertypes\.GlobalConfig" --include="*.go" --exclude-dir=".git"
```

**検証**:
```bash
make test
make lint
```

**コミットメッセージ**: `refactor: remove deprecated types (Config, GlobalConfig, CommandGroup, Command)`

## マイルストーン

| マイルストーン | 完了条件 | 推定完了日 |
|--------------|---------|-----------|
| M1: ヘルパー実装完了 | ヘルパーメソッド追加、コンパイル成功 | Day 1 午前 |
| M2: 基本テスト移行完了 | TestNewRunner系 3個完了 | Day 1 午後 |
| M3: 中程度テスト移行完了 | 中程度の複雑度テスト 9個完了 | Day 2 終了 |
| M4: 複雑テスト移行完了 | 複雑なテスト 9個完了 | Day 3 午前 |
| M5: 最終検証完了 | skip_integration_testsタグ削除、全テストPASS | Day 3 午後 |

## リスクと対策

### リスク1: ExecuteGroup等のAPI変更による大規模修正

**対策**:
- 最初にAPI仕様を確認
- 必要に応じてRunnerにラッパーメソッドを追加
- ヘルパー関数でAPI呼び出しをカプセル化

### リスク2: 予想外の型不一致

**対策**:
- 段階的移行（1テスト関数ずつ）
- 各ステップでコンパイル確認
- 問題が発生したらドキュメントを更新

### リスク3: テストの意味が変わる可能性

**対策**:
- 各テストの意図を理解してから移行
- 動作が変わった場合はコメントで説明
- 必要に応じてテストケースを追加

## 成功基準

1. ✅ すべてのテスト関数（21個）が新しい型を使用
2. ✅ `make test` で全テスト PASS
3. ✅ `make lint` でエラーなし
4. ✅ `skip_integration_tests`タグが削除されている
5. ✅ 古い型定義（Config, GlobalConfig, CommandGroup, Command）が削除されている
6. ✅ テストカバレッジが低下していない

## 参考資料

- [移行ガイド](./01_migration_guide.md)
- [Task 0035 アーキテクチャ設計](../0035_spec_runtime_separation/02_architecture.md)
- [テスト再有効化計画](../0035_spec_runtime_separation/test_reactivation_plan.md)
