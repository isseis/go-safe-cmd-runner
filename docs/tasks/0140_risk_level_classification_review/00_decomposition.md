# 0140 リスクレベル分類の一貫化 — 分割・再構成ノート（根本原因への対処）

## ステータス

| Item | Value |
|---|---|
| Status | `draft`（再構成方針。承認後に 0141/0142/0143 を生成） |
| Created | 2026-06-19 |
| Decision | (A) アーキ訂正＋(C) スコープ3分割（PR #765 のレビューを受けた判断） |

> 本書は 0140 を **3 つの後続タスク 0141/0142/0143 へ分割**し、PR #765 のレビューで露見した**根本原因**
> （アーキ起因の脆さ）を是正するための再構成ノートである。0140 の [01_requirements.md](01_requirements.md)・
> [02_architecture.md](02_architecture.md) は**履歴として残し（superseded）**、確定要件・確定アーキは各後続
> タスクへ引き継ぐ。

---

## 1. 背景：なぜ分割・訂正するか（根本原因の要約）

PR #765（0140 の実装計画）に対し自動レビューで 4 ラウンド計 41 件の指摘が付き、いずれも妥当だが
**少数のテーマへ収斂**し、「直すと隣接ギャップが出る」状態だった。根本原因：

- **根本原因1（argv 解析の対象が膨大で列挙しきれない）**: 判断軸2 は本質的に、多数の CLI ツールの**コマンドライン
  引数（argv）を解析する**セキュリティ重要パーサである。解析すべき「コマンド×フラグ×形式」の組み合わせは膨大・
  非有界（distro 差・新フラグ・エッジケース）で、計画が個別フラグ/形を散文で列挙したため、レビューは必ず次の漏れ
  （`--one-top-level`/`scp`/world-write/末尾`/`…）を出せた。
- **根本原因2（max 抑止の脆さ＝アーキ §3.8 起因）**: 最終リスクは全判定の **max** で決まるため、D7 の「引き下げ」を
  成立させるには、対象コマンドを High に分類している**既存の判定をすべて無力化**せねばならない。コードには
  これらを High にする**独立した判定経路が最低 5 つ**（`IsDestructiveFileOperation`／`CoreutilsCommandRisk`／profile
  `DestructionRisk`／`dangerousCommandPatterns` rank6／setuid 下限）あり、経路ごとに抑止/非抑止の規則が異なる
  （whack-a-mole）。
- **根本原因3（shadow ロールアウトが過剰スコープ＝アーキ §6.4 起因）**: 元計画は raise/lower を段階導入する
  shadow/audit-only ＋デプロイ可能フラグを全フェーズに織り込んでいた。だが**本プロジェクトは後方互換性を要求
  しない**ため、フラグでデプロイをガードする必要はなく、新分類は直接適用でよい（§3.2）。フラグ機構・shadow・
  両モードテスト・依存逆転は不要スコープとして除去する。
- **根本原因4（端から端への結線/配送の欠落）**: 「何を計算するか」は書くが、constructor/TOML loader/audit logger/
  DTO 配置/パッケージ依存（`risktypes↔security` 循環）/build 範囲を繰り返し省いた。

詳細な指摘分類は本 PR のレビュースレッド参照。

---

## 2. 分割マップ（0141 / 0142 / 0143）

| 新タスク | スコープ | 主担当の根本原因訂正 | 依存 |
|---|---|---|---|
| **0141** `axis1_fixed_class_risk` | 判断軸1＝コマンド名分類の引き上げ・Critical 限定・実行ラッパ/特権昇格拡張・env/timeout High。**argv の宛先解析を伴わない**名前/ラッパ判定に限定。 | 根本原因4（evaluator/wrapper 結線をフェーズ内に明示） | なし |
| **0142** `axis2_destination_zoning` | 判断軸2＝ファイル操作コマンドの宛先ゾーン分類（trust-critical/ordinary/safe-zone/unresolved）。path 解決・safe-zone・オペランド毎のTrusted・argv オペランド抽出。 | **根本原因1・2・4**（下記 §3） | 0141（**共有境界**: evaluator dispatch / 名前集合。直列） |
| **0143** `risk_audit_and_docs` | オペランド毎の監査フィールド・理由コード網羅・変更ノート（changelog）・文書整合・sample config・ガイド最終化。**段階ロールアウト/フラグは設けない（後方互換不要）**。 | 監査・文書（横断成果物の集約） | 0141・0142 |

**実装順序**: 0141（判断軸1）→ 0142（判断軸2）→ 0143（監査・文書）。
- **共有境界**: `risk/evaluator.go` の `evaluateDimensions`（判断軸のディスパッチ）と `security/command_analysis.go` の名前集合は 0141 が
  再編し、0142 はその上に**直列**で構築する（同一ファイルで衝突するため「並行可」とは言わない）。01/02 の**文書
  作成**は並行できるが、**実装は 0141 の共有境界確定後に 0142**。
- 新分類は各フェーズで**直接適用**する（後方互換不要のため、フラグ・旧経路保持・enforce 切替フェーズは無し）。

---

## 3. アーキテクチャ訂正（(A)）— 各後続タスクの 02 に反映する確定方針

### 3.1 根本原因2 の訂正（0142/02）: 「既存判定の選択的 max 抑止」をやめ「判断軸2 が唯一の判定基準として置き換える」

> **用語**: 「既存の High 判定（経路）」とは、これらのコマンドを現在 High に分類しているコード上の判定を指す
> （最終リスクは全判定の **max** なので、引き下げにはこれらすべてを無力化する必要がある）。「コマンド」は
> rm/cp/dd 等の個々の実行コマンドを指す（旧表記の "applet" を改称）。

ファイル操作コマンドについては、**判断軸2 のゾーン結果を唯一の判定基準**とし、これらを High に分類している
**既存の判定（5 系統）を選択的に max 抑止するのではなく、評価器のディスパッチで置き換える**。すなわち:

- **置き換え対象＝当該コマンドを High にしている既存判定を「全数」列挙する**（5 系統。§1 と一致）:
  ①`IsDestructiveFileOperation`、②`CoreutilsCommandRisk` の破壊系 High、③profile `DestructionRisk`、
  ④**`dangerousCommandPatterns`(rank6) の `{"rm","-rf"}`/`{"dd","if="}` 等のコマンドエントリ**（コードで存在確認済み。
  これを落とすと safe-zone `rm -rf $WORKDIR/build` が rank6 で High に残り AC-16/D7 と矛盾する＝引き下げのバイパス）、
  ⑤coreutils の **setuid/setgid lstat 下限**。ファイル操作コマンドと判定されたら、`evaluateDimensions` はこれら
  **全 5 系統を評価対象から外し**、判断軸2 の `LocationResult` を**唯一の寄与**とする。
- **置き換えは「完全認識（positive recognition）」に限定する（fail-open 回避）**: `applies=true` だけでなく、判断軸2 が
  **非 `Unknown` の確定ゾーンを返し、かつ Kind 別オペランド抽出表が当該危険オペランドを実際に消費した**ときに限る。
  コマンド名の一致だけで①〜⑤を外すと、判断軸2 が認識できない危険な形が 低リスクゾーンと誤判定され Low の fail-open になる。
- 例外（判断軸2 がカバーしない破壊）は判断軸2 自身が下限として内包する: **setuid/setgid 付与（判断軸 A）**・
  **ブロック/危険キャラクタデバイス**・**再帰がゾーン外**・**`rsync --delete` 等「引数による破壊」**は
  `LocationResult` 内の下限として High を返す。**⑤の setuid 下限は再パースせず、既存の lstat シグナル
  （`hasSetuidOrSetgidBit` 相当）を判断軸 A がそのまま流用**する（新規 argv パースに置換すると lstat より弱くなり
  setuid バイナリの High→Low 退行を招くため）。
- `ZoneUnresolved`（解決不能）または上記の完全認識が不成立のときは、安全網として**既存判定の High を残す**
  （唯一の fail-open 箇所をここに限定）。

これにより「判定経路が 5 つあって 1 つ漏れる」構造を解消し、**1 経路 = 1 箇所**にする。ファイル操作コマンド以外の
コマンド（`find -exec` 等の引数破壊、判断軸2 が扱わない未知のコマンドの fail-safe）は従来どおり既存判定/間接実行が担う。

### 3.2 根本原因3 の解消: ロールアウトフラグ／shadow を設けない（後方互換不要）

- **本プロジェクトは後方互換性を要求しない**ため、新旧を並走させる shadow/audit-only モードも、デプロイを
  ガードするフラグ（`RolloutMode` 等）も**設けない**。各タスクは新分類を**直接 enforce へ適用（in-place 置換）**する。
- これにより、元計画の「フラグ機構・両モードテスト・旧経路保持・enforce 切替フェーズ・依存逆転」は
  すべて不要スコープとして除去される（PR #765 レビューのうち shadow/フラグ関連の指摘は moot になる）。
- 破壊的変更（raise/lower）は **0143 の変更ノート（changelog）で周知**するに留め、実行時の運用ガードは設けない。
  既存 sample/test config の追従（必要な `risk_level` 付与）は 0143 が担う。

### 3.3 根本原因1 の訂正（0142/01・0142/02）: argv 列挙を「実装成果物＋網羅テスト」へ委譲

- 計画/要件で個別フラグを散文列挙して安全性を主張しない。代わりに:
  1. **fail-closed 既定を要件化**: 未知/曖昧フラグ・特定不能な書込先 → `ZoneUnresolved` → Kind 依存の下限
     （書込/削除=High）。これが安全網の本体。
  2. **`LocationKind` 別オペランド抽出仕様表**を実装成果物（コード内の単一テーブル）とし、
     **プロパティ/網羅テスト**（既知コマンド×代表フラグの表駆動）で被覆を担保する。レビューで挙がった
     個別ケース（`--one-top-level`/`scp`・`sftp`/world-write/末尾`/`/`truncate`/`sed -i`/`ln -s` 相対 等）は
     **この表のエントリ**として扱い、要件本文では「fail-closed＋仕様表＋網羅テスト」を求めるに留める。

### 3.4 根本原因4 の訂正（各 02 に「結線/配送」章）: 端から端の結線を明示

- 各タスクの 02 に**データフロー結線章**を設け、TOML→loader→runner→evaluator→（判断軸2 では）ゾーン分類→
  `RiskAssessment`→audit logger の経路と、各新型の**配置パッケージ**を明記する。
- **DTO 配置（import cycle 回避）**: オペランド毎の監査 DTO（`OperandZone`/`PathZone` 相当）は**`risktypes`
  に定義**する。根拠（依存方向はコードで確認済み: `security → risktypes → runnertypes` の一方向）: 監査キャリアは
  `risktypes.RiskAssessment` に埋め込むため、**そこに埋め込む型は `risktypes` から import 可能でなければならない**
  （`risktypes` の依存は `runnertypes` のみ）。DTO を `security` に置いて `risktypes.RiskAssessment` から参照すると
  `risktypes → security` の逆向き依存が生じ、既存の `security → risktypes` と合わせて循環する。よって DTO は
  `risktypes`（または `runnertypes`）へ置く。*別案*: DTO を `RiskAssessment` に埋め込まず `security` 所有のまま
  `evaluator.go`（既に `security` を import）へ渡し、監査へは射影のみ渡す形もあるが、本設計は監査キャリアを
  `RiskAssessment` に載せる（allow/deny いずれの経路でも logger へ伝播）ため `risktypes` 配置を採る。0142/02 で確定。
- **identity 注入（非決定性）**: run-as 名→UID/GID/補助 group の解決は**ゾーン分類の外（評価器結線層）**で行い、
  precomputed identity を注入する。ゾーン分類は live identity を読まない。これを**grep ガードでなく、注入入力の
  純関数であることを検証するテスト**で担保する（grep はガード範囲/API 変種で破れるため補助に留める）。
- **build/完了ゲート**: 各フェーズの完了基準は、そのフェーズが触る統合パッケージ（`internal/runner`/
  `internal/runner/config` 等）も**コンパイルする範囲**（`./internal/runner/...` または `make test`）にする。

---

## 4. 受け入れ基準（AC）の配分

0140/01 の AC を後続タスクへ配分する（各タスクで AC は振り直し。下表で原 AC との対応を保持）。詳細な
AC 本文・根拠は 0140/[01_requirements.md](01_requirements.md) を正とし、各タスク 01 はスコープ分の AC を
再掲・精緻化する。

| 0140 AC | 内容 | 配分先 |
|---|---|---|
| AC-01〜AC-03 | 大規模破壊/デバイス初期化 High | **0141** |
| AC-04〜AC-10a | kernel/auth/boot/service/power/FW/setcap/trust-intrinsic/scheduler High | **0141** |
| AC-11〜AC-13 | LVM 作成/ip/mount = Medium | **0141** |
| AC-23, AC-24 | Critical=特権昇格ラッパ限定 / High≠Critical | **0141** |
| AC-25（名前下限） | データ送信コマンドの**名前→Medium 下限**（名前集合エントリのみ）＋ヘルパー実行（ssh ProxyCommand/rsync -e）＝間接 | **0141** |
| AC-25（書込先＋合成） | ローカル trust-critical 書込（curl -o/-O・wget・scp/sftp dest・rsync DEST/--delete）→High、**および `max(名前 Medium, 書込先 High)` の合成・テスト**（同一コマンドが両タスクのコードで評価されるため、**合成の所有者は 0142**） | **0142** |
| AC-26 | 検出限界（名前ベース AI vs データ送信）の文書化 | **0141** |
| AC-27 | fdisk/mkfs/parted/fsck=High 訂正 | **0141** |
| AC-29, AC-29a | chroot/unshare/nsenter/flock/watch・env/timeout High | **0141** |
| AC-14〜AC-22e | ファイル操作コマンドの 3 ゾーン判定（判断軸2 全体） | **0142** |
| AC-28 | runtime==dry-run | **全タスク横断 NF**（各タスクが自分の変更で担保）。**パス解決/identity の決定性サブケースは 0142 が主担当** |
| AC-31 | max 合成（判断軸1×判断軸2） | **0142** |
| AC-30 | deny 時の理由コード記録（監査） | **0143** |
| AC-32, AC-33 | 移行ノート（引き上げ/引き下げ） | **0143** |
| AC-34, AC-35, AC-36 | 文書整合・sample config・ガイド最終化 | **0143** |
| NF-001 | 理由コード網羅性 | **各タスク**（自タスクが追加した `ReasonCode` の網羅性テストを**そのタスク内で緑に保つ**）。0143 は監査ストリームの family 区別の**最終化**のみ所有 |
| NF-002 | make test/lint/fmt 緑 | 全タスク共通 |
| NF-003 | 決定的・副作用なし・read-only | **0142**（主）／全タスク |

> 新規 AC（0140 に無いが本訂正で要件化）:
> - 0141: 新規 AC なし（ロールアウトフラグは廃止。後方互換不要）。
> - 0142: 「fail-closed パース既定」「`LocationKind` 別オペランド抽出仕様表＋網羅テスト」「判断軸2 が既存の High 判定
>   5 系統を**完全認識のときのみ**置き換える（単一権威）」「オペランド毎のDTO は `risktypes` 配置」「identity 注入の
>   純関数性」を AC 化。
> - 0143: 「オペランド毎の監査フィールドの logger 出力」「変更ノート（changelog）/文書/ガイド」を AC 化
>   （shadow/フラグは無し）。

---

## 5. 0140 と PR #765 の扱い

- 0140/[01_requirements.md](01_requirements.md)・[02_architecture.md](02_architecture.md) は**履歴として保持**し、
  冒頭に superseded バナーを付す（内容・承認状態は改変しない）。確定要件/アーキは 0141/0142/0143 へ継承。
- 0140/03_implementation_plan.md（PR #765）は本再構成により**置き換える**ため、PR #765 を**クローズ**する
  （新タスクで出し直す）。

---

## 6. 次の手順（gated process 厳守）

1. 本 `00_decomposition.md`（分割方針）のレビュー・合意。
2. 0141 → 0142 → 0143 の順に `01_requirements.md`（draft）を作成 → 人手承認。
3. 各タスク 01 承認後に `02_architecture.md`（draft、§3 のアーキ訂正を反映）→ 承認。
4. 各タスク 02 承認後に `03_implementation_plan.md`（draft）→ 承認 → 実装。新分類は各フェーズで直接適用する
   （フラグ・旧経路保持・切替/撤去フェーズは無し）。破壊的変更の周知（変更ノート）・sample config 追従は 0143。
