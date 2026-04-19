# Mach-O arm64 svc #0x80 キャッシュ統合・キャッシュ優先判定 実装計画書

## 1. 実装の進め方

本実装計画書は詳細仕様書（`03_detailed_specification.md`）に基づき、
依存関係の順番を考慮した実装手順と進捗管理チェックリストを定義する。

### 実装ステップ概要

1. **Step 1**: `machoanalyzer` パッケージの拡張（依存なし）
2. **Step 2**: `filevalidator` パッケージの拡張（Step 1 に依存）
3. **Step 3**: `network_analyzer` の拡張（fileanalysis パッケージへの依存のみ）
4. **Step 4**: `risk/evaluator.go` の更新（Step 3 に依存）
5. **Step 5**: 統合確認とビルド検証

## 2. 事前確認事項

### 2.1 インポートサイクル確認

- [x] `internal/runner/security/machoanalyzer` が `internal/filevalidator` を import していないこと
  - 確認コマンド: `grep -r "filevalidator" internal/runner/security/machoanalyzer/`
- [x] `internal/filevalidator` が `internal/runner/security/machoanalyzer` を import 可能なこと
  - 確認コマンド: `go build ./internal/filevalidator/...` が通ること

### 2.2 `safefileio.FileSystem.SafeOpenFile` の返り値型確認

- [x] `SafeOpenFile` の返り値型: `safefileio.File` インターフェース
  - `io.Reader`, `io.Writer`, `io.Seeker`, `io.ReaderAt` を実装する
  - `macho.NewFile` および `macho.NewFatFile` に直接渡せる（型アサーション不要）

### 2.3 `fileanalysis.SyscallAnalysisData` の型確認

- [x] `fileanalysis.SyscallAnalysisData` が `common.SyscallAnalysisResultCore` を embed していること
  - ファイル: `internal/fileanalysis/schema.go`

## 3. Step 1: `machoanalyzer` パッケージの拡張

**対象ファイル**: `internal/runner/security/machoanalyzer/svc_scanner.go`

### 3.1 実装チェックリスト

- [x] `svc_scanner.go` に `safefileio` パッケージのインポートを追加する:
  `"github.com/isseis/go-safe-cmd-runner/internal/safefileio"`
- [x] `svc_scanner.go` に `"os"` パッケージのインポートを追加する
- [x] `collectSVCAddresses(f *macho.File) ([]uint64, error)` を実装する
  - [x] `f.Cpu != macho.CpuArm64` の場合は `nil, nil` を返す
  - [x] `__TEXT,__text` セクションが存在しない場合は `nil, nil` を返す
  - [x] セクションデータ読み出しエラー時はエラーを返す
  - [x] 4 バイトアラインで走査し、`svcInstruction` にマッチした仮想アドレスを収集する
  - [x] `section.Addr + uint64(i)` で仮想アドレスを算出する
  - [x] 検出なしの場合は `nil, nil` を返す
- [x] `containsSVCInstruction` を `collectSVCAddresses` に委譲するよう変更する
  - [x] 変更後も既存の動作（bool 返し）を維持する
- [x] `CollectSVCAddressesFromFile(filePath string, fs safefileio.FileSystem) ([]uint64, error)` を実装する
  - [x] `fs.SafeOpenFile` でファイルを開く
  - [x] 先頭 4 バイトでマジック確認を行い、非 Mach-O には `nil, nil` を返す
  - [x] Fat バイナリ: arm64 スライスのみ `collectSVCAddresses` を呼び結果を連結する
  - [x] 単一アーキテクチャ: `collectSVCAddresses` を呼ぶ
  - [x] パースエラー時はエラーを返す

### 3.2 テストチェックリスト

**ファイル**: `internal/runner/security/machoanalyzer/svc_scanner_test.go`（追加分）

- [x] `TestCollectSVCAddresses_Arm64WithSVC`: arm64 + svc #0x80 → アドレスを返す
- [x] `TestCollectSVCAddresses_Arm64NoSVC`: arm64 + svc なし → `nil, nil`
- [x] `TestCollectSVCAddresses_NonArm64`: x86_64 → `nil, nil`
- [x] `TestCollectSVCAddresses_MultipleSVC`: 複数 svc #0x80 → 全アドレスを返す
- [x] `TestCollectSVCAddressesFromFile_NotMacho`: ELF ファイル → `nil, nil`
- [x] `TestCollectSVCAddressesFromFile_FatBinary`: Fat バイナリ → arm64 スライスのみ走査
- [x] `TestContainsSVCInstruction_DelegatesToCollect`: リファクタリング後も正常動作

**実行コマンド**:
```
go test -tags test -v ./internal/runner/security/machoanalyzer/
```

## 4. Step 2: `filevalidator` パッケージの拡張

**対象ファイル**: `internal/filevalidator/validator.go`

### 4.1 実装チェックリスト

- [x] `machoanalyzer` パッケージのインポートを追加する:
  `"github.com/isseis/go-safe-cmd-runner/internal/runner/security/machoanalyzer"`
- [x] `buildSVCSyscallAnalysis(addrs []uint64) *fileanalysis.SyscallAnalysisData` を実装する
  - [x] `Architecture: "arm64"` を設定する
  - [x] `AnalysisWarnings: []string{"svc #0x80 detected: direct syscall bypassing libSystem.dylib"}` を設定する
  - [x] `DetectedSyscalls` に各アドレスを `Number=-1, DeterminationMethod="direct_svc_0x80", Source="direct_svc_0x80"` で記録する
  - [x] `ArgEvalResults` は設定しない（nil のまま）
- [x] `updateAnalysisRecord` のコールバック内、`analyzeSyscalls()` 呼び出し直後に Mach-O svc スキャンを追加する
  - [x] `v.binaryAnalyzer != nil` の条件分岐を追加する（binaryAnalyzer が nil の場合はスキップ）
  - [x] `machoanalyzer.CollectSVCAddressesFromFile(filePath.String(), v.fileSystem)` を呼ぶ
  - [x] エラー時はラップして返す
  - [x] `len(addrs) > 0` の場合のみ `record.SyscallAnalysis = buildSVCSyscallAnalysis(addrs)` を設定する
  - [x] `SymbolAnalysis = NetworkDetected` の場合も svc スキャンを実行すること（`runner` 側で参照可否を制御する）

### 4.2 テストチェックリスト

**ファイル**: `internal/filevalidator/validator_macho_test.go`（新規推奨）

- [x] `TestBuildSVCSyscallAnalysis`: 単体テスト
  - [x] `Architecture == "arm64"` を確認
  - [x] `AnalysisWarnings` に検出メッセージが含まれる
  - [x] `DetectedSyscalls` に正しいフィールドが設定される
- [x] `TestUpdateAnalysisRecord_MachoSVCDetected`: svc ありの Mach-O (NoNetworkSymbols) で SyscallAnalysis が設定される
- [x] `TestUpdateAnalysisRecord_MachoNoSVC`: svc なしの Mach-O で SyscallAnalysis が nil
- [x] `TestUpdateAnalysisRecord_MachoNetworkDetected_SVCDetected`: NetworkDetected + svc あり → SyscallAnalysis が保存される
- [x] `TestUpdateAnalysisRecord_MachoNetworkDetected_NoSVC`: NetworkDetected + svc なし → SyscallAnalysis が nil
- [x] `TestUpdateAnalysisRecord_ELFNotAffected`: ELF バイナリのフロー変更なし（linux のみ、またはモック）

**注意**:
- `debug/macho` はクロスプラットフォームで利用できるため、darwin ビルドタグは不要
- 既存の `validator_test.go` に統合してもよいが、Mach-O 向けケース分離のため専用ファイルを推奨

**実行コマンド**:
```
go test -tags test -v ./internal/filevalidator/
```

## 5. Step 3: `network_analyzer` の拡張

**対象ファイル**: `internal/runner/security/network_analyzer.go`

### 5.1 実装チェックリスト

- [x] `NetworkAnalyzer` 構造体に `syscallStore fileanalysis.SyscallAnalysisStore` フィールドを追加する
- [x] `NewNetworkAnalyzerWithStores` コンストラクタを実装する
  - [x] `symStore` と `svcStore` を受け取り `NetworkAnalyzer` を返す
  - [x] `binaryAnalyzer: NewBinaryAnalyzer()` を設定する
- [x] `syscallAnalysisHasSVCSignal(result *fileanalysis.SyscallAnalysisResult) bool` を実装する
  - [x] nil の場合は `false` を返す
  - [x] `DetectedSyscalls` に `DeterminationMethod == "direct_svc_0x80"` のエントリがある場合のみ `true` を返す
  - [x] `AnalysisWarnings` は判定条件に含めない（ELF 側の警告による誤検知を防ぐ）
- [x] `isNetworkViaBinaryAnalysis` の cache-backed path を書き直し、SVC 判定の live 解析フォールバックを削除する
  - [-] `a.store == nil` / `contentHash == ""` の互換ガード（legacy live 解析経路）を削除する（実装計画書 Section 8 参照）
  - [x] SymbolAnalysis ロードエラー → `return true, true`（AnalysisError、live 解析なし）
  - [x] `a.binaryAnalyzer.AnalyzeNetworkSymbols()` の呼び出しを cache-backed path から削除する
  - [x] SymbolAnalysis ロード成功後、`SymbolAnalysis` の結果に関わらず `a.syscallStore.LoadSyscallAnalysis(cmdPath, contentHash)` を呼ぶ（FR-3.3.1）
  - [x] `svcErr == nil` かつ `syscallAnalysisHasSVCSignal(svcResult)` → `true, true` を返す（`SymbolAnalysis` の結果に関わらず高リスク確定）
  - [x] `svcErr == nil` かつ svc signal なし → `SymbolAnalysis` 結果に基づいて判定を続ける（fall through）
  - [x] `ErrNoSyscallAnalysis` → `SymbolAnalysis` 結果に基づいて判定を続ける（fall through、v15 保証：スキャン済み・svc 未検出）
  - [x] `ErrHashMismatch` → `return true, true`
  - [x] `SchemaVersionMismatchError` → `return true, true`（再 `record` 要求）
  - [x] `ErrRecordNotFound` / その他エラー → `return true, true`（整合性エラー）
  - [x] svc signal なし時の `SymbolAnalysis` 結果判定: `NetworkDetected` → `true, false`、`NoNetworkSymbols` → `false, false`

### 5.2 テストチェックリスト

**ファイル**: `internal/runner/security/network_analyzer_test.go`（追加分）

- [x] `TestSyscallAnalysisHasSVCSignal_Nil`: nil → false
- [x] `TestSyscallAnalysisHasSVCSignal_Empty`: 空の result → false
- [x] `TestSyscallAnalysisHasSVCSignal_WithWarningsOnly`: AnalysisWarnings のみ（DeterminationMethod なし）→ false
- [x] `TestSyscallAnalysisHasSVCSignal_WithDeterminationMethod`: DeterminationMethod == "direct_svc_0x80" → true
- [x] `TestIsNetworkViaBinaryAnalysis_SymbolAnalysisCacheMiss`: SymbolAnalysis ロードエラー → AnalysisError（live 解析なし）
- [x] `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheHit`: NoNetworkSymbols + svc あり → true, true
- [x] `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheNil`: ロード成功・svc なし → false, false
- [x] `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCHashMismatch`: ErrHashMismatch → AnalysisError
- [x] `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCNoSyscallAnalysis`: ErrNoSyscallAnalysis → false, false（フォールバックなし）
- [x] `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCSchemaMismatch`: SchemaVersionMismatchError → AnalysisError
- [x] `TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCRecordNotFound`: ErrRecordNotFound → AnalysisError（live 解析なし）
- [x] `TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCCacheHit`: NetworkDetected + svc あり → true, true（isHighRisk 格上げ）
- [x] `TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCNoSyscallAnalysis`: NetworkDetected + ErrNoSyscallAnalysis → true, false（格上げなし）
- [x] `TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSVC`: NetworkDetected + svc なし（ロード成功）→ true, false（格上げなし）

**実行コマンド**:
```
go test -tags test -v ./internal/runner/security/
```

## 6. Step 4: ストア注入チェーンの更新

**対象ファイル**:
- `internal/verification/manager.go`
- `internal/runner/runner.go`
- `internal/runner/resource/default_manager.go`
- `internal/runner/resource/normal_manager.go`
- `internal/runner/risk/evaluator.go`

### 6.1 実装チェックリスト

- [x] `internal/verification/manager.go` に `GetSyscallAnalysisStore() fileanalysis.SyscallAnalysisStore` を追加する
- [x] `internal/runner/runner.go` で path resolver から `SyscallAnalysisStore` を取得する
  - [x] `networkSymbolStoreProvider` と同様の provider interface を追加するか、共通 provider を拡張する
  - [x] `resource.NewDefaultResourceManager()` 呼び出しに `syscallStore` を渡す
- [x] `internal/runner/resource/default_manager.go` のシグネチャを変更して `syscallStore` を追加する:
  ```go
  func NewDefaultResourceManager(..., store fileanalysis.NetworkSymbolStore, syscallStore fileanalysis.SyscallAnalysisStore) (*DefaultResourceManager, error)
  ```
- [x] `internal/runner/resource/normal_manager.go` のシグネチャを変更して `syscallStore` を追加する:
  `NewNormalResourceManagerWithStores` として実装（後方互換 wrapper `NewNormalResourceManagerWithOutput` を維持）
  ```go
  func NewNormalResourceManagerWithStores(..., symStore fileanalysis.NetworkSymbolStore, syscallStore fileanalysis.SyscallAnalysisStore) *NormalResourceManager
  ```
- [x] `NewStandardEvaluator` のシグネチャを変更して `syscallStore` を追加する:
  `NewStandardEvaluatorWithStores` として実装（後方互換 wrapper `NewStandardEvaluator` を維持）
  ```go
  func NewStandardEvaluatorWithStores(symStore fileanalysis.NetworkSymbolStore, syscallStore fileanalysis.SyscallAnalysisStore) Evaluator
  ```
- [x] `security.NewNetworkAnalyzerWithStore(store)` を
  `security.NewNetworkAnalyzerWithStores(store, syscallStore)` に変更する
- [x] `NewStandardEvaluator` の呼び出し箇所を全て更新する
  - 呼び出し箇所の確認: `grep -r "NewStandardEvaluator" --include="*.go" .`

### 6.2 呼び出し箇所のチェックリスト

- [x] `internal/runner/resource/normal_manager.go` 内の `NewStandardEvaluator` 呼び出しを更新する
- [x] `internal/runner/resource/default_manager.go` から `NewNormalResourceManagerWithOutput` への引数転送を更新する
- [x] `internal/runner/runner.go` の `createNormalResourceManager()` で `SyscallAnalysisStore` を取得して渡す
- [x] `internal/runner/runner_test.go` の path resolver モックに `GetSyscallAnalysisStore()` を追加する
- [x] `internal/runner/resource/*_test.go` のコンストラクタ呼び出しを更新する
- [x] `internal/runner/risk/evaluator_test.go` の `NewStandardEvaluator(nil)` 呼び出しを更新する
  （後方互換 wrapper が維持されているため変更不要）

**実行コマンド**:
```
go build ./...
go test -tags test -v ./internal/runner/risk/
go test -tags test -v ./internal/runner/resource/
go test -tags test -v ./internal/runner/
```

## 7. Step 5: 統合確認

### 7.1 ビルドチェックリスト

- [x] `make build` でビルドエラーなし
- [x] `make lint` でリントエラーなし
- [x] `make fmt` でフォーマット適用後に変更差分なし

### 7.2 テストチェックリスト

- [x] `make test` で全テストパス
  ```
  go test -tags test -v ./...
  ```
- [x] Step 1〜4 で追加したテストがすべてパス

### 7.3 最終確認チェックリスト

- [x] `go vet ./...` でエラーなし
- [x] `make test` が全て GREEN
- [x] `make lint` が全て GREEN
- [x] `make build` が成功

## 8. 後続作業: runner の svc #0x80 live 解析コード削除

本タスクの実装完了後、`runner` が `SyscallAnalysis` キャッシュを利用した cache-backed path で
正常に動作することを確認した上で、既存の live 解析コードを削除する。

### 8.1 削除対象

`isNetworkViaBinaryAnalysis` 内に現在残存する svc #0x80 live 解析コード。
具体的には `a.binaryAnalyzer.AnalyzeNetworkSymbols()` による再判定パスが対象。

互換ガード（`a.store == nil` / `contentHash == ""` 分岐）を本タスクで切り出した場合は、
その互換経路ごと削除対象に含める。

### 8.2 削除のタイミング

- 本タスク（0097）の全 Step（1〜5）完了後
- `make test` が全 GREEN であることを確認後
- 削除は別コミットまたは別 PR として実施する

### 8.3 削除後の確認

- [ ] `a.binaryAnalyzer.AnalyzeNetworkSymbols()` の呼び出しが `isNetworkViaBinaryAnalysis` から完全に除去されていること
- [ ] `make test` が引き続き全 GREEN であること
- [ ] `make lint` でデッドコード警告が出ないこと

## 9. リスクと対策

| リスク | 影響 | 対策 |
|-------|------|------|
| `machoanalyzer` → `filevalidator` インポートサイクル | ビルド不可 | 事前確認（§2.1）で検出。循環が発生する場合は `CollectSVCAddressesFromFile` を別パッケージに移動 |
| `SafeOpenFile` の返り値型が `io.ReaderAt` を実装しない | `macho.NewFile` に渡せない | 事前確認（§2.2）で検出。型アサーションまたは `os.Open` 直接使用への代替を検討 |
| darwin 環境以外でのテスト失敗 | CI の失敗 | darwin ビルドタグに依存せず、Mach-O フィクスチャを使ったクロスプラットフォームテストにする |
| `NewStandardEvaluator` や resource manager の呼び出し箇所の見落とし | コンパイルエラー | `go build ./...` と `go test ./internal/runner/...` で早期検出 |
| path resolver provider の追加漏れ | キャッシュが常に無効化される | `runner.go` と `runner_test.go` に `GetSyscallAnalysisStore()` 経路のテストを追加 |

## 10. 実装順序の根拠

```
machoanalyzer (Step 1)
    ↓ (CollectSVCAddressesFromFile を利用)
filevalidator (Step 2)

fileanalysis (既存・変更なし)
    ↓ (SyscallAnalysisStore を利用)
network_analyzer (Step 3)
    ↓ (NewNetworkAnalyzerWithStores を利用)
risk/evaluator (Step 4)
    ↓
resource/normal_manager
    ↓
resource/default_manager
    ↓
runner.createNormalResourceManager
    ↑ (GetSyscallAnalysisStore を提供)
verification.Manager
    ↓
runner 通常実行パス
```

Step 1 と Step 3 は相互依存がないが、本実装計画書では依存関係と検証順序を優先し、Step 1 から順に実装する。
