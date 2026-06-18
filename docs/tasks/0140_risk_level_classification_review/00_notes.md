# メモ：コマンド名ベース リスクレベル分類の見直し

> 本書は要件定義（`01_requirements.md`）作成前の課題整理ノート。決定事項ではなく
> 発見事項と論点の洗い出しを目的とする。設計・確定は `mkarch` / 要件プロセスで行う。
>
> | Item | Value |
> |---|---|
> | Status | `notes`（要件前） |
> | Created | 2026-06-18 |

## 発端（課題提起の要約）

「mkfs など大規模・破壊的な結果をもたらし得るコマンドがリスクレベル Medium になっている」
という指摘から、コマンド名ベースのリスク判定を棚卸しした。調査の結果、問題の本質は
個別コマンドのレベルが低いことではなく、**破壊的／システム変更系コマンドのレベルが複数の
判定系統に分散し、(A) 同類なのに High と Medium に割れる、(B) どの系統にも載らず Low を
素通りする**、という構造的な不整合・抜けにあると判明した。

> 補足：指摘にあった `mkfs` 自体は**実装上すでに High**（[evaluator_test.go:215](../../../internal/runner/base/risk/evaluator_test.go#L215)
> が `/sbin/mkfs.ext4 → High` を担保）。経路は `CheckDangerousArgPatterns`。
> 一方 0139 の **AC-06 は「fdisk/mkfs=Medium 維持」と記述**しており、実装と乖離している
> （[0139/01_requirements.md:115](../0139_coarse_system_modification_risk/01_requirements.md#L115)）。
> この doc/実装の乖離も本タスクで扱う候補。

## 現状アーキテクチャ（事実確認）

最終リスク＝**適用される全 dimension の最大値**（[evaluator.go:142-157](../../../internal/runner/base/risk/evaluator.go#L142-L157)、
`evaluateDimensions`）。名前ベース判定に関与する dimension が以下のように散在している。

| 系統 | 関数 / 定義（command_analysis.go ほか） | 出力レベル |
|---|---|---|
| システム変更 | `SystemModificationRisk` / `systemModificationCommandNames`・`packageManagerNames` | systemctl=サブコマンド条件付, service=High, **その他名集合=一律 Medium** |
| 破壊的ファイル操作 | `IsDestructiveFileOperation` / `destructiveCommandNames`(`rm`/`rmdir`/`unlink`/`shred`/`dd`) | High |
| 危険引数パターン | `CheckDangerousArgPatterns` / `dangerousCommandPatterns` | High / Medium |
| プロファイル | `commandProfileDefinitions`（`ResolveProfile`） | sudo系=Critical, 各種=Medium 等 |
| 任意コード実行 | `IsArbitraryCodeExecutionRunner` | High |
| coreutils 単一バイナリ | `CoreutilsCommandRisk` / `safe`・`destructive` 集合 | safe=Low, 破壊=High, **不明=High(fail-safe)** |

**構造上の根本原因**：名前のみで決まる破壊／システム変更系の「レベルの高低」が、
本来引数パターン用である `CheckDangerousArgPatterns`（`dangerousCommandPatterns`）に
偶発的に同居している。`SystemModificationRisk` 側は名集合を**一律 Medium**にしているため、
`dangerousCommandPatterns` に載っているか否かで同類コマンドの結論が割れる。

## 発見事項

### A. レベル不整合（同類が High と Medium に割れている）

`dangerousCommandPatterns`（[command_analysis.go:216-234](../../../internal/runner/base/security/command_analysis.go#L216-L234)）に
載るものは High、載らず `systemModificationCommandNames` だけのものは Medium になる。

| コマンド | 実レベル | 経路 | 所見 |
|---|---|---|---|
| `mkfs`, `mkfs.*` | High | 危険引数パターン | ○ |
| `fdisk` | High | 危険引数パターン | ○ |
| **`parted`** | **Medium** | システム変更のみ | ✗ fdisk と同等の破壊力 |
| **`fsck`** | **Medium** | システム変更のみ | ✗ FS 破損リスク |
| `mount`/`umount` | Medium | システム変更 | △ |
| `crontab`/`at`/`batch` | Medium | システム変更 | △ 永続化手段 |
| **`chkconfig`/`update-rc.d`** | **Medium** | システム変更 | ✗ systemctl(High, 0139後) と同類 |

- 最も明白：**`parted`(Medium) vs `fdisk`(High)** — どちらもパーティションテーブル編集。
- `chkconfig`/`update-rc.d` は「ブート時サービス有効化」で systemctl/service（High）と同質。
  0139 §1.3 の境界原則（未検証コードの特権実行＝High）に照らすと High が筋。

### B. 検出漏れ（どの系統にも載らず Low 素通り）

通常配置（`/usr/bin/*` 等、coreutils 単一バイナリディレクトリ外）では、以下はどの
dimension にも該当せず **Low**。grep 確認済み（security パッケージ内に名前定義なし）。

- **コマンド／バイナリ置換（最重要）**：`cp` `mv` `ln`(`-sf`) `install` `update-alternatives`。
  信頼バイナリを上書き・差し替え可能で、**allowlist + ハッシュ固定（第一防御）を次回実行時に
  無力化**し得る唯一のクラス。二次ゲートであるリスク判定の中では予防価値が突出して高い。
  ※ coreutils 単一バイナリ配下に置かれた場合のみ `CoreutilsCommandRisk` の fail-safe で
  High になるが、通常の独立バイナリ配置では効かない。
- **カーネル／モジュール**：`insmod` `modprobe` `rmmod` `kexec`。任意カーネルコード実行に等しい。
- **アカウント／認証境界**：`useradd` `usermod` `chpasswd` `visudo` `gpasswd` `adduser`
  （`passwd` は別系統の `dangerousPrivilegedCommands` パスにあるがリスクレベル次元には未反映）。
- **ディスク低レベル／LVM**：`wipefs` `blkdiscard` `sgdisk` `mkswap` `lvremove`/`vgremove`/`pvremove`。
- **ネットワーク／FW・ブート設定**：`iptables` `nft` `ip` `ifconfig` / `grub-install` `update-grub`。
- **権限付与**：`setcap` `setfacl`。

### C. データ送信が Medium 止まり

`curl` `wget` `scp` `rsync` `ssh` `nc` は一律 Medium（`dangerousCommandPatterns` または
プロファイル）。High は AI サービス系（`claude`/`gemini`/...）のみ。

### D. 提案たたき台の内部矛盾（要訂正）

リスク再定義の議論で挙がった分類案には、以下 3 点の内部矛盾がある。

1. **`visudo`=Critical と「Critical=正統な用途が無い」が衝突**。visudo の正統用途は
   「sudoers を安全に編集する」ことそのもの。さらに実装の Critical は**無条件ブロック
   ＝per-command でも実行不可**（論点5）。visudo/useradd には正当な特権バッチ用途があり、
   実行不可にすると壊れる。→ **visudo は High**（「権限付与=High」のバケツ、setcap と同列）が筋。
   Critical 定義を「**任意の内側コマンドを透過実行する特権昇格ラッパ（sudo/su/pkexec/doas）。
   ゲートを完全バイパスするので無条件ブロック**」と尖らせると、sudo=Critical / visudo=High /
   insmod=High が同一理屈（論点5）で揃う。「sudoers 編集＝root 付与能力」と見て Critical に
   寄せる解釈も成立するため、**「Critical=実行不可」を受け入れるかが決定点**（推奨は High）。
2. **`cp`/`mv`=Medium（b 案）と「バイナリ置換=最重要 High」（発見事項 B）が矛盾**。
   同じ cp/mv が両方に出る。解は論点4補足の宛先ゾーン化: bare = safe/ordinary ゾーンで Low/Medium、
   宛先が trust-critical なら High。`update-alternatives`/`install` は **/usr/bin 改変が目的**
   なので名前のみで High でよい（cp と粒度が違う）。これを入れないと B の穴が閉じない。
3. **`rm`/`mv`/`cp` は現状 High からの「格下げ」**。`destructiveCommandNames`
   （rm/rmdir/unlink/shred/dd）は現状 **High**。b 案は High→Medium/Low への引き下げ＝
   セキュリティを弱める方向で、引き上げ作業（論点6）とは性質が逆。**明示 AC + changelog** が要る。
   `dd`（b で言及なし）の扱いも決定要（of=device→High / of=file→宛先ゾーン）。

## 論点（要件・設計で詰める）

### 論点 1: 「High と Medium の境界原則」をどう再定義するか

0139 §1.3 の境界「High=未検証コードの特権実行 / Medium=定義済み操作のシステム状態変更」を
出発点に、見直し後の High カテゴリをどう括るか。たたき台（指摘者の提示観点＋追加観点）：

1. **永続的システム設定変更**（ブート／サービス／アカウント／認証／FW／カーネルモジュール）
2. **コマンド／バイナリ置換**（信頼境界＝allowlist+ハッシュの破壊）
3. **大規模・不可逆なファイル／ディスク破壊**
4. **外部へのデータ送信**
5. **AI**

→ どの観点を High に含めるか、特に ④（データ送信）を Medium のままにするかは要決定。

#### 論点 1 補足: 2 軸モデルと改訂版・統一原則

**根本フレーム＝最終リスクは 2 軸の max**:
- **軸1（コマンド階級 / name 固定）**: system 変更・auth・kernel・権限付与・信頼バイナリ置換を
  名前で固定レベル化（High/Medium）。論点2の一元化リファクタ先。
- **軸2（呼び出し危険度 / arg）**: `rm -rf`・`dd if=/of=device`・`chmod 777`、宛先パスゾーン
  （論点4補足）で High/Low へ調整。本来の `CheckDangerousArgPatterns` の役割。

これにより `mkfs`/`parted`=High（軸1, name 由来）と `rm`=Medium/`rm -rf`=High（軸2, arg 由来）が
**矛盾なく両立**する。「大規模」の運用定義も粒度で固定: **device/FS/ツリー粒度に作用しうる→High、
named-file 粒度→Medium**（rm=named→Medium / rm -r=tree→High / mkfs=device→High）。

**改訂版・統一原則（たたき台）**:

```
最終リスク = max(軸1: コマンド階級[name固定], 軸2: 呼び出し危険度[arg/宛先ゾーン])

critical — 任意内側コマンドを透過実行する特権昇格ラッパ（sudo/su/pkexec/doas）。無条件ブロック。
high     — ①device/FS/ツリー粒度の不可逆破壊（能力 or 危険arg）
           ②永続的 system/boot/service/account/auth 変更
           ③高権限での任意コード実行（kernelモジュール, dlsym/LD_PRELOAD, interpreter, AI駆動）
           ④信頼境界の破壊（信頼バイナリ/設定の置換, allowlist+ハッシュ無力化）  ← 原則に新規追加
           ⑤権限/能力付与（setcap/setfacl/visudo/chmod-grant/chown）
medium   — 永続的だが named-file 粒度の影響（rm/mv/cp/rmdir, 非クリティカルパス）
           / データ egress（境界越え: curl/scp/ssh/rsync）
           / 定義済み・限定スコープの system 変更（mount, 単一IF設定）
low      — それ以外（safe-zone 内のロケーション定義コマンド等）
```

- **④信頼境界の破壊を High 原則に新規追加**: バイナリ/設定の置換は**単一ファイル操作**なので
  「大規模ファイル喪失」では捕まらず「system 変更」とも別物（allowlist+ハッシュ＝第一防御を
  次回実行時に無力化）。発見事項 B で最重要としながら原則 4 行に無かった抜けを補う。
- **ネットワークは 1 語で括らない**: データ egress（境界越え軸→Medium）と ネット設定変更
  （system 変更軸: FW=iptables/nft→High、単一IF=ip/ifconfig→Medium）は別軸。
- **AI⇔egress の非対称（既知の限界として明文化）**: `claude`/`gemini`=High だが
  `curl api.anthropic.com`=Medium で**素通りバイパス可能**。name ベース AI 検出は egress 一般を
  塞ぐものではなく、salient な明示ケースの defense-in-depth。また AI は「任意バイナリ実行」より
  「未検証コンテンツ駆動＋egress」が実体で、③へ同居させると分類が濁る点に留意。C（Medium 据え置き）
  を選ぶ場合もこの限界を doc に残す。

### 論点 2: 判定構造をリファクタするか（割れの原理的解消）

名前のみで決まる破壊／システム変更系を `SystemModificationRisk` 側に一元化し、
名集合ごとに High/Medium を明示する構造へ変えると、`dangerousCommandPatterns` に
載るか否かで割れる現象が原理的に消える。`CheckDangerousArgPatterns` は本来の
「引数依存パターン（`rm -rf`/`dd if=`/`chmod 777`）」専用に戻す。
- 影響：`dangerousCommandPatterns` から `mkfs`/`fdisk`/`format` の名前のみエントリを移設。
- 0139（PM/systemctl の粗粒度化）と方針が整合（名マッチ・固定レベル）。同時/直後にやるか。

### 論点 3: 新規対象コマンドの列挙範囲（線引き）

脅威モデル（0136 AC-66/67）上、ブロックリストは非有界で backstop は allowlist+ハッシュ。
よって「どこまで列挙するか」は精度ではなく fail-safe 既定値とレベル一貫性の問題。
- 確実に入れる候補：`parted` `fsck` `chkconfig` `update-rc.d`（既存 Medium の引き上げ）、
  `insmod`/`modprobe`/`rmmod`/`kexec`、`useradd`/`usermod`/`chpasswd`/`visudo`、
  `wipefs`/`blkdiscard`/`sgdisk`/`mkswap`、`cp`/`mv`/`ln`/`install`/`update-alternatives`。
- 検討：`iptables`/`nft`/`ip`、`grub-install`/`update-grub`、`setcap`/`setfacl`、
  LVM 群、`mount`/`umount` を High に上げるか Medium 据え置きか。

### 論点 4: 「コマンド置換」をどう判定するか（引数 vs 名前）

`cp`/`mv`/`ln`/`install` は通常用途（データコピー）が大多数で、危険なのは
「信頼パス配下への書き込み」。粗粒度（名前のみで全呼び出し High）に倒すか、
引数の宛先パス（`/usr/bin` 等の system-critical path）を見るかは精度／保守コストのトレードオフ。
- 名前のみ＝0139 の YAGNI 方針と整合だが、日常的な cp/mv が軒並み High になる影響大。
- `HasSystemCriticalPaths`（[command_analysis.go:320](../../../internal/runner/base/security/command_analysis.go#L320)）が既存。
  宛先がクリティカルパスのときのみ High、という折衷が可能か。

#### 論点 4 補足: 「3 ゾーン・パス関数」への一般化

`cp`/`mv`/`mkdir`/`rmdir`/`ln`/`install`/`touch` は**名前自体に固有の危険がなく、
リスクが本質的に宛先パスの関数**になる「**ロケーション定義コマンド**」階級。軸2を
「critical→High」の二値ではなく **`pathZone(target)` の 3 ゾーン関数**に格上げする:

| target ゾーン | レベル | 例 |
|---|---|---|
| trust-critical（`/usr/bin` `/etc` `/boot` …） | High | 信頼境界破壊 |
| ordinary | Medium | 既定 |
| safe-zone（正規化後の指定 workdir / output / private temp） | **Low** | home/temp 内の日常操作 |

- 適用は**ロケーション定義コマンド階級に限定**。`mkfs`/`parted`/`wipefs`/`insmod`/`useradd`
  等の **intrinsic 階級はパスで下げてはいけない**（device/アカウントが対象でパス引数を持たない）。
  最終リスクは max 合成なので、別 dimension を踏めば結局 max が効く。

**Low へ下げる際の安全要件（必須。「下げる」は「上げる」より危険＝Low は無審査素通り）:**

1. **正規化済みパスで判定**（symlink/traversal 耐性）。`~/link→/etc`、`$HOME/../../etc/passwd`、
   mv のシンボリックリンク追従。`safefileio` の強化済み解決経路を流用（独自実装しない＝DRY）。
2. **safe-zone は曖昧な `$HOME` ではなく、run の指定 workdir / output / 構成済み temp を
   正規化したものに限定**。特権バッチ委譲が主目的のため `$HOME` は root/対象ユーザで曖昧。
3. **/tmp は無条件 safe ではない**（world-writable・symlink レース・他プロセス clobber）。
   temp を safe 扱いするなら run 専用 private temp に限る。
4. **宛先を確実に取れない呼び出しは Low にしない**（`-t`/`--target-directory`/複数 source/`--`
   等で解析非自明）→ fail-safe で Medium 据え置き。
5. **glob/変数展開が絡む宛先は下げない**（評価時に実宛先が未確定 → safe 判定不可）。

**スコープ判断（要明文化）**: 「home 内の cp/mv/rm = Low」は実質
**「本ツールはシステム完全性と信頼境界を守る。ユーザ自身のデータの自己破壊（`rm -rf ~/`）は
守備範囲外」**という宣言。妥当なスコープだが要件に 1 文で明記しないと後で揉める。

**粒度の注意**:
- `rmdir` は**空ディレクトリのみ**削除、`mkdir` も低 blast。cp/mv（上書き破壊あり）と同じ表に
  載せると過大評価。critical-path 以外は Low / critical-path のみ Medium〜High の一段低いカーブが妥当か。
- `rm` をこの階級に入れるかは論点（前回の格下げ論点と接続）。`rm ~/file`=Low は自然だが
  `rm -r`/glob は軸2で High 昇格が残る。
- 0139 の YAGNI（名前のみ・粗粒度）とは**逆方向の精密化**。`HasSystemCriticalPaths` は upgrade 側に
  既存だが、**downgrade 側（safe-zone 判定）は新規で最も堅牢性が要る**部分。
- **精密化の線引き（決定）**: コマンド階級で粗粒度／精密を使い分ける。判断基準＝
  「コマンドラインフラグの安定性 × 使用頻度」。
  - **package manager**（ディストリ毎に異なり機能も多様）→ **一律・名前のみ粗粒度**
    （引数解析しない。0139 方針を踏襲）。
  - **基本コマンド**（cp/mv/mkdir/rmdir/ln 等、頻用かつフラグが安定）→ **引数解析を行う**
    （宛先パスゾーンで判定）。

### 論点 5: Critical を使うか（カーネルコード実行など）

`insmod`/`kexec` は任意カーネルコード実行で破滅性は特権昇格に匹敵するが、Critical 枠は
現状「特権昇格＝無条件ブロック」専用。正当用途（特権バッチ委譲）があるため、
**High（per-command で明示許可可能）に留める**のが現実的か。要決定。

### 論点 6: 既存テスト・doc の整合

- 0139 AC-06（fdisk/mkfs=Medium 維持）の実装乖離を本タスクで訂正するか、0139 内で直すか。
- `risk_assessment.ja.md`/`.md`、用語集、開発者向け `command-risk-evaluation.{ja,md}` への反映。
- 引き上げで既存 sample/テスト config がブロックされ得るため、`risk_level` 付与の追従が要る
  （0139 AC-14 と同型の作業）。

## 決定事項（本ノート時点で確定。要件・設計の前提とする）

議論を経て以下を確定。詳細根拠は各論点・発見事項を参照。

| # | 決定 | 根拠 |
|---|---|---|
| D1 | **2 軸モデル採用**：最終リスク = `max(軸1: コマンド階級[name固定], 軸2: 呼び出し危険度[arg/宛先ゾーン])` | 論点1補足 |
| D2 | **改訂版・統一原則を採用**（critical/high/medium/low の 4 段、High に **④信頼境界の破壊**を新規追加） | 論点1補足 |
| D3 | **Critical 定義の尖鋭化**：任意内側コマンドを透過実行する特権昇格ラッパ（sudo/su/pkexec/doas）に限定。無条件ブロック＝実行不可を意味する | 発見事項D-1, 論点5 |
| D4 | **ロケーション定義コマンドは 3 ゾーン・パス関数**（critical→High / ordinary→Medium / safe-zone→Low）。Low 化は安全要件 5 点（正規化解決・safe-zone 限定・/tmp 非無条件・パース不能/glob は下げない）を満たす場合のみ | 論点4補足 |
| D5 | **精密化の線引き**：package manager=一律名前のみ粗粒度／基本コマンド（cp/mv/mkdir/rmdir/ln 等）=引数解析。基準＝フラグ安定性 × 使用頻度 | 論点4補足 |
| D6 | **`visudo` 等 権限付与／認証境界系 = High**（Critical にしない。正当な特権バッチが実行不可になるため） | 発見事項D-1 |
| D7 | **`rm`/`rmdir`/`shred`/`dd` = 宛先ゾーン+arg 化**（`rm ~/file`=Low、`rm -r`/glob/critical-path=High）。現状 name のみ High からの変更につき**明示 AC + changelog 必須** | 発見事項D-3, 論点4補足 |
| D8 | **カーネルモジュール（insmod/modprobe/rmmod/kexec）= High**（Critical にしない） | 論点5 |
| D9 | **データ送信（egress: curl/scp/ssh/rsync）= Medium 据え置き**（C, As is）。AI⇔egress 非対称は**既知の限界として doc 明記** | 論点1補足, 発見事項C/D |
| D10 | **0139 AC-06 乖離（fdisk/mkfs=Medium 維持 ⇄ 実装 High）は本タスク 0140 で訂正**（0139 は触らない） | 発見事項 冒頭, 論点6 |
| D11 | **「検討」群の配置を確定**（下表） | 論点1補足の原則を適用 |
| D12 | **`update-alternatives`=名前のみ High（intrinsic に system バイナリ改変。`dpkg-divert`/`alternatives` も同類）／`install`=arg 条件**（宛先ゾーン＋setuid/setgid→High。cp/mv 類の location-defined） | 発見事項B/D, D4 |

### D11: 「検討」群の最終配置（残課題2の確定）

| コマンド | レベル | 判定方式 | 根拠（原則の該当項） |
|---|---|---|---|
| `iptables`/`nft`/`iptables-restore` | **High** | 名前のみ | ②FW=システム全体のセキュリティ境界変更 |
| `iptables-save`（読取のみ） | **Low** | 名前のみ | 副作用なし |
| `grub-install`/`update-grub`/`grub-mkconfig`/`efibootmgr` | **High** | 名前のみ | ②永続的 boot 変更＋③任意 boot コード（systemctl-enable と同質） |
| `setcap` | **High** | 名前のみ | ⑤能力付与（capability＝特権ベクタ。intrinsic） |
| `ip`/`ifconfig`/`route` | **Medium** | 名前のみ（粗粒度） | 「ネットは原則 mid」。`ip route` の system 全体性は粗粒度で Medium に丸める（D5） |
| `mount`/`umount` | **Medium**（既定）／**High** | mountpoint の宛先ゾーン | 既定は定義済み system 変更=Medium。**mountpoint が trust-critical（/usr/bin・/etc 等）なら ④信頼境界破壊（shadowing）→ High**。cp/mv と同じ宛先ゾーン判定を mountpoint に適用 |
| `setfacl` | **Medium**（既定）／**High** | arg 条件（付与方向/対象） | chmod と同型。**付与方向（権限拡大）または critical リソース対象のみ High**、他 Medium |
| LVM 破壊系：`lvremove`/`vgremove`/`pvremove`/`lvreduce`/`vgreduce`/`pvmove` | **High** | 名前のみ（別バイナリ） | ①大規模・不可逆なディスク/ボリューム破壊 |
| LVM 作成/設定系：`lvcreate`/`lvextend`/`vgcreate`/`pvcreate`/`vgchange`/`lvchange` | **Medium** | 名前のみ（別バイナリ） | 永続的だが破壊を伴わない system 変更 |

- `mount`/`setfacl` は arg/宛先依存判定が要る（D4 の宛先ゾーン・dd of= と同類の精密化群）。実装段取りは `mkarch`。

## 未決事項（要件・設計で詰める残課題）

1. **判定構造の一元化リファクタ（論点2）を本タスクで実施するか、0139 との順序**（実装段取り。`mkarch` で確定）。
2. **arg/宛先依存判定群の具体ロジック**（実装段取りは `mkarch`）：
   - `dd` の `of=`（device→High / file→宛先ゾーン）
   - `mount` の mountpoint 宛先ゾーン（trust-critical→High）
   - `setfacl` の付与方向/対象判定（権限拡大 or critical リソース→High）
   - `install` の宛先ゾーン＋mode（setuid/setgid→High）
   - `rm`/`cp`/`mv`/`ln` の宛先ゾーン（D4/D7）
3. **ユーザー/開発者文書の更新範囲**（`risk_assessment.{ja,}.md`、用語集、`command-risk-evaluation.{ja,}`）と、
   引き上げに伴う既存 sample/テスト config の `risk_level` 追従（0139 AC-14 と同型）。

## 関連コード・参照

- [command_analysis.go](../../../internal/runner/base/security/command_analysis.go):
  `SystemModificationRisk`/`isSystemModificationByNames`/`systemModificationCommandNames`/
  `packageManagerNames`、`IsDestructiveFileOperation`/`destructiveCommandNames`、
  `CheckDangerousArgPatterns`/`dangerousCommandPatterns`。
- [evaluator.go](../../../internal/runner/base/risk/evaluator.go): `evaluateDimensions`（dimension の max 合成）。
- [coreutils.go](../../../internal/runner/base/security/coreutils.go): `CoreutilsCommandRisk`、
  `safeCoreutilsCommands`/`destructiveCoreutilsCommands`（単一バイナリ配下のみ適用・不明は High）。
- [profile_builder.go](../../../internal/runner/base/security/profile_builder.go) / `commandProfileDefinitions`。
- 0139（PM/systemctl 粗粒度化、本見直しと方針連動）: [01_requirements.md](../0139_coarse_system_modification_risk/01_requirements.md)。
- 脅威モデル前提：0136 AC-66/67（ブロックリストは非有界・allowlist+ハッシュ固定が backstop）。
