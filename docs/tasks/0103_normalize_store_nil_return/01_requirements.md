# 解析ストア API の正規化：「解析済み・未検出」を `(nil, nil)` で返す 要件定義書

## 1. 概要

### 1.1 背景

`fileanalysis` パッケージの `LoadSyscallAnalysis` および `LoadNetworkSymbolAnalysis` は、
レコードが存在してもレコード内に対応するフィールドがない場合（「解析済み・未検出」）に
センチネルエラー（`ErrNoSyscallAnalysis`、`ErrNoNetworkSymbolAnalysis`）を返す設計となっている。

```
// 現状
(nil, ErrNoSyscallAnalysis)       ← 解析済み・syscall 未検出
(nil, ErrNoNetworkSymbolAnalysis) ← 解析済み・ネットワークシンボル未検出
```

センチネルエラーはエラーハンドリングのパスに「正常ケース」を混在させるため、
呼び出し元のコードが複雑になる。「解析済み・未検出」は正常な処理結果であり、
エラーではなく `(nil, nil)` で表現するのが Go の慣用的な API 設計である。

タスク 0100（Mach-O libSystem syscall キャッシュ）の仕様策定において、
`ErrNoSyscallAnalysis` を ELF と Mach-O で一貫して「正常ケース（fall-through）」と
定義したことを契機に、このリファクタリングを独立タスクとして実施する。

### 1.2 目的

- `ErrNoSyscallAnalysis` および `ErrNoNetworkSymbolAnalysis` を削除し、
  「解析済み・未検出」を `(nil, nil)` で返す API に統一する
- 呼び出し元の switch/case からこれらのセンチネルエラー向けケースを除去し、
  `result == nil` チェックに置き換える

### 1.3 スコープ

**変更対象:**

- `internal/fileanalysis/syscall_store.go` — `LoadSyscallAnalysis` の返却値
- `internal/fileanalysis/network_symbol_store.go` — `LoadNetworkSymbolAnalysis` の返却値
- `internal/fileanalysis/errors.go` — センチネルエラー定義の削除
- `internal/fileanalysis/schema.go` — `ErrNoSyscallAnalysis` への言及を含むコメントの更新
- `internal/runner/security/network_analyzer.go` — 呼び出し側の switch 文
- `internal/runner/security/elfanalyzer/standard_analyzer.go` — 呼び出し側の switch 文
- 上記ファイルに対応するテストファイル

**変更対象外:**

- `ErrRecordNotFound`、`ErrHashMismatch`、`SchemaVersionMismatchError` の挙動
- キャッシュや記録ファイルのフォーマット・スキーマ
- 機能的な挙動（ネットワーク判定の結果）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| 解析済み・未検出 | `record` コマンドが解析を実施したが、対象データ（syscall・ネットワークシンボル）を検出しなかった状態。現状はセンチネルエラーで表現されているが、本タスク後は `(nil, nil)` で表現する |
| センチネルエラー | 正常な処理結果をエラーとして返すための番兵値。`ErrNoSyscallAnalysis`、`ErrNoNetworkSymbolAnalysis` が該当 |

## 3. 機能要件

### FR-3.1: `LoadSyscallAnalysis` の返却値変更

`fileanalysis.SyscallAnalysisStore.LoadSyscallAnalysis` は、レコードが存在するが
`SyscallAnalysis` フィールドが nil の場合、`(nil, ErrNoSyscallAnalysis)` ではなく
`(nil, nil)` を返すこと。

**変更前:**
```go
if record.SyscallAnalysis == nil {
    return nil, ErrNoSyscallAnalysis
}
```

**変更後:**
```go
if record.SyscallAnalysis == nil {
    return nil, nil
}
```

### FR-3.2: `LoadNetworkSymbolAnalysis` の返却値変更

`fileanalysis.NetworkSymbolStore.LoadNetworkSymbolAnalysis` は、レコードが存在するが
`SymbolAnalysis` フィールドが nil の場合、`(nil, ErrNoNetworkSymbolAnalysis)` ではなく
`(nil, nil)` を返すこと。

### FR-3.3: センチネルエラーの削除

`ErrNoSyscallAnalysis` および `ErrNoNetworkSymbolAnalysis` を
`internal/fileanalysis/errors.go` から削除すること。

### FR-3.4: 呼び出し元の修正

#### FR-3.4.1: `network_analyzer.go`

`isNetworkViaBinaryAnalysis` 内の switch 文から
`errors.Is(err, fileanalysis.ErrNoNetworkSymbolAnalysis)` ケースを削除し、
`result == nil` の確認に移行すること。

`ErrNoSyscallAnalysis` ケース（`errors.Is(svcErr, fileanalysis.ErrNoSyscallAnalysis)`）を削除し、
`svcErr == nil && svcResult == nil` の場合は fall-through とすること。

#### FR-3.4.2: `elfanalyzer/standard_analyzer.go`

`lookupSyscallAnalysis` 内の switch 文から
`errors.Is(err, fileanalysis.ErrNoSyscallAnalysis)` ケースを削除すること。

また、`LoadSyscallAnalysis` が `(nil, nil)` を返す新ケースに対応するため、
`convertSyscallResult` 呼び出し前に `result == nil` チェックを追加し、
nilポインタ参照パニックを防止すること。

**追加するガード:**
```go
if result == nil {
    // Syscall analysis not stored for this file. Fall back silently.
    return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.StaticBinary}
}
```

## 4. 非機能要件

### NFR-4.1: 動作の非変更

本リファクタリングは API の形式変更のみであり、ネットワーク判定の結果（`isNetwork`、`isHighRisk`
の返却値）は変更しないこと。

### NFR-4.2: テストの更新

センチネルエラーを返すモックや、`errors.Is` でこれらを検査するテストを、
`(nil, nil)` 返却・`result == nil` 検査に更新すること。

## 5. 受け入れ条件

### AC-1: `ErrNoSyscallAnalysis` の削除

- [ ] `ErrNoSyscallAnalysis` が `errors.go` から削除されていること
- [ ] `errors.Is(_, fileanalysis.ErrNoSyscallAnalysis)` の参照がコードベース全体に存在しないこと
- [ ] `lookupSyscallAnalysis` 内に `result == nil` ガードが追加されていること

### AC-2: `ErrNoNetworkSymbolAnalysis` の削除

- [ ] `ErrNoNetworkSymbolAnalysis` が `errors.go` から削除されていること
- [ ] `errors.Is(_, fileanalysis.ErrNoNetworkSymbolAnalysis)` の参照がコードベース全体に存在しないこと

### AC-3: `(nil, nil)` の返却

- [ ] `LoadSyscallAnalysis` が「解析済み・syscall 未検出」のレコードに対して `(nil, nil)` を返すこと
- [ ] `LoadNetworkSymbolAnalysis` が「解析済み・ネットワークシンボル未検出」のレコードに対して `(nil, nil)` を返すこと

### AC-4: 動作の非変更

- [ ] 既存のすべてのテストがパスすること
- [ ] `make lint` がエラーなしで通ること

## 6. 実施タイミング

タスク 0100（Mach-O libSystem syscall キャッシュ）の実装完了後に実施すること。
タスク 0100 の `03_detailed_specification.md` §13.2 はタスク 0103 実施前の状態
（`ErrNoSyscallAnalysis` を fall-through として扱う）を前提としている。
