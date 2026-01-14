# テンプレートファイルのInclude機能 - アーキテクチャ設計書

## 1. 設計の全体像

### 1.1 設計原則

本機能の設計において、以下の原則を採用する：

1. **既存設計の尊重**
   - 既存のテンプレート機能を最大限活用
   - 設定ファイルの読み込みフローへの最小限の変更

2. **セキュリティファースト**
   - Includeファイルもチェックサム検証対象
   - パストラバーサル攻撃の防止
   - シンボリックリンクの安全な処理

3. **明確なエラーハンドリング**
   - Include関連のエラーは詳細な情報を提供
   - ファイルパス、定義場所、エラー原因を明示

4. **段階的な機能拡張**
   - 多段includeは禁止（将来拡張の余地を残す）
   - テンプレートファイルは `command_templates` のみ（責務の明確化）

### 1.2 コンセプトモデル

```mermaid
graph TB
    subgraph "ファイル構成"
        CF["コマンド定義ファイル<br/>config.toml"]
        TF1["テンプレートファイル1<br/>templates/common.toml"]
        TF2["テンプレートファイル2<br/>templates/backup.toml"]
    end

    subgraph "Include関係"
        CF -->|includes| TF1
        CF -->|includes| TF2
        TF1 -.->|多段includeは禁止| X[❌]
        TF2 -.->|多段includeは禁止| X
    end

    subgraph "テンプレートのマージ"
        TF1 -->|command_templates| MERGE["テンプレート統合<br/>重複チェック"]
        TF2 -->|command_templates| MERGE
        CF -->|command_templates| MERGE
        MERGE --> FINAL["最終的なテンプレート集合"]
    end

    style CF fill:#e1f5ff
    style TF1 fill:#fff4e1
    style TF2 fill:#fff4e1
    style MERGE fill:#e8f5e8
    style X fill:#ffe1e1
```

**設計の核心**:
- コマンド定義ファイルが複数のテンプレートファイルを参照
- テンプレートファイルは再利用可能な `command_templates` 定義のみを含む
- すべてのテンプレートは読み込み時に統合され、重複がチェックされる
- 多段include（テンプレートファイルからのinclude）は禁止

## 2. システム構成

### 2.1 全体アーキテクチャ

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef security fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#8b0000;

    A[("コマンド定義TOML")] -->|includes配列| B["Config Loader<br/>(Include機能追加)"]
    B -->|各includeパス| C["Path Resolver"]
    C -->|解決済みパス| D["Template File Loader"]
    D -->|テンプレート内容| E["Template Validator"]
    E -->|検証済みテンプレート| F["Template Merger"]

    B -->|メインファイルの<br/>command_templates| F
    F -->|統合されたテンプレート| G["Configuration Object"]

    H[("テンプレートファイル1")] -->|読み込み| D
    I[("テンプレートファイル2")] -->|読み込み| D

    J["Checksum Verifier"] -->|検証要求| K[("ハッシュファイル")]
    B -.->|includeリスト| J
    J -->|検証結果| B

    class A,H,I,K data;
    class C,D,E process;
    class B,F enhanced;
    class J security;
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef security fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#8b0000;

    D1[("Data Source")] --> P1["Existing Component"] --> E1["Enhanced Component"] --> S1["Security Component"]
    class D1 data
    class P1 process
    class E1 enhanced
    class S1 security
```

### 2.2 コンポーネント配置

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef security fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#8b0000;

    subgraph "内部パッケージ構成"

        subgraph "internal/runner/runnertypes"
            A["spec.go<br/>ConfigSpec拡張<br/>Includesフィールド追加"]
        end

        subgraph "internal/runner/config"
            B["loader.go<br/>Include処理統合"]
            C["template_loader.go<br/>テンプレートファイル読み込み<br/>(新規)"]
            D["template_merger.go<br/>テンプレート統合・重複検出<br/>(新規)"]
            E["validation.go<br/>Include検証追加"]
        end

        subgraph "internal/common"
            F["filesystem.go<br/>ファイル読み込み<br/>(既存・再利用)"]
        end

        subgraph "internal/verification"
            G["manager.go<br/>チェックサム検証<br/>(拡張)"]
        end

        subgraph "cmd/runner"
            H["main.go<br/>Include対応の<br/>検証フロー統合"]
        end
    end

    A --> B
    B --> C
    B --> D
    B --> E
    C --> F
    B --> G
    H --> B
    H --> G

    class A data;
    class B,D,E enhanced;
    class C,F,G process;
    class H process;
```

### 2.3 データフロー

```mermaid
sequenceDiagram
    participant Main as "runner main"
    participant Loader as "Config Loader"
    participant Resolver as "Path Resolver"
    participant TLoader as "Template File Loader"
    participant Validator as "Template Validator"
    participant Merger as "Template Merger"
    participant Verifier as "Checksum Verifier"

    Main->>Loader: LoadConfig(content)
    Loader->>Loader: Parse main TOML
    Loader->>Loader: Extract includes array

    loop For each include path
        Loader->>Resolver: ResolvePath(include_path, base_dir)
        Resolver-->>Loader: Resolved absolute path
        Loader->>TLoader: LoadTemplateFile(resolved_path)
        TLoader->>TLoader: Read file content
        TLoader->>TLoader: Parse TOML
        TLoader->>Validator: ValidateTemplateFile(content)

        alt Validation failed
            Validator-->>TLoader: Error (prohibited section)
            TLoader-->>Loader: Error
            Loader-->>Main: Error
        else Validation success
            Validator-->>TLoader: OK
            TLoader-->>Loader: command_templates map
        end
    end

    Loader->>Merger: MergeTemplates(all_templates)

    alt Duplicate found
        Merger-->>Loader: Error (duplicate template name)
        Loader-->>Main: Error
    else No duplicates
        Merger-->>Loader: Merged template map
        Loader->>Loader: Continue with validation
        Loader-->>Main: ConfigSpec with all templates

        Main->>Verifier: VerifyFiles(config_file, include_files)
        Verifier->>Verifier: Verify each file checksum

        alt Verification failed
            Verifier-->>Main: Error
        else All verified
            Verifier-->>Main: OK
            Main->>Main: Execute commands
        end
    end
```

## 3. コンポーネント設計

### 3.1 データ構造の拡張

#### 3.1.1 ConfigSpec 構造体の拡張

```go
// internal/runner/runnertypes/spec.go
type ConfigSpec struct {
    Version          string                      `toml:"version"`

    // 新規フィールド: Includeするテンプレートファイルのリスト
    Includes         []string                    `toml:"includes"`

    Global           GlobalSpec                  `toml:"global"`
    CommandTemplates map[string]CommandTemplate  `toml:"command_templates"`
    Groups           []GroupSpec                 `toml:"groups"`
}
```

**設計のポイント**:
- `Includes` は文字列配列として定義
- 省略可能（デフォルトは空配列）
- パスは相対パスまたは絶対パスを受け付ける
- **重要**: コマンド定義ファイルも `DisallowUnknownFields()` でパースすることで、未定義のフィールド・セクションを検出

#### 3.1.2 テンプレートファイル専用構造体

```go
// internal/runner/config/template_loader.go
type TemplateFileSpec struct {
    Version          string                      `toml:"version"`
    CommandTemplates map[string]CommandTemplate  `toml:"command_templates"`
}
```

**設計のポイント**:
- テンプレートファイルは `version` と `command_templates` のみを許可
- `toml.Decoder.DisallowUnknownFields()` を使用して、未定義のフィールド・セクションをすべてエラーとする
- これにより `includes`, `[global]`, `[[groups]]`, `[misc]` など、定義されていないあらゆるフィールドを検出可能
- 明示的な禁止リストではなく、許可リスト方式で安全性を確保

### 3.2 パス解決

#### 3.2.1 相対パス解決の戦略

```mermaid
flowchart TB
    Start(["Include Path"]) --> Check1{"絶対パス?"}

    Check1 -->|Yes<br/>/で始まる| Absolute["絶対パスをそのまま使用"]
    Check1 -->|No| Relative["相対パス解決"]

    Relative --> Base["ベースディレクトリ取得<br/>(config.tomlの親ディレクトリ)"]
    Base --> Join["filepath.Join(base, include_path)"]
    Join --> Clean["filepath.Clean()で正規化"]

    Absolute --> Verify["存在確認"]
    Clean --> Verify

    Verify -->|存在する| Success(["解決済みパス"])
    Verify -->|存在しない| Error(["エラー"])

    style Success fill:#e1ffe1
    style Error fill:#ffe1e1
```

**実装の詳細**:
- ベースディレクトリはコマンド定義TOMLファイルの親ディレクトリ
- `filepath.Join()` でパスを結合
- `filepath.Clean()` で正規化（`.`, `..` の解決）
- パストラバーサル対策として、解決後のパスが想定範囲外でないか確認

#### 3.2.2 パス解決インターフェース

```go
// internal/runner/config/path_resolver.go
type PathResolver interface {
    // ResolvePath resolves an include path relative to the base directory.
    // Returns the absolute path or an error if the file does not exist.
    ResolvePath(includePath string, baseDir string) (string, error)
}
```

**責務**:
- 相対パスと絶対パスの両方に対応
- ファイルの存在確認
- セキュリティ検証（パストラバーサル防止）

### 3.3 テンプレートファイルの読み込みと検証

#### 3.3.1 読み込みプロセス

```mermaid
flowchart TB
    classDef success fill:#e1ffe1
    classDef error fill:#ffe1e1
    classDef process fill:#fff4e1

    Start(["Resolved File Path"]) --> Read["ファイル読み込み<br/>(FileSystem.ReadFile)"]
    Read --> CreateDecoder["TOML Decoder作成<br/>DisallowUnknownFields()設定"]

    CreateDecoder --> Parse["TOML解析<br/>(Decoder.Decode)"]

    Parse -->|失敗| CheckError{"エラー種別?"}
    CheckError -->|未知のフィールド/セクション| UnknownField(["未知のフィールド<br/>エラー"])
    CheckError -->|その他| ParseError(["パースエラー"])

    Parse -->|成功| Extract["command_templates<br/>抽出"]

    Extract --> Success(["テンプレートマップ"])

    class Success success
    class ParseError,UnknownField error
    class Read,CreateDecoder,Parse,Extract process
```

**検証項目**:
1. TOMLとしてパース可能か
2. `version` と `command_templates` 以外のフィールド・セクションが存在しないか
   - `DisallowUnknownFields()` により、`includes`, `[global]`, `[[groups]]`, `[misc]` など未定義のあらゆるフィールドを自動検出
3. 許可リスト方式により、新たな禁止項目の追加が不要

#### 3.3.2 テンプレートローダーインターフェース

```go
// internal/runner/config/template_loader.go
type TemplateFileLoader interface {
    // LoadTemplateFile loads and validates a template file.
    // Returns the command_templates map or an error.
    LoadTemplateFile(path string) (map[string]CommandTemplate, error)
}
```

**責務**:
- テンプレートファイルの読み込み
- TOML解析
- 禁止セクション・フィールドの検出
- `command_templates` の抽出

### 3.4 テンプレートのマージと重複検出

#### 3.4.1 マージ戦略

```mermaid
flowchart TB
    classDef success fill:#e1ffe1
    classDef error fill:#ffe1e1
    classDef process fill:#fff4e1

    Start(["テンプレート収集"]) --> Init["空のマップ作成<br/>merged := make(map[string]TemplateSource)"]

    Init --> Loop1["Includeファイル1<br/>のテンプレート"]
    Loop1 --> Process1["各テンプレートを処理"]

    Process1 --> Check1{"同名テンプレート<br/>存在?"}
    Check1 -->|Yes| Dup1(["重複エラー"])
    Check1 -->|No| Add1["マージマップに追加<br/>(ファイル名も記録)"]

    Add1 --> Loop2["Includeファイル2<br/>のテンプレート"]
    Loop2 --> Process2["各テンプレートを処理"]

    Process2 --> Check2{"同名テンプレート<br/>存在?"}
    Check2 -->|Yes| Dup2(["重複エラー<br/>定義箇所を列挙"])
    Check2 -->|No| Add2["マージマップに追加"]

    Add2 --> Loop3["メインファイル<br/>のテンプレート"]
    Loop3 --> Process3["各テンプレートを処理"]

    Process3 --> Check3{"同名テンプレート<br/>存在?"}
    Check3 -->|Yes| Dup3(["重複エラー<br/>定義箇所を列挙"])
    Check3 -->|No| Add3["マージマップに追加"]

    Add3 --> Success(["統合完了"])

    class Success success
    class Dup1,Dup2,Dup3 error
    class Init,Process1,Process2,Process3,Add1,Add2,Add3 process
```

**重複検出の詳細**:
- テンプレート名をキーとしたマップを使用
- 各テンプレートの定義元ファイル名を記録
- 重複発見時は、すべての定義箇所を含むエラーを返す

#### 3.4.2 マージャーインターフェース

```go
// internal/runner/config/template_merger.go
type TemplateMerger interface {
    // MergeTemplates merges templates from multiple sources.
    // Returns merged templates or an error if duplicates are found.
    MergeTemplates(sources []TemplateSource) (map[string]CommandTemplate, error)
}

type TemplateSource struct {
    FilePath  string                      // 定義元ファイルパス
    Templates map[string]CommandTemplate  // テンプレート定義
}
```

**責務**:
- 複数のテンプレートソースを統合
- 重複するテンプレート名の検出
- 重複エラー時の詳細情報提供

### 3.5 チェックサム検証の統合

#### 3.5.1 検証フロー

```mermaid
sequenceDiagram
    participant Runner as "runner main"
    participant Config as "Config Loader"
    participant VM as "Verification Manager"
    participant FV as "File Validator"
    participant FS as "File System"

    Runner->>Config: LoadConfig(main_toml_content)
    Config-->>Runner: ConfigSpec (includes含む)

    Runner->>Runner: Parse includes from ConfigSpec
    Runner->>VM: RegisterVerificationFiles()

    Note over Runner,VM: メインファイルを登録
    Runner->>VM: AddFile(main_toml_path)

    Note over Runner,VM: Includeファイルを登録
    loop For each include
        Runner->>Runner: ResolvePath(include)
        Runner->>VM: AddFile(resolved_include_path)
    end

    Runner->>VM: VerifyAll()

    loop For each registered file
        VM->>FV: VerifyFileHash(file_path)
        FV->>FS: ReadFile(hash_file)
        FS-->>FV: Expected hash
        FV->>FS: ReadFile(target_file)
        FS-->>FV: Actual content
        FV->>FV: Calculate hash

        alt Hash mismatch
            FV-->>VM: Error
            VM-->>Runner: Verification failed
        else Hash match
            FV-->>VM: OK
        end
    end

    VM-->>Runner: All files verified
    Runner->>Runner: Execute commands
```

**検証のポイント**:
1. メインのコマンド定義ファイルを検証リストに追加
2. `includes` 配列の各ファイル（解決済みパス）を検証リストに追加
3. すべてのファイルのチェックサムを検証
4. 1つでも失敗したら実行を中止

#### 3.5.2 Verification Managerの拡張

```go
// internal/verification/manager.go
type Manager interface {
    // AddFile adds a file to the verification list
    AddFile(filePath string) error

    // VerifyAll verifies all registered files
    VerifyAll() error
}
```

**既存実装への影響**:
- 基本的な検証ロジックは既存のまま
- 複数ファイルの登録・検証に対応（既存機能の拡張）

## 4. エラーハンドリング設計

### 4.1 エラー型の定義

```go
// internal/runner/config/errors.go

// ErrIncludedFileNotFound is returned when an included file does not exist
type ErrIncludedFileNotFound struct {
    IncludePath    string // includes配列に記述されたパス
    ResolvedPath   string // 解決後の絶対パス
    ReferencedFrom string // includeを記述したファイル
}

// ErrConfigFileInvalidFormat is returned when a config file contains
// fields or sections other than version, includes, global, command_templates, and groups
type ErrConfigFileInvalidFormat struct {
    ConfigFile string // コマンド定義ファイルのパス
    ParseError error  // go-tomlからの元のエラー（未知のフィールド情報を含む）
}

// ErrTemplateFileInvalidFormat is returned when a template file contains
// fields or sections other than version and command_templates
type ErrTemplateFileInvalidFormat struct {
    TemplateFile string // テンプレートファイルのパス
    ParseError   error  // go-tomlからの元のエラー（未知のフィールド情報を含む）
}

// ErrDuplicateTemplateName is returned when the same template name
// is defined in multiple locations
type ErrDuplicateTemplateName struct {
    Name      string   // 重複しているテンプレート名
    Locations []string // 定義されているファイルパスの配列
}
```

**エラー型の設計変更**:
- `ErrMultiLevelInclude` と `ErrTemplateFileContainsProhibitedSection` を統合
- コマンド定義ファイル用に `ErrConfigFileInvalidFormat` を追加
- テンプレートファイル用に `ErrTemplateFileInvalidFormat` を使用
- `DisallowUnknownFields()` によって自動検出されるため、個別のチェックロジックが不要

### 4.2 エラーメッセージの設計

各エラー型は、ユーザーが問題を特定し修正できる詳細情報を提供する：

```go
func (e *ErrIncludedFileNotFound) Error() string {
    return fmt.Sprintf(
        "included file not found: %q\n"+
        "  Include path: %s (as written)\n"+
        "  Resolved path: %s\n"+
        "  Referenced from: %s",
        e.IncludePath,
        e.IncludePath,
        e.ResolvedPath,
        e.ReferencedFrom,
    )
}

func (e *ErrConfigFileInvalidFormat) Error() string {
    return fmt.Sprintf(
        "config file contains invalid fields or sections: %s\n"+
        "  File: %s\n"+
        "  Config files can only contain 'version', 'includes', 'global', 'command_templates', and 'groups'\n"+
        "  Detail: %v",
        e.ConfigFile,
        e.ConfigFile,
        e.ParseError,
    )
}

func (e *ErrTemplateFileInvalidFormat) Error() string {
    return fmt.Sprintf(
        "template file contains invalid fields or sections: %s\n"+
        "  File: %s\n"+
        "  Template files can only contain 'version' and 'command_templates'\n"+
        "  Detail: %v",
        e.TemplateFile,
        e.TemplateFile,
        e.ParseError,
    )
}

func (e *ErrDuplicateTemplateName) Error() string {
    locations := strings.Join(e.Locations, "\n    - ")
    return fmt.Sprintf(
        "duplicate command template name %q\n"+
        "  Defined in:\n    - %s",
        e.Name,
        locations,
    )
}
```

## 5. セキュリティ考慮事項

### 5.1 パストラバーサル対策

```mermaid
flowchart TB
    classDef safe fill:#e1ffe1
    classDef unsafe fill:#ffe1e1
    classDef check fill:#fff4e1

    Start(["Include Path"]) --> Clean["filepath.Clean()で正規化"]
    Clean --> Join["ベースディレクトリと結合"]
    Join --> Abs["絶対パスに変換"]
    Abs --> Check{"解決後のパスが<br/>予期した範囲内?"}

    Check -->|Yes| Exist{"ファイル存在?"}
    Check -->|No| Danger(["セキュリティエラー<br/>パストラバーサル検出"])

    Exist -->|Yes| Symlink["シンボリックリンク<br/>チェック (safefileio)"]
    Exist -->|No| NotFound(["ファイル未発見エラー"])

    Symlink -->|安全| Safe(["安全な解決済みパス"])
    Symlink -->|危険| SymDanger(["セキュリティエラー<br/>危険なシンボリックリンク"])

    class Safe safe
    class Danger,NotFound,SymDanger unsafe
    class Clean,Join,Abs,Check,Exist,Symlink check
```

**実装方針**:
1. `filepath.Clean()` でパスを正規化
2. ベースディレクトリを基準に絶対パスを構築
3. 解決後のパスが意図した範囲内か確認
4. シンボリックリンクは `safefileio` パッケージで安全に処理

### 5.2 チェックサム検証の強制

```mermaid
flowchart TB
    classDef verified fill:#e1ffe1
    classDef unverified fill:#ffe1e1
    classDef process fill:#fff4e1

    Start(["runner実行"]) --> LoadConfig["設定ファイル読み込み"]
    LoadConfig --> Extract["includes配列抽出"]
    Extract --> Register["検証ファイル登録"]

    Register --> RegMain["メインファイル登録"]
    RegMain --> RegIncludes["Includeファイル全て登録"]

    RegIncludes --> VerifyLoop["各ファイルの検証"]

    VerifyLoop --> Check1{"チェックサム<br/>記録済み?"}
    Check1 -->|No| NoHash(["エラー：<br/>ハッシュ未記録"])
    Check1 -->|Yes| Check2{"チェックサム<br/>一致?"}

    Check2 -->|No| Mismatch(["エラー：<br/>ファイル改ざん検出"])
    Check2 -->|Yes| NextFile{"次のファイル?"}

    NextFile -->|あり| VerifyLoop
    NextFile -->|なし| AllVerified(["全ファイル検証完了"])

    AllVerified --> Execute["コマンド実行"]

    class AllVerified,Execute verified
    class NoHash,Mismatch unverified
    class LoadConfig,Extract,Register,VerifyLoop process
```

**セキュリティポリシー**:
- Includeファイルのチェックサムが記録されていない場合は実行拒否
- 1つでもチェックサム検証に失敗したら実行拒否
- ユーザーは事前に全ファイルに対して `record` コマンドを実行する必要がある

## 6. 処理フロー詳細

### 6.1 設定ファイル読み込みフロー

```mermaid
flowchart TB
    classDef success fill:#e1ffe1
    classDef error fill:#ffe1e1
    classDef process fill:#fff4e1

    Start(["LoadConfig(content)"]) --> CreateDecoder["TOML Decoder作成<br/>DisallowUnknownFields()設定"]
    CreateDecoder --> Parse1["メインTOMLをパース<br/>(Decoder.Decode)"]

    Parse1 -->|エラー| CheckMainError{"エラー種別?"}
    CheckMainError -->|未知のフィールド| MainUnknownField(["コマンド定義ファイル<br/>未知のフィールドエラー"])
    CheckMainError -->|その他| MainParseError(["パースエラー"])

    Parse1 -->|成功| Extract["includes配列を抽出"]

    Extract --> InitMerge["テンプレート統合の準備"]

    InitMerge --> LoopIncludes["Includeファイル処理ループ"]
    LoopIncludes --> ResolvePath["パス解決"]

    ResolvePath -->|エラー| PathError(["パス解決エラー"])
    ResolvePath -->|成功| LoadTemplate["テンプレートファイル読み込み<br/>(DisallowUnknownFields)"]

    LoadTemplate -->|エラー| LoadError(["読み込みエラー"])
    LoadTemplate -->|成功| ValidateTemplate["テンプレートファイル検証"]

    ValidateTemplate -->|エラー| ValidateError(["検証エラー"])
    ValidateTemplate -->|成功| CollectTemplates["テンプレート収集"]

    CollectTemplates --> MoreIncludes{"次のInclude<br/>ファイル?"}
    MoreIncludes -->|あり| LoopIncludes
    MoreIncludes -->|なし| AddMain["メインファイルの<br/>command_templates追加"]

    AddMain --> Merge["全テンプレートのマージ"]
    Merge -->|重複エラー| DupError(["重複エラー"])
    Merge -->|成功| ValidateAll["全体検証"]

    ValidateAll -->|エラー| ValidationError(["検証エラー"])
    ValidateAll -->|成功| Success(["ConfigSpec完成"])

    class Success success
    class MainUnknownField,MainParseError,PathError,LoadError,ValidateError,DupError,ValidationError error
    class CreateDecoder,Parse1,Extract,InitMerge,LoopIncludes,ResolvePath,LoadTemplate,ValidateTemplate,CollectTemplates,AddMain,Merge,ValidateAll process
```

### 6.2 実行時検証フロー

```mermaid
flowchart TB
    classDef success fill:#e1ffe1
    classDef error fill:#ffe1e1
    classDef process fill:#fff4e1

    Start(["runner起動"]) --> ReadMainFile["メインTOMLファイル読み込み"]
    ReadMainFile --> LoadConfig["LoadConfig()"]

    LoadConfig -->|エラー| ConfigError(["設定エラー"])
    LoadConfig -->|成功| ExtractIncludes["ConfigSpec.Includes取得"]

    ExtractIncludes --> CreateVerifier["Verification Manager作成"]
    CreateVerifier --> AddMain["メインファイルを<br/>検証リストに追加"]

    AddMain --> LoopAddIncludes["Includeファイル追加ループ"]
    LoopAddIncludes --> ResolveInclude["Includeパス解決"]
    ResolveInclude --> AddInclude["検証リストに追加"]

    AddInclude --> MoreIncludes{"次のInclude?"}
    MoreIncludes -->|あり| LoopAddIncludes
    MoreIncludes -->|なし| VerifyAll["全ファイル検証実行"]

    VerifyAll --> LoopVerify["各ファイル検証ループ"]
    LoopVerify --> CheckHash{"チェックサム一致?"}

    CheckHash -->|不一致| VerifyError(["検証失敗"])
    CheckHash -->|一致| MoreFiles{"次のファイル?"}

    MoreFiles -->|あり| LoopVerify
    MoreFiles -->|なし| AllVerified["全ファイル検証完了"]

    AllVerified --> Execute["コマンド実行"]
    Execute --> Success(["正常終了"])

    class Success,AllVerified success
    class ConfigError,VerifyError error
    class ReadMainFile,LoadConfig,ExtractIncludes,CreateVerifier,AddMain,LoopAddIncludes,ResolveInclude,AddInclude,VerifyAll,LoopVerify,Execute process
```

## 7. テスト戦略

### 7.1 ユニットテスト

各コンポーネントを独立してテスト：

1. **Path Resolver**
   - 相対パス解決の正確性
   - 絶対パスの処理
   - パストラバーサル検出
   - ファイル未発見の処理

2. **Config File Loader**
   - コマンド定義ファイルの読み込み（`DisallowUnknownFields()`使用）
   - 未定義のフィールド・セクション検出
   - パースエラー処理

3. **Template File Loader**
   - 正常なテンプレートファイルの読み込み（`DisallowUnknownFields()`使用）
   - 未定義のフィールド・セクション検出
   - パースエラー処理

4. **Template Merger**
   - 複数ソースからのマージ
   - 重複検出
   - 空のテンプレート処理
   - エラーメッセージの正確性

### 7.2 統合テスト

実際のファイル構成でエンドツーエンドをテスト：

```
testdata/
├── valid_single_include/
│   ├── config.toml          # 単一includeの正常系
│   └── templates/
│       └── common.toml
├── valid_multiple_includes/
│   ├── config.toml          # 複数includeの正常系
│   └── templates/
│       ├── backup.toml
│       └── restore.toml
├── duplicate_template/
│   ├── config.toml          # 重複エラーケース
│   └── templates/
│       ├── temp1.toml       # backup テンプレート定義
│       └── temp2.toml       # backup テンプレート定義（重複）
├── multi_level_include/
│   ├── config.toml
│   └── templates/
│       └── invalid.toml     # includes含む（エラー）
├── prohibited_section_in_template/
│   ├── config.toml
│   └── templates/
│       └── invalid.toml     # [global]含む（エラー）
├── unknown_field_in_config/
│   └── config.toml          # [misc]など未定義セクション含む（エラー）
└── relative_path_test/
    ├── configs/
    │   └── app.toml         # ../templates/common.toml を参照
    └── templates/
        └── common.toml
```

### 7.3 セキュリティテスト

1. **パストラバーサル攻撃**
   - `../../etc/passwd` のような危険なパス
   - シンボリックリンクによる攻撃

2. **チェックサム検証バイパス**
   - ハッシュファイルがない状態での実行試行
   - Includeファイルのみハッシュが欠けている状態

## 8. 実装の優先順位

### フェーズ1: 基本機能

1. `ConfigSpec` への `Includes` フィールド追加
2. Path Resolver の実装
3. Template File Loader の実装（基本的な読み込みと検証）
4. Template Merger の実装（重複検出）

### フェーズ2: 検証機能

1. エラー型の定義と実装
2. テンプレートファイルの禁止セクション検出
3. 多段include検出
4. 詳細なエラーメッセージ

### フェーズ3: セキュリティ統合

1. Verification Manager との統合
2. チェックサム検証フローへの組み込み
3. パストラバーサル対策の実装
4. シンボリックリンク安全性チェック

### フェーズ4: テストとドキュメント

1. ユニットテスト作成
2. 統合テスト作成
3. セキュリティテスト作成
4. ユーザードキュメント作成

## 9. 将来の拡張性

現在の設計は将来的な機能拡張を考慮している：

### 9.1 オプショナルInclude

```toml
# 将来の拡張案
includes = [
    {path = "templates/common.toml"},
    {path = "templates/optional.toml", optional = true}
]
```

現在の `[]string` から `[]IncludeSpec` への変更で対応可能。

### 9.2 Include時の名前空間プレフィックス

```toml
# 将来の拡張案
includes = [
    {path = "templates/common.toml", prefix = "common_"}
]
# → common_backup, common_restore としてインポート
```

Template Merger での処理追加で対応可能。

### 9.3 条件付きInclude

```toml
# 将来の拡張案
includes = [
    {path = "templates/linux.toml", os = "linux"},
    {path = "templates/darwin.toml", os = "darwin"}
]
```

Path Resolver での条件評価追加で対応可能。

## 10. 参考資料

- 既存のテンプレート機能: `internal/runner/config/template_expansion.go`
- 設定ファイルローダー: `internal/runner/config/loader.go`
- ファイル検証: `internal/verification/manager.go`
- 安全なファイルI/O: `internal/safefileio/`
- テンプレート検証: `internal/runner/config/validation.go`
