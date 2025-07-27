# 型安全なパス検証システム - アーキテクチャ設計書

## 1. システム概要

型安全なパス検証システムは、Go言語の型システムを活用して、ファイルパスの検証状態をコンパイル時に強制するシステムです。未検証のパスが安全な操作に使用されることを防ぎ、コードの安全性と保守性を向上させます。

## 2. アーキテクチャ原則

### 2.1 型安全性第一
- コンパイル時にセキュリティ違反を検出
- 実行時エラーを最小化
- 明示的な検証プロセス

### 2.2 段階的移行
- 既存コードとの互換性維持
- 漸進的な型安全化
- 後方互換APIの提供

### 2.3 ゼロコスト抽象化
- 実行時オーバーヘッドの最小化
- メモリ効率の維持
- インライン化の活用

## 3. システム構成

### 3.1 レイヤー構造

```
┌─────────────────────────────────────┐
│         Application Layer           │  アプリケーション固有のロジック
├─────────────────────────────────────┤
│       Type-Safe Path API Layer     │  型安全なパスAPI
├─────────────────────────────────────┤
│      Path Validation Layer         │  パス検証ロジック
├─────────────────────────────────────┤
│       Safe File I/O Layer          │  安全なファイル操作
├─────────────────────────────────────┤
│        Operating System            │  OS ファイルシステム
└─────────────────────────────────────┘
```

### 3.2 パッケージ構成

```
internal/
├── safepath/                    # 新しい型安全パスパッケージ
│   ├── types.go                # 型定義
│   ├── validation.go           # 検証ロジック
│   ├── operations.go           # パス操作
│   └── conversion.go           # 既存コードとの互換性
├── filevalidator/              # 既存パッケージ（型安全化）
│   ├── validator.go           # 型安全化されたValidator
│   └── privileged_validator.go # 型安全化されたPrivilegedValidator
└── safefileio/                # 既存パッケージ（型安全化）
    ├── safe_fileio.go         # 型安全化されたファイル操作
    └── legacy.go              # 後方互換API
```

## 4. 型設計

### 4.1 コア型定義

```go
// ValidatedPath represents a file path that has been validated
type ValidatedPath struct {
    path string
}

// PathValidationError represents validation errors
type PathValidationError struct {
    Path    string
    Reason  ValidationReason
    Message string
}

// ValidationReason enumeration
type ValidationReason int

const (
    ReasonEmpty ValidationReason = iota
    ReasonInvalidCharacters
    ReasonNotAbsolute
    ReasonSymlinkLoop
    ReasonNotRegularFile
    ReasonPermissionDenied
)
```

### 4.2 型の不変条件

- `ValidatedPath` は常に検証済みの絶対パスを保持
- 外部からの直接構築は不可能
- `String()` メソッドは検証済みパスを返す
- ゼロ値は有効な空の状態を表す

### 4.3 型の階層構造

```
ValidatedPath
├── RegularFilePath      # 通常ファイルの検証済みパス
├── DirectoryPath        # ディレクトリの検証済みパス
└── ExecutablePath       # 実行可能ファイルの検証済みパス
```

## 5. コンポーネント設計

### 5.1 パス検証コンポーネント

```go
type PathValidator interface {
    Validate(path string) (ValidatedPath, error)
    ValidateWithOptions(path string, opts ValidationOptions) (ValidatedPath, error)
}

type ValidationOptions struct {
    RequireExists    bool
    RequireRegular   bool
    RequireReadable  bool
    RequireWritable  bool
    RequireExecutable bool
}
```

### 5.2 型安全操作コンポーネント

```go
type SafePathOperations interface {
    Join(base ValidatedPath, elem string) (ValidatedPath, error)
    Dir(path ValidatedPath) ValidatedPath
    Base(path ValidatedPath) string
    Ext(path ValidatedPath) string
    Clean(path ValidatedPath) ValidatedPath
}
```

### 5.3 互換性レイヤー

```go
type CompatibilityLayer interface {
    // Legacy support
    ToLegacyString(path ValidatedPath) string
    FromLegacyString(path string) (ValidatedPath, error)

    // Migration helpers
    WrapUnsafe(path string) UnsafePath
    ConvertSafely(unsafe UnsafePath) (ValidatedPath, error)
}
```

## 6. データフロー

### 6.1 パス検証フロー

```
Raw String Path
    ↓
[Path Validation]
    ↓ (success)
ValidatedPath ←─────┐
    ↓               │
[Safe Operations]   │
    ↓               │
File System ────────┘
Operations
```

### 6.2 エラーハンドリングフロー

```
Raw String Path
    ↓
[Path Validation]
    ↓ (failure)
PathValidationError
    ↓
[Error Recovery]
    ↓
├── Retry with Correction
├── Alternative Path
└── Operation Cancellation
```

## 7. セキュリティ設計

### 7.1 防御層

1. **型システム層**: コンパイル時の型検査
2. **検証層**: 実行時のパス検証
3. **操作層**: 安全なファイル操作
4. **監査層**: セキュリティイベントのログ

### 7.2 脅威モデル

| 脅威 | 対策 | 実装 |
|------|------|------|
| パストラバーサル | 絶対パス強制 | `ValidatedPath` 型 |
| シンボリックリンク攻撃 | リンク解決 | `filepath.EvalSymlinks` |
| 型安全性迂回 | 型システム活用 | プライベートフィールド |
| 検証バイパス | 強制検証 | コンストラクタパターン |

### 7.3 セキュリティ監査

```go
type SecurityAuditor interface {
    LogPathValidation(path string, result ValidationResult)
    LogSafeOperation(operation string, path ValidatedPath)
    LogSecurityViolation(violation SecurityViolation)
}
```

## 8. パフォーマンス設計

### 8.1 最適化戦略

- **インライン化**: 小さな関数の最適化
- **文字列プール**: 頻繁に使用されるパスのキャッシュ
- **遅延評価**: 必要時のみ検証実行
- **ゼロコピー**: 可能な限りコピーを避ける

### 8.2 メモリ管理

```go
type PathCache struct {
    validated sync.Map // string -> ValidatedPath
    maxSize   int
    hitCount  int64
    missCount int64
}
```

### 8.3 ベンチマーク目標

- 型変換オーバーヘッド: < 5%
- メモリ使用量増加: < 5%
- 検証時間: < 1ms (通常パス)

## 9. 互換性設計

### 9.1 移行戦略

#### Phase 1: 基盤構築
- `ValidatedPath` 型の定義
- 基本的な検証機能の実装
- 単体テストの作成

#### Phase 2: 新機能での採用
- 新しい機能で型安全パスを使用
- 既存機能との連携テスト
- パフォーマンス測定

#### Phase 3: 既存機能の移行
- 段階的な既存コードの更新
- 後方互換APIの提供
- 移行ガイドの作成

#### Phase 4: 完全移行
- レガシーAPIの廃止予告
- 最終的な互換性テスト
- ドキュメントの更新

### 9.2 後方互換性

```go
// Legacy functions (deprecated)
func LegacyValidatePath(path string) (string, error) {
    validated, err := ValidatePath(path)
    if err != nil {
        return "", err
    }
    return validated.String(), nil
}
```

## 10. 監視・運用設計

### 10.1 メトリクス

- パス検証成功率
- 型変換エラー率
- パフォーマンス指標
- セキュリティインシデント数

### 10.2 ログ設計

```go
type PathOperationLog struct {
    Timestamp   time.Time
    Operation   string
    Path        string
    Validated   bool
    Duration    time.Duration
    Error       error
}
```

### 10.3 アラート設計

- 異常な検証失敗率
- パフォーマンス劣化
- セキュリティ違反の検出
- 型安全性迂回の試行

## 11. テスト戦略

### 11.1 テスト分類

- **単体テスト**: 各コンポーネントの機能テスト
- **統合テスト**: コンポーネント間の連携テスト
- **セキュリティテスト**: 脅威シナリオのテスト
- **パフォーマンステスト**: 性能要件の検証

### 11.2 テストデータ

```go
var testCases = []struct {
    name     string
    input    string
    expected ValidatedPath
    error    error
}{
    {"valid absolute path", "/tmp/test.txt", ValidatedPath{"/tmp/test.txt"}, nil},
    {"invalid relative path", "test.txt", ValidatedPath{}, ErrRelativePath},
    // ... more test cases
}
```

### 11.3 プロパティベーステスト

```go
func TestPathValidationProperties(t *testing.T) {
    property := func(path string) bool {
        validated, err := ValidatePath(path)
        if err != nil {
            return true // エラーは許容される
        }

        // 検証済みパスは常に絶対パス
        return filepath.IsAbs(validated.String())
    }

    quick.Check(property, nil)
}
```

## 12. 将来拡張性

### 12.1 型システムの拡張

- より具体的な型の追加（`ConfigFilePath`, `LogFilePath`など）
- 型レベルでの権限管理
- プラットフォーム固有の型

### 12.2 検証ルールの拡張

- カスタム検証ルールの追加
- 動的検証ルールの設定
- 外部検証サービスとの連携

### 12.3 パフォーマンス最適化

- JITコンパイラとの連携
- ハードウェア固有の最適化
- 並列検証の実装
