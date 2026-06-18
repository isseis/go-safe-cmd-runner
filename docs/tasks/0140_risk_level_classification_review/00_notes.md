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

## 論点（要件・設計で詰める）

### 論点 1: 「High と Medium の境界原則」をどう再定義するか

0139 §1.3 の境界「High=未検証コードの特権実行 / Medium=定義済み操作のシステム状態変更」を
出発点に、見直し後の High カテゴリをどう括るか。たたき台（指摘者の提示観点＋追加観点）：

1. **永続的システム設定変更**（ブート／サービス／アカウント／認証／FW／カーネルモジュール）
2. **コマンド／バイナリ置換**（信頼境界＝allowlist+ハッシュの破壊）
3. **大規模・不可逆なファイル／ディスク破壊**
4. **外部へのデータ送信**

→ どの観点を High に含めるか、特に ④（データ送信）を Medium のままにするかは要決定。

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

### 論点 5: Critical を使うか（カーネルコード実行など）

`insmod`/`kexec` は任意カーネルコード実行で破滅性は特権昇格に匹敵するが、Critical 枠は
現状「特権昇格＝無条件ブロック」専用。正当用途（特権バッチ委譲）があるため、
**High（per-command で明示許可可能）に留める**のが現実的か。要決定。

### 論点 6: 既存テスト・doc の整合

- 0139 AC-06（fdisk/mkfs=Medium 維持）の実装乖離を本タスクで訂正するか、0139 内で直すか。
- `risk_assessment.ja.md`/`.md`、用語集、開発者向け `command-risk-evaluation.{ja,md}` への反映。
- 引き上げで既存 sample/テスト config がブロックされ得るため、`risk_level` 付与の追従が要る
  （0139 AC-14 と同型の作業）。

## 未決事項（要件で確定すべき点）

1. High の境界再定義（論点 1 の 4 観点のうちどれを High に含めるか。特に ④ データ送信）。
2. 判定構造の一元化リファクタを伴うか（論点 2）。0139 との順序関係。
3. 新規対象コマンドの列挙範囲（論点 3）。
4. コマンド置換の判定方式（名前のみ／宛先パス併用）（論点 4）。
5. カーネルモジュール等を High か Critical か（論点 5）。
6. 0139 AC-06 乖離の是正先（本タスク／0139）と、ユーザー/開発者文書の更新範囲（論点 6）。

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
