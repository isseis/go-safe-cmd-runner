# 実装計画書: Allowlist機能の短期的改善

## 1. 実装概要

### 1.1 実装方針
- **段階的実装**: 各フェーズを独立して完了させ、テスト・検証を行う
- **最小リスク**: 既存機能への影響を最小限に抑制
- **検証重視**: 各段階で包括的なテストを実施
- **ドキュメント同期**: 実装とドキュメントの整合性維持

### 1.2 実装スケジュール

```
Phase 1: 継承ロジックの明確化       (推定工数: 3-4日)
├── 設計実装                      (1.5日)
├── テスト作成・実行               (1日)
└── レビュー・修正                 (0.5-1.5日)

Phase 2: Command.Env除外実装       (推定工数: 3-4日)
├── 設計実装                      (2日)
├── テスト作成・実行               (1日)
└── レビュー・修正                 (0.5-1日)

Phase 3: 設定検証機能追加          (推定工数: 4-5日)
├── 設計実装                      (2.5-3日)
├── テスト作成・実行               (1日)
└── レビュー・修正                 (0.5-1日)

総合テスト・統合                   (推定工数: 2-3日)
├── 統合テスト実行                 (1日)
├── パフォーマンステスト           (0.5日)
└── ドキュメント最終更新           (0.5-1.5日)

合計推定工数: 12-16日
```

## 2. Phase 1: 継承ロジックの明確化

### 2.1 実装対象ファイル

```
修正対象:
├── internal/runner/environment/filter.go      (主要修正)
├── internal/runner/runnertypes/types.go       (列挙型追加)
└── internal/runner/environment/filter_test.go (テスト追加)

影響範囲:
└── internal/runner/runner.go                  (軽微な修正の可能性)
```

### 2.2 実装手順

#### Step 1.1: データ構造の実装 (0.5日)

```go
// internal/runner/runnertypes/types.go に追加

// InheritanceMode represents how environment allowlist inheritance works
type InheritanceMode int

const (
    // InheritanceModeInherit indicates the group inherits from global allowlist
    // This occurs when env_allowlist field is not defined (nil slice)
    InheritanceModeInherit InheritanceMode = iota

    // InheritanceModeExplicit indicates the group uses only its explicit allowlist
    // This occurs when env_allowlist field has values: ["VAR1", "VAR2"]
    InheritanceModeExplicit

    // InheritanceModeReject indicates the group rejects all environment variables
    // This occurs when env_allowlist field is explicitly empty: []
    InheritanceModeReject
)

// String returns a string representation of InheritanceMode for logging
func (m InheritanceMode) String() string {
    switch m {
    case InheritanceModeInherit:
        return "inherit"
    case InheritanceModeExplicit:
        return "explicit"
    case InheritanceModeReject:
        return "reject"
    default:
        return "unknown"
    }
}

// AllowlistResolution contains resolved allowlist information for debugging and logging
type AllowlistResolution struct {
    Mode            InheritanceMode
    GroupAllowlist  []string
    GlobalAllowlist []string
    EffectiveList   []string    // The actual allowlist being used
    GroupName       string      // For logging context
}

// IsAllowed checks if a variable is allowed based on the resolved allowlist
func (r *AllowlistResolution) IsAllowed(variable string) bool {
    switch r.Mode {
    case InheritanceModeReject:
        return false
    case InheritanceModeExplicit:
        return slices.Contains(r.GroupAllowlist, variable)
    case InheritanceModeInherit:
        return slices.Contains(r.GlobalAllowlist, variable)
    default:
        return false
    }
}
```

**実装チェックリスト:**
- [ ] InheritanceMode型の定義
- [ ] String()メソッドの実装
- [ ] AllowlistResolution構造体の定義
- [ ] IsAllowed()メソッドの実装
- [ ] 必要なimportの追加 (slices パッケージ)

#### Step 1.2: 継承ロジック関数の実装 (0.5日)

```go
// internal/runner/environment/filter.go に追加

// determineInheritanceMode determines the inheritance mode based on group configuration
func (f *Filter) determineInheritanceMode(group *runnertypes.CommandGroup) (runnertypes.InheritanceMode, error) {
    if group == nil {
        return 0, fmt.Errorf("group is nil")
    }

    // nil slice = inherit, empty slice = reject, non-empty = explicit
    if group.EnvAllowlist == nil {
        return runnertypes.InheritanceModeInherit, nil
    }

    if len(group.EnvAllowlist) == 0 {
        return runnertypes.InheritanceModeReject, nil
    }

    return runnertypes.InheritanceModeExplicit, nil
}

// resolveAllowlistConfiguration resolves the effective allowlist configuration for a group
func (f *Filter) resolveAllowlistConfiguration(group *runnertypes.CommandGroup) (*runnertypes.AllowlistResolution, error) {
    mode, err := f.determineInheritanceMode(group)
    if err != nil {
        return nil, fmt.Errorf("failed to determine inheritance mode: %w", err)
    }

    resolution := &runnertypes.AllowlistResolution{
        Mode:           mode,
        GroupAllowlist: group.EnvAllowlist,
        GroupName:      group.Name,
    }

    // Convert global allowlist map to slice for consistent interface
    globalList := make([]string, 0, len(f.globalAllowlist))
    for variable := range f.globalAllowlist {
        globalList = append(globalList, variable)
    }
    resolution.GlobalAllowlist = globalList

    // Set effective list based on mode
    switch mode {
    case runnertypes.InheritanceModeInherit:
        resolution.EffectiveList = resolution.GlobalAllowlist
    case runnertypes.InheritanceModeExplicit:
        resolution.EffectiveList = resolution.GroupAllowlist
    case runnertypes.InheritanceModeReject:
        resolution.EffectiveList = []string{} // Explicitly empty
    }

    // Log the resolution for debugging
    slog.Debug("Resolved allowlist configuration",
        "group", group.Name,
        "mode", mode.String(),
        "group_allowlist_size", len(group.EnvAllowlist),
        "global_allowlist_size", len(f.globalAllowlist),
        "effective_allowlist_size", len(resolution.EffectiveList))

    return resolution, nil
}
```

**実装チェックリスト:**
- [ ] determineInheritanceMode関数の実装
- [ ] resolveAllowlistConfiguration関数の実装
- [ ] エラーハンドリングの実装
- [ ] ログ出力の実装
- [ ] 必要なimportの追加

#### Step 1.3: 改善された変数許可チェック関数 (0.5日)

```go
// internal/runner/environment/filter.go に追加/修正

// resolveAllowedVariable checks if a variable is allowed based on the inheritance configuration
// This replaces the old isVariableAllowed function with clearer logic
func (f *Filter) resolveAllowedVariable(variable string, group *runnertypes.CommandGroup) (bool, error) {
    resolution, err := f.resolveAllowlistConfiguration(group)
    if err != nil {
        return false, fmt.Errorf("failed to resolve allowlist configuration: %w", err)
    }

    allowed := resolution.IsAllowed(variable)

    if !allowed {
        slog.Warn("Variable access denied",
            "variable", variable,
            "group", group.Name,
            "inheritance_mode", resolution.Mode.String(),
            "effective_allowlist_size", len(resolution.EffectiveList))
    } else {
        slog.Debug("Variable access granted",
            "variable", variable,
            "group", group.Name,
            "inheritance_mode", resolution.Mode.String())
    }

    return allowed, nil
}

// IsVariableAccessAllowed checks if a variable can be accessed in the given group context
// This function now uses the improved inheritance logic
func (f *Filter) IsVariableAccessAllowed(variable string, group *runnertypes.CommandGroup) bool {
    if group == nil {
        slog.Error("IsVariableAccessAllowed called with nil group - this indicates a programming error")
        return false
    }

    allowed, err := f.resolveAllowedVariable(variable, group)
    if err != nil {
        slog.Error("Failed to resolve variable allowlist",
            "variable", variable,
            "group", group.Name,
            "error", err)
        return false
    }

    return allowed
}
```

**実装チェックリスト:**
- [ ] resolveAllowedVariable関数の実装
- [ ] IsVariableAccessAllowed関数の更新
- [ ] 既存のisVariableAllowed関数のdeprecation/置き換え確認
- [ ] エラーハンドリングとログの実装

#### Step 1.4: 単体テストの作成 (1日)

```go
// internal/runner/environment/filter_test.go に追加

func TestDetermineInheritanceMode(t *testing.T) {
    tests := []struct {
        name           string
        group          *runnertypes.CommandGroup
        expectedMode   runnertypes.InheritanceMode
        expectError    bool
    }{
        {
            name: "nil group should return error",
            group: nil,
            expectError: true,
        },
        {
            name: "nil allowlist should inherit",
            group: &runnertypes.CommandGroup{
                Name: "test",
                EnvAllowlist: nil,
            },
            expectedMode: runnertypes.InheritanceModeInherit,
        },
        {
            name: "empty allowlist should reject",
            group: &runnertypes.CommandGroup{
                Name: "test",
                EnvAllowlist: []string{},
            },
            expectedMode: runnertypes.InheritanceModeReject,
        },
        {
            name: "non-empty allowlist should be explicit",
            group: &runnertypes.CommandGroup{
                Name: "test",
                EnvAllowlist: []string{"VAR1", "VAR2"},
            },
            expectedMode: runnertypes.InheritanceModeExplicit,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            filter := NewFilter(&runnertypes.Config{})
            mode, err := filter.determineInheritanceMode(tt.group)

            if tt.expectError {
                assert.Error(t, err)
                return
            }

            assert.NoError(t, err)
            assert.Equal(t, tt.expectedMode, mode)
        })
    }
}

func TestAllowlistInheritanceIntegration(t *testing.T) {
    // 統合テストケースの実装
    // (詳細設計書のテストケースを参照)
}
```

**テスト実装チェックリスト:**
- [ ] determineInheritanceMode関数のテスト
- [ ] resolveAllowlistConfiguration関数のテスト
- [ ] resolveAllowedVariable関数のテスト
- [ ] 統合テストケースの実装
- [ ] エッジケース（nil、空配列等）のテスト
- [ ] パフォーマンステストの実装

### 2.3 Phase 1 完了条件

- [ ] すべての新しい単体テストがパス
- [ ] 既存のテストがすべてパス（回帰なし）
- [ ] コードカバレッジが80%以上
- [ ] ログ出力の検証完了
- [ ] コードレビュー完了

## 3. Phase 2: Command.Env の allowlist チェック除外

### 3.1 実装対象ファイル

```
修正対象:
├── internal/runner/runner.go                    (主要修正)
├── internal/runner/environment/processor.go     (新規作成)
└── internal/runner/runner_test.go               (テスト追加)

影響範囲:
└── internal/runner/environment/filter.go        (軽微な修正)
```

### 3.2 実装手順

#### Step 2.1: CommandEnvProcessor の実装 (1日)

```go
// internal/runner/environment/processor.go (新規作成)

package environment

import (
    "fmt"
    "os"
    "regexp"
    "strings"
    "log/slog"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// CommandEnvProcessor handles command-specific environment variable processing
type CommandEnvProcessor struct {
    filter *Filter
    logger *slog.Logger
}

// NewCommandEnvProcessor creates a new processor for command environment variables
func NewCommandEnvProcessor(filter *Filter) *CommandEnvProcessor {
    return &CommandEnvProcessor{
        filter: filter,
        logger: slog.Default(),
    }
}

// ProcessCommandEnvironment processes Command.Env variables without allowlist checks
func (p *CommandEnvProcessor) ProcessCommandEnvironment(
    cmd runnertypes.Command,
    baseEnvVars map[string]string,
    group *runnertypes.CommandGroup,
) (map[string]string, error) {

    // Create a copy of base environment variables
    envVars := make(map[string]string, len(baseEnvVars))
    for k, v := range baseEnvVars {
        envVars[k] = v
    }

    // Process each Command.Env entry
    for i, env := range cmd.Env {
        variable, value, ok := ParseEnvVariable(env)
        if !ok {
            p.logger.Warn("Invalid environment variable format in Command.Env",
                "command", cmd.Name,
                "env_index", i,
                "env_entry", env)
            continue
        }

        // Basic validation (but no allowlist check)
        if err := p.validateBasicEnvVariable(variable, value); err != nil {
            return nil, fmt.Errorf("invalid command environment variable %s in command %s: %w",
                variable, cmd.Name, err)
        }

        // Resolve variable references with security policy
        resolvedValue, err := p.resolveVariableReferencesForCommandEnv(value, envVars, group)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve variable references in %s for command %s: %w",
                variable, cmd.Name, err)
        }

        envVars[variable] = resolvedValue

        p.logger.Debug("Processed command environment variable",
            "command", cmd.Name,
            "variable", variable,
            "value_length", len(resolvedValue))
    }

    return envVars, nil
}
```

**実装チェックリスト:**
- [ ] CommandEnvProcessor構造体の実装
- [ ] ProcessCommandEnvironment関数の実装
- [ ] 基本的なvalidation関数の実装
- [ ] エラーハンドリングの実装
- [ ] ログ出力の実装

#### Step 2.2: 変数参照解決の実装 (0.5日)

```go
// internal/runner/environment/processor.go に追加

// resolveVariableReferencesForCommandEnv resolves variable references for Command.Env values
func (p *CommandEnvProcessor) resolveVariableReferencesForCommandEnv(
    value string,
    envVars map[string]string,
    group *runnertypes.CommandGroup,
) (string, error) {

    if !strings.Contains(value, "${") {
        return value, nil
    }

    result := value
    maxIterations := 10  // Prevent infinite loops
    var resolutionError error

    for i := 0; i < maxIterations && strings.Contains(result, "${"); i++ {
        oldResult := result

        result = regexp.MustCompile(`\$\{([^}]+)\}`).ReplaceAllStringFunc(result, func(match string) string {
            varName := match[2 : len(match)-1] // Remove ${ and }

            resolvedValue, err := p.resolveVariableWithSecurityPolicy(varName, envVars, group)
            if err != nil {
                if resolutionError == nil {
                    resolutionError = fmt.Errorf("failed to resolve variable reference ${%s}: %w", varName, err)
                }
                return match // Continue processing other variables
            }

            return resolvedValue
        })

        if result == oldResult {
            break
        }
    }

    if resolutionError != nil {
        return "", resolutionError
    }

    return result, nil
}

// resolveVariableWithSecurityPolicy resolves a variable reference with appropriate security checks
func (p *CommandEnvProcessor) resolveVariableWithSecurityPolicy(
    varName string,
    envVars map[string]string,
    group *runnertypes.CommandGroup,
) (string, error) {
    // Priority 1: Check existing resolved variables (Command.Env + trusted sources)
    if val, exists := envVars[varName]; exists {
        p.logger.Debug("Variable resolved from trusted source",
            "variable", varName,
            "source", "resolved_env_vars")
        return val, nil
    }

    // Priority 2: Check system environment with allowlist validation
    if sysVal, exists := os.LookupEnv(varName); exists {
        return p.resolveSystemVariable(varName, sysVal, group)
    }

    // Priority 3: Variable not found
    return "", fmt.Errorf("variable reference not found: %s", varName)
}

// resolveSystemVariable resolves a system environment variable with allowlist checks
func (p *CommandEnvProcessor) resolveSystemVariable(
    varName, sysVal string,
    group *runnertypes.CommandGroup,
) (string, error) {
    allowed, err := p.filter.resolveAllowedVariable(varName, group)
    if err != nil {
        p.logger.Error("Failed to check variable allowlist",
            "variable", varName,
            "group", group.Name,
            "error", err)
        return "", fmt.Errorf("allowlist check failed for variable %s: %w", varName, err)
    }

    if !allowed {
        p.logger.Warn("Command.Env references disallowed system variable",
            "variable", varName,
            "group", group.Name)
        return "", fmt.Errorf("variable %s is not allowed by group allowlist", varName)
    }

    p.logger.Debug("System variable resolved for Command.Env",
        "variable", varName,
        "group", group.Name)
    return sysVal, nil
}
```

**実装チェックリスト:**
- [ ] 変数参照解決アルゴリズムの実装
- [ ] セキュリティポリシーの実装
- [ ] システム変数のallowlistチェック
- [ ] 循環参照防止機能
- [ ] エラーハンドリング

#### Step 2.3: Runner の更新 (0.5日)

```go
// internal/runner/runner.go の修正

// resolveEnvironmentVars の更新
func (r *Runner) resolveEnvironmentVars(cmd runnertypes.Command, group *runnertypes.CommandGroup) (map[string]string, error) {
    // Step 1: Resolve system and .env file variables with allowlist filtering
    systemEnvVars, err := r.envFilter.ResolveGroupEnvironmentVars(group, r.envVars)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve group environment variables: %w", err)
    }

    slog.Debug("Resolved system environment variables",
        "group", group.Name,
        "system_vars_count", len(systemEnvVars))

    // Step 2: Process Command.Env variables without allowlist checks
    processor := environment.NewCommandEnvProcessor(r.envFilter)
    finalEnvVars, err := processor.ProcessCommandEnvironment(cmd, systemEnvVars, group)
    if err != nil {
        return nil, fmt.Errorf("failed to process command environment variables: %w", err)
    }

    slog.Debug("Processed command environment variables",
        "command", cmd.Name,
        "group", group.Name,
        "final_vars_count", len(finalEnvVars))

    return finalEnvVars, nil
}
```

**実装チェックリスト:**
- [ ] resolveEnvironmentVars関数の更新
- [ ] CommandEnvProcessorの統合
- [ ] 既存のCommand.Env処理ロジックの削除
- [ ] エラーハンドリングの更新

#### Step 2.4: 単体テストの作成 (1日)

**テスト実装チェックリスト:**
- [ ] CommandEnvProcessor単体テスト
- [ ] 変数参照解決のテスト
- [ ] セキュリティポリシーのテスト
- [ ] 統合テスト（詳細設計書参照）
- [ ] エラーケースのテスト
- [ ] パフォーマンステスト

### 3.3 Phase 2 完了条件

- [ ] CommandEnvProcessorのすべてのテストがパス
- [ ] 既存のテストがすべてパス
- [ ] Command.Env変数がallowlistを迂回することの確認
- [ ] 変数参照でのallowlistチェックが適切に動作
- [ ] コードレビュー完了

## 4. Phase 3: 設定検証機能の追加

### 4.1 実装対象ファイル

```
新規作成:
├── internal/runner/config/validator.go          (主要実装)
├── internal/runner/config/validator_test.go     (テスト)
└── internal/runner/config/validation_types.go  (型定義)

修正対象:
└── cmd/runner/main.go                          (CLI統合)
```

### 4.2 実装手順

#### Step 3.1: 検証関連型定義 (0.5日)

```go
// internal/runner/config/validation_types.go (新規作成)

package config

import "time"

// ValidationResult contains the results of configuration validation
type ValidationResult struct {
    Valid      bool                    `json:"valid"`
    Errors     []ValidationError       `json:"errors"`
    Warnings   []ValidationWarning     `json:"warnings"`
    Summary    ValidationSummary       `json:"summary"`
    Timestamp  time.Time              `json:"timestamp"`
}

// ValidationError represents a configuration error that prevents operation
type ValidationError struct {
    Type        string `json:"type"`
    Message     string `json:"message"`
    Location    string `json:"location"`    // e.g., "groups[0].env_allowlist"
    Severity    string `json:"severity"`
}

// ValidationWarning represents a configuration issue that might cause problems
type ValidationWarning struct {
    Type        string `json:"type"`
    Message     string `json:"message"`
    Location    string `json:"location"`
    Suggestion  string `json:"suggestion"`
}

// ValidationSummary provides a high-level overview of validation results
type ValidationSummary struct {
    TotalGroups       int `json:"total_groups"`
    GroupsWithAllowlist int `json:"groups_with_allowlist"`
    GlobalAllowlistSize int `json:"global_allowlist_size"`
    TotalCommands     int `json:"total_commands"`
    CommandsWithEnv   int `json:"commands_with_env"`
}
```

#### Step 3.2: ConfigValidator の実装 (2日)

```go
// internal/runner/config/validator.go (新規作成)

package config

import (
    "bytes"
    "errors"
    "fmt"
    "slices"
    "strings"
    "time"
    "log/slog"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
)

// ConfigValidator provides comprehensive validation for runner configurations
type ConfigValidator struct {
    logger *slog.Logger
}

// NewConfigValidator creates a new configuration validator
func NewConfigValidator() *ConfigValidator {
    return &ConfigValidator{
        logger: slog.Default(),
    }
}

// ValidateConfig performs comprehensive validation of the configuration
func (v *ConfigValidator) ValidateConfig(config *runnertypes.Config) (*ValidationResult, error) {
    result := &ValidationResult{
        Valid:     true,
        Errors:    []ValidationError{},
        Warnings:  []ValidationWarning{},
        Timestamp: time.Now(),
    }

    // Validate global configuration
    if err := v.validateGlobalConfig(&config.Global, result); err != nil {
        return nil, fmt.Errorf("failed to validate global config: %w", err)
    }

    // Validate groups
    for i, group := range config.Groups {
        if err := v.validateGroup(&group, i, &config.Global, result); err != nil {
            return nil, fmt.Errorf("failed to validate group %s: %w", group.Name, err)
        }
    }

    // Calculate summary
    v.calculateSummary(config, result)

    // Set overall validity
    result.Valid = len(result.Errors) == 0

    return result, nil
}

// Implement all validation methods from detailed design
// (validateGlobalConfig, validateGroup, etc.)
```

**実装チェックリスト:**
- [ ] ConfigValidator構造体の実装
- [ ] ValidateConfig主要関数の実装
- [ ] validateGlobalConfig関数の実装
- [ ] validateGroup関数の実装
- [ ] validateAllowlist関数の実装
- [ ] validateCommand関数の実装

#### Step 3.3: レポート生成機能 (0.5日)

```go
// internal/runner/config/validator.go に追加

// GenerateValidationReport generates a human-readable validation report
func (v *ConfigValidator) GenerateValidationReport(result *ValidationResult) (string, error) {
    var buf bytes.Buffer

    // Header
    fmt.Fprintf(&buf, "Configuration Validation Report\n")
    fmt.Fprintf(&buf, "Generated: %s\n", result.Timestamp.Format(time.RFC3339))
    fmt.Fprintf(&buf, "Overall Status: %s\n", v.getStatusString(result.Valid))
    fmt.Fprintf(&buf, "\n")

    // Summary, Errors, Warnings sections
    // (詳細設計書の実装を参照)

    return buf.String(), nil
}
```

#### Step 3.4: CLI統合 (1日)

```go
// cmd/runner/main.go に追加

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/config"
)

// validateConfigCommand implements config validation CLI command
func validateConfigCommand(configPath string) error {
    // Load config
    cfg, err := loadConfig(configPath)
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // Validate config
    validator := config.NewConfigValidator()
    result, err := validator.ValidateConfig(cfg)
    if err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Generate and display report
    report, err := validator.GenerateValidationReport(result)
    if err != nil {
        return fmt.Errorf("failed to generate report: %w", err)
    }

    fmt.Print(report)

    // Exit with appropriate code
    if !result.Valid {
        os.Exit(1)
    }

    return nil
}
```

#### Step 3.5: 単体テストの作成 (1日)

**テスト実装チェックリスト:**
- [ ] ConfigValidator単体テスト
- [ ] 各検証関数のテスト
- [ ] レポート生成のテスト
- [ ] CLI統合のテスト
- [ ] エラーケース・エッジケースのテスト

### 4.3 Phase 3 完了条件

- [ ] ConfigValidatorのすべてのテストがパス
- [ ] CLI統合が正常に動作
- [ ] 各種設定パターンで適切な検証結果
- [ ] レポート出力の確認
- [ ] パフォーマンスへの影響が許容範囲内
- [ ] コードレビュー完了

## 5. 統合テストと最終検証

### 5.1 統合テスト実装 (1日)

```go
// internal/runner/integration_test.go (新規作成または拡張)

func TestFullAllowlistIntegration(t *testing.T) {
    // Phase 1-3 のすべての機能を統合したテスト
    // (詳細設計書のテストケースを実装)

    config := createTestConfig()

    // Phase 3: Validate configuration
    validator := config.NewConfigValidator()
    validationResult, err := validator.ValidateConfig(config)
    require.NoError(t, err)
    require.True(t, validationResult.Valid)

    // Phase 1: Test inheritance logic
    filter := environment.NewFilter(config)
    // ... 継承ロジックのテスト

    // Phase 2: Test Command.Env processing
    processor := environment.NewCommandEnvProcessor(filter)
    // ... Command.Env処理のテスト

    // Verify end-to-end functionality
    runner := NewRunner(config)
    // ... エンドツーエンドのテスト
}
```

### 5.2 パフォーマンステスト (0.5日)

```go
// performance_test.go
func BenchmarkAllowlistResolution(b *testing.B) {
    // Phase 1 の継承ロジック性能テスト
}

func BenchmarkCommandEnvProcessing(b *testing.B) {
    // Phase 2 の Command.Env 処理性能テスト
}

func BenchmarkConfigValidation(b *testing.B) {
    // Phase 3 の設定検証性能テスト
}
```

### 5.3 最終検証チェックリスト

#### 機能検証
- [ ] すべてのPhaseの機能が期待通りに動作
- [ ] 既存機能への影響なし（回帰テスト）
- [ ] エラーハンドリングが適切に動作
- [ ] ログ出力が適切

#### 性能検証
- [ ] パフォーマンスの劣化なし
- [ ] メモリ使用量が許容範囲内
- [ ] 大規模設定での動作確認

#### セキュリティ検証
- [ ] Command.Env変数のallowlist除外が動作
- [ ] システム変数のallowlistチェックが動作
- [ ] 変数参照のセキュリティポリシーが動作

## 6. デプロイメント準備

### 6.1 ドキュメント更新 (0.5日)

- [ ] README.md の更新
- [ ] API ドキュメントの更新
- [ ] 設定例の更新
- [ ] CHANGELOG の作成

### 6.2 リリースノート作成 (0.5日)

```markdown
# Release Notes: Allowlist機能の短期的改善

## 新機能
- 環境変数allowlist継承ロジックの明確化
- Command.Env変数のallowlistチェック除外
- 設定検証機能の追加

## 改善項目
- 継承モード（inherit/explicit/reject）の明確な区別
- Command.Env処理とsystem environment処理の分離
- 包括的な設定検証とレポート生成

## 破壊的変更
なし（内部実装のみの変更）

## 移行ガイド
既存の設定ファイルは変更不要。新しい設定検証機能は任意で利用可能。
```

### 6.3 最終チェックリスト

#### コード品質
- [ ] すべてのテストがパス（単体・統合・パフォーマンス）
- [ ] コードカバレッジが目標値（80%）以上
- [ ] 静的解析ツールでの検証をパス
- [ ] コードレビュー完了

#### ドキュメント
- [ ] 技術文書の最新化
- [ ] API ドキュメントの更新
- [ ] 設定例の検証
- [ ] リリースノートの作成

#### 運用準備
- [ ] ログレベルの適切な設定
- [ ] エラー処理の検証
- [ ] 監視項目の確認
- [ ] ロールバック手順の準備

## 7. リスク管理

### 7.1 技術リスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| 既存機能の回帰 | 高 | 包括的な回帰テスト実施 |
| パフォーマンス劣化 | 中 | ベンチマークテストの実行 |
| メモリリーク | 中 | メモリプロファイリング実施 |
| 設定互換性問題 | 低 | 既存設定での動作テスト |

### 7.2 スケジュールリスク

| リスク | 対策 |
|--------|------|
| Phase 1 の実装遅延 | 優先度を下げたPhase 3機能の後回し |
| テスト工数の過小評価 | バッファ時間の確保、段階的テスト |
| レビュー時間不足 | 早期レビュー開始、並行レビュー |

### 7.3 品質リスク

| リスク | 対策 |
|--------|------|
| テストカバレッジ不足 | 目標カバレッジ80%の設定と監視 |
| エッジケース未考慮 | 詳細設計書のテストケース完全実装 |
| ドキュメント不整合 | 実装と同時進行でのドキュメント更新 |

## 8. 成功指標

### 8.1 機能指標
- [ ] すべてのフェーズの機能要件を満たす
- [ ] 既存機能への回帰なし
- [ ] 新機能のテストカバレッジ80%以上

### 8.2 品質指標
- [ ] 静的解析ツールでの警告ゼロ
- [ ] パフォーマンス劣化5%以内
- [ ] メモリ使用量増加10%以内

### 8.3 運用指標
- [ ] 設定検証機能による設定エラーの早期発見
- [ ] 環境変数関連のログ出力の改善
- [ ] 継承ロジックの理解しやすさの向上

---

この実装計画書に従って段階的に実装を進めることで、安全で確実にallowlist機能の改善を達成できます。各フェーズの完了条件を満たしてから次のフェーズに進むことで、品質とスケジュールの両方を管理します。
