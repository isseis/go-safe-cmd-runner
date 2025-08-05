# 権限昇格期間最小化 - 詳細設計書

## 1. 詳細設計概要

### 1.1 設計スコープ

本詳細設計書は、権限昇格期間最小化のための具体的な実装仕様を定義する。
対象となるファイルと機能：

- 既存: `internal/filevalidator/validator.go` (拡張)
- 新規実装: `internal/filevalidator/privileged_file.go`

### 1.2 実装戦略

1. **API分離**: ファイルオープンとファイル検証処理の分離
2. **段階的実装**: 既存コードを破綻させずに段階的に改修
3. **テスト駆動**: 新機能のテストを先行実装
4. **後方互換性**: 既存APIの完全な互換性維持

## 2. 関数仕様設計

### 2.1 OpenFileWithPrivileges

#### 2.1.1 関数シグネチャ

```go
// internal/filevalidator/privileged_file.go

package filevalidator

import (
    "fmt"
    "os"
    "syscall"
    "time"
    "log/slog"
)

// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them
func OpenFileWithPrivileges(filepath string) (*os.File, error)
```

#### 2.1.2 エラー型定義

```go
// PrivilegeError represents privilege-related errors
type PrivilegeError struct {
    Operation string // "escalate" or "restore"
    UID       int
    Cause     error
}

func (e *PrivilegeError) Error() string {
    return fmt.Sprintf("privilege %s failed for UID %d: %v", e.Operation, e.UID, e.Cause)
}

func (e *PrivilegeError) Unwrap() error {
    return e.Cause
}
```

### 2.2 VerifyFromHandle

#### 2.2.1 メソッドシグネチャ

```go
// VerifyFromHandle verifies a file's hash using an already opened file handle
func (v *Validator) VerifyFromHandle(file *os.File, targetPath string) error
```

#### 2.2.2 処理概要

1. ファイルハンドルから内容を読み取り
2. ハッシュを計算
3. 記録されたハッシュと比較
4. 結果を返却

### 2.3 権限判定関数

#### 2.3.1 needsPrivileges

```go
// needsPrivileges determines if a file requires privilege escalation to access
func needsPrivileges(filepath string) bool {
    // ファイルアクセステストで権限必要性を判定
    _, err := os.Open(filepath)
    return os.IsPermission(err)
}
```

## 3. 実装詳細

### 3.1 OpenFileWithPrivileges 実装

```go
// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them
func OpenFileWithPrivileges(filepath string) (*os.File, error) {
    // 現在のUIDを保存
    originalUID := os.Getuid()

    // 権限昇格
    if err := syscall.Seteuid(0); err != nil {
        return nil, &PrivilegeError{
            Operation: "escalate",
            UID:       0,
            Cause:     err,
        }
    }

    // deferで確実に権限復元
    defer func() {
        if restoreErr := syscall.Seteuid(originalUID); restoreErr != nil {
            slog.Error("Failed to restore privileges",
                slog.String("error", restoreErr.Error()),
                slog.String("file", filepath))
        }
    }()

    // ファイルオープン（権限昇格状態）
    file, err := os.Open(filepath)
    if err != nil {
        return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
    }

    return file, nil
}
```

### 3.2 VerifyFromHandle 実装

```go
// VerifyFromHandle verifies a file's hash using an already opened file handle
func (v *Validator) VerifyFromHandle(file *os.File, targetPath string) error {
    // ファイル内容を読み取り（一般権限）
    content, err := io.ReadAll(file)
    if err != nil {
        return fmt.Errorf("failed to read file content: %w", err)
    }

    // ハッシュを計算（一般権限）
    actualHash, err := v.algorithm.Sum(bytes.NewReader(content))
    if err != nil {
        return fmt.Errorf("failed to calculate hash: %w", err)
    }

    // 記録されたハッシュを読み取り（一般権限）
    _, expectedHash, err := v.readAndParseHashFile(targetPath)
    if err != nil {
        return err
    }

    // ハッシュ比較
    if expectedHash != actualHash {
        return ErrMismatch
    }

    return nil
}
```

### 3.3 Verify メソッドの統合

```go
// Verify checks if the file at filePath matches its recorded hash.
// Automatically uses privilege escalation if needed.
func (v *Validator) Verify(filePath string) error {
    // パスを正規化
    targetPath, err := validatePath(filePath)
    if err != nil {
        return err
    }

    // 権限が必要か判定
    if needsPrivileges(targetPath) {
        // 権限昇格でファイルオープン
        file, err := OpenFileWithPrivileges(targetPath)
        if err != nil {
            return err
        }
        defer file.Close()

        // ファイルハンドルから検証（一般権限）
        return v.VerifyFromHandle(file, targetPath)
    }

    // 通常の検証処理（既存ロジック）
    return v.verifyNormally(targetPath)
}
```

### 3.4 verifyNormally ヘルパー関数

```go
// verifyNormally performs normal file verification without privilege escalation
func (v *Validator) verifyNormally(targetPath string) error {
    // 既存の検証ロジックを使用
    actualHash, err := v.calculateHash(targetPath)
    if os.IsNotExist(err) {
        return err
    }
    if err != nil {
        return fmt.Errorf("failed to calculate file hash: %w", err)
    }

    _, expectedHash, err := v.readAndParseHashFile(targetPath)
    if err != nil {
        return err
    }

    if expectedHash != actualHash {
        return ErrMismatch
    }

    return nil
}
```

## 4. テスト設計

### 4.1 OpenFileWithPrivileges テスト

```go
func TestOpenFileWithPrivileges(t *testing.T) {
    tests := []struct {
        name        string
        filepath    string
        setup       func(t *testing.T) string
        expectError bool
        errorType   string
    }{
        {
            name:     "normal file access",
            setup:    func(t *testing.T) string { return createTestFile(t) },
            expectError: false,
        },
        {
            name:     "root-only file access",
            setup:    func(t *testing.T) string { return createRootOnlyFile(t) },
            expectError: false, // should succeed with privilege escalation
        },
        {
            name:        "non-existent file",
            filepath:    "/tmp/non_existent_file",
            expectError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var filepath string
            if tt.setup != nil {
                filepath = tt.setup(t)
                defer os.Remove(filepath)
            } else {
                filepath = tt.filepath
            }

            file, err := OpenFileWithPrivileges(filepath)
            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, file)
                file.Close()
            }
        })
    }
}
```

### 4.2 VerifyFromHandle テスト

```go
func TestValidator_VerifyFromHandle(t *testing.T) {
    validator, err := New(NewSHA256(), t.TempDir())
    require.NoError(t, err)

    // テストファイル作成
    testFile := createTestFileWithContent(t, "test content")
    defer os.Remove(testFile)

    // ハッシュ記録
    _, err = validator.Record(testFile)
    require.NoError(t, err)

    // ファイルオープン
    file, err := os.Open(testFile)
    require.NoError(t, err)
    defer file.Close()

    // VerifyFromHandle テスト
    err = validator.VerifyFromHandle(file, testFile)
    assert.NoError(t, err)
}
```

### 4.3 統合テスト

```go
func TestPrivilegeMinimization_Integration(t *testing.T) {
    validator, err := New(NewSHA256(), t.TempDir())
    require.NoError(t, err)

    // root専用ファイル作成
    rootFile := createRootOnlyTestFile(t)
    defer removeTestFile(t, rootFile)

    // ハッシュ記録（権限昇格を使用）
    _, err = validator.Record(rootFile)
    require.NoError(t, err)

    // 検証（権限昇格を使用）
    err = validator.Verify(rootFile)
    assert.NoError(t, err)
}
```

## 5. エラーハンドリング

### 5.1 権限エラーの処理

```go
// IsPrivilegeError checks if error is a privilege-related error
func IsPrivilegeError(err error) bool {
    var privErr *PrivilegeError
    return errors.As(err, &privErr)
}
```

### 5.2 緊急時権限復元

権限復元に失敗した場合の処理：

1. エラーログの記録
2. セキュリティアラート
3. プロセス終了の検討

## 6. 実装順序

### Phase 1: 基本実装
1. `OpenFileWithPrivileges()` 関数の実装
2. `VerifyFromHandle()` メソッドの実装
3. `needsPrivileges()` ヘルパー関数の実装

### Phase 2: 統合
1. `Verify()` メソッドの権限判定ロジック統合
2. エラーハンドリングの実装
3. ログ機能の追加

### Phase 3: テスト
1. 単体テストの実装
2. 統合テストの実装
3. セキュリティテストの実行

この設計により、権限昇格期間を最小化し、セキュリティリスクを大幅に削減できます。

## 7. 実装完了状況

### 7.1 実装済みAPI

#### OpenFileWithPrivileges

```go
// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them
// This function uses the existing privilege management infrastructure
func OpenFileWithPrivileges(filepath string, privManager runnertypes.PrivilegeManager) (*os.File, error)
```

- **機能**: 指定されたファイルを権限昇格を使用してオープン
- **最適化**: 通常アクセスを先に試行し、権限エラーの場合のみ昇格
- **権限期間**: `WithPrivileges`コールバック内のみ

#### VerifyFromHandle

```go
// VerifyFromHandle verifies a file's hash using an already opened file handle
func (v *Validator) VerifyFromHandle(file *os.File, targetPath string) error
```

- **機能**: 既にオープンされたファイルハンドルからハッシュ検証
- **セキュリティ**: 権限昇格期間と検証処理を分離

#### VerifyWithPrivileges

```go
// VerifyWithPrivileges verifies a file's integrity using privilege escalation
func (v *Validator) VerifyWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) error
```

- **機能**: 権限昇格を使用したファイル検証
- **統合**: `OpenFileWithPrivileges`と`VerifyFromHandle`を組み合わせ

### 7.2 セキュリティ強化事項

1. **権限昇格期間の最小化**: ファイルオープンのみに限定
2. **フェイルセーフ機構**: 権限復元の確実な実行
3. **エラーハンドリング**: 静的エラー変数による一貫した処理
4. **既存API互換性**: 完全な後方互換性を維持

### 7.3 パフォーマンス最適化

- **Fast Path**: 通常アクセス可能なファイルは権限昇格をスキップ
- **早期リターン**: 権限エラー以外は即座に処理終了
- **最小権限原則**: 必要最小限の権限と期間のみ使用

### 7.4 品質保証

- **テストカバレッジ**: 単体テスト、統合テスト、エラーケースを網羅
- **リント適合**: golangci-lintでエラーゼロ
- **セキュリティ監査**: 権限処理の安全性を検証済み
