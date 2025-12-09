# コマンドテンプレート機能 - 実装計画書

## 1. 実装概要

### 1.1 目的

詳細仕様書 (03_detailed_spec.md) で定義されたコマンドテンプレート機能を、段階的に実装する。

### 1.2 実装原則

1. **段階的実装**: 小さく動作するユニットを積み上げる
2. **テスト駆動**: 各ステップでユニットテストを先行実装
3. **後方互換性**: 既存の設定ファイルが引き続き動作することを保証
4. **セキュリティ優先**: 各段階でセキュリティ検証を含める

## 2. 実装ステップ

### Phase 1: 基盤整備 (Foundation)

**目的**: 型定義とエラー型を整備し、後続の実装基盤を確立する。

#### Step 1.1: 型定義の追加

**ファイル**: `internal/runner/runnertypes/spec.go`

**作業内容**:
1. `CommandTemplate` 構造体の追加
2. `ConfigSpec` に `CommandTemplates map[string]CommandTemplate` フィールド追加
3. `CommandSpec` に `Template string` と `Params map[string]interface{}` フィールド追加

**成功条件**:
- コンパイルが通る
- 既存のユニットテストが全てパスする

**推定工数**: 0.5時間

**コード例**:
```go
// CommandTemplate represents a reusable command definition.
type CommandTemplate struct {
	Cmd             string   `toml:"cmd"`
	Args            []string `toml:"args"`
	Env             []string `toml:"env"`
	WorkDir         string   `toml:"workdir"`
	Timeout         *int32   `toml:"timeout"`
	OutputSizeLimit *int64   `toml:"output_size_limit"`
	RiskLevel       string   `toml:"risk_level"`
}

// ConfigSpec の変更
type ConfigSpec struct {
	Version          string                     `toml:"version"`
	Global           GlobalSpec                 `toml:"global"`
	CommandTemplates map[string]CommandTemplate `toml:"command_templates"` // NEW
	Groups           []GroupSpec                `toml:"groups"`
}

// CommandSpec の変更
type CommandSpec struct {
	Name     string                 `toml:"name"`
	Template string                 `toml:"template"` // NEW
	Params   map[string]interface{} `toml:"params"`   // NEW
	// ... existing fields
}
```

#### Step 1.2: エラー型定義

**ファイル**: `internal/runner/config/template_errors.go` (新規)

**作業内容**:
1. テンプレート関連エラー型の実装
2. パラメータ関連エラー型の実装
3. プレースホルダー解析エラー型の実装

**成功条件**:
- 全エラー型が `error` インターフェースを実装
- エラーメッセージが詳細仕様書の定義通り

**推定工数**: 1時間

**実装するエラー型**:
- `ErrTemplateNotFound`
- `ErrTemplateFieldConflict`
- `ErrDuplicateTemplateName`
- `ErrInvalidTemplateName`
- `ErrReservedTemplateName`
- `ErrTemplateContainsNameField`
- `ErrMissingRequiredField`
- `ErrRequiredParamMissing`
- `ErrTypeMismatch`
- `ErrForbiddenPatternInTemplate`
- `ErrArrayInMixedContext`
- `ErrInvalidArrayElement`
- `ErrUnsupportedParamType`
- `ErrInvalidParamName`
- `ErrEmptyPlaceholderName`
- `ErrMultipleValuesInStringContext`
- `ErrUnclosedPlaceholder`
- `ErrEmptyPlaceholder`
- `ErrInvalidPlaceholderName`

**テスト**:
```go
// template_errors_test.go
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name: "template not found",
			err: &ErrTemplateNotFound{
				CommandName:  "backup",
				TemplateName: "missing",
			},
			expected: `template "missing" not found (referenced by command "backup")`,
		},
		// ... other error types
	}
	// ...
}
```

### Phase 2: プレースホルダー解析 (Placeholder Parsing)

**目的**: テンプレート文字列中のプレースホルダー (`${param}`, `${?param}`, `${@param}`) を解析する機能を実装する。

#### Step 2.1: プレースホルダー型定義と解析関数

**ファイル**: `internal/runner/config/template_expansion.go` (新規)

**作業内容**:
1. `placeholderType` 定数の定義
2. `placeholder` 構造体の定義
3. `parsePlaceholders()` 関数の実装
4. エスケープシーケンス (`\$`, `\\`) のハンドリング

**成功条件**:
- 正常なプレースホルダーを正しく解析できる
- エスケープシーケンスを正しく扱える
- 不正な構文でエラーを返す

**推定工数**: 2時間

**テスト**:
```go
// template_expansion_test.go
func TestParsePlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []placeholder
		wantErr  bool
	}{
		{
			name:  "required parameter",
			input: "${path}",
			expected: []placeholder{
				{fullMatch: "${path}", name: "path", ptype: placeholderRequired, start: 0, end: 7},
			},
		},
		{
			name:  "optional parameter",
			input: "${?verbose}",
			expected: []placeholder{
				{fullMatch: "${?verbose}", name: "verbose", ptype: placeholderOptional, start: 0, end: 11},
			},
		},
		{
			name:  "array parameter",
			input: "${@flags}",
			expected: []placeholder{
				{fullMatch: "${@flags}", name: "flags", ptype: placeholderArray, start: 0, end: 9},
			},
		},
		{
			name:  "escaped dollar",
			input: "\\$100",
			expected: []placeholder{},
		},
		{
			name:    "unclosed placeholder",
			input:   "${path",
			wantErr: true,
		},
		{
			name:    "empty placeholder",
			input:   "${}",
			wantErr: true,
		},
	}
	// ...
}
```

#### Step 2.2: エスケープシーケンス変換

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `applyEscapeSequences()` 関数の実装
2. `\$` → `$`, `\\` → `\` の変換

**成功条件**:
- エスケープシーケンスが正しく変換される
- 既存の `%{var}` 展開のエスケープ (`\%`, `\\`) との一貫性

**推定工数**: 1時間

**テスト**:
```go
func TestApplyEscapeSequences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dollar escape",
			input:    "\\$100",
			expected: "$100",
		},
		{
			name:     "backslash escape",
			input:    "C:\\\\path",
			expected: "C:\\path",
		},
		{
			name:     "no escape",
			input:    "normal text",
			expected: "normal text",
		},
	}
	// ...
}
```

### Phase 3: パラメータ展開 (Parameter Expansion)

**目的**: 解析されたプレースホルダーにパラメータ値を適用して展開する。

#### Step 3.1: 単一引数の展開

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `expandSingleArg()` 関数の実装
2. Pure array placeholder (`${@param}` のみ) の検出と処理
3. Pure optional placeholder (`${?param}` のみ) の検出と処理
4. 文字列置換モードの実装

**成功条件**:
- 各展開モードが正しく動作する
- 配列の混合コンテキストでエラーを返す

**推定工数**: 3時間

**テスト**:
```go
func TestExpandSingleArg(t *testing.T) {
	tests := []struct {
		name         string
		arg          string
		params       map[string]interface{}
		templateName string
		expected     []string
		wantErr      bool
	}{
		{
			name:         "required parameter",
			arg:          "${path}",
			params:       map[string]interface{}{"path": "/data"},
			templateName: "test_template",
			expected:     []string{"/data"},
		},
		{
			name:         "optional with value",
			arg:          "${?flag}",
			params:       map[string]interface{}{"flag": "-v"},
			templateName: "test_template",
			expected:     []string{"-v"},
		},
		{
			name:         "optional empty",
			arg:          "${?flag}",
			params:       map[string]interface{}{"flag": ""},
			templateName: "test_template",
			expected:     []string{},
		},
		{
			name:         "array expansion",
			arg:          "${@flags}",
			params:       map[string]interface{}{"flags": []string{"-a", "-b"}},
			templateName: "test_template",
			expected:     []string{"-a", "-b"},
		},
		{
			name:         "mixed context error",
			arg:          "pre${@arr}post",
			params:       map[string]interface{}{"arr": []string{"a", "b"}},
			templateName: "test_template",
			wantErr:      true,
		},
	}
	// ...
}
```

#### Step 3.2: 配列引数の展開

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `ExpandTemplateArgs()` 関数の実装
2. 各要素に対する `expandSingleArg()` 呼び出し
3. 結果の連結とエスケープシーケンス適用

**成功条件**:
- 複数要素の args 配列を正しく展開できる
- オプショナル要素が空の場合に要素が削除される
- 配列展開で複数要素が挿入される

**推定工数**: 2時間

**テスト**:
```go
func TestExpandTemplateArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		params       map[string]interface{}
		templateName string
		expected     []string
		wantErr      bool
	}{
		{
			name:         "basic expansion",
			args:         []string{"${@flags}", "backup", "${path}"},
			params:       map[string]interface{}{"flags": []string{"-q"}, "path": "/data"},
			templateName: "restic_backup",
			expected:     []string{"-q", "backup", "/data"},
		},
		{
			name:         "optional removal",
			args:         []string{"${@flags}", "backup", "${?verbose}", "${path}"},
			params:       map[string]interface{}{"flags": []string{}, "verbose": "", "path": "/data"},
			templateName: "restic_backup",
			expected:     []string{"backup", "/data"},
		},
		{
			name:         "multiple array elements",
			args:         []string{"${@flags}", "backup"},
			params:       map[string]interface{}{"flags": []string{"-v", "-q", "--no-cache"}},
			templateName: "restic_backup",
			expected:     []string{"-v", "-q", "--no-cache", "backup"},
		},
	}
	// ...
}
```

#### Step 3.3: ヘルパー関数の実装

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `expandArrayPlaceholder()` 関数
2. `expandOptionalPlaceholder()` 関数
3. `expandStringPlaceholders()` 関数
4. 型チェックとエラーハンドリング

**成功条件**:
- 各ヘルパー関数が正しく動作する
- 型不一致でエラーを返す

**推定工数**: 2時間

### Phase 4: セキュリティ検証 (Security Validation)

**目的**: テンプレート定義とパラメータに対するセキュリティ検証を実装する。

#### Step 4.1: テンプレート名検証

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `ValidateTemplateName()` 関数の実装
2. 既存の `ValidateVariableName()` の再利用
3. `__` プレフィックスの禁止チェック

**成功条件**:
- 有効なテンプレート名を受け入れる
- 無効な名前でエラーを返す
- 予約名 (`__*`) でエラーを返す

**推定工数**: 1時間

**テスト**:
```go
func TestValidateTemplateName(t *testing.T) {
	tests := []struct {
		name    string
		tmplName string
		wantErr bool
	}{
		{name: "valid name", tmplName: "restic_backup", wantErr: false},
		{name: "valid with number", tmplName: "backup_v2", wantErr: false},
		{name: "invalid start", tmplName: "123invalid", wantErr: true},
		{name: "reserved prefix", tmplName: "__reserved", wantErr: true},
		{name: "single underscore", tmplName: "_valid", wantErr: false},
	}
	// ...
}
```

#### Step 4.2: テンプレート定義検証

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `ValidateTemplateDefinition()` 関数の実装
2. NF-006: `%{` パターンの禁止チェック (cmd, args, env, workdir)
3. 必須フィールド (cmd) のチェック

**成功条件**:
- `%{` を含むテンプレートを拒否する
- cmd が空の場合にエラーを返す

**推定工数**: 1.5時間

**テスト**:
```go
func TestValidateTemplateDefinition(t *testing.T) {
	tests := []struct {
		name     string
		tmplName string
		template CommandTemplate
		wantErr  bool
		errType  error
	}{
		{
			name:     "valid template",
			tmplName: "restic_backup",
			template: CommandTemplate{Cmd: "restic", Args: []string{"backup", "${path}"}},
			wantErr:  false,
		},
		{
			name:     "forbidden %{ in cmd",
			tmplName: "bad_template",
			template: CommandTemplate{Cmd: "%{root}/bin/restic"},
			wantErr:  true,
			errType:  &ErrForbiddenPatternInTemplate{},
		},
		{
			name:     "forbidden %{ in args",
			tmplName: "bad_template",
			template: CommandTemplate{Cmd: "restic", Args: []string{"%{group_root}/data"}},
			wantErr:  true,
			errType:  &ErrForbiddenPatternInTemplate{},
		},
		{
			name:     "missing cmd",
			tmplName: "incomplete",
			template: CommandTemplate{Args: []string{"backup"}},
			wantErr:  true,
			errType:  &ErrMissingRequiredField{},
		},
	}
	// ...
}
```

#### Step 4.3: パラメータ値検証

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `ValidateParams()` 関数の実装
2. パラメータ名の検証 (`ValidateVariableName` 使用)
3. 型検証 (string または []string のみ許可)
4. NF-006 確認: `%{var}` は params 内で許可される (検証しない)

**成功条件**:
- 有効なパラメータを受け入れる
- 無効なパラメータ名でエラーを返す
- サポート外の型でエラーを返す
- `%{var}` を含む値を許可する (NF-006)

**推定工数**: 1.5時間

**テスト**:
```go
func TestValidateParams(t *testing.T) {
	tests := []struct {
		name         string
		params       map[string]interface{}
		templateName string
		wantErr      bool
	}{
		{
			name:         "valid string param",
			params:       map[string]interface{}{"path": "/data"},
			templateName: "test",
			wantErr:      false,
		},
		{
			name:         "valid array param",
			params:       map[string]interface{}{"flags": []string{"-v", "-q"}},
			templateName: "test",
			wantErr:      false,
		},
		{
			name:         "variable reference allowed in params",
			params:       map[string]interface{}{"path": "%{group_root}/data"},
			templateName: "test",
			wantErr:      false, // NF-006: %{} is allowed in params
		},
		{
			name:         "invalid param name",
			params:       map[string]interface{}{"123invalid": "value"},
			templateName: "test",
			wantErr:      true,
		},
		{
			name:         "unsupported type",
			params:       map[string]interface{}{"number": 123},
			templateName: "test",
			wantErr:      true,
		},
	}
	// ...
}
```

#### Step 4.4: CommandSpec 排他性検証

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `ValidateCommandSpecExclusivity()` 関数の実装
2. `Template` フィールドと `Cmd`/`Args`/`Env`/`WorkDir` の排他チェック
3. `Template` 未指定時の `Cmd` 必須チェック

**成功条件**:
- `Template` と実行フィールドが同時指定された場合にエラー
- `Template` のみ、または実行フィールドのみの場合は成功

**推定工数**: 1時間

**テスト**:
```go
func TestValidateCommandSpecExclusivity(t *testing.T) {
	tests := []struct {
		name    string
		spec    CommandSpec
		wantErr bool
	}{
		{
			name:    "template only (valid)",
			spec:    CommandSpec{Name: "backup", Template: "restic_backup", Params: map[string]interface{}{"path": "/data"}},
			wantErr: false,
		},
		{
			name:    "cmd only (valid)",
			spec:    CommandSpec{Name: "backup", Cmd: "restic", Args: []string{"backup"}},
			wantErr: false,
		},
		{
			name:    "template + cmd (invalid)",
			spec:    CommandSpec{Name: "backup", Template: "restic_backup", Cmd: "restic"},
			wantErr: true,
		},
		{
			name:    "template + args (invalid)",
			spec:    CommandSpec{Name: "backup", Template: "restic_backup", Args: []string{"backup"}},
			wantErr: true,
		},
	}
	// ...
}
```

#### Step 4.5: 使用パラメータ収集

**ファイル**: `internal/runner/config/template_expansion.go`

**作業内容**:
1. `CollectUsedParams()` 関数の実装
2. cmd, args, env からプレースホルダーを抽出
3. 使用されているパラメータ名のセットを返す

**成功条件**:
- 全てのプレースホルダーが検出される
- 重複は除外される

**推定工数**: 1.5時間

**テスト**:
```go
func TestCollectUsedParams(t *testing.T) {
	tests := []struct {
		name     string
		template CommandTemplate
		expected map[string]struct{}
	}{
		{
			name: "multiple params",
			template: CommandTemplate{
				Cmd:  "restic",
				Args: []string{"${@flags}", "backup", "${path}"},
				Env:  []string{"RESTIC_REPO=${repo}"},
			},
			expected: map[string]struct{}{
				"flags": {},
				"path":  {},
				"repo":  {},
			},
		},
		{
			name: "duplicate params",
			template: CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${msg}", "${msg}"},
			},
			expected: map[string]struct{}{
				"msg": {},
			},
		},
	}
	// ...
}
```

### Phase 5: Loader 統合 (Loader Integration)

**目的**: TOML ファイル読み込み時にテンプレートを解析・検証する。

#### Step 5.1: TOML 読み込み対応

- [x] `LoadConfig()` に `ValidateTemplates()` 呼び出し追加
- [x] `ValidateTemplates()` 関数の実装 (各テンプレートに対して検証実行)
- [x] 重複テンプレート名の検出

**成功条件**:
- 有効なテンプレート定義を含む TOML をロードできる
- 無効なテンプレート定義でエラーを返す
- 重複テンプレート名を検出する

**推定工数**: 2時間

**実装済み**: `ValidateTemplates()` を loader.go に実装。LoadConfig() から呼び出し。

#### Step 5.2: "name" フィールド禁止チェック

- [x] TOML パース後、`CommandTemplate` に "name" フィールドが含まれる場合のエラー処理
- [x] カスタム TOML デコーダーまたはパース後の検証
- [x] `checkTemplateNameField()` 関数の実装

**成功条件**:
- テンプレート定義に "name" が含まれる場合にエラーを返す

**推定工数**: 1.5時間

**実装済み**: `checkTemplateNameField()` を loader.go に実装。TOML を map としてパースして name フィールドを検出。

**テスト**:
- [x] `TestLoaderWithTemplates` に各種テンプレートローディングテストを追加
  - [x] 有効なテンプレート
  - [x] 重複テンプレート名
  - [x] 禁止パターン (%{} in cmd/args/env/workdir)
  - [x] 必須フィールド欠如
  - [x] 無効なテンプレート名
  - [x] 予約済みプレフィックス
  - [x] name フィールド禁止

### Phase 6: 展開統合 (Expansion Integration)

**目的**: ExpandCommand にテンプレート展開を統合する。

#### Step 6.1: expandTemplateToSpec 実装

**ファイル**: `internal/runner/config/expansion.go`

**作業内容**:
1. `expandTemplateToSpec()` 関数の実装
2. テンプレート定義から CommandSpec への展開
3. 未使用パラメータの警告生成
4. `expandTemplateString()` および `expandTemplateEnv()` ヘルパー関数の実装

**成功条件**:
- テンプレートから CommandSpec を正しく生成できる
- 未使用パラメータの警告を生成できる
- env の KEY=VALUE 形式を正しく処理できる

**推定工数**: 3時間

**テスト**:
```go
func TestExpandTemplateToSpec(t *testing.T) {
	tests := []struct {
		name         string
		cmdSpec      CommandSpec
		template     CommandTemplate
		expectedSpec CommandSpec
		warnings     []string
	}{
		{
			name: "basic expansion",
			cmdSpec: CommandSpec{
				Name:     "backup_data",
				Template: "restic_backup",
				Params: map[string]interface{}{
					"path": "/data",
				},
			},
			template: CommandTemplate{
				Cmd:  "restic",
				Args: []string{"backup", "${path}"},
			},
			expectedSpec: CommandSpec{
				Name: "backup_data",
				Cmd:  "restic",
				Args: []string{"backup", "/data"},
			},
		},
		{
			name: "unused param warning",
			cmdSpec: CommandSpec{
				Name:     "backup_data",
				Template: "restic_backup",
				Params: map[string]interface{}{
					"path":   "/data",
					"unused": "value",
				},
			},
			template: CommandTemplate{
				Cmd:  "restic",
				Args: []string{"backup", "${path}"},
			},
			warnings: []string{`unused parameter "unused" in template "restic_backup"`},
		},
	}
	// ...
}
```

#### Step 6.2: ExpandCommand への統合

**ファイル**: `internal/runner/config/expansion.go`

**作業内容**:
1. `ExpandCommand()` に `templates map[string]CommandTemplate` パラメータ追加
2. `spec.Template != ""` の場合のテンプレート展開フロー実装
3. 排他性検証の追加
4. 展開後の CommandSpec で既存の変数展開・セキュリティ検証を実行
5. **呼び出し元の修正**: `internal/runner/group_executor.go` およびテストコード全般で `ExpandCommand` の呼び出しシグネチャを修正

**成功条件**:
- テンプレート参照を含むコマンドを正しく展開できる
- 展開後に変数展開 (`%{var}`) が適用される
- 展開後にセキュリティ検証が適用される
- テンプレート未使用のコマンドが引き続き動作する
- 全ての呼び出し元が修正され、コンパイルが通る

**推定工数**: 3時間

**テスト**:
```go
func TestExpandCommandWithTemplate(t *testing.T) {
	tests := []struct {
		name           string
		cmdSpec        CommandSpec
		templates      map[string]CommandTemplate
		groupVars      map[string][]string
		expectedCmd    string
		expectedArgs   []string
		wantErr        bool
	}{
		{
			name: "template expansion with variable",
			cmdSpec: CommandSpec{
				Name:     "backup_data",
				Template: "restic_backup",
				Params: map[string]interface{}{
					"path": "%{backup_root}/data",
				},
			},
			templates: map[string]CommandTemplate{
				"restic_backup": {
					Cmd:  "restic",
					Args: []string{"backup", "${path}"},
				},
			},
			groupVars: map[string][]string{
				"backup_root": {"/mnt/backups"},
			},
			expectedCmd:  "restic",
			expectedArgs: []string{"backup", "/mnt/backups/data"},
		},
		{
			name: "template not found",
			cmdSpec: CommandSpec{
				Name:     "backup_data",
				Template: "nonexistent",
			},
			templates: map[string]CommandTemplate{},
			wantErr:   true,
		},
	}
	// ...
}
```

### Phase 7: 統合テスト (Integration Tests)

**目的**: 実際の TOML ファイルを使用したエンドツーエンドテストを実施する。

#### Step 7.1: サンプル設定ファイル作成

**ファイル**: `sample/command_template_example.toml` (新規)

**作業内容**:
1. 基本的なテンプレート使用例
2. オプショナルパラメータの使用例
3. 配列パラメータの使用例
4. 変数展開との組み合わせ例

**成功条件**:
- 各サンプルが正しく動作する
- ドキュメントとして十分な内容

**推定工数**: 1.5時間

**サンプル例**:
```toml
version = "1.0"

# Template definitions
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${path}"]

[command_templates.restic_restore]
cmd = "restic"
args = ["restore", "latest", "${?target_flag}", "--target", "${target}"]

# Groups using templates
[[groups]]
name = "daily_backup"

[groups.vars]
backup_root = "/data/backups"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"
params.verbose_flags = ["-q"]
params.path = "%{backup_root}/volumes"

[[groups.commands]]
name = "backup_db"
template = "restic_backup"
params.verbose_flags = ["-v", "--no-cache"]
params.path = "%{backup_root}/database"

[[groups]]
name = "restore"

[[groups.commands]]
name = "restore_volumes"
template = "restic_restore"
params.target_flag = "--verify"
params.target = "/mnt/restore"
```

#### Step 7.2: 統合テスト実装

**ファイル**: `internal/runner/config/template_integration_test.go` (新規)

**作業内容**:
1. サンプル設定ファイルの読み込みテスト
2. テンプレート展開 + 変数展開の統合テスト
3. セキュリティ検証の統合テスト
4. cmd_allowed チェックの統合テスト

**成功条件**:
- 全ての統合テストがパスする
- エラーケースが正しく検出される

**推定工数**: 3時間

**テスト例**:
```go
func TestTemplateWithVariableExpansion(t *testing.T) {
	toml := `
version = "1.0"

[command_templates.echo_msg]
cmd = "echo"
args = ["${message}"]

[[groups]]
name = "test"

[groups.vars]
greeting = "Hello"

[[groups.commands]]
name = "say_hello"
template = "echo_msg"
params.message = "%{greeting} World"
`
	// Load and expand
	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(toml))
	if err != nil {
		t.Fatal(err)
	}

	// Expand command
	// ... (詳細は既存の expansion_test.go を参照)
}

func TestTemplateSecurityValidation(t *testing.T) {
	// Test that security validation is applied after template expansion
	// ...
}

func TestTemplateCmdAllowedCheck(t *testing.T) {
	// Test that cmd_allowed check works with templates
	// ...
}
```

### Phase 8: 後方互換性テスト (Backward Compatibility)

**目的**: 既存の設定ファイルが引き続き動作することを確認する。

#### Step 8.1: 既存設定ファイルのテスト

**ファイル**: `internal/runner/config/template_backward_compat_test.go` (新規)

**作業内容**:
1. `sample/` ディレクトリ内の既存設定ファイルをロード
2. テンプレート機能を使用しない設定が正常に動作することを確認

**成功条件**:
- 全ての既存サンプルファイルが正常にロードされる
- 展開結果が変更前と同一

**推定工数**: 2時間

**テスト例**:
```go
func TestBackwardCompatibility(t *testing.T) {
	sampleFiles := []string{
		"sample/starter.toml",
		"sample/comprehensive.toml",
		"sample/risk-based-control.toml",
		// ... other sample files
	}

	loader := NewLoader()
	for _, file := range sampleFiles {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			cfg, err := loader.LoadConfig(content)
			if err != nil {
				t.Errorf("failed to load %s: %v", file, err)
			}

			// Verify basic structure
			if cfg == nil {
				t.Error("config is nil")
			}
		})
	}
}
```

### Phase 9: ドキュメント整備 (Documentation)

**目的**: ユーザー向けドキュメントを作成・更新する。

#### Step 9.1: ユーザーガイド作成

**ファイル**: `docs/user/command_templates.md` (新規)

**作業内容**:
1. テンプレート機能の概要
2. プレースホルダー構文の説明
3. 使用例
4. ベストプラクティス
5. セキュリティに関する注意事項

**推定工数**: 3時間

#### Step 9.2: README.md の更新

**ファイル**: `README.md`, `README.ja.md`

**作業内容**:
1. テンプレート機能の追加を記載
2. 簡単な使用例の追加

**推定工数**: 1時間

#### Step 9.3: CHANGELOG.md の更新

**ファイル**: `CHANGELOG.md`

**作業内容**:
1. 新機能としてテンプレート機能を記載
2. 関連する型定義の変更を記載

**推定工数**: 0.5時間

## 3. 実装順序とマイルストーン

### Milestone 1: 基盤整備 (Phase 1)
- **期間**: 1-2日
- **成果物**: 型定義、エラー型定義
- **確認**: コンパイル成功、既存テスト全パス

### Milestone 2: パース・展開機能 (Phase 2-3)
- **期間**: 3-4日
- **成果物**: プレースホルダー解析、パラメータ展開機能
- **確認**: ユニットテスト全パス

### Milestone 3: セキュリティ検証 (Phase 4)
- **期間**: 2-3日
- **成果物**: セキュリティ検証機能
- **確認**: セキュリティテスト全パス

### Milestone 4: 統合実装 (Phase 5-6)
- **期間**: 3-4日
- **成果物**: Loader 統合、ExpandCommand 統合
- **確認**: 統合テスト全パス

### Milestone 5: テスト・ドキュメント (Phase 7-9)
- **期間**: 3-4日
- **成果物**: 統合テスト、後方互換性テスト、ドキュメント
- **確認**: 全テストパス、ドキュメントレビュー完了

**合計推定期間**: 12-17日

## 4. テスト戦略

### 4.1 ユニットテスト

各関数に対して以下をカバーする:
1. 正常系 (Happy Path)
2. 境界値 (Edge Cases)
3. 異常系 (Error Cases)

**カバレッジ目標**: 90%以上

### 4.2 統合テスト

実際の TOML ファイルを使用した以下のシナリオをカバーする:
1. テンプレート展開 + 変数展開
2. テンプレート展開 + セキュリティ検証
3. テンプレート展開 + cmd_allowed チェック
4. 複数グループでの同一テンプレート使用

### 4.3 後方互換性テスト

既存のサンプル設定ファイルを全てロードし、正常に動作することを確認する。

### 4.4 セキュリティテスト

以下のセキュリティシナリオをテストする:
1. テンプレート定義での `%{var}` 使用 (NF-006違反) → エラー
2. params での `%{var}` 使用 (NF-006準拠) → 許可
3. コマンドインジェクションパターンの検出
4. パストラバーサルパターンの検出

## 5. リスク管理

### 5.1 技術リスク

| リスク | 影響 | 対策 |
|--------|------|------|
| TOML パーサーの制限 (追加フィールド検出) | 中 | カスタムバリデーションで対応 |
| 既存コードへの影響 | 高 | 後方互換性テストを徹底 |
| セキュリティホール | 高 | Phase 4 で重点的に検証 |
| 複雑性の増加 | 中 | ドキュメントを充実させる |

### 5.2 スケジュールリスク

| リスク | 影響 | 対策 |
|--------|------|------|
| 工数見積もりの誤差 | 中 | 各 Phase でバッファを設定 |
| 予期しないバグ | 中 | 段階的実装で早期発見 |
| レビュー遅延 | 低 | 小さな PR を作成 |

## 6. 実装チェックリスト

### Phase 1: 基盤整備
- [x] CommandTemplate 構造体追加
- [x] ConfigSpec への CommandTemplates フィールド追加
- [x] CommandSpec への Template/Params フィールド追加
- [x] 全エラー型定義
- [x] エラーメッセージテスト

### Phase 2: プレースホルダー解析
- [x] placeholderType 定義
- [x] placeholder 構造体定義
- [x] parsePlaceholders() 実装
- [x] applyEscapeSequences() 実装
- [x] ユニットテスト (正常系・異常系)

### Phase 3: パラメータ展開
- [x] expandSingleArg() 実装
- [x] ExpandTemplateArgs() 実装
- [x] expandArrayPlaceholder() 実装
- [x] expandOptionalPlaceholder() 実装
- [x] expandStringPlaceholders() 実装
- [x] ユニットテスト (全展開モード)

### Phase 4: セキュリティ検証
- [x] ValidateTemplateName() 実装
- [x] ValidateTemplateDefinition() 実装
- [x] ValidateParams() 実装
- [x] ValidateCommandSpecExclusivity() 実装
- [x] CollectUsedParams() 実装
- [x] セキュリティテスト

### Phase 5: Loader 統合
- [x] ValidateTemplates() 実装
- [x] LoadConfig() への統合
- [x] "name" フィールド禁止チェック
- [x] 重複テンプレート名検出
- [x] ユニットテスト

### Phase 6: 展開統合
- [x] expandTemplateToSpec() 実装
- [x] ExpandCommand() への統合
  - [x] ExpandCommand() シグネチャ更新 (templates パラメータ追加)
  - [x] テンプレート展開ロジック統合
  - [x] 全呼び出し箇所の更新 (group_executor.go, テストファイル)
- [x] 統合テスト
  - [x] TestExpandTemplateToSpec: expandTemplateToSpec() の単体テスト
  - [x] TestExpandCommandWithTemplate: ExpandCommand() 経由のE2Eテスト

### Phase 7: 統合テスト
- [ ] サンプル設定ファイル作成
- [ ] template_integration_test.go 実装
- [ ] 全統合テスト実装・パス

### Phase 8: 後方互換性テスト
- [ ] template_backward_compat_test.go 実装
- [ ] 全既存サンプルファイルのテスト
- [ ] 互換性確認

### Phase 9: ドキュメント整備
- [ ] command_templates.md 作成
- [ ] README.md 更新
- [ ] README.ja.md 更新
- [ ] CHANGELOG.md 更新
- [ ] ドキュメントレビュー

## 7. 成功基準

1. **機能完成度**
   - 全ての要件定義 (01_requirements.md) の機能が実装されている
   - 詳細仕様書 (03_detailed_spec.md) の全仕様が実装されている

2. **品質**
   - ユニットテストカバレッジ 90%以上
   - 全統合テストがパス
   - 全既存テストがパス (後方互換性)

3. **セキュリティ**
   - NF-006 が正しく実装されている
   - セキュリティテストが全てパス
   - コードレビューでセキュリティ問題が指摘されていない

4. **ドキュメント**
   - ユーザーガイドが完成している
   - サンプル設定ファイルが用意されている
   - CHANGELOG が更新されている

## 8. 次のステップ

実装完了後:
1. プルリクエスト作成
2. コードレビュー
3. セキュリティレビュー
4. マージ
5. リリースノート作成
6. ユーザー向けアナウンス (必要に応じて)
