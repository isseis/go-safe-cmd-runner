# 実装計画書: pclntab オフセット検出の .symtab 非依存化

## 進捗

- [ ] Step 1: pclntab magic 値・ヘッダレイアウトを Go ソースで確認
- [ ] Step 2: 案 A — pclntab ヘッダ解析関数の実装（Go 1.18–1.25）
- [ ] Step 3: Go 1.26 の pclntab 形式を確認し対応方針を確定
- [ ] Step 4: 案 B — CALL ターゲット相互参照の実装（Go 1.26+ フォールバック）
- [ ] Step 5: `detectPclntabOffset` の置き換え（.symtab 参照を削除）
- [ ] Step 6: テスト追加（AC-1〜AC-6）
- [ ] Step 7: `make fmt && make test && make lint` 通過確認

---

## Step 1: pclntab magic 値の確認（調査）

**目的:** 実装に使うヘッダ定数を正確に確認する。

確認先:
```bash
# Go 標準ライブラリの pclntab 定義を確認
cat $GOROOT/src/debug/gosym/pclntab.go | grep -E 'magic|go11[6-9]|go12[0-9]'

# Go 1.26 での変更内容を確認（magic の変化有無）
go version
```

確認事項:
- `go118magic` (`0xfffffff0`) の textStart オフセット位置
- `go120magic` (`0xfffffff1`) の textStart オフセット位置
- Go 1.26 での magic 定数と textStart 削除の確認

---

## Step 2: 案 A の実装（Go 1.18–1.25 対応）

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser.go`

**追加する関数:** `readPclntabTextStart`

```go
// readPclntabTextStart extracts the textStart field from the pclntab header
// for Go 1.18–1.25 binaries. Returns (textStart, true) if the header format
// is recognized and the field can be read; otherwise returns (0, false).
//
// Go 1.18–1.19 (magic 0xfffffff0): textStart is at offset 8+ptrSize
// Go 1.20–1.25 (magic 0xfffffff1): textStart is at offset 8+2*ptrSize
func readPclntabTextStart(data []byte, byteOrder binary.ByteOrder) (uint64, bool) {
    const minHeaderSize = 8
    if len(data) < minHeaderSize {
        return 0, false
    }

    magic := byteOrder.Uint32(data[0:4])
    ptrSize := int(data[7])
    if ptrSize != 4 && ptrSize != 8 {
        return 0, false
    }

    var textStartOffset int
    switch magic {
    case go118magic: // 0xfffffff0
        textStartOffset = 8 + ptrSize
    case go120magic: // 0xfffffff1
        textStartOffset = 8 + 2*ptrSize
    default:
        return 0, false
    }

    if len(data) < textStartOffset+ptrSize {
        return 0, false
    }

    var textStart uint64
    if ptrSize == 8 {
        textStart = byteOrder.Uint64(data[textStartOffset:])
    } else {
        textStart = uint64(byteOrder.Uint32(data[textStartOffset:]))
    }
    if textStart == 0 {
        return 0, false
    }
    return textStart, true
}
```

**magic 定数の追加（または既存定数を参照）:**
```go
// These constants match the Go standard library's debug/gosym package.
const (
    go118magic = 0xfffffff0
    go120magic = 0xfffffff1
)
```

---

## Step 3: Go 1.26 の pclntab 形式確認（調査）

**目的:** Go 1.26+ での pclntab ヘッダ形式（新 magic 値の有無）を確認し、
案 B フォールバックの実装範囲を確定する。

確認方法:
```bash
# 現在の GOROOT のバージョンを確認
go version

# pclntab ヘッダ定数を確認（go1.26 での変更状況）
cat $GOROOT/src/debug/gosym/pclntab.go

# テスト用 CGO バイナリの pclntab 先頭を hexdump
readelf -S /tmp/cgo_test | grep gopclntab
dd if=/tmp/cgo_test bs=1 skip=<offset> count=32 | hexdump -C
```

確認事項:
- Go 1.26 に新しい magic 値があるか（あれば Step 2 に追加検討）
- textStart が完全に削除されているかどうか

---

## Step 4: 案 B の実装（Go 1.26+ フォールバック）

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser.go`

**追加する関数:** `detectOffsetByCallTargets`

```go
// detectOffsetByCallTargets detects the pclntab address offset in CGO binaries
// by cross-referencing CALL/BL instruction targets with pclntab function entries.
// This method works independently of the pclntab header format (including Go 1.26+).
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

**実装ポイント:**
- `MachineCodeDecoder` を使わず、x86_64 と arm64 の CALL/BL のみを独自デコード
- `elfFile.Machine` から アーキテクチャを判定
- 差分のヒストグラムは `map[int64]int` で管理

---

## Step 5: `detectPclntabOffset` の置き換え

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser.go`

現行の `.symtab` ベースの実装を削除し、案 A + 案 B のハイブリッドに置き換える。

**シグネチャ変更:** `pclntabData []byte` 引数を追加（案 A で使用）。
`ParsePclntab` 側で既に持っている `pclntabData` を渡す。

```go
// 変更前:
func detectPclntabOffset(elfFile *elf.File, pclntabFuncs map[string]PclntabFunc) int64

// 変更後:
func detectPclntabOffset(
    elfFile *elf.File,
    pclntabData []byte,
    pclntabFuncs map[string]PclntabFunc,
) int64
```

**本体:**
```go
func detectPclntabOffset(
    elfFile *elf.File,
    pclntabData []byte,
    pclntabFuncs map[string]PclntabFunc,
) int64 {
    textSection := elfFile.Section(".text")
    if textSection == nil {
        return 0
    }

    // Option A: read textStart directly from pclntab header (Go 1.18-1.25).
    if headerTextStart, ok := readPclntabTextStart(pclntabData, elfFile.ByteOrder); ok {
        if headerTextStart > textSection.Addr {
            offset := int64(headerTextStart) - int64(textSection.Addr) //nolint:gosec
            if uint64(offset) <= textSection.FileSize {
                return offset
            }
        }
    }

    // Option B: CALL/BL target cross-reference (Go 1.26+ fallback).
    return detectOffsetByCallTargets(elfFile, pclntabFuncs)
}
```

`ParsePclntab` 内の呼び出し箇所も更新:
```go
// 変更前:
if offset := detectPclntabOffset(elfFile, functions); offset != 0 {

// 変更後:
if offset := detectPclntabOffset(elfFile, pclntabData, functions); offset != 0 {
```

---

## Step 6: テスト追加

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser_test.go`

### ユニットテスト（`readPclntabTextStart`）

| テスト名 | 検証内容 |
|---------|---------|
| `TestReadPclntabTextStart_Go118` | go118magic のヘッダから textStart を正しく読む（ptrSize=8）|
| `TestReadPclntabTextStart_Go120` | go120magic のヘッダから textStart を正しく読む（ptrSize=8）|
| `TestReadPclntabTextStart_Go118_32bit` | go118magic + ptrSize=4 で正しく読む |
| `TestReadPclntabTextStart_UnknownMagic` | 未知 magic → (0, false) |
| `TestReadPclntabTextStart_TooShort` | データが短すぎる → (0, false) |
| `TestReadPclntabTextStart_InvalidPtrSize` | ptrSize が 4 でも 8 でもない → (0, false) |
| `TestReadPclntabTextStart_ZeroTextStart` | textStart = 0 → (0, false) |

### ユニットテスト（`detectOffsetByCallTargets`）

| テスト名 | 検証内容 |
|---------|---------|
| `TestDetectOffsetByCallTargets_WithOffset_x86` | x86_64 CALL 命令で 0x100 のずれを検出 |
| `TestDetectOffsetByCallTargets_WithOffset_arm64` | arm64 BL 命令で 0x100 のずれを検出 |
| `TestDetectOffsetByCallTargets_NoOffset` | ずれなし → 0 |
| `TestDetectOffsetByCallTargets_InsufficientVotes` | 一致 CALL が minVotes 未満 → 0 |
| `TestDetectOffsetByCallTargets_NoText` | .text セクションなし → 0 |

### 受け入れ基準テスト（`build` タグ: `integration`）

| AC | テスト名 | 実施方法 |
|----|---------|---------|
| AC-1 | `TestDetectPclntabOffset_NotStrippedCGO` | not-stripped CGO バイナリに offset = 0x100 |
| AC-2 | `TestDetectPclntabOffset_StrippedCGO` | `strip` 済み CGO バイナリに offset = C_startup_size |
| AC-3 | `TestDetectPclntabOffset_NonCGO` | 純粋 Go バイナリに offset = 0 |
| AC-4 | `TestDetectPclntabOffset_InvalidPclntab` | 壊れた pclntab → offset = 0 |
| AC-5 | `TestDetectPclntabOffset_Go1_18_25` | Go 1.18–1.25 ビルドバイナリ（CI 環境）|
| AC-6 | `TestDetectPclntabOffset_Go1_26` | Go 1.26 ビルドバイナリ（現テスト環境）|

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
  -run 'TestReadPclntabTextStart|TestDetectOffsetByCallTargets|TestDetectPclntabOffset' \
  ./internal/runner/security/elfanalyzer/

# CGO バイナリ統合テスト（Go 1.26 環境）
go test -tags "test integration" -v \
  -run 'TestDetectPclntabOffset|TestAC1_CgoBinaryNetworkDetection' \
  ./internal/runner/security/elfanalyzer/
```

---

## 実装上の依存関係

```
Step 1（調査: magic 値確認）
    ↓
Step 2（案 A 実装）     Step 3（調査: Go 1.26 確認）
    ↓                       ↓
    └─────────┬─────────────┘
              ↓
         Step 4（案 B 実装）
              ↓
         Step 5（detectPclntabOffset 置き換え）
              ↓
         Step 6（テスト追加）
              ↓
         Step 7（make fmt / test / lint）
```

Step 1 と Step 3 は実装開始前の調査ステップ。
Step 2 と Step 4 は独立して実装可能（Step 5 で統合）。
