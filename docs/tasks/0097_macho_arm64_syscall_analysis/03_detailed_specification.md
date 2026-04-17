# Mach-O arm64 svc #0x80 キャッシュ統合・CGO フォールバック 詳細仕様書

## 1. 概要

本ドキュメントはアーキテクチャ設計書（`02_architecture.md`）を基に、各変更ファイルの
具体的な実装仕様を定義する。受け入れ条件（AC-1〜AC-4）と各テストケースの対応関係も示す。

## 2. 変更ファイル一覧

| ファイル | 変更種別 |
|---------|---------|
| `internal/runner/security/machoanalyzer/svc_scanner.go` | 拡張 |
| `internal/runner/security/machoanalyzer/svc_scanner_test.go` | 新規（テスト追加） |
| `internal/filevalidator/validator.go` | 拡張 |
| `internal/filevalidator/validator_macho_test.go` | 新規または拡張（Mach-O 向けテスト追加） |
| `internal/runner/security/network_analyzer.go` | 拡張 |
| `internal/runner/security/network_analyzer_test.go` | 拡張（テスト追加） |
| `internal/runner/risk/evaluator.go` | 拡張 |
| `internal/runner/resource/normal_manager.go` | 拡張 |
| `internal/runner/resource/default_manager.go` | 拡張 |
| `internal/runner/runner.go` | 拡張 |
| `internal/verification/manager.go` | 拡張 |

## 3. `internal/runner/security/machoanalyzer/svc_scanner.go`

### 3.1 追加関数: `collectSVCAddresses`

**シグネチャ**:
```go
func collectSVCAddresses(f *macho.File) ([]uint64, error)
```

**仕様**:
- `f.Cpu != macho.CpuArm64` の場合は即 `nil, nil` を返す
- `f.Section("__text")` が nil または `Seg != "__TEXT"` の場合は `nil, nil` を返す
- セクションデータ読み出しエラー時は `nil, fmt.Errorf("failed to read __TEXT,__text section: %w", err)` を返す
- データを 4 バイトアラインで走査し、`svcInstruction` エンコード（`0xD4001001`）にマッチする
  命令を収集する
- マッチした命令の仮想アドレスは `section.Addr + uint64(i)` で算出する
- 返り値: 検出した仮想アドレスのスライス（検出なしの場合は `nil, nil`）

**`containsSVCInstruction` のリファクタリング**:
- `containsSVCInstruction` は `collectSVCAddresses` を呼び出す形に変更し、
  `len(addrs) > 0` を返すよう実装を委譲する（DRY）
- 関数シグネチャ・呼び出し元への影響なし

### 3.2 追加関数: `CollectSVCAddressesFromFile`（エクスポート）

**シグネチャ**:
```go
func CollectSVCAddressesFromFile(filePath string, fs safefileio.FileSystem) ([]uint64, error)
```

**依存パッケージ追加（`svc_scanner.go` に追加）**:
- `github.com/isseis/go-safe-cmd-runner/internal/safefileio`
- `"os"` （`os.O_RDONLY` を使用）

**仕様（処理フロー）**:

1. `fs.SafeOpenFile(filePath, os.O_RDONLY, 0)` でファイルを開く
   - エラー時はそのままラップして返す
2. 先頭 4 バイトを読み込み Mach-O マジックを確認する
   - マジック判定: `macho.Magic32 (0xFEEDFACE)`, `macho.Magic64 (0xFEEDFACF)`,
     `macho.MagicFat (0xCAFEBABE)`, バイトスワップ版 (`0xCEFAEDFE`, `0xCFFAEDFE`, `0xBEBAFECA`)
   - 非 Mach-O（マジック不一致）の場合は `nil, nil` を返す
3. Fat バイナリの場合 (`MagicFat`):
   - `macho.NewFatFile(f)` でパースする（`safefileio.File` は `io.ReaderAt` を実装するため
     シーク不要。先頭 4 バイト読み出し後もそのまま渡せる）
   - 各 `FatArch` スライスを順次確認する
   - `fa.Cpu == macho.CpuArm64` のスライスに対してのみ `collectSVCAddresses(&fa.File)` を呼ぶ
   - 各スライスの結果を `append` で連結して返す
   - `macho.NewFatFile` でパースエラーの場合はエラーを返す
4. 単一アーキテクチャ Mach-O の場合:
   - `macho.NewFile(f)` でパースする（同上、シーク不要）
   - `collectSVCAddresses` を呼ぶ
   - `macho.NewFile` でパースエラーの場合はエラーを返す

**ファイルアクセス戦略**:
- `macho.NewFile` / `macho.NewFatFile` はいずれも `io.ReaderAt` を受け取る。
  `safefileio.File` はこのインターフェースを実装しているため、先頭 4 バイト読み出し後に
  シークする必要はなく、ファイルをそのまま渡せる

### 3.3 テスト仕様 (`svc_scanner_test.go` 追加分)

| テスト名 | 検証内容 | AC |
|---------|---------|-----|
| `TestCollectSVCAddresses_Arm64WithSVC` | arm64 Mach-O で svc #0x80 を検出しアドレスを返す | AC-1 |
| `TestCollectSVCAddresses_Arm64NoSVC` | arm64 Mach-O で svc #0x80 なし → `nil, nil` | AC-1 |
| `TestCollectSVCAddresses_NonArm64` | x86_64 Mach-O → `nil, nil` | AC-4 |
| `TestCollectSVCAddresses_MultipleSVC` | 複数 svc #0x80 → 全アドレスを返す | AC-1 |
| `TestCollectSVCAddressesFromFile_NotMacho` | ELF ファイル → `nil, nil` | AC-4 |
| `TestCollectSVCAddressesFromFile_FatBinary` | Fat バイナリ（arm64 含む）→ arm64 スライスのみ走査 | AC-1 |
| `TestContainsSVCInstruction_DelegatesToCollect` | `containsSVCInstruction` が正常に委譲する | AC-4 |

## 4. `internal/filevalidator/validator.go`

### 4.1 インポート追加

**`internal/filevalidator/validator.go`** に追加:
```go
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/machoanalyzer"
```

**インポートサイクルの確認**: `machoanalyzer` パッケージは現在
`internal/runner/security/machoanalyzer` に存在し、`filevalidator` を import していない。
逆方向の import は可能であることを事前に確認すること。

**`internal/runner/security/machoanalyzer/svc_scanner.go`** に追加:
```go
"os"

"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
```

### 4.2 `updateAnalysisRecord` の変更

`store.Update()` コールバック内、`analyzeSyscalls()` 呼び出しの**直後**に
以下のコードを追加する。

**変更の前提**: `updateAnalysisRecord` コールバック内の `if v.binaryAnalyzer != nil` ブロックに
ローカル変数 `networkResult binaryanalyzer.AnalysisResult` を追加し、
`output.Result` の値をブロック外でも参照できるようにする。

**追加箇所 1**: `if v.binaryAnalyzer != nil` ブロック前（またはブロック内先頭）に
ローカル変数宣言を追加する:

```go
var networkResult binaryanalyzer.AnalysisResult
if v.binaryAnalyzer != nil {
    output := v.binaryAnalyzer.AnalyzeNetworkSymbols(filePath.String(), contentHash)
    networkResult = output.Result
    // ... 既存の switch output.Result ブロック ...
}
```

**追加箇所 2**: `v.analyzeSyscalls(record, filePath.String())` の呼び出し行直後、
shebang 設定の前:

```go
// Mach-O arm64 svc #0x80 scan.
// Run after analyzeSyscalls() to overwrite the nil it sets for non-ELF files.
// Only runs when binaryanalyzer returned NoNetworkSymbols for this file.
// CollectSVCAddressesFromFile checks magic bytes and returns nil for non-Mach-O files,
// so this is safe to call on all platforms and binary formats.
if networkResult == binaryanalyzer.NoNetworkSymbols {
    addrs, svcErr := machoanalyzer.CollectSVCAddressesFromFile(filePath.String(), v.fileSystem)
    if svcErr != nil {
        return fmt.Errorf("mach-o svc scan failed: %w", svcErr)
    }
    if len(addrs) > 0 {
        record.SyscallAnalysis = buildSVCSyscallAnalysis(addrs)
    }
}
```

**`networkResult == binaryanalyzer.NoNetworkSymbols` 判定の根拠**:
- `output.Result` の値を直接変数に保持するため、フィールド値推測より正確
- `v.binaryAnalyzer == nil` の場合は `networkResult` がゼロ値（`NetworkDetected`(0)）となり
  条件は `false` になるため、svc スキャンは実行されない（意図通り）
- `NetworkDetected` / `StaticBinary` / `AnalysisError` の場合は条件が `false` になる
- `CollectSVCAddressesFromFile` は内部でマジックバイトを確認し、非 Mach-O ファイルには
  `nil, nil` を返すため、OS を問わず安全に呼び出せる

**`SymbolAnalysis` が nil（`StaticBinary` / `NotSupportedBinary` / `binaryAnalyzer == nil`）の
場合は `CollectSVCAddressesFromFile` を呼ばない**（`networkResult != NoNetworkSymbols` のため）。

**FR-3.2.2 との整合性について**:
要件定義 FR-3.2.2 は「NetworkDetected 時も svc スキャンを継続すべき」と記述しているが、
アーキテクチャ設計書 §3.2.1 はこの不整合を解消しており、受け入れ条件 AC-1 に従い
**NetworkDetected 時は svc スキャンをスキップする**と決定している。
`runner` は `SymbolAnalysis = NoNetworkSymbols` の場合のみ `SyscallAnalysis` を参照するため、
NetworkDetected バイナリに svc 結果を保存しても実用上の追加セキュリティ効果はない。
本仕様書はこのアーキテクチャ決定を実装に反映している。

### 4.3 追加関数: `buildSVCSyscallAnalysis`

**シグネチャ**:
```go
func buildSVCSyscallAnalysis(addrs []uint64) *fileanalysis.SyscallAnalysisData
```

**仕様**:
- `addrs` は svc #0x80 が検出された仮想アドレスのスライス（len > 0 を前提）
- 返り値: 以下のフィールドを持つ `*fileanalysis.SyscallAnalysisData`

```go
&fileanalysis.SyscallAnalysisData{
    SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
        Architecture: "arm64",
        AnalysisWarnings: []string{
            "svc #0x80 detected: direct syscall bypassing libSystem.dylib",
        },
        DetectedSyscalls: syscalls, // 以下参照
    },
}
```

`DetectedSyscalls` の各エントリ（`addrs[i]` に対して）:
```go
common.SyscallInfo{
    Number:              -1,
    IsNetwork:           false,
    Location:            addrs[i],
    DeterminationMethod: "direct_svc_0x80",
    Source:              "direct_svc_0x80",
}
```

**注意**: `ArgEvalResults` は設定しない（`nil` のまま）。

### 4.4 テスト仕様 (`validator_macho_test.go` 追加分)

| テスト名 | 検証内容 | AC |
|---------|---------|-----|
| `TestUpdateAnalysisRecord_MachoSVCDetected` | svc ありの Mach-O: SyscallAnalysis が設定される | AC-1, AC-2 |
| `TestUpdateAnalysisRecord_MachoNoSVC` | svc なしの Mach-O: SyscallAnalysis が nil | AC-1, AC-2 |
| `TestUpdateAnalysisRecord_MachoNetworkDetected_NoSVC` | NetworkDetected Mach-O: SyscallAnalysis が保存されない | AC-1 |
| `TestUpdateAnalysisRecord_ELFNotAffected` | ELF バイナリ: Mach-O パスが呼ばれない | AC-4 |
| `TestBuildSVCSyscallAnalysis` | 単体: 正しいフィールド値が設定される | AC-1 |

**テスト実装上の注意**:
- `debug/macho` と `CollectSVCAddressesFromFile` はクロスプラットフォームでビルド可能なため、
    darwin 専用ビルドタグは不要。
- 既存の `validator_test.go` に追加してもよいが、Mach-O 専用ケースを分離するため
    `validator_macho_test.go` のような専用テストファイルを推奨する。

## 5. `internal/runner/security/network_analyzer.go`

### 5.1 `NetworkAnalyzer` 構造体への `syscallStore` 追加

```go
type NetworkAnalyzer struct {
    binaryAnalyzer binaryanalyzer.BinaryAnalyzer
    store          fileanalysis.NetworkSymbolStore   // 既存: nil means cache disabled
    syscallStore   fileanalysis.SyscallAnalysisStore // 新規: nil means svc cache disabled
}
```

### 5.2 追加コンストラクタ: `NewNetworkAnalyzerWithStores`

```go
// NewNetworkAnalyzerWithStores creates a NetworkAnalyzer with both
// symbol and syscall stores for cache-based analysis.
// If either store is nil, the corresponding cache lookup is disabled.
func NewNetworkAnalyzerWithStores(
    symStore fileanalysis.NetworkSymbolStore,
    svcStore fileanalysis.SyscallAnalysisStore,
) *NetworkAnalyzer {
    return &NetworkAnalyzer{
        binaryAnalyzer: NewBinaryAnalyzer(),
        store:          symStore,
        syscallStore:   svcStore,
    }
}
```

**既存コンストラクタとの互換性**: `NewNetworkAnalyzerWithStore` は変更しない
（`syscallStore = nil` のまま）。

### 5.3 `isNetworkViaBinaryAnalysis` の変更

**変更箇所**: `case err == nil:` ブランチ内の `else` ブロック（`output.Result = binaryanalyzer.NoNetworkSymbols` を設定する箇所）を以下のように変更する。

```go
} else {
    output.Result = binaryanalyzer.NoNetworkSymbols
    // Check SyscallAnalysis cache for svc #0x80 signal (Mach-O arm64).
    if a.syscallStore != nil {
        svcResult, svcErr := a.syscallStore.LoadSyscallAnalysis(cmdPath, contentHash)
        var svcSchemaMismatch *fileanalysis.SchemaVersionMismatchError
        switch {
        case svcErr == nil:
            if syscallAnalysisHasSVCSignal(svcResult) {
                slog.Warn("SyscallAnalysis cache indicates svc #0x80; treating as high risk",
                    "path", cmdPath)
                return true, true
            }
            // No svc signal: treat as NoNetworkSymbols (fall through to handleAnalysisOutput).
        case errors.Is(svcErr, fileanalysis.ErrHashMismatch):
            slog.Warn("SyscallAnalysis cache hash mismatch; treating as high risk",
                "path", cmdPath)
            return true, true
        case errors.Is(svcErr, fileanalysis.ErrRecordNotFound) ||
            errors.Is(svcErr, fileanalysis.ErrNoSyscallAnalysis):
            // Cache miss: fall through to handleAnalysisOutput (which returns false, false).
        case errors.As(svcErr, &svcSchemaMismatch):
            slog.Warn("SyscallAnalysis cache has outdated schema; ignoring svc cache",
                "path", cmdPath,
                "expected_schema", svcSchemaMismatch.Expected,
                "actual_schema", svcSchemaMismatch.Actual)
            // Fall through to handleAnalysisOutput.
        default:
            slog.Warn("unexpected error loading SyscallAnalysis cache; ignoring svc cache",
                "path", cmdPath,
                "error", svcErr)
            // Fall through to handleAnalysisOutput.
        }
    }
}
return handleAnalysisOutput(output, cmdPath)
```

**`return handleAnalysisOutput(output, cmdPath)` 呼び出しについて**:
svc キャッシュが `AnalysisError` を返さなかった場合、`else` ブロックを抜けた後に
`return handleAnalysisOutput(output, cmdPath)` が実行される（既存コードを維持）。
`handleAnalysisOutput` は `NoNetworkSymbols` に対して `false, isHighRisk` を返す。

### 5.4 追加関数: `syscallAnalysisHasSVCSignal`

```go
// syscallAnalysisHasSVCSignal reports whether the given SyscallAnalysisResult
// contains evidence of svc #0x80 direct syscall usage.
// Returns true only when any DetectedSyscall has DeterminationMethod == "direct_svc_0x80".
// AnalysisWarnings is not checked here because it may contain warnings from ELF syscall
// analysis that are unrelated to svc #0x80, which would cause false positives.
func syscallAnalysisHasSVCSignal(result *fileanalysis.SyscallAnalysisResult) bool {
    if result == nil {
        return false
    }
    for _, s := range result.DetectedSyscalls {
        if s.DeterminationMethod == "direct_svc_0x80" {
            return true
        }
    }
    return false
}
```

### 5.5 呼び出し元の変更

`NewNetworkAnalyzerWithStores` の追加だけでは不十分であり、`SyscallAnalysisStore` を
`runner` の通常実行パスまで運ぶ注入チェーン全体を更新する必要がある。

対象となる本番コード:

1. `internal/verification/manager.go`
    - `GetSyscallAnalysisStore() fileanalysis.SyscallAnalysisStore` を追加する
2. `internal/runner/runner.go`
    - path resolver から `GetNetworkSymbolStore()` と `GetSyscallAnalysisStore()` を取得する
    - `resource.NewDefaultResourceManager()` 呼び出しに両ストアを渡す
3. `internal/runner/resource/default_manager.go`
    - `NewDefaultResourceManager()` に `SyscallAnalysisStore` 引数を追加する
4. `internal/runner/resource/normal_manager.go`
    - `NewNormalResourceManagerWithOutput()` に `SyscallAnalysisStore` 引数を追加する
5. `internal/runner/risk/evaluator.go`
    - `NewStandardEvaluator()` に `SyscallAnalysisStore` 引数を追加し、
      `security.NewNetworkAnalyzerWithStores()` を使用する

関連テストも合わせて更新すること。

### 5.6 テスト仕様 (`network_analyzer_test.go` 追加分)

| テスト名 | 検証内容 | AC |
|---------|---------|-----|
| `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheHit` | NoNetworkSymbols + svc キャッシュあり → AnalysisError | AC-3 |
| `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheNil` | NoNetworkSymbols + SyscallAnalysis nil → 通過 | AC-3 |
| `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCHashMismatch` | ErrHashMismatch → AnalysisError | AC-3 |
| `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCNoCache` | ErrNoSyscallAnalysis → 通過 | AC-3 |
| `TestIsNetworkViaBinaryAnalysis_NetworkDetected_Unchanged` | NetworkDetected はそのまま通過 | AC-4 |
| `TestSyscallAnalysisHasSVCSignal_WithWarningsOnly` | AnalysisWarnings のみ（DeterminationMethod なし）→ false | AC-3 |
| `TestSyscallAnalysisHasSVCSignal_WithDeterminationMethod` | DeterminationMethod == "direct_svc_0x80" → true | AC-3 |
| `TestSyscallAnalysisHasSVCSignal_Empty` | 空の SyscallAnalysisResult → false | AC-3 |
| `TestSyscallAnalysisHasSVCSignal_Nil` | nil → false | AC-3 |

## 6. 受け入れ条件とテストの対応関係

| AC | テスト | ファイル |
|----|-------|---------|
| AC-1: svc あり Mach-O に SyscallAnalysis が保存される | `TestUpdateAnalysisRecord_MachoSVCDetected` | `validator_macho_test.go` |
| AC-1: Architecture が "arm64" | `TestBuildSVCSyscallAnalysis` | `validator_macho_test.go` |
| AC-1: AnalysisWarnings に検出メッセージ | `TestBuildSVCSyscallAnalysis` | `validator_macho_test.go` |
| AC-1: DetectedSyscalls に Number=-1, DeterminationMethod="direct_svc_0x80" | `TestBuildSVCSyscallAnalysis` | `validator_macho_test.go` |
| AC-1: svc なし Mach-O は SyscallAnalysis が nil | `TestUpdateAnalysisRecord_MachoNoSVC` | `validator_macho_test.go` |
| AC-1: NetworkDetected → SyscallAnalysis 保存なし | `TestUpdateAnalysisRecord_MachoNetworkDetected_NoSVC` | `validator_macho_test.go` |
| AC-2: NoNetworkSymbols + svc あり → SyscallAnalysis 保存 | `TestUpdateAnalysisRecord_MachoSVCDetected` | `validator_macho_test.go` |
| AC-2: NoNetworkSymbols + svc なし → SyscallAnalysis nil | `TestUpdateAnalysisRecord_MachoNoSVC` | `validator_macho_test.go` |
| AC-3: NoNetworkSymbols + SyscallAnalysis に svc → AnalysisError | `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheHit` | `network_analyzer_test.go` |
| AC-3: NoNetworkSymbols + SyscallAnalysis nil → 通過 | `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheNil` | `network_analyzer_test.go` |
| AC-3: ErrHashMismatch → AnalysisError | `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCHashMismatch` | `network_analyzer_test.go` |
| AC-4: ELF バイナリのフロー変更なし | `TestUpdateAnalysisRecord_ELFNotAffected` | `validator_macho_test.go` |
| AC-4: NetworkDetected Mach-O の判定変更なし | `TestIsNetworkViaBinaryAnalysis_NetworkDetected_Unchanged` | `network_analyzer_test.go` |

## 7. エッジケースと注意事項

### 7.1 `safefileio.FileSystem.SafeOpenFile` の返り値型

`safefileio.FileSystem.SafeOpenFile` は `safefileio.File` インターフェースを返す。
`safefileio.File` は `io.Reader`, `io.Writer`, `io.Seeker`, `io.ReaderAt` を実装するため、
`macho.NewFile`（`io.ReaderAt` を要求）および `macho.NewFatFile`（`io.ReaderAt` を要求）に
直接渡せる。型アサーションは不要。

### 7.2 `machoanalyzer` → `filevalidator` インポートサイクル

`machoanalyzer` パッケージが現在 `filevalidator` を import していないことを
`grep -r "filevalidator" internal/runner/security/machoanalyzer/` で確認する。
循環が発生する場合は `CollectSVCAddressesFromFile` を別パッケージ（例: `internal/machoscan`）に配置する。

**`machoanalyzer` → `safefileio` インポートについて**:
`CollectSVCAddressesFromFile` が `safefileio.FileSystem` をパラメータとして受け取るため、
`svc_scanner.go` に `safefileio` パッケージのインポートが必要。既存の `machoanalyzer` は
`safefileio` を import していない可能性があるため、インポート追加を事前確認すること。

### 7.3 darwin ビルドタグ

`CollectSVCAddressesFromFile` は Mach-O マジックを自前で判定し、非 Mach-O には `nil, nil` を
返すため、呼び出し元で `runtime.GOOS == "darwin"` に制限する必要はない。
`debug/macho` も全 OS でビルド可能なため、`svc_scanner.go` / `validator.go` / 関連テストに
darwin 専用ビルドタグは不要。

### 7.4 `FilterSyscallsForStorage` との整合性

既存の `FilterSyscallsForStorage` は `IsNetwork == true` または `Number == -1` を
フィルタリング条件とする。Mach-O svc スキャン結果は `Number = -1` であるため、
フィルタを通過する（意図通り）。

### 7.5 `SyscallAnalysis` の上書きタイミング

`analyzeSyscalls()` は非 ELF ファイルに対して `record.SyscallAnalysis = nil` を設定する。
Mach-O svc スキャンはその直後に実行し、検出時のみ上書きする。
これにより「ELF でも非 Mach-O でも nil」「Mach-O + svc ありのみ非 nil」の挙動が成立する。

## 8. 依存関係の検証チェックリスト

実装前に以下を確認する:

- [ ] `internal/runner/security/machoanalyzer` が `internal/filevalidator` を import していないこと
- [ ] `safefileio.FileSystem.SafeOpenFile` の返り値型が `macho.NewFile` に渡せること
- [ ] `GetNetworkSymbolStore()` に加えて `GetSyscallAnalysisStore()` の注入チェーンを追加すべき箇所を特定すること
- [ ] `fileanalysis.SyscallAnalysisData` が `common.SyscallAnalysisResultCore` を embed していること（スキーマ確認）
