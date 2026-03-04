# 詳細仕様書: Mach-O バイナリ解析によるネットワーク操作検出（macOS 対応）

## 0. 前提と参照

受け入れ条件は [01_requirements.md](01_requirements.md) で定義されている。アーキテクチャ設計は
[02_architecture.md](02_architecture.md) を参照。

## 1. パッケージ構成

### 1.1 新規追加ファイル

```
internal/runner/security/machoanalyzer/
    analyzer.go              # StandardMachOAnalyzer 構造体定義・コンストラクタ
    standard_analyzer.go     # AnalyzeNetworkSymbols 実装
    symbol_normalizer.go     # シンボル名正規化（normalizeSymbolName）
    svc_scanner.go           # svc #0x80 命令スキャン（containsSVCInstruction）
    analyzer_test.go         # ユニットテスト（//go:build test）
    testdata/
        network_macho_arm64      # socket 等をインポートする arm64 Mach-O（C/C++）
        no_network_macho_arm64   # ネットワークシンボルなしの arm64 Mach-O
        svc_only_arm64           # svc #0x80 のみ（ネットワークシンボルなし）の arm64 バイナリ
        fat_binary               # arm64 + x86_64 Fat バイナリ
        network_go_macho_arm64   # net パッケージを使用する Go バイナリ（arm64）
        no_network_go_arm64      # ネットワーク操作なし Go バイナリ（arm64）
        script.sh                # 非 Mach-O ファイル
```

### 1.2 変更ファイル

```
internal/runner/security/elfanalyzer/
    analyzer.go                  # ELFAnalyzer → BinaryAnalyzer、NotELFBinary → NotSupportedBinary にリネーム
    standard_analyzer.go         # NotELFBinary → NotSupportedBinary（参照箇所を更新）
    analyzer_test.go             # NotELFBinary → NotSupportedBinary（参照箇所を更新）
internal/runner/security/
    network_analyzer.go          # フィールドを BinaryAnalyzer に変更、macOS フォールバック追加
    network_analyzer_test_helpers.go  # NewNetworkAnalyzerWithELFAnalyzer → NewNetworkAnalyzerWithBinaryAnalyzer にリネーム
    command_analysis_test.go     # mockELFAnalyzer → mockBinaryAnalyzer、関連参照を一括更新
```

## 2. elfanalyzer パッケージの変更

### 2.1 BinaryAnalyzer インターフェース（analyzer.go）

`ELFAnalyzer` インターフェースを `BinaryAnalyzer` にリネームする。コメント・ドキュメントも合わせて更新する。

```go
// BinaryAnalyzer defines the interface for binary network analysis,
// independent of the binary format (ELF, Mach-O, etc.).
type BinaryAnalyzer interface {
    // AnalyzeNetworkSymbols examines the binary at the given path
    // and determines if it contains network-related symbols.
    //
    // contentHash is the pre-computed hash in "algo:hex" format (e.g. "sha256:abc123...").
    // Pass an empty string when no pre-computed hash is available.
    //
    // Returns:
    //   - NetworkDetected: Binary contains network-related symbols
    //   - NoNetworkSymbols: Binary has no network-related symbols
    //   - NotSupportedBinary: File format is not supported by this analyzer
    //   - StaticBinary: Binary is statically linked (ELF-specific)
    //   - AnalysisError: An error occurred (check Error field)
    AnalyzeNetworkSymbols(path string, contentHash string) AnalysisOutput
}
```

### 2.2 NotSupportedBinary 定数（analyzer.go）

`NotELFBinary` 定数を `NotSupportedBinary` にリネームする。`String()` メソッドも合わせて更新する。

```go
const (
    NetworkDetected  AnalysisResult = iota
    NoNetworkSymbols

    // NotSupportedBinary indicates that the file format is not supported
    // by this analyzer (e.g., ELF analyzer receiving a Mach-O file,
    // or Mach-O analyzer receiving an ELF file).
    NotSupportedBinary

    StaticBinary
    AnalysisError
)
```

`String()` の更新:
```go
case NotSupportedBinary:
    return "not_supported_binary"
```

### 2.3 AnalysisResult の String() 更新

`NotELFBinary` の case が `NotSupportedBinary` に変更されるため、`String()` 全体を以下のように更新する:

```go
func (r AnalysisResult) String() string {
    switch r {
    case NetworkDetected:
        return "network_detected"
    case NoNetworkSymbols:
        return "no_network_symbols"
    case NotSupportedBinary:
        return "not_supported_binary"
    case StaticBinary:
        return "static_binary"
    case AnalysisError:
        return "analysis_error"
    default:
        return fmt.Sprintf("unknown(%d)", int(r))
    }
}
```

### 2.4 standard_analyzer.go の更新

`AnalyzeNetworkSymbols` メソッド内の `NotELFBinary` 参照を `NotSupportedBinary` に変更する（2 箇所）。

## 3. machoanalyzer パッケージ

### 3.1 パッケージ宣言・doc

```go
// Package machoanalyzer implements BinaryAnalyzer for Mach-O binaries (macOS/arm64).
// It uses Go's standard debug/macho package to inspect imported symbols
// and detect network-related function calls.
package machoanalyzer
```

### 3.2 StandardMachOAnalyzer（analyzer.go）

```go
package machoanalyzer

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// StandardMachOAnalyzer implements elfanalyzer.BinaryAnalyzer for Mach-O binaries.
// It analyzes imported symbols and scans for svc #0x80 instructions (arm64 only).
type StandardMachOAnalyzer struct {
    fs             safefileio.FileSystem
    networkSymbols map[string]elfanalyzer.SymbolCategory
}

// NewStandardMachOAnalyzer creates a new StandardMachOAnalyzer.
// If fs is nil, safefileio.NewFileSystem(safefileio.FileSystemConfig{}) is used as the default.
// networkSymbols is obtained from elfanalyzer.GetNetworkSymbols().
func NewStandardMachOAnalyzer(fs safefileio.FileSystem) *StandardMachOAnalyzer {
    if fs == nil {
        fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
    }
    return &StandardMachOAnalyzer{
        fs:             fs,
        networkSymbols: elfanalyzer.GetNetworkSymbols(),
    }
}
```

### 3.3 AnalyzeNetworkSymbols（standard_analyzer.go）

`BinaryAnalyzer` インターフェースを実装する中心メソッド。

```go
package machoanalyzer

import (
    "debug/macho"
    "fmt"
    "io"
    "log/slog"
    "os"
    "runtime"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// AnalyzeNetworkSymbols implements elfanalyzer.BinaryAnalyzer.
//
// Returns:
//   - NetworkDetected: Binary imports network-related symbols
//   - NoNetworkSymbols: No network symbols and no svc #0x80 detected
//   - NotSupportedBinary: File is not in Mach-O format, or is a
//     x86_64-only Fat binary (arm64 slice not found)
//   - AnalysisError: Parse error, or svc #0x80 detected (high risk)
func (a *StandardMachOAnalyzer) AnalyzeNetworkSymbols(path string, contentHash string) elfanalyzer.AnalysisOutput {
    // Step 1: safefileio でファイルを安全にオープン
    file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.AnalysisError,
            Error:  fmt.Errorf("failed to open file: %w", err),
        }
    }
    defer func() {
        if closeErr := file.Close(); closeErr != nil {
            slog.Warn("error closing file during Mach-O analysis", slog.Any("error", closeErr))
        }
    }()

    // Step 2: ファイルタイプチェック（正規ファイルのみ許可）
    // Step 3: 先頭4バイトを読み取り、Mach-O / Fat マジックを確認
    // Step 4: Mach-O ファイルとして解析（Fat は arm64 スライスを抽出）
    // Step 5: ImportedSymbols() でインポートシンボルを取得し正規化してネットワークシンボルと照合
    // Step 6: 一致あり → NetworkDetected を返す
    // Step 7: 一致なし → __TEXT,__text セクションで svc #0x80 を検索
    //   - 検出: AnalysisError (high risk) を返す
    //   - 未検出: NoNetworkSymbols を返す
}
```

#### 3.3.1 Mach-O マジックナンバーの定義

```go
// Mach-O magic numbers (see <mach-o/loader.h>)
const (
    machoMagic64    = 0xFEEDFACF // 64-bit Mach-O (native endian)
    machoCigam64    = 0xCFFAEDFE // 64-bit Mach-O (byte-swapped)
    fatMagic        = 0xCAFEBABE // Fat binary
    fatCigam        = 0xBEBAFECA // Fat binary (byte-swapped)
)
```

判定関数:
```go
// isMachOMagic returns true if the first 4 bytes match any Mach-O or Fat binary magic.
func isMachOMagic(b []byte) bool {
    if len(b) < 4 {
        return false
    }
    magic := binary.LittleEndian.Uint32(b[:4])
    switch magic {
    case machoMagic64, machoCigam64, fatMagic, fatCigam:
        return true
    }
    return false
}
```

#### 3.3.2 ファイルオープンとマジックチェック

```go
// openMachOFile opens the Mach-O file at path.
// Returns (machOFile, nil) on success.
// Returns (nil, AnalysisOutput{NotSupportedBinary}) if file is not Mach-O.
// Returns (nil, AnalysisOutput{AnalysisError}) on I/O or parse error.
func (a *StandardMachOAnalyzer) openMachOFile(file safefileio.File) (*macho.File, *elfanalyzer.AnalysisOutput)
```

実装手順:
1. `file.Stat()` で正規ファイル確認（非正規ファイルは `AnalysisError`）
2. 先頭4バイトを `io.ReadFull` で読み取り
3. `isMachOMagic` で確認 → false なら `NotSupportedBinary` を返す
4. Fat バイナリの場合は `selectMachOFromFat` で arm64 スライスを抽出
5. `macho.NewFile(file)` でパース → エラーなら `AnalysisError`

#### 3.3.3 Fat バイナリのスライス選択（standard_analyzer.go）

```go
// selectMachOFromFat selects the arm64 slice from a Fat binary.
// Returns an error if no arm64 slice is found.
func selectMachOFromFat(fat *macho.FatFile) (*macho.File, error) {
    for _, arch := range fat.Arches {
        if arch.Cpu == macho.CpuArm64 {
            return arch.File, nil
        }
    }
    return nil, fmt.Errorf("no arm64 slice found in Fat binary")
}
```

エラー時の戻り値: `NotSupportedBinary`（arm64 スライスなし = 解析対象外）。

**注意**: `macho.FatFile` と `macho.File` のリソース管理に注意する。`macho.FatFile.Close()` を呼ぶと
内包する `macho.File` も閉じられる。`selectMachOFromFat` が返す `*macho.File` を直接使用し、
`FatFile` の Close は呼び出し側で管理する。

### 3.4 シンボル名正規化（symbol_normalizer.go）

```go
package machoanalyzer

import "strings"

// normalizeSymbolName strips the leading underscore and version suffix
// from a macOS imported symbol name.
//
// Examples:
//   normalizeSymbolName("_socket")          → "socket"
//   normalizeSymbolName("_socket$UNIX2003") → "socket"
//   normalizeSymbolName("socket")           → "socket"
func normalizeSymbolName(name string) string {
    // Strip leading underscore (macOS C symbol convention)
    if strings.HasPrefix(name, "_") {
        name = name[1:]
    }
    // Strip version suffix (e.g., "$UNIX2003", "$INODE64")
    if idx := strings.IndexByte(name, '$'); idx >= 0 {
        name = name[:idx]
    }
    return name
}
```

### 3.5 svc #0x80 命令スキャン（svc_scanner.go）

```go
package machoanalyzer

import (
    "debug/macho"
    "encoding/binary"
)

// svcInstruction is the encoding of "svc #0x80" for arm64 (little-endian).
// ARM64 encoding: 0xD4001001 → bytes [0x01, 0x10, 0x00, 0xD4]
var svcInstruction = []byte{0x01, 0x10, 0x00, 0xD4}

// containsSVCInstruction scans the __TEXT,__text section of a Mach-O file
// for the svc #0x80 instruction (0xD4001001 in little-endian).
//
// Uses 4-byte aligned scan, exploiting arm64 fixed-width instruction encoding.
// Only processes arm64 binaries (CpuArm64); returns false for other architectures.
//
// Background: Regular macOS binaries (both Go and C) call libSystem.dylib for
// system calls and never contain svc #0x80 directly. Its presence indicates
// a direct kernel call, bypassing libSystem.dylib.
func containsSVCInstruction(f *macho.File) bool {
    if f.Cpu != macho.CpuArm64 {
        return false
    }

    section := f.Section("__TEXT", "__text")
    if section == nil {
        return false
    }

    data, err := section.Data()
    if err != nil {
        return false
    }

    target := binary.LittleEndian.Uint32(svcInstruction)
    for i := 0; i+4 <= len(data); i += 4 {
        if binary.LittleEndian.Uint32(data[i:i+4]) == target {
            return true
        }
    }
    return false
}
```

**エンコード根拠**: `svc #0x80` の ARM64 命令語は `0xD4001001`。リトルエンディアンで格納すると
`01 10 00 D4`。アセンブルで確認済み（要件書の `01 80 00 D4` は誤記、設計書参照）。

**x86_64 は対象外**: macOS のサポート対象は arm64 のみ。`f.Cpu != macho.CpuArm64` の
ガードで非 arm64 バイナリに対しては `false` を返す。

**4バイトアラインスキャン**: arm64 命令は4バイト境界に整列するため、4バイト刻みでスキャンする。
実バイナリ（216,985命令）での計測で誤検知は0件。

### 3.6 AnalyzeNetworkSymbols 完全実装（standard_analyzer.go）

```go
func (a *StandardMachOAnalyzer) AnalyzeNetworkSymbols(path string, contentHash string) elfanalyzer.AnalysisOutput {
    file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.AnalysisError,
            Error:  fmt.Errorf("failed to open file: %w", err),
        }
    }
    defer func() {
        if closeErr := file.Close(); closeErr != nil {
            slog.Warn("error closing file during Mach-O analysis", slog.Any("error", closeErr))
        }
    }()

    // ファイルタイプ確認（正規ファイルのみ）
    fileInfo, err := file.Stat()
    if err != nil {
        return elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.AnalysisError,
            Error:  fmt.Errorf("failed to stat file: %w", err),
        }
    }
    if !fileInfo.Mode().IsRegular() {
        return elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.NotSupportedBinary,
            Error:  fmt.Errorf("not a regular file: %s", fileInfo.Mode()),
        }
    }

    // マジックナンバー確認
    magic := make([]byte, 4)
    if _, err := io.ReadFull(file, magic); err != nil {
        return elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.AnalysisError,
            Error:  fmt.Errorf("failed to read magic: %w", err),
        }
    }
    if !isMachOMagic(magic) {
        return elfanalyzer.AnalysisOutput{Result: elfanalyzer.NotSupportedBinary}
    }

    // Seek back to start for macho.NewFile / macho.NewFatFile
    if _, err := file.Seek(0, io.SeekStart); err != nil {
        return elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.AnalysisError,
            Error:  fmt.Errorf("failed to seek: %w", err),
        }
    }

    // Fat バイナリ判定と arm64 スライス抽出
    machOFile, closer, output := a.parseMachO(file, magic)
    if output != nil {
        return *output
    }
    defer func() {
        if closeErr := closer.Close(); closeErr != nil {
            slog.Warn("error closing Mach-O file", slog.Any("error", closeErr))
        }
    }()

    // インポートシンボル取得・正規化・照合
    symbols, err := machOFile.ImportedSymbols()
    if err != nil {
        return elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.AnalysisError,
            Error:  fmt.Errorf("failed to get imported symbols: %w", err),
        }
    }

    var detected []elfanalyzer.DetectedSymbol
    for _, sym := range symbols {
        normalized := normalizeSymbolName(sym)
        if cat, found := a.networkSymbols[normalized]; found {
            detected = append(detected, elfanalyzer.DetectedSymbol{
                Name:     normalized,
                Category: string(cat),
            })
        }
    }

    if len(detected) > 0 {
        return elfanalyzer.AnalysisOutput{
            Result:          elfanalyzer.NetworkDetected,
            DetectedSymbols: detected,
        }
    }

    // svc #0x80 スキャン（シンボルが検出されなかった場合のみ）
    if containsSVCInstruction(machOFile) {
        return elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.AnalysisError,
            Error:  fmt.Errorf("svc #0x80 instruction detected (direct syscall, high risk)"),
        }
    }

    return elfanalyzer.AnalysisOutput{Result: elfanalyzer.NoNetworkSymbols}
}
```

#### parseMachO ヘルパー

`parseMachO` は `*macho.File` とともにそれを閉じるための `io.Closer` を返す。
Fat バイナリの場合は `*macho.FatFile`、単一 Mach-O の場合は `*macho.File` 自身を
`io.Closer` として返す。クローズの責務は呼び出し元（`AnalyzeNetworkSymbols`）が担う。

**設計根拠**: `defer fat.Close()` を `parseMachO` 内で行うと、関数リターン時に
`*macho.FatFile` が閉じられ、返却した `arch.File`（スライス）が use-after-close となる。
`io.Closer` を呼び出し元に返すことでこの問題を回避する。

```go
// parseMachO parses the file as a Mach-O or Fat binary.
// For Fat binaries, selects the arm64 slice.
//
// Returns (*macho.File, io.Closer, nil) on success.
// The caller must call closer.Close() when done with the *macho.File.
// For Fat binaries, closer is the *macho.FatFile; for single Mach-O, closer is the *macho.File itself.
//
// Returns (nil, nil, &AnalysisOutput) when the binary cannot be parsed or arm64 slice is absent.
func (a *StandardMachOAnalyzer) parseMachO(file safefileio.File, magic []byte) (*macho.File, io.Closer, *elfanalyzer.AnalysisOutput) {
    m := binary.LittleEndian.Uint32(magic)
    if m == fatMagic || m == fatCigam {
        // Fat バイナリ: arm64 スライスを抽出
        fat, err := macho.NewFatFile(file)
        if err != nil {
            output := elfanalyzer.AnalysisOutput{
                Result: elfanalyzer.AnalysisError,
                Error:  fmt.Errorf("failed to parse Fat binary: %w", err),
            }
            return nil, nil, &output
        }

        slice, err := selectMachOFromFat(fat)
        if err != nil {
            // arm64 スライスなし: fat を閉じてからエラーを返す
            _ = fat.Close()
            output := elfanalyzer.AnalysisOutput{Result: elfanalyzer.NotSupportedBinary}
            return nil, nil, &output
        }
        // fat を closer として返す。slice は fat が生きている間だけ有効
        return slice, fat, nil
    }

    // 単一 Mach-O
    machOFile, err := macho.NewFile(file)
    if err != nil {
        output := elfanalyzer.AnalysisOutput{
            Result: elfanalyzer.AnalysisError,
            Error:  fmt.Errorf("failed to parse Mach-O: %w", err),
        }
        return nil, nil, &output
    }
    return machOFile, machOFile, nil
}
```

呼び出し側の `AnalyzeNetworkSymbols` でのクローズ:

```go
machOFile, closer, output := a.parseMachO(file, magic)
if output != nil {
    return *output
}
defer func() {
    if closeErr := closer.Close(); closeErr != nil {
        slog.Warn("error closing Mach-O file", slog.Any("error", closeErr))
    }
}()
// machOFile を安全に使用できる
```

## 4. NetworkAnalyzer の変更

### 4.1 フィールドのリネームと macOS 対応（network_analyzer.go）

```go
package security

import (
    "log/slog"
    "path/filepath"
    "runtime"
    "slices"
    "strings"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/machoanalyzer"
)

// NetworkAnalyzer provides network operation detection for commands.
type NetworkAnalyzer struct {
    binaryAnalyzer elfanalyzer.BinaryAnalyzer
}

// NewNetworkAnalyzer creates a new NetworkAnalyzer.
// On macOS, uses StandardMachOAnalyzer; on Linux and other platforms, uses StandardELFAnalyzer.
func NewNetworkAnalyzer() *NetworkAnalyzer {
    var analyzer elfanalyzer.BinaryAnalyzer
    switch runtime.GOOS {
    case "darwin":
        analyzer = machoanalyzer.NewStandardMachOAnalyzer(nil)
    default: // "linux", etc.
        analyzer = elfanalyzer.NewStandardELFAnalyzer(nil, nil)
    }
    return &NetworkAnalyzer{binaryAnalyzer: analyzer}
}
```

### 4.2 isNetworkViaBinaryAnalysis（network_analyzer.go）

`isNetworkViaELFAnalysis` を `isNetworkViaBinaryAnalysis` にリネームし、
case 文のコメントと log メッセージを更新する。

```go
func (a *NetworkAnalyzer) isNetworkViaBinaryAnalysis(cmdPath string, contentHash string) bool {
    if !filepath.IsAbs(cmdPath) {
        panic("isNetworkViaBinaryAnalysis: cmdPath must be an absolute path, got: " + cmdPath)
    }

    output := a.binaryAnalyzer.AnalyzeNetworkSymbols(cmdPath, contentHash)

    switch output.Result {
    case elfanalyzer.NetworkDetected:
        slog.Debug("Binary analysis detected network symbols",
            "path", cmdPath,
            "symbols", formatDetectedSymbols(output.DetectedSymbols))
        return true

    case elfanalyzer.NoNetworkSymbols:
        slog.Debug("Binary analysis found no network symbols",
            "path", cmdPath)
        return false

    case elfanalyzer.NotSupportedBinary:
        // File format is not supported by this analyzer (e.g., ELF analyzer
        // receiving a Mach-O, or Mach-O analyzer receiving an ELF).
        // Assume no network operation, consistent with ELF/Mach-O mismatch handling.
        slog.Debug("Binary analysis: unsupported binary format, assuming no network operation",
            "path", cmdPath)
        return false

    case elfanalyzer.StaticBinary:
        slog.Debug("Binary analysis: static binary detected, cannot determine network capability",
            "path", cmdPath)
        return false

    case elfanalyzer.AnalysisError:
        slog.Warn("Binary analysis failed, treating as potential network operation",
            "path", cmdPath,
            "error", output.Error,
            "reason", "Unable to determine network capability, assuming middle risk for safety")
        return true

    default:
        slog.Warn("Binary analysis returned unknown result",
            "path", cmdPath,
            "result", output.Result)
        return true
    }
}
```

`IsNetworkOperation` 内の `a.isNetworkViaELFAnalysis` 呼び出しを `a.isNetworkViaBinaryAnalysis` に変更する。

### 4.3 テストヘルパー（network_analyzer_test_helpers.go）

```go
//go:build test

package security

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// NewNetworkAnalyzerWithBinaryAnalyzer creates a NetworkAnalyzer with a custom BinaryAnalyzer.
// This function is only available in test builds.
func NewNetworkAnalyzerWithBinaryAnalyzer(analyzer elfanalyzer.BinaryAnalyzer) *NetworkAnalyzer {
    return &NetworkAnalyzer{binaryAnalyzer: analyzer}
}
```

## 5. テスト仕様

### 5.1 テストフィクスチャの用意方針

macOS SDK がない Linux 環境でのクロスコンパイルが困難なため、実際のバイナリをリポジトリに含める。
各フィクスチャの生成コマンドを `testdata/README.md` に記録する。

| ファイル | 説明 | 生成方法 |
|--------|------|---------|
| `network_macho_arm64` | socket をインポートする arm64 C バイナリ | macOS で `cc -target arm64-apple-macos11` |
| `no_network_macho_arm64` | ネットワークシンボルなし arm64 バイナリ | macOS で `cc -target arm64-apple-macos11` |
| `svc_only_arm64` | `svc #0x80` のみを含む最小バイナリ | macOS arm64 でアセンブルしてリンク |
| `fat_binary` | arm64 + x86_64 Fat バイナリ | macOS で `lipo` を使用して結合 |
| `network_go_macho_arm64` | `net.Dial` を使う Go バイナリ | `GOOS=darwin GOARCH=arm64 go build` |
| `no_network_go_arm64` | ネット操作なし Go バイナリ | `GOOS=darwin GOARCH=arm64 go build` |
| `script.sh` | 非 Mach-O（シェルスクリプト） | テキストファイルとして作成 |

### 5.2 テストケース一覧（analyzer_test.go）

テストファイルは `//go:build test` ビルドタグを付与する。

| テストケース | 入力 | 期待結果 | 対応 AC |
|------------|------|---------|--------|
| `TestStandardMachOAnalyzer_NetworkSymbols_Detected` | `network_macho_arm64` | `NetworkDetected`、シンボルあり | AC-2 |
| `TestStandardMachOAnalyzer_NoNetworkSymbols` | `no_network_macho_arm64` | `NoNetworkSymbols` | AC-2 |
| `TestStandardMachOAnalyzer_SVCOnly_HighRisk` | `svc_only_arm64` | `AnalysisError`（high risk） | AC-6 |
| `TestStandardMachOAnalyzer_NetworkSymbols_SVCIgnored` | ネットワークシンボルあり + svc #0x80 バイナリ | `NetworkDetected`（シンボル優先） | AC-6 |
| `TestStandardMachOAnalyzer_FatBinary_Arm64Selected` | `fat_binary` | arm64 スライスで判定 | AC-1 |
| `TestStandardMachOAnalyzer_FatBinary_NoArm64Slice` | x86_64 のみの Fat バイナリ | `NotSupportedBinary` | AC-1 |
| `TestStandardMachOAnalyzer_GoNetwork_Detected` | `network_go_macho_arm64` | `NetworkDetected` | AC-3 |
| `TestStandardMachOAnalyzer_GoNoNetwork_NoSymbols` | `no_network_go_arm64` | `NoNetworkSymbols` | AC-3 |
| `TestStandardMachOAnalyzer_NonMachO_Script` | `script.sh` | `NotSupportedBinary` | AC-1 |
| `TestStandardMachOAnalyzer_InvalidMachO_NoPanic` | 破損 Mach-O | `AnalysisError`（パニックなし） | AC-5 |
| `TestStandardMachOAnalyzer_FileOpenError` | 存在しないパス | `AnalysisError` | AC-5 |

### 5.3 symbol_normalizer テスト

```go
func TestNormalizeSymbolName(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"_socket", "socket"},
        {"_socket$UNIX2003", "socket"},
        {"socket", "socket"},
        {"_connect$INODE64", "connect"},
        {"SSL_connect", "SSL_connect"},
        {"", ""},
    }
    // ...
}
```

### 5.4 containsSVCInstruction テスト

モック `macho.File` の作成が困難なため、`AnalyzeNetworkSymbols` の統合テスト経由で検証する。
`svc_only_arm64` フィクスチャで `AnalysisError` が返ることで間接的に確認する。

### 5.5 既存テストの更新（command_analysis_test.go）

`mockELFAnalyzer` 型を `mockBinaryAnalyzer` にリネームし、
`NewNetworkAnalyzerWithELFAnalyzer` 呼び出しを `NewNetworkAnalyzerWithBinaryAnalyzer` に変更する。
`elfanalyzer.NotELFBinary` 参照を `elfanalyzer.NotSupportedBinary` に変更する。

### 5.6 統合テスト（macOS のみ）

macOS 環境でのみ実行する統合テストを用意し、`runtime.GOOS != "darwin"` の場合はスキップする。

```go
func TestNetworkAnalyzer_Integration_MachO(t *testing.T) {
    if runtime.GOOS != "darwin" {
        t.Skip("macOS-only integration test")
    }
    // ... /usr/bin/curl 等の実バイナリで NetworkDetected を確認
}
```

## 6. エラー処理の詳細

### 6.1 エラー分類

| 発生条件 | Result | 補足 |
|---------|--------|------|
| ファイルオープン失敗（権限不足等） | `AnalysisError` | Middle Risk（安全側） |
| 正規ファイルでない（ディレクトリ等） | `NotSupportedBinary` | 解析スキップ |
| マジックナンバー不一致 | `NotSupportedBinary` | ELF、スクリプト等 |
| Fat バイナリに arm64 スライスなし | `NotSupportedBinary` | x86_64 のみの Fat |
| Mach-O パースエラー | `AnalysisError` | Middle Risk |
| ImportedSymbols() エラー | `AnalysisError` | Middle Risk |
| svc #0x80 検出 | `AnalysisError` | High Risk |

### 6.2 静的エラー変数

```go
// errors.go（または standard_analyzer.go 内）
var (
    // ErrDirectSyscall indicates svc #0x80 was found, indicating a direct syscall.
    ErrDirectSyscall = errors.New("direct syscall instruction detected (svc #0x80)")
)
```

`AnalysisError` 返却時の `Error` フィールドに `ErrDirectSyscall` を `wrap` することで、
呼び出し側が `errors.Is` で判定可能にする（将来のロギング拡張に対応）。

## 7. リネーム対応一覧

実装時に変更が必要なファイルと変更内容をまとめる。

| ファイル | 変更内容 |
|--------|--------|
| `elfanalyzer/analyzer.go` | `ELFAnalyzer` → `BinaryAnalyzer`、`NotELFBinary` → `NotSupportedBinary`、コメント更新 |
| `elfanalyzer/standard_analyzer.go` | `NotELFBinary` → `NotSupportedBinary`（2箇所） |
| `elfanalyzer/analyzer_test.go` | `NotELFBinary` → `NotSupportedBinary` |
| `security/network_analyzer.go` | `elfAnalyzer` → `binaryAnalyzer`、`ELFAnalyzer` → `BinaryAnalyzer`、`isNetworkViaELFAnalysis` → `isNetworkViaBinaryAnalysis`、`NotELFBinary` → `NotSupportedBinary`、`machoanalyzer` import 追加、`runtime` import 追加 |
| `security/network_analyzer_test_helpers.go` | `NewNetworkAnalyzerWithELFAnalyzer` → `NewNetworkAnalyzerWithBinaryAnalyzer`、`ELFAnalyzer` → `BinaryAnalyzer` |
| `security/command_analysis_test.go` | `mockELFAnalyzer` → `mockBinaryAnalyzer`、`NewNetworkAnalyzerWithELFAnalyzer` → `NewNetworkAnalyzerWithBinaryAnalyzer`、`NotELFBinary` → `NotSupportedBinary` |

## 8. 依存関係

本タスクで追加する外部依存はない。使用するパッケージはすべて Go 標準ライブラリである。

```
debug/macho   # Mach-O / Fat バイナリのパース
encoding/binary # マジックナンバーのバイト解釈
io            # io.ReadFull, io.SeekStart
runtime       # runtime.GOOS による OS 判定
```

## 9. 受け入れ条件との対応

| 受け入れ条件 | 実装箇所 |
|------------|---------|
| AC-1: Mach-O バイナリ判定 | `isMachOMagic`、`selectMachOFromFat`、`parseFatBinaryArm64Slice` |
| AC-2: ネットワークシンボル検出 | `AnalyzeNetworkSymbols` Step 5-6 |
| AC-3: Go バイナリ検出 | `AnalyzeNetworkSymbols`（`ImportedSymbols()` は Go バイナリのシンボルも返す） |
| AC-4: フォールバック動作 | `NetworkAnalyzer.IsNetworkOperation`（変更なし、既存ロジック） |
| AC-5: 解析失敗時の安全性 | `AnalysisError` 返却（ブロックしない）、`debug/macho` はパニックしない |
| AC-6: svc #0x80 high risk 検出 | `containsSVCInstruction`、判定優先順位 |
| AC-7: 既存機能への非影響 | リネームのみ（動作変更なし）、既存テストのパスで確認 |
