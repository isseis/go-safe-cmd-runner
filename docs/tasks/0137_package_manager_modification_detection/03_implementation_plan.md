# パッケージマネージャのシステム変更検出の一般化（dpkg/rpm フラグ方式対応） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-18 |
| Review date | 2026-06-18 |
| Reviewer | isseis |
| Comments | - |

関連文書: [01_requirements.md](01_requirements.md) / [02_architecture.md](02_architecture.md)

---

## 1. 実装概要

### 1.1 目的

`internal/runner/base/security` のシステム変更検出を、pacman 固有のフラグ判定からツール別機構へ
一般化し、dpkg/rpm のフラグ方式変更操作を Medium として検出する。設計の詳細は
[02_architecture.md](02_architecture.md) を参照（本書では再掲しない）。

### 1.2 実装原則

- **リファクタ先行（NF-005）**: まずツール別機構へ置換（pacman 移送）し、既存の有効な使用のテストが
  緑であることを確認してから、dpkg/rpm をデータとして追加する。
- **単一情報源の維持**: `SystemModificationRisk` のシグネチャ・呼び出し側（`evaluator.go` /
  `indirect_execution.go`）は変更しない。変更は `security` パッケージ内に閉じる。検出拡張は top-level
  `EvaluateRisk`（normal/dry-run が共有）へ波及する。ラッパー内側（RoleInner）は 0136 の flat High
  floor に倒れるため sysmod を経由せず High（≥ Medium）になる（AC-18 を満たす。02 §6.1/§6.2 参照）。
- **既存ヘルパの再利用**: テストは既存の `newVerifiedEvaluator()` / `verifiedCmd()` /
  `evalLevel` / `isSystemModification` ヘルパとラッパーテストの既存パターンを再利用する。

### 1.3 既存コード調査結果

| 対象 | 現状 | 本タスクでの扱い |
|---|---|---|
| `command_analysis.go` `isSystemModificationByNames` (l.558〜) | verb 方式（`packageManagerNames`+`packageModifyingVerbs`）＋ `isPacman` 個別分岐で pacman フラグを判定 | `isPacman` 分岐を削除し、`flagStyleManagers` を引く汎用ループへ置換 |
| `command_analysis.go` `isPacmanModifyingFlag` (l.482〜494) | pacman 専用関数。`ContainsAny(arg[1:], "SRU")`（any-char） | **削除**。規則を `flagStyleManagers["pacman"]` のデータへ移送（first-char 一致へ） |
| `command_analysis.go` `packageManagerNames` / `packageModifyingVerbs` | verb 方式マネージャ集合・変更系 verb 集合 | **不変**。dpkg/rpm は追加しない（02 §3.2 のゲート分離） |
| `command_analysis.go` `SystemModificationRisk` (l.589〜) | システム変更次元の単一情報源 | シグネチャ・本体構造とも不変 |
| `evaluator.go` l.231（top-level）/ `indirect_execution.go` l.840（RoleInterpreter 経路） | `SystemModificationRisk` を呼ぶ既存呼び出し元 | **変更不要**。検出拡張は top-level（normal/dry-run 共有）へ波及する。ラッパー内側（RoleInner）は `indirect_execution.go` l.803 で flat High floor（`ReasonIndirectExecutionWrapper`）に倒れ sysmod を経由しないが、結果は High（≥ Medium）で AC-18 を満たす |
| `internal/runner/base/security/command_analysis_test.go` | `isSystemModification(cmd,args)` ヘルパ、`TestIsSystemModification_PackageManagerVerbs`、`TestIsSystemModification`（build タグなし、`package security`） | pacman の既存ケースは退行検証として維持。dpkg/rpm ケースを追加 |
| `internal/runner/base/risk/test_helpers.go` / `internal/runner/base/risk/evaluator_test.go` | `newVerifiedEvaluator()` / `verifiedCmd()` / `evalLevel`（`//go:build test`） | 再利用 |
| `internal/runner/base/security/indirect_execution_test.go` | `TestIndirect_WrapperDestructive`（`env rm -rf`→High 等）/ `TestIndirect_WrapperSudoCritical`（`//go:build test`） | 同パターンで dpkg/rpm ラッパーケースを追加 |
| `internal/runner/base/risk/coreutils_consistency_test.go` | 共有評価器の出力固定（`//go:build test`） | 同方式で dpkg/rpm 用の整合テストを新規作成 |

新規追加（02 §3.3）: `package_manager_flags.go`（本体）、`package_manager_flags_test.go`（単体）、
`internal/runner/base/risk/package_manager_consistency_test.go`（整合）。テストヘルパの新規追加は不要。

---

## 2. 実装ステップ

### フェーズ 1: ツール別機構へのリファクタ（振る舞い不変・pacman 移送）

**対象ファイル**: `internal/runner/base/security/package_manager_flags.go`（新規）、
`internal/runner/base/security/command_analysis.go`（変更）

- [ ] **1-1**: `package_manager_flags.go`（`package security`、build タグなし）を作成し、`flagRule` 型と
  `flagStyleManagers` マップリテラルを定義する。初期エントリは **pacman のみ**
  （`modifyingShortChars: "SRU"`、`modifyingLongForms: {--sync,--remove,--upgrade}`、exclude 空）。
  型・フィールドの定義は 02 §3.1 に従う。コメント・識別子はすべて英語。
- [ ] **1-2**: `command_analysis.go` の `isSystemModificationByNames` を、`flagStyleManagers` を引いて
  各フラグ方式マネージャに規則を適用する汎用ループへ置換する。判定規則（除外優先 → 長形式完全一致 →
  短形式の先頭文字一致、退化トークンは長さ検査で非該当）は 02 §3.1 に従う。verb 方式判定
  （`packageManagerNames`+`packageModifyingVerbs`）と systemctl/service 分岐は不変。フラグ方式ループは
  `flagStyleManagers` 所属で独立に発火させる（`packageManagerNames` ゲートに依存しない）。
- [ ] **1-3**: `isPacmanModifyingFlag` 関数（コメント含む l.478〜494）と、`isSystemModificationByNames`
  内の `_, isPacman := names["pacman"]` 行および `isPacman && isPacmanModifyingFlag(arg)` 分岐を削除する。
- [ ] **1-4（完了ゲート）**: `make fmt` → `make test` → `make lint` を実行し、既存テスト（特に
  `TestIsSystemModification_PackageManagerVerbs` の pacman・verb ケース、`TestIsSystemModification`）が
  **変更なしで緑**であることを確認する。`rg -n "isPacmanModifyingFlag|isPacman\b" internal --glob '*.go'`
  が 0 件であることを確認する（AC-10 / NF-005）。

### フェーズ 2: dpkg/rpm のデータ追加とテスト

**対象ファイル**: `internal/runner/base/security/package_manager_flags.go`（変更）、
`internal/runner/base/security/package_manager_flags_test.go`（新規）、
`internal/runner/base/security/command_analysis_test.go`（変更）、
`internal/runner/base/risk/evaluator_test.go`（変更）、
`internal/runner/base/security/indirect_execution_test.go`（変更）、
`internal/runner/resource/audit_wiring_test.go`（変更）、
`internal/runner/base/risk/package_manager_consistency_test.go`（新規）

- [ ] **2-1**: `flagStyleManagers` に dpkg・rpm のエントリを追加する（値は 02 §3.1 の規則表どおり。
  dpkg: `irP` / `{--install,--remove,--purge,--unpack,--configure}` / exclude 空。rpm: `iUFe` /
  `{--install,--upgrade,--freshen,--erase,--reinstall,--import,--initdb,--rebuilddb}` /
  exclude short `qV`・long `{--query,--verify}`）。本体ロジックの変更は不要（データ追加のみ）。
- [ ] **2-2**: `package_manager_flags_test.go`（`package security`、build タグなし。`command_analysis_test.go`
  に合わせる）を作成し、`isSystemModification` ヘルパを用いてツール別の肯定・否定・境界を固定する:
  - [ ] `TestSystemModification_DpkgFlags`: 肯定 `-i`/`-r`/`-P`/`--install`/`--remove`/`--purge`/
    `--unpack`/`--configure`（`--configure -a` 含む）。否定 `-l`/`-L`/`-s`/`-S`/`-p`/`-I`/`-c`/
    `--info`/`--list`/`--status`/`--get-selections`/`--print-avail`/引数なし、先頭文字境界 `-D<level>`/`-list`。
    大小区別 `-i`≠`-I`（AC-01/AC-02/AC-03）。
  - [ ] `TestSystemModification_RpmFlags`: 肯定 `-i`/`-U`/`-F`/`-e`/`-ivh`/`-Uvh`/`-e --nodeps`/
    `-e --verbose`/`-U --force`/`--install`/`--upgrade`/`--freshen`/`--erase`/`--reinstall`/`--import`/
    `--initdb`/`--rebuilddb`。否定 `-qi`/`-qa`/`-qpi`/`-ql`/`-V`/`-qa --verbose`/`--query`/`--verify`/
    `--eval`/`--querytags`/引数なし、先頭文字境界 `-E%{_libdir}`/`-D'enable_foo 1'`（AC-05/AC-06）。
  - [ ] `TestSystemModification_RpmExcludePriority`: 除外優先 `-q -i`/`-i -q`/`-e -q`（いずれも非検出。AC-07）。
  - [ ] `TestSystemModification_PacmanFlags`: 肯定 `-S`/`-R`/`-U`/`-Syu`/`-Rns`/`--sync`/`--remove`/
    `--upgrade`、既存過検出 `-Ss`/`-Si`（先頭 `S` で検出）。否定 `-Q`/`-Qi`、許容差異 `-yS`（非検出）（AC-09）。
  - [ ] `TestSystemModification_AbsolutePathAndSymlink`: 絶対パス `/usr/bin/dpkg -i`・`/usr/bin/rpm -U` が
    検出されること、および symlink エイリアス経由（既存 `TestExtractAllCommandNamesWithSymlinks` と同様に
    `t.TempDir` 配下へリンクを作成）でも検出されること（AC-04/AC-08）。
- [ ] **2-3**: `command_analysis_test.go::TestIsSystemModification_PackageManagerVerbs` に dpkg/rpm の
  代表ケース（`dpkg -i`→true、`dpkg -l`→false、`rpm -U`→true、`rpm -qi`→false）を追加する。pacman・verb の
  既存ケースは変更しない（AC-11 退行検証）。
- [ ] **2-4**: `internal/runner/base/risk/evaluator_test.go::TestStandardEvaluator_EvaluateRisk_SystemModifications` に
  `dpkg -i pkg.deb`→`RiskLevelMedium`、`rpm -U pkg.rpm`→`RiskLevelMedium` を追加する（`evalLevel` 利用。AC-12）。
- [ ] **2-5**: `internal/runner/base/risk/evaluator_test.go` に `TestEvaluateRisk_PackageManagerReasonCode`（新規）を追加し、
  `dpkg -i pkg.deb` の `plan.Assessment.ReasonCodes` に `risktypes.ReasonSystemModification` が含まれる
  ことを検証する（分類が reason code を生成すること。AC-19 の前段）。
- [ ] **2-5b**: `internal/runner/resource/audit_wiring_test.go` に `TestExecute_PackageManagerDenyAuditable`（新規）を
  追加し、Medium かつ `ReasonSystemModification` を持つ**非ブロッキング**のプラン（dpkg/rpm の分類を模す。
  既存 `fixedPlanEvaluator` を利用）を `max_allowed = low` で実行したとき、deny の監査エントリ
  （`decision=deny`、`reason_codes` に `system_modification`、`resolved_path`）が出力されることを検証する。
  既存 `TestExecute_RejectedCommandAuditable` のヘルパ（`newAuditingNormalManager` / `findRiskProfileEntry`）を
  再利用する。これにより「閾値超過の deny が新分類の監査エントリとして記録される」ことを実経路で固定する（AC-19）。
- [ ] **2-6**: `internal/runner/base/security/indirect_execution_test.go`（`package security`、
  `//go:build test`）に `TestIndirect_WrapperPackageManager`（新規）を追加し、ラッパー経由の実効リスクが
  直接実行と**同等以上（≥ Medium）**になることを検証する: `env dpkg -i pkg.deb`→`RiskLevelHigh`、
  `timeout 60 rpm -U pkg.rpm`→`RiskLevelHigh`、`env sudo rpm -U pkg.rpm`→`RiskLevelCritical`。
  ラッパー内側（RoleInner）は 0136 で確立した flat High floor（`ReasonIndirectExecutionWrapper`）に
  倒れるため High、ラップした sudo は特権昇格で Critical（**直接 `sudo` は indirect 解析の対象外**＝
  `AnalyzeIndirectExecution` は `IndirectNone` を返すため、`env` でラップした形にする。既存
  `TestIndirect_WrapperSudoCritical` と同方式）。いずれも dpkg/rpm の Medium が結果を引き下げないこと
  （AC-14 の観測）と ≥ Medium であること（AC-18）を同時に示す。既存 `TestIndirect_WrapperDestructive`
  と同じテーブル形式・評価ヘルパを用いる。
- [ ] **2-7**: `internal/runner/base/risk/package_manager_consistency_test.go`（新規・`//go:build test`）に
  `TestPackageManagerRiskConsistency_RuntimeVsDryRun` を追加し、`coreutils_consistency_test.go` と同方式で、
  代表 dpkg/rpm コマンド集合に対し共有評価器（`newVerifiedEvaluator().EvaluateRisk`）が一意の実効リスクを
  返すことを固定する（実行時と dry-run が同一評価器を共有することの回帰防止。AC-13）。本テストは
  `coreutilsDir` グローバルを変更しないため `t.Parallel()` を使用してよい。
- [ ] **2-8（完了ゲート）**: `make fmt` → `make test` → `make lint` が緑。`make deadcode` で未使用検出なし。

### フェーズ 3: ドキュメント整合（F-005）

**対象ファイル**: `docs/user/risk_assessment.ja.md` / `.md`、
`docs/dev/architecture_design/command-risk-evaluation.ja.md` / `.md`、`docs/translation_glossary.md`

- [ ] **3-1**: `risk_assessment.ja.md` / `.md` の §3.1 表「その他のシステム変更コマンド」行に、dpkg/rpm の
  フラグ方式変更操作（dpkg: install/remove/purge/unpack/configure、rpm: install/upgrade/freshen/erase/
  reinstall/import/initdb/rebuilddb）を `medium` として追記する（AC-16）。
- [ ] **3-2**: 同 §8 移行ノートに破壊的変更の項目を追記する（旧 `low`/未指定の上記操作が `medium` で
  ブロックされる旨、回避策＝該当コマンドに `risk_level = "medium"` を明示、`--dry-run` での事前確認。
  purge/erase/freshen/reinstall を含む全操作種別を明示）（AC-20）。
- [ ] **3-3**: 同 §3.1 注記または §7 脅威モデルと限界に、検出は dpkg/rpm/pacman 対象（ラッパー経由も評価）
  である一方、照会形・未列挙マネージャ（apk/snap 等）・multi-call（`busybox <pm>`）・リネームは対象外で
  `low` 通過しうること、allowlist+ハッシュ固定が前提であることを追記する（AC-21）。
- [ ] **3-4**: `command-risk-evaluation.ja.md` / `.md` の「システム変更リスク」節に、フラグ方式
  （pacman/dpkg/rpm）検出と rpm の照会/検証除外規則を追記する（AC-17）。
- [ ] **3-5**: `translation_glossary.md` に新規用語（フラグ方式/flag style、verb 方式/verb style、
  照会/query、検証/verify、システム変更/system modification、ツール別機構/per-tool mechanism）の
  対訳を追加する（AC-22）。
- [ ] **3-6（完了ゲート）**: 日英 2 ファイルで対象操作集合・値が一致することを確認（§7 AC 検証の static
  コマンドを実行）。

---

## 3. 実装順序とマイルストーン

| マイルストーン | 内容 | 完了条件 |
|---|---|---|
| M1 | フェーズ 1（リファクタ・pacman 移送） | 既存テスト緑、`isPacmanModifyingFlag`/`isPacman` 残存 0 件 |
| M2 | フェーズ 2（dpkg/rpm 追加・テスト） | 新規/拡張テスト緑、`make test`/`make lint`/`make deadcode` 通過 |
| M3 | フェーズ 3（ドキュメント） | §7 の文書 static 検証がすべて期待どおり、日英整合 |

順序は 02 §8 のフェーズ定義に一致する（M1 を先行させる NF-005）。

---

## 4. テスト戦略

- **単体（ツール別規則）**: `package_manager_flags_test.go` で dpkg/rpm/pacman の肯定・否定・境界
  （大小区別・先頭文字・rpm 除外・修飾子併用・退化トークン）を固定（AC-01〜AC-09 / NF-001）。
- **回帰（振る舞い不変）**: `command_analysis_test.go` の既存 pacman・verb ケースを無変更で維持
  （F-003 / AC-09 / AC-11）。
- **統合（実効リスク・複合・間接・監査）**: `internal/runner/base/risk/evaluator_test.go`（top-level
  Medium・reason code）/ `internal/runner/base/security/indirect_execution_test.go`（ラッパー・`env sudo`
  複合）/ `internal/runner/resource/audit_wiring_test.go`（deny 監査エントリ）（AC-12/14/18/19）。
- **整合（実行時/dry-run）**: `package_manager_consistency_test.go`（共有評価器出力固定）（AC-13）。
- **文書（static）**: §7 の rg コマンドで存在・日英整合を確認（AC-15〜AC-17 / AC-20〜AC-22）。
- 後方互換: 既存の verb 方式・systemctl/service・coreutils・破壊操作の判定は変更しない（回帰テストで担保）。

---

## 5. リスク管理

| リスク | 対応 |
|---|---|
| pacman の any-char→first-char 変更で予期しない既存ケース退行 | フェーズ 1 完了ゲートで既存テスト無変更緑を必須化。差異は無効入力（`-yS`）に限ることを単体テストで明示（AC-09 許容範囲） |
| dpkg/rpm 追加が他次元（破壊・特権）に影響 | データ追加のみで本体不変。最大値合成は 0136 既存機構（AC-14 をテストで確認） |
| ドキュメント日英不整合 | フェーズ 3 完了ゲートで static 検証（日英両ファイル）を必須化 |
| 削除した `isPacmanModifyingFlag` の残存参照 | 完了ゲートとクロス検索（§8）で 0 件確認 |

---

## 6. 実装チェックリスト

- [ ] フェーズ 1: 1-1 / 1-2 / 1-3 / 1-4（ゲート）
- [ ] フェーズ 2: 2-1 / 2-2 / 2-3 / 2-4 / 2-5 / 2-6 / 2-7 / 2-8（ゲート）
- [ ] フェーズ 3: 3-1 / 3-2 / 3-3 / 3-4 / 3-5 / 3-6（ゲート）
- [ ] 全体: `make test` / `make lint` / `make deadcode` 緑
- [ ] §8 クロス検索チェックリスト完了

---

## 7. 受け入れ基準の検証

`test` = 実行可能テスト、`static` = rg/コンパイル、`manual` = PR 観察。すべての AC に `test` か `static` を持たせる。

| AC | 種別 | 検証 |
|---|---|---|
| AC-01 dpkg 検出 | test | `internal/runner/base/security/package_manager_flags_test.go::TestSystemModification_DpkgFlags`（肯定群が true） |
| AC-02 dpkg 照会非検出 | test | `…::TestSystemModification_DpkgFlags`（否定群・先頭文字境界が false） |
| AC-03 dpkg 大小区別 | test | `…::TestSystemModification_DpkgFlags`（`-i`=true / `-I`=false） |
| AC-04 dpkg 絶対パス・symlink | test | `…::TestSystemModification_AbsolutePathAndSymlink`（`/usr/bin/dpkg -i` と symlink エイリアスが true） |
| AC-05 rpm 検出（修飾子併用含む） | test | `…::TestSystemModification_RpmFlags`（肯定群が true。`-e --verbose`/`--reinstall`/`--import` 等を含む） |
| AC-06 rpm 照会非検出 | test | `…::TestSystemModification_RpmFlags`（否定群・先頭文字境界が false） |
| AC-07 rpm 除外優先 | test | `…::TestSystemModification_RpmExcludePriority`（`-q -i`/`-i -q`/`-e -q` が false） |
| AC-08 rpm 絶対パス・symlink | test | `…::TestSystemModification_AbsolutePathAndSymlink`（`/usr/bin/rpm -U` と symlink が true） |
| AC-09 pacman 回帰・許容差異 | test | `…::TestSystemModification_PacmanFlags`（`-S`/`-Syu`/`-Rns`/`-Ss`/`-Si`=true、`-Q`/`-Qi`/`-yS`=false）＋ `command_analysis_test.go::TestIsSystemModification_PackageManagerVerbs`（既存 pacman ケース無変更緑） |
| AC-10 マネージャ名分岐の排除 | static | `rg -n "isPacmanModifyingFlag|isPacman\b" internal --glob '*.go'` → 0 件（期待: マッチなし） |
| AC-11 verb 方式の非退行 | test | `command_analysis_test.go::TestIsSystemModification_PackageManagerVerbs`（apt/yum/dnf/yarn/npm/brew ケース無変更緑） |
| AC-12 実効 Medium | test | `internal/runner/base/risk/evaluator_test.go::TestStandardEvaluator_EvaluateRisk_SystemModifications`（`dpkg -i`/`rpm -U`=`RiskLevelMedium`） |
| AC-13 実行時/dry-run 一致 | test | `internal/runner/base/risk/package_manager_consistency_test.go::TestPackageManagerRiskConsistency_RuntimeVsDryRun` |
| AC-14 最大値合成 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_WrapperPackageManager`（`env dpkg -i`=High／`env sudo rpm -U`=Critical＝より高い次元が支配し、dpkg/rpm の Medium に引き下がらない）＋ 既存 `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_MaxOfDimensionsOrderIndependent`（順序非依存の最大値不変条件。0136） |
| AC-18 ラッパー/間接 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_WrapperPackageManager`（`env dpkg -i`／`timeout 60 rpm -U`=High、`env sudo rpm -U`=Critical。いずれも ≥ Medium） |
| AC-19 監査 reason code | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_PackageManagerReasonCode`（dpkg→`ReasonSystemModification` を分類が生成）＋ `internal/runner/resource/audit_wiring_test.go::TestExecute_PackageManagerDenyAuditable`（Medium+`system_modification` の閾値 deny が `decision=deny`・`reason_codes`・`resolved_path` 付きで監査記録される） |
| AC-15 AC-34 上書き明記 | static | `rg -n "AC-34" docs/tasks/0137_package_manager_modification_detection/01_requirements.md` → §1.2 に「別途の要件変更として扱う」を上書きする記述がヒット（期待: マッチあり） |
| AC-16 §3.1 表反映（日英） | static | `rg -n "dpkg|rpm" docs/user/risk_assessment.ja.md docs/user/risk_assessment.md` で §3.1 表行に dpkg/rpm の `medium` 記述がヒット（期待: 日英両ファイルでマッチ。値 `medium`・操作集合（dpkg: install/remove/purge/unpack/configure、rpm: install/upgrade/freshen/erase/**reinstall**/import/initdb/rebuilddb）が一致） |
| AC-17 開発者文書反映（日英） | static | `rg -n "dpkg.*rpm|フラグ方式|flag style" docs/dev/architecture_design/command-risk-evaluation.ja.md docs/dev/architecture_design/command-risk-evaluation.md` で「システム変更リスク」節に flag-style 検出と rpm 除外規則がヒット（期待: 日英両方マッチ） |
| AC-20 移行ノート（日英） | static | `rg -n "dpkg|rpm" docs/user/risk_assessment.ja.md docs/user/risk_assessment.md` の §8 に破壊的変更・`risk_level = "medium"`・`--dry-run` を含む移行項目がヒット（期待: 日英両方マッチ） |
| AC-21 検出限界（日英） | static | `rg -n "apk|snap|busybox|allowlist" docs/user/risk_assessment.ja.md docs/user/risk_assessment.md` で検出対象外・backstop 記述がヒット（期待: 日英両方マッチ） |
| AC-22 用語集 | static | `rg -n "フラグ方式|flag style|ツール別|per-tool" docs/translation_glossary.md` → 追加用語がヒット（期待: マッチあり） |

> AC-16/AC-20/AC-21 の rg は dpkg/rpm 等の語のヒットに加え、対象節（§3.1 表 / §8 / §7）での記述であることと
> 日英で対応する内容であることを目視で確認する（static の補助としての manual 確認）。

NF の検証: NF-001 は単体テストの境界ケース（上記 AC-01/03/05/06）で固定。NF-002 は AC-13。NF-003 は AC-10
（データ追加で完結＝本体分岐なし）。NF-004 は AC-21。NF-005 はフェーズ 1 完了ゲート（既存テスト無変更緑）。

---

## 8. クロス検索チェックリスト（`make lint`/`make test` で検出できない項目のみ）

- [ ] **削除シンボルの残存参照**: `rg -n "isPacmanModifyingFlag" internal docs --glob '!docs/tasks/**'`
  → 0 件（コード・コメント・ライブ docs。`docs/tasks/` 配下のスナップショットは本タスクのコミット履歴・
  設計記述として `isPacmanModifyingFlag` を含むため、検索対象から除外する）。
- [ ] **`isPacman` ローカル変数の残存**: `rg -n "isPacman\b" internal --glob '*.go'` → 0 件。
- [ ] **新規識別子の衝突確認**: `rg -n "flagStyleManagers|flagRule\b" internal --glob '*.go'` が
  `internal/runner/base/security` 配下のみであること（汎用名のため他パッケージ衝突がないこと）。
- [ ] **用語集整合**: フェーズ 3 で追加した対訳が本文（要件・設計・ユーザー文書）の用語と一致すること。

---

## 9. 次のステップ

- 本計画の承認後、フェーズ 1 から実装に着手する。
- 実装中は各ステップのチェックボックスを随時更新する。
- 全フェーズ完了後、PR を作成しレビューを受ける（既存の PR 運用に従う）。
