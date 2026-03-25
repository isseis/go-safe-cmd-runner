# `pkey_mprotect(PROT_EXEC)` 静的検出 要件定義書

## 1. 概要

### 1.1 背景

タスク 0078 では `mprotect(PROT_EXEC)` の静的検出を実装した。Linux では `mprotect` の拡張版として
`pkey_mprotect` syscall（Linux 4.9+）が存在する。

```c
int pkey_mprotect(void *addr, size_t len, int prot, int pkey);
```

`pkey_mprotect` は `mprotect` に第4引数 `pkey`（Memory Protection Key）を追加したものであり、
`prot` 引数の位置・セマンティクスは `mprotect` と同一（第3引数、x86_64: `rdx`、arm64: `x2`）である。

したがって、タスク 0078 で実装した後方スキャンロジックをほぼそのまま流用して
`pkey_mprotect(PROT_EXEC)` を検出できる。

### 1.2 目的

タスク 0078 の `mprotect(PROT_EXEC)` 検出を `pkey_mprotect(PROT_EXEC)` にも拡張し、
動的コードロードシグナルの検出漏れを防ぐ。

### 1.3 スコープ

- **対象**: ELF バイナリ（x86_64 Linux、arm64 Linux）
- **対象**: タスク 0078 の `mprotect` 検出の延長として実装する
- **対象外**: 非 ELF ファイル（スクリプト等）
- **対象外**: Mach-O バイナリ（`pkey_mprotect` は Linux 固有機能）
- **対象外**: `pkey_mprotect` の `pkey` 引数の評価（本タスクでは `prot` 引数のみを評価対象とする）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `pkey_mprotect` | Linux 4.9+ で導入された `mprotect` の拡張 syscall。シグネチャは `int pkey_mprotect(void *addr, size_t len, int prot, int pkey)`。x86_64 syscall 番号: 329、arm64 syscall 番号: 288 |
| `mprotect` ファミリー | `mprotect`（x86_64: 10、arm64: 226）と `pkey_mprotect`（x86_64: 329、arm64: 288）の総称。どちらも `prot` フラグで実行権限を制御する |
| `pkey` | Memory Protection Key。`pkey_mprotect` の第4引数。本タスクでは評価対象外 |
| 既存定義 | `PROT_EXEC`、`PROT_READ`、`PROT_WRITE`、即値確定、引数不確定、`SyscallArgEvalResult` はタスク 0078 の用語定義を引き継ぐ |

## 3. 機能要件

### 3.1 `pkey_mprotect` syscall の特定

#### FR-3.1.1: `pkey_mprotect` syscall の識別

Pass 1（直接 `syscall` 命令の解析）において、syscall 番号が `pkey_mprotect`
（x86_64: 329、arm64: 288）であるエントリを識別できること。

識別ロジックはタスク 0078 FR-3.1.1（`mprotect` 識別）と同一であり、
対象 syscall 番号のみが異なる。

#### FR-3.1.2: `prot` 引数の後方スキャン

`pkey_mprotect` syscall と判定されたエントリに対し、`prot` 引数レジスタ
（x86_64: `rdx`、arm64: `x2`）への設定命令を後方スキャンで探索できること。

スキャンロジックはタスク 0078 FR-3.1.2 の `mprotect` 用後方スキャンと完全に同一である。
`prot` 引数の位置（第3引数）が `mprotect` と `pkey_mprotect` で共通であるため、
`backwardScanForRegister` の既存実装をそのまま流用する。

#### FR-3.1.3: `PROT_EXEC` フラグの判定

後方スキャンで `prot` フラグの即値が取得できた場合、`value & 0x4`（`PROT_EXEC`）の有無を
判定できること。判定ロジックはタスク 0078 FR-3.1.3 と同一。

#### FR-3.1.4: 3段階の判定結果

`pkey_mprotect` syscall ごとに以下の3段階で判定結果を記録すること
（タスク 0078 FR-3.1.4 と同一の判定基準）：

| 判定 | 条件 |
|------|------|
| `exec_confirmed` | `prot` 即値が取得でき、かつ `PROT_EXEC` フラグが立っている |
| `exec_unknown` | `prot` 即値が取得できなかった（引数不確定） |
| `exec_not_set` | `prot` 即値が取得でき、かつ `PROT_EXEC` フラグが立っていない |

### 3.2 解析結果への統合

#### FR-3.2.1: `pkey_mprotect` エントリの `ArgEvalResults` への追加

`pkey_mprotect` が検出された場合、`SyscallName = "pkey_mprotect"` のエントリを
`ArgEvalResults` に追加すること。

`mprotect` と `pkey_mprotect` は独立したエントリとして保持し、それぞれ最大1件ずつ記録する。
`ArgEvalResults` には `SyscallName == "mprotect"` のエントリと
`SyscallName == "pkey_mprotect"` のエントリが共存しうる。

複数の `pkey_mprotect` syscall が検出された場合の集約ルールは
タスク 0078 FR-3.2.1 と同一（`exec_confirmed` > `exec_unknown` > `exec_not_set` の優先順位）。

`pkey_mprotect` が検出されなかった場合は `ArgEvalResults` に
`SyscallName == "pkey_mprotect"` のエントリを追加しない。

#### FR-3.2.2: `EvalProtExecRisk` の拡張

`EvalProtExecRisk`（`elfanalyzer` パッケージ）の評価対象を `pkey_mprotect` エントリにも
拡張すること。

現在の実装は `SyscallName == "mprotect"` のエントリのみを評価しているが、
本タスク以降は `SyscallName == "pkey_mprotect"` のエントリも同じマッピングルール
（`exec_confirmed` / `exec_unknown` → `true`）で評価すること。

旧名 `EvalMprotectRisk` を `EvalProtExecRisk` に改名すること。
`elfanalyzer` は `internal` パッケージであるため改名コストは低い。
ファイル名も `mprotect_risk.go` → `prot_exec_risk.go` に変更すること（§6.3 参照）。

#### FR-3.2.3: `AnalysisWarnings` メッセージ

`pkey_mprotect` 検出時に `AnalysisWarnings` へ追加するメッセージのフォーマットは以下とする：

- `exec_confirmed`: `"pkey_mprotect at 0x%x: PROT_EXEC confirmed (%s)"`
- `exec_unknown`: `"pkey_mprotect at 0x%x: PROT_EXEC could not be ruled out (%s)"`

（`mprotect` のメッセージと同一パターンで syscall 名のみ異なる）

警告の生成は `mprotect` と `pkey_mprotect` のエントリそれぞれに対して独立して行う。
生成タイミングおよび生成主体は `mprotect` と同一（`analyzeSyscallsInCode` 内でループ処理）。

### 3.3 `evaluateMprotectFamilyArgs` シグネチャ変更

#### FR-3.3.1: 内部関数のリファクタリング

現在の `evaluateMprotectArgs`（`*SyscallArgEvalResult, uint64` を返す）は
`mprotect` 単一 syscall のみを対象とし、戻り値が1件固定である。

本タスクでは `mprotect` と `pkey_mprotect` の両方を処理し、
それぞれ独立した `ArgEvalResults` エントリを返す必要があるため、
関数シグネチャを以下のように変更すること：

- **関数名**: `evaluateMprotectArgs` → `evaluateMprotectFamilyArgs`
  （内部関数であり呼び出し元への波及なし）
- **戻り値**: `(*SyscallArgEvalResult, uint64)` →
  `[]mprotectFamilyEvalResult`
  （`result` と `location` を持つローカル構造体のスライス）

`evaluateMprotectFamilyArgs` は `mprotect` ファミリー（`mprotect` と `pkey_mprotect`）を
対象に syscall 名ごとに集約し、検出された syscall 名ごとに最大1件のエントリを返す。

`evalSingleMprotect` は syscall 名を引数（`syscallName string`）で受け取るよう汎化し、
`SyscallName` フィールドへの埋め込み値を動的に設定できるようにすること。

### 3.4 保存と読み込み

#### FR-3.4.1: スキーマバージョン更新

`CurrentSchemaVersion` を 6 → 7 に更新し、旧バージョンの解析結果を無効化すること。
`pkey_mprotect` エントリを含む `ArgEvalResults` は既存の JSON 構造（`[]SyscallArgEvalResult`）に
そのまま保存される（構造変更なし）。

`schema.go` の `CurrentSchemaVersion` 定数コメントに以下の1行を追加すること：

```
// Version 7 adds pkey_mprotect PROT_EXEC detection.
```

#### FR-3.4.2: 後方互換性

スキーマバージョン不一致時の既存の動作（解析結果を無効化して再解析を要求）を維持すること。

## 4. 非機能要件

### 4.1 パフォーマンス

#### NFR-4.1.1: 解析オーバーヘッド

`pkey_mprotect` は実用上ほとんどのバイナリには存在しないため、
追加の解析オーバーヘッドは軽微であること。

### 4.2 テスト可能性

#### NFR-4.2.1: ユニットテスト

x86_64 / arm64 それぞれについて、以下のバイト列パターンに対するユニットテストを実装すること。
テスト構成はタスク 0078 NFR-4.2.1 に準拠する。

**x86_64**

- `mov $0x7, %rdx` + `pkey_mprotect` syscall（syscall 番号 329）→ `exec_confirmed`
- `mov $0x4, %edx` + `pkey_mprotect` syscall → `exec_confirmed`（32bit edx 形式）
- `mov $0x3, %rdx` + `pkey_mprotect` syscall → `exec_not_set`
- `mov %rsi, %rdx` + `pkey_mprotect` syscall → `exec_unknown`（間接設定）
- `pkey_mprotect` syscall のみ（スキャン範囲内に `rdx` 変更命令なし）→ `exec_unknown`
- 制御フロー命令を挟んだ場合にスキャンが打ち切られ `exec_unknown` となること

**arm64**

- `mov x2, #0x7` + `pkey_mprotect` syscall（syscall 番号 288）→ `exec_confirmed`
- `mov x2, #0x3` + `pkey_mprotect` syscall → `exec_not_set`
- `mov x2, x1`（レジスタ間コピー）+ `pkey_mprotect` syscall → `exec_unknown`
- `pkey_mprotect` syscall のみ（スキャン範囲内に `x2` 変更命令なし）→ `exec_unknown`
- 制御フロー命令を挟んだ場合にスキャンが打ち切られ `exec_unknown` となること

**統合テスト**

- `mprotect` と `pkey_mprotect` が両方検出された場合に `ArgEvalResults` に2件のエントリが
  含まれること（`SyscallName` がそれぞれ `"mprotect"` / `"pkey_mprotect"`）

## 5. リスク判定への組み込み

`EvalProtExecRisk` のマッピングルールを `pkey_mprotect` エントリにも適用する。

| `ArgEvalResults` の状態 | `Summary.IsHighRisk` への影響 |
|---|---|
| `mprotect` または `pkey_mprotect` の `exec_confirmed` が1件以上ある | `true` に設定する |
| `mprotect` または `pkey_mprotect` の `exec_unknown` が1件以上ある（`exec_confirmed` なし） | `true` に設定する |
| `exec_not_set` のみ（`exec_confirmed` / `exec_unknown` なし） | 変更しない |
| `mprotect` / `pkey_mprotect` とも未検出（該当エントリなし） | 変更しない |

既存の `Summary.IsHighRisk = true`（ネットワーク syscall 等の他要因による）は上書きしないこと。

## 6. 設計上の考慮事項

### 6.1 `evaluateMprotectArgs` の拡張

現在の `evaluateMprotectArgs` は `info.Name == "mprotect"` のエントリのみを処理し、
内部の `evalSingleMprotect` は `SyscallName: "mprotect"` を結果に埋め込んでいる。

本タスクでは `pkey_mprotect` も処理対象に加える必要がある。
実装方針として、`evaluateMprotectArgs` を `mprotect` ファミリー（`mprotect` と `pkey_mprotect`）に
対してループし、syscall 名ごとに独立した集約（最大1件/syscall 名）を行う形に拡張するのが
最もシンプルである。このとき `evalSingleMprotect` は syscall 名を引数で受け取るよう汎化する。

### 6.2 `maxValidSyscallNumber` コメントの更新

`syscall_analyzer.go` の `maxValidSyscallNumber` コメントは
「Current x86_64 Linux syscalls range from 0-288」と記載されているが、
`pkey_mprotect`（329）が追加されることでこの記述が実態と合わなくなる。
機能的な影響はない（`maxValidSyscallNumber = 500` の範囲内）が、
コメントを実態に合わせて更新すること（詳細仕様書 § に記載）。

### 6.3 `EvalProtExecRisk` への改名

旧名 `EvalMprotectRisk` は `mprotect` のみを想起させるため、本タスクで `EvalProtExecRisk` に
改名する。`elfanalyzer` は `internal` パッケージであり、呼び出し元（`standard_analyzer.go`、
テストファイル）も同一パッケージ内のため改名コストは低い。

改名に伴いファイル名も `mprotect_risk.go` → `prot_exec_risk.go`、
`mprotect_risk_test.go` → `prot_exec_risk_test.go` に変更する。

### 6.4 `pkey_mprotect` の実用的な出現頻度

`pkey_mprotect` は Memory Protection Keys（MPK）を使用するアプリケーション固有の機能であり、
一般的なバイナリにはほぼ存在しない。Go バイナリ、glibc の `dlopen` 実装はいずれも
`pkey_mprotect` を使用しない。したがって `exec_unknown` 偽陽性の懸念は
タスク 0078 より低い。

## 7. 受け入れ条件

### AC-1: `pkey_mprotect` の識別

- [ ] syscall 番号が `pkey_mprotect`（x86_64: 329、arm64: 288）のエントリを識別できること
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs`（x86_64、syscall 329）
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64`（arm64、syscall 288）

### AC-2: `prot` 引数の取得

- [ ] `rdx`（x86_64）/ `x2`（arm64）への即値設定を後方スキャンで取得できること
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` "PROT_EXEC confirmed (64bit rdx/32bit edx)"
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64` "exec_confirmed (mov x2, #7)"
- [ ] 制御フロー命令を越えた走査を行わないこと
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` "control flow boundary"
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64` "exec_unknown (control flow boundary)"
- [ ] `syscall` 命令から 50 命令以内（`defaultMaxBackwardScan`）に即値設定が見つからない場合は
  `exec_unknown` と判定されること
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` "pkey_mprotect syscall only"
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64` "exec_unknown (pkey_mprotect syscall only)"

### AC-3: `PROT_EXEC` フラグの判定

- [ ] `prot & 0x4 != 0` の場合に `exec_confirmed` と判定されること
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` "PROT_EXEC confirmed (64bit rdx)" (0x7) / "(32bit edx)" (0x4)
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64` "exec_confirmed (mov x2, #7)"
- [ ] `prot & 0x4 == 0` の場合に `exec_not_set` と判定されること
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` "PROT_EXEC not set" (0x3)
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64` "exec_not_set (mov x2, #3)"
- [ ] 即値が取得できない場合に `exec_unknown` と判定されること
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` "indirect register setting", "control flow boundary", "pkey_mprotect syscall only"
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64` 各 "exec_unknown" ケース

### AC-4: `ArgEvalResults` への統合

- [ ] `pkey_mprotect` 検出時に `SyscallName = "pkey_mprotect"` のエントリが
  `ArgEvalResults` に追加されること
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` 各ケースの `SyscallName` 検証
- [ ] `mprotect` と `pkey_mprotect` が両方検出された場合に `ArgEvalResults` に
  2件のエントリが共存すること（`SyscallName` がそれぞれ `"mprotect"` / `"pkey_mprotect"`）
  - `TestSyscallAnalyzer_MprotectAndPkeyMprotect`
- [ ] `pkey_mprotect` が検出されなかった場合に `ArgEvalResults` に
  `SyscallName == "pkey_mprotect"` のエントリが存在しないこと
  - `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` "non-pkey_mprotect syscall only"

### AC-5: `EvalProtExecRisk` の拡張

- [ ] `SyscallName == "pkey_mprotect"` の `exec_confirmed` エントリに対して
  `EvalProtExecRisk` が `true` を返すこと
  - `TestEvalProtExecRisk` に `pkey_mprotect` ケースを追加
- [ ] `SyscallName == "pkey_mprotect"` の `exec_unknown` エントリに対して
  `EvalProtExecRisk` が `true` を返すこと
- [ ] `SyscallName == "pkey_mprotect"` の `exec_not_set` エントリのみの場合に
  `EvalProtExecRisk` が `false` を返すこと

### AC-6: スキーマバージョン更新

- [ ] `CurrentSchemaVersion` が 7 に更新されること
- [ ] スキーマバージョン不一致時の既存の動作（`SchemaVersionMismatchError` の返却）が維持されること
  - `TestStore_SchemaVersionMismatch` が引き続きパス

### AC-7: 既存機能への非影響

- [ ] 既存の `mprotect` 検出ロジックが引き続き正しく動作すること
  （タスク 0078 の既存テスト群がすべてパス）
- [ ] 既存の `Summary.HasNetworkSyscalls` の判定結果が変わらないこと
- [ ] リポジトリ全体の既存のテスト（`make test`）がすべてパスすること

## 8. 未解決事項

なし
