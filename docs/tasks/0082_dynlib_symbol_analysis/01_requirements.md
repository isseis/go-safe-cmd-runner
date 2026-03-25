# 直接依存ライブラリによるネットワーク検出強化 要件定義書

## 1. 概要

### 1.1 背景

タスク 0069〜0081 により、ELF バイナリ自身の `.dynsym` UNDEF シンボルや syscall 命令からネットワーク関連機能を検出できるようになった。

しかし、Ruby のように実装のほぼ全てを共有ライブラリ（`libruby.so`）に移動し、実行ファイル本体は共有ライブラリの関数を呼び出すだけという設計のバイナリでは、ネットワーク関連シンボルが共有ライブラリ側にあるため `record` コマンドが検出できない。

**不採用アプローチ（検討済み・却下済み）:**

依存ライブラリの全 UNDEF シンボルを解析する方法は、バイナリが実際に使用する関数の経路を追わないため false positive が多発する。たとえば `xmlparser` バイナリが `libxml2.so` の `xmlParseFile()` のみを呼び出す場合でも、`libxml2.so` が HTTP フェッチ機能のために `socket()` を UNDEF に持つため、ネットワーク有りと誤検知される。コマンド実行がブロックされ設定ファイルの書き直しが必要になるため、false positive は許容できない。

呼び出しグラフ解析（エントリポイント関数から `socket()` への到達可能性を確認）は技術的には正確だが、大規模ライブラリへの適用は実装コストが非常に高く（間接呼び出し・関数ポインタの扱い等）、現時点では採用しない。

### 1.2 採用アプローチ

以下の 2 つの方策を組み合わせる：

- **方策 A（コマンドプロファイル拡張）**: 既存の `commandRiskProfiles` に言語ランタイムの実行ファイル名を追加する。バイナリ名で一致した場合は静的解析なしにネットワーク有りと判定する（PR-3.1 参照）。
- **方策 C（SOName ベース検出）**: `record` 時に `DynLibDeps` に含まれるライブラリの SOName を既知ネットワークライブラリリストと照合し、一致した場合はネットワーク有りとして記録する（FR-3.2 参照）。方策 C は方策 A では捕捉できない「実行ファイル名が登録されていないが既知ネットワークライブラリにリンクしているバイナリ」をカバーする。

### 1.3 スコープ

- **対象（方策 A）**: commandRiskProfiles への言語ランタイムバイナリ名の追加
- **対象（方策 C）**: `DynLibDeps` を持つ ELF バイナリの SOName ベース検出
- **対象外**: macOS Mach-O バイナリ（方策 C）
- **対象外**: 呼び出しグラフ解析による精密検出（別途検討）

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| SOName | `DT_NEEDED` に記録されたライブラリ名（例: `libruby.so.3.2`） |
| SOName プレフィックス | SOName の先頭に現れるライブラリ識別子（例: `libruby.so.3.2` の `libruby`、`libpython3.11.so.1.0` に対する登録プレフィックス `libpython`） |
| 既知ネットワークライブラリリスト | 登録済みプレフィックスとの安全な前方一致で照合に使用するリスト（FR-3.2.1 参照） |

---

## 3. 機能要件

### 3.1 方策 A: commandRiskProfiles への言語ランタイム追加

#### FR-3.1.1: 追加対象バイナリ名

以下の言語ランタイムを `command_analysis.go` の `commandProfileDefinitions` に追加する。リスクレベルは既存の ruby/python/node と同じく `NetworkRisk: Medium`、`AlwaysNetwork()` とする。

**スクリプト言語インタープリタ**

| バイナリ名 | 言語 | 追加理由 |
|---|---|---|
| `lua`, `lua5.1`, `lua5.2`, `lua5.3`, `lua5.4`, `luajit` | Lua | ネットワーク拡張（LuaSocket 等）を `require` で動的ロード可能 |
| `tclsh`, `tclsh8.5`, `tclsh8.6`, `wish`, `wish8.5`, `wish8.6` | Tcl/Tk | 標準の `socket` コマンドでネットワーク接続可能 |
| `R`, `Rscript` | R | `curl`, `httr` 等のパッケージでネットワーク通信可能 |
| `julia` | Julia | 標準ライブラリに HTTP クライアントあり |
| `guile`, `guile2`, `guile3` | GNU Guile (Scheme) | `(web client)` モジュールでネットワーク通信可能 |
| `elixir`, `iex` | Elixir | Erlang VM 上で動作しネットワーク機能を持つ |
| `erl`, `erlc`, `escript` | Erlang | ネットワーク指向の言語仕様 |

**JVM ベースのランタイム**

| バイナリ名 | 言語/ランタイム | 追加理由 |
|---|---|---|
| `java`, `javaw` | Java | クラスロードにより任意コードを実行、標準ライブラリにネットワーク機能 |
| `groovy`, `groovysh`, `groovyConsole` | Groovy | JVM 上で動作、Grape でネットワーク経由のライブラリ取得 |
| `kotlin` | Kotlin | JVM 上で動作、Kotlin スクリプトはネットワーク利用可能 |
| `scala`, `scala3` | Scala | JVM 上で動作 |
| `clojure` | Clojure | JVM 上で動作 |
| `jruby` | JRuby (Ruby on JVM) | Ruby のネットワーク機能を全て持つ |
| `jython` | Jython (Python on JVM) | Python のネットワーク機能を全て持つ |

**.NET ランタイム**

| バイナリ名 | 言語/ランタイム | 追加理由 |
|---|---|---|
| `dotnet` | .NET | System.Net でネットワーク通信可能 |
| `mono` | Mono (.NET) | 同上 |
| `pwsh`, `powershell` | PowerShell | Invoke-WebRequest 等でネットワーク通信可能 |

#### FR-3.1.2: リスクレベルの統一

追加するバイナリはすべて以下のプロファイルとする：

```go
NetworkRisk(runnertypes.RiskLevelMedium, "<言語名> interpreter with built-in network capabilities").
AlwaysNetwork()
```

### 3.2 方策 C: SOName ベースのネットワーク検出

#### FR-3.2.1: 既知ネットワークライブラリリスト

以下の SOName プレフィックスを既知ネットワークライブラリリストとして定義する。照合は登録プレフィックスに対する安全な前方一致で行い、SOName が `<prefix>` で始まり、その直後が `.`, `-`, 数字のいずれかである場合のみ一致とみなす。これにより `libpython3.11.so.1.0` は `libpython` に一致する一方、`libpythonista.so` は一致しない。

**ネットワーク・プロトコルライブラリ**

| SOName プレフィックス | ライブラリ名 | 理由 |
|---|---|---|
| `libcurl` | libcurl | HTTP/FTP/SMTP 等のネットワーク通信 |
| `libssl` | OpenSSL libssl | TLS 接続（ネットワーク前提） |
| `libssh` | libssh | SSH 接続 |
| `libssh2` | libssh2 | SSH 接続 |
| `libzmq` | ZeroMQ | ネットワークメッセージング |
| `libnanomsg` | nanomsg | ネットワークメッセージング |
| `libnng` | NNG | ネットワークメッセージング |
| `libnghttp2` | nghttp2 | HTTP/2 プロトコル実装 |
| `libwebsockets` | libwebsockets | WebSocket 接続 |
| `libmosquitto` | libmosquitto | MQTT（IoT メッセージング） |
| `libnss3` | Mozilla NSS | TLS 実装（Firefox 系） |
| `libuv` | libuv | 非同期 I/O（Node.js コア、ネットワーク含む） |

**言語ランタイムライブラリ**

| SOName プレフィックス | 対応言語 | 理由 |
|---|---|---|
| `libruby` | Ruby | スクリプト経由でネットワーク通信可能 |
| `libpython` | Python | 標準ライブラリに socket, urllib, http 等。`libpython3.11.so.1.0` などのバージョン付き SOName も対象に含む |
| `libperl` | Perl | LWP, IO::Socket 等でネットワーク通信可能 |
| `libphp` | PHP | 標準関数に curl, fsockopen 等 |
| `liblua` | Lua | LuaSocket 等の拡張でネットワーク通信可能 |
| `libjvm` | Java VM | Java 標準ライブラリに java.net |
| `libmono` | Mono (.NET) | System.Net でネットワーク通信可能 |
| `libnode` | Node.js (embedded) | Node.js ランタイムの埋め込み |

#### FR-3.2.2: 検出処理のタイミング

`record` コマンド実行時、`DynLibDeps` 解決後に各ライブラリの SOName を既知ネットワークライブラリリストと照合する。

#### FR-3.2.3: 検出結果の記録

SOName が一致したライブラリの一覧を `fileanalysis.SymbolAnalysisData` の新フィールド `KnownNetworkLibDeps []string` に記録する（SOName 文字列のリスト）。

このフィールドが非空の場合、`runner` 実行時にネットワーク有りと判定する。

#### FR-3.2.4: スキーマバージョンの更新

`CurrentSchemaVersion` を 7 → 8 に更新する。

### 3.3 エラー処理

SOName 照合は文字列比較のみであり、ファイル I/O を行わないため専用のエラー処理は不要。

---

## 4. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | `ruby` バイナリを record すると、commandRiskProfiles のヒットによりネットワーク有りと判定される（既存） |
| AC-2 | `luajit` バイナリを record すると commandRiskProfiles のヒットによりネットワーク有りと判定される |
| AC-3 | `java` バイナリを record すると commandRiskProfiles のヒットによりネットワーク有りと判定される |
| AC-4 | `libruby.so` を DT_NEEDED に持つバイナリを record すると `KnownNetworkLibDeps: ["libruby.so.3.2"]` が記録される |
| AC-5 | `libcurl.so` を DT_NEEDED に持つバイナリを record すると `KnownNetworkLibDeps: ["libcurl.so.4"]` が記録される |
| AC-5.1 | `libpython3.11.so.1.0` のようなバージョン付き Python SOName を DT_NEEDED に持つバイナリを record すると `KnownNetworkLibDeps` に記録される |
| AC-6 | `KnownNetworkLibDeps` が非空のバイナリは runner 実行時にネットワーク有りと判定される |
| AC-7 | `libstdc++.so` や `libz.so` など既知ネットワークライブラリリストに含まれないライブラリは `KnownNetworkLibDeps` に記録されない |
| AC-8 | `libpythonista.so` のように登録プレフィックスで始まっても区切り条件を満たさないライブラリは `KnownNetworkLibDeps` に記録されない |
| AC-9 | 既存の symbol 検出（`socket()` の直接インポート等）の動作が変わらないこと |
