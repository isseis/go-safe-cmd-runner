# 実行時リスク判定の強化 — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-13 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 1. 背景と目的

### 背景

PR #724（コマンドリスク判定の技術ドキュメント追加）のレビューにおいて、ドキュメントと実装の差異を調査した結果、`runner` の **実行時リスク判定（`risk.StandardEvaluator.EvaluateRisk`）** に複数の不具合・仕様不整合が確認された。これらはいずれも「実行時に算出される実効リスクが、設計意図より低く（安全側でなく）なる」方向の問題を含み、セキュリティ上の影響がある。

確認された事象（PR #724 のレビューコメントに対応）:

| ID | 概要 | 種別 |
|----|------|------|
| A | `EvaluateRisk` がコマンドリスクプロファイルの全要因（`BaseRiskLevel` = 各要因の最大値）を実行時に反映しない。プロファイルは `PrivilegeRisk`（特権昇格）と `NetworkType`（ネットワーク種別）しか実行時に参照されず、`DataExfilRisk` 等の宣言済みリスク要因が実効リスクに寄与しない。例: `claude` は `DataExfilRisk=High` を宣言しているが実行時は Medium となる。 | コード不具合 |
| B | リスク判定結果を記録する監査ログ（`audit.Logger.LogRiskProfile`）が本番コードパスから一度も呼び出されていない。ドキュメントは `command_risk_profile` 監査エントリが出力されると説明しているが、実際には出力されない。 | コード不具合 |
| C | `risk_level` はコマンドレベル（およびテンプレートレベルのフォールバック）でのみ設定可能で、グループレベル・グローバルレベルでは設定できない。ドキュメントおよびコードコメント（`nil: inherit from global default`）は、存在しない継承を示唆している。 | ドキュメント/コメント不整合 |
| D | 実行時、`cmd.ExpandedCmd` は絶対パスに解決済み（`group_executor.go` でのパス解決後）であるにもかかわらず、`IsDestructiveFileOperation` / `IsSystemModification` はコマンド名のリテラル一致（`rm`, `systemctl` 等）で判定する。このため `/usr/bin/rm` 等の絶対パスは破壊的操作・システム変更として検出されず、（coreutils ディレクトリ配下でない限り）実効リスクが Low に落ちる。 | コード不具合（セキュリティ） |
| E | バイナリ解析の不確実ケース（解析レコード欠落、ハッシュ/スキーマ不一致、`contentHash` 空など）は実効リスク **High** を返すが、エラーは返さないため `risk_level = "high"` の設定下では実行され得る。ドキュメントの要約は「フェイルクローズド（実行しない）」と一括りに記述しており、過大な保証になっている。真に実行を中止する（エラーを返す）のはシンボリックリンク深度超過・coreutils stat エラーのみ。 | ドキュメント不整合 |
| F | ドライランパスはパス解決失敗・解析エラー時にエラーを返す（High として表示継続しない）。ドキュメントの表「エラー時の挙動: フェイルセーフ（High として表示継続）」は過度の一般化。 | ドキュメント不整合 |

### 目的

1. 実行時リスク判定が、設計意図どおりに **安全側（fail-safe / fail-closed）** で動作することを保証する（A, D）。
2. リスク判定結果の **監査可能性** を確保する（B）。
3. ドキュメント・コードコメントを実装の実態と一致させ、利用者の誤解（とくに `risk_level` のスコープと「フェイルクローズド」の意味）を防ぐ（C, E, F）。
4. 実行時パス（`EvaluateRisk`）とドライランパス（`AnalyzeCommandSecurity`）の判定結果の整合性を高める。

### スコープ外

- グループレベル / グローバルレベルの `risk_level` 継承の **新規追加** は本タスクのスコープ外とする（C はあくまで現状スコープの正確な記述に留める）。
- リスクレベルの段階定義（Unknown/Low/Medium/High/Critical）の変更。
- バイナリ静的解析アルゴリズム自体の変更。

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

### F-002: 名前ベースの判定が解決済み絶対パスに対しても機能する（D）

`IsDestructiveFileOperation` および `IsSystemModification` は、実行時に渡される **解決済み絶対パス**（例: `/usr/bin/rm`）に対しても、対象コマンドを正しく検出しなければならない。判定はコマンド名（basename）に対して行い、シンボリックリンクの解決を考慮する。

**Acceptance Criteria**:
- **AC-06**: `IsDestructiveFileOperation("/usr/bin/rm", ...)` が `true` を返す（絶対パスでも破壊的操作として検出）。
- **AC-07**: `IsSystemModification("/usr/sbin/systemctl", ["restart", ...])` が `true` を返す（絶対パスでもシステム変更として検出）。
- **AC-08**: `EvaluateRisk` において、絶対パスで指定された `/usr/bin/rm -rf /tmp/x` の実効リスクが **High** になる（従来は Low）。
- **AC-09**: 部分一致による誤検出が起きない（例: `/usr/bin/lsrm` や `/usr/bin/systemctl-helper` のような basename は `rm` / `systemctl` と一致しない）。
- **AC-10**: 既存の basename 入力（`rm`, `systemctl` 等）に対する判定結果は従来どおり維持される（後方互換）。

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
- **AC-16**: 誤解を招くコードコメント（`spec.go` の `nil: inherit from global default` 等）が、実態（`nil` の場合は `low`）に即した記述へ修正される。

### F-005: 「フェイルクローズド」の意味を正確に記述する（E）

不確実ケースの扱いについて、(a) エラーを返して **実行を中止** するケース（シンボリックリンク深度超過・coreutils stat エラー）と、(b) **High に昇格** するが `risk_level=high` 下では実行され得るケース（解析レコード欠落・ハッシュ/スキーマ不一致・`contentHash` 空など）を区別して記述する。

**Acceptance Criteria**:
- **AC-17**: ドキュメントが上記 (a)（実行中止）と (b)（High 昇格・許可設定下では実行可）を明確に区別して記述する。

### F-006: ドライランのエラー時挙動を正確に記述する（F）

ドライランパスはパス解決失敗・解析エラー時にエラーを返す（High 表示で継続しない）。一方、`AnalyzeCommandSecurity` 内部の特定チェックは失敗を High/Critical に変換する。この区別をドキュメントに反映する。

**Acceptance Criteria**:
- **AC-18**: ドキュメントの実行時パス/ドライランパス比較表が、ドライランはパス解決・解析の失敗時にエラーを返すこと、High/Critical への変換は `AnalyzeCommandSecurity` 内の特定チェックに限られることを正確に記述する。

## 3. 非機能要件

### NF-001: 後方互換性

- 既存の TOML 設定の解釈を破壊しない。ただし F-001/F-002 により、一部コマンド（例: `claude`、絶対パス指定の `rm`）の実効リスクが上昇し、`risk_level` 設定によっては従来実行できていたコマンドが拒否される可能性がある。これは **意図的な安全性向上** であり、移行時の注意点としてドキュメントに記載する。

**Acceptance Criteria**:
- **AC-19**: 実効リスク上昇により拒否され得る代表的ケース（`claude` を `risk_level=medium` で実行、絶対パス `rm` を `risk_level=low` で実行）が、移行ノートまたは変更点としてドキュメントに記載される。

### NF-002: 実行時パスとドライランパスの整合性

- 同一コマンドに対し、実行時（`EvaluateRisk`）とドライラン（`AnalyzeCommandSecurity`）の実効リスクが、F-001/F-002 の修正後により高い整合性を持つ。

**Acceptance Criteria**:
- **AC-20**: 既存の整合性テスト（`coreutils_consistency_test.go`）が修正後も成立し、破壊的コマンド（絶対パス含む）について両パスが同じ High を返すことを検証する。

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

## 5. 参考

- PR #724: コマンドリスク判定ドキュメント追加とそのレビュー
- `docs/dev/architecture_design/command-risk-evaluation.ja.md`
- `internal/runner/base/risk/evaluator.go`
- `internal/runner/base/security/command_analysis.go`
- `internal/runner/base/security/command_risk_profile.go`
- `internal/runner/resource/normal_manager.go`
- `internal/runner/base/audit/logger.go`
