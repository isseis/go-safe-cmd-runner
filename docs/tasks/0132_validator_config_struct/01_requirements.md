# Validator 設定構造体への移行 要件定義書

## 1. 概要

### 1.1 背景

現在 `filevalidator.Validator` は、`New(algorithm, hashDir)` で基本部分のみを構成し、
動的解析に必要な依存（`BinaryAnalyzer`, `SyscallAnalyzer`, `LibcCache` 等）を
個別のセッターメソッドで後から注入する設計になっている。

各セッターのドキュメントには「`Call before the first SaveRecord() invocation`」という
契約コメントを付与している（タスク 0130 で実施）が、この制約はコンパイラによって強制されない。

セッターによる設定変更には以下のリスクがある。

- `includeDebugInfo` を `SaveRecord` 後に変更すると、セッション内の
  `processedInterpreterAnalysis` / `processedLibAnalysis` キャッシュが
  debug 出力の有無について一貫性を欠く（タスク 0130 で文書化）
- `binaryAnalyzer` / `syscallAnalyzer` 等を変更すると、キャッシュ済みの
  解析結果（`SymbolAnalysis` / `SyscallAnalysis`）と後続の解析結果が
  異なるアナライザで生成される

### 1.2 目的

セッターで注入していた「`SaveRecord` 前に 1 度だけ設定する」フィールドを
`ValidatorConfig` 構造体に集約し、`New` のコンストラクタ引数として受け取ることで、
Validator の設定フェーズと実行フェーズをコンパイル時に分離する。

ただし、`dynamicLibAnalysisStore` だけは `Validator` 自身を `dynamicanalysis.Analyzer`
として渡す必要があり（`dynamicanalysis.New(storeDir, fv)` の形）、完全なコンストラクタ
一括初期化が構造上困難なため、引き続きセッターで注入する（後述 1.3）。

### 1.3 スコープ

**対象（`ValidatorConfig` に移す）**:
- `binaryAnalyzer`（`SetBinaryAnalyzer` を削除）
- `syscallAnalyzer`（`SetSyscallAnalyzer` を削除）
- `libcCache`（`SetLibcCache` を削除）
- `libSystemCache`（`SetLibSystemCache` を削除）
- `machoSyscallTable`（`SetMachoSyscallTable` を削除）
- `elfDynlibAnalyzer`（`SetELFDynLibAnalyzer` を削除）
- `machoDynlibAnalyzer`（`SetMachODynLibAnalyzer` を削除）
- `includeDebugInfo`（`SetIncludeDebugInfo` を削除）

**対象外（引き続きセッターで注入）**:
- `dynamicLibAnalysisStore`（`SetDynamicLibAnalysisStore` は存続）
  理由: `Validator` 自身を `dynamicanalysis.Analyzer` として `dynamicanalysis.New` に
  渡す必要があり、`ValidatorConfig` の段階では `*Validator` がまだ存在しないため

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| 設定フェーズ | `New` 呼び出しから最初の `SaveRecord` 呼び出しまでの期間 |
| 実行フェーズ | `SaveRecord` / `VerifyRecord` 等を呼び出す期間 |
| `ValidatorConfig` | 設定フェーズでのみ使用するフィールドをまとめたオプション構造体 |
| セッター | `Set*` メソッド群。本タスク後は `SetDynamicLibAnalysisStore` のみ残存 |

---

## 3. 機能要件

### 3.1 `ValidatorConfig` 構造体

#### FR-3.1.1: フィールド定義

`ValidatorConfig` を `internal/filevalidator` パッケージに定義する。

必須フィールド（ゼロ値 = 無効化）:

| フィールド名 | 型 | 意味 |
|---|---|---|
| `ELFDynLibAnalyzer` | `*elfdynlib.DynLibAnalyzer` | nil で ELF dynlib 解析無効 |
| `MachODynLibAnalyzer` | `*machodylib.MachODynLibAnalyzer` | nil で Mach-O dynlib 解析無効 |
| `BinaryAnalyzer` | `binaryanalyzer.BinaryAnalyzer` | nil でシンボル解析無効 |
| `SyscallAnalyzer` | `SyscallAnalyzerInterface` | nil で syscall 解析無効 |
| `LibcCache` | `LibcCacheInterface` | nil で libc ラッパ解析無効 |
| `LibSystemCache` | `LibSystemCacheInterface` | nil で Mach-O libSystem 解析無効 |
| `MachoSyscallTable` | `SyscallNumberTable` | nil で番号解決なし |
| `DebugInfo` | `bool` | false でデバッグ出力なし |

`ValidatorConfig{}` のゼロ値は「すべての解析を無効にした最小構成」として機能する。
これにより既存の `New(&SHA256{}, hashDir)` 相当の呼び出しは
`New(&SHA256{}, hashDir, ValidatorConfig{})` に自然に移行できる。

#### FR-3.1.2: `New` のシグネチャ変更

```go
func New(algorithm HashAlgorithm, hashDir string, cfg ValidatorConfig) (*Validator, error)
```

- `ValidatorConfig` のゼロ値（`ValidatorConfig{}`）は既存の「アナライザなし」構成に相当し、
  既存テストのほとんどはゼロ値を渡せばよい
- `dynamicLibAnalysisStore` は `ValidatorConfig` に含まない（1.3 参照）

### 3.2 セッター削除

対象セッター（FR-3.1.1 に対応するもの）を削除し、既存呼び出し箇所を
`ValidatorConfig` フィールドへの代入に変更する。

`SetDynamicLibAnalysisStore` は残存させる。

### 3.3 既存 API との互換

#### FR-3.3.1: `cmd/record/main.go` の `validatorFactory` 型変更

現在の factory は `func(hashDir string) (hashRecorder, error)` だが、
`ValidatorConfig` を渡せるよう変更が必要。

方針: factory の型を `func(hashDir string, cfg ValidatorConfig) (hashRecorder, error)` に変更し、
`run()` 内で `cfg` を組み立てて factory に渡す。

#### FR-3.3.2: テスト側の更新

テストが `Set*` メソッドを呼んでいた箇所は `ValidatorConfig` フィールド指定に置き換える。
`New(&SHA256{}, hashDir)` は `New(&SHA256{}, hashDir, ValidatorConfig{})` に変更する。

---

## 4. 非機能要件

### 4.1 互換性

- `fileanalysis.Record` のスキーマ変更なし
- `dynamicanalysis.Store` のスキーマ変更なし
- `Validator` の実行時挙動（解析結果の内容）は変更なし

### 4.2 テスタビリティ

- テストでは `ValidatorConfig` の該当フィールドだけを設定し、残りはゼロ値のまま使えることを確認する
- `SetDynamicLibAnalysisStore` は残存するため、テストでの遅延注入パターンは維持される

---

## 5. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | `ValidatorConfig{}` を渡した `New` 呼び出しが、旧来の「セッターなし」構成と同一の挙動をする |
| AC-2 | 削除対象のセッター（`SetBinaryAnalyzer` 等）を呼び出すコードがコンパイルエラーになる |
| AC-3 | `SetDynamicLibAnalysisStore` は引き続き利用可能であり、`dynamicanalysis.New(dir, fv)` 後に呼び出せる |
| AC-4 | `cmd/record/main.go` がすべての設定フィールドを `ValidatorConfig` 経由で渡し、旧セッター呼び出しが残っていない |
| AC-5 | `make fmt` / `go test -tags test -v ./...` / `make lint` がすべて成功する |

---

## 6. 制約事項

1. `dynamicLibAnalysisStore` の注入は `ValidatorConfig` 経由にしない（1.3 の理由による）
2. `validatorFactory` の型変更は `cmd/record` パッケージ内にとどめる
3. `Validator` のエクスポートされていないフィールド名は変更しない

---

## 7. 想定外（Non-Goals）

- `dynamicLibAnalysisStore` の循環依存問題の解消（別タスク）
- `Validator` のセッターを完全にゼロにすること
- `validatorFactory` を `Validator` 以外の型で使えるようにすること
