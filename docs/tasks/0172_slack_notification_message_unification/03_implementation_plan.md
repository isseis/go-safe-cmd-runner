# 実装計画書: Slack 通知メッセージの書式統一とスコープ情報の付与

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-09 |
| Review date | 2026-09-11 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- [01_requirements.md](01_requirements.md) — 受け入れ基準（AC-01〜AC-33）の定義元
- [02_architecture.md](02_architecture.md) — 設計の定義元。本書は設計を再掲せず参照する
- [requirements_process.md](../../dev/developer_guide/requirements_process.md) — 本書の必須節と AC 追跡の規約
- [test_organization.md](../../dev/developer_guide/test_organization.md) — テストヘルパーの配置規約

## 1. 実装概要

### 1.1 目的

本番で発火しない 3 個の通知種別を削除したうえで、存続する 3 種別の Slack 通知に対して、
発生箇所を表す通知コンテキストと、製品名を先頭に置く統一書式を導入する。あわせて、種別の
一覧・メッセージの組み立て・キュー優先度を単一の通知種別定義から引く構造へ移し、種別の登録
漏れが黙って通る現在の状態をなくす。設計の詳細は 02_architecture.md を参照する。

### 1.2 実装原則

1. 02_architecture.md の設計に従い、本書では設計判断を作り直さない。設計に無い判断が必要に
   なった場合は、実装を止めて 02_architecture.md を先に改訂する。
2. 削除（Phase 1〜3）は種別ごとに独立したコミットとし、1 件ずつ revert できる形を保つ。
3. Go のコメント・識別子・文字列リテラルはすべて英語で書く。本書の説明文だけが日本語である。
4. 各 Phase の終わりに `make fmt`（Go を変更した場合）、`make test`、`make lint` を通す。
   Phase 3 の後と Phase 7 では `make deadcode` も実行する。
5. 既存の実装・テスト・ヘルパーを優先して再利用する。とくに構文木を走査する検査は
   `internal/testutil` の既存ヘルパーを使い、走査対象ファイルの定義を複製しない。

### 1.3 既存コード調査結果

実装前に HEAD を調査した結果を、対象領域ごとに記す。既存コードに手を入れる必要がない領域は
省いてある。

#### 削除対象 3 種別の所在

| 種別 | 本番コードの所在 | テストの所在 |
|---|---|---|
| `privileged_command_failure` | `internal/logging/slack_sender.go` の `messageTypePrivilegedCommandFailure`、`internal/logging/slack_handler.go` の `Handle` 分岐と `buildPrivilegedCommandFailure`、`internal/common/logschema.go` の `PrivilegedCommandFailureAttrs` | `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` 内のケース「privileged command failure」 |
| `security_alert` | 上記 3 ファイルの対応する定義に加え、`internal/common/logschema.go` の `SecurityAlertAttrs`・`SeverityCritical`・`SeverityHigh`、`internal/logging/slack_sender.go` の `isHighPriority`、`internal/runner/base/audit/logger.go` の `LogSecurityEvent` | `internal/runner/base/audit/logger_test.go` の 4 テストと補助型 `sensitiveLogValuer`、`internal/logging/slack_handler_test.go` のケース「security alert」、`internal/logging/slack_sender_test.go` のヘルパー `securityAlertRecord` とその 3 箇所の利用 |
| `privilege_escalation_failure` | 上記に加え、`internal/common/logschema.go` の `PrivilegeEscalationFailureAttrs`、`internal/runner/base/audit/logger.go` の `LogPrivilegeEscalation` | `internal/runner/base/audit/logger_test.go` の 2 テスト、`internal/logging/slack_handler_test.go` のケース「privilege escalation failure」 |

- `LogSecurityEvent` と `LogPrivilegeEscalation` は、`internal/` と `cmd/` を通しで検索しても
  定義と `_test.go` 以外に呼び出し元が無い（要件定義の調査結果を HEAD で再確認した）。
- **紛らわしい同名の別物**が 3 つある。いずれも本タスクの削除対象ではない。
  - `internal/runner/runerrors` の `ErrorSeverityCritical`。`common.SeverityCritical` とは
    無関係の別パッケージの列挙である。検索は `common.Severity` で修飾して行う。
  - `internal/logging/security.go` の `SecurityLogger`。タイムアウト関連のセキュリティ事象を
    記録する型で、`security_alert` 通知とは無関係である。
  - `internal/runner/resource/manager.go:24` の `ErrEmptyCommandName`。実行境界で空のコマンド
    名を弾く既存センチネルであり、Phase 4 で設定境界に足すセンチネルとは別物である
    （§4.4 で名前の扱いを決める）。
- `internal/logging/slack_handler.go` の絵文字定数のうち、`emojiAlert` は
  `buildSecurityAlert` と `buildPreExecutionError` の 2 箇所で使われている。Phase 1〜3 の削除
  後も `buildPreExecutionError` に残り、Phase 5 で Text 行がレベル由来の表示へ移った時点で
  未使用になる。Phase 5 に削除タスクを置く。
- `internal/runner/base/audit/logger_test.go:25` の補助型 `sensitiveLogValuer` は、参照が
  同ファイル 483 行目の 1 箇所だけで、それは Phase 2 で削除する
  `TestLogSecurityEvent_DetailsRedaction` の中にある。テストだけを消すと未使用型が残り
  `make lint` が落ちるため、型と doc コメントも同じコミットで消す。

#### 通知の発火元（`slack_notify` を書く本番コード）

HEAD で `slack_notify` を書く本番コードは 5 箇所ある。うち 2 箇所（`audit/logger.go` の
`LogPrivilegeEscalation` と `LogSecurityEvent`）は Phase 2〜3 で消えるため、**Phase 3 完了後に
残る発火元は次の 3 箇所**である。

| 発火元 | 現在の書き方 | 必要な変更 |
|---|---|---|
| `internal/runner/runner.go` の `logGroupExecutionSummary` | `slog.Info`／`slog.Error` に `...any` の可変長引数で `"slack_notify", true`、`"message_type", "command_group_summary"` を渡す | `LogAttrs` を使う形へ変え、`logging.NotificationAttrs` の返す `[]slog.Attr` を渡す |
| `internal/logging/pre_execution_error.go` の `handleErrorCommon` | `errorHandlingParams` の `slackNotify`・`slogMsgType` から自前で組み立てる。記録は `slog.Error(msg, ...any)` である | 呼び出し元が渡す `[]slog.Attr` をそのまま記録する形へ変える（02_architecture.md §3.2）。`[]slog.Attr` を渡すため `slog.LogAttrs` へ切り替える |
| `internal/runner/base/audit/logger.go` の `LogUserGroupExecution` | 文字列リテラル `"message_type"`・`"user_group_command_failure"` を直接書く。`command_name`・`exit_code`・`stdout`・`stderr` もリテラルである | `logging.NotificationAttrs` へ移し、4 個の属性名を `common.UserGroupCommandFailureAttrs` から引く |

`audit` パッケージは現在 `internal/logging` を import していない。02_architecture.md §2.1 の
とおり、この辺を 1 本追加する。`go list -deps ./internal/logging` は `ansicolor`・`common`・
`groupmembership`・`safefileio`・`terminal` だけを返し、`audit` も `runnertypes` も含まない
ため循環は生じない。

#### 表示安全な補間契約の配置

02_architecture.md §3.5 は補間契約を実装する関数の**パッケージを指定していない**。一方で
契約の利用者は 3 者ある。

| 利用者 | 位置 | 用途 |
|---|---|---|
| 通知コンテキストの妥当性判定 | `internal/common`（§3.1「読み側の復元処理は `internal/common` の 1 箇所に置く」） | group 名・command 名が契約を通すと表示できる文字を残すかの判定 |
| 識別子の設定検証 | `internal/runner/config` および `internal/runner/bootstrap`（§3.1） | 同上 |
| 共通エンベロープ | `internal/logging`（§3.5） | Text 行と添付フィールドへの補間 |

`internal/logging` は `internal/common` に依存しているため、契約を `internal/logging` に
置くと `internal/common` から呼べない（import 循環）。したがって**補間契約は
`internal/common` に置く**。`internal/runner/config` は既に `internal/common` に依存して
おり、新しいパッケージ間の辺は生じない。この配置なら 3 者が同じ実装を参照でき、§3.5 の
「他節はこの契約を参照し、同じ規則を書き写さない」も満たせる。

02_architecture.md §2.2 のコンポーネント配置表には、この契約を置くファイルの行が無い。
設計判断そのものは変わらない（§3.5 はパッケージを定めていない）ため実装は進められるが、
表に行を足す文書上の追補が要る。Phase 6 の文書更新に含める。

#### `PreExecutionError` のリテラルと `HandlePreExecutionError` の呼び出し元

`rg -n "PreExecutionError\{" --type go cmd internal` の非テスト結果は 17 箇所で、内訳は
`cmd/runner/main.go` が 11、`internal/runner/bootstrap/config.go` が 4、
`internal/runner/bootstrap/environment.go` が 2 である。02_architecture.md §3.2 の記述と一致
する。

`HandlePreExecutionError` の本番呼び出し元は `cmd/runner/main.go` に 5 箇所
（132、177、186、217、221 行目）、`internal/runner/runner.go` に 1 箇所（429 行目）ある。
このうち `main.go` の 4 箇所（132、177、186、221）と `runner.go` の 1 箇所は、現在リテラルを
作らず位置引数を組み立てている。構造体を渡す形へ移すと、この **5 箇所に新しい
`PreExecutionError` 複合リテラルが増える**。上記 17 箇所は移行前の数であり、Phase 4 では
移行で新設する 5 箇所にも通知コンテキストを付ける。

#### 共通エンベロープが置き換える既存コード

- ホスト名: `internal/logging/slack_handler.go` に `common.GetHostname()` の直接呼び出しが
  5 箇所ある。Phase 1〜3 の削除後に残るのは `buildCommandGroupSummary` と
  `buildPreExecutionError` の 2 箇所で、Phase 5 でこれを継ぎ目となるパッケージ変数へ
  置き換える。`internal/runner/bootstrap/logger.go` の `common.GetHostname()` は Slack とは
  無関係であり、変更しない。継ぎ目の書き方は `internal/common/system.go` の `osHostname`
  にならう。
- フィールド見出し: `fieldTitleHostname`・`fieldTitleRunID` の定数が既にある。Scope の定数を
  同じ場所へ足し、この 3 個を予約見出しとして共通エンベロープとビルダー検査の双方から参照
  する。
- 色と絵文字: `colorGood`・`colorWarning`・`colorDanger`、`emojiSuccess`・`emojiWarning`・
  `emojiFailure` は既にあり、レベル対応表からそのまま使える。新規定義は不要である。
- 添付フィールド型 `SlackAttachmentField`（`Title`／`Value`／`Short`）は既存のものを使う。
- 出力の切り詰め: `slack_handler.go` の 621・635・799 行目付近に、`outputMaxLength`（1000）と
  `stderrMaxLength`（500）を使う同じ切り詰め処理が 3 箇所に重複している。3 個目は Phase 1 で消える。
  Phase 5 で `user_group_command_failure` のビルダーを足すとまた増えるため、切り詰めは
  ヘルパー 1 個へ括り出して両ビルダーが呼ぶ。**発火元
  `audit/logger.go` は `RedactText` を通すだけで切り詰めていない**（現在この種別は汎用
  メッセージへ落ちるため上限が効いていない）。新ビルダーで上限を適用する。

#### 設定検証と redaction

- `internal/runner/config/validation.go` の `ValidateGroupNames` は、group 名について空
  （`ErrEmptyGroupName`）と文字種（`GroupNamePattern` = `^[A-Za-z_][A-Za-z0-9_]*$`）と重複を
  検査する。command 名についての検査はどこにも無く、どちらの名前にも長さ上限が無い。
  関数名と doc コメントは group 名専用の書き方であり、呼び出し元は
  `internal/runner/config/loader.go:236` の 1 箇所だけである。
- `internal/redaction/redactor.go` の `Config.RedactLogAttribute` は、文字列値に対して
  `RedactText`（キー名由来の置換 → `ValueDetector.Mask`）を先に適用し、値が変化しなかった
  ときだけ `patterns.IsSensitiveValue` を見る。02_architecture.md §3.1 の記述と一致する。
  したがって述語は `RedactText` と `IsSensitiveValue` の両方を通す形にする。
- `internal/redaction/value_detector.go` の実際のパターンを確認した結果、02_architecture.md
  §7.1 が求めるテスト行の作り分けは次のとおり実在する。
  - group 名としても command 名としても書ける値形式: `awsKeyID`（`\bAKIA[0-9A-Z]{16}\b`）と
    `githubToken`（`\bgh[pors]_\s*[A-Za-z0-9_]{36,}\b`）。どちらも英数字と下線だけで綴れる
    ため `GroupNamePattern` を通る。
  - command 名でしか書けない値形式: `jwt`（`.` を含む）と Webhook ホストを含む URL 形
    （`:` と `/` を含む）。どちらも `GroupNamePattern` に反するため、group 名では
    `ErrInvalidGroupName` が先に返り redaction 用のセンチネルへ届かない。
- 識別子の redaction 検査が使う `*redaction.Config` の入手経路を HEAD で確認した。
  `internal/runner/bootstrap/config.go` の `LoadAndPrepareConfig` は `cfgLoader.LoadConfig`
  の後に `normalizeSlackAllowedHost` を呼び、`cfg.Global.SlackAllowedHost` へ書き戻す
  （`config.go:101-112`）。本番の `redaction.Config` を組み立てるのはその後に走る
  `AddSlackHandlers`（`internal/runner/bootstrap/logger.go:405`）であり、
  `SetupSlackLogging` がその値を戻り値として返している（`environment.go`。doc コメントが
  「callers can share the same webhook-host masking ... instead of each rebuilding it」と
  目的を明示している）。`cmd/runner/main.go` は `:320` で `LoadAndPrepareConfig`、`:326` で
  `SetupSlackLogging` を呼び、受け取った `redactionConfig` を `:386` の `executeRunner` を
  経て `runner.WithRedactionConfig`（`:547`）へ渡している。したがって 02_architecture.md
  §3.1 が定める検査の位置は `:326` の直後、`slog.Info("Verification and configuration
  completed")` の前であり、そこまでに識別子を載せた通知は 1 件も出ない。
- `SetupSlackLogging` は Slack 未設定（`slackConfig == nil`、または成功用・エラー用 URL が
  ともに空）で `nil` を返す。その場合も Phase 1 の `RedactingHandler` は動いており、
  `NewRedactingHandler` は `config == nil` のとき `DefaultConfig()` を使う
  （`internal/redaction/redactor.go:663-666`）。すなわち Slack 未設定でも識別子は既定の
  変換で `[REDACTED]` になりうるため、`nil` を「検査しない」と読んではならない。

#### リポジトリ同梱の TOML が新しい識別子検証に抵触する

`internal/redaction/sensitive_patterns.go` の既定パターンには、アンカーの無い
`(?i)(password|token|secret|key|api_key)` と `(?i)basic` が含まれる。実際に
`DefaultSensitivePatterns().IsSensitiveValue` を呼んで確かめたところ、同梱 TOML の次の識別子
はいずれも真を返す。すなわち Phase 4 の redaction 検査を入れると、**リポジトリ自身の設定で
実バイナリが起動できなくなる**。読み込み（`LoadConfig`）自体は通る。検査は
`internal/runner/bootstrap/identifier_redaction.go` にあり、呼ぶのは `cmd/runner/main.go`
だけだからである（§4.4）。

| ファイル | 識別子 |
|---|---|
| `sample/comprehensive.toml:38` | `basic_tests` |
| `sample/variable_expansion_test.toml:15,28` | `cmd_expansion_basic`、`args_expansion_basic` |
| `sample/output_capture_basic.toml:18` | `basic_output_examples` |
| `sample/auto_env_test.toml:15` | `basic_auto_env` |
| `sample/variable_expansion_security.toml:111` | `api_call_with_token` |

`sample/comprehensive.toml` は `integration-test`・`e2e-test`・`slack-e2e-test` が使い、かつ
ハッシュ記録の対象である。識別子を改名すると記録済みハッシュも取り直しになる。検査に実際に
掛かるのはこの 1 個だけである。実バイナリを起動する経路がここしか無いためで、他の 5 個を読む
`internal/runner/config` の後方互換テスト群（`loader_compatibility_test.go`、
`backward_compat_test.go`、`template_backward_compat_test.go`）は `LoadConfig` しか呼ばず、
redaction 検査を通らない。それでも 6 個すべてを改名する。`sample/` は利用者が実行できる例と
して置いてあり、起動できない例を残す意味が無いためである。**この後方互換テスト群の期待値は
改名しても更新が要らない。** 6 個の名前を `rg` で追うと現れるのは `sample/*.toml` と本書だけ
であり、同テスト群は識別子を名前ではなくファイル名と `cfg.Groups[0].Commands[0]` のような位置
で参照しているためである（実測結果は §4.4）。Phase 4 で改名と再記録を同じコミットで行い、
再発を防ぐメタテストを置く（§4.4）。

なお `backup` や `pg_dump` のような通常の名前は偽陽性にならないことも同じ確認で見ている。

#### 既存テストが空の `message_type` を多用している

`internal/logging/slack_sender_test.go` には `slackRecord(level, "", text)` の形で
**`message_type` が空のレコードを作る箇所が 29 個**ある。Phase 5 以降、空の `message_type` は
未知種別として扱われる。汎用メッセージは送られるが、通知コンテキストを持たないため
`Slack notification schema violation` の WARN が送信失敗ロガーへ 1 件増える。影響を受ける代表例は
`TestSlackSender_FlushLogsMessageTypeBreakdown` の集計キー、478 行目と 726 行目付近の
出現数の assert、および「失敗の記録自体が Slack 要求を生まないこと」を見る assert である。
Phase 5 で個別に気付く形にせず、共有ヘルパー `slackRecord` の側で一度に決める（§5.5）。

#### 構文木を走査する検査の再利用先

本タスクは、本番コードが `slack_notify`／`message_type` を直接組み立てないことと、
`PreExecutionError` のリテラルが `NotificationContext` を省略しないことを、構文木の静的検査で
確かめる（02_architecture.md §3.4）。同種の検査は既にあり、次を再利用する。

- `internal/testutil/identitymutationguard` の `ProductionGoFiles`（`_test.go` と `test` タグ
  付きファイルを除いた本番ファイルの列挙、`helpers.go:151`）と `ResolveLocalImports`
  （`helpers.go:302`）。
- `internal/testutil/synccensus/census_guard_test.go` にある、リポジトリ全体を歩く走査
  （`scanRoots` を `filepath.WalkDir` で辿り、ディレクトリごとに `ProductionGoFiles` を呼び、
  `testdata` を `fs.SkipDir` で除外する）。**現在この走査はそのテストファイルの中にあり
  再利用できない**。本タスクで 2 本目を書くと複製になるため、走査部分を
  `internal/testutil/identitymutationguard` へ関数として括り出し、`synccensus` と新しいガード
  の双方から呼ぶ。括り出しは、最初の利用者である構文木ガードと同じ Phase 4 で行う（§4.3）。
- これらのヘルパーは `//go:build test` を持つため、それを import するテストファイルにも同じ
  タグが要る（`cmd/runner/startup_order_guard_test.go` と同じ扱い）。

#### 削除・改修の対象になる既存テスト（実在を確認済み）

`TestSlackHandler_Handle_WithMockServer`（`internal/logging/slack_handler_test.go`）、
`TestSlackSender_HighPriorityBypassesFullNormalQueue`・`TestSlackSender_QueueOverflowDropsAndRecords`・
`TestSlackSender_FlushLogsMessageTypeBreakdown`（`internal/logging/slack_sender_test.go`）、
`TestHandlePreExecutionError_AllTypes`・`TestHandlePreExecutionError_SlackNotification`
（`internal/logging/pre_execution_error_test.go`）、`TestRuntimeCommand_Structure`・
`TestRuntimeCommand_HelperMethods`（`internal/runner/base/runnertypes/runtime_test.go`）、
`TestValidateGroupNames`（`internal/runner/config/validation_test.go`）、
`TestLogger_LogUserGroupExecution`（`internal/runner/base/audit/logger_test.go`）、
`TestSlackNotification`（`internal/runner/runner_test.go`）、
`TestReportStartupPrivilegeFailure_UsesValidRunID`（`cmd/runner/startup_privilege_test.go`）、
`TestIntegration_RunnerFlushesSlackOnNormalExit`（`cmd/runner/integration_slack_flush_test.go`）、
`TestE2E_SlackWebhookSeparation_MessageFormat`（`internal/runner/e2e_slack_webhook_separation_test.go`）、
`TestE2E_SlackWebhookWithMockServer`（`internal/runner/e2e_slack_webhook_test.go`）は、
いずれも記載どおりの場所に存在する。

ただし `TestSlackNotification` は名前に反して通知を検証していない。本体が assert するのは
`runner.runID` の値だけで、テーブルの `expectedStatus`・`expectedCalls` は宣言されたまま
未使用であり、レコードを捕捉するハンドラも差し込んでいない。さらに呼ぶのは `ExecuteGroup`
（`runner.go:493`）であって、検証エラーの分岐がある `executeGroups`（`runner.go:404`、
`Execute` 経由）ではない。したがって AC-13・AC-14 の材料には使えない。通知レコードを実際に
捕捉している既存テストは `TestLogGroupExecutionSummary_LogLevel`（同ファイル 2318 行目、
`tu.NewCallbackHandler` を `slog.SetDefault` へ差し込み `logGroupExecutionSummary` を直接
呼ぶ）であり、グループ集計側の拡張先はこちらである（§5.5）。

`internal/runner/base/privilege` の既存テスト `TestWithPrivileges_WritesNoRecordWhileElevated` は
native root の記録が存在することまでしか assert せず、記録の属性（operation・command・original_uid）を
固定するテストは無い。
AC-05 のテストは Phase 3 で新規に書く。ただし到達性に制約がある。`escalatePrivileges`
（`unix.go:299`）は、`originalUID == 0` のときだけ `elevationNativeRoot` を設定して早期
return し、それ以外では実際に `syscall.Seteuid(0)` を呼ぶ。同ファイル冒頭のコメントが
記すとおり CI と開発コンテナは非 root で走るため、`Seteuid(0)` は EPERM で失敗し
`execCtx.elevation` は `elevationNone` のままとなり、`logElevationOutcome` は何も記録しない。
すなわち **`seteuid` 分岐は `WithPrivileges` 経由では到達できない**。native root 分岐は
`originalUID: 0` の構造体リテラルで到達でき、これは同パッケージのテストで既に使われている
書き方である（`unix_privilege_test.go:126, 250, 590, 689`）。Phase 3 のタスクはこの制約に
合わせて書く。

#### 利用者向け・開発者向け文書の該当箇所

| 文書 | 該当行 | 扱い |
|---|---|---|
| `README.ja.md` 96 行目 | 「**Slack統合**: セキュリティイベントのリアルタイム通知」 | Phase 6 で実際に通知される内容へ改める |
| `README.ja.md` 57 行目 | 「**包括的監査証跡**: 特権操作とセキュリティイベントの完全なログ記録」 | Slack ではなく監査ログ全般の記述であり、削除後も事実として残る。Phase 6 で内容を確認し、変更しない場合はその判断をコミットメッセージに記す |
| `docs/user/security-risk-assessment.ja.md` 301 行目 | 高優先度キューを「セキュリティアラート等」と説明 | Phase 6 で `pre_execution_error` へ改める |
| `docs/user/security-risk-assessment.ja.md` 465 行目 | 「基本的なセキュリティイベント記録は実装済み」 | 監査ログの記述であり Slack 通知ではない。Phase 6 で確認のうえ判断を記す |
| `docs/dev/architecture_design/slack_async_delivery.ja.md` 35 行目 | `highPriority`(セキュリティアラート等) | Phase 6 で `pre_execution_error` へ改める |
| `docs/dev/architecture_design/security-architecture.ja.md` 898・1307・1310 行目 | セキュリティイベントの Slack 通知を提供すると記述 | Phase 6 で削除後の実態へ改める |
| `docs/dev/architecture_design/security-architecture.ja.md` 406・1143 行目 | 「セキュリティイベント記録」 | 監査ログの記述。Phase 6 で確認のうえ判断を記す |
| `docs/user/runner_command.ja.md` の `### 4.2 通知設定` | Slack 通知の設定を説明する節。通知の書式についての記載は無い | Phase 6 で通知種別・統一書式・Scope・製品名の節を追加する |

英語版（`README.md`、`docs/user/security-risk-assessment.md`、
`docs/dev/architecture_design/slack_async_delivery.md`、`security-architecture.md`、
`docs/user/runner_command.md` の `### 4.2 Notification Configuration`）は、いずれも日本語版と
対応する位置に同じ記述がある。日本語版を先にコミットし、英語版へは `/mktrans` で反映する。
日英を直接両方編集しない。

### 1.4 テストヘルパーの方針

新しいテストヘルパー**ファイル**は作らない。既存ファイルへの追記だけで足りる。

- 通知コンテキストの構造的なテストは `internal/common` 内の同一パッケージテストで書ける。
  ただし `RedactingHandler` を挟んだ経路の検証（§4.1）は `internal/redaction` と
  `internal/logging` を要するため、`internal/common` には置けない。`internal/logging` 側の
  テストとして書く。
- ホスト名の継ぎ目は `internal/logging` の非公開パッケージ変数であり、同一パッケージの
  テストから差し替えられる。
- 構文木の走査は `internal/testutil/identitymutationguard`（既存、`//go:build test`）へ
  リポジトリ全体を歩く関数を 1 個追加して使う。新規パッケージは作らない。
- `internal/logging/test_helpers.go`（`//go:build test`）と
  `internal/runner/base/audit/test_helpers.go` は既に存在する。新しいヘルパーが要る場合は
  これらへ追記し、新規ファイルを増やさない。

## 2. 実装ステップ

Phase の並びと内容は 02_architecture.md §8.1 に従う。

### Phase 1: `privileged_command_failure` の削除

**対象ファイル**: `internal/common/logschema.go`、`internal/logging/slack_sender.go`、
`internal/logging/slack_handler.go`、`internal/logging/slack_handler_test.go`

- [x] `internal/common/logschema.go` から `PrivilegedCommandFailureAttrs` の定義と、その直前の
      「Write side not yet implemented」と記すコメントを削除する。
- [x] `internal/logging/slack_sender.go` から定数 `messageTypePrivilegedCommandFailure` を
      削除する。
- [x] `internal/logging/slack_handler.go` の `Handle` から
      `case messageTypePrivilegedCommandFailure:` の分岐を削除する。
- [x] `internal/logging/slack_handler.go` から `buildPrivilegedCommandFailure` を削除する。
- [x] `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` から
      テーブルケース「privileged command failure」を削除する。
- [x] 削除の直前と直後に
      `go test -tags test -coverprofile=<file> ./internal/logging/... ./internal/common/...`
      を実行し、`go tool cover -func=<file>` を存続する関数ごとに比較する。差が無いことを
      コミットメッセージへ記す。
- [x] 削除したテストが参照していたヘルパーや型がファイル内で未参照になっていないことを確認
      する。未参照になったものは同じコミットで削除する。

**完了条件**:
- `rg -n -e privileged_command_failure -e PrivilegedCommandFailureAttrs -e buildPrivilegedCommandFailure -e messageTypePrivilegedCommandFailure --type go cmd internal` が一致なし（終了コード 1）。
- `make test` と `make lint` が通り、この Phase だけで 1 コミットになっている。

### PR-1 作成ポイント: remove privileged_command_failure notification type

**対象ステップ**: Phase 1

**推奨タイトル**: `refactor(0172): remove privileged_command_failure notification type`

**レビュー観点**: 削除漏れが無いこと（AC-01 の `rg` 検索が一致なしであること）／削除前後のカバレッジ比較（`go tool cover -func`）が存続関数について差が無いこと／テストテーブルケース削除後に未参照になったヘルパーが残っていないこと

**実装モデル要件**: standard

**判定理由**: 単純な削除作業で、設計判断は 02_architecture.md に既決。未確定の実装アプローチや高リスク分岐は無く、トリガーは一致しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: `security_alert` の削除

**対象ファイル**: `internal/common/logschema.go`、`internal/logging/slack_sender.go`、
`internal/logging/slack_handler.go`、`internal/runner/base/audit/logger.go`、
`internal/logging/slack_handler_test.go`、`internal/logging/slack_sender_test.go`、
`internal/runner/base/audit/logger_test.go`

- [x] `internal/common/logschema.go` から `SecurityAlertAttrs` を削除する。
- [x] `internal/common/logschema.go` から `SeverityCritical` と `SeverityHigh` を、その
      `SecuritySeverity` の見出しコメントごと削除する。
- [x] `internal/runner/base/audit/logger.go` から `LogSecurityEvent` を削除する。削除後に
      未使用になる import があれば取り除く。
- [x] `internal/logging/slack_sender.go` から定数 `messageTypeSecurityAlert` を削除する。
- [x] `internal/logging/slack_sender.go` の `isHighPriority` の `case` から
      `messageTypeSecurityAlert` を外し、関数の doc コメントから "Security alerts" の記述を
      削除する。
- [x] `internal/logging/slack_handler.go` の `Handle` から `case messageTypeSecurityAlert:` の
      分岐を削除する。
- [x] `internal/logging/slack_handler.go` から `buildSecurityAlert` を削除する。
- [x] `internal/logging/slack_sender_test.go` のヘルパー `securityAlertRecord` を、存続する
      高優先度種別で書いた `preExecutionErrorRecord` へ置き換える。レコードはレベル ERROR、
      `message_type` は `messageTypePreExecutionError` とし、**引数の文字列は
      `common.PreExecErrorAttrs.ErrorType` 属性へ載せる**。`buildPreExecutionError` は Text 行を
      この属性から作り、レコード本文（`r.Message`）は読まないためである。Phase 5 以降も
      要約は `error_type` のままなので、この形は両フェーズで有効である。
- [x] `securityAlertRecord` の 3 箇所の利用（`TestSlackSender_HighPriorityBypassesFullNormalQueue`、
      `TestSlackSender_QueueOverflowDropsAndRecords` のテーブル行「high priority queue」、
      `TestSlackSender_FlushLogsMessageTypeBreakdown`）を新しいヘルパーへ差し替える。
      配送順を見分ける `assert.Contains` の期待文字列も、新しいヘルパーが Text 行へ出す値に
      合わせて更新する。`TestSlackSender_FlushLogsMessageTypeBreakdown` の期待マップのキーも
      `messageTypeSecurityAlert` から `messageTypePreExecutionError` へ変える。
- [x] `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` から
      テーブルケース「security alert」を削除する。
- [x] `internal/runner/base/audit/logger_test.go` から `TestLogger_LogSecurityEvent` を削除する。
- [x] 同ファイルから `TestLogSecurityEvent_Masking` を削除する。
- [x] 同ファイルから `TestLogSecurityEvent_DetailsRedaction` を削除する。
- [x] 同ファイルから `TestLogSecurityEvent_DetailsKeyCollisionPrevention` を削除する。
- [x] 同ファイルの補助型 `sensitiveLogValuer`（25 行目付近）とその doc コメント、`LogValue`
      メソッドを削除する。唯一の参照が `TestLogSecurityEvent_DetailsRedaction` の中にあり、
      テストだけを消すと未使用型として `make lint` が落ちるためである。
- [x] 削除後、同ファイル内の他のヘルパー（`NewAuditLoggerWithCustomRedaction` など）が未参照に
      なっていないことを確認する。
- [x] 削除の直前と直後に
      `go test -tags test -coverprofile=<file> ./internal/logging/... ./internal/runner/base/audit/... ./internal/common/...`
      を実行し、`go tool cover -func=<file>` を比較して結果をコミットメッセージへ記す。

**完了条件**:
- `rg -n -e security_alert -e SecurityAlertAttrs -e buildSecurityAlert -e LogSecurityEvent -e messageTypeSecurityAlert --type go cmd internal` が一致なし。
  **`sensitiveLogValuer` はこの検索に入れない。** 同名の別物が
  `internal/redaction/redactor_test.go` にあり（本タスクは触らない）、`audit` 側を正しく
  削除しても一致が残るため、入れるとゲートが永久に赤になる。削除の確認は
  `rg -n sensitiveLogValuer internal/runner/base/audit/` が一致なし、で行う。§7 の AC-02 の
  行もこの形である。
- `rg -n -e common.SeverityCritical -e common.SeverityHigh --type go cmd internal` が一致なし、
  かつ `rg -n -e SeverityCritical -e SeverityHigh internal/common/` が一致なし
  （`runerrors` の同名定数は対象外なので、この 2 つの検索で切り分ける）。
- `TestSlackSender_HighPriorityBypassesFullNormalQueue` が `pre_execution_error` で緑になり、
  `isHighPriority` を常に `false` へ倒すと失敗することを確認済みである。
- `make test` と `make lint` が通り、この Phase だけで 1 コミットになっている。

### PR-2 作成ポイント: remove security_alert notification type

**対象ステップ**: Phase 2

**推奨タイトル**: `refactor(0172): remove security_alert notification type`

**レビュー観点**: `securityAlertRecord` から `preExecutionErrorRecord` への置き換えが Text 行の期待値・集計キーの双方で一貫していること／`sensitiveLogValuer` が未参照化されており `audit` パッケージ内の検索でのみ確認していること（`redaction_test.go` の同名別物を巻き込まないこと）／`TestSlackSender_HighPriorityBypassesFullNormalQueue` が `pre_execution_error` 基準で緑になり、優先度判定を倒すと落ちること

**実装モデル要件**: standard

**判定理由**: 削除と既存ヘルパーの置換が中心で、設計判断は 02_architecture.md に既決。複数の実装アプローチの検討は無く、トリガーは一致しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: `privilege_escalation_failure` の削除と特権監査テストの追加

**対象ファイル**: `internal/common/logschema.go`、`internal/logging/slack_sender.go`、
`internal/logging/slack_handler.go`、`internal/runner/base/audit/logger.go`、
`internal/logging/slack_handler_test.go`、`internal/runner/base/audit/logger_test.go`、
`internal/runner/base/privilege/unix_privilege_test.go`

- [x] `internal/common/logschema.go` から `PrivilegeEscalationFailureAttrs` を削除する。
- [x] `internal/runner/base/audit/logger.go` から `LogPrivilegeEscalation` を削除する。削除後に
      未使用になる import があれば取り除く。
- [x] `internal/logging/slack_sender.go` から定数 `messageTypePrivilegeEscalationFail` を
      削除する。
- [x] `internal/logging/slack_sender.go` の `isHighPriority` の `case` から
      `messageTypePrivilegeEscalationFail` を外し、doc コメントの記述も合わせる。この時点で
      高優先度は `messageTypePreExecutionError` だけになる。
- [x] `internal/logging/slack_handler.go` の `Handle` から
      `case messageTypePrivilegeEscalationFail:` の分岐を削除する。
- [x] `internal/logging/slack_handler.go` から `buildPrivilegeEscalationFailure` を削除する。
- [x] `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` から
      テーブルケース「privilege escalation failure」を削除する。
- [x] `internal/runner/base/audit/logger_test.go` から `TestLogger_LogPrivilegeEscalation` を
      削除する。
- [x] 同ファイルから `TestLogPrivilegeEscalation_Masking` を削除する。
- [x] `internal/runner/base/privilege/unix_privilege_test.go` へ、native root の昇格結果が
      記録され続けることを `WithPrivileges` 経由で検証するテストを追加する。マネージャは
      `originalUID: 0` の構造体リテラルで組む（同ファイル 126・250・590・689 行目と同じ
      書き方）。`unix.go:129` の `defer m.logElevationOutcome(execCtx)` を取り除くと失敗する
      形にし、`logElevationOutcome` の本体だけを見るテストにしない。
- [x] 同ファイルへ、`seteuid` 経路の昇格結果が記録されることを、`execCtx.elevation` を
      `elevationSeteuid` に設定して `logElevationOutcome` の境界で検証するテストを追加する。
      `WithPrivileges` 経由にしないのは、非 root では `syscall.Seteuid(0)` が EPERM で失敗し
      `elevation` が `elevationNone` のままとなって何も記録されず、この分岐へ到達できない
      ためである（§1.3）。到達できない経路を緑に見せないよう、この制約をテストの doc コメント
      へ英語で記す。
- [x] 追加する 2 つのテストは `t.Parallel()` を呼ばない。同ファイルはプロセス全体の識別情報を
      共有するためである。
- [x] 削除の直前と直後で `go tool cover -func` を比較し、結果をコミットメッセージへ記す。
- [x] `make deadcode` を実行し、新たな到達不能コードが報告されないことを確認する。

**完了条件**:
- `rg -n -e privilege_escalation_failure -e PrivilegeEscalationFailureAttrs -e buildPrivilegeEscalationFailure -e LogPrivilegeEscalation -e messageTypePrivilegeEscalationFail --type go cmd internal` が一致なし。
- 追加した native root のテストが緑で、`defer m.logElevationOutcome(execCtx)` を外すと失敗
  することを確認済みである。
- `make test`、`make lint`、`make deadcode` が通り、この Phase だけで 1 コミットになっている。

### PR-3 作成ポイント: remove privilege_escalation_failure and add elevation-outcome tests

**対象ステップ**: Phase 3

**推奨タイトル**: `feat(0172): remove privilege_escalation_failure and add elevation-outcome tests`

**レビュー観点**: native root 側テストが `WithPrivileges` 経由で `defer m.logElevationOutcome(execCtx)` を外すと落ちること／`seteuid` 側テストが到達不能である理由をテストの doc コメントに残し、`logElevationOutcome` の境界だけを検証していること／2 テストが `t.Parallel()` を呼ばない理由（プロセス全体の識別情報の共有）が妥当であること

**実装モデル要件**: frontier-recommended

**判定理由**: 特権昇格結果ログの孤立した高リスクステップ。CI・開発コンテナが非 root で走るため `seteuid` 分岐が `WithPrivileges` 経由では到達できないという制約に対して、到達可能な境界を選び直す設計判断を要する（§1.3 の到達性分析）。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: 通知コンテキストの追加と伝搬、識別子の設定検証

**対象ファイル**: `internal/common/notification_context.go`（新規）、
`internal/common/interpolation.go`（新規）、
`internal/common/notification_context_test.go`（新規）、
`internal/common/interpolation_test.go`（新規）、`internal/common/logschema.go`、
`internal/common/errors.go`、
`internal/runner/base/runnertypes/runtime.go`、`internal/runner/base/runnertypes/runtime_test.go`、
`internal/logging/pre_execution_error.go`、`internal/logging/pre_execution_error_test.go`、
`cmd/runner/main.go`、`internal/runner/bootstrap/config.go`、
`internal/runner/bootstrap/config_test.go`、`internal/runner/bootstrap/environment.go`、
`internal/runner/bootstrap/identifier_redaction.go`（新規）、
`internal/runner/bootstrap/identifier_redaction_test.go`（新規）、
`internal/runner/runner.go`、`internal/runner/base/audit/logger.go`、
`internal/runner/config/validation.go`、`internal/runner/config/errors.go`、
`internal/runner/config/validation_test.go`、`internal/runner/config/loader.go`、
`internal/runner/cli/filter.go`、`internal/redaction/redactor.go`、
`internal/redaction/redactor_test.go`、`cmd/runner/startup_privilege_test.go`、
`internal/logging/notification_context_test.go`（新規）、
`internal/logging/notification_contract_guard_test.go`（新規）、
`internal/testutil/identitymutationguard/helpers.go`、
`internal/testutil/synccensus/census_guard_test.go`、`internal/runner/runner_test.go`、
`internal/runner/base/audit/logger_test.go`、
`cmd/runner/integration_pre_execution_error_test.go`、
`cmd/runner/startup_order_guard_test.go`、`sample/*.toml`

この一覧は本 Phase 全体（PR-4／PR-5／PR-6 の 3 PR の和）が触るファイルの範囲であり、
下のタスクが触るファイルはすべてここに現れる。PR 境界は §4.0〜§4.2（PR-4）・§4.3（PR-5）・
§4.4（PR-6）で切られており、各 PR が実際に変更するのはこの一覧の部分集合である。とくに
`cmd/runner` の 2 本は、起動経路で検査が呼ばれていることと、渡している `*redaction.Config` が
`SetupSlackLogging` の戻り値であることを見る唯一の検証であり（§4.4、PR-6 に属する）、一覧から
漏らすと AC-32 の確認がこの一覧の外へ散る。

#### 4.0 表示安全な補間契約

- [x] `internal/common/interpolation.go` を新規作成し、02_architecture.md §3.5 の表示安全な
      補間契約を実装する。役割（識別子／エンベロープ値／自由文／大量出力）を enum で受け取り、
      切り詰めの有無を `switch` で決める。ゼロ値と `default` は自由文と同じ最も強い加工へ
      倒す。置き換えの順序・対象文字・上限は 02_architecture.md §3.5 の「変換規則」に
      そのまま従う。
- [x] 同ファイルへ、補間契約を通した結果に Unicode の White_Space 以外の rune が 1 個以上
      残るかを返す述語を置く。設定検証と通知コンテキストの妥当性判定が共有する。
- [x] `internal/common/interpolation_test.go` を新規作成し、02_architecture.md §7.1 の
      「表示安全な補間契約」と「自由文の切り詰め」の観点を表駆動で検証する。出力側は §3.5 の
      「出力の性質」5 項目を共通の検査として当てる。置き換え集合から 1 文字を外すと、その
      文字の行が対応する性質の検査で失敗する形にする。裸の URL の行は綴りが変わらないことを
      期待値とする。同じ入力を識別子の役割で通すと切り詰められないことも確認する。

補間契約を `internal/common` に置く理由は §1.3「表示安全な補間契約の配置」に記した。
`internal/logging` に置くと `internal/common` から呼べず import 循環になる。

#### 4.1 通知コンテキストの型

- [x] `internal/common/notification_context.go` を新規作成し、02_architecture.md §3.1 の
      `NotificationScope`、`NotificationContext`、コンストラクタ 3 個、参照メソッド 3 個、
      `LogValue`、`LogAttr` を定義する。フィールドはすべて非公開にする。
- [x] 同ファイルへ識別子 1 個あたりの長さ上限を定数として置く。値は 128 byte とし、
      設定検証だけが参照する（02_architecture.md §3.1）。
- [x] 同ファイルへ、レコード上のエンコードから通知コンテキストを復元し妥当性を判定する関数を
      置く。判定は 02_architecture.md §3.1 の表と §3.6 の理由コード分類に従う。下位キーは
      `scope`・`group`・`command` のちょうど 3 種（`command` だけ条件付き出力）に限り、
      未知キー・重複キー・非文字列値・スコープと名前の組の矛盾をすべて不正として返す。
      不正は `internal/common/errors.go` の `ErrInvalidNotificationContext` として返し、
      SlackHandler 側で §3.6 の理由コード `invalid_notification_context` に対応付ける。
      「表示できる文字が残らない名前」の判定は §4.0 の述語を呼ぶ。
- [x] `internal/common/logschema.go` へ、通知コンテキストの属性キー名とスコープ名
      （`global`／`group`／`command`）の対応を加える。
- [x] `internal/common/logschema.go` へ `UserGroupCommandFailureAttrs` を追加する。項目は
      `command_name`（string）、`exit_code`（int）、`stdout`（string）、`stderr`（string）で、
      記録側と参照側が共有する（02_architecture.md §3.4）。
- [x] `internal/common/notification_context_test.go` を新規作成し、ゼロ値と `GlobalScope()` が
      同じエンコードになること、各スコープの往復、`command` の条件付き出力、および復元と
      妥当性判定の全行（02_architecture.md §7.1 の「妥当性判定の全行」「下位キーの重複」の
      観点）を検証する。`GroupScope("")` については、`scope=group`・`group=""` として
      エンコードされ、判定が `invalid_notification_context` を返すことを assert する。
- [x] `internal/logging/notification_context_test.go` へ、通知コンテキストを持つレコードを
      `RedactingHandler` へ流し、**下位ハンドラを捕捉用ハンドラ**（`tu.NewCallbackHandler`）
      とし、そこで受け取った属性を §4.1 の復元関数へ直接渡して、直接エンコードした場合と
      同じ妥当性判定になることを検証するケースを追加する
      （02_architecture.md §7.1「エンコードの往復」）。設計は
      `RedactingHandler.processLogValuer` が `LogValue()` を解決してから下位ハンドラへ渡す
      ことを前提にしているため、この経路を確かめないと前提が崩れても気付けない。
      **下位ハンドラを `SlackHandler` にはしない。** 本 Phase の `SlackHandler.Handle` は
      通知コンテキスト属性を読みも検証もせず（その実装は §5.3）、本 Phase の対象ファイルにも
      `slack_handler.go` は入っていない。`SlackHandler` へ流す形で書くと、妥当・不正・欠落の
      どれもが同じく無視されるため、テストが宣言した理由では失敗しえない。捕捉用ハンドラと
      復元関数の直接呼び出しにすれば、検証対象（`LogValue()` の解決が redaction を跨いで
      保たれること）だけが結果を決める。Phase 5 で `Handle` が実際に判定を行うようになった
      後の経路全体の検証は §5.5 の不正な通知コンテキストのケースが受け持つ。

#### 4.2 `RuntimeCommand` のグループ名

- [x] `internal/runner/base/runnertypes/runtime.go` へ、`TimeoutResolution.GroupName` を返す
      参照メソッド `GroupName()` を追加する。新しいフィールドは足さない。
- [x] `internal/runner/base/runnertypes/runtime_test.go` へ、`NewRuntimeCommand` に渡した
      group 名を `GroupName()` が返すことを検証するテストを追加する。構造体リテラルで
      `RuntimeCommand` を組む既存ケースでは `TimeoutResolution` を明示する。

### PR-4 作成ポイント: display-safe interpolation contract and notification context type

**対象ステップ**: Phase 4 §4.0 / §4.1 / §4.2

**推奨タイトル**: `feat(0172): add interpolation contract and notification context type`

**レビュー観点**: 補間契約の変換規則・役割ごとの切り詰め有無が 02_architecture.md §3.5 と一致すること／`NotificationContext` のフィールドが非公開でコンストラクタ 3 個経由でのみ構築できる設計であること／エンコード往復・妥当性判定の全行・下位キー重複のテスト網羅性（02_architecture.md §7.1）／RuntimeCommand.GroupName が正しくグループ名を返し、既存のテストケースで TimeoutResolution が明示されていること

**実装モデル要件**: frontier-recommended

**判定理由**: 表示安全な補間契約はセキュリティ上重要な新規設計で、識別子／エンベロープ値／自由文／大量出力という複数の役割にまたがる切り分けを要する孤立した高リスク・複雑ステップである。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### 4.3 `PreExecutionError` の構造体化と発火元への伝搬

- [ ] `internal/logging/pre_execution_error.go` の `PreExecutionError` へ
      `NotificationContext common.NotificationContext` を追加する。
- [ ] `HandlePreExecutionError` のシグネチャを
      `func HandlePreExecutionError(preExecErr *PreExecutionError)` へ変える。本文は
      `preExecErr.Detail()`、Run ID は `preExecErr.RunID`、種別とコンポーネントは同名
      フィールドを使う（02_architecture.md §3.2 の表）。
- [ ] `handleErrorCommon` を、`slack_notify` と `message_type` を自前で組まず、呼び出し元が
      渡した `[]slog.Attr` をそのまま記録する形へ変える。`errorHandlingParams` から
      `slackNotify` と `slogMsgType` を取り除き、属性スライスの項目を加える。記録は現在
      `slog.Error(msg, ...any)` なので `slog.LogAttrs` へ切り替える。
- [ ] **Phase 4 の時点では**、`HandlePreExecutionError` が渡す属性スライスを、現在と同じ
      `slack_notify=true` と `message_type="pre_execution_error"` に通知コンテキスト属性を
      足したものとして、`pre_execution_error.go` の中で組み立てる。`NotificationAttrs` へ
      差し替えるのは Phase 5 である。この中間状態にするのは、各コミットで `make test` が
      通る状態を保つためである（AC-30）。
- [ ] `HandleExecutionError` は Slack へ送らない自分の属性（`slack_notify=false` と
      `message_type="execution_error"`）を渡す形へ合わせる。Slack 通知を行わないという既存
      契約は変えない。
- [ ] `cmd/runner/main.go` の既存 `PreExecutionError` リテラル 11 箇所すべてへ
      `NotificationContext: common.GlobalScope()` を加える。
- [ ] `cmd/runner/main.go` の `HandlePreExecutionError` 呼び出し 5 箇所（132、177、186、217、
      221 行目）を、構造体を渡す形へ移す。このうち 132・177・186・221 行目は現在リテラルを
      作っていないため、**新設するリテラルにも `NotificationContext: common.GlobalScope()` を
      付ける**。報告境界で `preExecErr.RunID` へプロセス唯一の Run ID を代入してから呼ぶ
      （02_architecture.md §3.2）。
- [ ] `internal/runner/bootstrap/config.go` の `PreExecutionError` リテラル 4 箇所へ
      `NotificationContext: common.GlobalScope()` を加える。
- [ ] `internal/runner/bootstrap/environment.go` の `PreExecutionError` リテラル 2 箇所へ
      `NotificationContext: common.GlobalScope()` を加える。
- [ ] `internal/runner/runner.go:429` の検証エラー経路を、
      `NotificationContext: common.GroupScope(verErr.Group)` を持つ**新設リテラル**を渡す形へ
      移す。本文からの `Group: <name>, ` 除去は Phase 5 で行う（02_architecture.md §8.2）。
- [ ] `internal/runner/runner.go` の `logGroupExecutionSummary` へ
      `common.GroupScope(groupSpec.Name)` の通知コンテキスト属性を加える。`...any` の可変長
      引数から `LogAttrs` を使う形へ変える。既存のトップレベル `group` 属性は残す。
- [ ] `internal/runner/base/audit/logger.go` の `LogUserGroupExecution` の失敗経路へ
      `common.CommandScope(cmd.GroupName(), cmd.Name())` の通知コンテキスト属性を加える。
- [ ] 同関数で、`common.UserGroupCommandFailureAttrs` の **4 項目すべて**（`command_name`、
      `exit_code`、`stdout`、`stderr`）を構造体から引く形へ変える。`command_name` と
      `exit_code` は成功経路と共有する `baseAttrs` にあるため、成功経路も同じ定数を使う。
      片側だけリテラルを残すと、キー名を変えたときに読み側がコマンド名を取り落とす場合がある。
      しかし記録側のコードは変更なしにテストを通ってしまうため、AC-23 がサイレントに壊れる。
- [ ] `internal/runner/runner_test.go` の `TestLogGroupExecutionSummary_LogLevel`
      （`tu.NewCallbackHandler` で `logGroupExecutionSummary` の出力を集める既存テスト）を
      拡張し、グループ集計のレコードにグループスコープの通知コンテキスト属性が載ることを
      検証する。`TestSlackNotification` は拡張先にしない（理由は §1.3）。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogger_LogUserGroupExecution` を
      拡張し、失敗経路のレコードがコマンドスコープの通知コンテキストを持つことを検証する。
直上 2 件のテスト拡張を Phase 5 ではなく Phase 4 に置くのは、属性を載せるのが本節の
`logGroupExecutionSummary` と `LogUserGroupExecution` のタスクだからである。本 Phase の構文木
ガードは `PreExecutionError` リテラルしか見ないため、この 2 発火元から通知コンテキスト属性を
落としても Phase 4 は緑のままになる。AC-11 は「存続する 3 種別の全発火点が通知コンテキストを
持つ」であり、3 発火元のうち 2 つの検証を Phase 5 へ送ると、Phase 4 の完了条件が実際には
確かめていないものを緑と称することになる。

- [ ] **リポジトリ全体を歩く走査を `internal/testutil/identitymutationguard/helpers.go` へ
      括り出す。** `internal` と `cmd` の本番 Go ファイルをリポジトリ全体から列挙する関数を
      追加する。中身は `internal/testutil/synccensus/census_guard_test.go` にある走査
      （`filepath.WalkDir`、`testdata` の `fs.SkipDir`、ディレクトリごとの
      `ProductionGoFiles`）を移したものとし、`synccensus` 側はその関数を呼ぶ形へ書き換える。
      走査の定義を 2 本にしないためである。**走査の起点は移植元をそのまま持ち込まない。**
      `census_guard_test.go` の `scanRoots = []string{"../../../internal", "../../../cmd"}` と
      `repoRootPrefix = "../../../"` は、その 1 ファイルの位置（`internal/testutil/synccensus`、
      リポジトリ root から 3 階層）に固定された相対パスである。新しい呼び出し元は
      `internal/logging`（2 階層）にあり、同じ literal では存在しないディレクトリを歩いて
      `WalkDir` がエラーで落ちる。関数は呼び出し元の深さに依らず root を自分で解決し
      （`go.mod` を上へ辿るなどして）、返すパスを root からの相対に正規化する形にする。
      `synccensus` の期待表は root 相対のパスで書かれているため、正規化の結果が
      `repoRootPrefix` を剥がした現在の表記と一致することを、書き換え後に既存の census
      テストが緑であることで確認する。**この括り出しを Phase 5 ではなく Phase 4 に置くのは、
      直下の構文木ガードが本 Phase で入り、その走査がこの関数を呼ぶためである。** Phase 5 に
      残すと Phase 4 のガードが存在しない API を参照し、Phase 4 の `make test`／AC-30 が
      通らない（複製して回避することは §1.2 の 5 が禁じている）。
- [ ] `internal/logging/notification_contract_guard_test.go` を新規作成し（`//go:build test`）、
      本番コードの `PreExecutionError` 複合リテラルが `NotificationContext` を省略していない
      ことを検証する。走査は直上で括り出したヘルパーを使う。この半分を Phase 4 に置くのは、
      検査対象が Phase 4 の構造体変更だけに依存し、Phase 4 の完了条件（AC-11）を Phase 5 の
      成果物に依存させないためである。残る半分（`slack_notify`／`message_type` の直接構築の
      禁止と `NotificationAttrs` の引数制限）は Phase 5 で足す。
- [ ] 同ファイルへ、**AC-09 のコンストラクタ検査**も本 Phase で入れる。`internal/common` を
      import する本番ファイルごとに `ResolveLocalImports` で実際の import 名を解決し、
      解決結果が `internal/common` を指す `NotificationContext` について、**コンストラクタ
      呼び出し以外でゼロ値を生じさせる構文をすべて拒否する**。既知の形は
      **(i) 複合リテラル・(ii) 初期化式の無い `var` 宣言・(iii) `new` 呼び出し・(iv) その型の
      名前付き戻り値**（`func global() (ctx common.NotificationContext) { return }` は
      (i)〜(iii) のどれでもないまま暗黙にゼロ値を返す）である。**この 4 つは網羅ではなく
      既知の例として書いてある。** 検査は「ゼロ値の生成」という性質に対して書き、形を 1 つずつ
      足していく作りにしない。既存の値を読むだけの経路（構造体フィールド、引数、
      コンストラクタの戻り値を受けた変数）は対象にしない。禁じるのは**生成**である。
      **`internal/common` 自身の本番ファイルも走査対象に含める。** パッケージは自分自身を
      import しないため、import の有無で対象を決めると `common` の中だけが素通りし、そこへ
      足したヘルパーがコンストラクタを迂回できてしまう。同パッケージ内では修飾子が付かない
      ため、構文木の照合は修飾子つきと修飾子なしの双方を見る。除外するのは
      `notification_context.go` の 3 コンストラクタ（`GlobalScope`・`GroupScope`・
      `CommandScope`）の本体だけであり、これらは非公開フィールドを初期化する正規の実装で
      ある。除外はファイル単位ではなく関数単位にする。ファイルごと除外すると、同じファイルへ
      足した別の関数が迂回できる。上の各形はいずれも
      コンストラクタを迂回してゼロ値を作る経路であり、AC-09 は「値はコンストラクタでのみ
      構築する」だけでなく「グローバルな場合も `GlobalScope()` で明示的に」付与することを
      要求している（§7 の AC-09 の行）。検査対象は Phase 4 の構造体変更だけに依存するため、
      Phase 5 へ送らない。**(i)〜(iv) それぞれについて、本番ファイルへ 1 個足すと落ちることを
      確認する**（1 形でも見落とす実装が緑のまま残らないようにする）。別名 import
      （`import c ".../internal/common"`）の行も含める。
- [ ] **この検査は完全にはできない。その前提で書く。** AC-09 はゼロ値を `ScopeGlobal` と
      定めており（`TestNotificationContext_ZeroValueIsGlobalScope` が固定している）、Go で
      ゼロ値を得る書き方は構文として列挙しきれない。したがってこの検査が防ぐのは、
      **事故でコンストラクタを迂回すること**であって、意図した迂回のすべてではない。
      発火元が実際に正しいスコープを載せていることは、§5.5 の 3 発火元のスコープ assert が
      実行時に確かめる。構文木ガードと実行テストのどちらか一方に寄せず、両方を持つ理由が
      ここにある。AC-27 の (d) に同じ断りを書いてあるのと同じ立場である。
- [ ] `cmd/runner/startup_privilege_test.go` の `TestReportStartupPrivilegeFailure_UsesValidRunID`
      を、新しい引数形（構造体）に合わせて更新する。
- [ ] `internal/logging/pre_execution_error_test.go` の `TestHandlePreExecutionError_AllTypes`
      と `TestHandlePreExecutionError_SlackNotification` を、位置引数から構造体を渡す形へ
      移す。あわせて、グローバルとグループの通知コンテキストがレコードへ載ることと、既存の
      stderr／stdout 出力の形が変わらないことを検証する。

### PR-5 作成ポイント: propagate NotificationContext through PreExecutionError and firing points

**対象ステップ**: Phase 4 §4.3

**推奨タイトル**: `feat(0172): propagate NotificationContext to firing points`

**レビュー観点**: 存続する 3 発火元（`logGroupExecutionSummary`、`HandlePreExecutionError` の全呼び出し元、`LogUserGroupExecution`）がすべて正しいスコープを載せること／AC-09 のコンストラクタ迂回検査（複合リテラル・`var` 宣言・`new` 呼び出し・名前付き戻り値の 4 形）が本番ファイルへ 1 個ずつ足すと個別に落ちること／`synccensus` の走査を `identitymutationguard` へ括り出した後もリポジトリ root の解決とパス正規化が既存テストと整合すること

**実装モデル要件**: frontier-recommended

**判定理由**: `PreExecutionError` の構造体化と 3 発火元への通知コンテキスト伝搬という、状態の整合性を発火元ごとに保証しなければならない孤立した高リスク・複雑ステップである。とくに AC-09 のコンストラクタ迂回検査は「ゼロ値の生成」という性質に対して書く AST 検査であり、既知の 4 形は例示であって網羅ではないという設計上の割り切りを要する（§4.3 内の検査設計）。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### 4.4 識別子の設定検証

- [ ] `internal/runner/config/errors.go` へ、`ErrEmptyGroupName` に並べて 5 個のセンチネル
      エラーを追加する。検査ごとに独立させ、対応は次のとおりとする。
      (a) 空のコマンド名、(b) 制御文字または書式制御文字を含む識別子、(c) 補間契約を通すと
      表示できる文字が残らない識別子、(d) 長さ上限を超える識別子、(e) redaction の変換が
      書き換える識別子。(b) と (c) は独立した検査であり、02_architecture.md §3.1 の表も
      5 行に分けてある。名前は素直に付けてよい。`internal/runner/resource/manager.go:24` に
      `ErrEmptyCommandName` があるが、パッケージが違えば同名でも共存する。現に
      `internal/runner/config/errors.go` の `ErrEmptyGroupName` と
      `internal/runner/resource/manager.go` の同名センチネルは今も共存している。名前を避ける
      規則は分かりにくい名前を強いるだけで、コンパイル上も可読性上も得るものが無い。
- [ ] `internal/redaction/redactor.go` へ、`RedactLogAttribute` が文字列値に施す変換
      （`RedactText` と `IsSensitiveValue` の両方）が値を書き換えるかを返す述語を公開する。
      既存の変換を読み取るだけで、redaction の適用範囲は変えない。
- [ ] `internal/redaction/redactor_test.go` へ、この述語が `IsSensitiveValue` の語一致だけで
      なく `ValueDetector` による値形式の検出（`AKIA` 形、`ghp_` 形、JWT 形、設定した
      Webhook ホストを含む URL 形）でも真を返すことを検証するケースを追加する。**各値形式の
      行では、まず `IsSensitiveValue` 単独ではその値が素通りすることを assert する**。これが
      無いと、述語が `ValueDetector` まで含んだ経路を通っているのか語一致だけなのかを行が
      区別できず、`IsSensitiveValue` だけの実装へ戻しても緑のままになる。既存テストは変更
      しない。
- [ ] `internal/runner/config/validation.go` へ、確定した検査のうち外部依存を持たないもの
      （redaction 検査以外のすべて）を追加する。すなわち (a) コマンド名が空でないこと、
      (b) 制御文字（一般カテゴリ Cc）と書式制御文字（同 Cf）を含まないこと、(c) 補間契約を
      通した後に White_Space 以外の rune が 1 個以上残ること、および (d) 長さが上限を超えない
      こと。**検査ごとの対象は 02_architecture.md §3.1 の表に従う。** (a)(b)(c) は
      **command 名だけ**を対象とする。group 名は既存の `ErrEmptyGroupName` と
      `GroupNamePattern`（`^[A-Za-z_][A-Za-z0-9_]*$`）で空も制御文字も空白も既に拒否済みで
      あり、group 名へ広げると到達しない検査になる。(d) だけが group 名と command 名の双方を
      対象とする。長さの上限は既存のどの検証も持たないためである。
      (b) と (c) を独立させる根拠は次のとおりである。制御文字を含みつつ表示できる
      文字も残す名前は (c) を通ってしまい、半角空白だけの名前は (b) を通ってしまう。
- [ ] 検査を足すにあたり、`ValidateGroupNames` の扱いを決める。現在この関数は group だけを
      走査し、関数名・doc コメント（英語）・テスト名も group 名専用である。呼び出し元は
      `internal/runner/config/loader.go:236` の 1 箇所だけなので、**`ValidateIdentifiers` へ
      改名し、doc コメント・呼び出し元・テスト名（`TestValidateIdentifiers`）をすべて更新
      する**。改名せずに command 名の検査を足すと、関数名が内容を偽ることになる。更新対象は
      定義とテストだけではない。呼び出し元の `internal/runner/config/loader.go:236` と、
      旧名を doc コメントで参照している `internal/runner/cli/filter.go:44` と同 `:93` も同じ
      コミットで直す。後者はコンパイルエラーにならないため、放っておくと存在しない関数を指す
      コメントが残る。
- [ ] `internal/runner/bootstrap/identifier_redaction.go` を新規作成し、5 個目の検査
      （redaction の変換が識別子を書き換えないこと）をここに置く。読み込み済みの
      `*runnertypes.ConfigSpec` と `SetupSlackLogging` が返した `*redaction.Config` の 2 個だけを
      受け取り、全 group 名と全 command 名を §4.4 の述語へ通す。**Run ID は受け取らない。**
      02_architecture.md §3.2 のとおり `RunID` は報告境界で `main.go` が代入する値であり、
      構築時にも渡すと 1 個の `PreExecutionError` が 2 箇所から値を得ることになって食い違い
      うる。02_architecture.md §2.2 の当該行も引数を 2 個と書いている。`Config` を
      **この関数の中で組み立てない**（02_architecture.md §3.1、付録 B.11）。返すエラーは
      `internal/runner/config` のセンチネルを包んだ `PreExecutionError` とし、
      `NotificationContext` は `GlobalScope()`、本文には設定内の位置（何番目の group、その中の
      何番目の command）と、どの検査に掛かったかを含める。
- [ ] **本文に識別子の値そのものを入れてはならない。** この検査が拒否する識別子は、定義上
      redaction の変換が書き換える値であり、実際の AWS キー ID・GitHub トークン・JWT・Webhook
      認証情報がそのまま group 名や command 名に書かれている場合を含む。ところが
      `HandlePreExecutionError` の stderr 出力は `RedactingHandler` を通らない。
      `internal/logging/pre_execution_error.go:116-125` は `errorType`・`component`・
      `errorMsg`・`runID` を組み立てて `fmt.Fprint(os.Stderr, ...)` へ直接書いており、
      とくに `:120` の `Details: %s` が本文をそのまま出す。値を本文へ入れると、**redaction が
      潰すはずの秘匿値を、まさにそれを理由に拒否した経路が端末とプロセスログへ平文で出す**
      ことになる。位置を本文に入れるのはこのためでもある。利用者は自分の TOML の該当位置を
      見れば対象を特定でき、値の再掲は不要である。
- [ ] `internal/runner/bootstrap/identifier_redaction_test.go` へ、値が漏れないことを固定する
      テストを置く。実際の秘匿値の形（`AKIA` 形と `ghp_` 形）を command 名に持つ設定を拒否
      させ、**返った `PreExecutionError` の本文とその stderr 表現のどちらにも、その値が部分文字列
      として現れないこと**、および位置と検査名は現れることを assert する。本文へ値を戻す変更を
      入れるとこのテストが落ちる。
- [ ] **本番側の実装に戻って**、`internal/runner/bootstrap/identifier_redaction.go` で、
      受け取った `*redaction.Config` が `nil`（Slack 未設定）のときは
      `redaction.DefaultConfig()` を使う。`nil` は「検査しない」ではない。Phase 1 の
      `RedactingHandler` が既定の `Config` で動き続けており（§1.3）、Slack が無効でも
      識別子は JSON ログで潰れうるためである。この分岐は `NewRedactingHandler` の
      `nil` 扱いと同じ意味であることをコメントに記す。
- [ ] `cmd/runner/main.go` で、`bootstrap.SetupSlackLogging` の戻り値検査の直後、
      `slog.Info("Verification and configuration completed", ...)` の前に上の関数を呼び、
      `SetupSlackLogging` が返した `redactionConfig` をそのまま渡す。`LoadAndPrepareConfig`
      の中へは置かない。本番と同じ変換を体現する `Config` は `AddSlackHandlers` が組み立てる
      まで存在せず、そこは `LoadAndPrepareConfig` より後だからである。
- [ ] **同梱 TOML の改名とハッシュ再記録。** 検査を有効にする前に、`sample/` 配下と
      テストデータの全 TOML の group 名・command 名を新しい述語へ通し、抵触するものを列挙
      する。§1.3 で確認済みの 6 個（`basic_tests`、`cmd_expansion_basic`、
      `args_expansion_basic`、`basic_output_examples`、`basic_auto_env`、
      `api_call_with_token`）を改名する。**改名の影響範囲は実測した。** 6 個の名前を
      `rg` で追うと、現れるのは `sample/*.toml` と本書だけである。`internal/runner/config` の
      後方互換テスト（`loader_compatibility_test.go`、`backward_compat_test.go`、
      `template_backward_compat_test.go`）は識別子を名前で参照しておらず、ファイル名と
      `cfg.Groups[0].Commands[0]` のような位置で参照しているため、期待値の更新は不要である。
      改名後に同じ `rg` を流し、`sample/*.toml` と本書以外に残っていないことを確認する。
      `sample/comprehensive.toml` はハッシュ記録の対象なので、`make hash`・
      `hash-integration-test`・`hash-e2e-test` で記録を取り直す。改名・記録・検査の有効化は
      1 コミットにまとめ、中間状態でテストが落ちないようにする。
- [ ] 再発防止として、`sample/*.toml` を range し、すべての group 名・command 名が新しい
      検査を通ることを assert するメタテストを `internal/runner/bootstrap/identifier_redaction_test.go`
      へ置く。5 個目の検査は `*redaction.Config` を要し、それを受け取る関数がこのパッケージに
      あるためである（Slack 未設定に相当する既定の `Config` で通す）。**`sample/` の位置は
      `../../../sample` のように呼び出し元の深さへ固定した相対パスで書かない。** §4.3 の走査
      ヘルパーと同じ手段でリポジトリ root を解決し、そこからの相対で辿る。これにより、将来
      `key`・`token`・`basic` を含む名前のサンプルが増えても `make test` の段階で検出でき、
      発覚が e2e ターゲットまで遅れることを防げる。
- [ ] `internal/runner/config/validation_test.go` の検証テーブルへ、02_architecture.md §7.1 の
      「設定の検証」の観点の行を追加する。空のコマンド名、制御文字だけの名前、書式制御文字を
      含みつつ表示できる文字も残す名前（`backup` + U+202E + `evil`）、改行を含みつつ表示できる
      文字も残す名前（`backup\nother`）、半角空白だけの名前（`"   "`）、長さ上限ちょうどの
      名前（通る）、上限を 1 byte 超える名前（拒否）を含める。長さの行には、**マルチバイト
      文字を含む command 名で 128 byte 境界をまたぐもの**を必ず入れる。group 名は
      `[A-Za-z0-9_]` に限られ byte 数と rune 数を区別できないため、rune 単位で数える実装を
      落とせるのは command 名の行だけである。判定は `errors.Is` で行う。
- [ ] `internal/runner/bootstrap/identifier_redaction_test.go` を新規作成し、
      02_architecture.md §7.1 の「識別子と redaction」の観点の行を書く。テストは
      読み込み済みの設定と `*redaction.Config` を渡して検査関数を直接呼ぶ。`LoadConfig`
      だけを呼ぶ形では redaction のセンチネルは出ない。語の一致で潰れる名前（`monkey`、`keyring`、
      `rotate_api_key`）は group 名・command 名の双方で、`AKIA` 形と `ghp_` 形も双方で、
      JWT 形と URL 形は command 名の行としてのみ書く。後者を group 名で書かないのは、本番経路
      では `.`・`:`・`/` を含む group 名が `ValidateIdentifiers` の `GroupNamePattern` で先に
      落ち、redaction 検査へ到達しないためである。直接呼び出しなら到達させられるが、それは
      本番に無い入力を固定することであり、02_architecture.md §3.2 の「production では届かない
      経路を単体テストだけで緑にしない」に反する。`slack_allowed_host` を設定した場合と未設定の
      場合を 1 組の行として持ち、どれにも当たらない名前が通ることも同じ表で確認する。
- [ ] **許可ホストの 1 組が何を落とせるかを取り違えない。** この組が落とすのは
      `redaction.DefaultConfig()` を使う実装と `nil` を渡す実装である。
      `NewConfig(WithWebhookHost(cfg.Global.SlackAllowedHost))` で**組み直す実装は落とせない**。
      `AddSlackHandlers` が渡すオプションは `WithWebhookHost` 1 個だけであり
      （`internal/runner/bootstrap/logger.go:405`）、正規化済みホストは
      `cfg.Global.SlackAllowedHost` へ書き戻されているため（`internal/runner/bootstrap/config.go:111`）、
      組み直した `Config` は本番と同一に振る舞う。組み直しを防ぐのはテストではなく §8 の検索と、
      下の `startup_order_guard_test.go` の構文木ガードである。テストのコメントにもこの区別を書き、「組み直しても落ちる」とは
      書かない。許可ホストを大文字で書く行も作らない。`compileWebhookHostPattern` は `(?i)` で
      組み（`internal/redaction/value_detector.go:118`）、`validateWebhookURL` も
      `strings.ToLower` を通す（`internal/logging/slack_handler.go:192`）ため、大文字小文字は
      どの実装でも差を生まない。
- [ ] 同ファイルへ、`*redaction.Config` に `nil` を渡す行を持つ。既定の変換による語一致
      （`monkey` など）と値形式（`AKIA` 形）が依然として拒否され、許可ホストを含む URL 形
      だけが通ることを確認する。`nil` を受けて検査を飛ばす実装を落とすためである。
- [ ] 同ファイルへ、拒否した名前を値に持つ属性が `RedactingHandler` を通ると `[REDACTED]` に
      なることを確認するケースを追加する。設定境界で拒否する理由が実在することを示すためで
      あり、これが無いと検査は「なぜ拒否するのか」を根拠づけられない。
- [ ] **起動経路で実際に呼ばれることを検証する。** 上のテスト群は検査関数を直接呼ぶため、
      `cmd/runner/main.go` が呼び出しを持たなくても、あるいは別の `*redaction.Config` を
      渡していても、すべて緑のままになる。すなわち本番の配線が欠けたまま規定のゲートが
      すべて通り、拒否されるはずの設定が実行され、Scope が `[REDACTED]` に潰れる。関数の
      正しさと配線の存在は別の主張であり、別に確かめる。
      `cmd/runner/integration_pre_execution_error_test.go` へ、redaction が書き換える
      command 名（`AKIA` 形）を持つ TOML で runner を起動し、**どのコマンドも実行されずに**
      起動前エラーで終わることを検証するケースを追加する。`main.go` から呼び出しを 1 行
      削ると落ちる形にする。
- [ ] 同じ配線について、`cmd/runner/startup_order_guard_test.go`（既存、`//go:build test`）へ
      順序と引数の検査を足す。すなわち、検査の呼び出しが `SetupSlackLogging` の**後**に
      あること、および渡している値が `SetupSlackLogging` の戻り値であって
      `redaction.DefaultConfig()` や新規に組み立てた `Config` ではないことを構文木で確かめる。
      **戻り値を受けた変数が、受け取りから検査呼び出しまでの間に再代入・再宣言されていない
      ことも同時に確かめる。** 順序と識別子だけを見ると、
      `redactionConfig, err := SetupSlackLogging(...)` の後に
      `redactionConfig = redaction.DefaultConfig()` を挟んでから渡す形が、同じ識別子・同じ
      順序のまま通ってしまう。同一ブロック内の代入と、内側スコープでの同名再宣言の双方を
      対象にする。新規ファイルを作らず既存の起動順ガードへ足すのは、そこが起動経路の構造を
      見る既存の置き場だからである。
- [ ] **許可ホスト依存の起動経路テストを `cmd/runner/integration_pre_execution_error_test.go`
      へ追加する。** 上の `AKIA` 形の統合テストは「拒否されること」を示すが、**どの `Config`
      で**拒否したかは示さない（既定の `Config` でも `AKIA` 形は拒否されるため）。そこで、
      `slack_allowed_host` を設定した TOML と、**その許可ホストを含む URL 形の command 名**で
      runner を起動し、起動前エラーで終わることを検証する行を持つ。**この行は Webhook の
      環境変数を設定しないと成立しない。** `SetupSlackLogging` は
      `bootstrap.ValidateSlackWebhookEnv()` が `GSCR_SLACK_WEBHOOK_URL_SUCCESS` と
      `GSCR_SLACK_WEBHOOK_URL_ERROR` から設定を作れなければ `nil` を返し、検査は
      `redaction.DefaultConfig()` へ倒れる。既定の `Config` は許可ホストを知らないので URL 形の
      command 名を拒否せず、テストが落ちる。したがって両方の環境変数を**許可ホスト上の URL**
      に設定したうえで起動する（同種の設定は
      `cmd/runner/integration_pre_execution_error_test.go:173` に既存の書き方がある）。この形は
      `WithWebhookHost(<設定した許可ホスト>)` を持つ `Config` でしか拒否されないため、
      `main.go` が `redaction.DefaultConfig()` や新規に組み立てた `Config` を渡す実装、
      および上の再代入を挟む実装では**この行だけが落ちる**。構文木ガードが構造として見る
      ものを、実行時の挙動として裏づける対になる。02_architecture.md 付録 B.11 が共有を
      要求している以上、共有を見る検査は構造と挙動の両方から持つ。

**完了条件**:
- AC-09、AC-10、AC-11、AC-16 の検証が緑である。AC-11 については、構文木ガードのうち
  `PreExecutionError` リテラルを見る半分と、残る 2 発火元（グループ集計、ユーザー／グループ
  指定コマンドの失敗）が正しいスコープを載せることの実行テストが、いずれも本 Phase で入る。
  グループ検証エラー経路の新規テスト（AC-13・AC-14）だけは、本文からの `Group: %s, ` 除去が
  Phase 5 のため Phase 5 に残る。
- 同梱 TOML の改名とハッシュ再記録が済み、`make test`・`make integration-test` が通る。
- 追加した各テストについて、対象の実装を一時的に壊すと失敗することを確認し、その旨を
  コミットメッセージへ記す（AC-32）。
- `make fmt`、`make test`、`make lint` が通る。

### PR-6 作成ポイント: identifier redaction validation at config boundary

**対象ステップ**: Phase 4 §4.4

**推奨タイトル**: `feat(0172): validate identifiers against redaction rules at startup`

**レビュー観点**: 同梱 TOML 6 個の改名・`sample/comprehensive.toml` のハッシュ再記録・検査の有効化が 1 コミットにまとまっていること／`identifier_redaction.go` が `*redaction.Config` を組み直さず `SetupSlackLogging` の戻り値をそのまま使い、`nil`（Slack 未設定）時のみ `DefaultConfig()` に倒すこと／拒否時のエラー本文に識別子の値そのものを含めず位置情報だけを載せていること／`startup_order_guard_test.go` が呼び出し順序と再代入・再宣言の有無まで構文木で検証していること

**実装モデル要件**: frontier-required

**判定理由**: mkplan panel-mode の security-gate／migration トリガーに該当する。ロジック追加（検査ロジック・redaction 述語）とデータ移行（同梱 TOML 6 個の改名・`sample/comprehensive.toml` のハッシュ再記録）を切り離さず 1 PR に収めるのは §4.4 自身の設計判断であり、「改名・記録・検査の有効化は 1 コミットにまとめ、中間状態でテストが落ちないようにする」（§4.4）ためである。検査だけを先に有効化すると同梱 TOML がリポジトリ自身のテストと e2e を壊し、改名だけを先に行うと再発防止のメタテストが検査対象を持たない。この不可分性自体が、単純なロジック追加より高いレビュー慎重度を要する根拠である。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: 通知種別定義・共通エンベロープ・書式の統一

**対象ファイル**: `internal/logging/notification.go`（新規）、
`internal/logging/notification_test.go`（新規）、
`internal/logging/notification_contract_guard_test.go`、
`internal/logging/slack_handler.go`、`internal/logging/slack_handler_test.go`、
`internal/logging/slack_sender.go`、`internal/logging/slack_sender_test.go`、
`internal/logging/pre_execution_error.go`、`internal/runner/runner.go`、
`internal/runner/runner_test.go`、`internal/runner/base/audit/logger.go`、
`internal/runner/base/audit/logger_test.go`、
`internal/runner/e2e_slack_webhook_separation_test.go`、
`internal/runner/e2e_slack_webhook_test.go`、
`cmd/runner/integration_pre_execution_error_test.go`、
`cmd/runner/integration_slack_flush_test.go`

本 Phase は 1 個の取り消し可能なコミットとして入れる（02_architecture.md §8.1）。

#### 5.1 通知種別定義

- [ ] `internal/logging/notification.go` を新規作成し、02_architecture.md §3.4 の
      `notificationPriority`、`messageDetails`、`messageBuilder`、`messageTypeDefinition`、
      `Notification`、`notificationDefinitions`、`registerNotification` を定義する。
      `messageBuilder` は `slog.Record` だけを受け取り、`*SlackHandler` は取らない。
- [ ] 同ファイルで存続する 3 種別を `var` 初期化の 1 回きりの登録として宣言する
      （`command_group_summary` は通常優先度、`pre_execution_error` は高優先度、
      `user_group_command_failure` は通常優先度）。登録の呼び出しはこのファイル内に閉じる。
- [ ] 同ファイルへ、発火元向けの公開アクセサ 3 個と `NotificationAttrs` を置く。
      `NotificationAttrs` はゼロ値の `Notification` を受け取った場合、`slack_notify=true` と
      空の `message_type` を返す（通知を黙って落とさない）。
- [ ] 同ファイルへ製品名の定数 `"go-safe-cmd-runner"` を置く。本番コードでの定義はこの 1 箇所
      だけにする。
- [ ] 同ファイルへ予約フィールド見出しの集合を置く。要素は既存の `fieldTitleHostname`・
      `fieldTitleRunID` と、新設する Scope の見出し定数の 3 個で、共通エンベロープの生成と
      ビルダー側の検査が同じ定数を読む。

#### 5.2 共通エンベロープ

- [ ] `internal/logging/slack_handler.go` へ、ホスト名取得を指す非公開のパッケージ変数
      （初期値は `common.GetHostname`）を置く。書き方は `internal/common/system.go` の
      `osHostname` にならう。
- [ ] `internal/logging/slack_handler.go` に残る 2 箇所の `common.GetHostname()` 直接呼び出し
      を、共通エンベロープ経由に一本化して取り除く。
- [ ] 共通エンベロープの生成を 1 箇所へ実装する。ログレベル、通知コンテキスト、種別固有部分
      から `SlackMessage` を作り、Text 行を
      `[<製品名>] <絵文字> *<STATUS>* — <Scope> : <要約>` の形にする。添付フィールドは種別
      固有フィールドの後ろへ Scope、Hostname、Run ID をこの順で足す。埋め込む動的な値は、
      02_architecture.md §3.5「役割ごとの規則」の対応に従って §4.0 の補間契約を通す。
- [ ] レベル対応表を 02_architecture.md §3.5 の 4 行として全域関数で実装する（上から順に
      最初に一致した行を使い、INFO 未満は WARNING へ倒す）。色と絵文字は既存の
      `colorGood`／`colorWarning`／`colorDanger`、`emojiSuccess`／`emojiWarning`／
      `emojiFailure` を使う。
- [ ] stdout／stderr の切り詰め（`outputMaxLength` 1000、`stderrMaxLength` 500）を
      ヘルパー 1 個へ括り出す。現在 `slack_handler.go` の 621・635 行目付近にインラインで
      重複しており、新ビルダーでさらに増えるためである。上限値そのものは変更しない。

#### 5.3 ビルダーの移行と `Handle` の再構成

- [ ] `buildCommandGroupSummary` を、種別固有部分だけを返す `messageBuilder` へ書き換える。
      Text 行、色、Hostname、Run ID の組み立てを取り除く。コマンド結果ごとの `Command`
      フィールドは合成値のままとし、`cmd.Name` の部分だけを識別子の役割で補間してから合成
      する（終了コードは自由文、バッククォートと `(exit: ...)` は骨格）。切り詰めは §5.2 の
      ヘルパーを使う。
- [ ] `buildPreExecutionError` を同じく種別固有部分だけを返す形へ書き換える。要約は
      `error_type`、固有フィールドは Error Message と Component とする。
- [ ] `user_group_command_failure` のビルダーを新規に書く。要約は
      `command failed (exit <終了コード>)`、固有フィールドは Command、Exit Code、存在する
      場合の Output と Error Output とする。属性名は `common.UserGroupCommandFailureAttrs`
      から引き、文字列リテラルを複製しない。**Output と Error Output には §5.2 のヘルパーで
      stdout 1000 文字・stderr 500 文字の上限を適用する**。発火元は切り詰めていないため、
      ここで適用しないと 02_architecture.md §3.5 の「大量出力」役割の既存規則が守られない。
- [ ] `buildGenericMessage` を、レコードの `Message` を要約とする汎用の種別固有部分を返す形へ
      書き換える。共通エンベロープは他種別と同じ経路で付ける。
- [ ] `internal/logging/slack_handler.go` の `Handle` を 02_architecture.md §6.1 の処理順へ
      再構成する。`message_type` の `switch` は通知種別定義の参照へ置き換える。
- [ ] 未知種別と不正な通知コンテキストの WARN を実装する。メッセージは
      `Slack notification schema violation` に固定し、理由コードを `reasons` 属性へ
      「未知種別 → 通知コンテキスト」の順で列挙する。1 レコードにつき WARN は 1 件とする。
      属性は `message_type`、`run_id`、ログレベル、宣言された scope、`webhook_label` とし、
      通知本文・Webhook URL・group 名・command 名は含めない。出力先は既存の送信失敗ロガー
      だけとする。WARN は受付停止判定より前に出すため、`SlackHandler` からその出力先へ届く
      経路を用意する（現在 `failureLogger` は `slackSender` が持つ）。
- [ ] `slackRequest` へ確定済みの優先度を持たせ、`queueFor` がそれを読む形へ変える。
      未知種別の優先度は 02_architecture.md §3.6 の表に従い、`level >= slog.LevelWarn` を
      高優先度、**それ未満（INFO と、§3.5 の対応表が WARNING へ倒す INFO 未満の値）はすべて
      通常優先度**とする。閾値で書き、`level == slog.LevelInfo` を通常・それ以外を高とする
      書き方はしない。後者は DEBUG の未知種別を予約レーンへ入れる（§3.6 の理由）。
      既知の 3 種別はレベルを見ず、通知種別定義の確定値をそのまま使う。
- [ ] `internal/logging/slack_sender.go` から `isHighPriority` と、種別定数
      `messageTypeCommandGroupSummary`・`messageTypePreExecutionError` を削除する。種別名は
      通知種別定義だけが持つ。定数を参照している既存テストは通知種別定義の公開アクセサ経由
      へ移す。
- [ ] `internal/logging/slack_handler.go` から、使われなくなった `emojiAlert` を削除する。

#### 5.4 発火元の属性生成関数への移行

- [ ] `internal/logging/pre_execution_error.go` の `HandlePreExecutionError` を、Phase 4 の
      中間実装から `NotificationAttrs(PreExecutionErrorNotification(), preExecErr.NotificationContext)`
      の返す属性を渡す形へ差し替える。
- [ ] `internal/runner/runner.go` の `logGroupExecutionSummary` を
      `NotificationAttrs(CommandGroupSummaryNotification(), ...)` 経由へ移し、
      `"slack_notify"` と `"message_type"` の文字列リテラルを取り除く。
- [ ] `internal/runner/base/audit/logger.go` の `LogUserGroupExecution` を
      `NotificationAttrs(UserGroupCommandFailureNotification(), ...)` 経由へ移し、
      `"slack_notify"` と `"message_type"` の文字列リテラルを取り除く。
- [ ] `internal/runner/runner.go` の検証エラー本文から `Group: %s, ` の接頭辞を取り除く。
      除去後の本文は `Total: %d, Verified: %d, Failed: %d, Error: %s` とし、group 名の
      唯一の表示場所を Scope にする。

#### 5.5 テスト

- [ ] **発火元が「正しい」token を選んでいることを、発火元ごとに assert する。** 構文木ガードの
      (b) が見るのは第 1 引数が**登録済みアクセサのいずれか**であることだけで、どの発火元が
      どの token を渡すかは見ない。したがって `LogUserGroupExecution` が
      `UserGroupCommandFailureNotification()` の代わりに
      `CommandGroupSummaryNotification()` を渡しても、(b) は通り、コマンドスコープだけを見る
      `TestLogger_LogUserGroupExecution` も通り、別に組み立てた SlackHandler のテストも通る。
      それでいて本番の失敗通知はグループ集計のビルダーへ回り、AC-23 のメッセージにならない。
      3 発火元の各テスト（`TestLogger_LogUserGroupExecution`、
      `TestLogGroupExecutionSummary_LogLevel`、`TestHandlePreExecutionError_*`）で、
      **捕捉したレコードの `message_type` 属性が期待する種別名であること**を assert する。
      期待値は公開アクセサ経由で引き、文字列リテラルを書かない。token を取り違える変更を
      入れると、その発火元のテストだけが落ちる形にする。
- [ ] リポジトリ全体を歩く走査ヘルパー（`internal/testutil/identitymutationguard/helpers.go`）
      は Phase 4 で括り出し済みである（§4.3）。本 Phase では追加せず、そのまま呼ぶ。
- [ ] `internal/logging/notification_contract_guard_test.go` へ、Phase 4 で入れた
      `PreExecutionError` リテラルの検査に加えて、(a) **`slack_notify` を `true` で構築して
      よいのは `notification.go` の `NotificationAttrs` だけであること**（`internal/logging` を
      含むすべての本番ファイルが対象。パッケージ単位の除外にしない）。**値が `false` の構築は
      拒否しない。** Slack へ送るかどうかを決めるのは `true` だけであり、この検査の目的は
      通知の**発火**を `NotificationAttrs` の 1 本に束ねることだからである。`false` まで
      拒否すると、§4.3 で Phase 4 が意図的に残す `HandleExecutionError` の
      `slack_notify=false`（通知を送らない実行時エラーの経路）が落ち、Phase 5 が緑にならない。
      判定はリテラルの値で行い、変数経由の値は `true` とみなして拒否する（`false` と静的に
      分かる形だけを通す）。および `message_type` の
      文字列リテラルを `internal/logging` 以外のパッケージが直接構築していないこと、
      (b) `NotificationAttrs` の第 1 引数が登録済み token を返す公開アクセサの呼び出しだけで
      あること、(c) **登録済み種別名と同じ文字列リテラルが、`internal/logging/notification.go`
      の登録以外のどの本番ファイルにも現れないこと**を追加する。(c) の期待値は
      `notificationDefinitions` を range して実行時に集め、種別名を検査側へ書き写さない。
      (c) が AC-27 の「単一定義」を旧名の残骸ではなく**構造**で見る行である。ただし
      `slack_sender.go` が集計・送信失敗ログで書く `slog.String("message_type", req.messageType)`
      のように、**属性キー `message_type` と、変数から読んだ種別名**は対象外である。禁じるのは
      種別名の**リテラル**であって、属性キーの使用ではない。
      **(a) で `slack_notify` だけをパッケージ単位ではなくファイル単位に絞る理由。** 通知を
      発生させる引き金は `slack_notify` であり、これを組み立てる正当な場所は
      `NotificationAttrs` 1 箇所しかない。一方 `message_type` は、`slack_sender.go` が集計と
      送信失敗ログで、`slack_handler.go` が読み取りで正当に扱うため、`internal/logging` の中では
      許す必要がある。2 つを同じ粒度で扱うと、緩い方に引きずられて `slack_notify` の除外が
      パッケージ全体に広がる。**これは仮想の攻撃者を想定した話ではない。** 本計画は Phase 4 で
      `pre_execution_error.go` に `slack_notify=true` を手で組ませ（§4.3 の中間実装）、Phase 5 で
      `NotificationAttrs` へ移す（§5.4）。パッケージ単位の除外のままだと、**この移行をやり残しても
      どの検査も鳴らない**。移行の完了を見張るのが (a) の主目的である。
- [ ] (c) だけでは足りないため、(d) を追加する。**`notification.go` 以外の本番コードが
      `Notification` 値を得る経路は、`NotificationAttrs` の第 1 引数位置での公開アクセサ
      呼び出し 1 つだけである。** 具体的には次をすべて拒否する。
      (d-1) 公開アクセサの、その位置以外での呼び出し。
      (d-2) §5.1 が置く**非公開の token 変数**（`commandGroupSummaryNotification` など）への
      参照。`internal/logging` の中からは同じパッケージなので直接触れてしまう。
      (d-3) `notificationDefinitions` そのものへの参照。
      個別の抜け道を 1 つずつ塞ぐ形にせず、**値の入手経路を 1 本に限る**書き方にしてある。
      この検査はここまでに 3 通りの迂回（種別名の文字列、公開アクセサの列挙、非公開変数の
      列挙）が見つかっており、列挙を足していく形では次の抜け道で同じことを繰り返す。
      **拒否する対象の名前は構文木から導く。`notificationDefinitions` からは得られない。**
      同スライスが持つのは `*messageTypeDefinition`（種別名・優先度・ビルダー）だけであり、
      非公開 token 変数の**識別子名**も公開アクセサの**関数名**も入っていない。Go は識別子名を
      実行時に保持しないため、`notificationDefinitions` を range しても (d-1)(d-2) が拒否
      すべき名前は分からない。代わりに `notification.go` の構文木を読み、
      (i) 初期化式が `registerNotification(...)` の呼び出しであるパッケージレベルの `var` 宣言
      からその変数名を、(ii) 同ファイルでそれらの変数を返す関数の宣言からアクセサ名を、
      それぞれ集める。名前を検査側へ書き写さないという意図は変わらず、出どころが実行時の値
      ではなく構文木になる。名前の綴りに依存した推測（`*Notification` で終わる、など）は
      使わない。登録の形が変われば (i) が空を返し、そのとき検査は緑ではなく失敗させる
      （集めた名前が 0 個なら fail する assert を置く）。
      呼び出しの**位置**を縛るのであって、ファイルあたりの個数を数えるのではない。
      (c) は文字列リテラルを見るが、`Notification` は登録値を保持する比較可能な構造体であり、
      公開アクセサがその値を返す。したがって
      `map[Notification]...{CommandGroupSummaryNotification(): ..., ...}` のような
      **token をキーにした 2 本目の登録簿は、種別名の文字列を 1 つも含まずに書ける**。
      (a)〜(c) はこれを素通りさせる。
      **個数を数える形にしないのはなぜか。** 「1 ファイルにつき高々 1 個」でも上の 1 箇所に
      集めた表は弾けるが、3 ファイルがそれぞれ `init` で同じ表へ 1 個ずつ登録する**分散した
      登録簿**は、どのファイルも 1 個しか参照しないため通ってしまう。位置で縛れば、`init` の
      中であれ map リテラルの中であれ変数への代入であれ、`NotificationAttrs` の引数以外に
      現れた時点で落ちる。§5.4 の発火元 3 箇所はいずれも
      `NotificationAttrs(<アクセサ>(), <ctx>)` と第 1 引数へ直接書くため、そのまま通る。
      アクセサをいったん変数へ受けてから渡す書き方も落ちるが、これは意図した制約である
      （分散した登録簿はまさにその形を取る）。アクセサの集合の**出どころは上と同じく構文木**で
      あり、`notificationDefinitions` からは得られない（実行時に関数名は残らない）。
      走査は Phase 4 のヘルパーを使い、対象ディレクトリを書き並べない。
      このファイルを `notification_test.go` と分けるのは、`cmd/runner/startup_order_guard_test.go`
      と同じく、構文木ガードを独立したファイルに置く既存の慣行に合わせるためである。
- [ ] `internal/logging/notification_test.go` を新規作成する。まず、登録済みの各種別について
      代表となる `slog.Record` を作る**フィクスチャ表**を同ファイル内に置く。range する
      テストはこの表からレコードを取り、ビルダーを実際に呼ぶ。表が無いと range テストは
      メタデータしか見られない。
- [ ] 同ファイルで `notificationDefinitions` を range し、02_architecture.md §7.1 の
      「共通エンベロープ」「予約フィールド見出し」「単一定義」「製品名」の観点を検証する。
      種別名の一意性、公開アクセサが返す token と定義の同一性、どのビルダーも予約見出しを
      使わないこと、静的な部分に `###` が無いこと、末尾 3 フィールドの順序、Text 行が製品名で
      始まることを含める。
- [ ] 同ファイルへ、`notificationDefinitions` を range して**各種別固有ビルダーが返す
      フィールドの値がすべて 02_architecture.md §3.5「動的な値の一覧」のいずれかの行に対応
      すること**を検証するテストを置く。一覧に無いフィールドを足すと失敗する形にする。
      これが §3.5 の 2 つの一覧を将来にわたって噛み合わせ続ける仕掛けである。
- [ ] `internal/logging/slack_handler_test.go` へ、02_architecture.md §7.1 の次の観点を
      追加する。レベル表示の全域性（全レベルに対して絵文字・STATUS・色が定義されていること）、
      レベル表示の内容が全種別で一致すること、ゼロ値トークン、構築の遅延、未知種別、WARN の
      件数、不正な通知コンテキスト（`(scope: invalid)` と WARN）、ユーザー／グループ指定
      コマンドの失敗、識別子を載せる他フィールド、エンベロープ値の出力の性質（ホスト名の
      継ぎ目を差し替える）。
- [ ] 同ファイルへ、未知種別で**レベルが WARN 以上のレコードは通常キューが満杯でも高優先度で
      送られる**ことを検証するケースを追加する（02_architecture.md §7.1「未知種別」）。
- [ ] 同ファイルへ、識別子を切り詰めないことを検証するケースを追加する。長さ上限ちょうどの
      group 名が Scope と Text 行に全体として現れること、および**先頭が長く一致する 2 つの
      group 名が異なる Scope として表示される**ことを行として持つ。後者が無いと、接頭辞で
      切り詰める実装でも短い名前の assert は通ってしまう。
- [ ] 同ファイルへ、02_architecture.md §7.3 の負の検証を 2 件追加する。(a) 未知種別と不正
      スコープの WARN が送信失敗ロガーだけへ届き、新しい Slack 通知を再帰的に発生させない
      こと。(b) WARN に通知本文、group 名、command 名、Webhook URL が含まれないこと。
      前者は通知のループを、後者は秘匿値の漏れを防ぐ検証であり、WARN の件数を数えるだけの
      テストでは代替できない。
- [ ] `internal/logging/slack_handler_test.go` の `TestSlackHandler_Handle_WithMockServer` を
      新しい書式に合わせて更新する。
- [ ] **空の `message_type` を使う既存テストの一括見直し。** `internal/logging/slack_sender_test.go`
      には `slackRecord(level, "", text)` が 29 箇所ある（§1.3）。Phase 5 以降これらは未知種別
      として WARN を 1 件増やす。共有ヘルパー `slackRecord` の側で、既定を登録済み種別と
      通知コンテキスト付きに変えるのか、未知種別のまま WARN を織り込むのかを一度に決め、
      テストごとに場当たりで直さない。決めた方針を同ファイルの doc コメントへ英語で記す。
      影響を確認する箇所は `TestSlackSender_FlushLogsMessageTypeBreakdown` の集計キー、
      478 行目と 726 行目付近の出現数の assert、および「失敗の記録自体が Slack 要求を
      生まないこと」を見る assert である。`internal/logging/slack_handler_test.go` にも同じ
      形が無いか確認する。
- [ ] `internal/logging/slack_sender_test.go` の高優先度・キュー溢れ・種別別集計のテストを、
      確定済み優先度を持つ `slackRequest` の形に合わせて更新する。優先度を通常へ倒すと
      `TestSlackSender_HighPriorityBypassesFullNormalQueue` が失敗することを確認する。
- [ ] `internal/logging/slack_handler_test.go` へ、未知種別の優先度が全域であることを
      検証する行を追加する（02_architecture.md §7.1）。**レベルから優先度を返す写像を関数
      として直接呼ぶ表**とし、ERROR・WARN・INFO に加えて INFO と WARN の中間値
      （`slog.LevelInfo + 2`）と DEBUG の行を持つ。`level == slog.LevelInfo` だけを通常と
      する実装へ変えると中間値の行が失敗することを確認する。**`Handle` を通す形でこの表を
      書かない。** 本番が構築するのは `LevelModeExactInfo`（`bootstrap/logger.go:355`）と
      `LevelModeWarnAndAbove`（同 `:379`）だけで、どちらも中間値と INFO 未満を弾くため、
      `Handle` 経由で緑にすると 02_architecture.md §3.2 が禁じる形になる。`Handle` を通す
      行は本番に到達する INFO・WARN・ERROR だけとする。
- [ ] `internal/runner/runner_test.go` の**グループ集計**については、通知コンテキスト属性が
      載ることの検証は Phase 4（§4.3）で済んでいる。本 Phase では
      `TestLogGroupExecutionSummary_LogLevel` が `NotificationAttrs` 経由へ移した後も同じ属性
      を出し続けることを確認するだけでよく、新しい assert は要らない。
      `TestSlackNotification` は拡張先にしない。同テストは名前に反して
      `runner.runID` が設定されていることしか assert しておらず、宣言だけされて未使用の
      `expectedStatus`／`expectedCalls` フィールドが残っている。通知レコードを一切見ないため、
      拡張は実質的な新規作成になる。
- [ ] **検証エラー経路のテストは新規に書く。** `Group: <name>, ` を出す分岐は
      `runner.go:429`、すなわち `executeGroups`（`Execute` 経由）の中にある。
      `TestSlackNotification` が呼ぶのは `ExecuteGroup`（`runner.go:493`）であり、この分岐へは
      到達しない。`verification.Error` を返す検証マネージャを与えて `Execute` を通し、Scope に
      group 名が一度だけ現れること、Error Message から `Group: <name>, ` が消えていることを
      検証するテストを追加する。既存テストの表に行を足す形では到達経路が変わらない。
- [ ] `internal/runner/base/audit/logger_test.go` の `TestLogger_LogUserGroupExecution` は、
      コマンドスコープの通知コンテキストを持つことの検証を Phase 4（§4.3）で済ませてある。
      本 Phase では `NotificationAttrs` 経由への移行後も同じ属性が載ることを確認する。
- [ ] `cmd/runner/integration_pre_execution_error_test.go` へ、SlackHandler の登録後に起きる
      グローバルな起動前エラー（グローバル対象ファイルの検証失敗）で Scope が `(global)` に
      なることを検証するケースを追加する。設定ファイルの読み込み・解析の失敗は登録前に起きて
      通知が発生しないため使わない。
- [ ] `internal/runner/e2e_slack_webhook_separation_test.go` を新書式に合わせて更新し、
      INFO は成功用、WARN 以上はエラー用という宛先分離が変わらないことを確認する。
- [ ] `internal/runner/e2e_slack_webhook_test.go` の `TestE2E_SlackWebhookWithMockServer` を、
      新しいペイロード全体を検証する形へ更新する。
- [ ] `cmd/runner/integration_slack_flush_test.go` の
      `TestIntegration_RunnerFlushesSlackOnNormalExit` を、通知コンテキストを持つレコードと
      新書式に合わせて更新する。
- [ ] `go test -race -tags test ./internal/logging/...` を実行し、通知種別定義の参照と既存の
      並行投入・flush に競合が無いことを確認する。
- [ ] 追加・変更した各テストについて、対象の実装（コンストラクタ呼び出し、共通エンベロープ、
      通知種別定義の要素、優先度、補間契約の置き換え集合の 1 文字）を一時的に壊すと失敗する
      ことを確認し、その旨をコミットメッセージへ記す（AC-32）。

**完了条件**:
- AC-12〜AC-15、AC-17〜AC-27、AC-31〜AC-33 の検証が緑である。
- `make fmt`、`make test`、`make lint` が通り、この Phase が 1 コミットになっている。
- **`make slack-e2e-test` が Linux で通る。macOS の結果はこのゲートを満たさない。**
  同 target は `uname -s` が `Darwin` のとき**テストを実行せず** `go vet -tags e2e,test` だけを
  走らせる（Makefile の Darwin 分岐。3 テストが `/usr/bin/echo` を要求し macOS に無いため）。
  したがって macOS だけで確認すると、宛先分離やペイロードの assert が実行時に落ちる状態でも
  AC-31 を緑にできる。macOS で作業する場合は、コンパイルと vet が通ることをローカルの確認と
  し、**ゲートの判定は Linux（CI もしくは開発コンテナ）の実行結果で行う**。本 Phase が変更する
  `internal/runner/e2e_slack_webhook_separation_test.go` と
  `internal/runner/e2e_slack_webhook_test.go` は `//go:build e2e && test` を持ち、
  `make test`・`make lint`・`go test -race -tags test ./internal/logging/...` のいずれも
  `e2e` タグを付けないためこの 2 ファイルを**ビルドしない**（Makefile 自身が
  「neither `make test` nor `make lint` builds with the e2e tag」と記している）。この target を
  加えないと、2 ファイルがコンパイルできない状態でも AC-31 が落ちる状態でも、規定のゲートは
  すべて緑になる。なお同 target は `-list` の一致数を `SLACK_E2E_COUNT` と突き合わせるため、
  テストを増減した場合は同じコミットで `SLACK_E2E_COUNT` を更新する。

### PR-7 作成ポイント: unify Slack notification format via notification type definitions

**対象ステップ**: Phase 5 §5.1 / §5.2 / §5.3 / §5.4 / §5.5

**推奨タイトル**: `feat(0172): unify Slack notification format and type definitions`

**レビュー観点**: **本 PR は 1 コミットで不可分だが、レビューは §5.1→§5.2→§5.3→§5.4→§5.5 の小見出し順に段階的に読み進めること**（型定義 → 共通エンベロープ → ビルダー移行と `Handle` 再構成 → 発火元の移行 → テストの順で、後段は前段の型・関数を前提にする）。個別の重点は、`notification_contract_guard_test.go` の (a)〜(d) がそれぞれ意図した迂回経路（`slack_notify` の直接構築、種別名リテラルの複製、非公開 token 変数・公開アクセサの位置外呼び出し）を実際に塞ぐこと／未知種別・不正スコープの WARN が送信失敗ロガーだけへ届き Slack 通知を再帰させず秘匿値を含まないこと／レベル→表示のマッピングが全域（DEBUG・中間値を含む）で検証されていること／`make slack-e2e-test` を Linux で実行した結果を確認していること（macOS の結果はゲートを満たさない）

**実装モデル要件**: frontier-required

**判定理由**: mkplan panel-mode トリガー（重い統合テスト／外部リソース面: `e2e && test` タグの Slack e2e テスト 2 ファイルが `make slack-e2e-test` でしかビルドされず、Linux 限定実行が必須）に該当する。加えて AC-27 の token 迂回防止 AST 検査（種別名リテラル走査だけでなく、非公開 token 変数・公開アクセサの構文木ベースの経路制限）は前例の無い設計判断であり、Phase 5 全体が 1 個の取り消し可能なコミットとして扱われる高リスクの統合ステップでもある。この不可分性（差分をこれ以上 PR 単位で割れないこと）が、レビュー観点の先頭で段階的読解の順序を明示している理由でもある。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 6: 文書の更新と翻訳

**対象ファイル**: `docs/user/runner_command.ja.md`、
`docs/dev/architecture_design/security-architecture.ja.md`、
`docs/dev/architecture_design/slack_async_delivery.ja.md`、`README.ja.md`、
`docs/user/security-risk-assessment.ja.md`、`02_architecture.md`、および対応する英語版

- [ ] `docs/user/runner_command.ja.md` の `### 4.2 通知設定` へ、通知される 3 種別、統一書式の
      Text 行の形、Scope の表示（`(global)`・`group=<名前>`・
      `group=<名前> command=<名前>`・`(scope: invalid)`）、Text 行の先頭に付く製品名、および
      ドライラン時に通知が送られないことを日本語で記す。
- [ ] 追加した記述を実装と突き合わせる。Text 行の例は、実装後の
      `internal/logging/notification_test.go` が検証する形と一字一句一致させ、Scope の表示は
      02_architecture.md §3.6 の表と対応させる。一致は目視ではなく、テストの期待値と文書の
      例を並べて確認する。
- [ ] `docs/dev/architecture_design/security-architecture.ja.md` の 898・1307・1310 行目の
      セキュリティイベント Slack 通知の記述を、削除後の実態へ改める。406・1143 行目は監査
      ログ全般の記述であり、削除後も事実として残るかを確認し、変更しない場合はその判断を
      コミットメッセージへ記す。
- [ ] `docs/dev/architecture_design/slack_async_delivery.ja.md` 35 行目の
      「`highPriority`(セキュリティアラート等)」を `pre_execution_error` へ改める。
- [ ] `README.ja.md` 96 行目の「セキュリティイベントのリアルタイム通知」を、実際に通知される
      内容（グループ実行の結果と実行前エラー）へ改める。57 行目は監査ログ全般の記述であり、
      上と同じ扱いとする。
- [ ] `docs/user/security-risk-assessment.ja.md` 301 行目の「高優先度キュー（セキュリティ
      アラート等）」を `pre_execution_error` へ改める。あわせて、通常キューの飽和で個々の
      失敗通知が落ちうる残余リスク（02_architecture.md §5.4）を記す。465 行目は監査ログ全般の
      記述であり、上と同じ扱いとする。
- [ ] `02_architecture.md` §2.2 のコンポーネント配置表へ、表示安全な補間契約を実装する
      `internal/common` のファイルの行を追加する（§1.3「表示安全な補間契約の配置」）。
      設計判断は変わらないが、表に配置が書かれていないままだと実装と文書が食い違う。
- [ ] 上記の日本語版をコミットする。
- [ ] `/mktrans` で `README.md`、`docs/user/security-risk-assessment.md`、
      `docs/dev/architecture_design/security-architecture.md`、
      `docs/dev/architecture_design/slack_async_delivery.md`、`docs/user/runner_command.md`
      へ翻訳を反映する。日英を直接両方編集しない。
- [ ] 翻訳後、`docs/user/runner_command.md` の `### 4.2 Notification Configuration` と日本語版
      の該当節を対照し、通知種別・書式・Scope の 4 形・製品名が過不足なく対応していることを
      確認する。英語版が日本語をそのまま貼り付けただけになっていないことも見る。

**完了条件**:
- AC-28、AC-29、AC-30 の検証が緑である。
- 日本語版と英語版が別コミットに分かれている。

### PR-8 作成ポイント: update user and developer documentation

**対象ステップ**: Phase 6

**推奨タイトル**: `docs(0172): update notification docs for unified format and removed types`

**レビュー観点**: `docs/user/runner_command.ja.md` に載せた Text 行の例が `internal/logging/notification_test.go` の期待値と一字一句一致していること／日本語版を先にコミットし英語版は `/mktrans` で反映していること（直接両方編集していないこと）／削除した 3 種別についての記述が実態（監査ログ記述は残す・Slack 通知記述は改める）に沿って区別されていること

**実装モデル要件**: standard

**判定理由**: 文書更新と翻訳のみで、設計判断や高リスク分岐を伴わない。Conditional checks・panel-mode トリガーいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 7: 全体検証と実 Slack 表示確認

- [ ] `make fmt`、`make test`、`make lint`、`make deadcode` をすべて実行して通す。
- [ ] `go test -race -tags test ./...` を実行する。
- [ ] `make slack-e2e-test` を **Linux で**実行する。上の `-tags test` は `e2e` を含まないため、
      `e2e && test` タグを持つ Slack の e2e テスト 2 ファイルはここまでのどのコマンドでも
      ビルドされない。AC-31 の宛先分離はこの target でしか実行されない。**macOS で回しても
      テストは走らない**（Darwin 分岐が `go vet` だけを実行する）ため、macOS の結果を
      本 Phase の確認としない。
- [ ] 本書 §7 の受け入れ基準検証表の全行を実行し、結果を記録する。
- [ ] 本書 §8 の横断検索チェックリストを実行する。
- [ ] `GSCR_SLACK_WEBHOOK_URL_SUCCESS` と `GSCR_SLACK_WEBHOOK_URL_ERROR` をテスト用チャンネル
      の Webhook に設定し、`make slack-notify-test` と `make slack-group-notification-test` を
      実行する。これらが出すのは `command_group_summary`（成功・失敗の両方）である。
- [ ] `pre_execution_error` を実チャンネルで確認するため、SlackHandler 登録後にグローバル
      対象ファイルの検証を失敗させる設定（AC-15 の統合テストと同じ材料）で runner を 1 回
      実行する。02_architecture.md §8.2 が「3 種別、未知種別、宛先分離を確認してから展開」と
      定めており、上の 2 ターゲットだけでは種別が 1 つしか出ないためである。
- [ ] `user_group_command_failure` と未知種別についても、実チャンネルで表示を確認する手段を
      用意して実行する。未知種別は本番の発火元が無いため、確認できない場合はモックサーバーの
      ペイロード検証をもって代え、その旨を記録する。
- [ ] 上記の各実行について、02_architecture.md §5.3 の 5 項目（`*SUCCESS*` などの強調、
      `—` と `[...]` がそのまま表示されること、プッシュ通知で製品名が読めること、添付の色、
      末尾 3 フィールドの順序）を確認する。
- [ ] 強調が期待どおり表示されない場合、02_architecture.md §5.3 の代替（強調記法を外して素の
      文字列にする）は**そのままでは適用できない**。AC-18 は Text 行が
      `[<製品名>] <絵文字> *<STATUS>* — <スコープ> : <要約>` の形であることを
      `*<STATUS>*` の `*` ごと字面で要求しており（01_requirements.md の F-004）、
      `TestNotificationDefinitions_TextLineFormat` もその形を assert する。`*` を落とすと、
      受け入れテストを落としたまま出すか、承認済みの要件から離れる向きにテストを弱めるかの
      どちらかになる。したがって代替を採るときは、**先に 01_requirements.md の AC-18 を改訂
      して強調記法を要求から外し、02_architecture.md §5.3 と本書 §7 の AC-18 の行、および
      `TestNotificationDefinitions_TextLineFormat` の期待値を同時に更新する**。表示を直す前に
      要件を直す。§1.2 の 1 と同じ理由であり、実装の都合で受け入れ基準を後から緩めない。
- [ ] 実表示の確認結果を記録する。確認できない環境の場合は、モックサーバーによるペイロード
      検証を必須とし、実表示未確認をリリース前の残存リスクとして記録する。
- [ ] リリースノートに新旧のペイロード例と、追加される通知コンテキスト属性を示す。ペイロード
      例は `internal/logging/notification_test.go` の期待値から起こし、実装と一致させる。

**完了条件**: 全 AC と Success Criteria を満たす。

### PR-9 作成ポイント: final verification and live Slack display confirmation

**対象ステップ**: Phase 7

**推奨タイトル**: `chore(0172): run full verification suite and confirm live Slack display`

**レビュー観点**: §7 受け入れ基準検証表と §8 横断検索チェックリストの全行を実行した記録が残っていること／実 Slack 表示確認（または未確認時のモックサーバー代替検証）の記録が残っていること／AC-18 の強調表示が期待どおりでない場合、表示を直す前に 01_requirements.md の AC-18 を改訂する手順が守られていること

**実装モデル要件**: standard

**判定理由**: 既定のコマンド実行と結果記録が中心で、新規の設計判断を伴わない。Conditional checks・panel-mode トリガーいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## 3. 実装順序とマイルストーン

| マイルストーン | 含む Phase | 成果物 | 判定 |
|---|---|---|---|
| M1: 死んだ種別の除去 | Phase 1〜3 | 3 個の独立した削除コミットと、特権昇格結果ログの新規テスト | AC-01〜AC-08 が緑。`make deadcode` が新たな到達不能コードを報告しない |
| M2: 型と伝搬 | Phase 4 | 補間契約、通知コンテキスト、`RuntimeCommand.GroupName`、構造体化した `PreExecutionError`、識別子の設定検証、同梱 TOML の改名 | AC-09〜AC-11、AC-16 が緑。Slack の表示はまだ旧書式のまま |
| M3: 書式の統一 | Phase 5 | 通知種別定義、共通エンベロープ、WARN、`user_group_command_failure` の固有ビルダー | AC-12〜AC-15、AC-17〜AC-27、AC-31〜AC-33 が緑 |
| M4: 文書 | Phase 6 | 日本語版 5 文書、02_architecture.md の追補、英語版 5 文書 | AC-28、AC-29、AC-30 が緑 |
| M5: 全体検証 | Phase 7 | 実 Slack 表示の確認記録、リリースノート | 全 AC と Success Criteria |

M2 の時点では Slack の表示は変わらない。AC-12〜AC-15 と AC-17 が M3 の判定に入るのは、
いずれも表示についての基準であり、Phase 4 のコミットでは満たしようがないためである
（02_architecture.md §8.2）。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `privileged_command_failure` の本番コードとテストを削除 | standard |
| PR-2 | Phase 2 | `security_alert` の本番コードとテストを削除し、高優先度テストを `pre_execution_error` へ移す | standard |
| PR-3 | Phase 3 | `privilege_escalation_failure` の本番コードとテストを削除し、特権昇格結果ログのテストを追加 | frontier-recommended |
| PR-4 | Phase 4 §4.0 / §4.1 / §4.2 | 表示安全な補間契約、通知コンテキストの型、`RuntimeCommand.GroupName` | frontier-recommended |
| PR-5 | Phase 4 §4.3 | `PreExecutionError` の構造体化と存続する 3 発火元への伝搬、AC-09 のコンストラクタ迂回検査 | frontier-recommended |
| PR-6 | Phase 4 §4.4 | 識別子の設定検証、redaction 検査の配線、同梱 TOML の改名とハッシュ再記録 | frontier-required |
| PR-7 | Phase 5 §5.1 / §5.2 / §5.3 / §5.4 / §5.5 | 通知種別定義、共通エンベロープ、ビルダー移行、`Handle` 再構成、WARN、構文木ガード (a)〜(d) | frontier-required |
| PR-8 | Phase 6 | 利用者向け・開発者向け文書の更新と翻訳 | standard |
| PR-9 | Phase 7 | 全体検証と実 Slack 表示確認 | standard |

## 4. テスト戦略

テストの観点は 02_architecture.md §7 が定義する。本節では、その観点を実装作業へ割り当てる
方針だけを記す。

### 4.1 単体テスト

- **新規**: `internal/common/interpolation_test.go`（補間契約と切り詰め）、
  `internal/common/notification_context_test.go`（通知コンテキストの型、エンコード往復、
  妥当性判定の全行、下位キーの重複）、`internal/logging/notification_test.go`
  （通知種別定義の集合を range する共通契約とフィールド一覧の対応）、
  `internal/logging/notification_contract_guard_test.go`（構文木の静的契約）、
  `internal/runner/bootstrap/identifier_redaction_test.go`（識別子の redaction 検査）。
- **拡張**: `internal/logging/slack_handler_test.go`（レベル表示、共通エンベロープ、未知種別、
  WARN の件数と内容、識別子を載せるフィールド、切り詰めないこと。いずれも Phase 5）。
  `RedactingHandler` を挟んだエンコード往復の検証は Phase 4 に置き、下位ハンドラを捕捉用
  ハンドラとして復元関数を直接呼ぶ（§4.1）。Phase 4 の `SlackHandler` はまだ通知コンテキストを
  読まないため、`SlackHandler` を経路に含めるとテストが宣言した理由で失敗しえない。
  `internal/runner/config/validation_test.go`（識別子の設定検証）、
  `internal/redaction/redactor_test.go`（新しい述語と層の切り分け）、
  `internal/runner/base/runnertypes/runtime_test.go`（`GroupName`）、
  `internal/runner/base/privilege/unix_privilege_test.go`（特権昇格結果ログ）。
- **置換**: `internal/logging/slack_sender_test.go` の高優先度テストの基準種別を
  `security_alert` から `pre_execution_error` へ移す。
- **削除**: 削除する 3 種別の発火元テスト 6 個、補助型 `sensitiveLogValuer`、Slack 側の該当
  テーブルケース 3 個。削除の前後で `go tool cover -func` を比較し、存続する関数のカバレッジ
  が下がっていないことを各コミットメッセージへ記す。

### 4.2 層の切り分け

補間契約のテストは、役割ごとに分けて書く。識別子と自由文は「切り詰めの有無」で挙動が違う
ため、同じ入力を両方の役割で通し、識別子では切り詰められないことを確かめる。これにより、
役割の `switch` を取り違えた実装が落ちる。

設定検証のテストも同様に、制御文字の検査と「表示できる内容を持たない」の検査を別々に落とせる
入力を持つ（制御文字を含みつつ表示できる文字も残す名前と、制御文字を含まない半角空白だけの
名前）。

redaction の述語のテストでは、値形式の各行について `IsSensitiveValue` 単独では素通りすること
を先に assert する。これが層の切り分けであり、これが無いと述語が語一致だけの実装へ戻っても
行が緑のままになる。

### 4.3 統合テストと後方互換

- SlackHandler 登録後のグローバルな起動前エラー（`cmd/runner/integration_pre_execution_error_test.go`）。
- 検証エラー経路で group 名が Scope に一度だけ現れること（`internal/runner/runner_test.go`）。
- 宛先分離が変わらないこと（`internal/runner/e2e_slack_webhook_separation_test.go`）。
- 終了時 flush で新書式の通知が失われないこと（`cmd/runner/integration_slack_flush_test.go`）。
- 同梱 TOML を読む後方互換テスト群が、改名後も通ること。
- 既存のトップレベル `group` 属性、stdout 1000 文字・stderr 500 文字の切り詰め、redaction の
  適用範囲、ドライランの契約が変わらないこと。

### 4.4 セキュリティテスト

02_architecture.md §7.3 の 5 項目を実施する。うち 3 項目は §2 の具体的なタスクへ割り当てて
ある。補間契約の入力（§4.0）、WARN の出力先と再帰の不在（§5.5）、WARN に秘匿値が入らないこと
（§5.5）である。残る 2 項目は `go test -race ./internal/logging/...`（§5.5）と、ドライランで
Webhook へ届かない既存テストの維持（§4.3）である。裸の URL については、自動リンクが残ること
と表示文字が URL そのものと一致することを期待値として書き、残余リスクを黙って通さない。

## 5. リスク管理

### 5.1 技術リスク

| リスク | 影響 | 対応 |
|---|---|---|
| 同梱 TOML が新しい識別子検証に抵触する（確認済み、6 個） | リポジトリ自身のテストと e2e が読み込み失敗する | Phase 4 で改名・ハッシュ再記録・検査の有効化を 1 コミットで行い、`sample/*.toml` を range するメタテストで再発を防ぐ |
| 実 Slack で `*STATUS*` の強調が期待どおり表示されない | Text 行が読みにくくなる。AC-18 が `*<STATUS>*` を字面で要求しているため、強調記法を外すと受け入れ基準から外れる | Phase 7 で実機確認する。外す場合は先に 01_requirements.md の AC-18 を改訂し、02_architecture.md §5.3・本書 §7 の AC-18 の行・`TestNotificationDefinitions_TextLineFormat` の期待値を同時に更新する（§Phase 7）。実装の都合で受け入れ基準を後から緩めない |
| Phase 5 が 1 コミットに集約されるため差分が大きい | レビューの負荷と回帰の切り分けが難しい | Phase 4 までで型と伝搬を終え、Phase 5 の差分を表示の変更だけに絞る。問題があれば Phase 5 の単一コミットを取り消す |
| 統一書式が Slack ワークフローや監視ルールを壊す | 外部の運用が止まる | リポジトリ内の利用箇所と文書を先に検索する。外部利用者にはリリースノートで新旧のペイロード例を示す。テスト用チャンネルで先に検証する |
| 識別子の設定検証が利用者の既存 TOML を拒否する | 既存利用者の設定が読み込めなくなる | センチネルを検査ごとに独立させ、直す箇所と直し方が分かるメッセージにする。リリースノートに拒否される名前の条件（機密語を含む名前を含む）を明記する |
| 削除に伴うカバレッジ低下を見落とす | 存続コードの検証がサイレントに薄くなる | 各削除コミットで `go tool cover -func` を前後比較し、結果をコミットメッセージへ記す |
| `seteuid` 経路が CI で到達不能 | AC-05 の片方の分岐が実質未検証になる | native root は `WithPrivileges` 経由、`seteuid` は `logElevationOutcome` の境界で検証し、到達不能な理由をテストの doc コメントに残す（§Phase 3）。境界で検証できるのは記録の側だけで、`escalatePrivileges` が成功時に `execCtx.elevation` へ `elevationSeteuid` を代入する行は本番コードに注入点（`syscall.Seteuid` を差し替える seam）を設けない限り未検証のまま残る。代入を削除しても `make test` は緑であり、これは本 Phase で閉じない残余リスクとして記録する |

### 5.2 スケジュールリスク

| リスク | 影響 | 緩衝策 |
|---|---|---|
| Phase 4 の同梱 TOML 改名とハッシュ再記録が想定より広がる | Phase 4 が長期化し、Phase 5 の着手が遅れる | 改名対象は Phase 4 着手時に全 TOML を述語へ通して確定させ、範囲を先に見積もる。ハッシュ再記録が必要なファイルは `sample/comprehensive.toml` だけであることを確認済み |
| Phase 5 の差分が大きく、レビューが 1 回で終わらない | M3 が滞留する | Phase 4 までで型・伝搬・設定検証を完了させ、Phase 5 のレビュー範囲を表示に限定する。§5.1〜§5.5 の小見出し単位でレビューを分割できる形に保つ |
| Phase 7 の実 Slack 確認に必要な Webhook を用意できない | リリース判断が遅れる | 実表示未確認を残存リスクとして記録し、モックサーバーのペイロード検証で代替する手順を Phase 7 に含めてある |
| `/mktrans` による英語版反映が Phase 6 内に収まらない | M4 が滞留する | 日本語版と英語版を別コミットに分けてあるため、日本語版だけを先に確定できる |

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: Phase 1）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2）
- [ ] PR-3 マージ済み（対象ステップ: Phase 3）
- [ ] PR-4 マージ済み（対象ステップ: Phase 4 §4.0 / §4.1 / §4.2）
- [ ] PR-5 マージ済み（対象ステップ: Phase 4 §4.3）
- [ ] PR-6 マージ済み（対象ステップ: Phase 4 §4.4）
- [ ] PR-7 マージ済み（対象ステップ: Phase 5 §5.1 / §5.2 / §5.3 / §5.4 / §5.5）
- [ ] PR-8 マージ済み（対象ステップ: Phase 6）
- [ ] PR-9 マージ済み（対象ステップ: Phase 7）

## 7. 受け入れ基準の検証

各行の「種別」は `test`（実行可能で、挙動を壊すと失敗する）、`static`（`rg` またはビルド）、
`manual`（PR やデプロイでの観察）を表す。

検索コマンドは、パイプによる選択（`|`）を使わず `-e` を並べる形で書いてある。Markdown の表
セル内でパイプを書くとエスケープが必要になり、そのまま写すと `rg` の正規表現では**リテラルの
パイプ文字**を探すことになって、直っていなくても一致 0 件で緑に見えるためである。

| AC | 種別 | 検証方法 |
|---|---|---|
| AC-01 | static | `rg -n -e privileged_command_failure -e PrivilegedCommandFailureAttrs -e buildPrivilegedCommandFailure -e messageTypePrivilegedCommandFailure --type go cmd internal` が一致なし（終了コード 1） |
| AC-02 | static | `rg -n -e security_alert -e SecurityAlertAttrs -e buildSecurityAlert -e LogSecurityEvent -e messageTypeSecurityAlert --type go cmd internal` が一致なし。かつ `rg -n -e common.SeverityCritical -e common.SeverityHigh --type go cmd internal` が一致なし。かつ `rg -n -e SeverityCritical -e SeverityHigh internal/common/` が一致なし（`runerrors` の同名定数を巻き込まないため 3 本に分ける） |
| AC-03 | static | `rg -n -e privilege_escalation_failure -e PrivilegeEscalationFailureAttrs -e buildPrivilegeEscalationFailure -e LogPrivilegeEscalation -e messageTypePrivilegeEscalationFail --type go cmd internal` が一致なし |
| AC-04 | static | 表の下の**注 1** の 3 コマンドをすべて満たすこと。シェルのパイプを含むため表の外に置いてある |
| AC-05 | test | `internal/runner/base/privilege/unix_privilege_test.go` の新規 2 テスト。native root 側は `WithPrivileges` 経由で、`unix.go:129` の `defer m.logElevationOutcome(execCtx)` を外すと失敗する。`seteuid` 側は `logElevationOutcome` の境界で検証する（非 root では `WithPrivileges` 経由の到達が不可能なため。§Phase 3） |
| AC-06 | static | Phase 1〜3 の各コミットの直前と直後に `go test -tags test -coverprofile=<file> ./internal/logging/... ./internal/runner/base/audit/... ./internal/common/...` を実行し、`go tool cover -func=<file>` を関数ごとに比較する。検証は `git log -1 --format=%B <sha>` に比較結果が含まれることで行う |
| AC-07 | test | `internal/logging/slack_sender_test.go::TestSlackSender_HighPriorityBypassesFullNormalQueue`（`pre_execution_error` へ移行済み）。優先度判定を常に通常へ倒すと失敗する |
| AC-08 | static | Phase 3 の完了時と Phase 7 で `make deadcode` を実行し、新たな到達不能コードの報告が無い |
| AC-09 | test + static | test: `internal/common/notification_context_test.go::TestNotificationContext_ZeroValueIsGlobalScope` と `::TestGroupScope_EmptyNameIsInvalidAtDisplayBoundary`（`GroupScope("")` が構築でき、`scope=group`・`group=""` としてエンコードされ、判定が `invalid_notification_context` を返すこと）。static + test: 本番コードがコンストラクタを迂回して `NotificationContext` の値を作っていないこと。**複合リテラルだけでなく、`var ctx common.NotificationContext` と `new(common.NotificationContext)` も拒否する。** 01_requirements.md の AC-09 は「値はコンストラクタでのみ構築する」に加えて「**発火元はグローバルな場合も `GlobalScope()` で明示的に**通知コンテキスト属性を付与する」と定めている。ゼロ値がグローバルスコープとして正しいことは、この要求を満たす理由にならない。AC-09 が求めているのは値の妥当性ではなく発火元での**明示性**であり、`var` 宣言で暗黙にグローバルになる書き方はまさにそれが禁じる形である（グローバルであることが、書いてあることではなく書いていないことから決まる）。**判定は `internal/logging/notification_contract_guard_test.go` の構文木検査で行う。** `internal/common` を import するファイルごとに、Phase 4 で括り出した走査と `ResolveLocalImports` で**その file の実際の import 名を解決**し、解決結果が `internal/common` を指す修飾子つきの `NotificationContext` について、**複合リテラル・`var` 宣言・`new` 呼び出しのいずれも**拒否する（3 形すべてがコンストラクタを迂回してゼロ値を作る経路である）。補助として `rg -n -g '!*_test.go' -F "common.NotificationContext{" --type go cmd internal` が一致なし（`-F` は `{` を量指定子として解釈させないため必須。付け忘れると `regex parse error` で落ちる。`--type go` は `*.go` で `_test.go` を含むため、本番だけを見るには `-g '!*_test.go'` が要る。テストは複合リテラルを正当に書く）。**この `rg` は補助でしかない。** 既定の import 名を literal で探すため、`import c ".../internal/common"` と別名を付けて `c.NotificationContext{}` と書けば素通りする。これは正規の Go であり、`GlobalScope()` を呼ばずにゼロ値を構築できてしまう。別名に強い判定は AST 側だけが持つ |
| AC-10 | test | `internal/common/notification_context_test.go::TestNotificationContext_LogValueEncoding`。`scope` と `group` が常に出力され、command 名が無い場合に `command` が出ないことを検証する |
| AC-11 | test + static | static: `internal/logging/notification_contract_guard_test.go`（本番コードの `PreExecutionError` リテラルが `NotificationContext` を省略していないこと。省略したリテラルを 1 個足すと失敗する）。test: `internal/runner/runner_test.go::TestLogGroupExecutionSummary_LogLevel`（グループ集計が通知コンテキストを持つこと）とグループ検証エラー経路の新規テスト（§5.5）、および `internal/runner/base/audit/logger_test.go::TestLogger_LogUserGroupExecution`（失敗経路がコマンドスコープを持つこと）。構文木ガードは引数 1 個の形しか見ないため、残る 2 発火元が実際に正しいスコープを載せることは実行テストで確かめる。**この 2 件の実行テストは、属性を載せるコード変更と同じ Phase 4 に置く**（§4.3）。Phase 5 へ送ると、2 発火元から属性を落としても Phase 4 が緑のままになり、Phase 4 の完了条件が確かめていないものを緑と称することになる |
| AC-12 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_InvalidNotificationContext`。02_architecture.md §3.1 の判定表と §3.6 の理由コードを行として持ち、`(scope: invalid)` の表示と送信失敗ロガーへの WARN を検証する。表示できる文字を残さない名前の行も含む |
| AC-13 | test | `internal/runner/runner_test.go` に新規追加するグループ検証エラーのテスト（`Execute` 経由で `executeGroups` の検証エラー分岐へ到達させる。Scope に group 名が現れること）。既存の `TestSlackNotification` は `ExecuteGroup` しか呼ばずこの分岐に到達しないため使わない（§5.5） |
| AC-14 | test | 同上。Error Message に `Group: ` が現れないことを assert する |
| AC-15 | test | `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope`。SlackHandler 登録後に起きるグローバル対象ファイルの検証失敗を使い、Scope が `(global)` になることを検証する |
| AC-16 | test | `internal/runner/base/runnertypes/runtime_test.go::TestRuntimeCommand_GroupName` |
| AC-17 | test | `internal/runner/base/audit/logger_test.go::TestLogger_LogUserGroupExecution`（コマンドスコープの属性）と `internal/logging/slack_handler_test.go::TestSlackHandler_UserGroupCommandFailure`（Scope に group 名と command 名の双方が表示されること） |
| AC-18 | test | `internal/logging/notification_test.go::TestNotificationDefinitions_TextLineFormat`。フィクスチャ表から各種別のレコードを取り、`notificationDefinitions` を range して Text 行が `[<製品名>] <絵文字> *<STATUS>* — <Scope> : <要約>` の形であることを検証する |
| AC-19 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_LevelDeterminesDisplay`。`Handle` を通す行は本番に到達する INFO・WARN・ERROR だけとし、絵文字・STATUS・色が全種別で同じになることを検証する。中間値（`slog.LevelInfo + 2`）と INFO 未満は、レベルから表示を返す写像を関数として直接呼ぶ形で全域性だけを確認する。本番が構築する 2 モードはどちらもこの 2 種類を `Enabled` で弾くため、`Handle` 経由の行は作らない（02_architecture.md §3.5） |
| AC-20 | test | `internal/logging/notification_test.go::TestNotificationDefinitions_EnvelopeHasNoHeadingMarkup`（静的な部分に `###` が無い）と `internal/logging/slack_handler_test.go::TestSlackHandler_EnvelopeValueProperties`（ホスト名の継ぎ目を差し替え、Hostname フィールドの値が §3.5 の出力の性質 5 項目を満たすこと） |
| AC-21 | test | `internal/logging/notification_test.go::TestNotificationDefinitions_TrailingFieldsOrder`（末尾 3 件の順序）と `::TestNotificationDefinitions_BuildersAvoidReservedFieldTitles`（ビルダーが予約見出しを使わない）。どちらも `notificationDefinitions` を range する |
| AC-22 | test + static | test: 上記 2 テストと `::TestNotificationDefinitions_FieldsAreDeclaredInInventory`（各ビルダーの返すフィールドが §3.5 の一覧のいずれかに対応すること）。static: `rg -n "type messageBuilder" internal/logging/notification.go` の結果が `func(slog.Record) messageDetails` であること（`*SlackHandler` を取らないため、ビルダーはエンベロープを組み立てる手段を持たない） |
| AC-23 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_UserGroupCommandFailure`。固有メッセージとして組み立てられ、コマンド名と終了コードが含まれることを検証する。加えて `internal/runner/base/audit/logger_test.go::TestLogger_LogUserGroupExecution` が、**捕捉したレコードの `message_type` が `user_group_command_failure` であること**を assert する（§5.5）。前者はハンドラへ正しい種別が渡った場合の組み立てを見るだけで、発火元がその種別を選んだかは見ない。構文木ガードの (b) も第 1 引数が登録済みアクセサのいずれかであることしか見ないため、発火元が別の token を渡す取り違えはこの assert だけが捕まえる |
| AC-24 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_UnknownMessageType`（汎用メッセージの送信、`unknown_message_type` の WARN、WARN 以上は通常キュー満杯でも高優先度で送られること）、`::TestSlackHandler_SchemaViolationWarnIsSingle`（未知種別と不正な通知コンテキストが同時に成立しても WARN が 1 件で、`reasons` が規定の順に並ぶ）、`::TestSlackHandler_SchemaViolationWarnDoesNotRecurse`（WARN が送信失敗ロガーだけへ届き Slack 通知を再発しない）、`::TestSlackHandler_SchemaViolationWarnOmitsSensitiveValues`（WARN に通知本文・group 名・command 名・Webhook URL が含まれない） |
| AC-25 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_GenericMessageHasEnvelope` |
| AC-26 | test | `internal/logging/notification_test.go` の各テストが `notificationDefinitions` を range して書かれていること。エンベロープを満たさない種別を 1 個登録すると失敗することを確認する |
| AC-27 | test + static | test: `internal/logging/notification_test.go::TestNotificationDefinitions_UniqueTypesAndTokens` と `internal/logging/notification_contract_guard_test.go` の (c)（登録済み種別名と同じ文字列リテラルが `notification.go` の登録以外のどの本番ファイルにも現れないこと）と (d)（`notification.go` 以外の本番コードが `Notification` 値を得る経路は `NotificationAttrs` の第 1 引数位置での公開アクセサ呼び出しだけであること。公開アクセサのそれ以外の位置での呼び出し、非公開 token 変数への参照、`notificationDefinitions` への参照をすべて拒否する）。(c) の期待値は `notificationDefinitions` を range して実行時に集める。(d) が拒否する識別子名は実行時には得られない（同スライスは種別名・優先度・ビルダーしか持たず、Go は変数名も関数名も実行時に保持しない）ため、`notification.go` の構文木から、`registerNotification(...)` で初期化されるパッケージレベル `var` とそれを返す関数の宣言として集める。**AC-27 の主たる検証はこの (c) と (d) である。** (c) だけでは不足する。`Notification` は登録値を持つ比較可能な構造体なので、token をキーにした 2 本目の dispatch／優先度表は種別名の文字列を 1 つも含まずに書け、(c) を素通りする。(d) を「値の入手経路を 1 本に限る」形で書くのは、迂回を 1 つずつ塞ぐ形が続かないためである。個数制限は分散した `init` 登録を通し、公開アクセサだけの制限は非公開 token 変数を通した。発火元 3 箇所は `NotificationAttrs(<アクセサ>(), <ctx>)` の形で書くため通る。**ただしこの検査は、事故で 2 本目の登録簿ができることを防ぐものであって、意図的な迂回を全て塞ぐものではない**（同一パッケージ内では最終的に何でも書ける）。完全性を追うより、経路を 1 本に保つことを設計側で維持する。 static: `rg -n -g '!*_test.go' "messageType[A-Z]" internal/logging/` の一致が `messageTypeDefinition` の行だけであること、かつ `rg -n -g '!*_test.go' "func isHighPriority" internal/logging/` が一致なし（`-g` を付けないとテストが持つ参照まで数え、削除済みでも赤くなる。`messageTypeDefinition` を残る一致として許すのは、§5.1 が定義する**新しい型名**がこのパターンに一致するためで、除かないと Phase 5 を設計どおり実装した時点でゲートが赤になる。狙いは旧い種別定数（`messageTypeSecurityAlert` など）だけである。1 本目の結果を 2 本目の `rg -v` へパイプで渡す書き方はしない。本節冒頭のとおり、表セル内のパイプはエスケープが要り、そのまま写すと壊れた検索になる）。**この 2 本の `rg` は補助でしかない。** どちらも HEAD の**旧名**（`messageTypeX` という綴りと `isHighPriority` という関数名）だけを探すため、別名で書かれた 2 本目の登録簿は素通りする。実際 HEAD では種別定数が `slack_sender.go` にあるのに種別の `switch` は `slack_handler.go:344` にあり、ファイルを限定した検索では種別の分岐そのものを取り逃す。ゆえに検索対象はファイルではなくパッケージ全体とし、「並行する登録簿が無いこと」自体は (c) の構造検査で見る |
| AC-28 | static + test | static: **5 語を 1 本の `rg` にまとめてはならない。1 語につき 1 本ずつ、5 本を個別に実行し、それぞれの終了コードが 0 であることを見る**（注 2 のループを使う）。`-e` を並べた 1 本は「いずれか 1 つでも一致した行」を出し、1 語でも当たれば終了コード 0 になるため、残る 4 語が文書から抜けていても緑になる。各語は `rg -n -F -e "<語>" docs/user/runner_command.ja.md` の形で、**`-F` は必須**である。付け忘れると `(global)` は捕捉グループとして解釈され、括弧の無い裸の `global` にも一致するため、Scope の表記が書かれていなくても緑になる。対象 5 語は `[go-safe-cmd-runner]`、`(global)`、`command_group_summary`、`pre_execution_error`、`user_group_command_failure`。test: 文書に載せた Text 行の例が `internal/logging/notification_test.go` の期待値と一字一句一致することを、Phase 6 の突き合わせタスクで確認する（presence だけでは書式の誤記を検出できない） |
| AC-29 | static | AC-28 と同じ 5 語を、同じく**1 語 1 本ずつ**（注 2 のループ）`docs/user/runner_command.md` に対して実行し、それぞれ終了コード 0。まとめた 1 本では 1 語の一致で 5 語すべてを満たしたことになってしまう。加えて `### 4.2 Notification Configuration` の節が Scope の 4 形（`(global)`、`group=`、`command=`、`(scope: invalid)`）を英語で説明していることを目視で確認する（日本語をそのまま貼り付けただけの状態を通さないため） |
| AC-30 | static | Phase 1〜7 の各コミット sha について、作業ツリーが clean な状態で `git checkout <sha> && make test && make lint` を実行し、いずれも終了コード 0。確認後 `git checkout -` で戻る |
| AC-31 | test | `internal/runner/e2e_slack_webhook_separation_test.go::TestE2E_SlackWebhookSeparation_MessageFormat` ほか同ファイルの分離テスト。INFO が成功用、WARN 以上がエラー用のハンドラだけで有効になることを検証する。**実行は `make slack-e2e-test` を Linux で行う。** 同ファイルは `//go:build e2e && test` を持ち、`make test` も `make lint` も `go test -tags test ./...` も `e2e` タグを付けないためビルドされない。この target を回さないと、AC-31 は「実行されていない」を「緑」と取り違える。**macOS の実行結果はこの行を満たさない。** 同 target は Darwin では `go vet` だけを走らせテストを実行しないため（Makefile の Darwin 分岐）、macOS だけの確認は「ビルドが通った」以上を意味しない |
| AC-32 | static | Phase 3〜5 の各コミットについて `git log -1 --format=%B <sha>` に、壊した対象と失敗を確認したテスト名の記述が含まれること。対応するテストは AC-05、AC-09〜AC-27 の各行が指す |
| AC-33 | test + static | test: `internal/logging/notification_test.go::TestNotificationDefinitions_TextLineFormat`（登録済み種別）と `internal/logging/slack_handler_test.go::TestSlackHandler_GenericMessageHasEnvelope`（汎用メッセージ）。どちらも Text 行が製品名で始まることを assert する。static: `rg -n -g '!*_test.go' '"go-safe-cmd-runner"' --type go cmd internal` がちょうど 1 件（本番コードでの定義が 1 箇所。import パス `github.com/isseis/go-safe-cmd-runner/...` はこの引用符付きパターンに一致しないことを HEAD で確認済み）。**`-g '!*_test.go'` は必須**である。`rg --type-list` が示すとおり `--type go` は `*.go` であり、`_test.go` を含む。除外しないと、製品名の接頭辞を assert するテスト（AC-18・AC-33 の `notification_test.go`）が持つリテラルを本番の定義と一緒に数え、**正しい実装を「定義が複数ある」として落とす** |

**注 1: AC-04 の検証コマンド**

```sh
# (1) Phase 1〜3 がちょうど 3 コミットであること（3 以外なら非 0 で落ちる）
test "$(git rev-list --count <Phase 1 の親>..<Phase 3 の HEAD>)" -eq 3
git log --oneline <Phase 1 の親>..<Phase 3 の HEAD>   # 内訳の目視用

# (2) 各コミットが他の 2 種別に触れていないこと（一致なし = 終了コード 1 を期待）
#     --unified=0 と '^[+-]' で変更行だけに絞る。3 個の種別定数は
#     slack_sender.go:59-63 で隣接しており、コンテキスト行まで数えると
#     正しいコミットでも一致してしまうためである。
git show <sha> --unified=0 -- '*.go' | rg '^[+-]' | rg -e <他 2 種別の message_type>

# (3) 各コミットが、統合後の枝から単独で revert 可能であること（終了コード 0 を期待）
#     Git の 3-way revert そのもので確かめる。作業ツリーは毎回元へ戻す。
#     <base> は revert を実際に行う想定の枝、すなわち Phase 3 完了時点（削除 3 コミットが
#     揃った状態）とする。1 コミットずつ、毎回 <base> から始めて試す。
#     失敗は status へ溜め、後片付けの後にそれで抜ける。`|| echo` だけで
#     済ませると echo と後続の reset が成功するため、衝突しても 0 で終わる。
#     `git reset --hard <base>` は内容を戻すだけで detached HEAD のままなので、
#     最後に元のブランチへ必ず戻す。戻し忘れると以降のコミットが
#     detached HEAD に積まれて失われる。
orig=$(git rev-parse --abbrev-ref HEAD)
rc=0
for sha in <Phase 1 の sha> <Phase 2 の sha> <Phase 3 の sha>; do
  if ! git checkout --detach <base> >/dev/null 2>&1; then
    echo "CHECKOUT FAILED: <base>"; rc=1; break
  fi
  if ! git revert --no-commit "$sha"; then
    echo "REVERT FAILED: $sha"; rc=1
  fi
  git revert --quit 2>/dev/null || true
  git reset --hard <base> >/dev/null
done
git checkout "$orig" >/dev/null || rc=1
exit "$rc"
```

**注 2: AC-28・AC-29 の 1 語 1 本の検索**

```sh
# 5 語すべてが存在することを、語ごとに独立した終了コードで確かめる。
# <doc> は docs/user/runner_command.ja.md（AC-28）または
# docs/user/runner_command.md（AC-29）。
# 欠落を status へ溜め、最後にそれで抜ける。`|| echo` だけで済ませると
# echo が成功するためループが 0 で終わり、欠落しても緑になる。
missing=0
for term in '[go-safe-cmd-runner]' '(global)' command_group_summary \
            pre_execution_error user_group_command_failure; do
  if ! rg -n -F -e "$term" <doc> >/dev/null; then
    echo "MISSING: $term"
    missing=1
  fi
done
exit "$missing"
```

`rg -F -e A -e B -e ...` を 1 本で書いてはならない。`rg` は「いずれかのパターンに一致した
行」を出し、1 語でも当たれば終了コード 0 を返す。5 語のうち 4 語が抜けていても緑になり、
AC-28 が主張する「5 個の語すべてについて 1 件以上」を確かめたことにならない。

**注 1 の補足**: `git apply -R --check` は使わない。`--check` が見るのは逆パッチが**その時点の作業ツリー**へ
文字どおり当たるかだけで、3-way マージを行わない。コミット自身の上で走らせれば当たるのが
当たり前（ほぼ恒真）であり、後続 Phase を積んだ枝の上で走らせると、Git の revert なら解決
できる無害なコンテキスト差でも落ちる。AC-04 が主張するのは「統合された枝から 1 件ずつ
取り消せること」であり、それを確かめられるのは 3-way マージを行う `git revert` だけである。

## 8. 横断検索チェックリスト

`make lint` と `make test` では検出できない残存参照と表記の一致だけを挙げる。§7 の検証表に
ある検索はここへ重複させない。

- [ ] 削除した 3 種別の名残がコメント・テスト名・エラー文言に残っていないこと:
      `rg -n -i -e "security alert" -e "privilege escalation" -e "privileged command" --type go cmd internal` の結果が、
      存続する機能についての記述だけであること（`internal/runner/base/privilege` の昇格処理
      そのものは残るため、0 件にはならない。1 件ずつ見て通知に関する記述が無いことを確かめる）。
- [ ] 削除した 3 種別が文書に残っていないこと。識別子だけでなく**散文の語**も探す:
      `rg -n --glob '!docs/tasks/**' -e security_alert -e privilege_escalation_failure -e privileged_command_failure -e セキュリティアラート -e "security alert" docs README.ja.md README.md` の結果が、
      Phase 6 で残すと判断した監査ログ関連の記述だけであること。散文を検索語に入れるのは、
      実際の残骸が `README.ja.md:96` の「セキュリティイベントのリアルタイム通知」や
      `security-risk-assessment.ja.md:301` の「セキュリティアラート等」のように散文だから
      であり、snake_case だけを探すと直っていなくても 0 件になる。
- [ ] `Group: ` の重複表示の名残:
      `rg -n '"Group: %s, ' --type go internal` が一致なし。
- [ ] 改名した検証関数の旧名が残っていないこと: `rg -n ValidateGroupNames --type go cmd internal`
      が一致なし。コメントの中の参照（HEAD では `internal/runner/cli/filter.go:44` と同 `:93`）は
      コンパイルエラーにならないため、この検索でしか捕まらない。
- [ ] 識別子の redaction 検査が `redaction.Config` を組み直していないこと:
      `rg -n "redaction\.(NewConfig|DefaultConfig)" internal/runner/bootstrap/identifier_redaction.go`
      の結果が、`nil` を受けたときの `DefaultConfig()` の 1 件だけであること。`NewConfig` が
      現れたら、02_architecture.md 付録 B.11 が退けた「同じ式で組み直す」形へ戻っている。
- [ ] 新設する識別子の名前衝突: `NotificationContext`、`NotificationScope`、`GlobalScope`、
      `GroupScope`、`CommandScope`、`Notification`、`NotificationAttrs` の各々について
      `rg -n --type go cmd internal` を実行し、本タスクが定義した箇所とその利用箇所以外に
      同名の別物が無いことを確認する。§4.4 のセンチネルはこの対象に含めない。パッケージが
      違えば同名でも共存するためであり、実際に `config.ErrEmptyGroupName` と
      `resource.ErrEmptyGroupName` は今も共存している。
- [ ] 用語の一致: 日本語版文書で「通知コンテキスト」「通知種別定義」「種別固有部分」
      「共通エンベロープ」「送信失敗ロガー」が 02_architecture.md の用語表と同じ意味で
      使われていること。`rg -n -e 通知コンテキスト -e 通知種別定義 -e 種別固有部分 -e 共通エンベロープ docs/user docs/dev README.ja.md` の結果を目視で確認する。
- [ ] 翻訳の一致: Phase 6 で追加した英語の用語が `docs/translation_glossary.md` に登録済みで
      あるか、未登録なら `/mktrans` の手順に従って登録すること。

## 9. 成功基準

### 9.1 機能の完成度

- 存続する 3 種別と汎用メッセージのすべてが、製品名で始まる統一書式の Text 行と、末尾 3
  フィールド（Scope、Hostname、Run ID）を持つ。
- 通知から、グローバルか、どの group か、どのコマンドかが判別できる。
- 本番で発火しない 3 種別が production コードに残っていない。
- 種別の一覧・メッセージの組み立て・キュー優先度が単一の定義から引かれている。

### 9.2 品質

- `make test`、`make lint`、`make deadcode` が通る。
- `go test -race -tags test ./...` が通る。
- `make slack-e2e-test` が Linux で通る（`e2e && test` タグのファイルは上のどれもビルドせず、
  macOS ではこの target 自体がテストを実行しない）。
- 削除の前後で、存続する関数のカバレッジが下がっていない。
- 各テストが、対象の挙動を壊すと失敗することを確認済みである。

### 9.3 セキュリティ

- 02_architecture.md §7.3 の 5 項目が緑である。
- 補間契約の出力が、1 行であること、制御文字と書式制御文字を含まないこと、実体参照化されて
  いること、長さ上限を超えないこと、有効な UTF-8 であることの 5 つの性質を満たす。
- 未知種別と不正スコープの WARN が送信失敗ロガーだけへ届き、新しい Slack 通知を再帰的に
  発生させない。WARN に通知本文・group 名・command 名・Webhook URL が含まれない。

### 9.4 文書

- `docs/user/runner_command.ja.md` と `docs/user/runner_command.md` に通知種別・統一書式・
  Scope・製品名が記載され、Text 行の例が実装のテスト期待値と一致している。
- 削除した通知を機能として記載していた 4 文書が、日英とも実態に合わせて更新されている。
- 02_architecture.md §2.2 のコンポーネント配置表に補間契約の配置が記載されている。

## 10. 次のステップ

- 本書のレビューと承認（status を `approved` にする）。
- 承認後、Phase 1 から実装に着手する。
- Phase 7 の実 Slack 表示確認の結果を本書へ追記する。
- 統一書式の破壊的変更と、識別子の設定検証で拒否される名前の条件をリリースノートへ記載する。
- 02_architecture.md §9 が挙げる将来の拡張（製品名の設定上書き、Block Kit への移行、削除した
  通知の再配線）は、必要になった時点で別タスクとして起票する。
