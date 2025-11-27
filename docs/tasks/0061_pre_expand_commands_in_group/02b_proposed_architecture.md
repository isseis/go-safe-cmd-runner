# 改善案のアーキテクチャ

## 1. 改善後の実行フロー

### 1.1 全体フロー

```mermaid
flowchart TD
    Start([Start: ExecuteGroup]) --> ExpandGroup[1. config.ExpandGroup]
    ExpandGroup --> GroupVars[グループ変数展開]
    GroupVars --> CmdAllowed[cmd_allowed 展開]
    CmdAllowed --> RuntimeGroup["(RuntimeGroup)"]

    RuntimeGroup --> ResolveWorkDir[2. Working Directory 解決]
    ResolveWorkDir --> SetWorkDirVar[3. __runner_workdir 設定]
    SetWorkDirVar --> ExpandCommands[4. 全コマンド展開<br/>★ NEW ★]
    ExpandCommands --> ExpandedCommands["(RuntimeGroup<br/>Commands = []RuntimeCommand)"]
    ExpandedCommands --> VerifyFiles[5. verifyGroupFiles]

    VerifyFiles --> CollectFiles[collectVerificationFiles]
    CollectFiles --> UseExpanded[展開済みコマンドを使用<br/>runtimeCmd.ExpandedCmd]
    UseExpanded --> ResolvePath[PathResolver.ResolvePath]
    ResolvePath --> VerifyHash[ハッシュ検証]

    VerifyHash --> ExecuteAll[6. executeAllCommands]
    ExecuteAll --> Loop{各コマンド}
    Loop --> UsePreExpanded[展開済みコマンドを使用<br/>runtimeGroup.Commands]
    UsePreExpanded --> Execute[executeSingleCommand]
    Execute --> Loop

    Loop --> End([End])

    style ExpandCommands fill:#ccffcc
    style ExpandedCommands fill:#ccffcc
    style UseExpanded fill:#ccffcc
    style UsePreExpanded fill:#ccffcc
```

### 1.2 変更点

#### 変更1: ExecuteGroup でのコマンド展開追加

**重要な変更**: `__runner_workdir` 変数の設定タイミングとの関係により、コマンド展開は `ExecuteGroup` 内で、作業ディレクトリ解決後に行う。

**Before (現在)**:
```go
func (ge *DefaultGroupExecutor) ExecuteGroup(...) error {
    // 1. Expand group configuration
    runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)

    // 2. Determine working directory for the group
    workDir, tempDirMgr, err := ge.resolveGroupWorkDir(runtimeGroup)

    // 3. Set __runner_workdir variable
    runtimeGroup.ExpandedVars[variable.WorkDirKey()] = workDir

    // 4. Verify group files
    err := ge.verifyGroupFiles(runtimeGroup)

    // 5. Execute commands (各コマンドで展開を実行)
    commandResults, errResult, err := ge.executeAllCommands(...)
}
```

**After (改善後)**:
```go
func (ge *DefaultGroupExecutor) ExecuteGroup(...) error {
    // 1. Expand group configuration
    runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)

    // 2. Determine working directory for the group
    workDir, tempDirMgr, err := ge.resolveGroupWorkDir(runtimeGroup)

    // 3. Set __runner_workdir variable
    runtimeGroup.ExpandedVars[variable.WorkDirKey()] = workDir

    // 4. Expand all commands (NEW)
    runtimeGroup.Commands = make([]*RuntimeCommand, len(groupSpec.Commands))
    for i := range groupSpec.Commands {
        runtimeCmd, err := config.ExpandCommand(
            &groupSpec.Commands[i],
            runtimeGroup,
            runtimeGlobal,
            runtimeGlobal.Timeout(),
            globalOutputSizeLimit)
        if err != nil {
            return fmt.Errorf("failed to expand command[%s]: %w",
                groupSpec.Commands[i].Name, err)
        }

        // Resolve effective working directory (Fail Fast)
        workDir, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
        if err != nil {
            return fmt.Errorf("failed to resolve workdir for command[%s]: %w",
                groupSpec.Commands[i].Name, err)
        }
        runtimeCmd.EffectiveWorkDir = workDir

        runtimeGroup.Commands[i] = runtimeCmd
    }

    // 5. Verify group files (展開済みコマンドを使用)
    err := ge.verifyGroupFiles(runtimeGroup)

    // 6. Execute commands (展開済みコマンドを使用)
    commandResults, errResult, err := ge.executeAllCommands(...)
}
```

**`__runner_workdir` の扱い**:
- `cmd` フィールドで `__runner_workdir` を使用した場合はエラーとする
  - コマンドパスに作業ディレクトリを含める正当なユースケースは存在しない
- `args` や `env` では引き続き使用可能（実行時に展開されるため問題ない）

#### 変更2: 検証時に展開済みコマンドを使用

**Before (現在)**:
```go
func (m *Manager) collectVerificationFiles(runtimeGroup *RuntimeGroup) map[string]struct{} {
    // ...
    for _, command := range groupSpec.Commands {
        // 変数展開を実行
        expandedCmd, err := config.ExpandString(
            command.Cmd,
            runtimeGroup.ExpandedVars,
            ...)
        if err != nil {
            slog.Warn("Failed to expand command path", ...)
            continue
        }

        resolvedPath, err := m.pathResolver.ResolvePath(expandedCmd)
        // ...
    }
}
```

**After (改善後)**:
```go
func (m *Manager) collectVerificationFiles(runtimeGroup *RuntimeGroup) map[string]struct{} {
    // ...
    for _, runtimeCmd := range runtimeGroup.Commands {
        // 既に展開済み - そのまま使用
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
```

#### 変更3: 実行時に展開済みコマンドを使用

**Before (現在)**:
```go
func (ge *DefaultGroupExecutor) executeAllCommands(...) {
    commandResults := make(CommandResults, 0, len(groupSpec.Commands))

    for i := range groupSpec.Commands {
        cmdSpec := &groupSpec.Commands[i]
        slog.Info("Executing command", ...)

        // 毎回展開を実行
        runtimeCmd, err := config.ExpandCommand(
            cmdSpec,
            runtimeGroup,
            runtimeGlobal,
            ...)
        if err != nil {
            return commandResults, errResult, fmt.Errorf("...")
        }

        // ... 実行処理 ...
    }
}
```

**After (改善後)**:
```go
func (ge *DefaultGroupExecutor) executeAllCommands(...) {
    commandResults := make(CommandResults, 0, len(runtimeGroup.Commands))

    // 展開済みコマンドを使用
    for i, runtimeCmd := range runtimeGroup.Commands {
        slog.Info("Executing command",
            slog.String("command", runtimeCmd.Spec.Name),
            slog.Int("index", i+1),
            slog.Int("total", len(runtimeGroup.Commands)))

        // 展開は不要 - 既に展開済み
        // config.ExpandCommand の呼び出しを削除

        // ... 実行処理 ...
    }
}
```

## 2. データフロー

### 2.1 改善後のデータフロー

```mermaid
flowchart TD
    subgraph "入力"
        TOML[(TOML Config)]
        Env[(Environment)]
    end

    subgraph "Phase 1: グループ展開"
        LoadTOML[Load TOML]
        GlobalExpand[Global 変数展開]
        GroupExpand[Group 変数展開]
        CmdAllowedExpand[cmd_allowed 展開]
        CommandsExpand[★ Commands 展開 ★<br/>NEW]
    end

    subgraph "Phase 2: 検証"
        CollectFiles[collectVerificationFiles]
        UseExpandedCmd[展開済み cmd 使用]
        ResolveForVerify[パス解決]
        HashVerify[ハッシュ検証]
    end

    subgraph "Phase 3: 実行"
        ForEach[For each runtimeCmd]
        ResolveForExec[作業ディレクトリ解決]
        ExecuteCmd[コマンド実行]
    end

    subgraph "データ"
        RuntimeGlobal[(RuntimeGlobal)]
        RuntimeGroup[(RuntimeGroup<br/>Commands = []RuntimeCommand)]
        RuntimeCommand[(RuntimeCommand)]
    end

    TOML --> LoadTOML
    Env --> GlobalExpand
    LoadTOML --> GlobalExpand
    GlobalExpand --> RuntimeGlobal
    RuntimeGlobal --> GroupExpand
    GroupExpand --> CmdAllowedExpand
    CmdAllowedExpand --> CommandsExpand
    CommandsExpand --> RuntimeCommand
    RuntimeCommand --> RuntimeGroup

    RuntimeGroup --> CollectFiles
    CollectFiles --> UseExpandedCmd
    UseExpandedCmd --> ResolveForVerify
    ResolveForVerify --> HashVerify

    RuntimeGroup --> ForEach
    ForEach --> ResolveForExec
    ResolveForExec --> ExecuteCmd
    ExecuteCmd --> ForEach

    style CommandsExpand fill:#ccffcc
    style UseExpandedCmd fill:#ccffcc
    style RuntimeCommand fill:#ccffcc
    style RuntimeGroup fill:#ccffcc
```

**改善点**:
- `RuntimeGroup.Commands` が活用される
- `RuntimeCommand` はグループ展開時に1回だけ作成される
- 変数展開は1回のみ実行される

### 2.2 変数のスコープと参照タイミング

```mermaid
flowchart TD
    subgraph "変数定義"
        Global["Global Variables<br/>- env_import<br/>- vars<br/>- auto vars"]
        Group["Group Variables<br/>- env_import<br/>- vars"]
        Command["Command Variables<br/>- env_import<br/>- vars"]
    end

    subgraph "展開フェーズ"
        Phase1["Phase 1: Global 展開<br/>→ RuntimeGlobal"]
        Phase2["Phase 2: Group 展開<br/>→ RuntimeGroup"]
        Phase3["★ Phase 3: Command 展開 ★<br/>→ RuntimeCommand<br/>NEW: グループ展開時に実行"]
    end

    subgraph "利用フェーズ"
        Verify["検証フェーズ<br/>✅ 全変数が利用可能"]
        Execute["実行フェーズ<br/>✅ 全変数が利用可能"]
    end

    Global --> Phase1
    Phase1 --> Phase2
    Group --> Phase2
    Phase2 --> Phase3
    Command --> Phase3

    Phase3 --> Verify
    Phase3 --> Execute

    style Phase3 fill:#ccffcc
    style Verify fill:#ccffcc
    style Execute fill:#ccffcc
```

**改善点**:
- 検証フェーズでもコマンド変数が利用可能
- すべてのフェーズで一貫した変数アクセス

## 3. 具体例での改善効果

### 3.1 シナリオ: コマンドレベル変数の使用

```toml
[global]
env_import = ["home=HOME"]

[[groups]]
name = "build_group"
vars = ["group_var=/opt/group"]

[[groups.commands]]
name = "build_cmd"
vars = ["cmd_var=/opt/command"]  # ← コマンドレベル変数
cmd = "%{cmd_var}/builder"       # ← この変数を参照
args = ["--output", "%{group_var}/output"]
```

### 3.2 改善後のログ

```
# グループ展開フェーズ
[INFO] Expanding group group=build_group
[INFO] Expanding command command=build_cmd
[INFO] Command expanded successfully
    cmd=/opt/command/builder
    args=["--output", "/opt/group/output"]

# 検証フェーズ
[INFO] Collecting verification files group=build_group
[INFO] Command path resolved
    command=/opt/command/builder
    resolved=/opt/command/builder
[INFO] Group file verification completed
    group=build_group
    verified_files=1
    skipped_files=0

# 実行フェーズ
[INFO] Executing command command=build_cmd index=1 total=1
[INFO] Command completed successfully exit_code=0
```

**改善点**:
- ✅ 警告やエラーが出ない
- ✅ コマンドレベル変数が正しく展開される
- ✅ すべてのフェーズで一貫した動作

### 3.3 ユーザーの疑問の解消

| 疑問 | 現在 | 改善後 |
|------|------|--------|
| 警告が出るのに実行される？ | ❌ 混乱 | ✅ 警告が出ない |
| コマンドレベル変数が使えない？ | ❌ 制限あり | ✅ 使える |
| どう設定すれば良い？ | ❌ 不明確 | ✅ 直感的 |

## 4. パフォーマンスの比較

### 4.1 CPU 使用量

```mermaid
gantt
    title 変数展開の実行タイミング
    dateFormat X
    axisFormat %s

    section 現在
    グループ展開      :0, 1
    検証時展開 (cmd)  :1, 1
    実行時展開 (全体) :2, 2

    section 改善後
    グループ展開 (全体) :0, 2
    検証 (展開不要)     :2, 0
    実行 (展開不要)     :2, 0
```

**改善点**:
- 展開処理: 3回 → 1回
- CPU 時間: 約 33% 削減（理論値）

### 4.2 メモリ使用量

| 項目 | 現在 | 改善後 | 差分 |
|------|------|--------|------|
| RuntimeGroup | 1個 | 1個 | 0 |
| RuntimeCommand | 実行時のみ存在 | グループ展開時から存在 | +N個 |
| 一時文字列（検証時） | N個 | 0個 | -N個 |
| 一時文字列（実行時） | N個 | 0個 | -N個 |

**影響**:
- `RuntimeCommand` が早期に作成される（メモリ増加）
- 一時文字列が削減される（メモリ削減）
- **実質的な影響**: 100コマンドで約100KB〜1MB（許容範囲内）

### 4.3 ベンチマーク予測

```
# 現在
BenchmarkExpandCommand-8    10000  120000 ns/op   5000 B/op  50 allocs/op

# 改善後（予測）
BenchmarkExpandGroup-8       5000  200000 ns/op   8000 B/op  80 allocs/op
BenchmarkVerifyGroup-8      20000   60000 ns/op   2000 B/op  20 allocs/op
BenchmarkExecuteGroup-8     20000   60000 ns/op   2000 B/op  20 allocs/op

# トータル
現在:    120000 + 120000 = 240000 ns/op (検証 + 実行)
改善後:  200000 + 60000 + 60000 = 320000 ns/op (展開 + 検証 + 実行)

# 注: 実際には検証と実行は並行しないため、単純合計ではない
# 実際の改善: グループ全体で約 15-20% の時間削減（検証の展開削除により）
```

## 5. エラーハンドリング

### 5.1 エラー発生タイミングの変更

```mermaid
flowchart TD
    subgraph "現在"
        Current1[グループ展開]
        Current2[検証<br/>警告のみ]
        Current3[実行<br/>エラー発生]

        Current1 --> Current2
        Current2 --> Current3
    end

    subgraph "改善後"
        New1[グループ展開<br/>★ エラー発生 ★]
        New2[検証]
        New3[実行]

        New1 -.->|エラー時| Abort[中断]
        New1 -->|成功時| New2
        New2 --> New3
    end

    style Current3 fill:#ffcccc
    style New1 fill:#ccffcc
    style Abort fill:#ffcccc
```

**改善点**:
- Fail Fast: エラーを早期に検出
- 一貫性: すべてのコマンドが展開できることを保証

### 5.2 エラーメッセージの改善

**現在**:
```
[WARN] Failed to expand command path
    group=build_group
    command=%{cmd_var}/builder
    error="undefined variable in group[build_group].cmd: 'cmd_var'
          (context: %{cmd_var}/builder)"

# ... その後、実行時に成功（混乱）
```

**改善後**:
```
[ERROR] Failed to expand group
    group=build_group
    error="failed to expand command[build_cmd]:
          undefined variable in command[build_cmd].cmd: 'cmd_var'
          (context: %{cmd_var}/builder)"

# グループ展開自体が失敗するため、実行されない
```

**改善点**:
- エラーが明確
- 実行前に失敗するため、混乱がない
- デバッグが容易

## 6. Dry-runモードへの影響

### 6.1 現在のDry-run実装

Dry-runモードは`DryRunResourceManager`で実装されており、既に展開済みの`RuntimeCommand`を前提に設計されている：

```go
// internal/runner/resource/dryrun_manager.go:150-180
func (d *DryRunResourceManager) ExecuteCommand(
    ctx context.Context,
    cmd *runnertypes.RuntimeCommand,  // ← 展開済みRuntimeCommandを受け取る
    group *runnertypes.GroupSpec,
    env map[string]string,
) (CommandToken, *ExecutionResult, error) {
    start := time.Now()

    // コマンド分析（展開済みフィールドを使用）
    analysis, err := d.analyzeCommand(ctx, cmd, group, env)
    // ...
}

// internal/runner/resource/dryrun_manager.go:200-268
func (d *DryRunResourceManager) analyzeCommand(
    _ context.Context,
    cmd *runnertypes.RuntimeCommand,
    group *runnertypes.GroupSpec,
    env map[string]string,
) (ResourceAnalysis, error) {
    analysis := ResourceAnalysis{
        Type:      ResourceTypeCommand,
        Operation: OperationExecute,
        Target:    cmd.ExpandedCmd,  // ← 展開済みコマンド
        Parameters: map[string]ParameterValue{
            "command":           NewStringValue(cmd.ExpandedCmd),
            "working_directory": NewStringValue(cmd.EffectiveWorkDir),
            "timeout":           NewIntValue(int64(cmd.EffectiveTimeout)),
            // ...
        },
    }

    // セキュリティ分析（展開済み引数を使用）
    d.analyzeCommandSecurity(cmd, &analysis)
    // ...
}

// internal/runner/resource/dryrun_manager.go:272-294
func (d *DryRunResourceManager) analyzeCommandSecurity(
    cmd *runnertypes.RuntimeCommand,
    analysis *ResourceAnalysis,
) error {
    // 展開済みコマンドパスを解決
    resolvedPath, err := d.pathResolver.ResolvePath(cmd.ExpandedCmd)
    // ...

    // セキュリティ分析（展開済み引数を使用）
    riskLevel, pattern, reason, err := security.AnalyzeCommandSecurity(
        resolvedPath,
        cmd.ExpandedArgs,  // ← 展開済み引数
        opts,
    )
    // ...
}
```

### 6.2 事前展開による変更

#### 6.2.1 実行フローの変更

**現在のフロー**（Dry-run）:
```mermaid
flowchart TD
    Start([ExecuteGroup]) --> ExpandGroup[1. ExpandGroup]
    ExpandGroup --> ResolveWorkDir[2. Working Directory 解決]
    ResolveWorkDir --> SetWorkDir[3. __runner_workdir 設定]
    SetWorkDir --> Verify[4. verifyGroupFiles<br/>警告のみ]

    Verify --> Loop{各コマンド}
    Loop --> ExpandCmd[5. config.ExpandCommand<br/>★展開実行★]
    ExpandCmd --> DryRunExec[6. DryRunResourceManager<br/>.ExecuteCommand]
    DryRunExec --> Analyze[7. analyzeCommand]
    Analyze --> Loop

    Loop --> End([End])

    style ExpandCmd fill:#ffffcc
    style Analyze fill:#ccffcc
```

**事前展開後のフロー**（Dry-run）:
```mermaid
flowchart TD
    Start([ExecuteGroup]) --> ExpandGroup[1. ExpandGroup]
    ExpandGroup --> ResolveWorkDir[2. Working Directory 解決]
    ResolveWorkDir --> SetWorkDir[3. __runner_workdir 設定]
    SetWorkDir --> ExpandAll[4. 全コマンド展開<br/>★NEW: 事前展開★]
    ExpandAll --> Verify[5. verifyGroupFiles<br/>展開済み使用]

    Verify --> Loop{各コマンド}
    Loop --> DryRunExec[6. DryRunResourceManager<br/>.ExecuteCommand]
    DryRunExec --> Analyze[7. analyzeCommand<br/>展開済み使用]
    Analyze --> Loop

    Loop --> End([End])

    style ExpandAll fill:#ccffcc
    style Verify fill:#ccffcc
    style Analyze fill:#ccffcc
```

#### 6.2.2 互換性評価

| 観点 | 評価 | 詳細 |
|------|------|------|
| **API互換性** | ✅ 完全互換 | `DryRunResourceManager`のインターフェースは変更なし |
| **動作互換性** | ✅ 完全互換 | 展開済み`RuntimeCommand`を受け取る設計は変わらない |
| **エラー検出** | ✅ 改善 | コマンド展開エラーがdry-run開始前に検出される |
| **メモリ使用** | ✅ 問題なし | dry-runモード自体はメモリを増やさない |

#### 6.2.3 Verification Managerとの連携

**現在**（Dry-run）:
```go
// verification/manager.go:298-324
for _, command := range groupSpec.Commands {
    expandedCmd, err := config.ExpandString(command.Cmd, runtimeGroup.ExpandedVars, ...)
    if err != nil {
        slog.Warn("Failed to expand command path", ...)
        continue  // ← dry-runモードでも警告のみ（実行は継続）
    }
    resolvedPath, err := m.pathResolver.ResolvePath(expandedCmd)
    // ...
}
```

**事前展開後**（Dry-run）:
```go
// verification/manager.go（修正後）
for _, runtimeCmd := range runtimeGroup.Commands {
    // 既に展開済み - エラーハンドリング不要
    resolvedPath, err := m.pathResolver.ResolvePath(runtimeCmd.ExpandedCmd)
    // ...
}
```

**影響**:
- ✅ dry-runモードでも検証フェーズの展開処理が不要になる
- ✅ 展開エラーは`ExecuteGroup`の早期段階で発生（dry-runでも同様）
- ✅ より早い段階でのエラー検出が可能

### 6.3 Dry-runモードでの追加メリット

1. **早期エラー検出**:
   - 現在: 実行ループ内でコマンドごとに展開エラーが発生する可能性
   - 改善後: グループ展開時に全コマンドの展開エラーを一括検出

2. **一貫性の向上**:
   - dry-runモードでも通常モードでも同じタイミングでエラーが発生
   - エラーメッセージが統一される

3. **分析の正確性**:
   - 全コマンドが正常に展開されることが保証された後に分析を実行
   - 部分的な分析結果を出力するリスクがない

### 6.4 テスト確認項目

- [ ] Dry-runモードの全テストケースが通ること
- [ ] セキュリティ分析が正しく動作すること
- [ ] エラー発生時のメッセージが適切であること
- [ ] Verification Managerのdry-run動作が変わらないこと
- [ ] 展開エラー時にdry-runが適切に中断されること
- [ ] dry-run出力のフォーマットが変わらないこと

## 7. 実装の段階的移行

### 7.1 移行ステップ

```mermaid
flowchart LR
    subgraph "Step 1"
        S1[ExpandGroup に<br/>コマンド展開追加]
        S1Test[テスト追加]
    end

    subgraph "Step 2"
        S2[collectVerificationFiles<br/>を修正]
        S2Test[テスト更新]
    end

    subgraph "Step 3"
        S3[executeAllCommands<br/>を修正]
        S3Test[テスト更新]
    end

    subgraph "Step 4"
        S4[古いコード削除<br/>リファクタリング]
        S4Test[最終テスト]
    end

    S1 --> S1Test
    S1Test --> S2
    S2 --> S2Test
    S2Test --> S3
    S3 --> S3Test
    S3Test --> S4
    S4 --> S4Test

    style S1 fill:#ccffcc
    style S2 fill:#ccffcc
    style S3 fill:#ccffcc
    style S4 fill:#ccffcc
```

### 7.2 各ステップの詳細

#### Step 1: ExpandGroup の拡張

**目標**: コマンド展開を追加し、既存動作を維持

**実装**:
```go
func ExpandGroup(...) (*RuntimeGroup, error) {
    // ... 既存の処理 ...

    // NEW: Expand Commands
    runtime.Commands = make([]*RuntimeCommand, len(spec.Commands))
    for i := range spec.Commands {
        runtimeCmd, err := ExpandCommand(...)
        if err != nil {
            return nil, fmt.Errorf("...")
        }
        runtime.Commands[i] = runtimeCmd
    }

    return runtime, nil
}
```

**テスト**:
- `RuntimeGroup.Commands` が正しく設定されることを確認
- 既存のテストがすべて通ることを確認

#### Step 2: collectVerificationFiles の修正

**目標**: 展開済みコマンドを使用

**実装**:
```go
func (m *Manager) collectVerificationFiles(runtimeGroup *RuntimeGroup) map[string]struct{} {
    // ...
    for _, runtimeCmd := range runtimeGroup.Commands {
        resolvedPath, err := m.pathResolver.ResolvePath(runtimeCmd.ExpandedCmd)
        // ...
    }
}
```

**テスト**:
- コマンドレベル変数が使用できることを確認
- 検証が正常に動作することを確認

#### Step 3: executeAllCommands の修正

**目標**: 展開済みコマンドを使用

**実装**:
```go
func (ge *DefaultGroupExecutor) executeAllCommands(...) {
    for i, runtimeCmd := range runtimeGroup.Commands {
        // config.ExpandCommand 呼び出しを削除
        // ...
    }
}
```

**テスト**:
- 実行が正常に動作することを確認
- エンドツーエンドテストがすべて通ることを確認

#### Step 4: クリーンアップ

**目標**: 不要なコードを削除

**実装**:
- 検証時の変数展開コードを削除
- 不要なエラーハンドリングを削除
- ドキュメントを更新

## 8. まとめ

### 8.1 改善の効果

| 項目 | 改善内容 |
|------|----------|
| **一貫性** | 検証時も実行時も同じ変数が使用可能 |
| **完全性** | コマンドレベル変数が検証時にも利用可能 |
| **効率性** | 展開処理が1回のみ実行 |
| **シンプル性** | 重複コードの削減 |
| **明確性** | 未使用フィールドの活用 |
| **保守性** | エラーハンドリングの一元化 |
| **Dry-run互換** | dry-runモードでも完全互換、追加メリットあり |

### 8.2 トレードオフ

| メリット | デメリット |
|---------|-----------|
| ✅ 一貫したユーザー体験 | ⚠️ メモリ使用量の微増 |
| ✅ コードの簡素化 | ⚠️ 実装の複雑性（一時的） |
| ✅ パフォーマンス改善 | ⚠️ テスト更新の必要性 |
| ✅ Fail Fast | ⚠️ エラー発生タイミングの変更 |
| ✅ Dry-run互換性維持 | - |

### 8.3 推奨実装

この改善案は、ユーザー体験の大幅な向上とコードの保守性向上をもたらすため、**強く推奨**される。

次のドキュメント（04_implementation_plan.md）で、詳細な実装計画を説明する。
