# 要件定義書: ELF バイナリ VERNEED なし時の名前ベースシンボル検出

## 1. 概要

### 1.1 背景

Task 0106 で導入した ELF シンボルフィルタは `sym.Library`（GNU VERNEED セクション由来）を使って libc 由来シンボルを識別する。この方式は glibc 環境（Ubuntu / Debian / RHEL 等）では正確に機能するが、**musl libc**（Alpine Linux 等）では機能しない。

musl はシンボルバージョニングを採用しておらず、VERNEED セクションを生成しない。そのため musl リンクバイナリでは全 `SHN_UNDEF` シンボルの `Library` フィールドが空になる。Task 0106 では DT_NEEDED フォールバックを実装したが、「libc のみを DT_NEEDED に持つバイナリ」という極めて限定的な条件でしか機能しないため削除された。

結果として、現状では musl 環境の ELF バイナリは `socket` や `connect` をインポートしていても `DetectedSymbols` が空になり、ネットワーク操作を誤って検出漏れする。

### 1.2 目的

VERNEED なしの ELF バイナリに対して、インポートシンボル名によるネットワーク関連シンボルの検出を実現する。

### 1.3 スコープ

#### 対象

- `internal/runner/security/elfanalyzer/standard_analyzer.go` の `checkDynamicSymbols`
  - VERNEED なし（全 `SHN_UNDEF` の `Library` が空）の場合に名前ベースフィルタを適用する
  - 既存の `networkSymbols` マップに一致するシンボルのみを `DetectedSymbols` に記録する
  - カテゴリは VERNEED ありの場合と同じ（`"socket"`, `"dns"`, `"tls"`, `"http"` 等）
- 上記に伴うテストの追加

#### 対象外

- Mach-O バイナリ（Symtab の library ordinal で既に対応済み）
- `syscall_wrapper` カテゴリ（`read`, `write` 等）の VERNEED なし対応
- VERNEED ありの ELF バイナリの既存ロジック変更

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| VERNEED | GNU シンボルバージョニングのセクション（`.gnu.version_r`）。どのシンボルがどのライブラリから提供されるかを記録する。glibc は生成するが musl は生成しない |
| 名前ベースフィルタ | ライブラリ帰属を用いず、インポートシンボル名のみで照合するフィルタリング方式 |
| networkSymbols | `binaryanalyzer.GetNetworkSymbols()` が返すシンボル名→カテゴリのマップ。`socket`, `connect`, `getaddrinfo`, `SSL_CTX_new` 等を含む |
| musl | Alpine Linux 等で使用される C 標準ライブラリ。VERNEED セクションを生成しない |

## 3. 機能要件

### FR-1: VERNEED なし時の名前ベースシンボル検出

`checkDynamicSymbols` にて VERNEED なし（全 `SHN_UNDEF` の `Library` が空）と判定した場合、以下の処理を行う。

- `SHN_UNDEF` の各シンボルに対し、`networkSymbols` マップで名前を検索する
- 一致した場合、対応するカテゴリ（`"socket"`, `"dns"`, `"tls"`, `"http"` 等）を付与して `DetectedSymbols` に記録する
- 一致しないシンボルは `DetectedSymbols` に記録しない（`syscall_wrapper` は付与しない）

**設計上の考慮**：ライブラリ帰属を無視することで、`socket` という名前の関数がどのライブラリで定義されていてもネットワーク操作として検出される。誤検知リスクは低い。理由：`networkSymbols` に登録されている名前（`socket`, `connect`, `SSL_CTX_new` 等）は POSIX / OpenSSL 標準に由来する固有名であり、異なるセマンティクスで定義されることはまれである。また、セキュリティツールとして偽陰性（検出漏れ）の方が偽陽性（誤検知）より危険なため、このトレードオフは許容する。

### FR-2: VERNEED あり時の既存動作維持

VERNEED あり（いずれかの `SHN_UNDEF` シンボルに `Library != ""` が存在する）の場合は、Task 0106 の `sym.Library` ベースのフィルタをそのまま使用する。FR-1 の名前ベースフィルタは適用しない。

### FR-3: `DynamicLoadSymbols` の既存動作維持

`dlopen` 等の動的ロードシンボルの検出ロジック（`IsDynamicLoadSymbol`）は VERNEED の有無に関わらず変更しない。

## 4. 非機能要件

### NFR-1: glibc 環境への影響なし

VERNEED ありの glibc バイナリに対して、Task 0106 以前・以後と同一の結果を返すこと。

### NFR-2: 偽陽性の限定

名前ベースフィルタが適用されるのは VERNEED なしのバイナリのみとし、VERNEED ありのバイナリでは従来どおりライブラリ帰属を使用することで、glibc 環境での偽陽性を増やさない。

## 5. 受け入れ基準

### AC-1: VERNEED なし・ネットワークシンボルあり

- `socket` をインポートし VERNEED のない ELF バイナリで `checkDynamicSymbols` を呼ぶと、`DetectedSymbols` に `{Name: "socket", Category: "socket"}` が含まれること
- `Result` が `NetworkDetected` であること

### AC-2: VERNEED なし・TLS シンボル（非 libc ライブラリ由来）

- `SSL_CTX_new` をインポートし VERNEED のない ELF バイナリで `checkDynamicSymbols` を呼ぶと、`DetectedSymbols` に `{Name: "SSL_CTX_new", Category: "tls"}` が含まれること

### AC-3: VERNEED なし・ネットワークシンボルなし

- `read` のみをインポートし VERNEED のない ELF バイナリで `checkDynamicSymbols` を呼ぶと、`DetectedSymbols` が空であること（`networkSymbols` に登録されていないシンボルは記録しない）
- `Result` が `NoNetworkSymbols` であること

### AC-4: VERNEED なし・複数ライブラリリンク時も検出

- `socket` と `SSL_CTX_new` を持ち、`libpthread.so`, `libssl.so` 等の複数ライブラリにリンクしている VERNEED なし ELF バイナリでも、両シンボルが `DetectedSymbols` に含まれること（旧 DT_NEEDED フォールバックとの差別化）

### AC-5: VERNEED あり時は変更なし（回帰防止）

- `sym.Library` が設定された VERNEED ありの ELF バイナリで、Task 0106 の結果と同一の `DetectedSymbols` が得られること
- libc 以外のライブラリのシンボル（`libm` 等）は記録されないこと

### AC-6: 既存テストの通過

- `make test` / `make lint` がエラーなしで通過すること
- AC-1 〜 AC-5 を検証するテストケースが `elfanalyzer/analyzer_test.go` に存在すること

## 6. 先行タスクとの関係

| タスク | 関係 |
|-------|------|
| 0106 シンボル解析ライブラリフィルタ | VERNEED ベースのフィルタを導入。DT_NEEDED フォールバックを実装したが削除。本タスクはその後継として musl 対応を実現する |
| 0076 ネットワークシンボルキャッシュ | `networkSymbols` マップを導入。本タスクで名前ベースフィルタの照合に再利用する |
