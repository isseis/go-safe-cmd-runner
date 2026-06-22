# 判断軸1: コマンド名分類の一貫化 — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-21 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は [01_requirements.md](01_requirements.md)（`approved`）と [02_architecture.md](02_architecture.md)（`approved`）に基づく
> 判断軸1（コマンド名分類）の実装計画である。設計の詳細・図は重複させず、必要箇所で 02 の節を参照する。

---

## 1. 実装概要

### 1.1 目的

コマンド名（とラッパ構造）だけで決まる固定リスクレベルの分類を、改訂統一原則へ一貫化する。具体的には
(A) 大規模・不可逆破壊系および永続システム変更系の High 化、(B) 限定スコープのシステム変更の Medium 化、
(C) Critical の特権昇格ラッパへの限定と profile 拡張、(D) ラッパ/間接実行経由の整合維持と `env`/`timeout` の
High 化、(E) データ送信系の Medium 据え置き補完と検出限界の文書化を行う（設計の全体像は
[02_architecture.md §1](02_architecture.md) を参照）。

### 1.2 実装原則

- **新評価経路・新レベルを追加しない**。既存の名前ベース判定（`SystemModificationRisk` の名前集合、特権 profile、
  間接実行リゾルバのラッパ集合・オプション処理）の**拡張**として表現する（[02_architecture.md §1.1](02_architecture.md)）。
- **新しい `ReasonCode` を導入しない**。既存コードを流用する（NF-001、[02_architecture.md §4](02_architecture.md)）。
- **Go ソース（識別子・コメント・文字列リテラル）は英語**。テストソースも含む（要件 §3）。
- **各 Phase 完了時に `make fmt`→`make test`→`make lint` を緑にする**（NF-002）。
- **ラッパのオプション解析は共有スキャナへ統一する（横断方針）**。新規ラッパハンドラはオプションスキップを
  ハンド-rolled ループで再導出せず、拡張した `optSpec`／`wrapperSpec`（value/bool/optional-argument オプションを
  宣言）と既存 `skipLeadingOptions` 経由で処理する。各ラッパの文法は §1.4 のオプション文法表で一元管理し、各 P
  フェーズはそこを参照する（個別ラッパの取りこぼし＝fail-open を構造的に防ぐ。レビュー指摘群の根本対処）。
- **クローズド集合（完全一致マップ）の完全性をテストで強制する（横断方針）**。`SystemModificationRisk` 等の
  exact-name 集合に対し、要件のオープン列挙（「等」・`grub2-*` 等のグロブ）を**実装側で展開列挙**したうえで、
  テストは代表サンプルでなく**実装と同じ定数集合を網羅検証**する（または「family に一致する全名が期待レベル」を
  property 的に検証）。これにより列挙漏れがサイレントに Low へ落ちる穴をテストが検出できるようにする（具体化は
  P1 の `TestSystemModificationRisk` 強化、§4.1）。

### 1.3 既存コード調査結果

調査対象の現状と本タスクで必要な変更を以下に示す。変更が不要な領域は記載しない。

#### (a) `internal/runner/base/security/command_analysis.go`

- **`highSystemModificationNames`**（[command_analysis.go:438](../../../internal/runner/base/security/command_analysis.go#L438)）: 現状は
  `apt`/`apt-get`/`yum`/`dnf`/`zypper`/`pacman`/`brew`/`pip`/`npm`/`yarn`/`dpkg`/`rpm`/`systemctl`/`service` のみ。
  → AC-01〜AC-12 の High 群を追加する。
- **`mediumSystemModificationNames`**（[command_analysis.go:451](../../../internal/runner/base/security/command_analysis.go#L451)）: 現状は
  `chkconfig`/`update-rc.d`/`mount`/`umount`/`fdisk`/`parted`/`mkfs`/`fsck`/`crontab`/`at`/`batch`。
  → `parted`/`fsck`/`crontab`/`at`/`batch`/`chkconfig`/`update-rc.d` を High 集合へ**移動**し、`fdisk`/`mkfs` も High へ。
    残る Medium は `mount`/`umount`、加えて AC-13/AC-14 の新規 Medium（LVM 作成系・`ip`/`ifconfig`/`route`）。
- **`SystemModificationRisk`**（[command_analysis.go:526](../../../internal/runner/base/security/command_analysis.go#L526)）: 名前集合→レベルの純関数。シグネチャ変更なし。
- **`CheckDangerousArgPatterns` / `dangerousCommandPatterns`**（[command_analysis.go:542](../../../internal/runner/base/security/command_analysis.go#L542),
  [:216](../../../internal/runner/base/security/command_analysis.go#L216)）: `mkfs.` プレフィクス特例は既存。`dangerousCommandPatterns` には `mkfs`/`fdisk` はあるが
  `parted`/`fsck` は無く、`fsck.*` プレフィクス特例も無い（`fsck.ext4` は現状 Medium 止まり）。
  → `fsck.*` 派生名規則を追加（[02_architecture.md §3.2 留意点](02_architecture.md)）。
- **`commandProfileDefinitions`**（[command_analysis.go:29](../../../internal/runner/base/security/command_analysis.go#L29)）: 特権 profile は `NewProfile("sudo","su","doas")`
  のみ。ネットワーク profile は `NewProfile("ssh","scp")`（[:64](../../../internal/runner/base/security/command_analysis.go#L64)）で `sftp` を含まない（`sftp` は現状 profile 無し=Low）。
  → 特権 profile に `pkexec`/`runuser`/`setpriv`/`capsh` を追加（AC-16）、`NewProfile("ssh","scp")` を
    `NewProfile("ssh","scp","sftp")` へ拡張（AC-18）。

#### (b) `internal/runner/base/security/indirect_execution.go`

- **`wrapperSpecs`**（[indirect_execution.go:92](../../../internal/runner/base/security/indirect_execution.go#L92)）: `timeout`/`nice`/`ionice`/`nohup`/`stdbuf`/`setsid`/`time`/`chrt`。
  `chroot`/`unshare`/`nsenter`/`flock`/`watch` は固定 `wrapperSpec` モデルに収まらないため**専用ハンドラ**で追加
  （[02_architecture.md §3.3(a)](02_architecture.md)）。
- **`analyzeWrapper` の no-command Medium 下限**（[indirect_execution.go:681](../../../internal/runner/base/security/indirect_execution.go#L681)）: `timeout` を含む汎用ラッパ共有。
  → `timeout` のみ High 下限へ（ラッパ種別ごとに floor レベルを保持できるよう拡張。[02_architecture.md §3.3(c)](02_architecture.md)）。
- **`analyzeEnv` の no-command Medium 下限**（[indirect_execution.go:585](../../../internal/runner/base/security/indirect_execution.go#L585), [:597](../../../internal/runner/base/security/indirect_execution.go#L597) の 2 箇所）:
  `env` は専用 `analyzeEnv` を通り `analyzeWrapper` を通らない。→ 両箇所を High 下限へ（AC-23）。
- **`remoteShellOptionPrefixes` / `analyzeRemoteShellOption`**（[indirect_execution.go:125](../../../internal/runner/base/security/indirect_execution.go#L125), [:1015](../../../internal/runner/base/security/indirect_execution.go#L1015)）:
  `rsync {-e, --rsh}`・`tar {--to-command, --checkpoint-action}` を一律 Reject。`ssh` は未登録。
  → `rsync -e`/`--rsh` を「抽出→内側ゲート（下限 High）、抽出不能のみ Reject」へ移管、`ssh -o ProxyCommand`/`LocalCommand`
    の新規 `-o` サブパーサ追加、`tar` は Reject 据え置き（AC-19、[02_architecture.md §3.3(d)](02_architecture.md)）。
- **fail-closed 分割規約の流用元**: `analyzeEnvSplitString`（[indirect_execution.go:646](../../../internal/runner/base/security/indirect_execution.go#L646)）が
  `` strings.ContainsAny(s, "\\'\"$#`") `` でシェルメタ文字を含む値を Reject する規約を持つ。AC-19 と `flock -c`/`watch`
  のコマンド文字列抽出はこれと同一規約に束縛する。
- **`evaluateInnerAs`**（[indirect_execution.go:765](../../../internal/runner/base/security/indirect_execution.go#L765)）: `RoleInner` のフラット High 下限・`isPrivilegeCommand` 判定は既存。
  特権 profile 拡張だけで直接形・ネスト形の両経路（`isPrivilegeCommand` は profile 参照）が整う。シグネチャ変更なし。
- **doc コメントの更新が必要な箇所**:
  - `IndirectCritical` の doc コメント（[indirect_execution.go:29-30](../../../internal/runner/base/security/indirect_execution.go#L29)）が特権トークンを `(sudo/su/doas)` と例示。
  - `IndirectFloor` の doc コメント（[indirect_execution.go:38-42](../../../internal/runner/base/security/indirect_execution.go#L38)）が「env with no command -> Medium」と記述。
  - いずれも本タスクの拡張に合わせて更新する（§2 P2・P5 に明示）。

#### (c) `internal/runner/base/risk/evaluator.go`

- 順位 1〜8 の構造・取り込み方は不変（[evaluator.go:55](../../../internal/runner/base/risk/evaluator.go#L55)）。本タスクの拡張は evaluator が既に呼ぶ
  `security` 関数群（`SystemModificationRisk`/`ResolveProfile`/`AnalyzeIndirectExecution`/`CheckDangerousArgPatterns`）に
  閉じるため、evaluator.go のコード変更は不要（参照のみ）。

#### (d) ドキュメント

- `docs/dev/architecture_design/command-risk-evaluation.ja.md` は AI 検出の限界（AC-20）を追記する対象。
  なお同ファイル [:298](../../../docs/dev/architecture_design/command-risk-evaluation.ja.md) はシステム変更名の現行分類（`parted`/`fsck` 等を Medium）を列挙しているが、**この分類リストの
  訂正・日英反映は 0143 の所掌**であり本タスクでは触れない（[02_architecture.md §3.2, §3.5](02_architecture.md)）。本タスクで
  追記するのは AI 検出限界の節のみ。一時的な記述不整合は 0143 完了までの受容済みリスクとして扱う。

#### (e) 既存テスト（要更新の所在）

| テスト | 所在 | 現状 | 必要変更 |
|---|---|---|---|
| `TestSystemModificationRisk` | [command_analysis_test.go:1102](../../../internal/runner/base/security/command_analysis_test.go#L1102) | `parted`/`fsck`/`crontab`/`at`/`batch`/`chkconfig`/`update-rc.d`=Medium | High へ変更＋新規 High/Medium 群を追加 |
| `TestCommandRiskProfiles_PrivilegeEscalation` | [:1346](../../../internal/runner/base/security/command_analysis_test.go#L1346) | privilegeCommands={sudo,su,doas} | pkexec/runuser/setpriv/capsh を追加 |
| `TestMigration_IsPrivilegeConsistency` | [:1571](../../../internal/runner/base/security/command_analysis_test.go#L1571) | privilegeCommands={sudo,su,doas} | 同上を privilege 側へ追加 |
| `TestMigration_RiskLevelConsistency` | [:1447](../../../internal/runner/base/security/command_analysis_test.go#L1447) | privilege=Critical は sudo/su/doas のみ | pkexec 等=Critical、sftp=Medium を追加 |
| `TestMigration_NetworkTypeConsistency` | [:1501](../../../internal/runner/base/security/command_analysis_test.go#L1501) | noneNetwork に sudo/su/doas; alwaysNetwork に sftp 無し | pkexec 等を noneNetwork、sftp を alwaysNetwork へ追加 |
| `TestCommandRiskProfiles_NetworkCommands` | [:1361](../../../internal/runner/base/security/command_analysis_test.go#L1361) | sftp 無し | sftp=NetworkTypeAlways を追加 |
| `TestStandardEvaluator_EvaluateRisk_PrivilegeEscalation` | [evaluator_test.go:36](../../../internal/runner/base/risk/evaluator_test.go#L36) | {sudo,su,doas} | pkexec/runuser/setpriv/capsh を追加 |
| `TestEvaluateRisk_NoProfileAbsolutePath` | [evaluator_test.go:230](../../../internal/runner/base/risk/evaluator_test.go#L230) | `/usr/bin/crontab`=Medium | crontab を High へ変更（同テストの `mount`=Medium は不変） |
| `TestIndirect_WrapperNoCommandMedium` | [indirect_execution_test.go:281](../../../internal/runner/base/security/indirect_execution_test.go#L281) | env/timeout no-command=Medium | env/timeout を High へ（後述のとおりテスト分割） |
| `TestIndirect_UnextractableWrapperRejected` | [indirect_execution_test.go:693-695](../../../internal/runner/base/security/indirect_execution_test.go#L693) | `env FOO=bar`=Medium の対比ブロック | env no-command=High へ変更（コメント「env with no command is Medium」も更新） |
| `TestIndirect_CommandExecOptionsGated` | [:877](../../../internal/runner/base/security/indirect_execution_test.go#L877) | rsync 各形=IndirectReject。case struct は `want IndirectExecutionKind` のみ（Level 非保持） | 抽出可能=High/抽出不能=Reject へ更新。tar は据え置き。ssh ケース新規追加。**case struct に期待 `Level`/特権 reason を追加**（High と Critical を区別するため） |

新規のテストヘルパー／モックは不要。既存ヘルパー（`cmdNameSet`・`analyzeIndirectCmd`・`hasReason`・
`newVerifiedEvaluator`・`verifiedCmd`・`evalLevel`）を再利用する（test_organization 指針の新規ファイル追加は発生しない）。

> **全件監査の根拠**: 上表は、引き上げ対象名（`parted`/`fsck`/`fdisk`/`mkfs`/`crontab`/`at`/`batch`/`chkconfig`/
> `update-rc.d` および新規 High 名）と `env`/`timeout` no-command の現行アサーションを、全 `*_test.go` を横断検索して
> 列挙した結果である（`rg -n "RiskLevelMedium" --glob '*_test.go'` 等で確認済み）。`command_analysis_dangerous_test.go`
> の `/sbin/fdisk`・`validator_dangerous_patterns_test.go` の `{"mkfs","fdisk"}` は `IsDangerousRootCommand`／
> `ValidateDangerousRootPatterns`（DangerousRootPatterns 設定機構）を検証するもので `SystemModificationRisk`／
> `CheckDangerousArgPatterns` とは独立のため、本タスクの引き上げの影響を受けない（更新不要）。

### 1.4 ラッパ／間接実行の共通設計方針（横断）

本タスクが追加・変更する間接実行ラッパは、各々の getopt 文法を個別ハンドラで再導出すると、value/bool/optional-argument・
連結/バンドル短縮形・positional 前後関係・no-command 形のいずれかを取りこぼし、内側コマンド（特権トークン）を
素通りさせる fail-open になりやすい。これを構造的に防ぐため、以下を全ラッパ共通の方針とする。

#### (a) 共有オペランド・ロケータと拡張オプションクラス

- `optSpec`／`wrapperSpec` に**オプションクラスを 3 種**まで表現できるよう拡張する: (1) **value**（分離形で次トークンを
  値として消費。`-t PID`・`flock -w SECS` 等）、(2) **bool**（値を取らない。`timeout --foreground`・`chroot --skip-chdir`
  等）、(3) **optional-argument**（getopt の `optional_argument`。値は**連結形でしか束縛されない**＝`-m=FILE`/`-mFILE`。
  分離トークンは消費しない。`nsenter`/`unshare` の名前空間オプション `-m`/`-u`/`-i`/`-n`/`-p`/`-U`/`-C`/`-T` 等）。
  現状の `wrapperSpec` は value＋positionals のみ、`optSpec` は value/bool＋unknown ポリシーのみで、(3) を表現できない。
- **optional-argument を value と同じ集合に登録してはならない**。登録すると分離形で次トークン（しばしば内側コマンド）を
  値として食い、`unshare -m sudo id` の `sudo` を取りこぼす。optional-argument は bool 同様に分離トークンを消費せず、
  最初の operand を内側コマンドとしてゲートする。
- 全ラッパハンドラ（新規の `chroot`/`unshare`/`nsenter`/`flock`/`watch`/`ip` exec、既存の `analyzeWrapper`・`analyzeEnv`・
  `analyzeTaskset`）は、オプションスキップを**ハンド-rolled ループでなく共有 `skipLeadingOptions` 経由**で行う
  （専用ハンドラの存在自体はアーキ §3.3(a) のとおり維持。共有するのは「オペランド境界の特定」だけ）。unknown-arity・
  境界不確定は Reject（fail-closed）。
- なお optional-argument クラスはアーキ §3.3(a) の「各ツールの値を取るオプションを網羅的に登録」「境界不確定は Reject」の
  実装レベルの精緻化であり、アーキの方針と整合する（アーキ改訂は不要。任意引数の連結のみ束縛という getopt 事実を
  実装で正しく反映する）。

#### (b) ラッパ別オプション文法表（権威ソース = 各ツールの `--help`/manpage）

各ラッパが取り得るオプションクラスと operand 構造を下表で一元管理する。**完全なオプション列挙は実装時に各ツールの
`--help`/manpage から確定**し（WHAT/HOW 分離）、本表は「どのクラスを宣言する必要があるか」を強制するチェックリストと
する。代表例のみ示す。

| ラッパ | value（分離値） | bool | optional-argument（連結のみ） | positional | COMMAND 形 | no-command 下限 |
|---|---|---|---|---|---|---|
| `chroot` | `--userspec`/`--groups` | `--skip-chdir` | — | NEWROOT×1 | argv | High（暗黙シェル） |
| `unshare` | `-S`/`-G`/`-R`/`--root`/`-w`/`--wd`/`--map-user`/`--map-group`/`--propagation`（`--help` で `<dir>` 必須） | `-r`/`--map-root-user`/`-f`/`--kill-child` 等 | `-m`/`-u`/`-i`/`-n`/`-p`/`-U`/`-C`/`-T`（namespace 系のみ `[=<file>]`） | — | argv | High（暗黙シェル） |
| `nsenter` | `-t`/`-S`/`-G` | `-F`/`--preserve-credentials` 等 | `-m`/`-u`/`-i`/`-n`/`-p`/`-U`/`-C`/`-T`/`-r`/`-w` | — | argv | High（暗黙シェル） |
| `flock` | `-w`/`--timeout`・`-E`/`--conflict-exit-code`・`-c`/`--command`（値＝cmd-string） | `-s`/`-x`/`-u`/`-n`/`-F`/`-o` | — | `<file>`×1 | `<file> <cmd>`（argv）／`-c <cmd-string>`（§3.3(d)）。getopt は permutation するので **`-c` は lock operand の前後どちらでも可**（例 `flock -c '…' <file>`／`flock <file> -c '…'`） | fd 専用形 `flock <N>`（数値 fd・コマンド無し）=IndirectNone。コマンド形で抽出不能時のみ Reject |
| `watch` | `-n`/`--interval`・`-q`/`--equexit` | `-t`/`-b`/`-e`/`-g`/`-c`/`-C`/`-w`/`-p` | `-d`/`--differences` | — | cmd-string（`-x`/`--exec` は argv） | Reject（抽出不能時） |
| `ip`（netns/vrf exec） | `-f`/`-family`・`-B`/`-batch`・`-rc`/`-rcvbuf`・`-n`/`-netns` | `-j`/`-json`・`-4`/`-6`・`-br`・`-o`・`-s`・`-d` 等 | — | object 前の global opts → `netns`/`vrf` → `exec` → `<NAME>` | argv | Reject（exec 形で内側欠落時） |
| `timeout` | `-s`/`--signal`・`-k`/`--kill-after` | `--foreground`・`--preserve-status`・`-v`/`--verbose` | — | DURATION×1 | argv | **High**（redundant-with-config） |
| `env` | `-u`/`--unset` | `-i`/`-0`/`-v` | — | NAME=VALUE 連 | argv（`-S` は split-string、`-C` は Reject） | **High**（redundant-with-config） |
| `rsync`（`-e`/`--rsh`） | `-e`/`--rsh`（値＝cmd-string §3.3(d)。連結 `-essh`/バンドル末尾 `-avze ssh`/中間 `-aevz` は getopt 準拠で残部優先） | — | — | — | cmd-string | （抽出ゲート。抽出不能=Reject） |
| `ssh`（`-o`） | `-o`（値中の `ProxyCommand`/`LocalCommand` を `KEY=VALUE`・`KEY VALUE` 両形式で認識） | — | — | — | cmd-string | （抽出ゲート。抽出不能=Reject） |

> 上表は P3〜P5 の各ラッパタスクの**単一の正**とする。各 P フェーズは「§1.4(b) の該当行のクラスを `optSpec` に宣言」と
> 参照し、オプション列挙を散文で重複記述しない。

> **オプション列挙の確定境界（WHAT/HOW 分離・レビュー対応方針）**: 本表が計画として固定するのは、各ラッパの
> **オプションクラス**（value / bool / optional-argument / cmd-string / positional 構造）と no-command の扱いであって、
> 各クラスに属する個別オプション名の**完全な列挙ではない**。完全な列挙は実装時に各ツールの `--help`/manpage を権威
> ソースとして確定し（§1.2 横断方針）、PR レビューで突き合わせる。網羅性は §4.5 のラッパ一様テスト網が観点単位で
> 構造的に担保する（個別オプションの足し忘れは matrix の該当観点が検出する）。したがって**今後のレビュー対応の切り分け**:
>
> - **pure tail enumeration**（クラスは正しく、既存クラスへ個別オプション名を 1 つ足すだけ）の指摘は、計画改訂の対象と
>   せず「実装時に各ツールの `--help` 準拠で確定し、§4.5 の観点で検証する」として扱う（個別対応しない）。
> - 次の (i)〜(iv) は tail ではなく**構造的欠陥**なので、従来どおり計画を修正する: (i) オプションを誤ったクラスに分類
>   （例 optional-argument を value として登録）、(ii) フォーム/分岐そのものの欠落（例 `flock -c` の位置・`flock` の fd 専用形）、
>   (iii) 分割規約・境界判定の fail-open（例 改行の取りこぼし）、(iv) AC 検証の観点欠落（例 LocalCommand 未検証）。
>
> 判断に迷う場合は「§4.5 の既存観点で当該挙動が捕捉されるか」を基準とする——捕捉されるなら tail（実装で対応）、
> 観点自体が無い／クラスが誤りなら構造的欠陥（計画修正）。
>
> **オプションの arity クラスは各ツールの実 `--help`/manpage で検証する（記憶に頼らない）**: §1.4(b) の value/bool/
> optional-argument の列は単なる分類でなく、その option の getopt arity（required_argument→value、optional_argument→
> optional-argument、no_argument→bool）そのものであり、誤ると fail-open（特権トークンの取りこぼし）になる。同名・同義の
> option でもツール／バージョンで arity が異なる（例: `-w`/`-r` は nsenter で任意引数だが unshare では `-w`/`-R` が必須値・
> `-r` はフラグ。`watch -q/--equexit` は procps-ng では `<cycles>` を取る value）。各セルは実装環境のツールの `--help` で
> 確認すること——この検証は計画側・レビュー指摘側いずれの arity 誤りも検出できる。

> **COMMAND 形が cmd-string の場合の fail-closed 分割集合（明示）**: `rsync -e`・`ssh -o ProxyCommand/LocalCommand`・
> `flock -c`・`watch "<cmd-string>"` の値は `/bin/sh -c` に渡るシェルコマンド文字列であり、抽出は §3.3(d) の fail-closed
> 分割規約に従う。ただし既存 `analyzeEnvSplitString` の拒否集合（`` \ ' " $ # ` ``）は**シェルのコマンド区切りを含まない**
> ため、本タスクの cmd-string 抽出ではそれを**明示的に拡張**し、`;` `|` `&` および**改行（LF/CR）**をシェルメタ文字として
> 拒否集合へ加える（改行は `sh -c` の文末区切りで、`ssh\nsudo id` のような後続コマンドを `strings.Fields` がクリーン分割して
> 取りこぼす fail-open を塞ぐ）。拡張拒否集合 = `` \ ' " $ # ` `` ＋ `;` `|` `&` ＋ LF/CR。クリーン分割可能時のみ
> first-token を内側ゲートし、いずれかを含めば Reject。

#### (c) 直接形 vs ラッパ形のセマンティクス一般則（reason code）

ラッパ内側（`evaluateInnerAs` の `RoleInner`）はフラット High 下限を返し、**reason code は per-dimension でなく
`ReasonIndirectExecutionWrapper` に収斂する**（system-modification の細かい reason は持たない。アーキ §4）。したがって:

- 直接形（例 `modprobe`）= High かつ `ReasonSystemModification`。
- ラッパ形（例 `env modprobe`）= High かつ **`ReasonIndirectExecutionWrapper`**（`ReasonSystemModification` を期待しない）。

AC/NF 検証でラッパ形に直接形の dimension reason を期待しないこと。これは NF-001 で個別修正したものを一般則として固定する
（直接形を薄い透過層と見なしてラッパ形へ同じ reason を期待する誤りを防ぐ）。

---

## 2. 実装ステップ（Phase 別）

Phase 構成と順序・依存は [02_architecture.md §8](02_architecture.md) に従う。各 Phase の完了時に
`make fmt`→`make test`→`make lint` を緑にする（NF-002）。

### P1: システム変更名集合の再配置・拡張（AC-01〜AC-15, AC-21）

**対象ファイル**: `internal/runner/base/security/command_analysis.go`、`command_analysis_test.go`

- [ ] `highSystemModificationNames` に F-001 大規模破壊系を追加: `wipefs`/`blkdiscard`/`sgdisk`/`gdisk`/`cgdisk`/
  `sfdisk`/`cfdisk`/`mkswap`、bare 名 `parted`/`fsck`/`fdisk`/`mkfs`（AC-01, AC-03, AC-21）。
- [ ] `highSystemModificationNames` に F-001 LVM 破壊/デバイス初期化系を追加: `lvremove`/`vgremove`/`pvremove`/
  `lvreduce`/`vgreduce`/`pvmove`/`lvresize`/`pvresize`/`pvcreate`（AC-02）。
- [ ] `highSystemModificationNames` に F-001 直接 FS ユーティリティを追加: `e2fsck`/`mke2fs`/`tune2fs`/`resize2fs`
  ほか（AC-03。確定列挙は [01_requirements.md §4](01_requirements.md) を正とする）。要件・アーキともに末尾を「等」と
  しており closed set ではないため、末尾の残り（`xfs_*`/`btrfs` 系等）の採否は実装者判断とし PR レビューで確定する
  （WHAT/HOW 分離の方針。アーキ §3.2）。
- [ ] `highSystemModificationNames` に F-002 群を追加: kernel/module `insmod`/`modprobe`/`rmmod`/`kexec`/`sysctl`
  （AC-04）、account/auth `useradd`/`usermod`/`userdel`/`groupadd`/`groupmod`/`groupdel`/`gpasswd`/`passwd`/`chpasswd`/
  `chage`/`newusers`/`adduser`/`deluser`/`addgroup`/`delgroup`/`vipw`/`vigr`/`visudo`（AC-05。0140 AC-05 から継承する
  account/auth ミューテータを網羅。Debian 系 `adduser`/`deluser` 等も含める）、bootloader
  `grub-install`/`grub-mkconfig`/`grub2-install`/`grub2-mkconfig`/`grub2-set-default`/`grub2-reboot`/`grub2-editenv`/
  `update-grub`/`update-grub2`/`efibootmgr`/`kernel-install`/`installkernel`（AC-06。`grub2-*` ファミリは
  exact-name map では `*` を表現できないため、概念上のグロブを既知の `grub2-*` 名へ**展開列挙**する。tail は実装者判断で
  PR レビュー確定。代替として `grub-`/`grub2-` の前方一致規則を導入する設計余地もあるが、現要件の有限集合には展開列挙で
  足りる＝YAGNI）、boot service `chkconfig`/`update-rc.d`（AC-07）、power `shutdown`/`reboot`/`halt`/`poweroff`/`telinit`
  （AC-08）、firewall `iptables`/`ip6tables`/`iptables-restore`/`ip6tables-restore`/`nft`/`ufw`/`firewall-cmd`
  （AC-09。`iptables-save`/`ip6tables-save` は含めない）、capability `setcap`（AC-10）、trust-intrinsic
  `update-alternatives`/`dpkg-divert`/`alternatives`/`ldconfig`（AC-11）、scheduler `crontab`/`at`/`batch`/`systemd-run`
  （AC-12）。
- [ ] `mediumSystemModificationNames` を再構成: 上記で High へ移した名（`parted`/`fsck`/`fdisk`/`mkfs`/`crontab`/
  `at`/`batch`/`chkconfig`/`update-rc.d`）を除去し、`mount`/`umount` を残す（AC-15）。LVM 作成系
  `lvcreate`/`vgcreate`/`lvextend`/`vgchange`/`lvchange`（AC-13）と `ip`/`ifconfig`/`route`（AC-14）を新規追加。
- [ ] `CheckDangerousArgPatterns` に `fsck.*` 派生名規則を追加する（`mkfs.` と同じ前方一致特例。`fsck.ext4` 等を
  High 化。[02_architecture.md §3.2 留意点](02_architecture.md)）。`mkfs.` と max 合成される。
- [ ] `highSystemModificationNames`/`mediumSystemModificationNames` の doc コメントを新分類に合わせて更新する
  （現コメントは「package managers + service/init」「mount, crontab, mkfs ...」の例示。英語で記述）。
- [ ] `TestSystemModificationRisk` を更新: `parted`/`fsck`/`crontab`/`at`/`batch`/`chkconfig`/`update-rc.d` の期待値を
  High へ変更し、AC-01〜AC-12 の High 代表（`wipefs`/`lvremove`/`modprobe`/`useradd`/`visudo`/`grub-install`/
  `shutdown`/`iptables`/`setcap`/`update-alternatives`/`ldconfig`/`systemd-run` 等）、AC-13/AC-14 の Medium 代表
  （`lvcreate`/`vgcreate`/`ip`/`ifconfig`/`route`）を表駆動で追加する。`iptables-save`/`ip6tables-save` は
  Unknown（=不一致名。evaluator 層では既定 Low に落ちる。この層では寄与しないことを示す。AC-09）として追加する。
- [ ] **クローズド集合の完全性を強制する**（§1.2 横断方針）: 代表サンプルだけでなく、`highSystemModificationNames`／
  `mediumSystemModificationNames` の**定数集合を直接 range して各名が期待レベルを返す**網羅テストを追加する
  （例 `TestSystemModificationRisk_AllNamesEnumerated`: `for n := range highSystemModificationNames { assert High }`、
  Medium 集合も同様）。これにより実装へ名を足し忘れた／集合へ入れ忘れたケースをテストが検出でき、`grub2-*`・
  account/auth ファミリ等の展開列挙漏れがサイレントに Low へ落ちない（AC-05/AC-06 の family 完全性も同時に担保）。
- [ ] `evaluator_test.go::TestEvaluateRisk_NoProfileAbsolutePath`（[:230](../../../internal/runner/base/risk/evaluator_test.go#L230)）の `/usr/bin/crontab`=Medium
  アサーションを High へ変更する（同テストの `/usr/bin/mount`=Medium は Medium 据え置きのため不変）。
- [ ] `CheckDangerousArgPatterns` の `fsck.*` 規則を検証する**新規の直接ユニットテスト**を `command_analysis_test.go`
  に追加する（例 `TestCheckDangerousArgPatterns_FsFamily`）。`fsck.ext4`=High、`mkfs.ext4`=High（既存 `mkfs.` 規則の
  回帰）、bare `fsck`/`mkfs` が `SystemModificationRisk` 経由で High（max 合成）になることを検証する。
  注: 現状 `CheckDangerousArgPatterns` の直接ユニットテストは存在せず、`mkfs.ext4`=High は evaluator 層の
  `evaluator_test.go::TestEvaluateRisk_DangerousArgPatternsRuntime`（[:237](../../../internal/runner/base/risk/evaluator_test.go#L237)）のみが担保している。本タスクは
  関数直下の単体検証を新設する（`command_analysis_dangerous_test.go` は `IsDangerousRootCommand` 系専用のため
  対象外）。

**完了基準**: 上記テストが緑。`make test` が `./internal/runner/...` を含めて緑。

### P2: 特権 profile 拡張（AC-16, AC-17）

**対象ファイル**: `internal/runner/base/security/command_analysis.go`、`indirect_execution.go`（doc コメント）、
`command_analysis_test.go`、`internal/runner/base/risk/evaluator_test.go`

- [ ] `commandProfileDefinitions` の特権 profile を `NewProfile("sudo","su","doas")` から
  `NewProfile("sudo","su","doas","pkexec","runuser","setpriv","capsh")` へ拡張する（`PrivilegeRisk(Critical, …)` の
  まま。[02_architecture.md §3.1](02_architecture.md)）。
- [ ] `IndirectCritical` の doc コメント（[indirect_execution.go:29-30](../../../internal/runner/base/security/indirect_execution.go#L29)）の特権トークン例示を更新する。
  変更前: `// IndirectCritical means a privilege-escalation token (sudo/su/doas) was found`
  変更後: `// IndirectCritical means a privilege-escalation token (sudo/su/doas/pkexec/runuser/setpriv/capsh) was found`
  （後続行 `// as the effective target; the command is Critical (always denied).` は不変）。
- [ ] `TestCommandRiskProfiles_PrivilegeEscalation` の `privilegeCommands` に `pkexec`/`runuser`/`setpriv`/`capsh` を
  追加する。
- [ ] `TestMigration_IsPrivilegeConsistency` の `privilegeCommands` に同 4 名を追加する。
- [ ] `TestMigration_RiskLevelConsistency` に同 4 名=Critical を追加する。
- [ ] `TestMigration_NetworkTypeConsistency` の `noneNetwork` に同 4 名を追加する（特権ラッパはネットワーク profile を
  持たない）。
- [ ] `TestStandardEvaluator_EvaluateRisk_PrivilegeEscalation`（evaluator_test.go）のコマンド列に同 4 名を追加し、
  直接形 `/usr/bin/pkexec …` 等が Critical になることを検証する。
- [ ] ネスト形が Critical になることを `indirect_execution_test.go::TestIndirect_BypassAttackerScenarios`
  （[:947](../../../internal/runner/base/security/indirect_execution_test.go#L947)）の attacker シナリオ表にケース追加して検証する。`pkexec` だけでなく
  `runuser`/`setpriv`/`capsh` の 4 名すべてについて `env <name> …`=IndirectCritical を確認する（profile 登録漏れの
  片側バイパスを防ぐため全名を網羅）。
- [ ] AC-17 の確認: F-002 系（`visudo`/`useradd`/`insmod` 等）が High であって Critical でないことを検証する。
  `TestSystemModificationRisk`（High。P1）に加え、evaluator レベルで `visudo`/`insmod` の最終 Level が High かつ
  特権 Critical でないことを確認するケースを `evaluator_test.go` に追加する。

**完了基準**: 上記テストが緑。`pkexec`/`runuser`/`setpriv`/`capsh` の直接形・ネスト形が Critical、F-002 系が High。
**依存**: なし（P1 と独立だが、AC-17 検証は P1 の High 分類を前提とする）。

### P3: 間接実行ラッパ拡張（chroot/unshare/nsenter/flock/watch、AC-22）

**対象ファイル**: `internal/runner/base/security/indirect_execution.go`、`indirect_execution_test.go`

> 本フェーズの全ラッパは §1.4(a) の共有スキャナ方針に従い、オプションクラスを §1.4(b) のオプション文法表のとおり
> `optSpec` に宣言する（value/bool/optional-argument を明示）。テストは §4.5 のラッパ一様テスト網を満たすこと。
> 以下の各 bullet はその具体化であり、オプション列挙の正は §1.4(b)。

- [ ] `chroot` 専用ハンドラを追加: **NEWROOT の前に置ける `chroot` 自身のオプションを先に読み飛ばす**
  （`chroot [OPTION] NEWROOT [COMMAND]`）。GNU coreutils `chroot` は値を取る `--userspec=USER:GROUP`・
  `--groups=G_LIST` と boolean の `--skip-chdir` を持つため、分離形 `--userspec 0:0` も含めて値を消費してから
  NEWROOT を読み飛ばし、後続 COMMAND を `evaluateInnerAs`（`RoleInner`）でゲート（下限 High）。さもないと
  `chroot --userspec=0:0 /mnt sudo …` の `--userspec=0:0`/`/mnt` を NEWROOT と誤認して内側 `sudo` の Critical ゲートを
  取りこぼす。COMMAND 省略形（`chroot /mnt`）は暗黙シェル起動として **High 下限**を返す
  （[02_architecture.md §3.3(a)](02_architecture.md)）。option-arity 不確定は Reject（fail-closed）。
  `TestIndirect_NamespaceWrappersGated` に `chroot --userspec=0:0 /mnt sudo id`=Critical の回帰を加える。
- [ ] `unshare`/`nsenter` 専用ハンドラを追加: 各ツールの**必須分離値オプション**を `valueOpts` 登録し（nsenter:
  `-t`/`-S`/`-G`/`-W`/`--wdns`；unshare: `-S`/`-G`/`-R`/`--root`/`-w`/`--wd`/`--map-user`/`--map-group`/`--propagation`。
  集合は 2 ツールで異なる。§1.4(b) の各行を正とする）、operand 境界を確定して内側 COMMAND をゲート。**`-w`/`-r` の
  arity は 2 ツールで異なる（`--help` で確認）**——nsenter の `-w`/`--wd`・`-r`/`--root` は任意引数（次項）だが、
  **unshare の `-w`/`--wd`・`-R`/`--root` は必須分離値（`valueOpts`）、unshare の `-r`/`--map-root-user` はフラグ
  （`boolOpts`）**。no-command 形（`unshare`/`nsenter -t 1 -m`）は暗黙シェル High 下限。境界不確定・`-` 始まり誤抽出は
  Reject（fail-closed）。
- [ ] **任意引数オプションを必須分離値・フラグとツール別に区別する（最重要）**: getopt の `optional_argument` は
  **連結形でしか**束縛されない（`-m=FILE`/`-mFILE`）——分離形の次トークンは値でなく operand（＝COMMAND）。任意引数に
  該当するのは: **nsenter** の namespace 系 `-m`/`-u`/`-i`/`-n`/`-p`/`-U`/`-C`/`-T` と `-w`/`--wd[=dir]`・
  `-r`/`--root[=dir]`、**unshare** の namespace 系 `-m`/`-u`/`-i`/`-n`/`-p`/`-U`/`-C`/`-T`（`[=<file>]`）。これらは
  `valueOpts` に入れず連結値のみ消費する。一方 **unshare の `-w`/`--wd`・`-R`/`--root` は必須分離値（`valueOpts`）、
  `-r`/`--map-root-user` はフラグ（`boolOpts`）** で、混同すると `unshare -w /tmp sudo id` などで fail-open/誤抽出になる
  （unshare `-w` を任意引数扱いすると `/tmp` を operand=COMMAND と誤認し本来の `sudo` を取りこぼす）。
  `TestIndirect_NamespaceWrappersGated` の回帰: `nsenter -m sudo id`=Critical・`nsenter -t 1 -w sudo id`=Critical
  （nsenter `-w` 任意引数→`sudo` を消費しない）、`unshare -m sudo id`=Critical（namespace 任意引数）、
  `unshare -w /tmp sudo id`=Critical（unshare `-w` 必須値が `/tmp` を消費し `sudo` を COMMAND としてゲート）、
  `unshare -r sudo id`=Critical（`-r` フラグ→`sudo` を COMMAND としてゲート）。
- [ ] `flock`/`watch` 専用ハンドラを追加: **両ツール自身のオプション（値付き含む）を先に読み飛ばしてから**
  内側を抽出する（オプションクラスは §1.4(b) の各行を正とする）。`flock` の値オプションは `-w`/`--timeout`・`-E`/
  `--conflict-exit-code`・**`-c`/`--command`（値＝cmd-string）**、boolean は `-s`/`-x`/`-u`/`-n`/`-F`/`-o`。`watch` の値オプションは `-n`/`--interval`・
  **`-q`/`--equexit`**、boolean は `-t`/`-b`/`-e`/`-g`/`-c`/`--color`/`-C`/`-w`/`-p`、optional-attached は
  `-d`/`--differences`。読み飛ばさないと `flock -w 10 /tmp/l sudo id` の `10`/ロックパス、`watch -n 1 sudo id`・
  `watch -q 1 sudo id` の `1` を内側コマンドと誤認し、`watch -c sudo id` の `-c` を値オプション扱いして `sudo` を
  食う等で Critical ゲートを取りこぼす。読み飛ばしは `skipLeadingOptions`＋各ツール用 `optSpec`、unknown-arity は Reject。
- [ ] **`flock` の COMMAND 形を 3 種に分岐する**: lock operand と `-c` 値を読み飛ばした後、(1) `<file> <cmd>`
  はトークン列として `<cmd>` を first-token ゲート、(2) **`-c <cmd-string>`**（`-c`/`--command` は flock の値オプション。
  getopt は permutation するので **lock operand の前後どちらでも可**：`flock -c '…' <file>`／`flock <file> -c '…'`）は
  §1.4(b) の cmd-string fail-closed 分割集合で抽出、(3) **fd 専用形 `flock <N>`**（数値 fd・後続コマンド無し。例
  `exec 9>/lock; flock 9`）は helper を実行しないため **IndirectNone**（Reject にしない）。`-c` を `valueOpts` に
  登録しないと `flock -c 'sudo id' /tmp/l` で `-c` を値なしフラグと誤判定し `'sudo id'` を lock ファイル、`/tmp/l` を
  COMMAND と誤抽出して `sudo` を取りこぼす（fail-open）。`watch "<cmd-string>"` も §1.4(b) の cmd-string 集合で抽出。
  コマンド形で抽出不能時のみ Reject。`TestIndirect_NamespaceWrappersGated` に `flock -w 10 /tmp/l sudo id`=Critical・
  `flock /tmp/l -c 'sudo id'`=Critical・**`flock -c 'sudo id' /tmp/l`=Critical（permuted）**・`flock 9`=IndirectNone・
  `watch -n 1 sudo id`=Critical・`watch -q 1 sudo id`=Critical・`watch -c sudo id`=Critical の回帰を加える。
- [ ] **`watch -x`/`--exec` の argv 形を別経路として扱う**: `watch -x`（`--exec`）は内側を `sh -c` でなく直接
  `execvp` で起動するため、引数はコマンド文字列ではなく通常の argv 列（例 `watch -x rm -rf /`）として渡される。
  この形では `-x`/`--exec` 以降の `watch` 自身のオプションを読み飛ばした最初の operand を内側コマンドとして
  そのままゲートする（シェル分割規約は適用しない）。`-x` 無しの単一コマンド文字列形（`watch "<cmd-string>"`）とは
  分岐する。境界不確定は Reject。`TestIndirect_NamespaceWrappersGated` に `watch -x rm -rf /`=High の回帰を加える。
- [ ] `analyzeIndirect`（[indirect_execution.go:312](../../../internal/runner/base/security/indirect_execution.go#L312)）のディスパッチに上記ハンドラの名前判定を追加する（既存の
  `env`/`taskset` 専用分岐と同じ位置づけ。disposition 強度の順序を崩さない）。
- [ ] 新ラッパの内側ゲートテスト `TestIndirect_NamespaceWrappersGated` を追加（`indirect_execution_test.go`）:
  `chroot /mnt rm -rf /`・`unshare -r modprobe x`・`nsenter -t 1 sh`・`flock f cmd`・`watch cmd` が High 以上、
  特権トークン内側（`unshare -r sudo`）が Critical、抽出不能形が Reject になることを検証する。case struct は
  期待 `Kind` と（Floor 時の）`Level` を保持する。
- [ ] no-command 暗黙シェル High のテスト `TestIndirect_NoCommandImplicitShellHigh` を追加:
  `chroot /mnt`・`unshare`・`nsenter -t 1 -m` が IndirectFloor かつ Level=High になることを検証する（汎用ラッパの
  no-command Medium とは別経路であることを固定）。
- [ ] バイパス回帰を `TestIndirect_NamespaceWrappersGated` に含める: `nsenter -S 0 sh` のようにオプション値（`0`）を
  内側コマンドと誤抽出して `sh` をゲートし損なわないこと（value-option 網羅の確認）。

**完了基準**: 上記テストが緑。新ラッパ経由で危険内側を素通りさせない。
**依存**: P1（システム変更名集合の参照）・P2（特権 profile の参照）。

### P4: `ip netns/vrf exec` の内側ゲート（AC-14）

**対象ファイル**: `internal/runner/base/security/indirect_execution.go`、`indirect_execution_test.go`

> `ip` のグローバルオプション解析は §1.4(a) の共有スキャナ＋§1.4(b) の `ip` 行（value/bool クラス）に従う。
> テストは §4.5 の一様テスト網を満たすこと。

- [ ] `ip netns exec <NAME> <cmd>`・`ip vrf exec <NAME> <cmd>` を間接実行ファミリとして処理するハンドラを追加:
  `<NAME>` を読み飛ばし内側 `<cmd>` を `evaluateInnerAs`（`RoleInner`）でゲート（下限 High）。
- [ ] **オブジェクト語（`netns`/`vrf`）の前に置ける `ip` グローバルオプションを読み飛ばす**（位置固定で
  `args[0]=="netns"` を前提にしない）。`ip` は object の前にグローバルオプションを取り、値を取るもの
  （`-family TYPE`・`-batch FILE`・`-rcvbuf SIZE`・`-netns`/`-n NAME` 等）と boolean のもの（`-json`/`-j`・`-4`/`-6`・
  `-br`・`-o`・`-s`・`-d`・`-N`・`-c` 等）が混在する。これらを既存の `skipLeadingOptions`＋`ip` 用 `optSpec`
  （value/bool を列挙、unknown は fail-closed）で読み飛ばしてから `netns`/`vrf` と `exec` を判定する。さもないと
  `ip -json netns exec ns rm -rf /` のようにグローバルオプションを挿入して内側ゲートをバイパスできてしまう
  （fail-open）。`skipLeadingOptions` が境界不確定（unknown-arity）を返した場合は Reject。
  `TestIndirect_IpExecGated` に `ip -json netns exec ns rm -rf /`=High・`ip -n foo netns exec ns sh`=High の回帰を加える。
- [ ] `exec` 以外のサブコマンド（`ip netns list`/`add`/`delete`、`ip vrf show` 等）は `IndirectNone` として通常の
  `ip`（Medium、P1）に委ねる（ブロックしない。[02_architecture.md §3.3(b)](02_architecture.md)）。
- [ ] `exec` 形だが内側を安全に抽出できない場合（`exec` の後に `<cmd>` 無し、`<NAME>` 抽出不能）のみ Reject。
- [ ] テスト `TestIndirect_IpExecGated` を追加（`indirect_execution_test.go`）: `ip netns exec ns rm -rf /`=High・
  `ip netns exec ns modprobe x`=High、`ip vrf exec v sh`=High、内側欠落（`ip netns exec ns`）=Reject、非 `exec`
  サブコマンド（`ip netns list`）=`IndirectNone`（=`ip` Medium 評価へ委譲）を検証する。

**完了基準**: 上記テストが緑。
**依存**: P3（間接実行ファミリの拡張基盤）。

### P5: `env`/`timeout` の redundant-High とヘルパーオプション抽出（AC-19, AC-23）

**対象ファイル**: `internal/runner/base/security/indirect_execution.go`、`indirect_execution_test.go`

> `timeout`/`env` のオプションクラス（特に `timeout` の bool オプション）と `rsync -e`/`ssh -o` の値抽出は §1.4(b) の
> 各行を正とし、共有スキャナ（§1.4(a)）経由で処理する。テストは §4.5 の一様テスト網を満たすこと。

- [ ] `analyzeEnv` の no-command 下限 2 箇所（[indirect_execution.go:585](../../../internal/runner/base/security/indirect_execution.go#L585), [:597](../../../internal/runner/base/security/indirect_execution.go#L597)）を Medium→High へ変更する
  （`env FOO=bar` 単独・`env` 単独。reason code は既存 `ReasonIndirectExecutionWrapper` を流用）。
- [ ] 汎用 `analyzeWrapper` の no-command 下限（[indirect_execution.go:681](../../../internal/runner/base/security/indirect_execution.go#L681)）を、ラッパ種別ごとに floor レベルを
  保持できるよう拡張し、`timeout` のみ High・それ以外（`nice`/`ionice`/`stdbuf`/`setsid` 等）は Medium 据え置きと
  する。レベル切替はパース箇所（ラッパ抽出時）で行い、トップレベル `timeout 5` でもネスト `nice timeout 5` でも
  一貫して High になるようにする（[02_architecture.md §3.3(c)](02_architecture.md)）。
- [ ] **`timeout` の value-less オプションを登録し、no-command/option 付き形でも Reject させない**: 現状の
  `wrapperSpec` は `valueOpts`/`positionals` のみで boolean オプションを持たないため、`analyzeWrapper` が組む
  `optSpec`（`unknown: shortOptsAreBoolean`）では未知の**長**オプションが境界不確定となり Reject される。`timeout` の
  boolean 長オプション（`--foreground`・`--preserve-status`・`-v`/`--verbose`）を認識できるよう `wrapperSpec` に
  `boolOpts` フィールドを追加して `optSpec.boolOpts` へ渡し、`timeout` に登録する。これにより
  `timeout --foreground 5`（no-command）は Reject ではなく **High 下限**になる（AC-23 の「無害に見える形も High」を
  満たす）。`TestIndirect_EnvTimeoutNoCommandHigh` に `timeout --foreground 5`=High・`timeout -v 5`=High の回帰を加える。
- [ ] `IndirectFloor` の doc コメント（[indirect_execution.go:38-42](../../../internal/runner/base/security/indirect_execution.go#L38)）の no-command 例示を更新する。
  変更前: `// level (env with no command -> Medium; inline shell/interpreter, package`
  変更後: `// level (env/timeout with no command -> High; nice/stdbuf/... with no command`
  `// -> Medium; inline shell/interpreter, package`
  （以降の「script runner -> High; …」の記述は不変。英語で記述し、文の連続性を保つよう調整する）。
- [ ] `rsync -e`/`--rsh` を `analyzeRemoteShellOption` の一律 Reject から「値を §3.3(d) 分割規約で抽出→内側ゲート
  （下限 High、特権トークンは Critical）、抽出不能なら Reject」へ移管する（`remoteShellOptionPrefixes` の `rsync`
  エントリを当該 Reject 経路から外し、抽出ゲート経路へ配線）。`tar` は Reject 据え置き。
- [ ] **ショートオプションのバンドル/連結形からの値抽出規約を getopt 準拠で明示する**（検出は既存の
  `matchesRemoteShellOption`／`shortFlagInBundle` を再利用し、値の取り出しのみ新設）。`rsync -e` の値は getopt の
  「値を取るショートオプションは同一トークンの残部を値とし、残部が空なら次トークンを値とする」規約で一意に定まる:
  (1) 連結形 `-essh` → `-e` 直後の残部 `ssh`、(2) バンドル末尾 `-avze ssh` → 残部が空ゆえ次トークン `ssh`、
  (3) バンドル中間 `-aevz` → `-e` 以降の残部 `vz`（次トークンではない）、(4) `--rsh=VALUE`／`--rsh VALUE` →
  等号後／次トークン。次トークンを値とすべきだが後続トークンが無い形のみ抽出不能として **Reject**（fail-closed）。
  取り出した値は §1.4(b) の**拡張 fail-closed 分割集合**（§3.3(d) 準拠＋`;`/`|`/`&`/改行 LF/CR）に通し、いずれかを
  含めば Reject、クリーン分割時のみ first-token を内側ゲート。バンドル中間形を「次トークン」と誤読すると
  `ssh`（=source path）を内側コマンドと取り違える過小抽出になるため、残部優先の規約を厳守する。`TestIndirect_CommandExecOptionsGated` に `-essh`／`-avze ssh`／`-aevz`
  各形の値抽出（High もしくは抽出値に応じた評価）を固定する回帰を加える。
- [ ] `ssh -o ProxyCommand=`/`LocalCommand=` の新規 `-o` サブパーサを rank-2 経路に追加する。`-o` を値オプションと
  認識（分離 `-o ProxyCommand=…`・連結 `-oProxyCommand=…`・分離値 `-o` `ProxyCommand=…`・空白区切り
  `-o "ProxyCommand …"`）し、値中の `ProxyCommand`/`LocalCommand` を `KEY=VALUE` と `KEY VALUE` の両形式で認識、
  コマンド文字列を §1.4(b) の拡張 fail-closed 分割集合（§3.3(d) 準拠＋`;`/`|`/`&`/改行）で抽出して内側ゲート
  （下限 High）、抽出不能なら Reject（[02_architecture.md §3.3(d)](02_architecture.md)）。
- [ ] `TestIndirect_WrapperNoCommandMedium`（[indirect_execution_test.go:281](../../../internal/runner/base/security/indirect_execution_test.go#L281)）を分割する:
  - [ ] env/timeout ケース（`env FOO=bar`・`env` bare・`timeout 5`）を新テスト `TestIndirect_EnvTimeoutNoCommandHigh`
    へ移し、期待値 High とする。加えてネスト `nice timeout 5`=High を追加する。
  - [ ] `nice`/`ionice`/`stdbuf`/`setsid` 等の no-command=Medium 据え置きを `TestIndirect_WrapperNoCommandMedium` に
    残す（名称と期待値が一致するようケースを入れ替える）。
- [ ] `TestIndirect_UnextractableWrapperRejected`（[indirect_execution_test.go:693-695](../../../internal/runner/base/security/indirect_execution_test.go#L693)）の
  `env FOO=bar`=Medium 対比ブロックを High へ更新する。あわせて同箇所のコメント
  `// Contrast: env with no command is Medium, not Reject.` を `env` no-command=High の意味に合わせて英語で更新する
  （例: `// Contrast: env with no command is a High floor (redundant-with-config), not Reject.`）。
- [ ] `env dpkg -i`=High・`sudo env …`/`env sudo …`=Critical・`env LD_PRELOAD=…`=Reject の既存挙動が維持される
  ことを回帰確認する（既存 `TestIndirect_WrapperLoaderEnvRejected`・`TestIndirect_BypassAttackerScenarios` で担保。
  不足あれば追加）。
- [ ] `TestIndirect_CommandExecOptionsGated`（[indirect_execution_test.go:877](../../../internal/runner/base/security/indirect_execution_test.go#L877)）を更新する:
  - [ ] case struct を拡張する。現状は `want IndirectExecutionKind` のみで Level/reason を検証できない。抽出可能形の
    下限 High と特権トークンの Critical を区別するため、期待 `Level runnertypes.RiskLevel`（および必要なら期待 reason）
    フィールドを追加し、各ケースで assert する。
  - [ ] rsync の抽出可能形（`rsync -e ssh`・`--rsh=ssh`・`-essh`・`-avze ssh`）の期待値を IndirectReject から
    IndirectFloor（Level=High）へ変更する。
  - [ ] tar の `--to-command`/`--checkpoint-action` ケースは IndirectReject 据え置きの回帰確認として残す。
  - [ ] ssh ケースを新規追加: **`ProxyCommand` と `LocalCommand` の両方**を対称に検証する——
    `ssh -o ProxyCommand=…`=High、空白区切り `ssh -o "ProxyCommand …"`=High、`ssh -o LocalCommand='sudo id'`=Critical
    （特権トークン）、`ssh -o LocalCommand='nc %h %p; modprobe x'`=Reject（fail-closed）。LocalCommand を省くと
    ProxyCommand のみ実装しても AC-19 が通ってしまうため、両オプションを必ず固定する。
  - [ ] fail-closed 分割規約の検証ケースを追加: §1.4(b) の拡張拒否集合を含む値が Reject になること——シェルメタ文字
    （`rsync -e 'ssh; rm -rf /'`・`ssh -o ProxyCommand='nc %h %p; modprobe x'`）、置換形（`rsync -e "$(printf sudo)"`）、
    **改行を含む値**（`rsync -e $'ssh\nsudo id'`・`ssh -o ProxyCommand=$'ssh\nsudo id'`、および `flock`/`watch` の
    cmd-string パス）が Reject になり、後続コマンドを High 下限へ落とさないこと。特権トークンを含む抽出可能形
    （`rsync -e 'sudo cmd'`）は Critical。

**完了基準**: 上記テストが緑。`env`/`timeout` が無害形含め High、ヘルパーオプションが抽出ゲート/Reject に分かれる。
**依存**: P3（間接実行ファミリの拡張基盤）。

### P6: データ送信 Medium 補完（sftp）と検出限界の文書追記（AC-18, AC-20）

**対象ファイル**: `internal/runner/base/security/command_analysis.go`、`command_analysis_test.go`、
`docs/dev/architecture_design/command-risk-evaluation.ja.md`

- [ ] `commandProfileDefinitions` の `NewProfile("ssh","scp")`（[command_analysis.go:64](../../../internal/runner/base/security/command_analysis.go#L64)）を `NewProfile("ssh","scp","sftp")`
  へ拡張する（AlwaysNetwork・Medium。DRY のため新規 profile を起こさない。[02_architecture.md §3.4](02_architecture.md)）。
- [ ] `TestCommandRiskProfiles_NetworkCommands` に `sftp`=NetworkTypeAlways を追加する。
- [ ] `TestMigration_RiskLevelConsistency` に `sftp`=Medium を追加する。
- [ ] `TestMigration_NetworkTypeConsistency` の `alwaysNetwork` に `sftp` を追加する。
- [ ] `command-risk-evaluation.ja.md` に AI 検出の限界（AC-20）を、固有の見出し
  `### 名前ベース AI 検出の限界` を持つ節として追記する: 名前ベース AI 検出（`claude`/`gemini` 等=High）は一般
  データ送信（Medium）を塞ぐものではなく salient な明示ケースの defense-in-depth であること、未列挙/リネーム/
  multi-call が素通りし得ること、安全運用は allowlist＋ハッシュ固定が前提であることを明記する
  （[02_architecture.md §3.5](02_architecture.md)）。既存の allowlist＋ハッシュ固定の記述（[:437](../../../docs/dev/architecture_design/command-risk-evaluation.ja.md), [:438](../../../docs/dev/architecture_design/command-risk-evaluation.ja.md)）と整合させる。
  見出し文字列はこの節に固有で、AC-20 の `static` 検証アンカーになる。

**完了基準**: 上記テストが緑。`sftp` が Medium/AlwaysNetwork。AC-20 の追記が doc に存在する。
**依存**: なし。

---

## 3. 実装順序とマイルストーン

[02_architecture.md §8](02_architecture.md) の Phase 順に実装する。

| マイルストーン | 含む Phase | 成果物 | 依存 |
|---|---|---|---|
| M1: 名前ベース固定分類の確立 | P1, P2 | システム変更名 High/Medium 再配置、特権 profile 拡張、`fsck.*` 規則 | なし |
| M2: ラッパ/間接実行の整合 | P3, P4, P5 | 新ラッパ・ip-exec・env/timeout High・ヘルパーオプション抽出 | M1 |
| M3: データ送信据え置きと文書化 | P6 | sftp Medium 補完、AC-20 doc 追記 | なし（M1/M2 と並行可） |

各マイルストーン末で `make fmt`→`make test`→`make lint` を緑にする（NF-002）。M2 内では P3 が前提で、P4 と P5 は
いずれも P3 のみに依存する独立な兄弟（相互依存なし）。Phase 単位の正確な依存は §2 各 Phase の `依存` 行を正とする。

---

## 4. テスト戦略

詳細なケースは [02_architecture.md §7](02_architecture.md) を正とし、ここでは観点と所在を示す。

### 4.1 単体テスト

- **システム変更名分類（P1）**: `TestSystemModificationRisk` を表駆動で拡張。High/Medium 各層の代表名と
  `iptables-save`=Unknown、`fsck.*` 派生名の High（`CheckDangerousArgPatterns`）を検証。
- **特権ラッパ（P2）**: profile 系テスト 4 種＋evaluator テストへ `pkexec`/`runuser`/`setpriv`/`capsh` を追加。
  直接形 Critical・ネスト形 Critical・F-002 系 High（非 Critical）を検証。
- **間接実行（P3〜P5）**: 新ラッパの内側ゲート/no-command High、`ip … exec` の内側ゲートと非 exec 委譲、
  env/timeout の no-command High とネスト一貫性、ヘルパーオプションの抽出ゲート/Reject、fail-closed 分割規約
  （シェルメタ文字 Reject・特権トークン Critical）を検証。
- **データ送信（P6）**: `sftp` の Medium/AlwaysNetwork を profile テストへ追加。

### 4.2 統合テスト

- `EvaluateRisk` レベルで、引き上げ対象コマンドが deny されたとき対応する理由コードが評価結果に付与されること
  （NF-001）。`env modprobe`／直接 `modprobe` が双方 High になる整合を `evaluator_test.go` に追加する。

### 4.3 セキュリティテスト

- バイパス回帰: 新ラッパ・ip-exec・ヘルパーオプション経由で危険内側（`rm -rf`/`modprobe`/`sudo`）を素通り
  させないこと、特権ラッパ列挙漏れ（`pkexec` のネスト）が Critical を逃さないことを既存 attacker シナリオ表に
  ケース追加して検証する。
- 理由コード網羅性: `reason_codes_test.go::TestReasonCodes_AllDistinct` は本タスクで新規コードを追加しないため
  不変（回帰確認のみ）。

### 4.4 後方互換性

本プロジェクトは後方互換性を要求しない（要件 §3）。新分類は直接適用するため、互換テストは設けない。破壊的変更
（raise/lower）の周知・sample config 追従は 0143 の所掌。

### 4.5 ラッパ一様テスト網（横断）

§1.4 の共通方針を担保するため、**§1.4(b) の全ラッパに対し同一の観点セットをテーブル駆動で適用**する（ラッパ固有の
個別ケースに加える）。tool ごとに「あるオプションを忘れた」を散発的に拾うのでなく、観点の網羅を構造的に強制する。
各ラッパにつき以下を最低限固定する（該当しない観点は N/A を明記）:

| 観点 | 確認内容（内側に特権トークンを置き、取りこぼさないことを軸に） |
|---|---|
| value-opt 前置 | `<wrapper> <value-opt> <val> sudo …` が内側 `sudo`=Critical（値を内側と誤らない） |
| bool-opt 前置 | `<wrapper> <bool-opt> sudo …` が Critical（bool を値オプションと誤らない。例 `timeout --foreground 5`=High も含む no-command 検証） |
| optional-argument | `<wrapper> <opt-arg-opt> sudo …` が Critical（連結のみ束縛＝分離 `sudo` を食わない。`nsenter`/`unshare` 該当） |
| 連結/バンドル短縮 | 連結 `-xval`・バンドル末尾/中間の値抽出が getopt 準拠（`rsync -essh`/`-avze ssh`/`-aevz`） |
| cmd-string 区切り | cmd-string 形（`rsync -e`/`ssh -o`/`flock -c`/`watch`）で §1.4(b) 拡張集合（`;`/`\|`/`&`/改行含む）を含む値=Reject |
| positional 前後 | positional（NEWROOT/DURATION/`<file>`/`<NAME>`/global-opts）を読み飛ばした先の operand を内側とする |
| no-command 形 | 期待下限（暗黙シェル系=High、`env`/`timeout`=High、`nice` 等=Medium、`flock`/`watch`/`ip exec`=Reject）を返す |
| 抽出不能/境界不確定 | fail-closed で Reject |

この網は `TestIndirect_NamespaceWrappersGated`／`TestIndirect_IpExecGated`／`TestIndirect_CommandExecOptionsGated`／
`TestIndirect_EnvTimeoutNoCommandHigh` に分散して実装する（§2 P3〜P5 の各テストタスクが上記観点を満たすこと）。

---

## 5. リスク管理

| リスク | 影響 | 緩和策 |
|---|---|---|
| `nsenter`/`unshare` の value-option 取りこぼし | オプション値を内側コマンドと誤抽出し fail-open | 各ツールの値オプションを網羅登録し、境界不確定は Reject。バイパス回帰テスト（`nsenter -S 0 sh`）で固定 |
| ヘルパーオプションのコマンド文字列を素朴分割 | trailer・置換・特権トークンの取りこぼし | 既存 `analyzeEnvSplitString` の fail-closed 分割規約に束縛。シェルメタ文字 Reject・特権 Critical をテストで固定 |
| `timeout` の no-command 下限がネストで Medium に戻る | `nice timeout 5` が High に上がらない | floor レベルをパース箇所（ラッパ抽出時）でラッパ名により切替。トップレベルとネストの双方をテストで固定 |
| doc の一時的不整合（システム変更名分類リスト） | 開発者向け doc が新分類と齟齬 | 0143 が文書整合を所有することを §1.3(d) に明示。本タスクは AC-20 追記のみ |
| 既存テストの期待値変更漏れ | High 化したコマンドが Medium のまま緑になる | §1.3(e) の更新所在表に沿って全該当テストを更新。`make test` で検出 |

スケジュールリスク: 本タスクは既存集合・処理の拡張に閉じ、新パッケージ・新型を伴わないため、Phase 単位で独立に
完了・検証できる。

---

## 6. 実装チェックリスト

- [ ] **P1**: システム変更名 High/Medium 再配置＋`fsck.*` 規則＋`TestSystemModificationRisk` 更新
- [ ] **P2**: 特権 profile 拡張＋`IndirectCritical` コメント更新＋privilege/evaluator テスト更新＋AC-17 確認
- [ ] **P3**: chroot/unshare/nsenter/flock/watch 専用ハンドラ＋内側ゲート/no-command High テスト
- [ ] **P4**: `ip netns/vrf exec` 内側ゲート＋非 exec 委譲テスト
- [ ] **P5**: env/timeout redundant-High＋`IndirectFloor` コメント更新＋rsync/ssh ヘルパーオプション抽出＋テスト分割/更新
- [ ] **P6**: sftp Medium 補完＋profile テスト更新＋AC-20 doc 追記
- [ ] 全 Phase 末で `make fmt`→`make test`→`make lint` 緑（NF-002）

---

## 7. 受け入れ基準検証

各 AC を、それを検証するタスク/テストへ対応づける。`test`=実行可能テスト、`static`=rg/grep/コンパイル、
`manual`=PR 観察。各 AC は最低 1 つの `test` または `static` を持つ。

| AC | 区分 | 検証 |
|---|---|---|
| AC-01 | test | `command_analysis_test.go::TestSystemModificationRisk`（`parted`/`fsck`/`wipefs`/`blkdiscard`/`sgdisk`/`gdisk`/`cgdisk`/`sfdisk`/`cfdisk`/`mkswap`=High） |
| AC-02 | test | `command_analysis_test.go::TestSystemModificationRisk`（`lvremove`/`vgremove`/`pvremove`/`lvreduce`/`vgreduce`/`pvmove`/`lvresize`/`pvresize`/`pvcreate`=High） |
| AC-03 | test | `command_analysis_test.go::TestSystemModificationRisk`（`mkfs`/`fdisk`/`e2fsck`/`mke2fs`/`tune2fs`/`resize2fs`=High）＋ 新規 `command_analysis_test.go::TestCheckDangerousArgPatterns_FsFamily`（`mkfs.ext4`/`fsck.ext4`=High、bare `mkfs`/`fsck` の max 合成 High） |
| AC-04 | test | `command_analysis_test.go::TestSystemModificationRisk`（`insmod`/`modprobe`/`rmmod`/`kexec`/`sysctl`=High） |
| AC-05 | test | `command_analysis_test.go::TestSystemModificationRisk`（`useradd`/`usermod`/`userdel`/`groupadd`/`groupmod`/`groupdel`/`gpasswd`/`passwd`/`chpasswd`/`chage`/`newusers`/`adduser`/`deluser`/`addgroup`/`delgroup`/`vipw`/`vigr`/`visudo`=High。0140 AC-05 継承の account/auth ミューテータを網羅） |
| AC-06 | test | `command_analysis_test.go::TestSystemModificationRisk`（`grub-install`/`grub-mkconfig`/`grub2-install`/`grub2-mkconfig`/`grub2-set-default`/`update-grub`/`efibootmgr`/`kernel-install`/`installkernel`=High。`grub2-*` は単一代表でなく ≥2 変種（`grub2-install` と `grub2-mkconfig`）を検証して展開列挙の漏れを検出） |
| AC-07 | test | `command_analysis_test.go::TestSystemModificationRisk`（`chkconfig`/`update-rc.d`=High） |
| AC-08 | test | `command_analysis_test.go::TestSystemModificationRisk`（`shutdown`/`reboot`/`halt`/`poweroff`/`telinit`=High） |
| AC-09 | test | `command_analysis_test.go::TestSystemModificationRisk`（`iptables`/`ip6tables`/`iptables-restore`/`ip6tables-restore`/`nft`/`ufw`/`firewall-cmd`=High、`iptables-save`/`ip6tables-save`=Unknown） |
| AC-10 | test | `command_analysis_test.go::TestSystemModificationRisk`（`setcap`=High） |
| AC-11 | test | `command_analysis_test.go::TestSystemModificationRisk`（`update-alternatives`/`dpkg-divert`/`alternatives`/`ldconfig`=High） |
| AC-12 | test | `command_analysis_test.go::TestSystemModificationRisk`（`crontab`/`at`/`batch`/`systemd-run`=High） |
| AC-13 | test | `command_analysis_test.go::TestSystemModificationRisk`（`lvcreate`/`vgcreate`/`lvextend`/`vgchange`/`lvchange`=Medium） |
| AC-14 | test | `command_analysis_test.go::TestSystemModificationRisk`（`ip`/`ifconfig`/`route`=Medium）＋ `indirect_execution_test.go::TestIndirect_IpExecGated`（`ip netns exec ns rm -rf /`=High、`ip netns exec ns modprobe x`=High、内側欠落=Reject、`ip netns list`=IndirectNone、グローバルオプション挿入 `ip -json netns exec ns rm -rf /`/`ip -n foo netns exec ns sh`=High でバイパス不可） |
| AC-15 | test | `command_analysis_test.go::TestSystemModificationRisk`（`mount`/`umount`=Medium） |
| AC-16 | test | `command_analysis_test.go::TestCommandRiskProfiles_PrivilegeEscalation`／`TestMigration_IsPrivilegeConsistency`／`TestMigration_RiskLevelConsistency`（`pkexec`/`runuser`/`setpriv`/`capsh`=Critical/IsPrivilege）＋ `evaluator_test.go::TestStandardEvaluator_EvaluateRisk_PrivilegeEscalation`（4 名の直接形 Critical）＋ `indirect_execution_test.go::TestIndirect_BypassAttackerScenarios`（`env pkexec`/`env runuser`/`env setpriv`/`env capsh`=IndirectCritical の 4 ケース） |
| AC-17 | test | `command_analysis_test.go::TestSystemModificationRisk`（`visudo`/`useradd`/`insmod`=High）＋ `evaluator_test.go`（新規ケース: `/usr/sbin/visudo`・`/sbin/insmod` の最終 Level=High かつ `BlockingReason != ReasonPrivilegeEscalation`） |
| AC-18 | test | `command_analysis_test.go::TestMigration_RiskLevelConsistency`（profile の `BaseRiskLevel()`==Medium を `curl`/`wget`/`scp`/`sftp`/`ssh`/`nc` で確認。`rsync` は `ConditionalNetwork()` ゆえ `BaseRiskLevel()`==Medium だが実効 Medium はリモートスタイル引数時のみ＝据え置きで High 化しない、アーキ §3.4）＋ `TestCommandRiskProfiles_NetworkCommands`／`TestMigration_NetworkTypeConsistency`（`sftp`=AlwaysNetwork） |
| AC-19 | test | `indirect_execution_test.go::TestIndirect_CommandExecOptionsGated`（case struct 拡張後。`rsync -e ssh`/`--rsh=ssh`/`-essh`/`-avze ssh`/`-aevz`（バンドル値抽出 getopt 準拠）=IndirectFloor かつ Level=High、抽出不能=Reject、`ssh -o ProxyCommand=…`/空白区切り=Level High、**`ssh -o LocalCommand='sudo id'`=Critical**（ProxyCommand と対称に検証）、`tar --to-command`/`--checkpoint-action`=Reject 据え置き、シェルメタ文字値=Reject、**改行を含む値 `rsync -e $'ssh\nsudo id'`/`ssh -o ProxyCommand=$'ssh\nsudo id'`=Reject**、特権トークン `rsync -e 'sudo cmd'`=IndirectCritical） |
| AC-20 | static | `rg -n "名前ベース AI 検出の限界" docs/dev/architecture_design/command-risk-evaluation.ja.md` 期待: 追記した固有見出しが 1 件ヒットする（既存記述と衝突しない一意マーカー。本文に未列挙/リネーム/multi-call/defense-in-depth/allowlist の論点を含む） |
| AC-21 | test | `command_analysis_test.go::TestSystemModificationRisk`（`parted`/`fsck`/`fdisk`/`mkfs`=High。AC-01/AC-03 と同テストで担保） |
| AC-22 | test | `indirect_execution_test.go::TestIndirect_NamespaceWrappersGated`（`chroot`/`unshare`/`nsenter`/`flock`/`watch` の内側ゲート High・特権内側 Critical・抽出不能 Reject。オプション読み飛ばし回帰: `nsenter -S 0 sh`=内側 `sh` を正しくゲート、任意引数 `unshare -m sudo id`/`nsenter -m sudo id`=Critical（`sudo` を値として食わない）、`nsenter -t 1 -w sudo id`=Critical（nsenter `-w` は任意引数）、`unshare -w /tmp sudo id`=Critical（unshare `-w` は必須値→`/tmp` を消費）、`unshare -r sudo id`=Critical（unshare `-r` はフラグ）、`chroot --userspec=0:0 /mnt sudo id`=Critical、`flock -w 10 /tmp/l sudo id`=Critical、`flock /tmp/l -c 'sudo id'`=Critical／`flock -c 'sudo id' /tmp/l`=Critical（`-c` は値オプション・位置自由）、`flock 9`=IndirectNone（fd 専用形）、`watch -n 1 sudo id`/`watch -q 1 sudo id`/`watch -c sudo id`=Critical、`watch -x rm -rf /`=High の argv 形）＋ `TestIndirect_NoCommandImplicitShellHigh`（`chroot /mnt`/`unshare`/`nsenter -t 1 -m`=Level High）＋ `evaluator_test.go`（`env modprobe x` の最終 Level=High 追加ケース。`sudo useradd u`=Critical は既存 `TestStandardEvaluator_EvaluateRisk_PrivilegeEscalation` の sudo 経路で担保） |
| AC-23 | test | `indirect_execution_test.go::TestIndirect_EnvTimeoutNoCommandHigh`（`env FOO=bar`/`env`/`timeout 5` no-command=High、ネスト `nice timeout 5`=High、value-less オプション付き `timeout --foreground 5`/`timeout -v 5`=High（Reject させない））＋ `TestIndirect_WrapperNoCommandMedium`（`nice`/`ionice`/`stdbuf`/`setsid` no-command=Medium 据え置き）＋ `TestIndirect_UnextractableWrapperRejected`（`env FOO=bar`=High へ更新）＋ `TestIndirect_WrapperLoaderEnvRejected`（`env LD_PRELOAD`=Reject）／`TestIndirect_BypassAttackerScenarios`（`env dpkg`=High・`sudo env`=Critical）の回帰 |
| NF-001 | test | `evaluator_test.go`（新規ケース。reason code は経路で異なることに注意: 直接 `modprobe`=High かつ `ReasonSystemModification`、`env modprobe`=High かつ `ReasonIndirectExecutionWrapper`（`evaluateInnerAs` の `RoleInner` フラット High 経路は per-command の system-modification reason を持たず wrapper reason に収斂する。アーキ §4）。両者を別アサーションとし、`env modprobe` に `ReasonSystemModification` を期待しない）＋ `risktypes/reason_codes_test.go::TestReasonCodes_AllDistinct`（新規コード無しの回帰） |
| NF-002 | static | `make test && make lint` が成功し、`make fmt && git diff --exit-code` が差分なし（exit 0）で完了する |
| NF-003 | static + manual | static: `SystemModificationRisk`/`ResolveProfile` のシグネチャは `map[string]struct{}` のみを引数に取り FS ハンドル・identity を持たない純関数であり、`TestSystemModificationRisk`／`TestMigration_*` が同一入力で決定的に同値を返すことを担保（runtime==dry-run は自明）。manual: P1〜P5 で追加・変更する判定経路の関数本体が書込・ネットワーク・identity 参照を含まず symlink メタ参照（`Lstat`/`Readlink`）に留まることを PR で確認 |

---

## 8. 成功基準

- **機能完全性**: AC-01〜AC-23 のすべてが §7 の対応テスト/検証で緑。
- **品質**: `make test`・`make lint`・`make fmt` がすべて成功（NF-002）。新規 `ReasonCode` を導入しない（NF-001）。
- **セキュリティ**: 新ラッパ・ip-exec・ヘルパーオプション経由のバイパス回帰がすべて deny/High。特権ラッパの
  直接形・ネスト形が Critical。fail-closed 分割規約がシェルメタ文字・置換・特権トークンを取りこぼさない。
- **文書**: AC-20 の検出限界が `command-risk-evaluation.ja.md` に追記済み。

---

## 9. 次のステップ

- 本実装計画書のレビューと承認（Reviewer による status 更新）。
- 承認後、`docs/dev/developer_guide/requirements_process.md` の手順に従い P1 から実装に着手する。
- 引き上げ/引き下げの移行ノート（changelog）・sample/test config 追従・ユーザー向け文書の最終整合・日英反映・
  オペランド毎の監査 family 区別は後続タスク 0142（判断軸2）・0143（監査/文書）が所有する（本タスクのスコープ外）。
