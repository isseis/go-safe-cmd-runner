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

- [ ] `internal/libccache` が `internal/filevalidator` を直接 import しないこと
- [ ] `internal/filevalidator` から `internal/libccache` への依存がインターフェース経由で閉じること
- [ ] `internal/machodylib` が `internal/libccache` に依存しないこと

### 2.2 既存 API の確認

- [ ] `internal/libccache/cache.go` の `writeFileAtomic`、`LibcCacheFile`、`WrapperEntry` を再利用できること
- [ ] `internal/machodylib` にタスク 0096 のライブラリ解決ロジック再利用点があること
- [ ] `internal/runner/security/machoanalyzer` のシンボル正規化処理を外部再利用できること
- [ ] `internal/fileanalysis` の `SyscallAnalysisData` が Mach-O 用の `DetectedSyscalls` マージを表現できること

### 2.3 外部依存の確認

- [ ] `github.com/blacktop/ipsw/pkg/dyld` を Darwin ビルドタグ配下に閉じ込められること
- [ ] `pkg/dyld` の API 差異を `dyld_extractor_darwin.go` 内で吸収できること
- [ ] Linux 環境で `go test ./...` が non-Darwin スタブのみで通ること

## 3. Step 1: `internal/libccache` の拡張

**対象ファイル**:
- `internal/libccache/schema.go`
- `internal/libccache/macos_syscall_table.go`
- `internal/libccache/macho_analyzer.go`
- `internal/libccache/macho_cache.go`
- `internal/libccache/matcher.go`
- `internal/libccache/adapters.go`

### 3.1 実装チェックリスト

- [ ] `SourceLibsystemSymbolImport` を追加する
- [ ] `DeterminationMethodLibCacheMatch` と `DeterminationMethodSymbolNameMatch` を追加する
- [ ] macOS arm64 BSD syscall テーブルを定義する
- [ ] フォールバック専用のネットワーク syscall 名リストを定義する
- [ ] `MachoLibSystemAnalyzer.Analyze()` を実装する
- [ ] Mach-O `__TEXT,__text` 範囲から `svc #0x80` を検出する
- [ ] `x16` 後方スキャンで BSD クラスプレフィックスを除去した syscall 番号を抽出する
- [ ] 256 バイト超の関数を除外する
- [ ] 複数の異なる syscall 番号を持つ関数を除外する
- [ ] `MachoLibSystemCacheManager.GetOrCreate()` を実装する
- [ ] `ImportSymbolMatcher.MatchWithMethod()` を追加する
- [ ] `MachoLibSystemAdapter` で cache hit / fallback の両経路を実装する
- [ ] 非 arm64 ライブラリ時は info ログを出して解析をスキップする

### 3.2 テストチェックリスト

- [ ] `internal/libccache/macos_syscall_table_test.go`
  - [ ] ネットワーク syscall 定義の存在確認
  - [ ] `socket=97`, `connect=98` の番号確認
- [ ] `internal/libccache/macho_analyzer_test.go`
  - [ ] `svc #0x80` を含む関数の正常検出
  - [ ] 256 バイト超関数の除外
  - [ ] 複数番号関数の除外
  - [ ] 同一番号複数 `svc` の許容
  - [ ] BSD クラスプレフィックス除去
  - [ ] 非 arm64 のスキップ確認
- [ ] `internal/libccache/macho_cache_test.go`
  - [ ] キャッシュ生成
  - [ ] キャッシュヒット
  - [ ] schema mismatch 再生成
  - [ ] hash mismatch 再生成
  - [ ] 破損キャッシュ再生成
  - [ ] キャッシュ書き込み失敗時のエラー
- [ ] `internal/libccache/adapters_test.go`
  - [ ] import symbol 照合
  - [ ] 同一 Number dedup
  - [ ] フォールバック時 `symbol_name_match`
  - [ ] フォールバック理由ログ
  - [ ] 非 arm64 スキップ

## 4. Step 2: `internal/machodylib` の拡張

**対象ファイル**:
- `internal/machodylib/libsystem_resolver.go`
- `internal/machodylib/dyld_extractor.go`
- `internal/machodylib/dyld_extractor_darwin.go`

### 4.1 実装チェックリスト

- [ ] `DynLibDeps` から `libSystem.B.dylib` と `libsystem_kernel.dylib` を区別して抽出する
- [ ] `libsystem_kernel.dylib` が直接ある場合はその `Path` を優先使用する
- [ ] `libSystem.B.dylib` のみ存在する場合は `LC_REEXPORT_DYLIB` を走査する
- [ ] re-export 解決にタスク 0096 のパス解決ロジックを再利用する
- [ ] ウェルノウンパス `/usr/lib/system/libsystem_kernel.dylib` を試行する
- [ ] dyld shared cache 抽出を Darwin 実装に閉じ込める
- [ ] 抽出バイト列の SHA-256 を `lib_hash` に使用する
- [ ] dyld shared cache 不在・未収録・抽出失敗時は `nil, nil` でフォールバックに委譲する

### 4.2 テストチェックリスト

- [ ] `internal/machodylib/libsystem_resolver_test.go`
  - [ ] direct `libsystem_kernel.dylib` 優先
  - [ ] umbrella から re-export 解決
  - [ ] ウェルノウンパス使用
  - [ ] libSystem 依存なしで `nil, nil`
- [ ] `internal/machodylib/dyld_extractor_test.go`
  - [ ] `arm64e` キャッシュ優先
  - [ ] `arm64` フォールバック
  - [ ] 抽出バイト列ハッシュ確認
  - [ ] 対象イメージ未検出時の `nil, nil`

## 5. Step 3: `internal/filevalidator` の拡張

**対象ファイル**:
- `internal/filevalidator/validator.go`

### 5.1 実装チェックリスト

- [ ] `LibSystemCacheInterface` を追加する
- [ ] `Validator` に `libSystemCache` フィールドを追加する
- [ ] `SetLibSystemCache()` を追加する
- [ ] Mach-O import symbol 取得ヘルパーを追加する
- [ ] import symbol 正規化を `machoanalyzer.NormalizeSymbolName()` で行う
- [ ] `analyzeLibSystemImports()` を追加する
- [ ] `buildSVCSyscallEntries()` に既存 Mach-O svc 保存ロジックを分離する
- [ ] `buildMachoSyscallAnalysisData()` と `mergeMachoSyscallInfos()` を追加する
- [ ] `updateAnalysisRecord()` で svc 結果と libSystem 結果をマージする
- [ ] libSystem キャッシュ書き込み成功後のみ record が保存されることを維持する

### 5.2 テストチェックリスト

- [ ] `internal/filevalidator/validator_macho_test.go`
  - [ ] svc のみ保存
  - [ ] libSystem import のみ保存
  - [ ] svc + libSystem 共存保存
  - [ ] `Location=0` 設定
  - [ ] `Source=libsystem_symbol_import` 設定
  - [ ] `record` 保存前に cache write failure で中断
  - [ ] ライブラリ unreadable 時のエラー
  - [ ] export symbol 取得失敗時のエラー

## 6. Step 4: `internal/runner/security` の拡張

**対象ファイル**:
- `internal/runner/security/network_analyzer.go`
- `internal/runner/security/machoanalyzer/symbol_normalizer.go`

### 6.1 実装チェックリスト

- [ ] `NormalizeSymbolName()` をエクスポートする
- [ ] `syscallAnalysisHasNetworkSignal()` を追加する
- [ ] `isNetworkViaBinaryAnalysis()` に `IsNetwork` 判定を追加する
- [ ] 優先順位を `direct_svc_0x80` → `IsNetwork` → `SymbolAnalysis` にする
- [ ] `SymbolAnalysis` の結果にかかわらず `LoadSyscallAnalysis()` を行う現行方針を維持する

### 6.2 テストチェックリスト

- [ ] `internal/runner/security/network_analyzer_test.go`
  - [ ] libSystem network entry のみで `true, false`
  - [ ] direct svc + libSystem network で `true, true`
  - [ ] 非ネットワーク libSystem entry のみで `false, false`
  - [ ] hash mismatch / schema mismatch の高リスク化

## 7. Step 5: 注入と統合確認

**対象ファイル**:
- `cmd/record/main.go`

### 7.1 実装チェックリスト

- [ ] `MachoLibSystemCacheManager` を初期化する
- [ ] `MachoLibSystemAdapter` を初期化する
- [ ] `Validator.SetLibSystemCache()` で注入する
- [ ] 既存 ELF libc cache 初期化フローに影響しないことを確認する

### 7.2 統合テストチェックリスト

- [ ] macOS arm64 で dyld shared cache 経由の E2E テスト
- [ ] dyld 抽出失敗時の最終フォールバック E2E テスト
- [ ] ELF フロー非回帰テスト

### 7.3 最終確認チェックリスト

- [ ] `make fmt`
- [ ] `make test`
- [ ] `make lint`
- [ ] `go build ./...`

## 8. 完了条件

- [ ] `01_requirements.md` の AC-1 から AC-7 をすべてテストでカバーしている
- [ ] `02_architecture.md` のコンポーネント境界どおりに依存が収まっている
- [ ] `03_detailed_specification.md` に未確定 API 名や実装保留メモが残っていない
- [ ] Linux と macOS arm64 の両方でビルドが成立する
