# 実装計画書: pclntab オフセット検出の .symtab 非依存化

## 進捗

- [x] Step 1: pclntab magic 値・ヘッダレイアウトを Go ソースで確認（2026-03-13 完了）
- [x] Step 2: 案 A の調査 → **廃止**（ヘッダから textStart を読む手段が存在しないと判明）
- [x] Step 3: Go 1.26 の pclntab 形式を確認し対応方針を確定（2026-03-13 完了）
- [ ] Step 4: 案 B — CALL ターゲット相互参照の実装（全バージョン対応）
- [ ] Step 5: `detectPclntabOffset` の置き換え（.symtab 参照を削除、案 B 単独）
- [ ] Step 6: テスト追加（AC-1〜AC-6）
- [ ] Step 7: `make fmt && make test && make lint` 通過確認

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

**方針変更: 案 A を廃止し、案 B を全バージョン対応の単独手段として採用。**

---

## Step 4: 案 B の実装（全バージョン対応）

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser.go`

**追加する関数:** `detectOffsetByCallTargets`

```go
// detectOffsetByCallTargets detects the pclntab address offset in CGO binaries
// by cross-referencing CALL/BL instruction targets with pclntab function entries.
// This method works independently of the pclntab header format and Go version
// (Go 1.18–1.26+). It is the sole offset detection mechanism.
//
// It scans the first 256 KB of .text for CALL/BL targets, builds a histogram of
// (target - nearestPclntabEntry) differences, and returns the most frequent value
// if it appears at least minVotes times. Returns 0 if detection fails.
func detectOffsetByCallTargets(
    elfFile *elf.File,
    pclntabFuncs map[string]PclntabFunc,
) int64 {
    const (
        scanLimit = 256 * 1024 // scan first 256 KB of .text
        minVotes  = 3
    )
    // ... Implementation (see algorithm in architecture doc section 3.2)
}
```

**import への追加（Step 4 時点で行う）:**
```go
import (
    "debug/elf"
    "debug/gosym"
    "encoding/binary"  // detectOffsetByCallTargets, checkPclntabVersion で使用
    "errors"
    "fmt"
    "sort"             // detectOffsetByCallTargets の二分探索で使用
)
```

**実装ポイント:**
- `MachineCodeDecoder` を使わず、x86_64 と arm64 の CALL/BL のみを独自デコード
- `elfFile.Machine` からアーキテクチャを判定（`elf.EM_X86_64` / `elf.EM_AARCH64`）
- 差分のヒストグラムは `map[int64]int` で管理
- `.text` セクションのデータは `textSection.Data()` で取得（最大 `scanLimit` バイト）
- pclntab エントリのアドレスをソート済みスライスに入れ、`sort.Search` で最近傍探索

**x86_64 CALL デコード:**
```go
// opcode 0xE8: CALL rel32
// callSite はバイト配列内のインデックス（.text.Addr からの相対）
if data[i] == 0xe8 && i+5 <= len(data) {
    rel := int32(binary.LittleEndian.Uint32(data[i+1 : i+5]))
    target := textSection.Addr + uint64(i) + 5 + uint64(int64(rel))
    // ...
    i += 5
    continue
}
i++
```

**arm64 BL デコード:**
```go
// BL 命令: bits[31:26] == 0b100101
instr := binary.LittleEndian.Uint32(data[i : i+4])
if instr>>26 == 0b100101 {
    imm26 := int32(instr&0x03ffffff) << 6 >> 6 // sign-extend 26-bit
    target := textSection.Addr + uint64(i) + uint64(int64(imm26)*4)
    // ...
}
i += 4
```

---

## Step 5: `detectPclntabOffset` の置き換えと `ParsePclntab` のバージョンチェック

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser.go`

現行の `.symtab` ベースの実装を削除し、案 B 単独に置き換える。
バージョンチェックを `ParsePclntab` に追加し、magic ≠ `0xfffffff1`（Go 1.18–1.19 以前）は明示的エラーとする。

**新規エラー定数を追加（既存の3つはそのまま残す）:**

現行コード（変更なし）:
```go
var (
    ErrNoPclntab          = errors.New("no .gopclntab section found")
    ErrUnsupportedPclntab = errors.New("unsupported pclntab format")
    ErrInvalidPclntab     = errors.New("invalid pclntab structure")
)
```

追加する定数:
```go
    ErrUnsupportedPclntabVersion = errors.New("unsupported pclntab version: only magic 0xfffffff1 (Go 1.20+) is supported")
```

**（import の追加は Step 4 で実施済みのため、Step 5 では不要）**

**`ParsePclntab` にバージョンチェックを追加:**
```go
// checkPclntabVersion verifies that the pclntab magic is supported.
// Only magic = 0xfffffff1 (Go 1.20–1.26, CurrentPCLnTabMagic) is supported.
// Other magic values (e.g. 0xfffffff0 for Go 1.18–1.19) return
// ErrUnsupportedPclntabVersion to prevent incorrect offset application.
//
// Note: Go 1.20–1.25 share the same magic (0xfffffff1) and will pass this
// check. The officially supported version is Go 1.26 (tested), but Go
// 1.20–1.25 binaries may also work in practice.
func checkPclntabVersion(data []byte, byteOrder binary.ByteOrder) error {
    if len(data) < 4 {
        return ErrInvalidPclntab
    }
    magic := byteOrder.Uint32(data[0:4])
    const go120magic = 0xfffffff1 // Go 1.20–1.26 (CurrentPCLnTabMagic)
    if magic != go120magic {
        return fmt.Errorf("%w (got magic 0x%x)", ErrUnsupportedPclntabVersion, magic)
    }
    return nil
}
```

`ParsePclntab` に呼び出しを追加（`gosym.NewTable` の前）:
```go
if err := checkPclntabVersion(pclntabData, elfFile.ByteOrder); err != nil {
    return nil, err
}
```

**`ParsePclntab` のコメントを更新:**

現行コメント（変更前）:
```
// For CGO binaries, the .text section contains C runtime startup code before
// the Go runtime functions. This causes pclntab addresses to be offset from
// the actual virtual addresses. ParsePclntab detects and corrects this offset
// by comparing pclntab entries against .symtab entries when available.
```

変更後:
```
// For CGO binaries, the .text section contains C runtime startup code before
// the Go runtime functions. This causes pclntab addresses to be offset from
// the actual virtual addresses. ParsePclntab detects and corrects this offset
// using CALL/BL instruction cross-referencing (no .symtab required).
//
// Only pclntab with magic 0xfffffff1 (Go 1.20+, officially supported: Go 1.26)
// is supported. Other versions return ErrUnsupportedPclntabVersion.
```

**`detectPclntabOffset` のシグネチャは変更なし。本体を置き換え:**
```go
func detectPclntabOffset(elfFile *elf.File, pclntabFuncs map[string]PclntabFunc) int64 {
    textSection := elfFile.Section(".text")
    if textSection == nil {
        return 0
    }

    // CALL/BL target cross-reference (Go 1.26+).
    // Only reached after checkPclntabVersion confirms a supported binary.
    // CGO binaries always have a positive offset (C startup code precedes Go
    // text), so negative or zero results indicate detection failure.
    offset := detectOffsetByCallTargets(elfFile, pclntabFuncs)
    if !isValidOffset(offset, textSection.FileSize) {
        return 0
    }
    return offset
}

// isValidOffset checks that offset is a plausible CGO text-start correction.
// A valid offset is strictly positive (distinguishes CGO from non-CGO where offset=0)
// and does not exceed the .text section size.
// Negative offsets are theoretically impossible for CGO binaries (C startup code
// always precedes Go text) and must be rejected to prevent address corruption.
func isValidOffset(offset int64, textFileSize uint64) bool {
    return offset > 0 && uint64(offset) <= textFileSize //nolint:gosec
}
```

---

## Step 6: テスト追加

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser_test.go`

### 既存テストの変更・削除

Step 5 で `detectPclntabOffset` の実装が変わるため、以下の既存テストを更新する。

| 既存テスト名 | 対応 | 理由 |
|------------|------|------|
| `TestDetectPclntabOffset_NoSymtab` | **削除** | `.symtab` の有無は新実装で無関係になる。新実装のフォールバック（CALL が minVotes 未満）は `TestDetectOffsetByCallTargets_InsufficientVotes` でカバー |
| `TestDetectPclntabOffset_NoMatch` | **削除** | 同上。`.symtab` 名前一致に依存したロジックが消える |
| `TestDetectPclntabOffset_NonCGO` | **変更** | 非 CGO バイナリで offset = 0 を確認する目的は残す。ただし `ParsePclntab` が `ErrUnsupportedPclntabVersion` を返す場合は `require.NoError` の前提が崩れるため、テスト対象バイナリが magic = `0xfffffff1` であることを確認して使用 |
| `TestParsePclntab_InvalidData` | **変更** | `checkPclntabVersion` 追加後、magic が `0xfffffff1` でないケース（"invalid magic bytes", "random garbage"）は `ErrUnsupportedPclntabVersion` を返すようになる。各サブケースの期待値を調整する |
| `TestParsePclntab_ErrorWrapping` | **変更** | `ErrUnsupportedPclntabVersion` の `Error()` 文字列検証を追加 |

#### `TestParsePclntab_InvalidData` の変更内容

新実装では `checkPclntabVersion` が `gosym.NewTable` より先に呼ばれるため、
magic が `0xfffffff1` でないデータは `ErrUnsupportedPclntabVersion` を返す。
magic が短すぎる（4 バイト未満）データは `ErrInvalidPclntab` を返す。

| ケース名 | 旧期待値 | 新期待値 |
|---------|---------|---------|
| `"empty pclntab"` | `NoError, empty result` | `errors.Is(err, ErrInvalidPclntab)` |
| `"too short for header"` | `NoError, empty result` | `errors.Is(err, ErrInvalidPclntab)` |
| `"invalid magic bytes"` | `NoError, empty result` | `errors.Is(err, ErrUnsupportedPclntabVersion)` |
| `"random garbage"` | `NoError, empty result` | `errors.Is(err, ErrUnsupportedPclntabVersion)` |

#### `TestParsePclntab_ErrorWrapping` の変更内容

追加する検証:
```go
assert.Equal(t, "unsupported pclntab version: only magic 0xfffffff1 (Go 1.20+) is supported",
    ErrUnsupportedPclntabVersion.Error())
```

### ユニットテスト（`checkPclntabVersion`）

| テスト名 | 検証内容 |
|---------|---------|
| `TestCheckPclntabVersion_Go120magic` | magic = 0xfffffff1 → nil（サポート対象）|
| `TestCheckPclntabVersion_Go118magic` | magic = 0xfffffff0 → `ErrUnsupportedPclntabVersion` |
| `TestCheckPclntabVersion_Go116magic` | magic = 0xfffffffa → `ErrUnsupportedPclntabVersion` |
| `TestCheckPclntabVersion_Go12magic`  | magic = 0xfffffffb → `ErrUnsupportedPclntabVersion` |
| `TestCheckPclntabVersion_TooShort`   | データが 4 バイト未満 → `ErrInvalidPclntab` |
| `TestCheckPclntabVersion_BigEndian`  | ビッグエンディアン + magic = 0xf1ffffff → nil |

### ユニットテスト（`detectOffsetByCallTargets`）

| テスト名 | 検証内容 |
|---------|---------|
| `TestDetectOffsetByCallTargets_WithOffset_x86` | x86_64 CALL 命令で 0x100 のずれを検出 |
| `TestDetectOffsetByCallTargets_WithOffset_arm64` | arm64 BL 命令で 0x100 のずれを検出 |
| `TestDetectOffsetByCallTargets_NoOffset` | ずれなし → 0 |
| `TestDetectOffsetByCallTargets_InsufficientVotes` | 一致 CALL が minVotes 未満 → 0 |
| `TestDetectOffsetByCallTargets_NoText` | .text セクションなし → 0 |

### 受け入れ基準テスト（`build` タグ: `integration`）

#### バイナリ調達方針

既存の統合テスト（`syscall_analyzer_integration_test.go`）と同じく、
**テスト内でオンザフライにビルド**する方針を採用する。
`SafeTempDir(t)` でテンポラリディレクトリを作成し、テスト終了時に自動削除される。

| AC | バイナリ種別 | 調達方法 | 実行条件 |
|----|-----------|---------|---------|
| AC-1 | not-stripped CGO バイナリ（x86_64） | `go build`（`CGO_ENABLED=1`、`GOARCH=amd64`） | `runtime.GOARCH == "amd64"` かつ `gcc` が存在する |
| AC-2 | stripped CGO バイナリ（x86_64） | AC-1 と同バイナリを `strip` コマンドで処理 | 同上 かつ `strip` コマンドが存在する |
| AC-3 | 非 CGO バイナリ（x86_64） | `go build`（`CGO_ENABLED=0`、`GOARCH=amd64`） | `runtime.GOARCH == "amd64"` |
| AC-4 | — | インメモリで壊れた pclntab を構築（既存 `buildELF64WithPclntab` を流用） | なし（ユニットテストに統合可） |
| AC-5 | — | インメモリで magic = `0xfffffff0` の pclntab を構築（`buildELF64WithPclntab` を流用） | なし（ユニットテストに統合可） |
| AC-6 | CGO バイナリ（x86_64） | AC-1 と同バイナリを再利用 | `runtime.GOARCH == "amd64"` かつ `gcc` が存在する |

#### CGO バイナリのソースコード（AC-1/AC-2/AC-6 共通）

```go
const cgoBinarySrc = `package main

// #include <stdio.h>
import "C"

import "net"

func main() {
    C.puts(C.CString("hello from C"))
    conn, _ := net.Dial("tcp", "127.0.0.1:1")
    if conn != nil { conn.Close() }
}
`
```

**注:** CGO バイナリであればソースの内容は問わないが、`import "C"` が必要（これにより C スタートアップコードが `.text` 先頭に挿入される）。ネットワーク呼び出しは pclntab テストには不要だが、既存テストとの一貫性のため残す。

#### strip コマンドの確認（AC-2）

```go
if _, err := exec.LookPath("strip"); err != nil {
    t.Skip("strip command not available")
}
// AC-1 と同じバイナリを一時ディレクトリにコピーして strip を適用
strippedBin := filepath.Join(tmpDir, "cgo_stripped")
require.NoError(t, copyFile(binFile, strippedBin))
cmd := exec.Command("strip", strippedBin)
require.NoError(t, cmd.Run())
```

#### 各テストが検証すること

| AC | テスト名 | 検証内容 |
|----|---------|---------|
| AC-1 | `TestParsePclntab_NotStrippedCGO` | `ParsePclntab` が成功し、`syscall.RawSyscall` 等のアドレスが `.symtab` と一致（offset 補正が正しい）|
| AC-2 | `TestParsePclntab_StrippedCGO` | `.symtab` なしでも AC-1 と同じ offset が検出される |
| AC-3 | `TestParsePclntab_NonCGO` | `ParsePclntab` が成功し、offset 補正なし（CALL 相互参照で最頻値が `minVotes` に達しない）|
| AC-4 | `TestParsePclntab_InvalidPclntab` | `buildELF64WithPclntab` で不正データ → `ErrInvalidPclntab` |
| AC-5 | `TestParsePclntab_UnsupportedVersion` | `buildELF64WithPclntab` で magic = `0xfffffff0` → `ErrUnsupportedPclntabVersion` |
| AC-6 | `TestParsePclntab_Go126CGO` | `detectOffsetByCallTargets` が返す offset = `C_startup_size`（`> 0`）を直接検証 |

**注（AC-1 のアドレス一致検証）:** `.symtab` の `syscall.RawSyscall` エントリと補正後の pclntab エントリのアドレスを比較する。厳密な一致が理想だが、関数の終端アドレス精度の違いがある場合は `|diff| < threshold`（例: 16 バイト）で許容する。AC-6（offset > 0 の確認）と組み合わせることで十分な検証精度が得られる。

---

## Step 7: 確認コマンド

```bash
# フォーマット
make fmt

# 全テスト
make test

# リンター
make lint

# pclntab パーサのユニットテスト（追加した関数）
go test -tags test -v \
  -run 'TestDetectOffsetByCallTargets|TestDetectPclntabOffset' \
  ./internal/runner/security/elfanalyzer/

# CGO バイナリ統合テスト（Go 1.26 環境）
go test -tags "test integration" -v \
  -run 'TestDetectPclntabOffset|TestAC1_CgoBinaryNetworkDetection' \
  ./internal/runner/security/elfanalyzer/
```

---

## 実装上の依存関係

```
Step 1/2/3（調査完了: 2026-03-13）
    ↓
Step 4（案 B 実装: detectOffsetByCallTargets）
    ↓
Step 5（detectPclntabOffset 置き換え）
    ↓
Step 6（テスト追加）
    ↓
Step 7（make fmt / test / lint）
```

Step 1/2/3 は調査済み。Step 4 から実装を開始する。
