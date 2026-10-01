# 要件定義書: 設定の展開・検証のエラー型を構造化する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-30 |
| Review date | 2026-10-01 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue

- [#1197](https://github.com/isseis/go-safe-cmd-runner/issues/1197) 設定の展開・検証のエラーが値全体置換で [REDACTED] になる: config のエラー型を構造化する
- 親タスク: Task 0178（[#1168](https://github.com/isseis/go-safe-cmd-runner/issues/1168)、`docs/tasks/0178_structured_error_message_redaction/`）
- 先行の後続タスク: Task 0179（[#1196](https://github.com/isseis/go-safe-cmd-runner/issues/1196)、`docs/tasks/0179_dynlib_shebang_error_structuring/README.md`）

## 背景

### 現在の流れ

Task 0178 で、エラーの本文は `internal/errmsg` の構造化メッセージとして運ばれるようになった。構造化メッセージは、役割（`Constant`・`Identifier`・`Path`・`Text`）を型で宣言した部分の列であり、`RedactingHandler` は部分ごとに役割に応じた redaction を適用する。

| 役割 | 適用する redaction |
|---|---|
| `Constant` | 部分単体には適用しない |
| `Identifier` | 免除 |
| `Path` | `RedactText` のみ（値全体置換は適用しない） |
| `Text` | `RedactText` と、変化がなければ値全体置換 |

ここで値全体置換とは、`SensitivePatterns.IsSensitiveValue`（`password|token|secret|key` などの未アンカーな部分一致）に当たった部分を丸ごと `[REDACTED]` に置き換えることをいう。

構造化メッセージを返さないエラーは、`Error()` 全体を 1 つの `Text` の部分として扱う（fail-closed）。`internal/runner/config` のエラー型のうち、0178 で構造化したのは `ErrUndefinedVariableDetail` だけである。そのほかのエラー型は `Text` のままである。

0178 は `internal/runner/config/expansion.go` のエラー書式（`fmt.Errorf` で原因に文言を付加する箇所）をおおむね構造化した。ただし、次の関数は対象から除いた（`internal/errmsg/errmsg_guard_test.go` の `inScopeExpansionExclusions`）。

- `ProcessEnvImport`
- `ProcessEnv`
- `resolveAndPrepareCommandSpec`
- `ApplyTemplateInheritance`
- `expandTemplateToSpec`

### 問題

`internal/runner/config` のエラーの多くは、group 名・コマンド名・変数名・環境変数名・テンプレート名を本文に含む。これらの名前が機密を示す語を含むと、エラーの本文全体が 1 つの `Text` として値全体置換を受け、`[REDACTED]` になる。

```
system environment variable 'GITHUB_TOKEN' not in allowlist (referenced as 'gh' in group[deploy].from_env)
```

`env_import` で取り込むシステム環境変数の名前には、`TOKEN`・`KEY`・`SECRET` がよく付く。このエラーは、group の展開の失敗の原因として Slack に届く。0178 の後は group 名と要約文は残るが、原因の部分は消える。何が失敗したかは分かるが、どの環境変数で失敗したかは分からない。

原因の全文が残るのは、redaction を通らない最終報告の stderr の `Details:` だけである。

### 届く出力先

`internal/runner/config` と `internal/runner/cli` のエラーは、発生する時点により、次の出力先に届く。Slack のハンドラは設定の読み込みの後に追加される（`cmd/runner/main.go` の `bootstrap.SetupSlackLogging`）。

| 時点 | 例 | 届く出力先 |
|---|---|---|
| 設定の読み込み時の検証 | `ValidateTimeouts`・`ValidateIdentifiers`・`ValidateTemplates`・`ValidateCommands`・テンプレートファイルの読み込み | ログファイル・コンソール・stderr |
| global の展開 | `config.ExpandGlobal`（`env_import`・`vars`・`env` の処理） | Slack・ログファイル・コンソール・stderr |
| テンプレートの検証 | `config.ValidateAllTemplates` | 同上 |
| `--groups` の検証 | `cli.FilterGroups` | 同上 |
| group・コマンドの展開 | `config.ExpandGroup`・`config.ExpandCommand`（テンプレートの展開を含む） | 同上 |

## 目的

- `internal/runner/config` のエラー型と `cli.FilterGroups` のエラーの本文を、役割を宣言した構造化メッセージとして運ぶ。機密ではない名前のために、原因の部分全体が `[REDACTED]` にならないようにする。
- 秘密の保護を弱めない。`Text` から役割を変えるのは、名前やパスのように、設定の中で名前として定義され、検証を通った値と、運用者が `--groups` で指定した group 名・コマンドの `template` で参照したテンプレート名・重複して定義されたテンプレート名などの、運用者が名前の位置に書いた名前に限る（決定事項 3）。生の設定値（`env` の `KEY=VALUE` のエントリ、テンプレートの入力文字列など）と、パラメータの値から来うる文字列（テンプレートの `env_vars` のキーなど）は `Text` のままとする。
- 役割は、エラーを作るコードが型で宣言する。描画済みの文字列（`group[deploy]`・`vars.token_file` など）を解析して役割を決めない（CLAUDE.md「Declare, don't infer」）。

## 用語

- **名前の検証:** 名前そのものの形式や使用可否を確かめる検査。変数名の形式の検査（`validateVariableName`）、システム環境変数の名前の形式の検査（`security.ValidateVariableName`）、予約済みの接頭辞の検査、禁止された環境変数の検査、テンプレート名の検査（`ValidateTemplateName`）、プレースホルダ名の検査を指す。
- **定義された名前:** 設定の中で名前として定義され、その名前に対する名前の検証を通った値。group 名・コマンド名・変数名・テンプレート名・テンプレートのパラメータ名・システム環境変数の名前・`env` のキーが当たる。
- **エラー型:** `internal/runner/config` で宣言され、`Error() string` を持つ型。`errors.go` と `template_errors.go` にある。

## スコープ

### 対象

1. **エラー型:** `internal/runner/config` のすべてのエラー型（現行のコード（`c82781ff`）で 51 型。うち `ErrUndefinedVariableDetail` は構造化済み）に構造化メッセージを実装する。対象を型の集合で定めるのは、「どのエラー型がどの経路に現れるか」の一覧を保守せずに、テストがパッケージのエラー型を走査して検証できるようにするためである（決定事項 5）。
2. **エラー書式:** 次の経路で、原因に文言を付加する箇所（`fmt.Errorf` の `%w`）を構造化する。対象の関数の集合は設計（02）で確定する。
   - 0178 が除いた `expansion.go` の 5 関数（`ProcessEnvImport`・`ProcessEnv`・`resolveAndPrepareCommandSpec`・`ApplyTemplateInheritance`・`expandTemplateToSpec`）
   - テンプレートの展開と検証の経路（`template_expansion.go` のうち、`ValidateAllTemplates` と、group・コマンドの展開から呼ばれる関数）
   - `ExpandGlobal` のうち、0178 で原因をラップする文言を `Text` として宣言した箇所（例: `failed to process global env_import: `）。固定の文言は `Constant` として宣言する。
3. **`--groups` の検証:** `cli.FilterGroups` が返すエラーを構造化する。
4. **ガード:** 0178 の AST ガード（`internal/errmsg/errmsg_guard_test.go` など）の許可位置と対象範囲を、上の 1〜3 に合わせて更新する。
5. **文書:** `docs/dev/architecture_design/security-architecture.ja.md` の「識別子の型宣言による免除」に、システム環境変数の名前・テンプレート名（存在しないテンプレートへの参照名と、重複して定義された名前を含む）・パラメータ名・`--groups` で指定された名前を `Identifier` として免除すること、名前の検証で拒否された名前を `Text` とすることを追記する。英語版は `/mktrans` で反映する。

### 対象外

- **設定の読み込み時の検証のエラー書式:** `validation.go`・`loader.go`・`template_loader.go` の `fmt.Errorf`（例: `group at index %d: %w`、`failed to parse config: %w`）は変更しない。ログファイル・コンソールにしか届かず、stderr に全文が残る。`ValidateIdentifiers` のエラーは、拒否した名前をもともと本文に含めない。ただし、これらの経路で返るエラー型そのものは対象 1 により構造化される。
- **設定ファイルの検証・読み込みのエラー**（`internal/filevalidator`・`internal/verification`、主にパス）: Slack に届かず、stderr に全文が残る。
- **go-toml の文言**（`toml: key timeout is already defined` など）: 語が外部ライブラリの固定の文言の中にあり、`Constant` として宣言できない。`ErrTemplateFileInvalidFormat` が運ぶ go-toml のエラーは、原因として `Text` のまま扱う。
- **Webhook URL の検証エラー:** URL そのものが秘密なので、`Text` のまま現状の保護を維持する。
- **値全体置換のパターンの変更:** 0178 の決定を維持する。
- **`Error()` の文言の変更:** 文言は変えない（F-004）。`checkGroupsExist` が定義済みの group 名の一覧を map の反復順で並べることも変えない。

## 変更の効果: 救われるケースと救われないケース

本節は規範ではない例示であり、要件は決定事項と受け入れ基準が定める。「変更後」は、決定事項の提案どおりに承認された場合に期待される描画であり、実装で確かめた結果ではない。`…` は省略を表す。

### 救われるケース

| ケース | 変更前 | 変更後 |
|---|---|---|
| `env_import` のシステム環境変数の名前が allowlist にない（例: `GITHUB_TOKEN`） | 原因が `[REDACTED]` | `system environment variable 'GITHUB_TOKEN' not in allowlist (referenced as 'gh' in group[deploy].from_env)` がすべて残る |
| 変数の循環参照で、変数名が語を含む（例: `api_key`） | 原因が `[REDACTED]` | `circular reference in group[backup].vars.…: 'api_key' (chain: [api_key token_file api_key])` がすべて残る |
| テンプレート名・パラメータ名が語を含むテンプレートのエラー（例: テンプレート `rotate_token` の必須パラメータ `secret_file` がない） | 原因が `[REDACTED]` | `template "rotate_token" args[0]: required parameter "secret_file" not provided` がすべて残る |
| `vars` と `env_import` で同じ名前を定義し、名前が語を含む | 原因が `[REDACTED]` | 変数名・レベルを含む本文がすべて残る |
| `--groups` に存在しない group 名を指定し、定義済みの group 名が語を含む（例: `token_rotate`） | 原因が `[REDACTED]` | 指定された名前と定義済みの group 名の一覧がすべて残る |

### 一部だけ救われるケース

本文は残るが、`Text` の部分が語を含むときは、その部分だけが `[REDACTED]` になる。

| ケース | 消える部分 |
|---|---|
| 変数名の形式の検証で拒否され、拒否された名前が語を含む | 拒否された名前の部分（決定事項 3） |
| `env` のエントリの形式が不正（例: `API_TOKEN` に `=` がない） | エントリの部分。生の設定値である |
| テンプレートの入力文字列（プレースホルダを含む生の文字列）が語を含む | 入力文字列の部分 |
| テンプレートの `env_vars` で重複したキー（`ErrDuplicateEnvVariableDetail`）が語を含む | キーの部分。パラメータの値から来ることがある（決定事項 3） |
| 検証の理由として内側のエラーの文言を運ぶもの（例: `ErrInvalidSystemVariableNameDetail` の `Reason`） | 理由の部分。`Reason` が固定の文言であることを型で宣言できない場合 |

### 救われないケース・変わらないケース

- **設定の読み込み時の検証のエラー書式:** 対象外。ラップする文言を含めて 1 つの `Text` のままであり、変更前と同じ保護を受ける。
- **stderr の `Details:`:** 変更前から redaction を通らず、全文が出る。文言は変わらない。
- **秘密:** key=value、`Bearer `・`Basic ` の次の語、値形式の検出、属性名による判定でマスクされていた値は、`Identifier` 以外の部分では変更後もマスクされる。

## 決定事項

各項目の「提案」は本書のレビューで承認を受ける。承認後に「決定」へ改める。

### 1. システム環境変数の名前の役割

`env_import` の右辺の名前（例: `gh=GITHUB_TOKEN` の `GITHUB_TOKEN`）は、設定に書かれた名前である。`env_allowed` の名前は名前の検証を受けず、現行のエラーの本文にも現れないので、本決定の対象としない。

→ **提案:** 名前の検証（`security.ValidateVariableName` と禁止された環境変数の検査）を通った名前は `Identifier` とする。値ではなく名前であり、形式は `[A-Za-z_][A-Za-z0-9_]*` に限られる。名前の検証で拒否された名前は、決定事項 3 に従い `Text` とする。

### 2. テンプレート名・パラメータ名・フィールド名の役割

→ **提案:**

- **テンプレート名**（`command_templates` のキーで、`ValidateTemplateName` を通ったもの）とテンプレートのパラメータ名（`${param}` の `param`）は、定義された名前なので `Identifier` とする。
- **コマンドの `template` が参照する名前**は、参照先が存在しないとき（`ErrTemplateNotFound`）も、決定事項 3 に従い `Identifier` とする。この名前は名前の検証を受けないが、`--groups` で指定された存在しない名前（決定事項 4）と同じ理由による。運用者が自分で書いた名前であり、実際に起きるのはテンプレート名の打ち間違いである。誤って秘密を書いた場合に、値形式の検出も免除されて Slack に出ることを、保護の境界として受け入れる。
- **`ErrDuplicateTemplateName` の名前**も、決定事項 3 に従い `Identifier` とする。この名前は `loader.go` で `ValidateTemplateName` より前に検査されるので、形式が検証されていない。それでも、運用者がテンプレートファイルに定義した名前であり、秘密を書く場所ではない。誤って秘密を書いた場合に値形式の検出も免除されることを、保護の境界として受け入れる。このエラーは設定の読み込み時に起き、Slack には届かない。
- **`env` のキー**（`KEY=VALUE` の `KEY`）は、テンプレートを使わない `env_vars` では、名前の検証（`ProcessEnv` の `security.ValidateVariableName` と禁止された環境変数の検査）を通ったものを `Identifier` とする。キーにプレースホルダを含むもの（`ErrPlaceholderInEnvKey`）は名前の検証で拒否されたものとして `Text` とする。
- **テンプレートの `env_vars` のキー**は、パラメータの展開の後に読むので、パラメータの値から来ることがある（例: エントリ全体を `${@param}` で書いた場合）。エラー型はキーの出所を宣言できないので、名前の検証より前に報告される `ErrDuplicateEnvVariableDetail` のキーは、決定事項 3 に従い `Text` とする。
- **フィールド名**（`cmd`・`args[0]`・`env_vars[1]`・`vars.<name>` など）は、キーを `Constant`、添字を `Text`、変数名を `Identifier` とする。0178 の `ErrUndefinedVariableDetail` と同じく、フィールドとレベル（`global`・`group[<name>]`・`command[<name>]`・`template[<name>]`）は、描画済みの文字列ではなく型付きの値（既存の `Level`・`Field`）で運ぶ。`fmt.Sprintf("vars.%s", name)` のように組み立てた文字列を後から解析して分けることはしない。
- **数値**（添字・件数・上限・深さ）は、0178 と同じく `Text` とする。

### 3. 名前の検証で拒否された名前の役割

`validateGroupName`・`validateCommandName` は、拒否した名前を本文に含めない（拒否された識別子が資格情報の形をしている可能性がある、という理由がコードのコメントにある）。変数名・システム環境変数の名前・テンプレート名・プレースホルダ名のエラーは、拒否した名前を本文に含める。

→ **提案:** 名前の役割は、その文字列がどこから来たかで決める。本決定を一般の規則とし、決定事項 1・2・4 は個々の名前への適用を述べる。

- 名前の検証で拒否された名前は、定義された名前ではないので `Text` とする。形式が正しく、検証以外の理由（未定義・重複・衝突・allowlist にない・循環参照など）でエラーになった名前は `Identifier` とする。
- 運用者が名前を書く位置（名前の位置）に自分で書いた名前は、名前の検証を受ける前でも `Identifier` とする。テンプレートの `vars` のキー（`ExpandTemplateVars` は値だけを展開し、キーは展開しない）、コマンドの `template` が参照する名前（`ErrTemplateNotFound`）、重複して定義されたテンプレート名（`ErrDuplicateTemplateName`）、`--groups` で指定された名前が当たる。誤って秘密を書いた場合に値形式の検出も免除されることを、保護の境界として受け入れる（個々の理由は決定事項 2・4）。
- パラメータの値や生の設定値から来た文字列は、名前の位置に入っても `Text` とする。テンプレートの `env_vars` のキー（決定事項 2）が当たる。

本文に含めない方向への文言の変更は、本タスクでは行わない（F-004）。

### 4. `--groups` の名前の役割

→ **提案:**

- `--groups` で指定され、設定に存在しない名前も、決定事項 3 に従い `Identifier` とする。理由は次のとおりである。
  - コマンドライン引数は、プロセス一覧・シェルの履歴・systemd のユニットや cron の定義に残る。秘密を渡す経路ではない。
  - 入力するのは runner を起動する運用者自身であり、外部からの入力ではない。
  - 実際に起きるのは group 名の打ち間違いであり、打ち間違えた名前は定義済みの group 名と同じく機密を示す語を含みうる（例: `token_rotate` のつもりの `token_rotat`）。`Text` にすると、救いたい本文そのものが消える。
- この扱いにより、誤って秘密を `--groups` に貼り付けた場合、その値は値形式の検出も免除されて Slack に出る。これを保護の境界として受け入れる。
- 定義済みの group 名の一覧（`Available groups:`）は、`ValidateIdentifiers` を通った group 名なので、各要素を `Identifier` とする。
- `%v` による `[a b c]` の書式は、区切り文字と括弧を `Constant` として保ち、`Error()` の文言を変えない。

### 5. 対象範囲

Issue は優先順位として、(1) `env_import`・allowlist、(2) 循環参照、(3) テンプレート、(4) `cli.FilterGroups`、(5) 設定の読み込み時の検証を挙げている。

→ **提案:**

- **エラー型:** (1)〜(5) を区別せず、`internal/runner/config` のすべてのエラー型を対象とする。(5) だけに現れる型（`ErrDuplicateTemplateName`・`ErrTemplateFieldConflict`・`ErrIncludedFileNotFound` など）も含める。型ごとの追加の費用は小さい。さらに、対象を型の集合で定めると、「すべてのエラー型が構造化メッセージを実装する」をパッケージの型の走査で検証できる。経路ごとに対象の型を選ぶと、どの型がどの経路に現れるかの一覧が要り、一覧の漏れを検証できない。
- **エラー書式:** Slack に届く経路（global の展開・テンプレートの検証・group とコマンドの展開）と `cli.FilterGroups` に限る。(5) の設定の読み込み時の検証のエラー書式は対象外とする（スコープの「対象外」）。
- **PR の分け方:** 実装計画（03）で決める。Issue の優先順位を PR の順序の目安とする。

### 6. `Error()` の文言と到達性

- 対象のエラーの `Error()` の文言は、変更前とバイト単位で一致させる。`%q` で引用していた値は、引用と逃がし文字（エスケープ）の結果も含めて一致させる。コマンド名のように `"` や `\` を含みうる値で、引用の結果を役割付きの部分としてどう表すかは設計（02）で決める。
- `errors.Is`・`errors.AsType` が届く対象は変えない。エラー書式を構造化した箇所は、変更前に `%w` で届いた原因に引き続き届く。

### 7. 原因の構造を保つ

原因をラップするエラー（`ErrInvalidVariableScopeDetail.Err`、`ErrTemplateFileInvalidFormat.ParseError`、対象のエラー書式）は、原因の構造を保つ。原因が構造化メッセージを返せばその部分を使い、返さなければ原因の `Error()` を `Text` とする（0178 の `errmsg.Cause`）。原因を 1 つの `Text` に平らにしない。

## 受け入れ基準（Acceptance Criteria）

#### F-001: エラー型の構造化

**Acceptance Criteria**:
- **AC-01**: `internal/runner/config` で宣言され `Error() string` を持つすべての型が、`errmsg.Structured` を実装する。テストはパッケージの型宣言を走査して確かめ、対象の型の一覧を保守しない。構造化メッセージを実装しない型を加えると、このテストが失敗する。
- **AC-02**: 各エラー型の構造化メッセージで、定義された名前（決定事項 1・2 の名前、group 名・コマンド名・変数名）は `Identifier`、パスは `Path`、固定の文言は `Constant` として宣言される。
- **AC-03**: 名前の検証で拒否された名前、生の設定値（`env` のエントリ・`env_import` のマッピング・テンプレートの入力文字列）、パラメータの値から来うる名前（`ErrDuplicateEnvVariableDetail` のテンプレートの `env_vars` のキー）、数値は `Text` として宣言される（決定事項 3）。
- **AC-04**: レベルとフィールドの部分は、型付きの `Level`・`Field` から作られる。エラー型のレベル・フィールドを描画済みの文字列として持つフィールドは残らない。
- **AC-05**: 原因を持つエラー型は、原因の構造を保つ。構造化メッセージを返す原因の `Identifier` の部分は、外側のエラーを通しても `Identifier` として描画される。

#### F-002: エラー書式と `--groups` の構造化

**Acceptance Criteria**:
- **AC-06**: スコープの対象 2 の関数の中に、`%w` を含む `fmt.Errorf` がない。この AC は、対象の関数の中を実際に走査する AST の検査で確かめる。後から加わった箇所も、一覧を保守せずに検査の対象になる。
- **AC-07**: スコープの対象 2 の箇所で原因に付加する固定の文言は `Constant`、挿入する名前は決定事項 1〜3 の役割として宣言される。
- **AC-08**: `cli.FilterGroups` が存在しない group 名で失敗したとき、返すエラーは構造化メッセージを実装し、指定された存在しない名前と定義済みの group 名は `Identifier` として宣言される。`errors.Is(err, cli.ErrGroupNotFound)` が成り立つ。

#### F-003: 通知の本文

次の AC は、経路全体を通した動作の例示である。すべての箇所の網羅は AC-01・AC-06 が担う。

- **AC-09**: group の展開で、`env_import` のシステム環境変数の名前（例: `GITHUB_TOKEN`）が allowlist になく、group 名とシステム環境変数の名前が機密を示す語を含むとき、Slack の `Error Message` では、システム環境変数の名前・変数名・group 名が置き換えられずに出る。
- **AC-10**: global の `vars` の展開で、名前が機密を示す語を含む変数が循環参照するとき、Slack の `Error Message` では、循環の経路の変数名が置き換えられずに出る。
- **AC-11**: `ValidateAllTemplates` の失敗で、テンプレート名と変数名が機密を示す語を含むとき、Slack の `Error Message` では、テンプレート名と変数名が置き換えられずに出る。
- **AC-12**: `--groups` に存在しない名前を指定し、指定した名前と定義済みの group 名がどちらも機密を示す語を含むとき、Slack の `Error Message` では、指定した名前と定義済みの group 名がどちらも置き換えられずに出る。
- **AC-13**: コマンドの展開でテンプレートの展開が失敗し、テンプレート名・パラメータ名が機密を示す語を含むとき、Slack の `Error Message` では、テンプレート名・パラメータ名・コマンド名・group 名が置き換えられずに出る。

#### F-004: 既存の出力の維持

**Acceptance Criteria**:
- **AC-14**: 対象のエラーの `Error()` の文言は、変更前とバイト単位で一致する。`%q` で引用していた値が `"`・`\`・非 ASCII の文字を含む場合も一致する。
- **AC-15**: `errors.Is`・`errors.AsType` は、変更前と同じ対象に届く。
- **AC-16**: 最終報告の stderr の `Details:` の文言は、変更前と同じである。
- **AC-17**: 通知の件数・`message_type`・`error_type`・Scope・Slack のフィールド構成は、変更前と同じである。

#### F-005: 保護の維持

**Acceptance Criteria**:
- **AC-18**: `Text` として宣言した部分（生の設定値・拒否された名前）に値全体置換だけが反応する入力を与えると、その部分が置換文字列になる。
- **AC-19**: `Identifier` 以外の部分に値形式の検出だけが反応する値（例: GitHub トークン形式の値）を含む `env` のエントリやテンプレートの入力文字列は、変更後もマスクされる。

#### F-006: 宣言の保証

**Acceptance Criteria**:
- **AC-20**: 0178 の AST ガードが、本タスクで変更する箇所（`internal/runner/config` のエラー型と対象のエラー書式、`internal/runner/cli` の該当箇所）を対象に含む。`Identifier`・`Path` の宣言は許可位置でだけ通り、許可位置以外では拒否される。`Constant` には定数式だけが渡される。
- **AC-21**: 本番コードに、描画済みのレベル・フィールドの文字列を解析して役割を決める処理がない。
- **AC-22**: 上の各ガードとテストに、対象の実装を壊すと失敗することを確かめる自己テストがある。

#### F-007: 文書

**Acceptance Criteria**:
- **AC-23**: `docs/dev/architecture_design/security-architecture.ja.md` に、システム環境変数の名前・テンプレート名・パラメータ名を `Identifier` とすること、名前の検証で拒否された名前を `Text` とすること、`--groups` で指定された名前・存在しないテンプレートへの参照名・重複して定義されたテンプレート名を `Identifier` とすることとその保護の境界が記載されている。英語版は日本語版と同じ内容である。

#### F-008: 全体の健全性

**Acceptance Criteria**:
- **AC-24**: `make test`・`make lint` が通る。

### テストの入力についての制約

- 層ごとの効果を確かめるテストは、1 つの層だけが反応する入力を使う。値全体置換だけが反応する入力（例: 名前 `GITHUB_TOKEN`・`api_key`）と、値形式の検出だけが反応する入力（例: GitHub トークン形式の値）を分けて用意する。そのうえで、他の層だけでは入力が変わらないことを先に確かめる（CLAUDE.md「A layered path needs inputs only one layer can handle」）。
- `Identifier` の部分が免除されることを示すテストは、同じメッセージに同じ文字列の `Text` の対照の部分を置き、対照が置換され、`Identifier` の部分が置換されないことを 1 つのテストで示す（0179 と同じ）。
- 例示のシナリオ（AC-09〜AC-13）は、エラーの発生元（`ExpandGlobal`・`ValidateAllTemplates`・`cli.FilterGroups`・group executor）から、`RedactingHandler` を通った後のレコードまでを通すテストで確かめる。Slack に届くレコードでは、Slack のメッセージ組み立てまでを通す。

## Success Criteria（要件レベル）

- global・group・コマンドの展開、テンプレートの検証、`--groups` の検証の失敗の Slack 通知で、定義された名前（システム環境変数の名前・テンプレート名・パラメータ名を含む）のために原因の部分全体が `[REDACTED]` にならない。
- 生の設定値・拒否された名前は `Text` として、変更前と同じ保護を受ける。
- 役割は型で宣言され、文字列の内容から推測されない。
- `Error()` の文言、stderr の文言、通知の種別と構成は変わらない。
