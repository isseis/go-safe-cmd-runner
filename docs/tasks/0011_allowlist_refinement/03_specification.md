# 詳細設計書: Allowlist機能の短期的改善

## 1. 詳細設計概要

### 1.1 設計方針
- **最小限の変更**: 既存の公開APIを変更せず、内部実装のみ改善
- **明確な分離**: Command.Env処理とallowlistチェック処理を分離
- **段階的実装**: 各フェーズを独立して実装・テスト可能にする
- **包括的テスト**: すべての変更に対して詳細なテストを実装

### 1.2 変更影響範囲
```
変更影響レベル:
├── High Impact: なし
├── Medium Impact: internal/runner/runner.go (内部実装のみ)
├── Low Impact: internal/runner/environment/filter.go (内部実装のみ)
└── New Components: internal/runner/config/validator.go
```

## 2. Phase 1: 継承ロジックの明確化

### 2.1 データ構造の設計

#### 2.1.1 InheritanceMode列挙型

```go
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
```

#### 2.1.2 AllowlistResolution構造体

```go
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

### 2.2 関数設計

#### 2.2.1 継承モード判定関数

```go
// determineInheritanceMode determines the inheritance mode based on group configuration
// This function distinguishes between nil (inherit) and empty slice (reject)
func (f *Filter) determineInheritanceMode(group *runnertypes.CommandGroup) (InheritanceMode, error) {
    if group == nil {
        return 0, fmt.Errorf("%w: group is nil", ErrGroupNotFound)
    }

    // Check if env_allowlist was explicitly set in the TOML
    // This requires modification to the CommandGroup struct to track this information
    // For now, we use the heuristic: nil slice = inherit, empty slice = reject
    if group.EnvAllowlist == nil {
        return InheritanceModeInherit, nil
    }

    if len(group.EnvAllowlist) == 0 {
        return InheritanceModeReject, nil
    }

    return InheritanceModeExplicit, nil
}
```

#### 2.2.2 改善された allowlist 解決関数

```go
// resolveAllowlistConfiguration resolves the effective allowlist configuration for a group
func (f *Filter) resolveAllowlistConfiguration(group *runnertypes.CommandGroup) (*AllowlistResolution, error) {
    mode, err := f.determineInheritanceMode(group)
    if err != nil {
        return nil, fmt.Errorf("failed to determine inheritance mode: %w", err)
    }

    resolution := &AllowlistResolution{
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
    case InheritanceModeInherit:
        resolution.EffectiveList = resolution.GlobalAllowlist
    case InheritanceModeExplicit:
        resolution.EffectiveList = resolution.GroupAllowlist
    case InheritanceModeReject:
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

#### 2.2.3 新しい変数許可チェック関数

```go
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
```

### 2.3 既存関数の更新

#### 2.3.1 IsVariableAccessAllowed の更新

```go
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

### 2.4 テスト設計

#### 2.4.1 継承モードテストケース

```go
func TestDetermineInheritanceMode(t *testing.T) {
    tests := []struct {
        name           string
        group          *runnertypes.CommandGroup
        expectedMode   InheritanceMode
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
            expectedMode: InheritanceModeInherit,
        },
        {
            name: "empty allowlist should reject",
            group: &runnertypes.CommandGroup{
                Name: "test",
                EnvAllowlist: []string{},
            },
            expectedMode: InheritanceModeReject,
        },
        {
            name: "non-empty allowlist should be explicit",
            group: &runnertypes.CommandGroup{
                Name: "test",
                EnvAllowlist: []string{"VAR1", "VAR2"},
            },
            expectedMode: InheritanceModeExplicit,
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
```

#### 2.4.2 統合テストケース

```go
func TestAllowlistInheritanceIntegration(t *testing.T) {
    tests := []struct {
        name                string
        globalAllowlist     []string
        groupAllowlist      []string
        groupAllowlistSet   bool  // Whether env_allowlist field was set
        variable            string
        expectedAllowed     bool
        expectedMode        InheritanceMode
    }{
        {
            name: "inherit from global - variable allowed",
            globalAllowlist: []string{"GLOBAL_VAR", "SHARED_VAR"},
            groupAllowlist: nil,
            groupAllowlistSet: false,
            variable: "GLOBAL_VAR",
            expectedAllowed: true,
            expectedMode: InheritanceModeInherit,
        },
        {
            name: "inherit from global - variable denied",
            globalAllowlist: []string{"GLOBAL_VAR"},
            groupAllowlist: nil,
            groupAllowlistSet: false,
            variable: "OTHER_VAR",
            expectedAllowed: false,
            expectedMode: InheritanceModeInherit,
        },
        {
            name: "explicit group allowlist - variable allowed",
            globalAllowlist: []string{"GLOBAL_VAR"},
            groupAllowlist: []string{"GROUP_VAR"},
            groupAllowlistSet: true,
            variable: "GROUP_VAR",
            expectedAllowed: true,
            expectedMode: InheritanceModeExplicit,
        },
        {
            name: "explicit group allowlist - global variable denied",
            globalAllowlist: []string{"GLOBAL_VAR"},
            groupAllowlist: []string{"GROUP_VAR"},
            groupAllowlistSet: true,
            variable: "GLOBAL_VAR",
            expectedAllowed: false,
            expectedMode: InheritanceModeExplicit,
        },
        {
            name: "explicit rejection - all variables denied",
            globalAllowlist: []string{"GLOBAL_VAR"},
            groupAllowlist: []string{},
            groupAllowlistSet: true,
            variable: "GLOBAL_VAR",
            expectedAllowed: false,
            expectedMode: InheritanceModeReject,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            config := &runnertypes.Config{
                Global: runnertypes.GlobalConfig{
                    EnvAllowlist: tt.globalAllowlist,
                },
            }

            filter := NewFilter(config)

            group := &runnertypes.CommandGroup{
                Name: "testgroup",
                EnvAllowlist: tt.groupAllowlist,
            }

            // Test mode determination
            mode, err := filter.determineInheritanceMode(group)
            assert.NoError(t, err)
            assert.Equal(t, tt.expectedMode, mode)

            // Test variable access
            allowed := filter.IsVariableAccessAllowed(tt.variable, group)
            assert.Equal(t, tt.expectedAllowed, allowed)
        })
    }
}
```

## 3. Phase 2: Command.Env の allowlist チェック除外

### 3.1 関数設計

#### 3.1.1 Command環境変数処理の分離

```go
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
// These variables are trusted as they come from the configuration file
func (p *CommandEnvProcessor) ProcessCommandEnvironment(
    cmd runnertypes.Command,
    baseEnvVars map[string]string,
    group *runnertypes.CommandGroup,
) (map[string]string, error) {

    // Create a copy of base environment variables to avoid modifying the original
    envVars := make(map[string]string, len(baseEnvVars))
    for k, v := range baseEnvVars {
        envVars[k] = v
    }

    // Process each Command.Env entry
    for i, env := range cmd.Env {
        variable, value, ok := environment.ParseEnvVariable(env)
        if !ok {
            p.logger.Warn("Invalid environment variable format in Command.Env",
                "command", cmd.Name,
                "env_index", i,
                "env_entry", env)
            continue
        }

        // Validate variable name and value (basic security checks)
        if err := p.filter.ValidateEnvironmentVariable(variable, value); err != nil {
            return nil, fmt.Errorf("invalid command environment variable %s in command %s: %w",
                variable, cmd.Name, err)
        }

        // Resolve variable references in the value
        // Note: References to system/env variables will still be subject to allowlist checks
        resolvedValue, err := p.resolveVariableReferencesForCommandEnv(value, envVars, group)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve variable references in %s for command %s: %w",
                variable, cmd.Name, err)
        }

        // Add the resolved variable to the environment
        envVars[variable] = resolvedValue

        p.logger.Debug("Processed command environment variable",
            "command", cmd.Name,
            "variable", variable,
            "value_length", len(resolvedValue))
    }

    return envVars, nil
}
```

#### 3.1.2 Command.Env専用の変数参照解決

```go
// VariableSource represents the source of a variable reference
type VariableSource int

const (
    VariableSourceCommandEnv VariableSource = iota // From Command.Env (trusted)
    VariableSourceSystem                          // From system environment (partial trust)
    VariableSourceEnvFile                         // From .env file (partial trust)
    VariableSourceUnknown                         // Unknown or not found
)

// resolveVariableReferencesForCommandEnv resolves variable references for Command.Env values
// This method applies different security policies based on the source of referenced variables
func (p *CommandEnvProcessor) resolveVariableReferencesForCommandEnv(
    value string,
    envVars map[string]string,
    group *runnertypes.CommandGroup,
) (string, error) {

    if !strings.Contains(value, "${") {
        // No variable references, return as-is
        return value, nil
    }

    result := value
    maxIterations := 10  // Prevent infinite loops
    var resolutionError error

    for i := 0; i < maxIterations && strings.Contains(result, "${"); i++ {
        oldResult := result

        // Find and replace variable references
        result = regexp.MustCompile(`\$\{([^}]+)\}`).ReplaceAllStringFunc(result, func(match string) string {
            // Extract variable name from ${VAR}
            varName := match[2 : len(match)-1] // Remove ${ and }

            // Resolve variable with security policy
            resolvedValue, err := p.resolveVariableWithSecurityPolicy(varName, envVars, group)
            if err != nil {
                // Store the first error encountered
                if resolutionError == nil {
                    resolutionError = fmt.Errorf("failed to resolve variable reference ${%s}: %w", varName, err)
                }
                return match // Return original match to continue processing other variables
            }

            return resolvedValue
        })

        // Break if no changes were made (avoid infinite loop)
        if result == oldResult {
            break
        }
    }

    // Return error if any variable resolution failed
    if resolutionError != nil {
        return "", resolutionError
    }

    return result, nil
}

// resolveVariableWithSecurityPolicy resolves a variable reference with appropriate security checks
// Returns an error if the variable cannot be resolved for any reason
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

    // Priority 2: Check system environment with allowlist validation (partial trust)
    if sysVal, exists := os.LookupEnv(varName); exists {
        return p.resolveSystemVariable(varName, sysVal, group)
    }

    // Priority 3: Variable not found - return error
    return "", fmt.Errorf("variable reference not found: %s", varName)
}

// resolveSystemVariable resolves a system environment variable with allowlist checks
// Returns an error if the variable is not allowed or if allowlist check fails
func (p *CommandEnvProcessor) resolveSystemVariable(
    varName, sysVal string,
    group *runnertypes.CommandGroup,
) (string, error) {
    // Apply allowlist check for system environment variables
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

### 3.2 Runner の変更

#### 3.2.1 resolveEnvironmentVars の改善

```go
// resolveEnvironmentVars resolves environment variables for a command with group context
// This method now separates system/env file processing from Command.Env processing
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
    processor := NewCommandEnvProcessor(r.envFilter)
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

### 3.3 テスト設計

#### 3.3.1 Command.Env 除外テストケース

```go
func TestCommandEnvAllowlistExclusion(t *testing.T) {
    tests := []struct {
        name            string
        globalAllowlist []string
        groupAllowlist  []string
        commandEnv      []string
        systemEnv       map[string]string
        expectedVars    map[string]string
        expectError     bool
    }{
        {
            name: "Command.Env variables bypass allowlist",
            globalAllowlist: []string{"ALLOWED_VAR"},
            groupAllowlist: []string{"ALLOWED_VAR"},
            commandEnv: []string{
                "NOT_IN_ALLOWLIST=command_value",
                "ALSO_NOT_ALLOWED=another_value",
            },
            systemEnv: map[string]string{
                "ALLOWED_VAR": "system_value",
                "NOT_IN_ALLOWLIST": "system_value_ignored",
            },
            expectedVars: map[string]string{
                "ALLOWED_VAR": "system_value",
                "NOT_IN_ALLOWLIST": "command_value",
                "ALSO_NOT_ALLOWED": "another_value",
            },
            expectError: false,
        },
        {
            name: "Command.Env with variable references respects allowlist for references",
            globalAllowlist: []string{"ALLOWED_VAR"},
            groupAllowlist: nil, // Inherit global
            commandEnv: []string{
                "COMMAND_VAR=${ALLOWED_VAR}_suffix",
            },
            systemEnv: map[string]string{
                "ALLOWED_VAR": "allowed_value",
            },
            expectedVars: map[string]string{
                "ALLOWED_VAR": "allowed_value",
                "COMMAND_VAR": "allowed_value_suffix",
            },
            expectError: false,
        },
        {
            name: "Command.Env with disallowed variable reference should error",
            globalAllowlist: []string{"ALLOWED_VAR"},
            groupAllowlist: nil, // Inherit global
            commandEnv: []string{
                "COMMAND_VAR=${NOT_ALLOWED_VAR}_suffix",
            },
            systemEnv: map[string]string{
                "ALLOWED_VAR": "allowed_value",
                "NOT_ALLOWED_VAR": "not_allowed_value",
            },
            expectError: true, // Should fail due to disallowed variable reference
        },
        {
            name: "Command.Env with undefined variable reference should error",
            globalAllowlist: []string{"ALLOWED_VAR"},
            groupAllowlist: nil,
            commandEnv: []string{
                "COMMAND_VAR=${UNDEFINED_VAR}_suffix",
            },
            systemEnv: map[string]string{
                "ALLOWED_VAR": "allowed_value",
            },
            expectError: true, // Should fail due to undefined variable reference
        },
        {
            name: "Invalid Command.Env variable name should error",
            globalAllowlist: []string{},
            groupAllowlist: []string{},
            commandEnv: []string{
                "123INVALID=value",
            },
            systemEnv: map[string]string{},
            expectError: true,
        },
        {
            name: "Dangerous Command.Env variable value should error",
            globalAllowlist: []string{},
            groupAllowlist: []string{},
            commandEnv: []string{
                "DANGEROUS_VAR=rm -rf /",
            },
            systemEnv: map[string]string{},
            expectError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            config := &runnertypes.Config{
                Global: runnertypes.GlobalConfig{
                    EnvAllowlist: tt.globalAllowlist,
                },
            }

            group := &runnertypes.CommandGroup{
                Name: "testgroup",
                EnvAllowlist: tt.groupAllowlist,
            }

            cmd := runnertypes.Command{
                Name: "testcmd",
                Env: tt.commandEnv,
            }

            // Mock system environment
            for k, v := range tt.systemEnv {
                t.Setenv(k, v)
            }

            // Create runner and filter
            filter := NewFilter(config)
            runner := &Runner{
                envFilter: filter,
                envVars: make(map[string]string), // Empty .env vars for this test
            }

            // Test
            result, err := runner.resolveEnvironmentVars(cmd, group)

            if tt.expectError {
                assert.Error(t, err)
                return
            }

            assert.NoError(t, err)

            // Verify expected variables are present with correct values
            for expectedVar, expectedValue := range tt.expectedVars {
                actualValue, exists := result[expectedVar]
                assert.True(t, exists, "Expected variable %s to exist", expectedVar)
                assert.Equal(t, expectedValue, actualValue, "Expected variable %s to have value %s, got %s", expectedVar, expectedValue, actualValue)
            }
        })
    }
}
```

## 4. Phase 3: 設定検証機能の追加

### 4.1 ConfigValidator の設計

#### 4.1.1 バリデータ構造体

```go
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

#### 4.1.2 主要検証関数

```go
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

// validateGlobalConfig validates the global configuration section
func (v *ConfigValidator) validateGlobalConfig(global *runnertypes.GlobalConfig, result *ValidationResult) error {
    // Validate global allowlist
    if err := v.validateAllowlist(global.EnvAllowlist, "global.env_allowlist", result); err != nil {
        return fmt.Errorf("failed to validate global allowlist: %w", err)
    }

    // Check for common issues
    if len(global.EnvAllowlist) == 0 {
        result.Warnings = append(result.Warnings, ValidationWarning{
            Type:       "empty_global_allowlist",
            Message:    "Global allowlist is empty, all environment variables will be blocked by default",
            Location:   "global.env_allowlist",
            Suggestion: "Consider adding common environment variables like PATH, HOME, USER",
        })
    }

    return nil
}

// validateGroup validates a single command group
func (v *ConfigValidator) validateGroup(
    group *runnertypes.CommandGroup,
    index int,
    global *runnertypes.GlobalConfig,
    result *ValidationResult,
) error {
    location := fmt.Sprintf("groups[%d]", index)

    // Validate group name
    if group.Name == "" {
        result.Errors = append(result.Errors, ValidationError{
            Type:     "empty_group_name",
            Message:  "Group name cannot be empty",
            Location: fmt.Sprintf("%s.name", location),
            Severity: "error",
        })
    }

    // Validate group allowlist
    if group.EnvAllowlist != nil {
        allowlistLocation := fmt.Sprintf("%s.env_allowlist", location)
        if err := v.validateAllowlist(group.EnvAllowlist, allowlistLocation, result); err != nil {
            return fmt.Errorf("failed to validate group allowlist: %w", err)
        }

        // Check allowlist consistency with global
        v.checkAllowlistConsistency(group.EnvAllowlist, global.EnvAllowlist, allowlistLocation, result)
    }

    // Validate commands
    for cmdIndex, cmd := range group.Commands {
        cmdLocation := fmt.Sprintf("%s.commands[%d]", location, cmdIndex)
        if err := v.validateCommand(&cmd, cmdLocation, result); err != nil {
            return fmt.Errorf("failed to validate command %s: %w", cmd.Name, err)
        }
    }

    return nil
}

// validateAllowlist validates an allowlist for common issues
func (v *ConfigValidator) validateAllowlist(allowlist []string, location string, result *ValidationResult) error {
    seen := make(map[string]bool)

    for i, variable := range allowlist {
        varLocation := fmt.Sprintf("%s[%d]", location, i)

        // Check for empty variable names
        if variable == "" {
            result.Errors = append(result.Errors, ValidationError{
                Type:     "empty_variable_name",
                Message:  "Environment variable name cannot be empty",
                Location: varLocation,
                Severity: "error",
            })
            continue
        }

        // Check for duplicate entries
        if seen[variable] {
            result.Warnings = append(result.Warnings, ValidationWarning{
                Type:       "duplicate_allowlist_entry",
                Message:    fmt.Sprintf("Duplicate allowlist entry: %s", variable),
                Location:   varLocation,
                Suggestion: "Remove duplicate entries to avoid confusion",
            })
        }
        seen[variable] = true

        // Validate variable name format
        if err := validateVariableNameFormat(variable); err != nil {
            result.Warnings = append(result.Warnings, ValidationWarning{
                Type:       "invalid_variable_name_format",
                Message:    fmt.Sprintf("Variable name '%s' may be invalid: %v", variable, err),
                Location:   varLocation,
                Suggestion: "Use valid environment variable name format (letters, digits, underscores)",
            })
        }
    }

    return nil
}

// checkAllowlistConsistency checks for consistency issues between group and global allowlists
func (v *ConfigValidator) checkAllowlistConsistency(
    groupAllowlist []string,
    globalAllowlist []string,
    location string,
    result *ValidationResult,
) {
    if len(groupAllowlist) == 0 {
        result.Warnings = append(result.Warnings, ValidationWarning{
            Type:       "explicit_allowlist_rejection",
            Message:    "Group explicitly rejects all environment variables (empty allowlist)",
            Location:   location,
            Suggestion: "Consider inheriting from global allowlist by omitting env_allowlist field",
        })
        return
    }

    // Check for common variables that might be missing
    globalSet := make(map[string]bool)
    for _, v := range globalAllowlist {
        globalSet[v] = true
    }

    commonVars := []string{"PATH", "HOME", "USER", "SHELL"}
    for _, commonVar := range commonVars {
        if globalSet[commonVar] && !slices.Contains(groupAllowlist, commonVar) {
            result.Warnings = append(result.Warnings, ValidationWarning{
                Type:       "missing_common_variable",
                Message:    fmt.Sprintf("Common variable '%s' is in global allowlist but not in group allowlist", commonVar),
                Location:   location,
                Suggestion: fmt.Sprintf("Consider adding '%s' to group allowlist or inherit from global", commonVar),
            })
        }
    }
}

// validateCommand validates a single command
func (v *ConfigValidator) validateCommand(cmd *runnertypes.Command, location string, result *ValidationResult) error {
    // Validate command name
    if cmd.Name == "" {
        result.Errors = append(result.Errors, ValidationError{
            Type:     "empty_command_name",
            Message:  "Command name cannot be empty",
            Location: fmt.Sprintf("%s.name", location),
            Severity: "error",
        })
    }

    // Validate command executable
    if cmd.Cmd == "" {
        result.Errors = append(result.Errors, ValidationError{
            Type:     "empty_command_executable",
            Message:  "Command executable cannot be empty",
            Location: fmt.Sprintf("%s.cmd", location),
            Severity: "error",
        })
    }

    // Validate command environment variables
    for envIndex, env := range cmd.Env {
        envLocation := fmt.Sprintf("%s.env[%d]", location, envIndex)
        if err := v.validateCommandEnvEntry(env, envLocation, result); err != nil {
            return fmt.Errorf("failed to validate command env entry: %w", err)
        }
    }

    return nil
}

// validateCommandEnvEntry validates a single command environment variable entry
func (v *ConfigValidator) validateCommandEnvEntry(env, location string, result *ValidationResult) error {
    variable, value, ok := environment.ParseEnvVariable(env)
    if !ok {
        result.Errors = append(result.Errors, ValidationError{
            Type:     "invalid_env_format",
            Message:  fmt.Sprintf("Invalid environment variable format: %s", env),
            Location: location,
            Severity: "error",
        })
        return nil
    }

    // Basic validation of variable name
    if err := validateVariableNameFormat(variable); err != nil {
        result.Warnings = append(result.Warnings, ValidationWarning{
            Type:       "invalid_command_env_name",
            Message:    fmt.Sprintf("Command environment variable name may be invalid: %v", err),
            Location:   location,
            Suggestion: "Use valid environment variable name format",
        })
    }

    // Check for variable references
    if strings.Contains(value, "${") {
        result.Warnings = append(result.Warnings, ValidationWarning{
            Type:       "command_env_variable_reference",
            Message:    fmt.Sprintf("Command environment variable '%s' contains variable reference", variable),
            Location:   location,
            Suggestion: "Ensure referenced variables are available and allowed",
        })
    }

    return nil
}

// calculateSummary calculates the validation summary
func (v *ConfigValidator) calculateSummary(config *runnertypes.Config, result *ValidationResult) {
    result.Summary.TotalGroups = len(config.Groups)
    result.Summary.GlobalAllowlistSize = len(config.Global.EnvAllowlist)

    for _, group := range config.Groups {
        if group.EnvAllowlist != nil {
            result.Summary.GroupsWithAllowlist++
        }

        result.Summary.TotalCommands += len(group.Commands)
        for _, cmd := range group.Commands {
            if len(cmd.Env) > 0 {
                result.Summary.CommandsWithEnv++
            }
        }
    }
}

// Helper function to validate variable name format
func validateVariableNameFormat(name string) error {
    if name == "" {
        return errors.New("empty name")
    }

    // First character must be letter or underscore
    if !isLetter(rune(name[0])) && name[0] != '_' {
        return errors.New("must start with letter or underscore")
    }

    // Rest must be letters, digits, or underscores
    for _, char := range name[1:] {
        if !isLetter(char) && !isDigit(char) && char != '_' {
            return errors.New("contains invalid character")
        }
    }

    return nil
}

func isLetter(r rune) bool {
    return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isDigit(r rune) bool {
    return r >= '0' && r <= '9'
}
```

### 4.2 検証レポート機能

#### 4.2.1 レポート生成

```go
// GenerateValidationReport generates a human-readable validation report
func (v *ConfigValidator) GenerateValidationReport(result *ValidationResult) (string, error) {
    var buf bytes.Buffer

    // Header
    fmt.Fprintf(&buf, "Configuration Validation Report\n")
    fmt.Fprintf(&buf, "Generated: %s\n", result.Timestamp.Format(time.RFC3339))
    fmt.Fprintf(&buf, "Overall Status: %s\n", v.getStatusString(result.Valid))
    fmt.Fprintf(&buf, "\n")

    // Summary
    fmt.Fprintf(&buf, "Summary:\n")
    fmt.Fprintf(&buf, "  Total Groups: %d\n", result.Summary.TotalGroups)
    fmt.Fprintf(&buf, "  Groups with Allowlist: %d\n", result.Summary.GroupsWithAllowlist)
    fmt.Fprintf(&buf, "  Global Allowlist Size: %d\n", result.Summary.GlobalAllowlistSize)
    fmt.Fprintf(&buf, "  Total Commands: %d\n", result.Summary.TotalCommands)
    fmt.Fprintf(&buf, "  Commands with Environment: %d\n", result.Summary.CommandsWithEnv)
    fmt.Fprintf(&buf, "\n")

    // Errors
    if len(result.Errors) > 0 {
        fmt.Fprintf(&buf, "Errors (%d):\n", len(result.Errors))
        for i, err := range result.Errors {
            fmt.Fprintf(&buf, "  %d. [%s] %s\n", i+1, err.Location, err.Message)
        }
        fmt.Fprintf(&buf, "\n")
    }

    // Warnings
    if len(result.Warnings) > 0 {
        fmt.Fprintf(&buf, "Warnings (%d):\n", len(result.Warnings))
        for i, warn := range result.Warnings {
            fmt.Fprintf(&buf, "  %d. [%s] %s\n", i+1, warn.Location, warn.Message)
            if warn.Suggestion != "" {
                fmt.Fprintf(&buf, "     Suggestion: %s\n", warn.Suggestion)
            }
        }
        fmt.Fprintf(&buf, "\n")
    }

    if result.Valid {
        fmt.Fprintf(&buf, "Configuration is valid and ready for use.\n")
    } else {
        fmt.Fprintf(&buf, "Configuration has errors that must be fixed before use.\n")
    }

    return buf.String(), nil
}

func (v *ConfigValidator) getStatusString(valid bool) string {
    if valid {
        return "VALID"
    }
    return "INVALID"
}
```

### 4.3 テスト設計

#### 4.3.1 設定検証テストケース

```go
func TestConfigValidation(t *testing.T) {
    tests := []struct {
        name               string
        config             *runnertypes.Config
        expectedValid      bool
        expectedErrorCount int
        expectedWarnCount  int
        expectedErrorTypes []string
    }{
        {
            name: "valid configuration",
            config: &runnertypes.Config{
                Global: runnertypes.GlobalConfig{
                    EnvAllowlist: []string{"PATH", "HOME"},
                },
                Groups: []runnertypes.CommandGroup{
                    {
                        Name: "valid-group",
                        EnvAllowlist: []string{"PATH", "HOME", "USER"},
                        Commands: []runnertypes.Command{
                            {
                                Name: "valid-cmd",
                                Cmd:  "echo",
                                Env:  []string{"MY_VAR=value"},
                            },
                        },
                    },
                },
            },
            expectedValid:      true,
            expectedErrorCount: 0,
            expectedWarnCount:  0,
        },
        {
            name: "invalid configuration with multiple errors",
            config: &runnertypes.Config{
                Global: runnertypes.GlobalConfig{
                    EnvAllowlist: []string{"PATH", "", "PATH"}, // Empty and duplicate
                },
                Groups: []runnertypes.CommandGroup{
                    {
                        Name: "", // Empty name
                        Commands: []runnertypes.Command{
                            {
                                Name: "", // Empty name
                                Cmd:  "", // Empty cmd
                                Env:  []string{"INVALID=FORMAT=VALUE"}, // Invalid format
                            },
                        },
                    },
                },
            },
            expectedValid:      false,
            expectedErrorCount: 4,
            expectedWarnCount:  1,
            expectedErrorTypes: []string{
                "empty_variable_name",
                "empty_group_name",
                "empty_command_name",
                "empty_command_executable",
            },
        },
        {
            name: "explicit rejection configuration",
            config: &runnertypes.Config{
                Global: runnertypes.GlobalConfig{
                    EnvAllowlist: []string{"PATH", "HOME", "USER"},
                },
                Groups: []runnertypes.CommandGroup{
                    {
                        Name: "restricted-group",
                        EnvAllowlist: []string{}, // Explicit rejection
                        Commands: []runnertypes.Command{
                            {
                                Name: "restricted-cmd",
                                Cmd:  "echo",
                            },
                        },
                    },
                },
            },
            expectedValid:      true,
            expectedErrorCount: 0,
            expectedWarnCount:  1, // Warning about explicit rejection
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            validator := NewConfigValidator()
            result, err := validator.ValidateConfig(tt.config)

            assert.NoError(t, err)
            assert.Equal(t, tt.expectedValid, result.Valid)
            assert.Equal(t, tt.expectedErrorCount, len(result.Errors))
            assert.Equal(t, tt.expectedWarnCount, len(result.Warnings))

            // Check specific error types if provided
            if len(tt.expectedErrorTypes) > 0 {
                errorTypes := make([]string, len(result.Errors))
                for i, err := range result.Errors {
                    errorTypes[i] = err.Type
                }
                assert.ElementsMatch(t, tt.expectedErrorTypes, errorTypes)
            }
        })
    }
}
```

## 5. 統合とテスト

### 5.1 統合戦略

#### 5.1.1 段階的統合

```
Phase 1 Complete → Phase 1 Testing → Phase 2 Development
     ↓
Phase 2 Complete → Phase 2 Testing → Phase 3 Development
     ↓
Phase 3 Complete → Phase 3 Testing → Integration Testing
     ↓
Full Integration → End-to-end Testing → Deployment
```

#### 5.1.2 統合テストシナリオ

```go
func TestFullAllowlistIntegration(t *testing.T) {
    // Test scenario that combines all three phases
    config := &runnertypes.Config{
        Global: runnertypes.GlobalConfig{
            EnvAllowlist: []string{"GLOBAL_VAR", "SHARED_VAR"},
        },
        Groups: []runnertypes.CommandGroup{
            {
                Name: "inherit-group",
                // EnvAllowlist: nil, // Inherits from global
                Commands: []runnertypes.Command{
                    {
                        Name: "inherit-cmd",
                        Cmd:  "echo",
                        Env:  []string{"COMMAND_VAR=${GLOBAL_VAR}_suffix"},
                    },
                },
            },
            {
                Name: "explicit-group",
                EnvAllowlist: []string{"GROUP_VAR"},
                Commands: []runnertypes.Command{
                    {
                        Name: "explicit-cmd",
                        Cmd:  "echo",
                        Env:  []string{"ANOTHER_VAR=direct_value"},
                    },
                },
            },
            {
                Name: "reject-group",
                EnvAllowlist: []string{}, // Explicit rejection
                Commands: []runnertypes.Command{
                    {
                        Name: "reject-cmd",
                        Cmd:  "echo",
                        Env:  []string{"SAFE_VAR=safe_value"},
                    },
                },
            },
        },
    }

    // Phase 3: Validate configuration
    validator := NewConfigValidator()
    validationResult, err := validator.ValidateConfig(config)
    assert.NoError(t, err)
    assert.True(t, validationResult.Valid)

    // Setup environment
    os.Setenv("GLOBAL_VAR", "global_value")
    os.Setenv("GROUP_VAR", "group_value")
    os.Setenv("FORBIDDEN_VAR", "forbidden_value")

    // Create runner with improved filter
    filter := NewFilter(config)
    runner := &Runner{
        config:    config,
        envFilter: filter,
        envVars:   make(map[string]string),
    }

    // Test inherit-group (should inherit global allowlist)
    inheritGroup := &config.Groups[0]
    inheritCmd := inheritGroup.Commands[0]
    inheritEnv, err := runner.resolveEnvironmentVars(inheritCmd, inheritGroup)
    assert.NoError(t, err)

    // Should have global variables and command variable
    assert.Equal(t, "global_value", inheritEnv["GLOBAL_VAR"])
    assert.Equal(t, "global_value_suffix", inheritEnv["COMMAND_VAR"])
    assert.NotContains(t, inheritEnv, "FORBIDDEN_VAR")

    // Test explicit-group (should use only group allowlist)
    explicitGroup := &config.Groups[1]
    explicitCmd := explicitGroup.Commands[0]
    explicitEnv, err := runner.resolveEnvironmentVars(explicitCmd, explicitGroup)
    assert.NoError(t, err)

    // Should have only group variables and command variable
    assert.Equal(t, "group_value", explicitEnv["GROUP_VAR"])
    assert.Equal(t, "direct_value", explicitEnv["ANOTHER_VAR"])
    assert.NotContains(t, explicitEnv, "GLOBAL_VAR") // Not in group allowlist
    assert.NotContains(t, explicitEnv, "FORBIDDEN_VAR")

    // Test reject-group (should reject all system variables)
    rejectGroup := &config.Groups[2]
    rejectCmd := rejectGroup.Commands[0]
    rejectEnv, err := runner.resolveEnvironmentVars(rejectCmd, rejectGroup)
    assert.NoError(t, err)

    // Should have only command variable
    assert.Equal(t, "safe_value", rejectEnv["SAFE_VAR"])
    assert.NotContains(t, rejectEnv, "GLOBAL_VAR")
    assert.NotContains(t, rejectEnv, "GROUP_VAR")
    assert.NotContains(t, rejectEnv, "FORBIDDEN_VAR")
}
```

### 5.2 パフォーマンステスト

```go
func BenchmarkAllowlistResolution(b *testing.B) {
    config := &runnertypes.Config{
        Global: runnertypes.GlobalConfig{
            EnvAllowlist: make([]string, 100), // Large allowlist
        },
        Groups: []runnertypes.CommandGroup{
            {
                Name: "test-group",
                EnvAllowlist: make([]string, 50),
            },
        },
    }

    // Initialize allowlist with dummy values
    for i := 0; i < 100; i++ {
        config.Global.EnvAllowlist[i] = fmt.Sprintf("GLOBAL_VAR_%d", i)
    }
    for i := 0; i < 50; i++ {
        config.Groups[0].EnvAllowlist[i] = fmt.Sprintf("GROUP_VAR_%d", i)
    }

    filter := NewFilter(config)
    group := &config.Groups[0]

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Test the most common operation
        _ = filter.IsVariableAccessAllowed("GROUP_VAR_25", group)
    }
}
```

## 6. セキュリティアーキテクチャ詳細

### 6.1 信頼度レベル別セキュリティモデル

#### 6.1.1 セキュリティ境界の定義

```go
// SecurityLevel represents the trust level of variable sources
type SecurityLevel int

const (
    SecurityLevelTrusted SecurityLevel = iota  // Command.Env, config file
    SecurityLevelPartial                       // System env, .env file
    SecurityLevelUntrusted                     // External input
)

// VariableSecurityContext contains security information for variable resolution
type VariableSecurityContext struct {
    VariableName    string
    Source          VariableSource
    SecurityLevel   SecurityLevel
    AllowlistCheck  bool
    ValidationCheck bool
}
```

#### 6.1.2 セキュリティポリシーマトリックス

```
┌─────────────────┬──────────────┬─────────────┬───────────────┐
│ Variable Source │ Trust Level  │ Allowlist   │ Validation    │
├─────────────────┼──────────────┼─────────────┼───────────────┤
│ Command.Env     │ Trusted      │ No          │ Basic only    │
│ System Env      │ Partial      │ Yes         │ Full          │
│ .env File       │ Partial      │ Yes         │ Full          │
│ External Input  │ Untrusted    │ Yes         │ Strict        │
└─────────────────┴──────────────┴─────────────┴───────────────┘
```

#### 6.1.3 変数参照時のセキュリティ制御

```go
// applySecurityPolicy applies appropriate security controls based on variable source
func (p *CommandEnvProcessor) applySecurityPolicy(
    ctx VariableSecurityContext,
    value string,
    group *runnertypes.CommandGroup,
) (string, error) {

    switch ctx.SecurityLevel {
    case SecurityLevelTrusted:
        // Command.Env variables: basic validation only
        if ctx.ValidationCheck {
            return p.validateTrustedVariable(ctx.VariableName, value)
        }
        return value, nil

    case SecurityLevelPartial:
        // System/env file variables: allowlist + validation
        if ctx.AllowlistCheck {
            allowed, err := p.filter.resolveAllowedVariable(ctx.VariableName, group)
            if err != nil {
                return "", fmt.Errorf("allowlist check failed: %w", err)
            }
            if !allowed {
                return "", fmt.Errorf("variable not allowed: %s", ctx.VariableName)
            }
        }
        if ctx.ValidationCheck {
            return p.validatePartialTrustVariable(ctx.VariableName, value)
        }
        return value, nil

    case SecurityLevelUntrusted:
        // External input: strict validation
        return p.validateUntrustedVariable(ctx.VariableName, value)

    default:
        return "", fmt.Errorf("unknown security level: %v", ctx.SecurityLevel)
    }
}
```

### 6.2 防御的プログラミング原則

#### 6.2.1 安全なデフォルト動作

```go
// Secure defaults for variable resolution
const (
    // When allowlist check fails, return error (fail-fast)
    DefaultAllowlistFailAction = "error"

    // When validation fails, return error (fail-fast)
    DefaultValidationFailAction = "error"

    // When variable reference not found, return error (fail-fast)
    DefaultReferenceNotFoundAction = "error"

    // Maximum variable reference depth to prevent DoS
    MaxVariableReferenceDepth = 10

    // Maximum variable name length to prevent memory exhaustion
    MaxVariableNameLength = 256
)

// resolveWithSecureDefaults applies secure defaults throughout resolution
func (p *CommandEnvProcessor) resolveWithSecureDefaults(
    varName, value string,
    group *runnertypes.CommandGroup,
) (string, error) {

    // Input validation with secure limits
    if len(varName) > MaxVariableNameLength {
        return "", fmt.Errorf("variable name too long: %d > %d", len(varName), MaxVariableNameLength)
    }

    // Apply security policy with fallback
    ctx := VariableSecurityContext{
        VariableName:    varName,
        Source:          p.determineVariableSource(varName),
        SecurityLevel:   p.determineSecurityLevel(varName),
        AllowlistCheck:  true,
        ValidationCheck: true,
    }

    result, err := p.applySecurityPolicy(ctx, value, group)
    if err != nil {
        // Fail-fast: return error immediately for security violations
        return "", fmt.Errorf("security policy violation for variable %s: %w", varName, err)
    }

    return result, nil
}
```

#### 6.2.2 入力検証とサニタイゼーション

```go
// validateTrustedVariable performs basic validation for trusted sources
func (p *CommandEnvProcessor) validateTrustedVariable(name, value string) (string, error) {
    // Even trusted sources get basic validation
    if err := p.validateVariableName(name); err != nil {
        return "", fmt.Errorf("invalid variable name: %w", err)
    }

    // Check for obviously dangerous patterns
    if p.containsDangerousPattern(value) {
        return "", fmt.Errorf("dangerous pattern detected in trusted variable: %s", name)
    }

    return value, nil
}

// validatePartialTrustVariable performs enhanced validation for partial trust sources
func (p *CommandEnvProcessor) validatePartialTrustVariable(name, value string) (string, error) {
    // Full name validation
    if err := p.validateVariableName(name); err != nil {
        return "", fmt.Errorf("invalid variable name: %w", err)
    }

    // Enhanced value validation
    if err := p.validateVariableValue(value); err != nil {
        return "", fmt.Errorf("invalid variable value: %w", err)
    }

    // Check for injection patterns
    if p.containsInjectionPattern(value) {
        return "", fmt.Errorf("potential injection pattern in variable: %s", name)
    }

    return value, nil
}
```

## 7. エラーハンドリング詳細

### 7.1 エラー分類と対応

```go
// New error types for the improved implementation
var (
    // Configuration-related errors
    ErrInvalidInheritanceMode = errors.New("invalid inheritance mode")
    ErrAllowlistResolution    = errors.New("failed to resolve allowlist configuration")
    ErrConfigValidation       = errors.New("configuration validation failed")

    // Runtime errors
    ErrCommandEnvProcessing   = errors.New("command environment processing failed")
    ErrVariableReference      = errors.New("variable reference resolution failed")
    ErrVariableNotFound       = errors.New("variable reference not found")
    ErrCircularReference      = errors.New("circular reference detected")

    // Security-related errors
    ErrSecurityPolicyViolation = errors.New("security policy violation")
    ErrVariableNotAllowed      = errors.New("variable not allowed by allowlist")
    ErrDangerousPattern        = errors.New("dangerous pattern detected")
    ErrInjectionAttempt        = errors.New("injection attempt detected")
)
```

### 7.2 エラー処理パターン

```go
// Graceful degradation for non-critical errors
func (f *Filter) resolveAllowedVariableWithFallback(variable string, group *runnertypes.CommandGroup) bool {
    allowed, err := f.resolveAllowedVariable(variable, group)
    if err != nil {
        // Log error but don't fail the operation
        slog.Error("Failed to resolve variable allowlist, falling back to denial",
            "variable", variable,
            "group", group.Name,
            "error", err)
        return false // Secure default: deny access
    }
    return allowed
}

// Detailed error context for debugging
func (p *CommandEnvProcessor) wrapProcessingError(err error, cmd runnertypes.Command, variable string) error {
    return fmt.Errorf("command environment processing failed: command=%s, variable=%s: %w",
        cmd.Name, variable, err)
}
```

## 8. ログとデバッグ

### 8.1 構造化ログ設計

```go
// Standardized logging with consistent structure
func (f *Filter) logAllowlistResolution(resolution *AllowlistResolution, variable string, allowed bool) {
    slog.Debug("Variable allowlist resolution",
        "variable", variable,
        "group", resolution.GroupName,
        "inheritance_mode", resolution.Mode.String(),
        "allowed", allowed,
        "group_allowlist_size", len(resolution.GroupAllowlist),
        "global_allowlist_size", len(resolution.GlobalAllowlist),
        "effective_allowlist_size", len(resolution.EffectiveList))
}

// Performance logging
func (p *CommandEnvProcessor) logProcessingStats(cmd runnertypes.Command, startTime time.Time, envCount int) {
    duration := time.Since(startTime)
    slog.Debug("Command environment processing completed",
        "command", cmd.Name,
        "processing_duration_ms", duration.Milliseconds(),
        "env_vars_processed", len(cmd.Env),
        "final_env_count", envCount)
}
```

### 8.2 デバッグ支援機能

```go
// Debug dump functionality
func (f *Filter) DumpAllowlistConfiguration(group *runnertypes.CommandGroup) (string, error) {
    resolution, err := f.resolveAllowlistConfiguration(group)
    if err != nil {
        return "", err
    }

    var buf bytes.Buffer
    fmt.Fprintf(&buf, "Allowlist Configuration for Group: %s\n", resolution.GroupName)
    fmt.Fprintf(&buf, "Inheritance Mode: %s\n", resolution.Mode.String())
    fmt.Fprintf(&buf, "Group Allowlist: %v\n", resolution.GroupAllowlist)
    fmt.Fprintf(&buf, "Global Allowlist: %v\n", resolution.GlobalAllowlist)
    fmt.Fprintf(&buf, "Effective Allowlist: %v\n", resolution.EffectiveList)

    return buf.String(), nil
}
```

## 9. 移行とデプロイメント

### 9.1 移行チェックリスト

```markdown
Phase 1 完了後:
- [ ] 継承ロジックの単体テストがすべてパス
- [ ] 既存の統合テストがすべてパス
- [ ] パフォーマンステストで劣化がないことを確認
- [ ] ログ出力の検証

Phase 2 完了後:
- [ ] Command.Env除外の単体テストがすべてパス
- [ ] Command.Env変数が期待通り処理されることを確認
- [ ] セキュリティテストで基本検証が維持されることを確認
- [ ] 既存のCommand.Env使用例が動作することを確認

Phase 3 完了後:
- [ ] 設定検証の単体テストがすべてパス
- [ ] 各種設定パターンで適切な検証が行われることを確認
- [ ] パフォーマンスへの影響が許容範囲内であることを確認
- [ ] エラーメッセージの改善を確認
```

### 9.2 リリース準備

```markdown
技術的準備:
- [ ] すべてのテストがパス（単体、統合、パフォーマンス）
- [ ] コードカバレッジが目標値（80%）以上
- [ ] ドキュメントの更新（API、設定例、移行ガイド）
- [ ] 既存システムでの動作確認

運用準備:
- [ ] ログレベルの適切な設定
- [ ] 監視メトリクスの確認
- [ ] エラー処理の検証
- [ ] ロールバック計画の策定
```

## 10. 今後の拡張可能性

### 10.1 アーキテクチャの拡張性

設計により、以下の将来的な拡張が容易になる：

1. **動的設定リロード**: 設定検証機能を活用した動的な設定更新
2. **きめ細かい権限制御**: 変数ごと、コマンドごとの詳細な権限設定
3. **外部認証連携**: 外部システムとの連携による動的なallowlist生成
4. **監査ログ**: 環境変数アクセスの詳細な監査機能

### 10.2 保守性の確保

- **明確なインターフェース**: 各コンポーネント間の責任境界が明確
- **包括的なテスト**: 機能追加時のテストガイドラインの確立
- **構造化ログ**: 運用時の問題調査を容易にする一貫したログ構造
- **文書化**: 設計思想と実装の詳細な文書化

## 11. まとめ

この詳細設計書では、要件定義書で特定された3つの主要な問題に対する具体的な解決策を提供している：

1. **継承ロジックの明確化**: nil（継承）、empty（拒否）、values（明示）の3パターンを明確に区別
2. **Command.Env の適切な処理**: 設定ファイルの変数をallowlistチェックから除外し、参照先のみチェック
3. **設定検証の追加**: 包括的な設定検証とエラーの早期発見

各フェーズは独立して実装・テスト可能であり、既存システムへの影響を最小限に抑えながら段階的な改善が可能である。
