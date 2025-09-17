# 実装計画書：ハイブリッドハッシュファイル名エンコーディング

## 1. 既存コードとの関係分析

### 1.1. 既存コードベース分析結果

- **HashFilePathGetter インターフェース**: `internal/filevalidator/validator.go:42-45` で既に定義済み
- **ProductionHashFilePathGetter**: `internal/filevalidator/validator.go:47-61` で既存実装済み（SHA256ベース）
- **Hash関連エラー**: `internal/filevalidator/errors.go` で定義済み
- **ロガー**: `internal/runner/audit/logger.go` で slog ベースのロガーが存在
- **FileSystem**: `internal/verification/types.go` および関連ファイルで抽象化済み

### 1.2. 重複回避戦略

1. **HashFilePathGetter インターフェースを再利用**（新規定義しない）
2. **既存エラータイプを拡張**（完全に新規作成しない）
3. **slog ベースのロガーを利用**（独自ロガーインターフェースを定義しない）
4. **既存のファイルシステム抽象化を使用**

## 2. 段階的実装計画

### フェーズ 1: エンコーディングコア機能（第1週）

#### ☐ 1.1. エンコーディングパッケージ基盤作成
- **場所**: `internal/filevalidator/encoding/`
- **ファイル**:
  - `substitution_hash_escape.go` - メインエンコーダー
  - `encoding_result.go` - 結果構造体
  - `errors.go` - エンコーディング固有エラー
- **依存関係**: なし（標準ライブラリのみ）

#### ☐ 1.2. SubstitutionHashEscape の基本実装
- **機能**:
  - `Encode()` - 基本エンコード
  - `Decode()` - 基本デコード
  - `substitute()` / `reverseSubstitute()` - 文字置換
  - `doubleEscape()` - ダブルエスケープ
- **テスト**: 基本的なユニットテスト作成

#### ☐ 1.3. フォールバック機能実装
- **機能**:
  - `EncodeWithFallback()` - ハイブリッド機能
  - `generateSHA256Fallback()` - SHA256フォールバック
  - `IsNormalEncoding()` / `IsFallbackEncoding()` - 判定機能
- **テスト**: フォールバック条件のテスト

### フェーズ 2: 統合とValidator接続（第2週）

#### ☐ 2.1. HybridHashFilePathGetter 実装
- **場所**: `internal/filevalidator/hybrid_hash_file_path_getter.go`
- **既存インターフェース実装**: `HashFilePathGetter`
- **依存関係**:
  - `encoding` パッケージの関数群
  - 既存の `slog.Logger`
- **機能**:
  - `GetHashFilePath()` - インターフェース実装
  - `AnalyzeFilePath()` - 分析機能
  - `GetEncodingStats()` - 統計情報

#### ☐ 2.2. 既存エラータイプ拡張
- **場所**: `internal/filevalidator/errors.go` に追加
- **追加エラー**:
  - `ErrFallbackNotReversible`
  - `ErrPathTooLong`
  - `ErrInvalidEncodedName`
- **既存エラーとの整合性確保**

#### ☐ 2.3. Validator との統合テスト
- **場所**: `internal/filevalidator/hybrid_hash_file_path_getter_test.go`
- **テスト内容**:
  - 既存Validatorとの結合テスト
  - エラーハンドリングテスト
  - パフォーマンステスト

### フェーズ 3: 移行機能実装（第3週）

#### ☐ 3.1. MigrationHashFilePathGetter 実装
- **場所**: `internal/filevalidator/migration_hash_file_path_getter.go`
- **依存関係**:
  - `HybridHashFilePathGetter`
  - `ProductionHashFilePathGetter` (既存)
  - `internal/verification` のFileSystem抽象化
- **機能**:
  - `GetHashFilePath()` - 移行サポート付き実装。以下の優先順位でハッシュファイルのパスを探索・決定する:
    1.  **新しい形式のパス**: `HybridHashFilePathGetter` を用いて、ファイルパスに応じた最適なエンコーディング（Normal EncodingまたはSHA256 Fallback）のハッシュファイルパスを計算し、そのファイルが存在するか確認する。
    2.  **古い形式のパス（後方互換性）**: 1.のファイルが存在しない場合、`ProductionHashFilePathGetter` を用いて計算した純粋なSHA256形式のハッシュファイルパスが存在するか確認する。これは、短いファイルパスであっても過去に作成されたハッシュファイルに対応するための措置である。
    3.  **新規作成パス**: 1.と2.のどちらも存在しない場合は、1.で計算した新しい形式のパスを、これから作成されるべきパスとして返す。
  - `MigrateHashFile()` - 単体ファイル移行
  - `BatchMigrate()` - バッチ移行

#### ☐ 3.2. ファイルシステム抽象化の活用
- **既存活用**: `internal/verification` のFileSystemInterface
- **必要な場合のみ拡張**: ファイル存在確認、コピー、削除機能
- **テスト**: モックファイルシステムでの移行テスト

### フェーズ 4: 分析・デバッグ機能（第4週）

#### ☐ 4.1. 分析機能実装
- **場所**: `encoding/substitution_hash_escape.go` に追加
- **機能**:
  - `AnalyzeEncoding()` - 詳細分析
  - `analyzeCharFrequency()` - 文字頻度分析
  - `countEscapeOperations()` - エスケープ操作カウント

#### ☐ 4.2. パフォーマンス最適化
- **ベンチマークテスト**: `internal/filevalidator/benchmark_encoding_test.go`
- **最適化項目**:
  - 文字列操作の効率化
  - メモリアロケーション削減
  - キャッシュ機能（必要な場合）

#### ☐ 4.3. プロパティベーステスト
- **場所**: `internal/filevalidator/encoding/property_test.go`
- **テスト内容**:
  - リバーシビリティ
  - 決定論的動作
  - ユニークネス

### フェーズ 5: 統合テスト・ドキュメント（第5週）

#### ☐ 5.1. 完全統合テスト
- **エンドツーエンドテスト**
- **レグレッションテスト**
- **既存機能との互換性確認**

#### ☐ 5.2. エラーハンドリング検証
- **エラー分類とメッセージの一貫性**
- **ログ出力の適切性**
- **回復処理の検証**

#### ☐ 5.3. ドキュメント更新
- **CLAUDE.md** の更新（必要に応じて）
- **コード内ドキュメント** の完備
- **使用例とベストプラクティス**

## 3. 実装の詳細事項

### 3.1. 既存コード再利用箇所

```go
// 既存インターフェースを実装
type HybridHashFilePathGetter struct {
    logger  *slog.Logger  // 既存のslogを利用
}

// 既存インターフェースを実装
func (h *HybridHashFilePathGetter) GetHashFilePath(
    hashAlgorithm HashAlgorithm,        // 既存の型
    hashDir string,
    filePath common.ResolvedPath) (string, error) {  // 既存の型
    // 実装
}
```

### 3.2. 新規作成が必要な箇所

```go
// 新規パッケージ
package encoding

// 関数ベースの実装 - 構造体不要
// func Encode(path string) string
// func EncodeWithFallback(path string) EncodingResult
// func Decode(encoded string) (string, error)

type EncodingResult struct {
    EncodedName    string
    IsFallback     bool
    OriginalLength int
    EncodedLength  int
}
```

### 3.3. 既存エラータイプ拡張

```go
// internal/filevalidator/errors.go に追加
var (
    // 新規エラー
    ErrFallbackNotReversible = errors.New("fallback encoding cannot be decoded to original path")
    ErrPathTooLong          = errors.New("encoded path too long")
    ErrInvalidEncodedName   = errors.New("invalid encoded name format")
)
```

## 4. テスト戦略

### 4.1. ユニットテスト
- **各関数の単体テスト**
- **エラーケースの網羅**
- **境界値テスト**

### 4.2. 統合テスト
- **既存Validatorとの結合**
- **ファイルシステム操作**
- **移行シナリオ**

### 4.3. パフォーマンステスト
- **エンコード・デコード速度**
- **メモリ使用量**
- **大量ファイル処理**

### 4.4. プロパティベーステスト
- **エンコード/デコードの可逆性**
- **決定論的動作**
- **ユニークネス保証**

## 5. リスク管理

### 5.1. 技術的リスク
- **既存システムとの互換性**: 段階的移行で対応
- **パフォーマンス劣化**: ベンチマークテストで監視
- **エッジケース**: 豊富なテストケースで対応

### 5.2. 移行リスク
- **データ損失**: バックアップ機能を強制実装
- **ダウンタイム**: 手動移行のみサポート
- **ロールバック**: 既存ファイルの保持

## 6. 成功基準

### 6.1. 機能要件
- ☐ 既存HashFilePathGetterインターフェースの完全実装
- ☐ SHA256フォールバック機能の動作
- ☐ エンコード/デコードの可逆性（通常エンコードのみ）
- ☐ 手動移行機能の動作

### 6.2. 非機能要件
- ☐ テストカバレッジ 90% 以上
- ☐ エンコード速度: 既存実装の150%以内
- ☐ メモリ使用量: 既存実装の120%以内
- ☐ 既存テストの100%パス

### 6.3. 品質要件
- ☐ リンターエラー 0件
- ☐ コードレビュー承認
- ☐ ドキュメント完備
- ☐ エラーハンドリング一貫性

## 7. 次のアクション

1. **フェーズ1の開始**: エンコーディングコア機能の実装
2. **開発環境準備**: テスト環境とベンチマーク環境の整備
3. **詳細設計**: 各クラスの詳細インターフェース設計
4. **プロトタイプ**: 小規模な動作確認実装

この計画により、既存コードとの重複を最小限に抑えながら、ハイブリッドハッシュファイル名エンコーディング機能を段階的に実装できます。
