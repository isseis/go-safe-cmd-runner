# コマンド名ベース リスクレベル分類の一貫化 — アーキテクチャ設計書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-19 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は [01_requirements.md](01_requirements.md)（`approved`）の受け入れ基準を実装可能な設計へ落とす。
> 既存の実装レベル解説は [command-risk-evaluation.ja.md](../../dev/architecture_design/command-risk-evaluation.ja.md)、
> 概念モデルは [risk-level-classification-guide.ja.md](../../dev/architecture_design/risk-level-classification-guide.ja.md)。
> 決定経緯（撤回案・タスク横断の判断）は本文ではなく末尾「付録: 決定履歴」に隔離する。

---

## 1. 設計の全体像

### 1.1 設計原則

1. **既存の max 合成を維持する**（AC-31）。最終リスク = 適用 dimension の最大値という現行
   `evaluateDimensions`（[evaluator.go](../../../internal/runner/base/risk/evaluator.go)）の骨格は変えない。
   本タスクは「各 dimension が返す値」を 2 軸モデルへ整理し、不足していた**軸2（宛先ゾーン）dimension**を
   追加する。
2. **軸1（名前固定）と軸2（宛先ゾーン）を分離する**（D1）。名前で固定レベルが決まるファミリは
   名集合で、引数の対象パスで決まるロケーション定義コマンドは新規のゾーン分類器で扱う。
3. **判定は決定的・副作用なし・read-only**（NF-003, AC-28）。ゾーン判定のパス解決は `lstat`/`readlink`
   等の読み取りのみで、実行時と dry-run で同一結果になる。
4. **降格（Low/Medium 化）は fail-closed**（AC-17, AC-18）。safe-zone への Low 降格は安全要件を満たす
   場合に限り、満たせなければより高い既定（ordinary=Medium）に倒す。

### 1.2 なぜ既存方式では足りないか（YAGNI 確認）

現行は「名前固定レベル（`SystemModificationRisk`）＋引数パターン（`CheckDangerousArgPatterns`）」の
2 系統で、いずれも**同一コマンドに単一レベル**しか与えられない。F-004 は `cp`/`mv`/`rm` を**宛先パスの
関数**として（`/usr/bin`→High、workdir→Low）分類することを要求し、これは名前固定でも引数パターン
（固定の語句一致）でも表現できない。したがって **宛先パスを正規化・ゾーン分類する新規 dimension（軸2）** が
必要になる。名前で一意に決まるファミリ（kernel/auth/boot 等）には引き続き名集合（軸1）を用い、過剰な
精密化はしない（D5）。

### 1.3 概念モデル（2 軸 × max）

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    CMD[("RuntimeCommand")]
    GATE["前段ゲート（ランク1〜3）"]
    A1["軸1: 名前固定階級分類器"]
    A2["軸2: 宛先ゾーン分類器"]
    OTH["その他 dimension"]
    MAX["max 合成器"]
    OUT[("RiskAssessment")]

    CMD --> GATE
    GATE -->|"許可（Critical/Reject でない）"| A1
    GATE --> A2
    GATE --> OTH
    A1 --> MAX
    A2 --> MAX
    OTH --> MAX
    MAX --> OUT

    class CMD,OUT data
    class GATE,OTH,A1,MAX process
    class A2 enhanced
```

> 矢印 `X → Y` は「X の出力が Y の入力になる」処理フローを表す。`GATE` が Critical/Reject を返す場合は
> 軸計算に進まず即時確定する（§6.1）。軸1=`SystemModificationRisk` 他、軸2=`LocationDefinedRisk`（新規）、
> max 合成=`addDimension`。
>
> **Legend**: 🟦 data=入力/出力データ / 🟧 process=既存コンポーネント（変更なし） / 🟩 enhanced=本タスクで追加・変更。

---

## 2. システム構成

### 2.1 コンポーネント配置（パッケージ構造）

```mermaid
graph TB
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    subgraph risk ["internal/runner/base/risk/"]
        EV["evaluator.go<br>StandardEvaluator.evaluateDimensions"]
    end

    subgraph sec ["internal/runner/base/security/"]
        CA["command_analysis.go<br>名集合（軸1）/ ResolveCommandNames"]
        LZ["location_zoning.go（新規）<br>LocationDefinedRisk（軸2）"]
        SZ["safezone.go（新規）<br>SafeZoneResolver"]
        PR["path_resolve.go（新規）<br>operand 用 symlink 追従解決"]
        CU["coreutils.go<br>CoreutilsCommandRisk"]
        IE["indirect_execution.go<br>ラッパ/特権昇格解析"]
        TY["types.go<br>Config.SystemCriticalPaths"]
    end

    subgraph rtypes ["internal/runner/base/risktypes/"]
        RC["reason_codes.go<br>ReasonCode 定数"]
    end

    EV --> CA
    EV --> LZ
    EV --> CU
    EV --> IE
    LZ --> SZ
    LZ --> PR
    LZ --> CA
    LZ --> TY
    EV --> RC
    LZ --> RC

    class CA,CU,IE,EV,TY,RC enhanced
    class LZ,SZ,PR newpkg
```

> 矢印 `A → B` は「A が B を呼び出す/依存する」依存関係を表す。
>
> **Legend**: 🟩 enhanced=本タスクで改修する既存ファイル / 🟪 newpkg=新規ファイル。

### 2.2 データフロー（軸2 の評価）

```mermaid
sequenceDiagram
    participant EV as evaluator.go<br>evaluateDimensions
    participant LZ as LocationDefinedRisk
    participant SZ as SafeZoneResolver
    participant PR as path resolver
    participant CA as HasSystemCriticalPaths

    EV->>LZ: names, cmdPath, args, runCtx
    alt ロケーション定義ファミリでない
        LZ-->>EV: (Unknown, nil, applies=false)
    else 該当
        LZ->>SZ: runCtx（WorkDir/OutputFile/temp）
        SZ-->>LZ: SafeZone{Roots, Trusted}
        LZ->>PR: 対象オペランド（宛先/source/FILE）
        PR-->>LZ: 正規化済み絶対パス or Unresolved
        LZ->>CA: 正規化パス
        CA-->>LZ: trust-critical か否か
        LZ-->>EV: (level, reasonCodes, applies=true)
    end
    Note over EV: addDimension で max 合成
```

> 矢印は同期呼び出し（`-->>` は戻り値）。本図はシーケンス図で配色クラスを用いないため凡例は不要。
> `Unresolved`（宛先不確定）は AC-18 の fail-safe（Low にしない）へ分岐する。

---

## 3. コンポーネント設計

### 3.1 リスクレベル段階（変更なし）

`runnertypes.RiskLevel`（[config.go](../../../internal/runner/base/runnertypes/config.go)）の段数・
意味づけは維持する（スコープ外、§6）。**リスクの全順序は `Low(1) < Medium(2) < High(3) < Critical(4)`**。
`Unknown(0)` は**リスクの最小値ではなく「未判定／該当なし」の番兵**であり、リスクとして比較しない。max 合成
では `Unknown` を返した dimension は「寄与なし」を意味し（評価は `RiskLevelLow` を起点に積み上げる）、最終の
有効リスクが `Unknown` になることはない。

### 3.2 軸1: 名前固定階級（`command_analysis.go` の改修）

名前で固定レベルが決まるファミリを**意味のある名集合に再編**する。現行の
`highSystemModificationNames`／`mediumSystemModificationNames` を出発点に、High ファミリを追加し、
Medium 集合から High へ移すべきもの（`fdisk`/`parted`/`mkfs`/`fsck`）を移設する（AC-27, F-007）。

| 名集合（High。原則） | 対応 AC | 内容（代表例。確定列挙は実装で） |
|---|---|---|
| 破壊/デバイス初期化（①） | AC-01, AC-02, AC-03 | parted/fsck/wipefs/blkdiscard/sgdisk/gdisk/cgdisk/sfdisk/cfdisk/mkswap/mkfs(.\*)/fdisk/e2fsck/mke2fs/tune2fs/resize2fs/LVM 破壊系 |
| カーネル/モジュール・パラメータ（③/②） | AC-04 | insmod/modprobe/rmmod/kexec/sysctl |
| アカウント・認証 DB（②） | AC-05 | useradd/usermod/.../passwd/chage/newusers/vipw/vigr/visudo |
| ブート改変（②③） | AC-06 | grub-install/grub2-\*/efibootmgr/kernel-install/installkernel |
| サービス有効化（②） | AC-07 | chkconfig/update-rc.d（既存 PM/systemctl/service と同列） |
| 電源状態/ランレベル（②） | AC-07a | shutdown/reboot/halt/poweroff/telinit |
| ファイアウォール（②） | AC-08 | iptables/ip6tables/(ip6)tables-restore/nft/ufw/firewall-cmd |
| 能力付与（⑤） | AC-09 | setcap |
| 信頼境界の置換 intrinsic（④） | AC-10 | update-alternatives/dpkg-divert/alternatives/ldconfig |
| ジョブ/遅延・transient 実行（②③） | AC-10a | crontab/at/batch/systemd-run |

| 名集合（Medium。原則） | 対応 AC | 内容 |
|---|---|---|
| 限定スコープの system 変更 | AC-11, AC-12, AC-13 | LVM 作成/設定系・ip/ifconfig/route・mount/umount（既定） |

- `SystemModificationRisk(names)` は再編後の High 集合→High、Medium 集合→Medium、非該当→Unknown を返す
  （既存シグネチャ維持）。各ファミリは別 `map[string]struct{}` として定義し、`anyNameInSet` で照合する。
- `iptables-save`/`ip6tables-save`（stdout 出力）は High 集合に**含めない**（既定 Low）。`-f <file>` 出力は
  軸2 の zoning 対象（AC-08）。
- **High だが Critical にはしない**（AC-24）: kernel/auth/権限付与系（insmod・visudo・useradd 等）は本軸で
  High に固定するが Critical にはしない。これにより per-command の `risk_level` 明示許可で正当な特権バッチを
  実行可能に保つ（Critical は §3.6 の特権昇格ラッパに限定）。
- **記述方針（WHAT/HOW）**: 上表の「代表例」は非有界。distro 別名・完全列挙は実装時に各名集合へ追加する。

### 3.3 軸2: ロケーション定義ゾーン分類（新規 `location_zoning.go`）

宛先パスの関数でレベルが決まる「ロケーション定義コマンド」を評価する新規 dimension。

**責務**:
1. コマンドがロケーション定義ファミリか判定（名集合）。
2. コマンド別に**作用オペランド**を抽出（宛先/source/FILE/`if=`/`of=`/mountpoint）。
3. 各オペランドを正規化（§3.5 のパス解決）。
4. ゾーン分類（trust-critical / ordinary / safe-zone / unresolved）。
5. arg 軸の floor（再帰・setuid/権限付与・ブロックデバイス・機微 source・fail-safe）を適用。
6. 全 floor の max を返す。

**型定義（高レベル）**:

```go
// PathZone はロケーション定義コマンドのオペランドが属するゾーン。
type PathZone int

const (
    ZoneUnresolved PathZone = iota // 宛先不確定（AC-18 fail-safe）
    ZoneSafe                       // 信頼された safe-zone 内（Low 候補）
    ZoneOrdinary                   // 通常パス（Medium）
    ZoneCritical                   // trust-critical（High）
)

// LocationKind はロケーション定義コマンドの種別（オペランド抽出規則の切替に用いる）。
type LocationKind int

const (
    KindNotLocation LocationKind = iota
    KindWriteTarget   // cp/mv/install/tee/sponge/truncate/sed -i/redirect-writers
    KindDeleteTarget  // rm/rmdir/unlink/shred
    KindLink          // ln（source も対象）
    KindDevice        // dd（if=/of=）
    KindMount         // mount/umount
    KindPermission    // chmod/chown/chgrp/setfacl/chattr（軸 A・⑤）
    KindArchive       // tar/unzip（展開先）
    KindMknod         // mknod（無条件 High）
)
```

> 上記 enum と名集合の対応・確定メンバは実装で確定（WHAT/HOW 分離）。

**軸 A / 軸 B**（要件 F-004 の表をそのまま実装規則とする）:

- **軸 A（ゾーン非依存で High／拒否）**: 権限付与（setuid/setgid・world-write・trust-critical 所有権）→High
  （AC-20, AC-22a）。内側コマンド実行（`find -exec` 等）→**間接実行で Reject/Blocking**（§3.6, AC-22e。
  floor ではなく拒否）。
- **軸 B（ゾーン依存）**: trust-critical→High / ブロックデバイス IO→High / safe-zone 外ツリー再帰→High /
  機微 source 複製→Medium 下限 / 宛先不確定→Medium 下限 / ordinary→Medium / safe-zone→Low。

**`LocationKind` 別のオペランド抽出規則**（複数オペランドは各々を zoning し max。AC-31）:

| Kind | zoning 対象オペランド | 特則 |
|---|---|---|
| KindWriteTarget（cp/mv/install/tee/sponge/truncate/sed -i） | 書込先 FILE すべて（非フラグ引数）。`tee`/`sponge` は**全 FILE 引数**（AC-22d）、`-t/--target-directory` は宛先ディレクトリ | `install` の setuid/setgid・`-o`/`-g` は軸 A で High（AC-22a）。`cp -p`/`-a` の特権メタデータ複製は High（AC-22b） |
| KindDeleteTarget（rm/rmdir/unlink/shred） | 削除対象の全オペランド（AC-22b）。`-r`/`-R` で safe-zone 外なら再帰 High（AC-22） | — |
| KindLink（ln） | **宛先と source（リンク先）の双方**。trust-critical source の safe-zone 別名は High（AC-22b） | — |
| KindDevice（dd） | `if=`/`of=` のデバイス/ファイル | ブロックデバイス→High、`/dev/null` 等の無害シンクは除外、機微 `if=`→Medium 下限（AC-21） |
| KindMount（mount/umount） | **mountpoint と source の双方**（`--bind`/`--rbind`/`--move` の trust-critical source、デバイス source `/dev/sdaN` を含む）。`umount` は対象 FS/ディレクトリ | `umount -a`（全 FS）は無条件 **High**（AC-19） |
| KindPermission（chmod/chown/chgrp/setfacl/chattr） | 変更対象パス（軸 A・⑤） | 権限拡大/setuid/`chattr -i`/trust-critical 対象→High（AC-20） |
| KindArchive（tar/unzip） | 展開先（`-C`/`-d`） | 展開ルート脱出メンバ・特権メタデータ復元は fail-safe High（AC-22e 注記） |
| KindMknod（mknod） | （対象に依らず） | 無条件 **High**（safe-zone へのデバイスノード生成。AC-16 注記） |

`find` の書込/破壊/実行アクション（AC-22e）は KindNotLocation 扱いとし、`-fprint*`（FILE 書込）は
KindWriteTarget と同じ FILE オペランド zoning、`-delete` は再帰破壊（AC-22）、`-exec`/`-execdir`/`-ok`/
`-okdir` は間接実行 Reject（§3.6）として扱う。読取専用検索は昇格しない。

**公開関数（高レベル・シグネチャのみ）**:

```go
// LocationDefinedRisk は軸2を評価する。applies=false のとき本 dimension は寄与しない。
func LocationDefinedRisk(
    names map[string]struct{},
    cmdPath string,
    args []string,
    sz SafeZone,
) (level runnertypes.RiskLevel, reasons []risktypes.ReasonCode, applies bool)
```

### 3.4 safe-zone の導出（新規 `safezone.go`）

```go
// SafeZone は Low 降格を許す信頼ディレクトリ集合。Trusted=false の場合は Low 降格を行わない。
type SafeZone struct {
    Roots   []string // 正規化済み絶対パス（run の EffectiveWorkDir / OutputFile の親 / 専用 temp）
    Trusted bool     // AC-17(d): Low 降格を許す信頼前提を満たすか（下記「信頼判定」）
}

// SafeZoneResolver は実行コンテキストと信頼ポリシー（Config 由来）から SafeZone を構築する。
type SafeZoneResolver interface {
    Resolve(cmd *runnertypes.RuntimeCommand, cfg *security.Config) SafeZone
}
```

**safe-zone の起点ディレクトリ（AC-17(b)）**: 曖昧な `$HOME` ではなく、`RuntimeCommand.EffectiveWorkDir`
（フィールド）と `OutputFile()`（メソッド。空なら除外）の親ディレクトリ、および構成済みの専用 temp に
限定する。共有 `/tmp` は無条件 safe にしない。`$HOME` は含めない（§3.9 で既存
`EvaluateOutputSecurityRisk` との差として明記）。

**信頼判定（AC-17(d) TOCTOU、本書で確定）**: Low 降格は「外部コマンドが対象を open するまでに非特権ユーザが
パスをすり替えられない」ことを前提とする。Low へ下げるのは緩和方向のため**保守的に倒す**:
- `Trusted=true` の条件 = 次をすべて満たすこと:
  (1) safe-zone の起点ディレクトリが **config の信頼ディレクトリ許可リスト**（新規ポリシー。既定は空）に
      含まれる、かつ
  (2) 起点ディレクトリおよび解決後の対象パスの各経路要素が**非特権ユーザから書込不可**（owner が実行
      主体/管理者、かつ world/group-writable でない）であること。
- いずれも満たさない（既定）場合は `Trusted=false`。このとき `ZoneSafe` 該当パスは **Low ではなく
  ordinary（Medium）扱い**にフォールバックする（fail-closed）。
- **既存 TOCTOU モデルとの関係**: セキュリティアーキテクチャの既知制限（「FS 書込攻撃者は信頼境界外、
  ハッシュ検証＋実行ディレクトリ権限で緩和」）を**継承しつつ強化**する。Low 降格は『緩和』なので、
  同モデルの前提が成り立つ証拠（信頼ディレクトリ＋書込不可）が無ければ降格しない、という上乗せの安全側規則。
- 起点ディレクトリの存在チェック・権限確認は read-only（NF-003）。

**trust-critical との優先（AC-17(c)）**: safe-zone が trust-critical と重複/配下なら safe-zone として
扱わない。max 合成で自然に満たされる（trust-critical=High > safe-zone=Low）が、ゾーン分類器は
trust-critical を優先評価して二重計上を避ける（§6.3）。

### 3.5 オペランド用パス解決（新規 `path_resolve.go`）

軸2 は「対象パスが**実際に**どこを指すか」を解決する必要がある（AC-14, AC-17(a)）。

- **`safefileio` は流用しない**。`safefileio` は TOCTOU 対策として symlink を **解決せず拒否（Reject）** する
  設計であり、「どこを指すか」を返さない。zoning には**symlink を安全に追従して最終パスを返す**専用解決が要る。
  （要件 §5 の「safefileio 再利用」は参考記述で、AC-17(a) が優先する。）
- 既存 `walkSymlinkChain`（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go)、
  コマンド名解決用）と同型の**深さ制限つき symlink 追従＋サイクル検出**を、任意のオペランドパス向けに用意する。
- **未存在 leaf の扱い（重要・正規化契約）**: 書込/作成系（`cp x /newdir/y`）では宛先 leaf が未存在なのが通常
  ケース。leaf が存在しない場合は**存在する最深の親まで symlink 解決し、残りの末尾要素を正規化（`..`/`.`
  畳み込み）して合成**したパスをゾーン判定に用いる。これを欠くと作成系がすべて `ZoneUnresolved`→Medium に
  倒れ safe-zone→Low（AC-16）が成立しなくなる。親自体が symlink で trust-critical を指す場合はその解決先で判定。
- 解決不能（深さ超過/親まで到達不能/`%{VAR}` 等で未確定）は `ZoneUnresolved` とし、AC-18 の
  fail-safe（Low 不可）に倒す。
- 解決は read-only（NF-003）。

```go
// ResolveOperandPath は引数パスを正規化（symlink 追従）した絶対パスへ解決する。
// 解決できない場合は ok=false（呼び出し側は fail-safe で Low にしない）。
func ResolveOperandPath(p string) (resolved string, ok bool)
```

### 3.6 間接実行・特権昇格の拡張（`indirect_execution.go` の改修）

既存の `AnalyzeIndirectExecution` / `wrapperSpecs` / 子プロセス実行解析（find/xargs）/ loader 変数拒否
（`isLoaderControlVar`）を**拡張**する。新規パッケージは作らない（既存責務に集約）。

| 改修点 | 対応 AC | 内容 |
|---|---|---|
| 特権昇格ファミリ拡張 | AC-23 | `runuser`/`setpriv`/`capsh` を Critical 対象へ追加（既存 sudo/su/doas と同列） |
| 実行ラッパ拡張 | AC-29 | `chroot`/`unshare`/`nsenter`（名前空間/ルート変更）、`flock`/`watch`（コマンド文字列）を `wrapperSpecs` 系へ追加。内側は既存 RoleInner の flat High floor |
| COMMAND 省略の暗黙シェル | AC-29 | `chroot/unshare/nsenter` が内側未指定なら暗黙シェル起動とみなし素通りさせない（High 以上） |
| サブコマンド実行 | AC-12 | `ip netns exec`/`ip vrf exec <cmd>` を内側ゲート対象に |
| ヘルパー実行オプション | AC-25 | `ssh -o ProxyCommand/LocalCommand`・`rsync -e/--rsh` を内側ゲート/拒否 |
| redundant-with-config | AC-29a | `env`/`timeout` を軸1 High に分類（§3.7）。内側ゲートは従来どおり |

- **RoleInner の flat High floor は維持**（AC-29a）。`nice`/`ionice`/`stdbuf`/`setsid` 等は追加 floor を
  課さないが、抽出可能ラッパの内側は引き続き High floor を受ける。

### 3.7 `env`/`timeout` の High 化（AC-29a, D13）

`env`/`timeout` は TOML（`env_vars`/`env_import`/`timeout`）に安全な代替があるため、直接呼び出しを軸1 で
High に分類する。実装上は名集合（`redundantWrapperNames` 等）として `SystemModificationRisk` と同型の固定
レベル付与にするか、軸1 の High 集合の一つとして扱う。内側コマンドは間接実行解析で引き続きゲートされ、
最終は max（`sudo env …`→Critical）。Critical にはしない（D3）。

### 3.8 ロケーション定義 applet の固定 High 抑止（AC-22c。**3 系統すべて**）

D7（`rm`/`dd` 等の格下げ）と safe-zone Low（AC-16）を成立させるには、これら applet に**固定 High を与える
既存の全系統**を抑止し、軸2（`LocationDefinedRisk`）へ一元化する必要がある。現行コードには固定 High の
発生源が **3 系統**あり、いずれか 1 つでも残ると max 合成で軸2 の降格を打ち消す。

| # | 固定 High の発生源 | 該当 applet | 抑止方針 |
|---|---|---|---|
| ① | `IsDestructiveFileOperation`→High（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go) `destructiveCommandNames`=rm/rmdir/unlink/shred/dd） | rm/rmdir/unlink/shred/dd | 軸2 が `applies=true` の applet では本次元を寄与させない |
| ② | `CoreutilsCommandRisk`→High（[coreutils.go](../../../internal/runner/base/security/coreutils.go)。rm/dd=破壊系 High、cp/mv/不明=fail-safe High） | coreutils 配下の rm/cp/mv/dd/ln/install/truncate 等 | 同上（ロケーション定義 applet に限り固定 High を抑止） |
| ③ | **profile の `DestructionRisk`→High**（[command_analysis.go](../../../internal/runner/base/security/command_analysis.go) `commandProfileDefinitions`: `NewProfile("rm").DestructionRisk(High)`・`NewProfile("dd").DestructionRisk(High)`。`applyProfileFactors`→`ProfileFactorRisk` 経由） | rm/dd | これら applet の `DestructionRisk` 因子を軸2 担当へ移譲（profile からの固定 High を外す） |

- 抑止は**ロケーション定義 applet かつ軸2 が `applies=true` のとき**に限定する。`find -exec`/`rsync --delete`
  等「引数による破壊/実行」は軸2 ではなく間接実行・arg 軸が担うため、①〜③の安全網は非ロケーション applet には残す。
- coreutils 次元は引き続き「coreutils ディレクトリ配下の未知/読み取り applet」を fail-safe 分類する
  （本改修は重複責務の解消＝論点2 であり、coreutils 次元の安全網自体は撤去しない）。
- **影響テスト**は §7.2 に列挙（①〜③それぞれの High を表明する既存テストの期待値更新）。

### 3.9 軸2 と既存 `EvaluateOutputSecurityRisk` の統合（DRY）

既存 [file_validation.go](../../../internal/runner/base/security/file_validation.go) の
`EvaluateOutputSecurityRisk(path, workDir string) (runnertypes.RiskLevel, error)` は、**出力キャプチャ
（`output_file`）の宛先**を Critical/High/Low に分類する。軸2 はこれと**目的が異なる**（軸2 は任意の
コマンド引数オペランドのゾーン分類）が、判定要素が重なるため**部品を共有し、別々のレベル体系を作らない**。

| 要素 | 既存 `EvaluateOutputSecurityRisk` | 軸2 `LocationDefinedRisk` |
|---|---|---|
| 境界内包含判定 | `common.IsPathWithinDirectory`（セグメント境界） | **同じ `common.IsPathWithinDirectory` を再利用** |
| critical 集合とマッチ方式 | `OutputCriticalPathPatterns`/`OutputHighRiskPathPatterns`（部分文字列一致） | `Config.SystemCriticalPaths`＋境界マッチ `HasSystemCriticalPaths`（AC-14） |
| safe-zone（Low） | WorkDir 配下 **および現在ユーザの `$HOME` 配下**を Low | WorkDir/OutputFile/専用 temp のみ（**`$HOME` は safe-zone にしない**。AC-17(b)） |

- **意図的な差異（インライン明記）**: 軸2 は `EvaluateOutputSecurityRisk` より**厳格**で、`$HOME` を Low と
  しない。理由は本ツールが特権バッチ委譲を主目的とし、`$HOME` が root/対象ユーザで曖昧なため（AC-17(b)）。
  両者は別 API として併存する（出力キャプチャの既存挙動は変えない）。共通化できるのは境界内包含ヘルパーと
  critical パス集合の参照であり、これらを軸2 から再利用して重複実装を避ける。
- マッチ方式の差（部分文字列 vs 境界）は既存仕様であり、軸2 は**境界マッチ**（`HasSystemCriticalPaths`）に
  統一する（AC-14。`/srv` が `/` 配下で誤検出されない）。

### 3.10 evaluator の結線（`evaluator.go` の改修）

`evaluateDimensions` に軸2 dimension を追加し、safe-zone コンテキストを渡す。

- 追加: `LocationDefinedRisk(names, cmdPath, args, safeZone)` を呼び、`applies` のとき `addDimension` で max 合成。
- 改修（**§3.8 の 3 系統抑止を適用**）: ロケーション定義 applet かつ軸2 が `applies=true` のとき、
  ①`IsDestructiveFileOperation`→High、②`CoreutilsCommandRisk`→High、③profile の `DestructionRisk`→High
  を**いずれも寄与させない**（D7 の格下げ成立。§7.2 の影響テスト参照）。`find -exec`/`rsync --delete` 等
  「引数による破壊/実行」判定は間接実行・arg 軸側に残す。
- safe-zone は `SafeZoneResolver.Resolve(cmd, cfg)` で `RuntimeCommand` と `security.Config` から導出。

### 3.11 監査・理由コード（`reason_codes.go` の追加。NF-001）

軸2 と新カテゴリの監査可読性のため理由コードを追加する。新規コードは網羅性/一意性テスト（NF-001）に追従する。

```go
// 追加候補（最終名は実装で確定）
const (
    ReasonTrustBoundaryWrite ReasonCode = "trust_boundary_write" // ④ 信頼バイナリ/設定の置換・書込
    ReasonPermissionGrant    ReasonCode = "permission_grant"     // ⑤ setuid/所有権/能力の付与
    ReasonLocationZone       ReasonCode = "location_zone"        // 軸2 の宛先ゾーン由来（High/Medium）
)
```

- 既存コード（`ReasonSystemModification`/`ReasonDestructiveFileOperation`/`ReasonDangerousArgPattern`/
  `ReasonArbitraryCodeExecution`/`ReasonNetworkArgument`/`ReasonPrivilegeEscalation`）は引き続き使用する。
  軸1 の新ファミリは `ReasonSystemModification` を流用してよい（粒度は監査要件に応じ実装で確定）。

---

## 4. エラーハンドリング設計

- **解決失敗は fail-safe**（拒否ではなく下限引き上げ）: 軸2 のパス解決不能（`ResolveOperandPath` ok=false）は
  `ZoneUnresolved`→Medium 下限。コマンドを拒否（Blocking）にはしない（AC-18）。
- **間接実行の拒否は Blocking 維持**: `find -exec`/ヘルパー実行は identity-bind 不可のため既存どおり
  `IndirectReject`（Blocking 拒否）を返す（AC-22e）。これは軸2 の floor とは別系統。
- **判定中のエラーは内部エラーとして上位へ**: 既存 `evaluateDimensions` は coreutils の file-info 失敗を
  `blockingAssessment` で fail-closed にしている。軸2 は read-only 解決のみで、I/O エラーは「未解決」として
  fail-safe（Medium 下限）に倒し、評価全体を中断しない。
- 新規エラー型は導入しない（既存 `risktypes` の Blocking/Reason 機構に集約）。

---

## 5. セキュリティ考慮事項

### 5.1 脅威モデル

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    T1["脅威: 信頼バイナリの置換"]
    T2["脅威: 安全領域経由のすり替え"]
    T3["脅威: ラッパによる難読化"]
    T4["脅威: 遅延・永続実行"]

    D1["対策: 軸2 trust-critical 分類"]
    D2["対策: safe-zone 安全要件（AC-17）"]
    D3["対策: 間接実行ゲート / Reject"]
    D4["対策: 軸1 遅延実行ファミリ分類"]

    BK[("backstop データ<br>allowlist + ハッシュ固定")]

    T1 --> D1
    T2 --> D2
    T3 --> D3
    T4 --> D4
    D1 --> BK
    D2 --> BK
    D3 --> BK
    D4 --> BK

    class T1,T2,T3,T4 problem
    class D1,D2,D3,D4 enhanced
    class BK data
```

> 矢印 `脅威 → 対策 → backstop` は緩和経路を表す。具体例: T1=`cp/mv/ln/install` の `/usr/bin` 書込、
> T2=ハードリンク/別名/TOCTOU、T3=`env`/`chroot`/`ip netns exec`、T4=`crontab`/`at`/`systemd-run`。
> リスク判定は**二次ゲート**であり、列挙漏れは allowlist + ハッシュ固定（一次防御）が backstop する
> （AC-26, 0136 AC-66/67）。
>
> **Legend**: 🟥 problem=脅威 / 🟩 enhanced=本タスクの対策 / 🟦 data=一次防御データ。

### 5.2 設計上のセキュリティ要点

- **降格の安全性**（最重要）: 軸2 の Low 降格は「正規化 symlink 解決（AC-17a）＋信頼 safe-zone（AC-17b/c）
  ＋ TOCTOU 耐性（AC-17d）」を満たすときのみ。いずれか不成立なら Low に下げない（fail-closed）。これにより
  D7 の rm 格下げ（緩和）が安全領域に限定される。
- **dry-run 一貫性**（AC-28）: 軸2 は read-only 解決のみで副作用がなく、実行時／dry-run で同一レベル。
- **検出限界の明示**（AC-26）: 名前ベース分類は非有界・列挙漏れ前提。AI=High は明示ケースの
  defense-in-depth で、一般的なデータ送信（Medium）と確実には区別しない。

### 5.3 既存ポリシーへの例外（インライン明記）

- **0139 AC-06（fdisk/mkfs=Medium 維持）への例外**: 0139 要件書
  [0139/01_requirements.md](../0139_coarse_system_modification_risk/01_requirements.md) は AC-06 で
  「fdisk/mkfs=Medium 維持」と記す。**現状の実レベルは不揃い**: `fdisk`/`mkfs` は `dangerousCommandPatterns`
  経由で実際には **High**、`parted`/`fsck` は `mediumSystemModificationNames` 由来で実際に **Medium**。
  本設計は **fdisk/mkfs/parted/fsck=High を正**として軸1（破壊/デバイス初期化ファミリ）へ集約し、
  parted/fsck を Medium→High に引き上げる（AC-27）。0139 のドキュメントは改変せず、本書と移行ノートで
  上書き関係を明示する。**影響テスト**: [evaluator_test.go](../../../internal/runner/base/risk/evaluator_test.go)
  `TestStandardEvaluator_EvaluateRisk_SystemModifications`、`command_analysis_test.go`（`mediumSystemModificationNames` 表明）。

---

## 6. 主要処理フローの詳細

### 6.1 評価全体（ランク構造の中での軸2）

既存の `EvaluateRisk` のランク順は維持する。

```mermaid
flowchart TD
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    S(["EvaluateRisk"]) --> R1{"ランク1: 同一性ゲート OK?"}
    R1 -->|"NG"| BLK["Blocking 拒否"]
    R1 -->|"OK"| R2{"ランク2: 間接実行<br>Critical/Reject?"}
    R2 -->|"Critical"| CRIT["Critical"]
    R2 -->|"Reject"| RJ["Blocking 拒否"]
    R2 -->|"Floor/None"| R3{"ランク3: 特権昇格?"}
    R3 -->|"Yes"| CRIT
    R3 -->|"No"| DIM["ランク4〜8 + 軸2<br>evaluateDimensions（max）"]
    DIM --> OUT(["RiskAssessment"])

    class S,R1,R2,R3,BLK,RJ,CRIT,OUT process
    class DIM enhanced
```

> 矢印は制御フロー、菱形は分岐条件、丸括弧は開始/終了。軸2 はランク4〜8 と同じ max 合成内に位置し、
> Critical/Reject の確定後には評価されない（AC-23 の Critical 優先と整合）。
>
> **Legend**: 🟧 process=既存ステップ / 🟩 enhanced=本タスクで軸2 を加えるステップ。

### 6.2 `--dry-run` の副作用契約

- 本タスクはリスク**判定**のみを変更し、コマンドの副作用（書込/削除/送信）は変更しない。
- 軸2 のパス解決は `--dry-run` でも実行時でも**同一の read-only 解決**を行い、同一レベルを返す（AC-28, NF-003）。
  dry-run はコマンドの外部副作用を抑止するが、リスク判定ロジックの分岐には影響しない。

### 6.3 ゾーン分類の判定順（単一オペランド）

```mermaid
flowchart TD
    P["オペランドパス"] --> RES{"正規化解決できる?"}
    RES -->|"No"| UNRES["ZoneUnresolved → Medium 下限"]
    RES -->|"Yes"| CRT{"trust-critical?"}
    CRT -->|"Yes"| HIGH["ZoneCritical → High"]
    CRT -->|"No"| SAFE{"信頼 safe-zone 内?"}
    SAFE -->|"Yes"| LOW["ZoneSafe → Low"]
    SAFE -->|"No"| MED["ZoneOrdinary → Medium"]
```

> 矢印は制御フロー、菱形は分岐条件。本図は配色クラスを用いないため凡例は不要。各終端は
> ZoneCritical→High（AC-14）/ ZoneOrdinary→Medium（**AC-15**）/ ZoneSafe→Low（AC-16）/ ZoneUnresolved→
> Medium 下限（AC-18）に対応。複数オペランド・arg 軸 floor（再帰/setuid/デバイス/機微 source）はこの単一
> 判定の **max** を取る（AC-31）。trust-critical を safe-zone より先に判定して AC-17(c) を保証する。

---

## 7. テスト戦略

### 7.1 ユニットテスト

- **軸1 名集合**: 新 High ファミリ（kernel/auth/boot/FW/power/setcap/trust-boundary/scheduler）と Medium
  ファミリ（LVM 作成系/ip/mount）の名前→レベルを表明（AC-01〜AC-13）。
- **軸2 ゾーン分類**: trust-critical/ordinary/safe-zone/unresolved × 各 LocationKind の代表で
  level を表明。再帰（safe-zone 内 Low / 外 High）、setuid 付与、ブロックデバイス、機微 source、TOCTOU
  非信頼ゾーン→Medium フォールバックを網羅（AC-14〜AC-22e）。
- **間接実行拡張**: runuser/setpriv/capsh=Critical、chroot/unshare/nsenter（COMMAND 有/無）、flock/watch、
  ip netns/vrf exec、ssh ProxyCommand、rsync -e、env/timeout=High（AC-23, AC-25, AC-29, AC-29a）。
- **max 合成**: 軸1×軸2 同時該当の最大値（例 `cp -a … /usr/bin`=High）、順序非依存（AC-31）。
- **dry-run 一貫性**: 同一コマンドで runtime/dry-run 同値（AC-28）。

### 7.2 更新が必要な既存テスト（破壊的変更）

| テスト | 変更理由 |
|---|---|
| [evaluator_test.go](../../../internal/runner/base/risk/evaluator_test.go) `TestStandardEvaluator_EvaluateRisk_DestructiveFileOperations` / `TestEvaluateRisk_AbsoluteRmRfHigh` | D7: `rm`/`dd` 等の無条件 High → 宛先ゾーン依存（safe-zone=Low）。期待値の見直し |
| `evaluator_test.go` の profile 系（`TestEvaluateRisk_ProfileFactorFloor` 等）/ `profile_builder_test.go` | §3.8 ③: `rm`/`dd` の profile `DestructionRisk`→High をロケーション定義経路で抑止することに伴う期待値変更 |
| `TestStandardEvaluator_EvaluateRisk_SystemModifications` ほか | fdisk/mkfs/parted/fsck=High、新 High/Medium ファミリの追加（AC-01〜AC-13, AC-27） |
| `coreutils_test.go` / `coreutils_consistency_test.go` | §3.8 ②: ロケーション定義 applet の coreutils 固定 High 抑止に伴う期待値変更 |
| `command_analysis_test.go` / `command_analysis_dangerous_test.go` | 名集合再編・`dangerousCommandPatterns` からの名前のみエントリ移設（論点2） |
| `indirect_execution_test.go` | ラッパ/特権昇格ファミリ拡張 |

### 7.3 統合・後方互換・文書整合

- 既存 sample／テスト config で本変更により引き上がるコマンドに `risk_level` を付与（AC-35, 0139 AC-14 と同型）。
- 移行ノート（引き上げ AC-32 / 引き下げ AC-33）を整備。
- **文書整合（AC-34）**: `docs/user/risk_assessment.{ja,}.md`・用語集 `docs/translation_glossary.md`・
  開発者向け `command-risk-evaluation.{ja,}.md` を本タスクの分類（軸1 High/Medium 名集合・軸2 3 ゾーン・
  Critical 尖鋭化）に一致するよう更新する。概念ガイド `risk-level-classification-guide.ja.md` の最終化と
  英語版作成は**実装完了後**（AC-36 の順序）。

---

## 8. 実装の優先順位（フェーズ）

| Phase | 内容 | 主な AC |
|---|---|---|
| P1 | 軸1 名集合の再編（High/Medium ファミリ化、fdisk/mkfs 等の移設） | AC-01〜AC-13, AC-27 |
| P2 | パス解決（`path_resolve.go`）＋ safe-zone 導出（`safezone.go`） | AC-14, AC-17, AC-18 |
| P3 | 軸2 ゾーン分類（`location_zoning.go`）＋ evaluator 結線＋ coreutils 整合 | AC-14〜AC-22e, AC-22c, AC-31 |
| P4 | 間接実行・特権昇格の拡張＋ env/timeout High | AC-23, AC-25, AC-29, AC-29a |
| P5 | 理由コード追加・監査整合・dry-run 一貫性 | AC-30, AC-28, NF-001 |
| P6 | 文書整合・移行ノート・既存 config 追従 | AC-32〜AC-35（AC-36 のガイド最終化は実装完了後） |

> 0139 との関係: 0139 の名マッチ・固定レベル方針の延長（論点2 の一元化リファクタ）として P1 を実施する。
> `dangerousCommandPatterns` に残る名前のみエントリ（mkfs/fdisk/format）は P1 で軸1 名集合へ移設する。

## 9. 将来拡張

- **trust-critical 集合の拡張**: `/usr/local` 等は現状 `/usr` 配下として境界マッチで包含されるが、
  allowlist バイナリの配置運用に応じ `Config.SystemCriticalPaths` を拡張できる。
- **情報漏えい（read）モデル**: 機微 source の floor は本タスクで導入するが、完全な read 系分類は将来課題。
- **ファミリの確定列挙**: 各名集合は distro 別名を含め継続的に拡充できる（非有界・backstop は一次防御）。

---

## コンポーネント責務一覧（新規・変更ファイル）

| ファイル | 区分 | 責務 | 主な変更 |
|---|---|---|---|
| `security/command_analysis.go` | 変更 | 軸1 名集合・`SystemModificationRisk`・`ResolveCommandNames` | High/Medium ファミリ再編、fdisk/mkfs 等の移設、名前のみ危険パターンの移設 |
| `security/location_zoning.go` | 新規 | 軸2 ゾーン分類（`LocationDefinedRisk`・`PathZone`・`LocationKind`） | 新規 |
| `security/safezone.go` | 新規 | safe-zone 起点ディレクトリの導出・信頼判定（`SafeZone`・`SafeZoneResolver`） | 新規 |
| `security/path_resolve.go` | 新規 | オペランド用 symlink 追従解決（`ResolveOperandPath`） | 新規 |
| `security/coreutils.go` | 変更 | coreutils applet 分類 | §3.8 ②: ロケーション定義 applet の固定 High 抑止（軸2 委譲） |
| `security/command_analysis.go`（profile 定義） / `security/command_risk_profile.go` | 変更 | `commandProfileDefinitions`・`ProfileFactorRisk` | §3.8 ③: `rm`/`dd` の `DestructionRisk`→High をロケーション定義経路で抑止 |
| `security/file_validation.go` | 変更（小・再利用） | `EvaluateOutputSecurityRisk` | §3.9: 境界内包含ヘルパー（`common.IsPathWithinDirectory`）・critical パス集合を軸2 と共有。出力キャプチャ既存挙動は不変 |
| `security/indirect_execution.go` | 変更 | ラッパ/特権昇格/子プロセス実行解析 | 特権昇格・実行ラッパ・ヘルパーオプション拡張 |
| `security/types.go` | 変更（小） | `Config.SystemCriticalPaths`・safe-zone 信頼ポリシー | safe-zone 信頼ディレクトリ許可リスト設定の追加 |
| `risk/evaluator.go` | 変更 | `evaluateDimensions` | 軸2 dimension 結線、§3.8 の 3 系統 High 抑止、safe-zone 受け渡し |
| `risktypes/reason_codes.go` | 変更 | `ReasonCode` 定数 | 新規理由コード追加（網羅性テスト追従） |

---

## 付録: 決定履歴（本文の現在状態と分離）

- 本設計の WHAT（どのファミリ/ゾーンをどのレベルにするか）は [01_requirements.md](01_requirements.md) で確定。
  決定 D1〜D13・論点・撤回案の経緯は [00_notes.md](00_notes.md) に隔離してある。
- 主要な設計上の選択:
  - **軸2 を新規 dimension として追加**（既存の名前固定/引数パターンでは宛先依存を表現できないため。§1.2）。
  - **`safefileio` を zoning に流用しない**（symlink を拒否する設計で「指す先」を返さないため。§3.5）。
  - **coreutils 固定 High をロケーション定義 applet で抑止**（軸2 へ責務一元化。§3.8、論点2）。
  - **env/timeout を Critical でなく High**（特権昇格しないため。D3/D13、§3.7）。
- 過去タスクとの関係: 0139（PM/systemctl 粗粒度化）の延長として軸1 を再編。0139 AC-06 のドキュメント乖離は
  本タスクで訂正（§5.3）。
