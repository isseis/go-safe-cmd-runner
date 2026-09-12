# 要件定義書: 識別子の型宣言と値ベース redaction からの免除

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-12 |
| Review date | `-` |
| Reviewer | `-` |
| Comments | `-` |

## 関連 Issue

- なし（Task 0172 の `02_architecture.md` §3.5 で残余リスクとして受容した事項から派生）

## 背景

### 現行 redaction の構造

`RedactingHandler` は Slack・JSON・text を含む全出力先を対象とし（[`bootstrap.AddSlackHandlers`](../../../internal/runner/bootstrap/logger.go)）、文字列属性に対して次の 3 層を順に適用する。

1. **本文中の key=value 置換** — `RedactText` が [`DefaultKeyValuePatterns`](../../../internal/redaction/sensitive_patterns.go) の各キーについて、`=`・`:` 区切り、引用値、`Bearer `/`Basic ` の次トークン、`Authorization` ヘッダを置換する（[`Config.RedactText`](../../../internal/redaction/redactor.go)）。
2. **値形式検出** — `RedactText` の後段で [`ValueDetector.Mask`](../../../internal/redaction/value_detector.go) が AWS アクセスキー ID・GitHub/Slack トークン・JWT・URL 埋め込み認証情報・PEM・許可ホスト配下 URL を値の形式だけで置換する。
3. **値まるごと判定** — 1・2 で変化が無かった場合、[`SensitivePatterns.IsSensitiveValue`](../../../internal/redaction/sensitive_patterns.go) が未アンカーの部分一致 `(?i)(password|token|secret|key|api_key)` など（[`DefaultSensitivePatterns`](../../../internal/redaction/sensitive_patterns.go) が定義する）で値全体を `[REDACTED]` に置き換える（[`Config.RedactLogAttribute`](../../../internal/redaction/redactor.go) と [`RedactingHandler.redactLogAttributeWithContext`](../../../internal/redaction/redactor.go) の共通判定）。

### 識別子が書き換わる経路

group 名・コマンド名は属性値としてこの 3 層を通る。キー名 `group`・`command`・`command_name`・`name`・`notification_context` のいずれも `IsSensitiveKey` には一致しないため、値の内容だけで判定される。

| 層 | 識別子で発火する例 |
|---|---|
| key=value 置換 | コマンド名 `backup --password=x`。[`buildKeyValueRegex`](../../../internal/redaction/redactor.go) の `key` の隣接 `=` 代替は境界を持たないため、`monkey="a b"` が `monkey=[REDACTED] b"` になる（[`TestRedactText_AlternativePriority`](../../../internal/redaction/redactor_test.go)） |
| 値形式検出 | group 名 `AKIAIOSFODNN7EXAMPLE`、`ghp_` + 36 文字、`github_pat_` + 30 文字 |
| 値まるごと判定 | group 名 `monkey`、`keyboard`、`rotate_api_key`、`basic_auth`、`my_github_token` |

### 実害

1. **通知 Scope が発生箇所を指せなくなる。** `monkey` という group の通知は Scope が `[REDACTED]` になり、どの group で起きたのかを通知から判別できない。Task 0172 はこの挙動を残余リスクとして受け入れている（[`02_architecture.md`](../0172_slack_notification_message_unification/02_architecture.md) §3.5「表示安全な補間契約」）。
2. **エラー全文が失われうる。** [`RedactingHandler.processError`](../../../internal/redaction/redactor.go) は `error` 属性の文字列を `RedactText` に掛けたうえで `IsSensitiveValue` の全置換にも通す。`failed to execute group monkey: ...` のように名前を含むエラーメッセージは、全文が `[REDACTED]` になりうる。Scope だけでなくエラーの内容そのものが消える。
3. **JSON ログでも同じ。** 免除は Slack ハンドラ単体の話ではなく、全出力先に掛かる。

### 識別子は redaction の保護対象ではない

group 名・コマンド名は TOML に人間が書くリテラルであり、変数展開も外部入力も経由しない。同じ文字列は設定ファイルに平文で存在し、名前に機密を書く経路が現実に無いことは Task 0172 が既に結論している（[`02_architecture.md`](../0172_slack_notification_message_unification/02_architecture.md) §3.1「識別子は設定境界で検査する」、[`03_implementation_plan.md`](../0172_slack_notification_message_unification/03_implementation_plan.md)「設定検証」節）。したがって値ベース redaction が識別子を守る効果は薄く、通知の識別機能を壊す害の方が大きい。

Task 0172 は識別子を設定境界で拒否する旧検査を撤去した（commit `8d0667c5`）。拒否は語の部分一致による誤検知で設定を実行不能にするだけであり、残った論点は「表示時に書き換えないこと」である。0172 は `internal/redaction` の適用範囲を変更せず、その見直しを別タスクと明記している（[`02_architecture.md`](../0172_slack_notification_message_unification/02_architecture.md) §3.5、同 §5.2「既存の保護との関係」）。本タスクがその別タスクである。

### キー名では除外できない

キー `"command"` は TOML のコマンド名（[`DefaultGroupExecutor.executeAllCommands`](../../../internal/runner/group_executor.go)）と展開済みコマンド行（[`DefaultExecutor.executeWithUserGroup`](../../../internal/runner/base/executor/executor.go)、[`DefaultExecutor.prepareCommand`](../../../internal/runner/base/executor/command_lifecycle.go) ほか）の両方に使われる。キー `"name"` も group 名（[`DefaultGroupExecutor.ExecuteGroup`](../../../internal/runner/group_executor.go)）・一時ファイル名（[`moveFileAnchored`](../../../internal/safefileio/safe_file_linux.go)）・コマンド結果の名前（[`CommandResult.LogValue`](../../../internal/common/logschema.go)）に使われる。キー単位の除外は、コマンド行の redaction を弱めるか、名前の一部を残すかのどちらかになり、両立しない。

## 目的

- group 名・コマンド名を「識別子」として**型で宣言**し、redaction の値ベース変換から明示的に除外する。
- 自由文（stdout・stderr・コマンド行・引数・環境変数値・message・error 文字列）の redaction は一切弱めない。
- Slack・JSON・text の読み取り構造と、設定検証の挙動を変えない。

## スコープ

### 対象

1. 識別子を表す型の追加（`internal/common`。型名とメソッドは `02_architecture.md` で確定する）。
2. `internal/redaction` に、宣言された識別子を値ベース変換の対象外として明示的に認識する経路を追加し、下流ハンドラには string として正規化して渡す。
3. group 名・コマンド名を属性値として書く production の全経路を宣言型へ置き換える。少なくとも `group`、`command`、`command_name`、`name`、`notification_context` の名前値、`CommandResult`／`CommandResults` の名前が対象（約 13 ファイル・40 属性サイト）。
4. 免除ケースと対照ケース（同じ内容の plain string は従来どおり redact される）を固定するテスト、コマンド行 redaction の維持を固定するテスト、Slack・JSON の表示を確認するテスト。
5. `docs/dev/architecture_design/security-architecture.md` と `.ja.md`、`docs/user/security-risk-assessment.md` と `.ja.md` の更新。

### 対象外

- **値ベース検出パターン自体の変更。** `IsSensitiveValue` の語境界化や `ValueDetector` のパターン調整は行わない（自由文の検出挙動を変えないため）。
- **message・error 文字列に連結された識別子の免除。** 型では宣言できず、`RedactText`・`IsSensitiveValue` が引き続き適用される。この実害は残余リスクとして記録する（F-004）。
- **設定境界の検査追加。** Task 0172 の決定（識別子の中身を redaction と照合しない）を維持する。
- **`record.Message` の redaction 免除。**
- **キー名ベースの除外。** `"command"`・`"name"` の多重用途により成立しない（背景参照）。
- **Slack の表示安全契約の変更。** 1 行化・entity 変換・制御文字除去・切り詰めは現状維持とする。
- **0172 の承認済み文書の改訂。** 残余リスクの記述は履歴として残し、本タスクが置き換えることを本タスク側に記す。

## 決定事項

### 識別子は型で宣言する（Declare, don't infer）

除外の判断を文字列の内容や属性キーから推測せず、値の型で宣言する。識別子として宣言された値は、内容にかかわらず値ベース変換の対象外とする。型の構築は production コードに限られ、外部入力から任意の値を作る経路は設けない。

### キー名による除外はしない

属性キーは「何の値か」を保証しない。同じキーに識別子と自由文の両方が載る現状では、キー単位の除外はコマンド行の redaction を弱める。型で区別することが唯一の一貫した方法である。

### 免除の範囲は値ベース 3 層すべて

key=value 置換・値形式検出・値まるごと判定のいずれも、宣言された識別子には適用しない。コマンド名に `=` や `:` が含まれても書き換えない。この帰結として、設定の名前に機密を書いた場合は通知・ログにそのまま出るが、その文字列は設定ファイルに平文で存在し、名前に機密を書く経路が現実に無いことは Task 0172 で確認済みである。この帰結は利用者向けセキュリティ文書に明記する（AC-16）。

### パターン集合は変更しない

`IsSensitiveValue` を語境界付きにすれば `monkey` は救えるが、`AKIA…` 形の値形式一致と `monkey=` の隣接形は救えず、自由文の検出挙動まで変わる。免除は宣言にのみ基づかせ、パターン集合は据え置く。

### 下流ハンドラには string として正規化する

`RedactingHandler` の免除経路は宣言された識別子を string 値として後続へ渡す。Slack ハンドラ・JSON ハンドラ・`message_formatter` の読み取りコードは変更しない。テスト用の捕捉ヘルパーが値の型に依存している場合の扱いは `02_architecture.md` で決める。

### 0172 の残余リスクを置き換える

本タスクの完了をもって、Task 0172 `02_architecture.md` §3.5 の残余リスク（redaction が識別子を書き換え、Scope が `[REDACTED]` になりうる）は解消される。0172 の承認済み文書は履歴として残し、本タスクの文書から相互参照する（AC-17）。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 識別子の型宣言と値ベース redaction からの免除

**Acceptance Criteria**:
- **AC-01**: `monkey` という group 名を持つ通知の Scope に、`[REDACTED]` ではなく `monkey` が表示される。
- **AC-02**: `rotate_api_key` というコマンド名を持つ通知の Scope に、`rotate_api_key` が表示される。
- **AC-03**: 値形式に一致する識別子（`AKIAIOSFODNN7EXAMPLE`、`ghp_` + 36 文字、`github_pat_` + 30 文字）が、値形式検出で書き換わらない。
- **AC-04**: 免除が key=value 置換・値形式検出・値まるごと判定の 3 層すべてに及ぶ。同じ識別子を 3 層それぞれの一致形（`=` を含むコマンド名、AWS キー ID 形、`key` を含む語）で用意し、いずれも書き換わらない。
- **AC-05**: 同じ文字列が識別子として宣言されず自由文（stdout・stderr・コマンド行・引数・環境変数値・message・error）に現れた場合の redaction は、本タスクの前後で変わらない。`--password=x`、`token=...`、`Bearer ...`、AWS/GitHub/Slack トークン形は引き続き redact される。
- **AC-06**: 下流ハンドラが受け取る識別子は string 値であり、JSON ログでも元の文字列として出力される。Slack の Scope 表示と `message_formatter` の読み取りが、本タスクによる変更を要さない。

#### F-002: 書き込み側の宣言

**Acceptance Criteria**:
- **AC-07**: production コードで group 名・コマンド名をログ属性値として書く経路がすべて宣言型を使う。少なくとも次を含む。
  - `notification_context` の `group`／`command`: [`NotificationContext.LogValue`](../../../internal/common/notification_context.go)
  - `CommandResult`／`CommandResults` の名前: [`CommandResult.LogValue`](../../../internal/common/logschema.go)、[`CommandResults.LogValue`](../../../internal/common/logschema.go)
  - `group` 属性: [`Runner.logGroupExecutionSummary`](../../../internal/runner/runner.go)、[`DefaultGroupExecutor.verifyGroupFiles`](../../../internal/runner/group_executor.go)、[`DefaultGroupExecutor.outputDryRunDebugInfo`](../../../internal/runner/group_executor.go)、[`DefaultGroupExecutor.executeCommandInGroup`](../../../internal/runner/group_executor.go)、[`DefaultGroupExecutor.resolveGroupWorkDir`](../../../internal/runner/group_executor.go)、[`DefaultTempDirManager.Create`](../../../internal/runner/base/executor/tempdir_manager.go)、[`DryRunResourceManager.validateRunAsIdentity`](../../../internal/runner/resource/dryrun_manager.go)、[`Manager.VerifyGroupFiles`](../../../internal/verification/manager.go)、[`Manager.collectVerificationFiles`](../../../internal/verification/manager.go)
  - `command` 属性のうちコマンド名: [`DefaultGroupExecutor.executeAllCommands`](../../../internal/runner/group_executor.go)、[`DefaultGroupExecutor.executeCommandInGroup`](../../../internal/runner/group_executor.go)、[`DefaultGroupExecutor.createCommandContext`](../../../internal/runner/group_executor.go)、[`buildCommandDebugLogArgs`](../../../internal/runner/group_executor.go)、[`DefaultGroupExecutor.executeSingleCommand`](../../../internal/runner/group_executor.go)、[`SecurityLogger.LogUnlimitedExecution`](../../../internal/logging/security.go)、[`SecurityLogger.LogLongRunningProcess`](../../../internal/logging/security.go)、[`SecurityLogger.LogTimeoutExceeded`](../../../internal/logging/security.go)、[`SecurityLogger.LogTimeoutConfiguration`](../../../internal/logging/security.go)、[`DryRunResourceManager.validateRunAsIdentity`](../../../internal/runner/resource/dryrun_manager.go)、[`DryRunResourceManager.evaluateCommandRisk`](../../../internal/runner/resource/dryrun_manager.go)、[`NormalResourceManager.ExecuteCommand`](../../../internal/runner/resource/normal_manager.go)、[`UnixPrivilegeManager.WithPrivileges`](../../../internal/runner/base/privilege/unix.go)、[`UnixPrivilegeManager.logElevationOutcome`](../../../internal/runner/base/privilege/unix.go)、[`DefaultExecutor.Execute`](../../../internal/runner/base/executor/executor.go)、[`DefaultExecutor.executeWithUserGroup`](../../../internal/runner/base/executor/executor.go)、[`resolveAndPrepareCommandSpec`](../../../internal/runner/config/expansion.go)
  - `command_name` 属性: [`Logger.LogUserGroupExecution`](../../../internal/runner/base/audit/logger.go)、[`Logger.LogRiskProfile`](../../../internal/runner/base/audit/logger.go)
  - `name` 属性のうち group 名: [`DefaultGroupExecutor.ExecuteGroup`](../../../internal/runner/group_executor.go)
- **AC-08**: キー `"command"` に載る展開済みコマンド行・解決済みパス（[`DefaultExecutor.executeWithUserGroup`](../../../internal/runner/base/executor/executor.go)、[`DefaultExecutor.executeNormal`](../../../internal/runner/base/executor/executor.go)、[`DefaultExecutor.prepareCommand`](../../../internal/runner/base/executor/command_lifecycle.go)、[`Manager.collectVerificationFiles`](../../../internal/verification/manager.go) など）は宣言型にしない。コマンド行に `token=...` を含めると redact され、同名のコマンド名は redact されないことを固定するテストがある。
- **AC-09**: 監査ログの `command_name` と、`command_group_summary` のコマンド一覧に載る名前が、redaction 後も残る。

#### F-003: 既存の決定との整合

**Acceptance Criteria**:
- **AC-10**: 設定検証は識別子の中身を redaction と照合しない。[`TestValidateIdentifiers`](../../../internal/runner/config/validation_test.go) の受容行と [`TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted`](../../../cmd/runner/integration_pre_execution_error_test.go) が引き続き通る。
- **AC-11**: Task 0172 の Scope 表示契約（空名・非表示名は `(scope: invalid)`、それ以外は名前を表示）を変えない。
- **AC-12**: `SensitivePatterns`・`ValueDetector`・`DefaultKeyValuePatterns` のパターン集合が変更されていない。

#### F-004: 残余リスクの記録

**Acceptance Criteria**:
- **AC-13**: message・error 文字列に連結された識別子が本タスクの免除対象外であること、およびその実害（`monkey` を含むエラー文字列が全文 `[REDACTED]` になりうる）が、設計文書またはセキュリティ文書に記載されている。
- **AC-14**: 識別子を redact しないことの帰結（設定の名前に機密を書いた場合は通知・ログにそのまま出る）が、利用者向けセキュリティ文書に記載されている。

#### F-005: ドキュメント

**Acceptance Criteria**:
- **AC-15**: `docs/dev/architecture_design/security-architecture.ja.md` と `security-architecture.md` の redaction 層の説明に、識別子の型宣言による免除が記載されている。
- **AC-16**: `docs/user/security-risk-assessment.ja.md` と `security-risk-assessment.md` の Limitations に AC-14 の内容が反映されている。
- **AC-17**: 本タスクが Task 0172 `02_architecture.md` §3.5 の残余リスクを置き換えることが、本タスクの文書から参照できる。

#### F-006: 全体の健全性

**Acceptance Criteria**:
- **AC-18**: 各コミットの時点で `make test` と `make lint` が通る。
- **AC-19**: F-001 から F-005 までの各 AC を検証するテストが、検証対象の挙動を壊すと失敗する（CLAUDE.md「Every test must be able to fail for its stated reason」）。確認したことをコミットメッセージに記す。

## Success Criteria（要件レベル）

- group 名・コマンド名が、その内容にかかわらず通知・ログで元の文字列のまま表示される。
- 自由文・コマンド行・引数・環境変数値の redaction は弱まっていない。
- Slack・JSON・text の読み取り構造、設定検証、既存の redaction パターン集合が変わっていない。
- 免除の適用範囲と残余リスクが文書化され、Task 0172 の残余リスクが解消されたことが追跡できる。
