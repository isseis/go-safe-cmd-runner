# CGO バイナリのネットワーク検出 要件定義書

## 1. 概要

### 1.1 背景

タスク 0069 では ELF バイナリの `.dynsym` セクションを解析してネットワーク関連シンボルを検出する機能を実装した。このアプローチは、libc 等の共有ライブラリから `socket`、`connect` 等をインポートするバイナリに対しては有効である。

しかし、CGO を使用した Go バイナリでは以下の理由から `.dynsym` 解析でネットワーク使用を検出できない：

- Go ランタイムはネットワーク syscall を libc 経由ではなく直接発行する（`syscall.RawSyscall(SYS_SOCKET, ...)` 等）
- そのため `.dynsym` に `socket` 等のシンボルは現れない
- 一方、CGO バイナリは libc 等をリンクするため `.dynsym` を持ち、「動的バイナリ」として分類される
- 結果として `SyscallAnalysis`（機械語 syscall 解析）フローに到達せず、ネットワーク使用が未検出のまま `NoNetworkSymbols` を返す

タスク 0070/0072 で実装した `SyscallAnalysis`（機械語 syscall 解析）は静的バイナリ向けに設計されているが、バイナリ本体の `.text` セクションをスキャンするため、CGO バイナリ本体に埋め込まれた Go ランタイムの syscall ラッパーも理論上は検出対象になりうる。

### 1.2 目的

動的 ELF バイナリ（`.dynsym` を持つもの）に対しても `SyscallAnalysis` を実行し、`.dynsym` 解析で見逃した network syscall（`SYS_SOCKET` 等）を検出できるようにする。

特に CGO ビルドの Go バイナリを対象とするが、同様に「動的バイナリだが libc の `socket()` を経由せず直接 syscall する」C バイナリも対象となる。

### 1.3 スコープ

- **対象**: 動的リンクされた ELF バイナリ（`.dynsym` を持つもの）全般。主たる対象は `CGO_ENABLED=1`（デフォルト）でビルドした CGO Go バイナリ。`CGO_ENABLED=0` でも `-buildmode=c-shared` / `-buildmode=plugin` 等で動的リンクになる場合も含む
- **対象外**: 静的 ELF バイナリ（タスク 0070/0072 の既存フローを維持）
- **対象外**: macOS Mach-O バイナリ（別途検討）
- **対象外**: スクリプトファイル

### 1.4 前提調査結果

タスク 0076 のレビュー時に以下を確認した：

#### 検出可否の分類

| シナリオ | `.dynsym` 解析 | SyscallAnalysis | 現状 |
|---------|--------------|----------------|------|
| C バイナリ（libc `socket()` 経由） | **検出可** | 不要 | 対応済み |
| CGO Go バイナリ（`CGO_ENABLED=1`、デフォルト） | **検出不可** | 理論上検出可能 | **未対応** |
| 純粋 Go バイナリ（`CGO_ENABLED=0`、`-buildmode=c-shared` / `-buildmode=plugin` 等で動的リンクになる場合） | 検出不可 | 検出可能（同上） | **未対応** |
| 静的 Go バイナリ（`CGO_ENABLED=0` 通常ビルド） | 対象外 | 対応済み（タスク 0070） | 対応済み |

#### 技術的不確実性

`SyscallAnalysis` が動的バイナリで機能するかは **実際の CGO バイナリで検証が必要**。以下が不確実：

- Go ランタイムの syscall ラッパーが最適化・インライン展開された場合、SYSCALL 命令直前のレジスタ追跡（現行の最大 50 命令後方スキャン）が成功するか
- Go バイナリの `.text` セクション構造が現行の x86_64/ARM64 スキャンロジックと適合するか

**本タスクの実装前に、まず上記を検証することを強く推奨する。**

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| CGO バイナリ | `CGO_ENABLED=1`（デフォルト）でビルドした Go バイナリ。libc をリンクするため `.dynsym` を持つが、ネットワーク syscall は Go ランタイムが直接発行する。通常の `go build` で生成される最も一般的なケースであり、本タスクの主たる対象 |
| 動的バイナリ | `.dynsym` セクションを持つ ELF バイナリ。タスク 0069 の `.dynsym` 解析対象 |
| SyscallAnalysis | タスク 0070/0072 で実装した機械語 syscall 解析。バイナリ本体の `.text` セクションをスキャンし SYSCALL 命令を検出する |
| フォールバック SyscallAnalysis | 本タスクで追加する、動的バイナリへの SyscallAnalysis 適用。`.dynsym` 解析が `NoNetworkSymbols` の場合のみ実行 |

## 3. 機能要件

### 3.1 事前検証（実装前の必須作業）

#### FR-3.1.1: CGO バイナリでの SyscallAnalysis 動作確認

実装に先立ち、テスト用 CGO バイナリを作成して「`.dynsym` では `NoNetworkSymbols` になるが `SyscallAnalysis` では検出できる」ことを確認する。これが本タスクで解消すべき盲点そのものである。

検証方法：
1. Go の `syscall` パッケージ（`syscall.Socket()` 等）でソケットを直接生成する CGO バイナリを作成する。`net` パッケージは CGO ビルドで `getaddrinfo` 等の DNS 関数を `.dynsym` にリンクし `NetworkDetected` を返す可能性があるため使わない。`syscall.Socket` を使えば Go ランタイムが `SYS_SOCKET` を直接発行するため `.dynsym` に `socket` シンボルは現れない
2. `.dynsym` 解析（タスク 0069 の `elfanalyzer.StandardELFAnalyzer.AnalyzeNetworkSymbols()`）を呼び出し `NoNetworkSymbols` が返ることを確認する
3. 同バイナリに対して `elfanalyzer.SyscallAnalyzer` を呼び出し、`SYS_SOCKET` を含む network syscall を検出できるかを確認する

**この検証が失敗した場合（手順 3 で `HasNetworkSyscalls: false` かつ `IsHighRisk: true` になる場合）、本タスクのアプローチを再検討すること。**

**事前検証結果（`ac1_verification_result.md` 参照）:**
arm64 環境での検証の結果、手順 3 は部分的成功に留まった：
- `.dynsym` 解析が `NoNetworkSymbols` を返すことは確認（盲点の再現）✅
- `SyscallAnalyzer` は `socket`(#198) を検出できず `HasNetworkSyscalls: false`、`IsHighRisk: true` を返した
  - 原因（Pass 1）: arm64 では `SVC #0` 直前の命令が `LDR x8, [sp, #8]`（スタックロード）であり、後方スキャンが `unknown:indirect_setting` と判定する
  - 原因（Pass 2）: `GoWrapperResolver` の `knownSyscallImpls` に登録された名称（`"internal/runtime/syscall/linux.Syscall6"`）が実際のシンボル名（`"internal/runtime/syscall.Syscall6.abi0"`）と一致せず、補完解析が適用されなかった
- CGO バイナリは `AnalysisError`（高リスク）扱いとなり実行が禁止される。これは §4.2 の安全方向フェイルセーフとして機能している

この結果を受け、本タスクの実装では **GoWrapperResolver の arm64 対応強化**（`knownSyscallImpls` のシンボル名更新）を含める（詳細は §8 参照）。

### 3.2 `record` コマンドの拡張

#### FR-3.2.1: 動的バイナリへの SyscallAnalysis 実行

`record` コマンド実行時に、`.dynsym` 解析が `NoNetworkSymbols` を返した動的 ELF バイナリに対して `SyscallAnalysis` を実行し、結果を保存する。

現行の `syscallAnalysisContext.analyzeFile()`（`cmd/record/main.go`）は `.dynsym` セクションの存在を確認して `ErrNotStaticELF` を返し動的バイナリをスキップしている。本タスクでは、`.dynsym` 解析が `NoNetworkSymbols` を返したバイナリに限りスキップせず `SyscallAnalysis` を実行するよう変更する。

`NetworkDetected` のバイナリは対象外とする。`.dynsym` 解析がすでにネットワーク使用を検出しており、`runner` は `SyscallAnalysis` を参照せずに正しく判定できるため、追加の解析コストは不要である。

- 静的バイナリの既存フロー（タスク 0070/0072）は変更しない
- `SyscallAnalysis` がエラーの場合は `record` をエラーで終了する（現行の静的バイナリと同様）
- `SyscallAnalysis` の `IsHighRisk`（未知の syscall が存在する）は `record` をエラーで終了せずそのまま保存し、`runner` の判断に委ねる
- 結果の保存は既存の `syscallStore.SaveSyscallAnalysis()` を使用する

### 3.3 `runner` 実行時のフォールバック

#### FR-3.3.1: NoNetworkSymbols 時の SyscallAnalysis 参照

現行の `StandardELFAnalyzer.AnalyzeNetworkSymbols()`（`internal/runner/security/elfanalyzer/standard_analyzer.go`）は、動的バイナリで `.dynsym` に network symbol がなければ `checkDynamicSymbols()` が `NoNetworkSymbols` を返してそのまま終了する。`syscallStore` は静的バイナリの `handleStaticBinary()` 経由でしか参照されない。

`StandardELFAnalyzer` には `syscallStore` フィールドと `NewStandardELFAnalyzerWithSyscallStore()` コンストラクタが既に定義されており、アダプタ `NewELFSyscallStoreAdapter()`（`internal/runner/security/syscall_store_adapter.go`）も存在する。しかし現状の production コードでは `NewBinaryAnalyzer()` が常に `NewStandardELFAnalyzer(nil, nil)` を呼んでおり、`syscallStore` は**静的バイナリを含め一切注入されていない**。`NewStandardELFAnalyzerWithSyscallStore` はテストでしか使われていない。

本タスクでは以下の注入チェーンを新設・結合する：

```
normal_manager.go
  → NewStandardEvaluator(NetworkSymbolStore, SyscallAnalysisStore)  ← 引数追加
    → NewNetworkAnalyzerWithStore(NetworkSymbolStore, SyscallAnalysisStore)  ← 引数追加
      → NewBinaryAnalyzer(SyscallAnalysisStore)  ← 引数追加
        → NewStandardELFAnalyzerWithSyscallStore(nil, nil, store)  ← 既存コンストラクタ使用
```

または `fileanalysis.Store`（両方のストアのファクトリ）を単一引数として渡し、内部で `NewSyscallAnalysisStore()` / `NewNetworkSymbolAnalysisStore()` を生成する方式も検討する。具体的な設計は `02_architecture.md` で決定する。

変更後の振る舞い（`checkDynamicSymbols()` が `NoNetworkSymbols` を返した後）：

1. `syscallStore` が nil（未設定）の場合はそのまま `NoNetworkSymbols` を返す（フォールバックなし）
2. `syscallStore.LoadSyscallAnalysis()` を呼び出す
   - `ErrRecordNotFound` / `ErrNoSyscallAnalysis`（キャッシュなし）→ `NoNetworkSymbols` を返す
   - `ErrHashMismatch`（バイナリが record 時から変更されている）→ `AnalysisError` を返す（既存の静的バイナリと同様の安全側挙動）
   - その他エラー → `NoNetworkSymbols` を返す（既存の静的バイナリキャッシュ miss と同様）
   - 取得成功 → `convertSyscallResult()` を呼び出す（既存の静的バイナリと同じ変換ロジックを流用）
     - `IsHighRisk: true` → `AnalysisError` を返す（既存の `convertSyscallResult()` と同様）
     - `HasNetworkSyscalls: true` → `NetworkDetected` を返す
     - それ以外 → `NoNetworkSymbols` を返す

## 4. 非機能要件

### 4.1 スキーマ変更なし

`SyscallAnalysis` フィールドは `fileanalysis.Record` に既に存在する。本タスクでは `schema_version` を変更しない（動的バイナリへの `SyscallAnalysis` 記録は任意フィールドの追加であり、既存の読み込みとの互換性を維持する）。

### 4.2 検出精度の限界を受け入れる

Go ランタイムの最適化によって SYSCALL 命令のレジスタ追跡が失敗した場合、`HasUnknownSyscalls = true` となり `AnalysisError` 扱いになる（安全方向のフェイルセーフ）。この挙動は許容する。

**事前検証で確認された arm64 の挙動**: arm64 環境での CGO バイナリでは、`GoWrapperResolver`（Pass 2）のシンボル名不一致により、現状では `IsHighRisk: true` が返る（§3.1 FR-3.1.1 参照）。これは安全方向フェイルセーフの一例であり、CGO バイナリが誤って許可されることはない。GoWrapperResolver の arm64 対応強化（§8 参照）により、`HasNetworkSyscalls: true` での正確な検出に移行することを目指す。

**通常の動的 C バイナリへの副作用なし**: 標準的な動的リンク C バイナリは、システムコールを本体の `.text` 内ではなく共有ライブラリ（`libc.so` 等）側で発行する。`SyscallAnalysis` はバイナリ本体の `.text` セクションのみをスキャンするため、ネットワーク通信を行わない通常の C バイナリに対してはスキャン結果が 0 件となり `HasNetworkSyscalls = false`、`IsHighRisk = false` で終了する。本タスクのフォールバック追加によって通常の C バイナリが誤って高リスク判定される頻発は起きない。

## 5. 受け入れ条件

### AC-1: 事前検証

- [x] `syscall.Socket()` を直接呼び出す CGO バイナリ（FR-3.1.1 手順 1 のサンプルコード参照）に対して `.dynsym` 解析（`AnalyzeNetworkSymbols()`）が `NoNetworkSymbols` を返すことが確認されていること（本タスクの盲点を再現している）
- [x] 同バイナリに対して `elfanalyzer.SyscallAnalyzer` を実行した結果（成功/失敗・原因）が確認されており、本文書のセクション 8「未解決事項」に記録されていること
  - **実測結果（arm64）**: `HasNetworkSyscalls: false`、`IsHighRisk: true`（`socket`(#198) は未検出）
  - arm64 環境での Pass 1（直接スキャン）は `unknown:indirect_setting` 判定、Pass 2（GoWrapperResolver）はシンボル名不一致により未適用
  - 詳細は `ac1_verification_result.md` 参照
  - **実測結果（x86_64）**: `HasNetworkSyscalls: false`、`IsHighRisk: true`（`socket`(#41) は未検出）
  - x86_64 環境での Pass 1 は `knownSyscallImpls` 除外により未検出、Pass 2 は CGO バイナリ固有の pclntab アドレスずれ（−0x100）により未適用
  - 詳細は `ac1_verification_result_x86_64.md` 参照
- [ ] GoWrapperResolver の arm64 シンボル名対応（`knownSyscallImpls` 更新）により `HasNetworkSyscalls: true` が返るようになること（実装フェーズで検証）

### AC-2: `record` 拡張

- [ ] `.dynsym` 解析が `NoNetworkSymbols` を返した動的 ELF バイナリに対して `SyscallAnalysis` が実行・保存されること
- [ ] `.dynsym` 解析が `NetworkDetected` を返したバイナリに対して `SyscallAnalysis` が実行されないこと
- [ ] 静的 ELF バイナリの既存フローが変更されないこと

### AC-3: `runner` フォールバック

- [ ] `.dynsym` 解析が `NoNetworkSymbols` でも `SyscallAnalysis` に `HasNetworkSyscalls: true` が記録されている場合、`NetworkDetected` を返すこと
- [ ] `SyscallAnalysis` が未記録（`ErrRecordNotFound` / `ErrNoSyscallAnalysis`）の場合は `NoNetworkSymbols` のままであること
- [ ] `ErrHashMismatch`（バイナリが record 時から変更）の場合は `AnalysisError` を返すこと（安全側フェイルセーフ）
- [ ] `SyscallAnalysis` の `IsHighRisk: true` の場合は `AnalysisError` を返すこと（安全側フェイルセーフ）

### AC-4: 既存テストへの非影響

- [ ] 既存のテストがすべてパスすること

## 6. 検証方法

### 6.1 事前検証用スクリプト

以下のような Go ファイルを用意して CGO バイナリをビルドし、`.dynsym` 解析と `SyscallAnalysis` の両方を実行して結果を比較する。

**ポイント**: `import "C"` で CGO を有効にし（動的バイナリにする）、ネットワーク syscall は Go の `syscall` パッケージから直接発行する。`net` パッケージを使うと CGO ビルドでは `getaddrinfo` 等の DNS 関数が `.dynsym` にリンクされ `NetworkDetected` が返る可能性があるため使わない。`syscall.Socket` を直接呼ぶことで `.dynsym` には `socket` シンボルが現れない（`NoNetworkSymbols`）が、`.text` セクションには SYSCALL 命令が埋め込まれる状態を確実に再現できる。

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

```bash
CGO_ENABLED=1 go build -o /tmp/cgo_test main.go

# 手順 2: .dynsym 解析が NoNetworkSymbols を返すことを確認
# （elfanalyzer.StandardELFAnalyzer.AnalyzeNetworkSymbols() を使ったテストまたは手動確認）

# 手順 3: SyscallAnalysis が result.Summary.HasNetworkSyscalls == true を返すことを確認
# （elfanalyzer.SyscallAnalyzer を使ったテストまたは手動確認）
```

### 6.2 ユニットテスト

| テストケース | 検証内容 |
|------------|---------|
| CGO バイナリ（ネットワーク使用） | `record` 後に `SyscallAnalysis.HasNetworkSyscalls: true` が保存されること |
| `.dynsym` で `NoNetworkSymbols`、`SyscallAnalysis` で `HasNetworkSyscalls: true` | `runner` が `NetworkDetected` を返すこと |
| `.dynsym` で `NoNetworkSymbols`、`SyscallAnalysis` 未記録 | `runner` が `NoNetworkSymbols` を返すこと |
| `.dynsym` で `NoNetworkSymbols`、`SyscallAnalysis` で `ErrHashMismatch` | `runner` が `AnalysisError`（高リスク）を返すこと |
| `.dynsym` で `NoNetworkSymbols`、`SyscallAnalysis` で `IsHighRisk: true` | `runner` が `AnalysisError`（高リスク）を返すこと |

## 7. 先行タスクとの関係

| 項目 | タスク 0069 | タスク 0070/0072 | タスク 0076 | 本タスク（0077）|
|------|------------|-----------------|------------|----------------|
| 解析手法 | `.dynsym` シンボル解析 | 機械語 syscall 解析（静的バイナリ） | `.dynsym` 解析結果のキャッシュ | 機械語 syscall 解析（動的バイナリへ拡張） |
| 対象バイナリ | 動的 ELF | 静的 ELF | 動的 ELF | 動的 ELF（CGO・純粋 Go 含む） |
| 目的 | ネットワーク使用検出（C バイナリ） | 静的バイナリの syscall 検出 | runner 時の再解析廃止 | 動的バイナリの syscall 直接呼び出しによる検出漏れ解消 |

## 8. 未解決事項

- [x] **検証結果**: `SyscallAnalysis` が実際の CGO バイナリで機能するか（AC-1 の検証後に更新）

  **検証結果・arm64（詳細は `ac1_verification_result.md` 参照）:**
  - `.dynsym` 解析が `NoNetworkSymbols` を返すことは確認（盲点の再現）✅
  - `SyscallAnalysis` は `HasNetworkSyscalls: true` を返さず、代わりに `IsHighRisk: true` を返した
    - 検出 syscall: `exit_group`(94), `exit`(93), `close`(57), `mmap`(222), `munmap`(215) 等（合計 34 件）
    - `socket`(198) は未検出（`unknown:indirect_setting` → `IsHighRisk`）
    - 原因: arm64 では `SVC #0` 直前が `LDR x8, [sp, #8]`（スタックロード）のため、後方スキャンが `indirect_setting` と判定する
    - これは §4.2 「安全方向フェイルセーフ」として想定された挙動であり、CGO バイナリは `AnalysisError`（高リスク）扱いとなる
  - AC-1 の 2 番目の条件（`HasNetworkSyscalls: true`）は満たされないが、安全方向（`AnalysisError`）には倒れることを確認
  - タスク 0077 の実装では GoWrapperResolver の arm64 対応強化が必要（`knownSyscallImpls` のシンボル名更新、または Pass 2 の呼び出し元解析の改善）

  **検証結果・x86_64（詳細は `ac1_verification_result_x86_64.md` 参照）:**
  - `.dynsym` 解析が `NoNetworkSymbols` を返すことは確認（盲点の再現）✅
  - `SyscallAnalysis` は `HasNetworkSyscalls: true` を返さず、代わりに `IsHighRisk: true` を返した
    - 検出 syscall: `exit_group`(231), `exit`(60), `close`(3), `write`(1), `read`(0), `mmap`(9), `munmap`(11) 等（合計 38 件）
    - `socket`(41) は未検出
    - 原因: CGO バイナリ固有の pclntab アドレスずれ（−0x100）により `wrapperAddrs` のルックアップが失敗し、Pass 2 が機能しない
    - これは arm64 とは異なる根本原因だが、最終結果は同じ（`IsHighRisk: true`）
  - AC-1 の 2 番目の条件（`HasNetworkSyscalls: true`）は満たされないが、安全方向（`AnalysisError`）には倒れることを確認

- [ ] **GoWrapperResolver arm64 対応**: `knownSyscallImpls` に `"internal/runtime/syscall.Syscall6.abi0"` 等、Go 1.23+ / arm64 実環境のシンボル名を追加する。これにより Pass 2 が CGO バイナリの `SVC #0` を `go_wrapper` として解決し、`HasNetworkSyscalls: true` を返せるようになることを検証する（`02_architecture.md` で設計、実装フェーズで実施）
- [ ] **pclntab アドレスずれ対応（x86_64 CGO バイナリ）**: CGO バイナリでは `.gopclntab` から取得した関数アドレスが実際の仮想アドレスと 256 バイトずれる問題を解消する必要がある（`02_architecture.md` で設計）

- [ ] **アーキテクチャ**: `record` / `runner` への統合方法（02_architecture.md を別途作成）
