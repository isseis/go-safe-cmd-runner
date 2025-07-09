# アーキテクチャー設計書: ファイル改竄検出機能の実装

## 1. 概要

本設計書では、go-safe-cmd-runnerにおけるファイル改竄検出機能の実装アーキテクチャについて説明する。

### 1.1 設計原則

- **セキュリティファースト**: 改竄検出は起動時の最初期段階で実行
- **シンプルな制御**: 設定による機能の有効/無効制御
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
│  Manager                                │
│  ├─ VerifyConfigFile()                  │
│  └─ ValidateHashDirectory()             │
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
[起動] → [検証設定チェック] → [設定ファイル検証] → [設定ファイル読み込み] → [コマンド実行]
   │           │                     │                   │
   │           │                     │                   └─ 成功時: 通常動作
   │           │                     │
   │           │                     └─ 失敗時: エラー終了
   │           │
   │           └─ 無効時: 検証スキップ
```

## 3. 詳細設計

### 3.1 新規コンポーネント設計

#### 3.1.1 Manager

```go
// Package verification provides config file integrity verification
package verification

type Manager struct {
    config    *Config                    // 設定
    fs        common.FileSystem          // ファイルシステム抽象化
    validator *filevalidator.Validator   // ファイル検証用
    security  *security.Validator        // セキュリティ検証用
}

type Config struct {
    Enabled       bool   `toml:"enabled"`        // 検証機能の有効/無効
    HashDirectory string `toml:"hash_directory"` // ハッシュファイル格納ディレクトリ
}

// メソッド
func NewManager(config *Config) (*Manager, error)
func NewManagerWithFS(config *Config, fs common.FileSystem) (*Manager, error)
func (vm *Manager) VerifyConfigFile(configPath string) error
func (vm *Manager) ValidateHashDirectory() error
func (vm *Manager) IsEnabled() bool
```

### 3.2 既存コンポーネントとの統合

#### 3.2.1 main.go の変更点

```go
func main() {
    // 1. 設定ファイル読み込み
    cfgLoader := config.NewLoader()
    cfg, err := cfgLoader.LoadConfig(*configPath)
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // 2. 検証マネージャーの初期化
    verificationManager, err := verification.NewManager(&cfg.Verification)
    if err != nil {
        return fmt.Errorf("failed to initialize verification: %w", err)
    }

    // 3. 設定ファイル検証
    if err := verificationManager.VerifyConfigFile(*configPath); err != nil {
        return fmt.Errorf("config verification failed: %w", err)
    }

    // 4. 既存のRunner処理
    runner, err := runner.NewRunnerWithComponents(cfg, cfgLoader.GetTemplateEngine(), nil)
    // ...
}
```

### 3.3 ディレクトリ構造

```
internal/
├── verification/           # 新規パッケージ
│   ├── manager.go         # Manager実装
│   ├── manager_test.go    # テスト
│   ├── config.go          # 設定構造体
│   └── errors.go          # エラー定義
├── runner/
│   └── config/
│       └── loader.go      # 既存（変更なし）
└── filevalidator/         # 既存（活用）
    └── validator.go
```

### 3.4 設定ファイル拡張

```toml
# sample/config.toml
[verification]
enabled = false  # 検証機能の有効/無効
hash_directory = "/etc/go-safe-cmd-runner/hashes"

[global]
# 既存の設定...
```

## 4. セキュリティ設計

### 4.1 ハッシュファイル保護

```
/etc/go-safe-cmd-runner/hashes/
├── config.toml.hash      # 設定ファイルハッシュ
│   └── 権限: 644 (root:root)
└── metadata.json         # ハッシュメタデータ
    └── 権限: 644 (root:root)
```

### 4.2 検証順序

1. **設定による有効性確認**
   - 検証機能が有効であることを確認
   - 無効時は検証をスキップ

2. **ハッシュディレクトリ権限検証**
   - ディレクトリ所有者が root であることを確認
   - 書き込み権限が root のみであることを確認

3. **設定ファイル検証**
   - filevalidator.Validator を使用してハッシュ値検証
   - ハッシュ不一致時はエラー終了

### 4.3 エラーハンドリング

```go
// verification/errors.go
var (
    ErrVerificationDisabled = errors.New("verification is disabled")
    ErrHashDirectoryEmpty = errors.New("hash directory cannot be empty")
    ErrHashDirectoryInvalid = errors.New("hash directory is invalid")
    ErrConfigNil = errors.New("config cannot be nil")
    ErrSecurityValidatorNotInitialized = errors.New("security validator not initialized")
)
```

## 5. パフォーマンス設計

### 5.1 起動時間への影響

- **ハッシュ計算**: SHA-256計算は高速（設定ファイルサイズは通常小さい）
- **ファイルI/O**: 最小限（ハッシュファイル読み込み + 設定ファイル読み込み）
- **予想起動時間増加**: < 50ms

### 5.2 メモリ使用量

- **ハッシュファイル**: < 1KB（通常）
- **一時的なハッシュ計算**: < 32bytes per file
- **総メモリ増加**: < 10KB

## 6. テスト設計

### 6.1 テスト戦略

```
verification/
├── manager_test.go
│   ├─ TestNewManager
│   ├─ TestManager_IsEnabled
│   ├─ TestManager_VerifyConfigFile
│   ├─ TestManager_ValidateHashDirectory
│   └─ TestManager_DisabledMode
├── config_test.go
│   ├─ TestDefaultConfig
│   ├─ TestConfig_Validate
│   └─ TestConfig_IsEnabled
└── errors_test.go
    ├─ TestError_Error
    ├─ TestError_Unwrap
    └─ TestError_Is
```

### 6.2 モックファイルシステム

```go
// テストでのMockFileSystem使用
func TestManager_WithMockFS(t *testing.T) {
    mockFS := common.NewMockFileSystem()
    mockFS.AddFile("/etc/go-safe-cmd-runner/hashes/config.toml.hash", 0644, validHashData)

    vm := NewManagerWithFS(config, mockFS)
    err := vm.VerifyConfigFile("/path/to/config.toml")
    assert.NoError(t, err)
}
```

## 7. 実装状況

### 設定ファイル検証機能（実装完了）
- ✅ verification パッケージ実装
- ✅ main.go への統合
- ✅ テストケース作成
- ✅ エラーハンドリング
- ✅ 設定による有効/無効制御

### 将来の拡張（未実装）
- ⏳ 実行ファイル検証
- ⏳ 動的ライブラリ検証
- ⏳ 参照ファイル検証

## 8. 運用考慮事項

### 8.1 デプロイメント

```bash
# ハッシュディレクトリの作成
sudo mkdir -p /etc/go-safe-cmd-runner/hashes
sudo chown root:root /etc/go-safe-cmd-runner/hashes
sudo chmod 755 /etc/go-safe-cmd-runner/hashes

# 設定ファイルハッシュの記録
sudo ./build/record /path/to/config.toml

# 設定ファイルで検証機能を有効化
# [verification]
# enabled = true
# hash_directory = "/etc/go-safe-cmd-runner/hashes"
```

### 8.2 監視とロギング

```go
// 構造化ログの例
slog.Info("Configuration file verification successful",
    "config_path", configPath,
    "hash_algorithm", "SHA-256")

slog.Error("Configuration file verification failed",
    "config_path", configPath,
    "error", err.Error(),
    "hash_directory", hashDirectory)
```

## 9. 将来拡張

### 9.1 拡張ポイント

- 実行ファイルバイナリの検証
- 動的ライブラリの検証
- 設定で参照される外部ファイルの検証

### 9.2 設定拡張

```toml
[verification.advanced]  # 将来追加予定
verify_binaries = true
verify_libraries = true
verify_referenced_files = true
hash_cache_ttl = "1h"
```
