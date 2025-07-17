# 詳細仕様書: 環境変数安全化機能の実装

## 1. 概要

本仕様書では、環境変数安全化機能の詳細な実装仕様を定義する。設定ファイルで明示的に許可された環境変数のみを使用することで、セキュリティリスクを排除する。

## 2. データ構造定義

### 2.1 設定ファイル構造拡張

#### 2.1.1 global セクション拡張

```toml
# config.toml
[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"

# 新規追加: 許可する環境変数のリスト
env_allowlist = [
    "PATH",        # コマンド検索パス
    "HOME",        # ホームディレクトリ
    "USER",        # ユーザー名
    "LANG",        # 言語設定
    "TERM",        # ターミナルタイプ
    "TMPDIR"       # 一時ディレクトリ
]
```

#### 2.1.2 groups セクション拡張

```toml
[[groups]]
name = "web-server"
description = "Web server deployment and management"

# 新規追加: グループ固有の許可環境変数
# 重要: env_allowlistが定義されている場合、global.env_allowlistは無視される
env_allowlist = [
    "PATH",           # 基本パス
    "HOME",           # ホームディレクトリ
    "NODE_ENV",       # Node.js環境設定
    "PORT",           # ポート番号
    "DATABASE_URL",   # データベース接続URL
    "API_KEY"         # API認証キー
]

# 既存: コマンド定義（env 設定は既存通り動作）
[[groups.commands]]
name = "start_server"
cmd = "node"
args = ["server.js"]
# env設定での変数参照は、env_allowlistで許可された変数のみ使用可能
env = [
    "DEBUG=app:*",
    "NODE_ENV=${NODE_ENV}",     # env_allowlistで許可が必要
    "PORT=${PORT}"              # env_allowlistで許可が必要
]

[[groups.commands]]
name = "database_migrate"
cmd = "npm"
args = ["run", "migrate"]
env = ["DATABASE_URL=${DATABASE_URL}"]  # env_allowlistで許可が必要

# 明示的拒否の例
[[groups]]
name = "secure_batch"
env_allowlist = []  # 環境変数なし（globalの設定も無視）

[[groups.commands]]
name = "secure_command"
cmd = "/usr/bin/echo"  # 絶対パス必須（PATHなし）
args = ["Hello World"]

# グローバル継承の例
[[groups]]
name = "inherit_group"
# env_allowlist未定義 → global.env_allowlistを継承

[[groups.commands]]
name = "basic_command"
cmd = "ls"  # global.env_allowlistでPATHが許可されていれば動作
args = ["-la"]
```

### 2.2 Go構造体定義

#### 2.2.1 設定構造体拡張

```go
// internal/runner/runnertypes/config.go
package runnertypes

// GlobalConfig に EnvAllowlist フィールドを追加
type GlobalConfig struct {
    // 既存フィールド
    Timeout           int      `toml:"timeout" json:"timeout"`
    WorkDir           string   `toml:"workdir" json:"workdir"`
    LogLevel          string   `toml:"log_level" json:"log_level"`
    VerifyFiles       []string `toml:"verify_files" json:"verify_files"`
    SkipStandardPaths bool     `toml:"skip_standard_paths" json:"skip_standard_paths"`

    // 新規追加
    EnvAllowlist           []string `toml:"env_allowlist" json:"env_allowlist"`  // 許可する環境変数
}

// CommandGroup に EnvAllowlist フィールドを追加
type CommandGroup struct {
    // 既存フィールド
    Name        string    `toml:"name" json:"name"`
    Description string    `toml:"description" json:"description"`
    Priority    int       `toml:"priority" json:"priority"`
    DependsOn   []string  `toml:"depends_on" json:"depends_on"`
    Template    string    `toml:"template" json:"template"`
    Commands    []Command `toml:"commands" json:"commands"`
    VerifyFiles []string  `toml:"verify_files" json:"verify_files"`

    // 新規追加
    EnvAllowlist     []string  `toml:"env_allowlist" json:"env_allowlist"`  // グループ固有の許可環境変数
}

// Command構造体は変更なし（既存のenv設定を維持）
type Command struct {
    Name        string   `toml:"name" json:"name"`
    Description string   `toml:"description" json:"description"`
    Cmd         string   `toml:"cmd" json:"cmd"`
    Args        []string `toml:"args" json:"args"`
    Env         []string `toml:"env" json:"env"`        // 既存: コマンド固有環境変数
    Dir         string   `toml:"dir" json:"dir"`
    Privileged  bool     `toml:"privileged" json:"privileged"`
    Timeout     int      `toml:"timeout" json:"timeout"`
}
```

#### 2.2.2 環境変数管理構造体

```go
// internal/runner/environment/filter.go - 新規パッケージ
package environment

import (
    "fmt"
    "log"
    "regexp"
    "strings"
)

// Filter は環境変数のフィルタリング機能を提供
type Filter struct {
    allowedVars    map[string]bool   // 許可された環境変数の高速検索用
    logger         *log.Logger       // 通常ログ
    securityLogger *log.Logger       // セキュリティ監査ログ
    config         FilterConfig      // フィルタ設定
}

// FilterConfig はフィルタリングの設定を定義
type FilterConfig struct {
    MaxVariableNameLength  int    // 変数名の最大長（デフォルト: 256）
    MaxVariableValueLength int    // 変数値の最大長（デフォルト: 4096）
    AllowEmptyValues      bool   // 空値を許可するか（デフォルト: true）
    StrictNameValidation  bool   // 厳密な変数名検証（デフォルト: true）
}

// NewFilter は新しいFilterインスタンスを作成
func NewFilter(allowedVars []string, config FilterConfig) *Filter {
    allowedMap := make(map[string]bool)
    for _, v := range allowedVars {
        allowedMap[v] = true
    }

    return &Filter{
        allowedVars:    allowedMap,
        logger:         log.New(os.Stdout, "ENV: ", log.LstdFlags),
        securityLogger: log.New(os.Stdout, "SECURITY: ", log.LstdFlags),
        config:         config,
    }
}

// FilterSystemEnvironment はシステム環境変数をフィルタリング
func (f *Filter) FilterSystemEnvironment() map[string]string {
    // システム環境変数の取得とフィルタリング実装
}

// FilterEnvFileVariables は.envファイルからの環境変数をフィルタリング
func (f *Filter) FilterEnvFileVariables(envVars map[string]string) map[string]string {
    filtered := make(map[string]string)
    for key, value := range envVars {
        if f.allowedVars[key] {
            filtered[key] = value
        } else {
            f.securityLogger.Printf("Denied .env variable: %s", key)
        }
    }
    return filtered
}

// ValidateVariableReference は変数参照の妥当性を検証
func (f *Filter) ValidateVariableReference(varName string) error {
    if !f.allowedVars[varName] {
        return fmt.Errorf("variable %s is not in allowlist", varName)
    }
    return nil
}

// ValidationResult はバリデーション結果を表す
type ValidationResult struct {
    IsValid      bool     // バリデーション成功フラグ
    ErrorMessage string   // エラーメッセージ
    FilteredVars []string // フィルタリングされた変数名
    AllowedVars  []string // 許可された変数名
}
```

#### 2.2.3 拡張されたRunner構造体

```go
// internal/runner/runner.go
type Runner struct {
    // 既存フィールド
    config         *runnertypes.Config
    envVars        map[string]string
    templateEngine *template.Engine
    validator      *security.Validator

    // 新規追加
    envFilter      *environment.Filter    // 環境変数フィルタ
    globalEnvAllowlist  map[string]bool       // グローバル許可環境変数（高速検索用）
    groupEnvAllowlist   map[string]map[string]bool  // グループ別許可環境変数
}

// EnvironmentContext はコマンド実行時の環境変数コンテキスト
type EnvironmentContext struct {
    GlobalVars    map[string]string  // グローバル環境変数
    GroupVars     map[string]string  // グループ環境変数
    CommandVars   map[string]string  // コマンド固有環境変数
    ResolvedVars  map[string]string  // 最終解決済み環境変数
    AllowedVars   []string          // 許可された変数リスト
}
```

### 2.3 エラー定義

```go
// internal/runner/errors.go
package runner

import "errors"

// 環境変数関連の新規エラー定義
var (
    // 変数名エラー
    ErrInvalidVariableName         = errors.New("invalid environment variable name")
    ErrVariableNameTooLong         = errors.New("environment variable name too long")
    ErrReservedVariableName        = errors.New("reserved environment variable name")

    // 変数値エラー
    ErrVariableValueTooLong        = errors.New("environment variable value too long")
    ErrInvalidVariableValue        = errors.New("invalid environment variable value")

    // アクセス制御エラー
    ErrVariableNotAllowed          = errors.New("environment variable not in allowed list")
    ErrVariableAccessDenied        = errors.New("access denied to environment variable")

    // 参照解決エラー
    ErrCircularReference           = errors.New("circular reference in environment variable")
    ErrUnresolvedVariableReference = errors.New("unresolved environment variable reference")
)

// ErrorWithContext は詳細なコンテキスト情報を含むエラー
type ErrorWithContext struct {
    Err         error
    VariableName string
    GroupName    string
    CommandName  string
    Context      string
}

func (e *ErrorWithContext) Error() string {
    return fmt.Sprintf("%s (variable: %s, group: %s, command: %s, context: %s)",
        e.Err.Error(), e.VariableName, e.GroupName, e.CommandName, e.Context)
}
```

## 3. 関数仕様

### 3.1 環境変数フィルタリング機能

#### 3.1.1 システム環境変数フィルタリング

```go
// internal/runner/runner.go

// LoadEnvironment は環境変数を安全に読み込む（大幅変更）
func (r *Runner) LoadEnvironment(envFile string, loadSystemEnv bool) error {
    // 1. 許可リスト構築（env_allowlistが未定義の場合は空リスト）
    if err := r.buildAllowedVariableMaps(); err != nil {
        return fmt.Errorf("failed to build allowed variable maps: %w", err)
    }

    envMap := make(map[string]string)

    // 2. システム環境変数フィルタリング
    if loadSystemEnv {
        // env_allowlistが未定義（空）の場合、すべての環境変数をフィルタ
        filteredEnv := r.filterSystemEnvironment(r.config.Global.EnvAllowlist)
        for k, v := range filteredEnv {
            envMap[k] = v
        }
        r.logEnvironmentFiltering(r.config.Global.EnvAllowlist, filteredEnv)
    }

    // 3. .env ファイル読み込み（セキュリティ強化）
    if envFile != "" {
        if err := r.validator.ValidateFilePermissions(envFile); err != nil {
            return fmt.Errorf("security validation failed for environment file: %w", err)
        }

        fileEnv, err := godotenv.Read(envFile)
        if err != nil {
            return fmt.Errorf("failed to load environment file %s: %w", envFile, err)
        }

        // .envファイルの変数にもenv_allowlistフィルタリングを適用（セキュリティ強化）
        filteredFileEnv := r.filterEnvFileVariables(fileEnv, r.config.Global.EnvAllowlist)
        for k, v := range filteredFileEnv {
            if err := r.validateEnvironmentVariable(k, v); err != nil {
                return fmt.Errorf("invalid environment variable %s: %w", k, err)
            }
            envMap[k] = v
        }

        r.logger.Printf("SECURITY: .env file variables filtered - original: %d, allowed: %d",
            len(fileEnv), len(filteredFileEnv))
    }

    // 4. 最終検証
    if err := r.validateEnvironmentSecurity(envMap); err != nil {
        return fmt.Errorf("environment variable security validation failed: %w", err)
    }

    r.envVars = envMap
    return nil
}

// filterSystemEnvironment はシステム環境変数をフィルタリング
func (r *Runner) filterSystemEnvironment(allowedVars []string) map[string]string {
    allowed := make(map[string]bool)
    for _, v := range allowedVars {
        allowed[v] = true
    }

    filtered := make(map[string]string)
    originalCount := 0

    for _, env := range os.Environ() {
        originalCount++
        if i := strings.Index(env, "="); i >= 0 {
            key := env[:i]
            value := env[i+1:]

            if allowed[key] {
                filtered[key] = value
            }
        }
    }

    // セキュリティ監査ログ
    r.logger.Printf("SECURITY: System environment filtered - original: %d, allowed: %d",
        originalCount, len(filtered))

    return filtered
}

// filterEnvFileVariables は.envファイルからの環境変数をフィルタリング
func (r *Runner) filterEnvFileVariables(envVars map[string]string, allowedVars []string) map[string]string {
    allowed := make(map[string]bool)
    for _, v := range allowedVars {
        allowed[v] = true
    }

    filtered := make(map[string]string)

    for key, value := range envVars {
        if allowed[key] {
            filtered[key] = value
        } else {
            r.logger.Printf("SECURITY: .env variable '%s' denied - not in env_allowlist", key)
        }
    }

    return filtered
}

// buildAllowedVariableMaps は許可変数の高速検索マップを構築
func (r *Runner) buildAllowedVariableMaps() error {
    // グローバル許可変数マップ（空の場合でも正常処理）
    r.globalEnvAllowlist = make(map[string]bool)
    for _, v := range r.config.Global.EnvAllowlist {
        if err := r.validateVariableName(v); err != nil {
            return fmt.Errorf("invalid global env var %s: %w", v, err)
        }
        r.globalEnvAllowlist[v] = true
    }

    // グループ別許可変数マップ
    r.groupEnvAllowlist = make(map[string]map[string]bool)
    for _, group := range r.config.Groups {
        groupMap := make(map[string]bool)

        // グループでenv_allowlistが定義されているかチェック
        if group.EnvAllowlist != nil {
            // 定義されている場合（空リスト含む）、その設定を使用
            for _, v := range group.EnvAllowlist {
                if err := r.validateVariableName(v); err != nil {
                    return fmt.Errorf("invalid group env var %s in group %s: %w", v, group.Name, err)
                }
                groupMap[v] = true
            }
        } else {
            // 未定義の場合、global.env_allowlistを継承
            for _, v := range r.config.Global.EnvAllowlist {
                groupMap[v] = true
            }
        }

        r.groupEnvAllowlist[group.Name] = groupMap
    }

    return nil
}
```

#### 3.1.2 グループレベル環境変数解決

```go
// resolveGroupEnvironmentVars はグループ固有の環境変数を解決
func (r *Runner) resolveGroupEnvironmentVars(group runnertypes.CommandGroup) (map[string]string, error) {
    var allowedVars []string

    // グループでenv_allowlistが定義されているかチェック
    if group.EnvAllowlist != nil {
        // 定義されている場合（空リスト含む）、その設定を使用（globalは無視）
        allowedVars = group.EnvAllowlist
        if len(allowedVars) == 0 {
            r.logger.Printf("INFO: Group '%s' has empty env_allowlist, no environment variables will be available", group.Name)
            return make(map[string]string), nil
        }
    } else {
        // 未定義の場合、global.env_allowlistを継承
        allowedVars = r.config.Global.EnvAllowlist
        if len(allowedVars) == 0 {
            r.logger.Printf("INFO: Group '%s' inherits empty global.env_allowlist, no environment variables will be available", group.Name)
            return make(map[string]string), nil
        }
        r.logger.Printf("DEBUG: Group '%s' inheriting global env_allowlist: %v", group.Name, allowedVars)
    }

    // 許可された変数のみをベース環境変数（システム環境変数+.envファイル、両方フィルタ済み）から抽出
    groupEnv := make(map[string]string)
    allowedMap := make(map[string]bool)
    for _, v := range allowedVars {
        allowedMap[v] = true
    }

    for key, value := range r.envVars {
        if allowedMap[key] {
            groupEnv[key] = value
        }
    }

    r.logger.Printf("DEBUG: Group '%s' environment variables: %v", group.Name,
        r.getVariableNames(groupEnv))

    return groupEnv, nil
}

// resolveEnvironmentVars はコマンド実行時の環境変数を解決（大幅変更）
func (r *Runner) resolveEnvironmentVars(cmd runnertypes.Command, groupEnv map[string]string) (map[string]string, error) {
    // 1. グループ環境変数をベースとして開始
    envVars := make(map[string]string)
    for k, v := range groupEnv {
        envVars[k] = v
    }

    // 2. コマンド固有の環境変数を追加・上書き
    for _, env := range cmd.Env {
        parts := strings.SplitN(env, "=", 2)
        if len(parts) == 2 {
            key := parts[0]
            value := parts[1]

            // 変数参照を解決
            resolvedValue, err := r.resolveVariableReferences(value, envVars, cmd.Name)
            if err != nil {
                return nil, fmt.Errorf("failed to resolve variable %s in command %s: %w",
                    key, cmd.Name, err)
            }

            envVars[key] = resolvedValue
        }
    }

    return envVars, nil
}

// resolveVariableReferences は${VAR}形式の変数参照を解決（セキュリティ強化）
func (r *Runner) resolveVariableReferences(value string, envVars map[string]string, groupName string) (string, error) {
    return r.resolveVariableReferencesWithDepth(value, envVars, make(map[string]bool), 0, groupName)
}

func (r *Runner) resolveVariableReferencesWithDepth(value string, envVars map[string]string, resolving map[string]bool, depth int, groupName string) (string, error) {
    if depth > maxResolutionDepth {
        return "", fmt.Errorf("%w: maximum resolution depth exceeded (%d)", ErrCircularReference, maxResolutionDepth)
    }

    result := value
    iterations := 0

    for strings.Contains(result, "${") {
        iterations++
        if iterations > maxResolutionDepth {
            return "", fmt.Errorf("%w: too many resolution iterations", ErrCircularReference)
        }

        // ${VAR} パターンを検索
        start := strings.Index(result, "${")
        if start == -1 {
            break
        }

        end := strings.Index(result[start:], "}")
        if end == -1 {
            return "", fmt.Errorf("unclosed variable reference in: %s", value)
        }
        end += start

        // 変数名を抽出
        varName := result[start+2 : end]
        if varName == "" {
            return "", fmt.Errorf("empty variable reference in: %s", value)
        }

        // セキュリティチェック: 参照される変数が許可リストにあるか確認
        if !r.isVariableAccessAllowed(varName, groupName) {
            return "", fmt.Errorf("%w: variable %s not allowed for group %s",
                ErrVariableNotAllowed, varName, groupName)
        }

        // 循環参照チェック
        if resolving[varName] {
            return "", fmt.Errorf("%w: variable %s", ErrCircularReference, varName)
        }

        // 変数値を取得
        varValue, exists := envVars[varName]
        if !exists {
            return "", fmt.Errorf("%w: variable %s", ErrUnresolvedVariableReference, varName)
        }

        // 再帰的に解決
        resolving[varName] = true
        resolvedVarValue, err := r.resolveVariableReferencesWithDepth(varValue, envVars, resolving, depth+1, groupName)
        delete(resolving, varName)

        if err != nil {
            return "", err
        }

        // 置換実行
        result = result[:start] + resolvedVarValue + result[end+1:]
    }

    return result, nil
}
```

### 3.2 検証機能

#### 3.2.1 環境変数名検証

```go
// validateVariableName は環境変数名の妥当性を検証
func (r *Runner) validateVariableName(name string) error {
    if name == "" {
        return ErrInvalidVariableName
    }

    if len(name) > maxVariableNameLength {
        return ErrVariableNameTooLong
    }

    // 変数名パターン検証（英数字とアンダースコアのみ）
    validNamePattern := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
    if !validNamePattern.MatchString(name) {
        return ErrInvalidVariableName
    }

    // 予約変数名チェック
    reservedNames := []string{"PWD", "OLDPWD", "PS1", "PS2"}
    for _, reserved := range reservedNames {
        if name == reserved {
            return ErrReservedVariableName
        }
    }

    return nil
}

// validateVariableValue は環境変数値の妥当性を検証
func (r *Runner) validateVariableValue(value string) error {
    if len(value) > maxVariableValueLength {
        return ErrVariableValueTooLong
    }

    // 危険な文字列パターンチェック
    if strings.Contains(value, "\x00") {
        return ErrInvalidVariableValue
    }

    // 制御文字チェック（改行、タブは許可）
    for _, char := range value {
        if char < 32 && char != '\n' && char != '\t' {
            return ErrInvalidVariableValue
        }
    }

    return nil
}

// validateEnvironmentVariable は環境変数名と値の組み合わせを検証
func (r *Runner) validateEnvironmentVariable(name, value string) error {
    if err := r.validateVariableName(name); err != nil {
        return err
    }

    if err := r.validateVariableValue(value); err != nil {
        return err
    }

    return nil
}
```

#### 3.2.2 アクセス制御検証

```go
// isVariableAccessAllowed は変数アクセスが許可されているかチェック
func (r *Runner) isVariableAccessAllowed(varName string, groupName string) bool {
    // グローバル許可リストをチェック
    if r.globalEnvAllowlist[varName] {
        return true
    }

    // 指定されたグループの許可リストをチェック
    if groupVars, exists := r.groupEnvAllowlist[groupName]; exists {
        return groupVars[varName]
    }

    return false
}

// validateEnvironmentSecurity は環境変数全体のセキュリティを検証
func (r *Runner) validateEnvironmentSecurity(envVars map[string]string) error {
    for name, value := range envVars {
        if err := r.validateEnvironmentVariable(name, value); err != nil {
            return fmt.Errorf("security validation failed for %s: %w", name, err)
        }
    }

    // 機密情報検出（基本的なパターンマッチング）
    for name, value := range envVars {
        if r.containsSensitiveData(name, value) {
            r.securityLogger.Printf("SECURITY: Potential sensitive data in variable %s", name)
        }
    }

    return nil
}

// containsSensitiveData は機密情報の可能性をチェック
func (r *Runner) containsSensitiveData(name, value string) bool {
    sensitivePatternsInName := []string{"PASSWORD", "SECRET", "KEY", "TOKEN", "CREDENTIAL"}
    for _, pattern := range sensitivePatternsInName {
        if strings.Contains(strings.ToUpper(name), pattern) {
            return true
        }
    }

    // 値のパターンチェック（JWT、APIキー等）
    if len(value) > 20 && (strings.HasPrefix(value, "eyJ") || // JWT
        regexp.MustCompile(`^[A-Za-z0-9_-]{20,}$`).MatchString(value)) { // APIキー形式
        return true
    }

    return false
}
```

### 3.3 ログとモニタリング機能

#### 3.3.1 ログ出力機能

```go
// logEnvironmentFiltering は環境変数フィルタリング結果をログ出力
func (r *Runner) logEnvironmentFiltering(allowedVars []string, filteredVars map[string]string) {
    originalCount := 0
    for range os.Environ() {
        originalCount++
    }

    filteredOut := originalCount - len(filteredVars)

    // セキュリティ監査ログ
    r.securityLogger.Printf("Environment filtering completed - original: %d, allowed: %d, filtered out: %d",
        originalCount, len(filteredVars), filteredOut)

    // デバッグログ（デバッグレベル時のみ）
    if r.config.Global.LogLevel == "debug" {
        r.logger.Printf("DEBUG: Allowed global env vars: %v", allowedVars)
        r.logger.Printf("DEBUG: Final filtered env vars: %v", r.getVariableNames(filteredVars))
    }
}

// logVariableAccess は変数アクセスをログに記録
func (r *Runner) logVariableAccess(varName string, groupName string, allowed bool) {
    if allowed {
        r.logger.Printf("DEBUG: Variable access granted - var: %s, group: %s", varName, groupName)
    } else {
        r.securityLogger.Printf("SECURITY: Variable access denied - var: %s, group: %s", varName, groupName)
    }
}

// getVariableNames は環境変数マップから変数名のリストを取得（デバッグ用）
func (r *Runner) getVariableNames(envVars map[string]string) []string {
    names := make([]string, 0, len(envVars))
    for name := range envVars {
        // 機密情報の可能性がある変数は値をマスク
        if r.containsSensitiveData(name, envVars[name]) {
            names = append(names, name+"=***")
        } else {
            names = append(names, name)
        }
    }
    return names
}
```

## 4. 設定ファイル仕様

### 4.1 TOML設定形式

#### 4.1.1 完全な設定例

```toml
# config.toml - 環境変数安全化機能対応版
version = "1.0"

[global]
timeout = 3600
workdir = "/tmp/cmd-runner"
log_level = "info"

# オプション: グローバル許可環境変数（未定義時は環境変数なし）
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "TERM",
    "TMPDIR"
]

# Web サーバー管理グループ
[[groups]]
name = "web-server"
description = "Web server deployment and management"
priority = 1

# オプション: グループ固有許可環境変数
# 重要: env_allowlistが定義されている場合、global.env_allowlistは無視される
env_allowlist = [
    "PATH",           # 基本パス
    "HOME",           # ホームディレクトリ
    "NODE_ENV",       # Node.js環境
    "PORT",           # サーバーポート
    "DATABASE_URL"    # DB接続URL
]

[[groups.commands]]
name = "start_server"
description = "Start the web server"
cmd = "node"
args = ["server.js"]
env = [
    "DEBUG=app:*",
    "NODE_ENV=${NODE_ENV}",    # env_allowlistで許可が必要
    "PORT=${PORT}"             # env_allowlistで許可が必要
]

# データベース管理グループ（明示的拒否の例）
[[groups]]
name = "secure_database"
description = "Secure database operations"
priority = 2

env_allowlist = []  # 環境変数なし（globalの設定も無視）

[[groups.commands]]
name = "secure_backup"
description = "Secure database backup"
cmd = "/usr/bin/pg_dump"  # 絶対パス必須（PATHなし）
args = ["--host=localhost", "mydb"]

# 基本タスクグループ（グローバル継承の例）
[[groups]]
name = "basic_tasks"
description = "Basic system tasks"
priority = 3
# env_allowlist未定義 → global.env_allowlist ["PATH", "HOME", "USER"]を継承

[[groups.commands]]
name = "list_files"
description = "List files"
cmd = "ls"  # global.env_allowlistでPATHが許可されていれば動作
args = ["-la"]
```

#### 4.1.2 最小限の設定例

```toml
# minimal-config.toml
version = "1.0"

[global]
# env_allowlist未定義（環境変数なしで実行）

[[groups]]
name = "basic"
# env_allowlist未定義（global.env_allowlistを継承、この場合は環境変数なし）

[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"  # 絶対パス必須（PATHなし）
args = ["-la"]

[[groups]]
name = "secure"
env_allowlist = []  # 明示的に環境変数なし

[[groups.commands]]
name = "secure_echo"
cmd = "/bin/echo"  # 絶対パス必須
args = ["secure operation"]
```

### 4.2 設定検証ルール

#### 4.2.1 必須フィールド検証

```go
// internal/runner/config/validator.go - 新規
package config

// ValidateEnvAllowlistConfiguration は env_allowlist 設定の妥当性を検証
func ValidateEnvAllowlistConfiguration(config *runnertypes.Config) error {
    // Global env_allowlist は未定義でも正常（空リスト扱い）

    // 各グループの env_allowlist も未定義でも正常
    for i, group := range config.Groups {
        // グループ内のコマンドで使用される変数がenv_allowlistに含まれているかチェック
        // （env_allowlistが未定義の場合、変数参照があるとエラー）
        if err := validateCommandEnvironmentReferences(group); err != nil {
            return fmt.Errorf("validation failed for group '%s': %w", group.Name, err)
        }
    }

    return nil
}

// validateCommandEnvironmentReferences はコマンドの環境変数参照を検証
func validateCommandEnvironmentReferences(group runnertypes.CommandGroup) error {
    var allowedVars []string

    // グループでenv_allowlistが定義されているかチェック
    if group.EnvAllowlist != nil {
        // 定義されている場合（空リスト含む）、その設定を使用
        allowedVars = group.EnvAllowlist
    } else {
        // 未定義の場合、このバリデーションではスキップ
        // （実行時にglobal.env_allowlistを継承する）
        return nil
    }

    allowedVarMap := make(map[string]bool)
    for _, v := range allowedVars {
        allowedVarMap[v] = true
    }

    for _, cmd := range group.Commands {
        for _, env := range cmd.Env {
            // ${VAR} 形式の参照を抽出
            refs := extractVariableReferences(env)
            for _, ref := range refs {
                if !allowedVarMap[ref] {
                    if len(allowedVars) == 0 {
                        return fmt.Errorf("command '%s' references variable '%s' but group has empty env_allowlist",
                            cmd.Name, ref)
                    } else {
                        return fmt.Errorf("command '%s' references variable '%s' not in group env_allowlist",
                            cmd.Name, ref)
                    }
                }
            }
        }
    }

    return nil
}

// extractVariableReferences は文字列から${VAR}形式の参照を抽出
func extractVariableReferences(s string) []string {
    re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
    matches := re.FindAllStringSubmatch(s, -1)

    refs := make([]string, 0, len(matches))
    for _, match := range matches {
        if len(match) > 1 {
            refs = append(refs, match[1])
        }
    }

    return refs
}
```

## 5. エラーハンドリング仕様

### 5.1 エラー分類と処理方針

#### 5.1.1 致命的エラー（アプリケーション終了）

```go
// セキュリティエラー
ErrVariableNotAllowed          // 許可されていない変数参照 → セキュリティ違反として終了
ErrVariableAccessDenied        // 変数アクセス拒否 → セキュリティ違反として終了

// 処理方針
func handleFatalError(err error) {
    log.Printf("FATAL: %v", err)
    log.Printf("Application will terminate for security reasons")
    os.Exit(1)
}
```

#### 5.1.2 警告レベルエラー（処理継続）

```go
// 値検証エラー（一部ケース）
ErrVariableValueTooLong        // 値が長すぎる → ログ出力して継続
ErrInvalidVariableValue        // 値形式不正 → ログ出力して継続（軽微な場合）

// 処理方針
func handleWarningError(err error, context string) {
    log.Printf("WARNING: %v (context: %s)", err, context)
    // 処理継続
}
```

### 5.2 エラーメッセージ仕様

#### 5.2.1 ユーザー向けエラーメッセージ

```go
// エラーメッセージのテンプレート
const (
    msgEnvAllowlistNotDefined = `
INFO: No environment variables defined

The configuration file has no 'env_allowlist' defined in [global] or [[groups]] sections.
This means no system environment variables will be inherited, providing maximum security.

If you need environment variables, add them explicitly:

[global]
env_allowlist = ["PATH", "HOME", "USER"]

[[groups]]
name = "example"
env_allowlist = ["PATH", "HOME", "NODE_ENV"]

Only explicitly allowed environment variables will be available.
`

    msgVariableNotAllowed = `
SECURITY ERROR: Environment variable '%s' not allowed

The variable '%s' referenced in command '%s' is not included in the env_allowlist list.

Add '%s' to the env_allowlist list in the configuration file:

[[groups]]
name = "%s"
env_allowlist = ["existing_vars", "%s"]

This restriction exists for security purposes.
`

    msgInvalidVariableName = `
CONFIGURATION ERROR: Invalid environment variable name '%s'

Environment variable names must:
- Start with a letter or underscore
- Contain only letters, numbers, and underscores
- Be no longer than %d characters
- Not be a reserved name

Reserved names: %v
`
)

// formatUserError はユーザー向けエラーメッセージを生成
func formatUserError(err error, context map[string]string) string {
    switch err {
    case ErrVariableNotAllowed:
        return fmt.Sprintf(msgVariableNotAllowed,
            context["variable"], context["variable"], context["command"],
            context["variable"], context["group"], context["variable"])

    case ErrInvalidVariableName:
        return fmt.Sprintf(msgInvalidVariableName,
            context["variable"], maxVariableNameLength, reservedVariableNames)

    default:
        return fmt.Sprintf("Error: %v", err)
    }
}
```

## 6. テスト仕様

### 6.1 単体テスト仕様

#### 6.1.1 環境変数フィルタリングテスト

```go
// internal/runner/runner_test.go
func TestRunner_filterSystemEnvironment(t *testing.T) {
    tests := []struct {
        name         string
        allowedVars  []string
        systemEnv    map[string]string
        expected     map[string]string
    }{
        {
            name:        "basic filtering",
            allowedVars: []string{"PATH", "HOME"},
            systemEnv: map[string]string{
                "PATH":      "/usr/bin:/bin",
                "HOME":      "/home/user",
                "SECRET":    "should_be_filtered",
                "MALWARE":   "dangerous_value",
            },
            expected: map[string]string{
                "PATH": "/usr/bin:/bin",
                "HOME": "/home/user",
            },
        },
        {
            name:        "empty allowed list",
            allowedVars: []string{},
            systemEnv: map[string]string{
                "PATH": "/usr/bin:/bin",
                "HOME": "/home/user",
            },
            expected: map[string]string{}, // 空リストの場合、すべてフィルタ
        },
        {
            name:        "no matching variables",
            allowedVars: []string{"CUSTOM_VAR"},
            systemEnv: map[string]string{
                "PATH": "/usr/bin:/bin",
                "HOME": "/home/user",
            },
            expected: map[string]string{},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テスト用環境変数設定
            cleanup := setupTestEnvironment(t, tt.systemEnv)
            defer cleanup()

            runner := &Runner{}
            result := runner.filterSystemEnvironment(tt.allowedVars)

            assert.Equal(t, tt.expected, result)
        })
    }
}
```

#### 6.1.2 変数参照解決テスト

```go
func TestRunner_resolveVariableReferences(t *testing.T) {
    tests := []struct {
        name        string
        value       string
        envVars     map[string]string
        allowedVars []string
        expected    string
        expectError bool
    }{
        {
            name:  "simple reference",
            value: "prefix_${VAR1}_suffix",
            envVars: map[string]string{
                "VAR1": "value1",
            },
            allowedVars: []string{"VAR1"},
            expected:    "prefix_value1_suffix",
            expectError: false,
        },
        {
            name:  "variable not allowed",
            value: "${FORBIDDEN_VAR}",
            envVars: map[string]string{
                "FORBIDDEN_VAR": "secret",
            },
            allowedVars: []string{"ALLOWED_VAR"},
            expected:    "",
            expectError: true,
        },
        {
            name:  "circular reference",
            value: "${VAR1}",
            envVars: map[string]string{
                "VAR1": "${VAR2}",
                "VAR2": "${VAR1}",
            },
            allowedVars: []string{"VAR1", "VAR2"},
            expected:    "",
            expectError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            runner := setupRunnerWithAllowedVars(tt.allowedVars)

            result, err := runner.resolveVariableReferences(tt.value, tt.envVars, "test_command")

            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}
```

### 6.2 統合テスト仕様

#### 6.2.1 設定ファイル〜コマンド実行統合テスト

```go
func TestRunner_Integration_EnvironmentSecurity(t *testing.T) {
    // テスト用設定ファイル作成
    configContent := `
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME", "TEST_VAR"]

[[groups]]
name = "test_group"
env_allowlist = ["PATH", "HOME", "GROUP_VAR"]

[[groups.commands]]
name = "test_command"
cmd = "echo"
args = ["${GROUP_VAR}"]
env = ["GROUP_VAR=test_value"]
`

    // 一時設定ファイル作成
    configFile := createTempConfig(t, configContent)
    defer os.Remove(configFile)

    // システム環境変数設定（攻撃シミュレーション）
    maliciousEnv := map[string]string{
        "PATH":         "/usr/bin:/bin",
        "HOME":         "/home/test",
        "TEST_VAR":     "allowed_value",
        "MALICIOUS":    "should_be_filtered",
        "EVIL_COMMAND": "rm -rf /",
    }
    cleanup := setupTestEnvironment(t, maliciousEnv)
    defer cleanup()

    // Runner作成と実行
    runner, err := NewRunnerFromConfig(configFile)
    require.NoError(t, err)

    err = runner.LoadEnvironment("", true)
    require.NoError(t, err)

    // 実行後の環境変数チェック
    // MALICIOUSとEVIL_COMMANDは除外されているべき
    assert.NotContains(t, runner.envVars, "MALICIOUS")
    assert.NotContains(t, runner.envVars, "EVIL_COMMAND")
    // env_allowlistで許可された変数のみ存在
    assert.Contains(t, runner.envVars, "PATH")
    assert.Contains(t, runner.envVars, "HOME")
    assert.Contains(t, runner.envVars, "TEST_VAR")
}
```

### 6.3 セキュリティテスト仕様

#### 6.3.1 攻撃シナリオテスト

```go
func TestSecurity_EnvironmentVariableInjection(t *testing.T) {
    attackScenarios := []struct {
        name           string
        maliciousEnv   map[string]string
        configEnvAllowlist  []string
        shouldBeBlocked bool
    }{
        {
            name: "PATH manipulation attack",
            maliciousEnv: map[string]string{
                "PATH": "/tmp/malicious:/usr/bin:/bin",
            },
            configEnvAllowlist:  []string{"HOME"}, // PATH not allowed
            shouldBeBlocked: true,
        },
        {
            name: ".env file LD_PRELOAD injection attack",
            maliciousEnv: map[string]string{
                "LD_PRELOAD": "/tmp/malicious.so",
            },
            configEnvAllowlist:  []string{"PATH", "HOME"}, // LD_PRELOAD not allowed
            shouldBeBlocked: true,
        },
        {
            name: ".env file environment bypass attack",
            maliciousEnv: map[string]string{
                "LD_LIBRARY_PATH": "/tmp/malicious",
                "PYTHONPATH": "/tmp/malicious/python",
            },
            configEnvAllowlist:  []string{"PATH"}, // library paths not allowed
            shouldBeBlocked: true,
        },
        {
            name: "LD_PRELOAD injection",
            maliciousEnv: map[string]string{
                "LD_PRELOAD": "/tmp/malicious.so",
            },
            configEnvAllowlist:  []string{"PATH", "HOME"},
            shouldBeBlocked: true,
        },
        {
            name: "Shell injection via env",
            maliciousEnv: map[string]string{
                "SHELL": "/bin/sh; rm -rf /",
            },
            configEnvAllowlist:  []string{"PATH", "HOME"},
            shouldBeBlocked: true,
        },
    }

    for _, scenario := range attackScenarios {
        t.Run(scenario.name, func(t *testing.T) {
            config := &runnertypes.Config{
                Global: runnertypes.GlobalConfig{
                    EnvAllowlist: scenario.configEnvAllowlist,
                },
            }

            runner, err := NewRunner(config)
            require.NoError(t, err)

            // 攻撃環境変数設定
            cleanup := setupTestEnvironment(t, scenario.maliciousEnv)
            defer cleanup()

            // 環境変数フィルタリング実行
            filtered := runner.filterSystemEnvironment(scenario.configEnvAllowlist)

            // 攻撃ベクトルがブロックされているかチェック
            for maliciousVar := range scenario.maliciousEnv {
                if scenario.shouldBeBlocked {
                    assert.NotContains(t, filtered, maliciousVar,
                        "Malicious variable %s should be blocked", maliciousVar)
                }
            }
        })
    }
}
```

## 7. パフォーマンス仕様

### 7.1 処理時間要件

- 環境変数フィルタリング: < 10ms (1000変数時)
- 変数参照解決: < 5ms (100参照時)
- 設定検証: < 50ms

### 7.2 メモリ使用量要件

- 追加メモリオーバーヘッド: < 1MB
- 環境変数マップサイズ: O(許可変数数)

### 7.3 ベンチマークテスト

```go
func BenchmarkRunner_filterSystemEnvironment(b *testing.B) {
    // 1000個の環境変数を設定
    largeEnv := make(map[string]string)
    for i := 0; i < 1000; i++ {
        largeEnv[fmt.Sprintf("VAR_%d", i)] = fmt.Sprintf("value_%d", i)
    }

    allowedVars := []string{"PATH", "HOME", "USER", "LANG", "TERM"}

    cleanup := setupTestEnvironment(b, largeEnv)
    defer cleanup()

    runner := &Runner{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        runner.filterSystemEnvironment(allowedVars)
    }
}
```
