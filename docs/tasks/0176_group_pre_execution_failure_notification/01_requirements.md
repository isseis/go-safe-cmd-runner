# 要件定義書: group 実行前段の失敗の Slack 通知

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-24 |
| Review date | 2026-09-25 |
| Reviewer | isseis |
| Comments | 2026-09-25 追記（editorial correction、決定の変更なし）: 用語集への「ラップする」登録（Task 0176）に合わせ、エラーのラップを表す「包む」「包み」を「ラップする」「ラップ」に置き換えた。 |

## 関連 Issue

- [#1152](https://github.com/isseis/go-safe-cmd-runner/issues/1152) group 実行中の失敗（command 依存検証・パス解決を含む）を Slack へ通知する経路がない
- 派生元: Task 0175 [`02_architecture.md`](../0175_group_verification_error_failed_files/02_architecture.md) §5.5「command 依存検証・パス解決の失敗は Slack に届かない」、§9

## 背景

### Slack へ届く group の失敗は 2 種類だけ

[`DefaultGroupExecutor.ExecuteGroup`](../../../internal/runner/group_executor.go) で group が失敗したとき、Slack に届くのは次の 2 つだけである。

| 失敗 | Slack への経路 |
|---|---|
| group ファイル検証の失敗（`*verification.Error`） | [`Runner.executeGroups`](../../../internal/runner/runner.go) が `pre_execution_error`（`group_file_verification_failed`）として通知する |
| コマンド実行の失敗（`executeAllCommands` がエラーを返す） | `executionResult` が設定されるため、deferred の `command_group_summary`（status `error`）が通知する |

それ以外の失敗は `executeGroups` で `groupErrs` に積まれる。[`cmd/runner/main.go`](../../../cmd/runner/main.go) がこれを `logging.ExecutionError` でラップし、[`HandleExecutionError`](../../../internal/logging/pre_execution_error.go) は `slack_notify=false` 固定で記録する。`executionResult` も未設定のため `command_group_summary` も出ない。つまり Slack には何も届かない。

### 無通知になる失敗

`ExecuteGroup` の中で、コマンド実行が始まる前に失敗しうる箇所は次のとおりである。いずれも Slack へは届かない。

| # | 段階 | 発生箇所 | 返すエラー |
|---|---|---|---|
| 1 | group の展開 | `config.ExpandGroup` | `failed to expand group[%s]: %w` |
| 2 | group の作業ディレクトリ解決 | `resolveGroupWorkDir` | `failed to resolve work directory: %w` |
| 3 | コマンドの展開・コマンドの作業ディレクトリ解決 | `preExpandCommands` | `failed to pre-expand commands for group[%s]: command[%s] (index %d): ...` |
| 4 | group のディレクトリ権限監査 | `auditGroupDirPermissions` | `ErrDirPermViolation` をラップしたエラー |
| 5 | group ファイル検証のうち `*verification.Error` ではない失敗 | `VerifyGroupFiles`（`ensureHashDirectoryValidated` の `*verification.OpError` など） | 生のエラー |
| 6 | コマンドのパス解決（検証後の再解決） | `verifyGroupFiles` の `ResolvePath` | `command path resolution failed for %q: %w` |
| 7 | コマンドの依存検証（動的ライブラリ・shebang インタプリタ） | `verifyGroupFiles` の `VerifyCommandDependencies` | 生のエラー |

6・7 はコマンドレベルの検証である。グローバルと group のファイル検証の失敗は通知されるのに、同じ検証系の失敗でもコマンドレベルだけ通知されない。1〜4 は同じ根（`executeGroups` が `*verification.Error` 以外を通知しない）による無通知である。

6 の範囲には注意が要る。コマンドのパスが解決できないという通常の失敗は、6 より前に `VerifyGroupFiles` の対象収集（`collectVerificationFiles`）で検出され、`ErrGroupVerificationCollectionFailed` をラップした `*verification.Error` として返る。これは既存の `group_file_verification_failed`（group スコープ、`failed_file_paths` に解決できなかったコマンドを載せる）で通知済みであり、本タスクでは変えない（対象外、AC-12）。6 は、対象収集での解決が成功したのに `verifyGroupFiles` での再解決が失敗した場合（検証と再解決の間にファイルが消えた・差し替えられたなど）に限られる。

### 実害

- 依存ライブラリやインタプリタの改ざん・差し替えで検証が失敗しても、Slack を監視する運用者は気付かない。検証失敗はセキュリティ上の信号であり、通知されるべき事象である。
- ディレクトリ権限の違反（4）も同様にセキュリティ上の信号だが、group レベルでは通知されない。起動時のグローバルな権限監査の違反は `pre_execution_error`（`file_access_failed`）として通知されるため、group レベルとで非対称になっている。
- 設定の誤り（1〜3）で group が 1 件もコマンドを実行せずに終わっても、成功通知も失敗通知も来ない。「通知が来ない」ことを「正常に完了した」ことと区別できない。

### 0172 の通知種別との関係

Task 0172 は `message_type` を本番で発火する 3 種別（`command_group_summary`・`pre_execution_error`・`user_group_command_failure`）に絞った。`error_type` は `pre_execution_error` 種別の中の値であり、`message_type` とは別の軸である。`pre_execution_error` に新しい `error_type` を足しても、通知種別の定義（`message_type`・ビルダー・優先度・フィールド集合）は変わらない。

### 標準出力の `RUN_SUMMARY` 行

`HandlePreExecutionError` は通知レコードを記録するだけでなく、stderr への報告と stdout への `RUN_SUMMARY` 行の出力も行う（`handleErrorCommon`）。group の途中で失敗を通知するためにこれを呼ぶと、実行の途中に `RUN_SUMMARY` 行が出る。さらに、最後に `main.go` が `HandleExecutionError` を呼ぶとき 2 行目が出る。group ファイル検証の通知経路はすでにこの形で途中に `RUN_SUMMARY` 行を出しているが、本タスクで同じ形の経路を増やすと、`RUN_SUMMARY` 行を解析する監視系から見た行数と意味がさらに崩れる。

## 目的

- `ExecuteGroup` がコマンド実行を始める前に失敗したとき、その失敗を Slack へ通知する。
- 通知から、どの group・どのコマンドの、どの段階で失敗したかを判別できるようにする。
- 失敗の段階は型で宣言し、エラー文字列の内容から推測しない。
- 同じ失敗を二重に通知しない。既に通知されている失敗（group ファイル検証・コマンド実行）の通知は変えない。
- 通知種別の定義（`message_type`・ビルダー・フィールド集合）を変えない。

## スコープ

### 対象

1. 背景の表の 1〜7 の失敗を、group ごとに発生時点で Slack へ通知する。
2. 通知には `pre_execution_error` 種別を使い、段階を表す `error_type` を新たに定義する（「決定事項」参照）。
3. 通知の Scope は、失敗がコマンドに帰属するとき（3・6・7）コマンドスコープ（`group=<group> command=<command>`）、それ以外（1・2・4・5）は group スコープとする。
4. `Error Message` には失敗の原因（パス解決・依存検証ではコマンドのパスと理由）を載せる。
5. 失敗の段階は group executor が構造化されたエラー型で宣言し、`executeGroups` はその型から通知を組み立てる。エラー文字列を検査して段階を選ばない。
6. 新しい通知経路は、stdout の `RUN_SUMMARY` 行と stderr のエラー報告を増やさない。プロセスの終了コードと、最後に `main.go` が行う `ExecutionError` の報告は変えない。
7. 利用者向け文書（`docs/user/runner_command.ja.md` の「通知設定」）に、group 実行前段の失敗が `pre_execution_error` で通知されることと、新しい `error_type` を追記する。日本語版を先に更新し、英語版は `/mktrans` で反映する。

### 対象外

- **group ファイル検証の失敗（`*verification.Error`）の通知。** 0175 で整えた経路をそのまま使い、本タスクでは変えない。
- **コマンド実行の失敗の通知。** `command_group_summary` と `user_group_command_failure`（0174）が扱う。本タスクの通知経路はコマンド実行開始後の失敗には発火しない。
- **通知種別（`message_type`）・Slack フィールド集合・共通エンベロープの変更。** `pre_execution_error` のビルダーとフィールド（`Error Message`・`Component` など）をそのまま使う。
- **redaction と表示安全な補間契約の変更。** 既存の `error_message` の redaction と 0172 の補間契約（1 行・制御文字なし・500 byte 上限）をそのまま使う。
- **`*verification.OpError` など失敗対象一覧を持たない検証失敗の報告形式の統一。** 本タスクは対象 5 を通知経路に乗せるだけで、本文の形式を global の報告と揃える改善は [#1154](https://github.com/isseis/go-safe-cmd-runner/issues/1154) で扱う。
- **`Runner.executeGroups` が先頭のエラーしか返さない点。** [#1153](https://github.com/isseis/go-safe-cmd-runner/issues/1153) で扱う。本タスクの通知は group ごとに発生時点で出るため、2 件目以降の group の失敗も Slack には届くようになるが、戻り値の扱いは変えない。
- **group ファイル検証の通知経路が実行途中に `RUN_SUMMARY` 行を出す点、および group ファイル検証が失敗しても他に失敗がなければ終了コードが 0 になる点。** どちらも既存の挙動であり、本タスクでは変えない。別 issue として起票する候補とする。
- **dry-run。** dry-run 時は Slack へ送信しない既存の挙動のままとする。
- **コンテキストのキャンセル（`context.Canceled`・`context.DeadlineExceeded`）。** 通知の対象としない。現状どおり `executeGroups` が即座に返す。

## 決定事項

### 通知種別は `pre_execution_error` を使う

新しい `message_type` を足さず、`pre_execution_error` を使う。理由は次のとおり。

- 対象の失敗はいずれも group のコマンドが 1 件も実行される前に起きる。「group の実行前エラー」として `pre_execution_error` の意味に収まる。
- `pre_execution_error` のビルダーは `error_type`・`Error Message`・`Component` を表示し、失敗の原因を運ぶ枠がすでにある。
- `command_group_summary` はコマンド結果の件数と所要時間を表示する種別で、エラー本文を表示するフィールドを持たない。0 件のコマンドで status `error` を送っても、原因は通知に載らない。
- `ExecutionError` に通知種別を持たせる案は、コマンド実行後の失敗と実行前の失敗を同じ型で扱うことになり、どちらの通知を出すかを型の外で判断する必要が生じる。また 0172 の通知種別定義の改訂が要る。

### 段階は `error_type` で区別する

段階ごとに `error_type` を定義する。受け手が通知の見出し（`error_type`）だけで「設定の誤り」か「検証の失敗」か「権限の違反」かを区別できるようにするためである。

| `error_type` | 対象の段階（背景の表の #） | Scope |
|---|---|---|
| `group_preparation_failed`（新設） | 1 group の展開、2 group の作業ディレクトリ解決 | group |
| `group_preparation_failed`（新設） | 3 コマンドの展開・作業ディレクトリ解決 | コマンド |
| `group_dir_permission_violation`（新設） | 4 group のディレクトリ権限監査 | group |
| `group_file_verification_failed`（既存） | 5 `*verification.Error` 以外の group ファイル検証失敗 | group |
| `command_verification_failed`（新設） | 6 コマンドのパス解決（検証後の再解決）、7 コマンドの依存検証 | コマンド |
| `group_pre_execution_failed`（新設、汎用） | 段階不明（ゼロ値・未知の値。次節参照） | group |

`error_type` の名前は設計段階で変えてよいが、段階と `error_type` の対応（汎用の `error_type` を含む）は 1 つの表（コード上の 1 箇所）で定義する。

### 段階は型で宣言する

group executor は、失敗した段階を列挙型のフィールドに持つ構造化エラーを返す。`executeGroups` はそのフィールドで `error_type` と Scope を選ぶ。エラー文字列やセンチネルのラップの仕方（`strings.Contains`、文字列の接頭辞）から段階を推測しない（CLAUDE.md「Declare, don't infer」）。

列挙型のゼロ値と未知の値は「段階不明」として扱い、通知を落とさずに汎用の `error_type` で通知する（fail secure）。段階の宣言を忘れた失敗が無通知に戻ることを防ぐためである。

### 通知は 1 つの失敗につき 1 回

- 対象の失敗 1 件につき、Slack 通知はちょうど 1 件である。
- group ファイル検証の失敗（`*verification.Error`）は既存の経路で 1 件だけ通知し、本タスクの経路では通知しない。
- コマンド実行開始後の失敗は `command_group_summary` だけが通知し、本タスクの経路では通知しない。
- `main.go` の最後の `ExecutionError` は従来どおり `slack_notify=false` のままとし、Slack に 2 件目を送らない。

### 新しい経路は `RUN_SUMMARY` 行と stderr 報告を増やさない

本タスクの通知は Slack 通知のための構造化ログレコードを記録するだけにする。stdout の `RUN_SUMMARY` 行と stderr の `Error:` ブロックは出さない。これらはプロセスの最後に `main.go` が `ExecutionError` として 1 回だけ出す（現状どおり）。実行途中に `RUN_SUMMARY` 行を出す既存の group ファイル検証の経路は本タスクでは変えない（対象外）。

### 本文は既存の redaction と補間契約を通す

`Error Message` の本文は、`pre_execution_error` の `error_message` 属性として記録し、既存の `RedactingHandler` と 0172 の表示安全な補間契約（1 行、制御文字と書式制御文字を含まない、500 byte 上限）を通す。これらのエラー文字列は現状でも stderr と構造化ログに出ているが、本タスクで Slack という新しい出力先に届く。redaction の適用範囲は変えず、`error_message` として記録することで既存の保護を受ける。

## 受け入れ基準（Acceptance Criteria）

#### F-001: group 実行前段の失敗を通知する

**Acceptance Criteria**:
- **AC-01**: group の展開（`ExpandGroup`）が失敗したとき、`pre_execution_error` の通知レコードが 1 件記録され、`error_type` は `group_preparation_failed`、Scope は `group=<group>` である。
- **AC-02**: group の作業ディレクトリ解決が失敗したとき、AC-01 と同じ `error_type` と Scope で通知レコードが 1 件記録される。
- **AC-03**: コマンドの展開、またはコマンドの作業ディレクトリ解決が失敗したとき、`error_type` が `group_preparation_failed`、Scope が `group=<group> command=<command>` の通知レコードが 1 件記録される。
- **AC-04**: group のディレクトリ権限監査で違反が検出されたとき、`error_type` が `group_dir_permission_violation`、Scope が `group=<group>` の通知レコードが 1 件記録される。
- **AC-05**: group ファイル検証が `*verification.Error` 以外のエラーで失敗したとき、`error_type` が `group_file_verification_failed`、Scope が `group=<group>` の通知レコードが 1 件記録される。
- **AC-06**: `verifyGroupFiles` でのコマンドのパスの再解決が失敗したとき（対象収集での解決失敗は AC-12 の既存経路）、`error_type` が `command_verification_failed`、Scope が `group=<group> command=<command>` の通知レコードが 1 件記録され、`Error Message` に解決できなかったコマンドが現れる。
- **AC-07**: コマンドの依存検証（動的ライブラリ・shebang インタプリタ）が失敗したとき、`error_type` が `command_verification_failed`、Scope が `group=<group> command=<command>` の通知レコードが 1 件記録され、`Error Message` に依存検証に失敗したコマンドのパスと失敗の理由の両方が現れる。
- **AC-08**: 複数の group がそれぞれ AC-01〜AC-07 のいずれかで失敗したとき、失敗した group ごとに 1 件ずつ通知レコードが記録される（先頭の group だけではない）。

#### F-002: 段階を型で宣言する

**Acceptance Criteria**:
- **AC-09**: `executeGroups` が `error_type` と Scope を選ぶ根拠は、group executor が返す構造化エラーの列挙型フィールドである。本番コードにエラー文字列を検査して段階を選ぶ分岐がない。
- **AC-10**: 段階が未設定（ゼロ値）または未知の値の構造化エラーを受け取ったとき、通知は落ちずに汎用の `error_type`（決定事項の表の段階不明の行）と group スコープで 1 件記録される。
- **AC-11**: 背景の表の 1〜7 の各発生箇所が、それぞれ AC-01〜AC-07 の段階を宣言したエラーを返す。

#### F-003: 二重通知をしない・既存挙動を変えない

**Acceptance Criteria**:
- **AC-12**: group ファイル検証の失敗（`*verification.Error`）では、通知レコードは従来の `group_file_verification_failed` の 1 件だけで、本文・`failed_file_paths`・Scope は 0175 の挙動から変わらない。
- **AC-13**: コマンド実行開始後の失敗では、本タスクの通知レコードは記録されず、既存の通知（`command_group_summary`（status `error`）、および `run_as_user`/`run_as_group` 付きコマンドの失敗では `user_group_command_failure`）だけが従来どおり記録される。
- **AC-14**: `context.Canceled` / `context.DeadlineExceeded` では本タスクの通知レコードは記録されない。
- **AC-15**: 本タスクの通知経路は stdout に `RUN_SUMMARY` 行を出さず、stderr に `Error:` ブロックを出さない。AC-01〜AC-07 の失敗が 1 件だけ起きた実行で、stdout の `RUN_SUMMARY` 行は従来どおり 1 行である。
- **AC-16**: AC-01〜AC-07 の失敗が起きた実行のプロセス終了コードと、`main.go` が最後に行う `ExecutionError` の報告（`slack_notify=false`）は変わらない。
- **AC-17**: `message_type` の登録（3 種別）、`pre_execution_error` のビルダーとフィールド集合、共通エンベロープは変わらない。

#### F-004: 本文の安全性

**Acceptance Criteria**:
- **AC-18**: 本タスクの通知の `Error Message` は既存の表示安全な補間契約の性質（1 行、制御文字と書式制御文字を含まない、上限を超えない）を保つ。改行や制御文字を含むコマンドパスで失敗させても、この性質が保たれる。
- **AC-19**: 本文は `error_message` 属性として記録され、既存の redaction を受ける。エラー文字列に値形式の機密（トークン等）が含まれる場合、Slack へ送るペイロードではマスクされる。

#### F-005: 文書

**Acceptance Criteria**:
- **AC-20**: `docs/user/runner_command.ja.md` の「通知設定」に、group 実行前段の失敗が `pre_execution_error` として通知されること、新しい `error_type` とその意味、Scope の表示が記載され、英語版 `docs/user/runner_command.md` に `/mktrans` で反映されている。

#### F-006: 全体の健全性

**Acceptance Criteria**:
- **AC-21**: 各コミットの時点で `make fmt`（Go を変更した場合）・`make test`・`make lint` が通る。
- **AC-22**: 追加・変更したテストが、検証対象の挙動を壊すと失敗することを確認し、その旨をコミットメッセージに記す（CLAUDE.md「Every test must be able to fail for its stated reason」）。

## Success Criteria（要件レベル）

- group のコマンドが 1 件も実行されずに終わった失敗は、段階を問わず Slack に通知される。
- 通知の `error_type` と Scope から、どの group・コマンドのどの段階で失敗したかを判別できる。
- コマンドレベルの検証（検証後のパス再解決・依存ライブラリ・インタプリタ）の失敗が、ファイル検証の失敗と同じく通知される。
- 同じ失敗が二重に通知されず、既に通知されている失敗の通知内容は変わらない。
- 通知種別の定義・`RUN_SUMMARY` 行・終了コードは変わらない。
