# Verification Manager API Documentation

## 概要

このドキュメントは、ハッシュディレクトリセキュリティ強化後のVerification Manager APIの使用方法を説明します。

## 重要な変更点

### BREAKING CHANGES

1. **`--hash-directory`フラグの削除**
   - プロダクション環境でのカスタムハッシュディレクトリ指定は不可能
   - デフォルトハッシュディレクトリ（`/usr/local/etc/go-safe-cmd-runner/hashes`）のみ使用

2. **プロダクション用APIの分離**
   - `verification.NewManager()`: プロダクション専用API
   - `verification.NewManagerForTest()`: テスト専用API（`//go:build test`タグ必須）

3. **セキュリティ制約の強化**
   - テスト用API誤用の自動検出
   - ビルドタグによるAPI分離
   - 静的解析による不正使用検出

## プロダクション用API

### NewManager()

**用途**: プロダクション環境での検証マネージャー作成

```go
package main

import (
    "github.com/isseis/go-safe-cmd-runner/internal/verification"
)

func main() {
    // プロダクション用マネージャー作成
    manager, err := verification.NewManager()
    if err != nil {
        log.Fatal(err)
    }

    // 使用例
    validator, err := manager.CreateValidator("unique_file_id")
    if err != nil {
        log.Fatal(err)
    }
}
```

**特徴**:
- デフォルトハッシュディレクトリ強制使用
- セキュリティログ自動記録
- プロダクション用制約適用

**制限事項**:
- カスタムハッシュディレクトリ指定不可
- テスト用オプション利用不可

## テスト用API

### NewManagerForTest()

**用途**: テスト環境での検証マネージャー作成

```go
//go:build test

package main_test

import (
    "testing"
    "github.com/isseis/go-safe-cmd-runner/internal/verification"
)

func TestVerificationManager(t *testing.T) {
    // テスト用マネージャー作成
    manager, err := verification.NewManagerForTest("/tmp/test-hashes")
    if err != nil {
        t.Fatal(err)
    }

    // テスト用オプション付きで作成
    manager, err = verification.NewManagerForTest("/tmp/test-hashes",
        verification.WithFileValidatorDisabled(),
        verification.WithFS(mockFS),
    )
    if err != nil {
        t.Fatal(err)
    }
}
```

**特徴**:
- カスタムハッシュディレクトリ指定可能
- テスト用オプション利用可能
- 呼び出し元検証（テストファイルからのみ呼び出し可能）

**制限事項**:
- `//go:build test`タグ必須
- テストファイルからの呼び出しのみ許可
- プロダクションビルドでは除外

## 利用可能オプション（テスト専用）

### WithFileValidatorDisabled()

ファイル検証を無効化（テスト専用）

```go
manager, err := verification.NewManagerForTest("/tmp/test",
    verification.WithFileValidatorDisabled(),
)
```

### WithFS(fs FileSystem)

カスタムファイルシステム指定（モック対応）

```go
manager, err := verification.NewManagerForTest("/tmp/test",
    verification.WithFS(mockFileSystem),
)
```

## マイグレーションガイド

### 変更前（非推奨）
```go
// ❌ 削除されたAPI
hashDir, err := hashdir.GetWithValidation(customHashDir, defaultHashDir)
verificationManager, err := bootstrap.InitializeVerificationManager(hashDir, runID)
```

### 変更後（推奨）

#### プロダクション環境
```go
// ✅ プロダクション用API
verificationManager, err := verification.NewManager()
if err != nil {
    log.Fatal(err)
}
```

#### テスト環境
```go
//go:build test

// ✅ テスト用API
verificationManager, err := verification.NewManagerForTest("/tmp/test-hashes")
if err != nil {
    t.Fatal(err)
}
```

## セキュリティ考慮事項

### 1. API分離

- プロダクション用APIでは外部からのハッシュディレクトリ指定を完全に禁止
- テスト用APIは明示的なビルドタグでのみ利用可能

### 2. 呼び出し元検証

- テスト用APIは`runtime.Caller`を使用してテストファイルからの呼び出しのみ許可
- 不正な呼び出しは`ProductionAPIViolationError`エラーで拒否

### 3. 静的解析

- `golangci-lint forbidigo`による不正API使用検出
- `additional-security-checks.py`による補助的セキュリティ検証

## エラーハンドリング

### HashDirectorySecurityError

ハッシュディレクトリセキュリティエラー

```go
if err != nil {
    if errors.Is(err, &verification.HashDirectorySecurityError{}) {
        // セキュリティ制約違反の処理
    }
}
```

### ProductionAPIViolationError

プロダクションAPI違反エラー（テスト用API誤用）

```go
if err != nil {
    if errors.Is(err, &verification.ProductionAPIViolationError{}) {
        // API誤用の処理
    }
}
```

## ベストプラクティス

### 1. プロダクションコード

```go
package main

import (
    "log"
    "github.com/isseis/go-safe-cmd-runner/internal/verification"
)

func main() {
    // ✅ シンプルなプロダクション用API
    manager, err := verification.NewManager()
    if err != nil {
        log.Fatal("Failed to create verification manager:", err)
    }

    // 通常の使用
    validator, err := manager.CreateValidator("file_id")
    if err != nil {
        log.Fatal("Failed to create validator:", err)
    }
}
```

### 2. テストコード

```go
//go:build test

package main_test

import (
    "path/filepath"
    "testing"
    "github.com/isseis/go-safe-cmd-runner/internal/verification"
)

func TestSomething(t *testing.T) {
    // ✅ テスト用一時ディレクトリ使用
    tempDir := t.TempDir()
    hashDir := filepath.Join(tempDir, "hashes")

    manager, err := verification.NewManagerForTest(hashDir)
    if err != nil {
        t.Fatal("Failed to create test manager:", err)
    }

    // テスト実行
}

func TestWithMockFS(t *testing.T) {
    mockFS := &MockFileSystem{}

    // ✅ モック使用
    manager, err := verification.NewManagerForTest("/mock/hashes",
        verification.WithFS(mockFS),
        verification.WithFileValidatorDisabled(),
    )
    if err != nil {
        t.Fatal("Failed to create mock manager:", err)
    }
}
```

### 3. 避けるべきパターン

```go
// ❌ プロダクションコードでテスト用API使用（ビルドエラー）
manager, err := verification.NewManagerForTest("/custom/path")

// ❌ 削除されたAPI使用（コンパイルエラー）
hashDir, err := hashdir.GetWithValidation(custom, default)

// ❌ 不正なビルドタグ（静的解析で検出）
// ビルドタグなしでテスト用API使用
```

## 参考資料

- [実装計画書](docs/tasks/0022_hash_directory_security_enhancement/04_implementation_plan.md)
- [セキュリティ強化詳細設計](docs/tasks/0022_hash_directory_security_enhancement/03_specification.md)
- [アーキテクチャ設計](docs/tasks/0022_hash_directory_security_enhancement/02_architecture.md)
