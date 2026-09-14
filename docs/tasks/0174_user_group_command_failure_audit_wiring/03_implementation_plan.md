# 実装計画書: run-as コマンド失敗時の監査記録と `user_group_command_failure` 通知の配線

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-14 |
| Review date | `-` |
| Reviewer | `-` |
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
4. Phase の順序と内容は 02_architecture.md §8 の実装優先順位に従う。

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
| 既存テスト | `TestLogger_LogUserGroupExecution`／`TestLogUserGroupExecution_OutputMasking` | `internal/runner/base/audit/logger_test.go:38,228` | 失敗分岐の通知属性と redaction の既存保証 |
| 既存テスト | `TestSlackHandler_UserGroupCommandFailure` | `internal/logging/slack_handler_test.go:2032` | Slack ペイロードの既存保証 |

**変更が必要な既存コメント**

- `internal/runner/base/executor/executor_test.go:680-683` のテスト doc コメントと `:716` のサブテスト内コメントは「監査は成功時にのみ発火する」と述べている。配線後はこの前提が変わるため、Phase 3 で「この失敗は子プロセス開始前なのでレコードが出ない」という趣旨へ更新する。アサーションは変えない。

---

## 2. 実装ステップ

### Phase 1: `childState` 型と `preparedCommand.child`、状態を刻む 3 箇所（遷移 2 箇所と零値のまま 1 箇所）を実装

**対象ファイル**: `internal/runner/base/executor/command_lifecycle.go`

**作業内容**:

- [ ] `childState` 型と定数 `childNotStarted`（零値）・`childRunning`・`childExited`・`childTerminated` を追加する。意味は 02_architecture.md §3.1 のとおり。Go のコメントは英語で書く。
- [ ] `started()` メソッドを追加する。`childRunning`・`childExited`・`childTerminated` を `true`、`childNotStarted` と未知の値を `false` とする（02_architecture.md §3.1）。
- [ ] `preparedCommand` に `child childState` フィールドを追加する。
- [ ] `startPrepared` の `pc.execCmd.Start()` 成功直後に `pc.child = childRunning` を代入する。
- [ ] `superviseCommand` で `killed` が確定した後、`Result` を組み立てる前に `pc.child` を `childExited`（kill 経路を通っていない）または `childTerminated`（kill 経路を通った）へ確定する（02_architecture.md §3.2 の表）。
- [ ] 既存の戻り値・ログ・`Result` の値・エラーを変更しない。

**完了確認**: `make fmt`（Go を変更するため）→ `make test` → `make lint` が通る。この時点では配線がないため実行時の挙動は変わらない。

### Phase 2: `auditUserGroupExecution` を抽出し、失敗分岐へ開始済みのときだけ配線

**対象ファイル**: `internal/runner/base/executor/executor.go`

**作業内容**:

- [ ] `auditUserGroupExecution(ctx context.Context, cmd *runnertypes.RuntimeCommand, result *Result, startTime time.Time, metrics audit.PrivilegeMetrics)` を追加する。`e.AuditLogger == nil` は no-op とする。中身は既存の `audit.ExecutionResult` の組み立てと `LogUserGroupExecution` の呼び出しをそのまま移す（属性・時間計測・redaction を変えない）。
- [ ] 成功経路の監査ブロック（`executor.go:297-306`）を `auditUserGroupExecution` 呼び出しへ置き換える。
- [ ] 失敗分岐（`executor.go:282-295`）で `pc.child.started()` が true のときだけ `auditUserGroupExecution` を呼ぶ。`prepareCommand` 失敗など `runCommand` より前の return 経路では呼ばない。既存の `failureAttrs`・`User/group privilege execution failed` ログ・戻り値は変えない（02_architecture.md §3.3）。
- [ ] 成功経路の `Result` の非 nil 前提は現状のままとする。

**完了確認**: `make fmt` → `make test` → `make lint` が通る。成功経路のレコード内容が変わらないことは既存テスト（`TestLogger_LogUserGroupExecution` と setuid 成功テスト）で確認する。

### Phase 3: 遷移の単体テスト、配線の setuid 統合テスト、未開始・成功の回帰テスト

**対象ファイル**: `internal/runner/base/executor/executor_supervise_test.go`、`internal/runner/base/executor/executor_privilege_gap_integration_test.go`、`scripts/verification/run_executor_setuid_integration.sh`、`internal/runner/base/executor/executor_test.go`

**作業内容**:

3.1 遷移の単体テスト（`executor_supervise_test.go`、`package executor`）:

- [ ] `TestRunCommand_ChildStateTransitions` を追加する。`prepareForSupervise` と `runUnprivileged` を再利用し、正常終了・非ゼロ終了・キャンセル／タイムアウトによる強制終了・開始前失敗（存在しない絶対パス）・起動区間が start を実行しない場合・spent の各ケースで、`pc.child`・`started()`・`Result.ExitCode` を観測する。強制終了は `WithKillGraceDelay` で待ち時間を短縮する。spent は `TestStartPrepared_RejectsSpentCommand`（`executor_lifecycle_test.go:390`）と同じく `&preparedCommand{spent: true}` を直接組んで入力する。
- [ ] 起動直後を観測するケースを同テストに含める。`startForSupervise` で開始した後、`superviseCommand` を呼ぶ前に `pc.child == childRunning` かつ `started()` が true であることを観測する。`startPrepared` の遷移は `superviseCommand` が終了種別で上書きするため、`runCommand` の戻り値だけでは観測できない。
- [ ] `TestSupervise_ProcessAlreadyDoneIsNotAnError`（`executor_supervise_test.go:317`）に `pc.child == childTerminated` の観測を追加する（kill が exit 0 の回収と競合しても開始済みのままであることを状態で固定する）。
- [ ] `TestChildState_StartedClassification` を追加する。4 定数に対する `started()` の分類と、列挙外の値（例: `childState(99)`）が `false` になることを観測する（02_architecture.md §7.1）。

3.2 配線の setuid 統合テスト（`executor_privilege_gap_integration_test.go`）。ケースと assert の内容は 02_architecture.md §7.2 の表に従う:

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
- [ ] ケース (e) は統合テストが `package executor_test` にあり `pc.child` を読めないため、状態は 3.1 の遷移テストで固定し、統合テストはレコードと通知の不在だけを観測する。この分担をコメントに残す。
- [ ] 非特権の `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging` に、`prepareCommand` が失敗する開始前キャンセル（キャンセル済み ctx で run-as 実行）で監査レコードが出ないサブテストを追加する（AC-05 の `prepareCommand` 失敗の観測）。

3.3 必須テストの登録（`run_executor_setuid_integration.sh`）:

- [ ] 必須テスト名の一覧（`:151`）に新設 3 テスト（`TestPrivilegeGap_UserGroupFailureRecord`・`TestPrivilegeGap_UserGroupFailureWithoutAuditLogger`・`TestPrivilegeGap_UserGroupNotStartedNoAudit`）を追加する。既存 2 テストの拡張は一覧に既にある。

3.4 既存コメントの更新（`executor_test.go`）:

- [ ] `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging` の doc コメント（`:680-683`）とサブテスト内コメント（`:716`）を、この失敗は子プロセス開始前であり監査レコードが出ないという趣旨へ更新する。アサーションは変えない。

**完了確認**: `make fmt` → `make test` → `make lint` が通る。加えて setuid ゲートを skip なしで実行する（§4.5）。

### Phase 4: 0172 §10 の follow-up を解消済みとして履歴に追記

**対象ファイル**: `docs/tasks/0172_slack_notification_message_unification/03_implementation_plan.md`

**作業内容**:

- [ ] §10 の `user_group_command_failure` の follow-up に、タスク 0174 で解消した旨を追記する。PR 番号は PR 作成時に判明するため、その時点で記入し、実装コミットの SHA を併記する。既存の記述は履歴として残し、削除しない。
- [ ] 記録した PR 番号とコミット SHA が実在することを PR レビュー時に確認する（存在しない参照を残さない）。
- [ ] 0172 は `approved` のため、追記は履歴の追加であり決定を変えない editorial correction である旨を 0172 の文書ステータス `Comments` に記録する（requirements_process.md「Editing an approved document」）。
- [ ] docs の検証を通す: `go test -tags test ./internal/testutil/docsguard/` と `make verify-docs-checks`。

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 完了条件 |
|---|---|---|
| M1: 実行状態の導入 | Phase 1 | `childState` の遷移が実装され、`make test`・`make lint` が通る |
| M2: 監査配線 | Phase 2 | 失敗分岐が開始済みのときだけ監査を呼び、成功経路の属性が変わらない |
| M3: テストで固定 | Phase 3 | `make test` と setuid ゲート（skip なし）が通る |
| M4: 履歴更新 | Phase 4 | 0172 §10 が更新され（PR 内で完了）、docs の検証が通る |

### 3.2 順序の根拠

02_architecture.md §8 の実装優先順位どおりである。状態の導入（Phase 1）がなければ配線（Phase 2）は開始済みを判別できず、配線がなければ統合テスト（Phase 3）は失敗レコードを観測できない。Phase 4 は PR 番号が確定した後に追記するため最後に置く。

---

## 4. テスト戦略

### 4.1 単体テスト（特権不要・`make test` で常に実行）

- `TestRunCommand_ChildStateTransitions`（`executor_supervise_test.go`）: 4 状態への遷移と、未開始の 3 経路（開始前失敗・起動区間が start を実行しない・spent）を、run-as 資格情報を伴わない実際の子プロセスで観測する。キャンセル・タイムアウトの強制終了は実際の子プロセスを kill して観測する。
- `TestChildState_StartedClassification`（同ファイル）: `started()` の分類と、列挙外の値が `false` になることを観測する。
- 既存の `TestStartPrepared_RejectsSpentCommand` と `TestRunCommand_StartWindowThatRunsNothingIsRejected` はそのまま残す。

### 4.2 setuid 統合テスト（`make executor-setuid-integration-test`）

開始済みの失敗を `executeWithUserGroup` 越しに観測するテストは、実資格情報で子を起動できる環境を要する。ケース・assert・ゲートの仕組みは 02_architecture.md §7.2 に定めたとおりで、本計画ではテスト名と登録先だけを固定する。同じ開始済みの失敗を生む既存の `TestPrivilegeGap_StagingCancellationCleansUp` と `TestPrivilegeGap_OutputLimitAbortsRunningChild` にも同じ検証を足す（両テストはすでに必須一覧にある）。実行手順は §4.5。

### 4.3 回帰

- 失敗分岐の通知属性と redaction は既存の `audit/logger_test.go` のテストが保証し、重複してテストしない。統合テストは「発火元がどのレコードを書くか」を観測する。
- Slack ペイロードは `TestSlackHandler_UserGroupCommandFailure` が保証する（変更しない）。
- 非特権環境の既存回帰 `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging` はコメントだけ更新し、アサーションを維持する。

### 4.4 実装時に行うテスト失敗確認（AC-11）

CLAUDE.md「Every test must be able to fail for its stated reason」に従い、次の確認を実装時に行い、結果をコミットメッセージに記す。設計書・計画書では結果を予測しない。

- [ ] `startPrepared` の `childRunning` 代入を一時的に外し、遷移テストのうち起動直後を観測するケースが失敗することを確認して復元する。
- [ ] `superviseCommand` の終了種別の確定を一時的に外し、遷移テストが失敗することを確認して復元する。
- [ ] `started()` が `childNotStarted` でも true を返すように一時的に変え、`TestPrivilegeGap_UserGroupNotStartedNoAudit` だけを `-run` で実行して失敗を確認して復元する。この確認は Start 失敗の経路（プレースホルダの非 nil `Result` が返る）を対象にし、nil `Result` を返す昇格拒否の経路は対象にしない。
- [ ] 失敗分岐の監査呼び出しを一時的に外し、ケース (a)〜(c) のいずれかが失敗することを確認して復元する。

### 4.5 実行手順と環境

- 特権不要: `make test`（CGO=1 `-race` と CGO=0 の 2 回）・`make lint`。
- setuid ゲート: `TEST_RUNAS_TARGET_USER=<fixture-user> make executor-setuid-integration-test`。非 root の実行者、非 root の対象ユーザー、sudo が必要で、環境が揃わない場合はスクリプトが FATAL を返す（skip を許さない）。CI の非特権レグ `make executor-privileged-integration-test` は skip を許容するため、配線の保証は setuid ゲートが担う。
- 文書: `go test -tags test ./internal/testutil/docsguard/`・`make verify-docs-checks`。

---

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| setuid ゲートを実行できない環境（sudo・対象ユーザーなし） | 配線の実行検証が未実施のまま残る | スクリプトが skip を拒否する。実行できる環境で §4.5 を実施し、結果をコミットメッセージと PR に記録する。実行できない場合は未検証であることを明示する |
| 状態の代入箇所の片側漏れ | 監査の欠落または誤記録 | Phase 3.1 の遷移テストで 4 状態と未開始 3 経路を観測する |
| 失敗分岐の追加が成功経路へ波及 | AC-08 違反 | 成功経路はヘルパ呼び出しへの置換だけにする。既存の成功レコードテストで観測する |
| 0172 の approved 文書への追記 | プロセス上の懸念 | 決定を変えない editorial correction として `Comments` に記録する（Phase 4） |
| 通知量の増加（タイムアウト・キャンセルも失敗通知になる） | 通常キューの飽和と破棄（0172 §5.4） | 0172 のキューは飽和時に破棄を記録し flush 時に種別別集計する設計であり、増加はその想定内とする。件数の抑制が必要になった場合は別タスクとして扱う |
| `executor_test.go` の stale なコメント | レビュー時の混乱 | Phase 3.4 で更新する |

---

## 6. 実装チェックリスト

### Phase 1
- [ ] `childState` 型と 4 定数を追加
- [ ] `started()` を追加
- [ ] `preparedCommand.child` を追加
- [ ] `startPrepared` の遷移
- [ ] `superviseCommand` の終了種別確定
- [ ] `make fmt`・`make test`・`make lint` が green

### Phase 2
- [ ] `auditUserGroupExecution` を追加
- [ ] 成功経路をヘルパ呼び出しへ置換
- [ ] 失敗分岐へ `pc.child.started()` ガード付きで配線
- [ ] `make fmt`・`make test`・`make lint` が green

### Phase 3
- [ ] `TestRunCommand_ChildStateTransitions` を追加（起動直後の `childRunning` 観測を含む）
- [ ] `TestChildState_StartedClassification` を追加
- [ ] `TestSupervise_ProcessAlreadyDoneIsNotAnError` に `childTerminated` の観測を追加
- [ ] `assertCommandFailureWindows` を追加
- [ ] `TestPrivilegeGap_UserGroupFailureRecord` を追加
- [ ] `TestPrivilegeGap_TimeoutKillsChild` を拡張
- [ ] `TestPrivilegeGap_CancelKillsChild` を拡張
- [ ] `TestPrivilegeGap_StagingCancellationCleansUp` を拡張
- [ ] `TestPrivilegeGap_OutputLimitAbortsRunningChild` を拡張
- [ ] `TestPrivilegeGap_UserGroupFailureWithoutAuditLogger` を追加
- [ ] `TestPrivilegeGap_UserGroupNotStartedNoAudit` を追加
- [ ] `TestPrivilegeGap_ChildCredentialsMatchTarget` を拡張
- [ ] `TestPrivilegeGap_RefusedElevationDoesNotRecordWindow` を拡張
- [ ] `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging` に `prepareCommand` 失敗のサブテストを追加
- [ ] 必須テスト一覧へ新設 3 テストを追加
- [ ] `executor_test.go` のコメント 2 箇所を更新
- [ ] AC-11 の失敗確認 4 件を実施しコミットメッセージに記録
- [ ] `make test`・`make lint`・setuid ゲート（skip なし）が green

### Phase 4
- [ ] 0172 §10 の follow-up に解消を追記
- [ ] 0172 の `Comments` に editorial correction である旨を記録
- [ ] docs の検証が green

### 全体
- [ ] すべての AC が §7 の検証で green
- [ ] `make deadcode` の出力に新規の死んだコードがない

---

## 7. 受け入れ基準の検証

| AC | 実装タスク | 検証（種別 / アーティファクト） |
|---|---|---|
| AC-01 | Phase 1・Phase 2 | `test`: `executor_supervise_test.go::TestRunCommand_ChildStateTransitions`（終了コードと状態）、`executor_privilege_gap_integration_test.go::TestPrivilegeGap_UserGroupFailureRecord`・`::TestPrivilegeGap_TimeoutKillsChild`・`::TestPrivilegeGap_CancelKillsChild`・`::TestPrivilegeGap_OutputLimitAbortsRunningChild`・`::TestPrivilegeGap_StagingCancellationCleansUp`（ERROR レコードと `exit_code`）。kill が exit 0 の回収と競合した場合は INFO 成功レコードとなり AC-01 の対象外（02_architecture.md §4 の補足） |
| AC-02 | Phase 2 | `test`: 上記 setuid 5 テストの通知メタデータ検証（`message_type`・通知コンテキスト） |
| AC-03 | Phase 2 | `test`: `TestPrivilegeGap_UserGroupFailureRecord`（相異なる stdout／stderr と redaction 後の値）、`internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_OutputMasking` |
| AC-04 | Phase 2 | `test`: `TestPrivilegeGap_ChildCredentialsMatchTarget`（成功レコード 1 件のみ）、`audit/logger_test.go::TestLogger_LogUserGroupExecution` |
| AC-05 | Phase 1・Phase 2・Phase 3 | `test`: `TestPrivilegeGap_UserGroupNotStartedNoAudit`、`TestPrivilegeGap_RefusedElevationDoesNotRecordWindow`、`executor_test.go::TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging`（開始前 EPERM と、キャンセルによる `prepareCommand` 失敗を含む）、`TestRunCommand_ChildStateTransitions`（未開始 3 経路） |
| AC-06 | Phase 3.1・3.2 | `test`: `TestRunCommand_ChildStateTransitions`（強制終了の状態）、`TestPrivilegeGap_UserGroupFailureRecord`（非ゼロ）、`TestPrivilegeGap_TimeoutKillsChild`・`TestPrivilegeGap_CancelKillsChild`（強制終了） |
| AC-07 | Phase 3.2 | `test`: `TestPrivilegeGap_UserGroupNotStartedNoAudit` |
| AC-08 | Phase 2 | `test`: `TestPrivilegeGap_ChildCredentialsMatchTarget`、`audit/logger_test.go::TestLogger_LogUserGroupExecution` |
| AC-09 | 変更なし | `test`: `internal/logging/slack_handler_test.go::TestSlackHandler_UserGroupCommandFailure`（`make test` で実行） |
| AC-10 | 各 Phase | `static`: `make fmt`・`make test`・`make lint`（各 Phase の完了確認） |
| AC-11 | Phase 1〜3（§4.4） | `test`: §4.4 の失敗確認を実施しコミットメッセージに記録する。対象は `TestRunCommand_ChildStateTransitions`・`TestSupervise_ProcessAlreadyDoneIsNotAnError`・`TestPrivilegeGap_UserGroupNotStartedNoAudit`・`TestPrivilegeGap_UserGroupFailureRecord` |

---

## 8. Success Criteria

- **機能**: AC-01〜AC-09 を検証するテストが green。
- **品質**: `make fmt`・`make test`・`make lint` が green。setuid ゲートが skip なしで green。
- **セキュリティ**: 開始済みの実行は監査レコードがちょうど 1 件、未開始の失敗は 0 件という不変条件がテストで観測される。
- **文書**: 0172 §10 に解消が記録され、`make verify-docs-checks` と docsguard テストが green。

---

## 9. 次のステップ

- 本計画書のレビューと承認（status を `approved` にする）。
- 承認後、Phase 1 から実装する。Phase ごとにコミットを分け、AC-11 の失敗確認の結果を各コミットメッセージに記す。
- setuid ゲートを実行できる環境で §4.5 を実施し、結果を PR に記録する。
- 必要に応じて、実チャンネルで `user_group_command_failure` の Slack 表示を確認する（手動。AC の対象外）。
