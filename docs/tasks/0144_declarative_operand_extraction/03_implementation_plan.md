# オペランド抽出の宣言的フラグ仕様化（単一 getopt パーサ） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-25 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は [02_architecture.md](02_architecture.md)（approved）の実装計画である。設計の詳細（型・パーサ・解析形態・脅威モデル）は
> 設計書を参照し、本書では重複させない。用語は [01_requirements.md](01_requirements.md) §1（オペランド・引数付きフラグ・
> 真偽フラグ・再帰フラグ・引数省略可・非フラグ引数）に従う。

## 1. 実装概要

### 1.1 目的

コマンドごとの個別抽出処理を、宣言的フラグ仕様＋単一 getopt パーサ＋薄い意味づけ関数（`ToExtraction`）へ置き換える。
観測可能な挙動（`LocationResult` の全フィールド）を変えない挙動保存リファクタである（設計 §1.1）。

### 1.2 実装原則

- 挙動保存を最優先する。既存テストは無改変で緑を保ち、移行は差分テストでゲートする（設計 §7）。
- フラグ知識はコードからデータ（`FlagSpec`）へ移す。区分判定・リゾルバは触らない。
- fail-closed の `Recognized` contract を保存する（未知/曖昧形・引数欠落で High 下限）。
- 1 コマンドずつ移行し、各段階で常に緑のチェックポイントを保つ（設計 §8）。

### 1.3 既存コード調査結果

調査対象は `internal/runner/base/security`。`zoningSpecs`（`destination_zoning_spec.go:87`、`map[string]commandSpec`、約 31 エントリ）
と 26 個のコマンド別抽出関数（`extractCopyMove` など。総称 `extractXxx`）、共通の `scanFlags`
（`destination_zoning_spec.go:127`）が現状の抽出層を成す。

- 変更が必要: `destination_zoning_spec.go` — `commandSpec{kind, extract func}` を `CommandFlagSpec{Kind, Flags, ToExtraction}`
  参照へ移行し、各 `extractXxx` を薄い `ToExtraction` へ縮小、`scanFlags` と各抽出処理内の重複フラグ集合定義を撤去する。
- 新規: `getopt.go`（`parseArgs`/`ParseResult`/`HasFlag`）、`flag_spec.go`（`FlagArity`/`ValueRole`/`FlagSpec`/`CommandFlagSpec`
  と全コマンドの宣言的仕様表）。いずれも未作成であることを確認済み。
- 再利用（不変）: 値内文法ヘルパ `chmodGrantsHigh`・`aclGrantsWrite`・`tarMode`/`normalizeTarArgs`・`isRemoteTerminus`/
  `extractRemoteCopy`・`isChattrMode` は意味づけ関数から呼ぶ。新規実装しない。
- 不変: `destination_zoning.go`（`classifyDestinationZone` ほか）と `operand_path_resolver.go`。`extraction` 入力契約は不変。
- 既存テスト（無改変で緑を保つ。AC-09）: `destination_zoning_test.go`（25 テスト関数）、`operand_path_resolver_test.go`
  （11 テスト関数）。
- 別パッケージのテスト更新: `internal/runner/base/risk/live_identity_guard_test.go`（0142 の静的ガード）の対象ファイル集合へ
  新規 2 ファイルを追加する（設計 §2.1・§7）。

## 2. 実装ステップ

### Phase 1: 単一 getopt パーサと型定義

**変更ファイル**:
- 新規 `internal/runner/base/security/flag_spec.go`（型定義のみ。仕様表は Phase 2）
- 新規 `internal/runner/base/security/getopt.go`
- 新規 `internal/runner/base/security/getopt_test.go`

**作業内容**:
- [ ] `flag_spec.go` に `FlagArity`（`ArityNone`/`ArityRequired`/`ArityOptional`）・`ValueRole`（`ValueUnset`/`ValueNonPath`/
      `ValueWrite`/`ValueRead`）・`FlagSpec`・`CommandFlagSpec` の型を定義する（設計 §3.1）。`parseArgs` が `FlagSpec` に依存するため
      型は本フェーズで先に置く（仕様表の中身は Phase 2）。
- [ ] `getopt.go` に `parseArgs(flags []FlagSpec, args []string) ParseResult`・`ParseResult`・`HasFlag(canonicalKey string) bool`
      を実装する（設計 §3.1）。一元処理する形式: `--flag=value`・付随短縮値・短縮連結・`--`・引数省略可・別名正規化。
      短縮連結中の引数付きフラグ規則と引数省略可の付随形限定は設計 §3.1 の規則に従う。総 argv 長に対して線形。
- [ ] `getopt_test.go`（表駆動）を追加: 全形式の網羅（AC-03）／語を暗黙に捨てない・未知フラグ・引数必須フラグの値欠落で
      `Recognized=false`（AC-04）／別名正規化で表記違いが同一結果（AC-05）／引数省略可は付随形のみ・分離後続語を消費しない・
      クラスタ内省略可（`sed -ir` → `-i` の値 `r`）（AC-06）／大量 argv・長い短縮連結の病的入力。

**成功基準**:
- [ ] `go test -tags test ./internal/runner/base/security/` で `getopt_test.go` が緑。
- [ ] `make fmt && make test && make lint` が緑。

### Phase 2: 宣言的仕様表・完全性メタテスト・差分テスト基盤

**変更ファイル**:
- 変更 `internal/runner/base/security/flag_spec.go`（全コマンドの仕様表を追加）
- 新規 `internal/runner/base/security/flag_spec_test.go`
- 新規 `internal/runner/base/security/extraction_legacy_test.go`（凍結スナップショット）
- 新規 `internal/runner/base/security/extraction_diff_test.go`

**作業内容**:
- [ ] `flag_spec.go` に全対象コマンドの `CommandFlagSpec`（`Flags` の `FlagSpec` 群と `Kind`）を定義する（設計 §3.1）。同一フラグの
      全表記を 1 つの `FlagSpec.Names` にまとめ（AC-01）、引数の必須/省略可を `Arity` で区別する。アリティ不変条件（設計 §3.1）を守る:
      現行で次の語を必ず消費するフラグは `ArityRequired`、`ArityOptional` は実 CLI で省略可なフラグのみ。
- [ ] `flag_spec_test.go` に完全性メタテスト `TestSpecCompleteness` を追加（AC-07）: 全 `FlagSpec.Names` が 1 要素以上／
      `ArityNone` の `FlagSpec` は `Value == ValueUnset`／引数付きフラグは `Value != ValueUnset`（operand 化 or 非 path 明示）。
- [ ] `flag_spec_test.go` に `TestArityInvariant` を追加: 各 `ArityRequired` フラグに `--flag NEXT`（分離形）を与えると NEXT が
      値として消費されること、各 `ArityOptional` フラグでは分離後続語が消費されないことを、凍結 `legacyExtractXxx` の挙動と
      突き合わせて確認する。これにより「現行で次の語を消費するフラグが `ArityOptional` へ誤分類される」fail-open（リスク表 行2）を
      機械検出する。
- [ ] `extraction_legacy_test.go` に現行の `extractXxx` 群を `legacyExtractXxx` として**凍結コピー**する（`//go:build test`、
      テスト専用、`private` 型 `extraction` を使うため同一パッケージ）。差分テストの不変なオラクルとし、以降変更しない。
- [ ] `extraction_diff_test.go` を追加: 各コマンド×{各フラグの全形（`-x`/`-x=v`/`-xv`/`-x v`/連結/`--long`/`--long=v`/引数省略可の
      付随形・分離形/`--`/先頭 `-`/空語/重複フラグ）＋少量のファズ}の生成コーパスで、`legacyExtractXxx` と新実装の `extraction` を
      `reflect.DeepEqual` で**構造体全体**を一致比較する。フィールドを個別列挙すると `applies` 等の取りこぼしや将来追加の漏れが
      起きるため、構造体をまるごと比較する。対象は `extraction` の全 8 フィールド（`applies`/`recognized`/`recursive`/
      `grantsPermission`/`preserveMeta`/`umountAll`/`remoteEgress`/`operands`。順序・role・base を含む）である（設計 §7）。
      dd・chattr の異常形（`dd if=` 欠落・`chattr +i`/`-i`）もコーパスに含める。

**成功基準**:
- [ ] 完全性メタテストとアリティ不変条件チェックが緑。
- [ ] 凍結スナップショットと差分テスト基盤がコンパイルでき、移行前は旧実装同士で恒真（基盤の健全性確認）。

### Phase 3: コマンド単位の移行と旧実装の撤去

**変更ファイル**:
- 変更 `internal/runner/base/security/destination_zoning_spec.go`
- 変更 `internal/runner/base/security/flag_spec.go`（必要に応じ仕様の調整）
- 新規 `internal/runner/base/security/destination_zoning_parity_test.go`（回帰代表ケースと挙動同一性テストの追加先。既存 `_test.go` は無改変に保つ）

**作業内容**:
- [ ] `zoningSpecs` の各エントリを 1 コマンドずつ `CommandFlagSpec`＋`ToExtraction`（`parseArgs` を消費）へ移行する。移行中は
      旧 `extract` 経路と新経路が一時的に共存してよい（設計 §8）。getopt 適合コマンドの `ToExtraction` は `ParseResult` のみ参照し、
      `Values` を正規キー（`FlagSpec` 由来）で読む（map を `for range` しない。設計 §3.1 決定性制約）。
- [ ] tar・chattr は事前正規化を挟む（tar: `normalizeTarArgs`、chattr: `isChattrMode` 合致トークンを `parseArgs` 前に分離）。
      dd は `Flags` を空にし `ToExtraction` 内で `if=`/`of=` を専用解析（設計 §3.5）。
- [ ] 各コマンドの移行ごとに、当該コマンドの差分テスト（`extraction_diff_test.go`）と既存テスト（`destination_zoning_test.go`）が
      緑であることをゲートとする。緑にならない限り次のコマンドへ進まない。
- [ ] 回帰代表ケース（AC-08）を `destination_zoning_parity_test.go`（新規）に追加する（既存 `destination_zoning_test.go` は無改変に保つ）。
      既存の `TestExtractorHardening*`/`TestACLGrantsWrite_DefaultEntry`/`TestTarExtractRecognized` が既にカバーするケースは再掲せず、
      未カバー分のみ追加: 別名表記・引数省略可・`sed -e`・`chmod` シンボリック setuid・`setfacl` default ACL・`chown`/`chgrp` の
      `--from`/`--reference`・`ln` シンボリック/ハードリンク・`tar` 第1語限定モード解析。
- [ ] 全コマンド移行後、`scanFlags` と production 側の各 `extractXxx`（および各抽出処理内の重複フラグ集合定義）を撤去する。
      旧 `commandSpec` 型が未使用になれば併せて撤去する。

**成功基準**:
- [ ] 全コマンドで差分テストが緑。
- [ ] `make deadcode` で旧 `extractXxx`/`scanFlags` の取り残しがない（凍結スナップショットはテスト専用のため対象外）。
- [ ] `make fmt && make test && make lint` が緑。

### Phase 4: 挙動同一性・fail-closed・静的ガード

**変更ファイル**:
- 追記 `internal/runner/base/security/destination_zoning_parity_test.go`（`LocationResult` 同一性表・fail-closed ケース）
- 変更 `internal/runner/base/risk/live_identity_guard_test.go`（対象ファイル集合の拡張）

**作業内容**:
- [ ] `LocationResult` 同一性テスト `TestLocationResultParity`（AC-10）を `destination_zoning_parity_test.go` に追加: `zoningSpecs` の
      全エントリ（件数はハードコードせず実集合を range）×代表フラグで、リファクタ後の `LocationResult`（`Applies`/`Recognized`/`Level`/
      `Operands`/`ReasonCodes`）が期待値と一致することを表駆動で固定する。
- [ ] fail-closed テスト（AC-11）を `destination_zoning_parity_test.go` に追加: 未知/曖昧形・引数欠落・必須非フラグ引数欠落・解決不能で
      `Recognized=false`→High 下限。
- [ ] 既存テスト無改変の確認（AC-09）: `git diff origin/main -- internal/runner/base/security/destination_zoning_test.go
      internal/runner/base/security/operand_path_resolver_test.go` が**空**であること（新規ケースは別ファイルに置くため、既存 2 ファイルは
      機械的に無改変であることを保証する）。
- [ ] `live_identity_guard_test.go` の `zoningGuardedFiles` に `../security/getopt.go`・`../security/flag_spec.go` を追加する
      （新規ガードは作らず既存を再利用。設計 §7）。

**成功基準**:
- [ ] AC-09〜AC-11 が緑。`TestNoLiveIdentityInZoning` が新規 2 ファイルを含めて緑。
- [ ] `make fmt && make test && make lint` が緑、`./internal/runner/...` がコンパイル。

## 3. 実装順序とマイルストーン

| Phase | マイルストーン（成果物） | 反映 AC | 依存 |
|---|---|---|---|
| P1 | 単一パーサ＋型定義（`getopt.go`/`flag_spec.go` 型） | AC-03〜AC-06 | なし |
| P2 | 宣言的仕様表＋完全性メタテスト＋差分テスト基盤 | AC-01, AC-07 | P1 |
| P3 | コマンド単位移行＋旧実装撤去＋回帰代表ケース | AC-02, AC-08 | P2 |
| P4 | 挙動同一性・fail-closed・静的ガード | AC-09, AC-10, AC-11 | P3 |

## 4. テスト戦略

設計 §7 を実装に落とす。挙動保存の主担保は差分テスト（`extraction_diff_test.go`）に置き、固定表（AC-09/AC-10）は例示と位置づける。

- 単体（パーサ）: `getopt_test.go` の表駆動で全形式・fail-closed・別名正規化・引数省略可・病的入力（AC-03〜AC-06）。
- 完全性メタテスト／不変条件: `flag_spec_test.go`（`TestSpecCompleteness`＝AC-07・Names 非空・`ArityNone→ValueUnset`、`TestArityInvariant`＝アリティ不変条件）。
- 差分テスト: `extraction_diff_test.go`（凍結 `legacyExtractXxx` vs 新実装、生成コーパスを `reflect.DeepEqual` で構造体全体一致）。各コマンド移行のゲート。
- 回帰代表ケース: `destination_zoning_parity_test.go`（新規。AC-08。既存の `TestExtractorHardening*` 等は無改変で維持）。
- 挙動同一性: 既存テスト無改変緑（AC-09。既存 2 ファイルの git diff が空）＋ `destination_zoning_parity_test.go::TestLocationResultParity`（`zoningSpecs` 全件 range、AC-10）。
- fail-closed: `destination_zoning_parity_test.go`（分類器）/`getopt_test.go`（パーサ）＋既存 `TestUnresolvedAsymmetry`（AC-11）。
- 静的ガード: `risk/live_identity_guard_test.go::TestNoLiveIdentityInZoning` に新規 2 ファイルを追加（NF-003 補助）。
- テストヘルパ配置: 凍結スナップショット・差分テストはいずれも `security` パッケージ内の `*_test.go`（private 型 `extraction` を
  使うため同一パッケージ）。本パッケージの既存 `_test.go` の慣習に合わせ `//go:build test` を付し、`-tags test` で実行する。
  新規の cross-package ヘルパ（`testutil/`）は不要。

## 5. リスク管理

| リスク | 影響 | 緩和策 |
|---|---|---|
| パーサ書き換えによる未列挙入力形での挙動差（取りこぼし＝fail-open 方向も含む） | 区分判定の誤り | 差分テストを生成コーパス＋全フィールド比較で実施し、各コマンド移行のゲートにする（固定表に依存しない） |
| `ArityOptional` 誤分類で値（path）取りこぼし | fail-open | アリティ不変条件を完全性メタテストで強制（現行挙動と突き合わせ） |
| chattr の `-i` 等が未知フラグ誤認 | fail-closed（過剰分類） | chattr は事前正規化で属性トークンを分離してから `parseArgs`（設計 §3.5） |
| `Values` の map 反復で順序非決定 | `Operands`/`ReasonCodes` 順序揺れ | `ToExtraction` は正規キー直接参照のみ・順序は `NonFlagArgs` から（設計 §3.1）。差分テストが順序差を検出 |
| 移行途中の中間状態での退行 | 一時的なバグ | コマンド単位移行＋各段階で差分・既存テスト緑をゲート（常に緑のチェックポイント） |

## 6. 実装チェックリスト

- [ ] Phase 1 完了（パーサ・型・パーサテスト緑）
- [ ] Phase 2 完了（仕様表・メタテスト・差分基盤）
- [ ] Phase 3 完了（全コマンド移行・旧実装撤去・回帰ケース）
- [ ] Phase 4 完了（同一性・fail-closed・静的ガード）
- [ ] 全 PR マージ後: `make fmt`→`make test`→`make lint` 全緑、`./internal/runner/...` コンパイル（NF-001）

## 7. 受け入れ基準の検証

| AC | 種別 | 検証 | 期待 |
|---|---|---|---|
| AC-01 単一宣言テーブル・全表記1エントリ | test | `security/flag_spec_test.go::TestFlagSpec_AllSpellingsOneEntry`（例: `cp` の `-t`/`--target-directory` が単一 `FlagSpec.Names`） | 同一フラグの全表記が 1 エントリ |
| AC-02 別名追加がデータ1箇所で完結 | test | `security/flag_spec_test.go::TestAliasAddition_NoCodeBranch`（`Names` に別名を加えると当該別名経由で値が取れる） | コード分岐追加不要で値取得可 |
| AC-03 全フラグ形式の一元処理 | test | `security/getopt_test.go::TestParseArgs_Forms`（表駆動） | 各形式が正しく解析される |
| AC-04 語を捨てない・未知/欠落で fail-closed | test | `security/getopt_test.go::TestParseArgs_FailClosed` | 未知/欠落で `Recognized=false` |
| AC-05 別名正規化で同一結果 | test | `security/getopt_test.go::TestParseArgs_AliasNormalization` | 表記違いが同一抽出結果 |
| AC-06 引数省略可は付随形のみ | test | `security/getopt_test.go::TestParseArgs_OptionalArg`（`tar --one-top-level -xf a.tar`・`sed -ir`）＋`security/flag_spec_test.go::TestArityInvariant`（必須→省略可の誤分類検出） | 分離後続語を消費しない・クラスタ規則どおり・アリティ不変 |
| AC-07 完全性メタテスト | test | `security/flag_spec_test.go::TestSpecCompleteness`（Names 非空・`ArityNone→ValueUnset`・引数付きは `ValueRole != ValueUnset`） | 未分類・不整合で失敗 |
| AC-08 回帰代表ケース | test | 既存（無改変）`security/destination_zoning_test.go::TestExtractorHardening`/`TestExtractorHardening2`/`TestExtractorHardening3`/`TestACLGrantsWrite_DefaultEntry`/`TestTarExtractRecognized`＋不足分を `security/destination_zoning_parity_test.go` に追加 | 各代表ケースが緑 |
| AC-09 既存テスト無改変緑 | test+static | test: `go test -tags test ./internal/runner/base/security/`／static: `git diff origin/main -- internal/runner/base/security/destination_zoning_test.go internal/runner/base/security/operand_path_resolver_test.go` の出力が**空**（新規ケースは別ファイルに置くため既存 2 ファイルは無改変） | 既存テストが変更なしで緑 |
| AC-10 LocationResult 同一性 | test | `security/destination_zoning_parity_test.go::TestLocationResultParity`（`zoningSpecs` 全件 range×代表フラグ）＋差分テスト `security/extraction_diff_test.go::TestExtractionDiff`（`reflect.DeepEqual` 全フィールド） | リファクタ前後で全フィールド同一 |
| AC-11 fail-closed の保存 | test | 既存（無改変）`security/destination_zoning_test.go::TestUnresolvedAsymmetry`＋`security/getopt_test.go::TestParseArgs_FailClosed`＋`security/destination_zoning_parity_test.go` の分類器 fail-closed ケース | 未知/欠落/解決不能で High 下限 |
| NF-001 ビルド/テスト緑 | static | `make fmt && make test && make lint`（終了コード 0） | 0 |
| NF-003 決定性・read-only | test+static | test: `security/extraction_diff_test.go::TestExtractionDiff`（決定的一致）／static: `internal/runner/base/risk/live_identity_guard_test.go::TestNoLiveIdentityInZoning` の対象に `getopt.go`・`flag_spec.go` を追加して緑 | live identity/環境/非決定 API 不参照 |

## 8. 成功基準

- AC-01〜AC-11・NF-001/NF-003 が §7 の test/static 検証で緑。
- 既存の `destination_zoning_test.go`・`operand_path_resolver_test.go` が無改変で緑（挙動保存の最重要証拠）。
- `scanFlags` と production 側の個別 `extractXxx` が撤去され、フラグ知識が宣言データへ一元化されている。
- 新規ファイル操作コマンド/フラグの追加が、仕様データのエントリ追加（＋必要なら薄い `ToExtraction`）で完結する（NF-002 の保守性）。

## 9. 次のステップ

- 本計画が `approved` になり次第、Phase 1 から実装を開始する（`/runplan 0144`）。
- 監査の family 区別・logger 出力は 0143 の所掌であり本タスクの対象外（設計 §9）。

## 10. クロス検索チェックリスト（`make lint`/`make test` で検出できない項目）

- [ ] `scanFlags` の残存参照: `rg -n "scanFlags" internal/runner/base/security` が production コード（非 `_test.go`）でマッチ 0
      （Phase 3 完了後。テストの言及があれば併せて整理）。
- [ ] production 側 `extractXxx` の取り残し: `rg -n "func extract[A-Z]" internal/runner/base/security/destination_zoning_spec.go`
      がマッチ 0（凍結スナップショット `legacyExtractXxx` は `extraction_legacy_test.go` にのみ存在）。
- [ ] 旧 `commandSpec` 型の残存: `rg -n "commandSpec" internal/runner/base/security` が未使用の取り残しを含まない。
- [ ] 用語整合: 計画・設計・コード識別子で `NonFlagArgs`/`ToExtraction`/`ValueUnset` 等の表記が一致（設計 §3.1 と一致）。
