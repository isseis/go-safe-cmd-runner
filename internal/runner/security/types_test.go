//go:build test

package security

// NewPermissiveTestConfig creates a config with relaxed permissions for specific tests
// This function is only available in test builds and should be used sparingly
func NewPermissiveTestConfig() *Config {
	config := DefaultConfig()
	config.testPermissiveMode = true
	return config
}
