# 詳細仕様書：ファイル改ざん検出（現行実装版）

## 1. 概要

このドキュメントは、現在実装されているファイル改ざん検出ライブラリの各コンポーネントの具体的な仕様を定義する。実装は `02_architecture.md` で定義された設計方針に基づき、セキュリティを重視した堅牢な設計となっている。

## 2. パッケージ構成

- パッケージ名: `filevalidator`
- 配置場所: `internal/filevalidator`

### 2.1. ディレクトリ構成

```
internal/
└── filevalidator/
    ├── validator.go                // Validator構造体と主要メソッド（Record, Verify等）
    ├── hash.go                     // HashAlgorithmインターフェースとSHA256実装
    ├── errors.go                   // カスタムエラーの定義
    ├── validator_helper.go         // SafeReadFile, SafeWriteFile等のセキュリティ機能
    ├── nofollow_error.go           // Unix系OS用のO_NOFOLLOWエラー処理
    ├── nofollow_error_netbsd.go    // NetBSD用のO_NOFOLLOWエラー処理
    ├── validator_test.go           // 主要機能の単体テスト
    ├── hash_test.go                // ハッシュ関連のテスト
    ├── validator_helper_test.go    // セキュリティ機能のテスト
    ├── validator_error_test.go     // エラーハンドリングのテスト
    └── nofollow_error_*_test.go    // プラットフォーム別エラー処理テスト
```

## 3. データ構造とインターフェース

### 3.1. `Validator` 構造体

`validator.go` に定義されている。

```go
// Validator は、ファイルのハッシュ値を記録・検証する機能を提供する。
// 外部から直接インスタンス化せず、Newファクトリ関数を通じて生成する。
type Validator struct {
	algorithm          HashAlgorithm
	hashDir            string
	hashFilePathGetter HashFilePathGetter
}
```

### 3.2. `HashAlgorithm` インターフェース

`hash.go` に定義されている。

```go
// HashAlgorithm は、ハッシュ計算アルゴリズムの振る舞いを定義するインターフェース。
// io.Reader を受け取ることで、メモリ効率の良いストリーミング処理を可能にする。
type HashAlgorithm interface {
	// Name は、アルゴリズムの名前（例: "sha256"）を返す。
	// この名前はハッシュファイルの拡張子として使用される。
	Name() string

	// Sum は、r から読み取ったデータのハッシュ値を計算し、16進数文字列として返す。
	Sum(r io.Reader) (string, error)
}
```

### 3.3. `SHA256` 構造体

`hash.go` に定義されている。

```go
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
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
```

### 3.4. `HashFilePathGetter` インターフェース

`validator.go` に定義されている。

```go
// HashFilePathGetter は、ファイルのハッシュファイルパスを取得するためのインターフェース。
// テスト時にハッシュ衝突ロジックをテストするために使用される。
type HashFilePathGetter interface {
	// GetHashFilePath は、指定されたファイルのハッシュファイルパスを返す。
	GetHashFilePath(hashAlgorithm HashAlgorithm, hashDir string, filePath string) (string, error)
}
```

### 3.5. `ProductionHashFilePathGetter` 構造体

`validator.go` に定義されている。

```go
// ProductionHashFilePathGetter は、本番環境用のハッシュファイルパス生成実装。
type ProductionHashFilePathGetter struct{}

// GetHashFilePath は、ファイルパスのSHA-256ハッシュから一意なハッシュファイルパスを生成する。
func (p *ProductionHashFilePathGetter) GetHashFilePath(hashAlgorithm HashAlgorithm, hashDir string, filePath string) (string, error) {
	if hashAlgorithm == nil {
		return "", ErrNilAlgorithm
	}

	targetPath, err := validatePath(filePath)
	if err != nil {
		return "", err
	}

	h := sha256.Sum256([]byte(targetPath))
	hashStr := base64.URLEncoding.EncodeToString(h[:])

	return filepath.Join(hashDir, hashStr[:12]+"."+hashAlgorithm.Name()), nil
}
```

### 3.6. ファイルシステムインターフェース

`validator_helper.go` に定義されている。

```go
// FileSystem は、ファイルシステム操作を抽象化するインターフェース。
type FileSystem interface {
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
}

// File は、ファイル操作を抽象化するインターフェース。
type File interface {
	Write(b []byte) (n int, err error)
	Close() error
	Stat() (os.FileInfo, error)
}

// osFS は、実際のOSファイルシステムを使用する実装。
type osFS struct{}

func (osFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	// #nosec G304 - パスは開いた後で検証することでTOCTOU攻撃を防ぐ
	return os.OpenFile(name, flag, perm)
}
```

`validator.go` に定義されている。

```go
// ProductionHashFilePathGetter は、HashFilePathGetterの本番環境用実装。
type ProductionHashFilePathGetter struct{}

// GetHashFilePath は、指定されたファイルのハッシュファイルパスを返す。
// シンプルなハッシュ関数を使用してハッシュファイルパスを生成する。
func (p *ProductionHashFilePathGetter) GetHashFilePath(hashAlgorithm HashAlgorithm, hashDir string, filePath string) (string, error) {
	if hashAlgorithm == nil {
		return "", ErrNilAlgorithm
	}

	targetPath, err := validatePath(filePath)
	if err != nil {
		return "", err
	}

	h := sha256.Sum256([]byte(targetPath))
	hashStr := base64.URLEncoding.EncodeToString(h[:])

	return filepath.Join(hashDir, hashStr[:12]+"."+hashAlgorithm.Name()), nil
}
```

## 4. 公開API仕様

### 4.1. `New` 関数

`validator.go` に定義されている。

```go
// New は、指定されたハッシュアルゴリズムとハッシュファイルの保存先ディレクトリで
// 初期化された新しいValidatorを返す。
// hashDirは存在するディレクトリでなければならない。
// algorithmがnilの場合はエラーを返す。
func New(algorithm HashAlgorithm, hashDir string) (*Validator, error) {
	return newValidator(algorithm, hashDir, &ProductionHashFilePathGetter{})
}

// newValidator は、テスト用の内部関数。
func newValidator(algorithm HashAlgorithm, hashDir string, hashFilePathGetter HashFilePathGetter) (*Validator, error) {
	if algorithm == nil {
		return nil, ErrNilAlgorithm
	}

	hashDir, err := filepath.Abs(hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for hash directory: %w", err)
	}

	// ハッシュディレクトリの存在とディレクトリかどうかを確認
	info, err := os.Stat(hashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrHashDirNotExist, hashDir)
		}
		return nil, fmt.Errorf("failed to access hash directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrHashPathNotDir, hashDir)
	}

	return &Validator{
		algorithm:          algorithm,
		hashDir:            hashDir,
		hashFilePathGetter: hashFilePathGetter,
	}, nil
}
```

	// ハッシュディレクトリの存在確認
	info, err := os.Stat(hashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrHashDirNotExist, hashDir)
		}
		return nil, fmt.Errorf("failed to access hash directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrHashPathNotDir, hashDir)
	}

	return &Validator{
		algorithm:          algorithm,
		hashDir:            hashDir,
		hashFilePathGetter: hashFilePathGetter,
	}, nil
}
```

### 4.2. `Validator.Record` メソッド

```go
// Record は、指定されたfilePathのファイルのハッシュ値を計算し、
// ハッシュファイル保存先ディレクトリに保存する。
//
// 処理手順:
// 1. ファイルパスの検証
// 2. ファイルのハッシュ値計算
// 3. ハッシュファイルパスの取得
// 4. ハッシュファイルディレクトリの作成
// 5. 既存ハッシュファイルの衝突チェック
// 6. ハッシュファイルの書き込み
//
// 以下のエラーを返す可能性がある:
// - ErrInvalidFilePath: パスが無効な場合
// - ErrIsSymlink: パスがシンボリックリンクの場合
// - ErrHashCollision: ハッシュ衝突が検出された場合
// - ファイルI/Oに関する各種エラー
func (v *Validator) Record(filePath string) error {
	// ファイルパスの検証
	targetPath, err := validatePath(filePath)
	if err != nil {
		return err
	}

	// ファイルのハッシュ値計算
	hash, err := v.calculateHash(targetPath)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// ハッシュファイルパスの取得
	hashFilePath, err := v.GetHashFilePath(targetPath)
	if err != nil {
		return err
	}

	// ディレクトリの確保
	if err := os.MkdirAll(filepath.Dir(hashFilePath), 0o755); err != nil {
		return fmt.Errorf("failed to create hash directory: %w", err)
	}

	// 既存ハッシュファイルの衝突チェック
	if existingContent, err := SafeReadFile(hashFilePath); err == nil {
		parts := strings.SplitN(string(existingContent), "\n", 2)
		if len(parts) == 0 {
			return fmt.Errorf("%w: empty file", ErrInvalidHashFileFormat)
		}
		recordedPath := parts[0]
		if recordedPath != targetPath {
			return fmt.Errorf("%w: path '%s' conflicts with existing path '%s'", ErrHashCollision, targetPath, recordedPath)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check existing hash file: %w", err)
	}

	// ハッシュファイルの書き込み
	return SafeWriteFile(hashFilePath, fmt.Appendf(nil, "%s\n%s", targetPath, hash), 0o644)
}
```

### 4.3. `Validator.Verify` メソッド

```go
// Verify は、指定されたfilePathのファイルが、記録されたハッシュ値と
// 一致するかどうかを検証する。
//
// 以下のエラーを返す可能性がある:
// - ErrMismatch: ハッシュ値が一致しない場合
// - ErrHashFileNotFound: ハッシュファイルが見つからない場合
// - ErrInvalidFilePath: パスが無効な場合
// - ErrIsSymlink: パスがシンボリックリンクの場合
// - ErrHashCollision: 記録されたパスと現在のパスが一致しない場合
// - ファイルI/Oに関する各種エラー
func (v *Validator) Verify(filePath string) error {
	// ファイルパスの検証
	targetPath, err := validatePath(filePath)
	if err != nil {
		return err
	}

	// 現在のハッシュ値計算
	actualHash, err := v.calculateHash(targetPath)
	if os.IsNotExist(err) {
		return err
	} else if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	// ハッシュファイルの読み込みと解析
	_, expectedHash, err := v.readAndParseHashFile(targetPath)
	if err != nil {
		return err
	}

	// ハッシュ値の比較
	if expectedHash != actualHash {
		return ErrMismatch
	}

	return nil
}
```

### 4.4. `Validator.GetHashFilePath` メソッド

```go
// GetHashFilePath は、指定されたfilePathに対応するハッシュファイルのパスを返す。
// ハッシュファイル名は以下の手順で生成される:
// 1. 対象ファイルの絶対パスを検証・正規化する
// 2. 絶対パスのSHA-256ハッシュを計算する
// 3. ハッシュ値をURL-safe Base64でエンコードする
// 4. エンコードされた文字列の先頭12文字をファイル名として使用する
// 5. ハッシュアルゴリズムの拡張子（例: ".sha256"）を付与する
//
// 以下のエラーを返す可能性がある:
// - ErrInvalidFilePath: パスが無効な場合
// - ErrIsSymlink: パスがシンボリックリンクの場合
// - ErrNilAlgorithm: アルゴリズムが設定されていない場合
// - ファイルI/Oに関する各種エラー
func (v *Validator) GetHashFilePath(filePath string) (string, error) {
	return v.hashFilePathGetter.GetHashFilePath(v.algorithm, v.hashDir, filePath)
}
```

### 4.5. その他のメソッド

```go
// GetHashAlgorithm は、Validatorが使用するハッシュアルゴリズムを返す。
func (v *Validator) GetHashAlgorithm() HashAlgorithm {
	return v.algorithm
}

// GetHashDir は、ハッシュファイルの保存ディレクトリを返す。
func (v *Validator) GetHashDir() string {
	return v.hashDir
}
```

## 5. セキュリティ機能仕様

### 5.1. `SafeReadFile` 関数

`validator_helper.go` に定義されている。

```go
// MaxFileSize は、SafeReadFileの最大許可ファイルサイズ（128 MB）
const MaxFileSize = 128 * 1024 * 1024

// SafeReadFile は、パスの検証とファイルプロパティのチェック後にファイルを安全に読み込む。
// MaxFileSizeの制限を設けてメモリ枯渇攻撃を防ぐ。
// O_NOFOLLOWを使用してシンボリンク攻撃を防ぎ、
// すべてのチェックを原子的に実行する。
func SafeReadFile(filePath string) ([]byte, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// O_NOFOLLOWでファイルを開く
	file, err := os.OpenFile(absPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		switch {
		case isNoFollowError(err):
			return nil, ErrIsSymlink
		default:
			return nil, err
		}
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("error closing file: %v\n", closeErr)
		}
	}()

	// TOCTOU攻撃防止のためディレクトリコンポーネント検証
	if err := verifyPathComponents(absPath); err != nil {
		return nil, err
	}

	// ファイルが通常ファイルであることを確認
	if _, err := validateFile(file, absPath); err != nil {
		return nil, err
	}

	return readFileContent(file, filePath)
}

// readFileContent は、開いたファイルの内容を読み込み、検証する。
func readFileContent(file *os.File, filePath string) ([]byte, error) {
	fileInfo, err := validateFile(file, filePath)
	if err != nil {
		return nil, err
	}

	if fileInfo.Size() > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	content, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if int64(len(content)) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	return content, nil
}
```
### 5.3. `verifyPathComponents` 関数

`validator_helper.go` に定義されている。

```go
// verifyPathComponents は、パスの各コンポーネントがシンボリックリンクでないことをチェックする。
// この関数はファイルを開いた後に呼び出されて、TOCTOU攻撃を防ぐ。
func verifyPathComponents(absPath string) error {
	// ファイルのディレクトリを取得
	dir := filepath.Dir(absPath)
	if dir == "." {
		// カレントディレクトリの場合は作業ディレクトリを取得
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// ディレクトリの絶対パスを取得
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// 各ディレクトリコンポーネントをチェック
	current := dir
	for {
		// 親ディレクトリを取得
		parent := filepath.Dir(current)
		if parent == current {
			break // ルートディレクトリに到達
		}

		// os.Lstatでシンボリックリンクチェック（追跡しない）
		fi, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // ディレクトリが存在しない場合は安全として継続
			}
			return fmt.Errorf("failed to stat %s: %w", current, err)
		}

		// シンボリックリンクかチェック
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrIsSymlink, current)
		}

		current = parent
	}

	return nil
}
```

### 5.4. `validateFile` 関数

`validator_helper.go` に定義されている。

```go
// validateFile は、ファイルが通常ファイルかどうかをチェックし、FileInfoを返す。
// TOCTOU攻撃を防ぐため、ファイルディスクリプタを使用してファイル情報を取得する。
func validateFile(file File, filePath string) (os.FileInfo, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, filePath)
	}

	return fileInfo, nil
}
```

### 5.5. `validatePath` 関数

`validator.go` に定義されている。

```go
// validatePath は、指定されたファイルパスを検証・正規化する。
func validatePath(filePath string) (string, error) {
	if filePath == "" {
		return "", ErrInvalidFilePath
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", err
	}

	// 解決されたパスが通常ファイルかどうかチェック
	fileInfo, err := os.Lstat(resolvedPath)
	if err != nil {
		return "", err
	}
	if !fileInfo.Mode().IsRegular() {
		return "", fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, resolvedPath)
	}
	return resolvedPath, nil
}
```
```

### 5.2. `SafeWriteFile` 関数

`validator_helper.go` に定義されている。

```go
// SafeWriteFile は、パスの検証とファイルプロパティのチェックを行った後、
// ファイルを安全に書き込む。
// すべてのパスコンポーネントがシンボリックリンクでないことをチェックし、
// O_NOFOLLOWを使用してシンボリックリンク攻撃を防ぐ。
// TOCTOU（Time-of-Check Time-of-Use）レース条件に対して安全になるよう、
// ファイルを最初に開いてからパスコンポーネントを検証する設計になっている。
func SafeWriteFile(filePath string, content []byte, perm os.FileMode) (err error) {
	return safeWriteFileWithFS(filePath, content, perm, defaultFS)
}

// safeWriteFileWithFS は、テスト用のFileSystemを受け取る内部実装
func safeWriteFileWithFS(filePath string, content []byte, perm os.FileMode, fs FileSystem) (err error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// O_NOFOLLOW|O_CREATE|O_EXCLを使用してセキュアにファイルを開く
	file, err := fs.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, perm)
	if err != nil {
		switch {
		case os.IsExist(err):
			return ErrFileExists
		case isNoFollowError(err):
			return ErrIsSymlink
		default:
			return fmt.Errorf("failed to open file: %w", err)
		}
	}

	// エラー時にファイルが確実に閉じられるようにする
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// TOCTOU攻撃防止のためディレクトリコンポーネント検証
	if err := verifyPathComponents(absPath); err != nil {
		return err
	}

	// ファイルが通常ファイルであることを確認
	if _, err := validateFile(file, absPath); err != nil {
		return err
	}

	// 内容を書き込み
	if _, err = file.Write(content); err != nil {
		return fmt.Errorf("failed to write to %s: %w", absPath, err)
	}

	return nil
}
```
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// TOCTOU攻撃を防ぐため、ファイルディスクリプタを使用してディレクトリコンポーネントを検証
	if err := verifyPathComponents(absPath); err != nil {
		return err
	}

	// ファイルが通常ファイル（デバイス、パイプ等でない）であることを検証
	if _, err := validateFile(file, absPath); err != nil {
		return err
	}

	// コンテンツの書き込み
	if _, err = file.Write(content); err != nil {
		return fmt.Errorf("failed to write to %s: %w", absPath, err)
	}

	return nil
}
```

### 5.6. プラットフォーム固有のエラー処理

`nofollow_error.go` および `nofollow_error_netbsd.go` に定義されている。

```go
//go:build !netbsd
// +build !netbsd

// isNoFollowError は、O_NOFOLLOWでシンボリックリンクを開こうとした際のエラーかどうかをチェックする。
// Unix系システム（NetBSD以外）では ELOOP と EMLINK エラーを確認する。
func isNoFollowError(err error) bool {
	if pathErr, ok := err.(*os.PathError); ok {
		if errno, ok := pathErr.Err.(syscall.Errno); ok {
			return errno == syscall.ELOOP || errno == syscall.EMLINK
		}
	}
	return false
}
```

```go
//go:build netbsd
// +build netbsd

// NetBSD固有のO_NOFOLLOWエラー処理
// NetBSDではO_NOFOLLOWでシンボリックリンクを開こうとするとEFTYPEエラーが返される。
func isNoFollowError(err error) bool {
	if pathErr, ok := err.(*os.PathError); ok {
		if errno, ok := pathErr.Err.(syscall.Errno); ok {
			return errno == syscall.EFTYPE
		}
	}
	return false
}
```
func isNoFollowError(err error) bool {
	var e *os.PathError
	if !errors.As(err, &e) {
		return false
	}
	return errors.Is(e.Err, syscall.ELOOP) || errors.Is(e.Err, syscall.EMLINK)
}
```

```go
//go:build netbsd

// isNoFollowError は、シンボリックリンクを開こうとしたエラーかどうかをチェックする
func isNoFollowError(err error) bool {
	var e *os.PathError
	if !errors.As(err, &e) {
		return false
	}
	return errors.Is(e.Err, syscall.EFTYPE)
}
```

## 6. エラー仕様

`errors.go` に定義されている13種類のカスタムエラー型。

```go
var (
	// ErrMismatch は、検証時にファイル内容が記録されたハッシュと一致しないことを示す。
	ErrMismatch = errors.New("file content does not match the recorded hash")

	// ErrHashFileNotFound は、検証用のハッシュファイルが見つからないことを示す。
	ErrHashFileNotFound = errors.New("hash file not found")

	// ErrInvalidFilePath は、指定されたファイルパスが無効であることを示す。
	ErrInvalidFilePath = errors.New("invalid file path")

	// ErrIsSymlink は、指定されたパスがシンボリックリンクで許可されないことを示す。
	ErrIsSymlink = errors.New("path is a symbolic link")

	// ErrNilAlgorithm は、Validator初期化時にアルゴリズムがnilであることを示す。
	ErrNilAlgorithm = errors.New("algorithm cannot be nil")

	// ErrHashDirNotExist は、ハッシュディレクトリが存在しないことを示す。
	ErrHashDirNotExist = errors.New("hash directory does not exist")

	// ErrHashPathNotDir は、ハッシュパスがディレクトリでないことを示す。
	ErrHashPathNotDir = errors.New("hash path is not a directory")

	// ErrInvalidHashFileFormat は、ハッシュファイルの形式が無効であることを示す。
	ErrInvalidHashFileFormat = errors.New("invalid hash file format")

	// ErrHashCollision は、ハッシュ衝突が検出されたことを示す。
	ErrHashCollision = errors.New("hash collision detected")

	// ErrInvalidFilePathFormat は、無効なファイルパス形式が提供されたことを示す。
	ErrInvalidFilePathFormat = errors.New("invalid file path format")

	// ErrSuspiciousFilePath は、潜在的に悪意のあるファイルパスが検出されたことを示す。
	ErrSuspiciousFilePath = errors.New("suspicious file path detected")

	// ErrFileTooLarge は、ファイルが大きすぎることを示す（MaxFileSize = 128MB超過）。
	ErrFileTooLarge = errors.New("file too large")

	// ErrFileExists は、ファイルが既に存在することを示す（SafeWriteFileでO_EXCL使用時）。
	ErrFileExists = errors.New("file exists")
)
```

## 7. 内部ヘルパー関数

### 7.1. ハッシュ計算

`validator.go` に定義されている。

```go
// calculateHash は、指定されたパスのファイルのハッシュ値を計算する。
// filePathはvalidatePathによって事前に検証されている必要がある。
func (v *Validator) calculateHash(filePath string) (string, error) {
	content, err := SafeReadFile(filePath)
	if err != nil {
		return "", err
	}
	return v.algorithm.Sum(bytes.NewReader(content))
}
```

### 7.2. ハッシュファイルの読み込みと解析

`validator.go` に定義されている。

```go
// readAndParseHashFile は、ハッシュファイルを読み込み、解析する。
func (v *Validator) readAndParseHashFile(targetPath string) (string, string, error) {
	// ハッシュファイルのパスを取得
	hashFilePath, err := v.GetHashFilePath(targetPath)
	if err != nil {
		return "", "", err
	}

	// 保存されたハッシュファイルを読み込み
	hashFileContent, err := SafeReadFile(hashFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrHashFileNotFound
		}
		return "", "", fmt.Errorf("failed to read hash file: %w", err)
	}

	// ハッシュファイルの内容を解析（形式: "filepath\nhash"）
	parts := strings.SplitN(string(hashFileContent), "\n", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: expected 'path\nhash', got %d parts", ErrInvalidHashFileFormat, len(parts))
	}

	// 記録されたパスが現在のファイルパスと一致するかチェック
	recordedPath := parts[0]
	if recordedPath == "" {
		return "", "", fmt.Errorf("%w: empty path", ErrInvalidHashFileFormat)
	}
	if recordedPath != targetPath {
		return "", "", fmt.Errorf("%w: recorded path '%s' does not match current path '%s'", ErrHashCollision, recordedPath, targetPath)
	}

	expectedHash := parts[1]
	return recordedPath, expectedHash, nil
}
```

## 8. 使用例

### 8.1. 基本的な使用例

```go
package main

import (
	"fmt"
	"log"
	"path/filepath"

	"your-project/internal/filevalidator"
)

func main() {
	// SHA256アルゴリズムでValidatorを初期化
	validator, err := filevalidator.New(&filevalidator.SHA256{}, "/path/to/hash/dir")
	if err != nil {
		log.Fatal(err)
	}

	// ファイルのハッシュを記録
	targetFile := "/path/to/target/file"
	if err := validator.Record(targetFile); err != nil {
		log.Fatal(err)
	}

	// ファイルの検証
	if err := validator.Verify(targetFile); err != nil {
		switch err {
		case filevalidator.ErrMismatch:
			fmt.Println("ファイルが変更されています")
		case filevalidator.ErrHashFileNotFound:
			fmt.Println("ハッシュファイルが見つかりません")
		default:
			log.Fatal(err)
		}
	} else {
		fmt.Println("ファイルは変更されていません")
	}
}
```

### 8.2. エラーハンドリングの例

```go
package main

import (
	"errors"
	"fmt"
	"log"

	"your-project/internal/filevalidator"
)

func handleFileValidation(validator *filevalidator.Validator, filePath string) {
	err := validator.Verify(filePath)
	switch {
	case err == nil:
		fmt.Println("ファイルは安全です")
	case errors.Is(err, filevalidator.ErrMismatch):
		fmt.Println("警告: ファイルが改ざんされています")
	case errors.Is(err, filevalidator.ErrHashFileNotFound):
		fmt.Println("情報: ハッシュが記録されていません。新規記録を検討してください")
	case errors.Is(err, filevalidator.ErrIsSymlink):
		fmt.Println("エラー: シンボリックリンクは許可されていません")
	case errors.Is(err, filevalidator.ErrFileTooLarge):
		fmt.Println("エラー: ファイルサイズが制限を超えています（128MB超過）")
	default:
		log.Printf("予期しないエラー: %v", err)
	}
}
```

### 8.3. セキュアファイル操作の例

```go
package main

import (
	"fmt"
	"log"

	"your-project/internal/filevalidator"
)

func secureFileOperations() {
	// セキュアな読み込み
	data, err := filevalidator.SafeReadFile("/path/to/file")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("読み込んだデータサイズ: %d bytes\n", len(data))

	// セキュアな書き込み
	content := []byte("Hello, secure world!")
	err = filevalidator.SafeWriteFile("/path/to/new/file", content, 0644)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ファイルを安全に書き込みました")
}
```

## 9. 制約事項

### 9.1. ファイルサイズ制限
- `SafeReadFile` は最大128MB（`MaxFileSize`）のファイルサイズ制限がある
- この制限を超えるファイルは `ErrFileTooLarge` エラーを返す

### 9.2. プラットフォーム制約
- Unix系システムでの動作を前提としている
- NetBSDでは特別なエラーコード（EFTYPE）処理が必要
- WindowsでのO_NOFOLLOWサポートは限定的

### 9.3. パス制約
- シンボリックリンクは許可されない（安全上の理由）
- 通常ファイルのみが対象（デバイスファイル、パイプ、ソケット等は除外）
- 空のファイルパスは無効

### 9.4. ハッシュファイル制約
- ハッシュファイル名の衝突が発生した場合、異なるパスのファイルは記録できない
- ハッシュディレクトリは事前に存在している必要がある
- ハッシュファイルの形式は固定（パス + 改行 + ハッシュ値）

## 10. パフォーマンス特性

### 10.1. 計算量
- ファイルハッシュ計算: O(n) （nはファイルサイズ）
- パス検証: O(d) （dはディレクトリ階層の深さ）
- ハッシュファイル操作: O(1)

### 10.2. メモリ使用量
- ファイル読み込み時：最大128MB + バッファ領域
- ハッシュ計算時：ストリーミング処理により一定量のメモリ使用
- パス検証時：パス文字列分のメモリのみ

### 10.3. I/O回数
- Record操作: 対象ファイル読み込み1回 + ハッシュファイル書き込み1回 + 各種Stat操作
- Verify操作: 対象ファイル読み込み1回 + ハッシュファイル読み込み1回 + 各種Stat操作

## 11. セキュリティ保証

### 11.1. 攻撃対策
- **TOCTOU攻撃**: ファイルオープン後のディスクリプタベース検証
- **シンボリックリンク攻撃**: O_NOFOLLOW + ディレクトリ階層チェック
- **パストラバーサル攻撃**: 絶対パス変換 + シンボリックリンク解決
- **DoS攻撃**: ファイルサイズ制限 + LimitReader使用
- **ハッシュ衝突攻撃**: パス情報併記による衝突検出

### 11.2. 保証しないセキュリティ
- ハッシュファイル自体の改ざん検出
- 権限昇格攻撃の防止
- ハッシュファイル自体の暗号化
- ネットワーク攻撃の防止

現在の実装は、ファイル完全性検証に特化した堅牢なセキュリティ機能を提供している。
}
```

## 8. テスト仕様

### 8.1. テスト用モック

- `MockHashAlgorithm`: 固定のハッシュ値を返すテスト用実装
- `CollidingHashAlgorithm`: 常に同じハッシュ値を返す衝突テスト用実装
- `CollidingHashFilePathGetter`: 常に同じパスを返すパス衝突テスト用実装
- 各種ファイルシステムモック（failingFile, failingCloseFS等）

### 8.2. テストカバレッジ

- 正常系のテスト
- エラーケースの網羅的テスト
- セキュリティ機能のテスト
- プラットフォーム固有の機能テスト
- ハッシュ衝突シナリオのテスト
現在の実装は、ファイル完全性検証に特化した堅牢なセキュリティ機能を提供している。
