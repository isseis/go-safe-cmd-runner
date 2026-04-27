# 要件定義書: record 側リスク分類フィールドの除去

## 1. 概要

### 1.1 背景

`record` コマンドは実行ファイルを解析し、その結果をキャッシュファイルに記録する。`runner` はそのキャッシュを参照してリスク判断を行う。

現行のスキーマには、`record` が分類判断を行い、その結果を記録しているフィールドが 2 つ存在する。

**`SyscallInfo.IsNetwork`**（`common.SyscallInfo`）

`record` が syscall 番号をシステムコールテーブルに照合し、「ネットワーク系か否か」を判定した結果を `is_network: true/false` として JSON に保存している。しかし、この判定はシステムコール番号が記録されていれば `runner` 起動時にテーブルを 1 回引くだけで導出できる（O(1)）。`record` がこのフラグを焼き込むと、「ネットワーク系 syscall の定義」がスキーマに固定され、定義を変更するたびにスキーマバージョンアップと全ファイルの再 `record` が必要になる。

**`DetectedSymbolEntry.Category`**（`fileanalysis.DetectedSymbolEntry`）

`record` が検出したシンボル名をシンボルレジストリに照合し、`dns` / `socket` / `tls` / `http` / `syscall_wrapper` という分類文字列を記録している。しかし `runner` がこのフィールドを使うのは `IsNetworkCategory(sym.Category)` の真偽判定のみであり、`dns` と `socket` の区別は最終的なリスク判断に影響しない。`runner` はシンボル名から `binaryanalyzer.IsNetworkSymbol(name)` を呼べば同等の判定を行える。`Category` 文字列を記録し続けることは、分類定義の変更をスキーマ変更に直結させる不要な結合を生む。

### 1.2 目的

- `SyscallInfo.IsNetwork` をスキーマから除去し、`runner` が syscall 番号からネットワーク判定を導出する設計に変更する。
- `DetectedSymbolEntry.Category` をスキーマから除去し、`runner` がシンボル名からネットワーク判定を導出する設計に変更する。
- `record` を「観測事実の記録者」、`runner` を「リスク判断の実施者」という責務分担に沿った設計に整合させる。

### 1.3 スコープ

#### 対象

| 対象 | 変更内容 |
|------|---------|
| `internal/common/syscall_types.go` | `SyscallInfo.IsNetwork` フィールドを削除 |
| `internal/common/syscall_grouping.go` | `IsNetwork` の伝播ロジックを削除 |
| `internal/fileanalysis/schema.go` | `DetectedSymbolEntry.Category` フィールドを削除、スキーマバージョンを 18 に更新 |
| `internal/filevalidator/validator.go` | `IsNetwork` の設定を削除、`Category` の設定を削除 |
| `internal/runner/security/elfanalyzer/syscall_analyzer.go` | `IsNetwork` の設定を削除 |
| `internal/runner/security/machoanalyzer/pass1_scanner.go` | `IsNetwork` の設定を削除 |
| `internal/runner/security/machoanalyzer/pass2_scanner.go` | `IsNetwork` の設定を削除 |
| `internal/libccache/adapters.go` | `IsNetwork` の設定を削除 |
| `internal/libccache/matcher.go` | `IsNetwork` の設定を削除 |
| `internal/runner/security/network_analyzer.go` | `syscallAnalysisHasNetworkSignal` を syscall 番号ベースの判定に変更、`convertNetworkSymbolEntries` を名前ベースの判定に変更 |
| `internal/runner/security/binaryanalyzer/network_symbols.go` | `IsNetworkSymbolName` を追加し、名前ベース判定を共通化 |
| 上記に伴うテストの更新 | — |

#### 対象外

- `libccache` パッケージ独自のスキーマ（`LibcCacheFile`）の変更
- `DynamicLoadSymbols` フィールドの廃止（別リスクシグナルのため維持）
- `IsNetworkSyscall` / `IsNetworkSymbol` 等の分類テーブル自体の変更
- `DetectedSymbols` に記録するシンボルの取捨選択（`syscall_wrapper` を記録するか否か）の変更

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| 観測事実 | `record` が実行ファイルを解析して得た生データ。syscall 番号、シンボル名、ライブラリパス、ファイルハッシュなど |
| リスク判断 | 観測事実に分類ポリシーを適用してリスクを決定すること。`runner` の責務 |
| `IsNetwork`（syscall） | syscall 番号がネットワーク系（socket, connect, sendto 等）かの真偽値。現行は `SyscallInfo` に保存されているが、本タスクで除去する |
| `Category`（symbol） | シンボルの分類文字列（`dns` / `socket` / `tls` / `http` / `syscall_wrapper`）。現行は `DetectedSymbolEntry` に保存されているが、本タスクで除去する |
| syscall テーブル | syscall 番号 → 名前・ネットワーク判定のマッピング（`X86_64SyscallTable`, `ARM64LinuxSyscallTable` 等） |
| シンボルレジストリ | シンボル名 → カテゴリのマッピング（`binaryanalyzer.networkSymbolRegistry`） |

## 3. 機能要件

### FR-1: `SyscallInfo.IsNetwork` の除去

`common.SyscallInfo` から `IsNetwork bool` フィールドおよび対応する JSON タグ `is_network` を削除する。

**変更前:**
```go
type SyscallInfo struct {
    Number      int                `json:"number"`
    Name        string             `json:"name,omitempty"`
    IsNetwork   bool               `json:"is_network"`
    Occurrences []SyscallOccurrence `json:"occurrences,omitempty"`
}
```

**変更後:**
```go
type SyscallInfo struct {
    Number      int                `json:"number"`
    Name        string             `json:"name,omitempty"`
    Occurrences []SyscallOccurrence `json:"occurrences,omitempty"`
}
```

`IsNetwork` フィールドを設定していた全箇所（elfanalyzer、machoanalyzer、libccache、filevalidator）から対応するコードを削除する。

### FR-2: `syscallAnalysisHasNetworkSignal` の syscall 番号ベース判定への変更

`runner/security/network_analyzer.go` の `syscallAnalysisHasNetworkSignal` 関数を、`s.IsNetwork` の参照から、syscall テーブルへの問い合わせに変更する。

**変更前:**
```go
func syscallAnalysisHasNetworkSignal(result *fileanalysis.SyscallAnalysisResult) bool {
    for _, s := range result.DetectedSyscalls {
        if s.IsNetwork {
            return true
        }
    }
    return false
}
```

**変更後（概念）:**
```go
func syscallAnalysisHasNetworkSignal(result *fileanalysis.SyscallAnalysisResult) bool {
    table := syscallTableForArch(result.Architecture)
    if table == nil {
        return false
    }
    for _, s := range result.DetectedSyscalls {
        if s.Number >= 0 && table.IsNetworkSyscall(s.Number) {
            return true
        }
    }
    return false
}
```

`syscallAnalysisHasNetworkSignal` の関数シグネチャは変更しない。`table` は関数内でアーキテクチャ（`result.Architecture`）から選択する。アーキテクチャが不明または未サポートの場合は「ネットワーク検知をスキップ（`false` を返す）」動作とする（fail-open 挙動）。

### FR-3: `DetectedSymbolEntry.Category` の除去

`fileanalysis.DetectedSymbolEntry` から `Category string` フィールドおよび対応する JSON タグ `category` を削除する。

**変更前:**
```go
type DetectedSymbolEntry struct {
    Name     string `json:"name"`
    Category string `json:"category"`
}
```

**変更後:**
```go
type DetectedSymbolEntry struct {
    Name string `json:"name"`
}
```

`Category` フィールドを設定していた全箇所（`filevalidator/validator.go:convertDetectedSymbols`）から対応するコードを削除する。

### FR-4: runner 側のシンボルネットワーク判定の名前ベース変更

`runner/security/network_analyzer.go` の `convertNetworkSymbolEntries` が `Category` を `binaryanalyzer.DetectedSymbol` に引き渡していた部分を変更する。runner 内部の `binaryanalyzer.DetectedSymbol` への変換時に `binaryanalyzer.IsNetworkSymbol(name)` を呼び出してカテゴリを導出する。

**変更前（概念）:**
```go
syms[i] = binaryanalyzer.DetectedSymbol{Name: e.Name, Category: e.Category}
```

**変更後（概念）:**
```go
cat, _ := binaryanalyzer.IsNetworkSymbol(e.Name)
syms[i] = binaryanalyzer.DetectedSymbol{Name: e.Name, Category: string(cat)}
```

`runner` 内部の `binaryanalyzer.DetectedSymbol` は引き続き `Category` フィールドを持ち、ログ出力等に利用してよい。除去の対象は **永続化スキーマ**（`fileanalysis.DetectedSymbolEntry`）の `category` フィールドに限る。

### FR-5: スキーマバージョンの更新

`fileanalysis.CurrentSchemaVersion` を 18 に更新し、バージョン履歴コメントに本変更の概要を追記する。

`Load` は `schema_version != 18` のレコードに対して `SchemaVersionMismatchError` を返す（既存の動作）。古いスキーマ（`Actual < Expected`）のレコードは `Store.Update` が上書き可能と扱い、`record` の再実行で自動移行される（既存の動作）。

## 4. 非機能要件

### NFR-1: 外部動作の不変

本変更はスキーマのクリーンアップであり、`runner` のネットワーク判定結果は変更前後で同一でなければならない。

- syscall テーブルのネットワーク判定セットは変更しない
- シンボルレジストリのネットワーク判定セットは変更しない
- これらは変更前後で同一の判定を与えるため、`runner` の `IsNetworkOperation` の返り値は変わらない

### NFR-2: 古いレコードの扱い

`schema_version < 18` のレコードを読み込んだ場合は `SchemaVersionMismatchError` を返す（既存の動作と同じ）。`record` を再実行することで新スキーマのレコードが生成される。

## 5. 受け入れ基準

### AC-1: `SyscallInfo` に `is_network` フィールドが存在しない

- `record` が出力する JSON に `is_network` キーが含まれないこと
- `fileanalysis.SyscallAnalysisData` をロードしたとき、`SyscallInfo.IsNetwork` フィールドへのアクセスがコンパイルエラーになること（フィールド削除により保証）

### AC-2: `DetectedSymbolEntry` に `category` フィールドが存在しない

- `record` が出力する JSON に `detected_symbols[].category` キーが含まれないこと
- `fileanalysis.DetectedSymbolEntry.Category` フィールドへのアクセスがコンパイルエラーになること（フィールド削除により保証）

### AC-3: ネットワーク検出結果が変更前後で一致する

- 既存のテストバイナリ（ELF/Mach-O）に対して `runner` の `IsNetworkOperation` が返す `(isNetwork, isHighRisk)` の値が、スキーマ変更前後で同一であること

### AC-4: スキーマバージョンが 18 に更新されている

- `fileanalysis.CurrentSchemaVersion == 18` であること
- `schema_version: 17` のレコードをロードしたとき `SchemaVersionMismatchError` が返ること

### AC-5: 古いスキーマのレコードが自動移行される

- `schema_version: 17` のレコードが存在する状態で `record` を実行すると、`--force` なしで新レコードが書き込まれること（`Store.Update` の既存動作）

### AC-6: `make test` および `make lint` が通過する

- 全テストがエラーなしで通過すること
- リンターエラーがないこと

### AC-7: FR-2 の検知スキップ挙動（fail-open）が満たされる

- `result.Architecture` が不明または未サポートの場合、`syscallAnalysisHasNetworkSignal` は `false` を返すこと
- syscall 番号が負値など無効値の場合、ネットワーク判定に使用せず安全に処理されること

## 6. 先行タスクとの関係

| タスク | 関係 |
|--------|------|
| 0107 syscall のグループ化 | `IsNetwork` を `GroupAndSortSyscalls` が伝播する実装を導入したタスク。本タスクで伝播ロジックを削除する |
| 0104 Mach-O syscall 番号解析 | `pass1_scanner` / `pass2_scanner` で `IsNetwork` を設定する実装を導入したタスク |
| 0100 Mach-O libSystem syscall キャッシュ | `libccache` アダプターで `IsNetwork` を設定する実装を導入したタスク |
