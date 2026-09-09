# アーキテクチャ設計書: Slack 通知メッセージの書式統一とスコープ情報の付与

## Document Status

| Item | Value |
|---|---|
| Status | `review` |
| Created | 2026-09-08 |
| Review date | - |
| Reviewer | - |
| Comments | PR #1108 との比較レビューを経て、本設計を採用ブランチとした。#1108 から Text 行の補間規則（§3.5）、空のコマンド名の設定境界での拒否（§3.1）、過剰なスコープ情報の拒否（§3.1）、利用者向け文書 2 件の更新対象への追加（§2.2）、実機確認の代替（§5.3）を移植し、`audit` → `logging` 依存の根拠（§2.1）と Mermaid 凡例の規約適合を加えた。レビュー観点は移植部分と依存判断。PR レビューを受け、§3.5 の表示安全な補間契約をフィールド名ではなく値の役割（識別子・エンベロープ値・自由文・大量出力）で範囲付けする形へ再構成した。続くレビューで、§3.5 を「動的な値の一覧」と「出力の性質」という 2 つの列挙から規則とテストを導く形へ改め、識別子を値まるごとの redaction から除外する案を撤回して、設定の読み込みでの拒否に置き換えた |

## 関連文書

- [01_requirements.md](01_requirements.md) — 本設計が満たす要件と受け入れ基準
- [0163 redaction coverage and slack async](../0163_redaction_coverage_and_slack_async/02_architecture.md) — 送信キュー、送信失敗ロガー、ドライランの既存契約
- [0068 separate slack webhooks](../0068_separate_slack_webhooks/02_architecture.md) — 成功用・エラー用 Webhook の分離
- [security-architecture.ja.md](../../dev/architecture_design/security-architecture.ja.md) / [security-architecture.md](../../dev/architecture_design/security-architecture.md) — 削除する通知について記述を更新する
- [slack_async_delivery.ja.md](../../dev/architecture_design/slack_async_delivery.ja.md) / [slack_async_delivery.md](../../dev/architecture_design/slack_async_delivery.md) — 高優先度通知の説明を更新する
- [README.ja.md](../../../README.ja.md) / [README.md](../../../README.md) — Slack 統合を「セキュリティイベントのリアルタイム通知」と説明しており、削除後の実態に合わせて更新する
- [security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) / [security-risk-assessment.md](../../user/security-risk-assessment.md) — 高優先度キューが「セキュリティアラート等」を保持すると記しており、更新する

## 用語

| 用語 | 意味 |
|---|---|
| 通知コンテキスト | 通知の発生箇所を表す `common.NotificationContext`。ゼロ値はグローバルであり、有効値はグローバル、グループ、コマンドのいずれか。レコード上の属性欠落とは区別する |
| 通知種別定義 | `message_type`、メッセージ組み立て関数、送信キューの優先度を一体として保持する定義 |
| 種別固有部分 | 通知種別ごとに異なる要約と添付フィールド。Scope、Hostname、Run ID は含まない |
| 共通エンベロープ | 製品名、ログレベルに対応する表示、Scope、Hostname、Run ID を全通知へ加える共通部分 |
| 送信失敗ロガー | Slack への再送を起こさない出力先へ、送信失敗、破棄、通知定義の不備を記録する既存ロガー |

## 1. 設計の全体像

### 1.1 設計原則

1. 通知の発生箇所は文字列の有無から推測せず、`common.NotificationContext` で宣言する。ゼロ値は要件どおりグローバルとする一方、レコードに通知コンテキスト属性が無い状態は付与漏れとして不正にする。レコード上では §3.1 の固定エンコードで運び、未知の語は不正へ倒して復元する。
2. 存続する通知種別は 1 つの通知種別定義に集約する。種別名、組み立て関数、キュー優先度を別々に列挙しない。
3. 種別ごとの組み立て関数は種別固有部分だけを返し、共通エンベロープの生成は 1 箇所に集約する。
4. 未知の通知種別や矛盾した通知コンテキストを補正しない。利用者への汎用通知と送信失敗ロガーの WARN により定義漏れを観測可能にする。
5. Task 0163 で定めた送信キュー、ワーカー、リトライ、flush、送信失敗ロガー、ドライランの境界を維持する。本設計はメッセージの構築と優先度の参照元だけを変更する。
6. 現在の添付形式を維持し、Slack の新しいレイアウト機能を導入しない。

### 1.2 概念モデル

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    SRC["通知の発火元"] --> REC[("slog レコード<br>message_type・notification_context")]
    REC --> REG["通知種別定義"]
    REG --> SPEC["種別固有部分"]
    REC --> ENV["共通エンベロープ"]
    SPEC --> MSG[("SlackMessage")]
    ENV --> MSG
    MSG --> SEND["既存の送信機構"]
    SEND --> SLACK[("Slack Incoming Webhook")]

    class REC,MSG,SLACK data
    class SRC,SEND process
    class ENV enhanced
    class REG,SPEC newpkg
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    L1[("データ")]
    L2["変更しない既存処理"]
    L3["変更・追加する処理"]
    L4["新しく追加する型"]

    class L1 data
    class L2 process
    class L3 enhanced
    class L4 newpkg
```

矢印 A → B は「A が B へデータを渡す、または B の生成に寄与する」ことを表す。本タスクでは新規パッケージを作らないため、紫は新しく追加する型を指す。

通知コンテキストと通知種別定義は役割が異なる。通知コンテキストは発火元が知る「どこで起きたか」を保持する。通知種別定義は `internal/logging` が知る「どの固有部分を作り、どちらのキューへ入れるか」を保持する。この分割により、発火元は Slack の表示形式や送信キューを知らずに済む。

### 1.3 現在の仕組みと変更後の境界

現在の `SlackHandler.Handle` は `slack_notify=true` のレコードだけを扱い、`message_type` の `switch` で `SlackMessage` を構築する。`slackSender` は別の `switch` で高優先度かを判定する。`user_group_command_failure` は発火元に文字列がある一方、どちらの一覧にも登録されていないため汎用メッセージとして処理される。

変更後は `SlackHandler.Handle` が通知種別定義を参照し、登録済みなら対応する種別固有部分と優先度を得る。未知の種別では汎用の種別固有部分を使用し、送信失敗ロガーへ WARN を記録する。未知種別の WARN 以上のレコードは高優先度、INFO は通常優先度とする。キュー選択は `slackRequest` に確定済みの優先度を載せて行うため、`slackSender` が種別名を再列挙する必要はない。

既存方式である複数の `switch` を残したまま `user_group_command_failure` を追記するだけでは、F-005 が求める「次の追加時にも登録漏れを検知する」ことを満たせない。このため、通知種別定義へ集約する。

## 2. システム構成

### 2.1 全体アーキテクチャ

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    subgraph Producers["通知の発火元"]
        MAIN["cmd/runner"]
        RUNNER["internal/runner"]
        AUDIT["internal/runner/base/audit"]
        RT["internal/runner/base/runnertypes"]
    end

    COMMON["internal/common<br>ログ属性名"]
    NCTX["common.NotificationContext"]
    LOGGING["internal/logging<br>固有部分 / 共通エンベロープ"]
    DEF["messageTypeDefinition"]
    SENDER["internal/logging<br>既存の slackSender"]
    RECORD[("構造化 slog レコード")]
    PAYLOAD[("SlackMessage")]
    WEBHOOK[("成功用／エラー用 Webhook")]

    MAIN -.->|"型を利用"| COMMON
    RUNNER -.->|"型を利用"| COMMON
    AUDIT -.->|"型を利用"| RT
    AUDIT -.->|"型を利用"| COMMON
    MAIN -.->|"通知属性 API を利用"| LOGGING
    RUNNER -.->|"通知属性 API を利用"| LOGGING
    AUDIT -.->|"通知属性 API を利用"| LOGGING
    COMMON --> NCTX
    NCTX --> RECORD
    MAIN --> RECORD
    RUNNER --> RECORD
    AUDIT --> RECORD
    COMMON --> RECORD
    RECORD --> DEF
    DEF --> LOGGING
    LOGGING --> PAYLOAD
    PAYLOAD --> SENDER
    SENDER --> WEBHOOK

    class RECORD,PAYLOAD,WEBHOOK data
    class SENDER process
    class MAIN,RUNNER,AUDIT,RT,COMMON,LOGGING enhanced
    class NCTX,DEF newpkg
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    L1[("データ")]
    L2["変更しない既存処理"]
    L3["変更・追加する処理"]
    L4["新しく追加する型"]

    class L1 data
    class L2 process
    class L3 enhanced
    class L4 newpkg
```

実線の矢印 A → B は「A のデータが B へ流れる」こと、破線の矢印 A ⇢ B は「A が B の型を利用する」というパッケージ依存を表す。`internal/logging` から発火元への逆向き依存は作らない。`internal/common` に通知コンテキストを置くのは、`internal/runner` と `internal/logging` の循環 import を避け、既存のログスキーマ共有責務を再利用するためである。

**`internal/runner/base/audit` から `internal/logging` への依存**。この図で `AUDIT ⇢ LOGGING` は新しい辺である。`audit` は現在 `internal/common`、`internal/redaction`、`internal/runner/base/*` にしか依存しておらず、`internal/logging` を取り込んでいない。§3.4 の設計では発火元が `logging.NotificationAttrs` と公開アクセサの返す token を使うため、`audit` もこの依存を得る。

循環は生じない。`internal/logging` の依存は `internal/ansicolor`、`internal/common`、`internal/groupmembership`、`internal/safefileio`、`internal/terminal` であり、`runnertypes` も `audit` も含まない（`go list -deps ./internal/logging` で確認済み）。`cmd/runner` と `internal/runner` は既に `internal/logging` に依存しているため、新しい辺は `audit` の 1 本だけである。

それでもこれは base 層から `logging` への辺を 1 本増やす判断である。代案は、種別名を `internal/common` に定数として置き、発火元が文字列で名乗る形にすることであった。採らなかったのは、それが AC-27 の求める「同じ種別集合を独立に列挙する箇所が他に無い」を崩すためである。定数の一覧と読み側の定義という 2 本目の列挙が生まれ、その一致を別の機構で守る必要が出る。token を渡す形なら発火元は種別名の文字列を一度も書かないため、列挙は §3.4 の登録 1 箇所に留まる。依存 1 本と引き換えに並行リストを 1 本消す取引として、前者を選ぶ。

### 2.2 コンポーネント配置

| ファイル | 種別 | 責務 | 更新が必要な既存テスト |
|---|---|---|---|
| `internal/common/notification_context.go` | 新規 | 通知スコープ、通知コンテキスト、コンストラクタ、参照メソッド、`slog.LogValuer`、および識別子 1 個あたりの長さ上限の定数を定義する | 新規 `internal/common/notification_context_test.go` でゼロ値、各スコープ、属性の省略、矛盾値を検証する |
| `internal/common/notification_context_test.go` | 新規 | F-002 の型とログ表現を検証する | - |
| `internal/common/logschema.go` | 変更 | 削除する 3 種別の属性定義と重大度定数を除き、`UserGroupCommandFailureAttrs` を含む存続する通知の共有属性名と型を定義する。既存の `GroupSummaryAttrs.Group` を維持し、通知コンテキストのキー名とスコープ名の対応表を加える | 該当定義を直接使う各パッケージのテスト。通知コンテキストを追加する各パッケージのテスト |
| `internal/logging/notification.go` | 新規 | 存続する通知種別の唯一の定義、発火元用の属性生成関数、確定済み優先度、種別固有部分の契約を定義する | 新規 `internal/logging/notification_test.go` で通知種別定義の集合と発火元の静的契約を検証する |
| `internal/logging/notification_test.go` | 新規 | 全定義の共通契約と、本番コードの発火元が属性生成関数を迂回しないことを検証する | - |
| `internal/logging/slack_handler.go` | 変更 | 通知種別定義の参照、種別固有部分の生成、共通エンベロープ、未知種別と不正スコープの WARN を担う。削除対象の 3 ビルダーを削除する。ホスト名取得を指す非公開のパッケージ変数（初期値は `common.GetHostname`）を置き、共通エンベロープの Hostname をこれだけに経由させる（§3.5） | `TestSlackHandler_Handle_WithMockServer` と同ファイルのメッセージ構築、切り詰め、属性抽出のテスト |
| `internal/logging/slack_handler_test.go` | 変更 | 存続する全種別と汎用メッセージの書式、Scope、フィールド順、未知種別、不正スコープを検証する。ホスト名の継ぎ目を差し替えてエンベロープ値の「出力の性質」も検証する（§7.1） | 削除対象 3 種別のケースを除く |
| `internal/logging/slack_sender.go` | 変更 | 独立した種別定数一覧と `isHighPriority` を削除し、確定済みの優先度で既存キューを選ぶ | `TestSlackSender_HighPriorityBypassesFullNormalQueue`、`TestSlackSender_QueueOverflowDropsAndRecords`、`TestSlackSender_FlushLogsMessageTypeBreakdown` |
| `internal/logging/slack_sender_test.go` | 変更 | 存続する `pre_execution_error` で高優先度の実効性を検証する | `security_alert` を使う既存ケースを置換する |
| `internal/logging/pre_execution_error.go` | 変更 | `PreExecutionError` に通知コンテキストを加え、`HandlePreExecutionError` が構造体を受け取って属性として記録する。共有ヘルパー `handleErrorCommon` は `slack_notify` と `message_type` を自前で組まず、呼び出し元の属性をそのまま記録する | `TestHandlePreExecutionError_AllTypes`、`TestHandlePreExecutionError_SlackNotification`、`HandleExecutionError` の既存テスト |
| `internal/logging/pre_execution_error_test.go` | 変更 | グローバルとグループの通知コンテキスト、および既存の stderr/stdout 出力を検証する | 位置引数を使う全ケースを移行する |
| `internal/runner/base/runnertypes/runtime.go` | 変更 | 既存の `TimeoutResolution.GroupName` を返す参照メソッドを追加する | `TestRuntimeCommand_Structure`、`TestRuntimeCommand_HelperMethods`、`TestNewRuntimeCommand_TimeoutResolution*` |
| `internal/runner/base/runnertypes/runtime_test.go` | 変更 | 既存の保持値を参照メソッドが返すことを検証する | 構造体リテラルを使う既存ケースでは `TimeoutResolution.GroupName` を明示する |
| `internal/runner/config/validation.go` | 変更 | コマンド名が空である設定、表示できる文字を含まない設定、group 名・command 名が長さ上限を超える設定、の 3 検査を、既存の `ValidateGroupNames` と同じ経路へ追加する。redaction の検査だけは正規化済みの許可ホストが要るため `bootstrap` 側に置く（§3.1） | `TestValidateGroupNames` と同ファイルの検証テーブル |
| `internal/runner/config/errors.go` | 変更 | 空のコマンド名、表示できる文字を持たないコマンド名、長さ上限を超える識別子、redaction の変換が書き換える識別子に対する 4 個のセンチネルエラーを、既存の `ErrEmptyGroupName` に並べて定義する | - |
| `internal/redaction/redactor.go` | 変更 | `RedactLogAttribute` が文字列値へ施す変換（`RedactText` と `IsSensitiveValue`）が値を書き換えるかを返す述語を公開する。既存の変換を読み取るだけで、redaction の適用範囲は変えない（§3.1） | 既存テストは変更しない。述語が `ValueDetector` の検出にも及ぶことを新規ケースで検証する |
| `internal/runner/base/audit/logger.go` | 変更 | `LogSecurityEvent` と `LogPrivilegeEscalation` を削除し、失敗したユーザー／グループ指定コマンドへコマンドスコープを付ける | `TestLogger_LogUserGroupExecution` とマスク関連テスト。削除対象テストは F-001 のカバレッジ比較対象 |
| `internal/runner/base/audit/logger_test.go` | 変更 | 削除対象の発火元のテストを削除し、ユーザー／グループ指定コマンドの失敗通知のコンテキスト属性を検証する | `TestLogger_LogPrivilegeEscalation`、`TestLogPrivilegeEscalation_Masking`、`TestLogger_LogSecurityEvent`、`TestLogSecurityEvent_*` を削除する |
| `internal/runner/base/privilege/unix_privilege_test.go` | 変更 | `logElevationOutcome` が native root と `seteuid` の結果を記録し続けることを検証するテストを追加する | 既存ケースは変更しない。同ファイルはプロセス全体の識別情報を共有するため、追加するテストも `t.Parallel()` を呼ばない |
| `internal/runner/runner.go` | 変更 | グループ検証エラーを `GroupScope` と構造化された本文で通知し、グループ集計へ通知コンテキストを付ける | `TestSlackNotification` と検証エラー経路のテスト |
| `internal/runner/runner_test.go` | 変更 | グループ集計とグループ検証エラーの通知コンテキストを検証する | `TestSlackNotification` を拡張する |
| `cmd/runner/main.go` | 変更 | 11 箇所の `PreExecutionError` リテラルへ `GlobalScope` を加え、報告境界で Run ID を代入してから構造体を渡す | 起動前エラーの統合テスト群 |
| `internal/runner/bootstrap/config.go` | 変更 | 4 箇所の `PreExecutionError` リテラルへ `GlobalScope` を加える。ここで構築される設定読み込み失敗は SlackHandler の登録前に起きるため Slack へは届かず、AC-15 の検証対象ではない。あわせて `normalizeSlackAllowedHost` の成功直後に識別子の redaction 検査を置き、`AddSlackHandlers` と同じ `NewConfig(WithWebhookHost(...))` で組み立てた `redaction.Config` を使う（§3.1） | 同パッケージの設定読み込みエラーのテスト |
| `internal/runner/bootstrap/environment.go` | 変更 | 2 箇所の `PreExecutionError` リテラルへ `GlobalScope` を加える | 同パッケージの環境準備エラーのテスト |
| `cmd/runner/startup_privilege_test.go` | 変更 | 特権降格エラーの新しい引数形を検証する | `TestReportStartupPrivilegeFailure_UsesValidRunID` |
| `cmd/runner/integration_pre_execution_error_test.go` | 変更 | 設定読み込みなどの起動前エラーがグローバルスコープを保つことを検証する | 同ファイルの既存 E2E テスト |
| `cmd/runner/integration_slack_flush_test.go` | 変更 | 終了時 flush で送る通知レコードへ通知コンテキストを付け、共通書式を検証する | `TestIntegration_RunnerFlushesSlackOnNormalExit` |
| `internal/runner/e2e_slack_webhook_separation_test.go` | 変更 | INFO と WARN 以上の Webhook 分離および新書式を検証する | `TestE2E_SlackWebhookSeparation_*`、`TestE2E_SlackWebhookSeparation_MessageFormat` |
| `internal/runner/e2e_slack_webhook_test.go` | 変更 | モックサーバーで新しいペイロード全体を検証する | `TestE2E_SlackWebhookWithMockServer` |
| `docs/dev/architecture_design/security-architecture.ja.md`・`.md` | 変更 | 削除後のセキュリティイベント通知の実態へ更新し、英語版へ反映する | - |
| `docs/dev/architecture_design/slack_async_delivery.ja.md`・`.md` | 変更 | 高優先度通知の説明を `pre_execution_error` に合わせ、英語版へ反映する | - |
| `README.ja.md`（96 行目付近） | 変更 | Slack 統合の説明「セキュリティイベントのリアルタイム通知」を、実際に通知される内容（グループ実行の結果と実行前エラー）へ改める | - |
| `docs/user/security-risk-assessment.ja.md`（301 行目付近） | 変更 | 高優先度キューの説明「セキュリティアラート等」を、削除後に唯一残る `pre_execution_error` へ改める | - |
| `docs/user/runner_command.ja.md` | 変更 | 通知種別、統一書式、Scope、製品名、ドライラン時の挙動を日本語で説明する | - |
| `README.md`・`docs/user/security-risk-assessment.md`・`docs/user/runner_command.md` | 変更 | 上記 3 件の日本語版を先にコミットしたうえで、`/mktrans` で英語版へ反映する | - |

既存のトップレベル `group` 属性は、構造化ログを解析する外部利用者との互換性のため維持する。新しい通知コンテキスト属性を追加しても、Slack 表示とスコープ判定は通知コンテキストだけを参照し、文字列の有無からスコープを推測しない。

削除する `security_alert` と `privilege_escalation_failure` は、開発者向けだけでなく**利用者向けの文書にも機能として記載されている**。放置すると、production から消えた通知を約束したままの記述が残る。次の 4 件を実態に合わせる。

| 文書 | 現在の記述 |
|---|---|
| `docs/dev/architecture_design/security-architecture.ja.md` | セキュリティイベントの Slack 通知を提供すると記している |
| `docs/dev/architecture_design/slack_async_delivery.ja.md` | 高優先度キューの根拠を「セキュリティアラート等」と記している |
| `README.ja.md` | Slack 統合を「セキュリティイベントのリアルタイム通知」と説明している |
| `docs/user/security-risk-assessment.ja.md` | 高優先度キューが「セキュリティアラート等」を保持すると記している |

いずれも日本語版を先に直してコミットし、英語版（`security-architecture.md`、`slack_async_delivery.md`、`README.md`、`docs/user/security-risk-assessment.md`）へは `/mktrans` で反映する。日英を直接両方編集しない。

新規パッケージは作らず、既存の `internal/common`、`internal/logging`、`internal/runner` の責務を再利用する。

### 2.3 通知データフロー

```mermaid
sequenceDiagram
    box rgb(232,245,232) 変更・追加する処理
        participant P as 通知の発火元
        participant C as common.NotificationContext
        participant H as SlackHandler.Handle
        participant D as 通知種別定義
        participant E as 共通エンベロープ生成
    end
    box rgb(255,241,230) 変更しない既存処理
        participant S as slackSender
    end
    box rgb(230,247,255) 外部サービス
        participant W as Slack Incoming Webhook
    end

    P->>C: 発生箇所を宣言
    P->>H: slog レコードを発行
    H->>D: message_type を照合
    alt 登録済み
        D-->>H: 組み立て関数と優先度
    else 未知
        H->>H: WARN を記録し汎用の固有部分を選択
    end
    H->>E: 固有部分、ログレベル、通知コンテキスト
    E-->>H: SlackMessage
    H->>S: ペイロードと確定済み優先度を投入
    S->>W: 既存のリトライ方針で送信
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;

    L1["変更・追加する処理"]
    L2["変更しない既存処理"]
    L3[("外部サービス")]

    class L1 enhanced
    class L2 process
    class L3 data
```

矢印 A → B は「処理の呼び出し、またはデータの受け渡し」を表し、破線の矢印は戻り値を表す。参加者を囲む色付きボックスの色は上の凡例に対応する。未知種別の WARN と不正スコープの WARN は Slack へ戻さず、既存の送信失敗ロガーへ書く。

### 2.4 副作用の境界

本タスクは新しいフラグやモードを追加しない。既存モードの副作用は次のとおりである。

| モード | Slack への HTTP 送信 | 送信キューとワーカー | ローカルログ | ファイルの書き込み・削除、コマンド実行 |
|---|---|---|---|---|
| 通常 | 許可する | 既存どおり使用する | 通知定義の WARN を含めて許可する | 本タスクでは変更しない |
| `--dry-run` | 抑止する | 生成しない | 既存どおり許可する。Slack メッセージ構築と送信時のスキーマ診断は行わない | 既存のドライラン契約に従い、外部副作用を抑止する |
| `GSCR_SLACK_SYNC=1` | 許可する | キューとワーカーを使わず `Handle` 内で送る | 通常モードと同じ | 本タスクでは変更しない |

ドライランで送信時の WARN を出さないのは、Task 0163 §3.4.10 が定める「送信機構を作らず、メッセージも構築しない」という既存契約の帰結である。本番コードの発火元が属性生成関数と登録済み通知トークンを使うことは静的テストで検証するため、既知の発火元にある登録漏れはビルド時に検知される。通知コンテキストに含まれる実際の値の妥当性は通常実行時に検証する。

## 3. コンポーネント設計

### 3.1 通知コンテキスト

`internal/common` に次の型を置く。フィールドを非公開にすることで、パッケージ外の発火元が group と command を直接組み替えることを防ぎ、スコープをコンストラクタで宣言させる。要件どおり `ScopeGlobal` をゼロ値にする。

```go
type NotificationScope int

const (
    ScopeGlobal NotificationScope = iota
    ScopeGroup
    ScopeCommand
)

type NotificationContext struct {
    scope   NotificationScope
    group   string
    command string
}

func GlobalScope() NotificationContext
func GroupScope(group string) NotificationContext
func CommandScope(group, command string) NotificationContext

func (c NotificationContext) Scope() NotificationScope
func (c NotificationContext) GroupName() string
func (c NotificationContext) CommandName() string
func (c NotificationContext) LogValue() slog.Value
func (c NotificationContext) LogAttr() slog.Attr
```

`NotificationContext` のゼロ値と `GlobalScope()` の戻り値は同じ有効なグローバルスコープである。ただし、通知レコードに通知コンテキスト属性自体が無い状態とは区別する。発火元はグローバルな場合も `GlobalScope().LogAttr()` を明示的に付ける。これにより、型のゼロ値要件を満たしながら、属性の付与漏れを正常なグローバル通知に見せない。

#### レコード上のエンコード

`SlackHandler` は `common.NotificationContext` という具象型をレコードから受け取れない。`RedactingHandler.processLogValuer` が `LogValue()` を解決したうえで下位ハンドラへ渡すため、SlackHandler に届くのは解決後の `slog.KindGroup` である。このため `LogValue` の出力を固定エンコードとして規定する。

| キー | 型 | 出力条件 | 値 |
|---|---|---|---|
| `scope` | string | 常に | `global` / `group` / `command` のいずれか |
| `group` | string | 常に | group 名。グローバルでは空文字 |
| `command` | string | 空でないときだけ | command 名 |

読み側の復元処理は `internal/common` の 1 箇所に置く。下位キーを SlackHandler 側へ複製しない。属性の存在確認は SlackHandler が行い、属性値の復元と値の妥当性確認は common の関数が行う。

| 入力 | Scope の表示 | 妥当性 |
|---|---|---|
| 属性があり、値がゼロ値または `GlobalScope()` のエンコード | `(global)` | 妥当。group と command は空であること |
| `GroupScope("backup")` | `group=backup` | group が空でなく、command が空であること |
| `CommandScope("backup", "pg_dump")` | `group=backup command=pg_dump` | group と command がともに空でないこと |
| 通知コンテキスト属性が無い、重複する、またはグループ値でない | `(scope: invalid)` | 不正。WARN を記録する |
| 未知の scope | `(scope: invalid)` | 不正。WARN を記録する |
| `scope`、`group`、`command` のいずれかの値が文字列でない | `(scope: invalid)` | 不正。エンコードは 3 個とも string と定めている |
| `scope=group` または `scope=command` で group が空 | `(scope: invalid)` | 不正。名乗ったスコープに必要な情報を欠く |
| `scope=command` で command が空 | `(scope: invalid)` | 不正。同上 |
| `scope=global` で group または command が空でない | `(scope: invalid)` | 不正。名乗ったスコープより多くの情報を伴う |
| `scope=group` で command が空でない | `(scope: invalid)` | 不正。同上 |
| group 名または command 名が、§3.5 の補間契約を通すと表示できる文字を 1 文字も残さない | `(scope: invalid)` | 不正。名乗ったスコープの発生箇所を指し示せない |

情報を**欠く**組み合わせだけでなく、名乗ったスコープより情報を**過剰に伴う**組み合わせも不正とする。`scope=global` と名乗りながら group 名を持つ値は、コンストラクタからは作れない。これを受け入れて `(global)` と描画すると、レコードに残っていた発生箇所の情報を黙って捨てたうえ、「グローバルで起きた」という誤った断定を通知に載せることになる。どちらの読み方が正しいのかを補正で決める形であり、本設計の「補正しない」に反する。`scope=group` と名乗りながら command 名を伴う値も同じ理由で拒否する。

同様に、command スコープを group 名だけで描画することもしない。command 名を欠いた通知が正常な group スコープの通知と見分けられなくなり、AC-17 が黙って満たされない状態を作るためである。

コンストラクタは空文字をエラーとして返さず、必ず値を返す全域関数とする。理由は 2 つある。1 つは、通知コンテキストの構築がログ出力の途中に置かれるため、ここで失敗するとログ経路そのものを中断させてしまうこと。もう 1 つは、呼び出し元に意味のある回復手段が無く、エラーを返しても握り潰すか panic するかしか選べないことである。

したがって `GroupScope("")` や `CommandScope("group", "")` は構築でき、AC-09 が言う「パッケージ外から不正な値を作れない」は、フィールドが非公開でコンストラクタ以外の経路が無いという意味に限られる。空の名前は発火元（producer）の不具合であり、グローバル扱いへ黙って正規化することはしない。SlackHandler の表示境界で `(scope: invalid)` と送信失敗ロガーの WARN として大きく表面化させ、不具合が正しい通知と見分けられない状態を作らない。command scope で command 名が空の場合も、どのコマンドかを示せず Success Criteria を満たさないため同様に不正とする。

#### 識別子は設定の読み込みで検査する

表示境界の検知は最後の防御であって、最初の防御ではない。`audit.Logger.LogUserGroupExecution` が `CommandScope` へ渡す `cmd.Name()` は設定ファイル由来の値であり、現在の設定検証はこれが空である場合を拒否していない。`internal/runner/config/validation.go` の `ValidateGroupNames` はグループ名が空の設定を `ErrEmptyGroupName` で拒否し、`GroupNamePattern` で文字種も縛る一方、コマンド名について空を拒否する検証はどこにも無く、どちらの名前についても長さの上限は無い。すなわち `name` を書き忘れたコマンドを含む TOML が現在は読み込みを通り、`CommandScope(group, "")` が実在の設定から到達しうる。

そこで、設定の読み込み時に識別子（group 名と command 名）が**通知で発生箇所を指し示せる形である**ことを検査し、外れる場合は専用のセンチネルエラーで拒否する。検証の位置は既存の `ValidateGroupNames` と同じ、`config.Loader` が設定を読み込む経路とする。

| 検査 | 対象 | 拒否する理由 |
|---|---|---|
| 空でないこと | command 名（group 名は既存の `ErrEmptyGroupName` で拒否済み） | `name` の書き忘れが読み込みを通り、`CommandScope(group, "")` として発火点へ届く |
| 制御文字と書式制御文字を含まず、§3.5 の補間契約を通すと表示できる文字が 1 文字以上残ること | command 名（group 名は既存の文字種検証で担保済み） | 制御文字や空白だけの名前は、通知でもログでも監査記録でも指し示せない。双方向表示制御などの書式制御文字は、名前を実際とは別の順序で見せる |
| 長さが上限を超えないこと | group 名と command 名 | 補間契約は識別子を切り詰めない（§3.5）。長すぎる名前を表示境界で短くすると、先頭が一致する 2 つの名前が同じ Scope として表示され、どの group・どの command の通知か判別できなくなる |
| redaction が値を書き換えないこと | group 名と command 名 | 書き換えられる名前は `RedactingHandler` が `[REDACTED]` へ置き換えるため、通知でも JSON ログでも発生箇所を指し示せなくなる（§3.5）。語の一致（`IsSensitiveValue`）だけでなく、`ValueDetector` による値形式の検出（AWS キー ID、GitHub トークン、JWT など）も対象に含む |

センチネルエラーは検査ごとに独立させる。空の名前を書き忘れた設定、見た目には値があるのに指し示せない設定、長すぎる設定、redaction に潰される設定では、利用者が直す箇所も直し方も違うためである。空だけを拒否すると、制御文字だけからなる名前が読み込みを通り、通知では Scope が空白に潰れる。「補正しない」に従い、通知側で空白へ潰れる名前も、表示境界で短くするしかない名前も、設定境界で拒否する。

識別子 1 個あたりの長さ上限は `internal/common` の定数として置き、設定検証だけが参照する。値は補間契約の 500 byte 上限より十分小さい 128 byte とする。Text 行には製品名、STATUS、Scope、要約が並ぶため、識別子 1 個がその大半を占めると Text 行の役割を果たせない。上限を `internal/common/notification_context.go` に置くのは、通知コンテキストが載せる識別子の制約であり、その型の定義と同じ場所に置くことで同じ値を 2 箇所へ書かずに済むためである。

この検査は通知のためだけのものではない。名前を持たないコマンドは、Slack 通知に限らずログでも監査記録でも指し示せない。名前を設定境界で検査することで、「外部入力の誤りは読み込みで拒否する」「発火点へ届いた空の名前は呼び出し側の不具合であり、表示境界で `(scope: invalid)` として表に出す」という役割分担が成り立つ。空の名前を黙って通したうえで通知だけを不正表示にすると、設定が誤っていることは Slack を見た者にしか分からず、当のコマンドはそのまま実行され続ける。

機密パターンの検査は、`DefaultSensitivePatterns().IsSensitiveValue` だけを呼ぶ形にはしない。この判定は redaction が識別子を書き換える経路の一部にしか及ばないためである。`Config.RedactLogAttribute`（`internal/redaction/redactor.go`）は、文字列値に対してまず `RedactText` を呼ぶ。`RedactText` はキー名由来の `key=value` 置換を当てたのち `ValueDetector.Mask` を通し、ここが AWS アクセスキー ID（`AKIA`／`ASIA` + 16 文字）、GitHub トークンと fine-grained PAT、JWT、および設定された Webhook ホストを含む URL を、語の一致とは無関係に `[REDACTED]` へ置き換える。`IsSensitiveValue` が呼ばれるのはその後、値が変化しなかったときだけである。したがって `AKIAIOSFODNN7EXAMPLE` のような group 名は `IsSensitiveValue` に一致しないまま `ValueDetector` に潰される。`validateGroupName` の `[A-Za-z_][A-Za-z0-9_]*` はこの形の名前を許すため、実在する TOML から到達できる。

そこで検査は語の一覧ではなく**変換そのもの**を基準にする。すなわち、本番と同じ redaction の変換を識別子へ適用し、**値が変化したら拒否する**。判定は `internal/redaction` 側に述語として置き、`RedactLogAttribute` が文字列値へ施す変換（`RedactText` と `IsSensitiveValue` の両方）と同じ経路を通す。設定検証側で変換の一覧を複製しない。この形にすると、将来 `ValueDetector` に検出器が増えたときも設定検証が自動的に追随する。列挙で書けば、増えた検出器の分だけ Scope が `[REDACTED]` に潰れる名前が読み込みを通るようになる。

Webhook ホストの変換も対象に含める。許可ホストは TOML の `slack_allowed_host` であり、`GlobalSpec.SlackAllowedHost` として設定の復号で埋まる。`loadConfigInternal` は ConfigSpec 全体を復号したうえで `ValidateGroupNames` を呼ぶため（`internal/runner/config/loader.go`）、識別子の検査が走る時点で許可ホストの値は既に手元にある。したがって既定の `NewConfig()` だけを対象にする理由は無く、`https://hooks.slack.com/services/example` のような command 名が読み込みを通ったうえで Scope と `Command` の双方で `[REDACTED]` になる状態（AC-17 違反）を防げる。

ただし検査を `ValidateGroupNames` の中に置くことはできない。本番が `redaction.WithWebhookHost` へ渡すのは生の TOML 値ではなく `normalizeSlackAllowedHost` が小文字化と括弧除去を施した値であり（`bootstrap/config.go`）、この正規化は `LoadConfig` が返った**後**に走る。生の値で検査すると、TOML が大文字を含むときに検査と本番で別のパターンを使うことになり、検査を通った名前が本番で潰れる。正規化関数は `internal/runner/bootstrap` にあり、`bootstrap` は既に `internal/runner/config` へ依存しているため、逆向きの import は循環になる。

そこで識別子の redaction 検査は、`normalizeSlackAllowedHost` が成功した直後の `bootstrap` に置く。ここでは読み込み済みの設定と、本番が使うのと同じ正規化済みホストの双方が揃っており、検査用の `redaction.Config` を `AddSlackHandlers` と同じ `NewConfig(WithWebhookHost(...))` で組み立てられる。すなわち検査と本番が同一の変換を使うことを構成で保証する。空・表示できる内容を持たない・長すぎるの 3 検査は外部への依存を持たないため `ValidateGroupNames` に残す。センチネルエラーは 4 個とも `internal/runner/config/errors.go` に置いたままでよい。`bootstrap` は `config` に依存しており、そこから返せる。実行はいずれの検査より後であり、どのコマンドも走る前に拒否される点は変わらない。

`internal/runner/config` から `internal/redaction` への依存は新しい辺であるが、`internal/redaction` は `internal/runner/config` に依存しないため循環は生じない。

コンストラクタ側は前段落までのとおり全域関数のままとし、事前条件違反で panic させることはしない。通知コンテキストの構築はログ出力の途中に置かれるため、ここで panic するとエラー報告の最中にプロセスを落とす。拒否は設定境界に、検知は表示境界に置き、その中間にあるログ経路は決して停止させない。

### 3.2 `PreExecutionError` と発火元

`HandlePreExecutionError` の 4 個の位置引数を廃止し、既存の `PreExecutionError` を受け取る。これにより、呼び出し元で型、本文、コンポーネント、Run ID、通知コンテキストの対応が崩れにくくなる。

```go
type PreExecutionError struct {
    Type                ErrorType
    Message             string
    Component           string
    RunID               string
    NotificationContext common.NotificationContext
    Err                 error
}

func HandlePreExecutionError(preExecErr *PreExecutionError)
```

`cmd/runner` の設定読み込み、Run ID、ビルド設定、特権降格などの起動前エラーは `GlobalScope()` を渡す。`runner.executeGroups` のグループ検証エラーだけは `GroupScope(verErr.Group)` を渡す。検証結果本文から `Group: <name>, ` を除き、グループ名の唯一の表示場所を Scope にする。stderr と stdout の既存形式、および `HandleExecutionError` が Slack 通知を行わない契約は維持する。

`NotificationContext` のゼロ値は有効なグローバルスコープである。それでも意図を明示し `GlobalScope()` を production で使用するため、`cmd/runner/main.go` の 11 箇所、`internal/runner/bootstrap/config.go` の 4 箇所、`internal/runner/bootstrap/environment.go` の 2 箇所を移行対象とする。`HandlePreExecutionError` はこの値から通知コンテキスト属性を必ず生成する。構文木の静的契約テストは、本番の `PreExecutionError` リテラルで `NotificationContext` が省略されていないことを検証する。

`HandlePreExecutionError` が報告に使うフィールドは次のとおり固定し、現在の挙動をそのまま保つ。

| 項目 | 使用するフィールド | 根拠 |
|---|---|---|
| 本文 | `preExecErr.Detail()` | `bootstrap` のリテラルは原因を `Message` に平坦化せず `Err` に持たせており（`bootstrap/config.go` の該当コメント）、`Message` だけを読むと stderr、`RUN_SUMMARY` 行、Slack から原因が消える |
| Run ID | `preExecErr.RunID` | 現在 `main.go` が渡しているプロセス唯一の Run ID を失わないため、報告境界で `main.go` が `preExecErr.RunID` へ代入してから呼ぶ。Run ID の確定は `main` だけが知る事実であり、現在の位置引数と同じ値になる |
| 種別・コンポーネント | `preExecErr.Type`、`preExecErr.Component` | 現在と同じ |

`HandlePreExecutionError` と `HandleExecutionError` が共有する `handleErrorCommon` は、`slack_notify` と `message_type` を構造体フィールドから直接組み立てず、呼び出し元が渡した `[]slog.Attr` をそのまま記録する形へ変える。`HandlePreExecutionError` は §3.4 の `NotificationAttrs` が返す属性を渡し、`HandleExecutionError` は Slack へ送らない自分の属性を渡す。これにより、共有ヘルパーへ例外を設けずに §3.4 の静的契約テストを適用できる。

#### Slack へ届く実行前エラーの範囲

SlackHandler は TOML から許可ホストを読んだ後に登録されるため、TOML 自体の読み込み・解析失敗、ログ設定、Webhook URL 検証など登録前のエラーは Slack へ届かない。登録後に起きるグローバル設定の展開、テンプレート・対象ファイルの検証、グループ選択、および `runner.executeGroups` の検証エラーは届く。この境界は本タスクで変更しない。

到達性は `ErrorType` ではなく発生時点で決まる。たとえば `config_parsing_failed` は登録前後の両方で使われるため、AC-15 の統合テストには SlackHandler 登録後に発生するグローバル対象ファイルの検証失敗などを使う。production では届かない経路を handler 単体テストだけで緑にしない。

### 3.3 `RuntimeCommand` のグループ名

`RuntimeCommand` には、`NewRuntimeCommand` が受け取ったグループ名が既に `TimeoutResolution.GroupName` として保持されている。新しい重複フィールドは追加せず、その値を返す参照メソッドだけを追加する。

```go
func (r *RuntimeCommand) GroupName() string {
    return r.TimeoutResolution.GroupName
}
```

`audit.Logger.LogUserGroupExecution` は失敗時に `CommandScope(cmd.GroupName(), cmd.Name())` を付ける。コマンド名をログ本文や引数から推測しない。本番の `RuntimeCommand` は `NewRuntimeCommand` を通じて構築されるため、通知経路では生成時のグループ名を参照できる。テストで構造体リテラルを使う場合は `TimeoutResolution` を明示し、コンストラクタを迂回した不完全な値を本番相当として扱わない。

### 3.4 通知種別定義

`internal/logging` に、存続する 3 種別を保持する唯一の通知種別定義の集合を置く。各要素は種別名、種別固有部分の組み立て関数、キュー優先度を一体で保持する。

```go
// notificationPriority は送信キューの選択に使う確定済みの優先度。
type notificationPriority int

const (
    priorityNormal notificationPriority = iota
    priorityHigh
)

// messageDetails は種別固有部分。共通エンベロープ（製品名、レベルに対応する
// 表示、Scope、Hostname、Run ID）は含まない。
type messageDetails struct {
    headline string
    fields   []SlackAttachmentField
}

// messageBuilder は種別固有部分だけを作る。レコード以外の入力を取らない。
type messageBuilder func(slog.Record) messageDetails

// messageTypeDefinition は 1 種別の定義。値は registerNotification でのみ設定し、
// 初期化後は変更しない。
type messageTypeDefinition struct {
    messageType string
    priority    notificationPriority
    build       messageBuilder
}

// Notification は発火元が種別を名乗るための token。フィールドが非公開であり、
// パッケージ外で作れるのはゼロ値だけである。
type Notification struct {
    definition *messageTypeDefinition
}

// notificationDefinitions は通知種別の唯一の定義集合である。
// 種別の一覧、メッセージの組み立て、キュー優先度はすべてここから引く。
var notificationDefinitions []*messageTypeDefinition

// registerNotification は定義を notificationDefinitions へ加え、
// それを指す token を返す。呼び出しは下の var 宣言に限る。
func registerNotification(messageType string, priority notificationPriority, build messageBuilder) Notification

var (
    commandGroupSummaryNotification     = registerNotification(...)
    preExecutionErrorNotification       = registerNotification(...)
    userGroupCommandFailureNotification = registerNotification(...)
)

// 発火元向けの公開 API。再代入できる公開変数は置かない。
func CommandGroupSummaryNotification() Notification
func PreExecutionErrorNotification() Notification
func UserGroupCommandFailureNotification() Notification
func NotificationAttrs(notification Notification, notificationContext common.NotificationContext) []slog.Attr
```

`messageTypeDefinition`、`messageDetails`、`notificationDefinitions` はいずれも `internal/logging` の外から名指しできない。したがってフィールドも非公開とする。非公開の型に公開フィールドを持たせても、パッケージ外から書き換えられる範囲は変わらない一方、「外から設定される値である」という誤った合図を残す。

**`messageBuilder` が `*SlackHandler` を取らない理由**。組み立て関数が受け取るのはレコードだけとする。現在の 3 ビルダーが receiver から読んでいるのは `s.runID` の 1 個だけであり、その Run ID は本設計では共通エンベロープの担当へ移る。ハンドラを渡さなければ、ビルダーはエンベロープの要素を組み立てる手段そのものを持たない。AC-22 の「Text 行と末尾 3 フィールドの生成が 1 箇所に集約されている」が、規約ではなく引数の型によって保証される。副次的に、ビルダーはハンドラを構築せずに単体で検証できる純粋な関数になる。

定義する 3 種別は次のとおりである。

| `message_type` | 優先度 | 種別固有の要約 | 種別固有フィールド |
|---|---|---|---|
| `command_group_summary` | 通常 | 成功時は `<総数> commands in <時間>`、失敗時は `<総数> commands, <失敗数> failed in <時間>` | Command Count、Duration、各 Command と既存の Output / Error |
| `pre_execution_error` | 高 | `error_type` | Error Message、Component |
| `user_group_command_failure` | 通常 | `command failed (exit <終了コード>)` | Command、Exit Code、存在する場合は Output と Error Output |

**Scope、Hostname、Run ID の 3 個のフィールド見出しは共通エンベロープの予約語とする**。種別固有のビルダーはこの 3 語を見出しに使えない。同じ見出しのフィールドが種別固有部分と末尾 3 件の双方に現れると、読み手にはどちらが発生箇所を表すのか判別できず、AC-21 が言う「末尾 3 件が Scope、Hostname、Run ID である」ことも見た目には確かめられなくなる。予約語は共通エンベロープと同じ 1 箇所に定数として置き、末尾 3 フィールドの生成とビルダー側の検査が同じ定数を読む。検査は `notificationDefinitions` を走査するテストで行う（§7.1）。

`common.UserGroupCommandFailureAttrs` は `command_name`（string）、`exit_code`（int）、`stdout`（string）、`stderr`（string）を定義し、`audit.Logger.LogUserGroupExecution` とユーザー／グループ指定コマンド固有のビルダーが共有する。属性の記録側と参照側が同じ属性名と型を使うため、固有ビルダーに文字列リテラルを複製しない。

`command_group_summary` の `status` 属性は Slack 表示の判定に使わず、ログレベルを唯一の判定基準とする。発火元は既存どおり実行結果から INFO または ERROR を選ぶ。`GroupSummaryAttrs.Group` は既存の構造化ログ利用者との互換性のため残す。Slack の Scope は通知コンテキストだけから生成し、トップレベルの `group` を推論や表示には使わない。

各非公開 `Notification` 変数は、パッケージ初期化時に非公開の登録関数へ `message_type`、組み立て関数、優先度を 1 回だけ渡して生成する。登録関数は同じ `messageTypeDefinition` を通知種別定義の集合へ加え、そのポインタを非公開フィールドに保持するトークンを返す。外部パッケージには、再代入できる公開変数ではなく、値を返すアクセサ関数だけを公開する。この 1 回の宣言だけで、文字列、ビルダー、優先度、発火元が使うトークンを定義する。別の種別一覧や対応表は作らず、初期化後の集合は変更しない。`Notification` のフィールドは非公開なのでパッケージ外で作れるのはゼロ値だけである。`NotificationAttrs` はゼロ値を受け取った場合、`slack_notify=true` と空の `message_type` を返す。空文字は §3.6 の未知種別に一致するため、通知は汎用メッセージとして送られ、送信失敗ロガーへ `unknown_message_type` の WARN が残る。通知を黙って落とす選択はしない。それは F-005 が取り除こうとしている無言の握り潰しそのものであり、未知の `message_type` でも汎用送信と WARN を出すという §3.6 の扱いとも食い違うためである。本番コードでのゼロ値の使用は静的契約テストでも拒否するが、静的テストは本番コードの構文しか見ないため、実行時の挙動も併せて loud failure にしておく。

発火元は `NotificationAttrs` と公開アクセサが返す opaque token を使い、`slack_notify`、`message_type`、通知コンテキストを一組でレコードへ加える。この API を `internal/logging` に置くのは、発火元が既に依存するログ通知契約へ属性生成を集約し、`internal/common` に Slack 固有の種別を持ち込まないためである。テストは登録済みの通知種別定義を順に走査し種別名の一意性、組み立て関数、共通エンベロープ、優先度、公開アクセサが返す token との同一性を検証する。さらに本番コードが `slack_notify=true` または `message_type` を直接構築せず、`NotificationAttrs` の第 1 引数に登録済み token を返す公開アクセサの呼び出しだけを渡すことを Go 構文木の静的契約テストで検証する。

`security_alert`、`privilege_escalation_failure`、`privileged_command_failure` は定義、ビルダー、ログスキーマ、発火元、テストから削除する。`pre_execution_error` は削除対象の高優先度種別の代わりに、高優先度キューの実効性を検証する基準となる。

### 3.5 共通エンベロープ

共通エンベロープ生成はログレベル、通知コンテキスト、種別固有部分から 1 個の `SlackMessage` を作る。本番コードでは、製品名 `go-safe-cmd-runner` を一つの定数としてのみ定義する。

対応表はログレベルの全域に対して定義する。`LevelModeDefault` は公開されており `level >= s.level` で通過するため、本番の 2 モード以外では DEBUG など INFO 未満のレコードや、INFO と WARN の中間の値も `SlackHandler` へ届きうる。上から順に最初に一致した行を使い、どのレベルも必ずいずれかの行に一致する。

| 条件 | 絵文字 | STATUS | 添付色 |
|---|---|---|---|
| `level >= slog.LevelError` | ❌ | `ERROR` | `danger` |
| `level >= slog.LevelWarn` | ⚠️ | `WARNING` | `warning` |
| `level >= slog.LevelInfo` かつ WARN 未満 | ✅ | `SUCCESS` | `good` |
| INFO 未満 | ⚠️ | `WARNING` | `warning` |

閾値による範囲判定にするのは、AC-19 が求める「ログレベルだけで決まる」写像を全域関数にするためである。本番の成功用・エラー用ハンドラでは INFO 未満は到達しないが、公開された既定モードから想定外レベルが届いても成功と誤表示しないよう WARNING へ倒す。

Text 行は常に次の形とする。

```text
[go-safe-cmd-runner] <絵文字> *<STATUS>* — <Scope> : <種別固有の要約>
```

添付フィールドは、種別固有フィールドの後ろに Scope、Hostname、Run ID をこの順で追加する。汎用メッセージも同じ処理を通す。Scope フィールドの値は Text 行と同じスコープ表現にし、下の表示安全な補間契約を通す。この 3 個の見出しは §3.4 のとおり予約語であり、種別固有フィールドには現れない。

動的な group 名、command 名、要約、添付フィールド値は既存の redaction 後の値を受け取る。これらの加工は表示境界だけで行い、構造化ログの元値は変更しない。stdout 1000 文字、stderr 500 文字の既存上限は変えない。

#### 表示安全な補間契約

エンベロープが行の骨格を組み立てるとはいえ、Scope、要約、識別子はその行やフィールドの**中へ**埋め込まれる。group 名は設定の読み込みで文字種が検証されるが、command 名に文字種の検証は無く、汎用メッセージの要約はレコードの本文そのものである。したがって、埋め込む値の側に制約を課す。この制約の集合を**表示安全な補間契約**（以下、補間契約）と呼ぶ。

契約は 2 つの列挙から成る。1 つは、Text 行または添付フィールドへ到達しうる動的な値をすべて挙げ、ちょうど 1 つの役割へ対応付ける一覧である。もう 1 つは、契約の出力が必ず満たす性質の一覧である。変換規則は性質から導き、§7.1 のテストは 2 つの一覧を走査して作る。規則を列挙するだけの形にすると、役割を割り当て忘れた値も、どの規則も生み出さない性質も、書かれていないという理由だけで見過ごされる。2 つの一覧から導けば、どちらもテストの失敗として現れる。

##### 動的な値の一覧

適用範囲は**フィールド名では決めない**。「Text 行と Scope フィールドだけを対象にする」という決め方では、同じ値が別のフィールドへ載った時点で契約から外れる。`user_group_command_failure` の `Command` フィールドはその実例であり、そこに載る command 名は Scope と同じ設定由来の値でありながら、フィールド名で範囲を切ると素のまま出る。範囲は値の**役割**で決める。役割は識別子、エンベロープ値、自由文、大量出力の 4 つとする。

次の表は、Text 行または添付フィールドへ到達しうる動的な値をすべて挙げ、ちょうど 1 つの役割へ対応付ける。表に無い動的な値を通知へ載せることはしない。種別固有フィールドを追加するときは、この表へ行を足して役割を宣言する。

| 動的な値 | 現れる場所 | 役割 |
|---|---|---|
| group 名 | Text 行の Scope、Scope フィールド | 識別子 |
| command 名 | Text 行の Scope、Scope フィールド、`user_group_command_failure` の `Command` フィールド、`command_group_summary` がコマンド結果ごとに出す `Command` フィールド | 識別子 |
| `command_group_summary` の要約（コマンド数、失敗数、所要時間） | Text 行 | 自由文 |
| `pre_execution_error` の要約（`error_type`） | Text 行 | 自由文 |
| `user_group_command_failure` の要約（終了コード） | Text 行 | 自由文 |
| 汎用メッセージの要約（レコードの `Message`） | Text 行 | 自由文 |
| Error Message（`PreExecutionError.Detail()`） | `pre_execution_error` の添付フィールド | 自由文 |
| Component | `pre_execution_error` の添付フィールド | 自由文 |
| Command Count、Duration、Exit Code | 各種別の添付フィールド | 自由文 |
| Hostname | 共通エンベロープの添付フィールド | エンベロープ値 |
| Run ID | 共通エンベロープの添付フィールド | エンベロープ値 |
| stdout、stderr | `command_group_summary` と `user_group_command_failure` の添付フィールド | 大量出力 |

`command_group_summary` の `Command` フィールドは合成値である。現在の `buildCommandGroupSummary` はコマンド結果ごとに `<絵文字> \`<cmd.Name>\` (exit: <終了コード>)` という 1 個のフィールド値を組み立てており、設定由来の command 名がその中に埋め込まれる。この行を落とすと、`user_group_command_failure` の `Command` と同じ値が、別の種別の同名フィールドでは素のまま出ることになる。合成値の中では役割を部分ごとに割り当てる。すなわち command 名は識別子として補間してから合成し、終了コードは自由文とする。フィールド値を丸ごと 1 個の役割で通すのではない。バッククォートと `(exit: ...)` は生成側の骨格であり、動的な値ではない。

Error Message を自由文に置く理由を補足する。`pre_execution_error` の要約は `error_type` という短い語だが、Error Message フィールドの値は `PreExecutionError.Detail()` であり、`Err` に包まれた原因（TOML の構文エラー、ファイルパスなど）を連結した文字列である。改行も `<` も含みうるし、長さの上限も無い。要約が短いことは添付フィールドの値が短いことを意味しない。Command Count、Duration、Exit Code のように本来は書式化された数値であっても、経路は分けず自由文として扱う。実際には制御文字を含まない値でも、種別固有ビルダーが返す値をすべて同じ経路へ通すほうが、値ごとの例外を憶える形より漏れにくい。

##### 出力の性質

契約の出力は、次の性質を必ず満たす。AC-18 と AC-20 が主張するのはこの性質であり、§7.1 の表駆動テストは置き換え集合の各行ではなくこの性質そのものを検査する。置き換え集合に文字を足し忘れたときは、対応する性質の検査が失敗する。

| 性質 | 内容 | 適用する役割 |
|---|---|---|
| 1 行であること | 改行として扱われる文字（U+000A、U+000D、U+2028、U+2029）が出力に 1 文字も含まれない | 識別子、エンベロープ値、自由文 |
| 制御文字と書式制御文字を含まないこと | Unicode の一般カテゴリ Cc（制御文字。C0 の U+0000〜U+001F と、DEL および C1 の U+007F〜U+009F）、および一般カテゴリ Cf（書式制御文字。双方向表示制御の U+202A〜U+202E と U+2066〜U+2069 を含む）の文字が出力に 1 文字も含まれない | 同上 |
| 実体参照化されていること | 裸の `&`、`<`、`>` が出力に含まれない | 同上 |
| 長さが上限を超えないこと | 出力が UTF-8 で 500 byte 以下である | エンベロープ値、自由文 |
| 有効な UTF-8 であること | 出力が常に有効な UTF-8 であり、`&amp;`・`&lt;`・`&gt;` を途中で切らない | 識別子、エンベロープ値、自由文 |

`###` を含まないことは契約の性質に含めない。置き換え集合は `#` を対象にしておらず、どの規則も生み出さない性質を契約が主張することになるためである。AC-20 の `###` に関する主張は、Text 行の骨格とフィールド見出しという静的な部分に限る（後述）。

##### 役割ごとの規則

| 値の役割 | 適用する規則 |
|---|---|
| 識別子 | 設定の読み込みで、空・表示できる内容を持たない名前・書式制御文字を含む名前・過剰な長さ・redaction の変換が書き換える名前を拒否する（§3.1）。そのうえで、識別子を載せる**すべての**フィールド（Scope、Text 行、`Command` など）で 1 行化、書式制御文字の除去、実体参照化を通す。接頭辞での切り詰めは行わない |
| エンベロープ値 | 1 行化、書式制御文字の除去、実体参照化を通す。長さ上限も同じく適用する（実際の値はいずれも上限より短いが、経路を分けない） |
| 自由文 | 1 行化、書式制御文字の除去、実体参照化、長さ上限のすべてを適用する |
| 大量出力 | 既存の切り詰め規則（stdout 1000 文字、stderr 500 文字）のままとし、本タスクでは変更しない |

補間契約を実装する関数は、この役割を enum で受け取り、切り詰めの有無を `switch` で決める。ゼロ値は自由文とし、`default` も自由文と同じ最も強い加工へ倒す。値の内容から役割を推測しない。他節（AC-20、§3.1、§3.6）はこの契約を参照し、同じ規則を書き写さない。

##### 変換規則

上の性質を実現する規則は次のとおりである。

| 制約 | 実現方法 | 実現する性質 | 理由 |
|---|---|---|---|
| 改行と制御文字を含めない | 一般カテゴリ Cc の文字（U+0000〜U+001F と U+007F〜U+009F。C0、DEL、C1 をすべて含む）、および Unicode の行区切り U+2028 と段落区切り U+2029 を 1 文字ずつ半角空白へ置き換える | 1 行であること、制御文字と書式制御文字を含まないこと | 改行を通すと、本物の見出し行の直下に任意の行を作れる（§5.1 の脅威1）。範囲を C0 と DEL の列挙ではなくカテゴリ Cc で定めるのは、C1（U+0080〜U+009F）が C0 にも DEL にも Cf にも属さず、列挙では漏れるためである。とりわけ U+0085（NEL）は Unicode が改行として扱う文字であり、漏らせば 1 行の保証が破れる。Cc はこの 3 者を過不足なく含む。U+2028 と U+2029 は Cc に含まれないが、Unicode では改行として扱われる文字であり、同じ経路を与えてしまう |
| 書式制御文字を含めない | 一般カテゴリ Cf の文字を 1 文字ずつ半角空白へ置き換える | 制御文字と書式制御文字を含まないこと | 双方向表示制御の U+202A〜U+202E と U+2066〜U+2069 は Cc でも U+2028／U+2029 でもないため、前の規則を素通りする。これらは表示順を反転させるため、Scope や `Command` を実際とは別のコマンドを名乗るように見せられる。装飾が変わるだけの残余リスクとは別種の強さであり、受け入れない |
| Slack の制御構文を解釈させない | `&`→`&amp;`、`<`→`&lt;`、`>`→`&gt;` へ置き換える | 実体参照化されていること | `<!channel>` や `<@U012345>` がメンションとして、`<URL｜表示文字>` がリンクとして解釈されるのを防ぐ。裸の URL の自動リンクはこの規則の対象外であり、下の残余リスクで扱う |
| 長さに上限を設ける | 置き換え後の値を UTF-8 で 500 byte 以下に切り詰める | 長さが上限を超えないこと、有効な UTF-8 であること | Text 行はプッシュ通知に出る 1 行であり、長大な本文が入ると読めなくなる |

置き換えの順序は、**行区切りを含む制御文字 → 書式制御文字 → `&`・`<`・`>` → 切り詰め**に固定する。`&` の置き換えを先に行わないと、後段が入れた `&amp;` の `&` が二重に置き換えられる。

切り詰めは 500 byte を越えない最大の接頭辞を採る。ただし末尾が不完全な UTF-8 rune、またはこの処理が生成した `&amp;`・`&lt;`・`&gt;` の途中になってはならず、その場合は当該 rune または実体参照の全体を取り除く。切り詰め後の値は常に有効な UTF-8 であり、裸の `&am` や `&l` を末尾に残さない。上限を表す定数は自由文とエンベロープ値で共有する。

書式制御文字の置き換えは、絵文字を ZWJ（U+200D）で連結した文字列を構成要素へ分解する。共通エンベロープが置く ✅・⚠️・❌ は静的な部分であり契約を通さないため、STATUS の表示は変わらない。分解が起こりうるのは自由文と、書式制御文字を含む識別子だけであり、後者は設定の読み込みで拒否される（§3.1）。

**識別子を接頭辞で切り詰めない理由**。`command_group_summary` では、group 名が現れる場所は Scope フィールドだけである。ここで接頭辞を素朴に切ると、先頭 500 byte が一致する 2 つの group は同じ Scope として表示され、どちらの group の集計なのか通知から判別できなくなる。`validateGroupName` の `[A-Za-z_][A-Za-z0-9_]*` は文字種を縛るだけで長さを縛らないため、この状態は現在の設定検証を通る。「補正しない」に従い、長すぎる識別子は表示境界で黙って短くせず、設定の読み込みで拒否する（§3.1）。接頭辞の切り詰めは、識別子ではない自由文とエンベロープ値にだけ残す。

**識別子と既存の値まるごと redaction の関係**。`RedactingHandler` の `Config.RedactLogAttribute` は、キー名の判定と本文中の `key=value` 形式の置換に加えて、**値まるごと**を未アンカーの `SensitivePatterns.IsSensitiveValue`（`(?i)(password|token|secret|key|api_key)`、`bearer`、`basic`、`authorization` など）で判定する。語の一部に一致するため、`monkey`、`keyring`、`rotate_api_key` のような group 名や command 名は `[REDACTED]` へ置き換わり、Scope が発生箇所を指し示せなくなる。語の一致はこの経路の一部でしかない。`RedactLogAttribute` は `IsSensitiveValue` より先に `RedactText` を呼び、その中の `ValueDetector` が AWS アクセスキー ID、GitHub トークン、JWT、Webhook ホストの URL を値の形式だけで潰す。`AKIAIOSFODNN7EXAMPLE` のような名前はどの機密語も含まないまま `[REDACTED]` になる（§3.1）。

この過剰な置換を、識別子のキーだけ値まるごとの判定から外すことでは解消しない。`RedactingHandler` が包むのは Slack ハンドラ単体ではなく、JSON ログを含むすべての出力先を持つ `MultiHandler` である（`bootstrap/logger.go`）。除外を入れれば Slack だけでなく JSON ログのマスクも同時に弱まる。01_requirements.md の対象外は `internal/redaction` の適用範囲を変更しないと定めており、広げるにせよ狭めるにせよ本タスクでは触らない。

代わりに、設定の読み込みで拒否する（§3.1）。本番と同じ redaction の変換が値を書き換える group 名・command 名を持つ設定は、専用のセンチネルエラーで読み込みに失敗する。「補正しない」に従い、通知で `[REDACTED]` に潰れる名前を黙って通すことも、識別子だけ redaction を緩めることもせず、利用者に名前を変えてもらう。判定は語の一覧ではなく変換そのものを基準にし、redaction 側に置いた述語を呼ぶ（§3.1）。設定検証側へ機密語や値形式の一覧を複製しない。

**Hostname と Run ID も同じ契約を通す理由**。Hostname の値は `common.GetHostname()`（`os.Hostname`）の戻り値をそのまま載せており、どこでも検証していない。契約を通すことで、上の「出力の性質」をエンベロープ値についても処理として保証し、ホスト名を差し替えたテストで検証できるようにする（テスト機の実際のホスト名に依存させない）。ただしこれは注入ではなく一貫性の欠落である。ホスト名は本設計の脅威モデルが信用しない TOML の作成者ではなく、機械の管理者が決める値である。危険度を実際より大きく見せず、AC-20 が検証できる状態にすることが目的である。なお `###` は Slack の mrkdwn では見出しにならず、契約はこの 3 文字を対象にしない。AC-20 の `###` に関する主張は Text 行の骨格とフィールド見出しに限り、Hostname と Run ID の値については「出力の性質」を検証する。

**ホスト名を差し替える継ぎ目を `internal/logging` に置く**。`internal/common/system.go` の `osHostname` は非公開のパッケージ変数であり、同じパッケージのテストからしか差し替えられない。一方、上の性質を検証する対象は共通エンベロープが組み立てた Hostname フィールドの値であり、その組み立ては `internal/logging` にある。したがって `internal/common` 側の変数を差し替える形ではテストが書けない。`internal/common/system.go` と同じ書き方で、`internal/logging` にホスト名取得を指す非公開のパッケージ変数（初期値は `common.GetHostname`）を 1 個置き、テストはこれを入れ替える。共通エンベロープの Hostname はこの変数だけを経由し、現在 `slack_handler.go` の 5 箇所に散っている `common.GetHostname()` の直接呼び出しは残さない。`SlackHandlerOptions` へホスト名の項目を公開して渡す形は採らない。本番の呼び出し元が誰も設定しない拡張点になり、ホスト名の出どころが 2 つに割れるためである。

**書式の解釈を止めるのではなく、埋め込む値の側を書き換える理由**。Slack には「この項目だけ mrkdwn として解釈しない」という項目別の切り替えが無い。ペイロードの `mrkdwn` を false にすると、エンベロープ自身の `*STATUS*` の強調も同時に失われる。したがって補間される値の側を、Text 行へ置く前に無害化する。

**`*`、`_`、`~`、バッククォートは置き換えない**。これらを似た字形の別文字へ差し替えれば装飾は防げるが、利用者の group 名や command 名を黙って別の文字列に書き換えることになり、通知に出る名前が TOML の定義と一致しなくなる。「補正しない」という本設計の立場に反するうえ、名前による突き合わせを壊す。entity 変換と 1 行化で**偽装**リンク・メンション・偽の見出し行は防げ、書式制御文字の除去で表示順の反転も防げるため、**装飾だけが変わりうる点は残余リスクとして受け入れる**。装飾は文字の並びを偽らないが、表示順の反転は偽る。この差が、装飾を残して書式制御文字を残さない理由である。

**裸の URL の自動リンクも同じ理由で残余リスクとする**。Slack の mrkdwn は `https://evil.example` のような裸の URL を自動的にリンクにする。置き換え集合は `#` と同じくスキームや `://` を対象にしないため、この契約を通しても自動リンクは残る。防ぐには URL の綴りを壊すしかなく、それは利用者の command 名を別の文字列へ書き換えることであり、homoglyph 置換や `*`・`_` の置き換えを退けたのと同じ理由で採らない。受け入れられるのは、この場合に表示される文字列が遷移先そのものだからである。読み手は押す前に行き先を読める。契約が防ぐのは `<URL｜表示文字>` のように**表示文字が遷移先を偽る**リンクであり、そちらは `<` と `>` の実体参照化で確実に防がれる。したがって「リンクを作らせない」ではなく「偽装リンクを作らせない」が本契約の主張である。§7.3 の観点も同じ語で書く。

補間後に表示できる文字が 1 文字も残らない値は、埋め込む前に不正として扱う。Scope については §3.6 のとおり `(scope: invalid)` とし、コマンド名についてはそもそも §3.1 のとおり設定境界で拒否する。空白だけに潰れた名前を通知へ載せると、発生箇所を書いていない通知と見分けられなくなる。

大量出力の役割に当たる stdout と stderr の添付フィールド値は、既存の切り詰め規則のままとする。本タスクでこれらへ 1 行化と実体参照化を広げることはしない。既存の上限と redaction の契約を変えないためであり、識別子を載せるフィールドを範囲に含めることとは独立の判断である。

### 3.6 未知種別と不正スコープ

未知の `message_type`（空文字を含む）は、レコードの `Message` を要約とする汎用の種別固有部分へ変換する。共通エンベロープを必ず付ける。未知種別が WARN または ERROR の場合は高優先度、INFO の場合は通常優先度で送信する。同時に送信失敗ロガーへ、理由コード `unknown_message_type` を持つ WARN を記録する。WARN の書式と属性は本節末の共通規定に従う。

通知コンテキストの妥当性は、§3.1 のエンコードに対して次の順で判定する。

| 判定 | 条件 | 理由コード |
|---|---|---|
| 欠落 | 通知コンテキストのキーを持つ属性がない | `missing_notification_context` |
| 重複 | 同じキーの属性が 2 個以上ある | `duplicate_notification_context` |
| 不正 | 属性値が `slog.KindGroup` でない、`scope` が無い、`group` が無い、`scope`・`group`・`command` のいずれかの値が `slog.KindString` でない、`scope` が 3 語のいずれでもない、グループの中で `scope`・`group`・`command` のいずれかが 2 回以上現れる、group 名または command 名が §3.5 の補間契約を通すと表示できる文字を残さない、またはスコープと group／command の組が §3.1 の表に反する | `invalid_notification_context` |

下位キーの型を presence だけで済ませないのは、`slog.Value.String()` がどの種別の値でも文字列を返すためである。`group` に整数を持たせた手組みのレコードは、型を見なければ空でない group 名として復元され、スキーマ違反の WARN も出ないまま通ってしまう。§3.1 のエンコードが 3 個とも string と定めている以上、その型を読み側でも要求する。

同じ理由で、`group` は**存在すること**も要求する。§3.1 のエンコードは `group` を常に出力すると定めており（`command` だけが「空でないときだけ」である）、`group` を欠くグループは正規の `LogValue` からは生じない。ここを見ないと、`scope="global"` だけを持つ手組みのレコードが素通りする。欠けた `group` は空文字として復元され、それはグローバルが要求する値そのものであるため、組み合わせの検査にも掛からず、`(global)` が WARN 無しで表示されてしまう。すなわち偽装された正当なスコープになる。`scope="group"` で `group` を欠く場合は復元後に空となり既存の組み合わせ検査が捕まえるため、この穴はグローバルに固有である。存在の検査は `command` には課さない。エンコードが条件付き出力と定めており、不在が正常だからである。

グループの**外側**のキーが重複する場合と、グループの**内側**の下位キーが重複する場合は、検出する場所が違うだけで扱いは同じである。外側は SlackHandler の属性走査が `duplicate_notification_context` として検出し、内側は復元関数が `invalid_notification_context` として拒否する。内側の重複を「最初の値を採る」「最後の値を採る」のいずれかで通すことはしない。どちらの値が発生箇所なのかを補正で決める形になり、本設計の「補正しない」に反する。

`scope=global` で group と command がともに空の場合だけ `(global)` と表示する。通知コンテキストが不正でも、汎用メッセージには切り替えず、元の通知種別の固有部分を保ったまま Scope を `(scope: invalid)` とする。

WARN は 1 レコードにつき 1 件に固定する。未知種別と不正な通知コンテキストは同時に成立しうる。未登録の `message_type` を出す発火元は通知コンテキストも付けていないのが通例であり、2 件の WARN を出すとオンコール側の件数がレコード数と一致しなくなるためである。WARN のメッセージは `Slack notification schema violation` に固定し、検出した理由コードをすべて `reasons` 属性へ列挙する。理由が 1 個のときも要素 1 個の列挙とし、順序は「未知種別 → 通知コンテキスト」に固定する。WARN には `message_type`、`run_id`、ログレベル、宣言された scope、`webhook_label` を含め、通知本文、Webhook URL、group 名、command 名は含めない。group 名と command 名は利用者が入力できる値であり、診断に不要な本文の複製を避ける。

通知種別の照合と通知コンテキストの妥当性確認は、通常実行の `SlackHandler` 境界へ一元化し、送信機構が受付停止済みか確認する前に行う。これにより、終了処理との競合で通知が破棄された場合でも定義不備は記録される。ドライランまたは nil 送信機構の早期 return は Task 0163 の契約どおり先に行う。

一方、種別固有部分と共通エンベロープの構築は受付停止判定の後に行う。現在の `SlackHandler.Handle` は、受付停止済みの送信機構がすべての要求を破棄することを理由に、`buildCommandGroupSummary` のようなコマンド結果を全走査する構築を破棄経路で行わないよう、構築前に `isClosed` を確認している。検証だけを前へ出し構築は後ろへ残すことで、この既存の最適化を保ったまま定義不備の記録を得る。したがって処理順は「照合と検証（必要なら WARN）→ 受付停止判定 → 構築 → 投入」となる。

## 4. エラーハンドリング設計

### 4.1 エラー分類

| 状況 | Slack への結果 | 送信失敗ロガー | 呼び出し元への結果 |
|---|---|---|---|
| 未知の `message_type` | INFO は通常優先度、WARN 以上は高優先度で汎用メッセージを送る | 理由コードを含む WARN を 1 件 | `Handle` は既存どおり送信成否を返さない |
| 不正な通知コンテキスト | `(scope: invalid)` を含む元種別の通知を送る | 同上。未知種別と同時に成立しても WARN は 1 件 | `Handle` は既存どおり処理を続ける |
| キュー溢れ、受付停止、HTTP 失敗 | Task 0163 の既存契約に従う | 既存の記録 | 変更しない |

ログ処理の不備によってコマンド実行を失敗させない一方、正常な通知に見せかけない。WARN の固定メッセージと理由属性を用い、オンコール担当者が `run_id` と `message_type` から元の JSON ログを追跡できるようにする。

### 4.2 エラー型

本タスクは、呼び出し元へ返す新しいエラー型を導入しない。未知種別や不正スコープは送信経路で観測するスキーマ違反であり、`SlackHandler.Handle` の既存のエラー契約を変えずに WARN と通知表示で報告する。`PreExecutionError` の既存 `Error`、`Detail`、`Is`、`As`、`Unwrap` の契約も維持する。

## 5. セキュリティ考慮事項

### 5.1 脅威モデル

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    INPUT[("TOML の group 名<br>command 名 / ログ本文")]
    T1["脅威1: Slack の書式や<br>メンション構文として解釈される"]
    T2["脅威2: 不正スコープが<br>グローバル通知に見える"]
    T3["脅威3: 未知種別が<br>警告なしで汎用表示になる"]
    C1["対策1: 動的値を<br>表示境界でエスケープ"]
    C2["対策2: 不正な Scope の表示と<br>Slack に依存しない WARN"]
    C3["対策3: 単一の種別定義と<br>通知種別定義を走査するテスト"]
    LOG[("redaction 済み構造化ログ")]
    SLACK[("Slack 通知")]

    INPUT --> T1
    INPUT --> T2
    INPUT --> T3
    T1 --> C1
    T2 --> C2
    T3 --> C3
    C1 --> SLACK
    C2 --> SLACK
    C2 --> LOG
    C3 --> SLACK

    class INPUT,LOG,SLACK data
    class C1,C2,C3 enhanced
    class T1,T2,T3 problem
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    L1[("保護対象データまたは出力")]
    L2["追加する対策"]
    L3["脅威"]

    class L1 data
    class L2 enhanced
    class L3 problem
```

矢印 A → B は、脅威入力から脅威への辺では「A から B が生じる」こと、脅威から対策への辺では「B が A を抑制する」こと、対策から出力への辺では「A の適用後に B へ出力する」ことを表す。新しい通知コンテキストは権限判断やコマンド実行には使わず、表示と監査相関にだけ使うため、改ざんされても実行権限は拡大しない。

### 5.2 既存の保護との関係

- `internal/redaction` がログ境界で機密値を置換する現在の責務を維持する。本タスクの Slack 表示用エスケープは redaction の代替ではなく、その後段で書式制御文字だけを扱う。
- `internal/redaction` の適用範囲は変更しない。redaction の変換が書き換える識別子は、redaction を識別子だけ緩めるのではなく、設定の読み込みで拒否する（§3.1、§3.5）。判定用の述語を `internal/redaction` へ足すことは、既存の変換を読み取るだけであり適用範囲を変えない。`RedactingHandler` は Slack ハンドラだけでなく JSON ログを含む全出力先を包むため、除外は Slack 以外のマスクも弱める。
- Webhook URL、許可ホスト、HTTPS 検証、送信失敗ロガーが Slack に依存しない構成を変更しない。
- INFO は成功用 Webhook、WARN 以上はエラー用 Webhook という `SlackHandlerLevelMode` の振り分けを変更しない。
- キューの容量、優先処理、リトライ、送信期限、flush 期限、同期モードを変更しない。削除対象の高優先度種別を除いた後も `pre_execution_error` を高優先度に保つ。
- コマンドの stdout は 1000 文字、stderr は 500 文字という既存の切り詰め上限を維持する。

### 5.3 Slack での表示互換性

本設計では、既存の Incoming Webhook、トップレベルの `text`、legacy secondary attachment の `color` と `fields` だけを使用する。新しい Slack API 機能は導入しない。Slack は secondary attachment の `fields` を 2〜3 個以内にすることを推奨している。一方、現在のグループ集計はコマンドごとにフィールドを持つため、既にこの推奨数を超える場合がある。F-004 では stdout 1000 文字、stderr 500 文字の既存上限を据え置くため、本タスクでは集約上限やフィールド数を変更しない。コマンド数の多いグループやサイズの大きい種別固有メッセージが Slack に拒否される可能性は、既存のリスクとして残る。送信失敗は、既存の送信失敗ロガーが記録する HTTP エラーから確認できる。Text 行の動的値は 1 行化し `&`、`<`、`>` を entity へ変換するが、その他の mrkdwn 記号による装飾変化は残余リスクとして受け入れる。集約上限や Block Kit への移行は、利用者向けの挙動と AC を定めた別タスクで扱う。Slack の公式文書は 2026-09-08 に確認した。Incoming Webhook で通常のメッセージ書式を利用できること、トップレベルのメッセージでは `mrkdwn` が既定であること、`*bold*` が太字になること、`&`、`<`、`>` のエスケープが必要なことを確認している。

- [Sending messages using incoming webhooks](https://api.slack.com/messaging/webhooks)
- [Formatting message text](https://docs.slack.dev/messaging/formatting-message-text/)

対象クライアント環境は Slack のみである。実装時には `make slack-notify-test` と `make slack-group-notification-test` で、次の項目を確認する。

| 確認項目 | 期待 |
|---|---|
| Text 行の `*SUCCESS*` などの強調 | 太字として表示される |
| Text 行の `—`（em dash）と `[...]` | そのまま表示され、書式指定として解釈されない |
| プッシュ通知の表示 | Text 行が先頭から表示され、製品名が読み取れる |
| 添付の色 | `good` / `warning` / `danger` が従来どおり反映される |
| 添付フィールドの並び | 末尾 3 件が Scope、Hostname、Run ID の順である |

**設計時点ではこの実機確認は未実施である**。`###` が Slack の mrkdwn で見出しにならないことは要件定義の調査で確認済みであり、`*...*` は mrkdwn の基本記法であるため機能しない可能性は低いと判断しているが、判断であって確認ではない。上記を実装フェーズの完了条件に含める（§8.1 の Phase 7）。

強調が期待どおり表示されない環境があった場合の代替は、強調記法を外して素の文字列にすることである。Text 行の構造（製品名・絵文字・STATUS・Scope・要約の並び）は強調記法に依存しないため、この代替でも要件は満たせる。実サービスの検証を実行できない環境では、モックサーバーによるペイロード検証を必須とし、実表示未確認をリリース前の残存リスクとして記録する。

### 5.4 他の設計文書のポリシーとの関係

本設計は [Task 0163 §3.4](../0163_redaction_coverage_and_slack_async/02_architecture.md#34-slack-送信の非同期化f-004-f-005-f-006) の非同期送信ポリシーを維持する。とくに、メッセージはキュー投入前に構築すること、高優先度キューを先に処理すること、送信失敗と破棄を Slack に依存しないロガーへ記録すること、ドライランでは送信機構を作らないことを変更しない。

Task 0163 では `security_alert` と `privilege_escalation_failure` を高優先度としていた。本タスクは、どちらにも本番の呼び出し元がないという承認済み要件 F-001 に基づいて、この 2 種別を削除する。これは高優先度の意味を緩める例外ではなく、到達不能な定義を取り除く変更である。存続する `pre_execution_error` の優先度は変えない。古い種別を使う `TestSlackSender_HighPriorityBypassesFullNormalQueue` と `TestSlackSender_QueueOverflowDropsAndRecords` は `pre_execution_error` を使うよう更新し、優先度を通常へ倒すと失敗することを確認する。

削除後、高優先度種別は `pre_execution_error` だけになる。コマンド失敗を表す `command_group_summary` と `user_group_command_failure` は、大量発生によって高優先度キューを埋めないよう通常優先度に保つ。その結果、通常キューが飽和すると個々の失敗通知が落ちうる残余リスクを受け入れる。既存の送信失敗記録と flush 時の種別別集計を運用上の検知手段とする。

削除後、通常キューにあるコマンド失敗通知は大量の失敗によって破棄されうる。これらを高優先度へ移すと、氾濫する側が `pre_execution_error` を押し出しうるため移さない。破棄の個別記録と flush 時の種別別集計を運用上の担保とし、この残余リスクを関連文書にも反映する。

Task 0163 §3.4.1 は `slackSender` が `failureLogger` を所有すると定めている。本設計も所有権を移さず、`SlackHandler` は既存の `slackSender` を介して WARN を記録する。メッセージ構築を省く契約も、nil 送信機構、ドライラン、受付停止済み送信機構の 3 つすべてについて維持する。§3.6 と §6.1 が受付停止判定より前へ出すのは種別の照合と通知コンテキストの検証だけであり、`buildCommandGroupSummary` を含む構築は現在と同じく受付停止判定の後に置く。

## 6. 処理フロー詳細

### 6.1 Slack メッセージ構築フロー

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    START(["SlackHandler.Handle"])
    NOTIFY{"slack_notify が真?"}
    NOSENDER{"送信機構がある?"}
    LOOKUP{"message_type は登録済み?"}
    DEF["定義から組み立て関数と<br>優先度を選ぶ"]
    GENERIC["汎用の組み立て関数を選び<br>未知種別を理由に加える"]
    SCOPE{"通知コンテキストは妥当?"}
    VALID["宣言された Scope を採用"]
    INVALID["不正な Scope を採用し<br>理由に加える"]
    WARNLOG{"理由が 1 個以上?"}
    EMITWARN["送信失敗ロガーへ<br>WARN を 1 件記録"]
    CLOSED{"送信機構は受付中?"}
    DROP["既存の破棄記録"]
    BUILD["種別固有部分を構築"]
    ENVELOPE["ログレベルから表示を選び<br>共通エンベロープを付与"]
    QUEUE["確定済み優先度のキューへ投入"]
    DONE(["終了"])
    RECORD[("SlackMessage / slackRequest")]

    START --> NOTIFY
    NOTIFY -->|"いいえ"| DONE
    NOTIFY -->|"はい"| NOSENDER
    NOSENDER -->|"ない"| DONE
    NOSENDER -->|"ある"| LOOKUP
    LOOKUP -->|"はい"| DEF
    LOOKUP -->|"いいえ"| GENERIC
    DEF --> SCOPE
    GENERIC --> SCOPE
    SCOPE -->|"はい"| VALID
    SCOPE -->|"いいえ"| INVALID
    VALID --> WARNLOG
    INVALID --> WARNLOG
    WARNLOG -->|"はい"| EMITWARN
    WARNLOG -->|"いいえ"| CLOSED
    EMITWARN --> CLOSED
    CLOSED -->|"いいえ"| DROP
    CLOSED -->|"はい"| BUILD
    BUILD --> ENVELOPE
    ENVELOPE --> RECORD
    RECORD --> QUEUE
    DROP --> DONE
    QUEUE --> DONE

    class RECORD data
    class START,NOTIFY,NOSENDER,CLOSED,DROP,QUEUE,DONE process
    class LOOKUP,DEF,GENERIC,SCOPE,VALID,INVALID,WARNLOG,EMITWARN,BUILD,ENVELOPE enhanced
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    L1[("データ")]
    L2["変更しない既存処理"]
    L3["変更・追加する処理"]

    class L1 data
    class L2 process
    class L3 enhanced
```

矢印 A → B は「A の判定または処理の次に B を実行する」ことを表す。通知種別とスコープの検証を受付停止判定より前に置くため、終了時に破棄されたレコードでも定義不備の WARN が残る。一方で種別固有部分と共通エンベロープの構築は受付停止判定の後に置き、破棄経路で全コマンド結果を走査しない既存の性質を保つ。キュー投入以降の並行処理は Task 0163 のままであり、本タスクでは変更しない。

### 6.2 発火元ごとのデータ契約

| 発火元 | ログレベル | 通知コンテキスト | 種別固有データ |
|---|---|---|---|
| `runner.logGroupExecutionSummary` 成功 | INFO | `GroupScope(groupSpec.Name)` | コマンド結果、所要時間 |
| `runner.logGroupExecutionSummary` 失敗 | ERROR | `GroupScope(groupSpec.Name)` | コマンド結果、所要時間 |
| `logging.HandlePreExecutionError` の起動前エラー | ERROR | `GlobalScope()` | Error Type、Error Message、Component |
| `runner.executeGroups` の検証エラー | ERROR | `GroupScope(verErr.Group)` | Error Type、グループ名を除いた検証結果、Component |
| `audit.Logger.LogUserGroupExecution` の失敗 | ERROR | `CommandScope(cmd.GroupName(), cmd.Name())` | Command、Exit Code、redaction 済み stdout / stderr |

すべての発火元は `run_id` を既存どおり記録する。共通エンベロープの Run ID は、現在と同じく `SlackHandler` の生成時に設定した値を使う。

## 7. テスト戦略

### 7.1 単体テスト

| 観点 | 検証内容 | 対応 AC |
|---|---|---|
| 通知コンテキスト | ゼロ値と `GlobalScope()` がグローバルとして同じエンコードになり、属性欠落・重複・型違い・値の矛盾とは区別されることを検証する。`GroupScope("")` は構築でき、表示境界で `(scope: invalid)` と WARN になることも確認する | AC-09, AC-10, AC-12 |
| エンコードの往復 | 各スコープを `LogValue` して復元すると元のスコープに戻り、未知の `scope` 語と非グループ値は不正になる。`RedactingHandler` を挟んだ経路でも同じ判定になる | AC-10, AC-12 |
| 妥当性判定の全行 | §3.1 の判定表を行ごとに検証する。`scope`・`group`・`command` の値が文字列でない行を 3 個の下位キーそれぞれについて持ち、`Value.String()` によって数値などが名前として復元されないことを確認する。`scope="global"` だけを持ち `group` を欠くレコードが `(global)` にならず `invalid_notification_context` になる行を持ち、`group` の存在検査を外すとこの行が失敗する形にする。`command` を欠く group スコープは正常として通ることも同じ表で確認し、存在検査が `command` へ広がっていないことを示す。情報を欠く組み合わせ（group が空の group／command スコープ、command が空の command スコープ）に加え、情報を過剰に伴う組み合わせ（group または command を持つ global スコープ、command を持つ group スコープ）が `(global)` や `group=<名前>` へ落ちず `(scope: invalid)` になることを確認する。補間契約を通すと表示できる文字が残らない group 名・command 名（制御文字だけの名前）も `(scope: invalid)` になることを同じ表で確認する | AC-12 |
| 通知コンテキストの下位キーの重複 | グループの内側で `scope`、`group`、`command` のいずれかが 2 回以上現れるレコードが `invalid_notification_context` として拒否され、最初の値や最後の値を採って通らないことを、3 個の下位キーそれぞれについて検証する。グループの外側のキーの重複が `duplicate_notification_context` になることと区別して確認する | AC-12, AC-24 |
| レベル表示の全域性 | DEBUG と INFO・WARN の中間値を含む全レベルが表の 4 行のいずれかに一致し、`r.Level.String()` が表示へ漏れない | AC-19 |
| ゼロ値トークン | ゼロ値の `Notification` を渡すと汎用メッセージが送られ、`unknown_message_type` の WARN が残る（無送信にならない） | AC-24, AC-25 |
| 構築の遅延 | 受付停止済みの送信機構では種別固有部分を構築せず、それでも定義不備の WARN は残る | AC-24 |
| 発火元 | 存続する 3 種別の全発火点が通知コンテキストを持つ。SlackHandler 登録後のグローバルな起動前エラーでは `(global)` になる | AC-11, AC-13, AC-15, AC-17 |
| グループ検証エラー | Scope に group 名があり、Error Message に `Group: <name>, ` がない | AC-13, AC-14 |
| `RuntimeCommand` | コンストラクタへ渡した group 名を `GroupName` が返す | AC-16 |
| 設定の検証 | コマンド名が空の設定、制御文字や書式制御文字だけで表示できる文字を持たない設定、および長さ上限を 1 byte 超える group 名・command 名を持つ設定が、それぞれ別のセンチネルエラーで拒否されることを `errors.Is` で検証する。上限ちょうどの名前は通ることも同じ表で確認する。各検査を外すと対応する行が失敗する | AC-17 |
| 識別子を切り詰めない | 長さ上限ちょうどまでの group 名が Scope フィールドと Text 行に接頭辞ではなく全体として現れることを検証する。先頭が長く一致する 2 つの group 名が異なる Scope として表示されることも確認し、識別子へ切り詰めを入れると失敗する形にする | AC-13, AC-18 |
| 識別子と redaction | 語の一致で潰れる名前（`monkey`、`keyring`、`rotate_api_key`）と、値の形式だけで潰れる名前（`AKIAIOSFODNN7EXAMPLE`、`ghp_` で始まるトークン形、`eyJ` で始まる JWT 形）の双方を group 名・command 名に持つ設定が、redaction 用のセンチネルエラーで読み込みを拒否されることを `errors.Is` で検証する。どちらにも該当しない名前が通ることも同じ表で確認する。値の形式の行は `IsSensitiveValue` 単独では素通りすることを先に確かめ、検査が `ValueDetector` まで含んだ経路を通っていることを層として示す（`IsSensitiveValue` だけの実装へ戻すとこの行が失敗する）。さらに `slack_allowed_host` を設定した状態で、そのホストを含む URL 形の command 名が拒否されることと、同じ名前が許可ホスト未設定なら通ることを 1 組の行として持つ。既定の `NewConfig()` で検査する実装へ戻すとこの組が失敗する。TOML の許可ホストを大文字で書いた行も持ち、正規化前の値で検査する実装を落とす。あわせて同じ名前を値に持つ属性が `RedactingHandler` を通ると `[REDACTED]` になることを確認し、設定境界で拒否する理由が実在することを示す。この検査を外すと、通知の Scope が `[REDACTED]` になる | AC-13, AC-17 |
| レベル表示 | INFO、WARN、ERROR の絵文字、STATUS、色を全種別で検証する。各ビルダーが表示を上書きできないことも確認する | AC-18, AC-19 |
| 共通エンベロープ | 登録済みの通知種別定義を順に走査し、製品名、Text 形式、エンベロープの静的な部分（Text 行の骨格、フィールド見出し）における `###` の不在、末尾 3 フィールドの順序を検証する。Scope や要約へ補間される動的な値は対象にせず、下の「表示安全な補間契約」の行で扱う | AC-18〜AC-22, AC-26 |
| エンベロープ値の出力の性質 | §3.5 の継ぎ目（`internal/logging` の非公開パッケージ変数）を差し替え、改行・双方向表示制御・`<!channel>` を含むホスト名を返させたうえで、Hostname フィールドの値が §3.5 の「出力の性質」（1 行、制御文字と書式制御文字の不在、実体参照化、長さ上限、有効な UTF-8）をすべて満たすことを検証する。テスト機の実際のホスト名には依存させない。Hostname を契約から外すとこの行が失敗する | AC-20 |
| 予約フィールド見出し | `notificationDefinitions` を走査し、どの種別固有ビルダーの返すフィールドも Scope、Hostname、Run ID を見出しに使わないことを検証する。ビルダーの 1 つに予約見出しのフィールドを足すと失敗する | AC-21, AC-26 |
| 製品名 | 登録済み種別と汎用メッセージが同じ製品名で始まり、本番コード内の定義箇所が 1 つである | AC-33 |
| ユーザー／グループ指定コマンドの失敗 | 固有ビルダーが command 名、終了コード、Scope を表示する | AC-17, AC-23 |
| 識別子を載せる他フィールド | `user_group_command_failure` の `Command` フィールドと、`command_group_summary` がコマンド結果ごとに出す `Command` フィールドの**双方**について、`Scope: (global)` に相当する行を作る改行入りの command 名、`<!channel>` を含む command 名、双方向表示制御を含む command 名が、1 行化・書式制御文字の除去・実体参照化を経て、偽の行もメンションも表示順の反転も作らないことを検証する。どちらか一方だけを契約の対象にすると、対象外にした側の行が失敗する。group 集計側は合成値のため、`cmd.Name` の部分だけが識別子として処理され、終了コードと骨格が壊れないことも同じ行で確認する | AC-17, AC-20, AC-23 |
| 未知種別 | 空文字と未知文字列が汎用メッセージとして送られ、共通エンベロープと固定理由コードの WARN を持つ。WARN 以上は通常キューが満杯でも高優先度で送られる | AC-24, AC-25 |
| WARN の件数 | 未知種別と不正な通知コンテキストが同時に成立するレコードで WARN が 1 件だけ記録され、`reasons` に両方の理由コードが規定の順で並ぶ | AC-24 |
| 単一定義 | 登録関数が返すトークンと、そのトークンが参照する定義に、種別名、ビルダー、優先度がまとめて保持されることを検証する。`notificationDefinitions` を走査して種別名が一意であることも確かめる。構文木の静的契約テストで本番コードによる直接の `slack_notify=true` と `message_type` の構築、および登録済み token を返す公開アクセサ以外の引数を禁止する。同じ静的テストで `PreExecutionError` のリテラルが `NotificationContext` を省略していないことも検証する | AC-26, AC-27 |
| 優先度 | 通常キューを満たしても `pre_execution_error` が高優先度キューへ入り、先に送られる。優先度を通常へ変えると失敗する | AC-07, AC-27 |
| 削除対象の種別 | 本番コードを `rg` で検索し、対象の型、定数、関数、文字列がない | AC-01〜AC-03 |
| 特権監査 | `privilege.logElevationOutcome` が native root と `seteuid` の結果を記録し続けることを、新規テストで検証する。`logElevationOutcome` を対象とする既存テストは無いため、Phase 3 で追加する。呼び出し元から `logElevationOutcome` の呼び出しを取り除くと失敗する形にし、関数本体だけの検証にしない | AC-05 |
| 表示安全な補間契約 | §3.5 の 2 つの一覧から 1 個の表駆動テストを作る。入力側は「動的な値の一覧」の各行を役割ごとに代表させ、識別子（group 名、command 名）、エンベロープ値（Hostname、Run ID）、自由文（各種別の要約、Error Message、Component、Duration）を対象とする。フィールド名ではなく役割で範囲が決まることを行として持ち、値の途中に U+0000〜U+001F の各制御文字、DEL（U+007F）、C1 制御文字（とくに改行として扱われる U+0085）、U+2028、U+2029、双方向表示制御（U+202A〜U+202E、U+2066〜U+2069）、`&`、`<`、`>`、Slack のリンク・メンション形式、および裸の URL を置く。出力側は「出力の性質」の各行を共通の検査として当てる。裸の URL の行だけは、綴りが変わらないことを期待値とし、§3.5 の残余リスクと一致させる。置き換え集合から 1 文字を外すと、その文字の行が対応する性質の検査で失敗する。あわせて `notificationDefinitions` を走査し、各種別固有ビルダーが返すフィールドの値がすべて一覧のいずれかの行に対応することを検証する。一覧に無いフィールドを足すと失敗する | AC-18, AC-20, AC-33 |
| 自由文の切り詰め | 500 byte の境界直前・境界上・境界直後について、ASCII、複数 byte の rune、`&amp;`／`&lt;`／`&gt;` の各ケースが rune や実体参照の途中で切れず、常に有効な UTF-8 になることを検証する。同じ入力を識別子の役割で通すと切り詰められないことも確認し、役割の `switch` を取り違えると失敗する形にする | AC-18 |
| 切り詰めと redaction | stdout 1000 文字、stderr 500 文字の既存上限と、既存 redaction が維持される | F-004, F-007 |
| 宛先分離 | INFO は成功用、WARN と ERROR はエラー用ハンドラだけで有効になる | AC-31 |

F-001 で列挙された削除対象テストは、各種別を削除する直前と直後に `go tool cover -func` を取得し、存続する関数ごとに比較する。各削除コミットのメッセージへ比較結果を記録する。削除対象を 3 個の独立したコミットに分け、各コミットで `make test` と `make lint` を実行する（AC-04, AC-06, AC-30）。削除後に `make deadcode` も実行する（AC-08）。

F-002 から F-005 の各テストは、対象のコンストラクタ呼び出し、共通エンベロープ、レジストリ要素、優先度を一時的に壊して失敗することを確認し、その結果を該当コミットのメッセージへ記録する（AC-32）。

### 7.2 統合テスト

- `cmd/runner/integration_pre_execution_error_test.go` で、SlackHandler 登録後に起きるグローバルな起動前エラー（グローバル対象ファイルの検証失敗など）が `(global)` として Slack 用レコードへ伝わることを確認する。設定ファイルの読み込み・解析の失敗は登録前に起きて通知が発生しないため使わない（AC-15）。
- `internal/runner` の検証エラー経路で group 名が Scope に一度だけ現れることを確認する（AC-13, AC-14）。
- ユーザー／グループ指定コマンドを失敗させ、group 名、command 名、終了コードを持つ固有通知になることを確認する（AC-17, AC-23）。
- `internal/runner/e2e_slack_webhook_separation_test.go` で INFO と WARN 以上の宛先が変わらないことを確認する（AC-31）。
- `cmd/runner/integration_slack_flush_test.go` で新書式の通知が終了時の flush で失われないことを確認する。
- `docs/user/runner_command.ja.md` と英語版の対応を確認し、通知種別、書式、Scope、製品名の記載を静的に検証する（AC-28, AC-29）。

### 7.3 セキュリティテスト

- group 名、command 名、汎用メッセージ、Error Message に CR、LF、C0 制御文字、DEL、C1 制御文字（U+0085 を含む）、U+2028、U+2029、双方向表示制御（U+202A〜U+202E、U+2066〜U+2069）、`&`、`<`、`>`、Slack のリンク・メンション形式を含め、Text 行、Scope フィールド、識別子を載せる `Command` フィールド、自由文の添付フィールドが 1 行のままで、entity 変換後のペイロードが偽装リンク（表示文字が遷移先と異なるもの）、メンション、偽の見出し、表示順の反転を作らないことを確認する（§3.5 の表示安全な補間契約）。裸の URL については、自動リンクが残ること、およびその表示文字が URL そのものと一致することを期待値として書き、残余リスクを黙って通さない。URL の綴りを壊す実装を入れるとこの行が失敗する。
- 未知種別と不正スコープの WARN が送信失敗ロガーだけへ届き、新しい Slack 通知を再帰的に発生させないことを確認する。
- WARN に通知本文、group 名、command 名、Webhook URL が含まれないことを確認する。
- `go test -race ./internal/logging/...` で、通知種別定義の参照と既存の並行投入・flush に競合がないことを確認する。定義は実行中に変更しない。
- `--dry-run` で成功用・エラー用のどちらの Webhook にも HTTP リクエストが届かず、キューとワーカーも生成されない既存テストを維持する。

## 8. 実装優先順位

### 8.1 フェーズ分割

| Phase | 内容 | 完了条件 |
|---|---|---|
| 1 | `privileged_command_failure` の本番コードとテストを削除 | AC-01、AC-04、AC-06、AC-08、AC-30 を満たす独立コミット |
| 2 | `security_alert` の本番コードとテストを削除し、高優先度テストを `pre_execution_error` へ移す | AC-02、AC-04、AC-06〜AC-08、AC-30 を満たす独立コミット |
| 3 | `privilege_escalation_failure` の本番コードとテストを削除し、特権昇格結果ログが残ることを検証するテストを追加する | AC-03〜AC-06、AC-08、AC-30 を満たす独立コミット |
| 4 | 通知コンテキストと `RuntimeCommand.GroupName` を追加し、`cmd/runner` と `internal/runner/bootstrap` を含む全発火元へ伝搬する。あわせて空・表示できない・長すぎる・redaction の変換が書き換える識別子を設定の読み込みで拒否する（§3.1） | AC-09〜AC-17、AC-30、AC-32 |
| 5 | 通知種別定義、全発火元の属性生成関数への移行、ユーザー／グループ指定コマンド固有のビルダー、共通エンベロープ、役割ごとの補間契約、WARN を 1 個の取り消し可能なコミットで導入する | AC-18〜AC-27、AC-31〜AC-33 |
| 6 | `runner_command`、`security-architecture`、`slack_async_delivery`、`README`、`security-risk-assessment` の日本語版を更新し（§2.2）、各英語版へ `/mktrans` で翻訳を反映する | AC-28〜AC-30 |
| 7 | 全体検証と実 Slack 表示確認を行う | 全 AC、Success Criteria |

### 8.2 実装順の根拠

削除対象の種別を先に除くことで、通知種別定義と共通エンベロープは実際に発火する 3 種別だけを扱う。型の伝搬を表示変更より先に行うことで、各発火元の契約と Slack 表示の問題を分けて検証できる。文書は最終的な表示が確定してから更新する。削除する通知を記載した開発者向け設計書も同時に更新する。Phase 5 では発火元とハンドラを同じコミットで切り替え、既存のトップレベル `group` 属性は維持する。

統一書式では Text 行と添付フィールドの順序が変わるため、Slack ワークフロー、通知本文を解析する監視ルール、運用スクリプトに影響する破壊的変更となる。まず、リポジトリ内の利用箇所と文書を検索する。外部利用者には、リリースノートで新旧のペイロード例を示す。既存のトップレベル `group` 属性は維持し、追加される通知コンテキスト属性をリリースノートで示す。実際の Webhook はテスト用チャンネルで先に検証し、3 種別、未知種別、宛先分離を確認してから通常のチャンネルへ展開する。問題があれば Phase 5 の単一コミットを取り消す。製品名を固定する承認済み方針と YAGNI に従い、設定で旧書式へ切り替える機能は追加しない。

各 Phase では変更した Go コードに `make fmt` を適用し、`make test` と `make lint` を通す。Phase 3 後と最終 Phase では `make deadcode` を追加する。実サービスのテストに必要な Webhook はリポジトリへ保存しない。

## 9. 将来の拡張性

- 通知種別を追加するときは通知種別定義へ 1 要素を追加し、通知種別定義の集合を走査する共通契約テストを通す。別の `switch` や優先度一覧は追加しない。
- Scope に新しい階層を追加する場合は `NotificationScope` とコンストラクタ、妥当性、表示を同時に拡張する。文字列の内容から階層を推測しない。
- Slack Block Kit や別の通知先への抽象化は、具体的な要件と全対象クライアントでの検証が生じた時点で別タスクとして設計する。本タスクでは既存の Incoming Webhook ペイロードを維持する。
- 製品名や表示名の設定上書きは F-006 の範囲外である。チャンネルごとの識別が Scope と Hostname で足りないという運用上の証拠が得られた場合に検討する。

## 付録 A: Acceptance Criteria と設計の対応

| AC | 主な設計箇所 |
|---|---|
| AC-01〜AC-03 | §2.2、§3.4、§7.1、§8.1 |
| AC-04 | §7.1、§8.1 |
| AC-05〜AC-08 | §2.2、§3.4、§7.1、§8.1 |
| AC-09〜AC-12 | §3.1、§3.6、§7.1 |
| AC-13〜AC-17 | §3.2、§3.3、§6.2、§7.1〜§7.2 |
| AC-18〜AC-22 | §3.4〜§3.5、§7.1。AC-20 の動的な値と Hostname・Run ID の値は §3.5 の表示安全な補間契約が担う |
| AC-23〜AC-27 | §1.3、§3.4、§3.6、§6.1、§7.1 |
| AC-28〜AC-29 | §2.2、§7.2、§8.1 |
| AC-30〜AC-32 | §2.4、§5.2、§7、§8 |
| AC-33 | §3.5、§7.1 |

## 付録 B: 決定履歴

### B.1 通知種別定義を初期化時の 1 回きりの登録とする理由

通知種別は 3 個であり、実行時に追加・削除しない。当初は要素を一望できる素の配列を検討したが、§3.4 が必要とする「発火元が公開アクセサから受け取る opaque token」を配列だけでは型として与えられない。配列の添字や種別名の文字列を発火元へ渡すと、結局は種別を独立に列挙する第 2 の一覧が生まれ、AC-27 を満たせない。

そこで、定義の記述は 1 ファイル内の `var` 初期化に閉じ、非公開の登録関数が `message_type`、組み立て関数、優先度を 1 回だけ受け取って集合へ加え、その要素を指すトークンを返す形を採る。この形でも B.1 が避けたかった性質は残さない。定義は 1 箇所に並んで一望でき、テストは同じ集合を range でき、集合は初期化後に変更されないため競合状態を持たず（§7.3 の `-race` で確認する）、パッケージ外へ登録 API や再代入可能な token 変数を公開しないため、利用されない拡張点や実行中の書き換えも作らない。初期化順序への依存は、登録を同一パッケージの同一ファイルに閉じ、他パッケージの `init` から呼ばせないことで避ける。

### B.2 `switch` を 1 個にまとめる案を採用しない理由

単一の `switch` でも組み立て関数の選択は集約できるが、キュー優先度を同じ分岐から安全に返すための組を別途管理する必要がある。また、テストが全種別の通知種別定義の集合を順に走査する AC-26 を自然に満たせない。通知種別定義は名前、関数、優先度を 1 要素として扱えるため採用する。

### B.3 共通エンベロープを各ビルダーから呼ぶ案を採用しない理由

各ビルダーが共通関数を呼ぶ形では、呼び忘れや呼び出し後の上書きを型で防げない。F-004 と F-005 は全種別への一律適用と漏れの検知を求めるため、種別固有部分の戻り型から共通フィールドを除き、`SlackHandler.Handle` 側で必ず共通エンベロープを付ける。


### B.4 ゼロ値と属性欠落を区別する理由

AC-09 に従い `NotificationContext` のゼロ値はグローバルとする。一方、SlackHandler が受け取るレコードでは「ゼロ値を明示的に属性へ変換した状態」と「属性そのものが無い状態」を区別できる。後者までグローバルにすると AC-11 の付与漏れが正常に見えるため、属性欠落は不正とする。

### B.5 既存のトップレベル `group` 属性を残す理由

通知コンテキストは Slack のスコープを構造化するために追加する。既存の `GroupSummaryAttrs.Group` を削除すると、Slack 書式の変更に加えて JSON ログ利用者へ不要な破壊的変更を課す。SlackHandler は通知コンテキストだけを参照すれば推論禁止を守れるため、既存属性は互換性のため維持する。

### B.6 `RuntimeCommand` にグループ名を重複保持しない理由

`NewRuntimeCommand` が受け取るグループ名は、既に `TimeoutResolution.GroupName` に保持されている。通知専用の非公開フィールドを追加すると同じ値を同期する不変条件が増えるため、`GroupName()` は既存値を返す。

### B.7 Text 行を 1 行へ正規化する理由

製品名を先頭に固定しても、動的な値に改行があれば直下へ偽の見出し行を作れる。要件が要約を 1 行と定めていることにも合わせ、Text 行へ埋め込む値だけ CR、LF、制御文字を空白へ置換する。構造化ログや添付フィールドの元値は変えない。

### B.8 公開変数ではなく token アクセサを使う理由

フィールドが非公開の `Notification` でも、公開パッケージ変数は外部から再代入できる。登録済み token は非公開変数に保持し、公開関数が値を返す形にすることで、発火元が文字列を再列挙せず利用でき、実行中の書き換えも防げる。

### B.9 空の名前をコンストラクタの panic で拒否する案を採用しない理由

`GroupScope("")` と `CommandScope(group, "")` を事前条件違反として panic させる案を検討した。空の名前は検証済みの名前を渡すはずの発火点側の不具合であり、エラーを返して握り潰せる形にすべきではない、という論拠である。

採らなかったのは、通知コンテキストの構築がログ出力の途中に置かれるためである。ここで panic すると、エラーを報告している最中にプロセスを落とすことになり、報告そのものが失われる。ログ経路は、入力が壊れていても停止しないことが求められる側である。

代わりに、拒否を 1 段上流の設定境界へ移した（§3.1）。空のコマンド名は設定の読み込みで拒否され、そこを通った値が発火点で空になることは無い。それでも空が届いた場合は表示境界で `(scope: invalid)` と WARN として表に出る。「外部入力は読み込みで拒否し、内部の不整合は表示で検知し、その中間は停止させない」という 3 段の配置であり、panic は不要になる。

### B.10 組み立て関数に `*SlackHandler` を渡す案を採用しない理由

当初のスケッチは `messageBuilder` を `func(*SlackHandler, slog.Record) messageDetails` としていた。現在の各ビルダーが `SlackHandler` のメソッドであるため、その形を引き継いだものである。

採らなかったのは、AC-22 を型で守れなくなるためである。ハンドラを渡せば、ビルダーは Run ID も Hostname も製品名も手に取れる。すなわちエンベロープの要素を種別ごとに組み立て直すことが可能なままであり、「エンベロープの生成は 1 箇所」は規約でしか守られない。

引数をレコードだけにすると、ビルダーはエンベロープを組み立てる手段そのものを持たない。現行コードを確認したところ、存続する 3 ビルダーが receiver から読んでいるのは `s.runID` の 1 個だけであり、その Run ID は本設計で共通エンベロープの担当へ移る。したがってハンドラを渡さないことによる不足は生じない。副次的に、ビルダーはハンドラを構築せずに検証できる純粋な関数になる。
