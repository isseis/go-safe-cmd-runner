# アーキテクチャー設計書: ファイル改竄検出機能の実装

## 1. 概要

本設計書では、go-safe-cmd-runnerにおけるファイル改竄検出機能の実装アーキテクチャについて説明する。

### 1.1 設計原則

- **セキュリティファースト**: 改竄検出は起動時の最初期段階で実行
- **段階的実装**: Phase 1（警告）、Phase 2（設定ファイル検証）、Phase 3（実行ファイル検証）
- **既存コンポーネント活用**: filevalidator、security パッケージの再利用
- **最小権限原則**: ハッシュファイルは root のみ書き込み可能

## 2. システム全体アーキテクチャ

### 2.1 コンポーネント図

```
┌─────────────────────────────────────────┐
│              go-safe-cmd-runner         │
├─────────────────────────────────────────┤
│  main.go                                │
│  ├─ 1. Config File Hash Verification   │
│  ├─ 2. Config Loading                   │
│  └─ 3. Command Execution               │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│        Config Hash Verification        │
├─────────────────────────────────────────┤
│  VerificationManager                    │
│  ├─ LoadHashManifest()                  │
│  ├─ VerifyConfigFile()                  │
│  └─ ValidateHashFilePermissions()       │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│           既存コンポーネント              │
├─────────────────────────────────────────┤
│  filevalidator.Validator                │
│  ├─ Verify(filePath)                    │
│  └─ Record(filePath)                    │
│                                         │
│  security.Validator                     │
│  └─ ValidateFilePermissions(filePath)   │
└─────────────────────────────────────────┘
```

### 2.2 データフロー

```
[起動] → [ハッシュマニフェスト読み込み] → [設定ファイル検証] → [設定ファイル読み込み] → [コマンド実行]
   │              │                      │                   │
   │              │                      │                   └─ 成功時: 通常動作
   │              │                      │
   │              │                      └─ 失敗時: エラー終了
   │              │
   │              └─ 失敗時: エラー終了
   │
   └─ Phase 1: 警告表示のみ
```

## 3. 詳細設計

### 3.1 新規コンポーネント設計

#### 3.1.1 VerificationManager

```go
// Package verification provides config file integrity verification
package verification

type VerificationManager struct {
    hashDir      string                    // ハッシュファイル格納ディレクトリ
    validator    *filevalidator.Validator  // ファイル検証用
    security     *security.Validator       // セキュリティ検証用
    fs           common.FileSystem         // ファイルシステム抽象化
}

type Config struct {
    HashDirectory string // ハッシュファイル格納ディレクトリ
    Enabled       bool   // 検証機能の有効/無効
}

// メソッド
func NewVerificationManager(config *Config, fs common.FileSystem) (*VerificationManager, error)
func (vm *VerificationManager) VerifyConfigFile(configPath string) error
func (vm *VerificationManager) ValidateHashDirectory() error
```

#### 3.1.2 Phase管理

```go
// Phase定義
type Phase int

const (
    PhaseWarningOnly Phase = iota  // Phase 1: 警告のみ
    PhaseConfigVerify              // Phase 2: 設定ファイル検証
    PhaseFullVerify                // Phase 3: 全ファイル検証（将来）
)

// 設定による Phase 制御
type VerificationConfig struct {
    Phase         Phase  `toml:"phase"`
    HashDirectory string `toml:"hash_directory"`
    Enabled       bool   `toml:"enabled"`
}
```

### 3.2 既存コンポーネントとの統合

#### 3.2.1 main.go の変更点

```go
func main() {
    // 1. 検証マネージャーの初期化
    verificationManager, err := verification.NewVerificationManager(...)
    if err != nil {
        log.Fatal("Failed to initialize verification manager:", err)
    }

    // 2. 設定ファイル検証（Phase 2以降）
    if err := verificationManager.VerifyConfigFile(configPath); err != nil {
        log.Fatal("Config file verification failed:", err)
    }

    // 3. 既存の設定読み込み処理
    loader := config.NewLoader()
    cfg, err := loader.LoadConfig(configPath)
    // ...
}
```

#### 3.2.2 config/loader.go の変更点

```go
// Phase 1: 警告メッセージの追加
func (l *Loader) LoadConfig(path string) (*runnertypes.Config, error) {
    // 警告ログの出力
    slog.Warn("Configuration file integrity verification is not implemented yet",
        "phase", "1",
        "security_risk", "Configuration files may be tampered without detection")

    // 既存の処理...
}
```

### 3.3 ディレクトリ構造

```
internal/
├── verification/           # 新規パッケージ
│   ├── manager.go         # VerificationManager実装
│   ├── manager_test.go    # テスト
│   ├── config.go          # 設定構造体
│   └── errors.go          # エラー定義
├── runner/
│   └── config/
│       └── loader.go      # 警告メッセージ追加
└── filevalidator/         # 既存（活用）
    └── validator.go
```

### 3.4 設定ファイル拡張

```toml
# sample/config.toml
[verification]
enabled = true
phase = 2  # 1: 警告のみ, 2: 設定ファイル検証, 3: 全ファイル検証
hash_directory = "/etc/go-safe-cmd-runner/hashes"

[global]
# 既存の設定...
```

## 4. セキュリティ設計

### 4.1 ハッシュファイル保護

```
/etc/go-safe-cmd-runner/hashes/
├── manifest.json         # ハッシュマニフェストファイル
│   └── 権限: 644 (root:root)
└── config_hashes/        # 設定ファイルハッシュ格納
    └── 権限: 755 (root:root)
```

### 4.2 検証順序

1. **ハッシュディレクトリ権限検証**
   - ディレクトリ所有者が root であることを確認
   - 書き込み権限が root のみであることを確認

2. **ハッシュマニフェスト検証**
   - マニフェストファイルの存在確認
   - マニフェストファイルの権限確認

3. **設定ファイル検証**
   - 設定ファイルのハッシュ値計算
   - マニフェストとの比較

### 4.3 エラーハンドリング

```go
// verification/errors.go
var (
    ErrHashDirectoryNotFound = errors.New("hash directory not found")
    ErrHashDirectoryPermission = errors.New("hash directory has invalid permissions")
    ErrManifestNotFound = errors.New("hash manifest not found")
    ErrManifestPermission = errors.New("hash manifest has invalid permissions")
    ErrConfigHashMismatch = errors.New("config file hash mismatch")
    ErrVerificationDisabled = errors.New("verification is disabled")
)
```

## 5. パフォーマンス設計

### 5.1 起動時間への影響

- **ハッシュ計算**: SHA-256計算は高速（設定ファイルサイズは通常小さい）
- **ファイルI/O**: 最小限（マニフェスト読み込み + 設定ファイル読み込み）
- **予想起動時間増加**: < 50ms

### 5.2 メモリ使用量

- **ハッシュマニフェスト**: < 1KB（通常）
- **一時的なハッシュ計算**: < 32bytes per file
- **総メモリ増加**: < 10KB

## 6. テスト設計

### 6.1 テスト戦略

```
verification/
├── manager_test.go
│   ├─ TestVerificationManager_Success
│   ├─ TestVerificationManager_HashMismatch
│   ├─ TestVerificationManager_MissingManifest
│   ├─ TestVerificationManager_InvalidPermissions
│   └─ TestVerificationManager_DisabledMode
├── integration_test.go
│   ├─ TestEndToEnd_VerificationSuccess
│   ├─ TestEndToEnd_VerificationFailure
│   └─ TestEndToEnd_WarningMode
└── testdata/
    ├─ valid_manifest.json
    ├─ invalid_manifest.json
    └─ test_configs/
```

### 6.2 モックファイルシステム

```go
// テストでのMockFileSystem使用
func TestVerificationManager_WithMockFS(t *testing.T) {
    mockFS := common.NewMockFileSystem()
    mockFS.AddFile("/etc/go-safe-cmd-runner/hashes/manifest.json", 0644, validManifestData)

    vm := NewVerificationManagerWithFS(config, mockFS)
    err := vm.VerifyConfigFile("/path/to/config.toml")
    assert.NoError(t, err)
}
```

## 7. 段階的実装戦略

### Phase 1: 警告モード（優先度: 高）
- config/loader.go に警告ログ追加
- README.md にセキュリティ制限事項記載
- --verify-config オプションのヘルプ追加

### Phase 2: 設定ファイル検証（優先度: 高）
- verification パッケージ実装
- main.go への統合
- テストケース作成

### Phase 3: 実行ファイル検証（優先度: 低、将来実装）
- verification パッケージ拡張
- 実行ファイルハッシュ管理
- 動的検証機能

## 8. 運用考慮事項

### 8.1 デプロイメント

```bash
# ハッシュディレクトリの作成
sudo mkdir -p /etc/go-safe-cmd-runner/hashes
sudo chown root:root /etc/go-safe-cmd-runner/hashes
sudo chmod 755 /etc/go-safe-cmd-runner/hashes

# 設定ファイルハッシュの記録
sudo ./build/record /path/to/config.toml
```

### 8.2 監視とロギング

```go
// 構造化ログの例
slog.Info("Config verification completed",
    "config_path", configPath,
    "verification_time_ms", elapsedTime,
    "hash_algorithm", "SHA-256")

slog.Error("Config verification failed",
    "config_path", configPath,
    "error", err,
    "hash_expected", expectedHash,
    "hash_actual", actualHash)
```

## 9. 将来拡張

### 9.1 Phase 3 拡張ポイント

- 実行ファイルバイナリの検証
- 動的ライブラリの検証
- 設定で参照される外部ファイルの検証

### 9.2 設定拡張

```toml
[verification.advanced]  # Phase 3 で追加予定
verify_binaries = true
verify_libraries = true
verify_referenced_files = true
hash_cache_ttl = "1h"
```
