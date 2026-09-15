# 要件定義書: group 検証エラー通知での失敗ファイル名の表示

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-12 |
| Review date | 2026-09-15 |
| Reviewer | `isseis` |
| Comments | - |

## 関連 Issue

- なし（Task 0172 [`03_implementation_plan.md`](../0172_slack_notification_message_unification/03_implementation_plan.md) §10「次のステップ」の follow-up から派生）

## 背景

### global と group で失敗ファイルの提示が非対称

Task 0172 では、group ファイル検証の失敗を通知する経路が [`Runner.executeGroups`](../../../internal/runner/runner.go) にある。この経路は `verErr.Err`（センチネル `ErrGroupVerificationFailed`）を `Error: %v` に埋め込むだけで、失敗ファイルの一覧を持つ `verErr.Details` を使っていない。一方、グローバル検証エラーの経路は `err.Error()` を渡す。そのため [`verification.Error.Error()`](../../../internal/verification/errors.go) が `Details` を連結した文字列を返し、こちらはファイルパスを含む。Phase 7 の実チャンネル確認でこの非対称が判明した（同 §10 follow-up）。

### 実害

- Slack の group 検証エラー通知から、どのファイルが検証に失敗したかを判別できない。group 名は通知コンテキスト（Scope）で分かるが、失敗ファイルを特定するには別途ログを調べる必要がある。
- 同じファイル検証の失敗でも、グローバルはファイルパスが出て group は出ない。
- 検証対象の収集に失敗した場合（例: コマンドのパス解決失敗）は、どのコマンドが原因かを通知から特定できない。`Details` が空のため `failed_file_paths` に何も載らず、通知に残るのは本文中のコマンド文字列だけである。

### 失敗ファイルの出所は既にある

[`verification.Error`](../../../internal/verification/errors.go) は `Details []string` に失敗ファイルを持ち、`Error()` はそれを連結して返す。原因は group 側の通知経路がこの値を使っていないことにある。新たに失敗ファイルのデータを作り出す必要はない。

### 収集失敗では対象名が通知されない

`VerifyGroupFiles` は検証対象を収集する段階でコマンドのパス解決に失敗すると、`Details` を持たない検証エラー（[`manager.go`](../../../internal/verification/manager.go) の収集失敗分岐）を返す。この経路の `verErr.Err` は `command.ExpandedCmd` を包むためコマンド文字列は `Message` に現れるが、`failed_file_paths` に載る一覧が無い。`Message` からパスを除く方針（対象 6）の下では、失敗した対象を通知から判別できなくなる。解決に失敗した対象は `collectVerificationFiles` の中で既に判明しているため、一覧の出所は `Details` に揃えたままこの経路へも渡せる。

## 目的

- group 検証エラー通知に、失敗したファイルの一覧を表示する。
- 検証対象の収集失敗でも、パス解決に失敗したコマンド（対象名）を一覧として表示する。
- 失敗ファイル一覧は自由文の本文へ連結せず、専用の構造化属性として運び、通知ビルダーが `Error Message` フィールドへ描画する。
- group 名を本文やフィールドへ別個のメタデータとして重複させない。
- グローバルと group の両方で、失敗ファイル一覧を同じ構造化属性 `failed_file_paths` として運び、同じ通知ビルダーが同じ予算管理で `Error Message` へ描画する。
- Task 0172 のメッセージ書式・通知種別定義・`error_type` を変えない（`Error Message` フィールドの値だけを組み立て直す）。

## スコープ

### 対象

1. `Runner.executeGroups` の group 検証エラー分岐で、`verification.Error.Details` の失敗ファイル一覧を通知へ載せる。
2. 失敗ファイル一覧が表示上限に収まるときは全件を載せ、超えるときは上限内に収まる範囲と省略件数を載せる。
3. `Details` が空のときは、`Total`／`Verified`／`Failed` の件数と `verErr.Err` を含む既存の文言へフォールバックする。
4. 通知にファイル一覧が現れることを固定するテストを追加する。このテストは、`Details` を通知から落とす実装へ戻すと失敗する形にする。
5. 利用者向け文書（`docs/user/runner_command.ja.md` など）の group 検証エラー通知の表示が不足していれば、必要に応じて追記する。日本語版を先に更新し、英語版は `/mktrans` で反映する。
6. 失敗ファイル一覧を `error_message` へ連結せず、専用の構造化属性（`common.PreExecErrorAttrs.FailedFilePaths` = `failed_file_paths`）として記録する。`Message` は件数とセンチネルのみでパスを含めず、`handleErrorCommon` が stderr へ書く文字列もパスを含めない（stdout には `handleErrorCommon` は `Message` を書かない）。
7. `pre_execution_error` の `Error Message` フィールドは、`error_message` 属性の値と `failed_file_paths` から通知ビルダーが組み立てる。各パスは `strconv.Quote` で引用・エスケープした表示形（`"`・`\`・制御文字・書式制御文字・行区切り・不正な UTF-8 バイトを可視のエスケープにする）とする。上限内は全件、超える場合は上限内に収まる範囲（丸ごと優先・省略記号付き切り詰め）と省略件数を示す。
8. `failed_file_paths` の各要素が、値形式の機密（トークン等）はマスクし、`key` などを通常含むパス（例: `/opt/monkey/data`）はマスクしないことを回帰で固定する。これは文字列スライス属性に対する既存の redaction 挙動であり、redaction の実装は変更しない。
9. `cmd/runner/main.go` のグローバル検証エラー報告でも `verification.Error.Details` を `failed_file_paths` として設定し、`Message` からはパスを除く。失敗ファイル一覧は通知ビルダーが group と同じ予算管理で `Error Message` へ描画する。
10. 検証対象の収集失敗（コマンドのパス解決失敗）で、解決に失敗した対象を `verification.Error.Details` に設定し、`failed_file_paths` として通知へ表示する。1 件目で打ち切らず、解決に失敗した対象を全て載せる。
11. 収集失敗の `Message` から対象名を除く。`Err` の文言には対象名を含めず、パス解決の生の原因は検証マネージャの既存の構造化ログに残す。

### 対象外

- **`verification.Error` の型・`Error()` の変更。** 既存の表現を使う。パスを含まないセンチネルの追加はこれに含まない。グローバルは発行元でパスを含まない `Message` を組み立てる。
- **共通エンベロープ・メッセージ書式・通知種別定義の変更。** 新しい Slack フィールドは足さず、既存の `Error Message` の値だけを組み立て直す。
- **redaction の適用範囲の変更。** 既存の `RedactText` と既存の `processSlice` の挙動をそのまま使う（対象 8 は回帰固定のみ）。
- **`user_group_command_failure` 通知の配線。** 別タスク（0174）で扱う。

## 決定事項

### 失敗ファイルの出所は `verErr.Details` に一本化する

通知に載せるファイル一覧は `verification.Error.Details` だけから取る。`Total`／`Verified`／`Failed` の件数や別のログから作り直さない。件数と一覧が食い違う場合に、出所が 2 つあると同じ誤りを二重に直すことになるためである。

### 収集失敗も `Details` に一本化する

検証対象の収集失敗（コマンドのパス解決失敗）でも、解決に失敗した対象の一覧を `verification.Error.Details` に設定する。通知へ載せる一覧の出所は常に `Details` であり、収集失敗だけ別経路にしない。`Details` が空のまま残るのは、`*verification.Error` 以外の失敗と、失敗対象を持たない検証エラーに限る。

収集失敗の `Message` には対象名を入れない。`Err` にはパスを含まないセンチネル（"failed to collect verification files"）を用い、パス解決の生の原因は検証マネージャの既存の構造化ログに残す。件数は、対象総数（`verify_files` とコマンド数の合計）・検証済み 0・解決に失敗した対象数とする。

### group 名は Scope に一本化する

group 名を `Group: <name>, ` のような別個のメタデータとして本文・フィールドへ重複して表示しない。0172 の AC-13・AC-14 が定めた「group 名の構造的な表示場所を Scope にする」を維持する。失敗ファイルのパスに group 名と同名の文字列が含まれることはあるが、それはパスの内容であり、group 名の重複表示ではない。

### 空の `Details` は既存の文言へフォールバックする

`verification.Error.Details` が空のときは、現在の `Total: %d, Verified: %d, Failed: %d, Error: %v` の形を保つ。一覧が無いことを空文字や空フィールドで示さない。収集失敗は `Details` を持つようになるため、このフォールバックが使われるのは失敗対象を持たない経路に限られる。

### 表示は既存の表示安全な補間契約に従う

Error Message は動的な値であり、0172 の表示安全な補間契約を通る。したがって 1 行であること・制御文字と書式制御文字を含まないこと・長さが上限（500 byte）を超えないことは既存の仕組みで保たれる。失敗ファイル一覧がこの上限を超える場合に全件を表示することはできないため、上限内に収まる範囲のファイルパスを表示し、残りを省略したこととその件数を示す。上限超過時の振る舞いを、切り詰め任せにせず本タスクの要件として定義する。

### 失敗ファイル一覧は構造化属性で運び、ビルダーが描画する

失敗ファイル一覧を自由文 `Message` へ連結しない。専用属性 `failed_file_paths`（`[]string`）として運び、`Error Message` フィールドの値は通知ビルダーが `Message` と `failed_file_paths` から組み立てる。これにより、パスは redaction の外にある stderr（`handleErrorCommon` の出力）へ届かず、`error_message` という自由文フィールドの意味も変えない。新しい Slack フィールドは足さず、`Error Message` の中身だけを組み立て直す。

### グローバルと group は同じ属性・同じビルダーを使う

グローバル検証エラーの報告（`cmd/runner/main.go`）も group と同じ `failed_file_paths` を設定し、`Message` からパスを除く。通知ビルダーは `error_type` を問わず `failed_file_paths` を読むため、両者とも同じ予算管理（全件、または上限内の範囲と省略件数）で `Error Message` に一覧が描画される。グローバルは従来 `err.Error()` の切り詰めに委ねていたが、これで切り詰めと省略件数の扱いが group と揃う。

### 文字列スライス要素の redaction は既存挙動を回帰で固定する

`failed_file_paths` の要素はファイルパス（データ）であり、`RedactingHandler.processSlice` は文字列要素に `RedactText`（値形式検出と key=value 置換）だけを適用し、値全体置換は行わない（`internal/redaction/redactor.go:1436-1439`）。この挙動は既に存在するため、本タスクは redaction を変更せず、機密がマスクされ普通のパスが残ることをテストで固定する。`error_message`（`KindString`）の値全体置換は従来どおりである。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 失敗ファイル名の表示

**Acceptance Criteria**:
- **AC-01**: 失敗ファイル一覧の表示が表示上限に収まるとき、検証エラー通知の `Error Message` フィールドに `verification.Error.Details` の各ファイルパスが（区切りと衝突しないエンコード後の表示形で）現れる。
- **AC-02**: 失敗ファイル一覧が表示上限（既存の表示安全な補間契約の 500 byte）を超えるとき、上限内に収まる範囲のファイルパスと、省略した件数が `Error Message` に現れる。
- **AC-03**: group 名は通知コンテキスト（Scope）に表示され、Error Message には `Group: <name>, ` のような別個のメタデータとして重複しない。失敗ファイルパスに偶然含まれる group 名の文字列は許容する。
- **AC-04**: `Details` が空のときは、`Total: N, Verified: N, Failed: N` と `verErr.Err` を含む既存の文言へフォールバックする。
- **AC-05**: グローバル検証エラー通知も `failed_file_paths` 属性を使い、group と同じ予算管理（全件、または上限内に収まる範囲と省略件数）で `Error Message` に失敗ファイル一覧を描画する。

#### F-002: 配線をテストで固定する

**Acceptance Criteria**:
- **AC-06**: `Runner.Execute` を通したテストが、通知レコードの `failed_file_paths` 属性に失敗ファイルパスが入り、`Error Message` に描画されることを検証する。上限超過時には省略件数が示されることも検証する。

#### F-003: 既存の挙動を変えない

**Acceptance Criteria**:
- **AC-07**: `error_type` は `group_file_verification_failed` のままで、共通エンベロープと通知種別定義が変わらない。
- **AC-08**: 通知のメッセージは既存の表示安全な補間契約（1 行、制御文字と書式制御文字を含まないこと、長さが上限を超えないこと）の性質を保つ。

#### F-004: 全体の健全性

**Acceptance Criteria**:
- **AC-09**: 各コミットの時点で `make fmt`（Go を変更した場合）・`make test`・`make lint` が通る。
- **AC-10**: 追加・変更したテストが、検証対象の挙動を壊すと失敗することを確認し、その旨をコミットメッセージに記す（CLAUDE.md「Every test must be able to fail for its stated reason」）。

#### F-006: 構造化された失敗ファイル一覧

**Acceptance Criteria**:
- **AC-14**: グローバル／group 検証エラーの通知レコードは、失敗対象（失敗ファイルまたは収集で解決に失敗したコマンド）を持つとき専用属性 `failed_file_paths` にそれを記録し、`handleErrorCommon` が stderr へ書く `Message` はパスを含まない（検証マネージャが各失敗ファイルを別途ログする経路と、console ハンドラが属性を描画する点は残存リスク）。
- **AC-15**: `failed_file_paths` の各要素は、値形式の機密（トークン等）がマスクされ、`key` などを通常含むパス（例: `/opt/monkey/data`）はマスクされない。これは文字列スライス属性に対する既存の redaction 挙動であり、本タスクは redaction を変更しない。
- **AC-16**: グローバル検証エラーの通知も、`Runner.Execute` ではなく `cmd/runner` の報告境界を通したテストで、`failed_file_paths` が `Error Message` に描画されることを固定する。
- **AC-17**: 検証対象の収集失敗（コマンドのパス解決失敗）でも、解決に失敗した対象が全て `failed_file_paths` に記録され、`Error Message` に表示される。`HandlePreExecutionError` が記録する `Message` と `handleErrorCommon` が stderr へ書く文字列は対象名を含まず、対象名は検証マネージャの構造化ログに残る。

## Success Criteria（要件レベル）

- group 検証エラー通知から失敗ファイルを判別できる。
- 検証対象の収集失敗でも、原因のコマンド（対象名）を通知から判別できる。
- 失敗ファイル一覧は構造化属性として運ばれ、通常の `key` などを含むパスが失敗しても通知は消えない。`handleErrorCommon` の stderr 出力にパスが出ない。
- group 名が別個のメタデータとして本文へ重複しない（失敗ファイルパス内の同名文字列は除く）。
- グローバルと group の通知が、同じ構造化属性と同じ予算管理による失敗ファイルの提示で対称になる。
- メッセージ書式・通知種別定義・`error_type` が変わらない（`Error Message` の値の組み立てだけを変える）。
