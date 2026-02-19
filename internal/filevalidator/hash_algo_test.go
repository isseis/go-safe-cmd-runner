package filevalidator

import (
	"io"
	"strings"
)

// MockHashAlgorithm is a test implementation of HashAlgorithm that returns a fixed hash value.
// The hash is simply the first 64 characters of the input, padded with '0' if needed.
// This allows us to simulate hash collisions by using the same prefix for different inputs.
type MockHashAlgorithm struct{}

// Name returns the algorithm name "mock".
func (m *MockHashAlgorithm) Name() string {
	return "mock"
}

// Sum returns a "hash" that is simply the first 64 characters of the input.
// If the input is shorter than 64 characters, it's padded with '0'.
func (m *MockHashAlgorithm) Sum(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	// Use the first 64 bytes as the hash, or pad with '0' if shorter
	hash := string(b)
	if len(hash) > 64 {
		hash = hash[:64]
	} else {
		hash += strings.Repeat("0", 64-len(hash))
	}
	return hash, nil
}
