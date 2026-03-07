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

上記のルールを `ResolveContext` でどう表現しているか:

| ld.so のルール | `ResolveContext` での表現 |
|--------------|--------------------------|
| 自分の RPATH（RUNPATH なし時）| `OwnRPATH` |
| 自分の RUNPATH | `OwnRUNPATH` |
| 祖先から継承された RPATH チェーン | `InheritedRPATH []ExpandedRPATHEntry`（各エントリに `OriginDir` 付き） |
| RUNPATH があれば OwnRPATH を無効化 | `NewRootContext`/`NewChildContext` で `OwnRUNPATH` と `OwnRPATH` を排他的に設定 |
| loading object に RUNPATH があれば継承チェーン打ち切り | `NewChildContext` で `childRUNPATH` が非空の場合 `InheritedRPATH = nil` |

### NewChildContext の打ち切りロジック

```go
if len(childRUNPATH) > 0 {
    child.OwnRUNPATH = childRUNPATH
    // InheritedRPATH は nil のまま（打ち切り）
} else {
    // InheritedRPATH に親・祖先の RPATH を積む
}
```

これは「child が RUNPATH を持つ場合、child の DT_NEEDED 解決で祖先 RPATH を使わない」という glibc の動作に対応する。`c.InheritedRPATH`（= 既に親が継承していた祖先 RPATH）も含めて破棄するのは正しい。なぜなら glibc は loading object（child）が RUNPATH を持つ時点でローダーチェーン全体の遡りを停止するためである。

### Resolve() の検索順序

```
1. OwnRPATH     （OwnRUNPATH なし時のみ）
2. InheritedRPATH
3. LD_LIBRARY_PATH（runner 実行時のみ）
4. OwnRUNPATH
5. /etc/ld.so.cache
6. デフォルトパス
```

`OwnRUNPATH` が Step 4 なのは、ld.so の検索順序（RPATH → LD_LIBRARY_PATH → RUNPATH → cache → default）に対応している。

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
- 実装: [`internal/dynlibanalysis/resolver_context.go`](../../internal/dynlibanalysis/resolver_context.go)
- 実装: [`internal/dynlibanalysis/resolver.go`](../../internal/dynlibanalysis/resolver.go)
- 仕様書: [`docs/tasks/0074_elf_dynlib_integrity/03_detailed_specification.md`](../tasks/0074_elf_dynlib_integrity/03_detailed_specification.md) §3.3, §3.4
