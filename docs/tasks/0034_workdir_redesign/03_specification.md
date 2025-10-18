# 詳細仕様書: 作業ディレクトリ仕様の再設計

## 1. 概要

本ドキュメントは、タスク0034「作業ディレクトリ仕様の再設計」の詳細仕様を記述します。

## 2. 型定義

### 2.1 設定型の変更

#### 2.1.1 削除対象フィールド

**変更対象**: `internal/runner/types/config_types.go`

```go
// 変更前
type GlobalConfig struct {
    WorkDir string `toml:"workdir"`  // 削除対象
    // ... その他のフィールド
}

type CommandGroup struct {
    TempDir bool   `toml:"temp_dir"`  // 削除対象
    WorkDir string `toml:"workdir"`
    // ... その他のフィールド
}

type Command struct {
    Dir string `toml:"dir"`  // 削除対象（変更対象）
    // ... その他のフィールド
}

// 変更後
type GlobalConfig struct {
    // WorkDir は削除（グローバルレベルでのデフォルト設定は廃止）
    // ... その他のフィールド
}

type CommandGroup struct {
    // TempDir は削除（デフォルトで一時ディレクトリを使用するため不要）
    WorkDir string `toml:"workdir"`
    // ... その他のフィールド
}

type Command struct {
    WorkDir string `toml:"workdir"`  // 名称変更（旧: dir）
    // ... その他のフィールド
}
```

#### 2.1.2 破壊的変更への対応

**既存設定ファイルの移行**:

| 旧設定 | 新設定 | 対応 |
|-------|-------|------|
| `[global]` `workdir = "/tmp"` | 削除 | エラー（未知フィールド） |
| `temp_dir = true` | 削除 | エラー（未知フィールド） |
| `dir = "/path"` | `workdir = "/path"` | エラー（未知フィールド） |

**エラーメッセージ例**:
```
Error: unknown field 'workdir' in section [global]
Error: unknown field 'temp_dir' in section [[groups]]
Error: unknown field 'dir' in section [[groups.commands]]
```

### 2.2 実行時型

```go
// GroupContext: グループ実行時のコンテキスト
// パッケージ: internal/runner/executor
type GroupContext struct {
    // GroupName: グループ名
    GroupName string

    // WorkDir: 実際に使用されるワークディレクトリの絶対パス
    // - グループレベル workdir が指定: その値
    // - グループレベル workdir が未指定: 自動生成された一時ディレクトリパス
    WorkDir string

    // IsTempDir: true = 一時ディレクトリ、false = 固定ディレクトリ
    IsTempDir bool

    // TempDirPath: 一時ディレクトリの場合のパス
    // IsTempDir=false の場合は空文字列
    TempDirPath string

    // KeepTempDirs: --keep-temp-dirs フラグの値
    KeepTempDirs bool

    // その他の実行時情報（将来の拡張用）
    // Metadata map[string]interface{}
}

// ExecutionOptions: グループ実行時のオプション
// パッケージ: internal/runner/executor
type ExecutionOptions struct {
    // KeepTempDirs: --keep-temp-dirs フラグ
    KeepTempDirs bool

    // DryRun: ドライランモード（コマンド実行しない）
    DryRun bool

    // その他のオプション...
}
```

## 3. API 仕様

### 3.1 TempDirManager インターフェース

**パッケージ**: `internal/runner/executor`

```go
// TempDirManager: 一時ディレクトリの生成・管理・削除
type TempDirManager interface {
    // CreateTempDir: グループの一時ディレクトリを生成
    //
    // 引数:
    //   groupName: グループ名
    //
    // 戻り値:
    //   string: 生成された一時ディレクトリの絶対パス
    //   error: エラー（例: パーミッションエラー、ディスク容量不足）
    //
    // 動作:
    //   1. プレフィックス "scr-<groupName>-" でランダムなディレクトリを生成
    //   2. パーミッションは 0700 に設定
    //   3. INFO レベルでログ出力
    //
    // 例:
    //   tempDir, err := mgr.CreateTempDir("backup")
    //   // → "/tmp/scr-backup-a1b2c3d4"
    CreateTempDir(groupName string) (string, error)

    // CleanupTempDir: 一時ディレクトリを削除
    //
    // 引数:
    //   tempDirPath: 削除対象のディレクトリパス
    //   keepTempDirs: --keep-temp-dirs フラグの値
    //
    // 戻り値:
    //   error: エラー（例: アクセス権限なし）
    //           返されたエラーは記録されるが、処理は継続される
    //
    // 動作:
    //   1. keepTempDirs=true の場合: 削除しない（INFO ログ出力）
    //   2. IsTempDir() で一時ディレクトリか確認
    //   3. 一時ディレクトリの場合のみ削除
    //   4. 削除成功: DEBUG ログ
    //   5. 削除失敗: ERROR ログ + 標準エラー出力
    //
    // 例:
    //   err := mgr.CleanupTempDir("/tmp/scr-backup-a1b2c3d4", false)
    //   // 削除成功の場合 err=nil、失敗の場合 err != nil
    //   // ただし呼び出し元の処理は継続される
    CleanupTempDir(tempDirPath string, keepTempDirs bool) error

    // IsTempDir: 与えられたパスが一時ディレクトリか判定
    //
    // 引数:
    //   tempDirPath: 判定対象のパス
    //
    // 戻り値:
    //   bool: true = 一時ディレクトリ、false = 固定ディレクトリ
    //
    // 動作:
    //   ディレクトリ名の basename が "scr-" プレフィックスを持つか確認
    //
    // 例:
    //   mgr.IsTempDir("/tmp/scr-backup-a1b2c3d4")  // true
    //   mgr.IsTempDir("/var/data")                  // false
    IsTempDir(tempDirPath string) bool
}
```

### 3.2 GroupExecutor インターフェース

**パッケージ**: `internal/runner/executor`

```go
// GroupExecutor: グループ実行の制御
type GroupExecutor interface {
    // ExecuteGroups: 設定のすべてのグループを実行
    //
    // 引数:
    //   config: 設定オブジェクト
    //   opts: 実行オプション
    //
    // 戻り値:
    //   error: エラー（グループまたはコマンド実行失敗）
    //
    // 動作:
    //   1. グループループを開始
    //   2. 各グループに対して ExecuteGroup() を呼び出し
    //   3. グループ実行失敗時の処理（中止/継続）は設定で決定
    ExecuteGroups(config *runnertypes.Config, opts *ExecutionOptions) error

    // ExecuteGroup: 1つのグループを実行
    //
    // 引数:
    //   group: グループ設定
    //   opts: 実行オプション
    //
    // 戻り値:
    //   *GroupContext: グループ実行コンテキスト（外部参照不可）
    //   error: エラー（コマンド実行失敗など）
    //
    // ライフサイクル:
    //   1. ワークディレクトリを決定（resolveGroupWorkDir）
    //   2. 一時ディレクトリの場合は生成（TempDirManager.CreateTempDir）
    //   3. GroupContext を作成
    //   4. defer CleanupTempDir を登録 ← 重要: エラー時も実行
    //   5. コマンド実行ループ
    //   6. グループ実行終了（成功・失敗問わず） → defer 実行
    ExecuteGroup(group *runnertypes.CommandGroup, opts *ExecutionOptions) error
}
```

### 3.3 VariableExpander インターフェース

**パッケージ**: `internal/runner/expansion`

```go
// VariableExpander: コマンド内の変数を展開
type VariableExpander interface {
    // ExpandCommand: コマンド内の %{__runner_workdir} を展開
    //
    // 引数:
    //   groupCtx: グループコンテキスト（WorkDir を含む）
    //   cmd: 元のコマンド設定
    //
    // 戻り値:
    //   *runnertypes.Command: 展開済みコマンド（新規オブジェクト）
    //   error: エラー（パス検証失敗など）
    //
    // 動作:
    //   1. cmd.Cmd を展開
    //   2. cmd.Args 各要素を展開
    //   3. cmd.WorkDir を展開
    //   4. 絶対パス化（パストラバーサル検証含む）
    //   5. 原本 cmd は変更しない
    //
    // 例:
    //   cmd := &runnertypes.Command{
    //       Cmd: "cp",
    //       Args: []string{"%{__runner_workdir}/dump.sql", "/backup/"},
    //   }
    //   expanded, _ := expander.ExpandCommand(groupCtx, cmd)
    //   // expanded.Args[0] = "/tmp/scr-backup-a1b2c3d4/dump.sql"
    ExpandCommand(groupCtx *GroupContext, cmd *runnertypes.Command) (*runnertypes.Command, error)

    // ExpandString: 文字列内の %{__runner_workdir} を展開
    //
    // 引数:
    //   groupCtx: グループコンテキスト
    //   str: 変数を含む元の文字列
    //
    // 戻り値:
    //   string: 展開済み文字列
    //   error: エラー（パス検証失敗など）
    //
    // 動作:
    //   1. %{__runner_workdir} を groupCtx.WorkDir に置換
    //   2. パス文字列の場合は絶対パス化
    //   3. パストラバーサル検証
    //
    // 例:
    //   str := "/data/%{__runner_workdir}/report.txt"
    //   expanded, _ := expander.ExpandString(groupCtx, str)
    //   // expanded = "/data//tmp/scr-backup-a1b2c3d4/report.txt"
    ExpandString(groupCtx *GroupContext, str string) (string, error)
}
```

### 3.4 CommandExecutor への統合

**パッケージ**: `internal/runner/executor`

```go
// CommandExecutor.Execute() の変更
// 既存のシグネチャは変更しない
func (e *DefaultCommandExecutor) Execute(
    ctx context.Context,
    cmd *runnertypes.Command,
    groupCtx *GroupContext,  // 新パラメータ
) (output.Output, error) {
    // 1. VariableExpander で変数展開
    expandedCmd, err := e.varExpander.ExpandCommand(groupCtx, cmd)
    if err != nil {
        return output.Output{}, fmt.Errorf("failed to expand variables: %w", err)
    }

    // 2. ワークディレクトリを決定
    workDir := resolveWorkDir(expandedCmd, groupCtx)

    // 3. コマンド実行
    // (既存のロジックに workDir を適用)

    return output.Output{}, nil
}

// resolveWorkDir: 実際に使用するワークディレクトリを決定
// 優先度: Command.WorkDir > Group.WorkDir > カレントディレクトリ
func resolveWorkDir(cmd *runnertypes.Command, groupCtx *GroupContext) string {
    if cmd.WorkDir != "" {
        return cmd.WorkDir  // 優先度1: コマンドレベル
    }
    return groupCtx.WorkDir  // 優先度2: グループレベル
}
```

## 4. ワークディレクトリ決定ロジック

### 4.1 優先順位（新規）

```
Level 1: コマンドレベル (Command.WorkDir)
  └─ 指定: そのパスを使用
  └─ 未指定: Level 2 へ

Level 2: グループレベル (Group.WorkDir)
  └─ 指定: そのパスを使用
  └─ 未指定: Level 3 へ

Level 3: 自動生成一時ディレクトリ（デフォルト）
  └─ /tmp/scr-<groupName>-XXXXXX を自動生成
```

### 4.2 決定アルゴリズム

```go
// resolveGroupWorkDir: グループのワークディレクトリを決定
// 戻り値: (ワークディレクトリ, 一時ディレクトリフラグ, エラー)
func resolveGroupWorkDir(
    groupConfig *runnertypes.CommandGroup,
    tempDirMgr TempDirManager,
) (string, bool, error) {
    // グループレベル WorkDir が指定されている?
    if groupConfig.WorkDir != "" {
        // 固定ディレクトリを使用
        return groupConfig.WorkDir, false, nil
    }

    // 自動一時ディレクトリを生成
    tempDir, err := tempDirMgr.CreateTempDir(groupConfig.Name)
    if err != nil {
        return "", false, fmt.Errorf("failed to create temp directory: %w", err)
    }

    return tempDir, true, nil
}

// resolveCommandWorkDir: コマンドのワークディレクトリを決定
func resolveCommandWorkDir(
    cmd *runnertypes.Command,
    groupCtx *GroupContext,
) string {
    // コマンドレベル WorkDir が指定されている?
    if cmd.WorkDir != "" {
        return cmd.WorkDir
    }

    // グループのワークディレクトリを使用
    return groupCtx.WorkDir
}
```

## 5. 一時ディレクトリ管理の実装

### 5.1 DefaultTempDirManager の実装

**ファイル**: `internal/runner/executor/tempdir_manager.go`

```go
package executor

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "go-safe-cmd-runner/internal/logging"
)

// DefaultTempDirManager: 一時ディレクトリの標準実装
type DefaultTempDirManager struct {
    logger logging.Logger
}

// NewDefaultTempDirManager: 新規インスタンスを作成
func NewDefaultTempDirManager(logger logging.Logger) *DefaultTempDirManager {
    return &DefaultTempDirManager{
        logger: logger,
    }
}

// CreateTempDir: 一時ディレクトリを生成
func (m *DefaultTempDirManager) CreateTempDir(groupName string) (string, error) {
    // プレフィックス: "scr-<groupName>-"
    prefix := fmt.Sprintf("scr-%s-", groupName)

    // OS の TempDir() 関数を使用
    baseTmpDir := os.TempDir()

    // MkdirTemp でランダムディレクトリを生成（パーミッション 0700）
    tempDir, err := os.MkdirTemp(baseTmpDir, prefix)
    if err != nil {
        return "", fmt.Errorf("failed to create temporary directory: %w", err)
    }

    // ログ出力 (INFO レベル)
    m.logger.Info(fmt.Sprintf(
        "Created temporary directory for group '%s': %s",
        groupName, tempDir,
    ))

    return tempDir, nil
}

// CleanupTempDir: 一時ディレクトリを削除
func (m *DefaultTempDirManager) CleanupTempDir(
    tempDirPath string,
    keepTempDirs bool,
) error {
    // --keep-temp-dirs フラグが指定されている場合は削除しない
    if keepTempDirs {
        m.logger.Info(fmt.Sprintf(
            "Keeping temporary directory (--keep-temp-dirs): %s",
            tempDirPath,
        ))
        return nil
    }

    // 一時ディレクトリか確認
    if !m.IsTempDir(tempDirPath) {
        // 固定ディレクトリの場合は削除しない
        return nil
    }

    // ディレクトリを削除
    err := os.RemoveAll(tempDirPath)
    if err != nil {
        // エラーログ出力 (ERROR レベル)
        m.logger.Error(fmt.Sprintf(
            "Failed to cleanup temporary directory: %s: %v",
            tempDirPath, err,
        ))

        // 標準エラー出力
        fmt.Fprintf(os.Stderr,
            "Warning: Failed to cleanup temporary directory: %s\n",
            tempDirPath,
        )

        return fmt.Errorf("failed to cleanup temporary directory: %w", err)
    }

    // ログ出力 (DEBUG レベル)
    m.logger.Debug(fmt.Sprintf(
        "Cleaned up temporary directory: %s",
        tempDirPath,
    ))

    return nil
}

// IsTempDir: パスが一時ディレクトリか判定
func (m *DefaultTempDirManager) IsTempDir(tempDirPath string) bool {
    baseName := filepath.Base(tempDirPath)
    return strings.HasPrefix(baseName, "scr-")
}
```

### 5.2 一時ディレクトリ生成の命名規則

```
プレフィックス: "scr-"
グループ名: "<group>"
ランダムサフィックス: "XXXXXX" (OS が生成)

例:
  グループ "backup" → /tmp/scr-backup-a1b2c3d4e5f6
  グループ "build"  → /tmp/scr-build-f7g8h9i0j1k2
  グループ "test"   → /tmp/scr-test-l3m4n5o6p7q8
```

## 6. 変数展開の実装

### 6.1 DefaultVariableExpander の実装

**ファイル**: `internal/runner/expansion/variable_expander.go`

```go
package expansion

import (
    "fmt"
    "path/filepath"
    "strings"

    "go-safe-cmd-runner/internal/logging"
    "go-safe-cmd-runner/internal/runner/executor"
    "go-safe-cmd-runner/internal/runner/types"
)

// DefaultVariableExpander: %{__runner_workdir} の展開実装
type DefaultVariableExpander struct {
    logger logging.Logger
}

// NewDefaultVariableExpander: 新規インスタンスを作成
func NewDefaultVariableExpander(logger logging.Logger) *DefaultVariableExpander {
    return &DefaultVariableExpander{
        logger: logger,
    }
}

// ExpandCommand: コマンド内の変数を展開
func (e *DefaultVariableExpander) ExpandCommand(
    groupCtx *executor.GroupContext,
    cmd *types.Command,
) (*types.Command, error) {
    expandedCmd := &types.Command{
        Name: cmd.Name,
        // ... その他のフィールド（コピー）
    }

    var err error

    // Cmd の展開
    expandedCmd.Cmd, err = e.ExpandString(groupCtx, cmd.Cmd)
    if err != nil {
        return nil, fmt.Errorf("failed to expand cmd: %w", err)
    }

    // Args の展開
    expandedCmd.Args = make([]string, len(cmd.Args))
    for i, arg := range cmd.Args {
        expandedCmd.Args[i], err = e.ExpandString(groupCtx, arg)
        if err != nil {
            return nil, fmt.Errorf("failed to expand args[%d]: %w", i, err)
        }
    }

    // WorkDir の展開
    if cmd.WorkDir != "" {
        expandedCmd.WorkDir, err = e.ExpandString(groupCtx, cmd.WorkDir)
        if err != nil {
            return nil, fmt.Errorf("failed to expand workdir: %w", err)
        }
    }

    return expandedCmd, nil
}

// ExpandString: 文字列内の %{__runner_workdir} を展開
func (e *DefaultVariableExpander) ExpandString(
    groupCtx *executor.GroupContext,
    str string,
) (string, error) {
    // %{__runner_workdir} が含まれていない場合はそのまま返す
    if !strings.Contains(str, "%{__runner_workdir}") {
        return str, nil
    }

    // %{__runner_workdir} を置換
    expanded := strings.ReplaceAll(str, "%{__runner_workdir}", groupCtx.WorkDir)

    // 絶対パス化
    absPath, err := filepath.Abs(expanded)
    if err != nil {
        return "", fmt.Errorf("failed to resolve absolute path: %w", err)
    }

    // パス検証（トラバーサル攻撃防止）
    if err := validatePath(absPath); err != nil {
        return "", fmt.Errorf("invalid path after expansion: %w", err)
    }

    return absPath, nil
}

// validatePath: パストラバーサル攻撃を防ぐ
func validatePath(path string) error {
    // 絶対パスのみ許可
    if !filepath.IsAbs(path) {
        return fmt.Errorf("path must be absolute: %s", path)
    }
    // パストラバーサル攻撃を防ぐため、パスコンポーネントに ".." が含まれることを禁止する
    for _, part := range strings.Split(path, string(filepath.Separator)) {
        if part == ".." {
            return fmt.Errorf("path contains '..' component, which is a security risk: %s", path)
        }
    }
    return nil
}
```

### 6.2 変数展開の統合点

**ファイル**: `internal/runner/executor/command_executor.go`

```go
// CommandExecutor.Execute() への統合
func (e *DefaultCommandExecutor) Execute(
    ctx context.Context,
    cmd *types.Command,
    groupCtx *executor.GroupContext,
) (output.Output, error) {
    // 1. VariableExpander で変数展開
    expandedCmd, err := e.varExpander.ExpandCommand(groupCtx, cmd)
    if err != nil {
        return output.Output{}, fmt.Errorf("failed to expand command: %w", err)
    }

    // 2. ワークディレクトリを決定
    workDir := e.resolveWorkDir(expandedCmd, groupCtx)

    // 3. 以下、既存のコマンド実行ロジック
    // (workDir を適用)

    return output.Output{}, nil
}

// resolveWorkDir: 実際に使用するワークディレクトリを決定
func (e *DefaultCommandExecutor) resolveWorkDir(
    cmd *types.Command,
    groupCtx *executor.GroupContext,
) string {
    // 優先度1: コマンドレベル WorkDir
    if cmd.WorkDir != "" {
        return cmd.WorkDir
    }

    // 優先度2: グループレベル WorkDir
    return groupCtx.WorkDir
}
```

## 7. コマンドラインオプション

### 7.1 `--keep-temp-dirs` フラグの実装

**ファイル**: `cmd/runner/main.go`

```go
package main

import (
    "flag"
    "fmt"
    "os"

    "go-safe-cmd-runner/internal/runner"
)

var (
    configPath   = flag.String("config", "", "Configuration file path")
    keepTempDirs = flag.Bool(
        "keep-temp-dirs",
        false,
        "Keep temporary directories after execution (for debugging)",
    )
)

func main() {
    flag.Parse()

    if *configPath == "" {
        fmt.Fprintf(os.Stderr, "Error: --config is required\n")
        os.Exit(1)
    }

    // 設定読み込み
    config, err := runner.LoadConfig(*configPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: Failed to load config: %v\n", err)
        os.Exit(1)
    }

    // グループ実行オプション
    opts := &runner.ExecutionOptions{
        KeepTempDirs: *keepTempDirs,
    }

    // グループ実行
    err = runner.ExecuteGroups(config, opts)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

### 7.2 使用例

```bash
# 通常実行（一時ディレクトリ自動削除）
$ ./runner --config backup.toml

# デバッグ実行（一時ディレクトリ保持）
$ ./runner --config backup.toml --keep-temp-dirs

# 実行後に一時ディレクトリを確認
$ ls -la /tmp/scr-backup-*/
$ cat /tmp/scr-backup-*/dump.sql
```

## 8. GroupExecutor の実装

### 8.1 グループ実行のライフサイクル

**ファイル**: `internal/runner/executor/group_executor.go`

```go
package executor

import (
    "context"
    "fmt"

    "go-safe-cmd-runner/internal/logging"
    "go-safe-cmd-runner/internal/runner/types"
)

// DefaultGroupExecutor: グループ実行の標準実装
type DefaultGroupExecutor struct {
    logger       logging.Logger
    tempDirMgr   TempDirManager
    varExpander  VariableExpander
    cmdExecutor  CommandExecutor
}

// NewDefaultGroupExecutor: 新規インスタンスを作成
func NewDefaultGroupExecutor(
    logger logging.Logger,
    tempDirMgr TempDirManager,
    varExpander VariableExpander,
    cmdExecutor CommandExecutor,
) *DefaultGroupExecutor {
    return &DefaultGroupExecutor{
        logger:      logger,
        tempDirMgr:  tempDirMgr,
        varExpander: varExpander,
        cmdExecutor: cmdExecutor,
    }
}

// ExecuteGroup: 1つのグループを実行
func (e *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    group *types.CommandGroup,
    opts *ExecutionOptions,
) error {
    // ステップ1: ワークディレクトリを決定
    workDir, isTempDir, err := e.resolveGroupWorkDir(group)
    if err != nil {
        return fmt.Errorf("failed to resolve work directory: %w", err)
    }

    // ステップ2: GroupContext を作成
    groupCtx := &GroupContext{
        GroupName:    group.Name,
        WorkDir:      workDir,
        IsTempDir:    isTempDir,
        TempDirPath:  "", // 後で設定
        KeepTempDirs: opts.KeepTempDirs,
    }
    if isTempDir {
        groupCtx.TempDirPath = workDir
    }

    // ステップ3: defer でクリーンアップを登録（重要: エラー時も実行）
    defer func() {
        if isTempDir {
            err := e.tempDirMgr.CleanupTempDir(groupCtx.TempDirPath, opts.KeepTempDirs)
            if err != nil {
                // エラーをログするが、処理は継続
                e.logger.Error(fmt.Sprintf("Cleanup warning: %v", err))
            }
        }
    }()

    // ステップ4: コマンド実行ループ
    for _, cmd := range group.Commands {
        // コマンド実行
        output, err := e.cmdExecutor.Execute(ctx, cmd, groupCtx)
        if err != nil {
            return fmt.Errorf("command '%s' failed: %w", cmd.Name, err)
        }

        // 出力ハンドリング（既存ロジック）
        e.handleCommandOutput(cmd, output)
    }

    return nil
}

// resolveGroupWorkDir: グループのワークディレクトリを決定
func (e *DefaultGroupExecutor) resolveGroupWorkDir(
    group *types.CommandGroup,
) (string, bool, error) {
    // グループレベル WorkDir が指定されている?
    if group.WorkDir != "" {
        // 固定ディレクトリを使用
        e.logger.Info(fmt.Sprintf(
            "Using group workdir for '%s': %s",
            group.Name, group.WorkDir,
        ))
        return group.WorkDir, false, nil
    }

    // 自動一時ディレクトリを生成
    tempDir, err := e.tempDirMgr.CreateTempDir(group.Name)
    if err != nil {
        return "", false, err
    }

    return tempDir, true, nil
}

// handleCommandOutput: コマンド出力を処理（既存ロジック）
func (e *DefaultGroupExecutor) handleCommandOutput(
    cmd *types.Command,
    output Output,
) {
    // 既存のハンドリングロジック
}
```

## 9. ロギング仕様

### 9.1 ログレベルと出力

| イベント | ログレベル | 出力先 | 形式 |
|---------|-----------|--------|------|
| 一時ディレクトリ作成 | INFO | ログ | `Created temporary directory for group 'X': /path` |
| 一時ディレクトリ削除成功 | DEBUG | ログ | `Cleaned up temporary directory: /path` |
| 一時ディレクトリ削除失敗 | ERROR | ログ+stderr | `Failed to cleanup temporary directory: /path: error` |
| keep-temp-dirs フラグ | INFO | ログ | `Keeping temporary directory (--keep-temp-dirs): /path` |

### 9.2 ログ出力の実装例

```go
// 一時ディレクトリ作成時
logger.Info(fmt.Sprintf("Created temporary directory for group '%s': %s", groupName, tempDir))

// 一時ディレクトリ削除成功時
logger.Debug(fmt.Sprintf("Cleaned up temporary directory: %s", tempDir))

// 一時ディレクトリ削除失敗時
logger.Error(fmt.Sprintf("Failed to cleanup temporary directory: %s: %v", tempDir, err))
fmt.Fprintf(os.Stderr, "Warning: Failed to cleanup temporary directory: %s\n", tempDir)

// keep-temp-dirs フラグ指定時
logger.Info(fmt.Sprintf("Keeping temporary directory (--keep-temp-dirs): %s", tempDir))
```

## 10. エラーハンドリング

### 10.1 エラータイプの定義

```go
// ErrTempDirCreationFailed: 一時ディレクトリ生成失敗
type ErrTempDirCreationFailed struct {
    GroupName string
    Err       error
}

// ErrTempDirCleanupFailed: 一時ディレクトリ削除失敗
type ErrTempDirCleanupFailed struct {
    TempDir string
    Err     error
}

// ErrVariableExpansionFailed: 変数展開失敗
type ErrVariableExpansionFailed struct {
    CommandName string
    Reason      string
    Err         error
}

// ErrInvalidPath: パス検証エラー
type ErrInvalidPath struct {
    Path   string
    Reason string
}
```

### 10.2 エラーハンドリングのポリシー

| エラー | 処理 | 例外動作 |
|-------|------|--------|
| 一時ディレクトリ生成失敗 | グループ実行中止 | 処理は継続しない |
| 一時ディレクトリ削除失敗 | ログのみ | 処理は継続する |
| 変数展開エラー | コマンド実行中止 | グループは継続（設定依存） |
| パス検証エラー | エラーを返す | グループ実行中止 |

## 11. パス検証

### 11.1 検証ルール

```go
func validatePath(path string) error {
    // ルール1: 絶対パスのみ許可
    if !filepath.IsAbs(path) {
        return fmt.Errorf("path must be absolute: %s", path)
    }

    // ルール2: パストラバーサル攻撃を防ぐため、".." コンポーネントを禁止
    // filepath.Clean() は // を正規化してしまうため、コンポーネント単位で検証
    for _, part := range strings.Split(path, string(filepath.Separator)) {
        if part == ".." {
            return fmt.Errorf("path contains '..' component, which is a security risk: %s", path)
        }
    }

    // ルール3: シンボリックリンク検証
    // (既存の SafeFileIO の仕組みを活用)

    return nil
}
```

## 12. TOML 設定スキーマ

### 12.1 新しいスキーマ

```toml
# グローバルレベル
[global]
# workdir は削除（廃止）

# グループレベル
[[groups]]
name = "backup"
workdir = "/var/backup"  # オプション（指定時は固定ディレクトリ）

[[groups.commands]]
name = "dump"
cmd = "pg_dump"
args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]
# workdir は指定不可（コマンドレベルで指定）

# グループレベル workdir が未指定 → 一時ディレクトリを自動生成
[[groups]]
name = "temp_backup"
# workdir 未指定 → /tmp/scr-temp_backup-XXXXXX が自動生成

[[groups.commands]]
name = "compress"
cmd = "gzip"
args = ["%{__runner_workdir}/dump.sql"]

# コマンドレベル workdir
[[groups]]
name = "mixed"

[[groups.commands]]
name = "cmd1"
cmd = "echo"
args = ["test"]
workdir = "/opt/app"  # このコマンドのみ /opt/app で実行

[[groups.commands]]
name = "cmd2"
cmd = "echo"
args = ["test"]
# workdir 未指定 → グループレベル workdir を使用（自動一時ディレクトリ）
```

### 12.2 削除されたフィールド

```toml
# 以下は削除（エラーになる）

[global]
workdir = "/tmp"  # ← 削除

[[groups]]
temp_dir = true   # ← 削除

[[groups.commands]]
dir = "/path"     # ← workdir に変更
```

## 13. テスト仕様

### 13.1 単体テストケース

#### T001: TempDirManager.CreateTempDir
```go
func TestCreateTempDir(t *testing.T) {
    // テスト条件:
    // - グループ名: "test"
    // - 期待: /tmp/scr-test-XXXXXX が生成される
    // - 確認: ディレクトリ存在、パーミッション 0700、ログ出力
}
```

#### T002: TempDirManager.CleanupTempDir
```go
func TestCleanupTempDir(t *testing.T) {
    // テスト条件:
    // - keepTempDirs: false
    // - 期待: ディレクトリが削除される
    // - 確認: ディレクトリ削除、DEBUG ログ

    // テスト条件:
    // - keepTempDirs: true
    // - 期待: ディレクトリが保持される
    // - 確認: ディレクトリ存在、INFO ログ

    // テスト条件:
    // - 削除失敗（パーミッション拒否）
    // - 期待: エラーを返す
    // - 確認: ERROR ログ、標準エラー出力
}
```

#### T003: VariableExpander.ExpandCommand
```go
func TestExpandCommand(t *testing.T) {
    // テスト条件:
    // - コマンド: cp %{__runner_workdir}/file /dest
    // - groupCtx.WorkDir: /tmp/scr-test-XXXXXX
    // - 期待: args[0] = /tmp/scr-test-XXXXXX/file
}
```

#### T004: VariableExpander.ExpandString
```go
func TestExpandString(t *testing.T) {
    // テスト条件:
    // - 文字列: /data/%{__runner_workdir}/report.json
    // - groupCtx.WorkDir: /tmp/scr-test-XXXXXX
    // - 期待: /data//tmp/scr-test-XXXXXX/report.json（絶対パス化）
}
```

### 13.2 統合テストケース

#### T005: グループ実行全体
```go
func TestExecuteGroupWithTempDir(t *testing.T) {
    // テスト条件:
    // - グループレベル workdir: 未指定
    // - コマンド1: ファイル作成（%{__runner_workdir}/file）
    // - コマンド2: ファイル検証
    // - 期待: 同じ一時ディレクトリで実行、グループ終了後削除
}
```

## 14. 設定ロードエラーハンドリング

### 14.1 廃止フィールド検出

**実装**: `internal/runner/config/loader.go`

```go
// 廃止フィールドのバリデーション
func (l *Loader) validateDeprecatedFields(cfg *runnertypes.Config) error {
    // Global.WorkDir の検出
    // (TOML パーサーで自動的に "unknown field" エラーになる)

    // Group.TempDir の検出
    // (TOML パーサーで自動的に "unknown field" エラーになる)

    // Command.Dir の検出
    // (TOML パーサーで自動的に "unknown field" エラーになる)

    return nil
}
```

### 14.2 エラーメッセージ

```
Error: toml: unmarshal error
  in the file at line X
  key 'workdir' is not valid in the [global] section
  (この field は廃止されました)

Error: toml: unmarshal error
  in the file at line X
  key 'temp_dir' is not valid in the [[groups]] section
  (この field は廃止されました)

Error: toml: unmarshal error
  in the file at line X
  key 'dir' is not valid in the [[groups.commands]] section
  (Field は 'workdir' に名称変更されました)
```

## 15. 実装チェックリスト

### Phase 1: 型定義
- [ ] `Global.WorkDir` を削除
- [ ] `Group.TempDir` を削除
- [ ] `Command.Dir` → `Command.WorkDir` に変更
- [ ] `GroupContext` 型を定義
- [ ] `ExecutionOptions` 型を定義

### Phase 2: 一時ディレクトリ機能
- [ ] `TempDirManager` インターフェース定義
- [ ] `DefaultTempDirManager` 実装
- [ ] `--keep-temp-dirs` フラグ実装
- [ ] `GroupExecutor` に統合
- [ ] `defer` でクリーンアップ登録

### Phase 3: 変数展開
- [ ] `VariableExpander` インターフェース定義
- [ ] `DefaultVariableExpander` 実装
- [ ] `CommandExecutor` に統合
- [ ] パストラバーサル検証

### Phase 4: テスト
- [ ] 単体テスト実装
- [ ] 統合テスト実装
- [ ] エラーケーステスト

### Phase 5: ドキュメント
- [ ] ユーザードキュメント更新
- [ ] サンプルファイル更新
- [ ] CHANGELOG 更新

## まとめ

本詳細仕様書は、タスク0034の実装に必要な詳細な仕様を定義します。

**重要なポイント**:
- 3階層 → 2階層への簡素化
- デフォルトで一時ディレクトリ使用（セキュリティ向上）
- `defer` で確実にクリーンアップ（Fail-Safe）
- `%{__runner_workdir}` 変数で柔軟なパスアクセス
- 破壊的変更は TOML パーサーレベルで検出
