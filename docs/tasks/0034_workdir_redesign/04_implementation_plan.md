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

### Phase 0: 前提条件の確認（Task 0035完了 - ✅ 完了）

**目的**: 構造体分離（Task 0035）の完了を待ち、その成果を本タスクに反映する

**依存関係**: Task 0035「構造体分離（Spec/Runtime分離）」の完了

**作業項目**:

| ID | タスク | 作業内容 | 所要時間 | 状態 |
|----|-------|---------|---------|------|
| P0-1 | Task 0035完了確認 | 構造体分離プロジェクトの完了を確認 | - | ✅ |
| P0-2 | アーキテクチャ設計書更新 | 02_architecture.md を新しい構造体前提で書き直し | 2h | ✅ |
| P0-3 | 詳細仕様書更新 | 03_specification.md を新しい構造体前提で書き直し | 2h | ✅ |
| P0-4 | 実装計画書更新 | Phase 1以降を新しい構造体前提で書き直し | 2h | ✅ |

**Task 0035 の成果**:

Task 0035 により、以下の Spec/Runtime 分離が完了しました:

**Spec 層** (`internal/runner/runnertypes/spec.go`):
- `ConfigSpec`: TOML ルート設定
- `GlobalSpec`: グローバル設定
- `GroupSpec`: グループ設定
- `CommandSpec`: コマンド設定

**Runtime 層** (`internal/runner/runnertypes/runtime.go`):
- `RuntimeGlobal`: 実行時グローバル設定（`ExpandedVars` を含む）
- `RuntimeGroup`: 実行時グループ設定（`ExpandedVars`, `Commands []*RuntimeCommand` を含む）
- `RuntimeCommand`: 実行時コマンド設定（`ExpandedCmd`, `ExpandedArgs`, `ExpandedEnv` を含む）

**変数展開の遅延評価**:
- TOMLロード時: `ConfigSpec` のみ生成、変数は未展開
- 実行時: `ExpandGlobal()`, `ExpandGroup()`, `ExpandCommand()` で段階的に展開

**成果物**:
- ✅ 更新された `02_architecture.md`（新しい Spec/Runtime 構造体を前提）
- ✅ 更新された `03_specification.md`（新しい Spec/Runtime 構造体を前提）
- ✅ 更新された `04_implementation_plan.md` Phase 1以降（新しい構造体を前提）

**完了条件**:
- [x] Task 0035 が完了している（Phase 1-9 完了）
- [x] アーキテクチャ設計書が新しい構造体前提で更新されている
- [x] 詳細仕様書が新しい構造体前提で更新されている
- [x] 実装計画書 Phase 1以降が新しい構造体前提で更新されている

**注記**:
- Task 0035 の詳細は `docs/tasks/0035_spec_runtime_separation/` を参照してください
- 古い型定義（`Config`, `GlobalConfig`, `CommandGroup`, `Command`）はすべて削除されました
- Task 0034 は Spec/Runtime 構造体を前提として実装します

---

### Phase 1: 型定義の変更（破壊的変更 - Task 0035 完了後）

**目的**: Spec/Runtime 構造体にワークディレクトリ機能を追加する

**依存関係**: Phase 0完了後に開始（Task 0035 完了済み）

**Task 0035 による変更点**:
- 型定義ファイルが `config.go` から `spec.go` と `runtime.go` に分離されました
- `GlobalConfig` → `GlobalSpec`, `CommandGroup` → `GroupSpec`, `Command` → `CommandSpec` に変更されました
- Runtime 層（`RuntimeGlobal`, `RuntimeGroup`, `RuntimeCommand`）が導入されました

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P1-1 | GlobalSpec変更 | `internal/runner/runnertypes/spec.go` | `WorkDir` フィールドを削除 | 0.5h |
| P1-2 | GroupSpec変更 | `internal/runner/runnertypes/spec.go` | `TempDir` フィールドを削除 | 0.5h |
| P1-3 | CommandSpec変更 | `internal/runner/runnertypes/spec.go` | `Dir` → `WorkDir` に名称変更 | 0.5h |
| P1-4 | RuntimeGroup拡張 | `internal/runner/runnertypes/runtime.go` | `EffectiveWorkDir` フィールドを追加 | 0.5h |
| P1-5 | RuntimeCommand拡張 | `internal/runner/runnertypes/runtime.go` | `EffectiveWorkDir` フィールドを追加 | 0.5h |
| P1-6 | 単体テスト作成 | `internal/runner/runnertypes/spec_test.go` | 型定義のテスト | 1h |
| P1-7 | 廃止フィールド検出テスト | `internal/runner/config/loader_test.go` | TOMLパーサーエラー検証 | 1h |

**詳細実装内容（Task 0035 完了後）**:

#### P1-1: GlobalSpec変更

```go
// 変更前（Task 0035 完了時点）
type GlobalSpec struct {
    WorkDir          string   `toml:"workdir"`  // 削除対象
    Timeout          int      `toml:"timeout"`
    // ... その他のフィールド
}

// 変更後（Task 0034 完了後）
type GlobalSpec struct {
    // WorkDir は削除
    Timeout          int      `toml:"timeout"`
    // ... その他のフィールド
}
```

#### P1-2: GroupSpec変更

```go
// 変更前（Task 0035 完了時点）
type GroupSpec struct {
    Name         string        `toml:"name"`
    TempDir      bool          `toml:"temp_dir"`  // 削除対象
    WorkDir      string        `toml:"workdir"`   // TOMLから読み込んだ生の値
    Commands     []CommandSpec `toml:"commands"`
    // ... その他のフィールド
}

// 変更後（Task 0034 完了後）
type GroupSpec struct {
    Name         string        `toml:"name"`
    // TempDir は削除
    WorkDir      string        `toml:"workdir"`   // TOMLから読み込んだ生の値
    Commands     []CommandSpec `toml:"commands"`
    // ... その他のフィールド
}
```

#### P1-3: CommandSpec変更

```go
// 変更前（Task 0035 完了時点）
type CommandSpec struct {
    Name         string   `toml:"name"`
    Dir          string   `toml:"dir"`  // 削除対象
    // ... その他のフィールド
}

// 変更後（Task 0034 完了後）
type CommandSpec struct {
    Name         string   `toml:"name"`
    WorkDir      string   `toml:"workdir"`  // 名称変更（旧: dir）
    // ... その他のフィールド
}
```

#### P1-4, P1-5: Runtime 構造体の拡張

```go
// RuntimeGroup への追加
type RuntimeGroup struct {
    Spec *GroupSpec  // 元の Spec への参照

    // 既存フィールド（Task 0035 で実装済み）
    ExpandedVerifyFiles []string
    ExpandedEnv         map[string]string
    ExpandedVars        map[string]string
    Commands            []*RuntimeCommand

    // Task 0034 で追加
    EffectiveWorkDir string  // 実行時に決定されたワークディレクトリ（一時または固定）
}

// RuntimeCommand への追加
type RuntimeCommand struct {
    Spec *CommandSpec  // 元の Spec への参照

    // 既存フィールド（Task 0035 で実装済み）
    ExpandedCmd      string
    ExpandedArgs     []string
    ExpandedEnv      map[string]string
    ExpandedVars     map[string]string
    EffectiveTimeout int

    // Task 0034 で追加
    EffectiveWorkDir string  // コマンドレベルで決定されたワークディレクトリ
}
```

**変更の詳細**:
- **Spec 層**: TOML由来の設定値のみを保持（`WorkDir` フィールドの削除・名称変更）
- **Runtime 層**: 実行時に決定される値を保持（`EffectiveWorkDir` フィールドの追加）
- Task 0035 の Spec/Runtime 分離パターンに準拠

**成果物**:
- 更新された `spec.go`（Spec 層の型定義）
- 更新された `runtime.go`（Runtime 層の型定義）
- 型定義のテストコード
- TOMLパーサーエラーの検証テスト

**完了条件**:
- [x] `GlobalSpec.WorkDir` が削除されている
- [x] `GroupSpec.TempDir` が削除されている
- [x] `CommandSpec.Dir` が `CommandSpec.WorkDir` に変更されている
- [x] `RuntimeGroup.EffectiveWorkDir` が追加されている
- [x] `RuntimeCommand.EffectiveWorkDir` が追加されている
- [x] 廃止フィールドを含むTOMLファイルの動作が確認されている（go-toml/v2は unknown field を無視）
- [x] 全テストが成功している（`make test` パス、`make lint` 0 issues）

**リスク**:
- **破壊的変更**: 既存のTOMLファイルがロードできなくなる
  - 対策: CHANGELOG.mdに明記し、サンプルファイルを更新

---

### Phase 2: TempDirManager実装（新規機能 - ✅ 完了）

**目的**: 一時ディレクトリのライフサイクル管理を実装する

**依存関係**: Phase 1完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 | 状態 |
|----|-------|---------|---------|---------|------|
| P2-1 | インターフェース定義 | `internal/runner/executor/tempdir_manager.go` | `TempDirManager` インターフェース定義 | 0.5h | ✅ |
| P2-2 | 標準実装 | `internal/runner/executor/tempdir_manager.go` | `DefaultTempDirManager` 実装 | 2h | ✅ |
| P2-3 | 単体テスト（通常モード） | `internal/runner/executor/tempdir_manager_test.go` | Create/Cleanup/Pathのテスト | 1.5h | ✅ |
| P2-4 | 単体テスト（dry-run） | `internal/runner/executor/tempdir_manager_test.go` | dry-runモードのテスト | 1h | ✅ |
| P2-5 | エラーケーステスト | `internal/runner/executor/tempdir_manager_test.go` | パーミッションエラー等のテスト | 1h | ✅ |

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
- [x] `TempDirManager` インターフェースが定義されている
- [x] `DefaultTempDirManager` が実装されている
- [x] 通常モードで一時ディレクトリが生成・削除できる
- [x] dry-runモードで仮想パスが生成される
- [x] パーミッションが厳密に0700に設定される
- [x] エラーケースが適切に処理される
- [x] 全テストが成功している

**リスク**:
- **パーミッション設定の失敗**: umaskの影響でパーミッションが期待通りにならない可能性
  - 対策: `os.Chmod()` で明示的に0700を設定

---

### Phase 3: ワークディレクトリ決定ロジック（Task 0035 完了後）

**目的**: `__runner_workdir` 変数の実装とワークディレクトリ決定ロジックの統合

**依存関係**: Phase 1, 2完了後に開始

**Task 0035 による変更点**:
- `buildVarsForCommand()`, `expandCommand()` は削除されました（`config.ExpandGroup/ExpandCommand` に統合）
- 変数展開は実行時に `ExpandGroup()`, `ExpandCommand()` で自動的に行われます
- Task 0034 では、ワークディレクトリの決定と `__runner_workdir` 変数の設定のみを実装します

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 | 状態 |
|----|-------|---------|---------|---------|------|
| P3-1 | 定数定義 | `internal/runner/variable/auto.go` | `AutoVarKeyWorkDir` 定数追加 | 0.5h | ✅ |
| P3-2 | resolveGroupWorkDir実装 | `internal/runner/group_executor.go` | グループワークディレクトリ決定ロジック（RuntimeGroup を受け取る） | 2h | ✅ |
| P3-3 | resolveCommandWorkDir実装 | `internal/runner/group_executor.go` | コマンドワークディレクトリ決定ロジック（RuntimeCommand を受け取る） | 1h | ✅ |
| P3-4 | ワークディレクトリ決定テスト | `internal/runner/group_executor_test.go` | resolveGroupWorkDir、resolveCommandWorkDirのテスト | 1h | ✅ |
| P3-5 | `__runner_workdir`展開テスト | `internal/runner/group_executor_test.go` | `__runner_workdir`変数の展開テスト（TestExecuteGroup_RunnerWorkdirExpansion を作成） | 1h | ✅ |

**作業完了状況**:
- [x] P3-1: `AutoVarKeyWorkDir` 定数を `internal/runner/variable/auto.go` に追加
- [x] P3-2: `resolveGroupWorkDir()` を実装（TempDirManager統合、dry-run対応）
- [x] P3-3: `resolveCommandWorkDir()` を実装（変数展開対応）
- [x] P3-4: `TestResolveGroupWorkDir` および `TestResolveCommandWorkDir` を実装（各4テストケース）
- [x] P3-5: `__runner_workdir` 変数の展開を `ExecuteGroup()` で実装し、テストで確認
- [x] 全テストおよびlintチェックが成功

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

#### P3-2: resolveGroupWorkDir実装（Task 0035 完了後）

**注意**: Task 0035 により、引数が `*runnertypes.CommandGroup` から `*runnertypes.RuntimeGroup` に変更されました。

```go
// internal/runner/group_executor.go

// resolveGroupWorkDir: グループのワークディレクトリを決定
// 戻り値: (workdir, tempDirManager, error)
//
// Task 0035 による変更点:
//   - 引数: *runnertypes.CommandGroup → *runnertypes.RuntimeGroup
//   - runtimeGroup.Spec.WorkDir で TOML の生の値にアクセス
//   - runtimeGroup.ExpandedVars で展開済み変数にアクセス
func (e *DefaultGroupExecutor) resolveGroupWorkDir(
    runtimeGroup *runnertypes.RuntimeGroup,
) (string, executor.TempDirManager, error) {
    // グループレベル WorkDir が指定されている?
    if runtimeGroup.Spec.WorkDir != "" {
        // 変数展開（注意: __runner_workdir はまだ未定義）
        expandedWorkDir, err := config.ExpandString(
            runtimeGroup.Spec.WorkDir,
            runtimeGroup.ExpandedVars,  // Task 0035 で展開済み
            fmt.Sprintf("group[%s]", runtimeGroup.Spec.Name),
            "workdir",
        )
        if err != nil {
            return "", nil, fmt.Errorf("failed to expand group workdir: %w", err)
        }

        // パス検証（セキュリティチェックのみ: 絶対パス、パストラバーサル禁止）
        if err := validatePath(expandedWorkDir); err != nil {
            return "", nil, err
        }

        return expandedWorkDir, nil, nil
    }

    // 一時ディレクトリマネージャーを作成
    tempDirMgr := executor.NewTempDirManager(e.logger, runtimeGroup.Spec.Name, e.isDryRun)

    // 一時ディレクトリを生成
    tempDir, err := tempDirMgr.Create()
    if err != nil {
        return "", nil, err
    }

    return tempDir, tempDirMgr, nil
}
```

#### P3-3: resolveCommandWorkDir実装（Task 0035 完了後）

**注意**: Task 0035 により、引数が変更されました。

```go
// internal/runner/executor/command_executor.go

// resolveCommandWorkDir: コマンドのワークディレクトリを決定
// 優先度: RuntimeCommand.EffectiveWorkDir > RuntimeGroup.EffectiveWorkDir
//
// Task 0035 による変更点:
//   - 引数: (*runnertypes.Command, *runnertypes.CommandGroup) → (*runnertypes.RuntimeCommand, *runnertypes.RuntimeGroup)
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
    return runtimeGroup.EffectiveWorkDir
}
```

**Task 0035 により削除された関数**:
- `buildVarsForCommand()`: `config.ExpandGroup/ExpandCommand` に統合されました
- `expandCommand()`: `config.ExpandCommand()` に統合されました

これらの機能は Task 0035 で実装済みのため、Task 0034 では実装不要です。

#### P3-4, P3-5: テスト実装

**テスト対象**: `resolveGroupWorkDir()`, `resolveCommandWorkDir()`, `__runner_workdir`の展開

**テストケース**:

| テストID | テストケース | 検証内容 |
|---------|------------|---------|
| T006 | resolveGroupWorkDir: workdir未指定 | 一時ディレクトリが生成される |
| T007 | resolveGroupWorkDir: workdir指定 | 固定パスが使用される |
| T008 | resolveGroupWorkDir: 変数展開 | runtimeGroup.Spec.WorkDirの変数が正しく展開される |
| T009 | resolveGroupWorkDir: dry-runモード | 仮想パスが生成される |
| T010 | resolveCommandWorkDir: コマンドworkdir優先 | runtimeCmd.EffectiveWorkDirが優先される |
| T011 | resolveCommandWorkDir: グループworkdir使用 | runtimeCmd.EffectiveWorkDirが空の場合、runtimeGroup.EffectiveWorkDirが使用される |
| T012 | `__runner_workdir`展開 | コマンド引数中の`%{__runner_workdir}`が正しく展開される（`ExpandCommand()`経由） |

**成果物**:
- `AutoVarKeyWorkDir` 定数
- `resolveGroupWorkDir()` 関数（RuntimeGroup を受け取る）
- `resolveCommandWorkDir()` 関数（RuntimeCommand を受け取る）
- 単体テスト一式

**完了条件**:
- [x] `AutoVarKeyWorkDir` 定数が定義されている
- [x] グループワークディレクトリ決定ロジックが実装されている（RuntimeGroup を受け取る）
- [x] コマンドワークディレクトリ決定ロジックが実装されている（RuntimeCommand を受け取る）
- [x] `__runner_workdir` が正しく展開される（`config.ExpandCommand()` を通して）
- [x] 優先順位が正しく動作する
- [x] 全テストが成功している

---

### Phase 4: GroupExecutor統合（部分完了 - P4-1〜P4-4完了、P4-5は別ブランチで作成中）

**目的**: GroupExecutorに一時ディレクトリ管理とワークディレクトリ決定を統合する

**依存関係**: Phase 2, 3完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 | 状態 |
|----|-------|---------|---------|---------|------|
| P4-1 | ExecuteGroup更新 | `internal/runner/group_executor.go` | ワークディレクトリ決定・設定を統合 | 2h | ✅ |
| P4-2 | クリーンアップロジック | `internal/runner/group_executor.go` | `defer` でのクリーンアップ実装 | 1h | ✅ |
| P4-3 | keep-temp-dirsフラグ | `cmd/runner/main.go` | コマンドラインフラグ追加 | 0.5h | ✅ |
| P4-4 | Runnerへのフラグ伝播 | `internal/runner/runner.go` | フラグをGroupExecutorに渡す | 0.5h | ✅ |
| P4-5 | 統合テスト | `cmd/runner/integration_test.go` | エンドツーエンドテスト | 3h | 🔄 |

**注記**: P4-5（統合テスト）は別ブランチで作成中のため、マージ後に再検討予定。

**詳細実装内容**:

#### P4-1: ExecuteGroup更新（Task 0035 完了後）

**注意**: Task 0035 により、変数展開ロジックが大幅に変更されました。

```go
// internal/runner/group_executor.go

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

    // ステップ3: 一時ディレクトリの場合、クリーンアップを登録
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
    runtimeGroup.EffectiveWorkDir = workDir

    // (2) グループレベル変数に __runner_workdir を設定
    //     コマンドレベルの変数展開で参照可能
    runtimeGroup.ExpandedVars[variable.AutoVarPrefix + variable.AutoVarKeyWorkDir] = workDir

    // ステップ5: コマンド実行ループ（Task 0035 で実装済み + Task 0034 で拡張）
    for i := range groupSpec.Commands {
        cmdSpec := &groupSpec.Commands[i]

        // ステップ5-1: コマンド変数を展開（Task 0035 で実装済み）
        runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup.ExpandedVars, runtimeGroup.Spec.Name)
        if err != nil {
            return fmt.Errorf("failed to expand command '%s': %w", cmdSpec.Name, err)
        }

        // ステップ5-2: コマンドレベルのワークディレクトリを決定（Task 0034 で実装）
        runtimeCmd.EffectiveWorkDir = e.resolveCommandWorkDir(runtimeCmd, runtimeGroup)

        // ステップ5-3: コマンド実行
        err = e.executor.Execute(ctx, runtimeCmd)
        if err != nil {
            return fmt.Errorf("command '%s' failed: %w", cmdSpec.Name, err)
        }
    }

    return nil
}
```

**Task 0035 による変更点**:
- 引数: `*runnertypes.CommandGroup` → `*runnertypes.GroupSpec`
- グループ変数展開: `ExpandGroup()` を使用して `RuntimeGroup` を生成
- コマンド変数展開: `ExpandCommand()` を使用して `RuntimeCommand` を生成
- `buildVarsForCommand()`, `expandCommand()` は削除（`config.ExpandGroup/ExpandCommand` に統合）

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
- [x] `ExecuteGroup()` がワークディレクトリを決定・設定している
- [x] 一時ディレクトリが自動的にクリーンアップされる
- [x] `--keep-temp-dirs` フラグが動作する
- [x] dry-runモードで仮想パスが使用される
- [x] エラー時もクリーンアップが実行される
- [x] 全テストが成功している

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
- [ ] ユーザーマニュアルが新仕様に更新されている (未実装)
- [ ] 全サンプルファイルが新仕様で動作する (未実装)
- [ ] CHANGELOGに破壊的変更が記載されている (未実装)
- [ ] READMEに新機能が説明されている (未実装)
- [ ] マイグレーションガイドが提供されている (未実装)

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

- [x] 全ての型定義変更が完了している
- [x] `TempDirManager` が実装され、テストが成功している
- [x] `__runner_workdir` 変数が正しく動作している
- [x] ワークディレクトリ決定ロジックが実装されている
- [x] `--keep-temp-dirs` フラグが動作している
- [x] dry-runモードで仮想パスが使用されている

### 6.2 テストの完了基準

- [x] 全単体テストが成功している（カバレッジ > 80%）
- [ ] 全統合テストが成功している (P4-5は別ブランチで作成中)
- [x] 全エラーケーステストが成功している
- [x] パフォーマンステストが成功基準を満たしている

### 6.3 ドキュメントの完了基準

- [ ] ユーザーマニュアルが更新されている (未実装)
- [ ] 全サンプルファイルが新仕様で動作する (未実装)
- [ ] CHANGELOGに破壊的変更が記載されている (未実装)
- [ ] READMEに新機能が説明されている (未実装)
- [ ] マイグレーションガイドが提供されている (未実装)

### 6.4 コードレビューの完了基準

- [ ] コードレビューが完了している
- [ ] 指摘事項が全て対応されている
- [ ] コーディング規約に準拠している

---

### 7.2 実装チェックリスト（詳細仕様書からの転記）

### Phase 1: 型定義
- [x] `Global.WorkDir` を削除
- [x] `Group.TempDir` を削除
- [x] `RuntimeGroup.EffectiveWorkDir` フィールドを追加（実行時ワークディレクトリ）
- [x] `Command.Dir` → `Command.WorkDir` に変更
- [x] `RuntimeCommand.EffectiveWorkDir` フィールドを追加

### Phase 2: 一時ディレクトリ機能
- [x] `TempDirManager` インターフェース定義
- [x] `DefaultTempDirManager` 実装
  - [x] `isDryRun` フラグのサポート
  - [x] `Create()`, `Cleanup()`, `Path()` メソッド
  - [x] dry-runモードでのログ出力（"[DRY-RUN]" プレフィックス）
- [x] `--keep-temp-dirs` フラグを Runner に追加
- [x] Runner から GroupExecutor へ `keepTempDirs` と `isDryRun` を渡す
- [x] `defer` で条件付きクリーンアップ登録（`if !keepTempDirs { mgr.Cleanup() }`）

### Phase 3: 変数展開
- [x] `AutoVarKeyWorkDir` 定数を追加（`internal/runner/variable`）
- [x] `GroupExecutor.ExecuteGroup()` で以下を設定:
  - [x] `runtimeGroup.EffectiveWorkDir` に展開済みワークディレクトリを設定
  - [x] `runtimeGroup.ExpandedVars["__runner_workdir"]` に同じ値を設定
- [x] `resolveGroupWorkDir()` を実装
- [x] `resolveCommandWorkDir()` を実装

### Phase 4: テスト
- [x] 単体テスト実装
- [x] 統合テスト実装
  - [x] IT-001: グループ実行（一時ディレクトリ）自動生成・自動削除 (`TestIntegration_TempDirHandling`)
  - [x] IT-002: --keep-temp-dirsフラグ (`TestIntegration_TempDirHandling`)
  - [x] IT-003: エラー時のクリーンアップ (`TestIntegration_ErrorCleanup`)
  - [x] IT-004: 複数グループ実行 (`TestIntegration_MultipleGroups`)
  - [x] IT-005: dry-runモード (`TestIntegration_DryRunWithTempDir`)
  - [x] IT-006: 固定ディレクトリ使用 (`TestIntegration_TempDirHandling`)
  - [x] IT-007: コマンドレベルworkdir (`TestIntegration_CommandLevelWorkdir`)
- [x] エラーケーステスト

### Phase 5: ドキュメント
- [ ] ユーザードキュメント更新 (未実装)
- [ ] サンプルファイル更新 (未実装)
- [ ] CHANGELOG 更新 (未実装)

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
