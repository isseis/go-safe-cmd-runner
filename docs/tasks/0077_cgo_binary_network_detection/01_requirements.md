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

- **対象**: 動的リンクされた ELF バイナリ（`.dynsym` を持つもの）全般（CGO Go バイナリ、純粋 Go バイナリ（CGO なし、動的リンク）を含む）
- **対象外**: 静的 ELF バイナリ（タスク 0070/0072 の既存フローを維持）
- **対象外**: macOS Mach-O バイナリ（別途検討）
- **対象外**: スクリプトファイル

### 1.4 前提調査結果

タスク 0076 のレビュー時に以下を確認した：

#### 検出可否の分類

| シナリオ | `.dynsym` 解析 | SyscallAnalysis | 現状 |
|---------|--------------|----------------|------|
| C バイナリ（libc `socket()` 経由） | **検出可** | 不要 | 対応済み |
| CGO Go バイナリ（`syscall.RawSyscall` 直接） | **検出不可** | 理論上検出可能 | **未対応** |
| 純粋 Go バイナリ（CGO なし、動的リンク） | 検出不可 | 検出可能（同上） | **未対応** |
| 静的 Go バイナリ | 対象外 | 対応済み（タスク 0070） | 対応済み |

#### 技術的不確実性

`SyscallAnalysis` が動的バイナリで機能するかは **実際の CGO バイナリで検証が必要**。以下が不確実：

- Go ランタイムの syscall ラッパーが最適化・インライン展開された場合、SYSCALL 命令直前のレジスタ追跡（現行の最大 50 命令後方スキャン）が成功するか
- Go バイナリの `.text` セクション構造が現行の x86_64/ARM64 スキャンロジックと適合するか

**本タスクの実装前に、まず上記を検証することを強く推奨する。**

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| CGO バイナリ | `CGO_ENABLED=1`（デフォルト）でビルドした Go バイナリ。libc をリンクするため `.dynsym` を持つが、ネットワーク syscall は Go ランタイムが直接発行する |
| 動的バイナリ | `.dynsym` セクションを持つ ELF バイナリ。タスク 0069 の `.dynsym` 解析対象 |
| SyscallAnalysis | タスク 0070/0072 で実装した機械語 syscall 解析。バイナリ本体の `.text` セクションをスキャンし SYSCALL 命令を検出する |
| フォールバック SyscallAnalysis | 本タスクで追加する、動的バイナリへの SyscallAnalysis 適用。`.dynsym` 解析が `NoNetworkSymbols` の場合のみ実行 |

## 3. 機能要件

### 3.1 事前検証（実装前の必須作業）

#### FR-3.1.1: CGO バイナリでの SyscallAnalysis 動作確認

実装に先立ち、テスト用 CGO バイナリを作成して「`.dynsym` では `NoNetworkSymbols` になるが `SyscallAnalysis` では検出できる」ことを確認する。これが本タスクで解消すべき盲点そのものである。

検証方法：
1. Go の `net` パッケージ（`net.Dial()` 等）でネットワーク接続する CGO バイナリを作成する。Go ランタイムが直接 syscall を発行するため `.dynsym` に `socket` 等のシンボルは現れない
2. `.dynsym` 解析（タスク 0069 の `DynSymAnalyzer`）を呼び出し `NoNetworkSymbols` が返ることを確認する
3. 同バイナリに対して `elfanalyzer.SyscallAnalyzer` を呼び出し、`SyscallSummary.HasNetworkSyscalls` が `true` になることを確認する

**この検証が失敗した場合（手順 3 で `false` になる場合）、本タスクのアプローチを再検討すること。**

### 3.2 `record` コマンドの拡張

#### FR-3.2.1: 動的バイナリへの SyscallAnalysis 実行

`filevalidator.Validator.Record()` の `saveHash` 内で、動的 ELF バイナリ（`NetworkDetected` または `NoNetworkSymbols`）に対しても `SyscallAnalysis` を実行し、`SyscallAnalysis` フィールドへ保存する。

`NetworkDetected` のバイナリも対象とする理由：動的 ELF バイナリであっても libc を介さず直接 syscall する可能性があり、`record` はすべてのネットワーク関連 syscall の使用をファイルに記録することが期待されているため。

- 静的バイナリの場合は既存フロー（タスク 0070/0072）を維持し変更しない
- `SyscallAnalysis` がエラーの場合は `record` をエラーで終了する
- `SyscallAnalysis` の `IsHighRisk`（未知の syscall が存在する）は `record` をエラーで終了せずそのまま保存し、`runner` の判断に委ねる
- 既存の `SyscallAnalysis` フィールドへの保存は既存コードと同じメソッドを使用する

### 3.3 `runner` 実行時のフォールバック

#### FR-3.3.1: NoNetworkSymbols 時の SyscallAnalysis 参照

`isNetworkViaBinaryAnalysis` において、動的バイナリで `.dynsym` 解析（またはキャッシュ）が `NoNetworkSymbols` を返した場合、追加で `SyscallAnalysis` キャッシュを参照する：

1. `fileanalysis.Store` から `SyscallAnalysis` を読み込む
2. `SyscallSummary.HasNetworkSyscalls` が `true` なら `NetworkDetected` として扱う
3. `SyscallAnalysis` が未記録の場合はそのまま `NoNetworkSymbols` を返す（フォールバックなし）

**注意**: `SyscallAnalysis` の `IsHighRisk`（未知の syscall が存在する）は `AnalysisError` として扱う（既存の `convertSyscallResult()` と同様）。

## 4. 非機能要件

### 4.1 スキーマ変更なし

`SyscallAnalysis` フィールドは `fileanalysis.Record` に既に存在する。本タスクでは `schema_version` を変更しない（動的バイナリへの `SyscallAnalysis` 記録は任意フィールドの追加であり、既存の読み込みとの互換性を維持する）。

### 4.2 検出精度の限界を受け入れる

Go ランタイムの最適化によって SYSCALL 命令のレジスタ追跡が失敗した場合、`HasUnknownSyscalls = true` となり `AnalysisError` 扱いになる（安全方向のフェイルセーフ）。この挙動は許容する。

## 5. 受け入れ条件

### AC-1: 事前検証

- [ ] `net.Dial()` 等を使う CGO バイナリに対して `.dynsym` 解析が `NoNetworkSymbols` を返すことが確認されていること（本タスクの盲点を再現している）
- [ ] 同バイナリに対して `SyscallAnalysis` が `SYS_SOCKET` 等を検出できること（`HasNetworkSyscalls: true`）が確認されていること
- [ ] 検証結果（成功/失敗・検出できたシステムコール番号）が `record` の出力するファイルに記録されていること

### AC-2: `record` 拡張

- [ ] 動的 ELF バイナリに対して `SyscallAnalysis` が実行・保存されること
- [ ] 静的 ELF バイナリの既存フローが変更されないこと

### AC-3: `runner` フォールバック

- [ ] `.dynsym` 解析が `NoNetworkSymbols` でも `SyscallAnalysis` にネットワーク syscall が記録されている場合、`NetworkDetected` を返すこと
- [ ] `SyscallAnalysis` が未記録の場合は `NoNetworkSymbols` のままであること

### AC-4: 既存テストへの非影響

- [ ] 既存のテストがすべてパスすること

## 6. 検証方法

### 6.1 事前検証用スクリプト

以下のような Go ファイルを用意して CGO バイナリをビルドし、`.dynsym` 解析と `SyscallAnalysis` の両方を実行して結果を比較する。

**ポイント**: `import "C"` のみで C の socket 関数は呼ばず、Go の `net` パッケージを使う。Go ランタイムが直接 `SYS_SOCKET` syscall を発行するため `.dynsym` には `socket` シンボルが現れない（`NoNetworkSymbols`）が、`.text` セクションには SYSCALL 命令が埋め込まれる。

```go
// main.go（CGO バイナリ用テスト）
// CGO_ENABLED=1 でビルドされるが、ネットワーク syscall は Go ランタイムが直接発行する
package main

import "C" // CGO を有効にして動的バイナリにする（libc をリンクさせる）

import "net"

func main() {
    conn, _ := net.Dial("tcp", "127.0.0.1:0")
    if conn != nil {
        conn.Close()
    }
}
```

```bash
CGO_ENABLED=1 go build -o /tmp/cgo_test main.go

# 手順 2: .dynsym 解析が NoNetworkSymbols を返すことを確認
# （elfanalyzer の DynSymAnalyzer を使ったテストまたは手動確認）

# 手順 3: SyscallAnalysis が HasNetworkSyscalls: true を返すことを確認
# （elfanalyzer の SyscallAnalyzer を使ったテストまたは手動確認）
```

### 6.2 ユニットテスト

| テストケース | 検証内容 |
|------------|---------|
| CGO バイナリ（ネットワーク使用） | `record` 後に `SyscallAnalysis.HasNetworkSyscalls: true` が保存されること |
| `.dynsym` で `NoNetworkSymbols`、`SyscallAnalysis` で `HasNetworkSyscalls: true` | `runner` が `NetworkDetected` を返すこと |
| `.dynsym` で `NoNetworkSymbols`、`SyscallAnalysis` 未記録 | `runner` が `NoNetworkSymbols` を返すこと |

## 7. 先行タスクとの関係

| 項目 | タスク 0069 | タスク 0070/0072 | タスク 0076 | 本タスク（0077）|
|------|------------|-----------------|------------|----------------|
| 解析手法 | `.dynsym` シンボル解析 | 機械語 syscall 解析（静的バイナリ） | `.dynsym` 解析結果のキャッシュ | 機械語 syscall 解析（動的バイナリへ拡張） |
| 対象バイナリ | 動的 ELF | 静的 ELF | 動的 ELF | 動的 ELF（CGO・純粋 Go 含む） |
| 目的 | ネットワーク使用検出（C バイナリ） | 静的バイナリの syscall 検出 | runner 時の再解析廃止 | 動的バイナリの syscall 直接呼び出しによる検出漏れ解消 |

## 8. 未解決事項

- [ ] **検証結果**: `SyscallAnalysis` が実際の CGO バイナリで機能するか（AC-1 の検証後に更新）
- [ ] **アーキテクチャ**: 検証が成功した場合の `record` / `runner` への統合方法（02_architecture.md を別途作成）
