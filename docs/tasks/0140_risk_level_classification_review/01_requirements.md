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

要件作成にあたり以下を決定済み（2026-06-18, isseis）。各根拠は [00_notes.md](00_notes.md) の
対応 D 番号を参照。

| # | 決定 | 根拠 |
|---|---|---|
| D1 | **2 軸モデル**：最終リスク = `max(軸1: コマンド階級[name固定], 軸2: 呼び出し危険度[arg/宛先ゾーン])` | 論点1補足 |
| D2 | **改訂版・統一原則**（下記）を採用。High に **④信頼境界の破壊**を新規追加 | 論点1補足 |
| D3 | **Critical の尖鋭化**：任意内側コマンドを透過実行する特権昇格ラッパ（sudo/su/pkexec/doas）に限定。無条件ブロック＝実行不可を意味する | 発見事項D-1, 論点5 |
| D4 | **ロケーション定義コマンドは 3 ゾーン・パス関数**（critical→High / ordinary→Medium / safe-zone→Low）。Low 化は安全要件を満たす場合のみ | 論点4補足 |
| D5 | **精密化の線引き**：package manager=一律名前のみ粗粒度／基本コマンド（cp/mv/mkdir/rmdir/ln 等）=引数解析。基準＝フラグ安定性 × 使用頻度 | 論点4補足 |
| D6 | **権限付与／認証境界系（visudo/useradd 等）= High**（Critical にしない） | 発見事項D-1 |
| D7 | **`rm`/`rmdir`/`shred`/`dd`/`unlink` = 宛先ゾーン+arg 化**（現状 name のみ High からの変更） | 発見事項D-3 |
| D8 | **カーネルモジュール（insmod/modprobe/rmmod/kexec）= High**（Critical にしない） | 論点5 |
| D9 | **データ送信（egress）= Medium 据え置き**。AI⇔egress 非対称は既知の限界として doc 明記 | 論点1補足 |
| D10 | **0139 AC-06 乖離は本タスクで訂正**（fdisk/mkfs/parted/fsck=High を正とする） | 論点6 |
| D11 | **「検討」群の配置を確定**（F-002/F-003/F-004 の各 AC へ反映） | 論点1補足 |
| D12 | **`update-alternatives`=名前のみ High（intrinsic）／`install`=arg 条件（cp/mv 類）** | 発見事項B/D |

**改訂版・統一原則（境界の再定義）**:

```
最終リスク = max(軸1: コマンド階級[name固定], 軸2: 呼び出し危険度[arg/宛先ゾーン])

critical — 任意内側コマンドを透過実行する特権昇格ラッパ（sudo/su/pkexec/doas）。無条件ブロック。
high     — ①device/FS/ツリー粒度の不可逆破壊（能力 or 危険arg）
           ②永続的 system/boot/service/account/auth 変更
           ③高権限での任意コード実行（kernelモジュール, dlsym/LD_PRELOAD, interpreter, AI駆動）
           ④信頼境界の破壊（信頼バイナリ/設定の置換, allowlist+ハッシュ無力化）
           ⑤権限/能力付与（setcap/setfacl/visudo/chmod-grant/chown）
medium   — 永続的だが named-file 粒度の影響（rm/mv/cp/rmdir, 非クリティカルパス）
           / データ egress（境界越え: curl/scp/ssh/rsync）
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
  - データ送信 Medium 据え置きと AI⇔egress 限界の明記（F-006）。
  - 0139 AC-06 乖離の訂正（F-007）。
  - 一貫性・統合の維持（F-008）。
  - 文書整合・移行周知・検出限界・既存 config 追従（F-009）。
- **Out**（詳細・根拠は §6）:
  - `RiskLevel` の段階定義（Low/Medium/High/Critical の意味づけ・段数）の変更。**新レベルは追加しない**。
  - egress（curl/scp/ssh/rsync）を High へ引き上げること（D9: Medium 据え置き）。
  - 判定構造の一元化リファクタの実施可否・0139 との実装順序（02_architecture.md で確定）。
  - `dd of=`／`mount` mountpoint／`setfacl`／宛先ゾーンの**具体パースロジック**（挙動は F-004 で規定、
    実装方式は 02_architecture.md で確定）。

## 3. 機能要件と受け入れ基準

### F-001: 大規模・不可逆破壊系の High 化（軸1・原則①）

device/FS/ボリューム/パーティション粒度の不可逆破壊をもたらし得るバイナリは、引数によらず
**High**（システム変更次元相当の理由コードを記録）に分類する。

**Acceptance Criteria**:
- **AC-01**: 解決済みバイナリ名が `parted`・`fsck`（`fsck.*` 含む）・`wipefs`・`blkdiscard`・
  `sgdisk`・`mkswap` のいずれかであるコマンドは、引数によらず **High** に分類される。
- **AC-02**: LVM 破壊系 `lvremove`・`vgremove`・`pvremove`・`lvreduce`・`vgreduce`・`pvmove` は、
  引数によらず **High** に分類される。
- **AC-03**: `mkfs`（`mkfs.*` 含む）・`fdisk` は **High** に分類される（既存挙動の確定・維持。F-007 と整合）。

### F-002: 永続的システム変更・特権コード実行・権限付与・信頼境界の High 化（軸1・原則②③④⑤）

以下の名集合は、引数によらず **High** に分類する。列挙は代表例であり**非有界**（脅威モデル上、
backstop は allowlist + ハッシュ固定。AC-26 と整合）。

**Acceptance Criteria**:
- **AC-04**（カーネル／モジュール, ③）: `insmod`・`modprobe`・`rmmod`・`kexec` は **High**。
- **AC-05**（認証／アカウント境界, ②）: `useradd`・`usermod`・`userdel`・`groupadd`・`groupdel`・
  `gpasswd`・`chpasswd`・`adduser`・`passwd`・`visudo` は **High**。
- **AC-06**（ブート設定, ②③）: `grub-install`・`update-grub`・`grub-mkconfig`・`efibootmgr` は **High**。
- **AC-07**（ブート時サービス有効化, ②）: `chkconfig`・`update-rc.d` は **High**（`systemctl`/`service`
  と同質。0139 で High となった両者に整合）。
- **AC-08**（ファイアウォール, ②）: `iptables`・`iptables-restore`・`nft` は **High**。読取専用の
  `iptables-save` は本次元で High/Medium を受けない（Low）。
- **AC-09**（能力付与, ⑤）: `setcap` は **High**。
- **AC-10**（信頼境界の置換 intrinsic, ④）: `update-alternatives`・`dpkg-divert`・`alternatives` は
  **High**（intrinsic に system バイナリ/シンボリックリンクを改変するため、宛先によらず High）。

### F-003: 限定スコープのシステム変更の Medium 化（軸1・medium 原則）

永続的だが破壊を伴わない／限定スコープのシステム変更は **Medium**。

**Acceptance Criteria**:
- **AC-11**: LVM 作成/設定系 `lvcreate`・`vgcreate`・`pvcreate`・`lvextend`・`vgchange`・`lvchange` は
  **Medium**。
- **AC-12**: `ip`・`ifconfig`・`route` は **Medium**（名前のみ・粗粒度。サブコマンド解析は行わない）。
- **AC-13**: 既存の Medium 名マッチ系（`crontab`・`at`・`batch`、および `mount`/`umount` の既定）は
  **Medium** を維持する（`mount` の宛先条件付き引き上げは F-004 AC-19 で規定）。

### F-004: ロケーション定義コマンドの 3 ゾーン判定（軸2・宛先ゾーン）

宛先パス/対象によって脅威が決まる「ロケーション定義コマンド」は、宛先ゾーンの関数として
判定する：**trust-critical → High / ordinary → Medium / safe-zone → Low**。対象は
`cp`・`mv`・`rm`・`rmdir`・`unlink`・`shred`・`ln`・`mkdir`・`install`、および `dd`・`mount`・`setfacl`。

**Acceptance Criteria**:
- **AC-14**（宛先ゾーン基本）: ロケーション定義コマンドの宛先が trust-critical パス
  （`/usr/bin`・`/bin`・`/sbin`・`/etc`・`/boot`・`/lib*` 等、`HasSystemCriticalPaths`
  ([command_analysis.go](../../../internal/runner/base/security/command_analysis.go)) 相当）のとき
  **High** に分類される。例: `cp evil /usr/bin/ls`・`mv x /etc/passwd`・`ln -sf x /usr/bin/python`・
  `install -m755 x /usr/local/bin/y`。
- **AC-15**（ordinary）: 宛先が trust-critical でも safe-zone でもない通常パスの named-file 操作は
  **Medium**。例: `rm /var/log/app.log`・`cp a /srv/data/b`。
- **AC-16**（safe-zone → Low）: 宛先が safe-zone（§ AC-17 で定義）内の named-file 操作は **Low**。
  例: run の作業ディレクトリ配下での `cp`/`mv`/`rm`/`mkdir`。
- **AC-17**（safe-zone の定義と解決, 安全要件）: safe-zone 判定は以下をすべて満たす：
  (a) **正規化済み（シンボリックリンク解決後）の絶対パス**で判定し、文字列の prefix 一致では
  判定しない（`~/link→/etc`、`$HOME/../../etc` 等で破れない）。`safefileio` の強化済み解決経路を用いる。
  (b) safe-zone は**曖昧な `$HOME` ではなく、run が指定する作業/出力ディレクトリおよび run 専用の
  private temp** に限定する。共有の `/tmp` を無条件 safe としない。
- **AC-18**（fail-safe: 宛先不確定なら Low にしない, 安全要件）: 宛先オペランドを確実に解決できない
  呼び出し（複数 source・`-t`/`--target-directory` 等で宛先が一意に取れない、または glob/変数展開で
  評価時に実宛先が未確定）は **Low に分類しない**（最低 Medium。trust-critical 判定の上振れは維持）。
- **AC-19**（`mount` mountpoint）: `mount` の mountpoint が trust-critical パスのとき **High**
  （信頼バイナリ/設定の shadowing）。それ以外の `mount`/`umount` は Medium（AC-13）。
- **AC-20**（`setfacl`）: `setfacl` は権限を拡大する付与、または trust-critical 対象に対する操作の
  とき **High**、それ以外は **Medium**（chmod の `chmod 777` 等と同型）。
- **AC-21**（`dd of=`）: `dd` は `of=` がブロックデバイス（`/dev/*`）のとき **High**、`of=` が
  通常ファイルのときは宛先ゾーン（AC-14〜18）に従う。
- **AC-22**（ツリー破壊の arg 昇格）: `rm`/`cp`/`mv` 等が再帰フラグ（`-r`/`-R`）または複数対象の
  glob 展開によりツリー粒度で作用する場合は、宛先ゾーンによらず **High**（`CheckDangerousArgPatterns`
  相当の arg 軸）。例: `rm -rf <任意>`。

### F-005: Critical の尖鋭化

**Acceptance Criteria**:
- **AC-23**: Critical（無条件ブロック）に分類されるのは、**任意の内側コマンドを透過実行する特権昇格
  ラッパ**（`sudo`・`su`・`pkexec`・`doas`）のみとする。
- **AC-24**: F-002 の権限付与/認証境界系（`visudo`・`useradd` 等）および F-001/F-002 のカーネル
  モジュール（`insmod` 等）は **High** であり Critical ではない（per-command で明示許可可能であること
  を担保し、正当な特権バッチを実行不可にしない）。

### F-006: データ送信の据え置きと AI⇔egress 限界の明記

**Acceptance Criteria**:
- **AC-25**: データ egress 系（`curl`・`wget`・`scp`・`rsync`・`ssh`・`nc`）は **Medium** を維持する
  （High へ引き上げない）。
- **AC-26**（検出限界の明記）: 名前ベース AI 検出（`claude`/`gemini` 等 = High）は generic egress
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
  `sudo useradd u` は **Critical**（特権昇格）に分類される。
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

## 4. 非機能要件

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

- **`RiskLevel` の段階定義変更・新レベル追加**: Low/Medium/High/Critical の意味づけ・段数は維持。
  本タスクは「どのコマンドをどのレベルに分類するか」を一貫化するのみ。特に kernel/auth を Critical に
  しないのは、Critical=無条件ブロック（実行不可）であり正当な特権バッチ用途を壊すため（D6/D8）。
- **egress の High 化**: AI⇔egress の非対称（AC-26）は既知の限界として受容し、egress 一般は Medium
  据え置き（D9）。名前ベースでは generic egress と AI 利用を確実には区別できず、過剰な引き上げは
  日常的なデータ転送を阻害する。
- **判定構造の一元化リファクタ・arg パースの具体ロジック**: 観測可能な分類（本要件の AC）と実装方式を
  分離する。リファクタ実施可否・0139 との順序・`dd of=`/`mount`/`setfacl`/宛先ゾーンのパース実装は
  02_architecture.md で確定する。
