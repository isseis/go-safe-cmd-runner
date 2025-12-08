# ADR: コマンドテンプレートにおける変数展開順序の決定

## ステータス

採用（未実装）

## コンテキスト

コマンドテンプレート機能（Task 0062）において、テンプレートパラメータ（`${...}`）と既存の変数参照（`%{...}`）の展開順序を決定する必要がある。

### 背景

テンプレート機能では、以下の2種類の置換処理が必要となる：

1. **テンプレートパラメータ展開**: `${param}`, `${?param}`, `${@list}` をパラメータ値で置換
2. **変数展開**: `%{variable}` をグループローカル変数の値で置換

特に、params 内で変数参照を使用できるようにする場合、以下のような設定が可能となる：

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${backup_path}"]

[[groups]]
name = "group1"
[groups.vars]
group_root = "/data/group1"

[[groups.commands]]
template = "restic_backup"
params.verbose_flags = ["-q"]
params.backup_path = "%{group_root}/volumes"
```

この場合、`params.backup_path = "%{group_root}/volumes"` の `%{group_root}` をどのタイミングで展開すべきかが問題となる。

### 検討した展開順序

#### 案1: params を先に展開してからテンプレート展開

```
Step 1: params 内の %{...} を展開
  params.backup_path = "%{group_root}/volumes" → "/data/group1/volumes"

Step 2: テンプレート展開
  args = ["${@verbose_flags}", "backup", "${backup_path}"]
       → ["-q", "backup", "/data/group1/volumes"]
```

**メリット**:
- 直感的: params は「値を渡すもの」なので、渡す前に値が確定している方が自然
- デバッグしやすい: params の値がログに出力される時点で既に展開済み
- エラー箇所の特定が容易: 変数が未定義の場合、テンプレート展開前にエラーが出るので、問題が params にあることが明確

**デメリット**:
- 実装の複雑性: params の値だけを先に展開する処理が必要（通常の `[[groups.commands]]` 全体の展開とは別ロジック）
- 展開順序の例外: 他の `[[groups.commands]]` フィールドは後で展開されるのに、params だけ先に展開されるのは一貫性に欠ける
- 2段階の変数展開: params で1回、その後テンプレート適用後にさらに `%{...}` 展開が必要な箇所があれば2回
- パフォーマンス: 変数展開を複数回実行する必要がある

#### 案2: テンプレート展開を先に実行してから変数展開

```
Step 1: テンプレート展開（params 値をそのまま代入）
  args = ["${@verbose_flags}", "backup", "${backup_path}"]
       → ["-q", "backup", "%{group_root}/volumes"]

Step 2: 変数展開
  args = ["-q", "backup", "%{group_root}/volumes"]
       → ["-q", "backup", "/data/group1/volumes"]
```

**メリット**:
- **一貫性**: 全ての `%{...}` 展開が同じタイミング（最後）で行われる
- **実装がシンプル**: テンプレート展開はパラメータ文字列置換（`${...}` → params値）だけ、変数展開は既存の処理を再利用
- **パフォーマンス**: 変数展開は1回だけ（テンプレート展開後の最終形に対して）
- **処理フローが明確**: 「テンプレート展開 → 変数展開」という一方向の流れ
- **既存コードとの親和性**: 既存の `%{...}` 展開ロジックをそのまま使える

**デメリット**:
- params の値が分かりにくい: デバッグ時に `params.backup_path = "%{group_root}/volumes"` と表示され、実際の値が見えない
- エラーメッセージが複雑: 変数未定義エラーが出た場合、それが params 由来か、テンプレート定義由来か区別しにくい
- 直感に反する: "params に値を渡す" という概念からすると、渡す時点で値が確定していないのは不自然

#### 案3: ハイブリッド（params は展開、テンプレート定義は未展開を維持）

案1と同様だが、テンプレート定義内の `%{...}` は NF-006 で既に禁止済みなので、params だけを考慮すれば良い。

**メリット**: 案1と同様

**デメリット**: 案1と同様

#### 案4: params 内でも %{...} を禁止

変数を使いたい場合はテンプレートを使わず通常の `[[groups.commands]]` を使う。

**メリット**:
- 実装が最もシンプル
- セキュリティリスクが低い

**デメリット**:
- 柔軟性の欠如: グループごとに異なる変数値を使えない（テンプレートの再利用性が低下）
- ユーザビリティの低下: ハードコードが必要
- YAGNI に反する: 変数を使いたいという要求は十分想定される

## 採用した案

**案2: テンプレート展開を先に実行してから変数展開**

### 採用理由

1. **実装コストが最も低い**
   - 既存の変数展開ロジック（`internal/runner/config/expansion.go`）を再利用できる
   - テンプレート展開は純粋な文字列置換処理として実装可能
   - 特別な変数展開タイミングの制御が不要

2. **一貫性が高い**
   - 全ての `%{...}` 展開が同じタイミング（変数展開フェーズ）で行われる
   - 処理フローが「テンプレート展開 → 変数展開」という単純な一方向

3. **セキュリティ検証が簡単**
   - 展開後の最終形に対して既存の検証（`cmd_allowed`, コマンドインジェクション検出等）を適用するだけ
   - テンプレート展開と変数展開の中間状態を検証する必要がない

4. **パフォーマンス**
   - 変数展開は1回のみ（テンプレート展開後の最終コマンド定義に対して）

5. **現在の設計との整合性**
   - NF-006 でテンプレート定義内の `%{...}` は禁止済み
   - params 内での `%{...}` 使用のみを考慮すれば良い

### デメリットへの対処

案2のデメリット（デバッグの難しさ、エラーメッセージの複雑さ、直感に反する）に対して、以下の対策を実装する：

#### 1. デバッグログでの展開前後の値表示（NF-002）

```
DEBUG: Expanding template "restic_backup" with params: {backup_path: "%{group_root}/volumes"}
DEBUG: Template expansion result: args = ["backup", "%{group_root}/volumes"]
DEBUG: Variable expansion result: args = ["backup", "/data/group1/volumes"]
```

テンプレート展開時と変数展開時の両方でログを出力し、各ステップでの状態を追跡可能にする。

#### 2. Dry-run モードでの可視化（NF-007）

```
Group: group1
Command: restic_backup (from template)
  Template parameters:
    verbose_flags = ["-q"]
    backup_path = "%{group_root}/volumes" → "/data/group1/volumes"

  Expanded command:
    cmd: restic
    args: ["-q", "backup", "/data/group1/volumes"]
```

`--dry-run` モードで展開前後の値を両方表示し、ユーザーが結果を事前に確認できるようにする。

#### 3. エラーメッセージへのコンテキスト情報追加（NF-002）

```
Error: variable 'group_root' is not defined in group 'group1',
       referenced by template parameter 'backup_path' in template 'restic_backup' (command #2)

Hint: The parameter 'backup_path' was set to "%{group_root}/volumes"
      Please define 'group_root' in [groups.vars] section or fix the parameter value.
```

変数未定義エラーに以下の情報を含める：
- どの params フィールドから参照されたか
- グループ名、コマンド番号、テンプレート名
- params の値（展開前の式）
- 修正のヒント

#### 4. メンタルモデルの提供（ドキュメント）

params は「値」ではなく「式」を渡すという概念を明確化：
- `params.backup_path = "%{group_root}/volumes"` は「式」を渡している
- テンプレート展開は「式の代入」
- 変数展開は「式の評価」

## 実装への影響

### 展開処理の実装順序

1. **テンプレート展開**（`internal/runner/config/template_expansion.go`）
   - `${...}`, `${?...}`, `${@...}` を params 値で置換
   - params 値内の `%{...}` はそのまま保持（文字列として扱う）
   - 結果として通常の `CommandSpec` を生成

2. **変数展開**（`internal/runner/config/expansion.go`）
   - 既存の変数展開ロジックを使用
   - テンプレート由来のコマンドも通常のコマンドも同じ処理

3. **セキュリティ検証**（`internal/runner/security/`）
   - 変数展開後の最終コマンド定義に対して実行
   - 既存の検証ロジックをそのまま適用

### パフォーマンス考慮事項

- テンプレート展開: 設定ファイル読み込み時に1回のみ
- 変数展開: 既存と同様、設定ファイル読み込み時に1回のみ
- 実行時のオーバーヘッドなし

## セキュリティ考慮事項

### NF-006: テンプレート定義内での変数参照禁止

テンプレート定義（`command_templates` セクション）の `cmd`, `args`, `env`, `workdir` に `%{` が含まれる場合、エラーとして拒否する。

**理由**:
1. テンプレートは複数のグループで再利用される
2. 異なるコンテキストで同じ変数名が異なる意味を持つ可能性
3. グループAでは安全でも、グループBでは機密変数を参照してしまう可能性

### params 内での変数参照は許可

params での `%{...}` 使用は許可する（ローカル変数参照のため）。

**理由**:
1. 各コマンド定義で明示的に変数を参照する
2. どの変数が使用されるか明確
3. コンテキスト（グループ）は明確

### 展開後のセキュリティ検証（NF-005）

テンプレート展開と変数展開の後、最終的なコマンド定義に対して以下を検証：
- コマンドパス検証（`cmd_allowed` / `AllowedCommands`）
- コマンドインジェクション検出
- パストラバーサル検出
- 環境変数検証

## 関連要件

- **F-006**: 展開タイミング（`docs/tasks/0062_command_templates/01_requirements.md`）
- **NF-002**: 明確なエラーメッセージとデバッグ情報
- **NF-005**: 展開後のセキュリティ検証
- **NF-006**: 変数参照のセキュリティ境界
- **NF-007**: Dry-run モードでのテンプレート展開可視化

## 参考資料

- `docs/tasks/0062_command_templates/01_requirements.md` - コマンドテンプレート機能要件定義書
- `internal/runner/config/expansion.go` - 既存の変数展開ロジック
- `internal/runner/security/validator.go` - セキュリティ検証

## 決定日

2025-12-08

## 決定者

設計レビュー（要件定義フェーズ）
