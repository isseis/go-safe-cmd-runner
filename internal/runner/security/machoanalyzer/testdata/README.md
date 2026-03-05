# machoanalyzer テストフィクスチャ

このディレクトリには `machoanalyzer` パッケージのテストに使用するバイナリフィクスチャが含まれる。
macOS 環境（arm64）で生成したバイナリをリポジトリに含めることで、
macOS SDK が存在しない環境でも CI でのビルド検証が可能になる。

## フィクスチャ一覧

| ファイル | 種別 | 説明 |
|--------|------|------|
| `network_macho_arm64` | arm64 Mach-O (C) | `socket(AF_INET, ...)` をインポートする arm64 C バイナリ |
| `no_network_macho_arm64` | arm64 Mach-O (C) | ネットワークシンボルなし arm64 C バイナリ |
| `svc_only_arm64` | arm64 Mach-O (asm) | `svc #0x80` 命令のみを含む最小 arm64 バイナリ |
| `fat_binary` | Fat バイナリ | arm64 + x86_64 のネットワーク使用バイナリ |
| `network_macho_x86_64` | x86_64 Mach-O (C) | `fat_binary` 生成用の x86_64 中間ファイル |
| `network_go_macho_arm64` | arm64 Mach-O (Go) | `net.Dial` を使用する Go バイナリ |
| `no_network_go_arm64` | arm64 Mach-O (Go) | ネットワーク操作なし Go バイナリ |
| `script.sh` | テキスト | 非 Mach-O ファイル（シェルスクリプト） |
| `network_go/main.go` | Go ソース | `network_go_macho_arm64` の生成元ソース |
| `no_network_go/main.go` | Go ソース | `no_network_go_arm64` の生成元ソース |

## 再生成コマンド

フィクスチャを再生成する場合は、macOS 環境で以下のコマンドを実行する。

### network_macho_arm64

```bash
cat > /tmp/network.c << 'EOF'
#include <sys/socket.h>
int main() { return socket(AF_INET, SOCK_STREAM, 0); }
EOF
cc -target arm64-apple-macos11 /tmp/network.c \
    -o internal/runner/security/machoanalyzer/testdata/network_macho_arm64
```

### no_network_macho_arm64

```bash
cat > /tmp/no_network.c << 'EOF'
#include <stdio.h>
int main() { return 0; }
EOF
cc -target arm64-apple-macos11 /tmp/no_network.c \
    -o internal/runner/security/machoanalyzer/testdata/no_network_macho_arm64
```

### svc_only_arm64

```bash
cat > /tmp/svc_only.s << 'EOF'
.section __TEXT,__text
.globl _main
_main:
    .long 0xd4001001 /* svc #0x80 */
    ret
EOF
as -arch arm64 /tmp/svc_only.s -o /tmp/svc_only.o
ld -o internal/runner/security/machoanalyzer/testdata/svc_only_arm64 \
    /tmp/svc_only.o -lSystem -syslibroot $(xcrun --sdk macosx --show-sdk-path) \
    -arch arm64
```

### fat_binary（arm64 + x86_64）

```bash
# /tmp/network.c は network_macho_arm64 の生成時に作成済み
cc -target x86_64-apple-macos11 /tmp/network.c \
    -o internal/runner/security/machoanalyzer/testdata/network_macho_x86_64
lipo -create \
    internal/runner/security/machoanalyzer/testdata/network_macho_arm64 \
    internal/runner/security/machoanalyzer/testdata/network_macho_x86_64 \
    -output internal/runner/security/machoanalyzer/testdata/fat_binary
```

### network_go_macho_arm64

```bash
GOOS=darwin GOARCH=arm64 go build \
    -o internal/runner/security/machoanalyzer/testdata/network_go_macho_arm64 \
    ./internal/runner/security/machoanalyzer/testdata/network_go/
```

### no_network_go_arm64

```bash
GOOS=darwin GOARCH=arm64 go build \
    -o internal/runner/security/machoanalyzer/testdata/no_network_go_arm64 \
    ./internal/runner/security/machoanalyzer/testdata/no_network_go/
```

### script.sh

```bash
echo '#!/bin/sh' > \
    internal/runner/security/machoanalyzer/testdata/script.sh
```
