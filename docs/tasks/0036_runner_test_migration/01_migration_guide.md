# Task 0036: runner_test.go 型移行ガイド

## 概要

`internal/runner/runner_test.go`（2569行）を古い型システムから新しい型システム（Spec/Runtime分離）に移行するための詳細ガイド。

## 現状分析

### ファイル統計
- **総行数**: 2569行
- **テスト関数数**: 21個
- **型変換箇所**: 約650箇所

### 使用されている古い型
1. `runnertypes.Config` → `runnertypes.ConfigSpec`（33箇所）
2. `runnertypes.GlobalConfig` → `runnertypes.GlobalSpec`（36箇所）
3. `runnertypes.CommandGroup` → `runnertypes.GroupSpec`（推定100箇所）
4. `runnertypes.Command` → `runnertypes.CommandSpec`/`RuntimeCommand`（推定300箇所）

### テスト関数一覧
1. `TestNewRunner` (行114-178)
2. `TestNewRunnerWithSecurity` (行180-221)
3. `TestRunner_ExecuteGroup` (行223-331)
4. `TestRunner_ExecuteGroup_ComplexErrorScenarios` (行333-453)
5. `TestRunner_ExecuteAll` (行455-585)
6. `TestRunner_ExecuteAllWithPriority` (行587-711)
7. `TestRunner_GroupPriority` (行713-817)
8. `TestRunner_DependencyHandling` (行819-987)
9. `TestRunner_ExecuteCommand` (行989-1097)
10. `TestRunner_OutputCapture` (行1099-1244)
11. `TestRunner_OutputCaptureEdgeCases` (行1246-1398)
12. `TestRunner_OutputSizeLimit` (行1400-1524)
13. `TestRunner_CommandTimeout` (行1526-1630)
14. `TestRunner_GroupTimeout` (行1632-1730)
15. `TestRunner_GlobalTimeout` (行1732-1818)
16. `TestRunner_PrivilegedCommand` (行1820-1934)
17. `TestRunner_SecurityValidation` (行1936-2034)
18. `TestRunner_EnvironmentVariables` (行2036-2186)
19. `TestRunner_OutputCaptureErrorCategorization` (行2188-2278)
20. `TestRunner_OutputCaptureErrorHandlingStages` (行2280-2410)
21. `TestRunner_SecurityIntegration` (行2412-2569)

## 移行戦略

### Phase 1: ヘルパーメソッドの準備（優先度：最高）

#### 1.1 テストヘルパーメソッドの実装

`runner_test.go`内に以下のヘルパーメソッドを追加（`MockResourceManager`の型エイリアスの後に配置）：

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

#### 1.2 型変換ヘルパー関数の作成

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

### Phase 2: テスト関数の段階的移行

各テスト関数を個別に移行します。1つのテストが完了したらコミットし、次に進みます。

#### 2.1 TestNewRunner の移行（最優先）

**Before**:
```go
config := &runnertypes.Config{
	Global: runnertypes.GlobalConfig{
		Timeout:  3600,
		WorkDir:  "/tmp",
		LogLevel: "info",
	},
}
```

**After**:
```go
config := &runnertypes.ConfigSpec{
	Global: runnertypes.GlobalSpec{
		Timeout:  3600,
		WorkDir:  "/tmp",
		LogLevel: "info",
	},
}
```

**影響範囲**: 行115-121（7行）

**注意点**:
- `runner.config`の型も変わるため、アサーションを更新
- `ConfigSpec`には`Version`フィールドが必要な場合がある

#### 2.2 TestNewRunnerWithSecurity の移行

**影響範囲**: 行180-221（42行）
**変更内容**: `TestNewRunner`と同様の変更

#### 2.3 TestRunner_ExecuteGroup の移行（重要）

この関数は`CommandGroup`と`Command`を使用しており、複雑な変更が必要です。

**Before**:
```go
group := &runnertypes.CommandGroup{
	Name: "test-group",
	Commands: []runnertypes.Command{
		{
			Name: "cmd1",
			Cmd:  "/bin/echo",
			Args: []string{"hello"},
		},
	},
}
```

**After**:
```go
group := &runnertypes.GroupSpec{
	Name: "test-group",
	Commands: []runnertypes.CommandSpec{
		{
			Name: "cmd1",
			Cmd:  "/bin/echo",
			Args: []string{"hello"},
		},
	},
}
```

**追加作業**:
- `Runner.ExecuteGroup()`は`GroupSpec`と`RuntimeGlobal`を受け取るため、`RuntimeGlobal`の作成が必要
- テスト内で`config.ExpandGlobal()`を呼び出して`RuntimeGlobal`を取得

**コード例**:
```go
// Before
err := runner.ExecuteGroup(ctx, group)

// After
runtimeGlobal, err := config.ExpandGlobal()
require.NoError(t, err)
err = runner.groupExecutor.ExecuteGroup(ctx, &groupSpec, runtimeGlobal)
```

### Phase 3: フィールド名の変更

古い型と新しい型でフィールド名が異なる場合があります：

| 古いフィールド | 新しいフィールド | 備考 |
|--------------|----------------|------|
| `Command.Dir` | `CommandSpec.WorkDir` | 作業ディレクトリ |
| `GlobalConfig.Env` | `GlobalSpec.Env` | 同じ |
| `GlobalConfig.ExpandedEnv` | `RuntimeGlobal.ExpandedEnv` | Runtimeオブジェクトに移動 |

### Phase 4: `PrepareCommand()` の削除

古いコードで使用されている`PrepareCommand()`メソッドは削除されています。

**Before**:
```go
cmd := runnertypes.Command{
	Name: "test",
	Cmd:  "echo",
}
runnertypes.PrepareCommand(&cmd)
```

**After**:
```go
// PrepareCommand() は不要
// ExpandGroup() または ExpandCommand() を使用
cmdSpec := runnertypes.CommandSpec{
	Name: "test",
	Cmd:  "echo",
}
// 必要に応じて config.ExpandCommand() を呼び出す
```

### Phase 5: 検証とテスト

各フェーズ完了後：

```bash
# 個別テスト実行
go test -tags test -v ./internal/runner -run TestNewRunner

# 全テスト実行
make test

# Lint確認
make lint
```

## 詳細な変換マッピング

### Config → ConfigSpec

| フィールド | Config | ConfigSpec | 変更内容 |
|-----------|--------|------------|---------|
| Version | `string` | `string` | 同じ |
| Global | `GlobalConfig` | `GlobalSpec` | 型名変更 |
| Groups | `[]CommandGroup` | `[]GroupSpec` | 型名変更 |

### GlobalConfig → GlobalSpec

| フィールド | GlobalConfig | GlobalSpec | 変更内容 |
|-----------|-------------|-----------|---------|
| Timeout | `int` | `int` | 同じ |
| WorkDir | `string` | `string` | 同じ |
| LogLevel | `string` | `string` | 同じ |
| VerifyFiles | `[]string` | `[]string` | 同じ |
| SkipStandardPaths | `bool` | `bool` | 同じ |
| EnvAllowlist | `[]string` | `[]string` | 同じ |
| MaxOutputSize | `int64` | `int64` | 同じ |
| Env | `[]string` | `[]string` | 同じ |
| FromEnv | `[]string` | `[]string` | 同じ |
| Vars | `[]string` | `[]string` | 同じ |
| ExpandedVerifyFiles | `[]string` | - | RuntimeGlobalに移動 |
| ExpandedEnv | `map[string]string` | - | RuntimeGlobalに移動 |
| ExpandedVars | `map[string]string` | - | RuntimeGlobalに移動 |

### CommandGroup → GroupSpec

| フィールド | CommandGroup | GroupSpec | 変更内容 |
|-----------|-------------|-----------|---------|
| Name | `string` | `string` | 同じ |
| Description | `string` | `string` | 同じ |
| Priority | `int` | `int` | 同じ |
| TempDir | `bool` | - | 削除（未実装） |
| WorkDir | `string` | `string` | 同じ |
| Commands | `[]Command` | `[]CommandSpec` | 型名変更 |
| VerifyFiles | `[]string` | `[]string` | 同じ |
| EnvAllowlist | `[]string` | `[]string` | 同じ |
| Env | `[]string` | `[]string` | 同じ |
| FromEnv | `[]string` | `[]string` | 同じ |
| Vars | `[]string` | `[]string` | 同じ |
| ExpandedVerifyFiles | `[]string` | - | RuntimeGroupに移動 |
| ExpandedEnv | `map[string]string` | - | RuntimeGroupに移動 |
| ExpandedVars | `map[string]string` | - | RuntimeGroupに移動 |

### Command → CommandSpec

| フィールド | Command | CommandSpec | 変更内容 |
|-----------|---------|------------|---------|
| Name | `string` | `string` | 同じ |
| Description | `string` | `string` | 同じ |
| Cmd | `string` | `string` | 同じ |
| Args | `[]string` | `[]string` | 同じ |
| Env | `[]string` | `[]string` | 同じ |
| Dir | `string` | `WorkDir: string` | **フィールド名変更** |
| Timeout | `int` | `int` | 同じ |
| RunAsUser | `string` | `string` | 同じ |
| RunAsGroup | `string` | `string` | 同じ |
| MaxRiskLevel | `string` | `string` | 同じ |
| Output | `string` | `string` | 同じ |
| FromEnv | `[]string` | `[]string` | 同じ |
| Vars | `[]string` | `[]string` | 同じ |
| ExpandedCmd | `string` | - | RuntimeCommandに移動 |
| ExpandedArgs | `[]string` | - | RuntimeCommandに移動 |
| ExpandedEnv | `map[string]string` | - | RuntimeCommandに移動 |
| ExpandedVars | `map[string]string` | - | RuntimeCommandに移動 |
| EffectiveWorkDir | `string` | - | RuntimeCommandに移動 |
| EffectiveTimeout | `int` | - | RuntimeCommandに移動 |

## 移行チェックリスト

### Phase 1: 準備
- [ ] ヘルパーメソッド（`SetupDefaultMockBehavior`等）を追加
- [ ] 型変換ヘルパー関数（`createConfigSpec`等）を追加
- [ ] 最初のテストでヘルパーが動作することを確認

### Phase 2: テスト関数移行（1つずつコミット）
- [ ] TestNewRunner (行114-178)
- [ ] TestNewRunnerWithSecurity (行180-221)
- [ ] TestRunner_ExecuteGroup (行223-331) ⚠️ 複雑
- [ ] TestRunner_ExecuteGroup_ComplexErrorScenarios (行333-453)
- [ ] TestRunner_ExecuteAll (行455-585)
- [ ] TestRunner_ExecuteAllWithPriority (行587-711)
- [ ] TestRunner_GroupPriority (行713-817)
- [ ] TestRunner_DependencyHandling (行819-987) ⚠️ 複雑
- [ ] TestRunner_ExecuteCommand (行989-1097)
- [ ] TestRunner_OutputCapture (行1099-1244)
- [ ] TestRunner_OutputCaptureEdgeCases (行1246-1398)
- [ ] TestRunner_OutputSizeLimit (行1400-1524)
- [ ] TestRunner_CommandTimeout (行1526-1630)
- [ ] TestRunner_GroupTimeout (行1632-1730)
- [ ] TestRunner_GlobalTimeout (行1732-1818)
- [ ] TestRunner_PrivilegedCommand (行1820-1934)
- [ ] TestRunner_SecurityValidation (行1936-2034)
- [ ] TestRunner_EnvironmentVariables (行2036-2186)
- [ ] TestRunner_OutputCaptureErrorCategorization (行2188-2278)
- [ ] TestRunner_OutputCaptureErrorHandlingStages (行2280-2410)
- [ ] TestRunner_SecurityIntegration (行2412-2569)

### Phase 3: 最終確認
- [ ] すべてのテストが個別に PASS
- [ ] `make test` で全テスト PASS
- [ ] `make lint` でエラーなし
- [ ] `skip_integration_tests`タグを削除
- [ ] 古い型定義（Config, GlobalConfig, CommandGroup, Command）の削除を検討

## よくある問題と解決策

### 問題1: `runner.config`の型が一致しない

**エラー**:
```
cannot use config (variable of type *runnertypes.ConfigSpec) as *runnertypes.Config value in argument
```

**解決策**:
`Runner`構造体の`config`フィールドが`*ConfigSpec`を期待しているため、テストでの型も合わせる。

### 問題2: `ExecuteGroup()`のシグネチャ変更

**エラー**:
```
not enough arguments in call to runner.ExecuteGroup
```

**解決策**:
```go
// Before
err := runner.ExecuteGroup(ctx, group)

// After
runtimeGlobal, err := config.ExpandGlobal()
require.NoError(t, err)
err = runner.groupExecutor.ExecuteGroup(ctx, groupSpec, runtimeGlobal)
```

### 問題3: `Dir`フィールドが見つからない

**エラー**:
```
unknown field 'Dir' in struct literal of type runnertypes.CommandSpec
```

**解決策**:
```go
// Before
Dir: "/tmp"

// After
WorkDir: "/tmp"
```

### 問題4: `PrepareCommand()`が存在しない

**エラー**:
```
undefined: runnertypes.PrepareCommand
```

**解決策**:
`PrepareCommand()`の呼び出しを削除。変数展開が必要な場合は`config.ExpandCommand()`を使用。

## 推定作業時間

| フェーズ | 推定時間 | 備考 |
|---------|---------|------|
| Phase 1: ヘルパー準備 | 2-3時間 | ヘルパーメソッドの実装とテスト |
| Phase 2: 簡単なテスト移行（10個） | 4-6時間 | TestNewRunner等 |
| Phase 2: 複雑なテスト移行（11個） | 8-12時間 | ExecuteGroup, Dependency等 |
| Phase 3: 最終確認 | 2-3時間 | 全テスト実行、lint、調整 |
| **合計** | **16-24時間** | **2-3日の作業** |

## 次のステップ

1. このガイドを参照しながらPhase 1から開始
2. 各テスト関数の移行後にコミット
3. 問題が発生したらこのドキュメントを更新
4. すべてのテストが成功したら Task 0037 に進む
