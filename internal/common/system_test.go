//go:build test

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHostname(t *testing.T) {
	// GetHostname should always return a non-empty string
	hostname := GetHostname()
	assert.NotEmpty(t, hostname, "GetHostname should return a non-empty string")

	// In normal environments, it should not return the fallback value
	// However, we cannot guarantee this in all test environments
	// so we just verify it returns something
	t.Logf("Hostname: %s", hostname)
}
