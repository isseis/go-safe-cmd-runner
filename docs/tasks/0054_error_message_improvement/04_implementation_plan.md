# 実装計画書: エラーメッセージ改善

## 1. 実装概要

### 1.1 実装の目的

エラーメッセージの簡潔化とデバッグ情報の分離により、ユーザーエクスペリエンスを向上させ、トラブルシューティングを効率化する。

### 1.2 実装の範囲

- **Phase 0**: ValidationError インフラストラクチャの構築
- **Phase 1**: ディレクトリパーミッションエラーの改善
- **Phase 2**: ファイルパーミッションエラーの改善（オプション）
- **Phase 3**: コマンド実行エラーのログ改善
- **Phase 4**: 全エラーの監査と一貫性確保（オプション）

### 1.3 実装の前提条件

- Go 1.21 以上（`maps.Clone()`, `maps.Copy()` の使用）
- 既存のテストが全てパスしていること
- Git ブランチ: `issei/error-message-improvement-02`

## 2. Phase 0: ValidationError インフラストラクチャ

### 2.1 目的

ValidationError 型とヘルパー関数を実装し、後続のフェーズで使用できるようにする。

### 2.2 タスク一覧

#### 2.2.1 ディレクトリ作成

```bash
mkdir -p internal/runner/errors
```

#### 2.2.2 ValidationError 型の実装

**ファイル**: `internal/runner/errors/validation_error.go`

**実装内容**:
1. パッケージドキュメント
2. `ValidationError` 構造体の定義
3. `Error()` メソッド
4. `Unwrap()` メソッド
5. `GetChain()` メソッド
6. `GetContext()` メソッド
7. `FormatChain()` メソッド
8. `WrapValidation()` 関数
9. `WrapValidationWithContext()` 関数

**推定工数**: 2時間

**実装詳細**: `03_detailed_specification.md` の「1. ValidationError 型の詳細仕様」を参照

#### 2.2.3 ユニットテストの実装

**ファイル**: `internal/runner/errors/validation_error_test.go`

**テストケース**:

1. **TestValidationError_Basic**: ValidationError の基本機能
   - `Error()` が簡潔なメッセージを返すこと
   - `Unwrap()` が元のエラーを返すこと
   - `errors.Is()` が動作すること

2. **TestWrapValidation**: WrapValidation の動作
   - 新しいエラーを ValidationError でラップできること
   - 既存の ValidationError のチェーンに追加できること
   - チェーンの順序が正しいこと（呼び出し元が先頭）

3. **TestWrapValidationWithContext**: WrapValidationWithContext の動作
   - コンテキスト情報が保持されること
   - コンテキストがマージされること
   - 重複キーが上書きされること
   - nil コンテキストが安全に処理されること

4. **TestValidationError_GetChain**: GetChain のコピー動作
   - 返されたスライスを変更しても内部状態に影響しないこと

5. **TestValidationError_GetContext**: GetContext のコピー動作
   - 返されたマップを変更しても内部状態に影響しないこと

6. **TestValidationError_FormatChain**: FormatChain の出力形式
   - " -> " で結合された文字列が返されること
   - 空のチェーンで空文字列が返されること

7. **TestValidationError_ErrorsIs**: errors.Is() との互換性
   - ValidationError でラップされたエラーが `errors.Is()` で検出できること

8. **TestValidationError_ErrorsAs**: errors.As() との互換性
   - ValidationError でラップされたエラーが `errors.As()` で抽出できること

**推定工数**: 3時間

**実装例**:

```go
package runnererrors

import (
    "errors"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestValidationError_Basic(t *testing.T) {
    baseErr := errors.New("base error")

    ve := &ValidationError{
        Err:   baseErr,
        Chain: []string{"func1"},
        Context: map[string]interface{}{
            "key": "value",
        },
    }

    // Error() は簡潔なメッセージを返す
    assert.Equal(t, "base error", ve.Error())

    // Unwrap() は元のエラーを返す
    assert.Equal(t, baseErr, ve.Unwrap())

    // errors.Is() が動作する
    assert.True(t, errors.Is(ve, baseErr))
}

func TestWrapValidation(t *testing.T) {
    baseErr := errors.New("base error")

    // 最初のラップ
    err1 := WrapValidation(baseErr, "validatePermission")

    var ve1 *ValidationError
    require.True(t, errors.As(err1, &ve1))
    assert.Equal(t, []string{"validatePermission"}, ve1.Chain)

    // 2回目のラップ
    err2 := WrapValidation(err1, "validateSecurity")

    var ve2 *ValidationError
    require.True(t, errors.As(err2, &ve2))
    assert.Equal(t, []string{"validateSecurity", "validatePermission"}, ve2.Chain)

    // Error() は簡潔なまま
    assert.Equal(t, "base error", err2.Error())

    // errors.Is() は引き続き動作
    assert.True(t, errors.Is(err2, baseErr))
}

func TestWrapValidationWithContext(t *testing.T) {
    baseErr := errors.New("base error")

    // 最初のラップ
    err1 := WrapValidationWithContext(baseErr, "validatePermission", map[string]interface{}{
        "path": "/tmp/test",
        "uid":  1000,
    })

    var ve1 *ValidationError
    require.True(t, errors.As(err1, &ve1))
    assert.Equal(t, "/tmp/test", ve1.Context["path"])
    assert.Equal(t, 1000, ve1.Context["uid"])

    // 2回目のラップ（追加のコンテキスト）
    err2 := WrapValidationWithContext(err1, "validateSecurity", map[string]interface{}{
        "permissions": "0775",
    })

    var ve2 *ValidationError
    require.True(t, errors.As(err2, &ve2))
    assert.Equal(t, "/tmp/test", ve2.Context["path"])
    assert.Equal(t, 1000, ve2.Context["uid"])
    assert.Equal(t, "0775", ve2.Context["permissions"])

    // 3回目のラップ（キーの上書き）
    err3 := WrapValidationWithContext(err2, "validateOutputPath", map[string]interface{}{
        "path": "/tmp/new_path",
    })

    var ve3 *ValidationError
    require.True(t, errors.As(err3, &ve3))
    assert.Equal(t, "/tmp/new_path", ve3.Context["path"])  // 上書きされた
    assert.Equal(t, 1000, ve3.Context["uid"])              // 保持されていることを確認
    assert.Equal(t, "0775", ve3.Context["permissions"])    // 保持されていることを確認
}

func TestValidationError_GetChain(t *testing.T) {
    ve := &ValidationError{
        Err:   errors.New("test"),
        Chain: []string{"func1", "func2"},
    }

    // GetChain() でコピーを取得
    chain := ve.GetChain()

    // コピーを変更しても内部状態に影響しない
    chain[0] = "modified"
    assert.Equal(t, "func1", ve.Chain[0])
}

func TestValidationError_GetContext(t *testing.T) {
    ve := &ValidationError{
        Err: errors.New("test"),
        Context: map[string]interface{}{
            "key": "value",
        },
    }

    // GetContext() でコピーを取得
    context := ve.GetContext()

    // コピーを変更しても内部状態に影響しない
    context["key"] = "modified"
    assert.Equal(t, "value", ve.Context["key"])
}

func TestValidationError_FormatChain(t *testing.T) {
    tests := []struct {
        name     string
        chain    []string
        expected string
    }{
        {
            name:     "multiple functions",
            chain:    []string{"func1", "func2", "func3"},
            expected: "func1 -> func2 -> func3",
        },
        {
            name:     "single function",
            chain:    []string{"func1"},
            expected: "func1",
        },
        {
            name:     "empty chain",
            chain:    []string{},
            expected: "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ve := &ValidationError{
                Err:   errors.New("test"),
                Chain: tt.chain,
            }
            assert.Equal(t, tt.expected, ve.FormatChain())
        })
    }
}
```

#### 2.2.4 テスト実行

```bash
# ユニットテストの実行
go test -v ./internal/runner/errors/

# カバレッジの確認
go test -cover ./internal/runner/errors/
```

**期待される結果**:
- 全てのテストがパス
- カバレッジ 90% 以上

**推定工数**: 0.5時間

### 2.3 Phase 0 の成果物

- [ ] `internal/runner/errors/validation_error.go`
- [ ] `internal/runner/errors/validation_error_test.go`
- [ ] 全ユニットテストがパス
- [ ] カバレッジ 90% 以上

### 2.4 Phase 0 の総推定工数

**合計**: 5.5時間

## 3. Phase 1: ディレクトリパーミッションエラーの改善

### 3.1 目的

ディレクトリパーミッション関連のエラーメッセージを簡潔化し、ValidationError でラップする。

### 3.2 影響を受けるファイル

- `internal/runner/security/file_validation.go`
- `internal/runner/security/file_validation_test.go`

### 3.3 タスク一覧

#### 3.3.1 validateGroupWritePermissions() の改善

**ファイル**: `internal/runner/security/file_validation.go`

**行番号**: L236-237

**変更前**:
```go
return fmt.Errorf("%w: directory %s has group write permissions (%04o) but group membership cannot be verified",
    ErrInvalidDirPermissions, dirPath, perm)
```

**変更後**:
```go
return fmt.Errorf("%w: %s has group write permissions (%04o) but group membership cannot be verified",
    ErrInvalidDirPermissions, dirPath, perm)
```

**変更内容**:
- "directory" という冗長な単語を削除（パスから明らか）

**推定工数**: 0.1時間

#### 3.3.2 validateOutputDirectoryAccess() の改善

**ファイル**: `internal/runner/security/file_validation.go`

**行番号**: L311

**変更前**:
```go
return fmt.Errorf("directory security validation failed for %s: %w", currentPath, err)
```

**変更後**:
```go
return runnererrors.WrapValidationWithContext(err,
    "validateOutputDirectoryAccess",
    map[string]interface{}{
        "current_path": currentPath,
    })
```

**変更内容**:
- ValidationError でラップ
- 検証チェーンに関数名を追加
- コンテキスト情報（`current_path`）を追加
- 冗長なメッセージ（"directory security validation failed"）を削除

**推定工数**: 0.2時間

#### 3.3.3 ValidateOutputWritePermission() の改善

**ファイル**: `internal/runner/security/file_validation.go`

**行番号**: L283

**変更前**:
```go
return fmt.Errorf("directory validation failed: %w", err)
```

**変更後**:
```go
return runnererrors.WrapValidationWithContext(err,
    "ValidateOutputWritePermission",
    map[string]interface{}{
        "output_path": outputPath,
        "real_uid": realUID,
    })
```

**変更内容**:
- ValidationError でラップ
- 検証チェーンに関数名を追加
- コンテキスト情報（`output_path`, `real_uid`）を追加
- 冗長なメッセージ（"directory validation failed"）を削除

**推定工数**: 0.2時間

#### 3.3.4 import 文の追加

**ファイル**: `internal/runner/security/file_validation.go`

**追加するimport**:
```go
import (
    // 既存のimport
    // ...

    runnererrors "github.com/isseis/go-safe-cmd-runner/internal/runner/errors"
)
```

**推定工数**: 0.1時間

#### 3.3.5 テストコードの更新

**ファイル**: `internal/runner/security/file_validation_test.go`

**更新が必要なテストケース**:

1. **TestValidateOutputWritePermission_GroupWriteNoMembership**
   - エラーメッセージ文字列のチェックを更新
   - ValidationError の抽出とチェーンの検証を追加

2. **TestValidateDirectoryPermissions_GroupWrite**
   - エラーメッセージ文字列のチェックを更新

3. **その他の関連テスト**
   - エラーメッセージに依存するテストを検索して更新

**更新パターン**:

```go
// Before
func TestValidateOutputWritePermission_GroupWriteNoMembership(t *testing.T) {
    // ...
    err := validator.ValidateOutputWritePermission(testDir, realUID)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "validation failed")
    assert.ErrorIs(t, err, security.ErrInvalidDirPermissions)
}

// After
func TestValidateOutputWritePermission_GroupWriteNoMembership(t *testing.T) {
    // ...
    err := validator.ValidateOutputWritePermission(testDir, realUID)
    require.Error(t, err)

    // センチネルエラーの型チェック
    assert.ErrorIs(t, err, security.ErrInvalidDirPermissions)

    // ValidationError の抽出
    var ve *runnererrors.ValidationError
    require.True(t, errors.As(err, &ve))

    // 検証チェーンの確認
    chain := ve.GetChain()
    assert.Contains(t, chain, "ValidateOutputWritePermission")
    assert.Contains(t, chain, "validateOutputDirectoryAccess")

    // エラーメッセージの確認（冗長な表現が含まれないこと）
    errMsg := err.Error()
    assert.Contains(t, errMsg, "group write permissions")
    assert.NotContains(t, errMsg, "validation failed")

    // コンテキスト情報の確認
    context := ve.GetContext()
    assert.Contains(t, context, "output_path")
    assert.Contains(t, context, "current_path")
}
```

**推定工数**: 2時間

#### 3.3.6 テスト実行

```bash
# file_validation パッケージのテスト実行
go test -v ./internal/runner/security/ -run TestValidateOutputWritePermission

# 全体のテスト実行
go test -v ./internal/runner/security/
```

**期待される結果**:
- 全てのテストがパス
- エラーメッセージが簡潔になっていることを確認

**推定工数**: 0.5時間

### 3.4 Phase 1 の成果物

- [ ] `internal/runner/security/file_validation.go` の更新
- [ ] `internal/runner/security/file_validation_test.go` の更新
- [ ] 全テストがパス
- [ ] エラーメッセージが簡潔化されたことを確認

### 3.5 Phase 1 の総推定工数

**合計**: 3.1時間

## 4. Phase 2: ファイルパーミッションエラーの改善（オプション）

### 4.1 目的

ファイルパーミッション関連のエラーメッセージを簡潔化し、Phase 1 と同様のパターンを適用する。

### 4.2 影響を受けるファイル

- `internal/runner/security/file_validation.go`
- `internal/runner/security/file_validation_test.go`

### 4.3 タスク一覧

#### 4.3.1 ValidateFilePermissions() の改善

**ファイル**: `internal/runner/security/file_validation.go`

**行番号**: L48-86

**変更箇所**:
- エラーメッセージの簡潔化
- ValidationError でのラッピング（必要に応じて）

**推定工数**: 0.5時間

#### 4.3.2 validateOutputFileWritePermission() の改善

**ファイル**: `internal/runner/security/file_validation.go`

**行番号**: L336-349

**変更前**:
```go
return fmt.Errorf("file write permission check failed: %w", err)
```

**変更後**:
```go
return runnererrors.WrapValidation(err, "validateOutputFileWritePermission")
```

**推定工数**: 0.2時間

#### 4.3.3 テストコードの更新

**ファイル**: `internal/runner/security/file_validation_test.go`

**推定工数**: 1時間

#### 4.3.4 テスト実行

```bash
go test -v ./internal/runner/security/
```

**推定工数**: 0.3時間

### 4.4 Phase 2 の成果物

- [ ] ファイルパーミッションエラーの改善
- [ ] 関連テストの更新
- [ ] 全テストがパス

### 4.5 Phase 2 の総推定工数

**合計**: 2時間

**注**: Phase 2 はオプションであり、時間に余裕がある場合のみ実施

## 5. Phase 3: コマンド実行エラーのログ改善

### 5.1 目的

コマンド実行エラーのログ出力を改善し、ERROR レベルで簡潔なメッセージ、DEBUG レベルで詳細情報を記録する。

### 5.2 影響を受けるファイル

- `internal/runner/group_executor.go`
- `internal/runner/group_executor_test.go`

### 5.3 タスク一覧

#### 5.3.1 validateOutputPath() の改善

**ファイル**: `internal/runner/group_executor.go`

**関数**: `validateOutputPath()`

**変更内容**:
- `ValidateOutputWritePermission()` からのエラーを ValidationError でラップ

**変更前**:
```go
func (e *DefaultGroupExecutor) validateOutputPath(outputPath string, cmd *config.Command, realUID int) error {
    if err := e.validator.ValidateOutputWritePermission(outputPath, realUID); err != nil {
        return fmt.Errorf("output path validation failed: %w", err)
    }
    return nil
}
```

**変更後**:
```go
func (e *DefaultGroupExecutor) validateOutputPath(outputPath string, cmd *config.Command, realUID int) error {
    if err := e.validator.ValidateOutputWritePermission(outputPath, realUID); err != nil {
        return runnererrors.WrapValidation(err, "validateOutputPath")
    }
    return nil
}
```

**推定工数**: 0.2時間

#### 5.3.2 executeCommandWithOutputCapture() のログ改善

**ファイル**: `internal/runner/group_executor.go`

**関数**: `executeCommandWithOutputCapture()`

**変更内容**:
- ERROR レベル: 簡潔なメッセージ（`err.Error()`）
- DEBUG レベル: ValidationError を抽出し、Chain と Context を記録

**変更前**:
```go
if err := e.validateOutputPath(...); err != nil {
    slog.Error("Command failed", "command", cmd.Name(), "error", err)
    return err
}
```

**変更後**:
```go
if err := e.validateOutputPath(...); err != nil {
    // ERROR: 簡潔なメッセージ（Error()メソッド）
    slog.Error("Command failed", "command", cmd.Name(), "error", err)

    // DEBUG: 詳細情報（ValidationError の場合のみ）
    var ve *runnererrors.ValidationError
    if errors.As(err, &ve) {
        // 根本エラーを取得（ValidationErrorをアンラップ）
        rootErr := ve.Err

        slog.Debug("Command failed with details",
            "command", cmd.Name(),
            "error_message", err.Error(),           // 簡潔なエラーメッセージ
            "root_error", rootErr,                  // 根本エラー（センチネルエラー）
            "root_error_type", fmt.Sprintf("%T", rootErr),  // 根本エラーの型
            "validation_chain", ve.FormatChain(),   // 検証チェーン
            "context", ve.GetContext())             // コンテキスト情報
    }

    return err
}
```

**推定工数**: 0.5時間

#### 5.3.3 import 文の追加

**ファイル**: `internal/runner/group_executor.go`

**追加するimport**:
```go
import (
    // 既存のimport
    // ...

    runnererrors "github.com/isseis/go-safe-cmd-runner/internal/runner/errors"
)
```

**推定工数**: 0.1時間

#### 5.3.4 テストコードの追加

**ファイル**: `internal/runner/group_executor_test.go`

**新規テストケース**:

```go
func TestExecuteCommandWithOutputCapture_ErrorLogging(t *testing.T) {
    // ログキャプチャの設定
    var logBuffer bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    }))
    slog.SetDefault(logger)

    // エラーが発生するコマンドを実行
    // (グループ書き込みパーミッションを持つディレクトリに出力)
    // ...

    err := executor.executeCommandWithOutputCapture(...)
    require.Error(t, err)

    // ログの解析
    logs := logBuffer.String()

    // ERROR レベル: 簡潔なメッセージ
    assert.Contains(t, logs, `"level":"ERROR"`)
    assert.Contains(t, logs, `"msg":"Command failed"`)
    assert.Contains(t, logs, `"error":"invalid directory permissions`)
    assert.NotContains(t, logs, "validation failed")  // 冗長な表現が含まれないこと

    // DEBUG レベル: 検証チェーンとコンテキスト
    assert.Contains(t, logs, `"level":"DEBUG"`)
    assert.Contains(t, logs, `"msg":"Command failed with details"`)
    assert.Contains(t, logs, `"validation_chain":"validateOutputPath -> ValidateOutputWritePermission`)
    assert.Contains(t, logs, `"context":`)
}
```

**推定工数**: 2時間

#### 5.3.5 統合テストの実行

```bash
# group_executor パッケージのテスト実行
go test -v ./internal/runner/ -run TestExecuteCommandWithOutputCapture

# 全体のテスト実行
go test -v ./internal/runner/
```

**推定工数**: 0.5時間

### 5.4 Phase 3 の成果物

- [ ] `internal/runner/group_executor.go` の更新
- [ ] `internal/runner/group_executor_test.go` の更新
- [ ] ERROR/DEBUG ログの分離が実装されたことを確認
- [ ] 全テストがパス

### 5.5 Phase 3 の総推定工数

**合計**: 3.3時間

## 6. Phase 4: 全エラーの監査と一貫性確保（オプション）

### 6.1 目的

全てのエラーメッセージを監査し、一貫性を確保する。

### 6.2 タスク一覧

#### 6.2.1 エラーメッセージの監査

```bash
# "validation failed" を含むエラーメッセージを検索
grep -rn "validation failed" internal/runner/

# "failed" を含むエラーメッセージを検索
grep -rn 'fmt.Errorf.*failed' internal/runner/

# エラーメッセージの一覧を作成
grep -rn 'fmt.Errorf' internal/runner/ > error_messages.txt
```

**推定工数**: 1時間

#### 6.2.2 改善が必要なエラーの特定

監査結果から、以下の基準で改善が必要なエラーを特定:
- "validation failed" などの冗長表現が含まれる
- エラーメッセージが長すぎる（200文字以上）
- 同じような表現が複数回繰り返される

**推定工数**: 1時間

#### 6.2.3 エラーメッセージの改善

特定されたエラーについて、Phase 1-3 と同様のパターンで改善。

**推定工数**: 2-4時間（エラー数に依存）

#### 6.2.4 テストコードの更新

**推定工数**: 2-3時間（エラー数に依存）

#### 6.2.5 全テストの実行

```bash
# 全体のテスト実行
go test -v ./...

# 統合テストの実行
go test -v ./cmd/runner/
```

**推定工数**: 0.5時間

### 6.3 Phase 4 の成果物

- [ ] 全エラーメッセージの監査レポート
- [ ] 改善されたエラーメッセージ
- [ ] 更新されたテストコード
- [ ] 全テストがパス

### 6.4 Phase 4 の総推定工数

**合計**: 6.5-9.5時間

**注**: Phase 4 はオプションであり、時間に余裕がある場合のみ実施

## 7. 全体のマイルストーン

### 7.1 Milestone 1: 基盤構築（Phase 0）

**目標**: ValidationError インフラストラクチャの完成

**成果物**:
- ValidationError 型とヘルパー関数
- ユニットテスト（カバレッジ 90% 以上）

**期間**: 1日

### 7.2 Milestone 2: 主要エラーの改善（Phase 1）

**目標**: ディレクトリパーミッションエラーの改善

**成果物**:
- 簡潔化されたエラーメッセージ
- ValidationError を使用したエラー伝播
- 更新されたテスト

**期間**: 0.5日

### 7.3 Milestone 3: ログ改善（Phase 3）

**目標**: ERROR/DEBUG ログの分離

**成果物**:
- 簡潔な ERROR ログ
- 詳細な DEBUG ログ
- ログ出力のテスト

**期間**: 0.5日

### 7.4 Milestone 4: 完全性確保（Phase 2, 4）

**目標**: 全エラーの一貫性確保

**成果物**:
- 全エラーメッセージの改善
- 包括的なテストカバレッジ

**期間**: 1-2日（オプション）

## 8. テスト戦略

### 8.1 ユニットテスト

**対象**:
- `internal/runner/errors/validation_error_test.go`

**テストカバレッジ目標**: 90% 以上

**重要なテストケース**:
- ValidationError の基本機能
- WrapValidation() の動作
- WrapValidationWithContext() の動作
- GetChain() / GetContext() のコピー動作
- FormatChain() の出力形式
- errors.Is() / errors.As() との互換性

### 8.2 統合テスト

**対象**:
- `internal/runner/security/file_validation_test.go`
- `internal/runner/group_executor_test.go`

**テストカバレッジ目標**: 既存のカバレッジを維持

**重要なテストケース**:
- エラーの伝播とチェーン構築
- ログ出力（ERROR / DEBUG）
- センチネルエラーの型チェック
- ValidationError の抽出

### 8.3 手動テスト

**シナリオ**:

1. **グループ書き込みパーミッションエラー**
   ```bash
   # テスト用ディレクトリの作成
   mkdir -p /tmp/test-dir
   chmod 775 /tmp/test-dir

   # runner の実行（エラーが発生）
   ./runner --config test_config.toml

   # ERROR ログが簡潔であることを確認
   # DEBUG ログで詳細情報が記録されていることを確認
   ```

2. **所有者不一致エラー**
   ```bash
   # 他のユーザーが所有するディレクトリに出力
   sudo mkdir -p /tmp/other-user-dir
   sudo chown root:root /tmp/other-user-dir

   # runner の実行（エラーが発生）
   ./runner --config test_config.toml

   # エラーメッセージが明確であることを確認
   ```

### 8.4 回帰テスト

**対象**:
- 全てのテストスイート

**実行コマンド**:
```bash
# 全テストの実行
go test -v ./...

# カバレッジの確認
go test -cover ./...

# 統合テストの実行
go test -v ./cmd/runner/
```

**期待される結果**:
- 全てのテストがパス
- カバレッジが維持または向上

## 9. 品質保証

### 9.1 コードレビュー

**レビューポイント**:
- エラーメッセージが簡潔で明確か
- ValidationError が適切に使用されているか
- コンテキスト情報に機密情報が含まれていないか
- ログ出力が適切なレベルで行われているか
- テストカバレッジが十分か

### 9.2 静的解析

```bash
# golangci-lint の実行
golangci-lint run ./...

# go vet の実行
go vet ./...

# go fmt のチェック
go fmt ./...
```

### 9.3 パフォーマンステスト

ValidationError の作成とラッピングがパフォーマンスに影響しないことを確認。

```bash
# ベンチマークテストの実行
go test -bench=. -benchmem ./internal/runner/errors/
```

**期待される結果**:
- ValidationError の作成: < 1μs
- WrapValidation(): < 1μs

## 10. リスク管理

### 10.1 識別されたリスク

| リスク | 確率 | 影響 | 緩和策 |
|--------|------|------|--------|
| テストの失敗 | 中 | 高 | 事前にメッセージ依存テストを特定し、更新計画を作成 |
| パフォーマンス低下 | 低 | 中 | ベンチマークテストで確認 |
| ログ解析スクリプトの影響 | 低 | 低 | 変更内容をドキュメント化 |
| 実装コストの増加 | 中 | 中 | 段階的実装、Phase 2/4 をオプション化 |

### 10.2 緊急時の対応

**ロールバック手順**:
1. 変更をコミット前の状態に戻す
2. 既存のテストが全てパスすることを確認
3. 問題を分析し、修正計画を再作成

## 11. ドキュメント

### 11.1 更新が必要なドキュメント

- [ ] `README.md`: エラーメッセージ改善の概要を追加
- [ ] `docs/dev/error-handling.md`: ValidationError の使用方法を追加（新規作成）
- [ ] `CHANGELOG.md`: 変更内容を記録

### 11.2 新規作成が必要なドキュメント

**ファイル**: `docs/dev/error-handling.md`

**内容**:
- ValidationError の概要
- 使用方法と例
- エラーメッセージのベストプラクティス
- トラブルシューティング

**推定工数**: 1時間

## 12. デプロイ計画

### 12.1 ブランチ戦略

- **開発ブランチ**: `issei/error-message-improvement-02`（既存）
- **ターゲットブランチ**: `main`

### 12.2 マージ手順

1. Phase 0 の実装とテスト
2. Phase 1 の実装とテスト
3. Phase 3 の実装とテスト
4. 全テストの実行
5. コードレビュー
6. プルリクエストの作成
7. CI/CD パイプラインの確認
8. マージ

### 12.3 CI/CD パイプライン

```yaml
# .github/workflows/test.yml (例)
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: go test -v -cover ./...
      - run: golangci-lint run ./...
```

## 13. 総推定工数

| Phase | タスク | 推定工数 | 優先度 |
|-------|--------|----------|--------|
| Phase 0 | ValidationError インフラ | 5.5時間 | 必須 |
| Phase 1 | ディレクトリパーミッションエラー | 3.1時間 | 必須 |
| Phase 3 | コマンド実行エラーログ | 3.3時間 | 必須 |
| Phase 2 | ファイルパーミッションエラー | 2時間 | オプション |
| Phase 4 | 全エラーの監査 | 6.5-9.5時間 | オプション |
| ドキュメント | 各種ドキュメント更新 | 1時間 | 必須 |
| **合計（必須のみ）** | | **12.9時間** | |
| **合計（全て含む）** | | **21.4-24.4時間** | |

**推奨実装範囲**: Phase 0, 1, 3 + ドキュメント（12.9時間 ≈ 2日）

## 14. スケジュール例

### 14.1 2日間での実装（必須部分のみ）

**Day 1**:
- 午前: Phase 0（ValidationError インフラ）
- 午後: Phase 1（ディレクトリパーミッションエラー）

**Day 2**:
- 午前: Phase 3（コマンド実行エラーログ）
- 午後: テスト、ドキュメント更新、コードレビュー準備

### 14.2 3-4日間での実装（全て含む）

**Day 1**: Phase 0
**Day 2**: Phase 1
**Day 3**: Phase 3
**Day 4**: Phase 2, Phase 4（部分的）、ドキュメント更新

## 15. 成功基準

### 15.1 技術的基準

- [ ] 全ユニットテストがパス（Phase 0）
- [ ] 全統合テストがパス（Phase 1, 3）
- [ ] エラーメッセージの平均長が50%以上削減
- [ ] "validation failed" などの冗長表現が削除
- [ ] DEBUG ログで検証チェーンが利用可能
- [ ] `errors.Is()` / `errors.As()` との互換性が保たれる
- [ ] カバレッジが維持または向上

### 15.2 品質基準

- [ ] コードレビュー完了
- [ ] 静的解析ツールのエラーなし
- [ ] ベンチマークテストで性能低下なし
- [ ] ドキュメントが更新されている

### 15.3 ユーザーエクスペリエンス基準

- [ ] エラーメッセージが即座に理解できる
- [ ] トラブルシューティング時に必要な情報が DEBUG ログで得られる
- [ ] エラーログが見やすい

## 16. 参照

- アーキテクチャ設計書: `02_architecture.md`
- 詳細仕様書: `03_detailed_specification.md`
- 要件定義書: `01_requirements.md`
