# 実装計画書: Slack 通知メッセージの書式統一とスコープ情報の付与

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-08 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 1. 実装概要

### 1.1 目的

本番で発火しない 3 つの通知種別を削除したうえで、残る 3 種別に通知コンテキスト（発生箇所を表す型）を付与し、Text 行・添付色・末尾フィールドを 1 つの規則へ統一する。設計の詳細は [02_architecture.md](02_architecture.md) を参照し、本書では作業の分割と検証手段だけを記す。

### 1.2 実装原則

1. 削除を先に行い、統一の対象を実際に発火する 3 種別へ絞る（[02_architecture.md §8.2](02_architecture.md#82-実装順の根拠)）。
2. 削除は種別ごとに 1 コミットとし、1 件ずつ revert できる形にする。
3. 型の伝搬（Phase 4）と表示の変更（Phase 5）を別のコミットに分け、発火元の契約と Slack 表示を別々に検証する。
4. Go のコメント・識別子・文字列リテラルはすべて英語で書く。本計画書の説明文だけが日本語である。
5. 各 Phase の完了時に `make fmt`（Go 変更時）、`make test`、`make lint` を通す。Phase 1〜3 と Phase 7 では `make deadcode` も実行する。削除コミットは 1 件ずつ revert できる必要があるため、3 つの削除 Phase それぞれが単独で AC-08 を満たす。

### 1.3 既存コード調査結果

実装前に対象パッケージを調査した結果は次のとおりである。行番号は変化するため、以後の手順では検索パターンで場所を示す。

#### 削除対象 3 種別の全出現箇所

| 種別 | production コードの出現箇所 | テストの出現箇所 |
|---|---|---|
| `privileged_command_failure` | `internal/logging/slack_sender.go` の定数 `messageTypePrivilegedCommandFailure`、`internal/logging/slack_handler.go` の `Handle` 分岐と `buildPrivilegedCommandFailure`、`internal/common/logschema.go` の `PrivilegedCommandFailureAttrs` | `internal/logging/slack_handler_test.go` の該当ケース 1 件 |
| `security_alert` | `internal/logging/slack_sender.go` の定数 `messageTypeSecurityAlert` と `isHighPriority` のエントリ、`internal/logging/slack_handler.go` の `Handle` 分岐と `buildSecurityAlert`、`internal/common/logschema.go` の `SecurityAlertAttrs`・`SeverityCritical`・`SeverityHigh`、`internal/runner/base/audit/logger.go` の `LogSecurityEvent` | `internal/runner/base/audit/logger_test.go` の `TestLogger_LogSecurityEvent`、`TestLogSecurityEvent_Masking`、`TestLogSecurityEvent_DetailsRedaction`、`TestLogSecurityEvent_DetailsKeyCollisionPrevention`、`internal/logging/slack_sender_test.go` の 3 箇所、`internal/logging/slack_handler_test.go` の該当ケース |
| `privilege_escalation_failure` | `internal/logging/slack_sender.go` の定数 `messageTypePrivilegeEscalationFail` と `isHighPriority` のエントリ、`internal/logging/slack_handler.go` の `Handle` 分岐と `buildPrivilegeEscalationFailure`、`internal/common/logschema.go` の `PrivilegeEscalationFailureAttrs`、`internal/runner/base/audit/logger.go` の `LogPrivilegeEscalation` | `internal/runner/base/audit/logger_test.go` の `TestLogger_LogPrivilegeEscalation`、`TestLogPrivilegeEscalation_Masking`、`internal/logging/slack_handler_test.go` の該当ケース |

削除に伴って古くなるコメントが 2 箇所ある。`internal/logging/slack_sender.go` のキュー容量の根拠を述べたコメント（`more than 32 security alerts in one run` を含む行）と、同ファイルの定数ブロック冒頭のコメント（`the queue-priority decision below and Handle's message builder switch must agree` を含む段落）である。前者は Phase 2 で書き換える。後者は Phase 5.1 で定数ブロックごと削除されるが、Phase 1 の削除時点で既に不正確になるため、Phase 2 で存続する種別だけを述べる文へ直し、Phase 5.1 で削除する。`isHighPriority` の doc コメントは関数ごと削除される。

`internal/runner/runerrors` の `ErrorSeverityCritical` は `common.SeverityCritical` とは別の型であり、削除対象ではない。検索時に混同しないよう、`common.` 修飾子を含めて検索する。

`docs/tasks/0068_separate_slack_webhooks/` と `docs/tasks/0163_redaction_coverage_and_slack_async/` にも 3 種別の名前が出現するが、これらは当時の設計を記録した過去の文書であり、書き換えない。

#### 通知コンテキストの伝搬先

`PreExecutionError` の複合リテラルは production コードに 17 箇所ある。内訳は `cmd/runner/main.go` が 11 箇所、`internal/runner/bootstrap/config.go` が 4 箇所、`internal/runner/bootstrap/environment.go` が 2 箇所であり、[02_architecture.md §3.2](02_architecture.md#32-preexecutionerror-と発火元) の記述と一致する。`HandlePreExecutionError` の呼び出しは 6 箇所（`cmd/runner/main.go` に 5 箇所、`internal/runner/runner.go` に 1 箇所）で、いずれも 4 個の位置引数を渡している。

`NewRuntimeCommand` は既に `groupName` を引数で受け取っているが保持していない。production コードの呼び出しは `internal/runner/config/expansion.go` の 1 箇所だけであり、シグネチャ変更は不要である。`RuntimeCommand` には既に非公開フィールド `timeout` があるため、非公開の `groupName` を足す形は既存の構造と整合する。`&runnertypes.RuntimeCommand{...}` の複合リテラルは production コードには無く、テストヘルパー 3 箇所（`internal/runner/base/risk/test_helpers.go` に 1 箇所、`internal/runner/base/executor/testutil/helpers.go` に 2 箇所）と `_test.go` に 36 箇所ある。いずれもグループ名を設定しないため、Phase 4.4 の変更後は `GroupName()` が空文字を返す。空文字のまま `common.CommandScope("", name)` を作ると [02_architecture.md §3.1](02_architecture.md#31-通知コンテキスト) の表で不正なスコープになる。

調査の結果、`audit.Logger.LogUserGroupExecution` を呼ぶテストはすべて `internal/runner/base/executor/testutil/helpers.go` の `CreateRuntimeCommand` と `CreateRuntimeCommandFromSpec` を経由しており、`_test.go` の 36 箇所の複合リテラルは監査ロガーの経路に到達しない。したがって対処はこの 2 個のヘルパーに限定でき、Phase 4.4 で両ヘルパーを `NewRuntimeCommand` 経由の構築へ移す。非公開フィールドは別パッケージのヘルパーからは設定できないため、複合リテラルのままでは対処できない。

`common.GroupSummaryAttrs.Group` の参照は production 2 箇所（`internal/logging/slack_handler.go`、`internal/runner/runner.go`）とテスト 5 箇所（`internal/logging/slack_handler_test.go` に 2 箇所、`internal/logging/slack_sender_test.go`、`internal/runner/integration_command_results_test.go`、`internal/redaction/redactor_test.go` に各 1 箇所）である。フィールド削除により後者 5 箇所のコンパイルが壊れるため、Phase 5 で機械的に更新する。

#### 再利用できる既存の仕組み

- **静的契約テストの土台**: 本リポジトリには Go の構文木を走査する guard テストの前例がある。`internal/testutil/synccensus/census_guard_test.go` は `internal/` と `cmd/` を `filepath.WalkDir` で走査して production 宣言を集める形であり、本タスクの静的契約テストはこの構成をそのまま踏襲する。ファイル一覧の取得には `internal/testutil/identitymutationguard` の `ProductionGoFiles` を再利用でき、import 解決には同パッケージの `ResolveLocalImports` を使える。新しい構文木走査の枠組みを作る必要はない。
- **送信失敗ロガーへの WARN**: `slackSender` は既に `failureLogger` を所有し、`warnNotDelivered` が `message_type`・`run_id`・`level`・`reason` を載せた WARN を出している。スキーマ違反の WARN はこれと同じ `failureLogger` を使う新しいメソッドとして `internal/logging/slack_sender.go` に追加する。ロガーの所有権は移さない。`warnNotDelivered` の doc コメントが述べるとおり、通知を識別する属性（`message_type`・`run_id`・`level`）の組を 1 箇所に保つのが同ファイルの方針であるため、新しいメソッドはこの 3 属性を独自に並べ直さず、`warnNotDelivered` と共通の属性生成を経由する。
- **記録用ロガー**: `internal/testutil`（パッケージ名は `tu`）の `tu.NewRecordingLogger` と `tu.LogRecorder`（`RecordsAtLevel`、`FindRecords`）が既にあり、WARN の件数と属性の検証に使える。新しいモックは作らない。
- **表示定数**: `internal/logging/slack_handler.go` の `colorGood` / `colorWarning` / `colorDanger`、`emojiSuccess` / `emojiWarning` / `emojiFailure`、`fieldTitleHostname` / `fieldTitleRunID`、`outputMaxLength` / `stderrMaxLength` / `truncationSuffix` はそのまま使う。`emojiAlert` は削除対象 3 種別に加えて存続する `buildPreExecutionError` でも使われており、5.4 の書き換え後に未使用となる。
- **ホスト名**: `common.GetHostname` を引き続き使う。

#### 不足しているもの

- Slack 向けのエスケープ関数は存在しない。`&`・`<`・`>` だけを entity へ変換する関数を `internal/logging` に新設する。`html.EscapeString` は `"` と `'` も変換するため使わない。
- `logElevationOutcome` が出す 2 つの Info レコードを検証するテストが存在しない。AC-05 の根拠となるテストを Phase 3 で新設する。`newPlatformManager` は `Manager` インタフェースを返し、`logElevationOutcome` は `*UnixPrivilegeManager` の非公開メソッドであるため、この関数経由では呼び出せない。テストは同一パッケージ内にあるので `&UnixPrivilegeManager{logger: rec}` を直接組み立てる。`newPlatformManager` は内部で `isPrivilegeExecutionSupported` を呼び、root 実行時に Info を 1 件出すため、この関数を通すとレコード件数の検証も汚染される。

  **承認済みアーキテクチャとの差異**: [02_architecture.md §7.1](02_architecture.md#71-単体テスト) は AC-05 を「`privilege.logElevationOutcome` の native root と `seteuid` の**既存テスト**が残り」で検証するとしているが、調査の結果そのような既存テストは存在しない。本計画では新規テストを追加する形へ改める。承認済み文書の検証前提を変える差異であるため、実装計画のレビュー時にこの点を明示的に確認し、必要なら 02_architecture.md §7.1 と §8.1 を改訂して再承認する。
- `slackRequest` は優先度を持たない。Phase 5 で確定済み優先度のフィールドを追加する。

## 2. 実装ステップ

Phase の名前と順序は [02_architecture.md §8.1](02_architecture.md#81-フェーズ分割) の分割に従う。

### Phase 1: `privileged_command_failure` の削除

**対象ファイル**: `internal/logging/slack_sender.go`、`internal/logging/slack_handler.go`、`internal/common/logschema.go`、`internal/logging/slack_handler_test.go`

**作業内容**:

- [ ] `internal/logging/slack_sender.go` の定数 `messageTypePrivilegedCommandFailure` を削除する。
- [ ] `internal/logging/slack_handler.go` の `Handle` から `case messageTypePrivilegedCommandFailure:` の分岐を削除する。
- [ ] `internal/logging/slack_handler.go` の `buildPrivilegedCommandFailure` を doc コメントごと削除する。
- [ ] `internal/common/logschema.go` の `PrivilegedCommandFailureAttrs` を doc コメント（`Write side not yet implemented` の行を含む）ごと削除する。
- [ ] `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` から `messageType: "privileged_command_failure"` のケースを削除する。

**完了条件**:

- [ ] `rg -n -e 'privileged_command_failure' -e 'PrivilegedCommandFailure' cmd/ internal/` の一致が 0 件である。
- [ ] 削除の直前と直後に `go tool cover -func` を取得し、存続する関数のカバレッジが下がっていないことを確認し、その結果をコミットメッセージへ記す。
- [ ] `make deadcode` が新たな到達不能コードを報告しない。
- [ ] `make fmt`、`make test`、`make lint` が通る。
- [ ] この Phase だけで 1 コミットとし、単独で revert できる。

### Phase 2: `security_alert` の削除

**対象ファイル**: `internal/logging/slack_sender.go`、`internal/logging/slack_handler.go`、`internal/common/logschema.go`、`internal/runner/base/audit/logger.go`、`internal/logging/slack_sender_test.go`、`internal/logging/slack_handler_test.go`、`internal/runner/base/audit/logger_test.go`

**作業内容**:

- [ ] `internal/logging/slack_sender.go` の定数 `messageTypeSecurityAlert` を削除する。
- [ ] `internal/logging/slack_sender.go` の `isHighPriority` の `case` から `messageTypeSecurityAlert` を除く。
- [ ] `internal/logging/slack_sender.go` の `isHighPriority` の doc コメントを、存続する高優先度種別だけを述べる文へ書き換える。変更後の 2 行目以降は `// queue. Pre-execution errors must not be pushed out by a flood of ordinary` / `// command notifications.` とする。
- [ ] `internal/logging/slack_sender.go` の定数ブロック冒頭のコメントを、存続する種別だけを述べる文へ直す。変更前の `// Message types carried by slack_notify records. They are named here because` / `// the queue-priority decision below and Handle's message builder switch must` / `// agree on the exact strings.` を、`// Message types carried by slack_notify records. They are named here because` / `// the queue-priority decision below and Handle's message builder switch must` / `// agree on the exact strings. The set shrinks to the types with a production` / `// writer; Phase 5 replaces this block with the notification definitions.` に置き換える。この段落は Phase 1 の削除時点で既に不正確になっている。
- [ ] `internal/logging/slack_sender.go` のキュー容量の根拠コメントから security alert への言及を除く。変更前の `// the judgement that more than 32 security alerts in one run is already an` / `// incident, where the count alone is enough.` を、`// the judgement that more than 32 high-priority notifications in one run is` / `// already an incident, where the count alone is enough.` に置き換える。
- [ ] `internal/logging/slack_handler.go` の `Handle` から `case messageTypeSecurityAlert:` の分岐を削除する。
- [ ] `internal/logging/slack_handler.go` の `buildSecurityAlert` を doc コメントごと削除する。
- [ ] `internal/common/logschema.go` の `SecurityAlertAttrs` を doc コメントごと削除する。
- [ ] `internal/common/logschema.go` の `SecuritySeverity` 定数ブロック（`SeverityCritical` と `SeverityHigh`）をコメントごと削除する。`internal/runner/runerrors` の `ErrorSeverityCritical` には触れない。
- [ ] `internal/runner/base/audit/logger.go` の `LogSecurityEvent` を doc コメントごと削除する。削除後に未使用となる import があれば併せて除く。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogger_LogSecurityEvent` を削除する。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogSecurityEvent_Masking` を削除する。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogSecurityEvent_DetailsRedaction` を削除する。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogSecurityEvent_DetailsKeyCollisionPrevention` を削除する。
- [ ] `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` から `security_alert` のケースを削除する。
- [ ] `internal/logging/slack_sender_test.go` の共有ヘルパー `securityAlertRecord(eventType string)` を `preExecutionErrorRecord(errorType string)` へ書き換える。`message_type` を `messageTypePreExecutionError` にし、`common.SecurityAlertAttrs.EventType` と `common.SecurityAlertAttrs.Severity`／`common.SeverityCritical` の参照を `common.PreExecErrorAttrs.ErrorType` へ置き換える。この 3 個の参照は Phase 2 で削除される定義を指しているため、識別子の差し替えでは足りずヘルパーごと書き直す。
- [ ] 上記ヘルパーの呼び出し元 3 箇所（`TestSlackSender_HighPriorityBypassesFullNormalQueue`、`TestSlackSender_QueueOverflowDropsAndRecords`、`TestSlackSender_FlushLogsMessageTypeBreakdown`）を新しいヘルパー名と期待値へ合わせる。
- [ ] `TestSlackSender_HighPriorityBypassesFullNormalQueue` の順序検証が使う識別子を差し替える。現在は `assert.Contains(t, texts[1], "intrusion", ...)` が `buildSecurityAlert` の出力する `event_type` に依存している。`buildPreExecutionError` は `error_type` を出力するため、ヘルパーへ渡す `errorType` の値を識別子とし、その値がペイロードに現れることで送信順を判定する形へ変える。識別子を持たない検証（件数だけ、順序だけ）に退化させない。

**完了条件**:

- [ ] `rg -n -e 'security_alert' -e 'SecurityAlertAttrs' -e 'LogSecurityEvent' -e 'buildSecurityAlert' -e 'messageTypeSecurityAlert' cmd/ internal/` の一致が 0 件である。
- [ ] `rg -n --pcre2 'common\.Severity(Critical|High)' cmd/ internal/` の一致が 0 件である。参照側を含めて確認する。この形なら `internal/runner/runerrors` の `ErrorSeverityCritical` は一致しない。
- [ ] `TestSlackSender_HighPriorityBypassesFullNormalQueue` で `isHighPriority` を常に `false` を返すよう一時的に変えると失敗することを確認し、その結果をコミットメッセージへ記す。
- [ ] 削除の直前と直後の `go tool cover -func` を比較し、`internal/runner/base/audit` と `internal/logging` の存続する関数のカバレッジが下がっていないことをコミットメッセージへ記す。`LogSecurityEvent` の削除で `internal/runner/base/audit/logger.go` の `optStr` など他の関数の被覆が落ちる場合は、落ちた関数名と補うテストを併記する。
- [ ] `make deadcode` が新たな到達不能コードを報告しない。
- [ ] `make fmt`、`make test`、`make lint` が通る。
- [ ] この Phase だけで 1 コミットとし、単独で revert できる。

### Phase 3: `privilege_escalation_failure` の削除

**対象ファイル**: `internal/logging/slack_sender.go`、`internal/logging/slack_handler.go`、`internal/common/logschema.go`、`internal/runner/base/audit/logger.go`、`internal/runner/base/audit/logger_test.go`、`internal/logging/slack_handler_test.go`、`internal/runner/base/privilege/unix_privilege_test.go`

**作業内容**:

- [ ] `internal/logging/slack_sender.go` の定数 `messageTypePrivilegeEscalationFail` を削除する。
- [ ] `internal/logging/slack_sender.go` の `isHighPriority` の `case` から `messageTypePrivilegeEscalationFail` を除き、残りが `messageTypePreExecutionError` だけになることを確認する。Phase 2 で書き換えた doc コメントとの整合を保つ。
- [ ] `internal/logging/slack_handler.go` の `Handle` から `case messageTypePrivilegeEscalationFail:` の分岐を削除する。
- [ ] `internal/logging/slack_handler.go` の `buildPrivilegeEscalationFailure` を doc コメントごと削除する。
- [ ] `internal/common/logschema.go` の `PrivilegeEscalationFailureAttrs` を doc コメントごと削除する。
- [ ] `internal/runner/base/audit/logger.go` の `LogPrivilegeEscalation` を doc コメントごと削除する。削除後に未使用となる import があれば併せて除く。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogger_LogPrivilegeEscalation` を削除する。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogPrivilegeEscalation_Masking` を削除する。
- [ ] `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` から `privilege_escalation_failure` のケースを削除する。
- [ ] `internal/runner/base/privilege/unix_privilege_test.go` に `TestLogElevationOutcome_RecordsElevationModes` を追加する。次の 3 点を守る。
  - `tu.NewRecordingLogger` で作ったロガーを持つ `&UnixPrivilegeManager{...}` を直接組み立てる。`newPlatformManager` は `Manager` インタフェースを返すため非公開メソッドへ到達できず、さらに内部の `isPrivilegeExecutionSupported` が root 実行時に Info を 1 件出してレコード件数の検証を汚すため、この関数は使わない。
  - `logElevationOutcome` を直接呼ぶだけでなく、既存の `unix_privilege_test.go` が `execCtx.elevation` を検証している経路（`escalatePrivileges` とその呼び出し元）を通して記録が出ることを検証する。関数本体だけを対象にすると、呼び出し元から `logElevationOutcome` の呼び出しを削っても失敗しない。AC-05 が求めるのは「記録し続けること」であり、呼び出し点を含めた経路が検証対象である。
  - `elevationNativeRoot` と `elevationSeteuid` で Info レコードがそれぞれ 1 件記録され `operation` と `command` の属性を持つこと、`elevationNone` では 0 件であることを検証する。
- [ ] 上記のテストに `t.Parallel()` を書かない。`internal/runner/base/privilege/unix_privilege_test.go` の冒頭コメントが、プロセス全体の識別情報を共有するため同パッケージのどのテストも `t.Parallel()` を呼んではならないと定めている。

**完了条件**:

- [ ] `rg -n -e 'privilege_escalation_failure' -e 'PrivilegeEscalationFailureAttrs' -e 'LogPrivilegeEscalation' -e 'buildPrivilegeEscalationFailure' -e 'messageTypePrivilegeEscalationFail' cmd/ internal/` の一致が 0 件である。
- [ ] `TestLogElevationOutcome_RecordsElevationModes` が、呼び出し元から `logElevationOutcome` の呼び出しを一時的に削ると失敗することを確認し、その結果をコミットメッセージへ記す。本体を空にする形だけでは呼び出し点の削除を検知できないため、注入対象は呼び出し点とする。
- [ ] 削除の直前と直後の `go tool cover -func` を比較し、存続する関数のカバレッジが下がっていないことをコミットメッセージへ記す。
- [ ] `make deadcode` が新たな到達不能コードを報告しない。
- [ ] `make fmt`、`make test`、`make lint` が通る。
- [ ] この Phase だけで 1 コミットとし、単独で revert できる。

### Phase 4: 通知コンテキストの追加と全発火元への伝搬

設計は [02_architecture.md §3.1](02_architecture.md#31-通知コンテキスト)、[§3.2](02_architecture.md#32-preexecutionerror-と発火元)、[§3.3](02_architecture.md#33-runtimecommand-のグループ名) を参照する。この Phase では表示の変更を行わず、型と属性の伝搬だけを行う。

**対象ファイル**: `internal/common/notification_context.go`（新規）、`internal/common/notification_context_test.go`（新規）、`internal/logging/pre_execution_error.go`、`internal/runner/base/runnertypes/runtime.go`、`internal/runner/base/audit/logger.go`、`internal/runner/runner.go`、`cmd/runner/main.go`、`internal/runner/bootstrap/config.go`、`internal/runner/bootstrap/environment.go`、および対応する各テストファイル

**4.1 通知コンテキスト型の追加**

- [ ] `internal/common/notification_context.go` を新規作成し、`NotificationScope`（`ScopeUnknown`／`ScopeGlobal`／`ScopeGroup`／`ScopeCommand`）、非公開フィールドを持つ `NotificationContext`、コンストラクタ `GlobalScope` / `GroupScope` / `CommandScope`、参照メソッド `Scope` / `GroupName` / `CommandName` を定義する。
- [ ] 同ファイルに `NotificationScope` の `String()` と、文字列からスコープへ戻す関数を定義する。両者が参照する対応表は 1 つだけ置き、`String()` は `unknown` / `global` / `group` / `command` の 4 語を返し、復元側は `switch` の `default` を `ScopeUnknown` へ倒す。
- [ ] 同ファイルに `LogValue() slog.Value` を実装する。出力は [02_architecture.md §3.1](02_architecture.md#31-通知コンテキスト) の固定エンコード表のとおり、`scope` と `group` を常に出し、`command` は空でないときだけ出す。
- [ ] 同ファイルに、レコード上で通知コンテキストを載せる属性キーの定数を定義する。

**4.2 通知コンテキスト型のテスト**

- [ ] `internal/common/notification_context_test.go` に `TestNotificationContext_ConstructorsAndZeroValue` を追加する。ゼロ値の `Scope()` が `ScopeUnknown` であること、`GlobalScope()` が `ScopeGlobal` かつ group と command が空であること、`GroupScope("backup")` と `CommandScope("backup", "pg_dump")` が渡した値を返すことを検証する。
- [ ] `internal/common/notification_context_test.go` に `TestNotificationContext_LogValueEncoding` を追加する。各スコープについて `LogValue()` が `slog.KindGroup` を返し、`scope` と `group` が常に含まれ、command が空のときだけ `command` 属性が無いことを検証する。
- [ ] `internal/common/notification_context_test.go` に `TestNotificationScope_StringRoundTrip` を追加する。4 つのスコープすべてについて `String()` の結果を復元関数へ通すと元のスコープへ戻ること、未知の語が `ScopeUnknown` になることを検証する。

**4.3 `PreExecutionError` の構造体化**

- [ ] `internal/logging/pre_execution_error.go` の `PreExecutionError` に `NotificationContext common.NotificationContext` フィールドを追加する。
- [ ] `HandlePreExecutionError` のシグネチャを `func HandlePreExecutionError(preExecErr *PreExecutionError)` に変える。本文には `preExecErr.Detail()`、Run ID には `preExecErr.RunID`、種別とコンポーネントには `preExecErr.Type` と `preExecErr.Component` を使う（[02_architecture.md §3.2](02_architecture.md#32-preexecutionerror-と発火元) の表）。
- [ ] `handleErrorCommon` の `errorHandlingParams` から `slogMsgType` と `slackNotify` を除き、代わりに `[]slog.Attr` を受け取る形へ変える。`error_type`・`error_message`・`component`・`run_id` の 4 属性は `handleErrorCommon` が引き続き構築し、呼び出し元から渡された属性をその後ろへ追加する。呼び出し元へ移すのは `slack_notify` と `message_type` の 2 つだけである。`error_type` と `error_message` と `component` は `buildPreExecutionError` が読むため、ここから落とすと Slack の種別固有フィールドが空になる。`HandlePreExecutionError` は `slack_notify`・`message_type`・通知コンテキストを含む属性を渡し、`HandleExecutionError` は `slack_notify` を含まない自分の属性を渡す。stderr と stdout の出力形式は変えない。
- [ ] `cmd/runner/main.go` の `HandlePreExecutionError` 呼び出し 5 箇所を構造体渡しへ移行する。`reportStartupPrivilegeFailure`、`--run-id` 検証失敗、ハッシュディレクトリ検証失敗、既定分岐（`ErrorTypeSystemError`）は `GlobalScope()` を設定した新しい `PreExecutionError` を構築して渡す。`preExecErr` を受ける分岐は、報告境界で `preExecErr.RunID = runID` を代入してからそのまま渡す。
- [ ] `internal/runner/runner.go` の `executeGroups` にある検証エラーの呼び出しを構造体渡しへ移行し、`NotificationContext` に `common.GroupScope(verErr.Group)` を設定する。`errorMsg` の書式を `"Group: %s, Total: %d, Verified: %d, Failed: %d, Error: %s"` から `"Total: %d, Verified: %d, Failed: %d, Error: %s"` へ変え、`verErr.Group` を本文から除く。

  この変更により、`handleErrorCommon` が stderr へ出す `  Details: ` 行からもグループ名が消える。通知コンテキストは構造化レコードにだけ載り stderr には出ないためである。stderr と stdout の**書式**は変えないが、この 1 項目については**情報**が減る。AC-14 はグループ名の重複表示の除去を求めており、Scope が唯一の表示場所になることは設計どおりであるため、これを受け入れる。グループ名は JSON ログの通知コンテキストから引き続き参照できる。
- [ ] `cmd/runner/main.go` の `PreExecutionError` 複合リテラル 11 箇所すべてに `NotificationContext: common.GlobalScope(),` を加える。
- [ ] `internal/runner/bootstrap/config.go` の `PreExecutionError` 複合リテラル 4 箇所すべてに `NotificationContext: common.GlobalScope(),` を加える。
- [ ] `internal/runner/bootstrap/environment.go` の `PreExecutionError` 複合リテラル 2 箇所すべてに `NotificationContext: common.GlobalScope(),` を加える。

**4.4 `RuntimeCommand` のグループ名**

- [ ] `internal/runner/base/runnertypes/runtime.go` の `RuntimeCommand` に非公開フィールド `groupName string` を追加し、`NewRuntimeCommand` が引数の `groupName` をこのフィールドへ保持するようにする。シグネチャは変えない。
- [ ] 同ファイルに `func (r *RuntimeCommand) GroupName() string` を追加する。既存の `Name()` などと同じく、レシーバが nil の場合の扱いを周囲のメソッドに合わせる。
- [ ] `internal/runner/base/executor/testutil/helpers.go` の `CreateRuntimeCommand` と `CreateRuntimeCommandFromSpec` を、複合リテラルではなく `runnertypes.NewRuntimeCommand` 経由の構築へ移し、グループ名を渡す。両ヘルパーは別パッケージにあり非公開フィールドを設定できないため、複合リテラルのままではグループ名を持てない。`audit.Logger.LogUserGroupExecution` を呼ぶテストはすべてこの 2 個のヘルパーを経由するため、ここを直せば 4.5 で不正なコマンドスコープが生じない。既存の呼び出し元のシグネチャは変えず、グループ名は既定値（`CreateRuntimeCommandFromSpec` が既にタイムアウト解決へ渡している `"test-group"`）を使う。
- [ ] `internal/runner/base/runnertypes/runtime_test.go` に `TestNewRuntimeCommand_RetainsGroupName` を追加し、コンストラクタへ渡したグループ名を `GroupName()` が返すこと、空文字を渡した場合に空文字が返ることを検証する。

**4.5 監査ロガーへのコマンドスコープ付与**

- [ ] `internal/common/logschema.go` に `UserGroupCommandFailureAttrs` を追加する。フィールドは `CommandName`（`command_name`）、`ExitCode`（`exit_code`）、`Stdout`（`stdout`）、`Stderr`（`stderr`）とする。
- [ ] `internal/runner/base/audit/logger.go` の `LogUserGroupExecution` の失敗経路で、`common.CommandScope(cmd.GroupName(), cmd.Name())` を通知コンテキスト属性として加える。`additionalAttrs` の `stdout` と `stderr` の属性キーを `UserGroupCommandFailureAttrs` から引く形へ変える。`command_name` と `exit_code` は既に `baseAttrs` にあるため属性を追加せず、`baseAttrs` 側のキー指定を同じ定義から引く形へ差し替える。属性を足すと同じキーが 1 レコードに 2 回現れ、設計が `duplicate_notification_context` として不正に扱う重複と同じ状態を作ってしまう。
- [ ] `internal/runner/base/audit/logger_test.go` に `TestLogger_LogUserGroupExecution_CommandScope` を追加し、失敗レコードが `scope=command`、`group`、`command` を持つこと、成功レコードには通知コンテキストが付かないこと、および失敗レコードに同じ属性キーが 2 回現れないことを検証する。

**4.6 グループ集計への通知コンテキスト付与**

- [ ] `internal/runner/runner.go` の `logGroupExecutionSummary` に、`common.GroupScope(groupSpec.Name)` の通知コンテキスト属性を加える。この Phase では `common.GroupSummaryAttrs.Group` の属性は残したままとし、削除は Phase 5 で行う。
- [ ] `internal/runner/runner_test.go` の `TestSlackNotification` を拡張し、グループ集計レコードが `scope=group` と正しい group 名を持つことを検証する。
- [ ] `internal/runner/runner_test.go` に `TestSlackNotification_GroupVerificationErrorScope` を追加し、検証エラー経路のレコードが `scope=group` と `verErr.Group` を持ち、`error_message` 属性に `Group: ` の接頭辞を含まないことを検証する。

**4.7 既存テストの移行**

- [ ] `internal/logging/pre_execution_error_test.go` の位置引数を使う全ケース（`TestHandlePreExecutionError_AllTypes` と `TestHandlePreExecutionError_SlackNotification` の呼び出し 2 箇所）を構造体渡しへ移行する。既存の stderr／stdout の検証は維持する。
- [ ] `internal/logging/pre_execution_error_test.go` に `TestHandlePreExecutionError_EmitsNotificationContext` を追加し、`GlobalScope()` を設定した `PreExecutionError` が `scope=global`、`GroupScope("g")` を設定したものが `scope=group` かつ `group=g` のレコードを出すことを検証する。
- [ ] `cmd/runner/startup_privilege_test.go` の `TestReportStartupPrivilegeFailure_UsesValidRunID` を新しい引数形へ移行する。
- [ ] `internal/runner/base/runnertypes/runtime_test.go` の既存ケースのうち、構造体リテラルでグループ名を必要とするものがあれば `NewRuntimeCommand` 経由へ移す。非公開フィールドに依存する新しいテストヘルパーは追加しない。

**完了条件**:

- [ ] `rg -n 'PreExecutionError{' cmd/ internal/ | grep -v _test.go | wc -l` が `17` である。リテラルが `NotificationContext` を宣言していることの検証は 4.8 の静的テストが行う（複合リテラルは複数行にまたがるため、`rg` の出力行からは判定できない）。
- [ ] `make fmt`、`make test`、`make lint` が通る。
- [ ] 4.2、4.4、4.5、4.6、4.8 で追加した各テストについて、第 4.5 節の表に従って対象を一時的に壊すと失敗することを確認し、その結果をコミットメッセージへ記す。4.8 の guard テストも対象に含める。guard テストは違反が無いときに常に合格するため、意図的な違反を 1 件入れて検出されることを確認しないと、走査が空振りしていても気付けない。

**4.8 `PreExecutionError` リテラルの静的契約テスト**

- [ ] `internal/testutil/notificationguard/notification_guard_test.go` を新規作成する。`//go:build test` を付け、パッケージ名は `notificationguard` とする。`internal/testutil/synccensus/census_guard_test.go` と同じ形で `../../../internal` と `../../../cmd` を `filepath.WalkDir` で走査し、ファイル一覧の取得には `internal/testutil/identitymutationguard` の `ProductionGoFiles` を使う。
- [ ] 同ファイルに `TestPreExecutionErrorLiteralsDeclareNotificationContext` を追加する。production コードの `logging.PreExecutionError` および同パッケージ内の `PreExecutionError` の複合リテラルをすべて集め、`NotificationContext` キーを持たないものがあれば、そのファイルと位置を挙げて失敗する。走査対象が 0 件のときも失敗させ、走査が空振りしたまま合格しないようにする。

### Phase 5: 通知種別定義と共通エンベロープの導入

設計は [02_architecture.md §3.4](02_architecture.md#34-通知種別定義)、[§3.5](02_architecture.md#35-共通エンベロープ)、[§3.6](02_architecture.md#36-未知種別と不正スコープ)、[§6.1](02_architecture.md#61-slack-メッセージ構築フロー) を参照する。[02_architecture.md §8.2](02_architecture.md#82-実装順の根拠) の方針に従い、発火元とハンドラの切り替えを 1 個の取り消し可能なコミットにまとめる。

**対象ファイル**: `internal/logging/notification.go`（新規）、`internal/logging/notification_test.go`（新規）、`internal/logging/slack_handler.go`、`internal/logging/slack_sender.go`、`internal/common/logschema.go`、`internal/runner/runner.go`、`internal/runner/base/audit/logger.go`、`internal/logging/pre_execution_error.go`、および対応する各テストファイル

**5.1 通知種別定義**

- [ ] `internal/logging/notification.go` を新規作成し、`notificationPriority`（`priorityNormal` / `priorityHigh`）、`messageDetails`（`Headline` と `Fields`）、`messageBuilder`、`messageTypeDefinition`、公開トークン型 `Notification` を定義する。
- [ ] 同ファイルに非公開の登録関数を定義する。`message_type`、組み立て関数、優先度を 1 回だけ受け取って通知種別定義の集合へ加え、その要素を指す `Notification` を返す。登録 API はパッケージ外へ公開しない。
- [ ] 同ファイルの `var` 初期化で `CommandGroupSummaryNotification`、`PreExecutionErrorNotification`、`UserGroupCommandFailureNotification` の 3 個を登録する。優先度は順に通常、高、通常とする（[02_architecture.md §3.4](02_architecture.md#34-通知種別定義) の表）。
- [ ] 同ファイルに `func NotificationAttrs(notification Notification, notificationContext common.NotificationContext) []slog.Attr` を定義する。`slack_notify=true`、`message_type`、通知コンテキストの 3 属性を返す。ゼロ値の `Notification` を受け取った場合は `message_type` を空文字として返し、通知を落とさない。
- [ ] `internal/logging/slack_sender.go` の定数 `messageTypeCommandGroupSummary` と `messageTypePreExecutionError` を削除し、種別名の定義を `internal/logging/notification.go` の登録呼び出しへ一本化する。定数ブロック冒頭の doc コメント（`the queue-priority decision below and Handle's message builder switch must agree on the exact strings` を含む段落）も削除する。

**5.2 キュー優先度の一本化**

- [ ] `internal/logging/slack_sender.go` の `isHighPriority` を削除する。
- [ ] `slackRequest` に確定済み優先度のフィールドを追加し、`queueFor` が種別名ではなくそのフィールドで振り分けるようにする。
- [ ] `internal/logging/slack_sender.go` にスキーマ違反を記録するメソッドを追加する。`failureLogger` へ WARN を 1 件出し、メッセージは `Slack notification schema violation` に固定する。理由コードと列挙順は [02_architecture.md §3.6](02_architecture.md#36-未知種別と不正スコープ) の表に従う。次の 3 点を守る。
  - 通知を識別する属性（`message_type`・`run_id`・`level`）は `warnNotDelivered` と同じ生成箇所から得る。同ファイルの doc コメントがこの 3 属性を 1 箇所に保つ方針を明記しており、新しいメソッドで並べ直すと同じ並行リストが再びできる。
  - Webhook を示す属性のキーは、同ファイルの既存の記録に合わせて `webhook` とする（`webhook_label` という別名を作らない）。理由コードは複数を列挙しうるため、既存の単数形 `reason` とは別に `reasons` を使い、要素が 1 個のときも列挙とする。
  - `run_id` を渡すため、同ファイルの他の `failureLogger` 呼び出し（`warnNotDelivered` を含む 5 箇所）と同じく、行単位に絞った `//nolint:gosec // G706: run_id is an internal identifier, not user input` を付ける。`.golangci.yml` の gosec 除外は `_test.go` にしか適用されないため、これを省くと Phase 5 の `make lint` が落ちる。

**5.3 共通エンベロープと表示**

- [ ] `internal/logging/slack_handler.go` に製品名の定数 `productName = "go-safe-cmd-runner"` を 1 箇所だけ定義する。
- [ ] `internal/logging/slack_handler.go` に、ログレベルから絵文字・STATUS・添付色を返す関数を追加する。判定は [02_architecture.md §3.5](02_architecture.md#35-共通エンベロープ) の表のとおり閾値比較で行い、全レベルがいずれかに一致する全域関数にする。等値比較で書かない。
- [ ] `internal/logging/slack_handler.go` に、Slack が特別に解釈する `&`・`<`・`>` だけを entity へ変換する関数を追加する。`strings.NewReplacer` を使い、`&` を最初に列挙する。`html.EscapeString` は `"` と `'` も変換するため使わない。Text 行と添付フィールドの値のうち、group 名・command 名・要約・種別固有フィールド値に適用する。
- [ ] `internal/logging/slack_handler.go` に、レコードから通知コンテキスト属性を取り出して妥当性を判定する関数を追加する。判定は [02_architecture.md §3.6](02_architecture.md#36-未知種別と不正スコープ) の表の順（欠落 → 重複 → 不正）で行い、対応する理由コードを返す。妥当なときはスコープ表示（`"(global)"` / Go の書式で `"group=%s"` / `"group=%s command=%s"`）を、不正なときは `"(scope: invalid)"` を返す。表示文字列に日本語を入れない。
- [ ] `internal/logging/slack_handler.go` に共通エンベロープ生成を追加する。Text 行を `[<製品名>] <絵文字> *<STATUS>* — <Scope> : <要約>` の形で組み立て、添付には種別固有フィールドの後ろに Scope、Hostname、Run ID をこの順で追加する。汎用メッセージも同じ処理を通す。
- [ ] `internal/logging/slack_handler.go` の `Handle` を [02_architecture.md §6.1](02_architecture.md#61-slack-メッセージ構築フロー) のフローへ書き換える。種別の照合と通知コンテキストの妥当性確認を受付停止判定より前に置き、WARN は 1 レコードにつき 1 件だけ出す。種別固有部分と共通エンベロープの構築は受付停止判定の後に置く。ドライランと nil 送信機構の早期 return は現在の位置のまま変えない。

**5.4 種別固有の組み立て関数**

- [ ] `buildCommandGroupSummary` を、`messageDetails` を返す形へ書き換える。`Headline` は成功時が Go の書式で `"%d commands in %s"`、失敗時が `"%d commands, %d failed in %s"` である（総数、失敗数、所要時間）。失敗数と成功・失敗の判定は `extractCommandResults` が返すコマンド結果の終了コードから数える。`status` 属性は要約にも表示にも使わない。`status` で分岐すると、AC-19 が禁じる「種別ごとの裁量による表示決定」を別の形で戻すことになる。`Fields` は Command Count、Duration、各 Command と既存の Output / Error だけとし、Hostname と Run ID を含めない。`###` を組み立てる `title` の生成と `status` 属性による色分けを削除する。stdout 1000 文字・stderr 500 文字の切り詰めは維持する。
- [ ] `buildPreExecutionError` を `messageDetails` を返す形へ書き換える。`Headline` は `error_type` の値とし、`Fields` は Error Message と Component だけにする。
- [ ] `internal/logging/slack_handler.go` にユーザー／グループ指定コマンドの失敗用の組み立て関数を追加する。`Headline` は Go の書式で `"command failed (exit %d)"`（終了コード）、`Fields` は Command、Exit Code、値がある場合の Output と Error Output とし、属性キーは `common.UserGroupCommandFailureAttrs` から引く。
- [ ] `buildGenericMessage` を、レコードの `Message` を `Headline` とする `messageDetails` を返す形へ書き換える。`r.Level.String()` と Run ID を Text 行へ直接埋める現在の実装をやめ、共通エンベロープに任せる。
- [ ] `internal/logging/slack_handler.go` の定数 `emojiAlert` が未使用になっていれば削除する。

**5.5 グループ属性の置き換え**

- [ ] `internal/common/logschema.go` の `GroupSummaryAttrs` から `Group` フィールドを削除する。
- [ ] `internal/runner/runner.go` の `logGroupExecutionSummary` から `common.GroupSummaryAttrs.Group` の属性を削除し、`NotificationAttrs` が返す属性で `slack_notify`・`message_type`・通知コンテキストをまとめて渡す形へ変える。
- [ ] `internal/logging/slack_handler.go` の `buildCommandGroupSummary` から `case common.GroupSummaryAttrs.Group:` の分岐を削除する。
- [ ] `common.GroupSummaryAttrs.Group` を参照する残り 5 箇所のテスト（`internal/logging/slack_handler_test.go` に 2 箇所、`internal/logging/slack_sender_test.go`、`internal/runner/integration_command_results_test.go`、`internal/redaction/redactor_test.go` に各 1 箇所）から当該行を除く。いずれも検証している挙動は変えない。

**5.6 発火元の属性生成関数への移行**

- [ ] `internal/logging/pre_execution_error.go` の `HandlePreExecutionError` が `NotificationAttrs(PreExecutionErrorNotification, preExecErr.NotificationContext)` の結果を `handleErrorCommon` へ渡すようにする。
- [ ] `internal/runner/base/audit/logger.go` の `LogUserGroupExecution` の失敗経路が、`slack_notify` と `message_type` を直接書かず `logging.NotificationAttrs(logging.UserGroupCommandFailureNotification, ...)` を使うようにする。
- [ ] `internal/runner/runner.go` の `logGroupExecutionSummary` が `logging.NotificationAttrs(logging.CommandGroupSummaryNotification, ...)` を使うようにする。現在この関数は `[]any` を組み立てて `slog.Error` / `slog.Info` を呼んでいるが、`NotificationAttrs` は `[]slog.Attr` を返すため、呼び出しを `slog.LogAttrs` へ切り替えて `[]slog.Attr` のまま渡す。`[]any` へ詰め替えない。監査ロガーが既に `LogAttrs` を使っており、書き方をそろえられる。

**5.7 テスト**

- [ ] `internal/logging/notification_test.go` に `TestAllDefinitionsSatisfyEnvelope` を追加する。登録済みの通知種別定義の集合を `range` し、各定義について代表レコードを組み立てて次を検証する。並行リストを書かず、集合そのものを走査する。
  - `Handle` 経由のメッセージの Text 行が `[<製品名>] <絵文字> *<STATUS>* — <Scope> : <要約>` の形に一致する（正規表現で全体の形を検証し、製品名で始まることだけの部分検証にしない）。
  - メッセージのどこにも `###` が現れない。
  - 添付フィールドの末尾 3 件が Scope、Hostname、Run ID の順である。
  - 各定義の組み立て関数を**直接呼び**、返る `messageDetails.Fields` のタイトルに Scope・Hostname・Run ID が現れない。この層を分けた検証がないと、組み立て関数が重複して 3 件を出し共通エンベロープがさらに追加しても、末尾 3 件の検証は通ってしまい AC-22 を確かめられない。
- [ ] `internal/logging/notification_test.go` に `TestNotificationDefinitionsAreSingleSource` を追加する。集合の要素数が公開トークンの数と一致すること、`message_type` が一意であること、各公開トークンが指す定義に種別名・組み立て関数・優先度がそろっていることを検証する。
- [ ] `internal/logging/notification_test.go` に `TestNotificationAttrs_ZeroTokenYieldsEmptyMessageType` を追加する。ゼロ値の `Notification` を渡すと `NotificationAttrs` の戻り値が `slack_notify=true` と空文字の `message_type` になることだけを検証する（`Handle` は通さない）。空文字が汎用メッセージと WARN になる末端の挙動は `TestSlackHandler_UnknownMessageType` が空文字ケースとして担うため、ここでは重複させない。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_LevelDrivesDisplay` を追加する。DEBUG、INFO、INFO と WARN の中間値、WARN、ERROR、ERROR より上の各レベルについて絵文字・STATUS・色が表のとおりになること、`r.Level.String()` が表示へ漏れないことを検証する。同じレベルで種別を変えても表示が変わらないことも確認する。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_GenericMessageFollowsTextLineFormat` を追加する。汎用メッセージの Text 行が製品名で始まり統一書式に一致すること、`###` を含まないことを検証する。登録済み 3 種別は `TestAllDefinitionsSatisfyEnvelope` が通知種別定義の集合の走査で covering するため、ここで種別を書き写した並行リストは作らない。汎用メッセージは通知種別定義の集合の要素ではないため、この経路だけを別に検証する。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_ScopeRendering` を追加する。`GlobalScope()` が `(global)`、`GroupScope("backup")` が `group=backup`、`CommandScope("backup", "pg_dump")` が `group=backup command=pg_dump` と表示されることを検証する。
- [ ] 以下 5.7 で追加する `slack_handler_test.go` のテストは、いずれも既存の `newTestSlackHandler` でハンドラを構築する。このヘルパーは構築と同時に `t.Cleanup(func() { handler.Close() })` を登録するため、アサーションが途中で失敗してもキューとワーカーが残らない。新しい構築ヘルパーは追加しない。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_InvalidNotificationContext` を追加する。属性の欠落、同じキーの重複、`slog.KindGroup` でない値、`scope` の欠落、未知の `scope` 語、`scope=unknown`、`scope=group` で group 名が空、の各入力について、Scope 表示が `(scope: invalid)` になり、対応する理由コードの WARN が 1 件記録されることを検証する。グローバル扱いへ落ちないこと、元の種別の固有部分が保たれることも確認する。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_UnknownMessageType` を追加する。空文字と未知文字列の `message_type` について、汎用メッセージが共通エンベロープ付きで送られ、`unknown_message_type` の WARN が残ることを検証する。WARN 以上のレコードが通常キュー満杯でも高優先度キューで送られることも確認する。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_SingleWarnForCombinedViolations` を追加する。未知種別かつ通知コンテキスト欠落のレコードについて、WARN が 1 件だけ記録され、`reasons` に `unknown_message_type` と `missing_notification_context` がこの順で並ぶことを検証する。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_WarnOmitsSensitiveValues` を追加する。WARN の属性に通知本文、Webhook URL、group 名、command 名が含まれないことを検証する。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_DetailsAreBuiltAfterClosedCheck` を追加する。受付停止済みの送信機構では種別固有の組み立て関数が呼ばれず、それでも定義不備の WARN は記録されることを検証する。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_UserGroupCommandFailure` を追加する。固有の組み立て関数が使われ、汎用メッセージに落ちないこと、失敗したコマンド名・終了コード・Scope が通知に含まれることを検証する。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_EscapesSlackSpecialCharacters` を追加する。group 名・command 名・要約に `&`・`<`・`>` および Slack のリンク・メンション記法を含む値を与え、ペイロード上で entity へ変換され、意図しないリンクやメンションが生じないことを検証する。
- [ ] `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` と `TestSlackHandler_WithRedactingHandler` を新しい書式へ更新する。`RedactingHandler` を挟んだ経路でも通知コンテキストの復元と判定が同じ結果になることを確認するケースを加える。
- [ ] `internal/logging/slack_handler_test.go` の切り詰めと属性抽出のテストが、stdout 1000 文字・stderr 500 文字の上限と既存の redaction を引き続き検証していることを確認する。
- [ ] `internal/logging/slack_sender_test.go` の `TestSlackSender_HighPriorityBypassesFullNormalQueue` と `TestSlackSender_QueueOverflowDropsAndRecords` を、確定済み優先度フィールドを使う形へ更新する。優先度を通常へ倒すと失敗することを確認する。
- [ ] `internal/testutil/notificationguard/notification_guard_test.go` に `TestProductionCodeUsesNotificationAttrs` を追加する。禁止する構文と除外範囲を次のとおり限定する。読み取り側と診断ログも同じ属性名を使うため、単に「`slack_notify` と `message_type` を含むコードを禁止する」と書くと、機能を実装しているコード自身が違反として挙がる。
  - 禁止するのは、発火元による属性の**構築**、すなわち `slog.Bool("slack_notify", ...)` と、種別名の**文字列リテラル**を第 2 引数に取る `slog.String("message_type", "<リテラル>")` である。`req.messageType` のような変数の転送は禁止しない。
  - `NotificationAttrs` の第 1 引数が `internal/logging` の登録済み公開トークンの識別子だけであることを併せて検証する。
  - 除外するファイルは `internal/logging/notification.go`（定義そのもの）、`internal/logging/slack_handler.go`、`internal/logging/slack_sender.go` の 3 つとする。後の 2 つは、レコードから属性を読む `case "slack_notify":`／`case "message_type":`、ドライランの Debug 行、`warnNotDelivered` と送信 Debug 行、および 5.2 で追加するスキーマ違反 WARN が、いずれも発火元ではなく読み取り側・診断側であるためである。除外理由をテストの doc コメントへ英語で記す。
  - 走査対象が 0 件のときは失敗させる。
- [ ] `internal/testutil/notificationguard/notification_guard_test.go` に `TestProductNameIsDefinedOnce` を追加する。判定は次のとおり厳密に定める。`go-safe-cmd-runner` はモジュールパス `github.com/isseis/go-safe-cmd-runner` の一部でもあるため、部分一致で数えると全 production ファイルの import パスに一致して初日から失敗し、実装者が表明を緩める方向へ「修正」してしまう。
  - `*ast.BasicLit` の文字列のうち、**復号後の値が厳密に `go-safe-cmd-runner` と等しいもの**だけを数える。
  - `*ast.ImportSpec` のパスとコメントは走査対象から除く。
  - 一致がちょうど 1 個で、`internal/logging/slack_handler.go` の `productName` 定数の右辺であることを検証する。
  - `internal/cmdcommon/common.go` の `DefaultHashDirectory = "/usr/local/etc/go-safe-cmd-runner/hashes"` は製品名を部分文字列として含むが、厳密一致ではないため対象外である。この除外は意図的なものであり、判定を緩めた結果ではない。
- [ ] `internal/runner/e2e_slack_webhook_separation_test.go` の `TestE2E_SlackWebhookSeparation_MessageFormat` を新しい統一書式へ更新し、`TestE2E_SlackWebhookSeparation_SuccessOnly` / `_ErrorOnly` / `_WarnToError` が宛先分離を引き続き検証していることを確認する。
- [ ] `internal/runner/e2e_slack_webhook_test.go` の `TestE2E_SlackWebhookWithMockServer` を、新しいペイロード全体（Text 行、色、フィールド順）を検証する形へ更新する。
- [ ] `cmd/runner/integration_slack_flush_test.go` の `TestIntegration_RunnerFlushesSlackOnNormalExit` を新しい書式へ更新し、終了時 flush で通知が失われないことを確認する。
- [ ] `cmd/runner/integration_pre_execution_error_test.go` の既存 E2E に、設定読み込み失敗の通知がグローバルスコープを保つことの検証を加える。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogger_LogUserGroupExecution` と `TestLogUserGroupExecution_OutputMasking` を、新しい属性キーと通知コンテキストへ合わせて更新する。

**完了条件**:

- [ ] `make fmt`、`make test`、`make lint` が通る。
- [ ] `make slack-e2e-test` が通る。`internal/runner/e2e_slack_webhook_separation_test.go` と `internal/runner/e2e_slack_webhook_test.go` は `//go:build e2e && test` であり、`make test`（`-tags test`）も `make lint`（`--build-tags test`）も e2e タグでビルドしない。この target を回さないと、5.7 で書き換える 2 ファイルはどのゲートでもコンパイルされず、AC-31 を検証するテストが一度も実行されないまま Phase 5 が完了してしまう。この target は `hash-e2e-test` を前提とする。
- [ ] `make slack-e2e-test` が数える対象テスト数を確認する。Makefile の `SLACK_E2E_RUN := ^TestE2E_SlackWebhook` に一致するテスト数が `SLACK_E2E_COUNT := 7` と異なると target が失敗するため、5.7 で e2e テストを増減または改名した場合は `SLACK_E2E_COUNT` を更新する。
- [ ] `go test -race -tags test ./internal/logging/...` が通り、通知種別定義の参照と既存の並行投入・flush に競合がない。
- [ ] `--dry-run` で Webhook への HTTP リクエストが届かず、キューとワーカーも生成されない既存テストが通り続ける。
- [ ] 5.7 で追加・更新した各テストについて、第 4.5 節の表に挙げた注入対象を一時的に壊すと失敗することを確認し、その結果をコミットメッセージへ記す。表は 5.7 のすべてのテストを網羅しており、一部の代表テストだけを確認して済ませない。
- [ ] Phase 5 全体を 1 コミットとし、単独で revert できる。

### Phase 6: 利用者向け文書の更新

**対象ファイル**: `docs/user/runner_command.ja.md`、`docs/user/runner_command.md`

- [ ] `docs/user/runner_command.ja.md` の「4.2 通知設定」に通知メッセージの節を追加する。通知される 3 種別（グループ実行の集計、実行前エラー、ユーザー／グループ指定コマンドの失敗）、統一書式の Text 行、ログレベルと絵文字・STATUS・色の対応表、Scope の表示（`(global)`、`group=<名前>`、`group=<名前> command=<名前>`）、Text 行の先頭に付く製品名 `go-safe-cmd-runner`、添付フィールド末尾の Scope・Hostname・Run ID の順序、`--dry-run` では通知を送らないことを記載する。
- [ ] 記載する Text 行の例を、`TestAllDefinitionsSatisfyEnvelope` が検証している書式と同一の文字列にする。文書へ書いた例そのものを `rg -F` で検索して 1 件一致することを確認し、実装のテストが期待する形と文字単位で一致していることを突き合わせる。書式が食い違う場合は、文書を実装へ合わせるのではなく、どちらが正しいかを先に決めてから直す。
- [ ] 日本語版をコミットしたうえで、`/mktrans` により `docs/user/runner_command.md` へ英語版を反映する。英語版を直接書かない。
- [ ] 日英の節構成・表の行数・例の行が対応していることを確認する。

**完了条件**:

- [ ] `rg -c -e '\[go-safe-cmd-runner\]' -e '\(global\)' -e 'Scope' docs/user/runner_command.ja.md` の一致が 3 パターンとも 1 件以上あり、いずれも追加した通知メッセージの節の中にある。`(global)` は括弧を `\(` `\)` でエスケープする。エスケープしないと `rg` の正規表現では捕捉グループになり、`global` という語だけに一致するため、`(global)` を一度も書いていない文書でもこの検査が通ってしまう。
- [ ] `rg -c -e '\[go-safe-cmd-runner\]' -e '\(global\)' -e 'Scope' docs/user/runner_command.md` について、日本語版と同じ 3 パターンが 1 件以上一致する。
- [ ] `make verify-docs` が通る。この target は `scripts/verification/run_all.sh` を通じて `compare_doc_structure.go` を実行し、日英の構成差を機械的に照合する。日英の対応確認は目視ではなくこの target で行う。
- [ ] `make test` と `make lint` が通る。

### Phase 7: 全体検証

- [ ] `make fmt`、`make test`、`make lint`、`make deadcode` をすべて実行し、通ることを確認する。
- [ ] 実 Slack 確認の前提を満たす。`GSCR_SLACK_WEBHOOK_URL_SUCCESS` と `GSCR_SLACK_WEBHOOK_URL_ERROR` の両方を設定し、`make hash` を実行して `sample/slack-notify.toml` と `sample/slack-group-notification-test.toml` のハッシュを登録する。Makefile の `check_slack_webhook` は環境変数が未設定でも警告を出すだけで target は成功するため、前提を確認せずに実行すると 1 件も投稿されないまま「目視で確認した」ことになってしまう。両 target とも起動時検証を通るため、この前提は 2 つの実行の両方に必要である。前提を満たせない場合は本ステップを未実施として記録する。
- [ ] `make slack-notify-test` と `make slack-group-notification-test` をテスト用チャンネルに対して実行し、Text 行、添付色、フィールド順、プッシュ通知での製品名と Scope の表示を目視で確認する。両方の実行に `slack-0172-$(date +%s)` の形の一意な Run ID を明示的に渡し、投稿を Run ID フィールドで特定する。共有チャンネルには過去の実行の投稿が残るため、新しさだけでは自分の実行の投稿を判別できない。
- [ ] 未登録の `message_type` の汎用メッセージと WARN は、既存のモックサーバー経由のテスト（`TestSlackHandler_UnknownMessageType`）で確認する。production コードに一時的な発火元を追加しない。追加すると 5.7 の guard テストに違反して `make test` が落ちるうえ、取り除いたことを確かめるゲートが無い。
- [ ] 実 Slack での確認ができない環境では、モックサーバーによるペイロード検証をもって代替とし、実表示未確認をリリース前の残存リスクとして記録する。
- [ ] リリースノートに、Text 行と添付フィールド順序の新旧例、および `group` が構造化ログのトップレベルから通知コンテキストの下へ移る JSON の新旧例を記載する。
- [ ] リポジトリ内で通知本文や `group` 属性を解析している箇所がないか `rg` で確認する。
- [ ] 第 7 節の受け入れ基準検証表の各行を実際に実行し、すべて期待どおりであることを確認する。

## 3. 実装順序とマイルストーン

| マイルストーン | 含む Phase | 成果物 | 完了の判断 |
|---|---|---|---|
| M1: 死んだ種別の除去 | Phase 1〜3 | 3 個の独立した削除コミット | AC-01〜AC-08 が満たされ、`make deadcode` が新たな到達不能コードを報告しない |
| M2: 型の伝搬 | Phase 4 | 通知コンテキスト型、`RuntimeCommand.GroupName`、全発火元への付与 | AC-09〜AC-17 が満たされ、表示は未変更のまま `make test` が通る |
| M3: 書式の統一 | Phase 5 | 通知種別定義、共通エンベロープ、WARN、統一書式 | AC-18〜AC-27、AC-31〜AC-33 が満たされる |
| M4: 文書 | Phase 6 | 日英の利用者向け文書 | AC-28〜AC-30 が満たされる |
| M5: 全体検証 | Phase 7 | 実 Slack での確認記録とリリースノート | 全 AC と Success Criteria が満たされる |

M1 の 3 コミットは順序を入れ替えても成立するが、Phase 2 で高優先度テストの基準を `pre_execution_error` へ移すため、Phase 3 は Phase 2 の後に行う。Phase 4 は Phase 1〜3 の削除後に着手し、統一の対象を 3 種別に絞った状態で型を伝搬させる。Phase 5 は Phase 4 の通知コンテキストが全発火元に届いていることを前提とする。Phase 6 は Phase 5 で表示が確定してから着手する。

## 4. テスト戦略

方針の全体像は [02_architecture.md §7](02_architecture.md#7-テスト戦略) を参照する。本節では実装計画としての進め方だけを記す。

### 4.1 単体テスト

- 通知コンテキストの型とエンコードは `internal/common/notification_context_test.go` で閉じて検証する。Slack の表示に依存しない。
- 通知種別定義の共通契約は `internal/logging/notification_test.go` で、通知種別定義の集合を `range` して検証する。種別名を書き写した並行リストは作らない。種別を 1 つ足してエンベロープを満たさないようにすると失敗することを、実装時に一時的な定義を加えて確認する。
- 表示の検証は `internal/logging/slack_handler_test.go` に集約する。レベル対応の全域性は境界値（DEBUG、INFO、INFO と WARN の中間値、WARN、ERROR、ERROR より上）で確認する。
- 異常系は、未知種別（空文字と未知文字列）、通知コンテキストの欠落・重複・型違い・値の矛盾、未知種別と不正コンテキストの同時成立、受付停止済み送信機構、の各経路を個別に検証する。

### 4.2 統合テスト・E2E テスト

既存の E2E テストを新書式へ更新する形を取り、新しい E2E の枠組みは作らない。対象は `internal/runner/e2e_slack_webhook_separation_test.go`、`internal/runner/e2e_slack_webhook_test.go`、`cmd/runner/integration_slack_flush_test.go`、`cmd/runner/integration_pre_execution_error_test.go` である。宛先分離（INFO は成功用、WARN 以上はエラー用）の検証は既存テストがそのまま担う。

### 4.3 静的契約テスト

`internal/testutil/notificationguard/notification_guard_test.go` に 3 個の guard テストを置く。実行時に観測できない「発火元が属性生成関数を迂回していないこと」「`PreExecutionError` のリテラルが通知コンテキストを省略していないこと」「製品名の定義が 1 箇所であること」を構文木で検証する。既存の `internal/testutil/synccensus` と同じ走査構成を取り、`internal/testutil/identitymutationguard` の `ProductionGoFiles` を再利用する。いずれのテストも、走査対象が 0 件のときに失敗させることで、走査の空振りによる偽の合格を防ぐ。

### 4.4 テストヘルパーの方針

新しいテストヘルパーファイルは作らない。記録用ロガーは `internal/testutil` の `NewRecordingLogger` と `LogRecorder`、構文木の走査は `internal/testutil/identitymutationguard` を再利用する。`internal/testutil/notificationguard/` は [test_organization.md](../../dev/developer_guide/test_organization.md) の Classification A のうち、既存の `synccensus` と同じくリポジトリ横断の guard テストを置く区画であり、`//go:build test` を付ける。パッケージ名は既存の `synccensus` と `sourceorder` に倣い `notificationguard` とする。同ガイドの `<domain>testutil` という命名規則は、あるパッケージ直下の `testutil/` を対象とするものであり、リポジトリ横断の guard テスト用パッケージには適用しない。パッケージ内の非公開 API に触れる必要は生じないため、`test_helpers.go` の追加は不要である。

### 4.5 各テストが理由どおりに失敗することの確認

CLAUDE.md の「Every test must be able to fail for its stated reason」に従い、Phase 2〜5 で追加・更新した各テストについて、検証対象の挙動を一時的に壊して失敗することを確認する。壊す対象は次のとおりで、確認結果は該当コミットのメッセージへ記す。

| Phase | 壊す対象 |
|---|---|
| 2 | 優先度判定を常に低優先度へ倒す |
| 3 | 呼び出し元から `logElevationOutcome` の呼び出しを削る |
| 4 | コンストラクタが渡された値を捨てるようにする、`LogValue` から属性を落とす、`GroupName` が空文字を返すようにする、発火元の通知コンテキスト付与を外す、`PreExecutionError` のリテラル 1 個から `NotificationContext` を削る（4.8 の guard 用） |
| 5 | 共通エンベロープの付与を外す、通知種別定義の集合から 1 要素を落とす、優先度を通常へ倒す、スコープ判定を常に妥当へ倒す、WARN の記録を外す、組み立て関数に Scope・Hostname・Run ID を重複して出させる、エスケープ関数の呼び出しを 1 箇所外す、WARN へ group 名を加える、受付停止判定より前で組み立て関数を呼ぶ、理由コードの列挙から 1 個落とす、production コードへ `slack_notify` の直接構築を 1 個加える（guard 用）、`"go-safe-cmd-runner"` の厳密一致リテラルを production コードへもう 1 個加える（guard 用） |

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| Text 行と添付フィールド順序の変更が、通知本文を解析する外部の Slack ワークフローや監視ルールを壊す | 利用者側の自動化が停止する | Phase 7 でリリースノートに新旧のペイロード例を記載し、テスト用チャンネルで先に確認してから通常チャンネルへ展開する。問題があれば Phase 5 の単一コミットを取り消す |
| `GroupSummaryAttrs.Group` の削除が、JSON ログを解析する外部利用者を壊す | ログ解析が `group` を見失う | Phase 7 のリリースノートで JSON の新旧例を示す。[02_architecture.md §3.4](02_architecture.md#34-通知種別定義) の方針どおり、移行期間中に新旧両方を出すことはしない |
| Phase 5 の変更範囲が大きく、1 コミットに収める方針と衝突する | レビューが難しくなる | Phase 4 で型の伝搬を先に済ませ、Phase 5 の差分を表示と定義の切り替えに限定する。Phase 5 の内部では 5.1〜5.7 の順で進め、各段階で `make test` を通してから次へ進む |
| 実 Slack のテスト用チャンネルを用意できず、実表示を確認できない | 表示崩れがリリース後に判明する | モックサーバーによるペイロード検証を必須とし、実表示未確認を残存リスクとして記録する（[02_architecture.md §5.3](02_architecture.md#53-slack-での表示互換性)） |
| コマンド数の多いグループで添付フィールドが Slack の推奨数を超え、送信が拒否される | 通知が届かない | 既存リスクであり本タスクでは上限を変えない。送信失敗は既存の送信失敗ロガーが記録する HTTP エラーから確認できる |
| 静的契約テストが走査対象を取り違え、違反を検出しないまま合格する | 登録漏れが素通りする | 各 guard テストで走査対象が 0 件のときに失敗させる。実装時に意図的な違反を 1 件入れて検出されることを確認する |

## 6. 実装チェックリスト

- [ ] Phase 1: `privileged_command_failure` の削除（1 コミット）
- [ ] Phase 2: `security_alert` の削除と高優先度テストの基準移行（1 コミット）
- [ ] Phase 3: `privilege_escalation_failure` の削除と特権昇格結果ログのテスト追加（1 コミット）
- [ ] Phase 4: 通知コンテキスト型、`RuntimeCommand.GroupName`、全発火元への伝搬、リテラルの静的契約テスト
- [ ] Phase 5: 通知種別定義、共通エンベロープ、統一書式、未知種別と不正スコープの WARN（1 コミット）
- [ ] Phase 6: 利用者向け日本語文書の更新と英語版への翻訳
- [ ] Phase 7: 全体検証、実 Slack での表示確認、リリースノート
- [ ] 第 7 節の受け入れ基準検証表の全行が期待どおりである
- [ ] 第 9 節の横断検索チェックリストの全項目が期待どおりである

## 7. 受け入れ基準の検証

各行の「種別」は、`test` が実行可能なテスト（挙動を壊すと失敗する）、`static` が `rg` などの検索コマンドまたはビルド・ツールによる確認、`manual` が人による観察を表す。全 AC が `test` または `static` を少なくとも 1 つ持つ。

| AC | 種別 | 検証方法 | 期待結果 |
|---|---|---|---|
| AC-01 | static | `rg -n -e 'privileged_command_failure' -e 'PrivilegedCommandFailure' cmd/ internal/` | 一致 0 件 |
| AC-02 | static | `rg -n -e 'security_alert' -e 'SecurityAlertAttrs' -e 'LogSecurityEvent' -e 'buildSecurityAlert' -e 'messageTypeSecurityAlert' cmd/ internal/` および `rg -n --pcre2 'common\.Severity(Critical|High)' cmd/ internal/` | どちらも一致 0 件。後者は参照側も含めて確認し、別型の `internal/runner/runerrors.ErrorSeverityCritical` には一致しない |
| AC-03 | static | `rg -n -e 'privilege_escalation_failure' -e 'PrivilegeEscalationFailureAttrs' -e 'LogPrivilegeEscalation' -e 'buildPrivilegeEscalationFailure' -e 'messageTypePrivilegeEscalationFail' cmd/ internal/` | 一致 0 件 |
| AC-04 | static | `git log --oneline` で Phase 1〜3 が 3 個の別コミットであることを確認し、各コミットについて `git revert --no-commit <sha> && git revert --abort` を実行する | 3 コミットが存在し、いずれの revert も競合なく適用できる |
| AC-05 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestLogElevationOutcome_RecordsElevationModes` | native root と `seteuid` の各モードで Info レコードが 1 件ずつ記録され、呼び出し元から `logElevationOutcome` の呼び出しを削ると失敗する |
| AC-06 | static | Phase 1〜3 の各コミットについて削除前後の `go tool cover -func` を取得して差分を取り、結果をコミットメッセージへ記載する。`git log` で 3 コミットすべてに記載があることを確認する | 存続する関数のカバレッジが下がっておらず、3 コミットすべてに比較結果の記載がある |
| AC-07 | test | `internal/logging/slack_sender_test.go::TestSlackSender_HighPriorityBypassesFullNormalQueue` | 存続する `pre_execution_error` で高優先度を検証し、優先度を常に通常へ倒すと失敗する |
| AC-08 | static | `make deadcode` | 新たな到達不能コードの報告なし |
| AC-09 | test | `internal/common/notification_context_test.go::TestNotificationContext_ConstructorsAndZeroValue` | ゼロ値が `ScopeUnknown` であり、`GlobalScope()` の `ScopeGlobal` と区別される。フィールドが非公開であることは、同パッケージ外のテストがコンストラクタ以外で有効値を作れないことをコンパイルで保証する |
| AC-10 | test | `internal/common/notification_context_test.go::TestNotificationContext_LogValueEncoding` および `::TestNotificationScope_StringRoundTrip` | `scope` と `group` が常に出力され、command が空のときだけ `command` 属性が無い |
| AC-11 | test | `internal/testutil/notificationguard/notification_guard_test.go::TestProductionCodeUsesNotificationAttrs` および `internal/logging/notification_test.go::TestAllDefinitionsSatisfyEnvelope` | production コードの発火元が `NotificationAttrs` を経由して通知コンテキストを付与している |
| AC-12 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_InvalidNotificationContext` | Scope 表示が `(scope: invalid)` になり、送信失敗ロガーへ WARN が 1 件記録され、グローバル扱いへ落ちない |
| AC-13 | test | `internal/runner/runner_test.go::TestSlackNotification_GroupVerificationErrorScope` | 検証エラーの通知に `scope=group` と該当 group 名が付く |
| AC-14 | test | `internal/runner/runner_test.go::TestSlackNotification_GroupVerificationErrorScope` | `error_message` 属性が `Group: ` の接頭辞を含まない |
| AC-15 | test | `internal/logging/pre_execution_error_test.go::TestHandlePreExecutionError_EmitsNotificationContext` と `internal/logging/slack_handler_test.go::TestSlackHandler_ScopeRendering`、および `internal/testutil/notificationguard/notification_guard_test.go::TestPreExecutionErrorLiteralsDeclareNotificationContext` | グローバルな起動前エラーが `scope=global` を持ち `(global)` と表示される。`bootstrap/config.go` を含む全リテラルが通知コンテキストを宣言している |
| AC-16 | test | `internal/runner/base/runnertypes/runtime_test.go::TestNewRuntimeCommand_RetainsGroupName` | コンストラクタへ渡した group 名を `GroupName()` が返す |
| AC-17 | test | `internal/runner/base/audit/logger_test.go::TestLogger_LogUserGroupExecution_CommandScope` と `internal/logging/slack_handler_test.go::TestSlackHandler_UserGroupCommandFailure` | 失敗通知に group 名と command 名の双方が表示される |
| AC-18 | test | `internal/logging/notification_test.go::TestAllDefinitionsSatisfyEnvelope`（登録済み 3 種別）と `internal/logging/slack_handler_test.go::TestSlackHandler_GenericMessageFollowsTextLineFormat`（汎用メッセージ） | Text 行が `[<製品名>] <絵文字> *<STATUS>* — <スコープ> : <要約>` の形である |
| AC-19 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_LevelDrivesDisplay` | 絵文字・STATUS・色がログレベルだけで決まり、同レベルなら種別が変わっても変化しない |
| AC-20 | test | `internal/logging/notification_test.go::TestAllDefinitionsSatisfyEnvelope`（登録済み 3 種別）と `internal/logging/slack_handler_test.go::TestSlackHandler_GenericMessageFollowsTextLineFormat`（汎用メッセージ） | どのメッセージにも `###` が現れない。汎用メッセージは通知種別定義の集合の要素ではないため、走査だけでは覆えず別途検証する |
| AC-21 | test | `internal/logging/notification_test.go::TestAllDefinitionsSatisfyEnvelope` | 添付フィールドの末尾 3 件が Scope、Hostname、Run ID の順である |
| AC-22 | test | `internal/logging/notification_test.go::TestAllDefinitionsSatisfyEnvelope` の組み立て関数を直接呼ぶ検証 | 各組み立て関数が返す `messageDetails.Fields` のタイトルに Scope、Hostname、Run ID が現れず、共通エンベロープ側だけがこれらを付ける |
| AC-23 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_UserGroupCommandFailure` | 固有メッセージとして組み立てられ、失敗したコマンド名と終了コードを含む |
| AC-24 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_UnknownMessageType` と `::TestSlackHandler_SingleWarnForCombinedViolations` | 汎用メッセージが送られ、`unknown_message_type` の WARN が 1 件記録される |
| AC-25 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_UnknownMessageType` と `::TestSlackHandler_GenericMessageFollowsTextLineFormat` | 汎用メッセージが色、Scope、Hostname、Run ID を備える |
| AC-26 | test | `internal/logging/notification_test.go::TestAllDefinitionsSatisfyEnvelope` | 通知種別定義の集合を `range` して検証しており、エンベロープを満たさない種別を 1 つ足すと失敗する |
| AC-27 | test | `internal/logging/notification_test.go::TestNotificationDefinitionsAreSingleSource` と `internal/testutil/notificationguard/notification_guard_test.go::TestProductionCodeUsesNotificationAttrs` | 種別名・組み立て関数・優先度が単一の定義から引かれ、同じ種別集合を独立に列挙する箇所が他に無い。優先度が定義から引かれることは `internal/logging/slack_sender_test.go::TestSlackSender_HighPriorityBypassesFullNormalQueue` が併せて確認する |
| AC-28 | static | `rg -n -e '\[go-safe-cmd-runner\]' -e '\(global\)' -e 'Scope' docs/user/runner_command.ja.md` | 追加した通知メッセージの節に一致し、種別・統一書式・Scope 表示・製品名の記載がある |
| AC-29 | static | `rg -n -e '\[go-safe-cmd-runner\]' -e '\(global\)' -e 'Scope' docs/user/runner_command.md` と、日英の節構成・表の行数の突き合わせ | 日本語版と対応する記載が英語版にある |
| AC-30 | static | 各コミットで `make test && make lint` | すべてのコミットで成功する |
| AC-31 | test | `internal/runner/e2e_slack_webhook_separation_test.go::TestE2E_SlackWebhookSeparation_SuccessOnly`、`::TestE2E_SlackWebhookSeparation_ErrorOnly`、`::TestE2E_SlackWebhookSeparation_WarnToError` を `make slack-e2e-test` で実行する | INFO は成功用、WARN 以上はエラー用 Webhook へ届く挙動が変わらない。これらは `//go:build e2e && test` であり `make test` では実行されないため、`make slack-e2e-test` の実行が必須である |
| AC-32 | static | 第 4.5 節の表に従って各テストを壊し、失敗することを確認して該当コミットのメッセージへ記す。`git log` で Phase 2〜5 の各コミットに記載があることを確認する | Phase 2〜5 の各コミットに確認結果の記載がある |
| AC-33 | test | `internal/logging/notification_test.go::TestAllDefinitionsSatisfyEnvelope` と `internal/logging/slack_handler_test.go::TestSlackHandler_GenericMessageFollowsTextLineFormat`、`internal/testutil/notificationguard/notification_guard_test.go::TestProductNameIsDefinedOnce` | 汎用メッセージを含む全メッセージが製品名で始まり、production コードで厳密一致する製品名リテラルの定義が 1 箇所である |

AC-04、AC-06、AC-30、AC-32 はコミットの形と記載を対象とするため、上表の `static` な確認に加えて、Phase 7 でプルリクエスト上のコミット一覧を目視で確認する（`manual`）。この目視は上表の検証を置き換えるものではない。

## 8. 成功基準

- [ ] Slack に届くすべての通知から、送信元が go-safe-cmd-runner であることと、グローバルか、どの group か、どのコマンドかが判別できる。
- [ ] 通知の見出し・色・末尾フィールドが 1 つの規則に従っており、種別ごとの例外が無い。
- [ ] 本番で発火しない通知種別が production コードに残っていない。
- [ ] 種別の登録漏れがテストで検知され、汎用メッセージへ黙って落ちることがない。
- [ ] 第 7 節の受け入れ基準検証表の全 33 行が期待結果を満たす。
- [ ] `make test`、`make lint`、`make deadcode` がすべて通る。

## 9. 横断検索チェックリスト

`make lint` と `make test` では検出できない項目だけを挙げる。第 7 節の検証表に既に載っているコマンドは重複させない。

- [ ] 削除した 3 種別に言及する古いコメントが残っていないことを確認する。`rg -n -i -e 'security alert' -e 'privilege escalation failure' -e 'privileged command' internal/logging/ internal/common/ internal/runner/base/audit/ Makefile sample/` の一致が 0 件である。
- [ ] `docs/user/` に削除した 3 種別への言及が無いことを確認する。`rg -n -e 'security_alert' -e 'privilege_escalation_failure' -e 'privileged_command_failure' docs/user/` の一致が 0 件である。`docs/tasks/0068_separate_slack_webhooks/` と `docs/tasks/0163_redaction_coverage_and_slack_async/` の一致は当時の設計を記録した過去の文書であり、書き換えない。
- [ ] 通知本文や `group` 属性を解析している箇所が他に無いことを確認する。`rg -n -e '"group"' -e 'GroupSummaryAttrs' -e 'message_type=' -e 'status=success' cmd/ internal/ scripts/ Makefile sample/` の結果を 1 件ずつ確認し、Phase 5 で更新済みでない箇所が残っていない。とくに `slack-group-notification-test` が表示する運用手順（`message_type=command_group_summary`、`status=success` / `status=error` への言及）は、5.4 で `status` 属性が表示判定に使われなくなるため、記述が誤解を招かないか確認する。
- [ ] `NotificationScope`、`NotificationContext`、`Notification`、`GroupName` の各識別子が、他パッケージの同名の型・メソッドと紛らわしくないことを確認する。`rg -n -e 'func .*GroupName\(' -e 'type Notification' cmd/ internal/` の結果を確認し、`RuntimeCommand.GroupName` 以外に `GroupName()` メソッドがある場合は、その所在を把握したうえで用途が異なることを確認する。
- [ ] 用語集との整合を確認する。「スコープ」は `docs/translation_glossary.md` に変数スコープの文脈で登録済みであるため、通知スコープの意味で新たに使う語（通知コンテキスト、共通エンベロープ、通知種別定義）が Phase 6 の利用者向け文書へ持ち込まれる場合は、初出で定義するか平易な表現へ言い換える。

## 10. 次のステップ

- [ ] 本実装計画書を人がレビューし、Status を `approved` へ更新する。
- [ ] 承認後に Phase 1 から実装を開始する。
- [ ] Phase 7 の完了後、リリースノートを公開し、テスト用チャンネルでの確認結果を記録する。
- [ ] 削除した 3 種別の通知が将来必要になった場合は、[01_requirements.md](01_requirements.md) の「対象外」に記したとおり別タスクとして起票する。
- [ ] 添付フィールド数の集約上限、追加の表示正規化、Block Kit への移行は、[02_architecture.md §5.3](02_architecture.md#53-slack-での表示互換性) のとおり別タスクで扱う。
