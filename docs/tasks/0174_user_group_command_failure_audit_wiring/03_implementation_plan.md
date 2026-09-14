# 実装計画書: run-as コマンド失敗時の監査記録と `user_group_command_failure` 通知の配線

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-14 |
| Review date | 2026-09-14 |
| Reviewer | isseis |
| Comments | `-` |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)
- 起点: [0172 実装計画書 §10 の follow-up](../0172_slack_notification_message_unification/03_implementation_plan.md#10-次のステップ)
- テストヘルパ配置: [test_organization.md](../../dev/developer_guide/test_organization.md)

---

## 1. 実装の概要

### 1.1 目的

`run_as_user`／`run_as_group` を指定したコマンドが開始した後に失敗した場合、監査ログに `user_group_execution` の ERROR レコードを書き、`user_group_command_failure` 通知を発生させる。逆に、子プロセスが一度も開始していない失敗は実行として監査しない。設計の詳細は [02_architecture.md](02_architecture.md) §1・§3 を参照する。

### 1.2 実装方針

1. 子プロセスの開始と終了の種別を `preparedCommand.child`（`childState`）で宣言し、監査の可否をその値だけで決める（02_architecture.md §3.1・§3.2）。
2. 監査レコードの組み立てを `auditUserGroupExecution` に集約し、成功経路と失敗経路の呼び出しを `err` の分岐で排他にする（02_architecture.md §3.3・§3.4）。
3. `Result`・`LogUserGroupExecution`・0172 の通知種別定義・メッセージ書式は変更しない（02_architecture.md §3.5）。
4. Phase の順序は 02_architecture.md §8 の実装優先順位を保つ。テストは対応する実装と同じ Phase・PR に置き、§8 の Phase 番号とは対応が異なる（§3.3）。

### 1.3 既存コード調査結果

2026-09-14 時点の commit `33339562`（`docs(0174): Approved the architecture document`）のコードを読んだ結果を示す。行番号は同じ commit のものである。

**状態を刻む対象**

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| `preparedCommand` | `internal/runner/base/executor/command_lifecycle.go:134` | `spent`・`binding`・`kill` などを持つ。`child childState` を追加する |
| `startPrepared` | `command_lifecycle.go:447` | `pc.execCmd.Start()` は `:461`。成功時に `childRunning` を代入する |
| `superviseCommand` | `command_lifecycle.go:613` | `killed := !outcome.reaped` は `:652`。`Result` 組み立ては `:720`。終了種別をここで確定して代入する |
| `runCommand` | `command_lifecycle.go:513` | `!opened` は `:545-551` で nil Result、`!started` は `:552-553` で `reportStartFailure`。どちらも状態は零値 |
| `reportStartFailure` | `command_lifecycle.go:580` | `ExitCodeUnknown` のプレースホルダ Result を返す。状態は零値のまま |

**監査の配線対象**

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| `executeWithUserGroup` | `internal/runner/base/executor/executor.go:175` | 失敗分岐は `:282-295` で早期 return し、成功経路の監査ブロック `:297-306` に到達しない。失敗分岐に開始済みのときだけ監査を足す |
| `LogUserGroupExecution` | `internal/runner/base/audit/logger.go:70` | `ExitCode` で INFO／ERROR を分岐し（`:115-129`）、失敗分岐では通知属性（`:123-124`）と redaction 済み stdout／stderr（`:120-121`）を載せる。無変更 |
| `WithAuditLogger`／`AuditLogger` | `executor.go:61,101-105` | `Option`（`executor.go:92`）は順に適用される（`:121-123`）。監査ロガーなしの executor はテスト側で組める |

**事実として確認した既存挙動**

- `ExitCodeUnknown`（`internal/runner/base/executor/interface.go:14`）は `-1` で、未開始のプレースホルダとシグナルによる強制終了の双方がこの値を取り得る。`TestSupervise_ProcessAlreadyDoneIsNotAnError`（`executor_supervise_test.go:317`）は、kill が回収済みの exit 0 と競合すると `err != nil` かつ `ExitCode == 0` になる経路を固定している。この競合の扱いは 02_architecture.md §4 の補足に記した。
- `validatePrivilegedCommand`（`executor.go:680-701`）は絶対パスであることだけを確認し、ファイルの存在は確認しない。存在しない絶対パス（例: `/nonexistent/scr-0174-missing`）は `prepareCommand` を通過し、`exec.Cmd.Start()` が `ENOENT` で失敗する（`go run` による実測、commit `33339562` 時点の Go ツールチェーン）。この失敗は `reportStartFailure` に到達し、状態は零値のままになる。
- `newSetuidFixture`（`executor_privilege_gap_integration_test.go:86`）の基本オプションは常に `WithAuditLogger` を含む（`:142-147`）。`opts` はその後に適用されるため、テスト側で `AuditLogger` を nil にするオプションを渡せる。
- setuid ゲート `scripts/verification/run_executor_setuid_integration.sh` は実行対象を `^(TestPrivilegeGap_.*|TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot)$` に限定し（`:94`）、必須テスト名の一覧を `:151` に持つ。skip を 1 件でも検出すると FATAL になる。

**再利用する既存テストとヘルパー**

| 種別 | 名前 | 位置 | 用途 |
|---|---|---|---|
| 単体ヘルパー | `prepareForSupervise` | `executor_supervise_test.go:39` | run-as 資格情報なしの `preparedCommand` を作る |
| 単体ヘルパー | `runUnprivileged` | `command_lifecycle.go:494` | `runCommand` の開始区間なし経路 |
| 単体テスト | `TestStartPrepared_RejectsSpentCommand` | `executor_lifecycle_test.go:390` | spent の起動拒否 |
| 単体テスト | `TestRunCommand_StartWindowThatRunsNothingIsRejected` | `executor_lifecycle_test.go:957` | `!opened` の拒否 |
| 統合ヘルパー | `newSetuidFixture` | `executor_privilege_gap_integration_test.go:86` | 実 setuid 環境の fixture |
| 統合ヘルパー | `runtimeCommand` | 同 `:250` | run-as コマンドの生成 |
| 統合ヘルパー | `credentialShellCommand` | 同 `:230` | 子の資格情報を確認する shell コマンド |
| 統合ヘルパー | `executeAsync`／`waitForOutcome`／`waitForReady` | 同 `:305,337,291` | 実行中コマンドのキャンセル |
| 統合ヘルパー | `assertWindowAttrs` | 同 `:348` | レコードの `elevation_count` と `privilege_duration_*_us` の照合 |
| 統合ヘルパー | `assertAuditWindows`／`assertFailureWindows` | 同 `:369,375` | 既存レコードへの `assertWindowAttrs` 適用 |
| 記録ヘルパー | `tu.RecordSnapshot.AssertAttrs`／`AssertNotificationContext`／`LogRecorder.RequireRecord` | `internal/testutil/handlers.go:202,256,188` | レコードの属性・通知コンテキストの検証 |
| 監査ロガー | `audit.NewAuditLoggerWithCustom` | `internal/runner/base/audit/test_helpers.go:13` | `redaction.DefaultConfig()` 付きのテスト用ロガー |
| 既存テスト | `TestLogger_LogUserGroupExecution`／`TestLogUserGroupExecution_OutputMasking` | `internal/runner/base/audit/logger_test.go:38,230` | 失敗分岐の通知属性と redaction の既存保証 |
| 既存テスト | `TestSlackHandler_UserGroupCommandFailure` | `internal/logging/slack_handler_test.go:2032` | Slack ペイロードの既存保証 |

**変更が必要な既存コメント**

- `internal/runner/base/executor/executor_test.go:680-683` のテスト doc コメントと `:716` のサブテスト内コメントは「監査は成功時にのみ発火する」と述べている。配線後はこの前提が変わるため、Phase 3 で「この失敗は子プロセス開始前なのでレコードが出ない」という趣旨へ更新する。アサーションは変えない。

---

## 2. 実装ステップ

### Phase 1: `childState` 型と `preparedCommand.child`、遷移 2 箇所と遷移テストを実装

**対象ファイル**: `internal/runner/base/executor/command_lifecycle.go`、`internal/runner/base/executor/executor_supervise_test.go`

**作業内容**:

- [x] `childState` 型と定数 `childNotStarted`（零値）・`childRunning`・`childExited`・`childTerminated` を追加する。意味は 02_architecture.md §3.1 のとおり。Go のコメントは英語で書く。
- [x] `started()` メソッドを追加する。`childRunning`・`childExited`・`childTerminated` を `true`、`childNotStarted` と未知の値を `false` とする（02_architecture.md §3.1）。
- [x] `preparedCommand` に `child childState` フィールドを追加する。
- [x] `startPrepared` の `pc.execCmd.Start()` 成功直後に `pc.child = childRunning` を代入する。
- [x] `superviseCommand` で `killed` が確定した後、`Result` を組み立てる前に `pc.child` を `childExited`（kill 経路を通っていない）または `childTerminated`（kill 経路を通った）へ確定する（02_architecture.md §3.2 の表）。
- [x] 既存の戻り値・ログ・`Result` の値・エラーを変更しない。
- [x] `TestRunCommand_ChildStateTransitions` を追加する。`prepareForSupervise` と `runUnprivileged` を再利用し、正常終了・非ゼロ終了・キャンセル／タイムアウトによる強制終了・開始前失敗（存在しない絶対パス）・起動区間が start を実行しない場合・spent の各ケースで、`pc.child`・`started()`・`Result.ExitCode` を観測する。強制終了は `WithKillGraceDelay` で待ち時間を短縮する。spent は `TestStartPrepared_RejectsSpentCommand`（`executor_lifecycle_test.go:390`）と同じく `&preparedCommand{spent: true}` を直接組んで入力する。
- [x] 起動直後を観測するケースを同テストに含める。`startForSupervise` で開始した後、`superviseCommand` を呼ぶ前に `pc.child == childRunning` かつ `started()` が true であることを観測する。`startPrepared` の遷移は `superviseCommand` が終了種別で上書きするため、`runCommand` の戻り値だけでは観測できない。
- [x] `TestSupervise_ProcessAlreadyDoneIsNotAnError`（`executor_supervise_test.go:317`）に `pc.child == childTerminated` の観測を追加する（kill が exit 0 の回収と競合しても開始済みのままであることを状態で固定する）。
- [x] `TestChildState_StartedClassification` を追加する。4 定数に対する `started()` の分類と、列挙外の値（例: `childState(99)`）が `false` になることを観測する（02_architecture.md §7.1）。

**完了確認**: `make fmt`（Go を変更するため）→ `make test` → `make lint` が通る。この時点では配線がないため実行時の挙動は変わらない。

### PR-1 作成ポイント: child execution state and transition tests

**対象ステップ**: Phase 1

**推奨タイトル**: `feat(0174): add the child execution state and its transition tests`

**レビュー観点**: `childState` の零値が `childNotStarted` で、`started()` が宣言済み 3 状態だけを true とし未知の値を false に倒すこと（02_architecture.md §3.1）／遷移の代入が `Start()` 成功直後と `superviseCommand` の終了種別確定後の 2 箇所だけで、`Result` の組み立て前に確定されること（§3.2）／遷移テストが 4 状態と未開始 3 経路を観測し、`started()` の列挙外の値の倒れ方も固定していること／この PR では production の読み出しがまだ無く、`started()` の到達性がテストに限られること

**実装モデル要件**: frontier-recommended

**判定理由**: Phase 1 は kill と回収の競合（exit 0 競合）を含む終了種別の状態機械を導入する孤立した複雑ステップであり、frontier-recommended の「孤立した高リスク／複雑ステップ（状態機械）」に該当する。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: `auditUserGroupExecution` を抽出し、成功経路を置き換える

**対象ファイル**: `internal/runner/base/executor/executor.go`

**作業内容**:

- [ ] `auditUserGroupExecution(ctx context.Context, cmd *runnertypes.RuntimeCommand, result *Result, startTime time.Time, metrics audit.PrivilegeMetrics)` を追加する。`e.AuditLogger == nil` は no-op とする。中身は既存の `audit.ExecutionResult` の組み立てと `LogUserGroupExecution` の呼び出しをそのまま移す（属性・時間計測・redaction を変えない）。
- [ ] 成功経路の監査ブロック（`executor.go:297-306`）を `auditUserGroupExecution` 呼び出しへ置き換える。
- [ ] 失敗分岐（`executor.go:282-295`）にはこの Phase では触れない（配線は Phase 3）。
- [ ] 成功経路の `Result` の非 nil 前提は現状のままとする。

**完了確認**: `make fmt` → `make test` → `make lint` が通る。成功経路のレコード内容が変わらないことは既存テスト（`TestLogger_LogUserGroupExecution` と `TestPrivilegeGap_ChildCredentialsMatchTarget`）で確認する。加えて setuid ゲートを skip なしで実行し、`TestPrivilegeGap_ChildCredentialsMatchTarget` が通ることを確認する（この既存テストの `assertAuditWindows` は `executeWithUserGroup` 越しに INFO 成功レコードを要求するため、成功経路の呼び出しの欠落・改変を検出する）。

### PR-2 作成ポイント: extract the audit helper (no behavior change)

**対象ステップ**: Phase 2

**推奨タイトル**: `refactor(0174): extract the user group audit helper`

**レビュー観点**: 既存の成功経路の監査ブロックがメソッドへ移動しただけで、属性・時間計測・redaction の値が変わっていないこと（02_architecture.md §3.4）／`e.AuditLogger == nil` の no-op が維持されていること／失敗分岐にはまだ触れておらず、`err != nil` の挙動がこの PR で変わらないこと／成功経路の `auditUserGroupExecution` 呼び出しが setuid 回帰 `TestPrivilegeGap_ChildCredentialsMatchTarget` によって `executeWithUserGroup` 越しに実行され、`assertAuditWindows` が INFO 成功レコードを要求すること（`TestLogger_LogUserGroupExecution` 単体では executor を通らない）

**実装モデル要件**: standard

**判定理由**: 既存ブロックのメソッド化に限られ、設計判断・高リスク分岐・未確定の実装アプローチは無く、Conditional checks・panel-mode トリガーのいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: 失敗分岐への配線と setuid 統合テストを実装

**対象ファイル**: `internal/runner/base/executor/executor.go`、`internal/runner/base/executor/executor_privilege_gap_integration_test.go`、`scripts/verification/run_executor_setuid_integration.sh`、`internal/runner/base/executor/executor_test.go`

**作業内容**:

- [ ] 失敗分岐（`executor.go:282-295`）で `pc.child.started()` が true のときだけ `auditUserGroupExecution` を呼ぶ。`prepareCommand` 失敗など `runCommand` より前の return 経路では呼ばない。既存の `failureAttrs`・`User/group privilege execution failed` ログ・戻り値は変えない（02_architecture.md §3.3）。
- [ ] `assertCommandFailureWindows` を追加する。`assertFailureWindows` に倣い、"User/group command failed" の ERROR レコードを取得して既存の `assertWindowAttrs` を適用する。
- [ ] `TestPrivilegeGap_UserGroupFailureRecord` を追加する（02_architecture.md §7.2 のケース (a)）。
- [ ] `TestPrivilegeGap_TimeoutKillsChild` に失敗レコードの検証を追加する（ケース (b)）。
- [ ] `TestPrivilegeGap_CancelKillsChild` に同様の検証を追加する（ケース (c)）。
- [ ] `TestPrivilegeGap_UserGroupFailureWithoutAuditLogger` を追加する（ケース (d)）。fixture の基本オプションは常に `WithAuditLogger` を含むため、`AuditLogger` を nil にする `executor.Option` を fixture の追加オプションとして渡す。
- [ ] `TestPrivilegeGap_UserGroupNotStartedNoAudit` を追加する（ケース (e)）。`runtimeCommand` に存在しない絶対パスを渡す。
- [ ] `TestPrivilegeGap_ChildCredentialsMatchTarget` に「成功レコード 1 件のみで失敗レコードがない」検証を追加する。
- [ ] `TestPrivilegeGap_RefusedElevationDoesNotRecordWindow` に "User/group command failed" レコードが出ないことの検証を追加する（昇格拒否＝未開始の一形態。02_architecture.md §7.2 のケース外だが AC-05 の対象）。
- [ ] `TestPrivilegeGap_StagingCancellationCleansUp` に失敗レコードの検証を追加する（staging フォールバックのキャンセルで、開始・kill・staging cleanup のメトリクスを含む）。
- [ ] `TestPrivilegeGap_OutputLimitAbortsRunningChild` に失敗レコードの検証を追加する（出力上限でシグナル終了する経路。開始区間のメトリクスを含む）。
- [ ] ケース (e) は統合テストが `package executor_test` にあり `pc.child` を読めないため、状態は Phase 1 の遷移テストで固定し、統合テストはレコードと通知の不在だけを観測する。この分担をコメントに残す。
- [ ] 非特権の `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging` に、`prepareCommand` が失敗する開始前キャンセル（キャンセル済み ctx で run-as 実行）で監査レコードが出ないサブテストを追加する（AC-05 の `prepareCommand` 失敗の観測）。
- [ ] 必須テスト名の一覧（`:151`）に新設 3 テスト（`TestPrivilegeGap_UserGroupFailureRecord`・`TestPrivilegeGap_UserGroupFailureWithoutAuditLogger`・`TestPrivilegeGap_UserGroupNotStartedNoAudit`）を追加する。既存 2 テストの拡張は一覧に既にある。
- [ ] `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging` の doc コメント（`:680-683`）とサブテスト内コメント（`:716`）を、この失敗は子プロセス開始前であり監査レコードが出ないという趣旨へ更新する。アサーションは変えない。

**完了確認**: `make fmt` → `make test` → `make lint` が通る。加えて integration タグ付きのコンパイル（`go test -tags "test integration" -run '^$' ./internal/runner/base/executor/`）と setuid ゲートを skip なしで実行する（§4.5）。成功経路のレコード内容が変わらないことは既存テスト（`TestLogger_LogUserGroupExecution` と拡張後の `TestPrivilegeGap_ChildCredentialsMatchTarget`）で確認する。

### PR-3 作成ポイント: failure audit wiring and setuid integration tests

**対象ステップ**: Phase 3

**推奨タイトル**: `fix(0174): audit started run-as command failures`

**レビュー観点**: 失敗分岐で `pc.child.started()` が true のときだけ監査し、`prepareCommand` 失敗など `runCommand` より前の return 経路では呼ばないこと（02_architecture.md §3.3）／成功経路がヘルパ呼び出しのまま変わらず、レコードのレベル・属性が変わらないこと（AC-08）／setuid 統合テストがケース (a)〜(e) と成功の回帰を観測し、失敗レコードの通知メタデータとメトリクスを検証していること／新設 3 テストが `run_executor_setuid_integration.sh` の必須一覧（`:151`）に追加され、skip を許さないゲートで検証されること／`make test`・`make lint` の対象外である統合テストの型エラーが integration タグ付きコンパイルで PR 内に検出されること

**実装モデル要件**: frontier-required

**判定理由**: 実 setuid 資格情報・sudo・非 root の対象ユーザーを要する専用ゲート（skip を 1 件でも検出すると FATAL）で新設 3 テストの登録まで伴う重い統合テスト面であり、mkplan.md step 8 の panel-mode トリガー「重い統合テスト／CI／外部リソース面」に該当する。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: 0172 §10 の follow-up を解消済みとして履歴に追記

**対象ファイル**: `docs/tasks/0172_slack_notification_message_unification/03_implementation_plan.md`

**作業内容**:

- [ ] §10 の `user_group_command_failure` の follow-up に、タスク 0174 で解消した旨を追記する。実装 PR（PR-3）の番号はその PR 作成時に判明するため、その時点で記入し、実装コミットの SHA を併記する。既存の記述は履歴として残し、削除しない。
- [ ] 記録した PR 番号とコミット SHA が実在することを PR レビュー時に確認する（存在しない参照を残さない）。
- [ ] 0172 は `approved` のため、追記は履歴の追加であり決定を変えない editorial correction である旨を 0172 の文書ステータス `Comments` に記録する（requirements_process.md「Editing an approved document」）。
- [ ] docs の検証を通す: `go test -tags test ./internal/testutil/docsguard/` と `make verify-docs-checks`。

**完了確認**: 0172 §10 に解消が記録され、docs の検証が通る。

### PR-4 作成ポイント: resolved follow-up history

**対象ステップ**: Phase 4

**推奨タイトル**: `docs(0174): record the resolved user_group_command_failure follow-up`

**レビュー観点**: 0172 §10 の該当 follow-up に、解消した実装 PR の番号とコミット SHA が実在する形で追記されていること／既存の記述を履歴として残していること／0172 の `Comments` に editorial correction である旨が記録されていること／`go test -tags test ./internal/testutil/docsguard/` と `make verify-docs-checks` が green であること

**実装モデル要件**: standard

**判定理由**: 承認済み文書への履歴追記と docs 検証のみで、設計判断・高リスク分岐・未確定の実装アプローチは無く、Conditional checks・panel-mode トリガーのいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 完了条件 |
|---|---|---|
| M1: 実行状態の導入 | Phase 1 | `childState` の遷移と分類の単体テストが green で、`make test`・`make lint` が通る |
| M2: 監査ヘルパの抽出 | Phase 2 | 成功経路がヘルパ呼び出しへ置き換わり、setuid ゲートの成功回帰 `TestPrivilegeGap_ChildCredentialsMatchTarget` を含めて green |
| M3: 監査配線 | Phase 3 | 失敗分岐が開始済みのときだけ監査を呼び、setuid ゲート（skip なし）を含めて green |
| M4: 履歴更新 | Phase 4 | 0172 §10 が更新され、docs の検証が通る |

### 3.2 PR 構成

各 Phase の作業項目がその PR の対象ステップであり、PR と Phase は 1:1 に対応する。

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `childState` 型と `preparedCommand.child`、遷移 2 箇所、遷移・分類の単体テスト | frontier-recommended |
| PR-2 | Phase 2 | `auditUserGroupExecution` の抽出と成功経路の置換（挙動不変） | standard |
| PR-3 | Phase 3 | 失敗分岐への配線、setuid 統合テストと必須一覧の更新、既存コメントの更新 | frontier-required |
| PR-4 | Phase 4 | 0172 §10 の follow-up の履歴追記と docs 検証 | standard |

### 3.3 順序の根拠

02_architecture.md §8 は 4 Phase（状態の導入 → 配線 → テスト → 履歴）の優先順位を定める。本計画はその優先順位を保ちつつ、テストを対応する実装 Phase に統合し、ヘルパ抽出（Phase 2）を配線（Phase 3）から分けた。§8 の Phase 番号とは対応が異なる。統合の理由は、実装と検証を同じ PR のレビュー対象にするためである。状態の導入（Phase 1）がなければ配線（Phase 3）は開始済みを判別できず、配線がなければ setuid 統合テストは失敗レコードを観測できない。Phase 4（履歴更新）は実装 PR の番号とコミット SHA が確定した後に追記するため最後に置く。

---

## 4. テスト戦略

### 4.1 単体テスト（特権不要・`make test` で常に実行）

- `TestRunCommand_ChildStateTransitions`（`executor_supervise_test.go`）: 4 状態への遷移と、未開始の 3 経路（開始前失敗・起動区間が start を実行しない・spent）を、run-as 資格情報を伴わない実際の子プロセスで観測する。キャンセル・タイムアウトの強制終了は実際の子プロセスを kill して観測する。
- `TestChildState_StartedClassification`（同ファイル）: `started()` の分類と、列挙外の値が `false` になることを観測する。
- 既存の `TestStartPrepared_RejectsSpentCommand` と `TestRunCommand_StartWindowThatRunsNothingIsRejected` はそのまま残す。

### 4.2 setuid 統合テスト（`make executor-setuid-integration-test`）

開始済みの失敗を `executeWithUserGroup` 越しに観測するテストは、実資格情報で子を起動できる環境を要する。ケース・assert・ゲートの仕組みは 02_architecture.md §7.2 に定めたとおりで、本計画ではテスト名と登録先だけを固定する。同じ開始済みの失敗を生む既存の `TestPrivilegeGap_StagingCancellationCleansUp` と `TestPrivilegeGap_OutputLimitAbortsRunningChild` にも同じ失敗レコード（レベル・メッセージ・メトリクス）の検証を足す（両テストはすでに必須一覧にある）。実行手順は §4.5。

### 4.3 回帰

- 失敗分岐の通知属性と redaction は既存の `audit/logger_test.go` のテストが保証し、重複してテストしない。統合テストは「発火元がどのレコードを書くか」を観測する。
- Slack ペイロードは `TestSlackHandler_UserGroupCommandFailure` が保証する（変更しない）。
- 非特権環境の既存回帰 `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging` はコメントだけ更新し、アサーションを維持する。

### 4.4 実装時に行うテスト失敗確認（AC-11）

CLAUDE.md「Every test must be able to fail for its stated reason」に従い、各確認は実装時に行い、結果をコミットメッセージに記す。設計書・計画書では結果を予測しない。

| 対象 PR | 追加・変更するテスト | 実装時に行う変異（失敗を確認して復元する） |
|---|---|---|
| PR-1 | `TestRunCommand_ChildStateTransitions`（起動直後の観測を含む） | `startPrepared` の `pc.child = childRunning` 代入を一時的に外し、起動直後の観測ケースが失敗することを確認して復元する。 |
| PR-1 | `TestRunCommand_ChildStateTransitions`（終了種別の観測）・`TestSupervise_ProcessAlreadyDoneIsNotAnError` | `superviseCommand` の終了種別の確定を一時的に外し、遷移の観測と `TestSupervise_ProcessAlreadyDoneIsNotAnError` の `childTerminated` 観測が失敗することを確認して復元する。 |
| PR-1 | `TestChildState_StartedClassification` | `started()` を `childNotStarted` と列挙外の値でも true を返すように一時的に変え、分類の観測が失敗することを確認して復元する。 |
| PR-2 | `TestPrivilegeGap_ChildCredentialsMatchTarget`（既存。setuid ゲートで実行） | 成功経路の `auditUserGroupExecution` 呼び出しを一時的に外し、`assertAuditWindows` が要求する INFO 成功レコードが得られず失敗することを確認して復元する。 |
| PR-3 | `TestPrivilegeGap_UserGroupFailureRecord`・`TestPrivilegeGap_TimeoutKillsChild`・`TestPrivilegeGap_CancelKillsChild` | 失敗分岐の `auditUserGroupExecution` 呼び出しを一時的に外し、各テストが要求する ERROR 失敗レコードが得られず失敗することを確認して復元する。 |
| PR-3 | `TestPrivilegeGap_UserGroupFailureWithoutAuditLogger` | `auditUserGroupExecution` の `e.AuditLogger == nil` ガードを一時的に外し、nil ロガーで panic して失敗することを確認して復元する。 |
| PR-3 | `TestPrivilegeGap_UserGroupNotStartedNoAudit` | `started()` が `childNotStarted` でも true を返すように一時的に変え、未開始の Start 失敗（プレースホルダの非 nil `Result` が返る経路）で失敗レコードが出て失敗することを確認して復元する。nil `Result` を返す昇格拒否の経路は対象にしない。 |
| PR-3 | `TestPrivilegeGap_ChildCredentialsMatchTarget`（成功レコード 1 件の検証を追加） | 成功経路の `auditUserGroupExecution` 呼び出しを一時的に外し、成功レコードの検証が失敗することを確認して復元する。 |
| PR-3 | `TestPrivilegeGap_RefusedElevationDoesNotRecordWindow` | 失敗分岐の `pc.child.started()` ガードを一時的に外して未開始の昇格拒否でも監査を呼ぶようにし、このテストが失敗することを確認して復元する。 |
| PR-3 | `TestPrivilegeGap_StagingCancellationCleansUp`・`TestPrivilegeGap_OutputLimitAbortsRunningChild` | 失敗分岐の `auditUserGroupExecution` 呼び出しを一時的に外し、各テストが要求する失敗レコードとメトリクス属性が得られず失敗することを確認して復元する。 |
| PR-3 | `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging`（`prepareCommand` 失敗のサブテストを追加） | `prepareCommand` 失敗の return 経路で `auditUserGroupExecution` を呼ぶように一時的に変え、監査レコードが出て失敗することを確認して復元する。 |

setuid 統合テストを使う確認（表のデータ行の 4・6・7・8・9・10 番目）は、setuid ゲートを実行できない環境では未実施として §5 のリスク行と同じ扱いで記録する。7 番目の変異は非特権の `TestChildState_StartedClassification` も失敗させるため、環境が無い場合はその失敗を部分的な証拠として記録する。

### 4.5 実行手順と環境

- 特権不要: `make test`（CGO=1 `-race` と CGO=0 の 2 回）・`make lint`。
- setuid ゲート: `TEST_RUNAS_TARGET_USER=<fixture-user> make executor-setuid-integration-test`。非 root の実行者、非 root の対象ユーザー、sudo が必要で、環境が揃わない場合はスクリプトが FATAL を返す（skip を許さない）。PR-2（成功回帰 `TestPrivilegeGap_ChildCredentialsMatchTarget`）と PR-3（配線の検証）がこのゲートを必要とする。CI の非特権レグ `make executor-privileged-integration-test` は skip を許容するため、配線の保証は setuid ゲートが担う。
- integration タグ付きコンパイル: `go test -tags "test integration" -run '^$' ./internal/runner/base/executor/`（PR-3。`make test` は `-tags test`、`make lint` は `--build-tags test` のため、統合テストの型エラーはこの確認で検出する）。
- 文書: `go test -tags test ./internal/testutil/docsguard/`・`make verify-docs-checks`。

---

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| setuid ゲートを実行できない環境（sudo・対象ユーザーなし） | 配線の実行検証が未実施のまま残る | スクリプトが skip を拒否する。実行できる環境で §4.5 を実施し、結果を PR-2・PR-3 に記録する。実行できない場合は、配線が未検証であることと、§4.4 の setuid 依存の失敗確認が未実施であることを明示する |
| 状態の代入箇所の片側漏れ | 監査の欠落または誤記録 | Phase 1 の遷移テストで 4 状態と未開始 3 経路を観測する |
| 失敗分岐の追加が成功経路へ波及 | AC-08 違反 | 成功経路はヘルパ呼び出しへの置換だけにする。既存の成功レコードテストで観測する |
| 0172 の approved 文書への追記 | プロセス上の懸念 | 決定を変えない editorial correction として `Comments` に記録する（Phase 4） |
| 通知量の増加（タイムアウト・キャンセルも失敗通知になる） | 通常キューの飽和と破棄（0172 §5.4） | 0172 のキューは飽和時に破棄を記録し flush 時に種別別集計する設計であり、増加はその想定内とする。件数の抑制が必要になった場合は別タスクとして扱う |
| `executor_test.go` の stale なコメント | レビュー時の混乱 | Phase 3 で更新する |

---

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: Phase 1）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2）
- [ ] PR-3 マージ済み（対象ステップ: Phase 3）
- [ ] PR-4 マージ済み（対象ステップ: Phase 4）
- [ ] すべての AC が §7 の検証で green
- [ ] `make deadcode` の出力に新規の死んだコードがない（`started()` は PR-1 の時点ではテストのみから到達するため、配線後の PR-3 以降に確認する）

---

## 7. 受け入れ基準の検証

| AC | 実装タスク | 検証（種別 / アーティファクト） |
|---|---|---|
| AC-01 | Phase 1・Phase 3 | `test`: `executor_supervise_test.go::TestRunCommand_ChildStateTransitions`（終了コードと状態）、`executor_privilege_gap_integration_test.go::TestPrivilegeGap_UserGroupFailureRecord`・`::TestPrivilegeGap_TimeoutKillsChild`・`::TestPrivilegeGap_CancelKillsChild`・`::TestPrivilegeGap_OutputLimitAbortsRunningChild`・`::TestPrivilegeGap_StagingCancellationCleansUp`（ERROR レコードと `exit_code`）。kill が exit 0 の回収と競合した場合は INFO 成功レコードとなり AC-01 の対象外（02_architecture.md §4 の補足） |
| AC-02 | Phase 3 | `test`: `TestPrivilegeGap_UserGroupFailureRecord`・`TestPrivilegeGap_TimeoutKillsChild`・`TestPrivilegeGap_CancelKillsChild`（ケース (a)〜(c)。`message_type`・通知コンテキスト `CommandScope(group, command)`） |
| AC-03 | Phase 3 | `test`: `TestPrivilegeGap_UserGroupFailureRecord`（相異なる stdout／stderr と redaction 後の値）、`internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_OutputMasking` |
| AC-04 | Phase 3 | `test`: `TestPrivilegeGap_ChildCredentialsMatchTarget`（成功レコード 1 件のみ）、`audit/logger_test.go::TestLogger_LogUserGroupExecution` |
| AC-05 | Phase 1・Phase 3 | `test`: `TestPrivilegeGap_UserGroupNotStartedNoAudit`、`TestPrivilegeGap_RefusedElevationDoesNotRecordWindow`、`executor_test.go::TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging`（開始前 EPERM と、キャンセルによる `prepareCommand` 失敗を含む）、`TestRunCommand_ChildStateTransitions`（未開始 3 経路） |
| AC-06 | Phase 1・Phase 3 | `test`: `TestRunCommand_ChildStateTransitions`（強制終了の状態）、`TestPrivilegeGap_UserGroupFailureRecord`（非ゼロ）、`TestPrivilegeGap_TimeoutKillsChild`・`TestPrivilegeGap_CancelKillsChild`（強制終了） |
| AC-07 | Phase 3 | `test`: `TestPrivilegeGap_UserGroupNotStartedNoAudit` |
| AC-08 | Phase 2・Phase 3 | `test`: `TestPrivilegeGap_ChildCredentialsMatchTarget`、`audit/logger_test.go::TestLogger_LogUserGroupExecution` |
| AC-09 | 変更なし | `test`: `internal/logging/slack_handler_test.go::TestSlackHandler_UserGroupCommandFailure`（`make test` で実行） |
| AC-10 | 各 Phase | `static`: `make fmt`・`make test`・`make lint`（各 Phase の完了確認） |
| AC-11 | Phase 1・2・3（§4.4） | `test`: §4.4 の PR 別の表に従い、追加・変更した各テストについて変異の失敗確認を実施しコミットメッセージに記録する |

---

## 8. Success Criteria

- **機能**: AC-01〜AC-09 を検証するテストが green。
- **品質**: `make fmt`・`make test`・`make lint` が green。setuid ゲートが skip なしで green。
- **セキュリティ**: 開始済みの実行は監査レコードがちょうど 1 件、未開始の失敗は 0 件という不変条件がテストで観測される。
- **文書**: 0172 §10 に解消が記録され、`make verify-docs-checks` と docsguard テストが green。

---

## 9. 次のステップ

- PR-1 から順に実装する。PR ごとにブランチとコミットを分け、AC-11 の失敗確認の結果を各コミットメッセージに記す。
- setuid ゲートを実行できる環境で §4.5 を実施し、結果を PR-3 に記録する。
- 必要に応じて、実チャンネルで `user_group_command_failure` の Slack 表示を確認する（手動。AC の対象外）。
