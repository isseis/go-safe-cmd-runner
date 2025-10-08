# 移行マッピング表: 既存プロファイルから新形式への変換

## 概要

本ドキュメントは、既存の`commandGroupDefinitions`を新しいリスク要因ベースの形式に移行するためのマッピング表を提供する。

## リスク要因の分類

新しい形式では、以下のリスク要因を明示的に分離する：

- **PrivilegeRisk**: 権限昇格に関するリスク
- **NetworkRisk**: ネットワーク操作に関するリスク
- **DestructionRisk**: 破壊的操作（ファイル削除、ディスクフォーマットなど）に関するリスク
- **DataExfilRisk**: 外部サービスへのデータ流出に関するリスク
- **SystemModRisk**: システム設定変更に関するリスク

## 移行マッピング表

### 1. 権限昇格コマンド (Privilege Escalation)

**既存定義:**
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

**新形式:**
```go
NewProfile("sudo", "su", "doas").
    PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges, can compromise entire system").
    Build()
```

**リスク要因分析:**
- PrivilegeRisk: Critical (権限昇格の主要リスク)
- 他のリスク要因: なし

---

### 2. システム制御コマンド (System Control)

**既存定義:**
```go
{
    commands: []string{"systemctl", "service"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelHigh,
        Reason:        "System control",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeNone,
    },
}
```

**新形式:**
```go
NewProfile("systemctl", "service").
    SystemModRisk(runnertypes.RiskLevelHigh, "Can modify system services and configuration").
    Build()
```

**リスク要因分析:**
- SystemModRisk: High (システムサービスの変更)
- 他のリスク要因: なし

---

### 3. 破壊的操作コマンド (Destructive Operations)

**既存定義:**
```go
{
    commands: []string{"rm", "dd"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelHigh,
        Reason:        "Destructive operations",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeNone,
    },
}
```

**新形式:**
```go
NewProfile("rm").
    DestructionRisk(runnertypes.RiskLevelHigh, "Can delete files and directories").
    Build()

NewProfile("dd").
    DestructionRisk(runnertypes.RiskLevelCritical, "Can overwrite entire disks, potential data loss").
    Build()
```

**リスク要因分析:**
- rm: DestructionRisk High (ファイル削除)
- dd: DestructionRisk Critical (ディスク全体の上書き可能)
- 注: `dd`は`rm`よりも高いリスクレベルを持つため、分離して定義

---

### 4. AIサービスコマンド (AI Service with Data Exfiltration)

**既存定義:**
```go
{
    commands: []string{"claude", "gemini", "chatgpt", "gpt", "openai", "anthropic"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelHigh,
        Reason:        "AI service with potential data exfiltration",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeAlways,
    },
}
```

**新形式:**
```go
NewProfile("claude", "gemini", "chatgpt", "gpt", "openai", "anthropic").
    NetworkRisk(runnertypes.RiskLevelHigh, "Always communicates with external AI API").
    DataExfilRisk(runnertypes.RiskLevelHigh, "May send sensitive data to external service").
    AlwaysNetwork().
    Build()
```

**リスク要因分析:**
- NetworkRisk: High (常にネットワーク通信)
- DataExfilRisk: High (機密データの外部送信可能性)
- NetworkType: Always

---

### 5. ネットワーク要求コマンド (Network Request)

**既存定義:**
```go
{
    commands: []string{"curl", "wget"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelMedium,
        Reason:        "Network request",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeAlways,
    },
}
```

**新形式:**
```go
NewProfile("curl", "wget").
    NetworkRisk(runnertypes.RiskLevelMedium, "Always performs network operations").
    AlwaysNetwork().
    Build()
```

**リスク要因分析:**
- NetworkRisk: Medium (ネットワーク通信)
- NetworkType: Always
- 注: AIサービスと異なり、DataExfilRiskは設定しない（明示的なデータ流出意図がないため）

---

### 6. ネットワーク接続コマンド (Network Connection)

**既存定義:**
```go
{
    commands: []string{"nc", "netcat", "telnet"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelMedium,
        Reason:        "Network connection",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeAlways,
    },
}
```

**新形式:**
```go
NewProfile("nc", "netcat", "telnet").
    NetworkRisk(runnertypes.RiskLevelMedium, "Establishes network connections").
    AlwaysNetwork().
    Build()
```

**リスク要因分析:**
- NetworkRisk: Medium (ネットワーク接続)
- NetworkType: Always

---

### 7. リモート操作コマンド (Remote Operations)

**既存定義:**
```go
{
    commands: []string{"ssh", "scp"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelMedium,
        Reason:        "Remote operations",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeAlways,
    },
}
```

**新形式:**
```go
NewProfile("ssh", "scp").
    NetworkRisk(runnertypes.RiskLevelMedium, "Remote operations via network").
    AlwaysNetwork().
    Build()
```

**リスク要因分析:**
- NetworkRisk: Medium (ネットワーク経由のリモート操作)
- NetworkType: Always

---

### 8. Git (条件付きネットワーク)

**既存定義:**
```go
{
    commands: []string{"git"},
    profile: CommandRiskProfile{
        BaseRiskLevel:      runnertypes.RiskLevelLow,
        Reason:             "Conditional network operations",
        IsPrivilege:        false,
        NetworkType:        NetworkTypeConditional,
        NetworkSubcommands: []string{"clone", "fetch", "pull", "push", "remote"},
    },
}
```

**新形式:**
```go
NewProfile("git").
    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for clone/fetch/pull/push/remote").
    ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
    Build()
```

**リスク要因分析:**
- NetworkRisk: Medium (サブコマンドによってはネットワーク通信)
- NetworkType: Conditional
- NetworkSubcommands: ["clone", "fetch", "pull", "push", "remote"]
- 注: BaseRiskLevelはLowからMediumに変更（ネットワーク操作時のリスクを反映）

---

### 9. rsync (条件付きネットワーク)

**既存定義:**
```go
{
    commands: []string{"rsync"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelLow,
        Reason:        "Conditional network operations",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeConditional,
    },
}
```

**新形式:**
```go
NewProfile("rsync").
    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations when using remote sources/destinations").
    ConditionalNetwork().
    Build()
```

**リスク要因分析:**
- NetworkRisk: Medium (リモートソース/宛先使用時)
- NetworkType: Conditional
- NetworkSubcommands: なし（引数ベースで判定）
- 注: BaseRiskLevelはLowからMediumに変更（ネットワーク操作時のリスクを反映）

---

### 10. クラウドサービスコマンド (Cloud Service Operations)

**既存定義:**
```go
{
    commands: []string{"aws"},
    profile: CommandRiskProfile{
        BaseRiskLevel: runnertypes.RiskLevelMedium,
        Reason:        "Cloud service operations",
        IsPrivilege:   false,
        NetworkType:   NetworkTypeAlways,
    },
}
```

**新形式:**
```go
NewProfile("aws").
    NetworkRisk(runnertypes.RiskLevelMedium, "Cloud service operations via network").
    AlwaysNetwork().
    Build()
```

**リスク要因分析:**
- NetworkRisk: Medium (クラウドサービス操作)
- NetworkType: Always

---

## リスクレベル変更の要約

以下のコマンドでリスクレベルが変更される：

| コマンド | 旧リスクレベル | 新リスクレベル | 理由 |
|---------|-------------|-------------|------|
| dd      | High        | Critical    | ディスク全体の上書き可能性を明示 |
| git     | Low         | Medium      | ネットワーク操作時のリスクを反映 |
| rsync   | Low         | Medium      | ネットワーク操作時のリスクを反映 |

**注意:** `git`と`rsync`は条件付きネットワークコマンドのため、実際の実行時のリスクレベルはコンテキストに依存する。新形式では`NetworkRisk`がMediumとして明示的に定義されるが、`BaseRiskLevel()`メソッドは全リスク要因の最大値を返すため、ネットワーク操作時はMediumとなる。

## 移行優先順位

1. **Critical/High risk コマンド**: 権限昇格、破壊的操作、システム変更
2. **AI サービスコマンド**: データ流出リスクを含む複合リスク
3. **ネットワークコマンド (常時)**: 単一リスク要因
4. **ネットワークコマンド (条件付き)**: 複雑な条件分岐を含む

## バリデーション

移行後は以下を確認する：

1. **リスクレベル一致**: `BaseRiskLevel()`が旧`BaseRiskLevel`と一致する（意図的な変更を除く）
2. **IsPrivilege一致**: 権限昇格コマンドで`IsPrivilege`がtrueになる
3. **NetworkType一致**: ネットワークタイプが保持される
4. **NetworkSubcommands一致**: 条件付きネットワークコマンドのサブコマンドリストが保持される
5. **バリデーション成功**: 全プロファイルが`Validate()`を通過する
