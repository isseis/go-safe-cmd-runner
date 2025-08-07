# 詳細仕様書：ハッシュマニフェスト形式

## 1. 概要

このドキュメントは、ハッシュファイル形式をテキスト形式からJSON形式に完全移行するための詳細仕様を定義する。実装は `02_architecture.md` で定義された設計方針に基づき、レガシー形式のサポートを廃止し、JSON形式のみサポートする。

## 2. データ構造定義

### 2.1. JSON形式の構造体定義

#### 2.1.1. HashManifest 構造体
```go
// HashManifest は、ハッシュファイルのJSON形式を定義する
type HashManifest struct {
    Version   string    `json:"version"`
    Format    string    `json:"format"`
    Timestamp time.Time `json:"timestamp"`
    File      FileInfo  `json:"file"`
}

// FileInfo は、ファイル情報を定義する
type FileInfo struct {
    Path string   `json:"path"`
    Hash HashInfo `json:"hash"`
}

// HashInfo は、ハッシュ情報を定義する
type HashInfo struct {
    Algorithm string `json:"algorithm"`
    Value     string `json:"value"`
}

const (
    // HashManifestVersion はハッシュマニフェストの現在のバージョン
    HashManifestVersion = "1.0"
    // HashManifestFormat はハッシュマニフェストのフォーマット識別子
    HashManifestFormat = "file-hash"
)
```

#### 2.1.2. フィールド詳細仕様

| フィールド | 型 | 必須 | 説明 | 例 |
|-----------|----|------|------|-----|
| version | string | ✓ | ファイル形式バージョン。`HashManifestVersion` 定数で定義 | "1.0" |
| format | string | ✓ | ファイル形式識別子。`HashManifestFormat` 定数で定義 | "file-hash" |
| timestamp | time.Time | ✓ | ハッシュ記録日時（UTC）。ゼロ値はエラーとなる | "2025-07-04T10:30:00Z" |
| file.path | string | ✓ | 対象ファイルの絶対パス。空文字列はエラー | "/home/user/file.txt" |
| file.hash.algorithm | string | ✓ | ハッシュアルゴリズム名（小文字） | "sha256" |
| file.hash.value | string | ✓ | ハッシュ値（16進数、小文字）。空文字列はエラー | "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" |

#### 2.1.3. バリデーション仕様

**version フィールド:**
- 形式: セマンティックバージョニング（major.minor）
- 現在サポート: `1.0` 定数のみ
- 不一致時エラー: `ErrUnsupportedVersion`

**format フィールド:**
- 固定値: `file-hash` 定数のみ
- 大文字小文字の区別: あり
- 不一致時エラー: `ErrInvalidManifestFormat`

**timestamp フィールド:**
- 形式: RFC3339（ISO 8601）
- タイムゾーン: UTC必須
- ゼロ値はエラー: `ErrInvalidTimestamp`

**file.path フィールド:**
- 形式: 絶対パス
- 空文字列はエラー: `ErrInvalidManifestFormat`
- 検証対象のファイルパスと完全一致しない場合は `ErrHashCollision`

**file.hash.algorithm フィールド:**
- 検証時に使用されるアルゴリズムと一致する必要あり
- 不一致時エラー: `ErrInvalidManifestFormat`

**file.hash.value フィールド:**
- 形式: 16進数文字列（小文字）
- 空文字列はエラー: `ErrInvalidManifestFormat`

### 2.2. JSON出力例

#### 2.2.1. 標準形式
```json
{
  "version": "1.0",
  "format": "file-hash",
  "timestamp": "2025-07-04T10:30:00Z",
  "file": {
    "path": "/home/user/documents/important.txt",
    "hash": {
      "algorithm": "SHA256",
      "value": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
    }
  }
}
```

#### 2.2.2. 実際の出力形式
- **インデント**: 2スペース
- **改行**: Unix形式（LF）
- **エンコーディング**: UTF-8
- **BOM**: なし

## 3. 機能仕様

### 3.1. JSON形式検証機能

#### 3.1.1. unmarshalHashManifest 関数
```go
func unmarshalHashManifest(content []byte) (HashManifest, error)
```

**パラメータ:**
- `content`: ハッシュファイルの内容

**戻り値:**
- `HashManifest`: 解析されたハッシュマニフェスト
- `error`: 解析エラー

**処理フロー:**
1. JSON形式として解析を試行
2. 構文エラーの場合、エラー位置を特定して `ErrInvalidManifestFormat` を返す
3. 型が不正な場合、フィールド名を特定して `ErrInvalidManifestFormat` を返す
4. その他のエラーの場合は `ErrJSONParseError` を返す

#### 3.1.2. validateHashManifest 関数
```go
func validateHashManifest(manifest HashManifest, algoName string, targetPath string) error
```

**パラメータ:**
- `manifest`: 検証するハッシュマニフェスト
- `algoName`: 期待するハッシュアルゴリズム名
- `targetPath`: 期待するファイルパス

**戻り値:**
- `error`: バリデーションエラー

**バリデーション内容:**
1. バージョンが `HashManifestVersion` と一致するか
2. フォーマットが `HashManifestFormat` と一致するか
3. ファイルパスが空でないか
4. ファイルパスが `targetPath` と一致するか
5. ハッシュアルゴリズムが `algoName` と一致するか
6. ハッシュ値が空でないか
7. タイムスタンプがゼロ値でないか

### 3.2. JSON読み込み機能

#### 3.2.1. parseAndValidateHashFile 関数
```go
func (v *Validator) parseAndValidateHashFile(content []byte, targetPath string) (string, string, error)
```

**処理手順:**
1. `unmarshalHashManifest` でJSONをパース
2. `validateHashManifest` でマニフェストを検証
3. ファイルパスとハッシュ値を返却

#### 3.2.2. readAndParseHashFile 関数
```go
func (v *Validator) readAndParseHashFile(targetPath string) (string, string, error)
```

**処理手順:**
1. ハッシュファイルのパスを取得
2. ハッシュファイルを安全に読み込み
3. `parseAndValidateHashFile` で検証とパースを実行

### 3.3. ハッシュマニフェスト作成機能

#### 3.3.1. createHashManifest 関数
```go
func createHashManifest(path, hash, algorithm string) HashManifest
```

**パラメータ:**
- `path`: ファイルの絶対パス
- `hash`: ハッシュ値（16進数、小文字）
- `algorithm`: ハッシュアルゴリズム名（小文字）

**戻り値:**
- `HashManifest`: 作成されたハッシュマニフェスト

**処理内容:**
1. 現在時刻をUTCで取得
2. 指定されたパラメータで `HashManifest` 構造体を初期化
3. バージョンとフォーマットは定数値を使用

#### 3.3.2. writeHashManifest 関数
```go
func (v *Validator) writeHashManifest(filePath string, manifest HashManifest) error
```

**処理手順:**
1. マニフェストをJSONにシリアライズ（2スペースインデント）
2. 末尾に改行を追加
3. 安全なファイル書き込み機能を使用して保存（パーミッション: 0o640）
- `ErrInvalidJSONFormat`: ファイル形式がJSON形式でない

### 3.4. JSON書き込み機能

#### 3.4.1. writeHashFileJSON 関数
```go
func (v *Validator) writeHashFileJSON(filePath string, format HashFileFormat) error
```

**処理手順:**
1. JSON形式への変換
2. インデント付きでマーシャル
3. SafeWriteFileでファイル書き込み

**実装例:**
```go
func (v *Validator) writeHashFileJSON(filePath string, format HashFileFormat) error {
    // JSON形式でマーシャル（インデント付き）
    jsonData, err := json.MarshalIndent(format, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal JSON: %w", err)
    }

    // 改行を追加
    jsonData = append(jsonData, '\n')

    // ファイル書き込み
    return safefileio.SafeWriteFile(filePath, jsonData, 0o640)
}
```

### 3.5. JSON構造体作成機能

#### 3.5.1. createHashFileFormat 関数
```go
func createHashFileFormat(path, hash, algorithm string) HashFileFormat
```

**実装:**
```go
func createHashFileFormat(path, hash, algorithm string) HashFileFormat {
    return HashFileFormat{
        Version:   "1.0",
        Format:    "file-hash",
        Timestamp: time.Now().UTC(),
        File: FileInfo{
            Path: path,
            Hash: HashInfo{
                Algorithm: algorithm,
                Value:     hash,
            },
        },
    }
}
```

## 4. 更新された関数仕様

### 4.1. Record 関数の更新

#### 4.1.1. 新しい実装
```go
func (v *Validator) Record(filePath string) error {
    // 既存の処理（パス検証、ハッシュ計算）
    targetPath, err := validatePath(filePath)
    if err != nil {
        return err
    }

    hash, err := v.calculateHash(targetPath)
    if err != nil {
        return fmt.Errorf("failed to calculate hash: %w", err)
    }

    hashFilePath, err := v.GetHashFilePath(targetPath)
    if err != nil {
        return err
    }

    if err := os.MkdirAll(filepath.Dir(hashFilePath), 0o750); err != nil {
        return fmt.Errorf("failed to create hash directory: %w", err)
    }

    // 既存ファイルの確認（JSON形式のみサポート）
    if existingContent, err := safefileio.SafeReadFile(hashFilePath); err == nil {
        // 既存ファイルが存在する場合、JSON形式かどうか確認
        if !isJSONFormat(existingContent) {
            return ErrInvalidHashFileFormat
        }

        // JSON形式の場合、衝突チェック
        if err := v.checkJSONHashCollision(existingContent, targetPath); err != nil {
            return err
        }
    } else if !os.IsNotExist(err) {
        return fmt.Errorf("failed to check existing hash file: %w", err)
    }

    // JSON形式でハッシュファイル作成
    format := createHashFileFormat(targetPath, hash, v.algorithm.Name())

    return v.writeHashFileJSON(hashFilePath, format)
}
```

### 4.2. readAndParseHashFile 関数の更新

#### 4.2.1. 新しい実装
```go
func (v *Validator) readAndParseHashFile(targetPath string) (string, string, error) {
    hashFilePath, err := v.GetHashFilePath(targetPath)
    if err != nil {
        return "", "", err
    }

    hashFileContent, err := safefileio.SafeReadFile(hashFilePath)
    if err != nil {
        if os.IsNotExist(err) {
            return "", "", ErrHashFileNotFound
        }
        return "", "", fmt.Errorf("failed to read hash file: %w", err)
    }

    // JSON形式の検証と解析
    format, err := validateHashFileFormat(hashFileContent)
    if err != nil {
        return "", "", fmt.Errorf("failed to validate hash file format: %w", err)
    }

    return v.parseJSONHashFile(format, targetPath)
}
```

## 5. エラーハンドリング仕様

### 5.1. 新しいエラー定義

#### 5.1.1. errors.go への追加
```go
var (
    // JSON関連エラー
    ErrInvalidJSONFormat = errors.New("invalid JSON format in hash file")
    ErrUnsupportedVersion = errors.New("unsupported hash file version")
    ErrInvalidTimestamp = errors.New("invalid timestamp in hash file")
    ErrJSONParseError = errors.New("failed to parse JSON hash file")

    // 既存エラーの継続使用
    ErrHashCollision = errors.New("hash collision detected")
    ErrHashFileNotFound = errors.New("hash file not found")
)
```

### 5.2. エラー処理フロー

#### 5.2.1. JSON解析エラー
```go
func parseJSONHashFile(content []byte) (HashFileFormat, error) {
    var format HashFileFormat
    if err := json.Unmarshal(content, &format); err != nil {
        return HashFileFormat{}, fmt.Errorf("%w: %v", ErrJSONParseError, err)
    }
    return format, nil
}
```

#### 5.2.2. レガシー形式エラー
```go
func validateHashFileFormat(content []byte) (HashFileFormat, error) {
    if !isJSONFormat(content) {
        return HashFileFormat{}, ErrInvalidJSONFormat
    }

    // JSON解析処理...
    return parseJSONHashFile(content)
}
```

## 6. テスト仕様

### 6.1. 単体テスト

#### 6.1.1. JSON形式テスト
```go
func TestDetectHashFileFormat_JSON(t *testing.T) {
    jsonContent := `{
        "version": "1.0",
        "format": "file-hash",
        "timestamp": "2025-07-04T10:30:00Z",
        "file": {
            "path": "/tmp/test.txt",
            "hash": {
                "algorithm": "SHA256",
                "value": "abc123"
            }
        }
    }`

    format, isJSON, err := detectHashFileFormat([]byte(jsonContent))
    assert.NoError(t, err)
    assert.True(t, isJSON)
    assert.Equal(t, "1.0", format.Version)
    assert.Equal(t, "file-hash", format.Format)
    assert.Equal(t, "/tmp/test.txt", format.File.Path)
    assert.Equal(t, "SHA256", format.File.Hash.Algorithm)
    assert.Equal(t, "abc123", format.File.Hash.Value)
}
```

#### 6.1.2. レガシー形式エラーテスト
```go
func TestDetectHashFileFormat_Legacy(t *testing.T) {
    legacyContent := "/tmp/test.txt\nabc123"

    _, err := validateHashFileFormat([]byte(legacyContent))
    assert.Error(t, err)
    assert.True(t, errors.Is(err, ErrInvalidJSONFormat))
}
```

#### 6.1.3. エラーケーステスト
```go
func TestDetectHashFileFormat_InvalidJSON(t *testing.T) {
    invalidJSON := `{"version": "1.0", "format": "file-hash"`

    _, err := validateHashFileFormat([]byte(invalidJSON))
    assert.Error(t, err)
    assert.True(t, errors.Is(err, ErrJSONParseError))
}

func TestDetectHashFileFormat_UnsupportedVersion(t *testing.T) {
    unsupportedVersion := `{
        "version": "2.0",
        "format": "file-hash",
        "timestamp": "2025-07-04T10:30:00Z",
        "file": {
            "path": "/tmp/test.txt",
            "hash": {
                "algorithm": "SHA256",
                "value": "abc123"
            }
        }
    }`

    _, err := validateHashFileFormat([]byte(unsupportedVersion))
    assert.Error(t, err)
    assert.True(t, errors.Is(err, ErrUnsupportedVersion))
}
```

### 6.2. 統合テスト

#### 6.2.1. Record/Verify統合テスト
```go
func TestValidator_RecordVerify_JSON(t *testing.T) {
    tempDir := t.TempDir()
    validator, err := New(&SHA256{}, tempDir)
    require.NoError(t, err)

    // テストファイル作成
    testFile := filepath.Join(tempDir, "test.txt")
    err = os.WriteFile(testFile, []byte("test content"), 0o644)
    require.NoError(t, err)

    // Record（JSON形式で保存）
    err = validator.Record(testFile)
    assert.NoError(t, err)

    // ハッシュファイルがJSON形式で作成されているか確認
    hashFilePath, err := validator.GetHashFilePath(testFile)
    require.NoError(t, err)

    content, err := os.ReadFile(hashFilePath)
    require.NoError(t, err)

    assert.True(t, isJSONFormat(content))

    // Verify
    err = validator.Verify(testFile)
    assert.NoError(t, err)
}
```

#### 6.2.2. レガシー形式エラーテスト
```go
func TestValidator_LegacyFormatError(t *testing.T) {
    tempDir := t.TempDir()
    validator, err := New(&SHA256{}, tempDir)
    require.NoError(t, err)

    // テストファイル作成
    testFile := filepath.Join(tempDir, "test.txt")
    err = os.WriteFile(testFile, []byte("test content"), 0o644)
    require.NoError(t, err)

    // レガシー形式でハッシュファイル作成（手動）
    hashFilePath, err := validator.GetHashFilePath(testFile)
    require.NoError(t, err)

    hash, err := validator.calculateHash(testFile)
    require.NoError(t, err)

    legacyContent := fmt.Sprintf("%s\n%s", testFile, hash)
    err = os.WriteFile(hashFilePath, []byte(legacyContent), 0o644)
    require.NoError(t, err)

    // レガシー形式でのVerify（エラーになるはず）
    err = validator.Verify(testFile)
    assert.Error(t, err)
    assert.True(t, errors.Is(err, ErrInvalidJSONFormat))
}
```

## 7. 性能仕様

### 7.1. 性能目標

| 項目 | 目標値 | 測定方法 |
|------|--------|----------|
| 処理時間 | 既存の110%以内 | ベンチマークテスト |
| メモリ使用量 | 既存の110%以内 | プロファイリング |
| CPU使用率 | 既存の110%以内 | プロファイリング |

### 7.2. ベンチマークテスト

#### 7.2.1. Record性能テスト
```go
func BenchmarkValidator_Record_JSON(b *testing.B) {
    tempDir := b.TempDir()
    validator, err := New(&SHA256{}, tempDir)
    require.NoError(b, err)

    testFile := filepath.Join(tempDir, "test.txt")
    err = os.WriteFile(testFile, []byte("test content"), 0o644)
    require.NoError(b, err)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = validator.Record(testFile)
    }
}
```

#### 7.2.2. Verify性能テスト
```go
func BenchmarkValidator_Verify_JSON(b *testing.B) {
    tempDir := b.TempDir()
    validator, err := New(&SHA256{}, tempDir)
    require.NoError(b, err)

    testFile := filepath.Join(tempDir, "test.txt")
    err = os.WriteFile(testFile, []byte("test content"), 0o644)
    require.NoError(b, err)

    // 事前にRecordを実行
    err = validator.Record(testFile)
    require.NoError(b, err)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = validator.Verify(testFile)
    }
}
```

## 8. 実装チェックリスト

### 8.1. Phase 1: 基盤整備
- [ ] HashFileFormat構造体の定義
- [ ] 新しいエラータイプの追加
- [ ] JSON読み書き基本機能の実装
- [ ] JSON形式検証機能の実装

### 8.2. Phase 2: 書き込み機能
- [ ] Record関数の更新
- [ ] JSON形式でのファイル書き込み
- [ ] タイムスタンプ自動設定
- [ ] レガシー形式に対するエラー処理

### 8.3. Phase 3: 読み込み機能
- [ ] readAndParseHashFile関数の更新
- [ ] JSON形式の解析機能
- [ ] レガシー形式に対するエラー処理
- [ ] エラーハンドリングの実装

### 8.4. Phase 4: テスト
- [ ] 単体テストの実装
- [ ] 統合テストの実装
- [ ] レガシー形式エラーテストの実装
- [ ] 性能テストの実装

### 8.5. Phase 5: 検証
- [ ] 全体的な動作確認
- [ ] 性能測定
- [ ] セキュリティ検証
- [ ] ドキュメント更新
