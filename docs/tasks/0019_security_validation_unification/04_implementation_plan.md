# 実装計画書: セキュリティ検証メカニズムの統一

## 1. 実装概要

### 1.1 実装目標
パス制限を撤廃し、ハッシュ検証とリスクベース評価による統合セキュリティシステムを段階的に実装する。

### 1.2 実装原則
- **段階的実装**: 機能追加 → 簡素化 → 統合の順序で実装
- **テスト駆動**: 各段階でテストを先行実装
- **下位互換性**: 既存APIインターフェースを維持

## 2. 実装フェーズ

### 2.1 Phase 1: セキュリティ分析エンジン拡張 (3-5日)

#### 2.1.1 新規ファイル作成

**ファイル1: `internal/runner/security/directory_risk.go`**
```go
// 標準ディレクトリとデフォルトリスクレベル定義
var StandardDirectories = []string{...}
var DefaultRiskLevels = map[string]runnertypes.RiskLevel{...}

// getDefaultRiskByDirectory - ディレクトリベースリスク判定
// isStandardDirectory - 標準ディレクトリ判定
```

**ファイル2: `internal/runner/security/hash_validation.go`**
```go
// shouldSkipHashValidation - ハッシュ検証スキップ判定
// validateFileHash - ファイルハッシュ検証実行
```

**ファイル3: `internal/runner/security/command_overrides.go`**
```go
// CommandRiskOverrides - 個別コマンドオーバーライド定義
// getCommandRiskOverride - オーバーライド取得
```

#### 2.1.2 既存ファイル拡張

**ファイル: `internal/runner/security/command_analysis.go`**
- `AnalyzeCommandSecurity` 関数の拡張
- ディレクトリベースリスク判定統合
- ハッシュ検証統合
- 個別オーバーライド適用

#### 2.1.3 テスト実装

**ファイル: `internal/runner/security/directory_risk_test.go`**
```go
func TestGetDefaultRiskByDirectory(t *testing.T)
func TestIsStandardDirectory(t *testing.T)
```

**ファイル: `internal/runner/security/hash_validation_test.go`**
```go
func TestShouldSkipHashValidation(t *testing.T)
func TestValidateFileHash(t *testing.T)
```

**ファイル: `internal/runner/security/command_analysis_test.go`**
```go
func TestAnalyzeCommandSecurity_Integration(t *testing.T)
```

### 2.2 Phase 2: PathResolver簡素化 (2-3日)

#### 2.2.1 PathResolver構造体の簡素化

**ファイル: `internal/verification/path_resolver.go`**
- `security` フィールド削除
- `ValidateCommand` メソッド削除
- `validateCommandSafety` メソッド削除
- コンストラクタの簡素化

#### 2.2.2 Manager調整

**ファイル: `internal/verification/manager.go`**
- `ResolveAndValidateCommand` の `ValidateCommand` 呼び出し削除
- セキュリティ検証をAnalyzeCommandSecurityに委譲

#### 2.2.3 テスト更新

**ファイル: `internal/verification/path_resolver_test.go`**
- ValidateCommand関連テストの削除
- 純粋なパス解決テストに集中

**ファイル: `internal/verification/manager_test.go`**
- セキュリティ検証削除に対応したテスト更新

### 2.3 Phase 3: 設定システム統合 (1-2日)

#### 2.3.1 設定構造体拡張

**ファイル: `internal/runner/runnertypes/config.go`**
- `GlobalConfig.SkipStandardPaths` フィールドの活用確認

#### 2.3.2 エラーハンドリング拡張

**ファイル: `internal/runner/security/errors.go`**
```go
var ErrHashValidationFailed = errors.New("hash validation failed")
type HashValidationError struct {...}
```

### 2.4 Phase 4: 統合テストとドキュメント (2-3日)

#### 2.4.1 統合テスト実装

**ファイル: `internal/runner/security/integration_test.go`**
```go
func TestSecurityValidationUnification_E2E(t *testing.T)
func TestPathResolverToAnalyzeCommandSecurity_Flow(t *testing.T)
```

#### 2.4.2 ベンチマークテスト

**ファイル: `internal/runner/security/benchmark_test.go`**
```go
func BenchmarkAnalyzeCommandSecurity_WithHashValidation(b *testing.B)
func BenchmarkAnalyzeCommandSecurity_SkipHashValidation(b *testing.B)
```

## 3. 実装詳細

### 3.1 Phase 1 詳細実装

#### 3.1.1 directory_risk.go 実装

```go
package security

import (
    "path/filepath"
    "strings"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Standard system directories with predefined risk levels
var StandardDirectories = []string{
    "/bin",
    "/usr/bin",
    "/usr/local/bin",
    "/sbin",
    "/usr/sbin",
    "/usr/local/sbin",
}

// Default risk levels for standard directories
var DefaultRiskLevels = map[string]runnertypes.RiskLevel{
    "/bin":             runnertypes.RiskLevelLow,
    "/usr/bin":         runnertypes.RiskLevelLow,
    "/usr/local/bin":   runnertypes.RiskLevelLow,
    "/sbin":            runnertypes.RiskLevelMedium,
    "/usr/sbin":        runnertypes.RiskLevelMedium,
    "/usr/local/sbin":  runnertypes.RiskLevelMedium,
}

// getDefaultRiskByDirectory returns the default risk level based on command path
func getDefaultRiskByDirectory(cmdPath string) runnertypes.RiskLevel {
    dir := filepath.Dir(cmdPath)

    // Exact match check
    if risk, exists := DefaultRiskLevels[dir]; exists {
        return risk
    }

    // Prefix match for subdirectories
    for stdDir, risk := range DefaultRiskLevels {
        if strings.HasPrefix(cmdPath, stdDir+"/") {
            return risk
        }
    }

    // Default: defer to individual pattern analysis
    return runnertypes.RiskLevelUnknown
}

// isStandardDirectory checks if the command path is in a standard directory
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

#### 3.1.2 hash_validation.go 実装

```go
package security

import (
    "fmt"
    "os"
    "github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// shouldSkipHashValidation determines whether to skip hash validation
func shouldSkipHashValidation(cmdPath string, globalConfig *runnertypes.GlobalConfig) bool {
    if !globalConfig.SkipStandardPaths {
        return false // Validate all files when SkipStandardPaths=false
    }

    return isStandardDirectory(cmdPath) // Skip only standard directories
}

// validateFileHash performs file hash validation
func validateFileHash(cmdPath string) error {
    // Check file existence
    if _, err := os.Stat(cmdPath); err != nil {
        return fmt.Errorf("%w: file not found: %s", ErrHashValidationFailed, cmdPath)
    }

    // Use existing filevalidator package
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

#### 3.1.3 command_overrides.go 実装

```go
package security

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Individual command risk level overrides
var CommandRiskOverrides = map[string]runnertypes.RiskLevel{
    "/usr/bin/sudo":       runnertypes.RiskLevelCritical,  // Privilege escalation
    "/bin/su":             runnertypes.RiskLevelCritical,  // Privilege escalation
    "/usr/bin/curl":       runnertypes.RiskLevelMedium,    // Network access
    "/usr/bin/wget":       runnertypes.RiskLevelMedium,    // Network access
    "/usr/sbin/systemctl": runnertypes.RiskLevelHigh,      // System control
    "/usr/sbin/service":   runnertypes.RiskLevelHigh,      // System control
    "/bin/rm":             runnertypes.RiskLevelHigh,      // Destructive operations
    "/usr/bin/dd":         runnertypes.RiskLevelHigh,      // Destructive operations
}

// getCommandRiskOverride retrieves the risk override for a specific command
func getCommandRiskOverride(cmdPath string) (runnertypes.RiskLevel, bool) {
    risk, exists := CommandRiskOverrides[cmdPath]
    return risk, exists
}
```

#### 3.1.4 AnalyzeCommandSecurity 拡張

```go
// Enhanced AnalyzeCommandSecurity with unified security validation
func AnalyzeCommandSecurity(resolvedPath string, args []string, globalConfig *runnertypes.GlobalConfig) (riskLevel runnertypes.RiskLevel, detectedPattern string, reason string, err error) {
    // Step 1: Input validation
    if resolvedPath == "" {
        return runnertypes.RiskLevelUnknown, "", "", fmt.Errorf("%w: empty command path", ErrInvalidPath)
    }

    if !filepath.IsAbs(resolvedPath) {
        return runnertypes.RiskLevelUnknown, "", "", fmt.Errorf("%w: path must be absolute, got relative path: %s", ErrInvalidPath, resolvedPath)
    }

    // Step 2: Symbolic link depth check
    if _, exceededDepth := extractAllCommandNames(resolvedPath); exceededDepth {
        return runnertypes.RiskLevelHigh, resolvedPath, "Symbolic link depth exceeds security limit (potential symlink attack)", nil
    }

    // Step 3: Directory-based default risk assessment
    defaultRisk := getDefaultRiskByDirectory(resolvedPath)

    // Step 4: Hash validation
    if shouldSkipHashValidation(resolvedPath, globalConfig) {
        // Log: hash validation skipped for standard directory
    } else {
        if err := validateFileHash(resolvedPath); err != nil {
            return runnertypes.RiskLevelCritical, resolvedPath,
                fmt.Sprintf("Hash validation failed: %v", err), nil
        }
    }

    // Step 5: High-risk pattern analysis
    if riskLevel, pattern, reason := checkCommandPatterns(resolvedPath, args, highRiskPatterns);
       riskLevel != runnertypes.RiskLevelUnknown {
        return riskLevel, pattern, reason, nil
    }

    // Step 6: setuid/setgid check
    hasSetuidOrSetgid, setuidErr := hasSetuidOrSetgidBit(resolvedPath)
    if setuidErr != nil {
        return runnertypes.RiskLevelHigh, resolvedPath,
            fmt.Sprintf("Unable to check setuid/setgid status: %v", setuidErr), nil
    }
    if hasSetuidOrSetgid {
        return runnertypes.RiskLevelHigh, resolvedPath,
            "Executable has setuid or setgid bit set", nil
    }

    // Step 7: Medium-risk pattern analysis
    if riskLevel, pattern, reason := checkCommandPatterns(resolvedPath, args, mediumRiskPatterns);
       riskLevel != runnertypes.RiskLevelUnknown {
        return riskLevel, pattern, reason, nil
    }

    // Step 8: Individual command override application
    if overrideRisk, found := getCommandRiskOverride(resolvedPath); found {
        return overrideRisk, resolvedPath, "Explicit risk level override", nil
    }

    // Step 9: Apply default risk level
    if defaultRisk != runnertypes.RiskLevelUnknown {
        return defaultRisk, "", "Default directory-based risk level", nil
    }

    // Fallback: use existing pattern analysis result
    return runnertypes.RiskLevelUnknown, "", "", nil
}
```

### 3.2 Phase 2 詳細実装

#### 3.2.1 PathResolver簡素化

```go
// Simplified PathResolver structure
type PathResolver struct {
    pathEnv string
    cache   map[string]string
    mu      sync.RWMutex
    // Removed security-related fields:
    // security          *security.Validator
    // skipStandardPaths bool
    // standardPaths     []string
}

// Simplified constructor
func NewPathResolver(pathEnv string) *PathResolver {
    return &PathResolver{
        pathEnv: pathEnv,
        cache:   make(map[string]string),
    }
}

// Removed methods:
// - ValidateCommand
// - validateCommandSafety
// - ShouldSkipVerification
```

#### 3.2.2 Manager調整

```go
// Updated ResolveAndValidateCommand method
func (m *Manager) ResolveAndValidateCommand(command string) (string, error) {
    if m.pathResolver == nil {
        return "", ErrPathResolverNotInitialized
    }

    // Only perform path resolution
    resolvedPath, err := m.pathResolver.ResolvePath(command)
    if err != nil {
        return "", err
    }

    // Removed ValidateCommand call
    // Security validation is now handled by AnalyzeCommandSecurity

    return resolvedPath, nil
}
```

## 4. テスト戦略

### 4.1 ユニットテスト計画

#### 4.1.1 新機能テスト
- `TestGetDefaultRiskByDirectory`: ディレクトリベースリスク判定
- `TestIsStandardDirectory`: 標準ディレクトリ判定
- `TestShouldSkipHashValidation`: ハッシュ検証スキップ判定
- `TestValidateFileHash`: ハッシュ検証実行
- `TestGetCommandRiskOverride`: 個別オーバーライド取得

#### 4.1.2 統合テスト
- `TestAnalyzeCommandSecurity_Integration`: 全機能統合テスト
- `TestSecurityValidationUnification_E2E`: エンドツーエンドテスト

#### 4.1.3 既存機能回帰テスト
- 全既存テストが通過することを確認
- セキュリティレベルの維持確認

### 4.2 パフォーマンステスト

#### 4.2.1 ベンチマーク
- ハッシュ検証ありvsスキップの性能比較
- ディレクトリ判定の処理時間測定
- 全体的な性能劣化の確認（目標: <3%）

## 5. 品質保証

### 5.1 コードレビュー観点

#### 5.1.1 セキュリティ観点
- ハッシュ検証スキップの安全性確認
- リスクレベル判定の妥当性確認
- 権限昇格防止の確認

#### 5.1.2 パフォーマンス観点
- キャッシュ効率の確認
- メモリ使用量の確認
- 処理時間の確認

#### 5.1.3 保守性観点
- コードの可読性確認
- テストカバレッジ確認
- ドキュメント整合性確認

### 5.2 受け入れテスト

#### 5.2.1 機能テスト
- [ ] 任意ディレクトリからのコマンド実行が可能
- [ ] skip_standard_paths=trueで標準ディレクトリのハッシュ検証がスキップされる
- [ ] skip_standard_paths=falseで全ファイルのハッシュ検証が実行される
- [ ] ディレクトリベースのデフォルトリスクレベルが正しく適用される
- [ ] ハッシュ検証失敗時にコマンドがブロックされる
- [ ] 個別コマンドのリスクレベル上書きが機能する

#### 5.2.2 互換性テスト
- [ ] 既存のAPIインターフェースが変更されていない
- [ ] 既存のテストケースが全て通過する
- [ ] 既存の設定ファイルが正常に動作する

#### 5.2.3 性能テスト
- [ ] 全体的な性能劣化が3%以内
- [ ] ハッシュ検証スキップによる性能向上を確認
- [ ] メモリ使用量に大きな変化がない

## 6. リスク管理

### 6.1 技術的リスク

#### 6.1.1 互換性リスク
**リスク**: 既存機能への予期しない影響
**対策**: 段階的実装、包括的テスト、回帰テスト

#### 6.1.2 性能リスク
**リスク**: 新機能による性能劣化
**対策**: ベンチマークテスト、プロファイリング、最適化

#### 6.1.3 セキュリティリスク
**リスク**: セキュリティレベルの低下
**対策**: セキュリティテスト、専門家レビュー

### 6.2 スケジュールリスク

#### 6.2.1 実装遅延リスク
**リスク**: 複雑な統合による実装遅延
**対策**: 段階的実装、マイルストーン管理

#### 6.2.2 テスト不足リスク
**リスク**: 十分なテストを行う時間不足
**対策**: テスト駆動開発、継続的テスト

## 7. 完了基準

### 7.1 開発完了基準
- [ ] 全新機能の実装完了
- [ ] PathResolver簡素化完了
- [ ] 設定システム統合完了
- [ ] 全テストケース通過
- [ ] ドキュメント更新完了

### 7.2 品質基準
- [ ] コードカバレッジ85%以上
- [ ] 静的解析エラー0件
- [ ] セキュリティスキャン合格
- [ ] 性能劣化3%以内
- [ ] メモリリーク検出なし

### 7.3 運用準備基準
- [ ] 運用ドキュメント整備
- [ ] 移行手順書作成
- [ ] トラブルシューティングガイド作成
- [ ] モニタリング設定完了

この実装計画により、パス制限撤廃とハッシュ検証統合による統一セキュリティシステムを確実に実装できます。
