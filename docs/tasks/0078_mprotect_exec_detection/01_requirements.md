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
| 即値確定 | 後方スキャンにより引数レジスタへの即値設定命令が見つかり、`prot` フラグの値が静的に決定できた状態 |
| 引数不確定 | 後方スキャンで引数レジスタへの即値設定命令が見つからなかった、または制御フロー境界に到達した状態 |
| 動的コードロード | `dlopen` 等を使い、実行時に外部の共有ライブラリを読み込んでそのコードを実行すること |

## 3. 機能要件

### 3.1 `mprotect(PROT_EXEC)` の検出

#### FR-3.1.1: `mprotect` syscall の特定

タスク 0070 の Pass 1（直接 `syscall` 命令の解析）において、
syscall 番号が `mprotect`（x86_64: 10、arm64: 226）であるエントリを識別できること。

#### FR-3.1.2: `prot` 引数の後方スキャン

`mprotect` syscall と判定されたエントリに対し、`prot` 引数レジスタ
（x86_64: `rdx`、arm64: `x2`）への設定命令を後方スキャンで探索できること。

スキャンルールはタスク 0070 FR-3.1.3 の syscall 番号取得（`rax`/`eax` 対象）と同一とし、
対象レジスタのみが異なる。

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

#### FR-3.2.1: 解析結果への追加

タスク 0070 の `SyscallAnalysisResultCore`（`common` パッケージ）に、
`mprotect` 検出結果を格納するフィールドを追加すること。
既存の `DetectedSyscalls` リストとは独立したフィールドとして保持する。

#### FR-3.2.2: 既存フィールドへの非影響

`mprotect(PROT_EXEC)` の検出は、既存の `HasNetworkSyscalls` / `IsHighRisk` フィールドに
影響を与えないこと。リスク判定への組み込みは本タスクのスコープ外とする（第5節参照）。

### 3.3 保存と読み込み

#### FR-3.3.1: 解析結果の永続化

`mprotect` 検出結果を既存の TOML 解析結果ファイルに追記する形で保存できること。
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

- `mov $0x7, %rdx` + `mprotect` syscall → `PROT_EXEC`（`0x4`）確定、`exec_confirmed`
- `mov $0x3, %rdx` + `mprotect` syscall → `PROT_READ|PROT_WRITE`（`0x3`）、`exec_not_set`
- `mov %rsi, %rdx` + `mprotect` syscall → 間接設定、`exec_unknown`
- `mprotect` syscall のみ（スキャン範囲内に `rdx` 変更命令なし） → `exec_unknown`

## 5. リスク判定への組み込み（未決定）

`mprotect(PROT_EXEC)` 検出をリスク判定にどう組み込むかは本タスク時点では決定しない。
以下に選択肢を pros / cons つきで列挙する。

### 選択肢 A: 既存の `IsHighRisk` に統合する

`exec_confirmed` または `exec_unknown` の場合に `IsHighRisk = true` とする。

**Pros:**
- 実装変更が最小。既存のリスク判定フローを変更せずに済む
- 呼び出し側（runner）の変更が不要

**Cons:**
- `IsHighRisk` の意味が「syscall 番号不明」から「動的コードロードの可能性あり」にも拡張され、意味が曖昧になる
- `exec_not_set`（`PROT_EXEC` なし）の場合に low risk と判定できない（`IsHighRisk = false` のままで情報が欠落する）
- 将来さらに別のリスク要因が加わった際に `IsHighRisk` の意味がさらに希薄になる

### 選択肢 B: 専用の `RiskLevel` 型フィールドを新設する

`SyscallAnalysisResultCore` に `RiskLevel string`（`"low"` / `"middle"` / `"high"`）フィールドを追加し、
`mprotect` 検出結果をもとに設定する。

| 条件 | RiskLevel |
|------|-----------|
| `exec_confirmed` | `"middle"` |
| `exec_unknown` | `"middle"` |
| `exec_not_set` | `"low"` |
| `mprotect` 未検出 | `"low"` |
| 既存 `IsHighRisk` | `"high"` |

**Pros:**
- 3段階のリスクを明示的に表現でき、将来の拡張に対応しやすい
- `IsHighRisk` の意味が変わらず既存ロジックが維持される

**Cons:**
- `IsHighRisk`（bool）と `RiskLevel`（string）の2フィールドが並存し、冗長になる
- 既存フィールドとの整合性を保つ変換ロジックが必要
- スキーマ・API の変更範囲が広い

### 選択肢 C: `mprotect` 検出を独立したフィールドで持ち、リスク判定は呼び出し側に委ねる

`SyscallAnalysisResultCore` に `MprotectExecStatus string` フィールドを追加するだけとし、
リスク判定へのマッピングは runner 側で行う。

**Pros:**
- 解析層とリスク判定層の責務が明確に分離される
- 解析結果の意味変更なしに保存・再利用できる
- 将来の判定ロジック変更時に解析結果の再収集が不要

**Cons:**
- runner 側に判定ロジックが分散し、一元管理しにくくなる
- 呼び出し側ごとに判定基準がばらつくリスクがある

### 選択肢 D: `IsHighRisk` を廃止し `RiskLevel` に一本化する

既存の `IsHighRisk` フィールドを削除し、`RiskLevel`（`"low"` / `"middle"` / `"high"`）に統一する。

| 旧条件 | 新 RiskLevel |
|--------|-------------|
| `IsHighRisk = true`（syscall 番号不明） | `"high"` |
| `exec_confirmed` / `exec_unknown` | `"middle"` |
| それ以外 | `"low"` |

**Pros:**
- フィールドが一本化され、意味が明確になる
- 将来の拡張余地が最も大きい

**Cons:**
- 既存コードへの影響範囲が最も広い（`IsHighRisk` の全参照箇所を変更）
- スキーマバージョンの更新が必要で、既存の解析結果が全て無効になる

## 6. 設計上の考慮事項

### 6.1 静的解析の限界

- **JIT コンパイラ**: Python、Java、V8 等は `mprotect(PROT_EXEC)` を JIT コードのために使用する。
  `dlopen` 由来かどうかを静的解析で区別することはできない。
- **コンパイラ最適化**: `prot` 引数がコンパイル時定数であれば即値として現れるが、
  計算結果や変数経由の場合は `exec_unknown` となる。
  glibc の `dlopen` 実装では `prot` は定数で渡されることが多く、静的解析で捕捉できる可能性が高い。
- **Go バイナリ**: Go ランタイムは `mprotect` を goroutine スタックの管理にも使用する。
  これが `PROT_EXEC` を設定するかどうかは実装依存であり、誤検出の原因となりうる。

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

## 7. 受け入れ条件

### AC-1: `mprotect` の識別

- [ ] syscall 番号が `mprotect`（x86_64: 10、arm64: 226）のエントリを識別できること

### AC-2: `prot` 引数の取得

- [ ] `rdx`（x86_64）/ `x2`（arm64）への即値設定を後方スキャンで取得できること
- [ ] 制御フロー命令を越えた走査を行わないこと
- [ ] スキャン範囲を超えた場合は `exec_unknown` と判定されること

### AC-3: `PROT_EXEC` フラグの判定

- [ ] `prot & 0x4 != 0` の場合に `exec_confirmed` と判定されること
- [ ] `prot & 0x4 == 0` の場合に `exec_not_set` と判定されること
- [ ] 即値が取得できない場合に `exec_unknown` と判定されること

### AC-4: 解析結果の保存・読み込み

- [ ] `mprotect` 検出結果が TOML 解析結果ファイルに保存されること
- [ ] スキーマバージョンが更新され、旧バージョンの解析結果が無効化されること
- [ ] 保存・読み込みの往復で情報が欠落しないこと

### AC-5: 既存機能への非影響

- [ ] 既存の `HasNetworkSyscalls` / `IsHighRisk` の判定結果が変わらないこと
- [ ] 既存のテストがすべてパスすること

## 8. 未解決事項

- リスク判定への組み込み方（第5節の選択肢 A〜D から決定する）
- `mprotect` 未検出のバイナリを low risk とみなして良いか（Go ランタイムが内部で使う可能性）
- arm64 における `x2` レジスタのスキャン実装詳細
