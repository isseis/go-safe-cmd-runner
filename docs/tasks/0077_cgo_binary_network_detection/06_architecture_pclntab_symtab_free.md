# アーキテクチャ設計書: pclntab オフセット検出の .symtab 非依存化

## 1. 問題の再整理

```mermaid
flowchart TD
    subgraph 現行実装["現行実装（Strip 済みバイナリで失敗）"]
        A[ParsePclntab] --> B["gosym.NewLineTable<br>textStart = .text.Addr"]
        B --> C["fn.Entry ← ずれたアドレス<br>（CGO バイナリのみ）"]
        C --> D[detectPclntabOffset]
        D --> E{.symtab あり？}
        E -->|"あり<br>（not stripped）"| F["symtab と照合<br>→ offset 検出 ✅"]
        E -->|"なし<br>（stripped）"| G["offset = 0<br>補正なし ❌"]
        G --> H["GoWrapperResolver<br>アドレスずれで Pass 2 失敗"]
    end

    style G fill:#ffb347
    style H fill:#ff6b6b
```

```mermaid
flowchart TD
    subgraph 目標["目標（Strip 済みバイナリでも動作）※案 A 廃止・案 B 単独採用・window exact-match"]
        A2[ParsePclntab] --> M2{"pclntab magic 確認<br>checkPclntabVersion"}
        M2 -->|"magic ≠ 0xfffffff1"| ERR2["ErrUnsupportedPclntabVersion ❌"]
        M2 -->|"magic = 0xfffffff1<br>（Go 1.20+）"| B2["gosym.NewLineTable<br>textStart = .text.Addr"]
        B2 --> C2["fn.Entry ← ずれたアドレス<br>（CGO バイナリのみ）"]
        C2 --> D2["detectOffsetByCallTargets<br>CALL/BL ターゲット相互参照<br>window exact-match<br>（.symtab 不要）"]
        D2 --> H2["正しい offset ✅"]
        H2 --> I2["GoWrapperResolver<br>正しいアドレスで Pass 2 成功"]
    end

    style H2 fill:#90ee90
    style I2 fill:#90ee90
    style ERR2 fill:#ff6b6b
```

---

## 2. 案 A: pclntab ヘッダからの textStart 直接読み取り

> **⚠️ Go 1.26.0 ソースコード調査結果（2026-03-13 確認）**
>
> 当初の設計仮説は誤りであった。以下に実際の動作を記録する。

### 2.1 Go 1.26 確認結果: magic 値とヘッダ構造

**magic 値（`$GOROOT/src/internal/abi/symtab.go` より）:**

| 定数 | 値 | 適用バージョン |
|------|-----|--------------|
| `Go118PCLnTabMagic` | `0xfffffff0` | Go 1.18–1.19 |
| `Go120PCLnTabMagic` | `0xfffffff1` | Go 1.20–現在 |
| `CurrentPCLnTabMagic` | `Go120PCLnTabMagic` | Go 1.26 も同値 |

**結論: Go 1.26 で新しい magic 値は追加されていない。**

**pclntab ヘッダレイアウト（`$GOROOT/src/cmd/link/internal/ld/pcln.go` より）:**

Go 1.18 以降すべてのバージョン（1.18–1.26）で、リンカは以下の順でヘッダを書き込む:

| オフセット | フィールド | サイズ | 説明 |
|----------|----------|-------|------|
| 0 | magic | 4 bytes | Go 1.18–1.19: 0xfffffff0, Go 1.20+: 0xfffffff1 |
| 4–5 | pad | 2 bytes | 0, 0 |
| 6 | minLC | 1 byte | quantum（x86: 1, ARM: 4）|
| 7 | ptrSize | 1 byte | 4 or 8 |
| 8 | nfunc | ptrSize | 関数数 |
| 8 + ptrSize | nfiles | ptrSize | ファイル数（Go 1.18+）|
| 8 + 2*ptrSize | **_(unused)_** | ptrSize | **常に 0**（`SetUintptr(0) // unused`）|
| 8 + 3*ptrSize | funcnametab offset | ptrSize | pcHeader からの相対オフセット |
| … | cutab, filetab, pctab, pclntab | ptrSize 各 | 同上 |

**重要: Go 1.18–1.19 のヘッダ `8+ptrSize` は textStart ではなく nfiles である。**
`8+2*ptrSize` は Go 1.18–1.25 ではリロケーション予定フィールドだったが、
Go 1.20 以降は `0` で固定（`// unused`）されている。

### 2.2 `debug/gosym` の textStart 処理（Go 1.26 確認）

`$GOROOT/src/debug/gosym/pclntab.go` の `parsePclnTab` 関数（ver118/ver120 分岐）:

```go
case ver118, ver120:
    t.nfunctab = uint32(offset(0))   // word 0 = nfunc
    t.nfiletab = uint32(offset(1))   // word 1 = nfiles
    t.textStart = t.PC               // ヘッダのワード2(unused)は読まない
    t.funcnametab = data(3)          // word 3 = funcnametab
    ...
```

`t.textStart = t.PC` — `NewLineTable(data, addr)` の第2引数 `addr` が textStart になる。
**ヘッダのバイトからは一切読まない。**

### 2.3 案 A の実際の対応範囲

| Go バージョン | ヘッダ `8+N*ptrSize` の値 | `debug/gosym` の textStart | 案 A の効果 |
|-------------|--------------------------|--------------------------|------------|
| 1.18–1.19 | nfunc/nfiles/reloc(textStart)/… | `t.PC`（引数） | **ヘッダ読み取り不可**（textStart 位置が誤り） |
| 1.20–1.26 | nfunc/nfiles/**0**/funcnametab/… | `t.PC`（引数） | **ヘッダ読み取り不可**（フィールドが 0）|

**結論: 案 A（`readPclntabTextStart`）は Go 1.18–1.26 のいずれのバージョンでも機能しない。**
ヘッダから有効な textStart を読み取る方法は存在せず、案 A はすべてのバージョンで
`return 0, false` となる（textStart が 0 または誤った位置）。

### 2.4 設計への影響

案 A は実質的に**常に失敗するパス**であり、有害ではないが無意味である。

採用方針の変更:
- **案 A の `readPclntabTextStart` は削除する**（または `gosymAlreadyAppliedTextStart` の前提として流用不可）
- **案 B（CALL ターゲット相互参照）を唯一の検出手段とする**
- Go バージョンによる分岐は不要（案 B はヘッダ形式に依存しない）

比較表（4節）および 5節の採用決定も本調査結果を踏まえて更新する。

---

## 3. 案 B: CALL ターゲット相互参照

### 3.1 原理

`.text` セクション内の CALL 即値命令のターゲットアドレスはリンカが計算した
**実際の仮想アドレス**であり、pclntab のずれとは無関係に正確である。
一方、pclntab の関数エントリは `C_startup_size` 分低い値になっている。

複数の CALL/BL 命令ターゲットと pclntab エントリを照合して差分のヒストグラムを作れば、
出現頻度が最も高い差分（= offset）を統計的に検出できる。

```
(CALL ターゲット実際の VA) - (pclntab が返す fn.Entry) = C_startup_size = offset
```

### 3.2 nearest-neighbor の失敗（旧実装の欠陥）

> **⚠️ 2026-03-14 実バイナリ検証により判明。旧アルゴリズムは機能しない。**

旧実装は「CALL ターゲット T に最も近い pclntab エントリ」に差分を取っていた。
この方式は関数間隔が 4 KB 以上のテスト用バイナリではたまたま機能するが、
実際の Go バイナリでは機能しない。

**実測値（Go 1.26.0 / x86_64 / CGO バイナリ、offset = 0x100）:**

| アルゴリズム | 正解 offset=0x100 の得票 | 得票順位 |
|------------|------------------------|---------|
| nearest-neighbor（旧実装） | 380票 | **7位** |
| window exact-match（新実装） | **4733票** | **1位**（2位: 2076票）|

**失敗の原因:**

pclntab エントリの間隔分布（実測）:

| パーセンタイル | 間隔 |
|--------------|------|
| min | 0x20（32 B） |
| p10 | 0x40（64 B） |
| p50 | 0xc0（192 B） |
| p90 | 0x320（800 B） |
| max | 0x12a0（4.7 KB） |

関数間隔の中央値が 0xc0 バイトであるため、CALL ターゲット T の最近傍エントリは
ほぼ常に「呼び先以外の隣接関数」に当たる。その結果、差分が全方向にばらけて
正解 offset が多数票を取れない。テストで 4 KB 間隔を使っていたのはこの欠陥を
意図せず回避していたためである（[pclntab_parser_test.go:380](../../../internal/runner/security/elfanalyzer/pclntab_parser_test.go) のコメント参照）。

### 3.3 新アルゴリズム: Window Exact-Match

**キー洞察:** CALL が Go 関数を呼ぶとき、ターゲット VA は必ず `rawEntry + offset` に
一致する（`rawEntry` は pclntab の補正前エントリ値）。したがって：

```
T - rawEntry = offset   (exact, not approximate)
```

正しい `offset` では全 Go 関数呼び出しが同じ差分を投票するため、
ノイズ（他の差分）に対して圧倒的に優位になる。

**アルゴリズム:**

```
各 CALL ターゲット T について:
    [T - maxOffset, T] の範囲に含まれる全 pclntab エントリ E を列挙（二分探索）
    各 E について: diffCounts[T - E]++

// 正しい offset では:
//   T = E + offset  →  T - E = offset  ←  多数決で1位になる
// ノイズ（T が Go 関数でない / E が呼び先でない）:
//   差分がバラバラ → 票が分散
```

`maxOffset` は C スタートアップコードの最大サイズ上界。実測では 0x100（256 B）だが、
バイナリによって異なるため 0x2000（8 KB）程度の余裕を持たせる。

**旧実装との比較:**

```
// 旧実装（nearest-neighbor）:
nearest := findNearest(sortedEntries, T)   // 前後1件のみ
diffCounts[T - nearest]++

// 新実装（window exact-match）:
lo := T - maxOffset
for each E in sortedEntries where lo <= E <= T:
    diffCounts[T - E]++
```

変更点は `recordDiff` / `findNearest` の置き換えのみ。呼び出し元の構造は変わらない。

### 3.4 長所・短所

| 観点 | 評価 |
|------|------|
| 実装複雑度 | 中（約 70 行）|
| 計算コスト | O(calls × W) — W は [T-maxOffset, T] 内のエントリ数。実測平均 ~数十件。先頭 256 KB サンプリングで実用上問題なし |
| `.symtab` 依存 | なし |
| **Go 1.26+ 対応** | **可能**（pclntab ヘッダ形式に依存しない）|
| 統計的検出 | 正解 offset が2位の2倍以上の票を獲得（実測）。誤検出困難 |
| CALL 命令が少ない場合 | minVotes を下回ると offset = 0（フェイルセーフに倒れる）|
| デコーダ依存 | なし（既存 `MachineCodeDecoder` を使わず個別実装）|

---

## 4. 案の比較

> **⚠️ 更新（2026-03-13 調査結果反映）**: 案 A はすべての Go バージョンで機能しないことが判明。

| 観点 | 案 A（廃止） | 案 B |
|------|------|------|
| Go 1.18–1.25 CGO 対応 | ❌（ヘッダから textStart を読めない） | ✅ |
| Go 1.26+ CGO 対応 | ❌ | ✅ |
| `.symtab` 依存 | なし | なし |
| 実装複雑度 | — | 中 |
| 信頼性 | — | 高（統計的；複数一致で検証）|
| バージョン依存 | — | なし |

---

## 5. 採用決定

### 5.1 調査結果（2026-03-13）

Go 1.26.0 ソースコードの調査（`$GOROOT/src/internal/abi/symtab.go`、
`src/cmd/link/internal/ld/pcln.go`、`src/debug/gosym/pclntab.go`）により、
以下が確認された：

- **magic 値**: Go 1.26 は `CurrentPCLnTabMagic = Go120PCLnTabMagic = 0xfffffff1`。新 magic は追加されていない
- **ヘッダの textStart フィールド**: Go 1.20 以降、`8+2*ptrSize` は `SetUintptr(0) // unused` で **常に 0**
- **`debug/gosym` の動作**: ver118/ver120 ともに `t.textStart = t.PC`（引数の `addr`）を使用。ヘッダのバイトは読まない
- **案 A の結論**: Go 1.18–1.26 のいずれのバージョンでも `readPclntabTextStart` は機能しない

### 5.2 案 B 単独採用

案 A を廃止し、案 B（CALL ターゲット相互参照）を唯一の検出手段として採用する。

採用根拠：

1. **案 B はすべての Go バージョン（1.18–1.26+）に対応** — ヘッダ形式に依存しない
2. **`.symtab` 不要** — strip されたプロダクションバイナリに対応
3. **案 A は全バージョンで機能しないため廃止** — `readPclntabTextStart` は実装しない

```mermaid
flowchart TD
    S[ParsePclntab] --> M{"pclntab magic<br>確認"}
    M -->|"0xfffffff1<br>（Go 1.20–1.26）"| B["detectOffsetByCallTargets<br>CALL ターゲット相互参照"]
    M -->|"その他<br>（magic ≠ 0xfffffff1）"| E["ErrUnsupportedPclntabVersion<br>（明示的エラー）"]
    B --> H["offset = ヒストグラム<br>最頻値"]
    H --> V{isValidOffset}
    V -->|"0 < offset <=<br>.text.FileSize"| R["return offset"]
    V -->|"範囲外または失敗"| Z["return 0<br>（フェイルセーフ）"]

    style R fill:#90ee90
    style Z fill:#ffb347
    style E fill:#ff6b6b
```

### 5.3 サポートバージョン

**magic = `0xfffffff1` のバイナリのみサポート（Go 1.20–1.26 の共通 magic）**

magic ≠ `0xfffffff1` のバイナリは `ErrUnsupportedPclntabVersion` エラーとして `ParsePclntab` から返す。

**公称サポートバージョン: Go 1.26**（実環境テストが Go 1.26 のみのため）

理由:
- magic `0xfffffff0` を持つ Go 1.18–1.19 バイナリは明示的に拒否する
- Go 1.20–1.25 は magic が `0xfffffff1` で同一のため技術的には通過するが、
  テスト環境が Go 1.26 のみのため動作保証外（通過するが公称サポート外）
- 未検証バイナリへの誤った offset 適用は誤動作（誤検出/見逃し）を引き起こす
- 明示的エラーにより呼び出し元が適切に処理できる（フェイルセーフより明確）

> **注**: `checkPclntabVersion` は magic = `0xfffffff1` を通過させるため、
> Go 1.20–1.25 のバイナリも実質的には通過する。内部コメントに
> 「テストが Go 1.26 のみのため Go 1.26 を公称サポートバージョンとする」旨を記載する。

### 5.4 実装しないこと（スコープ外）

- Go 1.19 以前のバイナリへの対応（magic ≠ `0xfffffff1` のため `ErrUnsupportedPclntabVersion` で明示的に拒否。Go 1.20–1.25 は magic が同一で通過するが動作保証外）
- macOS Mach-O バイナリへの対応
- 既存 `MachineCodeDecoder` インターフェース経由の CALL 検出
  （`ParsePclntab` 呼び出し時点では decoder インスタンスがないため、案 B は独自実装）

---

## 6. 実装上の注意事項

### 6.1 `debug/gosym` の textStart 動作（参考）

Go 1.18–1.26 の `debug/gosym` は `t.textStart = t.PC`（`NewLineTable` 第2引数）を使用する。
現行実装では `NewLineTable(pclntabData, textSection.Addr)` として `.text.Addr` を渡しているため、
`fn.Entry` は `.text.Addr` を基点とした値になる（CGO バイナリでは C_startup_size 分ずれている）。

案 B で検出した offset を `fn.Entry + offset` として適用することで正しい VA が得られる。
**案 A は廃止されたため、二重補正のリスクは存在しない。**

### 6.2 案 B の minVotes 調整

`minVotes = 3` は保守的な初期値。実際のバイナリで先頭 256 KB をスキャンすれば
通常数十〜数百の CALL 命令が含まれるため、正しいオフセットの出現回数は 3 を大きく超える。

**実測値（Go 1.26.0 / x86_64 CGO バイナリ）:**
- CALL ターゲット総数（先頭 256 KB）: 5,033 件
- 正解 offset=0x100 の得票: 4,733 票（全体の 94%）
- 2位の得票: 2,076 票（正解の 44%）

非 CGO バイナリでは `diff=0` が最多（4,585 票）だが、`isValidOffset` の `offset > 0` 条件で除外される。
CGO バイナリでは正解 offset が 2 位の 2 倍以上の票差で1位になる。

### 6.3 バリデーション

検出した offset が不正値でないことをチェックする：

```
isValidOffset(offset int64, textFileSize uint64) bool:
    offset > 0 &&                      // 非 CGO (0) と区別
    uint64(offset) <= textFileSize     // バイナリサイズを超えない
```

負の offset（pclntab のアドレスが実際より大きい）は理論的にあり得ないため除外する。

**maxOffset パラメータ（window exact-match）:**

```
maxOffset = 0x2000  // 8 KB — C スタートアップコードの上界
```

- 実測の C スタートアップサイズは 0x100（256 B）
- maxOffset を大きくするほど計算量が増えるが、エントリが密なため影響は小さい
- 小さすぎると正しい offset が検索ウィンドウ外に出る恐れがある
- 8 KB は実用的な CGO バイナリの C スタートアップサイズを十分カバーする
