# E1: エントリポイント（cmd/runner, cmd/record, cmd/verify, bootstrap, cli）セキュリティ監査所見

- 監査日: 2026-07-19
- 対象:
  - `cmd/runner/` (main.go)
  - `cmd/record/` (main.go)
  - `cmd/verify/` (main.go)
  - `internal/runner/bootstrap/` (config.go, environment.go, logger.go, verification_manager.go)
  - `internal/runner/cli/` (filter.go, output.go, errors.go)
- 方法: ソースコードの静的分析（読み取り専用）。関連する呼び出し先（`internal/logging/safeopen.go`, `internal/logging/pre_execution_error.go`, `internal/verification/manager_production.go`, `internal/safefileio/safe_file.go`, `internal/runner/base/privilege/unix.go`）も参照。

## 所見サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 3 |
| 🟠 Low | 7 |
| 🔵 Info | 6 |

---

## 所見

### 🟡 M-1: `--run-id` が無検証のままログファイル名に埋め込まれ、パストラバーサル / ログ行注入が可能

- **該当箇所**: `cmd/runner/main.go:78` (フラグ定義), `cmd/runner/main.go:88-90` (無検証で採用), `internal/runner/bootstrap/logger.go:138` (`logPath := filepath.Join(config.LogDir, fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, config.RunID))`), `internal/logging/pre_execution_error.go:124` (`RUN_SUMMARY run_id=%s ...`)
- **問題**: `--run-id` は「auto-generates ULID if not provided」とあるが、ユーザー指定値に対して ULID 形式（英数字のみ）の検証が一切行われない。この値は:
  1. `filepath.Join` でログファイル名の一部となるため、`/` や `..` を含む値（例: `--run-id '../../../tmp/evil'`）で `--log-dir` の外にログファイルを作成できる（`filepath.Join` はパスを Clean するだけで封じ込めは行わず、`safefileio.SafeOpenFile` は symlink は防ぐが `..` 由来の脱出は防がない）。
  2. `RUN_SUMMARY run_id=%s` 行および stderr のエラー出力にそのまま埋め込まれるため、空白・改行を含む値で機械可読サマリ行の偽装（`exit_code=0 status=success` を含む run-id 等）やログ行注入が可能。
- **悪用シナリオ**: runner を root（sudo）で起動する運用で、CI/ジョブスケジューラ等が外部由来の識別子をそのまま `--run-id` に渡している場合、root 権限で `--log-dir` 外の任意パスに `O_CREATE|O_TRUNC` でファイルが作成・切り詰められる（内容は JSON ログ）。また RUN_SUMMARY をパースする監視系に偽の成功行を注入できる。
- **推奨対応**: `--run-id` 受領時に ULID 形式（または `^[A-Za-z0-9_-]+$` + 長さ上限）を検証し、不一致は PreExecutionError で拒否する。ファイル名構築側（logger.go:138）でも `filepath.Base(runID) == runID` 相当の防御的チェックを追加する（多層防御）。

### 🟡 M-2: 起動時の特権降格が euid のみで、egid・補助グループが降格されない

- **該当箇所**: `cmd/runner/main.go:99` (`syscall.Seteuid(syscall.Getuid())`)
- **問題**: setuid バイナリとしての起動を想定した即時降格は euid のみを対象としており、`Setegid(Getgid())` や補助グループの確認が無い。バイナリが setgid でインストールされた場合（あるいは setuid+setgid の場合）、実効 GID は特権グループのまま以後の全処理（フラグ処理、ログセットアップ、設定読み込み、TOCTOU チェック、コマンド実行準備）が走る。また、降格より前に `flag.Parse()`（main.go:85）と `logging.HandlePreExecutionError`（main.go:95、stderr/stdout への書き込みと slog 呼び出し）が実行される。flag 処理自体はファイル I/O を伴わないため実害は小さいが、「特権降格は main の可能な限り先頭で」という原則からの逸脱である。
- **悪用シナリオ**: 将来 setgid 構成で配布・インストールされた場合、グループ特権で書けるパス（ログディレクトリ等）への書き込みが特権 GID で行われ、グループ所有ファイルの作成・改変につながる。現行の setuid-root のみの想定では実害は無いが、防御が構成に暗黙依存している。
- **推奨対応**: `Seteuid` と併せて `Setegid(syscall.Getgid())` を実行し（順序: egid → euid）、失敗時は同様に fail-closed で終了する。降格処理を `flag.Parse()` より前（`main()` の先頭）へ移動する。

### 🟡 M-3: cmd/verify の TOCTOU 権限チェックが fail-open（警告のみで続行）で、record と非対称

- **該当箇所**: `cmd/verify/main.go:58-89`（コメント「Violations are logged as warnings only — verify continues even if the check fails.」）、対比: `cmd/record/main.go:96-152`（fail-closed）
- **問題**: record はハッシュ DB を「root of trust」と位置づけ、ディレクトリ権限違反時にハッシュ生成を拒否する（fail-closed）。一方 verify は同じ違反を WARN ログのみで通過させ、検証を続行して `OK` / exit 0 を返し得る。verify は `-hash-dir` で任意のハッシュディレクトリを指定できるため、攻撃者が書き込めるディレクトリ（違反が検出されるケースそのもの）ではハッシュ記録自体を差し替え可能であり、その状態での「検証成功」は完全性の保証にならない。運用者やスクリプトが verify の exit 0 を信頼判断に使うと、改ざんを見逃す。
- **悪用シナリオ**: group-writable な祖先ディレクトリを持つハッシュディレクトリに対し、攻撃者が対象ファイルとハッシュ記録の両方を差し替える。verify は権限違反を WARN で出すのみで `OK` を返し、監視スクリプトは改ざんに気づかない（WARN は stderr のログに紛れる）。
- **推奨対応**: verify も record と同様に fail-closed とするか、少なくとも違反検出時は exit code を非 0（または専用コード）にして「検証結果は信頼できない」ことを機械可読に伝える。fail-open を維持するなら、その理由（可用性要件等）をコード上に明記し、Summary 行にも violation 件数を出力する。

### 🟠 L-1: cmd/record が TOCTOU 権限チェックより前にハッシュディレクトリを mkdirAll で作成する

- **該当箇所**: `cmd/record/main.go:238` (`parseArgs` 内の `d.mkdirAll(dir, hashDirPermissions)`)、チェック本体は `run()` の main.go:96-152
- **問題**: 引数パース段階でハッシュディレクトリを作成してから権限チェックを行うため、チェックが失敗して exit 1 する場合でもディレクトリ作成の副作用が残る。また `os.MkdirAll` は途中経路の symlink を辿るため、作成そのものは safefileio の防御を経由しない。作成後に `EvalSymlinks` で解決した実パスをチェックするので検証自体は正しいが、「検証前に書き込み副作用」という順序は原則に反する。
- **推奨対応**: ディレクトリ作成を TOCTOU チェック通過後（または少なくとも `run()` 内のチェック直前で祖先の権限を確認した後）に移動する。

### 🟠 L-2: record / verify の `filepath.Abs` / `EvalSymlinks` 失敗時のサイレントフォールバック

- **該当箇所**: `cmd/record/main.go:111-130`, `cmd/verify/main.go:68-87`
- **問題**: `filepath.Abs` 失敗時は元の（相対）パス、`EvalSymlinks` 失敗時は未解決の lexical パスにフォールバックし、警告なしで TOCTOU チェックに渡す。対象ファイルが未作成の場合など `EvalSymlinks` は容易に失敗するため、親ディレクトリに symlink を含むパスでは実体とは別のディレクトリ群が検査され、チェックの実効性が下がる（symlink 先の緩い権限を見逃す方向にも、逆に誤検出する方向にも作用し得る）。
- **推奨対応**: フォールバック発生時に DEBUG/WARN ログを出す。可能なら「存在する最深の祖先まで EvalSymlinks で解決する」ヘルパーに置き換え、record/verify 間の重複コード（ほぼ同一の 20 行）を共通化する（DRY）。

### 🟠 L-3: cmd/verify が存在しないハッシュディレクトリを作成する副作用 + パーミッション定数の不一致

- **該当箇所**: `cmd/verify/main.go:18` (`hashDirPermissions = 0o750`), `cmd/verify/main.go:130` (`mkdirAll`), 対比: `cmd/record/main.go:27` (`0o700`)
- **問題**: 検証専用ツールである verify が、`-d` に typo したパス等を渡された場合に空のハッシュディレクトリを 0o750 で新規作成する。検証は全件 FAILED になるため fail-closed ではあるが、読み取り専用であるべきツールが書き込み副作用を持つのは意図に反する。また record は 0o700、verify は 0o750 と同じディレクトリに対する期待パーミッションが食い違っている（group への読み取りを許すか否かのポリシーが不明確）。
- **推奨対応**: verify では mkdirAll をやめ、ディレクトリ不存在をエラーとして報告する。パーミッションポリシー（0o700 か 0o750 か）を一本化し、`cmdcommon` に定数として集約する。

### 🟠 L-4: runTOCTOUCheck が変数参照・相対パスをサイレントにスキップし、スキップ実績を記録しない

- **該当箇所**: `cmd/runner/main.go:310-315` (`resolveStaticAbsPath`), main.go:322-342
- **問題**: `%{` を含むパスと相対パスは起動時 TOCTOU チェックから除外される。コメントには per-group チェックで補われるとあるが、スキップが発生した事実・件数はログに残らないため、「起動時チェックが通った」ことがどの範囲のパスを保証しているのか運用側から観測できない。将来 per-group 側のチェックが変更された場合に盲点が生じても検知しにくい。
- **推奨対応**: スキップしたパス数（可能ならパス自体）を DEBUG レベルでログ出力し、起動時チェックのカバレッジを可観測にする。

### 🟠 L-5: ログファイル名のタイムスタンプがローカル時刻なのに "Z" (UTC) サフィックスを付けている

- **該当箇所**: `internal/runner/bootstrap/logger.go:77` (`time.Now().Format("20060102T150405Z")`)
- **問題**: フォーマット文字列の `Z` はリテラル文字として出力され、実際の時刻は `time.Now()` のローカル時刻である。ISO 8601 の `Z` は UTC を意味するため、ログファイル名のタイムスタンプが UTC であるかのように偽装される。インシデント調査でタイムライン照合を誤らせる（監査証跡の正確性の問題）。
- **推奨対応**: `time.Now().UTC().Format("20060102T150405Z")` にするか、`Z` を外してローカル時刻であることを明示する。

### 🟠 L-6: Phase 1 と Phase 2 の間に発生するエラー（設定改ざん検出を含む）が Slack 通知されない

- **該当箇所**: `cmd/runner/main.go:190-243`（`SetupLogging` → `LoadAndPrepareConfig` → `SetupSlackLogging` の順序）, `internal/runner/bootstrap/environment.go:105-137`
- **問題**: Slack の AllowedHost が TOML 由来であるため Slack ハンドラの追加は設定ロード後になる。この設計上、設定ファイルのハッシュ検証失敗（= 改ざんの可能性が最も高いイベント）や TOML パースエラーは Slack に通知されず、コンソール/ファイルログにのみ残る。攻撃者が設定を改ざんした場合、最も通知したいはずの検出イベントが通知チャネルに乗らない。
- **推奨対応**: 設計上のトレードオフとして文書化されているか確認の上、改善するなら「AllowedHost 検証前の暫定ハンドラで、URL ホストが既知の Slack ドメインの場合のみ pre-config エラーを通知する」「エラー種別と run_id のみの最小ペイロードで通知する」等のフォールバックを検討する。

### 🟠 L-7: 3 つのエントリポイントで DirectoryPermChecker 初期化失敗時に panic する

- **該当箇所**: `cmd/runner/main.go:347`, `cmd/record/main.go:108`, `cmd/verify/main.go:66`
- **問題**: `NewDirectoryPermChecker` 失敗時に、整備されたエラーハンドリング経路（PreExecutionError → HandlePreExecutionError → RUN_SUMMARY 出力）を通らず panic する。fail-closed ではあるものの、スタックトレースがそのまま stderr に出力され、RUN_SUMMARY を期待する監視系には異常終了の構造化情報が渡らない。同一コメント・同一ロジックが 3 箇所に重複している点も code smell。
- **推奨対応**: panic ではなくエラーリターンで既存経路に乗せる（runner は PreExecutionError、record/verify は stderr + exit 1）。共通ヘルパーに集約する。

### 🔵 I-1: 起動時降格後も saved-uid は root のまま（設計意図の明示が望ましい）

- **該当箇所**: `cmd/runner/main.go:99`
- **説明**: `Seteuid(Getuid())` は saved set-user-ID を変更しないため、以後 privilege manager (`internal/runner/base/privilege/unix.go`) が `Seteuid(0)` で再昇格できる。これは意図された設計（オンデマンド特権実行）だが、main.go 側のコメントには「一時降格であり再昇格可能」という意図が書かれていない。恒久降格（`Setuid`）と誤読されないよう、コメントで privilege manager との関係を明示するとよい。

### 🔵 I-2: dry-run の formatter switch に default ケースがない

- **該当箇所**: `cmd/runner/main.go:472-478`
- **説明**: `outputFormat` は事前に `ParseDryRunOutputFormat` で検証済みのため実害はないが、enum に値が追加された場合 `formatter` が nil のまま `FormatResult` 呼び出しで panic する。default で明示的にエラーを返すのが防御的。

### 🔵 I-3: Slack env 検証エラーが stderr に二重出力される

- **該当箇所**: `cmd/runner/main.go:181` (`fmt.Fprintln(os.Stderr, err.Error())`) と、返却された PreExecutionError を処理する `HandlePreExecutionError`（同内容を再度 stderr へ出力）
- **説明**: 同じエラーメッセージ（複数行の使用方法案内を含む）が 2 回表示される。機能上の問題はないが、Fprintln を削除して単一経路に統一するのが望ましい。

### 🔵 I-4: normalizeSlackAllowedHost の IPv6 分岐で小文字化されない・長さ制限未実施

- **該当箇所**: `internal/runner/bootstrap/config.go:44` (IPv6: `u.Hostname()` をそのまま返す) vs `config.go:54` (ホスト名: `strings.ToLower`)
- **説明**: IPv6 リテラルの 16 進部分は大文字のまま返り、ホスト名分岐との正規化ポリシーが不一致。比較側の実装が case-sensitive だと `[::FFFF:1.2.3.4]` 形式で許可ホスト照合が意図せず不一致/一致し得る。また RFC 1123 のラベル 63 文字・全体 253 文字制限が未実施である点はコメントで明示されている（許容範囲だが記録しておく）。IPv6 分岐にも `strings.ToLower` を適用して統一するのが望ましい。

### 🔵 I-5: cmd/verify のパッケージレベル可変変数によるテスト注入と record との実装様式乖離

- **該当箇所**: `cmd/verify/main.go:23-27` (`validatorFactory`, `mkdirAll` がパッケージレベル var), 対比: `cmd/record/main.go:38-59` (`deps` 構造体による注入)
- **説明**: record は依存を `deps` 構造体で明示的に注入するのに対し、verify はグローバル可変変数の差し替えで注入しており、同種ツール間で設計が乖離している。また `cmd/record/main.go:156,165` で `cacheDir` と `machoCacheDir` が同一値を重複計算している。機能上の問題はないが、verify を record と同じ deps パターンに揃えるのが望ましい（DRY / テスト分離性）。

### 🔵 I-6: bootstrap/logger のグローバル可変状態（シングルスレッド前提は文書化済み）

- **該当箇所**: `internal/runner/bootstrap/logger.go:43-57` (`redactionErrorCollector`, `redactionReporter`, `phase1BaseHandlers`, `phase1FailureLogger`, `newSlackHandlerFunc`)
- **説明**: Phase 1 → Phase 2 の状態受け渡しがパッケージグローバル変数で行われる。`SetupLoggerWithConfig` のコメントで「起動時に一度だけ・シングルスレッドで呼ぶ」制約が明記されており、`AddSlackHandlers` は Phase 1 未初期化を `errPhase1NotInitialized` で検出する防御もある。現状の使い方では問題ないが、状態を構造体（例: `LoggerBootstrap`）に閉じ込めれば制約自体を型で表現できる。

---

## 観察された良好な防御層

1. **setuid 起動直後の即時 euid 降格と fail-closed**: `cmd/runner/main.go:99-102` で `Seteuid(Getuid())` 失敗時は即 exit 1。降格失敗を握りつぶさない。
2. **設定ファイルの atomic verify-and-read**: `bootstrap.LoadAndPrepareConfig` は `VerifyAndReadConfigFile` で「一度読んだ内容をハッシュ検証し、その内容からパースする」ため、検証と読み込みの間の TOCTOU 差し替えが成立しない（`internal/runner/bootstrap/config.go:87-97`）。include されるファイルも verified loader 経由。
3. **本番ハッシュディレクトリの固定**: runner には `-hash-dir` フラグが存在せず、`verification.NewManagerForProduction` はビルド時定数 `cmdcommon.DefaultHashDirectory`（絶対パスであることを main.go:94 で起動時検証）のみを使用。攻撃者が CLI 経由で信頼ルートを差し替えられない。
4. **record の fail-closed TOCTOU チェック**: ハッシュ DB の祖先ディレクトリ権限違反時はバイパスフラグ無しで記録を拒否し、修復手順（chmod）まで提示する（`cmd/record/main.go:96-152`）。
5. **Slack webhook URL の redaction 配慮**: `SetupSlackLogging` は URL 検証エラーを `Message` に整形せず `Err` に格納し、URL が stderr/slog に出力されないようにしている（`internal/runner/bootstrap/environment.go:124-133`）。webhook URL は env 変数からのみ取得し、フラグでは受け取らない。
6. **Slack 設定の fail-safe 検証**: SUCCESS のみ設定・ERROR 未設定をエラーとし、「エラー通知の無効化による silent failure」を防ぐ（`ValidateSlackWebhookEnv`）。
7. **redaction 失敗の収集と終了時報告**: RedactingHandler + InMemoryErrorCollector（上限 1000 件で無制限成長を防止）+ `ReportRedactionFailures()` を `os.Exit` 前に必ず呼ぶ構造（`cmd/runner/main.go:108`）。
8. **Phase 1 状態のコミット順序**: `SetupLoggerWithConfig` は全初期化成功後にのみグローバル状態を設定し、`AddSlackHandlers` が部分初期化状態に対して動作しないようガードしている（logger.go:187-193, 210-212）。
9. **テストヘルパの本番ビルド除外**: `cmd/runner/integration_test_helpers.go` は `//go:build test` タグ付きで、AllowAllEvaluator 等の許可的テスト部品が本番バイナリに混入しない。
10. **run_id の既定生成が暗号学的乱数ベースの ULID**: `logging.GenerateRunID` は `crypto/rand` を entropy 源に使用（`internal/logging/safeopen.go:57-60`）。
11. **ログファイル作成の symlink 防御**: ログファイルは `safefileio.SafeOpenFile`（Linux では openat2）経由で 0o600 作成（`internal/runner/bootstrap/logger.go:139-141`）。
12. **dry-run 時の出力チャネル分離**: dry-run ではログを stderr に退避し、stdout を機械可読な dry-run 出力専用に保つ（`cmd/runner/main.go:171-176`）。deny プレビューは専用 exit code で CI から判別可能（main.go:513-517）。
13. **エラー型の構造化ハンドリング**: `errors.As` による型ベースの分岐（SilentExitError / PreExecutionError / ExecutionError / dryRunPreviewExit）で、文字列マッチに依存しないエラー処理（`cmd/runner/main.go:115-142`）。
