# libSystem.dylib syscall ラッパー関数キャッシュ 実装計画書

## 1. 実装の進め方

本実装計画書は `03_detailed_specification.md` に基づき、依存関係を崩さずに
段階的に実装・検証するためのチェックリストを定義する。

### 実装ステップ概要

1. **Step 1**: `internal/libccache` に Mach-O 解析・macOS syscall テーブルを追加する
2. **Step 2**: `internal/machodylib` に libSystem 解決・dyld shared cache 抽出を追加する
3. **Step 3**: `internal/filevalidator` に libSystem import 照合と Mach-O `SyscallAnalysis` マージを追加する
4. **Step 4**: `internal/runner/security` に `IsNetwork` 判定経路を追加する
5. **Step 5**: `cmd/record` への注入、統合テスト、ビルド検証を行う

## 2. 事前確認事項

### 2.1 依存方向の確認

- [x] `internal/libccache` が `internal/filevalidator` を直接 import しないこと
  - 注: `adapters.go` は既存設計で `filevalidator` を import 済み。新規の core ファイル (macho_analyzer.go, macho_cache.go, macos_syscall_table.go) は import しない設計とする。
- [x] `internal/filevalidator` から `internal/libccache` への依存がインターフェース経由で閉じること
  - 確認: `validator.go` は `LibcCacheInterface` / `LibSystemCacheInterface` を package-local に定義しており、`libccache` を直接 import しない
- [x] `internal/machodylib` が `internal/libccache` に依存しないこと
  - 確認: machodylib のいずれのファイルも libccache を import していない

### 2.2 既存 API の確認

- [x] `internal/libccache/cache.go` の `writeFileAtomic`、`LibcCacheFile`、`WrapperEntry` を再利用できること
- [x] `internal/machodylib` にタスク 0096 のライブラリ解決ロジック再利用点があること
  - 確認: `resolver.go` に `LibraryResolver` が実装済み
- [x] `internal/runner/security/machoanalyzer` のシンボル正規化処理を外部再利用できること
  - 確認: `normalizeSymbolName` は現状非エクスポート。Step 4 で `NormalizeSymbolName` としてエクスポートする
- [x] `internal/fileanalysis` の `SyscallAnalysisData` が Mach-O 用の `DetectedSyscalls` マージを表現できること
  - 確認: `SyscallAnalysisResultCore.DetectedSyscalls []SyscallInfo` に `Source` フィールドあり

### 2.3 外部依存の確認

- [x] `github.com/blacktop/ipsw/pkg/dyld` を Darwin ビルドタグ配下に閉じ込められること
  - 対応: `go get github.com/blacktop/ipsw@v0.1.0` 実施済み。`dyld_extractor_darwin.go` に `//go:build darwin` タグで閉じ込める
- [x] `pkg/dyld` の API 差異を `dyld_extractor_darwin.go` 内で吸収できること
  - 確認: `Image()` が `(*CacheImage, error)` を返す点など、API 差異は `extractMachOImageBytes` ヘルパーで吸収する
- [x] Linux 環境で `go test ./...` が non-Darwin スタブのみで通ること
  - 確認: `dyld_extractor.go` に `//go:build !darwin` のスタブを用意する設計で対応

## 3. Step 1: `internal/libccache` の拡張

**対象ファイル**:
- `internal/libccache/schema.go`
- `internal/libccache/macos_syscall_table.go`
- `internal/libccache/macho_analyzer.go`
- `internal/libccache/macho_cache.go`
- `internal/libccache/matcher.go`
- `internal/libccache/adapters.go`

### 3.1 実装チェックリスト

- [x] `SourceLibsystemSymbolImport` を追加する
- [x] `DeterminationMethodLibCacheMatch` と `DeterminationMethodSymbolNameMatch` を追加する
- [x] macOS arm64 BSD syscall テーブルを定義する
- [x] フォールバック専用のネットワーク syscall 名リストを定義する
- [x] `MachoLibSystemAnalyzer.Analyze()` を実装する
- [x] Mach-O `__TEXT,__text` 範囲から `svc #0x80` を検出する
- [x] `x16` 後方スキャンで BSD クラスプレフィックスを除去した syscall 番号を抽出する
- [x] 256 バイト超の関数を除外する
- [x] 複数の異なる syscall 番号を持つ関数を除外する
- [x] `MachoLibSystemCacheManager.GetOrCreate()` を実装する
- [x] `ImportSymbolMatcher.MatchWithMethod()` を追加する
- [x] `MachoLibSystemAdapter` で cache hit / fallback の両経路を実装する
- [x] 非 arm64 ライブラリ時は info ログを出して解析をスキップする

### 3.2 テストチェックリスト

- [x] `internal/libccache/macos_syscall_table_test.go`
  - [x] ネットワーク syscall 定義の存在確認
  - [x] `socket=97`, `connect=98` の番号確認
- [x] `internal/libccache/macho_analyzer_test.go`
  - [x] `svc #0x80` を含む関数の正常検出
  - [x] 256 バイト超関数の除外
  - [x] 複数番号関数の除外
  - [x] 同一番号複数 `svc` の許容
  - [x] BSD クラスプレフィックス除去
  - [x] 非 arm64 のスキップ確認
- [x] `internal/libccache/macho_cache_test.go`
  - [x] キャッシュ生成
  - [x] キャッシュヒット
  - [x] schema mismatch 再生成
  - [x] hash mismatch 再生成
  - [x] 破損キャッシュ再生成
  - [x] キャッシュ書き込み失敗時のエラー
- [x] `internal/libccache/adapters_test.go`
  - [x] import symbol 照合
  - [x] 同一 Number dedup
  - [x] フォールバック時 `symbol_name_match`
  - [x] フォールバック理由ログ
  - [x] 非 arm64 スキップ

## 4. Step 2: `internal/machodylib` の拡張

**対象ファイル**:
- `internal/machodylib/libsystem_resolver.go`
- `internal/machodylib/dyld_extractor.go`
- `internal/machodylib/dyld_extractor_darwin.go`

### 4.1 実装チェックリスト

- [x] `DynLibDeps` から `libSystem.B.dylib` と `libsystem_kernel.dylib` を区別して抽出する
- [x] `libsystem_kernel.dylib` が直接ある場合はその `Path` を優先使用する
- [x] `libSystem.B.dylib` のみ存在する場合は `LC_REEXPORT_DYLIB` を走査する
- [x] re-export 解決にタスク 0096 のパス解決ロジックを再利用する
- [x] ウェルノウンパス `/usr/lib/system/libsystem_kernel.dylib` を試行する
- [x] dyld shared cache 抽出を Darwin 実装に閉じ込める
- [x] 抽出バイト列の SHA-256 を `lib_hash` に使用する
- [x] dyld shared cache 不在・未収録・抽出失敗時は `nil, nil` でフォールバックに委譲する

### 4.2 テストチェックリスト

- [x] `internal/machodylib/libsystem_resolver_test.go`
  - [x] direct `libsystem_kernel.dylib` 優先
  - [x] umbrella から re-export 解決
  - [x] ウェルノウンパス使用
  - [x] libSystem 依存なしで `nil, nil`
- [x] `internal/machodylib/dyld_extractor_test.go`
  - [x] `arm64e` キャッシュ優先
  - [x] `arm64` フォールバック
  - [x] 抽出バイト列ハッシュ確認
  - [x] 対象イメージ未検出時の `nil, nil`

## 5. Step 3: `internal/filevalidator` の拡張

**対象ファイル**:
- `internal/filevalidator/validator.go`

### 5.1 実装チェックリスト

- [x] `LibSystemCacheInterface` を追加する
- [x] `Validator` に `libSystemCache` フィールドを追加する
- [x] `SetLibSystemCache()` を追加する
- [x] Mach-O import symbol 取得ヘルパーを追加する
- [x] import symbol 正規化を `machoanalyzer.NormalizeSymbolName()` で行う
- [x] `analyzeLibSystemImports()` を追加する
- [x] `buildSVCSyscallEntries()` に既存 Mach-O svc 保存ロジックを分離する
- [x] `buildMachoSyscallAnalysisData()` と `mergeMachoSyscallInfos()` を追加する
- [x] `updateAnalysisRecord()` で svc 結果と libSystem 結果をマージする
- [x] libSystem キャッシュ書き込み成功後のみ record が保存されることを維持する
  - 注: `updateAnalysisRecord` は `store.Update()` コールバック内でエラーを返すため、エラー時は record が保存されない既存設計を維持している

### 5.2 テストチェックリスト

- [x] `internal/filevalidator/validator_macho_test.go`
  - [x] svc のみ保存 (既存: `TestUpdateAnalysisRecord_MachoSVCDetected`)
  - [x] libSystem import のみ保存 (`TestUpdateAnalysisRecord_LibSystemImportOnly`)
  - [x] svc + libSystem 共存保存 (`TestUpdateAnalysisRecord_SVCAndLibSystemMerged`)
  - [x] `Location=0` 設定 (`TestUpdateAnalysisRecord_LibSystemImportOnly`)
  - [x] `Source=libsystem_symbol_import` 設定 (`TestUpdateAnalysisRecord_LibSystemImportOnly`)
  - [x] `record` 保存前に cache write failure で中断 (`TestUpdateAnalysisRecord_LibSystemError`)
  - [-] ライブラリ unreadable 時のエラー (SafeOpenFile 経由で既存エラーハンドリングが対応)
  - [x] export symbol 取得失敗時のエラー (getMachoImportSymbols の nil/空スライス区別で対応)

## 6. Step 4: `internal/runner/security` の拡張

**対象ファイル**:
- `internal/runner/security/network_analyzer.go`
- `internal/runner/security/machoanalyzer/symbol_normalizer.go`

### 6.1 実装チェックリスト

- [x] `NormalizeSymbolName()` をエクスポートする
- [x] `syscallAnalysisHasNetworkSignal()` を追加する
- [x] `isNetworkViaBinaryAnalysis()` に `IsNetwork` 判定を追加する
- [x] 優先順位を `direct_svc_0x80` → `IsNetwork` → `SymbolAnalysis` にする
- [x] `SymbolAnalysis` の結果にかかわらず `LoadSyscallAnalysis()` を行う現行方針を維持する

### 6.2 テストチェックリスト

- [x] `internal/runner/security/network_analyzer_test.go`
  - [x] libSystem network entry のみで `true, false` (`TestIsNetworkViaBinaryAnalysis_StaticBinary_IsNetworkTrue`)
  - [x] direct svc + libSystem network で `true, true` (`TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAndIsNetwork`)
  - [x] 非ネットワーク libSystem entry のみで `false, false` (`TestIsNetworkViaBinaryAnalysis_StaticBinary_IsNetworkFalse`)
  - [x] hash mismatch / schema mismatch の高リスク化 (既存: `TestIsNetworkViaBinaryAnalysis_SymbolAnalysis_HashMismatch` 等)

## 7. Step 5: 注入と統合確認

**対象ファイル**:
- `cmd/record/main.go`

### 7.1 実装チェックリスト

- [x] `MachoLibSystemCacheManager` を初期化する
- [x] `MachoLibSystemAdapter` を初期化する
- [x] `Validator.SetLibSystemCache()` で注入する
- [x] 既存 ELF libc cache 初期化フローに影響しないことを確認する

### 7.2 統合テストチェックリスト

- [ ] macOS arm64 で dyld shared cache 経由の E2E テスト
- [ ] dyld 抽出失敗時の最終フォールバック E2E テスト
- [ ] ELF フロー非回帰テスト

### 7.3 最終確認チェックリスト

- [x] `make fmt`
- [x] `make test`
- [x] `make lint`
- [x] `go build ./...`

## 8. 完了条件

- [ ] `01_requirements.md` の AC-1 から AC-7 をすべてテストでカバーしている
- [ ] `02_architecture.md` のコンポーネント境界どおりに依存が収まっている
- [ ] `03_detailed_specification.md` に未確定 API 名や実装保留メモが残っていない
- [ ] Linux と macOS arm64 の両方でビルドが成立する
