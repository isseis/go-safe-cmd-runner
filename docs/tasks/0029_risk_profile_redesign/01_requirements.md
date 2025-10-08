# 要件定義: コマンドリスクプロファイルのリファクタリング

## 1. 背景

### 1.1 現状の問題点

現在の`CommandRiskProfile`構造には以下の問題がある：

1. **暗黙的な依存関係**
   - `NetworkType == NetworkTypeAlways`の場合、`BaseRiskLevel`は`Medium`以上であるべきだが、これが型システムで強制されていない
   - 現在はバリデーション関数で実行時チェックを行っているが、理想的には設計で防ぐべき

2. **リスク要因の不透明性**
   - `BaseRiskLevel`が何に基づいて決定されるのか（ネットワーク操作、破壊性、権限昇格など）が明確でない
   - 複数のリスク要因が混在する場合の優先順位が不明確

3. **拡張性の問題**
   - 新しいリスク要因（データ流出リスク、暗号化リスク、APIアクセスリスクなど）を追加する際の設計が不明確
   - リスク評価ロジックがコード全体に分散している

4. **監査とレポートの困難さ**
   - なぜそのリスクレベルになったのか、内訳を説明できない
   - セキュリティ監査時に各リスク要因を個別に評価できない

### 1.2 具体的な問題例

```go
// 現在の定義
{
    commands: []string{"aws"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelMedium,  // なぜMedium?
        Reason:        "Cloud service operations",    // 曖昧
        IsPrivilege:   false,
        NetworkType:   NetworkTypeAlways,
    },
}
```

このケースでは：
- ネットワーク操作によるリスク？
- クラウドサービス特有のリスク？
- データ流出リスク？

などの要因が混在しているが、個別に評価できない。

## 2. 目標

### 2.1 主要目標

1. **リスク要因の明示化**
   - 各コマンドのリスクを構成する要因を明確に分離
   - ネットワークリスク、破壊リスク、権限リスク、データ流出リスクなどを独立して評価

2. **型安全性の向上**
   - 可能な限り型システムで一貫性を保証
   - 実行時エラーではなくコンパイル時エラーで問題を検出

3. **拡張性の確保**
   - 新しいリスク要因の追加が容易
   - 既存コードへの影響を最小化

4. **監査可能性の向上**
   - リスク評価の根拠を明確に記録
   - 各リスク要因の内訳を監査ログに出力可能

### 2.2 非目標

- 既存のリスクレベル判定ロジックの変更（互換性を維持）
- パフォーマンスの大幅な改善（現状維持で可）

## 3. 提案する設計

### 3.1 新しいCommandRiskProfile構造

```go
// RiskFactor represents individual risk factors for a command
type RiskFactor struct {
    Level  runnertypes.RiskLevel
    Reason string
}

// CommandRiskProfile defines comprehensive risk information for a command
type CommandRiskProfile struct {
    // Individual risk factors (explicit separation)
    NetworkRisk       RiskFactor  // Risk from network operations
    DestructionRisk   RiskFactor  // Risk from destructive operations (rm, dd, etc.)
    PrivilegeRisk     RiskFactor  // Risk from privilege escalation
    DataExfilRisk     RiskFactor  // Risk from data exfiltration to external services
    SystemModRisk     RiskFactor  // Risk from system modifications (systemctl, etc.)

    // Network behavior configuration
    NetworkType        NetworkOperationType
    NetworkSubcommands []string

    // Computed properties (derived from risk factors)
    IsPrivilege bool  // Convenience flag (true if PrivilegeRisk.Level >= runnertypes.RiskLevelHigh)
}

// BaseRiskLevel computes the overall risk level as the maximum of all risk factors
func (p CommandRiskProfile) BaseRiskLevel() runnertypes.RiskLevel {
    return max(
        p.NetworkRisk.Level,
        p.DestructionRisk.Level,
        p.PrivilegeRisk.Level,
        p.DataExfilRisk.Level,
        p.SystemModRisk.Level,
    )
}

// GetRiskReasons returns all reasons contributing to the risk level
func (p CommandRiskProfile) GetRiskReasons() []string {
    var reasons []string
    if p.NetworkRisk.Level > runnertypes.RiskLevelUnknown {
        reasons = append(reasons, p.NetworkRisk.Reason)
    }
    if p.DestructionRisk.Level > runnertypes.RiskLevelUnknown {
        reasons = append(reasons, p.DestructionRisk.Reason)
    }
    // ... other factors
    return reasons
}

// Validate ensures consistency between risk factors and configuration
func (p CommandRiskProfile) Validate() error {
    // Rule 1: NetworkTypeAlways implies NetworkRisk >= Medium
    if p.NetworkType == NetworkTypeAlways && p.NetworkRisk.Level < runnertypes.RiskLevelMedium {
        return fmt.Errorf("%w: NetworkTypeAlways requires NetworkRisk >= Medium (got %v)",
            ErrInconsistentRiskProfile, p.NetworkRisk.Level)
    }

    // Rule 2: IsPrivilege implies PrivilegeRisk >= High
    if p.IsPrivilege && p.PrivilegeRisk.Level < runnertypes.RiskLevelHigh {
        return fmt.Errorf("%w: IsPrivilege requires PrivilegeRisk >= High (got %v)",
            ErrInconsistentRiskProfile, p.PrivilegeRisk.Level)
    }

    // Rule 3: NetworkSubcommands only for NetworkTypeConditional
    if len(p.NetworkSubcommands) > 0 && p.NetworkType != NetworkTypeConditional {
        return fmt.Errorf("%w: NetworkSubcommands only for NetworkTypeConditional",
            ErrInconsistentRiskProfile)
    }

    return nil
}
```

### 3.2 使用例

```go
// AI CLIツールの定義例
{
    commands: []string{"claude", "gemini", "chatgpt"},
    profile: CommandRiskProfile{
        NetworkRisk: RiskFactor{
            Level:  runnertypes.RiskLevelHigh,
            Reason: "Always communicates with external AI API",
        },
        DataExfilRisk: RiskFactor{
            Level:  runnertypes.RiskLevelHigh,
            Reason: "May send sensitive data to external service",
        },
        DestructionRisk: RiskFactor{
            Level:  runnertypes.RiskLevelUnknown,
            Reason: "",
        },
        PrivilegeRisk: RiskFactor{
            Level:  runnertypes.RiskLevelUnknown,
            Reason: "",
        },
        SystemModRisk: RiskFactor{
            Level:  runnertypes.RiskLevelUnknown,
            Reason: "",
        },
        NetworkType:   NetworkTypeAlways,
        IsPrivilege:   false,
    },
}

// BaseRiskLevel() -> RiskLevelHigh (max of Network and DataExfil)
// GetRiskReasons() -> ["Always communicates with external AI API",
//                      "May send sensitive data to external service"]
```

### 3.3 ビルダーパターンによるDSL

定義を簡潔にするため、ビルダーパターンを採用する。セキュリティ上の理由から、これらの定義はソースコードにハードコードされ、ユーザーによるオーバーライドは許可しない（または、root権限でのみ書き込み可能なディレクトリに格納）。

```go
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

// 使用例
var commandProfileDefinitions = []CommandProfileDef{
    NewProfile("claude", "gemini", "chatgpt").
        NetworkRisk(runnertypes.RiskLevelHigh, "Always communicates with external AI API").
        DataExfilRisk(runnertypes.RiskLevelHigh, "May send sensitive data to external service").
        AlwaysNetwork().
        Build(),

    NewProfile("sudo", "su", "doas").
        PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges").
        Build(),

    NewProfile("git").
        NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for clone/fetch/pull/push/remote").
        ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
        Build(),
}
```

**DSL採用の理由：**

1. **簡潔性**: 現在の構造体リテラルよりも読みやすく、記述量が少ない
2. **型安全性**: コンパイル時に型チェックが行われる
3. **バリデーション**: `Build()`時に整合性チェックを実行し、panicで即座に問題を検出
4. **メンテナンス性**: IDEのコード補完が効き、リファクタリングが容易
5. **セキュリティ**: ソースコードにハードコードされ、ユーザーが改変できない

**代替案との比較：**

- **TOML/YAML設定ファイル**: セキュリティ上の理由で却下（ユーザーによる改変リスク）
- **構造体リテラル**: 冗長で読みにくい
- **ビルダーパターン（採用）**: 型安全で簡潔、コンパイル時バリデーションが可能

## 4. 移行計画

### 4.1 フェーズ1: 新構造の導入（後方互換性維持）

1. `RiskFactor`型の追加
2. 新しい`CommandRiskProfile`構造の追加
3. 既存の`CommandRiskProfile`を`LegacyCommandRiskProfile`にリネーム
4. 既存コードは`LegacyCommandRiskProfile`を使い続ける

### 4.2 フェーズ2: 段階的な移行

1. 新しいコマンド定義は新構造を使用
2. 既存定義を少しずつ移行
3. 移行完了後、`LegacyCommandRiskProfile`を削除

### 4.3 フェーズ3: 監査ログの拡張

1. リスク要因の内訳を監査ログに出力
2. セキュリティレポート機能の追加

## 5. 期待される効果

### 5.1 開発者体験の向上

- リスク要因が明確になり、新しいコマンド定義が容易に
- バリデーションにより設定ミスを早期発見

### 5.2 セキュリティ監査の改善

- 各コマンドのリスク評価根拠が明確に
- リスク要因別のレポート生成が可能

### 5.3 保守性の向上

- リスク評価ロジックが一箇所に集約
- 新しいリスク要因の追加が容易

## 6. リスクと対策

### 6.1 技術的リスク

| リスク | 影響 | 対策 |
|--------|------|------|
| 既存コードの大規模な変更が必要 | 高 | 段階的移行により影響を最小化 |
| パフォーマンスへの影響 | 低 | 構造が複雑化するがコンパイル時に解決 |
| バグ混入のリスク | 中 | 包括的なテストケースの作成 |

### 6.2 運用リスク

| リスク | 影響 | 対策 |
|--------|------|------|
| 学習コストの増加 | 中 | ヘルパー関数とドキュメントで軽減 |
| 既存定義の移行コスト | 高 | 自動変換ツールの提供を検討 |

## 7. 代替案との比較

### 7.1 現状維持 + バリデーション強化（採用済み）

**pros:**
- 最小限の変更
- 短期的には十分

**cons:**
- 根本的な設計改善ではない
- リスク要因の内訳が不明確

### 7.2 カテゴリベースの分類

**pros:**
- 分類が明確
- レポート生成が容易

**cons:**
- カテゴリが固定的
- 複数カテゴリの扱いが難しい

### 7.3 リスク要因の分離（本提案）

**pros:**
- 最も柔軟で拡張性が高い
- 監査可能性が最も高い

**cons:**
- 実装コストが最も高い
- 移行に時間がかかる

## 8. 実装の優先順位

1. **高**: `RiskFactor`型と新`CommandRiskProfile`の実装
2. **高**: バリデーション関数の実装
3. **中**: ヘルパー関数の実装
4. **中**: 既存定義の移行
5. **低**: 監査ログの拡張
6. **低**: レポート機能の追加

## 9. 成功基準

- [ ] 全ての既存コマンド定義を新構造に移行完了
- [ ] バリデーションテストが全てパス
- [ ] パフォーマンスが既存実装と同等以上
- [ ] 監査ログにリスク要因の内訳が出力される
- [ ] セキュリティレビューで承認される
