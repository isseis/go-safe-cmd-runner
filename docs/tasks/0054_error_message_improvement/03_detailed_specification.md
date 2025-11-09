# 詳細仕様書: エラーメッセージ改善

## 1. ValidationError 型の詳細仕様

### 1.1 パッケージ構成

```
internal/
└── runner/
    └── errors/
        ├── validation_error.go       # ValidationError 型とヘルパー関数
        └── validation_error_test.go  # ユニットテスト
```

**パッケージ名**: `runnererrors`

**理由**:
- `errors` パッケージ名は標準ライブラリと衝突する
- `runnererrors` は `internal/runner` 配下のエラー関連機能を明示
- 短く覚えやすい

### 1.2 ValidationError 型の定義

```go
// ValidationError は検証エラーを表すカスタムエラー型
//
// Error() メソッドは簡潔なメッセージのみを返し、
// 検証チェーン情報は Chain フィールドに保持する。
// これにより、エラーログを簡潔に保ちながら、
// デバッグ時には詳細な情報にアクセスできる。
//
// 使用例:
//   err := &ValidationError{
//       Err:   ErrInvalidPermissions,
//       Chain: []string{"ValidateOutputWritePermission"},
//       Context: map[string]interface{}{"path": "/tmp/data"},
//   }
type ValidationError struct {
    // Err は元のエラー（センチネルエラーまたは基底エラー）
    // このフィールドは nil であってはならない
    Err error

    // Chain は検証チェーン（関数呼び出し経路）
    // 順序: 呼び出し元（上位の関数）が先頭、呼び出し先（下位の関数）が末尾
    // 例: ["validateOutputPath", "ValidateOutputWritePermission", "validateOutputDirectoryAccess"]
    //
    // 空のスライスの場合は、チェーン情報が記録されていないことを示す
    Chain []string

    // Context は追加のコンテキスト情報（オプション）
    // 例: {"path": "/tmp/data", "permissions": "0775", "real_uid": 1000}
    //
    // nil の場合は、コンテキスト情報が記録されていないことを示す
    // 空のマップ（make(map[string]interface{})）の場合は、
    // コンテキストは初期化されているが、現時点で追加情報がないことを示す
    Context map[string]interface{}
}
```

### 1.3 ValidationError のメソッド

#### 1.3.1 Error() メソッド

```go
// Error は簡潔なエラーメッセージを返す
// 検証チェーン情報は含めない（DEBUGログで別途表示）
//
// 返り値は Err フィールドの Error() メソッドの結果
// これにより、エラーの本質的な情報のみがログに記録される
//
// 例:
//   ve := &ValidationError{
//       Err: fmt.Errorf("invalid permissions: %s has group write", path),
//       Chain: []string{"ValidateOutputWritePermission", "validateOutputDirectoryAccess"},
//   }
//   ve.Error() // => "invalid permissions: /tmp/data has group write"
func (e *ValidationError) Error() string {
    return e.Err.Error()
}
```

#### 1.3.2 Unwrap() メソッド

```go
// Unwrap は errors.Is() / errors.As() との互換性のためにラップされたエラーを返す
//
// このメソッドにより、既存のエラーハンドリングコード（errors.Is()）が
// ValidationError でラップされたエラーに対しても正常に動作する
//
// 例:
//   ve := &ValidationError{Err: ErrInvalidDirPermissions, ...}
//   errors.Is(ve, ErrInvalidDirPermissions) // => true
func (e *ValidationError) Unwrap() error {
    return e.Err
}
```

#### 1.3.3 GetChain() メソッド

```go
// GetChain は検証チェーンを返す
// 内部状態を保護するため、スライスのコピーを返す
//
// 返り値:
//   検証チェーンの複製（呼び出し元が変更しても内部状態に影響しない）
//   Chain が nil の場合は空のスライスを返す
//
// 例:
//   chain := ve.GetChain()
//   // chain を変更しても ve.Chain には影響しない
func (e *ValidationError) GetChain() []string {
    if e.Chain == nil {
        return []string{}
    }
    chain := make([]string, len(e.Chain))
    copy(chain, e.Chain)
    return chain
}
```

#### 1.3.4 GetContext() メソッド

```go
// GetContext はコンテキスト情報を返す
// 内部状態を保護するため、マップのコピーを返す（浅いコピー）
//
// 返り値:
//   コンテキスト情報の複製（呼び出し元が変更しても内部状態に影響しない）
//   Context が nil の場合は空のマップを返す
//
// 注意:
//   浅いコピーのため、値が参照型（スライス、マップなど）の場合は
//   その参照がコピーされる。通常のユースケースでは問題ない。
//
// 例:
//   context := ve.GetContext()
//   // context を変更しても ve.Context には影響しない
func (e *ValidationError) GetContext() map[string]interface{} {
    return maps.Clone(e.Context)
}
```

#### 1.3.5 FormatChain() メソッド

```go
// FormatChain は検証チェーンを文字列として整形
// DEBUGログで使用することを想定
//
// 返り値:
//   検証チェーンを " -> " で結合した文字列
//   Chain が空の場合は空文字列を返す
//
// 例:
//   ve := &ValidationError{
//       Chain: []string{"validateOutputPath", "ValidateOutputWritePermission", "validateOutputDirectoryAccess"},
//   }
//   ve.FormatChain() // => "validateOutputPath -> ValidateOutputWritePermission -> validateOutputDirectoryAccess"
func (e *ValidationError) FormatChain() string {
    if len(e.Chain) == 0 {
        return ""
    }
    return strings.Join(e.Chain, " -> ")
}
```

### 1.4 ヘルパー関数の詳細仕様

#### 1.4.1 WrapValidation() 関数

```go
// WrapValidation は ValidationError でエラーをラップする
// 既に ValidationError の場合は、チェーンに関数名を追加する
//
// パラメータ:
//   err: ラップするエラー（nil の場合は nil を返す）
//   functionName: 検証チェーンに追加する関数名
//
// 返り値:
//   ValidationError またはラップされたエラー
//   err が nil の場合は nil
//
// チェーンの順序:
//   呼び出し元（上位の関数）が先頭、呼び出し先（下位の関数）が末尾
//   例: [validateOutputPath, ValidateOutputWritePermission, validateOutputDirectoryAccess, ...]
//
// 既存の ValidationError の処理:
//   - Err フィールドは保持される（元のエラーを維持）
//   - Chain の先頭に functionName が追加される
//   - Context は保持される
//
// 新しいエラーの処理:
//   - Err に err が設定される
//   - Chain に functionName のみが含まれる
//   - Context は空のマップで初期化される（nil を避ける）
//
// 使用例:
//   func validateSecurity(path string) error {
//       if err := checkPermissions(path); err != nil {
//           return WrapValidation(err, "validateSecurity")
//       }
//       return nil
//   }
func WrapValidation(err error, functionName string) error {
    if err == nil {
        return nil
    }

    var ve *ValidationError
    if errors.As(err, &ve) {
        // 既に ValidationError の場合は、チェーンの先頭に追加
        // これにより呼び出し元が常にチェーンの先頭に来る
        newChain := make([]string, 0, len(ve.Chain)+1)
        newChain = append(newChain, functionName)
        newChain = append(newChain, ve.Chain...)

        return &ValidationError{
            Err:     ve.Err,
            Chain:   newChain,
            Context: ve.Context,
        }
    }

    // 新しい ValidationError を作成
    // Context は空のマップで初期化（nil を避ける）
    return &ValidationError{
        Err:     err,
        Chain:   []string{functionName},
        Context: make(map[string]interface{}),
    }
}
```

#### 1.4.2 WrapValidationWithContext() 関数

```go
// WrapValidationWithContext はコンテキスト情報を含めて ValidationError でラップする
//
// パラメータ:
//   err: ラップするエラー（nil の場合は nil を返す）
//   functionName: 検証チェーンに追加する関数名
//   context: 追加するコンテキスト情報（nil の場合も安全に処理される）
//
// 返り値:
//   ValidationError またはラップされたエラー
//   err が nil の場合は nil
//
// チェーンの順序:
//   WrapValidation と同様に、呼び出し元が先頭
//
// コンテキストのマージ:
//   - 既存の ValidationError の場合、既存のコンテキストと新しいコンテキストがマージされる
//   - 重複したキーがある場合は、新しい値で上書きされる
//   - context パラメータが nil の場合も安全に処理される
//
// nil コンテキストの処理:
//   - context パラメータが nil の場合、既存のコンテキストのみが保持される
//   - 新しいエラーで context が nil の場合、空のマップで初期化される
//
// 使用例:
//   func validateOutputDirectoryAccess(dirPath string, realUID int) error {
//       if err := validateCompletePath(dirPath, dirPath, realUID); err != nil {
//           return WrapValidationWithContext(err,
//               "validateOutputDirectoryAccess",
//               map[string]interface{}{
//                   "current_path": dirPath,
//                   "real_uid": realUID,
//               })
//       }
//       return nil
//   }
func WrapValidationWithContext(err error, functionName string, context map[string]interface{}) error {
    if err == nil {
        return nil
    }

    var ve *ValidationError
    if errors.As(err, &ve) {
        // 既に ValidationError の場合は、チェーンの先頭に追加
        newChain := make([]string, 0, len(ve.Chain)+1)
        newChain = append(newChain, functionName)
        newChain = append(newChain, ve.Chain...)

        // コンテキストをマージ（重複キーは新しい値で上書き）
        // maps.Clone は nil マップを安全に処理し、空のマップを返す
        newContext := maps.Clone(ve.Context)
        // maps.Copy は nil マップも安全に処理（何もコピーしない）
        maps.Copy(newContext, context)

        return &ValidationError{
            Err:     ve.Err,
            Chain:   newChain,
            Context: newContext,
        }
    }

    // 新しい ValidationError を作成
    // maps.Clone は nil マップを安全に処理し、空のマップを返す
    return &ValidationError{
        Err:     err,
        Chain:   []string{functionName},
        Context: maps.Clone(context),
    }
}
```

## 2. エラーメッセージフォーマットの詳細仕様

### 2.1 ERROR レベルのメッセージフォーマット

#### 2.1.1 基本構造

```
<エラー型>: <影響を受けるリソース> <問題の具体的内容> <根本原因>
```

**各要素の説明**:

| 要素 | 説明 | 必須 | 例 |
|------|------|------|-----|
| エラー型 | センチネルエラーのメッセージ部分 | はい | `invalid directory permissions` |
| リソース | 問題が発生したパス、コマンド名など | はい | `/tmp/data` |
| 具体的内容 | 何が問題なのか（パーミッション値など） | はい | `has group write permissions (0775)` |
| 根本原因 | なぜ問題なのか | 状況依存 | `but group membership cannot be verified` |

#### 2.1.2 ディレクトリパーミッションエラー

**現状**:
```
output path validation failed: security validation failed: directory validation failed: directory security validation failed for /tmp/scr-mattermost_backup-4288425963/data: invalid directory permissions: directory /tmp/scr-mattermost_backup-4288425963/data has group write permissions (0775) but group membership cannot be verified
```

**改善後**:
```
invalid directory permissions: /tmp/scr-mattermost_backup-4288425963/data has group write permissions (0775) but group membership cannot be verified
```

**実装箇所**: `internal/runner/security/file_validation.go`
- `validateGroupWritePermissions()`: L236-237

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

**削減される要素**:
- "directory" という冗長な単語（パスから明らか）

#### 2.1.3 ファイルパーミッションエラー

**改善前の例**:
```
file write permission check failed: invalid file permissions: file /tmp/data/output.log has group write permissions (0664) but group membership cannot be verified
```

**改善後**:
```
invalid file permissions: /tmp/data/output.log has group write permissions (0664) but group membership cannot be verified
```

**実装箇所**: `internal/runner/security/file_validation.go`
- `validateOutputFileWritePermission()`: L340-341

#### 2.1.4 所有者不一致エラー

**改善前の例**:
```
directory validation failed: invalid directory permissions: directory /tmp/data is owned by UID 1001 but execution user is UID 1000
```

**改善後**:
```
invalid directory permissions: /tmp/data is owned by UID 1001 but execution user is UID 1000
```

**実装箇所**: `internal/runner/security/file_validation.go`
- `validateDirectoryComponentPermissions()`: L214-215

### 2.2 DEBUG レベルのメッセージフォーマット

#### 2.2.1 構造化ログ形式

slogの属性として以下の情報を記録:

```go
slog.Debug("Command failed with details",
    "command", cmd.Name(),                      // コマンド名
    "error_message", err.Error(),               // 簡潔なエラーメッセージ
    "root_error", rootErr,                      // 根本エラー（センチネルエラー）
    "root_error_type", fmt.Sprintf("%T", rootErr),  // 根本エラーの型
    "validation_chain", ve.FormatChain(),       // 検証チェーン
    "context", ve.GetContext())                 // コンテキスト情報
```

**出力例**:
```
[DEBUG] Command failed with details command=dump_db error_message="invalid directory permissions: /tmp/data has group write permissions (0775) but group membership cannot be verified" root_error="invalid directory permissions" root_error_type=*errors.errorString validation_chain="validateOutputPath -> ValidateOutputWritePermission -> validateOutputDirectoryAccess -> validateCompletePath -> validateDirectoryComponentPermissions -> validateGroupWritePermissions" context=map[current_path:/tmp/data output_path:/tmp/data/output.log real_uid:1000]
```

#### 2.2.2 各フィールドの詳細

| フィールド名 | 型 | 説明 | 例 |
|-------------|-----|------|-----|
| `command` | string | 実行されたコマンド名 | `dump_db` |
| `error_message` | string | `Error()` メソッドの出力 | `invalid directory permissions: /tmp/data has group write permissions (0775) but group membership cannot be verified` |
| `root_error` | error | `ve.Err` の文字列表現 | `invalid directory permissions` |
| `root_error_type` | string | `fmt.Sprintf("%T", ve.Err)` の結果 | `*errors.errorString` |
| `validation_chain` | string | `ve.FormatChain()` の結果 | `validateOutputPath -> ValidateOutputWritePermission -> validateOutputDirectoryAccess` |
| `context` | map | `ve.GetContext()` の結果 | `map[current_path:/tmp/data output_path:/tmp/data/output.log real_uid:1000]` |

#### 2.2.3 実装パターン

```go
func (e *DefaultGroupExecutor) executeCommandWithOutputCapture(...) error {
    if err := e.validateOutputPath(...); err != nil {
        // ERROR: 簡潔なメッセージ（Error()メソッド）
        slog.Error("Command failed", "command", cmd.Name(), "error", err)

        // DEBUG: 詳細情報（ValidationError の場合のみ）
        var ve *runnererrors.ValidationError
        if errors.As(err, &ve) {
            // 根本エラーを取得（ValidationErrorをアンラップ）
            // 根本エラーを取得（ValidationErrorをアンラップ）
            rootErr := ve.Err
            sentinelErr := errors.Unwrap(rootErr)
            if sentinelErr == nil {
                sentinelErr = rootErr // ラップされていない場合へのフォールバック
            }

            slog.Debug("Command failed with details",
                "command", cmd.Name(),
                "error_message", err.Error(),           // 簡潔なエラーメッセージ
                "root_error", sentinelErr,                  // 根本エラー（センチネルエラー）
                "root_error_type", fmt.Sprintf("%T", sentinelErr),  // 根本エラーの型
                "validation_chain", ve.FormatChain(),   // 検証チェーン
                "context", ve.GetContext())             // コンテキスト情報

        return err
    }
    // ...
}
```

### 2.3 メッセージ品質基準

#### 2.3.1 簡潔性

- **最大長**: 200文字以内（目安）
- **1行**: 改行を含まない
- **冗長表現の回避**: "failed", "error", "validation" などの繰り返しを避ける

#### 2.3.2 明確性

- **問題の本質**: 何が問題なのかを明示
- **影響範囲**: どのリソースが影響を受けるか
- **根本原因**: なぜ問題なのか

#### 2.3.3 実用性

- **パス情報**: 絶対パスを含める
- **数値情報**: パーミッション値、UID/GIDなど
- **条件情報**: 何ができない/期待されるかを明示

## 3. コンテキスト情報の仕様

### 3.1 コンテキスト情報の定義

コンテキスト情報は `map[string]interface{}` 型で、検証過程で得られた追加情報を保持する。

**目的**:
- デバッグ時の詳細情報提供
- エラーの発生経路の追跡
- トラブルシューティングの効率化

### 3.2 コンテキスト情報に含めるべきデータ

#### 3.2.1 必須情報

| キー名 | 型 | 説明 | 例 |
|--------|-----|------|-----|
| `path` または `current_path` | string | 検証対象のパス | `/tmp/data` |
| `output_path` | string | 出力ファイルのパス（該当する場合） | `/tmp/data/output.log` |

#### 3.2.2 オプション情報

| キー名 | 型 | 説明 | 例 |
|--------|-----|------|-----|
| `real_uid` | int | 実行ユーザーのUID | `1000` |
| `permissions` | string | パーミッション値（8進数文字列） | `"0775"` |
| `owner_uid` | uint32 | 所有者のUID | `1001` |
| `owner_gid` | uint32 | 所有者のGID | `1001` |
| `command` | string | 実行されたコマンド名 | `dump_db` |

#### 3.2.3 含めるべきでない情報

- **機密情報**: パスワード、APIキー、トークンなど
- **冗長情報**: 既にエラーメッセージに含まれている情報
- **大きすぎるデータ**: ログサイズを肥大化させるもの

### 3.3 コンテキスト情報の追加パターン

#### 3.3.1 検証関数でのコンテキスト追加

```go
// validateOutputDirectoryAccess 内
if err := v.validateCompletePath(currentPath, currentPath, realUID); err != nil {
    return runnererrors.WrapValidationWithContext(err,
        "validateOutputDirectoryAccess",
        map[string]interface{}{
            "current_path": currentPath,
        })
}
```

#### 3.3.2 最上位でのコンテキスト追加

```go
// ValidateOutputWritePermission 内
if err := v.validateOutputDirectoryAccess(dir, realUID); err != nil {
    return runnererrors.WrapValidationWithContext(err,
        "ValidateOutputWritePermission",
        map[string]interface{}{
            "output_path": outputPath,
            "real_uid": realUID,
        })
}
```

#### 3.3.3 コンテキストのマージ動作

既存のコンテキストと新しいコンテキストがマージされる際の動作:

```go
// 下位の関数
err1 := WrapValidationWithContext(baseErr, "func1", map[string]interface{}{
    "path": "/tmp",
    "uid": 1000,
})

// 上位の関数（キー "path" を上書き）
err2 := WrapValidationWithContext(err1, "func2", map[string]interface{}{
    "path": "/tmp/data",  // 既存の "path" を上書き
    "permissions": "0775",
})

// 結果:
// Context = {
//   "path": "/tmp/data",      // 上書きされた
//   "uid": 1000,              // 保持された
//   "permissions": "0775",    // 追加された
// }
```

### 3.4 コンテキスト情報のアクセス

#### 3.4.1 GetContext() メソッド

```go
var ve *runnererrors.ValidationError
if errors.As(err, &ve) {
    context := ve.GetContext()

    // キーが存在するかチェック
    if path, ok := context["path"].(string); ok {
        // path を使用
    }

    // 型アサーション
    if realUID, ok := context["real_uid"].(int); ok {
        // realUID を使用
    }
}
```

#### 3.4.2 ログ出力での使用

```go
slog.Debug("Command failed with details",
    "command", cmd.Name(),
    "validation_chain", ve.FormatChain(),
    "context", ve.GetContext())  // マップ全体をログに出力
```

## 4. エラー生成と伝播のフロー

### 4.1 エラー生成の3層構造

```
┌─────────────────────────────────────────────────────────────────┐
│ 最上層（ログ出力層）                                            │
│ - group_executor.go                                              │
│ - ERROR ログ: err.Error()                                        │
│ - DEBUG ログ: ValidationError 抽出 + Chain + Context             │
└─────────────────────────────────────────────────────────────────┘
                               ↑
                    WrapValidation()
                               |
┌─────────────────────────────────────────────────────────────────┐
│ 中間層（検証関数）                                               │
│ - ValidateOutputWritePermission()                                │
│ - validateOutputDirectoryAccess()                                │
│ - WrapValidation() / WrapValidationWithContext()                 │
└─────────────────────────────────────────────────────────────────┘
                               ↑
                    return error
                               |
┌─────────────────────────────────────────────────────────────────┐
│ 最下層（エラー発生層）                                           │
│ - validateGroupWritePermissions()                                │
│ - validateDirectoryComponentPermissions()                        │
│ - 簡潔なエラーメッセージ生成（fmt.Errorf）                      │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 各層の責務

#### 4.2.1 最下層（エラー発生層）

**責務**:
- 問題を検出
- センチネルエラーと簡潔なメッセージを含むエラーを生成
- ValidationError は作成しない

**実装パターン**:
```go
func validateGroupWritePermissions(...) error {
    // 問題を検出
    if groupWriteDetected && !canVerifyMembership {
        // 簡潔なメッセージでエラーを返す
        return fmt.Errorf("%w: %s has group write permissions (%04o) but group membership cannot be verified",
            ErrInvalidDirPermissions, dirPath, perm)
    }
    return nil
}
```

#### 4.2.2 中間層（検証関数）

**責務**:
- 下位の関数を呼び出し
- エラーを ValidationError でラップ
- 検証チェーンに自身の関数名を追加
- 必要に応じてコンテキスト情報を追加

**実装パターン**:
```go
func validateOutputDirectoryAccess(dirPath string, realUID int) error {
    if err := v.validateCompletePath(currentPath, currentPath, realUID); err != nil {
        // ValidationError でラップし、チェーンに追加
        return runnererrors.WrapValidationWithContext(err,
            "validateOutputDirectoryAccess",
            map[string]interface{}{
                "current_path": currentPath,
            })
    }
    return nil
}
```

#### 4.2.3 最上層（ログ出力層）

**責務**:
- エラーをログに記録
- ERROR レベル: `err.Error()` で簡潔なメッセージ
- DEBUG レベル: ValidationError を抽出し、Chain と Context を記録

**実装パターン**:
```go
func (e *DefaultGroupExecutor) executeCommandWithOutputCapture(...) error {
    if err := e.validateOutputPath(...); err != nil {
        // ERROR: 簡潔なメッセージ
        slog.Error("Command failed", "command", cmd.Name(), "error", err)

        // DEBUG: 詳細情報
        var ve *runnererrors.ValidationError
        if errors.As(err, &ve) {
            rootErr := ve.Err
            slog.Debug("Command failed with details",
                "command", cmd.Name(),
                "error_message", err.Error(),
                "root_error", rootErr,
                "root_error_type", fmt.Sprintf("%T", rootErr),
                "validation_chain", ve.FormatChain(),
                "context", ve.GetContext())
        }

        return err
    }
    return nil
}
```

### 4.3 エラー伝播の具体例

```
validateGroupWritePermissions()
  └─> エラー生成: "invalid directory permissions: /tmp/data has group write permissions (0775) but group membership cannot be verified"
        ↓
validateCompletePath()
  └─> そのまま返す（ラッピングしない場合）
        ↓
validateOutputDirectoryAccess()
  └─> WrapValidationWithContext("validateOutputDirectoryAccess", {"current_path": "/tmp/data"})
        ValidationError {
          Err: "invalid directory permissions: ..."
          Chain: ["validateOutputDirectoryAccess"]
          Context: {"current_path": "/tmp/data"}
        }
        ↓
ValidateOutputWritePermission()
  └─> WrapValidationWithContext("ValidateOutputWritePermission", {"output_path": "/tmp/data/output.log", "real_uid": 1000})
        ValidationError {
          Err: "invalid directory permissions: ..."
          Chain: ["ValidateOutputWritePermission", "validateOutputDirectoryAccess"]
          Context: {"current_path": "/tmp/data", "output_path": "/tmp/data/output.log", "real_uid": 1000}
        }
        ↓
validateOutputPath()
  └─> WrapValidation("validateOutputPath")
        ValidationError {
          Err: "invalid directory permissions: ..."
          Chain: ["validateOutputPath", "ValidateOutputWritePermission", "validateOutputDirectoryAccess"]
          Context: {"current_path": "/tmp/data", "output_path": "/tmp/data/output.log", "real_uid": 1000}
        }
        ↓
executeCommandWithOutputCapture()
  └─> ERROR ログ: "Command failed error=invalid directory permissions: /tmp/data has group write permissions (0775) but group membership cannot be verified command=dump_db"
  └─> DEBUG ログ: "Command failed with details command=dump_db error_message=... root_error=... validation_chain=validateOutputPath -> ValidateOutputWritePermission -> validateOutputDirectoryAccess context=..."
```

## 5. 既存コードとの互換性

### 5.1 errors.Is() との互換性

ValidationError は `Unwrap()` メソッドを実装しているため、既存の `errors.Is()` による型チェックは引き続き動作する。

**例**:
```go
// 既存コード（影響なし）
if errors.Is(err, security.ErrInvalidDirPermissions) {
    // この条件は ValidationError でラップされたエラーに対しても true になる
}
```

### 5.2 errors.As() との互換性

ValidationError は `Unwrap()` メソッドを実装しているため、既存の `errors.As()` も正常に動作する。

**例**:
```go
// 既存コード（影響なし）
var permErr *security.PermissionError
if errors.As(err, &permErr) {
    // ValidationError 内にラップされたエラーも抽出できる
}
```

### 5.3 新しいエラー情報へのアクセス

ValidationError 固有の情報（Chain, Context）にアクセスする場合は、新しいコードを追加する必要がある。

**例**:
```go
// 新しいコード（ValidationError の活用）
var ve *runnererrors.ValidationError
if errors.As(err, &ve) {
    // 検証チェーンとコンテキストにアクセス可能
    chain := ve.FormatChain()
    context := ve.GetContext()
}
```

### 5.4 センチネルエラーの維持

既存のセンチネルエラー（`ErrInvalidDirPermissions` など）は変更せず、そのまま使用する。

```go
// 変更なし
var (
    ErrInvalidDirPermissions  = errors.New("invalid directory permissions")
    ErrInvalidFilePermissions = errors.New("invalid file permissions")
    ErrInvalidPath            = errors.New("invalid path")
    // ...
)
```

## 6. パフォーマンス考慮事項

### 6.1 ValidationError 作成のコスト

- **構造体の割り当て**: 軽量（数十バイト）
- **スライスの作成**: Chain は通常数個の要素（数十バイト）
- **マップの作成**: Context は通常数個のキー（数百バイト）

**結論**: エラーパスでのみ実行されるため、パフォーマンスへの影響は無視できる。

### 6.2 GetChain() / GetContext() のコスト

- **スライスのコピー**: O(n) where n = len(Chain)
- **マップのコピー**: O(m) where m = len(Context)

**結論**: Chain と Context は小さいため、コピーコストは無視できる。

### 6.3 FormatChain() のコスト

- **文字列結合**: `strings.Join()` は効率的に実装されている
- **呼び出し頻度**: DEBUGログでのみ使用される

**結論**: DEBUGログが無効の場合は呼び出されないため、影響なし。

## 7. セキュリティ考慮事項

### 7.1 機密情報の漏洩防止

Context に機密情報を含めないようにする。

**避けるべき情報**:
- パスワード
- APIキー
- トークン
- 暗号化キー

### 7.2 ログサイズの制限

Context に大きすぎるデータを含めないようにする。

**推奨サイズ**:
- 1つのキーの値: 最大1KB
- Context 全体: 最大10KB

### 7.3 パス情報のサニタイズ

絶対パスをログに記録する場合、ユーザーのホームディレクトリなどの機密情報が含まれる可能性がある。必要に応じてサニタイズを検討。

**例**:
```go
// ホームディレクトリを ~ に置換
sanitizedPath := strings.Replace(path, os.Getenv("HOME"), "~", 1)
```

## 8. テスト仕様

### 8.1 ユニットテストの範囲

- ValidationError の基本機能
- WrapValidation() の動作
- WrapValidationWithContext() の動作
- GetChain() / GetContext() のコピー動作
- FormatChain() の出力形式

### 8.2 統合テストの範囲

- エラーの伝播とチェーン構築
- ログ出力（ERROR / DEBUG）
- errors.Is() / errors.As() との互換性

### 8.3 テストケースの例

詳細は `04_implementation_plan.md` の「テスト計画」セクションを参照。

## 9. 拡張性

### 9.1 将来的な拡張ポイント

#### 9.1.1 Suggestion フィールドの追加

```go
type ValidationError struct {
    Err        error
    Chain      []string
    Context    map[string]interface{}
    Suggestion string  // ユーザーへの対処法提案
}
```

#### 9.1.2 国際化（i18n）対応

```go
type LocalizableValidationError struct {
    *ValidationError
    MessageKey string
    Params     map[string]interface{}
}
```

#### 9.1.3 エラーカタログ

```go
type ErrorCatalog struct {
    Code           string
    Category       string
    MessagePattern string
    Actions        []string
}
```

### 9.2 後方互換性の保証

将来の拡張時も、既存の ValidationError インターフェース（`Error()`, `Unwrap()`）は維持する。

## 10. ドキュメント

### 10.1 パッケージドキュメント

`internal/runner/errors/validation_error.go` にパッケージレベルのドキュメントを追加。

```go
// Package runnererrors provides error types and utilities for the runner package.
//
// ValidationError is a custom error type that wraps validation errors
// with a validation chain and context information.
// This allows for concise error messages while preserving detailed
// debugging information.
//
// Example usage:
//   func validate(path string) error {
//       if err := checkPermissions(path); err != nil {
//           return runnererrors.WrapValidation(err, "validate")
//       }
//       return nil
//   }
package runnererrors
```

### 10.2 関数ドキュメント

各関数に詳細なGoDocコメントを追加（上記の仕様に記載）。

### 10.3 使用例

README または開発者向けドキュメントに使用例を追加。

## 11. 参照

- アーキテクチャ設計書: `02_architecture.md`
- 実装計画書: `04_implementation_plan.md`
- 要件定義書: `01_requirements.md`
