# 判断軸2: 宛先パス信頼区分の一貫化 — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-23 |
| Review date | 2026-06-23 |
| Reviewer | isseis |
| Comments | - |

> 本書は 0140 を 3 分割した第 2 タスク（判断軸2＝宛先パス信頼区分）の実装計画である。要件は
> [01_requirements.md](01_requirements.md)、設計は [02_architecture.md](02_architecture.md) を正とする。
> 本書は設計図を再掲せず、各タスクから設計の該当節（§3.x 等）を参照する。本タスクは 0141 が再編した共有コード
> （`evaluateDimensions`・名前集合）の**完了後**にその上へ構築する。

---

## 1. 実装概要

### 1.1 目的

ファイル操作コマンド（`cp`/`rm`/`dd`/`tar`/`mount`/`chmod` 等）の作用先パスを解決後パスへ正規化し、パス信頼区分
（trust-critical/ordinary/safe-zone/unresolved）へ分類してリスクを判定する判断軸2 を、既存のリスク評価パイプライン
（`StandardEvaluator.evaluateDimensions`）へ組み込む。完全認識時には、これらコマンドを現在 High に分類している既存
5 系統の判定を判断軸2 の結果で置き換える（設計 §3.4）。

### 1.2 実装方針

- **設計に従う**: 型・関数・判定規則・脅威モデルはすべて [02_architecture.md](02_architecture.md) を正とし、本書は
  作業の分解・順序・検証方法のみを定義する。
- **fail-closed 既定**: 解決/抽出が不確実なオペランドは `ZoneUnresolved`（書込/削除=High・読み取り元=Medium）に倒す
  （設計 §3.1・§5.1）。実装で迷う形は安全側へ。
- **純関数・決定的**: パス解決は `lstat`/`readlink` のみの read-only。判定は注入 `RunAsIdent`・注入パス集合と評価時点の
  ファイルシステム状態のみに依存し、live identity（`os.Geteuid` 等）・`$HOME` env を読まない（設計 §3.6・§5.3）。
- **English ソース**: 追加する Go の識別子・コメント・文字列リテラルはすべて英語（テストソースを含む）。本計画書の
  地の文は日本語。
- **段階ロールアウトなし**: 後方互換不要のため新分類を直接適用する。フラグ/shadow は設けない（設計 付録）。
- **完了基準**: 各 Phase 完了時に `make fmt`→`make test`→`make lint` を緑にする。中核 2 パッケージ（`security`/`risk`）に
  加え、本タスクが変更する統合パッケージ（`internal/runner`・`internal/runner/config`）まで、コンパイルとテストが通ることを
  完了基準とする。具体的には `./internal/runner/...` または `make test` 全体が緑であること（NF-002）。

### 1.3 既存コード調査結果

実装着手前に、設計が参照する全シンボルの存在・シグネチャを確認した（`internal/runner/` 配下）。要点:

**risktypes（`internal/runner/base/risktypes/`）**
- `RiskAssessment`（`types.go:112-134`）: 既存フィールドは `Level`/`Blocking`/`BlockingReason`/`ErrorClass`/
  `ReasonCodes`/`Reasons`/`NetworkType`。本タスクは `OperandZones []OperandZone` を**追加**する（既存は不変）。
- `ReasonCode`（`reason_codes.go:9`、`type ReasonCode string`）: 既存 29 定数。本タスクは新 family 7 種を追加する。
- `TestReasonCodes_AllDistinct`（`reason_codes_test.go:12-52`）: **ハードコードされた全定数スライス**（`reason_codes_test.go:13`）を
  走査し、空値なし・重複なしを検証する。新コード追加時はこのスライスへの登録も必要（登録漏れでテストが緑のまま穴になる）。
- import は `runnertypes`＋stdlib のみ。`security → risktypes → runnertypes` の一方向依存が成立（DTO を `risktypes` に
  置く設計 §3.1 の前提を確認済み）。

**security（`internal/runner/base/security/`）**
- `Config`（`types.go:109-151`）: `SystemCriticalPaths`（`:137`）・`OutputCriticalPathPatterns`（`:141`）・
  `TrustedGIDs`（`:148`）を保持。本タスクは `TrustedDirectories []string` を**追加**する。
- `(*Config).GetSystemCriticalPaths()`（`types.go:309-312`）・`DefaultConfig()`（`types.go:160-258`、既定の
  `SystemCriticalPaths`＝`:226-229`、`OutputCriticalPathPatterns`＝`:230-246`）存在。
- 置き換え対象 5 系統の所在:
  - ① `IsDestructiveFileOperation`（`command_analysis.go:531-562`）→ `evaluateDimensions` の `evaluator.go:229`。
  - ② `CoreutilsCommandRisk`（`coreutils.go:116-171`、破壊系で `RiskLevelHigh,true,nil`＝`:162-164`）→ `evaluator.go:218-224`。
    `coreutilsHandled` は binary 解析抑止（`evaluator.go:263`）にも使うため、破壊系 High の抑止と binary 解析抑止を分離して扱う。
  - ③ profile `DestructionRisk`（`command_risk_profile.go:37`、`rm`/`dd` のみが保持＝`command_analysis.go:41-46`）→
    `applyProfileFactors`（`evaluator.go:287`）/`ProfileFactorRisk`（`command_risk_profile.go:180`）。
  - ④ `dangerousCommandPatterns` rank6（`command_analysis.go:216-234`）: `{rm,-rf}` High・`{dd,if=}` High・
    `{chmod,777}` High・`{chown,root}` **Medium**（`:227`、設計 §3.4 表と一致）・`{sudo,rm}` High。`CheckDangerousArgPatterns` 経由で
    `evaluator.go:243`。ネットワーク系（`wget`/`curl`/`nc`）は同テーブルにあるが置き換え対象外。
  - ⑤ setuid/setgid lstat 下限: シグナルは `hasSetuidOrSetgidBit`（`command_analysis.go:802-828`、`os.Stat`＋
    `os.ModeSetuid|os.ModeSetgid`）。`CoreutilsCommandRisk` の setuid 経路（`coreutils_test.go:110` が表明）と連動。設計 §3.2 注のとおり
    **再パースせず既存シグナルを流用**する。
- リゾルバ流用元: `walkSymlinkChain`（`command_analysis.go:635`）・`ResolveCommandNames`（`command_analysis.go:727`）、
  `MaxSymlinkDepth=40`（`types.go:84`）。専用リゾルバはこの追従型を手本に新規実装する（`safefileio` は symlink 拒否設計のため流用不可、設計 §3.3）。
- ネットワーク終端検出: `hasNetworkArguments`/`HasNetworkArguments`（`network_analyzer.go:202`/`:212`）・
  `containsSSHStyleAddress`（`command_analysis.go:390-411`、`user@host:path`・`host:path` を `/`・`~` 含有で検出、二重コロン
  `host::module` は未検出）。P5 でこの隙間を塞ぐ（設計 §3.5）。
- `RuntimeCommand.EffectiveWorkDir`（`runnertypes/runtime.go:260-261`）存在。

**risk（`internal/runner/base/risk/`）**
- `StandardEvaluator`（`evaluator.go:30-34`）: `networkAnalyzer`・注入可能 `openIdentity`。
- `NewStandardEvaluator(networkAnalyzer *security.NetworkAnalyzer) Evaluator`（`evaluator.go:36-42`）: 本タスクで
  `*security.Config` 引数を**1 つ追加**する。
- `evaluateDimensions(cmd, names, profile, profileFound) (risktypes.RiskAssessment, error)`（`evaluator.go:204-274`）:
  順位 4-8 の max 合成。本タスクで判断軸2 ディスパッチを追加する。
- `resolveRiskEvaluator(opts, verificationManager) risk.Evaluator`（`runner.go:238-250`）: 現状 `securityConfig` を
  受け取らない。

**runner（`internal/runner/`）**
- `NewRunner`（`runner.go:285-381`）が `security.DefaultConfig()`（`:306`）を構築し `TrustedGIDs` を転送（`:308-309`）するが、
  `securityConfig` ローカルは `security.NewValidator` に渡るのみで、`createResourceManager`（`:196`）→
  `createDryRunResourceManager`（`:208`）/`createNormalResourceManager`（`:253`）→`resolveRiskEvaluator` の鎖には**未到達**。
  本タスクはこの鎖へ `securityConfig`（または信頼区分判定用 config）を通す（設計 §3.6）。
- `opts.riskEvaluator`（`runner.go:93`）・`WithRiskEvaluator`（`:139-147`）でテスト注入可能。

**runnertypes / config / privilege**
- `SecuritySpec`（`runnertypes/spec.go:111-117`、`TrustedGIDs []uint32 toml:"trusted_gids"`＝`:116`）に
  `TrustedDirectories []string toml:"trusted_directories"` を追加する。
- `config/loader.go` は go-toml/v2 で `toml.Unmarshal`（`:261`）する自動マッピング。新フィールドは構造体タグで自動デコード
  されるため、ローダ変更は最小（追加フィールドの転送経路のみ確認）。
- `privilege/unix.go` は `user.Lookup`/`user.LookupGroup` を使用。run-as 解決はこれと同質の純解決を注入層に置く（設計 §3.6）。

**更新が必要な既存テスト**
- `risk/coreutils_consistency_test.go`: `TestConsistency_RmAllForms`（`:186-213`）・`TestConsistency_DestructiveAbsolutePath`
  （`:170-180`）・`TestCoreutilsRiskConsistency_Setuid`（`:138-163`）。
- `risk/evaluator_test.go`: `TestEvaluateRisk_AbsoluteRmRfHigh`（`:192`）等、`rm`/`dd` を一律 High と表明する破壊系ケース。
- `security/coreutils_test.go`: `CoreutilsCommandRisk` の挙動テスト（`:52` 破壊系・`:110` setuid）は**関数自体を変えないため不変**
  （置き換えは評価層のディスパッチに閉じる、設計 §3.4）。
- `security/network_analyzer_test.go`・`security/command_analysis_test.go`（`TestContainsSSHStyleAddress`＝`:21`、
  rsync ssh-style ＝`:234`）: P5 で `host::module` の回帰を追加。

---

## 2. 実装ステップ

各 Phase の完了時に `make fmt`→`make test`→`make lint` を緑にする。Phase 順序は設計 §8 の優先順位表に一致させる。

### Phase 1: DTO・型定義・reason code family（AC-05, AC-19, NF-001）

**変更ファイル**:
- 新規 `internal/runner/base/risktypes/operand_zone.go`
- 変更 `internal/runner/base/risktypes/types.go`
- 変更 `internal/runner/base/risktypes/reason_codes.go`
- 変更 `internal/runner/base/risktypes/reason_codes_test.go`
- 新規 `internal/runner/base/risktypes/operand_zone_test.go`

**作業内容**:
- [x] `operand_zone.go` に `PathTrustZone`（`ZoneTrustCritical`/`ZoneOrdinary`/`ZoneSafeZone`/`ZoneUnresolved`）を定義（設計 §3.1）。
- [x] `operand_zone.go` に `OperandRole`（`OperandRoleWrite`/`OperandRoleRead`）を定義（設計 §3.1）。
- [x] `operand_zone.go` に `OperandZone` 構造体（`Index`/`Raw`/`Resolved`/`Zone`/`Role`/`MatchedCritical`/`Trusted`/`UnresolvedErr`）を定義（設計 §3.1）。
- [x] `operand_zone.go` に `RunAsIdent` 構造体（`UID`/`GID`/`Groups`）を定義（設計 §3.1）。
- [x] `types.go` の `RiskAssessment` に `OperandZones []OperandZone` フィールドを追加（既存フィールドは不変、設計 §3.1）。
- [x] `reason_codes.go` に新 family 7 定数を追加（設計 §4）: `ReasonTrustBoundaryWrite`=`"trust_boundary_write"`・
      `ReasonDestinationZone`=`"destination_zone"`・`ReasonPermissionGrant`=`"permission_grant"`・`ReasonDeviceIO`=`"device_io"`・
      `ReasonRecursiveOutsideSafeZone`=`"recursive_outside_safe_zone"`・`ReasonSensitiveSourceCopy`=`"sensitive_source_copy"`・
      `ReasonUnresolvedDestination`=`"unresolved_destination"`。
- [x] `reason_codes_test.go` の全定数スライス（`:13`）へ上記 7 定数を登録する（登録漏れ防止）。

**成功基準**:
- [x] `go build -tags test ./internal/runner/base/risktypes/` が通る。
- [x] `TestReasonCodes_AllDistinct` が新 7 定数を含めて緑（空値なし・重複なし、NF-001）。
- [x] `operand_zone_test.go` が `OperandZone` のゼロ値・各 enum 値の文字列表現を表明（型定義の健全性）。

### PR-1 作成ポイント: risktypes data model and reason codes

**対象ステップ**: P1

**推奨タイトル**: `feat(0142): add operand-zone DTO and destination reason codes`

**レビュー観点**: reason code の一意性/網羅性（NF-001・全定数スライス登録漏れ防止） / `OperandZone`・`RunAsIdent` 型定義の健全性 / 既存 `RiskAssessment` への非破壊追加

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/781）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: 専用リゾルバ＋Trusted 述語（AC-04, AC-23）

**変更ファイル**:
- 新規 `internal/runner/base/security/operand_path_resolver.go`
- 新規 `internal/runner/base/security/operand_path_resolver_test.go`
- 新規（必要時）`internal/runner/base/security/test_helpers_zoning.go`（`//go:build test`）

**作業内容**:
- [x] `ResolveOperandPath(operand, base string, maxHops int) (resolved string, err error)` を実装（設計 §3.3）。
      symlink チェーンを leaf＋親で追従し、正規化済み絶対パスを返す。相対オペランドは `base`（`ln -s` 相対 target は
      リンク親、他は `EffectiveWorkDir`）基点で解決。チェーン追従中に遭遇する中間の相対 symlink は、そのリンク自身が
      存在する親ディレクトリを基点に解決する（`base`/`EffectiveWorkDir` 基点ではない。パストラバーサル・区分誤判定の防止）。
      read-only（`lstat`/`readlink` のみ）。
- [x] 未存在 leaf を「最深の存在親」まで解決して末尾を畳み込む。**実装は経路要素を 1 つずつ追従する完全正規化方式**を採り、
      存在する symlink 要素は常に追従するため、末尾を畳み込む対象（最深の存在親）は常に解決済みの実ディレクトリになる。
      よって「最深存在親が symlink」という状態は構造的に発生せず、専用のエラー分岐は不要（fail-closed は次項の
      mid-chain 非 ENOENT 失敗のエラーで担保。設計 §3.3 注の意図＝未解決 symlink 下に宛先を置かないことを保証）。
- [x] cycle・深さ超過（`maxHops` 超）・mid-chain の `readlink`/`lstat` 失敗（ENOENT 以外）で `error` を返す（fail-closed、設計 §4）。
- [x] 解決コスト計測のため、リゾルバが `lstat`/`readlink` を**注入可能な関数セット**経由で呼ぶ構造にする
      （既定は実 os、テストは呼出回数を数えるスタブを注入。`StandardEvaluator.openIdentity` と同じ注入パターン）。注入集合は
      read-only の `lstat`/`readlink` のみとし、symlink を追従する `os.Stat` は含めない（設計 §3.3 の read-only 制約。`os.Stat` を
      混ぜると経路要素の所有権検査が symlink ターゲットを見てしまい区分判定を欺けるため）。
- [x] メモ化: 解決のメモ化を 1 回の判定呼び出し内にスコープし、鍵は**解決対象ノードの絶対パス（中間ノードを含む）**とする。
      relative なオペランド／relative な symlink target は基点ディレクトリと結合・正規化した絶対パスにしてから鍵にする（relative
      文字列そのものを鍵にすると、異なる基点下の同名 relative——例 `/dir1/link` と `/dir2/link` がともに `../target` を指す——で
      誤ヒットする）。鍵は当該ノードを **symlink 追従する前**の絶対パスであり、追従後の解決済み絶対パスではない（追従後を鍵に
      すると解決完了まで鍵が得られず循環する）。メモ化対象は存在する非 symlink ノード（実ディレクトリ/ファイル）のみとし、
      これにより共有する親チェーンの `lstat`/`readlink` 結果を 1 回に畳む（線形計数の前提）。identity 依存の Trusted 判定
      そのものはキャッシュしない（設計 §3.3(e) の鍵の記述を精緻化）。
- [x] Trusted 述語を実装（設計 §3.3(d)）: 解決後パスが `TrustedDirectories` 配下、かつ**safe-zone 起点の親以上**の経路
      要素が run-as から書込不可（run-as 所有は chmod で自己書込付与可能なため書込可とみなす・group/other 書込ビット・
      ただし sticky ディレクトリは other 書込を除外）のとき Trusted。参照 identity は注入 `RunAsIdent`（live euid 不参照）。
      書込不可検査の対象は**起点ディレクトリの親以上**に限定（起点配下は対象外、設計 §3.3(d) の根拠）。group 判定は
      precomputed `RunAsIdent.Groups`（live なシステム参照なし）。

**成功基準**:
- [x] `go test -tags test ./internal/runner/base/security/...` が通る。本 Phase は単一テストファイル
      `operand_path_resolver_test.go`（`package security` ホワイトボックス）にヘルパを内包し、共有が生じないため
      `test_helpers_zoning.go` は作成しない（test_organization の「共有が生じない場合は作らない」に従う。`//go:build test`
      専用ファイルの追加は Phase 3 で共有が生じた時点で行う）。
- [x] `cp evil $WORKDIR/link`（`link→/etc/passwd`）のターゲット解決を表明（`operand_path_resolver_test.go::TestResolveOperandPath_SymlinkTarget`
      で leaf symlink が `/etc/passwd` に解決される＝Phase 3 の trust-critical High の前提。区分判定自体は Phase 3）。
- [x] `ln -s` 相対 target がリンク親基点で解決される（`EffectiveWorkDir`／`base` 基点ではない）。
- [x] 深い symlink チェーン（`maxHops` 超）・cycle で `error`（→ fail-closed）になる。
- [x] メモ化の**反証可能な呼出回数表明**: 共通の親チェーン（深さ D）を共有する K 個のオペランドを持つフィクスチャで
      注入 `lstat` スタブの呼出回数が `D + K` に等しいことを `==` で表明し、メモ化を外した素朴な `K×(D+1)` と対比して
      有意に小さいことを表明（`TestMemoizationLinear`、D・K は実装で確定）。
- [x] Trusted 述語の差分テスト: 注入 `RunAsIdent` を実 euid/gid と異なる値にし、起点親の所有者/権限により Trusted が
      切り替わる（`TestTrustedPredicate`）。

### PR-2 作成ポイント: read-only operand path resolver and Trusted predicate

**対象ステップ**: P2

**推奨タイトル**: `feat(0142): add read-only operand path resolver and Trusted predicate`

**レビュー観点**: symlink 追従の read-only 性（`lstat`/`readlink` のみ、`os.Stat` 不参照） / メモ化キーの正しさ（相対パスのクロス基点衝突・循環の回避） / fail-closed（cycle・深さ超過・未存在 leaf が symlink） / 注入 `RunAsIdent` による Trusted 判定の決定性

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/782）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: オペランド抽出仕様表＋区分判定＋操作固有の下限（AC-01〜AC-15）

**変更ファイル**:
- 新規 `internal/runner/base/security/destination_zoning.go`（型・オーケストレーション・区分判定）
- 新規 `internal/runner/base/security/destination_zoning_spec.go`（コマンド仕様テーブル・抽出・操作固有の下限）
- 新規 `internal/runner/base/security/destination_zoning_test.go`
- `test_helpers_zoning.go` は作成せず（共有ヘルパは既存 `_test.go`（`tempRoot`・`cmdNameSet`）で足りる）

**作業内容**:
- [x] `LocationKind`（`KindNone`〜`KindDataTransferWrite`）・`ZoningInput`・`LocationResult` を定義（設計 §3.2）。
      `ZoningInput` に `OutputCriticalPathPatterns`（機密複製の下限の判定集合）を注入フィールドとして追加（純関数化のため。設計 §3.2 更新済み）。
- [x] コマンド→`LocationKind`→オペランド抽出規則を**単一の仕様テーブル**（`zoningSpecs`）として実装（AC-06／根本原因1、設計 §3.2）。
      対象コマンドは要件 §F-002（cp/mv/rm/rmdir/unlink/shred/ln/mkdir/touch/install/tee/sponge/truncate/`sed -i`/tar/
      unzip/dd/mount/umount/chmod/chown/chgrp/setfacl/chattr/find）。データ送信書込形（`KindDataTransferWrite`）は型のみ定義し
      抽出は P5。難所のエントリは要件 §F-002 の表を正とする。
- [x] 各オペランドに `OperandRole`（write/read）を付与する（unresolved の非対称下限のため、AC-05）。
- [x] `ClassifyDestinationZone(input ZoningInput, names map[string]struct{}, cmdPath string, args []string) LocationResult`
      を実装（設計 §3.2）。抽出→`ResolveOperandPath` で解決→区分判定→操作固有の下限→全オペランド max。
      1 コマンド評価につき `operandResolver` を 1 つ生成し全オペランドで共有（メモ化で `lstat`/`readlink` を畳む）。デバイス種別判定の
      テスト注入のため、resolver を受け取る内部 `classifyDestinationZone` を分離（公開 API は実 os resolver を生成）。
- （P7 へ繰り越し / PR-2 レビュー由来。本項は P3 の作業対象外のため task checkbox を付けない）`isTrustedOperand` の
      祖先書込可否キャッシュは P7（解決コスト上限テストと同居）で導入する。現状は 1 コマンド 1 resolver でパス解決はメモ化済み。
      祖先書込可否は identity 依存のため、`operandResolver` への素朴な dir-only キャッシュは複数 identity 問い合わせ（テスト等）で
      stale になる。鍵に identity を含めるか単一 identity スコープの確立が要るため、検証テスト（線形呼出）と合わせて P7 で導入する。
      Do は小さく現状コストは線形に収まる。
- [x] パス信頼区分の判定（AC-01〜AC-05）: trust-critical（`SystemCriticalPaths` 一致/配下、`/` は完全一致のみ）→**書込=High／読み取り=Medium**
      （情報露出。設計 §6.2 の `cp /etc/shadow` 例に一致）、ordinary→Medium、safe-zone（Trusted→Low／非 Trusted→Medium フォールバック）、
      unresolved（write=High・read=Medium）。safe-zone が trust-critical と重複/配下のときは trust-critical 優先（AC-04(c)）。
- [x] 操作固有の下限を区分判定後に上乗せ（safe-zone でも Low に降格しない、AC-08〜AC-12、設計 §3.2 表）:
  - [x] 権限/所有権/属性付与（setuid/setgid・world-writable・trust-critical 所有権変更・`chattr -i`）→ High（AC-08, AC-09）。
        chmod/install の付与は argv（`chmodGrantsHigh`）から、`chattr` は属性トークンの `i` から検出。`cp -p`/`-a` の特権メタデータ源
        （setuid/root 所有）は解決後パスを注入 `lstat` で検査（read-only）。**コマンド・バイナリ自体の setuid lstat 下限（⑤）の
        `hasSetuidOrSetgidBit` 流用は P4 の置き換え時の例外**であり本 Phase の付与下限とは別（設計 §3.2 注／§3.4 例外）。
  - [x] `dd` デバイス IO は**デバイス種別**で判定（パス文字列でない）: dd オペランドはゾーンでなくデバイス種別で評価し、ブロック/
        危険キャラクタデバイス→High、無害シンク（`/dev/null`/`/dev/zero` 等）→Low（`/dev` が trust-critical でも降格可能なのは dd の
        この経路のみ）。機密/trust-critical な `if=`（read、非デバイス）は Medium 下限（AC-10）。
  - [x] safe-zone 外への再帰（`rm -r`/`cp -R`/`-a` 等が ordinary/trust-critical へ及ぶ）→ High（AC-11）。信頼 safe-zone 内に閉じた再帰は Low。
  - [x] 機密ファイル複製（コピー元が機密/trust-critical）→ Medium 下限（読み取り元、AC-12）。判定集合は注入された
        `OutputCriticalPathPatterns`（既存 `Config` 由来）を流用（設計 §3.2 注、DRY）。
- [x] コマンド別オペランド特則を実装（AC-12〜AC-15、設計 §3.2／要件 §F-003(b)）: mv の移動元（role=write）/ln のリンク先 target
      （trust-critical→High floor）、`cp -p`/`-a` の特権メタデータ複製 High、mount/umount（全 positional＝mountpoint＋マウント元、
      `umount -a` 無条件 High）、tee/sponge（全 FILE max）、find（`-delete`/`-fprint*` の宛先判定、`-exec` 系の内側実行は本タスク対象外）。
- [x] `LocationResult.Recognized` を計算（完全認識: 全 argv を解析しきった **かつ全オペランドが解決済み（非 `ZoneUnresolved`）**。
      未解析トークン/未知フラグ、または解決不能オペランドが残れば `Recognized=false`＋不完全認識時は High 下限へ倒す、設計 §3.4）。
- [x] 各オペランドの判定を `[]OperandZone`（Index/Raw/Resolved/Zone/Role/MatchedCritical/Trusted/UnresolvedErr）として記録し
      `LocationResult.Operands` に格納。判定由来の `ReasonCode` を `LocationResult.ReasonCodes` に積む。「空（非適用）」と「適用済み
      解決不能（`Zone==ZoneUnresolved` 要素）」を区別可能にする（0143 AC-01 の消費契約、設計 §3.1）。この区別は **2 つの独立したケース**
      として表明する（非ファイル操作コマンド→`len(OperandZones)==0`、適用済み解決不能→`Zone==ZoneUnresolved` 要素を持つ）。

**成功基準**:
- [x] 既知コマンド×代表フラグの表駆動テストで、要件 §F-002 の難所（in-place 編集・`ln -s` 相対 target・アーカイブ抽出 vs 一覧・
      末尾 `/` 削除・権限付与）が各々テスト行を持つ（`TestOperandExtraction_SpecTable`）。`dd` デバイスは `TestFloor_DeviceIO`、
      データ送信書込先は P5。
- [x] 名指しされた全書込/削除/付与形が仕様表のテスト行を持つ（`TestOperandSpecific_*`・`TestFloor_*`、ドリフト防止）。
- [x] 区分判定テスト: trust-critical（`/usr/bin` 配下=High・`/` 完全一致のみ）・ordinary（`/srv`/`/opt`=Medium）・
      safe-zone（Trusted=Low／非 Trusted=Medium）・unresolved（write=High・read=Medium）。
- [x] 操作固有の下限が safe-zone でも降格しないこと（`chmod u+s`・`chmod 0777`・`chown root /usr/bin/x`・`chattr -i`・
      `dd of=/dev/sda`・`rm -r` 外部・`cp /etc/shadow $WORKDIR/x`）。
- [x] 複数オペランドの max（設計 §6.2 の `cp /etc/shadow $WORKDIR/x`=Medium 例を含む、`TestMultipleOperandsMax`）。

### PR-3 作成ポイント: destination zoning classifier and operation floors

**対象ステップ**: P3

**レビュー観点**: オペランド抽出仕様表の網羅（要件 §F-002 の難所が各々テスト行を持つ） / 区分判定（trust-critical/ordinary/safe-zone/unresolved、重複は trust-critical 優先） / 操作固有の下限が safe-zone でも非降格 / 完全認識（`Recognized`）の計算と複数オペランド max

**推奨タイトル**: `feat(0142): add ClassifyDestinationZone with extraction spec table and floors`

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/783）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: `evaluateDimensions` 組み込み＋既存 5 系統の完全認識時抑止（AC-17, AC-18）

**変更ファイル**:
- 変更 `internal/runner/base/risk/evaluator.go`
- 変更 `internal/runner/base/security/command_risk_profile.go`（`ProfileFactorRisk` に DestructionRisk-only 抑止フラグ）
- 変更 `internal/runner/base/risk/evaluator_test.go`
- 変更 `internal/runner/base/risk/coreutils_consistency_test.go`
- 新規 `internal/runner/base/risk/destination_zoning_integration_test.go`

**作業内容**:
- [x] `evaluateDimensions` に判断軸2 ディスパッチを追加（設計 §3.4・§6.1）: `ClassifyDestinationZone` を呼び、
      `Applies==true && Recognized==true` のときだけ既存 5 系統の破壊系 High 寄与を抑止し `LocationResult.Level` を唯一の
      寄与にする。不完全認識/非ファイル操作のときは既存 5 系統をそのまま残す（fail-open 回避）。
- [x] **注入シーム（PR 独立緑のため、§3.2 参照）**: `StandardEvaluator` に axis-2 入力を組み立てるための**注入可能フィールド**
      （`zoning *zoningParams`、既存 `openIdentity` と同じ注入様式。nil=axis-2 無効＝レガシー挙動）を本 Phase で導入し、
      `evaluateDimensions` はそこから `ZoningInput` を作ってディスパッチする。**設計差分**: 注入を生の `*security.Config` でなく
      専用 struct `zoningParams`（systemCriticalPaths／trustedDirectories／outputCriticalPathPatterns／dedicatedTempDir／runAsIdent）
      とした。理由: `Config.TrustedDirectories` フィールド追加は P6 の所掌のため、P4 で `*security.Config` を注入すると同フィールドが
      未存在でコンパイルできない。`zoningParams` は P6 で `security.Config`＋run-as 解決から populate する。統合テストは評価器を
      **直接注入**（`newZoningEvaluator` テストコンストラクタ）して `NewRunner` を経由しない。公開 `NewStandardEvaluator` の引数追加・
      本番 run-as 名解決・`runner.go` 配線・`Config`/`SecuritySpec`/`loader` のフィールド追加は **P6**（よって本 Phase は P6 を待たず緑）。
- [x] ① `IsDestructiveFileOperation` の寄与を完全認識時に飛ばす。
- [x] ② `CoreutilsCommandRisk` の破壊系 High 寄与を完全認識時に飛ばす。**binary 解析抑止に使う `coreutilsHandled` は維持**し、
      破壊系 High の加算のみ抑止する（両者を分離。`applyCoreutilsRisk` ヘルパに切り出し）。
- [x] ③ profile `DestructionRisk` を**破壊コンポーネント粒度**で抑止（設計 §3.4）: `ProfileFactorRisk`/`applyProfileFactors`
      に `suppressDestruction` フラグを渡す。他因子（`NetworkRisk`/`DataExfilRisk` 等）と `NetworkType`/`Reasons` は引き続き適用する。
- [x] ④ `CheckDangerousArgPatterns` の寄与を完全認識のファイル操作コマンドで**ディスパッチ粒度**で抑止する（`Applies && Recognized`
      のとき④を呼ばない）。一致エントリは破壊系で、判断軸2 の区分/操作固有の下限で再確立される（設計 §3.4 表）。`wget`/`curl`/`nc` 等の
      ネットワーク系エントリは④抑止対象外（`Applies==false` のため従来どおり効く）。
- [x] ⑤ setuid/setgid lstat 下限は**再パースせず**、既存シグナルを流用する。完全認識で②coreutils を抑止する経路では、
      `CommandHasSetuidOrSetgidBit`（`hasSetuidOrSetgidBit` の公開ラッパ）で setuid バイナリ High を再確立する（設計 §3.2 注・§3.4 例外）。
- [x] `RiskAssessment.Level` を判断軸1 と max 合成（AC-18）。`LocationResult.ReasonCodes` を `RiskAssessment.ReasonCodes` に追記し、
      `OperandZones` を格納（`foldZoning` ヘルパ）。
- [x] **multicall の保守的扱い（fail-closed の明示）**: `coreutils rm …`（multicall）はコマンド名が `coreutils` で axis-2 が rm 操作に
      分解しないため `Applies==false` となり、レガシー分類（②）が残って宛先非依存に High のまま（fail-closed＝安全、過剰分類）。
      multicall の宛先依存分解は本タスク対象外（将来拡張）。
- [x] 既存テストを宛先依存へ更新:
  - [x] `coreutils_consistency_test.go::TestConsistency_RmAllForms`（basename/absolute は trust-critical=High／safe-zone=Low、
        multicall は上記のとおり保守的に High）。
  - [x] `coreutils_consistency_test.go::TestConsistency_DestructiveAbsolutePath`（trust-critical=High／safe-zone=Low）。
  - [x] `coreutils_consistency_test.go::TestCoreutilsRiskConsistency_Setuid`（setuid バイナリは⑤再確立で High を維持）。
  - [x] `evaluator_test.go::TestEvaluateRisk_AbsoluteRmRfHigh` を `TestEvaluateRisk_RmRfDestinationDependent` に改名し宛先依存へ。

**成功基準**:
- [x] 信頼 safe-zone `rm -rf $WORKDIR/build`=Low（`TestAxis2ReplacesLegacyHigh`、AC-17）。
- [x] ordinary `rm /srv/app/cache.dat`=Medium（AC-17）。
- [x] 未知フラグで宛先不確実な `rm`=High（①〜⑤を残す、AC-17）。
- [x] `cp -a … /usr/bin`=High（判断軸1×判断軸2 の max、`TestAxis1Axis2MaxComposition`、AC-18）。
- [x] 置き換えの取りこぼし防止条件（`TestAxis2RecuperatesSuppressedHigh`）: `chmod 0777`（safe-zone でも High）・
      `chown root`（trust-critical 宛先=High／ordinary=Medium）が §3.2 の権限付与下限/区分で再確立されることを表明。
- [x] 非ファイル操作コマンドでは axis-2 が分類を変えないこと（`TestAxis2NonFileOpUnaffected`、zoning 有無で同一レベル）。

### PR-4 作成ポイント: evaluateDimensions integration and legacy High suppression

**対象ステップ**: P4

**推奨タイトル**: `feat(0142): integrate axis-2 zoning into evaluateDimensions and suppress legacy High`

**レビュー観点**: 完全認識（`Applies && Recognized`）時のみ 5 系統抑止（不完全認識は 5 系統 High を残し fail-open 回避） / `coreutilsHandled` の破壊系 High 抑止と binary 解析抑止の分離 / `DestructionRisk` のコンポーネント粒度抑止（他因子の取りこぼし防止） / 取りこぼし防止条件の再確立と判断軸1×2 の max 合成

> **レビュー上の注意（本 PR は最大かつ最難。①〜⑤を独立に評価する）**: 本 PR は 5 系統（①`IsDestructiveFileOperation`／
> ②`CoreutilsCommandRisk`／③profile `DestructionRisk`／④`CheckDangerousArgPatterns`／⑤setuid lstat 下限）の抑止と、
> 既存テスト 4 本（`coreutils_consistency_test.go` 3 本＋`evaluator_test.go`）の宛先依存化を 1 PR に含む。各抑止は**独立した
> fail-open リスク**を持つため、①〜⑤を個別に——各々が §3.4 取りこぼし防止表の対応行と P4 成功基準のどの行で再確立されるかを
> 突き合わせて——レビューすること。**分割しない**理由: 段階ロールアウト/フラグを設けない方針（§1.2）のため、抑止を別 PR に
> 切り出すと中間状態で既存テストが二重カウントで赤くなり、各 PR 独立緑を満たせない。よって 5 系統＋テスト更新は不可分の単位。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/784）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: データ送信書込先合成＋rsync `host::module` 検出（AC-16）

**変更ファイル**:
- 変更 `internal/runner/base/security/destination_zoning_spec.go`（`KindDataTransferWrite` 抽出器＝curl/wget/scp/sftp/rsync、`isRemoteTerminus`／`host::module` 判定）
- 変更 `internal/runner/base/security/destination_zoning.go`（`remoteEgress` の Medium 床）
- 変更 `internal/runner/base/risk/destination_zoning_integration_test.go`
- 変更 `internal/runner/base/security/destination_zoning_test.go`
- 変更 `internal/runner/base/security/network_analyzer_test.go`（`host::module`/`std::string` が global では非検出の回帰）
- **不変**: `network_analyzer.go`・`command_analysis.go`（`hasNetworkArguments`/`containsSSHStyleAddress` は変更しない。下記の設計選択を参照）

**作業内容**:
- [x] `KindDataTransferWrite` の書込先抽出（`curl -o`/`-O`・`wget` 既定/`-O`/`-P <dir>`・`scp … DEST`・`sftp`・
      `rsync … DEST`/`--delete`）を仕様表に実装（設計 §3.5）。ローカル書込先は zone 判定、リモート宛先（scp/rsync/sftp）は
      `remoteEgress`（ローカル書込パス無し）とする。**scp/rsync のローカル送信元（`!isRemoteTerminus`）も read オペランドとして
      抽出**する（ローカル→ローカル `rsync /etc/shadow $WORKDIR/dst` の機密複製 Medium 下限が `cp` と同等に効く・アップロード時の
      監査記録、レビュー Issue）。機密複製の Medium 下限は `KindCopyMove`／`KindDataTransferWrite` の双方に適用する。
- [x] 最終リスク = `max(データ送信の名前 Medium〔0141〕, 書込先のパス信頼区分)` を合成する（評価層の dimension max が所有。
      curl/wget の egress Medium は network profile（不抑止）から、rsync `host::module` の egress Medium は axis-2 の
      `remoteEgress` 床から供給される、AC-16）。
- [x] rsync/scp のリモート終端判定は **rsync 自身の位置規則**（最初の `/` より前に `:` があればリモートホスト区切り）で実装。
      これにより `host:path`・`user@host:path`・daemon bare module `host::module`・**相対形 `host:file`/`host:`** を一様に検出する
      （相対形は path 部に `/` を持たないため global `hasNetworkArguments` が取りこぼす同類の隙間。設計 §3.5(1)、レビュー Issue 1）。
      リモート宛先のときは zone 対象のローカルパスが無く egress（Medium）が支配、ローカル宛先のときのみ書込先区分。
- [x] **設計選択**: `host::module` の egress 検出は **rsync/scp の `KindDataTransferWrite` 抽出器内に閉じ込めた**（設計 §3.5(2) の
      第 1 案）。`hasNetworkArguments` は変更しない。理由: `hasNetworkArguments` は profile 無しの全コマンドへ広く適用されるため、
      ここへ `host::module` を足すと `std::string`／`HTTP::Tiny` 等を誤検出する。抽出器に閉じ込めることで rsync/scp のみで検出し、
      無関係コマンドの過剰分類をゼロにする。egress の理由コードは既存 `ReasonNetworkArgument` を流用。

**成功基準**:
- [x] (i) `curl <url> -o $WORKDIR/safe`=Medium。**Medium の出所**を `RiskAssessment.ReasonCodes` に `ReasonProfileNetwork`
      （0141 名前由来の network profile Medium）が含まれることで表明（canary 回避。`TestDataTransferWriteComposition`）。
- [x] (ii) `curl -o /usr/bin/x`=High（書込先が名前下限を上回る、AC-16）。
- [x] `rsync src host::module`=Medium（egress 由来、`ReasonNetworkArgument` で出所表明）。
- [x] 純ローカル `rsync $WORKDIR/a $WORKDIR/b`=Low（過剰分類しない）。
- [x] `rsync $WORKDIR/a /usr/bin/x`（ローカル trust-critical 宛先）=High（書込先区分が支配）。
- [x] **非 rsync の `::` 引数は過剰分類しない**: `HasNetworkArguments` が `std::string`／`HTTP::Tiny`／`Namespace::Class`／
      `host::module` を global では非検出（`network_analyzer_test.go::TestHasNetworkArguments_DoubleColonNotMatched`）。
- [x] sftp の限界（明示）: sftp の実書込は対話/`-b` バッチにあり argv に現れないため、`remoteEgress`（Medium）として扱う
      （argv からローカル trust-critical 書込を判別できない既知の限界。profile Medium と同水準）。

### PR-5 作成ポイント: data-transfer-write composition and rsync host::module detection

**対象ステップ**: P5

**推奨タイトル**: `feat(0142): compose data-transfer-write zone and scope rsync host::module egress`

**レビュー観点**: Medium の出所表明（既存 network profile Medium と区別、canary 偽陽性回避） / `host::module` 検出の rsync 限定（`std::string`／`HTTP::Tiny` 等の過剰分類回避） / 純ローカル rsync の非昇格 / 名前下限と書込先パス信頼区分の max 合成

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/785）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 6: config 組み込み＋identity 注入（AC-20, AC-21）

**変更ファイル**:
- 変更 `internal/runner/base/security/types.go`（`Config.TrustedDirectories`）
- 変更 `internal/runner/base/runnertypes/spec.go`（`SecuritySpec.TrustedDirectories`）
- 変更 `internal/runner/base/runnertypes/spec_test.go`
- 変更（必要時）`internal/runner/config/loader.go`
- 変更 `internal/runner/base/risk/evaluator.go`（`NewStandardEvaluator` 引数追加・`RunAsIdent` 注入フィールド）
- 変更 `internal/runner/runner.go`（呼び出し鎖への securityConfig 転送）
- 変更 `internal/runner/base/risk/destination_zoning_integration_test.go`

**作業内容**:
- [x] `security.Config` に `TrustedDirectories []string` を追加（設計 §3.6）。
- [x] `SecuritySpec` に `TrustedDirectories []string toml:"trusted_directories"` を追加（既存 `TrustedGIDs` と並置、設計 §3.6）。
- [x] `NewStandardEvaluator` に `*security.Config` 引数を 1 つ追加（既存引数・戻り型は不変、設計 §3.6）。本引数で、**P4 で導入した
      `StandardEvaluator.zoning *zoningParams` 注入フィールド**を本番経路から populate する（`security.Config` の
      `SystemCriticalPaths`/`TrustedDirectories`/`OutputCriticalPathPatterns`＋run-as 解決 `RunAsIdent` から構築。テストは P4 同様に直接注入）。
- [x] `StandardEvaluator` の run-as 解決ロジック（既定は os/user ベース、注入可能、既存 `openIdentity` と同じ注入パターン）を実装する
      （注入フィールド自体の導入は P4。本 Phase は本番の run-as 名→identity 解決を与える、設計 §3.6）。`runas_identity.go` の
      `resolveRunAsIdent`（注入フィールド `StandardEvaluator.resolveRunAs`）。
- [x] run-as 名→`RunAsIdent` の解決を評価層（組み込み層）で行い、precomputed 値を `ZoningInput` へ注入（設計 §3.6・AC-21）。
      `ClassifyDestinationZone` 以下は live identity API を読まない。`evaluator.go` の `zoningInput`。
- [x] run-as 未設定時は **runner が起動時に確定した original 実行 identity** を注入時に一度だけ解決して用いる。`RunAsIdent` の
      zero 値（`UID:0`）を「未設定」既定にしない（設計 §3.6）。`originalExecutionIdentity()` を `NewStandardEvaluator` で一度解決。
- [x] run-as 名が解決できない場合は当該コマンドの全オペランドを `ZoneUnresolved`（write=High・read=Medium）に倒す（fail-closed、設計 §3.6）。
      `ZoningInput.IdentityUnresolved` → `classifyOperand` 早期 return。
- [x] `runner.go` の呼び出し鎖（`createResourceManager`・両 `create*ResourceManager`・`resolveRiskEvaluator`）へ `securityConfig`
      を通し、`NewStandardEvaluator` へ渡す。normal/dry-run の両経路に**同一に**渡す（AC-22 の一貫性、設計 §3.6）。
- [x] `config/loader.go` で `[security] trusted_directories` が `SecuritySpec` へデコードされ runner へ転送されることを確認。
      go-toml の自動デコード（`toml:"trusted_directories"`）で足り、`runner.go` の `securityConfig.TrustedDirectories` 転送で届くため
      `loader.go` の変更は不要だった。

**成功基準**:
- [x] `spec_test.go` で `[security] trusted_directories` が `SecuritySpec.TrustedDirectories` にパースされる（AC-20）。
- [x] config 経由（テスト注入でなく本番経路）で `SystemCriticalPaths`/`TrustedDirectories` が判定に届き、AC-01/AC-04 が成立する
      （AC-20）。`TestConfigWiredEndToEnd`・`TestRunAsIdentDifferential`（`newConfigEvaluator` で本番 `NewStandardEvaluator` 経路）。
- [x] identity 注入の差分テスト: 注入 `RunAsIdent` をテストプロセスの実 euid/gid と**異なる**値にし、Trusted/Low 判定が注入
      identity に従って変わる（AC-21、決定性テストでは証明できない live 不参照を表明）。`TestRunAsIdentDifferential`。
- [x] run-as 名解決失敗で `ZoneUnresolved`（High）に倒れる（AC-21 fail-closed）。`TestRunAsResolutionFailsClosed`。

### PR-6 作成ポイント: config wiring and run-as identity injection

**対象ステップ**: P6

**推奨タイトル**: `feat(0142): wire TrustedDirectories config and inject run-as identity`

**レビュー観点**: `securityConfig` を normal/dry-run 両経路へ同一に転送（AC-22 一貫性） / run-as 名解決失敗の fail-closed（`ZoneUnresolved` High） / `RunAsIdent` zero 値を「未設定」既定にしない（起動時 original identity 解決） / `[security] trusted_directories` の自動デコードと runner への転送経路

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/787）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 7: 決定性・上限の統合テスト・grep ガード（AC-22, AC-23）

**変更ファイル**:
- 変更 `internal/runner/base/risk/destination_zoning_integration_test.go`
- 新規 `internal/runner/base/risk/live_identity_guard_test.go`（grep ガード）

**作業内容**:
- [x] 決定性テスト（AC-22）: 同一入力・同一 FS 状態で runtime 経路と dry-run 経路が同一レベルを返すことを表明。
      `risk/destination_zoning_integration_test.go::TestDeterminismRuntimeEqualsDryRun`（level/reason/operand zones 一致・
      `$HOME` 不参照を構築間で表明）。
- [x] 解決コスト上限テスト（AC-23）: 多数オペランド（>N）・深い symlink チェーン（>M）で fail-closed（High）になり、メモ化により
      注入 `lstat`/`readlink` 呼出回数が線形に収まることを表明。symlink ホップ上限の fail-closed は P2 で実装済み
      （`operand_path_resolver_test.go::TestResolveOperandPath_DepthExceeded`/`_Cycle`）、線形コストも P2 実装済み
      （`::TestMemoizationLinear`/`::TestMemoizationSymlinkParentNotFolded`）。本 Phase はオペランド数上限の fail-closed を
      追加（`destination_zoning_test.go::TestResolutionCeiling`。上限は分類器側 `classifyDestinationZone` にあるためリゾルバ
      テストではなく分類器テストへ配置）。
- [x] grep ガード（AC-21 補助）: 判断軸2 の組み込みコード（`destination_zoning.go`・`operand_path_resolver.go`）が live-identity
      API（`os.Geteuid`/`os.Getuid`/`syscall`/`unix` の uid/gid/groups・`user.Current`）を呼ばないことを正規表現（§7 footnote の選択
      `|` を用いた式）で検査するテスト。**正のコントロール**（既知の悪例文字列が必ずマッチすること）を併せて表明し、
      パターンの空振り（fail-open）を防ぐ。`risk/live_identity_guard_test.go::TestNoLiveIdentityInZoning`（対象ファイルの存在・
      非空も必須化し、rename による空振りも検出）。

**成功基準**:
- [x] 決定性テストが緑（runtime==dry-run、AC-22）。
- [x] 上限超過で High・呼出回数が線形（AC-23）。
- [x] grep ガードが緑（live-identity API の不在、AC-21）。

### PR-7 作成ポイント: determinism, cost ceiling, and live-identity grep guard

**対象ステップ**: P7

**推奨タイトル**: `test(0142): add determinism, resolution-ceiling, and live-identity guard tests`

**レビュー観点**: runtime==dry-run の決定性表明（AC-22） / 上限超過の fail-closed と注入 `lstat`/`readlink` の線形呼出（AC-23） / grep ガードの正のコントロール（既知の悪例文字列が必ずマッチ、空振り＝fail-open 検出） / live-identity API の不在（AC-21）

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

| Phase | マイルストーン（成果物） | 反映 AC | 依存 |
|---|---|---|---|
| P1 | DTO・型・reason code family（`risktypes`） | AC-05, AC-19, NF-001 | なし |
| P2 | 専用リゾルバ＋Trusted 述語（`security`） | AC-04, AC-23 | P1 |
| P3 | `ClassifyDestinationZone`（抽出仕様表＋区分判定＋操作固有の下限） | AC-01〜AC-15 | P2 |
| P4 | `evaluateDimensions` 組み込み＋既存 5 系統抑止 | AC-17, AC-18 | P3 |
| P5 | データ送信書込先合成＋rsync `host::module` | AC-16 | P3, P4（risk 層の統合テストが P4 のディスパッチを要する） |
| P6 | config 組み込み＋identity 注入（本番経路） | AC-20, AC-21 | P4 |
| P7 | 決定性・上限・grep ガードの統合検証 | AC-22, AC-23, NF-003 | P4-P6 |

各マイルストーン完了時に `make fmt`→`make test`→`make lint` を緑にする。最終マイルストーンの完了基準は、中核 2 パッケージに
加え統合パッケージ（`./internal/runner/...` または `make test` 全体）がコンパイル・緑であること（NF-002）。

### 3.2 PR 構成

各 Phase は単一の関心事を持ち、それ自身でグリーンゲート（`make test && make lint`）を満たす独立単位であるため、Phase 単位で
1 PR にマッピングする（依存順、並べ替えなし）。最大の難所である P4（評価層の置き換え）は単独 PR に隔離する。本機能は
`internal/` 内に閉じ、`cmd/` 層の変更はない（最も本番寄りの P6 配線も `internal/runner` 内）。

| PR | 対象ステップ | 主な変更内容 |
|---|---|---|
| PR-1 | P1 | `risktypes` に `OperandZone` 系 DTO・`RunAsIdent`・新 reason code 7 種を追加（既存 `RiskAssessment` へ非破壊追加） |
| PR-2 | P2 | `security` に read-only オペランドパスリゾルバと Trusted 述語を実装（`lstat`/`readlink` のみ、メモ化、fail-closed） |
| PR-3 | P3 | `ClassifyDestinationZone`（抽出仕様表＋区分判定＋操作固有の下限＋コマンド別特則） |
| PR-4 | P4 | `evaluateDimensions` へ判断軸2 を組み込み、完全認識時に既存 5 系統の破壊系 High を抑止（高リスク・単独隔離） |
| PR-5 | P5 | データ送信書込先の max 合成と rsync `host::module` 検出（rsync 限定・過剰分類回避） |
| PR-6 | P6 | `TrustedDirectories` config 組み込みと run-as identity 注入（本番経路へ securityConfig 転送） |
| PR-7 | P7 | 決定性・解決コスト上限・live-identity grep ガードの統合検証 |

依存関係: PR-2 は PR-1（`RunAsIdent` 等）に、PR-3 は PR-2（`ResolveOperandPath`）に、PR-4 は PR-3 に、PR-5 は PR-3＋PR-4
（risk 層の統合テストがディスパッチを要する）に、PR-6 は PR-4 に、PR-7 は PR-4〜PR-6 に依存する。この順序で各 PR の
依存は先行 PR で満たされる。

**各 PR が単独で緑になるための注入シーム（buildability contract）**: 各 PR が「未来 PR のスタブ無しで `make test` 緑」に
なるのは、信頼区分判定が `security.Config` を直接読まず、**呼び出し側が組み立てて注入する入力**（`ZoningInput`＝
`SystemCriticalPaths`/`TrustedDirectories`/`RunAsIdent` を保持、設計 §3.2）と評価器の注入可能フィールド経由で値を受けるためである。
具体的には:

- **PR-2／PR-3**: `ResolveOperandPath`・Trusted 述語・`ClassifyDestinationZone` は `TrustedDirectories`/`SystemCriticalPaths` を
  **引数／`ZoningInput` フィールド**で受け取り、`security.Config.TrustedDirectories` を参照しない（同フィールドと本番転送は PR-6）。
  テストは `ZoningInput` を直接構築して safe-zone=Low（Trusted）等を表明する。よって PR-6 を待たずコンパイル・緑になる。
- **PR-4**: `evaluateDimensions` は `StandardEvaluator` の**注入可能フィールド** `zoning *zoningParams`（既存 `openIdentity` と
  同じ注入様式。nil=axis-2 無効）から `ZoningInput` を組み立ててディスパッチする。`zoningParams` は生の `*security.Config` でなく
  専用 struct（`Config.TrustedDirectories` 追加が PR-6 のため、PR-4 で `*security.Config` を注入すると未存在フィールドでコンパイル
  不能）。このフィールドは PR-4 で導入し、PR-4 の統合テストは評価器を**直接注入**（`newZoningEvaluator` テストコンストラクタ）して
  `NewRunner` を経由しない。公開 API の `NewStandardEvaluator` 引数追加・本番 run-as 名解決・`runner.go` 配線・
  `Config`/`SecuritySpec`/`loader` のフィールド追加は PR-6 が担い、`zoningParams` を `security.Config`＋run-as 解決から populate して
  本番経路へ接続する。したがって PR-4 は PR-6 を待たず緑になる。

---

## 4. テスト戦略

設計 §7 を正とし、本節は配置とカバレッジ目標を定める。

### 4.1 単体テスト
- **`security/operand_path_resolver_test.go`**: symlink 追従（ターゲット解決で safe-zone 偽装を破れない）・`ln -s` 相対 target・
  深さ/cycle の fail-closed・メモ化の線形呼出・Trusted 述語の差分（AC-04, AC-23）。
- **`security/destination_zoning_test.go`**: オペランド抽出仕様表の表駆動（難所網羅、AC-06/AC-06a）・区分判定（AC-01〜AC-05）・
  操作固有の下限（AC-08〜AC-12）・コマンド別特則（AC-12〜AC-15）・複数オペランド max（AC-07）。
- **`security/network_analyzer_test.go`・`security/command_analysis_test.go`**: `host::module` をリモート終端として検出、
  純ローカルは非ネットワークの回帰（AC-16）。
- **`risktypes/operand_zone_test.go`・`reason_codes_test.go`**: DTO 型健全性・reason code 網羅性/一意性（AC-19, NF-001）。
- **`runnertypes/spec_test.go`**: `[security] trusted_directories` パース（AC-20）。
- **`risk/runas_identity_test.go`**: 本番 run-as 解決器の分岐（user 単独/group 単独/両方/未知ユーザー・グループの
  fail-closed）と起動時 original identity の確定（AC-21 補助）。

### 4.2 統合テスト
- **`risk/destination_zoning_integration_test.go`**: `evaluateDimensions` 経由の置き換え観測可能プロパティ（AC-17）・
  max 合成（AC-18）・データ送信書込先（AC-16）・DTO 格納の値検証（AC-19）・identity 注入の差分（AC-21）・決定性（AC-22）・
  解決コスト上限（AC-23）。
- **更新する既存テスト**: `risk/coreutils_consistency_test.go`・`risk/evaluator_test.go`（破壊系を宛先依存へ）。

### 4.3 後方互換テスト
- 後方互換は不要（新分類を直接適用、設計 付録）。代わりに**置き換えの取りこぼし防止条件**（設計 §3.4 表）をテストで固定し、
  外した 5 系統が捕捉していた水準を判断軸2 が同等以上に再確立することを表明する（fail-open の隙間を塞ぐ）。

### 4.4 テストヘルパ方針（`test_organization.md`）
- FS フィクスチャ（特定の所有者/権限/setuid/symlink を持つディレクトリ・ファイル）を 2 つの新規テストファイルで共有する場合は、
  `security` パッケージ内部の `test_helpers_zoning.go`（`//go:build test`、production と同一パッケージ名）に置く（private API・
  内部型を使うため Classification B。既存 `security/test_helpers.go` と同じ `//go:build test`＋`package security` 規約）。共有が
  生じない場合はヘルパファイルを作らずテスト内に閉じる。
- **FS フィクスチャのクリーンアップ規律**: symlink チェーン・所有者/権限制御ディレクトリ・setuid/world-writable ファイルは実
  ファイルシステム上に作る（注入スタブは AC-23 の呼出計数専用で、実 `lstat`/`readlink` の symlink 追従は実 FS で検証する）。全
  フィクスチャは `t.TempDir`（自動クリーンアップ）配下に作り、temp 配下に閉じない作成があれば取得箇所で `t.Cleanup` を登録する
  （後続 assert が失敗してもリークしない）。
- **他 UID 所有の親ディレクトリ（Trusted 述語の非降格ケース）**: AC-04(d) の「起点親が run-as 以外所有」を実 FS で再現するには
  別 UID 所有のディレクトリ作成（`chown`）が必要で、通常は root 権限を要する。検出は純関数
  `missingForeignOwnerCapability(getuid func() int) []string`（root でない・`chown` 不可なら理由を返す）＋薄い `t.Skip` ラッパとして
  実装し、required 条件欠如・正常系を単体テストする（環境スキップの純粋性）。root 不要で表現できる非 Trusted ケース（group/other
  書込可な親など、`os.Chmod` で再現可能）を優先し、root 必須ケースのみスキップ対象にする。
- **gosec `//nolint` の確定手順**: world-writable/setuid フィクスチャ（`os.Chmod(path, 0o777)`／`os.ModeSetuid` 付与）は gosec が
  flag しうる。**まず**捨てフィクスチャに対し `make lint` を実行して(1)実際に発火する規則（G302 poor file perms／G306 等）と(2)この
  リポジトリで `_test.go` が lint 対象かを確認する。**その後**、発火した各該当箇所（`//go:build test` の `test_helpers_zoning.go` を
  含む。同ファイルは lint 対象にコンパイルされる）に**最小ブロック限定の `//nolint:<発火規則>` を理由コメント付き**で付与し、
  同一パターンの全インスタンスへミラーする。コメントは「test-only fixture」である旨を明記し、production 能力と読めないようにする。

---

## 5. リスク管理

| リスク | 影響 | 緩和策 |
|---|---|---|
| 既存 5 系統の抑止漏れ（fail-open） | 危険形が Low で素通り | 設計 §3.4 の取りこぼし防止条件をテストで固定。完全認識時のみ抑止し、不完全認識は 5 系統 High を残す（P4 成功基準） |
| ②coreutils の `coreutilsHandled` 二重用途 | 破壊系 High 抑止と binary 解析抑止の取り違え | 破壊系 High 加算の抑止と binary 解析抑止（`:263`）を分離して実装（P4 作業内容） |
| ③profile 抑止の粒度誤り | 他因子（network 等）の取りこぼし／破壊 High の残存 | DestructionRisk コンポーネント粒度で無効化（P4 作業内容、設計 §3.4） |
| canary の偽陽性（AC-16(i)） | 名前下限の欠落を既存 profile Medium が隠す | レベルでなく Medium の出所（理由コード）を表明（P5 成功基準、設計 §3.5） |
| live identity 混入（AC-21 退行） | dry-run と runtime の乖離 | 注入 `RunAsIdent` のみ参照＋grep ガード＋差分テスト（P6/P7） |
| ホットパスの無制限 FS I/O（DoS） | ExecuteCommand 遅延 | メモ化＋オペランド/ホップ上限で fail-closed（P2/P7、AC-23） |
| 未存在 leaf の FS 状態依存 | dry-run と runtime でレベル差 | 最深存在親が symlink なら `ZoneUnresolved`（High）に倒す（P2、設計 §3.3 注） |

スケジュールリスク: P4（評価層の置き換え）が最大の難所。P1-P3 を先に固め、P4 はテスト先行（既存テストの宛先依存化）で着手する。

---

## 6. 実装チェックリスト

PR 単位で完了を追跡する（PR 構成は §3.2、各 PR の作成ポイントは Phase 末尾の「PR-N 作成ポイント」節を参照）。

- [x] PR-1 マージ済み（対象ステップ: P1）— DTO・型・reason code family（`risktypes`）／`TestReasonCodes_AllDistinct` 緑
- [x] PR-2 マージ済み（対象ステップ: P2）— 専用リゾルバ＋Trusted 述語／symlink 偽装・fail-closed・メモ化・差分テスト緑
- [x] PR-3 マージ済み（対象ステップ: P3）— `ClassifyDestinationZone`（仕様表＋区分判定＋操作固有の下限＋特則）／表駆動・区分・下限テスト緑
- [x] PR-4 マージ済み（対象ステップ: P4）— `evaluateDimensions` 組み込み＋5 系統抑止／観測可能プロパティ・取りこぼし防止条件・既存テスト更新緑
- [x] PR-5 マージ済み（対象ステップ: P5）— データ送信書込先合成＋rsync `host::module`／(i)(ii)・rsync 3 ケース緑
- [ ] PR-6 マージ済み（対象ステップ: P6）— config 組み込み＋identity 注入（本番経路）／spec パース・差分テスト・fail-closed 緑
- [ ] PR-7 マージ済み（対象ステップ: P7）— 決定性・上限・grep ガード／runtime==dry-run・線形呼出・live 不参照 緑
- [ ] 全 PR マージ後: `make fmt`→`make test`→`make lint` 全緑、`./internal/runner/...` コンパイル（NF-002）

---

## 7. 受け入れ基準の検証

各 AC を検証するタスク/テストを対応づける。種別は `test`（実行可能・誤挙動で落ちる）/`static`（rg/grep/compile）/`manual`。

| AC | 種別 | テスト場所 / 検証コマンド | 期待結果 |
|---|---|---|---|
| AC-01 trust-critical High | test | `security/destination_zoning_test.go::TestClassifyDestinationZone_TrustCritical` | `/usr/local/bin`=High、`/` は完全一致のみ |
| AC-02 ordinary Medium | test | `security/destination_zoning_test.go::TestClassifyDestinationZone_Ordinary` | `/srv`/`/opt`=Medium |
| AC-03 safe-zone Low/Medium | test | `security/destination_zoning_test.go::TestClassifyDestinationZone_SafeZone` | Trusted=Low／非 Trusted=Medium |
| AC-04(a) 解決後パス判定 | test | `security/operand_path_resolver_test.go::TestResolveOperandPath_SymlinkTarget` | `cp evil $WORKDIR/link`(`link→/etc/passwd`)=High |
| AC-04(b) safe-zone 起点限定 | test | `security/operand_path_resolver_test.go::TestSafeZoneOrigin` | `$HOME`/共有 `/tmp` は起点に含めない |
| AC-04(c) 重複は trust-critical 優先 | test | `security/destination_zoning_test.go::TestSafeZoneOverlapsCritical` | 重複/配下は High |
| AC-04(d) Trusted 述語 | test | `security/operand_path_resolver_test.go::TestTrustedPredicate` | 起点親が書込可なら非 Trusted→Medium |
| AC-05 unresolved 非対称 | test | `security/destination_zoning_test.go::TestUnresolvedAsymmetry` | write=High・read=Medium |
| AC-06 抽出網羅 | test | `security/destination_zoning_test.go::TestOperandExtraction_SpecTable`（表駆動） | 難所各行が抽出される |
| AC-06a 仕様表↔AC 連動 | test | `security/destination_zoning_test.go::TestOperandExtraction_SpecTable` | AC-08〜AC-15 名指し形が各々テスト行を持つ |
| AC-07 複数オペランド max | test | `security/destination_zoning_test.go::TestMultipleOperandsMax` | 各区分の max |
| AC-08 権限/所有権/属性付与 | test | `security/destination_zoning_test.go::TestFloor_PermissionGrant` | setuid/0777/trust-critical 所有権/chattr -i=High、safe-zone でも降格せず |
| AC-09 install 権限フラグ | test | `security/destination_zoning_test.go::TestFloor_InstallPermission` | `-m` setuid/`-o`/`-g`=High |
| AC-10 dd デバイス IO | test | `security/destination_zoning_test.go::TestFloor_DeviceIO` | 危険デバイス=High、`/dev/null` 除外 |
| AC-11 safe-zone 外再帰 | test | `security/destination_zoning_test.go::TestFloor_RecursiveOutside` | 外部再帰=High、内部閉鎖=Low |
| AC-12 cp/mv/rm/ln 特則 | test | `security/destination_zoning_test.go::TestOperandSpecific_CopyMoveLink` | 機密コピー元=Medium 下限、`cp -a` 特権メタ=High |
| AC-13 mount/umount | test | `security/destination_zoning_test.go::TestOperandSpecific_Mount` | trust-critical=High、`umount -a`=High |
| AC-14 tee/sponge | test | `security/destination_zoning_test.go::TestOperandSpecific_Tee` | 全 FILE の max、内側未実行 |
| AC-15 find 破壊 | test | `security/destination_zoning_test.go::TestOperandSpecific_Find` | `-delete`/`-fprint*` の宛先判定、読取専用非昇格 |
| AC-16 データ送信書込先＋max | test | `risk/destination_zoning_integration_test.go::TestDataTransferWriteComposition`＋`security/network_analyzer_test.go`（非 rsync `::` 非過剰分類） | (i)=Medium(出所=名前)、(ii)=High、rsync 3 ケース、非 rsync `std::string` 非ネットワーク |
| AC-17 既存 High 置き換え | test | `risk/destination_zoning_integration_test.go::TestAxis2ReplacesLegacyHigh` | safe-zone=Low／ordinary=Medium／unresolved=High |
| AC-18 max 合成 | test | `risk/destination_zoning_integration_test.go::TestAxis1Axis2MaxComposition` | `cp -a … /usr/bin`=High、順序非依存 |
| AC-19 監査 DTO 格納 | test | `risk/destination_zoning_integration_test.go::TestOperandZonesStored` | 各要素の値検証、非適用=空・解決不能=`Zone==ZoneUnresolved` |
| AC-20 config 組み込み | test | `runnertypes/spec_test.go::TestConfigSpec_Parse`（"valid config with security trusted directories"）＋`risk/destination_zoning_integration_test.go::TestConfigWiredEndToEnd` | 本番経路で判定に届く |
| AC-21 identity 注入純粋性 | test+static | test: `risk/destination_zoning_integration_test.go::TestRunAsIdentDifferential`・`::TestRunAsResolutionFailsClosed`／static: `risk/live_identity_guard_test.go::TestNoLiveIdentityInZoning` | 注入 identity で判定変化／fail-closed／live API 不参照 |
| AC-22 runtime==dry-run | test | `risk/destination_zoning_integration_test.go::TestDeterminismRuntimeEqualsDryRun` | 同一入力・同一 FS で同一レベル |
| AC-23 解決コスト上限 | test | symlink ホップ上限: `security/operand_path_resolver_test.go::TestResolveOperandPath_DepthExceeded`/`_Cycle`／オペランド数上限: `security/destination_zoning_test.go::TestResolutionCeiling`／線形コスト: `security/operand_path_resolver_test.go::TestMemoizationLinear` | 上限超過=High、呼出線形 |
| NF-001 reason code 網羅/一意 | test | `risktypes/reason_codes_test.go::TestReasonCodes_AllDistinct` | 新 7 定数含め緑 |
| NF-002 ビルド/テスト緑 | static | `make test && make lint`（または `go build -tags test ./internal/runner/...`） | 終了コード 0 |
| NF-003 決定的・read-only | test+static | test: `::TestDeterminismRuntimeEqualsDryRun`／static: `rg -n 'os\.(Create|CreateTemp|Remove|RemoveAll|WriteFile|Mkdir|MkdirAll|MkdirTemp|OpenFile|Symlink|Link|Rename|Chmod|Chown|Lchown|Chtimes|Truncate|NewFile)|(syscall|unix)\.(Write|Pwrite|Unlink|Unlinkat|Rmdir|Mkdir|Mkdirat|Open|Openat|Symlink|Link|Rename|Chmod|Fchmod|Chown|Fchown|Lchown|Truncate|Ftruncate)' internal/runner/base/security/destination_zoning.go internal/runner/base/security/operand_path_resolver.go`（期待: **マッチ 0 件**） | 書込系 API 不在（read-only） |

> **grep ガードの正規表現に関する注意（ripgrep のメタ文字）**: ripgrep 既定（Rust regex）では `|` が選択（alternation）で、`\|` は
> **リテラルのパイプ文字**になる。`\|` を選択のつもりで使うとパターンが空振りし、危険 API が存在してもガードが「0 件＝合格」と
> 誤判定する（fail-open）。よって選択は素の `|` で書く（上記 NF-003 行・下記 AC-21 ガードとも修正済み）。
>
> grep ガード（AC-21 static）の具体コマンド: `rg -n 'os\.Get(euid|uid|gid|egid|groups)|user\.(Current|Lookup)|syscall\.Get(euid|uid|gid|egid|groups)|unix\.Get(euid|uid|gid|egid|groups)' internal/runner/base/security/destination_zoning.go internal/runner/base/security/operand_path_resolver.go` の期待結果は **マッチ 0 件**（uid/euid/gid/egid/groups を `os`/`syscall`/`unix` の各パッケージで網羅し、`user.Current`／`user.Lookup*` の OS ユーザーDB 参照も塞ぐ）。`live_identity_guard_test.go` がこの検査をテストとして実行する。
>
> **静的 grep ガードの位置づけ（再発防止の原則）**: NF-003／AC-21 の grep は禁止 API の**非網羅な denylist** であり、完全性の
> 保証ではない。read-only／live-identity 不参照の**権威ある保証は挙動テスト**——`TestDeterminismRuntimeEqualsDryRun`
> （runtime==dry-run）と `TestRunAsIdentDifferential`（注入 identity と実 euid を変えて判定が変わることを表明）——が担う。grep は
> defense-in-depth の二次チェックとして、代表的な書込／identity API を低コストに塞ぐ。将来「API X が漏れている」との指摘は、
> X が安価なら正規表現へ追加し、そうでなくても上記挙動テストが本質的に捕捉することを確認すれば足りる（正規表現の網羅性
> 自体を完全性条件にはしない）。
> **自己検証（正のコントロール）を必須にする**: ガードテストは、検査用の正規表現がコンパイルでき、かつ既知の悪例文字列
> （例 `"os.Geteuid()"`・`"os.Create(p)"`）に**必ずマッチする**ことを表明する正のコントロールを含める。これにより将来の
> タイプミスでパターンが空振りに戻っても、ガードが沈黙で壊れることを防ぐ。

---

## 8. 成功基準

- **機能完全性**: AC-01〜AC-23・NF-001〜NF-003 が §7 の test/static 検証で緑。
- **品質**: `make test`・`make lint`・`make fmt` 緑。中核 2 パッケージ＋統合パッケージ（`./internal/runner/...`）コンパイル（NF-002）。
- **セキュリティ検証**: 設計 §5.2 脅威モデルの各脅威（symlink 偽装・宛先隠蔽・safe-zone 内の危険操作・部分認識 fail-open・
  live identity 乖離）が対応テストで緑。置き換えの取りこぼし防止条件（設計 §3.4 表）が固定されている。
- **文書整合**: 本タスクは DTO 定義と `RiskAssessment` への格納まで。logger への JSON 出力・移行ノート・sample config 追従・
  ガイドは 0143 が担う（要件 §2 Out）。

---

## 9. 次のステップ

- 本計画書を `approved` にした後、Phase 順（P1→P7）に実装を進める。各 Phase 完了時にチェックボックスを更新する。
- 実装完了後、0143（監査フィールドの logger 出力・移行ノート・文書整合・sample config 追従）へ引き継ぐ。本タスクが格納する
  `RiskAssessment.OperandZones` の「空 vs 適用済み解決不能」の区別が 0143 AC-01 の消費契約となる（設計 §3.1）。

---

## 10. クロス検索チェックリスト（`make lint`/`make test` で検出できない項目のみ）

各項目は、対応する変更を導入する PR の作成前に実行するゲートである（末尾の owning PR を参照）。

- [ ] **（PR-6）** `NewStandardEvaluator` の呼び出し箇所を全列挙し、新 `*security.Config` 引数を全て更新: `rg -n "NewStandardEvaluator\(" -g '*.go' internal`（本番＋テストの全呼び出しに引数追加。引数追加はコンパイルで検出されるが、テスト注入箇所の見落とし防止に列挙する）。
- [ ] **（PR-1）** reason code 新 7 定数の二重定義・タイポ確認: `rg -n "trust_boundary_write|destination_zone|permission_grant|device_io|recursive_outside_safe_zone|sensitive_source_copy|unresolved_destination" -g '*.go' internal`（定義は `reason_codes.go` の 1 箇所のみ）。
- [ ] **（PR-6）** `TrustedDirectories` の用語一貫性（設計・要件・spec・loader・config sample で同一表記。sample config 追従自体は 0143）: `rg -n "trusted_directories|TrustedDirectories" -g '*.go' internal`。
- [ ] **（PR-4）** 設計 §3.4 で「不変」とした `security/coreutils_test.go` の `CoreutilsCommandRisk` テストが誤って変更されていないこと（関数挙動は不変、置き換えは評価層のみ）。
- [ ] **（PR-5）** AC-16(i) の出所表明で 0141 名前下限由来の理由コードを参照する場合は、その定数が 0141 で確定済みかを事前確認: `rg -n "ReasonCode = \"" internal/runner/base/risktypes/reason_codes.go`（データ送信の名前下限に対応する定数名を特定。未確定なら自己完結の canary 形を採る）。
