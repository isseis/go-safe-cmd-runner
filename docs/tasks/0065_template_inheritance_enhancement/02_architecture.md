# テンプレート継承機能拡張 - アーキテクチャ設計書

## 1. システム概要

### 1.1 目的

テンプレート継承機能を拡張し、WorkDir, OutputFile, EnvImport, Vars の継承とマージをサポートする。既存のテンプレート展開アーキテクチャ（タスク 0062）に準拠し、一貫した継承モデルを実現する。

### 1.2 設計原則

1. **既存パターンとの一貫性**: Timeout, OutputSizeLimit, RiskLevel で採用されている継承パターンに準拠
2. **明示的な意図表現**: nil（未指定）と 空/ゼロ値（明示的な設定）を区別
3. **Defense in Depth**: 継承後もセキュリティ検証を実施
4. **最小変更原則**: 既存コードへの影響を最小化

## 2. 全体アーキテクチャ

### 2.1 継承フロー概要

```mermaid
graph TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph "入力"
        T[("CommandTemplate<br/>workdir, output_file<br/>env_import, vars")]
        C[("CommandSpec<br/>workdir, output_file<br/>env_import, vars")]
    end

    subgraph "継承処理"
        M["Field Merger<br/>(expansion.go)"]
    end

    subgraph "出力"
        R[("RuntimeCommand<br/>最終的なフィールド値")]
    end

    T --> M
    C --> M
    M --> R

    class T,C,R data;
    class M enhanced;
```

### 2.2 フィールド別継承モデル

```mermaid
graph TB
    classDef override fill:#ffe6e6,stroke:#d9534f,stroke-width:1px;
    classDef merge fill:#e8f5e8,stroke:#2e8b57,stroke-width:1px;
    classDef pointer fill:#fff3e6,stroke:#ff9800,stroke-width:1px;

    subgraph "オーバーライドモデル"
        W["WorkDir (*string)"]
        O["OutputFile (*string)"]
        T["Timeout (*Duration)"]
        S["OutputSizeLimit (*int64)"]
        R["RiskLevel (*RiskLevel)"]
    end

    subgraph "マージモデル"
        E["EnvImport ([]string)"]
        V["Vars (map[string]string)"]
    end

    W -.->|"コマンドの値が nil なら<br/>テンプレートを使用"| RES1["最終値"]
    O -.->|"コマンドの値が nil なら<br/>テンプレートを使用"| RES1
    E -.->|"和集合として<br/>マージ"| RES2["最終値"]
    V -.->|"マップマージ<br/>コマンド優先"| RES2

    class W,O,T,S,R pointer;
    class E,V merge;
```

**凡例（Legend）**

| モデル | 対象フィールド | セマンティクス |
|-------|-------------|--------------|
| オーバーライド | WorkDir, OutputFile, Timeout など | コマンドの値が nil ならテンプレートを使用、非 nil ならコマンドの値 |
| マージ | EnvImport, Vars | テンプレートとコマンドの値を組み合わせ |

## 3. データフロー

### 3.1 展開処理全体フロー

```mermaid
sequenceDiagram
    participant TOML as "TOML Configuration"
    participant Loader as "Config Loader"
    participant Expander as "Template Expander"
    participant Security as "Security Validator"

    TOML->>Loader: "Parse configuration"
    Loader->>Loader: "Validate template definitions"

    loop For each command with template
        Loader->>Expander: "Expand command with template"
        Expander->>Expander: "Step 1: Resolve template reference"
        Expander->>Expander: "Step 2: Apply field inheritance"
        Note right of Expander: "WorkDir, OutputFile: override model<br/>EnvImport, Vars: merge model"
        Expander->>Expander: "Step 3: Expand template parameters"
        Expander->>Expander: "Step 4: Expand %{var} variables"
        Expander-->>Loader: "Expanded command spec"
    end

    Loader->>Security: "Validate expanded commands"
    Security->>Security: "Validate WorkDir path"
    Security->>Security: "Validate OutputFile path"
    Security->>Security: "Validate EnvImport against allowlist"
```

### 3.2 フィールド継承の詳細フロー

```mermaid
flowchart TD
    classDef decision fill:#fff3cd,stroke:#856404,stroke-width:1px;
    classDef action fill:#d4edda,stroke:#155724,stroke-width:1px;

    A["開始: フィールド継承処理"]

    subgraph "WorkDir 処理"
        B{"コマンドの<br/>WorkDir == nil?"}
        C["テンプレートの<br/>WorkDir を使用"]
        D["コマンドの<br/>WorkDir を使用"]
    end

    subgraph "EnvImport 処理"
        E["テンプレートの<br/>EnvImport を取得"]
        F["コマンドの<br/>EnvImport を追加"]
        G["重複を排除して<br/>和集合を作成"]
    end

    subgraph "Vars 処理"
        H["テンプレートの<br/>Vars をコピー"]
        I["コマンドの Vars で<br/>上書き・追加"]
    end

    A --> B
    B -->|"Yes"| C
    B -->|"No"| D

    A --> E
    E --> F
    F --> G

    A --> H
    H --> I

    class B decision;
    class C,D,G,I action;
```

## 4. コンポーネント設計

### 4.1 データ構造の変更

#### 4.1.1 CommandTemplate の拡張

```mermaid
classDiagram
    class CommandTemplate {
        <<existing>>
        +Cmd string
        +Args []string
        +Env []string
        +Timeout *time.Duration
        +OutputSizeLimit *int64
        +RiskLevel *RiskLevel
        <<new>>
        +WorkDir *string
        +OutputFile *string
        +EnvImport []string
        +Vars map[string]string
    }
```

#### 4.1.2 CommandSpec の WorkDir 変更

```mermaid
classDiagram
    class CommandSpec {
        <<changed>>
        +WorkDir *string
        <<unchanged>>
        +OutputFile *string
        +EnvImport []string
        +Vars map[string]string
    }
```

### 4.2 継承ロジック

#### 4.2.1 オーバーライドモデル（WorkDir, OutputFile）

```
入力:
  template.WorkDir = "/template/path"
  command.WorkDir = nil

処理:
  if command.WorkDir == nil {
      result.WorkDir = template.WorkDir
  } else {
      result.WorkDir = command.WorkDir
  }

出力:
  result.WorkDir = "/template/path"
```

#### 4.2.2 マージモデル（EnvImport）

```
入力:
  template.EnvImport = ["VAR_A", "VAR_B"]
  command.EnvImport = ["VAR_B", "VAR_C"]

処理:
  result.EnvImport = unique(template.EnvImport + command.EnvImport)

出力:
  result.EnvImport = ["VAR_A", "VAR_B", "VAR_C"]
```

#### 4.2.3 マージモデル（Vars）

```
入力:
  template.Vars = {"key1": "template_value1", "key2": "template_value2"}
  command.Vars = {"key2": "command_value2", "key3": "command_value3"}

処理:
  result.Vars = copy(template.Vars)
  for key, value in command.Vars {
      result.Vars[key] = value  // コマンドが優先
  }

出力:
  result.Vars = {"key1": "template_value1", "key2": "command_value2", "key3": "command_value3"}
```

## 5. コンポーネント配置

### 5.1 パッケージ構成

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph "変更対象パッケージ"
        subgraph "internal/runner/runnertypes"
            A["template.go<br/>CommandTemplate 拡張"]
            B["command_spec.go<br/>WorkDir ポインタ型化"]
        end

        subgraph "internal/runner/config"
            C["expansion.go<br/>継承ロジック追加"]
            D["loader.go<br/>TOML パース対応"]
        end
    end

    subgraph "影響を受けるパッケージ"
        E["internal/runner/executor<br/>WorkDir 参照更新"]
        F["internal/runner/security<br/>検証ロジック更新"]
    end

    A --> C
    B --> C
    C --> E
    C --> F
    D --> C

    class A,B data;
    class C,D enhanced;
    class E,F process;
```

### 5.2 主要な変更ファイル

| ファイル | 変更内容 |
|---------|---------|
| `internal/runner/runnertypes/template.go` | CommandTemplate に WorkDir, OutputFile, EnvImport, Vars を追加 |
| `internal/runner/runnertypes/command_spec.go` | CommandSpec の WorkDir を *string に変更 |
| `internal/runner/config/expansion.go` | 継承・マージロジックの実装 |
| `internal/runner/config/loader.go` | TOML パース時の nil 判定対応 |

## 6. セキュリティ考慮事項

### 6.1 検証フロー

```mermaid
flowchart TD
    classDef validation fill:#fff3cd,stroke:#856404,stroke-width:1px;
    classDef security fill:#f8d7da,stroke:#721c24,stroke-width:1px;

    A["継承・マージ後の値"] --> B{"WorkDir の検証"}
    B --> C{"OutputFile の検証"}
    C --> D{"EnvImport の検証"}
    D --> E["RuntimeCommand 生成"]

    B -->|"パス検証"| B1["絶対パス確認<br/>ディレクトリ存在確認"]
    C -->|"パス検証"| C1["出力先ディレクトリ確認<br/>書き込み権限確認"]
    D -->|"allowlist 検証"| D1["env_allowlist との照合"]

    class B,C,D validation;
    class B1,C1,D1 security;
```

### 6.2 RunAsUser, RunAsGroup について

これらのフィールドは意図的にテンプレートサポート対象外としている：

- セキュリティ上重要なフィールドであり、各コマンドで明示的に指定すべき
- テンプレートでの暗黙的な設定は、セキュリティレビューを困難にする
- 誤設定のリスクが高く、意図しない権限でコマンドが実行される可能性がある

## 7. 後方互換性

### 7.1 TOML パース動作

| TOML 記述 | 解釈 | 動作 |
|----------|------|------|
| フィールド省略 | nil ポインタ | テンプレートから継承 |
| `workdir = ""` | 空文字列ポインタ | カレントディレクトリを明示 |
| `workdir = "/path"` | 値ありポインタ | 指定パスを使用 |

### 7.2 既存設定の移行

既存の設定ファイルは変更なしで動作する：

- `workdir` を省略している場合 → nil として解釈され、従来通りテンプレートから継承
- `workdir` に値を設定している場合 → 従来通りその値を使用

## 8. テスト戦略

### 8.1 単体テスト

- 各フィールドの継承動作（nil → テンプレート値）
- 各フィールドのオーバーライド動作（非 nil → コマンド値）
- EnvImport の和集合マージ
- Vars のマップマージ（コマンド優先）
- エッジケース（両方 nil、両方空、など）

### 8.2 統合テスト

- 実際の TOML ファイルを使用した継承動作
- セキュリティ検証との組み合わせ
- 変数展開との組み合わせ

## 9. 実装順序

1. **Phase 1**: データ構造の変更（CommandTemplate, CommandSpec）
2. **Phase 2**: 継承ロジックの実装（expansion.go）
3. **Phase 3**: TOML パースの対応（loader.go）
4. **Phase 4**: セキュリティ検証の更新
5. **Phase 5**: テストの追加と既存テストの修正
6. **Phase 6**: ドキュメントの更新
