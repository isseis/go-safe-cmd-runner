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
| `[global]` `workdir = "/tmp"` | 削除 | TOMLパーサーエラー（unknown field） |
| `temp_dir = true` | 削除 | TOMLパーサーエラー（unknown field） |
| `dir = "/path"` | `workdir = "/path"` | TOMLパーサーエラー（unknown field） |

**エラーメッセージ例**:
```
Error: toml: line X: unknown field 'workdir'
Error: toml: line Y: unknown field 'temp_dir'
Error: toml: line Z: unknown field 'dir'
```

**注意**: 詳細なエラーメッセージ（セクション情報など）は`go-toml/v2`の実装に依存します。

### 2.2 実行時の変数管理

実行時のワークディレクトリ情報は `AutoVarProvider` を通じて管理されます：

1. グループ実行開始時に `AutoVarProvider.SetWorkDir(workDir)` を呼び出す
2. `AutoVarProvider.Generate()` で `__runner_workdir` を含む変数マップを取得
3. この変数マップを既存の変数展開機構（`config.ExpandString`）で利用

**利点**:
- 新しい型（GroupContext）が不要
- 既存の変数展開機構をそのまま活用
- 一貫した変数管理（`__runner_datetime`, `__runner_pid` と同じパターン）

## 3. API 仕様

### 3.1 TempDirManager インターフェース

**パッケージ**: `internal/runner/executor`

**設計方針**:
- グループ単位でインスタンスを作成・破棄
- 一時ディレクトリを使用する場合のみインスタンスを作成
- インスタンス作成時にloggerとgroupNameを渡し、内部で保持
- 固定ディレクトリを使用する場合はインスタンスを作成しない

```go
// TempDirManager: グループ単位の一時ディレクトリ管理
//
// ライフサイクル:
//   1. NewTempDirManager(logger, groupName) でインスタンス作成
//   2. Create() で一時ディレクトリ生成
//   3. defer で Cleanup() を登録
//   4. グループ実行完了時に自動クリーンアップ
type TempDirManager interface {
    // Create: 一時ディレクトリを生成
    //
    // 戻り値:
    //   string: 生成された一時ディレクトリの絶対パス
    //   error: エラー（例: パーミッションエラー、ディスク容量不足）
    //
    // 動作:
    //   1. プレフィックス "scr-<groupName>-" でランダムなディレクトリを生成
    //   2. パーミッションは 0700 に設定
    //   3. INFO レベルでログ出力
    //   4. 生成されたパスを内部で保持
    //
    // 例:
    //   mgr := NewTempDirManager(logger, "backup")
    //   tempDir, err := mgr.Create()
    //   // → "/tmp/scr-backup-a1b2c3d4"
    Create() (string, error)

    // Cleanup: 一時ディレクトリを削除
    //
    // 戻り値:
    //   error: エラー（例: アクセス権限なし）
    //           返されたエラーは記録されるが、処理は継続される
    //
    // 動作:
    //   1. Create() で生成されたディレクトリを削除
    //   2. 削除成功: DEBUG ログ
    //   3. 削除失敗: ERROR ログ + 標準エラー出力
    //
    // 注意:
    //   - 一時ディレクトリを保持する場合は、呼び出し元が Cleanup() を呼ばないことで制御する
    //   - Runner が --keep-temp-dirs フラグに応じて呼び出しを制御する
    //
    // 例:
    //   if !keepTempDirs {
    //       err := mgr.Cleanup()
    //       // 削除成功の場合 err=nil、失敗の場合 err != nil
    //   }
    Cleanup() error

    // Path: 生成された一時ディレクトリのパスを取得
    //
    // 戻り値:
    //   string: 一時ディレクトリの絶対パス
    //           Create() が呼ばれていない場合は空文字列
    //
    // 例:
    //   path := mgr.Path()
    //   // → "/tmp/scr-backup-a1b2c3d4"
    Path() string
}

// NewTempDirManager: TempDirManager のコンストラクタ
//
// 引数:
//   logger: ロギングインターフェース
//   groupName: グループ名
//
// 戻り値:
//   TempDirManager: 一時ディレクトリマネージャーのインスタンス
//
// 例:
//   mgr := NewTempDirManager(logger, "backup")
func NewTempDirManager(logger logging.Logger, groupName string) TempDirManager
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
    //
    // 戻り値:
    //   error: エラー（グループまたはコマンド実行失敗）
    //
    // 動作:
    //   1. グループループを開始
    //   2. 各グループに対して ExecuteGroup() を呼び出し
    //   3. グループ実行失敗時の処理（中止/継続）は設定で決定
    ExecuteGroups(config *runnertypes.Config) error

    // ExecuteGroup: 1つのグループを実行
    //
    // 引数:
    //   group: グループ設定
    //
    // 戻り値:
    //   error: エラー（コマンド実行失敗など）
    //
    // ライフサイクル:
    //   1. ワークディレクトリを決定（resolveGroupWorkDir）
    //   2. 一時ディレクトリの場合は生成（TempDirManager.Create）
    //   3. AutoVarProvider.SetWorkDir() でワークディレクトリを設定
    //   4. defer で条件付きクリーンアップを登録（if !keepTempDirs { mgr.Cleanup() }）
    //   5. コマンド実行ループ
    //   6. グループ実行終了（成功・失敗問わず） → defer 実行
    ExecuteGroup(group *runnertypes.CommandGroup) error
}
```

### 3.3 AutoVarProvider の拡張

**パッケージ**: `internal/runner/variable`

**既存の `AutoVarProvider` インターフェースに `SetWorkDir` メソッドを追加**:

```go
// AutoVarProvider provides automatic internal variables
type AutoVarProvider interface {
    // Generate returns all auto internal variables as a map.
    // All keys have the AutoVarPrefix (__runner_).
    Generate() map[string]string

    // SetWorkDir sets the current group's working directory
    // This must be called before each group execution to update __runner_workdir
    SetWorkDir(workdir string)
}

// autoVarProvider implements AutoVarProvider
type autoVarProvider struct {
    clock   Clock
    workdir string  // 追加: グループごとに設定される作業ディレクトリ
}

// SetWorkDir sets the current group's working directory
func (p *autoVarProvider) SetWorkDir(workdir string) {
    p.workdir = workdir
}

// Generate returns all auto internal variables as a map.
// This includes:
//   - __runner_datetime: 現在時刻（UTC、YYYYMMDDHHmmSS.msec形式）
//   - __runner_pid: プロセスID
//   - __runner_workdir: 現在のグループの作業ディレクトリ（SetWorkDir で設定された場合のみ）
func (p *autoVarProvider) Generate() map[string]string {
    now := p.clock()
    vars := map[string]string{
        AutoVarPrefix + AutoVarKeyDatetime: now.UTC().Format(DatetimeLayout),
        AutoVarPrefix + AutoVarKeyPID:      strconv.Itoa(os.Getpid()),
    }

    // workdir が設定されている場合のみ追加
    if p.workdir != "" {
        vars[AutoVarPrefix + AutoVarKeyWorkDir] = p.workdir
    }

    return vars
}
```

**定数の追加**:

```go
const (
    // AutoVarKeyWorkDir is the key for the workdir auto internal variable (without prefix)
    AutoVarKeyWorkDir = "workdir"
)
```

### 3.4 既存の変数展開機構の活用

**既存の `config.ExpandString` を活用**:

既存の `internal/runner/config/expansion.go` の `ExpandString` 関数をそのまま利用します。
この関数は `%{VAR}` 形式の変数参照を展開する汎用機能を提供しています。

```go
// 既存関数（変更なし）
// ExpandString expands %{VAR} references in a string using the provided
// internal variables. It detects circular references and reports detailed errors.
func ExpandString(
    input string,
    expandedVars map[string]string,
    level string,
    field string,
) (string, error)
```

**統合方法**:

1. グループ実行開始時に `AutoVarProvider.SetWorkDir(workDir)` を呼び出す
2. `AutoVarProvider.Generate()` で `__runner_workdir` を含む変数マップを取得
3. この変数マップを `ExpandString` に渡してコマンド引数を展開

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
// 戻り値: (workdir, tempDirManager, error)
//   - 固定ディレクトリの場合: tempDirManager は nil
//   - 一時ディレクトリの場合: tempDirManager は非nil（クリーンアップに使用）
func (e *DefaultGroupExecutor) resolveGroupWorkDir(
    group *runnertypes.CommandGroup,
) (string, TempDirManager, error) {
    // グループレベル WorkDir が指定されている?
    if group.WorkDir != "" {
        // 固定ディレクトリを使用
        e.logger.Info(fmt.Sprintf(
            "Using group workdir for '%s': %s",
            group.Name, group.WorkDir,
        ))
        return group.WorkDir, nil, nil
    }

    // 一時ディレクトリマネージャーを作成
    tempDirMgr := NewTempDirManager(e.logger, group.Name)

    // 一時ディレクトリを生成
    tempDir, err := tempDirMgr.Create()
    if err != nil {
        return "", nil, err
    }

    return tempDir, tempDirMgr, nil
}

// resolveCommandWorkDir: コマンドのワークディレクトリを決定
// 優先度: Command.ExpandedWorkDir > Group の実際のワークディレクトリ
func (e *DefaultCommandExecutor) resolveCommandWorkDir(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
) string {
    // 優先度1: コマンドレベル ExpandedWorkDir
    if cmd.ExpandedWorkDir != "" {
        return cmd.ExpandedWorkDir
    }

    // 優先度2: グループの実際のワークディレクトリ
    // 注: ExecuteGroup で resolveGroupWorkDir により決定済み
    //     （一時ディレクトリまたは固定ディレクトリの物理パス）
    return group.WorkDir
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

    "go-safe-cmd-runner/internal/logging"
)

// DefaultTempDirManager: 一時ディレクトリの標準実装
type DefaultTempDirManager struct {
    logger      logging.Logger
    groupName   string  // グループ名（インスタンス作成時に設定）
    tempDirPath string  // Create() で生成されたパス（Path() と Cleanup() で使用）
}

// NewTempDirManager: 新規インスタンスを作成
func NewTempDirManager(logger logging.Logger, groupName string) TempDirManager {
    return &DefaultTempDirManager{
        logger:    logger,
        groupName: groupName,
    }
}

// Create: 一時ディレクトリを生成
func (m *DefaultTempDirManager) Create() (string, error) {
    // プレフィックス: "scr-<groupName>-"
    prefix := fmt.Sprintf("scr-%s-", m.groupName)

    // OS の TempDir() 関数を使用
    baseTmpDir := os.TempDir()

    // MkdirTemp でランダムディレクトリを生成（パーミッション 0700）
    tempDir, err := os.MkdirTemp(baseTmpDir, prefix)
    if err != nil {
        return "", fmt.Errorf("failed to create temporary directory: %w", err)
    }

    // 生成されたパスを保存
    m.tempDirPath = tempDir

    // ログ出力 (INFO レベル)
    m.logger.Info(fmt.Sprintf(
        "Created temporary directory for group '%s': %s",
        m.groupName, tempDir,
    ))

    return tempDir, nil
}

// Cleanup: 一時ディレクトリを削除
func (m *DefaultTempDirManager) Cleanup() error {
    if m.tempDirPath == "" {
        // Create() が呼ばれていない場合は何もしない
        return nil
    }

    // ディレクトリを削除
    err := os.RemoveAll(m.tempDirPath)
    if err != nil {
        // エラーログ出力 (ERROR レベル)
        m.logger.Error(fmt.Sprintf(
            "Failed to cleanup temporary directory: %s: %v",
            m.tempDirPath, err,
        ))

        // 標準エラー出力
        fmt.Fprintf(os.Stderr,
            "Warning: Failed to cleanup temporary directory: %s\n",
            m.tempDirPath,
        )

        return fmt.Errorf("failed to cleanup temporary directory: %w", err)
    }

    // ログ出力 (DEBUG レベル)
    m.logger.Debug(fmt.Sprintf(
        "Cleaned up temporary directory: %s",
        m.tempDirPath,
    ))

    return nil
}

// Path: 生成された一時ディレクトリのパスを取得
func (m *DefaultTempDirManager) Path() string {
    return m.tempDirPath
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

## 6. 変数展開の統合

### 6.1 グループ実行時の AutoVarProvider 更新

**ファイル**: `internal/runner/executor/group_executor.go`

```go
// ExecuteGroup: 1つのグループを実行
func (e *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    group *types.CommandGroup,
) error {
    // ステップ1: ワークディレクトリを決定
    workDir, tempDirMgr, err := e.resolveGroupWorkDir(group)
    if err != nil {
        return fmt.Errorf("failed to resolve work directory: %w", err)
    }

    // ステップ2: defer でクリーンアップを登録（重要: エラー時も実行）
    if tempDirMgr != nil && !e.keepTempDirs {
        defer func() {
            err := tempDirMgr.Cleanup()
            if err != nil {
                e.logger.Error(fmt.Sprintf("Cleanup warning: %v", err))
            }
        }()
    }

    // ステップ3: AutoVarProvider に workdir をセット
    e.autoVarProvider.SetWorkDir(workDir)

    // ステップ4: グループレベル変数を更新（__runner_workdir を含める）
    // 注: 既存の ExpandGroupConfig は Global 後に実行されるため、
    //     ここでは __runner_workdir のみを追加する
    autoVars := e.autoVarProvider.Generate()
    // group.ExpandedVars に __runner_workdir を追加
    maps.Copy(group.ExpandedVars, autoVars)

    // ステップ5: コマンド実行ループ
    for _, cmd := range group.Commands {
        // ステップ5-1: コマンドレベルの変数マップを更新
        // group.ExpandedVars（__runner_workdir を含む）を cmd.ExpandedVars にマージ
        // これにより、コマンド固有の変数とグループレベルの変数が統合される
        maps.Copy(cmd.ExpandedVars, group.ExpandedVars)

        // ステップ5-2: コマンドレベルの変数展開を再実行（__runner_workdir を含める）
        err := e.expandCommandWithWorkDir(cmd, group)
        if err != nil {
            return fmt.Errorf("failed to expand command '%s': %w", cmd.Name, err)
        }

        // ステップ5-3: コマンド実行
        output, err := e.cmdExecutor.Execute(ctx, cmd)
        if err != nil {
            return fmt.Errorf("command '%s' failed: %w", cmd.Name, err)
        }

        // ステップ5-4: 出力ハンドリング（既存ロジック）
        e.handleCommandOutput(cmd, output)
    }

    return nil
}

// expandCommandWithWorkDir: コマンド変数を再展開（__runner_workdir を含める）
func (e *DefaultGroupExecutor) expandCommandWithWorkDir(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
) error {
    // コマンドレベルの変数を group.ExpandedVars をベースに再展開
    // この時点で group.ExpandedVars には __runner_workdir が含まれている
    level := fmt.Sprintf("command[%s]", cmd.Name)

    // ExpandedCmd の再展開
    expandedCmd, err := config.ExpandString(cmd.Cmd, cmd.ExpandedVars, level, "cmd")
    if err != nil {
        return err
    }
    cmd.ExpandedCmd = expandedCmd

    // ExpandedArgs の再展開
    for i, arg := range cmd.Args {
        expanded, err := config.ExpandString(arg, cmd.ExpandedVars, level, fmt.Sprintf("args[%d]", i))
        if err != nil {
            return err
        }
        cmd.ExpandedArgs[i] = expanded
    }

    // WorkDir の展開（新規追加）
    if cmd.WorkDir != "" {
        expandedWorkDir, err := config.ExpandString(cmd.WorkDir, cmd.ExpandedVars, level, "workdir")
        if err != nil {
            return err
        }
        cmd.ExpandedWorkDir = expandedWorkDir
    }

    return nil
}
```

### 6.2 Command 型への ExpandedWorkDir フィールド追加

**ファイル**: `internal/runner/runnertypes/config_types.go`

```go
type Command struct {
    Name    string   `toml:"name"`
    Cmd     string   `toml:"cmd"`
    Args    []string `toml:"args"`
    WorkDir string   `toml:"workdir"`  // 変更: dir → workdir

    // Expanded fields
    ExpandedCmd     string
    ExpandedArgs    []string
    ExpandedWorkDir string  // 追加: 展開済み WorkDir
    ExpandedVars    map[string]string
    ExpandedEnv     map[string]string
}
```

### 6.3 ワークディレクトリ決定ロジック

**ファイル**: `internal/runner/executor/command_executor.go`

```go
// resolveWorkDir: 実際に使用するワークディレクトリを決定
// 優先度: Command.ExpandedWorkDir > Group.WorkDir
func (e *DefaultCommandExecutor) resolveWorkDir(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
) string {
    // 優先度1: コマンドレベル WorkDir
    if cmd.ExpandedWorkDir != "" {
        return cmd.ExpandedWorkDir
    }

    // 優先度2: グループレベル WorkDir（ExecuteGroup で決定済み）
    // 注: この時点で group.WorkDir は物理的なディレクトリパス
    //     （一時ディレクトリまたは固定ディレクトリ）
    return group.WorkDir
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

    // Runner を作成（keepTempDirs フラグを渡す）
    r, err := runner.NewRunner(config, runner.WithKeepTempDirs(*keepTempDirs))
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: Failed to create runner: %v\n", err)
        os.Exit(1)
    }

    // グループ実行
    err = r.ExecuteGroups()
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
    logger          logging.Logger
    autoVarProvider variable.AutoVarProvider  // 変更: VariableExpander → AutoVarProvider
    cmdExecutor     CommandExecutor
    keepTempDirs    bool  // Runner から受け取った --keep-temp-dirs フラグ
}

// NewDefaultGroupExecutor: 新規インスタンスを作成
func NewDefaultGroupExecutor(
    logger logging.Logger,
    autoVarProvider variable.AutoVarProvider,  // 変更: VariableExpander → AutoVarProvider
    cmdExecutor CommandExecutor,
    keepTempDirs bool,
) *DefaultGroupExecutor {
    return &DefaultGroupExecutor{
        logger:          logger,
        autoVarProvider: autoVarProvider,
        cmdExecutor:     cmdExecutor,
        keepTempDirs:    keepTempDirs,
    }
}

// ExecuteGroup: 1つのグループを実行
func (e *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    group *types.CommandGroup,
) error {
    // ステップ1: ワークディレクトリを決定
    workDir, tempDirMgr, err := e.resolveGroupWorkDir(group)
    if err != nil {
        return fmt.Errorf("failed to resolve work directory: %w", err)
    }

    // ステップ2: defer でクリーンアップを登録（重要: エラー時も実行）
    if tempDirMgr != nil && !e.keepTempDirs {
        defer func() {
            err := tempDirMgr.Cleanup()
            if err != nil {
                // エラーをログするが、処理は継続
                e.logger.Error(fmt.Sprintf("Cleanup warning: %v", err))
            }
        }()
    }

    // ステップ3: AutoVarProvider に workdir をセット
    e.autoVarProvider.SetWorkDir(workDir)

    // ステップ4: グループレベル変数を更新（__runner_workdir を含める）
    autoVars := e.autoVarProvider.Generate()
    maps.Copy(group.ExpandedVars, autoVars)

    // ステップ5: コマンド実行ループ
    for _, cmd := range group.Commands {
        // ステップ5-1: コマンドレベルの変数マップを更新
        // group.ExpandedVars（__runner_workdir を含む）を cmd.ExpandedVars にマージ
        // これにより、コマンド固有の変数とグループレベルの変数が統合される
        maps.Copy(cmd.ExpandedVars, group.ExpandedVars)

        // ステップ5-2: コマンドレベルの変数展開を再実行（__runner_workdir を含める）
        err := e.expandCommandWithWorkDir(cmd, group)
        if err != nil {
            return fmt.Errorf("failed to expand command '%s': %w", cmd.Name, err)
        }

        // ステップ5-3: コマンド実行
        output, err := e.cmdExecutor.Execute(ctx, cmd)
        if err != nil {
            return fmt.Errorf("command '%s' failed: %w", cmd.Name, err)
        }

        // ステップ5-4: 出力ハンドリング（既存ロジック）
        e.handleCommandOutput(cmd, output)
    }

    return nil
}

// resolveGroupWorkDir: グループのワークディレクトリを決定
// 戻り値: (workdir, tempDirManager, error)
//   - 固定ディレクトリの場合: tempDirManager は nil
//   - 一時ディレクトリの場合: tempDirManager は非nil（クリーンアップに使用）
func (e *DefaultGroupExecutor) resolveGroupWorkDir(
    group *types.CommandGroup,
) (string, TempDirManager, error) {
    // グループレベル WorkDir が指定されている?
    if group.WorkDir != "" {
        // 固定ディレクトリを使用
        e.logger.Info(fmt.Sprintf(
            "Using group workdir for '%s': %s",
            group.Name, group.WorkDir,
        ))
        return group.WorkDir, nil, nil
    }

    // 一時ディレクトリマネージャーを作成
    tempDirMgr := NewTempDirManager(e.logger, group.Name)

    // 一時ディレクトリを生成
    tempDir, err := tempDirMgr.Create()
    if err != nil {
        return "", nil, err
    }

    return tempDir, tempDirMgr, nil
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
# workdir はグループでは指定せず自動作成される一時ディレクトリを使用
# 必要に応じてコマンドレベルで指定

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
    // テスト1: 正常削除
    // - Cleanup() を呼び出し
    // - 期待: ディレクトリが削除される
    // - 確認: ディレクトリ削除、DEBUG ログ

    // テスト2: Cleanup() を呼ばない（keepTempDirs = true の動作）
    // - Cleanup() を呼び出さない
    // - 期待: ディレクトリが保持される
    // - 確認: ディレクトリ存在

    // テスト3: 削除失敗（パーミッション拒否）
    // - 期待: エラーを返す
    // - 確認: ERROR ログ、標準エラー出力
}
```

#### T003: コマンド引数の展開（__runner_workdir を含む）
```go
func TestExpandCommandArgsWithRunnerWorkdir(t *testing.T) {
    // テスト条件:
    // - コマンド: cp
    // - 引数: ["%{__runner_workdir}/file", "/dest"]
    // - cmd.ExpandedVars["__runner_workdir"]: /tmp/scr-test-XXXXXX
    // - 期待: ExpandedArgs[0] = /tmp/scr-test-XXXXXX/file
    // - 期待: ExpandedArgs[1] = /dest
}
```

#### T004: config.ExpandString での __runner_workdir 展開
```go
func TestExpandStringWithRunnerWorkdir(t *testing.T) {
    // テストケース1: __runner_workdir をルートとして使用
    // - 文字列: %{__runner_workdir}/report.json
    // - expandedVars["__runner_workdir"]: /tmp/scr-test-XXXXXX
    // - 期待: /tmp/scr-test-XXXXXX/report.json

    // テストケース2: 相対パスとの組み合わせ
    // - 文字列: %{__runner_workdir}/data/output.log
    // - expandedVars["__runner_workdir"]: /tmp/scr-test-XXXXXX
    // - 期待: /tmp/scr-test-XXXXXX/data/output.log

    // テストケース3: __runner_workdir が含まれない場合
    // - 文字列: /var/log/app.log
    // - 期待: /var/log/app.log（変更なし）
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

TOMLパーサー(`go-toml/v2`)が返す標準エラーメッセージを使用します。

**エラー例**:

```
Error: toml: line X: unknown field 'workdir'
```

**注意**: カスタムエラーメッセージは実装しません（YAGNIの原則に従い、シンプルな実装を維持）。

## 15. 実装チェックリスト

### Phase 1: 型定義
- [ ] `Global.WorkDir` を削除
- [ ] `Group.TempDir` を削除
- [ ] `Command.Dir` → `Command.WorkDir` に変更
- [ ] `Command.ExpandedWorkDir` フィールドを追加

### Phase 2: 一時ディレクトリ機能
- [ ] `TempDirManager` インターフェース定義
- [ ] `DefaultTempDirManager` 実装
- [ ] `--keep-temp-dirs` フラグを Runner に追加
- [ ] Runner から GroupExecutor へ keepTempDirs を渡す
- [ ] `defer` で条件付きクリーンアップ登録（`if !keepTempDirs { mgr.Cleanup() }`）

### Phase 3: 変数展開
- [ ] `AutoVarProvider` に `SetWorkDir` メソッドを追加
- [ ] `AutoVarProvider.Generate()` で `__runner_workdir` を返すように実装
- [ ] `AutoVarKeyWorkDir` 定数を追加
- [ ] `GroupExecutor.ExecuteGroup()` でグループ実行時に `SetWorkDir` を呼び出す
- [ ] `GroupExecutor.expandCommandWithWorkDir()` を実装（コマンド変数の再展開）
- [ ] `CommandExecutor.resolveWorkDir()` を実装（ワークディレクトリ決定ロジック）

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
