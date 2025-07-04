package filevalidator

import (
	"encoding/json"
	"fmt"
	"time"
)

// HashFileFormat はハッシュファイルのJSON形式を定義する
type HashFileFormat struct {
	Version   string    `json:"version"`
	Format    string    `json:"format"`
	Timestamp time.Time `json:"timestamp"`
	File      FileInfo  `json:"file"`
}

// FileInfo はファイル情報を定義する
type FileInfo struct {
	Path string   `json:"path"`
	Hash HashInfo `json:"hash"`
}

// HashInfo はハッシュ情報を定義する
type HashInfo struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

// createHashFileFormat はハッシュファイル形式を作成する
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

// isJSONFormat はコンテンツがJSON形式かどうかを判定する
func isJSONFormat(content []byte) bool {
	// 空白文字をスキップして先頭文字を確認
	for _, b := range content {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

// validateHashFileFormat はハッシュファイルの形式を検証し、解析する
func validateHashFileFormat(content []byte) (HashFileFormat, error) {
	if !isJSONFormat(content) {
		return HashFileFormat{}, ErrInvalidJSONFormat
	}

	var format HashFileFormat
	if err := json.Unmarshal(content, &format); err != nil {
		return HashFileFormat{}, fmt.Errorf("%w: %v", ErrJSONParseError, err)
	}

	return format, nil
}

// validateJSONHashFileFormat はJSON形式のハッシュファイルの内容を検証する
func (v *Validator) validateJSONHashFileFormat(format HashFileFormat, targetPath string) error {
	// バージョン検証
	if format.Version != "1.0" {
		return fmt.Errorf("%w: version %s", ErrUnsupportedVersion, format.Version)
	}

	// フォーマット検証
	if format.Format != "file-hash" {
		return fmt.Errorf("%w: format %s", ErrInvalidJSONFormat, format.Format)
	}

	// ファイルパス検証
	if format.File.Path == "" {
		return fmt.Errorf("%w: empty file path", ErrInvalidJSONFormat)
	}

	// パス一致確認
	if format.File.Path != targetPath {
		return fmt.Errorf("%w: path mismatch", ErrHashCollision)
	}

	// ハッシュアルゴリズム検証
	if format.File.Hash.Algorithm != v.algorithm.Name() {
		return fmt.Errorf("%w: algorithm mismatch", ErrInvalidJSONFormat)
	}

	// ハッシュ値検証
	if format.File.Hash.Value == "" {
		return fmt.Errorf("%w: empty hash value", ErrInvalidJSONFormat)
	}

	return nil
}
