# コマンド別フラグ仕様の実 CLI 整合（過剰認識・宣言漏れの是正） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-25 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は [01_requirements.md](01_requirements.md)（要件）と [02_architecture.md](02_architecture.md)（設計）の実装計画である。
> 設計の詳細は本書で再掲せず、節番号で参照する。用語・フェーズ構成は `02_architecture.md` に従う。

## 1. 実装概要

### 1.1 目的

各対象コマンドの宣言的フラグ仕様（`internal/runner/base/security/flag_spec.go` の `FlagSpec` 群）を実 CLI に整合させ、
過剰認識（実 CLI に無いフラグの受理）と宣言漏れ（実 CLI にあるフラグの未宣言）を解消する。是正は宣言データの編集に閉じ、
パーサ（`parseArgs`）・意味づけ関数（`ToExtraction`）・区分判定・リゾルバのコードは変更しない（[02 §1.1](02_architecture.md)）。

### 1.2 実装方針

- **データのみの編集**: 本番コードの変更は `flag_spec.go` の 1 ファイルに閉じる（[02 §2.1](02_architecture.md)）。
- **安全側の挙動変更**: 過剰認識除去は `recognized` を true→false（fail-closed 方向・無条件に安全）、宣言漏れ追加・役割修正は
  正しい `ValueRole`/`Arity` 付与で fail-open を防ぐ（[02 §5](02_architecture.md)）。
- **意図的逸脱の管理**: 凍結オラクル（pre-0145 挙動を表す独立オラクル）を基準とする差分テストからの逸脱を `diffExclusions` へ
  厳密一致で登録し、肯定側の回帰テストで新挙動を表明する（[02 §3.3](02_architecture.md)）。
- **argv を直接走査して決まる信号を保つ**: `preserveMeta` 等は宣言フラグ（`Flags`）と独立に argv を直接走査して決まるため、その走査処理には手を加えない（[02 §3.2](02_architecture.md)）。

### 1.3 既存コード調査結果

`internal/runner/base/security` パッケージの現状を調査した。本タスクで触れる対象と要否は次のとおり。

**本番コード（`flag_spec.go`）— 変更対象:**
- 共有ヘルパ `copyMoveFlags()`（`cp`/`mv`）・`removeFlags()`（`rm`/`rmdir`/`unlink`）・`simpleWriteFlags()`（`mkdir`/`sponge`）は、
  実 CLI が異なるコマンドへ同一のフラグ集合を付与しており過剰認識の温床（[02 §1.2](02_architecture.md)）。**分割が必要。**
- `touch` は既に個別宣言済みだが、真偽フラグ `-p`/`-v`/`-i` が過剰認識（要件 §1.1）。**当該 3 フラグの削除が必要。**
- ビルダ `valueFlag`/`boolFlag`/`recursiveFlag`/`optionalFlag` と型（`FlagSpec`/`FlagArity`/`ValueRole`/`CommandFlagSpec`）は不変。
  **追加・変更不要**（[02 §3.5](02_architecture.md)）。
- `ownerFlags()`（`chown`/`chgrp`）・`tarFlagSet`・`chattrFlagSet`・`curlFlags()`/`wgetFlags()`/`scpFlags()`/`rsyncFlags()`・
  `sedRestFlagSet` ほか個別宣言は、フェーズ 1 の典拠突き合わせ後に過不足があれば修正（**棚卸し対象**）。

**本番コード — 不変（変更不可）:**
- `destination_zoning_spec.go`（`extractRemove`/`extractSimpleWrite`/`extractCopyMove`/`extractSed`/`extractOwner`/`extractLink` ほか
  `ToExtraction` 群、および `preserveMeta` 等を argv から直接求める走査処理）、`getopt.go`、`destination_zoning.go`、`operand_path_resolver.go`。

**テストコード — 変更対象:**
- `extraction_diff_test.go`（`//go:build test`）: `diffExclusions`・`diffFixtures` を更新。現状の `diffExclusions` には 0144 で確定した
  長形再帰逸脱（`cp`/`mv` `--recursive`/`--archive`、`rm`/`rmdir`/`unlink` `--recursive`）が登録済み。`diffFixtures["mv"]` に
  `{"-ra","s","d"}` があり、`mv` の `-r`/`-a` 削除後は凍結オラクルと乖離するため、**この入力を `diffExclusions` へ逸脱登録する必要がある**。`rmdir`/`unlink` は `diffFixtures` に項目が無い。
- `destination_zoning_parity_test.go`（プレーン）: 肯定側の回帰テスト。`TestExtractionRegressionCases`（`runExtraction` ヘルパで
  `extraction` を直接アサート）・`TestLocationResultParity`（`commandFlagSpecs` を range して全コマンドの代表入力を検証）。
  AC-02 の肯定的表明と削除フラグ網羅メタテストの追加先。

**テストコード — 原則不変（緑維持）:**
- `flag_spec_test.go`（プレーン・`commandFlagSpecs` を range するデータ駆動メタテスト群: `TestSpecCompleteness`・
  `TestSpecNoDuplicateNames`・`TestEveryCommandHasExtractor`・`TestArityInvariant`・`TestAliasAddition`）は無改変で緑。
- `destination_zoning_test.go`・`operand_path_resolver_test.go`・`extraction_legacy_test.go`（凍結オラクル）は不変。
  既存の代表入力はすべて実 CLI 上のフラグ（例 `mkdir -m 0777`・`ln -s`・`tar -xf -C`）を用いており、過剰認識される偽フラグを
  使っていないため、原則無改変で緑を保つ（[02 §3.4](02_architecture.md)）。

## 2. 実装ステップ

フェーズ構成は [02 §8](02_architecture.md) の実装優先順位に一致させる。

### フェーズ 1: 典拠確定（man ページ棚卸し）

**対象成果物**: 本書 付録 A「実 CLI フラグ典拠表」。

- [ ] `commandFlagSpecs` の全登録コマンドについて、権威ある仕様を典拠として現行宣言フラグと突き合わせ、(a) 過剰認識（削除すべき
  真偽フラグ）、(b) 宣言漏れ（追加すべきフラグ）、(c) 表記・アリティ・値役割の不一致 を一覧化する。典拠は付録 A に明記する（AC-01）。
  - 典拠の所在: GNU coreutils コマンド（`cp`/`mv`/`rm`/`rmdir`/`unlink`/`ln`/`mkdir`/`touch`/`install`/`shred`/`truncate`/`mknod`/
    `chmod`/`chown`/`chgrp`/`dd`）は GNU coreutils マニュアル、`sed` は GNU sed マニュアル、`tar` は GNU tar マニュアル、`mount`/`umount`/
    `chattr` は util-linux / e2fsprogs マニュアル、`setfacl` は acl パッケージ man、`sponge` は moreutils man、`tee` は coreutils、
    `unzip` は Info-ZIP man、`curl`/`wget`/`scp`/`rsync`/`sftp` は各 man ページ。
- [ ] 付録 A の各行に「削除/追加/修正」の別と典拠（man ページ名）を記す。要件 §1.1 が挙げた代表例（`sponge`/`mkdir`/`touch`・
  `rmdir`/`unlink`・`mv`）は既に典拠付きで確定しているため、後述のフェーズ 2 で確定入力として扱う。

**完了条件**: 全コマンドの是正内容（削除・追加・修正）が付録 A に典拠付きで確定している。

### フェーズ 2: 共有ヘルパ分割と実 CLI 整合（`flag_spec.go`）

付録 A に基づき `flag_spec.go` を編集する。意味づけ関数（`ToExtraction`）はコマンド間で共有のまま保つ（[02 §1.2](02_architecture.md)）。

**共有ヘルパの分割（de-share。過剰認識除去の構造的原因）:**
- [ ] `removeFlags()` を分割する。`rm` 専用ヘルパ（実 CLI フラグ集合: `-r`/`-R`/`--recursive`・`-f`/`--force`・`-i`・`-I`・
  `--interactive`・`-v`/`--verbose`・`-d`/`--dir`・`--one-file-system`・`--preserve-root`/`--no-preserve-root` 等）を残し、
  `rmdir` 専用（`-p`/`--parents`・`-v`/`--verbose`・`--ignore-fail-on-non-empty`）と `unlink` 専用（オプション無し＝空 `[]FlagSpec`）を
  新設する。`Flags` を分割しても `ToExtraction` は分岐させず、`rm`/`rmdir`/`unlink` は引き続き同一の `extractRemove`
  （`extractAllWrite` への委譲）を共有する（`cp`/`mv` の `extractCopyMove`・`mkdir`/`sponge` の `extractSimpleWrite` と同様）。
- [ ] `simpleWriteFlags()` を分割する。`mkdir` 専用（`-m`/`--mode`・`-p`/`--parents`・`-v`/`--verbose`・`-Z`/`--context`）と
  `sponge` 専用（`-a`/`--append` のみ）を新設する。`extractSimpleWrite` は共有を維持する。
- [ ] `copyMoveFlags()` を分割する。`cp` 専用（再帰・リンク・デリファレンス系を含む cp 実 CLI 集合）を残し、`mv` 専用（`-f`/`--force`・
  `-i`/`--interactive`・`-n`/`--no-clobber`・`-u`/`--update`・`-v`/`--verbose`・`-t`/`--target-directory`〔`ValueWrite`〕・
  `-T`/`--no-target-directory`・`-b`/`--backup`・`-S`/`--suffix`〔`ValueNonPath`〕・`--strip-trailing-slashes`・`-Z`/`--context` 等）を
  新設する。`mv` 専用には再帰・リンク・デリファレンス系（`-r`/`-R`/`-a`/`-s`/`-l`/`-L`/`-P`/`-H`/`-d`/`-x`）を含めない。`extractCopyMove` は `cp`/`mv` で共有を維持し、argv を直接走査して `preserveMeta` を求める処理（`-a`/`--archive`/`-p`/`--preserve` を見る）には触れない（[02 §3.2](02_architecture.md)）。

**個別宣言の修正（要件 §1.1 の確定分）:**
- [ ] `touch` から真偽フラグ `-p`/`--parents`・`-v`/`--verbose`・`-i` を削除する。値フラグ `-r`/`--reference`（`ArityRequired`/
  `ValueNonPath`）と他の実フラグは保存する（[02 §3.2](02_architecture.md)）。

**残コマンドの整合（付録 A に基づく）:**
- [ ] 残りの登録コマンド（`ln`・`install`・`shred`・`truncate`・`tee`・`mknod`・`tar`・`unzip`・`mount`・`umount`・`chmod`・
  `chown`・`chgrp`・`setfacl`・`chattr`・`sed`・`curl`・`wget`・`scp`・`rsync`）について、付録 A の是正（削除・追加・修正）を適用する。
  値フラグの追加・役割修正では実 CLI に即した `ValueRole`（path は `ValueWrite`/`ValueRead`、非 path は `ValueNonPath`）と `Arity` を
  付与する（[02 §5.2](02_architecture.md)）。`dd`・`find`・`sftp`（フラグ非宣言）は対象外。
- [ ] 特殊フラグの解釈依存（AC-03a）を保存する: `chown`/`chgrp` の `--reference`/`--from`（`ArityRequired`）、`ln` の `-s`/`--symbolic`
  および `-t`/`--target-directory`（`ArityRequired`/`ValueWrite`）、`sed` の `-e`/`-f`（`ArityRequired`）と `-i`/`--in-place`
  （`ArityOptional`）。これらの表記・アリティ・役割を変えない（[02 §3.2](02_architecture.md)）。

**フェーズ 2 完了ゲート:**
- [ ] `go test -tags test ./internal/runner/base/security/` を実行し、データ駆動メタテスト（`TestSpecCompleteness`・
  `TestArityInvariant`・`TestSpecNoDuplicateNames`・`TestEveryCommandHasExtractor`）が緑であることを確認する（AC-06）。

### フェーズ 3: 逸脱登録と肯定表明（テストコード）

**差分テストの意図的逸脱登録（`extraction_diff_test.go`）:**
- [ ] 既存 `diffFixtures` を**全 de-share コマンド**（`cp`/`mv`/`rm`/`rmdir`/`unlink`/`mkdir`/`sponge`）について機械的に監査し、
  フェーズ 2 で削除したフラグのトークンを含む既存入力を網羅特定する（少なくとも `diffFixtures["mv"]` の `{"-ra","s","d"}`。なお
  `diffFixtures["rm"]` の `-rf`/`-rfv` 等は `rm` が保持するフラグのため対象外）。残存 fixture に削除トークンが 1 つも含まれない
  ことを確認する。
- [ ] AC-02 の代表入力を `diffFixtures` へ追加する: `sponge -r FILE`・`mkdir -a DIR`・`touch -p FILE`・`unlink -r FILE`・
  `rmdir -r DIR`・`mv -s SRC DST`。各 de-share コマンドにつき削除フラグを含む**クラスタ形**（例 `mv -rf SRC DST`・`sponge -rv FILE`）も
  最低 1 つ追加する（[02 §7](02_architecture.md)）。削除フラグは整合後の `Flags` から外れ `diffCorpus` の自動生成対象でなくなるため、これらの
  fixture が当該乖離を差分テストに乗せる唯一の経路である。
- [ ] 上で特定・追加した乖離入力**すべて**を `diffExclusions` へ登録する（追加と登録は対で必須。登録漏れがあると
  `TestExtractionDifferential` が赤になる）。各述語は、既存 `isLongRecursionDeviation` と同じく argv の長さと全トークンを完全一致で
  判定する共有ヘルパ経由とする。`args[0]=="-r"` のような位置を問わない緩い一致をインラインで書かない。各述語には man ページ典拠を
  記す英語コメントを添える（AC-04、[02 §3.3](02_architecture.md)）。
- [ ] `go test -tags test -run TestExtractionDifferential ./internal/runner/base/security/` が緑であることを確認する（AC-04）。

**削除フラグ網羅メタテスト（`destination_zoning_parity_test.go`）:**
- [ ] 削除した過剰認識フラグを単一のソース集合（テスト内データ。例 `removedOverRecognizedFlags map[string][]string`）として定義し、
  各（コマンド×削除フラグ）入力が本番経路（`parseArgs`＋`ToExtraction`）で `recognized=false` になることを `range` 検証する新テスト
  `TestRemovedOverRecognizedFlagsFailClosed` を追加する。手書きの並行リストに依存せずソース集合を直接走査する（[02 §7](02_architecture.md)、AC-02/AC-05）。

**肯定側の回帰アサーション（`destination_zoning_parity_test.go`）:**
- [ ] `TestExtractionRegressionCases` に AC-02 代表入力の `recognized=false` を表明する `t.Run` 群を追加する（`runExtraction` で
  `extraction.recognized == false` を確認）。
- [ ] 値役割・アリティを path 役割へ／から変更した各フラグについて、捕捉値が `extraction.operands` に期待どおり現れる
  （または正しく現れない）ことを目印値で表明する `t.Run` を追加する（[02 §5.2](02_architecture.md)、AC-05）。とくに既存の path 運搬フラグ
  （`curl` `-o`〔`ValueWrite`〕/`-T`〔`ValueRead`〕・`wget` `-O`/`-P`〔`ValueWrite`〕/`--post-file`〔`ValueRead`〕・`install` `-t`〔`ValueWrite`〕・
  `tar` `-f`/`-C`/`--directory`/`--one-top-level`〔`ValueWrite`〕・`ln` `-t`〔`ValueWrite`〕）の役割・アリティを付録 A の是正で変更する場合は、
  当該フラグのオペランド出現を必ず再固定する（格下げによる fail-open を防ぐ）。

**回帰の維持:**
- [ ] `go test -tags test ./internal/runner/base/security/` 全体が緑であることを確認する。`TestExtractionRegressionCases`（既存の
  `chown --from`/`sed -e`/`ln symbolic` 等の解釈保存サブテストを含む）・`TestLocationResultParity`・`TestLocationResultFloors`・
  `TestFailClosed`・`destination_zoning_test.go`・`operand_path_resolver_test.go` が緑であること（AC-03a/AC-06/AC-07）。
- [ ] 既存テストの期待値を変更した場合は、その入力が安全側挙動変化に該当する根拠を本書「AC-07 根拠記録」へ記す（無根拠の変更は不適合）。

### フェーズ 4: 安全性確認と非機能

- [ ] すべての挙動変化が安全側であることを確認・記録する（AC-05）: (a) 過剰認識除去はすべて `recognized=true→false`、(b) 宣言漏れ追加・
  役割修正で path 値の取りこぼし（fail-open）が無いことを、`TestRemovedOverRecognizedFlagsFailClosed` と肯定アサーションで確認する。
- [ ] 各意図的逸脱の理由（どのコマンドのどのフラグで何がどう変わるか）が `diffExclusions` の英語コメントと本書に記録されていることを
  確認する（AC-04）。
- [ ] `make fmt` → `make test` → `make lint` を実行し、すべて成功することを確認する（NF-001）。

## 3. 実装順序とマイルストーン

| マイルストーン | 内容 | 完了条件 |
|---|---|---|
| M1 | フェーズ 1: 典拠確定 | 付録 A が全コマンド分の是正内容を典拠付きで記載 |
| M2 | フェーズ 2: `flag_spec.go` 整合 | 共有ヘルパ分割＋全コマンド整合、メタテスト緑（AC-06） |
| M3 | フェーズ 3: テスト更新 | 逸脱登録・網羅メタテスト・肯定アサーション追加、全テスト緑（AC-02/AC-04/AC-07） |
| M4 | フェーズ 4: 確認 | 安全側確認の記録＋`make fmt`/`make test`/`make lint` 緑（AC-05/NF-001） |

> 各コマンドの整合は独立に進められ、差分テスト（登録済み逸脱以外は緑）・メタテスト・既存挙動テストの緑をコマンド単位のゲートとする
> （[02 §8](02_architecture.md)）。ただし argv を直接走査して決まる信号（`preserveMeta` 等）を共有する `cp`/`mv` では [02 §3.2](02_architecture.md) の不変条件を守る。

## 4. テスト戦略

- **メタテスト（AC-06）**: 既存のデータ駆動メタテスト群（`flag_spec_test.go`）を無改変で緑に保つ。値フラグの役割付け忘れ
  （`ValueUnset`）は `TestSpecCompleteness` が検出する。
- **削除フラグ網羅（AC-02/AC-05）**: 新規 `TestRemovedOverRecognizedFlagsFailClosed` が、削除した全フラグについて本番経路で
  `recognized=false` を機械検証する（ソース集合 range）。
- **肯定回帰（AC-02/AC-03/AC-03a/AC-05）**: `destination_zoning_parity_test.go` の `TestExtractionRegressionCases` に、AC-02 の
  `recognized=false` と役割修正のオペランド出現を目印値で固定する。`TestArityInvariant` がアリティ整合を担保する。
- **差分テスト（AC-04/AC-05/AC-07）**: `TestExtractionDifferential` が登録逸脱を除く全コーパスで凍結オラクルと一致することを担保。
  単独形に加え削除フラグのクラスタ形をコーパスに含める。
- **既存挙動（AC-07）**: `TestLocationResultParity`・`TestLocationResultFloors`・`TestFailClosed`・`destination_zoning_test.go`・
  `operand_path_resolver_test.go` を原則無改変で緑に保つ。
- **非機能（NF-001/NF-003）**: `make fmt`/`make test`/`make lint` 緑。決定性・read-only は 0144 の静的ガードが継続検証する（本タスクは
  純データ編集で新たな環境依存を導入しない）。

新規のテストヘルパファイルやモックは不要（既存の `runExtraction`／`classify`／`zoningInput` 等のパッケージ内ヘルパを再利用する。
[test_organization.md](../../dev/developer_guide/test_organization.md) の分類 B に該当する追加は発生しない）。

## 5. リスク管理

| リスク | 影響 | 緩和策 |
|---|---|---|
| 典拠誤り（man ページ解釈ミス）で実 CLI と不一致 | 過剰認識/宣言漏れの残存 | 付録 A に典拠を明記し、削除分は `TestRemovedOverRecognizedFlagsFailClosed`、追加・修正分は肯定アサーションで検証 |
| 役割の誤分類（path を `ValueNonPath` と誤る） | fail-open（path 無評価通過） | メタテストは `ValueUnset` のみ検出するため、役割変更フラグは肯定アサーションでオペランド出現を固定（[02 §5.2](02_architecture.md)） |
| argv を直接走査して決まる信号の誤削除 | `cp`/`mv` のメタ保持 High 下限取りこぼし | `destination_zoning_spec.go` に触れない方針を厳守（[02 §3.2](02_architecture.md)）。差分テストが乖離を検出 |
| 既存 `diffFixtures` の削除フラグ入力の見落とし | 差分テスト赤 or 退行の隠蔽 | フェーズ 3 で既存 `diffFixtures` を監査し乖離入力を網羅登録 |

## 6. 実装チェックリスト

- [ ] フェーズ 1: 付録 A（典拠表）確定
- [ ] フェーズ 2: 共有ヘルパ 3 種分割（`removeFlags`/`simpleWriteFlags`/`copyMoveFlags`）
- [ ] フェーズ 2: `touch` の `-p`/`-v`/`-i` 削除
- [ ] フェーズ 2: 残コマンド整合（付録 A）
- [ ] フェーズ 2: メタテスト緑ゲート
- [ ] フェーズ 3: `diffFixtures` 追加（AC-02 代表＋クラスタ形）と既存監査
- [ ] フェーズ 3: `diffExclusions` 厳密一致登録
- [ ] フェーズ 3: `TestRemovedOverRecognizedFlagsFailClosed` 追加
- [ ] フェーズ 3: 肯定アサーション追加（AC-02／役割修正）
- [ ] フェーズ 3: 全テスト緑
- [ ] フェーズ 4: 安全側確認の記録
- [ ] フェーズ 4: `make fmt`/`make test`/`make lint` 緑

## 7. 受け入れ基準の検証（Acceptance Criteria Verification）

検証種別: `test`=実行可能テスト（誤挙動で赤）、`static`=`rg`/コンパイル等の静的検査、`manual`=PR/レビュー観察。

| AC | 検証種別 | 検証場所・コマンド | 合格条件 |
|---|---|---|---|
| AC-01（全コマンド実 CLI 整合） | static | 付録 A に全 `commandFlagSpecs` 登録コマンドの是正内容と典拠が記載されていること。`rg -n 'valueFlag|boolFlag|recursiveFlag|optionalFlag' internal/runner/base/security/flag_spec.go` で宣言を付録 A と突き合わせ | 全コマンドが付録 A の典拠と一致 |
| AC-01 | test | `internal/runner/base/security/flag_spec_test.go::TestSpecCompleteness`、同 `::TestArityInvariant`、同 `::TestSpecNoDuplicateNames` | 緑 |
| AC-02（過剰認識除去の表明） | test | `internal/runner/base/security/destination_zoning_parity_test.go::TestRemovedOverRecognizedFlagsFailClosed` | 削除フラグ全件で `recognized=false`（`sponge -r`/`mkdir -a`/`touch -p`/`unlink -r`/`rmdir -r`/`mv -s` を含む） |
| AC-02 | test | `internal/runner/base/security/destination_zoning_parity_test.go::TestExtractionRegressionCases`（AC-02 代表入力の `t.Run` 群） | 各代表入力で `recognized=false` |
| AC-03（表記・アリティ・役割整合） | test | `internal/runner/base/security/flag_spec_test.go::TestArityInvariant`、`::TestSpecCompleteness` | 緑（引数付きフラグは具体的 `ValueRole`、`ArityNone` は `ValueUnset`） |
| AC-03a（解釈ロジック保存） | test | `internal/runner/base/security/destination_zoning_parity_test.go::TestExtractionRegressionCases`（`chown --from keeps spec positional…`／`sed -e script not confused…`／`ln symbolic vs hard link…` サブテスト） | 無改変で緑 |
| AC-04（意図的逸脱の登録機構） | test | `internal/runner/base/security/extraction_diff_test.go::TestExtractionDifferential` | 登録逸脱以外の全コーパスで緑 |
| AC-04 | static | `rg -n 'args\[[0-9]' internal/runner/base/security/extraction_diff_test.go` で、`diffExclusions` の述語が共有の完全一致ヘルパを介さずに argv 要素を直接添字参照するインライン緩い一致を書いていないこと（ヒットは完全一致ヘルパ内のみであること）。各述語に典拠コメントがあることは目視確認 | 共有ヘルパ外の添字参照ヒットが無い |
| AC-05（安全側の確認） | test | `destination_zoning_parity_test.go::TestRemovedOverRecognizedFlagsFailClosed`（true→false 方向）＋役割修正の肯定アサーション（オペランド出現） | 緑 |
| AC-05 | manual | 本書「AC-07 根拠記録」と `diffExclusions` コメントに各変化の安全側根拠が記載されていること | 全逸脱に根拠あり |
| AC-06（完全性・不変条件の維持） | test | `flag_spec_test.go::TestSpecCompleteness`／`::TestSpecNoDuplicateNames`／`::TestEveryCommandHasExtractor`／`::TestArityInvariant`／`::TestAliasAddition` | 全件緑 |
| AC-07（既存挙動の維持） | test | `internal/runner/base/security/destination_zoning_test.go` 全体、`operand_path_resolver_test.go` 全体、`destination_zoning_parity_test.go::TestLocationResultParity`／`::TestLocationResultFloors`／`::TestFailClosed` | 意図的に変える分を除き緑（変更分は根拠記録あり） |
| NF-001 | static | `make fmt`（差分なし）→ `make test`（緑）→ `make lint`（緑） | 全成功 |

### AC-07 根拠記録

（フェーズ 3 で既存テストの期待値を変更した場合のみ記入する。各エントリに「コマンド・入力・変更前後・安全側である根拠」を記す。
現時点で既存代表入力は実 CLI フラグのみを用いるため、期待値変更は想定しない。）

## 8. 成功基準

- AC-01〜AC-07 が test/static で緑（§7）。
- 要件 §1.1 の代表的な過剰認識（`sponge`/`mkdir`/`touch`・`rmdir`/`unlink`・`mv`）が解消され、全対象コマンドのフラグ仕様が付録 A の
  典拠と整合する。
- すべての挙動変化が安全側であることが差分テストの意図的逸脱と肯定アサーションで明示・記録されている。
- `make fmt`/`make test`/`make lint` が成功する（NF-001）。

## 9. 次のステップ

- 本書を `approved` にした後、フェーズ 1（付録 A の典拠確定）から実装に着手する。
- 実装着手後は本書のチェックボックスを随時更新する。

---

## 付録 A: 実 CLI フラグ典拠表（フェーズ 1 で確定）

> 本表はフェーズ 1 の成果物であり、実装着手時に各コマンドの man ページ典拠で確定・追記する。下記の代表行は要件 §1.1 で典拠付きで
> 確定済みのもの。残コマンド（`ln`・`install`・`shred`・`truncate`・`tee`・`mknod`・`tar`・`unzip`・`mount`・`umount`・`chmod`・
> `chown`・`chgrp`・`setfacl`・`chattr`・`sed`・`curl`・`wget`・`scp`・`rsync`）はフェーズ 1 で行を追加する。

| コマンド | 是正の別 | 対象フラグ | 典拠 |
|---|---|---|---|
| `sponge` | 削除 | `-c`/`-h`/`-p`/`-v`/`-f`/`-i`/`-r`（実在は `-a` のみ） | moreutils `sponge(1)` |
| `mkdir` | 削除 | `-a`/`-c`/`-h`/`-f`/`-i`/`-r`（実在は `-m`/`-p`/`-v`/`-Z`） | GNU coreutils `mkdir(1)` |
| `touch` | 削除 | `-p`/`-v`/`-i` | GNU coreutils `touch(1)` |
| `unlink` | 削除 | `rm` 由来の全オプション（実在はオプション無し） | GNU coreutils `unlink(1)` |
| `rmdir` | 削除 | `-r`/`-R`/`-f`/`-i`/`-I`/`-d` 等（実在は `-p`/`-v`/`--ignore-fail-on-non-empty`） | GNU coreutils `rmdir(1)` |
| `mv` | 削除 | `-r`/`-R`/`-a`/`-s`/`-l`/`-L`/`-P`/`-H`/`-d`/`-x`（再帰・リンク・デリファレンス系） | GNU coreutils `mv(1)` |
