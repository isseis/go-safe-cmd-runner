# 詳細仕様書: セキュリティ検証メカニズムの統一

## 1. 概要

### 1.1 仕様の目的
本仕様書では、パス制限を撤廃し、ハッシュ検証とリスクベース評価による統合セキュリティシステムの詳細実装仕様を定義する。

### 1.2 適用範囲
- `internal/verification/path_resolver.go` の簡素化
- `internal/runner/security/command_analysis.go` の拡張
- 新規ハッシュ検証統合機能
- 新規ディレクトリベースリスク判定機能

## 2. 関数仕様

### 2.1 AnalyzeCommandSecurity 拡張仕様

#### 2.1.1 関数シグネチャ
```go
func AnalyzeCommandSecurity(resolvedPath string, args []string, globalConfig *runnertypes.GlobalConfig) (riskLevel runnertypes.RiskLevel, detectedPattern string, reason string, err error)
```

#### 2.1.2 処理フロー詳細

**Step 1: 入力検証**
```go
if resolvedPath == "" {
    return runnertypes.RiskLevelUnknown, "", "", fmt.Errorf("%w: empty command path", ErrInvalidPath)
}

if !filepath.IsAbs(resolvedPath) {
    return runnertypes.RiskLevelUnknown, "", "", fmt.Errorf("%w: path must be absolute, got relative path: %s", ErrInvalidPath, resolvedPath)
}
```

**Step 2: シンボリックリンク深度チェック**
```go
if _, exceededDepth := extractAllCommandNames(resolvedPath); exceededDepth {
    return runnertypes.RiskLevelHigh, resolvedPath, "Symbolic link depth exceeds security limit (potential symlink attack)", nil
}
```

**Step 3: ディレクトリベースデフォルトリスク判定**
```go
defaultRisk := getDefaultRiskByDirectory(resolvedPath)
```

**Step 4: ハッシュ検証**
```go
if shouldSkipHashValidation(resolvedPath, globalConfig) {
    // ログ出力: hash validation skipped for standard directory
} else {
    if err := validateFileHash(resolvedPath); err != nil {
        return runnertypes.RiskLevelCritical, resolvedPath,
            fmt.Sprintf("Hash validation failed: %v", err), nil
    }
}
```

**Step 5: 高リスクパターン分析**
```go
if riskLevel, pattern, reason := checkCommandPatterns(resolvedPath, args, highRiskPatterns);
   riskLevel != runnertypes.RiskLevelUnknown {
    return riskLevel, pattern, reason, nil
}
```

**Step 6: setuid/setgid チェック**
```go
hasSetuidOrSetgid, setuidErr := hasSetuidOrSetgidBit(resolvedPath)
if setuidErr != nil {
    return runnertypes.RiskLevelHigh, resolvedPath,
        fmt.Sprintf("Unable to check setuid/setgid status: %v", setuidErr), nil
}
if hasSetuidOrSetgid {
    return runnertypes.RiskLevelHigh, resolvedPath,
        "Executable has setuid or setgid bit set", nil
}
```

**Step 7: 中リスクパターン分析**
```go
if riskLevel, pattern, reason := checkCommandPatterns(resolvedPath, args, mediumRiskPatterns);
   riskLevel != runnertypes.RiskLevelUnknown {
    return riskLevel, pattern, reason, nil
}
```

**Step 8: 個別コマンドオーバーライド適用**
```go
if overrideRisk, found := getCommandRiskOverride(resolvedPath); found {
    return overrideRisk, resolvedPath, "Explicit risk level override", nil
}
```

**Step 9: デフォルトリスクレベル適用**
```go
if defaultRisk != runnertypes.RiskLevelUnknown {
    return defaultRisk, "", "Default directory-based risk level", nil
}

// フォールバック: 既存のパターン分析結果を使用
return runnertypes.RiskLevelUnknown, "", "", nil
```

### 2.2 新規関数仕様

#### 2.2.1 getDefaultRiskByDirectory

```go
func getDefaultRiskByDirectory(cmdPath string) runnertypes.RiskLevel {
    dir := filepath.Dir(cmdPath)

    // 1. 完全一致チェック
    if risk, exists := DefaultRiskLevels[dir]; exists {
        return risk
    }

    // 2. プレフィックスマッチ（サブディレクトリ対応）
    for stdDir, risk := range DefaultRiskLevels {
        if strings.HasPrefix(cmdPath, stdDir+"/") {
            return risk
        }
    }

    // 3. デフォルト: 個別分析に委ねる
    return runnertypes.RiskLevelUnknown
}
```

**入力値と期待される出力:**
| 入力パス | 期待される出力 |
|---------|---------------|
| `/bin/ls` | `RiskLevelLow` |
| `/bin/subdir/tool` | `RiskLevelLow` |
| `/usr/sbin/systemctl` | `RiskLevelMedium` |
| `/opt/custom/tool` | `RiskLevelUnknown` |
| `/home/user/script` | `RiskLevelUnknown` |

#### 2.2.2 isStandardDirectory

```go
func isStandardDirectory(cmdPath string) bool {
    dir := filepath.Dir(cmdPath)

    for _, stdDir := range StandardDirectories {
        if dir == stdDir || strings.HasPrefix(cmdPath, stdDir+"/") {
            return true
        }
    }
    return false
}
```

**テストケース:**
| 入力パス | 期待される結果 |
|---------|---------------|
| `/bin/ls` | `true` |
| `/usr/bin/git` | `true` |
| `/opt/tool` | `false` |
| `/home/user/script` | `false` |

#### 2.2.3 shouldSkipHashValidation

```go
func shouldSkipHashValidation(cmdPath string, globalConfig *runnertypes.GlobalConfig) bool {
    if !globalConfig.SkipStandardPaths {
        return false
    }

    return isStandardDirectory(cmdPath)
}
```

**真理値表:**
| SkipStandardPaths | isStandardDirectory | 結果 |
|-------------------|-------------------|------|
| `false` | `true` | `false` (検証実行) |
| `false` | `false` | `false` (検証実行) |
| `true` | `true` | `true` (検証スキップ) |
| `true` | `false` | `false` (検証実行) |

#### 2.2.4 validateFileHash

```go
func validateFileHash(cmdPath string) error {
    // 1. ファイル存在確認
    if _, err := os.Stat(cmdPath); err != nil {
        return fmt.Errorf("%w: file not found: %s", ErrHashValidationFailed, cmdPath)
    }

    // 2. filevalidatorを使用したハッシュ検証
    validator, err := filevalidator.NewValidator()
    if err != nil {
        return fmt.Errorf("%w: failed to create validator: %v", ErrHashValidationFailed, err)
    }

    if err := validator.ValidateFile(cmdPath); err != nil {
        return fmt.Errorf("%w: %v", ErrHashValidationFailed, err)
    }

    return nil
}
```

#### 2.2.5 getCommandRiskOverride

```go
func getCommandRiskOverride(cmdPath string) (runnertypes.RiskLevel, bool) {
    risk, exists := CommandRiskOverrides[cmdPath]
    return risk, exists
}
```

**オーバーライドマップ:**
```go
var CommandRiskOverrides = map[string]runnertypes.RiskLevel{
    "/usr/bin/sudo":       runnertypes.RiskLevelCritical,  // 特権昇格
    "/bin/su":             runnertypes.RiskLevelCritical,  // 特権昇格
    "/usr/bin/curl":       runnertypes.RiskLevelMedium,    // ネットワーク
    "/usr/bin/wget":       runnertypes.RiskLevelMedium,    // ネットワーク
    "/usr/sbin/systemctl": runnertypes.RiskLevelHigh,      // システム制御
    "/usr/sbin/service":   runnertypes.RiskLevelHigh,      // システム制御
    "/bin/rm":             runnertypes.RiskLevelHigh,      // 破壊的操作
    "/usr/bin/dd":         runnertypes.RiskLevelHigh,      // 破壊的操作
}
```

### 2.3 PathResolver最大限簡素化仕様

#### 2.3.1 ValidateCommandメソッドの完全削除

**変更前:**
```go
func (pr *PathResolver) ValidateCommand(resolvedPath string) error {
    return pr.validateCommandSafety(resolvedPath)
}

func (pr *PathResolver) validateCommandSafety(command string) error {
    if pr.security == nil {
        return nil
    }
    return pr.security.ValidateCommand(command)  // 正規表現パス制限
}
```

**変更後:**
```go
// ValidateCommandメソッドを完全削除
// validateCommandSafetyメソッドも完全削除
// セキュリティ検証は AnalyzeCommandSecurity で実行される

// PathResolverは純粋なパス解決のみ実行
// - ResolvePath(command string) (string, error)
// - キャッシュ機能
```

#### 2.3.2 Manager.ResolveAndValidateCommandの調整

**変更前:**
```go
func (m *Manager) ResolveAndValidateCommand(command string) (string, error) {
    if m.pathResolver == nil {
        return "", ErrPathResolverNotInitialized
    }

    resolvedPath, err := m.pathResolver.ResolvePath(command)
    if err != nil {
        return "", err
    }

    // パス制限チェックを実行
    if err := m.pathResolver.ValidateCommand(resolvedPath); err != nil {
        return "", fmt.Errorf("unsafe command rejected: %w", err)
    }

    return resolvedPath, nil
}
```

**変更後:**
```go
func (m *Manager) ResolveAndValidateCommand(command string) (string, error) {
    if m.pathResolver == nil {
        return "", ErrPathResolverNotInitialized
    }

    // パス解決のみ実行
    resolvedPath, err := m.pathResolver.ResolvePath(command)
    if err != nil {
        return "", err
    }

    // ValidateCommand呼び出しを削除
    // セキュリティ検証は後続AnalyzeCommandSecurityで実行

    return resolvedPath, nil
}
```

#### 2.3.3 PathResolver簡素化のまとめ

**削除されるフィールド:**
```go
type PathResolver struct {
    pathEnv           string
    cache             map[string]string
    mu                sync.RWMutex
    // 以下を完全削除:
    // security          *security.Validator
    // skipStandardPaths bool
    // standardPaths     []string
}
```

**削除されるメソッド:**
- `func (pr *PathResolver) ValidateCommand(resolvedPath string) error`
- `func (pr *PathResolver) validateCommandSafety(command string) error`
- `func (pr *PathResolver) ShouldSkipVerification(path string) bool`

**簡素化されるコンストラクタ:**
```go
// 変更前
func NewPathResolver(pathEnv string, security *security.Validator, skipStandardPaths bool) *PathResolver

// 変更後
func NewPathResolver(pathEnv string) *PathResolver {
    return &PathResolver{
        pathEnv: pathEnv,
        cache:   make(map[string]string),
    }
}
```

## 3. データ構造仕様

### 3.1 設定構造体拡張

```go
// internal/runner/security/types.go
type Config struct {
    // 既存フィールド
    AllowedCommands              []string
    RequiredFilePermissions      os.FileMode
    RequiredDirectoryPermissions os.FileMode
    SensitiveEnvVars            []string
    MaxPathLength               int
    DangerousPrivilegedCommands []string
    ShellCommands               []string
    ShellMetacharacters         []string
    DangerousRootPatterns       []string
    DangerousRootArgPatterns    []string
    SystemCriticalPaths         []string
    LoggingOptions              LoggingOptions

    // 新規追加フィールド
    SkipStandardPaths           bool `toml:"skip_standard_paths"`
}
```

### 3.2 ハッシュ検証エラー構造体

```go
type HashValidationError struct {
    Command      string
    ExpectedHash string
    ActualHash   string
    Err          error
    Timestamp    time.Time
}

func (e *HashValidationError) Error() string {
    return fmt.Sprintf("hash_validation_failed: %s (expected: %s, actual: %s): %v",
        e.Command, e.ExpectedHash, e.ActualHash, e.Err)
}

func (e *HashValidationError) Unwrap() error {
    return e.Err
}
```

## 4. 設定仕様

### 4.1 TOML設定ファイル仕様

今回の開発では、既存のTOMLファイルに新たな設定項目は追加しません。ハッシュ検証スキップ制御は `GlobalConfig.SkipStandardPaths` フィールドを使用します。

```toml
[global]
timeout = 300
workdir = "/tmp"
log_level = "info"
verify_files = ["config.toml"]
skip_standard_paths = false  # ハッシュ検証制御: false=全ファイル検証, true=標準ディレクトリはスキップ
env_allowlist = ["PATH", "HOME"]
```

**注意**: 以下の項目はハードコーディングされており、設定ファイルでは変更できません：
- 標準ディレクトリ定義 (`StandardDirectories`)
- デフォルトリスクレベルマップ (`DefaultRiskLevels`)
- 個別コマンドオーバーライド (`CommandRiskOverrides`)
- セキュリティパターン（高リスク・中リスクパターン）
- ファイル権限要件
- ログ設定オプション

### 4.2 設定値検証仕様

```go
func (c *Config) Validate() error {
    // skip_standard_paths の検証
    // bool型なので特別な検証は不要

    // 既存の検証処理を継続...
    return nil
}
```

## 5. エラーハンドリング仕様

### 5.1 エラー分類

| エラータイプ | エラー定数 | 用途 |
|-------------|-----------|------|
| 入力検証エラー | `ErrInvalidPath` | パスが空文字列、相対パス |
| ハッシュ検証エラー | `ErrHashValidationFailed` | ハッシュ値不一致 |
| セキュリティ違反 | `ErrCommandNotAllowed` | リスクレベル超過 |
| システムエラー | 標準errorインターフェース | ファイルアクセス失敗など |

### 5.2 エラーメッセージ仕様

**ハッシュ検証失敗:**
```
hash_validation_failed: /home/user/custom-tool (expected: sha256:abc123..., actual: sha256:def456...): file content modified
```

**リスクレベル超過:**
```
command_risk_exceeded: /usr/bin/sudo (detected: CRITICAL, max_allowed: MEDIUM): explicit risk level override
```

**設定エラー:**
```
invalid_configuration: skip_standard_paths must be boolean, got string
```

## 6. パフォーマンス仕様

### 6.1 性能目標

| 項目 | 目標値 |
|------|-------|
| ディレクトリ判定処理時間 | < 1μs |
| ハッシュ検証スキップ判定時間 | < 0.5μs |
| 設定キャッシュヒット率 | > 95% |
| 全体的な性能劣化 | < 3% |

### 6.2 最適化実装

**設定キャッシュ:**
```go
var (
    configCache     *Config
    configCacheMux  sync.RWMutex
    configCacheTime time.Time
    configCacheTTL  = 5 * time.Minute
)

func getCurrentHashValidationConfig() *Config {
    configCacheMux.RLock()
    if configCache != nil && time.Since(configCacheTime) < configCacheTTL {
        defer configCacheMux.RUnlock()
        return configCache
    }
    configCacheMux.RUnlock()

    // キャッシュ更新...
}
```

## 7. テスト仕様

### 7.1 ユニットテスト仕様

#### 7.1.1 AnalyzeCommandSecurity テストケース

```go
func TestAnalyzeCommandSecurity_Integration(t *testing.T) {
    testCases := []struct {
        name            string
        resolvedPath    string
        args            []string
        skipStdPaths    bool
        hashValidResult error
        expectedRisk    runnertypes.RiskLevel
        expectedPattern string
        expectedReason  string
        expectedError   error
    }{
        {
            name:         "standard_dir_skip_hash_low_risk",
            resolvedPath: "/bin/ls",
            args:         []string{"-la"},
            skipStdPaths: true,
            expectedRisk: runnertypes.RiskLevelLow,
            expectedReason: "Default directory-based risk level",
        },
        {
            name:         "custom_dir_hash_fail",
            resolvedPath: "/opt/custom/tool",
            args:         []string{},
            skipStdPaths: false,
            hashValidResult: errors.New("hash mismatch"),
            expectedRisk: runnertypes.RiskLevelCritical,
            expectedPattern: "/opt/custom/tool",
            expectedReason: "Hash validation failed: hash mismatch",
        },
        {
            name:         "sudo_override_critical",
            resolvedPath: "/usr/bin/sudo",
            args:         []string{"id"},
            skipStdPaths: true,
            expectedRisk: runnertypes.RiskLevelCritical,
            expectedPattern: "/usr/bin/sudo",
            expectedReason: "Explicit risk level override",
        },
        {
            name:         "dangerous_pattern_high_risk",
            resolvedPath: "/bin/rm",
            args:         []string{"-rf", "/"},
            skipStdPaths: true,
            expectedRisk: runnertypes.RiskLevelHigh,
            expectedPattern: "rm -rf",
            expectedReason: "Recursive file removal",
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // テスト実装
        })
    }
}
```

### 7.2 統合テスト仕様

```go
func TestSecurityValidationUnification_E2E(t *testing.T) {
    // PathResolver → AnalyzeCommandSecurity の完全フロー
    // 様々な設定パターンでのテスト
    // エラーケースの網羅的テスト
}
```

## 8. 移行仕様

### 8.1 段階的移行計画

**Phase 1: AnalyzeCommandSecurity拡張**
- ディレクトリベースリスク判定機能追加
- ハッシュ検証統合機能追加
- 個別オーバーライド機能追加

**Phase 2: PathResolver簡素化**
- ValidateCommand の簡素化
- パス制限関連コードの削除

**Phase 3: 設定システム統合**
- skip_standard_paths 設定追加
- デフォルト設定の更新

### 8.2 検証手順

1. **機能テスト**: 新機能の動作確認
2. **互換性テスト**: 既存機能への影響確認
3. **性能テスト**: パフォーマンス劣化の確認
4. **セキュリティテスト**: セキュリティレベルの維持確認

この詳細仕様により、パス制限撤廃とハッシュ検証統合による統一セキュリティシステムを確実に実装できます。
