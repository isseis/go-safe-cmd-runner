# 設計方針書：ファイル改ざん検出（現行実装版）

## 1. 概要

現在実装されているファイル改ざん検出のためのGoライブラリの設計方針を記述する。
このライブラリは、指定されたファイルのハッシュ値を記録し、現在の状態と比較することでファイルの完全性を検証する機能を提供し、セキュリティを重視した設計が採用されている。

## 2. アーキテクチャ

### 2.1. 全体構成

- Goの `internal` ディレクトリ内に、`filevalidator` という名前の独立したパッケージとして実装される。
- ライブラリは、ハッシュ化アルゴリズムをインターフェースとして抽象化し、具象的なアルゴリズム（現在はSHA-256）を注入（Dependency Injection）する設計を採用している。
- テスト可能性を向上させるため、ファイルシステム操作やハッシュファイルパス生成もインターフェース化されている。

### 2.2. コンポーネント

#### 2.2.1. 主要コンポーネント

1. **`Validator` (構造体):**
   - ライブラリの主要なエントリーポイント
   - ハッシュ化アルゴリズムのインターフェース、ハッシュファイルの保存先ディレクトリパス、ハッシュファイルパス生成器を保持
   - ファイルの記録や検証の責務を持つ

2. **`HashAlgorithm` (インターフェース):**
   - ハッシュ計算のロジックを抽象化
   - `Name() string` と `Sum(io.Reader) (string, error)` の2つのメソッドを定義
   - `io.Reader` を引数に取ることで、大きなファイルでもメモリ効率良く処理

3. **`SHA256` (構造体):**
   - `HashAlgorithm` インターフェースの具体的な実装
   - Go標準ライブラリの `crypto/sha256` を用いてハッシュ値を計算

4. **`HashFilePathGetter` (インターフェース):**
   - ハッシュファイルのパス生成ロジックを抽象化
   - `GetHashFilePath(HashAlgorithm, string, string) (string, error)` メソッドを定義
   - テスト時にモック実装を注入可能

5. **`ProductionHashFilePathGetter` (構造体):**
   - 本番環境用のハッシュファイルパス生成器の実装
   - ファイルパスのSHA-256ハッシュからBase64URL形式で先頭12文字を使用

#### 2.2.2. セキュリティコンポーネント

1. **`SafeReadFile` 関数:**
   - O_NOFOLLOWフラグを使用したセキュアなファイル読み込み
   - ファイルサイズ制限（128MB）による DoS 攻撃対策
   - TOCTOU攻撃対策（ファイルオープン後の検証）
   - `verifyPathComponents` による親ディレクトリのシンボリックリンクチェック
   - `validateFile` による通常ファイル確認

2. **`SafeWriteFile` 関数:**
   - O_NOFOLLOW + O_CREATE + O_EXCL フラグを使用したセキュアなファイル書き込み
   - 既存ファイルへの意図しない上書きを防止
   - パスコンポーネントのシンボリックリンクチェック
   - TOCTOU攻撃対策

3. **`FileSystem` + `File` インターフェース:**
   - ファイルシステム操作の抽象化
   - テスト時のモック化を可能にする
   - `osFS` 構造体による実際のファイルシステム操作

4. **`verifyPathComponents` 関数:**
   - ファイルパスの親ディレクトリ階層をルートまで遡ってシンボリックリンクを検出
   - `os.Lstat` を使用して安全にシンボリックリンクチェック
   - 存在しないディレクトリは安全とみなして処理継続

5. **`validateFile` 関数:**
   - ファイルディスクリプタから `Stat()` を呼び出してファイル情報を取得
   - 通常ファイル（`IsRegular()`）であることを確認
   - デバイスファイル、パイプ、ソケットなどを排除

#### 2.2.3. プラットフォーム対応

1. **`isNoFollowError` 関数:**
   - プラットフォーム別のO_NOFOLLOWエラー処理
   - NetBSD（EFTYPE）とその他のUnix系（ELOOP, EMLINK）に対応
   - `nofollow_error.go` と `nofollow_error_netbsd.go` でビルドタグ分岐

### 2.3. ハッシュファイル仕様

#### 2.3.1. 命名規則

ハッシュファイル名は以下の手順で生成される：
1. 対象ファイルの絶対パスを `validatePath` で検証・正規化する
2. 絶対パスのSHA-256ハッシュを計算する
3. ハッシュ値をURL-safe Base64でエンコードする
4. エンコードされた文字列の先頭12文字をファイル名として使用する
5. ハッシュアルゴリズム名を拡張子として付与する（例: `.sha256`）

例: 対象ファイル `/usr/local/bin/app` → ハッシュファイル名 `AB1cD3fG4hI5.sha256`

#### 2.3.2. ファイル形式

ハッシュファイルはプレーンテキスト形式で、以下の構造を持つ：

```
<対象ファイルの絶対パス>
<ハッシュ値>
```

- 1行目: 検証対象ファイルの絶対パス
- 2行目: ファイルのハッシュ値（16進数文字列）
- 改行コード: `\n` (LF)

例：
```
/usr/local/bin/app
2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae
```

この形式により、ハッシュコリジョンの検出が可能になり、異なるファイルが同じハッシュファイル名を生成した場合にエラーを検出できる。

### 2.4. ディレクトリ構成

```
internal/
└── filevalidator/
    ├── validator.go                // Validator構造体と主要メソッド（Record, Verify等）
    ├── hash.go                     // HashAlgorithmインターフェースとSHA256実装
    ├── errors.go                   // カスタムエラーの定義
    ├── validator_helper.go         // SafeReadFile, SafeWriteFile, セキュリティヘルパー
    ├── nofollow_error.go           // Unix系OS用のO_NOFOLLOWエラー処理
    ├── nofollow_error_netbsd.go    // NetBSD用のO_NOFOLLOWエラー処理
    ├── validator_test.go           // 主要機能の単体テスト
    ├── hash_test.go                // ハッシュ関連のテスト
    ├── validator_helper_test.go    // セキュリティ機能のテスト
    ├── validator_error_test.go     // エラーハンドリングのテスト
    └── nofollow_error_*_test.go    // プラットフォーム別エラー処理テスト
```

### 2.5. データフロー

#### 2.5.1. ハッシュ記録フロー（Record）

```
1. ファイルパス入力
    ↓
2. validatePath（絶対パス化、シンボリックリンク解決、通常ファイル確認）
    ↓
3. SafeReadFile（O_NOFOLLOW使用、セキュア読み込み）
    ↓
4. ハッシュ計算（SHA-256）
    ↓
5. ハッシュファイルパス生成（Base64URL、先頭12文字）
    ↓
6. 既存ハッシュファイルチェック（パス衝突検出）
    ↓
7. SafeWriteFile（O_NOFOLLOW|O_CREATE|O_EXCL使用、セキュア書き込み）
```

#### 2.5.2. ハッシュ検証フロー（Verify）

```
1. ファイルパス入力
    ↓
2. validatePath（絶対パス化、シンボリックリンク解決、通常ファイル確認）
    ↓
3. SafeReadFile（現在のファイル内容読み込み）
    ↓
4. 現在のハッシュ値計算（SHA-256）
    ↓
5. ハッシュファイルパス生成
    ↓
6. SafeReadFile（ハッシュファイル読み込み）
    ↓
7. ハッシュファイル解析（パス + ハッシュ値）
    ↓
8. パス一致確認（ハッシュ衝突対策）
    ↓
9. ハッシュ値比較（一致/不一致）
```
    ├── hash_test.go                // ハッシュ関連のテスト
    ├── validator_helper_test.go    // セキュリティ機能のテスト
    ├── validator_error_test.go     // エラーハンドリングのテスト
    └── nofollow_error_*_test.go    // プラットフォーム別エラー処理テスト
```

## 3. 使用技術

- **Go:** 1.21 以上
- **ハッシュ計算:** `crypto/sha256` (標準ライブラリ)
- **ファイルI/O:** `os`, `io`, `syscall` (標準ライブラリ)
- **パス操作:** `path/filepath` (標準ライブラリ)
- **エンコーディング:** `encoding/base64` (標準ライブラリ)
- **テスト:** `testing` (標準ライブラリ)

## 4. API設計

### 4.1. 公開API

```go
package filevalidator

// New は、指定されたハッシュアルゴリズムとハッシュファイルの保存先ディレクトリで
// 初期化された新しいValidatorを返す。
func New(algorithm HashAlgorithm, hashDir string) (*Validator, error)

// Record は、指定されたfilePathのファイルのハッシュ値を計算し、
// Validatorに設定されたディレクトリ内に保存する。
func (v *Validator) Record(filePath string) error

// Verify は、指定されたfilePathのファイルが、記録されたハッシュ値と
// 一致するかどうかを検証する。
func (v *Validator) Verify(filePath string) error

// GetHashFilePath は、指定されたfilePathに対応するハッシュファイルのパスを返す。
func (v *Validator) GetHashFilePath(filePath string) (string, error)

// GetHashAlgorithm は、Validatorが使用するハッシュアルゴリズムを返す。
func (v *Validator) GetHashAlgorithm() HashAlgorithm

// GetHashDir は、ハッシュファイルの保存ディレクトリを返す。
func (v *Validator) GetHashDir() string
```

### 4.2. セキュリティAPI

```go
// SafeReadFile は、セキュリティを考慮してファイルを安全に読み込む。
func SafeReadFile(filePath string) ([]byte, error)

// SafeWriteFile は、セキュリティを考慮してファイルを安全に書き込む。
func SafeWriteFile(filePath string, content []byte, perm os.FileMode) error
```

## 5. セキュリティ設計

### 5.1. TOCTOU攻撃対策
- ファイルを開いた後、ファイルディスクリプタを使用して検証を行う
- パス解決とファイル操作を原子的に実行する
- `verifyPathComponents` 関数による事後的なディレクトリチェック

### 5.2. シンボリックリンク攻撃対策
- O_NOFOLLOWフラグを使用してシンボリックリンクを追跡しない
- ディレクトリパスの各コンポーネントがシンボリックリンクでないことを `verifyPathComponents` で確認
- プラットフォーム固有のエラーコード（NetBSDのEFTYPE等）に対応
- `validatePath` での事前シンボリックリンク解決とレギュラーファイルチェック

### 5.3. パス・トラバーサル攻撃対策
- `filepath.Abs`で絶対パスに変換
- `filepath.EvalSymlinks`でシンボリックリンクを解決
- パスの正規化を実行

### 5.4. DoS攻撃対策
- ファイルサイズ制限（MaxFileSize = 128MB）を設ける
- `io.LimitReader`を使用してメモリ使用量を制限
- `readFileContent` でファイルサイズの二重チェック実装

### 5.5. ハッシュ衝突攻撃対策
- ハッシュファイルに元のファイルパスを記録
- 検証時にパスの一致もチェックする
- 異なるファイルが同じハッシュファイル名を生成した場合に`ErrHashCollision`を返す
- `readAndParseHashFile` での厳密なパス一致確認

### 5.6. ファイル上書き攻撃対策
- `SafeWriteFile` でO_EXCLフラグを使用して意図しない上書きを防止
- 既存ファイルの場合は`ErrFileExists`エラーを返す
- ハッシュ記録時は同一パスの場合のみ上書きを許可

### 5.7. エラーハンドリング
- 機密情報を含まない適切なエラーメッセージ
- エラーの種類に応じた処理を可能にする型安全なエラー定義
- ファイルI/Oエラーの詳細な情報を提供
- 13種類の定義済みエラー型による詳細なエラー分類

## 6. テスト設計

### 6.1. テスト戦略
- 依存性注入によるモック化
- プラットフォーム固有のテスト（build tags使用）
- エラーケースの網羅的テスト
- セキュリティ機能の検証

### 6.2. テスト用コンポーネント
- `MockHashAlgorithm`: テスト用ハッシュアルゴリズム
- `CollidingHashAlgorithm`: ハッシュ衝突テスト用
- `CollidingHashFilePathGetter`: パス衝突テスト用
- 各種ファイルシステムモック

## 7. 拡張性

### 7.1. 新しいハッシュアルゴリズムの追加
- `HashAlgorithm`インターフェースを実装するだけで新しいアルゴリズムを追加可能
- アルゴリズム名がハッシュファイルの拡張子として使用される

### 7.2. 新しいセキュリティ機能の追加
- インターフェース設計により、セキュリティ機能の追加が容易
- プラットフォーム固有の処理は build tags で分離

### 7.3. テスト機能の拡張
- 依存性注入により、新しいテストシナリオの追加が容易
- モック実装の作成により、複雑なエラーケースのテストが可能
