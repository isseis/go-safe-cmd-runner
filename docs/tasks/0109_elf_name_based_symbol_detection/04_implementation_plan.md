# 実装計画書: ELF バイナリ VERNEED なし時の名前ベースシンボル検出

## 1. 実装方針

詳細仕様書（`03_detailed_specification.md`）に従い、`checkDynamicSymbols` に FR-2 二段階フィルタを実装する。変更対象は 3 ファイルのみで、テストも同一パッケージ内で完結する。

## 2. 実装タスク

### Phase 1: テストヘルパーの追加

**対象ファイル**: `internal/security/elfanalyzer/testing/helpers.go`

- [x] `SymbolSpec` 型を定義する（`Name string` フィールドのみ）
- [x] `CreateELFWithSymbols(t, path, symbols []SymbolSpec)` 関数を実装する
  - `null` / `.dynsym` / `.dynstr` / `.shstrtab` の 4 セクションのみ生成（VERNEED なし）
  - 各シンボルを `SHN_UNDEF`・`STT_FUNC`・`STB_GLOBAL` として `.dynsym` に追加する
  - バイナリレイアウト: ELF header → section headers → `.dynsym` → `.dynstr` → `.shstrtab`
  - 詳細は詳細仕様書 3 節の実装コードを参照
- [x] `CreateDynamicELFFile` を `CreateELFWithSymbols` に委譲してコードの重複を排除する
  - `CreateDynamicELFFile(t, path)` → `CreateELFWithSymbols(t, path, []SymbolSpec{{Name: "__libc_start_main"}})`
  - 既存のテストが `CreateDynamicELFFile` を呼んでいる箇所は変更不要（シグネチャは維持）

### Phase 2: 本体の変更

**対象ファイル**: `internal/security/elfanalyzer/standard_analyzer.go`

- [x] `checkDynamicSymbols` の関数コメントを更新する（詳細仕様書 2.3 参照）
- [x] `hasVERNEED` スキャンを削除し、`hasAnyUndef` のスキャン 1 本に置き換える（詳細仕様書 2.1 参照）
- [x] シンボル分類ループを FR-2 二段階フィルタに置き換える（詳細仕様書 2.1 参照）
  - **Step 1**: `a.networkSymbols[sym.Name]` が存在すれば対応するカテゴリで `detected` に追加する
  - **Step 2**: Step 1 で不一致の場合、`isLibcLibrary(sym.Library)` が true なら `syscall_wrapper` カテゴリで `detected` に追加する
  - `IsDynamicLoadSymbol` 判定は変更なし
- [x] `categorizeELFSymbol` 関数を削除する（詳細仕様書 2.2 参照）

### Phase 3: 既存テストの更新

**対象ファイル**: `internal/security/elfanalyzer/analyzer_test.go`

- [x] `TestStandardELFAnalyzer_AnalyzeNetworkSymbols` の `"binary with ssl symbols"` ケースの `expectedResult` を `NoNetworkSymbols` → `NetworkDetected` に更新する（詳細仕様書 4.1 参照、AC-5 bullet 3 対応）
- [x] `TestStandardELFAnalyzer_LibcSymbolFiltering` の `"non-libc symbols are not recorded"` サブテストを更新する（詳細仕様書 4.1 参照、AC-5 bullet 3 対応）
  - サブテスト名を `"non-libc network symbols recorded with correct category"` にリネームする
  - `SSL_CTX_new` が `NotEqual` → `SSL_CTX_new` が `tls` カテゴリで記録されることを assert する

### Phase 4: 新規テストの追加

**対象ファイル**: `internal/security/elfanalyzer/analyzer_test.go`

- [x] `TestCheckDynamicSymbols_NameBasedFilter` 関数を追加する（詳細仕様書 4.2 参照）
  - [x] `"no-VERNEED binary importing socket yields NetworkDetected with socket category"` サブテスト
  - [x] `"no-VERNEED binary importing SSL_CTX_new yields NetworkDetected with tls category"` サブテスト
  - [x] `"no-VERNEED binary importing only non-network symbols yields NoNetworkSymbols"` サブテスト
  - [x] `"no-VERNEED binary with mixed symbols records only networkSymbols matches"` サブテスト
  - [x] `"dlopen in no-VERNEED binary appears in DynamicLoadSymbols"` サブテスト

### Phase 5: 品質確認（AC-7 対応）

- [x] `make fmt` を実行して Go フォーマットを確認する
- [x] `make test` を実行してすべてのテストが通ることを確認する
- [x] `make lint` を実行してリンターエラーがないことを確認する

## 3. AC 対応表

| AC | 対応 Phase | 対応テスト・確認方法 |
|----|-----------|-----------------|
| AC-1 | Phase 4 | `TestCheckDynamicSymbols_NameBasedFilter/"no-VERNEED binary importing socket..."` |
| AC-2 | Phase 4 | `TestCheckDynamicSymbols_NameBasedFilter/"no-VERNEED binary importing SSL_CTX_new..."` |
| AC-3 | Phase 4 | `TestCheckDynamicSymbols_NameBasedFilter/"no-VERNEED binary importing only non-network..."` |
| AC-4 | Phase 4 | `TestCheckDynamicSymbols_NameBasedFilter/"no-VERNEED binary with mixed symbols..."` |
| AC-5 (bullet 1: libc+network → network カテゴリ) | Phase 3 | `TestStandardELFAnalyzer_LibcSymbolFiltering/"libc network symbol has socket category"` (既存、変更なし) |
| AC-5 (bullet 2: libc+非 network → syscall_wrapper) | Phase 3 | `TestStandardELFAnalyzer_LibcSymbolFiltering/"non-network libc symbols have syscall_wrapper category"` (既存、変更なし) |
| AC-5 (bullet 3: libssl+SSL_CTX_new → tls) | Phase 3 | `TestStandardELFAnalyzer_AnalyzeNetworkSymbols/"binary with ssl symbols"`、`TestStandardELFAnalyzer_LibcSymbolFiltering/"non-libc network symbols recorded with correct category"` (更新) |
| AC-5 (bullet 4: 非 libc+非 network → 記録なし) | Phase 4 | `"no-VERNEED binary with mixed symbols..."` で pthread_create 不記録を検証（VERNEED ありバイナリでの explicit テストなし） |
| AC-5 (bullet 5: VERNEED あり + Library="" + network → 記録) | — | Step 1 が Library の値によらず適用されるため設計上保証。explicit テストなし |
| AC-6 | Phase 4 | `TestCheckDynamicSymbols_NameBasedFilter/"dlopen in no-VERNEED binary..."` |
| AC-7 | Phase 5 | `make test` / `make lint` |
