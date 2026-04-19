# Mach-O 既知ネットワークライブラリ検出 要件定義書

## 1. 概要

### 1.1 背景

タスク 0082（方策 C）で ELF バイナリの `DT_NEEDED` に含まれる SOName（例: `libruby.so.3.2`）を既知ネットワークライブラリリストと照合し、`SymbolAnalysisData.KnownNetworkLibDeps` に記録する仕組みを実装した。

タスク 0096 で Mach-O バイナリの `LC_LOAD_DYLIB` 依存関係を `DynLibDeps` に記録するようになった。しかし、現在の `KnownNetworkLibDeps` 導出ロジック（`validator.go`）は `lib.SOName` を `IsKnownNetworkLibrary()` に直接渡すため、Mach-O では正しく機能しない。

**問題の詳細:**

ELF の `LibEntry.SOName` はライブラリのベース名（例: `libruby.so.3.2`）だが、Mach-O の `LibEntry.SOName` は `LC_LOAD_DYLIB` のインストール名、すなわちフルパス（例: `/usr/local/opt/ruby/lib/libruby.3.2.dylib`）である。

`IsKnownNetworkLibrary()` は `strings.HasPrefix(soname, prefix)` による前方一致で動作するため、フルパスに対してはプレフィックス一致が成立せず、既知ネットワークライブラリが検出されない。

### 1.2 採用アプローチ

Mach-O の `LibEntry.SOName`（インストール名）からベース名（例: `libruby.3.2.dylib`）を抽出し、既存の `IsKnownNetworkLibrary()` に渡す正規化ステップを追加する。

**設計方針:**

- **プレフィックスリストは共用**: ELF 版の `knownNetworkLibPrefixes`（`known_network_libs.go`）をそのまま再利用する。Mach-O 専用リストは不要。
- **最小変更**: Mach-O 側での追加実装はインストール名 → ベース名の正規化のみ。
- **`KnownNetworkLibDeps` の記録値**: インストール名をそのまま記録する（`libruby.so.3.2` でなく `/usr/local/opt/ruby/lib/libruby.3.2.dylib`）。runner 判定では非空かどうかのみを見るため、値の形式は問わない。

**既存の `matchesKnownPrefix()` との互換性:**

`libruby.3.2.dylib`（ベース名）に対して `matchesKnownPrefix("libruby.3.2.dylib", "libruby")` を実行すると:

- `rest = ".3.2.dylib"`、`rest[0] = '.'` → 一致 ✓

`libpythonista.dylib` に対して `matchesKnownPrefix("libpythonista.dylib", "libpython")` を実行すると:

- `rest = "ista.dylib"`、`rest[0] = 'i'` → 不一致 ✓

既存の区切り文字チェック（`.`、`-`、数字）は `.dylib` の Mach-O 命名規則でも正しく機能する。

### 1.3 スコープ

- **対象**: Mach-O バイナリの `DynLibDeps` を対象とした `KnownNetworkLibDeps` 導出
- **対象外**: ELF バイナリの既存の SOName ベース検出（変更なし）
- **対象外**: `knownNetworkLibPrefixes` への Mach-O 専用エントリの追加
- **対象外**: dyld shared cache 内のライブラリ（タスク 0096 にてスキップ済みのため `DynLibDeps` に存在しない）

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| インストール名 | `LC_LOAD_DYLIB` に記録されたライブラリのパス文字列（例: `/usr/local/opt/ruby/lib/libruby.3.2.dylib`） |
| ベース名 | インストール名の末尾ファイル名部分（例: `libruby.3.2.dylib`）。`filepath.Base()` により取得 |
| SOName | ELF の `DT_NEEDED` に記録されたライブラリ名（例: `libruby.so.3.2`）。`LibEntry.SOName` フィールドに格納されるが、Mach-O ではインストール名が格納される |
| 既知ネットワークライブラリリスト | `binaryanalyzer.knownNetworkLibPrefixes` に定義された SOName プレフィックスのリスト |

---

## 3. 機能要件

### FR-1: インストール名のベース名正規化

Mach-O バイナリの `DynLibDeps` に格納されたインストール名（`LibEntry.SOName`）に対して `KnownNetworkLibDeps` を導出する際、`filepath.Base()` でベース名を抽出してから `IsKnownNetworkLibrary()` に渡す。

**正規化ルール:**

| 入力（インストール名） | ベース名 | 既知ライブラリ判定 |
|---|---|---|
| `/usr/local/opt/ruby/lib/libruby.3.2.dylib` | `libruby.3.2.dylib` | 一致（プレフィックス `libruby`） |
| `/usr/local/lib/libcurl.4.dylib` | `libcurl.4.dylib` | 一致（プレフィックス `libcurl`） |
| `/usr/local/opt/python/lib/libpython3.11.dylib` | `libpython3.11.dylib` | 一致（プレフィックス `libpython`） |
| `@rpath/libcurl.dylib` | `libcurl.dylib` | 一致（プレフィックス `libcurl`） |
| `/usr/local/lib/libz.1.dylib` | `libz.1.dylib` | 不一致（リストにない） |
| `/usr/local/lib/libpythonista.dylib` | `libpythonista.dylib` | 不一致（区切り文字条件を満たさない） |

**ベース名抽出の実装:**

```
base := filepath.Base(lib.SOName)
if IsKnownNetworkLibrary(base) {
    matched = append(matched, lib.SOName)  // インストール名を記録
}
```

インストール名がスラッシュを含まない場合（ELF の SOName 形式）は `filepath.Base()` により変化しないため、ELF と Mach-O の両方に同じロジックを適用できる。

### FR-2: KnownNetworkLibDeps への記録

一致したライブラリのインストール名を `SymbolAnalysisData.KnownNetworkLibDeps` に記録する。ELF との相違として、記録される文字列はインストール名（フルパス）となる。

このフィールドが非空の場合、runner 実行時にネットワーク有りと判定される（既存の `network_analyzer.go` の判定ロジックを変更しない）。

### FR-3: 既存 ELF 検出との共存

`IsKnownNetworkLibrary()` および `matchesKnownPrefix()` は変更しない。ELF バイナリの `DynLibDeps` に対する既存の検出動作は維持される。

### FR-4: スキーマバージョンの非更新

`KnownNetworkLibDeps` フィールドは既にスキーマバージョン 8 で導入済みであり、本タスクではスキーマ変更は不要。

---

## 4. 実装方針

### 4.1 変更対象ファイル

| ファイル | 変更内容 |
|---|---|
| `internal/filevalidator/validator.go` | `KnownNetworkLibDeps` 導出ループでベース名正規化を追加 |

### 4.2 変更の最小化方針

`validator.go` の既存ループを以下のように修正するだけで実装可能：

```go
// 変更前
if binaryanalyzer.IsKnownNetworkLibrary(lib.SOName) {
    matched = append(matched, lib.SOName)
}

// 変更後
base := filepath.Base(lib.SOName)
if binaryanalyzer.IsKnownNetworkLibrary(base) {
    matched = append(matched, lib.SOName)
}
```

ELF の SOName（例: `libruby.so.3.2`）は `/` を含まないため `filepath.Base()` で変化せず、既存の ELF 動作は維持される。

---

## 5. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | `/usr/local/opt/ruby/lib/libruby.3.2.dylib` を `LC_LOAD_DYLIB` に持つ Mach-O バイナリを record すると、`KnownNetworkLibDeps` にそのインストール名が記録される |
| AC-2 | `/usr/local/lib/libcurl.4.dylib` を `LC_LOAD_DYLIB` に持つ Mach-O バイナリを record すると、`KnownNetworkLibDeps` にそのインストール名が記録される |
| AC-3 | `/usr/local/opt/python/lib/libpython3.11.dylib` を `LC_LOAD_DYLIB` に持つ Mach-O バイナリを record すると、`KnownNetworkLibDeps` にそのインストール名が記録される |
| AC-4 | `KnownNetworkLibDeps` が非空の Mach-O バイナリは runner 実行時にネットワーク有りと判定される |
| AC-5 | `/usr/lib/libz.1.dylib` のように既知ネットワークライブラリリストに含まれないライブラリは `KnownNetworkLibDeps` に記録されない |
| AC-6 | `/usr/local/lib/libpythonista.dylib` のように登録プレフィックスで始まっても区切り条件を満たさないライブラリは `KnownNetworkLibDeps` に記録されない |
| AC-7 | ELF バイナリの `DynLibDeps`（SOName 形式: `libruby.so.3.2` 等）に対する既存の `KnownNetworkLibDeps` 導出動作が変わらないこと |
| AC-8 | `SymbolAnalysis` が nil の場合（静的バイナリ等）、Mach-O であっても `KnownNetworkLibDeps` は記録されない |

---

## 6. 関連タスク・依存関係

| タスク | 関係 |
|--------|------|
| 0082 (ELF SOName ベース検出) | 本タスクの Mach-O 版。`knownNetworkLibPrefixes` と `IsKnownNetworkLibrary()` を共用 |
| 0095 (Mach-O 機能パリティ) | 本タスクは FR-4.8 を実装する |
| 0096 (Mach-O LC_LOAD_DYLIB 整合性検証) | Mach-O `DynLibDeps` 記録の基盤。本タスクはその `DynLibDeps` を利用 |
