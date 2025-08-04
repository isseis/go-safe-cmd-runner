# 権限昇格期間最小化 - 実装計画書

## 1. 実装概要

### 1.1 実装目標

権限昇格期間を最小化するため、ファイルオープンとファイル検証処理を分離し、以下の新規APIを追加する：

- `OpenFileWithPrivileges(filepath string) (*os.File, error)` - 権限昇格でのファイルオープン
- `Validator.VerifyFromHandle(file *os.File, targetPath string) error` - ファイルハンドルからの検証

### 1.2 実装方針

- **段階的実装**: 既存機能に影響を与えない段階的な追加
- **テスト駆動開発**: 各機能のテスト実装を先行
- **後方互換性**: 既存APIの完全な互換性維持
- **セキュリティ優先**: 権限管理の安全性を最優先

## 2. 実装スケジュール

### Phase 1: 基本API実装 (1-2日)
- [ ] `OpenFileWithPrivileges` 関数の実装
- [ ] `VerifyFromHandle` メソッドの実装
- [ ] `needsPrivileges` ヘルパー関数の実装
- [ ] 基本的なエラーハンドリング

### Phase 2: 統合と安全性 (1-2日)
- [ ] `Verify` メソッドの権限判定ロジック統合
- [ ] 権限復元の安全性強化
- [ ] セキュリティログの追加
- [ ] 包括的なエラーハンドリング

### Phase 3: テストと検証 (1-2日)
- [ ] 単体テストの実装
- [ ] 統合テストの実装
- [ ] セキュリティテストの実行
- [ ] パフォーマンステスト

### Phase 4: 最適化と仕上げ (1日)
- [ ] パフォーマンス最適化
- [ ] ログレベルの調整
- [ ] ドキュメント更新
- [ ] 最終検証

**総実装期間: 4-7日**

## 3. 実装詳細

### 3.1 Phase 1: 基本API実装

#### 3.1.1 新規ファイル作成

**ファイル**: `internal/filevalidator/privileged_file.go`

```go
package filevalidator

import (
    "fmt"
    "os"
    "syscall"
    "log/slog"
)

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

// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them
func OpenFileWithPrivileges(filepath string) (*os.File, error) {
    // 実装内容は詳細設計書に基づく
}

// needsPrivileges determines if a file requires privilege escalation to access
func needsPrivileges(filepath string) bool {
    // 実装内容は詳細設計書に基づく
}

// IsPrivilegeError checks if error is a privilege-related error
func IsPrivilegeError(err error) bool {
    var privErr *PrivilegeError
    return errors.As(err, &privErr)
}
```

#### 3.1.2 既存ファイル拡張

**ファイル**: `internal/filevalidator/validator.go`

```go
// VerifyFromHandle verifies a file's hash using an already opened file handle
func (v *Validator) VerifyFromHandle(file *os.File, targetPath string) error {
    // 実装内容は詳細設計書に基づく
}

// verifyNormally performs normal file verification without privilege escalation
func (v *Validator) verifyNormally(targetPath string) error {
    // 既存のVerifyロジックを抽出
}
```

#### 3.1.3 テストファイル作成

**ファイル**: `internal/filevalidator/privileged_file_test.go`

```go
package filevalidator

import (
    "os"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestOpenFileWithPrivileges(t *testing.T) {
    // テストケース実装
}

func TestNeedsPrivileges(t *testing.T) {
    // テストケース実装
}
```

**ファイル**: `internal/filevalidator/validator_test.go` (拡張)

```go
func TestValidator_VerifyFromHandle(t *testing.T) {
    // テストケース実装
}
```

### 3.2 Phase 2: 統合と安全性

#### 3.2.1 Verifyメソッドの統合

**ファイル**: `internal/filevalidator/validator.go`

```go
// Verify checks if the file at filePath matches its recorded hash.
// Automatically uses privilege escalation if needed.
func (v *Validator) Verify(filePath string) error {
    // 既存のvalidatePathロジック
    targetPath, err := validatePath(filePath)
    if err != nil {
        return err
    }

    // 権限判定と分岐処理
    if needsPrivileges(targetPath) {
        file, err := OpenFileWithPrivileges(targetPath)
        if err != nil {
            return err
        }
        defer file.Close()

        return v.VerifyFromHandle(file, targetPath)
    }

    // 既存の通常処理
    return v.verifyNormally(targetPath)
}
```

#### 3.2.2 セキュリティログの追加

```go
// ログ設定の追加
var logger = slog.Default()

// 権限操作時のログ記録
func logPrivilegeOperation(operation string, filepath string, success bool, err error) {
    level := slog.LevelDebug
    if !success {
        level = slog.LevelError
    }

    logger.Log(context.Background(), level, "Privilege operation",
        slog.String("operation", operation),
        slog.String("file", filepath),
        slog.Bool("success", success),
        slog.Int("uid", os.Getuid()),
        slog.String("error", func() string {
            if err != nil {
                return err.Error()
            }
            return ""
        }()))
}
```

### 3.3 Phase 3: テストと検証

#### 3.3.1 統合テストの実装

**ファイル**: `internal/filevalidator/integration_test.go`

```go
package filevalidator

import (
    "os"
    "path/filepath"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestPrivilegeMinimization_Integration(t *testing.T) {
    // root権限でのテストファイル作成
    // 権限昇格を使った検証テスト
    // 権限昇格期間の測定
}

func TestValidator_Verify_WithPrivileges(t *testing.T) {
    // 統合された Verify メソッドのテスト
}

// テストヘルパー関数
func createRootOnlyTestFile(t *testing.T) string {
    // root専用ファイルの作成
}

func createTestFileWithContent(t *testing.T, content string) string {
    // テストファイルの作成
}
```

#### 3.3.2 セキュリティテストの実装

```go
func TestSecurity_PrivilegeRestoration(t *testing.T) {
    // 権限復元の確実性テスト
}

func TestSecurity_PrivilegeEscalationMinimization(t *testing.T) {
    // 権限昇格期間の最小化検証
}
```

### 3.4 Phase 4: 最適化と仕上げ

#### 3.4.1 パフォーマンス最適化

- 権限チェックの最適化
- ファイルI/Oの効率化
- メモリ使用量の最適化

#### 3.4.2 ドキュメント更新

- API仕様の更新
- 使用例の追加
- セキュリティガイドライン

## 4. 実装チェックリスト

### 4.1 機能要件
- [ ] `OpenFileWithPrivileges` 関数が正常に動作する
- [ ] `VerifyFromHandle` メソッドが正常に動作する
- [ ] 権限判定ロジックが正確に動作する
- [ ] 既存の `Verify` メソッドが正常に動作する
- [ ] 既存の `Record` メソッドに影響がない

### 4.2 セキュリティ要件
- [ ] 権限昇格が必要最小限の期間のみ実行される
- [ ] 権限復元が確実に実行される
- [ ] パニック発生時も権限復元が実行される
- [ ] 権限操作がログに記録される
- [ ] セキュリティエラーが適切に処理される

### 4.3 品質要件
- [ ] すべての単体テストが成功する
- [ ] すべての統合テストが成功する
- [ ] コードカバレッジが90%以上
- [ ] リントエラーがゼロ
- [ ] パフォーマンス劣化が5%以内

### 4.4 互換性要件
- [ ] 既存のAPIが変更されていない
- [ ] 既存のテストがすべて成功する
- [ ] 既存の設定ファイルが動作する
- [ ] 既存のエラーハンドリングが動作する

## 5. リスク管理

### 5.1 技術的リスク

| リスク | 影響度 | 発生確率 | 対策 |
|--------|--------|----------|------|
| 権限復元失敗 | 高 | 低 | フェイルセーフ機構、包括的テスト |
| 既存機能の破綻 | 高 | 中 | 段階的実装、回帰テスト |
| パフォーマンス劣化 | 中 | 中 | パフォーマンステスト、プロファイリング |
| 権限判定の誤り | 高 | 低 | 詳細テスト、セキュリティレビュー |

### 5.2 緊急時対応

- **ロールバック計画**: 実装前の状態に即座に戻せる準備
- **セキュリティインシデント対応**: 権限関連の問題発生時の対応手順
- **エスカレーション**: 重大な問題発生時の連絡体制

## 6. 成功基準

### 6.1 定量的基準
- [ ] 権限昇格期間を50%以上短縮
- [ ] 既存テストの100%成功
- [ ] 新規テストのカバレッジ90%以上
- [ ] パフォーマンス劣化5%以内

### 6.2 定性的基準
- [ ] コードレビューでの設計承認
- [ ] セキュリティ監査での問題指摘ゼロ
- [ ] 最小権限の原則への準拠確認
- [ ] 保守性の向上確認

## 7. 実装後のフォローアップ

### 7.1 監視項目
- 権限操作の成功率
- 権限昇格期間の推移
- エラー発生率の監視
- パフォーマンスメトリクス

### 7.2 継続的改善
- ユーザーフィードバックの収集
- セキュリティアップデートの適用
- パフォーマンス最適化の継続
- ドキュメントの継続的更新

この実装計画に基づいて、段階的かつ安全に権限昇格期間の最小化を実現します。
