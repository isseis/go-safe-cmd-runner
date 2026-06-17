# メモ：パッケージマネージャのシステム変更検出の一般化（フラグ方式 dpkg/rpm 対応）

> 本書は要件定義（`01_requirements.md`）作成前の課題整理ノート。決定事項ではなく
> 論点の洗い出しを目的とする。設計・確定は `mkarch` / 要件プロセスで行う。

## 発端（課題提起の原文・要約）

タスク 0136（実行時リスク判定の強化, PR #729）のレビューで、システム変更検出
（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go) の
`SystemModificationRisk` と内部ヘルパ `isSystemModificationByNames`。0136 で旧 `IsSystemModification`
から改名・分離された）のパッケージマネージャ検出に非対称があることが判明した。

- **verb 方式**（`install`/`remove`/`purge`/`add`/`upgrade` 等のサブコマンド）は
  `packageManagerNames` に載る全マネージャ（apt/apt-get/yum/dnf/zypper/pacman/brew/pip/npm/yarn）
  に対して `packageModifyingVerbs` で検出する。
- **フラグ方式**（`-S`/`-R`/`-U`/`-Syu` のように操作をオプションフラグで選ぶ）は
  **pacman のみ**（`isPacmanModifyingFlag`）に限定されている。

この限定は「現在の `packageManagerNames` に載るマネージャの中でフラグ方式は pacman だけ」
という意味では正しい。しかし **dpkg（`-i`/`-r`/`-P`）・rpm（`-i`/`-e`/`-U`/`-F`）も
フラグ方式**であり、これらは検出対象（`packageManagerNames`）に入っていないため、
`dpkg -i pkg.deb` / `rpm -U pkg.rpm` のようなシステム変更操作が **system modification として
検出されない**。

## 背景・制約（重要）：0136 のスコープ境界 AC-34

dpkg 検出の追加は **0136 では意図的にスコープ外**とされている。
`docs/tasks/0136_runtime_risk_evaluation_enforcement/01_requirements.md` の **AC-34**:

> §3.1 から `dpkg` を削除する（実装に `dpkg` 検出ロジックが存在しないため）。
> **`dpkg` 検出の追加は本タスクのスコープ外**とし、必要であれば別途の要件変更として扱う。

したがって本タスク（0137）は、AC-34 が「別途の要件変更」として明示的に切り出した範囲を
扱う。**0137 の要件定義は AC-34 の方針更新を伴う**（dpkg/rpm を検出対象に含める方向への変更）。

## スコープ（たたき台）

- **In**: フラグ方式マネージャ（dpkg, rpm）のシステム変更操作検出。`isSystemModificationByNames`
  （`SystemModificationRisk` の内部ヘルパ）の pacman 固有実装を per-tool 化して一般化する。
  ユーザー文書（`risk_assessment`）への dpkg/rpm 反映。
- **要検討（In/Out 未確定）**: verb 方式の追加マネージャ（`apk add`/`del`、`snap install`/`remove`、
  `flatpak install`/`uninstall`、`gem install`、`pip3`、`pnpm` 等）。脅威モデル上は非有界なので
  「どこまで列挙するか」は論点（後述）。
- **Out**: パッケージマネージャ以外のシステム変更系の見直し。リスクレベルの段階定義変更。

## 論点（要件・設計で詰める）

### 論点 1: フラグ方式の一般化方法（per-tool フラグマップ）

pacman 固有の `isPacmanModifyingFlag` を、ツール別のフラグ集合へ一般化する案。

```go
// たたき台。同じ文字でもツールで意味が違うため tool 別・大小区別が必須。
var pkgManagerModifyingFlags = map[string]string{
    "pacman": "SRU",  // -S sync/install, -R remove, -U upgrade
    "dpkg":   "irP",  // -i install, -r remove, -P purge
    "rpm":    "ieUF", // -i install, -e erase, -U upgrade, -F freshen
}
```

- 長形式（`--install`/`--remove`/`--purge`/`--erase`/`--upgrade`/`--freshen`/`--sync`）を
  共通集合で別途見るか、ツール別に持つか。
- 名前集合（symlink 解決後の `names`）に複数のフラグ方式マネージャが現れた場合の優先順位
  （通常は 1 つだが、symlink エイリアスの組合せを考慮するか）。

### 論点 1 の補足: 実装構造の凝集（`isSystemModificationByNames` のハードコード解消）

現状の問題（保守性）: マネージャの**識別情報**と**操作検出規則**が分散している。

- マネージャ名は `packageManagerNames`（集合）。
- 変更系 verb は `packageModifyingVerbs`（全マネージャ共有の集合）。
- フラグ規則は `isPacmanModifyingFlag`（pacman 専用関数）＋ `isSystemModificationByNames` 本体の
  `_, isPacman := names["pacman"]` という**個別分岐のハードコード**。

→ pacman 1 種でも見通しが悪く、dpkg/rpm を足すたびに本体の分岐・専用関数が増える。

改善方針（たたき台）: **1 マネージャ = 1 定義**に凝集し、`isSystemModificationByNames` からツール固有の
知識（特に大小区別のフラグ文字や個別分岐）を追い出す。

```go
// 例: マネージャごとに識別と検出規則を 1 か所へ。
type packageManager struct {
    Name           string // basename 照合キー（例 "pacman"/"dpkg"/"rpm"）
    ModifyingFlags string // 操作選択フラグ文字（fail-safe 文字含有用。空=verb のみ）
    // 必要なら: ModifyingVerbs/長形式フラグをツール別に持つか、共有集合のままにするか
}

var packageManagers = []packageManager{
    {Name: "pacman", ModifyingFlags: "SRU"},
    {Name: "dpkg",   ModifyingFlags: "irP"},
    {Name: "rpm",    ModifyingFlags: "ieUF"},
    {Name: "apt"},   // verb のみ
    // ...
}
```

- `isSystemModificationByNames` は「解決済み名前集合に一致する `packageManager` を引き、その規則を適用」
  という一般処理になり、新規マネージャ追加は **データ追加（1 行）** で済む（コード分岐を増やさない）。
- 検討点:
  - verb 集合をツール別に持つか（`apk del` のように共有集合に無い語の局所化）、共有のままにするか。
  - 長形式フラグ（`--install` 等）をこの定義に含めるか共有集合に置くか。
  - 論点 2（rpm モード先頭判定）を採る場合、`ModifyingFlags string` では表現しきれないため、
    判定を関数値（`func(args []string) bool`）にする等、定義の表現力を上げる必要がある
    （＝定義モデルの設計は論点 2 の方針と連動）。
- 位置づけ: これは**機能追加ではなく構造リファクタ**だが、論点 1（一般化）と同時に行うのが自然。
  0136（PR #729）では pacman 固有のハードコードのまま残し、本タスクで凝集した構造へ置換する。

### 論点 2: rpm の「モード先頭判定」を実装するか（最重要の精度論点）

文字含有方式（`ContainsAny`）は **fail-safe（過検出＝Medium 側）** だが、rpm で誤検出が出る。

- rpm は「メジャーモード」を要求する: `-i`(install)/`-U`(upgrade)/`-F`(freshen)/`-e`(erase)/
  `-q`(query)/`-V`(verify) 等。修飾子（`v` verbose, `h` hash, `p` package, `a` all 等）と
  組み合わさる（例 `-ivh` install, `-qa` query all, `-qi` query info, `-qip` query info package）。
- **問題**: `rpm -qi`（query info）は `i` を含むため、単純な「`i` を含む＝install」判定では
  **照会コマンドを誤って Medium にする**。
- 取りうる方針:
  - **(A) fail-safe 文字含有（簡易）**: `-qi` の誤検出を許容し Medium に倒す。実装は最小
    （pacman 既存方式の踏襲）。`pacman -Ss`（検索）も同様に既に誤検出する前提と整合。
    - 長所: 実装小・安全側（取りこぼしなし）。短所: 照会系の偽陽性で「最小権限」を阻害
      （`risk_level=low` の rpm 照会が拒否され得る）。
  - **(B) モード先頭判定（厳密）**: rpm のメジャーモード（最初に現れるモード規定オプション）を
    抽出し、修飾子（`q` が付く＝query 系）を除外して install/upgrade/freshen/erase のみ modifying
    と判定。`-qi`/`-qa` は query モードとして除外。
    - 長所: 偽陽性を抑え最小権限を保つ。短所: rpm 引数文法（短縮結合・`--`・順序）の解析が増え
      実装・テストコスト増。dpkg/pacman にも同様の厳密化を求めるか整合判断が要る。
  - **(C) ハイブリッド**: rpm のみ `q`/`V` を含むトークンを「照会・検証モード」として除外し、
    それ以外は文字含有 fail-safe。中庸だがツール固有規則が分散する。
- 決定材料: 本プロジェクトのリスク判定は fail-safe 優先だが、F-011（systemctl 読み取り系を
  Medium 下限にして最小権限を守る）と同じ思想なら、照会系の偽陽性は望ましくない → (B) 寄り。
  ただし「ブロックリストは非有界・allowlist+ハッシュが backstop」という脅威モデル（AC-66/67）
  からは (A) でも許容範囲。**要件で「rpm 照会の偽陽性をどこまで許容するか」を決める必要がある。**

### 論点 3: 大文字・小文字の区別

- dpkg: `-i`(install, 小文字) と `-I`(--info, 照会, 大文字) は別物 → **大小区別必須**。
  modifying は小文字 `i`/`r` と大文字 `P`(purge)。
- rpm: `-i`(install) と `-q`(query) … `-U`/`-F` は大文字。大小を厳密に扱う。
- pacman: `-S`/`-R`/`-U`（大文字）。`-s`（小文字, 一部サブで search 等）と区別。
- → `ContainsAny` の文字集合は **ツールごとに大小を厳密指定**する。テストで境界を固定する。

### 論点 4: 対象マネージャの範囲（列挙の線引き）

- 確実に入れる: `dpkg`, `rpm`（フラグ方式・主要 distro）。
- 検討: `apk`（`add`/`del`）, `snap`（`install`/`remove`/`refresh`）, `flatpak`（`install`/`uninstall`）,
  `gem`（`install`/`uninstall`）, `pip3`（`pip` の別名）, `pnpm`（`add`/`remove`）, `port`（MacPorts）,
  `emerge`（Gentoo, `--unmerge` 等）。
- verb 集合に不足する語（例 `del`, `refresh`, `erase`）の追加。
- 脅威モデル（AC-66/67）に照らし「列挙は非網羅・backstop は allowlist+ハッシュ」を要件に明記し、
  **どこで線を引くか（主要 distro のみ / 広く取るか）**を決める。

### 論点 5: 実行時 / dry-run の整合（NF-002 相当）

> 【0136 で状況変化】本ノート作成後、0136 で dry-run と本番（normal）の経路が統合された。
> 旧記述（dry-run は `AnalyzeCommandSecurity`、実行時は `EvaluateRisk` という別経路）は現状と合わない。

- 現状: **dry-run と実行時の双方が同一の `risk.StandardEvaluator.EvaluateRisk` を共有**する
  （`dryrun_manager.go` / `normal_manager.go` のいずれも `EvaluateRisk` を呼ぶ）。
  dry-run 専用だった `AnalyzeCommandSecurity`（early-return 方式）は **廃止済み**。
- システム変更検出はその `EvaluateRisk` から `security.SystemModificationRisk`（内部で
  `isSystemModificationByNames`）として呼ばれる **単一の入口**に集約されている。したがって
  early-return（旧 dry-run）と最大値モデル（実行時）の差異という論点は **解消済み**で、
  dry-run と実行時で結論が分岐する余地は構造上なくなった。
- 帰結: dpkg/rpm 検出を `SystemModificationRisk` 系に追加すれば、両経路は単一実装の共有により
  自動的に整合する。0136 の整合性テスト `coreutils_consistency_test.go` は現在
  「両経路が同一評価器を使う（single source）」前提のもとで **共有評価器の結果を固定する回帰テスト**
  になっている。dpkg/rpm でも同様に共有評価器の出力を固定するテストを追加すれば足りる
  （経路差そのものの検証は不要）。

### 論点 6: リスクレベルと判定の位置づけ

- パッケージ install/remove は 0136 で **Medium**（`ReasonSystemModification`）。dpkg/rpm も同水準で
  よいか（システム全体への変更影響度を踏まえ High を検討するかは要件判断）。
- `service`（High, 未検証 init スクリプト実行）や systemctl 変更系（High）との水準バランス。

### 論点 7: ドキュメント・トレーサビリティ整合

- **01_requirements.md（0136）の AC-34** を 0137 側で「方針更新」する形（dpkg/rpm を検出対象へ）。
  0136 の承認済み AC を直接書き換えるのではなく、0137 の要件で上書き・追補する運用を要確認。
- **risk_assessment.ja.md / .md**: 0136 の AC-34 で削除した `dpkg` を、検出を実装する本タスクで
  **再追加**（rpm も）。日英 2 ファイル整合。0136 PR-6（ユーザー文書）との順序・重複に注意。
- 開発者向け `command-risk-evaluation.{ja,md}`（PR #724 系）への反映要否。

### 論点 8: テスト方針

- 肯定: `dpkg -i/-r/-P`, `rpm -i/-U/-e/-F`, `pacman -S/-R/-U/-Syu/-Rns`, 長形式
  `--install/--remove/...`。
- 否定（偽陽性監視）: `dpkg -l/-L/-s/-I`(照会), `rpm -qa/-qi/-V`(照会・検証), `pacman -Q/-Ss`,
  `apt list`。論点 2 の方針（A/B/C）でこの期待値が変わる。
- basename・symlink 解決（エイリアス経由）込みのケース。

## 関連コード・参照

- [command_analysis.go](../../../internal/runner/base/security/command_analysis.go):
  `SystemModificationRisk`（公開・`RiskLevel` を返すディメンション入口）, `isSystemModificationByNames`
  （内部ヘルパ）, `packageManagerNames`, `packageModifyingVerbs`, `isPacmanModifyingFlag`
  （いずれも 0136 で旧 `IsSystemModification` から改名・分離）
- 経路統合（0136）: [evaluator.go](../../../internal/runner/base/risk/evaluator.go) の
  `StandardEvaluator.EvaluateRisk` が `SystemModificationRisk` を呼ぶ唯一の実行時入口。dry-run も
  `dryrun_manager.go` 経由で同じ `EvaluateRisk` を共有（旧 `AnalyzeCommandSecurity` は廃止）。
  整合テストは [coreutils_consistency_test.go](../../../internal/runner/base/risk/coreutils_consistency_test.go)。
- 0136: `docs/tasks/0136_runtime_risk_evaluation_enforcement/01_requirements.md`（AC-34, F-010）,
  PR #729（パッケージマネージャ verb 拡張＋pacman フラグ検出を導入した経緯）
- ユーザー文書: `docs/user/risk_assessment.ja.md` / `.md`
- 脅威モデル前提: 0136 AC-66/67（ブロックリストは非網羅・allowlist+ハッシュ固定が backstop）

## 未決事項（要件で確定すべき点）

1. 対象マネージャの線引き（`dpkg`/`rpm` のみか、`apk`/`snap`/`flatpak` 等まで広げるか）。
2. `rpm`/`dpkg` の判定精度（論点 2 の `A`/`B`/`C` のどれを採るか＝偽陽性許容度）。
3. リスクレベル（Medium 維持か）。
4. AC-34 の更新方法（0137 要件での上書き運用）とユーザー文書の再追加範囲。
5. 定義モデルの凝集（論点 1 の補足）: マネージャ定義を 1 か所へ集約し `isSystemModificationByNames` の
   個別分岐を解消するか。表現力（文字集合 vs 関数値）は論点 2 の精度方針に依存。
