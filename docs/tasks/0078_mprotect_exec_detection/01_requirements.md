# `mprotect(PROT_EXEC)` 静的検出 要件定義書

## 1. 概要

### 1.1 背景

タスク 0070（ELF syscall 静的解析）では、`socket` や `connect` 等のネットワーク関連
syscall を静的に検出する仕組みを実装した。同じ静的解析の枠組みを使えば、
実行時に動的コードを読み込む可能性を示す `mprotect(PROT_EXEC)` パターンも
検出できる。

`dlopen` / `dlsym` / `dlvsym` は最終的に以下の syscall に帰結する：

1. `openat` — 共有ライブラリファイルを開く
2. `mmap` — ファイル内容をメモリにマップする
3. **`mprotect(PROT_READ | PROT_EXEC)`** — コードセグメントを実行可能にする
4. `close` — ファイルディスクリプタを閉じる

このうち `openat` / `mmap` / `close` は通常のファイル I/O でも発生するため識別力が低い。
一方 **`mprotect` で `PROT_EXEC` フラグを付与する操作**は、動的コード実行（共有ライブラリ
ロード、JIT コンパイル等）に特有のパターンであり、静的解析による検出シグナルとして有用である。

タスク 0070 で実装した後方スキャン（backward scan）は、syscall 命令直前の
`mov $imm, %eax` パターンから syscall 番号を抽出する。同じ手法を `mprotect`（syscall 番号 10）の
第3引数レジスタ（x86_64: `rdx`）に拡張することで、`prot` フラグの即値を静的に読み取ることができる。

### 1.2 目的

ELF バイナリの静的解析において、`mprotect(PROT_EXEC)` 呼び出しパターンを検出し、
動的コードロード（`dlopen` 等）の可能性をリスク指標として記録する。

### 1.3 スコープ

- **対象**: ELF バイナリ（x86_64 Linux、arm64 Linux）
- **対象**: タスク 0070 の syscall 静的解析の延長として実装する
- **対象外**: 非 ELF ファイル（スクリプト等）
- **対象外**: Mach-O バイナリ（タスク 0073 の範囲）
- **対象外**: `mprotect` の引数を実行時に動的に決定するかどうかの判断（静的解析の限界）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `PROT_EXEC` | `mprotect` の第3引数 `prot` フラグ。値は `0x4`。このフラグが設定されたメモリ領域はCPUで実行可能になる |
| `PROT_READ` | `mprotect` の `prot` フラグ。値は `0x1` |
| `PROT_WRITE` | `mprotect` の `prot` フラグ。値は `0x2` |
| 即値確定 | 後方スキャンにより引数レジスタへの即値設定命令が見つかり、`prot` フラグの値が静的に決定できた状態 |
| 引数不確定 | 後方スキャンで引数レジスタへの即値設定命令が見つからなかった、または制御フロー境界に到達した状態 |
| 動的コードロード | `dlopen` 等を使い、実行時に外部の共有ライブラリを読み込んでそのコードを実行すること |
| `ArgEvalResult` | 特定の syscall に対する引数値の静的評価結果を表す構造体。syscall 名・評価ステータス・詳細を保持する |

## 3. 機能要件

### 3.1 `mprotect(PROT_EXEC)` の検出

#### FR-3.1.1: `mprotect` syscall の特定

タスク 0070 の Pass 1（直接 `syscall` 命令の解析）において、
syscall 番号が `mprotect`（x86_64: 10、arm64: 226）であるエントリを識別できること。

#### FR-3.1.2: `prot` 引数の後方スキャン

`mprotect` syscall と判定されたエントリに対し、`prot` 引数レジスタ
（x86_64: `rdx`、arm64: `x2`）への設定命令を後方スキャンで探索できること。

スキャンルールはタスク 0070 FR-3.1.3 の syscall 番号取得（`rax`/`eax` 対象）と同一とし、
対象レジスタのみが異なる。x86_64 においては 64bit 形式（`mov $imm, %rdx`）および
32bit 形式（`mov $imm, %edx`）の両方を対象とすること。

スキャン範囲は既存定数 `defaultMaxBackwardScan`（`elfanalyzer` パッケージ、値: 50 命令）を流用する。
この上限は x86_64 / arm64 で共通であり、命令数（バイト数ではない）で計測する。

#### FR-3.1.3: `PROT_EXEC` フラグの判定

後方スキャンで `prot` フラグの即値が取得できた場合、
`value & 0x4`（`PROT_EXEC`）の有無を判定できること。

#### FR-3.1.4: 3段階の判定結果

`mprotect` syscall ごとに以下の3段階で判定結果を記録すること：

| 判定 | 条件 |
|------|------|
| `exec_confirmed` | `prot` 即値が取得でき、かつ `PROT_EXEC` フラグが立っている |
| `exec_unknown` | `prot` 即値が取得できなかった（引数不確定） |
| `exec_not_set` | `prot` 即値が取得でき、かつ `PROT_EXEC` フラグが立っていない |

### 3.2 解析結果への統合

#### FR-3.2.1: `ArgEvalResults` リストへの追加

`SyscallAnalysisResultCore`（`common` パッケージ）に、syscall 引数の静的評価結果を
格納する `ArgEvalResults []SyscallArgEvalResult` フィールドを追加すること。

`SyscallArgEvalResult` は以下のフィールドを持つ：

| フィールド | 型 | 内容 |
|---|---|---|
| `SyscallName` | `string` | syscall 名（例: `"mprotect"`） |
| `Status` | `string` | 評価ステータス（`"exec_confirmed"` / `"exec_unknown"` / `"exec_not_set"` の3値のみ）。実装では型付き文字列定数（`type SyscallArgEvalStatus string`）として定義し、任意文字列を防ぐこと |
| `Details` | `string` | 補足情報（省略可）。即値確定時は取得した `prot` 値の16進数表記（例: `"prot=0x5"`）、引数不確定時はスキャンを打ち切った理由（例: `"control flow instruction encountered"`）を格納する |

`mprotect` が検出された場合は当該エントリを `ArgEvalResults` に追加し、
`mprotect` が検出されなかった場合は `ArgEvalResults` にエントリを追加しない。

複数の `mprotect` syscall が検出された場合、最も高リスクのエントリ1件のみを `ArgEvalResults` に記録する。
リスクの優先順位は `exec_confirmed` > `exec_unknown` > `exec_not_set` とする。
同順位のエントリが複数ある場合はいずれか1件を記録する（実装依存）。

#### FR-3.2.2: `IsHighRisk` フィールドへの反映

`ArgEvalResults` の内容をもとに、既存の `IsHighRisk` フィールドを更新すること。
マッピングルールは第5節で定義する。`HasNetworkSyscalls` フィールドは影響を受けないこと。

### 3.3 保存と読み込み

#### FR-3.3.1: 解析結果の永続化

`ArgEvalResults` を既存の JSON 解析結果ファイルに追記する形で保存できること。
スキーマバージョンを更新し、旧バージョンの解析結果を無効化すること。

#### FR-3.3.2: 後方互換性

スキーマバージョン不一致時の既存の動作（解析結果を無効化して再解析を要求）を維持すること。

## 4. 非機能要件

### 4.1 パフォーマンス

#### NFR-4.1.1: 解析オーバーヘッド

`mprotect` エントリのみを対象とした追加スキャンであるため、
全体の解析時間への影響は軽微であること。

### 4.2 テスト可能性

#### NFR-4.2.1: ユニットテスト

x86_64 / arm64 それぞれについて、以下のバイト列パターンに対するユニットテストを実装すること：

**x86_64**

- `mov $0x7, %rdx` + `mprotect` syscall → `PROT_EXEC`（`0x4`）確定、`exec_confirmed`
- `mov $0x4, %edx` + `mprotect` syscall → 32bit 形式でも `exec_confirmed`（`edx`/`rdx` 両形式カバー）
- `mov $0x3, %rdx` + `mprotect` syscall → `PROT_READ|PROT_WRITE`（`0x3`）、`exec_not_set`
- `mov %rsi, %rdx` + `mprotect` syscall → 間接設定、`exec_unknown`
- `mprotect` syscall のみ（スキャン範囲内に `rdx` 変更命令なし） → `exec_unknown`
- 制御フロー命令（`jmp`、`call` 等）を挟んだ場合にスキャンが打ち切られ `exec_unknown` となること

**arm64**

- `mov x2, #0x7`（`MOV Xn, #imm` エンコーディング）+ `mprotect` syscall → `exec_confirmed`
- `mov x2, #0x3` + `mprotect` syscall → `exec_not_set`
- `mov x2, x1`（レジスタ間コピー）+ `mprotect` syscall → `exec_unknown`
- `mprotect` syscall のみ（スキャン範囲内に `x2` 変更命令なし） → `exec_unknown`
- 制御フロー命令（`b`、`bl` 等）を挟んだ場合にスキャンが打ち切られ `exec_unknown` となること

## 5. リスク判定への組み込み

`mprotect(PROT_EXEC)` の検出結果（`ArgEvalResults`）を既存の `IsHighRisk` フィールドに反映する。
`ArgEvalResults` は解析事実のみを保持し、リスク判定ロジックは runner 層（`network_analyzer.go`
の `handleAnalysisOutput` 等）に置く。判定基準の分散を防ぐため、`ArgEvalResults` から
`IsHighRisk` へのマッピングは `security` パッケージ内の共通ヘルパー関数として一元化すること。

### 5.1 マッピングルール

| `ArgEvalResults` の状態 | `IsHighRisk` への影響 |
|---|---|
| `exec_confirmed` が1件以上ある | `true` に設定する |
| `exec_unknown` が1件以上ある（`exec_confirmed` なし） | `true` に設定する（注1） |
| `exec_not_set` のみ（`exec_confirmed` / `exec_unknown` なし） | 変更しない |
| `mprotect` 未検出（`ArgEvalResults` にエントリなし） | 変更しない |

**注1:** `exec_unknown` を `IsHighRisk = true` とする理由は、引数が静的に確定できない場合に動的コードロードの可能性を排除できないためである。ただし Go バイナリ等で false positive が多発することが実バイナリ調査で判明した場合は、`exec_unknown` を `IsHighRisk` に反映しない方向に変更することを検討する（未解決事項 8 参照）。

既存の `IsHighRisk = true`（ネットワーク syscall 等の他要因による）は上書きしないこと。
`IsHighRisk` は一度 `true` になったら `false` に戻さない（OR 条件）。

### 検討した案

設計の選択肢として以下の案を検討した。採用案（C'）との比較のために記録する。

採用案 **C'** は選択肢 C の変形であり、`MprotectExecStatus` フラットフィールドの代わりに
`ArgEvalResults []SyscallArgEvalResult` リスト構造を `SyscallAnalysisResultCore` に持つ。
リスク判定（`IsHighRisk` への反映）は `security` パッケージのヘルパー関数で一元化する。
選択肢 A と異なり、`ArgEvalResults` が解析事実を独立して保持するため、
リスク判定ロジックの変更時に解析結果の再収集が不要である点が重要な差異である。

#### 選択肢 A: 既存の `IsHighRisk` に統合する

`exec_confirmed` または `exec_unknown` の場合に `IsHighRisk = true` とする。

**不採用理由:**
- `IsHighRisk` の意味が「syscall 番号不明」から「動的コードロードの可能性あり」にも拡張され、意味が曖昧になる
- `exec_not_set`（`PROT_EXEC` なし）の場合に low risk と判定できない（`IsHighRisk = false` のままで情報が欠落する）
- 将来さらに別の引数依存リスク要因が加わるたびに `IsHighRisk` の意味がさらに希薄になる

#### 選択肢 B: 専用の `RiskLevel` 型フィールドを新設する

`SyscallAnalysisResultCore` に `RiskLevel string`（`"low"` / `"medium"` / `"high"`）フィールドを追加し、
`mprotect` 検出結果をもとに設定する。

**不採用理由:**
- `IsHighRisk`（bool）と `RiskLevel`（string）の2フィールドが並存し、冗長になる
- 既存フィールドとの整合性を保つ変換ロジックが常に必要になる
- 将来の引数依存リスク要因の追加のたびに `RiskLevel` の設定ロジックが複雑化する

#### 選択肢 C: `mprotect` 検出を独立したフィールドで持つ

`SyscallAnalysisResultCore` に `MprotectExecStatus string` フィールドを追加するだけとし、
リスク判定へのマッピングは runner 側で行う。

**不採用理由:**
- 将来 `mmap(PROT_EXEC)` や `prctl` 等の引数依存評価が加わるたびにフィールドが増殖し、
  Core 型・スキーマバージョンが都度変わる
- 「引数依存の評価結果」という共通概念がフラットなフィールドとして散在し、構造的一貫性がない

#### 選択肢 D: `IsHighRisk` を廃止し `RiskLevel` に一本化する

既存の `IsHighRisk` フィールドを削除し、`RiskLevel`（`"low"` / `"medium"` / `"high"`）に統一する。

**不採用理由（現タスクでは）:**
- 既存コードへの影響範囲が最も広く（`IsHighRisk` の全参照箇所を変更）、本タスクの主目的に対して変更コストが不均衡に大きい
- 「引数の評価結果をどこに持つか」という問いには答えておらず、C' が持つ構造的一貫性を得られない
- 将来的に引数依存リスク要因が複数蓄積した段階での移行検討が適切

## 6. 設計上の考慮事項

### 6.1 静的解析の限界

- **JIT コンパイラ**: Python、Java、V8 等は `mprotect(PROT_EXEC)` を JIT コードのために使用する。
  `dlopen` 由来かどうかを静的解析で区別することはできない。
- **コンパイラ最適化**: `prot` 引数がコンパイル時定数であれば即値として現れるが、
  計算結果や変数経由の場合は `exec_unknown` となる。
  glibc の `dlopen` 実装では `prot` は定数で渡されることが多く、静的解析で捕捉できる可能性が高い。
- **Go バイナリ**: Go ランタイムは goroutine スタックの管理（guard page の `PROT_NONE` 設定等）のために `mprotect` を頻繁に呼び出す。これらは通常 `PROT_EXEC` を設定しないが、後方スキャンで `prot` 引数が確定できない（`exec_unknown`）ケースが多数発生しうる。そのため `exec_unknown` エントリが多く報告されても、それ自体は Go ランタイムの通常動作に起因する可能性がある。

### 6.2 `dlopen` ELF シンボル検出との関係

タスク 0069 では ELF `.dynsym` セクションの `dlopen` / `dlsym` / `dlvsym` シンボルを直接検出している。
動的リンクされたバイナリに対しては 0069 の手法の方が精度が高く、`mprotect` 検出は補完的なシグナルに留まる。

本タスクの主な価値は、**静的リンクされたバイナリ**（Go バイナリ等）において
`dl*` 系シンボルが `.dynsym` に現れないケースでの検出精度向上にある。

### 6.3 `rdx` スキャンの精度

x86_64 において `mprotect` の引数順序は：
- `rdi`: addr
- `rsi`: len
- `rdx`: prot

`rdx` は汎用レジスタであり、他の命令でも頻繁に使用される。
タスク 0070 の `rax` スキャンと同様に、制御フロー命令で走査を打ち切るルールを適用する。

### 6.4 `ArgEvalResults` の拡張性

`ArgEvalResults` はリスト構造を持つため、将来 `mmap(PROT_EXEC)` や `prctl` 等の
引数依存評価を追加する際に `SyscallAnalysisResultCore` の構造変更が不要となる。
スキーマバージョン更新は引き続き必要だが、型定義の変更は生じない。

## 7. 受け入れ条件

### AC-1: `mprotect` の識別

- [ ] syscall 番号が `mprotect`（x86_64: 10、arm64: 226）のエントリを識別できること

### AC-2: `prot` 引数の取得

- [ ] `rdx`（x86_64）/ `x2`（arm64）への即値設定を後方スキャンで取得できること
- [ ] 制御フロー命令を越えた走査を行わないこと
- [ ] `syscall` 命令から 50 命令以内（`defaultMaxBackwardScan`）に即値設定が見つからない場合は `exec_unknown` と判定されること

### AC-3: `PROT_EXEC` フラグの判定

- [ ] `prot & 0x4 != 0` の場合に `exec_confirmed` と判定されること
- [ ] `prot & 0x4 == 0` の場合に `exec_not_set` と判定されること
- [ ] 即値が取得できない場合に `exec_unknown` と判定されること

### AC-4: 解析結果の保存・読み込み

- [ ] `ArgEvalResults` が JSON 解析結果ファイルに保存されること
- [ ] `mprotect` syscall が検出されなかったバイナリの解析結果において、`ArgEvalResults` に `SyscallName == "mprotect"` のエントリが存在しないこと
- [ ] スキーマバージョンが更新され、旧バージョンの解析結果が無効化されること
- [ ] 保存・読み込みの往復で情報が欠落しないこと

### AC-5: 既存機能への非影響

- [ ] 既存の `HasNetworkSyscalls` の判定結果が変わらないこと
- [ ] リポジトリ全体の既存のテスト（`make test`）がすべてパスすること

### AC-6: 複数 `mprotect` 検出時の集約

- [ ] 複数の `mprotect` syscall が検出された場合に `ArgEvalResults` へのエントリが1件のみであること
- [ ] `exec_confirmed` が1件でもある場合に `ArgEvalResults` のエントリが `exec_confirmed` となること
- [ ] `exec_confirmed` がなく `exec_unknown` が1件以上ある場合に `ArgEvalResults` のエントリが `exec_unknown` となること

### AC-7: リスク判定への反映

- [ ] `exec_confirmed` が1件以上ある場合に `IsHighRisk = true` となること
- [ ] `exec_unknown` が1件以上ある場合（`exec_confirmed` なし）に `IsHighRisk = true` となること
- [ ] `exec_not_set` のみの場合に `IsHighRisk` が変更されないこと
- [ ] `mprotect` 未検出の場合に `IsHighRisk` が変更されないこと
- [ ] ネットワーク syscall 等の他要因で既に `IsHighRisk = true` の場合に値が上書き（`false` に変更）されないこと
- [ ] マッピングロジックが `security` パッケージのヘルパー関数として実装されていること

## 8. 未解決事項

- `exec_unknown` を `IsHighRisk = true` とすることの妥当性：Go バイナリ等で false positive が多発するか否かを実バイナリ調査で確認し、必要に応じてマッピングルールを改定する（5.1 注1 参照）
- arm64 における `x2` レジスタのスキャン実装詳細
