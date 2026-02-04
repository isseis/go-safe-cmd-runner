# ELF 動的シンボル解析によるネットワーク操作検出 詳細仕様書

## 1. 概要

### 1.1 目的

本仕様書は、ELF バイナリの `.dynsym` セクションを解析してネットワーク操作の可能性を検出する機能の詳細設計を定義する。

### 1.2 設計範囲

- `elfanalyzer` パッケージの実装詳細
- `IsNetworkOperation` への統合方法
- 検出対象シンボルリストの定義
- エラーハンドリングの詳細
- テスト実装の詳細

## 2. パッケージ構成

### 2.1 ディレクトリ構造

```
internal/runner/security/elfanalyzer/
├── analyzer.go           # インターフェースと型定義
├── analyzer_impl.go      # StandardELFAnalyzer の実装
├── network_symbols.go    # ネットワークシンボルのレジストリ
├── analyzer_test.go      # ユニットテスト
├── doc.go                # パッケージドキュメント
└── testdata/             # テストフィクスチャ
    ├── README.md         # テストデータの説明と生成方法
    ├── with_socket.elf   # socket API を使用するバイナリ
    ├── with_curl.elf     # libcurl をリンクするバイナリ
    ├── with_ssl.elf      # OpenSSL をリンクするバイナリ
    ├── no_network.elf    # ネットワークシンボルなしのバイナリ
    ├── static.elf        # 静的リンクされたバイナリ
    ├── script.sh         # シェルスクリプト（非 ELF）
    └── corrupted.elf     # 破損した ELF ファイル
```

## 3. 型定義

### 3.1 analyzer.go

```go
// Package elfanalyzer provides ELF binary analysis for detecting network operation capability.
// It analyzes the dynamic symbol table (.dynsym) of ELF binaries to identify
// imported network-related functions.
//
// This package is designed to work with dynamically linked binaries.
// Statically linked binaries (like Go binaries) will return StaticBinary result
// and should be handled by a separate syscall analysis mechanism (Task 0070).
package elfanalyzer

import (
    "fmt"
)

// AnalysisResult represents the result type of ELF network symbol analysis.
type AnalysisResult int

const (
    // NetworkDetected indicates that network-related symbols were found
    // in the .dynsym section. The binary is capable of network operations.
    NetworkDetected AnalysisResult = iota

    // NoNetworkSymbols indicates that no network-related symbols were found.
    // The binary does not appear to use standard network APIs.
    NoNetworkSymbols

    // NotELFBinary indicates that the file is not an ELF binary.
    // This includes scripts, text files, and other non-ELF executables.
    NotELFBinary

    // StaticBinary indicates a statically linked binary with no .dynsym section.
    // Network capability cannot be determined by this analyzer.
    // Falls back to alternative analysis methods (Task 0070).
    StaticBinary

    // AnalysisError indicates that an error occurred during analysis.
    // This should be treated as a potential network operation for safety.
    AnalysisError
)

// String returns a string representation of AnalysisResult.
func (r AnalysisResult) String() string {
    switch r {
    case NetworkDetected:
        return "network_detected"
    case NoNetworkSymbols:
        return "no_network_symbols"
    case NotELFBinary:
        return "not_elf_binary"
    case StaticBinary:
        return "static_binary"
    case AnalysisError:
        return "analysis_error"
    default:
        return fmt.Sprintf("unknown(%d)", int(r))
    }
}

// DetectedSymbol contains information about a detected network symbol.
type DetectedSymbol struct {
    // Name is the symbol name as it appears in .dynsym (e.g., "socket", "curl_easy_init")
    Name string

    // Category is the classification of the symbol (e.g., "socket", "http", "tls")
    Category string
}

// AnalysisOutput contains the complete result of ELF analysis.
type AnalysisOutput struct {
    // Result is the overall analysis result type
    Result AnalysisResult

    // DetectedSymbols contains all network-related symbols found in .dynsym.
    // Only populated when Result == NetworkDetected.
    // Useful for logging and debugging purposes.
    DetectedSymbols []DetectedSymbol

    // Error contains the error details when Result == AnalysisError.
    // nil for other result types.
    Error error
}

// IsNetworkCapable returns true if the analysis indicates the binary
// might perform network operations.
func (o AnalysisOutput) IsNetworkCapable() bool {
    return o.Result == NetworkDetected
}

// IsIndeterminate returns true if the analysis could not determine
// network capability (error or static binary).
func (o AnalysisOutput) IsIndeterminate() bool {
    return o.Result == AnalysisError || o.Result == StaticBinary
}

// ELFAnalyzer defines the interface for ELF binary network analysis.
type ELFAnalyzer interface {
    // AnalyzeNetworkSymbols examines the ELF binary at the given path
    // and determines if it contains network-related symbols in .dynsym.
    //
    // The path must be an absolute path to an executable file.
    // The analyzer uses safe file I/O to prevent symlink attacks.
    //
    // privManager is an optional PrivilegeManager for reading execute-only
    // binaries (permissions like 0111). If nil, normal file access is used.
    // If the file requires elevated privileges and privManager is available,
    // OpenFileWithPrivileges will be used to temporarily escalate privileges.
    //
    // Returns:
    //   - NetworkDetected: Binary contains network symbols
    //   - NoNetworkSymbols: Binary has .dynsym but no network symbols
    //   - NotELFBinary: File is not an ELF binary
    //   - StaticBinary: Binary is statically linked (no .dynsym)
    //   - AnalysisError: An error occurred (check Error field)
    AnalyzeNetworkSymbols(path string, privManager runnertypes.PrivilegeManager) AnalysisOutput
}
```

### 3.2 network_symbols.go

```go
package elfanalyzer

// SymbolCategory represents the category of a network-related symbol.
type SymbolCategory string

const (
    // CategorySocket represents POSIX socket API functions.
    CategorySocket SymbolCategory = "socket"

    // CategoryHTTP represents HTTP client library functions.
    CategoryHTTP SymbolCategory = "http"

    // CategoryTLS represents TLS/SSL library functions.
    CategoryTLS SymbolCategory = "tls"

    // CategoryDNS represents DNS resolution functions.
    CategoryDNS SymbolCategory = "dns"
)

// networkSymbolRegistry contains the default set of network-related symbols.
// Key: symbol name, Value: category
//
// Symbol names should NOT include version suffixes (e.g., @GLIBC_2.2.5)
// as Go's debug/elf.DynamicSymbols() returns names without versioning.
var networkSymbolRegistry = map[string]SymbolCategory{
    // =========================================
    // Socket API (POSIX)
    // =========================================

    // Socket creation and connection
    "socket":  CategorySocket,
    "connect": CategorySocket,
    "bind":    CategorySocket,
    "listen":  CategorySocket,
    "accept":  CategorySocket,
    "accept4": CategorySocket, // Linux-specific

    // Data transmission
    "send":    CategorySocket,
    "sendto":  CategorySocket,
    "sendmsg": CategorySocket,
    "recv":    CategorySocket,
    "recvfrom": CategorySocket,
    "recvmsg": CategorySocket,

    // Socket information
    "getpeername": CategorySocket,
    "getsockname": CategorySocket,

    // Address conversion
    "inet_ntop": CategorySocket,
    "inet_pton": CategorySocket,

    // =========================================
    // DNS Resolution
    // =========================================

    "getaddrinfo":   CategoryDNS,
    "getnameinfo":   CategoryDNS,
    "gethostbyname": CategoryDNS, // Legacy, but still widely used
    "gethostbyname2": CategoryDNS, // IPv4/IPv6 variant

    // =========================================
    // HTTP Libraries (libcurl)
    // =========================================

    "curl_easy_init":     CategoryHTTP,
    "curl_easy_perform":  CategoryHTTP,
    "curl_easy_cleanup":  CategoryHTTP,
    "curl_multi_init":    CategoryHTTP,
    "curl_multi_perform": CategoryHTTP,
    "curl_multi_cleanup": CategoryHTTP,
    "curl_global_init":   CategoryHTTP,

    // =========================================
    // TLS/SSL Libraries (OpenSSL)
    // =========================================

    "SSL_new":           CategoryTLS,
    "SSL_connect":       CategoryTLS,
    "SSL_accept":        CategoryTLS,
    "SSL_read":          CategoryTLS,
    "SSL_write":         CategoryTLS,
    "SSL_shutdown":      CategoryTLS,
    "SSL_free":          CategoryTLS,
    "SSL_CTX_new":       CategoryTLS,
    "SSL_CTX_free":      CategoryTLS,
    "SSL_library_init":  CategoryTLS, // Legacy OpenSSL 1.0
    "OPENSSL_init_ssl":  CategoryTLS, // OpenSSL 1.1+

    // =========================================
    // TLS/SSL Libraries (GnuTLS)
    // =========================================

    "gnutls_init":           CategoryTLS,
    "gnutls_handshake":      CategoryTLS,
    "gnutls_record_send":    CategoryTLS,
    "gnutls_record_recv":    CategoryTLS,
    "gnutls_bye":            CategoryTLS,
    "gnutls_deinit":         CategoryTLS,
    "gnutls_global_init":    CategoryTLS,
}

// GetNetworkSymbols returns a copy of the network symbol registry.
// This is used by StandardELFAnalyzer for symbol matching.
func GetNetworkSymbols() map[string]SymbolCategory {
    // Return a copy to prevent external modification
    result := make(map[string]SymbolCategory, len(networkSymbolRegistry))
    for k, v := range networkSymbolRegistry {
        result[k] = v
    }
    return result
}

// IsNetworkSymbol checks if the given symbol name is a known network symbol.
// Returns the category if found, empty string otherwise.
func IsNetworkSymbol(name string) (SymbolCategory, bool) {
    cat, found := networkSymbolRegistry[name]
    return cat, found
}

// SymbolCount returns the number of registered network symbols.
// Useful for testing and documentation.
func SymbolCount() int {
    return len(networkSymbolRegistry)
}
```

### 3.3 analyzer_impl.go

```go
package elfanalyzer

import (
    "debug/elf"
    "errors"
    "fmt"
    "io"
    "os"
    "strings"

    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// ELF magic number bytes
var elfMagic = []byte{0x7f, 'E', 'L', 'F'}

// StandardELFAnalyzer implements ELFAnalyzer using Go's debug/elf package.
type StandardELFAnalyzer struct {
    fs             safefileio.FileSystem
    networkSymbols map[string]SymbolCategory
}

// NewStandardELFAnalyzer creates a new StandardELFAnalyzer with the given file system.
// If fs is nil, the default safefileio.FileSystem is used.
func NewStandardELFAnalyzer(fs safefileio.FileSystem) *StandardELFAnalyzer {
    if fs == nil {
        fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
    }
    return &StandardELFAnalyzer{
        fs:             fs,
        networkSymbols: GetNetworkSymbols(),
    }
}

// NewStandardELFAnalyzerWithSymbols creates an analyzer with custom network symbols.
// This is primarily for testing purposes.
func NewStandardELFAnalyzerWithSymbols(fs safefileio.FileSystem, symbols map[string]SymbolCategory) *StandardELFAnalyzer {
    if fs == nil {
        fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
    }
    return &StandardELFAnalyzer{
        fs:             fs,
        networkSymbols: symbols,
    }
}

// AnalyzeNetworkSymbols implements ELFAnalyzer interface.
func (a *StandardELFAnalyzer) AnalyzeNetworkSymbols(path string, privManager runnertypes.PrivilegeManager) AnalysisOutput {
    // Step 1: Open file safely using safefileio
    // This prevents symlink attacks and TOCTOU race conditions
    file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        // If it's a permission error and we have privilege manager, try privileged access
        if errors.Is(err, os.ErrPermission) && privManager != nil {
            file, err = filevalidator.OpenFileWithPrivileges(path, privManager)
            if err != nil {
                return AnalysisOutput{
                    Result: AnalysisError,
                    Error:  fmt.Errorf("failed to open execute-only binary even with privileges: %w", err),
                }
            }
        } else {
            return AnalysisOutput{
                Result: AnalysisError,
                Error:  fmt.Errorf("failed to open file: %w", err),
            }
        }
    }
    defer file.Close()

    // Step 2: Check ELF magic number
    magic := make([]byte, 4)
    if _, err := io.ReadFull(file.(io.Reader), magic); err != nil {
        return AnalysisOutput{
            Result: AnalysisError,
            Error:  fmt.Errorf("failed to read magic number: %w", err),
        }
    }

    if !isELFMagic(magic) {
        return AnalysisOutput{
            Result: NotELFBinary,
        }
    }

    // Step 3: Parse ELF using debug/elf.NewFile
    // The safefileio.File interface implements io.ReaderAt, so we can
    // pass it directly to elf.NewFile without re-opening the file.
    // This eliminates potential TOCTOU race conditions.
    elfFile, err := elf.NewFile(file.(io.ReaderAt))
    if err != nil {
        return AnalysisOutput{
            Result: AnalysisError,
            Error:  fmt.Errorf("failed to parse ELF: %w", err),
        }
    }
    defer elfFile.Close()

    // Step 4: Get dynamic symbols
    dynsyms, err := elfFile.DynamicSymbols()
    if err != nil {
        // Check if error indicates no .dynsym section (static binary)
        if isNoDynsymError(err) {
            return AnalysisOutput{
                Result: StaticBinary,
            }
        }
        return AnalysisOutput{
            Result: AnalysisError,
            Error:  fmt.Errorf("failed to read dynamic symbols: %w", err),
        }
    }

    // Empty .dynsym is treated as static binary
    if len(dynsyms) == 0 {
        return AnalysisOutput{
            Result: StaticBinary,
        }
    }

    // Step 5: Check for network symbols
    var detected []DetectedSymbol
    for _, sym := range dynsyms {
        // Only check undefined symbols (imported from shared libraries)
        // Defined symbols are exported, not imported
        if sym.Section == elf.SHN_UNDEF {
            if cat, found := a.networkSymbols[sym.Name]; found {
                detected = append(detected, DetectedSymbol{
                    Name:     sym.Name,
                    Category: string(cat),
                })
            }
        }
    }

    if len(detected) > 0 {
        return AnalysisOutput{
            Result:          NetworkDetected,
            DetectedSymbols: detected,
        }
    }

    return AnalysisOutput{
        Result: NoNetworkSymbols,
    }
}

// isELFMagic checks if the given bytes match the ELF magic number.
func isELFMagic(magic []byte) bool {
    if len(magic) < 4 {
        return false
    }
    for i := 0; i < 4; i++ {
        if magic[i] != elfMagic[i] {
            return false
        }
    }
    return true
}

// isNoDynsymError checks if the error indicates no .dynsym section exists.
func isNoDynsymError(err error) bool {
    if err == nil {
        return false
    }
    // debug/elf returns specific errors for missing sections
    // The exact error message may vary, so we check for common patterns
    errStr := err.Error()
    return errors.Is(err, elf.ErrNoSymbols) ||
        containsAny(errStr, "no symbol", "no dynamic symbol", "SHT_DYNSYM")
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
    for _, sub := range substrs {
        if strings.Contains(s, sub) {
            return true
        }
    }
    return false
}
```

## 4. IsNetworkOperation への統合

### 4.1 command_analysis.go の変更

```go
// Package security に追加するコード

import (
    "fmt"
    "log/slog"
    "os/exec"
    "slices"
    "strings"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// defaultELFAnalyzer is the package-level ELF analyzer instance.
// Initialized lazily to avoid import cycle issues.
var defaultELFAnalyzer elfanalyzer.ELFAnalyzer

// getELFAnalyzer returns the default ELF analyzer, creating it if necessary.
func getELFAnalyzer() elfanalyzer.ELFAnalyzer {
    if defaultELFAnalyzer == nil {
        defaultELFAnalyzer = elfanalyzer.NewStandardELFAnalyzer(nil)
    }
    return defaultELFAnalyzer
}

// SetELFAnalyzer sets a custom ELF analyzer for testing purposes.
func SetELFAnalyzer(analyzer elfanalyzer.ELFAnalyzer) {
    defaultELFAnalyzer = analyzer
}

// IsNetworkOperation checks if the command performs network operations.
// This function considers symbolic links to detect network commands properly.
// Returns (isNetwork, isHighRisk) where isHighRisk indicates symlink depth exceeded.
//
// Detection priority:
// 1. commandProfileDefinitions (hardcoded list) - takes precedence
// 2. ELF .dynsym analysis for unknown commands
// 3. Argument-based detection (URLs, SSH-style addresses)
func IsNetworkOperation(cmdName string, args []string) (bool, bool) {
    // Extract all possible command names including symlink targets
    commandNames, exceededDepth := extractAllCommandNames(cmdName)

    // If symlink depth exceeded, this is a high risk security concern
    if exceededDepth {
        return false, true
    }

    // Check command profiles for network type using unified profiles
    var conditionalProfile *CommandRiskProfile
    foundInProfiles := false
    for name := range commandNames {
        if profile, exists := commandRiskProfiles[name]; exists {
            foundInProfiles = true
            switch profile.NetworkType {
            case NetworkTypeAlways:
                return true, false
            case NetworkTypeConditional:
                conditionalProfile = &profile
            }
        }
    }

    if conditionalProfile != nil {
        // Check for network subcommands (e.g., git fetch, git push)
        if len(conditionalProfile.NetworkSubcommands) > 0 {
            subcommand := findFirstSubcommand(args)
            if subcommand != "" && slices.Contains(conditionalProfile.NetworkSubcommands, subcommand) {
                return true, false
            }
        }

        // Check for network-related arguments
        allArgs := strings.Join(args, " ")
        if strings.Contains(allArgs, "://") || containsSSHStyleAddress(args) {
            return true, false
        }
        return false, false
    }

    // NEW: If not found in profiles, try ELF analysis
    if !foundInProfiles {
        isNetwork, isMiddleRisk := analyzeELFForNetwork(cmdName)
        if isNetwork || isMiddleRisk {
            return isNetwork, false // isMiddleRisk maps to isNetwork for safety
        }
    }

    // Check for network-related arguments in any command
    allArgs := strings.Join(args, " ")
    if strings.Contains(allArgs, "://") || containsSSHStyleAddress(args) {
        return true, false
    }

    return false, false
}

// analyzeELFForNetwork performs ELF .dynsym analysis on the command binary.
// Returns (isNetwork, isMiddleRisk) where:
//   - isNetwork: true if network symbols were detected
//   - isMiddleRisk: true if analysis failed (treat as potential network operation)
func analyzeELFForNetwork(cmdName string) (bool, bool) {
    // Resolve command path
    cmdPath, err := exec.LookPath(cmdName)
    if err != nil {
        // Cannot find command, skip ELF analysis
        slog.Debug("ELF analysis skipped: command not found in PATH",
            "command", cmdName,
            "error", err)
        return false, false
    }

    // Perform ELF analysis
    analyzer := getELFAnalyzer()
    output := analyzer.AnalyzeNetworkSymbols(cmdPath)

    switch output.Result {
    case elfanalyzer.NetworkDetected:
        slog.Debug("ELF analysis detected network symbols",
            "command", cmdName,
            "path", cmdPath,
            "symbols", formatDetectedSymbols(output.DetectedSymbols))
        return true, false

    case elfanalyzer.NoNetworkSymbols:
        slog.Debug("ELF analysis found no network symbols",
            "command", cmdName,
            "path", cmdPath)
        return false, false

    case elfanalyzer.NotELFBinary:
        slog.Debug("ELF analysis skipped: not an ELF binary",
            "command", cmdName,
            "path", cmdPath)
        return false, false

    case elfanalyzer.StaticBinary:
        // Static binary: cannot determine network capability
        // Return false for now, 2nd step (Task 0070) will handle this
        slog.Debug("ELF analysis: static binary detected, cannot determine network capability",
            "command", cmdName,
            "path", cmdPath)
        return false, false

    case elfanalyzer.AnalysisError:
        // Analysis failed: treat as potential network operation for safety
        slog.Warn("ELF analysis failed, treating as potential network operation",
            "command", cmdName,
            "path", cmdPath,
            "error", output.Error,
            "reason", "Unable to determine network capability, assuming middle risk for safety")
        return true, true // isNetwork=true for safety, isMiddleRisk=true

    default:
        // Unknown result: treat as potential network operation for safety
        slog.Warn("ELF analysis returned unknown result",
            "command", cmdName,
            "path", cmdPath,
            "result", output.Result)
        return true, true
    }
}

// formatDetectedSymbols formats detected symbols for logging.
func formatDetectedSymbols(symbols []elfanalyzer.DetectedSymbol) string {
    if len(symbols) == 0 {
        return "[]"
    }
    var parts []string
    for _, s := range symbols {
        parts = append(parts, fmt.Sprintf("%s(%s)", s.Name, s.Category))
    }
    return "[" + strings.Join(parts, ", ") + "]"
}
```

## 5. エラーハンドリング詳細

### 5.1 エラー種別と対応

| エラー種別 | 発生条件 | AnalysisResult | 呼び出し元の動作 |
|-----------|---------|----------------|----------------|
| ファイルオープン失敗 | 権限不足、パス不正 | `AnalysisError` | Middle Risk として扱う |
| マジックナンバー読み取り失敗 | 空ファイル、I/O エラー | `AnalysisError` | Middle Risk として扱う |
| 非 ELF ファイル | シェルスクリプト等 | `NotELFBinary` | 既存ロジックに委ねる |
| ELF パース失敗 | 破損した ELF | `AnalysisError` | Middle Risk として扱う |
| .dynsym セクションなし | 静的リンクバイナリ | `StaticBinary` | 2nd step に委ねる（現時点では検出なし） |
| シンボル読み取りエラー | 破損した .dynsym | `AnalysisError` | Middle Risk として扱う |

**特記事項：実行専用権限バイナリ**

実行権限のみ（`--x--x--x`、octal `0111`）のバイナリは実行可能だが読み取り不可。この場合：
- `SafeOpenFile` が `Permission denied` で失敗
- `AnalysisError` として扱い、Middle Risk 判定
- 詳細は [04_edge_cases.md](04_edge_cases.md) を参照

**推奨**: バイナリには読み取り権限も付与すること（例：`0755` = rwxr-xr-x）

### 5.2 ログメッセージフォーマット

```go
// 正常系: ネットワークシンボル検出
slog.Debug("ELF analysis detected network symbols",
    "command", "custom_downloader",
    "path", "/usr/local/bin/custom_downloader",
    "symbols", "[socket(socket), connect(socket), SSL_connect(tls)]")

// 正常系: ネットワークシンボルなし
slog.Debug("ELF analysis found no network symbols",
    "command", "myapp",
    "path", "/usr/local/bin/myapp")

// 正常系: 非 ELF バイナリ
slog.Debug("ELF analysis skipped: not an ELF binary",
    "command", "script.sh",
    "path", "/usr/local/bin/script.sh")

// 正常系: 静的リンクバイナリ
slog.Debug("ELF analysis: static binary detected, cannot determine network capability",
    "command", "go_binary",
    "path", "/usr/local/bin/go_binary")

// 異常系: 解析失敗（Middle Risk）
slog.Warn("ELF analysis failed, treating as potential network operation",
    "command", "unknown_binary",
    "path", "/usr/local/bin/unknown_binary",
    "error", "failed to parse ELF: invalid ELF header",
    "reason", "Unable to determine network capability, assuming middle risk for safety")
```

## 6. テスト仕様

### 6.1 テストフィクスチャの生成

`testdata/README.md` に以下の生成方法を記載：

```markdown
# Test Fixtures for ELF Analyzer

## Generation Instructions

### Prerequisites
- GCC (for dynamic binaries)
- musl-gcc or static GCC (for static binaries)

### Generate test binaries

# 1. Binary with socket API symbols
```bash
cat > /tmp/with_socket.c << 'EOF'
#include <sys/socket.h>
#include <netinet/in.h>
int main() {
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    struct sockaddr_in addr = {0};
    connect(fd, (struct sockaddr*)&addr, sizeof(addr));
    return 0;
}
EOF
gcc -o with_socket.elf /tmp/with_socket.c
```

# 2. Binary with libcurl
```bash
cat > /tmp/with_curl.c << 'EOF'
#include <curl/curl.h>
int main() {
    CURL *curl = curl_easy_init();
    if(curl) {
        curl_easy_cleanup(curl);
    }
    return 0;
}
EOF
gcc -o with_curl.elf /tmp/with_curl.c -lcurl
```

# 3. Binary with OpenSSL
```bash
cat > /tmp/with_ssl.c << 'EOF'
#include <openssl/ssl.h>
int main() {
    SSL_library_init();
    SSL_CTX *ctx = SSL_CTX_new(TLS_client_method());
    SSL_CTX_free(ctx);
    return 0;
}
EOF
gcc -o with_ssl.elf /tmp/with_ssl.c -lssl -lcrypto
```

# 4. Binary without network symbols
```bash
cat > /tmp/no_network.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
int main() {
    printf("Hello, World!\n");
    return 0;
}
EOF
gcc -o no_network.elf /tmp/no_network.c
```

# 5. Statically linked binary
```bash
gcc -static -o static.elf /tmp/no_network.c
```

# 6. Shell script (non-ELF)
```bash
echo '#!/bin/bash' > script.sh
echo 'echo "Hello"' >> script.sh
chmod +x script.sh
```

# 7. Corrupted ELF
```bash
printf '\x7fELF' > corrupted.elf
dd if=/dev/urandom bs=100 count=1 >> corrupted.elf 2>/dev/null
```
```

### 6.2 ユニットテスト

```go
// analyzer_test.go

package elfanalyzer

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestStandardELFAnalyzer_AnalyzeNetworkSymbols(t *testing.T) {
    // Skip if test fixtures don't exist
    testdataDir := "testdata"
    if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
        t.Skip("testdata directory not found, skipping ELF analysis tests")
    }

    analyzer := NewStandardELFAnalyzer(nil)

    tests := []struct {
        name           string
        filename       string
        expectedResult AnalysisResult
        expectSymbols  bool
        skipIfMissing  bool
    }{
        {
            name:           "binary with socket symbols",
            filename:       "with_socket.elf",
            expectedResult: NetworkDetected,
            expectSymbols:  true,
            skipIfMissing:  true,
        },
        {
            name:           "binary with curl symbols",
            filename:       "with_curl.elf",
            expectedResult: NetworkDetected,
            expectSymbols:  true,
            skipIfMissing:  true,
        },
        {
            name:           "binary with ssl symbols",
            filename:       "with_ssl.elf",
            expectedResult: NetworkDetected,
            expectSymbols:  true,
            skipIfMissing:  true,
        },
        {
            name:           "binary without network symbols",
            filename:       "no_network.elf",
            expectedResult: NoNetworkSymbols,
            expectSymbols:  false,
            skipIfMissing:  true,
        },
        {
            name:           "static binary",
            filename:       "static.elf",
            expectedResult: StaticBinary,
            expectSymbols:  false,
            skipIfMissing:  true,
        },
        {
            name:           "shell script (non-ELF)",
            filename:       "script.sh",
            expectedResult: NotELFBinary,
            expectSymbols:  false,
            skipIfMissing:  true,
        },
        {
            name:           "corrupted ELF",
            filename:       "corrupted.elf",
            expectedResult: AnalysisError,
            expectSymbols:  false,
            skipIfMissing:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            path := filepath.Join(testdataDir, tt.filename)
            if _, err := os.Stat(path); os.IsNotExist(err) {
                if tt.skipIfMissing {
                    t.Skipf("test file %s not found", tt.filename)
                }
                t.Fatalf("required test file %s not found", tt.filename)
            }

            output := analyzer.AnalyzeNetworkSymbols(path)

            assert.Equal(t, tt.expectedResult, output.Result,
                "unexpected result for %s", tt.filename)

            if tt.expectSymbols {
                assert.NotEmpty(t, output.DetectedSymbols,
                    "expected symbols for %s", tt.filename)
            } else {
                assert.Empty(t, output.DetectedSymbols,
                    "unexpected symbols for %s", tt.filename)
            }

            if tt.expectedResult == AnalysisError {
                assert.NotNil(t, output.Error,
                    "expected error for %s", tt.filename)
            }
        })
    }
}

func TestStandardELFAnalyzer_NonexistentFile(t *testing.T) {
    analyzer := NewStandardELFAnalyzer(nil)

    output := analyzer.AnalyzeNetworkSymbols("/nonexistent/path/to/binary")

    assert.Equal(t, AnalysisError, output.Result)
    assert.NotNil(t, output.Error)
}

func TestAnalysisOutput_IsNetworkCapable(t *testing.T) {
    tests := []struct {
        result   AnalysisResult
        expected bool
    }{
        {NetworkDetected, true},
        {NoNetworkSymbols, false},
        {NotELFBinary, false},
        {StaticBinary, false},
        {AnalysisError, false},
    }

    for _, tt := range tests {
        t.Run(tt.result.String(), func(t *testing.T) {
            output := AnalysisOutput{Result: tt.result}
            assert.Equal(t, tt.expected, output.IsNetworkCapable())
        })
    }
}

func TestAnalysisOutput_IsIndeterminate(t *testing.T) {
    tests := []struct {
        result   AnalysisResult
        expected bool
    }{
        {NetworkDetected, false},
        {NoNetworkSymbols, false},
        {NotELFBinary, false},
        {StaticBinary, true},
        {AnalysisError, true},
    }

    for _, tt := range tests {
        t.Run(tt.result.String(), func(t *testing.T) {
            output := AnalysisOutput{Result: tt.result}
            assert.Equal(t, tt.expected, output.IsIndeterminate())
        })
    }
}

func TestIsNetworkSymbol(t *testing.T) {
    tests := []struct {
        symbol   string
        expected bool
        category SymbolCategory
    }{
        {"socket", true, CategorySocket},
        {"connect", true, CategorySocket},
        {"curl_easy_init", true, CategoryHTTP},
        {"SSL_connect", true, CategoryTLS},
        {"getaddrinfo", true, CategoryDNS},
        {"printf", false, ""},
        {"malloc", false, ""},
        {"unknown_function", false, ""},
    }

    for _, tt := range tests {
        t.Run(tt.symbol, func(t *testing.T) {
            cat, found := IsNetworkSymbol(tt.symbol)
            assert.Equal(t, tt.expected, found)
            if tt.expected {
                assert.Equal(t, tt.category, cat)
            }
        })
    }
}

func TestSymbolCount(t *testing.T) {
    count := SymbolCount()
    // Ensure we have a reasonable number of symbols registered
    assert.Greater(t, count, 30, "expected at least 30 registered symbols")
}
```

### 6.3 統合テスト

```go
// command_analysis_test.go に追加

func TestIsNetworkOperation_ELFAnalysis(t *testing.T) {
    // Save and restore original analyzer
    originalAnalyzer := defaultELFAnalyzer
    defer func() { defaultELFAnalyzer = originalAnalyzer }()

    tests := []struct {
        name            string
        cmdName         string
        args            []string
        mockResult      elfanalyzer.AnalysisResult
        expectedNetwork bool
        expectedRisk    bool
    }{
        {
            name:            "profile command bypasses ELF analysis",
            cmdName:         "curl",
            args:            []string{"http://example.com"},
            mockResult:      elfanalyzer.NoNetworkSymbols, // Should not be used
            expectedNetwork: true,
            expectedRisk:    false,
        },
        {
            name:            "unknown command with network symbols",
            cmdName:         "custom_downloader",
            args:            []string{},
            mockResult:      elfanalyzer.NetworkDetected,
            expectedNetwork: true,
            expectedRisk:    false,
        },
        {
            name:            "unknown command without network symbols",
            cmdName:         "custom_tool",
            args:            []string{},
            mockResult:      elfanalyzer.NoNetworkSymbols,
            expectedNetwork: false,
            expectedRisk:    false,
        },
        {
            name:            "unknown command analysis error (middle risk)",
            cmdName:         "broken_binary",
            args:            []string{},
            mockResult:      elfanalyzer.AnalysisError,
            expectedNetwork: true, // Safety-first
            expectedRisk:    false,
        },
        {
            name:            "static binary falls back to arg detection",
            cmdName:         "go_tool",
            args:            []string{"http://example.com"},
            mockResult:      elfanalyzer.StaticBinary,
            expectedNetwork: true, // URL in args
            expectedRisk:    false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Set up mock analyzer
            SetELFAnalyzer(&mockELFAnalyzer{result: tt.mockResult})

            isNetwork, isRisk := IsNetworkOperation(tt.cmdName, tt.args)

            assert.Equal(t, tt.expectedNetwork, isNetwork,
                "unexpected network result")
            assert.Equal(t, tt.expectedRisk, isRisk,
                "unexpected risk result")
        })
    }
}

// mockELFAnalyzer implements elfanalyzer.ELFAnalyzer for testing
type mockELFAnalyzer struct {
    result elfanalyzer.AnalysisResult
}

func (m *mockELFAnalyzer) AnalyzeNetworkSymbols(path string) elfanalyzer.AnalysisOutput {
    output := elfanalyzer.AnalysisOutput{Result: m.result}
    if m.result == elfanalyzer.NetworkDetected {
        output.DetectedSymbols = []elfanalyzer.DetectedSymbol{
            {Name: "socket", Category: "socket"},
        }
    }
    if m.result == elfanalyzer.AnalysisError {
        output.Error = errors.New("mock analysis error")
    }
    return output
}
```

## 7. 受け入れ条件のトレーサビリティ

| 受け入れ条件 | 実装箇所 | テスト |
|------------|---------|--------|
| AC-1: ELF バイナリの判定 | `isELFMagic()` | `TestStandardELFAnalyzer_AnalyzeNetworkSymbols` |
| AC-2: ネットワークシンボルの検出 | `AnalyzeNetworkSymbols()` | `with_socket.elf`, `no_network.elf` テスト |
| AC-3: HTTP/TLS ライブラリの検出 | `networkSymbolRegistry` | `with_curl.elf`, `with_ssl.elf` テスト |
| AC-4: フォールバック動作 | `IsNetworkOperation()` | `TestIsNetworkOperation_ELFAnalysis` |
| AC-5: 静的リンクバイナリの扱い | `StaticBinary` result | `static.elf` テスト |
| AC-6: 解析失敗時の安全性 | `AnalysisError` handling | `corrupted.elf` テスト |
| AC-7: 既存機能への非影響 | Profile priority check | `TestIsNetworkOperation` 既存テスト |

## 8. 実装順序

### Phase 0: 前提条件（safefileio パッケージの拡張）

**重要**: 本タスクの実装前に、`internal/safefileio/safe_file.go` の `File` インターフェースに `io.ReaderAt` を追加する必要があります。

```go
// File is an interface that abstracts file operations
type File interface {
    io.Reader
    io.Writer
    io.ReaderAt  // 追加: debug/elf.NewFile との互換性のため
    Close() error
    Stat() (os.FileInfo, error)
    Truncate(size int64) error
}
```

`*os.File` は既に `io.ReaderAt` を実装しているため、実装側の変更は不要です。

### Phase 1: 基盤（elfanalyzer パッケージ）

1. `analyzer.go` - インターフェースと型定義
2. `network_symbols.go` - シンボルレジストリ
3. `analyzer_impl.go` - StandardELFAnalyzer 実装
4. テストフィクスチャの生成
5. `analyzer_test.go` - ユニットテスト

### Phase 2: 統合（security パッケージ）

1. `command_analysis.go` への ELF 解析統合
2. `command_analysis_test.go` への統合テスト追加
3. 既存テストの動作確認

### Phase 3: 検証

1. 全既存テストのパス確認
2. 新規テストのパス確認
3. `make test` の成功確認
4. サンプルコマンドでの動作検証
