# 実装計画書: redaction のカバレッジ拡張と Slack 送信の非同期化

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-09 |
| Review date | 2026-08-09 |
| Reviewer | isseis |
| Comments | - |

## 1. 実装概要

### 1.1 目的

0149 監査 D2 の未対応 Medium 3 件（M-2 / M-4 / M-5）を、`internal/redaction` と `internal/logging` を中心とした変更で解消する。設計の根拠・判断はすべて 02_architecture.md にあり、本計画はそれを実行可能なタスクへ分解する。設計内容の再掲は行わず、該当節を参照する。

### 1.2 実装原則

1. **既存挙動の非退行**: 既存テストの期待値文字列は変更しない。変更が避けられない箇所は 1.3.3 に列挙したものに限り、変更理由をテストのコメントとして残す。
2. **設計文書への追従**: フェーズの区切りと順序は 02_architecture.md 8 章に従う。設計文書と食い違う判断を採る場合は、1.4 に記録する。
3. **Go ソースは英語**: コメント・識別子・文字列リテラルはすべて英語で書く。本計画の日本語の記述をそのままコードへ持ち込まない。
4. **既存実装の再利用**: `Config.RedactText`、`ValueDetector`、`MultiHandler`、`compileRedactionRegex`、`sanitizeErrorForLog`、`bootstrap` の Phase 1 ハンドラ構成、既存のテスト用差し替え点 `newSlackHandlerFunc` をそのまま使う。
5. **各フェーズの完了判定**: Go コードを変更するフェーズ（Phase 1〜5）は `make fmt` → `make test` → `make lint` を、文書のみのフェーズ（Phase 6）は `make verify-docs` を完了条件に含める。各フェーズ固有の追加ゲートは各フェーズの完了条件に記す。

### 1.3 既存コード調査結果

実装前に確認した現状と、それが本計画に与える制約を示す。02_architecture.md に記載のない発見は「**設計文書に未記載**」と付す。

#### 1.3.1 `internal/redaction`

| 対象 | 現状 | 必要な対応 |
|---|---|---|
| `Config.RedactText` (`redactor.go:62`) | `KeyValuePatterns` を順に `performKeyValueRedaction` へ渡し、最後に `ValueDetector.Mask` を 1 回適用する | 変更なし。シグネチャも保つ |
| `performKeyValueRedaction` (`redactor.go:124`) | `:` 含む→コロン経路、空白含む→空白経路、それ以外→`performKeyValuePatternRedaction` | 変更なし |
| `performKeyValuePatternRedaction` (`redactor.go:220`) | キーが `=` を含むかで 2 つの正規表現を生成。`=` を含まない場合は `(?i)(key)(=)(\S+)` | 02_architecture.md 3.2.3〜3.2.5 のとおり選択肢と先頭境界を追加する。シグネチャは維持 |
| `compileRedactionRegex` (`redactor.go:140`) | 呼び出しのたびに `regexp.Compile`。失敗時は `slog.Warn` して nil を返す | 内部に上限付きキャッシュを差し込む。3 経路すべてが恩恵を受ける（02_architecture.md 3.2.7） |
| `DefaultKeyValuePatterns()` (`sensitive_patterns.go:147`) | 12 キー（`password`,`token`,`key`,`secret`,`api_key`,`_PASSWORD`,`_TOKEN`,`_KEY`,`_SECRET`,`Bearer `,`Basic `,`Authorization: `） | 変更なし。群 A=`password`/`api_key`、群 B=`token`/`key`/`secret`、群 C=`_` 始まりの 4 キー、残り 3 キーは第 1・第 2 経路 |
| `valueDetectorPatterns` (`value_detector.go:12`) | 7 パターンを構造体リテラルで保持し、`Mask` が固定順に適用 | 4 パターンを追加し、既存 7 種の後で適用する（02_architecture.md 3.3.1） |
| `Mask` の `$` エスケープ (`value_detector.go:64`) | `strings.ReplaceAll(placeholder, "$", "$$")` を全パターンで共用 | 変更なし。追加パターンも同じ `escapedPlaceholder` を使う |
| `regex_cache.go` | 存在しない | 新規作成 |

**既存テストへの影響（実測）**:

- `internal/redaction/redactor_test.go` に 3 セグメントの JWT を含む入力は 1 件（2455 行目 `Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc`）。コロン経路が先に行末まで置換するため、JWT パターン追加後も結果は変わらない。
- `internal/redaction/value_detector_test.go` の JWT を含む 4 箇所（68・157・182・184 行目）はいずれも `Bearer ` プレフィックス付きであり、既存の `bearerToken` パターンが先に消費する。JWT パターンは既存 7 種の**後**に適用するため結果は変わらない。
- `internal/runner/base/security/logging_security_test.go` の完全一致検証（85 行目 `api_key=abc123def`、102 行目 `Authorization: Bearer eyJ...`）は V1 とコロン経路のみを使うため影響を受けない。
- 群 B の厳しい先頭境界により、`token=abc Bearer xyz Authorization: Basic dGVzdA==`（`redactor_test.go:223`）はテキスト先頭が群 B の先頭境界を満たさず V1 へ落ちる。既存の期待値がそのまま成立する。

#### 1.3.2 `internal/logging`

| 対象 | 現状 | 必要な対応 |
|---|---|---|
| `SlackHandler` (`slack_handler.go:82`) | `webhookURL` / `httpClient` / `backoffConfig` を自身が保持 | 3 フィールドを `slackSender` へ移し、`sender *slackSender` を持つ（02_architecture.md 3.5） |
| `NewSlackHandler` (`slack_handler.go:175`) | `validateWebhookURL` → 既定値適用 → 構造体を返す | 送信失敗ロガーの構成検証、`slackSender` の生成、ワーカー起動を追加 |
| `validateWebhookURL` (`slack_handler.go:142`) | `https` 以外を拒否し、`parsedURL.Hostname()` と `AllowedHost` を小文字化して完全一致で比較する | 変更なし。テスト側の制約は 1.3.3 を参照 |
| `Handle` (`slack_handler.go:225`) | 末尾で `s.sendToSlack(ctx, message)` を同期呼び出し | 投入のみに変更（02_architecture.md 6.2 上段） |
| `WithAttrs` / `WithGroup` (`278` / `302`) | 全フィールドをコピーした新 `SlackHandler` を返す | `sender` をポインタでコピーする |
| `sendToSlack` (`slack_handler.go:862`) | `SlackHandler` のメソッド。`slog.Debug/Info/Warn/Error` を 10 箇所で使用し、`//nolint:gosec`（G704 が 1 件、G706 が 5 件）を伴う | `(*slackSender)` のメソッドへ移し、10 箇所すべてを送信失敗ロガー経由へ置換する。`nolint` 注釈は行単位のまま移設する |
| `sanitizeErrorForLog` (`slack_handler.go:848`) | `*url.Error` から URL を除去 | 変更なし。移設後の送信経路からそのまま呼ぶ |
| `MultiHandler.Handlers()` (`multihandler.go:59`) | 内部スライスのコピーを返す。実在を確認済み | 構成検証の再帰に利用する |
| `NewConditionalTextHandler` / `NewInteractiveHandler` | それぞれ `*ConditionalTextHandler` / `*InteractiveHandler` を返す | 構成検証の受理型に含める |
| `handler_chain.go` / `slack_sender.go` | 存在しない | 新規作成 |
| `test_helpers.go` | `//go:build test` 付きで `NewSecurityLoggerWithLogger` のみ | 変更不要（6.2） |
| Go バージョン | `go.mod` は `go 1.26.2` | `slog.DiscardHandler`（Go 1.24 追加）を受理型に含めてよい |

#### 1.3.3 更新が必要な既存テスト（実測）

**構造体リテラルによる構築**: `internal/logging/slack_handler_test.go` の `&SlackHandler{...}` は **16 箇所**である（02_architecture.md 3.1 の記述と一致する）。

| 行 | 所属テスト | 送信経路への到達 | 対応 |
|---|---|---|---|
| 37, 71, 98, 119, 133, 147, 192, 205 | `TestSlackHandler_WithAttrs` / `_WithGroup` / `_WithAttrsAndGroups` / `_WithAttrs_PreservesLevelMode` / `_WithGroup_PreservesLevelMode` / `_ApplyAccumulatedContext` / `_WithAttrsEmptySlice` / `_WithGroupEmptyString` | しない | 変更不要（nil 送信機構の契約、02_architecture.md 3.4.3） |
| 518, 615 | `TestSlackHandler_Enabled` / `_Enabled_LevelMode` | しない | 変更不要 |
| 631, 646 | `TestSlackHandler_Handle_NoSlackNotify` / `_Handle_SlackNotifyFalse` | しない（`slack_notify` 判定で復帰） | 変更不要 |
| 823 | `TestSlackHandler_Handle_WithMockServer` | する | `NewSlackHandler` 構築へ移行し、`Flush` を同期点として挟む |
| 864, 888 | `TestSlackHandler_SendToSlack_Retry` | する（`handler.sendToSlack` を直接呼ぶ） | `slackSender` を直接構築して `send` を呼ぶ形へ移行 |
| 959 | `TestSlackHandler_WithRedactingHandler` | する | `NewSlackHandler` 構築＋`Flush` へ移行 |

**削除するフィールドへの参照**: 上記の構築サイトとは別に、`TestNewSlackHandlerWithOptions`（`slack_handler_test.go:370` 開始）が削除対象の 3 フィールドを **6 箇所**で直接検証している。移設後は `handler.sender` 越しの参照へ書き換える必要があり、放置すると Phase 4 がコンパイルできない（02_architecture.md 3.1 の表に反映済み。以下は行単位の内訳）。

| 行 | 現在の参照 | 移行先 |
|---|---|---|
| 386 | `handler.httpClient` | `handler.sender.httpClient` |
| 387 | `handler.backoffConfig` | `handler.sender.backoffConfig` |
| 407 | `handler.httpClient.Timeout` | `handler.sender.httpClient.Timeout` |
| 408 | `handler.backoffConfig.Base` | `handler.sender.backoffConfig.Base` |
| 409 | `handler.backoffConfig.RetryCount` | `handler.sender.backoffConfig.RetryCount` |
| 467 | `handler.webhookURL` | `handler.sender.webhookURL` |

**ベンチマークファイル**: `internal/logging/slack_handler_benchmark_test.go` は `SlackHandler` を構築せず、削除対象の 3 フィールドも参照していない（含むのは `createBenchmarkCommandResults` と 3 つの `BenchmarkExtractCommandResults*` のみ）。**変更不要**であり、本計画の対象ファイルに含めない（02_architecture.md 3.1 に反映済み）。

**`internal/runner` の e2e テスト**: `//go:build e2e && test` の 2 ファイル（計 7 テスト）は実在の `logging.NewSlackHandler` を使い、送信の同期性に依存している。3 行目の `e2e_slack_redaction_test.go` は `//go:build test` であり既に `make test` で実行されているが、比較のために併記する。

| ファイル::テスト | 依存の形 | 対応 |
|---|---|---|
| `internal/runner/e2e_slack_webhook_test.go::TestE2E_SlackWebhookWithMockServer` | `Execute` 直後に待機なしで `receivedPayloads` を読む | `Flush` を同期点として挿入し、`receivedPayloads` を `sync.Mutex` で保護する |
| `internal/runner/e2e_slack_webhook_separation_test.go` の 6 テスト | `time.Sleep(500ms)` 後にスライスを読む（**6 箇所**: 126・227・294・343・414・491 行目、1 テストにつき 1 箇所） | `Sleep` を `Flush` へ置き換え、スライスを `sync.Mutex` で保護する |
| `internal/runner/e2e_slack_redaction_test.go` の 2 テスト（`//go:build test`。既に `make test` で実行される） | 独自の `MockSlackHandler` を使い、実 `SlackHandler` を経由しない | 変更不要 |

**実行経路**: `make test`（= `unit-test`）は `CGO_ENABLED=1 go test -tags test -race -p 4 ./...` を実行し（macOS 以外ではさらに `CGO_ENABLED=0` の 2 回目を実行する）、`e2e` タグを付けない。`make e2e-test` はビルド済みバイナリの dry-run と Python スクリプトを実行するだけで、`go test -tags e2e` を呼ばない。`.github/workflows/ci.yml` も `-tags test` のみである。したがって上記 7 つの e2e テストは**リポジトリのどのターゲットからも実行されない**。さらに `e2e` タグ付きのファイルは `make test` でも `make lint` でもコンパイルされないため、書き換えても型エラーが既定のゲートに現れない。移行した検証が退行しないよう、Phase 4 で Makefile ターゲットを新設して CI に組み込む（ステップ 4-3）。なお実行してみると、これらのテストは macOS では 0163 以前から 7 件中 3 件が失敗する（設定が `/usr/bin/echo` を指すが macOS では `/bin/echo` にある）。CI は `ubuntu-latest` のため CI では 7 件とも成立する。この失敗は本タスクの原因でも対象でもないため修正せず、ターゲットの適用範囲で切り分ける（ステップ 4-3）。

#### 1.3.4 `internal/runner/bootstrap` と `cmd/runner`

| 対象 | 現状 | 必要な対応 |
|---|---|---|
| `AddSlackHandlers` (`logger.go:217`) | 生成した `*SlackHandler` を `allHandlers` に積むだけで保持しない | 生成物をパッケージ変数へ保持し、部分失敗時と再呼び出し時に `Close` する（02_architecture.md 3.4.9） |
| `newSlackHandlerFunc` (`logger.go:61`) | `logging.NewSlackHandler` を指すパッケージ変数。既存テストが差し替えて `SlackHandlerOptions` を検査している（`environment_test.go`） | そのまま再利用する。部分失敗テスト・再呼び出しテスト・オプション伝播テストの唯一の注入点である |
| `phase1BaseHandlers` (`logger.go:53`) | Phase 1 の葉ハンドラの並び | `SlackHandlerOptions.FailureHandlers` としてそのまま渡す |
| `ReportRedactionFailures` (`logger.go:266`) | 戻り値なしで報告のみ | `FlushSlackNotifications` の形の手本にする |
| `SetupSlackLogging` (`environment.go:107`) | `AddSlackHandlers` の唯一の本番呼び出し元 | 変更不要 |
| `saveAndRestoreGlobals` (`logger_test.go:21`) | 5 つのパッケージ変数と既定ロガーを退避・復元する | 新設するハンドラ保持変数を追加する。復元前に `Close` してワーカーを残さない |
| `cmd/runner/main.go` `main` (`161`) | `exitCode := mainWithExitCode(runID)`（201 行目）→ `bootstrap.ReportRedactionFailures()`（204 行目）→ `os.Exit(exitCode)`（207 行目） | 201 行目と 204 行目の間に `bootstrap.FlushSlackNotifications()` を挿入 |
| `cmd/record` / `cmd/verify` | `AddSlackHandlers` を呼ばない | 変更しない（AC-28） |

#### 1.3.5 文書

| 対象 | 現状 | 必要な対応 |
|---|---|---|
| `docs/user/security-risk-assessment.ja.md` 240〜253 行目 | 「値ベース検出」の一覧が 7 項目、「限界」に空白入り値と非同期配送の記述なし | 4 項目の追加と、限界・配送方式の追記（AC-33, AC-34） |
| `docs/user/security-risk-assessment.md` | 上記の英語版 | `/mktrans` で反映 |
| `docs/dev/architecture_design/security-architecture.md` 600 行目・863 行目 | `SlackHandler` の構造体定義を 2 箇所で再掲 | `webhookURL` / `httpClient` / `backoffConfig` の移動を反映 |
| `docs/dev/architecture_design/security-architecture.ja.md` 597 行目・860 行目 | 同上の日本語版（02_architecture.md 3.1 の一覧に追加済み） | 英語版と同じ 2 箇所を更新する |
| `docs/translation_glossary.md` | 本タスク由来の用語は 4 語のみ登録済み（`送信失敗ロガー` 184、`flush` 186、`送信機構` 482、`終了要求チャネル` 484） | 未登録の用語を追加（AC-35） |
| `make verify-docs` (`Makefile:692`) | `scripts/verification/run_all.sh` が `docs/user` 配下の `*.ja.md` と `*.md` の構造一致を検査する | Phase 6 の完了条件に含める |

### 1.4 設計文書との差異

02_architecture.md と本計画で扱いが分かれる点を明示する。いずれも設計の意図を変えるものではなく、記述の粒度または場所の調整である。

なお本計画の作成時に発見した 02_architecture.md の誤り 3 件（3.1 の構造体リテラル数、3.1 のベンチマークファイルの追随要否、3.4.11 と 2.1 の間の `GSCR_SLACK_SYNC` の解釈場所の矛盾）は、差異として残さず設計文書本体を修正した。以下の表に挙げるのは、修正ではなく粒度の調整に留まるものだけである。

| 項目 | 02_architecture.md の記述 | 本計画の扱い | 理由 |
|---|---|---|---|
| `defaultSendTimeout` の可視性 | 3.4.1 は既定値の名前を定めていない | `DefaultSendTimeout` として公開する（`DefaultFlushTimeout` と対称） | `bootstrap.parseSlackEnvSettings` が未設定・不正値時のフォールバックとして参照する。非公開にすると 40 秒という値が 2 箇所に重複し、AC-19 の検証と乖離しうる |
| `afterDequeue` フック | 7.3 が「取り出しの直後に同期点を設けるテスト用フック」を要求するが、3.4.1 の `slackSender` の定義には含まれない | `slackSender` の非公開フィールドとして追加する | 7.3 のテストを実現する最小の形。3.4.1 の構造体定義に対する本計画からの追加である |
| `message_type` 別の内訳 | 3.4.8 は flush 時に webhook ごとの `message_type` 別内訳を出力すると定めるが、3.4.1 の `FlushStats` にその欄がない | `slackSender` が `mu` で保護した `map[string]int` の内訳を持ち、`Flush` の中で送信失敗ロガーへ自ら出力する。`FlushStats` は合計のみを運び、`bootstrap` はそれを webhook ごとに報告する | `FlushStats` に可変長のマップを持たせずに 3.4.8 の要求を満たす |

## 2. 実装ステップ

### Phase 1: 正規表現キャッシュの導入

**対象ファイル**: `internal/redaction/regex_cache.go`（新規）、`internal/redaction/redactor.go`、`internal/redaction/regex_cache_test.go`（新規）

#### ステップ 1-1: キャッシュの実装

- [x] `regex_cache.go` に `sync.Map` ベースのキャッシュと上限定数 `maxRegexCacheEntries = 256` を定義する。キーはコンパイル対象の正規表現文字列、値は `*regexp.Regexp`。
- [x] エントリ数を `atomic.Int64` で数え、上限到達後はキャッシュへ格納せず毎回コンパイルする（02_architecture.md 3.2.7）。エビクションは行わないため、上限に達した場合は**後から現れたパターンだけ**が恒久的に毎回コンパイルへ落ちる。上限値の妥当性を判断できるよう、定常状態のエントリ数の見積りを英語コメントで残す: 各キーは 3 経路（コロン・空白・パターン）のちょうど 1 つにルーティングされるため、既定 12 キーは 12 エントリであり、`KeyValuePatterns` を設定で拡張しても 1 キーにつき 1 エントリしか増えない。したがって 256 は既定の 20 倍を超える余裕がある。なお上限判定と格納は別操作であるため、並行実行の境界ではエントリ数が上限をわずかに超えうる（近似の上限。実害はなく、コードコメントにその旨を残す）。
- [x] `compileRedactionRegex` の先頭でキャッシュを引き、末尾で成功結果のみ格納する。コンパイル失敗時は格納せず、現行どおり nil を返す。

#### ステップ 1-2: キャッシュのテスト

- [x] `TestRegexCache_ReturnsSameCompiledRegex`: 同じパターン文字列に対して同一の `*regexp.Regexp` ポインタが返ること。
- [x] `TestRegexCache_LimitStopsCaching`: 上限を超えたパターンがキャッシュされず、それでも正しい置換結果を返すこと。
- [x] `TestRegexCache_CompileFailureIsNotCached`: コンパイル失敗経路を、`compileRedactionRegex` に不正なパターン文字列を直接渡して検証する。`RedactText` の 3 経路はすべて `regexp.QuoteMeta` でエスケープした断片から正規表現を組み立てるため、`KeyValuePatterns` からコンパイル失敗を生む入力は存在しない（着手時に実測済み）。検証内容: nil を返すこと（フェイルセキュア。呼び出し側が `RedactionFailurePlaceholder` を返すのは既存挙動）、失敗結果がキャッシュされず 2 回目も nil を返してエントリ数が増えないこと、同一 `Config` の他の正常なキーの置換が影響を受けないこと。
- [x] `TestRegexCache_ConcurrentAccess`: 複数 goroutine から同時に `RedactText` を呼び、結果が単一 goroutine の場合と一致すること。加えて、各 goroutine が異なるキー（異なるパターン）を同時にコンパイル・格納する局面でも、それぞれの置換結果が正しいことを確かめる。

**完了条件**: `make fmt && make test && make lint` が通り、`internal/redaction` の既存テストの期待値を 1 文字も変更していないこと。

### PR-1 作成ポイント: regex compilation cache

**対象ステップ**: 1-1 / 1-2

**推奨タイトル**: `feat(0163): cache compiled redaction regexes`

**レビュー観点**: 上限判定と格納が別操作であることを踏まえ、エントリ数の増分が `LoadOrStore` の `loaded` を尊重し二重計上しないこと / 上限到達後にキャッシュを止める分岐の正しさ / コンパイル失敗をキャッシュせずフェイルセキュアを保つこと / `sync.Map` の並行利用で既存の置換結果が変わらないこと

**実装モデル要件**: standard

**判定理由**: 挙動を変えないリファクタリングであり、未確立の設計判断・panel-mode トリガ・Conditional checks のいずれにも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: キー名ベース redaction の区切り・引用符拡張

**対象ファイル**: `internal/redaction/redactor.go`、`internal/redaction/redactor_test.go`

#### ステップ 2-1: 実装

- [x] `redactor.go` にパッケージレベルの一般語キー集合 `commonWordKeys`（小文字化したキーを要素とする `map[string]struct{}`。実行時に変更しない）を追加する。初期値は `key` / `token` / `secret` の 3 語。設定項目にしない理由を英語コメントで残す（02_architecture.md 3.2.4）。
- [x] キー文字列から群を導出する非公開関数 `keyBoundaryGroup(key string) boundaryGroup` を追加する。判定順は 02_architecture.md 3.2.4 の規則 4〜6 に従う。
- [x] 群ごとの先頭境界の正規表現断片を返す非公開関数を追加する。3 つの群の境界の定義は 02_architecture.md 3.2.4 の表に従い、本計画では再掲しない。
- [x] `performKeyValuePatternRedaction` の「キーが `=` を含まない」分岐で、V3 → V2 → V1 の順に並べた選択肢を生成する（02_architecture.md 3.2.3）。区切りの空白は `[ \t]*` のみとし改行を含めない。
- [x] V2・V3 のキー名側の閉じ引用符の扱い、値側の引用符の保持、閉じ引用符がない場合の行末までの置換を、02_architecture.md 3.2.5 のとおりに実装する。`"` 用と `'` 用の選択肢は別々に書く（RE2 に後方参照がないため）。
- [x] キーが `=` を含む分岐は変更しない。
- [x] **レビューで判明した 2 点を 02_architecture.md へ反映のうえ実装した**（設計文書本体を修正済み）。(1) V2 の値の先頭が `{` / `[` の場合は一致させない（3.2.2 に追記）。空白のない構造化データで行の残り全体を飲み込み、兄弟フィールドを消す過剰 redaction を防ぐため。(2) 群 B の二重引用符版 V3 にだけ緩い先頭境界を適用する（3.2.4 に追記）。厳しい境界のままでは `TOKEN="abc def"` が V1 へ落ち秘密の後半が平文で残るため。単一引用符版は `unexpected token: '}'` を守るため緩めない。
- [x] 各選択肢が捕捉するのは値のみとする。境界・キー・キー側引用符・区切り・値側引用符は一致範囲の内側にあるため、入力からそのまま複写でき、捕捉グループを必要としない。捕捉グループ数を選択肢あたり 1 個に抑えることで、キー 1 個あたり約 20 個の捕捉グループを追跡する場合に比べ大幅に速くなる。
- [x] **`RedactText` にキーの部分文字列による事前判定を入れる**（レビューで判明した劣化への対応）。選択肢を 4 本に増やした結果、正規表現プログラムが大きくなり、キー 1 個あたりの走査コストが上がる。どの選択肢もキーをリテラルとして必要とするため、テキストがキーを大文字小文字を無視して含まないならば一致は起こりえない。そこでテキストを 1 度だけ ASCII 小文字化し、各キーについて `strings.Contains` で存在を確かめてから正規表現を走らせる。あるキーが実際に置換を行ったときだけ小文字化をやり直す。
- [x] **事前判定は ASCII のテキストにのみ適用する**。Go の正規表現は Unicode の規則で大文字小文字を畳み込むため、U+212A KELVIN SIGN が `(?i)k` に、U+017F LATIN SMALL LETTER LONG S が `(?i)s` に一致する。ASCII 小文字化はこれらを変換しないので、非 ASCII を含むテキストに事前判定をかけると `paſſword=s3cr3t`（正規表現は一致する）を「一致しえない」と誤判定して redaction を弱めてしまう。非 ASCII を 1 バイトでも含むテキストでは事前判定を行わず、従来どおり全キーの正規表現を走らせる。`TestRedactText_PreFilterDoesNotWeakenRedaction` がこのケースを固定しており、ASCII 判定を外すと失敗する。

**実測値**（`go test -bench 'BenchmarkRedactText' -benchtime 3s -benchmem -count 3`、Apple M3 Pro、コミット済みの `BenchmarkRedactText` をそのまま使用。3 回の中央値）:

| ベンチマーク | 拡張前（main） | 事前判定なし | 事前判定あり（現状） |
|---|---|---|---|
| `BenchmarkRedactText` | 17.0 µs / 3562 B | 32.5 µs / 6279 B | **15.6 µs / 3259 B** |
| `BenchmarkRedactText_NoSensitiveData` | 13.6 µs / 3078 B | 29.1 µs / 5447 B | **5.85 µs / 1204 B** |

事前判定を入れる前は拡張前比で 1.91 倍・2.14 倍の劣化であった。`RedactLogAttribute` が文字列属性ごとに `RedactText` を呼ぶため、置換対象を含まない行の経路（下段）が実運用では支配的であり、そこが最も悪化していた。事前判定によりいずれも拡張前を下回る（0.92 倍・0.43 倍）。
- [x] 再構築を `FindAllStringSubmatchIndex` の結果から行い、`ReplaceAllStringFunc` は使わない（**本計画からの変更**。当初は「`ReplaceAllStringFunc` 内の再構築ロジックを拡張する」としていた）。`ReplaceAllStringFunc` のコールバックが受け取るのは一致した部分文字列だけであり、その文字列に正規表現を再適用すると、文脈では先頭境界を満たさず V1 へ落ちた一致が、先頭の文脈を失ったことで V2・V3 として再解釈されうる（群 A の先頭境界がテキスト先頭を許すため。例: `xpassword="a b"` の V1 一致 `password="a` を単体で再一致させると V3 になる）。どの選択肢が一致したかは、選択肢ごとのキー部分マッチが一致に参加したかで判別する。

#### ステップ 2-2: テスト

`internal/redaction/redactor_test.go` に以下を追加する。既存テスト関数の期待値は変更しない。

- [x] `TestRedactText_QuotedValue`: `password="abc def"` → `password="[REDACTED]"`、`password='abc def'` → `password='[REDACTED]'`、閉じ引用符なし `password="abc def` → 行末まで置換。置換後に元の値の断片が残らないことを `assert.NotContains` で確かめる。**追加**: 群 B のキーがテキスト先頭に現れる `TOKEN="abc def"` → `TOKEN="[REDACTED]"`、複数一致、行をまたぐ入力、および閉じ引用符がない場合に改行で止まること。さらに `TestRedactText_QuotedValueKnownLimits` で、対象外と定めたエスケープ引用符（3.2.5）と空の引用符付き値の結果を固定する。
- [x] `TestRedactText_JSONForm`: `"password": "secret"` → `"password": "[REDACTED]"`。
- [x] `TestRedactText_SeparatorVariants`: `password: secret`、`password = secret`、`password :secret`、`password=\tsecret` の置換。複数一致と行をまたぐ入力も含め、各ケースで `assert.NotContains` により値の断片が残らないことを確かめる。
- [x] `TestRedactText_AlternativePriority`: `password="abc def"` が V1 ではなく V3 として処理されること（結果に ` def"` が残らないこと）と、`monkey="a b"` が V1 へ落ちて `monkey=[REDACTED] b"` になること。
- [x] `TestRedactText_KeyGroupBehavior`: `DefaultKeyValuePatterns()` の全キーを表駆動で回し、群ごとの期待値を固定する。群 C は先頭境界を課さないため V2 / V3 も無条件に適用される点に注意し、`_KEY=secret` / `_KEY: secret` / `_KEY = secret` / `_KEY="a b"` / `"_KEY": "secret"` の 5 形すべてが置換されることを明示的に固定する（「現行どおり」という曖昧な期待値にしない）。群 A は 3 形式すべて置換、群 B は二重引用符付きの値（緩い先頭境界。3.2.4 の例外）と識別子内境界（`_` / `-` / `.`）および引用符付きキー名のみ置換。
- [x] `TestKeyBoundaryGroup_Classification`: 既定 12 キーが意図した群に落ちること、および `KeyValuePatterns` に `passphrase` を追加すると群 A として扱われること。
- [x] `TestRedactText_ExistingBehaviorPreserved`: `Authorization: Bearer xxx`、`Bearer xxx`、`Basic xxx`、キー自身が `=` を含む場合の結果が現行と同一であること。
- [x] `TestRedactText_NoNewOverRedaction`: 02_architecture.md 3.2.6 の「置換されない」行をすべて固定する（`Primary key: id`、`unexpected token: '}'`、`map[key:value]`、`configMapKeyRef: {key: LOG_LEVEL}`、`keyboard: qwerty`、`/usr/local/key/path`、`--timeout=30`、`password:\nsecret`）。群 B の先頭境界が半角スペース・`[`・`{` の 3 ケースを、それぞれ独立したテーブル行として明示する。**追加**: 値の先頭が `{` / `[` のため V2 を適用しない 3 ケース（`{"password":{"a":1},"port":80}`、`{"api_key":["a","b"],"port":80}`、`password: {json: here} trailing`）も固定する。
- [x] `TestRedactText_IntentionalOverRedaction`: `"key": "us-east-1"` が `"key": "[REDACTED]"` になること（第 1 類）。**追加**: 群 A のキーが散文中でコロンを伴う第 2 類（`failed to read password: permission denied` → `... password: [REDACTED] denied`）も固定する。いずれも AC-09 の除外規定に該当する意図した変更である旨を英語コメントで残す。
- [x] `TestRedactText_LongTextUnchanged`: 置換対象を含まない 10 KB 程度のテキストに対し `RedactText` の戻り値が入力と一致すること。
- [x] **追加** `TestRedactText_PreFilterDoesNotWeakenRedaction`: 事前判定が一致を取りこぼさないことを固定する。キー一致が大文字小文字を無視すること、非 ASCII のテキスト（`paſſword=s3cr3t`、`日本語 password=...`）でも置換されること、先行するキーの置換後に後続のキーが取りこぼされないこと。

**完了条件**: `make fmt && make test && make lint` が通り、`internal/redaction` と `internal/runner/base/security` の既存テストを 1 行も変更せずに合格すること。

### PR-2 作成ポイント: key-name redaction coverage

**対象ステップ**: 2-1 / 2-2

**推奨タイトル**: `feat(0163): extend key-name redaction to separators and quoted values`

**レビュー観点**: 群 A / 群 B / 群 C の先頭境界が 02_architecture.md 3.2.4 の表と一致すること / V3 → V2 → V1 の選択肢順が保たれ引用符付きの値が V1 へ落ちないこと / 既存テストの期待値が 1 行も変わっていないこと（AC-08） / 新たな過剰 redaction が `"key": "us-east-1"` の 1 ケースに限られること（AC-09）

**実装モデル要件**: frontier-required

**判定理由**: redaction 層というセキュリティゲートそのものを書き換えるステップである（`mkplan.md` step 8 の panel-mode トリガ「security-gate」）。検出範囲を広げながら（AC-01〜AC-03）、既存の置換結果がバイト単位で変わらないこと（AC-08）と、新たな過剰 redaction が列挙した 1 ケースに限られること（AC-09）を同時に証明する必要があり、正規表現の選択肢順と先頭境界の設計を一度で正しく組む判断が要る。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: 値形式検出パターンの追加

**対象ファイル**: `internal/redaction/value_detector.go`、`internal/redaction/value_detector_test.go`

#### ステップ 3-1: 実装

- [ ] `valueDetectorPatterns` に `githubPAT` / `slackPrefixToken` / `jwt` / `slackWebhookURL` の 4 フィールドを追加する。各パターンの検出対象・形の仕様・置換後の形は 02_architecture.md 3.3.1 の表と 3.3.2（JWT の 3 条件）に従い、本計画では再掲しない。既存 7 パターンの正規表現は変更しない。
- [ ] `Mask` の末尾に、既存 7 種の後で `githubPAT` → `slackPrefixToken` → `jwt` → `slackWebhookURL` の順に適用する処理を追加する（02_architecture.md 3.3.1「適用順序」）。すべて既存の `escapedPlaceholder` を使い、`slackWebhookURL` のみホスト部の捕捉グループを保持する形で置換する。

#### ステップ 3-2: テスト

`internal/redaction/value_detector_test.go` に以下を追加する。

- [ ] `TestValueDetector_GitHubFineGrainedPAT`: `github_pat_` に続く列が単体で現れた場合に置換されること。最小長のちょうど境界（下回る場合は非置換、満たす場合は置換）を対にして固定する。
- [ ] `TestValueDetector_SlackPrefixTokens`: `xapp-` / `xoxe-` / `xoxs-` が置換され、既存の `xoxb-` / `xoxp-` / `xoxa-` / `xoxr-` の検出結果が変わらないこと。最小長の境界を対にして固定する。
- [ ] `TestValueDetector_JWT`: 3 セグメントの JWT が置換されること、署名部が空（`alg=none`）の形も置換されること、ヘッダ部・ペイロード部の最小長 10 文字のちょうど境界（9 文字は非置換、10 文字は置換）を対にして固定すること。
- [ ] `TestValueDetector_SlackWebhookURL`: `https://hooks.slack.com/services/T000/B000/XXXX` がホスト部を保持したまま置換されること。誤検出の対として、`/services/` を含まない `https://hooks.slack.com/` と、`hooks.slack.com` 以外のホストの URL が置換されないことを固定する。
- [ ] `TestValueDetector_FreeTextEmbedding`: 上記 3 形式が、コマンド標準出力に相当する自由テキスト（前後に散文がある文字列）へ埋め込まれた場合でも置換されること。
- [ ] `TestValueDetector_FalsePositives`: `github_pattern`、`xapple`、ドットを含まない `eyJhbGciOiJIUzI1NiJ9`、ドットが 1 個だけの文字列、ドットが 3 個の文字列が置換されないこと。
- [ ] `TestValueDetector_PlaceholderWithDollarNoReinjection`: プレースホルダーを `[$1]` にした `ValueDetector` で、追加 4 パターンに一致する入力を `Mask` した結果に元の秘密の断片が含まれないこと。

**完了条件**: `make fmt && make test && make lint` が通り、`internal/redaction/value_detector_test.go` と `internal/redaction/redactor_test.go` の既存期待値が変わらないこと。

### PR-3 作成ポイント: value-format detection patterns

**対象ステップ**: 3-1 / 3-2

**推奨タイトル**: `feat(0163): detect fine-grained PATs, Slack prefixes, JWTs and webhook URLs`

**レビュー観点**: 4 パターンが既存 7 種の後に適用され既存の検出結果を変えないこと / JWT のドット 2 個固定と最小長により誤検出が抑えられていること / `slackWebhookURL` がホスト部を保持すること / 追加パターンが `escapedPlaceholder` を共用し `$` の再注入が起きないこと（AC-16）

**実装モデル要件**: standard

**判定理由**: 既存パターンを変更しない純粋な追加であり、未確立の設計判断・panel-mode トリガ・Conditional checks のいずれにも該当しない。誤検出リスクは境界値テストで閉じる。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: `slackSender` の導入と `SlackHandler` の非同期化

**対象ファイル**: `internal/logging/slack_sender.go`（新規）、`internal/logging/handler_chain.go`（新規）、`internal/logging/slack_handler.go`、`internal/logging/slack_sender_test.go`（新規）、`internal/logging/handler_chain_test.go`（新規）、`internal/logging/slack_handler_test.go`、`internal/runner/bootstrap/logger.go`、`internal/runner/bootstrap/logger_test.go`、`internal/runner/e2e_slack_webhook_test.go`、`internal/runner/e2e_slack_webhook_separation_test.go`、`Makefile`、`.github/workflows/ci.yml`

#### ステップ 4-1: テスト共通規則の確立（本フェーズと Phase 5 の全テストに適用する）

以下は個別のテストごとに書かず、1 度だけ定めて全箇所に適用する。実装時はこの 3 規則を、`slackSender` を伴うハンドラやモックサーバを生成するすべてのテストに漏れなく適用する。

- [ ] **規則 R1（モックサーバの構成）**: `NewSlackHandler` は `https` 以外の webhook URL を拒否するため、モックサーバは `httptest.NewTLSServer` で立て、`SlackHandlerOptions.HTTPClient` に `server.Client()` を渡し、`AllowedHost` に `url.Parse(server.URL)` の `Hostname()`（ポートを含まない）を渡す。`InsecureSkipVerify` は使わない（`server.Client()` が正規の手段であり、新たな `gosec` 抑止を導入しない）。既存の `mustWebhookAllowedHost` は `internal/runner` の `//go:build e2e && test` ファイルにあり他パッケージから使えないため、`internal/logging` と `internal/runner/bootstrap` はそれぞれ自パッケージの `_test.go` 内に同等の小さなヘルパーを置く。
- [ ] **規則 R2（資源の後始末）**: `NewSlackHandler` / `AddSlackHandlers` / `httptest.New*Server` で資源を得た**直後**に、`t.Cleanup` または `defer` で `Close` を登録する。テスト末尾ではなく取得地点に置き、後続のアサーションが失敗しても実行されるようにする。これによりワーカーが後続テストへ漏れない。
- [ ] **規則 R3（ワーカー終了の観測）**: 「ワーカーが終了していること」は `runtime.NumGoroutine()` の完全一致では検証しない（プロセス全体で共有される値であり、`httptest` の接続や `-p 4` の並列実行で揺れる）。`internal/logging` のテストは同一パッケージから `slackSender.done` チャネルが閉じたことを観測する。`internal/runner/bootstrap` のテストは、`Close` 済みハンドラへの新たな投入が `Dropped` に計上されることで受付停止を確認する。`NumGoroutine` を使う場合は `require.Eventually` による上限の確認に留める。

#### ステップ 4-2: 構成検証（`handler_chain.go`）

- [ ] `SlackFreeHandler` インターフェース、`ErrFailureLoggerContainsSlackHandler`、`ErrFailureLoggerUnverifiableHandler` を定義する（02_architecture.md 3.4.1）。
- [ ] `verifySlackFreeHandlers(handlers []slog.Handler) error` を実装する。受理・拒否の規則は 02_architecture.md 3.4.8 の表に従い、`*MultiHandler` は `Handlers()` で再帰する。認識できない型は `ErrFailureLoggerUnverifiableHandler` で拒否する（fail closed）。
- [ ] `internal/redaction` と共通化しない理由（import 循環と、走査ではなく既知型の許可という構造の違い）を英語コメントで残す。
- [ ] `handler_chain_test.go` に `TestVerifySlackFreeHandlers` を追加する。受理ケース（`*slog.JSONHandler`、`(*slog.JSONHandler).WithAttrs` の戻り値、`*slog.TextHandler`、`slog.DiscardHandler`、`*ConditionalTextHandler`、`*InteractiveHandler`、それらを包む `*MultiHandler`）、`ErrFailureLoggerContainsSlackHandler`（直接／`*MultiHandler` 越し）、`ErrFailureLoggerUnverifiableHandler`（`Handler()` も `Handlers()` も持たない独自ハンドラ）を `errors.Is` で判定する。最後のケースには「走査に頼る設計なら素通りしていた構成である」旨を英語コメントで残す。
- [ ] `SlackFreeHandler` を実装したテストダブルが受理されることを同テストに含める。

### PR-4 作成ポイント: Slack-free failure logger verification

**対象ステップ**: 4-1 / 4-2

**推奨タイトル**: `feat(0163): verify failure-logger handlers are Slack-free`

**レビュー観点**: 受理型の一覧が 02_architecture.md 3.4.8 の表と一致すること / 認識できない型が fail closed で拒否され、その拒否が起動失敗として扱えること / `*MultiHandler` の再帰が `Handlers()` の全要素を辿ること / 規則 R1〜R3 が後続ステップから参照できる形で定義されていること

**実装モデル要件**: standard

**判定理由**: 新規ファイルの追加のみで既存の挙動を変えず、未確立の設計判断・panel-mode トリガ・Conditional checks のいずれにも該当しない。なお `verifySlackFreeHandlers` は本 PR では本番の呼び出し元を持たないが、`handler_chain_test.go` から参照されるため `unused` リンタには掛からない（golangci-lint v2.11.4、`--build-tags test` で実測確認済み）。一方 `make deadcode` は `-test` を付けずに実行されるため、本 PR の時点では未到達関数として報告される（現在の baseline は 8 件、本 PR で 9 件になる）。ステップ 4-5 で `NewSlackHandler` が呼び出すと baseline に戻るため、本 PR に限り 9 件を許容する。なお `make deadcode` は `.github/workflows/ci.yml` のどのジョブにも含まれていない（CI が実行するのは `lint` / `fmt` / `test-ci-cgo1` / `test-ci-cgo0`）ため、この一時的な 9 件で CI が赤くなることはない。判定を後から再現できるよう、PR-4 の着手時に `make deadcode` の出力を取得して baseline の 8 件を PR 本文へ貼る。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] `make deadcode` の報告が baseline 8 件 ＋ `verifySlackFreeHandlers` の 9 件に留まることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### ステップ 4-3: e2e テストの実行経路の整備

1.3.3 のとおり、`e2e && test` タグのテストは現在どのターゲットからも実行されない。ステップ 4-6 で e2e テストを `Flush` 同期点へ移行する前にこの経路を用意し、移行が退行を検出できる状態にしておく。この時点では対象のテストは同期送信のまま変更されていないため、本ステップ単体でもゲートは成立する。

**macOS での既知の失敗（設計文書に未記載）**: 現行の `e2e_slack_webhook_test.go` と `e2e_slack_webhook_separation_test.go` は設定に `/usr/bin/echo` を用いているが、macOS では `echo` は `/bin/echo` にあり `/usr/bin/echo` は存在しない。このため macOS では 0163 以前から 7 件中 3 件（`TestE2E_SlackWebhookWithMockServer`、`TestE2E_SlackWebhookSeparation_SuccessOnly`、`TestE2E_SlackWebhookSeparation_MessageFormat`）が失敗する。残る 4 件は検証失敗そのものを検証しているため通る。CI は `ubuntu-latest` であり `/usr/bin/echo` が存在するため CI では 7 件すべてが成立する。この失敗は本タスクの原因でも対象でもないため修正せず、ターゲットの適用範囲で切り分ける。

**コンパイル可能性を macOS でも保つ**: 実行を読み飛ばすだけにすると、ステップ 4-6 で 7 本の e2e テストを書き換えても macOS ではコンパイルすらされず、型エラーが CI まで露見しない。`make test` も `make lint` も `-tags test` のみで走り `e2e` タグのファイルを対象にしないため、この穴は既定のゲートでは塞がらない。したがって Darwin では**実行のみを読み飛ばし、コンパイル検査は必ず行う**。

- [ ] `Makefile` に `slack-e2e-test` ターゲットを追加する。Darwin 以外では `$(ENVSET) $(GOTEST) -tags e2e,test -race -run '^TestE2E_SlackWebhook' ./internal/runner` を実行する。`-run` で絞るのは、タグ付きビルドが `internal/runner` の全 113 テストを 1 つのバイナリに含み、`unit-test` が既に実行している約 106 件を CI で二重に走らせてしまうためである。
- [ ] Darwin では上記の代わりに `$(ENVSET) go vet -tags e2e,test ./internal/runner` を実行し、コンパイルと vet だけを行って理由（`/usr/bin/echo` を前提とする既存 e2e テストが macOS では実行できないこと）を表示する。既存の `unit-test` が `CGO_ENABLED=0` の実行を Darwin で読み飛ばしているのと同じ `uname -s` による分岐を用いる。
- [ ] `.PHONY` の一覧と、Makefile 冒頭のターゲット説明コメントにも追加する。
- [ ] `test-ci` と `test-ci-cgo1` の依存に `slack-e2e-test` を加える。`-race` を使うため CGO=1 側の系に置く。
- [ ] Linux 環境で `make slack-e2e-test` が 7 件を実行して成功することを確認する。macOS では vet が通り、読み飛ばしのメッセージが出て成功終了することを確認する。

### PR-5 作成ポイント: run Slack e2e tests in CI

**対象ステップ**: 4-3

**推奨タイトル**: `build(0163): run Slack e2e tests in CI`

**レビュー観点**: `-run '^TestE2E_SlackWebhook'` の絞り込みが 7 件すべてを拾い、`unit-test` との二重実行を避けていること / Darwin 分岐が実行のみを読み飛ばし `go vet -tags e2e,test` によるコンパイル検査を残していること / `-race` を使うため CGO=1 側の系（`test-ci-cgo1`）に置かれていること / 既存 7 テストを未変更のまま緑になること

**実装モデル要件**: frontier-recommended

**判定理由**: 本リポジトリで一度も実行されたことのない 7 テストを、実 TLS モックサーバと `-race` 付きで CI の必須経路に組み込む。`mkplan.md` step 8 の panel-mode トリガ「heavy integration-test / CI surface」に触れ、失敗すれば無関係な PR まで CI が赤くなる。加えて Darwin 分岐という条件分岐を持つ。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### ステップ 4-4: 送信機構（`slack_sender.go`）

- [ ] 環境変数名の定数 `SlackSendTimeoutEnvVar = "GSCR_SLACK_SEND_TIMEOUT"`、`SlackFlushTimeoutEnvVar = "GSCR_SLACK_FLUSH_TIMEOUT"`、`SlackSyncEnvVar = "GSCR_SLACK_SYNC"` を定義する。値の解釈は行わない（1.4 参照）。
- [ ] 既定値の定数 `DefaultSendTimeout = 40 * time.Second`、`DefaultFlushTimeout = 15 * time.Second`、`flushPerSendTimeout = 5 * time.Second`、`defaultHighPriorityQueueSize = 32`、`defaultNormalQueueSize = 128` を定義する。前 2 者は `bootstrap` が参照するため公開する（1.4）。
- [ ] `slackSender`、`slackRequest`、`shutdownRequest`、`slackCounters`、`FlushStats` を 02_architecture.md 3.4.1 の定義どおりに実装する。ドキュメンテーションコメントは同節の英語コメントをそのまま用いる。
- [ ] `slackSender` に `mu` で保護した `map[string]int` の `message_type` 別内訳（送信・失敗・破棄）を追加する（1.4 参照。02_architecture.md 3.4.1 の定義への本計画からの追加）。
- [ ] `slackSender` の `runID` フィールドを `NewSlackHandler` が `SlackHandlerOptions.RunID` から設定する（02_architecture.md 3.4.1、3.5）。flush 時の集計レコードは特定の `slackRequest` に紐づかないため、この値から `run_id` を付ける。
- [ ] `slackSender` に非公開フィールド `afterDequeue func()` を追加する。ワーカーが 1 件を取り出した直後・送信開始前に、非 nil のときだけ呼ぶ。本番では常に nil で、同一パッケージのテストのみが代入する。テスト専用の同期点であることを英語コメントで明記する（1.4 参照）。
- [ ] `sendToSlack` を `slack_handler.go` から移設し、`(*slackSender).send(ctx context.Context, req slackRequest, singleAttempt bool) error` にする。`singleAttempt` が真のとき（flush 中）はリトライを行わない。
- [ ] 移設に伴い、`sendToSlack` 内の `slog.Debug` / `slog.Info` / `slog.Warn` / `slog.Error` の **10 箇所すべて**を `failureLogger` 経由の呼び出しへ置き換える。`sanitizeErrorForLog` と既存の 6 件の `//nolint:gosec`（G704 が 1 件、G706 が 5 件）は、行単位の注釈と G コードの理由づけをそのまま維持する。ファイル単位・パッケージ単位の抑止は導入しない。
- [ ] `slackSender` のメソッドのレシーバ名を `s` 以外（例: `sd`）にする。`SlackHandler` からフィールドが消えたことを 6.3 の検索で機械的に確かめられるようにするためである。
- [ ] `message_type` から投入先キューを選ぶ関数を実装する（高優先度＝`security_alert` / `privilege_escalation_failure` / `pre_execution_error`、それ以外は通常キュー。02_architecture.md 3.4.2）。
- [ ] `enqueue`（読み取りロック下で受付停止フラグ確認＋非ブロッキング投入＋カウンタ更新＋破棄時の 1 件記録）を実装する。記録に含めるのは `message_type`・`run_id`・ログレベル・理由（`queue_full` / `sender_closed` / `send_failed`）のみとし、通知本文は含めない。
- [ ] ワーカーループを実装する。待機は「高優先度キューのみを見る非ブロッキング `select`」→「2 本のキューと終了要求チャネルの `select`」の 2 段構成とする（02_architecture.md 3.4.7）。
- [ ] 1 件を取り出した後、**書き込みロックの 1 回の臨界区間**で終了状態の観測・送信用コンテキストの生成・キャンセル関数の格納を行う（02_architecture.md 3.4.6 の表）。送信完了後は同じロックの下でキャンセル関数を nil に戻す。
- [ ] `Flush(ctx)` / `Close()` を実装する。書き込みロック下で受付停止フラグと終了状態を設定し、フラグを立てたのが自分だった場合に限り終了要求を送り、保持しているキャンセル関数を呼ぶ。2 回目以降の呼び出しはワーカー終了通知を待って記録済みの `FlushStats` を返す。
- [ ] `Flush` は戻る前に、`message_type` 別内訳を 1 件の集計レコードとして送信失敗ロガーへ出力する。このレコードには `slackSender.runID` を `run_id` として含める（02_architecture.md 3.4.8）。
- [ ] 集計レコードは**送信機構 1 つにつき 1 度だけ**出力する。`Flush` → `Flush` や `Flush` → `Close` のように終了処理が複数回呼ばれても、2 回目以降は記録済みの `FlushStats` を返すだけで集計レコードを出力しない。`sync.Once` で出力を包み、理由（運用側で内訳を集計するときに二重計上させないため）を英語コメントで残す。
- [ ] 同期モードでは送信キューを確保せずワーカーも起動しない。`Handle` から `send` を直接呼ぶ（02_architecture.md 3.4.11）。

#### ステップ 4-5: `SlackHandler` の改修

- [ ] `SlackHandler` から `webhookURL` / `httpClient` / `backoffConfig` を削除し、`sender *slackSender` を追加する。
- [ ] `SlackHandlerOptions` に `FailureHandlers` / `SendTimeout` / `HighPriorityQueueSize` / `NormalQueueSize` / `Synchronous` を追加する（02_architecture.md 3.4.1 のコメントをそのまま用いる）。
- [ ] `NewSlackHandler` で `verifySlackFreeHandlers` を呼び、エラーならそのまま返す。`FailureHandlers` が空のときは stderr のみのロガーを内部で構築する（`slog.Default()` へフォールバックしない）。
- [ ] `NewSlackHandler` はドライラン時に `slackSender` を生成しない。同期モードでは生成するがキューとワーカーを持たせない（02_architecture.md 3.4.10、3.4.11）。
- [ ] `Handle` を 02_architecture.md 6.2 上段のとおりに書き換える。分岐順は「`slack_notify` 判定」→「送信機構の有無」→「メッセージ構築」→「同期モード判定」→「投入」。
- [ ] `WithAttrs` / `WithGroup` の構造体リテラルから 3 フィールドを外し、`sender: s.sender` を追加する。
- [ ] `Flush(ctx context.Context) FlushStats` と `Close() FlushStats` を公開する。`sender` が nil のときはゼロ値の `FlushStats` を返す。
- [ ] `NewSlackHandler` 冒頭の `slog.Debug`、ドライラン分岐の `slog.Debug`、`extractCommandResultsFromGroup` の 5 箇所の `slog.Debug` は Debug レベルのまま変更しない（02_architecture.md 3.4.8）。

**本番の既定を本 PR では同期に固定する（3.2 の PR-6 / PR-7 の中間状態対策）**: 本 PR には flush 経路（Phase 5）が含まれないため、非同期を本番の既定にしたまま本 PR だけがマージされると、プロセス終了時にキューの残件が失われる。これを構造的に防ぐため、本 PR では本番の唯一の生成経路である `AddSlackHandlers` に `Synchronous: true` を明示的に渡し、本番の挙動を現行（同期送信）のまま据え置く。PR-7 でこのリテラルを `parseSlackEnvSettings(os.Getenv)` の結果へ差し替え、flush 経路と同時に非同期を既定にする。

- [ ] `internal/runner/bootstrap/logger.go` の `AddSlackHandlers` の 2 つの `SlackHandlerOptions` リテラル（`logger.go:226` の成功用、`logger.go:240` のエラー用）に `Synchronous: true` を加える。PR-7 で差し替える暫定値であることを英語コメントで明記する。
- [ ] `logger_test.go` に `TestAddSlackHandlers_UsesSynchronousModeUntilFlushPathExists` を追加し、`newSlackHandlerFunc` を差し替えて捕捉した `SlackHandlerOptions.Synchronous` が両方の呼び出しで真であることを固定する。PR-7 でこのテストは `TestAddSlackHandlers_PropagatesEnvSettings` に置き換えて削除する旨を英語コメントで残す。

この措置は `internal/logging` のテストと `internal/runner` の e2e テストには影響しない。いずれも `logging.NewSlackHandler` を直接呼んでおり、`Synchronous` を指定しないためゼロ値（＝非同期）で動く。したがって本 PR でも非同期経路は 4-6 / 4-7 の全テストによって完全に検証される。

#### ステップ 4-6: 既存テストの移行

規則 R1〜R3（ステップ 4-1）を、以下のすべてに適用する。

- [ ] `TestNewSlackHandlerWithOptions` の削除フィールド参照 6 箇所（386・387・407・408・409・467 行目、1.3.3 の表）を `handler.sender` 越しの参照へ書き換える。
- [ ] `TestNewSlackHandlerWithOptions` に、`FailureHandlers` を与える正常系ケース、`SlackHandler` を含む構成が `ErrFailureLoggerContainsSlackHandler` になるケース、検証できない型を含む構成が `ErrFailureLoggerUnverifiableHandler` になるケースを追加する。判定は `errors.Is` で行う。
- [ ] `TestSlackHandler_Handle_WithMockServer`（`slack_handler_test.go:823`）を `NewSlackHandler` 構築へ移行し、`Handle` の後に `Flush` を呼んでから検証する。サーバ障害ケースの検証は `Handle` の戻り値ではなく `FlushStats.Failed` で行う。`receivedMessage` への書き込みを `sync.Mutex` で保護する。
- [ ] `TestSlackHandler_SendToSlack_Retry`（864・888 行目の 2 サブテスト）を `slackSender` の直接構築へ移行し、`send` を呼ぶ形に書き換える。呼び出し対象の変更に合わせてテスト名を `TestSlackSender_Send_Retry` へ改める。
- [ ] `TestSlackHandler_WithRedactingHandler`（959 行目）を `NewSlackHandler` 構築へ移行し、`redactingHandler.Handle` の後に `Flush` を挟んでから `receivedMessage` を検証する。`receivedMessage` を `sync.Mutex` で保護する。
- [ ] `internal/runner/e2e_slack_webhook_test.go::TestE2E_SlackWebhookWithMockServer` に `Flush` の同期点を挿入し、`receivedPayloads` を `sync.Mutex` で保護する。
- [ ] `internal/runner/e2e_slack_webhook_separation_test.go` の `time.Sleep(500 * time.Millisecond)` **6 箇所**（126・227・294・343・414・491 行目）を、当該テストが生成したハンドラの `Flush` 呼び出しへ置き換え、`successPayloads` / `errorPayloads` を `sync.Mutex` で保護する。

#### ステップ 4-7: 新規テスト（`slack_sender_test.go`）

テスト名は検証対象で決める。`slackSender` の内部挙動を直接見るものは `TestSlackSender_`、`SlackHandler` の公開 API 越しの挙動を見るものは `TestSlackHandler_` を前置する。

- [ ] `TestSlackHandler_HandleDoesNotBlockOnUnresponsiveServer`: 応答しないモックサーバに対し `Handle` の所要時間が短い上限（例: 100 ms）内であることを、複数回のログ呼び出しで確かめる。
- [ ] `TestSlackHandler_UnreachableSlackDoesNotDelayOtherHandlers`: `MultiHandler` に Slack ハンドラとバッファ書き込みハンドラを登録し、Slack 到達不能時もバッファへの書き込みが遅延しないこと。
- [ ] `TestSlackSender_SendTimeout`: `SendTimeout` に短い値を注入し、期限で送信が打ち切られ `FlushStats.Failed` が増えること。
- [ ] `TestSlackHandler_MessageIdenticalToSynchronousMode`: 同一レコードを同期モードと非同期モードの双方で処理し、モックサーバが受け取る `SlackMessage` が一致すること。
- [ ] `TestSlackHandler_DerivedHandlersShareOneSender`: `WithAttrs` / `WithGroup` を 100 回適用し、全派生インスタンスの `sender` が同一ポインタであること（決定的な検証）。加えて `require.Eventually` で `runtime.NumGoroutine()` が上限を超えないことを確かめる。
- [ ] `TestSlackHandler_DryRunCreatesNoSenderAndSendsNothing`: `IsDryRun: true` で `sender` が nil であること、モックサーバが 1 度もリクエストを受けないこと、`Flush` がゼロ値を返すこと。
- [ ] `TestSlackSender_HighPriorityBypassesFullNormalQueue`: `NormalQueueSize: 1` で通常キューを満杯にした状態でも高優先度の通知が受け入れられ、先に送信されること。
- [ ] `TestSlackSender_QueueOverflowDropsAndRecords`: `NormalQueueSize: 1` と `HighPriorityQueueSize: 1` のそれぞれで溢れを再現し、`Dropped` が増え、送信失敗ロガーに `queue_full` の記録が 1 件ずつ残ることを確かめる。本番容量（128 / 32）には依存しない。
- [ ] `TestSlackSender_DropRecordOmitsMessageBody`: 破棄の記録に `message_type` と理由が含まれ、通知本文の文字列が含まれないこと。
- [ ] `TestSlackSender_FailureLogGoesToNonSlackDestination`: 送信失敗の記録が `FailureHandlers` に与えたバッファへ書かれ、モックの Slack サーバへは記録由来のリクエストが届かないこと。
- [ ] `TestSlackSender_FlushLogsMessageTypeBreakdown`: 複数の `message_type` を投入して `Flush` を呼び、送信失敗ロガーの出力に `message_type` 別の内訳が現れること。集計レコードに `SlackHandlerOptions.RunID` と同じ `run_id` が含まれることも確かめる。
- [ ] `TestSlackHandler_FlushDeliversPendingAndReturnsStats`: 期限内に残件が送信され、`Sent` が投入数と一致すること。
- [ ] `TestSlackHandler_FlushDeadlineReportsPending`: 応答しないサーバに対し、`Flush` が flush 期限内に戻り、送り切れなかった件数が `Pending` に計上されること。
- [ ] `TestSlackHandler_FlushIsIdempotent`: `Flush` を複数回、および `Flush` 後に `Close` を呼んでも、同じ `FlushStats` が返り、ブロックも panic も起きないこと。加えて、これら複数回の呼び出しを通じて送信失敗ロガーへ出力される集計レコードが **1 件のまま**であることを確かめる（ステップ 4-4）。
- [ ] `TestSlackHandler_EnqueueAfterFlushIsDropped`: `Flush` 完了後の `Handle` が `Dropped` を増やして nil を返し、クローズ済みチャネルへの送信による panic が起きないこと。
- [ ] `TestSlackHandler_FlushReturnsWhenWorkerIsIdle`: 通知を 1 件も投入していない送信機構と、投入済みを送り終えた送信機構の双方に対し、`Flush` と `Close` が短い制限時間（例: 1 秒）内に戻り、`done` チャネルが閉じていること。制限時間を超えたらテストを失敗させる。
- [ ] `TestSlackHandler_FlushCancelsInFlightSend`: 応答しないサーバへの送信が進行中に `Flush` を呼び、flush 期限内に戻ること、その 1 件が `Pending` であること、`done` チャネルが閉じていること。
- [ ] `TestSlackSender_DequeueRegisterBoundary`: `afterDequeue` フックで「取り出し済み・送信前」のワーカーを待たせ、その間に `Flush` を呼ぶ。flush 期限内に戻ること、`done` チャネルが閉じていること、その 1 件が `Pending` であることを確かめる。`Close`（`abandon`）でも同じ境界を検証し、送信が 1 度も発生しないことを確かめる。
- [ ] `TestSlackHandler_NilSenderHandleReturnsNil`: 構造体リテラルで構築した `SlackHandler` の `Handle` が panic せず nil を返し、`Flush` / `Close` がゼロ値を返すこと。
- [ ] `TestSlackHandler_SynchronousMode`: `Synchronous: true` のとき送信キューとワーカーが存在せず、`Handle` が返った時点でモックサーバがリクエストを受け取り済みであること、`Flush` がゼロ値を返すこと。
- [ ] `TestSlackSender_CounterInvariants`: 並行投入・キュー溢れ・`Flush` を組み合わせた条件で `Submitted == Enqueued + Dropped` と `Enqueued == Sent + Failed + Pending` が成立すること。破棄が `Enqueued` に混入しないことを、溢れを含む条件で明示的に確かめる。
- [ ] `TestSlackHandler_ConcurrentHandleAndFlush`: 複数 goroutine から同時に `Handle` を呼び、その最中に `Flush` を呼ぶ。`go test -race` で競合が報告されないこと。

**完了条件**: `make fmt && make test && make lint` と、Linux 環境での `make slack-e2e-test` が通ること（macOS での扱いはステップ 4-3 を参照）。加えて `go test -race -tags test ./internal/logging/... ./internal/redaction/...` を実装中の高速な確認ループとして用いる（`make test` が同等の検証を含むため、独立したゲートではない）。

### PR-6 作成ポイント: asynchronous Slack sender

**対象ステップ**: 4-4 / 4-5 / 4-6 / 4-7

**推奨タイトル**: `feat(0163): send Slack notifications asynchronously`

**レビュー観点**: 取り出しと登録が 1 つの臨界区間に収まり、`Flush` からも終了要求からも届かない 1 件が生じないこと（02_architecture.md 3.4.6） / ワーカーの 3 つの位置すべてに引き戻す手段があること（3.4.5 の表） / 送信キューを閉じないこととカウンタ 2 式の整合 / `sendToSlack` の `slog.*` 10 箇所がすべて送信失敗ロガーへ移り、`nolint` の行単位注釈が保たれていること / 既存テスト移行後も期待値の意味が変わっていないこと

**実装モデル要件**: frontier-required

**判定理由**: ワーカーの状態機械・受付停止と投入の同期・送信中 1 件のキャンセルという並行処理の中核を含む（isolated high-risk step: concurrency / state machine）。加えて 02_architecture.md 3.4.3〜3.4.11 の全体をまとめて実装し、既存テスト 4 本の移行と新規テスト 22 本を伴うため、panel-mode トリガの「many test updates」にも該当する。

**レビュー順序の目安**: 本 PR は 4 ステップ分の差分が 1 本にまとまるため大きい。02_architecture.md 3.4.1 の型定義 → ワーカーループ（3.4.6 の臨界区間） → `Flush` / `Close`（3.4.7） → `Handle` の投入経路（6.2 上段） → 既存テストの移行 → 新規テスト、の順に読むと設計文書と対応が取れる。

**中間状態は同期モードの固定で塞ぐ**: 本 PR だけがマージされた状態では、`Handle` が投入のみを行う一方で `cmd/runner` にはまだ flush 呼び出しがない（ステップ 5-1 は PR-7）。非同期を本番の既定にしたままこの状態を作ると、プロセス終了時に送信キューの残件が失われる。これは AC-23 / AC-24 が防ごうとしている事象そのものである。そこでステップ 4-5 のとおり、本 PR では `AddSlackHandlers` に `Synchronous: true` を明示的に渡して本番の挙動を現行のまま据え置き、PR-7 で flush 経路と同時に非同期へ倒す。これによりマージ順序が崩れても通知の欠落は起こらず、順序はレビューの都合だけで決められる。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] `AddSlackHandlers` の 2 箇所が `Synchronous: true` を渡しており、本番の挙動が現行と変わらないことを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: `bootstrap` と `cmd/runner` の flush・ライフサイクル経路

**対象ファイル**: `internal/runner/bootstrap/logger.go`、`internal/runner/bootstrap/logger_test.go`、`cmd/runner/main.go`、`cmd/runner/integration_slack_flush_test.go`（新規）

#### ステップ 5-1: 実装

- [ ] `logger.go` にパッケージ変数 `slackHandlers []*logging.SlackHandler` を追加する。
- [ ] 環境変数を解釈する純粋関数 `parseSlackEnvSettings(getenv func(string) string) slackEnvSettings` を追加する。`GSCR_SLACK_SEND_TIMEOUT` と `GSCR_SLACK_FLUSH_TIMEOUT` は `time.ParseDuration` で解釈し、未設定または不正値なら `logging.DefaultSendTimeout` / `logging.DefaultFlushTimeout` を採る。`GSCR_SLACK_SYNC` は `"1"` のときのみ真とする。不正値のときは送信失敗ロガーへ警告を残す。
- [ ] `AddSlackHandlers` で、`FailureHandlers: phase1BaseHandlers` と `parseSlackEnvSettings(os.Getenv)` の結果（`SendTimeout`、`Synchronous`）を `SlackHandlerOptions` へ渡す。ここで PR-6 が置いた暫定の `Synchronous: true`（ステップ 4-5）を差し替える。**この差し替えが非同期を本番の既定にする変更であり、同じ PR に入る `FlushSlackNotifications` の組み込みと不可分である**。片方だけを入れてはならない。
- [ ] `AddSlackHandlers` の冒頭で、既に登録済みのハンドラがあればすべて `Close` してから `slackHandlers` を空にする（再呼び出し規則）。
- [ ] エラー通知用ハンドラの生成に失敗した場合、それまでに生成したハンドラを `Close` してからエラーを返す（部分失敗規則）。
- [ ] 生成に成功したハンドラを `slackHandlers` へ保持する。
- [ ] `FlushSlackNotifications()` を追加する。`slackHandlers` が空なら何もしない。`parseSlackEnvSettings` の flush 期限で期限付きコンテキストを構築し、各ハンドラの `Flush` を並行に呼び、webhook ごとの `FlushStats` を `phase1FailureLogger` と stderr へ報告する。戻り値は持たず、終了コードに影響を与えない。
- [ ] `cmd/runner/main.go` の `main` で、201 行目 `exitCode := mainWithExitCode(runID)` の直後かつ 204 行目 `bootstrap.ReportRedactionFailures()` の直前に `bootstrap.FlushSlackNotifications()` を挿入する。挿入理由（実行中に発行された Slack 宛レコードを先に送り切る）を英語コメントで残す。

#### ステップ 5-2: テスト

規則 R1〜R3（ステップ 4-1）を、モックサーバとハンドラを生成するすべてのテストに適用する。

- [ ] `logger_test.go` の `saveAndRestoreGlobals` に `slackHandlers` の退避・復元を追加し、`t.Cleanup` の中で復元前に登録済みハンドラを `Close` してワーカーを残さないようにする。
- [ ] `TestParseSlackEnvSettings`: 未設定・正常値・不正な duration・`GSCR_SLACK_SYNC` の各値について、`getenv` を注入した表駆動テストで既定値へのフォールバックと伝播を検証する。
- [ ] PR-6 が追加した `TestAddSlackHandlers_UsesSynchronousModeUntilFlushPathExists`（ステップ 4-5）を削除する。次の `TestAddSlackHandlers_PropagatesEnvSettings` が同じ経路をより広く覆うため、残すと `Synchronous` が常に真であることを誤って固定してしまう。
- [ ] `TestAddSlackHandlers_PropagatesEnvSettings`: `newSlackHandlerFunc` を差し替えて `SlackHandlerOptions` を捕捉し、`GSCR_SLACK_SEND_TIMEOUT` / `GSCR_SLACK_SYNC` に与えた値が `SendTimeout` / `Synchronous` として届くこと、および `FailureHandlers` が `phase1BaseHandlers` と一致することを検証する（既存の `environment_test.go` と同じ差し替え手法を再利用する）。
- [ ] `TestAddSlackHandlers_AcceptsInteractivePhase1Handlers`: `SetupLoggerWithConfig(config, forceInteractive=true, false)` を `LogDir` 付きで呼び、続く `AddSlackHandlers` が `ErrFailureLoggerUnverifiableHandler` にならないこと。`go test` の既定環境では `IsInteractive()` が偽になり `*InteractiveHandler` が構成に入らないため、この経路は明示的に強制しないと本番でのみ fail closed に落ちる危険がある。
- [ ] `TestFlushSlackNotifications_FlushesAllHandlers`: 成功用・エラー用の 2 つのモックサーバを立てて `AddSlackHandlers` を呼び、両方に通知を投入したうえで `FlushSlackNotifications` を呼ぶ。両サーバが通知を受け取っていること、webhook ごとの集計が `phase1FailureLogger` の出力先に現れることを検証する。
- [ ] `TestFlushSlackNotifications_NoSlackConfigured`: Slack ハンドラを登録せずに呼んでも panic せず、即座に戻ること。
- [ ] `TestAddSlackHandlers_SlackHandlersComeAfterPhase1Handlers`: `AddSlackHandlers` が構築する `MultiHandler` の `Handlers()` で、Slack ハンドラが `phase1BaseHandlers` の全要素より後ろに並ぶこと（02_architecture.md 2.4 の不変条件）。
- [ ] `TestAddSlackHandlers_ClosesFirstHandlerOnSecondFailure`: `newSlackHandlerFunc` を「1 回目は実物を返し、2 回目はエラーを返す」スタブに差し替える。`AddSlackHandlers` がエラーを返し、1 回目のハンドラが `Close` されていること（規則 R3 のとおり、そのハンドラへの新たな投入が `Dropped` に計上されること）を検証する。
- [ ] `TestAddSlackHandlers_ClosesPreviousHandlersOnReinvocation`: 2 回続けて呼んだとき、1 回目のハンドラが `Close` されていること（同上の観測方法）。
- [ ] `cmd/runner/integration_slack_flush_test.go::TestIntegration_RunnerFlushesSlackOnNormalExit`（02_architecture.md 7.2 の 1 番目の統合テスト、AC-24）: `cmd/runner` の正常終了経路で、実行の最後に発行される通知がモックサーバへ到達することを検証する。`cmd/runner` の本番経路は自己署名証明書を検証するため、既存の `integration_pre_execution_error_test.go` と同じく `go run .` で別プロセスを起動する方式は使えない。代わりに同一プロセス内で `bootstrap.SetupLoggerWithConfig` → `bootstrap.AddSlackHandlers`（規則 R1 のモックサーバ）→ 通知を発行 → `bootstrap.FlushSlackNotifications()` の順に呼び、モックサーバの受信を確認する。
    - `AddSlackHandlers` は `SlackHandlerOptions` を内部で組み立てており（`internal/runner/bootstrap/logger.go` の 226・240 行目）、`HTTPClient` を設定しない。したがって規則 R1 の `httptest.NewTLSServer` をそのまま使うと本番の HTTP クライアントが自己署名証明書を拒否し、本テストは `x509: certificate signed by unknown authority` で失敗する。そこで本テストは `newSlackHandlerFunc` をテスト用スタブへ差し替え（`TestAddSlackHandlers_PropagatesEnvSettings` と同じ手法）、受け取った `SlackHandlerOptions` に `server.Client()`（またはその `Transport`）を `HTTPClient` として注入したうえで `logging.NewSlackHandler` を呼ぶ。差し替えは `t.Cleanup` で元へ戻す。
    - `go run .` 方式を採らない理由と、`newSlackHandlerFunc` を差し替える理由を英語コメントで残す。

**完了条件**: `make fmt && make test && make lint` と、Linux 環境での `make slack-e2e-test` が通ること（`AddSlackHandlers` の変更は e2e テストが実行する経路に含まれるため、Phase 4 と同じゲートを再度課す）。

### PR-7 作成ポイント: process shutdown flush path

**対象ステップ**: 5-1 / 5-2

**推奨タイトル**: `feat(0163): flush pending Slack notifications on process exit`

**レビュー観点**: `AddSlackHandlers` の部分失敗経路と再呼び出し経路の双方でワーカーが所有者を失わないこと（02_architecture.md 3.4.9） / `FlushSlackNotifications` が `ReportRedactionFailures` より前に呼ばれ、終了コードに影響しないこと / `parseSlackEnvSettings` が純粋関数で、不正値が既定値へ落ちること / `record` / `verify` が影響を受けないこと（AC-28）

**実装モデル要件**: frontier-recommended

**判定理由**: `FlushSlackNotifications` が複数ハンドラの `Flush` を共有の期限の下で並行に呼び、さらに部分失敗時と再呼び出し時のワーカーの寿命を管理する。取り違えるとワーカー漏れやデッドロックに直結する、並行処理とライフサイクルの孤立した高リスクステップである。副次的に Conditional checks の「資源取得地点での後始末」にも該当する。

**本 PR が非同期を本番の既定にする**: PR-6 が置いた `Synchronous: true` を `parseSlackEnvSettings` の結果へ差し替える変更と、`FlushSlackNotifications` の組み込みは不可分である（ステップ 5-1）。片方だけを入れると、flush 経路のない非同期送信という 3.2 が排除した状態が生じる。レビューではこの 2 つが同じ PR に揃っていることを最初に確認する。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 6: 文書の更新

**対象ファイル**: `docs/user/security-risk-assessment.ja.md`、`docs/user/security-risk-assessment.md`、`docs/dev/architecture_design/security-architecture.ja.md`、`docs/dev/architecture_design/security-architecture.md`、`docs/translation_glossary.md`

#### ステップ 6-1: 利用者向け文書

- [ ] `docs/user/security-risk-assessment.ja.md` の「値ベース検出」の一覧（241〜249 行目付近）に 4 項目を追加する: GitHub fine-grained PAT（`github_pat_` プレフィックス）、Slack の追加プレフィックストークン（`xapp-`/`xoxe-`/`xoxs-`）、JWT（`eyJ` で始まる 3 セグメントの Base64URL 文字列）、Slack webhook URL（`https://hooks.slack.com/services/` 以降）。
- [ ] 同ファイルの「限界」の段落に、引用符で囲まれていない値に空白が含まれる場合は 2 語目以降が平文で残ること（02_architecture.md 3.2.2）、および `AllowedHost` に `hooks.slack.com` 以外を設定した構成では webhook URL の値形式検出が働かないこと（02_architecture.md 3.3.3）を追記する。
- [ ] 同ファイルに、一般的な英単語を `KeyValuePatterns` へ追加すると散文が置換されうること、および群 B のキーが引用符付きの構造化データのフィールド名として現れる場合は非機密でも置換されること（`"key": "us-east-1"`）を追記する。
- [ ] 同ファイルの「限界」の段落に、**群 B のキー（`key` / `token` / `secret`）は引用符のない YAML では捕捉されない**ことを追記する（02_architecture.md 3.2.6 の残存リスク）。`token: ghp_xxx` のように行頭・インデント直後・空白の直後に現れる形は、厳しい先頭境界を満たさないため置換されない。JSON のように引用符が付く形（`"token": "..."`）、および `_` / `-` / `.` に続く形（`api_key:`、`Public-key:`）は捕捉される。群 A のキー（`password`、`api_key` など）にはこの制限はない。回避策として、当該キーを含むより特定的なキー名（`auth_token` など）を `KeyValuePatterns` に追加できることを併記する。
- [ ] 同ファイルに Slack 通知の配送方式の節を追加する: 非同期に送信されること、プロセス終了時に既定 15 秒の期限で flush されること、到達不能時や強制終了時には通知が失われうること（02_architecture.md 2.4 の表の要約）、通知内容自体はログファイルに残ること、ワーカー 1 本の直列処理による最悪遅延の目安、`GSCR_SLACK_SYNC=1` が通常運用向けではないデバッグ用の退避手段であること。
- [ ] 追加した記述の裏付けを取る: 既定値（flush 期限 15 秒、送信デッドライン 40 秒、キュー容量 32 / 128）を `internal/logging/slack_sender.go` の定数と 1 件ずつ照合し、`GSCR_SLACK_SYNC` の判定条件を `bootstrap.parseSlackEnvSettings` の実装と照合する。
- [ ] `/mktrans` で `docs/user/security-risk-assessment.md` に反映する。

#### ステップ 6-2: 開発者向け文書と用語集

- [ ] `docs/dev/architecture_design/security-architecture.md` の 600 行目と 863 行目の `SlackHandler` 構造体定義を、`webhookURL` / `httpClient` / `backoffConfig` を除き `sender *slackSender` を加えた形へ更新する。周辺の説明文にも非同期配送になった旨を追記する。
- [ ] `docs/dev/architecture_design/security-architecture.ja.md` の 597 行目と 860 行目に同じ更新を行う。
- [ ] `docs/translation_glossary.md` に未登録の用語を追加する: 値形式検出（value-format detection）、キー名ベース redaction（key-name-based redaction）、区切り（separator）、キー名の先頭境界（leading boundary）、送信キュー（send queue）、ワーカー（worker）、受付停止（stop accepting）、破棄（drop）、flush 期限（flush deadline）、終了要求（shutdown request）、drain / abandon。用語の表記は 02_architecture.md 冒頭の用語表と一字一句一致させる。既登録の 4 語（送信失敗ロガー・flush・送信機構・終了要求チャネル）は重複させない。

**完了条件**: `make verify-docs` が通り、8 章の受け入れ基準検証がすべて期待どおりの結果を返すこと。

### PR-8 作成ポイント: user and developer documentation

**対象ステップ**: 6-1 / 6-2

**推奨タイトル**: `docs(0163): document new detection formats and async Slack delivery`

**レビュー観点**: 追記した既定値が `internal/logging/slack_sender.go` の定数と一致すること / 通知が失われうる条件が 02_architecture.md 2.4 の表と食い違わないこと / `security-architecture` の日英 2 ファイル計 4 箇所の構造体定義が同期していること / 用語集の表記が 02_architecture.md の用語表と一字一句一致すること

**実装モデル要件**: standard

**判定理由**: 文書のみの変更であり、実装コードを含まない。未確立の設計判断・panel-mode トリガ・Conditional checks のいずれにも該当しない。

- [ ] `make verify-docs` と、6.3 の X4・8.1 の G1 / G2 がパスしていることを確認した（1.2 の原則 5 のとおり、文書のみの変更はグリーンゲートではなくこれらで判定する。CI も `.md` のみの差分ではテストジョブを読み飛ばす）
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含むフェーズ | 成果物 | 依存 |
|---|---|---|---|
| M1: redaction 基盤 | Phase 1 | 上限付き正規表現キャッシュ | なし |
| M2: キー名ベース拡張 | Phase 2 | 区切り・引用符・群分けの実装とテスト | M1 |
| M3: 値形式検出拡張 | Phase 3 | 4 パターンの追加とテスト | なし（Phase 2 と独立） |
| M4: 非同期送信基盤 | Phase 4 | `slackSender`・構成検証・既存テスト移行・e2e 実行経路 | M1〜M3 と独立 |
| M5: 終了経路 | Phase 5 | `FlushSlackNotifications` と `main` への組み込み | M4 |
| M6: 文書 | Phase 6 | 利用者向け・開発者向け文書と用語集 | M2〜M5 |

順序と依存関係は 02_architecture.md 8.2 に従う。Phase 1 は Phase 2 の前提、Phase 4 は Phase 5 の前提、Phase 6 は Phase 2〜5 の後に行う。Phase 2 と Phase 3 は互いに独立である。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 | 上限付き正規表現キャッシュ（挙動を変えないリファクタリング） | standard |
| PR-2 | 2-1 / 2-2 | キー名ベース redaction の区切り・引用符拡張と群分け | frontier-required |
| PR-3 | 3-1 / 3-2 | 値形式検出パターン 4 種の追加 | standard |
| PR-4 | 4-1 / 4-2 | テスト共通規則の確立と、送信失敗ロガーの構成検証 | standard |
| PR-5 | 4-3 | Slack e2e テストの CI 組み込み（Makefile と CI の変更のみ） | frontier-recommended |
| PR-6 | 4-4 / 4-5 / 4-6 / 4-7 | `slackSender` の導入、`SlackHandler` の非同期化、既存テスト移行、新規テスト。本番の既定は `Synchronous: true` で据え置く | frontier-required |
| PR-7 | 5-1 / 5-2 | `bootstrap` の flush 経路と `cmd/runner` への組み込み。**本番の既定を非同期へ倒す** | frontier-recommended |
| PR-8 | 6-1 / 6-2 | 利用者向け・開発者向け文書と用語集 | standard |

**PR の順序と依存**: PR-1 → PR-2 の順に積む。PR-3 は PR-1 / PR-2 と独立であり、いつ入れてもよい。Slack 側は PR-4 → PR-5 → PR-6 → PR-7 の順に積む。PR-4 と PR-5 は `internal/redaction` 側の 3 本と独立に進められる。PR-8 は PR-2 / PR-3 / PR-6 / PR-7 がすべてマージされた後に行う。

**PR-6 の中間状態は同期モードの固定で塞ぐ**: PR-6 のマージ後 PR-7 のマージ前という中間状態では、`Handle` が投入のみを行う一方で `cmd/runner` に flush の呼び出しがない。非同期を本番の既定にしたままこの状態を作ると、プロセス終了時にキューへ残っていた通知が失われ、同期送信だった従来より悪化する。

これを手続き（「2 本をスタックして続けてマージする」）ではなく構造で防ぐ。PR-6 では本番の唯一の生成経路である `AddSlackHandlers` に `Synchronous: true` を明示的に渡し、本番の挙動を現行のまま据え置く（ステップ 4-5）。PR-7 がこのリテラルを `parseSlackEnvSettings` の結果へ差し替えると同時に `FlushSlackNotifications` を組み込むため、**非同期になる瞬間には必ず flush 経路が存在する**。中間状態でも通知の欠落は起こらないので、PR-6 と PR-7 の間隔はレビューの都合だけで決められる。

この措置は非同期経路の検証を弱めない。`internal/logging` のテストと `internal/runner` の e2e テストはいずれも `logging.NewSlackHandler` を直接呼び、`Synchronous` を指定しない（＝ゼロ値の非同期）ため、PR-6 の時点で非同期経路は全テストによって検証される。同期に固定されるのは本番の `AddSlackHandlers` 経路だけである。

**PR-6 を分割しない理由**: `slackSender` の追加（4-4）は `sendToSlack` の HTTP・リトライ処理を `slack_handler.go` から**移設**するため、`SlackHandler` の改修（4-5）と既存テストの移行（4-6）を伴わないとコンパイルが通らない。移設ではなく複製すれば 4-4 だけを先行させられるが、Slack 送信という秘密情報が通る経路のリトライ処理を 2 つの PR にまたがって二重に持つことになり、CLAUDE.md の DRY 原則にも反するため採らない。新規テスト（4-7）だけを切り出すことも可能だが、並行処理の中核を検証するテストであり、実装と同じ PR で突き合わせられるほうが安全である。差分が大きくなる代償は、PR-6 の「レビュー順序の目安」で補う。

## 4. テスト戦略

### 4.1 単体テスト

02_architecture.md 7.1 の観点表を、2 章の各テスト関数へ対応させる。既存テストが覆っている挙動（`validateWebhookURL`、`generateBackoffIntervals`、`extractCommandResultsFromGroup`、`sanitizeErrorForLog`、`Enabled` のレベル判定、`applyAccumulatedContext`）については新規テストを追加しない。

境界値と異常系として次を必ず含める。

- redaction: 閉じ引用符なし、区切りに改行、群 B の先頭境界が半角スペース・`[`・`{` の各ケース、群 C の 5 形、JWT のドット数 1 / 2 / 3、JWT のヘッダ・ペイロードの 9 文字と 10 文字、`github_pat_` と Slack プレフィックスの最小長の直前と直後、webhook URL の非該当ホストとパス、正規表現のコンパイル失敗。
- 送信機構: キュー容量 1 での溢れ（両キュー）、応答しないサーバ、送信中の `Flush`、取り出し直後の `Flush`、`Flush` 後の投入、`Flush` の複数回呼び出し、通知 0 件での `Flush`。
- 構成検証: `SlackHandler` を直接含む／`MultiHandler` 越しに含む／検証できない型を含む／`InteractiveHandler` を含む本番構成。
- 環境変数: 未設定・正常値・不正な duration、および `SlackHandlerOptions` への伝播。

### 4.2 統合テスト

- `cmd/runner/integration_slack_flush_test.go::TestIntegration_RunnerFlushesSlackOnNormalExit`（Phase 5）で、正常終了経路の flush と通知到達を検証する。
- `internal/runner` の e2e テスト（`make slack-e2e-test`）を `Flush` による同期点へ移行し、非同期化後も同じ検証が成立することを確かめる（1.3.3、ステップ 4-3）。
- Slack を到達不能にした状態での所要時間の確認と、実サービスでの表示確認は手動で行う（9 章の手順 3・4）。

### 4.3 セキュリティテスト

- `make test` は `CGO_ENABLED=1 go test -tags test -race` を含むため、Phase 1〜5 の各完了時に競合検出が走る。実装中は `go test -race -tags test ./internal/logging/... ./internal/redaction/...` を高速な確認ループとして使う。
- カウンタの不変条件（`TestSlackSender_CounterInvariants`）は競合検出器では捕まらない論理的な取りこぼしを検出するため、race 実行とは別に必ず走らせる。
- 置換後の文字列に元の秘密の断片が残らないことを、Phase 2 と Phase 3 の各テストで `assert.NotContains` により確かめる。

### 4.4 後方互換性

- `Config.RedactText`、`ValueDetector.Mask`、`performKeyValuePatternRedaction` のシグネチャは変更しない。
- `SlackHandlerOptions` への追加はすべてゼロ値が既定として機能するフィールドであり、既存の呼び出し側は変更なしで動く。
- `SlackHandler` から削除する 3 フィールドはいずれも非公開であり、パッケージ外への影響はない。

## 5. リスク管理

| リスク | 影響 | 緩和 |
|---|---|---|
| 群分けの実装漏れにより既存の redaction 結果が変わる | AC-08 違反 | Phase 2 の完了条件を「既存テストを 1 行も変更せず合格」とし、期待値の変更が必要になった時点で設計へ差し戻す |
| `slackSender` の並行制御の実装誤りでデッドロックする | `Flush` が戻らずプロセスが終了しない | `TestSlackHandler_FlushReturnsWhenWorkerIsIdle` と `TestSlackSender_DequeueRegisterBoundary` に短い制限時間を設け、デッドロックをテスト失敗として検出する |
| 取り出しと登録を別々の臨界区間に分けてしまう | flush 期限超過とワーカー残留が同時に起こる | `TestSlackSender_DequeueRegisterBoundary` がこの分割を検出する（02_architecture.md 3.4.6） |
| モックサーバの構成を誤り `NewSlackHandler` が失敗する | テスト移行が進まない | 規則 R1（ステップ 4-1）を全テストに適用する |
| テスト間でワーカーが漏れ、後続テストを不安定にする | 断続的な失敗 | 規則 R2（ステップ 4-1）で取得地点に後始末を登録する |
| `runtime.NumGoroutine()` の完全一致検証が CI で不安定になる | AC-21 / AC-32 のテストが偽陽性で落ちる | 規則 R3（ステップ 4-1）で決定的な観測手段（`done` チャネル、`Dropped` の増加）へ置き換える |
| e2e テストがどのターゲットからも実行されず退行を見落とす | 非同期化後に e2e が壊れたまま気付かない | Phase 4 で `slack-e2e-test` を新設し CI へ組み込む（ステップ 4-3）。Phase 4 と Phase 5 の完了条件に含める |
| `e2e` タグのファイルが macOS でコンパイルされず、ステップ 4-6 の書き換えの型エラーが CI まで露見しない | 手戻りが遅れる | ステップ 4-3 で、Darwin では実行を読み飛ばしつつ `go vet -tags e2e,test ./internal/runner` によるコンパイル検査は必ず行う形にする |
| PR-6 のみがマージされた中間状態で、プロセス終了時にキューの残件が失われる | 同期送信だった従来より通知の欠落が増える。AC-23 / AC-24 が防ごうとしている事象そのもの | PR-6 では `AddSlackHandlers` に `Synchronous: true` を渡して本番の既定を同期のまま据え置き、非同期化を flush 経路と同じ PR-7 で不可分に行う（3.2、ステップ 4-5・5-1）。マージ順序に依存しない構造的な対策である |
| 構成検証が本番の対話的構成のみで fail closed に落ちる | 起動できない環境が生じる | `TestAddSlackHandlers_AcceptsInteractivePhase1Handlers` で `forceInteractive=true` の構成を明示的に検証する |
| テスト用フック `afterDequeue` が本番経路で有効になる | 予期しない同期点の混入 | 本番コードでは代入せず nil のままとし、`slack_sender_test.go`（同一パッケージ）からのみ代入する。用途を英語コメントで明記する |
| 文書の記述と実装の既定値が食い違う | 利用者を誤らせる | Phase 6 に、既定値を実装の定数と照合する明示的なタスクを置く。`make verify-docs` で日英の構造一致も検査する |

## 6. 実装チェックリスト

### 6.1 PR 別

作業の進捗は各ステップのチェックボックスで追跡する。ここでは PR 単位のマージ完了だけを記録する。PR の区切りと根拠は 3.2 を参照。

- [ ] PR-1 マージ済み（対象ステップ: 1-1 / 1-2）
- [ ] PR-2 マージ済み（対象ステップ: 2-1 / 2-2）
- [ ] PR-3 マージ済み（対象ステップ: 3-1 / 3-2）
- [ ] PR-4 マージ済み（対象ステップ: 4-1 / 4-2）
- [ ] PR-5 マージ済み（対象ステップ: 4-3）
- [ ] PR-6 マージ済み（対象ステップ: 4-4 / 4-5 / 4-6 / 4-7）
- [ ] PR-7 マージ済み（対象ステップ: 5-1 / 5-2）— 非同期を本番の既定にする PR
- [ ] PR-8 マージ済み（対象ステップ: 6-1 / 6-2）

### 6.2 テストヘルパー

`docs/dev/developer_guide/test_organization.md` の判断に従い、**新規のテストヘルパーファイルは作らない**。理由は次のとおり。

- `slack_sender_test.go` は `package logging` に属するため、`afterDequeue` フックや `done` チャネルなど `slackSender` の非公開要素へ直接アクセスできる。`test_helpers.go` を経由する必要がない。
- `handler_chain_test.go` のテストダブル（`SlackFreeHandler` 実装、`Handler()` も `Handlers()` も持たない独自ハンドラ）は同ファイル内に閉じ、他パッケージから使われない。
- 規則 R1 の `AllowedHost` 導出は 2〜3 行の小さなヘルパーであり、`internal/logging` と `internal/runner/bootstrap` それぞれの `_test.go` 内に置く。パッケージをまたいで共有する必要がないため `testutil/` は不要である。
- `internal/runner` の e2e テストが必要とするのは公開 API（`Flush` / `Close`）のみである。

### 6.3 横断確認（`make lint` と `make test` で検出できないもの）

| # | コマンド | 期待結果 |
|---|---|---|
| X1 | `rg -n "\.sendToSlack\(" --glob '*.go'` | 一致なし（旧メソッド名への参照が残っていない） |
| X2 | `rg -n -e "\.webhookURL\b" -e "\.httpClient\b" -e "\.backoffConfig\b" internal/logging/slack_handler.go internal/logging/slack_handler_test.go` | 一致なし。3 フィールドが `slack_sender.go` とそのテストにのみ存在することを意味する（現状は 16 件が一致する） |
| X3 | `rg -n "^//nolint" internal/logging/slack_sender.go internal/logging/slack_handler.go` | 一致なし（ファイル単位の抑止を導入していない） |
| X4 | `rg -n -A 10 "type SlackHandler struct" docs/dev/architecture_design/security-architecture.md docs/dev/architecture_design/security-architecture.ja.md` | 4 箇所すべてに `sender` が含まれ、`httpClient` / `backoffConfig` / `webhookURL` が含まれないこと |
| X5 | 8.1 のコマンド G2 | 02_architecture.md 冒頭の用語表の全語が用語集に存在すること |

## 7. 成功基準

- 01_requirements.md のすべての Acceptance Criteria について、8 章の検証がすべて期待どおりの結果を返す。
- `make test` と `make lint` が警告なく通過する（`make test` は `-race` を含む）。
- Linux 環境（CI の `ubuntu-latest` を含む）で `make slack-e2e-test` が成功する。macOS では既存 e2e テストが `/usr/bin/echo` を前提とするため読み飛ばされる（ステップ 4-3）。
- `make verify-docs` が成功する。
- `make deadcode` が新たな未使用シンボルを報告しない。
- 6.3 の X1〜X5 がすべて期待結果を返す。
- 9 章の手順 3（Slack 到達不能時の所要時間）と手順 4（実サービスでの表示）が期待どおりである。
- `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` §1 の D2 の項目を削除できる状態になる。

## 8. Acceptance Criteria 検証

各 AC の検証手段を `test`（実行可能で、挙動が誤れば失敗する）／`static`（`rg` またはコンパイル）／`manual`（定められた手順による観察）で分類する。

### 8.1 表に収まらない検証コマンド

パターン中の `|` が Markdown の表セル区切りと衝突するため、以下は表外に置く。先頭の `.` は表の行頭の `|` に一致する。

**コマンド G1**（AC-35。Phase 6 で追加する用語が用語集の用語列に載ったこと）:

```sh
rg -n -e "^. 値形式検出 " -e "^. キー名ベース redaction " -e "^. 区切り " \
      -e "^. キー名の先頭境界 " -e "^. 送信キュー " -e "^. ワーカー " \
      -e "^. 受付停止 " -e "^. 破棄 " -e "^. flush 期限 " \
      -e "^. 終了要求 " -e '^. `drain` / `abandon` ' \
      docs/translation_glossary.md
```

期待結果: 11 行が一致する（1 語につき 1 行）。備考欄だけに現れる語は一致しない。最後の語は 02_architecture.md の用語表と同じくバッククォート付きで登録する。

**コマンド G2**（6.3 の X5。02_architecture.md の用語表との網羅的な突き合わせ）:

02_architecture.md 冒頭の用語表は 14 行である。次のコマンドでその 14 語を抜き出す。行番号ではなく見出しでアンカーするのは、02_architecture.md の冒頭に 1 行加わるだけで壊れる形を避けるためである。

```sh
awk '/^## 用語/{f=1;next} f&&/^## /{exit} f' \
      docs/tasks/0163_redaction_coverage_and_slack_async/02_architecture.md \
  | awk -F'|' 'NF>2 {gsub(/^ +| +$/,"",$2); print $2}' | tail -n +3
```

得られた各語について `rg -n -e "^. <語> " docs/translation_glossary.md` を実行する。期待結果: 14 語すべてが 1 件以上一致する。一致しない語は Phase 6 の追加漏れである。

### 8.2 AC 別の検証

| AC | 種別 | 検証 |
|---|---|---|
| AC-01 | test | `internal/redaction/redactor_test.go::TestRedactText_QuotedValue`、`internal/redaction/redactor_test.go::TestRedactText_AlternativePriority` |
| AC-02 | test | `internal/redaction/redactor_test.go::TestRedactText_JSONForm` |
| AC-03 | test | `internal/redaction/redactor_test.go::TestRedactText_SeparatorVariants` |
| AC-04 | test | `internal/redaction/redactor_test.go::TestRedactText_QuotedValue`、`::TestRedactText_JSONForm`、`::TestRedactText_SeparatorVariants`（いずれも `assert.NotContains` で元の値の断片が残らないことを確かめる） |
| AC-05 | test | `internal/redaction/redactor_test.go::TestRedactText_KeyGroupBehavior`、`internal/redaction/redactor_test.go::TestKeyBoundaryGroup_Classification` |
| AC-06 | test | `internal/redaction/redactor_test.go::TestRedactText_ExistingBehaviorPreserved` |
| AC-07 | test | `internal/redaction/redactor_test.go::TestRedactText_ExistingBehaviorPreserved` |
| AC-08 | test | `go test -tags test ./internal/redaction/... ./internal/runner/base/security/...` が、既存テスト関数を変更しないまま合格すること |
| AC-08 | static | `git diff $BASE...HEAD -- internal/redaction/redactor_test.go internal/redaction/value_detector_test.go internal/runner/base/security/logging_security_test.go` の差分が追加行のみであること（既存行の変更・削除が 0 であること）。`$BASE` は 0163 着手前の `main` の SHA を指す。**素の `git diff` を使ってはならない**: 作業ツリーとインデックスの比較になるため、コミット後は常に空になり検証として機能しない。8 本の PR がスタックしたブランチで進むため、比較対象は必ずこの固定の SHA とする |
| AC-09 | test | `internal/redaction/redactor_test.go::TestRedactText_NoNewOverRedaction`（除外規定に該当しないケース）、`internal/redaction/redactor_test.go::TestRedactText_IntentionalOverRedaction`（除外規定に該当するケース） |
| AC-10 | test | `internal/redaction/redactor_test.go::TestRedactText_LongTextUnchanged` |
| AC-11 | test | `internal/redaction/value_detector_test.go::TestValueDetector_GitHubFineGrainedPAT` |
| AC-12 | test | `internal/redaction/value_detector_test.go::TestValueDetector_SlackPrefixTokens` |
| AC-13 | test | `internal/redaction/value_detector_test.go::TestValueDetector_JWT` |
| AC-14 | test | `internal/redaction/value_detector_test.go::TestValueDetector_FreeTextEmbedding` |
| AC-15 | test | `internal/redaction/value_detector_test.go::TestValueDetector_FalsePositives`、`internal/redaction/value_detector_test.go::TestValueDetector_SlackWebhookURL`（非該当ホスト・非該当パス） |
| AC-16 | test | `internal/redaction/value_detector_test.go::TestValueDetector_PlaceholderWithDollarNoReinjection` |
| AC-17 | test | `internal/logging/slack_sender_test.go::TestSlackHandler_HandleDoesNotBlockOnUnresponsiveServer` |
| AC-18 | test | `internal/logging/slack_sender_test.go::TestSlackHandler_UnreachableSlackDoesNotDelayOtherHandlers` |
| AC-19 | test | `internal/logging/slack_sender_test.go::TestSlackSender_SendTimeout`（期限で打ち切られること）、`internal/runner/bootstrap/logger_test.go::TestParseSlackEnvSettings`（環境変数の解釈）、`internal/runner/bootstrap/logger_test.go::TestAddSlackHandlers_PropagatesEnvSettings`（解釈した値が `SendTimeout` として届くこと） |
| AC-19 | static | `rg -n -e DefaultSendTimeout -e SlackSendTimeoutEnvVar internal/logging/slack_sender.go` → 既定値 `40 * time.Second` と環境変数名 `GSCR_SLACK_SEND_TIMEOUT` の定義がそれぞれ 1 件見つかること（既定値の根拠は 02_architecture.md 3.4.6 に記載済み） |
| AC-20 | test | `internal/logging/slack_sender_test.go::TestSlackHandler_MessageIdenticalToSynchronousMode` |
| AC-21 | test | `internal/logging/slack_sender_test.go::TestSlackHandler_DerivedHandlersShareOneSender`、`internal/logging/slack_sender_test.go::TestSlackHandler_FlushCancelsInFlightSend` |
| AC-22 | test | `internal/logging/slack_sender_test.go::TestSlackHandler_DryRunCreatesNoSenderAndSendsNothing` |
| AC-23 | test | `cmd/runner/integration_slack_flush_test.go::TestIntegration_RunnerFlushesSlackOnNormalExit`、`internal/runner/bootstrap/logger_test.go::TestFlushSlackNotifications_FlushesAllHandlers`、`internal/logging/slack_sender_test.go::TestSlackHandler_FlushDeliversPendingAndReturnsStats` |
| AC-23 | static | `rg -n -e "bootstrap\.FlushSlackNotifications\(\)" -e "bootstrap\.ReportRedactionFailures\(\)" -e "os\.Exit\(exitCode\)" cmd/runner/main.go` → 3 件が一致し、行番号がこの順に昇順であること |
| AC-24 | test | `cmd/runner/integration_slack_flush_test.go::TestIntegration_RunnerFlushesSlackOnNormalExit`、`internal/runner/bootstrap/logger_test.go::TestAddSlackHandlers_SlackHandlersComeAfterPhase1Handlers`、`internal/logging/slack_sender_test.go::TestSlackSender_HighPriorityBypassesFullNormalQueue`、`internal/runner/e2e_slack_webhook_test.go::TestE2E_SlackWebhookWithMockServer`（`make slack-e2e-test`） |
| AC-25 | test | `internal/logging/slack_sender_test.go::TestSlackHandler_FlushDeadlineReportsPending`、`internal/logging/slack_sender_test.go::TestSlackHandler_FlushReturnsWhenWorkerIsIdle`、`internal/runner/bootstrap/logger_test.go::TestAddSlackHandlers_PropagatesEnvSettings`（flush 期限の環境変数が届くこと） |
| AC-25 | static | `rg -n -e DefaultFlushTimeout -e SlackFlushTimeoutEnvVar internal/logging/slack_sender.go` → 既定値 `15 * time.Second` と環境変数名 `GSCR_SLACK_FLUSH_TIMEOUT` の定義がそれぞれ 1 件見つかること |
| AC-26 | test | `internal/runner/bootstrap/logger_test.go::TestFlushSlackNotifications_FlushesAllHandlers`（webhook ごとの集計が送信失敗ロガーの出力先に現れること）、`internal/logging/slack_sender_test.go::TestSlackHandler_FlushDeadlineReportsPending`（`Pending` の件数）、`internal/logging/slack_sender_test.go::TestSlackSender_FlushLogsMessageTypeBreakdown` |
| AC-27 | test | `internal/logging/slack_sender_test.go::TestSlackHandler_EnqueueAfterFlushIsDropped`、`internal/logging/slack_sender_test.go::TestSlackHandler_NilSenderHandleReturnsNil`、`internal/logging/slack_sender_test.go::TestSlackHandler_FlushIsIdempotent` |
| AC-28 | test | `internal/runner/bootstrap/logger_test.go::TestFlushSlackNotifications_NoSlackConfigured` |
| AC-28 | static | `rg -n -e FlushSlackNotifications -e AddSlackHandlers -e SlackHandler cmd/record cmd/verify` → 一致なし。かつ `rg -ln "bootstrap\.FlushSlackNotifications" cmd/` → `cmd/runner/main.go` のみ |
| AC-29 | test | `internal/logging/slack_sender_test.go::TestSlackSender_QueueOverflowDropsAndRecords`、`internal/logging/slack_sender_test.go::TestSlackSender_DropRecordOmitsMessageBody` |
| AC-30 | test | `internal/logging/slack_sender_test.go::TestSlackSender_FailureLogGoesToNonSlackDestination` |
| AC-30 | static | `rg -n -e "slog\.Debug\(" -e "slog\.Info\(" -e "slog\.Warn\(" -e "slog\.Error\(" internal/logging/slack_sender.go` → 一致なし。送信経路の記録がすべて `failureLogger` 経由であることを意味する |
| AC-31 | test | `internal/logging/handler_chain_test.go::TestVerifySlackFreeHandlers`、`internal/logging/slack_handler_test.go::TestNewSlackHandlerWithOptions`、`internal/runner/bootstrap/logger_test.go::TestAddSlackHandlers_AcceptsInteractivePhase1Handlers` |
| AC-32 | test | `internal/logging/slack_sender_test.go::TestSlackHandler_ConcurrentHandleAndFlush`、`internal/logging/slack_sender_test.go::TestSlackSender_CounterInvariants`、`internal/redaction/regex_cache_test.go::TestRegexCache_ConcurrentAccess`。いずれも `make test`（`-race` を含む）で実行する |
| AC-33 | static | `rg -n -e "github_pat_" -e "xapp-" -e "JWT" -e "hooks\.slack\.com/services/" docs/user/security-risk-assessment.ja.md` → 4 つのパターンそれぞれが 1 件以上一致すること（現状は `JWT` の 1 パターンのみ一致する）。かつ `rg -n "引用符で囲まれていない" docs/user/security-risk-assessment.ja.md` → 1 件以上一致すること |
| AC-33 | static | 英語版: `rg -n -e "github_pat_" -e "xapp-" -e "JWT" -e "hooks\.slack\.com/services/" docs/user/security-risk-assessment.md` → 4 つのパターンそれぞれが 1 件以上一致すること |
| AC-33 | manual | 上記の一致行が「値ベース検出」の一覧内（日本語版は 241 行目付近の箇条書き）にあり、限界の記述が「限界」の段落にあることを目視で確認する。`rg` は出現位置までは保証しない |
| AC-34 | static | `rg -n -e "非同期" -e "GSCR_SLACK_SYNC" -e "GSCR_SLACK_FLUSH_TIMEOUT" docs/user/security-risk-assessment.ja.md` → 3 件すべてが一致すること。かつ `rg -n -e "asynchronous" -e "GSCR_SLACK_SYNC" docs/user/security-risk-assessment.md` → 英語版にも対応する記述があること |
| AC-34 | manual | 上記の一致行が Slack 通知の配送方式の節にまとまっていることを目視で確認する。加えて、追記した既定値（flush 期限 15 秒、送信デッドライン 40 秒、キュー容量 32 / 128）を `internal/logging/slack_sender.go` の定数と 1 件ずつ突き合わせ、一致することを確認する |
| AC-33, AC-34 | static | `make verify-docs` → 成功すること（`docs/user` の `*.ja.md` と `*.md` の構造が一致すること） |
| AC-35 | static | 8.1 のコマンド G1 → 11 語すべてが用語列（表の第 1 列）に現れること |

## 9. 次のステップ

1. 本計画のレビューを受け、`Status` を `approved` に更新する。
2. Phase 1 から順に実装し、各フェーズの完了条件を満たしたところで PR を作成する。
3. Phase 5 完了後、Slack webhook を到達不能なアドレスへ向けた設定で `runner` を実行し、実行全体の所要時間の増加が flush 期限（15 秒）に収まることを計測する（02_architecture.md 7.2、Success Criteria）。
4. Phase 6 完了後、`GSCR_SLACK_WEBHOOK_URL_SUCCESS` / `GSCR_SLACK_WEBHOOK_URL_ERROR` と TOML の `slack_allowed_host` を設定した環境で `make slack-notify-test` と `make slack-group-notification-test` を実行し、実サービスでの表示が変わらないことを確認する。両ターゲットは同じ起動時検証を通るため、同一の webhook 設定を両方の実行に適用する。
5. `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` §1 から D2 の項目を削除する。
