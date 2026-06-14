# 実行時リスク判定の強化 — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-13 |
| Review date | - |
| Reviewer | - |
| Comments | 2026-06-13: 作成中の事前フィードバック（正式レビュー前）により E の対応方針を「ドキュメント修正のみ」から「不確実ケースの実行中止（内部 Critical 化）＋ドキュメント修正」へ変更（F-005 改訂、AC-40〜AC-43 追加）。同日: アーキテクト観点のサブエージェントレビュー指摘（Critical 1・Major 8・Minor 7）を反映（F-005 根拠事実の訂正、F-011 追加、AC-44〜AC-51 追加ほか）。2026-06-14: F-011 を採用決定（サブコマンド条件付き評価）、AC-22 / AC-49 の条件文を一本化。同日: セキュリティ観点のレビュー指摘（High 3・Medium 4・Low 1）を反映 — 解析無効時の既定拒否＋オプトイン化（AC-51 拡張）、読み取り専用サブコマンドの Medium 下限（F-011/AC-49 改訂）、シンボリックリンク解決失敗の安全側化（F-012 / AC-54・AC-55 新設）、監査ログの相関フィールド・reason code・引数マスキング（AC-56・AC-57 新設、AC-12 改訂）、危険引数パターンの実行時評価必須化（F-008/AC-47 強化）、dry-run unknown 分類と終了コード・CI オプション（AC-46 改訂・AC-58 新設）、用語「リスク昇格（許可可能）」へ変更 |

## 1. 背景と目的

### 背景

PR #724（コマンドリスク判定の技術ドキュメント追加）のレビューを契機に、ドキュメントと実装の差異を徹底的に調査した結果、`runner` の **実行時リスク判定（`risk.StandardEvaluator.EvaluateRisk`）** とその周辺に複数の不具合・仕様不整合が確認された。これらの多くは「実行時に算出される実効リスクが、設計意図より低くなる」「ドキュメントが存在しない保証を約束する」方向の問題を含み、セキュリティ上の影響がある。

確認された事象（A〜F は PR #724 のレビューコメント、G〜J は本調査での追加発見）:

| ID | 概要 | 種別 |
|----|------|------|
| A | `EvaluateRisk` がコマンドリスクプロファイルの全要因（`BaseRiskLevel` = 各要因の最大値）を実行時に反映しない。プロファイルは `PrivilegeRisk`（特権昇格判定経由）と `NetworkType` しか実行時に参照されず、`DataExfilRisk` / `SystemModRisk` 等の宣言済み要因が実効リスクに寄与しない。例: `claude`（`DataExfilRisk=High`）は実行時 Medium。`systemctl`（`SystemModRisk=High`）は D との複合により実行時（絶対パス解決後）**Low**（basename 入力なら `IsSystemModification` により Medium）。「従来値」は A 単独でなく D との組み合わせで決まる点に注意。 | コード不具合 |
| B | リスク判定結果を記録する監査ログ（`audit.Logger.LogRiskProfile`）が本番コードパスから一度も呼び出されていない。`audit.Logger` 自体は `runner.go` で生成され特権昇格監査には使われているが、`LogRiskProfile` はデッドコード。ドキュメントは `command_risk_profile` 監査エントリが出力されると説明しているが、実際には出力されない。 | コード不具合 |
| C | `risk_level` はコマンドレベル（およびテンプレートレベルのフォールバック）でのみ設定可能で、グループレベル・グローバルレベルでは設定できない。ドキュメントおよびコードコメント（`CommandTemplate.RiskLevel` / `CommandSpec.RiskLevel` の `nil: inherit from global default` / `nil=inherit default`）は、存在しない継承を示唆している。 | ドキュメント/コメント不整合 |
| D | 実行時、`cmd.ExpandedCmd` は絶対パスに解決済み（`group_executor.go` でのパス解決後）であるにもかかわらず、`IsDestructiveFileOperation` / `IsSystemModification` はコマンド名のリテラル一致（`rm`, `systemctl` 等）で判定する。このため `/usr/bin/rm` 等の絶対パスは破壊的操作・システム変更として検出されず、（coreutils ディレクトリ配下でない限り）実効リスクが Low に落ち得る。既存テストは basename 入力のみで実行時条件（絶対パス）を検証していない。 | コード不具合（セキュリティ） |
| E | バイナリ解析の不確実ケース（解析レコード欠落、ハッシュ/スキーマ不一致など）は実効リスク **High** を返すが、エラーは返さないため `risk_level = "high"` の設定下では実行され得る。これらは「バイナリの素性を解析で確認できなかった」状態であり、利用者が意識的にリスクを受容できる「解析で危険な性質が検出された」状態（dlopen 検出等の High）とは質が異なる。レビュー判断により、不確実ケースは **設定によらず実行を中止すべき** ものとし、コード修正の対象とする（F-005）。なお `contentHash` 空 → High の分岐（`analyzeBinarySignals`）は、本番配線では **実質的に到達不能な防御コード** であることを確認済み: `RecordStore` は file validator そのもの（`GetAnalysisDeps`）であり、validator 無効時は RecordStore も nil となって contentHash チェックより **前に** `(false, false)` = フェイルオープン（Low 側）で返る。validator 有効時は検証済みコマンドに必ずハッシュが付与され、検証失敗はリスク判定以前に致命的エラーとなる。つまり現状の「解析無効時のフェイルオープン」という未分類状態こそが要対処であり、F-005 で明示的に分類する。あわせて、開発者向けドキュメントの「フェイルクローズド（実行しない）」という一括りの記述を新しい実装の区別に合わせて改訂する。 | コード不具合（セキュリティ）+ ドキュメント不整合 |
| F | dry-runパスはパス解決失敗・解析エラー時にエラーを返す（High として表示継続しない）。開発者向けドキュメントの表「エラー時の挙動: フェイルセーフ（High として表示継続）」は過度の一般化。 | ドキュメント不整合 |
| G | `ParseRiskLevel` が設定値 `"unknown"` を **エラーなく受理** し `RiskLevelUnknown`(0) を返す。これは同関数自身のエラーメッセージ（`supported: low, medium, high`）、`spec.go` のコメント、および各ドキュメントの記述（設定可能値は low/medium/high）と矛盾する。`risk_level = "unknown"` を設定すると最大許容リスクが 0 となり、実効リスクは（エラー時を除き）常に Low(1) 以上であるため **全コマンドが実行拒否** される。`critical` は明示的に拒否されるのに `unknown` は黙って受理される非対称も含め、意図された仕様とは考えにくい。 | コード不具合 |
| H | 同一コマンドのリスク定義が複数機構に **重複・矛盾して** 存在する。例: `systemctl` はプロファイル（`SystemModRisk=High`）と `IsSystemModification`（Medium）の両方に定義され、dry-runは High、実行時（basename 入力時）は Medium と分裂する。`rm` はプロファイル / `IsDestructiveFileOperation` / `dangerousCommandPatterns`（`rm -rf`）/ `destructiveCoreutilsCommands` の 4 箇所に定義がある。A の修正（プロファイル反映）の際、これらの定義間の優先順位と整合性を明確にしないと、修正自体が新たな不整合を生む。 | 設計負債（A 修正の前提） |
| I | dry-runパスに実行時との乖離がある: (1) `DryRunResourceManager` は `RiskEvaluator` を受け取らず、バイナリ解析シグナル（ネットワーク/動的ロード/exec/mprotect）がdry-runのリスク表示に反映されない。(2) dry-runは `risk_level` との比較を行わず、「実行時に拒否されるか」を表示できない。(3) 例: `apt-get install`（プロファイル/パターン非該当）は、F-002 修正後の実行時 Medium に対しdry-runはディレクトリ既定の Low となり乖離が顕在化する（現状の実行時は D によりバイナリ解析結果に依存）。ユーザー向け文書 §4 は「dry-run で実際の算出リスクを確認」と案内しており、誤解を招く。 | コード不具合 + ドキュメント不整合 |
| J | ユーザー向け文書 `docs/user/risk_assessment.ja.md` / `.md` に実装との差異がある: (a) §3.1 に `dpkg` が列挙されているがコード上どこにも検出ロジックがない。(b) §1 の「最終リスクはすべての因子の最大値」は、実行時の早期リターン方式・プロファイル非反映（A）と一致しない。(c) コマンド名ベースの常時ネットワーク判定（`curl`/`wget`/`ssh` に加え `bash`/`python`/`node` 等のシェル・インタプリタが名前だけで Medium になる）が記載されておらず、利用者が `risk_level = "low"` のシェルスクリプト実行で拒否される理由を文書から知れない。(d) coreutils 単一バイナリ分類（タスク 0135）が反映されていない。(e) §3.1 の「`systemctl` → medium」はプロファイル定義（High）ともdry-run表示とも一致しない。 | ドキュメント不整合 |

### 目的

1. 実行時リスク判定が、設計意図どおりに **安全側（fail-safe / fail-closed）** で動作することを保証する（A, D, E, G）。バイナリの素性を確認できない不確実ケースは、設定によらず実行を中止する。
2. コマンドごとのリスク定義の **一貫性（単一の真実の源泉）** を確立し、実行時・dry-run・ドキュメントの分裂をなくす（H）。
3. リスク判定結果の **監査可能性** を確保する（B）。
4. dry-runが実行時の判定（実効リスクと実行可否）を正しく予告できるようにする（I）。
5. 開発者向け・ユーザー向けドキュメントおよびコードコメントを実装の実態と一致させる（C, E, F, J）。

### スコープ外

- グループレベル / グローバルレベルの `risk_level` 継承の **新規追加**（C はあくまで現状スコープの正確な記述に留める）。
- リスクレベルの段階定義（Unknown/Low/Medium/High/Critical）の変更。
- バイナリ静的解析アルゴリズム自体の変更。
- root 実行コマンド向けの危険判定（`IsDangerousRootCommand` 等）の変更。

### 前提・依存

- AC-15 / AC-17 / AC-18 が改訂対象とする `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `.md` は **PR #724（未マージ）にのみ存在** する。本タスクのドキュメント改訂作業は PR #724 のマージ後に行うか、マージ順序を実装計画で調整する。

## 2. 機能要件

### F-001: 実行時リスク判定がコマンドリスクプロファイルの全要因を反映する（A）

`EvaluateRisk` は、コマンドに対応するリスクプロファイルが存在する場合、その `BaseRiskLevel()`（全リスク要因の最大値）を実効リスクに反映しなければならない。プロファイルに宣言された任意のリスク要因（`PrivilegeRisk` / `NetworkRisk` / `DestructionRisk` / `DataExfilRisk` / `SystemModRisk`）が、実行時の実効リスクに正しく寄与する。

リスクの算出は **安全側へのみ作用** する。すなわちプロファイル反映によって実効リスクが下がることはなく、既存の各ステップ判定結果とプロファイルの `BaseRiskLevel()` の **最大値** を採用する。

**Acceptance Criteria**:
- **AC-01**: `claude`（`DataExfilRisk=High` / `NetworkRisk=High` を宣言）の実行時実効リスクが **High** になる（従来は Medium）。
- **AC-02**: プロファイルで宣言された各リスク要因の最大値が実効リスクの下限となる（例: 任意の要因が High を宣言するコマンドは実効リスクが High 以上）。
- **AC-03**: プロファイル反映は安全側へのみ作用し、プロファイルが存在しても他ステップ（特権昇格→Critical 等）の結果より低い値に上書きされない。
- **AC-04**: **F-001 のプロファイル反映ステップ単体では**、プロファイルが存在しないコマンドの実効リスクを変化させない（F-002/F-005/F-007 等の他要件による変化はこの AC の対象外）。
- **AC-05**: コマンド名はシンボリックリンクを解決して照合する（既存の `extractAllCommandNames` と一貫した方式）。リンク連鎖上の複数の名前が異なるプロファイルに一致した場合は、一致した全プロファイルの `BaseRiskLevel()` の **最大値** を採用する。シンボリックリンク解決失敗時の安全側動作は F-012 を参照。
- **AC-22**: `systemctl` / `service`（`SystemModRisk=High` を宣言）の **変更系サブコマンド**（F-011 参照）の実行時実効リスクが **High** になる（従来は実行時（絶対パス解決後）Low、basename 入力時 Medium）。この振る舞い変更は意図的なものとして移行ノート（AC-19）に記載される。読み取り専用サブコマンドの扱いは F-011 / AC-49 を参照。

### F-002: 名前ベースの判定が解決済み絶対パスに対しても機能する（D）

`IsDestructiveFileOperation` および `IsSystemModification` は、実行時に渡される **解決済み絶対パス**（例: `/usr/bin/rm`）に対しても、対象コマンドを正しく検出しなければならない。判定はコマンド名（basename）に対して行い、シンボリックリンクの解決を考慮する。

**Acceptance Criteria**:
- **AC-06**: `IsDestructiveFileOperation("/usr/bin/rm", ...)` が `true` を返す（絶対パスでも破壊的操作として検出）。
- **AC-07**: `IsSystemModification("/usr/sbin/systemctl", ["restart", ...])` が `true` を返す（絶対パスでもシステム変更として検出）。
- **AC-08**: `EvaluateRisk` において、絶対パスで指定された `/usr/bin/rm -rf /tmp/x` の実効リスクが **High** になる（従来は Low）。
- **AC-09**: 部分一致による誤検出が起きない（例: `/usr/bin/lsrm` や `/usr/bin/systemctl-helper` のような basename は `rm` / `systemctl` と一致しない）。
- **AC-10**: 既存の basename 入力（`rm`, `systemctl` 等）に対する判定結果は従来どおり維持される（後方互換）。
- **AC-23**: `EvaluateRisk` のテストは、実行時の実態（`ExpandedCmd` が解決済み絶対パスであること）を反映した絶対パス入力のケースを含む。
- **AC-44**: プロファイルを持たないコマンド（例: `rmdir`, `shred`, `mount`, `crontab`）の絶対パス入力について、`EvaluateRisk` が F-002 修正によって正しいリスク（破壊的→High、システム変更→Medium）を返すことをテストで検証する（F-001 のプロファイル反映では救えない、F-002 固有の効果の分離検証）。

### F-012: シンボリックリンク解決の信頼境界を安全側に固める（A/D 修正の随伴）

リスク判定はコマンド名の照合のためシンボリックリンク連鎖を解決する（F-001 AC-05 / F-002）。現状の `extractAllCommandNames` は、`os.Lstat` 失敗・`os.Readlink` 失敗時に黙って走査を打ち切り、**それまでに収集した部分的な名前だけで評価を続行** する（解決失敗が `exceededDepth` フラグにも反映されない）。これは「危険な実体名を観測できないまま Low 側に落ちる」抜け穴になり得る。

シンボリックリンク解決に関わる以下の異常時は、部分結果で続行せず **安全側（実行中止: エラー中止または内部 Critical 化）** としなければならない。「解決できた名前だけで評価して続行」することを禁止する。

- `os.Lstat` 失敗（権限不足・TOCTOU でのファイル消失等）
- `os.Readlink` 失敗
- シンボリックリンクの循環
- 深度超過（既存の `ErrSymlinkDepthExceeded` 相当）
- 最終ターゲットの解決不能

なお、実行時パスでは `group_executor` の `ResolvePath` で正常系のパス解決が先行する点を踏まえ、本要件の対象は「リスク判定内のリンク走査」における異常検知と安全側動作とする。正常系で追加のリンクが無いケース（解決済み絶対パスが通常ファイル）は従来どおり正常に評価する。

**Acceptance Criteria**:
- **AC-54**: シンボリックリンク解決の失敗（stat 不能・readlink 失敗・循環・深度超過・解決不能）時、リスク判定は部分的に解決した名前だけで評価を続行せず、安全側（エラー中止または内部 Critical 化）とする。
- **AC-55**: 上記の解決失敗時に実効リスクが Low 側に落ちないことをテストで検証する（深度超過だけでなく stat/readlink 失敗ケースを含む）。

### F-003: リスク判定結果が監査ログに記録される（B）

実行時にコマンドのリスク判定を行った際、その結果が監査ログ（`LogRiskProfile` 相当の `command_risk_profile` エントリ）として出力されなければならない。インシデント調査に耐えるよう、「どのバイナリ・どの設定・どの解析レコードで、許可/拒否のいずれを下したか」を後から相関できる情報を含める。

**実現上の前提（アーキテクチャ設計への入力）**:
- 現行の `Evaluator.EvaluateRisk` は `(RiskLevel, error)` のみを返し、判定根拠（どのステップ・どの要因で決まったか）を返さない。プロファイル由来でないコマンド（破壊的操作・バイナリ解析・coreutils 分類等で決まったもの）について判定根拠・**機械可読な reason code** を監査に残すには、評価器の戻り値の拡張が必要である。
- `NormalResourceManager` は現在 `audit.Logger` を保持しておらず、配線の追加が必要である。
- `decision`（allow/deny）・`max_allowed_risk` の記録は、リスク比較を行う `NormalResourceManager`（および dry-run 側）でのみ可能であり、評価器単体では完結しない。

**Acceptance Criteria**:
- **AC-11**: 通常実行パスでコマンドのリスク判定が行われると、`audit_type=command_risk_profile` の監査ログエントリが 1 件出力される。
- **AC-12**: 出力エントリには、評価されたリスクレベルと判定根拠が含まれる。判定根拠は **機械可読な reason code**（例: `destructive_file_operation` / `binary_analysis_dynamic_load` / `coreutils_classification` / `profile_data_exfil` / `uncertain_missing_record`）として出力され、プロファイル由来のコマンドでは加えて人間可読なリスク要因の理由（`risk_factors`）を含む。
- **AC-13**: ログレベルがリスクレベルに対応する（Critical→Error, High→Warn, Medium→Info, それ以外→Debug）。
- **AC-48**: プロファイルを持たないコマンド（例: バイナリ解析で Medium となった未知コマンド）についても、AC-12 の判定根拠（reason code）が出力される。
- **AC-14**: リスク超過で実行拒否されたコマンドについても、リスク判定結果が監査可能である（拒否ログまたはリスクプロファイルログのいずれかで追跡できる）。
- **AC-56**: 監査ログのリスク判定エントリは、相関に必要な以下のフィールドを含む: `resolved_path`、`content_hash`、解析レコードの識別情報（レコードの hash または schema version。レコード非使用時はその旨）、`max_allowed_risk`、`decision`（allow/deny）、`reason_codes`。
- **AC-57**: コマンド引数を監査ログに出力する場合、機密情報の漏えいを防ぐマスキング方針（既存の redaction 機構との整合を含む）が定義され、出力に適用される。

### F-004: `risk_level` の設定スコープを実態どおりに記述する（C）

`risk_level` の設定可能スコープ（コマンドレベル＋テンプレートフォールバック、グループ/グローバル非対応、未指定時は `low`）について、ドキュメントおよびコードコメントを実態と一致させる。存在しない「グローバルデフォルトからの継承」を示唆する記述を排除する。

**Acceptance Criteria**:
- **AC-15**: コマンドリスク判定ドキュメント（`command-risk-evaluation.ja.md` / `.md`）が、`risk_level` はコマンドレベル（テンプレートフォールバックあり）でのみ設定可能で、グループ/グローバルレベルでは設定できないことを明記する。
- **AC-16**: 誤解を招くコードコメント（`spec.go` の `CommandTemplate.RiskLevel` の `nil: inherit from global default`、`CommandSpec.RiskLevel` の `nil=inherit default`）が、実態（`nil` の場合はテンプレートフォールバックの後、最終的に `low`）に即した記述へ修正される。

### F-005: バイナリ解析の不確実ケースでは実行を中止する（E）

バイナリ解析の **不確実ケース**（バイナリの素性・解析結果の信頼性を確認できない状態）では、`risk_level` の設定値によらずコマンドの実行を中止しなければならない。実現機構としては **内部的に Critical 扱いに昇格** させる方式を第一候補とする（Critical は設定不可のため、統一されたリスク比較パス `effectiveRisk > maxAllowedRisk` で必ず拒否され、拒否ログ・監査の対象にもなる）。エラー返却方式との比較・選定はアーキテクチャ設計で行う。

**実現上の前提（アーキテクチャ設計への入力）**:
- 現行のシグナル伝搬 `analyzeBinarySignals` → `IsNetworkOperation` → `EvaluateRisk` は `(isNetwork, isHighRisk bool)` の 2 値に **「危険検出」（dlopen 等、High 維持対象）と「不確実」（レコード欠落等、Critical 化対象）を合流** させており、情報が失われている。両者を区別可能な伝搬方式（3 値以上の分類またはエラー型）への変更が本要件の前提となる。

**不確実ケース（実行中止の対象）** — 前提条件: `RecordStore` が利用可能（バイナリ解析が有効）であること:
- `contentHash` が空（バイナリの同一性が未検証）。注: 本番配線では validator 無効時に `RecordStore` も nil となるため、現状この分岐は実質到達不能の防御コードである（背景 E 参照）。防御として分類は維持する。
- 解析レコードが見つからない（ハッシュ検証済みバイナリにレコードがない）
- 解析レコードのスキーマバージョン不一致
- 解析レコードの content hash がディスク上のバイナリと不一致（解析 DB の陳腐化）
- 解析処理の失敗（`AnalysisError`。記録時と実行時のバイナリ不一致 `ErrSyscallHashMismatch` を含む）・未知の解析結果値

**解析無効時（`RecordStore` が nil）の扱い**:
- セキュリティ境界として見ると、解析無効は「危険シグナルを観測できない状態」であり、黙ったフェイルオープン（Low 側通過）は不適切である。ログ出力を足すだけでは不十分とする。
- **既定は安全側（拒否）** とする。解析無効のままコマンドを実行することを許可するには、`risk_level` とは **独立した明示設定**（例: `allow_unanalyzed_binaries = true` 相当のオプトイン）を必須とする。`risk_level = "high"` による許可とは混同させない（High 許可と「解析不能まで受容」を分離する）。
- いずれの場合も、解析無効で実行している旨をログ・監査で明示する。
- オプトイン設定の具体的な配置（グローバル/グループ/コマンドのどのレベルか）・名称・既定値の表現は、アーキテクチャ設計で確定する。既定が安全側（拒否）であること自体は本要件で確定する。

**High のまま維持するケース（`risk_level = "high"` で意識的に許可可能）**:
- 有効な解析レコードから検出された危険な性質: 動的ロードシンボル（`dlopen`/`dlsym`/`dlvsym`）、exec シスコール、未解決の `svc #0x80` 直接シスコール、mprotect 系 `PROT_EXEC`（確定または不明）
- これらは「解析が成功し、バイナリの性質として確認・推定された」ものであり、利用者がリスクを受容して許可する運用（ログメッセージが案内する `set risk_level = "high" or higher to allow execution`）を維持する

**Acceptance Criteria**:
- **AC-40**: 上記の不確実ケース各々について（前提条件下で）、`risk_level = "high"` を設定していてもコマンドが実行されない。
- **AC-41**: 不確実ケースによる実行中止は、拒否理由（どの不確実ケースか）とともにログ・監査で追跡できる。
- **AC-42**: 有効な解析レコードから検出された危険な性質（動的ロード・exec・svc・mprotect）は従来どおり High であり、`risk_level = "high"` で実行できる。
- **AC-43**: dry-runも同じ分類を表示する（不確実ケースは実行拒否予告。既存の「ハッシュ検証失敗 → Critical」表示と整合する）。dry-run 固有の制約は AC-46 を参照。
- **AC-51**: `RecordStore` が nil（バイナリ解析無効）の構成では、(a) 既定でコマンド実行が拒否される（安全側既定）、(b) 実行を許可するには `risk_level` とは独立した明示オプトイン設定が必要、(c) その設定の既定値が「拒否」側である、(d) 解析無効で実行/拒否した旨がログ・監査で明示される。High 許可（`risk_level = "high"`）だけでは解析無効状態の許可にはならない。
- **AC-45**: バイナリ解析の結果分類の **全列挙値**（ネットワーク検出 / シンボルなし / 非対応形式 `NotSupportedBinary` / 静的バイナリ `StaticBinary` / 解析エラー / 未知の値）について、「不確実（中止）/ 危険検出（High）/ 安全側通過」のいずれに属するかが設計文書に明記され、テストで検証される。`StaticBinary` / `NotSupportedBinary` はシンボル解析が適用できないだけであり、シスコール解析等の他シグナルが有効レコードに存在する場合はそれに基づき判定する。他シグナルも得られない場合の扱い（安全側通過の容認可否）は設計文書で根拠とともに決定する。default 分岐（未知の値）は不確実（中止）側とする。
- **AC-17**: 開発者向けドキュメントが、(a) エラーで実行中止するケース（シンボリックリンク深度超過・coreutils stat エラー・`AnalysisError` 以外の予期しないレコード読込エラー）、(b) 不確実ケースとして設定によらず実行中止するケース（本要件）、(c) High として許可設定下で実行可能なケース、(d) 解析無効時の挙動、を明確に区別して記述する。ユーザー向け `risk_assessment.ja.md` §3.3 の表も新しい挙動に合わせて改訂する。

### F-006: dry-runのエラー時挙動を正確に記述する（F）

dry-runパスはパス解決失敗・解析エラー時にエラーを返す（High 表示で継続しない）。一方、`AnalyzeCommandSecurity` 内部の特定チェックは失敗を High/Critical に変換する。この区別をドキュメントに反映する。

**Acceptance Criteria**:
- **AC-18**: ドキュメントの実行時パス/dry-runパス比較表が、dry-runはパス解決・解析の失敗時にエラーを返すこと、High/Critical への変換は `AnalyzeCommandSecurity` 内の特定チェックに限られることを正確に記述する。

### F-007: 設定値 `"unknown"` を拒否する（G）

`ParseRiskLevel` は、設定値 `"unknown"` に対して `"critical"` と同様にエラーを返さなければならない。`risk_level` として受理される値は `"low"` / `"medium"` / `"high"`（および省略・空文字 = `low`）のみとする。

**Acceptance Criteria**:
- **AC-24**: `ParseRiskLevel("unknown")` がエラーを返す（`ErrInvalidRiskLevel` 系）。
- **AC-25**: `risk_level = "unknown"` を含む TOML 設定は、設定ロード時または実行前検証でエラーとなり、その旨が利用者に通知される。
- **AC-26**: `"low"` / `"medium"` / `"high"` / 省略 / 空文字の挙動は従来どおり維持される。

### F-008: コマンドごとのリスク定義の一貫性を確立する（H）

同一コマンドに対するリスク定義（コマンドリスクプロファイル / `IsDestructiveFileOperation` / `IsSystemModification` / `dangerousCommandPatterns` / coreutils 分類）の間で、定義の重複に起因する判定の分裂をなくす。アーキテクチャ設計で「単一の真実の源泉」または「明示的な優先順位」を定め、実行時とdry-runが同一コマンド・同一引数に対して矛盾したリスクを返さないようにする。

**実行時の引数評価は必須**: 実行可否を決める評価器（`EvaluateRisk`）は、コマンド名だけでなく **引数パターンも必ず評価** しなければならない。`dangerousCommandPatterns`（`rm -rf`、`dd if=`、`chmod 777` 等）は dry-run 専用の参考情報ではなく、実効リスク算出の入力に統合する。これにより「dry-run だけが危険な引数を検出し、実行時は見逃す」状態の再発を防ぐ。

**Acceptance Criteria**:
- **AC-27**: `systemctl`（引数任意）について、実行時（`EvaluateRisk`）とdry-run（`AnalyzeCommandSecurity`）が同じリスクレベルを返す。
- **AC-28**: `rm`（破壊的操作の代表）について、basename 入力・絶対パス入力・coreutils 配下のいずれでも、実行時とdry-runで High が返る。
- **AC-29**: 複数機構に定義が重複するコマンドの優先順位（どの定義がどのパスで効くか）が、設計文書および開発者向けドキュメントに明記される。
- **AC-47**: `dangerousCommandPatterns` にのみ定義があるコマンド・引数の組について、実行時（`EvaluateRisk`）とdry-runの実効リスクが分裂しないよう定義が統合され、テストで検証される。少なくとも以下の具体ケースを含む: `chmod -R 777 /`（High）、`chown -R root /tmp/x`（Medium 以上）、`mkfs.ext4 /dev/sdX`（High）、`dd if=...`（High）。いずれも **実行時の `EvaluateRisk` が引数を評価して** 該当リスクを返すこと（現状は dry-run のみパターンが効き、実行時は `IsSystemModification` の Medium またはバイナリ解析に落ちる）。

### F-009: dry-runが実行時の判定を予告できる（I）

dry-runは、実行時と同じ評価器（`EvaluateRisk`）による実効リスクと、`risk_level` 設定との比較結果（実行時に許可されるか拒否されるか）を表示しなければならない。既存の `AnalyzeCommandSecurity` による詳細表示（ディレクトリ既定リスク・ハッシュ検証・setuid・危険パターン等）は維持してよいが、実行可否を決めるのは実効リスク表示の側であることを出力上明確にする。

**dry-run 固有の制約**: dry-run はハッシュディレクトリが利用不能でも続行する設計（警告のみで検証スキップ）があり、その場合 `contentHash` が得られず、実行時と同一の判定を再現できない。このとき F-005 の不確実ケース（Critical/拒否予告）と誤って断定してはならない。

**Acceptance Criteria**:
- **AC-30**: dry-run出力に、実行時パスと同一ロジックで算出された実効リスクが含まれる（ハッシュ・解析レコードが利用可能な場合）。
- **AC-31**: dry-run出力に、`risk_level` 設定との比較結果（許可/拒否の予告）が含まれる。
- **AC-32**: バイナリ解析シグナル（ネットワーク/動的ロード/exec/mprotect）による High/Medium が、dry-runの実効リスク表示にも反映される（解析レコードが利用可能な場合）。
- **AC-33**: dry-runは引き続きコマンドを実行せず、判定不能・解析エラー時の挙動（エラー返却）は AC-18 の記述と一致する。
- **AC-46**: ハッシュディレクトリ利用不能などで dry-run が `contentHash` を取得できない場合、不確実ケース（Critical/拒否予告）と **誤断定せず**、`unknown`（判定不能: ハッシュ未検証のため実行時に再評価）として、allow/deny とは区別された分類で表示する。
- **AC-58**: dry-run の `unknown`（判定不能）は、出力上 allow/deny と明確に区別され、終了コードが通常成功と区別される（または区別するオプションがある）。dry-run 出力には「dry-run は本番実行時の許可を保証しない（本番では再評価され拒否され得る）」旨を含める。CI 利用向けに、`unknown` を失敗（非ゼロ終了）として扱うオプションを提供する。

### F-010: ユーザー向けリスク評価ガイドを実装と一致させる（J）

`docs/user/risk_assessment.ja.md` / `.md` を、本タスクの修正後の実装に合わせて改訂する。

**Acceptance Criteria**:
- **AC-34**: §3.1 から `dpkg` を削除する（または実装に `dpkg` 検出を追加する場合はその旨を要件に追記して整合させる。既定は削除）。
- **AC-35**: コマンド名ベースの常時ネットワーク判定（`curl`/`wget`/`ssh` 等に加え、シェル・スクリプトインタプリタ `bash`/`python`/`node` 等が名前のみで `medium` になること）が利用者向けに説明される。
- **AC-36**: coreutils 単一バイナリ分類（Low/Medium/High の 3 区分）が説明される。
- **AC-37**: `systemctl` 等のリスクレベル記載（§2 のレベル定義表「システム変更 = medium」を含む）が、F-001/F-008/F-011 で確立した定義と一致する。
- **AC-38**: 「最終リスクはすべての因子の最大値」の記述が、F-001 修正後の実装（プロファイル要因を含む最大値）と整合する。
- **AC-50**: §5 の設定例（現状 `systemctl status` + `risk_level = "low"` 等）が、修正後の実装で **実際に動作する** 設定例に改訂される（恒久的に拒否される設定例を残さない）。

### F-011: 読み取り専用サブコマンドの過剰な High 化を避ける（A 修正の随伴）

F-001 により `systemctl` / `service` プロファイルの `SystemModRisk=High` が実行時に反映されると、読み取り専用の典型用途（`systemctl status` / `is-active` / `list-units` 等による監視）まで一律 High となり、利用者は監視コマンドに `risk_level = "high"` を設定せざるを得なくなる。これは最小権限の原則（ユーザー向け文書 §5 が掲げる「実際の動作に必要な最低限のリスクレベルを設定する」）を利用者自身が損なう誘因（リスク補償）になる。

これを避けるため、`SystemModRisk` に **引数（サブコマンド）条件付き評価** を導入する。既存の `NetworkSubcommands`（`ConditionalNetwork`）と対称な機構とするが、読み取り専用を「昇格なし（Low 落ち）」にはせず **Medium の下限** を持たせる点が異なる（`systemctl show` 等はユニット設定・パス・ユーザー名等の情報露出になり得るため、Low は危険）。

- 変更系サブコマンド（`start` / `stop` / `restart` / `enable` / `disable` / `mask` 等）→ High
- 読み取り専用サブコマンド（`status` / `show` / `cat` / `is-active` / `list-units` / `list-unit-files` / `list-timers` / `list-dependencies` 等）→ **Medium 下限**（プロファイルが Medium を保証し、Low には落とさない。他ステップがより高いリスクを返す場合はその最大値）
- **未知のサブコマンド・サブコマンド判別不能 → High（安全側既定）**

> 情報露出量の多い読み取り系（`show` / `cat` / `list-unit-files` / `list-timers` / `list-dependencies` 等）を Medium とすることを明記する。読み取り専用/変更系の **網羅的な分類リスト** とその管理方針（新規サブコマンド追加時の既定 = High）はアーキテクチャ設計で確定する。

> ✅ **決定事項（2026-06-14）**: 条件付き評価の導入を採用する（無条件 High 案は不採用）。代替案として「無条件 High を受け入れ、ユーザー向け文書の設定例を `risk_level = "high"` に改訂する」案も検討したが、最小権限の原則を損なわないため条件付き評価を選択した。読み取り専用は Low ではなく Medium 下限とする（情報露出対策）。

**Acceptance Criteria**:
- **AC-49**: `systemctl status <unit>` / `systemctl show <unit>` の実行時実効リスクが High にならず、かつ **Low にも落ちず Medium 以上** になる。`systemctl restart <unit>` および未知サブコマンドは High になる。

## 3. 非機能要件

### NF-001: 後方互換性

- 既存の TOML 設定の解釈を破壊しない。ただし F-001/F-002/F-005/F-007 により、一部設定の挙動が変わる:
  - `claude` 等（実効リスク Medium→High）、`systemctl`/`service` の **変更系サブコマンド**（実行時 Low→High。読み取り専用サブコマンドは F-011 により据え置き）、絶対パス指定の破壊的コマンド（Low→High）は、`risk_level` 設定によっては従来実行できていたものが拒否される。これは **意図的な安全性向上** である。
  - バイナリ解析の不確実ケースは、従来 `risk_level = "high"` で実行できていたものが **設定によらず拒否** される。解消手段はケースごとに異なる: 解析レコード欠落・スキーマ不一致・content hash 不一致 → `record` の再実行による解析レコードの整備、`contentHash` 空 → file validator の有効化（防御的分類であり、本番配線では通常発生しない。背景 E 参照）。
  - `risk_level = "unknown"` は設定エラーになる（従来は全コマンド拒否として黙って動作）。
- これらの変更点を移行ノートとしてドキュメントに記載する。

**Acceptance Criteria**:
- **AC-19**: 実効リスク上昇により拒否され得る代表的ケース（`claude` を `risk_level=medium` で実行、絶対パス `rm` を `risk_level=low` で実行、`systemctl` を `risk_level=low` または `medium` で実行）、不確実ケースの設定によらない拒否（解消手段をケース別に記載）、および `"unknown"` の設定エラー化が、移行ノートまたは変更点としてドキュメントに記載される。

### NF-002: 実行時パスとdry-runパスの整合性

- 同一コマンドに対し、実行時（`EvaluateRisk`）と **dry-run に新設する実効リスク表示（F-009）** が一致する。dry-run の既存詳細表示（`AnalyzeCommandSecurity`）との関係は F-009 本文のとおり（実行可否を決めるのは実効リスク側）であり、両表示の値が異なる場合の意味（詳細表示は参考情報）を出力またはドキュメントで明確にする。

**Acceptance Criteria**:
- **AC-20**: 既存の整合性テスト（`coreutils_consistency_test.go`）が修正後も成立し、破壊的コマンド（絶対パス含む）について両パスが同じ High を返すことを検証する。
- **AC-39**: プロファイル定義コマンドの代表例（`claude`, `systemctl`, `curl`）について、実行時とdry-runの実効リスクが一致することをテストで検証する。

### NF-003: 品質ゲート

- `make fmt` / `make test` / `make lint` がすべて成功する。

**Acceptance Criteria**:
- **AC-21**: 変更後、`make test` と `make lint` がパスする（既存の無関係な lint 指摘を除く）。

## 4. 用語

| 用語 | 定義 |
|------|------|
| 実効リスク（effective risk） | `EvaluateRisk` がコマンド内容から算出する実際のリスクレベル |
| 最大許容リスク（maximum allowed risk） | `risk_level` 設定による実行可否の上限 |
| コマンドリスクプロファイル | `CommandRiskProfile`。複数のリスク要因を保持し、`BaseRiskLevel()` で最大値を返す |
| エラー中止 | エラーを返してコマンド実行を中止する挙動（シンボリックリンク深度超過、予期しないレコード読込エラー等） |
| 無条件拒否（Critical 化） | 実効リスクを内部 Critical に昇格させ、`risk_level` 設定によらずリスク比較で拒否する挙動（F-005 の不確実ケース） |
| リスク昇格（許可可能） | **危険シグナル検出時**に安全側（より高いリスク、典型的には High）へ倒すが、`risk_level = "high"` で意識的に許可可能な挙動。**不確実ケースはこれに含めない**（不確実は「無条件拒否」側）。注: これは厳密には fail-closed ではない（許可可能であるため）。 |
| フェイルクローズド | 上記「エラー中止」「無条件拒否（Critical 化）」の総称（確実に実行させない挙動）。「リスク昇格（許可可能）」は含まない。 |
| 不確実ケース | バイナリの同一性または解析結果の信頼性を確認できない状態（解析レコード欠落、スキーマ/ハッシュ不一致、`contentHash` 空、解析エラー等）。F-005 により設定によらず実行中止の対象 |
| 単一の真実の源泉 | 同一コマンドのリスク定義を一箇所に集約し、各判定パスがそれを参照する設計方針 |

## 5. 参考

- PR #724: コマンドリスク判定ドキュメント追加とそのレビュー
- `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `.md`（開発者向け、本タスクで改訂対象）
- `docs/user/risk_assessment.ja.md` / `.md`（ユーザー向け、本タスクで改訂対象）
- `internal/runner/base/risk/evaluator.go`
- `internal/runner/base/security/command_analysis.go`
- `internal/runner/base/security/command_risk_profile.go`
- `internal/runner/base/security/coreutils.go`
- `internal/runner/base/runnertypes/config.go`（`ParseRiskLevel`）
- `internal/runner/base/runnertypes/spec.go`（`RiskLevel` フィールドコメント）
- `internal/runner/resource/normal_manager.go` / `dryrun_manager.go` / `default_manager.go`
- `internal/runner/base/audit/logger.go`
- `internal/runner/group_executor.go`（パス解決と `ExpandedCmd` の上書き）
