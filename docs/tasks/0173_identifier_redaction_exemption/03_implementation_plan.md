# 実装計画書: 識別子の型宣言と値ベース redaction からの免除

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-13 |
| Review date | - |
| Reviewer | - |
| Comments | - |

本計画書で既存コードについて述べる箇所は、特に断りのない限り commit `26cf9564`（本計画書作成時点の HEAD。`docs(0173): Approved the architecture document`）で確認した。行番号は編集でずれるため、guard（§1.3 で説明する構文木テスト）は宣言サイト（§1.1 で説明する production の記述行）の合否を行番号ではなくファイル・関数・引数式・件数で判定する（§Phase 4）。本文が引用する検索コマンドは、すべて本書作成時に commit `26cf9564` で実行し、その結果を §7 の注に記録した。

## 関連文書

- [01_requirements.md](01_requirements.md) — 受け入れ基準（AC-01〜AC-19）の原本
- [02_architecture.md](02_architecture.md) — 本計画が実装する設計。以降の参照はすべて同文書の節番号を指す
- [requirements_process.md](../../dev/developer_guide/requirements_process.md) — 文書構成と AC トレーサビリティの規約
- [test_organization.md](../../dev/developer_guide/test_organization.md) — テストヘルパーの配置規約

## 1. 実装概要

### 1.1 目的

group 名・コマンド名を `common.Identifier` という型で宣言し、`internal/redaction` の値ベース redaction（key=value 置換・値形式検出・値まるごと判定）を受けないようにする。免除は宣言型の値だけに作用させ、同じ文字列でも自由文（コマンド行・引数・環境変数値・stdout・stderr・message・error）として書かれたものは従来どおり redact する。下流ハンドラ（JSON・text・Slack）へは string に正規化して渡し、読み取り側のコードと redaction のパターン集合は変更しない。あわせて、宣言サイト（group 名・コマンド名をログ属性値として書く production の行）を一覧（目録）として固定する構文木 guard（§1.3）を追加し、コマンド行を誤って識別子と宣言した場合はテストが失敗するようにする。

### 1.2 実装原則

1. **宣言は型で行う。** 免除の判断をキー名でも値の内容でもなく、`common.Identifier` という具象型で行う（Declare, don't infer）。任意の `slog.LogValuer` を免除する一般則は設けない。
2. **免除は値ベース redaction の手前で行う。** `slog.Attr` を扱う層で型を見て string へ正規化し、`RedactText` へは識別子を渡さない（`RedactText` は呼び出し時点で型情報を失う）。
3. **免除は宣言だけに基づかせる。** `SensitivePatterns`・`DefaultKeyValuePatterns`・`ValueDetector` のパターン集合は変更しない（設計 §1.1 原則 6）。
4. **下流は string を受け取る。** 免除経路は `slog.StringValue(name)` を返し、JSON・text・Slack の読み取りコードと `message_formatter` を変更しない（AC-06）。
5. **宣言サイトは目録と双方向に照合する。** 本番コードの `NewIdentifier` 呼び出しは設計 §3.4 の目録と一致しなければならず、目録外の宣言も目録の欠落も guard テストが失敗させる（AC-07、AC-08）。
6. **フェーズごとに green gate を通す。** 各コミットで `make fmt`・`make test`・`make lint` を通し、挙動を壊して失敗を確認したテストをコミットメッセージに記す（AC-18、AC-19）。

### 1.3 既存コード調査結果

commit `26cf9564` で次を確認した。

**識別子型**

- `Identifier`・`NewIdentifier` はまだリポジトリに存在しない（`rg -n -F 'NewIdentifier' -g '*.go' internal cmd` が 0 件）。追加先は `internal/common/identifier.go` の 1 ファイルで、既存ファイルとの名前衝突は無い。
- `internal/common` は `internal/` 配下のどのパッケージも import していない（`go list` で確認）。したがって設計 §2.1 の新規依存辺 3 本（`internal/redaction`・`internal/runner/base/executor`・`internal/runner/base/privilege` → `internal/common`）はいずれも非循環である。`internal/redaction` の production コードは現在どの内部パッケージにも依存しない。
- `internal/logging`・`internal/runner`・`internal/runner/resource`・`internal/runner/base/audit`・`internal/runner/config`・`internal/verification` は既に `internal/common` を直接 import しており、新しい辺は増えない。
- `cmd/` 配下に宣言サイトは無い。
- `slog.Value.String()` は `KindLogValuer` を解決せず `fmt.Append` で整形する（Go 1.26.3 の `log/slog/value.go:466-477`）。`Identifier` が `fmt.Stringer` を実装すれば、`slog.String` を前提とする既存の読み取り（[`internal/logging/slack_handler.go:718`](../../../internal/logging/slack_handler.go) の `extractFromAttrs`、[`internal/common/logschema_test.go:72`](../../../internal/common/logschema_test.go)）は名前を得られる。これが `String` を併せて実装する理由である（設計 §3.1）。

**免除を挿入する 3 箇所（設計 §3.2）**

- `Config.RedactLogAttribute`（[`internal/redaction/redactor.go:301`](../../../internal/redaction/redactor.go)）はキー名判定（`:312`）の後、`KindString`（`:317`）と `KindGroup`（`:332`）の分岐の前。
- `RedactingHandler.redactLogAttributeWithContext`（[`redactor.go:764`](../../../internal/redaction/redactor.go)）はキー名判定（`:769`）の後、`switch value.Kind()`（`:774`）の前。これが本番の主要経路である。
- `processSlice`（[`redactor.go:1264`](../../../internal/redaction/redactor.go)）は要素ごとの `element.(slog.LogValuer)` 型アサーション（[`:1330`](../../../internal/redaction/redactor.go)）の前。要素は `LogValue()`（[`:1368`](../../../internal/redaction/redactor.go)）を経由すると解決済み string になり型が失われるため、先頭で見る必要がある。
- `processLogValuer`（[`redactor.go:930`](../../../internal/redaction/redactor.go)）は解決後の値を `redactLogAttributeWithContext` へ再帰し、`processMap`（[`:1054`](../../../internal/redaction/redactor.go)）と `processStruct`（[`:1139`](../../../internal/redaction/redactor.go)）も各値・各フィールドを同関数へ再帰する。挿入点を 3 つに絞っても、グループ・マップ・構造体の経路は同じ判定を通る。
- `RedactingHandler.Handle`（[`redactor.go:722`](../../../internal/redaction/redactor.go)）は `record.Message` に `Config.RedactText` のみを適用する。error 属性は `processError` 経由で `RedactText` に続けて値まるごと判定が掛かる（設計 §5.2 の残余リスク）。

**通知コンテキストの符号化と復元（設計 §3.3）**

- `NotificationContext.LogValue`（[`internal/common/notification_context.go:78`](../../../internal/common/notification_context.go)）は `group` を常に、`command` を空でないときだけ `slog.String` で符号化する。
- `decodeNotificationContextParts`（[`notification_context.go:136`](../../../internal/common/notification_context.go)）は各下位値が `slog.KindString` であることを要求する（`:166`、`:169`、`:172`）。`slog.Value.Resolve()` はトップレベルの `LogValuer` だけを解決し、グループの下位値までは再帰しない（Go 1.26.3 `log/slog/value.go:499` 以降）。
- Slack 側の読み取りは `checkNotificationContext`（[`internal/logging/slack_handler.go:431`](../../../internal/logging/slack_handler.go)）で、属性を `Resolve()` してから `DecodeNotificationContext` に渡す。`display`（[`slack_handler.go:491`](../../../internal/logging/slack_handler.go)）は復号済みの名前を補間するため変更を要さない。
- テスト補助の `tu.LogRecorder`（[`internal/testutil/handlers.go:33`](../../../internal/testutil/handlers.go)）は `a.Value.Any()` を保存し `Resolve` しない（`:77`、`:84`）。そのため宣言型は `common.Identifier` のまま捕捉される。`NotificationContext()`（[`handlers.go:227`](../../../internal/testutil/handlers.go)）は `NotificationContext` と `[]slog.Attr` の両形式を復号する。`handlers.go` は `//go:build test || performance || integration` を持ち、`test` タグを必須としないため、`ProductionGoFilesInRepo` は同ファイルを production として走査する。テストの期待値を `NewIdentifier` で組み立てるコードは `_test.go` にだけ置き、`internal/testutil`・`internal/runner/base/executor/testutil` などの補助パッケージには置かない。

**宣言サイト（設計 §3.4、§3.5）**

- 宣言サイトの目録（ファイル・関数・行・キー・値の式）を全件調べ、設計 §3.4 の記載と一致することを確認した。対象は `internal/common`（`notification_context.go`・`logschema.go` の 2 ファイル）・`internal/runner`（`group_executor.go`・`runner.go`・`config/expansion.go` の 3 ファイル）・`internal/runner/base/executor`（`executor.go`・`tempdir_manager.go` の 2 ファイル）・`internal/runner/base/privilege`（1 ファイル）・`internal/runner/base/audit`（1 ファイル）・`internal/runner/resource`（2 ファイル）・`internal/verification`（1 ファイル）の計 12 ファイル、属性サイトは 43 件である（`SecurityLogger` へ渡す引数 2 件と `buildCommandDebugLogArgs` へ渡す引数 1 件を含む）。これは要件定義書の「約 13 ファイル・40 属性サイト」と整合する。
- `SecurityLogger` の 4 メソッド（[`internal/logging/security.go:22`](../../../internal/logging/security.go)、`:31`、`:40`、`:49`）は現在 `cmdName string` を直接 `"command"` に載せる。production の呼び出し元は `LogUnlimitedExecution`（[`internal/runner/group_executor.go:537`](../../../internal/runner/group_executor.go)）と `LogTimeoutExceeded`（[`:585`](../../../internal/runner/group_executor.go)）の 2 件だけで、`LogLongRunningProcess` と `LogTimeoutConfiguration` には production の呼び出し元が無い。設計 §3.4 のとおり、宣言は各呼び出し元で行い、メソッド内部では行わない。
- `buildCommandDebugLogArgs`（[`internal/runner/group_executor.go:553`](../../../internal/runner/group_executor.go)）は `cmdName string` を `logArgs` にそのまま詰める。設計 §3.4 のとおり引数型を `common.Identifier` にし、宣言は呼び出し元（[`:612`](../../../internal/runner/group_executor.go)）で行う。
- キー `"command"` は識別子と展開済みコマンド行の両方に載る。`DefaultExecutor.executeWithUserGroup` は同一関数内で `cmd.Name()`（[`internal/runner/base/executor/executor.go:254`](../../../internal/runner/base/executor/executor.go)）と `cmd.ExpandedCmd`（[`:191`](../../../internal/runner/base/executor/executor.go)、`:198`、`:247`、`:284`）を書き分けるため、guard はファイル・関数だけでなく引数式まで照合する必要がある（設計 §7.3）。
- キー `"name"` は複数用途を持つ。`logschema.go` の `name`（[`internal/common/logschema.go:120`](../../../internal/common/logschema.go)、`:164`）と group 名（[`internal/runner/group_executor.go:149`](../../../internal/runner/group_executor.go) ほか）は宣言型にし、一時ファイル名（[`internal/safefileio/safe_file_linux.go:241`](../../../internal/safefileio/safe_file_linux.go)）は plain string のままとする（設計 §3.5）。
- `internal/runner/group_executor.go` の `executeSingleCommand` は、`LogTimeoutExceeded`（`:585`）・`[]any` の `"command"`（`:594`、`:617`）・`buildCommandDebugLogArgs`（`:612`）の 4 箇所で `cmd.Name()` を宣言する。`createCommandContext` は `LogUnlimitedExecution`（`:537`）と `slog.Debug`（`:543`）の 2 箇所である。guard の目録は同一 `(ファイル, 関数, 引数式)` の組を件数で持つため、これらの重複を集合ではなく件数で照合する。

**guard の既存基盤**

- `internal/testutil/identitymutationguard`（[`helpers.go`](../../../internal/testutil/identitymutationguard/helpers.go)）が、`ProductionGoFilesInRepo`（`:207`）・`ReadProductionSource`（`:241`）・`Options.Extra`（`:101`）・`ExtraTrackedFunc`（`:94`）・`RefsInSourceWithOptions`（`:342`）・`ValueRef`（`:111`）を提供する。修飾形の値参照は `ValueRef` として報告される（`:523-536`）が、空 `ImportPath` の非修飾エントリは呼び出しサイトだけに一致する（`:337-341`）。`CallSite`（`:70`）は `FuncName`（レシーバ修飾済みの囲み関数名）・`CallExpr`（引数を含まない `Func(...)` 表記）・`File`・`Pos` を持ち、引数式は返さない。目録の `(ファイル, 関数, 引数式, 件数)` を照合するには、guard 自身が production ソースを AST 走査して各 `NewIdentifier` 呼び出しの第 1 引数式を取り出す必要がある。
- guard テストの書き方は `internal/runner/resource/identity_mutation_guard_test.go`（パッケージ内の走査と失敗メッセージ）と `internal/logging/notification_contract_guard_test.go`（`ProductionGoFilesInRepo` を回し、非修飾と修飾の両方を数える）を踏襲できる。
- `make lint` は `--build-tags test`、`make test` は `-tags test` で走る（`Makefile:24`、`:480`）。`//go:build test` の guard と `internal/testutil` の補助はこの構成でコンパイルされる。

**更新が必要な既存テスト（設計 §7.4）**

- `tu.LogRecorder` に `Identifier` がそのまま捕捉されるため、次の比較・引数を `common.NewIdentifier(…)` に更新する。
  - `internal/runner/group_executor_test.go::TestCreateCommandContext_UnlimitedTimeout_SecurityLogging`（期待値 `"command"`。`:2473`、`:2486`）
  - `internal/runner/group_executor_test.go::TestExecuteGroup_TimeoutExceeded_SecurityLogging`（`:2598`）
  - `internal/runner/group_executor_test.go::TestExecuteGroup_MultipleCommands_TimeoutLogging`（`:2674`）
  - `internal/runner/group_executor_test.go::TestCommandDebugLogArgs_StdoutTruncation`（`buildCommandDebugLogArgs` の引数。`:2880`）
  - `internal/runner/group_executor_timeout_test.go::TestExecuteSingleCommand_TimeoutLogsTimeoutExceeded`（`Attrs["command"]`。`:94`）
  - `internal/runner/base/privilege/unix_privilege_test.go::TestWithPrivileges_ReportsNativeRootOutcome`（`:812`）と `::TestLogElevationOutcome`（`:869`）
  - `internal/runner/base/audit/logger_test.go::TestLogger_LogUserGroupExecution`（`"command_name"`。`:117`）
  - `internal/runner/runner_test.go::TestCommandResult_LogValue`（`:1993`）。`attr.Value.Kind()` を `KindString`／`KindInt64` だけで分岐するため（`:2063-2070`）、`name` が `KindLogValuer` になると map から落ちて失敗する。`KindLogValuer` と `Value.Any()` が `Identifier` であることを検証する行に更新する。
  - `internal/logging/security_test.go::TestSecurityLogger_LogMethods`（4 メソッドの呼び出し引数。`:31`、`:45`、`:59`、`:73`、`:86`）
  - `internal/common/notification_context_test.go` の `groupAttr`／`commandAttr` ヘルパー（`:19`、`:23`）と符号化期待値（`:38` 以降）
- `Value.String()` の比較（`internal/common/logschema_test.go:72`）と JSON 出力を解析するテスト（`internal/runner/base/executor/executor_logging_test.go`、`internal/runner/resource/audit_wiring_test.go:85` など）は、`Identifier.String()` と免除経路の正規化により変更を要さない。
- `TestRedactText_AlternativePriority`（[`internal/redaction/redactor_test.go:494`](../../../internal/redaction/redactor_test.go)）の `monkey="a b"` は plain string を `RedactText` に渡すテストであり、本タスク後もそのまま有効である。
- 設定境界の既存テスト `TestValidateIdentifiers`（[`internal/runner/config/validation_test.go:126`](../../../internal/runner/config/validation_test.go)）と `TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted`（[`cmd/runner/integration_pre_execution_error_test.go:189`](../../../cmd/runner/integration_pre_execution_error_test.go)）は変更しない（AC-10）。

**文書**

- `docs/dev/architecture_design/security-architecture.md`／`.ja.md` の redaction 層の説明は、中央集権 redaction 基盤・二層防御・値形式検出・キー名ベースの限界までを記すが、識別子の型宣言による免除には触れていない。同文書に `identifier`・`Identifier`・`rollback`・`ロールバック`・`識別子` は 0 件である（§7 注 5 の実行結果）。
- `docs/user/security-risk-assessment.md`／`.ja.md` の Limitations（`## 🔍 Additional Security Features` の `**Limitations**` 段落。`.md:299`、`.ja.md:295`）にも `exempt`・`免除` は 0 件である（同注 5）。
- 文書は日本語版を先に直してコミットし、英語版は `/mktrans` で反映する（設計 §8.1、CLAUDE.md の翻訳方針）。

**変更が不要なもの**

- `internal/redaction/sensitive_patterns.go` と `internal/redaction/value_detector.go` は変更しない。設計書が既存挙動の検証基準とした commit `88624849` から `26cf9564` まで両ファイルに差分は無く、本タスクでも触らない（AC-12）。
- `internal/logging/slack_handler.go`・`message_formatter.go`・`internal/common/logschema.go` のフィールド型は変更しない（設計 §3.1、§3.6）。

### 1.4 テストヘルパーの方針

- 新規のテストヘルパーファイルは追加しない。`identifier_guard_test.go` が使う構文木走査と引数式抽出の補助関数は同ファイル内に閉じる（利用者が 1 テストだけのため、`test_helpers.go` へ切り出さない。test_organization.md の分類 B は複数テストで共有する場合や非公開 API を使う場合の置き場である。既存の `internal/logging/notification_contract_guard_test.go` も走査ヘルパーを同じ `_test.go` 内に持つ）。
- 既存の `internal/testutil/identitymutationguard` を走査の土台として再利用する。ただし同ヘルパーは引数式を返さないため、引数式の抽出だけは guard 内の AST 走査で補う（同等の呼び出し検出を再実装しない）。
- テストの期待値を組み立てる `common.NewIdentifier(…)` は `_test.go` にだけ書く。`internal/testutil` など `//go:build test` を必須としない補助パッケージに置くと、guard が目録外の宣言として失敗させる。

## 2. 実装ステップ

各フェーズの完了条件は、明記がない限り `make fmt`・`make test`・`make lint` が通ることである（AC-18）。挙動に関するテストを追加・更新したフェーズでは、AC-19 の確認（検証対象を壊して失敗することを確かめる）を行い、その結果をコミットメッセージに記す。

### Phase 1: 識別子型の追加

**対象ファイル**: `internal/common/identifier.go`（新規）、`internal/common/identifier_test.go`（新規）

**作業内容**:

- [ ] 設計 §3.1 の型を実装する。フィールドは非公開の `name string` とし、`NewIdentifier(name string) Identifier`・`(Identifier).Name() string`・`(Identifier).String() string`・`(Identifier).LogValue() slog.Value` を値レシーバで定義する。`LogValue` は `slog.StringValue(i.name)` を返す。
- [ ] `var _ slog.LogValuer = Identifier{}` のコンパイル時ガードを置く（設計 §3.1）。
- [ ] `identifier_test.go` を追加する。`Name()`／`String()` が名前を返すこと、`LogValue()` の `Kind` が `slog.KindString` で値が名前であること、ゼロ値が空名として扱われること、`fmt.Stringer` を満たすことを検証する。
- [ ] AC-19 の確認: 値レシーバと各メソッドを一時的に壊し、単体テストが失敗することを確認してコミットメッセージに記す。

**完了条件**: `go test -tags test ./internal/common/` が通る。

### Phase 2: 免除経路の追加

**対象ファイル**: `internal/redaction/redactor.go`、`internal/redaction/redactor_test.go`

**作業内容**:

- [ ] `internal/common` を import し、`slog.Value` が宣言型かを判定する非公開ヘルパーを 1 つ追加する。`common.Identifier` と非 nil の `*common.Identifier` を認識して名前を返し、キー名・値の内容・値の長さは見ない。型付き nil の `*common.Identifier` は認識せず false を返す（設計 §3.2）。
- [ ] `Config.RedactLogAttribute`（`:301`）のキー名判定の後・`KindString`／`KindGroup` 分岐の前に免除を挿入し、`slog.Attr{Key: key, Value: slog.StringValue(name)}` を返す。
- [ ] `redactLogAttributeWithContext`（`:764`）のキー名判定の後・`switch value.Kind()` の前に同じ免除を挿入する。
- [ ] `processSlice`（`:1264`）の要素ループで、`element.(slog.LogValuer)` 型アサーション（`:1330`）より前にヘルパーで要素の型を見る。免除した要素は属性経路と同じく名前の string を `processedElements` へ append する。生の `Identifier` を append すると下流の JSON 描画が `[{}]` になるためである（設計 §3.2）。
- [ ] `redactor_test.go` に次を追加する（設計 §7.1、§7.3）。
  - `TestRedactingHandler_IdentifierExemptFromAllThreeLayers`: 3 層それぞれの一致形（key=value 置換は `backup --password=x`、値形式検出は `AKIAIOSFODNN7EXAMPLE`・`ghp_` + 36 文字・`github_pat_` + 30 文字、値まるごと判定は `monkey`・`rotate_api_key`・`keyboard`）を `Identifier` として載せ、書き換わらないこと。同じ文字列を plain string で載せた対照については、どの層が作用したかを区別できる形でアサートする（key=value 層の行は部分置換 `backup --password=[REDACTED]` を期待し、値形式検出・値まるごと判定の行は `IsSensitiveValue` が反応しない入力と placeholder 全体置換で層を切り分ける）。
  - `TestRedactLogAttribute_IdentifierExemptFromValueRedaction`: `Config.RedactLogAttribute` 側でも同じ対を検証する（公開実装 2 つの一致を固定する）。
  - `TestRedactingHandler_SameStringsStillRedactedAsFreeText`: AC-05 の入力集合（`--password=x`、`token=...`、`Bearer ...`、AWS/GitHub/Slack トークン形）を plain string として載せ、従来どおり redact されること。
  - `TestRedactingHandler_CommandKeyIdentifierVsCommandLine`: 同じキー `"command"` にコマンド名の `Identifier` と展開済みコマンド行の plain string（`--password=x` を含む）を並べ、前者だけが残ること。
  - `TestRedactingHandler_IdentifierKeyMaskTakesPrecedence`: 機密キー `password` の下に `Identifier("monkey")` を載せてもマスクされること（キー名判定が免除より先。fail-closed）。
  - `TestRedactingHandler_IdentifierNormalizedToString`: 免除経路の下流ハンドラが受け取る属性値の `Kind` が `slog.KindString` であること（AC-06 の正規化）。
  - `TestRedactingHandler_IdentifierSliceElementsExempt`: `[]common.Identifier` と `[]*common.Identifier` を `slog.Any` で載せ、JSON 出力に名前が string として現れ、`[{}]` にならないこと。
  - `TestRedactingHandler_TypedNilIdentifierFailsClosed`: `slog.Any("command", (*common.Identifier)(nil))` がパニックせず `RedactionFailurePlaceholder` になること。
- [ ] AC-19 の確認: 3 挿入点を 1 つずつ外し、対応するテストが失敗することを確認する。あわせて plain string 側の `RedactText` 呼び出しを外して `TestRedactingHandler_SameStringsStillRedactedAsFreeText` が失敗すること（AC-05）、`DefaultKeyValuePatterns` を 1 件削除して `TestDefaultKeyValuePatterns_AreValid`・`TestKeyBoundaryGroup_Classification` が失敗することを確認する（AC-12）。結果をコミットメッセージに記す。

**完了条件**: 追加したテストが通り、既存のパターン集合テスト（`sensitive_patterns_test.go`、`value_detector_test.go`）に変更が無い。

### Phase 3: `notification_context` の符号化と復元

**対象ファイル**: `internal/common/notification_context.go`、`internal/common/notification_context_test.go`、`internal/logging/slack_handler_test.go`

**作業内容**:

- [ ] `NotificationContext.LogValue`（`:78`）の `group` を常に、`command` を空でないときだけ、`slog.Any` + `NewIdentifier` で符号化する。`scope` は string のままとする（設計 §3.3）。
- [ ] `decodeNotificationContextParts`（`:136`）に、`group`／`command` の下位値として `KindString` に加え `value.Any()` が `Identifier` である値を受け、`Name()` を名前として読む経路を追加する。`scope` は `KindString` だけを受け、`Identifier` 以外の `LogValuer` や非 string は従来どおり拒否する。汎用の `Resolve` は使わない（設計 §3.3）。
- [ ] `notification_context_test.go` の `groupAttr`／`commandAttr` ヘルパーを `NewIdentifier` で構築する形に更新し、`TestNotificationContext_LogValueEncoding` の期待値を追随させる。
- [ ] `TestDecodeNotificationContext_AcceptsIdentifierAndString` を追加する。生の宣言型（`LogValue` の出力）と、免除経路が正規化した string の両形式を復号でき、同じ `NotificationContext` になることを検証する。`TestDecodeNotificationContext_Validity` の拒否表は変更しない（AC-11）。
- [ ] `TestDecodeNotificationContext_RejectsUnsupportedForms` を追加する。`scope` に `Identifier` を載せた値、`group`／`command` に `Identifier` 以外の `LogValuer` を載せた値、`group` に非 nil の `*Identifier` を載せた値が、いずれも `ErrInvalidNotificationContext` で拒否されることを検証する（`Resolve` を使った受け入れに緩めない。設計 §3.3、AC-11）。
- [ ] `internal/logging/slack_handler_test.go` に `TestSlackHandler_IdentifierScopeSurvivesRedaction` を追加する。`TestSlackHandler_WithRedactingHandler`（`:1003`）と同じく `RedactingHandler` → `SlackHandler`（モックサーバー、同期または `Flush`）で配線し、`common.GroupScope("monkey")` と `common.CommandScope("monkey", "rotate_api_key")` の通知で Scope に `monkey`・`rotate_api_key` が現れ、`[REDACTED]` や `(scope: invalid)` にならないことを検証する（AC-01、AC-02）。
- [ ] 既存の `TestSlackHandler_InvalidNotificationContext` がそのまま通ることを確認する（AC-11）。
- [ ] AC-19 の確認: `LogValue` を `slog.String` に戻し Scope テストと復号テストが失敗すること、`decodeNotificationContextParts` の判定を汎用の `Resolve` に置き換えて `TestDecodeNotificationContext_RejectsUnsupportedForms` が失敗することを確認してコミットメッセージに記す。

**完了条件**: `notification_context_test.go` と追加した Slack テストが通る。

### Phase 4: 宣言サイトの置き換えと guard

**対象ファイル**: 設計 §3.4 の宣言サイトのうち Phase 3 で更新済みの `internal/common/notification_context.go` を除く 11 ファイル、`internal/logging/security.go`、§1.3「更新が必要な既存テスト」に挙げたテスト、`internal/common/identifier_guard_test.go`（新規）、`internal/runner/integration_command_results_test.go`、`internal/runner/base/audit/logger_test.go`

**作業内容**:

- [ ] `internal/common/logschema.go` の `LogValue` を宣言型で符号化する（`CommandResult.LogValue` は `slog.Any(LogFieldName, NewIdentifier(c.Name))`、`CommandResults.LogValue` の各 `cmd_%d` の `name` も同様。同ファイルは `package common` なので修飾子を付けない。設計 §3.4、§3.6）。
- [ ] 設計 §3.4 の目録に従い、group 名・コマンド名を属性値として書く production の各行を `slog.Any(key, common.NewIdentifier(name))` または引数 `common.NewIdentifier(name)` に置き換える。対象は `internal/runner/group_executor.go`、`internal/runner/runner.go`、`internal/runner/config/expansion.go`、`internal/runner/base/executor/executor.go`、`internal/runner/base/executor/tempdir_manager.go`、`internal/runner/base/privilege/unix.go`、`internal/runner/base/audit/logger.go`、`internal/runner/resource/normal_manager.go`、`internal/runner/resource/dryrun_manager.go`、`internal/verification/manager.go`。`internal/runner/base/executor` と `internal/runner/base/privilege` には `internal/common` の import を追加する。
- [ ] 設計 §3.5 のリストは plain string のままにする。とくに `executeWithUserGroup` の `cmd.ExpandedCmd`（`:191`、`:198`、`:247`、`:284`）と `cmd.RunAsGroup()`（OS グループ名。`:215`、`:247`、`:254`、`:286`）は宣言型にしない。`normal_manager.go:143` はキー `command_path` のまま値だけを宣言する。
- [ ] `internal/logging/security.go` の 4 メソッドの `cmdName` を `common.Identifier` にし、`buildCommandDebugLogArgs`（`internal/runner/group_executor.go:553`）も `cmdName common.Identifier` を受ける。宣言は呼び出し元（`:537`、`:585`、`:612`）で行う。
- [ ] §1.3「更新が必要な既存テスト」の比較・引数を `common.NewIdentifier(…)` に更新する。
- [ ] `internal/common/logschema_test.go` の `TestCommandResults_LogValue` に、`name` の下位値が `KindLogValuer` で `Identifier` を保持することを固定する行を足す（`Value.String()` の既存アサーションはそのまま残す）。
- [ ] `internal/runner/integration_command_results_test.go::TestCommandResults_E2E_Integration` を拡張し、値ベース redaction が掛かる名前（`rotate_api_key` など）を持つ `CommandResult` が `RedactingHandler` → JSON ハンドラの出力で元の文字列のまま現れることを検証する（AC-06、AC-09）。
- [ ] `internal/runner/base/audit/logger_test.go` に `TestLogUserGroupExecution_CommandNameSurvivesRedaction` を追加する。`RedactingHandler` でラップした JSON ロガーに、値ベース redaction が掛かる名前の監査レコードを通し、`command_name` が元の文字列で残ることを検証する（AC-09）。
- [ ] `internal/common/identifier_guard_test.go`（`//go:build test`）を追加する。設計 §7.3 のとおり、`ProductionGoFilesInRepo` の全 production ファイルを `identitymutationguard.RefsInSourceWithOptions` に `Options.Extra`（修飾形 `ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/common"` + `FuncName: "NewIdentifier"`、非修飾形 `ImportPath: ""` + `FuncName: "NewIdentifier"`）で走査し、`NewIdentifier` の呼び出しを設計 §3.4 の目録（ファイル・関数・引数式・件数）と双方向に照合する。目録外の宣言と目録にある宣言の欠落の双方を失敗させる。
  - `identitymutationguard.CallSite` は引数式を返さないため、guard 自身が各 production ファイルを AST 走査し、各 `NewIdentifier` 呼び出しの第 1 引数式をソースから復元して、囲む関数名（`identitymutationguard` と同じレシーバ修飾形。例: `(*DefaultGroupExecutor).executeSingleCommand`）と組にして目録と照合する。抽出器は合成ソースのテーブル（別名 import、括弧付き呼び出し、`cmd.Name()` と `cmd.ExpandedCmd` の対照）で検証し、引数式の比較がファイル・関数・件数だけに劣化しないようにする。
  - `NewIdentifier` の値参照（修飾形は `ValueRef`、非修飾形は自前の AST 走査）が 0 件であることも要求する。
  - guard を書く前に、`rg` で production の宣言を洗い出して目録の件数を再確認し、目録と実装が食い違わない状態にしてから固定する。
- [ ] AC-19 の確認: 次の 4 つを順に試し、guard が失敗することを確認してコミットメッセージに記す。(1) 目録エントリを 1 件削除する、(2) 目録に無い宣言を 1 件追加する、(3) `makeID := common.NewIdentifier` 相当の束縛を加える、(4) 目録にあるサイトの引数を `cmd.Name()` から `cmd.ExpandedCmd` に差し替える（引数式の照合が働くことの確認。設計 §5.1 T2）。
- [ ] AC-10 の確認: `go test -tags test -run 'TestValidateIdentifiers|TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted' ./internal/runner/config/ ./cmd/runner/` が通ることを確認する。

**完了条件**: `make test` が通り、`identifier_guard_test.go` と AC-06〜AC-09 のテストが緑である。

### Phase 5: 文書の更新と翻訳

**対象ファイル**: `docs/dev/architecture_design/security-architecture.ja.md`、`docs/user/security-risk-assessment.ja.md`、および対応する英語版

**作業内容**:

- [ ] `security-architecture.ja.md` の redaction 層の説明（「セキュアログと機密データ保護」節。第 2 層の説明の付近）へ、宣言型の識別子を値ベース redaction から免除することを追記する。免除を止める専用の実行時スイッチが無いことと、漏洩が疑われる場合の rollback 手順（該当する宣言サイトを plain string へ戻すコミットで免除を解除する）も記す（設計 §5.2、AC-15）。
- [ ] `security-risk-assessment.ja.md` の Limitations（`:295` の `**限界**` 段落の末尾）へ、識別子を redact しない帰結（設定の名前に機密を書いた場合は通知・ログにそのまま出る）を追記する（AC-14、AC-16）。error 文字列に埋め込まれた名前が免除されない残余リスク（設計 §5.2）は、設計文書側の記載を確認し、利用者向け文書へは持ち込まない。
- [ ] 追記した 2 文を設計 §5.2 の記述と 1 文ずつ対照し、記述が実装（免除の範囲は設計 §3.2 の 3 挿入点、rollback は免除経路が `common.Identifier` の型だけに依存すること）と矛盾しないことを確認する。対照結果（突き合わせた文と設計の節）を Phase 5 のコミットメッセージに記す（AC-14、AC-15、AC-16）。
- [ ] 日本語版 2 ファイルをコミットする。
- [ ] `/mktrans` で `security-architecture.md`・`security-risk-assessment.md` へ翻訳を反映する。日英を直接両方編集しない。
- [ ] 翻訳後、日英の該当節を対照し、記述の過不足・用語の不一致が無いことを確認する。対照結果を翻訳コミットのメッセージに記す（`make verify-docs` は `--docs=docs/user` のみを対象とし、見出しの訳文の対応を合否判定に用いないため、日英の構造・内容の照合は目視で行い、その記録をコミットメッセージに残す。`make verify-docs` を本タスクの判定には使わない）。
- [ ] AC-13 の確認: `rg -n -F -e 'record.Message' -e 'IsSensitiveValue' docs/tasks/0173_identifier_redaction_exemption/02_architecture.md` を実行し、残余リスク（error 文字列に連結された識別子の免除対象外と、error 属性が全文 `[REDACTED]` になりうる一方 `record.Message` には値まるごと判定が掛からないこと）が §5.2 に記載されていることを確認する（AC-13）。
- [ ] AC-17 の確認: 本タスクが Task 0172 §3.5 の残余リスクを置き換える旨が、本タスクの文書から参照できることを確認する。

**完了条件**: §7 の AC-13〜AC-17 の検証が緑である（AC-14〜AC-16 は static 検索が一致し、manual の対照記録がコミットメッセージにある）。日本語版と英語版が別コミットに分かれている。

### Phase 6: 全体検証

**対象ファイル**: なし（検証のみ）

**作業内容**:

- [ ] `make fmt`・`make test`・`make lint` を実行し、すべて通ることを確認する（AC-18）。
- [ ] 各フェーズの PR で CI（`.github/workflows/ci.yml`）が緑であることを確認し、各フェーズのローカル実行の `make test`・`make lint` の終了コード 0 を PR 本文またはコミットメッセージに記録する（AC-18）。
- [ ] §7 の全 AC 行を実行し、緑であることを確認する（AC-14〜AC-16 の用語検索は、Phase 5 のコミットで得た出力をそのまま記録する）。
- [ ] 各フェーズのコミットメッセージに AC-19 の確認（壊した対象と失敗したテスト）が記録されていることを `git log --format=%B` で確認する（AC-19）。
- [ ] `git diff <Phase 1 着手前のコミット>..HEAD -- internal/redaction/sensitive_patterns.go internal/redaction/value_detector.go` が空であることを確認する（AC-12）。

**完了条件**: `make test`・`make lint` が通り、§7 の全行が緑である。

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | フェーズ | 成果物 | 緑になる AC |
|---|---|---|---|
| M1: 識別子型 | Phase 1 | `common.Identifier` と単体テスト | — |
| M2: 免除経路 | Phase 2 | `internal/redaction` の免除と対照テスト | AC-03〜AC-05、AC-08（一部）、AC-12 |
| M3: 通知コンテキスト | Phase 3 | 宣言型での符号化・復号と Slack Scope テスト | AC-01、AC-02、AC-06（一部）、AC-11 |
| M4: 宣言サイトと guard | Phase 4 | 全宣言サイトの置換、guard、既存テスト更新 | AC-06〜AC-10 |
| M5: 文書 | Phase 5 | セキュリティ 2 文書の日英更新 | AC-13〜AC-17 |
| M6: 全体検証 | Phase 6 | green gate と AC 全行の確認記録 | AC-18、AC-19 |

### 3.2 フェーズ順の根拠

Phase 1 を最初に置くのは、宣言型が Phase 2〜4 すべての前提だからである。Phase 2 を Phase 3 より先に置くのは、免除が既存の `slog.String` 属性の扱いを変えないことを、通知コンテキストの符号化変更と分離して確認できるためである。Phase 3 で通知コンテキストが宣言型になると、Phase 2 の免除経路を通って Slack Scope の表示が変わるため、AC-01・AC-02・AC-11 の実行テストは Phase 3 に置く。Phase 4 は宣言サイトの数が多く、判定が固まってから機械的に進める。Phase 5 の文書は実装の確定後に書く。Phase 6 は全フェーズの green gate を再確認する。

## 4. テスト戦略

### 4.1 単体テスト

- `internal/common/identifier_test.go`: 型の基本契約（設計 §7.1）。
- `internal/redaction/redactor_test.go`: 3 層の免除と対照、コマンド行の維持、キー名マスクの優先、string 正規化、スライス要素、型付き nil の fail-closed（設計 §7.1、§7.3）。
- `internal/common/notification_context_test.go`: 符号化が宣言型であることと、生の宣言型・正規化後の string の両方を復号できること（設計 §7.1）。
- `internal/common/logschema_test.go`: `CommandResult`／`CommandResults` の `name` が宣言型で符号化されること（設計 §7.1）。
- 既存のパターン集合テスト（`internal/redaction/sensitive_patterns_test.go`、`value_detector_test.go`、`redactor_test.go::TestKeyBoundaryGroup_Classification`・`::TestDefaultKeyValuePatterns_AreValid`）は変更せず、AC-12 の検出挙動の基準として使う。

### 4.2 統合テスト

- `internal/runner/integration_command_results_test.go`: `RedactingHandler` → JSON ハンドラで、値ベース redaction が掛かる名前を持つコマンド結果の `name` が元の文字列で残ること（AC-06、AC-09）。
- `internal/logging/slack_handler_test.go`: `RedactingHandler` → Slack ハンドラで、Scope に `monkey`・`rotate_api_key` が表示されること（AC-01、AC-02、AC-11）。
- `internal/runner/base/audit/logger_test.go`: 監査ログの `command_name` が redaction 後も残ること（AC-09）。
- 監査・実行系の既存テストのうち、`Identifier` が捕捉されるものは §1.3 の一覧に従って更新する。

### 4.3 セキュリティテスト

- 同じ入力集合を免除ケースと対照ケースの両方に使い、免除が plain string へ漏れていないことを固定する（設計 §7.1 の表）。
- 展開済みコマンド行・引数・環境変数値・message・error の redaction が変わらないこと（AC-05）。
- guard テストが宣言サイトの目録と双方向に一致し、値参照（エイリアス）を拒否すること（AC-07、AC-08。設計 §7.3）。

### 4.4 後方互換

- `Value.String()` を比較するテストと JSON 出力を解析するテストは、`String` の実装と免除経路の正規化により変更を要さない。実装時に全 `*_test.go` を検索し、宣言型を比較する期待値を取りこぼさない（設計 §7.4）。
- 設定境界（`TestValidateIdentifiers`、`TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted`）とパターン集合のテストは変更しない。

## 5. リスク管理

### 5.1 技術リスク

| リスク | 影響 | 対策 |
|---|---|---|
| コマンド行を誤って `Identifier` と宣言する | 値ベース redaction が掛からず `token=…` が露出する | 目録を引数式・件数まで持つ guard を Phase 4 で追加し、双方向に照合する。`executeWithUserGroup` のような混在関数も引数式で区別する（設計 §5.1 T2） |
| 免除経路の挿入点が 1 つ漏れる | 特定の値の種別（スライス要素など）だけ免除が効かず、`[]Identifier` が `[{}]` 描画になる | 3 挿入点を同じヘルパーに集約し、`TestRedactingHandler_IdentifierSliceElementsExempt` と `TestRedactingHandler_IdentifierNormalizedToString` で固定する（設計 §3.2） |
| 型付き nil の `*Identifier` がパニックする | ログ呼び出しがプロセスを落とす | ヘルパーが nil を免除せず、既存のパニック回復が `RedactionFailurePlaceholder` を代入することを `TestRedactingHandler_TypedNilIdentifierFailsClosed` で固定する |
| 既存テストの更新漏れ | `make test` が落ちる、または期待値が緩む | §1.3 の一覧を Phase 4 で消化し、実装時に全 `*_test.go` を検索し直す |
| 免除が一般則に広がる | 任意の `LogValuer` が redaction を迂回する | 免除は `common.Identifier` と非 nil ポインタの具象型だけを認識し、`KindLogValuer` 一般を免除しない（設計 §3.1、§3.2） |
| 文書と実装の記述が食い違う | 免除範囲や rollback の説明が実態とずれる | Phase 5 で追記内容を設計 §3.2・§5.2 とテストに突き合わせ、日英を対照する |

### 5.2 スケジュールリスク

| リスク | 対策 |
|---|---|
| Phase 4 の宣言サイトが多く、レビューが大きくなる | フェーズ単位でコミットを分け、Phase 4 も guard と宣言サイトでレビュー可能な単位に切る。mkplan2 で PR 境界を設計する |
| guard の目録作成に時間がかかる | 目録は設計 §3.4 を出発点にし、guard 作成前に `rg` で件数を再確認する。行番号は合否判定に使わない |
| 文書翻訳が後ろへずれる | 日本語版を先にコミットし、`/mktrans` を Phase 5 内で実行する |

## 6. 実装チェックリスト

### Phase 1

- [ ] `internal/common/identifier.go` を追加した
- [ ] `internal/common/identifier_test.go` を追加した
- [ ] `make fmt`・`make test`・`make lint` が通る

### Phase 2

- [ ] 判定ヘルパーを追加し、3 挿入点を実装した
- [ ] 免除・対照・コマンド行維持・キー名優先・正規化・スライス・型付き nil のテストを追加した
- [ ] AC-19 の確認結果をコミットメッセージに記した

### Phase 3

- [ ] `NotificationContext.LogValue` を宣言型で符号化した
- [ ] `decodeNotificationContextParts` が `Identifier` と string の両方を受けるようにした
- [ ] `notification_context_test.go` と Slack Scope テストを更新・追加した
- [ ] 否定ケース（`scope` の `Identifier`、他の `LogValuer`、`*Identifier`）の復号テストを追加した
- [ ] AC-19 の確認結果をコミットメッセージに記した

### Phase 4

- [ ] 設計 §3.4 の全宣言サイトを置き換えた
- [ ] 設計 §3.5 のサイトを plain string のままにした
- [ ] `SecurityLogger` 4 メソッドと `buildCommandDebugLogArgs` の引数型を変えた
- [ ] `CommandResult`／`CommandResults` の符号化を変えた
- [ ] §1.3 の既存テスト更新を消化した
- [ ] `identifier_guard_test.go` を追加し、目録・引数式・エイリアス・非修飾値参照を検証した
- [ ] AC-06〜AC-10 のテストを追加・更新した
- [ ] AC-19 の確認結果をコミットメッセージに記した

### Phase 5

- [ ] `security-architecture.ja.md` を更新した
- [ ] `security-risk-assessment.ja.md` を更新した
- [ ] 日本語版をコミットした
- [ ] `/mktrans` で英語版を更新した
- [ ] 追記文を設計 §5.2 と対照した結果をコミットメッセージに記した
- [ ] 日英を対照し、その結果を翻訳コミットのメッセージに記した

### Phase 6

- [ ] `make test`・`make lint` が通る
- [ ] 各フェーズの PR の CI が緑で、ローカル実行の終了コードが記録されている
- [ ] §7 の全行を実行した
- [ ] AC-19 の記録を確認した
- [ ] パターン集合ファイルの差分が空であることを確認した

## 7. 受け入れ基準の検証

各行の「種別」は `test`（実行可能で、挙動を壊すと失敗する）、`static`（guard テスト・`make` target・コミット済みスクリプト・確認済みの検索）、`manual`（PR やデプロイでの観察）を表す。`test` の行はテスト関数名まで特定する。`static` の検索コマンドは §7 の注 5 で実行結果と commit を記録した。

| AC | 種別 | 検証方法 |
|---|---|---|
| AC-01 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_IdentifierScopeSurvivesRedaction`。`RedactingHandler` → `SlackHandler` の Scope に `monkey` が現れ、`[REDACTED]`・`(scope: invalid)` にならないこと。符号化側は `internal/redaction/redactor_test.go::TestRedactingHandler_IdentifierExemptFromAllThreeLayers` が同じ文字列を固定する |
| AC-02 | test | 同テストの `CommandScope("monkey", "rotate_api_key")` の行。Scope に `rotate_api_key` が現れること |
| AC-03 | test | `internal/redaction/redactor_test.go::TestRedactingHandler_IdentifierExemptFromAllThreeLayers` の値形式検出行（`AKIAIOSFODNN7EXAMPLE`、`ghp_` + 36 文字、`github_pat_` + 30 文字）。`Config.RedactLogAttribute` 側は `::TestRedactLogAttribute_IdentifierExemptFromValueRedaction` |
| AC-04 | test | 同上。3 層それぞれの免除行と、同じ文字列の plain string 対照行を同じテスト内に持つ |
| AC-05 | test | `internal/redaction/redactor_test.go::TestRedactingHandler_SameStringsStillRedactedAsFreeText`。`--password=x`、`token=...`、`Bearer ...`、AWS/GitHub/Slack トークン形が plain string では redact されること。既存のパターン集合テストも `make test` で走る |
| AC-06 | test | `internal/redaction/redactor_test.go::TestRedactingHandler_IdentifierNormalizedToString`（下流へ渡る値が `KindString`）、`internal/runner/integration_command_results_test.go::TestCommandResults_E2E_Integration`（JSON 出力で `name` が元の文字列）、`internal/logging/slack_handler_test.go::TestSlackHandler_IdentifierScopeSurvivesRedaction`（Slack Scope）。`message_formatter` は読み取りコードを変更しないため、既存テストがそのまま通ることを `make test` で確認する |
| AC-07 | test | `internal/common/identifier_guard_test.go::TestNewIdentifierCallSitesMatchCatalog`。設計 §3.4 の目録（ファイル・関数・引数式・件数）と production の `NewIdentifier` 呼び出しを双方向に照合し、目録外の宣言と目録の欠落の双方を失敗させる |
| AC-08 | test | `internal/redaction/redactor_test.go::TestRedactingHandler_CommandKeyIdentifierVsCommandLine`（同じキー `command` で、宣言型のコマンド名は残り、plain string のコマンド行は redact される）と `::TestRedactingHandler_IdentifierKeyMaskTakesPrecedence`（キー名マスクが免除より先）。guard は `cmd.ExpandedCmd` を包む誤宣言を目録外として拒否する |
| AC-09 | test | `internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_CommandNameSurvivesRedaction`（監査ログの `command_name`）と `internal/runner/integration_command_results_test.go::TestCommandResults_E2E_Integration`（グループ集計のコマンド一覧の `name`） |
| AC-10 | test | `internal/runner/config/validation_test.go::TestValidateIdentifiers` と `cmd/runner/integration_pre_execution_error_test.go::TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted` が無変更で通ること（`test` タグを付ける `make test` に含まれる） |
| AC-11 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_InvalidNotificationContext`（`(scope: invalid)` の判定表）と `internal/common/notification_context_test.go::TestDecodeNotificationContext_Validity`・`::TestDecodeNotificationContext_AcceptsIdentifierAndString`（受ける型の範囲） |
| AC-12 | test + static | test: `internal/redaction/sensitive_patterns_test.go::TestSensitivePatterns_CombinedPatterns`、`internal/redaction/value_detector_test.go::TestValueDetector_Mask_PositiveCases`、`internal/redaction/redactor_test.go::TestKeyBoundaryGroup_Classification`（`DefaultKeyValuePatterns` を全件 range する。`:626`）と `::TestDefaultKeyValuePatterns_AreValid`。static: Phase 6 で `internal/redaction/sensitive_patterns.go` と `value_detector.go` の設計基準からの差分が空であること（注 5 の 4 番目を Phase 6 の base で再実行する）。あわせて Phase 2 の AC-19 確認として `DefaultKeyValuePatterns` を 1 件削除し、上記テストが失敗することを確認する |
| AC-13 | static | 残余リスクが設計文書に記載されていること。`rg -n -F -e 'record.Message' -e 'IsSensitiveValue' docs/tasks/0173_identifier_redaction_exemption/02_architecture.md` が §5.2（`:533`）に一致することを Phase 5 で再確認する。本書作成時に同コマンドを commit `26cf9564` で実行し、`:28`・`:106`・`:112`・`:120`・`:533`・`:562`・`:742` の一致を確認済み（うち `:533` が AC-13 の記載） |
| AC-14 | static + manual | static: `docs/user/security-risk-assessment.ja.md` に `免除` が、`docs/user/security-risk-assessment.md` に `exempt` が現れること（注 5 の 2 番目・3 番目の検索を Phase 5 完了後に再実行する）。語の出現は存在確認であり内容の保証ではないため、Phase 5 で追記文を設計 §5.2 と 1 文ずつ対照し、その結果をコミットメッセージに記録する（manual の記録が内容の検証、`rg` は見落とし防止の補助） |
| AC-15 | static + manual | static: `docs/dev/architecture_design/security-architecture.ja.md` に `識別子` と `ロールバック` が、`docs/dev/architecture_design/security-architecture.md` に `identifier` と `rollback` が現れること（注 5 の 5 番目を Phase 5 完了後に再実行する）。manual: 免除範囲を設計 §3.2、rollback 手順を設計 §5.2 と対照し、日英の対応を Phase 5 のコミットメッセージに記録する。`make verify-docs` は `--docs=docs/user` のみを対象とし、見出しの訳文の対応を合否判定に用いないため、本 AC の判定には使わない |
| AC-16 | static + manual | AC-14 と同じ検索を Phase 5 完了後に再実行し、日英両方に記述があること。manual: 日英の記述の対応を Phase 5 のコミットメッセージに記録する（AC-15 と同じ理由で `make verify-docs` は使わない） |
| AC-17 | static | `rg -n -F '0172' docs/tasks/0173_identifier_redaction_exemption/01_requirements.md docs/tasks/0173_identifier_redaction_exemption/02_architecture.md docs/tasks/0173_identifier_redaction_exemption/03_implementation_plan.md` が一致し、0172 の残余リスクを置き換える記述（設計 §5.2、§5.4）を参照できること |
| AC-18 | static | 各フェーズの完了時に `make test`・`make lint` を実行し、各コミットで終了コード 0 であることを記録する。記録先は各フェーズの PR（`.github/workflows/ci.yml` が `pull_request` で `make test`・`make lint` を実行する）と、PR 本文またはコミットメッセージに書くローカル実行の結果である。Phase 6 のタスクで全フェーズ分を確認する |
| AC-19 | static | 各フェーズのコミットメッセージ（`git log --format=%B`）に、壊した対象と失敗したテストの記述があること。Phase 4 では 4 種の操作（目録エントリの削除・追加、`NewIdentifier` の値束縛、引数式を `cmd.ExpandedCmd` へ差し替え）をそれぞれ記録する。Phase 6 のタスクで全フェーズ分を確認する |

**注 1: AC-07 の guard が固定する目録**

目録は設計 §3.4 の表を出発点とする。guard は行番号ではなく `(ファイル, 関数, 引数式, 件数)` で照合し、同じ組が複数回現れる宣言（例: `ExecuteGroup` の `groupSpec.Name` 3 件、`executeSingleCommand` の `cmd.Name()` 4 件）を件数で区別する。guard 作成前に `rg` で production の宣言を洗い出し、件数を確定してから目録を固定する。

**注 2: AC-19 の確認方法**

テストを追加・更新した各フェーズで、検証対象の実装（免除の挿入点、`LogValue` の符号化、guard の目録、宣言サイトの型）を一時的に元へ戻し、対応するテストが失敗することを確認する。確認した「壊した対象」と「失敗したテスト」をコミットメッセージに記す。設計・計画文書に「失敗するはず」と書くだけでは確認にならない。

**注 3: テストの層の切り分け**

免除のテストは「宣言型だけが免除される」ことを示すため、同じ入力集合を免除ケースと plain string 対照ケースの両方に使い、対照側が redact されることも同じテスト内でアサートする。対照側の期待値は、どの層が作用したかを区別できる形にする。値形式検出（`AKIAIOSFODNN7EXAMPLE` 等）は `IsSensitiveValue` の語を含まず、値まるごと判定（`monkey`・`keyboard`）は key=value 形でも値形式でもないため層が一意になる。key=value 層の `backup --password=x` だけは値まるごと判定も `password` に反応しうるので、部分置換 `backup --password=[REDACTED]` を期待して key=value 層の作用を識別する。免除側は宣言型がその 3 層のどれにも掛からないこと（文字列が完全に一致すること）を全行でアサートする。

**注 4: 実行されていない検証を緑と数えない**

`static` の行は Phase 5・Phase 6 の該当タスクで実際に実行する。§7 の検索は Markdown の表セル内のパイプを避け、`-F` と `-e` を並べる形で書いてある。`test` の行は `-tags test` と `-race` を付ける `make test` で実行される。`cmd/runner` の E2E は `test` タグでビルドされ、`make test` に含まれる。

**注 5: 本書作成時に実行した検索（commit `26cf9564`）**

いずれも Phase 実装前の状態で、「現時点では 0 件」であることを確認した。Phase 5・Phase 6 では同じコマンドを再実行し、期待する一致に変わることをもって AC を満たす。

1. `rg -n -F -e 'exempt' -e '免除' docs/user/security-risk-assessment.md docs/user/security-risk-assessment.ja.md` → 0 件（終了コード 1）。
2. `rg -n -F 'exempt' docs/user/security-risk-assessment.md` → 0 件。Phase 5 後に 1 件以上を期待する。
3. `rg -n -F '免除' docs/user/security-risk-assessment.ja.md` → 0 件。Phase 5 後に 1 件以上を期待する。
4. `git diff --stat 88624849..HEAD -- internal/redaction/sensitive_patterns.go internal/redaction/value_detector.go` → 差分なし（終了コード 0）。Phase 6 では Phase 1 着手前のコミットを base にして同じ結果を確認する。
5. `rg -n -F -e 'identifier' -e 'Identifier' -e 'rollback' -e 'ロールバック' -e '識別子' docs/dev/architecture_design/security-architecture.md docs/dev/architecture_design/security-architecture.ja.md` → 0 件（終了コード 1）。Phase 5 後に `Identifier`（英）と `識別子`（日）、`rollback`（英）と `ロールバック`（日）が現れることを期待する。
6. `rg -n -F 'NewIdentifier' -g '*.go' internal cmd` → 0 件（終了コード 1）。Phase 4 完了後は `internal/common/identifier.go` の定義と呼び出しサイトに一致する。目録との一致は AC-07 の guard が判定する。

## 8. 横断検索チェックリスト

`make lint`・`make test` では検出できない項目だけを列挙する。AC 検証表と重複する確認は同表に置き、ここには置かない。

- [ ] 日英のセキュリティ文書が同じ挙動（識別子の免除範囲、スイッチが無いこと、rollback 手順、名前に機密を書いた場合の帰結）を記述しているか、Phase 5 で対照した。翻訳は `/mktrans` を経由し、日英を同時に手編集していない。
- [ ] `docs/translation_glossary.md` に `識別子`・`免除` の見出し語を追加すべきかを Phase 5 で判断し、追加しない場合はその理由をコミットメッセージに記した（文書で新しく使う用語のため）。
- [ ] `security-architecture.ja.md` の `Identifier` の綴りが、他の文書（設計 §3.1 の型名）と一致している。

## 9. 成功基準

### 9.1 機能の完成度

- 宣言された識別子（通知の Scope、監査ログの `command_name`、コマンド結果の `name`）が、値ベース redaction で書き換わらず元の文字列で表示される。
- 同じキー `"command"` に載るコマンド行は redact され続け、キー名マスクは免除より優先される。
- 下流ハンドラ（JSON・text・Slack）へ渡る識別子は string であり、読み取りコードは変更されていない。

### 9.2 品質

- `make fmt`・`make test`・`make lint` が通る。
- `identifier_guard_test.go` が production の宣言サイトを目録と双方向に照合し、エイリアスと非修飾の値参照を拒否する。
- 既存テストの更新は §1.3 の一覧を起点に、実装時に全 `*_test.go` を検索して洗い出した範囲に閉じており、無関係なテストを緩めていない。
- 各テストが、対象の挙動を壊すと失敗することを確認済みである。

### 9.3 セキュリティ

- `internal/redaction/sensitive_patterns.go` と `value_detector.go` に差分が無い。
- 自由文の redaction（コマンド行・引数・環境変数値・stdout・stderr・message・error）が本タスクの前後で変わらない。
- 免除は宣言型の具象型だけに作用し、任意の `slog.LogValuer` を迂回させない。

### 9.4 文書

- `security-architecture.{ja,}.md` に識別子免除・スイッチ無し・rollback 手順が、日英で記述されている。
- `security-risk-assessment.{ja,}.md` の Limitations に、名前に機密を書いた場合の帰結が日英で記述されている。
- Task 0172 の残余リスクを置き換えることが本タスクの文書から参照できる。

## 10. 次のステップ

- 本書のレビューと承認（status を `approved` にする）。
- 承認後、Phase 1 から実装に着手する。
- 実装中に設計と実装が食い違った場合は、設計側を改訂してから実装する（requirements_process.md「Editing an approved document」）。
- 実装完了後、Phase 6 の検証結果（AC-18・AC-19 の記録を含む）を本書へ追記する。
- 設計 §9 が挙げる将来の拡張（`IsSensitiveValue` の語境界化、`Identifier` の leaf パッケージ切り出し、新しい識別子の宣言）は、必要になった時点で別タスクとして起票する。
