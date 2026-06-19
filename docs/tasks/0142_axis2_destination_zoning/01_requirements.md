# 軸2: 宛先ゾーン分類の一貫化 — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-19 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は 0140 を 3 分割した第 2 タスク（**軸2＝宛先ゾーン分類**）の要件である。分割方針・root-cause 訂正は
> [0140/00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md)、原典の確定要件・根拠は
> [0140/01_requirements.md](../0140_risk_level_classification_review/01_requirements.md)（superseded）を参照する。
> **名前固定階級・ラッパ/特権（軸1）は 0141**、**監査フィールドの logger 出力・変更ノート・文書は 0143**。

## 1. 背景と目的

ロケーション定義コマンド（ファイルを書込/上書/削除/リンク/展開/touch するもの）の脅威は**宛先パスの関数**で
決まる。本タスクは宛先を**ゾーン分類**（trust-critical→High / ordinary→Medium / safe-zone→Low / 解決不能→
fail-closed floor）し、軸1（0141）と **max 合成**する新 dimension（軸2）を追加する。

本タスクは [0140/00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §3 の
**3 つの root-cause 訂正**を担う中核である:
- **根本原因1**: argv パースサーフェスの発散を、**fail-closed 既定＋オペランド抽出仕様の網羅テスト**で有界化する。
- **根本原因2**: D7 の引き下げを「max 抑止」でなく**単一権威ゾーン経路で旧 High 源を置換**して実現する。
- **根本原因4**: DTO 配置・identity 注入・config 結線を**端から端で明示**する。

## 2. スコープ

- **In**: ロケーション定義コマンドの宛先ゾーン分類（F-001）、作用オペランド抽出と網羅テスト（F-002）、軸 A の
  ゾーン非依存 floor（F-003）、データ送信のローカル書込 High 化と max 合成（F-004）、単一権威ゾーン経路による
  旧 High 源の置換（F-005）、結線・DTO・identity 注入（F-006）、決定性（F-007）。
- **Out**:
  - **名前固定階級（軸1 High/Medium）・Critical 尖鋭化・env/timeout・ラッパ/特権・データ送信の名前→Medium floor**
    → 0141。`find -exec`/`-execdir`/`-ok`/`-okdir`・`ssh -o ProxyCommand`・`rsync -e` 等の**内側コマンド実行
    （間接実行 Reject）**も 0141/既存（本タスクは `find -delete`/`-fprint*` の**宛先 zoning** のみ）。
  - **per-operand 監査フィールドの logger 出力・変更ノート（changelog）・文書整合・sample config 追従・ガイド** → 0143
    （本タスクは DTO 定義と `RiskAssessment` への格納まで）。
  - `RiskLevel` の段数/意味づけ変更（新レベル追加しない）。
  - **後方互換不要のため段階ロールアウト/フラグは設けない**（新分類は直接適用。0140/00 §3.2）。

## 3. 横断制約（0140/00_decomposition.md §3 を継承）

- **新分類は直接適用する**（後方互換不要。フラグ/shadow なし）。
- **結線をフェーズ内に含める**（根本原因4）: 軸2 を `risk/evaluator.go` の dimension へ結線し、`security.Config`
  （`SystemCriticalPaths`・信頼ディレクトリ許可リスト）を評価器へ通すコンストラクタ/`runner.go`/TOML ローダ改修を
  本タスクに含める。完了基準は触れる統合パッケージ（`internal/runner`・`internal/runner/config`）も**コンパイルする
  範囲**（`./internal/runner/...` または `make test`）。
- **0141 の共有境界の上に直列で構築する**: `evaluator.go` の dimension ディスパッチ・`command_analysis.go` の名前集合は
  0141 が再編済みである前提で、その上に軸2 を追加する。
- **English ソース**: Go の識別子・コメント・文字列リテラルは英語（テストソース含む）。

## 4. 機能要件と受け入れ基準

> 各 AC は 0140/01 の対応 AC を継承する（末尾「対応」列）。**個別フラグの散文列挙はしない**（根本原因1）。確定
> 列挙・LocationKind 定義は 0142/02_architecture.md と実装の仕様表で確定する。

### F-001: ゾーンモデルと fail-closed 既定

- **AC-01**（宛先ゾーン基本）: ロケーション定義コマンドの宛先が **trust-critical** のとき **High**。trust-critical 集合は
  `(*Config).GetSystemCriticalPaths()`（[security/types.go](../../../internal/runner/base/security/types.go)）を正とし、
  既定は `/`・`/bin`・`/sbin`・`/usr`・`/usr/bin`・`/usr/sbin`・`/etc`・`/var`・`/var/log`・`/boot`・`/sys`・`/proc`・
  `/dev`・`/lib`・`/lib64`・`/root`（deployment 拡張可。AC-20）。`/usr` が含まれるため `/usr/local/bin` 等の配下も
  trust-critical。**`/`（ルート）はルート完全一致のみ**（`/srv`・`/opt` 等の他トップレベルは ordinary）。判定は
  **正規化・symlink 解決後の絶対パス**で行い、生の引数文字列 prefix では判定しない（AC-04(a)）。（0140 AC-14）
- **AC-02**（ordinary）: trust-critical でも safe-zone でもない通常パスの named-file 操作は **Medium**
  （例: `/srv`・`/opt` 配下）。**`/var`・`/var/log` は trust-critical（AC-01）なので ordinary の例・テスト
  フィクスチャに使わない**。（0140 AC-15）
- **AC-03**（safe-zone → Low）: 宛先が safe-zone 内かつ AC-04 の **Trusted** 条件を満たす named-file 操作は **Low**。
  Trusted を満たさない safe-zone（run-as 書込可な一般 workdir 等）は **Medium** にフォールバックする。（0140 AC-16）
- **AC-04**（safe-zone の定義と解決, 安全要件）: safe-zone 判定は次をすべて満たす。
  (a) **正規化（symlink 解決後）の絶対パス**で判定する（`~/link→/etc`・`$HOME/../../etc` 等で破れない）。判定には
      **symlink チェーン（leaf＋親）を安全に追従・解決する専用リゾルバ**（`ResolveCommandNames`/`walkSymlinkChain`
      型の深さ制限つき追従。本タスクで新規実装し、未存在 leaf は最深の存在親まで解決して末尾を畳み込む）を用いる。
      **symlink を解決しない文字列 prefix ヘルパー（`common.IsPathWithinDirectory` 単独）で代替してはならない**
      （非適合）。`safefileio` は symlink を解決せず拒否する設計のため zoning には流用できない。**必須テスト**:
      `cp evil $WORKDIR/link`（`link→/etc/passwd`）は **High**（ターゲット解決）であり Low にならないこと。
  (b) safe-zone の起点は **`RuntimeCommand.EffectiveWorkDir` と構成済み専用 temp** に限定し、曖昧な `$HOME`・共有
      `/tmp`・出力先の親ディレクトリは**含めない**。
  (c) safe-zone が trust-critical と重複/配下のときは safe-zone として扱わず trust-critical（High）を優先する。
  (d) **TOCTOU 耐性（per-operand Trusted）**: Low 降格は、解決後の各オペランドパスが**信頼ディレクトリ許可リスト
      配下**にあり、かつ**経路要素が委譲先（run-as）から書込不可**（run-as 以外所有・group/other 非書込）である
      ときに限る。満たせなければ Low に降格しない（fail-closed）。参照 identity は live euid でなく config の run-as
      値（AC-21）。leaf が既存 symlink の場合は最終ターゲットで zoning する。（0140 AC-17）
- **AC-05**（fail-closed 既定, 安全要件・根本原因1の本体）: 宛先オペランドを確実に解決できない/曖昧な形（未確定の
  変数展開、未知・曖昧フラグで書込先を一意特定できない、解決コスト上限超過 等）は `ZoneUnresolved` とし、**Low に
  しない**。floor は **Kind 依存**——書込/削除系は **High**、読取主体（cp source・`dd if=` 等）は **Medium**。
  「不明フラグ＝安全」とは仮定しない。**読取主体の未解決を High でなく Medium とするのは意図的な非対称**（書込/削除の
  最悪は破壊、読取の最悪は情報露出という脅威差。02 で根拠を保持し「うっかり緩和」を防ぐ）。（0140 AC-18 を
  Kind 依存 High まで強化）

### F-002: 作用オペランドの抽出と網羅テスト（根本原因1）

- **AC-06**（オペランド抽出の網羅性は仕様表＋テストで担保）: 各ロケーション定義コマンドの**作用オペランド**
  （宛先/source/FILE/`if=`/`of=`/mountpoint/展開先 等）を抽出して zoning する。対象は `cp`・`mv`・`rm`・`rmdir`・
  `unlink`・`shred`・`ln`・`mkdir`・`touch`・`install`・`tee`・`sponge`・`truncate`・`sed -i`・`tar`・`unzip`・`dd`・
  `mount`・`umount`・`chmod`・`chown`・`chgrp`・`setfacl`・`chattr`・`mknod`・`find`（破壊/書込アクション）・データ
  送信の書込形（F-004）。**個別フラグ/形は要件本文で列挙せず、コマンド→Kind→オペランド抽出規則を単一の仕様
  （実装内テーブル）で表し、既知コマンド×代表フラグの表駆動（プロパティ/網羅）テストで被覆を担保**する。
  少なくとも次の難所は仕様表のエントリとしてテスト行を持つ: in-place 編集（`truncate`/`sed -i`）、`ln -s` の相対
  target（リンク親基点で解決）、アーカイブの抽出 vs 一覧（`tar -x`/`unzip` は展開先を zoning、`tar -t`/`unzip -l` は
  非昇格、`tar --one-top-level=DIR` を抽出先として扱う、`-C`/`-d` 省略時は `EffectiveWorkDir`）、末尾 `/` 付き削除の
  symlink dereference、world-write/所有権付与（軸 A・F-003）、`dd` の `if=`/`of=` デバイスオペランド、データ送信の
  書込先抽出（`curl -o`/`-O`、`wget` 既定/`-O`/`-P <dir>`、`scp host:/x <DEST>`、`sftp` バッチ書込、`rsync … <DEST>`/
  `--delete` の対象）。
- **AC-06a**（仕様表と AC の連動）: **AC-08〜AC-16 で High/Medium 化と名指しされた全ての書込/削除/付与形は、
  オペランド抽出仕様表に対応するテスト行を持つ**こと（散文 AC と仕様表のドリフト防止＝根本原因1）。未知/曖昧形は
  fail-closed（AC-05）。
- **AC-07**（複数オペランド）: 1 コマンドの作用オペランドが複数のときは各々を zoning し **max** を取る。（0140 AC-31 の一部）

### F-003: 軸 A（ゾーン非依存の floor）

- **AC-08**（権限/所有権/属性の付与）: setuid/setgid 付与・world-write 等の権限拡大・trust-critical 対象への所有権
  変更・完全性制御除去（`chattr -i`）は、**宛先ゾーンに依らず High**（`chmod u+s`・`chmod 0777`・`chown root
  /usr/bin/x`・`chattr -i /etc/shadow` 等）。（0140 AC-20）
- **AC-09**（`install` の権限フラグ）: `install` が `-m` に setuid/setgid を伴う、または `-o`/`-g` で所有者/グループを
  変更する場合は **High**（safe-zone でも Low に降格しない）。（0140 AC-22a）
- **AC-10**（`dd` のデバイス入出力）: `if=`/`of=` が**ブロックデバイスまたは危険キャラクタデバイス**（`/dev/mem`・
  `/dev/kmem`・`/dev/port` 等の物理/カーネルメモリ生アクセス）のとき **High**。無害シンク（`/dev/null`・`/dev/zero`）
  は除外。機微/trust-critical な `if=` source の複製は safe-zone 宛先でも **Medium**（情報露出 floor）。判定はパス
  文字列でなく**デバイス種別**で行う。（0140 AC-21）
- **AC-11**（ツリー再帰の昇格）: 再帰操作（`rm -r`/`-R`・`cp -R`/`-a` 等）が**作用対象を safe-zone の外（ordinary/
  trust-critical）に及ぼす**場合は **High**。**信頼された safe-zone 内に閉じた再帰**（`rm -rf $WORKDIR/build`）は
  Low のまま。複数オペランド指定自体は昇格条件にせず各々 zoning する。（0140 AC-22）
- **AC-12**（作用する全オペランドの zoning）: 破壊/移動の全オペランドを zoning する。`mv` は宛先＋source 双方
  （trust-critical source なら High）、`rm`/`shred`/`unlink` は全削除対象、`ln` は宛先＋source/リンク先双方、`cp` は
  宛先で判定するが**機微/trust-critical source の複製**は safe-zone でも Low に降格しない（Medium 下限）、`-p`/`-a`
  での特権メタデータ複製（setuid/root 所有 source）は High。（0140 AC-22b）
- **AC-13**（`mount`/`umount`）: 対象が trust-critical のとき **High**——`mount` は mountpoint と source 双方
  （`--bind`/`--rbind`/`--move` の trust-critical source、デバイス source を含む）、`umount` は対象 FS/ディレクトリ。
  `umount -a` は無条件 **High**。それ以外は Medium。（0140 AC-19）
- **AC-14**（`tee`/`sponge` の書込）: 非フラグ FILE 引数を書込先として zoning し、複数 FILE は各々 zoning して max
  （いずれかが trust-critical なら High）。`tee` は内側コマンドを実行しない。（0140 AC-22d）
- **AC-15**（`find` の破壊/書込アクション）: `find -delete`（ツリー破壊）・`-fprint*`（FILE 書込）を伴う場合、探索
  起点（省略時 `EffectiveWorkDir`）/書込先 FILE を zoning する（trust-critical 起点なら High、信頼 safe-zone 起点なら
  Low）。読取専用検索は昇格しない。`-exec`/`-execdir`/`-ok`/`-okdir` の**内側コマンド実行は間接実行 Reject**（0141/
  既存の所掌で、本タスクの zoning 対象外）。（0140 AC-22e のうち zoning 部分）

### F-004: データ送信のローカル書込 High 化と max 合成

- **AC-16**（ローカル trust-critical 書込）: データ送信系の**ファイル書込/削除形**がローカルの trust-critical パスへ
  作用する場合、④信頼境界破壊として **High**（`curl -o /usr/bin/x`・`curl -O <url>`＝URL 由来名を cwd へ・`wget -O
  /etc/cron.d/x`・`wget` 既定・`wget -P <dir>`・`scp host:/x /usr/bin/x`・`sftp` バッチ書込・`rsync … <DEST>`／
  `--delete`）。最終リスクは **`max(データ送信の名前→Medium〔0141〕, 書込先ゾーン)`**。**同一コマンドが 0141 と
  0142 の両方で評価されるため、この max 合成の所有者・テストは 0142**。書込先抽出は F-002 の仕様表に含める。
  **合成の必須テスト（両寄与が同時に生きていることを検証）**: (i) safe-zone 宛先への書込（`curl <url> -o $WORKDIR/safe`）は
  **Medium**（書込先ゾーンが Low でも 0141 の名前 floor が効く）、(ii) trust-critical 宛先（`curl -o /usr/bin/x`）は
  **High**（書込先ゾーンが名前 floor を上回る）。**前提**: 本テストは 0141 の名前→Medium floor が評価器に結線済み
  であること（共有境界）を要し、未結線では (i) が phantom floor で誤って通るため、結線後に意味を持つ。（0140 AC-25 の書込先部分）

### F-005: 単一権威ゾーン経路による旧 High 源の置換（根本原因2）

- **AC-17**（軸2 が旧 High 源を置換）: ロケーション定義 applet については、軸2 の結果を**唯一の権威**とする。
  当該 applet 向けの**旧 High 源 5 系統**——①`IsDestructiveFileOperation`、②`CoreutilsCommandRisk` の破壊系 High、
  ③profile `DestructionRisk`、④`dangerousCommandPatterns`(rank6) の `rm -rf`/`dd if=` 等 applet エントリ、⑤coreutils
  の setuid/setgid lstat floor——を**評価対象から外し**、`LocationResult` を唯一の寄与とする。
  **抑止は「完全認識（complete positive recognition）」のときのみ**: (a) 抽出された**全オペランド**が非 `Unknown` の
  確定ゾーンを返し、かつ (b) オペランド抽出器が**argv を完全消費**した（非フラグの未消費トークンが無く、パスを運び
  得る未知の値取りフラグが無い）こと。**部分的/不確実なパース（一部オペランド未認識・未消費トークン残存・未知の
  値取りフラグ）は `ZoneUnresolved` とし、①〜⑤の High を温存**する（「一部オペランドを認識した」だけで①〜⑤を
  落とすと、未認識の危険形が benign ゾーン→net Low になる fail-open。AC-05/AC-07 の「全オペランド max・未解決→
  High」が抑止後も floor として残る）。⑤の setuid floor は再パースせず既存 lstat シグナルを軸 A が流用する。非
  ロケーション applet（`find -exec`・未知 applet 等）は従来どおり旧源/間接実行が担う（同名でも非ロケーション用途では
  ④を無効化しない）。
  - **観測可能プロパティ（テスト対象）**: 信頼 safe-zone の `rm -rf $WORKDIR/build` は **Low**（④ rank6 や②coreutils の
    固定 High で打ち消されない）。ordinary の `rm /srv/app/cache.dat` は **Medium**。未知フラグで宛先不確実な `rm` は
    **High**（①〜⑤温存）。（0140 AC-22c を単一権威経路へ訂正）
- **AC-18**（max 合成）: 最終リスクは適用 dimension の **max**。軸1（名前固定）と軸2（宛先ゾーン）の双方が適用される
  コマンドはその最大値（例 `cp -a … /usr/bin`＝High）。順序非依存。（0140 AC-31）

### F-006: 結線・DTO・identity 注入（根本原因4）

- **AC-19**（per-operand 監査 DTO の配置と内容検証）: per-operand 判定記録 DTO（`OperandZone`/`PathZone` 相当: Index/
  Raw/Resolved/Zone/MatchedCritical/Trusted/UnresolvedErr）を **`risktypes` に定義**し、`RiskAssessment` に格納する
  （`security → risktypes` の一方向依存を維持。`security` に置くと循環）。**logger への JSON 出力は 0143**。本タスクは
  `RiskAssessment` への格納までを担保するが、**presence だけでなく格納値の正しさを検証する**: 代表コマンド（例
  `cp evil /usr/bin/ls`・symlink 経由・複数オペランド）について、`RiskAssessment` から直接 `[]OperandZone` を読み、
  各要素の Index/Raw/Resolved/Zone/Trusted が期待どおりであることをテストする（値が誤っても 0143 まで気付けない
  穴を塞ぐ）。（新規。0140/00 §3.4）
- **AC-20**（`security.Config` の結線）: deployment の `Config.SystemCriticalPaths` と**信頼ディレクトリ許可リスト**を
  評価器へ通す。`NewStandardEvaluator`・`runner.go` のコンストラクタ結線、および信頼ディレクトリの **TOML
  `[security]` spec＋ローダ＋runner 転送**を本タスクで追加する（無ければ configured 環境で AC-01/AC-04 が成立せず、
  テスト注入でしか通らない）。（新規。0140/00 §3.4）
- **AC-21**（identity 注入の純粋性）: run-as 名→UID/GID/補助 group の解決は**zoning の外（評価器結線層）**で行い、
  precomputed `RunAsIdent` を `ZoningInput` に注入する。**zoning は live identity（`os.Geteuid`/`os.Getuid`/
  `syscall`/`unix` の uid/gid/groups・`user.Current`）を読まない**。検証は**差分テストを主**とする: 注入 `RunAsIdent` を
  **テストプロセスの実 euid/gid と異なる値**に設定し、Trusted/Low 判定が**注入した identity に従って変わる**こと
  （プロセス identity ではない）を表明する（決定性テストだけでは「単一プロセス内で euid が一定なら `os.Geteuid()` を
  読んでも決定的」になり live 参照の不在を証明できないため、差分テストで担保する）。**補助**として `os/user`・
  `os.Geteuid`/`os.Getuid`/`syscall`/`unix` の uid/gid/groups を軸2 plumbing が呼ばない grep ガードを置く。
  （新規。0140/00 §3.4）

### F-007: 決定性・read-only

- **AC-22**（runtime==dry-run・read-only）: 軸2 のパス解決は `lstat`/`readlink` のみの **read-only** で、runtime と
  dry-run で同一レベルを返す。結果は live euid・`$HOME` env に依存しない（AC-21）。（0140 AC-28／NF-003）
- **AC-23**（解決コストの上限・fail-closed）: 解決は**評価単位でメモ化**（同一親の再解決をしない）し、**1 コマンド
  評価あたりのオペランド総数（>N）または symlink 追従ホップ総数（>M）が上限を超えたら `ZoneUnresolved`→High**
  （書込/削除）に倒す（具体値 N/M は 02/実装で確定）。**必須テスト**: 上限を超える入力（多数オペランド・深い
  symlink チェーン）で fail-closed（High）になり、メモ化により seam 呼出回数が線形に収まることを表明する
  （ExecuteCommand ホットパスでの無制限 FS I/O・DoS を防ぐ）。（新規／0140/02 §3.5）

## 5. 非機能要件

- **NF-001**: 本タスクが追加した `ReasonCode`（例: 信頼境界書込・権限付与・ゾーン由来）は、**本タスク内で**網羅性/
  一意性テストを緑に保つ（family 区別の最終化は 0143）。（0140 NF-001）
- **NF-002**: `make test`・`make lint`・`make fmt` が成功する。完了基準は統合パッケージ（`internal/runner`・
  `internal/runner/config`）をコンパイルする範囲とする。（0140 NF-002）
- **NF-003**: 判定は決定的で副作用がなく、safe-zone 判定のパス解決は読取のみ（AC-22）。（0140 NF-003）

## 6. スコープ外の根拠

- **名前固定階級・ラッパ/特権は 0141**: 名前で決まる固定レベル、Critical 尖鋭化、env/timeout、間接実行
  （`find -exec`/ProxyCommand/`rsync -e`）は argv の宛先解析を伴わず 0141 の所掌（D5 の線引き）。
- **logger 出力・文書・config 追従は 0143**: 監査フィールドの実際の JSON 出力、変更ノート、ユーザー/開発者文書、
  sample config の `risk_level` 追従は横断成果物として 0143 に集約する。
- **段階ロールアウト/フラグは無し**: 後方互換不要のため（0140/00 §3.2）。
- **`RiskLevel` 段数/新レベル**: 変更しない（0140 §6 を継承）。
- **完全な情報漏えい（read）モデル**: 機微 source の floor は導入するが、完全な read 系分類は将来課題
  （0140/02 §9 を継承）。
