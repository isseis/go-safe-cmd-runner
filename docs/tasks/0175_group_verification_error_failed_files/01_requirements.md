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

### `runerrors` は本番で呼ばれていない

共有コンストラクタの置き場所に予定している [`internal/runner/runerrors`](../../../internal/runner/runerrors/) の既存シンボル（`ClassifiedError`・`ClassifyVerificationError`・`LogClassifiedError`・`LogCriticalToStderr`）は、テスト以外に呼び出し元が 1 つもない。ここへ `NewVerificationPreExecutionError` だけを足すと、生きた関数 1 つと死んだ型・関数 4 つが同じパッケージに同居し、パッケージの説明「error classification and handling」も実態と合わなくなる。

### `verification.Error` の生成箇所は 3 つある

`Details` の昇順正規化を「`Error` を生成する唯一の場所」で行う方針に対し、実際の生成は [`manager.go`](../../../internal/verification/manager.go) の 3 箇所（グローバル検証失敗・group 収集失敗・group 検証失敗）の構造体リテラルである。3 箇所それぞれで並べ替えを書くと、規約頼みの状態に戻る。

### `Component` のリテラルが混在している

`PreExecutionError.Component` / `ExecutionError.Component` には `string(resource.ComponentVerification)` のような typed 定数経由の値と、`"main"`（[`cmd/runner/main.go`](../../../cmd/runner/main.go) 4 箇所）・`"runner"`（`main.go` の `ExecutionError` と [`runner.go`](../../../internal/runner/runner.go) の group 検証分岐）の生リテラルが混在する。本タスクは group の `Component` を `verification` へ変えるため、同じ箇所を触る。

### 収集失敗では対象名が通知されない

`VerifyGroupFiles` は検証対象を収集する段階でコマンドのパス解決に失敗すると、`Details` を持たない検証エラー（[`manager.go`](../../../internal/verification/manager.go) の収集失敗分岐）を返す。この経路の `verErr.Err` は `command.ExpandedCmd` を包むためコマンド文字列は `Message` に現れるが、`failed_file_paths` に載る一覧が無い。`Message` からパスを除く方針（対象 6）の下では、失敗した対象を通知から判別できなくなる。解決に失敗した対象は `collectVerificationFiles` の中で既に判明しているため、一覧の出所は `Details` に揃えたままこの経路へも渡せる。

## 目的

- group 検証エラー通知に、失敗したファイルの一覧を表示する。
- 検証対象の収集失敗でも、パス解決に失敗したコマンド（対象名）を一覧として表示する。
- 失敗ファイル一覧は自由文の本文へ連結せず、専用の構造化属性として運び、通知ビルダーが `Error Message` フィールドへ描画する。
- group 名を本文やフィールドへ別個のメタデータとして重複させない。
- グローバルと group の両方で、失敗ファイル一覧を同じ構造化属性 `failed_file_paths` として運び、同じ通知ビルダーが同じ予算管理で `Error Message` へ描画する。
- グローバルと group の検証エラー報告を共有コンストラクタ 1 箇所で組み立て、本文テンプレート（件数 + `Err`）と `Component` を揃える。
- 失敗ファイル一覧の並びは `verification.Error` の生成時に昇順へ正規化し、発火元ごとの並べ替えをなくす。
- 通知種別定義・フィールド集合・`error_type` を変えない（`Error Message` の値と group の `Component` を組み立て直す）。

## スコープ

### 対象

1. `Runner.executeGroups` の group 検証エラー分岐で、`verification.Error.Details` の失敗ファイル一覧を通知へ載せる。
2. 失敗ファイル一覧が表示上限に収まるときは全件を載せ、超えるときは上限内に収まる範囲と省略件数を載せる。
3. 本文テンプレートは `verErr.Err` の種別で選び（収集失敗のセンチネルなら収集段階の件数、それ以外は `Total`／`Verified`／`Failed` の件数と `verErr.Err`）、`Details` の有無では変えない。`Details` が空のときは `Files:` 節だけを付けず、グローバルと group で同じ規則にする。
4. 通知にファイル一覧が現れることを固定するテストを追加する。このテストは、`Details` を通知から落とす実装へ戻すと失敗する形にする。
5. 利用者向け文書（`docs/user/runner_command.ja.md` など）の group 検証エラー通知の表示が不足していれば、必要に応じて追記する。日本語版を先に更新し、英語版は `/mktrans` で反映する。
6. 失敗ファイル一覧を `error_message` へ連結せず、専用の構造化属性（`common.PreExecErrorAttrs.FailedFilePaths` = `failed_file_paths`）として記録する。`Message` は件数とセンチネルのみでパスを含めず、`handleErrorCommon` が stderr へ書く文字列もパスを含めない（stdout には `handleErrorCommon` は `Message` を書かない）。
7. `pre_execution_error` の `Error Message` フィールドは、`error_message` 属性の値と `failed_file_paths` から通知ビルダーが組み立てる。各パスは `strconv.Quote` で引用・エスケープした表示形（`"`・`\`・制御文字・書式制御文字・行区切り・不正な UTF-8 バイトを可視のエスケープにする）とする。上限内は全件、超える場合は上限内に収まる範囲（丸ごと優先・省略記号付き切り詰め）と省略件数を示す。
8. `failed_file_paths` の各要素が、値形式の機密（トークン等）はマスクし、`key` などを通常含むパス（例: `/opt/monkey/data`）はマスクしないことを回帰で固定する。これは文字列スライス属性に対する既存の redaction 挙動であり、redaction の実装は変更しない。
9. `cmd/runner/main.go` のグローバル検証エラー報告でも `verification.Error.Details` を `failed_file_paths` として設定し、`Message` からはパスを除く。本文・`Component`・一覧は group と同じ共有コンストラクタで組み立て、通知ビルダーが同じ予算管理で `Error Message` へ描画する。
10. 検証対象の収集失敗（コマンドのパス解決失敗）で、解決に失敗した対象を `verification.Error.Details` に設定し、`failed_file_paths` として通知へ表示する。1 件目で打ち切らず、解決に失敗した対象を全て載せる。
11. 収集失敗の `Message` から対象名を除く。`Err` の文言には対象名を含めず、パス解決の生の原因は検証マネージャの既存の構造化ログに残す。
12. グローバルと group の検証エラー報告（`logging.PreExecutionError` の組み立て）を `internal/runner/runerrors` の共有コンストラクタ `NewVerificationPreExecutionError` に一本化する。本文は両者同じテンプレート（`Total: %d, Verified: %d, Failed: %d, Error: %v`、収集失敗は `Collection failed: %d of %d targets unresolved, Error: %v`）とし、`Component` は両者 `verification` にする。
13. `verification.Error.Details` は Manager が `Error` を生成する時点で昇順に正規化する。通知の発火元は並べ替えず、正規化済みの `Details` から共有コンストラクタが複製する。
14. `internal/runner/runerrors` の本番呼び出しの無い既存シンボル（`ClassifiedError`・`ErrorSeverity`・`ErrorType`・`ClassifyVerificationError`・`LogClassifiedError`・`LogCriticalToStderr`）とそのテストを削除し、パッケージの説明を共有コンストラクタの責務に合わせる。README.ja.md / README.md のパッケージ一覧の表記も更新する（英語版は `/mktrans`）。
15. `verification.Error` の生成を `manager.go` の非公開コンストラクタ 1 箇所に集約し、対象 13 の昇順正規化はそのコンストラクタだけが行う。3 つの発生箇所（グローバル検証失敗・group 収集失敗・group 検証失敗）はすべてこれを経由する。
16. `PreExecutionError.Component` / `ExecutionError.Component` に渡す生リテラル `"main"`・`"runner"` を `string(resource.ComponentMain)` / `string(resource.ComponentRunner)` に置き換える。

### 対象外

- **`verification.Error` の型・`Error()` の変更。** 既存の表現を使う。パスを含まないセンチネルの追加はこれに含まない。対象 13 の昇順正規化は `Details` の値の並びを変えるため、`Error()` が `Details` を連結する順も昇順になる（`Error()` の実装は変えない）。グローバルも共有コンストラクタでパスを含まない `Message` を受け取る。
- **共通エンベロープ・通知種別定義・Slack フィールド集合の変更。** 新しい Slack フィールドは足さず、既存 `Error Message` の値と group の `Component` だけを組み立て直す（本文テンプレートは group の既存形へ統一する）。
- **`error_type` の統一。** グローバルは `file_access_failed`、group は `group_file_verification_failed` のままとする（0172 の通知種別定義。報告の起点が異なるための意図的な差）。
- **失敗対象一覧を持たない検証失敗の報告形式。** `ensureHashDirectoryValidated` が返す `*verification.OpError` など、`Details` を持たない失敗は本タスクの一覧契約の対象外とし、global は従来どおり `err.Error()`、group は既存の system_error 経路のままとする（アーキテクチャ §5.5・§9 に残存として記録、[#1154](https://github.com/isseis/go-safe-cmd-runner/issues/1154)）。
- **redaction の適用範囲の変更。** 既存の `RedactText` と既存の `processSlice` の挙動をそのまま使う（対象 8 は回帰固定のみ）。
- **`user_group_command_failure` 通知の配線。** 別タスク（0174）で扱う。
- **command 依存検証（`VerifyCommandDependencies`）とコマンドパス解決（`ResolvePath`）の失敗の通知。** [`group_executor.go`](../../../internal/runner/group_executor.go) の `verifyGroupFiles` は、動的ライブラリ・shebang の依存検証失敗とパス解決失敗を `*verification.Error` ではない生のエラーとして返す。そのため `executeGroups` の検証分岐に乗らず、`cmd/runner/main.go` で `ExecutionError`（`system_error`、`slack_notify=false`）になり、Slack へは届かない。加えて `executionResult` が未設定のため `command_group_summary` も出ない。グローバル・group のファイル検証は通知されるのに command レベルの検証だけ通知されない非対称であり、運ぶ情報が「一覧」ではなく「パス 1 件と理由」であるため本タスクの一覧契約とは別の形になる。認識済みの残存として記録し、次タスクの候補とする（アーキテクチャ §5.5・§9、[#1152](https://github.com/isseis/go-safe-cmd-runner/issues/1152)）。
- **`Runner.executeGroups` が先頭のエラーしか返さない点。** 複数 group が失敗した場合、2 件目以降のエラーは `groupErrs[0]` の返却で捨てられ、`ExpandGroup` 失敗のように group executor がログしない経路はどこにも残らない。`errors.Join` への置き換えは別タスクとする（[#1153](https://github.com/isseis/go-safe-cmd-runner/issues/1153)）。
- **`HandleExecutionError` が `PreExecutionError.Detail()` と同じ組み立てを重複実装している点。** 効果が小さいため、コードにコメントを残して当面据え置く（[#1156](https://github.com/isseis/go-safe-cmd-runner/issues/1156)）。
- **`Component` の型付け。** `resource.Component` を `common` へ移して `PreExecutionError.Component` / `ExecutionError.Component` を型付きにする改善（`logging` は `resource` を import できないため型の移動が要る）は、対象 16 のリテラル置換で実害が消えるため別タスクとする（[#1156](https://github.com/isseis/go-safe-cmd-runner/issues/1156)）。

## 決定事項

### 失敗ファイルの出所は `verErr.Details` に一本化する

通知に載せるファイル一覧は `verification.Error.Details` だけから取る。`Total`／`Verified`／`Failed` の件数や別のログから作り直さない。件数と一覧が食い違う場合に、出所が 2 つあると同じ誤りを二重に直すことになるためである。

### 収集失敗も `Details` に一本化する

検証対象の収集失敗（コマンドのパス解決失敗）でも、解決に失敗した対象の一覧を `verification.Error.Details` に設定する。通知へ載せる一覧の出所は常に `Details` であり、収集失敗だけ別経路にしない。`Details` が空のまま残るのは、`*verification.Error` 以外の失敗と、失敗対象を持たない検証エラーに限る。

収集失敗の `Message` には対象名を入れない。`Err` にはパスを含まないセンチネル（"failed to collect verification files"）を用い、パス解決の生の原因は検証マネージャの既存の構造化ログに残す。収集失敗では検証が 1 件も実行されず、対象のすべてが検証から除外されるため、`Total`／`Verified`／`Failed` の検証サマリを提示しない。本文は対象総数（`verify_files` とコマンド数の合計）と解決に失敗した対象数による収集段階の件数（例: `Collection failed: 1 of 3 targets unresolved`）で示す。

### group 名は Scope に一本化する

group 名を `Group: <name>, ` のような別個のメタデータとして本文・フィールドへ重複して表示しない。0172 の AC-13・AC-14 が定めた「group 名の構造的な表示場所を Scope にする」を維持する。失敗ファイルのパスに group 名と同名の文字列が含まれることはあるが、それはパスの内容であり、group 名の重複表示ではない。

### `Details` が空でも本文は件数と `Err` を示す

本文テンプレートは `verErr.Err` の種別で選び、`Details` の有無では変えない。`verification.Error.Details` が空でも、本文は同じテンプレート（検証失敗なら `Total: %d, Verified: %d, Failed: %d, Error: %v`）のままで、`Files:` 節だけを付けない。一覧が無いことを空文字や空フィールドで示さない。グローバルと group で同じ規則である。Manager は本タスク後 `Details` が空の `*verification.Error` を返さないため（収集失敗も `Details` を持つ）、この規則は共有コンストラクタの単体テストで固定する。

### 表示は既存の表示安全な補間契約に従う

Error Message は動的な値であり、0172 の表示安全な補間契約を通る。したがって 1 行であること・制御文字と書式制御文字を含まないこと・長さが上限（500 byte）を超えないことは既存の仕組みで保たれる。失敗ファイル一覧がこの上限を超える場合に全件を表示することはできないため、上限内に収まる範囲のファイルパスを表示し、残りを省略したこととその件数を示す。上限超過時の振る舞いを、切り詰め任せにせず本タスクの要件として定義する。

### 失敗ファイル一覧は構造化属性で運び、ビルダーが描画する

失敗ファイル一覧を自由文 `Message` へ連結しない。専用属性 `failed_file_paths`（`[]string`）として運び、`Error Message` フィールドの値は通知ビルダーが `Message` と `failed_file_paths` から組み立てる。これにより、パスは redaction の外にある stderr（`handleErrorCommon` の出力）へ届かず、`error_message` という自由文フィールドの意味も変えない。新しい Slack フィールドは足さず、`Error Message` の中身だけを組み立て直す。

### グローバルと group は同じコンストラクタ・同じ属性・同じビルダーを使う

グローバルと group の検証エラー報告は、`internal/runner/runerrors` の共有コンストラクタ `NewVerificationPreExecutionError` が `*verification.Error` から組み立てる。両者は同じ本文テンプレート（`Total: %d, Verified: %d, Failed: %d, Error: %v`、収集失敗は `Collection failed: %d of %d targets unresolved, Error: %v`）、同じ `Component`（`verification`）、同じ一覧の複製規則を使い、発火元ごとに本文やフィールドを組み立て直さない。`Message` はパスを含まず、通知ビルダーは `error_type` を問わず `failed_file_paths` を読むため、両者とも同じ予算管理（全件、または上限内の範囲と省略件数）で `Error Message` に一覧が描画される。グローバルは従来 `err.Error()` の切り詰めに委ねていたが、これで本文・切り詰め・省略件数の扱いが group と揃う。

### 失敗ファイル一覧の並びは Manager が生成時に正規化する

`verification.Error.Details` は Manager が `Error` を生成する時点で昇順に並べたコピーを設定する。group の `Details` は map に由来して順序が変わりうるが、発火元ごとの並べ替え規約に頼らず、`Error` の値を生成する唯一の場所で正規化する。`Details` の複製は共有コンストラクタが行い、通知の発火元は並べ替えもしない。ビルダーは並びを変えない。`Error()` の連結順も同時に安定する。

### `runerrors` は共有コンストラクタだけを持つパッケージにする

共有コンストラクタを置く `internal/runner/runerrors` から、本番呼び出しの無い既存シンボルを削除する（対象 14）。生きたコードと死んだコードを同居させると、パッケージの責務を読み手が誤解し、死んだ分類 API を新しい呼び出し元が使い始める余地を残すためである。削除は共有コンストラクタの追加とは別コミットで行い、`go test -coverprofile=c.out ./internal/runner/runerrors/` でプロファイルを作ってから `go tool cover -func=c.out` を実行し、残るシンボルのカバレッジが変わらないことを確認する。

### `verification.Error` は非公開コンストラクタ 1 箇所で生成する

`manager.go` の 3 つの生成箇所を非公開コンストラクタに集約し、`Details` の昇順コピーはそこでだけ作る（対象 15）。「唯一の場所で正規化する」を規約ではなくコード構造で保証するためである。コンストラクタは非公開のままとし、`verification.Error` の型・公開 API は変えない（対象外「`verification.Error` の型・`Error()` の変更」と矛盾しない）。

### `Component` は typed 定数だけから渡す

`Component` の生リテラルを `resource.Component` 定数経由に揃える（対象 16）。本タスクが group の `Component` を `"runner"` から `verification` へ変える際に同じ箇所を触るためで、値の集合を 1 箇所（`resource/types.go`）で読めるようにする。フィールドの型自体を変える改善は対象外に記録する。

### 文字列スライス要素の redaction は既存挙動を回帰で固定する

`failed_file_paths` の要素はファイルパス（データ）であり、`RedactingHandler.processSlice` は文字列要素に `RedactText`（値形式検出と key=value 置換）だけを適用し、値全体置換は行わない（`internal/redaction/redactor.go:1436-1439`）。この挙動は既に存在するため、本タスクは redaction を変更せず、機密がマスクされ普通のパスが残ることをテストで固定する。`error_message`（`KindString`）の値全体置換は従来どおりである。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 失敗ファイル名の表示

**Acceptance Criteria**:
- **AC-01**: 失敗ファイル一覧の表示が表示上限に収まるとき、検証エラー通知の `Error Message` フィールドに `verification.Error.Details` の各ファイルパスが（区切りと衝突しないエンコード後の表示形で）現れる。
- **AC-02**: 失敗ファイル一覧が表示上限（既存の表示安全な補間契約の 500 byte）を超えるとき、上限内に収まる範囲のファイルパスと、省略した件数が `Error Message` に現れる。
- **AC-03**: group 名は通知コンテキスト（Scope）に表示され、Error Message には `Group: <name>, ` のような別個のメタデータとして重複しない。失敗ファイルパスに偶然含まれる group 名の文字列は許容する。
- **AC-04**: 本文テンプレートは `verErr.Err` の種別で決まり、`Details` の有無で変わらない。`Details` が空のときは、同じテンプレートの本文（検証失敗なら `Total: N, Verified: N, Failed: N` と `verErr.Err`）に `Files:` 節が付かず、グローバルと group で同じ規則である。
- **AC-05**: グローバル検証エラー通知も、group と同じ共有コンストラクタ・同じ本文テンプレート・同じ予算管理（全件、または上限内に収まる範囲と省略件数）で `failed_file_paths` を `Error Message` に描画する。

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
- **AC-17**: 検証対象の収集失敗（コマンドのパス解決失敗）でも、解決に失敗した対象が全て `failed_file_paths` に記録され、`Error Message` に表示される。収集失敗の `Error Message` は `Total`／`Verified`／`Failed` の検証サマリを提示せず、収集段階の件数（解決に失敗した対象数と対象総数）を示す。`HandlePreExecutionError` が記録する `Message` と `handleErrorCommon` が stderr へ書く文字列は対象名を含まず、対象名は検証マネージャの構造化ログに残る。

#### F-007: 報告の組み立ての統一

**Acceptance Criteria**:
- **AC-18**: グローバルと group の検証エラー報告は共有コンストラクタ `runerrors.NewVerificationPreExecutionError` で組み立てられ、`Component` はどちらも `verification` である。同じ `*verification.Error` から同じ本文・同じ一覧属性が得られる。両発火元が実際にこのコンストラクタを呼び、`PreExecutionError` を手組みしないことは `go/ast` の静的ガードで固定する（挙動テストは同等の手組みを検出できないため）。
- **AC-19**: `verification.Error.Details` は Manager が `Error` を生成する時点で昇順に正規化される。発火元は並べ替えを行わず、通知ビルダーは並びを変えない。

#### F-008: 周辺の整理

**Acceptance Criteria**:
- **AC-20**: `internal/runner/runerrors` に残る本番シンボルは `NewVerificationPreExecutionError` だけである。`ClassifiedError`・`ClassifyVerificationError`・`LogClassifiedError`・`LogCriticalToStderr` とそのテストは削除され、README のパッケージ一覧の説明が更新されている。
- **AC-21**: `internal/verification/manager.go` で `&Error{...}` の構造体リテラルが現れるのは非公開コンストラクタの中だけであり、グローバル検証失敗・group 収集失敗・group 検証失敗の 3 経路はすべてそれを経由する。AC-19 の昇順テストは 3 経路それぞれで通る。
- **AC-22**: `cmd/runner`・`internal/runner` の本番コードで `PreExecutionError.Component` / `ExecutionError.Component` に渡す値はすべて `resource.Component` 定数経由であり、生リテラル `"main"`・`"runner"` は残らない。

## Success Criteria（要件レベル）

- group 検証エラー通知から失敗ファイルを判別できる。
- 検証対象の収集失敗でも、原因のコマンド（対象名）を通知から判別できる。
- 失敗ファイル一覧は構造化属性として運ばれ、通常の `key` などを含むパスが失敗しても通知は消えない。`handleErrorCommon` の stderr 出力にパスが出ない。
- group 名が別個のメタデータとして本文へ重複しない（失敗ファイルパス内の同名文字列は除く）。
- グローバルと group の通知が、同じ構造化属性・同じ共有コンストラクタ・同じ予算管理による失敗ファイルの提示で対称になる。
- 通知種別定義・`error_type`・Slack フィールド集合が変わらない（`Error Message` の値と group の `Component` の組み立てだけを変える）。本文は group の既存テンプレートに統一する。
- `runerrors` に死コードが残らず、`verification.Error` の生成と正規化が 1 箇所に集約され、`Component` の値が typed 定数だけから渡される。
- command 依存検証の通知欠落と `executeGroups` の先頭エラーのみ返却は、認識済みの残存として文書に記録されている。
