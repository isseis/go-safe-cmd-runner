# テンプレートファイルのInclude機能 - 要件定義書

## 1. 背景と目的

### 1.1 現状の課題

現在、`command_templates` を使用して重複したコマンド定義をまとめることができるが、テンプレート自体は各TOMLファイル内でしか定義できない。これにより、複数のTOMLファイル間で共通のテンプレートを使用したい場合、各ファイルでテンプレート定義を重複させる必要がある。

**問題の具体例**:

共通のバックアップテンプレートを複数のプロジェクトで使用したい場合：

```toml
# project1/backup.toml
version = "1.0"

[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

[[groups]]
name = "daily_backup"
[[groups.commands]]
template = "restic_backup"
params.path = "/data/project1"
params.repo = "/backup/project1"
```

```toml
# project2/backup.toml
version = "1.0"

# 同じテンプレート定義を繰り返す必要がある
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

[[groups]]
name = "daily_backup"
[[groups.commands]]
template = "restic_backup"
params.path = "/data/project2"
params.repo = "/backup/project2"
```

### 1.2 解決方針

テンプレート定義を別ファイルに分離し、複数のコマンド定義TOMLファイルから参照できる**include機能**を導入する。

```toml
# templates/backup_templates.toml (共通テンプレート定義ファイル)
version = "1.0"

[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]

[command_templates.restic_restore]
cmd = "restic"
args = ["restore", "latest", "--target", "${target}"]
env_vars = ["RESTIC_REPOSITORY=${repo}"]
```

```toml
# project1/backup.toml
version = "1.0"

includes = ["../templates/backup_templates.toml"]

[[groups]]
name = "daily_backup"
[[groups.commands]]
template = "restic_backup"  # includeされたテンプレートを使用
params.path = "/data/project1"
params.repo = "/backup/project1"
```

```toml
# project2/backup.toml
version = "1.0"

includes = ["../templates/backup_templates.toml"]

[[groups]]
name = "daily_backup"
[[groups.commands]]
template = "restic_backup"  # 同じテンプレートを再利用
params.path = "/data/project2"
params.repo = "/backup/project2"
```

### 1.3 利点

1. **DRY原則の実現**: テンプレート定義を1箇所に集約し、複数のファイルから再利用できる
2. **保守性の向上**: テンプレートの修正が1箇所で済む
3. **組織的な管理**: チームで共通のテンプレートライブラリを構築できる
4. **セキュリティ**: includeされたファイルもチェックサム検証対象とすることで、改ざんを検知できる

## 2. 機能要件

### 2.1 Include構文

#### F-001: トップレベル `includes` フィールド

コマンド定義TOMLファイルのトップレベルに `includes` 配列を定義できる。

```toml
version = "1.0"

includes = [
    "templates/common.toml",
    "../shared/backup_templates.toml"
]

[global]
timeout = 60

[command_templates.local_template]
cmd = "echo"
args = ["local"]

[[groups]]
name = "example"
```

**仕様**:
- `includes` は文字列の配列
- 配列の各要素は includeするファイルのパス
- `includes` フィールドは省略可能（省略時は空配列として扱う）
- 空配列 `includes = []` も有効（何もincludeしない）

#### F-002: 相対パス解決

includeパスは相対パスと絶対パスの両方をサポートする。

**相対パスの基準**:
- 相対パスは、**コマンド定義TOMLファイルが置かれているディレクトリ**からの相対パス
- 例: `/home/user/project/config.toml` で `includes = ["templates/common.toml"]` の場合、`/home/user/project/templates/common.toml` を参照

**絶対パス**:
- 絶対パスも使用可能
- 例: `includes = ["/etc/runner/templates/common.toml"]`

**パス解決の詳細**:
```toml
# /home/user/project/configs/backup.toml
includes = [
    "templates/common.toml",           # → /home/user/project/configs/templates/common.toml
    "../shared/backup.toml",            # → /home/user/project/shared/backup.toml
    "/etc/runner/templates/system.toml" # → /etc/runner/templates/system.toml (絶対パス)
]
```

### 2.2 Includeファイルの内容

#### F-003: Includeファイルは `command_templates` のみを含む

Includeされるテンプレートファイルには `version` と `[command_templates]` セクションのみを記述できる。

**許可される内容**:
```toml
# templates/backup_templates.toml
version = "1.0"

[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]

[command_templates.restic_restore]
cmd = "restic"
args = ["restore", "latest"]
```

**禁止される内容**:
- 上記以外のあらゆるフィールド・セクション
- 例: `includes`, `[global]`, `[[groups]]`, `[misc]` など

**エラー検出の仕組み**:
- TOMLパーサーの `DisallowUnknownFields()` オプションを使用
- 許可リスト方式により、未定義のフィールド・セクションを自動検出
- 個別の禁止リストが不要で、将来的な拡張にも安全

テンプレートファイルに未定義のセクションやフィールドが含まれている場合、設定ロード時にエラーとする。

#### F-004: 多段Includeの禁止（F-003に統合）

**注記**: 多段includeの禁止はF-003の実装により自動的に実現される。テンプレートファイルで `includes` フィールドは未定義のため、`DisallowUnknownFields()` によって検出される。

### 2.3 テンプレート名の重複検出

#### F-005: テンプレート名の一意性チェック

同じテンプレート名が複数の場所で定義されている場合、エラーとする。

**重複パターン**:

1. **複数のincludeファイル間での重複**:
```toml
# config.toml
includes = ["templates/backup.toml", "templates/restore.toml"]

# templates/backup.toml に [command_templates.common_task]
# templates/restore.toml にも [command_templates.common_task]
# → エラー
```

2. **Includeファイルとコマンド定義ファイル間での重複**:
```toml
# config.toml
includes = ["templates/common.toml"]

[command_templates.backup]  # ← templates/common.toml にも backup が存在
cmd = "echo"
# → エラー
```

3. **コマンド定義ファイル内での重複**:
```toml
# config.toml
[command_templates.backup]
cmd = "echo"

[command_templates.backup]  # ← 重複
cmd = "restic"
# → エラー
```

**エラーメッセージ**:
- 重複したテンプレート名
- 定義されているファイル名（複数）
を含むエラーメッセージを返す

### 2.4 ファイル検証との統合

#### F-006: Includeファイルもチェックサム検証対象

runner実行時のファイルチェックサム検証において、includeされたテンプレートファイルも検証対象に含める。

**ユーザー操作**:

1. **チェックサム記録時** (`record` コマンド):
```bash
# コマンド定義ファイル
./record -f config.toml

# Includeされるテンプレートファイル（個別に記録が必要）
./record -f templates/common.toml
./record -f templates/backup.toml
```

2. **実行時検証** (`runner` コマンド):
```bash
# config.toml を指定すると、includes で参照されている全てのファイルも自動的に検証される
./runner -c config.toml -g backup
```

**実装の詳細**:
- `runner` は config.toml をロードする際、`includes` フィールドを解析
- 各includeファイルのパスを解決し、チェックサム検証リストに追加
- すべてのファイル（config.toml + includeファイル）のチェックサムを検証してから実行

**受け入れ基準**:
1. メイン設定ファイルのハッシュが検証されること
2. includeされた全てのテンプレートファイルのハッシュが検証されること
3. いずれかのファイルのハッシュ検証が失敗した場合、実行前にエラーが返されること
4. テンプレートファイルのハッシュが記録されていない場合、実行前にエラーが返されること
5. テンプレートファイルが改ざんされた場合、実行前にエラーが返されること

#### F-007: Includeファイルの存在チェック

`includes` で指定されたファイルが存在しない場合、設定ロード時にエラーを返す。

```toml
includes = ["templates/missing.toml"]  # ファイルが存在しない
# → エラー: "included file not found: templates/missing.toml"
```

**エラーメッセージに含める情報**:
- 存在しないファイルのパス（解決後の絶対パス）
- includeを記述したファイル名
- 記述された相対パス（元の記述）

### 2.5 読み込み順序

#### F-008: Include処理の順序

設定ファイルの読み込みは以下の順序で行う：

1. コマンド定義TOMLファイルのパース（`includes` フィールドのみ先読み）
2. `includes` 配列に記述された順序で、各テンプレートファイルを読み込み
3. コマンド定義TOMLファイルの `command_templates` を読み込み
4. すべてのテンプレートをマージ（重複チェック）
5. 変数展開・バリデーション

**Include順序の重要性**:
- 複数のincludeファイル間でテンプレート名が重複している場合はエラー（順序に関係なく）
- エラーメッセージには、すべての重複定義の場所を含める

## 3. 非機能要件

### 3.1 セキュリティ

#### NF-001: パストラバーサル対策

includeパスの解決時、パストラバーサル攻撃を防止する。

**対策**:
- 相対パスを解決する際、`filepath.Clean()` を使用して正規化
- シンボリックリンクの評価は `safefileio` パッケージに従う
- 解決後のパスが意図しないディレクトリを指していないか検証

#### NF-002: チェックサム検証の強制

Includeされたファイルは、コマンド定義ファイルと同様にチェックサム検証を必須とする。

**動作**:
- Includeファイルのチェックサムが記録されていない場合、実行時にエラー
- Includeファイルが改ざんされている場合、実行時にエラー

### 3.2 パフォーマンス

#### NF-003: ファイル読み込みの効率化

同じテンプレートファイルが複数回読み込まれないようにする（ただし、現バージョンでは対象外）。

**注意**:
- 現在の要件では、単一のコマンド定義ファイルから複数のテンプレートファイルをincludeする想定
- 将来的に複数のコマンド定義ファイルを同時に扱う場合、キャッシュを検討

### 3.3 エラーハンドリング

#### NF-004: 明確なエラーメッセージ

Include機能に関するエラーは、問題の特定と修正が容易になるよう、詳細な情報を含める。

**含めるべき情報**:
- エラーの種類（ファイル未発見、重複、禁止されたセクション等）
- 関連するファイルパス（コマンド定義ファイル、includeファイル）
- 行番号（可能な場合）

**エラー例**:
```
Error: duplicate command template name "backup"
  Defined in:
    - /home/user/project/templates/common.toml
    - /home/user/project/templates/extra.toml
  Referenced from: /home/user/project/config.toml
```

```
Error: included file not found: "templates/missing.toml"
  Include path: templates/missing.toml (relative)
  Resolved path: /home/user/project/templates/missing.toml
  Referenced from: /home/user/project/config.toml (line 3)
```

```
Error: template file contains invalid fields or sections: /home/user/project/templates/invalid.toml
  File: /home/user/project/templates/invalid.toml
  Template files can only contain 'version' and 'command_templates'
  Detail: toml: line 5: unknown field 'global'
```

```
Error: config file contains invalid fields or sections: /home/user/project/config.toml
  File: /home/user/project/config.toml
  Config files can only contain 'version', 'includes', 'global', 'command_templates', and 'groups'
  Detail: toml: line 10: unknown field 'misc'
```

**注記**:
- `DisallowUnknownFields()` を使用することで、両方のファイルタイプで未定義のフィールド・セクションを自動検出
  - テンプレートファイル: `version`, `command_templates` のみ許可
  - コマンド定義ファイル: `version`, `includes`, `global`, `command_templates`, `groups` のみ許可
- 個別の禁止リストを管理する必要がなく、許可リスト方式でより安全

## 4. 制約事項

### 4.1 多段Includeの禁止

- コマンド定義TOML → テンプレートTOMLのincludeのみ許可
- テンプレートTOML → 他のファイルのincludeは禁止

### 4.2 テンプレートファイルの内容制限

- テンプレートファイルには `[command_templates]` のみ記述可能
- `[global]`、`[[groups]]`、`includes` は禁止

### 4.3 チェックサム記録の手動実行

- `record` コマンドはinclude機能を認識しない
- ユーザーは、コマンド定義ファイルとテンプレートファイルすべてに対して個別に `record` コマンドを実行する必要がある

## 5. 実装方針

### 5.1 データ構造の変更

`ConfigSpec` 構造体に `Includes` フィールドを追加：

```go
type ConfigSpec struct {
    Version          string                      `toml:"version"`
    Includes         []string                    `toml:"includes"`
    Global           GlobalSpec                  `toml:"global"`
    CommandTemplates map[string]CommandTemplate  `toml:"command_templates"`
    Groups           []GroupSpec                 `toml:"groups"`
}
```

### 5.2 ロードプロセスの変更

`Loader.LoadConfig()` を以下のように変更：

1. **メインTOMLファイルをパース**
   - `toml.Decoder.DisallowUnknownFields()` を使用
   - `ConfigSpec` で定義されたフィールド（`version`, `includes`, `global`, `command_templates`, `groups`）以外があればエラー
2. `Includes` フィールドを取得
3. 各includeファイルに対して：
   - パスを解決（相対パス → 絶対パス）
   - ファイルを読み込み
   - `toml.Decoder.DisallowUnknownFields()` を使用してパース
   - `TemplateFileSpec` で定義されたフィールド（`version`, `command_templates`）以外があればエラー
4. すべてのテンプレートをマージ
5. テンプレート名の重複チェック
6. 既存のバリデーション処理を実行

### 5.3 検証プロセスの拡張

`runner` 実行時の事前検証に、includeファイルの検証を追加：

1. コマンド定義ファイルのチェックサム検証
2. `includes` フィールドからincludeファイルのリストを取得
3. 各includeファイルのチェックサム検証
4. すべて成功したら実行を継続

### 5.4 エラー型の追加

Include機能用の新しいエラー型を定義：

```go
// ErrIncludedFileNotFound is returned when an included file does not exist
type ErrIncludedFileNotFound struct {
    IncludePath    string // Path as written in includes array
    ResolvedPath   string // Resolved absolute path
    ReferencedFrom string // Path of file containing the include
}

// ErrConfigFileInvalidFormat is returned when a config file contains
// fields or sections other than version, includes, global, command_templates, and groups
type ErrConfigFileInvalidFormat struct {
    ConfigFile string // Path of config file
    ParseError error  // Original error from go-toml (contains unknown field info)
}

// ErrTemplateFileInvalidFormat is returned when a template file contains
// fields or sections other than version and command_templates
type ErrTemplateFileInvalidFormat struct {
    TemplateFile string // Path of template file
    ParseError   error  // Original error from go-toml (contains unknown field info)
}
```

**設計のポイント**:
- `DisallowUnknownFields()` により、両方のファイルタイプで未定義のフィールド・セクションを自動検出
- コマンド定義ファイルとテンプレートファイルで異なるエラー型を使用（許可されるフィールドが異なるため）
- 個別の禁止リストが不要（`ErrMultiLevelInclude` や `ErrTemplateFileContainsProhibitedSection` は不要）
- `ParseError` にgo-tomlの元のエラーを保持し、詳細情報を提供

## 6. テストケース

### 6.1 正常系

1. **単一ファイルのinclude**
   - 1つのテンプレートファイルをinclude
   - includeされたテンプレートを使用してコマンドを実行

2. **複数ファイルのinclude**
   - 複数のテンプレートファイルをinclude
   - 各ファイルのテンプレートを使用

3. **相対パス解決**
   - 同一ディレクトリ
   - 親ディレクトリ
   - サブディレクトリ

4. **絶対パス**
   - 絶対パスでのinclude

5. **ローカルテンプレートとの併用**
   - includeされたテンプレートとコマンド定義ファイル内のテンプレートの両方を使用

6. **空のincludes**
   - `includes = []`
   - `includes` フィールド省略

### 6.2 異常系

1. **ファイル未発見**
   - 存在しないファイルをinclude
   - 相対パスの解決結果が存在しない

2. **テンプレート名の重複**
   - 複数のincludeファイル間で重複
   - includeファイルとコマンド定義ファイル間で重複
   - コマンド定義ファイル内で重複

3. **未定義のフィールド・セクション**
   - **テンプレートファイル内**:
     - テンプレートファイルに `includes` が存在（多段include）
     - テンプレートファイルに `[global]` が存在
     - テンプレートファイルに `[[groups]]` が存在
     - テンプレートファイルに `[misc]` などの未定義セクションが存在
   - **コマンド定義ファイル内**:
     - コマンド定義ファイルに `[misc]` などの未定義セクションが存在
     - コマンド定義ファイルに未定義のトップレベルフィールドが存在
   - `DisallowUnknownFields()` による自動検出のテスト

4. **チェックサム検証失敗**
   - includeファイルのチェックサムが記録されていない
   - includeファイルが改ざんされている

5. **パストラバーサル**
   - `../../etc/passwd` などの危険なパス

## 7. ドキュメント更新

以下のドキュメントを更新する必要がある：

1. **ユーザーガイド**
   - Include機能の使い方
   - テンプレートファイルの作成方法
   - チェックサム記録の手順

2. **設定ファイルリファレンス**
   - `includes` フィールドの説明
   - テンプレートファイルの仕様

3. **サンプル集**
   - Include機能を使用したサンプル設定

## 8. 今後の拡張案

以下は現バージョンの対象外だが、将来的に検討できる拡張：

1. **オプショナルInclude**
   ```toml
   includes = [
       {path = "templates/common.toml"},
       {path = "templates/optional.toml", optional = true}
   ]
   ```

2. **Include時の名前空間**
   ```toml
   includes = [
       {path = "templates/common.toml", prefix = "common_"}
   ]
   # → common_backup, common_restore としてインポート
   ```

3. **条件付きInclude**
   ```toml
   includes = [
       {path = "templates/linux.toml", os = "linux"},
       {path = "templates/darwin.toml", os = "darwin"}
   ]
   ```

4. **Includeファイルのキャッシュ**
   - 複数のコマンド定義ファイルから同じテンプレートファイルをincludeする場合、1回のみ読み込む

## 9. 参考資料

- 既存のテンプレート機能実装: `internal/runner/config/template_expansion.go`
- 設定ファイルローダー: `internal/runner/config/loader.go`
- ファイル検証: `internal/verification/`
