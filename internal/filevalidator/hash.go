package filevalidator

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// HashAlgorithm defines the behavior of a hash calculation algorithm.
// It allows for efficient streaming processing by accepting an io.Reader.
type HashAlgorithm interface {
	// Name returns the name of the algorithm (e.g., "sha256").
	// This name is used as the file extension for hash files.
	Name() string

	// Sum calculates the hash value of the data read from r and returns it as a hexadecimal string.
	Sum(r io.Reader) (string, error)
}

// SHA256 implements the HashAlgorithm interface for SHA-256 hash calculations.
type SHA256 struct{}

// Name returns the algorithm name "sha256".
func (s *SHA256) Name() string {
	return "sha256"
}

// Sum calculates the SHA-256 hash value of the data read from r.
func (s *SHA256) Sum(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
