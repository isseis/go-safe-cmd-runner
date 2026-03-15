# AC-1 事前検証結果

## サマリー

| 検証項目 | 想定結果 | 実測結果 | 合否 |
|---------|---------|---------|------|
| `.dynsym` 解析（`AnalyzeNetworkSymbols`）の結果 | `NoNetworkSymbols` | `NoNetworkSymbols` | ✅ 合格 |
| `SyscallAnalyzer` による `socket` 検出（初回） | `HasNetworkSyscalls: true` | `HasNetworkSyscalls: false`、`IsHighRisk: true` | ❌ 不合格（ただし安全方向） |
| `SyscallAnalyzer` による `socket` 検出（Pass 1/2 修正後） | `HasNetworkSyscalls: true` | `HasNetworkSyscalls: true`、`socket`(#198) 検出 | ✅ 合格 |

**結論（初回検証）:**
`SyscallAnalyzer` は CGO バイナリに対して `socket` システムコールの検出には**失敗**するが、代わりに `IsHighRisk: true`（`AnalysisError`相当）を返す。これは要件定義書 §4.2 に定義された安全方向フェイルセーフの挙動であり、期待通りの設計内動作である。

**結論（修正後検証 §7 参照）:**
`knownSyscallImpls` のシンボル名を `"internal/runtime/syscall.Syscall6"` に修正（pclntab の命名規則に合わせ `.abi0` サフィックスを除去）することで、Pass 1 の誤除外漏れが解消し、Pass 2 も `socket`(#198) を `go_wrapper` として正しく検出できるようになった。**AC-1 の全条件が充足された。**

---

## 1. 検証環境

| 項目 | 値 |
|------|---|
| OS | Linux 6.8.0-90-generic |
| アーキテクチャ | arm64 (aarch64) |
| Go バージョン | go1.26.0 linux/arm64 |
| CGO_ENABLED | 1 |
| バイナリ種別 | 動的リンク ELF (EM_AARCH64) |
| バイナリサイズ | 約 1.8 MB |
| リンクする共有ライブラリ | libc.so.6 |

---

## 2. 検証手順

### 手順 1: CGO バイナリのビルド

要件定義書 §6.1（FR-3.1.1 手順 1）記載のサンプルコードをビルドした。

```bash
mkdir -p /tmp/ac1_verify
# main.go を作成（§3 参照）
CGO_ENABLED=1 go build -o /tmp/ac1_verify/cgo_test /tmp/ac1_verify/main.go
```

ビルド結果:
```
cgo_test: ELF 64-bit LSB executable, ARM aarch64, version 1 (SYSV),
  dynamically linked, interpreter /lib/ld-linux-aarch64.so.1,
  BuildID[sha1]=ea10a71ab4855261162528e2d0e38fa81174c4a6, not stripped
Size: 1,672,368 bytes
```

### 手順 2: `.dynsym` 解析（`AnalyzeNetworkSymbols` 相当）

`AnalyzeNetworkSymbols()` を `elfanalyzer.StandardELFAnalyzer` から呼び出し、結果を確認した。

```bash
readelf --dyn-syms /tmp/ac1_verify/cgo_test
```

出力（52エントリ中にネットワーク関連シンボルなし）:
```
Symbol table '.dynsym' contains 52 entries:
  # socket, connect, bind, sendto, recvfrom, getaddrinfo 等のシンボルは存在しない
  # libc の setuid, malloc, mmap 等は存在する
```

→ `AnalyzeNetworkSymbols()` 戻り値: **`NoNetworkSymbols`**

### 手順 3: `SyscallAnalyzer` による syscall 解析

Go のテストとして `SyscallAnalyzer.AnalyzeSyscallsFromELF()` を実行した（統合テストビルドタグ `integration` を使用）。

```bash
go test -tags "test integration" -v -run TestAC1_CgoBinaryNetworkDetection \
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
    // syscall.Socket を直接呼ぶことで Go ランタイムが SYS_SOCKET を直接発行する。
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

func TestAC1_CgoBinaryNetworkDetection(t *testing.T) {
    // CGO バイナリをビルドして一時ディレクトリに保存
    // 1. AnalyzeNetworkSymbols() が NoNetworkSymbols を返すことを確認
    // 2. SyscallAnalyzer.AnalyzeSyscallsFromELF() の結果を確認
}
```

---

## 4. 実行結果

### 4.1 `.dynsym` 解析の結果

```
=== RUN   TestAC1_CgoBinaryNetworkDetection/dynsym_returns_NoNetworkSymbols
    AnalyzeNetworkSymbols result: no_network_symbols
--- PASS
```

**評価:** `.dynsym` に `socket`, `connect` 等のネットワークシンボルが存在しない。`AnalyzeNetworkSymbols()` は正しく `NoNetworkSymbols` を返した。これが本タスク（0077）が修正しようとしている「盲点」の再現である。

### 4.2 `SyscallAnalyzer` の結果

```
=== RUN   TestAC1_CgoBinaryNetworkDetection/syscall_analysis_detects_network
    SyscallAnalysis architecture: arm64
    TotalDetectedEvents: 34
    NetworkSyscallCount: 0
    HasNetworkSyscalls: false
    IsHighRisk: true
    HasUnknownSyscalls: true

    Syscall[0]:  #-1 () isNetwork=false method=unknown:indirect_setting at 0x40746c
    Syscall[1]:  #94  (exit_group)  method=immediate at 0x473f38
    Syscall[2]:  #93  (exit)        method=immediate at 0x473f54
    Syscall[3]:  #-1  ()            method=unknown:control_flow_boundary at 0x473f74
    Syscall[4]:  #57  (close)       method=immediate at 0x473f98
    ...（合計 34 件）
--- FAIL: HasNetworkSyscalls should be true
```

`socket`（arm64 syscall #198）は検出されなかった。

---

## 5. 詳細分析

### 5.1 なぜ `socket` syscall が検出されないのか

arm64 での `syscall.Socket()` の実行パスを逆アセンブルすると以下の通りである:

```
main()
  → syscall.socket()                  [0x46fa80]
      MOV x0, #198  (x0 に syscall 番号をセット)
      BL  syscall.RawSyscall          [0x46fb60]
        → BL syscall.RawSyscall6     [0x46fb90]
          → BL internal/runtime/syscall.Syscall6.abi0  [0x402cd0]
              LDR x8, [sp, #8]       ← スタックから x8 にロード（indirect）
              LDR x0, [sp, #16]
              ...
              SVC #0                 [0x402cec]  ← 実際の syscall 命令
```

**Pass 1（直接 SVC スキャン）の判定:**
`SVC #0`（`0x402cec`）の直前命令は `LDR x8, [sp, #8]`（メモリからのロード）である。`SyscallAnalyzer` の後方スキャンはこの `LDR` を検出し、syscall 番号をレジスタ（`x8`）に直接書き込む `MOV` ではなくメモリ参照であるため、`unknown:indirect_setting` と判定する。これが `IsHighRisk: true` の直接の原因である。

**Pass 2（GoWrapperResolver）が補完できなかった理由:**
`syscall.socket()` 内での `MOV x0, #198; BL syscall.RawSyscall` パターンは、GoWrapperResolver が解析対象とする「ラッパー関数への呼び出し」に該当する。Pass 2 がこれを検出・解決できなかった原因は未特定であり、引き続き調査が必要である（詳細は §5.4 参照）。

なお `knownSyscallImpls` のシンボル名不一致（§5.2）は Pass 2 の問題ではなく Pass 1 の問題である。`internal/runtime/syscall.Syscall6.abi0` が `knownSyscallImpls` に未登録であることで、この関数内の `SVC #0` 命令が Pass 1 で除外されず `unknown:indirect_setting` と判定される。これが `IsHighRisk: true` の直接の原因である。

### 5.2 実際のシンボル名の差異

| 想定シンボル名（`knownSyscallImpls`） | 実際のシンボル名 |
|--------------------------------------|-----------------|
| `internal/runtime/syscall/linux.Syscall6` | `internal/runtime/syscall.Syscall6.abi0` |

Go 1.26（arm64）では `internal/runtime/syscall.Syscall6.abi0` というシンボル名が使われており、`/linux` サブパッケージの命名は見当たらない。

### 5.3 `IsHighRisk: true` の意味

`IsHighRisk: true` は `convertSyscallResult()` において `AnalysisError` に変換される（要件定義書 §3.2.2 参照）。これは「unknown syscalls が存在する場合は安全方向のフェイルセーフ」という設計通りの挙動であり、**ネットワークアクセスを誤って許可しない**という点では問題ない。

ただし、`HasNetworkSyscalls: true` を返すという AC-1 の期待は満たされなかった。代わりに `AnalysisError` が返るため、CGO バイナリは「高リスク」扱いとなり実行が禁止される。

### 5.4 GoWrapperResolver による解析（Pass 2）の評価

`SyscallAnalyzer` の Pass 2（GoWrapperResolver）は `syscall.socket` 内の `BL syscall.RawSyscall` 呼び出しを検出し、直前の `MOV x0, #198` から syscall 番号を解決しようとする。しかし今回の結果では Pass 2 による `go_wrapper` 判定は発生しておらず、syscall の検出には Pass 1（直接スキャン）の結果のみが寄与している。

Pass 2 が機能しなかった原因は未特定であり、実装フェーズでの調査が必要である。考えられる仮説:
- pclntab から取得した `syscall.RawSyscall` のアドレスが実際の仮想アドレスとずれており、`wrapperAddrs` へのルックアップが失敗している（x86_64 CGO バイナリで確認された pclntab アドレスずれと同種の問題）
- あるいは `syscall.socket` 自体が何らかの理由で解析対象から除外されている

---

## 6. 検証結果の要件定義書への記録

要件定義書 §8「未解決事項」に記録すべき内容:

> **検証結果（AC-1）:**
> - `.dynsym` 解析が `NoNetworkSymbols` を返すことは確認された（盲点の再現）✅
> - `SyscallAnalysis` は `HasNetworkSyscalls: true` を返さず、代わりに `IsHighRisk: true` を返した
>   - 検出されたシステムコール: `exit_group`(94), `exit`(93), `close`(57), `mmap`(222), `munmap`(215) 等
>   - `socket`(198) は未検出（`unknown:indirect_setting`による `IsHighRisk`）
>   - これは §4.2 の「安全方向フェイルセーフ」として想定された挙動
> - CGO バイナリは現時点では `AnalysisError`（高リスク）扱いとなる
>
> → AC-1 の 2 番目の条件（`HasNetworkSyscalls: true`）は満たされないが、
>   安全方向（`AnalysisError`）には倒れることを確認した。
>   タスク 0077 の実装では以下の 2 点の対応が必要と考えられる:
>   1. **Pass 1 の修正**: `knownSyscallImpls` に arm64 実環境のシンボル名
>      （`internal/runtime/syscall.Syscall6.abi0`）を追加し、`unknown:indirect_setting`
>      に起因する `IsHighRisk: true` を解消する。
>   2. **Pass 2 の修正**: `syscall.socket` 内の `BL syscall.RawSyscall` が
>      解決されなかった原因を調査・修正し、`HasNetworkSyscalls: true` を返せるようにする。

---

## 7. AC-1 第三条件の検証（Pass 1/2 修正後）

### 7.1 修正内容

**Pass 1 修正（2026-03-15）:**

`knownSyscallImpls` のシンボル名を修正した（[go_wrapper_resolver.go](../../../../../internal/runner/security/elfanalyzer/go_wrapper_resolver.go)）。

| 変更前 | 変更後 |
|--------|--------|
| `"internal/runtime/syscall.Syscall6.abi0"` | `"internal/runtime/syscall.Syscall6"` |

**根本原因:** `loadFromPclntab` は `.gopclntab` セクションからシンボルを読み取るが、pclntab は ABI サフィックス（`.abi0`）なしのシンボル名を記録する。ELF `.symtab` では `"internal/runtime/syscall.Syscall6.abi0"` として現れるが、pclntab では `"internal/runtime/syscall.Syscall6"` として現れる。以前の `knownSyscallImpls` エントリは `.abi0` サフィックスを含んでいたため、pclntab からのシンボルとマッチせず、`IsInsideWrapper(0x402cec)` が `false` を返していた。

**Pass 2 への影響:** Pass 1 修正により `internal/runtime/syscall.Syscall6` (0x402cd0–0x402d1f) が `wrapperRanges` に登録されるようになった。これにより `IsInsideWrapper` が正しく機能し、Pass 2（GoWrapperResolver）も `syscall.socket` 内の `BL syscall.RawSyscall` を `go_wrapper` として解決できるようになった。Pass 2 自体のコード変更は不要だった。

### 7.2 修正後の検証結果

```
=== RUN   TestAC1_CgoBinaryNetworkDetection
=== RUN   TestAC1_CgoBinaryNetworkDetection/dynsym_returns_NoNetworkSymbols
    AnalyzeNetworkSymbols result: no_network_symbols (confirmed)
--- PASS

=== RUN   TestAC1_CgoBinaryNetworkDetection/syscall_analysis_detects_socket
    SyscallAnalysis architecture: arm64
    TotalDetectedEvents: 39
    NetworkSyscallCount: 1
    HasNetworkSyscalls: true                   ← ✅ 修正前は false
    IsHighRisk: true                           ← control_flow_boundary が残るため（§7.3 参照）
    HasUnknownSyscalls: true
    Syscall[37]: #198 (socket) isNetwork=true method=go_wrapper at 0x46faa8  ← ✅ 検出
--- PASS
```

| 検証項目 | 修正前 | 修正後 | 合否 |
|---------|--------|--------|------|
| `.dynsym` が `NoNetworkSymbols` を返す | ✅ | ✅ | ✅ |
| `socket`(#198) が検出される | ❌ 未検出 | ✅ `go_wrapper` で検出 | ✅ |
| `HasNetworkSyscalls: true` | ❌ `false` | ✅ `true` | ✅ |

### 7.3 残存する `IsHighRisk: true`

`control_flow_boundary` による `unknown` syscall が 3 件残存している（0x46c6f4, 0x46c740, 0x46c760, 0x46cf04）。これらは関数境界をまたいだ制御フローに起因するものであり、§4.2 の安全方向フェイルセーフの範囲内として現時点では許容する。

AC-1 の 3 番目の条件（`HasNetworkSyscalls: true` が返ること）は達成されたため、**AC-1 の全条件が充足された**。
