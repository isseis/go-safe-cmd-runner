# 判断軸1: コマンド名分類の一貫化 — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-19 |
| Review date | 2026-06-20 |
| Reviewer | isseis |
| Comments | - |

> 本書は 0140 を 3 分割した第 1 タスク（判断軸1＝コマンド名分類）の要件である。分割方針・根本原因への対処は
> [0140/00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md)、原典の確定要件・
> 根拠は [0140/01_requirements.md](../0140_risk_level_classification_review/01_requirements.md)（superseded）を
> 参照する。本書は判断軸1 分のみを再掲・精緻化する。宛先パスの解析を伴う判定（判断軸2）は 0142、段階
> ロールアウト・監査・文書は 0143 に属する。

## 1. 背景と目的

コマンド名でリスクレベルが決まる破壊／システム変更系コマンドの分類を、改訂統一原則
（[0140/01](../0140_risk_level_classification_review/01_requirements.md) §1.3）へ一貫化する。本タスクは
argv の宛先解析を伴わない「コマンド名分類（判断軸1）」——コマンド名だけでリスクレベルが決まる（引数は見ない）
分類——と、内側コマンドを透過実行するラッパ/特権昇格の判定に限定する。判断軸2（0142, 宛先パス信頼区分）が
「引数中のファイルパスの信頼区分で決まる」のと対になる。具体的には (A) 同類の割れの解消（同等の破壊力を同一 High へ）、
(C) Critical の限定（特権昇格ラッパのみ）、env/timeout 等 redundant-with-config ラッパの High 化を行う。

最終リスクは既存どおり適用される判定の max。本タスクは「コマンド名で決まるレベル」を整理し、判断軸2（0142）と
max 合成される。

## 2. スコープ

- **In**:
  - 大規模・不可逆破壊系の High 化（F-001）。
  - 永続システム変更・特権コード実行・権限付与・信頼境界の High 化（F-002）。
  - 限定スコープのシステム変更の Medium 化（F-003）。
  - Critical の限定＝特権昇格ラッパのみ（F-005）。
  - データ送信の Medium 据え置きと、ヘルパー実行オプションの間接実行扱い（F-006 のうち名前/ラッパ部分）。
  - 0139 AC-06 乖離（fdisk/mkfs=Medium）の訂正（F-007）。
  - ラッパー/間接実行経由の整合維持と env/timeout の High 化（F-008 のうちラッパ部分）。
  - 検出限界（名前ベース AI vs データ送信）の文書化（F-006 の doc 部分）。
- **Out**:
  - 宛先パスの信頼区分判定（trust-critical/ordinary/safe-zone）と、書込先オペランド抽出を伴う判定は 0142 が担当。
    特に `curl -o`/`scp` 等のローカル trust-critical 書込は High（0142）（本タスクはデータ送信の Medium 据え置きと
    ヘルパー実行の間接化のみ）。
  - オペランド毎の監査フィールド・移行ノート（changelog）・文書整合・sample config 追従・ガイド最終化は 0143 が担当。
    （後方互換不要のため段階ロールアウト/フラグは設けない。）
  - `RiskLevel` の段数/意味づけ変更（新レベル追加しない）。

## 3. 横断制約（0140/00_decomposition.md §3 を継承）

- **新分類は直接適用する（後方互換不要）**: 本プロジェクトは後方互換性を要求しないため、ロールアウトフラグ／
  shadow 機構は設けない。本タスクの enforce 引き上げ（判断軸1 High 化・env/timeout High・特権/ラッパ拡張）は
  新分類を直接適用（in-place 置換）する。破壊的変更（raise/lower）の周知は 0143 の移行ノート（changelog）に
  委ねる（[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §3.2）。
- **組み込みをフェーズ内に含める（根本原因4）**: 名前集合の追加に加え、判定が実際に `EvaluateRisk` の固定レベルへ
  反映されるよう、リスク評価ロジック（`risk/evaluator.go`）・間接実行（`security/indirect_execution.go`）・特権 profile への
  組み込みを本タスクのスコープに含める。完了基準は触れる統合パッケージをコンパイルする範囲（`./internal/runner/...`
  または `make test`）とする。
- **English ソース**: Go の識別子・コメント・文字列リテラルは英語（テストソース含む）。

## 4. 機能要件と受け入れ基準

> 各 AC は 0140/01 の対応 AC を継承する（末尾「対応」列）。代表例は網羅しきれない・確定列挙は実装で（WHAT/HOW 分離）。

### F-001: 大規模・不可逆破壊系の High 化

- **AC-01**: `parted`・`fsck`(`fsck.*`)・`wipefs`・`blkdiscard`・`sgdisk`・`gdisk`・`cgdisk`・`sfdisk`・`cfdisk`・
  `mkswap` は引数によらず High。（0140 AC-01）
- **AC-02**: LVM 破壊/デバイス初期化系 `lvremove`・`vgremove`・`pvremove`・`lvreduce`・`vgreduce`・`pvmove`・
  `lvresize`・`pvresize`・`pvcreate` は引数によらず High。（0140 AC-02）
- **AC-03**: `mkfs`(`mkfs.*`)・`fdisk`、および直接 FS ユーティリティ `e2fsck`/`mke2fs`/`tune2fs`/`resize2fs` 等は
  High。（0140 AC-03）

### F-002: 永続システム変更・特権コード実行・権限付与・信頼境界の High 化

- **AC-04**: カーネル/モジュール・パラメータ `insmod`・`modprobe`・`rmmod`・`kexec`・`sysctl` は High（引数に
  よらず。名前のみ粗粒度のため read-only な `sysctl -a` も fail-safe High＝引数で例外を作らない）。（0140 AC-04）
- **AC-05**: アカウント・認証 DB 系 `useradd`/`usermod`/`userdel`/`groupadd`/.../`passwd`/`chage`/`newusers`/
  `vipw`/`vigr`/`visudo` は High。（0140 AC-05）
- **AC-06**: ブートローダ/エントリ/カーネルイメージ改変 `grub-install`/`grub2-*`/`update-grub`/`efibootmgr`/
  `kernel-install`/`installkernel` は High。（0140 AC-06）
- **AC-07**: ブート時サービス有効化 `chkconfig`・`update-rc.d` は High（`systemctl`/`service` と同質）。（0140 AC-07）
- **AC-08**: 電源状態/ランレベル `shutdown`・`reboot`・`halt`・`poweroff`・`telinit` は High。（0140 AC-07a）
- **AC-09**: ファイアウォール `iptables`・`ip6tables`・`(ip6)tables-restore`・`nft`・`ufw`・`firewall-cmd` は
  High。`iptables-save`/`ip6tables-save`（stdout）は既定 Low（`-f <file>` 出力の宛先のパス信頼区分判定は 0142）。（0140 AC-08）
- **AC-10**: 能力付与 `setcap` は High。（0140 AC-09）
- **AC-11**: 信頼境界の置換 intrinsic `update-alternatives`・`dpkg-divert`・`alternatives`・`ldconfig` は
  High（宛先によらず）。（0140 AC-10）
- **AC-12**: ジョブ/遅延・transient 実行 `crontab`・`at`・`batch`・`systemd-run` は High。（0140 AC-10a）

### F-003: 限定スコープのシステム変更の Medium 化

- **AC-13**: LVM 作成/設定系 `lvcreate`・`vgcreate`・`lvextend`・`vgchange`・`lvchange` は Medium。（0140 AC-11）
- **AC-14**: `ip`・`ifconfig`・`route` は Medium（名前のみ・粗粒度）。ただし `ip netns exec <NAME> <cmd>`・
  `ip vrf exec <NAME> <cmd>` は内側 `<cmd>` の間接実行（AC-22 と同じ間接実行ファミリ）として扱い、最終リスク = 内側 `<cmd>` の評価値を
  High 以上に引き上げる（下限 High）（例: `ip netns exec ns rm -rf /` は 内側評価かつ最低 High、`ip netns exec ns modprobe x` は High）。
  内側を抽出できない形は Reject。（0140 AC-12）
- **AC-15**: `mount`/`umount` の既定は Mediumを維持する（対象 trust-critical の引き上げは 0142）。（0140 AC-13）

### F-005: Critical の限定

- **AC-16**: Critical（無条件ブロック）は任意の内側コマンドを透過実行する特権昇格ラッパに限定する。
  代表例: `sudo`・`su`・`pkexec`・`doas`・`runuser`・`setpriv`・`capsh`。現状の特権 profile は `sudo`/`su`/`doas`
  のみのため、`pkexec`/`runuser`/`setpriv`/`capsh` を特権 profile へ新規登録し、直接呼び出し
  （`/usr/bin/pkexec …`）が `EvaluateRisk` の特権ランクで Critical になること、ネスト形（`env pkexec …` 等）も
  Critical になることを担保する。（0140 AC-23）
- **AC-17**: F-002 の権限付与/認証境界系（`visudo`/`useradd` 等）・カーネルモジュール（`insmod` 等）は High
  であり Critical ではない（per-command の明示許可で正当な特権バッチを実行可能に保つ）。（0140 AC-24）

### F-006: データ送信の据え置きとヘルパー実行・検出限界

- **AC-18**: データ送信系 `curl`・`wget`・`scp`・`sftp`・`rsync`・`ssh`・`nc` はデータ送信の判断軸で Medium を維持
  （High へ引き上げない）。ローカル trust-critical 書込形（`curl -o /usr/bin/x` 等）の High 化は 0142 の所掌。
  本タスクでは Medium 据え置きのみを担保する。（0140 AC-25 のうちデータ送信 baseline 部分）
- **AC-19**: ローカルでヘルパーを実行させるオプション名を検出し間接実行として扱う: `ssh -o ProxyCommand=…`／
  `-o LocalCommand=…`・`rsync -e`/`--rsh=COMMAND`。最終リスク = 内側コマンド文字列を抽出して内側ゲート（評価値、
  下限 High）、抽出不能なら Reject。これはオプション名の検出であり宛先パスの信頼区分解析ではない（判断軸2 と
  区別。本タスクの「argv の宛先解析を伴わない」は宛先/オペランドのパス信頼区分判定を指し、内側コマンド文字列を選ぶ
  オプション検出は既存の間接実行解析の範疇）。（0140 AC-25 のヘルパー実行部分）
- **AC-20**（検出限界の記録, `static`）: 名前ベース AI 検出（`claude`/`gemini` 等 = High）は一般的なデータ送信
  （Medium）を塞ぐものではなく salient な明示ケースの defense-in-depth であること、未列挙/リネーム/multi-call が
  素通りし得ること（安全運用は allowlist＋ハッシュ固定前提）を記録する。本タスクでは開発者向けドキュメント
  （`command-risk-evaluation.ja.md` 等）に本限界を追記するに留め、ユーザー向け文書の最終整合・日英反映・移行
  ノートは 0143 が所有する（本 AC は doc 追記の有無を `static` 検証）。（0140 AC-26）

### F-007: 0139 AC-06 乖離の訂正

- **AC-21**: 0139 AC-06（fdisk/mkfs=Medium 維持）と実装の乖離を、fdisk/mkfs/parted/fsck=High を正として
  訂正する（`parted`/`fsck` を Medium から High に引き上げる）。0139 のドキュメントは触らず、訂正の文書反映は
  0143。（0140 AC-27）

### F-008: ラッパ/間接実行の整合と env/timeout の High 化

- **AC-22**: ラッパー/間接実行経由の判定: `env modprobe x` は High、`sudo useradd u` は Critical。
  名前空間/ルート変更ラッパ `chroot`・`unshare`・`nsenter`、コマンド文字列ラッパ `flock`・`watch` も間接実行
  ファミリに追加し（現状 `wrapperSpecs` に未登録＝新規配線）、内側コマンドをゲートして外側で素通りさせない:
  最終リスク = 内側評価値をHigh 以上に引き上げる（下限 High）（`flock f cmd`・`watch cmd`・`unshare -r <cmd>`・`nsenter -t 1 <cmd>`）。
  COMMAND を省略した形（`chroot /mnt`・`unshare`・`nsenter -t 1 -m`）は暗黙シェル起動とみなし内側未指定でも
  High 以上（`unshare -r`・`nsenter -t 1` 等の特権/名前空間エスケープ形を素通りさせない）。（0140 AC-29）
- **AC-23**: 安全な TOML 代替がある実行ラッパ `env`（→ `env_vars`/`env_import`）・`timeout`（→ `timeout`）は
  直接呼び出しを High に分類する（無害に見える形も含む）。内側は間接実行で引き続きゲート（`env dpkg -i` は High、
  `sudo env …` は Critical）。Critical にはしない。代替の無いラッパ（`nice`/`ionice`/`stdbuf`/`setsid`）には
  redundant 由来の追加の下限を課さないが、抽出可能ラッパ内側の一律 High 下限は維持。`env` 経由の loader 制御
  変数（`LD_PRELOAD` 等）は従来どおり forbidden-env-var で拒否。（0140 AC-29a）

## 5. 非機能要件

- **NF-001**: 本タスクが追加した `ReasonCode` は、本タスク内で網羅性/一意性テスト（`reason_codes_test.go`）を
  緑に保つ（各タスクが自分のコードを登録。監査ストリームの family 区別の最終化は 0143。
  [00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §4）。本タスクが引き上げ/
  変更したコマンドが deny されたとき、対応する理由コードが評価結果に付与される。
- **NF-002**: `make test`・`make lint`・`make fmt` がすべて成功する。完了基準は本タスクが触れる統合パッケージ
  （`internal/runner`・必要なら `internal/runner/config`）をコンパイルする範囲（`./internal/runner/...` または
  `make test`）とする。（0140 NF-002）
- **NF-003**（横断 NF: AC-28 runtime==dry-run を含む）: コマンド名分類の判定は決定的で副作用がなく、runtime と
  dry-run で同一（名前ベースは FS/identity に依存しないため自明に満たす）。AC-28 は全タスク横断 NF であり、
  パス解決/identity の決定性サブケースは 0142 が主担当（本タスクは判断軸1 分を担保）。（0140 NF-003／AC-28）

## 6. スコープ外の根拠

- **宛先パス信頼区分の判定は 0142**: trust-critical/ordinary/safe-zone の判定、書込先オペランド抽出、ローカル書込の High 化は
  argv 解析を要し、判断軸2（0142）の所掌。本タスクはコマンド名/ラッパで決まるレベルに限定する（D5 の線引き）。
- **監査/文書は 0143**: 引き上げ・引き下げの周知（移行ノート）、オペランド毎の監査フィールド、文書整合は横断成果物
  として 0143 に集約する。後方互換不要のため段階ロールアウト/フラグは設けない（根本原因3 の解消, §3.2）。
- **`RiskLevel` 段数/新レベル**: 変更しない（0140 §6 を継承）。
