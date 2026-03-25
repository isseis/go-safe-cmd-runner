# 実装計画書: 直接依存ライブラリによるネットワーク検出強化

## 進捗状況

- [ ] Phase 1: テスト先行実装
- [ ] Phase 2: 本体実装
- [ ] Phase 3: 動作確認・整合性検証

---

## Phase 1: テスト先行実装

### 1.1 `IsKnownNetworkLibrary` / `matchesKnownPrefix` テスト（`known_network_libs_test.go`）

- [ ] ネットワークライブラリ（libcurl, libssl, libssh, libssh2, libzmq, libnanomsg, libnng, libnghttp2, libwebsockets, libmosquitto, libnss3, libuv）→ true
- [ ] 言語ランタイムライブラリ（libruby, libpython, libperl, libphp, liblua, libjvm, libmono, libmonoboehm, libnode）→ true
- [ ] 除外ライブラリ（libstdc++, libz, libcrypto, libgnutls, libgcrypt, libpthread, libc）→ false
- [ ] バージョン付き SOName（`libruby.so.3.2`, `libpython3.11.so.1.0`, `libcurl.so.4`）→ true
- [ ] Python バージョン付き（`libpython3.11.so.1.0`）→ true（前方一致で `libpython` にマッチ）
- [ ] 紛らわしいケース（`libpythonista.so` 等）→ false

この時点では `IsKnownNetworkLibrary` 未実装のためテストは失敗する（RED）。

### 1.2 commandRiskProfiles テスト（`command_analysis_test.go` または `command_risk_profile_test.go`）

追加するバイナリのうち代表的なもの（各グループから1〜2件）が `NetworkTypeAlways` として登録されていることを確認するテストを追加：

- [ ] `luajit` → NetworkTypeAlways
- [ ] `tclsh` → NetworkTypeAlways
- [ ] `R` → NetworkTypeAlways
- [ ] `julia` → NetworkTypeAlways
- [ ] `guile` → NetworkTypeAlways
- [ ] `erl` → NetworkTypeAlways
- [ ] `elixir` → NetworkTypeAlways
- [ ] `java` → NetworkTypeAlways
- [ ] `groovy` → NetworkTypeAlways
- [ ] `scala` → NetworkTypeAlways
- [ ] `dotnet` → NetworkTypeAlways
- [ ] `pwsh` → NetworkTypeAlways

この時点ではエントリ未追加のためテストは失敗する（RED）。

### 1.3 `filevalidator` 統合テスト

- [ ] `DynLibDeps` に `libcurl.so.4` を含むレコードで `KnownNetworkLibDeps: ["libcurl.so.4"]` が記録される
- [ ] `DynLibDeps` に `libpython3.11.so.1.0` を含むレコードで `KnownNetworkLibDeps` に記録される
- [ ] `DynLibDeps` に `libz.so.1` のみ含む場合は `KnownNetworkLibDeps` が空
- [ ] `SymbolAnalysis` が nil の場合は `KnownNetworkLibDeps` は記録されない

### 1.4 `network_analyzer` テスト

- [ ] `KnownNetworkLibDeps` が非空で `DetectedSymbols` が空の場合、`NetworkDetected` を返す
- [ ] `KnownNetworkLibDeps` が空で `DetectedSymbols` も空の場合、`NoNetworkSymbols` を返す

---

## Phase 2: 本体実装

### 2.1 `known_network_libs.go` 新規作成（`binaryanalyzer` パッケージ）

- [ ] `knownNetworkLibPrefixes` マップを定義（ネットワークライブラリ + 言語ランタイム）
- [ ] `matchesKnownPrefix(soname, prefix string) bool` を実装
  - [ ] `strings.HasPrefix` でプレフィックス確認
  - [ ] `rest[0]` が `.`, `-`, 数字の場合のみ一致（`libpythonista.so` 等の誤検知防止）
- [ ] `IsKnownNetworkLibrary(soname string) bool` を実装
- [ ] `KnownNetworkLibraryCount() int` を実装

### 2.2 `schema.go` 更新（`fileanalysis` パッケージ）

- [ ] `SymbolAnalysisData` に `KnownNetworkLibDeps []string` フィールドを追加（`json:"known_network_lib_deps,omitempty"`）
- [ ] `CurrentSchemaVersion` を 7 → 8 に更新
- [ ] コメントに `// Version 8 adds KnownNetworkLibDeps to SymbolAnalysisData.` を追記

### 2.3 `validator.go` 更新（`filevalidator` パッケージ）

- [ ] `updateAnalysisRecord` のシンボル解析ブロック末尾に SOName 照合ロジックを追加
  - [ ] `record.DynLibDeps != nil && record.SymbolAnalysis != nil` の条件確認
  - [ ] 各 `lib.SOName` を `binaryanalyzer.IsKnownNetworkLibrary()` で照合
  - [ ] 一致した SOName を `record.SymbolAnalysis.KnownNetworkLibDeps` に設定

### 2.4 `command_analysis.go` 更新（`security` パッケージ）

- [ ] Lua 系バイナリを追加（`lua`, `lua5.1`, `lua5.2`, `lua5.3`, `lua5.4`, `luajit`）
- [ ] Tcl/Tk バイナリを追加（`tclsh`, `tclsh8.5`, `tclsh8.6`, `wish`, `wish8.5`, `wish8.6`）
- [ ] R / Julia を追加（`R`, `Rscript`, `julia`）
- [ ] GNU Guile を追加（`guile`, `guile2`, `guile3`）
- [ ] Erlang/Elixir を追加（`elixir`, `iex`, `erl`, `erlc`, `escript`）
- [ ] JVM 系を追加（`java`, `javaw`, `groovy`, `groovysh`, `groovyConsole`, `kotlin`, `scala`, `scala3`, `clojure`, `jruby`, `jython`）
- [ ] .NET 系を追加（`dotnet`, `mono`, `pwsh`, `powershell`）

### 2.5 `network_analyzer.go` 更新（`security` パッケージ）

- [ ] キャッシュ読み込み成功パスの判定条件を更新
  - [ ] `len(data.DetectedSymbols) > 0` → `len(data.DetectedSymbols) > 0 || len(data.KnownNetworkLibDeps) > 0`

---

## Phase 3: 動作確認・整合性検証

### 3.1 テスト実行

- [ ] `go test -tags test -v ./internal/runner/security/binaryanalyzer/...` — Phase 1.1 のテストが GREEN
- [ ] `go test -tags test -v ./internal/filevalidator/...` — Phase 1.3 のテストが GREEN
- [ ] `go test -tags test -v ./internal/runner/security/...` — Phase 1.2 と Phase 1.4 のテストが GREEN
- [ ] `go test -tags test -v ./internal/fileanalysis/...` — スキーマバージョンテストが GREEN
- [ ] `make test` — リポジトリ全体のテストが全て GREEN

### 3.2 コード品質

- [ ] `make fmt` — フォーマット差分がないこと
- [ ] `make lint` — lint エラーがないこと

---

## 実装上の注意事項

### `libpython` の SOName 形式

Python の SOName は `libpython3.11.so.1.0` のようにバージョン番号が `.so` の前に入るため、単純な完全一致では不一致になる。`matchesKnownPrefix` の安全な前方一致 + 区切り文字確認で対応する（仕様書 3.3 参照）。

### `SymbolAnalysis` が nil の場合のスキップ

静的バイナリや非 ELF ファイルでは `binaryAnalyzer.AnalyzeNetworkSymbols()` が `StaticBinary` / `NotSupportedBinary` を返し、`record.SymbolAnalysis` が nil になる。この場合は `KnownNetworkLibDeps` の記録もスキップする（`DynLibDeps` は存在しないことが多いが、万が一存在しても nil チェックで安全にスキップできる）。

### スキーマバージョン変更の波及

`CurrentSchemaVersion` を 8 に変更すると、バージョン 7 以前のレコードは次回 `record` 実行時に自動再解析される（`--force` 不要）。開発環境の既存レコードは無効化されるが、意図した動作である。
