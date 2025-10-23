# 実装計画書: TOML設定フィールド名の改善

## 1. 実装の概要

### 1.1 目的

TOML設定フィールド名の改善を段階的かつ安全に実装する。

### 1.2 実装方針

- **TDD (Test-Driven Development)**: テストを先に作成し、実装を後から行う
- **段階的実装**: 小さな単位で実装し、各段階でテストとコミットを行う
- **Breaking Change管理**: CHANGELOG.md に明確に記載し、ユーザーに通知

### 1.3 実装順序

1. **Phase 1**: データ構造変更とテスト作成（TDD）
2. **Phase 2**: 変換処理とバリデーションの更新
3. **Phase 3**: サンプルファイルの更新
4. **Phase 4**: ドキュメントの更新（日本語版 → 英語版）
5. **Phase 5**: 最終確認とリリース準備

## 2. Phase 1: データ構造変更とテスト作成

### 2.1 単語帳の作成

- [x] **Task**: `docs/translation_glossary.md` の作成または更新
- **内容**:
  - 既存の単語帳がある場合は、今回のタスクで使用する用語を追加
  - 新規作成の場合は、基本的な用語リストを作成
- **主要用語**:
  - `検証` → `verification`
  - `環境変数` → `environment variable`
  - `許可` → `allowed` / `allow`
  - `インポート` → `import`
  - `リスクレベル` → `risk level`
  - `出力ファイル` → `output file`
  - `サイズ制限` → `size limit`
  - `標準パス` → `standard path`
- [x] **コミット**: 単語帳の作成または更新後にコミット

### 2.2 データ構造の変更

#### 2.2.1 GlobalSpec 構造体の更新

- [x] **File**: `internal/runner/runnertypes/spec.go`
- **変更内容**:
  ```go
  type GlobalSpec struct {
      // 変更: skip_standard_paths → verify_standard_paths (bool → *bool)
      VerifyStandardPaths *bool    `toml:"verify_standard_paths"`

      // 変更: env → env_vars
      EnvVars             []string `toml:"env_vars"`

      // 変更: env_allowlist → env_allowed
      EnvAllowed          []string `toml:"env_allowed"`

      // 変更: from_env → env_import
      EnvImport           []string `toml:"env_import"`

      // 変更: max_output_size → output_size_limit
      OutputSizeLimit     int64    `toml:"output_size_limit"`

      // その他の既存フィールド（変更なし）
  }
  ```
- **注意点**:
  - `VerifyStandardPaths` は `*bool` 型（省略と明示的 `false` を区別）
  - 既存の他のフィールドは変更しない
- [x] **コミット**: 構造体定義の変更のみ（実装前）

#### 2.2.2 GroupSpec 構造体の更新

- [x] **File**: `internal/runner/runnertypes/spec.go`
- **変更内容**:
  ```go
  type GroupSpec struct {
      Name       string   `toml:"name"`

      // 変更: env → env_vars
      EnvVars    []string `toml:"env_vars"`

      // 変更: env_allowlist → env_allowed
      EnvAllowed []string `toml:"env_allowed"`

      // 変更: from_env → env_import
      EnvImport  []string `toml:"env_import"`

      // その他の既存フィールド（変更なし）
  }
  ```
- [x] **コミット**: 構造体定義の変更のみ

#### 2.2.3 CommandSpec 構造体の更新

- [x] **File**: `internal/runner/runnertypes/spec.go`
- **変更内容**:
  ```go
  type CommandSpec struct {
      Name       string   `toml:"name"`

      // 変更: env → env_vars
      EnvVars    []string `toml:"env_vars"`

      // 変更: from_env → env_import
      EnvImport  []string `toml:"env_import"`

      // 変更: max_risk_level → risk_level
      RiskLevel  string   `toml:"risk_level"`

      // 変更: output → output_file
      OutputFile string   `toml:"output_file"`

      // その他の既存フィールド（変更なし）
  }
  ```
- [x] **コミット**: 構造体定義の変更のみ

### 2.3 テストの作成（TDD）

#### 2.3.1 GlobalSpec のテスト

- [x] **File**: `internal/runner/runnertypes/spec_test.go` (既存テストで新フィールドをカバー)
- **テストケース**:
  ```go
  func TestGlobalSpec_UnmarshalTOML_NewFieldNames(t *testing.T) {
      // verify_standard_paths = true
      // verify_standard_paths = false
      // verify_standard_paths 省略（nil → デフォルト true）
      // env_vars のパース
      // env_allowed のパース
      // env_import のパース
      // output_size_limit のパース
  }
  ```
- [x] **実行**: `go test ./internal/runner/runnertypes -v`
- **期待結果**: テストが失敗する（構造体は定義済みだが、デフォルト値適用ロジック未実装）
- [x] **コミット**: テストコードのみ（失敗状態でコミット）

#### 2.3.2 GroupSpec のテスト

- [x] **File**: `internal/runner/runnertypes/spec_test.go`
- **テストケース**:
  ```go
  func TestGroupSpec_UnmarshalTOML_NewFieldNames(t *testing.T) {
      // env_vars のパース
      // env_allowed のパース
      // env_import のパース
  }
  ```
- [x] **実行**: `go test ./internal/runner/runnertypes -v`
- [x] **コミット**: テストコードのみ

#### 2.3.3 CommandSpec のテスト

- [x] **File**: `internal/runner/runnertypes/spec_test.go`
- **テストケース**:
  ```go
  func TestCommandSpec_UnmarshalTOML_NewFieldNames(t *testing.T) {
      // env_vars のパース
      // env_import のパース
      // risk_level のパース
      // output_file のパース
  }
  ```
- [x] **実行**: `go test ./internal/runner/runnertypes -v`
- [x] **コミット**: テストコードのみ

### 2.4 デフォルト値適用ロジックの実装

#### 2.4.1 デフォルト値定数の定義

- [x] **File**: `internal/runner/config/defaults.go` (新規作成)
- **内容**:
  ```go
  package config

  const (
      DefaultVerifyStandardPaths = true
      DefaultRiskLevel          = "low"
  )
  ```
- [x] **コミット**: 定数定義のみ

#### 2.4.2 GlobalSpec デフォルト値適用

- [x] **File**: `internal/runner/config/defaults.go`
- **内容**:
  ```go
  func ApplyGlobalDefaults(spec *runnertypes.GlobalSpec) {
      if spec.VerifyStandardPaths == nil {
          defaultValue := DefaultVerifyStandardPaths
          spec.VerifyStandardPaths = &defaultValue
      }
  }
  ```
- [x] **テスト実行**: `go test ./internal/runner/config -v`
- **期待結果**: GlobalSpec のテストがパスする
- [x] **コミット**: デフォルト値適用実装

#### 2.4.3 CommandSpec デフォルト値適用

- [x] **File**: `internal/runner/config/defaults.go`
- **内容**:
  ```go
  func ApplyCommandDefaults(spec *runnertypes.CommandSpec) {
      if spec.RiskLevel == "" {
          spec.RiskLevel = DefaultRiskLevel
      }
  }
  ```
- [x] **テスト実行**: `go test ./internal/runner/config -v`
- [x] **コミット**: デフォルト値適用実装

#### 2.4.4 Config Loader への統合

- [x] **File**: `internal/runner/config/loader.go`
- **変更箇所**:
  ```go
  func Load(path string) (*runnertypes.Config, error) {
      // TOML unmarshal
      var spec runnertypes.ConfigSpec
      if err := toml.Unmarshal(data, &spec); err != nil {
          return nil, err
      }

      // デフォルト値適用（新規追加）
      ApplyGlobalDefaults(&spec.Global)
      for i := range spec.Groups {
          for j := range spec.Groups[i].Commands {
              ApplyCommandDefaults(&spec.Groups[i].Commands[j])
          }
      }

      // 以降の処理は既存のまま
  }
  ```
- [x] **テスト実行**: `go test ./internal/runner/config -v`
- [x] **コミット**: Loader への統合

### 2.5 Phase 1 完了確認

- [x] **確認項目**:
  - [x] すべての Spec 構造体が新フィールド名を使用
  - [x] すべてのテストが作成されている
  - [x] デフォルト値適用ロジックが実装されている
  - [x] `go test ./internal/runner/runnertypes -v` がパス
  - [x] `go test ./internal/runner/config -v` がパス
- [x] **Phase 1 完了コミット**: "refactor: update Spec structs with new TOML field names (Phase 1)"

## 3. Phase 2: 変換処理とバリデーションの更新

### 3.1 変換処理の更新（expansion.go）

#### 3.1.1 ExpandGlobal 関数の更新

- [x] **File**: `internal/runner/config/expansion.go`
- **変更内容**:
  ```go
  func ExpandGlobal(spec *runnertypes.GlobalSpec, ...) (*runnertypes.RuntimeGlobal, error) {
      // 変更前: spec.Env → 変更後: spec.EnvVars
      // 変更前: spec.EnvAllowlist → 変更後: spec.EnvAllowed
      // 変更前: spec.FromEnv → 変更後: spec.EnvImport
      // 変更前: spec.MaxOutputSize → 変更後: spec.OutputSizeLimit
      // 変更前: spec.SkipStandardPaths → 変更後: spec.VerifyStandardPaths
      //   注: *bool のため nil チェック必要
  }
  ```
- **テスト作成** (TDD):
  - [x] `internal/runner/config/expansion_test.go` にテスト追加
  - [x] 新フィールド名でのテストケース作成
  - [x] テスト実行（失敗を確認）
  - [x] コミット: テストのみ
- **実装**:
  - [x] フィールド参照を新フィールド名に更新
  - [x] `VerifyStandardPaths` の nil チェック処理
  - [x] テスト実行（パスを確認）
  - [x] コミット: 実装

#### 3.1.2 ExpandGroup 関数の更新

- [x] **File**: `internal/runner/config/expansion.go`
- **変更内容**:
  ```go
  func ExpandGroup(globalRuntime *runnertypes.RuntimeGlobal, spec *runnertypes.GroupSpec, ...) (*runnertypes.RuntimeGroup, error) {
      // 変更前: spec.Env → 変更後: spec.EnvVars
      // 変更前: spec.EnvAllowlist → 変更後: spec.EnvAllowed
      // 変更前: spec.FromEnv → 変更後: spec.EnvImport
  }
  ```
- [x] **テスト作成**: TDD方式でテスト先行
- [x] **実装**: フィールド参照の更新
- [x] **コミット**: テストと実装を分けてコミット

#### 3.1.3 ExpandCommand 関数の更新

- [x] **File**: `internal/runner/config/expansion.go`
- **変更内容**:
  ```go
  func ExpandCommand(groupRuntime *runnertypes.RuntimeGroup, spec *runnertypes.CommandSpec, ...) (*runnertypes.RuntimeCommand, error) {
      // 変更前: spec.Env → 変更後: spec.EnvVars
      // 変更前: spec.FromEnv → 変更後: spec.EnvImport
      // 変更前: spec.MaxRiskLevel → 変更後: spec.RiskLevel
      // 変更前: spec.Output → 変更後: spec.OutputFile
  }
  ```
- [x] **テスト作成**: TDD方式
- [x] **実装**: フィールド参照の更新
- [x] **コミット**: テストと実装を分けてコミット

### 3.2 バリデーション処理の更新

#### 3.2.1 バリデーション関数の作成（TDD）

- [x] **File**: `internal/runner/config/validator.go` (既存ファイルで実装済み)
- **テスト作成**:
  - [x] `internal/runner/config/validator_test.go` に以下のテストを追加:
    - `TestValidateEnvVars`
    - `TestValidateEnvAllowed`
    - `TestValidateEnvImport`
    - `TestValidateRiskLevel`
    - `TestValidateOutputFile`
    - `TestValidateOutputSizeLimit`
  - [x] テスト実行（失敗を確認）
  - [x] コミット: テストのみ

#### 3.2.2 バリデーション関数の実装

- [x] **実装**:
  ```go
  func ValidateEnvVars(envVars []string) error { /* ... */ }
  func ValidateEnvAllowed(envAllowed []string) error { /* ... */ }
  func ValidateEnvImport(envImport []string) error { /* ... */ }
  func ValidateRiskLevel(level string) error { /* ... */ }
  func ValidateOutputFile(outputFile string) error { /* ... */ }
  func ValidateOutputSizeLimit(limit int64) error { /* ... */ }
  ```
- [x] **テスト実行**: `go test ./internal/runner/config -v`
- [x] **コミット**: バリデーション実装

#### 3.2.3 Loader へのバリデーション統合

- [x] **File**: `internal/runner/config/loader.go`
- **変更箇所**:
  ```go
  func Load(path string) (*runnertypes.Config, error) {
      // TOML unmarshal
      // デフォルト値適用

      // バリデーション（新規追加）
      if err := ValidateGlobalSpec(&spec.Global); err != nil {
          return nil, fmt.Errorf("global validation failed: %w", err)
      }
      for i := range spec.Groups {
          if err := ValidateGroupSpec(&spec.Groups[i]); err != nil {
              return nil, fmt.Errorf("group[%d] validation failed: %w", i, err)
          }
          for j := range spec.Groups[i].Commands {
              if err := ValidateCommandSpec(&spec.Groups[i].Commands[j]); err != nil {
                  return nil, fmt.Errorf("group[%d].command[%d] validation failed: %w", i, j, err)
              }
          }
      }

      // 以降の処理は既存のまま
  }
  ```
- [x] **テスト実行**: `go test ./internal/runner/config -v`
- [x] **コミット**: バリデーション統合

### 3.3 既存テストの更新

#### 3.3.1 ユニットテストの更新

- [x] **対象ファイル**:
  - `internal/runner/config/*_test.go`
  - `internal/runner/executor/*_test.go`
  - その他 Spec 構造体を使用するテストファイル
- [x] **変更内容**:
  - テストデータの TOML フィールド名を新フィールド名に更新
  - 構造体初期化時のフィールド名を更新
- [x] **テスト実行**: `make test`
- [x] **コミット**: "test: update unit tests for new field names"

#### 3.3.2 インテグレーションテストの更新

- [x] **対象ファイル**:
  - `cmd/runner/integration_*_test.go`
  - `internal/runner/integration_*_test.go`
  - その他の統合テスト
- [x] **変更内容**: テスト用 TOML 設定を新フィールド名に更新
- [x] **テスト実行**: `make test`
- [x] **コミット**: "test: update integration tests for new field names"

#### 3.3.3 テストデータファイルの更新

- [x] **対象ファイル**:
  - `internal/runner/config/testdata/*.toml`
  - `internal/runner/bootstrap/testdata/*.toml`
- [x] **変更内容**:
  - `env` → `env_vars`
  - `env_allowlist` → `env_allowed`
  - `from_env` → `env_import`
  - `max_risk_level` → `risk_level`
  - `output` → `output_file`
  - `max_output_size` → `output_size_limit`
  - `skip_standard_paths` → `verify_standard_paths` (値の反転が必要)
- [x] **テスト実行**: `make test`
- [x] **コミット**: "test: update test data files for new field names"

### 3.4 Phase 2 完了確認

- [x] **確認項目**:
  - [x] すべての変換処理が新フィールド名を使用
  - [x] すべてのバリデーションが実装されている
  - [x] `make test` がすべてパス (52パッケージ全てパス)
  - [x] `make lint` がエラーなし
- [x] **Phase 2 完了コミット**: "refactor: update expansion and validation for new field names (Phase 2)"

## 4. Phase 3: サンプルファイルの更新

### 4.1 サンプル TOML ファイルの更新

- [x] **対象ディレクトリ**: `sample/`
- [x] **対象ファイル**: `sample/*.toml` (すべての `.toml` ファイル)
- [x] **変更内容**:
  - `skip_standard_paths` → `verify_standard_paths` (値の反転が必要)
  - `env` → `env_vars`
  - `env_allowlist` → `env_allowed`
  - `from_env` → `env_import`
  - `max_risk_level` → `risk_level`
  - `output` → `output_file`
  - `max_output_size` → `output_size_limit`
- [x] **確認**: 各サンプルファイルが正常に動作することを確認
  - `./build/runner --config sample/<file>.toml --dry-run`
- [x] **コミット**: "refactor: update sample TOML files with new field names"

### 4.2 Phase 3 完了確認

- [x] **確認項目**:
  - [x] すべてのサンプルファイルが新フィールド名を使用
  - [x] すべてのサンプルファイルが `--dry-run` で正常動作
  - [x] `make build` が成功
- [x] **Phase 3 完了**: サンプルファイル更新完了

## 5. Phase 4: ドキュメントの更新

### 5.1 日本語ドキュメントの作成・更新

#### 5.1.1 CHANGELOG.md の更新

- [x] **File**: `CHANGELOG.md`
- **追加内容** (英語):
  ```markdown
  ## [Unreleased]

  ### Breaking Changes

  #### TOML Field Renaming

  All TOML configuration field names have been updated to improve clarity and consistency.

  **Migration Required**: Existing configuration files must be manually updated.

  ##### Field Name Mapping

  | Level | Old Field Name | New Field Name | Default Value Change |
  |-------|----------------|----------------|---------------------|
  | Global | `skip_standard_paths` | `verify_standard_paths` | `false` (verify) → `true` (verify) |
  | Global | `env` | `env_vars` | - |
  | Global | `env_allowlist` | `env_allowed` | - |
  | Global | `from_env` | `env_import` | - |
  | Global | `max_output_size` | `output_size_limit` | - |
  | Group | `env` | `env_vars` | - |
  | Group | `env_allowlist` | `env_allowed` | - |
  | Group | `from_env` | `env_import` | - |
  | Command | `env` | `env_vars` | - |
  | Command | `from_env` | `env_import` | - |
  | Command | `max_risk_level` | `risk_level` | - |
  | Command | `output` | `output_file` | - |

  ##### Key Changes

  1. **Positive Naming**: `skip_standard_paths` → `verify_standard_paths`
     - Old: `skip_standard_paths = false` (default: verify standard paths)
     - New: `verify_standard_paths = true` (default: verify standard paths)
     - **Default behavior unchanged (verification continues), but field name is now clearer**

  2. **Environment Variable Prefix Unification**: All environment-related fields now use `env_` prefix
     - `env` → `env_vars`
     - `env_allowlist` → `env_allowed`
     - `from_env` → `env_import`

  3. **Natural Word Order**: `max_output_size` → `output_size_limit`

  4. **Clarity**: `output` → `output_file`, `max_risk_level` → `risk_level`

  ##### Migration Guide

  See [Migration Guide](docs/migration/toml_field_renaming.en.md) for detailed instructions.
  ```
- [x] **コミット**: "docs: add CHANGELOG entry for TOML field renaming"

#### 5.1.2 移行ガイドの作成（日本語版）

- [x] **File**: `docs/migration/toml_field_renaming.md` (新規作成)
- **内容**:
  - Breaking change の概要
  - フィールド名対応表（詳細版）
  - デフォルト値変更の影響
  - 移行手順（sed による一括置換例）
  - よくある質問（FAQ）
- [x] **コミット**: "docs: add migration guide for TOML field renaming (Japanese)"

#### 5.1.3 ユーザーガイドの更新（日本語版）

- [x] **対象ファイル**:
  - `docs/user/toml_config/01_global_level.md` (日本語版)
  - `docs/user/toml_config/02_group_level.md` (日本語版)
  - `docs/user/toml_config/03_command_level.md` (日本語版)
  - その他 TOML 設定に関するドキュメント
- [x] **変更内容**:
  - フィールド名を新フィールド名に更新
  - コード例を更新
  - デフォルト値の説明を更新
- [x] **コミット**: "docs: update user guide with new TOML field names (Japanese)"

### 5.2 英語ドキュメントの作成（翻訳）

#### 5.2.1 単語帳の確認・更新

- [x] **File**: `docs/translation_glossary.md`
- **確認項目**:
  - 今回のタスクで使用したすべての用語が単語帳に含まれているか
  - 訳語が一貫しているか
- [x] **必要に応じて更新**
- [x] **コミット**: "docs: update translation glossary"

#### 5.2.2 移行ガイドの英語版作成

- [x] **File**: `docs/migration/toml_field_renaming.en.md` (新規作成)
- **作成方法**:
  - `docs/migration/toml_field_renaming.md` (日本語版) を基に翻訳
  - 単語帳の訳語を使用
  - 章見出しと文の構成を日本語版と一致させる
  - 流暢さより正確さを優先
- [x] **レビュー**:
  - 日本語版と英語版を並べて確認
  - 削除された内容や追加された内容がないか確認
  - 章見出しが一致しているか確認
- [x] **コミット**: "docs: add migration guide for TOML field renaming (English)"

#### 5.2.3 ユーザーガイドの英語版作成

- [x] **対象ファイル**:
  - `docs/user/toml_config/01_global_level.en.md` (英語版)
  - `docs/user/toml_config/02_group_level.en.md` (英語版)
  - `docs/user/toml_config/03_command_level.en.md` (英語版)
- [x] **作成方法**:
  - 対応する日本語版ドキュメントを基に翻訳
  - 単語帳の訳語を使用
  - 構成を一致させる
- [x] **コミット**: "docs: update user guide with new TOML field names (English)"

### 5.3 README の更新

#### 5.3.1 README.md (英語版)

- [x] **File**: `README.md`
- [x] **確認・更新内容**:
  - TOML 設定例が新フィールド名を使用しているか
  - Quick Start のコード例が新フィールド名を使用しているか
- [x] **コミット**: "docs: update README.md with new TOML field names"

### 5.4 Phase 4 完了確認

- [x] **確認項目**:
  - [x] CHANGELOG.md に Breaking Change が記載されている
  - [x] 移行ガイド（日本語版・英語版）が作成されている
  - [x] ユーザーガイド（日本語版・英語版）が更新されている
  - [x] README.md が更新されている
  - [x] すべてのドキュメント内のコード例が新フィールド名を使用
  - [x] 日本語版と英語版の章見出しが一致している
- [x] **Phase 4 完了**: ドキュメント更新完了

## 6. Phase 5: 最終確認とリリース準備

### 6.1 コード品質確認

- [ ] **フォーマット確認**: `make fmt`
- [ ] **Lint チェック**: `make lint`
  - エラーがあれば修正
- [ ] **すべてのテスト実行**: `make test`
  - すべてのテストがパスすることを確認

### 6.2 統合テスト

- [ ] **ビルド確認**: `make build`
- [ ] **サンプル実行テスト**:
  ```bash
  for file in sample/*.toml; do
      echo "Testing $file"
      ./build/runner --config "$file" --dry-run
  done
  ```
- [ ] **実行結果確認**: すべてのサンプルが正常動作することを確認

### 6.3 ドキュメント最終確認

- [ ] **リンク切れチェック**: すべてのドキュメント内リンクが有効か確認
- [ ] **コード例の一貫性**: すべてのコード例が新フィールド名を使用しているか確認
- [ ] **翻訳の一貫性**:
  - 日本語版と英語版で章構成が一致しているか
  - 単語帳の訳語が一貫して使用されているか

### 6.4 Git 履歴の確認

- [ ] **コミット履歴確認**: `git log --oneline`
  - 各 Phase のコミットが適切に分かれているか
  - コミットメッセージが明確か
- [ ] **変更内容確認**: `git diff main`
  - 意図しない変更が含まれていないか

### 6.5 Phase 5 完了確認

- [ ] **最終チェックリスト**:
  - [ ] `make fmt` 成功
  - [ ] `make lint` エラーなし
  - [ ] `make test` すべてパス
  - [ ] `make build` 成功
  - [ ] すべてのサンプルが動作
  - [ ] ドキュメントが完全
  - [ ] 翻訳が一貫
  - [ ] Git 履歴がクリーン

### 6.6 最終コミットとタグ

- [ ] **最終コミット**: "refactor: complete TOML field renaming (Phase 5)"
- [ ] **タグ作成準備**: 次のリリースバージョンを決定（例: v2.0.0）
  - Breaking change のため、メジャーバージョンを上げる

## 7. リリース後の対応

### 7.1 リリースノート作成

- [ ] **File**: `docs/releases/v2.0.0.md` (または該当バージョン)
- **内容**:
  - Breaking Changes の詳細
  - 移行ガイドへのリンク
  - 主な変更点の概要

### 7.2 コミュニティへの通知

- [ ] **GitHub Release**: リリースノートを含む GitHub Release を作成
- [ ] **CHANGELOG.md**: Unreleased を正式バージョンに変更

## 8. 進捗管理

### 8.1 Phase 完了状況

- [x] Phase 1: データ構造変更とテスト作成 ✅
- [x] Phase 2: 変換処理とバリデーションの更新 ✅
  - [x] ユニットテストの更新完了
  - [x] インテグレーションテストの更新完了
  - [x] テストデータファイルの更新完了
  - [x] 全52パッケージのテストがパス
- [x] Phase 3: サンプルファイルの更新 ✅
  - [x] コメント内の古いフィールド名を新しい名前に更新
  - [x] 全サンプルファイルでdry-runテストが動作確認済み
  - [x] `make build`, `make test`, `make lint` すべて成功
- [x] Phase 4: ドキュメントの更新 ✅
- [ ] Phase 5: 最終確認とリリース準備

### 8.2 重要なマイルストーン

- [x] すべてのテストが新フィールド名でパス ✅
- [x] すべてのサンプルファイルが新フィールド名で動作 ✅
- [x] すべてのドキュメントが新フィールド名に更新 ✅
- [x] 日本語版・英語版ドキュメントが一貫 ✅
- [x] Breaking change が適切にドキュメント化 ✅

### 8.3 現在の状況

**完了したタスク:**
1. Phase 1: データ構造の変更とデフォルト値適用ロジック実装済み
2. Phase 2: 全テストファイルの更新完了
   - `cmd/runner/integration_envpriority_test.go`
   - `cmd/runner/integration_workdir_test.go`
   - `cmd/runner/integration_security_test.go`
   - `internal/runner/runner_test.go`
   - `internal/runner/runner_security_test.go`
   - `internal/runner/config/loader_e2e_test.go`
   - `internal/runner/config/testdata/*.toml` (6ファイル)

**次のステップ:**
- Phase 5: 最終確認とリリース準備

## 9. リスクと対策

### 9.1 想定されるリスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| テストの見落とし | 高 | Phase 1 で TDD を徹底し、すべてのテストを先に作成 |
| ドキュメントの不整合 | 中 | Phase 4 で日本語版→英語版の順に作成し、構成を一致 |
| 既存ユーザーへの影響 | 高 | CHANGELOG と移行ガイドを詳細に作成 |
| デフォルト値変更の影響 | 高 | ドキュメントで明確に警告し、移行手順を提供 |

### 9.2 ロールバック計画

万が一、重大な問題が発見された場合:

1. 該当コミットを revert
2. 問題を修正
3. Phase を最初からやり直し

## 10. 参考資料

- [要件定義書](01_requirements.md)
- [アーキテクチャ設計書](02_architecture.md)
- [詳細仕様書](03_specification.md)
- [Translation Guidelines](../../CLAUDE.md#translation-guidelines-japanese-to-english)
