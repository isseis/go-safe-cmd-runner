# パッケージマネージャのシステム変更検出の一般化（dpkg/rpm フラグ方式対応） — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-18 |
| Review date | 2026-06-18 |
| Reviewer | isseis |
| Comments | 2026-06-18: アーキテクト／SRE／テクニカルライター観点のサブエージェントレビュー指摘を反映 — 長形式オプションの完全一致化（AC-02/06 の誤検出修正）、rpm 判定の走査単位を振る舞いで明確化（`-e --verbose` 取りこぼし防止）、HOW（文字集合・内部関数名・テストファイル名）を AC 本文から除去し 02 へ委譲、`-iG` 結合記述の削除、間接実行/ラッパー AC（AC-18）・監査追跡 AC（AC-19）・破壊的変更の移行ノート AC（AC-20）・検出限界文書化 AC（AC-21）・用語集 AC（AC-22）の新設、リファクタ先行のシーケンス要件（NF-005）、曖昧語の測定可能化、用語統一（変更操作／ツール別）。 |

## 1. 背景と目的

### 1.1 課題

タスク 0136（実行時リスク判定の強化, PR #729）のレビューで、システム変更検出
（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go) の
`SystemModificationRisk` と内部ヘルパ `isSystemModificationByNames`）のパッケージマネージャ検出に
**非対称**があることが判明した。

- **verb 方式**（`install`/`remove`/`purge`/`upgrade` 等のサブコマンド）は
  `packageManagerNames` に載る全マネージャ（apt/apt-get/yum/dnf/zypper/pacman/brew/pip/npm/yarn）に
  対して `packageModifyingVerbs` で検出される。
- **フラグ方式**（操作をオプションフラグで選ぶ）は **pacman のみ**（`isPacmanModifyingFlag`）に
  限定されている。

その結果、フラグ方式かつ `packageManagerNames` に含まれない **dpkg（`-i`/`-r`/`-P`）・
rpm（`-i`/`-e`/`-U`/`-F`）** によるシステム変更操作（例: `dpkg -i pkg.deb`, `rpm -U pkg.rpm`）が
**システム変更として検出されない**。

### 1.2 0136 との関係（AC-34 の方針更新）

dpkg 検出の追加は 0136 で意図的にスコープ外とされた。0136
`docs/tasks/0136_runtime_risk_evaluation_enforcement/01_requirements.md` の **AC-34**:

> §3.1 から `dpkg` を削除する（実装に `dpkg` 検出ロジックが存在しないため）。`dpkg` 検出の追加は
> 本タスクのスコープ外とし、必要であれば別途の要件変更として扱う。

本タスク（0137）は AC-34 が「別途の要件変更」として切り出した範囲を扱う。**0136 の承認済み AC を
直接書き換えず、本要件で方針を上書き（追補）する**（F-005 / AC-15）。

### 1.3 設計判断（要件確定前の意思決定）

要件作成にあたり、以下を決定済み（2026-06-18, isseis）。

| 論点 | 決定 | 根拠 |
|---|---|---|
| 対象マネージャの範囲 | **dpkg + rpm のみ** | フラグ方式の主要 distro 2 つで非対称ギャップを直接解消。YAGNI。verb 方式の追加マネージャ（apk/snap 等）は別軸でありスコープ外。一般化後はデータ追加で拡張可能。 |
| 判定精度（rpm 照会の偽陽性） | **(C) ツール別ルール** | 選択肢: (A) 全マネージャ共通の単純照合 / (B) rpm 引数文法の厳密解析 / (C) ツール別ルール。(C) は dpkg/pacman を単純照合とし、rpm のみ照会・検証操作（`rpm -qi`/`-V` 等）を非検出として偽陽性を抑制する。F-011（読み取り専用を Medium 下限に留め最小権限を守る）と同じ思想を維持しつつ、(B) の解析コストを避ける。 |
| リスクレベル | **Medium 維持** | 既存のパッケージ install/remove（apt/yum 等、`ReasonSystemModification`）と同水準。同一カテゴリ操作を揃え、dpkg/rpm だけ非対称にしない。 |
| 実装構造 | **ツール別機構への凝集リファクタを含める** | マネージャ識別と検出規則を 1 か所へ集約し、判定本体から pacman 個別分岐を排除。一般化と同時実施が自然。 |

### 1.4 目的

dpkg/rpm のシステム変更操作を、既存パッケージマネージャと同じ Medium として実行時・dry-run の
双方で一貫して検出する。あわせて pacman 固有のハードコードをツール別機構へ一般化し、ユーザー/開発者
文書を実装と一致させ、破壊的変更（後述）を移行ノートで周知する。

## 2. スコープ

- **In**:
  - dpkg・rpm のフラグ方式システム変更操作の検出（F-001 / F-002）。
  - pacman 固有実装のツール別機構への一般化（凝集リファクタ。F-003）。
  - リスクレベル整合・実行時/dry-run 一貫性・間接実行/監査の整合（F-004）。
  - 影響を受けるユーザー/開発者文書の整合と破壊的変更の移行周知（F-005）。
- **Out**（詳細・根拠は §6）:
  - verb 方式の追加マネージャ（apk/snap/flatpak/gem/pip3/pnpm/port/emerge 等）。
  - rpm 引数文法の厳密解析（論点 2 (B)）。
  - リスクレベルの段階定義の変更。
  - パッケージマネージャ以外のシステム変更系判定の見直し。
  - 0136 で確定済みの脅威モデル前提（ブロックリスト方式・allowlist+ハッシュ固定 backstop）の変更。

## 3. 前提・脅威モデル（0136 から継承・不変）

本タスクの検出強化は 0136 の脅威モデルの上に成立し、これを変更しない。

- リスク判定は**ブロックリスト方式**であり単独で全脅威を遮断しない。未知コマンド・未列挙の
  マネージャ（apk/snap 等）・multi-call ディスパッチ形式（`busybox <pm>` 等）は Low 通過し得る。
  安全運用は **allowlist + ハッシュ固定の併用**が前提（0136 AC-66 / AC-67）。
- フラグ方式の判定は **fail-safe（取りこぼしより過検出を許容＝安全側）** を基本とする。ただし
  rpm の照会・検証操作については最小権限を優先し、過検出を抑制する（F-002）。この例外が
  変更操作（install/erase 等）の取りこぼしを生まないことを境界テストで担保する（AC-05 / NF-001）。
- 判定は basename（解決済みシンボリックリンクを考慮）に対して行う。0136 の basename 方式の
  限界（ハードリンク/リネーム非対応）を引き継ぐ。

## 4. 機能要件

> **HOW の委譲**: 以降の AC は「コマンド → 算出リスク」の振る舞いで記述する。判定機構の内部表現
> （ツール別規則の表現形式・文字集合・走査アルゴリズム等）は `02_architecture.md` で確定する。

### F-001: dpkg のフラグ方式システム変更操作を検出する

dpkg はメジャー操作をオプションフラグで選択する。**変更操作**（短形式 `-i` install / `-r` remove /
`-P` purge、および長形式 `--install`/`--remove`/`--purge`／`--unpack`／`--configure`）をシステム変更
として検出する。`--unpack`（パッケージ展開）・`--configure`（展開済みパッケージの設定。postinst 等の
maintainer スクリプトを実行する）は短形式を持たない変更操作だが、システム状態を変える価値の高い操作
のため含める。一方、**照会・情報操作**（`-l` list, `-L`, `-s` status, `-S` search, `-p`, `-I` info,
`-c`、および長形式 `--list`/`--status`/`--info`/`--get-selections`/`--print-avail` 等）は検出しない。
大文字・小文字を厳密に区別する（変更は小文字 `i`/`r` と大文字 `P`。大文字 `-I`（info）は照会で非該当）。
長形式は完全一致で扱い、照会用長形式（`--info` 等）を変更操作と誤判定しない。

**Acceptance Criteria**:
- **AC-01**: `dpkg -i pkg.deb`、`dpkg -r pkg`、`dpkg -P pkg`、`dpkg --install …`／`--remove …`／
  `--purge …`、`dpkg --unpack pkg.deb`、`dpkg --configure pkg`／`dpkg --configure -a` がシステム変更
  （`medium`, `ReasonSystemModification`）として検出される。
- **AC-02**: 以下はシステム変更として検出されない（照会・情報系）: 短形式 `dpkg -l`／`-L pkg`／
  `-s pkg`／`-S /path`／`-p pkg`／`-I pkg.deb`／`-c pkg.deb`、長形式 `dpkg --list`／`--status pkg`／
  `--info pkg.deb`／`--get-selections`／`--print-avail pkg`、および引数なし `dpkg`。
- **AC-03**: 大文字 `-I`（info, 照会）と小文字 `-i`（install）が区別され、`-I` は非該当・`-i` は該当。
- **AC-04**: 絶対パス（例 `/usr/bin/dpkg -i …`）およびシンボリックリンク経由のエイリアスでも
  basename 解決により検出される。

### F-002: rpm のフラグ方式システム変更操作を検出する（照会・検証モード除外）

rpm はメジャーモードをフラグで指定し、修飾子と結合する（例 `-ivh` install, `-qi` query info）。
**変更モード**（install/upgrade/freshen/erase = `-i`/`-U`/`-F`/`-e`、結合形 `-ivh`/`-Uvh` 等、
長形式 `--install`/`--upgrade`/`--freshen`/`--erase`）をシステム変更として検出する。加えて、システム
状態を変更する rpm 長形式 `--reinstall`（パッケージの再導入。install 同質）／`--import`（GPG 信頼鍵の
追加）／`--initdb`／`--rebuilddb`（RPM データベースの初期化・再構築）も検出対象に含める。一方、
ファイルのメタデータ復元系（`--restore`／`--setperms`／`--setugids`）は影響が限定的なため対象外とする
（スコープ外。§6 参照）。一方、**照会（query, `-q`）・検証（verify, `-V`）モードはシステム
変更として検出しない**（最小権限の維持）。判定はモードを規定するフラグトークンに基づき、`--verbose`／
`--nodeps`／`--force` 等のモードを規定しない修飾オプションは、変更操作の分類を変えない（変更操作を
照会扱いに反転させない）。長形式は完全一致で扱い、上記の変更操作集合に含まれない照会系長形式
（`--eval`／`--querytags` 等）は検出しない。大文字・小文字を厳密に区別する。

**Acceptance Criteria**:
- **AC-05**: 次がシステム変更（`medium`）として検出される: `rpm -i pkg.rpm`、`rpm -U pkg.rpm`、
  `rpm -F pkg.rpm`、`rpm -e pkg`、結合形 `rpm -ivh pkg.rpm`／`rpm -Uvh pkg.rpm`、長形式
  `rpm --install`／`--upgrade`／`--freshen`／`--erase`。**修飾オプション併用時も検出される**:
  `rpm -e --nodeps pkg`、`rpm -e --verbose pkg`、`rpm -U --force pkg.rpm`（`--verbose` 等で取りこぼさない）。
  また長形式 `rpm --reinstall pkg.rpm`、`rpm --import KEY`、`rpm --initdb`、`rpm --rebuilddb` も検出される。
- **AC-06**: 次はシステム変更として検出されない（照会・検証）: `rpm -qi pkg`、`rpm -qa`、
  `rpm -qpi pkg.rpm`、`rpm -ql pkg`、`rpm -V pkg`、`rpm -qa --verbose`、長形式 `rpm --query …`／
  `--verify …`／`--eval '…'`／`--querytags`、および引数なし `rpm`。
- **AC-07**: コマンドの**いずれかのトークン**に照会・検証モードを規定するフラグ（`-q`/`-V`、長形式
  `--query`/`--verify`）が含まれる場合は、変更モードフラグが同一トークンに同居する場合（例 `-qi`）でも、
  別トークンに分かれて指定される場合（例 `rpm -q -i pkg`／`rpm -i -q pkg`）でも**非該当**とする
  （照会・検証を優先。fail-safe より最小権限を優先する rpm 固有規則）。
- **AC-08**: 絶対パス（例 `/usr/bin/rpm -U …`）およびシンボリックリンク経由のエイリアスでも
  basename 解決により検出される。

### F-003: フラグ方式判定をツール別機構へ一般化する（凝集リファクタ）

pacman 固有の判定（`isPacmanModifyingFlag` と判定本体の `isPacman` 個別分岐）を、マネージャごとの
識別情報と検出規則を 1 か所に集約したツール別機構へ置き換える。新規マネージャの追加が判定本体の
制御分岐を増やさず行えるようにする。本要件は**振る舞いを変えない構造変更**であり、既存 pacman・
verb 方式の判定結果を退行させない。実装順序は NF-005 に従う（リファクタ先行）。

**Acceptance Criteria**:
- **AC-09**: pacman の既存判定が**そのまま保持される**（本リファクタは振る舞いを変えない）。検出される:
  `pacman -S pkg`／`-R pkg`／`-U pkg`／`-Syu`／`-Rns pkg`、長形式 `--sync`／`--remove`／`--upgrade`。
  検出されない: `pacman -Q`／`-Qi pkg`（照会）。なお現行実装は `-S` を含む検索/情報照会（`pacman -Ss`／
  `-Si`）も変更操作として検出する（大文字 `S` の文字含有による既存の過検出）。この過検出は fail-safe
  側であり、その**是正は本タスクのスコープ外**として現状の挙動を維持する（退行させない）。
- **AC-10**: 新規のフラグ方式マネージャの追加が、判定本体へのマネージャ名分岐（`isPacman` 相当）の
  追加ではなく、**マネージャ定義 1 エントリの追加**で完結する。dpkg・rpm・pacman の判定が同一の
  ツール別機構を通じて行われ、判定本体にマネージャ名固有の分岐が残らない。
- **AC-11**: verb 方式の既存マネージャ（apt/apt-get/yum/dnf/zypper/brew/pip/npm/yarn）の検出が
  退行しない（`apt install`→検出、`apt list`→非検出 等の既存挙動を維持）。

### F-004: リスクレベル整合・実行時/dry-run 一貫性・間接実行/監査の整合

dpkg/rpm のシステム変更操作は、既存パッケージマネージャと同じ **Medium**（`ReasonSystemModification`）
として分類する。0136 の経路統合により実行時（normal）と dry-run は同一の評価器を共有するため、
新規検出も両経路・直接実行/間接実行で一貫し、既存の監査経路に乗る。

**Acceptance Criteria**:
- **AC-12**: dpkg/rpm のシステム変更操作の実効リスクが `medium`（`ReasonSystemModification`）である。
  既存のパッケージ install/remove（`apt install` 等）と同水準であり、High へは昇格しない。
- **AC-13**: 同一の dpkg/rpm コマンド集合について、実行時経路と dry-run 経路が**同一の実効リスク**を返す。
- **AC-14**: 他の高リスク次元（破壊的操作・特権昇格・任意コード実行等）と複合する場合は、0136 の
  最大値モデルに従い、より高い次元が実効リスクを支配する（dpkg/rpm の Medium が他次元の High 等を
  引き下げない）。
- **AC-18**: ラッパー・間接実行経由でも検出が保たれる。`env dpkg -i pkg.deb`／`timeout 60 rpm -U pkg.rpm`
  のように非特権ラッパーで包んだ場合の実効リスクが、直接実行と同等以上（≥ Medium）になる。
  `sudo dpkg -i …`／`sudo rpm -U …` は特権昇格次元（Critical）が支配する。
- **AC-19**: dpkg/rpm の新規検出が既存の監査経路に乗る。検出による deny が、0136 AC-56 の相関フィールド
  （reason code に `system_modification`、`decision`、解決済みパス等）を備えた監査エントリとして
  記録され、運用者が後から事象を追跡できる。

### F-005: 影響を受けるドキュメントの整合と破壊的変更の移行周知

本変更により記述が実装と乖離する、または周知が必要なドキュメントを以下に特定し更新する。
**`docs/tasks/` 配下は各タスク時点のスナップショットであり、本タスクの更新対象外とする。**

| 区分 | ファイル | 更新内容 | 対応 AC |
|---|---|---|---|
| ユーザー | `docs/user/risk_assessment.ja.md` / `.md` | §3.1 リスク評価表の「その他のシステム変更コマンド」行に dpkg/rpm のフラグ方式変更操作（dpkg: install/remove/purge/unpack/configure、rpm: install/upgrade/freshen/erase/import/initdb/rebuilddb）を `medium` として追加。 | AC-16 |
| ユーザー | 同上（§8 移行ノート） | 破壊的変更を 1 項追加（以前 `low`/未指定で通っていた dpkg/rpm の上記変更操作が `medium` でブロックされる旨、回避策、dry-run 事前確認）。purge/erase/freshen も含む全操作種別を明示。 | AC-20 |
| ユーザー | 同上（§3.1 注記 / §7 脅威モデルと限界） | 検出は dpkg/rpm/pacman を対象とする（ラッパー経由も評価される）一方、照会形・未列挙マネージャ（apk/snap 等）・multi-call（busybox 等）・リネームは対象外（Low 通過しうる）であることを明記。 | AC-21 |
| 開発者 | `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `.md` | 「システム変更リスク（`SystemModificationRisk`）」節に、フラグ方式（pacman/dpkg/rpm）検出と rpm の照会/検証除外規則を追記。 | AC-17 |
| 用語集 | `docs/translation_glossary.md` | 新規用語（フラグ方式/verb 方式/照会/検証/システム変更/ツール別機構）の対訳を追加。 | AC-22 |

**影響なしと判断したドキュメント（更新不要）**:
- `docs/user/toml_config/09_practical_examples.ja.md` / `.md`: dpkg 実例は `dpkg -l`（照会・`risk_level`
  未設定）で F-001 AC-02 により非検出。`record` の引数列挙の `/usr/bin/dpkg` はハッシュ記録対象であり
  リスク評価とは無関係。現状で整合。
- `docs/user/toml_config/appendix.{ja,md}` / `10_best_practices.{ja,md}` / `06_command_level.{ja,md}`:
  概括的なリスク例（`apt-get`/`systemctl`/`rm -rf` 等）で dpkg/rpm を含まず影響なし。
- `docs/dev/architecture_design/security-architecture.ja.md`: 判定軸の概括的言及のみで影響なし。
- `docs/user/security-risk-assessment.{ja,md}`: 「パッケージ」言及は依存パッケージ脆弱性の文脈で、
  コマンドリスク評価とは無関係。影響なし。
- `README.md` / `README.ja.md`、`CHANGELOG.md`: `risk_level` 例に dpkg/rpm が登場せず、また CHANGELOG は
  リスク評価の振る舞い変更を記録する慣行がない（0136 も未記載）。影響なし。

**Acceptance Criteria**:
- **AC-15**: 0136 の AC-34 の方針（dpkg 検出はスコープ外）が、本要件で「dpkg/rpm を検出対象に含める」へ
  更新されたことが本書（§1.2）に明記される。0136 の承認済み AC 本文は書き換えない。
- **AC-16**: `docs/user/risk_assessment.ja.md` および `.md` の §3.1 表に、dpkg のフラグ方式変更操作
  （install/remove/purge、`--unpack`/`--configure` を含む）および rpm のフラグ方式変更操作
  （install/upgrade/freshen/erase、`--import`/`--initdb`/`--rebuilddb` を含む）を `medium` とする記述が
  **追加されている**。日英 2 ファイルが同一の対象操作集合と `medium` 値を持つ（差分は訳語のみ）。
- **AC-17**: `docs/dev/architecture_design/command-risk-evaluation.ja.md` および `.md` の「システム変更
  リスク」節に、フラグ方式（pacman/dpkg/rpm）の検出と rpm の照会/検証除外規則の記述が**追加されている**。
  日英 2 ファイルが対応する記述を持つ。
- **AC-20**: `docs/user/risk_assessment.ja.md` および `.md` の §8 移行ノートに、dpkg のフラグ方式変更操作
  （install/remove/**purge**、`--unpack`/`--configure`）および rpm のフラグ方式変更操作
  （install/upgrade/**freshen**/**erase**、`--import`/`--initdb`/`--rebuilddb`）が新たに `medium` として
  検出される旨（以前 `low`/未指定で通っていた当該コマンドはアップグレード後ブロックされること）、
  回避策（該当コマンドへ `risk_level = "medium"` を明示）、事前確認手順（`--dry-run` でリスクを確認）を
  記載した項目が**追加されている**。日英 2 ファイルが対応する。
- **AC-21**: ユーザー文書（§3.1 注記または §7 脅威モデルと限界）に、フラグ方式検出が **dpkg/rpm/pacman を
  対象とする**こと（`env`/`timeout`/`sudo` 等のラッパー経由でも評価される。AC-18 参照）、および
  照会形・未列挙マネージャ（apk/snap/flatpak/gem 等）・multi-call 形式（`busybox <pm>` 等）・リネーム
  されたバイナリは検出対象外で `low` 通過しうること、安全運用は allowlist+ハッシュ固定が前提であることが、
  0136 AC-66/67 と整合する形で**記載されている**。日英 2 ファイルが対応する。
- **AC-22**: `docs/translation_glossary.md` に、本タスクで用いる用語（少なくとも「フラグ方式 / flag style」
  「verb 方式 / verb style」「照会 / query」「検証 / verify」「システム変更 / system modification」
  「ツール別機構 / per-tool mechanism」）の対訳が**追加されている**。

## 5. 非機能要件

- **NF-001（大小区別の厳密性）**: フラグ方式判定はツールごとに大文字・小文字を厳密に区別する。
  境界（dpkg `-i`/`-I`、rpm `-i`/`-qi` および `rpm -e --verbose`、pacman `-S`/`-s`）をテストで固定する。
- **NF-002（実行時/dry-run 一貫性）**: 検出は単一実装に集約し、実行時と dry-run で分裂しない
  （F-004 / AC-13）。
- **NF-003（拡張性）**: 新規フラグ方式マネージャの追加が、判定本体の制御分岐追加ではなくマネージャ
  定義の追加で行える構造とする（F-003 / AC-10）。
- **NF-004（脅威モデル整合）**: 列挙は非網羅であり allowlist+ハッシュ固定が backstop であることを
  ユーザー/開発者文書に既存記述（0136 AC-66/67）と矛盾なく保つ（F-005 / AC-21）。
- **NF-005（実装順序）**: F-003 の凝集リファクタは**振る舞いを変えない単独ステップ**として先行し、
  その時点で既存テストが退行しないことを確認する。dpkg/rpm の新規検出（F-001/F-002）はその上に
  データ追加として積む。回帰の切り分けのため両者を分離して実装・検証する（詳細は 03 で計画）。

## 6. スコープ外

- **verb 方式の追加マネージャ**（apk `add`/`del`、snap `install`/`remove`/`refresh`、
  flatpak `install`/`uninstall`、gem `install`/`uninstall`、pip3、pnpm `add`/`remove`、port、emerge 等）。
  脅威モデル上ブロックリストは非有界であり、本タスクは非対称の直接原因である dpkg/rpm に限定する。
  F-003 の一般化により、これらは将来データ追加で拡張可能（個別対応は別タスク）。
- **rpm 引数文法の厳密解析（論点 2 (B)）**: 短縮結合・`--` 終端・順序の完全解析は採用しない。
  F-002 の振る舞い（照会・検証モードを除外、修飾オプションは分類を変えない）で十分とする。
- **低価値・低頻度のパッケージ補助操作**: dpkg の `--set-selections`／`--clear-selections`／
  `--add-architecture`／`--remove-architecture`／`--update-avail`／`--triggers-only`、rpm の `--restore`／
  `--setperms`／`--setugids`（ファイルのメタデータ・パーミッション復元）等。状態変更ではあるが影響が
  限定的で使用頻度も低いため、本タスクでは検出対象に含めず allowlist+ハッシュ固定の backstop に委ねる
  （0136 AC-66/67）。高価値な変更操作（dpkg `--unpack`/`--configure`、rpm `--reinstall`/`--import`/
  `--initdb`/`--rebuilddb`）は In に含める。
- **リスクレベルの段階定義変更**: Medium 維持。High 昇格や新レベル導入は行わない。
- **パッケージマネージャ以外のシステム変更系**（systemctl/service/mount 等）の判定見直し。

## 7. 受け入れ基準トレーサビリティ

各 AC のテスト対応は `03_implementation_plan.md` の「受け入れ基準の検証」節で管理する。
AC-15〜AC-17 および AC-20〜AC-22 は文書整合（`static` 検証可）、それ以外は `test` 検証を要する。

## 8. 参照

- ノート: [00_notes.md](00_notes.md)（論点整理）
- 実装: [command_analysis.go](../../../internal/runner/base/security/command_analysis.go)
  （`SystemModificationRisk`, `isSystemModificationByNames`, `packageManagerNames`,
  `packageModifyingVerbs`, `isPacmanModifyingFlag`）、
  [evaluator.go](../../../internal/runner/base/risk/evaluator.go)（`EvaluateRisk`、`ReasonSystemModification`）、
  [indirect_execution.go](../../../internal/runner/base/security/indirect_execution.go)（ラップ内側コマンドの `SystemModificationRisk` 利用）
- 整合テスト参考: [coreutils_consistency_test.go](../../../internal/runner/base/risk/coreutils_consistency_test.go)
- 0136: `docs/tasks/0136_runtime_risk_evaluation_enforcement/01_requirements.md`
  （AC-34, F-010, 脅威モデル AC-66/67, 監査相関 AC-56, 移行ノート §8 相当）
- ユーザー文書: `docs/user/risk_assessment.ja.md` / `.md`（§3.1 表・§7 脅威モデル・§8 移行ノート）
- 開発者文書: `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `.md`
- 用語集: `docs/translation_glossary.md`
