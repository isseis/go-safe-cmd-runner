//go:build test

package security

// NewPermissiveTestConfig creates a config with relaxed permissions for specific tests
// This function is only available in test builds and should be used sparingly
func NewPermissiveTestConfig() *Config {
	config := DefaultConfig()
	config.testPermissiveMode = true
	return config
}

// NewTestConfigWithSkipHashValidation creates a config that skips hash validation for tests
// This function is only available in test builds and should be used when hash validation
// would prevent test execution
func NewTestConfigWithSkipHashValidation() *Config {
	config := DefaultConfig()
	config.testSkipHashValidation = true
	return config
}

// NewFullyPermissiveTestConfig creates a config with both relaxed permissions and skipped hash validation
// This function is only available in test builds and should be used very sparingly
func NewFullyPermissiveTestConfig() *Config {
	config := DefaultConfig()
	config.testPermissiveMode = true
	config.testSkipHashValidation = true
	return config
}
