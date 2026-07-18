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
