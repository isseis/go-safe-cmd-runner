# 詳細仕様書: verify_files フィールド環境変数展開機能

## 0. 既存機能活用方針

この実装では、重複開発を避け既存の環境変数展開インフラを最大限活用します：

- **Filter クラス**: システム環境変数の取得、allowlist 決定、継承モード判定
  - `ParseSystemEnvironment()`: システム環境変数をマップとして取得（エクスポート済み）
  - `ResolveAllowlistConfiguration()`: グループの allowlist 設定を解決（**エクスポートが必要**）
- **VariableExpander クラス**: 環境変数展開エンジン、循環参照検出、セキュリティ検証
  - `ExpandString()`: 文字列中の環境変数を展開（エクスポート済み）
- **既存エラー型**: 環境変数関連エラー（ErrVariableNotAllowed、ErrCircularReference 等）

**アーキテクチャ上の決定**:
- `filter.parseSystemEnvironment` は既に `Filter.ParseSystemEnvironment()` として公開されています
- `filter.resolveAllowlistConfiguration` は現在プライベートです。config パッケージから使用するため、`Filter.ResolveAllowlistConfiguration()` として公開する必要があります
  - この変更は environment パッケージ内で行い、メソッド名を大文字で始めるだけの簡単な作業です
  - 既存の呼び出し元（environment パッケージ内）も更新が必要です

これにより**実装工数を1日削減**し、**実証済みセキュリティ機能を継承**できます。

## 1. 実装詳細仕様

### 1.1 パッケージ構成詳細

```
# 既存コンポーネント（再利用）
internal/runner/environment/filter.go       # Filter を再利用
                                             # ResolveAllowlistConfiguration のエクスポートが必要
internal/runner/environment/processor.go    # VariableExpander を再利用

# 拡張対象コンポーネント
internal/runner/runnertypes/config.go       # GlobalConfig/CommandGroup 拡張
internal/runner/config/expansion.go         # verify_files 展開ロジック追加

# 更新対象コンポーネント
internal/verification/manager.go            # ExpandedVerifyFiles の使用
```

### 1.2 型定義とインターフェース

#### 1.2.1 GlobalConfig 構造体の拡張

```go
// internal/runner/runnertypes/config.go

type GlobalConfig struct {
    Timeout           int      `toml:"timeout"`
    WorkDir           string   `toml:"workdir"`
    LogLevel          string   `toml:"log_level"`
    VerifyFiles       []string `toml:"verify_files"`        // 既存フィールド
    SkipStandardPaths bool     `toml:"skip_standard_paths"`
    EnvAllowlist      []string `toml:"env_allowlist"`
    MaxOutputSize     int64    `toml:"max_output_size"`

    // ExpandedVerifyFiles contains verify_files with environment variables expanded.
    // It is populated during configuration loading (Phase 1) and used during
    // verification (Phase 2) to avoid re-expanding VerifyFiles for each verification.
    // The toml:"-" tag prevents this field from being set via TOML configuration.
    ExpandedVerifyFiles []string `toml:"-"`
}
```

#### 1.2.2 CommandGroup 構造体の拡張

```go
// internal/runner/runnertypes/config.go

type CommandGroup struct {
    Name        string `toml:"name"`
    Description string `toml:"description"`
    Priority    int    `toml:"priority"`

    TempDir bool   `toml:"temp_dir"`
    WorkDir string `toml:"workdir"`

    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`  // 既存フィールド
    EnvAllowlist []string  `toml:"env_allowlist"`

    // ExpandedVerifyFiles contains verify_files with environment variables expanded.
    // It is populated during configuration loading (Phase 1) and used during
    // verification (Phase 2) to avoid re-expanding VerifyFiles for each verification.
    // The toml:"-" tag prevents this field from being set via TOML configuration.
    ExpandedVerifyFiles []string `toml:"-"`
}
```

### 1.3 環境変数展開の実装

#### 1.3.1 グローバル verify_files の展開（既存機能活用）

```go
// internal/runner/config/expansion.go

// ExpandGlobalVerifyFiles expands environment variables in global verify_files.
// Uses existing Filter.ParseSystemEnvironment() and VariableExpander.ExpandString().
// Returns VerifyFilesExpansionError on failure, which wraps the underlying cause.
func ExpandGlobalVerifyFiles(
    global *runnertypes.GlobalConfig,
    filter *environment.Filter,
    expander *environment.VariableExpander,
) error {
    if global == nil {
        return ErrNilConfig
    }

    // Handle empty verify_files
    if len(global.VerifyFiles) == 0 {
        global.ExpandedVerifyFiles = []string{}
        return nil
    }

    // Use existing Filter.ParseSystemEnvironment() for system environment map
    // This is equivalent to buildSystemEnvironmentMap() but reuses proven logic
    systemEnv := filter.ParseSystemEnvironment(nil) // nil predicate = get all variables

    // Expand all paths using existing VariableExpander.ExpandString()
    expanded := make([]string, 0, len(global.VerifyFiles))
    for i, path := range global.VerifyFiles {
        expandedPath, err := expander.ExpandString(
            path,
            systemEnv,
            global.EnvAllowlist,
            "global",
            make(map[string]bool),
        )
        if err != nil {
            return &VerifyFilesExpansionError{
                Level:     "global",
                Index:     i,
                Path:      path,
                Cause:     err,
                Allowlist: global.EnvAllowlist,
            }
        }
        expanded = append(expanded, expandedPath)
    }

    global.ExpandedVerifyFiles = expanded
    return nil
}
```

#### 1.3.2 グループ verify_files の展開（既存機能活用）

```go
// internal/runner/config/expansion.go

// ExpandGroupVerifyFiles expands environment variables in group verify_files.
// Uses existing Filter.ResolveAllowlistConfiguration() and VariableExpander.ExpandString().
// Returns VerifyFilesExpansionError on failure, which wraps the underlying cause.
func ExpandGroupVerifyFiles(
    group *runnertypes.CommandGroup,
    global *runnertypes.GlobalConfig,
    filter *environment.Filter,
    expander *environment.VariableExpander,
) error {
    if group == nil {
        return ErrNilConfig
    }

    // Handle empty verify_files
    if len(group.VerifyFiles) == 0 {
        group.ExpandedVerifyFiles = []string{}
        return nil
    }

    // Use existing Filter.ParseSystemEnvironment() for system environment
    // verify_files expansion only uses system environment variables
    systemEnv := filter.ParseSystemEnvironment(nil) // nil predicate = get all variables

    // Use existing Filter.ResolveAllowlistConfiguration() for allowlist determination
    resolution := filter.ResolveAllowlistConfiguration(group.EnvAllowlist, group.Name)
    allowlist := resolution.EffectiveList

    // Expand all paths using existing VariableExpander.ExpandString()
    expanded := make([]string, 0, len(group.VerifyFiles))
    for i, path := range group.VerifyFiles {
        expandedPath, err := expander.ExpandString(
            path,
            systemEnv,
            allowlist,
            group.Name,
            make(map[string]bool),
        )
        if err != nil {
            return &VerifyFilesExpansionError{
                Level:     group.Name,
                Index:     i,
                Path:      path,
                Cause:     err,
                Allowlist: allowlist,
            }
        }
        expanded = append(expanded, expandedPath)
    }

    group.ExpandedVerifyFiles = expanded
    return nil
}
```

#### 1.3.3 Config Parser への統合（既存機能活用）

```go
// internal/runner/config/loader.go (既存ファイル)

// LoadConfig loads and validates configuration from a TOML file
// Uses existing Filter and VariableExpander for verify_files expansion
func LoadConfig(configPath string) (*runnertypes.Config, error) {
    // Load TOML file
    config, err := loadTOMLFile(configPath)
    if err != nil {
        return nil, err
    }

    // Create Filter and VariableExpander using existing infrastructure
    filter := environment.NewFilter(config.Global.EnvAllowlist)
    expander := environment.NewVariableExpander(filter)

    return processConfig(config, filter, expander)
}

// processConfig processes the configuration by expanding variables.
// Uses existing Filter and VariableExpander for consistency with command variable expansion.
func processConfig(config *runnertypes.Config, filter *environment.Filter, expander *environment.VariableExpander) (*runnertypes.Config, error) {
    // Expand global verify_files using existing infrastructure
    if err := ExpandGlobalVerifyFiles(&config.Global, filter, expander); err != nil {
        return nil, fmt.Errorf("failed to expand global verify_files: %w", err)
    }

    // Expand group verify_files and command variables
    for i := range config.Groups {
        group := &config.Groups[i]

        // Expand verify_files for this group using existing infrastructure
        if err := ExpandGroupVerifyFiles(group, &config.Global, filter, expander); err != nil {
            return nil, fmt.Errorf("failed to expand verify_files for group %s: %w", group.Name, err)
        }

        // Expand command variables (existing logic - unchanged)
        for j := range group.Commands {
            cmd := &group.Commands[j]
            if err := ExpandCommandVariables(cmd, group, &config.Global, expander); err != nil {
                return nil, fmt.Errorf("failed to expand variables for command %s in group %s: %w", cmd.Name, group.Name, err)
            }
        }
    }

    return config, nil
}
```

**注**: テスト専用ヘルパー関数（LoadConfigFromString）は不要です。既存の `Loader.LoadConfig([]byte)` メソッドが十分な機能を提供しており、以下の利点があります：

- デフォルト値の自動設定（timeout, workdir, log_level, max_output_size）
- workdir の厳密な検証（絶対パス、相対パス成分のチェック）
- 環境変数の予約プレフィックス検証
- verify_files の自動展開（processConfig による）

テストでは以下のように使用します：

```go
// テストでの使用例
loader := config.NewLoader()
cfg, err := loader.LoadConfig([]byte(tomlContent))
require.NoError(t, err)

// cfg.Global.ExpandedVerifyFiles と cfg.Groups[i].ExpandedVerifyFiles が
// 自動的に展開されている
```

### 1.4 Verification Manager の更新

#### 1.4.1 VerifyGlobalFiles の更新

```go
// internal/verification/manager.go

// VerifyGlobalFiles verifies the integrity of global files
func (m *Manager) VerifyGlobalFiles(globalConfig *runnertypes.GlobalConfig) (*Result, error) {
    if globalConfig == nil {
        return nil, ErrConfigNil
    }

    // Ensure hash directory is validated
    if err := m.ensureHashDirectoryValidated(); err != nil {
        return nil, err
    }

    result := &Result{
        // 変更: ExpandedVerifyFiles を使用
        TotalFiles:   len(globalConfig.ExpandedVerifyFiles),
        FailedFiles:  []string{},
        SkippedFiles: []string{},
    }

    start := time.Now()
    defer func() {
        result.Duration = time.Since(start)
    }()

    // Update PathResolver with skip_standard_paths setting
    if m.pathResolver != nil {
        m.pathResolver.skipStandardPaths = globalConfig.SkipStandardPaths
    }

    // 変更: ExpandedVerifyFiles を使用
    for _, filePath := range globalConfig.ExpandedVerifyFiles {
        // Check if file should be skipped
        if m.shouldSkipVerification(filePath) {
            result.SkippedFiles = append(result.SkippedFiles, filePath)
            slog.Info("Skipping global file verification for standard system path",
                "file", filePath)
            continue
        }

        // Verify file hash (try normal verification first, then with privileges if needed)
        if err := m.verifyFileWithFallback(filePath); err != nil {
            result.FailedFiles = append(result.FailedFiles, filePath)
            slog.Error("Global file verification failed",
                "file", filePath,
                "error", err)
        } else {
            result.VerifiedFiles++
        }
    }

    if len(result.FailedFiles) > 0 {
        slog.Error("CRITICAL: Global file verification failed - program will terminate",
            "failed_files", result.FailedFiles,
            "verified_files", result.VerifiedFiles,
            "total_files", result.TotalFiles)
        return result, &VerificationError{
            Op:      "global",
            Details: result.FailedFiles,
            Err:     ErrGlobalVerificationFailed,
        }
    }

    return result, nil
}
```

#### 1.4.2 collectVerificationFiles の更新

```go
// internal/verification/manager.go

// collectVerificationFiles collects all files to verify for a group
func (m *Manager) collectVerificationFiles(groupConfig *runnertypes.CommandGroup) []string {
    if groupConfig == nil {
        return []string{}
    }

    // 変更: ExpandedVerifyFiles を使用
    allFiles := make([]string, 0, len(groupConfig.ExpandedVerifyFiles)+len(groupConfig.Commands))

    // Add explicit files (変更: ExpandedVerifyFiles を使用)
    allFiles = append(allFiles, groupConfig.ExpandedVerifyFiles...)

    // Add command files
    if m.pathResolver != nil {
        for _, command := range groupConfig.Commands {
            resolvedPath, err := m.pathResolver.ResolvePath(command.ExpandedCmd)
            if err != nil {
                slog.Warn("Failed to resolve command path",
                    "group", groupConfig.Name,
                    "command", command.ExpandedCmd,
                    "error", err.Error())
                continue
            }
            allFiles = append(allFiles, resolvedPath)
        }
    }

    // Remove duplicates
    return removeDuplicates(allFiles)
}
```

### 1.5 エラーハンドリング

#### 1.5.1 設計判断（ADR）

**背景**: 環境変数展開時のエラーには複数のレイヤーが存在します：

1. 最上位: グローバル/グループレベルでの展開失敗
2. 中位: 個別パスの展開失敗（インデックス、パス、allowlist の情報）
3. 下位: 根本原因（allowlist 違反、未定義変数、循環参照など）

**決定**: カスタムエラー型 `VerifyFilesExpansionError` を導入し、エラーチェーンを保持する設計を採用します。

**根拠**:

- `fmt.Errorf` による単純なラッピングでは、`errors.Is()` / `errors.As()` で元のエラー型を判定できない
- エラーメッセージ文字列マッチングは脆弱で保守性が低い
- デバッグ時に詳細なコンテキスト情報（インデックス、パス、allowlist）が必要

**利点**:

1. **型安全性**: エラー文字列ではなく型で判定
2. **保守性**: エラーメッセージ変更の影響を受けない
3. **デバッグ性**: エラーの詳細情報にアクセス可能
4. **拡張性**: 将来的なエラー処理の拡張が容易

#### 1.5.2 エラー種別

```go
// internal/runner/config/expansion.go

// Sentinel errors for verify_files expansion
var (
    // ErrGlobalVerifyFilesExpansionFailed indicates global verify_files expansion failed
    ErrGlobalVerifyFilesExpansionFailed = errors.New("global verify_files expansion failed")

    // ErrGroupVerifyFilesExpansionFailed indicates group verify_files expansion failed
    ErrGroupVerifyFilesExpansionFailed = errors.New("group verify_files expansion failed")

    // ErrNilConfig indicates a nil config was provided
    ErrNilConfig = errors.New("config is nil")
)
```

#### 1.5.3 エラーコンテキスト

```go
// VerifyFilesExpansionError represents an error that occurred during verify_files expansion.
// It wraps the underlying error while preserving the error chain for errors.Is() and errors.As().
type VerifyFilesExpansionError struct {
    Level     string   // "global" or group name
    Index     int      // verify_files array index
    Path      string   // path being expanded
    Cause     error    // root cause error
    Allowlist []string // applied allowlist
}

// Error returns the error message with full context information
func (e *VerifyFilesExpansionError) Error() string {
    return fmt.Sprintf(
        "failed to expand verify_files[%d] (%s) at %s level: %v (allowlist: %v)",
        e.Index,
        e.Path,
        e.Level,
        e.Cause,
        e.Allowlist,
    )
}

// Unwrap returns the underlying cause error, enabling errors.Is() and errors.As() to work correctly
func (e *VerifyFilesExpansionError) Unwrap() error {
    return e.Cause
}

// Is enables comparison with sentinel errors like ErrGlobalVerifyFilesExpansionFailed
func (e *VerifyFilesExpansionError) Is(target error) bool {
    if e.Level == "global" && target == ErrGlobalVerifyFilesExpansionFailed {
        return true
    }
    if e.Level != "global" && target == ErrGroupVerifyFilesExpansionFailed {
        return true
    }
    return false
}
```

#### 1.5.4 エラー使用例

```go
// エラー生成例
func ExpandGlobalVerifyFiles(...) error {
    for i, path := range global.VerifyFiles {
        expandedPath, err := processor.Expand(...)
        if err != nil {
            return &VerifyFilesExpansionError{
                Level:     "global",
                Index:     i,
                Path:      path,
                Cause:     err,
                Allowlist: global.EnvAllowlist,
            }
        }
    }
    return nil
}

// エラー判定例（errors.Is を使用）
if err := ExpandGlobalVerifyFiles(...); err != nil {
    // sentinel error との比較
    if errors.Is(err, ErrGlobalVerifyFilesExpansionFailed) {
        // グローバルレベルの展開エラーとして処理
    }

    // 元のエラー型との比較（例: allowlist エラー）
    if errors.Is(err, environment.ErrVariableNotAllowed) {
        // allowlist 違反として処理
    }

    // カスタムエラー型の取得（errors.As を使用）
    var expansionErr *VerifyFilesExpansionError
    if errors.As(err, &expansionErr) {
        // エラーの詳細情報にアクセス
        log.Error("Expansion failed",
            "level", expansionErr.Level,
            "index", expansionErr.Index,
            "path", expansionErr.Path,
            "allowlist", expansionErr.Allowlist)
    }
}
```

### 1.6 変数展開仕様

#### 1.6.1 変数形式

verify_files の変数展開は、タスク 0026 と同じ仕様を使用:

- **サポート形式**: `${VAR}` のみ
- **エスケープ**: `\$` → `$` (リテラル)、`\\` → `\` (リテラル)
- **循環参照検出**: visited map による検出

#### 1.6.2 展開順序

```
1. TOML ファイルの読み込み
2. グローバル verify_files の展開
   - システム環境変数のみ使用
   - global.env_allowlist を適用
3. 各グループの verify_files の展開
   - システム環境変数のみ使用
   - group.env_allowlist を適用（継承モードに従う）
4. 各コマンドの cmd/args の展開（既存ロジック）
```

#### 1.6.3 環境変数の優先順位

グループ verify_files の展開時:

```
1. システム環境変数のみ使用
   （注: Command レベルの env フィールドは verify_files 展開には使用しない）
```

### 1.7 セキュリティ検証

#### 1.7.1 allowlist 検証

```go
// CommandEnvProcessor.Expand 内で実行される
// グローバルレベル: global.env_allowlist を使用
// グループレベル: 継承モードに応じて allowlist を決定

func (p *CommandEnvProcessor) Expand(
    value string,
    envVars map[string]string,
    allowlist []string,
    groupName string,
    visited map[string]bool,
) (string, error) {
    // 変数参照を抽出
    vars := extractVariableReferences(value)

    // allowlist 検証
    for _, varName := range vars {
        if !isInAllowlist(varName, allowlist) {
            return "", fmt.Errorf("%w: %s not in allowlist for %s",
                ErrVariableNotAllowed, varName, groupName)
        }
    }

    // 変数展開（循環参照検出を含む）
    return expandWithCircularCheck(value, envVars, allowlist, groupName, visited)
}
```

#### 1.7.2 循環参照検出

タスク 0026 と同じ visited map 方式を使用:

```go
// 展開時に visited map で循環参照を検出
func expandWithCircularCheck(
    value string,
    envVars map[string]string,
    allowlist []string,
    groupName string,
    visited map[string]bool,
) (string, error) {
    // 変数を展開する際、visited map に記録
    // 既に visited に存在する変数を再度展開しようとした場合、循環参照エラー
    // ...
}
```

### 1.8 テストケース仕様

#### 1.8.1 単体テスト

**ExpandGlobalVerifyFiles のテスト**:

```go
func TestExpandGlobalVerifyFiles(t *testing.T) {
    tests := []struct {
        name                  string
        global                *runnertypes.GlobalConfig
        systemEnv             map[string]string
        expected              []string
        expectError           bool
        expectedSentinelError error  // errors.Is でチェック
        expectedCauseError    error  // Unwrap 後の元のエラーを errors.Is でチェック
    }{
        {
            name: "basic expansion with single variable",
            global: &runnertypes.GlobalConfig{
                VerifyFiles:  []string{"${HOME}/bin/tool.sh"},
                EnvAllowlist: []string{"HOME"},
            },
            systemEnv: map[string]string{"HOME": "/home/user"},
            expected:  []string{"/home/user/bin/tool.sh"},
        },
        {
            name: "multiple variables in single path",
            global: &runnertypes.GlobalConfig{
                VerifyFiles:  []string{"${BASE}/${VERSION}/tool"},
                EnvAllowlist: []string{"BASE", "VERSION"},
            },
            systemEnv: map[string]string{"BASE": "/opt", "VERSION": "1.0"},
            expected:  []string{"/opt/1.0/tool"},
        },
        {
            name: "variable not in allowlist",
            global: &runnertypes.GlobalConfig{
                VerifyFiles:  []string{"${PATH}/bin/tool"},
                EnvAllowlist: []string{"HOME"},
            },
            systemEnv:   map[string]string{"PATH": "/usr/bin"},
            expectError: true,
            // errors.Is でのチェック
            expectedSentinelError: ErrGlobalVerifyFilesExpansionFailed,
            // 元のエラー型のチェック
            expectedCauseError: environment.ErrVariableNotAllowed,
        },
        {
            name: "undefined variable",
            global: &runnertypes.GlobalConfig{
                VerifyFiles:  []string{"${UNDEFINED}/tool"},
                EnvAllowlist: []string{"UNDEFINED"},
            },
            systemEnv:             map[string]string{},
            expectError:           true,
            expectedSentinelError: ErrGlobalVerifyFilesExpansionFailed,
            expectedCauseError:    environment.ErrUndefinedVariable,
        },
        {
            name: "no expansion needed",
            global: &runnertypes.GlobalConfig{
                VerifyFiles:  []string{"/usr/bin/python3"},
                EnvAllowlist: []string{},
            },
            expected: []string{"/usr/bin/python3"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup processor with mocked system environment
            processor := setupProcessorWithEnv(tt.systemEnv)

            err := ExpandGlobalVerifyFiles(tt.global, processor)

            if tt.expectError {
                require.Error(t, err)

                // sentinel error のチェック
                if tt.expectedSentinelError != nil {
                    assert.ErrorIs(t, err, tt.expectedSentinelError,
                        "error should match sentinel error")
                }

                // 元のエラー型のチェック
                if tt.expectedCauseError != nil {
                    assert.ErrorIs(t, err, tt.expectedCauseError,
                        "error chain should contain expected cause error")
                }

                // カスタムエラー型のチェック
                var expansionErr *VerifyFilesExpansionError
                if assert.ErrorAs(t, err, &expansionErr) {
                    assert.Equal(t, "global", expansionErr.Level)
                    assert.NotEmpty(t, expansionErr.Path)
                }
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expected, tt.global.ExpandedVerifyFiles)
            }
        })
    }
}
```

**ExpandGroupVerifyFiles のテスト**:

```go
func TestExpandGroupVerifyFiles(t *testing.T) {
    tests := []struct {
        name                  string
        group                 *runnertypes.CommandGroup
        global                *runnertypes.GlobalConfig
        systemEnv             map[string]string
        expected              []string
        expectError           bool
        expectedSentinelError error // errors.Is でチェック
        expectedCauseError    error // Unwrap 後の元のエラーを errors.Is でチェック
    }{
        {
            name: "system env variable expansion",
            group: &runnertypes.CommandGroup{
                Name:         "test",
                VerifyFiles:  []string{"${TOOLS_DIR}/verify.sh"},
                EnvAllowlist: []string{"TOOLS_DIR"},
                Commands:     []runnertypes.Command{},
            },
            global:    &runnertypes.GlobalConfig{},
            systemEnv: map[string]string{"TOOLS_DIR": "/opt/tools"},
            expected:  []string{"/opt/tools/verify.sh"},
        },
        {
            name: "inherit global allowlist",
            group: &runnertypes.CommandGroup{
                Name:         "test",
                VerifyFiles:  []string{"${HOME}/config.conf"},
                EnvAllowlist: nil, // Inherit from global
                Commands:     []runnertypes.Command{},
            },
            global: &runnertypes.GlobalConfig{
                EnvAllowlist: []string{"HOME"},
            },
            systemEnv: map[string]string{"HOME": "/home/user"},
            expected:  []string{"/home/user/config.conf"},
        },
        {
            name: "reject all variables with empty allowlist",
            group: &runnertypes.CommandGroup{
                Name:         "test",
                VerifyFiles:  []string{"${HOME}/file"},
                EnvAllowlist: []string{}, // Explicit empty - reject all
                Commands:     []runnertypes.Command{},
            },
            systemEnv:             map[string]string{"HOME": "/home/user"},
            expectError:           true,
            expectedSentinelError: ErrGroupVerifyFilesExpansionFailed,
            expectedCauseError:    environment.ErrVariableNotAllowed,
        },
        {
            name: "circular reference in system environment",
            group: &runnertypes.CommandGroup{
                Name:         "test",
                VerifyFiles:  []string{"${VAR1}/file"},
                EnvAllowlist: []string{"VAR1", "VAR2"},
                Commands:     []runnertypes.Command{},
            },
            systemEnv: map[string]string{
                "VAR1": "${VAR2}/path",
                "VAR2": "${VAR1}/path",
            },
            expectError:           true,
            expectedSentinelError: ErrGroupVerifyFilesExpansionFailed,
            expectedCauseError:    environment.ErrCircularReference,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            processor := setupProcessorWithEnv(tt.systemEnv)

            err := ExpandGroupVerifyFiles(tt.group, tt.global, processor)

            if tt.expectError {
                require.Error(t, err)

                // sentinel error のチェック
                if tt.expectedSentinelError != nil {
                    assert.ErrorIs(t, err, tt.expectedSentinelError,
                        "error should match sentinel error")
                }

                // 元のエラー型のチェック
                if tt.expectedCauseError != nil {
                    assert.ErrorIs(t, err, tt.expectedCauseError,
                        "error chain should contain expected cause error")
                }

                // カスタムエラー型のチェック
                var expansionErr *VerifyFilesExpansionError
                if assert.ErrorAs(t, err, &expansionErr) {
                    assert.Equal(t, tt.group.Name, expansionErr.Level)
                    assert.NotEmpty(t, expansionErr.Path)
                }
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expected, tt.group.ExpandedVerifyFiles)
            }
        })
    }
}
```

#### 1.8.2 統合テスト

```go
func TestVerifyFilesExpansionIntegration(t *testing.T) {
    // Create test TOML content
    tomlContent := `
version = "1.0"

[global]
env_allowlist = ["HOME"]
verify_files = ["${HOME}/bin/tool.sh"]

[[groups]]
name = "test"
env_allowlist = ["TOOLS_DIR", "HOME"]
verify_files = ["${TOOLS_DIR}/verify.sh", "${HOME}/config.conf"]

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
`

    // Set up system environment for testing
    t.Setenv("HOME", "/home/user")
    t.Setenv("TOOLS_DIR", "/opt/tools")

    // Load and expand using Loader.LoadConfig
    loader := config.NewLoader()
    cfg, err := loader.LoadConfig([]byte(tomlContent))
    require.NoError(t, err)

    // Verify global expansion
    assert.Equal(t, []string{"/home/user/bin/tool.sh"}, cfg.Global.ExpandedVerifyFiles)

    // Verify group expansion
    assert.Equal(t, []string{
        "/opt/tools/verify.sh",
        "/home/user/config.conf",
    }, cfg.Groups[0].ExpandedVerifyFiles)
}
```

### 1.9 パフォーマンス要件

#### 1.9.1 性能目標

| メトリクス | 目標値 | 測定方法 |
|----------|-------|---------|
| 展開処理時間（パスあたり） | < 1ms | ベンチマークテスト |
| メモリ増加量 | < 10% | メモリプロファイリング |
| 全体処理時間への影響 | < 5% | 統合テストでの測定 |

#### 1.9.2 ベンチマークテスト

```go
func BenchmarkExpandGlobalVerifyFiles(b *testing.B) {
    global := &runnertypes.GlobalConfig{
        VerifyFiles: []string{
            "${HOME}/bin/tool1.sh",
            "${HOME}/bin/tool2.sh",
            "${HOME}/bin/tool3.sh",
        },
        EnvAllowlist: []string{"HOME"},
    }

    processor := setupProcessor()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = ExpandGlobalVerifyFiles(global, processor)
    }
}
```

## 2. 実装チェックリスト

### 2.1 Phase 1: データ構造の拡張
- [ ] GlobalConfig に ExpandedVerifyFiles フィールドを追加
- [ ] CommandGroup に ExpandedVerifyFiles フィールドを追加
- [ ] フィールドのドキュメントコメントを追加

### 2.2 Phase 2: 環境変数展開の実装（既存機能活用）
- [ ] Filter と VariableExpander の既存機能確認
- [ ] Filter.ResolveAllowlistConfiguration メソッドのエクスポート（小文字 → 大文字化）
- [ ] ExpandGlobalVerifyFiles 関数の実装（Filter.ParseSystemEnvironment 使用）
- [ ] ExpandGroupVerifyFiles 関数の実装（Filter.ResolveAllowlistConfiguration 使用）
- [ ] 既存機能との統合テスト

### 2.3 Phase 3: Config Parser の統合（既存機能活用）
- [ ] LoadConfig で Filter と VariableExpander を初期化
- [ ] processConfig 関数の引数を Filter/VariableExpander に変更
- [ ] ExpandGlobalVerifyFiles の呼び出しを追加（expander 使用）
- [ ] ExpandGroupVerifyFiles の呼び出しを追加（filter/expander 使用）
- [ ] エラーハンドリングの実装

### 2.4 Phase 4: Verification Manager の更新
- [ ] VerifyGlobalFiles を ExpandedVerifyFiles 使用に変更
- [ ] collectVerificationFiles を ExpandedVerifyFiles 使用に変更
- [ ] 既存のテストの更新

### 2.5 Phase 5: テストの実装
- [ ] ExpandGlobalVerifyFiles の単体テスト（既存機能との統合確認）
- [ ] ExpandGroupVerifyFiles の単体テスト（既存機能との統合確認）
- [ ] Filter/VariableExpander 統合の動作確認テスト
- [ ] 統合テストの実装
- [ ] エラーケースのテスト
- [ ] ベンチマークテストの実装

### 2.6 Phase 6: ドキュメント
- [ ] ユーザーガイドの更新
- [ ] サンプル TOML ファイルの作成
- [ ] CHANGELOG の更新

## 3. 参照

- タスク 0026: Variable Expansion Implementation（環境変数展開の基盤実装）
- タスク 0007: verify_hash_all（ファイル検証機能）
- タスク 0008: env_allowlist（環境変数 allowlist 機能）
