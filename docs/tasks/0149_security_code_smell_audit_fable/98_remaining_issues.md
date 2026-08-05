# セキュリティクリティカル部 code smell 監査 — 残件一覧

- 集約元: [99_summary.md](99_summary.md)（Task 0149 監査の集約サマリ、Task 0150〜0161 の対応状況を追記済み）
- 目的: Task 0150〜0161 で対応しなかった所見のみを一覧化し、今後の改善サイクル・タスク化の入力とする。
- 詳細所見の原本: `findings/*.md`（各コンポーネント個別ファイル）

このドキュメントに載っている項目は、2026-07 時点でタスク化・修正のいずれも行われていない（各対応タスクの要件定義書「対象外」節で明示的に見送られたもの、またはそもそもどのタスクの対象にもならなかったもの）。ここでいう「タスク化」とは Task 0150〜0161 のような `01_requirements.md`/`02_architecture.md`/`03_implementation_plan.md` を伴う実装タスク化を指し、後述する B3 M2 のように追跡目的で GitHub Issue を作成した（例: [#972](https://github.com/isseis/go-safe-cmd-runner/issues/972)）だけの項目は「タスク化」には含めない——それは所見が認知されたことを示すのみで、対応や解消を意味しない。番号・記号は `findings/*.md` の所見 ID に対応する。

E1（エントリポイント: `cmd/runner`・`cmd/record`・`cmd/verify`・`bootstrap`・`cli`）は Task 0150〜0161 のいずれからも直接の対象にされておらず、16件（🟡Medium 3・🟠Low 7・🔵Info 6）全件が未着手のまま残っている。2026-08-05 時点で現行コードと照合し、後続タスク（とくに特権管理・識別情報解決を変更した 0157/0158/0160/0161）による副次的解消がないかを個別に確認したが、**16件全件が解消されておらず現存する**。以下では重大度ごとに他コンポーネントの残件と合わせて記載する。詳細な確認手順・根拠コード引用は [99_summary.md](99_summary.md) §7.1 を参照。

---

## 1. 🔴High / 🟡Medium の未対応残件

> **D1 M-3 について**: 0160（基準UID決定方針の明示化）・0161（`SUDO_UID` の実在確認・記録）で解消済み。0161 は「呼び出し元の実 UID との突き合わせ」を対象外としたが、これは信頼できる突き合わせ手段が存在しないという技術的な結論であり（0161 要件定義書「突き合わせを対象外とする根拠」参照）、[#921](https://github.com/isseis/go-safe-cmd-runner/issues/921)（`runner` の native root 実行サポート）を実施しないと決定したことで、この残余リスクは今後再検討すべき残件ではなく恒久的に受容する既決事項として確定した。したがって D1 M-3 は本ドキュメントの一覧から除外する（D1 の他所見 L-2/L-3 は §2 に残る）。

### B3（verification）M2: `isDeferredHashDirUnavailable` のゲート漏れ

0153（P1 fail-open 横断修正）・0155（P3 TOCTOU 修正）のいずれの対象にも含まれず未着手。dry-run 限定でない箇所でのゲート漏れが残る。→ [#972](https://github.com/isseis/go-safe-cmd-runner/issues/972) を作成済み。

### E1（entrypoints）M-1: `--run-id` が未検証のままログファイル名・ログ行に埋め込まれる

未対応。`cmd/runner/main.go` の `run-id` フラグはユーザー指定値をそのまま `logger.go:138` の `filepath.Join` に渡しており、ULID 形式検証・`filepath.Base` チェックのいずれも追加されていない。パストラバーサル（`--log-dir` 外へのファイル作成）およびログ行注入（RUN_SUMMARY の偽装）が引き続き可能。→ [#974](https://github.com/isseis/go-safe-cmd-runner/issues/974)（E1 M-1/M-2/M-3 まとめタスク）を作成済み。

### E1（entrypoints）M-2: 起動時特権降格が euid のみ（egid・補助グループ未降格）

未対応。`cmd/runner/main.go:109` は依然 `syscall.Seteuid(syscall.Getuid())` のみで `Setegid` 呼び出しはなく、`flag.Parse()`（:95）より後で実行される順序も変わっていない。**0157/0160 で整理された「特権実行を exec 時の `syscall.Credential` 指定に一本化する」仕組みは、実行するコマンドへの per-command 特権付与を扱うものであり、`runner` バイナリ自身が setuid-root として起動した直後の自己降格（本所見の対象）とは別レイヤーである。したがって本所見は 0157/0160/0161 のいずれによっても解消されていない。** → [#974](https://github.com/isseis/go-safe-cmd-runner/issues/974)（E1 M-1/M-2/M-3 まとめタスク）を作成済み。

### E1（entrypoints）M-3: verify の TOCTOU チェックが fail-open（警告のみで続行）

未対応。`cmd/verify/main.go` の TOCTOU チェックのコメント「Violations are logged as warnings only — verify continues even if the check fails.」は現行のまま存置されており、戻り値で `run` を打ち切る分岐はない。record（fail-closed）との非対称も残る。→ [#974](https://github.com/isseis/go-safe-cmd-runner/issues/974)（E1 M-1/M-2/M-3 まとめタスク）を作成済み。

### D2（logging/redaction）

- **M-2**: key=value redaction の空白・引用符・JSON 形式カバレッジ不足。
- **M-4**: `ValueDetector` パターン網羅性（GitHub fine-grained PAT、JWT 等）。
- **M-5**: Slack 送信の同期ブロッキング。
- 0154（P2 redaction 境界統一）はいずれも「根本原因（`slog.Any` 未再帰）とは独立した所見」として対象外とした。詳細は `findings/D2_logging_redaction.md` を参照。
- → [#975](https://github.com/isseis/go-safe-cmd-runner/issues/975)（D2 M-2/M-4/M-5 まとめタスク）を作成済み。

---

## 2. 🟠Low の未対応残件

### D1（groupmembership）

- **L-2**: 非 CGO 版実装がローカルファイル（`/etc/group`・`/etc/passwd`）のみを参照するため、LDAP/SSSD 等の NSS ディレクトリ管理メンバーが列挙結果に現れない。0151 は「列挙 API がエラーを返した場合」の fail-closed 化のみを対象とし、本件（NSS を経由しないため件数が過少に見える）は明示的に対象外としている。
- **L-3**: L-2 と同系統の残存リスクとして 0151/0153 で言及されているが、個別タスクの対象にはなっていない。
- → [#976](https://github.com/isseis/go-safe-cmd-runner/issues/976) を作成済み。

### A1（privilege）

- **L-2**: 昇格・復元処理でテスト注入フィールド（`syscallSeteuid`/`syscallSetegid` 相当）を使わない構造。0157 はむしろ当該フィールドを削除する方向で対応しており、L-2 の推奨（注入フィールドを実経路でも使う）とは方向が逆。0157 に着手する時点で「必要な注入点を改めて設計する方が健全」と判断し見送られた。
- **L-3**: metrics の恒偽項（常に真になる記録項目）。いずれのタスクの対象にもなっていない。
- **L-4**: 再入デッドロックの可能性。いずれのタスクの対象にもなっていない。
- → [#977](https://github.com/isseis/go-safe-cmd-runner/issues/977) を作成済み。

### B1（safefileio）

- **F-2〜F-9**（F-1 を除く）: 0155（P3 TOCTOU）は F-1（`AtomicMoveFile` の fd アンカー化）のみを対象とし、以下は対象外のまま。
  - F-2: 非 Linux フォールバック経路の symlink TOCTOU
  - F-3: fd リーク
  - F-4: ロールバック欠如
  - F-5: `Remove` の安全性契約
  - F-6〜F-9: 詳細は `findings/B1_safefileio.md` を参照
- → [#978](https://github.com/isseis/go-safe-cmd-runner/issues/978) を作成済み。

### B2（filevalidator）

- **B2-2, B2-4〜B2-13**: 0155 は B2-1・B2-3（record 時のハッシュ計算と解析の一貫性）のみを対象とし、以下は未着手。
  - B2-2: 解析器無効時の温存ロジック
  - B2-4, B2-5: エラーハンドリング
  - B2-6: Mach-O 解析の縮退
  - B2-7〜B2-13: 詳細は `findings/B2_filevalidator.md` を参照
- → [#979](https://github.com/isseis/go-safe-cmd-runner/issues/979) を作成済み。

### B3（verification）

- **L2**: 詳細は `findings/B3_verification.md` を参照。いずれのタスクの対象にもなっていない。
- → [#980](https://github.com/isseis/go-safe-cmd-runner/issues/980) を作成済み。

### C1（binary analysis）

- **F-2〜F-8**: 0153 は F-1（syscall analysis store の想定外エラー）のみを対象とし、残りの頑健性・保守性所見（F-2〜F-8）は「fail-open の実害が限定的、または既に fail-closed 方向」として対象外。詳細は `findings/C1_binary_analysis.md` を参照。
- → [#981](https://github.com/isseis/go-safe-cmd-runner/issues/981) を作成済み。

### C2（dynlib）

- **F-2**: ld.so.cache の `Flags`/`HWCap` 無視。
- **F-4**: `ResolveRealPath` のエラー種別非区別。
- **F-6〜F-11**: libccache キャッシュ検証強化など。0153・0155 いずれの対象にも含まれない。詳細は `findings/C2_dynlib.md` を参照。
- → [#982](https://github.com/isseis/go-safe-cmd-runner/issues/982) を作成済み。

### C3（shebang/fileanalysis）

- **F1, F2, F4 以降**: 0157 は F3（`SaveSyscallAnalysis` の不整合レコード生成・未使用ストアの削除）のみを対象とした。残りの shebang/fileanalysis 所見は未着手。詳細は `findings/C3_shebang_fileanalysis.md` を参照。
- → [#983](https://github.com/isseis/go-safe-cmd-runner/issues/983) を作成済み。

### A3（environment）

- **F-6**: `ParseSystemEnvironment` が不正形式エントリを無音スキップする（挙動変更を伴うため 0157 は対象外とした）。
- **F-7**: allowlist 判定ロジックの分散（設計変更を伴うため 0157 は対象外とした）。
- → [#984](https://github.com/isseis/go-safe-cmd-runner/issues/984) を作成済み。

### A7（audit）

- **L-1**: severity 判定の二重実装・fail-open。
- **L-2**: `chain[].path` の redaction 非適用。0154 の根本原因修正（`slog.Any` の map 要素への再帰的 redaction）により副次的に緩和され得るが、明示的な対応は行っていない。
- → [#985](https://github.com/isseis/go-safe-cmd-runner/issues/985) を作成済み（Info I-1〜I-5 含む）。

### E1（entrypoints）

- **L-1**: record が TOCTOU チェック前にハッシュディレクトリを `mkdirAll` する。未対応。`cmd/record/main.go` の `parseArgs`（:274）が `mkdirAll` を実行し、`run`（:159）がその後で `checkDirPermissions`（:185）を呼ぶ順序は変わっていない。
- **L-2**: `filepath.Abs`/`EvalSymlinks` 失敗時の無警告フォールバック。未対応。`cmd/record/main.go`・`cmd/verify/main.go` とも失敗時に元のパスへ黙ってフォールバックする同一パターンのコードが残る（ログ出力なし、共通化もされていない）。
- **L-3**: verify のハッシュディレクトリ作成副作用 + パーミッション不一致。未対応。`cmd/verify/main.go` は `hashDirPermissions = 0o750` のまま無条件に `mkdirAll` を実行し、`cmd/record/main.go` の `0o700` との不一致も残る。
- **L-4**: `runTOCTOUCheck` の変数参照・相対パスのサイレントスキップ。未対応。`resolveStaticAbsPath`（`cmd/runner/main.go:320`）は `%{` を含むパス・相対パスを無音でスキップする実装のままで、スキップ件数のログ出力は追加されていない。
- **L-5**: ログファイル名のタイムスタンプが実際はローカル時刻なのに `Z`（UTC）表記。未対応。`internal/runner/bootstrap/logger.go:77` は `time.Now().Format("20060102T150405Z")` のままで `.UTC()` 変換はない。
- **L-6**: Phase 1/Phase 2 間のエラー（設定改ざん検出含む）が Slack 未通知。未対応。`cmd/runner/main.go` の `SetupLogging` → `LoadAndPrepareConfig` → `SetupSlackLogging` の順序は変わらず、設定ロード失敗時点では Slack ハンドラが未登録のまま。
- **L-7**: `DirectoryPermChecker` 初期化失敗時に3箇所とも panic。未対応。`cmd/runner/main.go`・`cmd/record/main.go`・`cmd/verify/main.go` のいずれも `NewDirectoryPermChecker` 失敗時に同一の `panic(fmt.Sprintf(...))` を実行する実装のまま。
- → [#986](https://github.com/isseis/go-safe-cmd-runner/issues/986) を作成済み（Info I-1〜I-6 含む）。

---

## 3. 🔵Info の未対応残件

- **A5**: Low-4, Info-1（0153 の対象外）。
- **A7**: I-1〜I-5（0154 の対象外）。
- **D2**: Low-1〜L-6, Info-1〜Info-6（0154 の対象外）。
- **E1**:
  - **I-1**: 起動時降格後も saved-uid が root のままである設計意図が未記載。未対応。`cmd/runner/main.go:109` 付近に privilege manager との関係を説明するコメントは追加されていない。
  - **I-2**: dry-run formatter の switch に `default` 節がない。未対応。`cmd/runner/main.go:478` の `switch outputFormat` は `OutputFormatText`/`OutputFormatJSON` の2ケースのみで `default` 節はない。
  - **I-3**: Slack env 検証エラーの stderr 二重出力。未対応。`cmd/runner/main.go:191` の `fmt.Fprintln(os.Stderr, err.Error())` は残っており、`HandlePreExecutionError` 経由の出力と重複する構造は変わっていない。
  - **I-4**: `normalizeSlackAllowedHost` の IPv6 分岐で大文字小文字未統一。未対応。`internal/runner/bootstrap/config.go` の IPv6 分岐は引き続き `u.Hostname()` を無変換で返し、ホスト名分岐のみ `strings.ToLower`（:54）を適用する非対称が残る。
  - **I-5**: verify のパッケージレベル変数注入と record の `deps` 構造体注入の様式乖離。未対応。`cmd/verify/main.go` は依然 `validatorFactory`/`mkdirAll` をパッケージレベル変数で注入し、`cmd/record/main.go` の `deps` 構造体方式と乖離したまま。付随する `cacheDir`/`machoCacheDir` の重複計算（`cmd/record/main.go`）も解消されていない。
  - **I-6**: bootstrap/logger のグローバル可変状態。未対応。`redactionErrorCollector`/`phase1BaseHandlers` 等のパッケージグローバル変数によるフェーズ間受け渡しは変わらず、`LoggerBootstrap` 相当の構造体化は行われていない。
  - → [#986](https://github.com/isseis/go-safe-cmd-runner/issues/986) を作成済み。
- 上記以外の各コンポーネントの Info 所見全般（A5, A7, D2 の Info 群など）は個別 issue 化していない。詳細は各 `findings/*.md` を参照。

---

## 4. 今後の扱いについて

- 🔴High・大半の 🟡Medium（横断パターン P1〜P5 の「該当箇所」）は解消済み。残るのは各パターンの対象外節に明示された同系統の所見と、🟠Low/🔵Info の大半、および E1 全件である。E1 は他コンポーネントと異なり横断パターン経由の部分対応も一度もなく、個別タスク化が未着手のまま残っている。
- 上記はいずれも「直接の侵入経路ではない」「前提条件（group-writable 権限、fail-open の悪用条件等）が必要」と評価されたものであり、緊急対応は不要と判断されている（各所見の詳細評価は `findings/*.md` を参照）。
- 個別タスク化する際は、本ドキュメントの該当節と `findings/*.md` の該当所見 ID を起点に、0150〜0161 と同様の要件定義プロセス（[requirements_process.md](../../dev/developer_guide/requirements_process.md)）に従うこと。
