# 要件定義書: エラー本文を役割付きの部分として運び、部分ごとに redaction する

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-27 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#1168](https://github.com/isseis/go-safe-cmd-runner/issues/1168) 通知の自由文本文が値全体置換で丸ごと [REDACTED] になる: 本文を役割付きの構造で運び部分ごとに redaction する
- 派生元: Task 0176（`docs/tasks/0176_group_pre_execution_failure_notification/02_architecture.md` §3.7・§5.2・§9）
- 関連: [#1153](https://github.com/isseis/go-safe-cmd-runner/issues/1153)（Task 0177 で解決済み。`executeGroups` が全 group のエラーを返す）
- 関連: [#1154](https://github.com/isseis/go-safe-cmd-runner/issues/1154)（失敗対象一覧を持たない検証失敗の報告形式の統一）

## 背景

### 現在の流れ

エラーの本文は、次の 2 種類の構造化ログレコードに `error_message` 属性（文字列）として記録される。

| レコード | 記録する関数 | 本文 | 出力先 |
|---|---|---|---|
| group 実行前段の失敗（0176 の #1〜#7） | `logging.NotifyPreExecutionError`（[`runner.go`](../../../internal/runner/runner.go) の `executeGroups` から） | `PreExecutionError.Detail()` = 段階の要約文 + `: ` + 原因の `Error()` | Slack・ログファイル・コンソール |
| 最終の実行エラー（`execution_error`、`slack_notify=false`） | `logging.HandleExecutionError`（[`cmd/runner/main.go`](../../../cmd/runner/main.go) から） | `error running commands` + 外側の context + `: ` + `GroupErrors.Error()`（全 group の原因を改行で連結） | ログファイル・コンソール |

どちらのレコードも同じ `RedactingHandler` を通る（`internal/runner/bootstrap/logger.go` の `SetupLoggerWithConfig`）。文字列の属性は `RedactingHandler.redactLogAttributeWithContext` で次の順に処理される。

1. 属性名が機密を示せば、値を丸ごと置換する。
2. 値が宣言済みの識別子（`identifier.Identifier`）なら、redaction を免除する。
3. `Config.RedactText` を適用する。これは key=value 置換、`Bearer `・`Basic ` の次の語の置換、値形式の検出（AWS キー・GitHub トークン・PEM ブロックなど）から成る。
4. 3 で値が変わらなければ、`SensitivePatterns.IsSensitiveValue` で値全体を判定する。これは `(?i)(password|token|secret|key|api_key)|bearer|basic|authorization…` の未アンカーな部分一致で、当たれば値全体を `[REDACTED]` に置き換える（以下「値全体置換」）。

4 は、属性名からも値の形式からも分からない秘密を拾う最後の網である。

最終報告の stderr の `Details:` は redaction を通らない（`internal/logging/pre_execution_error.go` の `handleErrorCommon`）。Task 0177 以降、stderr には失敗した全 group の原因が出る。

### 問題

本文は、固定の文言と、設定やファイルシステムに由来する値（group 名・コマンド名・変数名・パス・生のテンプレート）を 1 本の文字列に連結したものである。redaction はどの部分に秘密がありうるかを区別できないため、機密ではない名前やパスが上の語を含むだけで、本文全体を消す。

現行のコード（`1c760ac6`）の既定設定の `RedactingHandler` に、本文の形式に沿って組み立てた `error_message` を通した結果を示す。

| 本文（抜粋） | 結果 | 引き金になった部分 |
|---|---|---|
| `Group preparation failed: failed to expand group[backup]: undefined variable in group[backup].vars: 'api_key' (context: %{api_key})` | 全体が `[REDACTED]` | 変数名 |
| `Command preparation failed: … command[upload] (index 0): undefined variable in command[upload].args: 'token_file' (context: --config=%{token_file})` | 全体 | 変数名 |
| `Group preparation failed: failed to expand group[monkey-test]: undefined variable in group[monkey-test].vars: 'dir' (context: %{dir})` | 全体 | group 名（`monkey`） |
| `Group directory permission audit failed: … for group[token-rotate]: 1 directory violation(s) detected; …` | 全体 | group 名 |
| `Group preparation failed: failed to resolve work directory: group[backup]: working directory must be an absolute path: "keycloak/data" (relative paths are not allowed for security reasons)` | 全体 | 作業ディレクトリのパス |
| `Group preparation failed: failed to resolve work directory: failed to create temporary directory: mkdir /tmp/scr-token-rotate-…: no space left on device` | 全体 | 一時ディレクトリのパス（group 名を含む） |
| `Command verification failed: command path resolution failed for "ssh-keygen": …` | 全体 | コマンドパス |
| `Command verification failed: command dependency verification failed for "/usr/bin/curl": … /lib/x86_64-linux-gnu/libkeyutils.so.1` | 全体 | 依存ライブラリのパス |
| `error running commands: failed to execute group token-rotate: command renew in group token-rotate failed: …` | 全体 | group 名 |
| `Command preparation failed: … undefined variable in command[upload].cmd: 'bin_dir' (context: %{bin_dir}/rclone)` | 残る | — |

最終の実行エラーは全 group の原因を 1 本に連結する。そのため、1 つの group の名前やパスが語を含むだけで、全 group の原因がログファイルとコンソールからまとめて消える。

原因の全文が残るのは、redaction を通らない stderr だけである。Slack の通知では、`error_type` と Scope（識別子として免除される group 名・コマンド名）しか残らない。

### 先例

- Task 0175 の `failed_file_paths` は、文字列スライスの要素として運ばれる。各要素は `RedactText` だけを受け、値全体置換を受けない（`RedactingHandler.processSlice`）。構造化して運んだパスを値全体置換の対象外とすることは、すでに受け入れられている。
- `identifier.Identifier` は、group 名・コマンド名を型で宣言し、redaction を免除する（`docs/dev/architecture_design/security-architecture.ja.md`「識別子の型宣言による免除」）。同じ文書は、エラー文字列に連結された識別子は免除できず、値全体置換で全文が `[REDACTED]` になりうると記している。本タスクはこの制約を、対象の本文について解消する。

## 目的

- 対象の本文を、役割を型で宣言した部分の列として運ぶ。redaction は部分ごとに役割に応じて適用し、機密ではない名前やパスのために本文全体が消えないようにする。
- 秘密の保護を弱めない。現在マスクされる値は、引き続きマスクされる。値全体置換を免れるのは、型で役割を宣言した部分だけとする。
- 役割は、エラーを作るコードが宣言する。redaction や通知ビルダーは、文字列の内容から役割を推測しない（CLAUDE.md「Declare, don't infer」）。
- 構造を持たないエラーは、現状と同じ保護を受ける（fail-closed）。これにより、エラー型を段階的に移行できる。

## スコープ

### 対象

1. 役割付きの部分の列を持つ本文の型（以下「構造化メッセージ」）と、エラー型が構造化メッセージを返すためのインターフェースを新設する。
2. `RedactingHandler` が、構造化メッセージの部分ごとに役割に応じた redaction を適用し、1 本の文字列に描画する。
3. 次の 2 つのレコードの `error_message` を、構造化メッセージとして記録する。
   - group 実行前段の失敗（`NotifyPreExecutionError`。0176 の #1〜#7）
   - 最終の実行エラー（`HandleExecutionError`）
4. 次のエラーを構造化する（役割の割り当ては決定事項を参照）。
   - `PreExecutionError`（段階の要約文と原因）と `ExecutionError`（要約文と外側の context）
   - `GroupStageError`・`GroupErrors`・`GroupError`・`CommandExecutionError`
   - `group_executor.go` が `fmt.Errorf` で付加するエラー書式（group 名・コマンド名・コマンドパス・index を含むもの）と、コマンドの終了コードのエラー書式（`ErrCommandFailed` をラップするもの）
   - `config.ErrUndefinedVariableDetail`
   - 作業ディレクトリの解決の失敗。group・コマンドの両方について、次の 3 種類を対象とする。
     - 変数の展開の失敗（`config.ExpandWorkDir` の `failed to expand workdir: %w`）
     - 相対パスの拒否（`config.ExpandWorkDir` の `ErrInvalidWorkDir`。group・コマンドの名前と展開後のパスを含む）
     - 一時ディレクトリの作成・権限設定の失敗（`executor.DefaultTempDirManager.Create`。OS のエラー `*fs.PathError` がパスを持つ）
   - 実行全体の中断と group の失敗が重なったときの最終エラー。現在は `executeGroups` が `errors.Join(ctxErr, err)` で返している。これを、中断であることを宣言する専用の型に置き換える。
   - `cmd/runner/main.go` で、原因を `fmt.Sprintf("…: %v", err)` で `Message` に埋め込んでいる次の 4 か所。`Message` を固定の文言にし、原因は `Err` で運ぶ。これにより、構造を持つ原因（`ErrUndefinedVariableDetail` など）の役割の宣言が、この経路でも失われない。
     - global の展開の失敗（`Failed to expand global configuration`）
     - テンプレート検証の失敗（`Template validation failed`）
     - ディレクトリ権限チェッカーの初期化の失敗（`directory permission checker initialisation failed`）
     - `--groups` の指定誤り（`Invalid groups specified`）
5. 役割 `Constant` の部分が定数式からしか作られないことを、AST ガードのテストで保証する。
6. 開発者向け文書 `docs/dev/architecture_design/security-architecture.ja.md` の「識別子の型宣言による免除」に、構造化メッセージの役割ごとの redaction と、`Path` 役を値全体置換の対象外とする保護の境界を追記する。英語版は `/mktrans` で反映する。

### 対象外

- **その他の `pre_execution_error` の原因の構造化**（設定の読み込み、global の展開、テンプレート検証、`--groups` の指定誤りなど）。対象 4 の `Err` への付け替えを除き、原因は構造を持たない `Text` として現状と同じ保護を受ける。
  - 設定の展開・検証のエラー型（`internal/runner/config` の 51 型と 48 か所のエラー書式）と `cli.FilterGroups` のエラーの構造化は、[#1197](https://github.com/isseis/go-safe-cmd-runner/issues/1197) で扱う。この中には、0178 の対象の本文（group の展開の失敗の原因）に現れる `env_import`・allowlist のエラーも含まれる。
  - 設定ファイルの検証・読み込みのエラー（主にパス）は、Slack に届かず stderr に全文が残るため、対処しない。
  - go-toml のエラー文言（`toml: key timeout is already defined` など）は、語が外部ライブラリの固定の文言の中にあり、型で宣言できないため、対処しない。
  - Webhook URL の検証エラーは、URL そのものが秘密なので `Text` のまま現状の保護を維持する。
- **パス解決・依存ライブラリ検証・shebang 検証の内側のエラー、および外部ライブラリや OS のエラー。** 当初は `Text` のままとする。ただし、作業ディレクトリの解決の失敗（対象 4）の OS のエラーは除く。例えば依存検証の失敗では、要約文・コマンドパスは残り、依存ライブラリのパスを含む内側の原因だけが `[REDACTED]` になりうる。 dynlib・shebang 検証のエラー型の構造化は [#1196](https://github.com/isseis/go-safe-cmd-runner/issues/1196) で扱う。
- **値全体置換のパターンの変更。** アンカーや単語境界の追加、特定の語句の除外は行わない（決定事項「検討して採らなかった案」）。
- **`error_message` 以外の属性。** `slog.Error(..., "error", err)` のように error 型の値を持つ属性（`RedactingHandler.processError`）の扱いは変えない。
- **最終報告の stderr の `Details:`。** redaction を通さない現状を維持する（決定事項を参照）。
- **redaction を通らない経路。** ログの設定前に報告される失敗（起動時の特権降格、`--run-id`、Webhook 環境変数の検証など）は、現状どおり redaction を通らない。
- **通知種別・Slack のフィールド・0172 の表示安全な補間契約の変更。**

## 変更の効果: 救われるケースと救われないケース

本節の「変更後」は、本要件と決定事項の役割の割り当てから期待される描画である。実装で確かめた結果ではない。`…` は省略を表す。

### 救われるケース

役割を宣言した部分（`Identifier`・`Path`・`Constant`）が機密を示す語を含んでも、本文全体は消えなくなる。

| ケース | 変更前 | 変更後 |
|---|---|---|
| group 名が語を含む group 実行前段の失敗（例: ディレクトリ権限監査の違反） | 全体が `[REDACTED]` | `Group directory permission audit failed: directory permission audit failed for group[token-rotate]: 1 directory violation(s) detected; …` がすべて残る |
| コマンド名が語を含むコマンドの展開の失敗 | 全体 | 要約文・group 名・コマンド名が残る。内側の原因は後述の「一部だけ救われるケース」のとおり |
| 変数名が語を含む未定義変数（例: `api_key`） | 全体 | 要約文・group 名・変数名 `'api_key'` が残る。生のテンプレートの部分は、下の表のとおり |
| コマンドパスが語を含むコマンドのパス解決の失敗（例: `ssh-keygen`） | 全体 | `Command verification failed: command path resolution failed for "ssh-keygen": ` までが残る。内側の原因は `Text` |
| コマンドが 0 以外の終了コードで失敗し、group 名かコマンド名が語を含む（最終の実行エラー） | 全体 | `error running commands (group: token-rotate, command: renew): failed to execute group token-rotate: command renew in group token-rotate failed: …command renew failed with exit code 1` がすべて残る |
| 複数 group が失敗し、どれか 1 つの group 名が語を含む（最終の実行エラー） | 全 group の原因がまとめて消える | 各 group の行が残る。消えるのは、各行の中で `Text` の部分が語を含むときのその部分だけ |
| 作業ディレクトリが相対パスで、パスが語を含む（例: `keycloak/data`） | 全体 | `Group preparation failed: failed to resolve work directory: group[backup]: working directory must be an absolute path: "keycloak/data" (relative paths are not allowed for security reasons)` がすべて残る |
| 一時ディレクトリの作成に失敗し、パスが語を含む（group 名 `token-rotate` から作られる `/tmp/scr-token-rotate-…`） | 全体 | `Group preparation failed: failed to resolve work directory: failed to create temporary directory: mkdir /tmp/scr-token-rotate-…: no space left on device` がすべて残る。OS のエラーのうち、パス以外の部分（`mkdir`、`no space left on device`）は `Text` |
| 実行全体の中断（SIGINT・SIGTERM）と group の失敗が重なり、group 名かコマンド名が語を含む（最終の実行エラー） | 全体 | 中断の文言（`context canceled` など）と、中断時に失敗した group の原因が残る。原因の中は、上の各ケースと同じく部分ごとに扱われる |

### 一部だけ救われるケース

本文は残るが、`Text` の部分が語を含むときは、その部分だけが `[REDACTED]` になる。

| ケース | 変更後 | 消える部分 |
|---|---|---|
| 未定義変数 `api_key` を生のテンプレート `%{api_key}` で参照（作業ディレクトリの展開の失敗、global の展開の失敗も同じ） | `Group preparation failed: failed to expand group[backup]: undefined variable in group[backup].vars: 'api_key' (context: [REDACTED])` | 生のテンプレート。変数名そのものを含むので、変数名が語を含めば必ず消える |
| 依存検証の失敗で、原因に `libkeyutils.so.1` などのパスを含む | `Command verification failed: command dependency verification failed for "/usr/bin/curl": [REDACTED]` | 依存ライブラリのパスを含む内側の原因（dynlib 検証のエラーは構造化しない。[#1196](https://github.com/isseis/go-safe-cmd-runner/issues/1196)） |
| shebang 検証・パス解決の内側のエラーが語を含む | 要約文とコマンドパスは残る | 内側の原因 |

### 救われないケース

次のケースでは、変更前と同じく本文全体が `[REDACTED]` になりうる。

| ケース | 理由 |
|---|---|
| 対象外の `pre_execution_error` のうち、原因が構造を持たないもの（例: `Invalid groups specified: group(s) [secret-rotate] specified in --groups do not exist in configuration …`、`Failed to verify and read the configuration file: … /etc/gscr/basic.toml`） | 原因を構造化しない（[#1197](https://github.com/isseis/go-safe-cmd-runner/issues/1197)）。`Message` の固定の文言は残るが、原因全体が `Text` |
| group・コマンドの展開の失敗で、原因が `env_import`・allowlist などのエラー（例: `system environment variable 'GITHUB_TOKEN' not in allowlist …`） | 要約文・group 名・コマンド名は残る。原因のエラー型を構造化しないため（[#1197](https://github.com/isseis/go-safe-cmd-runner/issues/1197)）、原因の部分は `Text` |
| 設定の読み込みの失敗で、go-toml の文言が語を含む（例: `toml: key timeout is already defined`） | 同上。値全体置換のパターンも変えない |
| `error_message` 以外の属性（例: 依存検証の失敗時の `slog.Error` の `error` 属性・`command` 属性） | 対象外。属性の扱いを変えない |

### 変わらないケース

- **stderr の `Details:`:** 変更前から redaction を通らず、原因の全文が出る。本タスクの前後で文言は変わらない。
- **ログの設定前に報告される失敗:** 起動時の特権降格や `--run-id` の検証などは、変更前から redaction を通らない。
- **語を含まない本文:** 変更前から値全体置換を受けず、変更後も同じ文字列になる（例: `undefined variable in command[upload].cmd: 'bin_dir' (context: %{bin_dir}/rclone)`）。
- **秘密:** key=value、`Bearer `・`Basic ` の次の語、値形式の検出、属性名による判定でマスクされていた値は、どの部分にあっても変更後もマスクされる（`Identifier` の部分を除く。F-001）。

## 決定事項

### 役割と適用する redaction

構造化メッセージの各部分は、次のいずれかの役割を持つ。ゼロ値は、最も保守的な `Text` とする。

| 役割 | 意味 | 適用する redaction |
|---|---|---|
| `Constant` | コードに書かれた固定の文言 | 部分単体には適用しない（部分をまたぐ検出は次節） |
| `Identifier` | 設定で定義された名前（group 名・コマンド名・変数名） | 免除（`identifier.Identifier` と同じ） |
| `Path` | ファイルシステム上のパス（コマンドパスなど） | `RedactText` のみ。値全体置換は適用しない |
| `Text` | 上のどれにも当たらない自由文 | 現行の全段（`RedactText` と、変化がなければ値全体置換） |

`Text` の値全体置換は、その部分だけを `[REDACTED]` に置き換える。本文の他の部分は残る。

`Path` 役に値全体置換を適用しないことは、保護の境界として承認されている。0175 の `failed_file_paths` と同じ扱いである。パスに直書きされた秘密は、key=value などの形式か値形式の検出に当たる場合にだけマスクされる。

### 属性全体と部分の境界に効く保護

- 属性名による判定は、従来どおり部分より先に属性全体へ効かせる。機密を示す属性名の下では、構造化メッセージも値ごと置換する。
- 部分の境界をまたぐ秘密の形式もマスクする。例えば `Identifier` の `API_KEY`、`Constant` の `=`、`Text` の値が連結されて key=value になる場合、値はマスクされる。どの手段で満たすかは設計で決める。

### 構造を持たないエラーは `Text`

原因の連鎖の中で構造化メッセージを返さないエラーは、その `Error()` 全体を 1 つの `Text` の部分とする。未対応のエラー型、`errors.Join` などの標準ライブラリのエラー、外部ライブラリのエラーは、現状と同じ保護を受ける。

`errors.Join` の結果の子を、`Unwrap() []error` を持つという形から判定して個別に扱うことはしない。これは、Task 0177 が複数 group の失敗を形から判定しないと決めたのと同じ理由による。部分ごとに扱いたい複合エラーは、専用の型で宣言する。

### `PreExecutionError.Message` の役割

`PreExecutionError.Message` は、現在は普通の文字列のフィールドである。`Constant` として扱えるのは、定数式で作られたことが型で保証される場合だけとする。

- group 実行前段の要約文（段階の定義表の定数）と、対象 4 で固定の文言にする 4 か所の `Message` は、`Constant` として扱う。
- `fmt.Sprintf` などで値を含めて作る `Message`（例: `Total: %d, Verified: %d, …`、`unhandled check skip reason %d for path %s`）は `Text` として扱う。
- どう保証するか（型の変更、構築関数、AST ガードの範囲）は設計で決める。

### 中断時の最終エラーは専用の型で宣言する

実行全体の中断と group の失敗が重なったとき、`executeGroups` は `errors.Join(ctxErr, err)` の代わりに、中断であることを宣言する専用の型を返す。この型は、中断の原因（`ctx.Err()`）と、中断時に失敗した group のエラーを持ち、それぞれを部分として描画する。

- `Error()` の文言は、変更前の `errors.Join(ctxErr, err)` と同じである。
- `errors.Is(err, context.Canceled)`（または `context.DeadlineExceeded`）と、失敗した group の原因への `errors.Is`・`errors.AsType` の到達性は変えない。
- 中断時に返すエラーの中身（それ以前に集めた group の失敗を含めないこと）は変えない。

### 作業ディレクトリの解決の失敗の宣言

- 相対パスの拒否のエラーは、group・コマンドの名前を `Identifier`、展開後のパスを `Path` として宣言する。名前は現在、`group[<name>]`・`command[<name>]` の形の文字列（`level`）として渡されている。この文字列の中の名前も `Identifier` として宣言する。
- 一時ディレクトリの作成・権限設定の失敗では、OS のエラー `*fs.PathError` の `Path` を `Path` として宣言する。`Op`（`mkdir` など）と `Err`（`no space left on device` など）は `Text` とする。`*fs.PathError` を部分に分けるのは、型（`errors.AsType[*fs.PathError]`）によってであり、文言の解析によらない。この扱いは作業ディレクトリの解決の失敗に限り、他の経路の `*fs.PathError` は `Text` のままとする。

### 役割は宣言で決める

役割は、エラーを作るコードが型で宣言する。`RedactingHandler`・通知ビルダー・ログ出力のどこも、部分の文字列の内容を見て役割を選んだり変えたりしない。

### 対象のエラーの役割の割り当て

| 対象 | `Identifier` | `Path` | `Text` |
|---|---|---|---|
| `group_executor.go` のエラー書式 | group 名・コマンド名 | 展開済みのコマンドパス・解決済みのコマンドパス | ラップした原因（構造を持たなければ） |
| `GroupError`・`CommandExecutionError`・`ExecutionError` の外側の context | group 名・コマンド名 | — | ラップした原因（同上） |
| 終了コードのエラー書式 | コマンド名 | — | — |
| `config.ErrUndefinedVariableDetail` | 変数名・展開経路（`Chain`）の各変数名・`Level` に含まれる group・コマンドの名前 | — | 生のテンプレート（`Context`） |
| 作業ディレクトリの相対パスの拒否 | group・コマンドの名前 | 展開後のパス | — |
| 一時ディレクトリの作成・権限設定の失敗 | — | `*fs.PathError` の `Path` | `*fs.PathError` の `Op`・`Err` |
| 中断時の最終エラー | — | — | 中断の原因（`ctx.Err()`）。失敗した group のエラーは、その型の宣言に従う |

- 表に無い固定の文言は `Constant` とする。
- 数値（index・終了コード・件数）の役割と、`ErrUndefinedVariableDetail` の `Field` の役割は、設計で決める。ただし `Constant` にできるのは定数式だけである。
- 生のテンプレートは、秘密が直書きされうるので `Text` とする。

### 描画後の本文

- **文言:** redaction を適用する前の描画結果は、現在の `Error()`・`Detail()` の文言と一致する。`Error()` の文言と `errors.Is`・`errors.AsType` の到達性は変えない。
- **ログの形:** 構造化ログの `error_message` は、`RedactingHandler` の後段のハンドラ（Slack・ログファイル・コンソール）に、描画済みの文字列として渡す。ログの利用者と通知ビルダー（`buildPreExecutionError`）は変わらない。
- **redaction を通らない場合:** `RedactingHandler` を通らないハンドラに渡った場合も、文字列として描画される。
- **Slack での表示:** Slack の `Error Message` は、描画済みの文字列に従来どおり 0172 の補間契約（`common.Interpolate` の `InterpolationRoleFreeText`：1 行・制御文字なし・500 byte 上限）を適用する。切り詰められても、先頭の段階の要約文は残る。

### stderr は現状を維持する

最終報告の stderr の `Details:` は、redaction を通さない現状を維持する。stderr は、原因の全文が残る唯一の出力として運用上の価値がある。本タスクの前後で、stderr の文言は変わらない。

### 検討して採らなかった案

- **値全体置換のパターンにアンカーや単語境界を付ける。** すべての属性の保護が一律に下がる。そのうえ、`api_key` のような変数名は単語として一致するため解決しない。
- **値全体置換のパターンから特定の語句（go-toml の `key` を含む文言など）を除外する。** 除外すべき文言が複数に分かれ、依存ライブラリの更新で文言が変わると黙って効かなくなる。文字列の内容で判断を変える点でも「Declare, don't infer」に反する。また、設定の読み込みの失敗は Slack ハンドラの追加前に起き、Slack には届かない。
- **本文から名前やパスを文字列加工で取り除く。** 「Declare, don't infer」に反し、文言の形式が変わると壊れる。
- **`error_message` だけ値全体置換を外す。** 生のテンプレートに直書きされた値など、形式の分からない秘密が漏れる。
- **部分を構造化した属性としてログに出す。** 機械で読みやすくなるが、ログのスキーマと通知ビルダーの変更が要る。本文が読めるようになるという目的には不要である。
- **stderr にも部分ごとの redaction を適用する。** 原因の全文が残る出力がなくなる。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 部分ごとの redaction

**Acceptance Criteria**:
- **AC-01**: `Identifier` の部分は、機密を示す語を含んでも（例: `api_key`・`monkey-test`）、key=value の形でも、値形式の検出に当たる形でも、書き換えられずに描画される。
- **AC-02**: `Path` の部分は、機密を示す語を含むだけ（例: `/usr/bin/ssh-keygen`・`/lib/x86_64-linux-gnu/libkeyutils.so.1`）では書き換えられない。
- **AC-03**: `Path` の部分に `RedactText` が反応する値（key=value の形、値形式の検出に当たるトークン）が含まれるとき、その値はマスクされる。
- **AC-04**: `Text` の部分は、`RedactText` が反応すればその結果になる。反応せず値全体置換に当たれば、その部分だけが `[REDACTED]` になり、同じ本文の他の部分は残る。
- **AC-05**: `Constant` の部分は、機密を示す語を含んでも書き換えられない。
- **AC-06**: 部分の境界をまたいで key=value の形になる秘密（例: `Identifier` の `API_KEY`、`Constant` の `=`、`Text` の値）は、値がマスクされる。
- **AC-07**: 機密を示す属性名の下に置かれた構造化メッセージは、値ごと置換される。
- **AC-08**: 役割を明示しない部分（ゼロ値）は `Text` として扱われる。

#### F-002: 構造を持たないエラーの保護

**Acceptance Criteria**:
- **AC-09**: 構造化メッセージを返さないエラーを原因とする本文では、その原因の `Error()` 全体が 1 つの `Text` の部分として扱われる。この部分に値全体置換だけが反応する入力（例: `token` を含み、key=value の形でも値形式にも当たらない文言）では、その部分が `[REDACTED]` になる。
- **AC-10**: 構造化メッセージを返すエラーが、構造を持たないエラーをラップしているとき、外側の宣言された部分は役割どおりに扱われ、内側の原因は `Text` として扱われる。
- **AC-11**: 対象外の `pre_execution_error`（例: 設定の読み込みの失敗）の `error_message` は、現状と同じ redaction の結果になる。

#### F-003: 対象の本文

**Acceptance Criteria**:
- **AC-12**: 未定義の変数 `api_key` を参照する group の展開の失敗について、Slack の `Error Message` に、段階の要約文・group 名・変数名 `api_key` が出る。生のテンプレートの部分は、`Text` として扱われる。
- **AC-13**: 名前が機密を示す語を含む group（例: `token-rotate`）の、group 実行前段の失敗（#1〜#7 のいずれか）について、Slack の `Error Message` に group 名が出て、本文全体は `[REDACTED]` にならない。
- **AC-14**: コマンドパスが機密を示す語を含むコマンド（例: `ssh-keygen`）のパス解決の失敗について、Slack の `Error Message` に要約文とコマンドパスが出る。内側の原因は `Text` として扱われる。
- **AC-15**: 依存検証の失敗について、Slack の `Error Message` に要約文と解決済みのコマンドパスが出る。内側の原因（依存ライブラリのパスを含む）は `Text` として扱われる。
- **AC-16**: 2 つの group が失敗し、一方の group 名が機密を示す語を含むとき、最終の実行エラーのレコードの `error_message` に、両方の group の名前と、構造を持つ原因の部分が出る。
- **AC-17**: コマンドが 0 以外の終了コードで失敗したとき、最終の実行エラーのレコードの `error_message` に、group 名・コマンド名・終了コードが出る。
- **AC-28**: 作業ディレクトリが相対パスで拒否され、group 名・コマンド名・パスが機密を示す語を含むとき（例: パス `keycloak/data`）、group 実行前段の失敗の Slack の `Error Message` に、要約文・名前・パスが出る。コマンドの作業ディレクトリの場合も同じである。
- **AC-29**: 一時ディレクトリの作成に失敗し、そのパスが機密を示す語を含むとき（例: group 名 `token-rotate` から作られるパス）、Slack の `Error Message` に要約文とパスが出る。`*fs.PathError` の `Op`・`Err` は `Text` として扱われる。
- **AC-30**: 作業ディレクトリの解決以外の経路で生じた `*fs.PathError` は、1 つの `Text` の部分として扱われる。
- **AC-31**: 実行全体の中断と group の失敗が重なり、失敗した group の名前が機密を示す語を含むとき、最終の実行エラーのレコードの `error_message` に、中断の原因の文言と、失敗した group の宣言された部分（group 名など）が出る。
- **AC-34**: global の展開で未定義の変数 `api_key` を参照したとき、Slack の `Error Message` に、`Failed to expand global configuration` と変数名 `api_key` が出る。対象 4 で `Err` へ付け替える 4 か所の `PreExecutionError.Detail()` の文言は、変更前と同じである。

#### F-004: 既存の出力の維持

**Acceptance Criteria**:
- **AC-18**: 対象のエラーの `Error()`・`PreExecutionError.Detail()` の文言は、変更前と同じである。`errors.Is`・`errors.AsType` は、変更前と同じ原因に届く。
- **AC-19**: 最終報告の stderr の `Details:` の文言は、変更前と同じである（redaction を通らない）。
- **AC-32**: 実行全体の中断と group の失敗が重なったとき、`executeGroups` が返すエラーの `Error()` の文言は、変更前の `errors.Join(ctxErr, err)` と同じである。`errors.Is(err, ctx.Err())` が成り立ち、`errors.Is`・`errors.AsType` は失敗した group の原因に届く。中断時に返すエラーが含む group の失敗は、変更前と同じである。
- **AC-20**: `RedactingHandler` の後段のハンドラが受け取る `error_message` は、文字列である。
- **AC-21**: `RedactingHandler` を通らないハンドラに渡った構造化メッセージは、redaction 前の描画結果（AC-18 の文言）の文字列として出力される。
- **AC-22**: Slack の `Error Message` の値は、0172 の補間契約（1 行・制御文字なし・500 byte 上限）を満たす。500 byte を超える本文でも、先頭の段階の要約文が残る。
- **AC-23**: 通知の件数・`message_type`・`error_type`・Scope・Slack のフィールド構成は、変更前と同じである。

#### F-005: 宣言の保証

**Acceptance Criteria**:
- **AC-24**: 本番コードで `Constant` の部分を作る箇所は、定数式だけを渡している。定数式でない値を渡すコードがあると、AST ガードのテストが失敗する。
- **AC-25**: 本番コードの `RedactingHandler`・通知ビルダー・ログ出力に、部分の文字列の内容を見て役割を選ぶ分岐がない。
- **AC-33**: 本番コードの構造化メッセージの組み立てに、`Unwrap() []error` を持つかどうかで複合エラーの子を個別に扱う分岐がない。中断時の最終エラーは専用の型で宣言される。
- **AC-35**: 値を含めて作られた `PreExecutionError.Message` は `Text` として扱われる。この `Message` に値全体置換だけが反応する入力では、`Message` の部分が `[REDACTED]` になる。

#### F-006: 文書

**Acceptance Criteria**:
- **AC-26**: `docs/dev/architecture_design/security-architecture.ja.md` に、構造化メッセージの役割ごとの redaction、`Path` 役を値全体置換の対象外とする保護の境界、構造を持たないエラーが `Text` として扱われることが記載されている。英語版は日本語版と同じ内容である。

#### F-007: 全体の健全性

**Acceptance Criteria**:
- **AC-27**: `make test`・`make lint` が通る。

### テストの入力についての制約

- 層ごとの効果を確かめるテストは、1 つの層だけが反応する入力を使う。値全体置換だけが反応する入力（例: 変数名 `api_key`、group 名 `monkey-test`）と、値形式の検出だけが反応する入力（例: GitHub トークン形式の値）を分けて用意する。そのうえで、他の層だけでは入力が変わらないことを先に確かめる。
- AC-12〜AC-17・AC-28・AC-29・AC-31・AC-34 は、group executor から、`RedactingHandler` を通った後のレコードまでを通すテストで確かめる。Slack へ届くレコード（group 実行前段の失敗）では、Slack のメッセージ組み立てまでを通す。

## Success Criteria（要件レベル）

- group 実行前段の失敗の Slack 通知と、最終の実行エラーのログ（実行全体の中断と重なった場合を含む）で、機密ではない group 名・コマンド名・変数名・コマンドパス・作業ディレクトリのパスのために本文全体が `[REDACTED]` にならない。
- 現在マスクされている秘密（key=value、`Bearer`・`Basic` の次の語、値形式の検出、属性名による判定、構造を持たない自由文の値全体置換）は、引き続きマスクされる。
- 役割は型で宣言され、文字列の内容から推測されない。構造を持たないエラーは、現状と同じ保護を受ける。
- `Error()` の文言、stderr の文言、通知の種別と構成は変わらない。
