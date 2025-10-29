# 実装計画書: タイムアウト解決コンテキストの強化

## 1. 概要

本ドキュメントは、タイムアウト解決コンテキストの強化機能の実装を4つのフェーズに分けて段階的に進めるための詳細な実装計画を定義する。

## 2. 実装フェーズ

### フェーズ1: ResolveTimeoutWithContextの型安全化
**目的**: `ResolveTimeoutWithContext`を`Timeout`型を直接受け取るように変更し、型安全性を強化。冗長な`ResolveTimeout`ラッパーを削除

### フェーズ2: RuntimeCommandの拡張
**目的**: タイムアウト解決コンテキストを`RuntimeCommand`に統合

### フェーズ3: ResolveEffectiveTimeoutの削除
**目的**: 重複したタイムアウト解決ロジックを削除し、コードを統一

### フェーズ4: Dry-Run統合
**目的**: dry-run出力にタイムアウト解決コンテキストを追加

## 3. フェーズ1: ResolveTimeoutWithContextの型安全化

### 3.1. タスク一覧

#### Task 1.1: ResolveTimeoutWithContextのシグネチャ変更とResolveTimeoutの削除
**ファイル**: `internal/common/timeout_resolver.go`

**変更前**:
```go
func ResolveTimeout(cmdTimeout, groupTimeout, globalTimeout *int) int {
	resolvedValue, _ := ResolveTimeoutWithContext(cmdTimeout, groupTimeout, globalTimeout, "", "")
	return resolvedValue
}

func ResolveTimeoutWithContext(cmdTimeout, groupTimeout, globalTimeout *int, commandName, groupName string) (int, TimeoutResolutionContext) {
	var resolvedValue int
	var level string

	switch {
	case cmdTimeout != nil:
		resolvedValue = *cmdTimeout
		level = "command"
	case groupTimeout != nil:
		resolvedValue = *groupTimeout
		level = "group"
	case globalTimeout != nil:
		resolvedValue = *globalTimeout
		level = "global"
	default:
		resolvedValue = DefaultTimeout
		level = "default"
	}
	// ...
}
```

**変更後**:
```go
// ResolveTimeout関数は削除（冗長なため）

func ResolveTimeoutWithContext(cmdTimeout, groupTimeout, globalTimeout Timeout, commandName, groupName string) (int, TimeoutResolutionContext) {
	var resolvedValue int
	var level string

	switch {
	case cmdTimeout.IsSet():
		resolvedValue = cmdTimeout.Value()
		level = "command"
	case groupTimeout.IsSet():
		resolvedValue = groupTimeout.Value()
		level = "group"
	case globalTimeout.IsSet():
		resolvedValue = globalTimeout.Value()
		level = "global"
	default:
		resolvedValue = DefaultTimeout
		level = "default"
	}

	context := TimeoutResolutionContext{
		CommandName: commandName,
		GroupName:   groupName,
		Level:       level,
	}

	return resolvedValue, context
}
```

**実装位置**: `internal/common/timeout_resolver.go`の既存関数を置き換え

**注意**: `ResolveTimeout`関数は削除し、すべての呼び出し元で`ResolveTimeoutWithContext`を直接使用する。

#### Task 1.2: テストの更新とResolveTimeoutテストの削除
**ファイル**: `internal/common/timeout_resolver_test.go`

**変更内容**: `TestResolveTimeout`を削除し、`TestResolveTimeoutWithContext`に統合

**削除対象**: `TestResolveTimeout`関数全体

**統合先**: `TestResolveTimeoutWithContext`に以下のテストケースを追加:
```go
func TestResolveTimeoutWithContext(t *testing.T) {
	tests := []struct {
		name          string
		cmdTimeout    Timeout
		groupTimeout  Timeout
		globalTimeout Timeout
		want          int
	}{
		{
			name:          "command level timeout",
			cmdTimeout:    NewFromIntPtr(IntPtr(30)),
			groupTimeout:  NewUnsetTimeout(),
			globalTimeout: NewFromIntPtr(IntPtr(60)),
			want:          30,
		},
		{
			name:          "global level timeout",
			cmdTimeout:    NewUnsetTimeout(),
			groupTimeout:  NewUnsetTimeout(),
			globalTimeout: NewFromIntPtr(IntPtr(60)),
			want:          60,
		},
		{
			name:          "default timeout",
			cmdTimeout:    NewUnsetTimeout(),
			groupTimeout:  NewUnsetTimeout(),
			globalTimeout: NewUnsetTimeout(),
			want:          DefaultTimeout,
		},
		{
			name:          "zero timeout (unlimited)",
			cmdTimeout:    NewFromIntPtr(IntPtr(0)),
			groupTimeout:  NewUnsetTimeout(),
			globalTimeout: NewFromIntPtr(IntPtr(60)),
			want:          0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ctx := ResolveTimeoutWithContext(
				tt.cmdTimeout,
				tt.groupTimeout,
				tt.globalTimeout,
				"test-command",
				"test-group",
			)
			assert.Equal(t, tt.want, got)
			// Context fields are validated in other test cases
		})
	}
}
```

**検証方法**:
```bash
go test ./internal/common -run TestResolveTimeoutWithContext -v
```

### 3.2. 完了基準

- [x] `ResolveTimeoutWithContext`のシグネチャが`Timeout`型に変更されている
- [x] `ResolveTimeout`関数が削除されている
- [x] 内部実装が`IsSet()`と`Value()`を使うように更新されている
- [x] `TestResolveTimeout`が削除され、テストケースが`TestResolveTimeoutWithContext`に統合されている
- [x] すべてのテストが成功する
- [x] `go vet ./...`がエラーなく完了する

### 3.3. 所要時間見積もり

**開発**: 1時間
**テスト更新**: 1時間
**合計**: 2時間

## 4. フェーズ2: RuntimeCommandの拡張

### 4.1. タスク一覧

#### Task 2.1: RuntimeCommand構造体の拡張
**ファイル**: `internal/runner/runnertypes/runtime.go`

**実装内容**: `RuntimeCommand`構造体に新しいフィールドを追加

```go
type RuntimeCommand struct {
	// Existing fields...
	Spec             *CommandSpec
	timeout          common.Timeout
	EffectiveTimeout int

	// New field
	TimeoutResolution common.TimeoutResolutionContext

	// Other existing fields...
	ExpandedCmd     string
	ExpandedArgs    []string
	ExpandedEnv     map[string]string
	ExpandedVars    map[string]string
	EffectiveWorkDir string
}
```

**実装位置**: `EffectiveTimeout`フィールドの直後（約175行目付近）

#### Task 2.2: NewRuntimeCommandのシグネチャ変更
**ファイル**: `internal/runner/runnertypes/runtime.go`

**変更前**:
```go
func NewRuntimeCommand(spec *CommandSpec, globalTimeout common.Timeout) (*RuntimeCommand, error)
```

**変更後**:
```go
func NewRuntimeCommand(
	spec *CommandSpec,
	globalTimeout common.Timeout,
	groupName string,
) (*RuntimeCommand, error)
```

#### Task 2.3: NewRuntimeCommandの実装更新
**ファイル**: `internal/runner/runnertypes/runtime.go`

**変更前** (約196-210行):
```go
func NewRuntimeCommand(spec *CommandSpec, globalTimeout common.Timeout) (*RuntimeCommand, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}

	// Resolve the effective timeout using the hierarchy
	commandTimeout := common.NewFromIntPtr(spec.Timeout)
	effectiveTimeout := common.ResolveEffectiveTimeout(commandTimeout, globalTimeout)

	return &RuntimeCommand{
		Spec:             spec,
		timeout:          commandTimeout,
		ExpandedArgs:     []string{},
		ExpandedEnv:      make(map[string]string),
		ExpandedVars:     make(map[string]string),
		EffectiveTimeout: effectiveTimeout,
	}, nil
}
```

**変更後**:
```go
func NewRuntimeCommand(
	spec *CommandSpec,
	globalTimeout common.Timeout,
	groupName string,
) (*RuntimeCommand, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}

	// Resolve the effective timeout using the hierarchy with context
	commandTimeout := common.NewFromIntPtr(spec.Timeout)
	effectiveTimeout, resolutionContext := common.ResolveTimeoutWithContext(
		commandTimeout,
		common.NewUnsetTimeout(),  // Group timeout not yet supported
		globalTimeout,
		spec.Name,
		groupName,
	)

	return &RuntimeCommand{
		Spec:              spec,
		timeout:           commandTimeout,
		ExpandedArgs:      []string{},
		ExpandedEnv:       make(map[string]string),
		ExpandedVars:      make(map[string]string),
		EffectiveTimeout:  effectiveTimeout,
		TimeoutResolution: resolutionContext,
	}, nil
}
```

#### Task 2.4: 呼び出し元の更新 - ExpandCommand
**ファイル**: `internal/runner/config/expansion.go`

**対象箇所**: 約513行目

**変更前**:
```go
runtime, err := runnertypes.NewRuntimeCommand(spec, globalTimeout)
```

**変更後**:
```go
runtime, err := runnertypes.NewRuntimeCommand(spec, globalTimeout, group.Name)
```

**変更箇所の特定**:
```bash
grep -n "NewRuntimeCommand" internal/runner/config/expansion.go
```

#### Task 2.5: 呼び出し元の更新 - テストコード
**ファイル**: 複数のテストファイル

**対象ファイル**:
1. `internal/runner/runnertypes/runtime_test.go`
2. `internal/runner/runnertypes/timeout_test.go`
3. `cmd/runner/integration_timeout_test.go`

**変更内容**: すべての`NewRuntimeCommand`呼び出しに`groupName`引数を追加

**例**:
```go
// Before
runtime, err := NewRuntimeCommand(spec, common.NewUnsetTimeout())

// After
runtime, err := NewRuntimeCommand(spec, common.NewUnsetTimeout(), "test-group")
```

**変更箇所の特定**:
```bash
grep -rn "NewRuntimeCommand" internal/runner/runnertypes/ cmd/runner/integration_timeout_test.go
```

#### Task 2.6: 単体テストの追加
**ファイル**: `internal/runner/runnertypes/timeout_test.go`

**実装内容**: `TimeoutResolution`フィールドのテスト

```go
func TestNewRuntimeCommand_TimeoutResolutionContext(t *testing.T) {
	tests := []struct {
		name          string
		cmdTimeout    *int
		globalTimeout *int
		commandName   string
		groupName     string
		wantValue     int
		wantLevel     string
	}{
		{
			name:          "command level resolution",
			cmdTimeout:    common.IntPtr(30),
			globalTimeout: common.IntPtr(60),
			commandName:   "test-cmd",
			groupName:     "test-group",
			wantValue:     30,
			wantLevel:     "command",
		},
		{
			name:          "global level resolution",
			cmdTimeout:    nil,
			globalTimeout: common.IntPtr(60),
			commandName:   "test-cmd",
			groupName:     "test-group",
			wantValue:     60,
			wantLevel:     "global",
		},
		{
			name:          "default level resolution",
			cmdTimeout:    nil,
			globalTimeout: nil,
			commandName:   "test-cmd",
			groupName:     "test-group",
			wantValue:     common.DefaultTimeout,
			wantLevel:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &CommandSpec{
				Name:    tt.commandName,
				Cmd:     "/bin/echo",
				Timeout: tt.cmdTimeout,
			}

			runtime, err := NewRuntimeCommand(
				spec,
				common.NewFromIntPtr(tt.globalTimeout),
				tt.groupName,
			)

			assert.NoError(t, err, "NewRuntimeCommand should not fail")
			assert.Equal(t, tt.wantValue, runtime.EffectiveTimeout, "effective timeout should match")
			assert.Equal(t, tt.wantLevel, runtime.TimeoutResolution.Level, "resolution level should match")
			assert.Equal(t, tt.commandName, runtime.TimeoutResolution.CommandName, "command name should match")
			assert.Equal(t, tt.groupName, runtime.TimeoutResolution.GroupName, "group name should match")
		})
	}
}
```

**検証方法**:
```bash
go test ./internal/runner/runnertypes -run TestNewRuntimeCommand_TimeoutResolutionContext -v
```

### 4.2. 完了基準

- [x] `RuntimeCommand`に`TimeoutResolution`フィールドが追加されている
- [x] `NewRuntimeCommand`のシグネチャが更新されている
- [x] `NewRuntimeCommand`の実装が`ResolveTimeoutWithContext`を使用している
- [x] `ExpandCommand`での呼び出しが更新されている
- [x] すべてのテストコードが更新されている
- [x] 新しい単体テストが追加されている
- [x] すべてのテストが成功する
- [x] コンパイルエラーがない

### 4.3. 所要時間見積もり

**開発**: 2時間
**テスト更新**: 1.5時間
**新規テスト作成**: 1時間
**合計**: 4.5時間

## 5. フェーズ3: ResolveEffectiveTimeoutの削除

### 5.1. 事前調査

#### Task 3.1: ResolveEffectiveTimeout使用箇所の特定

**コマンド**:
```bash
grep -rn "ResolveEffectiveTimeout" internal/ cmd/
```

**予想される結果**:
- `internal/common/timeout.go`: 関数定義
- `internal/common/timeout_test.go`: テストコード
- `internal/runner/resource/normal_manager_test.go`: テスト内での使用
- `internal/runner/runnertypes/runtime.go`: コメント内での言及

### 5.2. タスク一覧

#### Task 3.2: normal_manager_test.goの更新
**ファイル**: `internal/runner/resource/normal_manager_test.go`

**対象箇所**: 約203行目

**変更前**:
```go
effectiveTimeout := common.ResolveEffectiveTimeout(commandTimeout, globalTimeout)
```

**変更後**:
```go
effectiveTimeout := common.ResolveTimeout(
	commandTimeout,
	common.NewUnsetTimeout(),  // group timeout
	globalTimeout,
)
```

**変更後（コンテキスト情報あり）**:
```go
effectiveTimeout, _ := common.ResolveTimeoutWithContext(
	commandTimeout,
	common.NewUnsetTimeout(),
	globalTimeout,
	"", // command name not needed for test
	"", // group name not needed for test
)
```

**注意**: `ResolveTimeout`ラッパー関数は削除されたため、すべての箇所で`ResolveTimeoutWithContext`を使用する。

#### Task 3.3: コメントの更新
**ファイル**: `internal/runner/runnertypes/runtime.go`

**対象箇所**: 約63行目

**変更前**:
```go
// Use common.ResolveEffectiveTimeout() to resolve the effective timeout with proper fallback logic.
```

**変更後**:
```go
// Use common.ResolveTimeoutWithContext() to resolve the effective timeout with proper fallback logic.
```

#### Task 3.4: ResolveEffectiveTimeoutテストの移植
**ファイル**: `internal/common/timeout_resolver_test.go`

**実装内容**: `timeout_test.go`にある`TestResolveEffectiveTimeout`のテストケースを`timeout_resolver_test.go`の`TestResolveTimeoutWithContext`に統合

**移植するテストケース** (from `internal/common/timeout_test.go:279-337`):
```go
// These test cases should be added to TestResolveTimeoutWithContext in timeout_resolver_test.go
{
	name:            "command timeout takes precedence (Timeout type)",
	cmdTimeout:      common.NewFromIntPtr(common.IntPtr(30)),
	groupTimeout:    common.NewUnsetTimeout(),
	globalTimeout:   common.NewFromIntPtr(common.IntPtr(60)),
	want:            30,
},
{
	name:            "global timeout used when command not set (Timeout type)",
	cmdTimeout:      common.NewUnsetTimeout(),
	groupTimeout:    common.NewUnsetTimeout(),
	globalTimeout:   common.NewFromIntPtr(common.IntPtr(60)),
	want:            60,
},
{
	name:            "default used when both unset (Timeout type)",
	cmdTimeout:      common.NewUnsetTimeout(),
	groupTimeout:    common.NewUnsetTimeout(),
	globalTimeout:   common.NewUnsetTimeout(),
	want:            DefaultTimeout,
},
{
	name:            "zero command timeout is respected (Timeout type)",
	cmdTimeout:      common.NewFromIntPtr(common.IntPtr(0)),
	groupTimeout:    common.NewUnsetTimeout(),
	globalTimeout:   common.NewFromIntPtr(common.IntPtr(60)),
	want:            0,
},
{
	name:            "zero global timeout is respected (Timeout type)",
	cmdTimeout:      common.NewUnsetTimeout(),
	groupTimeout:    common.NewUnsetTimeout(),
	globalTimeout:   common.NewFromIntPtr(common.IntPtr(0)),
	want:            0,
},
```

**注意**: これらのテストケースは既に`TestResolveTimeoutWithContext`に含まれている可能性があるため、重複を確認して統合する。

#### Task 3.5: ResolveEffectiveTimeout関数の削除
**ファイル**: `internal/common/timeout.go`

**削除箇所**: 約97-120行目

**削除対象**:
```go
// ResolveEffectiveTimeout determines the effective timeout value using the priority chain:
// 1. Command-level timeout (if set)
// 2. Global timeout (if set)
// 3. DefaultTimeout constant (60 seconds)
//
// This function encapsulates the timeout resolution logic used throughout the command runner,
// ensuring consistent behavior in both production code and tests.
//
// Parameters:
//   - commandTimeout: The command-specific timeout (may be unset)
//   - globalTimeout: The global timeout (may be unset)
//
// Returns:
//   - The resolved timeout value in seconds
func ResolveEffectiveTimeout(commandTimeout, globalTimeout Timeout) int {
	if commandTimeout.IsSet() {
		return commandTimeout.Value()
	}
	if globalTimeout.IsSet() {
		return globalTimeout.Value()
	}
	return DefaultTimeout
}
```

**検証**: コンパイルエラーが発生しないことを確認
```bash
go build ./...
```

#### Task 3.6: ResolveEffectiveTimeoutテストの削除
**ファイル**: `internal/common/timeout_test.go`

**削除箇所**: 約279-337行目

**削除対象**: `TestResolveEffectiveTimeout`関数全体

**検証**:
```bash
go test ./internal/common -v
```

### 5.3. 完了基準

- [x] `ResolveEffectiveTimeout`の全使用箇所が`ResolveTimeoutWithContext`に置き換えられている
- [x] 関連するコメントが更新されている
- [x] `ResolveEffectiveTimeout`のテストケースが`ResolveTimeoutWithContext`に統合されている
- [x] `ResolveEffectiveTimeout`関数が削除されている
- [x] `TestResolveEffectiveTimeout`が削除されている
- [x] すべてのテストが成功する
- [x] `go build ./...`がエラーなく完了する
- [x] `grep -r "ResolveEffectiveTimeout" .`で残存がないことを確認

### 5.4. 所要時間見積もり

**事前調査**: 30分
**コード更新**: 1時間
**テスト移植**: 1時間
**検証**: 30分
**合計**: 3時間

## 6. フェーズ4: Dry-Run統合

### 6.1. タスク一覧

#### Task 4.1: analyzeCommandの更新
**ファイル**: `internal/runner/resource/dryrun_manager.go`

**対象箇所**: 約163-175行目

**変更前**:
```go
analysis := ResourceAnalysis{
	Type:      ResourceTypeCommand,
	Operation: OperationExecute,
	Target:    cmd.ExpandedCmd,
	Parameters: map[string]any{
		"command":           cmd.ExpandedCmd,
		"working_directory": cmd.EffectiveWorkDir,
		"timeout":           cmd.Timeout(),
	},
	Impact: ResourceImpact{
		Reversible:  false,
		Persistent:  true,
		Description: fmt.Sprintf("Execute command: %s", cmd.ExpandedCmd),
	},
	Timestamp: time.Now(),
}
```

**変更後**:
```go
analysis := ResourceAnalysis{
	Type:      ResourceTypeCommand,
	Operation: OperationExecute,
	Target:    cmd.ExpandedCmd,
	Parameters: map[string]any{
		"command":           cmd.ExpandedCmd,
		"working_directory": cmd.EffectiveWorkDir,
		"timeout":           cmd.EffectiveTimeout,
		"timeout_level":     cmd.TimeoutResolution.Level,
	},
	Impact: ResourceImpact{
		Reversible:  false,
		Persistent:  true,
		Description: fmt.Sprintf("Execute command: %s", cmd.ExpandedCmd),
	},
	Timestamp: time.Now(),
}
```

**変更内容**:
1. `cmd.Timeout()`を`cmd.EffectiveTimeout`に修正
2. `"timeout_level": cmd.TimeoutResolution.Level`を追加

#### Task 4.2: 統合テストの作成
**ファイル**: `cmd/runner/integration_timeout_test.go`（既存ファイルに追加）

**実装内容**: dry-run出力にタイムアウトコンテキストが含まれることを確認

```go
func TestDryRun_TimeoutResolutionContext(t *testing.T) {
	tests := []struct {
		name               string
		configContent      string
		expectedTimeout    int
		expectedLevel      string
	}{
		{
			name: "command level timeout in dry-run",
			configContent: `
[global]
timeout = 60

[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/sleep"
args = ["1"]
timeout = 30
`,
			expectedTimeout: 30,
			expectedLevel:   "command",
		},
		{
			name: "global level timeout in dry-run",
			configContent: `
[global]
timeout = 45

[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/sleep"
args = ["1"]
`,
			expectedTimeout: 45,
			expectedLevel:   "global",
		},
		{
			name: "default timeout in dry-run",
			configContent: `
[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/sleep"
args = ["1"]
`,
			expectedTimeout: 60,
			expectedLevel:   "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpFile, err := os.CreateTemp("", "test-config-*.toml")
			require.NoError(t, err)
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(tt.configContent)
			require.NoError(t, err)
			tmpFile.Close()

			// Run command in dry-run mode
			cmd := exec.Command("go", "run", ".", "-config", tmpFile.Name(), "-dry-run")
			cmd.Dir = "../../cmd/runner"

			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "dry-run should succeed")

			outputStr := string(output)

			// Check for timeout value
			assert.Contains(t, outputStr, fmt.Sprintf("timeout: %d", tt.expectedTimeout),
				"output should contain timeout value")

			// Check for timeout_level
			assert.Contains(t, outputStr, fmt.Sprintf("timeout_level: %s", tt.expectedLevel),
				"output should contain timeout_level")
		})
	}
}
```

**検証方法**:
```bash
go test ./cmd/runner -run TestDryRun_TimeoutResolutionContext -v
```

#### Task 4.3: JSON出力のテスト
**ファイル**: 新規作成 `cmd/runner/integration_dryrun_json_test.go`

**実装内容**: JSON形式のdry-run出力にタイムアウトコンテキストが含まれることを確認

```go
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDryRun_JSON_TimeoutResolutionContext(t *testing.T) {
	configContent := `
[global]
timeout = 60

[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
timeout = 30
`

	// Create temporary config file
	tmpFile, err := os.CreateTemp("", "test-config-*.toml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Run command in dry-run mode with JSON output
	cmd := exec.Command("go", "run", ".", "-config", tmpFile.Name(), "-dry-run", "-format", "json")
	cmd.Dir = "../../cmd/runner"

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "dry-run should succeed")

	// Parse JSON output
	var result struct {
		ResourceAnalyses []struct {
			Type       string         `json:"type"`
			Parameters map[string]any `json:"parameters"`
		} `json:"resource_analyses"`
	}

	err = json.Unmarshal(output, &result)
	require.NoError(t, err, "output should be valid JSON")

	// Find command analysis
	require.NotEmpty(t, result.ResourceAnalyses, "should have at least one analysis")

	cmdAnalysis := result.ResourceAnalyses[0]
	assert.Equal(t, "command", cmdAnalysis.Type)

	// Check timeout parameters
	timeout, ok := cmdAnalysis.Parameters["timeout"]
	require.True(t, ok, "parameters should contain timeout")
	assert.Equal(t, float64(30), timeout, "timeout should be 30")

	timeoutLevel, ok := cmdAnalysis.Parameters["timeout_level"]
	require.True(t, ok, "parameters should contain timeout_level")
	assert.Equal(t, "command", timeoutLevel, "timeout_level should be 'command'")
}
```

**検証方法**:
```bash
go test ./cmd/runner -run TestDryRun_JSON_TimeoutResolutionContext -v
```

#### Task 4.4: 手動テストシナリオの実行

**テストケース1: コマンドレベルタイムアウト**

設定ファイル: `/tmp/test-command-timeout.toml`
```toml
[global]
timeout = 60

[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/sleep"
args = ["1"]
timeout = 30
```

実行:
```bash
go run ./cmd/runner -config /tmp/test-command-timeout.toml -dry-run
```

期待される出力:
```
=== RESOURCE OPERATIONS ===
1. Execute command: /bin/sleep [command]
   Operation: execute
   Target: /bin/sleep
   Parameters:
     command: /bin/sleep
     working_directory: /home/user/project
     timeout: 30
     timeout_level: command
```

**テストケース2: グローバルレベルタイムアウト**

設定ファイル: `/tmp/test-global-timeout.toml`
```toml
[global]
timeout = 45

[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/sleep"
args = ["1"]
```

実行:
```bash
go run ./cmd/runner -config /tmp/test-global-timeout.toml -dry-run
```

期待される出力:
```
Parameters:
  timeout: 45
  timeout_level: global
```

**テストケース3: デフォルトタイムアウト**

設定ファイル: `/tmp/test-default-timeout.toml`
```toml
[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/sleep"
args = ["1"]
```

実行:
```bash
go run ./cmd/runner -config /tmp/test-default-timeout.toml -dry-run
```

期待される出力:
```
Parameters:
  timeout: 60
  timeout_level: default
```

**テストケース4: JSON出力**

実行:
```bash
go run ./cmd/runner -config /tmp/test-command-timeout.toml -dry-run -format json | jq .
```

期待される出力:
```json
{
  "resource_analyses": [
    {
      "type": "command",
      "operation": "execute",
      "target": "/bin/sleep",
      "parameters": {
        "command": "/bin/sleep",
        "working_directory": "/home/user/project",
        "timeout": 30,
        "timeout_level": "command"
      }
    }
  ]
}
```

### 6.2. 完了基準

- [ ] `analyzeCommand`の`Parameters`に`timeout_level`が追加されている
- [ ] `timeout`フィールドが`EffectiveTimeout`を使用している（int値）
- [ ] 統合テストが追加されている
- [ ] JSON出力のテストが追加されている
- [ ] すべての自動テストが成功する
- [ ] 手動テストシナリオが全て期待通りに動作する
- [ ] テキスト出力とJSON出力の両方で`timeout_level`が正しく表示される

### 6.3. 所要時間見積もり

**コード更新**: 30分
**統合テスト作成**: 2時間
**JSON出力テスト作成**: 1時間
**手動テスト**: 1時間
**合計**: 4.5時間

## 7. 総合テストとドキュメント

### 7.1. タスク一覧

#### Task 7.1: 全体的な回帰テストの実行

**実行コマンド**:
```bash
# All unit tests
go test ./... -v

# Specific integration tests
go test ./cmd/runner -run "TestDryRun|Integration.*Timeout" -v

# Build verification
go build ./...

# Vet check
go vet ./...

# Test coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

#### Task 7.2: サンプル設定ファイルの追加
**ファイル**: `sample/timeout_resolution_demo.toml`

**実装内容**:
```toml
# Timeout Resolution Demonstration
# This configuration demonstrates how timeout values are resolved
# and displayed in dry-run mode.

[global]
# Global timeout applies to all commands unless overridden
timeout = 60

[[groups]]
name = "default-timeout-demo"

[[groups.commands]]
name = "uses-default"
cmd = "/bin/sleep"
args = ["1"]
# No timeout specified - will use global (60s)
# dry-run will show: timeout_level: global

[[groups.commands]]
name = "uses-command-timeout"
cmd = "/bin/sleep"
args = ["5"]
timeout = 30
# Command-level timeout overrides global
# dry-run will show: timeout_level: command

[[groups]]
name = "no-global-timeout-demo"

[[groups.commands]]
name = "uses-system-default"
cmd = "/bin/sleep"
args = ["1"]
# Neither command nor global timeout specified
# Will fall back to system default (60s)
# dry-run will show: timeout_level: default

[[groups.commands]]
name = "unlimited-execution"
cmd = "/bin/sleep"
args = ["10"]
timeout = 0
# Zero timeout means unlimited execution
# dry-run will show: timeout: 0, timeout_level: command
```

#### Task 7.3: READMEの更新
**ファイル**: `README.md` および `README.ja.md`

**追加セクション**:
```markdown
### Timeout Resolution Debugging

When running in dry-run mode, the tool displays not only the effective timeout value
but also the level at which it was configured:

```bash
$ go run ./cmd/runner -config sample/timeout_resolution_demo.toml -dry-run
```

Output:
```
=== RESOURCE OPERATIONS ===
1. Execute command: /bin/sleep [command]
   Parameters:
     timeout: 30
     timeout_level: command  # Configured at command level

2. Execute command: /bin/sleep [command]
   Parameters:
     timeout: 60
     timeout_level: global   # Inherited from global configuration

3. Execute command: /bin/sleep [command]
   Parameters:
     timeout: 60
     timeout_level: default  # Using system default
```

This feature helps you understand the timeout resolution hierarchy:
1. Command-level timeout (highest priority)
2. Group-level timeout (not yet implemented)
3. Global timeout
4. Default timeout (60 seconds)
```

#### Task 7.4: CHANGELOGの更新
**ファイル**: `CHANGELOG.md`

**追加内容**:
```markdown
## [Unreleased]

### Added
- Timeout resolution context in dry-run mode
  - Display `timeout_level` field showing where timeout was configured
  - Show resolved timeout value (int) instead of internal structure
  - Support for both text and JSON output formats

### Changed
- Enhanced type safety of timeout resolution
  - `ResolveTimeout` and `ResolveTimeoutWithContext` now accept `Timeout` type directly instead of `*int`
  - Eliminated unnecessary pointer conversions
- Unified timeout resolution logic to use `ResolveTimeoutWithContext`
- Removed duplicate `ResolveEffectiveTimeout` function
- Updated `NewRuntimeCommand` to accept group name for better context

### Fixed
- Fixed dry-run output showing incorrect timeout value (internal structure instead of resolved int)

### Internal
- Added `TimeoutResolution` field to `RuntimeCommand` structure
- Enhanced timeout resolution testing with `Timeout` type
```

### 7.2. 完了基準

- [x] すべてのテストが成功する（`go test ./... -v`）
- [x] ビルドが成功する（`go build ./...`）
- [x] `go vet`がエラーなく完了する
- [x] テストカバレッジが低下していない
- [ ] サンプル設定ファイルが追加されている
- [ ] READMEが更新されている
- [ ] CHANGELOGが更新されている

### 7.3. 所要時間見積もり

**テスト実行**: 1時間
**サンプル作成**: 30分
**ドキュメント更新**: 1時間
**合計**: 2.5時間

## 8. リスク管理と緩和策

### 8.1. 特定されたリスク

#### リスク1: API変更による既存コードへの影響
**深刻度**: 中
**発生確率**: 高

**緩和策**:
- コンパイラがエラーを検出するため、見落としのリスクは低い
- フェーズ2で慎重に全呼び出し箇所を更新
- テストを実行して動作確認

#### リスク2: ResolveEffectiveTimeout削除時の見落とし
**深刻度**: 低
**発生確率**: 低

**緩和策**:
- `grep`で全箇所を事前確認
- コンパイルエラーで確実に検出可能
- テストの実行で動作確認

#### リスク3: テストカバレッジの低下
**深刻度**: 中
**発生確率**: 中

**緩和策**:
- フェーズ1でテストを拡充
- 各フェーズでテストカバレッジを確認
- 統合テストで実際の動作を確認

#### リスク4: 手動テストの不足
**深刻度**: 低
**発生確率**: 中

**緩和策**:
- フェーズ4で詳細な手動テストシナリオを実行
- テキスト出力とJSON出力の両方を確認
- サンプル設定ファイルで実際の動作を確認

### 8.2. ロールバック計画

各フェーズは独立しているため、問題が発生した場合は該当フェーズのみをロールバック可能。

**ロールバック手順**:
```bash
# Changes in current branch
git status

# View specific changes
git diff HEAD~1

# Rollback if needed
git reset --hard HEAD~1

# Or create a revert commit
git revert HEAD
```

## 9. スケジュールと工数見積もり

### 9.1. 工数サマリー

| フェーズ | 開発 | テスト | ドキュメント | 合計 |
|---------|------|--------|-------------|------|
| フェーズ1 | 1.0h | 1.0h | - | 2.0h |
| フェーズ2 | 2.0h | 2.5h | - | 4.5h |
| フェーズ3 | 1.5h | 1.5h | - | 3.0h |
| フェーズ4 | 0.5h | 3.0h | 1.0h | 4.5h |
| 総合テスト | - | 1.0h | 1.5h | 2.5h |
| **合計** | **5.0h** | **9.0h** | **2.5h** | **16.5h** |

### 9.2. 実装スケジュール（目安）

**1日目（4時間）**:
- フェーズ1完了（2時間）
- フェーズ2開始（2時間）

**2日目（4時間）**:
- フェーズ2完了（2.5時間）
- フェーズ3開始（1.5時間）

**3日目（4時間）**:
- フェーズ3完了（1.5時間）
- フェーズ4開始（2.5時間）

**4日目（4.5時間）**:
- フェーズ4完了（2時間）
- 総合テストとドキュメント（2.5時間）

**合計**: 4日間（16.5時間）

## 10. チェックリスト

### 10.1. フェーズ1チェックリスト
- [x] `ResolveTimeoutWithContext`シグネチャ変更
- [x] `ResolveTimeout`関数削除
- [x] 内部実装を`IsSet()`/`Value()`使用に更新
- [x] `TestResolveTimeout`削除とテストケース統合
- [x] 全テスト成功
- [x] `go vet`クリア

### 10.2. フェーズ2チェックリスト
- [x] `RuntimeCommand.TimeoutResolution`フィールド追加
- [x] `NewRuntimeCommand`シグネチャ変更
- [x] `NewRuntimeCommand`実装更新
- [x] `ExpandCommand`更新
- [x] 全テストコード更新
- [x] 新規単体テスト追加
- [x] 全テスト成功
- [x] コンパイル成功

### 10.3. フェーズ3チェックリスト
- [x] `ResolveEffectiveTimeout`使用箇所特定
- [x] `normal_manager_test.go`更新
- [x] コメント更新
- [x] テストケース移植
- [x] `ResolveEffectiveTimeout`関数削除
- [x] `TestResolveEffectiveTimeout`削除
- [x] 全テスト成功
- [x] `go build`成功
- [x] 残存確認（grep）

### 10.4. フェーズ4チェックリスト
- [ ] `analyzeCommand`更新
- [ ] 統合テスト実装
- [ ] JSON出力テスト実装
- [ ] 手動テストシナリオ実行
- [ ] 全テスト成功

### 10.5. 総合チェックリスト
- [ ] 全単体テスト成功
- [ ] 全統合テスト成功
- [ ] ビルド成功
- [ ] `go vet`クリア
- [ ] カバレッジ確認
- [ ] サンプルファイル追加
- [ ] README更新
- [ ] CHANGELOG更新

## 11. 付録

### 11.1. 便利なコマンド集

```bash
# Specific package tests
go test ./internal/common -v
go test ./internal/runner/runnertypes -v
go test ./cmd/runner -v

# Coverage report
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep -E "total|timeout"

# Find TODO/FIXME comments
grep -rn "TODO\|FIXME" internal/ cmd/ | grep -i timeout

# Check for specific function usage
grep -rn "ResolveEffectiveTimeout" .
grep -rn "ResolveTimeout" . | grep -v "ResolveEffectiveTimeout"
grep -rn "NewRuntimeCommand" .

# Build and test specific command
go build ./cmd/runner
./cmd/runner -config sample/timeout_resolution_demo.toml -dry-run

# Format and vet
go fmt ./...
go vet ./...
```

### 11.2. デバッグのヒント

**コンパイルエラーが発生した場合**:
```bash
# Show detailed error
go build -v ./...

# Check specific package
go build ./internal/runner/runnertypes
```

**テストが失敗した場合**:
```bash
# Run with verbose output
go test ./... -v

# Run specific test
go test ./internal/common -run TestTimeout_ToIntPtr -v

# Show test coverage for specific file
go test ./internal/common -coverprofile=coverage.out
go tool cover -html=coverage.out
```

**dry-run出力を確認する場合**:
```bash
# Text output
go run ./cmd/runner -config sample/timeout_resolution_demo.toml -dry-run

# JSON output with pretty print
go run ./cmd/runner -config sample/timeout_resolution_demo.toml -dry-run -format json | jq .

# Save output to file
go run ./cmd/runner -config sample/timeout_resolution_demo.toml -dry-run > /tmp/dryrun-output.txt
```

### 11.3. レビューポイント

コードレビュー時に確認すべきポイント：

1. **型安全性**
   - `ResolveTimeout`/`ResolveTimeoutWithContext`が`Timeout`型を受け取っているか
   - `IsSet()`と`Value()`が適切に使用されているか
   - `TimeoutResolutionContext`の各フィールドが正しく設定されているか

2. **テストカバレッジ**
   - テストが`Timeout`型を使用するように更新されているか
   - エッジケース（未設定、0、正の値など）がカバーされているか

3. **後方互換性**
   - 既存のテストが全て成功するか
   - 既存の動作が変わっていないか

4. **ドキュメント**
   - コメントが英語で書かれているか
   - 新機能の説明がREADMEに追加されているか

5. **コードスタイル**
   - `go fmt`が適用されているか
   - `go vet`の警告がないか

## 12. まとめ

本実装計画書は、タイムアウト解決コンテキストの強化機能を4つのフェーズで段階的に実装するための詳細な手順を提供する。各フェーズは独立しており、問題が発生した場合はロールバックが容易である。

**重要なポイント**:
1. フェーズ1で`ResolveTimeoutWithContext`の型安全性を強化し、冗長な`ResolveTimeout`を削除
2. フェーズ2でAPIを変更し、すべての呼び出し元を更新する
3. フェーズ3で重複コードを削除し、保守性を向上させる
4. フェーズ4でユーザーに見える機能を追加する

**型安全性とコードの簡潔性**:
- `*int`への変換を排除し、`Timeout`型の意味を保持
- `IsSet()`と`Value()`メソッドで明確な意図を表現
- 冗長な`ResolveTimeout`ラッパーを削除し、単一関数に統一
- すべての呼び出し元で明示的にコンテキスト情報を扱う
- コンパイル時の型チェックで安全性を確保

総工数は約16.5時間で、4日間での完了を目指す。各フェーズで十分なテストを行い、品質を確保する。
