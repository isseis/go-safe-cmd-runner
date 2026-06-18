# コマンド名ベース リスクレベル分類の一貫化 — 要件定義書

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

名前ベースのリスク判定が複数の dimension（[evaluator.go](../../../internal/runner/base/risk/evaluator.go)
`evaluateDimensions`、最終リスク＝適用 dimension の **max**）に散在しており、破壊／システム
変更系コマンドのレベルに**構造的な不整合・抜け**がある（詳細は [00_notes.md](00_notes.md)）。

- **(A) 同類が High と Medium に割れる**：`fdisk`=High（危険引数パターン）に対し同等の破壊力を
  持つ `parted`/`fsck` は `SystemModificationRisk` 経由で Medium。`systemctl`(High, 0139 後) と
  同質の `chkconfig`/`update-rc.d` も Medium。
- **(B) どの dimension にも載らず Low を素通り**：通常配置（`/usr/bin/*` 等）の
  `cp`/`mv`/`ln`/`install`/`update-alternatives`（信頼バイナリ置換）、`insmod`/`modprobe`/`kexec`
  （カーネルコード実行）、`useradd`/`usermod`/`visudo`（認証境界）、`wipefs`/`blkdiscard`/`lvremove`
  （ディスク低レベル）、`iptables`/`grub-install`（FW/boot）、`setcap`（能力付与）など。
- **(C) データ送信が Medium 止まり**：`curl`/`scp`/`ssh` 等。High は AI サービス系のみ。

根本原因は、名前のみで決まる破壊／システム変更系の「レベルの高低」が、本来引数パターン用の
`CheckDangerousArgPatterns`（`dangerousCommandPatterns`）に偶発的に同居し、`SystemModificationRisk`
側は名集合を一律 Medium にしているため、**どちらの dimension に載るかでレベルが割れる**点にある。

### 1.2 0139（システム変更リスクの粗粒度化）との関係

0139 は PM／systemctl を「バイナリ名マッチによる固定レベル（High）」へ単純化した。本タスクは
その**名マッチ・固定レベル方針を、破壊／システム変更系の名集合全体へ一貫適用する延長**である。
0139 の承認済み AC を原則踏襲しつつ、**0139 AC-06（fdisk/mkfs=Medium 維持）の実装乖離
（実装は既に High）を本タスクで訂正する**（F-007）。

### 1.3 設計判断（要件確定前の意思決定）

要件作成にあたり以下を決定済み（2026-06-18, isseis）。根拠列は原則 [00_notes.md](00_notes.md) の
対応 D 番号を指すが、**本書で新規に確定した決定（例 D13）は本書内の AC を参照**する。

| # | 決定 | 根拠 |
|---|---|---|
| D1 | **2 軸モデル**：最終リスク = `max(軸1: コマンド階級[name固定], 軸2: 呼び出し危険度[arg/宛先ゾーン])` | 論点1補足 |
| D2 | **改訂版・統一原則**（下記）を採用。High に **④信頼境界の破壊**を新規追加 | 論点1補足 |
| D3 | **Critical の尖鋭化**：任意内側コマンドを透過実行する特権昇格ラッパ（sudo/su/pkexec/doas/runuser/setpriv/capsh 等。AC-23）に限定。無条件ブロック＝実行不可を意味する | 発見事項D-1, 論点5 |
| D4 | **ロケーション定義コマンドは 3 ゾーン・パス関数**（trust-critical→High / ordinary→Medium / safe-zone→Low）。Low 化は安全要件を満たす場合のみ | 論点4補足 |
| D5 | **精密化の線引き**：package manager=一律名前のみ粗粒度／基本コマンド（cp/mv/mkdir/rmdir/ln 等）=引数解析。基準＝フラグ安定性 × 使用頻度 | 論点4補足 |
| D6 | **権限付与／認証境界系（visudo/useradd 等）= High**（Critical にしない） | 発見事項D-1 |
| D7 | **`rm`/`rmdir`/`shred`/`dd`/`unlink` = 宛先ゾーン+arg 化**（現状 name のみ High からの変更） | 発見事項D-3 |
| D8 | **カーネルモジュール（insmod/modprobe/rmmod/kexec）= High**（Critical にしない） | 論点5 |
| D9 | **データ送信（外部送信）= Medium 据え置き**。AI⇔データ送信の非対称は既知の限界として doc 明記 | 論点1補足 |
| D10 | **0139 AC-06 乖離は本タスクで訂正**（fdisk/mkfs/parted/fsck=High を正とする） | 論点6 |
| D11 | **「検討」群の配置を確定**（F-002/F-003/F-004 の各 AC へ反映） | 論点1補足 |
| D12 | **`update-alternatives`=名前のみ High（intrinsic）／`install`=arg 条件（宛先ゾーン＋setuid/setgid/所有者変更→High。cp/mv 類）** | 発見事項B/D |
| D13 | **安全な TOML 代替がある実行ラッパは High（redundant-with-config）**：`env`（→ `env_vars`/`env_import`）・`timeout`（→ `timeout`）は直接呼び出し不要で難読化/注入ベクタのため High。内側コマンドは間接実行解析で引き続きゲート（max）。Critical にはしない（D3）。代替の無いラッパ（nice/ionice/stdbuf/setsid）は据え置き | AC-29a |

**改訂版・統一原則（境界の再定義）**:

```
最終リスク = max(軸1: コマンド階級[name固定], 軸2: 呼び出し危険度[arg/宛先ゾーン])

critical — 任意内側コマンドを透過実行する特権昇格ラッパ（sudo/su/pkexec/doas/runuser/setpriv/capsh 等。AC-23）。無条件ブロック。
high     — ①device/FS/ツリー粒度の不可逆破壊（能力 or 危険arg）
           ②永続的 system/boot/service/account/auth 変更
           ③高権限での任意コード実行（kernelモジュール, dlsym/LD_PRELOAD, interpreter, AI駆動）
           ④信頼境界の破壊（信頼バイナリ/設定の置換, allowlist+ハッシュ無力化）
           ⑤権限/能力付与（setcap/setfacl/visudo/chmod-grant/chown）
medium   — 永続的だが named-file 粒度の影響（rm/mv/cp/rmdir, 非クリティカルパス）
           / データ送信（境界越え: curl/scp/ssh/rsync）
           / 定義済み・限定スコープの system 変更（mount 既定, 単一IF設定）
low      — それ以外（safe-zone 内のロケーション定義コマンド等）
```

**「大規模」の運用定義**：device/FS/ツリー粒度に作用しうる→High、named-file 粒度→Medium。

### 1.4 目的

破壊／システム変更系の名前ベース判定を、上記の 2 軸モデルと改訂統一原則に沿って**一貫化**する。
具体的には (A) 割れの解消（同類を同一レベルへ）、(B) 抜けの封鎖（新規対象の列挙とレベル付与）、
(C) Critical の尖鋭化を行い、関連するユーザー/開発者文書と既存 config を実装に追従させ、
破壊的変更（引き上げ・引き下げ双方）を移行ノートで周知する。

## 2. スコープ

- **In**:
  - 軸1（名前固定）High カテゴリの新規/引き上げ（F-001, F-002）。
  - 軸1 Medium カテゴリの確定（F-003）。
  - 軸2（宛先ゾーン／arg）＝ロケーション定義コマンドの 3 ゾーン判定と safe-zone Low 化の安全要件（F-004）。
  - Critical の尖鋭化（F-005）。
  - データ送信の Medium 据え置きと AI⇔データ送信の限界の明記（F-006）。
  - 0139 AC-06 乖離の訂正（F-007）。
  - 一貫性・統合の維持（F-008）。
  - 文書整合・移行周知・検出限界・既存 config 追従（F-009）。
- **Out**（詳細・根拠は §6）:
  - `RiskLevel` の段階定義（Low/Medium/High/Critical の意味づけ・段数）の変更。**新レベルは追加しない**。
  - データ送信（curl/scp/ssh/rsync）を High へ引き上げること（D9: Medium 据え置き）。
  - 判定構造の一元化リファクタの実施可否・0139 との実装順序（02_architecture.md で確定）。
  - `dd of=`／`mount` mountpoint／`setfacl`／宛先ゾーンの**具体パースロジック**（挙動は F-004 で規定、
    実装方式は 02_architecture.md で確定）。

## 3. 機能要件と受け入れ基準

### F-001: 大規模・不可逆破壊系の High 化（軸1・原則①）

device/FS/ボリューム/パーティション粒度の不可逆破壊をもたらし得るバイナリは、引数によらず
**High**（システム変更次元相当の理由コードを記録）に分類する。

**Acceptance Criteria**:
- **AC-01**: 解決済みバイナリ名が `parted`・`fsck`（`fsck.*` 含む）・`wipefs`・`blkdiscard`・
  `sgdisk`・`gdisk`・`cgdisk`・`sfdisk`・`cfdisk`・`mkswap` のいずれかであるコマンドは、引数によらず
  **High** に分類される（`gdisk`/`cgdisk`/`sfdisk`/`cfdisk` は任意ブロックデバイスのパーティション
  テーブルを編集する util-linux/GPT エディタで `fdisk`/`sgdisk` と同等の破壊力）。
- **AC-02**: LVM 破壊/デバイス初期化系 `lvremove`・`vgremove`・`pvremove`・`lvreduce`・`vgreduce`・
  `pvmove`・`lvresize`・`pvresize`・`pvcreate` は、引数によらず **High** に分類される
  （`lvresize`/`pvresize` は縮小＝破壊を含み得るため引数を見ず High に倒す。`pvcreate` はブロック
  デバイスへ LVM ラベル/メタデータを書き込むデバイス初期化で、全ディスク使用時はパーティション
  テーブルを消去する）。
- **AC-03**: `mkfs`（`mkfs.*` 含む）・`fdisk` は **High** に分類される（既存挙動の確定・維持。F-007 と
  整合）。device/FS を直接生成・検査・改変する**ファイルシステムツールファミリ**（フロントエンドが
  内部呼出しする直接ユーティリティ `e2fsck`/`mke2fs`/`tune2fs`/`resize2fs` 等を含む）も同水準で High
  とする（確定列挙は 02）。

### F-002: 永続的システム変更・特権コード実行・権限付与・信頼境界の High 化（軸1・原則②③④⑤）

以下の名集合は、引数によらず **High** に分類する。**各 AC の列挙はコマンドファミリの代表例であり
非有界**（脅威モデル上 backstop は allowlist + ハッシュ固定。AC-26 と整合）。

> **記述方針（WHAT/HOW の分離）**: 本書は「どのファミリをどのレベルにするか（WHAT）」を規定する。
> **個別バイナリの完全な列挙・distro 別名・引数/パス判定の具体機構（ブロックデバイス判定方式・
> フラグ解析・宛先パス解決の実装等の HOW）は 02_architecture.md / 実装で確定**する。レビューで
> 個別バイナリ（例: `e2fsck`/`sfdisk`/`vipw`/`kernel-install`/`runuser` 等）が指摘された場合も、
> 本書では該当ファミリに含める方針を示すに留め、確定列挙は 02 に委ねる。

**Acceptance Criteria**（各行が AC。**レベルはいずれも High**。「代表例」は非有界で確定列挙は 02）:

| AC | ファミリ（原則） | 代表例（非有界・確定列挙は 02） | 補足 |
|---|---|---|---|
| AC-04 | カーネル/モジュール・カーネルパラメータ（③/②） | `insmod`・`modprobe`・`rmmod`・`kexec`・`sysctl` | `sysctl` はカーネルパラメータ変更（②）。名前のみ粗粒度のため read-only `sysctl -a` も fail-safe High |
| AC-05 | アカウント・認証 DB（passwd/group/shadow/sudoers）の作成/変更/編集（②） | `useradd`・`usermod`・`userdel`・`groupadd`/`groupmod`/`groupdel`・`gpasswd`・`chpasswd`・`adduser`/`deluser`/`delgroup`・`passwd`・`chage`・`newusers`・`vipw`/`vigr`・`visudo` | — |
| AC-06 | ブートローダ/エントリ/カーネルイメージの改変（②③） | `grub-install`/`grub2-install`・`update-grub`・`grub-mkconfig`/`grub2-mkconfig`・`efibootmgr`・`kernel-install`/`installkernel` | `/boot` のカーネル/initrd 追加削除を含む |
| AC-07 | ブート時サービス有効化（②） | `chkconfig`・`update-rc.d` | `systemctl`/`service`（0139 で High）と同質 |
| AC-07a | 電源状態/ランレベル変更（②） | `shutdown`・`reboot`・`halt`・`poweroff`・`telinit` | compat 形（`shutdown now`・`reboot -f`）が `systemctl` 経路の High を迂回しないため |
| AC-08 | ファイアウォール（②） | `iptables`・`ip6tables`・`iptables-restore`/`ip6tables-restore`・`nft`・`ufw`・`firewall-cmd` | `iptables-save`/`ip6tables-save` は既定（stdout）= **Low**。`-f <file>` 出力は宛先 zoning（trust-critical なら High。無条件 Low とはしない） |
| AC-09 | 能力付与（⑤） | `setcap` | — |
| AC-10 | 信頼境界の置換 intrinsic（④） | `update-alternatives`・`dpkg-divert`・`alternatives`・`ldconfig` | system バイナリ/symlink/共有ライブラリキャッシュを intrinsic に改変。宛先によらず High（`ldconfig` はライブラリ差し替え経路） |
| AC-10a | ジョブ/遅延・transient 実行のインストール（②③） | `crontab`・`at`・`batch`・`systemd-run`（`--on-calendar` 等の transient 含む） | ゲートされない遅延任意特権実行（直接なら High の `useradd`/`chmod u+s` 等を素通りさせない）。ペイロード解析の要否は 02 |

### F-003: 限定スコープのシステム変更の Medium 化（軸1・medium 原則）

永続的だが破壊を伴わない／限定スコープのシステム変更は **Medium**。

**Acceptance Criteria**:
- **AC-11**: LVM 作成/設定系 `lvcreate`・`vgcreate`・`lvextend`・`vgchange`・`lvchange` は
  **Medium**（`pvcreate` はデバイス初期化のため AC-02 で High）。
- **AC-12**: `ip`・`ifconfig`・`route` は **Medium**（名前のみ・粗粒度。サブコマンド解析は行わない）。
  ただし `ip netns exec <NAME> <cmd> ...` および `ip vrf exec <NAME> <cmd> ...` は内側 `<cmd>` を実行する
  **間接実行形**であり、`ip` 自体の Medium ではなく**間接実行解析（内側コマンドのゲート適用）または
  High** として扱う（`ip netns exec ns rm -rf /`・`ip vrf exec red modprobe x` 等が外側 `ip` の Medium に
  埋もれないようにする）。
- **AC-13**: `mount`/`umount` の既定は **Medium** を維持する（対象が trust-critical の場合の引き上げは
  F-004 AC-19 で規定）。（注: `crontab`/`at`/`batch` は本タスクで High へ引き上げ。AC-10a 参照。）

### F-004: ロケーション定義コマンドの 3 ゾーン判定（軸2・宛先ゾーン）

宛先パス/対象によって脅威が決まる「ロケーション定義コマンド」は、宛先ゾーンの関数として
判定する：**trust-critical → High / ordinary → Medium / safe-zone → Low**。対象ファミリは
ファイルを書込/上書/削除/リンク/展開/touch するもの——`cp`・`mv`・`rm`・`rmdir`・`unlink`・`shred`・
`ln`・`mkdir`・`touch`・`install`・`tee`・`sponge`・`truncate`（FILE 切詰め/上書き）・`sed -i` 等の
**インプレース編集**・`tar`/`unzip` 等の**アーカイブ展開先**（`tar -C`/`unzip -d`、上書き展開）、および
`dd`・`mount`・`setfacl` 等（確定列挙は 02）。なお `mknod`（safe-zone 配下へのデバイスノード作成は
`/dev` ゾーニングを迂回するエイリアス生成）は宛先によらず **High**。

> アーカイブ展開の注意（02 で詰める）: 展開先ディレクトリの zoning だけでは不十分なケースがある。
> (1) **展開ルートを脱出するメンバ**（`tar -P/--absolute-names`、`unzip -:`、`../` メンバ）は
> safe-zone 内 `-C`/`-d` でも `/etc/...` を書き得るため、メンバ検査ができないなら fail-safe **High**。
> (2) **特権メタデータ/特殊ファイルの復元**（`tar -p/--same-owner/--same-permissions`、`unzip -K`(SUID/SGID)）
> は setuid-root やデバイスエイリアスを safe-zone に生成し得るため、`install`/`mknod` と同じ **High** floor。

#### 判定の構造（条件 → 最小リスク floor の max。**順序不問**）

最終リスク = **適用される全ルールが与える最小リスク（floor）の max**。**各行は独立した floor であり、
評価順序・行の並びは結果に影響しない**（max は可換）。一見「優先」に見えるものも max に還元される:
- **trust-critical > safe-zone**（AC-17(c)）: trust-critical=High・safe-zone=Low なので max が自動的に
  High を選ぶ（別個の順序規則ではない）。
- **fail-safe**（AC-18）: 「Low を禁ずる」＝ Medium の floor。max(Medium, …) と等価。

唯一 floor として素直でないのが**ツリー再帰（AC-22）と safe-zone の関係**である。「再帰＝無条件 High」
だと safe-zone 内に閉じた `rm -rf $WORKDIR/build` まで High になり、**§6 の「safe-zone 内の自己破壊は
対象外（Low）」と矛盾**する。これを floor 化するため、**AC-22 の High は再帰対象が safe-zone の外
（ordinary / trust-critical）に及ぶ場合に限る**と定義する（safe-zone 内に閉じた再帰は Low のまま）。
これで全行が純粋な floor となり順序不問が成立する。

条件は 2 軸に分かれる。**軸 A は何を付与/実行するか（対象パスに依らず High）**、**軸 B はどの
パス/デバイスへ入出力するか（ゾーン依存）**。最終リスクは両軸の全 floor の max。

**軸 A: 権限付与・コード実行（対象ゾーンに依らず High。原則⑤③）**

| 条件 | 最小リスク (floor) | 規定 AC |
|---|---|---|
| **特権付与**（setuid/setgid・world-write・trust-critical 所有権変更） | **High** | AC-20（install=AC-22a） |
| **内側コマンド実行**（`find -exec`/`-execdir` 等） | **High**（内側ゲート） | AC-22e |

**軸 B: ファイル入出力の対象（パス/デバイスのゾーン。原則①④＋ゾーン）**

| 条件（複数該当時は max） | 最小リスク (floor) | 規定 AC |
|---|---|---|
| 対象/宛先/リンク先が **trust-critical** パス | **High** | AC-14（mount/umount=AC-19、mv/rm/ln source=AC-22b、データ送信の書込=AC-25、find 起点=AC-22e） |
| **ブロックデバイス**入出力（`dd if=`/`of=`） | **High** | AC-21 |
| **safe-zone の外に及ぶツリー再帰**（`rm -r /etc`・`cp -a … /opt` 等） | **High** | AC-22 |
| **機微 source の複製**（`cp /etc/shadow` 等） | **Medium**（Low 不可） | AC-22b |
| **宛先不確定**（変数展開未確定・宛先非一意） | **Medium**（Low 不可） | AC-18 |
| 対象が **ordinary** パス（named-file） | **Medium** | AC-15 |
| 対象が **safe-zone** 内（AC-17 充足・再帰も内に閉じる） | **Low** | AC-16/AC-17 |

**Acceptance Criteria**:

_ゾーン定義と既定マッピング（AC-14〜18）_
- **AC-14**（宛先ゾーン基本）: ロケーション定義コマンドの宛先が trust-critical パス
  （`SystemCriticalPaths` 既定集合 = `/`・`/bin`・`/sbin`・`/usr`・`/usr/bin`・`/usr/sbin`・`/etc`・
  `/var`・`/var/log`・`/boot`・`/sys`・`/proc`・`/dev`・`/lib`・`/lib64`・`/root` 等、
  [types.go](../../../internal/runner/base/security/types.go) / `HasSystemCriticalPaths`
  ([command_analysis.go](../../../internal/runner/base/security/command_analysis.go)) 相当）のとき
  **High** に分類される。trust-critical 判定は **AC-17(a) と同じく正規化・symlink 解決後の絶対パスで
  行い、生の引数文字列 prefix では判定しない**。判定はパス境界単位（critical パスと一致、またはその
  配下）であり、**`/usr` が集合に含まれるため `/usr/local/bin` 等の配下も trust-critical に含まれる**。
  境界マッチ（一致 or 次文字が `/`）のため、集合中の **`/`（ルート）はルート自体にのみマッチし、
  `/srv`・`/opt` 等の他トップレベルは trust-critical にならない**（AC-15 の ordinary 例と整合）。
  例: `cp evil /usr/bin/ls`・`mv x /etc/passwd`・`ln -sf x /usr/bin/python`・`install -m755 x /usr/sbin/y`。
  （critical パス集合の正確な定義・拡張は 02 で確定。）
- **AC-15**（ordinary）: 宛先が trust-critical でも safe-zone でもない通常パスの named-file 操作は
  **Medium**。例: `rm /srv/app/cache.dat`・`cp a /opt/data/b`（`/srv`・`/opt` は `SystemCriticalPaths`
  既定集合に含まれない。**`/var`・`/var/log` は trust-critical なので ordinary の例には使わない**）。
- **AC-16**（safe-zone → Low）: 宛先が safe-zone（§ AC-17 で定義）内の named-file 操作は **Low**。
  例: run の作業ディレクトリ配下での `cp`/`mv`/`rm`/`mkdir`。
- **AC-17**（safe-zone の定義と解決, 安全要件）: safe-zone 判定は以下をすべて満たす：
  (a) **正規化済み（シンボリックリンク解決後）の絶対パス**で判定し、文字列の prefix 一致では
  判定しない（`~/link→/etc`、`$HOME/../../etc` 等で破れない）。`safefileio` の強化済み解決経路を用いる。
  (b) safe-zone は**曖昧な `$HOME` ではなく、run が指定する作業/出力ディレクトリおよび run 専用の
  private temp** に限定する。共有の `/tmp` を無条件 safe としない。
  (c) **safe-zone が trust-critical パスと重複またはその配下である場合（例: 作業ディレクトリに
  `/etc` や `/usr/bin` が指定された場合）は、safe-zone として扱わず、trust-critical 判定（High）を
  優先する**。trust-critical が safe-zone より常に優先（max 合成と整合）し、設定ミスによる
  セキュリティバイパスを防ぐ。
  (d) **TOCTOU 耐性**: 評価時の 1 回限りの symlink 解決だけでは Low 降格の根拠として不十分
  （外部 `cp`/`mv`/`rm` が後刻 open するまでに `$WORKDIR/out` が `/etc/passwd` への symlink に
  すり替えられ得る）。safe-zone 降格は、**safe-zone ディレクトリが非特権ユーザから書込/すり替え
  できない（信頼された）こと**を前提とし、満たせない場合は fail-closed（Low に降格しない）とする。
  具体的な担保方式は 02。
- **AC-18**（fail-safe: 宛先不確定なら Low にしない, 安全要件）: 宛先オペランドを確実に解決できない
  呼び出し（宛先が一意に取れない、または変数展開（`%{VAR}`）で評価時に実宛先が未確定）は
  **Low に分類しない**（最低 Medium。trust-critical 判定の上振れは維持）。
  注: 本ランナーは**シェルを介さず glob（`*`/`?`）を展開しない**ため、`*` は特殊文字ではなく単なる
  リテラルなパスオペランドとして扱われる。よって glob 展開に起因する宛先不確定は考慮不要。
_コマンド別の特則（上記ゾーンに上乗せ／例外。AC-19〜22e）_
- **AC-19**（`mount`/`umount` の対象ゾーニング）: 対象が trust-critical のとき **High**——`mount` は
  **mountpoint と source の双方**（mountpoint への shadowing、および `--bind`/`--rbind`/`--move` の
  trust-critical source・デバイス source `/dev/sdaN` を safe-zone へ別名化して後続で改変する形を含む）、
  `umount` は対象 FS/ディレクトリ（trust-critical FS の detach）。`umount -a`（全 FS アンマウント）も
  **High**。それ以外は Medium（AC-13）。bind source 等の解決方式は 02。
- **AC-20**（権限/所有権/属性の付与・変更, ⑤）: パーミッション/所有権/FS 属性を変更するファミリ
  （`setfacl`・`chmod`・`chown`・`chgrp`・`chattr`）は、**特権を付与する操作（setuid/setgid 付与、
  world-write 等の権限拡大）、完全性制御の除去（`chattr -i` で immutable 解除）、または trust-critical
  対象への変更**のとき **High**（`chmod u+s`・`chmod 4755`・`chown root /usr/bin/x`・`chattr -i /etc/shadow`
  等）。それ以外（safe-zone 内の通常モード変更）は宛先ゾーンに従う。判定基準の詳細は 02 で確定。
- **AC-21**（`dd` のデバイス入出力）: `dd` は **`if=` または `of=` がブロックデバイス（物理ディスク等）
  のとき High**（`if=/dev/sda …` の全デバイス読取、`of=/dev/sda` の全デバイス書込を含む。既存の
  `dd if=`=High 保護を維持）。**無害なシンク（`/dev/null` 等のキャラクタデバイス/疑似ファイル）は
  High に上げない**。それ以外（通常ファイル宛先）は宛先ゾーン（AC-14〜18）に従う。加えて、**機微/
  trust-critical な `if=` source の複製（`dd if=/etc/shadow of=$WORKDIR/shadow`）は safe-zone 宛先でも
  Low に降格しない**（情報露出の floor。cp の AC-22b と同型）。ブロックデバイスか否かの判定方式・
  `/dev` 配下の扱いは 02 で確定（パス文字列ではなくデバイス種別で判定する方針）。
- **AC-22**（ツリー粒度操作の arg 昇格）: `rm`/`cp`/`mv` 等が**ツリー粒度で再帰的に作用し、その対象が
  safe-zone の外（ordinary / trust-critical）に及ぶ場合**（`rm -r`/`-R`、`cp -R`/`-a`/`--archive`＝
  `-dR` 相当 等）は **High**。例: `rm -rf /etc/x`・`cp -a tree /opt/x`。**safe-zone 内に閉じた再帰
  （`rm -rf $WORKDIR/build`）は Low のまま**（§6「safe-zone 内の自己破壊は対象外」と整合。これにより
  本ルールも純粋な floor となり max 合成の順序不問が成立）。一方、**複数オペランドの指定
  （`rm file1 file2`）自体は High 昇格条件とせず**、各オペランドを個別に zoning する（AC-22b/AC-14〜
  AC-16）。本ランナーは**シェルを介さず glob を展開しない**ため、`rm *` は「`*` という名前のファイル」を
  指す単なるパスオペランドとして通常どおり zoning され、glob 起因のツリー破壊は発生しない。再帰とみなす
  フラグ集合の確定は 02。
- **AC-22a**（`install` の権限フラグ）: `install` は宛先が safe-zone であっても、`-m` に setuid/setgid
  ビット（例 `-m 4755`/`-m 2755`）を伴う、または `-o`/`-g` で所有者/グループを変更する場合は
  **High**（**Low に降格しない**）。`install -o root -m 4755 tool $WORKDIR/tool` のような setuid-root
  実行ファイル生成を safe-zone 経由で素通りさせない。権限付与軸（⑤・chmod-grant と同型）。
- **AC-22b**（作用する全オペランドの zoning）: 破壊/移動の対象オペランドをすべて zoning する。
  - `mv`: **移動元（source）と宛先（destination）の双方**を zoning 対象とする。source が trust-critical
    なら宛先が safe でも **High**（例 `mv /etc/passwd $WORKDIR/passwd` は `/etc` から trust-critical
    ファイルを除去する）。
  - `rm`/`shred`/`unlink`: 構文上「宛先」概念がなく、**削除/破壊対象となる全オペランド（source）**を
    zoning 対象とする。いずれかが trust-critical なら High。
  - `ln`: **source（リンク先）も zoning 対象**。trust-critical な source へのハード/シンボリックリンクを
    safe-zone 内に作る形（`ln /etc/passwd $WORKDIR/passwd`）は、後続の解決を回避して critical ファイルを
    別名経由で書換え得るため **High**（safe-zone 宛先でも降格しない）。
  - `cp`: source を mutate しないため宛先ゾーンで判定するが、**trust-critical/機微な source の複製
    （`cp /etc/shadow $WORKDIR/...`）は safe-zone でも Low に降格しない**（情報露出の floor。既存の
    `OutputCriticalPathPatterns` と整合）。完全な情報漏えい（read）モデル化は本タスクのスコープ外。
    さらに、**メタデータ保持コピー（`-p`/`-a`/`--preserve=mode,ownership`）で setuid/setgid・root 所有・
    capability 付き source を複製する形（`cp -a /usr/bin/sudo $WORKDIR/sudo`）は High**（safe-zone でも
    特権実行ファイルを再生成するため。`install -o root -m4755` と同型の権限付与＝⑤）。
- **AC-22c**（既存次元との整合）: safe-zone Low（AC-16）が、他次元（coreutils 単一バイナリ分類等）の
  固定 High により max 合成で打ち消されないこと。すなわち本タスクで Low/Medium へ降格すると定めた
  ロケーション定義コマンドについては、**軸2（宛先ゾーン）の判定が有効に反映される**ことを要件とする
  （降格は AC-17/AC-18 の安全要件を満たす場合に限る）。具体的な次元調整方式は 02 で確定。
- **AC-22d**（`tee`/`sponge` のファイル書き込み）: `tee`（および moreutils `sponge`）は stdin を引数の
  FILE に書き込むため、ロケーション定義コマンドとして **FILE オペランドを zoning 対象**とする。FILE が
  trust-critical（例 `echo x | tee /usr/bin/y`・`tee /etc/passwd`）なら **High**、safe-zone なら Low、
  それ以外は Medium。`tee` の非フラグ引数はすべて書き込み先 FILE（`-a`/`-i`/`-p` 等のフラグを除く）。
  **複数 FILE 指定時（`tee safe /etc/passwd`）は各 FILE を個別に zoning し max を取る**（いずれかが
  trust-critical なら High。AC-31 の max 合成と整合）。
  注: `tee` は**内側コマンドを実行しない**——脅威は信頼ファイルの上書き（④信頼境界破壊）であって
  コマンド実行ではない。宛先パースは AC-18 の fail-safe（不確定なら Low にしない）に従う。
- **AC-22e**（`find` の破壊/実行/書込アクション）: 危険アクションを伴う `find` を **Low に素通り
  させない**。
  - `-exec`/`-execdir`/`-ok`/`-okdir`（子プロセス実行）: 子ヘルパーは `find` の fork まで identity-bind
    できないため、**間接実行 Reject/Blocking**（High floor ではなく拒否）として扱う（既存 resolver の
    find/xargs 扱いを維持）。
  - `-delete`（ツリー削除）: AC-22 のツリー破壊と同等。
  - `-fprint`/`-fprint0`/`-fprintf`/`-fls`（FILE への書込）: `tee` 同様の FILE オペランド zoning
    （trust-critical 宛先なら High）。
  - 上記 **破壊/書込/実行アクションを伴う場合に**、探索起点が trust-critical なら High。**読取専用検索
    （`find /etc -name '*.conf'` 等）は本 AC では昇格しない**。アクション検出の具体は 02。

### F-005: Critical の尖鋭化

**Acceptance Criteria**:
- **AC-23**: Critical（無条件ブロック）に分類されるのは、**任意の内側コマンドを透過実行する特権昇格
  ラッパファミリ**（対象実効 UID/GID **または capability/特権設定**を変えて内側コマンドを透過実行する
  もの）とする。代表例: `sudo`・`su`・`pkexec`・`doas`・`runuser`（UID/GID 変更）、`setpriv`
  （`--ambient-caps` 等）・`capsh`（capability 設定後にシェル/プログラム起動）（確定列挙は 02）。
- **AC-24**: F-002 の権限付与/認証境界系（`visudo`・`useradd` 等）および F-001/F-002 のカーネル
  モジュール（`insmod` 等）は **High** であり Critical ではない（per-command で明示許可可能であること
  を担保し、正当な特権バッチを実行不可にしない）。

### F-006: データ送信の据え置きと AI⇔データ送信の限界の明記

**Acceptance Criteria**:
- **AC-25**: データ送信系（`curl`・`wget`・`scp`・`sftp`・`rsync`・`ssh`・`nc`）は データ送信軸で
  **Medium** を維持する（High へ引き上げない）。ただし Medium はあくまで floor であり、**これらが
  ファイル書込/削除形でローカルの trust-critical パスへ作用する場合は、ロケーション定義と同じく
  ④信頼境界破壊として High**（`curl -o /usr/bin/x`・`wget -O /etc/cron.d/x`・`rsync --delete … /etc/`
  等。cp/mv と同じ AC-14 基準で max 合成）。判定対象の書込先オペランド抽出は 02 で確定。
  さらに、**ローカルでヘルパーを実行させるオプションは データ送信は Medium に留めず間接実行として扱う**:
  `ssh -o ProxyCommand=…`・`-o PermitLocalCommand=yes -o LocalCommand=…`、および `rsync -e/--rsh=COMMAND`
  （リモートシェルとして任意ローカルコマンドを実行）は内側ゲート/拒否の対象とする。オプション検出の
  具体は 02。
- **AC-26**（検出限界の明記）: 名前ベース AI 検出（`claude`/`gemini` 等 = High）は 一般的なデータ送信
  （`curl <AI エンドポイント>` 等, Medium）を塞ぐものではなく salient な明示ケースの defense-in-depth で
  あること、ならびに未列挙コマンド・リネームバイナリ・multi-call 形式が本次元を素通りし得ること
  （安全運用は allowlist + ハッシュ固定が前提。0136 AC-66/67 と整合）を文書化する。

### F-007: 0139 AC-06 乖離の訂正

**Acceptance Criteria**:
- **AC-27**: 0139 AC-06（fdisk/mkfs=Medium 維持）が実装（fdisk/mkfs=High）と乖離している点を、
  **本タスクで fdisk/mkfs/parted/fsck=High を正**として訂正する。関連ユーザー/開発者文書から
  「fdisk/mkfs=Medium」の記述を除去し、High へ更新する（0139 のドキュメントは触らず、本タスクの
  移行ノートで上書き関係を明示）。

### F-008: 一貫性・統合の維持

**Acceptance Criteria**:
- **AC-28**: 同一コマンドに対し、実行時（runtime）と dry-run で同一のリスク分類となる。
- **AC-29**: ラッパー/間接実行経由の判定が維持される。例: `env modprobe x` は High 以上、
  `sudo useradd u` は **Critical**（特権昇格）に分類される。内側コマンドを実行する**名前空間/ルート
  変更ラッパファミリ**（`chroot`・`unshare`・`nsenter` 等）も間接実行として内側コマンドをゲートし
  外側で素通りさせない（`unshare -r`・`nsenter -t 1 …` 等の特権/名前空間エスケープ形は High 以上）。
  **COMMAND を省略した形（`chroot /mnt`・`unshare`・`nsenter -t 1 -m` 等）は暗黙の対話シェル
  （`$SHELL -i`/`/bin/sh -i`）を起動する**ため、内側コマンド未指定でも素通りさせず High 以上として扱う。
  またコマンドを引数で受けて実行する**コマンド文字列ラッパ**（`flock <file> <cmd>`/`flock -c <cmd>`・
  `watch <cmd>` 等）も間接実行ファミリに含める。これらラッパの解析対象と確定列挙は 02。
- **AC-29a**（安全な TOML 代替がある実行ラッパは High — redundant-with-config 原則, D13）:
  効果を TOML スキーマでより安全に表現できる実行ラッパは **High** に分類する。対象と代替フィールド:
  `env`（→ `env_vars`/`env_import` による環境変数設定）、`timeout`（→ `timeout` フィールド）。
  これらは直接呼び出しが不要であり、コマンド名の難読化・環境変数注入の難読化ベクタになるため、
  `env FOO=bar ls`・`timeout 10 ls` のような benign 形も含め High とする（過小分類より hygiene を優先）。
  - 内側コマンドは間接実行解析（`wrapperSpecs`,
    [indirect_execution.go](../../../internal/runner/base/security/indirect_execution.go)）で引き続き
    ゲートされ、最終リスクは **max**（`env dpkg -i`→High、`sudo env ...`→Critical）。
  - **Critical にはしない**（D3: Critical は特権昇格ラッパに限定＝無条件ブロック。`env`/`timeout` は
    権限を昇格しないため High に留め、稀な正当用途の escape hatch を残す）。
  - `env` 経由の loader 制御変数（`LD_PRELOAD`・`LD_LIBRARY_PATH`・`LD_AUDIT`・`LD_DEBUG` 等、信頼
    バイナリへのコード注入＝④信頼境界破壊）は従来どおり **forbidden-env-var
    （`ReasonForbiddenEnvVar`）として拒否**される。
  - **代替の無いラッパ（`nice`・`ionice`・`stdbuf`・`setsid` 等）には redundant-with-config 由来の
    追加 floor を課さない**（TOML 等価物が無いため）。ただし**全ての抽出可能なラッパは、外側のみが
    検証され内側が hash-gate されないため、既存の間接実行解析による flat High floor を維持する**
    （`nice ./unverified-tool` が低リスク許可で未検証バイナリを実行しないよう、内側＝High floor。
    [indirect_execution.go](../../../internal/runner/base/security/indirect_execution.go) の RoleInner）。
    D13 の `env`/`timeout` High はこの floor と整合（加えて redundant-with-config の観点でも明示 High）。
- **AC-30**: 本タスクで引き上げ/変更されたコマンドが deny されたとき、監査ログに対応する理由
  （システム変更・破壊的操作・危険引数パターン等の理由コード）が記録される。
- **AC-31**: 最終リスクは適用 dimension の **max** で合成され、軸1（名前固定）と軸2（arg/宛先ゾーン）の
  両方が適用されるコマンドはその最大値となる（例: `cp` が trust-critical 宛先かつ再帰のとき High）。

### F-009: 文書整合・移行周知・既存 config 追従

**Acceptance Criteria**:
- **AC-32**（移行周知・引き上げ）: 本変更で **Low/Medium → High** に引き上がるコマンド群
  （F-001〜F-002, F-004 の trust-critical ケース）により、従来許可していた config がブロックされ得る
  ことを移行ノートとして文書化する。安全運用は allowlist + ハッシュ固定 + 明示的な `risk_level` 設定が
  前提であることを併記する。
- **AC-33**（移行周知・引き下げ）: 本変更で **High → Medium/Low** に引き下がるコマンド
  （`rm`/`rmdir`/`shred`/`unlink`/`dd` の safe-zone/ordinary ケース, D7）を**セキュリティ緩和方向の変更**
  として移行ノートに明示する（baseline は直近リリースの挙動）。
- **AC-34**: [docs/user/risk_assessment.md](../../../docs/user/risk_assessment.md)（および日本語版・
  用語集 `docs/translation_glossary.md`、開発者向け `command-risk-evaluation.{ja,}.md`）を、本タスクの
  分類（軸1 High/Medium 名集合・軸2 3 ゾーン・Critical 尖鋭化）に一致するよう更新する。
- **AC-35**: 本変更で分類が引き上がるコマンドを使用する既存の sample／テスト用 config が、新しい
  レベルの下でも整合する（必要な `risk_level` 設定が付与されている）よう更新・検証する
  （0139 AC-14 と同型）。
- **AC-36**（リスクレベル分類ガイドの最終化）: アーキテクト/SRE 向け概念ガイド
  [risk-level-classification-guide.ja.md](../../../docs/dev/architecture_design/risk-level-classification-guide.ja.md)
  を、最終確定した分類（軸1/軸2・max 合成・Critical 尖鋭化・安全要件等）に一致するよう内容を確定し、
  **英語版 `risk-level-classification-guide.md` を `/mktrans` で作成**する（日本語を先に確定・コミット
  → 英語版へ反映。CLAUDE.md のバイリンガル方針・翻訳ガイドラインに従う）。本ガイドの記述が本要件
  （01）・実装と齟齬しないこと。

- **NF-001**: 新規に理由コードを追加する場合、`ReasonCode` の網羅性テスト（[reason_codes.go](../../../internal/runner/base/risktypes/reason_codes.go)
  の exhaustive/distinct 検証）に追従する。
- **NF-002**: `make test`・`make lint`・`make fmt` がすべて成功する。
- **NF-003**: 判定は決定的で副作用がない（同一入力に対し常に同一レベル。ファイルシステムへの
  書き込みを伴わない。safe-zone 判定のパス解決は読取のみ）。

## 5. 影響範囲（実装時の参考、本要件では確定事項ではない）

- [command_analysis.go](../../../internal/runner/base/security/command_analysis.go):
  `SystemModificationRisk`／`systemModificationCommandNames`／`destructiveCommandNames`／
  `dangerousCommandPatterns`／`HasSystemCriticalPaths`。名集合の再編（High/Medium の明示化）と、
  名前のみエントリの `dangerousCommandPatterns` からの移設（論点2、実施可否は 02 で確定）。
- 軸2 の宛先ゾーン判定（safe-zone 解決）は新規。`safefileio` のパス解決を再利用（DRY）。
- [evaluator.go](../../../internal/runner/base/risk/evaluator.go) `evaluateDimensions`: 新 dimension/
  名集合の組み込みと max 合成。
- テスト: [evaluator_test.go](../../../internal/runner/base/risk/evaluator_test.go) ほかへ、新規/変更
  コマンドのレベル表明テストを追加。

詳細設計は 02_architecture.md（要件承認後に作成）で確定する。

## 6. スコープ外の根拠

- **ユーザー自身のデータ自己破壊の保護**: 本ツールはシステムの完全性と信頼境界の保護を主目的とする。
  そのため、**safe-zone（指定 workdir・出力先・private temp 等）内におけるユーザー自身によるデータの
  自己破壊（例: `rm -rf $WORKDIR/`、safe-zone 配下の上書き）の防止は守備範囲外（Out of Scope）**とする。
  これは AC-16（safe-zone での Low 化）の設計前提であり、safe-zone 内の破壊的操作が Low となる根拠でもある
  （[00_notes.md](00_notes.md) のスコープ判断と一致）。なお `~/`（`$HOME`）は safe-zone に含めないため、
  `rm -rf ~/` は safe-zone 外への再帰削除＝ AC-22 で **High**（自己破壊スコープ外の例としては不適切）。
- **`RiskLevel` の段階定義変更・新レベル追加**: Low/Medium/High/Critical の意味づけ・段数は維持。
  本タスクは「どのコマンドをどのレベルに分類するか」を一貫化するのみ。特に kernel/auth を Critical に
  しないのは、Critical=無条件ブロック（実行不可）であり正当な特権バッチ用途を壊すため（D6/D8）。
- **データ送信の High 化**: AI⇔データ送信の非対称（AC-26）は既知の限界として受容し、データ送信一般は Medium
  据え置き（D9）。名前ベースでは 一般的なデータ送信と AI 利用を確実には区別できず、過剰な引き上げは
  日常的なデータ転送を阻害する。
- **判定構造の一元化リファクタ・arg パースの具体ロジック**: 観測可能な分類（本要件の AC）と実装方式を
  分離する。リファクタ実施可否・0139 との順序・`dd of=`/`mount`/`setfacl`/宛先ゾーンのパース実装は
  02_architecture.md で確定する。
