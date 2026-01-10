# 詳細仕様書: テンプレートファイルのInclude機能

## 0. 既存機能活用方針

この実装では、既存のインフラを最大限活用し、最小限の変更で機能を実現する：

### 0.1 再利用する既存コンポーネント

- **Config Loader (`internal/runner/config/loader.go`)**:
  - `LoadConfig()`: 設定ファイル読み込みのエントリポイント
  - `toml.Unmarshal()` → `toml.NewDecoder().DisallowUnknownFields()` に変更

- **File System (`internal/common/filesystem.go`)**:
  - `FileSystem` インターフェース: テスト可能なファイルI/O抽象化
  - `ReadFile()`: ファイル読み込み

- **Safe File I/O (`internal/safefileio/`)**:
  - シンボリックリンク検証
  - パストラバーサル対策

- **Verification Manager (`internal/verification/manager.go`)**:
  - チェックサム検証の既存フロー
  - 複数ファイルの検証サポート（拡張）

### 0.2 新規追加するコンポーネント

- **Path Resolver**:
  - Include パスの解決（相対パス → 絶対パス）
  - セキュリティ検証

- **Template File Loader**:
  - テンプレートファイル専用の読み込みと検証
  - `DisallowUnknownFields()` による厳格なパース

- **Template Merger**:
  - 複数ソースからのテンプレート統合
  - 重複検出とエラーレポート

### 0.3 設計原則

1. **最小変更**: 既存のコードフローへの影響を最小化
2. **型安全**: Go の型システムを活用したコンパイル時検証
3. **テスタビリティ**: インターフェース駆動設計でモック可能
4. **セキュリティファースト**: 許可リスト方式、パストラバーサル対策

## 1. データ構造の詳細

### 1.1 ConfigSpec 構造体の拡張

```go
// internal/runner/runnertypes/spec.go

type ConfigSpec struct {
    // Version specifies the configuration file version (e.g., "1.0")
    Version string `toml:"version"`

    // Includes specifies template files to include.
    // Paths can be relative (to the config file directory) or absolute.
    // Each included file must contain only 'version' and 'command_templates'.
    // Multi-level includes are not allowed (template files cannot include other files).
    //
    // Example:
    //   includes = ["templates/common.toml", "../shared/backup.toml"]
    Includes []string `toml:"includes"`

    // Global contains global-level configuration
    Global GlobalSpec `toml:"global"`

    // CommandTemplates contains reusable command template definitions.
    // After loading, this will contain templates from:
    //   1. All included template files (in order)
    //   2. The main config file's command_templates section
    // Duplicate template names across sources will cause an error.
    CommandTemplates map[string]CommandTemplate `toml:"command_templates"`

    // Groups contains all command groups defined in the configuration
    Groups []GroupSpec `toml:"groups"`
}
```

**変更内容**:
- `Includes []string` フィールドを追加
- 既存フィールドは変更なし

### 1.2 TemplateFileSpec 構造体（新規）

```go
// internal/runner/config/template_loader.go

// TemplateFileSpec represents the structure of a template file.
// Template files can only contain 'version' and 'command_templates'.
// Any other fields will cause an error when parsed with DisallowUnknownFields().
type TemplateFileSpec struct {
    // Version specifies the template file version (e.g., "1.0")
    Version string `toml:"version"`

    // CommandTemplates contains template definitions
    CommandTemplates map[string]CommandTemplate `toml:"command_templates"`
}
```

**設計のポイント**:
- 許可リスト方式: 定義されたフィールドのみ許可
- `DisallowUnknownFields()` により、未定義のフィールドを自動検出

### 1.3 TemplateSource 構造体（新規）

```go
// internal/runner/config/template_merger.go

// TemplateSource represents templates loaded from a single file.
// This structure is used during the merge process to track the origin
// of each template for error reporting.
type TemplateSource struct {
    // FilePath is the absolute path to the source file
    FilePath string

    // Templates is the map of template name to template definition
    Templates map[string]CommandTemplate
}
```

**用途**:
- テンプレートマージ時に定義元ファイルを追跡
- 重複エラー時に全ての定義箇所を報告

## 2. エラー型の詳細

### 2.1 エラー型定義

```go
// internal/runner/config/errors.go

// ErrIncludedFileNotFound is returned when an included file does not exist.
type ErrIncludedFileNotFound struct {
    // IncludePath is the path as written in the includes array
    IncludePath string

    // ResolvedPath is the resolved absolute path
    ResolvedPath string

    // ReferencedFrom is the path of the config file containing the include
    ReferencedFrom string
}

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

// ErrConfigFileInvalidFormat is returned when a config file contains
// unknown fields or sections.
type ErrConfigFileInvalidFormat struct {
    // ConfigFile is the path to the config file
    ConfigFile string

    // ParseError is the original error from go-toml
    // (contains details about the unknown field)
    ParseError error
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

func (e *ErrConfigFileInvalidFormat) Unwrap() error {
    return e.ParseError
}

// ErrTemplateFileInvalidFormat is returned when a template file contains
// unknown fields or sections.
type ErrTemplateFileInvalidFormat struct {
    // TemplateFile is the path to the template file
    TemplateFile string

    // ParseError is the original error from go-toml
    ParseError error
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

func (e *ErrTemplateFileInvalidFormat) Unwrap() error {
    return e.ParseError
}

// ErrDuplicateTemplateName is returned when the same template name
// is defined in multiple locations.
type ErrDuplicateTemplateName struct {
    // Name is the duplicate template name
    Name string

    // Locations is the list of file paths where the template is defined
    Locations []string
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

### 2.2 エラー検出タイミング

| エラー型 | 検出タイミング | 検出方法 |
|---------|--------------|----------|
| `ErrIncludedFileNotFound` | パス解決時 | ファイル存在確認 |
| `ErrConfigFileInvalidFormat` | メインファイル読み込み時 | `DisallowUnknownFields()` |
| `ErrTemplateFileInvalidFormat` | テンプレートファイル読み込み時 | `DisallowUnknownFields()` |
| `ErrDuplicateTemplateName` | テンプレートマージ時 | マップキー重複チェック |

## 3. パス解決の詳細

### 3.1 PathResolver インターフェース

```go
// internal/runner/config/path_resolver.go

// PathResolver resolves include paths to absolute paths.
type PathResolver interface {
    // ResolvePath resolves an include path to an absolute path.
    //
    // Parameters:
    //   - includePath: Path as written in the includes array
    //   - baseDir: Directory containing the config file (absolute path)
    //
    // Returns:
    //   - Resolved absolute path
    //   - Error if file does not exist or path is invalid
    //
    // Security:
    //   - Validates against path traversal attacks
    //   - Checks for symlink safety using safefileio
    ResolvePath(includePath string, baseDir string) (string, error)
}

// DefaultPathResolver is the production implementation of PathResolver.
type DefaultPathResolver struct {
    fs common.FileSystem
}

// NewDefaultPathResolver creates a new DefaultPathResolver.
func NewDefaultPathResolver(fs common.FileSystem) *DefaultPathResolver {
    return &DefaultPathResolver{fs: fs}
}
```

### 3.2 パス解決アルゴリズム

```go
func (r *DefaultPathResolver) ResolvePath(includePath string, baseDir string) (string, error) {
    // Step 1: Check if path is absolute
    var candidatePath string
    if filepath.IsAbs(includePath) {
        candidatePath = includePath
    } else {
        // Step 2: Join with base directory
        candidatePath = filepath.Join(baseDir, includePath)
    }

    // Step 3: Clean the path (resolve . and ..)
    candidatePath = filepath.Clean(candidatePath)

    // Step 4: Check file existence
    exists, err := r.fs.Exists(candidatePath)
    if err != nil {
        return "", fmt.Errorf("failed to check file existence: %w", err)
    }
    if !exists {
        return "", &ErrIncludedFileNotFound{
            IncludePath:    includePath,
            ResolvedPath:   candidatePath,
            ReferencedFrom: baseDir, // Will be set by caller
        }
    }

    // Step 5: Security validation (symlink check via safefileio)
    // This will be handled by the file system abstraction
    // when actually reading the file

    // Step 6: Convert to absolute path (if not already)
    absPath, err := filepath.Abs(candidatePath)
    if err != nil {
        return "", fmt.Errorf("failed to get absolute path: %w", err)
    }

    return absPath, nil
}
```

### 3.3 セキュリティ考慮事項

1. **パストラバーサル対策**:
   - `filepath.Clean()` でパスを正規化
   - `..` による親ディレクトリアクセスは許可（相対パスの正当な用途）
   - ただし、解決後のパスが実在することを確認

2. **シンボリックリンク検証**:
   - ファイル読み込み時に `safefileio` パッケージで検証
   - 危険なシンボリックリンクをブロック

3. **絶対パス**:
   - 絶対パスも許可（明示的な指定）
   - ただしファイル存在確認は必須

## 4. テンプレートファイルローダーの詳細

### 4.1 TemplateFileLoader インターフェース

```go
// internal/runner/config/template_loader.go

// TemplateFileLoader loads and validates template files.
type TemplateFileLoader interface {
    // LoadTemplateFile loads a template file from the given path.
    //
    // Parameters:
    //   - path: Absolute path to the template file
    //
    // Returns:
    //   - Map of template name to CommandTemplate
    //   - Error if file cannot be loaded or contains invalid format
    //
    // Validation:
    //   - Uses DisallowUnknownFields() to reject unknown fields
    //   - Only 'version' and 'command_templates' are allowed
    LoadTemplateFile(path string) (map[string]CommandTemplate, error)
}

// DefaultTemplateFileLoader is the production implementation.
type DefaultTemplateFileLoader struct {
    fs common.FileSystem
}

// NewDefaultTemplateFileLoader creates a new DefaultTemplateFileLoader.
func NewDefaultTemplateFileLoader(fs common.FileSystem) *DefaultTemplateFileLoader {
    return &DefaultTemplateFileLoader{fs: fs}
}
```

### 4.2 読み込み処理の詳細

```go
func (l *DefaultTemplateFileLoader) LoadTemplateFile(path string) (map[string]CommandTemplate, error) {
    // Step 1: Read file content
    content, err := l.fs.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read template file %s: %w", path, err)
    }

    // Step 2: Create decoder with DisallowUnknownFields
    decoder := toml.NewDecoder(bytes.NewReader(content))
    decoder.DisallowUnknownFields()

    // Step 3: Parse into TemplateFileSpec
    var spec TemplateFileSpec
    if err := decoder.Decode(&spec); err != nil {
        // Check if error is due to unknown field
        return nil, &ErrTemplateFileInvalidFormat{
            TemplateFile: path,
            ParseError:   err,
        }
    }

    // Step 4: Return command_templates
    // Note: spec.CommandTemplates may be nil if not defined
    if spec.CommandTemplates == nil {
        return make(map[string]CommandTemplate), nil
    }

    return spec.CommandTemplates, nil
}
```

### 4.3 DisallowUnknownFields の動作

`toml.Decoder.DisallowUnknownFields()` を設定すると：

- 構造体に定義されていないフィールドがTOMLに存在する場合、エラーを返す
- エラーメッセージには未知のフィールド名と行番号が含まれる
- 例: `toml: line 5: unknown field 'global'`

**検出されるケース**:
- `includes` フィールド（多段include）
- `[global]` セクション
- `[[groups]]` セクション
- `[misc]` などの任意の未定義セクション
- トップレベルの未定義フィールド

## 5. テンプレートマージャーの詳細

### 5.1 TemplateMerger インターフェース

```go
// internal/runner/config/template_merger.go

// TemplateMerger merges templates from multiple sources.
type TemplateMerger interface {
    // MergeTemplates merges templates from multiple sources.
    //
    // Parameters:
    //   - sources: List of template sources (in order)
    //
    // Returns:
    //   - Merged map of template name to CommandTemplate
    //   - Error if duplicate template names are found
    //
    // Behavior:
    //   - Sources are processed in order
    //   - Duplicate names across sources cause an error
    //   - Error message includes all locations where duplicate is defined
    MergeTemplates(sources []TemplateSource) (map[string]CommandTemplate, error)
}

// DefaultTemplateMerger is the production implementation.
type DefaultTemplateMerger struct{}

// NewDefaultTemplateMerger creates a new DefaultTemplateMerger.
func NewDefaultTemplateMerger() *DefaultTemplateMerger {
    return &DefaultTemplateMerger{}
}
```

### 5.2 マージアルゴリズム

```go
func (m *DefaultTemplateMerger) MergeTemplates(sources []TemplateSource) (map[string]CommandTemplate, error) {
    // Map to store merged templates
    merged := make(map[string]CommandTemplate)

    // Map to track the file where each template is defined
    locations := make(map[string][]string)

    // Process each source in order
    for _, source := range sources {
        for name, template := range source.Templates {
            // Check for duplicates
            if _, exists := merged[name]; exists {
                // Record this location
                locations[name] = append(locations[name], source.FilePath)

                // Return error with all locations
                return nil, &ErrDuplicateTemplateName{
                    Name:      name,
                    Locations: locations[name],
                }
            }

            // Add template to merged map
            merged[name] = template

            // Record location (for potential future error)
            locations[name] = []string{source.FilePath}
        }
    }

    return merged, nil
}
```

### 5.3 マージ順序

テンプレートのマージは以下の順序で行う：

1. **Include ファイル** (includes 配列の順序)
2. **メインファイル** (config.toml の command_templates)

**例**:
```toml
# config.toml
includes = ["templates/common.toml", "templates/backup.toml"]

[command_templates.local]
cmd = "echo"
```

マージ順序:
1. `templates/common.toml` のテンプレート
2. `templates/backup.toml` のテンプレート
3. `config.toml` のテンプレート (local)

## 6. Config Loader の変更詳細

### 6.1 LoadConfig の変更

```go
// internal/runner/config/loader.go

func (l *Loader) LoadConfig(content []byte) (*runnertypes.ConfigSpec, error) {
    // Step 1: Parse main config file with DisallowUnknownFields
    decoder := toml.NewDecoder(bytes.NewReader(content))
    decoder.DisallowUnknownFields()

    var cfg runnertypes.ConfigSpec
    if err := decoder.Decode(&cfg); err != nil {
        return nil, &ErrConfigFileInvalidFormat{
            ConfigFile: "<main>", // Will be set by caller
            ParseError: err,
        }
    }

    // Step 2: Check for prohibited "name" field in command_templates
    // (This is a separate check for backward compatibility)
    if err := checkTemplateNameField(content); err != nil {
        return nil, err
    }

    // Step 3: Process includes
    if len(cfg.Includes) > 0 {
        if err := l.processIncludes(&cfg); err != nil {
            return nil, err
        }
    }

    // Step 4: Apply default values
    ApplyGlobalDefaults(&cfg.Global)

    // Step 5: Validate timeout values
    if err := ValidateTimeouts(&cfg); err != nil {
        return nil, err
    }

    // Step 6: Validate group names
    if err := ValidateGroupNames(&cfg); err != nil {
        return nil, err
    }

    // Step 7: Validate command templates
    if err := ValidateTemplates(&cfg); err != nil {
        return nil, err
    }

    // Step 8: Validate commands
    if err := ValidateCommands(&cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

### 6.2 processIncludes の実装

```go
func (l *Loader) processIncludes(cfg *runnertypes.ConfigSpec) error {
    // Initialize components
    pathResolver := NewDefaultPathResolver(l.fs)
    templateLoader := NewDefaultTemplateFileLoader(l.fs)
    templateMerger := NewDefaultTemplateMerger()

    // Determine base directory (will be set by caller)
    baseDir := getBaseDir(cfg) // Implementation detail

    // Collect all template sources
    var sources []TemplateSource

    // Step 1: Load templates from include files
    for _, includePath := range cfg.Includes {
        // Resolve path
        resolvedPath, err := pathResolver.ResolvePath(includePath, baseDir)
        if err != nil {
            // Add context to error
            if pathErr, ok := err.(*ErrIncludedFileNotFound); ok {
                pathErr.ReferencedFrom = baseDir
            }
            return err
        }

        // Load template file
        templates, err := templateLoader.LoadTemplateFile(resolvedPath)
        if err != nil {
            return err
        }

        // Add to sources
        sources = append(sources, TemplateSource{
            FilePath:  resolvedPath,
            Templates: templates,
        })
    }

    // Step 2: Add main file templates
    if cfg.CommandTemplates != nil {
        sources = append(sources, TemplateSource{
            FilePath:  baseDir, // Main config file path
            Templates: cfg.CommandTemplates,
        })
    }

    // Step 3: Merge all templates
    mergedTemplates, err := templateMerger.MergeTemplates(sources)
    if err != nil {
        return err
    }

    // Step 4: Update config with merged templates
    cfg.CommandTemplates = mergedTemplates

    return nil
}
```

### 6.3 baseDir の決定

```go
// getBaseDir extracts the base directory from config metadata.
// This will be set by the caller (cmd/runner/main.go) when loading the config.
func getBaseDir(cfg *runnertypes.ConfigSpec) string {
    // Implementation note: This will require adding metadata to ConfigSpec
    // or passing baseDir as a parameter to LoadConfig
    //
    // Option 1: Add metadata field to ConfigSpec (not persisted in TOML)
    // Option 2: Change LoadConfig signature to accept baseDir parameter
    //
    // Recommended: Option 2 for cleaner separation
    panic("not implemented - requires design decision")
}
```

**設計決定が必要**:
- `LoadConfig` の引数に `configFilePath` を追加
- または、`ConfigSpec` に内部フィールドとして `__baseDir` を追加

## 7. Verification との統合

### 7.1 Verification Manager の拡張

```go
// internal/verification/manager.go

// Manager now supports registering multiple files for verification.
type Manager struct {
    validator *filevalidator.Validator
    files     []string // List of files to verify
}

// AddFile adds a file to the verification list.
func (m *Manager) AddFile(filePath string) error {
    // Validate path
    if filePath == "" {
        return errors.New("file path cannot be empty")
    }

    // Add to list
    m.files = append(m.files, filePath)
    return nil
}

// VerifyAll verifies all registered files.
func (m *Manager) VerifyAll() error {
    for _, filePath := range m.files {
        if err := m.validator.VerifyFile(filePath); err != nil {
            return fmt.Errorf("verification failed for %s: %w", filePath, err)
        }
    }
    return nil
}
```

### 7.2 Runner での使用

```go
// cmd/runner/main.go

func run(configPath string, groupName string) error {
    // Step 1: Read config file
    content, err := os.ReadFile(configPath)
    if err != nil {
        return err
    }

    // Step 2: Load config
    loader := config.NewLoader()
    cfg, err := loader.LoadConfigWithPath(content, configPath) // New method with path
    if err != nil {
        return err
    }

    // Step 3: Setup verification
    verifier := verification.NewManager(validator)

    // Add main config file
    if err := verifier.AddFile(configPath); err != nil {
        return err
    }

    // Add include files
    baseDir := filepath.Dir(configPath)
    pathResolver := config.NewDefaultPathResolver(common.NewDefaultFileSystem())
    for _, includePath := range cfg.Includes {
        resolvedPath, err := pathResolver.ResolvePath(includePath, baseDir)
        if err != nil {
            return err
        }
        if err := verifier.AddFile(resolvedPath); err != nil {
            return err
        }
    }

    // Step 4: Verify all files
    if err := verifier.VerifyAll(); err != nil {
        return err
    }

    // Step 5: Execute commands
    // ... existing execution logic ...

    return nil
}
```

## 8. テスト戦略

### 8.1 ユニットテスト

#### 8.1.1 PathResolver のテスト

```go
// internal/runner/config/path_resolver_test.go

func TestPathResolver_ResolvePath_RelativePath(t *testing.T) {
    // Test relative path resolution
}

func TestPathResolver_ResolvePath_AbsolutePath(t *testing.T) {
    // Test absolute path handling
}

func TestPathResolver_ResolvePath_FileNotFound(t *testing.T) {
    // Test error when file doesn't exist
}

func TestPathResolver_ResolvePath_ParentDirectory(t *testing.T) {
    // Test ".." in relative paths
}
```

#### 8.1.2 TemplateFileLoader のテスト

```go
// internal/runner/config/template_loader_test.go

func TestTemplateFileLoader_LoadTemplateFile_Valid(t *testing.T) {
    // Test loading valid template file
}

func TestTemplateFileLoader_LoadTemplateFile_UnknownField(t *testing.T) {
    // Test error on unknown field (DisallowUnknownFields)
}

func TestTemplateFileLoader_LoadTemplateFile_EmptyTemplates(t *testing.T) {
    // Test file with no templates
}
```

#### 8.1.3 TemplateMerger のテスト

```go
// internal/runner/config/template_merger_test.go

func TestTemplateMerger_MergeTemplates_NoDuplicates(t *testing.T) {
    // Test successful merge
}

func TestTemplateMerger_MergeTemplates_Duplicate(t *testing.T) {
    // Test error on duplicate template names
}

func TestTemplateMerger_MergeTemplates_EmptySources(t *testing.T) {
    // Test with empty source list
}
```

### 8.2 統合テスト

```go
// internal/runner/config/include_integration_test.go

func TestIncludeFeature_SingleInclude(t *testing.T) {
    // Test single include file
}

func TestIncludeFeature_MultipleIncludes(t *testing.T) {
    // Test multiple include files
}

func TestIncludeFeature_DuplicateTemplate(t *testing.T) {
    // Test error on duplicate template across files
}

func TestIncludeFeature_UnknownFieldInConfig(t *testing.T) {
    // Test error on unknown field in main config
}

func TestIncludeFeature_UnknownFieldInTemplate(t *testing.T) {
    // Test error on unknown field in template file
}
```

### 8.3 E2Eテスト

```
test/e2e/include_feature/
├── single_include/
│   ├── config.toml
│   ├── templates/
│   │   └── common.toml
│   └── expected_output.txt
├── multiple_includes/
│   ├── config.toml
│   ├── templates/
│   │   ├── backup.toml
│   │   └── restore.toml
│   └── expected_output.txt
└── error_cases/
    ├── duplicate_template/
    ├── unknown_field_in_config/
    ├── unknown_field_in_template/
    └── file_not_found/
```

## 9. 実装の優先順位とマイルストーン

### Phase 1: 基本構造（1-2日）
- [ ] `ConfigSpec` に `Includes` フィールドを追加
- [ ] `TemplateFileSpec` 構造体を作成
- [ ] `TemplateSource` 構造体を作成
- [ ] エラー型を定義

### Phase 2: コアコンポーネント（2-3日）
- [ ] `PathResolver` の実装とテスト
- [ ] `TemplateFileLoader` の実装とテスト
- [ ] `TemplateMerger` の実装とテスト

### Phase 3: Loader 統合（2-3日）
- [ ] `LoadConfig` の変更（DisallowUnknownFields対応）
- [ ] `processIncludes` の実装
- [ ] baseDir 決定の設計と実装

### Phase 4: Verification 統合（1-2日）
- [ ] `Verification Manager` の拡張
- [ ] `runner` での include ファイル検証

### Phase 5: テストとドキュメント（2-3日）
- [ ] 統合テストの作成
- [ ] E2Eテストの作成
- [ ] ユーザードキュメントの作成
- [ ] サンプル設定ファイルの作成

## 10. パフォーマンス考慮事項

### 10.1 ファイルI/O

- Include ファイルは設定ロード時に1回のみ読み込む
- キャッシュは現時点では不要（単一設定ファイルの想定）
- 将来的に複数設定ファイルを扱う場合は検討

### 10.2 メモリ使用量

- テンプレートは全てメモリに展開される
- 想定: 数十〜数百のテンプレート（数MB程度）
- 大規模なテンプレートライブラリには適さない可能性

### 10.3 エラーレポート

- 重複検出時は全ての定義箇所を一度に報告
- ユーザーは1回の実行で全ての問題を確認可能

## 11. セキュリティ考慮事項

### 11.1 TOCTOU 対策

- ファイル内容は一度読み込んだら変更されない前提
- チェックサム検証後の内容を使用
- タイミングウィンドウは最小化

### 11.2 パストラバーサル

- `filepath.Clean()` による正規化
- 相対パスでの `..` は許可（正当な用途）
- 解決後のパスの実在確認

### 11.3 シンボリックリンク

- `safefileio` パッケージで検証
- 危険なシンボリックリンクをブロック

### 11.4 許可リスト方式

- `DisallowUnknownFields()` による厳格な検証
- 予期しないフィールドは全てエラー
- 将来の拡張にも自動的に対応

## 12. 後方互換性

### 12.1 既存設定ファイルへの影響

- `includes` フィールドは省略可能
- 省略時は空配列として扱う
- 既存の設定ファイルは変更不要

### 12.2 DisallowUnknownFields の影響

- **Breaking Change**: 既存のタイプミスや未使用フィールドがエラーになる
- 移行期間を設けるか、段階的導入を検討
- オプションで無効化できるようにするか検討

**推奨**:
- まず警告のみ（ログ出力）
- 次のメジャーバージョンでエラーに昇格

## 13. 参考資料

- go-toml ライブラリ: https://github.com/pelletier/go-toml
- `DisallowUnknownFields()` ドキュメント
- 既存の template 機能: `internal/runner/config/template_expansion.go`
- 既存の verification 機能: `internal/verification/`
