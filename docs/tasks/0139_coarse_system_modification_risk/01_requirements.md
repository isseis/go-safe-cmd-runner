# システム変更リスク判定の粗粒度化（バイナリ名マッチへの単純化） — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-18 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 1. 背景と目的

### 1.1 課題

タスク 0137（パッケージマネージャのシステム変更検出の一般化, dpkg/rpm フラグ方式対応）で、
パッケージマネージャのサブコマンド/フラグを精緻に分類する機構を導入した
（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go) の
`packageModifyingVerbs`／[package_manager_flags.go](../../../internal/runner/base/security/package_manager_flags.go) の
`flagStyleManagers`・`isFlagStyleModification`、および rpm の照会除外）。systemctl についても
[systemctl.go](../../../internal/runner/base/security/systemctl.go) でサブコマンド（verb）を解析し、
read-only=Medium / change=High / unknown=High と判定している。

設計レビューの結果、**この粒度は go-safe-cmd-runner の脅威モデルに対して過剰**と判断した。

- 本ツールの主目的は「非特権ユーザーへ特権バッチ操作を委譲する」こと。`apt list` / `dpkg -l` /
  `rpm -qa` / `systemctl status` のような**表示・照会系コマンドを単体で呼ぶ用途はほぼ存在しない**。
- サブコマンド単位の区別が意味を持つのは「許可済みバイナリ上で `risk_level` 上限を変更操作検知の
  トリップワイヤにする（引数に変数展開が入り、かつ `risk_level` は信頼 config に固定）」という
  **かなり狭いシナリオ**のみ。実 config がこれに依存していない以上、フラグ判定機構（rpm 照会除外を
  含む）や systemctl の getopt スキャナの**コード量・保守コストに見合わない**。
- 実際のセキュリティ制御は **allowlist + ハッシュ固定 + コマンドごとの `risk_level` 上限**で
  担保される（リスク判定は二次ゲート、ブロックリストは非有界）。0136 AC-66/67 と整合。

### 1.2 0137 / systemctl サブコマンド判定との関係

本タスクは、0137 で導入したフラグ方式検出（dpkg/rpm/pacman のフラグ判定・rpm 照会除外）と、
systemctl のサブコマンド粒度判定を **置換（撤回）** する。0137 等の承認済み AC を直接書き換えず、
本要件で方針を上書きする（F-005 / AC-13）。verb 方式の検出（`packageModifyingVerbs`）も同時に
撤去し、パッケージマネージャは一律のバイナリ名マッチへ統一する。

### 1.3 設計判断（要件確定前の意思決定）

要件作成にあたり、以下を決定済み（2026-06-18, isseis）。

| 論点 | 決定 | 根拠 |
|---|---|---|
| 判定方式 | **バイナリ名マッチによる固定リスクレベル**（引数を見ない） | YAGNI。表示/変更の区別は実用上ほぼゼロで保守コスト過大。実制御は allowlist + ハッシュ固定 + per-command `risk_level` 上限が担う。 |
| 対象範囲 | **パッケージマネージャ（verb 方式 + フラグ方式）と systemctl のみ** | サブコマンド/フラグ判定を持つのはこの 2 系統だけ。mount/fdisk 等は既に名マッチで変更不要。 |
| パッケージマネージャのレベル | **常に High** | install/upgrade はメンテナスクリプト・スクリプトレット（dpkg `postinst`／rpm `%post`／pip `setup.py`／npm `postinstall` 等）を**未検証のまま特権実行**し得る。これは systemctl/service の「未検証 unit/init スクリプト実行」と同質で、"arbitrary code execution = High" の分類および fail-safe に整合。粗粒度化でサブコマンドを見ない以上、最悪ケース（install）へ倒して High に統一する。 |
| systemctl のレベル | **常に High** | fail-safe。現状の change verb 水準に合わせる。Medium へ下げると `systemctl start/stop` が緩くなるため不可。`systemctl status` は Medium→High に上がる。 |
| service のレベル | **High 維持** | 既に名マッチ。未検証 init スクリプトを実行するため PM/systemctl と同質・同水準。 |
| High と Medium の境界（原則） | **High = 未検証コードを特権実行し得る系統**（PM／systemctl／service）。**Medium = 定義済み操作でシステム状態を変更する名マッチ系**（mount/mkfs/crontab 等、本タスクのスコープ外で据え置き）。 | 「コードを実行するか／定義済み操作か」で線を引くことで、粗粒度でも一貫した fail-safe 基準になる。 |

### 1.4 目的

パッケージマネージャと systemctl のリスク判定を、サブコマンド/フラグ解析を撤廃して
**バイナリ名のみに依存する固定レベル（いずれも High）** へ単純化する。あわせて関連する
サブコマンド/フラグ機構のコードとテストを撤去し、ユーザー/開発者文書を実装と一致させ、
破壊的変更（後述）を移行ノートで周知する。

## 2. スコープ

- **In**:
  - パッケージマネージャ（apt/apt-get/yum/dnf/zypper/pacman/brew/pip/npm/yarn/dpkg/rpm）の
    一律 High 判定（F-001）。
  - systemctl の一律 High 判定（F-002）。
  - 判定の引数非依存化と、サブコマンド/フラグ判定機構（systemctl.go, package_manager_flags.go,
    packageModifyingVerbs）の撤去（F-003）。
  - 名マッチ系の不変性・実行時/dry-run 一貫性・間接実行/監査の整合（F-004）。
  - 影響を受けるユーザー/開発者文書の整合・破壊的変更の移行周知・検出限界の明記（F-005）。
- **Out**（詳細・根拠は §6）:
  - `IsDestructiveFileOperation`（rm/dd/find -exec 等の引数パターン判定）。
  - `CheckDangerousArgPatterns`（`rm -rf`/`dd if=`/`mkfs.*` 等の危険引数パターン）。
  - mount/umount/fdisk/parted/mkfs/fsck/crontab/at/batch/chkconfig/update-rc.d 等の既存名マッチ系
    （Medium 据え置き。レベル見直しは別タスク）。
  - `RiskLevel` の段階定義（Low/Medium/High/Critical）そのものの変更。

## 3. 機能要件と受け入れ基準

### F-001: パッケージマネージャの粗粒度判定

パッケージマネージャバイナリの呼び出しは、サブコマンド・フラグによらず一律 High として
システム変更（`ReasonSystemModification` 系）に分類する。

**Acceptance Criteria**:
- **AC-01**: 解決済みバイナリ名が apt/apt-get/yum/dnf/zypper/pacman/brew/pip/npm/yarn/dpkg/rpm の
  いずれかであるコマンドは、引数（サブコマンド・フラグ・引数の有無）によらず **High**
  （システム変更次元・`ReasonSystemModification` 系の理由）に分類される。代表例として
  `apt list`・`dpkg -l`・`rpm -qa`・`pacman -Q`・`pip list`・`apt install nginx`・`dpkg -i pkg.deb`・
  `rpm -Uvh pkg.rpm`、および引数なしの `dpkg`／`apt` が、いずれも High となる。
- **AC-02**: パッケージマネージャ名・systemctl が**実行バイナリ（解決済みコマンド名）ではなく
  引数値としてのみ**現れる場合（例: `echo rpm`、`grep systemctl /etc/x`）は、本次元の分類を受けない。
  上記名集合に含まれないバイナリも同様に、本次元から High/Medium を受けない。

### F-002: systemctl の粗粒度判定

**Acceptance Criteria**:
- **AC-03**: 解決済みバイナリ名が systemctl のコマンドは、引数（サブコマンド・引数の有無）によらず
  **High** に分類される。`systemctl status`・`systemctl list-units` などの照会系、および引数なしの
  `systemctl` も High となる。
- **AC-04**: service は **High** を維持する。

### F-003: 判定の引数非依存化と旧機構の撤去

**Acceptance Criteria**:
- **AC-05**: システム変更リスク導出は、パッケージマネージャ・systemctl・service について
  引数（`args`）を参照せず、解決済みバイナリ名のみで決定する。
  （撤去対象シンボルの非残存は NF-001 で担保する。）

### F-004: 一貫性・統合の維持

**Acceptance Criteria**:
- **AC-06**: 既存の名マッチ系コマンド（mount/umount/fdisk/parted/mkfs/fsck/crontab/at/batch/
  chkconfig/update-rc.d）は Medium を維持する。
- **AC-07**: 同一コマンドに対し、実行時（runtime）と dry-run で同一のリスク分類となる。
- **AC-08**: ラッパー/間接実行経由の判定が維持される。例: `env dpkg -i pkg.deb`・
  `env systemctl restart nginx` は High 以上、`sudo dpkg -i pkg.deb`・`sudo systemctl restart nginx` は
  Critical（特権昇格）に分類される。
- **AC-09**: リスク超過により deny されたコマンドの監査ログに、システム変更を示す理由
  （`system_modification` 系の理由コード）が記録される。

### F-005: 文書整合・移行周知・検出限界

**Acceptance Criteria**:
- **AC-10**（破壊的変更）: 表示・照会系のパッケージマネージャ呼び出し（**従来 Low**）・パッケージ
  変更操作（0137 後は Medium）・systemctl read-only（**従来 Medium**）が、いずれも **High** へ
  引き上がり、従来許可していた config がブロックされ得ることを移行ノートとして文書化する。
  **比較基準（baseline）は直近リリースの挙動**とし、未リリースの 0137 を本タスクが撤回する点を
  踏まえて正味の差分を記述する。安全運用は allowlist + ハッシュ固定 + 明示的な `risk_level` 設定が
  前提であることを併記する。
- **AC-11**: [docs/user/risk_assessment.md](../../../docs/user/risk_assessment.md)（および日本語版・
  用語集）から、パッケージマネージャ／systemctl のサブコマンド粒度に依存した記述を削除し、
  固定レベル（パッケージマネージャ=High、systemctl=High）の記述へ更新する。
- **AC-12**（検出限界）: 粗粒度化後も、**未列挙のマネージャ（apk/snap/flatpak/gem 等）・リネーム
  されたバイナリ・multi-call 形式（`busybox <pm>` 等）は本次元で検出されず Low を素通りし得る**こと、
  および安全運用が allowlist + ハッシュ固定を前提とすることを文書化する（0136 AC-66/67 と整合）。
- **AC-13**: 本タスクが 0137 のフラグ方式検出・verb 方式検出、および systemctl サブコマンド粒度を
  置換（撤回）する関係を、本要件および関連文書に記録する。
- **AC-14**: 本変更で分類が引き上がるコマンドを使用する既存の sample／テスト用 config が、新しい
  固定レベル（High）の下でも整合する（必要な `risk_level` 設定が付与されている）よう更新・検証する。

## 4. 非機能要件

- **NF-001**: 撤去後、`SystemctlSubcommandRisk`・`flagStyleManagers`・`packageModifyingVerbs` 等の
  撤去対象シンボルへの参照がコードベースに残らない（テスト含む）。
- **NF-002**: `make test`・`make lint`・`make fmt` がすべて成功する。

## 5. 影響範囲（実装時の参考、本要件では確定事項ではない）

- 削除候補: [systemctl.go](../../../internal/runner/base/security/systemctl.go)、
  [package_manager_flags.go](../../../internal/runner/base/security/package_manager_flags.go)。
- 簡素化候補: [command_analysis.go](../../../internal/runner/base/security/command_analysis.go) の
  システム変更判定（`SystemModificationRisk`／`isSystemModificationByNames`／名集合定義）。
  パッケージマネージャ・systemctl・service を High、その他名マッチ系を Medium とする再構成。
  呼び出し元（[evaluator.go](../../../internal/runner/base/risk/evaluator.go) と wrapped-inner 経路）の
  シグネチャ追従。
- テスト: サブコマンド/フラグ粒度のテスト（フラグ判定・systemctl verb・0137 の照会非検出ケース）を
  撤去し、名マッチ・固定レベルのテストへ置換する。

詳細設計は 02_architecture.md（要件承認後に作成）で確定する。

## 6. スコープ外の根拠

- **破壊的ファイル操作・危険引数パターン（`IsDestructiveFileOperation` / `CheckDangerousArgPatterns`）**:
  これらは「引数の中身」によって危険性が決まる別軸（rm/dd/chmod -R 777 等）であり、バイナリ名のみでは
  代替できない。本タスクの「サブコマンド粒度の撤廃」とは性質が異なるため対象外。
- **mount/fdisk 等の名マッチ系（Medium 据え置き）**: これらは既にバイナリ名のみで Medium 判定して
  おり、サブコマンド解析を持たない。また §1.3 の境界原則では「定義済み操作によるシステム状態変更
  （未検証コードの特権実行ではない）」に該当するため、PM／systemctl（High）とは別水準で整合する。
  Medium 名集合のレベル妥当性の見直しは本タスクのスコープ外（必要なら別タスク）。
- **`RiskLevel` の段階定義変更**: Low/Medium/High/Critical の意味づけ・段数は維持する。本タスクは
  「どの粒度で判定するか」を変えるのみで、段階定義には踏み込まない。
