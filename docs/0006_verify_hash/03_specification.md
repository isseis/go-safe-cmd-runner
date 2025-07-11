# 詳細仕様書: ファイル改竄検出機能の実装

## 1. 概要

本仕様書では、ファイル改竄検出機能の詳細な実装仕様を定義する。

## 2. データ構造定義

### 2.1 ハッシュマニフェスト形式

#### 2.1.1 manifest.json 構造

```json
{
  "version": "1.0",
  "created_at": "2024-01-15T10:30:00Z",
  "created_by": "go-safe-cmd-runner-record",
  "algorithm": "SHA-256",
  "files": [
    {
      "path": "/etc/go-safe-cmd-runner/config.toml",
      "canonical_path": "/etc/go-safe-cmd-runner/config.toml",
      "hash": "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
      "size": 1024,
      "modified_at": "2024-01-15T10:29:30Z",
      "permissions": "0644",
      "owner": "root",
      "group": "root"
    }
  ]
}
```

#### 2.1.2 フィールド仕様

| フィールド | 型 | 必須 | 説明 |
|-----------|---|------|------|
| version | string | ○ | マニフェストファイルのバージョン |
| created_at | string (RFC3339) | ○ | マニフェスト作成日時 |
| created_by | string | ○ | 作成者（record コマンド） |
| algorithm | string | ○ | ハッシュアルゴリズム（"SHA-256"） |
| files | array | ○ | ファイル情報の配列 |
| files[].path | string | ○ | 元のファイルパス |
| files[].canonical_path | string | ○ | 正規化されたファイルパス |
| files[].hash | string | ○ | SHA-256ハッシュ値（16進数） |
| files[].size | integer | ○ | ファイルサイズ（バイト） |
| files[].modified_at | string (RFC3339) | ○ | ファイル最終更新日時 |
| files[].permissions | string | ○ | ファイル権限（8進数文字列） |
| files[].owner | string | ○ | ファイル所有者 |
| files[].group | string | ○ | ファイルグループ |

### 2.2 設定構造体

#### 2.2.1 VerificationConfig

```go
// internal/verification/config.go
type Config struct {
    // 検証機能の有効/無効
    Enabled bool `json:"enabled"`

    // ハッシュファイル格納ディレクトリ
    HashDirectory string `json:"hash_directory"`
}

// デフォルト設定（値型を返す）
func DefaultConfig() Config {
    return Config{
        Enabled:       true,  // デフォルトで有効
        HashDirectory: "/usr/local/etc/go-safe-cmd-runner/hashes",
    }
}
```

#### 2.2.2 マニフェスト構造体

```go
// filevalidator パッケージの既存構造体を使用
// 独自マニフェストは実装しない
```

## 3. インターフェース仕様

### 3.1 Manager インターフェース

```go
// internal/verification/manager.go
type Manager struct {
    config    Config                     // 値型に変更
    fs        common.FileSystem
    validator *filevalidator.Validator
    security  *security.Validator
}

// 主要メソッド
func NewManager(config Config) (*Manager, error)                   // 値渡し
func NewManagerWithFS(config Config, fs common.FileSystem) (*Manager, error) // 値渡し
func (vm *Manager) VerifyConfigFile(configPath string) error
func (vm *Manager) ValidateHashDirectory() error
func (vm *Manager) IsEnabled() bool
func (vm *Manager) GetConfig() Config                             // 値返し
```

### 3.2 メソッド詳細仕様

#### 3.2.1 NewManager

```go
func NewManager(config Config) (*Manager, error)
```

**パラメータ:**
- `config`: 検証設定（値渡し）

**戻り値:**
- `*Manager`: マネージャーインスタンス
- `error`: エラー（設定無効時等）

**処理内容:**
1. 設定のコピーと妥当性検証
2. filevalidator.Validator の初期化（有効時のみ）
3. security.Validator の初期化（有効時のみ、デフォルト設定使用）

#### 3.2.2 VerifyConfigFile

```go
func (vm *Manager) VerifyConfigFile(configPath string) error
```

**パラメータ:**
- `configPath`: 設定ファイルパス

**戻り値:**
- `error`: 検証エラー

**処理フロー:**
```
1. 有効性チェック
   ├─ 無効時: 何もしない（正常終了）
   └─ 有効時: 以下の処理を実行

2. ハッシュディレクトリ検証
   ├─ ディレクトリ存在チェック
   ├─ 権限チェック（root所有、755権限）
   └─ エラー時: ErrHashDirectoryInvalid

3. ハッシュ検証
   ├─ filevalidator.Verify() 呼び出し
   ├─ ハッシュ値比較
   └─ エラー時: filevalidator パッケージエラー
```

#### 3.2.3 ValidateHashDirectory

```go
func (vm *Manager) ValidateHashDirectory() error
```

**検証項目:**
1. 検証機能の有効性確認
2. ディレクトリ存在確認
3. 所有者が root であることを確認
4. 権限が 755 (rwxr-xr-x) であることを確認
5. 他ユーザーによる書き込み権限がないことを確認

**実装例:**
```go
func (vm *Manager) ValidateHashDirectory() error {
    if !vm.IsEnabled() {
        return fmt.Errorf("%w", ErrVerificationDisabled)
    }

    if vm.security == nil {
        return fmt.Errorf("%w", ErrSecurityValidatorNotInitialized)
    }

    // 権限チェック（securityパッケージ活用）
    return vm.security.ValidateDirectoryPermissions(vm.config.HashDirectory)
}
```

## 4. エラー仕様

### 4.1 エラー定義

```go
// internal/verification/errors.go
package verification

import "errors"

var (
    // 設定関連
    ErrVerificationDisabled = errors.New("verification is disabled")
    ErrHashDirectoryEmpty = errors.New("hash directory cannot be empty")
    ErrHashDirectoryInvalid = errors.New("hash directory is invalid")
    ErrConfigNil = errors.New("config cannot be nil")

    // マネージャー関連
    ErrSecurityValidatorNotInitialized = errors.New("security validator not initialized")
)

// 構造化エラー
type Error struct {
    Op       string // operation that failed
    Path     string // file path (if applicable)
    Expected string // expected value (if applicable)
    Actual   string // actual value (if applicable)
    Err      error  // underlying error
}

func (e *Error) Error() string {
    if e.Path != "" {
        return fmt.Sprintf("verification error in %s for %s: %v", e.Op, e.Path, e.Err)
    }
    return fmt.Sprintf("verification error in %s: %v", e.Op, e.Err)
}
```

### 4.2 エラーメッセージ設計

**原則:**
- ユーザーフレンドリーなメッセージ
- セキュリティ情報の過度な露出を避ける
- 運用者向けの詳細ログと分離

**例:**
```go
// ユーザー向けメッセージ
fmt.Errorf("Configuration file verification failed. Please contact system administrator.")

// ログ出力（詳細情報）
slog.Error("Config hash verification failed",
    "config_path", configPath,
    "expected_hash", expectedHash,
    "actual_hash", actualHash,
    "error", err)
```

## 5. ログ仕様

### 5.1 ログレベル定義

| レベル | 用途 | 例 |
|--------|------|---|
| DEBUG | 詳細な処理ログ | ハッシュ計算開始/終了、ファイルアクセス |
| INFO | 正常処理の記録 | 検証成功、マニフェスト読み込み成功 |
| WARN | 警告（Phase 1 等） | 検証機能未実装の警告 |
| ERROR | エラー事象 | 検証失敗、ファイル不存在 |

### 5.2 ログフォーマット

**構造化ログ（slog）使用:**
```go
// 成功時
slog.Info("Config file verification completed",
    "config_path", configPath,
    "hash_algorithm", "SHA-256",
    "verification_duration_ms", duration.Milliseconds(),
    "phase", vm.config.Phase)

// 失敗時
slog.Error("Config file verification failed",
    "config_path", configPath,
    "error", err.Error(),
    "error_type", reflect.TypeOf(err).String(),
    "phase", vm.config.Phase,
    "hash_directory", vm.config.HashDirectory)

// Phase 1 警告
slog.Warn("Configuration file integrity verification is not enabled",
    "phase", 1,
    "recommendation", "Enable verification in Phase 2 for production use",
    "security_risk", "Configuration files may be tampered without detection")
```

## 6. 設定ファイル統合仕様

### 6.1 設定制御の変更

**セキュリティ強化のため、設定ファイルからverificationセクションを削除。**

検証機能の制御は設定ファイルではなく、コマンドライン引数と環境変数のみで行う：

```toml
# sample/config.toml（verificationセクションは存在しない）
[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"

[groups.example]
# 既存のグループ設定...
```

**コマンドライン引数による制御：**
```bash
# デフォルト（検証有効）
./runner --config config.toml

# 検証無効化
./runner --disable-verification --config config.toml

# カスタムハッシュディレクトリ
./runner --hash-directory /custom/path --config config.toml
```

**環境変数による制御：**
```bash
# 検証無効化
GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION=true ./runner --config config.toml

# カスタムハッシュディレクトリ
GO_SAFE_CMD_RUNNER_HASH_DIRECTORY=/custom/path ./runner --config config.toml
```

### 6.2 ビルド時設定

```makefile
# Makefile
DEFAULT_HASH_DIRECTORY=/usr/local/etc/go-safe-cmd-runner/hashes
BUILD_FLAGS=-ldflags "-X main.DefaultHashDirectory=$(DEFAULT_HASH_DIRECTORY)"
```

### 6.3 runnertypes.Config は変更なし

設定ファイルからverificationセクションを削除したため、runnertypes.Configの変更は不要です。

## 7. テスト仕様

### 7.1 ユニットテスト

#### 7.1.1 VerificationManager テスト

```go
// internal/verification/manager_test.go
func TestVerificationManager_VerifyConfigFile_Success(t *testing.T) {
    // MockFileSystem セットアップ
    mockFS := common.NewMockFileSystem()

    // テストデータ準備
    configContent := `version = "1.0"
[global]
timeout = 3600`
    configHash := calculateSHA256([]byte(configContent))

    manifest := &Manifest{
        Version:   "1.0",
        Algorithm: "SHA-256",
        Files: []FileHash{{
            Path:          "/test/config.toml",
            CanonicalPath: "/test/config.toml",
            Hash:          configHash,
            Permissions:   "0644",
        }},
    }

    // MockFileSystem にファイル追加
    mockFS.AddFile("/test/config.toml", 0644, []byte(configContent))
    mockFS.AddFile("/usr/local/etc/go-safe-cmd-runner/hashes/manifest.json", 0644, manifestToJSON(manifest))

    // テスト実行
    config := &Config{
        Enabled:       true,
        Phase:         2,
        HashDirectory: "/usr/local/etc/go-safe-cmd-runner/hashes",
        ManifestFile:  "manifest.json",
    }

    vm, err := NewVerificationManagerWithFS(config, mockFS)
    require.NoError(t, err)

    err = vm.VerifyConfigFile("/test/config.toml")
    assert.NoError(t, err)
}

func TestVerificationManager_VerifyConfigFile_HashMismatch(t *testing.T) {
    // ハッシュ不一致のテストケース
    // ...
}

func TestVerificationManager_VerifyConfigFile_Phase1Warning(t *testing.T) {
    // Phase 1 警告モードのテストケース
    // ...
}
```

### 7.2 統合テスト

#### 7.2.1 エンドツーエンドテスト

```go
// internal/verification/integration_test.go
func TestEndToEnd_VerificationSuccess(t *testing.T) {
    // 1. テスト環境セットアップ
    tempDir := t.TempDir()
    configPath := filepath.Join(tempDir, "config.toml")
    hashDir := filepath.Join(tempDir, "hashes")

    // 2. 設定ファイル作成
    configContent := validConfigContent
    err := os.WriteFile(configPath, []byte(configContent), 0644)
    require.NoError(t, err)

    // 3. ハッシュ記録
    recorder := filevalidator.New(hashDir, sha256.New())
    err = recorder.Record(configPath)
    require.NoError(t, err)

    // 4. 検証実行
    config := Config{  // 値型に変更
        Enabled:       true,
        HashDirectory: hashDir,
    }

    vm, err := NewManager(config)  // 値渡しに変更
    require.NoError(t, err)

    err = vm.VerifyConfigFile(configPath)
    assert.NoError(t, err)
}
```

### 7.3 セキュリティテスト

```go
func TestSecurity_InvalidPermissions(t *testing.T) {
    testCases := []struct {
        name        string
        permissions os.FileMode
        shouldFail  bool
    }{
        {"valid_permissions_644", 0644, false},
        {"valid_permissions_600", 0600, false},
        {"invalid_world_writable", 0666, true},
        {"invalid_group_writable", 0664, true},
        {"invalid_executable", 0755, true},
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // テスト実装
        })
    }
}
```

## 8. パフォーマンス仕様

### 8.1 性能要件

| 項目 | 要件 | 測定方法 |
|------|------|----------|
| 起動時間増加 | < 100ms | time コマンドでの測定 |
| メモリ使用量増加 | < 1MB | pprof での測定 |
| ハッシュ計算時間 | < 10ms (1MB設定ファイル) | ベンチマークテスト |

### 8.2 パフォーマンステスト

```go
// internal/verification/manager_bench_test.go
func BenchmarkVerificationManager_VerifyConfigFile(b *testing.B) {
    // ベンチマーク実装
    for i := 0; i < b.N; i++ {
        err := vm.VerifyConfigFile(configPath)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## 9. 互換性仕様

### 9.1 後方互換性

- Phase 1 では既存機能に影響なし
- Phase 2 以降は新規インストール時のみ有効
- 既存設定ファイルは変更不要（verification セクションはオプション）

### 9.2 アップグレード手順

```bash
# 1. バイナリ更新
sudo cp ./build/runner /usr/local/bin/go-safe-cmd-runner

# 2. ハッシュディレクトリ作成
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# 3. 設定ファイルハッシュ記録
sudo /usr/local/bin/go-safe-cmd-runner record /usr/local/etc/go-safe-cmd-runner/config.toml

# 4. 検証機能確認（デフォルトで有効）
# 無効化する場合は --disable-verification フラグまたは環境変数を使用
```
