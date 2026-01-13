//go:build test

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVerifiedTemplateFileLoader_NilManager(t *testing.T) {
	// Should panic when manager is nil
	assert.Panics(t, func() {
		NewVerifiedTemplateFileLoader(nil)
	})
}

func TestVerifiedTemplateFileLoader_PanicOnNilManager(t *testing.T) {
	// Verify that the panic message is clear
	defer func() {
		if r := recover(); r != nil {
			assert.Equal(t, "verification.Manager cannot be nil", r)
		}
	}()

	NewVerifiedTemplateFileLoader(nil)
	t.Error("Expected panic")
}
