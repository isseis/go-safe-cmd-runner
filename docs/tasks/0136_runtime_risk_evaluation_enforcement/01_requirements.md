# 実行時リスク判定の強化 — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-13 |
| Review date | - |
| Reviewer | - |
| Comments | 2026-06-13: レビューフィードバックにより E の対応方針を「ドキュメント修正のみ」から「不確実ケースの実行中止（内部 Critical 化）＋ドキュメント修正」へ変更（F-005 改訂、AC-40〜AC-43 追加） |

## 1. 背景と目的

### 背景

PR #724（コマンドリスク判定の技術ドキュメント追加）のレビューを契機に、ドキュメントと実装の差異を徹底的に調査した結果、`runner` の **実行時リスク判定（`risk.StandardEvaluator.EvaluateRisk`）** とその周辺に複数の不具合・仕様不整合が確認された。これらの多くは「実行時に算出される実効リスクが、設計意図より低くなる」「ドキュメントが存在しない保証を約束する」方向の問題を含み、セキュリティ上の影響がある。

確認された事象（A〜F は PR #724 のレビューコメント、G〜J は本調査での追加発見）:

| ID | 概要 | 種別 |
|----|------|------|
| A | `EvaluateRisk` がコマンドリスクプロファイルの全要因（`BaseRiskLevel` = 各要因の最大値）を実行時に反映しない。プロファイルは `PrivilegeRisk`（特権昇格判定経由）と `NetworkType` しか実行時に参照されず、`DataExfilRisk` / `SystemModRisk` 等の宣言済み要因が実効リスクに寄与しない。例: `claude`（`DataExfilRisk=High`）は実行時 Medium、`systemctl`（`SystemModRisk=High`）は実行時 Medium となる。 | コード不具合 |
| B | リスク判定結果を記録する監査ログ（`audit.Logger.LogRiskProfile`）が本番コードパスから一度も呼び出されていない。`audit.Logger` 自体は `runner.go` で生成され特権昇格監査には使われているが、`LogRiskProfile` はデッドコード。ドキュメントは `command_risk_profile` 監査エントリが出力されると説明しているが、実際には出力されない。 | コード不具合 |
| C | `risk_level` はコマンドレベル（およびテンプレートレベルのフォールバック）でのみ設定可能で、グループレベル・グローバルレベルでは設定できない。ドキュメントおよびコードコメント（`CommandTemplate.RiskLevel` / `CommandSpec.RiskLevel` の `nil: inherit from global default` / `nil=inherit default`）は、存在しない継承を示唆している。 | ドキュメント/コメント不整合 |
| D | 実行時、`cmd.ExpandedCmd` は絶対パスに解決済み（`group_executor.go` でのパス解決後）であるにもかかわらず、`IsDestructiveFileOperation` / `IsSystemModification` はコマンド名のリテラル一致（`rm`, `systemctl` 等）で判定する。このため `/usr/bin/rm` 等の絶対パスは破壊的操作・システム変更として検出されず、（coreutils ディレクトリ配下でない限り）実効リスクが Low に落ち得る。既存テストは basename 入力のみで実行時条件（絶対パス）を検証していない。 | コード不具合（セキュリティ） |
| E | バイナリ解析の不確実ケース（解析レコード欠落、ハッシュ/スキーマ不一致、`contentHash` 空など）は実効リスク **High** を返すが、エラーは返さないため `risk_level = "high"` の設定下では実行され得る。これらは「バイナリの素性を解析で確認できなかった」状態であり、利用者が意識的にリスクを受容できる「解析で危険な性質が検出された」状態（dlopen 検出等の High）とは質が異なる。レビュー判断により、不確実ケースは **設定によらず実行を中止すべき** ものとし、コード修正の対象とする（F-005）。なお実行時に `contentHash` が空になるのは file validator 無効化時のみであることを確認済み（通常実行の検証済みコマンドには必ずハッシュが付与され、検証失敗はリスク判定以前に致命的エラーとなる）。あわせて、開発者向けドキュメントの「フェイルクローズド（実行しない）」という一括りの記述を新しい実装の区別に合わせて改訂する。 | コード不具合（セキュリティ）+ ドキュメント不整合 |
| F | dry-runパスはパス解決失敗・解析エラー時にエラーを返す（High として表示継続しない）。開発者向けドキュメントの表「エラー時の挙動: フェイルセーフ（High として表示継続）」は過度の一般化。 | ドキュメント不整合 |
| G | `ParseRiskLevel` が設定値 `"unknown"` を **エラーなく受理** し `RiskLevelUnknown`(0) を返す。これは同関数自身のエラーメッセージ（`supported: low, medium, high`）、`spec.go` のコメント、および各ドキュメントの記述（設定可能値は low/medium/high）と矛盾する。`risk_level = "unknown"` を設定すると最大許容リスクが 0 となり、実効リスクは（エラー時を除き）常に Low(1) 以上であるため **全コマンドが実行拒否** される。`critical` は明示的に拒否されるのに `unknown` は黙って受理される非対称も含め、意図された仕様とは考えにくい。 | コード不具合 |
| H | 同一コマンドのリスク定義が複数機構に **重複・矛盾して** 存在する。例: `systemctl` はプロファイル（`SystemModRisk=High`）と `IsSystemModification`（Medium）の両方に定義され、dry-runは High、実行時（basename 入力時）は Medium と分裂する。`rm` はプロファイル / `IsDestructiveFileOperation` / `dangerousCommandPatterns`（`rm -rf`）/ `destructiveCoreutilsCommands` の 4 箇所に定義がある。A の修正（プロファイル反映）の際、これらの定義間の優先順位と整合性を明確にしないと、修正自体が新たな不整合を生む。 | 設計負債（A 修正の前提） |
| I | dry-runパスに実行時との乖離がある: (1) `DryRunResourceManager` は `RiskEvaluator` を受け取らず、バイナリ解析シグナル（ネットワーク/動的ロード/exec/mprotect）がdry-runのリスク表示に反映されない。(2) dry-runは `risk_level` との比較を行わず、「実行時に拒否されるか」を表示できない。(3) 例: `apt-get install`（プロファイル/パターン非該当）は basename 入力の実行時 Medium に対しdry-runはディレクトリ既定の Low。ユーザー向け文書 §4 は「dry-run で実際の算出リスクを確認」と案内しており、誤解を招く。 | コード不具合 + ドキュメント不整合 |
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

## 2. 機能要件

### F-001: 実行時リスク判定がコマンドリスクプロファイルの全要因を反映する（A）

`EvaluateRisk` は、コマンドに対応するリスクプロファイルが存在する場合、その `BaseRiskLevel()`（全リスク要因の最大値）を実効リスクに反映しなければならない。プロファイルに宣言された任意のリスク要因（`PrivilegeRisk` / `NetworkRisk` / `DestructionRisk` / `DataExfilRisk` / `SystemModRisk`）が、実行時の実効リスクに正しく寄与する。

リスクの算出は **安全側へのみ作用** する。すなわちプロファイル反映によって実効リスクが下がることはなく、既存の各ステップ判定結果とプロファイルの `BaseRiskLevel()` の **最大値** を採用する。

**Acceptance Criteria**:
- **AC-01**: `claude`（`DataExfilRisk=High` / `NetworkRisk=High` を宣言）の実行時実効リスクが **High** になる（従来は Medium）。
- **AC-02**: プロファイルで宣言された各リスク要因の最大値が実効リスクの下限となる（例: 任意の要因が High を宣言するコマンドは実効リスクが High 以上）。
- **AC-03**: プロファイル反映は安全側へのみ作用し、プロファイルが存在しても他ステップ（特権昇格→Critical 等）の結果より低い値に上書きされない。
- **AC-04**: プロファイルが存在しないコマンドの実効リスクは、本変更によって従来から変化しない。
- **AC-05**: コマンド名はシンボリックリンクを解決して照合する（既存の `extractAllCommandNames` と一貫した方式）。
- **AC-22**: `systemctl` / `service`（`SystemModRisk=High` を宣言）の実行時実効リスクが **High** になる（従来は Medium）。この振る舞い変更は意図的なものとして移行ノート（AC-19）に記載される。

### F-002: 名前ベースの判定が解決済み絶対パスに対しても機能する（D）

`IsDestructiveFileOperation` および `IsSystemModification` は、実行時に渡される **解決済み絶対パス**（例: `/usr/bin/rm`）に対しても、対象コマンドを正しく検出しなければならない。判定はコマンド名（basename）に対して行い、シンボリックリンクの解決を考慮する。

**Acceptance Criteria**:
- **AC-06**: `IsDestructiveFileOperation("/usr/bin/rm", ...)` が `true` を返す（絶対パスでも破壊的操作として検出）。
- **AC-07**: `IsSystemModification("/usr/sbin/systemctl", ["restart", ...])` が `true` を返す（絶対パスでもシステム変更として検出）。
- **AC-08**: `EvaluateRisk` において、絶対パスで指定された `/usr/bin/rm -rf /tmp/x` の実効リスクが **High** になる（従来は Low）。
- **AC-09**: 部分一致による誤検出が起きない（例: `/usr/bin/lsrm` や `/usr/bin/systemctl-helper` のような basename は `rm` / `systemctl` と一致しない）。
- **AC-10**: 既存の basename 入力（`rm`, `systemctl` 等）に対する判定結果は従来どおり維持される（後方互換）。
- **AC-23**: `EvaluateRisk` のテストは、実行時の実態（`ExpandedCmd` が解決済み絶対パスであること）を反映した絶対パス入力のケースを含む。

### F-003: リスク判定結果が監査ログに記録される（B）

実行時にコマンドのリスク判定を行った際、その結果（リスクレベル・リスク要因・ネットワーク種別）が監査ログ（`LogRiskProfile` 相当の `command_risk_profile` エントリ）として出力されなければならない。

**Acceptance Criteria**:
- **AC-11**: 通常実行パスでコマンドのリスク判定が行われると、`audit_type=command_risk_profile` の監査ログエントリが 1 件出力される。
- **AC-12**: 出力エントリには、評価されたリスクレベルと（存在する場合）リスク要因の理由（`risk_factors`）が含まれる。
- **AC-13**: ログレベルがリスクレベルに対応する（Critical→Error, High→Warn, Medium→Info, それ以外→Debug）。
- **AC-14**: リスク超過で実行拒否されたコマンドについても、リスク判定結果が監査可能である（拒否ログまたはリスクプロファイルログのいずれかで追跡できる）。

### F-004: `risk_level` の設定スコープを実態どおりに記述する（C）

`risk_level` の設定可能スコープ（コマンドレベル＋テンプレートフォールバック、グループ/グローバル非対応、未指定時は `low`）について、ドキュメントおよびコードコメントを実態と一致させる。存在しない「グローバルデフォルトからの継承」を示唆する記述を排除する。

**Acceptance Criteria**:
- **AC-15**: コマンドリスク判定ドキュメント（`command-risk-evaluation.ja.md` / `.md`）が、`risk_level` はコマンドレベル（テンプレートフォールバックあり）でのみ設定可能で、グループ/グローバルレベルでは設定できないことを明記する。
- **AC-16**: 誤解を招くコードコメント（`spec.go` の `CommandTemplate.RiskLevel` の `nil: inherit from global default`、`CommandSpec.RiskLevel` の `nil=inherit default`）が、実態（`nil` の場合はテンプレートフォールバックの後、最終的に `low`）に即した記述へ修正される。

### F-005: バイナリ解析の不確実ケースでは実行を中止する（E）

バイナリ解析の **不確実ケース**（バイナリの素性・解析結果の信頼性を確認できない状態）では、`risk_level` の設定値によらずコマンドの実行を中止しなければならない。実現機構としては **内部的に Critical 扱いに昇格** させる方式を第一候補とする（Critical は設定不可のため、統一されたリスク比較パス `effectiveRisk > maxAllowedRisk` で必ず拒否され、拒否ログ・監査の対象にもなる）。エラー返却方式との比較・選定はアーキテクチャ設計で行う。

**不確実ケース（実行中止の対象）**:
- `contentHash` が空（バイナリの同一性が未検証 = file validator 無効化時のみ発生）
- 解析レコードが見つからない（ハッシュ検証済みバイナリにレコードがない）
- 解析レコードのスキーマバージョン不一致
- 解析レコードの content hash がディスク上のバイナリと不一致（解析 DB の陳腐化）
- 解析エラー・未知の解析結果

**High のまま維持するケース（`risk_level = "high"` で意識的に許可可能）**:
- 有効な解析レコードから検出された危険な性質: 動的ロードシンボル（`dlopen`/`dlsym`/`dlvsym`）、exec シスコール、未解決の `svc #0x80` 直接シスコール、mprotect 系 `PROT_EXEC`（確定または不明）
- これらは「解析が成功し、バイナリの性質として確認・推定された」ものであり、利用者がリスクを受容して許可する運用（ログメッセージが案内する `set risk_level = "high" or higher to allow execution`）を維持する

**Acceptance Criteria**:
- **AC-40**: 上記の不確実ケース各々について、`risk_level = "high"` を設定していてもコマンドが実行されない。
- **AC-41**: 不確実ケースによる実行中止は、拒否理由（どの不確実ケースか）とともにログ・監査で追跡できる。
- **AC-42**: 有効な解析レコードから検出された危険な性質（動的ロード・exec・svc・mprotect）は従来どおり High であり、`risk_level = "high"` で実行できる。
- **AC-43**: dry-runも同じ分類を表示する（不確実ケースは実行拒否予告。既存の「ハッシュ検証失敗 → Critical」表示と整合する）。
- **AC-17**: 開発者向けドキュメントが、(a) エラーで実行中止するケース（シンボリックリンク深度超過・coreutils stat エラー・予期しないレコード読込エラー）、(b) 不確実ケースとして設定によらず実行中止するケース（本要件）、(c) High として許可設定下で実行可能なケース、の三者を明確に区別して記述する。ユーザー向け `risk_assessment.ja.md` §3.3 の表も新しい挙動に合わせて改訂する。

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

**Acceptance Criteria**:
- **AC-27**: `systemctl`（引数任意）について、実行時（`EvaluateRisk`）とdry-run（`AnalyzeCommandSecurity`）が同じリスクレベルを返す。
- **AC-28**: `rm`（破壊的操作の代表）について、basename 入力・絶対パス入力・coreutils 配下のいずれでも、実行時とdry-runで High が返る。
- **AC-29**: 複数機構に定義が重複するコマンドの優先順位（どの定義がどのパスで効くか）が、設計文書および開発者向けドキュメントに明記される。

### F-009: dry-runが実行時の判定を予告できる（I）

dry-runは、実行時と同じ評価器（`EvaluateRisk`）による実効リスクと、`risk_level` 設定との比較結果（実行時に許可されるか拒否されるか）を表示しなければならない。既存の `AnalyzeCommandSecurity` による詳細表示（ディレクトリ既定リスク・ハッシュ検証・setuid・危険パターン等）は維持してよい。

**Acceptance Criteria**:
- **AC-30**: dry-run出力に、実行時パスと同一ロジックで算出された実効リスクが含まれる。
- **AC-31**: dry-run出力に、`risk_level` 設定との比較結果（許可/拒否の予告）が含まれる。
- **AC-32**: バイナリ解析シグナル（ネットワーク/動的ロード/exec/mprotect）による High/Medium が、dry-runの実効リスク表示にも反映される（解析レコードが利用可能な場合）。
- **AC-33**: dry-runは引き続きコマンドを実行せず、判定不能・解析エラー時の挙動（エラー返却）は AC-18 の記述と一致する。

### F-010: ユーザー向けリスク評価ガイドを実装と一致させる（J）

`docs/user/risk_assessment.ja.md` / `.md` を、本タスクの修正後の実装に合わせて改訂する。

**Acceptance Criteria**:
- **AC-34**: §3.1 から `dpkg` を削除する（または実装に `dpkg` 検出を追加する場合はその旨を要件に追記して整合させる。既定は削除）。
- **AC-35**: コマンド名ベースの常時ネットワーク判定（`curl`/`wget`/`ssh` 等に加え、シェル・スクリプトインタプリタ `bash`/`python`/`node` 等が名前のみで `medium` になること）が利用者向けに説明される。
- **AC-36**: coreutils 単一バイナリ分類（Low/Medium/High の 3 区分）が説明される。
- **AC-37**: `systemctl` 等のリスクレベル記載が、F-008 で確立した定義と一致する。
- **AC-38**: 「最終リスクはすべての因子の最大値」の記述が、F-001 修正後の実装（プロファイル要因を含む最大値）と整合する。

## 3. 非機能要件

### NF-001: 後方互換性

- 既存の TOML 設定の解釈を破壊しない。ただし F-001/F-002/F-005/F-007 により、一部設定の挙動が変わる:
  - `claude` 等（実効リスク Medium→High）、`systemctl`/`service`（Medium→High）、絶対パス指定の破壊的コマンド（Low→High）は、`risk_level` 設定によっては従来実行できていたものが拒否される。これは **意図的な安全性向上** である。
  - バイナリ解析の不確実ケース（解析レコード欠落等）は、従来 `risk_level = "high"` で実行できていたものが **設定によらず拒否** される。解消手段は `record` の再実行による解析レコードの整備である。
  - `risk_level = "unknown"` は設定エラーになる（従来は全コマンド拒否として黙って動作）。
- これらの変更点を移行ノートとしてドキュメントに記載する。

**Acceptance Criteria**:
- **AC-19**: 実効リスク上昇により拒否され得る代表的ケース（`claude` を `risk_level=medium` で実行、絶対パス `rm` を `risk_level=low` で実行、`systemctl` を `risk_level=medium` で実行）、不確実ケースの設定によらない拒否（解消手段 = `record` 再実行）、および `"unknown"` の設定エラー化が、移行ノートまたは変更点としてドキュメントに記載される。

### NF-002: 実行時パスとdry-runパスの整合性

- 同一コマンドに対し、実行時（`EvaluateRisk`）とdry-runの実効リスクが一致する（F-008/F-009 の帰結）。

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
| フェイルクローズド | エラーを返してコマンド実行を中止する挙動 |
| フェイルセーフ | 不確実時に安全側（より高いリスク）へ倒す挙動 |
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
