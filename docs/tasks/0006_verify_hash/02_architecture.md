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
    config    Config                     // 設定（値型）
    fs        common.FileSystem          // ファイルシステム抽象化
    validator *filevalidator.Validator   // ファイル検証用
    security  *security.Validator        // セキュリティ検証用
}

type Config struct {
    Enabled       bool   `json:"enabled"`        // 検証機能の有効/無効
    HashDirectory string `json:"hash_directory"` // ハッシュファイル格納ディレクトリ
}

// メソッド
func NewManager(config Config) (*Manager, error)
func NewManagerWithFS(config Config, fs common.FileSystem) (*Manager, error)
func (vm *Manager) VerifyConfigFile(configPath string) error
func (vm *Manager) ValidateHashDirectory() error
func (vm *Manager) IsEnabled() bool
func (vm *Manager) GetConfig() Config
```

### 3.2 既存コンポーネントとの統合

#### 3.2.1 main.go の変更点

```go
func main() {
    // 1. コマンドライン引数と環境変数から検証設定を取得
    verificationConfig := getVerificationConfig()

    // 2. 検証マネージャーの初期化
    verificationManager, err := verification.NewManager(verificationConfig)
    if err != nil {
        return fmt.Errorf("failed to initialize verification: %w", err)
    }

    // 3. 設定ファイル検証（設定ファイル読み込み前）
    if err := verificationManager.VerifyConfigFile(*configPath); err != nil {
        return fmt.Errorf("config verification failed: %w", err)
    }

    // 4. 設定ファイル読み込み
    cfgLoader := config.NewLoader()
    cfg, err := cfgLoader.LoadConfig(*configPath)
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // 5. 既存のRunner処理
    runner, err := runner.NewRunnerWithComponents(cfg, cfgLoader.GetTemplateEngine(), nil)
    // ...
}

// 検証設定取得関数
func getVerificationConfig() verification.Config {
    // デフォルト: 検証有効
    enabled := true
    hashDir := *hashDirectory

    // 環境変数チェック
    if envDisable := os.Getenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION"); envDisable != "" {
        if parsedDisable, err := strconv.ParseBool(envDisable); err == nil && parsedDisable {
            enabled = false
        }
    }

    if envHashDir := os.Getenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY"); envHashDir != "" {
        hashDir = envHashDir
    }

    // コマンドライン引数が環境変数より優先
    if *disableVerification {
        enabled = false
    }

    return verification.Config{
        Enabled:       enabled,
        HashDirectory: hashDir,
    }
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

### 3.4 設定制御

設定ファイルからverificationセクションを削除し、セキュリティを向上させました。
検証機能の制御はコマンドライン引数と環境変数のみで行います。

**コマンドライン引数:**
```bash
# 検証を無効化
./runner --disable-verification --config config.toml

# カスタムハッシュディレクトリ
./runner --hash-directory /custom/hash/dir --config config.toml
```

**環境変数:**
```bash
# 検証を無効化
GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION=true ./runner --config config.toml

# カスタムハッシュディレクトリ
GO_SAFE_CMD_RUNNER_HASH_DIRECTORY=/custom/hash/dir ./runner --config config.toml
```

**ビルド時設定:**
```makefile
# Makefile
DEFAULT_HASH_DIRECTORY=/usr/local/etc/go-safe-cmd-runner/hashes
BUILD_FLAGS=-ldflags "-X main.DefaultHashDirectory=$(DEFAULT_HASH_DIRECTORY)"
```

**設定ファイル例:**
```toml
# sample/config.toml（verificationセクションは存在しない）
[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"

[groups.example]
# 既存の設定...
```

## 4. セキュリティ設計

### 4.1 ハッシュファイル保護

```
/usr/local/etc/go-safe-cmd-runner/hashes/
├── config.toml.hash      # 設定ファイルハッシュ
│   └── 権限: 644 (root:root)
└── manifest.json         # ハッシュメタデータ（filevalidator形式）
    └── 権限: 644 (root:root)
```

### 4.2 検証順序

1. **コマンドライン・環境変数による設定取得**
   - デフォルト: 検証有効
   - 環境変数による無効化確認
   - コマンドライン引数による最終制御

2. **ハッシュディレクトリ完全パス検証**
   - **中間ディレクトリ権限検証**: ルートディレクトリから対象ディレクトリまでの全パスコンポーネントを検証
   - **シンボリックリンク攻撃対策**: `safefileio`パッケージの`openat2`システムコールと`RESOLVE_NO_SYMLINKS`フラグにより自動的に保護
   - **権限チェック**: 各中間ディレクトリが他ユーザー書き込み不可、システムディレクトリ以外はグループ書き込み不可

3. **設定ファイル検証**
   - filevalidator.Validator を使用してハッシュ値検証
   - ハッシュ不一致時はエラー終了

### 4.3 エラーハンドリング

```go
// verification/errors.go と security/security.go
var (
    ErrVerificationDisabled = errors.New("verification is disabled")
    ErrHashDirectoryEmpty = errors.New("hash directory cannot be empty")
    ErrHashDirectoryInvalid = errors.New("hash directory is invalid")
    ErrConfigNil = errors.New("config cannot be nil")
    ErrSecurityValidatorNotInitialized = errors.New("security validator not initialized")

    // 完全パス検証用エラー
    ErrInsecurePathComponent = errors.New("insecure path component")
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
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# 設定ファイルハッシュの記録
sudo ./build/record -file /path/to/config.toml -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 検証機能はデフォルトで有効（設定ファイルにはverificationセクションは存在しない）
# 無効化する場合は --disable-verification フラグまたは環境変数を使用

# カスタムハッシュディレクトリを使う場合
sudo mkdir -p /custom/hash/directory
sudo chown root:root /custom/hash/directory
sudo chmod 755 /custom/hash/directory
sudo ./build/record -file /path/to/config.toml -hash-dir /custom/hash/directory
./runner --hash-directory /custom/hash/directory --config /path/to/config.toml
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
