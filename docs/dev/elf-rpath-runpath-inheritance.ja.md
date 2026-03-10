# ELF DT_RPATH / DT_RUNPATH 継承ルール

このドキュメントは、Linux ld.so における `DT_RPATH` と `DT_RUNPATH` の検索・継承動作を整理したリファレンスです。`dynlibanalysis` パッケージの設計根拠として参照されます。

## 1. 基本的な違い

| 属性 | 適用範囲 | 継承 | LD_LIBRARY_PATH との順序 |
|------|---------|------|------------------------|
| `DT_RPATH` | 直接依存 + **推移的依存全体** | **継承される**（後述の打ち切り条件を除く） | `DT_RPATH` → LD_LIBRARY_PATH → ... |
| `DT_RUNPATH` | **直接依存のみ** | 継承されない | LD_LIBRARY_PATH → `DT_RUNPATH` → ... |

> **出典**: `man 8 ld.so` — "DT_RUNPATH directories are searched only to find objects required by DT_NEEDED entries and do not apply to those objects' children, which must themselves have their own DT_RUNPATH entries. This is unlike DT_RPATH, which is applied to searches for all children in the dependency tree."

## 2. ld.so の検索順序

ld.so が soname を解決するときの検索順序（glibc 実装に基づく）:

1. **DT_RPATH** — loading object（解決対象のライブラリを「読み込む」ELF）の `DT_RPATH`。ただし loading object が `DT_RUNPATH` を持つ場合は **スキップ**
2. **祖先の DT_RPATH 継承チェーン** — loading object の loader（親）→ さらにその loader（祖父）... と遡り、それぞれの `DT_RPATH` を検索する。ただし途中で `DT_RUNPATH` を持つ ELF に当たった時点で **打ち切り**（後述 §3）
3. **LD_LIBRARY_PATH** — 環境変数（セキュリティ上 record 時には使わない）
4. **DT_RUNPATH** — loading object の `DT_RUNPATH`
5. **/etc/ld.so.cache**
6. **デフォルトパス** — `/lib`, `/usr/lib` 等（アーキテクチャ依存）

> **出典**: glibc `elf/dl-load.c` `_dl_map_object` の実装。"Unless the loading object has RUNPATH, the RPATH of the loading object is checked, then the RPATH of its loader (unless it has a RUNPATH), and so on until the end of the chain."

## 3. DT_RPATH 継承チェーンの打ち切りルール

**打ち切りが発生する条件**:

> loading object（解決するライブラリを読み込む ELF）自身が `DT_RUNPATH` を持つ場合、祖先 RPATH の継承チェーンは打ち切られる。

この表現は glibc のコードそのものの挙動です:
- loading object に `DT_RUNPATH` があれば、自分の `DT_RPATH` は使わず（Step 1 スキップ）、祖先の RPATH チェーンも辿らない（Step 2 スキップ）
- loading object に `DT_RUNPATH` がなければ、自分の `DT_RPATH` を使い、さらに自分の loader（親）に遡る。ただし親が `DT_RUNPATH` を持っていたら、そこで打ち切り

### 具体例

```
main(RPATH=/gp) → libA(no RPATH, no RUNPATH) → libB(RUNPATH=/b) → libC
```

| 解決対象 | loading object | 使用される検索パス | 理由 |
|---------|---------------|-------------------|------|
| libA | main | /gp（main の RPATH） | main に RUNPATH なし → 自分の RPATH を使う |
| libB | libA | /gp（main の RPATH 継承） | libA に RPATH/RUNPATH なし → loader(main) の RPATH に遡る |
| libC | libB | /b（libB の RUNPATH） | libB に RUNPATH あり → 自分の RPATH(なし)も祖先チェーンも使わない |

```
grandparent(RPATH=/gp) → parent(RPATH=/p, no RUNPATH) → child(RUNPATH=/c) → grandchild
```

| 解決対象 | loading object | 使用される検索パス | 理由 |
|---------|---------------|-------------------|------|
| parent | grandparent | /gp | grandparent に RUNPATH なし |
| child | parent | /p, /gp（継承） | parent に RUNPATH なし → /p を使い、さらに grandparent の /gp を継承 |
| grandchild | child | /c のみ | child に RUNPATH あり → 祖先チェーン（/p, /gp）を使わない |

## 4. $ORIGIN 展開

`$ORIGIN` は、`$ORIGIN` を含む `DT_RPATH`/`DT_RUNPATH` エントリを持つ **ELF ファイルが置かれているディレクトリ** に展開される。

継承された RPATH エントリの `$ORIGIN` は、**そのエントリを定義した ELF のディレクトリ**（= `OriginDir`）で展開される。loading object のディレクトリではない。

```
/app/bin/main (RPATH=$ORIGIN/../lib) → /app/lib/libA.so → /app/lib/libB.so
```

libB を解決するとき、`$ORIGIN` は main のディレクトリ `/app/bin` に展開されるため、検索パスは `/app/bin/../lib` = `/app/lib` となる。

## 5. DT_RPATH と DT_RUNPATH の同時存在

同じ ELF に両方が存在する場合、`DT_RUNPATH` が優先され `DT_RPATH` は無視される。

glibc の実装では `DT_RPATH` は `DT_RUNPATH` が存在しない場合にのみ使用される。

## 6. dynlibanalysis パッケージでの設計対応

`dynlibanalysis` パッケージは ld.so アルゴリズムの**セキュリティ制限サブセット**を実装している。`DT_RPATH` と `LD_LIBRARY_PATH` はどちらもサポートされない。ELF ファイル（バイナリ・ライブラリを問わず）に `DT_RPATH` が含まれている場合、`Analyze()` は直ちに `ErrDTRPATHNotSupported` を返す。ELF 組み込みの検索パスとして参照されるのは `DT_RUNPATH` のみである。

### DT_RPATH と LD_LIBRARY_PATH を除外する理由

`DT_RPATH` は依存グラフ全体に推移的に継承され、`LD_LIBRARY_PATH` より前に検索されるため、権限昇格やライブラリハイジャックの既知のベクターとなっている。これを拒否することで、解決ロジックをシンプルに保ち、セキュリティ特性を明確にする。

`LD_LIBRARY_PATH` は、`record` 実行時には再現性確保のため無視し、`verify` 実行時にはハイジャック防止のためクリアする。いずれの場合も依存解決には使用しない。

### 主要な型

| 型 | 役割 |
|----|------|
| `DynLibAnalyzer` | エントリポイント。`/etc/ld.so.cache` を一度だけパースし、BFS トラバーサルを駆動する |
| `LibraryResolver` | 単一の soname をファイルシステムパスに解決する。RUNPATH → cache → デフォルトパスの順で検索 |

### BFS トラバーサルと RUNPATH の伝播

`DynLibAnalyzer.Analyze()` は依存グラフに対して BFS を実行する。各キューアイテムは以下を保持する:

- `soname` — 解決対象のライブラリ名
- `parentPath` — この soname を `DT_NEEDED` に持つ ELF のパス
- `runpath` — `parentPath` の `DT_RUNPATH` エントリ（`soname` の解決に使用）
- `depth` — 再帰ガード

ライブラリが解決されると、その `DT_NEEDED` と `DT_RUNPATH` が `parseELFDeps()` で抽出され、新たなキューアイテムとしてエンキューされる。各子は**自身の直接の親**の `DT_RUNPATH` を使って解決される。祖先の `DT_RUNPATH` は使用されない。これは ld.so の `DT_RUNPATH` 非継承ルールに自然に対応しており、複雑な祖先 RPATH チェーンロジックを完全に回避している。

### LibraryResolver.Resolve() の検索順序

```
1. 親 ELF の DT_RUNPATH エントリ（$ORIGIN → filepath.Dir(parentPath)）
2. /etc/ld.so.cache
3. デフォルトパス（アーキテクチャ依存、例: /lib, /usr/lib）
```

`LD_LIBRARY_PATH` は省略される。`record` は再現性のために無視し、`verify` はセキュリティのためにクリアする。

### ld.so ルールとの対応

| ld.so のルール | dynlibanalysis の動作 |
|--------------|----------------------|
| `DT_RPATH` は `LD_LIBRARY_PATH` より前に検索 | **未実装** — いずれかの ELF に `DT_RPATH` があれば `ErrDTRPATHNotSupported` |
| `DT_RUNPATH` は直接依存のみに適用 | 自然に保証される: 各 `resolveItem` は直接の親の `runpath` のみを保持する |
| `DT_RUNPATH` は祖先 RPATH チェーンを打ち切る | N/A — 祖先 RPATH チェーン自体を構築しない |
| `$ORIGIN` 展開 | `expandOrigin()` が `$ORIGIN`/`${ORIGIN}` を `filepath.Dir(parentPath)` に置換 |
| `DT_RUNPATH` は `DT_RPATH` を上書きする | N/A — `DT_RPATH` は無条件に拒否される |

## 7. よくある誤解

### 誤解 1: 「child が RUNPATH を持つと、child 自身の解決では ancestor RPATH が引き続き使われる」

**誤り**。glibc は loading object（child）が RUNPATH を持つ場合、loader のRPATH チェーン全体を辿らない。`InheritedRPATH = nil` が正しい。

### 誤解 2: 「継承打ち切りは『祖先のどこかが RUNPATH を持つ場合』に発生する」

**誤り**。打ち切りの判断は **loading object 自身** が RUNPATH を持つかどうかで決まる。祖先が RUNPATH を持っていても、それより下流で RUNPATH を持たない ELF が loading object になる場合はチェーンが継続する。

```
main(RPATH=/gp) → libA(RUNPATH=/a) → libB(no RPATH, no RUNPATH) → libC
```

- libB が loading object のとき: libB に RUNPATH なし → loader(libA) の RPATH チェーンに遡る
  - libA は RUNPATH を持つため、libA の段階で打ち切り
  - libA の RUNPATH(/a) は libA の **直接依存（libB）の解決にのみ** 使われ、libC には適用されない
  - libA の loader(main) の /gp も libC の解決に **使われない**
- よって libC の検索パス: RPATH/RUNPATH なし → LD_LIBRARY_PATH → /etc/ld.so.cache → デフォルトパスの順

### 誤解 3: 「DT_RUNPATH は DT_RPATH と同じ検索順序だが継承されないだけ」

**誤り**。検索順序も違う。`DT_RPATH` は `LD_LIBRARY_PATH` より**前**に検索されるが、`DT_RUNPATH` は `LD_LIBRARY_PATH` より**後**に検索される。これが `LD_LIBRARY_PATH` ハイジャック検出において重要な差異となる。

## 8. 参照

- `man 8 ld.so` (Linux manual page): https://man7.org/linux/man-pages/man8/ld.so.8.html
- glibc ソース: `elf/dl-load.c` の `_dl_map_object` 関数
- 実装: [`internal/dynlibanalysis/resolver.go`](../../internal/dynlibanalysis/resolver.go)
- 実装: [`internal/dynlibanalysis/analyzer.go`](../../internal/dynlibanalysis/analyzer.go)
- 仕様書: [`docs/tasks/0074_elf_dynlib_integrity/03_detailed_specification.md`](../tasks/0074_elf_dynlib_integrity/03_detailed_specification.md) §3.3, §3.4
