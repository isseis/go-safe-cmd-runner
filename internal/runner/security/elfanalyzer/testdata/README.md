# Test Fixtures for ELF Analyzer

## Overview

This directory contains test binaries for the elfanalyzer package unit tests.
Binaries are NOT checked into Git (see `.gitignore`). They must be generated locally.

## Prerequisites

- GCC (for dynamic binaries)
- GCC with `-static` support (for static binaries)
- libssl-dev (for OpenSSL test binary)
- libcurl4-openssl-dev (optional, for libcurl test binary)

## Generation Instructions

### 1. Binary with socket API symbols (`with_socket.elf`)

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

### 2. Binary with libcurl (`with_curl.elf`)

Requires `libcurl4-openssl-dev`.

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

### 3. Binary with OpenSSL (`with_ssl.elf`)

Requires `libssl-dev`.

```bash
cat > /tmp/with_ssl.c << 'EOF'
#include <openssl/ssl.h>
int main() {
    SSL_CTX *ctx = SSL_CTX_new(TLS_client_method());
    SSL_CTX_free(ctx);
    return 0;
}
EOF
gcc -o with_ssl.elf /tmp/with_ssl.c -lssl -lcrypto
```

### 4. Binary without network symbols (`no_network.elf`)

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

### 5. Statically linked binary (`static.elf`)

```bash
gcc -static -o static.elf /tmp/no_network.c
```

### 6. Shell script (`script.sh`)

```bash
echo '#!/bin/bash' > script.sh
echo 'echo "Hello"' >> script.sh
chmod +x script.sh
```

### 7. Corrupted ELF (`corrupted.elf`)

```bash
printf '\x7fELF' > corrupted.elf
dd if=/dev/urandom bs=100 count=1 >> corrupted.elf 2>/dev/null
```

## Test Binary Summary

| File | Type | Expected Result |
|------|------|-----------------|
| `with_socket.elf` | Dynamic, socket API | `NetworkDetected` |
| `with_curl.elf` | Dynamic, libcurl | `NetworkDetected` |
| `with_ssl.elf` | Dynamic, OpenSSL | `NetworkDetected` |
| `no_network.elf` | Dynamic, no network | `NoNetworkSymbols` |
| `static.elf` | Static | `StaticBinary` |
| `script.sh` | Shell script | `NotELFBinary` |
| `corrupted.elf` | Corrupted ELF | `AnalysisError` |
