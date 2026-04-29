//go:build test

package security

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBinaryAnalyzer_PanicOnEmptyGOOS(t *testing.T) {
	assert.Panics(t, func() {
		_ = NewBinaryAnalyzer("")
	})
}

func TestNewBinaryAnalyzer_AcceptCurrentGOOS(t *testing.T) {
	assert.NotPanics(t, func() {
		_ = NewBinaryAnalyzer(runtime.GOOS)
	})
}
