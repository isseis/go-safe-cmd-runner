# 詳細仕様書: record コマンドのシステムコールフィルタリング削除

## 1. 概要

本ドキュメントはアーキテクチャ設計書（`02_architecture.md`）を基に、各変更ファイルの具体的な実装仕様を定義する。受け入れ基準（AC-1〜AC-8）と各テストケースの対応関係も示す。

## 2. 変更ファイル一覧

| ファイル | 変更種別 |
|---------|---------|
| `internal/fileanalysis/syscall_store.go` | 削除（関数） |
| `internal/fileanalysis/syscall_store_test.go` | テスト削除・置換 |
| `internal/filevalidator/validator.go` | 修正 |
| `internal/filevalidator/validator_test.go` | テスト更新 |
| `internal/filevalidator/validator_macho_test.go` | テスト更新 |
| `internal/runner/security/network_analyzer.go` | 修正 |
| `internal/runner/security/network_analyzer_test.go` | テスト更新 |
| `internal/libccache/macos_syscall_table.go` | 修正（マップ定義削除） |
| `internal/libccache/macos_syscall_numbers.go` | 新規（自動生成ファイル） |
| `scripts/generate_syscall_table.py` | 拡張 |
| `Makefile` | 更新 |

## 3. `internal/fileanalysis/syscall_store.go`

### 3.1 変更内容: `FilterSyscallsForStorage` 関数の削除

**削除対象:**

```go
// FilterSyscallsForStorage filters a slice of SyscallInfo to only entries
// relevant to risk assessment:
//   - Network-related syscalls (IsNetwork == true)
//   - Syscalls with unknown numbers (Number == -1)
func FilterSyscallsForStorage(syscalls []common.SyscallInfo) []common.SyscallInfo {
    filtered := make([]common.SyscallInfo, 0, len(syscalls))
    for _, s := range syscalls {
        if s.IsNetwork || s.Number == -1 {
            filtered = append(filtered, s)
        }
    }
    return filtered
}
```

**対応 AC:** AC-1（`FilterSyscallsForStorage` が削除されていること）

## 4. `internal/fileanalysis/syscall_store_test.go`

### 4.1 変更内容: `FilterSyscallsForStorage` 前提テストの削除

削除対象テスト:
- `TestFilterSyscallsForStorage`
- `TestFilterSyscallsForStorage_Empty`

**対応 AC:** AC-7

## 5. `internal/filevalidator/validator.go`

### 5.1 変更内容: `buildSyscallData`（ELF）のフィルタリング削除

**対象箇所:** `buildSyscallData` 関数

**変更前:**
```go
func buildSyscallData(all []common.SyscallInfo, argEvalResults []common.SyscallArgEvalResult, machine elf.Machine) *fileanalysis.SyscallAnalysisData {
    retained := fileanalysis.FilterSyscallsForStorage(all)

    return &fileanalysis.SyscallAnalysisData{
        SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
            Architecture:     elfArchName(machine),
            DetectedSyscalls: retained,
            ArgEvalResults:   argEvalResults,
        },
    }
}
```

**変更後:**
```go
func buildSyscallData(all []common.SyscallInfo, argEvalResults []common.SyscallArgEvalResult, machine elf.Machine) *fileanalysis.SyscallAnalysisData {
    return &fileanalysis.SyscallAnalysisData{
        SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
            Architecture:     elfArchName(machine),
            DetectedSyscalls: all,
            ArgEvalResults:   argEvalResults,
        },
    }
}
```

**対応 AC:** AC-1

### 5.2 変更内容: `buildMachoSyscallData`（Mach-O）のフィルタリング削除と警告ロジック修正

**対象箇所:** `buildMachoSyscallData` 関数（関数コメントを含む）

**変更前（コメント含む）:**
```go
// buildMachoSyscallData merges svc and libSystem entries and constructs
// SyscallAnalysisData.
// AnalysisWarnings is populated only when unresolved svc #0x80 entries remain
// after filtering (i.e., entries with DeterminationMethod="direct_svc_0x80").
// When all svc entries are resolved to non-network syscalls they are dropped by
// FilterSyscallsForStorage and no warning is emitted.
// DetectedSyscalls is sorted by Number (svc entries with Number=-1 appear first).
func buildMachoSyscallData(
    svcEntries []common.SyscallInfo,
    libsysEntries []common.SyscallInfo,
    arch string,
) *fileanalysis.SyscallAnalysisData {
    merged := mergeMachoSyscallInfos(svcEntries, libsysEntries)
    retained := fileanalysis.FilterSyscallsForStorage(merged)

    var warnings []string
    for _, s := range retained {
        if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 {
            warnings = []string{"svc #0x80 detected: syscall number unresolved, direct kernel call bypassing libSystem.dylib"}
            break
        }
    }

    return &fileanalysis.SyscallAnalysisData{
        SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
            Architecture:     arch,
            AnalysisWarnings: warnings,
            DetectedSyscalls: retained,
        },
    }
}
```

**変更後（コメント含む）:**
```go
// buildMachoSyscallData merges svc and libSystem entries and constructs
// SyscallAnalysisData.
// AnalysisWarnings is populated only when unresolved svc #0x80 entries exist
// (i.e., entries with DeterminationMethod="direct_svc_0x80" AND Number == -1).
// When all svc entries are resolved (Number != -1), no warning is emitted.
// DetectedSyscalls contains all entries without filtering.
func buildMachoSyscallData(
    svcEntries []common.SyscallInfo,
    libsysEntries []common.SyscallInfo,
    arch string,
) *fileanalysis.SyscallAnalysisData {
    merged := mergeMachoSyscallInfos(svcEntries, libsysEntries)

    var warnings []string
    for _, s := range merged {
        if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 && s.Number == -1 {
            warnings = []string{"svc #0x80 detected: syscall number unresolved, direct kernel call bypassing libSystem.dylib"}
            break
        }
    }

    return &fileanalysis.SyscallAnalysisData{
        SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
            Architecture:     arch,
            AnalysisWarnings: warnings,
            DetectedSyscalls: merged,
        },
    }
}
```

**ポイント:**
- `FilterSyscallsForStorage` の呼び出しを削除し `retained` 変数を排除
- 警告判定を `retained` から `merged` に変更
- 警告条件に `s.Number == -1` を追加（解決済み svc では警告を発しない）
- `DetectedSyscalls` に `retained` ではなく `merged`（全エントリ）を格納

**対応 AC:** AC-2、AC-3

### 5.3 `fileanalysis` パッケージのインポート

`buildSyscallData` と `buildMachoSyscallData` の両方から `fileanalysis.FilterSyscallsForStorage` の呼び出しが削除される。もし `fileanalysis` パッケージのインポートが `FilterSyscallsForStorage` のためだけであれば削除を検討するが、`fileanalysis.SyscallAnalysisData` 等の型参照が残るため実際には削除不要。

## 6. `internal/filevalidator/validator_test.go`

### 6.1 変更内容: `buildSyscallData` テストの更新

**変更内容の方向性:**
- フィルタリング前提のアサーションを削除する
- 非ネットワーク・解決済み syscall（例: `write()`、`read()`）が `DetectedSyscalls` に含まれることを検証するテストケースを追加または更新する

**対応 AC:** AC-1、AC-7

## 7. `internal/filevalidator/validator_macho_test.go`

### 7.1 変更内容: `buildMachoSyscallData` テストの更新

**変更内容の方向性:**
- 解決済み非ネットワーク svc（例: `read()`、`Number=3`）が `DetectedSyscalls` に含まれることを検証するテストを追加または更新する
- 未解決 svc（`Number == -1`）が存在する場合のみ `AnalysisWarnings` に警告が含まれることを検証する
- 全エントリが解決済みの場合は `AnalysisWarnings` が空であることを検証する

**テストケース例:**

| テスト名 | 入力 | 期待される出力 |
|---------|------|--------------|
| 全エントリ解決済み | Number=97（socket, IsNetwork=true, DeterminationMethod="direct_svc_0x80"） | AnalysisWarnings=[]、DetectedSyscalls に socket エントリ含む |
| 未解決 svc あり | Number=-1（DeterminationMethod="direct_svc_0x80"） | AnalysisWarnings に警告含む |
| 混在 | Number=97 + Number=-1 | AnalysisWarnings に警告含む（未解決があるため）|
| 非ネットワーク解決済み svc | Number=3（read, IsNetwork=false, DeterminationMethod="direct_svc_0x80"） | AnalysisWarnings=[]、DetectedSyscalls に read エントリ含む |

**対応 AC:** AC-2、AC-3、AC-7

## 8. `internal/runner/security/network_analyzer.go`

### 8.1 変更内容: `syscallAnalysisHasSVCSignal` の修正

**対象箇所:** `syscallAnalysisHasSVCSignal` 関数（関数コメントを含む）

**変更前（コメント含む）:**
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
        if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 {
            return true
        }
    }
    return false
}
```

**変更後（コメント含む）:**
```go
// syscallAnalysisHasSVCSignal reports whether the given SyscallAnalysisResult
// contains evidence of unresolved svc #0x80 direct syscall usage (high risk).
// Returns true only when any DetectedSyscall has both
// DeterminationMethod == "direct_svc_0x80" AND Number == -1.
// Resolved svc entries (Number != -1) are not treated as high risk here;
// their network classification is handled by syscallAnalysisHasNetworkSignal.
func syscallAnalysisHasSVCSignal(result *fileanalysis.SyscallAnalysisResult) bool {
    if result == nil {
        return false
    }
    for _, s := range result.DetectedSyscalls {
        if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 && s.Number == -1 {
            return true
        }
    }
    return false
}
```

**変更理由:** フィルタリング削除後、解決済み svc（Number != -1）も `DetectedSyscalls` に含まれるため、`DeterminationMethod` のみの判定では解決済みの非リスク svc でも誤って高リスク判定してしまう。

**対応 AC:** AC-4

### 8.2 変更内容: `syscallAnalysisHasNetworkSignal` の修正

**対象箇所:** `syscallAnalysisHasNetworkSignal` 関数（関数コメントを含む）

**変更前（コメント含む）:**
```go
// syscallAnalysisHasNetworkSignal reports whether the given SyscallAnalysisResult
// contains any detected syscall classified as a network syscall (IsNetwork == true)
// that was not identified as a direct svc #0x80 instruction.
// Direct svc #0x80 entries are handled separately by syscallAnalysisHasSVCSignal
// (which escalates to high risk) and are therefore excluded here.
func syscallAnalysisHasNetworkSignal(result *fileanalysis.SyscallAnalysisResult) bool {
    if result == nil {
        return false
    }
    for _, s := range result.DetectedSyscalls {
        if s.IsNetwork && s.DeterminationMethod != common.DeterminationMethodDirectSVC0x80 {
            return true
        }
    }
    return false
}
```

**変更後（コメント含む）:**
```go
// syscallAnalysisHasNetworkSignal reports whether the given SyscallAnalysisResult
// contains any detected syscall classified as a network syscall (IsNetwork == true).
// This includes resolved svc entries (DeterminationMethod == "direct_svc_0x80" AND Number != -1)
// whose network classification is determined by the syscall table lookup.
func syscallAnalysisHasNetworkSignal(result *fileanalysis.SyscallAnalysisResult) bool {
    if result == nil {
        return false
    }
    for _, s := range result.DetectedSyscalls {
        if s.IsNetwork {
            return true
        }
    }
    return false
}
```

**変更理由:** フィルタリング削除後、解決済みネットワーク svc（`IsNetwork=true`、`DeterminationMethod="direct_svc_0x80"`、`Number != -1`）も `DetectedSyscalls` に含まれる。これらを `DeterminationMethod` で除外すると、ネットワーク syscall を見逃す。`DeterminationMethod` による除外条件を削除し、`IsNetwork` のみで判定する。

**対応 AC:** AC-5

### 8.3 変更しない: キャッシュエラー処理

`isNetworkViaBinaryAnalysis` のキャッシュエラー処理（`SchemaVersionMismatchError`・`ErrRecordNotFound`・`nil` 結果）は 0105 では変更しない。既存の挙動を明示的に維持する:

| エラー条件 | 挙動 | 理由 |
|-----------|------|------|
| `SchemaVersionMismatchError`（SymbolAnalysis または SyscallAnalysis） | `return true, true`（高リスク確定） | 古いスキーマのレコードは信頼できないため、許容するのではなく保守的に高リスクとする |
| `ErrRecordNotFound`（SyscallAnalysis のみ） | panic | SymbolAnalysis レコードが存在するのに SyscallAnalysis レコードが存在しない場合は一貫性バグであり、サイレントに許容してはならない |
| `nil` result（SyscallAnalysis） | SVC シグナルなしとして SymbolAnalysis 判定に委ねる | syscall 解析が存在しない静的バイナリ等では正常ケース |

**実装上の注意:** 0105 ではスキーマバージョンを変更しない（既存 v16 レコードの後方互換を維持）。そのため、フィルタリング前の旧レコードは `SchemaVersionMismatchError` を発生させず、より少ない `DetectedSyscalls` のまま利用される。これは意図的な設計であり（NFR-1）、`record` を再実行すれば最新の（フィルタなし）レコードへ自動的に上書きされる。

## 9. `internal/runner/security/network_analyzer_test.go`

### 9.1 変更内容: SVC / network signal テストの更新

**変更内容の方向性:**

加えて、旧レコード互換性（NFR-1）を確認するため、フィルタリング済み `DetectedSyscalls` のみを持つ入力に対しても判定結果が変わらないケースを残す。

`syscallAnalysisHasSVCSignal` テストケース:

| テスト名 | 入力 | 期待される出力 |
|---------|------|--------------|
| 未解決 svc | Number=-1, DeterminationMethod="direct_svc_0x80" | true（高リスク） |
| 解決済み非ネットワーク svc | Number=3, DeterminationMethod="direct_svc_0x80", IsNetwork=false | false（高リスクでない） |
| 解決済みネットワーク svc | Number=97, DeterminationMethod="direct_svc_0x80", IsNetwork=true | false（高リスクでない。network signal で検出） |
| nil | nil | false |

`syscallAnalysisHasNetworkSignal` テストケース:

| テスト名 | 入力 | 期待される出力 |
|---------|------|--------------|
| libSystem ネットワーク | IsNetwork=true, DeterminationMethod="libSystem" | true |
| 解決済みネットワーク svc | IsNetwork=true, DeterminationMethod="direct_svc_0x80", Number=97 | true |
| 非ネットワーク | IsNetwork=false | false |
| nil | nil | false |

**対応 AC/NFR:** AC-4、AC-5、AC-7、NFR-1

## 10. `internal/libccache/macos_syscall_table.go`

### 10.1 変更内容: `macOSSyscallEntries` マップ定義の削除

**削除対象:**
```go
var macOSSyscallEntries = map[int]macOSSyscallEntry{
    27:  {name: "recvmsg", isNetwork: true},
    // ... 17 エントリ
}
```

`macOSSyscallEntries` の定義は `internal/libccache/macos_syscall_numbers.go`（自動生成）に移動される。`MacOSSyscallTable` 構造体・`GetSyscallName`・`IsNetworkSyscall` メソッド・`networkSyscallWrapperNames` は変更なし。

**対応 AC:** AC-6

## 11. `internal/libccache/macos_syscall_numbers.go`（新規）

### 11.1 生成ファイルの構造

`scripts/generate_syscall_table.py --macos-header` により生成される。

**ファイルヘッダー:**
```go
// Code generated by scripts/generate_syscall_table.py; DO NOT EDIT.
// Source: syscall.h
// Regenerate: make generate-syscall-tables

package libccache
```

**生成される変数:**
```go
var macOSSyscallEntries = map[int]macOSSyscallEntry{
    1:   {name: "exit", isNetwork: false},
    2:   {name: "fork", isNetwork: false},
    3:   {name: "read", isNetwork: false},
    4:   {name: "write", isNetwork: false},
    // ... 全 BSD syscall
    27:  {name: "recvmsg", isNetwork: true},
    97:  {name: "socket", isNetwork: true},
    // ...
}
```

**注意:** `macOSSyscallEntry` 型は `macos_syscall_table.go` に残る。生成スクリプトは `macOSSyscallEntry` 型を使うコードを生成する。

**対応 AC:** AC-6

## 12. `scripts/generate_syscall_table.py`

### 12.1 変更内容: macOS ヘッダーパーサーの追加

#### 12.1.1 `MACOS_NETWORK_SYSCALL_NAMES` セットの追加

```python
MACOS_NETWORK_SYSCALL_NAMES = {
    "socket",
    "connect",
    "accept",
    "sendto",
    "recvfrom",
    "sendmsg",
    "recvmsg",
    "bind",
    "listen",
    "socketpair",
    "shutdown",
    "setsockopt",
    "getsockopt",
    "getpeername",
    "getsockname",
}
```

Linux 固有（macOS に存在しない）を除外: `accept4`・`recvmmsg`・`sendmmsg`
Linux にはないが macOS でソケット操作に使われるため追加: `shutdown`・`setsockopt`・`getsockopt`・`getpeername`・`getsockname`
（これらは既存の `networkSyscallWrapperNames` と一致させる）

`sendfile` は macOS でファイル-to-ソケット転送に使える syscall だが、既存の `networkSyscallWrapperNames` には含まれていないため追加しない。Linux の `NETWORK_SYSCALL_NAMES` にも含まれておらず、一貫性の観点から除外する。

#### 12.1.2 `parse_macos_header` 関数の追加

```python
def parse_macos_header(path: str) -> dict[str, int]:
    """Parse ``#define SYS_<name> <number>`` lines from a macOS syscall header.

    Returns a dict mapping syscall name to number.
    """
    pattern = re.compile(r"^#define\s+SYS_(\w+)\s+(\d+)\s*$")
    result: dict[str, int] = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            m = pattern.match(line)
            if m:
                name, number = m.group(1), int(m.group(2))
                result[name] = number
    return result
```

#### 12.1.3 macOS 用 `generate_macos` 関数の追加

macOS syscall テーブルは ELF テーブルと異なる型（`macOSSyscallEntry`）・パッケージ（`libccache`）を使うため、独立した生成関数を追加する。

**生成されるコード形式:**

```go
var macOSSyscallEntries = map[int]macOSSyscallEntry{
    3:   {name: "read", isNetwork: false},
    97:  {name: "socket", isNetwork: true},
    // ...
}
```

#### 12.1.4 `--macos-header` オプションの追加

```python
parser.add_argument(
    "--macos-header",
    default=None,
    help="Path to macOS BSD syscall header (e.g. $(xcrun --show-sdk-path)/usr/include/sys/syscall.h)",
)
```

オプションが指定された場合のみ macOS テーブルを生成し、既存の `--x86-header`・`--arm64-header` の動作には影響しない。

**対応 AC:** AC-6

## 13. `Makefile`

### 13.1 変更内容

**追加変数:**
```makefile
MACOS_SYSCALL_HEADER ?= $(shell xcrun --show-sdk-path 2>/dev/null | awk 'NF{print $$0"/usr/include/sys/syscall.h"}')
```

**`SYSCALL_TABLE_OUTPUTS` への追加:**
```makefile
SYSCALL_TABLE_OUTPUTS := \
    internal/runner/security/elfanalyzer/x86_syscall_numbers.go \
    internal/runner/security/elfanalyzer/arm64_syscall_numbers.go \
    internal/libccache/macos_syscall_numbers.go
```

**`generate-syscall-tables` ターゲットの更新:**

macOS SDK ヘッダーが存在する場合に `--macos-header` オプションを追加してスクリプトを呼び出す。存在しない環境（Linux CI 等）では macOS テーブル生成をスキップし、コミット済みファイルを使用する。

```makefile
generate-syscall-tables:
    # ... 既存の Linux ヘッダーチェック ...
    @if [ -f "$(MACOS_SYSCALL_HEADER)" ]; then \
        $(PYTHON) $(SYSCALL_TABLE_SCRIPT) \
            --x86-header $(X86_SYSCALL_HEADER) \
            --arm64-header $(ARM64_SYSCALL_HEADER) \
            --macos-header $(MACOS_SYSCALL_HEADER); \
    else \
        $(PYTHON) $(SYSCALL_TABLE_SCRIPT) \
            --x86-header $(X86_SYSCALL_HEADER) \
            --arm64-header $(ARM64_SYSCALL_HEADER); \
    fi
    $(GOFUMPTCMD) -w $(SYSCALL_TABLE_OUTPUTS)
```

**対応 AC:** AC-6

## 14. 関連ドキュメントの更新

### 14.1 `docs/tasks/0104_macho_syscall_number_analysis/` の整合修正

当該ディレクトリ配下に「`syscallAnalysisHasSVCSignal` を削除する」という記述がある場合、本タスク後の方針（削除ではなく `Number == -1` 条件追加により保持）と矛盾しないよう更新する。具体的には：

- 「削除する」という記述を「本タスク（0105）で `Number == -1` 条件を追加して修正した」という旨に変更する、または
- superseded として明示する

対象ファイル:
- `docs/tasks/0104_macho_syscall_number_analysis/03_detailed_specification.md`
- `docs/tasks/0104_macho_syscall_number_analysis/04_implementation_plan.md`

**対応 AC:** AC-8

## 15. 受け入れ基準とテストの対応

| 受け入れ基準 | 対応テストファイル | テスト内容 |
|------------|----------------|----------|
| AC-1: ELF 全 syscall 記録 | `validator_test.go` | 非ネットワーク・解決済み syscall が `DetectedSyscalls` に含まれる |
| AC-2: Mach-O 全 syscall 記録 | `validator_macho_test.go` | libSystem + svc 全エントリが `DetectedSyscalls` に含まれる |
| AC-3: `AnalysisWarnings` 正確な発出 | `validator_macho_test.go` | 未解決 svc のみ警告。解決済みのみの場合は警告なし |
| AC-4: 未解決 svc 高リスク判定 | `network_analyzer_test.go` | Number=-1 → high risk。Number!=−1 非ネットワーク → high risk でない |
| AC-5: 解決済みネットワーク svc 判定 | `network_analyzer_test.go` | IsNetwork=true, DeterminationMethod="direct_svc_0x80" → isNetwork=true |
| AC-6: macOS syscall テーブル拡張 | `libccache` テスト | GetSyscallName(3)="read", IsNetworkSyscall(97)=true, IsNetworkSyscall(3)=false |
| AC-7: 既存テスト通過 | 全テストファイル | `make test` `make lint` エラーなし |
| AC-8: 関連ドキュメント整合 | `docs/tasks/0104_macho_syscall_number_analysis/03_detailed_specification.md`, `docs/tasks/0104_macho_syscall_number_analysis/04_implementation_plan.md` | `syscallAnalysisHasSVCSignal` 削除前提が superseded または 0105 方針へ更新されていることを確認する |
| NFR-1: 既存レコード互換性 | `network_analyzer_test.go` | 旧レコード相当のフィルタ済み `DetectedSyscalls` でも判定が維持されることを確認する |
