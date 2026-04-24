# 要件定義書: シンボル解析のライブラリフィルタ導入

## 1. 概要

### 1.1 背景

現行の `record` コマンドは、バイナリのシンボル解析（`AnalyzeNetworkSymbols`）においてネットワーク関連名に一致するシンボルのみを記録している。このフィルタリングには 2 つの問題がある。

**問題 1: 関心の分離の逸脱**

「ネットワーク関連かどうか」の判断はポリシー（`runner` 側）の関心事であり、`record` 側が判断すべきではない。タスク 0105 でシステムコールフィルタリングを削除した理由と同様に、シンボル解析においても `record` は情報を収集するのみであるべきだ。

**問題 2: ライブラリ帰属によるフィルタの欠如**

現行の実装では、ネットワーク関連名に一致するシンボルをすべての動的ライブラリから収集している。しかし、最終的に何のシステムコールを呼び出すかを把握するには、システムコールの標準インターフェースである libc（ELF）/ libSystem（Mach-O）のシンボルに限定すれば十分であり、他のライブラリのシンボルは記録しても意味がない。

### 1.2 目的

- `AnalyzeNetworkSymbols` のネットワーク名フィルタを廃止し、**libc（ELF）/ libSystem（Mach-O）由来のシンボルに限定したフィルタ**へ置き換える
- libc / libSystem から import されているすべてのシンボルを `DetectedSymbols` に記録する
- `runner` がネットワーク判定にシンボルの `Category` フィールドを利用するよう更新する

### 1.3 スコープ

#### 対象

- `internal/runner/security/elfanalyzer/standard_analyzer.go` の `checkDynamicSymbols`
  - ネットワーク名フィルタ（`a.networkSymbols` によるマッチング）を廃止
  - libc 由来シンボルの判定ロジックを追加
  - すべての libc シンボルを記録し、ネットワーク関連には `Category: "network"` を付与
- `internal/runner/security/machoanalyzer/standard_analyzer.go` の `analyzeSlice`
  - ネットワーク名フィルタ（`a.networkSymbols` によるマッチング）を廃止
  - libSystem 由来シンボルの判定ロジックを追加
  - すべての libSystem シンボルを記録し、ネットワーク関連には `Category: "network"` を付与
- `internal/runner/security/network_analyzer.go` のシンボル依存判定ロジック
  - `Category == "network"` を条件としたネットワーク判定へ更新
- 上記に伴う既存テストの更新

#### 対象外

- `SyscallAnalysis`（システムコール解析）への影響。タスク 0105 で対応済み
- `DynamicLoadSymbols`（`dlopen` 等）の処理。既存の `IsDynamicLoadSymbol` ロジックは変更しない
- `networkSymbols` マップを参照する他の箇所（`StandardELFAnalyzer` 構造体の初期化など）のうち、シンボル解析以外の用途への影響

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| libc | ELF バイナリが動的リンクするシステム C ライブラリ（`libc.so.6`、`libc.musl-*.so.1` 等）。システムコールラッパーを提供する |
| libSystem | macOS の Mach-O バイナリが動的リンクするシステムライブラリ（`libSystem.B.dylib`）。macOS におけるシステムコールラッパーの標準インターフェース |
| ライブラリフィルタ | どのライブラリのシンボルを記録対象とするかによるフィルタリング。本タスクでは libc / libSystem に限定 |
| ネットワーク名フィルタ | 現行の `networkSymbols` マップによるシンボル名マッチング。本タスクで廃止する |

## 3. 機能要件

### FR-1: ELF シンボル解析のライブラリフィルタ導入

`checkDynamicSymbols`（`internal/runner/security/elfanalyzer/standard_analyzer.go`）を修正し、libc 由来の import シンボルをすべて記録する。

#### FR-1.1: libc 由来シンボルの判定

ELF の `.dynsym` 内の未定義シンボル（`SHN_UNDEF`）のうち、libc に由来するものを識別する方法を導入する。具体的な判定方法（`VERNEED` セクションの解析、libc シンボル名一覧との照合など）はアーキテクチャ設計で決定する。

#### FR-1.2: カテゴリ付与

記録するシンボルには以下のカテゴリを付与する。

| 条件 | Category |
|------|----------|
| ネットワーク関連シンボル名（既存の `networkSymbols` マップに一致） | `"socket"` / `"dns"` / `"tls"` / `"http"` など、既存マップの値をそのまま使用 |
| その他の libc シンボル | `"syscall_wrapper"` |

既存の `binaryanalyzer.SymbolCategory` 定数（`CategorySocket`, `CategoryDNS` 等）をそのまま利用し、新しい統合カテゴリ `"network"` は導入しない。

修正前（概念）:
```go
// networkSymbols マップに一致するものだけ記録
if cat, found := a.networkSymbols[sym.Name]; found {
    detected = append(...)
}
```

修正後（概念）:
```go
// libc 由来シンボルをすべて記録し、カテゴリを付与
if isLibcSymbol(sym) {
    cat := categorize(sym.Name, a.networkSymbols) // ネットワーク: 既存カテゴリ, その他: "syscall_wrapper"
    detected = append(...)
}
```

### FR-2: Mach-O シンボル解析のライブラリフィルタ導入

`analyzeSlice`（`internal/runner/security/machoanalyzer/standard_analyzer.go`）を修正し、libSystem 由来の import シンボルをすべて記録する。

#### FR-2.1: libSystem 由来シンボルの判定

`macho.File.ImportedSymbols()` は全インポートシンボルを返すが、ライブラリ帰属情報を含まない。libSystem 由来シンボルの識別方法（バインド情報の解析、既存の `MachoLibSystemCache` との照合など）はアーキテクチャ設計で決定する。

#### FR-2.2: カテゴリ付与

FR-1.2 と同様に、既存の `binaryanalyzer.SymbolCategory` 定数（`"socket"`, `"dns"`, `"tls"`, `"http"` 等）または `"syscall_wrapper"` を付与する。

### FR-3: `runner` ネットワーク判定ロジックの更新

`internal/runner/security/network_analyzer.go` のシンボル依存判定を、カテゴリフィールドに基づく判定へ更新する。

```go
// 修正後の概念
hasNetworkSymbol := false
for _, sym := range result.DetectedSymbols {
    if binaryanalyzer.IsNetworkCategory(sym.Category) { // "socket", "dns", "tls", "http" 等を判定
        hasNetworkSymbol = true
        break
    }
}
```

`AnalysisResult` の `NetworkDetected` / `NoNetworkSymbols` の判定は、`DetectedSymbols` 内にネットワーク系カテゴリのシンボルが 1 つ以上あるかどうかで決める。`"syscall_wrapper"` のみの場合は `NoNetworkSymbols` 扱いとする。

## 4. 非機能要件

### NFR-1: 既存レコードとの互換性

`DetectedSymbols` フィールドの内容が増えるが、JSON 構造（フィールド追加・削除・型変更）は変わらない。既存レコードはそのまま利用可能。

旧レコードを持つバイナリでは `"socket"` / `"dns"` / `"tls"` / `"http"` カテゴリのシンボルのみが `DetectedSymbols` に記録されている。新しい `runner` はこれらの既存カテゴリをネットワーク判定に使用するため、旧レコードに対しても正しく動作する（後方互換性あり）。

### NFR-2: `record` の出力サイズの増加

libc / libSystem のすべてのシンボルを記録するため、`DetectedSymbols` の件数が増加する。タスク 0105 の NFR-2 と同様にセキュリティ正確性を優先し許容する。

### NFR-3: スキーマバージョンの非変更とセマンティクス変更時の安全なフォールバック

`CurrentSchemaVersion` は変更しない。

ただし、本タスクにより `DetectedSymbols` のセマンティクスが変化する（ネットワークシンボルのみ → libc / libSystem の全シンボル）。スキーマバージョンを変えないため、`SchemaVersionMismatchError` は発生しない。一方、旧 `record` が生成したキャッシュと新 `runner` の間で意味的な不整合が生じうる。

この意味的な互換性は NFR-1 で保証する。旧キャッシュの `DetectedSymbols` にはネットワーク系カテゴリのシンボルのみが記録されており、新 `runner` がカテゴリベースのネットワーク判定を行っても結果は変わらない。

将来スキーマバージョンを変更する場合（本タスクのスコープ外）、`SchemaVersionMismatchError` が発生したときは high-risk 扱いではなくライブ解析（バイナリ直接解析）へフォールバックすること。これにより、古いキャッシュによる誤判定を防ぎつつ安全に解析を継続できる。

## 5. 受け入れ基準

### AC-1: ELF バイナリの libc シンボル全記録

- libc から `socket()` と `read()` の両方をインポートする ELF バイナリに対して `record` を実行すると、両シンボルが `detected_symbols` に記録されること
- `socket()` のカテゴリが `"socket"` であること
- `read()` のカテゴリが `"syscall_wrapper"` であること

### AC-2: ELF バイナリの非 libc シンボル非記録

- libc 以外のライブラリ（例: `libm`）からのみシンボルをインポートする ELF バイナリで、libc シンボルをインポートしない場合、`detected_symbols` が空であること

### AC-3: Mach-O バイナリの libSystem シンボル全記録

- libSystem から `socket` と `read` の両方をインポートする Mach-O バイナリに対して `record` を実行すると、両シンボルが `detected_symbols` に記録されること
- `socket` のカテゴリが `"socket"` であること
- `read` のカテゴリが `"syscall_wrapper"` であること

### AC-4: `runner` のネットワーク判定正確性（シンボル経由）

- `Category` が `"socket"` / `"dns"` / `"tls"` / `"http"` 等のネットワーク系カテゴリのシンボルを含むレコードで `runner` を実行すると、ネットワーク操作あり（`isNetwork = true`）と判定されること
- `Category == "syscall_wrapper"` のシンボルのみを含むレコードで `runner` を実行すると、シンボル由来のネットワーク操作なし（`isNetwork = false`）と判定されること

### AC-5: 既存テストの通過

- `make test` / `make lint` がエラーなしで通過すること
- ELF・Mach-O アナライザのテストが新しいフィルタロジックを検証すること
- `network_analyzer_test.go` のシンボル判定テストが `Category` フィールドを基準とした検証へ更新されること

## 6. 先行タスクとの関係

| タスク | 関係 |
|-------|------|
| 0076 ネットワークシンボルキャッシュ | `networkSymbols` マップを導入したタスク。本タスクで「ネットワーク限定フィルタ」としての役割を廃止し、カテゴリ付与のみに使用する |
| 0082 dynlib シンボル解析 | `DetectedSymbols` と `DynamicLoadSymbols` の記録構造を導入したタスク |
| 0100 Mach-O libSystem キャッシュ | `MachoLibSystemCache` による libSystem シンボルの識別基盤。本タスクの FR-2.1 で活用できる可能性がある |
| 0105 record フィルタ削除 | システムコールフィルタリングを削除したタスク。本タスクはシンボル解析で同様の関心の分離を実現する |
