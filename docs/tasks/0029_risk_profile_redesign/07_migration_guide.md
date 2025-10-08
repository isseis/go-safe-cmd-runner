# コマンドリスクプロファイル移行ガイド

## 1. 概要

このガイドは、従来の`CommandRiskProfile`から新しい`CommandRiskProfileNew`とProfileBuilderパターンへの移行方法を説明します。

## 2. 主な変更点

### 2.1 構造の変更

**従来の構造:**
```go
type CommandRiskProfile struct {
    BaseRiskLevel runnertypes.RiskLevel
    Reason        string
    IsPrivilege   bool
    NetworkType   NetworkOperationType
    NetworkSubcommands []string
}
```

**新しい構造:**
```go
type CommandRiskProfileNew struct {
    // 個別のリスク要因
    PrivilegeRisk   RiskFactor
    NetworkRisk     RiskFactor
    DestructionRisk RiskFactor
    DataExfilRisk   RiskFactor
    SystemModRisk   RiskFactor

    // ネットワーク動作設定
    NetworkType        NetworkOperationType
    NetworkSubcommands []string
}

type RiskFactor struct {
    Level  runnertypes.RiskLevel
    Reason string
}
```

### 2.2 主要な改善点

1. **リスク要因の明示的分離**: 単一の`BaseRiskLevel`と`Reason`から、5つの独立したリスク要因へ
2. **型安全性の向上**: ProfileBuilderによるビルド時バリデーション
3. **保守性の向上**: 各リスク要因が独立して管理可能
4. **監査性の向上**: リスク要因別の詳細な理由を提供

## 3. 移行パターン

### 3.1 権限昇格コマンド

**移行前:**
```go
{
    commands: []string{"sudo", "su", "doas"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelCritical,
        Reason:        "Privilege escalation",
        IsPrivilege:   true,
        NetworkType:   NetworkTypeNone,
    },
}
```

**移行後:**
```go
NewProfile("sudo", "su", "doas").
    PrivilegeRisk(runnertypes.RiskLevelCritical,
        "Allows execution with elevated privileges, can compromise entire system").
    Build()
```

**ポイント:**
- `IsPrivilege`は自動的に`PrivilegeRisk.Level >= High`で判定される
- より詳細な理由説明が可能

### 3.2 ネットワークコマンド（常時）

**移行前:**
```go
{
    commands: []string{"curl", "wget"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelMedium,
        Reason:        "Network operations",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeAlways,
    },
}
```

**移行後:**
```go
NewProfile("curl", "wget").
    NetworkRisk(runnertypes.RiskLevelMedium, "Always performs network operations").
    AlwaysNetwork().
    Build()
```

**ポイント:**
- `AlwaysNetwork()`で`NetworkType`を設定
- ネットワークリスクを`NetworkRisk`として明示

### 3.3 ネットワークコマンド（条件付き）

**移行前:**
```go
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
```

**移行後:**
```go
NewProfile("git").
    NetworkRisk(runnertypes.RiskLevelMedium,
        "Network operations for clone/fetch/pull/push/remote").
    ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
    Build()
```

**ポイント:**
- `ConditionalNetwork()`にサブコマンドを渡す
- サブコマンド実行時のみネットワークリスクが適用される

### 3.4 破壊的操作コマンド

**移行前:**
```go
{
    commands: []string{"rm"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelHigh,
        Reason:        "Destructive operations",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeNone,
    },
}
```

**移行後:**
```go
NewProfile("rm").
    DestructionRisk(runnertypes.RiskLevelHigh,
        "Can delete files and directories").
    Build()
```

**ポイント:**
- 破壊的操作は`DestructionRisk`として明示
- より具体的な理由を記述

### 3.5 複数リスク要因を持つコマンド

**新規追加例（AI CLIツール）:**
```go
NewProfile("claude", "gemini", "chatgpt").
    NetworkRisk(runnertypes.RiskLevelHigh,
        "Always communicates with external AI API").
    DataExfilRisk(runnertypes.RiskLevelHigh,
        "May send sensitive data to external service").
    AlwaysNetwork().
    Build()
```

**ポイント:**
- 複数のリスク要因を個別に設定可能
- `BaseRiskLevel()`は全リスク要因の最大値を返す
- `GetRiskReasons()`で全ての理由を取得可能

### 3.6 システム変更コマンド

**移行後の例:**
```go
// システムサービス管理
NewProfile("systemctl", "service").
    SystemModRisk(runnertypes.RiskLevelHigh,
        "Can modify system services and configuration").
    Build()

// パッケージ管理（複数リスク要因）
NewProfile("apt", "apt-get").
    SystemModRisk(runnertypes.RiskLevelHigh,
        "Can install/remove system packages").
    NetworkRisk(runnertypes.RiskLevelMedium,
        "May download packages from network").
    ConditionalNetwork("install", "update", "upgrade", "dist-upgrade").
    Build()
```

**ポイント:**
- `SystemModRisk`でシステム変更のリスクを明示
- 複数のリスク要因を組み合わせて使用可能

## 4. バリデーションルール

新しいProfileBuilderは以下のバリデーションをビルド時に実行します：

### 4.1 NetworkTypeAlwaysとNetworkRisk

**ルール:** `NetworkTypeAlways`の場合、`NetworkRisk.Level >= Medium`である必要があります。

**エラー例:**
```go
// これはpanicを引き起こす
NewProfile("test").
    NetworkRisk(runnertypes.RiskLevelLow, "test").
    AlwaysNetwork().
    Build()  // panic: NetworkTypeAlways commands must have NetworkRisk >= Medium
```

### 4.2 NetworkSubcommandsの使用

**ルール:** `NetworkSubcommands`は`NetworkTypeConditional`の場合のみ設定可能です。

**正しい使い方:**
```go
NewProfile("git").
    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations").
    ConditionalNetwork("clone", "fetch").  // OK
    Build()
```

**誤った使い方:**
```go
// これはpanicを引き起こす - AlwaysNetworkなのにNetworkRiskがLow
NewProfile("curl").
    NetworkRisk(runnertypes.RiskLevelLow, "test").  // Lowは不十分
    AlwaysNetwork().  // AlwaysNetworkはMedium以上が必要
    Build()  // panic: NetworkTypeAlways requires NetworkRisk >= Medium
```

## 5. よくある質問

### Q1: リスクレベルが変わることはありますか？

A: いいえ。移行により`BaseRiskLevel()`の計算方法が変わりましたが、結果は同じです。全リスク要因の最大値を返します。

### Q2: 既存のコードは動作しますか？

A: はい。移行は段階的に行われ、現在は`CommandRiskProfile`と`CommandRiskProfileNew`が共存しています。最終的には`CommandRiskProfile`が削除される予定です。

### Q3: 複数のリスク要因を設定する順序は重要ですか？

A: いいえ。リスク要因の設定順序は結果に影響しません。ただし、可読性のため、以下の順序を推奨します：
1. PrivilegeRisk
2. NetworkRisk
3. DestructionRisk
4. DataExfilRisk
5. SystemModRisk

### Q4: 理由（Reason）は必須ですか？

A: リスクレベルが`Unknown`より高い場合は推奨されます。空文字列も可能ですが、監査ログでの追跡が困難になります。

### Q5: カスタムリスク要因を追加できますか？

A: 現在は5つのリスク要因のみサポートしています。新しいリスク要因を追加する場合は、`CommandRiskProfileNew`構造体を拡張する必要があります。

## 6. トラブルシューティング

### 問題: Build()でpanicが発生する

**原因:** バリデーションエラーです。エラーメッセージを確認してください。

**解決方法:**
1. `NetworkTypeAlways`の場合、`NetworkRisk.Level >= Medium`を確認
2. `NetworkSubcommands`を使用する場合、`ConditionalNetwork()`を使用しているか確認

### 問題: GetRiskReasons()が空を返す

**原因:** 全てのリスク要因が`Unknown`レベル、または理由が空文字列です。

**解決方法:**
1. 少なくとも1つのリスク要因を`Unknown`より高いレベルに設定
2. 理由を空文字列でなく具体的な説明に設定

### 問題: IsPrivilege()がfalseを返す

**原因:** `PrivilegeRisk.Level < High`です。

**解決方法:**
`PrivilegeRisk`を`High`または`Critical`に設定してください。

## 7. 移行チェックリスト

新しいプロファイルを定義する際は、以下を確認してください：

- [ ] 適切なリスク要因を選択（Privilege, Network, Destruction, DataExfil, SystemMod）
- [ ] 各リスク要因に具体的な理由を記述
- [ ] ネットワーク動作を正しく設定（None, Always, Conditional）
- [ ] 条件付きネットワークの場合、サブコマンドを正しく指定
- [ ] `make test`でテストが通過することを確認
- [ ] `make lint`でエラーがないことを確認

## 8. 参考資料

- [要件定義書](01_requirements.md)
- [アーキテクチャ設計書](02_architecture.md)
- [詳細仕様書](03_specification.md)
- [実装計画書](04_implementation_plan.md)
- [移行チェックリスト](05_migration_checklist.md)
