# 実装計画: CGO バイナリネットワーク検出（タスク 0077）

## 進捗

- [x] Step 1: `knownSyscallImpls` に arm64 シンボル名を追加（Pass 1 修正）
- [x] Step 2: `ParsePclntab` に pclntab オフセット自動補正を実装（Pass 2 修正）
- [x] Step 3: `record` コマンドで動的バイナリも SyscallAnalysis を実行
- [x] Step 4: `runner` コマンドで動的バイナリの syscall store を参照
- [x] Step 5: テスト追加（各 AC に対応するテスト）
- [x] Step 6: make test / make lint 通過確認

---

## Step 1: `knownSyscallImpls` に arm64 シンボル名を追加

**ファイル:** `internal/runner/security/elfanalyzer/go_wrapper_resolver.go`

**変更内容:**

```go
var knownSyscallImpls = map[string]struct{}{
    "syscall.rawVforkSyscall":                 {},
    "syscall.rawSyscallNoError":               {},
    "internal/runtime/syscall/linux.Syscall6": {}, // Go 1.22 以前 / x86_64
    "internal/runtime/syscall.Syscall6.abi0":  {}, // Go 1.23+ / arm64
}
```

**効果:** Pass 1 が `internal/runtime/syscall.Syscall6.abi0` 内の `SVC #0` を除外し、`unknown:indirect_setting` に起因する `IsHighRisk: true` を解消する。

---

## Step 2: `ParsePclntab` に pclntab オフセット自動補正を実装

**ファイル:** `internal/runner/security/elfanalyzer/pclntab_parser.go`

**変更内容:**

`detectPclntabOffset` 関数を追加し、`.symtab` と pclntab の最初の共通関数を比較してオフセットを検出・補正する。

**効果:** CGO バイナリ（x86_64 で −0x100 のずれを確認）で Pass 2（GoWrapperResolver）が正しく `syscall.RawSyscall` のアドレスを解決できるようになる。

`.symtab` が存在しない stripped バイナリは補正をスキップし、現状通りフェイルセーフで対応する。

---

## Step 3: `record` コマンドで動的バイナリも SyscallAnalysis を実行

**ファイル:** `cmd/record/main.go`

**変更内容:**

`analyzeFile` から以下を削除:

```go
// 削除した行:
if dynsym := elfFile.Section(".dynsym"); dynsym != nil {
    return elfanalyzer.ErrNotStaticELF
}
```

呼び出し元の `ErrNotStaticELF` チェックも削除。

**効果:** `record` 実行時に動的 ELF バイナリ（CGO バイナリ含む）に対しても SyscallAnalysis が実行され、結果が Store に保存される。

---

## Step 4: `runner` コマンドで動的バイナリの syscall store を参照

**ファイル:** `internal/runner/security/elfanalyzer/standard_analyzer.go`

**変更内容:**

`AnalyzeNetworkSymbols` の Step 5 を変更:

```go
// 変更前:
return a.checkDynamicSymbols(dynsyms)

// 変更後:
dynOutput := a.checkDynamicSymbols(dynsyms)
if dynOutput.Result != binaryanalyzer.NoNetworkSymbols {
    return dynOutput
}
if a.syscallStore != nil {
    syscallOutput := a.lookupSyscallAnalysis(path, file, contentHash)
    if syscallOutput.Result != binaryanalyzer.StaticBinary {
        return syscallOutput
    }
}
return dynOutput
```

**効果:** `.dynsym` に network symbol がない動的バイナリ（CGO バイナリ等）で、`record` 時に保存された SyscallAnalysis の結果（`HasNetworkSyscalls: true`）を使ってネットワーク使用を検出できる。

---

## Step 5: テスト追加（TODO）

### AC-2: record 拡張
- [x] 動的 ELF バイナリに対して SyscallAnalysis が実行・保存されること
- [-] `.dynsym` で `NetworkDetected` のバイナリは SyscallAnalysis が実行されても問題ないこと（`analyzeFile` は `.dynsym` の結果に関係なく常に実行するため、個別テスト不要）

### AC-3: runner フォールバック
- [x] `.dynsym` で `NoNetworkSymbols`、SyscallAnalysis で `HasNetworkSyscalls: true` の場合 `NetworkDetected` を返すこと
- [x] SyscallAnalysis 未記録の場合 `NoNetworkSymbols` を返すこと
- [x] `ErrHashMismatch` の場合 `AnalysisError` を返すこと
- [x] SyscallAnalysis で `IsHighRisk: true` の場合 `AnalysisError` を返すこと

### pclntab 補正
- [x] `.symtab` なし（stripped）→ offset=0
- [x] pclntab に一致する関数なし → offset=0
- [x] 非 CGO バイナリ → offset=0

---

## Step 6: 確認コマンド

```bash
make fmt
make test
make lint
# integration テスト（CGO バイナリ検証）
go test -tags "test integration" -v -run TestAC1_CgoBinaryNetworkDetection_x86_64 \
  ./internal/runner/security/elfanalyzer/
# arm64 テストバイナリ確認
go test -tags "test integration" -v -run TestSyscallAnalyzer_IntegrationARM64 \
  ./internal/runner/security/elfanalyzer/
```
