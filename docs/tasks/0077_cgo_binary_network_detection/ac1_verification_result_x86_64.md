# AC-1 事前検証結果（x86_64/Linux）

## サマリー

| 検証項目 | 想定結果 | 実測結果 | 合否 |
|---------|---------|---------|------|
| `.dynsym` 解析（`AnalyzeNetworkSymbols`）の結果 | `NoNetworkSymbols` | `NoNetworkSymbols` | ✅ 合格 |
| `SyscallAnalyzer` による `socket` 検出 | `HasNetworkSyscalls: true` | `HasNetworkSyscalls: false`、`IsHighRisk: true` | ❌ 不合格（ただし安全方向） |

**結論:**
`SyscallAnalyzer` は x86_64/Linux 上の CGO バイナリに対して `socket` システムコールの検出に**失敗**するが、代わりに `IsHighRisk: true`（`AnalysisError` 相当）を返す。これは要件定義書 §4.2 に定義された安全方向フェイルセーフの挙動であり、CGO バイナリが誤って許可されることはない。AC-1 の想定する「盲点の再現」は確認されたが、`SyscallAnalyzer` が `HasNetworkSyscalls: true` を返すという期待は満たされなかった。

arm64 と同じ最終結果（`IsHighRisk: true`）だが、その**根本原因は arm64 と異なる**（§5 参照）。

---

## 1. 検証環境

| 項目 | 値 |
|------|---|
| OS | Linux 6.8.0-101-generic |
| アーキテクチャ | x86_64 (amd64) |
| Go バージョン | go1.26.0 linux/amd64 |
| CGO_ENABLED | 1 |
| バイナリ種別 | 動的リンク ELF (EM_X86_64) |
| バイナリサイズ | 約 1.8 MB |
| リンクする共有ライブラリ | libc.so.6 ほか（pthread 等） |

---

## 2. 検証手順

### 手順 1: CGO バイナリのビルド

要件定義書 §6.1（FR-3.1.1 手順 1）記載のサンプルコードをビルドした。

```bash
mkdir -p /tmp/ac1_verify_x86
# main.go を作成（§3 参照）
CGO_ENABLED=1 go build -o /tmp/ac1_verify_x86/cgo_test /tmp/ac1_verify_x86/main.go
```

ビルド結果:
```
cgo_test: ELF 64-bit LSB executable, x86-64, version 1 (SYSV),
  dynamically linked, interpreter /lib64/ld-linux-x86-64.so.2,
  BuildID[sha1]=41b3ae6a725e629471342be85a0c99364005e37a, not stripped
Size: 1,882,792 bytes
```

### 手順 2: `.dynsym` 解析（`AnalyzeNetworkSymbols` 相当）

```bash
readelf --dyn-syms /tmp/ac1_verify_x86/cgo_test
```

出力（51 エントリ中にネットワーク関連シンボルなし）:
```
Symbol table '.dynsym' contains 51 entries:
  # socket, connect, bind, sendto, recvfrom, getaddrinfo 等のシンボルは存在しない
  # libc の mmap, setuid, pthread_create 等は存在する
  # CGO 固有のシンボル: _cgo_panic, _cgo_topofstack, crosscall2
```

→ `AnalyzeNetworkSymbols()` 戻り値: **`NoNetworkSymbols`**

### 手順 3: `SyscallAnalyzer` による syscall 解析

Go のテストとして `SyscallAnalyzer.AnalyzeSyscallsFromELF()` を実行した（統合テストビルドタグ `integration` を使用）。

```bash
go test -tags "test integration" -v -run TestAC1_CgoBinaryNetworkDetection_x86_64 \
  ./internal/runner/security/elfanalyzer/
```

---

## 3. 使用したソースコード

### テスト対象 CGO バイナリ（要件定義書 §6.1 のサンプルコード）

```go
// main.go（CGO バイナリ用テスト）
// CGO_ENABLED=1 でビルドされるが、ネットワーク syscall は Go ランタイムが直接発行する
package main

import "C" // CGO を有効にして動的バイナリにする（libc をリンクさせる）

import "syscall"

func main() {
    // syscall.Socket を直接呼ぶことで Go ランタイムが SYSCALL 命令を直接発行する。
    // .dynsym には "socket" シンボルは現れない。
    fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
    if err == nil {
        _ = syscall.Close(fd)
    }
}
```

### AC-1 検証テスト

```go
//go:build integration

package elfanalyzer

func TestAC1_CgoBinaryNetworkDetection_x86_64(t *testing.T) {
    // CGO バイナリをビルドして一時ディレクトリに保存
    // 1. .dynsym にネットワークシンボルがないことを確認
    // 2. SyscallAnalyzer.AnalyzeSyscallsFromELF() の結果を確認
}
```

---

## 4. 実行結果

### 4.1 `.dynsym` 解析の結果

```
=== RUN   TestAC1_CgoBinaryNetworkDetection_x86_64/dynsym_returns_NoNetworkSymbols
    AnalyzeNetworkSymbols result: no_network_symbols
--- PASS
```

**評価:** `.dynsym` に `socket`, `connect` 等のネットワークシンボルが存在しない。これが本タスク（0077）が修正しようとしている「盲点」の再現である。

### 4.2 `SyscallAnalyzer` の結果

```
=== RUN   TestAC1_CgoBinaryNetworkDetection_x86_64/syscall_analysis_detects_network
    SyscallAnalysis architecture: x86_64
    TotalDetectedEvents: 38
    NetworkSyscallCount: 0
    HasNetworkSyscalls: false
    IsHighRisk: true
    HasUnknownSyscalls: true

    Syscall[0]:  #-1  ()              method=unknown:control_flow_boundary at 0x40950c
    Syscall[1]:  #231 (exit_group)    method=immediate                     at 0x47afc9
    Syscall[2]:  #60  (exit)          method=immediate                     at 0x47aff5
    Syscall[3]:  #257 ()              method=immediate                     at 0x47b018
    Syscall[4]:  #3   (close)         method=immediate                     at 0x47b049
    Syscall[5]:  #1   (write)         method=immediate                     at 0x47b073
    Syscall[6]:  #0   (read)          method=immediate                     at 0x47b092
    ...（合計 38 件）
    Syscall[37]: #2   (open)          method=go_wrapper                    at 0x47e4c0

--- FAIL: HasNetworkSyscalls should be true
```

`socket`（x86_64 syscall #41）は検出されなかった。

Pass 2（GoWrapperResolver）は `0x47e4c0` で `open`(#2) を `go_wrapper` として検出したが、これは後述の pclntab アドレスずれによる誤検出である（§5.3 参照）。

---

## 5. 詳細分析

### 5.1 なぜ `socket` syscall が検出されないのか

x86_64 での `syscall.Socket()` の実行パスを逆アセンブルすると以下の通りである:

```
main()
  → syscall.Socket()
      → syscall.socket()                 [0x47e000]
          MOV $0x29, %eax  (eax = 41 = SYS_SOCKET)
          CALL syscall.RawSyscall        [0x47e0e0]
            → syscall.RawSyscall6        [0x47e100]
              → internal/runtime/syscall/linux.Syscall6  [0x409500]
                  SYSCALL                [0x40950c]  ← 実際のシステムコール命令
```

**Pass 1（直接 SYSCALL スキャン）の判定:**
`SYSCALL`（`0x40950c`）は `internal/runtime/syscall/linux.Syscall6` の本体内にある。このシンボルは `knownSyscallImpls` に登録されているため `IsInsideWrapper` で除外される。これは正しい設計であり、`HighRiskReasons` には記録されない。

**Pass 2（GoWrapperResolver）が補完できなかった理由:**
CGO バイナリでは `.gopclntab` から取得した関数アドレスと実際の仮想アドレスの間に **256 バイト（0x100）のずれ**が生じる（§5.2 参照）。このため `wrapperAddrs` に登録されたアドレスが実際の `syscall.RawSyscall` のアドレスと一致せず、Pass 2 は `syscall.socket` 内の `CALL syscall.RawSyscall` を検出できない。

### 5.2 pclntab アドレスずれ（x86_64 CGO バイナリ固有の問題）

CGO バイナリの `.gopclntab` から `gosym.NewLineTable` で取得した関数アドレスと、実際の `.symtab` アドレスを比較した結果:

| シンボル | `.symtab` 実際のアドレス | pclntab が返すアドレス | ずれ |
|---------|----------------------|---------------------|------|
| `syscall.socket` | `0x47e000` | `0x47df00` | −0x100 |
| `syscall.RawSyscall` | `0x47e0e0` | `0x47dfe0` | −0x100 |
| `syscall.RawSyscall6` | `0x47e100` | `0x47e000` | −0x100 |
| `syscall.Syscall` | `0x47e120` | `0x47e020` | −0x100 |

`ParsePclntab` は `textSection.Addr`（`0x402300`）を `gosym.NewLineTable` の第2引数（textStart）として渡しているが、CGO バイナリでは `.text` セクション先頭に C ランタイム（crt0等）由来のコードが約 256 バイト含まれるため、Go ランタイム関数の実際の配置アドレスが pclntab の記録と一致しなくなる。

その結果:
- `wrapperAddrs[0x47dfe0]` = `"syscall.RawSyscall"` として登録（正しいアドレスは `0x47e0e0`）
- `wrapperAddrs[0x47e000]` = `"syscall.RawSyscall6"` として誤登録（実際は `syscall.socket` の先頭）

### 5.3 `go_wrapper` 誤検出（`open` #2）の説明

`0x47e4c0: CALL 0x47e000` は実際には `syscall.socket` を呼んでいるが、pclntab のずれにより `wrapperAddrs[0x47e000] = "syscall.RawSyscall6"` と誤って登録されているため、Pass 2 はこれを「`syscall.RawSyscall6` へのラッパー呼び出し」と誤解釈する。直前の命令 `MOV $0x2, %eax`（eax = 2 = SYS_OPEN）を読み取り、`open`(#2) として記録する。

### 5.4 arm64 との比較

| 項目 | arm64 | x86_64 |
|------|-------|--------|
| Pass 1 の `socket` 検出失敗理由 | `unknown:indirect_setting`（スタック経由のレジスタロード）。`knownSyscallImpls` のシンボル名不一致により `SVC #0` が除外されずスキャン対象に残ることも一因 | `IsInsideWrapper` 除外（`knownSyscallImpls` に登録済み） |
| Pass 2 の失敗理由 | 原因未特定（`BL syscall.RawSyscall` が GoWrapperResolver で解決されず） | pclntab アドレスと実際の仮想アドレスのずれ（CGO バイナリ固有） |
| 最終判定 | `IsHighRisk: true`、`HasNetworkSyscalls: false` | `IsHighRisk: true`、`HasNetworkSyscalls: false` |
| 安全方向フェイルセーフ | ✅ 動作 | ✅ 動作 |

### 5.5 `IsHighRisk: true` の意味

`IsHighRisk: true` は `convertSyscallResult()` において `AnalysisError` に変換される（要件定義書 §3.2.2 参照）。これは「unknown syscalls が存在する場合は安全方向のフェイルセーフ」という設計通りの挙動であり、**ネットワークアクセスを誤って許可しない**という点では問題ない。

ただし、`HasNetworkSyscalls: true` を返すという AC-1 の期待は満たされなかった。代わりに `AnalysisError` が返るため、CGO バイナリは「高リスク」扱いとなり実行が禁止される。

---

## 6. 検証結果の要件定義書への記録

要件定義書 §8「未解決事項」に記録すべき内容:

> **検証結果（AC-1、x86_64）:**
> - `.dynsym` 解析が `NoNetworkSymbols` を返すことは確認された（盲点の再現）✅
> - `SyscallAnalysis` は `HasNetworkSyscalls: true` を返さず、代わりに `IsHighRisk: true` を返した
>   - 検出されたシステムコール: `exit_group`(231), `exit`(60), `close`(3), `write`(1), `read`(0), `mmap`(9), `munmap`(11) 等
>   - `socket`(41) は未検出（CGO バイナリ固有の pclntab アドレスずれにより Pass 2 が機能せず）
>   - これは §4.2 の「安全方向フェイルセーフ」として想定された挙動
> - 最終結果は arm64 と同じ（`IsHighRisk: true`）だが根本原因が異なる
>
> → AC-1 の 2 番目の条件（`HasNetworkSyscalls: true`）は満たされないが、
>   安全方向（`AnalysisError`）には倒れることを確認した。
>   タスク 0077 の実装では x86_64 の CGO バイナリに対する pclntab アドレスずれ対応も必要と考えられる。
