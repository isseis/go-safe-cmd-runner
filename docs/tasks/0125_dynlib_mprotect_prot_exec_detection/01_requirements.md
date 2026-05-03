# 動的リンクライブラリ経由 `mprotect(PROT_EXEC)` 検出 要件定義書

## 1. 概要

### 1.1 背景

タスク 0123/0124 により、`record` は実行ファイルがリンクするライブラリ（推移的依存を含む）の
解析結果を保存し、`runner` はその結果を参照してネットワーク利用や動的ロードシンボルの
リスク判定を行える。

一方で、`mprotect(PROT_EXEC)` / `pkey_mprotect(PROT_EXEC)` のような
実行権限付与を伴う高リスク syscall については、実行ファイル本体での判定はあるが、
ライブラリ解析結果から `runner` 高リスク判定へ確実に伝播することを要件として明文化できていない。

### 1.2 目的

実行ファイルが直接 `mprotect(PROT_EXEC)` を呼ばない場合でも、
リンク先ライブラリ由来の同シグナルを `record` で保持し、`runner` が高リスクとして判定できるようにする。

### 1.3 スコープ

- 対象: Linux ELF（x86_64 / arm64）
- 対象: `record` のライブラリ解析結果生成、および `runner` の dynlib 解析結果参照ロジック
- 非対象: Mach-O 専用解析経路、関数到達可能性解析、動的ロード（dlopen）で実行時に追加される未記録ライブラリ

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| PROT_EXEC リスク | `mprotect` / `pkey_mprotect` の第3引数に実行権限フラグが含まれる、または否定できない状態 |
| ArgEvalResults | syscall 引数評価結果。`exec_confirmed` / `exec_unknown` / `exec_not_set` を持つ |
| ライブラリ解析結果 | `dynamicanalysis` ストアに保存される 1 ライブラリ単位の解析結果 |

---

## 3. 機能要件

### 3.1 `record` 側の保持

#### FR-3.1.1: ライブラリ解析での ArgEvalResults 保持

`record` がライブラリを syscall 解析した際、`AnalyzeSyscallsFromELF` が返す
`ArgEvalResults` を破棄せず、ライブラリ解析結果の `syscall_analysis.arg_eval_results` に保存すること。

#### FR-3.1.2: 条件付き保存

ライブラリ解析結果の `syscall_analysis` は、以下のいずれかを満たす場合に保存すること。

1. `detected_syscalls` が 1 件以上ある
2. `arg_eval_results` が 1 件以上ある

### 3.2 `runner` 側の判定

#### FR-3.2.1: dynlib 由来 PROT_EXEC リスクの高リスク化

`runner` の dynlib 解析結果参照時、各ライブラリの
`syscall_analysis.arg_eval_results` を評価し、
`mprotect` または `pkey_mprotect` が `exec_confirmed` または `exec_unknown` の場合、
当該コマンドを高リスクとして扱うこと。

#### FR-3.2.2: 既存判定との合成

FR-3.2.1 による高リスク化は、既存の以下判定と OR 合成で機能すること。

- dynlib のネットワーク syscall 検出
- dynlib のネットワークシンボル検出
- dynlib の動的ロードシンボル検出

### 3.3 後方互換

#### FR-3.3.1: 既存ネットワーク判定の非退行

本変更により、既存のネットワーク判定ロジックの結果が後退しないこと。

#### FR-3.3.2: 既存 mprotect 判定との整合

実行ファイル本体由来の `mprotect(PROT_EXEC)` 判定と、
ライブラリ由来の同判定を同一のリスク概念（高リスク）として扱うこと。

---

## 4. 非機能要件

### 4.1 パフォーマンス

- 追加処理は既存の `ArgEvalResults` 配列の走査のみとし、ライブラリ 1 件あたり O(n)（n は引数評価件数）であること
- 既存の dynlib ストア I/O 回数を増やさないこと

### 4.2 可観測性

- dynlib 由来の PROT_EXEC リスク検出時に、`cmd_path` と `dep_path` を含むログを出力できること

---

## 5. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | ライブラリ解析で `ArgEvalResults` が返る場合、ライブラリ解析結果の `syscall_analysis.arg_eval_results` に保存される |
| AC-2 | dynlib 解析結果に `mprotect` の `exec_confirmed` が含まれる場合、`runner` の判定結果が高リスクになる |
| AC-3 | dynlib 解析結果に `mprotect` の `exec_unknown` が含まれる場合、`runner` の判定結果が高リスクになる |
| AC-4 | `exec_not_set` のみの場合は PROT_EXEC 起因の高リスク化が発生しない |
| AC-5 | 既存の dynlib ネットワーク判定テストが回帰しない |
| AC-6 | 追加・変更した単体テストが `go test -tags test` で成功する |

---

## 6. テスト観点

- `filevalidator`:
  - ライブラリ解析結果に `ArgEvalResults` が保存されること
- `runner/base/security`:
  - dynlib 解析結果の `ArgEvalResults` が高リスク判定へ反映されること
- 回帰:
  - 既存の dynlib ネットワーク判定系テストが維持されること
