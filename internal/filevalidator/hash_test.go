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

// CollidingHashAlgorithm is a test implementation of HashAlgorithm that always returns the same hash.
// This is used to test hash collision scenarios.
type CollidingHashAlgorithm struct {
	// The fixed hash value to return for all inputs
	fixedHash string
	// The fixed name to return for the hash file
	fixedName string
}

// NewCollidingHashAlgorithm creates a new CollidingHashAlgorithm that always returns the given hash.
// If name is empty, it defaults to "colliding".
func NewCollidingHashAlgorithm(hash string) *CollidingHashAlgorithm {
	return &CollidingHashAlgorithm{
		fixedHash: hash,
		fixedName: "colliding",
	}
}

// WithName sets the name of the hash algorithm and returns the receiver for chaining.
func (c *CollidingHashAlgorithm) WithName(name string) *CollidingHashAlgorithm {
	c.fixedName = name
	return c
}

// Name returns the fixed algorithm name.
func (c *CollidingHashAlgorithm) Name() string {
	return c.fixedName
}

// Sum always returns the fixed hash value, regardless of input.
func (c *CollidingHashAlgorithm) Sum(_ io.Reader) (string, error) {
	return c.fixedHash, nil
}
