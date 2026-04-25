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
  - VERNEED あり（`Library != ""`）かつ非 libc ライブラリ由来のシンボルに対して、`networkSymbols` マップを用いた名前ベース検出を適用する（libssl、libcurl 等のネットワーク関連ライブラリのシンボルを捕捉）
  - 既存の `networkSymbols` マップに一致するシンボルのみを `DetectedSymbols` に記録する
  - カテゴリは VERNEED ありの場合と同じ（`"socket"`, `"dns"`, `"tls"`, `"http"` 等）
- 上記に伴うテストの追加

#### 対象外

- Mach-O バイナリ（Symtab の library ordinal で既に対応済み）
- `syscall_wrapper` カテゴリ（`read`, `write` 等）の VERNEED なし対応
- VERNEED あり・libc 由来シンボルの `syscall_wrapper` 検出（Task 0106 実装済み・変更なし）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| VERNEED | GNU シンボルバージョニングのセクション（`.gnu.version_r`）。どのシンボルがどのライブラリから提供されるかを記録する。glibc は生成するが musl は生成しない |
| 名前ベースフィルタ | ライブラリ帰属を用いず、インポートシンボル名のみで照合するフィルタリング方式 |
| networkSymbols | `binaryanalyzer.GetNetworkSymbols()` が返すシンボル名→カテゴリのマップ。`socket`, `connect`, `getaddrinfo`, `SSL_CTX_new` 等を含む |
| musl | Alpine Linux 等で使用される C 標準ライブラリ。VERNEED セクションを生成しない |

## 3. 機能要件

### FR-1: VERNEED なし時の期待動作

musl libc 等で生成された VERNEED なし（全 `SHN_UNDEF` の `Library` が空）の ELF バイナリに対して、FR-2 のシンボル単位フィルタを適用した結果は以下の通りとなる。

- 各シンボルに対し FR-2 ステップ 1 が適用され、`networkSymbols` に一致すれば対応するカテゴリで `DetectedSymbols` に記録する
- `networkSymbols` に一致しないシンボルは `DetectedSymbols` に記録しない。`Library` が常に空なので FR-2 ステップ 2 の `isLibcLibrary` 判定も偽となり、`syscall_wrapper` も付与されない
- ライブラリ帰属の推定に `DT_NEEDED` などの別フォールバックは用いない

**注記**：この動作はバイナリ単位の一括判定ではなく、FR-2 で定義するシンボル単位のフィルタ規則を適用した結果である。

### FR-2: シンボル単位でのフィルタ選択

各 `SHN_UNDEF` シンボルについて、以下の順序でフィルタを適用する。

1. `networkSymbols` マップでシンボル名を検索し、一致すれば対応するカテゴリで `DetectedSymbols` に記録する。`Library` フィールドの値（空か否か）に関わらず適用する。
2. ステップ 1 で一致しなかった場合、`isLibcLibrary(sym.Library)` が真ならば `syscall_wrapper` カテゴリで記録する（Task 0106 の既存動作を維持）。

バイナリ単位での一括判定は行わない。

**設計上の考慮**：
- ネットワーク関連シンボルの検出（ステップ 1）はライブラリ帰属を問わない。`socket`（libc）・`SSL_CTX_new`（libssl）・`curl_easy_perform`（libcurl）等、異なるライブラリ由来のシンボルを一律に捕捉する。`networkSymbols` に登録された名前は POSIX / OpenSSL / libcurl 標準に由来する固有名であり、異なるセマンティクスで定義されることはまれなため誤検知リスクは低い。
- `syscall_wrapper` の記録（ステップ 2）は libc 限定とする。`read` や `write` は汎用的な名前であり、libc 以外のライブラリ（libcurl 等）からインポートされた同名シンボルを `syscall_wrapper` と誤分類することを防ぐ。
- セキュリティツールとして偽陰性（検出漏れ）の方が偽陽性（誤検知）より危険なため、ステップ 1 はライブラリを問わず適用する。

### FR-3: `DynamicLoadSymbols` の既存動作維持

`dlopen` 等の動的ロードシンボルの検出ロジック（`IsDynamicLoadSymbol`）は VERNEED の有無に関わらず変更しない。

## 4. 非機能要件

### NFR-1: glibc 環境への回帰なし

VERNEED ありの glibc バイナリに対して、Task 0106 で検出されていたシンボル（libc 由来ネットワークシンボルおよび `syscall_wrapper`）は引き続き同一の結果で返すこと。加えて、非 libc ネットワークライブラリ（libssl 等）由来のシンボルが新たに検出される場合がある（これは回帰ではなく改善）。

### NFR-2: 偽陽性の限定

名前ベースフィルタの対象を `networkSymbols` に登録されたシンボル名に限定することで、汎用名のシンボルを無差別に記録することを防ぐ。`syscall_wrapper` の付与は libc 由来シンボルのみとし、汎用名（`read` 等）を持つ非 libc ライブラリのシンボルを誤分類しない。

## 5. 受け入れ基準

### AC-1: VERNEED なし・ネットワークシンボルあり

- `socket` をインポートし VERNEED のない ELF バイナリで `checkDynamicSymbols` を呼ぶと、`DetectedSymbols` に `{Name: "socket", Category: "socket"}` が含まれること
- `Result` が `NetworkDetected` であること

### AC-2: VERNEED なし・TLS シンボル名

- `SSL_CTX_new` をインポートし VERNEED のない ELF バイナリで `checkDynamicSymbols` を呼ぶと、`DetectedSymbols` に `{Name: "SSL_CTX_new", Category: "tls"}` が含まれること
- `Result` が `NetworkDetected` であること

### AC-3: VERNEED なし・ネットワークシンボルなし

- `read` のみをインポートし VERNEED のない ELF バイナリで `checkDynamicSymbols` を呼ぶと、`DetectedSymbols` が空であること（`networkSymbols` に登録されていないシンボルは記録しない）
- `Result` が `NoNetworkSymbols` であること

### AC-4: VERNEED なし・複数ライブラリリンク時も検出

- `socket` と `SSL_CTX_new` を持ち、`libpthread.so`, `libssl.so` 等の複数ライブラリにリンクしている VERNEED なし ELF バイナリでも、両シンボルが `DetectedSymbols` に含まれること（旧 DT_NEEDED フォールバックとの差別化）

### AC-5: VERNEED ありバイナリでのシンボル検出

- `Library = "libc.so.6"` かつ `networkSymbols` に一致するシンボル（`socket` 等）は、対応するカテゴリで `DetectedSymbols` に記録されること
- `Library = "libc.so.6"` かつ `networkSymbols` に一致しないシンボル（`read` 等）は、`syscall_wrapper` カテゴリで記録されること（Task 0106 の既存動作）
- `Library = "libssl.so.3"` かつ `SSL_CTX_new` のような `networkSymbols` に一致するシンボルは、`tls` カテゴリで `DetectedSymbols` に記録されること
- `Library = "libm.so.6"` 等、`networkSymbols` に一致しない非 libc ライブラリのシンボルは記録されないこと
- `Library == ""` のシンボルが存在する場合、`networkSymbols` に一致すれば `DetectedSymbols` に記録されること

### AC-6: `DynamicLoadSymbols` は変更なし

- `dlopen` 等の動的ロードシンボルを持つ ELF バイナリで、VERNEED の有無に関わらず `DynamicLoadSymbols` の検出結果が Task 0106 時点から変化しないこと

### AC-7: 既存テストの通過

- `make test` / `make lint` がエラーなしで通過すること
- AC-1 〜 AC-6 を検証するテストケースが `internal/runner/security/elfanalyzer/analyzer_test.go` に存在すること

## 6. 先行タスクとの関係

| タスク | 関係 |
|-------|------|
| 0106 シンボル解析ライブラリフィルタ | VERNEED ベースのフィルタを導入。DT_NEEDED フォールバックを実装したが削除。本タスクはその後継として musl 対応を実現する |
| 0076 ネットワークシンボルキャッシュ | `networkSymbols` マップを導入。本タスクで名前ベースフィルタの照合に再利用する |
