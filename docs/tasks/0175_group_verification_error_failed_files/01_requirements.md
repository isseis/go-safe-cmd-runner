# 要件定義書: group 検証エラー通知での失敗ファイル名の表示

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

### global と group で失敗ファイルの提示が非対称

Task 0172 では、group ファイル検証の失敗を通知する経路が [`Runner.executeGroups`](../../../internal/runner/runner.go) にある。この経路は `verErr.Err`（センチネル `ErrGroupVerificationFailed`）を `Error: %v` に埋め込むだけで、失敗ファイルの一覧を持つ `verErr.Details` を使っていない。一方、グローバル検証エラーの経路は `err.Error()` を渡す。そのため [`verification.Error.Error()`](../../../internal/verification/errors.go) が `Details` を連結した文字列を返し、こちらはファイルパスを含む。Phase 7 の実チャンネル確認でこの非対称が判明した（同 §10 follow-up）。

### 実害

- Slack の group 検証エラー通知から、どのファイルが検証に失敗したかを判別できない。group 名は通知コンテキスト（Scope）で分かるが、失敗ファイルを特定するには別途ログを調べる必要がある。
- 同じファイル検証の失敗でも、グローバルはファイルパスが出て group は出ない。

### 失敗ファイルの出所は既にある

[`verification.Error`](../../../internal/verification/errors.go) は `Details []string` に失敗ファイルを持ち、`Error()` はそれを連結して返す。原因は group 側の通知経路がこの値を使っていないことにある。新たに失敗ファイルのデータを作り出す必要はない。

## 目的

- group 検証エラー通知に、失敗したファイルの一覧を表示する。
- group 名を本文やフィールドへ別個のメタデータとして重複させない。
- グローバルと group で、失敗ファイルの提示が対称になる。
- Task 0172 のメッセージ書式・通知種別定義・`error_type` を変えない。

## スコープ

### 対象

1. `Runner.executeGroups` の group 検証エラー分岐で、`verification.Error.Details` の失敗ファイル一覧を通知へ載せる。
2. 失敗ファイル一覧が表示上限に収まるときは全件を載せ、超えるときは上限内に収まる範囲と省略件数を載せる。
3. `Details` が空のときは、`Total`／`Verified`／`Failed` の件数と `verErr.Err` を含む既存の文言へフォールバックする。
4. 通知にファイル一覧が現れることを固定するテストを追加する。このテストは、`Details` を通知から落とす実装へ戻すと失敗する形にする。
5. 利用者向け文書（`docs/user/runner_command.ja.md` など）の group 検証エラー通知の表示が不足していれば、必要に応じて追記する。日本語版を先に更新し、英語版は `/mktrans` で反映する。

### 対象外

- **`verification.Error` の型・`Error()` の変更。** 既存の表現を使う。
- **グローバル検証エラー経路の変更。** 対称性は group 側を直して得る。
- **共通エンベロープ・メッセージ書式・通知種別定義の変更。**
- **失敗ファイルのパスに対する追加の redaction。** 既存の redaction 経路に委ねる。
- **`user_group_command_failure` 通知の配線。** 別タスク（0174）で扱う。

## 決定事項

### 失敗ファイルの出所は `verErr.Details` に一本化する

通知に載せるファイル一覧は `verification.Error.Details` だけから取る。`Total`／`Verified`／`Failed` の件数や別のログから作り直さない。件数と一覧が食い違う場合に、出所が 2 つあると同じ誤りを二重に直すことになるためである。

### group 名は Scope に一本化する

group 名を `Group: <name>, ` のような別個のメタデータとして本文・フィールドへ重複して表示しない。0172 の AC-13・AC-14 が定めた「group 名の構造的な表示場所を Scope にする」を維持する。失敗ファイルのパスに group 名と同名の文字列が含まれることはあるが、それはパスの内容であり、group 名の重複表示ではない。

### 空の `Details` は既存の文言へフォールバックする

`verification.Error.Details` が空のときは、現在の `Total: %d, Verified: %d, Failed: %d, Error: %v` の形を保つ。一覧が無いことを空文字や空フィールドで示さない。

### 表示は既存の表示安全な補間契約に従う

Error Message は動的な値であり、0172 の表示安全な補間契約を通る。したがって 1 行であること・制御文字と書式制御文字を含まないこと・長さが上限（500 byte）を超えないことは既存の仕組みで保たれる。失敗ファイル一覧がこの上限を超える場合に全件を表示することはできないため、上限内に収まる範囲のファイルパスを表示し、残りを省略したこととその件数を示す。上限超過時の振る舞いを、切り詰め任せにせず本タスクの要件として定義する。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 失敗ファイル名の表示

**Acceptance Criteria**:
- **AC-01**: 失敗ファイル一覧の表示が表示上限に収まるとき、group 検証エラー通知に `verification.Error.Details` の各ファイルパスが現れる。
- **AC-02**: 失敗ファイル一覧が表示上限（既存の表示安全な補間契約の 500 byte）を超えるとき、上限内に収まる範囲のファイルパスと、省略した件数が通知に現れる。
- **AC-03**: group 名は通知コンテキスト（Scope）に表示され、Error Message には `Group: <name>, ` のような別個のメタデータとして重複しない。失敗ファイルパスに偶然含まれる group 名の文字列は許容する。
- **AC-04**: `Details` が空のときは、`Total: N, Verified: N, Failed: N` と `verErr.Err` を含む既存の文言へフォールバックする。
- **AC-05**: グローバル検証エラー通知の失敗ファイルパスの表示は変わらない。

#### F-002: 配線をテストで固定する

**Acceptance Criteria**:
- **AC-06**: `Runner.Execute` を通したテストが、失敗ファイルパスが通知に載ることを検証する。上限超過時には省略件数が示されることも検証する。

#### F-003: 既存の挙動を変えない

**Acceptance Criteria**:
- **AC-07**: `error_type` は `group_file_verification_failed` のままで、共通エンベロープと通知種別定義が変わらない。
- **AC-08**: 通知のメッセージは既存の表示安全な補間契約（1 行、制御文字と書式制御文字を含まないこと、長さが上限を超えないこと）の性質を保つ。

#### F-004: 全体の健全性

**Acceptance Criteria**:
- **AC-09**: 各コミットの時点で `make fmt`（Go を変更した場合）・`make test`・`make lint` が通る。
- **AC-10**: 追加・変更したテストが、検証対象の挙動を壊すと失敗することを確認し、その旨をコミットメッセージに記す（CLAUDE.md「Every test must be able to fail for its stated reason」）。

## Success Criteria（要件レベル）

- group 検証エラー通知から失敗ファイルを判別できる。
- group 名が別個のメタデータとして本文へ重複しない（失敗ファイルパス内の同名文字列は除く）。
- グローバルと group の通知が失敗ファイルの提示で対称になる。
- メッセージ書式・通知種別定義・`error_type` が変わらない。
