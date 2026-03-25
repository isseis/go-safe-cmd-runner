# 詳細仕様書: 直接依存ライブラリによるネットワーク検出強化

## 1. 方策 A: commandRiskProfiles への追加

### 1.1 変更ファイル

`internal/runner/security/command_analysis.go`

### 1.2 追加エントリ

以下を `commandProfileDefinitions` に追加する（既存の ruby/python/node エントリと同じパターン）。

```go
// Lua interpreter
NewProfile("lua", "lua5.1", "lua5.2", "lua5.3", "lua5.4", "luajit").
    NetworkRisk(runnertypes.RiskLevelMedium, "Lua interpreter can load network extensions (e.g. LuaSocket)").
    AlwaysNetwork().
    Build(),

// Tcl/Tk interpreter
NewProfile("tclsh", "tclsh8.5", "tclsh8.6", "wish", "wish8.5", "wish8.6").
    NetworkRisk(runnertypes.RiskLevelMedium, "Tcl interpreter with built-in socket command").
    AlwaysNetwork().
    Build(),

// R language
NewProfile("R", "Rscript").
    NetworkRisk(runnertypes.RiskLevelMedium, "R interpreter with network-capable packages").
    AlwaysNetwork().
    Build(),

// Julia
NewProfile("julia").
    NetworkRisk(runnertypes.RiskLevelMedium, "Julia interpreter with built-in network capabilities").
    AlwaysNetwork().
    Build(),

// GNU Guile (Scheme)
NewProfile("guile", "guile2", "guile3").
    NetworkRisk(runnertypes.RiskLevelMedium, "Guile Scheme interpreter with network module").
    AlwaysNetwork().
    Build(),

// Erlang/Elixir
NewProfile("elixir", "iex").
    NetworkRisk(runnertypes.RiskLevelMedium, "Elixir runtime with built-in network capabilities").
    AlwaysNetwork().
    Build(),
NewProfile("erl", "erlc", "escript").
    NetworkRisk(runnertypes.RiskLevelMedium, "Erlang runtime, network-oriented language").
    AlwaysNetwork().
    Build(),

// JVM-based runtimes
NewProfile("java", "javaw").
    NetworkRisk(runnertypes.RiskLevelMedium, "JVM with built-in java.net network libraries").
    AlwaysNetwork().
    Build(),
NewProfile("groovy", "groovysh", "groovyConsole").
    NetworkRisk(runnertypes.RiskLevelMedium, "Groovy runtime on JVM with network capabilities").
    AlwaysNetwork().
    Build(),
NewProfile("kotlin").
    NetworkRisk(runnertypes.RiskLevelMedium, "Kotlin runtime on JVM with network capabilities").
    AlwaysNetwork().
    Build(),
NewProfile("scala", "scala3").
    NetworkRisk(runnertypes.RiskLevelMedium, "Scala runtime on JVM with network capabilities").
    AlwaysNetwork().
    Build(),
NewProfile("clojure").
    NetworkRisk(runnertypes.RiskLevelMedium, "Clojure runtime on JVM with network capabilities").
    AlwaysNetwork().
    Build(),
NewProfile("jruby").
    NetworkRisk(runnertypes.RiskLevelMedium, "JRuby runtime with Ruby network libraries on JVM").
    AlwaysNetwork().
    Build(),
NewProfile("jython").
    NetworkRisk(runnertypes.RiskLevelMedium, "Jython runtime with Python network libraries on JVM").
    AlwaysNetwork().
    Build(),

// .NET runtimes
NewProfile("dotnet").
    NetworkRisk(runnertypes.RiskLevelMedium, ".NET runtime with System.Net network libraries").
    AlwaysNetwork().
    Build(),
NewProfile("mono").
    NetworkRisk(runnertypes.RiskLevelMedium, "Mono .NET runtime with network capabilities").
    AlwaysNetwork().
    Build(),
NewProfile("pwsh", "powershell").
    NetworkRisk(runnertypes.RiskLevelMedium, "PowerShell with built-in network cmdlets").
    AlwaysNetwork().
    Build(),
```

---

## 2. 方策 C: SOName ベース検出

### 2.1 新規ファイル: `known_network_libs.go`

**配置**: `internal/runner/security/binaryanalyzer/known_network_libs.go`

```go
package binaryanalyzer

import "strings"

// knownNetworkLibPrefixes lists SOName prefixes for known network-related libraries.
// Match against SOName using safe prefix matching.
// Examples: "libruby.so.3.2" matches "libruby",
//           "libpython3.11.so.1.0" matches "libpython",
//           "libpythonista.so" does not match.
var knownNetworkLibPrefixes = map[string]struct{}{
    // =====================================================
    // Network protocol libraries
    // =====================================================

    // Network communication such as HTTP/FTP/SMTP
    "libcurl": {},

    // TLS connections (network-oriented)
    // Note: Exclude libcrypto because it is also used for disk encryption and similar purposes
    "libssl": {},

    // SSH connections
    "libssh":  {},
    "libssh2": {},

    // Network messaging
    "libzmq":      {},
    "libnanomsg":  {},
    "libnng":      {},

    // HTTP/2 protocol implementation
    "libnghttp2": {},

    // WebSocket
    "libwebsockets": {},

    // MQTT (IoT messaging)
    "libmosquitto": {},

    // Mozilla NSS (Firefox-family TLS implementation)
    "libnss3": {},

    // libuv: asynchronous I/O (Node.js core, including network I/O)
    "libuv": {},

    // =====================================================
    // Language runtime libraries
    // =====================================================

    // Ruby runtime (can perform network communication via scripts)
    "libruby": {},

    // Python runtime (includes socket, urllib, http, and similar modules in the standard library)
    "libpython": {},

    // Perl runtime (can perform network communication via LWP, IO::Socket, and similar modules)
    "libperl": {},

    // PHP runtime (includes curl, fsockopen, and similar features)
    "libphp": {},

    // Lua runtime (can perform network communication via extensions such as LuaSocket)
    "liblua": {},

    // Java VM (includes java.net in the standard library)
    "libjvm": {},

    // Mono .NET runtime (includes System.Net in the standard library)
    "libmono":      {},
    "libmonoboehm": {},

    // Embedded Node.js runtime
    "libnode": {},
}

// matchesKnownPrefix reports whether the SOName matches a registered prefix.
// "libpython" matches "libpython3.11.so.1.0",
// but does not match "libpythonista.so".
func matchesKnownPrefix(soname, prefix string) bool {
    if !strings.HasPrefix(soname, prefix) {
        return false
    }

    rest := soname[len(prefix):]
    if len(rest) == 0 {
        return true
    }

    return rest[0] == '.' || rest[0] == '-' || (rest[0] >= '0' && rest[0] <= '9')
}

// IsKnownNetworkLibrary reports whether the SOName matches the known network library list.
// soname: DT_NEEDED value (for example, "libruby.so.3.2", "libcurl.so.4", "libpython3.11.so.1.0")
func IsKnownNetworkLibrary(soname string) bool {
    for prefix := range knownNetworkLibPrefixes {
        if matchesKnownPrefix(soname, prefix) {
            return true
        }
    }
    return false
}

// KnownNetworkLibraryCount returns the number of registered library prefixes (for tests and documentation).
func KnownNetworkLibraryCount() int {
    return len(knownNetworkLibPrefixes)
}
```

### 2.2 スキーマ変更: `SymbolAnalysisData`（`internal/fileanalysis/schema.go`）

```go
// After change
type SymbolAnalysisData struct {
    AnalyzedAt         time.Time            `json:"analyzed_at"`
    DetectedSymbols    []DetectedSymbolEntry `json:"detected_symbols,omitempty"`
    DynamicLoadSymbols []DetectedSymbolEntry `json:"dynamic_load_symbols,omitempty"`

    // KnownNetworkLibDeps lists SOName values of known network libraries
    // detected from DynLibDeps during record.
    // If non-empty, this binary is treated as network-capable.
    KnownNetworkLibDeps []string `json:"known_network_lib_deps,omitempty"`
}
```

`CurrentSchemaVersion` を 7 → 8 に更新する。コメントに `// Version 8 adds KnownNetworkLibDeps to SymbolAnalysisData.` を追記する。

### 2.3 record 処理の変更（`internal/filevalidator/validator.go`）

`updateAnalysisRecord` 内のシンボル解析ブロック（`binaryAnalyzer != nil` のブロック）末尾に以下を追加する：

```go
// Detect known network libraries based on SOName.
// Run only when DynLibDeps is recorded and SymbolAnalysis is set.
if record.DynLibDeps != nil && record.SymbolAnalysis != nil {
    var matched []string
    for _, lib := range record.DynLibDeps.Libs {
        if binaryanalyzer.IsKnownNetworkLibrary(lib.SOName) {
            matched = append(matched, lib.SOName)
        }
    }
    if len(matched) > 0 {
        record.SymbolAnalysis.KnownNetworkLibDeps = matched
    }
}
```

**前提条件**: `record.DynLibDeps` の設定は `record.SymbolAnalysis` の設定より前に行われている（現在の実装どおり）。`record.SymbolAnalysis` が nil の場合（静的バイナリ・非 ELF）は SOName 照合もスキップする。

### 2.4 runner 側の判定変更（`internal/runner/security/network_analyzer.go`）

`isNetworkViaBinaryAnalysis` 内のキャッシュ読み込み成功パス：

```go
// Before change
if len(data.DetectedSymbols) > 0 {
    output.Result = binaryanalyzer.NetworkDetected
} else {
    output.Result = binaryanalyzer.NoNetworkSymbols
}

// After change
if len(data.DetectedSymbols) > 0 || len(data.KnownNetworkLibDeps) > 0 {
    output.Result = binaryanalyzer.NetworkDetected
} else {
    output.Result = binaryanalyzer.NoNetworkSymbols
}
```

---

## 3. テスト仕様

### 3.1 `IsKnownNetworkLibrary` 単体テスト（`known_network_libs_test.go`）

| 入力 SOName | 期待結果 |
|---|---|
| `libruby.so.3.2` | true |
| `libcurl.so.4` | true |
| `libssl.so.3` | true |
| `libpython3.11.so.1.0` | true |
| `libjvm.so` | true |
| `libstdc++.so.6` | false |
| `libz.so.1` | false |
| `libcrypto.so.3` | false |
| `libgnutls.so.30` | false |
| `libgcrypt.so.20` | false |
| `libpthread.so.0` | false |

### 3.2 `matchesKnownPrefix` 単体テスト

| 入力 | 期待結果 |
|---|---|
| `matchesKnownPrefix("libruby.so.3.2", "libruby")` | true |
| `matchesKnownPrefix("libcurl.so.4", "libcurl")` | true |
| `matchesKnownPrefix("libpython3.11.so.1.0", "libpython")` | true |
| `matchesKnownPrefix("libpythonista.so", "libpython")` | false |
| `matchesKnownPrefix("libssl.so.3", "libssl")` | true |

### 3.3 `libpython` バージョン付き SOName への対応

Python の SOName は `libpython3.11.so.1.0`, `libpython3.12.so.1.0` のようにバージョン番号が `.so` の前に付く形式をとる場合がある。単純な完全一致では `libpython` でカバーできないため、安全な前方一致を採用する。

対応方針は以下のとおり：

```go
// Python SOName formats:
//   libpython.so.1.0          (Python 2 series)
//   libpython3.so             (generic)
//   libpython3.11.so.1.0      (Python 3.x)
// Register "libpython" and handle these with safe prefix matching.
```

ただしライブラリ名が他のライブラリ名のプレフィックスになる危険を避けるため、照合は `soname` が `<prefix>` の直後に `.`, `-`, 数字のいずれかを持つ場合のみ一致とする。

具体的には以下の関数で照合する：

```go
// matchesKnownPrefix reports whether the SOName matches a registered prefix.
// "libpython" also matches "libpython3.11.so.1.0" but does not match "libpythonista.so".
func matchesKnownPrefix(soname, prefix string) bool {
    if !strings.HasPrefix(soname, prefix) {
        return false
    }
    rest := soname[len(prefix):]
    if len(rest) == 0 {
        return true // Exact match
    }
    // Match only when followed by ".", "-", or a digit (for example, libpython3.11.so, libpython-2.7.so)
    return rest[0] == '.' || rest[0] == '-' || (rest[0] >= '0' && rest[0] <= '9')
}
```

### 3.4 commandRiskProfiles テスト（`command_analysis_test.go`）

新規追加バイナリ（`luajit`, `java`, `pwsh` 等）がネットワーク有り（`NetworkTypeAlways`）として判定されることを確認する。

### 3.5 統合テスト（`filevalidator` パッケージ）

- `DynLibDeps` に `libcurl.so.4` を含む場合、`SymbolAnalysis.KnownNetworkLibDeps = ["libcurl.so.4"]` が記録される
- `DynLibDeps` に `libz.so.1` のみを含む場合、`KnownNetworkLibDeps` は空（または nil）
- `SymbolAnalysis` が nil（静的バイナリ）の場合、`KnownNetworkLibDeps` は記録されない

### 3.6 受け入れ基準とテストの対応

| 受け入れ基準 | テスト |
|---|---|
| AC-2: `luajit` がネットワーク有りと判定 | `TestCommandRiskProfiles_Lua` |
| AC-3: `java` がネットワーク有りと判定 | `TestCommandRiskProfiles_Java` |
| AC-4: `libruby.so` → `KnownNetworkLibDeps` に記録 | `TestIsKnownNetworkLibrary_Ruby` + 統合テスト |
| AC-5: `libcurl.so` → `KnownNetworkLibDeps` に記録 | `TestIsKnownNetworkLibrary_Curl` + 統合テスト |
| AC-5.1: `libpython3.11.so.1.0` → `KnownNetworkLibDeps` に記録 | `TestIsKnownNetworkLibrary_PythonVersioned` + 統合テスト |
| AC-6: `KnownNetworkLibDeps` 非空 → ネットワーク有り | `network_analyzer` テスト |
| AC-7: `libstdc++.so` は記録されない | `TestIsKnownNetworkLibrary_StdCpp` |
| AC-8: `libpythonista.so` は記録されない | `TestMatchesKnownPrefix_Pythonista` |
| AC-9: 既存の symbol 検出が変わらない | 既存テストが全て通ること |
