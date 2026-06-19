# 軸1: 名前固定階級リスクの一貫化 — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-19 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は 0140 を 3 分割した第 1 タスク（**軸1＝名前固定階級**）の要件である。分割方針・root-cause 対処は
> [0140/00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md)、原典の確定要件・
> 根拠は [0140/01_requirements.md](../0140_risk_level_classification_review/01_requirements.md)（superseded）を
> 参照する。本書は**軸1 分のみ**を再掲・精緻化する。**宛先パスの解析を伴う判定（軸2）は 0142**、**段階
> ロールアウト・監査・文書は 0143** に属する。

## 1. 背景と目的

名前で固定レベルが決まる破壊／システム変更系コマンドの分類を、改訂統一原則（[0140/01](../0140_risk_level_classification_review/01_requirements.md) §1.3）へ
一貫化する。本タスクは **argv の宛先解析を伴わない**「名前固定階級（軸1）」と、内側コマンドを透過実行する
**ラッパ/特権昇格**の判定に限定する。具体的には (A) 同類の割れの解消（同等の破壊力を同一 High へ）、
(C) Critical の尖鋭化（特権昇格ラッパ限定）、env/timeout 等 redundant-with-config ラッパの High 化を行う。

最終リスクは既存どおり適用 dimension の **max**。本タスクは「名前で決まる固定レベル」を整理し、軸2（0142）と
max 合成される。

## 2. スコープ

- **In**:
  - 大規模・不可逆破壊系の High 化（F-001）。
  - 永続システム変更・特権コード実行・権限付与・信頼境界の High 化（F-002）。
  - 限定スコープのシステム変更の Medium 化（F-003）。
  - Critical の尖鋭化＝特権昇格ラッパ限定（F-005）。
  - データ送信の Medium 据え置きと、ヘルパー実行オプションの間接実行扱い（F-006 のうち**名前/ラッパ**部分）。
  - 0139 AC-06 乖離（fdisk/mkfs=Medium）の訂正（F-007）。
  - ラッパー/間接実行経由の整合維持と env/timeout の High 化（F-008 のうち**ラッパ**部分）。
  - 検出限界（名前ベース AI vs データ送信）の文書化（F-006 の doc 部分）。
- **Out**:
  - **宛先パスのゾーン判定（trust-critical/ordinary/safe-zone）と、書込先オペランド抽出を伴う判定** → 0142。
    特に `curl -o`/`scp` 等の**ローカル trust-critical 書込→High** は 0142（本タスクはデータ送信の Medium 据え置きと
    ヘルパー実行の間接化のみ）。
  - **段階ロールアウト（shadow/audit-only・デプロイ可能フラグ）・per-operand 監査フィールド・移行ノート・
    sample config・ガイド最終化** → 0143。
  - `RiskLevel` の段数/意味づけ変更（新レベル追加しない）。

## 3. 横断制約（0140/00_decomposition.md §3 を継承）

- **ロールアウト切替点は本タスクが用意する（根本原因3／依存逆転の解消）**: 本タスクが**最小の切替点**
  `RolloutMode ∈ {off, shadow, enforce}`（既定 `off`、constructor 注入）を導入し（[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §3.2(1)）、
  本タスクの全 enforce 引き上げ（軸1 High 化・env/timeout High・特権/ラッパ拡張）は**この切替点経由でのみ**
  enforce へ反映する。旧 enforce 経路を保持し in-place 破壊置換をしない。テストは `off`（旧 baseline 不変）／
  `enforce`（新挙動）の両モードを持つ。**切替点を TOML へ結線したデプロイ可能設定・`shadow` の差分ログは 0143**
  が所有する（本タスクは切替点の導入と自タスク変更の切替点経由の反映までを担保し、後続タスクの成果物に依存しない）。
- **結線をフェーズ内に含める（根本原因4）**: 名前集合の追加に加え、判定が実際に `EvaluateRisk` の固定レベルへ
  反映されるよう、評価器（`risk/evaluator.go`）・間接実行（`security/indirect_execution.go`）・特権 profile への
  結線を本タスクのスコープに含める。完了基準は触れる統合パッケージをコンパイルする範囲（`./internal/runner/...`
  または `make test`）とする。
- **English ソース**: Go の識別子・コメント・文字列リテラルは英語（テストソース含む）。

## 4. 機能要件と受け入れ基準

> 各 AC は 0140/01 の対応 AC を継承する（末尾「対応」列）。代表例は非有界・確定列挙は実装で（WHAT/HOW 分離）。

### F-001: 大規模・不可逆破壊系の High 化

- **AC-01**: `parted`・`fsck`(`fsck.*`)・`wipefs`・`blkdiscard`・`sgdisk`・`gdisk`・`cgdisk`・`sfdisk`・`cfdisk`・
  `mkswap` は引数によらず **High**。（0140 AC-01）
- **AC-02**: LVM 破壊/デバイス初期化系 `lvremove`・`vgremove`・`pvremove`・`lvreduce`・`vgreduce`・`pvmove`・
  `lvresize`・`pvresize`・`pvcreate` は引数によらず **High**。（0140 AC-02）
- **AC-03**: `mkfs`(`mkfs.*`)・`fdisk`、および直接 FS ユーティリティ `e2fsck`/`mke2fs`/`tune2fs`/`resize2fs` 等は
  **High**。（0140 AC-03）

### F-002: 永続システム変更・特権コード実行・権限付与・信頼境界の High 化

- **AC-04**: カーネル/モジュール・パラメータ `insmod`・`modprobe`・`rmmod`・`kexec`・`sysctl` → **High**（引数に
  よらず。名前のみ粗粒度のため read-only な `sysctl -a` も fail-safe High＝引数で例外を作らない）。（0140 AC-04）
- **AC-05**: アカウント・認証 DB 系 `useradd`/`usermod`/`userdel`/`groupadd`/.../`passwd`/`chage`/`newusers`/
  `vipw`/`vigr`/`visudo` → **High**。（0140 AC-05）
- **AC-06**: ブートローダ/エントリ/カーネルイメージ改変 `grub-install`/`grub2-*`/`update-grub`/`efibootmgr`/
  `kernel-install`/`installkernel` → **High**。（0140 AC-06）
- **AC-07**: ブート時サービス有効化 `chkconfig`・`update-rc.d` → **High**（`systemctl`/`service` と同質）。（0140 AC-07）
- **AC-08**: 電源状態/ランレベル `shutdown`・`reboot`・`halt`・`poweroff`・`telinit` → **High**。（0140 AC-07a）
- **AC-09**: ファイアウォール `iptables`・`ip6tables`・`(ip6)tables-restore`・`nft`・`ufw`・`firewall-cmd` →
  **High**。`iptables-save`/`ip6tables-save`（stdout）は既定 **Low**（`-f <file>` 出力の宛先 zoning は 0142）。（0140 AC-08）
- **AC-10**: 能力付与 `setcap` → **High**。（0140 AC-09）
- **AC-11**: 信頼境界の置換 intrinsic `update-alternatives`・`dpkg-divert`・`alternatives`・`ldconfig` →
  **High**（宛先によらず）。（0140 AC-10）
- **AC-12**: ジョブ/遅延・transient 実行 `crontab`・`at`・`batch`・`systemd-run` → **High**。（0140 AC-10a）

### F-003: 限定スコープのシステム変更の Medium 化

- **AC-13**: LVM 作成/設定系 `lvcreate`・`vgcreate`・`lvextend`・`vgchange`・`lvchange` → **Medium**。（0140 AC-11）
- **AC-14**: `ip`・`ifconfig`・`route` → **Medium**（名前のみ・粗粒度）。ただし `ip netns exec <NAME> <cmd>`・
  `ip vrf exec <NAME> <cmd>` は内側 `<cmd>` の**間接実行**として扱い、最終リスク = **内側 `<cmd>` の評価値を下限
  High で floor**（例: `ip netns exec ns rm -rf /` → 内側評価かつ最低 High、`ip netns exec ns modprobe x` → High）。
  内側を抽出できない形は Reject。（0140 AC-12）
- **AC-15**: `mount`/`umount` の**既定**は **Medium**を維持する（対象 trust-critical の引き上げは 0142）。（0140 AC-13）

### F-005: Critical の尖鋭化

- **AC-16**: Critical（無条件ブロック）は**任意の内側コマンドを透過実行する特権昇格ラッパ**に限定する。
  代表例: `sudo`・`su`・`pkexec`・`doas`・`runuser`・`setpriv`・`capsh`。**現状の特権 profile は `sudo`/`su`/`doas`
  のみ**のため、`pkexec`/`runuser`/`setpriv`/`capsh` を**特権 profile へ新規登録**し、直接呼び出し
  （`/usr/bin/pkexec …`）が `EvaluateRisk` の特権ランクで **Critical** になること、ネスト形（`env pkexec …` 等）も
  Critical になることを担保する。（0140 AC-23）
- **AC-17**: F-002 の権限付与/認証境界系（`visudo`/`useradd` 等）・カーネルモジュール（`insmod` 等）は **High**
  であり **Critical ではない**（per-command の明示許可で正当な特権バッチを実行可能に保つ）。（0140 AC-24）

### F-006: データ送信の据え置きとヘルパー実行・検出限界

- **AC-18**: データ送信系 `curl`・`wget`・`scp`・`sftp`・`rsync`・`ssh`・`nc` はデータ送信軸で **Medium** を維持
  （High へ引き上げない）。ローカル trust-critical 書込形（`curl -o /usr/bin/x` 等）の High 化は **0142** の所掌。
  本タスクでは Medium 据え置きのみを担保する。（0140 AC-25 のうちデータ送信 baseline 部分）
- **AC-19**: ローカルでヘルパーを実行させる**オプション名を検出**し間接実行として扱う: `ssh -o ProxyCommand=…`／
  `-o LocalCommand=…`・`rsync -e`/`--rsh=COMMAND`。最終リスク = 内側コマンド文字列を抽出して**内側ゲート（評価値、
  下限 High）**、抽出不能なら **Reject**。これは**オプション名の検出**であり**宛先パスのゾーン解析ではない**（軸2 と
  区別。本タスクの「argv の宛先解析を伴わない」は宛先/オペランドのパスゾーン分類を指し、内側コマンド文字列を選ぶ
  オプション検出は既存の間接実行解析の範疇）。（0140 AC-25 のヘルパー実行部分）
- **AC-20**（検出限界の記録, `static`）: 名前ベース AI 検出（`claude`/`gemini` 等 = High）は一般的なデータ送信
  （Medium）を塞ぐものではなく salient な明示ケースの defense-in-depth であること、未列挙/リネーム/multi-call が
  素通りし得ること（安全運用は allowlist＋ハッシュ固定前提）を記録する。**本タスクでは開発者向けドキュメント
  （`command-risk-evaluation.ja.md` 等）に本限界を追記するに留め**、ユーザー向け文書の最終整合・日英反映・移行
  ノートは 0143 が所有する（本 AC は doc 追記の有無を `static` 検証）。（0140 AC-26）

### F-007: 0139 AC-06 乖離の訂正

- **AC-21**: 0139 AC-06（fdisk/mkfs=Medium 維持）と実装の乖離を、**fdisk/mkfs/parted/fsck=High を正**として
  訂正する（`parted`/`fsck` を Medium→High に引き上げる）。0139 のドキュメントは触らず、訂正の文書反映は
  0143。（0140 AC-27）

### F-008: ラッパ/間接実行の整合と env/timeout の High 化

- **AC-22**: ラッパー/間接実行経由の判定: `env modprobe x` → **High**、`sudo useradd u` → **Critical**。
  名前空間/ルート変更ラッパ `chroot`・`unshare`・`nsenter`、コマンド文字列ラッパ `flock`・`watch` も間接実行
  ファミリに**追加**し（現状 `wrapperSpecs` に未登録＝新規配線）、内側コマンドをゲートして外側で素通りさせない:
  最終リスク = 内側評価値を**下限 High で floor**（`flock f cmd`・`watch cmd`・`unshare -r <cmd>`・`nsenter -t 1 <cmd>`）。
  **COMMAND を省略した形**（`chroot /mnt`・`unshare`・`nsenter -t 1 -m`）は暗黙シェル起動とみなし内側未指定でも
  **High 以上**（`unshare -r`・`nsenter -t 1` 等の特権/名前空間エスケープ形を素通りさせない）。（0140 AC-29）
- **AC-23**: 安全な TOML 代替がある実行ラッパ `env`（→ `env_vars`/`env_import`）・`timeout`（→ `timeout`）は
  直接呼び出しを **High** に分類する（benign 形も含む）。内側は間接実行で引き続きゲート（`env dpkg -i`→High、
  `sudo env …`→Critical）。**Critical にはしない**。代替の無いラッパ（`nice`/`ionice`/`stdbuf`/`setsid`）には
  redundant 由来の追加 floor を課さないが、抽出可能ラッパ内側の flat High floor は維持。`env` 経由の loader 制御
  変数（`LD_PRELOAD` 等）は従来どおり forbidden-env-var で拒否。（0140 AC-29a）

### F-009: ロールアウト切替点の導入（根本原因3／依存逆転の解消）

- **AC-24**: 本タスクが**ロールアウト切替点** `RolloutMode ∈ {off, shadow, enforce}`（既定 `off`、constructor 注入、
  配置は `risktypes` または評価器結線層）を導入する。本タスクの全 enforce 引き上げ（AC-01〜AC-23 の High 化・
  env/timeout・特権/ラッパ拡張）は**この切替点経由でのみ** enforce へ反映し、`off` では旧 baseline が不変である
  こと、`enforce` で新挙動になることを**両モードでテスト**する。切替点の TOML デプロイ面・`shadow` 差分ログは 0143。
  （新規。[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §3.2(1)）

## 5. 非機能要件

- **NF-001**: 本タスクが追加した `ReasonCode` は、**本タスク内で**網羅性/一意性テスト（`reason_codes_test.go`）を
  緑に保つ（各タスクが自分のコードを登録。監査ストリームの family 区別の最終化は 0143。
  [00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §4）。本タスクが引き上げ/
  変更したコマンドが deny されたとき、対応する理由コードが評価結果に付与される。
- **NF-002**: `make test`・`make lint`・`make fmt` がすべて成功する。完了基準は本タスクが触れる統合パッケージ
  （`internal/runner`・必要なら `internal/runner/config`）を**コンパイルする範囲**（`./internal/runner/...` または
  `make test`）とする。（0140 NF-002）
- **NF-003**（横断 NF: AC-28 runtime==dry-run を含む）: 名前固定階級の判定は決定的で副作用がなく、runtime と
  dry-run で同一（名前ベースは FS/identity に依存しないため自明に満たす）。**AC-28 は全タスク横断 NF であり、
  パス解決/identity の決定性サブケースは 0142 が主担当**（本タスクは軸1 分を担保）。（0140 NF-003／AC-28）

## 6. スコープ外の根拠

- **宛先ゾーン判定は 0142**: trust-critical/ordinary/safe-zone の判定、書込先オペランド抽出、ローカル書込の High 化は
  argv 解析を要し、軸2（0142）の所掌。本タスクは名前/ラッパで決まる固定レベルに限定する（D5 の線引き）。
- **ロールアウト/監査/文書は 0143**: 引き上げ・引き下げの同時周知、shadow 観測、監査フィールド、文書整合は
  横断関心事として 0143 に集約する（根本原因3）。
- **`RiskLevel` 段数/新レベル**: 変更しない（0140 §6 を継承）。
