# 要件定義書: コマンド Record 完全自己完結化

## 1. 背景と課題

### 1.1 現状のファイル構造

`record` コマンドはコマンドを解析した際に、以下の複数のファイルを生成する。

1. **コマンドの Record JSON**（`<hash-dir>/~path~to~command.json`）
   - コマンド自体のハッシュ、syscall 解析結果、シンボル解析結果
   - 依存共有ライブラリの参照情報（path + hash のみ。解析結果なし）
   - shebang インタープリターの参照情報（`raw_interpreter_path`、`interpreter_path`、`command_name`、`resolved_path` を保持。hash および解析結果なし）

2. **dynlib-analysis キャッシュ**（`<hash-dir>/dynlib-analysis/<encoded-lib-path>`）
   - 各共有ライブラリの syscall 解析結果、シンボル解析結果

### 1.2 課題

`runner` がコマンドを実行する際には、コマンドの Record JSON に加えて以下のファイルを個別に読み込む必要がある。

- **依存ライブラリの解析結果**: dynlib-analysis キャッシュを dep の数だけ読み込む
- **shebang インタープリターの Record**: インタープリターバイナリの hash および解析結果を持つ別 Record（インタープリター自体を `record` で別途処理した JSON）

これにより以下の課題が生じている。

- **例外処理の複雑さ**: dynlib キャッシュが存在しない場合（`ErrAnalysisNotFound`）は「高リスク扱い」とするフォールバックが必要であり、runner の実装が複雑になっている
- **I/O 回数の多さ**: コマンドが N 個の依存ライブラリを持つ場合、実行時に N+1 回以上のファイル読み込みが発生する
- **管理の煩雑さ**: Record ファイルを別環境に移植する際に dynlib キャッシュも一緒に持ち運ぶ必要がある

### 1.3 目標

コマンドの Record JSON を**完全自己完結型**にする。依存共有ライブラリおよび shebang インタープリターの解析結果を Record に埋め込み、`runner` が参照するファイルをコマンドの Record JSON 1ファイルのみとする。

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| Record | `record` コマンドが生成するコマンドの解析結果 JSON ファイル |
| deps | コマンドおよび shebang チェーン全バイナリが依存する共有ライブラリの統合リスト |
| shebang チェーン | スクリプトの shebang 行で指定されたインタープリター群。直接形式（例: `#!/bin/bash`）では1バイナリ、env 形式（例: `#!/usr/bin/env python3`）では env バイナリと解決済みバイナリの2バイナリ |
| dynlib キャッシュ | `record` の内部最適化用キャッシュ（`dynlib-analysis/` ディレクトリ）。`runner` は参照しない |

## 3. 機能要件

### F-001: deps フィールドへの解析結果埋め込み

現在の `dyn_lib_deps` フィールド（path + hash のみ）を `deps` フィールドに置き換える。`deps` はコマンドおよび shebang チェーン全バイナリの依存共有ライブラリを `path` を主キーとして dedup したリストであり（同一 path で hash が一致する場合に統合。不一致の場合は致命的エラー）、各ライブラリの解析結果（`syscall_analysis`、`symbol_analysis`）を含む。

**Acceptance Criteria:**

1. `deps` の各エントリに `soname`、`path`、`hash` に加えて `syscall_analysis`（nullable）と `symbol_analysis`（nullable）が含まれる
2. `deps` はコマンド自身と shebang チェーン全バイナリの依存ライブラリを合わせた dedup リスト（`path` を主キーとして重複排除。同一 path で hash が一致する場合に1エントリとして統合。同一 path で hash が不一致の場合は致命的エラーとして `record` を中断する）である
3. syscall wrapper ライブラリ（libc 等）および VDSO エントリは `deps` に含まれるが、解析フィールド（`syscall_analysis`、`symbol_analysis`）は null となる
4. 各 dep の解析中に発生した非致命的な警告は当該 `deps` エントリの `warnings` フィールドに記録される（現行の Record レベルの `analysis_warnings` に相当）

### F-002: shebang_chain フィールドへのインタープリター情報埋め込み

現在の `shebang_interpreter` フィールド（参照情報のみ）を `shebang_chain` フィールドに置き換える。`shebang_chain` は shebang チェーンを構成する各バイナリの情報（path、content_hash、syscall_analysis、symbol_analysis）をリストとして保持する。

**Acceptance Criteria:**

1. `shebang_chain` の各エントリに `path`（シンボリックリンク解決済み）、`content_hash`、`syscall_analysis`（nullable）、`symbol_analysis`（nullable）が含まれる
2. 直接形式の shebang（例: `#!/bin/bash`）では `shebang_chain` に1エントリが含まれ、`raw_path`（shebang 行の記述そのまま）が記録される
3. env 形式の shebang（例: `#!/usr/bin/env python3`）では `shebang_chain` に2エントリが含まれる。1つ目は env バイナリ（`raw_path` と `command_name` を保持）、2つ目は解決済みバイナリ
4. `shebang_chain` の各バイナリが依存するライブラリ（env バイナリの deps を含む）は F-001 の `deps` リストに dedup して含まれる

### F-003: runner から dynamicanalysis.Store 依存の除去

`runner` が dynlib 解析に使用する `dynamicanalysis.Store` インターフェースを除去し、`NetworkAnalyzer` が Record の `deps` フィールドから直接解析結果を読み込むよう変更する。shebang インタープリターの解析結果も同様に `shebang_chain` フィールドから直接読み込む。これにより `runner` が参照するファイルはコマンドの Record JSON のみとなる。

**Acceptance Criteria:**

1. `NetworkAnalyzer` の依存から `dynamicanalysis.Store` が除去される
2. `runner` は dynlib 解析結果を Record の `deps` フィールドから直接取得する
3. dynlib キャッシュファイル（`dynlib-analysis/`）が存在しなくても `runner` が正常に動作する
4. `ErrAnalysisNotFound` による「高リスクフォールバック」処理が `runner` から除去される
5. shebang インタープリターの解析結果（hash、syscall_analysis、symbol_analysis）も Record の `shebang_chain` から直接取得し、インタープリターの別 Record ファイルを参照しない
6. `deps` エントリの `syscall_analysis` および `symbol_analysis` が null であり、かつ syscall wrapper（libc 等）や VDSO ではない場合、`runner` は解析データ欠落として実行をエラー終了する（fail-closed）。高リスク扱いへのフォールバックは行わない

### F-004: -debug-info 時のみ dep 由来情報を記録

`-debug-info` フラグが指定された場合のみ、Record の `debug` フィールドに各 dep の由来情報（どのバイナリからの依存か）を記録する。

**Acceptance Criteria:**

1. `-debug-info` なしの場合、`debug` フィールドは JSON に含まれない（`omitempty`）
2. `-debug-info` ありの場合、`debug.dep_sources` に各 dep の絶対パス → 由来バイナリ絶対パスのリストが記録される
3. `runner` は `debug` フィールドを解析・参照しない

### F-005: スキーマバージョンアップと Record の再生成

スキーマ変更に伴い `CurrentSchemaVersion` を更新し、旧バージョンの Record は `record` コマンドの再実行を要求する。

**Acceptance Criteria:**

1. `CurrentSchemaVersion` が新しい値（22 以上）に更新される
2. 旧バージョン（21 以下）の Record を読み込んだ場合、`SchemaVersionMismatchError` が返される
3. `record` の再実行（スキーマ不一致時は `--force` 不要）により旧 Record を新フォーマットで上書き再生成できる

## 4. 非機能要件

### 4.1 パフォーマンス

- `runner` の Record 読み込みファイル数がコマンドごとに1ファイルになること（現在の N+1 から削減）

### 4.2 後方互換性

- 旧 Record は `record` の再実行により新フォーマットに移行できること
- `record` コマンドの外部インターフェース（フラグ、基本的な出力フォーマット）は維持すること
- dynlib-analysis キャッシュファイルは引き続き `record` が生成・利用すること（`runner` からは不可視）

### 4.3 データ整合性

- `deps` の dedup は `path` を主キーとする。同一 path で hash が一致する場合に1エントリとして統合し、同一 path で hash が不一致の場合は致命的エラーとして `record` を中断すること（どちらのバイナリが実際に使用されるか不明なため、不正なセキュリティポリシー適用を防ぐ）
- Record の書き出しはアトミックに行うこと（既存の動作を維持）

## 5. スコープ外

- `03_detailed_specification.md`（詳細仕様）および `04_implementation_plan.md`（実装計画）の作成
- shebang の多段チェーン（インタープリター自体がスクリプトである場合）のサポート
- 推移的な共有ライブラリ依存（ライブラリが依存するライブラリ）の解析
