# 詳細仕様書: コマンドリスクプロファイルのリファクタリング

## 1. 概要

本仕様書は、コマンドリスクプロファイルシステムのリファクタリングにおける詳細仕様を定義する。リスク要因の明示的な分離とビルダーパターンによるDSLを実装し、型安全性と保守性を向上させる。

## 2. データ構造仕様

### 2.1 RiskFactor

個別のリスク要因を表す構造体。

```go
// RiskFactor represents an individual risk factor with its level and explanation
type RiskFactor struct {
    Level  runnertypes.RiskLevel // Risk level for this specific factor
    Reason string                // Human-readable explanation of this risk
}
```

**フィールド仕様:**

| フィールド | 型 | 必須 | 説明 |
|-----------|---|------|------|
| `Level` | `runnertypes.RiskLevel` | Yes | このリスク要因のレベル（Unknown, Low, Medium, High, Critical） |
| `Reason` | `string` | No | リスクの説明（監査ログやデバッグで使用）。空文字列も許可 |

**制約:**
- `Level`が`Unknown`の場合、`Reason`は空文字列を推奨（ただし強制ではない）
- `Level`が`Unknown`以外の場合、`Reason`は非空文字列を強く推奨（バリデーションでは強制しないが、`GetRiskReasons()`で空文字列は除外される）

### 2.2 CommandRiskProfile

コマンドの包括的なリスクプロファイル。

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

**フィールド仕様:**

| フィールド | 型 | デフォルト | 説明 |
|-----------|---|-----------|------|
| `PrivilegeRisk` | `RiskFactor` | `{Unknown, ""}` | 権限昇格リスク |
| `NetworkRisk` | `RiskFactor` | `{Unknown, ""}` | ネットワーク操作リスク |
| `DestructionRisk` | `RiskFactor` | `{Unknown, ""}` | 破壊的操作リスク |
| `DataExfilRisk` | `RiskFactor` | `{Unknown, ""}` | データ流出リスク |
| `SystemModRisk` | `RiskFactor` | `{Unknown, ""}` | システム変更リスク |
| `NetworkType` | `NetworkOperationType` | `NetworkTypeNone` | ネットワーク操作の種類 |
| `NetworkSubcommands` | `[]string` | `nil` | ネットワーク操作を引き起こすサブコマンド |
| `IsPrivilege` | `bool` | `false` | 権限昇格コマンドフラグ |

**制約:**
- `NetworkType == NetworkTypeAlways` の場合、`NetworkRisk.Level >= Medium`
- `IsPrivilege == true` の場合、`PrivilegeRisk.Level >= High`
- `len(NetworkSubcommands) > 0` の場合、`NetworkType == NetworkTypeConditional`

### 2.3 CommandProfileDef

コマンドリストとプロファイルの組。

```go
// CommandProfileDef associates a list of commands with their risk profile
type CommandProfileDef struct {
    commands []string
    profile  CommandRiskProfile
}
```

**フィールド仕様:**

| フィールド | 型 | 必須 | 説明 |
|-----------|---|------|------|
| `commands` | `[]string` | Yes | このプロファイルを適用するコマンド名のリスト |
| `profile` | `CommandRiskProfile` | Yes | リスクプロファイル |

**制約:**
- `commands`は少なくとも1つの要素を持つ
- `commands`の各要素は非空文字列
- `profile`は`Validate()`をパスする

### 2.4 ProfileBuilder

ビルダーパターンによるDSL実装。

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
```

**フィールド仕様:**

| フィールド | 型 | 説明 |
|-----------|---|------|
| `commands` | `[]string` | ビルド対象のコマンドリスト |
| `privilegeRisk` | `*RiskFactor` | 権限昇格リスク（nil = 未設定） |
| `networkRisk` | `*RiskFactor` | ネットワークリスク（nil = 未設定） |
| `destructionRisk` | `*RiskFactor` | 破壊リスク（nil = 未設定） |
| `dataExfilRisk` | `*RiskFactor` | データ流出リスク（nil = 未設定） |
| `systemModRisk` | `*RiskFactor` | システム変更リスク（nil = 未設定） |
| `networkType` | `NetworkOperationType` | ネットワーク操作タイプ |
| `networkSubcommands` | `[]string` | ネットワークサブコマンド |

**設計上の注意:**
- リスクフィールドはポインタ型を使用し、未設定（nil）と明示的なUnknown（&RiskFactor{Level: Unknown}）を区別
- `Build()`時にnilのリスクフィールドは自動的にUnknownに変換

## 3. メソッド仕様

### 3.1 CommandRiskProfile.BaseRiskLevel()

全リスク要因の最大値を返す。

```go
func (p CommandRiskProfile) BaseRiskLevel() runnertypes.RiskLevel
```

**動作:**
1. `PrivilegeRisk.Level`, `NetworkRisk.Level`, `DestructionRisk.Level`, `DataExfilRisk.Level`, `SystemModRisk.Level`の最大値を計算
2. 最大値を返す

**戻り値:**
- 全リスク要因のうち最も高いリスクレベル

**計算例:**
```go
profile := CommandRiskProfile{
    NetworkRisk:     RiskFactor{Level: Medium},
    DataExfilRisk:   RiskFactor{Level: High},
    PrivilegeRisk:   RiskFactor{Level: Unknown},
    DestructionRisk: RiskFactor{Level: Unknown},
    SystemModRisk:   RiskFactor{Level: Unknown},
}
// BaseRiskLevel() returns High
```

### 3.2 CommandRiskProfile.GetRiskReasons()

リスクレベルが`Unknown`でない全要因の理由を返す。空文字列の理由は除外される。

```go
func (p CommandRiskProfile) GetRiskReasons() []string
```

**動作:**
1. 空のスライスを初期化
2. 各リスク要因について：
   - `Level > RiskLevelUnknown` かつ `Reason != ""` の場合、`Reason`を追加
3. 理由のリストを返す

**実装方針:**
- ヘルパー関数（クロージャ）を使用して重複コードを削減
- 将来的にリスク要因が増えた場合も、関数呼び出しを追加するだけで対応可能

**戻り値:**
- リスク要因の説明文字列のスライス（非空文字列のみ）
- リスク要因がない場合は空スライス（nilではない）

**順序:**
- Privilege → Network → Destruction → DataExfil → SystemMod の順

**出力例:**
```go
profile := CommandRiskProfile{
    NetworkRisk:   RiskFactor{Level: Medium, Reason: "Network access"},
    DataExfilRisk: RiskFactor{Level: High, Reason: "Data exfiltration"},
}
// GetRiskReasons() returns ["Network access", "Data exfiltration"]

// 空文字列の理由は除外される
profile2 := CommandRiskProfile{
    NetworkRisk:   RiskFactor{Level: Medium, Reason: ""},
    DataExfilRisk: RiskFactor{Level: High, Reason: "Data exfiltration"},
}
// GetRiskReasons() returns ["Data exfiltration"]
```

### 3.3 CommandRiskProfile.Validate()

プロファイルの整合性をチェック。

```go
func (p CommandRiskProfile) Validate() error
```

**バリデーションルール:**

#### Rule 1: NetworkTypeAlways requires NetworkRisk >= Medium
```go
if p.NetworkType == NetworkTypeAlways && p.NetworkRisk.Level < runnertypes.RiskLevelMedium {
    return fmt.Errorf("%w (got %v)", ErrNetworkAlwaysRequiresMediumRisk, p.NetworkRisk.Level)
}
```

**理由:** 常にネットワーク操作を行うコマンドは最低でもMediumリスク

**返却されるエラー:** `ErrNetworkAlwaysRequiresMediumRisk`

#### Rule 2: IsPrivilege requires PrivilegeRisk >= High
```go
if p.IsPrivilege && p.PrivilegeRisk.Level < runnertypes.RiskLevelHigh {
    return fmt.Errorf("%w (got %v)", ErrPrivilegeRequiresHighRisk, p.PrivilegeRisk.Level)
}
```

**理由:** 権限昇格コマンドは最低でもHighリスク

**返却されるエラー:** `ErrPrivilegeRequiresHighRisk`

#### Rule 3: NetworkSubcommands only for NetworkTypeConditional
```go
if len(p.NetworkSubcommands) > 0 && p.NetworkType != NetworkTypeConditional {
    return ErrNetworkSubcommandsOnlyForConditional
}
```

**理由:** サブコマンドベースのネットワーク判定はConditionalのみで有効

**返却されるエラー:** `ErrNetworkSubcommandsOnlyForConditional`

**戻り値:**
- バリデーション成功時: `nil`
- バリデーション失敗時: 具体的なエラー型（`ErrNetworkAlwaysRequiresMediumRisk`, `ErrPrivilegeRequiresHighRisk`, `ErrNetworkSubcommandsOnlyForConditional`）

**エラーハンドリングの例:**
```go
err := profile.Validate()
if err != nil {
    if errors.Is(err, ErrNetworkAlwaysRequiresMediumRisk) {
        // NetworkTypeAlways固有の処理
    } else if errors.Is(err, ErrPrivilegeRequiresHighRisk) {
        // IsPrivilege固有の処理
    } else if errors.Is(err, ErrNetworkSubcommandsOnlyForConditional) {
        // NetworkSubcommands固有の処理
    }
}
```

### 3.4 ProfileBuilder コンストラクタとメソッド

#### NewProfile()

新しいProfileBuilderを作成。

```go
func NewProfile(commands ...string) *ProfileBuilder
```

**引数:**
- `commands`: プロファイルを適用するコマンド名（可変長引数）

**戻り値:**
- 初期化されたProfileBuilder

**初期状態:**
- `networkType`: `NetworkTypeNone`
- 全リスクフィールド: `nil`
- `networkSubcommands`: `nil`

#### PrivilegeRisk()

権限昇格リスクを設定。

```go
func (b *ProfileBuilder) PrivilegeRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder
```

**引数:**
- `level`: リスクレベル
- `reason`: リスクの説明

**戻り値:**
- 自身のポインタ（メソッドチェーン用）

**動作:**
- `privilegeRisk`フィールドに`&RiskFactor{Level: level, Reason: reason}`を設定

#### NetworkRisk()

ネットワークリスクを設定。

```go
func (b *ProfileBuilder) NetworkRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder
```

**引数:**
- `level`: リスクレベル
- `reason`: リスクの説明

**戻り値:**
- 自身のポインタ（メソッドチェーン用）

#### DestructionRisk()

破壊リスクを設定。

```go
func (b *ProfileBuilder) DestructionRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder
```

**引数:**
- `level`: リスクレベル
- `reason`: リスクの説明

**戻り値:**
- 自身のポインタ（メソッドチェーン用）

#### DataExfilRisk()

データ流出リスクを設定。

```go
func (b *ProfileBuilder) DataExfilRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder
```

**引数:**
- `level`: リスクレベル
- `reason`: リスクの説明

**戻り値:**
- 自身のポインタ（メソッドチェーン用）

#### SystemModRisk()

システム変更リスクを設定。

```go
func (b *ProfileBuilder) SystemModRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder
```

**引数:**
- `level`: リスクレベル
- `reason`: リスクの説明

**戻り値:**
- 自身のポインタ（メソッドチェーン用）

#### AlwaysNetwork()

常にネットワーク操作を行うことを設定。

```go
func (b *ProfileBuilder) AlwaysNetwork() *ProfileBuilder
```

**戻り値:**
- 自身のポインタ（メソッドチェーン用）

**動作:**
- `networkType`を`NetworkTypeAlways`に設定

#### ConditionalNetwork()

条件付きネットワーク操作を設定。

```go
func (b *ProfileBuilder) ConditionalNetwork(subcommands ...string) *ProfileBuilder
```

**引数:**
- `subcommands`: ネットワーク操作を引き起こすサブコマンドのリスト

**戻り値:**
- 自身のポインタ（メソッドチェーン用）

**動作:**
- `networkType`を`NetworkTypeConditional`に設定
- `networkSubcommands`を`subcommands`に設定

#### Build()

CommandProfileDefを構築してバリデーション。

```go
func (b *ProfileBuilder) Build() CommandProfileDef
```

**戻り値:**
- 構築されたCommandProfileDef

**動作:**
1. `CommandRiskProfile`を構築：
   - 各リスクフィールドを`getOrDefault()`で変換（nilはUnknownに）
   - `IsPrivilege`を`privilegeRisk != nil && privilegeRisk.Level >= High`から導出
2. `profile.Validate()`を実行
3. バリデーション失敗時は`panic`
4. `CommandProfileDef{commands: b.commands, profile: profile}`を返す

**エラー処理:**
- バリデーション失敗時: `panic(fmt.Sprintf("invalid profile for commands %v: %v", b.commands, err))`

**設計上の理由:**
- `panic`を使用する理由: プログラマエラーであり、実行前に検出すべき
- プロファイル定義はソースコードにハードコードされるため、開発時に即座に問題を検出できる

#### getOrDefault()

nilのRiskFactorをUnknownに変換。

```go
func (b *ProfileBuilder) getOrDefault(risk *RiskFactor) RiskFactor
```

**引数:**
- `risk`: RiskFactorのポインタ（nilの可能性あり）

**戻り値:**
- `risk`が非nilの場合: `*risk`
- `risk`がnilの場合: `RiskFactor{Level: runnertypes.RiskLevelUnknown}`

## 4. エラー仕様

### 4.1 エラー定義

```go
var (
    // ErrNetworkAlwaysRequiresMediumRisk is returned when NetworkTypeAlways has NetworkRisk < Medium
    ErrNetworkAlwaysRequiresMediumRisk = errors.New("NetworkTypeAlways commands must have NetworkRisk >= Medium")

    // ErrPrivilegeRequiresHighRisk is returned when IsPrivilege is true but PrivilegeRisk < High
    ErrPrivilegeRequiresHighRisk = errors.New("privilege escalation commands must have PrivilegeRisk >= High")

    // ErrNetworkSubcommandsOnlyForConditional is returned when NetworkSubcommands is set for non-conditional network type
    ErrNetworkSubcommandsOnlyForConditional = errors.New("NetworkSubcommands should only be set for NetworkTypeConditional")
)
```

**エラー型の設計:**

| エラー型 | 返却条件 | 使用目的 |
|---------|---------|---------|
| `ErrNetworkAlwaysRequiresMediumRisk` | `NetworkTypeAlways` かつ `NetworkRisk < Medium` | ネットワーク操作コマンドのリスク不足を検出 |
| `ErrPrivilegeRequiresHighRisk` | `IsPrivilege` かつ `PrivilegeRisk < High` | 権限昇格コマンドのリスク不足を検出 |
| `ErrNetworkSubcommandsOnlyForConditional` | `NetworkSubcommands`が設定されているが`NetworkType != Conditional` | 設定ミスを検出 |

**使用箇所:**
- `CommandRiskProfile.Validate()`
- `ProfileBuilder.Build()` (panicメッセージに含まれる)

**ラップ例:**
```go
// Rule 1: NetworkTypeAlways requires NetworkRisk >= Medium
if p.NetworkType == NetworkTypeAlways && p.NetworkRisk.Level < runnertypes.RiskLevelMedium {
    return fmt.Errorf("%w (got %v)", ErrNetworkAlwaysRequiresMediumRisk, p.NetworkRisk.Level)
}

// Rule 2: IsPrivilege requires PrivilegeRisk >= High
if p.IsPrivilege && p.PrivilegeRisk.Level < runnertypes.RiskLevelHigh {
    return fmt.Errorf("%w (got %v)", ErrPrivilegeRequiresHighRisk, p.PrivilegeRisk.Level)
}

// Rule 3: NetworkSubcommands only for NetworkTypeConditional
if len(p.NetworkSubcommands) > 0 && p.NetworkType != NetworkTypeConditional {
    return ErrNetworkSubcommandsOnlyForConditional
}
```

**エラー判別の例:**
```go
err := profile.Validate()
if errors.Is(err, ErrNetworkAlwaysRequiresMediumRisk) {
    // NetworkTypeAlways固有の処理
    log.Printf("Network risk level is too low for always-network command")
}
```

### 4.2 エラー処理戦略

| コンポーネント | エラー処理 | 理由 |
|--------------|-----------|------|
| `ProfileBuilder.Build()` | `panic` | プログラマエラー、コンパイル後の初期化時に検出すべき |
| `CommandRiskProfile.Validate()` | `error`返却 | 将来の動的バリデーション用（現在は`Build()`内で使用） |

## 5. 使用例

### 5.1 基本的な使用例

#### 権限昇格コマンド

```go
NewProfile("sudo", "su", "doas").
    PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges, can compromise entire system").
    Build()
```

**結果:**
- `BaseRiskLevel()` → `Critical`
- `IsPrivilege` → `true`
- `GetRiskReasons()` → `["Allows execution with elevated privileges, can compromise entire system"]`

#### 常にネットワーク操作を行うコマンド

```go
NewProfile("curl", "wget").
    NetworkRisk(runnertypes.RiskLevelMedium, "Always performs network operations").
    AlwaysNetwork().
    Build()
```

**結果:**
- `BaseRiskLevel()` → `Medium`
- `NetworkType` → `NetworkTypeAlways`
- `GetRiskReasons()` → `["Always performs network operations"]`

#### 条件付きネットワーク操作コマンド

```go
NewProfile("git").
    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for clone/fetch/pull/push/remote").
    ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
    Build()
```

**結果:**
- `BaseRiskLevel()` → `Medium`
- `NetworkType` → `NetworkTypeConditional`
- `NetworkSubcommands` → `["clone", "fetch", "pull", "push", "remote"]`

#### 複数リスク要因を持つコマンド

```go
NewProfile("claude", "gemini", "chatgpt").
    NetworkRisk(runnertypes.RiskLevelHigh, "Always communicates with external AI API").
    DataExfilRisk(runnertypes.RiskLevelHigh, "May send sensitive data to external service").
    AlwaysNetwork().
    Build()
```

**結果:**
- `BaseRiskLevel()` → `High` (max of Network and DataExfil)
- `GetRiskReasons()` → `["Always communicates with external AI API", "May send sensitive data to external service"]`

### 5.2 エラー検出例

#### ケース1: NetworkTypeAlwaysなのにNetworkRisk < Medium

```go
// This will panic
NewProfile("test").
    NetworkRisk(runnertypes.RiskLevelLow, "test").
    AlwaysNetwork().
    Build()

// Panic message: "invalid profile for commands [test]: inconsistent risk profile: NetworkTypeAlways requires NetworkRisk >= Medium (got Low)"
```

#### ケース2: IsPrivilegeなのにPrivilegeRisk < High

```go
// This will panic (IsPrivilege is auto-set when PrivilegeRisk >= High)
// Actually this won't trigger since IsPrivilege is derived, but if manually set...
profile := CommandRiskProfile{
    PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelMedium},
    IsPrivilege:   true,
}
err := profile.Validate()
// err: "inconsistent risk profile: IsPrivilege requires PrivilegeRisk >= High (got Medium)"
```

#### ケース3: NetworkSubcommandsがあるのにNetworkTypeがConditionalでない

```go
// This won't happen with builder pattern, but if manually constructed...
profile := CommandRiskProfile{
    NetworkType:        NetworkTypeNone,
    NetworkSubcommands: []string{"clone"},
}
err := profile.Validate()
// err: "inconsistent risk profile: NetworkSubcommands only for NetworkTypeConditional"
```

## 6. 既存コードからの移行

### 6.1 移行マッピング

#### Before: 単純なコマンド

```go
// Old style
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

```go
// New style
NewProfile("sudo", "su", "doas").
    PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges, can compromise entire system").
    Build()
```

#### Before: ネットワークコマンド

```go
// Old style
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

```go
// New style
NewProfile("curl", "wget").
    NetworkRisk(runnertypes.RiskLevelMedium, "Always performs network operations").
    AlwaysNetwork().
    Build()
```

#### Before: 条件付きネットワーク

```go
// Old style
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

```go
// New style
NewProfile("git").
    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for clone/fetch/pull/push/remote").
    ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
    Build()
```

### 6.2 移行チェックリスト

既存のプロファイル定義を移行する際のチェックリスト：

- [ ] `BaseRiskLevel`の値を適切なリスク要因に分類
  - 権限昇格 → `PrivilegeRisk`
  - ネットワーク → `NetworkRisk`
  - 破壊的操作 → `DestructionRisk`
  - データ流出 → `DataExfilRisk`
  - システム変更 → `SystemModRisk`
- [ ] `Reason`を適切なリスク要因の説明に割り当て
- [ ] `IsPrivilege`は`PrivilegeRisk`から自動導出されることを確認
- [ ] `NetworkType`と`NetworkSubcommands`をビルダーメソッドで設定
- [ ] 移行前後で`BaseRiskLevel()`の値が一致することをテストで確認

## 7. テスト仕様

### 7.1 ユニットテストケース

#### RiskFactorのテスト

**テストケース:**
- 各リスクレベルの正しい設定と取得
- 空の`Reason`の処理

#### ProfileBuilderのテスト

**テストケース:**
1. **正常系: 各リスク要因の設定**
   - PrivilegeRiskのみ設定
   - NetworkRiskのみ設定
   - 複数リスク要因の組み合わせ

2. **正常系: ネットワーク設定**
   - AlwaysNetwork()
   - ConditionalNetwork()

3. **異常系: バリデーションエラー**
   - NetworkTypeAlways + NetworkRisk < Medium → panic
   - NetworkSubcommands + NetworkTypeNone → panic

#### CommandRiskProfileのテスト

**テストケース:**
1. **BaseRiskLevel()のテスト**
   - 単一リスク要因
   - 複数リスク要因（最大値の選択）
   - 全てUnknown

2. **GetRiskReasons()のテスト**
   - リスク要因なし → 空スライス
   - 単一リスク要因
   - 複数リスク要因
   - Unknown要因は除外される
   - 空文字列の理由は除外される

3. **Validate()のテスト**
   - 各バリデーションルールの成功/失敗

### 7.2 統合テスト

**テストケース:**
1. **既存コードとの互換性**
   - 移行前後でリスクレベルが一致
   - `AnalyzeCommandSecurity`が正しく動作

2. **全プロファイル定義のバリデーション**
   - `commandProfileDefinitions`の全要素が`Validate()`をパス
   - リスクレベルが`Unknown`より高いプロファイルは必ず理由を持つことを確認

### 7.3 テストデータ

#### 典型的なコマンド例

| カテゴリ | コマンド | 期待されるリスク構成 |
|---------|---------|-------------------|
| 権限昇格 | sudo, su, doas | PrivilegeRisk: Critical |
| 破壊的操作 | rm, dd, mkfs | DestructionRisk: High |
| ネットワーク (常時) | curl, wget, ssh | NetworkRisk: Medium, NetworkType: Always |
| ネットワーク (条件付き) | git, rsync | NetworkRisk: Medium, NetworkType: Conditional |
| AI CLI | claude, chatgpt | NetworkRisk: High, DataExfilRisk: High |
| システム変更 | systemctl, service | SystemModRisk: High |

## 8. パフォーマンス要件

### 8.1 メモリ使用量

**要件:**
- プロファイル数: 約20個（現状）
- 1プロファイルあたりの増加: 約100バイト
- 総増加量: 約2KB
- **許容範囲:** 10KB以下

### 8.2 実行時パフォーマンス

**要件:**
- `BaseRiskLevel()`の計算時間: O(1)（5つのフィールドの最大値計算）
- `GetRiskReasons()`の計算時間: O(1)（最大5要素のスライス構築）
- **許容範囲:** 既存実装と同等以上

## 9. セキュリティ要件

### 9.1 セキュリティ原則

1. **Fail-safe defaults**
   - 未設定のリスク要因は`Unknown`（最低リスク）
   - ただし、明示的な設定を推奨

2. **Complete mediation**
   - 全プロファイルに対して`Validate()`を実行
   - ビルド時とテスト時の二重チェック

3. **Separation of privilege**
   - プロファイル定義はソースコードに限定
   - ユーザーによる変更を許可しない

### 9.2 バリデーション要件

**必須バリデーション:**
- [x] NetworkTypeAlways → NetworkRisk >= Medium
- [x] IsPrivilege → PrivilegeRisk >= High
- [x] NetworkSubcommands → NetworkType == Conditional

**推奨バリデーション（将来追加）:**
- [ ] DestructionRisk >= High → 理由が明確
- [ ] DataExfilRisk >= High → 理由が明確

## 10. 互換性要件

### 10.1 後方互換性

**要件:**
- 既存の`AnalyzeCommandSecurity`インターフェースは変更しない
- 既存のリスクレベル判定ロジックは変更しない
- 既存のテストケースは全てパス

### 10.2 移行期間中の互換性

**要件:**
- 新旧両方のプロファイル定義を共存可能
- 移行完了まで既存コードは動作し続ける

## 11. ドキュメント要件

### 11.1 コードドキュメント

**必須:**
- [x] 全公開型にGoDocコメント
- [x] 各バリデーションルールにコメント
- [x] 使用例をコメントに含める

### 11.2 移行ガイド

**必須:**
- [x] 既存プロファイルから新プロファイルへの移行手順
- [x] 典型的なコマンドの移行例
- [x] トラブルシューティングガイド

## 12. 付録

### 12.1 リスク要因の分類ガイドライン

| リスク要因 | 該当する操作 | 典型的なコマンド |
|-----------|------------|----------------|
| PrivilegeRisk | 権限昇格、root権限実行 | sudo, su, doas, pkexec |
| NetworkRisk | ネットワーク通信 | curl, wget, ssh, scp, rsync, git |
| DestructionRisk | ファイル削除、ディスク操作 | rm, dd, mkfs, shred |
| DataExfilRisk | 外部サービスへのデータ送信 | AI CLI, クラウドアップロードツール |
| SystemModRisk | システム設定変更、サービス管理 | systemctl, service, apt, yum |

### 12.2 リスクレベルのガイドライン

| レベル | 説明 | 使用基準 |
|--------|------|---------|
| Unknown | リスク不明 | デフォルト、安全なコマンド |
| Low | 低リスク | 読み取り専用操作、ローカル限定 |
| Medium | 中リスク | ネットワーク操作、データ変更 |
| High | 高リスク | 破壊的操作、データ流出、システム変更 |
| Critical | 重大リスク | 権限昇格、システム全体への影響 |

### 12.3 用語集

| 用語 | 定義 |
|------|------|
| リスク要因 (Risk Factor) | コマンドのリスクを構成する個別の要素 |
| リスクプロファイル (Risk Profile) | コマンドの包括的なリスク情報 |
| ビルダーパターン | オブジェクトの構築を段階的に行う設計パターン |
| DSL (Domain-Specific Language) | 特定の目的に特化した言語 |
| Fluent Interface | メソッドチェーンによる流暢なAPI |

## 13. 実装完了記録

### 実装日: 2025-10-08

### 実装された仕様

すべての計画された仕様が実装完了：

1. **データ構造**
   - RiskFactor型（Level + Reason）
   - CommandRiskProfileNew型（5つのリスク要因）
   - CommandProfileDef型（コマンドとプロファイルの関連付け）

2. **メソッド仕様**
   - `BaseRiskLevel()`: 全リスク要因の最大値を計算
   - `GetRiskReasons()`: 非空の理由を収集
   - `IsPrivilege()`: 権限昇格判定
   - `Validate()`: 整合性検証

3. **ビルダーAPI**
   - `NewProfile()`: ビルダー初期化
   - `PrivilegeRisk()`, `NetworkRisk()`, etc.: リスク要因設定
   - `AlwaysNetwork()`, `ConditionalNetwork()`: ネットワーク設定
   - `Build()`: バリデーション付き構築

4. **バリデーションルール**
   - NetworkTypeAlways時のNetworkRiskレベル検証
   - NetworkSubcommandsの使用制限

### 実装結果

- 全ユニットテスト: パス
- 全統合テスト: パス
- Lintチェック: エラーなし
- 既存コマンド移行: 完了
- 監査ログ拡張: 完了
