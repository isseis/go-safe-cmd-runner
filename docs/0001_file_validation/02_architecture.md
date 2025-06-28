# 設計方針書：ファイル改ざん検出

## 2.1. 概要

要件定義書に基づき、ファイル改ざんを検出するためのGoライブラリの設計方針を定める。
このライブラリは、指定されたファイルのハッシュ値を記録し、現在の状態と比較することでファイルの完全性を検証する機能を提供する。

## 2.2. アーキテクチャ

### 2.2.1. 全体構成

-   Goの `internal` ディレクトリ内に、`filevalidator` という名前の独立したパッケージとして実装する。これにより、プロジェクト内部での利用に限定し、意図しない外部からの利用を防ぐ。
-   ライブラリは、ハッシュ化アルゴリズムをインターフェースとして抽象化し、具象的なアルゴリズム（今回はSHA-256）を注入（Dependency Injection）する設計を採用する。これにより、将来的なアルゴリズムの追加・変更が容易になる。

### 2.2.2. コンポーネント

1.  **`Validator` (構造体):**
    -   ライブラリの主要なエントリーポイント。
    -   ハッシュ化アルゴリズムのインターフェースと、ハッシュファイルの保存先ディレクトリパスを保持し、具体的な検証ロジックを実行する責務を持つ。
    -   利用者はこの構造体のインスタンスをファクトリ関数 `New` を通じて生成し、ファイルの記録や検証、関連パスの変換を行う。

2.  **`HashAlgorithm` (インターフェース):**
    -   ハッシュ計算のロジックを抽象化する。
    -   `Name() string` (アルゴリズム名、例: "sha256") と `Sum(io.Reader) (string, error)` (ハッシュ値計算) の2つのメソッドを定義する。
    -   `io.Reader` を引数に取ることで、大きなファイルでもメモリ効率良く処理できるようにする。

3.  **`SHA256` (構造体):**
    -   `HashAlgorithm` インターフェースの具体的な実装。
    -   Go標準ライブラリの `crypto/sha256` を用いてハッシュ値を計算する。

4.  **カスタムエラー:**
    -   検証失敗（ハッシュ値の不一致）、ファイルI/Oエラーなど、ライブラリが返しうるエラーを型として定義する。これにより、利用側はエラーの種類に応じた分岐処理を容易に記述できる。

### 2.2.3. ディレクトリ構成

```
internal/
└── filevalidator/
    ├── validator.go         // Validator構造体と主要メソッド
    ├── hash.go              // HashAlgorithmインターフェースとSHA256実装
    ├── errors.go            // カスタムエラーの定義
    └── validator_test.go    // 単体テスト
```

## 2.3. 使用技術

-   **Go:** 1.21 以上
-   **ハッシュ計算:** `crypto/sha256` (標準ライブラリ)
-   **ファイルI/O:** `os`, `io` (標準ライブラリ)
-   **パス操作:** `path/filepath` (標準ライブラリ)
-   **テスト:** `testing` (標準ライブラリ)

## 2.4. API設計

ライブラリの公開APIは以下のように設計する。

```go
package filevalidator

// New は、指定されたハッシュアルゴリズムとハッシュファイルの保存先ディレクトリで
// 初期化された新しいValidatorを返す。
// hashDirは存在するディレクトリでなければならない。
func New(algorithm HashAlgorithm, hashDir string) (*Validator, error)

// Record は、指定されたfilePathのファイルのハッシュ値を計算し、
// Validatorに設定されたディレクトリ内に保存する。
func (v *Validator) Record(filePath string) error

// Verify は、指定されたfilePathのファイルが、記録されたハッシュ値と
// 一致するかどうかを検証する。
// 一致しない場合は ErrMismatch を、ハッシュファイルが見つからない場合は
// ErrHashFileNotFound を返す。
func (v *Validator) Verify(filePath string) error

// GetHashFilePath は、指定されたfilePathに対応するハッシュファイルのパスを返す。
// パスの検証も内部的に行われる。
func (v *Validator) GetHashFilePath(filePath string) (string, error)

// GetTargetFilePath は、指定されたhashFilePathから元のファイルパスをデコードして返す。
// パスがv.hashDir内にない場合や、デコードに失敗した場合はエラーを返す。
func (v *Validator) GetTargetFilePath(hashFilePath string) (string, error)
```

## 2.5. セキュリティ設計

-   **パス・トラバーサル:** `path/filepath.Clean` を用いてパスを正規化する。また、`filepath.Abs` で絶対パスに変換し、意図しないディレクトリへのアクセスを防ぐ。
-   **シンボリックリンク:** `os.Lstat` を使用して、操作対象がシンボリックリンクでないことを確認する。シンボリックリンクの場合はエラーを返し、TOCTOU攻撃のリスクを低減する。
-   **エラーハンドリング:** ファイルが存在しない、権限がない等のI/Oエラーは、詳細な情報を含むカスタムエラーとしてラップし、呼び出し元に返す。
