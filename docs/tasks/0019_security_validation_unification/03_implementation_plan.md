# 実装計画書: セキュリティ検証メカニズムの統一

## 1. 実装概要

### 1.1 実装目標
- PathResolver.ValidateCommand とリスクレベル評価システムの統一
- リスクベース単一検証システムの構築
- ハードコーディングされたホワイトリストルールのリスクベース計算ルールへの変換
- セキュリティアーキテクチャの簡素化と保守性向上

### 1.2 実装範囲
- 統合リスクベース検証エンジンの実装
- ハードコーディングされたリスクレベル計算ルールの実装
- 包括的テストスイートの構築
- ドキュメント整備

###### Week 9: Final Release2. 実装フェーズ詳細

### 2.1 Phase 1: 基盤整備 (4週間)

#### Week 1: Core Components
**目標**: 統合リスクベース検証の基盤コンポーネント実装

**実装項目**:
```
既存パッケージの拡張:
internal/runner/security/
├── validator.go                # 既存Validatorにリスクベース機能追加
├── command_analysis.go         # 既存AnalyzeCommandSecurity関数を活用
└── types.go                   # 既存Config構造体の拡張

internal/verification/
└── path_resolver.go            # PathResolverへの統合機能追加

internal/runner/runnertypes/
└── config.go                   # 既存RiskLevel型を活用
```

**詳細タスク**:
- [ ] `Validator` 構造体にリスクベース検証メソッド追加
- [ ] 既存 `AnalyzeCommandSecurity` 関数との統合
- [ ] 既存 `Config` 構造体の拡張
- [ ] ハードコーディングリスク計算ロジックの実装
- [ ] 既存エラー型の活用と拡張
- [ ] 単体テストの作成

**成果物**:
- 既存Validatorに統合されたリスクベース検証機能
- 既存AnalyzeCommandSecurityと連携したリスク計算
- 既存設定システムの活用
- 基本テストカバレッジ 80%

#### Week 2: Path Resolver Integration
**目標**: PathResolver への統合検証システム組み込み

**実装項目**:
```go
// internal/verification/path_resolver.go の拡張
type PathResolver struct {
    // 既存フィールド...
    unifiedValidator security.UnifiedValidator
}

func (pr *PathResolver) ValidateCommand(resolvedPath string) error
func (pr *PathResolver) ValidateCommandWithArgs(resolvedPath string, args []string) error
```

**詳細タスク**:
- [ ] PathResolver 構造体の拡張
- [ ] 統合検証システムとの接続実装
- [ ] 既存メソッドの後方互換性保証
- [ ] 統合テストの作成

**成果物**:
- 統合された PathResolver
- 後方互換性の保証

#### Week 3: Risk Classification & Hardcoded Rules
**目標**: リスクレベル分類システムとハードコーディングルールの完成

**実装項目**:
```go
// リスクレベル分類の詳細実装
func (v *RiskBasedValidator) classifyCommandRisk(resolvedPath string, args []string) RiskLevel
func (v *RiskBasedValidator) validateWithRiskLevel(resolvedPath string, args []string) error

// ハードコーディングリスク計算
func (c *HardcodedRiskCalculator) CalculateDefaultRiskLevel(cmdPath string) RiskLevel {
    // /bin/*, /usr/bin/* → Low
    if strings.HasPrefix(cmdPath, "/bin/") || strings.HasPrefix(cmdPath, "/usr/bin/") {
        return RiskLevelLow
    }
    // /usr/sbin/*, /sbin/* → Medium
    if strings.HasPrefix(cmdPath, "/usr/sbin/") || strings.HasPrefix(cmdPath, "/sbin/") {
        return RiskLevelMedium
    }
    // /usr/local/bin/* → Low
    if strings.HasPrefix(cmdPath, "/usr/local/bin/") {
        return RiskLevelLow
    }
    return RiskLevelMiddle
}
```

**詳細タスク**:
- [ ] コマンドリスク分類ロジックの実装
- [ ] ハードコーディングされたパスベースリスク計算の実装
- [ ] 明示的リスクレベル指定ルールの実装
- [ ] エラーハンドリングの実装

**成果物**:
- 完全なリスク分類システム
- ハードコーディングリスク計算機能
- 包括的なテストカバレッジ

#### Week 4: Configuration System
**目標**: 統合設定システムと検証機能の完成

**実装項目**:
```go
// internal/security/config.go
type SecurityConfig struct {
    ValidationMode      string
    DefaultMaxRiskLevel string
    EnableCache         bool
    // ... 詳細設定
}

func (c *SecurityConfig) Validate() error
```

**詳細タスク**:
- [ ] 統合設定構造体の実装
- [ ] 設定検証ロジックの実装
- [ ] TOML パース機能の実装
- [ ] 設定エラーハンドリングの実装
- [ ] 設定関連テストの作成

**成果物**:
- 完全な統合設定システム
- 設定検証とエラーハンドリング
- 設定テストスイート

### 3.2 Phase 2: 統合・最適化 (3週間)

#### Week 4: Configuration System Integration
**目標**: 最小限の設定システムとハードコーディングルールの統合

**実装項目**:
```go
// internal/security/config.go
type SecurityConfig struct {
    MaxRiskLevel     string `toml:"max_risk_level"`
    EnableCache      bool   `toml:"enable_cache"`
    // ... キャッシュ関連設定のみ
}

func (c *SecurityConfig) Validate() error
func (c *SecurityConfig) LoadFromFile(path string) error
```

**詳細タスク**:
- [ ] 最小限の設定構造体の実装
- [ ] 設定検証ロジックの実装
- [ ] TOML パース機能の実装
- [ ] ハードコーディングルールとの統合
- [ ] 設定エラーハンドリングの実装
- [ ] 設定関連テストの作成

**成果物**:
- 最小限の設定システム
- ハードコーディングルール統合
- 設定検証とエラーハンドリング
- 設定テストスイート

#### Week 5: Comprehensive Testing
**目標**: 包括的テストスイートの構築

**実装項目**:
```
tests/
├── integration/
│   ├── risk_validator_test.go
│   ├── hardcoded_rules_test.go
│   └── performance_test.go
├── e2e/
│   ├── full_system_test.go
│   └── compatibility_test.go
└── security/
    ├── security_bypass_test.go
    └── risk_evaluation_test.go
```

**詳細タスク**:
- [ ] 統合テストスイート作成
- [ ] ハードコーディングルールテスト
- [ ] エンドツーエンドテスト実装
- [ ] セキュリティバイパステスト
- [ ] パフォーマンステスト実装

**成果物**:
- 包括的テストスイート
- テストカバレッジ 95% 以上
- CI/CD 統合テスト

#### Week 6: Comprehensive Testing
**目標**: エラーハンドリングとログシステムの完成

**実装項目**:
```go
// internal/security/logger.go
type SecurityLogger interface {
    LogValidationDecision(decision *ValidationDecision)
    LogConfigChange(change *ConfigChange)
    LogSecurityEvent(event *SecurityEvent)
}

// internal/security/audit.go
type AuditTrail interface {
    RecordEvent(event *SecurityEvent)
    QueryEvents(filter *EventFilter) ([]*SecurityEvent, error)
    GenerateReport(period TimePeriod) (*SecurityReport, error)
}
```

**詳細タスク**:
- [ ] 統一セキュリティログ機能
- [ ] 監査証跡システム
- [ ] エラーメッセージの改善
- [ ] ログレベル制御機能
- [ ] セキュリティレポート生成

**成果物**:
- 統一ログシステム
- 監査機能
- 改善されたエラーメッセージ

### 3.3 Phase 3: 最終調整・リリース準備 (3週間)

#### Week 6: Documentation & Examples
**目標**: 技術ドキュメントとサンプル設定の完成

**実装項目**:
```
docs/
├── risk_based_security_guide.md
├── hardcoded_rules_reference.md
├── migration_from_whitelist.md
└── configuration_examples.md

examples/
├── basic_risk_config.toml
├── advanced_risk_config.toml
└── compatibility_examples/
```

## 3. 詳細実装計画

### 3.1 Phase 1: 基盤構築 (3週間)

#### 3.1.1 Unified Validator
```go
// internal/runner/security/validator.go の拡張
package security

import (
    "context"
    "fmt"
    "time"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// 既存Validator構造体に機能追加
type Validator struct {
    // 既存フィールド...
    config                      *Config
    fs                          common.FileSystem
    allowedCommandRegexps       []*regexp.Regexp
    // 新規追加: リスクベース検証サポート
    riskBasedValidation         bool
    defaultMaxRiskLevel         runnertypes.RiskLevel
}

// 既存NewValidator関数を拡張（新規関数は作成しない）
// func NewValidator(config *Config) (*Validator, error) {
//     // 既存の実装にリスクベース機能を追加
// }

// 新規メソッド: リスクベース検証
func (v *Validator) ValidateCommandWithRisk(ctx context.Context, resolvedPath string, args []string, maxRiskLevel runnertypes.RiskLevel) error {
    // 既存のAnalyzeCommandSecurity関数を活用
    riskLevel, pattern, reason, err := AnalyzeCommandSecurity(resolvedPath, args)
    if err != nil {
        return fmt.Errorf("risk analysis failed: %w", err)
    }

    if riskLevel > maxRiskLevel {
        return fmt.Errorf("command risk level %s exceeds maximum allowed %s", riskLevel.String(), maxRiskLevel.String())
    }

    return nil
}

// 既存ValidateCommandメソッドの拡張 (新規メソッドは作成せず、既存メソッドを拡張)
// func (v *Validator) ValidateCommand(command string) error {
//     if v.riskBasedValidation {
//         return v.ValidateCommandWithRisk(context.Background(), command, []string{}, v.defaultMaxRiskLevel)
//     }
//
//     // 既存のホワイトリスト検証ロジック
//     for _, re := range v.allowedCommandRegexps {
//         if re.MatchString(command) {
//             return nil
//         }
//     }
//
//     return fmt.Errorf("%w: command %s does not match any allowed pattern", ErrCommandNotAllowed, command)
// }
```

#### 3.1.2 既存Config構造体の活用
```go
// internal/runner/security/types.go の拡張
package security

import (
    "fmt"
    "strings"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// 既存Config構造体にフィールド追加
type Config struct {
    // 既存フィールド...
    AllowedCommands []string
    RequiredFilePermissions os.FileMode
    // 新規追加
    UseRiskBasedValidation bool                   `toml:"use_risk_based_validation"`
    DefaultMaxRiskLevel    string                `toml:"default_max_risk_level"`
    riskLevelCache         runnertypes.RiskLevel // パース済み値のキャッシュ
}

// 既存DefaultConfig関数の拡張
func DefaultConfig() *Config {
    return &Config{
        // 既存設定...
        AllowedCommands: []string{
            "^/bin/.*",
            "^/usr/bin/.*",
            "^/usr/sbin/.*",
            "^/usr/local/bin/.*",
        },
        // 新規追加
        UseRiskBasedValidation: false,                          // デフォルトは既存のホワイトリストモード
        DefaultMaxRiskLevel:    runnertypes.MediumRiskLevelString, // "medium"
        riskLevelCache:         runnertypes.RiskLevelMedium,
    }
}

// 新規メソッド: リスクレベルのパースとキャッシュ
func (c *Config) GetDefaultMaxRiskLevel() (runnertypes.RiskLevel, error) {
    if c.riskLevelCache != runnertypes.RiskLevelUnknown {
        return c.riskLevelCache, nil
    }

    parsed, err := runnertypes.ParseRiskLevel(c.DefaultMaxRiskLevel)
    if err != nil {
        return runnertypes.RiskLevelUnknown, err
    }

    c.riskLevelCache = parsed
    return parsed, nil
}

// 新規メソッド: 設定の検証
func (c *Config) ValidateRiskBasedConfig() error {
    if !c.UseRiskBasedValidation {
        return nil // リスクベースではない場合は検証しない
    }

    _, err := runnertypes.ParseRiskLevel(c.DefaultMaxRiskLevel)
    if err != nil {
        return fmt.Errorf("invalid default_max_risk_level: %w", err)
    }

    return nil
}
```

### 3.2 Configuration Manager
```

### 3.3 Testing Implementation

#### 3.3.1 Integration Tests
```go
// tests/integration/unified_validator_test.go
package integration

import (
    "context"
    "testing"

    "github.com/isseis/go-safe-cmd-runner/internal/security"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestUnifiedValidator_EndToEnd(t *testing.T) {
    tests := []struct {
        name           string
        config         *security.SecurityConfig
        command        string
        args           []string
        expectAllowed  bool
        expectedError  string
    }{
        {
            name: "risk_based_mode_allows_safe_command",
            config: &security.SecurityConfig{
                ValidationMode:      "risk_based",
                DefaultMaxRiskLevel: "medium",
            },
            command:       "/usr/bin/echo",
            args:          []string{"hello"},
            expectAllowed: true,
        },
        {
            name: "risk_based_mode_blocks_dangerous_command",
            config: &security.SecurityConfig{
                ValidationMode:      "risk_based",
                DefaultMaxRiskLevel: "low",
            },
            command:       "/usr/bin/rm",
            args:          []string{"-rf", "/"},
            expectAllowed: false,
            expectedError: "command_verification_failed",
        },
        {
            name: "whitelist_mode_compatibility",
            config: &security.SecurityConfig{
                ValidationMode: "whitelist",
                AllowedCommands: []string{
                    "^/usr/bin/echo$",
                },
            },
            command:       "/usr/bin/echo",
            args:          []string{"test"},
            expectAllowed: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            logger := &MockSecurityLogger{}
            configMgr, err := security.NewSecurityConfigManager(tt.config, logger)
            require.NoError(t, err)

            validator, err := security.NewUnifiedValidator(configMgr, logger)
            require.NoError(t, err)

            ctx := context.Background()
            err = validator.ValidateCommandWithArgs(ctx, tt.command, tt.args)

            if tt.expectAllowed {
                assert.NoError(t, err, "Command should be allowed")
            } else {
                assert.Error(t, err, "Command should be blocked")
                if tt.expectedError != "" {
                    assert.Contains(t, err.Error(), tt.expectedError)
                }
            }
        })
    }
}
```

## 4. リスク管理

### 4.1 実装リスク

| リスク | 影響度 | 発生確率 | 対策 |
|--------|--------|----------|------|
| 性能劣化 | 中 | 低 | ベンチマーク継続実施、キャッシュ最適化 |
| 既存システム互換性問題 | 高 | 中 | 包括的互換性テスト、詳細テストスイート |
| 設定の複雑性 | 中 | 高 | 直感的な設定システム、詳細な手順書 |
| セキュリティホール | 高 | 低 | セキュリティレビュー、脆弱性テスト |

### 4.2 品質保証

- **コードレビュー**: 全コードの peer review 実施
- **セキュリティレビュー**: セキュリティ専門家による review
- **自動テスト**: CI/CD パイプラインでの自動テスト実行
- **ベンチマークテスト**: 性能劣化の継続監視

## 5. 成功指標

### 5.1 技術指標
- [ ] テストカバレッジ 95% 以上
- [ ] 性能劣化 5% 以内
- [ ] 既存テスト 100% 通過
- [ ] ゼロセキュリティインシデント

### 5.2 運用指標
- [ ] 導入成功率 95% 以上
- [ ] 設定エラー 50% 削減
- [ ] サポート問い合わせ 30% 削減
- [ ] ドキュメント満足度 90% 以上

### 5.3 保守性指標
- [ ] セキュリティロジック一元化 100%
- [ ] 新機能追加工数 50% 削減
- [ ] バグ修正工数 40% 削減

この実装計画に従って、統一セキュリティシステムを構築し、go-safe-cmd-runner のセキュリティアーキテクチャを大幅に改善します。
