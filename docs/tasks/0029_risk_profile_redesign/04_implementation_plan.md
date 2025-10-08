# 実装計画書: コマンドリスクプロファイルのリファクタリング

## 1. 概要

本計画書は、コマンドリスクプロファイルシステムのリファクタリングを実現するための実装計画を定義する。TDD（テスト駆動開発）アプローチに従い、段階的に実装を進める。

## 2. 実装方針

### 2.1 開発アプローチ

- **TDD (Test-Driven Development)**: まずテストを作成し、その後実装を進める
- **段階的移行**: 既存コードへの影響を最小化するため、フェーズを分けて実装
- **継続的検証**: 各フェーズで全テストが通過することを確認

### 2.2 品質基準

各フェーズ完了時に以下を満たすこと：
- [ ] 全ユニットテストがパス
- [ ] 全統合テストがパス
- [ ] `make lint`でエラーなし
- [ ] `make fmt`でフォーマット済み
- [ ] コードカバレッジ: 新規コードの80%以上

## 3. フェーズ1: 基盤実装

### 3.1 Phase 1.1: RiskFactor型の実装

#### タスク1.1.1: RiskFactor構造体の定義

**ファイル:** `internal/runner/security/risk_factor.go`

**実装内容:**
```go
package security

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// RiskFactor represents an individual risk factor with its level and explanation
type RiskFactor struct {
    Level  runnertypes.RiskLevel // Risk level for this specific factor
    Reason string                // Human-readable explanation of this risk
}
```

**チェックリスト:**
- [ ] ファイル作成
- [ ] 構造体定義
- [ ] GoDocコメント追加
- [ ] `make fmt`実行

#### タスク1.1.2: RiskFactorのテスト作成

**ファイル:** `internal/runner/security/risk_factor_test.go`

**テストケース:**
```go
func TestRiskFactor(t *testing.T) {
    tests := []struct {
        name   string
        risk   RiskFactor
        wantLevel runnertypes.RiskLevel
        wantReason string
    }{
        {
            name: "Unknown risk with empty reason",
            risk: RiskFactor{Level: runnertypes.RiskLevelUnknown},
            wantLevel: runnertypes.RiskLevelUnknown,
            wantReason: "",
        },
        {
            name: "Low risk with reason",
            risk: RiskFactor{Level: runnertypes.RiskLevelLow, Reason: "Low impact operation"},
            wantLevel: runnertypes.RiskLevelLow,
            wantReason: "Low impact operation",
        },
        {
            name: "Medium risk",
            risk: RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network operation"},
            wantLevel: runnertypes.RiskLevelMedium,
            wantReason: "Network operation",
        },
        {
            name: "High risk",
            risk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
            wantLevel: runnertypes.RiskLevelHigh,
            wantReason: "Data exfiltration",
        },
        {
            name: "Critical risk",
            risk: RiskFactor{Level: runnertypes.RiskLevelCritical, Reason: "Privilege escalation"},
            wantLevel: runnertypes.RiskLevelCritical,
            wantReason: "Privilege escalation",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.wantLevel, tt.risk.Level)
            assert.Equal(t, tt.wantReason, tt.risk.Reason)
        })
    }
}
```

**チェックリスト:**
- [ ] テストファイル作成
- [ ] テストケース実装
- [ ] `make test`実行（失敗を確認）
- [ ] RiskFactor実装後に`make test`実行（成功を確認）

### 3.2 Phase 1.2: CommandRiskProfile構造体の実装

#### タスク1.2.1: 新しいCommandRiskProfile構造体の定義

**ファイル:** `internal/runner/security/command_risk_profile.go`

**実装内容:**
```go
// CommandRiskProfile defines comprehensive risk information for a command
type CommandRiskProfile struct {
    // Individual risk factors (explicit separation)
    PrivilegeRisk   RiskFactor // Risk from privilege escalation (sudo, su, doas)
    NetworkRisk     RiskFactor // Risk from network operations
    DestructionRisk RiskFactor // Risk from destructive operations (rm, dd, format)
    DataExfilRisk   RiskFactor // Risk from data exfiltration to external services
    SystemModRisk   RiskFactor // Risk from system modifications (systemctl, service)

    // Network behavior configuration
    NetworkType        NetworkOperationType // How network operations are determined
    NetworkSubcommands []string              // Subcommands that trigger network operations

    // Derived properties
    IsPrivilege bool // True if PrivilegeRisk.Level >= High
}
```

**チェックリスト:**
- [ ] ファイル作成（または既存ファイルに追加）
- [ ] 構造体定義
- [ ] GoDocコメント追加
- [ ] `make fmt`実行

#### タスク1.2.2: BaseRiskLevel()メソッドのテスト作成

**ファイル:** `internal/runner/security/command_risk_profile_test.go`

**テストケース:**
```go
func TestCommandRiskProfile_BaseRiskLevel(t *testing.T) {
    tests := []struct {
        name    string
        profile CommandRiskProfile
        want    runnertypes.RiskLevel
    }{
        {
            name: "all unknown",
            profile: CommandRiskProfile{
                PrivilegeRisk:   RiskFactor{Level: runnertypes.RiskLevelUnknown},
                NetworkRisk:     RiskFactor{Level: runnertypes.RiskLevelUnknown},
                DestructionRisk: RiskFactor{Level: runnertypes.RiskLevelUnknown},
                DataExfilRisk:   RiskFactor{Level: runnertypes.RiskLevelUnknown},
                SystemModRisk:   RiskFactor{Level: runnertypes.RiskLevelUnknown},
            },
            want: runnertypes.RiskLevelUnknown,
        },
        {
            name: "single medium risk",
            profile: CommandRiskProfile{
                NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelMedium},
            },
            want: runnertypes.RiskLevelMedium,
        },
        {
            name: "multiple risks - max is high",
            profile: CommandRiskProfile{
                NetworkRisk:   RiskFactor{Level: runnertypes.RiskLevelMedium},
                DataExfilRisk: RiskFactor{Level: runnertypes.RiskLevelHigh},
            },
            want: runnertypes.RiskLevelHigh,
        },
        {
            name: "critical privilege risk",
            profile: CommandRiskProfile{
                PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelCritical},
                NetworkRisk:   RiskFactor{Level: runnertypes.RiskLevelMedium},
            },
            want: runnertypes.RiskLevelCritical,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.want, tt.profile.BaseRiskLevel())
        })
    }
}
```

**チェックリスト:**
- [ ] テストケース実装
- [ ] `make test`実行（失敗を確認）
- [ ] BaseRiskLevel()実装後に`make test`実行（成功を確認）

#### タスク1.2.3: BaseRiskLevel()メソッドの実装

**ファイル:** `internal/runner/security/command_risk_profile.go`

**実装内容:**
```go
// BaseRiskLevel computes the overall risk level as the maximum of all risk factors
func (p CommandRiskProfile) BaseRiskLevel() runnertypes.RiskLevel {
    return max(
        p.PrivilegeRisk.Level,
        p.NetworkRisk.Level,
        p.DestructionRisk.Level,
        p.DataExfilRisk.Level,
        p.SystemModRisk.Level,
    )
}
```

**チェックリスト:**
- [ ] メソッド実装
- [ ] GoDocコメント追加
- [ ] `make test`実行（成功を確認）
- [ ] `make fmt`実行

#### タスク1.2.4: GetRiskReasons()メソッドのテスト作成

**ファイル:** `internal/runner/security/command_risk_profile_test.go`

**テストケース:**
```go
func TestCommandRiskProfile_GetRiskReasons(t *testing.T) {
    tests := []struct {
        name    string
        profile CommandRiskProfile
        want    []string
    }{
        {
            name: "no risks",
            profile: CommandRiskProfile{
                PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelUnknown},
            },
            want: []string{},
        },
        {
            name: "single risk",
            profile: CommandRiskProfile{
                NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network access"},
            },
            want: []string{"Network access"},
        },
        {
            name: "multiple risks",
            profile: CommandRiskProfile{
                NetworkRisk:   RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network access"},
                DataExfilRisk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
            },
            want: []string{"Network access", "Data exfiltration"},
        },
        {
            name: "all risk types",
            profile: CommandRiskProfile{
                PrivilegeRisk:   RiskFactor{Level: runnertypes.RiskLevelCritical, Reason: "Privilege escalation"},
                NetworkRisk:     RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network access"},
                DestructionRisk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "File deletion"},
                DataExfilRisk:   RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
                SystemModRisk:   RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "System modification"},
            },
            want: []string{
                "Privilege escalation",
                "Network access",
                "File deletion",
                "Data exfiltration",
                "System modification",
            },
        },
        {
            name: "empty reason is excluded",
            profile: CommandRiskProfile{
                NetworkRisk:   RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: ""},
                DataExfilRisk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
            },
            want: []string{"Data exfiltration"},
        },
        {
            name: "mixed empty and non-empty reasons",
            profile: CommandRiskProfile{
                PrivilegeRisk:   RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: ""},
                NetworkRisk:     RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network access"},
                DestructionRisk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: ""},
                DataExfilRisk:   RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
            },
            want: []string{"Network access", "Data exfiltration"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := tt.profile.GetRiskReasons()
            assert.Equal(t, tt.want, got)
        })
    }
}
```

**チェックリスト:**
- [ ] テストケース実装
- [ ] `make test`実行（失敗を確認）
- [ ] GetRiskReasons()実装後に`make test`実行（成功を確認）

#### タスク1.2.5: GetRiskReasons()メソッドの実装

**ファイル:** `internal/runner/security/command_risk_profile.go`

**実装内容:**
```go
// GetRiskReasons returns all non-empty reasons contributing to the risk level
func (p CommandRiskProfile) GetRiskReasons() []string {
    var reasons []string

    // Helper function to add non-empty reasons
    addReason := func(risk RiskFactor) {
        if risk.Level > runnertypes.RiskLevelUnknown && risk.Reason != "" {
            reasons = append(reasons, risk.Reason)
        }
    }

    // Collect all risk factors in order
    addReason(p.PrivilegeRisk)
    addReason(p.NetworkRisk)
    addReason(p.DestructionRisk)
    addReason(p.DataExfilRisk)
    addReason(p.SystemModRisk)

    return reasons
}
```

**実装上の改善点:**
- ヘルパー関数（クロージャ）により重複コードを削減
- 空文字列の理由を除外し、有意義な情報のみを返す
- 将来的にリスク要因が増えた場合も、`addReason()`呼び出しを追加するだけで対応可能

**チェックリスト:**
- [ ] メソッド実装
- [ ] GoDocコメント追加
- [ ] `make test`実行（成功を確認）
- [ ] `make fmt`実行

### 3.3 Phase 1.3: バリデーション機能の実装

#### タスク1.3.1: エラー定義

**ファイル:** `internal/runner/security/errors.go`

**実装内容:**
```go
package security

import "errors"

var (
    // ErrNetworkAlwaysRequiresMediumRisk is returned when NetworkTypeAlways has NetworkRisk < Medium
    ErrNetworkAlwaysRequiresMediumRisk = errors.New("NetworkTypeAlways commands must have NetworkRisk >= Medium")

    // ErrPrivilegeRequiresHighRisk is returned when IsPrivilege is true but PrivilegeRisk < High
    ErrPrivilegeRequiresHighRisk = errors.New("privilege escalation commands must have PrivilegeRisk >= High")

    // ErrNetworkSubcommandsOnlyForConditional is returned when NetworkSubcommands is set for non-conditional network type
    ErrNetworkSubcommandsOnlyForConditional = errors.New("NetworkSubcommands should only be set for NetworkTypeConditional")
)
```

**設計上のポイント:**
- 各バリデーションルールに対応する具体的なエラー型を定義
- `errors.Is()`による型判別を可能にし、将来的な動的バリデーションでのエラーハンドリングを容易化

**チェックリスト:**
- [ ] ファイル作成（または既存ファイルに追加）
- [ ] 3つのエラー定義
- [ ] GoDocコメント追加
- [ ] `make fmt`実行

#### タスク1.3.2: Validate()メソッドのテスト作成

**ファイル:** `internal/runner/security/command_risk_profile_test.go`

**テストケース:**
```go
func TestCommandRiskProfile_Validate(t *testing.T) {
    tests := []struct {
        name    string
        profile CommandRiskProfile
        wantErr error
    }{
        {
            name: "valid profile - all unknown",
            profile: CommandRiskProfile{
                NetworkType: NetworkTypeNone,
            },
            wantErr: nil,
        },
        {
            name: "valid profile - privilege escalation",
            profile: CommandRiskProfile{
                PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelCritical},
                IsPrivilege:   true,
                NetworkType:   NetworkTypeNone,
            },
            wantErr: nil,
        },
        {
            name: "valid profile - always network",
            profile: CommandRiskProfile{
                NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelMedium},
                NetworkType: NetworkTypeAlways,
            },
            wantErr: nil,
        },
        {
            name: "valid profile - conditional network",
            profile: CommandRiskProfile{
                NetworkRisk:        RiskFactor{Level: runnertypes.RiskLevelMedium},
                NetworkType:        NetworkTypeConditional,
                NetworkSubcommands: []string{"clone", "fetch"},
            },
            wantErr: nil,
        },
        {
            name: "invalid - NetworkTypeAlways with low NetworkRisk",
            profile: CommandRiskProfile{
                NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelLow},
                NetworkType: NetworkTypeAlways,
            },
            wantErr: ErrNetworkAlwaysRequiresMediumRisk,
        },
        {
            name: "invalid - IsPrivilege with medium PrivilegeRisk",
            profile: CommandRiskProfile{
                PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelMedium},
                IsPrivilege:   true,
            },
            wantErr: ErrPrivilegeRequiresHighRisk,
        },
        {
            name: "invalid - NetworkSubcommands without Conditional",
            profile: CommandRiskProfile{
                NetworkType:        NetworkTypeNone,
                NetworkSubcommands: []string{"clone"},
            },
            wantErr: ErrNetworkSubcommandsOnlyForConditional,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.profile.Validate()
            if tt.wantErr != nil {
                assert.Error(t, err)
                assert.ErrorIs(t, err, tt.wantErr)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

**チェックリスト:**
- [ ] テストケース実装
- [ ] `make test`実行（失敗を確認）
- [ ] Validate()実装後に`make test`実行（成功を確認）

#### タスク1.3.3: Validate()メソッドの実装

**ファイル:** `internal/runner/security/command_risk_profile.go`

**実装内容:**
```go
// Validate ensures consistency between risk factors and configuration
func (p CommandRiskProfile) Validate() error {
    // Rule 1: NetworkTypeAlways implies NetworkRisk >= Medium
    if p.NetworkType == NetworkTypeAlways && p.NetworkRisk.Level < runnertypes.RiskLevelMedium {
        return fmt.Errorf("%w (got %v)", ErrNetworkAlwaysRequiresMediumRisk, p.NetworkRisk.Level)
    }

    // Rule 2: IsPrivilege implies PrivilegeRisk >= High
    if p.IsPrivilege && p.PrivilegeRisk.Level < runnertypes.RiskLevelHigh {
        return fmt.Errorf("%w (got %v)", ErrPrivilegeRequiresHighRisk, p.PrivilegeRisk.Level)
    }

    // Rule 3: NetworkSubcommands only for NetworkTypeConditional
    if len(p.NetworkSubcommands) > 0 && p.NetworkType != NetworkTypeConditional {
        return ErrNetworkSubcommandsOnlyForConditional
    }

    return nil
}
```

**設計上のポイント:**
- 各バリデーションルールで具体的なエラー型を返すことで、`errors.Is()`による型判別が可能
- Rule 1, 2では実際のリスクレベル値を`fmt.Errorf()`でラップして詳細情報を提供
- Rule 3は条件のみのチェックなので、エラー型をそのまま返却

**チェックリスト:**
- [ ] メソッド実装
- [ ] GoDocコメント追加
- [ ] `make test`実行（成功を確認）
- [ ] `make fmt`実行

### 3.4 Phase 1.4: ProfileBuilderの実装

#### タスク1.4.1: ProfileBuilder構造体の定義

**ファイル:** `internal/runner/security/profile_builder.go`

**実装内容:**
```go
package security

import (
    "fmt"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ProfileBuilder provides a fluent API for building CommandRiskProfile
type ProfileBuilder struct {
    commands           []string
    privilegeRisk      *RiskFactor
    networkRisk        *RiskFactor
    destructionRisk    *RiskFactor
    dataExfilRisk      *RiskFactor
    systemModRisk      *RiskFactor
    networkType        NetworkOperationType
    networkSubcommands []string
}

// NewProfile creates a new profile builder for the given commands
func NewProfile(commands ...string) *ProfileBuilder {
    return &ProfileBuilder{
        commands:    commands,
        networkType: NetworkTypeNone,
    }
}
```

**チェックリスト:**
- [ ] ファイル作成
- [ ] 構造体定義
- [ ] NewProfile()実装
- [ ] GoDocコメント追加
- [ ] `make fmt`実行

#### タスク1.4.2: リスク設定メソッドの実装

**ファイル:** `internal/runner/security/profile_builder.go`

**実装内容:**
```go
// PrivilegeRisk sets the privilege escalation risk factor
func (b *ProfileBuilder) PrivilegeRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
    b.privilegeRisk = &RiskFactor{Level: level, Reason: reason}
    return b
}

// NetworkRisk sets the network operation risk factor
func (b *ProfileBuilder) NetworkRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
    b.networkRisk = &RiskFactor{Level: level, Reason: reason}
    return b
}

// DestructionRisk sets the destructive operation risk factor
func (b *ProfileBuilder) DestructionRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
    b.destructionRisk = &RiskFactor{Level: level, Reason: reason}
    return b
}

// DataExfilRisk sets the data exfiltration risk factor
func (b *ProfileBuilder) DataExfilRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
    b.dataExfilRisk = &RiskFactor{Level: level, Reason: reason}
    return b
}

// SystemModRisk sets the system modification risk factor
func (b *ProfileBuilder) SystemModRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
    b.systemModRisk = &RiskFactor{Level: level, Reason: reason}
    return b
}
```

**チェックリスト:**
- [ ] 各メソッド実装
- [ ] GoDocコメント追加
- [ ] `make fmt`実行

#### タスク1.4.3: ネットワーク設定メソッドの実装

**ファイル:** `internal/runner/security/profile_builder.go`

**実装内容:**
```go
// AlwaysNetwork marks the command as always performing network operations
func (b *ProfileBuilder) AlwaysNetwork() *ProfileBuilder {
    b.networkType = NetworkTypeAlways
    return b
}

// ConditionalNetwork marks the command as conditionally performing network operations
func (b *ProfileBuilder) ConditionalNetwork(subcommands ...string) *ProfileBuilder {
    b.networkType = NetworkTypeConditional
    b.networkSubcommands = subcommands
    return b
}
```

**チェックリスト:**
- [ ] 各メソッド実装
- [ ] GoDocコメント追加
- [ ] `make fmt`実装

#### タスク1.4.4: CommandProfileDef構造体の定義

**ファイル:** `internal/runner/security/command_profile_def.go`

**実装内容:**
```go
package security

// CommandProfileDef associates a list of commands with their risk profile
type CommandProfileDef struct {
    commands []string
    profile  CommandRiskProfile
}

// Commands returns the list of commands for this profile
func (d CommandProfileDef) Commands() []string {
    return d.commands
}

// Profile returns the risk profile
func (d CommandProfileDef) Profile() CommandRiskProfile {
    return d.profile
}
```

**チェックリスト:**
- [ ] ファイル作成
- [ ] 構造体定義
- [ ] アクセサメソッド実装
- [ ] GoDocコメント追加
- [ ] `make fmt`実行

#### タスク1.4.5: Build()メソッドのテスト作成

**ファイル:** `internal/runner/security/profile_builder_test.go`

**テストケース:**
```go
func TestProfileBuilder_Build(t *testing.T) {
    t.Run("valid privilege escalation profile", func(t *testing.T) {
        def := NewProfile("sudo").
            PrivilegeRisk(runnertypes.RiskLevelCritical, "Privilege escalation").
            Build()

        assert.Equal(t, []string{"sudo"}, def.Commands())
        assert.Equal(t, runnertypes.RiskLevelCritical, def.Profile().BaseRiskLevel())
        assert.True(t, def.Profile().IsPrivilege)
    })

    t.Run("valid network profile", func(t *testing.T) {
        def := NewProfile("curl", "wget").
            NetworkRisk(runnertypes.RiskLevelMedium, "Network operations").
            AlwaysNetwork().
            Build()

        assert.Equal(t, []string{"curl", "wget"}, def.Commands())
        assert.Equal(t, runnertypes.RiskLevelMedium, def.Profile().BaseRiskLevel())
        assert.Equal(t, NetworkTypeAlways, def.Profile().NetworkType)
    })

    t.Run("valid conditional network profile", func(t *testing.T) {
        def := NewProfile("git").
            NetworkRisk(runnertypes.RiskLevelMedium, "Network operations").
            ConditionalNetwork("clone", "fetch", "pull", "push").
            Build()

        assert.Equal(t, []string{"git"}, def.Commands())
        assert.Equal(t, NetworkTypeConditional, def.Profile().NetworkType)
        assert.Equal(t, []string{"clone", "fetch", "pull", "push"}, def.Profile().NetworkSubcommands)
    })

    t.Run("multiple risk factors", func(t *testing.T) {
        def := NewProfile("claude").
            NetworkRisk(runnertypes.RiskLevelHigh, "AI API communication").
            DataExfilRisk(runnertypes.RiskLevelHigh, "Data exfiltration").
            AlwaysNetwork().
            Build()

        assert.Equal(t, runnertypes.RiskLevelHigh, def.Profile().BaseRiskLevel())
        reasons := def.Profile().GetRiskReasons()
        assert.Contains(t, reasons, "AI API communication")
        assert.Contains(t, reasons, "Data exfiltration")
    })

    t.Run("invalid - NetworkTypeAlways with low risk should panic", func(t *testing.T) {
        assert.Panics(t, func() {
            NewProfile("test").
                NetworkRisk(runnertypes.RiskLevelLow, "test").
                AlwaysNetwork().
                Build()
        })
    })

    t.Run("default values for unset risks", func(t *testing.T) {
        def := NewProfile("test").Build()

        profile := def.Profile()
        assert.Equal(t, runnertypes.RiskLevelUnknown, profile.PrivilegeRisk.Level)
        assert.Equal(t, runnertypes.RiskLevelUnknown, profile.NetworkRisk.Level)
        assert.Equal(t, runnertypes.RiskLevelUnknown, profile.DestructionRisk.Level)
        assert.Equal(t, runnertypes.RiskLevelUnknown, profile.DataExfilRisk.Level)
        assert.Equal(t, runnertypes.RiskLevelUnknown, profile.SystemModRisk.Level)
    })
}
```

**チェックリスト:**
- [ ] テストケース実装
- [ ] `make test`実行（失敗を確認）
- [ ] Build()実装後に`make test`実行（成功を確認）

#### タスク1.4.6: Build()メソッドの実装

**ファイル:** `internal/runner/security/profile_builder.go`

**実装内容:**
```go
// Build creates the final CommandProfileDef with validation
func (b *ProfileBuilder) Build() CommandProfileDef {
    profile := CommandRiskProfile{
        PrivilegeRisk:      b.getOrDefault(b.privilegeRisk),
        NetworkRisk:        b.getOrDefault(b.networkRisk),
        DestructionRisk:    b.getOrDefault(b.destructionRisk),
        DataExfilRisk:      b.getOrDefault(b.dataExfilRisk),
        SystemModRisk:      b.getOrDefault(b.systemModRisk),
        NetworkType:        b.networkType,
        NetworkSubcommands: b.networkSubcommands,
        IsPrivilege:        b.privilegeRisk != nil && b.privilegeRisk.Level >= runnertypes.RiskLevelHigh,
    }

    // Validate at build time
    if err := profile.Validate(); err != nil {
        panic(fmt.Sprintf("invalid profile for commands %v: %v", b.commands, err))
    }

    return CommandProfileDef{
        commands: b.commands,
        profile:  profile,
    }
}

func (b *ProfileBuilder) getOrDefault(risk *RiskFactor) RiskFactor {
    if risk == nil {
        return RiskFactor{Level: runnertypes.RiskLevelUnknown}
    }
    return *risk
}
```

**チェックリスト:**
- [ ] Build()メソッド実装
- [ ] getOrDefault()ヘルパー実装
- [ ] GoDocコメント追加
- [ ] `make test`実行（成功を確認）
- [ ] `make fmt`実行

### 3.5 Phase 1 完了チェック

- [ ] 全ユニットテストがパス
- [ ] `make lint`でエラーなし
- [ ] `make fmt`実行済み
- [ ] コードカバレッジ確認
- [ ] コミット作成（"feat: add RiskFactor and new CommandRiskProfile with builder pattern"）

## 4. フェーズ2: プロファイル定義の移行

### 4.1 Phase 2.1: 移行準備

#### タスク2.1.1: 既存プロファイル定義の分析

**ファイル:** `internal/runner/security/command_analysis.go`

**作業内容:**
- [ ] 現在の`commandProfileDefinitions`を確認
- [ ] 各プロファイルのリスク要因を分類
- [ ] 移行マッピング表を作成（ドキュメント）

#### タスク2.1.2: 移行対象リストの作成

**ドキュメント:** `docs/tasks/0029_risk_profile_redesign/05_migration_checklist.md`

**内容:**
- [ ] 全コマンドのリスト
- [ ] 各コマンドの移行前後の定義
- [ ] 移行優先順位（高リスクコマンドから）

### 4.2 Phase 2.2: 段階的移行

#### タスク2.2.1: 権限昇格コマンドの移行

**対象コマンド:** sudo, su, doas, pkexec

**移行例:**
```go
// Before
{
    commands: []string{"sudo", "su", "doas"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelCritical,
        Reason:        "Privilege escalation",
        IsPrivilege:   true,
        NetworkType:   NetworkTypeNone,
    },
}

// After
NewProfile("sudo", "su", "doas").
    PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges, can compromise entire system").
    Build()
```

**チェックリスト:**
- [ ] 移行実装
- [ ] リスクレベル一致を確認するテスト追加
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

#### タスク2.2.2: ネットワークコマンド（常時）の移行

**対象コマンド:** curl, wget, ssh, scp, sftp

**移行例:**
```go
// Before
{
    commands: []string{"curl", "wget"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelMedium,
        Reason:        "Network operations",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeAlways,
    },
}

// After
NewProfile("curl", "wget").
    NetworkRisk(runnertypes.RiskLevelMedium, "Always performs network operations").
    AlwaysNetwork().
    Build()
```

**チェックリスト:**
- [ ] 移行実装
- [ ] リスクレベル一致を確認するテスト追加
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

#### タスク2.2.3: ネットワークコマンド（条件付き）の移行

**対象コマンド:** git, rsync

**移行例:**
```go
// Before
{
    commands: []string{"git"},
    profile: CommandRiskProfile{
        BaseRiskLevel:      runnertypes.RiskLevelMedium,
        Reason:             "Network operations for certain subcommands",
        IsPrivilege:        false,
        NetworkType:        NetworkTypeConditional,
        NetworkSubcommands: []string{"clone", "fetch", "pull", "push", "remote"},
    },
}

// After
NewProfile("git").
    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for clone/fetch/pull/push/remote").
    ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
    Build()
```

**チェックリスト:**
- [ ] 移行実装
- [ ] リスクレベル一致を確認するテスト追加
- [ ] サブコマンド検出ロジックのテスト
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

#### タスク2.2.4: 破壊的操作コマンドの移行

**対象コマンド:** rm, dd, mkfs, shred

**移行例:**
```go
NewProfile("rm").
    DestructionRisk(runnertypes.RiskLevelHigh, "Can delete files and directories").
    Build()

NewProfile("dd").
    DestructionRisk(runnertypes.RiskLevelCritical, "Can overwrite entire disks, potential data loss").
    Build()
```

**チェックリスト:**
- [ ] 移行実装
- [ ] リスクレベル一致を確認するテスト追加
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

#### タスク2.2.5: AI CLIツールの移行（新規追加）

**対象コマンド:** claude, gemini, chatgpt

**実装例:**
```go
NewProfile("claude", "gemini", "chatgpt").
    NetworkRisk(runnertypes.RiskLevelHigh, "Always communicates with external AI API").
    DataExfilRisk(runnertypes.RiskLevelHigh, "May send sensitive data to external service").
    AlwaysNetwork().
    Build()
```

**チェックリスト:**
- [ ] 新規プロファイル追加
- [ ] テストケース追加
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

#### タスク2.2.6: システム変更コマンドの移行

**対象コマンド:** systemctl, service, apt, yum

**実装例:**
```go
NewProfile("systemctl", "service").
    SystemModRisk(runnertypes.RiskLevelHigh, "Can modify system services and configuration").
    Build()

NewProfile("apt", "apt-get").
    SystemModRisk(runnertypes.RiskLevelHigh, "Can install/remove system packages").
    NetworkRisk(runnertypes.RiskLevelMedium, "May download packages from network").
    ConditionalNetwork("install", "update", "upgrade", "dist-upgrade").
    Build()
```

**チェックリスト:**
- [ ] 移行実装
- [ ] 複数リスク要因のテスト追加
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

#### タスク2.2.7: 残りのコマンドの移行

**作業内容:**
- [ ] 全ての既存コマンド定義を新形式に移行
- [ ] 移行完了を確認するテスト追加
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

### 4.3 Phase 2.3: 移行完了チェック

#### タスク2.3.1: 全プロファイルのバリデーションテスト

**ファイル:** `internal/runner/security/command_analysis_test.go`

**テストケース:**
```go
func TestAllProfilesAreValid(t *testing.T) {
    for _, def := range commandProfileDefinitions {
        err := def.Profile().Validate()
        assert.NoError(t, err, "Profile for commands %v should be valid", def.Commands())
    }
}

func TestAllProfilesHaveReasons(t *testing.T) {
    for _, def := range commandProfileDefinitions {
        reasons := def.Profile().GetRiskReasons()
        assert.NotEmpty(t, reasons, "Profile for commands %v should have at least one reason", def.Commands())
    }
}
```

**チェックリスト:**
- [ ] テスト実装
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

#### タスク2.3.2: リスクレベル一致確認テスト

**ファイル:** `internal/runner/security/migration_test.go`

**テストケース:**
```go
func TestMigration_RiskLevelConsistency(t *testing.T) {
    // Old vs New risk level comparison
    tests := []struct {
        command   string
        oldRisk   runnertypes.RiskLevel
    }{
        {"sudo", runnertypes.RiskLevelCritical},
        {"curl", runnertypes.RiskLevelMedium},
        {"git", runnertypes.RiskLevelMedium},
        // ... all commands
    }

    for _, tt := range tests {
        t.Run(tt.command, func(t *testing.T) {
            profile := getProfileForCommand(tt.command)
            assert.Equal(t, tt.oldRisk, profile.BaseRiskLevel(),
                "Risk level mismatch for command %s", tt.command)
        })
    }
}
```

**チェックリスト:**
- [ ] テスト実装
- [ ] 全コマンドの移行前後一致を確認
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

### 4.4 Phase 2 完了チェック

- [ ] 全既存コマンドの移行完了
- [ ] 全ユニットテストがパス
- [ ] 全統合テストがパス
- [ ] `make lint`でエラーなし
- [ ] リスクレベル一致確認完了
- [ ] コミット作成（"feat: migrate all command profiles to new risk factor structure"）

## 5. フェーズ3: 機能拡張

### 5.1 Phase 3.1: 監査ログの拡張

#### タスク3.1.1: リスク要因詳細のログ出力

**ファイル:** `internal/runner/audit/audit_logger.go`

**実装内容:**
- リスク要因の内訳をログに出力
- 従来の形式との互換性を維持

**実装例:**
```go
func (l *AuditLogger) logRiskProfile(profile CommandRiskProfile) {
    baseRisk := profile.BaseRiskLevel()
    l.logf("Risk Level: %v", baseRisk)

    reasons := profile.GetRiskReasons()
    if len(reasons) > 0 {
        l.logf("Risk Factors:")
        for _, reason := range reasons {
            l.logf("  - %s", reason)
        }
    }
}
```

**チェックリスト:**
- [ ] ログ出力メソッド実装
- [ ] テストケース追加
- [ ] ログフォーマットの確認
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

#### タスク3.1.2: リスク要因別の統計情報

**ファイル:** `internal/runner/audit/statistics.go`

**実装内容:**
- 実行されたコマンドのリスク要因別集計
- レポート生成機能（オプション）

**チェックリスト:**
- [ ] 統計機能実装
- [ ] テストケース追加
- [ ] `make test`実行（成功を確認）
- [ ] コミット作成

### 5.2 Phase 3.2: ドキュメント整備

#### タスク3.2.1: APIドキュメントの更新

**作業内容:**
- [ ] 全公開型のGoDocコメント確認
- [ ] 使用例の追加
- [ ] `godoc`で確認

#### タスク3.2.2: 移行ガイドの作成

**ドキュメント:** `docs/tasks/0029_risk_profile_redesign/06_migration_guide.md`

**内容:**
- [ ] 新旧プロファイルの対応表
- [ ] 典型的な移行パターン
- [ ] トラブルシューティング

#### タスク3.2.3: 設計ドキュメントの最終更新

**作業内容:**
- [ ] 要件定義書の更新
- [ ] アーキテクチャ設計書の更新
- [ ] 詳細仕様書の更新
- [ ] 実装完了チェック

### 5.3 Phase 3 完了チェック

- [ ] 監査ログ拡張完了
- [ ] 全ドキュメント整備完了
- [ ] 全ユニットテストがパス
- [ ] 全統合テストがパス
- [ ] `make lint`でエラーなし
- [ ] コミット作成（"feat: enhance audit logging with risk factor details"）

## 6. 最終検証

### 6.1 全体テスト

- [ ] 全ユニットテストがパス
- [ ] 全統合テストがパス
- [ ] `make lint`でエラーなし
- [ ] `make fmt`実行済み
- [ ] コードカバレッジ: 80%以上

### 6.2 パフォーマンステスト

- [ ] メモリ使用量の確認（増加が2KB程度であることを確認）
- [ ] `BaseRiskLevel()`の実行時間測定
- [ ] ベンチマークテスト実行

### 6.3 セキュリティレビュー

- [ ] バリデーションロジックの確認
- [ ] エラーハンドリングの確認
- [ ] プロファイル定義の妥当性確認

### 6.4 ドキュメントレビュー

- [ ] 全ドキュメントの内容確認
- [ ] コードコメントの確認
- [ ] 移行ガイドの動作確認

## 7. リリース準備

### 7.1 変更履歴の作成

**ファイル:** `CHANGELOG.md`

**内容:**
```markdown
## [Unreleased]

### Added
- New risk factor-based command risk profiling system
- Builder pattern DSL for defining command risk profiles
- Detailed risk factor breakdown in audit logs
- Risk factor validation at build time

### Changed
- Migrated all command risk profiles to new structure
- Enhanced audit logging with risk factor details

### Deprecated
- (None)

### Removed
- (None)

### Fixed
- (None)

### Security
- Improved risk assessment transparency with explicit risk factors
```

**チェックリスト:**
- [ ] CHANGELOG.md更新
- [ ] バージョン番号の決定
- [ ] コミット作成

### 7.2 最終コミット

**コミットメッセージ:**
```
feat: refactor command risk profiling with explicit risk factors

- Introduce RiskFactor type for individual risk components
- Implement builder pattern DSL for profile definitions
- Migrate all existing command profiles to new structure
- Add risk factor validation at build time
- Enhance audit logging with risk factor breakdown

This refactoring improves:
- Type safety with compile-time validation
- Maintainability with explicit risk separation
- Auditability with detailed risk factor reporting
- Extensibility for future risk factor additions

Closes #XXX
```

**チェックリスト:**
- [ ] 全変更をステージング
- [ ] コミット作成
- [ ] タグ作成（バージョン番号）

## 8. 成功基準の確認

最終的に以下の成功基準を全て満たすこと：

- [ ] 全ての既存コマンド定義を新構造に移行完了
- [ ] バリデーションテストが全てパス
- [ ] パフォーマンスが既存実装と同等以上
- [ ] 監査ログにリスク要因の内訳が出力される
- [ ] 全ドキュメントが整備されている
- [ ] セキュリティレビューで承認される

## 9. ロールバック計画

万が一問題が発生した場合のロールバック手順：

1. [ ] 最終コミット前のコミットに戻る
2. [ ] 既存のテストが全てパスすることを確認
3. [ ] 問題の原因を特定
4. [ ] 修正または再設計の判断

## 10. 今後の拡張計画

本実装完了後の拡張候補：

### 短期（3ヶ月以内）
- [ ] セキュリティレポート生成機能
- [ ] リスク要因別の統計ダッシュボード

### 中期（6ヶ月以内）
- [ ] 新リスク要因の追加（CryptoRisk、DatabaseRisk）
- [ ] コマンド引数に基づく動的リスク評価

### 長期（1年以内）
- [ ] 機械学習によるリスク評価
- [ ] 外部セキュリティデータベースとの統合
