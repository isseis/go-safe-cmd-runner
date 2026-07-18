# セキュリティ監査所見: internal/logging/ + internal/redaction/

- 監査日: 2026-07-19
- 対象: `internal/logging/`（slack_handler.go, safeopen.go, multihandler.go, interactive_handler.go, conditional_text_handler.go, message_formatter.go, pre_execution_error.go, execution_error.go, security.go, log_line_tracker.go）、`internal/redaction/`（redactor.go, sensitive_patterns.go, value_detector.go, error_collector.go, reporter.go, errors.go）。テストは参照のみ。
- 方法: 静的コードレビュー（読み取り専用）。呼び出し元（`internal/runner/bootstrap/logger.go`, `internal/runner/runner.go`, `internal/runner/base/audit/logger.go`）の配線も確認。

これらのパッケージは、機密情報のログ出力抑止（redaction）と、Slack webhook という**外部への送出経路**を担うセキュリティクリティカルなコンポーネントである。`RedactingHandler` が `MultiHandler`（file / stderr / Slack）全体をラップする構成（bootstrap/logger.go:178, 249）であり、redaction の取りこぼしはそのまま外部送出につながり得る。

## 所見サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 1 |
| 🟡 Medium | 5 |
| 🟠 Low | 6 |
| 🔵 Info | 6 |

---

## 🔴 High

### H-1: `RedactingHandler.Handle` がレコードの **Message 本文を一切 redact しない**（Slack への送出経路を含む）

- 該当箇所: `internal/redaction/redactor.go:403-415`（`Handle`）、`internal/logging/slack_handler.go:823-827`（`buildGenericMessage` が `r.Message` をそのまま Slack 本文に使用）
- 問題:
  `Handle` は `slog.NewRecord(record.Time, record.Level, record.Message, record.PC)` で新レコードを作り、**属性（Attrs）だけ**を `redactLogAttributeWithContext` に通す。`record.Message` 文字列には `RedactText` が適用されない。
  呼び出し側が `slog.Error(fmt.Sprintf("failed to fetch %s: %v", url, err))` のように機密（credential 入り URL、トークン、コマンドライン等）をメッセージ本文へ埋め込むと、redaction を素通りして file / stderr へ書かれ、`slack_notify=true` が付いたレコードでは `buildGenericMessage` や各 build 系関数経由で **Slack にそのまま送出**される。属性は多層で防御しているのに、最も書きやすい場所（msg）が無防備という非対称がある。
- 悪用/漏洩シナリオ:
  1. 将来のコード変更や既存コードのどこかで、エラー文字列（例: `*url.Error` は URL 全体を含む）をメッセージ本文へ整形して `slog.Error(...)` する。
  2. `RedactText` は属性にしか適用されないため、`password=...` や `AKIA...` がメッセージ内にあっても redact されない。
  3. ログファイル（0600 だが管理者以外の閲覧経路があり得る）および Slack チャンネル（閲覧範囲がホストより広いことが多い）へ平文で流出する。
- 推奨対応: `Handle` で `newRecord` 作成時に `r.config.RedactText(record.Message)` を適用する（`WithAttrs` と同等の防御をメッセージにも）。パフォーマンスが懸念なら Slack 等の外部送出ハンドラの直前だけでも必須とする。

---

## 🟡 Medium

### M-1: `KindAny` の struct / map / 非 LogValuer 値が redaction を素通りする（ネスト構造への適用漏れ）

- 該当箇所: `internal/redaction/redactor.go:521-543`（`processKindAny` の「3. Unsupported type: pass through」）、`redactor.go:727-729`（`processSlice` の「Non-LogValuer element: keep as-is」）
- 問題:
  `slog.Any("config", cfg)` のように struct・map を渡すと、`processKindAny` は LogValuer とスライス以外を無加工で返す。下流の `slog.NewJSONHandler`（file ハンドラ）は `json.Marshal` で **全エクスポートフィールドを直列化**するため、`Password` フィールド等が redaction を経ずにログファイルへ書かれる。キー名チェックは属性キー（例: "config"）にしか効かない。
  さらに `processSlice` は LogValuer でない要素を「keep as-is」で通すため、`[]string{"password=secret"}` は redact されない。**同じ文字列が単独属性なら redact され、スライス要素だと漏れる**という非一貫性がある。map 要素・ネストした struct も同様。
- 悪用/漏洩シナリオ: デバッグ目的で `slog.Debug("loaded config", slog.Any("cfg", cfg))` を追加しただけで、TOML 由来の環境変数値（トークン等）がログファイルに平文で残る。
- 推奨対応: `processKindAny` で map / struct を reflection で走査してキー名・文字列値に redaction を適用するか、少なくとも未対応型は `RedactionFailurePlaceholder`（または `fmt.Sprint` 後に `RedactText`）へフォールバックして fail-secure にする。スライスの文字列要素にも `RedactText` を適用する。

### M-2: key=value redaction が `\S+` マッチのため、空白・引用符を含む値が部分的に漏れる

- 該当箇所: `internal/redaction/redactor.go:163`（`(\S+)`）、`redactor.go:228, 231`（`(\S+)`）
- 問題: `password=my secret phrase` → `password=[REDACTED] secret phrase` となり、最初の空白以降が残る。`password="abc def"` → `password=[REDACTED] def"`。JSON 形式 `"password": "secret"` はコロン＋空白区切りなので `key=value` パターンに一致せず、キー名ベースでは漏れる（値形式検出でカバーされる型のみ救済）。
- 悪用/漏洩シナリオ: 空白を含むパスフレーズや引用符付きの値をコマンド出力・エラー文字列経由でログすると、末尾部分が平文で残る。
- 推奨対応: 引用符付き値（`"..."` / `'...'`）を一括でマッチするパターンを追加する。`key\s*[:=]\s*` 形式（JSON/YAML）もカバーする。

### M-3: Slack webhook URL（それ自体が credential）が送信失敗時のエラーログに漏れる

- 該当箇所: `internal/logging/slack_handler.go:875-878`（`slog.Warn("Failed to send Slack request", slog.Any("error", err), ...)`）、`slack_handler.go:866-869`
- 問題: `http.Client.Do` の失敗は `*url.Error` を返し、その `Error()` 文字列は **URL 全体**（`Post "https://hooks.slack.com/services/T…/B…/秘密トークン": dial tcp ...`）を含む。この属性は `slog.Any`（KindAny の error）として記録されるため、M-1 のとおり redaction は文字列化後のテキストに適用されず、また文字列化されても `hooks.slack.com/services/...` 形式は `urlCred`（user:pass@ 形式のみ）にも他のパターンにも一致しない。Slack webhook URL は Slack 公式に「secret として扱え」とされる credential であり、これがログファイル・stderr に平文で残る。
- 悪用/漏洩シナリオ: ネットワーク断や DNS 障害で送信が失敗するたびに webhook URL がログへ書かれる。ログを閲覧できる者（あるいはログ収集基盤）が URL を入手すると、組織の Slack チャンネルへ任意メッセージを投稿できる（フィッシング・偽アラート注入）。
- 推奨対応: 送信エラーをログする際は `err` をそのまま渡さず、URL を含まない要約（エラー種別のみ）に整形する。あわせて `hooks.slack.com/services/\S+` を ValueDetector のパターンに追加する。

### M-4: 値形式検出（ValueDetector）のパターン網羅性の不足

- 該当箇所: `internal/redaction/value_detector.go:21-37`
- 問題: 以下の主要な secret 形式が検出されない。
  - GitHub fine-grained PAT（`github_pat_...`）: `\bgh[pors]_` に一致せず、キー名なしで現れると漏れる。
  - Slack の `xapp-`（App-level token）、`xoxe-`（refresh）、`xoxs-`: `xox[bpar]-` の文字クラス外。
  - AWS **Secret Access Key**（40 文字 base64 風）: Access Key ID（AKIA/ASIA）のみ検出。Secret 側は自己識別形式でないため困難だが、`aws_secret_access_key` を key=value パターン（`DefaultKeyValuePatterns`）に含めていない点は改善余地（`_KEY`/`key` の部分一致で拾えるのは `=` 区切りのときのみ）。
  - JWT（`eyJ...`. 形式）: パターンなし。
- 悪用/漏洩シナリオ: コマンド標準出力に fine-grained PAT が単体で現れた場合（キー名文脈なし）、redaction を素通りして command_group_summary として Slack に投稿される（slack_handler.go:526-537 は出力を最大 1000 文字まで転送する）。
- 推奨対応: 上記パターンの追加。特に `github_pat_[A-Za-z0-9_]{22,}`、`xox[abeprs]-`／`xapp-`、JWT。網羅性の限界は docs のリスク評価に追記済みか確認する。

### M-5: Slack 送信がログ呼び出しと同期実行され、リトライ込みで最長 30 秒超ブロックする（可用性）

- 該当箇所: `internal/logging/slack_handler.go:841-905`（`sendToSlack`: backoff 2s+4s+8s=14s ＋ 各試行の HTTP timeout 5s×4）、`internal/logging/multihandler.go:45-56`（同期 fan-out）
- 問題: `slog.Error(...)` 等の 1 回のログ呼び出しが、Slack 到達不能時に最大約 34 秒ブロックする。ログはコマンド実行のクリティカルパス（特権操作の失敗通知等）から呼ばれるため、外部サービス（Slack）の障害がツール全体の実行時間・タイムアウト挙動に波及する。攻撃者が egress を妨害することで実行を大幅に遅延させることも可能。
- 推奨対応: Slack 送信を goroutine ＋ バッファ付きキューに逃がし、シャットダウン時に flush する。少なくとも合計デッドライン（context）を課す。

---

## 🟠 Low

### L-1: `compileRedactionRegex` の失敗時ログが `slog.Warn`（デフォルトロガー）経由で、再帰の芽がある

- 該当箇所: `internal/redaction/redactor.go:140-156`
- 問題: この関数は `RedactingHandler.Handle` → `RedactText` の内側から呼ばれるが、失敗時に `slog.Warn` を使う。デフォルトロガーは通常 `RedactingHandler` を含むため、Warn 自身が再び redaction → 同じ compile 失敗 → Warn… という再帰構造になる。現状はパターンが `regexp.QuoteMeta` 済みで compile 失敗はほぼ到達不能だが、`failureLogger` を厳密に分離した設計思想（redactor.go:266-291）と矛盾している。あわせて、`RedactText` は**呼び出しごとに全パターンの regex を再コンパイル**しており（キャッシュなし）、長い出力の redaction で無駄なコストが大きい（`SensitivePatterns`/`valueDetectorPatterns` は事前コンパイルしているのと非対称）。
- 推奨対応: KeyValuePatterns の regex をコンストラクタで事前コンパイルする（失敗はその場でエラー返却）。ランタイムログが必要なら failureLogger を渡す。

### L-2: `SecurityLogger` が構築時の `slog.Default()` を固定で保持する

- 該当箇所: `internal/logging/security.go:15-19`
- 問題: bootstrap はフェーズ 1（暫定ロガー）→フェーズ 2（redaction+Slack 付き本ロガー）でデフォルトロガーを差し替えるが、`NewSecurityLogger` が差し替え前に呼ばれると、以後のセキュリティイベント（timeout 超過等）が redaction/Slack を経由しない古いロガーへ流れ続ける。初期化順序に暗黙依存する code smell。
- 推奨対応: `s.logger` を保持せず都度 `slog.Default()` を参照するか、ロガーを明示注入する。

### L-3: `handleErrorCommon` が errorMsg を redaction なしで stderr/stdout へ直接出力する

- 該当箇所: `internal/logging/pre_execution_error.go:96-127`
- 問題: `params.errorMsg` は設定パースエラー等に由来し、環境変数値やパス等の機密を含み得る。slog 経由の出力は `RedactingHandler` を通るが、`fmt.Fprint(os.Stderr, ...)`／`fmt.Print(...)` の直接出力は redaction を経ない。対話端末なら影響は小さいが、cron 等では stderr/stdout がメール・ログ収集基盤へ転送されることが多い。
- 推奨対応: 直接出力の前に `redaction.DefaultConfig().RedactText(errorMsg)` を適用する。

### L-4: `ValidateLogDir` の固定名 `.write_test` による DoS と、`MkdirAll` の中間パス symlink 追従

- 該当箇所: `internal/logging/safeopen.go:74-85`（`.write_test` + `O_EXCL`）、`safeopen.go:42-45`（`OpenFile` 内 `os.MkdirAll`）
- 問題:
  1. `.write_test` は固定名かつ `O_EXCL` のため、ログディレクトリに書ける他者（group 書き込み権限者等）が同名ファイル/シンボリックリンクを常置すると検証が恒久的に失敗する（DoS）。また作成と `os.Remove` の間に他プロセスの検証と競合し得る。
  2. `os.MkdirAll` は中間ディレクトリの symlink を追従する。ログパス自体は運用者設定のため実害は限定的だが、safefileio によるファイル open の保護と防御レベルが揃っていない。
- 推奨対応: テストファイル名に ULID 等のランダムサフィックスを付ける。ディレクトリ作成も openat 系（または事前検証済みパスのみ許可）に寄せる。

### L-5: 過剰 redaction — `key`/`basic` 等の部分文字列一致で無関係な値・キーが全消しされる

- 該当箇所: `internal/redaction/sensitive_patterns.go:39-51, 126-133`（`IsSensitiveKey`/`IsSensitiveValue` が同一パターン）、`sensitive_patterns.go:147-168`（`key` パターン）
- 問題: `(?i)(...|key|basic|...)` の部分一致のため、`keyboard`, `monkey=...`, 値中の "basically" 等を含む属性が丸ごと `[REDACTED]` になる。特に `IsSensitiveValue` は「値がキー名らしき単語を含むだけ」で値全体を消すため、運用時のデバッグ情報が大きく失われる（機密漏洩ではなく可用性・診断性の問題。fail-secure 方向ではある）。また `DefaultKeyValuePatterns` の `_PASSWORD`/`_TOKEN`/`_KEY`/`_SECRET` は case-insensitive の `password`/`token`/`key`/`secret` の完全な部分集合で冗長（重複 code smell）。
- 推奨対応: キー用パターンに語境界（`\b...\b` や `(^|_)key(_|$)`）を導入し、値用は値形式検出（ValueDetector）中心へ寄せる。冗長パターンを整理する。

### L-6: Slack 向け出力の truncation が UTF-8 のバイト境界で切るため、マルチバイト文字が壊れる

- 該当箇所: `internal/logging/slack_handler.go:528-531, 541-544, 706-710`
- 問題: `output[:truncationPoint]` はバイト単位スライスのため、日本語出力等でルーンの途中を切断し、不正な UTF-8 として Slack API に送られる（Slack 側で invalid_payload となり通知自体が失われる可能性）。セキュリティ通知の欠落につながり得る。
- 推奨対応: `strings.ToValidUTF8` を併用するか、ルーン境界で切る。

---

## 🔵 Info

### I-1: `ShutdownReporter.Report` が `*InMemoryErrorCollector` 以外を黙ってスキップする

- 該当箇所: `internal/redaction/reporter.go:29-34`
- 具象型への type assertion で分岐しており、別実装の collector を渡すと報告が無言で失われる。`ErrorCollector` インターフェースに `GetFailures()` を含めるのが素直。

### I-2: panic 値・スタックトレースが failureLogger（file/stderr）へ生のまま記録される

- 該当箇所: `internal/redaction/redactor.go:574-583`、`reporter.go:131-137`（`Err: %v` は `ErrLogValuePanic.PanicValue` を含む）
- LogValue() の panic 値が機密を含む場合、それが file/stderr に平文で残る。Slack を除外する設計（コメントに明記）は妥当なトレードオフだが、panic 値文字列に `RedactText` を掛ける余地はある。

### I-3: `if logger := slog.Default(); logger != nil` は常に真で、しかも `logger` を使わず `slog.Error` を呼ぶ

- 該当箇所: `internal/logging/pre_execution_error.go:111-120`
- `slog.Default()` は nil を返さない。取得した `logger` も未使用。dead check の code smell。

### I-4: `validateWebhookURL` はポート付き URL（`https://hooks.slack.com:8443/...`）を許容する

- 該当箇所: `internal/logging/slack_handler.go:164-168`（`Hostname()` はポートを除去）
- TLS + ホスト一致は保たれるため実害は小さいが、厳密には `parsedURL.Port()` の空チェックを足すとより堅い。

### I-5: `SlackHandler.applyAccumulatedContext` のグループ意味論が slog 標準と異なる

- 該当箇所: `internal/logging/slack_handler.go:907-943`
- `WithGroup` 後にレコードへ付いた属性はグループ内に置くのが slog の規約だが、ここでは蓄積属性のみグループ化しレコード属性は素のまま追加する。現状の build 系関数はトップレベルキーで抽出するため動作上は整合しているが、規約との乖離は将来の抽出バグの温床。

### I-6: `DefaultLogLineTracker` は旧式の `int64`+`atomic.LoadInt64` パターン

- 該当箇所: `internal/logging/log_line_tracker.go:21-43`
- `atomic.Int64` へ置き換えると alignment 問題の心配がなくなり、CLAUDE.md のモダン Go 指針とも整合する。動作自体は正しい。

---

## 観察された良好な防御層

1. **Webhook URL の宛先検証が fail-closed**（slack_handler.go:140-171）: HTTPS 強制、ホスト完全一致（大文字小文字正規化）、`allowedHost` 未設定時は全拒否。SSRF/宛先すり替えに対する適切な防御。
2. **redaction 失敗時の fail-secure 方針**: regex compile 失敗・再帰深度超過・LogValue() panic のいずれも `[REDACTION FAILED - OUTPUT SUPPRESSED]` へ置換し、機密を「出さない」側に倒している（redactor.go:52, 470-475, 546-557）。
3. **LogValuer の panic recovery と 2 段ログ**（redactor.go:566-597）: 詳細（panic 値・stack）は Slack を含まない failureLogger のみ、全宛先には安全な要約のみ、という情報分離が明確。
4. **failureLogger の循環依存検証**（redactor.go:304-372): `RedactingHandler` が failureLogger の handler chain に含まれる構成を起動時 panic で fail-fast に拒否し、panic recovery 中の無限再帰を構造的に防止。
5. **再帰深度制限**（`maxRedactionDepth=10`）による DoS（深いネスト・循環 LogValuer）対策。
6. **ValueDetector の placeholder `$` エスケープ**（value_detector.go:60-64): 置換文字列経由で捕捉グループ（秘密そのもの）が再注入される regex 置換の落とし穴を明示的に潰している。
7. **ログファイルのパーミッション**（safeopen.go:22-23: dir 0750 / file 0600）と `safefileio` 経由の symlink 保護付き open。
8. **Run ID に crypto/rand ベースの ULID**（safeopen.go:57-60）を使用し、推測可能な ID による相関攻撃を回避。
9. **dry-run モードでの Slack 送信抑止**（slack_handler.go:252-255）と、`slack_notify` 属性による明示的オプトイン送信（既定では外部送出しない）。
10. **リトライの分別**: 4xx（429 除く）は非リトライ、429/5xx のみ指数バックオフでリトライし、`ctx.Done()` を尊重（slack_handler.go:854-901）。ペイロードは再構築せず同一内容の再送のみで、リトライ時に追加情報が漏れる構造はない。
11. **エラー収集と shutdown 報告**（error_collector.go + reporter.go）: redaction 失敗を握りつぶさず運用者へ可視化する仕組みがあり、collector は mutex で並行安全、`GetFailures` はコピーを返す。
12. **ハンドラ群の immutability**: `WithAttrs`/`WithGroup` が常に新インスタンスを返し、`MultiHandler.Handle` は `r.Clone()` を渡すため、ハンドラ間の状態競合がない。
