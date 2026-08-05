# 要件定義書: entrypoints の run-id 検証・特権降格完全化・verify TOCTOU fail-closed化

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-08-05 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#974 [Security][E1 M-1/M-2/M-3] entrypoints: run-id未検証・特権降格不完全・verify TOCTOU fail-open](https://github.com/isseis/go-safe-cmd-runner/issues/974)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/E1_entrypoints.md](../0149_security_code_smell_audit_fable/findings/E1_entrypoints.md) の M-1・M-2・M-3
- 残件一覧: [docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §1
- 関連（本タスクでは扱わない）: E1 の L-1〜L-7・I-1〜I-6（[#986](https://github.com/isseis/go-safe-cmd-runner/issues/986)）

## 背景

0149 監査の所見 E1（entrypoints: `cmd/runner`・`cmd/record`・`cmd/verify`・`bootstrap`）のうち、Task 0150〜0161 のいずれの対象にもならず未着手のまま残っている Medium 3件（M-1・M-2・M-3）をまとめて解消する。3件はいずれも起動処理（`cmd/runner`・`cmd/verify` の `main`）という同一領域に属する。

### M-1: `--run-id` が未検証のままログファイル名・ログ行に埋め込まれる

`cmd/runner/main.go` の `--run-id` フラグ（`main.go:78` で定義）は "auto-generates ULID if not provided" と説明されているが、ユーザー指定値に対する形式検証は一切行われない（`main.go:88-90` でそのまま採用）。この値は2箇所でそのまま使われる。

1. `internal/runner/bootstrap/logger.go:138` の `logPath := filepath.Join(config.LogDir, fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, config.RunID))`。`filepath.Join` はパスを Clean するのみで封じ込めは行わないため、`--run-id '../../../tmp/evil'` のような値で `--log-dir` の外に `O_CREATE|O_TRUNC` でファイルを作成・切り詰めできる。
2. `internal/logging/pre_execution_error.go:124` の `RUN_SUMMARY run_id=%s exit_code=... status=...` 行、および同ファイル内の stderr 出力（`runID` を含む）。空白・改行を含む値を渡すことで、機械可読な RUN_SUMMARY 行の偽装（ログ行注入）が可能。

`--run-id` フラグを持つのは `cmd/runner` のみであり、`cmd/record`・`cmd/verify` にはこのフラグは存在しない（`--run-id` は runner 経由でのみユーザーから渡される）。

自動生成側は `internal/logging/safeopen.go` の `GenerateRunID` が `github.com/oklog/ulid/v2` で ULID を生成しており、この形式が「正規の run ID」の基準になる。

### M-2: 起動時特権降格が euid のみ（egid・補助グループ未降格、実行順序も不適切）

`runner` は setuid-root バイナリとしての起動を前提としており、`cmd/runner/main.go:109` で `syscall.Seteuid(syscall.Getuid())` を実行して即時に euid を降格している。しかし以下の2点が未対応。

1. `Setegid` が呼ばれておらず、egid は特権グループのまま残る。バイナリが setgid 構成で配布・インストールされた場合（あるいは setuid+setgid の場合）、以後の全処理（フラグ処理、ログセットアップ、設定読み込み、TOCTOU チェック、コマンド実行準備）が特権 GID のまま走る。現行の setuid-root のみの想定では実害はないが、この防御は現在の配布構成に暗黙のうちに依存しており、構成が変わると前提が崩れる。
2. 降格処理が `flag.Parse()`（`main.go:95`）より後に実行されている。「特権降格は `main` の可能な限り先頭で」という原則からの逸脱であり、`flag.Parse()` 自体は攻撃対象領域を広げないが、原則としての一貫性を欠く。

0157/0160/0161 で整理された「exec 時の `syscall.Credential` 指定への一本化」は、実行する**子コマンドへの** per-command 特権付与を扱うものであり、`runner` バイナリ**自身**が起動直後に行う自己降格（本件）とは別レイヤーであるため、これらのタスクによって本件は解消されていない。

### M-3: verify の TOCTOU（Time-Of-Check-To-Time-Of-Use。チェック時点と使用時点の間の状態変化を突く競合状態）チェックが fail-open（警告のみで続行）

`cmd/verify/main.go:78-109` の TOCTOU 権限チェックは、`security.RunTOCTOUPermissionCheck` が検出した違反を `slog.Warn` で記録するのみで、戻り値の `[]TOCTOUViolation` を無視して処理を継続する（コメント: "Violations are logged as warnings only — verify continues even if the check fails."）。

対比として `cmd/record/main.go` の `checkDirPermissions`（`main.go:102-152`）は、ハッシュ DB を root of trust（信頼の起点）と位置づけ、同じ違反検出時にハッシュ記録の生成を拒否する（fail-closed、`run` はこの関数が `false` を返すと即座に exit 1 する。`main.go:186`）。

verify は `-hash-dir`／`-d` で任意のハッシュディレクトリを指定できるため、攻撃者が書き込めるディレクトリ（＝違反が検出されるケースそのもの）では対象ファイルとハッシュ記録の両方を差し替えられる。この状態で verify が `OK`／exit 0 を返すと、それを信頼判断に使う運用者や監視スクリプトが改ざんを見逃す。

## 目的

- `--run-id` にユーザー指定の任意文字列を許さず、正規の ULID 形式（または runner が生成する形式）のみを受理し、形式に反する値は起動前に拒否する。ログファイル名構築側にも多層防御のチェックを追加する。
- `runner` の起動時特権降格を、egid を含めたうえで `flag.Parse()` より前に実行する形に改める。
- `verify` の TOCTOU チェック違反を fail-closed（対象ファイルを1件も処理せず非ゼロ終了）とし、`record` との非対称を解消する。

## スコープ

### 対象

1. `cmd/runner` の `--run-id` フラグの検証（ULID 形式チェック、不一致時の `PreExecutionError` による拒否）。
2. `internal/runner/bootstrap/logger.go` のログファイル名構築における多層防御チェック（`filepath.Base(runID) == runID` 相当）。
3. `cmd/runner/main.go` の起動時特権降格処理の移動（`flag.Parse()` より前）と `Setegid(syscall.Getgid())` の追加（順序: egid → euid）。
4. `cmd/verify/main.go` の TOCTOU チェックを fail-closed 化し、違反時は対象ファイルを1件も処理せず非ゼロで終了させる。
5. 上記に対応するテストの追加・更新。
6. 影響を受ける利用者向け文書（`--run-id` の形式制約、`verify` の fail-closed 挙動）の更新。

### 対象外

- `cmd/record`・`cmd/verify` に `--run-id` フラグを新設すること（現状どちらのバイナリにもこのフラグは存在せず、本タスクはその追加を含まない）。
- 補助グループ（supplementary groups、`syscall.Setgroups`）の降格。所見 M-2 の推奨対応は egid の降格のみを挙げており、補助グループの取り扱いは対象外とする（将来必要になった場合は別タスクとする）。
- E1 の L-1〜L-7・I-1〜I-6（[#986](https://github.com/isseis/go-safe-cmd-runner/issues/986) で別途管理）。特に以下は本タスクと隣接するが対象外:
  - L-2（`filepath.Abs`/`EvalSymlinks` 失敗時のサイレントフォールバック、record/verify 共通のコード重複）
  - L-3（verify のハッシュディレクトリ作成副作用とパーミッション不一致 `0o750`/`0o700`）
  - I-1（起動時降格後も saved-uid が root のままである設計意図の文書化）
- `record` の `checkDirPermissions` の挙動変更（既に fail-closed であり、本タスクでは変更しない。verify 側の実装で参考・再利用するのみ）。
- ULID 生成ライブラリ（`github.com/oklog/ulid/v2`）の差し替え。
- `runner` の native root 実行サポートの是非（[#921](https://github.com/isseis/go-safe-cmd-runner/issues/921)）。

## 検討事項（設計判断が必要な項目、02_architecture.md で決定する）

- **`--run-id` の受理形式**: 所見の推奨は「ULID 形式、または `^[A-Za-z0-9_-]+$` + 長さ上限」の二択。`GenerateRunID` が生成する値は厳密な ULID（Crockford Base32, 26文字）であるため、ユーザー指定値も `ulid.Parse` で検証可能な厳密な ULID 形式に限定するか、より緩い正規表現（英数字・アンダースコア・ハイフンのみ）を許容するかを決定する。後者を採る場合、`--log-dir` 外への書き込みとログ行注入を防げる文字集合であることを確認する。
- **拒否時のエラー種別**: 既存の `logging.ErrorType`（`ErrorTypeBuildConfig` 等）に倣い、`--run-id` 形式不正専用の `ErrorType` を新設するか、既存の型を流用するかを決定する。
- **`flag.Parse()` より前に降格する際の runID の扱い**: 現行コードは `flag.Parse()` の後に `runID` を確定し、降格失敗時のエラー報告（`logging.HandlePreExecutionError`）にその `runID` を使っている。降格処理を `flag.Parse()` より前に移動すると、降格失敗時点ではユーザー指定の `--run-id` はまだ解析されていない。降格失敗時のエラー報告に使う ID を「先に自動生成し、`--run-id` 解析後に正規値へ差し替える」か、他の方式を採るかを決定する。
- **`logger.go:138` の多層防御チェック失敗時の挙動**: `--run-id` の入口検証を通過していれば理論上到達しないが、防御的チェックが失敗した場合に `SetupLoggerWithConfig` がエラーを返す形にするか、別の対処にするかを決定する。
- **verify の fail-closed 化の実装方式**: `record.checkDirPermissions` とほぼ同一のロジックを `verify` にも実装することになるため、共通ヘルパーとして切り出すか、verify 内に同様のロジックを複製するかを決定する（既存の重複コード自体は L-2 で別途扱われるため、本タスクでは fail-closed 化に必要な範囲でのみ判断する）。
- **verify fail-closed 時の終了コード・メッセージ**: `record` の `checkDirPermissions` はエラーログ＋stderr メッセージ＋exit 1 という形を取っている。`verify` でも同じパターンに揃えるか、`verify` 固有の文言にするかを決定する。

## Acceptance Criteria

#### F-001: `--run-id` の形式検証

`cmd/runner` が `--run-id` に指定された値を検証し、不正な形式であれば起動前に拒否する。

**Acceptance Criteria**:
- **AC-01**: `--run-id` を指定しない場合、既存どおり `GenerateRunID` による ULID が自動生成され、実行が継続する（回帰なし）。
- **AC-02**: `--run-id` に有効な形式（検討事項で決定する受理形式に合致する値）を指定した場合、その値がそのまま run ID として採用され、実行が継続する。
- **AC-03**: `--run-id` にパストラバーサル文字（`/`、`..`）を含む値を指定した場合、実行前に拒否され、非ゼロで終了する。ログファイルは作成されない。
- **AC-04**: `--run-id` に空白・改行を含む値を指定した場合、実行前に拒否され、非ゼロで終了する。
- **AC-05**: AC-03・AC-04 の拒否は `logging.PreExecutionError` として発生し、`errors.Is`／`errors.As` で判別できる。ログファイルオープンおよび `RUN_SUMMARY` の出力より前に発生する。
- **AC-06**: AC-03・AC-04 の拒否時、stderr に理由（受理される形式）を含むメッセージが出力される。

#### F-002: ログファイル名構築側の多層防御

`internal/runner/bootstrap/logger.go` のログファイル名構築が、`RunID` に起因するパストラバーサルを二重に防ぐ。

**Acceptance Criteria**:
- **AC-07**: `SetupLoggerWithConfig` に渡された `LoggerConfig.RunID` が `filepath.Base(runID) == runID` を満たさない場合（`/` を含む、`..` そのものである等）、ログファイルを作成せずエラーを返す。
- **AC-08**: AC-07 のチェックは、F-001 の入口検証を経由しない呼び出し（テストからの直接呼び出し等）に対しても機能する。
- **AC-09**: 正常な ULID 形式の `RunID` を渡した既存の呼び出しでは、AC-07 のチェックによる回帰が発生しない（既存のログファイル作成テストが通過する）。

#### F-003: 起動時特権降格の完全化と実行順序修正

`cmd/runner` の起動時特権降格が、`flag.Parse()` より前に、egid を含めて実行される。

**Acceptance Criteria**:
- **AC-10**: `main()` の実行順序として、`Setegid(syscall.Getgid())` → `Seteuid(syscall.Getuid())` の順に実行され、両方とも `flag.Parse()` より前に完了する。
- **AC-11**: `Setegid` が失敗した場合、`Seteuid` は実行されず、fail-closed（非ゼロ終了、対象ファイル処理は一切行われない）となる。
- **AC-12**: `Seteuid` が失敗した場合、AC-11 と同様に fail-closed となる（現行の挙動を維持）。
- **AC-13**: `Setegid`・`Seteuid` がともに成功した場合、降格処理を `flag.Parse()` より前に移動したことによる `--run-id` 等の既存フラグ解析結果への回帰がない。
- **AC-14**: AC-11・AC-12 の失敗時のエラー報告（`logging.HandlePreExecutionError`）に使われる run ID が空文字列や未定義にならない（検討事項の決定に従う）。

#### F-004: verify の TOCTOU チェック fail-closed 化

`cmd/verify` の TOCTOU 権限チェックで違反が検出された場合、対象ファイルの検証を1件も行わず非ゼロで終了する。

**Acceptance Criteria**:
- **AC-15**: TOCTOU チェックで1件以上の違反が検出された場合、`verify` は対象ファイルを1件も検証せず、非ゼロで終了する。
- **AC-16**: AC-15 の場合、各違反が `log/slog` に ERROR レベルで記録される（現行の `slog.Warn` に加えて、または置き換えて）。
- **AC-17**: AC-15 の場合、stderr に「検証結果は信頼できない」旨と是正方法（該当ディレクトリの権限修正）を示すメッセージが出力される。
- **AC-18**: TOCTOU チェックで違反が検出されない場合、既存どおり検証が継続され、exit code はファイルごとの検証結果のみに依存する（回帰なし）。
- **AC-19**: AC-15 の fail-closed 判定は、`-hash-dir`／`-d` で指定された任意のハッシュディレクトリに対しても機能する。

#### F-005: 文書の更新

本タスクによる利用者向けの挙動変化を文書に反映する。

**Acceptance Criteria**:
- **AC-20**: `runner` の利用者向け文書に、`--run-id` が受理する形式（検討事項で決定）と、不正な値を指定した場合に起動前に拒否されることが記載されている。日本語版を先に更新し、英語版は `/mktrans` で反映する。
- **AC-21**: `verify` の利用者向け文書に、TOCTOU チェックで違反が検出された場合は対象ファイルを1件も処理せずに非ゼロで終了することが記載されている。日本語版を先に更新し、英語版は `/mktrans` で反映する。
- **AC-22**: `docs/translation_glossary.md` に、本タスクで新たに導入した用語があれば追加されている。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが `make test` で成功する。
- `make lint` が警告なく通過する。
- `runner` について、有効な ULID 形式の `--run-id` を指定する通常運用、および `--run-id` を指定しない通常運用のいずれにおいても、0149 完了時点からの外部から観測可能な挙動の変化がない。
- `verify` について、TOCTOU チェックで違反が検出されない通常運用では、0149 完了時点からの外部から観測可能な挙動の変化がない。
