# ELF 動的シンボル解析によるネットワーク操作検出 実装計画書

## 1. 概要

### 1.1 目的

本実装計画書は、ELF バイナリの `.dynsym` セクションを解析してネットワーク操作を検出する機能の実装手順を定義する。

### 1.2 実装範囲

- Phase 0: 前提条件の確認とテスト補完
- Phase 1: elfanalyzer パッケージの実装
- Phase 2: security パッケージへの統合
- Phase 3: テストと検証

### 1.3 想定工数

- Phase 0: 1-2 時間
- Phase 1: 6-8 時間
- Phase 2: 4-6 時間
- Phase 3: 4-6 時間
- **合計: 15-22 時間**

## 2. Phase 0: 前提条件の実装確認とテスト補完

### 2.1 目的

ELF 解析に必要なインターフェース等の実装状況を確認し、不足している mockFile のテスト（Seek, ReadAt）を追加する。

### 2.2 作業項目

#### 2.2.1 実装状況の確認（完了済み）

以下の項目はコードベースですでに実装されていることを確認する。変更は不要。

1. **safefileio.File インターフェースの拡張**: `io.Seeker` と `io.ReaderAt` が追加済み。
2. **mockFile の拡張**: `Seek` と `ReadAt` メソッド実装済み（スレッドセーフティ含む）。
3. **OpenFileWithPrivileges の変更**: `safefileio.File` を返すように変更済み。
4. **VerifyFromHandle の変更**: `io.ReadSeeker` を受け取るように変更済み。

**チェックリスト**:
- [ ] `internal/safefileio/safe_file.go` の確認
- [ ] `internal/safefileio/safe_file_cleanup_test.go` の確認
- [ ] `internal/filevalidator/privileged_file.go` の確認
- [ ] `internal/filevalidator/validator.go` の確認

#### 2.2.2 mockFile テストの追加

`internal/safefileio/safe_file_cleanup_test.go` には `Truncate` のテストなどはあるが、`Seek` と `ReadAt` のユニットテストが不足している。これらを追加する。

**ファイル**: `internal/safefileio/safe_file_cleanup_test.go`

**追加するテスト**:
- `TestMockFileSeek`: SeekStart, SeekCurrent, SeekEnd, エラーケース（負の位置、無効な whence）
- `TestMockFileReadAt`: 正常系、範囲外アクセス、負のオフセット

**チェックリスト**:
- [ ] `TestMockFileSeek` の実装
- [ ] `TestMockFileReadAt` の実装
- [ ] 全ての safefileio テストのパス確認

### 2.3 完了条件

- [ ] 実装済みのコードが仕様通りであることを確認
- [ ] `mockFile` の `Seek` と `ReadAt` のテストが追加され、パスしている
- [ ] 全ての既存テストがパス

### 2.4 リスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| 既存実装にバグがある可能性 | 中 | 新規追加するテストでバグを検出・修正する |

## 3. Phase 1: elfanalyzer パッケージの実装

### 3.1 目的

ELF バイナリの `.dynsym` セクションを解析し、ネットワーク関連シンボルを検出する独立したパッケージを実装する。

### 3.2 ディレクトリ構造の作成

```bash
mkdir -p internal/runner/security/elfanalyzer/testdata
```

### 3.3 作業項目

#### 3.3.1 パッケージドキュメント

**ファイル**: `internal/runner/security/elfanalyzer/doc.go`

```go
// Package elfanalyzer provides ELF binary analysis for detecting network operation capability.
//
// This package analyzes the dynamic symbol table (.dynsym) of ELF binaries to identify
// imported network-related functions from shared libraries. It is designed to work with
// dynamically linked binaries on Linux systems.
//
// # Usage
//
//     analyzer := elfanalyzer.NewStandardELFAnalyzer(nil, nil)
//     output := analyzer.AnalyzeNetworkSymbols("/usr/bin/curl")
//
//     if output.IsNetworkCapable() {
//         fmt.Printf("Network symbols detected: %v\n", output.DetectedSymbols)
//     }
//
// # Limitations
//
// - Static binaries (e.g., Go binaries) return StaticBinary result
// - Requires read access to the binary (execute-only binaries need privilege escalation)
// - Only analyzes .dynsym section (does not detect syscalls or runtime network operations)
//
// # Security Considerations
//
// This analyzer uses safefileio to prevent symlink attacks and TOCTOU race conditions.
// When analyzing execute-only binaries, provide a PrivilegeManager to enable
// temporary privilege escalation during file access.
package elfanalyzer
```

**チェックリスト**:
- [ ] パッケージドキュメントの作成

#### 3.3.2 型定義とインターフェース

**ファイル**: `internal/runner/security/elfanalyzer/analyzer.go`

**実装内容**: 仕様書 3.1 節の内容を実装

**チェックリスト**:
- [ ] `AnalysisResult` enum の定義
- [ ] `AnalysisResult.String()` の実装
- [ ] `DetectedSymbol` 構造体の定義
- [ ] `AnalysisOutput` 構造体の定義
- [ ] `AnalysisOutput.IsNetworkCapable()` の実装
- [ ] `AnalysisOutput.IsIndeterminate()` の実装
- [ ] `ELFAnalyzer` インターフェースの定義
- [ ] doc コメントの完備

#### 3.3.3 ネットワークシンボルレジストリ

**ファイル**: `internal/runner/security/elfanalyzer/network_symbols.go`

**実装内容**: 仕様書 3.2 節の内容を実装

**シンボルカテゴリ**:
- Socket API (socket, connect, bind, listen, accept, send, recv, etc.)
- DNS (getaddrinfo, getnameinfo, gethostbyname, etc.)
- HTTP (curl_easy_init, curl_easy_perform, etc.)
- TLS (SSL_connect, SSL_new, gnutls_init, etc.)

**チェックリスト**:
- [ ] `SymbolCategory` 型の定義
- [ ] `networkSymbolRegistry` マップの定義
- [ ] 各カテゴリのシンボル登録（30個以上）
- [ ] `GetNetworkSymbols()` の実装
- [ ] `IsNetworkSymbol()` の実装
- [ ] `SymbolCount()` の実装
- [ ] doc コメントの完備

#### 3.3.4 ELF アナライザの実装

**ファイル**: `internal/runner/security/elfanalyzer/analyzer_impl.go`

**実装内容**: 仕様書 3.3 節の内容を実装

**実装の流れ**:
1. `SafeOpenFile` でファイルを安全に開く
2. 権限エラーの場合、`OpenFileWithPrivileges` で再試行
3. ファイルの妥当性チェック（regular file、サイズ制限）
4. ELF マジックナンバーの確認
5. `debug/elf.NewFile` で ELF パース
6. `.dynsym` セクションの読み取り
7. ネットワークシンボルのマッチング
8. 結果の返却

**チェックリスト**:
- [ ] `StandardELFAnalyzer` 構造体の定義
- [ ] `NewStandardELFAnalyzer()` の実装
- [ ] `NewStandardELFAnalyzerWithSymbols()` の実装（テスト用）
- [ ] `AnalyzeNetworkSymbols()` の実装
- [ ] `isELFMagic()` ヘルパー関数の実装
- [ ] `isNoDynsymError()` ヘルパー関数の実装
- [ ] `containsAny()` ヘルパー関数の実装
- [ ] エラーハンドリングの完備
- [ ] ファイルサイズ制限（1GB）の実装
- [ ] doc コメントの完備

#### 3.3.5 テストフィクスチャの生成

**ファイル**: `internal/runner/security/elfanalyzer/testdata/README.md`

**生成するバイナリ**:
1. `with_socket.elf` - socket API を使用
2. `with_curl.elf` - libcurl をリンク
3. `with_ssl.elf` - OpenSSL をリンク
4. `no_network.elf` - ネットワークシンボルなし
5. `static.elf` - 静的リンクバイナリ
6. `script.sh` - シェルスクリプト（非 ELF）
7. `corrupted.elf` - 破損した ELF

**チェックリスト**:
- [ ] `README.md` に生成手順を記載
- [ ] 各テストバイナリの生成スクリプト作成
- [ ] バイナリの生成と配置
- [ ] `.gitignore` への追加（バイナリは Git 管理外）

#### 3.3.6 ユニットテストの実装

**ファイル**: `internal/runner/security/elfanalyzer/analyzer_test.go`

**実装内容**: 仕様書 6.2 節の内容を実装

**テストケース**:
- `TestStandardELFAnalyzer_AnalyzeNetworkSymbols` - 各種バイナリのテスト
- `TestStandardELFAnalyzer_NonexistentFile` - 存在しないファイル
- `TestAnalysisOutput_IsNetworkCapable` - 結果判定のテスト
- `TestAnalysisOutput_IsIndeterminate` - 不確定判定のテスト
- `TestIsNetworkSymbol` - シンボル検索のテスト
- `TestSymbolCount` - レジストリサイズのテスト

**チェックリスト**:
- [ ] 全テストケースの実装
- [ ] testify/assert, testify/require の使用
- [ ] テストフィクスチャがない場合の Skip 処理
- [ ] テーブル駆動テストの活用
- [ ] エラーケースのカバレッジ
- [ ] 全テストのパス確認

### 3.4 完了条件

- [ ] `elfanalyzer` パッケージの全ファイルを実装
- [ ] ユニットテストのカバレッジ 80% 以上
- [ ] `go test ./internal/runner/security/elfanalyzer/...` が成功
- [ ] `golangci-lint` のエラーなし
- [ ] doc コメントの完備

### 3.5 リスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| debug/elf のバージョン互換性問題 | 中 | Go 1.19+ を前提とし、CI で複数バージョンをテスト |
| テストフィクスチャの生成失敗 | 中 | README.md に詳細な手順を記載、CI でバイナリを自動生成 |
| 特殊な ELF フォーマットへの対応不足 | 低 | 実環境でのテストケースを収集し、段階的に対応 |

## 4. Phase 2: security パッケージへの統合

### 4.1 目的

`elfanalyzer` パッケージを `IsNetworkOperation` に統合し、未知のコマンドに対する ELF 解析を有効化する。

### 4.2 作業項目

#### 4.2.1 ELF アナライザの初期化とインジェクション

**ファイル**: `internal/runner/security/command_analysis.go`

**追加するコード**:

```go
import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// elfAnalyzerOnce ensures the default ELF analyzer is initialized exactly once.
var elfAnalyzerOnce sync.Once

// defaultELFAnalyzer is the package-level ELF analyzer instance.
var defaultELFAnalyzer elfanalyzer.ELFAnalyzer

// getELFAnalyzer returns the default ELF analyzer, creating it if necessary.
// Concurrency-safe via sync.Once.
func getELFAnalyzer() elfanalyzer.ELFAnalyzer {
    elfAnalyzerOnce.Do(func() {
        defaultELFAnalyzer = elfanalyzer.NewStandardELFAnalyzer(nil, nil)
    })
    return defaultELFAnalyzer
}

// SetELFAnalyzer sets a custom ELF analyzer for testing purposes.
// Must be called before any concurrent calls to IsNetworkOperation.
func SetELFAnalyzer(analyzer elfanalyzer.ELFAnalyzer) {
    defaultELFAnalyzer = analyzer
    elfAnalyzerOnce.Do(func() {})
}
```

**チェックリスト**:
- [ ] `getELFAnalyzer()` の実装
- [ ] `SetELFAnalyzer()` の実装（テスト用）
- [ ] `sync.Once` によるスレッドセーフな初期化
- [ ] doc コメントの追加

#### 4.2.2 IsNetworkOperation への統合

**ファイル**: `internal/runner/security/command_analysis.go`

**変更箇所**: `IsNetworkOperation()` 関数

**統合ロジック**:
1. 既存のプロファイル検索を優先
2. プロファイルに見つからない場合、ELF 解析を実行
3. 引数ベースの検出は最後のフォールバック

**実装内容**: 仕様書 4.1 節の内容を実装

**チェックリスト**:
- [ ] `analyzeELFForNetwork()` ヘルパー関数の実装
- [ ] `exec.LookPath` でコマンドパスを解決
- [ ] `filepath.Abs` で絶対パスに変換
- [ ] `getELFAnalyzer()` でアナライザを取得
- [ ] `AnalyzeNetworkSymbols()` を呼び出し
- [ ] 結果に応じた分岐処理
- [ ] 適切なログ出力（slog.Debug, slog.Warn）
- [ ] `formatDetectedSymbols()` ヘルパー関数の実装

#### 4.2.3 統合テストの実装

**ファイル**: `internal/runner/security/command_analysis_test.go`

**実装内容**: 仕様書 6.3 節の内容を実装

**テストケース**:
- プロファイルコマンドは ELF 解析をバイパス
- 未知のコマンドでネットワークシンボル検出
- 未知のコマンドでシンボルなし
- 解析エラー時の Middle Risk 扱い
- 静的バイナリのフォールバック動作

**モックアナライザの実装**:
```go
type mockELFAnalyzer struct {
    result elfanalyzer.AnalysisResult
}

func (m *mockELFAnalyzer) AnalyzeNetworkSymbols(path string) elfanalyzer.AnalysisOutput {
    // ...
}
```

**チェックリスト**:
- [ ] `TestIsNetworkOperation_ELFAnalysis` の実装
- [ ] `mockELFAnalyzer` の実装
- [ ] 各テストケースの実装
- [ ] `SetELFAnalyzer()` を使用したモック注入
- [ ] テスト後のクリーンアップ
- [ ] 全テストのパス確認

### 4.3 完了条件

- [ ] `IsNetworkOperation` が ELF 解析を統合
- [ ] 統合テストが全てパス
- [ ] 既存テストへの影響なし
- [ ] ログメッセージが適切に出力される

### 4.4 リスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| パフォーマンス劣化 | 中 | LookPath の結果をキャッシュ、頻繁に呼ばれる場合は最適化 |
| 既存機能の破壊 | 高 | プロファイル検索を優先、段階的に統合、既存テストを先に実行 |
| ログの過剰出力 | 低 | Debug レベルを使用、必要に応じて調整 |

## 5. Phase 3: テストと検証

### 5.1 目的

全体のテストと実環境での検証を行い、品質を保証する。

### 5.2 作業項目

#### 5.2.1 ユニットテストの完全実行

**チェックリスト**:
- [ ] `go test ./internal/runner/security/elfanalyzer/...` の成功
- [ ] `go test ./internal/runner/security/...` の成功
- [ ] `go test ./internal/filevalidator/...` の成功
- [ ] `go test ./internal/safefileio/...` の成功
- [ ] カバレッジレポートの確認（80% 以上）

#### 5.2.2 統合テストの実行

**チェックリスト**:
- [ ] `make test` の成功
- [ ] 全ての既存テストがパス
- [ ] リグレッションなし

#### 5.2.3 実環境テスト

**テストコマンド例**:

```bash
# Known network commands (should detect via profile)
./runner sample/test_elf_analysis.toml curl https://example.com
./runner sample/test_elf_analysis.toml wget https://example.com
./runner sample/test_elf_analysis.toml ssh user@host

# Unknown commands with network symbols (should detect via ELF)
./runner sample/test_elf_analysis.toml /usr/bin/python3 -c "import socket; socket.socket()"
./runner sample/test_elf_analysis.toml /usr/bin/nc localhost 8080

# Commands without network symbols (should not detect)
./runner sample/test_elf_analysis.toml /bin/ls
./runner sample/test_elf_analysis.toml /bin/cat /etc/hosts

# Static binary (Go command - should fallback to arg detection)
./runner sample/test_elf_analysis.toml go get example.com/pkg
```

**チェックリスト**:
- [ ] 既知のネットワークコマンドが正しく検出される
- [ ] 未知のネットワークコマンドが ELF 解析で検出される
- [ ] 非ネットワークコマンドが誤検出されない
- [ ] 静的バイナリがフォールバック動作する
- [ ] ログメッセージが適切に出力される
- [ ] パフォーマンスが許容範囲内

#### 5.2.4 エッジケーステスト

**テストケース**:

1. **実行専用バイナリ**:
   ```bash
   sudo chmod 111 /tmp/test_exec_only
   ./runner sample/test_elf_analysis.toml /tmp/test_exec_only
   # Expected: AnalysisError, treated as Middle Risk
   ```

2. **シンボリックリンク**:
   ```bash
   ln -s /usr/bin/curl /tmp/mycurl
   ./runner sample/test_elf_analysis.toml /tmp/mycurl https://example.com
   # Expected: Resolved to real binary, network detected
   ```

3. **大きなバイナリ**:
   ```bash
   # Create 2GB file (should be rejected)
   dd if=/dev/zero of=/tmp/huge bs=1M count=2048
   ./runner sample/test_elf_analysis.toml /tmp/huge
   # Expected: AnalysisError (file too large)
   ```

4. **破損した ELF**:
   ```bash
   printf '\x7fELF' > /tmp/corrupted
   dd if=/dev/urandom bs=100 count=1 >> /tmp/corrupted
   ./runner sample/test_elf_analysis.toml /tmp/corrupted
   # Expected: AnalysisError, treated as Middle Risk
   ```

**チェックリスト**:
- [ ] 実行専用バイナリの動作確認
- [ ] シンボリックリンクの動作確認
- [ ] 大きなバイナリの拒否確認
- [ ] 破損した ELF の動作確認

#### 5.2.5 パフォーマンステスト

**測定項目**:
- ELF 解析の平均時間（< 10ms 目標）
- メモリ使用量の増加
- キャッシュなしでの連続実行

**チェックリスト**:
- [ ] ベンチマークテストの実装
- [ ] パフォーマンス測定結果の記録
- [ ] ボトルネックの特定と対策

#### 5.2.6 ドキュメントの更新

**チェックリスト**:
- [ ] `CHANGELOG.md` の更新
- [ ] `README.md` の更新（必要に応じて）
- [ ] タスクドキュメントの完成度確認
- [ ] 実装ノートの記録（問題と解決策）

### 5.3 完了条件

- [ ] 全ユニットテストがパス
- [ ] 全統合テストがパス
- [ ] 実環境テストが成功
- [ ] エッジケーステストが成功
- [ ] パフォーマンスが許容範囲内
- [ ] ドキュメントが最新

### 5.4 受け入れ基準

仕様書の受け入れ条件（AC-1 〜 AC-7）の全てを満たすこと：

- [ ] AC-1: ELF バイナリを正しく判定
- [ ] AC-2: ネットワークシンボルを検出
- [ ] AC-3: HTTP/TLS ライブラリを検出
- [ ] AC-4: 既存機能にフォールバック
- [ ] AC-5: 静的バイナリを識別
- [ ] AC-6: 解析失敗時に安全に動作
- [ ] AC-7: 既存プロファイル優先

## 6. リスク管理

### 6.1 リスクサマリ

| フェーズ | リスク | 対策 | 担当 | ステータス |
|---------|--------|------|------|-----------|
| Phase 0 | mockFile 実装不備 | 段階的実装とテスト | 実装者 | - |
| Phase 1 | テストフィクスチャ生成失敗 | 詳細な手順書作成 | 実装者 | - |
| Phase 2 | 既存機能の破壊 | プロファイル優先、段階的統合 | 実装者 | - |
| Phase 3 | パフォーマンス劣化 | キャッシュ、ベンチマーク | 実装者 | - |

### 6.2 ロールバック計画

各フェーズで問題が発生した場合：

1. **Phase 0**: `safefileio.File` インターフェースを元に戻す
2. **Phase 1**: `elfanalyzer` パッケージを削除
3. **Phase 2**: `IsNetworkOperation` の変更を revert
4. **Phase 3**: フィーチャーフラグで無効化

## 7. スケジュール

### 7.1 タイムライン

```
Week 1:
  Day 1-2: Phase 0 (safefileio/filevalidator 拡張)
  Day 3:   Phase 0 テストと検証

Week 2:
  Day 1-2: Phase 1 (elfanalyzer パッケージ実装)
  Day 3:   Phase 1 テストフィクスチャ生成
  Day 4:   Phase 1 ユニットテスト実装

Week 3:
  Day 1-2: Phase 2 (security パッケージ統合)
  Day 3:   Phase 2 統合テスト
  Day 4:   Phase 3 実環境テストとエッジケース
  Day 5:   Phase 3 ドキュメント更新と最終確認
```

### 7.2 マイルストーン

| マイルストーン | 予定日 | 完了条件 |
|---------------|--------|---------|
| M1: Phase 0 完了 | Day 3 | 全既存テストがパス、新インターフェースが動作 |
| M2: Phase 1 完了 | Day 8 | elfanalyzer パッケージが独立して動作 |
| M3: Phase 2 完了 | Day 12 | IsNetworkOperation が ELF 解析を統合 |
| M4: Phase 3 完了 | Day 15 | 全テストがパス、ドキュメント完成 |

## 8. 品質基準

### 8.1 コード品質

- [ ] `golangci-lint` エラーなし
- [ ] `go fmt` 適用済み
- [ ] doc コメント完備
- [ ] ユニットテストカバレッジ 80% 以上
- [ ] 統合テストカバレッジ 70% 以上

### 8.2 パフォーマンス

- [ ] ELF 解析 < 10ms（平均）
- [ ] メモリ増加 < 10MB
- [ ] 既存機能のパフォーマンス劣化なし

### 8.3 セキュリティ

- [ ] TOCTOU 攻撃対策（safefileio 使用）
- [ ] シンボリックリンク攻撃対策
- [ ] リソース枯渇対策（ファイルサイズ制限）
- [ ] 権限昇格の適切な使用

## 9. 付録

### 9.1 依存関係

- Go 1.19+
- `debug/elf` 標準パッケージ
- `internal/safefileio` パッケージ
- `internal/filevalidator` パッケージ
- `internal/runner/runnertypes` パッケージ

### 9.2 参考資料

- [ELF Specification](https://refspecs.linuxfoundation.org/elf/elf.pdf)
- [Go debug/elf documentation](https://pkg.go.dev/debug/elf)
- [Task 0069 Specification](03_specification.md)
- [Task 0069 Architecture §7.2 (Execute-Only Binaries)](02_architecture.md)

### 9.3 変更履歴

| 日付 | バージョン | 変更内容 | 作成者 |
|------|-----------|---------|--------|
| 2026-02-04 | 1.0 | 初版作成 | - |
