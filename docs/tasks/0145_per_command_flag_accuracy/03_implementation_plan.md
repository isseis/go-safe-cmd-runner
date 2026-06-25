# コマンド別フラグ仕様の実 CLI 整合（過剰認識・宣言漏れの是正） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-25 |
| Review date | 2026-06-25 |
| Reviewer | isseis |
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

- [x] `commandFlagSpecs` の全登録コマンドについて、権威ある仕様を典拠として現行宣言フラグと突き合わせ、(a) 過剰認識（削除すべき
  真偽フラグ）、(b) 宣言漏れ（追加すべきフラグ）、(c) 表記・アリティ・値役割の不一致 を一覧化する。典拠は付録 A に明記する（AC-01）。
  - 典拠の所在: GNU coreutils コマンド（`cp`/`mv`/`rm`/`rmdir`/`unlink`/`ln`/`mkdir`/`touch`/`install`/`shred`/`truncate`/`mknod`/
    `chmod`/`chown`/`chgrp`/`dd`）は GNU coreutils マニュアル、`sed` は GNU sed マニュアル、`tar` は GNU tar マニュアル、`mount`/`umount`/
    `chattr` は util-linux / e2fsprogs マニュアル、`setfacl` は acl パッケージ man、`sponge` は moreutils man、`tee` は coreutils、
    `unzip` は Info-ZIP man、`curl`/`wget`/`scp`/`rsync`/`sftp` は各 man ページ。
- [x] 付録 A の各行に「削除/追加/修正」の別と典拠（man ページ名）を記す。要件 §1.1 が挙げた代表例（`sponge`/`mkdir`/`touch`・
  `rmdir`/`unlink`・`mv`）は既に典拠付きで確定しているため、後述のフェーズ 2 で確定入力として扱う。

**完了条件**: 全コマンドの是正内容（削除・追加・修正）が付録 A に典拠付きで確定している。

### PR-1 作成ポイント: flag citation inventory

**対象ステップ**: フェーズ 1（付録 A 典拠表の確定）

**推奨タイトル**: `docs(0145): record real-CLI flag citation inventory`

**レビュー観点**: 付録 A の各コマンドの典拠（man ページ）が正確か / 削除・追加・修正の別が要件 §1.1 と整合するか / `commandFlagSpecs` の全登録コマンドが網羅されているか

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/802）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ 2: 共有ヘルパ分割と実 CLI 整合（`flag_spec.go`）

付録 A に基づき `flag_spec.go` を編集する。意味づけ関数（`ToExtraction`）はコマンド間で共有のまま保つ（[02 §1.2](02_architecture.md)）。

**共有ヘルパの分割（de-share。過剰認識除去の構造的原因）:**
- [x] `removeFlags()` を分割する。`rm` 専用ヘルパ（実 CLI フラグ集合: `-r`/`-R`/`--recursive`・`-f`/`--force`・`-i`・`-I`・
  `--interactive`・`-v`/`--verbose`・`-d`/`--dir`・`--one-file-system`・`--preserve-root`/`--no-preserve-root` 等）を残し、
  `rmdir` 専用（`-p`/`--parents`・`-v`/`--verbose`・`--ignore-fail-on-non-empty`）と `unlink` 専用（オプション無し＝空 `[]FlagSpec`）を
  新設する。`Flags` を分割しても `ToExtraction` は分岐させず、`rm`/`rmdir`/`unlink` は引き続き同一の `extractRemove`
  （`extractAllWrite` への委譲）を共有する（`cp`/`mv` の `extractCopyMove`・`mkdir`/`sponge` の `extractSimpleWrite` と同様）。
- [x] `simpleWriteFlags()` を分割する。`mkdir` 専用（`-m`/`--mode`・`-p`/`--parents`・`-v`/`--verbose`・`-Z`/`--context`）と
  `sponge` 専用（`-a`/`--append` のみ）を新設する。`extractSimpleWrite` は共有を維持する。
- [x] `copyMoveFlags()` を分割する。`cp` 専用（再帰・リンク・デリファレンス系を含む cp 実 CLI 集合）を残し、`mv` 専用（`-f`/`--force`・
  `-i`/`--interactive`・`-n`/`--no-clobber`・`-u`/`--update`・`-v`/`--verbose`・`-t`/`--target-directory`〔`ValueWrite`〕・
  `-T`/`--no-target-directory`・`-b`/`--backup`・`-S`/`--suffix`〔`ValueNonPath`〕・`--strip-trailing-slashes`・`-Z`/`--context` 等）を
  新設する。`mv` 専用には再帰・リンク・デリファレンス系（`-r`/`-R`/`-a`/`-s`/`-l`/`-L`/`-P`/`-H`/`-d`/`-x`）を含めない。`extractCopyMove` は `cp`/`mv` で共有を維持し、argv を直接走査して `preserveMeta` を求める処理（`-a`/`--archive`/`-p`/`--preserve` を見る）には触れない（[02 §3.2](02_architecture.md)）。

**個別宣言の修正（要件 §1.1 の確定分）:**
- [x] `touch` から真偽フラグ `-p`/`--parents`・`-v`/`--verbose`・`-i` を削除する。値フラグ `-r`/`--reference`（`ArityRequired`/
  `ValueNonPath`）と他の実フラグは保存する（[02 §3.2](02_architecture.md)）。

**残コマンドの整合（付録 A に基づく）:**
- [x] 残りの登録コマンド（`ln`・`install`・`shred`・`truncate`・`tee`・`mknod`・`tar`・`unzip`・`mount`・`umount`・`chmod`・
  `chown`・`chgrp`・`setfacl`・`chattr`・`sed`・`curl`・`wget`・`scp`・`rsync`）について、付録 A の是正（削除・追加・修正）を適用する。
  値フラグの追加・役割修正では実 CLI に即した `ValueRole`（path は `ValueWrite`/`ValueRead`、非 path は `ValueNonPath`）と `Arity` を
  付与する（[02 §5.2](02_architecture.md)）。`dd`・`find`・`sftp`（フラグ非宣言）は対象外。
- [x] 特殊フラグの解釈依存（AC-03a）を保存する: `chown`/`chgrp` の `--reference`/`--from`（`ArityRequired`）、`ln` の `-s`/`--symbolic`
  および `-t`/`--target-directory`（`ArityRequired`/`ValueWrite`）、`sed` の `-e`/`-f`（`ArityRequired`）と `-i`/`--in-place`
  （`ArityOptional`）。これらの表記・アリティ・役割を変えない（[02 §3.2](02_architecture.md)）。

**フェーズ 2 完了ゲート:**
- [x] `go test -tags test ./internal/runner/base/security/` を実行し、データ駆動メタテスト（`TestSpecCompleteness`・
  `TestArityInvariant`・`TestSpecNoDuplicateNames`・`TestEveryCommandHasExtractor`）が緑であることを確認する（AC-06）。

### フェーズ 3: 逸脱登録と肯定表明（テストコード）

**差分テストの意図的逸脱登録（`extraction_diff_test.go`）:**
- [x] 既存 `diffFixtures` を**全 de-share コマンド**（`cp`/`mv`/`rm`/`rmdir`/`unlink`/`mkdir`/`sponge`）について機械的に監査し、
  フェーズ 2 で削除したフラグのトークンを含む既存入力を網羅特定する（少なくとも `diffFixtures["mv"]` の `{"-ra","s","d"}`。なお
  `diffFixtures["rm"]` の `-rf`/`-rfv` 等は `rm` が保持するフラグのため対象外）。残存 fixture に削除トークンが 1 つも含まれない
  ことを確認する。
- [x] AC-02 の代表入力を `diffFixtures` へ追加する: `sponge -r FILE`・`mkdir -a DIR`・`touch -p FILE`・`unlink -r FILE`・
  `rmdir -r DIR`・`mv -s SRC DST`。各 de-share コマンドにつき削除フラグを含む**クラスタ形**（例 `mv -rf SRC DST`・`sponge -rv FILE`）も
  最低 1 つ追加する（[02 §7](02_architecture.md)）。削除フラグは整合後の `Flags` から外れ `diffCorpus` の自動生成対象でなくなるため、これらの
  fixture が当該乖離を差分テストに乗せる唯一の経路である。
- [x] 上で特定・追加した乖離入力**すべて**を `diffExclusions` へ登録する（追加と登録は対で必須。登録漏れがあると
  `TestExtractionDifferential` が赤になる）。各述語は、既存 `isLongRecursionDeviation` と同じく argv の長さと全トークンを完全一致で
  判定する共有ヘルパ経由とする。`args[0]=="-r"` のような位置を問わない緩い一致をインラインで書かない。各述語には man ページ典拠を
  記す英語コメントを添える（AC-04、[02 §3.3](02_architecture.md)）。
- [x] `go test -tags test -run TestExtractionDifferential ./internal/runner/base/security/` が緑であることを確認する（AC-04）。

**削除フラグ網羅メタテスト（`destination_zoning_parity_test.go`）:**
- [x] 削除した過剰認識フラグを単一のソース集合（テスト内データ。例 `removedOverRecognizedFlags map[string][]string`）として定義し、
  各（コマンド×削除フラグ）入力が本番経路（`parseArgs`＋`ToExtraction`）で `recognized=false` になることを `range` 検証する新テスト
  `TestRemovedOverRecognizedFlagsFailClosed` を追加する。手書きの並行リストに依存せずソース集合を直接走査する（[02 §7](02_architecture.md)、AC-02/AC-05）。

**肯定側の回帰アサーション（`destination_zoning_parity_test.go`）:**
- [x] `TestExtractionRegressionCases` に AC-02 代表入力の `recognized=false` を表明する `t.Run` 群を追加する（`runExtraction` で
  `extraction.recognized == false` を確認）。
- [x] 値役割・アリティを path 役割へ／から変更した各フラグについて、捕捉値が `extraction.operands` に期待どおり現れる
  （または正しく現れない）ことを目印値で表明する `t.Run` を追加する（[02 §5.2](02_architecture.md)、AC-05）。とくに既存の path 運搬フラグ
  （`curl` `-o`〔`ValueWrite`〕/`-T`〔`ValueRead`〕・`wget` `-O`/`-P`〔`ValueWrite`〕/`--post-file`〔`ValueRead`〕・`install` `-t`〔`ValueWrite`〕・
  `tar` `-f`/`-C`/`--directory`/`--one-top-level`〔`ValueWrite`〕・`ln` `-t`〔`ValueWrite`〕）の役割・アリティを付録 A の是正で変更する場合は、
  当該フラグのオペランド出現を必ず再固定する（格下げによる fail-open を防ぐ）。
  *付録 A で役割未変更のため、既存の `TestExtractionRegressionCases` および `TestLocationResultParity` がカバーするアサーションで十分。*

**回帰の維持:**
- [x] `go test -tags test ./internal/runner/base/security/` 全体が緑であることを確認する。`TestExtractionRegressionCases`（既存の
  `chown --from`/`sed -e`/`ln symbolic` 等の解釈保存サブテストを含む）・`TestLocationResultParity`・`TestLocationResultFloors`・
  `TestFailClosed`・`destination_zoning_test.go`・`operand_path_resolver_test.go` が緑であること（AC-03a/AC-06/AC-07）。
- [x] 既存テストの期待値を変更した場合は、その入力が安全側挙動変化に該当する根拠を本書「AC-07 根拠記録」へ記す（無根拠の変更は不適合）。

### フェーズ 4: 安全性確認と非機能

- [x] すべての挙動変化が安全側であることを確認・記録する（AC-05）: (a) 過剰認識除去はすべて `recognized=true→false`、(b) 宣言漏れ追加・
  役割修正で path 値の取りこぼし（fail-open）が無いことを、`TestRemovedOverRecognizedFlagsFailClosed` と肯定アサーションで確認する。
- [x] 各意図的逸脱の理由（どのコマンドのどのフラグで何がどう変わるか）が `diffExclusions` の英語コメントと本書に記録されていることを
  確認する（AC-04）。
- [x] `make fmt` → `make test` → `make lint` を実行し、すべて成功することを確認する（NF-001）。

### PR-2 作成ポイント: flag-spec real-CLI alignment and test lock-in

**対象ステップ**: フェーズ 2（`flag_spec.go` 整合）/ フェーズ 3（逸脱登録と肯定表明）/ フェーズ 4（安全性確認と非機能）

**推奨タイトル**: `feat(0145): align command flag specs to real CLI`

**レビュー観点**: 共有ヘルパ分割後も `ToExtraction` が `cp`/`mv`・`rm`/`rmdir`/`unlink`・`mkdir`/`sponge` で共有を維持しているか / `touch`（個別宣言）から真偽フラグ `-p`/`-v`/`-i` のみが削除され値フラグ `-r`（`ArityRequired`/`ValueNonPath`）が保存されているか / 削除フラグが真偽フラグに限られ過剰認識除去が fail-closed 方向（`recognized=true→false`）か / 役割変更フラグのオペランド出現が肯定アサーションで再固定され fail-open を防いでいるか / 差分テストの逸脱登録が全トークン完全一致で無関係入力を巻き込まないか

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## 3. 実装順序とマイルストーン

| マイルストーン | 内容 | 完了条件 |
|---|---|---|
| M1 | フェーズ 1: 典拠確定 | 付録 A が全コマンド分の是正内容を典拠付きで記載 |
| M2 | フェーズ 2: `flag_spec.go` 整合 | 共有ヘルパ分割＋全コマンド整合、メタテスト緑（AC-06） |
| M3 | フェーズ 3: テスト更新 | 逸脱登録・網羅メタテスト・肯定アサーション追加、全テスト緑（AC-02/AC-04/AC-07） |
| M4 | フェーズ 4: 確認 | 安全側確認の記録＋`make fmt`/`make test`/`make lint` 緑（AC-05/NF-001） |

> 各コマンドの整合は独立に進められ、差分テスト（登録済み逸脱以外は緑）・メタテスト・既存挙動テストの緑をコマンド単位のゲートとする
> （[02 §8](02_architecture.md)）。ただし argv を直接走査して決まる信号（`preserveMeta` 等）を共有する `cp`/`mv` では [02 §3.2](02_architecture.md) の不変条件を守る。

### 3.2 PR 構成

| PR | 対象フェーズ | 主な変更内容 |
|---|---|---|
| PR-1 | フェーズ 1 | 付録 A: 全登録コマンドの実 CLI フラグ典拠表（削除・追加・修正の別と man ページ典拠）。ドキュメントのみ。 |
| PR-2 | フェーズ 2〜4 | `flag_spec.go` の実 CLI 整合（共有ヘルパ分割・全コマンド是正）、差分テストの逸脱登録・削除フラグ網羅メタテスト・肯定アサーション、安全側確認と NF ゲート。 |

> PR-2 がフェーズ 2〜4 を 1 つにまとめる理由: フラグ削除の瞬間に既存 `diffFixtures["mv"]` が凍結オラクルと乖離して
> `TestExtractionDifferential` が赤になるため、フラグ変更（フェーズ 2）と逸脱登録（フェーズ 3）は同一 PR でなければ
> グリーンゲートを満たせない。挙動変更はそれを証明するテスト（フェーズ 3）・安全側確認（フェーズ 4）と一体で出すのが
> レビュー単位として適切である。本番変更が単一ファイル（`flag_spec.go`）に閉じるため、PR-2 を分割せず 1 つの整合変更として扱う。

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

- [ ] PR-1 マージ済み（フェーズ 1: 付録 A 典拠表の確定）
- [ ] PR-2 マージ済み（フェーズ 2〜4: `flag_spec.go` 整合・共有ヘルパ分割・`touch` の `-p`/`-v`/`-i` 削除・差分テスト逸脱登録・`TestRemovedOverRecognizedFlagsFailClosed`・肯定アサーション・安全側確認・NF ゲート）

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

## 付録 A: 実 CLI フラグ典拠表（フェーズ 1 成果物）

各コマンドの宣言フラグ（`flag_spec.go` 現行）を実 CLI と突き合わせた結果。

**典拠の方針（和集合）**: 本タスクの「実 CLI」は GNU coreutils と uutils（Rust 版。Ubuntu 26.04 以降の既定）の**和集合**とする。
いずれか一方の実装に存在するフラグは「実在」とみなして宣言を残す／追加し、**どちらの実装にも存在しないフラグのみ**を過剰認識として
削除する。これは本タスクの目的（どの実 CLI にも無い共有ヘルパ由来のゴミフラグの除去）に最も忠実で、GNU/uutils いずれのホストでも
誤分類しない。§A.1 の削除対象（`sponge -r`・`mkdir -a` 等）は GNU・uutils のどちらにも存在しないため、和集合方針でも削除が成立する。

**ground truth**: 当環境のインストールは混在しており、`cp`/`mv`/`rm` は GNU coreutils 9.7、`rmdir`/`unlink`/`ln`/`mkdir`/`touch`/
`install`/`shred`/`truncate`/`mknod`/`chmod`/`chown`/`chgrp`/`dd`/`tee` は uutils 0.8.0 の `--help` を採取した（`tar` は GNU tar 1.35、
`mount`/`umount` は util-linux、`curl` は curl）。一方の実装しか手元に無いコマンドは、他方を当該 man ページ／公式ドキュメントで補完する。
未インストール（`sponge`/`unzip`/`setfacl`/`wget`/`scp`/`rsync`/`sftp`/`chattr`）は man ページを典拠とする。実装間で差のあるフラグは
和集合の方針に従い、由来（GNU/uutils）を備考に記す。なお `--help`/`--version`（および uutils の `-h`＝`--help` 等の短縮形）はファイル操作の
安全性に無関係なため、既存方針どおり宣言しない。したがって `mkdir` の削除対象 `-h` は現行宣言の `-h`＝`--no-dereference`（別名の誤宣言）を
指し、help の `-h` ではない。

**列の意味**: 「削除」＝実 CLI に無い宣言フラグ（過剰認識。除去対象、すべて真偽フラグで fail-closed 方向）。「追加/修正」＝
実 CLI にあるが未宣言、または別名・アリティ・役割の誤り（既存 `ToExtraction` で正しく扱えるデータのみの是正）。「備考（スコープ外）」＝
追加に `ToExtraction` の解釈ロジック改修を要するため本タスク（データのみ）では扱わず、未知フラグとして fail-closed のまま残す
安全な漏れ。

**スコープ原則**: 真偽フラグ・非 path 値フラグの追加はデータのみで安全に行える。path を運ぶ値フラグの追加は `ToExtraction` が当該値を
オペランド化する必要があり、`destination_zoning_spec.go` 不変の制約に反するため本タスクでは追加せず備考に記す（未宣言＝fail-closed で安全）。

### A.1 共有ヘルパ由来の過剰認識（本タスクの主眼）

| コマンド | 削除（過剰認識） | 追加/修正（データのみ） | 典拠 |
|---|---|---|---|
| `mv` | `-r`/`-R`/`--recursive`・`-a`/`--archive`・`-d`・`-L`/`--dereference`・`-P`/`--no-dereference`・`-H`・`-s`/`--symbolic-link`・`-l`/`--link`・`-x`/`--one-file-system` | 追加: `-Z`/`--context`・`--strip-trailing-slashes`（真偽）。保持: `-t`/`--target-directory`(Write)・`-S`/`--suffix`(NonPath)・`-f`・`-i`・`-n`・`-u`・`-v`・`-b`/`--backup`・`-T`/`--no-target-directory` | `mv --help` |
| `rm` | `-p`/`--parents`・`--ignore-fail-on-non-empty`（`rmdir` 由来） | 保持: `-r`/`-R`/`--recursive`・`-f`/`--force`・`-i`・`-I`・`--interactive`・`-v`/`--verbose`・`-d`/`--dir`・`--one-file-system`。追加: `--preserve-root`/`--no-preserve-root`（真偽） | `rm --help` |
| `rmdir` | `-r`/`-R`/`--recursive`・`-f`/`--force`・`-i`・`-I`・`--interactive`・`-d`/`--dir`・`--one-file-system` | 保持: `-p`/`--parents`・`-v`/`--verbose`・`--ignore-fail-on-non-empty` | `rmdir --help` |
| `unlink` | 宣言フラグ全件（実 CLI にオプション無し）→ `Flags` を空にする | — | `unlink --help` |
| `mkdir` | `-a`・`-c`・`-h`・`-f`・`-i`・`-r` | 保持: `-m`/`--mode`(NonPath)・`-p`/`--parents`・`-v`/`--verbose`。追加: `-Z`/`--context`（真偽） | `mkdir --help` |
| `sponge` | `-c`・`-h`・`-p`・`-v`・`-f`・`-i`・`-r`（実在は `-a` のみ） | 保持: `-a` | moreutils `sponge(1)` |
| `touch` | `-p`/`--parents`・`-v`/`--verbose`・`-i` | 修正: `-a` の誤別名 `--append` を除去（`touch -a` に長形なし）。追加: `-m`（真偽）・`--time`(NonPath)。保持: `-r`/`--reference`(NonPath)・`-d`/`--date`(NonPath)・`-t`(NonPath)・`-a`・`-c`/`--no-create`・`-h`/`--no-dereference`・`-f` | `touch --help` |

> `cp` は `copyMoveFlags` の「残す側」。現行宣言（`-t`/`-S`/`-r`/`-R`/`-a`/`-f`/`-i`/`-n`/`-v`/`-u`/`-d`/`-L`/`-P`/`-H`/`-s`/`-l`/`-T`/`-b`/`-x`）は
> すべて実 CLI に実在（`cp --help`）。削除なし。`extractCopyMove` は `cp`/`mv` で共有を維持し、`mv` 専用集合のみ新設する。

### A.2 個別宣言コマンド

| コマンド | 削除（過剰認識） | 追加/修正（データのみ） | 備考（スコープ外の安全な漏れ） | 典拠 |
|---|---|---|---|---|
| `mknod` | `-v`/`--verbose`（実在せず） | 保持: `-m`/`--mode`(NonPath)・`-Z`/`--context`。修正: `-Z` は真偽/任意引数（現行 `ArityRequired` を見直し。非 path のため影響軽微） | — | `mknod --help` |
| `ln` | なし | 追加: `--logical`(`-L` 長形)・`--physical`(`-P` 長形)・`--backup`(`-b` 長形)（真偽）。保持: `-t`/`--target-directory`(Write)・`-s`/`--symbolic`・`-S`/`--suffix`(NonPath)・`-f`・`-n`・`-r`・`-v`・`-i`・`-T`・`-b`・`-L`・`-P` | — | `ln --help` |
| `install` | なし | 追加: `-P`/`--preserve-context`・`-U`/`--unprivileged`・`-Z`・`--context`・`--strip-program`（真偽/値）。修正: `-b` は真偽（`--backup` は任意引数。現行 `valueFlag` 見直し、非 path） | — | `install --help` |
| `shred` | なし | 追加: `--random-source`(NonPath)。修正: `-u`/`--remove` は任意引数（現行真偽。非 path） | — | `shred --help` |
| `truncate` | なし | なし（全宣言が実在） | — | `truncate --help` |
| `tee` | なし | なし（`-a`/`--append`・`-i`/`--ignore-interrupts`・`-p`・`--output-error` すべて実在） | — | `tee --help` |
| `chmod` | なし | 追加: `-H`・`-L`・`-P`（真偽。GNU・uutils 共通）。さらに `-h`/`--no-dereference`・`--dereference`（真偽）— これらは uutils 0.8.0 が持ち GNU `chmod` には無いが、和集合方針により保持/追加する | `--reference=RFILE`（参照ファイル値かつモード非フラグ引数を省く解釈変更）は `extractChmod` 改修を要するため追加せず。未宣言＝fail-closed | uutils 0.8.0 `chmod --help` ＋ GNU `chmod(1)` |
| `chown` / `chgrp` | なし | 追加: `--preserve-root`/`--no-preserve-root`（真偽）。保持: `--from`(NonPath)・`--reference`(NonPath)・`-R`/`--recursive`・`-v`・`-c`/`--changes`・`-f`/`--silent`/`--quiet`・`-h`/`--no-dereference`・`-H`・`-L`・`-P`・`--dereference` | — | `chown --help`・`chgrp --help`（`chgrp` も `--from`/`--reference` を実装。`ownerFlags` 共有は妥当） |
| `sed` | なし | なし（`-i`/`-e`/`-f`/`-l`/`-n`/`-r`/`-E`/`-s`/`-z`/`-u`・`--posix`/`--debug`/`--sandbox`/`--follow-symlinks` すべて実在） | — | `sed --help` |
| `tar` | なし（現行 `tarFlagSet` の宣言は実在） | 照合のうえ長形の漏れがあれば補完 | — | GNU tar 1.35 `tar --help` |
| `mount` / `umount` | なし（現行宣言は実在） | 照合のうえ過不足を補正 | — | util-linux `mount --help`・`umount --help` |
| `unzip` | 要照合 | Info-ZIP man と照合。現行（`-d`(Write)・`-x`(値)・`-o`/`-n`/`-q`/`-qq`/`-v`/`-j`/`-a`/`-u`/`-f`、`-l`/`-Z` 意図的未宣言）の過不足を補正 | — | Info-ZIP `unzip(1)` |
| `setfacl` | 要照合 | acl man と照合 | — | acl `setfacl(1)` |
| `chattr` | 要照合 | e2fsprogs man と照合（現行 `chattrFlagSet`） | — | e2fsprogs `chattr(1)` |
| `curl` / `wget` / `scp` / `rsync` | 要照合（0144 で手作りした集合のため過剰認識リスクは低い） | 各 man と照合し過不足を補正。path 運搬値フラグ（`curl -o`/`-T`・`wget -O`/`-P`/`--post-file`）の役割・アリティは現行どおり保存 | path 運搬値フラグの新規追加で `ToExtraction` 改修が要るものは追加せず備考化 | curl `--help all`・`wget(1)`・`scp(1)`・`rsync(1)` |

> `dd`・`find`・`sftp` はフラグを宣言せず（`dd` は `if=`/`of=` の key=value、`find` は述語の位置解析、`sftp` はバッチ/対話）、
> 本タスクの対象外。
