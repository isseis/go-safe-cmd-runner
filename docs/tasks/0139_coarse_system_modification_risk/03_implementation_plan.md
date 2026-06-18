# システム変更リスク判定の粗粒度化（バイナリ名マッチへの単純化） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-18 |
| Review date | - |
| Reviewer | - |
| Comments | - |

関連文書: [01_requirements.md](01_requirements.md)（承認済み）、[02_architecture.md](02_architecture.md)（承認済み）。
設計の詳細は本計画では再掲せず、アーキテクチャの該当節を参照する。

---

## 1. 実装概要

### 1.1 目的

`SystemModificationRisk` から引数（サブコマンド／フラグ）解析を撤廃し、解決済みバイナリ名のみで
固定レベルを返す構造へ単純化する（設計: [02_architecture.md](02_architecture.md) §3.1〜§3.3）。
パッケージマネージャ・`systemctl`・`service` を High、その他名マッチ系を Medium とする。

### 1.2 実装原則

- アーキテクチャ §1.1 の設計原則に従う。新規パッケージ・新規エラー型は導入しない（§4）。
- Go ソースのコメント・識別子・文字列リテラルは英語で書く。
- 各フェーズ完了時に `make fmt` → `make test` → `make lint` を実行する（NF-002）。

### 1.3 既存コード調査結果（step 5）

調査で確認した現状と、本実装で必要な変更点は以下のとおり。

**変更・撤去対象（`internal/runner/base/security/`）**

- `SystemModificationRisk(names, args)`（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go) L576）:
  現状は `systemctl`→`SystemctlSubcommandRisk`、`service`→High、`isSystemModificationByNames`→Medium。
  これを名マッチ固定レベルへ書き換え、引数 `args` を撤去する。
- `isSystemModificationByNames(names, args)`（同 L540）: `systemModificationCommandNames`・
  `packageManagerNames`＋`packageModifyingVerbs`・`flagStyleManagers`/`isFlagStyleModification` を
  参照。撤去し、新 2 集合の名マッチへ置換。
- 名集合: `systemModificationCommandNames`（L434、`systemctl`/`service` を含む）と
  `packageManagerNames`（L452、`dpkg`/`rpm` を**含まない**）を、`highSystemModificationNames`
  （PM 12 種＋`systemctl`/`service`）と `mediumSystemModificationNames`（残り）へ再編。
  `dpkg`/`rpm` は現状どの集合にも無く本次元では未検出（実効 Low）→ High 集合へ新規追加。
- `packageModifyingVerbs`（L469）を削除。
- [systemctl.go](../../../internal/runner/base/security/systemctl.go) 全体（`SystemctlSubcommandRisk`／
  `firstSystemctlSubcommand`／`systemctlChangeVerbs`／`systemctlReadOnlyVerbs`／
  `systemctlValueOptions`／`systemctlBoolOptions`）を削除。
- [package_manager_flags.go](../../../internal/runner/base/security/package_manager_flags.go) 全体
  （`flagRule`／`flagStyleManagers`／`isFlagStyleModification`／`matchesShortFlag`）を削除。

**維持する既存資産（再利用、変更しない）**

- `anyNameInSet(names, set)`（command_analysis.go L479）: `arbitrary_code.go`・破壊的判定でも共有。
  新 `SystemModificationRisk` でも再利用する（新ヘルパは作らない）。
- `ResolveCommandNames`／`extractAllCommandNames`（symlink 解決）: 変更不要。
- 危険引数パターン次元（`CheckDangerousArgPatterns`／`dangerousCommandPatterns`）: スコープ外・不変。
  `mkfs`/`fdisk` の実効 High はこの次元が担う（§3.1 注記）。

**呼び出し元（シグネチャ追従のみ、ロジック不変）**

- [evaluator.go](../../../internal/runner/base/risk/evaluator.go) L231 `evaluateDimensions`。
- [indirect_execution.go](../../../internal/runner/base/security/indirect_execution.go) L840 `evaluateInnerAs`
  （`RoleInterpreter` 経路でのみ到達。`RoleInner` は手前の L803-810 で一律 High floor を返すため
  AC-08 の結論に影響しない）。

**影響を受ける既存テスト（全数。アーキテクチャ §3.5 の一覧に加え、本調査で追加特定したものを含む）**

| テスト | 場所 | 現状の表明 | 必要な対応 |
|---|---|---|---|
| `TestFirstSystemctlSubcommand` | security/systemctl_test.go | verb 解析前提 | ファイル削除に伴い削除 |
| `TestSystemctlSubcommandRisk` | security/systemctl_test.go | read-only=Medium | ファイル削除に伴い削除 |
| `isSystemModification`（ヘルパ） | security/command_analysis_test.go L1101 | `isSystemModificationByNames` を呼ぶ | 削除（被参照テストの改訂と同時） |
| `TestIsSystemModification_AbsolutePath` | 同 L1108 | ヘルパ経由 | `SystemModificationRisk` 直接呼びへ改訂 |
| `TestIsSystemModification_PackageManagerVerbs` | 同 L1117 | `apt list`/`pacman -Q`=false | 照会系も High（非該当でない）へ改訂・新テストへ統合 |
| `TestIsSystemModification` | 同 L1161 | `apt list`=false 等 | 新 `TestSystemModificationRisk` へ置換 |
| `TestStandardEvaluator_EvaluateRisk_SystemModifications` | risk/evaluator_test.go L101 | `apt/yum install`=Medium | **High へ改訂**（追加特定） |
| `TestStandardEvaluator_EvaluateRisk_SafeCommands` | 同 L121 | `apt list`=Low | `apt list` ケースを除去（apt は High）（追加特定） |
| `TestEvaluateRisk_SystemctlSubcommandConditional` | 同 L188 | `status`/`show`=Medium | High へ改訂（status/show 行） |
| `TestConsistency_Systemctl` | risk/coreutils_consistency_test.go L214 | `systemctl status`=Medium | High へ改訂（追加特定） |
| `TestEvaluateRisk_SystemctlChangeAndServiceHigh` | risk/evaluator_test.go L180 | High | 結論不変（維持） |
| `TestEvaluateRisk_ServiceAllActionsHigh` | 同 L197 | High | 結論不変（維持） |
| `TestEvaluateRisk_NoProfileAbsolutePath`（mount/crontab=Medium） | 同 L203 | Medium | 結論不変（維持） |
| `TestEvaluateRisk_DangerousArgPatternsRuntime`（mkfs.ext4=High） | 同 L212 | High | 結論不変（維持） |

**影響を受ける文書・config**

- ユーザー文書: `docs/user/risk_assessment.ja.md` / `.md`（systemctl/PM サブコマンド粒度の記述、
  L75-96 付近・L175-203 付近）。
- 開発者文書: `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `.md`（「System
  Modification Risk」節、`SystemModificationRisk(names, args)`・`SystemctlSubcommandRisk`・verb-only
  Medium の記述。EN は L291 付近）。
- ユーザーガイド: `docs/user/toml_config/09_practical_examples.ja.md` / `.md` ほか `toml_config/`
  配下の PM／systemctl 実践例（`risk_level=medium` の箇所）。
- sample: `sample/risk-based-control.toml`（apt update=medium L67、systemctl restart=medium L103）、
  `sample/timeout_examples.toml`（apt update/upgrade、`risk_level` 無し L72-73/79-80）。
- sample のロード検証は `internal/runner/config/template_backward_compat_test.go`
  （L24-25 で両 sample を列挙）が担う。`risk_level` 値は load 時に `ParseRiskLevel` で検証される
  （`high` は有効値）ため、変更後もロード成功を回帰確認できる。

---

## 2. 実装ステップ

### Phase 1: 判定本体の名マッチ固定レベル化（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go)）

**作業内容**

- [ ] `highSystemModificationNames`（PM 12 種：apt/apt-get/yum/dnf/zypper/pacman/brew/pip/npm/yarn/
  **dpkg/rpm** ＋ systemctl/service）を定義する（アーキテクチャ §3.1 の定義に一致させる）。
- [ ] `mediumSystemModificationNames`（chkconfig/update-rc.d/mount/umount/fdisk/parted/mkfs/fsck/
  crontab/at/batch）を定義する。
- [ ] `SystemModificationRisk` を新シグネチャ `func SystemModificationRisk(names map[string]struct{}) runnertypes.RiskLevel`
  へ書き換える（§3.2）。判定順: high 集合一致→High、medium 集合一致→Medium、いずれも非該当→Unknown。
  `anyNameInSet` を再利用する。
- [ ] `isSystemModificationByNames`／`packageModifyingVerbs`／`packageManagerNames`／
  `systemModificationCommandNames` を削除する。
- [ ] `SystemModificationRisk` の doc コメントを新仕様（引数非依存・固定レベル）へ更新する（英語）。

**完了基準**: パッケージ単体ビルドが通る（`go build -tags test ./internal/runner/base/security/`）。
旧シンボル参照が同ファイル内に残らない。

### Phase 2: 呼び出し元のシグネチャ追従

**作業内容**

- [ ] [evaluator.go](../../../internal/runner/base/risk/evaluator.go) L231 を
  `security.SystemModificationRisk(names)` へ変更する。
- [ ] [indirect_execution.go](../../../internal/runner/base/security/indirect_execution.go) L840 を
  `SystemModificationRisk(innerNames)` へ変更する。`innerArgs` がこの行でのみ未使用化しても他で
  使われているため未使用変数エラーは生じないことを確認する。

**完了基準**: `go build -tags test ./...` が通る。

### Phase 3: 旧機構ファイルの削除（NF-001）

**作業内容**

- [ ] [systemctl.go](../../../internal/runner/base/security/systemctl.go) を削除する。
- [ ] [package_manager_flags.go](../../../internal/runner/base/security/package_manager_flags.go) を削除する。

**完了基準**: `go build -tags test ./...` が通る。NF-001 の cross-search（§後述）で残存参照ゼロ。

### Phase 4: テスト改訂

**新規・改訂テスト**

- [ ] security/command_analysis_test.go に `TestSystemModificationRisk` を新設する。`SystemModificationRisk`
  を直接呼び、以下を表明する:
  - High: `apt`/`apt-get`/`yum`/`dnf`/`zypper`/`pacman`/`brew`/`pip`/`npm`/`yarn`/`dpkg`/`rpm`、
    `systemctl`、`service`（引数有無・install/照会の両方で High。AC-01/03/04）。
  - Medium: `mount`/`umount`/`fdisk`/`parted`/`mkfs`/`fsck`/`crontab`/`at`/`batch`/`chkconfig`/
    `update-rc.d`（AC-06）。
  - Unknown: 非該当名（`echo`/`ls`）、および `names` に pm 名を含まないケース（AC-02）。
  - args 非依存: 同一 `names` で異なる args（install 系／照会系／空）でも結果が一致（AC-05）。
  - symlink/絶対パス: `/usr/sbin/systemctl` は High、`systemctl-helper` は非該当（AC-01）。
- [ ] security/command_analysis_test.go の `isSystemModification` ヘルパ（L1101）を削除する。
- [ ] `TestIsSystemModification_AbsolutePath`（L1108）を削除する（`TestSystemModificationRisk` の
  symlink/絶対パスケースで代替）。
- [ ] `TestIsSystemModification_PackageManagerVerbs`（L1117）を削除する（同上で代替。照会系=High に変わる）。
- [ ] `TestIsSystemModification`（L1161）を削除する（`TestSystemModificationRisk` で代替）。
- [ ] security/systemctl_test.go を削除する（`TestFirstSystemctlSubcommand`／`TestSystemctlSubcommandRisk`
  の 2 関数を含む。両関数の不変条件は `TestSystemModificationRisk` の systemctl=High と
  evaluator 統合テストで代替）。
- [ ] risk/evaluator_test.go `TestStandardEvaluator_EvaluateRisk_SystemModifications`（L111-112）の
  `apt install`／`yum install` 期待値を `RiskLevelMedium` → `RiskLevelHigh` へ変更する。
- [ ] risk/evaluator_test.go `TestStandardEvaluator_EvaluateRisk_SafeCommands`（L131）の
  `{"apt list (query)", "apt", []string{"list", "--installed"}}` ケースを削除する（apt は High に
  なり Low 前提が崩れるため。AC-01/10）。
- [ ] risk/evaluator_test.go `TestEvaluateRisk_SystemctlSubcommandConditional`（L190-191）の
  `systemctl status`／`systemctl show` 期待値を `RiskLevelMedium` → `RiskLevelHigh` へ変更し、
  テスト名・コメントの「read-only は Medium floor」記述を「常に High」へ更新する（AC-03）。
- [ ] risk/coreutils_consistency_test.go `TestConsistency_Systemctl`（L218）の `systemctl status`
  期待値を `RiskLevelMedium` → `RiskLevelHigh` へ変更し、コメントを更新する（AC-07）。
- [ ] AC-09 検証テストを追加する（直接実行 sysmod deny の理由コードが `ReasonSystemModification`）。
  既存に該当がなければ risk/evaluator_test.go に追加。`risk_level` 上限超過で deny したときの
  `plan.Assessment.ReasonCodes` に `risktypes.ReasonSystemModification` が含まれることを表明。
- [ ] AC-08 回帰テストを追加する（本タスクで結論は不変だが明示固定）。security/indirect_execution_test.go に
  `analyzeIndirectCmd("env", "dpkg", "-i", "pkg.deb")` と `("env", "systemctl", "restart", "nginx")`
  が `IndirectFloor` かつ `Level == RiskLevelHigh` を、risk/evaluator_test.go に
  `sudo dpkg`／`sudo systemctl restart` が `RiskLevelCritical` を表明する。

**完了基準**: `make test` が緑。

### Phase 5: 文書・config の整合

**ユーザー文書（AC-10/11/12/13）**

- [ ] `docs/user/risk_assessment.ja.md` の systemctl/PM サブコマンド粒度記述（read-only=medium 等）を、
  固定レベル（PM／systemctl いずれも `high`）へ更新する。表示・照会系が High へ上がる旨の移行ノート
  （AC-10、baseline=直近リリース）と、検出限界（apk/snap/flatpak/gem・リネーム・busybox 形式、AC-12）、
  0137/systemctl 粒度の撤回（AC-13）を記載する。
- [ ] `docs/user/risk_assessment.md`（英語）を `/mktrans` で `.ja.md` から反映する
  （バイリンガル編集順序: ja を先に確定）。

**開発者文書（F-005 §1.4）**

- [ ] `docs/dev/architecture_design/command-risk-evaluation.ja.md` の「System Modification Risk」節を、
  `SystemModificationRisk(names)`（引数非依存）・名マッチ固定レベル（PM／systemctl=High）へ更新し、
  `SystemctlSubcommandRisk`・verb-only Medium の記述を削除する。
- [ ] `docs/dev/architecture_design/command-risk-evaluation.md`（英語）を `/mktrans` で反映する。

**ユーザーガイド（toml_config、F-005 §1.4）**

- [ ] `docs/user/toml_config/` 配下を横断検索し、PM／systemctl を `risk_level=medium`／既定で使う
  実践例（`09_practical_examples.{ja,md}` 等）を `high` へ更新する（日英両方）。

**sample config（AC-14）**

- [ ] `sample/risk-based-control.toml` L67 の `apt update` を `risk_level = "medium"` →
  `risk_level = "high"` に変更する。
- [ ] `sample/risk-based-control.toml` L103 の `systemctl restart` を `risk_level = "medium"` →
  `risk_level = "high"` に変更する。
- [ ] `sample/timeout_examples.toml` の `system_update_index`（apt update）ブロックの
  `args = ["update"]` 行の直後に `risk_level = "high"` 行を追加する。
- [ ] `sample/timeout_examples.toml` の `system_upgrade`（apt upgrade -y）ブロックの
  `args = ["upgrade", "-y"]` 行の直後に `risk_level = "high"` 行を追加する。
- [ ] `sample/` 配下を横断検索し、上記以外に PM／systemctl を含む config が無いことを確認する
  （見つかれば同様に `risk_level="high"` を付与）。

**完了基準**: §後述の cross-search でサブコマンド粒度の残存記述ゼロ。
`go test -tags test ./internal/runner/config/...` が緑（sample ロード回帰）。

### Phase 6: 緑化（NF-002）

- [ ] `make fmt` → `make test` → `make lint` をすべて成功させる。

---

## 3. 実装順序とマイルストーン

アーキテクチャ §8 のフェーズ順に従う（Phase 1→6）。

| マイルストーン | 内容 | 成果物 |
|---|---|---|
| M1 | Phase 1-3 | 判定本体の名マッチ化＋呼び出し元追従＋旧ファイル削除（ビルド通過） |
| M2 | Phase 4 | テスト改訂（`make test` 緑、全 AC のテストが存在） |
| M3 | Phase 5 | 文書・sample 整合（日英・cross-search クリア） |
| M4 | Phase 6 | `make test && make lint` 緑（PR ゲート） |

PR 分割は実装着手時に判断する（最小は M1-M2 を 1 PR、M3 を 1 PR）。

---

## 4. テスト戦略

詳細は [02_architecture.md](02_architecture.md) §7 を参照。要点:

- **単体（security）**: `TestSystemModificationRisk` で名集合・固定レベル・args 非依存・symlink を網羅。
- **統合（risk/evaluator）**: 実行時＝dry-run 共有評価器（`TestConsistency_Systemctl`）、ラッパー／特権
  （AC-08）、監査理由（AC-09）。
- **回帰（config）**: `template_backward_compat_test.go` による sample ロード成功。
- 既存で結論不変のテスト（service High、mount/crontab Medium、mkfs.ext4 High）は変更しない。

---

## 5. リスク管理

| リスク | 対策 |
|---|---|
| §3.5（arch）のテスト一覧が不完全で、見落としたテストが赤くなる | 本計画 §1.3 で全数を再列挙済み。`make test` を各 Phase で実行し未列挙の赤を検出。 |
| 旧シンボルの残存参照（コメント・文書） | NF-001 cross-search（§6）で網羅確認。 |
| 日英文書の不整合 | `/mktrans` で `.ja.md`→`.md` を反映（直接両方編集しない）。 |
| sample 変更による config テスト破壊 | `risk_level` は `ParseRiskLevel` で `high` が有効。ロード回帰テストで確認。 |

---

## 6. クロスサーチチェックリスト（`make lint`/`make test` で検出できない項目のみ）

- [ ] **NF-001 残存参照**: 次がコード・コメント・テスト・文書に残らない（`rg` でゼロ）。
  `rg -n "SystemctlSubcommandRisk|firstSystemctlSubcommand|systemctlChangeVerbs|systemctlReadOnlyVerbs|flagStyleManagers|flagRule|isFlagStyleModification|matchesShortFlag|packageModifyingVerbs|packageManagerNames|systemModificationCommandNames|isSystemModificationByNames" -g '!docs/tasks/0139_**'`
  期待: マッチ無し（0139 タスク文書内の経緯記述は除外）。
- [ ] **旧シグネチャ残存**: `rg -n "SystemModificationRisk\([^)]*,\s*\w*[Aa]rgs" -g '*.go'` 期待: マッチ無し。
- [ ] **文書のサブコマンド粒度記述**: `rg -n "read-only|サブコマンド|status/show|verb" docs/user/risk_assessment.ja.md docs/user/risk_assessment.md docs/dev/architecture_design/command-risk-evaluation.ja.md docs/dev/architecture_design/command-risk-evaluation.md`
  期待: systemctl/PM のリスク粒度に関する記述が残らない（coreutils 等の無関係な箇所は対象外）。
- [ ] **用語集**: 該当語なし（アーキテクチャ §3.4 で確認済み）。追加作業不要。

---

## 7. 受け入れ基準の検証（AC トレーサビリティ）

| AC | 種別 | 検証場所／コマンドと期待結果 |
|---|---|---|
| AC-01（PM 名→High、dpkg/rpm 含む） | test | `internal/runner/base/security/command_analysis_test.go::TestSystemModificationRisk`（apt/.../dpkg/rpm が High） |
| AC-02（名前が引数値のみ→非該当） | test | 同 `::TestSystemModificationRisk`（`echo` 名集合に pm 名なし→Unknown、非該当名が High/Medium を受けない） |
| AC-03（systemctl 全 args→High） | test | `command_analysis_test.go::TestSystemModificationRisk`（systemctl=High）＋ `risk/evaluator_test.go::TestEvaluateRisk_SystemctlSubcommandConditional`（status/show=High） |
| AC-04（service→High） | test | `risk/evaluator_test.go::TestEvaluateRisk_ServiceAllActionsHigh`、`::TestEvaluateRisk_SystemctlChangeAndServiceHigh` |
| AC-05（args 非参照） | test, static | test: `::TestSystemModificationRisk`（同 names・異 args で同結果）。static: `rg -n "func SystemModificationRisk\(names map\[string\]struct\{\}\) " internal/runner/base/security/command_analysis.go` 期待: 1 件（args 引数なし） |
| AC-06（Medium 集合維持） | test | `::TestSystemModificationRisk`（mount/.../update-rc.d=Medium）＋ `risk/evaluator_test.go::TestEvaluateRisk_NoProfileAbsolutePath`（mount/crontab=Medium） |
| AC-07（実行時＝dry-run） | test | `internal/runner/base/risk/coreutils_consistency_test.go::TestConsistency_Systemctl`（systemctl status=High、共有評価器） |
| AC-08（ラッパー High／特権 Critical） | test | `internal/runner/base/security/indirect_execution_test.go`（`env dpkg`/`env systemctl`→IndirectFloor High）＋ `risk/evaluator_test.go`（`sudo dpkg`/`sudo systemctl`→Critical） |
| AC-09（直接 sysmod deny の理由コード） | test | `risk/evaluator_test.go`（deny 時 `Assessment.ReasonCodes` に `risktypes.ReasonSystemModification`） |
| AC-10（移行ノート） | static | `rg -n "high|移行|baseline|従来 (Low|Medium)" docs/user/risk_assessment.ja.md` 期待: 表示/照会系が High へ上がる移行記述が存在 |
| AC-11（risk_assessment 更新） | static | `rg -n "medium.*floor|read-only" docs/user/risk_assessment.ja.md docs/user/risk_assessment.md` 期待: systemctl read-only=medium の記述が残らない（coreutils 無関係箇所を除く） |
| AC-12（検出限界明記） | static | `rg -n "apk|snap|flatpak|busybox|リネーム" docs/user/risk_assessment.ja.md` 期待: 検出限界の記述が存在 |
| AC-13（0137/systemctl 撤回の記録） | static | `rg -n "0137|撤回|置換|サブコマンド" docs/user/risk_assessment.ja.md docs/dev/architecture_design/command-risk-evaluation.ja.md` 期待: 撤回／固定レベル化の記述が存在（設計は 02_architecture.md §3.5 に記載済み） |
| AC-14（sample config 整合） | test, static | test: `internal/runner/config/template_backward_compat_test.go`（risk-based-control.toml／timeout_examples.toml ロード成功）。static: `rg -n "cmd = \"apt\"" -A4 sample/risk-based-control.toml sample/timeout_examples.toml \| rg "risk_level = \"high\""` 期待: 該当 apt コマンドの直近に `risk_level = "high"`（`risk_level` は `args` 行の直後に置く） |
| NF-001（撤去シンボル残存ゼロ） | static | §6 の NF-001 cross-search コマンド。期待: マッチ無し |
| NF-002（緑ゲート） | static | `make test && make lint` 期待: 成功 |

---

## 8. 実装チェックリスト（フェーズ別）

各 Phase の詳細チェックボックスは §2 にある。フェーズ完了の総括は以下で追跡する。

- [ ] Phase 1: 判定本体の名マッチ固定レベル化（§2 Phase 1 の全項目）
- [ ] Phase 2: 呼び出し元のシグネチャ追従（§2 Phase 2 の全項目）
- [ ] Phase 3: 旧機構ファイルの削除（§2 Phase 3 の全項目）
- [ ] Phase 4: テスト改訂（§2 Phase 4 の全項目）
- [ ] Phase 5: 文書・config の整合（§2 Phase 5 の全項目）
- [ ] Phase 6: 緑化 `make fmt`/`make test`/`make lint`（§2 Phase 6）
- [ ] §6 クロスサーチチェックリストの全項目クリア
- [ ] §7 の全 AC（AC-01〜AC-14、NF-001/002）が検証済み

## 9. 成功基準

- **機能完全性**: AC-01〜AC-14 がすべて §7 の対応テスト／静的検証で確認できる。
- **品質**: `make test && make lint` が緑（NF-002）。新規 `TestSystemModificationRisk` が
  名集合・固定レベル・args 非依存・symlink を網羅する。
- **撤去の完全性**: §6 NF-001 cross-search で撤去シンボルの残存参照がゼロ。
- **文書整合**: ユーザー／開発者文書・sample config が実装と一致し、サブコマンド粒度の残存記述が
  ない。日英文書が `/mktrans` で整合している。

## 10. 次のステップ

- 本計画のレビュー後、Status を `approved` に更新（レビュアー）。
- `approved` 後、Phase 1 から実装を開始（`/runplan`）。
- 実装中は本計画のチェックボックスを随時更新する。
