# テンプレート定義でのグローバル変数参照 - 要件定義書

## 1. 背景と目的

### 1.1 現状の課題

テンプレート機能において、テンプレート定義内での変数参照（`%{var}`）が全面的に禁止されている。これにより、複数のテンプレートで共通の値（例: コマンドパス）を使用する際、DRY原則に反する繰り返しが発生する。

**問題の具体例**:

複数のAWSコマンドテンプレートで、毎回AWSコマンドのパスをparamsで渡す必要がある：

```toml
[global.vars]
aws_path = "/usr/local/bin/aws"

# 各テンプレートでAWSコマンドを使いたい
[command_templates.s3_sync]
cmd = "${aws_cmd}"  # paramsで渡す必要がある
args = ["s3", "sync", "${src}", "${dst}"]

[command_templates.s3_cp]
cmd = "${aws_cmd}"  # 同じく
args = ["s3", "cp", "${src}", "${dst}"]

# 全てのコマンドで繰り返し
[[groups.commands]]
template = "s3_sync"
params.aws_cmd = "%{aws_path}"  # 繰り返し
params.src = "/data"
params.dst = "s3://bucket"

[[groups.commands]]
template = "s3_cp"
params.aws_cmd = "%{aws_path}"  # 繰り返し
params.src = "/file"
params.dst = "s3://bucket"
```

### 1.2 解決方針

**命名規則による名前空間の分離**を導入し、グローバル変数のみテンプレート定義で参照可能にする。

- **大文字始まり変数**: グローバル変数（`[global.vars]`でのみ定義可能）
- **小文字始まり変数**: ローカル変数（`[groups.vars]`, `[groups.commands.vars]`でのみ定義可能）

```toml
[global.vars]
AwsPath = "/usr/local/bin/aws"  # 大文字始まり = グローバル

[command_templates.s3_sync]
cmd = "%{AwsPath}"  # ✅ グローバル変数を直接参照
args = ["s3", "sync", "${src}", "${dst}"]

[command_templates.s3_cp]
cmd = "%{AwsPath}"  # ✅ 同じグローバル変数を参照
args = ["s3", "cp", "${src}", "${dst}"]

[[groups]]
name = "backup"

[groups.vars]
data_dir = "/data"  # 小文字始まり = ローカル

[[groups.commands]]
template = "s3_sync"
params.src = "%{data_dir}"  # ローカル変数はparamsで
params.dst = "s3://bucket"
# AwsPath は不要（テンプレートで解決される）
```

### 1.3 利点

1. **DRY原則の実現**: グローバル変数の定義が1箇所に集約
2. **スコープの明示**: 変数名を見るだけでグローバル/ローカルが判断可能
3. **override不可**: 名前空間が分離されているため、意図しない上書きが発生しない
4. **セキュリティ**: テンプレートがローカル変数にアクセスできないため、スコープ境界が明確

## 2. 機能要件

### F-001: 変数の命名規則

#### グローバル変数

- **定義場所**: `[global.vars]` のみ
- **命名規則**: 大文字（A-Z）で始まる
- **使用可能な文字**: 英数字（A-Z, a-z, 0-9）とアンダースコア（_）
- **例**: `AwsPath`, `PythonPath`, `DEFAULT_TIMEOUT`, `Max_Retries`

#### ローカル変数

- **定義場所**: `[groups.vars]`, `[groups.commands.vars]` のみ
- **命名規則**: 小文字（a-z）またはアンダースコア（_）で始まる
- **使用可能な文字**: 英数字（A-Z, a-z, 0-9）とアンダースコア（_）
- **例**: `data_dir`, `backup_path`, `log_level`, `_internal`

#### 予約済み変数

- **命名規則**: `__` で始まる変数名
- **制約**: 全ての場所で定義不可（将来の拡張用に予約）

### F-002: テンプレート定義での変数参照

#### 参照可能な変数

テンプレート定義（`cmd`, `args`, `env`, `workdir`等）では、**グローバル変数のみ**参照可能。

```toml
[global.vars]
AwsPath = "/usr/local/bin/aws"
DefaultTimeout = "3600"

[command_templates.s3_sync]
cmd = "%{AwsPath}"              # ✅ OK
args = ["s3", "sync", "${src}", "${dst}"]
timeout = "%{DefaultTimeout}"   # ✅ OK
```

#### 参照不可な変数

ローカル変数（小文字始まり）の参照は禁止。

```toml
[command_templates.bad_example]
cmd = "/bin/echo"
args = ["%{data_dir}"]  # ❌ エラー: ローカル変数は参照不可
```

### F-003: paramsでの変数参照

`params` 内では、グローバル変数とローカル変数の**両方を参照可能**。

```toml
[global.vars]
AwsPath = "/usr/local/bin/aws"

[[groups]]
name = "backup"

[groups.vars]
data_dir = "/data/prod"
backup_bucket = "s3://prod-backup"

[[groups.commands]]
template = "s3_sync"
params.src = "%{data_dir}"        # ✅ ローカル変数
params.dst = "%{backup_bucket}"   # ✅ ローカル変数
# params.custom_aws = "%{AwsPath}" # ✅ グローバル変数も参照可能（必要なら）
```

### F-004: 検証ルール

#### 定義時の検証

| 場所 | 許可される変数名 | 禁止される変数名 |
|------|----------------|-----------------|
| `[global.vars]` | 大文字始まり | 小文字始まり、`__`始まり |
| `[groups.vars]` | 小文字始まり、`_`始まり | 大文字始まり、`__`始まり |
| `[groups.commands.vars]` | 小文字始まり、`_`始まり | 大文字始まり、`__`始まり |

#### テンプレート定義での検証

- テンプレート内の全ての変数参照が大文字始まりであることを確認
- 参照されている全てのグローバル変数が `[global.vars]` で定義されていることを確認

**検証タイミング**: 設定ファイル読み込み時（実行前に全てのエラーを検出）

## 3. 非機能要件

### NF-001: セキュリティ

#### 名前空間の厳格な分離

- グローバル変数とローカル変数の名前空間を完全に分離
- テンプレートはグローバル変数のみアクセス可能（ローカル変数へのアクセスは不可）
- 定義場所と変数名の組み合わせを静的に検証

#### 既存のセキュリティ保護の維持

- 展開後のコマンドパス検証（`cmd_allowed`, `AllowedCommands`）
- コマンドインジェクション検出
- パストラバーサル検出
- 環境変数検証
- 非再帰的展開

### NF-002: エラーメッセージの明確性

全てのエラーメッセージに以下を含める：

1. **問題の内容**: 何が間違っているか
2. **発生場所**: ファイル、セクション、変数名
3. **理由**: なぜそのルールがあるか
4. **修正方法**: 具体的な修正例

**例**:

```
Error in [global.vars]: Variable "aws_path" must start with uppercase letter
  Rule: Global variables must start with uppercase (A-Z)
  Example: "AwsPath", "AWS_PATH", "DefaultTimeout"

  Hint: Global variables can be used in template definitions.
        Use lowercase for group-specific variables.

Error in template "s3_sync" field "cmd": Cannot reference local variable "data_dir"
  Rule: Templates can only reference global variables (uppercase start)

  Fix: Use a parameter instead:
    Template:  cmd = "${data_dir}"
    Command:   params.data_dir = "%{data_dir}"

Error in template "s3_sync": Global variable "AwsPath" is not defined
  Rule: All variables in templates must be defined in [global.vars]

  Fix: Add to [global.vars]:
    AwsPath = "/path/to/aws"
```

### NF-003: 後方互換性

既存の設定ファイルへの影響を最小化：

1. **互換性のあるケース**:
   - 既存の `[global.vars]` が既に大文字始まり変数のみ使用している場合
   - 既存の `[groups.vars]` が既に小文字始まり変数のみ使用している場合
   - テンプレートで `%{` を使用していない場合（既にエラーなので影響なし）

2. **互換性のないケース**:
   - `[global.vars]` に小文字始まり変数がある場合
   - `[groups.vars]` に大文字始まり変数がある場合
   - これらのケースでは明確なエラーメッセージを表示し、手作業での修正を促す

### NF-004: パフォーマンス

- **事前検証**: 全ての検証を設定読み込み時に完了
- **1回の展開**: 展開は設定読み込み時に1回のみ実行
- **実行時オーバーヘッドなし**: 展開済みコマンドを使用するため、実行時の追加コストなし

## 4. ユースケース

### UC-001: 複数のAWSコマンドテンプレート

```toml
[global.vars]
AwsPath = "/usr/local/bin/aws"
AwsRegion = "us-west-2"
DefaultTimeout = "3600"

[command_templates.s3_sync]
cmd = "%{AwsPath}"
args = ["--region", "%{AwsRegion}", "s3", "sync", "${src}", "${dst}"]
timeout = "%{DefaultTimeout}"

[command_templates.s3_cp]
cmd = "%{AwsPath}"
args = ["--region", "%{AwsRegion}", "s3", "cp", "${src}", "${dst}"]

[command_templates.ec2_describe]
cmd = "%{AwsPath}"
args = ["--region", "%{AwsRegion}", "ec2", "describe-instances", "${@filters}"]

[[groups]]
name = "backup_prod"

[groups.vars]
data_dir = "/data/prod"
backup_bucket = "s3://prod-backup"

[[groups.commands]]
name = "sync_data"
template = "s3_sync"
params.src = "%{data_dir}"
params.dst = "%{backup_bucket}/data"

[[groups.commands]]
name = "sync_logs"
template = "s3_sync"
params.src = "/var/log/app"
params.dst = "%{backup_bucket}/logs"
```

### UC-002: 複数のツールでの共通設定

```toml
[global.vars]
PythonPath = "/usr/bin/python3"
ResticPath = "/usr/local/bin/restic"
DefaultWorkDir = "/tmp/runner"

[command_templates.python_script]
cmd = "%{PythonPath}"
args = ["${script_path}", "${@script_args}"]
workdir = "%{DefaultWorkDir}"

[command_templates.restic_backup]
cmd = "%{ResticPath}"
args = ["backup", "${path}"]

[command_templates.restic_check]
cmd = "%{ResticPath}"
args = ["check"]

[[groups]]
name = "maintenance"

[groups.vars]
scripts_dir = "/opt/scripts"
data_dir = "/data"

[[groups.commands]]
name = "cleanup"
template = "python_script"
params.script_path = "%{scripts_dir}/cleanup.py"
params.script_args = ["%{data_dir}"]

[[groups.commands]]
name = "backup"
template = "restic_backup"
params.path = "%{data_dir}"

[[groups.commands]]
name = "verify"
template = "restic_check"
```

## 5. 実装上の重要ポイント

### 5.1 展開の順序

既存の展開順序を維持：

1. **テンプレート展開**: `${...}` をparams値で置換（`%{...}` はそのまま）
2. **変数展開**: `%{...}` を変数値で置換（グローバル/ローカル両方）

グローバル変数がテンプレート定義に含まれる場合：

```toml
[global.vars]
AwsPath = "/usr/bin/aws"

[command_templates.s3_sync]
cmd = "%{AwsPath}"                    # Step 2で展開
args = ["s3", "sync", "${src}", "${dst}"]  # Step 1で展開

[[groups.commands]]
template = "s3_sync"
params.src = "/data"
params.dst = "s3://bucket"

# 展開結果:
# Step 1: cmd = "%{AwsPath}", args = ["s3", "sync", "/data", "s3://bucket"]
# Step 2: cmd = "/usr/bin/aws", args = ["s3", "sync", "/data", "s3://bucket"]
```

### 5.2 型による安全性の向上

変数のスコープを型レベルで表現し、不正な使用を防止：

```go
type GlobalVariable struct {
    Name  string
    Value string
}

type LocalVariable struct {
    Name  string
    Value string
}

type VariableRegistry interface {
    RegisterGlobal(name, value string) error  // 大文字始まりを強制
    RegisterLocal(name, value string) error   // 小文字始まりを強制
    Resolve(name string) (string, error)      // 自動的にスコープを判定
}
```

### 5.3 検証の階層

1. **Phase 1: 命名規則検証** - 変数定義時に名前をチェック
2. **Phase 2: スコープ検証** - テンプレート内の変数参照をチェック
3. **Phase 3: 存在検証** - 参照されている変数が定義されているかチェック
4. **Phase 4: セキュリティ検証** - 展開後のコマンドを既存の検証で確認

## 6. 推奨スタイル

実装では大文字始まり/小文字始まりのみを強制するが、以下のスタイルを推奨：

### グローバル変数: UpperCamelCase

```toml
[global.vars]
# 推奨
AwsPath = "/usr/local/bin/aws"
PythonPath = "/usr/bin/python3"
DefaultTimeout = "3600"

# 許可されるが推奨しない
AWS_PATH = "/usr/local/bin/aws"  # ALL_CAPS
Aws_Path = "/usr/local/bin/aws"  # Mixed
```

### ローカル変数: snake_case

```toml
[groups.vars]
# 推奨
data_dir = "/data/prod"
backup_bucket = "s3://prod-backup"
log_level = "info"

# 許可されるが推奨しない
dataDir = "/data/prod"      # camelCase
data_Dir = "/data/prod"     # Mixed
_public = "value"           # アンダースコア始まりは内部用に推奨
```

## 7. 成功基準

以下の基準を満たすことで、機能が正しく実装されたと判断する：

- [ ] グローバル変数の命名規則違反を検出できる
- [ ] ローカル変数の命名規則違反を検出できる
- [ ] テンプレート内のローカル変数参照を検出できる
- [ ] テンプレート内の未定義グローバル変数参照を検出できる
- [ ] 全てのエラーメッセージが明確で修正方法を含む
- [ ] 展開後のコマンドが既存のセキュリティ検証を通過する
- [ ] 既存の全テストが通過する
- [ ] 新機能のテストカバレッジが90%以上
- [ ] パフォーマンスの劣化が5%以内

## 8. 用語

- **グローバル変数**: `[global.vars]`で定義される、大文字始まりの変数。全てのテンプレートとグループから参照可能。
- **ローカル変数**: `[groups.vars]`または`[groups.commands.vars]`で定義される、小文字始まりの変数。定義されたスコープ内でのみ参照可能。
- **名前空間**: 変数名の最初の文字（大文字 vs 小文字）によって区別されるスコープ。
- **予約済み変数**: `__`で始まる変数名。将来の拡張用に予約。
