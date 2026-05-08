# シェバンインタプリタ解析結果のセッション内キャッシュ 要件定義書

## 1. 概要

### 1.1 背景

現在の `record` コマンドは、シェバンを持つスクリプトファイルに対して実行されると、
スクリプト本体に加えてシェバン行に書かれたインタプリタバイナリも内部的に解析する。
具体的には [`Validator.populateShebangData`](../../../internal/filevalidator/validator.go) が
`ShebangChain` の各エントリ（インタプリタ本体、env 経由ならその先の解決済みコマンド）に対して
[`Validator.analyzeRecordTarget`](../../../internal/filevalidator/validator.go) を呼び出し、
以下を毎回実行している。

- ELF/Mach-O dynlib 依存解析（`analyzeDynLibDeps`）
- ネットワーク／dynamic-load シンボル解析（`binaryAnalyzer.AnalyzeNetworkSymbols`）
- ELF syscall 解析および Mach-O syscall 解析

`record` は複数ファイルをまとめて受け取れる仕様（`record <file1> <file2> ...`）であり、
複数のスクリプトが同一インタプリタ（例: `/usr/bin/python3`、`/bin/bash`）を共有しているケースでは、
プロセス内で同じインタプリタの解析を引数の数だけ繰り返している。

一方、`.so` ライブラリ単位の解析については以下の二段キャッシュが既に存在する。

- セッション内（プロセス内）: `Validator.processedLibAnalysis` マップ
- セッション横断（永続）: `Validator.dynamicLibAnalysisStore`

シェバンチェーンに含まれるインタプリタバイナリは、上記いずれのキャッシュも経由しない。
インタプリタの推移的な `.so` 依存は永続ストアでキャッシュされるが、インタプリタ本体の
ELF パース・dynlib 解析・シンボル解析・syscall 解析は毎回実行される。

### 1.2 目的

`record` の単一プロセス実行内において、シェバンチェーン上のインタプリタバイナリ解析結果を
プロセス内で再利用可能にする。これにより、同一インタプリタを共有する複数スクリプトを
1 回の `record` コマンドでまとめて処理する場合の重複解析を排除する。

### 1.3 スコープ

**対象**:
- `populateShebangData` から呼び出される `analyzeRecordTarget` の結果再利用
- `Validator` インスタンス内（= `record` プロセス内）に閉じたインメモリキャッシュ
- ELF / Mach-O 両プラットフォームのインタプリタ

**対象外**:
- セッション横断（プロセス間）での永続キャッシュ。これは将来的に検討するが、本タスクでは扱わない
- スクリプト本体（解析対象ファイル）に対する `analyzeRecordTarget` 呼び出しのキャッシュ。
  スクリプトはハッシュも内容も毎回異なる前提のため対象外
- `analyzeOneLibrary`（共有ライブラリ解析）の改修。既存の二段キャッシュは現状維持
- シェバン解析（`shebang.Parse`）自体のキャッシュ。コストはバイナリ解析と比べ無視できるため対象外

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| シェバンチェーン | スクリプトのシェバンから辿られるインタプリタバイナリ列。直接形式（`#!/bin/sh`）では 1 要素、env 形式（`#!/usr/bin/env python3`）では env バイナリと解決済みコマンドの 2 要素 |
| インタプリタ解析結果 | `analyzeRecordTarget` がインタプリタバイナリに対して返す `*fileanalysis.Record`。`DynLibDeps` / `SymbolAnalysis` / `SyscallAnalysis` / `AnalysisWarnings` を含む |
| セッション内キャッシュ | 単一の `Validator` インスタンスが保持するインメモリのキャッシュ。プロセス終了で消える |
| キャッシュキー | キャッシュ取得に用いる値。インタプリタの絶対パスとプレフィックス付きハッシュ（`sha256:<hex>`）の組 |
| キャッシュヒット | 同一キーで以前に計算したインタプリタ解析結果を再利用できる状態 |

---

## 3. 機能要件

### 3.1 セッション内キャッシュの導入

#### FR-3.1.1: `Validator` インスタンスにインタプリタ解析結果キャッシュを保持する

`Validator` 構造体に、シェバンチェーン上のインタプリタバイナリに対する
`analyzeRecordTarget` の戻り値をキャッシュするインメモリマップを追加する。

- キー: インタプリタの絶対パスとプレフィックス付きハッシュ（`sha256:<hex>`）の組
- 値: `*fileanalysis.Record`（`analyzeRecordTarget` の戻り値そのもの）
- スコープ: `Validator` インスタンス（= 1 回の `record` プロセス）

既存の `processedLibAnalysis`（共有ライブラリ用、値型は `*dynamicanalysis.Result`）とは
値型が異なるため、別フィールドとして追加する。

#### FR-3.1.2: キャッシュ参照箇所

キャッシュの参照と更新は、`populateShebangData` 内のシェバンチェーンエントリ処理ループから行う。
`populateAnalysisRecord` の冒頭で実行されるスクリプト本体に対する `analyzeRecordTarget` 呼び出しは
キャッシュ対象外とする（1.3 スコープで対象外と定義）。

### 3.2 キャッシュ判定ルール

#### FR-3.2.1: キャッシュキーの一致判定

シェバンチェーンエントリに対して以下の手順でキャッシュを参照する。

1. インタプリタの絶対パスでハッシュを計算（`prefixedHashForPath`）。失敗時は
   従来どおり `populateShebangData` からエラーを上位へ伝播し、キャッシュには触れない
2. (絶対パス, ハッシュ) のキャッシュキーを構築
3. キーがキャッシュに存在すれば、保存された `*fileanalysis.Record` を再利用
4. 存在しない場合は `analyzeRecordTarget` を実行し、成功した場合のみキャッシュに格納する。
   失敗時はエラーを上位へ伝播し、キャッシュには格納しない

#### FR-3.2.2: ハッシュ不一致時の扱い

同一プロセス内では同一パスの再ハッシュが起きるたびに最新ハッシュをキーに用いるため、
ハッシュ変更（例: テスト中のファイル差し替え）が起きた場合は別キーとして
新たに解析が走る。古いキー側のエントリはキャッシュに残ってよい
（メモリ上のサイズは無視できるため明示的な無効化は行わない）。

### 3.3 キャッシュと既存処理の協調

#### FR-3.3.1: `depCollector` への登録

キャッシュヒット時も、シェバンチェーンエントリ自身（インタプリタ本体）の `LibEntry` 登録、
およびそのエントリの `DynLibDeps` を `depCollector.addEntries` 経由で登録する処理は
従来どおり実行する。これにより、キャッシュヒットの有無で最終的な
`record.DynLibDeps` の内容は変化しない。

#### FR-3.3.2: `analysisAggregate` への登録

キャッシュヒット時も、再利用した `*fileanalysis.Record` を
`aggregate.addRecord` で集約に登録する処理は従来どおり実行する。これにより、
`record.SymbolAnalysis` / `record.SyscallAnalysis` / `record.AnalysisWarnings` の
最終出力はキャッシュヒットの有無で変化しない。

#### FR-3.3.3: 既存ライブラリキャッシュとの独立性

本タスクで導入するインタプリタ解析結果キャッシュは、
既存の `processedLibAnalysis` および `dynamicLibAnalysisStore` とは独立に動作する。
`analyzeRecordTarget` 内部から呼ばれる `analyzeLibraries` 経由のライブラリ解析については、
従来の二段キャッシュがそのまま利用される。

### 3.4 観測性

#### FR-3.4.1: キャッシュ効果の検証可能性

テストにおいて、同一インタプリタを共有する複数スクリプトを処理した際に、
当該インタプリタに対する `analyzeRecordTarget` 経路の重複実行が抑制されたことを
検証可能とする。具体的な観測手段は `02_architecture.md` § 9 で確定するが、
プロダクションコードにテスト専用フラグや build tag による分岐を持ち込まない方法を採る。

ロギング自体は実装の自由度を確保するため要件としない（`AnalysisWarnings` の意味を
変える追加は禁止する）。

---

## 4. 非機能要件

### 4.1 パフォーマンス

- 同一インタプリタを共有する N 個のスクリプトを 1 回の `record` 呼び出しで処理する場合、
  インタプリタ自身の `analyzeRecordTarget` の実行回数は 1 回になる
  （シェバンチェーン要素数 × N 回ではない）

### 4.2 互換性

- 既存の `record` コマンドの出力（書き込まれる `fileanalysis.Record` の内容）は
  キャッシュ導入前後で完全に一致する
- 既存のスキーマバージョン（`fileanalysis.CurrentSchemaVersion = 22`）は変更しない
- 既存の永続ストア（`dynamicLibAnalysisStore`）のスキーマも変更しない

### 4.3 セキュリティ

- キャッシュ判定はインタプリタ絶対パスとハッシュの両方を照合するため、
  異なるバイナリの解析結果が誤って再利用されることはない
- キャッシュはプロセス内に閉じ、ディスクへの書き出しは行わない

### 4.4 メモリ消費

- キャッシュサイズはシェバンチェーンに登場するユニークなインタプリタ数に比例する。
  通常のユースケースでは数個〜十数個のオーダーであり、明示的な上限・LRU は設けない

---

## 5. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | 単一の `Validator` インスタンスで、同一インタプリタを共有する N 個（N ≥ 2）のスクリプトに対して `SaveRecord` を連続実行した場合、当該インタプリタに対する `analyzeRecordTarget` 経由の解析（dynlib / シンボル / syscall）の実行回数が 1 回に抑えられることをテストで検証できる |
| AC-2 | キャッシュ導入前後で、最終的に書き込まれる `fileanalysis.Record`（`DynLibDeps` / `SymbolAnalysis` / `SyscallAnalysis` / `AnalysisWarnings` / `ShebangChain`）の内容が完全に一致する |
| AC-3 | 同一パスでハッシュが異なる 2 つのインタプリタを順に処理した場合、それぞれ独立に `analyzeRecordTarget` が実行され、各々の解析結果が正しく `record` に反映される |
| AC-4 | env 形式シェバン（`#!/usr/bin/env python3`）について、env バイナリと解決済みコマンドの両方がキャッシュ対象として機能する |
| AC-5 | キャッシュヒット時も、シェバンチェーンエントリ自身の `LibEntry` 登録および `DynLibDeps` の `depCollector` への取り込みが実行される |
| AC-6 | `make fmt` / `go test -tags test -v ./...` / `make lint` がすべて成功する |
| AC-7 | キャッシュヒット時の挙動は、`processedLibAnalysis` / `dynamicLibAnalysisStore` を経由する共有ライブラリ解析の結果に影響を与えない |

---

## 6. 制約事項

1. キャッシュは `Validator` インスタンスに閉じる。プロセス間共有は本タスクでは扱わない
2. キャッシュ無効化（明示的なクリア）はサポートしない。`Validator` の生存期間 = キャッシュの生存期間
3. キャッシュサイズの上限は設けない。シェバンチェーン上のインタプリタ数は実用上有限のため

---

## 7. 想定外（Non-Goals）

- セッション横断永続キャッシュ（別タスクで検討）
- インタプリタ解析結果のディスク書き出し
- スクリプト本体のキャッシュ
- ライブラリ単位キャッシュへの本機能の統合
