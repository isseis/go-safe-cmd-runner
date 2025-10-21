# 詳細仕様書: 作業ディレクトリ仕様の再設計

## 1. 概要

本ドキュメントは、タスク0034「作業ディレクトリ仕様の再設計」の詳細仕様を記述します。

## 2. 型定義（Task 0035 完了後）

### 2.1 設定型の変更

#### 2.1.1 Task 0035 による変更（完了済み）

Task 0035 により、以下の Spec/Runtime 構造体が導入されました:

**Spec 層** (`internal/runner/runnertypes/spec.go`):
- `ConfigSpec`: TOML ルート設定
- `GlobalSpec`: グローバル設定
- `GroupSpec`: グループ設定
- `CommandSpec`: コマンド設定

**Runtime 層** (`internal/runner/runnertypes/runtime.go`):
- `RuntimeGlobal`: 実行時グローバル設定（`ExpandedVars` を含む）
- `RuntimeGroup`: 実行時グループ設定（`ExpandedVars`, `Commands []*RuntimeCommand` を含む）
- `RuntimeCommand`: 実行時コマンド設定（`ExpandedCmd`, `ExpandedArgs` を含む）

#### 2.1.2 Task 0034 で変更するフィールド

**変更対象**: `internal/runner/runnertypes/spec.go`

```go
// 変更前（Task 0035 完了時点）
type GlobalSpec struct {
    WorkDir          string   `toml:"workdir"`  // Task 0034 で削除
    Timeout          int      `toml:"timeout"`
    LogLevel         string   `toml:"log_level"`
    SkipStandardPaths bool    `toml:"skip_standard_paths"`
    MaxOutputSize    int64    `toml:"max_output_size"`
    VerifyFiles      []string `toml:"verify_files"`
    EnvAllowlist     []string `toml:"env_allowlist"`
    Env              []string `toml:"env"`
    FromEnv          []string `toml:"from_env"`
    Vars             []string `toml:"vars"`
}

type GroupSpec struct {
    Name         string        `toml:"name"`
    Description  string        `toml:"description"`
    Priority     int           `toml:"priority"`
    TempDir      bool          `toml:"temp_dir"`  // Task 0034 で削除
    WorkDir      string        `toml:"workdir"`
    Commands     []CommandSpec `toml:"commands"`
    VerifyFiles  []string      `toml:"verify_files"`
    EnvAllowlist []string      `toml:"env_allowlist"`
    Env          []string      `toml:"env"`
    FromEnv      []string      `toml:"from_env"`
    Vars         []string      `toml:"vars"`
}

type CommandSpec struct {
    Name         string   `toml:"name"`
    Description  string   `toml:"description"`
    Cmd          string   `toml:"cmd"`
    Args         []string `toml:"args"`
    Dir          string   `toml:"dir"`  // Task 0034 で WorkDir に名称変更
    Timeout      int      `toml:"timeout"`
    RunAsUser    string   `toml:"run_as_user"`
    RunAsGroup   string   `toml:"run_as_group"`
    MaxRiskLevel string   `toml:"max_risk_level"`
    Output       string   `toml:"output"`
    Env          []string `toml:"env"`
    FromEnv      []string `toml:"from_env"`
    Vars         []string `toml:"vars"`
}

// 変更後（Task 0034 完了後）
type GlobalSpec struct {
    // WorkDir を削除（グローバルレベルでのデフォルト設定は廃止）
    Timeout          int      `toml:"timeout"`
    LogLevel         string   `toml:"log_level"`
    SkipStandardPaths bool    `toml:"skip_standard_paths"`
    MaxOutputSize    int64    `toml:"max_output_size"`
    VerifyFiles      []string `toml:"verify_files"`
    EnvAllowlist     []string `toml:"env_allowlist"`
    Env              []string `toml:"env"`
    FromEnv          []string `toml:"from_env"`
    Vars             []string `toml:"vars"`
}

type GroupSpec struct {
    Name         string        `toml:"name"`
    Description  string        `toml:"description"`
    Priority     int           `toml:"priority"`
    // TempDir を削除（デフォルトで一時ディレクトリを使用するため不要）
    WorkDir      string        `toml:"workdir"`  // TOMLから読み込んだ生の値
    Commands     []CommandSpec `toml:"commands"`
    VerifyFiles  []string      `toml:"verify_files"`
    EnvAllowlist []string      `toml:"env_allowlist"`
    Env          []string      `toml:"env"`
    FromEnv      []string      `toml:"from_env"`
    Vars         []string      `toml:"vars"`
}

type CommandSpec struct {
    Name         string   `toml:"name"`
    Description  string   `toml:"description"`
    Cmd          string   `toml:"cmd"`
    Args         []string `toml:"args"`
    WorkDir      string   `toml:"workdir"`  // 名称変更（旧: dir）
    Timeout      int      `toml:"timeout"`
    RunAsUser    string   `toml:"run_as_user"`
    RunAsGroup   string   `toml:"run_as_group"`
    MaxRiskLevel string   `toml:"max_risk_level"`
    Output       string   `toml:"output"`
    Env          []string `toml:"env"`
    FromEnv      []string `toml:"from_env"`
    Vars         []string `toml:"vars"`
}
```

#### 2.1.3 Runtime 構造体の拡張

**変更対象**: `internal/runner/runnertypes/runtime.go`

```go
// RuntimeGroup への追加
type RuntimeGroup struct {
    Spec *GroupSpec  // 元の Spec への参照

    // 展開済み変数（Task 0035 で実装済み）
    ExpandedVerifyFiles []string
    ExpandedEnv         map[string]string
    ExpandedVars        map[string]string

    // Task 0034 で追加
    EffectiveWorkDir string  // 実行時に決定されたワークディレクトリ（一時または固定）

    // 展開済みコマンド（Task 0035 で実装済み）
    Commands []*RuntimeCommand
}

// RuntimeCommand への追加
type RuntimeCommand struct {
    Spec *CommandSpec  // 元の Spec への参照

    // 展開済みコマンド情報（Task 0035 で実装済み）
    ExpandedCmd  string
    ExpandedArgs []string
    ExpandedEnv  map[string]string
    ExpandedVars map[string]string

    // Task 0034 で追加
    EffectiveWorkDir string  // コマンドレベルで決定されたワークディレクトリ
    EffectiveTimeout int     // 実行時タイムアウト（Task 0035 で実装済み）
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

### 2.2 実行時の変数管理（Task 0035 完了後）

#### 2.2.1 変数展開の遅延評価（Task 0035 で実装済み）

Task 0035 により、変数展開の遅延評価が実装されました:

**設計方針**:
- **TOMLロード時**: `ConfigSpec` のみ生成、変数は未展開
- **実行時**: `ExpandGlobal()`, `ExpandGroup()`, `ExpandCommand()` で段階的に展開

**展開の流れ**:
1. `ExpandGlobal(GlobalSpec)` → `RuntimeGlobal` (グローバル変数を展開)
2. `ExpandGroup(GroupSpec, RuntimeGlobal)` → `RuntimeGroup` (グループ変数を展開、グローバル変数を継承)
3. `ExpandCommand(CommandSpec, RuntimeGroup)` → `RuntimeCommand` (コマンド変数を展開、グループ変数を継承)

**利点**:
1. **優先順位の明確化**: コマンド変数 > グループ変数 > グローバル変数
2. **二重展開を回避**: 各レベルで一度だけ展開
3. **Spec/Runtime 分離**: TOML由来の値と実行時の値を明確に区別

#### 2.2.2 ワークディレクトリの設定（Task 0034 で実装）

Task 0034 では、Task 0035 の Runtime 構造体にワークディレクトリ機能を追加します:

**実行時のワークディレクトリ情報の設定**:

1. `ExpandGroup(GroupSpec, RuntimeGlobal)` でグループ変数を展開 → `RuntimeGroup` 生成
2. グループ実行開始時に `resolveGroupWorkDir(RuntimeGroup)` でワークディレクトリを決定
3. 決定した物理/仮想パスを以下に設定:
   - `RuntimeGroup.EffectiveWorkDir`: 後続処理（`resolveCommandWorkDir` など）で参照
   - `RuntimeGroup.ExpandedVars["__runner_workdir"]`: コマンドレベルの変数展開で参照
4. `ExpandCommand(CommandSpec, RuntimeGroup)` でコマンド変数を展開 → `RuntimeCommand` 生成

**設計の利点**:
- `GroupSpec.WorkDir`: TOMLの生の値を保持（トレーサビリティ）
- `RuntimeGroup.EffectiveWorkDir`: 展開済みの値を保持（処理ロジックで使用）
- Task 0035 の Spec/Runtime 分離パターンに準拠
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
- `TempDirManager` は `DefaultGroupExecutor.ExecuteGroup()` メソッド内でローカルに使用される
- `resourceManager` (`NormalResourceManager` / `DryRunResourceManager`) とは独立
- `isDryRun` フラグは `DefaultGroupExecutor` のフィールドとして保持

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

### 3.2 DefaultGroupExecutor.ExecuteGroup() メソッド（Task 0035 完了後）

**パッケージ**: `internal/runner`

```go
// DefaultGroupExecutor.ExecuteGroup: 1つのグループを実行
//
// 引数:
//   ctx: コンテキスト
//   groupSpec: グループ設定（Spec層）
//
// 戻り値:
//   error: エラー（グループまたはコマンド実行失敗）
//
// 動作（Task 0035 完了後 + Task 0034 実装）:
//   1. ExpandGroup(groupSpec, runtimeGlobal) でグループ変数を展開 → RuntimeGroup 生成
//   2. resolveGroupWorkDir(runtimeGroup) でワークディレクトリを決定
//   3. 一時ディレクトリの場合は TempDirManager.Create() で生成
//   4. 決定したワークディレクトリを以下に設定:
//      - runtimeGroup.EffectiveWorkDir
//      - runtimeGroup.ExpandedVars["__runner_workdir"]
//   5. defer で条件付きクリーンアップを登録（if !keepTempDirs { mgr.Cleanup() }）
//   6. コマンド実行ループ:
//      - for each commandSpec in groupSpec.Commands:
//        a. ExpandCommand(commandSpec, runtimeGroup) → RuntimeCommand 生成
//        b. executor.Execute(ctx, runtimeCommand)
func (e *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
) error
```

**Task 0035 による変更点**:
- 引数: `*runnertypes.CommandGroup` → `*runnertypes.GroupSpec`
- グループ変数展開: `ExpandGroup()` を使用して `RuntimeGroup` を生成
- コマンド変数展開: `ExpandCommand()` を使用して `RuntimeCommand` を生成
- ワークディレクトリ設定先: `group.ExpandedVars` → `runtimeGroup.ExpandedVars`

**dry-runモードでの動作**:
- `e.isDryRun` フラグに基づいて `TempDirManager` を作成（`isDryRun` 引数を渡す）
- 仮想パスが生成され、`runtimeGroup.ExpandedVars["__runner_workdir"]` に設定される
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

## 4. ワークディレクトリ決定ロジック（Task 0035 完了後）

### 4.1 優先順位（新規）

```
Level 1: コマンドレベル (CommandSpec.WorkDir → RuntimeCommand.EffectiveWorkDir)
  └─ 指定: そのパスを使用
  └─ 未指定: Level 2 へ

Level 2: グループレベル (GroupSpec.WorkDir → RuntimeGroup.EffectiveWorkDir)
  └─ 指定: そのパスを使用
  └─ 未指定: Level 3 へ

Level 3: 自動生成一時ディレクトリ（デフォルト）
  └─ /tmp/scr-<groupName>-XXXXXX を自動生成
```

### 4.2 決定アルゴリズム（Task 0035 完了後）

```go
// resolveGroupWorkDir: グループのワークディレクトリを決定
// 戻り値: (workdir, tempDirManager, error)
//   - 固定ディレクトリの場合: tempDirManager は nil
//   - 一時ディレクトリの場合: tempDirManager は非nil（クリーンアップに使用）
//
// Task 0035 による変更点:
//   - 引数: *runnertypes.CommandGroup → *runnertypes.RuntimeGroup
//   - group.WorkDir → runtimeGroup.Spec.WorkDir (TOML の生の値)
//   - group.ExpandedVars → runtimeGroup.ExpandedVars (展開済み変数)
func (e *DefaultGroupExecutor) resolveGroupWorkDir(
    runtimeGroup *runnertypes.RuntimeGroup,
) (string, TempDirManager, error) {
    // グループレベル WorkDir が指定されている?
    if runtimeGroup.Spec.WorkDir != "" {
        // 変数展開を実行（注意: __runner_workdir はまだ未定義）
        level := fmt.Sprintf("group[%s]", runtimeGroup.Spec.Name)
        expandedWorkDir, err := config.ExpandString(
            runtimeGroup.Spec.WorkDir,
            runtimeGroup.ExpandedVars,  // __runner_workdir は含まれない
            level,
            "workdir",
        )
        if err != nil {
            return "", nil, fmt.Errorf("failed to expand group workdir: %w", err)
        }

        e.logger.Info(fmt.Sprintf(
            "Using group workdir for '%s': %s",
            runtimeGroup.Spec.Name, expandedWorkDir,
        ))
        return expandedWorkDir, nil, nil
    }

    // 一時ディレクトリマネージャーを作成
    // 注: isDryRun フラグは GroupExecutor のフィールドとして保持
    tempDirMgr := NewTempDirManager(e.logger, runtimeGroup.Spec.Name, e.isDryRun)

    // 一時ディレクトリを生成
    // dry-runモードでは仮想パスが返される
    tempDir, err := tempDirMgr.Create()
    if err != nil {
        return "", nil, err
    }

    return tempDir, tempDirMgr, nil
}

// resolveCommandWorkDir: コマンドのワークディレクトリを決定
// 優先度: RuntimeCommand.EffectiveWorkDir > RuntimeGroup.EffectiveWorkDir
//
// Task 0035 による変更点:
//   - 引数: (*runnertypes.Command, *runnertypes.CommandGroup) → (*runnertypes.RuntimeCommand, *runnertypes.RuntimeGroup)
//   - RuntimeCommand は RuntimeGroup への参照を持つ（将来実装予定）
func (e *DefaultCommandExecutor) resolveCommandWorkDir(
    runtimeCmd *runnertypes.RuntimeCommand,
    runtimeGroup *runnertypes.RuntimeGroup,
) string {
    // 優先度1: コマンドレベル EffectiveWorkDir
    if runtimeCmd.EffectiveWorkDir != "" {
        return runtimeCmd.EffectiveWorkDir
    }

    // 優先度2: グループレベル EffectiveWorkDir
    // 注: ExecuteGroup で resolveGroupWorkDir により決定・設定済み
    //     （一時ディレクトリまたは固定ディレクトリの物理/仮想パス）
    return runtimeGroup.EffectiveWorkDir
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

### 6.1 グループ実行時の __runner_workdir 設定（Task 0035 完了後）

**ファイル**: `internal/runner/group_executor.go`

**注意**: 実装は `DefaultGroupExecutor.ExecuteGroup()` メソッドです。

```go
// ExecuteGroup: 1つのグループを実行（Task 0035 完了後 + Task 0034 実装）
func (e *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
) error {
    // ステップ1: グループ変数を展開（Task 0035 で実装済み）
    runtimeGroup, err := config.ExpandGroup(groupSpec, e.runtimeGlobal)
    if err != nil {
        return fmt.Errorf("failed to expand group '%s': %w", groupSpec.Name, err)
    }

    // ステップ2: ワークディレクトリを決定（Task 0034 で実装）
    workDir, tempDirMgr, err := e.resolveGroupWorkDir(runtimeGroup)
    if err != nil {
        return fmt.Errorf("failed to resolve work directory: %w", err)
    }

    // ステップ3: defer でクリーンアップを登録（重要: エラー時も実行）
    // dry-runモードでもCleanup()は呼ばれるが、実際の削除は行わない
    if tempDirMgr != nil && !e.keepTempDirs {
        defer func() {
            err := tempDirMgr.Cleanup()
            if err != nil {
                e.logger.Error(fmt.Sprintf("Cleanup warning: %v", err))
            }
        }()
    }

    // ステップ4: グループの実行時ワークディレクトリを設定（Task 0034 で実装）
    // (1) RuntimeGroup.EffectiveWorkDir に物理/仮想パスを設定
    //     dry-runモードでは仮想パスが設定される
    runtimeGroup.EffectiveWorkDir = workDir

    // (2) グループレベル変数に __runner_workdir を設定
    //     コマンドレベルの変数展開で参照可能
    runtimeGroup.ExpandedVars[variable.AutoVarPrefix + variable.AutoVarKeyWorkDir] = workDir

    // ステップ5: コマンド実行ループ（Task 0035 で実装済み + Task 0034 で拡張）
    for i := range groupSpec.Commands {
        cmdSpec := &groupSpec.Commands[i]

        // ステップ5-1: コマンド変数を展開（Task 0035 で実装済み）
        runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup)
        if err != nil {
            return fmt.Errorf("failed to expand command '%s': %w", cmdSpec.Name, err)
        }

        // ステップ5-2: コマンドレベルのワークディレクトリを決定（Task 0034 で実装）
        runtimeCmd.EffectiveWorkDir = e.resolveCommandWorkDir(runtimeCmd, runtimeGroup)

        // ステップ5-3: コマンド実行
        // dry-runモードでは executor が DryRunExecutor なので、
        // 実際のコマンド実行は行わず、分析のみ実施
        err = e.executor.Execute(ctx, runtimeCmd)
        if err != nil {
            return fmt.Errorf("command '%s' failed: %w", cmdSpec.Name, err)
        }
    }

    return nil
}

// Task 0035 により、buildVarsForCommand() と expandCommand() は削除されました。
// これらの機能は config.ExpandGroup() と config.ExpandCommand() に統合されています。
```

### 6.2 Runtime 構造体の変更（Task 0035 完了後）

**ファイル**: `internal/runner/runnertypes/runtime.go`

Task 0035 により、Spec/Runtime が分離されたため、Task 0034 では Runtime 構造体にワークディレクトリ機能を追加します:

```go
// RuntimeCommand: 実行時コマンド設定（Task 0035 で導入、Task 0034 で拡張）
type RuntimeCommand struct {
    Spec *CommandSpec  // 元の CommandSpec への参照

    // 展開済みコマンド情報（Task 0035 で実装済み）
    ExpandedCmd  string
    ExpandedArgs []string
    ExpandedEnv  map[string]string
    ExpandedVars map[string]string

    // 実行時情報（Task 0035 で実装済み）
    EffectiveTimeout int

    // Task 0034 で追加
    EffectiveWorkDir string  // 実行時に決定されたワークディレクトリ
}
```

**Task 0035 による変更点**:
1. `CommandSpec`: TOML から読み込んだ未展開の設定（`Cmd`, `Args`, `WorkDir`, `Env`, `Vars` など）
2. `RuntimeCommand`: 実行時に展開された設定（`ExpandedCmd`, `ExpandedArgs`, `ExpandedEnv`, `ExpandedVars` など）
3. `EffectiveWorkDir`: Task 0034 で追加するフィールド（`resolveCommandWorkDir()` で設定）

### 6.3 ワークディレクトリ決定ロジック

コマンド実行時のワークディレクトリ決定ロジックについては、**Section 4.2「ワークディレクトリ決定ロジック」** で詳細に定義されています。

**関数**: `DefaultCommandExecutor.resolveCommandWorkDir()`
**ファイル**: `internal/runner/executor/command_executor.go`
**定義箇所**: Section 4.2 (line 387-402)

**概要**:
- 優先度1: `Command.ExpandedWorkDir`（コマンドレベルで指定された場合）
- 優先度2: `Group.ExpandedWorkDir`（グループレベルで決定・展開済み）

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
// 詳細な実装は Section 4.1 (line 345-385) を参照
// 戻り値: (workdir, tempDirManager, error)
//   - 固定ディレクトリの場合: tempDirManager は nil
//   - 一時ディレクトリの場合: tempDirManager は非nil（クリーンアップに使用）

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

    // 注意: ファイル/ディレクトリの存在チェックは行わない
    // 理由:
    // - グループ内のコマンドがディレクトリを作成するケース (mkdir -p) を妨げない
    // - 存在しないパスへのアクセスは、コマンド実行時のOSエラーで適切にハンドリングされる
    // - dry-runモードとの動作の一貫性を保つ

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

#### T005: dry-runモードでの一時ディレクトリ管理
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

#### T006: グループ実行全体
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
- [ ] `GroupExecutor.expandCommand()` を実装（コマンド変数の再展開）
- [ ] `CommandExecutor.resolveCommandWorkDir()` を実装（`group.ExpandedWorkDir` を参照）

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
