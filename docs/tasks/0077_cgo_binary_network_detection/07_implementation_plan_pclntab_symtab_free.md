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

**実装ポイント:**
- `MachineCodeDecoder` を使わず、x86_64 と arm64 の CALL/BL のみを独自デコード
- `elfFile.Machine` から アーキテクチャを判定
- 差分のヒストグラムは `map[int64]int` で管理

---

## Step 5: `detectPclntabOffset` の置き換えと `ParsePclntab` のバージョンチェック

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser.go`

現行の `.symtab` ベースの実装を削除し、案 B 単独に置き換える。
バージョンチェックを `ParsePclntab` に追加し、Go 1.26 未満は明示的エラーとする。

**新規エラー定数を追加:**
```go
var (
    ErrNoPclntab                 = errors.New("no .gopclntab section found")
    ErrUnsupportedPclntab        = errors.New("unsupported pclntab format")
    ErrInvalidPclntab            = errors.New("invalid pclntab structure")
    ErrUnsupportedPclntabVersion = errors.New("unsupported pclntab version: only Go 1.26+ (magic 0xfffffff1) is supported")
)
```

**`ParsePclntab` にバージョンチェックを追加:**
```go
// checkPclntabVersion verifies that the pclntab was compiled with a supported
// Go version. Only Go 1.26+ (CurrentPCLnTabMagic = 0xfffffff1) is supported.
// Earlier versions return ErrUnsupportedPclntabVersion to prevent incorrect
// offset application on untested binary formats.
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

| AC | テスト名 | 実施方法 |
|----|---------|---------|
| AC-1 | `TestParsePclntab_NotStrippedCGO` | not-stripped Go 1.26+ CGO バイナリ → offset = C_startup_size、pclntab アドレス補正確認 |
| AC-2 | `TestParsePclntab_StrippedCGO` | strip 済み Go 1.26+ CGO バイナリ → offset = C_startup_size（.symtab なしで検出）|
| AC-3 | `TestParsePclntab_NonCGO` | Go 1.26+ 非 CGO バイナリ → offset = 0 |
| AC-4 | `TestParsePclntab_InvalidPclntab` | 壊れた pclntab → `ErrInvalidPclntab` または `ErrUnsupportedPclntab` |
| AC-5 | `TestParsePclntab_UnsupportedVersion` | Go 1.18–1.19 バイナリ（magic = 0xfffffff0）→ `ErrUnsupportedPclntabVersion` |
| AC-6 | `TestParsePclntab_Go126CGO` | Go 1.26 ビルド CGO バイナリで offset = C_startup_size を検出 |

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
