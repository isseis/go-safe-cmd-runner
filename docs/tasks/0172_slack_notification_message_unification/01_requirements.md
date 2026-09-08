# 要件定義書: Slack 通知メッセージの書式統一とスコープ情報の付与

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-08 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#1092 Slack post for an error command should contain richer infomration](https://github.com/isseis/go-safe-cmd-runner/issues/1092)

## 背景

runner は正常終了時もエラー時も Slack に通知を送るが、メッセージの書式と内容が種別ごとに
ばらばらである。とくにエラー通知には TOML で定義した `group_name` が入らないため、通知を
受け取っただけではどこで失敗したのかが分からない。

### 通知の仕組み

`slack_notify=true` と `message_type` の 2 属性を持つ slog レコードだけが
[`SlackHandler.Handle`](../../../internal/logging/slack_handler.go#L293) で Slack メッセージに
変換される。`message_type` の値で分岐し、種別ごとの build 関数がメッセージを組み立てる。
レベルで宛先が分かれ、INFO は成功用 Webhook、WARN 以上はエラー用 Webhook に送られる。

### 現状の通知種別

HEAD を調査した結果、`message_type` は 6 種類定義されているが、**本番で発火するのは 3 種類
だけ**である。

| message_type | 本番の発火元 | Text 行 | group 名 |
|---|---|---|---|
| `command_group_summary` | [`runner.logGroupExecutionSummary`](../../../internal/runner/runner.go#L542) | `### ✅ SUCCESS backup` | あり |
| `pre_execution_error` | [`logging.handleErrorCommon`](../../../internal/logging/pre_execution_error.go#L113) | `🚨 Error: config_parsing_failed` | なし |
| `user_group_command_failure` | [`audit.Logger.LogUserGroupExecution`](../../../internal/runner/base/audit/logger.go#L121) | `ERROR: User/group command failed (Run ID: ...)` | なし |
| `security_alert` | **なし**（`LogSecurityEvent` に呼び出し元がない） | `🚨 Security Alert: ...` | なし |
| `privilege_escalation_failure` | **なし**（`LogPrivilegeEscalation` に呼び出し元がない） | `⚠️ Privilege Escalation Failed: ...` | なし |
| `privileged_command_failure` | **なし**（書き手そのものが存在しない） | `❌ Privileged Command Failed: ...` | なし |

死んだ 3 種別について、調査で確認した事実は次のとおり。

- `LogSecurityEvent` と `LogPrivilegeEscalation` は、`internal/` と `cmd/` を通しで検索しても
  定義と `_test.go` 以外に出現しない。
- `privileged_command_failure` は読み側の
  [`buildPrivilegedCommandFailure`](../../../internal/logging/slack_handler.go#L778) だけが存在し、
  書き側は
  [`logschema.go:62`](../../../internal/common/logschema.go#L62) のコメントが
  「Write side not yet implemented」と明記している。
- `LogPrivilegeEscalation` を削除しても特権昇格の監査は失われない。
  [`privilege/unix.go:339`](../../../internal/runner/base/privilege/unix.go#L339) の
  `logElevationOutcome` が昇格結果を Info で記録し、昇格の失敗は `WithPrivileges` の戻り値の
  エラーとして呼び出し元へ伝わるためである。

### 問題点

1. **スコープが構造化されていない。** group 名を属性として持つのは `command_group_summary`
   だけである。group 実行中の検証エラーは
   [`runner.go:429`](../../../internal/runner/runner.go#L429) で
   `"Group: %s, Total: %d, ..."` と文字列に埋め込んで渡しているため、Slack 上では Error
   Message の本文に埋もれる。
2. **グローバルなエラーと group のエラーを区別できない。** TOML 読み込み失敗のように group に
   紐付かないエラーと、group 実行中のエラーが同じ `🚨 Error: ...` の形で届く。受け手は
   group 名が無いのを「グローバルなエラーだから」と読むべきか「単に入っていないだけ」と読む
   べきか判断できない。
3. **書式が種別ごとに違う。** 先頭の絵文字（✅ / ❌ / ⚠️ / 🚨）に規則がなく、見出しの形も
   `### <STATUS> <group>`、`Error: <type>`、`<何か> Failed: <name>` と 3 通りある。添付
   フィールドの並びも種別ごとに違い、Run ID の位置が揃っていない。
4. **取りこぼしが黙って通る。** `user_group_command_failure` は message type の定数にも
   `Handle` の分岐にも登録されておらず、`default` の
   [`buildGenericMessage`](../../../internal/logging/slack_handler.go#L916) に落ちる。結果として
   1 行のテキストだけが送られ、色も Hostname も、失敗したコマンド名すら付かない。分岐の
   `default` が未登録種別を黙って受け流すため、この欠落は誰にも気付かれないまま残っていた。
5. **`###` は Slack で見出しにならない。** [`slack_handler.go:574`](../../../internal/logging/slack_handler.go#L574)
   が組み立てる `### ✅ SUCCESS backup` の `###` は、Slack の mrkdwn では書式指定として解釈
   されず、そのまま文字として表示される。
6. **並行リストが 3 本ある。** message type の定数一覧
   （[`slack_sender.go:59-63`](../../../internal/logging/slack_sender.go#L59)）、`Handle` の分岐、
   高優先度キューの判定（`isHighPriority`）が、それぞれ独立に種別を列挙している。問題 4 は
   このうち 2 本への登録漏れである。

## 目的

- どの通知を受け取っても、**どこで起きたのか**（グローバルか、どの group か、どのコマンドか）
  が Text 行と添付フィールドの双方から読み取れるようにする。
- 通知の見出し・色・末尾フィールドを 1 つの規則に統一し、種別が増えても崩れないようにする。
- 本番で発火しない通知種別を先に削除し、統一の対象を実際に届く 3 種別に絞る。
- 種別の登録漏れが黙って通る現在の構造をやめ、漏れがあれば必ず表に出るようにする。

## スコープ

### 対象

1. 本番の書き手がない 3 種別（`security_alert`、`privilege_escalation_failure`、
   `privileged_command_failure`）の削除。
2. 通知スコープ（グローバル／group／コマンド）を表す型の追加と、生きている 3 種別すべてへの付与。
3. group 名およびコマンド名の発火点までの伝搬。
4. 全種別に共通する見出し・色・末尾フィールドの統一。
5. `user_group_command_failure` の種別登録と、未登録種別の検知。
6. 利用者向けドキュメントへの通知書式の記載。

### 対象外

- **通知の宛先分離の変更。** 成功用とエラー用の Webhook を分ける現在の仕組み
  （`GSCR_SLACK_WEBHOOK_URL_SUCCESS` / `_ERROR`）と、レベルによる振り分けは変更しない。
- **送信経路の変更。** キュー、リトライ、flush、タイムアウトには手を入れない。
- **秘匿値のマスク処理。** `internal/redaction` の適用範囲は変更しない。
- **出力の切り詰め長。** stdout 1000 文字・stderr 500 文字という現在の上限は据え置く。
- **削除した 3 種別の再実装。** 特権昇格の監査やセキュリティイベントの通知を新たに配線する
  ことは、本タスクでは行わない。必要になった時点で別タスクとして起票する。
- **`HandleExecutionError` の Slack 通知化。** コマンド実行失敗は `command_group_summary` の
  エラー側で通知される現在の設計を維持する。

## 決定事項

### 削除は統一より先に、種別ごとに独立したコミットで行う

死んだ種別を残したまま書式を統一すると、動作を誰も確認できないコードに対して書式を合わせる
作業が発生する。先に削除して対象を 3 種別へ減らす。削除は種別ごとに 1 コミットとし、後から
1 件ずつ revert できる形にする。

### 通知スコープは型で宣言する

「group 名が空であること」に意味を持たせるのをやめ、スコープを明示的な値として運ぶ。
`internal/common` に、非公開フィールドとコンストラクタだけで構築する型を置く。

```go
type NotificationScope int

const (
    ScopeGlobal NotificationScope = iota // ゼロ値。group にも command にも紐付かない
    ScopeGroup
    ScopeCommand
)

// NotificationContext は Slack 通知レコードに必ず付与する発生箇所の情報。
type NotificationContext struct { /* 非公開フィールド */ }

func GlobalScope() NotificationContext
func GroupScope(group string) NotificationContext
func CommandScope(group, command string) NotificationContext
```

ゼロ値は `ScopeGlobal`、すなわち入力について最も仮定しない解釈になる。構築の入口を
コンストラクタに限ることで、group 名を入れ忘れた `ScopeGroup` の値をパッケージ外から作れなく
する。

### スコープの矛盾は補正せず、通知の上で表に出す

`ScopeGroup` や `ScopeCommand` でありながら group 名が空のレコードを受け取った場合、
グローバル扱いに落とさない。落とすと定義の誤りが正しい通知と見分けられなくなる。Scope の
表示を `(scope: invalid)` とし、あわせて送信失敗ロガーに WARN を記録する。

### 種別の登録漏れは構造で防ぐ（実現方式はアーキテクチャ設計で決める）

問題 6 の並行リストを廃し、種別の一覧・メッセージの組み立て・キュー優先度を単一の定義から
引く形にする。未知の種別を受け取ったときは黙って汎用メッセージに落とさず、WARN を記録する。
テストは種別定義のソース集合を range して全種別を検証し、種別を足したときに検証から漏れない
ようにする。

要件として定めるのはここまでである。具体的な実現方式（種別ごとの組み立て関数を値として持つ
レジストリにするのか、列挙型と `switch` の組み合わせにするのか）は `02_architecture.md` で
決める。F-005 の受け入れ基準も、特定の方式を前提としない書き方にしてある。

### 統一書式

Text 行（Slack のプッシュ通知に出る行）を次の形に統一する。

```
✅ *SUCCESS* — group=backup : 3 commands in 1.2s
❌ *ERROR* — group=backup command=pg_dump : command failed (exit 2)
❌ *ERROR* — (global) : config_parsing_failed
```

- 先頭の絵文字、`*STATUS*`、添付の色は**ログレベルだけ**で決まる。種別ごとの裁量をなくす。

  | ログレベル | 絵文字 | STATUS | 色 |
  |---|---|---|---|
  | INFO | ✅ | `SUCCESS` | `good` |
  | WARN | ⚠️ | `WARNING` | `warning` |
  | ERROR | ❌ | `ERROR` | `danger` |

- `—` の直後は必ずスコープを置く。グローバルな通知は空欄にせず `(global)` と明示する。
- `:` の後ろは種別ごとの要約（headline）を 1 行で書く。
- `###` は使わない。強調は Slack の mrkdwn である `*...*` で表す。

添付フィールドは、種別固有のフィールドを先に並べ、**末尾に必ず Scope・Hostname・Run ID の
3 件をこの順で置く**。Component は `pre_execution_error` にしか無いため、末尾の固定 3 件には
含めず種別固有フィールドとして扱う。

### group 名の伝搬経路

| 通知種別 | 伝搬方法 |
|---|---|
| `pre_execution_error` | `logging.PreExecutionError` にスコープの項目を追加する。`HandlePreExecutionError` は 4 個の位置引数をやめ、スコープを含む構造体を受け取る形に変える |
| `pre_execution_error`（group 検証エラー） | [`runner.go:429`](../../../internal/runner/runner.go#L429) は `verErr.Group` を文字列連結ではなくスコープとして渡す |
| `user_group_command_failure` | `RuntimeCommand` に group 名を保持させる。[`NewRuntimeCommand`](../../../internal/runner/base/runnertypes/runtime.go#L279) は既に `groupName` を引数で受け取っており（現状はタイムアウト解決のログにしか使わず捨てている）、保持と参照メソッドの追加だけで済むため、シグネチャの変更は不要 |
| `command_group_summary` | 既に group 名を持つ。スコープ型に載せ替える |

## 受け入れ基準（Acceptance Criteria）

#### F-001: 本番の書き手がない通知種別の削除

`security_alert`、`privilege_escalation_failure`、`privileged_command_failure` の 3 種別を、
書き側・読み側・スキーマ定義・テストのすべてから削除する。

**Acceptance Criteria**:
- **AC-01**: `privileged_command_failure` に関する production コード（message type 定数、
  `Handle` の分岐、`buildPrivilegedCommandFailure`、`common.PrivilegedCommandFailureAttrs`）が
  残っていない。
- **AC-02**: `security_alert` に関する production コード（`audit.Logger.LogSecurityEvent`、
  `common.SecurityAlertAttrs`、`common.SeverityCritical`、`common.SeverityHigh`、
  message type 定数、`Handle` の分岐、`buildSecurityAlert`、高優先度判定のエントリ）が
  残っていない。
- **AC-03**: `privilege_escalation_failure` に関する production コード
  （`audit.Logger.LogPrivilegeEscalation`、`common.PrivilegeEscalationFailureAttrs`、
  message type 定数、`Handle` の分岐、`buildPrivilegeEscalationFailure`、高優先度判定の
  エントリ）が残っていない。
- **AC-04**: AC-01〜AC-03 の削除がそれぞれ独立したコミットになっており、1 件ずつ revert できる。
- **AC-05**: `LogPrivilegeEscalation` の削除後も、`privilege` パッケージが特権昇格の結果
  （native root 実行／`seteuid` による昇格）を記録し続ける。
- **AC-06**: 削除に伴って消えるテスト（`TestLogger_LogSecurityEvent`、
  `TestLogSecurityEvent_Masking`、`TestLogSecurityEvent_DetailsRedaction`、
  `TestLogSecurityEvent_DetailsKeyCollisionPrevention`、`TestLogger_LogPrivilegeEscalation`、
  `TestLogPrivilegeEscalation_Masking`、および Slack 側の該当テストケース）について、
  `go tool cover -func` を削除の前後で比較し、存続する関数のカバレッジが下がっていないことを
  確認した記録がコミットメッセージにある。
- **AC-07**: 高優先度キューへの振り分けを検証するテストが、削除された `security_alert` では
  なく存続する高優先度種別で書かれており、優先度判定を常に低優先度へ倒すと失敗する。
- **AC-08**: 削除後に `make deadcode` が新たな到達不能コードを報告しない。

#### F-002: 通知スコープの構造化

通知の発生箇所を、文字列の有無からではなく明示的な型から読み取れるようにする。

**Acceptance Criteria**:
- **AC-09**: `common.NotificationContext` はコンストラクタ（`GlobalScope`、`GroupScope`、
  `CommandScope`）以外の方法ではパッケージ外から構築できない。ゼロ値のスコープは
  `ScopeGlobal` として扱われる。
- **AC-10**: `NotificationContext` のログ出力に `scope` と `group` が含まれ、コマンド名が
  無い場合は `command` 属性を出さない。
- **AC-11**: 生きている 3 種別すべての発火点が、送出するレコードに `NotificationContext` を
  付与する。
- **AC-12**: スコープが group またはコマンドでありながら group 名が空のレコードを受け取った
  場合、Scope の表示が `(scope: invalid)` になり、送信失敗ロガーに WARN が記録される。
  グローバル扱いには落とさない。

#### F-003: group 名とコマンド名の伝搬

エラー通知から、どの group・どのコマンドで起きたのかが分かるようにする。

**Acceptance Criteria**:
- **AC-13**: group 実行中の検証エラーによる `pre_execution_error` 通知に、その group 名が
  Scope として表示される。
- **AC-14**: AC-13 の通知の Error Message から、group 名の重複表示（`Group: <name>, ` の
  接頭辞）が取り除かれている。
- **AC-15**: 設定ファイルの読み込み失敗など group に紐付かないエラーの通知で、Scope が
  `(global)` と表示される。
- **AC-16**: `RuntimeCommand` が、生成時に渡された group 名を参照メソッドから返す。
- **AC-17**: user/group 指定コマンドの失敗通知に、group 名とコマンド名の双方が表示される。

#### F-004: メッセージ書式の統一

**Acceptance Criteria**:
- **AC-18**: 生きている 3 種別すべての Text 行が
  `<絵文字> *<STATUS>* — <スコープ> : <要約>` の形になっている。
- **AC-19**: 絵文字・STATUS・添付の色が「統一書式」の表のとおりログレベルだけで決まり、
  種別によって変わらない。
- **AC-20**: 生成されるどのメッセージにも `###` が含まれない。
- **AC-21**: 生きている 3 種別すべてで、添付フィールドの末尾 3 件が Scope、Hostname、
  Run ID の順に並んでいる。
- **AC-22**: Text 行と末尾 3 フィールドの生成が 1 箇所に集約されており、種別ごとの組み立て
  関数は種別固有のフィールドだけを返す。

#### F-005: 種別の取りこぼし防止

種別を単一の定義に集約する方式は「決定事項」に記したとおり `02_architecture.md` で決める。
以下の受け入れ基準は、特定の方式を前提としない観測可能な挙動として書いてある。

**Acceptance Criteria**:
- **AC-23**: `user_group_command_failure` が固有のメッセージとして組み立てられ、汎用メッセージに
  落ちない。失敗したコマンド名と終了コードが通知に含まれる。
- **AC-24**: 定義されていない `message_type` を持つレコードを受け取った場合、汎用メッセージを
  送るとともに、送信失敗ロガーに未知の種別として WARN が記録される。
- **AC-25**: 汎用メッセージも共通エンベロープ（色、Scope、Hostname、Run ID）を備える。
- **AC-26**: 全種別が共通エンベロープを満たすことを検証するテストが、種別を書き写した並行
  リストではなく種別定義のソース集合を range して書かれている。エンベロープを満たさない種別を
  1 つ足すと、そのテストが失敗する。
- **AC-27**: 種別の一覧、メッセージの組み立て、キュー優先度が単一の定義から引かれており、
  同じ種別集合を独立に列挙する箇所が他に無い。

#### F-006: 利用者向けドキュメント

**Acceptance Criteria**:
- **AC-28**: `docs/user/runner_command.ja.md` の Slack 通知の節に、通知される種別、統一書式、
  Scope の表示（`(global)` を含む）が記載されている。
- **AC-29**: AC-28 の内容が `docs/user/runner_command.md` に翻訳として反映されている。

#### F-007: 全体の健全性

**Acceptance Criteria**:
- **AC-30**: 各コミットの時点で `make test` と `make lint` が通る。
- **AC-31**: 削除・統一の前後で、通知の宛先分離（INFO は成功用 Webhook、WARN 以上はエラー用
  Webhook）の挙動が変わらない。
- **AC-32**: AC-11〜AC-26 を検証する各テストが、検証対象の挙動を壊すと失敗する（CLAUDE.md
  「Every test must be able to fail for its stated reason」）。確認したことをコミット
  メッセージに記す。

## Success Criteria（要件レベル）

- Slack に届くすべての通知から、グローバルか、どの group か、どのコマンドかが判別できる。
- 通知の見出し・色・末尾フィールドが 1 つの規則に従っており、種別ごとの例外が無い。
- 本番で発火しない通知種別が production コードに残っていない。
- 種別の登録漏れがテストで検知され、汎用メッセージへ黙って落ちることがない。
