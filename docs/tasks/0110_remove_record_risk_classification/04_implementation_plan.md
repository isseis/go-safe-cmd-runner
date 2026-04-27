# 実装計画書: record 側リスク分類フィールドの除去

## 1. 実装概要

### 1.1 目的

要件定義書・アーキテクチャ設計書・詳細仕様書に基づき、`SyscallInfo.IsNetwork` と `DetectedSymbolEntry.Category` を JSON スキーマから除去する。record を「観測事実の記録者」、runner を「リスク判断の実施者」という責務分担に整合させる。

### 1.2 実装原則

1. **削除中心**: 新規ロジックの追加より既存コードの削除を優先する
2. **行動不変**: `IsNetworkOperation` の返り値を変更前後で同一に保つ
3. **コンパイルエラー活用**: フィールド削除後のコンパイルエラーを手がかりに修正箇所を特定する
4. **テストファースト**: 各フェーズで AC 対応テストを確認しながら進める

### 1.3 参照ドキュメント

- 仕様整合の基準: `01_requirements.md`
- 設計整合の基準: `02_architecture.md`
- 実装詳細と AC 対応の基準: `03_detailed_specification.md`

---

## 2. 実装スコープ

### 2.1 変更対象ファイル（プロダクションコード）

| ファイル | 変更概要 |
|---------|---------|
| `internal/common/syscall_types.go` | `SyscallInfo.IsNetwork` フィールド削除 |
| `internal/common/syscall_grouping.go` | `IsNetwork` 伝播ロジック削除 |
| `internal/fileanalysis/schema.go` | `DetectedSymbolEntry.Category` 削除、スキーマバージョン 18 |
| `internal/filevalidator/validator.go` | `IsNetwork: false` / `Category: s.Category` の設定を削除 |
| `internal/runner/security/elfanalyzer/syscall_analyzer.go` | `info.IsNetwork = ...` 2 箇所を削除 |
| `internal/runner/security/machoanalyzer/pass1_scanner.go` | `IsNetwork: ...` を削除 |
| `internal/runner/security/machoanalyzer/pass2_scanner.go` | `IsNetwork: ...` を削除 |
| `internal/libccache/adapters.go` | `IsNetwork: ...` を削除 |
| `internal/libccache/matcher.go` | `IsNetwork: ...` 2 箇所を削除 |
| `internal/runner/security/binaryanalyzer/network_symbols.go` | `IsNetworkSymbolName` ヘルパー追加 |
| `internal/runner/security/network_analyzer.go` | `syscallAnalysisHasNetworkSignal`・`convertNetworkSymbolEntries`・`hasNetworkSymbol` チェックを変更 |

### 2.2 変更対象ファイル（テストコード）

| ファイル | 対応方針 |
|---------|---------|
| `internal/common/syscall_types_test.go` | `IsNetwork` フィールドへの参照を削除 |
| `internal/common/syscall_grouping_test.go` | `IsNetwork` フィールドへの参照を削除 |
| `internal/fileanalysis/syscall_store_test.go` | `SyscallInfo.IsNetwork` への参照を削除 |
| `internal/fileanalysis/network_symbol_store_test.go` | `DetectedSymbolEntry.Category` への参照を削除 |
| `internal/filevalidator/validator_test.go` | `SyscallInfo.IsNetwork` への参照を削除 |
| `internal/filevalidator/validator_macho_test.go` | `SyscallInfo.IsNetwork` への参照を削除 |
| `internal/libccache/adapters_macho_test.go` | `SyscallInfo.IsNetwork` への期待値を削除 |
| `internal/libccache/matcher_test.go` | `SyscallInfo.IsNetwork` への期待値を削除 |
| `internal/libccache/integration_darwin_test.go` | `SyscallInfo.IsNetwork` への参照を削除 |
| `internal/runner/security/syscall_store_adapter_test.go` | `SyscallInfo.IsNetwork` への参照を削除 |
| `internal/runner/security/command_analysis_test.go` | `DetectedSymbolEntry.Category` への参照を削除 |
| `internal/runner/security/network_analyzer_test.go` | `syscallAnalysisHasNetworkSignal` / `Category` 参照を更新 |

---

## 3. 実装フェーズ

### Phase 1: 共通型・スキーマのフィールド削除

**目的**: 削除対象フィールドを型から除去してコンパイルエラーを発生させ、後続フェーズの修正範囲を明確にする。

対象:
- `internal/common/syscall_types.go`
- `internal/common/syscall_grouping.go`
- `internal/fileanalysis/schema.go`

作業内容:
- [ ] `SyscallInfo` から `IsNetwork bool \`json:"is_network"\`` を削除する
- [ ] `SyscallAnalysisResultCore` コメントの `IsNetwork` 言及を整理する
- [ ] `GroupAndSortSyscalls` から `IsNetwork:` フィールド設定と `if info.IsNetwork` ブロックを削除する
- [ ] `DetectedSymbolEntry` から `Category string \`json:"category"\`` を削除する
- [ ] `CurrentSchemaVersion` を 18 に更新し、バージョン履歴コメントに v18 の説明を追記する

成功条件:
- `go build ./internal/common/...` および `go build ./internal/fileanalysis/...` が通る
- 他パッケージのビルドでコンパイルエラーが発生する（後続フェーズで修正予定）

### Phase 2: プロデューサー側の `IsNetwork` / `Category` 設定の削除

**目的**: record 側の分類ロジックを除去する。

対象:
- `internal/runner/security/elfanalyzer/syscall_analyzer.go`
- `internal/runner/security/machoanalyzer/pass1_scanner.go`
- `internal/runner/security/machoanalyzer/pass2_scanner.go`
- `internal/libccache/adapters.go`
- `internal/libccache/matcher.go`
- `internal/filevalidator/validator.go`

作業内容:
- [ ] `elfanalyzer/syscall_analyzer.go`：`info.IsNetwork = table.IsNetworkSyscall(...)` を 2 箇所削除する
- [ ] `machoanalyzer/pass1_scanner.go`：`SyscallInfo` 初期化の `IsNetwork: ...` を削除する
- [ ] `machoanalyzer/pass2_scanner.go`：`SyscallInfo` 初期化の `IsNetwork: ...` を削除する
- [ ] `libccache/adapters.go`：`SyscallInfo` 初期化の `IsNetwork: ...` を削除する
- [ ] `libccache/matcher.go`：`SyscallInfo` 初期化の `IsNetwork: ...` を 2 箇所削除する
- [ ] `filevalidator/validator.go`：`buildSVCInfos` の `IsNetwork: false` を削除する
- [ ] `filevalidator/validator.go`：`convertDetectedSymbols` の `Category: s.Category` を削除する（`DetectedSymbolEntry{Name: s.Name}` のみにする）

成功条件:
- `go build ./internal/runner/security/elfanalyzer/...` など各パッケージのビルドが通る
- `internal/runner/security/network_analyzer.go` のビルドはまだ失敗してよい（Phase 3 で修正）

### Phase 3: コンシューマー側の判定ロジック変更

**目的**: runner が観測事実（syscall 番号・シンボル名）からリスク分類を導出する実装に置き換える。

対象:
- `internal/runner/security/binaryanalyzer/network_symbols.go`
- `internal/runner/security/network_analyzer.go`

作業内容:
- [ ] `binaryanalyzer/network_symbols.go` に `IsNetworkSymbolName(name string) bool` を追加する
- [ ] `network_analyzer.go` に `syscallTableInterface` インターフェースを定義する
- [ ] `network_analyzer.go` に `syscallTableForArch(arch string) syscallTableInterface` を実装する（darwin は `libccache.MacOSSyscallTable{}`、linux は arch に応じて選択、不明は `nil`）
- [ ] `network_analyzer.go` の import に `libccache` を追加する（循環参照がないことを `go build` で確認する）
- [ ] `syscallAnalysisHasNetworkSignal` を `s.IsNetwork` 参照から `table.IsNetworkSyscall(s.Number)` へ変更する（`table == nil` の場合は `false` を返す）
- [ ] `isNetworkViaBinaryAnalysis` 内の `hasNetworkSymbol` チェックを `binaryanalyzer.IsNetworkCategory(sym.Category)` から `binaryanalyzer.IsNetworkSymbolName(sym.Name)` に変更する
- [ ] `convertNetworkSymbolEntries` を `e.Category` 転写からランタイム導出へ変更する（`IsNetworkSymbol(e.Name)` を優先し、未分類時は `IsDynamicLoadSymbol(e.Name)` なら `dynamic_load`、それ以外は `syscall_wrapper` を設定）

成功条件:
- `go build ./...` がエラーなしで通る
- `make fmt` を実行してフォーマットエラーがないことを確認する

### Phase 4: 既存テストの修正

**目的**: Phase 1〜3 の変更によるコンパイルエラーをテストコードで解消する。

対象: 「2.2 変更対象ファイル（テストコード）」に列挙した全ファイル

作業内容:
- [ ] `common/syscall_types_test.go`：`IsNetwork` フィールドへの参照を削除・更新する
- [ ] `common/syscall_grouping_test.go`：`IsNetwork` フィールドへの参照を削除・更新する
- [ ] `fileanalysis/syscall_store_test.go`：`SyscallInfo` 構造体リテラルの `IsNetwork:` を削除する
- [ ] `fileanalysis/network_symbol_store_test.go`：`DetectedSymbolEntry` 構造体リテラルの `Category:` を削除する
- [ ] `filevalidator/validator_test.go`：`SyscallInfo` 構造体リテラルの `IsNetwork:` を削除する
- [ ] `filevalidator/validator_macho_test.go`：`SyscallInfo` 構造体リテラルの `IsNetwork:` を削除する
- [ ] `libccache/adapters_macho_test.go`：`SyscallInfo` の期待値から `IsNetwork:` を削除する
- [ ] `libccache/matcher_test.go`：`SyscallInfo` の期待値から `IsNetwork:` を削除する
- [ ] `libccache/integration_darwin_test.go`：`SyscallInfo.IsNetwork` への参照を削除する
- [ ] `runner/security/syscall_store_adapter_test.go`：`SyscallInfo.IsNetwork` への参照を削除する
- [ ] `runner/security/command_analysis_test.go`：`DetectedSymbolEntry.Category` への参照を削除する
- [ ] `runner/security/network_analyzer_test.go`：`syscallAnalysisHasNetworkSignal` 呼び出しと `Category` 参照を更新する

成功条件:
- `go test -tags test -run . ./...` がコンパイルエラーなしで実行できる（テスト失敗は Phase 5 で対処）

### Phase 5: 新規テストの追加

**目的**: 変更後の動作を AC 単位で検証するテストを追加する。

対象:
- `internal/common/syscall_types_test.go`
- `internal/fileanalysis/schema_test.go`（新規またはすでに存在するファイル）
- `internal/runner/security/network_analyzer_test.go`
- `internal/runner/security/binaryanalyzer/network_symbols_test.go`

作業内容:
- [ ] AC-1: `TestSyscallInfo_JSONDoesNotContainIsNetwork` を追加する
- [ ] AC-2: `TestDetectedSymbolEntry_JSONDoesNotContainCategory` を追加する
- [ ] AC-3a: `TestSyscallAnalysisHasNetworkSignal_NetworkSyscall` を追加する（OS条件付きの syscall 番号を使用: 例 Linux x86_64 socket=41 / Darwin socket=97）
- [ ] AC-3b: `TestSyscallAnalysisHasNetworkSignal_NonNetworkSyscall` を追加する（OS条件付きの syscall 番号を使用: 例 Linux x86_64 write=1 / Darwin write=4）
- [ ] AC-4: `TestCurrentSchemaVersion`（値が 18）を追加する
- [ ] AC-4: `TestLoad_SchemaVersion17_ReturnsSchemaVersionMismatchError` を追加する
- [ ] AC-7a: `TestSyscallAnalysisHasNetworkSignal_UnknownArch`（mips → ネットワーク検知スキップで false、fail-open）を追加する
- [ ] AC-7b: `TestSyscallAnalysisHasNetworkSignal_NegativeNumber`（Number=-1 → ネットワーク検知スキップで false、fail-open）を追加する
- [ ] AC-7c: `TestSyscallAnalysisHasNetworkSignal_Nil`（nil → ネットワーク検知スキップで false、fail-open）を追加する
- [ ] `TestIsNetworkSymbolName`（dns シンボル/syscall_wrapper/未知シンボルの判定）を追加する

成功条件:
- 上記テストがすべて成功する
- 既存テストが回帰しない

### Phase 6: 回帰確認と文書整合チェック

**目的**: 全テスト・lint を通過させ、ドキュメントと実装の整合を確認する。

対象: 変更済みコード全体

作業内容:
- [ ] `make fmt` を実行してフォーマットエラーがないことを確認する
- [ ] `make test` を実行して全テストが通過することを確認する
- [ ] `make lint` を実行してリンターエラーがないことを確認する
- [ ] AC-3（行動不変性）を既存テスト（`network_analyzer_test.go` 等）の通過で確認する
- [ ] AC-5（スキーマ自動移行）を `Store.Update` 既存テストの通過で確認する
- [ ] 実装計画書のチェックボックスをすべて完了に更新する

成功条件:
- `make fmt` / `make test` / `make lint` がすべてエラーなしで完了する

---

## 4. 受け入れ基準トレーサビリティ

| AC | 検証方法 | 主対象テスト |
|----|---------|------------|
| AC-1 | JSON 出力に `is_network` が含まれないこと | `TestSyscallInfo_JSONDoesNotContainIsNetwork` |
| AC-2 | JSON 出力に `category` が含まれないこと | `TestDetectedSymbolEntry_JSONDoesNotContainCategory` |
| AC-3 | ネットワーク検出結果が変更前後で同一 | `TestSyscallAnalysisHasNetworkSignal_*` + 既存統合テストの回帰確認 |
| AC-4 | `CurrentSchemaVersion == 18`、v17 で `SchemaVersionMismatchError` | `TestCurrentSchemaVersion` / `TestLoad_SchemaVersion17_*` |
| AC-5 | v17 レコードが `--force` なしで上書き可能 | `Store.Update` 既存テストの回帰確認 |
| AC-6 | `make test` / `make lint` が通過 | CI 相当コマンドの実行 |
| AC-7 | 不明アーキテクチャ・負値番号・nil ではネットワーク検知をスキップし `false` を返す（fail-open） | `TestSyscallAnalysisHasNetworkSignal_{UnknownArch,NegativeNumber,Nil}` |

---

## 5. 設計整合チェックポイント

- `02_architecture.md` の方針どおり、syscall テーブル選択は `syscallTableForArch` に閉じ込める
- `03_detailed_specification.md` のファイル一覧と一致する変更のみ実施する
- runner 内部の `binaryanalyzer.DetectedSymbol.Category` は削除しない（ログ出力で使用）
- `DynamicLoadSymbols` フィールドは変更しない（別リスクシグナルのため）

---

## 6. リスクと緩和策

| リスク | 影響 | 緩和策 |
|--------|------|--------|
| `libccache` import による循環参照 | ビルド不可 | `go build ./...` で早期確認、循環があれば `syscallTableInterface` の定義場所を調整する |
| `arm64` が Linux/macOS で同一アーキテクチャ文字列を使用 | ネットワーク判定誤り | `syscallTableForArch` で `runtime.GOOS` を優先判定し、darwin では常に `MacOSSyscallTable` を返す |
| 既存テストの `IsNetwork`/`Category` 参照漏れ | コンパイルエラーで CI 失敗 | Phase 1 後にコンパイルエラー一覧を取得して修正漏れを確認する |
| `syscallAnalysisHasNetworkSignal` の動作変化 | AC-3 不達 | `TestSyscallAnalysisHasNetworkSignal_NetworkSyscall` を追加して等価性を検証する |

---

## 7. 完了条件

- [ ] 変更対象ファイルの実装が完了している
- [ ] AC-1〜AC-7 の検証がテストで確認できる
- [ ] `make fmt` / `make test` / `make lint` が成功する
- [ ] 仕様書（要件・設計・詳細仕様）と実装の乖離がない
