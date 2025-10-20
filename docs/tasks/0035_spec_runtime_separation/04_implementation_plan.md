# 実装計画書: 構造体分離（Spec/Runtime分離）

## 1. 概要

### 1.1 前提ドキュメント

本実装計画は以下のドキュメントに基づいて作成されています：

| ドキュメント | 参照目的 |
|----------|---------|
| `01_requirements.md` | 機能要件、非機能要件、セキュリティ要件の確認 |
| `02_architecture.md` | アーキテクチャ設計、コンポーネント設計の理解 |
| `03_specification.md` | 詳細仕様、API仕様、実装方法の確認 |

### 1.2 実装方針

- **段階的な実装**: 依存関係を考慮し、7つのPhaseに分けて実装
- **TDD (Test-Driven Development)**: 各Phaseで単体テストを先行実装
- **後方互換性**: TOMLファイルフォーマットは変更しない
- **レビュー可能性**: 各PhaseをPRに分割し、レビュー可能な単位にする

### 1.3 実装スコープ

#### 対象機能（In Scope）

- ✅ Spec層の型定義（`ConfigSpec`, `GlobalSpec`, `GroupSpec`, `CommandSpec`）
- ✅ Runtime層の型定義（`RuntimeGlobal`, `RuntimeGroup`, `RuntimeCommand`）
- ✅ 展開関数の実装（`ExpandGlobal`, `ExpandGroup`, `ExpandCommand`）
- ✅ TOMLローダーの更新（`ConfigSpec` を返すように変更）
- ✅ GroupExecutor、Executorの更新（Runtime型を使用）
- ✅ 全テストコードの更新
- ✅ ドキュメントの更新

#### 対象外（Out of Scope）

- ❌ 新機能の追加（構造変更のみに集中）
- ❌ TOMLファイルフォーマットの変更（後方互換性を維持）
- ❌ パフォーマンスの最適化（構造変更が主目的）

---

## 2. フェーズ別実装計画

### Phase 1: Spec層の型定義

**目的**: TOML由来の設定を表現するSpec層の型を定義する

**依存関係**: なし（最初のフェーズ）

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P1-1 | ConfigSpec 定義 | `internal/runner/runnertypes/spec.go` | `ConfigSpec` 型を新規定義 | 0.5h |
| P1-2 | GlobalSpec 定義 | `internal/runner/runnertypes/spec.go` | `GlobalSpec` 型を新規定義 | 1h |
| P1-3 | GroupSpec 定義 | `internal/runner/runnertypes/spec.go` | `GroupSpec` 型を新規定義 | 1h |
| P1-4 | CommandSpec 定義 | `internal/runner/runnertypes/spec.go` | `CommandSpec` 型を新規定義 | 1h |
| P1-5 | メソッド実装 | `internal/runner/runnertypes/spec.go` | `GetMaxRiskLevel()`, `HasUserGroupSpecification()` | 0.5h |
| P1-6 | 単体テスト（TDD） | `internal/runner/runnertypes/spec_test.go` | Spec層のTOMLパーステスト | 2h |

**詳細実装内容**:

#### P1-1 ~ P1-4: Spec型の定義

**新規ファイル**: `internal/runner/runnertypes/spec.go`

```go
package runnertypes

// ConfigSpec represents the root configuration structure loaded from TOML file.
type ConfigSpec struct {
    Version string      `toml:"version"`
    Global  GlobalSpec  `toml:"global"`
    Groups  []GroupSpec `toml:"groups"`
}

// GlobalSpec contains global configuration options loaded from TOML file.
type GlobalSpec struct {
    // Execution control
    Timeout           int    `toml:"timeout"`
    LogLevel          string `toml:"log_level"`
    SkipStandardPaths bool   `toml:"skip_standard_paths"`
    MaxOutputSize     int64  `toml:"max_output_size"`

    // Security
    VerifyFiles  []string `toml:"verify_files"`
    EnvAllowlist []string `toml:"env_allowlist"`

    // Variable definitions (raw values)
    Env     []string `toml:"env"`
    FromEnv []string `toml:"from_env"`
    Vars    []string `toml:"vars"`
}

// GroupSpec represents a command group configuration loaded from TOML file.
type GroupSpec struct {
    // Basic information
    Name        string `toml:"name"`
    Description string `toml:"description"`
    Priority    int    `toml:"priority"`

    // Resource management
    WorkDir string `toml:"workdir"`

    // Command definitions
    Commands []CommandSpec `toml:"commands"`

    // Security
    VerifyFiles  []string `toml:"verify_files"`
    EnvAllowlist []string `toml:"env_allowlist"`

    // Variable definitions (raw values)
    Env     []string `toml:"env"`
    FromEnv []string `toml:"from_env"`
    Vars    []string `toml:"vars"`
}

// CommandSpec represents a single command configuration loaded from TOML file.
type CommandSpec struct {
    // Basic information
    Name        string `toml:"name"`
    Description string `toml:"description"`

    // Command definition (raw values)
    Cmd  string   `toml:"cmd"`
    Args []string `toml:"args"`

    // Execution settings
    WorkDir      string `toml:"workdir"`
    Timeout      int    `toml:"timeout"`
    RunAsUser    string `toml:"run_as_user"`
    RunAsGroup   string `toml:"run_as_group"`
    MaxRiskLevel string `toml:"max_risk_level"`
    Output       string `toml:"output"`

    // Variable definitions (raw values)
    Env     []string `toml:"env"`
    FromEnv []string `toml:"from_env"`
    Vars    []string `toml:"vars"`
}
```

#### P1-5: メソッド実装

```go
// GetMaxRiskLevel parses and returns the maximum risk level for this command.
func (s *CommandSpec) GetMaxRiskLevel() (RiskLevel, error) {
    return ParseRiskLevel(s.MaxRiskLevel)
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified.
func (s *CommandSpec) HasUserGroupSpecification() bool {
    return s.RunAsUser != "" || s.RunAsGroup != ""
}
```

#### P1-6: 単体テスト（TDD）

**新規ファイル**: `internal/runner/runnertypes/spec_test.go`

テストケース:
- 正常系: 有効なTOMLのパース
- 異常系: 不正なTOMLのパース失敗
- エッジケース: 空のフィールド、デフォルト値

**成果物**:
- `spec.go`: Spec層の型定義
- `spec_test.go`: Spec層のテスト

**完了条件**:
- [x] すべてのSpec型が定義されている
- [x] TOMLタグが正しく設定されている
- [x] GoDocコメントが記述されている
- [x] 単体テストが成功している

---

### Phase 2: Runtime層の型定義

**目的**: 実行時展開結果を表現するRuntime層の型を定義する

**依存関係**: Phase 1完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P2-1 | RuntimeGlobal 定義 | `internal/runner/runnertypes/runtime.go` | `RuntimeGlobal` 型を新規定義 | 0.5h |
| P2-2 | RuntimeGroup 定義 | `internal/runner/runnertypes/runtime.go` | `RuntimeGroup` 型を新規定義 | 0.5h |
| P2-3 | RuntimeCommand 定義 | `internal/runner/runnertypes/runtime.go` | `RuntimeCommand` 型を新規定義 | 0.5h |
| P2-4 | 便利メソッド実装 | `internal/runner/runnertypes/runtime.go` | `Name()`, `RunAsUser()` など | 1h |
| P2-5 | 単体テスト | `internal/runner/runnertypes/runtime_test.go` | Runtime層のテスト | 1.5h |

**詳細実装内容**:

#### P2-1 ~ P2-3: Runtime型の定義

**新規ファイル**: `internal/runner/runnertypes/runtime.go`

```go
package runnertypes

// RuntimeGlobal represents the runtime-expanded global configuration.
type RuntimeGlobal struct {
    Spec *GlobalSpec // Reference to the original spec

    // Expanded variables
    ExpandedVerifyFiles []string
    ExpandedEnv         map[string]string
    ExpandedVars        map[string]string
}

// RuntimeGroup represents the runtime-expanded group configuration.
type RuntimeGroup struct {
    Spec *GroupSpec // Reference to the original spec

    // Expanded variables
    ExpandedVerifyFiles []string
    ExpandedEnv         map[string]string
    ExpandedVars        map[string]string

    // Runtime resources
    EffectiveWorkDir string

    // Expanded commands
    Commands []*RuntimeCommand
}

// RuntimeCommand represents the runtime-expanded command configuration.
type RuntimeCommand struct {
    Spec *CommandSpec // Reference to the original spec

    // Expanded command information
    ExpandedCmd  string
    ExpandedArgs []string
    ExpandedEnv  map[string]string
    ExpandedVars map[string]string

    // Runtime information
    EffectiveWorkDir string
    EffectiveTimeout int
}
```

#### P2-4: 便利メソッド実装

```go
// Convenience methods for RuntimeCommand

func (r *RuntimeCommand) Name() string {
    return r.Spec.Name
}

func (r *RuntimeCommand) RunAsUser() string {
    return r.Spec.RunAsUser
}

func (r *RuntimeCommand) RunAsGroup() string {
    return r.Spec.RunAsGroup
}

func (r *RuntimeCommand) Output() string {
    return r.Spec.Output
}

func (r *RuntimeCommand) GetMaxRiskLevel() (RiskLevel, error) {
    return r.Spec.GetMaxRiskLevel()
}

func (r *RuntimeCommand) HasUserGroupSpecification() bool {
    return r.Spec.HasUserGroupSpecification()
}
```

**成果物**:
- `runtime.go`: Runtime層の型定義
- `runtime_test.go`: Runtime層のテスト

**完了条件**:
- [x] すべてのRuntime型が定義されている
- [x] Specへの参照が正しく設定されている
- [x] 便利メソッドが実装されている
- [x] GoDocコメントが記述されている
- [x] 単体テストが成功している

---

### Phase 3: 展開関数の実装

**目的**: Spec → Runtime への展開ロジックを実装する

**依存関係**: Phase 1, 2完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P3-1 | ExpandGlobal 実装 | `internal/runner/config/expansion.go` | `ExpandGlobal()` 関数を実装 | 2h |
| P3-2 | ExpandGroup 実装 | `internal/runner/config/expansion.go` | `ExpandGroup()` 関数を実装 | 2h |
| P3-3 | ExpandCommand 実装 | `internal/runner/config/expansion.go` | `ExpandCommand()` 関数を実装 | 2h |
| P3-4 | 単体テスト（TDD） | `internal/runner/config/expansion_test.go` | 展開関数のテスト | 3h |

**詳細実装内容**:

#### P3-1: ExpandGlobal 実装

**ファイル**: `internal/runner/config/expansion.go`（既存ファイルに追加）

```go
// ExpandGlobal expands a GlobalSpec into a RuntimeGlobal.
func ExpandGlobal(spec *GlobalSpec) (*RuntimeGlobal, error) {
    runtime := &RuntimeGlobal{
        Spec:         spec,
        ExpandedVars: make(map[string]string),
        ExpandedEnv:  make(map[string]string),
    }

    // 1. FromEnv の処理
    if err := ProcessFromEnv(spec.FromEnv, runtime.ExpandedVars, nil); err != nil {
        return nil, fmt.Errorf("failed to process global from_env: %w", err)
    }

    // 2. Vars の処理
    if err := ProcessVars(spec.Vars, runtime.ExpandedVars); err != nil {
        return nil, fmt.Errorf("failed to process global vars: %w", err)
    }

    // 3. Env の展開
    for _, envPair := range spec.Env {
        key, value, err := parseKeyValue(envPair)
        if err != nil {
            return nil, fmt.Errorf("invalid global env format: %w", err)
        }
        expandedValue, err := ExpandString(value, runtime.ExpandedVars, "global", fmt.Sprintf("env[%s]", key))
        if err != nil {
            return nil, err
        }
        runtime.ExpandedEnv[key] = expandedValue
    }

    // 4. VerifyFiles の展開
    runtime.ExpandedVerifyFiles = make([]string, len(spec.VerifyFiles))
    for i, file := range spec.VerifyFiles {
        expandedFile, err := ExpandString(file, runtime.ExpandedVars, "global", fmt.Sprintf("verify_files[%d]", i))
        if err != nil {
            return nil, err
        }
        runtime.ExpandedVerifyFiles[i] = expandedFile
    }

    return runtime, nil
}
```

#### P3-2, P3-3: ExpandGroup, ExpandCommand 実装

（詳細仕様書 Section 4 を参照）

#### P3-4: 単体テスト

テストケース:
- 正常系: 変数展開の成功
- 異常系: 未定義変数のエラー
- エッジケース: 複雑な変数展開パターン

**成果物**:
- 更新された `expansion.go`
- `expansion_test.go`: 展開関数のテスト

**完了条件**:
- [x] `ExpandGlobal()` が実装されている
- [x] `ExpandGroup()` が実装されている
- [x] `ExpandCommand()` が実装されている
- [x] すべてのテストが成功している
- [x] エラーハンドリングが適切

---

### Phase 4: TOMLローダーの更新

**目的**: TOMLローダーを更新し、`ConfigSpec` を返すようにする

**依存関係**: Phase 1完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P4-1 | Loader更新 | `internal/runner/config/loader.go` | 戻り値を `*ConfigSpec` に変更 | 1h |
| P4-2 | テスト更新 | `internal/runner/config/loader_test.go` | 既存テストを新しい型に対応 | 1h |

**詳細実装内容**:

#### P4-1: Loader更新

**変更前**:

```go
func (l *DefaultLoader) Load(path string) (*runnertypes.Config, error)
```

**変更後**:

```go
func (l *DefaultLoader) Load(path string) (*runnertypes.ConfigSpec, error) {
    // パース処理は変更なし
    var config runnertypes.ConfigSpec
    if err := toml.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("failed to parse TOML: %w", err)
    }
    return &config, nil
}
```

**成果物**:
- 更新された `loader.go`
- 更新された `loader_test.go`

**完了条件**:
- [x] `Load()` が `*ConfigSpec` を返す
- [x] 既存のテストが成功している
- [x] TOMLファイルフォーマットの互換性が維持されている

---

### Phase 5: GroupExecutorの更新

**目的**: GroupExecutorを更新し、Runtime型を使用するようにする

**依存関係**: Phase 2, 3完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P5-1 | ExecuteGroup更新 | `internal/runner/group_executor.go` | `GroupSpec` を受け取るように変更 | 3h |
| P5-2 | テスト更新 | `internal/runner/group_executor_test.go` | 既存テストを新しい型に対応 | 2h |

**詳細実装内容**:

#### P5-1: ExecuteGroup更新

**変更前**:

```go
func (e *DefaultGroupExecutor) ExecuteGroup(ctx context.Context, group *runnertypes.CommandGroup) error
```

**変更後**:

```go
func (e *DefaultGroupExecutor) ExecuteGroup(ctx context.Context, groupSpec *runnertypes.GroupSpec) error {
    // 1. グループを展開
    runtimeGroup, err := config.ExpandGroup(groupSpec, e.globalVars)
    if err != nil {
        return fmt.Errorf("failed to expand group[%s]: %w", groupSpec.Name, err)
    }

    // 2. 各コマンドを展開・実行
    for i := range groupSpec.Commands {
        cmdSpec := &groupSpec.Commands[i]

        // コマンドを展開
        runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup.ExpandedVars, groupSpec.Name)
        if err != nil {
            return fmt.Errorf("failed to expand command[%s]: %w", cmdSpec.Name, err)
        }

        // EffectiveTimeout を設定
        runtimeCmd.EffectiveTimeout = resolveTimeout(runtimeCmd.Spec.Timeout, e.globalTimeout)

        // コマンドを実行
        if err := e.executor.Execute(ctx, runtimeCmd); err != nil {
            return err
        }
    }

    return nil
}
```

**成果物**:
- 更新された `group_executor.go`
- 更新された `group_executor_test.go`

**完了条件**:
- [x] `ExecuteGroup()` が `GroupSpec` を受け取る
- [x] 内部で `ExpandGroup()`, `ExpandCommand()` を呼び出す
- [ ] 既存のテストが成功している（一部のテストは skip_integration_tests タグで無効化中）

---

### Phase 6: Executorの更新

**目的**: CommandExecutorを更新し、`RuntimeCommand` を受け取るようにする

**依存関係**: Phase 2完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P6-1 | Execute更新 | `internal/runner/executor/command_executor.go` | `RuntimeCommand` を受け取るように変更 | 2h |
| P6-2 | テスト更新 | `internal/runner/executor/command_executor_test.go` | 既存テストを新しい型に対応 | 2h |

**詳細実装内容**:

#### P6-1: Execute更新

**変更前**:

```go
func (e *DefaultCommandExecutor) Execute(ctx context.Context, cmd *runnertypes.Command) error
```

**変更後**:

```go
func (e *DefaultCommandExecutor) Execute(ctx context.Context, cmd *runnertypes.RuntimeCommand) error {
    // 展開済みフィールドを使用
    execCmd := exec.CommandContext(ctx, cmd.ExpandedCmd, cmd.ExpandedArgs...)

    // Spec フィールドも参照可能
    e.logger.Infof("Executing command: %s", cmd.Name())

    // 環境変数を設定
    for k, v := range cmd.ExpandedEnv {
        execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, v))
    }

    return execCmd.Run()
}
```

**成果物**:
- 更新された `command_executor.go`
- 更新された `command_executor_test.go`

**完了条件**:
- [ ] `Execute()` が `RuntimeCommand` を受け取る
- [ ] 展開済みフィールドを使用している
- [ ] 既存のテストが成功している

---

### Phase 7: クリーンアップとドキュメント更新

**目的**: 古いコードを削除し、ドキュメントを更新する

**依存関係**: Phase 1 ~ 6完了後に開始

**作業項目**:

| ID | タスク | ファイル | 作業内容 | 所要時間 |
|----|-------|---------|---------|---------|
| P7-1 | 古い型定義の削除 | `internal/runner/runnertypes/config.go` | `Config`, `GlobalConfig` などを削除 | 1h |
| P7-2 | 全テスト実行 | - | すべてのテストが成功することを確認 | 0.5h |
| P7-3 | ベンチマークテスト | `internal/runner/config/expansion_bench_test.go` | パフォーマンス測定 | 1h |
| P7-4 | GoDocコメント | 全ファイル | すべての型・関数にコメントを記述 | 2h |
| P7-5 | README更新 | `docs/tasks/0035_spec_runtime_separation/README.md` | プロジェクトサマリーを作成 | 1h |

**詳細実装内容**:

#### P7-1: 古い型定義の削除

**削除対象**:
- `Config`
- `GlobalConfig`
- `CommandGroup`
- `Command`

**注意**: すべての参照が新しい型に更新されていることを確認してから削除

#### P7-3: ベンチマークテスト

```go
func BenchmarkExpandGlobal(b *testing.B) {
    spec := &GlobalSpec{
        Vars: []string{"VAR1=value1", "VAR2=value2"},
        Env:  []string{"PATH=%{VAR1}/bin"},
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = ExpandGlobal(spec)
    }
}
```

**成果物**:
- クリーンアップされたコードベース
- 完全なGoDocコメント
- README.md
- ベンチマーク結果

**完了条件**:
- [ ] 古い型定義が削除されている
- [ ] すべてのテストが成功している
- [ ] ベンチマークテストが許容範囲内
- [ ] GoDocコメントが完全
- [ ] READMEが作成されている

---

## 3. テスト計画

### 3.1 単体テスト

| テストID | テスト対象 | テストケース | 期待結果 |
|---------|-----------|------------|---------|
| UT-001 | ConfigSpec | TOMLパース（正常系） | パース成功 |
| UT-002 | ConfigSpec | TOMLパース（異常系） | エラー返却 |
| UT-003 | ExpandGlobal | 変数展開（正常系） | 正しく展開される |
| UT-004 | ExpandGlobal | 未定義変数参照 | エラー返却 |
| UT-005 | ExpandGroup | グローバル変数継承 | 正しく継承される |
| UT-006 | ExpandCommand | コマンド引数展開 | 正しく展開される |
| UT-007 | RuntimeCommand | 便利メソッド | Specフィールドが返される |

### 3.2 統合テスト

| テストID | テストシナリオ | 検証内容 |
|---------|-------------|---------|
| IT-001 | エンドツーエンド | TOMLロード → 展開 → 実行 |
| IT-002 | 既存サンプルファイル | すべてのサンプルが動作する |
| IT-003 | 複雑な変数展開 | 多段階の変数展開が成功する |

### 3.3 リグレッションテスト

| テストID | テスト内容 | 成功基準 |
|---------|-----------|---------|
| RT-001 | 既存テストの成功 | すべてのテストが成功 |
| RT-002 | 既存サンプルの動作 | すべてのサンプルが動作 |

### 3.4 パフォーマンステスト

| テストID | テスト内容 | 成功基準 |
|---------|-----------|---------|
| PT-001 | 展開処理のパフォーマンス | 既存比 ±10% 以内 |
| PT-002 | メモリ使用量 | 既存比 +30% 以内 |

---

## 4. リスク管理

### 4.1 リスク一覧

| リスクID | リスク内容 | 影響度 | 発生確率 | 対策 |
|---------|-----------|-------|---------|------|
| R-001 | 大規模リファクタリングによるデグレーション | 高 | 中 | 段階的な移行、徹底的なテスト |
| R-002 | レビューコストの増大 | 中 | 高 | PR の分割、詳細なコメント |
| R-003 | パフォーマンス劣化 | 中 | 低 | ベンチマークテストの実施 |
| R-004 | メモリ使用量の増加 | 低 | 中 | メモリプロファイリング |
| R-005 | 既存コードとの非互換性 | 高 | 低 | 統合テストの徹底 |

### 4.2 回避・軽減策

**R-001: デグレーション対策**
- 段階的な移行（7つのPhaseに分割）
- 各Phaseで徹底的なテスト
- 統合テストでエンドツーエンドを検証

**R-002: レビューコスト対策**
- 各PhaseをPRに分割（最大7つのPR）
- 詳細なコメントとドキュメント
- レビュアーへの事前説明

**R-003: パフォーマンス劣化対策**
- ベンチマークテストの実施
- 既存ロジックの再利用
- 必要に応じて最適化

---

## 5. スケジュール

### 5.1 フェーズ別スケジュール（目安）

| Phase | 期間（累計） | 主要マイルストーン |
|-------|------------|------------------|
| Phase 1 | 1日 | Spec層の型定義完了 |
| Phase 2 | 1.5日 | Runtime層の型定義完了 |
| Phase 3 | 3日 | 展開関数の実装完了 |
| Phase 4 | 3.5日 | TOMLローダーの更新完了 |
| Phase 5 | 5日 | GroupExecutorの更新完了 |
| Phase 6 | 6.5日 | Executorの更新完了 |
| Phase 7 | 7.5日 | クリーンアップ完了 |

**合計所要時間**: 約7.5日（60時間）

### 5.2 クリティカルパス

```
Phase 1 → Phase 2 → Phase 3 → Phase 5 → Phase 6 → Phase 7
              ↓
         Phase 4（並行可能）
```

**注意**: Phase 4 は Phase 1 完了後に並行実行可能ですが、Phase 5 では Phase 3, 4 の両方が必要です。

---

## 6. 完了基準

### 6.1 機能実装の完了基準

- [ ] すべてのSpec型が定義されている
- [ ] すべてのRuntime型が定義されている
- [ ] すべての展開関数が実装されている
- [ ] TOMLローダーが `ConfigSpec` を返す
- [ ] GroupExecutor が `RuntimeGroup` を使用する
- [ ] Executor が `RuntimeCommand` を使用する
- [ ] 古い型定義が削除されている

### 6.2 テストの完了基準

- [ ] すべての単体テストが成功している
- [ ] すべての統合テストが成功している
- [ ] すべてのリグレッションテストが成功している
- [ ] パフォーマンステストが許容範囲内
- [ ] コードカバレッジ > 80%

### 6.3 ドキュメントの完了基準

- [ ] すべての型にGoDocコメントがある
- [ ] すべての関数にGoDocコメントがある
- [ ] README.md が作成されている
- [ ] Task 0034 のドキュメントが更新されている（Phase 0）

### 6.4 コードレビューの完了基準

- [ ] すべてのPRがレビューされている
- [ ] 指摘事項が全て対応されている
- [ ] コーディング規約に準拠している

---

## 7. 実装チェックリスト

### Phase 1: Spec層の型定義
- [x] `ConfigSpec` を定義
- [x] `GlobalSpec` を定義
- [x] `GroupSpec` を定義
- [x] `CommandSpec` を定義
- [x] `GetMaxRiskLevel()`, `HasUserGroupSpecification()` を実装
- [x] Spec層のテストを実装

### Phase 2: Runtime層の型定義
- [x] `RuntimeGlobal` を定義
- [x] `RuntimeGroup` を定義
- [x] `RuntimeCommand` を定義
- [x] 便利メソッドを実装
- [x] Runtime層のテストを実装

### Phase 3: 展開関数の実装
- [x] `ExpandGlobal()` を実装
- [x] `ExpandGroup()` を実装
- [x] `ExpandCommand()` を実装
- [x] 展開関数のテストを実装

### Phase 4: TOMLローダーの更新
- [x] `Load()` を更新（`ConfigSpec` を返す）
- [x] テストを更新

### Phase 5: GroupExecutorの更新
- [x] `ExecuteGroup()` を更新（`GroupSpec` を受け取る）
- [ ] テストを更新（一部のテストは skip_integration_tests タグで無効化中）

### Phase 6: Executorの更新
- [ ] `Execute()` を更新（`RuntimeCommand` を受け取る）
- [ ] テストを更新

### Phase 7: クリーンアップとドキュメント
- [ ] 古い型定義を削除
- [ ] すべてのテストが成功することを確認
- [ ] ベンチマークテストを実施
- [ ] GoDocコメントを完成
- [ ] README.md を作成

### 追加作業
- [x] `ExpandGlobal()` に from_env 処理を実装（Phase 5 完了後）
- [x] `TestRunner_SecurityIntegration` の修正
- [x] テスト再有効化計画の作成（`test_reactivation_plan.md`）
- [x] `types_test.go` の再有効化

---

## 8. 次のステップ

本プロジェクト（Task 0035）完了後:

1. **Task 0034 のドキュメント更新** (Phase 0)
   - `02_architecture.md` を新しい構造体前提で書き直し
   - `03_specification.md` を新しい構造体前提で書き直し
   - `04_implementation_plan.md` Phase 1以降を新しい構造体前提で書き直し

2. **Task 0034 の実装再開** (Phase 1以降)
   - 作業ディレクトリ仕様の再設計を実装

3. **Task 0036: LogLevel 型の導入**
   - カスタム LogLevel 型の導入により、TOML パース時点でログレベルのバリデーションを実現
   - 早期エラー検出と型安全性の向上
   - 詳細: `docs/tasks/0036_loglevel_type/`

---

## まとめ

本実装計画書は、Task 0035「構造体分離（Spec/Runtime分離）」を7つのPhaseに分けて段階的に実装するための詳細な計画を提供します。

**重要なポイント**:
- **段階的な実装**: 依存関係を考慮し、7つのPhaseに分割
- **TDD**: 各Phaseで単体テストを先行実装
- **後方互換性**: TOMLファイルフォーマットは変更しない
- **レビュー可能性**: 各PhaseをPRに分割
- **徹底的なテスト**: リグレッション防止

**推定期間**: 約7.5日（60時間）

**次のステップ**: Phase 1（Spec層の型定義）から開始してください。
