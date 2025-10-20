# 詳細仕様書: 作業ディレクトリ仕様の再設計

## 1. 概要

本ドキュメントは、タスク0034「作業ディレクトリ仕様の再設計」の詳細仕様を記述します。

## 2. 型定義

### 2.1 設定型の変更

#### 2.1.1 削除対象フィールド

**変更対象**: `internal/runner/runnertypes/config.go`

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
    WorkDir string `toml:"workdir"`  // TOMLから読み込んだ生の値
    ExpandedWorkDir string            // 展開済みの物理/仮想パス（実行時に設定）
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

#### 2.2.1 変数展開の遅延評価

**設計方針**:
- **コマンド実行時まで変数展開を遅延させる**
- ローディング時には変数を展開せず、元の定義を保持
- 実行時にグループ変数とコマンド変数を統合して一度だけ展開

**利点**:
1. **優先順位が自然に実現**: グループ変数をベースに、コマンド変数で上書き
2. **二重展開を回避**: 展開は実行時の1回のみ
3. **シンプルな実装**: マージロジックが不要

#### 2.2.2 ワークディレクトリの設定

実行時のワークディレクトリ情報は2箇所に設定されます：

1. グループ実行開始時に `resolveGroupWorkDir()` でワークディレクトリを決定・展開
2. 決定した物理/仮想パスを以下に設定:
   - `group.ExpandedWorkDir`: 後続処理（`resolveCommandWorkDir` など）で参照
   - `group.ExpandedVars["__runner_workdir"]`: コマンドレベルの変数展開で参照
3. コマンド実行時に、グループ変数とコマンド変数を統合して展開

**設計の利点**:
- `group.WorkDir`: TOMLの生の値を保持（トレーサビリティ）
- `group.ExpandedWorkDir`: 展開済みの値を保持（処理ロジックで使用）
- シンプルで直接的な実装（余計な間接化がない）
- `AutoVarProvider` への依存が不要
- ワークディレクトリの決定と設定が同じスコープ内で完結

## 3. API 仕様

### 3.1 TempDirManager インターフェース

**パッケージ**: `internal/runner/executor`

**設計方針**:
- グループ単位でインスタンスを作成・破棄
- 一時ディレクトリを使用する場合のみインスタンスを作成
- インスタンス作成時にlogger、groupName、isDryRunを渡し、内部で保持
- 固定ディレクトリを使用する場合はインスタンスを作成しない
- **dry-runモードでは実際のディレクトリ操作を行わず、ログ出力のみ**

**アーキテクチャ上の位置づけ**:
- `TempDirManager` は `Runner.ExecuteGroup()` メソッド内でローカルに使用される
- `Runner.resourceManager` (`NormalResourceManager` / `DryRunResourceManager`) とは独立
- `isDryRun` フラグは `Runner` から受け取り、`TempDirManager` に渡す

```go
// TempDirManager: グループ単位の一時ディレクトリ管理
//
// ライフサイクル:
//   1. NewTempDirManager(logger, groupName, isDryRun) でインスタンス作成
//   2. Create() で一時ディレクトリ生成（dry-runでは仮想パスを返す）
//   3. defer で Cleanup() を登録
//   4. グループ実行完了時に自動クリーンアップ（dry-runではログのみ）
type TempDirManager interface {
    // Create: 一時ディレクトリを生成
    //
    // 戻り値:
    //   string: 生成された一時ディレクトリの絶対パス
    //          (dry-runモード: 仮想パス "/tmp/scr-<groupName>-dryrun-<timestamp>")
    //   error: エラー（例: パーミッションエラー、ディスク容量不足）
    //
    // 動作（通常モード）:
    //   1. プレフィックス "scr-<groupName>-" でランダムなディレクトリを生成
    //   2. パーミッションは 0700 に設定
    //   3. INFO レベルでログ出力
    //   4. 生成されたパスを内部で保持
    //
    // 動作（dry-runモード）:
    //   1. 仮想パスを生成（実際のディレクトリは作成しない）
    //   2. INFO レベルでログ出力（"[DRY-RUN] Would create temp dir: <path>"）
    //   3. 仮想パスを内部で保持
    //
    // 例:
    //   // 通常モード
    //   mgr := NewTempDirManager(logger, "backup", false)
    //   tempDir, err := mgr.Create()
    //   // → "/tmp/scr-backup-a1b2c3d4"
    //
    //   // dry-runモード
    //   mgr := NewTempDirManager(logger, "backup", true)
    //   tempDir, err := mgr.Create()
    //   // → "/tmp/scr-backup-dryrun-20251018143025"
    Create() (string, error)

    // Cleanup: 一時ディレクトリを削除
    //
    // 戻り値:
    //   error: エラー（例: アクセス権限なし）
    //          返されたエラーは記録されるが、処理は継続される
    //          dry-runモードでは常に nil
    //
    // 動作（通常モード）:
    //   1. Create() で生成されたディレクトリを削除
    //   2. 削除成功: DEBUG ログ
    //   3. 削除失敗: ERROR ログ + 標準エラー出力
    //
    // 動作（dry-runモード）:
    //   1. 実際の削除は行わない
    //   2. DEBUG ログ（"[DRY-RUN] Would delete temp dir: <path>"）
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
//   isDryRun: dry-runモードフラグ（trueの場合、実際のファイルシステム操作を行わない）
//
// 戻り値:
//   TempDirManager: 一時ディレクトリマネージャーのインスタンス
//
// 例:
//   // 通常モード
//   mgr := NewTempDirManager(logger, "backup", false)
//
//   // dry-runモード
//   mgr := NewTempDirManager(logger, "backup", true)
func NewTempDirManager(logger logging.Logger, groupName string, isDryRun bool) TempDirManager
```

### 3.2 Runner.ExecuteGroup() メソッド

**パッケージ**: `internal/runner`

```go
// Runner.ExecuteGroup: 1つのグループを実行
//
// 引数:
//   ctx: コンテキスト
//   group: グループ設定
//
// 戻り値:
//   error: エラー（グループまたはコマンド実行失敗）
//
// 動作:
//   1. ワークディレクトリを決定（resolveGroupWorkDir）
//   2. 一時ディレクトリの場合は生成（TempDirManager.Create）
//   3. group.ExpandedVars["__runner_workdir"] にワークディレクトリを直接設定
//   4. defer で条件付きクリーンアップを登録（if !keepTempDirs { mgr.Cleanup() }）
//   5. コマンド実行ループ
func (r *Runner) ExecuteGroup(ctx context.Context, group *runnertypes.CommandGroup) error
```

**dry-runモードでの動作**:
- `r.dryRun` フラグに基づいて `TempDirManager` を作成（`isDryRun` 引数を渡す）
- 仮想パスが生成され、`group.ExpandedVars["__runner_workdir"]` に設定される
- コマンド実行は `DryRunResourceManager` 経由で分析のみ行われる

### 3.3 __runner_workdir の定数定義

**パッケージ**: `internal/runner/variable`

**定数の追加**:

```go
const (
    // AutoVarKeyWorkDir is the key for the workdir auto internal variable (without prefix)
    AutoVarKeyWorkDir = "workdir"
)
```

**変数展開の動作**:

| レベル | 変数展開 | `__runner_workdir` 参照 | その他の変数参照 |
|--------|---------|----------------------|----------------|
| グループ (`group.workdir`) | ✅ 可能 | ❌ 不可（未定義エラー） | ✅ 可能 |
| コマンド (`cmd.*`) | ✅ 可能 | ✅ 可能 | ✅ 可能 |

**理由**:
- `__runner_workdir` は `ExecuteGroup()` で `group.workdir` を決定した**後**に設定される
- グループレベルで参照すると循環参照になるため、未定義変数エラーとなる
- その他の変数（`%{backup_base}` など）はグループレベルでも参照可能

**使用例**:
```toml
[[groups]]
name = "backup"
workdir = "%{backup_base}/data"        # ✅ OK: 他の変数参照
# workdir = "%{__runner_workdir}/sub"  # ❌ NG: 未定義変数エラー

[[groups.commands]]
name = "dump"
args = ["%{__runner_workdir}/dump.sql"]  # ✅ OK: コマンドレベル
```

**注意**: `AutoVarProvider` インターフェースへの変更は不要です。`__runner_workdir` は `GroupExecutor` が直接 `group.ExpandedVars` に設定します。

### 3.4 変数展開の実行タイミング

**変更前（旧設計）**:
- ローディング時: `LoadConfig()` で `cmd.ExpandedVars` を計算
- 実行時: `ExecuteGroup()` で `cmd.ExpandedVars` を再展開

**変更後（新設計）**:
- ローディング時: 変数を展開せず、`cmd.Vars` に raw 値を保持
- 実行時: `buildVarsForCommand()` でグループ変数とコマンド変数を統合し、`expandCommand()` で一度だけ展開

**利点**:
1. 二重展開を回避
2. 優先順位が自然に実現（コマンド変数 > グループ変数）
3. `cmd.ExpandedVars` の状態管理が不要

### 3.5 既存の変数展開機構の活用

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

1. グループ実行開始時に `resolveGroupWorkDir()` でワークディレクトリを決定
2. 決定した `workDir` を `group.ExpandedVars["__runner_workdir"]` に直接設定
3. コマンド実行時に `buildVarsForCommand()` でグループ変数とコマンド変数を統合
4. 統合された変数マップを `ExpandString` に渡してコマンド引数を展開

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
        // 変数展開を実行（注意: __runner_workdir はまだ未定義）
        level := fmt.Sprintf("group[%s]", group.Name)
        expandedWorkDir, err := config.ExpandString(
            group.WorkDir,
            group.ExpandedVars,  // __runner_workdir は含まれない
            level,
            "workdir",
        )
        if err != nil {
            return "", nil, fmt.Errorf("failed to expand group workdir: %w", err)
        }

        e.logger.Info(fmt.Sprintf(
            "Using group workdir for '%s': %s",
            group.Name, expandedWorkDir,
        ))
        return expandedWorkDir, nil, nil
    }

    // 一時ディレクトリマネージャーを作成
    // 注: isDryRun フラグは GroupExecutor のフィールドとして保持
    tempDirMgr := NewTempDirManager(e.logger, group.Name, e.isDryRun)

    // 一時ディレクトリを生成
    // dry-runモードでは仮想パスが返される
    tempDir, err := tempDirMgr.Create()
    if err != nil {
        return "", nil, err
    }

    return tempDir, tempDirMgr, nil
}

// resolveCommandWorkDir: コマンドのワークディレクトリを決定
// 優先度: Command.ExpandedWorkDir > Group.ExpandedWorkDir
func (e *DefaultCommandExecutor) resolveCommandWorkDir(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
) string {
    // 優先度1: コマンドレベル ExpandedWorkDir
    if cmd.ExpandedWorkDir != "" {
        return cmd.ExpandedWorkDir
    }

    // 優先度2: グループレベル ExpandedWorkDir
    // 注: ExecuteGroup で resolveGroupWorkDir により決定・展開済み
    //     （一時ディレクトリまたは固定ディレクトリの物理/仮想パス）
    return group.ExpandedWorkDir
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
    "time"
    "path/filepath"

    "go-safe-cmd-runner/internal/logging"
)

// DefaultTempDirManager: 一時ディレクトリの標準実装
type DefaultTempDirManager struct {
    logger      logging.Logger
    groupName   string  // グループ名（インスタンス作成時に設定）
    isDryRun    bool    // dry-runモードフラグ
    tempDirPath string  // Create() で生成されたパス（Path() と Cleanup() で使用）
}

// NewTempDirManager: 新規インスタンスを作成
func NewTempDirManager(logger logging.Logger, groupName string, isDryRun bool) TempDirManager {
    return &DefaultTempDirManager{
        logger:    logger,
        groupName: groupName,
        isDryRun:  isDryRun,
    }
}

// Create: 一時ディレクトリを生成
func (m *DefaultTempDirManager) Create() (string, error) {
    // dry-runモード: 仮想パスを生成
    if m.isDryRun {
        // タイムスタンプベースの仮想パス
        timestamp := time.Now().Format("20060102150405")
        tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("scr-%s-dryrun-%s", m.groupName, timestamp))
        m.tempDirPath = tempDir

        // ログ出力（実際には作成しない）
        m.logger.Info(fmt.Sprintf(
            "[DRY-RUN] Would create temporary directory for group '%s': %s",
            m.groupName, tempDir,
        ))

        return tempDir, nil
    }

    // 通常モード: 実際にディレクトリを生成
    // プレフィックス: "scr-<groupName>-"
    prefix := fmt.Sprintf("scr-%s-", m.groupName)

    // OS の TempDir() 関数を使用
    baseTmpDir := os.TempDir()

    // MkdirTemp でランダムディレクトリを生成
    // os.MkdirTemp は内部的に 0700 を使用するが、プロセスの umask の影響を受ける
    // 実際のパーミッションは 0700 より厳しくなる可能性がある
    tempDir, err := os.MkdirTemp(baseTmpDir, prefix)
    if err != nil {
        return "", fmt.Errorf("failed to create temporary directory: %w", err)
    }

    // セキュリティ要件: 厳密に 0700 を保証（umask の影響を排除）
    if err := os.Chmod(tempDir, 0700); err != nil {
        // クリーンアップを試みる
        _ = os.RemoveAll(tempDir)
        return "", fmt.Errorf("failed to set directory permissions: %w", err)
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

    // dry-runモード: ログ出力のみ
    if m.isDryRun {
        m.logger.Debug(fmt.Sprintf(
            "[DRY-RUN] Would delete temporary directory: %s",
            m.tempDirPath,
        ))
        return nil
    }

    // 通常モード: 実際にディレクトリを削除
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

### 6.1 グループ実行時の __runner_workdir 設定

**ファイル**: `internal/runner/runner.go`

**注意**: 実際の実装は `Runner.ExecuteGroup()` メソッドです。

```go
// ExecuteGroup: 1つのグループを実行
func (r *Runner) ExecuteGroup(
    ctx context.Context,
    group *runnertypes.CommandGroup,
) error {
    // ステップ1: ワークディレクトリを決定
    // r.dryRun フラグに基づいて TempDirManager を作成
    workDir, tempDirMgr, err := r.resolveGroupWorkDir(group)
    if err != nil {
        return fmt.Errorf("failed to resolve work directory: %w", err)
    }

    // ステップ2: defer でクリーンアップを登録（重要: エラー時も実行）
    // dry-runモードでもCleanup()は呼ばれるが、実際の削除は行わない
    if tempDirMgr != nil && !r.keepTempDirs {
        defer func() {
            err := tempDirMgr.Cleanup()
            if err != nil {
                r.logger.Error(fmt.Sprintf("Cleanup warning: %v", err))
            }
        }()
    }

    // ステップ3: グループの展開済みワークディレクトリを設定
    // (1) group.ExpandedWorkDir に物理/仮想パスを設定
    //     dry-runモードでは仮想パスが設定される
    group.ExpandedWorkDir = workDir

    // (2) グループレベル変数に __runner_workdir を設定
    //     コマンドレベルの変数展開で参照可能
    group.ExpandedVars[variable.AutoVarPrefix + variable.AutoVarKeyWorkDir] = workDir

    // ステップ4: コマンド実行ループ
    for _, cmd := range group.Commands {
        // ステップ4-1: 変数マップを構築
        // グループ変数をベースに、コマンド固有の変数で上書き（コマンドが優先）
        vars := r.buildVarsForCommand(cmd, group)

        // ステップ4-2: コマンドの変数展開（一度だけ）
        expandedCmd, err := r.expandCommand(cmd, vars)
        if err != nil {
            return fmt.Errorf("failed to expand command '%s': %w", cmd.Name, err)
        }

        // ステップ4-3: コマンド実行
        // dry-runモードでは resourceManager が DryRunResourceManager なので、
        // 実際のコマンド実行は行わず、分析のみ実施
        output, err := r.resourceManager.ExecuteCommand(ctx, expandedCmd)
        if err != nil {
            return fmt.Errorf("command '%s' failed: %w", cmd.Name, err)
        }

        // ステップ4-4: 出力ハンドリング（既存ロジック）
        r.handleCommandOutput(cmd, output)
    }

    return nil
}

// buildVarsForCommand: コマンド実行用の変数マップを構築
// グループ変数とコマンド変数を統合（コマンド変数が優先）
func (e *DefaultGroupExecutor) buildVarsForCommand(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
) map[string]string {
    // 新しいマップを作成
    vars := make(map[string]string, len(group.ExpandedVars) + len(cmd.Vars))

    // 1. グループ変数をコピー（__runner_workdir を含む）
    maps.Copy(vars, group.ExpandedVars)

    // 2. コマンド固有の変数で上書き（コマンドが優先）
    // 注意: cmd.Vars は未展開の変数定義（TOML から読み込んだ raw 値）
    maps.Copy(vars, cmd.Vars)

    return vars
}

// expandCommand: コマンドの変数展開を実行
// 戻り値: 展開済みコマンド構造体（元の cmd は変更しない）
func (e *DefaultGroupExecutor) expandCommand(
    cmd *runnertypes.Command,
    vars map[string]string,
) (*runnertypes.Command, error) {
    level := fmt.Sprintf("command[%s]", cmd.Name)

    // 展開済みコマンド構造体を作成
    expanded := &runnertypes.Command{
        Name: cmd.Name,
    }

    // Cmd の展開
    expandedCmd, err := config.ExpandString(cmd.Cmd, vars, level, "cmd")
    if err != nil {
        return nil, err
    }
    expanded.ExpandedCmd = expandedCmd

    // Args の展開
    expanded.ExpandedArgs = make([]string, len(cmd.Args))
    for i, arg := range cmd.Args {
        expandedArg, err := config.ExpandString(arg, vars, level, fmt.Sprintf("args[%d]", i))
        if err != nil {
            return nil, err
        }
        expanded.ExpandedArgs[i] = expandedArg
    }

    // WorkDir の展開
    if cmd.WorkDir != "" {
        expandedWorkDir, err := config.ExpandString(cmd.WorkDir, vars, level, "workdir")
        if err != nil {
            return nil, err
        }
        expanded.ExpandedWorkDir = expandedWorkDir
    }

    // Env の展開（既存ロジックを保持）
    expanded.ExpandedEnv = make(map[string]string, len(cmd.Env))
    for key, value := range cmd.Env {
        expandedValue, err := config.ExpandString(value, vars, level, fmt.Sprintf("env[%s]", key))
        if err != nil {
            return nil, err
        }
        expanded.ExpandedEnv[key] = expandedValue
    }

    return expanded, nil
}
```

### 6.2 Command 型の変更

**ファイル**: `internal/runner/runnertypes/config.go`

```go
type Command struct {
    Name    string   `toml:"name"`
    Cmd     string   `toml:"cmd"`
    Args    []string `toml:"args"`
    WorkDir string   `toml:"workdir"`  // 変更: dir → workdir
    Env     map[string]string `toml:"env"`
    Vars    map[string]string `toml:"vars"`  // 重要: 未展開の変数定義

    // Expanded fields (実行時に設定)
    ExpandedCmd     string
    ExpandedArgs    []string
    ExpandedWorkDir string  // 追加: 展開済み WorkDir
    ExpandedEnv     map[string]string
    // 注意: ExpandedVars は削除（実行時に動的に構築するため不要）
}
```

**重要な変更点**:
1. `Vars` フィールド: TOML から読み込んだ未展開の変数定義を保持
2. `ExpandedVars` フィールド: 削除（実行時に `buildVarsForCommand()` で動的に構築）

### 6.3 ワークディレクトリ決定ロジック

**ファイル**: `internal/runner/executor/command_executor.go`

```go
// resolveWorkDir: 実際に使用するワークディレクトリを決定
// 優先度: Command.ExpandedWorkDir > Group.ExpandedWorkDir
func (e *DefaultCommandExecutor) resolveWorkDir(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
) string {
    // 優先度1: コマンドレベル ExpandedWorkDir
    if cmd.ExpandedWorkDir != "" {
        return cmd.ExpandedWorkDir
    }

    // 優先度2: グループレベル ExpandedWorkDir（ExecuteGroup で決定・展開済み）
    // 注: この時点で group.ExpandedWorkDir は物理/仮想ディレクトリパス
    //     （一時ディレクトリまたは固定ディレクトリ）
    return group.ExpandedWorkDir
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

**注**: Phase 0リファクタリングにより、GroupExecutorは既に`internal/runner/group_executor.go`として実装済みです。このセクションでは、workdir redesign機能を追加する際の変更点を記述します。

### 8.1 グループ実行のライフサイクル

**ファイル**: `internal/runner/group_executor.go` (既存ファイルを拡張)

```go
package runner

import (
    "context"
    "fmt"

    "go-safe-cmd-runner/internal/logging"
    "go-safe-cmd-runner/internal/runner/runnertypes"
)

// DefaultGroupExecutor: グループ実行の標準実装
type DefaultGroupExecutor struct {
    logger       logging.Logger
    cmdExecutor  CommandExecutor
    keepTempDirs bool  // Runner から受け取った --keep-temp-dirs フラグ
    isDryRun     bool  // dry-run モードフラグ
}

// NewDefaultGroupExecutor: 新規インスタンスを作成
func NewDefaultGroupExecutor(
    logger logging.Logger,
    cmdExecutor CommandExecutor,
    keepTempDirs bool,
    isDryRun bool,
) *DefaultGroupExecutor {
    return &DefaultGroupExecutor{
        logger:       logger,
        cmdExecutor:  cmdExecutor,
        keepTempDirs: keepTempDirs,
        isDryRun:     isDryRun,
    }
}

// ExecuteGroup: 1つのグループを実行
func (e *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    group *runnertypes.CommandGroup,
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

    // ステップ3: グループの展開済みワークディレクトリを設定
    // (1) group.ExpandedWorkDir に物理/仮想パスを設定
    group.ExpandedWorkDir = workDir

    // (2) グループレベル変数に __runner_workdir を設定
    group.ExpandedVars[variable.AutoVarPrefix + variable.AutoVarKeyWorkDir] = workDir

    // ステップ4: コマンド実行ループ
    for _, cmd := range group.Commands {
        // ステップ4-1: 変数マップを構築
        // グループ変数をベースに、コマンド固有の変数で上書き（コマンドが優先）
        vars := e.buildVarsForCommand(cmd, group)

        // ステップ4-2: コマンドの変数展開（一度だけ）
        expandedCmd, err := e.expandCommand(cmd, vars)
        if err != nil {
            return fmt.Errorf("failed to expand command '%s': %w", cmd.Name, err)
        }

        // ステップ4-3: コマンド実行
        output, err := e.cmdExecutor.Execute(ctx, expandedCmd)
        if err != nil {
            return fmt.Errorf("command '%s' failed: %w", cmd.Name, err)
        }

        // ステップ4-4: 出力ハンドリング（既存ロジック）
        e.handleCommandOutput(cmd, output)
    }

    return nil
}

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
    tempDirMgr := NewTempDirManager(e.logger, group.Name, e.isDryRun)

    // 一時ディレクトリを生成
    tempDir, err := tempDirMgr.Create()
    if err != nil {
        return "", nil, err
    }

    return tempDir, tempDirMgr, nil
}

// handleCommandOutput: コマンド出力を処理（既存ロジック）
func (e *DefaultGroupExecutor) handleCommandOutput(
    cmd *runnertypes.Command,
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
| 一時ディレクトリ作成（dry-run） | INFO | ログ | `[DRY-RUN] Would create temporary directory for group 'X': /path` |
| 一時ディレクトリ削除成功 | DEBUG | ログ | `Cleaned up temporary directory: /path` |
| 一時ディレクトリ削除（dry-run） | DEBUG | ログ | `[DRY-RUN] Would delete temporary directory: /path` |
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

#### 11.1.1 dry-runモードでの検証方針

**基本方針**:
- **構文的検証は実行**: 絶対パス要件、パストラバーサル検出
- **存在チェックはスキップ**: ファイル/ディレクトリの実在は検証しない

**理由**:
- dry-runでは仮想パス（`/tmp/scr-<group>-dryrun-<timestamp>`）を使用
- 仮想パスは実在しないが、構文的には正当なパス
- 変数展開の正当性（未定義変数参照でないこと）は、変数マップに存在するかで判断

**変数展開の検証**:
```go
// config.ExpandString の既存動作:
// - 変数が ExpandedVars に存在しない → ErrUndefinedVariable
// - 変数が存在する → 展開を実行（値が仮想パスでも問題なし）

// dry-runモード
group.ExpandedVars["__runner_workdir"] = "/tmp/scr-backup-dryrun-20251018143025"

// 展開時: 変数が定義されているため成功
ExpandString("%{__runner_workdir}/file", group.ExpandedVars, ...)
// → "/tmp/scr-backup-dryrun-20251018143025/file"

// 未定義変数の参照: 失敗
ExpandString("%{undefined_var}/file", group.ExpandedVars, ...)
// → ErrUndefinedVariable
```

### 11.2 検証の実装

```go
func validatePath(path string, isDryRun bool) error {
    // ルール1: 絶対パスのみ許可（dry-runでも実行）
    if !filepath.IsAbs(path) {
        return fmt.Errorf("path must be absolute: %s", path)
    }

    // ルール2: パストラバーサル攻撃を防ぐため、".." コンポーネントを禁止
    // (dry-runでも実行)
    // filepath.Clean() は // を正規化してしまうため、コンポーネント単位で検証
    for _, part := range strings.Split(path, string(filepath.Separator)) {
        if part == ".." {
            return fmt.Errorf("path contains '..' component, which is a security risk: %s", path)
        }
    }

    // ルール3: ファイル/ディレクトリの存在チェック
    // (dry-runではスキップ)
    if !isDryRun {
        if _, err := os.Stat(path); err != nil {
            // 注意: 存在しない場合はエラーを返すか、警告のみにするかは
            // コンテキストによる（グループのworkdirなら必須、コマンドの出力先なら作成される可能性がある）
            // ここでは検証のみを行い、実際のエラーハンドリングは呼び出し元で行う
            return fmt.Errorf("path does not exist: %s: %w", path, err)
        }
    }

    // ルール4: シンボリックリンク検証
    // (既存の SafeFileIO の仕組みを活用、dry-runではスキップされる可能性あり)

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
workdir = "/var/backup"              # OK: 固定パス
# workdir = "%{backup_base}/data"    # OK: 他の変数参照
# workdir = "%{__runner_workdir}"    # NG: 未定義変数エラー

[[groups.commands]]
name = "dump"
cmd = "pg_dump"
args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]
# OK: %{__runner_workdir} はコマンドレベルで使用可能

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
    // - group.ExpandedVars["__runner_workdir"]: /tmp/scr-test-XXXXXX
    // - vars := buildVarsForCommand(cmd, group) で変数を統合
    // - expandedCmd := expandCommand(cmd, vars) で展開
    // - 期待: expandedCmd.ExpandedArgs[0] = /tmp/scr-test-XXXXXX/file
    // - 期待: expandedCmd.ExpandedArgs[1] = /dest
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

#### T004-dryrun: dry-runモードでの一時ディレクトリ管理
```go
func TestTempDirManagerDryRun(t *testing.T) {
    // テスト1: dry-runモードでのCreate()
    // - isDryRun: true
    // - 期待: 仮想パスが返される（/tmp/scr-test-dryrun-YYYYMMDDHHMMSS）
    // - 期待: 実際のディレクトリは作成されない
    // - 確認: "[DRY-RUN] Would create temp dir" ログ

    // テスト2: dry-runモードでのCleanup()
    // - isDryRun: true
    // - 期待: エラーなし（常に nil）
    // - 期待: 実際の削除は行われない
    // - 確認: "[DRY-RUN] Would delete temp dir" ログ

    // テスト3: dry-runでの変数展開
    // - 仮想パスが %{__runner_workdir} に設定される
    // - コマンド引数で正しく展開される
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
- [ ] `CommandGroup.ExpandedWorkDir` フィールドを追加（展開済みワークディレクトリ）
- [ ] `Command.Dir` → `Command.WorkDir` に変更
- [ ] `Command.ExpandedWorkDir` フィールドを追加

### Phase 2: 一時ディレクトリ機能
- [ ] `TempDirManager` インターフェース定義
- [ ] `DefaultTempDirManager` 実装
  - [ ] `isDryRun` フラグのサポート
  - [ ] dry-runモードでの仮想パス生成
  - [ ] dry-runモードでのログ出力（"[DRY-RUN]" プレフィックス）
- [ ] `--keep-temp-dirs` フラグを Runner に追加
- [ ] Runner から GroupExecutor へ `keepTempDirs` と `isDryRun` を渡す
- [ ] `defer` で条件付きクリーンアップ登録（`if !keepTempDirs { mgr.Cleanup() }`）

### Phase 3: 変数展開
- [ ] `AutoVarKeyWorkDir` 定数を追加（`internal/runner/variable`）
- [ ] `GroupExecutor.ExecuteGroup()` で以下を設定:
  - [ ] `group.ExpandedWorkDir` に展開済みワークディレクトリを設定
  - [ ] `group.ExpandedVars["__runner_workdir"]` に同じ値を設定
- [ ] `GroupExecutor.expandCommandWithWorkDir()` を実装（コマンド変数の再展開）
- [ ] `CommandExecutor.resolveWorkDir()` を実装（`group.ExpandedWorkDir` を参照）

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
- 生の値（`WorkDir`）と展開済みの値（`ExpandedWorkDir`）を分離管理
- 破壊的変更は TOML パーサーレベルで検出
