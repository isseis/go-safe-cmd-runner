//go:build test

//nolint:revive // Package name "common" is intentional for internal common utilities
package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHostname(t *testing.T) {
	t.Run("success case", func(t *testing.T) {
		// GetHostname should always return a non-empty string
		hostname := GetHostname()
		assert.NotEmpty(t, hostname, "GetHostname should return a non-empty string")

		// In normal environments, it should not return the fallback value
		// However, we cannot guarantee this in all test environments
		// so we just verify it returns something
		t.Logf("Hostname: %s", hostname)
	})

	t.Run("failure case - returns fallback on error", func(t *testing.T) {
		// Save original function
		originalOsHostname := osHostname
		defer func() {
			osHostname = originalOsHostname
		}()

		// Mock os.Hostname to return an error
		osHostname = func() (string, error) {
			return "", errors.New("simulated hostname error")
		}

		hostname := GetHostname()
		assert.Equal(t, UnknownHostFallback, hostname, "GetHostname should return fallback on error")
	})

	t.Run("success case with mock - returns actual hostname", func(t *testing.T) {
		// Save original function
		originalOsHostname := osHostname
		defer func() {
			osHostname = originalOsHostname
		}()

		// Mock os.Hostname to return a specific value
		expectedHostname := "test-hostname"
		osHostname = func() (string, error) {
			return expectedHostname, nil
		}

		hostname := GetHostname()
		assert.Equal(t, expectedHostname, hostname, "GetHostname should return the mocked hostname")
	})
}
