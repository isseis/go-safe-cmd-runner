# システム変更リスク判定の粗粒度化（バイナリ名マッチへの単純化） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-18 |
| Review date | 2026-06-18 |
| Reviewer | isseis |
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
- **完全な緑ゲート（`make test`／`make lint`）は Phase 4 完了時以降に適用する**。Phase 1〜3 では
  プロダクションコードと既存テストの整合が一時的に崩れる（例: Phase 1 で `isSystemModificationByNames`
  を削除しても `command_analysis_test.go` は Phase 4 まで旧 API を呼ぶ。Phase 3 で `systemctl.go` を
  削除しても `systemctl_test.go` は Phase 4 まで残る）。`make test` は `go test -tags test ./...` で
  テストもコンパイルするため中間フェーズでは赤になる。したがって Phase 1〜3 の完了基準には**各 Phase に
  明記したビルドゲート（`go build`、テストファイルを含まない）**を用い、`make fmt` は随時実行してよい。
  `make test`／`make lint` の通過は Phase 4（テスト改訂完了＝PR-1 の緑ゲート）で確認し、以後の各 PR
  （PR-2／PR-3）でも PR 作成前に再確認する。

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
| `TestIsSystemModification_AbsolutePath` | 同 L1108 | ヘルパ経由 | 削除し、新 `TestSystemModificationRisk` の symlink/絶対パスケースで代替（Phase 4） |
| `TestIsSystemModification_PackageManagerVerbs` | 同 L1117 | `apt list`/`pacman -Q`=false | 削除し、新 `TestSystemModificationRisk` で代替（照会系も High に変わる）（Phase 4） |
| `TestIsSystemModification` | 同 L1161 | `apt list`=false 等 | 削除し、新 `TestSystemModificationRisk` で代替（Phase 4） |
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
  Medium の記述。EN は L291 付近）。`docs/dev/architecture_design/security-architecture.ja.md` / `.md`
  （L445「Medium risk: … package management (apt, yum)」のコマンドリスク分類）。
- ユーザーガイド: `docs/user/toml_config/09_practical_examples.ja.md` / `.md` ほか `toml_config/`
  配下の PM／systemctl 実践例（`risk_level=medium`・既定・**`risk_level=low`** の箇所すべて。
  systemctl は常に High になるため `low` 例も拒否される）。
- README: `README.ja.md` / `README.md`（`systemctl status` を `risk_level = "medium"` とする実践例
  L231-237 付近、リスク区分一覧 L444-448 のパッケージ管理=Medium 記述）。
- sample: `sample/risk-based-control.toml`（apt update=medium L67、systemctl restart=medium L103）、
  `sample/timeout_examples.toml`（apt update/upgrade、`risk_level` 無し L72-73/79-80）。
- sample のロード検証は `internal/runner/config/template_backward_compat_test.go`
  （L24-25 で両 sample を列挙）が担う。`risk_level` 値は load 時に `ParseRiskLevel` で検証される
  （`high` は有効値）ため、変更後もロード成功を回帰確認できる。

---

## 2. 実装ステップ

### Phase 1: 判定本体の名マッチ固定レベル化（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go)）

**作業内容**

- [x] `highSystemModificationNames`（PM 12 種：apt/apt-get/yum/dnf/zypper/pacman/brew/pip/npm/yarn/
  **dpkg/rpm** ＋ systemctl/service）を定義する（アーキテクチャ §3.1 の定義に一致させる）。
- [x] `mediumSystemModificationNames`（chkconfig/update-rc.d/mount/umount/fdisk/parted/mkfs/fsck/
  crontab/at/batch）を定義する。
- [x] `SystemModificationRisk` を新シグネチャ `func SystemModificationRisk(names map[string]struct{}) runnertypes.RiskLevel`
  へ書き換える（§3.2）。判定順: high 集合一致→High、medium 集合一致→Medium、いずれも非該当→Unknown。
  `anyNameInSet` を再利用する。
- [x] `isSystemModificationByNames`／`packageModifyingVerbs`／`packageManagerNames`／
  `systemModificationCommandNames` を削除する。
- [x] `SystemModificationRisk` の doc コメントを新仕様（引数非依存・固定レベル）へ更新する（英語）。
- [x] **同一 `security` パッケージ内の呼び出し元**
  [indirect_execution.go](../../../internal/runner/base/security/indirect_execution.go) L840 を
  `SystemModificationRisk(innerNames)` へ追従する。シグネチャ変更は同パッケージを一括コンパイルする
  ため、この追従を Phase 1 内で行わないと `security` パッケージのビルドが必ず赤になる（呼び出し元が
  旧シグネチャのまま残るため）。`innerArgs` がこの行で未使用化しても他で使われているため未使用変数
  エラーは生じないことを確認する。

**完了基準**: `security` パッケージのビルドが通る（`go build -tags test ./internal/runner/base/security/`。
同パッケージ呼び出し元の追従込み）。旧シンボル参照が同ファイル内に残らない。

### Phase 2: 別パッケージ呼び出し元のシグネチャ追従

**作業内容**

- [x] [evaluator.go](../../../internal/runner/base/risk/evaluator.go) L231（`risk` パッケージ）を
  `security.SystemModificationRisk(names)` へ変更する。

**完了基準**: `go build -tags test ./...` が通る。

### Phase 3: 旧機構ファイルの削除（NF-001）

**作業内容**

- [x] [systemctl.go](../../../internal/runner/base/security/systemctl.go) を削除する。
- [x] [package_manager_flags.go](../../../internal/runner/base/security/package_manager_flags.go) を削除する。
- [x] 上記削除に伴い `firstOperand`（indirect_execution.go）の `reliable` 戻り値が
  全呼び出し元で未使用になる（systemctl.go が唯一の利用者だった）。`unparam` 検出を解消するため
  `firstOperand` の戻り値を `operand string` のみへ簡素化し、`coreutils.go`／`command_risk_profile.go`
  の 2 呼び出し元を追従する（削除の機械的帰結。挙動不変：unreliable 時は従来どおり "" を返す）。

**完了基準**: `go build -tags test ./...` が通る。**プロダクションコードのみ**で旧シンボル残存ゼロ
（`rg -n "SystemctlSubcommandRisk|flagStyleManagers|packageModifyingVerbs|isSystemModificationByNames|packageManagerNames|systemModificationCommandNames" internal/ cmd/ -g '!**/*_test.go'` 期待: マッチ無し）。
**テストファイルを除外する理由**: Phase 3 時点では `systemctl_test.go` や `command_analysis_test.go` の
ヘルパが旧シンボル（`SystemctlSubcommandRisk`／`isSystemModificationByNames` 等）をまだ参照しており
（削除は Phase 4）、`_test.go` を含めると本ゲートが達成不能になるため。テストを含む完全な
NF-001 cross-search（§6、`_test.go` 込み・文書込み）は、テスト改訂（Phase 4）と文書更新（Phase 5）が
済んだ PR-1／PR-2 の段階で確認する。

### Phase 4: テスト改訂

**新規・改訂テスト**

- [x] security/command_analysis_test.go に `TestSystemModificationRisk` を新設する。`SystemModificationRisk`
  を直接呼び、以下を表明する:
  - High: `apt`/`apt-get`/`yum`/`dnf`/`zypper`/`pacman`/`brew`/`pip`/`npm`/`yarn`/`dpkg`/`rpm`、
    `systemctl`、`service`（引数有無・install/照会の両方で High。AC-01/03/04）。
  - Medium: `mount`/`umount`/`fdisk`/`parted`/`mkfs`/`fsck`/`crontab`/`at`/`batch`/`chkconfig`/
    `update-rc.d`（AC-06）。
  - Unknown: 非該当名（`echo`/`ls`）、および `names` に pm 名を含まないケース（AC-02）。
  - symlink/絶対パス: `/usr/sbin/systemctl` は High、`systemctl-helper` は非該当（AC-01）。
  - 注: `SystemModificationRisk` は引数を取らない新シグネチャのため、本関数に args を渡す形の
    「args 非依存」テストは書けない（書くと撤去した引数を復活させてしまう）。AC-05 は「引数を
    参照しないこと」を**シグネチャの静的確認**（§7 AC-05 の static チェック）で担保し、加えて
    evaluator レベルで**同一コマンド・異なる引数**（例: `apt install nginx` と `apt list`）が
    同一 High になることを `TestStandardEvaluator_EvaluateRisk_SystemModifications` に追加して
    補強する。
- [x] security/command_analysis_test.go の `isSystemModification` ヘルパ（L1101）を削除する。
- [x] `TestIsSystemModification_AbsolutePath`（L1108）を削除する（`TestSystemModificationRisk` の
  symlink/絶対パスケースで代替）。
- [x] `TestIsSystemModification_PackageManagerVerbs`（L1117）を削除する（同上で代替。照会系=High に変わる）。
- [x] `TestIsSystemModification`（L1161）を削除する（`TestSystemModificationRisk` で代替）。
- [x] security/systemctl_test.go を削除する（`TestFirstSystemctlSubcommand`／`TestSystemctlSubcommandRisk`
  の 2 関数を含む。両関数の不変条件は `TestSystemModificationRisk` の systemctl=High と
  evaluator 統合テストで代替）。
- [x] risk/evaluator_test.go `TestStandardEvaluator_EvaluateRisk_SystemModifications`（L111-112）の
  `apt install`／`yum install` 期待値を `RiskLevelMedium` → `RiskLevelHigh` へ変更する。あわせて
  `{"apt list (query)", "apt", []string{"list", "--installed"}, RiskLevelHigh}` ケースを追加する
  （`apt install` と同一コマンド・異なる引数で同一 High ＝ AC-05 の引数非依存を evaluator レベルで補強）。
- [x] risk/evaluator_test.go `TestStandardEvaluator_EvaluateRisk_SafeCommands`（L131）の
  `{"apt list (query)", "apt", []string{"list", "--installed"}}` ケースを削除する（apt は High に
  なり Low 前提が崩れるため。上記 SystemModifications へ High として移設。AC-01/10）。
- [x] risk/evaluator_test.go `TestEvaluateRisk_SystemctlSubcommandConditional`（L190-191）の
  `systemctl status`／`systemctl show` 期待値を `RiskLevelMedium` → `RiskLevelHigh` へ変更し、
  テスト名・コメントの「read-only は Medium floor」記述を「常に High」へ更新する（AC-03）。
- [x] risk/coreutils_consistency_test.go `TestConsistency_Systemctl`（L218）の `systemctl status`
  期待値を `RiskLevelMedium` → `RiskLevelHigh` へ変更し、コメントを更新する（AC-07）。
- [x] AC-09 検証テストを 2 層で追加する。AC-09 は「deny 時の**監査ログ**に system_modification 系
  理由コードが記録される」ことを要求するが、実際の `risk_level` 比較と deny／監査は evaluator ではなく
  `NormalResourceManager`／`DryRunResourceManager` で行われる。したがって:
  - (a) **dimension→理由コード**: `risk/evaluator_test.go` で、直接実行の sysmod コマンドの
    `plan.Assessment.ReasonCodes` に `risktypes.ReasonSystemModification` が含まれることを表明
    （名前→理由コードの結線確認）。
  - (b) **deny+監査**: `internal/runner/resource/audit_wiring_test.go` の既存パターン
    （`TestExecute_RejectedCommandAuditable`、`fixedPlanEvaluator` で deny plan を注入）に倣い、
    `ReasonCodes` に `risktypes.ReasonSystemModification` を持つ deny plan で監査エントリが
    `decision=deny` となり、その理由コード（監査ログに出力される reason コード群。logger.go が
    `ReasonCodes` を出力）に `system_modification` が含まれることを表明する。実際のフィールド名・
    `blocking_reason` の値は既存テスト（同ファイル）の表明形式に合わせる。これにより deny／監査の
    主張を統一 deny 経路で実証する。
- [x] AC-08 回帰テストを追加する（本タスクで結論は不変だが明示固定）。security/indirect_execution_test.go に
  `analyzeIndirectCmd("env", "dpkg", "-i", "pkg.deb")` と `("env", "systemctl", "restart", "nginx")`
  が `IndirectFloor` かつ `Level == RiskLevelHigh` を、risk/evaluator_test.go に
  `sudo dpkg`／`sudo systemctl restart` が `RiskLevelCritical` を表明する。

**完了基準**: `make fmt` → `make test` → `make lint` がすべて緑（コード PR の NF-002 ゲート。
Phase 1〜4 はこの 1 PR にまとめる。中間フェーズ単独では既存テストが旧 API を参照して赤になるため
分割できない。§1.2 参照）。

### PR-1 作成ポイント: internal risk classification (code + tests)

**対象ステップ**: Phase 1 / Phase 2 / Phase 3 / Phase 4

**推奨タイトル**: `feat(0139): coarse name-only system-modification risk classification`

**レビュー観点**: 名集合の正確性（PM 12 種＋systemctl/service=High・dpkg/rpm 追加・Medium 集合維持） / シグネチャ変更と全呼び出し元・全影響テストの追従（§1.3 のテスト全数） / 旧機構（systemctl.go・package_manager_flags.go）の完全撤去とライブコード NF-001

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: 文書整合

**ユーザー文書（AC-10/11/12/13）**

- [ ] `docs/user/risk_assessment.ja.md` の systemctl/PM サブコマンド粒度記述（read-only=medium 等）を、
  固定レベル（PM／systemctl いずれも `high`）へ更新する。移行ノート（AC-10、baseline=直近リリース）には
  **以下の引き上げをすべて**記載する: (a) 表示・照会系の PM 呼び出し（従来 Low）→High、
  (b) **パッケージ変更操作（apt install/update 等、0137 後は Medium）→High**、
  (c) **`dpkg`/`rpm`（従来 Low／未検出）→High**、(d) systemctl read-only（従来 Medium）→High。
  あわせて従来 `medium` で許可していた PM install/update config がブロックされ得る旨を明記する。
  検出限界（apk/snap/flatpak/gem・リネーム・busybox 形式、AC-12）、0137/systemctl 粒度の撤回（AC-13）も記載する。
- [ ] `docs/user/risk_assessment.md`（英語）を `/mktrans` で `.ja.md` から反映する
  （バイリンガル編集順序: ja を先に確定）。
- [ ] `README.ja.md` の実践例（`systemctl status` を `risk_level = "medium"` とする箇所、L231-233 付近）を
  `high` へ更新し、リスク区分一覧（L444-445「中: …パッケージ管理／高: システム管理(systemctl)」）から
  パッケージ管理を Medium 区分の例から外し High 側へ移す。`README.md` を `/mktrans` で反映する。

**開発者文書（F-005 §1.4）**

- [ ] `docs/dev/architecture_design/command-risk-evaluation.ja.md` の「System Modification Risk」節を、
  `SystemModificationRisk(names)`（引数非依存）・名マッチ固定レベル（PM／systemctl=High）へ更新し、
  `SystemctlSubcommandRisk`・verb-only Medium の記述を削除する。
- [ ] `docs/dev/architecture_design/command-risk-evaluation.md`（英語）を `/mktrans` で反映する。
- [ ] `docs/dev/architecture_design/security-architecture.md`（L445「Medium risk: … package management
  (apt, yum)」）のコマンドリスク分類記述を、パッケージマネージャ=High へ更新する。`.ja.md` も同様に更新する
  （systemctl は同節で既に High 例として記載済み）。

**ユーザーガイド（toml_config、F-005 §1.4）**

- [ ] `docs/user/toml_config/` 配下を横断検索し、PM／systemctl を使う実践例を `high` へ更新する
  （日英両方）。**`risk_level=medium`／既定（無設定）だけでなく `risk_level=low` の systemctl/PM 例も
  対象に含める**（systemctl は常に High になるため、`systemctl status` を `low` とする既存テンプレート
  も以後拒否される）。検索は `rg -n "systemctl|/usr/bin/apt|apt-get|\"apt\"|dpkg|rpm|pacman" docs/user/toml_config/`
  で該当行を洗い出し、各 risk_level を確認する。

**完了基準**: 文書のみで NF-001 残存ゼロ＋ stale な挙動記述ゼロ（§6 の NF-001／stale 記述チェックを
ライブコードと併せて全対象で実施し、撤回ノートのみ許容）。日英文書が `/mktrans` で整合。`make lint`
が緑（文書のみのため `make test` は不変だが念のため `make test` も緑であることを確認）。

### PR-2 作成ポイント: documentation alignment

**対象ステップ**: Phase 5（文書整合）

**推奨タイトル**: `docs(0139): align risk docs with name-only system-modification levels`

**レビュー観点**: 移行ノートの網羅（AC-10 の 4 引き上げ：照会 PM／PM 変更操作／dpkg・rpm／systemctl read-only） / 検出限界（AC-12、gem 含む）と 0137 撤回記録（AC-13）の記載 / 日英整合（`/mktrans`）と stale な現行仕様記述の不残存（文書込み NF-001）

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 6: sample config 整合

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

**完了基準**: `go test -tags test ./internal/runner/config/...` が緑（sample ロード回帰、AC-14）。
`make test`／`make lint` が緑。

### PR-3 作成ポイント: sample config alignment

**対象ステップ**: Phase 6（sample config 整合）

**推奨タイトル**: `chore(0139): raise risk_level to high for PM/systemctl sample configs`

**レビュー観点**: 引き上げ対象コマンドの網羅（apt update／apt upgrade／systemctl restart、横断検索での取りこぼし無し） / `risk_level = "high"` の値・配置の正しさ（`args` 行直後） / config ロード回帰（`template_backward_compat_test.go`）が緑

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

本計画は、アーキテクチャ §8 の実装優先順位（Phase 1→6）を**PR 境界向けに具体化**したものである。
アーキテクチャ §8 が 1 つの Phase 5 にまとめていた「文書・config」を、独立レビュー可能性のため
本計画では **Phase 5（文書整合）と Phase 6（sample config 整合）に分割**し、アーキテクチャ §8 の
独立した「緑化」Phase は**各 PR の緑ゲートへ畳み込んだ**（Phase 1→6 の優先順位の方向性は維持しつつ、
Phase 5 の分割と緑化フェーズの畳み込みのみ調整）。Phase 1〜4 は
緑ゲートの都合で 1 PR にまとめ（中間フェーズ単独では既存テストが旧 API を参照して赤）、文書と
sample config を後続 PR に分ける（§3.2）。cmd/ 変更は無く、変更は internal のみのため
internal-before-cmd の制約は非該当。

| マイルストーン | 対応 PR（Phase） | 成果物 |
|---|---|---|
| M1 | PR-1（Phase 1-4） | 名マッチ分類の本体＋全テスト改訂（`make test && make lint` 緑、全 AC のコードテストが存在） |
| M2 | PR-2（Phase 5） | 文書整合（日英・文書込み NF-001／stale 記述クリア） |
| M3 | PR-3（Phase 6） | sample config 整合（config ロード回帰緑、AC-14） |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 |
|---|---|---|
| PR-1 | Phase 1 / Phase 2 / Phase 3 / Phase 4 | `SystemModificationRisk` の名マッチ固定レベル化（`args` 撤去）、呼び出し元追従（同/別パッケージ）、`systemctl.go`・`package_manager_flags.go` 削除、全影響テストの改訂・新規（`TestSystemModificationRisk` 等）。internal のみ・分割不可（緑ゲートが Phase 4 完了でのみ通る）。 |
| PR-2 | Phase 5（文書整合） | risk_assessment／command-risk-evaluation／security-architecture／README／toml_config の日英整合（移行ノート・検出限界・0137 撤回記録）。 |
| PR-3 | Phase 6（sample config 整合） | `risk-based-control.toml`／`timeout_examples.toml` の該当 PM／systemctl エントリを `risk_level = "high"` へ追従。 |

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
| §3.5（arch）のテスト一覧が不完全で、見落としたテストが赤くなる | 本計画 §1.3 で全数を再列挙済み。Phase 4 完了時の `make test` で未列挙の赤を検出（中間フェーズはビルドゲートのみ。§1.2）。 |
| 旧シンボルの残存参照（コメント・文書） | NF-001 cross-search（§6）で網羅確認。 |
| 日英文書の不整合 | `/mktrans` で `.ja.md`→`.md` を反映（直接両方編集しない）。 |
| sample 変更による config テスト破壊 | `risk_level` は `ParseRiskLevel` で `high` が有効。ロード回帰テストで確認。 |

---

## 6. クロスサーチチェックリスト（`make lint`/`make test` で検出できない項目のみ）

- [ ] **NF-001 残存参照**: 次が**ライブコードとユーザー／開発者文書**に残らない（`rg` でゼロ）。
  検索対象は `internal/`・`cmd/`・`docs/user/`・`docs/dev/` に限定する。`docs/tasks/` 配下は
  0136/0137/0139 等の**歴史的タスク記録**が当該シンボル名を含む（経緯として正当に残る）ため、
  検索対象から除外する。
  `rg -n "SystemctlSubcommandRisk|firstSystemctlSubcommand|systemctlChangeVerbs|systemctlReadOnlyVerbs|flagStyleManagers|flagRule|isFlagStyleModification|matchesShortFlag|packageModifyingVerbs|packageManagerNames|systemModificationCommandNames|isSystemModificationByNames" internal/ cmd/ docs/user/ docs/dev/`
  期待: マッチ無し。
- [ ] **旧シグネチャ残存**: `rg -n "SystemModificationRisk\([^)]*,\s*\w*[Aa]rgs" -g '*.go'` 期待: マッチ無し。
- [ ] **stale な挙動記述の残存**: 「systemctl read-only を medium（下限）扱いする」「install/remove 系
  verb のみ Medium とする」といった**旧挙動を現行仕様として述べる記述**が残らないこと。
  `rg -n "read-only" docs/user/risk_assessment.ja.md docs/user/risk_assessment.md docs/dev/architecture_design/command-risk-evaluation.ja.md docs/dev/architecture_design/command-risk-evaluation.md`
  の各ヒットを確認し、systemctl read-only=medium／floor を**現行仕様として**述べる行が無いこと。
  **注意**: AC-13 が要求する撤回ノート（「0137 のフラグ／verb 方式と systemctl サブコマンド粒度を
  撤回した」等、`サブコマンド`/`verb` の語を含む経緯説明）はこのチェックの**対象外（残してよい）**。
  本チェックは「旧挙動を現行として述べる記述」だけを排除し、過去形の撤回ノートは許容する。
- [ ] **用語集**: 該当語なし（アーキテクチャ §3.4 で確認済み）。追加作業不要。

---

## 7. 受け入れ基準の検証（AC トレーサビリティ）

| AC | 種別 | 検証場所／コマンドと期待結果 |
|---|---|---|
| AC-01（PM 名→High、dpkg/rpm 含む） | test | `internal/runner/base/security/command_analysis_test.go::TestSystemModificationRisk`（apt/.../dpkg/rpm が High） |
| AC-02（名前が引数値のみ→非該当） | test | 同 `::TestSystemModificationRisk`（`echo` 名集合に pm 名なし→Unknown、非該当名が High/Medium を受けない） |
| AC-03（systemctl 全 args→High） | test | `command_analysis_test.go::TestSystemModificationRisk`（systemctl=High）＋ `risk/evaluator_test.go::TestEvaluateRisk_SystemctlSubcommandConditional`（status/show=High） |
| AC-04（service→High） | test | `risk/evaluator_test.go::TestEvaluateRisk_ServiceAllActionsHigh`、`::TestEvaluateRisk_SystemctlChangeAndServiceHigh` |
| AC-05（args 非参照） | static, test | static: `rg -n "func SystemModificationRisk\(names map\[string\]struct\{\}\) runnertypes.RiskLevel" internal/runner/base/security/command_analysis.go` 期待: 1 件（引数に args が無く、構造的に参照不可）。test: `risk/evaluator_test.go::TestStandardEvaluator_EvaluateRisk_SystemModifications`（`apt install` と `apt list` が同一 High ＝ 引数非依存） |
| AC-06（Medium 集合維持） | test | `::TestSystemModificationRisk`（mount/.../update-rc.d=Medium）＋ `risk/evaluator_test.go::TestEvaluateRisk_NoProfileAbsolutePath`（mount/crontab=Medium） |
| AC-07（実行時＝dry-run） | test | `internal/runner/base/risk/coreutils_consistency_test.go::TestConsistency_Systemctl`（systemctl status=High、共有評価器） |
| AC-08（ラッパー High／特権 Critical） | test | `internal/runner/base/security/indirect_execution_test.go`（`env dpkg`/`env systemctl`→IndirectFloor High）＋ `risk/evaluator_test.go`（`sudo dpkg`/`sudo systemctl`→Critical） |
| AC-09（直接 sysmod deny の監査理由） | test | `risk/evaluator_test.go`（Assessment.ReasonCodes に `ReasonSystemModification`）＋ `internal/runner/resource/audit_wiring_test.go`（`ReasonSystemModification` を持つ deny plan で監査エントリが `decision=deny` となり理由コードに `system_modification` が含まれる。deny／監査は resource manager 層で行われるため、評価器の assessment だけでは不十分） |
| AC-10（移行ノート） | static | `rg -n "従来" docs/user/risk_assessment.ja.md` 期待: 移行差分に**4 つの引き上げがすべて**記載される — (a) 表示・照会系 PM（従来 Low）→High、(b) **PM 変更操作 install/update（0137 後 Medium）→High**、(c) **dpkg/rpm（従来 Low）→High**、(d) systemctl read-only（従来 Medium）→High。従来 `medium` の PM install config がブロックされ得る旨も明記（baseline=直近リリース。単なる `high` 一致では不可、移行文を確認する） |
| AC-11（risk_assessment 更新） | static | `rg -n "read-only" docs/user/risk_assessment.ja.md docs/user/risk_assessment.md` 期待: systemctl read-only を medium（下限）とするサブコマンド粒度記述が残らない（coreutils 等の無関係箇所は対象外） |
| AC-12（検出限界明記） | static | 各語 `apk`／`snap`／`flatpak`／`gem`／`busybox`／リネーム を**個別に**確認し全語がヒットすること（OR 一括不可、全語必須）。`gem` は `gemini` への誤一致を避けるため**バッククォート付き／語境界**で照合する（例 `` rg -q '`gem`' docs/user/risk_assessment.ja.md `` または `rg -qP '\bgem\b(?!ini)'`）。他の語は `rg -q "<語>" docs/user/risk_assessment.ja.md` |
| AC-13（0137/systemctl 撤回の記録） | static | `rg -n "0137" docs/user/risk_assessment.ja.md docs/dev/architecture_design/command-risk-evaluation.ja.md` 期待: 0137 フラグ方式・systemctl サブコマンド粒度の撤回／固定レベル化の記述がヒット（設計は 02_architecture.md §3.5 に記載済み） |
| AC-14（sample config 整合） | test, static | test: `internal/runner/config/template_backward_compat_test.go`（risk-based-control.toml／timeout_examples.toml ロード成功）。static: `rg -n -A4 "^cmd = " sample/risk-based-control.toml sample/timeout_examples.toml` の出力で、**apt update／apt upgrade／systemctl restart** の各エントリに `risk_level = "high"` が付くことを確認（apt だけでなく systemctl も含む） |
| NF-001（撤去シンボル残存ゼロ） | static | §6 の NF-001 cross-search コマンド。期待: マッチ無し |
| NF-002（緑ゲート） | static | `make test && make lint` 期待: 成功 |

---

## 8. 実装チェックリスト（PR 別）

各 Phase の詳細チェックボックスは §2 に、PR 境界は §2 の各 `### PR-N 作成ポイント` と §3.2 にある。
PR 単位の完了は以下で追跡する。

- [ ] PR-1 マージ済み（対象ステップ: Phase 1 / Phase 2 / Phase 3 / Phase 4。`make test && make lint` 緑、
  ライブコード NF-001 クリア、§7 のコード系 AC が検証済み）
- [ ] PR-2 マージ済み（対象ステップ: Phase 5（文書整合）。文書込み NF-001／stale 記述クリア、日英整合、
  AC-10/11/12/13）
- [ ] PR-3 マージ済み（対象ステップ: Phase 6（sample config 整合）。config ロード回帰緑、AC-14）
- [ ] §6 クロスサーチチェックリストの全項目クリア（コード=PR-1、文書=PR-2 で確認）
- [ ] §7 の全 AC（AC-01〜AC-14、NF-001/002）が検証済み

## 9. 成功基準

- **機能完全性**: AC-01〜AC-14 がすべて §7 の対応テスト／静的検証で確認できる。
- **品質**: `make test && make lint` が緑（NF-002）。新規 `TestSystemModificationRisk` が
  名集合・固定レベル・args 非依存・symlink を網羅する。
- **撤去の完全性**: §6 NF-001 cross-search で撤去シンボルの残存参照がゼロ。
- **文書整合**: ユーザー／開発者文書・sample config が実装と一致し、サブコマンド粒度の残存記述が
  ない。日英文書が `/mktrans` で整合している。

## 10. 次のステップ

- 本計画は承認済み（Status: `approved`）。PR 構成は §3.2 に確定済み。
- PR-1（Phase 1-4）から実装を開始する（`/runplan`）。各 `### PR-N 作成ポイント` の手順に従い、
  PR-1 → PR-2 → PR-3 の順にブランチを切り替えながら進める。
- 実装中は本計画のチェックボックス（§2 各 Phase、§8 PR 別）を随時更新する。
