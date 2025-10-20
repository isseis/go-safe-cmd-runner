# 実装計画書: 作業ディレクトリ仕様の再設計

## 1. 概要

本ドキュメントは、タスク0034「作業ディレクトリ仕様の再設計」の実装作業計画を記述します。

### 1.1 前提ドキュメント

本実装計画は以下のドキュメントに基づいて作成されています：

| ドキュメント | 参照目的 |
|----------|---------|
| `01_requirements.md` | 機能要件、セキュリティ要件、テスト要件の確認 |
| `02_architecture.md` | アーキテクチャ設計、コンポーネント設計の理解 |
| `03_specification.md` | 詳細仕様、API仕様、実装方法の確認 |

### 1.2 実装方針

- **段階的な実装**: 依存関係を考慮し、5つのPhaseに分けて実装
- **TDD (Test-Driven Development)**: 各Phaseで単体テストを先行実装
- **破壊的変更の明示**: 既存TOMLファイルとの非互換性を明確に文書化
- **Dry-Runモードのサポート**: 全機能でdry-runモードを考慮した実装

### 1.3 実装スコープ

#### 対象機能（In Scope）

- ✅ `Global.WorkDir` フィールドの削除
- ✅ `Group.TempDir` フィールドの削除
- ✅ `Command.Dir` → `Command.WorkDir` への変更
- ✅ `TempDirManager` の新規実装（dry-runサポート含む）
- ✅ グループごとの自動一時ディレクトリ生成
- ✅ `__runner_workdir` 予約変数の実装
- ✅ 変数展開ロジックの統合
- ✅ `--keep-temp-dirs` フラグの実装
- ✅ 単体テスト・統合テスト
- ✅ サンプルファイルの更新
- ✅ ユーザードキュメントの更新

#### 対象外（Out of Scope）

- ❌ 既存TOMLファイルの自動マイグレーション（YAGNI原則による）
- ❌ カスタムエラーメッセージの実装（go-toml/v2の標準エラーを使用）
- ❌ Windows環境での一時ディレクトリパーミッション設定（Linux/Unixのみ）

## 2. フェーズ別実装計画

### Phase 0: 事前準備（準備作業）

**目的**: 実装に必要な情報の収集と確認

**作業項目**:

| ID | タスク | 作業内容 | 所要時間 |
|----|-------|---------|---------|
| P0-1 | 既存コード調査 | 現在の作業ディレクトリ関連コードの把握 | 1h |
| P0-2 | 影響範囲分析 | 変更が影響する全ファイルのリストアップ | 0.5h |
| P0-3 | テストデータ準備 | 単体テスト用のサンプルTOMLファイル作成 | 0.5h |

**成果物**:
- 影響範囲リスト（Markdown形式）
- テストデータディレクトリ（`test/fixtures/0034_workdir_redesign/`）

**完了条件**:
- [ ] 変更対象ファイルが全てリストアップされている
- [ ] テスト用のサンプルTOMLが最低3パターン用意されている

---

### Phase 1: 型定義の変更（破壊的変更）

**目的**: 設定構造体の変更を実施し、TOMLパーサーレベルでの検証を確立する

**依存関係**: Phase 0完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P1-1 | GlobalConfig変更 | `internal/runner/runnertypes/config.go` | `WorkDir` フィールドを削除 | 0.5h |
| P1-2 | CommandGroup変更 | `internal/runner/runnertypes/config.go` | `TempDir` 削除、`ExpandedWorkDir` 追加 | 0.5h |
| P1-3 | Command変更 | `internal/runner/runnertypes/config.go` | `Dir` 削除、`WorkDir` 追加（tomlタグ付き）、`ExpandedWorkDir` 追加 | 0.5h |
| P1-4 | 単体テスト作成 | `internal/runner/runnertypes/config_test.go` | 型定義のテスト | 1h |
| P1-5 | 廃止フィールド検出テスト | `internal/runner/config/loader_test.go` | TOMLパーサーエラー検証 | 1h |

**詳細実装内容**:

#### P1-1: GlobalConfig変更

```go
// 変更前
type GlobalConfig struct {
    WorkDir string `toml:"workdir"`  // 削除
    // ... その他のフィールド
}

// 変更後
type GlobalConfig struct {
    // WorkDir は削除
    // ... その他のフィールド
}
```

#### P1-2: CommandGroup変更

```go
// 変更前
type CommandGroup struct {
    TempDir bool   `toml:"temp_dir"`  // 削除
    WorkDir string `toml:"workdir"`
    // ... その他のフィールド
}

// 変更後
type CommandGroup struct {
    WorkDir         string `toml:"workdir"`  // TOMLから読み込んだ生の値
    ExpandedWorkDir string                   // 展開済みの物理/仮想パス（実行時に設定）
    // ... その他のフィールド
}
```

#### P1-3: Command変更

```go
// 変更前
type Command struct {
    Dir string `toml:"dir"`  // 削除
    // ... その他のフィールド
}

// 変更後
type Command struct {
    WorkDir         string `toml:"workdir"`  // 新規追加: TOMLから読み込む作業ディレクトリ（旧Dirフィールドの代替）
    ExpandedWorkDir string                   // 新規追加: 展開済みの作業ディレクトリ（実行時に設定）
    // ... その他のフィールド
}
```

**変更の詳細**:
- `Dir` フィールド（`toml:"dir"`）を**削除**
- `WorkDir` フィールド（`toml:"workdir"`）を**新規追加**（TOMLタグ名を変更）
- `ExpandedWorkDir` フィールドを**新規追加**（実行時に変数展開後の値を格納）

**成果物**:
- 更新された型定義ファイル
- 型定義のテストコード
- TOMLパーサーエラーの検証テスト

**完了条件**:
- [ ] `GlobalConfig.WorkDir` が削除されている
- [ ] `CommandGroup.TempDir` が削除されている
- [ ] `CommandGroup.ExpandedWorkDir` が追加されている
- [ ] `Command.Dir` が `Command.WorkDir` に変更されている
- [ ] `Command.ExpandedWorkDir` が追加されている
- [ ] 廃止フィールドを含むTOMLファイルでエラーが発生することが確認されている
- [ ] 全テストが成功している

**リスク**:
- **破壊的変更**: 既存のTOMLファイルがロードできなくなる
  - 対策: CHANGELOG.mdに明記し、サンプルファイルを更新

---

### Phase 2: TempDirManager実装（新規機能）

**目的**: 一時ディレクトリのライフサイクル管理を実装する

**依存関係**: Phase 1完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P2-1 | インターフェース定義 | `internal/runner/executor/tempdir_manager.go` | `TempDirManager` インターフェース定義 | 0.5h |
| P2-2 | 標準実装 | `internal/runner/executor/tempdir_manager.go` | `DefaultTempDirManager` 実装 | 2h |
| P2-3 | 単体テスト（通常モード） | `internal/runner/executor/tempdir_manager_test.go` | Create/Cleanup/Pathのテスト | 1.5h |
| P2-4 | 単体テスト（dry-run） | `internal/runner/executor/tempdir_manager_test.go` | dry-runモードのテスト | 1h |
| P2-5 | エラーケーステスト | `internal/runner/executor/tempdir_manager_test.go` | パーミッションエラー等のテスト | 1h |

**詳細実装内容**:

#### P2-1: インターフェース定義

```go
// TempDirManager: グループ単位の一時ディレクトリ管理
type TempDirManager interface {
    Create() (string, error)
    Cleanup() error
    Path() string
}

// NewTempDirManager: コンストラクタ
func NewTempDirManager(logger logging.Logger, groupName string, isDryRun bool) TempDirManager
```

#### P2-2: 標準実装（ポイント抜粋）

```go
type DefaultTempDirManager struct {
    logger      logging.Logger
    groupName   string
    isDryRun    bool
    tempDirPath string
}

func (m *DefaultTempDirManager) Create() (string, error) {
    if m.isDryRun {
        // 仮想パス生成（タイムスタンプベース）
        timestamp := time.Now().Format("20060102150405")
        tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("scr-%s-dryrun-%s", m.groupName, timestamp))
        m.tempDirPath = tempDir
        m.logger.Info(fmt.Sprintf("[DRY-RUN] Would create temporary directory for group '%s': %s", m.groupName, tempDir))
        return tempDir, nil
    }

    // 通常モード: 実際にディレクトリを生成
    prefix := fmt.Sprintf("scr-%s-", m.groupName)
    tempDir, err := os.MkdirTemp(os.TempDir(), prefix)
    if err != nil {
        return "", fmt.Errorf("failed to create temporary directory: %w", err)
    }

    // セキュリティ: 厳密に 0700 を保証
    if err := os.Chmod(tempDir, 0700); err != nil {
        os.RemoveAll(tempDir)
        return "", fmt.Errorf("failed to set permissions on temporary directory: %w", err)
    }

    m.tempDirPath = tempDir
    m.logger.Info(fmt.Sprintf("Created temporary directory for group '%s': %s", m.groupName, tempDir))
    return tempDir, nil
}
```

**テスト観点**:

| テストケース | 検証内容 |
|------------|---------|
| T001: 通常モードでの生成 | ディレクトリ存在、パーミッション0700、ログ出力 |
| T002: dry-runモードでの生成 | 仮想パス生成、実ディレクトリ不在、ログ出力 |
| T003: Cleanup成功 | ディレクトリ削除、DEBUGログ出力 |
| T004: Cleanup失敗 | ERRORログ、stderr出力、エラー返却 |
| T005: Cleanup (dry-run) | 実削除なし、DEBUGログ出力 |

**成果物**:
- `TempDirManager` インターフェース
- `DefaultTempDirManager` 実装
- 5種類以上の単体テスト

**完了条件**:
- [ ] `TempDirManager` インターフェースが定義されている
- [ ] `DefaultTempDirManager` が実装されている
- [ ] 通常モードで一時ディレクトリが生成・削除できる
- [ ] dry-runモードで仮想パスが生成される
- [ ] パーミッションが厳密に0700に設定される
- [ ] エラーケースが適切に処理される
- [ ] 全テストが成功している

**リスク**:
- **パーミッション設定の失敗**: umaskの影響でパーミッションが期待通りにならない可能性
  - 対策: `os.Chmod()` で明示的に0700を設定

---

### Phase 3: 変数展開とワークディレクトリ決定ロジック

**目的**: `__runner_workdir` 変数の実装とワークディレクトリ決定ロジックの統合

**依存関係**: Phase 1, 2完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P3-1 | 定数定義 | `internal/runner/variable/constants.go` | `AutoVarKeyWorkDir` 定数追加 | 0.5h |
| P3-2 | resolveGroupWorkDir実装 | `internal/runner/group_executor.go` | グループワークディレクトリ決定ロジック | 2h |
| P3-3 | resolveCommandWorkDir実装 | `internal/runner/executor/command_executor.go` | コマンドワークディレクトリ決定ロジック | 1h |
| P3-4 | buildVarsForCommand実装 | `internal/runner/group_executor.go` | 変数マップ構築ロジック | 1h |
| P3-5 | expandCommand実装 | `internal/runner/group_executor.go` | 新しいCommand構造体を生成し、全フィールド（Cmd/Args/WorkDir/Env）を展開 | 1.5h |
| P3-6 | ワークディレクトリ決定テスト | `internal/runner/group_executor_test.go` | resolveGroupWorkDir、resolveCommandWorkDirのテスト | 1h |
| P3-7 | 変数展開・統合テスト | `internal/runner/group_executor_test.go` | buildVarsForCommand、expandCommand、`__runner_workdir`展開のテスト | 2h |

**詳細実装内容**:

#### P3-1: 定数定義

```go
// internal/runner/variable/constants.go
package variable

const (
    // AutoVarKeyWorkDir is the key for the workdir auto internal variable (without prefix)
    AutoVarKeyWorkDir = "workdir"
)
```

#### P3-2: resolveGroupWorkDir実装

**注意**: `validatePath`関数は03_specification.md Section 11.2で定義されています。以下のセキュリティ検証のみを実施し、ディレクトリの存在チェックは行いません:
- 絶対パス要件（相対パス禁止）
- パストラバーサル禁止（".."コンポーネント検出）

存在しないディレクトリへのアクセスは、コマンド実行時のOSエラーで適切にハンドリングされます。これにより、グループ内でディレクトリを作成するコマンド（`mkdir -p`など）を妨げません。

```go
// internal/runner/group_executor.go

// resolveGroupWorkDir: グループのワークディレクトリを決定
// 戻り値: (workdir, tempDirManager, error)
func (e *DefaultGroupExecutor) resolveGroupWorkDir(
    group *runnertypes.CommandGroup,
) (string, executor.TempDirManager, error) {
    // グループレベル WorkDir が指定されている?
    if group.WorkDir != "" {
        // 変数展開（注意: __runner_workdir はまだ未定義）
        expandedWorkDir, err := config.ExpandString(
            group.WorkDir,
            group.ExpandedVars,
            fmt.Sprintf("group[%s]", group.Name),
            "workdir",
        )
        if err != nil {
            return "", nil, fmt.Errorf("failed to expand group workdir: %w", err)
        }

        // パス検証（セキュリティチェックのみ: 絶対パス、パストラバーサル禁止）
        // 注: ディレクトリの存在チェックは行わない（mkdir -pなどのケースを妨げない）
        if err := validatePath(expandedWorkDir); err != nil {
            return "", nil, err
        }

        return expandedWorkDir, nil, nil
    }

    // 一時ディレクトリマネージャーを作成
    tempDirMgr := executor.NewTempDirManager(e.logger, group.Name, e.isDryRun)

    // 一時ディレクトリを生成
    tempDir, err := tempDirMgr.Create()
    if err != nil {
        return "", nil, err
    }

    return tempDir, tempDirMgr, nil
}
```

#### P3-3: resolveCommandWorkDir実装

```go
// internal/runner/executor/command_executor.go

// resolveCommandWorkDir: コマンドのワークディレクトリを決定
func (e *DefaultCommandExecutor) resolveCommandWorkDir(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
) string {
    // 優先度1: コマンドレベル ExpandedWorkDir
    if cmd.ExpandedWorkDir != "" {
        return cmd.ExpandedWorkDir
    }

    // 優先度2: グループレベル ExpandedWorkDir
    return group.ExpandedWorkDir
}
```

#### P3-4: buildVarsForCommand実装

```go
// internal/runner/group_executor.go

// buildVarsForCommand: コマンド実行用の変数マップを構築
func (e *DefaultGroupExecutor) buildVarsForCommand(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
) map[string]string {
    vars := make(map[string]string)

    // グループ変数をコピー（ベースとして使用）
    for k, v := range group.ExpandedVars {
        vars[k] = v
    }

    // コマンド変数で上書き（優先）
    for k, v := range cmd.Vars {
        vars[k] = v
    }

    return vars
}
```

#### P3-5: expandCommand実装

**重要な設計方針**: 不変（immutable）なインスタンス生成

```go
// internal/runner/group_executor.go

// expandCommand: コマンドの変数展開を実行
// 重要: 元のcmdを変更せず、新しい展開済みコマンド構造体を作成する（immutable）
// 戻り値: 展開済みコマンド構造体
func (e *DefaultGroupExecutor) expandCommand(
    cmd *runnertypes.Command,
    vars map[string]string,
) (*runnertypes.Command, error) {
    level := fmt.Sprintf("command[%s]", cmd.Name)

    // 元のcmdのシャローコピーを作成（全フィールドをコピー）
    // これにより、TOMLから読み込んだ生の値（Cmd, Args, Vars, Envなど）を保持
    expanded := *cmd

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

    // Env の展開
    expanded.ExpandedEnv = make(map[string]string, len(cmd.Env))
    for key, value := range cmd.Env {
        expandedValue, err := config.ExpandString(value, vars, level, fmt.Sprintf("env[%s]", key))
        if err != nil {
            return nil, err
        }
        expanded.ExpandedEnv[key] = expandedValue
    }

    return &expanded, nil
}
```

**実装のポイント**:
1. **不変性の保証**: 元の`cmd`を変更せず、シャローコピーで`expanded`インスタンスを生成
2. **完全性の保証**: シャローコピーにより、TOMLから読み込んだ全フィールド（`Cmd`, `Args`, `Vars`, `Env`など）を保持
3. **全フィールドの展開**: `ExpandedCmd`, `ExpandedArgs`, `ExpandedWorkDir`, `ExpandedEnv`の各フィールドを変数展開して設定
4. **エラーハンドリング**: 各フィールドの展開時にエラーチェックを実施

**シャローコピーの利点**:
- デバッグ時に元のTOML定義を参照可能（トレーサビリティ）
- 後続処理で`cmd.Cmd`や`cmd.Args`などの生の値を参照できる
- 不完全なオブジェクトによるバグを防止

#### P3-6: ワークディレクトリ決定テスト

**テスト対象**: `resolveGroupWorkDir()`, `resolveCommandWorkDir()`

**テストケース**:

| テストID | テストケース | 検証内容 |
|---------|------------|---------|
| T006 | resolveGroupWorkDir: workdir未指定 | 一時ディレクトリが生成される |
| T007 | resolveGroupWorkDir: workdir指定 | 固定パスが使用される |
| T008 | resolveGroupWorkDir: 変数展開 | group.WorkDirの変数が正しく展開される |
| T009 | resolveGroupWorkDir: dry-runモード | 仮想パスが生成される |
| T010 | resolveCommandWorkDir: コマンドworkdir優先 | cmd.ExpandedWorkDirが優先される |
| T011 | resolveCommandWorkDir: グループworkdir使用 | cmd.ExpandedWorkDirが空の場合、group.ExpandedWorkDirが使用される |

#### P3-7: 変数展開・統合テスト

**テスト対象**: `buildVarsForCommand()`, `expandCommand()`, `__runner_workdir`の展開

**テストケース**:

| テストID | テストケース | 検証内容 |
|---------|------------|---------|
| T012 | buildVarsForCommand: グループ変数のコピー | group.ExpandedVarsが正しくコピーされる（`__runner_workdir`を含む） |
| T013 | buildVarsForCommand: コマンド変数の優先 | cmd.Varsがgroup.ExpandedVarsを上書きする |
| T014 | expandCommand: Cmd展開 | cmd.Cmdが正しく展開される |
| T015 | expandCommand: Args展開 | cmd.Argsの全要素が正しく展開される |
| T016 | expandCommand: WorkDir展開 | cmd.WorkDirが正しく展開される |
| T017 | expandCommand: Env展開 | cmd.Envの全要素が正しく展開される |
| T018 | expandCommand: `__runner_workdir`展開 | コマンド引数中の`%{__runner_workdir}`が正しく展開される |
| T019 | expandCommand: 不変性の保証 | 元のcmdが変更されていないことを確認 |
| T020 | expandCommand: エラーハンドリング | 未定義変数使用時にエラーが返される |

**成果物**:
- `AutoVarKeyWorkDir` 定数
- `resolveGroupWorkDir()` 関数
- `resolveCommandWorkDir()` 関数
- `buildVarsForCommand()` 関数
- `expandCommand()` 関数
- 単体テスト一式

**完了条件**:
- [ ] `AutoVarKeyWorkDir` 定数が定義されている
- [ ] グループワークディレクトリ決定ロジックが実装されている
- [ ] コマンドワークディレクトリ決定ロジックが実装されている
- [ ] 変数マップ構築ロジックが実装されている
- [ ] コマンド変数展開ロジックが実装されている
- [ ] `__runner_workdir` が正しく展開される
- [ ] 優先順位が正しく動作する
- [ ] 全テストが成功している

---

### Phase 4: GroupExecutor統合

**目的**: GroupExecutorに一時ディレクトリ管理とワークディレクトリ決定を統合する

**依存関係**: Phase 2, 3完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P4-1 | ExecuteGroup更新 | `internal/runner/group_executor.go` | ワークディレクトリ決定・設定を統合 | 2h |
| P4-2 | クリーンアップロジック | `internal/runner/group_executor.go` | `defer` でのクリーンアップ実装 | 1h |
| P4-3 | keep-temp-dirsフラグ | `cmd/runner/main.go` | コマンドラインフラグ追加 | 0.5h |
| P4-4 | Runnerへのフラグ伝播 | `internal/runner/runner.go` | フラグをGroupExecutorに渡す | 0.5h |
| P4-5 | 統合テスト | `cmd/runner/integration_test.go` | エンドツーエンドテスト | 3h |

**詳細実装内容**:

#### P4-1: ExecuteGroup更新

```go
// internal/runner/group_executor.go

func (e *DefaultGroupExecutor) ExecuteGroup(
    ctx context.Context,
    group *runnertypes.CommandGroup,
) error {
    // ステップ1: ワークディレクトリを決定
    workDir, tempDirMgr, err := e.resolveGroupWorkDir(group)
    if err != nil {
        return fmt.Errorf("failed to resolve group workdir: %w", err)
    }

    // ステップ2: group.ExpandedWorkDir に設定
    group.ExpandedWorkDir = workDir

    // ステップ3: __runner_workdir 変数に設定
    if group.ExpandedVars == nil {
        group.ExpandedVars = make(map[string]string)
    }
    group.ExpandedVars[variable.AutoVarPrefix+variable.AutoVarKeyWorkDir] = workDir

    // ステップ4: 一時ディレクトリの場合、クリーンアップを登録
    if tempDirMgr != nil {
        defer func() {
            if !e.keepTempDirs {
                if err := tempDirMgr.Cleanup(); err != nil {
                    e.logger.Error(fmt.Sprintf("Failed to cleanup temp dir: %v", err))
                }
            } else {
                e.logger.Info(fmt.Sprintf("Keeping temporary directory (--keep-temp-dirs): %s", tempDirMgr.Path()))
            }
        }()
    }

    // ステップ5: コマンド実行ループ
    for _, cmd := range group.Commands {
        // 変数マップを構築
        vars := e.buildVarsForCommand(cmd, group)

        // コマンドを展開
        expandedCmd, err := e.expandCommand(cmd, vars)
        if err != nil {
            return err
        }

        // コマンドを実行
        if err := e.commandExecutor.Execute(ctx, expandedCmd, group); err != nil {
            return err
        }
    }

    return nil
}
```

#### P4-3: keep-temp-dirsフラグ

```go
// cmd/runner/main.go

var (
    configPath    = flag.String("config", "", "Configuration file path")
    keepTempDirs  = flag.Bool("keep-temp-dirs", false, "Keep temporary directories after execution")
    // ... その他のフラグ
)

func main() {
    flag.Parse()

    // Runnerに渡す
    runner, err := runner.New(runner.Config{
        KeepTempDirs: *keepTempDirs,
        // ... その他の設定
    })
    // ...
}
```

**テスト観点**:

| テストケース | 検証内容 |
|------------|---------|
| T011: グループ実行（一時ディレクトリ） | 自動生成・自動削除 |
| T012: keep-temp-dirsフラグ | 一時ディレクトリが削除されない |
| T013: エラー時のクリーンアップ | エラー発生時も一時ディレクトリが削除される |
| T014: 複数グループ | 各グループで独立した一時ディレクトリ |
| T015: dry-runモード | 仮想パスで動作、実ディレクトリ不在 |

**成果物**:
- 更新された `ExecuteGroup()` メソッド
- `--keep-temp-dirs` フラグ
- 統合テスト一式

**完了条件**:
- [ ] `ExecuteGroup()` がワークディレクトリを決定・設定している
- [ ] 一時ディレクトリが自動的にクリーンアップされる
- [ ] `--keep-temp-dirs` フラグが動作する
- [ ] dry-runモードで仮想パスが使用される
- [ ] エラー時もクリーンアップが実行される
- [ ] 全テストが成功している

---

### Phase 5: ドキュメント・サンプル更新

**目的**: ユーザー向けドキュメントとサンプルファイルを更新する

**依存関係**: Phase 4完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P5-1 | ユーザーマニュアル更新 | `docs/user/README.ja.md` | ワークディレクトリの説明を更新 | 1h |
| P5-2 | サンプルファイル更新 | `sample/*.toml` | 廃止フィールドを削除、新仕様に更新 | 1h |
| P5-3 | CHANGELOG更新 | `CHANGELOG.md` | 破壊的変更の記載 | 0.5h |
| P5-4 | README更新 | `README.md`, `README.ja.md` | 新機能の説明追加 | 0.5h |
| P5-5 | サンプル動作確認 | `sample/` | 全サンプルファイルの動作確認 | 1h |

**詳細実装内容**:

#### P5-1: ユーザーマニュアル更新（追加セクション例）

```markdown
## 作業ディレクトリの設定

### デフォルト動作（推奨）

グループレベルで `workdir` を指定しない場合、自動的に一時ディレクトリが生成されます。
一時ディレクトリはグループ実行終了後に自動的に削除されます。

\`\`\`toml
[[groups]]
name = "backup"

[[groups.commands]]
name = "dump"
cmd = "pg_dump"
args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]
# /tmp/scr-backup-XXXXXX/dump.sql に出力される
\`\`\`

### 固定ディレクトリの使用

固定ディレクトリを使用する場合は、グループレベルで `workdir` を指定します。

\`\`\`toml
[[groups]]
name = "build"
workdir = "/opt/app"

[[groups.commands]]
name = "compile"
cmd = "make"
\`\`\`

### 予約変数 `%{__runner_workdir}`

コマンドレベルで `%{__runner_workdir}` を使用すると、実行時のワークディレクトリを参照できます。

### 一時ディレクトリの保持

デバッグ目的で一時ディレクトリを削除したくない場合は、`--keep-temp-dirs` フラグを使用します。

\`\`\`bash
$ ./runner --config backup.toml --keep-temp-dirs
\`\`\`
```

#### P5-3: CHANGELOG更新（追加セクション例）

```markdown
## [Unreleased]

### Changed - 破壊的変更

- **作業ディレクトリ仕様の再設計**: グローバルレベル設定を廃止し、デフォルトで一時ディレクトリを使用するように変更しました。
  - `Global.WorkDir` フィールドを削除（既存のTOMLファイルでエラーになります）
  - `Group.TempDir` フィールドを削除（既存のTOMLファイルでエラーになります）
  - `Command.Dir` を `Command.WorkDir` に変更（既存のTOMLファイルでエラーになります）
  - グループレベルで `workdir` を指定しない場合、自動的に一時ディレクトリが生成されるようになりました
  - 一時ディレクトリは実行終了後に自動的に削除されます（`--keep-temp-dirs` で保持可能）

### Added

- **`__runner_workdir` 予約変数**: コマンドレベルで実行時のワークディレクトリを参照できるようになりました
- **`--keep-temp-dirs` フラグ**: 一時ディレクトリをデバッグ目的で保持できるようになりました
- **Dry-runモードでの一時ディレクトリサポート**: dry-runモードで仮想パスを使用するようになりました

### Migration Guide

既存のTOMLファイルを以下のように更新してください：

1. `[global]` セクションの `workdir` を削除
2. `[[groups]]` セクションの `temp_dir` を削除
3. `[[groups.commands]]` の `dir` を `workdir` に変更
```

**サンプルファイル更新例**:

```toml
# 変更前（エラーになる）
[global]
workdir = "/tmp"

[[groups]]
name = "backup"
temp_dir = true

[[groups.commands]]
name = "dump"
cmd = "pg_dump"
dir = "/var/backups"

# 変更後
[[groups]]
name = "backup"
# workdir 未指定 → 自動一時ディレクトリ

[[groups.commands]]
name = "dump"
cmd = "pg_dump"
workdir = "/var/backups"  # dir → workdir に変更
```

**成果物**:
- 更新されたユーザードキュメント
- 更新されたサンプルファイル
- 更新されたCHANGELOG
- 更新されたREADME

**完了条件**:
- [ ] ユーザーマニュアルが新仕様に更新されている
- [ ] 全サンプルファイルが新仕様で動作する
- [ ] CHANGELOGに破壊的変更が記載されている
- [ ] READMEに新機能が説明されている
- [ ] マイグレーションガイドが提供されている

---

## 3. テスト計画

### 3.1 単体テスト

| テストID | テスト対象 | テストケース | 期待結果 |
|---------|-----------|------------|---------|
| UT-001 | TempDirManager.Create | 通常モード | ディレクトリ生成、0700、ログ出力 |
| UT-002 | TempDirManager.Create | dry-runモード | 仮想パス生成、実ディレクトリ不在 |
| UT-003 | TempDirManager.Cleanup | 正常削除 | ディレクトリ削除、DEBUGログ |
| UT-004 | TempDirManager.Cleanup | 削除失敗 | ERRORログ、stderr出力 |
| UT-005 | TempDirManager.Cleanup | dry-run | 実削除なし、DEBUGログ |
| UT-006 | resolveGroupWorkDir | workdir未指定 | 一時ディレクトリ生成 |
| UT-007 | resolveGroupWorkDir | workdir指定 | 固定パス使用 |
| UT-008 | resolveCommandWorkDir | コマンドworkdir優先 | コマンドレベル優先 |
| UT-009 | expandCommand | `__runner_workdir`展開 | 正しく展開される |
| UT-010 | config.ExpandString | グループレベルで`__runner_workdir`参照 | 未定義変数エラー |

### 3.2 統合テスト

| テストID | テストシナリオ | 検証内容 |
|---------|-------------|---------|
| IT-001 | グループ実行（一時ディレクトリ） | 自動生成・自動削除 |
| IT-002 | --keep-temp-dirsフラグ | 一時ディレクトリが削除されない |
| IT-003 | エラー時のクリーンアップ | エラー発生時も削除される |
| IT-004 | 複数グループ実行 | 各グループで独立した一時ディレクトリ |
| IT-005 | dry-runモード | 仮想パスで動作 |
| IT-006 | 固定ディレクトリ使用 | 指定したディレクトリで実行 |
| IT-007 | コマンドレベルworkdir | コマンドごとに異なるディレクトリ |

### 3.3 エラーケーステスト

| テストID | エラーシナリオ | 期待動作 |
|---------|-------------|---------|
| ET-001 | 一時ディレクトリ生成失敗 | グループ実行中止、エラーログ |
| ET-002 | 一時ディレクトリ削除失敗 | エラーログのみ、処理は継続 |
| ET-003 | 変数展開エラー | コマンド実行中止、エラーログ |
| ET-004 | 相対パス指定 | パス検証エラー |
| ET-005 | 廃止フィールド使用 | TOMLパーサーエラー |

### 3.4 パフォーマンステスト

| テストID | テスト内容 | 成功基準 |
|---------|-----------|---------|
| PT-001 | 一時ディレクトリ生成時間 | < 100ms |
| PT-002 | 一時ディレクトリ削除時間 | < 100ms |
| PT-003 | 100グループ連続実行 | メモリリークなし |

---

## 4. リスク管理

### 4.1 リスク一覧

| リスクID | リスク内容 | 影響度 | 発生確率 | 対策 |
|---------|-----------|-------|---------|------|
| R-001 | 破壊的変更により既存ユーザーの設定が動作しなくなる | 高 | 高 | CHANGELOGに明記、マイグレーションガイド提供 |
| R-002 | umaskの影響でパーミッション設定が期待通りにならない | 中 | 中 | os.Chmod()で明示的に設定 |
| R-003 | 一時ディレクトリ削除失敗でディスク容量を圧迫 | 中 | 低 | エラーログ出力、定期的なクリーンアップを推奨 |
| R-004 | dry-runモードでの変数展開が不完全 | 中 | 低 | 十分なテストケースを用意 |
| R-005 | Windows環境でのパーミッション設定が動作しない | 低 | 中 | ドキュメントに明記（Linux/Unixのみサポート） |

### 4.2 回避・軽減策

**R-001: 破壊的変更対策**
- CHANGELOGに「Breaking Changes」セクションを設ける
- マイグレーションガイドを提供（before/after例）
- サンプルファイルを新仕様に更新

**R-002: パーミッション設定対策**
- `os.MkdirTemp()` 実行後に必ず `os.Chmod(0700)` を実行
- 単体テストでパーミッションを検証

**R-003: 削除失敗対策**
- エラー時も処理を継続（ログ出力のみ）
- ユーザードキュメントに定期クリーンアップを推奨

---

## 5. スケジュール

### 5.1 フェーズ別スケジュール（目安）

| Phase | 期間（累計） | 主要マイルストーン |
|-------|------------|------------------|
| Phase 0 | 0.5日 | 事前準備完了 |
| Phase 1 | 1日 | 型定義変更完了、テスト成功 |
| Phase 2 | 2日 | TempDirManager実装完了、テスト成功 |
| Phase 3 | 3.5日 | 変数展開・ワークディレクトリ決定完了 |
| Phase 4 | 5日 | GroupExecutor統合完了、統合テスト成功 |
| Phase 5 | 5.5日 | ドキュメント・サンプル更新完了 |

**合計所要時間**: 約5.5日（44時間）

### 5.2 クリティカルパス

```
Phase 0 → Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5
                         ↓
                    Phase 3（並行可能）
```

**注意**: Phase 2とPhase 3は部分的に並行実装可能ですが、Phase 4では両方の成果物が必要です。

---

## 6. 完了基準

### 6.1 機能実装の完了基準

- [ ] 全ての型定義変更が完了している
- [ ] `TempDirManager` が実装され、テストが成功している
- [ ] `__runner_workdir` 変数が正しく動作している
- [ ] ワークディレクトリ決定ロジックが実装されている
- [ ] `--keep-temp-dirs` フラグが動作している
- [ ] dry-runモードで仮想パスが使用されている

### 6.2 テストの完了基準

- [ ] 全単体テストが成功している（カバレッジ > 80%）
- [ ] 全統合テストが成功している
- [ ] 全エラーケーステストが成功している
- [ ] パフォーマンステストが成功基準を満たしている

### 6.3 ドキュメントの完了基準

- [ ] ユーザーマニュアルが更新されている
- [ ] 全サンプルファイルが新仕様で動作する
- [ ] CHANGELOGに破壊的変更が記載されている
- [ ] READMEに新機能が説明されている
- [ ] マイグレーションガイドが提供されている

### 6.4 コードレビューの完了基準

- [ ] コードレビューが完了している
- [ ] 指摘事項が全て対応されている
- [ ] コーディング規約に準拠している

---

## 7. 実装チェックリスト（詳細仕様書からの転記）

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
  - [ ] `Create()`, `Cleanup()`, `Path()` メソッド
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

---

## 8. 参考資料

### 8.1 関連ドキュメント

- `01_requirements.md`: 要件定義書
- `02_architecture.md`: アーキテクチャ設計書
- `03_specification.md`: 詳細仕様書

### 8.2 参照コード

- `internal/runner/runnertypes/config.go`: 設定型定義
- `internal/runner/group_executor.go`: グループ実行ロジック
- `internal/runner/config/expansion.go`: 変数展開ロジック

### 8.3 外部ライブラリ

- `github.com/pelletier/go-toml/v2`: TOML パーサー
- `os.TempDir()`, `os.MkdirTemp()`: 一時ディレクトリAPI

---

## まとめ

本実装計画書は、タスク0034「作業ディレクトリ仕様の再設計」を5つのPhaseに分けて段階的に実装するための詳細な計画を提供します。

**重要なポイント**:
- **段階的な実装**: 依存関係を考慮し、5つのPhaseに分割
- **TDD**: 各Phaseで単体テストを先行実装
- **破壊的変更への対応**: CHANGELOGとマイグレーションガイドで明示
- **Dry-Runサポート**: 全機能でdry-runモードを考慮
- **リスク管理**: 想定されるリスクと対策を明確化

**推定期間**: 約5.5日（44時間）

**次のステップ**: Phase 0（事前準備）から開始してください。
