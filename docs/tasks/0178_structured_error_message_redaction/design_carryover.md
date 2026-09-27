# 01_requirements.md から移した詳細（設計・計画への引き継ぎ）

本書は設計書ではない。[01_requirements.md](01_requirements.md) から移した詳細（実装箇所の一覧、役割の割り当ての表、置換の細則、例、テストとの対応）を集めたものであり、`02_architecture.md` と `03_implementation_plan.md` を書くときの入力として使う。本書には承認の状態がない。本書の内容と 01 の要件が食い違う場合は、01 に従う。

本書で「旧 AC-NN」と書く識別子は、01 で使われなくなった識別子である（01 の受け入れ基準の末尾の一覧を参照）。これらの場面は、01 の AC-41（不変条件）と代表的なシナリオの AC で確かめる対象に含まれる。

## 02_architecture.md へ

### 対象の実装箇所

01 の対象 4 は、エラーの family として書かれている。次は、01 の作成時点で family に当たると確認した箇所である。AC-41 の検証の対象とする関数の集合は、02 で確定する。以下は網羅を保証するものではない。

- `PreExecutionError`（段階の要約文と原因）と `ExecutionError`（要約文と外側の context）
- `GroupStageError`・`GroupErrors`・`GroupError`・`CommandExecutionError`
- `group_executor.go` が `fmt.Errorf` で付加するエラー書式（group 名・コマンド名・コマンドパス・index を含むもの）と、コマンドの終了コードのエラー書式（`ErrCommandFailed` をラップするもの）
  - group の展開の `failed to expand group[%s]`
  - group の作業ディレクトリの解決の `failed to resolve work directory`
  - コマンドの作業ディレクトリの解決の `failed to resolve workdir: %w`（`group_executor.go`）
  - コマンドの事前展開の `failed to pre-expand commands for group[%s]: command[%s] (index %d)`
  - ディレクトリ権限監査の `for group[%s]`
  - コマンドのパス解決の `for %q`
  - 依存検証の `for %q`
  - group のファイル検証は、エラー書式を付加しない。
  - 防御用の `errUnhandledCheckSkipReason` の `… for path %s` は、設定の入力からは到達できない。役割の割り当ての表に従い、パスを `Path` とする。
- `config.ErrUndefinedVariableDetail`
- `ErrUndefinedVariableDetail` を `vars` の展開の経路で運ぶ中間のエラー書式。`config.ExpandGroup` の `failed to process group[%s] vars: %w`、`config.ExpandGlobal` の `failed to process global vars: %w`、`config.ExpandCommand` の `failed to process command[%s] vars: %w`（コマンドの準備の経路）の 3 つ。group 名・コマンド名は `Identifier`、固定の文言は `Constant` とする。
- コマンドの展開の経路で原因をラップする `config.ExpandCommand` の `failed to create RuntimeCommand for command[%s]: %w`（`expansion.go`）。コマンド名は `Identifier` とする。
- 作業ディレクトリの解決の失敗。group・コマンドの両方について、次の 3 種類。
  - 変数の展開の失敗（`config.ExpandWorkDir` の `failed to expand workdir: %w`）
  - 相対パスの拒否（`config.ExpandWorkDir` の `ErrInvalidWorkDir`。group・コマンドの名前と展開後のパスを含む）
  - 一時ディレクトリの作成・権限設定の失敗（`executor.DefaultTempDirManager.Create`。`failed to create temporary directory: %w` と `failed to set permissions on temporary directory: %w`（`tempdir_manager.go`）。OS のエラー `*fs.PathError` がパスを持つ）
- 実行全体の中断と group の失敗が重なったときの最終エラー。現在は `executeGroups` が `errors.Join(ctxErr, err)` で返している。これを、中断であることを宣言する専用の型に置き換える。
- `cmd/runner/main.go` で、原因を `fmt.Sprintf("…: %v", err)` で `Message` に埋め込んでいる次の 4 か所。`Message` を固定の文言にし、原因は `Err` で運ぶ。
  - global の展開の失敗（`Failed to expand global configuration`）
  - テンプレート検証の失敗（`Template validation failed`）
  - ディレクトリ権限チェッカーの初期化の失敗（`directory permission checker initialisation failed`）
  - `--groups` の指定誤り（`Invalid groups specified`）

### 対象のエラーの役割の割り当て

| 対象 | `Identifier` | `Path` | `Text` |
|---|---|---|---|
| `group_executor.go` のエラー書式 | group 名・コマンド名 | 展開済みのコマンドパス・解決済みのコマンドパス | ラップした原因（構造を持たなければ） |
| `GroupError`・`CommandExecutionError`・`ExecutionError` の外側の context | group 名・コマンド名 | — | ラップした原因（同上） |
| 終了コードのエラー書式 | コマンド名 | — | — |
| `vars` の中間のエラー書式（`failed to process group[%s] vars: %w`・`failed to process global vars: %w`・`failed to process command[%s] vars: %w`） | group 名・コマンド名 | — | ラップした原因（構造を持たなければ） |
| `config.ErrUndefinedVariableDetail` | 参照された変数名（`VariableName`）・展開経路（`Chain`）の各変数名・`Level` に含まれる group・コマンドの名前・`Field` に含まれる定義側の変数名（`vars.<name>`・`vars.<name>[<index>]` の `<name>`） | — | 生のテンプレート（`Context`） |
| 作業ディレクトリの相対パスの拒否 | group・コマンドの名前 | 展開後のパス | — |
| 一時ディレクトリの作成・権限設定の失敗 | — | `*fs.PathError` の `Path` | `*fs.PathError` の `Op`・`Err` |
| 中断時の最終エラー | — | — | 中断の原因（`ctx.Err()`）。失敗した group のエラーは、その型の宣言に従う |

- 表に無い固定の文言は `Constant` とする。
- 数値（index・終了コード・件数）の役割は、設計で決める。ただし `Constant` にできるのは定数式だけである。
- 生のテンプレートは、秘密が直書きされうるので `Text` とする。

### `ErrUndefinedVariableDetail` の `Field` の部分

`Field` は、現在は 1 本の文字列である。コードが作る形は `vars.<name>`・`vars.<name>[<index>]`（`ProcessVars` から呼ばれる `expandVarsWithLazyResolution`）、`cmd`・`args[<index>]`・`env`・`workdir`・`verify_files[<index>]`・`cmd_allowed[<index>]` である。

- 固定のキーの文言（`vars.`・`cmd`・`args`・`env`・`workdir`・`verify_files`・`cmd_allowed`・`[`・`]`）は `Constant` とし、定数式から作る。
- 利用者が定義した変数名（`vars.<name>` の `<name>`）は `Identifier` とする。
- `Field` の中の index は、数値の規則（上の表の下の箇条）に従う。
- 部分は、`Field` を組み立てるコード（`ProcessVars` の経路をはじめ、`Field` を作る各箇所）が組み立てるときに宣言する。組み立て済みの `Field` の文字列を後から解析して分けることはしない。

### 作業ディレクトリの解決の失敗の宣言

- 相対パスの拒否のエラーは、group・コマンドの名前を `Identifier`、展開後のパスを `Path` として宣言する。名前は現在、`group[<name>]`・`command[<name>]` の形の文字列（`level`）として渡されている。この文字列の中の名前も `Identifier` として宣言する。
- 一時ディレクトリの作成・権限設定の失敗では、OS のエラー `*fs.PathError` の `Path` を `Path` として宣言する。`Op`（`mkdir` など）と `Err`（`no space left on device` など）は `Text` とする。`*fs.PathError` を部分に分けるのは、型（`errors.AsType[*fs.PathError]`）によってであり、文言の解析によらない。
- この扱いは作業ディレクトリの解決の失敗に限り、他の経路の `*fs.PathError` は、構造を持たないエラーとして 1 つの `Text` の部分のままとする（旧 AC-30 の内容。01 の AC-09・AC-10 で確かめる）。

### 役割による分岐の仕組み

- 役割による分岐の既定の分岐（`default`）は、`Text` の全段を適用する。
- 設計で、どの役割にも当たらない値を作れないようにする（非公開のフィールドと構築関数など）場合は、02 にその保証を記す。そのうえで、範囲外の値を入力するテスト（AC-38）の代わりに、その保証を固定するテストを置く。

### 部分の境界をまたぐ置換の細則

01 の契約（決定事項「属性全体と部分の境界に効く保護」）を満たす手段は設計で決める。01 から移した細則と例は次のとおりである。

- 以下「全体の検出範囲」は、redaction 前の描画結果（AC-18 の文言）全体に `RedactText` を適用したときに置き換えられる範囲を指す。対象の検出の種類は、key=value、`Bearer `・`Basic ` の次の語、`Authorization` のヘッダ値、値形式の検出（bearer トークン・PEM ブロック・AWS キー・GitHub トークンなど）である。
- 範囲がすべて 1 つの `Identifier` の部分の中にあるものは、マスクしない。
- それ以外の範囲では、`Identifier` 以外の部分（`Constant`・`Path`・`Text`）のバイトを置き換え、`Identifier` の部分のバイトはそのまま出力する。範囲の中で `Identifier` 以外のバイトが連続する区間は、極大の区間ごとに 1 つの置換文字列になる。
- 例: `Identifier` の `AKIAIOSFODNN7` と `Text` の `EXAMPLE` が連結されて AWS キーの形になる場合、出力は `AKIAIOSFODNN7[REDACTED]` になる。
- 例: `Identifier` の `API_KEY`、`Constant` の `=`、`Text` の値が連結されて key=value になる場合、値はマスクされる。
- 理由: `Identifier` の部分は、`identifier.Identifier` と同じく例外なく免除する（承認済みの扱い）。そのため AC-01 は例外なく成り立つ。表示されるのは、その `Identifier` が単独で置かれた場合にも表示されるバイトだけであり、`Identifier` の外にある秘密の断片はマスクされる。

### AC-41 の検証の対象

AC-41 の検証は、対象の関数の中で原因をラップする箇所を実際に走査するテスト（例: 対象の関数の中の `%w` を含む `fmt.Errorf` を拒否する AST の検査）で行う。対象の関数の集合は 02 で確定する。上の「対象の実装箇所」の一覧は、その集合を決めるときの出発点として使う。

## 03_implementation_plan.md へ

### テストの入力の細則

01 の「テストの入力についての制約」の一般原則（1 つの層だけが反応する入力を使う）を、各 AC に当てはめた細則である。

- AC-37 は、`RedactText` の検出の種類（key=value、`Bearer `・`Basic ` の次の語、`Authorization` のヘッダ値、値形式の検出）ごとに 1 回ずつ確かめる。各入力は、接頭辞または key と値を別の部分に分ける。値形式の検出の場合の 1 つは、値形式に当たる値を `Identifier` の部分と `Text` の部分に分ける。どの入力でも、各部分だけに `RedactText` と値全体置換を適用しても秘密が見えたまま残ることを先に確かめる。
- `vars` の経路を通るシナリオ（AC-12・AC-34）の端から端までのテストは、`vars` の中の未定義変数を使う。定義側の変数名（`Field` の中の名前）が語を含む場合は、その名前以外の本文だけでは値全体置換が起きないことを先に確かめる。

### サイトごとの確認場面

01 で使われなくなった AC の場面を、テストの候補として残す。各サイトの宣言の網羅は AC-41 で確かめる。次の場面は、AC-41 の検証を補う端から端までのテストの候補である。

| 場面 | 通るエラー書式 | 旧 AC |
|---|---|---|
| 未定義の変数 `api_key` を参照する group の展開の失敗。Slack の `Error Message` に、段階の要約文・group 名・変数名 `api_key` が出る。生のテンプレートの部分は `Text` | `failed to expand group[%s]`、`failed to process group[%s] vars: %w` | AC-12（01 では group 名も語を含むシナリオに改めた） |
| 名前が語を含む group（例: `token-rotate`）でディレクトリ権限の違反を検出。要約文と `for group[%s]` の group 名が出て、本文全体は置換されない | ディレクトリ権限監査の `for group[%s]` | 旧 AC-13 |
| コマンドパスが語を含むコマンド（例: `ssh-keygen`）のパス解決の失敗。要約文とコマンドパスが出る。内側の原因は `Text` | コマンドのパス解決の `for %q` | 旧 AC-14 |
| 依存検証の失敗。要約文と解決済みのコマンドパスが出る。内側の原因（依存ライブラリのパスを含む）は `Text` | 依存検証の `for %q` | 旧 AC-15 |
| コマンドが 0 以外の終了コードで失敗。最終の実行エラーの `error_message` に group 名・コマンド名・終了コードが出る | 終了コードのエラー書式 | 旧 AC-17（01 では AC-16 のシナリオに含めた） |
| 作業ディレクトリが相対パスで拒否され、group 名・コマンド名・パスが語を含む（例: `keycloak/data`）。要約文・名前・パスが出る。コマンドの作業ディレクトリの場合も同じ | `failed to resolve work directory`（group）、`failed to pre-expand commands for group[%s]: command[%s] (index %d)`・`failed to resolve workdir: %w`（コマンド）、`ErrInvalidWorkDir` | 旧 AC-28 |
| 一時ディレクトリの作成に失敗し、パスが語を含む（例: group 名 `token-rotate` から作られるパス）。要約文とパスが出る。`*fs.PathError` の `Op`・`Err` は `Text` | `failed to resolve work directory`、`failed to create temporary directory: %w`（権限設定の `failed to set permissions on temporary directory: %w` も同じ） | 旧 AC-29 |
| 作業ディレクトリの解決以外の経路で生じた `*fs.PathError` は 1 つの `Text` の部分 | — | 旧 AC-30 |
| global の展開で未定義の変数 `api_key` を参照。Slack の `Error Message` に `Failed to expand global configuration` と変数名 `api_key` が出る。`Err` へ付け替える 4 か所の `Detail()` の文言は変更前と同じ（AC-18） | `failed to process global vars: %w` | AC-34（01 ではシナリオとして改めた） |
| group の `vars` の展開の失敗で、定義する変数の名前が語を含み（例: `token_file`）、その値が語を含まない未定義の変数（例: `dir`）を参照。`vars.token_file` が置き換えられずに出る | `failed to process group[%s] vars: %w`、`Field` の `vars.<name>` | 旧 AC-40（01 では AC-12 のシナリオに含めた） |

### 旧 AC の統合

- 旧 AC-05（`Constant` の部分は語を含んでも書き換えられず、全体の検出範囲に含まれるバイトだけがマスクされる）、旧 AC-06（境界をまたぐ key=value）、旧 AC-39（`Identifier` と `Text` にまたがる値形式）は、01 の AC-37 に統合した。上の「テストの入力の細則」の AC-37 の入力が、これらの場面を含む。
