# グループ展開時のコマンド事前展開 - 詳細仕様書

## 1. 仕様概要

### 1.1 目的

グループ展開時のコマンド事前展開機能の詳細な実装仕様を定義し、開発者が実装時に参照できる技術的な詳細を提供する。

### 1.2 適用範囲

- 関数のシグネチャと実装仕様
- コード変更の詳細
- エラーハンドリング仕様
- テストケース
- パフォーマンス計測方法

## 2. 関数仕様

### 2.1 ExecuteGroup の変更

#### 2.1.1 シグネチャ（変更なし）

**ファイル**: `internal/runner/group_executor.go`

```go
// ExecuteGroup executes all commands in a group sequentially
func (ge *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) error
```

#### 2.1.2 実装変更

**追加位置**: `__runner_workdir` 設定後、`verifyGroupFiles` 呼び出し前

```go
func (ge *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) error {
    // ... 既存処理 ...

    // 5. Set __runner_workdir variable for use in commands
    if runtimeGroup.ExpandedVars == nil {
        runtimeGroup.ExpandedVars = make(map[string]string)
    }
    runtimeGroup.ExpandedVars[variable.WorkDirKey()] = workDir

    // ★ 6. Pre-expand all commands (NEW) ★
    if err := ge.preExpandCommands(groupSpec, runtimeGroup, runtimeGlobal); err != nil {
        return fmt.Errorf("failed to pre-expand commands for group[%s]: %w", groupSpec.Name, err)
    }

    // 7. Verify group files before execution
    if err := ge.verifyGroupFiles(runtimeGroup); err != nil {
        return err
    }

    // 8. Execute commands in the group sequentially
    commandResults, errResult, err := ge.executeAllCommands(ctx, groupSpec, runtimeGroup, runtimeGlobal)
    // ... 残りの処理 ...
}
```

### 2.2 preExpandCommands 関数（新規）

#### 2.2.1 シグネチャ

```go
// preExpandCommands expands all commands in a group before execution.
//
// This function processes all commands in the group and stores the expanded
// RuntimeCommand instances in runtimeGroup.Commands. This enables:
//   - Consistent variable access during both verification and execution
//   - Access to command-level variables (vars, env_import) during verification
//   - Single point of expansion for better error detection (Fail Fast)
//   - Early detection of workdir configuration errors (Fail Fast)
//
// The function performs the following for each command:
//   1. Expand command configuration (cmd, args, env, vars) via config.ExpandCommand
//   2. Resolve effective working directory via resolveCommandWorkDir
//   3. Store the fully-expanded RuntimeCommand in runtimeGroup.Commands
//
// Parameters:
//   - groupSpec: The group specification containing command definitions
//   - runtimeGroup: The runtime group to store expanded commands
//   - runtimeGlobal: The global runtime configuration
//
// Returns:
//   - error: An error if any command expansion or workdir resolution fails
func (ge *DefaultGroupExecutor) preExpandCommands(
    groupSpec *runnertypes.GroupSpec,
    runtimeGroup *runnertypes.RuntimeGroup,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) error
```

#### 2.2.2 実装詳細

```go
func (ge *DefaultGroupExecutor) preExpandCommands(
    groupSpec *runnertypes.GroupSpec,
    runtimeGroup *runnertypes.RuntimeGroup,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) error {
    // Allocate slice for expanded commands
    runtimeGroup.Commands = make([]*runnertypes.RuntimeCommand, 0, len(groupSpec.Commands))

    // Get global output size limit for command expansion
    globalOutputSizeLimit := common.NewOutputSizeLimitFromPtr(runtimeGlobal.Spec.OutputSizeLimit)

    for i := range groupSpec.Commands {
        cmdSpec := &groupSpec.Commands[i]

        // Expand command configuration
        runtimeCmd, err := config.ExpandCommand(
            cmdSpec,
            runtimeGroup,
            runtimeGlobal,
            runtimeGlobal.Timeout(),
            globalOutputSizeLimit,
        )
        if err != nil {
            return fmt.Errorf("command[%s] (index %d): %w", cmdSpec.Name, i, err)
        }

        // Resolve effective working directory (Fail Fast)
        // Note: This was previously done in executeAllCommands loop,
        //       but moving it here enables earlier error detection.
        workDir, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
        if err != nil {
            return fmt.Errorf("command[%s] (index %d): failed to resolve workdir: %w", cmdSpec.Name, i, err)
        }
        runtimeCmd.EffectiveWorkDir = workDir

        runtimeGroup.Commands = append(runtimeGroup.Commands, runtimeCmd)
    }

    return nil
}
```

#### 2.2.3 resolveCommandWorkDir の呼び出しタイミング変更

**変更理由**: Fail Fast原則の徹底

**現状の問題**:
- `resolveCommandWorkDir`は現在`executeAllCommands`ループ内で呼ばれる
- `workdir`設定に未定義変数が含まれていた場合、エラーは**実行時**に検出される
- 複数コマンドがある場合、最初のコマンド実行時にエラーが発覚し、それ以降のコマンドは実行されない

**改善内容**:
```go
// 現在: executeAllCommands 内（group_executor.go:245-255）
for i := range groupSpec.Commands {
    cmdSpec := &groupSpec.Commands[i]
    runtimeCmd, err := config.ExpandCommand(...)

    // ★ ここで workdir 解決 ★
    workDir, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
    if err != nil {
        return ..., fmt.Errorf("failed to resolve command workdir[%s]: %w", ...)
    }
    runtimeCmd.EffectiveWorkDir = workDir

    // コマンド実行
    stdout, stderr, exitCode, err := ge.executeSingleCommand(...)
}

// 改善後: preExpandCommands 内（新規実装）
for i := range groupSpec.Commands {
    cmdSpec := &groupSpec.Commands[i]
    runtimeCmd, err := config.ExpandCommand(...)

    // ★ 事前展開フェーズで workdir 解決 ★
    workDir, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
    if err != nil {
        return fmt.Errorf("command[%s] (index %d): failed to resolve workdir: %w", ...)
    }
    runtimeCmd.EffectiveWorkDir = workDir

    runtimeGroup.Commands = append(runtimeGroup.Commands, runtimeCmd)
}
```

**メリット**:
1. **Fail Fast**: workdir設定ミスを実行前（検証前）に検出
2. **一貫性**: `EffectiveTimeout`と同様に`EffectiveWorkDir`も事前展開フェーズで確定
3. **完全性**: `RuntimeCommand`が完全に展開された状態で検証・実行フェーズに渡される

**実装可能性**:
- ✅ `resolveCommandWorkDir`は`runtimeCmd.ExpandedVars`と`runtimeGroup`を必要とする
- ✅ これらは`preExpandCommands`内で利用可能（`config.ExpandCommand`後）
- ✅ 依存関係に問題なし

**影響範囲**:
- `executeAllCommands`から`resolveCommandWorkDir`呼び出しを削除
- エラー発生タイミングが「実行時」から「事前展開時」に変更

### 2.3 collectVerificationFiles の変更

#### 2.3.1 シグネチャ（変更なし）

**ファイル**: `internal/verification/manager.go`

```go
// collectVerificationFiles collects all files to verify for a group
func (m *Manager) collectVerificationFiles(
    runtimeGroup *runnertypes.RuntimeGroup,
) map[string]struct{}
```

#### 2.3.2 実装変更

**変更概要**:
既存の`groupSpec.Commands`をループして変数展開とパス解決を行う処理を**完全に削除**し、事前展開済みの`runtimeGroup.Commands`をループする処理に**置き換える**。

**削除される処理**:
1. `groupSpec.Commands`のループ（`CommandSpec`を使用）
2. `config.ExpandString`による`cmd`フィールドの展開
3. 展開エラー時の警告ログとスキップ処理

**追加される処理**:
1. `runtimeGroup.Commands`のループ（`RuntimeCommand`を使用）
2. 展開済みの`ExpandedCmd`フィールドの使用
3. エラーハンドリングの簡素化（展開エラーは事前に検出済み）

**変更前**:
```go
func (m *Manager) collectVerificationFiles(runtimeGroup *runnertypes.RuntimeGroup) map[string]struct{} {
    // ... 既存処理 ...

    // Add command files
    if m.pathResolver != nil {
        for _, command := range groupSpec.Commands {
            // Expand command path using group variables
            expandedCmd, err := config.ExpandString(
                command.Cmd,
                runtimeGroup.ExpandedVars,
                fmt.Sprintf("group[%s]", groupSpec.Name),
                "cmd")
            if err != nil {
                slog.Warn("Failed to expand command path",
                    "group", groupSpec.Name,
                    "command", command.Cmd,
                    "error", err.Error())
                continue
            }

            // Resolve expanded command path
            resolvedPath, err := m.pathResolver.ResolvePath(expandedCmd)
            if err != nil {
                slog.Warn("Failed to resolve command path",
                    "group", groupSpec.Name,
                    "command", expandedCmd,
                    "error", err.Error())
                continue
            }
            fileSet[resolvedPath] = struct{}{}
        }
    }

    return fileSet
}
```

**変更後**:
```go
func (m *Manager) collectVerificationFiles(runtimeGroup *runnertypes.RuntimeGroup) map[string]struct{} {
    // ... 既存処理 (ExpandedVerifyFiles の追加) ...

    // Add command files from pre-expanded commands
    if m.pathResolver != nil && runtimeGroup.Commands != nil {
        for _, runtimeCmd := range runtimeGroup.Commands {
            // Use pre-expanded command path
            // Command-level variables are already resolved
            resolvedPath, err := m.pathResolver.ResolvePath(runtimeCmd.ExpandedCmd)
            if err != nil {
                slog.Warn("Failed to resolve command path",
                    "group", groupSpec.Name,
                    "command", runtimeCmd.ExpandedCmd,
                    "error", err.Error())
                continue
            }
            fileSet[resolvedPath] = struct{}{}
        }
    }

    return fileSet
}
```

#### 2.3.3 変更の詳細

**ループ対象の変更**:
```go
// Before: CommandSpec をループ（展開前）
for _, command := range groupSpec.Commands {
    // command は *CommandSpec 型
    // command.Cmd は変数展開前の文字列
}

// After: RuntimeCommand をループ（展開済み）
for _, runtimeCmd := range runtimeGroup.Commands {
    // runtimeCmd は *RuntimeCommand 型
    // runtimeCmd.ExpandedCmd は展開済み文字列
}
```

**変数展開処理の削除**:
```go
// Before: 検証時に変数展開を実行（削除される）
expandedCmd, err := config.ExpandString(
    command.Cmd,
    runtimeGroup.ExpandedVars,  // グループ変数のみ
    fmt.Sprintf("group[%s]", groupSpec.Name),
    "cmd")
if err != nil {
    slog.Warn("Failed to expand command path", ...)
    continue  // 警告を出してスキップ
}

// After: 展開済みコマンドを直接使用（新規）
// config.ExpandString の呼び出しは不要
// コマンド変数も含めて展開済み
resolvedPath, err := m.pathResolver.ResolvePath(runtimeCmd.ExpandedCmd)
```

**エラーハンドリングの変更**:
- **Before**: 展開エラーは警告としてログ出力し、該当コマンドをスキップ
- **After**: 展開エラーは`preExpandCommands`で検出済みのため、ここでは発生しない
- パス解決エラーのみ警告ログを出力（既存動作と同じ）

**コマンド変数へのアクセス**:
- **Before**: グループ変数のみ参照可能（`runtimeGroup.ExpandedVars`）
- **After**: コマンド変数も含めて展開済み（`runtimeCmd.ExpandedCmd`）

**NULL チェックの追加**:
```go
if m.pathResolver != nil && runtimeGroup.Commands != nil {
    // ↑ runtimeGroup.Commands の NULL チェック追加
}
```

### 2.4 executeAllCommands の変更

#### 2.4.1 シグネチャ（変更なし）

**ファイル**: `internal/runner/group_executor.go`

```go
// executeAllCommands executes all commands in a group sequentially
func (ge *DefaultGroupExecutor) executeAllCommands(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
    runtimeGroup *runnertypes.RuntimeGroup,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) (common.CommandResults, *groupExecutionResult, error)
```

#### 2.4.2 実装変更

**変更前**:
```go
func (ge *DefaultGroupExecutor) executeAllCommands(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
    runtimeGroup *runnertypes.RuntimeGroup,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) (common.CommandResults, *groupExecutionResult, error) {
    commandResults := make(common.CommandResults, 0, len(groupSpec.Commands))

    for i := range groupSpec.Commands {
        cmdSpec := &groupSpec.Commands[i]
        slog.Info("Executing command",
            slog.String("command", cmdSpec.Name),
            slog.Int("index", i+1),
            slog.Int("total", len(groupSpec.Commands)))

        // Expand command configuration
        globalOutputSizeLimit := common.NewOutputSizeLimitFromPtr(runtimeGlobal.Spec.OutputSizeLimit)
        runtimeCmd, err := config.ExpandCommand(
            cmdSpec, runtimeGroup, runtimeGlobal,
            runtimeGlobal.Timeout(), globalOutputSizeLimit)
        if err != nil {
            // ... error handling ...
        }

        // ... remaining execution logic ...
    }
}
```

**変更後**:
```go
func (ge *DefaultGroupExecutor) executeAllCommands(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
    runtimeGroup *runnertypes.RuntimeGroup,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) (common.CommandResults, *groupExecutionResult, error) {
    commandResults := make(common.CommandResults, 0, len(runtimeGroup.Commands))

    for i, runtimeCmd := range runtimeGroup.Commands {
        slog.Info("Executing command",
            slog.String("command", runtimeCmd.Spec.Name),
            slog.Int("index", i+1),
            slog.Int("total", len(runtimeGroup.Commands)))

        // Command is already fully expanded (including EffectiveWorkDir)
        // - config.ExpandCommand was called in preExpandCommands
        // - resolveCommandWorkDir was called in preExpandCommands

        // Execute the command with pre-expanded configuration
        stdout, stderr, exitCode, err := ge.executeSingleCommand(
            ctx, runtimeCmd, groupSpec, runtimeGroup, runtimeGlobal)

        // ... result handling logic (unchanged) ...
    }
}
```

**削除される処理**:
1. `config.ExpandCommand`の呼び出し（→ `preExpandCommands`に移動）
2. `resolveCommandWorkDir`の呼び出し（→ `preExpandCommands`に移動）
3. `EffectiveWorkDir`の設定（→ `preExpandCommands`で設定済み）

#### 2.4.3 変更の詳細

**ループ対象の変更**:
```go
// Before: CommandSpec をループ（展開前）、インデックスベース
for i := range groupSpec.Commands {
    cmdSpec := &groupSpec.Commands[i]
    // cmdSpec は *CommandSpec 型
}

// After: RuntimeCommand をループ（展開済み）、値ベース
for i, runtimeCmd := range runtimeGroup.Commands {
    // runtimeCmd は *RuntimeCommand 型（既に展開済み）
}
```

**コマンド展開処理の削除**:
```go
// Before: 実行ループ内で展開（削除される）
globalOutputSizeLimit := common.NewOutputSizeLimitFromPtr(runtimeGlobal.Spec.OutputSizeLimit)
runtimeCmd, err := config.ExpandCommand(
    cmdSpec, runtimeGroup, runtimeGlobal,
    runtimeGlobal.Timeout(), globalOutputSizeLimit)
if err != nil {
    // エラーハンドリング...
    return commandResults, errResult, fmt.Errorf("failed to expand command[%s]: %w", ...)
}

// After: 展開済みコマンドを直接使用（新規）
// config.ExpandCommand の呼び出しは不要
```

**Workdir解決処理の削除**:
```go
// Before: 実行ループ内で解決（削除される）
workDir, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
if err != nil {
    // エラーハンドリング...
    return commandResults, errResult, fmt.Errorf("failed to resolve command workdir[%s]: %w", ...)
}
runtimeCmd.EffectiveWorkDir = workDir

// After: 事前に解決済み（新規）
// resolveCommandWorkDir の呼び出しは不要
// runtimeCmd.EffectiveWorkDir は preExpandCommands で設定済み
```

**エラーハンドリングの簡素化**:
- **Before**: 展開エラーとworkdir解決エラーが実行ループ内で発生する可能性
- **After**: これらのエラーは`preExpandCommands`で検出済み
- 実行ループではコマンド実行エラーのみ処理すれば良い

**コマンド情報へのアクセス**:
```go
// Before: cmdSpec と runtimeCmd の両方を使用
cmdSpec := &groupSpec.Commands[i]              // 元の仕様
runtimeCmd, err := config.ExpandCommand(...)   // 展開後

// After: runtimeCmd のみ使用
runtimeCmd := runtimeGroup.Commands[i]         // 展開済み
// runtimeCmd.Spec で元の仕様にアクセス可能
```

## 3. エラー仕様

### 3.1 コマンド展開エラー

#### 3.1.1 エラーの発生箇所

**Before**: 実行ループ内（`executeAllCommands`）
**After**: グループ展開後、検証前（`preExpandCommands`）

#### 3.1.2 エラーメッセージ形式

```
failed to pre-expand commands for group[<group_name>]: command[<command_name>] (index <N>): <original_error>
```

**例**:
```
failed to pre-expand commands for group[build]: command[compile] (index 0): undefined variable in command[compile].cmd: 'undefined_var' (context: %{undefined_var}/bin/gcc)
```

### 3.2 Workdir解決エラー

#### 3.2.1 エラーの発生箇所

**Before**: 実行ループ内（`executeAllCommands`）
**After**: グループ展開後、検証前（`preExpandCommands`）

#### 3.2.2 エラーメッセージ形式

```
failed to pre-expand commands for group[<group_name>]: command[<command_name>] (index <N>): failed to resolve workdir: <original_error>
```

**例**:
```
failed to pre-expand commands for group[build]: command[compile] (index 0): failed to resolve workdir: failed to expand command workdir: undefined variable in command[compile].workdir: 'output_dir' (context: %{output_dir}/build)
```

### 3.3 動作変更の影響

#### 3.3.1 エラー検出タイミング

| 状況 | Before | After |
|------|--------|-------|
| 変数未定義（検証時） | 警告 → スキップ | エラー → 中断 |
| 変数未定義（実行時） | エラー → 中断 | N/A（事前検出） |
| コマンド展開失敗 | 実行ループ内で検出 | 検証前に検出 |
| Workdir解決失敗 | 実行ループ内で検出 | 検証前に検出 ★NEW★ |

#### 3.3.2 Fail Fast の効果

```mermaid
flowchart TD
    subgraph "Before"
        B1[グループ展開] --> B2[検証<br/>警告のみ]
        B2 --> B3[実行: コマンド1]
        B3 --> B4[実行: コマンド2<br/>★エラー発生★]
        B4 -.-> B5[コマンド3-N<br/>実行されない]
    end

    subgraph "After"
        A1[グループ展開] --> A2[コマンド事前展開<br/>★エラー発生★]
        A2 -.-> A3[検証<br/>実行されない]
        A3 -.-> A4[実行<br/>実行されない]
    end

    style B4 fill:#ffcccc
    style A2 fill:#ffcccc
```

## 4. テストケース仕様

### 4.1 単体テスト: preExpandCommands

#### 4.1.1 正常系

**ファイル**: `internal/runner/group_executor_test.go`

```go
func TestPreExpandCommands_Success(t *testing.T) {
    tests := []struct {
        name        string
        groupSpec   *runnertypes.GroupSpec
        runtimeGroup *runnertypes.RuntimeGroup
        runtimeGlobal *runnertypes.RuntimeGlobal
        wantCmdCount int
    }{
        {
            name: "single command",
            groupSpec: &runnertypes.GroupSpec{
                Name: "test_group",
                Commands: []runnertypes.CommandSpec{
                    {Name: "cmd1", Cmd: "/bin/echo"},
                },
            },
            wantCmdCount: 1,
        },
        {
            name: "multiple commands",
            groupSpec: &runnertypes.GroupSpec{
                Name: "test_group",
                Commands: []runnertypes.CommandSpec{
                    {Name: "cmd1", Cmd: "/bin/echo"},
                    {Name: "cmd2", Cmd: "/bin/cat"},
                    {Name: "cmd3", Cmd: "/bin/ls"},
                },
            },
            wantCmdCount: 3,
        },
        {
            name: "command with group variables",
            groupSpec: &runnertypes.GroupSpec{
                Name: "test_group",
                Commands: []runnertypes.CommandSpec{
                    {Name: "cmd1", Cmd: "%{tool_path}/binary"},
                },
            },
            runtimeGroup: &runnertypes.RuntimeGroup{
                ExpandedVars: map[string]string{"tool_path": "/opt/tools"},
            },
            wantCmdCount: 1,
        },
        {
            name: "command with command-level variables",
            groupSpec: &runnertypes.GroupSpec{
                Name: "test_group",
                Commands: []runnertypes.CommandSpec{
                    {
                        Name: "cmd1",
                        Vars: []string{"cmd_var=/custom/path"},
                        Cmd:  "%{cmd_var}/tool",
                    },
                },
            },
            wantCmdCount: 1,
        },
        {
            name: "empty commands",
            groupSpec: &runnertypes.GroupSpec{
                Name:     "test_group",
                Commands: []runnertypes.CommandSpec{},
            },
            wantCmdCount: 0,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ge := createTestGroupExecutor(t)
            runtimeGroup := tt.runtimeGroup
            if runtimeGroup == nil {
                runtimeGroup = &runnertypes.RuntimeGroup{
                    Spec:         tt.groupSpec,
                    ExpandedVars: make(map[string]string),
                }
            }
            runtimeGlobal := createTestRuntimeGlobal(t)

            err := ge.preExpandCommands(tt.groupSpec, runtimeGroup, runtimeGlobal)

            require.NoError(t, err)
            assert.Len(t, runtimeGroup.Commands, tt.wantCmdCount)
        })
    }
}
```

#### 4.1.2 異常系

```go
func TestPreExpandCommands_Error(t *testing.T) {
    tests := []struct {
        name         string
        groupSpec    *runnertypes.GroupSpec
        runtimeGroup *runnertypes.RuntimeGroup
        wantErrContains string
    }{
        {
            name: "undefined variable in cmd",
            groupSpec: &runnertypes.GroupSpec{
                Name: "test_group",
                Commands: []runnertypes.CommandSpec{
                    {Name: "cmd1", Cmd: "%{undefined_var}/binary"},
                },
            },
            runtimeGroup: &runnertypes.RuntimeGroup{
                ExpandedVars: make(map[string]string),
            },
            wantErrContains: "undefined variable",
        },
        {
            name: "undefined variable in args",
            groupSpec: &runnertypes.GroupSpec{
                Name: "test_group",
                Commands: []runnertypes.CommandSpec{
                    {
                        Name: "cmd1",
                        Cmd:  "/bin/echo",
                        Args: []string{"%{undefined_arg}"},
                    },
                },
            },
            runtimeGroup: &runnertypes.RuntimeGroup{
                ExpandedVars: make(map[string]string),
            },
            wantErrContains: "undefined variable",
        },
        {
            name: "error includes command name",
            groupSpec: &runnertypes.GroupSpec{
                Name: "test_group",
                Commands: []runnertypes.CommandSpec{
                    {Name: "failing_cmd", Cmd: "%{bad}/path"},
                },
            },
            runtimeGroup: &runnertypes.RuntimeGroup{
                ExpandedVars: make(map[string]string),
            },
            wantErrContains: "failing_cmd",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ge := createTestGroupExecutor(t)
            runtimeGlobal := createTestRuntimeGlobal(t)

            err := ge.preExpandCommands(tt.groupSpec, tt.runtimeGroup, runtimeGlobal)

            require.Error(t, err)
            assert.Contains(t, err.Error(), tt.wantErrContains)
        })
    }
}
```

### 4.2 単体テスト: collectVerificationFiles

#### 4.2.1 展開済みコマンドの使用

```go
func TestCollectVerificationFiles_UsesPreExpandedCommands(t *testing.T) {
    tests := []struct {
        name           string
        runtimeGroup   *runnertypes.RuntimeGroup
        wantFiles      []string
    }{
        {
            name: "uses pre-expanded command path",
            runtimeGroup: &runnertypes.RuntimeGroup{
                Spec: &runnertypes.GroupSpec{Name: "test"},
                Commands: []*runnertypes.RuntimeCommand{
                    {
                        Spec:        &runnertypes.CommandSpec{Name: "cmd1"},
                        ExpandedCmd: "/opt/tools/binary",
                    },
                },
            },
            wantFiles: []string{"/opt/tools/binary"},
        },
        {
            name: "command-level variable already expanded",
            runtimeGroup: &runnertypes.RuntimeGroup{
                Spec: &runnertypes.GroupSpec{Name: "test"},
                Commands: []*runnertypes.RuntimeCommand{
                    {
                        Spec: &runnertypes.CommandSpec{
                            Name: "cmd1",
                            Vars: []string{"cmd_var=/custom"},
                            Cmd:  "%{cmd_var}/tool",  // original
                        },
                        ExpandedCmd: "/custom/tool",  // pre-expanded
                    },
                },
            },
            wantFiles: []string{"/custom/tool"},
        },
        {
            name: "multiple commands",
            runtimeGroup: &runnertypes.RuntimeGroup{
                Spec: &runnertypes.GroupSpec{Name: "test"},
                Commands: []*runnertypes.RuntimeCommand{
                    {Spec: &runnertypes.CommandSpec{Name: "cmd1"}, ExpandedCmd: "/bin/a"},
                    {Spec: &runnertypes.CommandSpec{Name: "cmd2"}, ExpandedCmd: "/bin/b"},
                },
            },
            wantFiles: []string{"/bin/a", "/bin/b"},
        },
        {
            name: "nil Commands field (backward compatibility)",
            runtimeGroup: &runnertypes.RuntimeGroup{
                Spec:     &runnertypes.GroupSpec{Name: "test"},
                Commands: nil,
            },
            wantFiles: []string{},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m := createTestVerificationManager(t)

            result := m.collectVerificationFiles(tt.runtimeGroup)

            for _, wantFile := range tt.wantFiles {
                _, exists := result[wantFile]
                assert.True(t, exists, "expected file %s in result", wantFile)
            }
        })
    }
}
```

### 4.3 統合テスト

#### 4.3.1 コマンドレベル変数の検証時利用

**ファイル**: `cmd/runner/integration_preexpand_test.go`

```go
func TestIntegration_CommandLevelVarsInVerification(t *testing.T) {
    // Setup: Create a test binary
    testBinary := createTestBinary(t, "test_tool")

    // TOML configuration with command-level variable
    config := fmt.Sprintf(`
[global]
env_allowed = ["HOME"]

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
vars = ["tool_path=%s"]
cmd = "%%{tool_path}"
`, testBinary)

    // Execute
    result, err := runWithConfig(t, config)

    // Verify: No warnings about undefined variables during verification
    require.NoError(t, err)
    assert.NotContains(t, result.Logs, "Failed to expand command path")
    assert.Contains(t, result.Logs, "Group file verification completed")
}
```

#### 4.3.2 Fail Fast 動作

```go
func TestIntegration_FailFastOnExpansionError(t *testing.T) {
    // TOML configuration with undefined variable
    config := `
[global]
env_allowed = ["HOME"]

[[groups]]
name = "test_group"

[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"
args = ["before error"]

[[groups.commands]]
name = "cmd2"
cmd = "%{undefined_var}/tool"

[[groups.commands]]
name = "cmd3"
cmd = "/bin/echo"
args = ["after error"]
`

    // Execute
    result, err := runWithConfig(t, config)

    // Verify: Error occurs before any command execution
    require.Error(t, err)
    assert.Contains(t, err.Error(), "failed to pre-expand commands")
    assert.Contains(t, err.Error(), "cmd2")
    assert.Contains(t, err.Error(), "undefined_var")

    // cmd1 should not have been executed
    assert.NotContains(t, result.Output, "before error")
}
```

#### 4.3.3 Dry-run モード互換性

```go
func TestIntegration_DryRunWithPreExpand(t *testing.T) {
    testBinary := createTestBinary(t, "test_tool")

    config := fmt.Sprintf(`
[global]
env_allowed = ["HOME"]

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
vars = ["tool=%s"]
cmd = "%%{tool}"
args = ["--help"]
`, testBinary)

    // Execute in dry-run mode
    result, err := runWithConfig(t, config, WithDryRun(true))

    // Verify: Dry-run completes successfully with pre-expanded commands
    require.NoError(t, err)
    assert.Contains(t, result.Output, testBinary)  // Expanded path in output
    assert.Contains(t, result.Output, "DRY-RUN")
}
```

### 4.4 性能テスト

#### 4.4.1 ベンチマーク

**ファイル**: `internal/runner/group_executor_bench_test.go`

```go
func BenchmarkPreExpandCommands(b *testing.B) {
    benchmarks := []struct {
        name         string
        commandCount int
    }{
        {"1_command", 1},
        {"10_commands", 10},
        {"50_commands", 50},
        {"100_commands", 100},
    }

    for _, bm := range benchmarks {
        b.Run(bm.name, func(b *testing.B) {
            ge := createBenchGroupExecutor(b)
            groupSpec := createBenchGroupSpec(b, bm.commandCount)
            runtimeGroup := createBenchRuntimeGroup(b, groupSpec)
            runtimeGlobal := createBenchRuntimeGlobal(b)

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                // Reset Commands slice for each iteration
                runtimeGroup.Commands = nil
                _ = ge.preExpandCommands(groupSpec, runtimeGroup, runtimeGlobal)
            }
        })
    }
}

func BenchmarkExecuteGroupWithPreExpand(b *testing.B) {
    benchmarks := []struct {
        name         string
        commandCount int
    }{
        {"1_command", 1},
        {"10_commands", 10},
        {"50_commands", 50},
    }

    for _, bm := range benchmarks {
        b.Run(bm.name, func(b *testing.B) {
            ge := createBenchGroupExecutor(b)
            groupSpec := createBenchGroupSpec(b, bm.commandCount)
            runtimeGlobal := createBenchRuntimeGlobal(b)

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                _ = ge.ExecuteGroup(context.Background(), groupSpec, runtimeGlobal)
            }
        })
    }
}
```

#### 4.4.2 メモリプロファイリング

```go
func BenchmarkPreExpandCommands_Memory(b *testing.B) {
    commandCounts := []int{10, 50, 100}

    for _, count := range commandCounts {
        b.Run(fmt.Sprintf("%d_commands", count), func(b *testing.B) {
            ge := createBenchGroupExecutor(b)
            groupSpec := createBenchGroupSpec(b, count)
            runtimeGroup := createBenchRuntimeGroup(b, groupSpec)
            runtimeGlobal := createBenchRuntimeGlobal(b)

            b.ReportAllocs()
            b.ResetTimer()

            for i := 0; i < b.N; i++ {
                runtimeGroup.Commands = nil
                _ = ge.preExpandCommands(groupSpec, runtimeGroup, runtimeGlobal)
            }
        })
    }
}
```

## 5. パフォーマンス計測方法

### 5.1 ベンチマーク実行

```bash
# 基本ベンチマーク
go test -bench=BenchmarkPreExpandCommands -benchmem ./internal/runner/...

# メモリプロファイル
go test -bench=BenchmarkPreExpandCommands_Memory -benchmem -memprofile=mem.out ./internal/runner/...
go tool pprof -http=:8080 mem.out

# CPU プロファイル
go test -bench=BenchmarkExecuteGroupWithPreExpand -cpuprofile=cpu.out ./internal/runner/...
go tool pprof -http=:8080 cpu.out
```

### 5.2 期待値

| メトリクス | 閾値 | 備考 |
|-----------|------|------|
| 10コマンド展開時間 | < 1ms | `BenchmarkPreExpandCommands/10_commands` |
| 100コマンド展開時間 | < 10ms | `BenchmarkPreExpandCommands/100_commands` |
| 1コマンドあたりのメモリ | < 10KB | `BenchmarkPreExpandCommands_Memory` |
| 既存ベンチマークの劣化 | < 5% | 回帰テスト |

### 5.3 比較測定

変更前後の比較のため、以下のスクリプトを使用:

```bash
#!/bin/bash
# benchmark_compare.sh

# Before (main branch)
git checkout main
go test -bench=BenchmarkExecuteGroup -benchmem -count=5 ./internal/runner/... > bench_before.txt

# After (feature branch)
git checkout feature/pre-expand-commands
go test -bench=BenchmarkExecuteGroup -benchmem -count=5 ./internal/runner/... > bench_after.txt

# Compare
benchstat bench_before.txt bench_after.txt
```

## 6. 実装チェックリスト

### 6.1 コード変更

- [ ] `group_executor.go`: `preExpandCommands` 関数追加
- [ ] `group_executor.go`: `ExecuteGroup` で `preExpandCommands` 呼び出し追加
- [ ] `group_executor.go`: `executeAllCommands` を展開済みコマンド使用に変更
- [ ] `manager.go`: `collectVerificationFiles` を展開済みコマンド使用に変更

### 6.2 テスト

- [ ] 単体テスト: `preExpandCommands` 正常系
- [ ] 単体テスト: `preExpandCommands` 異常系
- [ ] 単体テスト: `collectVerificationFiles` 変更
- [ ] 単体テスト: `executeAllCommands` 変更
- [ ] 統合テスト: コマンドレベル変数の検証時利用
- [ ] 統合テスト: Fail Fast 動作
- [ ] 統合テスト: Dry-run モード互換性
- [ ] 性能テスト: ベンチマーク追加
- [ ] 回帰テスト: 既存テストがすべてパス

### 6.3 ドキュメント

- [ ] コード内コメントの更新
- [ ] CHANGELOG.md への記載

## 7. 参照

### 7.1 関連ファイル

- `internal/runner/group_executor.go`: グループ実行ロジック
- `internal/verification/manager.go`: ファイル検証ロジック
- `internal/runner/config/expansion.go`: 変数展開ロジック
- `internal/runner/runnertypes/runtime.go`: Runtime 型定義

### 7.2 関連ドキュメント

- 要件定義書: `01_requirements.md`
- アーキテクチャ設計書: `02_architecture.md`
- 提案アーキテクチャ: `02b_proposed_architecture.md`

---

**文書バージョン**: 1.0
**作成日**: 2025-11-27
**承認日**: [レビュー後に記載]
**次回レビュー予定**: [実装完了後]
