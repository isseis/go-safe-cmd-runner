# 判断軸2: 宛先ゾーン分類の一貫化 — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-19 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は 0140 を 3 分割した第 2 タスク（**判断軸2＝宛先ゾーン分類**）の要件である。分割方針・根本原因の訂正は
> [0140/00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md)、原典の確定要件・根拠は
> [0140/01_requirements.md](../0140_risk_level_classification_review/01_requirements.md)（superseded）を参照する。
> **コマンド名分類・ラッパ/特権（判断軸1）は 0141**、**監査フィールドの logger 出力・変更ノート・文書は 0143**。

## 1. 背景と目的

本タスクは**ファイル操作コマンド**——ファイル/ディレクトリを書込・上書・削除・リンク・展開・マウント・権限変更
するコマンド（`cp`/`rm`/`dd`/`tar`/`mount`/`chmod` 等。read 専用は対象外）——を対象に、新しい判断軸（**判断軸2**）を
追加する。判断軸2 は、これらコマンドの**作用先パスを解決し、安全ゾーンに分類**してリスクを判定する。ゾーンと
レベルの対応はおおむね次のとおり（厳密な定義・パス集合は §4 F-001／AC-04）:

| ゾーン | 説明（代表パス） | レベル |
|---|---|---|
| **trust-critical** | システム重要パス（`/usr`・`/etc`・`/boot` 等、書込でシステム/信頼境界を侵すパス） | **High** |
| **ordinary** | 通常パス（`/srv`・`/opt` 等、trust-critical でも safe-zone でもないパス） | **Medium** |
| **safe-zone** | run 専用の作業/出力ディレクトリ・専用 temp（run が所有する安全領域） | **Low** |
| **解決不能（unresolved）** | パスを確定できない/曖昧 | **fail-closed 下限**（書込/削除先=High・読み取り元〔`cp` のコピー元・`dd` `if=`〕=Medium） |

最終リスクは判断軸1（0141, コマンド名分類）と **max 合成**する。

本タスクは [0140/00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §3 の
**3 つの根本原因の訂正**を担う中核である:
- **根本原因1**: 解析すべきコマンドライン引数（コマンド×フラグ×形式）の組み合わせが膨大で際限がなく**個別列挙では
  網羅しきれない**問題を、**fail-closed 既定＋オペランド抽出仕様の網羅テスト**で有限の検証範囲に収める。
- **根本原因2**: D7 の引き下げを「既存判定の**選択的 max 抑止**」（High を出す既存判定を個別に無力化して最終 max を
  下げる旧方式。最終リスクは複数判定の max なので 1 つでも外し漏れると High が残る。詳細は F-005）でなく、
  **判断軸2 を唯一の判定基準として既存の High 判定を置き換える**方式で実現する。
- **根本原因4**: DTO 配置・identity 注入・config 結線を**端から端で明示**する。

## 2. スコープ

- **In**: ファイル操作コマンドの宛先ゾーン分類（F-001）、作用オペランド抽出と網羅テスト（F-002）、操作固有の下限
  （ゾーン非依存。F-003）、データ送信のローカル書込 High 化と max 合成（F-004）、判断軸2 を唯一の判定基準とするゾーン経路による
  既存の High 判定の置き換え（F-005）、結線・DTO・identity 注入（F-006）、決定性（F-007）。
- **Out**:
  - **コマンド名分類（判断軸1 High/Medium）・Critical 限定・env/timeout・ラッパ/特権・データ送信の名前→Medium 下限**
    → 0141。`find -exec`/`-execdir`/`-ok`/`-okdir`・`ssh -o ProxyCommand`・`rsync -e` 等の**内側コマンド実行
    （間接実行 Reject）**も 0141/既存（本タスクは `find -delete`/`-fprint*` の**宛先のゾーン分類** のみ）。
  - **オペランド毎の監査フィールドの logger 出力・変更ノート（changelog）・文書整合・sample config 追従・ガイド** → 0143
    （本タスクは DTO 定義と `RiskAssessment` への格納まで）。
  - `RiskLevel` の段数/意味づけ変更（新レベル追加しない）。
  - **後方互換不要のため段階ロールアウト/フラグは設けない**（新分類は直接適用。0140/00 §3.2）。

## 3. 横断制約（0140/00_decomposition.md §3 を継承）

- **新分類は直接適用する**（後方互換不要。フラグ/shadow なし）。
- **結線をフェーズ内に含める**（根本原因4）: 判断軸2 を `risk/evaluator.go` の `evaluateDimensions` へ結線し、`security.Config`
  （`SystemCriticalPaths`・信頼ディレクトリ許可リスト）を評価器へ通すコンストラクタ/`runner.go`/TOML ローダ改修を
  本タスクに含める。**完了の判定基準（ビルド／テストを通す対象範囲）には、`risk` パッケージ単体だけでなく、本タスクが
  変更する統合パッケージ（`internal/runner`・`internal/runner/config`）までコンパイルが通ることを含める**（具体的には
  `./internal/runner/...` のビルド、または `make test` 全体が成功すること）。
- **0141 が再編した共有コードの上に構築する（並行ではなく 0141 の完了後）**: `evaluator.go` の `evaluateDimensions`（判断軸のディスパッチ）・`command_analysis.go` の名前集合は
  0141 が再編済みである前提で、その上に判断軸2 を追加する（この両タスクが共に触る共有コード＝0140/00 §実装順序の「共有境界」）。
- **English ソース**: Go の識別子・コメント・文字列リテラルは英語（テストソース含む）。

## 4. 機能要件と受け入れ基準

> 各 AC は 0140/01 の対応 AC を継承する（「対応」列/末尾）。**個別フラグは列挙しない**（根本原因1）。確定
> 列挙・LocationKind 定義は 0142/02_architecture.md と実装の仕様表で確定する。

### 4.0 共通の判定規則（全 AC に適用。各 AC で再掲しない）

- **解決後パスで判定**: すべてのゾーン判定は、symlink チェーンを追従する**専用リゾルバ（AC-04(a)）**で得た正規化済み
  絶対パスで行う。文字列 prefix（`common.IsPathWithinDirectory` 単独）での判定は**非適合**。
- **全オペランドの max**: 1 コマンドの**全作用オペランド**をゾーン分類して、その max を取り（AC-07）、さらに**判断軸1 とも max
  合成**する（AC-18）。
- **fail-closed 既定**: 解決/抽出が不確実なら `ZoneUnresolved`（書込/削除先→**High**・読み取り元〔`cp` のコピー元・`dd` `if=` 等〕→**Medium**。AC-05）。
- **唯一の判定基準**: ファイル操作コマンド（`rm`・`cp`・`dd` 等）は**判断軸2 を唯一の判定基準**とする。判断軸2 がコマンドを
  **完全に認識できたときだけ**（＝完全認識：全オペランドが確定ゾーンを返し、かつ argv を完全消費。定義は AC-17）、
  これらを High に分類している**既存の5つの判定**を判断軸2 の結果で置き換える。
- **下限は降格不可（ゾーン非依存）**: 次の**操作固有の下限**は、宛先が **safe-zone でも Low に降格しない**（F-003）——
  権限付与・デバイス IO・safe-zone 外への再帰・**機密ファイルの複製**。※機密ファイル＝秘匿内容のファイル
  （`/etc/shadow`・SSH 鍵 等。複製＝情報露出。定義は F-003）。

### F-001: ゾーン分類モデル

ゾーン → レベルの基本対応（条件はすべて解決後パス。共通規則 4.0）:

| AC | ゾーン | 条件 | レベル | 対応 |
|---|---|---|---|---|
| AC-01 | trust-critical | `GetSystemCriticalPaths()` に一致/配下 | **High** | 0140 AC-14 |
| AC-02 | ordinary | trust-critical でも safe-zone でもない通常パス | **Medium** | 0140 AC-15 |
| AC-03 | safe-zone（Trusted 充足） | safe-zone 内かつ AC-04 充足 | **Low** | 0140 AC-16 |
| AC-03 | safe-zone（Trusted 不成立） | safe-zone だが AC-04 不成立 | **Medium**（フォールバック） | 0140 AC-16 |
| AC-05 | unresolved | 解決/抽出不能・曖昧（未確定変数展開・未知フラグ・上限超過 等） | 書込/削除先=**High**・読み取り元〔`cp` のコピー元・`dd` `if=`〕=**Medium** | 0140 AC-18 |

各 AC の確定事項:
- **AC-01**: trust-critical 集合は `(*Config).GetSystemCriticalPaths()`
  （[security/types.go](../../../internal/runner/base/security/types.go)）を正とし、既定は `/`・`/bin`・`/sbin`・`/usr`・
  `/usr/bin`・`/usr/sbin`・`/etc`・`/var`・`/var/log`・`/boot`・`/sys`・`/proc`・`/dev`・`/lib`・`/lib64`・`/root`
  （deployment 拡張可。AC-20）。`/usr` 配下（`/usr/local/bin` 等）を含む。**`/` は完全一致のみ**（`/srv`・`/opt` 等は ordinary）。
- **AC-02**: 例 `/srv`・`/opt` 配下。**`/var`・`/var/log` は trust-critical なので ordinary の例・テストフィクスチャに
  使わない**。
- **AC-05**: ここで「**読み取り元**」とは、ファイル操作コマンドが**複製/参照のために読むパス**（`cp` のコピー元、`dd` の
  `if=` 等）を指す。コマンド自体は書込/削除を行うが、それを読み取るため**情報露出**リスクを持つ
  （`cp /etc/shadow $WORKDIR/x` 等）——read **専用**コマンド（`cat` 等。本タスク対象外）とは別概念。「不明フラグ＝
  安全」とは仮定しない。**未解決の読み取り元を High でなく Medium とするのは意図的な非対称**（書込/削除の最悪＝破壊、
  読み取り元の最悪＝情報露出という脅威差。完全な read モデルは将来課題＝§6）。02 で根拠を保持し「うっかり緩和」を
  防ぐ。（0140 AC-18 を Kind 依存 High まで強化）

- **AC-04**（safe-zone の定義と解決, 安全要件。0140 AC-17）: safe-zone 判定は次をすべて満たす。
  - (a) **専用リゾルバ**で正規化（symlink 解決後）の絶対パスを得て判定する（`~/link→/etc`・`$HOME/../../etc` 等で
    破れない）。リゾルバは `ResolveCommandNames`/`walkSymlinkChain` 型の**深さ制限つき symlink 追従**（leaf＋親。本
    タスクで新規実装、未存在 leaf は最深の存在親まで解決して末尾を畳み込む）。**`common.IsPathWithinDirectory` 単独
    （非解決）での代替は非適合**。`safefileio` は symlink を解決せず拒否する設計のため流用不可。**必須テスト**:
    `cp evil $WORKDIR/link`（`link→/etc/passwd`）は **High**（ターゲット解決）であり Low にならない。
  - (b) 起点は **`RuntimeCommand.EffectiveWorkDir` と構成済み専用 temp** に限定。曖昧な `$HOME`・共有 `/tmp`・出力先の
    親ディレクトリは**含めない**。
  - (c) safe-zone が trust-critical と重複/配下のときは trust-critical（High）を優先する。
  - (d) **TOCTOU 耐性（オペランド毎のTrusted）**: Low 降格は、解決後の各オペランドパスが**信頼ディレクトリ許可リスト
    配下**かつ**経路要素が run-as から書込不可**（run-as 以外所有・group/other 非書込）のときに限る。満たせなければ
    降格しない（fail-closed）。参照 identity は live euid でなく config の run-as 値（AC-21）。leaf が既存 symlink なら
    最終ターゲットでゾーン分類。

### F-002: 作用オペランドの抽出と網羅テスト（根本原因1）

- **AC-06**（抽出の網羅性は仕様表＋テストで担保）: 各ファイル操作コマンドの**作用オペランド**（宛先/読み取り元/
  FILE/`if=`/`of=`/mountpoint/展開先 等）を抽出してゾーン分類する。対象コマンドは cp・mv・rm・rmdir・unlink・shred・
  ln・mkdir・touch・install・tee・sponge・truncate・`sed -i`・tar・unzip・dd・mount・umount・chmod・chown・chgrp・
  setfacl・chattr・mknod・find（破壊/書込アクション）・データ送信の書込形（F-004）。**個別フラグ/形は要件本文で
  列挙せず、コマンド→Kind→オペランド抽出規則を単一の仕様（実装内テーブル）で表し、既知コマンド×代表フラグの
  表駆動（プロパティ/網羅）テストで被覆を担保**する。未知/曖昧形は fail-closed（AC-05）。
  AC-06 の**必須テスト行**（仕様表のエントリとして持つ。下表の難所は最低限）:

  | 難所 | 抽出/ゾーン分類規則 |
  |---|---|
  | in-place 編集 | `truncate`/`sed -i` の被編集 FILE を書込先としてゾーン分類 |
  | `ln -s` 相対 target | リンク**親**ディレクトリ基点で解決（`EffectiveWorkDir` 基点ではない） |
  | アーカイブ 抽出 vs 一覧 | `tar -x`/`unzip`＝展開先をゾーン分類、`tar -t`/`unzip -l`＝非昇格、`tar --one-top-level=DIR`＝抽出先、`-C`/`-d` 省略時＝`EffectiveWorkDir` |
  | 末尾 `/` 付き削除 | symlink を dereference してターゲットをゾーン分類 |
  | `dd` デバイス | `if=`/`of=` をデバイス種別で判定（F-003 AC-10） |
  | 権限/所有権付与 | world-write/所有権付与は操作固有の下限（F-003 AC-08）へ |
  | データ送信 書込先 | `curl -o`/`-O`、`wget` 既定/`-O`/`-P <dir>`、`scp host:/x <DEST>`、`sftp` バッチ書込、`rsync … <DEST>`/`--delete`（F-004） |

- **AC-06a**（仕様表と AC の連動）: **AC-08〜AC-16 で High/Medium 化と名指しされた全ての書込/削除/付与形は、
  オペランド抽出仕様表に対応するテスト行を持つ**こと（AC 本文と仕様表のドリフト防止＝根本原因1）。
- **AC-07**（複数オペランド）: 1 コマンドの作用オペランドが複数のときは各々をゾーン分類し **max** を取る（共通規則 4.0）。
  （0140 AC-31 の一部）

### F-003: 操作固有の下限（ゾーン非依存）とオペランド別特則

**(a) ゾーン非依存の下限**（safe-zone でも Low に降格しない。共通規則 4.0）:

| AC | 下限 | 条件 → **High** | 補足 | 対応 |
|---|---|---|---|---|
| AC-08 | 権限/所有権/属性付与 | setuid/setgid 付与・world-write 等の権限拡大・trust-critical 所有権変更・`chattr -i`（完全性制御除去） | 例 `chmod u+s`・`chmod 0777`・`chown root /usr/bin/x`・`chattr -i /etc/shadow` | 0140 AC-20 |
| AC-09 | `install` 権限フラグ | `-m` に setuid/setgid、または `-o`/`-g` で所有者/グループ変更 | safe-zone でも降格しない | 0140 AC-22a |
| AC-10 | `dd` デバイス IO | `if=`/`of=` がブロックまたは危険キャラクタデバイス（`/dev/mem`・`/dev/kmem`・`/dev/port` 等の物理/カーネルメモリ生アクセス） | 無害シンク（`/dev/null`・`/dev/zero`）除外。機密ファイル/trust-critical な `if=`（読み取り元） は **Medium 下限**。パス文字列でなく**デバイス種別**で判定 | 0140 AC-21 |
| AC-11 | safe-zone 外への再帰 | `rm -r`/`-R`・`cp -R`/`-a` 等が作用対象を safe-zone の外（ordinary/trust-critical）に及ぼす | 信頼 safe-zone 内に閉じた再帰（`rm -rf $WORKDIR/build`）は Low。複数オペランド指定自体は昇格条件にせず各々ゾーン分類 | 0140 AC-22 |

**(b) コマンド別のオペランド特則**（どのオペランドをゾーン分類するか。ゾーンに従うが下記の上乗せ/例外あり）:

| AC | コマンド | ゾーン分類対象 | 特則 | 対応 |
|---|---|---|---|---|
| AC-12 | cp/mv/rm/shred/unlink/ln | 全オペランド（mv は移動元・ln はリンク元も） | trust-critical な移動元/リンク元の mv/ln は High。`cp` は宛先判定だが**機密ファイル/trust-critical なコピー元の複製**は safe-zone でも Medium 下限、`cp -p`/`-a` の特権メタデータ複製（setuid/root 所有のコピー元）は High | 0140 AC-22b |
| AC-13 | mount/umount | mountpoint＋マウント元 | trust-critical→High（`--bind`/`--rbind`/`--move` のマウント元・デバイス含む）、`umount -a`→無条件 High、他は Medium | 0140 AC-19 |
| AC-14 | tee/sponge | 全 FILE 引数（非フラグ） | 複数 FILE は各々ゾーン分類 して max。内側コマンドは実行しない | 0140 AC-22d |
| AC-15 | find（破壊/書込） | 探索起点（省略時 `EffectiveWorkDir`）/書込先 FILE | `-delete`/`-fprint*` をゾーン分類（trust-critical 起点→High、信頼 safe-zone 起点→Low）、読取専用は非昇格、`-exec`/`-execdir`/`-ok`/`-okdir` の内側実行は**間接実行 Reject**（0141/既存。本タスク対象外） | 0140 AC-22e |

> **用語「機密ファイル」**: 内容が秘匿情報のファイル（読む/複製すると**情報が露出**するもの）。安全ゾーンへ
> コピーしても内容（秘密）が漏れるため、**機密ファイル/trust-critical なコピー元の複製は safe-zone でも Medium 下限**にする
> （AC-12/AC-10。これが「読み取り元」の下限）。判定集合は既存の `OutputCriticalPathPatterns`
> （[file_validation.go](../../../internal/runner/base/security/file_validation.go)）を流用し、例として:
> 認証 DB（`/etc/shadow`・`/etc/sudoers`）、SSH/鍵（`id_rsa`・`id_ed25519`・`.ssh/`・`private_key`）、資格情報
> （`.aws/credentials`・`.kube/config`・`.gnupg/`・`.docker/config.json`）、keystore/ウォレット（`wallet.dat`・`keystore`）等。
> 完全な read 系分類は将来課題（§6）。

### F-004: データ送信のローカル書込 High 化と max 合成

- **AC-16**（ローカル trust-critical 書込）— 0140 AC-25 の書込先部分:
  - **規則**: データ送信系の**ファイル書込/削除形**がローカルの trust-critical パスへ作用する場合 → ④信頼境界破壊として
    **High**。対象形（書込先の抽出は F-002 仕様表）:
    - `curl -o /usr/bin/x`・`curl -O <url>`（URL 由来名を cwd へ）
    - `wget -O /etc/cron.d/x`・`wget` 既定・`wget -P <dir>`
    - `scp host:/x /usr/bin/x`・`sftp` バッチ書込
    - `rsync … <DEST>`・`--delete`
  - **合成**: 最終リスク = **`max(データ送信の名前→Medium〔0141〕, 書込先ゾーン)`**。同一コマンドが 0141/0142 両方で
    評価されるため、**max 合成の所有者・テストは 0142**。
  - **必須テスト**（両寄与が同時に生きていることを検証）:
    - (i) safe-zone 宛先（`curl <url> -o $WORKDIR/safe`）→ **Medium**（書込先ゾーンが Low でも名前下限が効く）
    - (ii) trust-critical 宛先（`curl -o /usr/bin/x`）→ **High**（書込先ゾーンが名前下限を上回る）
  - **前提**: 0141 の名前→Medium 下限が評価器に結線済みであること（0141 が再編する共有コード。§3）。未結線では (i) が見かけの下限で誤って通る。

### F-005: 判断軸2 を唯一の判定基準とし、既存の High 判定を置き換える（根本原因2）

> **用語**: ここでの「既存の High 判定」とは、これらのコマンドを現在 High に分類しているコード上の判定経路を指す
> （最終リスクは全判定の **max** で決まるため、引き下げにはこれらすべてを無力化する必要がある）。下記①〜⑤。

- **AC-17**（判断軸2 が既存の High 判定を置き換える）— 0140 AC-22c を「唯一の判定基準」方式へ訂正:
  - **規則**: ファイル操作コマンドは判断軸2 の結果を**唯一の判定基準**とし、`LocationResult` を唯一の寄与とする。当該
    コマンドを High に分類している**既存の判定 5 系統を評価対象から外す**:

    | # | 評価対象から外す既存判定 |
    |---|---|
    | ① | `IsDestructiveFileOperation` |
    | ② | `CoreutilsCommandRisk` の破壊系 High |
    | ③ | profile `DestructionRisk` |
    | ④ | `dangerousCommandPatterns`(rank6) の `rm -rf`/`dd if=` 等のコマンドエントリ |
    | ⑤ | coreutils の setuid/setgid lstat 下限 |

  - **置き換え条件（完全認識のときのみ）**: (a) 抽出された**全オペランド**が非 `Unknown` の確定ゾーンを返し、かつ
    (b) オペランド抽出器が **argv を完全消費**した（未消費の非フラグトークン無し・パスを運び得る未知の値取りフラグ無し）。
  - **不完全認識のとき（fail-open 回避）**: 部分的/不確実なパース（一部オペランド未認識・未消費トークン残存・未知の値取り
    フラグ）→ `ZoneUnresolved` とし **①〜⑤の High を残す**。「一部だけ認識した」で①〜⑤を外すと、理解できない危険形が
    低リスクゾーンと誤判定され Low で素通りする（fail-open）。AC-05/AC-07 の「全オペランド max・未解決→High」が置き換え後も下限として残る。
  - **例外**: ⑤の setuid 下限は再パースせず既存 lstat シグナル（`hasSetuidOrSetgidBit` 相当）を操作固有の下限の判定が流用。
    **ファイル操作コマンド以外**（`find -exec` の内側実行・判断軸2 が扱わない未知コマンド）は従来どおり既存判定/間接実行が
    担う（同名でも非ファイル操作用途では④を無効化しない）。
  - **観測可能プロパティ（テスト）**:
    - 信頼 safe-zone `rm -rf $WORKDIR/build` → **Low**（④ rank6・②coreutils の固定 High で打ち消されない）
    - ordinary `rm /srv/app/cache.dat` → **Medium**
    - 未知フラグで宛先不確実な `rm` → **High**（①〜⑤を残す）
- **AC-18**（max 合成）— 0140 AC-31: 最終リスクは**適用される判定の max**。判断軸1（コマンド名分類）と判断軸2（宛先
  ゾーン）が双方適用されるコマンドはその最大値（例 `cp -a … /usr/bin`＝High）。順序非依存。

### F-006: 結線・DTO・identity 注入（根本原因4）

- **AC-19**（オペランド毎の監査 DTO の配置と内容検証）— 新規。0140/00 §3.4:
  - **規則**: オペランド毎の判定記録 DTO（`OperandZone`/`PathZone` 相当: Index/Raw/Resolved/Zone/MatchedCritical/Trusted/
    UnresolvedErr）を **`risktypes` に定義**し、`RiskAssessment` に格納（`security → risktypes` 一方向依存を維持。
    `security` に置くと循環）。logger への JSON 出力は **0143**。本タスクは `RiskAssessment` への格納までを担保。
  - **テスト**: 存在確認だけでなく**格納値の正しさ**を検証。代表コマンド（`cp evil /usr/bin/ls`・symlink 経由・複数
    オペランド）で `RiskAssessment` から直接 `[]OperandZone` を読み、各要素の Index/Raw/Resolved/Zone/Trusted が期待
    どおりか表明（値が誤っても 0143 まで気付けない穴を塞ぐ）。
- **AC-20**（`security.Config` の結線）— 新規。0140/00 §3.4:
  - **規則**: deployment の `Config.SystemCriticalPaths` と**信頼ディレクトリ許可リスト**を評価器へ通す。
    `NewStandardEvaluator`・`runner.go` のコンストラクタ結線＋信頼ディレクトリの **TOML `[security]` spec＋ローダ＋
    runner 転送**を本タスクで追加。
  - **根拠**: 無ければ configured 環境で AC-01/AC-04 が成立せず、テスト注入でしか通らない。
- **AC-21**（identity 注入の純粋性）— 新規。0140/00 §3.4:
  - **規則**: run-as 名→UID/GID/補助 group の解決は**ゾーン分類の外（評価器結線層）**で行い、precomputed `RunAsIdent` を
    `ZoningInput` に注入。**ゾーン分類は live identity（`os.Geteuid`/`os.Getuid`/`syscall`/`unix` の uid/gid/groups・
    `user.Current`）を読まない**。
  - **テスト（差分テストを主）**: 注入 `RunAsIdent` を**テストプロセスの実 euid/gid と異なる値**にし、Trusted/Low 判定が
    **注入 identity に従って変わる**ことを表明（決定性テストだけでは「単一プロセス内で euid 一定なら `os.Geteuid()` を
    読んでも決定的」になり live 参照の不在を証明できない）。
  - **補助**: 上記 live-identity API を判断軸2 の結線コードが呼ばない grep ガード。

### F-007: 決定性・read-only

- **AC-22**（runtime==dry-run・read-only）: 判断軸2 のパス解決は `lstat`/`readlink` のみの **read-only** で、runtime と
  dry-run で同一レベルを返す。結果は live euid・`$HOME` env に依存しない（AC-21）。（0140 AC-28／NF-003）
- **AC-23**（解決コストの上限・fail-closed）: 解決は**評価単位でメモ化**（同一親の再解決をしない）し、**1 コマンド
  評価あたりのオペランド総数（>N）または symlink 追従ホップ総数（>M）が上限を超えたら `ZoneUnresolved`→High**
  （書込/削除）に倒す（具体値 N/M は 02/実装で確定）。**必須テスト**: 上限を超える入力（多数オペランド・深い
  symlink チェーン）で fail-closed（High）になり、メモ化により注入関数（lstat/stat）の呼出回数が線形に収まることを表明する
  （ExecuteCommand ホットパスでの無制限 FS I/O・DoS を防ぐ）。（新規／0140/02 §3.5）

## 5. 非機能要件

- **NF-001**: 本タスクが追加した `ReasonCode`（例: 信頼境界書込・権限付与・ゾーン由来）は、**本タスク内で**網羅性/
  一意性テストを緑に保つ（family 区別の最終化は 0143）。（0140 NF-001）
- **NF-002**: `make test`・`make lint`・`make fmt` が成功する。完了の判定基準には、`risk` パッケージ単体だけでなく、
  本タスクが変更する統合パッケージ（`internal/runner`・`internal/runner/config`）までコンパイルが通ることを含める。（0140 NF-002）
- **NF-003**: 判定は決定的で副作用がなく、safe-zone 判定のパス解決は読取のみ（AC-22）。（0140 NF-003）

## 6. スコープ外の根拠

- **コマンド名分類・ラッパ/特権は 0141**: コマンド名で決まるレベル、Critical 限定、env/timeout、間接実行
  （`find -exec`/ProxyCommand/`rsync -e`）は argv の宛先解析を伴わず 0141 の所掌（D5 の線引き）。
- **logger 出力・文書・config 追従は 0143**: 監査フィールドの実際の JSON 出力、変更ノート、ユーザー/開発者文書、
  sample config の `risk_level` 追従は横断成果物として 0143 に集約する。
- **段階ロールアウト/フラグは無し**: 後方互換不要のため（0140/00 §3.2）。
- **`RiskLevel` 段数/新レベル**: 変更しない（0140 §6 を継承）。
- **完全な情報漏えい（read）モデル**: 機密ファイルの下限は導入するが、完全な read 系分類は将来課題
  （0140/02 §9 を継承）。
