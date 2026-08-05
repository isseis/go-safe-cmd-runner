# セキュリティクリティカル部 code smell 監査 集約サマリ

- 監査日: 2026-07-18〜2026-07-19
- 監査方法: Claude モデル **fable** による静的コードレビュー（読み取り専用、コード修正なし）。17 コンポーネントを個別タスクとして逐次監査。
- 詳細所見: `findings/*.md`（各コンポーネント個別ファイル）
- 実行計画: `00_execution_plan.md`

---

## 1. 全体サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 2 |
| 🟡 Medium | 46 |
| 🟠 Low | 61 |
| 🔵 Info | 65 |
| **合計** | **174** |

致命的な脆弱性（任意コード実行・認証バイパス等に直結するもの）は検出されなかった。🔴High 2件はいずれも「エラー処理の縮退により安全側の判定が緩む fail-open」パターンであり、直接の侵入経路ではなく防御層の劣化として扱う。

### コンポーネント別内訳

| ID | コンポーネント | 🔴 | 🟡 | 🟠 | 🔵 | ファイル |
|---|---|---|---|---|---|---|
| A1 | `runner/base/privilege` | 0 | 2 | 5 | 4 | [A1_privilege.md](findings/A1_privilege.md) |
| A2 | `runner/base/executor` | 0 | 3 | 3 | 6 | [A2_executor.md](findings/A2_executor.md) |
| A3 | `runner/base/environment` | 0 | 2 | 2 | 3 | [A3_environment.md](findings/A3_environment.md) |
| A4 | `runner/base/security` | 0 | 2 | 4 | 3 | [A4_security.md](findings/A4_security.md) |
| A5 | `runner/base/risk` | 0 | 1 | 4 | 3 | [A5_risk.md](findings/A5_risk.md) |
| A6 | `runner/base/output` | 0 | 2 | 3 | 3 | [A6_output.md](findings/A6_output.md) |
| A7 | `runner/base/audit` | 0 | 3 | 2 | 5 | [A7_audit.md](findings/A7_audit.md) |
| B1 | `safefileio` | 0 | 2 | 3 | 4 | [B1_safefileio.md](findings/B1_safefileio.md) |
| B2 | `filevalidator` (+pathencoding) | 0 | 2 | 6 | 5 | [B2_filevalidator.md](findings/B2_filevalidator.md) |
| B3 | `verification` | 0 | 2 | 4 | 4 | [B3_verification.md](findings/B3_verification.md) |
| B4 | `runner/config` | 0 | 2 | 4 | 3 | [B4_config.md](findings/B4_config.md) |
| C1 | バイナリ解析 (elf/macho/binaryanalyzer) | 0 | 2 | 4 | 5 | [C1_binary_analysis.md](findings/C1_binary_analysis.md) |
| C2 | `dynlib` (+libccache) | 0 | 3 | 3 | 5 | [C2_dynlib.md](findings/C2_dynlib.md) |
| C3 | `shebang` / `fileanalysis` | 0 | 3 | 5 | 3 | [C3_shebang_fileanalysis.md](findings/C3_shebang_fileanalysis.md) |
| D1 | `groupmembership` | **1** | 4 | 4 | 2 | [D1_groupmembership.md](findings/D1_groupmembership.md) |
| D2 | `logging` / `redaction` | **1** | 5 | 6 | 6 | [D2_logging_redaction.md](findings/D2_logging_redaction.md) |
| E1 | エントリポイント (cmd/*, bootstrap, cli) | 0 | 3 | 7 | 6 | [E1_entrypoints.md](findings/E1_entrypoints.md) |

---

## 2. 🔴 High 所見（優先対応）

### H-1 (D1): CGO 版 `getGroupMembers` がエラーを「メンバー 0 人」に握りつぶし、group-writable 書き込み判定が fail-open になる

- **該当箇所**: `internal/groupmembership/membership_cgo.go:122-127`, `manager.go:185-197`
- **概要**: `getgrgid_r` の ERANGE・NSS 障害・malloc 失敗を Go 側が区別せず「メンバー 0 人・エラーなし」として返す。`isUserOnlyGroupMember` はこれを「ユーザーが唯一のメンバー」と誤解釈し、group-writable ファイルへの書き込みを安全と判定する（fail-open）。大きなグループ（`_SC_GETGR_R_SIZE_MAX` 超）や一時的な NSS/LDAP 障害で現実的に発生しうる。
- **推奨対応**: C 側で「見つからない」と「エラー」を区別し、エラー時は必ず `(nil, err)` を返す。ERANGE はバッファ倍々拡大でリトライ。

### H-2 (D2): `RedactingHandler.Handle` がログレコードの **Message 本文を redact しない**

- **該当箇所**: `internal/redaction/redactor.go:403-415`, `internal/logging/slack_handler.go:823-827`
- **概要**: redaction は属性（Attrs）のみに適用され、`record.Message` 文字列は素通り。`slog.Error(fmt.Sprintf("... %v", err))` のようにメッセージ本文へ機密（credential 入り URL・トークン等）を埋め込むコードが将来／既存に存在すると、file/stderr に加え `slack_notify=true` 経由で Slack へも平文で送出されうる。
- **推奨対応**: `Handle` 内で `newRecord` 作成時に message にも `RedactText` を適用する。

---

## 3. 横断的パターン（複数コンポーネントに共通する根本原因）

### P1: エラー処理の縮退による fail-open（最重要パターン、🔴2件を含む）

「解析・検証に失敗した場合、安全側ではなく『対象なし』『問題なし』に倒れる」という同型の欠陥が広く分布している。

- D1 (groupmembership) H-1, M-1, L-2, L-3: グループメンバー列挙失敗 → 「メンバー0人」→ 書き込み許可
- C1 (binary analysis) F-1: syscall ストア読み取り失敗 → `StaticBinary` に縮退
- C2 (dynlib) F-3, F-5: 子依存パース失敗・シーク失敗 → 「依存なし」
- B3 (verification) M1, L1: パス解決失敗・DynString エラー → 検証対象から除外
- A5 (risk) Low-3: 未知の `BinaryAnalysisClass` → 「寄与なし」（ゼロ値が Uncertain のため実害限定）

**推奨**: 「解析不能」「エラー」「対象なし」を型レベルで区別し、解析不能・エラー系は一律 fail-closed（Blocking/AnalysisError 相当）に倒す設計原則を横断的に適用する。

### P2: redaction 境界の不統一（機密情報漏洩リスク）

- D2 H-1: メッセージ本文が redaction 対象外
- D2 M-1: `slog.Any` の map/struct/スライス要素が redaction を素通り
- D2 M-3: Slack webhook URL 自体がエラーログに漏れる
- A7 M-1〜M-3 (audit): `LogRiskProfile` のみ境界 redaction を実装し、`LogUserGroupExecution`/`LogSecurityEvent`/`LogPrivilegeEscalation` は非対称
- A4 M-2 (security): `SanitizeEnvironmentVariables` がキー名のみで判定し値を見ない

**推奨**: 「audit/logging パッケージを通る文字列は必ず redaction される」という単一の不変条件を、メッセージ本文・map・スライス要素まで含めて全メソッドに遡及適用する。D2/A7 は同一の根本原因（RedactingHandler が slog.Any の map/slice に再帰しない）に帰着するため、まとめて解消可能。

### P3: 検証（verify）とバインド（open/exec）の間の TOCTOU 残存

多層防御（fd-bound exec, openat2）で大半は閉じられているが、以下の窓が残る:

- A5 Medium-1: risk 評価の `openVerifiedIdentity` がハッシュ検証時点と open 時点の間で内容ハッシュを再検証しない
- B1 F-1: `AtomicMoveFile` がfdで検証したソースをパス名でrenameする
- B2 B2-1, B2-3: record 時のハッシュ計算と各種解析（shebang/dynlib/syscall）が別々の open で行われる
- C2 F-1: verify 時に依存解決を再実行しないため RUNPATH/@rpath の探索順シャドーイングを検出できない
- B3 L3, L4: PathResolver の Stat→EvalSymlinks、shebang symlink 検査と exec 間

**推奨**: 「検証と使用は同一 fd/同一読み取りから行う」原則を record/risk 評価の残り経路に展開する（B1/B2/A5 が具体的な着手候補）。

### P4: 環境変数 denylist の非対称・抜け

- A2 M-1 (executor): `DYLD_*`・`GLIBC_TUNABLES`・`BASH_ENV` 等がdenylist未登録
- A4 M-1 (security): `env NAME=VALUE` 経由の `NODE_OPTIONS`/`PERL5OPT`/`PYTHONPATH` 等が `LD_*` と非対称に未拒否
- B4 L-1 (config): `env_vars` の KEY に対し config 層で禁止名チェックなし（実行層のスクラブに依存）

**推奨**: インタプリタ・ローダ制御変数のdenylistを一箇所に集約し（DRY）、`LD_*`/`DYLD_*` と同水準でインタプリタ系変数も拒否する。

### P5: 「フィルタする」と称して実質フィルタしていない・命名と実装の乖離

- A3 (environment): `FilterSystemEnvironment`/`FilterGlobalVariables` は allowlist を一切適用しない（死んだ `globalAllowlist` フィールド、誤用一歩手前の footgun）
- A1 M-2 (privilege): `changeUserGroupInternal` の実降格パスが本番到達不能なデッドコード、`WithUserGroup` の命名と実装が乖離
- D1 M-4 (groupmembership): `getProcessEUID` が実際には実UIDを返す

**推奨**: パッケージ縮退・改名によるリファクタリング（A3）、デッドコード削除（A1 M-2, C3 F3）を個別タスク化して解消する。

---

## 4. 観察された良好な防御層（横断ハイライト）

個別ファイルに詳細を記載しているため代表例のみ。全コンポーネントに共通する設計哲学として、以下が一貫して確認された。

1. **fail-closed 優先の設計文化**: risk evaluator・binary analyzer・security validator など判定系コンポーネントのほぼ全てで「不明・解析不能 → 拒否/High」に倒す方針が徹底されている（P1 の縮退パターンはこの方針からの局所的な逸脱として位置づけられる）。
2. **TOCTOU 対策の多層化**: `openat2(RESOLVE_NO_SYMLINKS)`、fd-bound exec（`/proc/self/fd/N`）、`SafeOpenFile` の一貫使用により、大半の symlink 差し替え攻撃が構造的に排除されている。
3. **特権管理の堅牢性**: `emergencyShutdown` による fail-closed、復元後の独立 identity 検証、saved-set-uid/gid 不変条件チェック（privilege パッケージ）。
4. **監査ログの縦深防御**: `LogRiskProfile` の境界 redaction、deny severity floor、Webhook URL の宛先 allowlist（fail-closed）。
5. **設定検証の多層 DoS 対策**: 再帰深度・変数数・文字列長の上限、循環参照検出、厳格 TOML デコード。

---

## 5. 対応優先度（推奨）

1. **H-1 (D1 groupmembership)** — グループ列挙失敗の fail-open。修正コストは中（C側リトライ実装＋エラー伝播）。**最優先**。
2. **H-2 (D2 logging)** — メッセージ本文 redaction 漏れ。修正コストは小〜中（Handle 関数に1行相当の適用漏れを追加）。**最優先**。
3. **P2（redaction 境界統一）** — H-2 と同根。A7 の3件（M-1〜M-3）と合わせて一括対応することで費用対効果が高い。
4. **P1（fail-open 系 Medium 群）** — 各所に分散するが根本原因は共通。横展開でまとめて修正方針を立てられる（C1 F-1, C2 F-3/F-5, B3 M1/L1 等）。
5. **P4（環境変数 denylist 拡充）** — A2 M-1 と A4 M-1 は同一防御思想の穴。denylist の一元化と合わせて対応。
6. **P3（TOCTOU 残存）** — 実運用では前提条件（対象ディレクトリへの書き込み権限）が必要なため優先度は中。A5 Medium-1・B2 B2-1 から着手が妥当。
7. **P5（デッドコード・命名乖離）** — 直接のセキュリティ実害はないが監査コスト増大要因。A3 パッケージ縮退を含め、YAGNI 原則に沿って整理。
8. **🟠Low/🔵Info 全般** — 各ファイルの推奨対応を参照し、通常の改善サイクルで対応。

---

## 6. 監査の限界

- 静的読解のみ（動的テスト・ファジングは対象外）。
- 各コンポーネントは独立監査のため、コンポーネント間の相互作用（例: config 層の TOCTOU が verification 層でどこまで緩和されるか）は個別ファイル内で言及されているものの、本監査全体を通じた統合的な動的検証は範囲外。
- 外部ライブラリ（`pelletier/go-toml/v2`, `oklog/ulid/v2` 等）の内部実装は対象外。

---

## 7. 対応状況（Task 0150〜0161）

本監査（Task 0149）で検出された所見のうち、🔴High 2 件・横断パターン P1〜P5 の主要部分は Task 0150〜0161 で対応済みである。個別の残件は [98_remaining_issues.md](98_remaining_issues.md) に集約した。

| Task | 対応内容 | 主な対応所見 | 状態 |
|---|---|---|---|
| [0150](../0150_groupmembership_getgrgid_failclosed/) | 0151 に統合（設計検討のみ、実装は 0151 に集約） | D1 H-1 の初期検討 | 統合済み |
| [0151](../0151_groupmembership_failclosed/) | groupmembership のグループメンバー列挙 fail-closed 化、CGO/非CGO 意味論統一、`isUserOnlyGroupMember` 特例分岐削除 | **H-1**、D1 M-1、M-2 | ✅完了 |
| [0152](../0152_redact_log_message_body/) | `RedactingHandler.Handle` がログメッセージ本文を redact するよう修正 | **H-2** | ✅完了 |
| [0153](../0153_failopen_error_handling_crosscut/) | P1（fail-open の縮退）の残り 6 箇所を fail-closed 化 | C1 F-1、C2 F-3/F-5、B3 M1/L1、A5 Low-3 | ✅完了 |
| [0154](../0154_redaction_boundary_unification/) | RedactingHandler の map/struct/slice 再帰 redaction、Slack エラーログの URL 除去、audit ログ 4 メソッドの境界 redaction 統一、環境変数値ベース redaction | P2（D2 M-1/M-3、A7 M-1/M-2/M-3、A4 M-2） | ✅完了 |
| [0155](../0155_toctou_verify_use_residual_gaps/) | risk 評価のハッシュ再検証、`AtomicMoveFile` の fd アンカー化、record 時のハッシュ計算と解析の一貫性、verify 時の依存解決再実行、PathResolver の解決順序修正、shebang symlink 残余リスクの文書化 | P3（A5 Medium-1、B1 F-1、B2 B2-1/B2-3、C2 F-1、B3 L3/L4） | ✅完了 |
| [0156](../0156_env_denylist_consolidation/) | 禁止環境変数名判定の一元化、`DYLD_*`/`GLIBC_TUNABLES`/インタプリタ起動時コード注入変数の追加、`env_vars` への denylist 適用 | P4（A2 M-1、A4 M-1、B4 L-1） | ✅完了 |
| [0157](../0157_dead_code_naming_cleanup/) | `environment.Filter` の縮退・改名、到達不能な実降格パスの削除、`getProcessEUID` の命名整合、未使用 syscall analysis ストアの削除 | P5（A3 F-1〜F-5、A1 M-2、D1 M-4、C3 F3） | ✅完了（軽微なドキュメント横断検索項目 2 件は 0159/0160 で解消済み） |
| [0158](../0158_dryrun_runas_ident_unification/) | dry-run のユーザー・グループ検証を実行時の識別情報解決（`ResolveRunAsIdent`）に統合し、`user.Lookup` の二重呼び出しを解消 | A1 L-1（0157 フォローアップ） | ✅完了 |
| [0159](../0159_security_architecture_update/) | セキュリティ設計文書 §5「特権管理」を実装に合わせて全面更新 | ドキュメント整合（0157 フォローアップ） | ✅完了 |
| [0160](../0160_permission_check_subject_explicit/) | 権限チェックの基準UIDをバイナリごとの明示方針（`RealUIDOnly`/`SudoUIDAware`）で決定するよう変更、`runner` の読み取り判定経路から `SUDO_UID` 参照を除去 | D1 M-3 の一部（0157 フォローアップ） | ✅完了 |
| [0161](../0161_sudo_uid_validation_and_logging/) | `record`/`verify` が採用する `SUDO_UID` の実在検証、監査ログへの記録 | D1 M-3 の残り | ✅完了 |

**要点**:

- 🔴High 2 件（H-1, H-2）はいずれも解消済み。
- 横断パターン P1〜P5（§3）は対応範囲に挙げた所見について解消済み。ただし各パターンの「該当箇所」リストに含まれなかった同系統の所見（同じ P1〜P5 のカテゴリに属するが個別タスクの対象外とされたもの）は残っている。詳細は [98_remaining_issues.md](98_remaining_issues.md) を参照。
- D1 M-3（`SUDO_UID` の無検証信頼）は 0160（権限チェック主体の明示指定）と 0161（`SUDO_UID` 実在検証・監査ログ）の 2 段階で解消済み。
- 未着手のまま残る所見（D1 L-2/L-3、A1 L-2〜L-4、B1 F-2〜F-9、B2 B2-2/B2-4〜13、B3 M2/L2/Info、C1 F-2〜F-8、C2 F-2/F-4/F-6〜11、C3 F1/F2/F4 以降、D2 M-2/M-4/M-5、A7 L-1/L-2/I-1〜5、E1 全件、および各コンポーネントの 🟠Low/🔵Info 全般）は [98_remaining_issues.md](98_remaining_issues.md) にまとめた。

### 7.1 E1（エントリポイント）所見の現行コードとの照合（2026-08-05 実施）

Task 0150〜0161 はいずれも `cmd/runner`・`cmd/record`・`cmd/verify`・`bootstrap`・`cli` を直接の対象にしていない。E1 の16件（🟡Medium 3・🟠Low 7・🔵Info 6）について、後続タスク（とくに 0157/0158/0160 が特権・識別情報解決の実装を変更している）による副次的解消がないかを現行コードで個別に確認した。結論として **16件全件が現行コードにそのまま該当し、解消されたものはない**。

| ID | 所見 | 現行コードでの確認結果 |
|---|---|---|
| M-1 | `--run-id` 未検証（パストラバーサル/ログ注入） | 未対応。`cmd/runner/main.go` の `run-id` フラグはユーザー指定値をそのまま `logger.go:138` の `filepath.Join` に渡しており、ULID 形式検証・`filepath.Base` チェックのいずれも追加されていない。 |
| M-2 | 起動時特権降格が euid のみ（egid・補助グループ未降格） | 未対応。`cmd/runner/main.go:109` は依然 `syscall.Seteuid(syscall.Getuid())` のみで `Setegid` 呼び出しはなく、`flag.Parse()`（:95）より後で実行される順序も変わっていない。**0157/0160 で整理された「特権実行を exec 時の `syscall.Credential` 指定に一本化する」仕組みは、実行するコマンドへの per-command 特権付与を扱うものであり、`runner` バイナリ自身が setuid-root として起動した直後の自己降格（本所見の対象）とは別レイヤーである。したがって本所見は 0157/0160/0161 のいずれによっても解消されていない。** |
| M-3 | verify の TOCTOU チェックが fail-open（警告のみ） | 未対応。`cmd/verify/main.go` の `run TOCTOU permission check` ブロックのコメント「Violations are logged as warnings only — verify continues even if the check fails.」は現行のまま存置されており、`RunTOCTOUPermissionCheck` の戻り値で `run` を打ち切る分岐はない。 |
| L-1 | record が TOCTOU チェック前にハッシュディレクトリを `mkdirAll` | 未対応。`cmd/record/main.go` の `parseArgs`（:274）が `mkdirAll` を実行し、`run`（:159）がその後で `checkDirPermissions`（:185）を呼ぶ順序は変わっていない。 |
| L-2 | `filepath.Abs`/`EvalSymlinks` 失敗時の無警告フォールバック | 未対応。`cmd/record/main.go`・`cmd/verify/main.go` とも失敗時に元のパスへ黙ってフォールバックする同一パターンのコードが残る（ログ出力なし、共通化もされていない）。 |
| L-3 | verify のハッシュディレクトリ作成副作用 + パーミッション不一致 | 未対応。`cmd/verify/main.go` は `hashDirPermissions = 0o750` のまま無条件に `mkdirAll` を実行し、`cmd/record/main.go` の `0o700` との不一致も残る。 |
| L-4 | `runTOCTOUCheck` の変数参照・相対パスのサイレントスキップ | 未対応。`resolveStaticAbsPath`（`cmd/runner/main.go:320`）は `%{` を含むパス・相対パスを無音でスキップする実装のままで、スキップ件数のログ出力は追加されていない。 |
| L-5 | ログファイル名のタイムスタンプが実際はローカル時刻なのに `Z`（UTC）表記 | 未対応。`internal/runner/bootstrap/logger.go:77` は `time.Now().Format("20060102T150405Z")` のままで `.UTC()` 変換はない。 |
| L-6 | Phase 1/Phase 2 間のエラー（設定改ざん検出含む）が Slack 未通知 | 未対応。`cmd/runner/main.go` の `SetupLogging` → `LoadAndPrepareConfig` → `SetupSlackLogging` の順序は変わらず、設定ロード失敗時点では Slack ハンドラが未登録のまま。 |
| L-7 | `DirectoryPermChecker` 初期化失敗時に3箇所とも panic | 未対応。`cmd/runner/main.go`・`cmd/record/main.go`・`cmd/verify/main.go` のいずれも `NewDirectoryPermChecker` 失敗時に同一の `panic(fmt.Sprintf(...))` を実行する実装のまま。 |
| I-1 | 起動時降格後も saved-uid が root のままである設計意図が未記載 | 未対応。`cmd/runner/main.go:109` 付近に privilege manager との関係を説明するコメントは追加されていない。 |
| I-2 | dry-run formatter の switch に `default` 節がない | 未対応。`cmd/runner/main.go:478` の `switch outputFormat` は `OutputFormatText`/`OutputFormatJSON` の2ケースのみで `default` 節はない。 |
| I-3 | Slack env 検証エラーの stderr 二重出力 | 未対応。`cmd/runner/main.go:191` の `fmt.Fprintln(os.Stderr, err.Error())` は残っており、`HandlePreExecutionError` 経由の出力と重複する構造は変わっていない。 |
| I-4 | `normalizeSlackAllowedHost` の IPv6 分岐で大文字小文字未統一 | 未対応。`internal/runner/bootstrap/config.go` の IPv6 分岐は引き続き `u.Hostname()` を無変換で返し、ホスト名分岐のみ `strings.ToLower`（:54）を適用する非対称が残る。 |
| I-5 | verify のパッケージレベル変数注入と record の `deps` 構造体注入の様式乖離 | 未対応。`cmd/verify/main.go` は依然 `validatorFactory`/`mkdirAll` をパッケージレベル変数で注入し、`cmd/record/main.go` の `deps` 構造体方式と乖離したまま。付随する `cacheDir`/`machoCacheDir` の重複計算（`cmd/record/main.go`）も解消されていない。 |
| I-6 | bootstrap/logger のグローバル可変状態 | 未対応。`redactionErrorCollector`/`phase1BaseHandlers` 等のパッケージグローバル変数によるフェーズ間受け渡しは変わらず、`LoggerBootstrap` 相当の構造体化は行われていない。 |

**結論**: E1 は 0149 監査以降、一度も個別タスク化されていない。他コンポーネントの横断パターン対応（0153〜0157）が特権管理・環境変数・redaction・fail-open 判定のロジックを広く修正した一方で、エントリポイント（`cmd/*`・`bootstrap`）自体のコードは変更されておらず、16件は当時の所見のまま現存する。次にタスク化する際の優先候補として扱う（[98_remaining_issues.md](98_remaining_issues.md) §4 参照）。
