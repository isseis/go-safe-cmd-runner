# 詳細仕様書: Mach-O arm64 syscall 番号解析

## 0. 既存機能活用方針

重複実装を避けるため、以下の既存コンポーネントをそのまま利用する：

- **`libccache.BackwardScanX16`**: X16 即値後方スキャン（`backwardScanX16` を公開化するだけ）
- **`libccache.MacOSSyscallTable`**: macOS BSD syscall 番号→ネットワーク判定テーブル
- **`machoanalyzer.collectSVCAddresses`**: `svc #0x80` アドレス列挙（既存）
- **`elfanalyzer.parsePclntabFuncsRaw` のコアロジック**: `gosym.NewLineTable` + `gosym.NewTable` による関数テーブル構築（Mach-O 版を新規実装するが内部ロジックは同じ）
- **`common.SyscallAnalysisResultCore`・`common.SyscallInfo`**: データ型（変更なし）
- **`fileanalysis.SyscallAnalysisStore`**: 保存・読み込みインターフェース（変更なし）

## 1. 変更対象ファイル一覧

```
# 変更
internal/libccache/macho_analyzer.go         # backwardScanX16 → BackwardScanX16 (公開)
internal/fileanalysis/schema.go              # CurrentSchemaVersion 15 → 16
internal/filevalidator/validator.go          # analyzeMachoSyscalls() を拡張（Pass1/Pass2 + 既存 libSystem 統合）
internal/runner/security/network_analyzer.go # 判定ロジック変更、syscallAnalysisHasSVCSignal 削除

# 新規
internal/runner/security/machoanalyzer/pclntab_macho.go
internal/runner/security/machoanalyzer/pass1_scanner.go
internal/runner/security/machoanalyzer/pass2_scanner.go
internal/runner/security/machoanalyzer/syscall_number_analyzer.go
```

## 2. `internal/libccache/macho_analyzer.go`

`backwardScanX16` を `BackwardScanX16` に改名して公開する。シグネチャ・動作・内部実装は変更しない。

```go
// BackwardScanX16 walks backward from the svc #0x80 instruction at
// funcCode[svcOffset] and looks for an immediate-load sequence into x16.
// When found, it returns the syscall number with the BSD class prefix removed.
// The scan is limited to maxBackwardScanInstructions instructions.
// （以降の docstring は既存と同一）
func BackwardScanX16(funcCode []byte, svcOffset int) (int, bool) {
    // 既存の backwardScanX16 の実装をそのまま移動
}
```

同パッケージ内の呼び出し箇所（`analyzeWrapperFunction` 内の `backwardScanX16(funcCode, i)` 呼び出し）を `BackwardScanX16(funcCode, i)` に更新する。

## 3. `internal/fileanalysis/schema.go`

```go
const (
    // Version 16 replaces the Mach-O "direct_svc_0x80" determination method
    // with proper syscall number analysis (Pass 1 + Pass 2). v15 records must
    // be re-recorded before they can be used.
    CurrentSchemaVersion = 16
)
```

## 4. `internal/runner/security/machoanalyzer/pclntab_macho.go`（新規）

### 4.1 型定義

```go
// MachoPclntabFunc holds the address range of a function extracted from
// the Mach-O .gopclntab section.
type MachoPclntabFunc struct {
    Name  string
    Entry uint64
    End   uint64
}
```

### 4.2 `ParseMachoPclntab`

```go
// ParseMachoPclntab reads the __gopclntab section from a Mach-O file and
// returns a map from function name to address range.
//
// Returns ErrNoPclntab when no __gopclntab section exists (stripped binary or
// non-Go binary). Callers must continue Pass 1 and Pass 2 without exclusion/
// resolution in that case.
//
// Only pclntab magic 0xfffffff1 (Go 1.20+) is supported; other versions
// return ErrUnsupportedPclntabVersion.
func ParseMachoPclntab(f *macho.File) (map[string]MachoPclntabFunc, error)
```

実装方針：
1. `f.Section("__gopclntab")` でセクションを取得
   - `nil` の場合は `ErrNoPclntab` を返す
2. `section.Data()` でバイト列を取得
3. `checkPclntabVersion(data, binary.LittleEndian)` でマジックを確認（内部ヘルパー、ELF 版と同一ロジック）
4. `f.Section("__text")` の `Addr` を `textStart` として取得
5. `gosym.NewLineTable(data, textStart)` → `gosym.NewTable(nil, lt)` で関数テーブルを構築
6. CGO オフセット補正: `detectMachoPclntabOffset(f, funcs)` を適用
   - `__TEXT,__text` セクションの BL 命令とのクロスリファレンスで補正量を検出
   - 非 CGO バイナリではオフセット = 0 なので補正なし

エラー定数は `machoanalyzer` パッケージに独立定義する（`elfanalyzer` に同名定義があるが、ELF/Mach-O をパッケージ分離するため。呼び出し側は `errors.Is()` で比較するのでパッケージ非依存に扱える）：

```go
var (
    ErrNoPclntab                 = errors.New("no __gopclntab section found")
    ErrUnsupportedPclntabVersion = errors.New("unsupported pclntab version")
    ErrInvalidPclntab            = errors.New("invalid pclntab data")
)
```

`detectMachoPclntabOffset` の CGO オフセット補正アルゴリズムは ELF 版（`elfanalyzer.detectPclntabOffset`）と同一：
1. pclntab から読み込んだ関数エントリのアドレスで `__TEXT,__text` 内の BL 命令を検索
2. BL の PC 相対オフセット計算値と実際のターゲットアドレスの差分を補正量とする
3. すべての関数エントリに対して補正量が一貫するか検証し、正の値のみ採用

実装時は `elfanalyzer/pclntab_parser.go` の `detectOffsetByCallTargets` を参照すること。

## 5. `internal/runner/security/machoanalyzer/pass1_scanner.go`（新規）

### 5.1 シグネチャ

```go
// scanSVCWithX16 performs Pass 1 analysis: scans svc #0x80 addresses,
// skips those inside known Go stub address ranges, and resolves the X16
// syscall number via backward scan.
//
// svcAddrs: virtual addresses of svc #0x80 instructions (from collectSVCAddresses)
// code:     raw bytes of __TEXT,__text section
// textBase: virtual address of the section start
// stubRanges: address ranges of known Go syscall stub functions (from pclntab)
//
// Returns one SyscallInfo per svc #0x80 that was NOT excluded.
func scanSVCWithX16(
    svcAddrs []uint64,
    code []byte,
    textBase uint64,
    stubRanges []funcRange,
    table libccache.MacOSSyscallTable,
) []common.SyscallInfo
```

### 5.2 ロジック詳細

```
for each addr in svcAddrs:
    if isInsideRange(addr, stubRanges):
        continue  // Go スタブは Pass 2 で処理

    svcOffset := int(addr - textBase)
    num, ok := libccache.BackwardScanX16(code, svcOffset)

    if ok:
        name := table.GetSyscallName(num)
        isNet := table.IsNetworkSyscall(num)
        method := DeterminationMethodImmediate  // elfanalyzer 定数を使用
        Number = num
    else:
        name = ""
        isNet = false
        method = DeterminationMethodUnknownIndirectSetting
        Number = -1

    append SyscallInfo{
        Number:              Number,
        Name:                name,
        IsNetwork:           isNet,
        Location:            addr,
        DeterminationMethod: method,
        Source:              "",  // Mach-O 直接 svc エントリは Source 空（ELF と同形式）
    }
```

### 5.3 funcRange と isInsideRange

`funcRange` 型は `pclntab_macho.go` で定義し、`pass1_scanner.go` および `pass2_scanner.go` の両方から参照する。

```go
// funcRange represents a contiguous address range [start, end).
// Defined in pclntab_macho.go and shared across pass1/pass2 scanners.
type funcRange struct {
    start uint64
    end   uint64
}

// isInsideRange reports whether addr falls within any range in ranges.
// ranges must be sorted by start for binary search (O(log n)).
// Used by both Pass 1 (stubRanges) and Pass 2 (wrapperRanges).
func isInsideRange(addr uint64, ranges []funcRange) bool
```

`stubRanges` は `knownMachoSyscallImpls` に対応する関数のアドレス範囲を格納する。

### 5.4 knownMachoSyscallImpls（pclntab 照合用）

```go
// knownMachoSyscallImpls is the set of known Go syscall stub function names
// whose bodies contain direct svc #0x80 with caller-supplied syscall numbers.
// These are excluded from Pass 1 and their call sites are analyzed by Pass 2.
var knownMachoSyscallImpls = map[string]struct{}{
    "syscall.Syscall":   {},
    "syscall.Syscall6":  {},
    "syscall.RawSyscall":  {},
    "syscall.RawSyscall6": {},
    // Go version-dependent internal stubs:
    "internal/runtime/syscall.Syscall6": {},
}
```

pclntab から関数エントリを読み込む際、名前が `knownMachoSyscallImpls` に含まれるものを `stubRanges` に追加する。

## 6. `internal/runner/security/machoanalyzer/pass2_scanner.go`（新規）

### 6.1 型定義

```go
// MachoWrapperCall represents a resolved call to a known Go syscall wrapper.
type MachoWrapperCall struct {
    CallSiteAddress     uint64
    TargetFunction      string
    SyscallNumber       int
    DeterminationMethod string
}
```

### 6.2 MachoWrapperResolver

```go
// MachoWrapperResolver resolves Go syscall wrapper call sites in Mach-O arm64
// binaries using the old stack ABI: the first argument (trap/syscall number)
// is stored at [SP, #8] by the caller before the BL instruction.
type MachoWrapperResolver struct {
    wrapperAddrs map[uint64]string  // start address → wrapper function name
    wrapperRanges []funcRange       // for IsInsideWrapper check
}

// NewMachoWrapperResolver creates a resolver from pclntab function info.
// Functions not in knownMachoGoWrappers are silently ignored.
// Returns a resolver with empty maps (HasWrappers() == false) when funcs is nil
// or contains no known wrapper functions.
func NewMachoWrapperResolver(funcs map[string]MachoPclntabFunc) *MachoWrapperResolver

// HasWrappers reports whether any known wrapper function addresses were resolved.
// Returns false when pclntab was unavailable or no known wrappers were found.
func (r *MachoWrapperResolver) HasWrappers() bool

// FindWrapperCalls scans code for BL instructions targeting known wrappers
// and resolves the syscall number from [SP, #8] setup preceding the BL.
func (r *MachoWrapperResolver) FindWrapperCalls(
    code []byte,
    textBase uint64,
    table libccache.MacOSSyscallTable,
) []MachoWrapperCall
```

### 6.3 knownMachoGoWrappers

```go
// knownMachoGoWrappers is the set of known Go syscall wrapper function names
// for macOS arm64. All use the old stack ABI (first arg at [SP, #8]).
var knownMachoGoWrappers = map[string]struct{}{
    "syscall.Syscall":     {},
    "syscall.Syscall6":    {},
    "syscall.RawSyscall":  {},
    "syscall.RawSyscall6": {},
    "runtime.syscall":     {},
    "runtime.syscall6":    {},
}
```

### 6.4 BL 命令検出

arm64 BL 命令のエンコーディング：
```
[31:26] = 100101
[25:0]  = imm26（PC相対オフセット、4バイト単位）
target = PC + SignExtend(imm26 << 2)
```

```go
// decodeBLTarget returns (targetAddr, true) if word is a BL instruction,
// or (0, false) otherwise.
func decodeBLTarget(word uint32, instrAddr uint64) (uint64, bool) {
    if word>>26 != 0b100101 {
        return 0, false
    }
    imm26 := int64(word & 0x03FFFFFF)
    if imm26 >= 1<<25 {  // 符号拡張
        imm26 -= 1 << 26
    }
    return uint64(int64(instrAddr) + imm26*4), true //nolint:gosec
}
```

### 6.5 呼び出しサイト後方スキャン（旧スタック ABI）

`[SP, #8]` は調査対象の Go arm64 Mach-O スタブで確認された第1引数スロットである（`SP+0` = 戻りアドレス用スロット、`SP+8` = `trap+0(FP)`）。

```
Pass 2 後方スキャン手順（BL 命令アドレスから最大 defaultMaxBackwardScan 命令）：

Phase A: [SP, #8] への書き込みストア命令を探す
  - STP xN, xM, [SP, #8]: bits[31:30]=10, load/store pair, offset=#8 → xN = first register
  - STR xN, [SP, #8]:     standard store, offset=#8 → xN

Phase B: xN への即値設定命令を探す（Phase A で xN 確定後）
  - MOVZ xN, #imm                    → syscall 番号 = imm
  - MOVZ xN, #hi, LSL#16 + MOVK xN, #lo → syscall 番号 = hi<<16 | lo
  - 制御フロー命令でスキャン停止

ARM64 STP エンコーディング（オフセット付きペアストア）：
  [31:30] = 10（64bit）
  [29:27] = 101（STP, オフセット付き）
  [26]    = 0
  [25:22] = 0010（signed offset, post/pre/offset モード判別に注意）
  imm7 = [21:15]（8バイト単位、実際の offset = imm7 × 8）
  Rt2  = [14:10]
  Rn   = [9:5]（= SP = 31）
  Rt   = [4:0]

  [SP, #8]: Rn=31, offset=8 → imm7 = 1（8÷8）

ARM64 STR エンコーディング（即値オフセット）：
  [31:30] = 11（64bit）
  [29:27] = 111
  [26]    = 0
  [25:24] = 01
  imm12  = [21:10]（8バイト単位、実際の offset = imm12 × 8）
  Rn     = [9:5]（= SP = 31）
  Rt     = [4:0]

  [SP, #8]: Rn=31, imm12 = 1（8÷8）
```

解決成功時: `DeterminationMethod = "go_wrapper"`（`elfanalyzer.DeterminationMethodGoWrapper` と同一値）

解決失敗時: `DeterminationMethod = "unknown:indirect_setting"`、`SyscallNumber = -1`

制御フロー境界: `isControlFlowInstruction(word)` を使用（`libccache/macho_analyzer.go` から `IsControlFlowInstruction` として公開するか、`machoanalyzer` に複製する）。

### 6.6 FindWrapperCalls のスキャンループ

```
for each 4-byte aligned instruction in code:
    word = code[offset]
    target, ok = decodeBLTarget(word, textBase + offset)
    if !ok: continue

    if target ∉ r.wrapperAddrs: continue

    callSiteAddr = textBase + offset
    if isInsideRange(callSiteAddr, r.wrapperRanges): continue  // ラッパー内からの再帰 BL をスキップ

    num, method = resolveStackABIArg(code, offset)
    append MachoWrapperCall{CallSiteAddress: callSiteAddr, TargetFunction: ..., SyscallNumber: num, ...}
```

## 7. `internal/runner/security/machoanalyzer/syscall_number_analyzer.go`（新規）

### 7.1 型定義

```go
// MachoSyscallNumberAnalyzer analyzes Mach-O arm64 binaries for direct syscall
// instructions (Pass 1) and Go wrapper call sites (Pass 2).
type MachoSyscallNumberAnalyzer struct{}

// NewMachoSyscallNumberAnalyzer creates a new MachoSyscallNumberAnalyzer.
func NewMachoSyscallNumberAnalyzer() *MachoSyscallNumberAnalyzer
```

### 7.2 Analyze

```go
// Analyze runs Pass 1 and Pass 2 on the given Mach-O file (arm64 slice only).
// For Fat binaries, callers must extract the arm64 slice before calling.
// Returns nil when f is not arm64 or has no __TEXT,__text section.
func (a *MachoSyscallNumberAnalyzer) Analyze(f *macho.File) (*fileanalysis.SyscallAnalysisResult, error) {
    if f.Cpu != macho.CpuArm64 {
        return nil, nil
    }

    // 1. Load pclntab (best-effort; ErrNoPclntab is non-fatal)
    funcs, err := ParseMachoPclntab(f)
    var noPclntab bool
    if errors.Is(err, ErrNoPclntab) || errors.Is(err, ErrUnsupportedPclntabVersion) {
        noPclntab = true
    } else if err != nil {
        return nil, fmt.Errorf("pclntab parse failed: %w", err)
    }

    // 2. Build stub ranges and wrapper resolver from pclntab
    stubRanges := buildStubRanges(funcs)
    resolver := NewMachoWrapperResolver(funcs)

    // 3. Collect svc #0x80 addresses
    svcAddrs, err := collectSVCAddresses(f)
    if err != nil {
        return nil, fmt.Errorf("svc scan failed: %w", err)
    }

    // 4. Read code section
    textSection := f.Section("__text")
    if textSection == nil || textSection.Seg != "__TEXT" {
        return nil, nil
    }
    code, err := textSection.Data()
    if err != nil {
        return nil, fmt.Errorf("failed to read __TEXT,__text: %w", err)
    }
    textBase := textSection.Addr

    table := libccache.MacOSSyscallTable{}

    // 5. Pass 1
    var pass1Results []common.SyscallInfo
    if len(svcAddrs) > 0 {
        pass1Results = scanSVCWithX16(svcAddrs, code, textBase, stubRanges, table)
    }

    // 6. Pass 2 (only when pclntab available and wrappers resolved)
    var pass2Results []common.SyscallInfo
    if !noPclntab && resolver.HasWrappers() {
        calls := resolver.FindWrapperCalls(code, textBase, table)
        pass2Results = wrapperCallsToSyscallInfos(calls, table)
    }

    // 7. Merge
    // Pass 1 と Pass 2 の結果は排他的（Pass 1 はスタブ範囲外、Pass 2 はスタブ範囲内の
    // 呼び出しサイトを対象とする）ため、単純な append でよく重複排除は不要。
    all := append(pass1Results, pass2Results...)
    if len(all) == 0 {
        return nil, nil  // Pass 1/Pass 2 としては解析済み・結果なし（最終保存は libSystem 結果とのマージ後に決定）
    }

    return &fileanalysis.SyscallAnalysisResult{
        SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
            Architecture:     "arm64",
            DetectedSyscalls: all,
        },
    }, nil
}
```

戻り値 `nil, nil` の意味: Pass 1/Pass 2 解析としては記録すべき syscall が存在しない（ELF パスと同様）。最終的な `record.SyscallAnalysis` は既存の libSystem import 解析結果とのマージ後に決定される。

## 8. `internal/filevalidator/validator.go` 変更

### 8.1 `analyzeMachoSyscalls` の差し替え

現在の `analyzeMachoSyscalls` は以下を行っている：
1. `machoanalyzer.ScanSVCAddrs` → `svc #0x80` アドレス取得
2. `buildSVCInfos(addrs)` → `Number=-1, DeterminationMethod="direct_svc_0x80"` エントリ生成
3. `analyzeLibSystem` → libSystem import 解析
4. `buildMachoSyscallData(svcEntries, libsysEntries, arch)` → マージして保存

変更後：

```go
func (v *Validator) analyzeMachoSyscalls(record *fileanalysis.Record, filePath string) error {
    // Pass 1 + Pass 2 解析
    result, err := v.machoSyscallAnalyzer.AnalyzeFile(filePath, v.fileSystem)
    if err != nil {
        return fmt.Errorf("mach-o syscall number analysis failed: %w", err)
    }

    // libSystem import 解析（変更なし）
    libsysEntries, libsysArch, err := v.analyzeLibSystem(record, filePath)
    if err != nil {
        return fmt.Errorf("libSystem import analysis failed: %w", err)
    }

    // マージ・保存
    merged := mergeMachoSyscallResults(result, libsysEntries, libsysArch)
    if merged != nil {
        record.SyscallAnalysis = merged
    }
    return nil
}
```

`AnalyzeFile` は `MachoSyscallNumberAnalyzer` のファイルパスベースラッパーで、以下の手順で処理する：
1. ファイルを `macho.NewFatFile` または `macho.NewFile` で開く
2. Fat バイナリの場合、`fat.Arches` から `CpuArm64` スライスを取得
3. arm64 スライス（または単一アーキテクチャ Mach-O）に対して `Analyze(f)` を呼び出す
4. arm64 スライスが存在しない場合は `nil, nil` を返す

Fat バイナリの x86_64 等の他スライスは対象外（既存の `collectSVCAddresses` と同様）。

`buildSVCInfos` 関数と `DeterminationMethodDirectSVC0x80` を使う箇所を削除する。

### 8.2 `machoSyscallAnalyzer` フィールド追加

```go
type Validator struct {
    // 既存フィールド（略）
    machoSyscallAnalyzer MachoSyscallAnalyzerInterface  // 新規
}
```

```go
// MachoSyscallAnalyzerInterface abstracts the Mach-O syscall number analyzer
// for testability.
type MachoSyscallAnalyzerInterface interface {
    AnalyzeFile(filePath string, fs safefileio.FileSystem) (*fileanalysis.SyscallAnalysisResult, error)
}
```

テスト時には `MachoSyscallAnalyzerInterface` のモックを注入できる。

## 9. `internal/runner/security/network_analyzer.go` 変更

### 9.1 `syscallAnalysisHasSVCSignal` の扱い

この節の初版では `syscallAnalysisHasSVCSignal` 関数の削除を前提としていたが、後続タスク 0105 で方針が更新された。

- 0104 完了時点の意図: v16 レコードでは `DeterminationMethod == "direct_svc_0x80"` を高リスク判定に直接使わない
- 0105 以降の確定方針: `syscallAnalysisHasSVCSignal` 自体は保持し、`Number == -1` かつ `DeterminationMethod == "direct_svc_0x80"` の未解決 svc のみを高リスクシグナルとして扱う

したがって、本節の「削除」は superseded とし、実装時は 0105 の要件・設計を優先する。

### 9.2 `isNetworkViaBinaryAnalysis` の判定ロジック変更

変更前（v15 ロジック）：
```go
if syscallAnalysisHasSVCSignal(svcResult) {
    return true, true  // svc #0x80 存在だけで高リスク
}
if syscallAnalysisHasNetworkSignal(svcResult) {
    return true, false  // ネットワーク syscall あるが isHighRisk=false
}
```

変更後（0104 初版で想定していた v16 ロジック）：
```go
if syscallAnalysisHasNetworkSignal(svcResult) {
    return true, true  // ネットワーク syscall 検出 → 高リスク確定
}
// IsNetwork=true なし → SymbolAnalysis 判定に委譲
// AC-4「X16 不明のみ + SymbolAnalysis 根拠なし → false, false」は
// SymbolAnalysis 判定ブランチ（既存コード）が担当する。
```

`syscallAnalysisHasNetworkSignal` は `IsNetwork == true` のエントリのみを判定条件とし、`DeterminationMethod`（`direct_svc_0x80` など）を参照しない実装にする。

注記: 0105 では `syscallAnalysisHasSVCSignal` を保持したまま、未解決 svc のみを高リスクとする条件が追加された。ここでの network 判定変更自体は 0105 後も維持される。

戻り値を `true, true`（高リスク確定）に変更する。変更前は `true, false` を返していたが、v16 では Pass 1/Pass 2 で確認されたネットワーク syscall は常に `isHighRisk = true` とする（FR-3.3.1 要件変更）。

### 9.3 ログメッセージ更新

```go
// 変更前
slog.Warn("SyscallAnalysis cache indicates svc #0x80; treating as high risk", ...)
// 変更後（削除）

// 変更前
slog.Info("SyscallAnalysis cache indicates network syscall", ...)
// 変更後
slog.Info("SyscallAnalysis cache indicates network syscall (high risk)", ...)
```

## 10. テスト仕様

各テストケースに対応する受け入れ条件（AC）を記載する。

### 10.1 `ParseMachoPclntab` テスト

| テストケース | 検証内容 | 対応 AC |
|---|---|---|
| `__gopclntab` セクションなし | `ErrNoPclntab` が返ること | AC-2, AC-3 |
| 不正マジック | `ErrUnsupportedPclntabVersion` が返ること | ― |
| 正常な pclntab バイト列 | `syscall.Syscall` 等のエントリが含まれること | AC-2, AC-3 |
| `nil` pclntab | `ErrNoPclntab` が返ること | AC-2, AC-3 |
| stripped バイナリ（`__gopclntab` セクション削除済み） | `ErrNoPclntab` が返り、呼び出し側が Pass 1/Pass 2 スキップで継続すること（`Analyze` がエラーを返さないこと） | AC-2, AC-3 |

### 10.2 Pass 1 テスト（AC-1, AC-2 対応）

各テストは合成 arm64 機械語バイト列を入力として渡す。

主要命令の arm64 エンコーディング参考値（リトルエンディアン）：

| 命令 | hex（LE） | 備考 |
|---|---|---|
| `svc #0x80` | `01 10 00 D4` | macOS syscall 命令 |
| `MOVZ X16, #98` | `10 0C 80 D2` | `0xD2800C10`：imm16=98, hw=0 |
| `MOVZ X16, #3` | `70 00 80 D2` | `0xD2800070`：imm16=3, hw=0 |
| `MOVZ X16, #0x0200, LSL#16` | `10 04 A0 D2` | `0xD2A00410`：imm16=0x200, hw=1 |
| `MOVK X16, #0x0062` | `50 0C 80 F2` | `0xF2800C50`：imm16=0x62 |
| `ldr x16, [sp, #0x18]` | `F0 63 40 F9` | スタックロード |
| `BL <offset>` | 先頭バイト `94` or `97` 等 | `[31:26]=100101` |

| テストケース | 入力 | 期待出力 | AC |
|---|---|---|---|
| `MOVZ X16, #98` + `svc #0x80` | `0xD2800C10` + `0xD4001001` | `Number=98, IsNetwork=true, Method="immediate"` | AC-1 |
| `MOVZ X16, #3` + `svc #0x80` | `0xD2800070` + `0xD4001001` | `Number=3, IsNetwork=false, Method="immediate"` | AC-1 |
| BSD prefix 付き 32bit（`MOVZ X16, #0x0200, LSL#16` + `MOVK X16, #0x0062`） | `0xD2A00410` + `0xF2800C50` + `0xD4001001` | `Number=98（0x2000062 - 0x2000000）, IsNetwork=true` | AC-1 |
| `ldr x16, [sp, #0x18]` + `svc #0x80` | `0xF963C0F9`（適切なオフセット値）+ `0xD4001001` | `Number=-1, IsNetwork=false, Method="unknown:indirect_setting"` | AC-1 |
| 制御フロー命令（`BL`）を挟んだ `svc #0x80` | `BL xxx`（`[31:26]=100101`）+ `MOVZ X16, #98` + `svc` | 後方スキャンが `BL` で停止 → `Number=-1` | AC-1 |
| `svc #0x80` が既知スタブアドレス範囲内 | stubRanges に含む | 結果スライスにエントリなし（除外） | AC-2 |
| `svc #0x80` がスタブ範囲外 | stubRanges に含まない | エントリあり | AC-2 |

### 10.3 Pass 2 テスト（AC-3 対応）

主要命令の arm64 エンコーディング参考値（リトルエンディアン）：

| 命令 | hex（LE） | 備考 |
|---|---|---|
| `MOVZ X0, #98` | `00 0C 80 D2` | `0xD2800C00`：imm16=98, Rd=X0 |
| `STP X0, X1, [SP, #8]` | `E0 07 00 A9` | `0xA9000FE0`（実際のオフセット/レジスタに応じて変わる）|
| `STR X0, [SP, #8]` | `E0 07 00 F9` | `0xF90007E0`：imm12=1（×8=8）, Rn=SP |
| `BL <target>` | `[31:26]=100101` | 上位6bit が `0x25` (big-endian) = `0x94` (LE 先頭バイト) |

| テストケース | 入力 | 期待出力 | AC |
|---|---|---|---|
| `MOV xN, #98` + `STP xN, ..., [SP, #8]` + `BL syscall.Syscall` | `MOVZ X0, #98`（`0xD2800C00`）+ STP + BL（ターゲット=wrapperAddr）| `Number=98, IsNetwork=true, Method="go_wrapper"` | AC-3 |
| `MOV xN, #3` + `STR xN, [SP, #8]` + `BL syscall.Syscall6` | `MOVZ X0, #3`（`0xD2800060`）+ STR + BL | `Number=3, IsNetwork=false, Method="go_wrapper"` | AC-3 |
| `MOV xN, #49` + `STP xN, ..., [SP, #8]` + `BL syscall.RawSyscall` | `MOVZ X0, #49`（`0xD2800620`）+ STP + BL | `Number=49 (munmap), IsNetwork=false, Method="go_wrapper"` | AC-3 |
| `[SP, #8]` への書き込みが間接ロード由来 | `LDR X0, [X1]` + STP + `BL syscall.RawSyscall6` | `Number=-1, Method="unknown:indirect_setting"` | AC-3 |
| `.gopclntab` なし（noPclntab=true） | wrapperAddrs 空 | Pass 2 結果なし（スキップ） | AC-3 |
| ラッパー関数内からの BL（IsInsideWrapper） | callSite ∈ wrapperRanges | 結果スライスに含まれない | ― |
| 制御フロー境界 | `BL other`（`[31:26]=100101`）+ `MOVZ X0, #98` + `BL syscall.Syscall` | 後方スキャンが BL で停止 → `Number=-1` | AC-3 |

### 10.4 リスク判定テスト（AC-4 対応）

`network_analyzer.go` の変更をテストする。既存の `network_analyzer_test.go` の `SyscallAnalysis` 関連テストを更新する。

| テストケース | SyscallAnalysis 状態 | 期待戻り値 | AC |
|---|---|---|---|
| `IsNetwork=true` エントリあり | DetectedSyscalls に IsNetwork=true | `true, true` | AC-4 |
| `IsNetwork=false` のみ | IsNetwork=true なし | `SymbolAnalysis` 判定に委譲 | AC-4 |
| `SyscallAnalysis = nil`（nil, nil） | nil | `SymbolAnalysis` 判定に委譲 | AC-4 |
| v15 レコード（SchemaVersionMismatch） | スキーマ不一致 | `true, true` | AC-4 |

最後のケース（v15 レコード）は `SchemaVersionMismatchError` のケースであり、既存の `errors.As(err, &svcSchemaMismatch)` ブランチで処理される。

### 10.5 スキーマ / AC-5 テスト

| テストケース | 検証内容 | AC |
|---|---|---|
| `CurrentSchemaVersion == 16` | 定数値の確認 | AC-5 |
| v15 レコード Load | `SchemaVersionMismatchError` が返ること | AC-5 |
| Pass 1 結果の `Source` フィールド | 空文字列 `""` であること | AC-5 |
| Pass 2 結果の `Source` フィールド | 空文字列 `""` であること | AC-5 |
| ELF パス（SourceLibcSymbolImport）の `Source` | `"libc_symbol_import"` のまま変更なし | AC-5, AC-6 |

### 10.6 統合テスト（AC-4, AC-6 対応）

`record` コマンド自身（`build/prod/record`）を解析する統合テスト。

| テストケース | 検証内容 | AC |
|---|---|---|
| `record` バイナリ → `runner` 判定 | `true, true` を返さないこと（偽陽性なし） | AC-4 |
| `record` バイナリの SyscallAnalysis | `IsNetwork=true` エントリが含まれないこと | ― |
| ELF バイナリの既存テスト | すべてパスすること | AC-6 |

## 11. 定数の扱い

`common.DeterminationMethodDirectSVC0x80 = "direct_svc_0x80"` は後方互換性のために定数として**残す**が、v16 以降のコードで新規使用しない。

v16 で使用する `DeterminationMethod` 定数は `elfanalyzer` パッケージ定義のものを参照するか、同一値を `machoanalyzer` パッケージに別途定義する。

```go
// machoanalyzer/syscall_number_analyzer.go
// DeterminationMethod constants for Mach-O syscall analysis.
// Values match the corresponding elfanalyzer constants for cross-architecture consistency.
const (
    determinationMethodImmediate          = "immediate"           // elfanalyzer.DeterminationMethodImmediate
    determinationMethodGoWrapper          = "go_wrapper"          // elfanalyzer.DeterminationMethodGoWrapper
    determinationMethodUnknownIndirect    = "unknown:indirect_setting" // elfanalyzer.DeterminationMethodUnknownIndirectSetting
)
```

## 12. 非機能要件

- **パフォーマンス**: コードセクション全体をO(N)でスキャン。pclntab はメモリマップ不要（`section.Data()` で全バイト取得）。
- **エラーハンドリング**: `analyzeMachoSyscalls` 内でエラーが発生した場合は `Validator.Record` がエラーを返し、`record` コマンドが処理を中断する。
- **Fat バイナリ**: arm64 スライスのみを `Analyze` に渡す。他アーキテクチャのスライスは対象外（現行動作と同じ）。
