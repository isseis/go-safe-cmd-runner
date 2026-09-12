# 要件定義書: `user_group_command_failure` 通知の配線と run-as コマンド失敗時の監査記録

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-12 |
| Review date | `-` |
| Reviewer | `-` |
| Comments | `-` |

## 関連 Issue

- なし（Task 0172 [`03_implementation_plan.md`](../0172_slack_notification_message_unification/03_implementation_plan.md) §10「次のステップ」の follow-up から派生）

## 背景

### 0172 が導入した通知が本番で発火しない

Task 0172 は Phase 5 で `user_group_command_failure` 通知種別を追加し、[`Logger.LogUserGroupExecution`](../../../internal/runner/base/audit/logger.go) の失敗分岐（`result.ExitCode != 0`）が `NotificationAttrs(UserGroupCommandFailureNotification(), common.CommandScope(...))` を記録するようにした。ところが Phase 7 の実 Slack 表示確認では、`run_as_user = "root"` の失敗コマンド（exit 2）を実行しても本通知は Slack へ届かず、送信されたのはグループ集計通知（`command_group_summary`）の 1 件だけであった（[`03_implementation_plan.md`](../0172_slack_notification_message_unification/03_implementation_plan.md) §10）。

### 原因: `executeWithUserGroup` の早期 return

[`DefaultExecutor.executeWithUserGroup`](../../../internal/runner/base/executor/executor.go) は、[`runCommand`](../../../internal/runner/base/executor/command_lifecycle.go) の結果に対して `if err != nil` で早期リターンする。`runCommand` は子プロセスが非ゼロで終了すると [`superviseCommand`](../../../internal/runner/base/executor/command_lifecycle.go) から非 nil のエラーを返すため、この分岐に入る。`AuditLogger.LogUserGroupExecution` の呼び出しはその後にあり、`err == nil` のときにしか到達しない。

一方 `LogUserGroupExecution` の失敗レコード（`slog.LevelError`、"User/group command failed"）は `result.ExitCode != 0` のときだけ書かれる。したがって `executeWithUserGroup` 経由では次が成立する。

- 子プロセスが実際に開始して非ゼロ終了しても、`user_group_execution` の監査レコードは書かれない。
- その結果 `user_group_command_failure` 通知も発生しない。

### 実害

1. **監査証跡の欠落。** run-as（`run_as_user`／`run_as_group`）で実行したコマンドの失敗が監査ログに残らない。成功は `audit_type=user_group_execution` の INFO レコードとして残るため、監査上は成功だけが起きたように見える。この早期リターンは 0172 以前から存在し、0172 の回帰ではない。
2. **通知が到達不能。** 0172 の F-001 は「本番で発火するのは 3 種別」を前提とし、AC-23 は `user_group_command_failure` のメッセージを定める。しかし `user_group_command_failure` は本番では到達不能であり、AC-23 を実チャンネルで検証できない。Phase 7 は本種別だけ実 Slack 表示確認ができず、ペイロードの正しさを `TestSlackHandler_UserGroupCommandFailure` でしか担保できなかった。

### 一度も開始していない失敗との区別

`executeWithUserGroup` の失敗経路には、コマンドが一度も開始していない場合も含まれる。

- 事前検証・`prepareCommand`・`runCommand` の開始失敗は `ExitCodeUnknown`（`-1`）の `Result` を伴って返る。子プロセスは開始していない。
- 昇格の起動区間を開けなかった場合は `Result` 自体を返さない。

一方、子プロセスが開始した後でタイムアウトやシグナルにより強制終了した場合も、`os.ProcessState.ExitCode()` は `-1`（`ExitCodeUnknown`）を返す（[`group_executor_timeout_test.go`](../../../internal/runner/group_executor_timeout_test.go) がこの値を固定している）。したがって `ExitCodeUnknown` でないことを開始の目印にすると、開始済みのコマンドを開始失敗と誤判定し、監査レコードと通知を落とす。

これらを実行の失敗として監査すると、実際には走っていない実行の `exit_code` を記録することになり、監査の意味が壊れる。配線は子プロセスが開始した失敗だけを対象にし、その区別は実行コンテキストの明示的な状態で宣言する。エラー文字列の内容でも `ExitCode` の値でも判定しない（CLAUDE.md「Declare, don't infer」）。

## 目的

- run-as コマンドが開始して失敗した場合（非ゼロ終了のほか、タイムアウトやシグナルによる強制終了を含む）に、`LogUserGroupExecution` の失敗レコードを監査ログへ書き、`user_group_command_failure` 通知を送る。
- 一度も開始していない失敗を、実行の失敗として監査しない。
- 成功経路の挙動、0172 の [`02_architecture.md`](../0172_slack_notification_message_unification/02_architecture.md) §3.4・§3.5 が定める通知種別定義と共通エンベロープ、メッセージ書式、`Result` の意味を変えない。

## スコープ

### 対象

1. `executeWithUserGroup` の開始後の失敗経路で、子プロセスが開始した場合に `AuditLogger.LogUserGroupExecution` を呼ぶ。
2. 呼び出しには実際の `Result`（`ExitCode`・`Stdout`・`Stderr`）と収集済みの `PrivilegeMetrics` を渡し、`common.CommandScope(cmd.GroupName(), cmd.Name())` の通知コンテキストを保つ。
3. 子プロセスが開始したことの判別を、実行コンテキストの明示的な状態（開始したか、およびその終了の種別）で宣言的に行う。`ExitCode` の値だけによる推測や文字列の内容による推測を新たに足さない。
4. `executeWithUserGroup` を通した経路で失敗の監査・通知が発火することを固定するテスト、および開始しなかった失敗では発火しないことを固定するテストを追加する。
5. Task 0172 の [`03_implementation_plan.md`](../0172_slack_notification_message_unification/03_implementation_plan.md) §10 の該当 follow-up が本タスクで解消されたことを、履歴として追記する。

### 対象外

- **通知種別定義・共通エンベロープ・メッセージ書式の変更。** 0172 の決定を維持する。
- **終了コードの値と `ExitCodeUnknown` の意味の変更。** 開始の判別は `ExitCode` の値ではなく実行コンテキストの状態で行い、終了コードが表す値そのものは変えない。
- **タイムアウト・kill の実行そのものの挙動の変更。** 強制終了の条件やシグナルは変えない。本タスクが変えるのは、開始済みの強制終了を監査・通知の対象に含めることだけである。
- **新たな通知種別・監査属性の追加。**
- **group 検証エラー通知の失敗ファイル名表示。** 別タスク（0175）で扱う。
- **0172 の承認済み要件・設計文書の改訂。** 本タスクは実装漏れの解消であり、0172 側の文書は履歴として残す。

## 決定事項

### 開始したことの判別を宣言的に行う

監査するかどうかを `ExitCode` の値だけで決めない。`os.ProcessState.ExitCode()` は、シグナルやタイムアウトで終了した子プロセスに対して `-1`（`ExitCodeUnknown`）を返す。そのため `ExitCodeUnknown` でないことを開始の目印にすると、実際には開始したもののシグナルで終了したコマンドを「開始していない」と誤判定し、監査レコードと通知を落とす。

判別は実行コンテキストの明示的な状態（開始したか、およびその終了の種別）で行う。少なくとも次を区別する。

- 子プロセスが一度も開始していない失敗（事前検証・`prepareCommand`・開始失敗、昇格の起動区間を開けなかった場合）は、開始済みとして監査しない。
- 子プロセスが開始した失敗は、終了ステータスが確定しているか（通常の終了か、シグナル・タイムアウトによる強制終了か）にかかわらず監査する。

エラー文字列の `strings.Contains` などの内容判定も、`ExitCode` の値だけによる推測も使わない。判別の根拠を型・値として宣言し、`switch` の `default` が安全側へ倒れる形にする。

### 監査は実行 1 回につき 1 件、失敗と成功で排他

`executeWithUserGroup` の 1 回の実行につき、監査レコードはちょうど 1 件とする。成功（exit 0）は INFO の成功レコード、開始後の失敗（非ゼロ終了およびタイムアウト・シグナルによる強制終了）は ERROR の失敗レコードとし、両方を書かない。`LogUserGroupExecution` が `ExitCode` で分岐する既存の実装をそのまま使う。

### 通知の内容は 0172 の定義を変えない

失敗時に `LogUserGroupExecution` が載せる `user_group_command_failure` の属性と通知コンテキスト、および記録する stdout／stderr は 0172 のままとする。本タスクはその分岐に到達できるようにすることだけを行う。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 開始した run-as コマンドの失敗を監査・通知する

**Acceptance Criteria**:
- **AC-01**: run-as コマンドが開始して失敗したとき（非ゼロ終了、またはタイムアウト・シグナルによる強制終了）、監査ログに `audit_type=user_group_execution` の ERROR レコードが 1 件書かれる。通常終了では `exit_code` がその終了コードを、強制終了では `ExitCodeUnknown`（`-1`）を示す。
- **AC-02**: 同じ実行で `user_group_command_failure` の通知が 1 件発生し、通知コンテキストが `CommandScope(group, command)` である。
- **AC-03**: 失敗レコードに stdout と stderr が含まれ、既存の redaction を通った値である。
- **AC-04**: 成功（exit 0）の run-as コマンドは、従来どおり INFO の成功レコード 1 件だけで、`user_group_command_failure` 通知を発生させない。
- **AC-05**: コマンドが一度も開始していない失敗（事前検証・`prepareCommand`・開始失敗、または昇格の起動区間を開けなかった場合）は、`user_group_execution` の失敗レコードも `user_group_command_failure` 通知も発生させない。

#### F-002: 配線をテストで固定する

**Acceptance Criteria**:
- **AC-06**: `executeWithUserGroup` を通した開始済みの失敗を検証するテストがあり、監査の失敗レコードと `user_group_command_failure` 通知の発火を観測する。非ゼロ終了に加え、タイムアウト・シグナルによる強制終了のケースを含める。
- **AC-07**: `executeWithUserGroup` を通した開始しなかった失敗を検証するテストがあり、監査の失敗レコードも `user_group_command_failure` 通知も発火しないことを観測する。

#### F-003: 既存の挙動を変えない

**Acceptance Criteria**:
- **AC-08**: 成功経路の INFO レコードのレベル・属性（`audit_type`・`command_name`・`exit_code` など）が変わらない。
- **AC-09**: `user_group_command_failure` の通知種別定義と Slack ペイロードが変わらない（0172 の `TestSlackHandler_UserGroupCommandFailure` が引き続き通る）。

#### F-004: 全体の健全性

**Acceptance Criteria**:
- **AC-10**: 各コミットの時点で `make fmt`（Go を変更した場合）・`make test`・`make lint` が通る。
- **AC-11**: 追加・変更したテストが、検証対象の挙動を壊すと失敗することを確認し、その旨をコミットメッセージに記す（CLAUDE.md「Every test must be able to fail for its stated reason」）。

## Success Criteria（要件レベル）

- 本番で run-as の失敗が監査ログに残り、Slack に `user_group_command_failure` 通知が届く。
- 一度も開始していないコマンドは実行として監査されない。
- 通知書式・通知種別定義・成功経路の挙動が変わらない。
