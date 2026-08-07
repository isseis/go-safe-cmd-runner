# 要件定義書: redaction のカバレッジ拡張と Slack 送信の非同期化

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-08-07 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#975 [Security][D2 M-2/M-4/M-5] redaction: key=value網羅性・ValueDetectorパターン・Slack同期送信](https://github.com/isseis/go-safe-cmd-runner/issues/975)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/D2_logging_redaction.md](../0149_security_code_smell_audit_fable/findings/D2_logging_redaction.md) の M-2 / M-4 / M-5
- 残件一覧: [docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §1
- 先行タスク: [0152（ログ本文の redaction）](../0152_redact_log_message_body/01_requirements.md)（D2 H-1 を解消）、[0154（redaction 境界の統一）](../0154_redaction_boundary_unification/01_requirements.md)（D2 M-1 を解消し、M-2/M-4/M-5 を「根本原因とは独立」として対象外とした）

## 背景

`internal/redaction` と `internal/logging` は、機密情報のログ出力抑止と、Slack webhook という外部への送出経路を担う。0149 監査の D2 所見のうち 🔴High（H-1: ログ本文が redact されない）は 0152 で、🟡Medium の M-1（`slog.Any` の struct/map が素通りする）は 0154 で解消された。一方、M-2・M-4・M-5 の 3 件は「根本原因とは独立した所見」として明示的に対象外とされたまま、2026-08-05 時点で未着手である。3 件はいずれも同一の 2 パッケージに閉じるため、1 タスクとしてまとめて扱う。

### M-2: key=value redaction が空白・引用符・JSON 形式をカバーしない

`Config.RedactText` は `KeyValuePatterns`（`password`, `token`, `key`, `secret`, `api_key`, `_PASSWORD` 等）の各キーについて `performKeyValueRedaction` を適用する。キーの形によって 3 経路に分岐する。

| キーの形 | 関数 | 生成する正規表現 |
|---|---|---|
| `:` を含む（例 `Authorization: `） | `performColonPatternRedaction` | `(?i)(key)([ \t]*)((?:bearer \|basic )?)[^\r\n]*` |
| 空白を含む（例 `Bearer `） | `performSpacePatternRedaction` | `(?i)(key)(\S+)` |
| それ以外（例 `password`） | `performKeyValuePatternRedaction` | `(?i)(key)(=)(\S+)` |

第 3 経路（キー名ベースの大多数がここを通る）には次の 3 つの取りこぼしがある。

1. **値に空白が含まれる場合**: `(\S+)` は最初の空白で止まるため、`password=my secret phrase` は `password=[REDACTED] secret phrase` となり、2 語目以降が平文で残る。
2. **値が引用符で囲まれる場合**: `password="abc def"` は `password=[REDACTED] def"` となり、閉じ引用符までの残りが平文で残る。
3. **区切りが `=` 以外の場合**: `(=)` を必須とするため、`"password": "secret"`（JSON）、`password: secret`（YAML）、`password = secret`（空白を挟む代入）はいずれも一致しない。値形式検出（`ValueDetector`）が拾える型（AWS キー等）のみが救済される。

これらの文字列は、コマンドの標準出力・標準エラー出力（stderr）、設定パースエラーのメッセージ、コマンドライン引数などを経由してログ属性に載る。ログファイル（0600）と stderr に平文で残り、`slack_notify` が付いたレコードでは Slack にも送出される。

なお、`RedactText` は属性値やメッセージ本文といった**任意のテキスト**に対して適用される。カバレッジを広げる際は、無関係なテキストを巻き込む過剰 redaction（D2 L-5 で既に指摘されている性質）を悪化させないことが同時に求められる。

### M-4: ValueDetector のパターン網羅性不足

`internal/redaction/value_detector.go` の `valueDetectorPatterns` は、キー名の文脈がなくても値の形式だけで秘密を検出する層である。現在の検出対象は AWS アクセスキー ID（`AKIA`/`ASIA`）、GitHub の classic トークン（`gh[pors]_`）、Slack トークン（`xox[bpar]-`）、GCP サービスアカウントのキー ID（JSON フィールド名に依存）、PEM 秘密鍵ブロック、`Bearer ` トークン、URL 埋め込み資格情報（`scheme://user:pass@`）の 7 種である。

次の主要な形式が検出対象外である。

- **GitHub fine-grained PAT**（`github_pat_...`）: `\bgh[pors]_` に一致しない。
- **Slack の `xapp-`（App-level token）・`xoxe-`（refresh）・`xoxs-`**: 文字クラス `[bpar]` の外。加えて既存の `xox[bpar]-` は `-[0-9]{10,}-[0-9]{10,}-` という 3 セグメント構造を要求するため、この構造を取らないトークンには一致しない。
- **JWT**（`eyJ` で始まる 3 セグメントの Base64URL 文字列）: パターンが存在しない。

キー名の文脈を伴わない秘密（例: コマンドの標準出力に単体で現れた fine-grained PAT）は、キー名ベースの層でも値形式の層でも捕捉されず、`command_group_summary` として Slack に投稿され得る（`internal/logging/slack_handler.go` は出力を最大 1000 文字まで転送する）。

#### 調査結果1: AWS Secret Access Key の扱い（2026-08-07 時点）

所見 M-4 は AWS Secret Access Key（40 文字の Base64 風文字列）にも言及している。この値は自己識別可能な形式を持たず、同じ文字種・長さの非機密文字列と値だけでは区別できないため、値形式検出の対象にできない。一方、キー名ベースの層では既に `aws_secret_access_key=...` が `KeyValuePatterns` の `key` によって捕捉される（`aws_secret_access_` に続く `key=` が `(?i)(key)(=)(\S+)` に一致するため）。`aws_secret_access_key` を `KeyValuePatterns` へ明示的に追加する必要はなく、M-2 の対応によって `=` 以外の区切り（JSON/YAML）にも同じ捕捉が及ぶ。したがって本タスクでは AWS Secret Access Key に対する新規の値形式パターンは追加せず、この結論を文書として残す。

#### 調査結果2: Slack webhook URL のパターン（2026-08-07 時点）

所見 M-3（webhook URL が送信失敗時のエラーログに漏れる）は、`sanitizeErrorForLog` の導入により `*url.Error` から URL を除去する形で解消済みであり、残件一覧にも挙がっていない。ただし M-3 の推奨対応には「`hooks.slack.com/services/\S+` を ValueDetector のパターンに追加する」も含まれており、この部分は未実施である。webhook URL 自体が credential であること、および `sanitizeErrorForLog` は `*url.Error` 以外の経路（設定値のログ出力、コマンド出力への混入など）を覆わないことから、値形式検出への追加は多層防御として意味を持つ。本タスクの対象に含めるか否かは「検討事項」で扱う。

### M-5: Slack 送信の同期ブロッキング

`SlackHandler.Handle` は `sendToSlack` を同期的に呼ぶ。`sendToSlack` は最大 `RetryCount` 回のリトライを指数バックオフ（既定 2s + 4s + 8s = 14s）で行い、各試行には HTTP クライアントのタイムアウト（既定 5s）が掛かる。Slack が到達不能な場合、1 回のログ呼び出しは最長で約 34 秒ブロックする。

`MultiHandler.Handle` は登録された全ハンドラを同期的に順次呼ぶため、このブロッキングはログ呼び出し元にそのまま伝播する。ログは特権操作の失敗通知やセキュリティアラートといったクリティカルパスから呼ばれるため、外部サービス（Slack）の障害がツール全体の実行時間とタイムアウト挙動に波及する。攻撃者がデータ送信（egress）を妨害することで実行を大幅に遅延させることも可能である。

また `sendToSlack` に渡される `ctx` は `slog` のログ呼び出し由来のコンテキストであり、送信全体に対する期限（デッドライン）は課されていない。バックオフ待機は `ctx.Done()` を尊重するが、`ctx` にデッドラインがなければ待機は最後まで実行される。

現在 `SlackHandler` には終了処理（`Close` / `Flush` 相当）が存在せず、`internal/runner/bootstrap` にも Slack 送信を待ち合わせる仕組みはない。送信を非同期化する場合、プロセス終了時に未送信の通知を送り切る経路を新たに設ける必要がある。

#### 調査結果3: プロセス終了経路（2026-08-07 時点）

`cmd/runner/main.go` の `main` は `mainWithExitCode` の戻り値を受け取り、`bootstrap.ReportRedactionFailures()` を呼んでから `os.Exit(exitCode)` する。ここが唯一の正常終了点である。これより前にある 2 箇所の `os.Exit(1)`（`--run-id` 検証失敗、`DefaultHashDirectory` 検証失敗）は `bootstrap.AddSlackHandlers` の呼び出し前に位置するため、Slack ハンドラが存在せず、未送信の通知も存在しない。したがって flush の呼び出し箇所は `ReportRedactionFailures` と同じ位置に 1 箇所設ければ足りる。

`record` / `verify` は Slack ハンドラを構成しないため、本タスクの M-5 対応の影響を受けない。

## 目的

- 機密情報がログファイル・stderr・Slack へ平文で出力される経路のうち、キー名ベースの層で取りこぼされていた値の形（空白入り・引用符付き・JSON/YAML 形式）を塞ぐ。
- 値形式検出の対象に、現在広く使われている秘密の形式（GitHub fine-grained PAT、Slack の追加プレフィックス、JWT）を加える。
- Slack という外部サービスの可用性が、コマンド実行のクリティカルパスの所要時間を左右しない構造にする。
- 上記により D2 の未対応 Medium 3 件を解消し、残件一覧 §1 から D2 の項目を除去できる状態にする。

### 本タスクで解消されないもの

- redaction は既知の形式に対する多層防御であり、未知の credential 形式・独自トークンスキーム・高エントロピー文字列は引き続き検出されない。この限界は `docs/user/security-risk-assessment.ja.md` に記載済みであり、本タスクは検出対象の集合を広げるに留まる。
- 引用符で囲まれていない値に空白が含まれる場合、値の終端を機械的に判定する手段はない。この場合の扱いは「検討事項」で決定するが、いずれの決定を採っても「任意の平文が確実に消える」状態にはならない。
- 非同期化は Slack 通知の**到達**を保証しない。到達不能が続く場合、通知は最終的に失われる（現状も同じ）。本タスクが変えるのは、その失敗が実行時間に波及しないことと、失われた事実が観測可能になることである。

## スコープ

### 対象

1. `internal/redaction` のキー名ベース redaction における、区切り文字（`=` / `:`）・区切り前後の空白・引用符付き値のカバレッジ拡張（M-2）。
2. 1 に伴う過剰 redaction の非悪化の担保。
3. `internal/redaction/value_detector.go` への値形式パターン追加（M-4）。
4. `internal/logging/slack_handler.go` の Slack 送信の非同期化、送信全体へのデッドライン付与、およびプロセス終了時の flush（M-5）。
5. 4 に伴う `internal/runner/bootstrap` および `cmd/runner` の終了処理の変更。
6. 上記に対応するテストの追加・更新。
7. 利用者向け文書（`docs/user/security-risk-assessment.ja.md` の検出対象と限界の記述）および開発者向け文書の更新。

### 対象外

- D2 H-1（ログ本文の redaction）: 0152 で解消済み。
- D2 M-1（`slog.Any` の struct/map への再帰適用）: 0154 で解消済み。
- D2 M-3（webhook URL のエラーログ漏洩）: `sanitizeErrorForLog` により解消済み。ただし推奨対応の一部（値形式パターンへの追加）の扱いは「検討事項」で決定する。
- D2 L-1〜L-6 および I-1〜I-6: 本タスクでは扱わない。ただし L-1 が指摘する「`RedactText` が呼び出しごとに正規表現を再コンパイルする」点は、M-2 でパターンが複雑化することにより影響が拡大するため、「検討事項」で扱う。
- D2 L-5（`key`/`basic` 等の部分一致による過剰 redaction）の解消。本タスクは過剰 redaction を**悪化させない**ことのみを要件とし、既存の過剰 redaction の是正は行わない。
- Slack 通知の到達保証（永続キュー、プロセス再起動をまたぐ再送）。
- Slack 以外の出力先（ファイル・stderr）の同期性。これらはローカル I/O であり、外部サービス障害の波及元にならない。
- `record` / `verify` の挙動変更。
- AWS Secret Access Key に対する値形式パターンの追加（「調査結果1」のとおり）。

## 検討事項（設計判断が必要な項目、02_architecture.md で決定する）

- **引用符なしで空白を含む値の扱い**: `password=my secret phrase` の 2 語目以降をどう扱うか。(a) 現行どおり最初の空白で止める（過剰 redaction を避けるが平文が残る）、(b) 行末まで置換する（`Authorization: ` の既存挙動と揃うが、`--flag=value other args` のようなコマンドライン断片で情報が大量に失われる）のいずれかを選ぶ。採らなかった側は残存リスクまたは診断性の低下として文書化する。
- **区切り文字拡張の適用範囲**: `key\s*[:=]\s*` を `KeyValuePatterns` の全キーに一律適用すると、`key` や `basic` のような短く一般的なキー（D2 L-5）が `monkey: value` のような無関係なテキストに一致する機会が増える。全キーに適用するか、語境界を導入するか、コロン区切りは特定のキーのみに限るかを決定する。
- **引用符の対応範囲**: `"..."` / `'...'` の 2 種に限るか、バッククォートや、エスケープされた引用符（`"a\"b"`）を含めるか。閉じ引用符が存在しない入力（切り詰められたログ行など）での終端の扱いも決める。
- **既存 3 経路の統合可否**: `performColonPatternRedaction` / `performSpacePatternRedaction` / `performKeyValuePatternRedaction` の分岐をそのまま残して拡張するか、1 つの生成規則に統合するか。統合する場合は `Authorization: ` の既存挙動（行末まで置換し、`Bearer`/`Basic` スキーム名を保持）を維持する必要がある。
- **正規表現の事前コンパイル**: D2 L-1 のとおり `RedactText` は呼び出しごとに全パターンをコンパイルしている。M-2 でパターンが複雑化し、かつ redaction は長いコマンド出力に対して呼ばれるため、コンストラクタでの事前コンパイルへ移すかを決定する。移す場合、コンパイル失敗時のフェイルセキュア挙動（現行は `RedactionFailurePlaceholder` へフォールバック）をどう保つかも併せて決める。
- **JWT パターンの誤検出**: `eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*` 形式は、JWT 以外の Base64URL 文字列にも一致し得る。セグメント数・最小長・署名部が空の場合（`alg=none`）の扱いを決定する。
- **Slack webhook URL パターンの採否**: 「調査結果2」の `hooks.slack.com/services/...` を値形式検出に追加するか。追加する場合、ホスト名を `AllowedHost` 設定から取るか固定パターンとするかを決める。
- **非同期化の方式**: goroutine ＋ バッファ付きキューとするか、送信ごとに goroutine を起こすか。前者を採る場合のキュー長、ワーカー数（順序保証の要否に直結する）、キューが満杯になったときの挙動（即時破棄／短時間の待機後に破棄／最古を破棄）を決定する。セキュリティアラートの通知が破棄され得る点をどう扱うかを含める。
- **デッドラインの設計**: 1 件あたりの送信全体（リトライ込み）に課すデッドラインと、プロセス終了時の flush 全体に課すデッドラインの値。既定の backoff（2s+4s+8s）とリトライ回数を維持したままデッドラインを課すのか、非同期化に伴ってリトライ方針自体を見直すのかを決定する。
- **破棄・失敗の記録経路**: キュー溢れや送信失敗を記録する際、`slog.Default()` を使うと `SlackHandler` 自身へ再入し、無限ループまたはキューの自己増殖を招き得る。`RedactingHandler` の `failureLogger`（Slack を含まないハンドラ列）と同じ構造を用いるか、`bootstrap` から専用のロガーを注入するかを決定する。`redaction.NewRedactingHandler` が行っている循環依存の起動時検証と同等の保護が必要かも併せて検討する。
- **flush の公開インターフェース**: `SlackHandler` に `Close` / `Flush` を持たせた場合、`slog.Handler` として `MultiHandler` に埋め込まれているため呼び出し側が具象型を知る必要がある。`bootstrap` が生成した `SlackHandler` を保持して直接呼ぶか、`MultiHandler` に「flush 可能なハンドラを走査する」責務を持たせるかを決定する。`AddSlackHandlers` が既定ロガーを差し替える構造（グローバル状態）との整合も含める。
- **`WithAttrs` / `WithGroup` とワーカーの共有**: `SlackHandler.WithAttrs` は新インスタンスを返す。各インスタンスが独自のキューとワーカーを持つと goroutine が増殖し、flush 対象の把握も困難になる。キューとワーカーを派生インスタンス間で共有する仕組みを決定する。
- **ドライラン時の扱い**: 現行は `isDryRun` のとき `sendToSlack` を呼ばずに `slog.Debug` を出す。非同期化後もこの分岐を Handle 側（enqueue 前）に置くかを決定する。

## Acceptance Criteria

#### F-001: キー名ベース redaction の区切り・引用符カバレッジ拡張

`Config.RedactText` が、キー名と値の間の区切りとして `=` に加えて `:` を、区切り前後の空白を、および引用符で囲まれた値を扱えるようにする。

**Acceptance Criteria**:
- **AC-01**: `password="abc def"` の形式（値が二重引用符で囲まれ、内部に空白を含む）に対し、閉じ引用符までの値全体が置換され、平文が残らない。単一引用符 `'...'` についても同じ結果になる。
- **AC-02**: `"password": "secret"`（JSON 形式）に対し、値 `secret` が置換される。キー名側の引用符とコロン、および値の引用符は保持され、JSON としての構造が壊れない。
- **AC-03**: `password: secret`（YAML 形式）および `password = secret`（区切りの前後に空白）に対し、値が置換される。
- **AC-04**: 上記いずれのケースでも、置換後の文字列に元の値の断片が一切含まれない。
- **AC-05**: `KeyValuePatterns` に列挙された全キーについて AC-01〜AC-03 が成立する（特定のキーだけが対応する状態にしない）。ただしキーの形により適用範囲を限定する設計判断を採る場合は、限定の対象と根拠が 02_architecture.md に記載され、限定されたキーについてはその旨をテストで固定する。

#### F-002: 既存の redaction 挙動の非退行と過剰 redaction の抑制

F-001 の拡張が、既存の redaction 結果および無関係なテキストの可読性を悪化させないことを保証する。

**Acceptance Criteria**:
- **AC-06**: `Authorization: Bearer xxx` の既存挙動（行末まで置換し、`Authorization: ` と `Bearer ` を保持する）が維持される。
- **AC-07**: `Bearer xxx` / `Basic xxx` の既存挙動（プレフィックスを保持して以降のトークンを置換する）が維持される。
- **AC-08**: 既存のテストが検証している redaction 結果は、文字列として変化しない。変化する場合は、その変化が意図した改善であることを 02_architecture.md に記載し、期待値の更新理由をテストのコメントとして残す。
- **AC-09**: 秘密を含まないテキスト（例: `--timeout=30`、`keyboard: qwerty`、`/usr/local/key/path`）に対し、F-001 の拡張によって新たに置換される範囲が生じない。過剰 redaction の範囲が拡張前と同一であることをテストで固定する。
- **AC-10**: 置換対象を含まない長いテキストに対する `RedactText` の結果が、拡張前と一致する。

#### F-003: 値形式検出パターンの追加

キー名の文脈を伴わずに現れる秘密のうち、現在検出されていない主要な形式を検出対象に加える。

**Acceptance Criteria**:
- **AC-11**: GitHub fine-grained PAT（`github_pat_` に続く英数字とアンダースコアの列）が、キー名の文脈なしに単体で現れた場合に置換される。
- **AC-12**: Slack の App-level token（`xapp-`）、refresh token（`xoxe-`）、および `xoxs-` プレフィックスのトークンが置換される。既存の `xoxb-`/`xoxp-`/`xoxa-`/`xoxr-` の検出は維持される。
- **AC-13**: JWT 形式の文字列（`eyJ` で始まる 3 セグメントの Base64URL 文字列）が置換される。
- **AC-14**: AC-11〜AC-13 の各形式が、コマンドの標準出力に相当するテキスト（キー名を伴わない自由テキスト）に含まれる場合でも置換される。
- **AC-15**: AC-11〜AC-13 の追加により、秘密でない文字列が新たに置換されないことを、誤検出の候補（例: `github_pattern`、`xapple`、`eyJ` で始まるが JWT ではない文字列）に対するテストで固定する。
- **AC-16**: 置換文字列（プレースホルダー）に `$` を含む値が設定されていても、捕捉グループの再展開によって秘密が再注入されない（既存の `$` エスケープの保証が新規パターンにも及ぶ）。

#### F-004: Slack 送信の非同期化

`SlackHandler.Handle` の呼び出しが、Slack への HTTP 送信の完了を待たないようにする。

**Acceptance Criteria**:
- **AC-17**: Slack webhook が応答しない（接続がハングする）状況で、`slog` の 1 回のログ呼び出しが返るまでの時間が、HTTP タイムアウトおよびバックオフ合計に依存しない。同一条件で複数回ログを呼んでも、呼び出し側の所要時間はキューの受け入れに要する時間のみで決まる。
- **AC-18**: `MultiHandler` に Slack ハンドラと他のハンドラが登録されている場合、Slack が到達不能でも他のハンドラへの書き込みは遅延しない。
- **AC-19**: 1 件の Slack 送信（リトライを含む全体）に対して期限が課され、期限を超えた送信は打ち切られる。期限は設定可能で、既定値が 02_architecture.md に根拠とともに記載される。
- **AC-20**: 非同期化後も、送信されるメッセージの内容（`SlackMessage` の各フィールド）は同期実装のときと同一である。メッセージはレコードを受け取った時点で構築され、後から変化するプロセス状態に依存しない。
- **AC-21**: `WithAttrs` / `WithGroup` で派生した `SlackHandler` を含め、1 つの webhook 設定に対して生成されるワーカー goroutine の数が有界である。派生インスタンスを多数生成しても goroutine が増え続けない。
- **AC-22**: ドライランモードでは、非同期化後も Slack への HTTP 送信が一切行われない。

#### F-005: プロセス終了時の flush

非同期化によって、プロセス終了時に未送信の通知が失われないようにする。

**Acceptance Criteria**:
- **AC-23**: `cmd/runner` の正常終了経路において、キューに残っている通知が送信されるまで（または flush の期限に達するまで）プロセスは終了しない。
- **AC-24**: 実行の最後に出力される通知（実行サマリ、特権操作の失敗通知等）が、非同期化後も Slack へ送信される。同期実装のときに送信されていた通知が、終了処理の競合によって失われない。
- **AC-25**: flush 全体に期限が課され、Slack が到達不能な場合でもプロセスの終了がその期限を超えて遅延しない。期限の既定値が 02_architecture.md に根拠とともに記載される。
- **AC-26**: flush の期限内に送信し切れなかった通知がある場合、その件数が運用者に分かる形で報告される。
- **AC-27**: flush 完了後に到着したログレコードは、Slack への送信を試みずに破棄される。破棄によってプロセスがブロックしたり panic したりしない（クローズ済みチャネルへの送信が発生しない）。
- **AC-28**: `record` / `verify` は Slack ハンドラを構成しないため、本要件による終了処理の変更の影響を受けない。

#### F-006: 送信失敗・破棄の可観測性と再入防止

非同期化によって、送信失敗がログ呼び出し元の戻り値から見えなくなる。失敗を別経路で観測可能にする。

**Acceptance Criteria**:
- **AC-29**: キューが満杯で通知を受け入れられなかった場合、その事実が記録される。記録には破棄された通知の件数が含まれ、破棄された通知の本文（機密を含み得る）は含まれない。
- **AC-30**: 送信失敗・キュー溢れの記録は、`SlackHandler` を含まない出力先へ書かれる。これらの記録が起点となって新たな Slack 送信が発生しない。
- **AC-31**: 上記の記録経路に `SlackHandler` が含まれる構成が作られた場合、実行時に無限ループへ陥るのではなく、起動時または構成時に検出できる。検出方法（起動時検証か、構造的に不可能とするか）は 02_architecture.md で決定する。
- **AC-32**: 非同期化に伴って追加された共有状態（キュー、ワーカー、終了フラグ）が並行安全であり、`go test -race` で競合が検出されない。複数の goroutine から同時にログを出力し、その最中に flush を呼ぶ経路を含めて検証する。

#### F-007: 文書の更新

検出対象の拡張と Slack 通知の配送方式の変更を文書に反映する。

**Acceptance Criteria**:
- **AC-33**: `docs/user/security-risk-assessment.ja.md` の「値ベース検出」の検出対象一覧に、F-003 で追加した形式が反映されている。「限界」の記述に、F-001 の検討事項で採らなかった側の残存リスク（引用符なしで空白を含む値の扱い）が追記されている。英語版は `/mktrans` で反映する。
- **AC-34**: Slack 通知が非同期に送信されること、プロセス終了時に有限の期限内で flush されること、および到達不能時には通知が失われ得ることが利用者向け文書に記載されている。日本語版を先に更新し、英語版は `/mktrans` で反映する。
- **AC-35**: `docs/translation_glossary.md` に、本タスクで新たに導入した用語（値形式検出、flush、バックプレッシャ等、実際に使用したもの）が追加されている。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが `make test` で成功する。
- `make lint` が警告なく通過する。
- `go test -race ./internal/logging/... ./internal/redaction/...` が成功する。
- Slack が到達不能な状態で `runner` を実行したとき、実行全体の所要時間の増加が flush の期限内に収まる。
- 0149 監査 D2 の M-2・M-4・M-5 について、[98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §1 の D2 の項目を削除できる状態になる。
