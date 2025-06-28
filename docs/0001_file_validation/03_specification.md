# 詳細仕様書：ファイル改ざん検出

## 3.1. 概要

このドキュメントは、`02_architecture.md`で定義された設計方針に基づき、ファイル改ざん検出ライブラリの各コンポーネントの具体的な仕様を定義する。

## 3.2. パッケージ構成

-   パッケージ名: `filevalidator`
-   配置場所: `internal/filevalidator`

## 3.3. データ構造とインターフェース

### 3.3.1. `Validator` 構造体

`validator.go` に定義する。

```go
package filevalidator

import "io"

// Validator は、ファイルのハッシュ値を記録・検証する機能を提供する。
// 外部から直接インスタンス化せず、NewValidatorファクトリ関数を通じて生成する。
type Validator struct {
	algorithm HashAlgorithm
	hashDir   string
}
```

### 3.3.2. `HashAlgorithm` インターフェース

`hash.go` に定義する。

```go
package filevalidator

import "io"

// HashAlgorithm は、ハッシュ計算アルゴリズムの振る舞いを定義するインターフェース。
io.Reader を受け取ることで、メモリ効率の良いストリーミング処理を可能にする。
type HashAlgorithm interface {
	// Name は、アルゴリズムの名前（例: "sha256"）を返す。
	// この名前はハッシュファイルの拡張子として使用される。
	Name() string

	// Sum は、r から読み取ったデータのハッシュ値を計算し、16進数文字列として返す。
	Sum(r io.Reader) (string, error)
}
```

### 3.3.3. `SHA256` 構造体

`hash.go` に定義する。

```go
package filevalidator

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// SHA256 は、HashAlgorithmインターフェースを実装し、SHA-256ハッシュ計算を行う。
type SHA256 struct{}

// Name はアルゴリズム名 "sha256" を返す。
func (s *SHA256) Name() string {
	return "sha256"
}

// Sum は、r から読み取ったデータのSHA-256ハッシュ値を計算する。
func (s *SHA256) Sum(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err // エラーはラップして返す
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
```

## 3.4. 公開API仕様

`validator.go` に定義する。

### 3.4.1. `New` 関数

```go
// New は、指定されたハッシュアルゴリズムで初期化された新しいValidatorを返す。
// algorithmがnilの場合はエラーを返す。
func New(algorithm HashAlgorithm) (*Validator, error) {
    // 実装の詳細
}
```

### 3.4.2. `Validator.Record` メソッド

```go
// Record は、指定されたfilePathのファイルのハッシュ値を計算し、
// `[filePath].[algorithm_name]` という名前のファイルに保存する。
//
// 以下のエラーを返す可能性がある:
// - ErrInvalidFilePath: パスが無効な場合
// - ErrIsSymlink: パスがシンボリックリンクの場合
// - ファイルI/Oに関する各種エラー
func (v *Validator) Record(filePath string) error {
    // 実装の詳細
}
```

### 3.4.3. `Validator.Verify` メソッド

```go
// Verify は、指定されたfilePathのファイルが、記録されたハッシュ値と
// 一致するかどうかを検証する。
//
// 以下のエラーを返す可能性がある:
// - ErrMismatch: ハッシュ値が一致しない場合
// - ErrHashFileNotFound: ハッシュファイルが見つからない場合
// - ErrInvalidFilePath: パスが無効な場合
// - ErrIsSymlink: パスがシンボリックリンクの場合
// - ファイルI/Oに関する各種エラー
func (v *Validator) Verify(filePath string) error {
    // 実装の詳細
}
```

## 3.5. エラー仕様

`errors.go` に定義する。

```go
package filevalidator

import "errors"

var (
	// ErrMismatch は、検証時にハッシュ値が一致しなかったことを示す。
	ErrMismatch = errors.New("file content does not match the recorded hash")

	// ErrHashFileNotFound は、検証対象のハッシュファイルが見つからないことを示す。
	ErrHashFileNotFound = errors.New("hash file not found")

	// ErrInvalidFilePath は、指定されたファイルパスが無効であることを示す。
	ErrInvalidFilePath = errors.New("invalid file path")

	// ErrIsSymlink は、指定されたパスが許容されないシンボリックリンクであることを示す。
	ErrIsSymlink = errors.New("path is a symbolic link")

	// ErrNilAlgorithm は、Validatorの初期化時にアルゴリズムがnilであることを示す。
	ErrNilAlgorithm = errors.New("algorithm cannot be nil")
)
```

## 3.6. 内部ヘルパー関数

`validator.go` 内に、非公開のヘルパー関数を定義する。

-   `getHashFilePath(filePath string) string`: 検証対象のファイルパスから、対応するハッシュファイルのパスを生成する。
-   `validatePath(filePath string) (string, error)`: パスの正規化、絶対パスへの変換、シンボリックリンクのチェックなど、パスに関する一連の検証を行う。
-   `calculateHash(filePath string) (string, error)`: 指定されたファイルのハッシュ値を計算する。
