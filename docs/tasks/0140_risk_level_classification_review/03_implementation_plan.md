# コマンド名ベース リスクレベル分類の一貫化 — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-19 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は [02_architecture.md](02_architecture.md)（`approved`）を実装可能なタスクへ分解する。設計の
> WHAT/HOW・図・型定義は 02 を参照し、本書では**再掲せず章番号で参照**する。受け入れ基準の原典は
> [01_requirements.md](01_requirements.md)（`approved`）。

---

## 1. 実装概要

### 1.1 目的

破壊／システム変更系の名前ベース判定を 2 軸モデル（軸1=名前固定階級・軸2=宛先ゾーン）と改訂統一原則へ
一貫化する（[01_requirements.md](01_requirements.md) §1.4）。具体的には (A) 割れの解消、(B) 抜けの封鎖、
(C) Critical の尖鋭化、(D) D7 による rm/dd の宛先ゾーン化（過剰 High の解消）を行い、関連文書・既存 config を
実装に追従させ、引き上げ・引き下げ双方を移行ノートで周知する。

### 1.2 実装原則

- **既存の max 合成骨格を維持**（02 §1.1, AC-31）。`evaluateDimensions` に軸2 dimension を**追加**し、
  既存 dimension は名集合再編にとどめる。
- **判定は決定的・副作用なし・read-only**（NF-003, AC-28）。軸2 のパス解決は `lstat`/`readlink` のみ。
- **降格は fail-closed**（02 §3.4/§3.5, AC-17/AC-18）。確定分類を返せないときは High（書込/削除）へ倒す。
- **shadow 期間は加法的に導入する**（02 §6.4, P5a）。新分類（軸1 新名集合・軸2）は**追加**で導入し、旧 enforce
  経路（現行 `EvaluateRisk` ルール）を shadow リリース中は feature flag 越しに保持する。**旧名集合エントリ・
  旧 `dangerousCommandPatterns` 名前のみエントリの物理削除は enforce を新へ切替えた後の cleanup**（§10）で行い、
  P1〜P4 では in-place 破壊置換をしない。P1〜P4 のテストは**新分類経路**を直接対象に新挙動を表明する。
- **Go ソースの識別子・コメント・文字列リテラルは英語**（[_context.md](../../../.claude/commands/_context.md)
  Source-language rule）。本計画書本文は日本語。テストソース（`*_test.go`・`testutil/`・`test_helpers*.go`）も同じ。

### 1.3 既存コード調査結果

調査で確認した既存シンボルと改修方針。詳細設計は 02 の該当節を参照。

**軸1（`internal/runner/base/security/command_analysis.go`）**
- `SystemModificationRisk(names) RiskLevel`（L526）: `highSystemModificationNames`→High、
  `mediumSystemModificationNames`→Medium、他 Unknown。`anyNameInSet`（L458）で照合。**互換のため残す**。
- `highSystemModificationNames`（L438）現状=PM 系＋`systemctl`/`service`。`mediumSystemModificationNames`
  （L451）現状=`chkconfig`/`update-rc.d`/`mount`/`umount`/`fdisk`/`parted`/`mkfs`/`fsck`/`crontab`/`at`/`batch`。
  → 02 §3.2 の High/Medium ファミリへ**再編**。`crontab`/`at`/`batch`（AC-10a）・`fdisk`/`parted`/`mkfs`/`fsck`
  （AC-27）は Medium→High へ移設。
- `dangerousCommandPatterns`（L216-234）の**名前のみエントリ** `format`/`mkfs`/`fdisk`（High）は軸1 名集合へ
  移設（02 §8 注記、論点2）。`mkfs.<fstype>` 動的判定（`CheckDangerousArgPatterns` L546）は High ファミリの
  名前正規化で吸収。`curl`/`wget`/`nc`/`netcat`（Medium）はデータ送信軸のため**据え置き**。
- `HasSystemCriticalPaths`（L320, `*Validator` メソッド）は**生引数の prefix スキャナで流用しない**
  （02 §3.9, C2 訂正）。新規述語 `isWithinCriticalPaths(resolvedAbsPath, criticalPaths) bool` を設ける。
- `walkSymlinkChain`（L579, `ErrSymlinkDepthExceeded`/`ErrSymlinkResolutionFailed`）と同型の解決を軸2 用に
  新規実装（02 §3.5）。`MaxSymlinkDepth=40`（types.go:84）を流用。
- profile 定義（L31 `sudo`/`su`/`doas`＝Critical、L41-46 `rm`/`dd`＝`DestructionRisk(High)`）。
- `IsDestructiveFileOperation(names, args) bool`（L482, =`destructiveCommandNames` rm/rmdir/unlink/shred/dd
  ＋find -delete/-exec＋rsync --delete）。**①固定 High の発生源**（02 §3.8）。

**軸2 が抑止すべき固定 High（02 §3.8 の 3 系統）**
- ① `IsDestructiveFileOperation`→High（evaluator.go L228）。
- ② `CoreutilsCommandRisk`（coreutils.go:140, `(level, isCoreutils, err)`）: rm/dd/shred/truncate＝High、
  未知 applet＝fail-safe High。`destructiveCoreutilsCommands`（L110）。
- ③ profile `DestructionRisk`→High（`ProfileFactorRisk` command_risk_profile.go:180 → `applyProfileFactors`
  evaluator.go:286）。
- 3 系統すべてを**軸2 が確定分類を返したときのみ**抑止（`ZoneUnresolved`/解決失敗時は High net を残す）。

**evaluator（`internal/runner/base/risk/evaluator.go`）**
- `EvaluateRisk`（L55）のランク1〜3（同一性ゲート→間接実行→特権昇格）後、`evaluateDimensions`（L203）が
  ランク4〜8 を `addDimension`（L277, max fold）で合成。ここに軸2 を結線（02 §3.10）。
- `blockingAssessment`（L339）は I/O 失敗の fail-closed に使用。軸2 の解決 I/O 失敗は**Blocking にせず**
  Kind 依存 floor（02 §4）に倒す。

**間接実行（`internal/runner/base/security/indirect_execution.go`）**
- `AnalyzeIndirectExecution`（L301）/ `wrapperSpecs`（L92, 現状 timeout/nice/ionice/nohup/stdbuf/setsid/time/chrt）
  / `isLoaderControlVar`（L1194, `LD_*`/`DYLD_*`）/ find・xargs 子プロセス解析（L346）/ RoleInner flat High floor。
  → 02 §3.6 で特権昇格（pkexec/runuser/setpriv/capsh）・実行ラッパ（chroot/unshare/nsenter/flock/watch）・
  ヘルパー（ssh ProxyCommand/rsync -e）・`ip netns exec`/`ip vrf exec` を拡張。
- 特権昇格は profile L31（`sudo`/`su`/`doas`）＋`isPrivilegeCommand`（L1199）で判定。pkexec 等を追加。

**監査・理由コード**
- `reason_codes.go`（定数 L13-86）の `ReasonCode` 定数群。新規コードは `TestReasonCodes_AllDistinct`
  （reason_codes_test.go L13）の `all` スライスへ**必ず登録**（NF-001）。
- `RiskAuditEntry`（risktypes/types.go:261）/ `RiskAssessment`（同 L113）/ `VerifiedCommandPlan`（同 L85）。
  per-operand 記録（`[]OperandZone`, 02 §3.3/§3.11）を運ぶ構造化フィールドを**新設**する。確認: 現行 3 型の
  いずれにも per-operand キャリアは存在しない。**新フィールドは `RiskAssessment` に置く**（評価結果＝監査の
  単一ソースで、`RiskAuditEntry.Assessment` 経由で監査へ自然に伝播するため。`VerifiedCommandPlan` には追加しない）。
- `ExecuteCommand`→`auditRiskDecision`→`LogRiskProfile`（normal_manager.go）。既存監査テストは
  `internal/runner/resource/audit_wiring_test.go`。

**出力リスク（DRY 共有, 02 §3.9）**
- `(*Validator).EvaluateOutputSecurityRisk(path, workDir) (RiskLevel, error)`（file_validation.go:355）は
  `user.Current()` で `$HOME` を Low 判定に使う（**軸2 はこの経路に到達してはならない**）。共有するのは
  `common.IsPathWithinDirectory`（common/filesystem.go:215, セグメント境界）と critical 述語のみ。

**軸2 が依存する `RuntimeCommand` フィールド（検証済み）**
- `RuntimeCommand.EffectiveWorkDir`（runnertypes/runtime.go:261）: 相対オペランド解決・`find`/`tar` 既定起点・
  safe-zone 根の基点（02 §3.3/§3.4/§3.5）。dry-run/runtime で同値（AC-28）。
- 出力先（`Output()`/`OutputFile` 相当）は safe-zone 根に**含めない**（02 §3.4）。P2 着手前にフィールド名・
  dry-run での非空を確認する（runtime.go の該当フィールド）。

**設定（`internal/runner/base/security/types.go`）**
- `Config.SystemCriticalPaths`（L226 既定 16 パス）/ `GetSystemCriticalPaths`（L309）。safe-zone 信頼ディレクトリ
  許可リスト（`TrustedRoots`）は**未実装**→追加（02 §3.4）。`RiskLevel`（runnertypes/config.go:28,
  Unknown=0/Low=1/Medium=2/High=3/Critical=4）は変更なし。

**テスト可観測性（テスト計画上の前提）**
- 軸2 のパス解決は `t.TempDir` 配下の実 symlink ツリーで検証可能（root 不要）。
- per-operand `Trusted` 条件(2)（経路要素が run-as から書込不可）は所有者/権限の探査を伴う。**root 無しで
  検証するため、所有者/権限探査は関数注入の seam（既存 `common.FileSystem` と同型の `lstat`/`stat`）で受け、
  テストは admin 所有の信頼ルートをモックで再現する**。この seam は 02 §3.4 の `ResolveSafeZone` 実装詳細に
  属し、`ResolveOperandPath`（02 §3.5 のシグネチャ）とは別関数とする。

調査の結論: **新規 3 ファイル**（`location_zoning.go`/`safezone.go`/`path_resolve.go`）＋既存 9 ファイルの改修。
重複実装は無く、境界内包含・critical 述語・symlink 追従は既存部品を流用する。

---

## 2. 実装ステップ（フェーズ別）

> フェーズ名・順序は 02 §8 と一致させる。各フェーズ末尾に**完了基準**を置く。新規 `.go` ファイル（製品コード）を
> 導入するフェーズの完了基準は、当該ファイルを最終的に用いる**同一ビルドタグ**でコンパイルが通ることを含める。

### Phase P1: 軸1 名集合の再編（AC-01〜AC-13, AC-27）

**対象**: `security/command_analysis.go`、`security/types.go`、`security/command_analysis_test.go`、
`security/command_analysis_dangerous_test.go`、`security/coreutils_test.go`（影響）。

> **加法的導入（§1.2/02 §6.4）**: 新 High/Medium 名集合は**追加**で導入し新分類経路へ結線する。旧
> `mediumSystemModificationNames` の該当エントリ・旧 `dangerousCommandPatterns` 名前のみエントリは shadow
> リリース中は保持し、**物理削除は enforce 切替後の cleanup（§10）**で行う。本フェーズのテストは新分類経路を対象に
> 新挙動を表明する。以下「移設/削除」は新集合への**追加＋切替後削除予約**を意味する。

- [ ] High 破壊/デバイス初期化ファミリ集合を新設（02 §3.2 表①）。確定メンバ: `parted`/`fsck`(`fsck.*`)/
  `wipefs`/`blkdiscard`/`sgdisk`/`gdisk`/`cgdisk`/`sfdisk`/`cfdisk`/`mkswap`/`mkfs`(`mkfs.*`)/`fdisk`/
  `e2fsck`/`mke2fs`/`tune2fs`/`resize2fs`、LVM 破壊系 `lvremove`/`vgremove`/`pvremove`/`lvreduce`/`vgreduce`/
  `pvmove`/`lvresize`/`pvresize`/`pvcreate`（AC-01, AC-02, AC-03）。
- [ ] High カーネル/モジュール集合 `insmod`/`modprobe`/`rmmod`/`kexec`/`sysctl`（AC-04）。
- [ ] High アカウント/認証集合 `useradd`/`usermod`/`userdel`/`groupadd`/`groupmod`/`groupdel`/`gpasswd`/
  `chpasswd`/`adduser`/`deluser`/`delgroup`/`passwd`/`chage`/`newusers`/`vipw`/`vigr`/`visudo`（AC-05）。
- [ ] High ブート集合 `grub-install`/`grub2-install`/`update-grub`/`grub-mkconfig`/`grub2-mkconfig`/
  `efibootmgr`/`kernel-install`/`installkernel`（AC-06）。
- [ ] High サービス有効化集合 `chkconfig`/`update-rc.d`（AC-07）。
- [ ] High 電源/ランレベル集合 `shutdown`/`reboot`/`halt`/`poweroff`/`telinit`（AC-07a）。
- [ ] High ファイアウォール集合 `iptables`/`ip6tables`/`iptables-restore`/`ip6tables-restore`/`nft`/`ufw`/
  `firewall-cmd`（AC-08）。`iptables-save`/`ip6tables-save` は**含めない**（既定 Low、`-f <file>` は P3 の
  KindFlagWriter で zoning）。
- [ ] High 能力付与集合 `setcap`（AC-09）。
- [ ] High 信頼境界 intrinsic 集合 `update-alternatives`/`dpkg-divert`/`alternatives`/`ldconfig`（AC-10）。
- [ ] High スケジューラ集合 `crontab`/`at`/`batch`/`systemd-run`（AC-10a）。`crontab`/`at`/`batch` の Medium→High
  移設は下記「Medium→High 移設」タスクで扱う。
- [ ] Medium 限定スコープ集合に `lvcreate`/`vgcreate`/`lvextend`/`vgchange`/`lvchange`（AC-11）・
  `ip`/`ifconfig`/`route`（AC-12）を追加。`mount`/`umount` は Medium 維持（AC-13）。
- [ ] **Medium→High 移設の全エントリを新 High 集合へ追加**（切替後に `mediumSystemModificationNames` から削除予約）:
  `fdisk`/`parted`/`mkfs`/`fsck`（AC-27）、`crontab`/`at`/`batch`（AC-10a）、`chkconfig`/`update-rc.d`（AC-07）。
  切替後の `mediumSystemModificationNames` 残存は `mount`/`umount` のみ。
- [ ] 名前のみエントリ `format`/`mkfs`/`fdisk` を**軸1 High 破壊ファミリへ追加**し High 化を一元化する。`format`/`mkfs`
  は破壊/デバイス初期化集合へ、`fdisk` は同集合へ（上記）。**`dangerousCommandPatterns`（L216-234）の対応 3 行**
  `{[]string{"mkfs"}, High, "File system creation"}`・`{[]string{"fdisk"}, High, "Disk partitioning"}`・
  `{[]string{"format"}, High, "Disk formatting"}` **の物理削除は切替後 cleanup（§10）に予約**（shadow 期間は重複の
  まま：同一 High なので enforce 差は生じない）。引数パターン行 `{"dd","if="}`/`{"chmod","777"}`/`{"rm","-rf"}`/
  `{"sudo","rm"}`/`{"chown","root"}`/network 系は**そのまま残す**。
- [ ] `SystemModificationRisk` を新集合で照合するよう更新（互換シグネチャ維持）。family を返す兄弟
  `SystemModificationFamily(names) (level, family, ok)` を追加（02 §3.2、family 別理由コード用。P5 で結線）。
- [ ] 影響テスト更新: `command_analysis_test.go::TestSystemModificationRisk`（表 L1125-1135）で **Medium→High に
  反転する全行**の期待値を変更する: `fdisk`/`parted`/`mkfs`/`fsck`（L1127-1130）・`crontab`/`at`/`batch`
  （L1131-1133）・`chkconfig`/`update-rc.d`（L1134-1135）。`mount`/`umount`（L1125-1126）は Medium 維持。
  さらに新 High/Medium ファミリ代表を追加。
- [ ] 影響テスト更新: `command_analysis_dangerous_test.go::TestValidator_IsDangerousRootCommand` は
  `DangerousRootPatterns`（types.go の別系統）に依存するため、本フェーズで意味が変わらないことを確認し、
  必要なら fdisk/mkfs の記述コメントのみ整合（挙動変更が無ければ据え置き）。

**新規テスト**:
- [ ] `command_analysis_test.go::TestSystemModificationRisk_HighFamilies`: 各 High 集合の代表名→High を表明
  （AC-01〜AC-10a、各ファミリ最低 1 名）。
- [ ] `command_analysis_test.go::TestSystemModificationRisk_MediumFamilies`: `lvcreate`/`vgcreate`/`ip`/
  `ifconfig`/`route`/`mount`/`umount`→Medium、`iptables-save`/`ip6tables-save`→Unknown(=既定 Low) を表明
  （AC-08 既定/AC-11/AC-12/AC-13）。

**完了基準**: `go test -tags test ./internal/runner/base/security/... ./internal/runner/base/risk/...` が緑。

### Phase P2: パス解決＋safe-zone 導出（AC-14, AC-17, AC-18）

**対象**: 新規 `security/path_resolve.go`・`security/safezone.go`、`security/types.go`（小）。

- [ ] `path_resolve.go`: `ResolveOperandPath(operand, workDir string) (resolved string, err error)`（02 §3.5
  シグネチャ）。相対は `workDir` 基点。未存在 leaf は存在する最深の親まで symlink 解決し残り末尾を `..`/`.`
  畳み込みで合成。型付きセンチネル（深さ超過/到達不能/未確定）を返す。read-only。
- [ ] `path_resolve.go`: leaf-symlink 追従は**操作別**（write 系=ターゲット解決、delete 系=追従しない。02 §3.5）。
  追従可否は呼び出し側 Kind から指定するパラメータで切替える。
- [ ] `safezone.go`: `SafeZone{Roots, TrustedRoots}` 型と `ResolveSafeZone(cmd *RuntimeCommand, cfg *Config)
  SafeZone`（02 §3.4）。Roots=`EffectiveWorkDir`＋専用 temp（`$HOME`・`OutputFile` 親は**含めない**）。
- [ ] `safezone.go`: per-operand `Trusted` 判定（02 §3.4 条件(1)〜(3)）。参照 identity は config の run-as 値
  （live euid 不使用, AC-28）。所有者/権限探査は注入された `lstat`/`stat` seam 経由（§1.3 テスト可観測性）。
  leaf-symlink は最終ターゲットを解決して zoning（symlink→/etc/passwd→Critical）。条件不成立は `Trusted=false`。
- [ ] `types.go`: `Config` に safe-zone 信頼ディレクトリ許可リスト（既定空）フィールドを追加。`DefaultConfig`/
  glossary 整合は P6。
- [ ] `isWithinCriticalPaths(resolvedAbsPath string, criticalPaths []string) bool` を新設（02 §3.9, C2）。
  `common.IsPathWithinDirectory` ＋ `/` はルート完全一致のみ。`Config.SystemCriticalPaths` を解決後パスに適用。

**新規テスト**:
- [ ] `path_resolve_test.go::TestResolveOperandPath`: 相対→workDir 基点（`cp x`→`<workDir>/x`）、未存在 leaf の
  親解決、親 symlink が critical を指す解決、深さ超過/サイクル→型付き err、write 系 leaf-symlink→ターゲット解決、
  delete 系 leaf-symlink→非追従（AC-14, AC-17(a), AC-18）。`t.TempDir` で symlink ツリー構築。
- [ ] `safezone_test.go::TestResolveSafeZone`: Roots に WorkDir/temp が入り `$HOME`/`OutputFile` 親が入らない
  （AC-17(b)）。
- [ ] `safezone_test.go::TestSafeZone_TrustedPerOperand`: 信頼ルート配下でも run-as 書込可な起点は `Trusted=false`、
  admin 所有・run-as 書込不可なら `Trusted=true`、trust-critical と重複する safe-zone は非 safe（AC-17(c)/(d)）。
  所有権は注入 seam のモックで再現。
- [ ] `path_resolve_test.go::TestIsWithinCriticalPaths`: `/usr/local/bin`→true（`/usr` 配下）、`/srv`/`/opt`→false、
  `/`（ルート）はルート自体のみ true、解決後パスで判定（生 prefix では判定しない）（AC-14, AC-15 整合）。

**完了基準**: `go test -tags test ./internal/runner/base/security/...` が緑。

### Phase P3: 軸2 ゾーン分類＋evaluator 結線＋coreutils 整合（AC-14〜AC-22e, AC-22c, AC-31）

**対象**: 新規 `security/location_zoning.go`、`risk/evaluator.go`、`security/coreutils.go`、
`security/command_risk_profile.go`（②③抑止）、関連テスト。

- [ ] `location_zoning.go`: `PathZone`/`LocationKind` enum と `ZoningInput`/`RunAsIdent`/`OperandZone`/
  `LocationResult` 型、`LocationDefinedRisk(names, args, in ZoningInput) (LocationResult, bool)`（02 §3.3）。
- [ ] ロケーション定義ファミリ名集合とコマンド→`LocationKind` 対応表を定義（02 §3.3 の Kind 表）。
- [ ] Kind 別オペランド抽出（02 §3.3 の表）。各 Kind を個別タスクで実装:
  - [ ] KindWriteCopy（cp）: 宛先のみ zoning、機微 source→Medium floor、`-p`/`-a` 特権メタデータ複製→High（AC-22b）。
  - [ ] KindWriteMove（mv/tee/sponge/truncate/sed -i）: mv は宛先＋source 双方、tee/sponge は全 FILE 引数（AC-22b/AC-22d）。
  - [ ] KindWriteInstall（install）: 宛先のみ＋setuid/setgid/`-o`/`-g`→軸 A High（AC-22a）。
  - [ ] KindCreate（mkdir/touch）: 作成対象 zoning（coreutils Low を P3 抑止で上書き可能に）。
  - [ ] KindDeleteTarget（rm/rmdir/unlink/shred）: 全オペランド zoning、safe-zone 外再帰→High、leaf-symlink 非追従（AC-22/AC-22b）。
  - [ ] KindLink（ln）: 宛先＋source/リンク先双方、`-s` 相対 target はリンク親基点解決（AC-22b）。
  - [ ] KindDevice（dd）: `of=` 書込先/`if=` read source、ブロックデバイス→High、`/dev/null` 等無害シンク除外を
    critical 判定**より先**に評価、機微/trust-critical `if=`→Medium floor（AC-21）。
  - [ ] KindMount（mount/umount）: mountpoint＋source 双方、`umount -a`→無条件 High（AC-19）。
  - [ ] KindPermission（chmod/chown/chgrp/setfacl/chattr）: 権限拡大/setuid/`chattr -i`/trust-critical→High（AC-20）。
  - [ ] KindArchive（tar/unzip）: `-C`/`-d` 展開先（省略時 `EffectiveWorkDir`）、脱出メンバ/特権メタデータ復元→fail-safe High（AC-22e 注記）。
  - [ ] KindFlagWriter（curl -o/-O・wget -O/-P/既定・iptables-save -f・rsync DEST/--delete）: 書込先 zoning、明示先無しは
    `EffectiveWorkDir` 配下の URL 由来名、解析不能は fail-closed High（AC-08/AC-25）。
  - [ ] KindMknod（mknod）: 無条件 High（AC-16 注記）。
  - [ ] `find` の破壊/書込アクション: 探索起点（省略時 `EffectiveWorkDir`）を zoning、`-delete`=ツリー破壊、
    `-fprint*`=FILE zoning、読取専用は非昇格（AC-22e）。
  - [ ] `find` の `-exec`/`-execdir`/`-ok`/`-okdir`（4 primary すべて）→ 間接実行 `IndirectReject`。既存
    `findExecActions`（command_analysis.go:423 付近）と `indirect_execution.go` の find 解析（L346）が**4 primary
    すべて**を Reject することを確認し、不足があれば拡張する（本タスクは P3 が所有。AC-22e）。
- [ ] **fail-closed パース契約**（02 §3.3）: 未知/曖昧フラグで書込先を一意特定できない場合は当該オペランドを
  `ZoneUnresolved`→High（書込/削除）。Medium に倒さない。
- [ ] 軸 A（ゾーン非依存 High）: setuid/setgid/world-write/trust-critical 所有権付与→High（AC-20, AC-22a）。
- [ ] 複数オペランドは各々 zoning し max（AC-31）。
- [ ] `evaluator.go`: `evaluateDimensions` に軸2 を結線（02 §3.10）。`ZoningInput` を `RuntimeCommand`
  （`EffectiveWorkDir`）＋`Config`（`SystemCriticalPaths`・`ResolveSafeZone`）から組立て、`applies` のとき
  `LocationResult.Level` を `addDimension` で max 合成し、`Operands` を監査へ運ぶ（P5）。
- [ ] **②③①抑止の結線**（02 §3.8, AC-22c）。ロケーション定義 applet かつ軸2 `applies=true` のとき:
  - [ ] ①`IsDestructiveFileOperation`→High を `ZoneOrdinary`/`ZoneSafe` で寄与させない。
  - [ ] ②`CoreutilsCommandRisk`→High を `ZoneOrdinary`/`ZoneSafe` で寄与させない（`coreutils.go` 改修。
    非ロケーション/未知 applet の fail-safe 分類は撤去しない）。
  - [ ] ③profile `DestructionRisk`→High（rm/dd）を `ZoneOrdinary`/`ZoneSafe` で寄与させない
    （`applyProfileFactors`/`ProfileFactorRisk` 経路）。
  - [ ] `ZoneUnresolved`/解決失敗のみ①〜③の High net を残す（唯一の fail-open 箇所のみ冗長 High）。
  - [ ] `find -exec`/`rsync --delete` 等「引数による破壊/実行」は間接実行・arg 軸に残し、①〜③の安全網を
    非ロケーション applet には維持する。

**新規テスト**（`location_zoning_test.go`）:
- [ ] `TestLocationDefinedRisk_Zones`: trust-critical→High（`cp evil /usr/bin/ls`・`mv x /etc/passwd`・
  `ln -sf x /usr/bin/python`・`install -m755 x /usr/sbin/y`, AC-14）／ordinary→Medium（`rm /srv/app/cache.dat`・
  `cp a /opt/data/b`, AC-15）／safe-zone→Low（workdir 配下 cp/mv/rm/mkdir, AC-16）。
- [ ] `TestLocationDefinedRisk_FailSafe`: Kind-floor 分岐を**1 行/分岐の表**で網羅（AC-18, 02 §3.5）。最低限:
  書込/削除系（KindWriteCopy/WriteMove/WriteInstall/FlagWriter/DeleteTarget/Device `of=`）の未解決→**High**、
  読取主体（cp source・`dd if=`）の未解決→**Medium**（Low でも High でもない境界を明示）、未知の値取りフラグ
  （`-t <unknownconsumed>` 等で書込先を消費・特定不能）→`ZoneUnresolved`→**High**。
- [ ] `TestLocationDefinedRisk_LeafSymlinkDeref`: write-deref vs delete-非追従の**分類結果**を表明（02 §3.4(3)/§3.5,
  AC-17/AC-22b）。`cp safe $WORKDIR/link`（link→`/etc/passwd`）→**High**（ターゲット解決）、`rm $WORKDIR/link`
  （同 link）→**Low**（safe-zone の link 削除＝非追従）、解決不能ターゲット→`ZoneUnresolved`→**High**。
- [ ] `TestLocationDefinedRisk_Recursion`: safe-zone 外再帰→High（`rm -rf /etc/x`・`cp -a tree /opt/x`）、
  safe-zone 内再帰→Low（`rm -rf $WORKDIR/build`）（AC-22）。
- [ ] `TestLocationDefinedRisk_PermissionGrant`: `install -o root -m4755 …`・`cp -a /usr/bin/sudo $WORKDIR/sudo`・
  `chmod u+s`・`chattr -i /etc/shadow`→High（AC-20, AC-22a, AC-22b）。
- [ ] `TestLocationDefinedRisk_AllOperands`: `mv /etc/passwd $WORKDIR/passwd`・`ln /etc/passwd $WORKDIR/passwd`・
  `cp /etc/shadow $WORKDIR/...`→High/Medium floor（AC-22b）。
- [ ] `TestLocationDefinedRisk_Device`: `dd if=/dev/sda`・`dd of=/dev/sda`→High、`dd of=/dev/null`→非 High、
  `dd if=/etc/shadow of=$WORKDIR/shadow`→Medium floor（AC-21）。
- [ ] `TestLocationDefinedRisk_MountUmount`: trust-critical mountpoint/source→High、`umount -a`→High、
  既定→Medium（AC-19）。
- [ ] `TestLocationDefinedRisk_Tee`/`Find`: tee 複数 FILE max（`tee safe /etc/passwd`→High, AC-22d）、
  `find /etc -delete`→High・`find $WORKDIR -delete`→Low・`find -delete`（起点省略）→workdir 基点・
  `find /etc -name '*.conf'`（読取専用）→非昇格（AC-22e）。
- [ ] `TestLocationDefinedRisk_Mknod`: `mknod`→無条件 High（AC-16 注記）。
- [ ] `TestLocationDefinedRisk_FlagWriter`: `curl -o /usr/bin/x`・`wget -O /etc/cron.d/x`・`iptables-save -f /etc/x`→High、
  safe-zone 宛先→Low（AC-08/AC-25）。
- [ ] **3 系統抑止ゲート（evaluator レベルの専用テスト）** `evaluator_test.go::TestEvaluateRisk_ZoneSuppressionGate`:
  `EvaluateRisk` 経由（①②③と軸2 が `addDimension` で合成される層）で表明する。`coreutils_consistency_test.go`（②単独）
  では①③の漏れを検出できないため**置き換えない**。ケース:
  - (i) safe-zone `rm $WORKDIR/x`→**Low**（①②③抑止）。
  - (ii) ordinary `rm /srv/app/cache.dat`→**Medium**（①②③抑止）。
  - (iii) `ZoneUnresolved` な rm/dd（未知フラグ/解決失敗の宛先）→**High**: ①`IsDestructiveFileOperation`・
    ②`CoreutilsCommandRisk`・③profile `DestructionRisk` の**各 net が個別に残る**ことを 3 ケースで表明
    （fail-open 回帰防止, D7）。
  - (iv) 非ロケーション/未知 applet が `EvaluateRisk` 経由でも High を維持（②の安全網非撤去, AC-22c）。
- [ ] `evaluator_test.go::TestEvaluateRisk_MaxOfDimensionsOrderIndependent` を拡張: 軸1×軸2 同時該当
  （`cp -a … /usr/bin`=High）が順序非依存で max（AC-31）。

**影響テスト更新**（02 §7.2）:
- [ ] `evaluator_test.go::TestStandardEvaluator_EvaluateRisk_DestructiveFileOperations`: rm/dd の無条件 High→
  宛先ゾーン依存（safe-zone=Low/ordinary=Medium）へ期待値更新。
- [ ] `evaluator_test.go::TestEvaluateRisk_AbsoluteRmRfHigh`: `/usr/bin/rm -rf` の対象ゾーンに応じた期待値更新
  （trust-critical/safe-zone 外なら High 維持）。
- [ ] `coreutils_consistency_test.go::TestCoreutilsRiskConsistency_RuntimeVsDryRun` ほか: ロケーション定義 applet の
  固定 High 抑止に伴う rm/cp/dd/shred/truncate の期待値を宛先ゾーン依存へ更新。
- [ ] `coreutils_test.go::TestCoreutilsCommandRisk_DestructiveCommands` / `TestCoreutils_UnknownSubcommandHigh`:
  `CoreutilsCommandRisk` を直接呼ぶ**ユニットレベル**で primitive が不変（②抑止は evaluator 層のみ）であることを
  確認する。**evaluator 層での未知 applet 維持は `TestEvaluateRisk_ZoneSuppressionGate` (iv) が担う**（本テストは
  抑止の正否を検出しないため代替にしない）。
- [ ] profile 系（`evaluator_test.go::TestEvaluateRisk_ProfileFactorFloor` 等・`profile_builder_test.go`）:
  ③抑止に伴う rm/dd の期待値変更。

**完了基準**: `go test -tags test ./internal/runner/base/...` が緑。

### Phase P4: 間接実行・特権昇格の拡張＋env/timeout High（AC-23, AC-25, AC-29, AC-29a）

**対象**: `security/indirect_execution.go`、`security/command_analysis.go`（特権 profile・`redundantWrapperNames`）、
`security/indirect_execution_test.go`。

- [ ] 特権昇格ファミリ拡張（02 §3.6, AC-23）: `pkexec`/`runuser`/`setpriv`/`capsh` を Critical 対象へ追加
  （L31 profile またはレゾルバへ）。`isPrivilegeCommand` が拾うことを確認。
- [ ] 実行ラッパ拡張（AC-29）: `chroot`/`unshare`/`nsenter`（名前空間/ルート変更）、`flock`/`watch`
  （コマンド文字列）を `wrapperSpecs` 系へ追加。RoleInner flat High floor を維持。
- [ ] COMMAND 省略の暗黙シェル（AC-29）: `chroot`/`unshare`/`nsenter` が内側未指定でも High 以上（暗黙シェル起動）。
- [ ] サブコマンド実行（AC-12）: `ip netns exec <NAME> <cmd>`・`ip vrf exec <NAME> <cmd>` を内側ゲート対象に。
- [ ] ヘルパー実行オプション（AC-25）: `ssh -o ProxyCommand=…`/`-o LocalCommand=…`・`rsync -e`/`--rsh` を
  内側ゲート/Reject。
- [ ] `env`/`timeout` の High 化（AC-29a, 02 §3.7）: 専用集合 `redundantWrapperNames` を軸1 固定 High の一つとして
  扱う（独立 dimension は作らない）。理由コード `ReasonRedundantWrapper`（P5）。内側は間接実行で引き続きゲート
  （`env modprobe x`→High、`sudo env …`→Critical）。Critical にはしない。
- [ ] `nice`/`ionice`/`stdbuf`/`setsid` は redundant floor を課さない（TOML 等価物無し）が RoleInner flat High floor は
  維持（AC-29a）。`env` loader 制御変数は従来どおり `ReasonForbiddenEnvVar` で Reject。

**新規/更新テスト**（`indirect_execution_test.go`・`evaluator_test.go`）:
- [ ] `TestIndirect_PrivilegeFamilyExtended`: `pkexec`/`runuser`/`setpriv`/`capsh`（および `sudo`/`su`/`doas`）→Critical（AC-23）。
- [ ] `TestIndirect_ExecWrappersExtended`: `chroot`/`unshare`/`nsenter`（COMMAND 有/無）→High 以上、`flock`/`watch`→
  内側ゲート、`ip netns exec ns rm -rf /`/`ip vrf exec red modprobe x`→内側ゲート（AC-29, AC-12）。
- [ ] `TestIndirect_HelperExecOptions`: `ssh -o ProxyCommand=…`・`rsync -e <cmd>`→Reject/内側ゲート（AC-25）。
- [ ] `TestIndirect_RedundantWrapperHigh`: `env FOO=bar ls`・`timeout 10 ls`→High、`env modprobe x`→High、
  `sudo env …`→Critical、`nice ./tool`→High floor（redundant floor 無し）（AC-29a）。
- [ ] `evaluator_test.go`: `env modprobe x`→High 以上・`sudo useradd u`→Critical（AC-29）。
- [ ] AC-24 回帰: `visudo`/`useradd`/`insmod`→High（Critical でない）を `evaluator_test.go` で表明。

**完了基準**: `go test -tags test ./internal/runner/base/...` が緑。

### Phase P5: 理由コード（family 別＋軸2）・監査フィールド・dry-run 一貫性（AC-30, AC-28, NF-001）

**対象**: `risktypes/reason_codes.go`＋`reason_codes_test.go`、`risktypes/types.go`（`RiskAssessment`/
`RiskAuditEntry`）、`risk/evaluator.go`、`resource/normal_manager.go`。

- [ ] 新規 `ReasonCode` を追加（02 §3.11）。最低限: `ReasonTrustBoundaryWrite`・`ReasonPermissionGrant`・
  `ReasonLocationZone`・`ReasonRedundantWrapper`、および軸1 family 別コード（カーネル/認証/ブート/FW/電源/
  スケジューラ/信頼境界 intrinsic を区別。最終名は実装で確定）。**English 文字列リテラルのみ**。
- [ ] `reason_codes_test.go::TestReasonCodes_AllDistinct` の `all` スライス（L13）へ新規コードを全登録（NF-001）。
- [ ] 軸1 family→理由コード対応を `SystemModificationFamily`（P1）の family 値から付与（一括
  `ReasonSystemModification` 集約をしない。02 §3.11）。
- [ ] per-operand 監査キャリアを新設: `RiskAssessment`（または `VerifiedCommandPlan`）へ `[]OperandZone`
  （`Index`/`Raw`/`Resolved`/`Zone`/`MatchedCritical`/`Trusted`/`UnresolvedErr`）相当を運ぶ構造化フィールドを
  追加し、`RiskAuditEntry` へ記録、`LogRiskProfile` で出力（02 §3.11）。
- [ ] dry-run/runtime 一貫性: 軸2 が config の run-as 値を参照し live euid・`$HOME` env に依存しないことを保証
  （AC-28, 02 §3.9）。

**新規/更新テスト**:
- [ ] `reason_codes_test.go::TestReasonCodes_AllDistinct`: 追加コードで緑（NF-001）。
- [ ] `evaluator_test.go::TestEvaluateRisk_LocationReasonCodes`: 軸2 deny 時に `ReasonLocationZone`/
  `ReasonTrustBoundaryWrite`/`ReasonPermissionGrant` が、軸1 family 時に family 別コードが、`env`/`timeout` 時に
  `ReasonRedundantWrapper` が記録される（AC-30）。
- [ ] `internal/runner/resource/audit_wiring_test.go::TestAudit_PerOperandZoneRecorded`: symlink で `/etc` を指す
  オペランドを持つコマンドを構築し、出力された `RiskAuditEntry`（`Assessment` 経由）に per-operand の
  `Resolved`/`Zone`/`MatchedCritical` が含まれることを表明（AC-30, 02 §3.11 のデバッグ可能性）。
- [ ] `evaluator_test.go::TestEvaluateRisk_DryRunRuntimeInvariant`: 同一コマンドで runtime/dry-run 同値（AC-28）。
  **`$HOME`/`HOME` env 不変**は `t.Setenv` で 2 値検証（`t.Setenv` 使用のため本テストは `t.Parallel()` を**使わない**）。
  **run-as 非依存**は euid を変える代わりに**構造的に**検証する: 異なる `RunAsIdent` を 2 つ与えて軸2 結果が
  `ZoningInput.RunAs` のみの関数であること（live euid 非依存）を表明する（02 §3.4。unprivileged テストで euid は
  変更不可なため euid 変動はテストしない）。
- [ ] **static 守備**（NF-003/§3.9 の非決定性排除）: `location_zoning.go`/`safezone.go`/`path_resolve.go` が
  `os/user` を import せず `os.Geteuid`/`user.Current` を呼ばないことを grep ガード:
  `rg -n "os/user|user\.Current|os\.Geteuid" internal/runner/base/security/{location_zoning,safezone,path_resolve}.go`
  期待: マッチ無し。

**完了基準**: `go test -tags test ./internal/runner/...` が緑。

### Phase P5a: shadow / audit-only ロールアウトモード（AC-32, AC-33 を支える観測基盤）

**対象**: `resource/normal_manager.go`（評価経路）、`risk/evaluator.go`（旧/新 並走 or feature flag）。

> 02 §6.4 の段階ロールアウト。本フェーズは AC-32/AC-33（引き上げ・引き下げの周知）を**運用で観測可能にする
> 支援基盤**であり、AC-32/AC-33 の受け入れ自体は P6 の移行ノート（static 検証）が担う。

- [ ] 「分類のみ・enforce しない」モードを `ExecuteCommand`→`EvaluateRisk` 経路に追加（read-only 評価を流用、02 §6.4）。
- [ ] 旧ルール（現行 enforce）と新ルールを並走計算し、差分（旧→新・newly-deny・newly-allow）を監査ログに出力。
  enforce は feature flag で旧/新を選択（既定は旧）。
- [ ] 緩和方向（D7: High→Low/Medium）を**明示的な監査イベント**として記録（AC-33 の緩和を静かに通さない）。
- [ ] shadow 期間は P1 の名集合「移設」を**追加**で行い、旧分類経路を保持（破壊的 in-place 置換をしない、02 §6.4）。

**新規テスト**:
- [ ] `normal_manager` テスト `TestShadowMode_DiffLoggedEnforceUnchanged`: shadow モードで旧→新差分（newly-deny/
  newly-allow）がログ出力され、**enforce 判定は旧ルールのまま**（緩和コマンドが新で allow でも旧で deny なら
  deny される）ことを表明（AC-32, AC-33 の観測前提）。

**完了基準**: `go test -tags test ./internal/runner/...` が緑。

### Phase P6: 文書整合・移行ノート・既存 config 追従（AC-32〜AC-35。AC-36 は実装完了後）

**対象**: `docs/user/risk_assessment.{ja,}.md`、`docs/dev/architecture_design/command-risk-evaluation.{ja,}.md`、
`docs/translation_glossary.md`、`sample/*.toml`、`internal/runner/config/template_backward_compat_test.go`。

> バイリンガル方針（[_context.md](../../../.claude/commands/_context.md)）: `.ja.md` を先に編集・コミットし、
> 英語版は `/mktrans` で反映する（直接両方編集しない）。

- [ ] 移行ノート（引き上げ, AC-32）: `risk_assessment.ja.md` に Low/Medium→High 引き上げ群（軸1 新 High ファミリ・
  軸2 trust-critical ケース・`env`/`timeout`）を記載。従来許可 config がブロックされ得る旨と allowlist＋ハッシュ＋
  明示 `risk_level` 前提を併記。
- [ ] 移行ノート（引き下げ, AC-33）: `rm`/`rmdir`/`shred`/`unlink`/`dd` の safe-zone/ordinary ケース（D7）を
  **セキュリティ緩和方向**として明示（baseline=直近リリース）。`env`/`timeout`→High を単独項目で強調（02 §6.4）。
- [ ] 文書整合（AC-34）: `risk_assessment.ja.md`（L75 fdisk/mkfs=high・L80 parted/fsck=medium の記述を修正）・
  `command-risk-evaluation.ja.md`・`translation_glossary.md` を軸1 High/Medium 名集合・軸2 3 ゾーン・Critical 尖鋭化に
  一致させる。**fdisk/mkfs/parted/fsck=High** に更新（AC-27）。
- [ ] 英語版反映: `risk_assessment.md`・`command-risk-evaluation.md` を `/mktrans` で `.ja.md` から反映（AC-34）。
- [ ] 検出限界（AC-26）: 名前ベース AI 検出 vs データ送信の非対称・未列挙/リネーム/multi-call の素通り・
  allowlist＋ハッシュ前提を文書化。
- [ ] 既存 config 追従（AC-35）: `sample/*.toml` を**全件**横断検索し、本変更で High へ引き上がるコマンドを含む
  エントリに必要な `risk_level` を付与（0139 AC-14 と同型）。`risk-based-control.toml`/`timeout_examples.toml`/
  `output_capture_security.toml`/`command_template_example.toml` に加え、`template_backward_compat_test.go` の
  ハードコード列挙から漏れている `templates_backup_commands.toml`/`templates_docker_commands.toml`/
  `includes_example.toml`/`template_inheritance_example.toml`（rm/dpkg 等を含み得る）を必ず確認する。
- [ ] `template_backward_compat_test.go` のロード対象列挙に、上記で `risk_level` を追加した sample を加え、ロード回帰が
  全件を覆うようにする（ロード成功は AC-35 の必要条件。十分条件＝risk_level 妥当性は上記 static 走査で担保）。
- [ ] AC-36（ガイド最終化, **実装完了後のみ**）: `risk-level-classification-guide.ja.md` を実装確定挙動へ改訂・
  確定（現状 draft）。**日本語確定後に**英語版 `risk-level-classification-guide.md` を `/mktrans` で作成。

**完了基準**: `make test`・`make lint`（NF-002）、`go test -tags test ./internal/runner/config/...`（sample ロード回帰, AC-35）。

---

## 3. 実装順序とマイルストーン

| MS | フェーズ | 成果物 |
|---|---|---|
| M1 | P1 | 軸1 名集合再編・既存テスト追従（緑）。AC-01〜AC-13, AC-27 |
| M2 | P2 | path_resolve.go / safezone.go（緑）。AC-14, AC-17, AC-18 の基盤 |
| M3 | P3 | location_zoning.go＋evaluator 結線＋3 系統抑止（緑）。軸2 全 AC |
| M4 | P4 | 間接実行・特権昇格拡張＋env/timeout High（緑）。AC-23, AC-25, AC-29, AC-29a |
| M5 | P5/P5a | 理由コード・監査フィールド・dry-run 一貫性・shadow モード（緑）。AC-28, AC-30, NF-001 |
| M6 | P6 | 文書整合・移行ノート・config 追従。AC-26, AC-32〜AC-35（AC-36 は実装後） |

### PR 作成ポイント

#### PR-1 作成ポイント: axis-1 name-set reorganization

**対象ステップ**: Phase P1
**推奨タイトル**: `feat(0140): reorganize axis-1 name sets into High/Medium families`
**レビュー観点**: High/Medium ファミリの網羅と移設（crontab/at/batch・chkconfig/update-rc.d・fdisk/parted/mkfs/fsck の
Medium→High が**新 High 集合へ追加**され、旧エントリの物理削除は切替後 cleanup に予約されること＝加法的導入の遵守）/
`dangerousCommandPatterns` 名前のみエントリの High 一元化に取りこぼし無し / 既存テスト期待値の整合。

#### PR-2 作成ポイント: axis-2 path resolution and zoning

**対象ステップ**: Phase P2, P3
**推奨タイトル**: `feat(0140): add destination-zone classification (axis 2) and 3-source High suppression`
**レビュー観点**: fail-closed パース契約 / 3 系統抑止が確定分類時のみ・`ZoneUnresolved` で High net 維持 /
TOCTOU per-operand Trusted の安全性 / max 合成順序非依存。

#### PR-3 作成ポイント: indirect execution and privilege wrappers

**対象ステップ**: Phase P4
**推奨タイトル**: `feat(0140): extend privilege/exec wrappers and classify env/timeout as High`
**レビュー観点**: 特権昇格ファミリ拡張 / COMMAND 省略の暗黙シェル / env/timeout High が Critical に上がらない /
AC-24（High であり Critical でない）回帰。

#### PR-4 作成ポイント: reason codes, audit fields, shadow mode

**対象ステップ**: Phase P5, P5a
**推奨タイトル**: `feat(0140): add family/zone reason codes, per-operand audit fields, shadow rollout mode`
**レビュー観点**: ReasonCode 網羅性テスト追従 / per-operand 監査キャリアの記録 / dry-run/env 不変性 /
shadow モードで enforce が旧のまま。

#### PR-5 作成ポイント: docs alignment, migration notes, sample configs

**対象ステップ**: Phase P6（AC-36 を除く）
**推奨タイトル**: `docs(0140): align risk docs, add migration notes, follow sample configs`
**レビュー観点**: 移行ノートの引き上げ/引き下げ網羅 / fdisk/mkfs/parted/fsck=High の文書修正 / 日英整合
（`/mktrans`）/ sample config ロード回帰緑。

> AC-36（ガイド最終化・英語版作成）は全実装 PR マージ後の独立 PR とする（要件の作業順序厳守）。

---

## 4. テスト戦略

- **ユニットテスト**: 02 §7.1 に準拠。軸1 名集合（AC-01〜13）、軸2 ゾーン×LocationKind（AC-14〜22e）、
  fail-closed パース、非決定性不変性、3 系統抑止ゲート、間接実行拡張、max 合成、dry-run 一貫性。
- **影響テスト更新**: 02 §7.2 の表（evaluator/coreutils/command_analysis/indirect_execution）を破壊的変更として更新。
  削除する不変条件（rm/dd 無条件 High）は宛先ゾーン依存の代替テストで置き換える（素の削除をしない）。
- **統合・後方互換**: `template_backward_compat_test.go` による sample ロード回帰（AC-35）。
- **文書整合**: 移行ノート・検出限界・docs/glossary の static 検証（§6 AC 検証表）。
- 既存テストヘルパー（`risk/test_helpers.go`・`security/test_helpers.go`、いずれも `//go:build test`）を流用。
- **新規ヘルパー（軸2 で複数テストが共有するため、配置を確定する。[test_organization.md](../../dev/developer_guide/test_organization.md)）**:
  - symlink ツリー構築ヘルパー（`safezone_test.go`/`path_resolve_test.go`/`location_zoning_test.go` で共有）は
    `security/test_helpers_zoning.go`（`//go:build test`, package 内・Classification B。private な zoning 型/解決
    seam に触れるため）。
  - 所有権/権限探査 seam のモック（条件(2) の root-free 検証用、§1.3）は、production の探査 seam が `common.FileSystem`
    互換の public インタフェースなら `security/testutil/helpers.go`（Classification A, `//go:build test`）。private
    seam に依存する場合は同じ `test_helpers_zoning.go` に置く。
  - 上記 seam が `//go:build test` の非 `_test.go` から参照される場合、当該ファイルは `go test -tags test ./...` で
    コンパイルされること（各フェーズ完了基準で担保）。

---

## 5. リスク管理

| リスク | 緩和策 |
|---|---|
| 引き上げ・引き下げ同時変更によるフリート破壊 | P5a の shadow/audit-only モードで事前観測（02 §6.4）＋移行ノート（AC-32/33）。 |
| fail-open（最も解析困難な入力が素通り） | `ZoneUnresolved`→High（書込/削除）と 3 系統抑止を「確定分類時のみ降格」に限定（02 §3.8）。 |
| TOCTOU（safe-zone すり替え） | per-operand Trusted の保守的条件＋fail-closed（02 §3.4）。Low 降格は限定最適化、主目的は ordinary→Medium。 |
| 既存テスト大量更新による回帰見落とし | 02 §7.2 の影響表をチェックリスト化し、削除した不変条件を代替テストで担保。 |
| パス解決のホットパス性能 | 評価単位メモ化＋オペランド総数/解決コスト上限、超過は fail-closed（02 §3.5）。 |

---

## 6. 受け入れ基準検証（Acceptance Criteria Verification）

> ラベル: `test`=実行可能テスト / `static`=rg/grep/コンパイル / `manual`=PR 観察。各テストは未作成のため
> 計画上の `path::TestName` を記す。

| AC | 検証 | 種別 | 場所 / コマンドと期待結果 |
|---|---|---|---|
| AC-01 | parted/fsck/wipefs/blkdiscard/sgdisk/gdisk/cgdisk/sfdisk/cfdisk/mkswap→High | test | `internal/runner/base/security/command_analysis_test.go::TestSystemModificationRisk_HighFamilies` |
| AC-02 | LVM 破壊系→High | test | 同上 `::TestSystemModificationRisk_HighFamilies` |
| AC-03 | mkfs/fdisk＋e2fsck/mke2fs/tune2fs/resize2fs→High | test | family 網羅は `TestSystemModificationRisk_HighFamilies`。`TestEvaluateRisk_DangerousArgPatternsRuntime` は `mkfs.*` 引数パターン由来 High の**回帰ガードのみ**（family 網羅ではない） |
| AC-04 | insmod/modprobe/rmmod/kexec/sysctl→High | test | `command_analysis_test.go::TestSystemModificationRisk_HighFamilies` |
| AC-05 | アカウント/認証 DB 系→High | test | 同上 |
| AC-06 | ブート系→High | test | 同上 |
| AC-07 | chkconfig/update-rc.d→High | test | 同上 |
| AC-07a | shutdown/reboot/halt/poweroff/telinit→High | test | 同上 |
| AC-08 | FW 系→High、iptables-save 既定→Low、`-f <file>`→zoning | test | `command_analysis_test.go::TestSystemModificationRisk_HighFamilies`/`_MediumFamilies`（iptables-save=Unknown）＋`location_zoning_test.go::TestLocationDefinedRisk_FlagWriter` |
| AC-09 | setcap→High | test | `TestSystemModificationRisk_HighFamilies` |
| AC-10 | update-alternatives/dpkg-divert/alternatives/ldconfig→High | test | 同上 |
| AC-10a | crontab/at/batch/systemd-run→High | test | 同上（新分類経路で High を返すことを表明。旧 Medium エントリの物理削除は切替後 cleanup） |
| AC-11 | lvcreate/vgcreate/lvextend/vgchange/lvchange→Medium | test | `command_analysis_test.go::TestSystemModificationRisk_MediumFamilies` |
| AC-12 | ip/ifconfig/route→Medium、`ip netns/vrf exec`→内側ゲート | test | `TestSystemModificationRisk_MediumFamilies`＋`indirect_execution_test.go::TestIndirect_ExecWrappersExtended` |
| AC-13 | mount/umount 既定→Medium | test | `TestSystemModificationRisk_MediumFamilies` |
| AC-14 | trust-critical 宛先→High（解決後パス判定） | test | `location_zoning_test.go::TestLocationDefinedRisk_Zones`＋`path_resolve_test.go::TestIsWithinCriticalPaths` |
| AC-15 | ordinary→Medium | test | `TestLocationDefinedRisk_Zones`（`rm /srv/app/cache.dat`） |
| AC-16 | safe-zone→Low | test | `TestLocationDefinedRisk_Zones`（workdir 配下） |
| AC-17 | safe-zone 定義/解決/信頼 (a)〜(d) | test | `path_resolve_test.go::TestResolveOperandPath`＋`safezone_test.go::TestResolveSafeZone`/`TestSafeZone_TrustedPerOperand` |
| AC-18 | 宛先不確定→Low にしない（Kind 依存 floor） | test | `location_zoning_test.go::TestLocationDefinedRisk_FailSafe` |
| AC-19 | mount/umount 対象 zoning、umount -a→High | test | `TestLocationDefinedRisk_MountUmount` |
| AC-20 | 権限/所有権/属性付与→High | test | `TestLocationDefinedRisk_PermissionGrant` |
| AC-21 | dd ブロックデバイス→High、/dev/null 除外、機微 if=→Medium | test | `TestLocationDefinedRisk_Device` |
| AC-22 | safe-zone 外再帰→High、内再帰→Low | test | `TestLocationDefinedRisk_Recursion` |
| AC-22a | install setuid/setgid/-o/-g→High | test | `TestLocationDefinedRisk_PermissionGrant` |
| AC-22b | 全オペランド zoning（mv/rm/ln source・cp 機微 source） | test | `TestLocationDefinedRisk_AllOperands` |
| AC-22c | safe-zone Low が固定 High で打ち消されない（3 系統抑止）＋fail-open 回帰防止 | test | `risk/evaluator_test.go::TestEvaluateRisk_ZoneSuppressionGate`（evaluator レベル。(i) safe-zone rm=Low・(ii) ordinary `rm /srv/app/cache.dat`=Medium・(iii) `ZoneUnresolved` rm/dd で①②③各 net が High・(iv) 未知 applet が High 維持） |
| AC-22d | tee/sponge FILE zoning、複数 FILE max | test | `TestLocationDefinedRisk_Tee` |
| AC-22e | find -exec/-execdir/-ok/-okdir Reject、-delete/-fprint*/起点 zoning | test | `location_zoning_test.go::TestLocationDefinedRisk_Find`（-delete/-fprint*/起点・読取専用非昇格）＋`indirect_execution_test.go::TestIndirect_FindXargsTargetGated`（4 primary すべて Reject。P3 で 4 primary 網羅を確認/拡張） |
| AC-23 | sudo/su/pkexec/doas/runuser/setpriv/capsh→Critical | test | `indirect_execution_test.go::TestIndirect_PrivilegeFamilyExtended` |
| AC-24 | visudo/useradd/insmod→High（Critical でない） | test | `risk/evaluator_test.go::TestEvaluateRisk_HighNotCritical`（新規） |
| AC-25 | データ送信 Medium、ローカル trust-critical 書込→High、ProxyCommand/-e→間接 | test | `location_zoning_test.go::TestLocationDefinedRisk_FlagWriter`＋`indirect_execution_test.go::TestIndirect_HelperExecOptions` |
| AC-26 | 検出限界の文書化 | static | 検出限界節に**3 アンカーすべて**が存在することを確認（単一キーワードでは不可）: (1) AI=High とデータ送信=Medium の非対称、(2) 未列挙/リネームバイナリ/multi-call の素通り、(3) allowlist＋ハッシュ固定が安全運用前提（0136 AC-66/67）。`rg -n "非対称\|リネーム\|multi-call\|allowlist\|ハッシュ" docs/user/risk_assessment.ja.md` で 3 アンカーの文を目視確認 |
| AC-27 | fdisk/mkfs/parted/fsck=High（実装＋文書） | test, static | test: `TestSystemModificationRisk_HighFamilies`。static: `rg -n "parted\|fsck" docs/user/risk_assessment.ja.md` 期待: high 表記のみ（medium 行に parted/fsck が残らない） |
| AC-28 | runtime==dry-run、`$HOME` env 不変・run-as 非依存（構造的） | test, static | test: `risk/evaluator_test.go::TestEvaluateRisk_DryRunRuntimeInvariant`（runtime/dry-run 同値・`$HOME` 2 値不変・`RunAsIdent` 2 値で同値、no `t.Parallel`）。static: 上記 `os/user`/`os.Geteuid` 不使用 grep ガード |
| AC-29 | env modprobe→High、sudo useradd→Critical、chroot/unshare/nsenter（COMMAND 有無）、flock/watch | test | `indirect_execution_test.go::TestIndirect_ExecWrappersExtended`＋`evaluator_test.go`（env modprobe/sudo useradd） |
| AC-29a | env/timeout→High、loader var 拒否、nice 等 floor 無しだが inner flat High | test | `indirect_execution_test.go::TestIndirect_RedundantWrapperHigh`＋既存 `TestIndirect_WrapperLoaderEnvRejected` |
| AC-30 | deny 時の理由コード記録＋per-operand 監査 | test | `risk/evaluator_test.go::TestEvaluateRisk_LocationReasonCodes`＋`internal/runner/resource/audit_wiring_test.go::TestAudit_PerOperandZoneRecorded` |
| AC-31 | max 合成（軸1×軸2、順序非依存） | test | `risk/evaluator_test.go::TestEvaluateRisk_MaxOfDimensionsOrderIndependent`（拡張） |
| AC-32 | 移行ノート（引き上げ） | static | 移行ノート見出し（例「## 移行ノート」）配下に**引き上げ群が列挙**されることを確認（単なる `high` 一致では不可）: 軸1 新 High ファミリ（カーネル/認証/ブート/FW/電源/スケジューラ/信頼境界 intrinsic）・軸2 trust-critical（cp/mv/ln/install）・env/timeout、かつ「従来許可 config がブロックされ得る」旨と allowlist＋ハッシュ＋明示 `risk_level` 前提。`rg -n "移行ノート" docs/user/risk_assessment.ja.md` で節を特定し本文を目視 |
| AC-33 | 移行ノート（引き下げ） | static | 移行ノート節に rm/rmdir/shred/unlink/dd の safe-zone/ordinary 引き下げが**「緩和方向（旧 deny→新 allow）」**として明示され、baseline=直近リリースが記載されることを目視確認（`rm`/`dd` の偶発出現と区別） |
| AC-34 | risk_assessment/command-risk-evaluation/glossary 整合（日英） | static | (a) 引き上げ確認: `rg -n "fdisk\|parted\|fsck\|mkfs" docs/user/risk_assessment.md docs/user/risk_assessment.ja.md docs/dev/architecture_design/command-risk-evaluation.{ja,}.md` で high 分類を確認。(b) **medium 残存の不在**: `parted`/`fsck` が `medium` 行（現 `risk_assessment.ja.md` L80 の複合行）に残らないことを確認（high 一致だけでは複合行の medium 残存を見落とすため両方確認）。(c) glossary 用語（trust-critical/safe-zone/軸1/軸2）が 02 と一致。(d) `/mktrans` 由来で日英の章構成一致 |
| AC-35 | sample/test config 追従 | test, static | test: `internal/runner/config/template_backward_compat_test.go`（既存 sample ロード回帰）。**ロード成功は必要だが不十分**（評価リスクは検証しない）ため static を主検証とする: `sample/*.toml` を**全件**（glob）走査し、本変更で High へ引き上がるコマンドを含むエントリに十分な `risk_level` が付与されていることを確認。ハードコード列挙から漏れる `command_template_example.toml`/`templates_backup_commands.toml`/`templates_docker_commands.toml`/`includes_example.toml`/`template_inheritance_example.toml` を必ず含める。`rg -n -A4 "^cmd = " sample/*.toml` で引き上げ対象エントリの `risk_level` を確認 |
| AC-36 | ガイド最終化（実装後）・日本語確定→英語版 | manual, static | static: `risk-level-classification-guide.ja.md` の Status が `draft`→確定、`risk-level-classification-guide.md`（英語版）が存在。manual: 実装完了後に着手したこと（PR 順序）を PR で確認 |
| NF-001 | ReasonCode 網羅性 | test | `internal/runner/base/risktypes/reason_codes_test.go::TestReasonCodes_AllDistinct` |
| NF-002 | make test/lint/fmt 成功 | static | `make test && make lint`（`make fmt` 差分無し）期待: exit 0 |
| NF-003 | 決定的・副作用なし・read-only | test, static | test: `risk/evaluator_test.go::TestEvaluateRisk_DryRunRuntimeInvariant`（`$HOME`/`RunAsIdent` 不変）＋`path_resolve_test.go::TestResolveOperandPath`（FS 書込なし）。static: `os/user`/`os.Geteuid` 不使用 grep ガード（§2 P5） |

---

## 7. クロスサーチチェックリスト（`make lint`/`make test` で検出できない項目のみ）

- [ ]（**切替後 cleanup 時**）旧 Medium エントリ撤去後に `mediumSystemModificationNames` 参照の残骸が無いか:
  `rg -n "crontab|\"at\"|batch" internal/runner/base/security docs/user docs/dev/architecture_design`。
- [ ]（**切替後 cleanup 時**）旧 `dangerousCommandPatterns` 名前のみ 3 行撤去後の残存参照:
  `rg -n "Disk formatting|File system creation|Disk partitioning" internal`。
- [ ] 新規 `ReasonCode` 識別子が `reason_codes.go` 定義・`TestReasonCodes_AllDistinct` の `all` スライス・
  family 付与箇所の 3 箇所で一致: `rg -n "ReasonTrustBoundaryWrite|ReasonPermissionGrant|ReasonLocationZone|ReasonRedundantWrapper" internal`。
- [ ] 新規公開シンボル（`LocationDefinedRisk`/`ResolveSafeZone`/`ResolveOperandPath`/`PathZone`/`LocationKind`）が
  security パッケージ内で一意（他パッケージの汎用名と衝突しない）: `rg -n "func ResolveSafeZone|type PathZone|type LocationKind" internal`。
- [ ] docs/glossary の用語一貫性（「trust-critical / safe-zone / ロケーション定義コマンド / 軸1 / 軸2」）が
  02_architecture.md と一致: `rg -n "trust-critical|safe-zone|ロケーション定義" docs/translation_glossary.md docs/user docs/dev/architecture_design`。

---

## 8. 実装チェックリスト（フェーズ別進捗）

> §2 の各タスク完了状況を集約する進捗トラッカ（§7 のクロスサーチとは別物）。各フェーズは完了基準（緑）到達で `[x]`。

- [ ] **P1** 軸1 名集合再編（新 High/Medium 集合追加・Medium→High 移設・`SystemModificationFamily` 追加・影響テスト更新）。
- [ ] **P2** `path_resolve.go`/`safezone.go`/`isWithinCriticalPaths`/Config 信頼ディレクトリ設定。
- [ ] **P3** `location_zoning.go`（全 Kind）・evaluator 結線・3 系統抑止・find 4 primary Reject・影響テスト更新。
- [ ] **P4** 特権昇格/実行ラッパ拡張・`ip netns/vrf exec`・ヘルパー実行オプション・env/timeout High。
- [ ] **P5** 理由コード（family＋軸2）・per-operand 監査フィールド（`RiskAssessment`）・dry-run/run-as 不変性。
- [ ] **P5a** shadow/audit-only モード（旧経路保持・差分ログ・enforce は旧）。
- [ ] **P6** 文書整合・移行ノート（引き上げ/引き下げ）・検出限界・sample config 追従（AC-36 は実装後）。
- [ ] **切替後 cleanup**（enforce 切替後・§10）: 旧 `mediumSystemModificationNames` 残存エントリ・旧
  `dangerousCommandPatterns` 名前のみ 3 行・旧分類経路/feature flag の物理削除。
- [ ] **全体**: `make test && make lint` 緑、`make fmt` 差分なし、§6 の全 AC 検証通過（AC-36 を除き実装フェーズ内）。
- [ ] PR-1〜PR-5 マージ済み（各 PR 作成ポイント参照）。

---

## 9. 完了基準（Success Criteria）

- **機能完全性**: AC-01〜AC-36・NF-001〜NF-003 が §6 の検証で満たされる（AC-36 は実装完了後に着手）。
- **品質**: `make test`・`make lint`（NF-002）が緑、`make fmt` 差分なし。02 §7.2 の影響テストがすべて新挙動へ更新済み。
- **セキュリティ**: fail-closed 契約（`ZoneUnresolved`→High）・3 系統抑止の確定分類限定・TOCTOU per-operand Trusted・
  dry-run/runtime 一貫性が専用テストで担保。
- **文書完全性**: 移行ノート（引き上げ/引き下げ）・検出限界・risk_assessment/command-risk-evaluation/glossary の
  日英整合・sample config 追従が完了。

---

## 10. 次のステップ（Next Steps）

- 本実装計画書のレビューと `approved` 化（本書 Status が `approved` になるまで実装コードに着手しない）。
- 承認後、P1 から順に実装し、各フェーズ完了基準で緑を確認しながら PR-1〜PR-5 を作成。
- 全実装 PR マージ後に AC-36（`risk-level-classification-guide` の日本語確定→英語版 `/mktrans`）を独立 PR で実施。
- ロールアウトは P5a の shadow/audit-only モードで 1 リリース観測後に enforce を新ルールへ切替える。
- **切替後 cleanup**: enforce を新へ切替えた後、旧名集合エントリ・旧 `dangerousCommandPatterns` 名前のみ 3 行・
  旧分類経路/feature flag を物理削除する（§1.2 の加法的導入の最終段。§8 のチェックリスト参照）。
