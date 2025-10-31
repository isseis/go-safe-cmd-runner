# 詳細設計書: Dry-Run Debug情報のJSON出力対応

## 1. ドキュメント概要

### 1.1 目的
本ドキュメントは、dry-runモードでJSON形式を指定した際のデバッグ情報出力機能の詳細な実装仕様を定義する。関数シグネチャ、エラーハンドリング、テストケースの詳細を含む。

### 1.2 対象読者
- 実装者
- コードレビュアー
- テスト担当者

### 1.3 関連ドキュメント
- [要件定義書](./01_requirements.ja.md)
- [アーキテクチャ設計書](./02_architecture.ja.md)
- [実装計画書](./04_implementation_plan.ja.md) - 今後作成予定

## 2. データ構造の詳細定義

### 2.1 新規構造体

#### 2.1.1 DebugInfo

**ファイル**: `internal/runner/resource/types.go`

```go
// DebugInfo contains debug information for dry-run analysis
// This is optional and only populated based on detail level
type DebugInfo struct {
	// InheritanceAnalysis contains environment variable inheritance information
	// Populated for DetailLevelDetailed and DetailLevelFull
	// Field content varies by detail level
	InheritanceAnalysis *InheritanceAnalysis `json:"inheritance_analysis,omitempty"`

	// FinalEnvironment contains the final resolved environment variables
	// Only populated for DetailLevelFull
	FinalEnvironment *FinalEnvironment `json:"final_environment,omitempty"`
}
```

#### 2.1.2 InheritanceAnalysis

**ファイル**: `internal/runner/resource/types.go`

```go
// InheritanceAnalysis contains detailed information about environment variable inheritance
type InheritanceAnalysis struct {
	// Configuration fields (always present when InheritanceAnalysis is not nil)
	GlobalEnvImport []string `json:"global_env_import"`
	GlobalAllowlist []string `json:"global_allowlist"`
	GroupEnvImport  []string `json:"group_env_import"`
	GroupAllowlist  []string `json:"group_allowlist"`

	// Computed field (always present when InheritanceAnalysis is not nil)
	InheritanceMode runnertypes.InheritanceMode `json:"inheritance_mode"`

	// Difference fields (only present for DetailLevelFull, omitempty otherwise)
	// Variables inherited from global configuration
	InheritedVariables []string `json:"inherited_variables,omitempty"`

	// Variables removed from global allowlist by group override
	RemovedAllowlistVariables []string `json:"removed_allowlist_variables,omitempty"`

	// Internal variables (from env_import) that become unavailable
	// when group overrides env_import
	UnavailableEnvImportVariables []string `json:"unavailable_env_import_variables,omitempty"`
}
```

#### 2.1.3 FinalEnvironment

**ファイル**: `internal/runner/resource/types.go`

```go
// FinalEnvironment contains the final resolved environment variables for a command
// Only populated for DetailLevelFull
type FinalEnvironment struct {
	Variables map[string]EnvironmentVariable `json:"variables"`
}

// EnvironmentVariable represents a single environment variable with metadata
type EnvironmentVariable struct {
	// Value of the environment variable
	// Only included when ShowSensitive is true, otherwise omitted
	Value string `json:"value,omitempty"`

	// Source indicates where this variable comes from:
	//   "system"     - from env_allowlist (system environment variable)
	//   "env_import" - from env_import mapping (originally system variable)
	//   "vars"       - from vars section
	//   "command"    - from command-level env_vars
	Source string `json:"source"`

	// Masked indicates whether the value was redacted for security
	// Only true when ShowSensitive is false and value contains sensitive data
	Masked bool `json:"masked,omitempty"`
}
```

### 2.2 既存構造体の拡張

#### 2.2.1 ResourceAnalysis

**ファイル**: `internal/runner/resource/types.go`

```go
// ResourceAnalysis represents analysis of a single resource operation
type ResourceAnalysis struct {
	Type       ResourceType      `json:"type"`
	Operation  ResourceOperation `json:"operation"`
	Target     string            `json:"target"`
	Impact     ResourceImpact    `json:"impact"`
	Timestamp  time.Time         `json:"timestamp"`
	Parameters map[string]any    `json:"parameters,omitempty"`

	// DebugInfo is optional and only populated based on dry-run detail level
	// 新規追加
	DebugInfo *DebugInfo `json:"debug_info,omitempty"`
}
```

#### 2.2.2 ResourceType の拡張

**ファイル**: `internal/runner/resource/types.go`

```go
// ResourceType represents the type of resource being analyzed
type ResourceType string

const (
	ResourceTypeCommand ResourceType = "command"
	ResourceTypeGroup   ResourceType = "group"    // 新規追加
	ResourceTypeFile    ResourceType = "file"
	// ... existing types ...
)
```

#### 2.2.3 ResourceOperation の拡張

**ファイル**: `internal/runner/resource/types.go`

```go
// ResourceOperation represents the operation being performed
type ResourceOperation string

const (
	OperationExecute ResourceOperation = "execute"
	OperationAnalyze ResourceOperation = "analyze"  // 新規追加
	OperationRead    ResourceOperation = "read"
	OperationWrite   ResourceOperation = "write"
	// ... existing operations ...
)
```

### 2.3 InheritanceMode の JSON 変換

#### 2.3.1 MarshalJSON メソッド

**ファイル**: `internal/runner/runnertypes/inheritance_mode.go`

```go
// MarshalJSON implements json.Marshaler interface
// Returns the string representation of InheritanceMode for JSON output
func (m InheritanceMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}
```

#### 2.3.2 UnmarshalJSON メソッド（将来の拡張性のため）

**ファイル**: `internal/runner/runnertypes/inheritance_mode.go`

**重要**: このメソッドは、将来JSON設定ファイルを読み込む機能を追加する場合に備えて実装する。現在のdry-run機能では、JSON出力のみを行い、JSON入力は受け付けないため、このメソッドが呼ばれることはない。

**セキュリティ上の考慮事項**:
- 不正な値を受け取った場合、エラーを返してグループ実行を中止する
- エラーメッセージには入力値を含めない（ログファイルへの機密情報漏洩を防ぐため）
- 将来的にJSON設定ファイルをサポートする場合、このメソッドが重要なバリデーション層となる

```go
// UnmarshalJSON implements json.Unmarshaler interface
// Parses string representation of InheritanceMode from JSON
//
// Security: This method validates that only known inheritance modes are accepted.
// Invalid values will cause the entire operation to fail, preventing execution
// with potentially dangerous or undefined configurations.
func (m *InheritanceMode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch s {
	case "inherit":
		*m = InheritanceModeInherit
	case "explicit":
		*m = InheritanceModeExplicit
	case "reject":
		*m = InheritanceModeReject
	default:
		// Security: Do not include the invalid value in the error message
		// to prevent potential log injection or information disclosure
		return fmt.Errorf("invalid inheritance mode value")
	}

	return nil
}
```

**エラーハンドリングの方針**:
1. **不正な値の検出**: 定義された3つの値（"inherit", "explicit", "reject"）以外はすべて拒否
2. **即座にエラーを返す**: グループやコマンドの実行を開始する前に失敗させる
3. **エラーメッセージから入力値を除外**: ログインジェクションや情報漏洩のリスクを軽減
4. **型安全性の維持**: 不正な値が`InheritanceMode`型として存在することを防ぐ

## 3. データ収集層の詳細設計

### 3.1 データ収集関数

#### 3.1.1 CollectInheritanceAnalysis

**ファイル**: `internal/runner/debug/collector.go` (新規作成)

```go
// CollectInheritanceAnalysis collects environment variable inheritance analysis information
// This function is the single source of truth for inheritance analysis data
// Returns nil for DetailLevelSummary
func CollectInheritanceAnalysis(
	runtimeGlobal *runnertypes.RuntimeGlobal,
	runtimeGroup *runnertypes.RuntimeGroup,
	detailLevel resource.DryRunDetailLevel,
) *resource.InheritanceAnalysis {
	// Return nil for summary level
	if detailLevel == resource.DetailLevelSummary {
		return nil
	}

	// Extract group spec safely
	groupSpec := runtimeGroup.GroupSpec
	if groupSpec == nil {
		groupSpec = &runnertypes.GroupSpec{}
	}

	// Build base analysis with configuration and computed fields
	analysis := &resource.InheritanceAnalysis{
		// Configuration fields from global
		GlobalEnvImport: safeStringSlice(runtimeGlobal.GlobalSpec.EnvImport),
		GlobalAllowlist: safeStringSlice(runtimeGlobal.GlobalSpec.EnvAllowed),

		// Configuration fields from group
		GroupEnvImport: safeStringSlice(groupSpec.EnvImport),
		GroupAllowlist: safeStringSlice(groupSpec.EnvAllowed),

		// Computed field
		InheritanceMode: runtimeGroup.InheritanceMode,
	}

	// Add difference fields only for DetailLevelFull
	if detailLevel == resource.DetailLevelFull {
		// Calculate inherited variables
		if runtimeGroup.InheritanceMode == runnertypes.InheritanceModeInherit {
			analysis.InheritedVariables = safeStringSlice(runtimeGlobal.GlobalSpec.EnvAllowed)
		}

		// Calculate removed allowlist variables
		if runtimeGroup.InheritanceMode == runnertypes.InheritanceModeExplicit ||
			runtimeGroup.InheritanceMode == runnertypes.InheritanceModeReject {
			globalSet := stringSliceToSet(runtimeGlobal.GlobalSpec.EnvAllowed)
			groupSet := stringSliceToSet(groupSpec.EnvAllowed)
			analysis.RemovedAllowlistVariables = setDifference(globalSet, groupSet)
		}

		// Calculate unavailable env_import variables
		if len(groupSpec.EnvImport) > 0 && len(runtimeGlobal.GlobalSpec.EnvImport) > 0 {
			globalVars := extractInternalVarNames(runtimeGlobal.GlobalSpec.EnvImport)
			groupVars := extractInternalVarNames(groupSpec.EnvImport)
			globalSet := stringSliceToSet(globalVars)
			groupSet := stringSliceToSet(groupVars)
			analysis.UnavailableEnvImportVariables = setDifference(globalSet, groupSet)
		}
	}

	return analysis
}

// Helper functions

// safeStringSlice returns a copy of the slice or an empty slice if nil
func safeStringSlice(slice []string) []string {
	if slice == nil {
		return []string{}
	}
	result := make([]string, len(slice))
	copy(result, slice)
	return result
}

// stringSliceToSet converts a string slice to a set (map[string]struct{})
func stringSliceToSet(slice []string) map[string]struct{} {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}
	return set
}

// setDifference returns elements in setA that are not in setB
func setDifference(setA, setB map[string]struct{}) []string {
	var result []string
	for key := range setA {
		if _, exists := setB[key]; !exists {
			result = append(result, key)
		}
	}
	sort.Strings(result) // Ensure deterministic output
	return result
}

// extractInternalVarNames extracts internal variable names from env_import mappings
// Example: "db_host=DB_HOST" -> "db_host"
func extractInternalVarNames(envImport []string) []string {
	var result []string
	for _, mapping := range envImport {
		parts := strings.SplitN(mapping, "=", 2)
		if len(parts) == 2 {
			result = append(result, parts[0])
		}
	}
	return result
}
```

#### 3.1.2 CollectFinalEnvironment

**ファイル**: `internal/runner/debug/collector.go` (新規作成)

```go
// CollectFinalEnvironment collects final resolved environment variables
// Returns nil for DetailLevelSummary and DetailLevelDetailed
func CollectFinalEnvironment(
	envMap map[string]executor.EnvVar,
	detailLevel resource.DryRunDetailLevel,
	showSensitive bool,
) *resource.FinalEnvironment {
	// Only collect for DetailLevelFull
	if detailLevel != resource.DetailLevelFull {
		return nil
	}

	finalEnv := &resource.FinalEnvironment{
		Variables: make(map[string]resource.EnvironmentVariable, len(envMap)),
	}

	// Create redactor for sensitive information
	redactor := redaction.NewRedactor(redaction.DefaultSensitivePatterns())

	for name, envVar := range envMap {
		variable := resource.EnvironmentVariable{
			Source: mapEnvVarSource(envVar.Source),
		}

		// Include value only if showSensitive is true
		if showSensitive {
			variable.Value = envVar.Value
		} else {
			// Check if variable is sensitive
			if redactor.IsSensitive(name) {
				variable.Value = "" // Omit value
				variable.Masked = true
			} else {
				variable.Value = envVar.Value
			}
		}

		finalEnv.Variables[name] = variable
	}

	return finalEnv
}

// mapEnvVarSource maps executor.EnvVarSource to string representation
func mapEnvVarSource(source executor.EnvVarSource) string {
	switch source {
	case executor.EnvVarSourceSystem:
		return "system"
	case executor.EnvVarSourceEnvImport:
		return "env_import"
	case executor.EnvVarSourceVars:
		return "vars"
	case executor.EnvVarSourceCommand:
		return "command"
	default:
		return "unknown"
	}
}
```

### 3.2 フォーマットヘルパー関数

#### 3.2.1 FormatInheritanceAnalysisText

**ファイル**: `internal/runner/debug/formatter.go` (新規作成)

```go
// FormatInheritanceAnalysisText formats InheritanceAnalysis as text output
// This reuses the logic from existing PrintFromEnvInheritance function
func FormatInheritanceAnalysisText(
	w io.Writer,
	analysis *resource.InheritanceAnalysis,
	groupName string,
) {
	if analysis == nil {
		return
	}

	// Header
	fmt.Fprintf(w, "\n%s\n\n", debugHeaderSeparator)
	fmt.Fprintf(w, "%s\n\n", inheritanceAnalysisHeader)

	// Global level
	fmt.Fprintf(w, "[Global Level]\n")
	fmt.Fprintf(w, "  env_import: %s\n", formatStringSlice(analysis.GlobalEnvImport))
	fmt.Fprintf(w, "  env_allowlist: %s\n", formatStringSlice(analysis.GlobalAllowlist))
	fmt.Fprintf(w, "\n")

	// Group level
	fmt.Fprintf(w, "[Group: %s]\n", groupName)
	fmt.Fprintf(w, "  env_import: %s\n", formatGroupField(
		analysis.GroupEnvImport,
		len(analysis.GlobalEnvImport) > 0,
		"Inheriting from Global",
	))
	fmt.Fprintf(w, "  env_allowlist: %s\n", formatGroupField(
		analysis.GroupAllowlist,
		len(analysis.GlobalAllowlist) > 0,
		"Inheriting from Global",
	))
	fmt.Fprintf(w, "\n")

	// Inheritance mode
	fmt.Fprintf(w, "[Inheritance Mode]\n")
	fmt.Fprintf(w, "  Mode: %s\n", analysis.InheritanceMode.String())
	fmt.Fprintf(w, "\n")

	// Difference fields (only if present)
	if len(analysis.InheritedVariables) > 0 {
		fmt.Fprintf(w, "[Inherited Variables]\n")
		for _, varName := range analysis.InheritedVariables {
			fmt.Fprintf(w, "  - %s\n", varName)
		}
		fmt.Fprintf(w, "\n")
	}

	if len(analysis.RemovedAllowlistVariables) > 0 {
		fmt.Fprintf(w, "[Removed Allowlist Variables]\n")
		for _, varName := range analysis.RemovedAllowlistVariables {
			fmt.Fprintf(w, "  - %s\n", varName)
		}
		fmt.Fprintf(w, "\n")
	}

	if len(analysis.UnavailableEnvImportVariables) > 0 {
		fmt.Fprintf(w, "[Unavailable Env Import Variables]\n")
		for _, varName := range analysis.UnavailableEnvImportVariables {
			fmt.Fprintf(w, "  - %s\n", varName)
		}
		fmt.Fprintf(w, "\n")
	}
}

// Helper functions

func formatStringSlice(slice []string) string {
	if len(slice) == 0 {
		return "not defined"
	}
	return strings.Join(slice, ", ")
}

func formatGroupField(groupValue []string, hasGlobal bool, inheritMsg string) string {
	if len(groupValue) == 0 {
		if hasGlobal {
			return inheritMsg
		}
		return "not defined"
	}
	return strings.Join(groupValue, ", ")
}
```

#### 3.2.2 FormatFinalEnvironmentText

**ファイル**: `internal/runner/debug/formatter.go` (新規作成)

```go
// FormatFinalEnvironmentText formats FinalEnvironment as text output
// This reuses the logic from existing PrintFinalEnvironment function
func FormatFinalEnvironmentText(
	w io.Writer,
	finalEnv *resource.FinalEnvironment,
) {
	if finalEnv == nil {
		return
	}

	fmt.Fprintf(w, "\n%s\n", finalEnvHeader)

	// Sort variable names for consistent output
	varNames := make([]string, 0, len(finalEnv.Variables))
	for name := range finalEnv.Variables {
		varNames = append(varNames, name)
	}
	sort.Strings(varNames)

	// Print each variable
	for _, name := range varNames {
		envVar := finalEnv.Variables[name]
		value := envVar.Value
		if envVar.Masked {
			value = "[REDACTED]"
		}
		fmt.Fprintf(w, "  %s=%s (source: %s)\n", name, value, envVar.Source)
	}
	fmt.Fprintf(w, "\n")
}
```

## 4. 実行層の詳細設計

### 4.1 GroupExecutor の変更

#### 4.1.1 デバッグ情報出力の条件分岐

**ファイル**: `internal/runner/group_executor.go`

変更箇所1: グループレベルの継承分析情報

```go
// 既存のコード（131-136行目付近）
if ge.isDryRun {
	_, _ = fmt.Fprintf(os.Stdout, "\n===== Variable Expansion Debug Information =====\n\n")
	debug.PrintFromEnvInheritance(os.Stdout, &ge.config.Global, groupSpec, runtimeGroup)
}

// 新しいコード
if ge.isDryRun {
	// Collect inheritance analysis data
	analysis := debug.CollectInheritanceAnalysis(
		&ge.config.Global,
		runtimeGroup,
		ge.dryRunDetailLevel,
	)

	// Format based on output format
	if ge.dryRunFormat == "json" {
		// Record to ResourceManager for JSON output
		debugInfo := &resource.DebugInfo{
			InheritanceAnalysis: analysis,
		}
		err := ge.resourceManager.RecordGroupAnalysis(groupSpec.Name, debugInfo)
		if err != nil {
			// Log error but continue execution
			// Note: Use slog.Warn with structured logging, never include sensitive data
			ge.logger.Warn("Failed to record group analysis",
				"error", err,
				"group", groupSpec.Name,
				"detail_level", ge.dryRunDetailLevel.String())
		}
	} else {
		// Text format: output immediately
		_, _ = fmt.Fprintf(os.Stdout, "\n===== Variable Expansion Debug Information =====\n\n")
		debug.FormatInheritanceAnalysisText(os.Stdout, analysis, groupSpec.Name)
	}
}
```

変更箇所2: コマンドレベルの最終環境変数

```go
// 既存のコード（286-288行目付近）
if ge.isDryRun && ge.dryRunDetailLevel == resource.DetailLevelFull {
	debug.PrintFinalEnvironment(os.Stdout, envMap, ge.dryRunShowSensitive)
}

// 新しいコード
if ge.isDryRun {
	// Collect final environment data
	finalEnv := debug.CollectFinalEnvironment(
		envMap,
		ge.dryRunDetailLevel,
		ge.dryRunShowSensitive,
	)

	if finalEnv != nil {
		if ge.dryRunFormat == "json" {
			// Update the command's ResourceAnalysis with debug info
			debugInfo := &resource.DebugInfo{
				FinalEnvironment: finalEnv,
			}
			// Get the last command's ResourceAnalysis and update it
			err := ge.resourceManager.UpdateLastCommandDebugInfo(debugInfo)
			if err != nil {
				ge.logger.Warn("Failed to update command debug info", "error", err)
			}
		} else {
			// Text format: output immediately
			debug.FormatFinalEnvironmentText(os.Stdout, finalEnv)
		}
	}
}
```

#### 4.1.2 新しいフィールドの追加

**ファイル**: `internal/runner/group_executor.go`

```go
type GroupExecutor struct {
	// ... existing fields ...

	dryRunFormat string // 新規追加: "text" or "json"
}
```

コンストラクタの変更:

```go
func NewGroupExecutor(..., dryRunFormat string) *GroupExecutor {
	return &GroupExecutor{
		// ... existing fields ...
		dryRunFormat: dryRunFormat,
	}
}
```

### 4.2 ResourceManager の変更

#### 4.2.1 新しいメソッドの追加

**ファイル**: `internal/runner/resource/manager.go`

```go
// RecordGroupAnalysis records a group-level resource analysis with debug info
func (rm *ResourceManager) RecordGroupAnalysis(
	groupName string,
	debugInfo *DebugInfo,
) error {
	if rm == nil {
		return fmt.Errorf("resource manager is nil")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	analysis := ResourceAnalysis{
		Type:      ResourceTypeGroup,
		Operation: OperationAnalyze,
		Target:    groupName,
		Impact: ResourceImpact{
			Description: "Group configuration analysis",
			Reversible:  true,
			Persistent:  false,
		},
		Timestamp: time.Now(),
		Parameters: map[string]any{
			"group_name": groupName,
		},
		DebugInfo: debugInfo,
	}

	rm.result.ResourceAnalyses = append(rm.result.ResourceAnalyses, analysis)
	return nil
}

// UpdateLastCommandDebugInfo updates the most recent command ResourceAnalysis with debug info
// This should be called after ExecuteCommand to add final environment information
func (rm *ResourceManager) UpdateLastCommandDebugInfo(debugInfo *DebugInfo) error {
	if rm == nil {
		return fmt.Errorf("resource manager is nil")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Find the last command resource analysis
	for i := len(rm.result.ResourceAnalyses) - 1; i >= 0; i-- {
		if rm.result.ResourceAnalyses[i].Type == ResourceTypeCommand {
			// Merge with existing debug info if present
			if rm.result.ResourceAnalyses[i].DebugInfo == nil {
				rm.result.ResourceAnalyses[i].DebugInfo = debugInfo
			} else {
				// Merge fields
				if debugInfo.FinalEnvironment != nil {
					rm.result.ResourceAnalyses[i].DebugInfo.FinalEnvironment = debugInfo.FinalEnvironment
				}
				if debugInfo.InheritanceAnalysis != nil {
					rm.result.ResourceAnalyses[i].DebugInfo.InheritanceAnalysis = debugInfo.InheritanceAnalysis
				}
			}
			return nil
		}
	}

	return fmt.Errorf("no command resource analysis found to update")
}
```

## 5. エラーハンドリング

### 5.1 エラー戦略

デバッグ情報収集のエラーは、dry-run実行自体を失敗させない：

- エラーログを出力
- 空のDebugInfoまたはnilを使用
- 実行を継続

### 5.2 エラーケース

| ケース | 対応 |
|--------|------|
| CollectInheritanceAnalysis が nil を返す | DebugInfo 全体を nil にする |
| CollectFinalEnvironment が nil を返す | FinalEnvironment フィールドのみ nil |
| RecordGroupAnalysis が失敗 | ログに警告を出力し、処理を継続 |
| UpdateLastCommandDebugInfo が失敗 | ログに警告を出力し、処理を継続 |
| JSON marshal が失敗 | エラーを返し、テキスト形式へのフォールバックを検討 |

## 6. テスト設計

### 6.1 ユニットテスト

#### 6.1.1 CollectInheritanceAnalysis のテスト

**ファイル**: `internal/runner/debug/collector_test.go` (新規作成)

```go
func TestCollectInheritanceAnalysis(t *testing.T) {
	tests := []struct {
		name              string
		runtimeGlobal     *runnertypes.RuntimeGlobal
		runtimeGroup      *runnertypes.RuntimeGroup
		detailLevel       resource.DryRunDetailLevel
		expectedAnalysis  *resource.InheritanceAnalysis
	}{
		{
			// 目的: DetailLevelSummaryでは何も収集しないことを検証
			// 期待結果: nilが返される（debug_infoフィールド自体がJSON出力に含まれない）
			name: "DetailLevelSummary returns nil",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				GlobalSpec: &runnertypes.GlobalSpec{
					EnvImport: []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				GroupSpec: &runnertypes.GroupSpec{},
				InheritanceMode: runnertypes.InheritanceModeInherit,
			},
			detailLevel: resource.DetailLevelSummary,
			expectedAnalysis: nil,
		},
		{
			// 目的: DetailLevelDetailedでは設定値と計算値のみ収集し、差分情報は含めないことを検証
			// 期待結果: 設定値フィールド（GlobalEnvImport等）と計算値フィールド（InheritanceMode）のみ設定
			//          差分情報フィールド（InheritedVariables等）はnilでJSONに含まれない（omitempty）
			name: "DetailLevelDetailed - basic fields only",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				GlobalSpec: &runnertypes.GlobalSpec{
					EnvImport: []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH", "HOME"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				GroupSpec: &runnertypes.GroupSpec{},
				InheritanceMode: runnertypes.InheritanceModeInherit,
			},
			detailLevel: resource.DetailLevelDetailed,
			expectedAnalysis: &resource.InheritanceAnalysis{
				GlobalEnvImport: []string{"db_host=DB_HOST"},
				GlobalAllowlist: []string{"PATH", "HOME"},
				GroupEnvImport:  []string{},
				GroupAllowlist:  []string{},
				InheritanceMode: runnertypes.InheritanceModeInherit,
				// 差分情報フィールドはnilであるべき（DetailLevelDetailedでは含めない）
				InheritedVariables:            nil,
				RemovedAllowlistVariables:     nil,
				UnavailableEnvImportVariables: nil,
			},
		},
		{
			// 目的: DetailLevelFullでは全フィールド（設定値、計算値、差分情報）を収集することを検証
			// 期待結果: すべてのフィールドが設定される
			//          - 設定値: GlobalEnvImport, GlobalAllowlist, GroupEnvImport, GroupAllowlist
			//          - 計算値: InheritanceMode (Explicit)
			//          - 差分情報:
			//            * InheritedVariables: 空配列（Explicitモードでは継承しない）
			//            * RemovedAllowlistVariables: ["HOME", "USER"]（グローバルから削除された変数）
			//            * UnavailableEnvImportVariables: ["api_key"]（グループで定義されていない内部変数）
			name: "DetailLevelFull - all fields including differences",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				GlobalSpec: &runnertypes.GlobalSpec{
					EnvImport: []string{"db_host=DB_HOST", "api_key=API_KEY"},
					EnvAllowed: []string{"PATH", "HOME", "USER"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				GroupSpec: &runnertypes.GroupSpec{
					EnvImport: []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH"},
				},
				InheritanceMode: runnertypes.InheritanceModeExplicit,
			},
			detailLevel: resource.DetailLevelFull,
			expectedAnalysis: &resource.InheritanceAnalysis{
				GlobalEnvImport: []string{"db_host=DB_HOST", "api_key=API_KEY"},
				GlobalAllowlist: []string{"PATH", "HOME", "USER"},
				GroupEnvImport:  []string{"db_host=DB_HOST"},
				GroupAllowlist:  []string{"PATH"},
				InheritanceMode: runnertypes.InheritanceModeExplicit,
				InheritedVariables:            []string{}, // Explicitモードなので継承なし
				RemovedAllowlistVariables:     []string{"HOME", "USER"}, // グローバルにあってグループにない変数
				UnavailableEnvImportVariables: []string{"api_key"}, // グローバルのenv_importにあってグループにない内部変数
			},
		},
		// 追加すべきテストケース:
		// - InheritanceModeInheritでInheritedVariablesが正しく計算される
		// - InheritanceModeRejectでRemovedAllowlistVariablesが全変数になる
		// - nil/空スライスのエッジケース
		// - GroupSpecがnilの場合の安全な処理
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := debug.CollectInheritanceAnalysis(
				tt.runtimeGlobal,
				tt.runtimeGroup,
				tt.detailLevel,
			)

			if tt.expectedAnalysis == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedAnalysis.GlobalEnvImport, result.GlobalEnvImport)
				assert.Equal(t, tt.expectedAnalysis.InheritanceMode, result.InheritanceMode)
				// ... more assertions
			}
		})
	}
}
```

#### 6.1.2 CollectFinalEnvironment のテスト

**ファイル**: `internal/runner/debug/collector_test.go`

```go
func TestCollectFinalEnvironment(t *testing.T) {
	tests := []struct {
		name          string
		envMap        map[string]executor.EnvVar
		detailLevel   resource.DryRunDetailLevel
		showSensitive bool
		expectedEnv   *resource.FinalEnvironment
	}{
		{
			// 目的: DetailLevelSummaryでは最終環境変数を収集しないことを検証
			// 期待結果: nilが返される（final_environmentフィールドがJSON出力に含まれない）
			name: "DetailLevelSummary returns nil",
			envMap: map[string]executor.EnvVar{
				"PATH": {Value: "/usr/bin", Source: executor.EnvVarSourceSystem},
			},
			detailLevel:   resource.DetailLevelSummary,
			showSensitive: false,
			expectedEnv:   nil,
		},
		{
			// 目的: DetailLevelDetailedでも最終環境変数を収集しないことを検証
			// 期待結果: nilが返される（final_environmentはDetailLevelFullのみで出力）
			name: "DetailLevelDetailed returns nil",
			envMap: map[string]executor.EnvVar{
				"PATH": {Value: "/usr/bin", Source: executor.EnvVarSourceSystem},
			},
			detailLevel:   resource.DetailLevelDetailed,
			showSensitive: false,
			expectedEnv:   nil,
		},
		{
			// 目的: DetailLevelFullかつshowSensitive=trueの場合、すべての環境変数の実際の値を含めることを検証
			// 期待結果:
			//   - PATH: 非センシティブなので値をそのまま表示、Masked=false
			//   - API_KEY: センシティブだが--dry-run-show-sensitiveが指定されているので値を表示、Masked=false
			name: "DetailLevelFull with showSensitive=true",
			envMap: map[string]executor.EnvVar{
				"PATH":    {Value: "/usr/bin", Source: executor.EnvVarSourceSystem},
				"API_KEY": {Value: "secret123", Source: executor.EnvVarSourceEnvImport},
			},
			detailLevel:   resource.DetailLevelFull,
			showSensitive: true,
			expectedEnv: &resource.FinalEnvironment{
				Variables: map[string]resource.EnvironmentVariable{
					"PATH": {
						Value:  "/usr/bin",
						Source: "system",
						Masked: false,
					},
					"API_KEY": {
						Value:  "secret123", // showSensitive=trueなので実際の値を表示
						Source: "env_import",
						Masked: false,
					},
				},
			},
		},
		{
			// 目的: DetailLevelFullかつshowSensitive=false（デフォルト）の場合、
			//      センシティブな環境変数の値をマスクすることを検証
			// 期待結果:
			//   - PATH: 非センシティブなので値をそのまま表示、Masked=false
			//   - API_KEY: センシティブなので値を空文字列に、Masked=true
			//   - JSONパーサーがMasked=trueを見て、値がマスクされていることを判別できる
			name: "DetailLevelFull with showSensitive=false masks sensitive vars",
			envMap: map[string]executor.EnvVar{
				"PATH":    {Value: "/usr/bin", Source: executor.EnvVarSourceSystem},
				"API_KEY": {Value: "secret123", Source: executor.EnvVarSourceEnvImport},
			},
			detailLevel:   resource.DetailLevelFull,
			showSensitive: false,
			expectedEnv: &resource.FinalEnvironment{
				Variables: map[string]resource.EnvironmentVariable{
					"PATH": {
						Value:  "/usr/bin",
						Source: "system",
						Masked: false,
					},
					"API_KEY": {
						Value:  "", // デフォルトでセンシティブな値はマスク
						Source: "env_import",
						Masked: true, // マスク状態を明示
					},
				},
			},
		},
		// 追加すべきテストケース:
		// - 各Sourceタイプ（system, env_import, vars, command）の検証
		// - 空の環境変数マップの処理
		// - redaction.Redactorによるセンシティブパターンマッチングの検証
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := debug.CollectFinalEnvironment(
				tt.envMap,
				tt.detailLevel,
				tt.showSensitive,
			)

			if tt.expectedEnv == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, len(tt.expectedEnv.Variables), len(result.Variables))
				// ... more assertions
			}
		})
	}
}
```

#### 6.1.3 InheritanceMode JSON変換のテスト

**ファイル**: `internal/runner/runnertypes/inheritance_mode_test.go`

```go
func TestInheritanceMode_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		mode     InheritanceMode
		expected string
	}{
		{
			// 目的: InheritanceModeInheritが文字列"inherit"としてJSON化されることを検証
			// 期待結果: `"inherit"`（数値の0ではなく文字列）
			name:     "InheritanceModeInherit",
			mode:     InheritanceModeInherit,
			expected: `"inherit"`,
		},
		{
			// 目的: InheritanceModeExplicitが文字列"explicit"としてJSON化されることを検証
			// 期待結果: `"explicit"`（数値の1ではなく文字列）
			name:     "InheritanceModeExplicit",
			mode:     InheritanceModeExplicit,
			expected: `"explicit"`,
		},
		{
			// 目的: InheritanceModeRejectが文字列"reject"としてJSON化されることを検証
			// 期待結果: `"reject"`（数値の2ではなく文字列）
			name:     "InheritanceModeReject",
			mode:     InheritanceModeReject,
			expected: `"reject"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.mode)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		})
	}
}

func TestInheritanceMode_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected InheritanceMode
		wantErr  bool
		errCheck func(*testing.T, error) // 追加: エラー内容の検証
	}{
		{
			// 目的: 文字列"inherit"を正しくInheritanceModeInheritに変換できることを検証
			// 期待結果: エラーなく、InheritanceModeInherit値が得られる
			name:     "inherit",
			input:    `"inherit"`,
			expected: InheritanceModeInherit,
			wantErr:  false,
		},
		{
			// 目的: 文字列"explicit"を正しくInheritanceModeExplicitに変換できることを検証
			// 期待結果: エラーなく、InheritanceModeExplicit値が得られる
			name:     "explicit",
			input:    `"explicit"`,
			expected: InheritanceModeExplicit,
			wantErr:  false,
		},
		{
			// 目的: 文字列"reject"を正しくInheritanceModeRejectに変換できることを検証
			// 期待結果: エラーなく、InheritanceModeReject値が得られる
			name:     "reject",
			input:    `"reject"`,
			expected: InheritanceModeReject,
			wantErr:  false,
		},
		{
			// 目的: 不正な値を受け取った場合、エラーメッセージに入力値を含めないことを検証（セキュリティ）
			// 期待結果:
			//   - エラーが返される
			//   - エラーメッセージに"invalid"（入力値）が含まれない
			//   - エラーメッセージは汎用的な"invalid inheritance mode value"
			// 理由: エラーメッセージに入力値を含めると、ログインジェクションや情報漏洩のリスクがある
			name:     "invalid value",
			input:    `"invalid"`,
			expected: InheritanceModeInherit,
			wantErr:  true,
			errCheck: func(t *testing.T, err error) {
				errMsg := err.Error()
				assert.NotContains(t, errMsg, "invalid", "error message should not contain input value")
				assert.Contains(t, errMsg, "invalid inheritance mode value")
			},
		},
		{
			// 目的: ログインジェクション攻撃を試みる入力（改行文字を含む）を安全に拒否することを検証
			// 期待結果:
			//   - エラーが返される
			//   - エラーメッセージに改行文字や攻撃文字列が含まれない
			// 理由: 改行を含む文字列がエラーメッセージに含まれると、ログファイルに偽のログエントリを挿入できる
			name:     "potential log injection attempt",
			input:    `"inherit\nmalicious_log_entry"`,
			expected: InheritanceModeInherit,
			wantErr:  true,
			errCheck: func(t *testing.T, err error) {
				errMsg := err.Error()
				assert.NotContains(t, errMsg, "\n")
				assert.NotContains(t, errMsg, "malicious")
			},
		},
		{
			// 目的: パストラバーサル攻撃を試みる入力を安全に拒否することを検証
			// 期待結果:
			//   - エラーが返される
			//   - エラーメッセージにファイルパスや".."が含まれない
			// 理由: パス情報がエラーメッセージに含まれると、システムの構造に関する情報が漏洩する
			name:     "potential information disclosure attempt",
			input:    `"../../../etc/passwd"`,
			expected: InheritanceModeInherit,
			wantErr:  true,
			errCheck: func(t *testing.T, err error) {
				errMsg := err.Error()
				assert.NotContains(t, errMsg, "passwd")
				assert.NotContains(t, errMsg, "..")
			},
		},
		{
			// 目的: 空文字列が適切に拒否されることを検証
			// 期待結果: エラーが返される
			name:     "empty string",
			input:    `""`,
			expected: InheritanceModeInherit,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mode InheritanceMode
			err := json.Unmarshal([]byte(tt.input), &mode)

			if tt.wantErr {
				assert.Error(t, err)
				// エラー内容の詳細チェック（指定されている場合）
				if tt.errCheck != nil {
					tt.errCheck(t, err)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, mode)
			}
		})
	}
}
```

### 6.2 統合テスト

#### 6.2.1 End-to-End JSON出力テスト

**ファイル**: `internal/runner/integration_test.go` または `cmd/runner/main_test.go`

```go
func TestDryRunJSONOutput_WithDebugInfo(t *testing.T) {
	// Setup test configuration
	configContent := `
[global]
env_import = ["db_host=DB_HOST"]
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "test-group"

  [[groups.commands]]
  command = "echo"
  args = ["Hello"]
`

	// Run dry-run with JSON format
	output := runDryRunWithFormat(t, configContent, "json", "full")

	// Parse JSON output
	var result resource.DryRunResult
	err := json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err)

	// Verify structure
	assert.True(t, len(result.ResourceAnalyses) >= 2) // At least group + command

	// Find group analysis
	var groupAnalysis *resource.ResourceAnalysis
	for i := range result.ResourceAnalyses {
		if result.ResourceAnalyses[i].Type == resource.ResourceTypeGroup {
			groupAnalysis = &result.ResourceAnalyses[i]
			break
		}
	}
	assert.NotNil(t, groupAnalysis)

	// Verify debug info in group analysis
	assert.NotNil(t, groupAnalysis.DebugInfo)
	assert.NotNil(t, groupAnalysis.DebugInfo.InheritanceAnalysis)
	assert.Equal(t, []string{"db_host=DB_HOST"}, groupAnalysis.DebugInfo.InheritanceAnalysis.GlobalEnvImport)
	assert.Equal(t, "inherit", groupAnalysis.DebugInfo.InheritanceAnalysis.InheritanceMode.String())

	// Find command analysis
	var commandAnalysis *resource.ResourceAnalysis
	for i := range result.ResourceAnalyses {
		if result.ResourceAnalyses[i].Type == resource.ResourceTypeCommand {
			commandAnalysis = &result.ResourceAnalyses[i]
			break
		}
	}
	assert.NotNil(t, commandAnalysis)

	// Verify final environment in command analysis
	assert.NotNil(t, commandAnalysis.DebugInfo)
	assert.NotNil(t, commandAnalysis.DebugInfo.FinalEnvironment)
	assert.NotEmpty(t, commandAnalysis.DebugInfo.FinalEnvironment.Variables)
}
```

#### 6.2.2 Detail Level別のテスト

```go
func TestDryRunJSONOutput_DetailLevels(t *testing.T) {
	tests := []struct {
		name                 string
		detailLevel          string
		expectDebugInfo      bool
		expectDiffFields     bool
		expectFinalEnv       bool
	}{
		{
			name:                 "summary - no debug info",
			detailLevel:          "summary",
			expectDebugInfo:      false,
			expectDiffFields:     false,
			expectFinalEnv:       false,
		},
		{
			name:                 "detailed - basic info only",
			detailLevel:          "detailed",
			expectDebugInfo:      true,
			expectDiffFields:     false,
			expectFinalEnv:       false,
		},
		{
			name:                 "full - all info",
			detailLevel:          "full",
			expectDebugInfo:      true,
			expectDiffFields:     true,
			expectFinalEnv:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runDryRunWithFormat(t, testConfig, "json", tt.detailLevel)

			var result resource.DryRunResult
			err := json.Unmarshal([]byte(output), &result)
			assert.NoError(t, err)

			// Check group analysis
			groupAnalysis := findResourceAnalysis(result, resource.ResourceTypeGroup)
			if tt.expectDebugInfo {
				assert.NotNil(t, groupAnalysis.DebugInfo)
				assert.NotNil(t, groupAnalysis.DebugInfo.InheritanceAnalysis)

				if tt.expectDiffFields {
					// For full level, difference fields should be present
					assert.NotNil(t, groupAnalysis.DebugInfo.InheritanceAnalysis.InheritedVariables)
				} else {
					// For detailed level, difference fields should be nil
					assert.Nil(t, groupAnalysis.DebugInfo.InheritanceAnalysis.InheritedVariables)
				}
			} else {
				assert.Nil(t, groupAnalysis.DebugInfo)
			}

			// Check command analysis
			commandAnalysis := findResourceAnalysis(result, resource.ResourceTypeCommand)
			if tt.expectFinalEnv {
				assert.NotNil(t, commandAnalysis.DebugInfo)
				assert.NotNil(t, commandAnalysis.DebugInfo.FinalEnvironment)
			} else {
				if commandAnalysis.DebugInfo != nil {
					assert.Nil(t, commandAnalysis.DebugInfo.FinalEnvironment)
				}
			}
		})
	}
}
```

### 6.3 回帰テスト

#### 6.3.1 既存テキスト出力の検証

```go
func TestDryRunTextOutput_Unchanged(t *testing.T) {
	// Ensure text output remains the same after changes
	configContent := `
[global]
env_import = ["db_host=DB_HOST"]
env_allowed = ["PATH"]

[[groups]]
name = "test-group"
  [[groups.commands]]
  command = "echo"
  args = ["test"]
`

	output := runDryRunWithFormat(t, configContent, "text", "full")

	// Verify text output contains expected sections
	assert.Contains(t, output, "Variable Expansion Debug Information")
	assert.Contains(t, output, "from_env Inheritance Analysis")
	assert.Contains(t, output, "[Global Level]")
	assert.Contains(t, output, "[Group: test-group]")
	assert.Contains(t, output, "Final Environment Variables")
}
```

## 7. パフォーマンス考慮事項

### 7.1 最適化ポイント

1. **Detail Level による早期リターン**
   - `DetailLevelSummary` の場合、データ収集をスキップ
   - `DetailLevelDetailed` の場合、差分計算をスキップ

2. **メモリ効率**
   - `omitempty` タグの活用
   - nil フィールドの適切な使用

3. **文字列処理の最適化**
   - `strings.Builder` の使用
   - 不要な文字列コピーの回避

### 7.2 パフォーマンス目標と計測方法

#### 7.2.1 目標値

- **実行時間オーバーヘッド**: 5-10%以内
- **メモリ使用量の増加**: 10%以内

#### 7.2.2 計測方法

**実行時間の計測**:

1. **ベースライン測定** (JSON出力機能追加前):
   ```bash
   # 同じ設定ファイルで10回実行し、平均実行時間を計測
   for i in {1..10}; do
     time ./build/runner --config benchmark.toml --dry-run --dry-run-format text --dry-run-detail full
   done | grep real | awk '{sum+=$2; count++} END {print sum/count}'
   ```

2. **JSON出力時の測定**:
   ```bash
   # JSON形式で同じ設定ファイルを10回実行
   for i in {1..10}; do
     time ./build/runner --config benchmark.toml --dry-run --dry-run-format json --dry-run-detail full > /dev/null
   done | grep real | awk '{sum+=$2; count++} END {print sum/count}'
   ```

3. **オーバーヘッドの計算**:
   ```
   オーバーヘッド = ((JSON時間 - ベースライン時間) / ベースライン時間) × 100%
   ```

**メモリ使用量の計測**:

1. **Goの組み込みベンチマークツールを使用**:
   ```bash
   # メモリアロケーションを含むベンチマーク
   go test -bench=BenchmarkDryRun -benchmem -run=^$ ./internal/runner/
   ```

2. **/usr/bin/time を使用** (より詳細な情報):
   ```bash
   # ベースライン
   /usr/bin/time -v ./build/runner --config benchmark.toml --dry-run --dry-run-format text --dry-run-detail full 2>&1 | grep "Maximum resident set size"

   # JSON形式
   /usr/bin/time -v ./build/runner --config benchmark.toml --dry-run --dry-run-format json --dry-run-detail full > /dev/null 2>&1 | grep "Maximum resident set size"
   ```

3. **pprof によるプロファイリング** (詳細分析):
   ```bash
   # メモリプロファイルの取得
   go test -memprofile=mem.prof -bench=BenchmarkDryRun ./internal/runner/

   # プロファイルの分析
   go tool pprof -alloc_space mem.prof
   go tool pprof -alloc_objects mem.prof
   ```

#### 7.2.3 ベンチマーク用設定ファイル

パフォーマンス計測には、実環境を模した設定ファイルを使用:

```toml
# benchmark.toml - 中規模の設定ファイル（100グループ、各5コマンド）
[global]
env_import = ["db_host=DB_HOST", "api_key=API_KEY", "service_url=SERVICE_URL"]
env_allowed = ["PATH", "HOME", "USER", "LANG", "TZ"]

[[groups]]
name = "group-001"
env_import = ["db_host=DB_HOST"]
env_allowed = ["PATH", "HOME"]
  [[groups.commands]]
  command = "echo"
  args = ["test1"]
  # ... 5 commands per group

# ... repeat for 100 groups
```

**設定ファイルのバリエーション**:
- 小規模: 10グループ、各2コマンド（軽量テスト）
- 中規模: 100グループ、各5コマンド（標準的なユースケース）
- 大規模: 500グループ、各10コマンド（スケーラビリティテスト）

#### 7.2.4 計測のタイミング

1. **Phase 1完了後**: データ構造の基本的なオーバーヘッドを計測
2. **Phase 2完了後**: データ収集層のオーバーヘッドを計測
3. **Phase 4完了後**: 統合後の全体的なパフォーマンスを計測
4. **最終確認時**: 本番環境を模した条件で計測

#### 7.2.5 許容基準

**実行時間**:
- ✅ 合格: オーバーヘッド ≤ 10%
- ⚠️  警告: 10% < オーバーヘッド ≤ 15%（最適化を検討）
- ❌ 不合格: オーバーヘッド > 15%（リファクタリング必須）

**メモリ使用量**:
- ✅ 合格: 増加量 ≤ 10%
- ⚠️  警告: 10% < 増加量 ≤ 20%（メモリ効率の改善を検討）
- ❌ 不合格: 増加量 > 20%（設計の見直しが必要）

#### 7.2.6 パフォーマンス改善の指針

目標を達成できない場合の対策:

1. **Detail Levelでの早期リターン**:
   - DetailLevelSummaryでデータ収集をスキップ
   - DetailLevelDetailedで差分計算をスキップ

2. **メモリアロケーションの最適化**:
   - 不要な文字列コピーを削減
   - スライスの事前割り当て（`make([]string, 0, expectedSize)`）
   - `omitempty`タグの活用

3. **JSON marshaling の最適化**:
   - 大きな構造体の場合、ストリーミングJSONエンコーダの使用を検討
   - 不要なフィールドのnilチェックを最小化

## 8. セキュリティ考慮事項

### 8.1 センシティブ情報の保護

1. **デフォルトでマスク**
   - `showSensitive=false` がデフォルト
   - センシティブなパターンは `redaction.DefaultSensitivePatterns()` を使用

2. **マスク状態の明示**
   - `Masked` フィールドで明示的に示す
   - JSONパーサーがマスク状態を判別可能

3. **Dry-Run出力とエラーログの分離**

   **Dry-Run出力（stdout）**:
   - `--show-sensitive`フラグでセンシティブ情報の表示を制御
   - `--dry-run-detail`レベルで出力の詳細度を制御
   - JSON形式とTEXT形式の両方で、これらのフラグに従う
   - ユーザーが明示的に`--show-sensitive`を指定した場合のみ、センシティブな値を表示
   - デフォルトは`showSensitive=false`でマスク
   - この制御は既に`CollectFinalEnvironment()`で実装済み

   **エラー時のログ出力（slog）**:
   - デバッグ情報収集の失敗は`slog.Warn()`で記録
   - 通常のロギングシステムへの出力（dry-run出力とは別）
   - エラーログには環境変数の値を含めない（セキュリティのため）
   - GroupExecutorから呼び出す際の例:
     ```go
     if err := ge.resourceManager.RecordGroupAnalysis(groupName, debugInfo); err != nil {
         ge.logger.Warn("Failed to record group analysis",
             "error", err,
             "group", groupName,
             "detail_level", ge.dryRunDetailLevel.String())
         // エラーを無視して実行を継続
     }
     ```

   **エラーログで安全な情報のみ記録**:
   - エラーメッセージ（入力値を含まない汎用的なもの）
   - グループ名、コマンド名
   - Detail Level（設定値）
   - エラーオブジェクト（`error`フィールド）
   - 環境変数の値や名前は含めない（dry-run出力で制御されるため、ログには不要）

## 9. ドキュメント更新

### 9.1 更新が必要なファイル

- [ ] `docs/user/dry_run.md` - dry-runモードのユーザーマニュアル
- [ ] `docs/user/output_formats.md` - 出力形式のドキュメント（新規作成の可能性）
- [ ] `README.md` - JSON出力機能の追加を記載

### 9.2 JSON Schema ドキュメント

新しい構造体の JSON Schema を文書化:

- `DebugInfo`
- `InheritanceAnalysis`
- `FinalEnvironment`
- `EnvironmentVariable`

## 10. 実装の優先順位

1. **高優先度** (Phase 1)
   - データ構造の定義
   - `InheritanceMode` の JSON 変換
   - データ収集関数

2. **中優先度** (Phase 2)
   - フォーマットヘルパー関数
   - `ResourceManager` の拡張

3. **低優先度** (Phase 3)
   - `GroupExecutor` の統合
   - 統合テスト
   - ドキュメント更新

詳細は [実装計画書](./04_implementation_plan.ja.md) を参照。
