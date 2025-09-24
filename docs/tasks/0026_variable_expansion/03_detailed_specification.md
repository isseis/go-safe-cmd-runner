# 詳細仕様書: コマンド・引数内環境変数展開機能

## 1. 実装詳細仕様

### 1.1 パッケージ構成詳細

```
internal/runner/expansion/
├── expander.go          # 変数展開エンジンのメイン実装
├── parser.go           # 変数パーサー実装
├── detector.go         # 循環参照検出実装
├── types.go           # 型定義とインターフェース
├── errors.go          # エラー型定義
└── expansion_test.go  # 統合テスト

# 既存コンポーネント活用
internal/runner/security/validator.go  # 既存のセキュリティ検証を拡張
```

### 1.2 型定義とインターフェース

#### 1.2.1 コア型定義 (types.go)

```go
package expansion

import (
    "context"
    "time"
)

// VariableExpander は変数展開の統合インターフェース
type VariableExpander interface {
    ExpandCommand(ctx context.Context, cmd string, env map[string]string, allowlist []string) (string, error)
    ExpandArgs(ctx context.Context, args []string, env map[string]string, allowlist []string) ([]string, error)
    ValidateVariables(ctx context.Context, cmd string, args []string, env map[string]string, allowlist []string) error
}

// VariableParser は変数参照の解析インターフェース
type VariableParser interface {
    ExtractVariables(text string) ([]VariableRef, error)
    ReplaceVariables(text string, variables map[string]string) (string, error)
}

// 既存のSecurity Validatorを活用
// internal/runner/security パッケージの Validator 型を使用
// 変数展開機能に必要なメソッド:
// - ValidateVariableValue(value string) error
// - ValidateAllEnvironmentVars(envVars map[string]string) error
// 必要に応じて拡張メソッドを追加

// CircularReferenceDetector は循環参照検出インターフェース
type CircularReferenceDetector interface {
    DetectCircularReference(env map[string]string) error
    BuildDependencyGraph(env map[string]string) (*DependencyGraph, error)
}

// VariableRef は変数参照の詳細情報
type VariableRef struct {
    Name       string         // 変数名
    StartPos   int            // テキスト内の開始位置
    EndPos     int            // テキスト内の終了位置
    Format     VariableFormat // 変数形式 ($VAR or ${VAR})
    FullMatch  string         // 完全マッチ文字列
}

// VariableFormat は変数形式の列挙型
type VariableFormat int

const (
    FormatSimple VariableFormat = iota // $VAR
    FormatBraced                      // ${VAR}
)

// DependencyGraph は変数依存関係のグラフ
type DependencyGraph struct {
    Nodes map[string]*GraphNode
    Edges map[string][]string
}

// GraphNode はグラフのノード
type GraphNode struct {
    Name         string
    Dependencies []string
    Visited      bool
    InStack      bool
}

// ExpansionContext は展開処理のコンテキスト
type ExpansionContext struct {
    MaxDepth     int
    CurrentDepth int
    ProcessedVars map[string]bool
}

// ExpansionMetrics は性能メトリクス
type ExpansionMetrics struct {
    TotalExpansions     int64
    ExpansionDuration   time.Duration
    VariableCount       int
    MaxNestingDepth     int
    CacheHitRatio       float64
    ErrorCount          int64
    SecurityViolations  int64
}
type ExpansionMetrics struct {
    TotalExpansions    int64
    ExpansionDuration  time.Duration
    VariableCount      int
    MaxNestingDepth    int
    CacheHitRatio      float64
    ErrorCount         int64
    SecurityViolations int64
}
```

#### 1.2.2 エラー型定義 (errors.go)

```go
package expansion

import (
    "fmt"
)

// ExpansionErrorType はエラータイプの列挙型
type ExpansionErrorType int

const (
    ErrorTypeUnknown ExpansionErrorType = iota
    ErrorTypeVariableNotFound
    ErrorTypeCircularReference
    ErrorTypeSecurityViolation
    ErrorTypeSyntaxError
    ErrorTypePathValidation
    ErrorTypeMaxDepthExceeded
    ErrorTypeInvalidFormat
)

// ExpansionError は変数展開エラーの詳細情報
type ExpansionError struct {
    Type      ExpansionErrorType
    Message   string
    Context   ErrorContext
    Cause     error
}

// ErrorContext はエラーコンテキスト
type ErrorContext struct {
    Variable     string
    Position     int
    Text         string
    CommandIndex int
    ArgIndex     int
}

func (e *ExpansionError) Error() string {
    return fmt.Sprintf("variable expansion error: %s (type: %s, variable: %s)",
                      e.Message, e.Type.String(), e.Context.Variable)
}

func (e *ExpansionError) Unwrap() error {
    return e.Cause
}

// 特定エラー型の判定関数
func IsVariableNotFoundError(err error) bool {
    if expErr, ok := err.(*ExpansionError); ok {
        return expErr.Type == ErrorTypeVariableNotFound
    }
    return false
}

func IsCircularReferenceError(err error) bool {
    if expErr, ok := err.(*ExpansionError); ok {
        return expErr.Type == ErrorTypeCircularReference
    }
    return false
}

func IsSecurityViolationError(err error) bool {
    if expErr, ok := err.(*ExpansionError); ok {
        return expErr.Type == ErrorTypeSecurityViolation
    }
    return false
}

// エラーファクトリ関数
func NewVariableNotFoundError(variable string, position int, text string) *ExpansionError {
    return &ExpansionError{
        Type:    ErrorTypeVariableNotFound,
        Message: fmt.Sprintf("variable '%s' not found or not allowed", variable),
        Context: ErrorContext{
            Variable: variable,
            Position: position,
            Text:     text,
        },
    }
}

func NewCircularReferenceError(variable string, chain []string) *ExpansionError {
    return &ExpansionError{
        Type:    ErrorTypeCircularReference,
        Message: fmt.Sprintf("circular reference detected in variable '%s': %v", variable, chain),
        Context: ErrorContext{
            Variable: variable,
        },
    }
}

func NewSecurityViolationError(variable string, reason string) *ExpansionError {
    return &ExpansionError{
        Type:    ErrorTypeSecurityViolation,
        Message: fmt.Sprintf("security violation: variable '%s' - %s", variable, reason),
        Context: ErrorContext{
            Variable: variable,
        },
    }
}
```

### 1.3 変数パーサー仕様 (parser.go)

#### 1.3.1 パーサー実装

```go
package expansion

import (
    "regexp"
    "sort"
    "strings"
)

// variableParser は変数パーサーの実装
type variableParser struct {
    simplePattern *regexp.Regexp // $VAR パターン
    bracedPattern *regexp.Regexp // ${VAR} パターン
}

// NewVariableParser は新しい変数パーサーを作成
func NewVariableParser() VariableParser {
    return &variableParser{
        // $VAR形式: $で始まり、英数字とアンダースコアの組み合わせ
        // 注意: prefix_$VAR_suffix形式では $VAR_suffix までが変数名と認識されるため
        // 推奨されない。代わりに prefix_${VAR}_suffix 形式を使用すること
        simplePattern: regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`),
        // ${VAR}形式: ${で始まり}で終わる
        bracedPattern: regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`),
    }
}

// ExtractVariables は文字列から変数参照を抽出
func (p *variableParser) ExtractVariables(text string) ([]VariableRef, error) {
    var variables []VariableRef

    // ${VAR}形式を先に処理（より具体的なパターン）
    bracedMatches := p.bracedPattern.FindAllStringSubmatchIndex(text, -1)
    for _, match := range bracedMatches {
        variables = append(variables, VariableRef{
            Name:      text[match[2]:match[3]],
            StartPos:  match[0],
            EndPos:    match[1],
            Format:    FormatBraced,
            FullMatch: text[match[0]:match[1]],
        })
    }

    // $VAR形式を処理（ブレース形式と重複しないように）
    simpleMatches := p.simplePattern.FindAllStringSubmatchIndex(text, -1)
    for _, match := range simpleMatches {
        // ブレース形式と重複チェック
        if !p.isOverlappingWithBraced(match[0], match[1], bracedMatches) {
            variables = append(variables, VariableRef{
                Name:      text[match[2]:match[3]],
                StartPos:  match[0],
                EndPos:    match[1],
                Format:    FormatSimple,
                FullMatch: text[match[0]:match[1]],
            })
        }
    }

    // 位置でソート
    sort.Slice(variables, func(i, j int) bool {
        return variables[i].StartPos < variables[j].StartPos
    })

    return variables, nil
}

// ReplaceVariables は変数を実際の値に置換
func (p *variableParser) ReplaceVariables(text string, variables map[string]string) (string, error) {
    result := text

    // 変数参照を抽出
    refs, err := p.ExtractVariables(text)
    if err != nil {
        return "", err
    }

    // 後ろから置換（位置ずれを防ぐため）
    for i := len(refs) - 1; i >= 0; i-- {
        ref := refs[i]
        value, exists := variables[ref.Name]
        if !exists {
            return "", NewVariableNotFoundError(ref.Name, ref.StartPos, text)
        }

        // 文字列置換
        result = result[:ref.StartPos] + value + result[ref.EndPos:]
    }

    return result, nil
}

// isOverlappingWithBraced はブレース形式との重複をチェック
func (p *variableParser) isOverlappingWithBraced(start, end int, bracedMatches [][]int) bool {
    for _, bracedMatch := range bracedMatches {
        // 範囲が重複している場合
        if start < bracedMatch[1] && end > bracedMatch[0] {
            return true
        }
    }
    return false
}

// ValidateVariableName は変数名の妥当性をチェック
func ValidateVariableName(name string) error {
    if name == "" {
        return &ExpansionError{
            Type:    ErrorTypeSyntaxError,
            Message: "variable name cannot be empty",
        }
    }

    // 先頭文字チェック
    if !((name[0] >= 'A' && name[0] <= 'Z') ||
         (name[0] >= 'a' && name[0] <= 'z') ||
         name[0] == '_') {
        return &ExpansionError{
            Type:    ErrorTypeSyntaxError,
            Message: fmt.Sprintf("variable name '%s' must start with letter or underscore", name),
        }
    }

    // 全文字チェック
    for _, char := range name {
        if !((char >= 'A' && char <= 'Z') ||
             (char >= 'a' && char <= 'z') ||
             (char >= '0' && char <= '9') ||
             char == '_') {
            return &ExpansionError{
                Type:    ErrorTypeSyntaxError,
                Message: fmt.Sprintf("variable name '%s' contains invalid character", name),
            }
        }
    }

    return nil
}
```

### 1.4 既存セキュリティ検証との統合 (security/validator.go)

#### 1.4.1 既存コンポーネント活用

```go
package expansion

import (
    "fmt"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// 既存の security.Validator を活用して変数検証を実行
type SecurityValidationAdapter struct {
    validator *security.Validator
}

// NewSecurityValidationAdapter は既存の Validator をラップ
func NewSecurityValidationAdapter(validator *security.Validator) *SecurityValidationAdapter {
    return &SecurityValidationAdapter{
        validator: validator,
    }
}

// ValidateVariables は変数のセキュリティ検証
func (a *SecurityValidationAdapter) ValidateVariables(variables []string, allowlist []string, commandEnv map[string]string) error {
    // allowlist 検証ロジック
    allowlistMap := make(map[string]bool)
    for _, allowed := range allowlist {
        allowlistMap[allowed] = true
    }

    for _, variable := range variables {
        // Command.Env で定義されている場合は無条件許可
        if _, inCommandEnv := commandEnv[variable]; inCommandEnv {
            continue
        }

        // allowlist に含まれている場合は許可
        if allowlistMap[variable] {
            continue
        }

        // どちらにも含まれていない場合はエラー
        return fmt.Errorf("variable '%s' not in allowlist and not defined in command environment", variable)
    }

    return nil
}

// ValidateVariableValues は変数値の安全性を検証
func (a *SecurityValidationAdapter) ValidateVariableValues(envVars map[string]string) error {
    // 既存の ValidateAllEnvironmentVars を使用
    return a.validator.ValidateAllEnvironmentVars(envVars)
}

// ValidateExpandedCommand は展開後のコマンドパスを検証
func (a *SecurityValidationAdapter) ValidateExpandedCommand(cmd string) error {
    // 既存の ValidateCommand を使用
    return a.validator.ValidateCommand(cmd)
}

// ValidateVariableValue は変数値の安全性をチェック
func ValidateVariableValue(name, value string) error {
    // 空値チェック
    if value == "" {
        return &ExpansionError{
            Type:    ErrorTypeSyntaxError,
            Message: fmt.Sprintf("variable '%s' has empty value", name),
        }
    }

    // 危険な文字チェック（シェル特殊文字）
    dangerousChars := []string{";", "&", "|", "`", "$", "(", ")", "<", ">"}
    for _, char := range dangerousChars {
        if strings.Contains(value, char) {
            return &ExpansionError{
                Type:    ErrorTypeSecurityViolation,
                Message: fmt.Sprintf("variable '%s' contains dangerous character '%s'", name, char),
            }
        }
    }

    // 注意: グロブパターン (*, ?) はリテラル文字として扱い、展開しない
    // エスケープ機能は提供されない（セキュリティ上の制約）

    return nil
}

// PathValidator は既存のパス検証インターフェース
type PathValidator interface {
    ValidateCommandPath(path string) error
}
```

### 1.5 循環参照検出仕様 (detector.go)

#### 1.5.1 循環参照アルゴリズム

```go
package expansion

import (
    "fmt"
)

// circularReferenceDetector は循環参照検出の実装
type circularReferenceDetector struct {
    maxDepth int
}

// NewCircularReferenceDetector は新しい循環参照検出器を作成
func NewCircularReferenceDetector(maxDepth int) CircularReferenceDetector {
    return &circularReferenceDetector{
        maxDepth: maxDepth,
    }
}

// DetectCircularReference は循環参照を検出
func (d *circularReferenceDetector) DetectCircularReference(env map[string]string) error {
    graph, err := d.BuildDependencyGraph(env)
    if err != nil {
        return err
    }

    // DFS で循環参照を検出
    for nodeName := range graph.Nodes {
        if err := d.dfsDetectCycle(graph, nodeName, []string{}); err != nil {
            return err
        }
    }

    return nil
}

// BuildDependencyGraph は依存関係グラフを構築
func (d *circularReferenceDetector) BuildDependencyGraph(env map[string]string) (*DependencyGraph, error) {
    graph := &DependencyGraph{
        Nodes: make(map[string]*GraphNode),
        Edges: make(map[string][]string),
    }

    parser := NewVariableParser()

    // 各環境変数について依存関係を解析
    for name, value := range env {
        node := &GraphNode{
            Name:         name,
            Dependencies: []string{},
            Visited:      false,
            InStack:      false,
        }
        graph.Nodes[name] = node

        // 値に含まれる変数参照を抽出
        refs, err := parser.ExtractVariables(value)
        if err != nil {
            return nil, fmt.Errorf("failed to extract variables from %s: %w", name, err)
        }

        // 依存関係を記録
        for _, ref := range refs {
            node.Dependencies = append(node.Dependencies, ref.Name)
            graph.Edges[name] = append(graph.Edges[name], ref.Name)
        }
    }

    return graph, nil
}

// dfsDetectCycle は深度優先探索で循環参照を検出
func (d *circularReferenceDetector) dfsDetectCycle(graph *DependencyGraph, nodeName string, path []string) error {
    node, exists := graph.Nodes[nodeName]
    if !exists {
        // 存在しない変数は循環参照の対象外
        return nil
    }

    // 既に処理済みの場合はスキップ
    if node.Visited {
        return nil
    }

    // 現在のパスに含まれている場合は循環参照
    for _, pathNode := range path {
        if pathNode == nodeName {
            cycle := append(path, nodeName)
            return NewCircularReferenceError(nodeName, cycle)
        }
    }

    // 最大深度チェック
    if len(path) >= d.maxDepth {
        return &ExpansionError{
            Type:    ErrorTypeMaxDepthExceeded,
            Message: fmt.Sprintf("maximum nesting depth %d exceeded at variable '%s'", d.maxDepth, nodeName),
            Context: ErrorContext{Variable: nodeName},
        }
    }

    // 現在のノードをパスに追加して依存関係を探索
    newPath := append(path, nodeName)
    for _, dependency := range node.Dependencies {
        if err := d.dfsDetectCycle(graph, dependency, newPath); err != nil {
            return err
        }
    }

    // 処理済みとしてマーク
    node.Visited = true
    return nil
}

// GetCircularReferenceChain は循環参照のチェーンを取得
func (d *circularReferenceDetector) GetCircularReferenceChain(env map[string]string, startVar string) ([]string, error) {
    visited := make(map[string]bool)
    stack := []string{}

    return d.findCycle(env, startVar, visited, stack)
}

// findCycle は循環参照のチェーンを再帰的に探索
func (d *circularReferenceDetector) findCycle(env map[string]string, current string, visited map[string]bool, stack []string) ([]string, error) {
    // 既にスタックに含まれている場合は循環参照
    for i, stackVar := range stack {
        if stackVar == current {
            return stack[i:], nil
        }
    }

    // 既に訪問済みの場合は終了
    if visited[current] {
        return nil, nil
    }

    visited[current] = true
    stack = append(stack, current)

    // 環境変数値を取得
    value, exists := env[current]
    if !exists {
        return nil, nil
    }

    // 値に含まれる変数参照を解析
    parser := NewVariableParser()
    refs, err := parser.ExtractVariables(value)
    if err != nil {
        return nil, err
    }

    // 各依存変数について再帰的に探索
    for _, ref := range refs {
        cycle, err := d.findCycle(env, ref.Name, visited, stack)
        if err != nil {
            return nil, err
        }
        if cycle != nil {
            return cycle, nil
        }
    }

    return nil, nil
}
```

### 1.6 変数展開エンジン仕様 (expander.go)

#### 1.6.1 メイン展開ロジック

```go
package expansion

import (
    "context"
    "fmt"
    "os"
    "time"
)

// variableExpander は変数展開エンジンの実装
type variableExpander struct {
    parser            VariableParser
    validator         *SecurityValidationAdapter  // 既存Validatorのアダプター
    circularDetector  CircularReferenceDetector
    metrics           *ExpansionMetrics
    maxDepth          int
}

// NewVariableExpander は新しい変数展開エンジンを作成
func NewVariableExpander(securityValidator *security.Validator, maxDepth int) VariableExpander {
    return &variableExpander{
        parser:           NewVariableParser(),
        validator:        NewSecurityValidationAdapter(securityValidator),
        circularDetector: NewCircularReferenceDetector(maxDepth),
        metrics:          &ExpansionMetrics{},
        maxDepth:         maxDepth,
    }
}

// ExpandCommand はコマンド名の変数を展開
func (e *variableExpander) ExpandCommand(ctx context.Context, cmd string, env map[string]string, allowlist []string) (string, error) {
    startTime := time.Now()
    defer func() {
        e.metrics.ExpansionDuration += time.Since(startTime)
        e.metrics.TotalExpansions++
    }()

    // 変数参照を抽出
    refs, err := e.parser.ExtractVariables(cmd)
    if err != nil {
        e.metrics.ErrorCount++
        return "", fmt.Errorf("failed to extract variables from command: %w", err)
    }

    if len(refs) == 0 {
        return cmd, nil // 変数参照がない場合はそのまま返す
    }

    e.metrics.VariableCount = len(refs)

    // セキュリティ検証
    varNames := make([]string, len(refs))
    for i, ref := range refs {
        varNames[i] = ref.Name
    }

    if err := e.validator.ValidateVariables(varNames, allowlist, env); err != nil {
        e.metrics.ErrorCount++
        e.metrics.SecurityViolations++
        return "", fmt.Errorf("security validation failed: %w", err)
    }

    // 展開用の環境変数マップを構築
    expandEnv, err := e.buildExpandEnv(env, allowlist)
    if err != nil {
        e.metrics.ErrorCount++
        return "", fmt.Errorf("failed to build expansion environment: %w", err)
    }

    // 循環参照チェック
    if err := e.circularDetector.DetectCircularReference(expandEnv); err != nil {
        e.metrics.ErrorCount++
        return "", fmt.Errorf("circular reference detected: %w", err)
    }

    // 変数展開実行
    expanded, err := e.expandString(cmd, expandEnv, 0)
    if err != nil {
        e.metrics.ErrorCount++
        return "", fmt.Errorf("expansion failed: %w", err)
    }

    // 展開後のコマンドパス検証
    if err := e.validator.ValidateExpandedCommand(expanded); err != nil {
        e.metrics.ErrorCount++
        return "", fmt.Errorf("expanded command validation failed: %w", err)
    }

    return expanded, nil
}

// ExpandArgs は引数リストの変数を展開
func (e *variableExpander) ExpandArgs(ctx context.Context, args []string, env map[string]string, allowlist []string) ([]string, error) {
    if len(args) == 0 {
        return args, nil
    }

    startTime := time.Now()
    defer func() {
        e.metrics.ExpansionDuration += time.Since(startTime)
        e.metrics.TotalExpansions++
    }()

    // 全引数から変数参照を収集
    allVarNames := make(map[string]bool)
    for _, arg := range args {
        refs, err := e.parser.ExtractVariables(arg)
        if err != nil {
            e.metrics.ErrorCount++
            return nil, fmt.Errorf("failed to extract variables from arg '%s': %w", arg, err)
        }
        for _, ref := range refs {
            allVarNames[ref.Name] = true
        }
    }

    // セキュリティ検証
    varNames := make([]string, 0, len(allVarNames))
    for name := range allVarNames {
        varNames = append(varNames, name)
    }

    if len(varNames) > 0 {
        if err := e.validator.ValidateVariables(varNames, allowlist, env); err != nil {
            e.metrics.ErrorCount++
            e.metrics.SecurityViolations++
            return nil, fmt.Errorf("security validation failed: %w", err)
        }
    }

    e.metrics.VariableCount = len(varNames)

    // 展開用の環境変数マップを構築
    expandEnv, err := e.buildExpandEnv(env, allowlist)
    if err != nil {
        e.metrics.ErrorCount++
        return nil, fmt.Errorf("failed to build expansion environment: %w", err)
    }

    // 循環参照チェック
    if len(expandEnv) > 0 {
        if err := e.circularDetector.DetectCircularReference(expandEnv); err != nil {
            e.metrics.ErrorCount++
            return nil, fmt.Errorf("circular reference detected: %w", err)
        }
    }

    // 各引数を展開
    expandedArgs := make([]string, len(args))
    for i, arg := range args {
        expanded, err := e.expandString(arg, expandEnv, 0)
        if err != nil {
            e.metrics.ErrorCount++
            return nil, fmt.Errorf("failed to expand arg[%d] '%s': %w", i, arg, err)
        }
        expandedArgs[i] = expanded
    }

    return expandedArgs, nil
}

// ValidateVariables は変数の事前検証
func (e *variableExpander) ValidateVariables(ctx context.Context, cmd string, args []string, env map[string]string, allowlist []string) error {
    // コマンドの変数を収集
    cmdRefs, err := e.parser.ExtractVariables(cmd)
    if err != nil {
        return fmt.Errorf("failed to extract variables from command: %w", err)
    }

    // 引数の変数を収集
    allVarNames := make(map[string]bool)
    for _, ref := range cmdRefs {
        allVarNames[ref.Name] = true
    }

    for _, arg := range args {
        refs, err := e.parser.ExtractVariables(arg)
        if err != nil {
            return fmt.Errorf("failed to extract variables from args: %w", err)
        }
        for _, ref := range refs {
            allVarNames[ref.Name] = true
        }
    }

    // セキュリティ検証
    varNames := make([]string, 0, len(allVarNames))
    for name := range allVarNames {
        varNames = append(varNames, name)
    }

    if len(varNames) > 0 {
        return e.validator.ValidateVariables(varNames, allowlist, env)
    }

    return nil
}

// buildExpandEnv は展開用の環境変数マップを構築
func (e *variableExpander) buildExpandEnv(commandEnv map[string]string, allowlist []string) (map[string]string, error) {
    expandEnv := make(map[string]string)

    // Command.Env の変数を優先的に追加
    for name, value := range commandEnv {
        expandEnv[name] = value
    }

    // allowlist に含まれるOS環境変数を追加（Command.Env で未定義の場合のみ）
    for _, allowedVar := range allowlist {
        if _, exists := expandEnv[allowedVar]; !exists {
            if osValue := os.Getenv(allowedVar); osValue != "" {
                expandEnv[allowedVar] = osValue
            }
        }
    }

    return expandEnv, nil
}

// expandString は文字列の変数を再帰的に展開
func (e *variableExpander) expandString(text string, env map[string]string, depth int) (string, error) {
    if depth >= e.maxDepth {
        return "", &ExpansionError{
            Type:    ErrorTypeMaxDepthExceeded,
            Message: fmt.Sprintf("maximum expansion depth %d exceeded", e.maxDepth),
        }
    }

    // ネストした深度を追跡
    if depth > e.metrics.MaxNestingDepth {
        e.metrics.MaxNestingDepth = depth
    }

    // 変数参照を検索
    refs, err := e.parser.ExtractVariables(text)
    if err != nil {
        return "", err
    }

    if len(refs) == 0 {
        return text, nil // 変数参照がない場合は終了
    }

    // 各変数を値に置換
    result := text
    for i := len(refs) - 1; i >= 0; i-- { // 後ろから処理して位置ずれを防ぐ
        ref := refs[i]
        value, exists := env[ref.Name]
        if !exists {
            return "", NewVariableNotFoundError(ref.Name, ref.StartPos, text)
        }

        // 値に変数参照が含まれている場合は再帰的に展開
        expandedValue, err := e.expandString(value, env, depth+1)
        if err != nil {
            return "", fmt.Errorf("failed to expand nested variable '%s': %w", ref.Name, err)
        }

        // 文字列を置換
        result = result[:ref.StartPos] + expandedValue + result[ref.EndPos:]
    }

    return result, nil
}

// GetMetrics は展開処理のメトリクスを取得
func (e *variableExpander) GetMetrics() ExpansionMetrics {
    return *e.metrics
}

// ResetMetrics はメトリクスをリセット
func (e *variableExpander) ResetMetrics() {
    e.metrics = &ExpansionMetrics{}
}
```

### 1.7 設定統合仕様

#### 1.7.1 Config Parser統合 (internal/runner/config/command.go への追加)

```go
// Command構造体への変数展開統合
func (c *Command) ExpandVariables(expander expansion.VariableExpander, allowlist []string) error {
    ctx := context.Background()

    // 環境変数マップを構築
    env, err := c.BuildEnvironmentMap()
    if err != nil {
        return fmt.Errorf("failed to build environment map: %w", err)
    }

    // 事前検証
    if err := expander.ValidateVariables(ctx, c.Cmd, c.Args, env, allowlist); err != nil {
        return fmt.Errorf("variable validation failed: %w", err)
    }

    // コマンド名の展開
    if expandedCmd, err := expander.ExpandCommand(ctx, c.Cmd, env, allowlist); err != nil {
        return fmt.Errorf("failed to expand command: %w", err)
    } else {
        c.Cmd = expandedCmd
    }

    // 引数の展開
    if expandedArgs, err := expander.ExpandArgs(ctx, c.Args, env, allowlist); err != nil {
        return fmt.Errorf("failed to expand args: %w", err)
    } else {
        c.Args = expandedArgs
    }

    return nil
}

// BuildEnvironmentMap は環境変数マップを構築
func (c *Command) BuildEnvironmentMap() (map[string]string, error) {
    env := make(map[string]string)

    for _, envVar := range c.Env {
        parts := strings.SplitN(envVar, "=", 2)
        if len(parts) != 2 {
            return nil, fmt.Errorf("invalid environment variable format: %s", envVar)
        }
        env[parts[0]] = parts[1]
    }

    return env, nil
}
```

### 1.8 テストケース仕様

#### 1.8.1 単体テストケース

```go
// expansion_test.go

package expansion

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestVariableParser_ExtractVariables(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected []VariableRef
    }{
        {
            name:  "simple variable",
            input: "$HOME",
            expected: []VariableRef{
                {Name: "HOME", StartPos: 0, EndPos: 5, Format: FormatSimple, FullMatch: "$HOME"},
            },
        },
        {
            name:  "braced variable",
            input: "${USER}",
            expected: []VariableRef{
                {Name: "USER", StartPos: 0, EndPos: 7, Format: FormatBraced, FullMatch: "${USER}"},
            },
        },
        {
            name:  "mixed variables",
            input: "$HOME/bin/${APP_NAME}",
            expected: []VariableRef{
                {Name: "HOME", StartPos: 0, EndPos: 5, Format: FormatSimple, FullMatch: "$HOME"},
                {Name: "APP_NAME", StartPos: 10, EndPos: 21, Format: FormatBraced, FullMatch: "${APP_NAME}"},
            },
        },
        {
            name:  "prefix_$VAR_suffix problem case",
            input: "prefix_$HOME_suffix",
            expected: []VariableRef{
                // 注意: $HOME_suffix 全体が変数名と認識されてしまう問題
                // このため prefix_${HOME}_suffix 形式が推奨される
                {Name: "HOME_suffix", StartPos: 7, EndPos: 19, Format: FormatSimple, FullMatch: "$HOME_suffix"},
            },
        },
        {
            name:  "recommended braced format",
            input: "prefix_${HOME}_suffix",
            expected: []VariableRef{
                {Name: "HOME", StartPos: 7, EndPos: 14, Format: FormatBraced, FullMatch: "${HOME}"},
            },
        },
        {
            name:  "glob patterns as literals",
            input: "$HOME/*.txt",
            expected: []VariableRef{
                // * はリテラル文字として扱われる
                {Name: "HOME", StartPos: 0, EndPos: 5, Format: FormatSimple, FullMatch: "$HOME"},
            },
        },
        {
            name:     "no variables",
            input:    "/usr/bin/ls",
            expected: []VariableRef{},
        },
    }

    parser := NewVariableParser()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := parser.ExtractVariables(tt.input)
            require.NoError(t, err)
            assert.Equal(t, tt.expected, result)
        })
    }
}

func TestVariableExpander_ExpandCommand(t *testing.T) {
    tests := []struct {
        name      string
        cmd       string
        env       map[string]string
        allowlist []string
        expected  string
        expectErr bool
    }{
        {
            name: "simple expansion",
            cmd:  "$DOCKER_CMD",
            env:  map[string]string{"DOCKER_CMD": "/usr/bin/docker"},
            allowlist: []string{},
            expected:  "/usr/bin/docker",
            expectErr: false,
        },
        {
            name: "braced expansion",
            cmd:  "${TOOL_DIR}/script",
            env:  map[string]string{"TOOL_DIR": "/opt/tools"},
            allowlist: []string{},
            expected:  "/opt/tools/script",
            expectErr: false,
        },
        {
            name: "security violation",
            cmd:  "$FORBIDDEN_VAR",
            env:  map[string]string{},
            allowlist: []string{},
            expected:  "",
            expectErr: true,
        },
        {
            name: "glob pattern preserved as literal",
            cmd:  "${FIND_CMD}",
            env:  map[string]string{"FIND_CMD": "/usr/bin/find /path/*.txt"},
            allowlist: []string{},
            expected:  "/usr/bin/find /path/*.txt", // * はリテラルとして保持
            expectErr: false,
        },
    }

    expander := NewVariableExpander(nil, 10)
    ctx := context.Background()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := expander.ExpandCommand(ctx, tt.cmd, tt.env, tt.allowlist)
            if tt.expectErr {
                assert.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}

func TestCircularReferenceDetector(t *testing.T) {
    tests := []struct {
        name      string
        env       map[string]string
        expectErr bool
    }{
        {
            name: "no circular reference",
            env: map[string]string{
                "A": "value_a",
                "B": "$A",
                "C": "$B",
            },
            expectErr: false,
        },
        {
            name: "direct circular reference",
            env: map[string]string{
                "A": "$B",
                "B": "$A",
            },
            expectErr: true,
        },
        {
            name: "indirect circular reference",
            env: map[string]string{
                "A": "$B",
                "B": "$C",
                "C": "$A",
            },
            expectErr: true,
        },
    }

    detector := NewCircularReferenceDetector(10)

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := detector.DetectCircularReference(tt.env)
            if tt.expectErr {
                assert.Error(t, err)
                assert.True(t, IsCircularReferenceError(err))
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 1.9 性能仕様

#### 1.9.1 ベンチマークテスト

```go
func BenchmarkVariableExpansion(b *testing.B) {
    expander := NewVariableExpander(nil, 10)
    ctx := context.Background()
    env := map[string]string{
        "HOME": "/home/user",
        "BIN":  "/usr/bin",
        "APP":  "myapp",
        "PATTERN": "*.txt", // グロブパターンはリテラル扱い
    }
    allowlist := []string{}

    b.Run("simple_expansion", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _, err := expander.ExpandCommand(ctx, "$HOME/bin/$APP", env, allowlist)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("complex_args", func(b *testing.B) {
        args := []string{"--input", "$HOME/data", "--output", "${BIN}/output"}
        for i := 0; i < b.N; i++ {
            _, err := expander.ExpandArgs(ctx, args, env, allowlist)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("braced_format_recommended", func(b *testing.B) {
        // prefix_${VAR}_suffix 形式（推奨）
        for i := 0; i < b.N; i++ {
            _, err := expander.ExpandCommand(ctx, "prefix_${APP}_suffix", env, allowlist)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("glob_pattern_literal", func(b *testing.B) {
        // グロブパターンがリテラル扱いされることを確認
        args := []string{"$HOME/$PATTERN"}
        for i := 0; i < b.N; i++ {
            result, err := expander.ExpandArgs(ctx, args, env, allowlist)
            if err != nil {
                b.Fatal(err)
            }
            // 結果は "/home/user/*.txt" となる（* は展開されない）
            if result[0] != "/home/user/*.txt" {
                b.Fatalf("Expected '/home/user/*.txt', got '%s'", result[0])
            }
        }
    })
}
```

### 1.10 統合仕様

#### 1.10.1 既存コードとの統合ポイント

1. **Config Parser統合**: 設定読み込み時に変数展開を実行
2. **Environment Processor統合**: 既存の環境変数処理と連携
3. **Command Executor統合**: 展開後のコマンド実行
4. **Error Handling統合**: 既存のエラーハンドリングシステムとの統合

#### 1.10.2 互換性保証

- 環境変数参照のない設定ファイルは無変更で動作
- 既存のCommand.Env処理は変更なし
- エラー処理の一貫性維持
- ログ出力形式の統一

この詳細仕様に基づいて実装を進めることで、要件定義とアーキテクチャ設計に適合した堅牢で高性能な変数展開機能を実現できます。
