# テンプレート定義でのグローバル変数参照 - アーキテクチャ設計書

## 1. 設計の全体像

### 1.1 設計原則

本機能の設計において、以下の原則を採用する：

1. **名前空間による明示的な分離**
   - グローバルとローカルの変数スコープを変数名の命名規則で区別
   - コンパイル時（設定読み込み時）に全ての違反を検出

2. **型安全性の向上**
   - 変数のスコープ情報を型レベルで表現
   - 不正な組み合わせをコンパイル時に防止

3. **単一責任の原則**
   - 変数名の検証、変数の解決、展開処理を明確に分離
   - 各モジュールが独立してテスト可能

4. **防御的設計**
   - 不正な状態を表現不可能にする
   - エラーは早期に、明確に

### 1.2 コンセプトモデル

```mermaid
graph TB
    subgraph "変数の名前空間"
        GV["グローバル変数<br/>Global Variables"]
        LV["ローカル変数<br/>Local Variables"]
    end

    subgraph "定義場所"
        GD["[global.vars]"]
        GRD["[groups.vars]"]
        CMD["[groups.commands.vars]"]
    end

    subgraph "参照場所"
        TPL["テンプレート定義<br/>Template Definition"]
        PRM["パラメータ<br/>params.*"]
    end

    GD -->|定義| GV
    GRD -->|定義| LV
    CMD -->|定義| LV

    GV -->|参照可能| TPL
    GV -->|参照可能| PRM
    LV -->|参照可能| PRM
    LV -.->|参照不可| TPL

    style GV fill:#e1f5ff
    style LV fill:#fff4e1
    style TPL fill:#f0f0f0
    style PRM fill:#f0f0f0
```

**設計の核心**:
- 変数名の最初の文字（大文字 vs 小文字）が名前空間を決定
- 名前空間と定義場所、参照場所の関係は静的に検証可能
- overrideという概念は存在しない（名前空間が完全に分離）

## 2. データモデル設計

### 2.1 変数スコープの型表現

従来の設計では変数はすべて `map[string]string` として扱われていたが、これではスコープ情報が失われる。新しい設計ではスコープを型で表現する。

```mermaid
classDiagram
    class VariableScope {
        <<enumeration>>
        GLOBAL
        LOCAL
    }

    class Variable {
        +string Name
        +string Value
        +VariableScope Scope
        +Validate() error
    }

    class GlobalVariable {
        +string Name
        +string Value
        +Validate() error
    }

    class LocalVariable {
        +string Name
        +string Value
        +Validate() error
    }

    Variable <|-- GlobalVariable
    Variable <|-- LocalVariable

    class VariableSet {
        +map~string,GlobalVariable~ Globals
        +map~string,LocalVariable~ Locals
        +Get(name string) (Variable, error)
        +Merge(other VariableSet) VariableSet
    }

    VariableSet o-- GlobalVariable
    VariableSet o-- LocalVariable
```

**設計のポイント**:

1. **型による区別**: `GlobalVariable` と `LocalVariable` は異なる型
2. **変数名の検証**: 各型のコンストラクタで命名規則を検証
3. **VariableSet**: グローバルとローカルを分離して保持
4. **不正な状態の排除**: 型システムにより、誤ったスコープの変数を誤った場所で使用することを防止

### 2.2 変数名の命名規則

```mermaid
flowchart TB
    Start(["変数名"]) --> Check1{"先頭2文字が<br/>'__'?"}
    Check1 -->|Yes| Reserved[予約済みエラー]
    Check1 -->|No| Check2{先頭1文字}

    Check2 -->|大文字<br/>A-Z| Global[グローバル変数]
    Check2 -->|小文字<br/>a-z| Local[ローカル変数]
    Check2 -->|アンダースコア<br/>_| Local
    Check2 -->|その他| Invalid[無効な変数名]

    Global --> ValidateBasic{"基本検証<br/>英数字+_"}
    Local --> ValidateBasic

    ValidateBasic -->|OK| Success(["検証成功"])
    ValidateBasic -->|NG| Invalid

    style Global fill:#e1f5ff
    style Local fill:#fff4e1
    style Reserved fill:#ffe1e1
    style Invalid fill:#ffe1e1
    style Success fill:#e1ffe1
```

**実装の詳細**:

- **判定ロジック**: 変数名の先頭文字が ASCII の範囲内（'A'-'Z' または 'a'-'z', '_'）であるかを確認
- **文字種制限**: 変数名全体が英数字とアンダースコア（`[a-zA-Z0-9_]+`）で構成されていることを検証
- **後方互換性**: 既存の `ValidateVariableName` を再利用し、その上にスコープ検証を追加

## 3. アーキテクチャ

### 3.1 モジュール構成

```mermaid
graph TB
    subgraph "設定読み込み層<br/>Configuration Loading Layer"
        Loader[Config Loader]
        Parser[TOML Parser]
    end

    subgraph "変数管理層<br/>Variable Management Layer"
        VarRegistry[Variable Registry]
        VarValidator[Variable Validator]
        VarResolver[Variable Resolver]
    end

    subgraph "テンプレート処理層<br/>Template Processing Layer"
        TplValidator[Template Validator]
        TplExpander[Template Expander]
        ParamExpander[Parameter Expander]
    end

    subgraph "変数展開層<br/>Variable Expansion Layer"
        VarExpander[Variable Expander]
        ExprEvaluator[Expression Evaluator]
    end

    subgraph "セキュリティ検証層<br/>Security Validation Layer"
        SecValidator[Security Validator]
        CmdValidator[Command Validator]
    end

    subgraph "データ層<br/>Data Layer"
        GlobalVars[("Global Variables")]
        LocalVars[("Local Variables")]
        Templates[("Command Templates")]
        Commands[("Commands")]
    end

    Parser --> Loader
    Loader --> VarRegistry
    Loader --> Templates

    VarRegistry --> VarValidator
    VarRegistry --> GlobalVars
    VarRegistry --> LocalVars

    Templates --> TplValidator
    TplValidator --> VarRegistry

    Commands --> TplExpander
    TplExpander --> Templates
    TplExpander --> ParamExpander

    ParamExpander --> VarResolver
    VarResolver --> VarRegistry
    VarResolver --> VarExpander

    VarExpander --> ExprEvaluator
    ExprEvaluator --> VarRegistry

    Commands --> SecValidator
    SecValidator --> CmdValidator

    style VarRegistry fill:#e1f5ff
    style TplValidator fill:#ffe1f5
    style VarExpander fill:#f5ffe1
    style SecValidator fill:#ffe1e1
```

### 3.2 処理フロー

```mermaid
sequenceDiagram
    participant User
    participant Loader as Config Loader
    participant VarReg as Variable Registry
    participant TplVal as Template Validator
    participant TplExp as Template Expander
    participant VarExp as Variable Expander
    participant SecVal as Security Validator

    User->>Loader: Load TOML

    Note over Loader: Phase 1: 変数の登録
    Loader->>VarReg: Register global.vars
    VarReg->>VarReg: Validate naming (uppercase)
    VarReg-->>Loader: OK / Error

    Loader->>VarReg: Register groups.vars
    VarReg->>VarReg: Validate naming (lowercase)
    VarReg-->>Loader: OK / Error

    Note over Loader: Phase 2: テンプレートの検証
    Loader->>TplVal: Validate templates
    TplVal->>VarReg: Check global var references
    VarReg-->>TplVal: Exists / Not found
    TplVal-->>Loader: OK / Error

    Note over Loader: Phase 3: コマンドの展開
    Loader->>TplExp: Expand command from template
    TplExp->>TplExp: Expand ${...} placeholders
    TplExp->>VarExp: Expand %{...} variables

    VarExp->>VarReg: Resolve variable
    VarReg->>VarReg: Check scope & existence
    VarReg-->>VarExp: Value / Error
    VarExp-->>TplExp: Expanded command

    TplExp-->>Loader: Expanded command

    Note over Loader: Phase 4: セキュリティ検証
    Loader->>SecVal: Validate expanded command
    SecVal->>SecVal: Check cmd_allowed, etc.
    SecVal-->>Loader: OK / Error

    Loader-->>User: Config / Error
```

**フェーズの説明**:

1. **Phase 1 - 変数の登録**: 全ての変数を命名規則に従って登録・検証
2. **Phase 2 - テンプレートの検証**: テンプレート内の変数参照が全てグローバル変数であることを確認
3. **Phase 3 - コマンドの展開**: テンプレート展開 → 変数展開の順で処理
4. **Phase 4 - セキュリティ検証**: 展開後のコマンドに対する既存のセキュリティチェック

### 3.3 変数解決の仕組み

```mermaid
flowchart TB
    Start(["変数参照 %{VarName}"]) --> CheckReserved{"予約済み?<br/>__で開始"}
    CheckReserved -->|Yes| Reserved[システム変数として解決]
    CheckReserved -->|No| CheckCase{変数名の<br/>先頭文字}

    CheckCase -->|大文字| GlobalLookup["Global変数を検索"]
    CheckCase -->|小文字/_ | LocalLookup["Local変数を検索"]

    GlobalLookup --> GlobalExists{"存在する?"}
    GlobalExists -->|Yes| GlobalValue["Global値を返す"]
    GlobalExists -->|No| GlobalError["未定義エラー"]

    LocalLookup --> LocalExists{"存在する?"}
    LocalExists -->|Yes| LocalValue["Local値を返す"]
    LocalExists -->|No| LocalError["未定義エラー"]

    GlobalValue --> Success(["解決成功"])
    LocalValue --> Success

    style GlobalLookup fill:#e1f5ff
    style LocalLookup fill:#fff4e1
    style Success fill:#e1ffe1
    style GlobalError fill:#ffe1e1
    style LocalError fill:#ffe1e1
```

**重要な特性**:

1. **決定的な解決**: 変数名を見るだけでグローバル/ローカルが判断できる
2. **override不可**: 名前空間が分離されているため、同名の変数が複数存在しない
3. **明確なエラー**: 未定義の場合、どちらの名前空間を探したかが明確
4. **非再帰的展開**: 変数の値に含まれる `%{...}` は展開されない（そのまま文字列として扱われる）

## 4. テンプレート展開の詳細設計

### 4.1 展開の2段階処理

```mermaid
flowchart LR
    subgraph "入力"
        TplDef[テンプレート定義]
        Params[パラメータ]
        GlobalVars[(グローバル変数)]
        LocalVars[(ローカル変数)]
    end

    subgraph "Stage 1: テンプレート展開"
        S1["${...} → params値"]
        S1Out["cmd = %{AwsPath}<br/>args = ['-v', '/data']"]
    end

    subgraph "Stage 2: 変数展開"
        S2["%{...} → 変数値"]
        S2Out["cmd = /usr/bin/aws<br/>args = ['-v', '/data']"]
    end

    subgraph "出力"
        Result[展開済みコマンド]
    end

    TplDef --> S1
    Params --> S1
    S1 --> S1Out

    S1Out --> S2
    GlobalVars --> S2
    LocalVars --> S2
    S2 --> S2Out

    S2Out --> Result

    style S1 fill:#ffe1f5
    style S2 fill:#f5ffe1
```

**例**:

```toml
# 入力
[global.vars]
AwsPath = "/usr/bin/aws"

[command_templates.s3_sync]
cmd = "%{AwsPath}"
args = ["${@flags}", "s3", "sync", "${src}", "${dst}"]

[[groups.commands]]
template = "s3_sync"
params.flags = ["-v"]
params.src = "/data"
params.dst = "s3://bucket"

# Stage 1: テンプレート展開（${...} を置換）
# cmd = "%{AwsPath}"
# args = ["-v", "s3", "sync", "/data", "s3://bucket"]

# Stage 2: 変数展開（%{...} を置換）
# cmd = "/usr/bin/aws"
# args = ["-v", "s3", "sync", "/data", "s3://bucket"]
```

### 4.2 テンプレート定義での変数参照の検証

```mermaid
flowchart TB
    Start(["テンプレート定義"]) --> Extract["変数参照を抽出<br/>%{...}パターン"]

    Extract --> Loop{"全ての参照を<br/>チェック"}

    Loop -->|次の参照| CheckName{"変数名が<br/>大文字始まり?"}

    CheckName -->|No| ErrLocal["エラー: ローカル変数は<br/>参照不可"]
    CheckName -->|Yes| CheckExists{"global.varsに<br/>定義済み?"}

    CheckExists -->|No| ErrUndef["エラー: 未定義の<br/>グローバル変数"]
    CheckExists -->|Yes| Loop

    Loop -->|全て完了| Success(["検証成功"])

    style Success fill:#e1ffe1
    style ErrLocal fill:#ffe1e1
    style ErrUndef fill:#ffe1e1
```

**検証のタイミング**:

1. **テンプレート定義時**: `ValidateTemplateDefinition()` 内で実行
2. **設定読み込み時**: 全てのテンプレートを検証してからコマンド展開に進む
3. **早期エラー検出**: 実行前に全ての問題を検出

## 5. エラーハンドリング設計

### 5.1 エラーの分類と対処

```mermaid
graph TB
    subgraph "命名規則違反"
        E1[グローバル変数が小文字始まり]
        E2[ローカル変数が大文字始まり]
        E3[予約済みプレフィックス使用]
    end

    subgraph "スコープ違反"
        E4[テンプレートでローカル変数参照]
        E5[未定義のグローバル変数参照]
    end

    subgraph "未定義エラー"
        E6[グローバル変数が未定義]
        E7[ローカル変数が未定義]
    end

    subgraph "対処"
        T1[設定読み込み時に拒否]
        T2[明確なエラーメッセージ]
        T3[修正方法の提示]
    end

    E1 --> T1
    E2 --> T1
    E3 --> T1
    E4 --> T1
    E5 --> T1
    E6 --> T2
    E7 --> T2

    T1 --> T3
    T2 --> T3

    style E1 fill:#ffe1e1
    style E2 fill:#ffe1e1
    style E3 fill:#ffe1e1
    style E4 fill:#ffe1e1
    style E5 fill:#ffe1e1
    style E6 fill:#ffe1e1
    style E7 fill:#ffe1e1
    style T3 fill:#e1ffe1
```

### 5.2 エラーメッセージの設計

**原則**:

1. **何が問題か**: 明確な問題の説明
2. **どこで発生したか**: ファイル、セクション、変数名
3. **なぜダメか**: ルールの簡潔な説明
4. **どう修正するか**: 具体的な修正例

**例**:

```
Error in [global.vars]: Variable "aws_path" must start with uppercase letter
  Location: config.toml, line 5
  Rule: Global variables must start with uppercase (A-Z)
  Example: "AwsPath", "AWS_PATH", "DefaultTimeout"

  Hint: Global variables can be used in template definitions.
        Use lowercase for group-specific variables instead.

Error in template "s3_sync" field "cmd": Cannot reference local variable "data_dir"
  Location: config.toml, line 23
  Rule: Templates can only reference global variables (uppercase start)

  Hint: Use a parameter instead:
    Template:  cmd = "${data_dir}"
    Command:   params.data_dir = "%{data_dir}"

Error in template "s3_sync": Global variable "AwsPath" is not defined
  Location: config.toml, line 23 (references undefined variable)
  Rule: All variables referenced in templates must be defined in [global.vars]

  Hint: Add to [global.vars]:
    AwsPath = "/path/to/aws"
```

## 6. セキュリティ設計

### 6.1 セキュリティ境界

```mermaid
graph TB
    subgraph "信頼境界 Trust Boundary"
        subgraph "設定ファイル Configuration File"
            GV[global.vars]
            TPL[Templates]
            GRP[Groups]
        end
    end

    subgraph "検証層 Validation Layer"
        V1[命名規則検証]
        V2[スコープ検証]
        V3[テンプレート検証]
        V4[セキュリティ検証]
    end

    subgraph "実行層 Execution Layer"
        CMD[実行されるコマンド]
    end

    GV --> V1
    GRP --> V1
    TPL --> V2
    TPL --> V3

    V1 --> V4
    V2 --> V4
    V3 --> V4

    V4 --> CMD

    style V1 fill:#ffe1e1
    style V2 fill:#ffe1e1
    style V3 fill:#ffe1e1
    style V4 fill:#ffe1e1
    style CMD fill:#e1ffe1
```

**セキュリティ原則**:

1. **Defense in Depth（多層防御）**:
   - 命名規則検証（第1層）
   - スコープ検証（第2層）
   - テンプレート検証（第3層）
   - 既存のセキュリティ検証（第4層）

2. **Fail-Safe Defaults**:
   - デフォルトは拒否
   - 明示的に許可されたもののみ通過

3. **Principle of Least Privilege**:
   - テンプレートはグローバル変数のみアクセス可能
   - ローカル変数へのアクセスは明示的にparamsを経由

### 6.2 攻撃シナリオと対策

| 攻撃シナリオ | 対策 |
|------------|------|
| **命名規則の悪用**: 大文字始まりのローカル変数を定義し、テンプレートに混入させる | 定義場所で命名規則を検証し、違反を拒否 |
| **スコープの混乱**: テンプレートでローカル変数を参照し、グループ固有の機密情報にアクセス | テンプレート検証時に参照を検出し、拒否 |
| **未定義変数の注入**: 存在しない変数をテンプレートで参照し、実行時エラーを引き起こす | テンプレート検証時に全ての参照を解決し、未定義を検出 |
| **予約済み変数の悪用**: `__` プレフィックスで将来の機能を先取り | 予約済みプレフィックスの定義を全て拒否 |

## 7. パフォーマンス設計

### 7.1 最適化戦略

```mermaid
graph LR
    subgraph "設定読み込み時 Load Time"
        L1[変数の検証・登録]
        L2[テンプレートの検証]
        L3[コマンドの展開]
    end

    subgraph "実行時 Runtime"
        R1[展開済みコマンド実行]
    end

    L1 --> L2
    L2 --> L3
    L3 --> R1

    style L1 fill:#fff4e1
    style L2 fill:#fff4e1
    style L3 fill:#fff4e1
    style R1 fill:#e1ffe1
```

**最適化のポイント**:

1. **事前検証**: 全ての検証を設定読み込み時に完了
2. **1回の展開**: テンプレート展開と変数展開は設定読み込み時に1回のみ
3. **キャッシュ不要**: 展開結果を保持するため、実行時の再展開は不要
4. **並列化の可能性**: 独立したテンプレートの検証は並列実行可能（将来の最適化）

### 7.2 メモリ効率

**設計方針**:

- 変数は `map[string]string` として保持（シンプル）
- スコープ情報は変数名から導出（追加メモリ不要）
- 展開済みコマンドのみを保持（中間状態は破棄）

## 8. テスト戦略

### 8.1 テストの階層

```mermaid
graph TB
    subgraph "Unit Tests"
        U1[変数名検証]
        U2[変数解決]
        U3[テンプレート展開]
        U4[変数展開]
    end

    subgraph "Integration Tests"
        I1[設定読み込み]
        I2[エンドツーエンド展開]
        I3[エラーメッセージ]
    end

    subgraph "Security Tests"
        S1[命名規則違反]
        S2[スコープ違反]
        S3[セキュリティバイパス試行]
    end

    U1 --> I1
    U2 --> I1
    U3 --> I2
    U4 --> I2

    I1 --> S1
    I2 --> S2
    I2 --> S3

    style U1 fill:#e1f5ff
    style U2 fill:#e1f5ff
    style U3 fill:#e1f5ff
    style U4 fill:#e1f5ff
    style I1 fill:#fff4e1
    style I2 fill:#fff4e1
    style I3 fill:#fff4e1
    style S1 fill:#ffe1e1
    style S2 fill:#ffe1e1
    style S3 fill:#ffe1e1
```

### 8.2 重要なテストケース

**命名規則検証**:
- ✅ 大文字始まりのグローバル変数
- ✅ 小文字始まりのローカル変数
- ✅ アンダースコア始まりのローカル変数
- ❌ 小文字始まりのグローバル変数定義
- ❌ 大文字始まりのローカル変数定義
- ❌ `__` 始まりの変数定義

**スコープ検証**:
- ✅ テンプレートでグローバル変数参照
- ✅ paramsでグローバル変数参照
- ✅ paramsでローカル変数参照
- ❌ テンプレートでローカル変数参照
- ❌ テンプレートで未定義グローバル変数参照

**展開検証**:
- ✅ グローバル変数を含むテンプレートの展開
- ✅ params内のローカル変数参照の展開
- ✅ params内のグローバル変数参照の展開
- ✅ 複雑なネスト構造の展開

## 9. 既存コードとの統合

### 9.1 影響を受けるモジュール

```mermaid
graph TB
    subgraph "新規モジュール New Modules"
        N1[variable/scope.go]
        N2[variable/validator.go]
        N3[variable/registry.go]
    end

    subgraph "修正が必要なモジュール Modified Modules"
        M1[config/loader.go]
        M2[config/template_expansion.go]
        M3[config/variable_expansion.go]
    end

    subgraph "影響を受けないモジュール Unaffected Modules"
        U1[runner/executor]
        U2[security/validator]
        U3[safefileio]
    end

    N1 --> M1
    N2 --> M1
    N3 --> M1
    N3 --> M2
    N3 --> M3

    M1 --> U1
    M2 --> U2

    style N1 fill:#e1ffe1
    style N2 fill:#e1ffe1
    style N3 fill:#e1ffe1
    style M1 fill:#fff4e1
    style M2 fill:#fff4e1
    style M3 fill:#fff4e1
```

### 9.2 インターフェース設計

**新しい抽象化**:

```go
// VariableRegistry は変数の登録と解決を管理する
type VariableRegistry interface {
    // RegisterGlobal はグローバル変数を登録する（命名規則を検証）
    RegisterGlobal(name, value string) error

    // RegisterLocal はローカル変数を登録する（命名規則を検証）
    RegisterLocal(name, value string) error

    // Resolve は変数名から値を解決する（スコープを自動判定）
    Resolve(name string) (string, error)

    // HasGlobal はグローバル変数が定義されているか確認する
    HasGlobal(name string) bool

    // HasLocal はローカル変数が定義されているか確認する
    HasLocal(name string) bool
}
```

**既存インターフェースへの影響**:

- `Config` 構造体: 変更なし（内部実装のみ変更）
- `CommandSpec` 構造体: 変更なし
- `ExpandedCommand` 構造体: 変更なし
- セキュリティ検証: 変更なし（展開後のコマンドを検証）

## 10. 段階的な実装戦略

### 10.1 実装の優先順位

```mermaid
gantt
    title 実装フェーズ
    dateFormat YYYY-MM-DD
    section Phase 1
    変数スコープの型定義       :p1a, 2024-01-01, 3d
    命名規則検証の実装         :p1b, after p1a, 3d
    ユニットテスト            :p1c, after p1b, 2d

    section Phase 2
    VariableRegistry実装      :p2a, after p1c, 4d
    Loader統合               :p2b, after p2a, 3d
    統合テスト               :p2c, after p2b, 2d

    section Phase 3
    テンプレート検証拡張       :p3a, after p2c, 4d
    変数参照の検証            :p3b, after p3a, 3d
    セキュリティテスト         :p3c, after p3b, 2d

    section Phase 4
    エラーメッセージ改善       :p4a, after p3c, 3d
    ドキュメント作成          :p4b, after p4a, 4d
    統合テスト               :p4c, after p4b, 2d
```

### 10.2 リスク軽減策

**技術的リスク**:

1. **既存機能の破壊**: 全既存テストが通過することを各フェーズで確認
2. **パフォーマンス劣化**: ベンチマークテストで性能を監視
3. **複雑性の増加**: コードレビューとペアプログラミングで品質を維持

**運用リスク**:

1. **後方互換性**: 既存設定が動作しないケースは明確なエラーメッセージで対処
2. **学習曲線**: 充実したドキュメントとサンプルで対処
3. **デバッグの困難さ**: dry-runモードでの詳細な情報表示

## 11. 将来の拡張性

### 11.1 拡張可能な設計

この設計により、将来以下の機能を追加することが容易になる:

1. **型付き変数**: 文字列以外の型（数値、真偽値、配列）のサポート
2. **変数の検証**: 値の範囲チェック、正規表現マッチング
3. **環境変数の統合**: `%{env:PATH}` のような記法
4. **計算式**: `%{timeout * 2}` のような簡単な演算
5. **条件付きテンプレート**: `${?debug_mode}` による動的な挙動変更

### 11.2 設計の柔軟性

**名前空間の拡張**:

```
現在: 大文字 = Global, 小文字 = Local
将来:
  - %{Global}    - グローバル変数
  - %{local}     - ローカル変数
  - %{env:VAR}   - 環境変数
  - %{sys:cpu}   - システム情報
  - %{__runner}  - システム予約変数
```

**型システムの拡張**:

```
現在: string のみ
将来:
  - string
  - int
  - bool
  - []string
  - map[string]string
```

## 12. まとめ

### 12.1 設計の利点

1. **型安全性**: スコープ情報を型で表現し、誤用を防止
2. **明示性**: 変数名を見るだけでスコープが判断可能
3. **保守性**: 単一責任の原則に基づく明確なモジュール分割
4. **セキュリティ**: 多層防御による堅牢な設計
5. **拡張性**: 将来の機能追加を考慮した柔軟な設計

### 12.2 トレードオフ

| 側面 | トレードオフ | 選択した理由 |
|------|------------|------------|
| 複雑性 | 型による厳密性 vs シンプルさ | 長期的な保守性を優先 |
| 後方互換性 | 既存設定の動作 vs 新機能 | 明確なマイグレーション経路を提供 |
| パフォーマンス | 検証の厳密性 vs 速度 | 設定読み込み時の1回のみなので影響は小さい |
| 学習曲線 | 機能の豊富さ vs 理解しやすさ | 命名規則という直感的なルールで対処 |

### 12.3 成功基準

以下の基準を満たすことで、設計が成功したと判断する:

- [ ] 全ての既存テストが通過する
- [ ] 新機能のテストカバレッジが90%以上
- [ ] エラーメッセージが明確で、修正方法が分かりやすい
- [ ] ドキュメントを読むだけで機能を理解できる
- [ ] パフォーマンスの劣化が5%以内
- [ ] セキュリティ検証が全て通過する
