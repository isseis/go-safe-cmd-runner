# 実装計画書: pclntab オフセット検出の .symtab 非依存化

## 進捗

- [x] Step 1: pclntab magic 値・ヘッダレイアウトを Go ソースで確認（2026-03-13 完了）
- [x] Step 2: 案 A の調査 → **廃止**（ヘッダから textStart を読む手段が存在しないと判明）
- [x] Step 3: Go 1.26 の pclntab 形式を確認し対応方針を確定（2026-03-13 完了）
- [x] Step 4: 案 B — CALL ターゲット相互参照の実装（nearest-neighbor 版、後に欠陥判明）
- [x] Step 5: `detectPclntabOffset` の置き換え（.symtab 参照を削除、案 B 単独）
- [x] Step 6: テスト追加（AC-1〜AC-6）
- [x] Step 7: `make fmt && make test && make lint` 通過確認
- [x] Step 8: nearest-neighbor の欠陥修正 → window exact-match へ置き換え（2026-03-14 完了）
- [x] Step 9: テストの修正（4 KB 間隔の回避策を除去し、実バイナリ相当の密度で検証）（2026-03-14 完了）
- [x] Step 10: `make fmt && make test && make lint` 通過確認（2026-03-14 完了）

---

## Step 1/2/3: 調査結果（2026-03-13 完了）

**確認したソースファイル:**
- `$GOROOT/src/internal/abi/symtab.go` — magic 定数
- `$GOROOT/src/cmd/link/internal/ld/pcln.go` — リンカのヘッダ書き込み
- `$GOROOT/src/debug/gosym/pclntab.go` — `debug/gosym` の解析ロジック

**確認結果:**

| 項目 | 結果 |
|------|------|
| Go 1.26 の magic 値 | `CurrentPCLnTabMagic = Go120PCLnTabMagic = 0xfffffff1`（新 magic なし）|
| ヘッダ `8+2*ptrSize` の内容 | Go 1.20+ では `SetUintptr(0) // unused`（常に 0）|
| `debug/gosym` の textStart | `t.textStart = t.PC`（引数の addr）— ヘッダは読まない |
| 案 A の実現可能性 | **不可能**（全バージョンでヘッダから有効な textStart を読めない）|

**方針変更: 案 A を廃止し、案 B を Go 1.20+ 対応の単独手段として採用。**

---

## Step 4/5/6/7: 初回実装（完了、ただし欠陥あり）

Step 4〜7 の実装内容（nearest-neighbor 版）は完了した。
ただし 2026-03-14 の実バイナリ検証により、アルゴリズムに根本的な欠陥が判明した。
詳細は Step 8 の「欠陥の内容」を参照。

---

## Step 8: nearest-neighbor の欠陥修正（window exact-match へ置き換え）

### 欠陥の内容（2026-03-14 実バイナリ検証により判明）

既存の `recordDiff` / `findNearest` は「CALL ターゲット T の前後最近傍 pclntab エントリ」に
差分を取っていた。実際の Go バイナリでは関数が密に並ぶため（中央値 0xc0 バイト間隔）、
最近傍エントリはほぼ常に「呼び先以外の隣接関数」に当たり、差分が全方向に分散する。

**実測結果（Go 1.26.0 / x86_64 CGO バイナリ、actual offset = 0x100）:**

| アルゴリズム | 正解 offset=0x100 の得票 | 得票順位 |
|------------|------------------------|---------|
| nearest-neighbor（現行） | 380票 | **7位** |
| window exact-match（新） | **4733票** | **1位**（2位: 2076票）|

**テストが欠陥を検出できなかった理由:**

`TestDetectOffsetByCallTargets_WithOffset_x86/arm64` は `entrySpacing = 0x1000`（4 KB）を
使用していた（[pclntab_parser_test.go:380](../../../internal/runner/security/elfanalyzer/pclntab_parser_test.go) のコメント参照）。
これは実バイナリでは不自然な広い間隔であり、nearest-neighbor が偶然正しいエントリに当たる条件を作っていた。

### 変更内容

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser.go`

`recordDiff` と `findNearest` を削除し、新関数 `collectWindowDiffs` に置き換える。

**旧実装（削除）:**

```go
// recordDiff — 最近傍エントリとの差分を1件記録
func recordDiff(target uint64, sortedEntries []uint64, diffCounts map[int64]int) {
    nearest := findNearest(sortedEntries, target)
    if nearest == 0 { return }
    diff := int64(target) - int64(nearest)
    const maxDiff = int64(0x1000)
    if diff > -maxDiff && diff < maxDiff {
        diffCounts[diff]++
    }
}

// findNearest — 前後1件の最近傍を返す
func findNearest(sortedEntries []uint64, target uint64) uint64 { ... }
```

**新実装（追加）:**

```go
// collectWindowDiffs records (target - E) for all pclntab entries E
// in the range [target - maxOffset, target].
//
// Rationale: in a CGO binary, every CALL to a Go function satisfies
//   target = rawEntry + offset  =>  target - rawEntry = offset  (exact)
// so the correct offset accumulates votes from all Go-function calls.
// Noise (calls to non-Go targets, or wrong-entry pairs) produces scattered
// diffs and cannot match the vote count of the true offset.
//
// maxOffset is the upper bound for C startup code size (8 KB is generous).
const maxOffset = int64(0x2000)

func collectWindowDiffs(target uint64, sortedEntries []uint64, diffCounts map[int64]int) {
    lo := uint64(0)
    if int64(target) > maxOffset {
        lo = uint64(int64(target) - maxOffset)
    }
    // Binary search: find first index where sortedEntries[i] >= lo
    idxLo := sort.Search(len(sortedEntries), func(i int) bool {
        return sortedEntries[i] >= lo
    })
    for i := idxLo; i < len(sortedEntries) && sortedEntries[i] <= target; i++ {
        diff := int64(target) - int64(sortedEntries[i]) //nolint:gosec
        diffCounts[diff]++
    }
}
```

**呼び出し元の変更（`collectX86CallDiffs` / `collectArm64BLDiffs`）:**

```go
// 旧:
recordDiff(targetVA, sortedEntries, diffCounts)

// 新:
collectWindowDiffs(targetVA, sortedEntries, diffCounts)
```

**`detectOffsetByCallTargets` のコメント更新（算法の説明を nearest-neighbor から window exact-match に変更）:**

```go
// It scans the first 256 KB of .text for CALL/BL targets, builds a histogram of
// (target - E) for all pclntab entries E within [target - maxOffset, target],
// and returns the most frequent value if it appears at least minVotes times.
// Returns 0 if detection fails.
//
// This window exact-match approach is reliable for real Go binaries where
// functions are typically 0x20–0x320 bytes apart. The nearest-neighbor approach
// (comparing only the single closest entry) fails in dense layouts because the
// closest entry is rarely the callee, scattering votes across many diff values.
```

---

## Step 9: テストの修正

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser_test.go`

### 9.1 既存テストの修正

#### `TestDetectOffsetByCallTargets_WithOffset_x86` / `TestDetectOffsetByCallTargets_WithOffset_arm64`

**現状の問題:**
- `entrySpacing = 0x1000`（4 KB）を使用 — nearest-neighbor の欠陥を回避するための人工的な設定
- コメント（`:380` 行）に「nearest-neighbor が別エントリに当たるため 4 KB 間隔が必要」と明記

**変更内容:**
- `entrySpacing` を実バイナリ相当の密な間隔（`0x60` = 96 バイト）に変更
- CALL 数（`numCalls`）を増やして `maxOffset` 内に複数エントリが収まることを確認
- コメントから nearest-neighbor に関する記述を削除

```go
// 旧:
const entrySpacing = uint64(0x1000)  // nearest-neighbor 回避のための人工的な間隔
const numCalls = 5

// 新:
const entrySpacing = uint64(0x60)   // 実バイナリ相当（中央値 0xc0 より小さく、密度テスト）
const numCalls = 10                  // window 内に複数エントリが入ることを保証
```

**なぜ `entrySpacing = 0x60` で正しく動くか:**

`offsetVal = 0x100`、`entrySpacing = 0x60` の場合、各 CALL ターゲット `T = entry_i + 0x100` に対して
window `[T - 0x2000, T]` には多数のエントリが含まれる。
しかし `T - entry_i = 0x100`（正解）の票が `numCalls` 件集まり、
他の差分（例: `T - entry_{i-1} = 0x100 + 0x60 = 0x160`）は最大 `numCalls - 1` 件しか集まらない。
`numCalls = 10` であれば正解が 10 票で最多になる（minVotes = 3 を超える）。

#### `TestDetectOffsetByCallTargets_NoOffset`

window exact-match では `diff=0`（非 CGO）も正しく多数票になる（各 CALL target が
何らかのエントリと `diff=0` で一致する）。`isValidOffset` の `offset > 0` 条件で除外されるため
テスト自体の変更は不要だが、コメントに window 版での動作を追記する。

#### `TestDetectOffsetByCallTargets_InsufficientVotes`

現状は numCalls = 2（minVotes = 3 未満）でテスト。window 版では各 CALL が複数エントリに
差分を記録するため、別の diff が偶然 minVotes に達しないことを確認する必要がある。
numCalls を 1 に減らすか、あるいはエントリ数も 1 に減らして確実に minVotes 未満にする。

```go
// 旧: numCalls = 2, entrySpacing = 0x1000
// 新: numCalls = 2, entrySpacing = 0x1000 のまま可
// ただし: window 内エントリが多いと diff の種類が増え各1票になる。
// numCalls * (entries in window / entrySpacing * maxOffset) < minVotes を確認。
// maxOffset=0x2000, entrySpacing=0x1000 なら window 内エントリ数 ≈ 2 → 各 diff 最大2票 < 3。
// entrySpacing = 0x1000 はこのテストでは維持する。
```

### 9.2 新規ユニットテスト

#### `TestCollectWindowDiffs_DenseEntries`

window exact-match の核心を直接テスト:

```
条件:
  target = 0x402200
  sortedEntries = [0x400000,  // window 外（target - 0x400000 = 0x2200 > maxOffset）
                   0x402100, 0x402140, 0x402180, 0x4021c0, 0x402200]
                  (window 内 5 件 + window 外 1 件)

期待:
  diffCounts の要素数 = 5（window 外エントリは記録されない）
  diffCounts[0x100] = 1   // target - 0x402100
  diffCounts[0xc0]  = 1   // target - 0x402140
  diffCounts[0x80]  = 1   // target - 0x402180
  diffCounts[0x40]  = 1   // target - 0x4021c0
  diffCounts[0x0]   = 1   // target - 0x402200
  // 0x400000 は target - 0x400000 = 0x2200 > maxOffset なので含まれない
```

#### `TestDetectOffsetByCallTargets_DenseLayout_x86`

実バイナリ相当の密な関数配置でオフセット検出が成功することを確認:

```
条件:
  offsetVal = 0x100
  entrySpacing = 0x40 (64 バイト、実バイナリの p10 値)
  numCalls = 20
  各 CALL ターゲット = entry_i + offsetVal

期待: detectOffsetByCallTargets が 0x100 を返す
```

#### `TestCollectWindowDiffs_MaxOffsetBoundary`（maxOffset 境界値テスト）

`collectWindowDiffs` が `maxOffset` の境界を正しく扱うことを直接検証する。
`maxOffset` の値は実装コード内の定数であり、テストはその値を参照して境界を構成する。
これにより、`maxOffset` の変更が境界テストの失敗として即座に検出される。

```
前提:
  maxOffset は実装内の定数（現在 0x2000）
  textAddr = 0x401000
  pclntab entry E = textAddr（= 0x401000）
  各ケースで CALL ターゲット T を変化させる

ケース A: offset = maxOffset - 1（= 0x1fff）→ ウィンドウ内の最遠点
  T = E + (maxOffset - 1) = 0x402fff
  期待: diffCounts[maxOffset-1] = 1（E が window に含まれる）

ケース B: offset = maxOffset（= 0x2000）→ ウィンドウ境界上（T - E = maxOffset）
  T = E + maxOffset = 0x403000
  期待: diffCounts[maxOffset] = 1
  （条件は `lo <= E <= T`、lo = T - maxOffset = E なので E がちょうど境界に含まれる）

ケース C: offset = maxOffset + 1（= 0x2001）→ ウィンドウ外
  T = E + maxOffset + 1 = 0x403001
  期待: diffCounts に E との差分が記録されない（E < lo = T - maxOffset）
```

**なぜケース B の期待が「含まれる」か:**

`lo = T - maxOffset`。ケース B では `T - E = maxOffset` なので `E = T - maxOffset = lo`。
条件 `lo <= E` が成立するため E はウィンドウに含まれる。
off-by-one（`<` vs `<=`）の誤りがあれば、ケース B が失敗として検出される。

#### `TestDetectOffsetByCallTargets_OffsetAtMaxBoundary_x86`（end-to-end 境界値テスト）

`collectWindowDiffs` の境界テストに加え、`detectOffsetByCallTargets` 全体として
`maxOffset` 近傍の offset が正しく検出・除外されることを確認する。

```
ケース 1: offsetVal = maxOffset - 1（成功期待）
  pclntab entries: E_0, E_1, ..., E_N (spacing = 0x40)
  各 CALL ターゲット = E_i + (maxOffset - 1)
  期待: detectOffsetByCallTargets が (maxOffset - 1) を返す

ケース 2: offsetVal = maxOffset（成功期待）
  各 CALL ターゲット = E_i + maxOffset
  期待: detectOffsetByCallTargets が maxOffset を返す
  （ただし isValidOffset(maxOffset, textFileSize) が true の場合のみ）

ケース 3: offsetVal = maxOffset + 1（失敗期待 = return 0）
  各 CALL ターゲット = E_i + maxOffset + 1
  期待: detectOffsetByCallTargets が 0 を返す
  （E_i が window [T - maxOffset, T] に入らないため票が集まらない）
```

**注:** ケース 2 の `isValidOffset` チェックは `.text.FileSize` に依存する。
テスト用 ELF を構築する際、`.text.FileSize >= maxOffset` になるようにデータを用意する。
既存の `buildELF64WithText` ヘルパーを拡張して `.text.Size` を指定可能にする、
または ELF を直接構築する既存パターン（`buildELF64WithPclntab`）を踏襲する。

### 9.3 実バイナリ回帰テスト（integration タグ）

**追加背景:** 今回の欠陥（nearest-neighbor）は合成テストでは検出できず、
実バイナリ検証で初めて判明した。アルゴリズムを変更した後も同じ盲点が生まれないよう、
実際の CGO バイナリを使った `ParsePclntab` の回帰テストを統合テストとして維持する。

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser_integration_test.go`（新規）

**ビルドタグ:** `//go:build integration`

**スキップ条件:** `syscall_analyzer_integration_test.go` の既存テストと同じ条件を踏襲する:
- `runtime.GOARCH != "amd64"` → `t.Skip`
- `gcc` が存在しない → `t.Skip`（CGO バイナリの場合）
- `strip` が存在しない → 該当テストのみ `t.Skip`

#### `TestParsePclntab_RealCGOBinary_NotStripped`（AC-1 / AC-6 回帰）

```
手順:
  1. CGO バイナリをオンザフライでビルド（CGO_ENABLED=1, GOARCH=amd64）
  2. ParsePclntab を呼び出す
  3. .symtab から runtime.text の VA を取得して実際の offset を算出

期待:
  - ParsePclntab がエラーなく成功する
  - ParsePclntab が返した offset（= 補正前後の Entry 差）が symtab の offset と一致する
  - offset > 0（C スタートアップコードが存在する）
```

#### `TestParsePclntab_RealCGOBinary_Stripped`（AC-2 回帰）

```
手順:
  1. 上記と同じ CGO バイナリを strip コマンドで処理
  2. ParsePclntab を呼び出す（.symtab なし）

期待:
  - ParsePclntab が not-stripped 版と同じ offset を返す
  - .symtab の有無でオフセット検出結果が変わらない

検証方法:
  not-stripped で検出した offset を変数に保持し、stripped でも同値であることを assert する
```

**ソースコード（2 テスト共通）:**

```go
const cgoParseSrc = `package main

// #include <stdio.h>
import "C"

import "fmt"

func main() {
    C.puts(C.CString("hello"))
    fmt.Println("hello from Go")
}
`
```

`import "C"` さえあれば内容は問わない。
ネットワーク呼び出しは不要（`ParsePclntab` 自体のテストのため）。

**offset の検証方法:**

```go
// not-stripped バイナリから symtab で真の offset を取得
elfFile, _ := elf.Open(binFile)
syms, _ := elfFile.Symbols()
var runtimeTextVA uint64
for _, s := range syms {
    if s.Name == "runtime.text" { runtimeTextVA = s.Value; break }
}
textSec := elfFile.Section(".text")
expectedOffset := int64(runtimeTextVA) - int64(textSec.Addr)

// ParsePclntab の補正結果を verified offset と照合
funcs, err := ParsePclntab(elfFile)
require.NoError(t, err)
// ParsePclntab はすでに補正済みエントリを返すため、
// 補正量は（補正後の最初のエントリ - 補正前）= expectedOffset と等しいはず。
// 直接 detectOffsetByCallTargets を呼んで offset 値自体を検証する。
detectedOffset := detectOffsetByCallTargets(elfFile, rawFuncs) // 補正前の funcs を渡す
assert.Equal(t, expectedOffset, detectedOffset)
assert.Greater(t, detectedOffset, int64(0))
```

---

## Step 10: 確認コマンド

```bash
# フォーマット
make fmt

# 全ユニットテスト
make test

# リンター
make lint

# pclntab パーサのユニットテスト（変更した関数）
go test -tags test -v \
  -run 'TestDetectOffsetByCallTargets|TestCollectWindowDiffs' \
  ./internal/runner/security/elfanalyzer/

# 実バイナリ回帰テスト（Step 9.3）— アルゴリズム変更後に必ず実行
go test -tags "test integration" -v \
  -run 'TestParsePclntab_RealCGOBinary' \
  ./internal/runner/security/elfanalyzer/
```

---

## 実装上の依存関係

```
Step 1/2/3（調査完了: 2026-03-13）
    ↓
Step 4（nearest-neighbor 版実装: 完了）
    ↓
Step 5（detectPclntabOffset 置き換え: 完了）
    ↓
Step 6（テスト追加: 完了、ただし 4 KB 間隔の回避策あり）
    ↓
Step 7（make fmt / test / lint 通過: 完了）
    ↓
Step 8（window exact-match への置き換え）← 現在ここ
    ↓
Step 9（テスト修正: entrySpacing を密に変更、新規テスト追加）
    ↓
Step 10（make fmt / test / lint 通過確認）
```
