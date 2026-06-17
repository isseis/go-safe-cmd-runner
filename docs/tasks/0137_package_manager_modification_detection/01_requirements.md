# パッケージマネージャのシステム変更検出の一般化（dpkg/rpm フラグ方式対応） — 要件定義書

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
**system modification として検出されない**。

### 1.2 0136 との関係（AC-34 の方針更新）

dpkg 検出の追加は 0136 で意図的にスコープ外とされた。0136
`docs/tasks/0136_runtime_risk_evaluation_enforcement/01_requirements.md` の **AC-34**:

> §3.1 から `dpkg` を削除する（実装に `dpkg` 検出ロジックが存在しないため）。`dpkg` 検出の追加は
> 本タスクのスコープ外とし、必要であれば別途の要件変更として扱う。

本タスク（0137）は AC-34 が「別途の要件変更」として切り出した範囲を扱う。**0136 の承認済み AC を
直接書き換えず、本要件で方針を上書き（追補）する**（F-005）。

### 1.3 設計判断（要件確定前の意思決定）

要件作成にあたり、以下を決定済み（2026-06-18, isseis）。

| 論点 | 決定 | 根拠 |
|---|---|---|
| 対象マネージャの範囲 | **dpkg + rpm のみ** | フラグ方式の主要 distro 2 つで非対称ギャップを直接解消。YAGNI。verb 方式の追加マネージャ（apk/snap 等）は別軸でありスコープ外。一般化後はデータ追加で拡張可能。 |
| 判定精度（rpm 照会の偽陽性） | **(C) ツール別ルール** | dpkg/pacman は大小区別の文字含有（偽陽性なし）、rpm のみ「変更モード文字を含み かつ 照会/検証モード文字を含まない」で `rpm -qi` 等を除外。F-011（読み取り専用を Medium 下限に留め最小権限を守る）と同じ思想を維持。 |
| リスクレベル | **Medium 維持** | 既存のパッケージ install/remove（apt/yum 等、`ReasonSystemModification`）と同水準。同一カテゴリ操作を揃え、dpkg/rpm だけ非対称にしない。 |
| 実装構造 | **凝集リファクタを含める** | マネージャ識別と検出規則を 1 か所へ集約し、`isSystemModificationByNames` から pacman 個別分岐を排除。一般化と同時実施が自然。 |

### 1.4 目的

dpkg/rpm のシステム変更操作を、既存パッケージマネージャと同じ Medium として実行時・dry-run の
双方で一貫して検出する。あわせて pacman 固有のハードコードを per-tool 機構へ一般化し、ユーザー/開発者
文書を実装と一致させる。

## 2. スコープ

- **In**:
  - dpkg・rpm のフラグ方式システム変更操作の検出（F-001 / F-002）。
  - pacman 固有実装の per-tool 一般化（凝集リファクタ。F-003）。
  - リスクレベル整合と実行時/dry-run 一貫性（F-004）。
  - ユーザー/開発者文書の整合（F-005）。
- **Out**（スコープ外。§6 参照）:
  - verb 方式の追加マネージャ（apk/snap/flatpak/gem/pip3/pnpm/port/emerge 等）。
  - リスクレベルの段階定義の変更（Medium 以外への変更や新レベル導入）。
  - パッケージマネージャ以外のシステム変更系判定の見直し。
  - 0136 で確定済みの脅威モデル前提（ブロックリスト方式・allowlist+ハッシュ固定 backstop）の変更。

## 3. 前提・脅威モデル（0136 から継承・不変）

本タスクの検出強化は 0136 の脅威モデルの上に成立し、これを変更しない。

- リスク判定は**ブロックリスト方式**であり単独で全脅威を遮断しない。未知コマンド・未列挙の
  マネージャは Low 通過し得る。安全運用は **allowlist + ハッシュ固定の併用**が前提
  （0136 AC-66 / AC-67）。
- フラグ方式の文字含有判定は **fail-safe（取りこぼしより過検出を許容＝安全側）** を基本とする。
  ただし rpm の照会コマンドについては最小権限を優先し偽陽性を抑制する（F-002）。
- 判定は basename（解決済みシンボリックリンクを考慮）に対して行う。0136 の basename 方式の
  限界（ハードリンク/リネーム非対応）を引き継ぐ。

## 4. 機能要件

### F-001: dpkg のフラグ方式システム変更操作を検出する

dpkg はメジャー操作をオプションフラグで選択する（`-i` install, `-r` remove, `-P` purge、および
長形式 `--install`/`--remove`/`--purge`）。これらをシステム変更操作として検出する。照会・情報系の
オプション（`-l`/`-L`/`-s`/`-S`/`-p`/`-I`/`-c` 等）はシステム変更として検出しない。大文字・小文字を
厳密に区別する（modifying は小文字 `i`/`r` と大文字 `P`、`-I`（info, 大文字）は照会で非該当）。

**Acceptance Criteria**:
- **AC-01**: `dpkg -i pkg.deb`、`dpkg -r pkg`、`dpkg -P pkg`、`dpkg --install …`/`--remove …`/`--purge …`
  がシステム変更（`medium`, `ReasonSystemModification`）として検出される。短縮結合（例 `-iG`、
  `-i` を含む結合トークン）でも modifying として検出される。
- **AC-02**: `dpkg -l`、`dpkg -L pkg`、`dpkg -s pkg`、`dpkg -S /path`、`dpkg -p pkg`、`dpkg -I pkg.deb`、
  `dpkg -c pkg.deb` および引数なし `dpkg` は、システム変更として検出されない（照会・情報系）。
- **AC-03**: 大文字 `-I`（--info, 照会）と小文字 `-i`（install）が区別され、`-I` は非該当・`-i` は該当。
- **AC-04**: 絶対パス（例 `/usr/bin/dpkg -i …`）およびシンボリックリンク経由のエイリアスでも
  basename 解決により検出される。

### F-002: rpm のフラグ方式システム変更操作を検出する（照会・検証モード除外）

rpm はメジャーモードをフラグで指定し、修飾子と結合する（例 `-ivh` install, `-qi` query info,
`-qpi` query package info）。install/upgrade/freshen/erase（`-i`/`-U`/`-F`/`-e`、長形式
`--install`/`--upgrade`/`--freshen`/`--erase`）をシステム変更として検出する。一方、**照会（query, `-q`）・
検証（verify, `-V`）モードを含むトークンはシステム変更として検出しない**（最小権限の維持）。
判定は「変更モード文字（`i`/`e`/`U`/`F`）を含み、かつ照会・検証モード文字（`q`/`V`）を含まない」を
基準とし、大文字・小文字を厳密に区別する。

**Acceptance Criteria**:
- **AC-05**: `rpm -i pkg.rpm`、`rpm -U pkg.rpm`、`rpm -F pkg.rpm`、`rpm -e pkg`、結合形 `rpm -ivh pkg.rpm`/
  `rpm -Uvh pkg.rpm`、および長形式 `--install`/`--upgrade`/`--freshen`/`--erase` がシステム変更
  （`medium`）として検出される。
- **AC-06**: `rpm -qi pkg`（query info）、`rpm -qa`（query all）、`rpm -qpi pkg.rpm`、`rpm -ql pkg`、
  `rpm -V pkg`（verify）、`rpm --query …`/`--verify …`、および引数なし `rpm` は、システム変更として
  検出されない（`i` を含む `-qi`/`-qpi` の誤検出を防ぐ）。
- **AC-07**: 変更モード文字と照会モード文字が同一トークンに混在する場合（例 `-qi`）は、照会・検証を
  優先して**非該当**とする（fail-safe より最小権限を優先する rpm 固有規則）。
- **AC-08**: 絶対パス（例 `/usr/bin/rpm -U …`）およびシンボリックリンク経由のエイリアスでも
  basename 解決により検出される。

### F-003: フラグ方式判定を per-tool 機構へ一般化する（凝集リファクタ）

pacman 固有の `isPacmanModifyingFlag` と `isSystemModificationByNames` 本体の個別分岐
（`_, isPacman := names["pacman"]`）を、マネージャごとの識別情報と検出規則を 1 か所に集約した
per-tool 機構へ置き換える。新規マネージャの追加が本体の制御分岐を増やさず行えるようにする。
本要件は**振る舞いを変えない構造変更**であり、既存 pacman の判定結果を退行させない。

**Acceptance Criteria**:
- **AC-09**: pacman の既存判定が保持される: `pacman -S pkg`/`-R pkg`/`-U pkg`/`-Syu`/`-Rns pkg`、
  長形式 `--sync`/`--remove`/`--upgrade` はシステム変更として検出され、`pacman -Q`/`-Ss pkg`/`-Si pkg`
  等の照会は従来どおりの結果（既存テストが pin する挙動）を維持する。
- **AC-10**: dpkg・rpm・pacman のフラグ方式判定が、同一の per-tool 機構（マネージャ別の規則定義）を
  通じて行われる。マネージャ固有の制御分岐（`isPacman` 相当のハードコード）が判定本体から除去される。
- **AC-11**: verb 方式の既存マネージャ（apt/apt-get/yum/dnf/zypper/brew/pip/npm/yarn）の検出が
  退行しない（`apt install`→検出、`apt list`→非検出 等の既存挙動を維持）。

### F-004: リスクレベル整合と実行時/dry-run 一貫性

dpkg/rpm のシステム変更操作は、既存パッケージマネージャと同じ **Medium**（`ReasonSystemModification`）
として分類する。0136 の経路統合により実行時（normal）と dry-run は同一の `EvaluateRisk` を共有する
ため、新規検出も両経路で同一の実効リスクを返す。

**Acceptance Criteria**:
- **AC-12**: dpkg/rpm のシステム変更操作の実効リスクが `medium`（`ReasonSystemModification`）である。
  既存のパッケージ install/remove（`apt install` 等）と同水準であり、High へは昇格しない。
- **AC-13**: 同一の dpkg/rpm コマンド集合について、実行時経路（`risk.StandardEvaluator.EvaluateRisk`）と
  dry-run 経路（同一 `EvaluateRisk` を共有）が同一の実効リスクを返すことが、整合テスト
  （`coreutils_consistency_test.go` と同様の共有評価器の出力固定）で検証される。
- **AC-14**: 他の高リスク次元（破壊的操作・特権昇格・任意コード実行等）と複合する場合は、0136 の
  最大値モデルに従い、より高い次元が実効リスクを支配する（dpkg/rpm の Medium が他次元の High 等を
  引き下げない）。

### F-005: 影響を受けるドキュメントの整合

本変更（dpkg/rpm のフラグ方式検出の追加、および pacman 固有実装の per-tool 一般化）により記述が実装と
乖離するドキュメントを以下に特定し、更新する。**`docs/tasks/` 配下は各タスク時点のスナップショット
であり、本タスクの更新対象外とする。**

| 区分 | ファイル | 更新内容 |
|---|---|---|
| ユーザー | `docs/user/risk_assessment.ja.md` / `docs/user/risk_assessment.md` | §3.1 の表「その他のシステム変更コマンド（…`apt install`）」行に、dpkg/rpm のフラグ方式 install/remove/upgrade を `medium` として反映する。照会形（`dpkg -l`、`rpm -qi` 等）は対象外である旨と矛盾させない。§5（最近の変更）の「パッケージマネージャは汎用的に扱う」移行ノートとも整合させる。 |
| 開発者 | `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `docs/dev/architecture_design/command-risk-evaluation.md` | 「システム変更リスク（`SystemModificationRisk`）」節の記述を、verb 方式に加え**フラグ方式**（pacman `-S`/`-R`/`-U`、dpkg `-i`/`-r`/`-P`、rpm `-i`/`-U`/`-F`/`-e`）も検出するよう更新する。rpm の照会/検証（`-q`/`-V`）除外規則を明記する。 |

**影響なしと判断したドキュメント（更新不要）**:
- `docs/user/toml_config/09_practical_examples.ja.md` / `.md`: dpkg の実例は `dpkg -l`（照会）であり
  検出対象外（F-001 AC-02）のため、現状の記述・`risk_level` 未設定のままで整合する。
- `docs/user/toml_config/appendix.ja.md` / `10_best_practices.ja.md` / `06_command_level.ja.md`:
  概括的なリスク例（`apt-get`/`systemctl`/`rm -rf` 等）であり、dpkg/rpm の追加による影響を受けない。
- `docs/dev/architecture_design/security-architecture.ja.md`: 判定軸の概括的言及のみで影響なし。

**Acceptance Criteria**:
- **AC-15**: 0136 の AC-34 の方針（dpkg 検出はスコープ外）が、本要件で「dpkg/rpm を検出対象に含める」へ
  更新されたことが本書（§1.2）に明記される。0136 の承認済み AC 本文は書き換えない。
- **AC-16**: `docs/user/risk_assessment.ja.md` および `docs/user/risk_assessment.md` の §3.1 の表に、
  dpkg/rpm のフラグ方式 install/remove/upgrade が `medium` として反映される。照会形（`dpkg -l`、
  `rpm -qi` 等）が対象外である点と移行ノートに矛盾がない。日英 2 ファイルが整合する。
- **AC-17**: `docs/dev/architecture_design/command-risk-evaluation.ja.md` および `.md` の「システム変更
  リスク」節が、フラグ方式（pacman/dpkg/rpm）の検出と rpm の照会/検証除外規則を含むよう更新される。
  日英 2 ファイルが整合する。

## 5. 非機能要件

- **NF-001（大小区別の厳密性）**: フラグ方式判定はツールごとに大文字・小文字を厳密に区別する。
  境界（dpkg `-i`/`-I`、rpm `-i`/`-qi`、pacman `-S`/`-s`）をテストで固定する。
- **NF-002（実行時/dry-run 一貫性）**: 検出は単一実装（`SystemModificationRisk` 系）に集約し、
  実行時と dry-run で分裂しない（F-004 / AC-13）。
- **NF-003（拡張性）**: 新規フラグ方式マネージャの追加が、判定本体の制御分岐追加ではなくマネージャ
  定義の追加で行える構造とする（F-003 / AC-10）。
- **NF-004（脅威モデル整合）**: 列挙は非網羅であり allowlist+ハッシュ固定が backstop であることを
  ユーザー/開発者文書に既存記述（0136 AC-66/67）と矛盾なく保つ。

## 6. スコープ外

- **verb 方式の追加マネージャ**（apk `add`/`del`、snap `install`/`remove`/`refresh`、
  flatpak `install`/`uninstall`、gem `install`/`uninstall`、pip3、pnpm `add`/`remove`、port、emerge 等）。
  脅威モデル上ブロックリストは非有界であり、本タスクは非対称の直接原因である dpkg/rpm に限定する。
  F-003 の一般化により、これらは将来データ追加で拡張可能（個別対応は別タスク）。
- **rpm のモード先頭厳密解析（論点 2 (B)）**: 引数文法（短縮結合・`--`・順序）の完全解析は採用しない。
  F-002 の「q/V 含有で照会・検証を除外」する規則で十分とする。
- **リスクレベルの段階定義変更**: Medium 維持。High 昇格や新レベル導入は行わない。
- **パッケージマネージャ以外のシステム変更系**（systemctl/service/mount 等）の判定見直し。

## 7. 受け入れ基準トレーサビリティ

各 AC のテスト対応は `03_implementation_plan.md` の「受け入れ基準の検証」節で管理する。
AC-15/16/17 は文書整合（`static` 検証可）、それ以外は `test` 検証を要する。

## 8. 参照

- ノート: [00_notes.md](00_notes.md)（論点整理）
- 実装: [command_analysis.go](../../../internal/runner/base/security/command_analysis.go)
  （`SystemModificationRisk`, `isSystemModificationByNames`, `packageManagerNames`,
  `packageModifyingVerbs`, `isPacmanModifyingFlag`）、
  [evaluator.go](../../../internal/runner/base/risk/evaluator.go)（`EvaluateRisk`）
- 整合テスト: [coreutils_consistency_test.go](../../../internal/runner/base/risk/coreutils_consistency_test.go)
- 0136: `docs/tasks/0136_runtime_risk_evaluation_enforcement/01_requirements.md`
  （AC-34, F-010, 脅威モデル AC-66/67）
- ユーザー文書: `docs/user/risk_assessment.ja.md` / `.md`
- 開発者文書: `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `.md`
