package security

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// NewPermissiveTestConfig creates a config with relaxed permissions for specific tests
// This function is only available in test builds and should be used sparingly
func NewPermissiveTestConfig() *Config {
	config := DefaultConfig()
	config.testPermissiveMode = true
	return config
}

// NewSkipHashValidationTestConfig creates a config that skips hash validation for tests
// This function is only available in test builds and should be used when hash validation
// would prevent test execution
func NewSkipHashValidationTestConfig() *Config {
	config := DefaultConfig()
	config.testSkipHashValidation = true
	return config
}

func TestConfig_GetSuspiciousFilePatterns(t *testing.T) {
	tests := []struct {
		name                       string
		outputCriticalPathPatterns []string
		expectedPatterns           []string
		description                string
	}{
		{
			name: "known_critical_patterns",
			outputCriticalPathPatterns: []string{
				// Specific critical system files
				"/etc/passwd", "/etc/shadow", "/etc/sudoers",
				// Critical system directories (should be excluded)
				"/boot/", "/sys/", "/proc/", "/root/",
				"/etc/", "/usr/bin/", "/usr/sbin/",
				"/bin/", "/sbin/", "/lib/", "/lib64/",
				// SSH and authentication files
				"authorized_keys", "id_rsa", "id_ed25519",
				".ssh/", "private_key", "secret_key",
				// Shell configuration files
				".bashrc", ".zshrc", ".login", ".profile",
				// Security-sensitive application configs
				".gnupg/", ".aws/credentials", ".kube/config", ".docker/config.json",
				// Cryptocurrency and keystore files
				"wallet.dat", "keystore",
			},
			expectedPatterns: []string{
				".aws/credentials", ".bashrc", ".docker/config.json", ".kube/config",
				".login", ".profile", ".zshrc", "authorized_keys", "id_ed25519",
				"id_rsa", "keystore", "passwd", "private_key", "secret_key",
				"shadow", "sudoers", "wallet.dat",
			},
			description: "Should extract file patterns from known critical path patterns",
		},
		{
			name: "absolute_paths_extraction",
			outputCriticalPathPatterns: []string{
				"/etc/passwd", "/etc/shadow", "/usr/bin/sudo",
				"/root/.ssh/id_rsa", "/home/user/.bashrc",
			},
			expectedPatterns: []string{
				".bashrc", "id_rsa", "passwd", "shadow", "sudo",
			},
			description: "Should extract base filenames from absolute paths",
		},
		{
			name: "relative_paths_and_filenames",
			outputCriticalPathPatterns: []string{
				"authorized_keys", "id_ed25519", "private_key",
				".env", "wallet.dat",
			},
			expectedPatterns: []string{
				".env", "authorized_keys", "id_ed25519", "private_key", "wallet.dat",
			},
			description: "Should include relative paths and filenames as-is",
		},
		{
			name: "directory_patterns_excluded",
			outputCriticalPathPatterns: []string{
				"/etc/", "/usr/bin/", "authorized_keys",
				".ssh/", "/root/", "id_rsa",
			},
			expectedPatterns: []string{
				"authorized_keys", "id_rsa",
			},
			description: "Should exclude directory patterns ending with '/'",
		},
		{
			name: "mixed_patterns",
			outputCriticalPathPatterns: []string{
				"/etc/passwd", "/etc/shadow", // absolute paths
				"authorized_keys", "id_rsa", // relative filenames
				"/usr/bin/", "/etc/", // directories (should be excluded)
				"/var/log/secure", // absolute path to file
				".env",            // dotfile
			},
			expectedPatterns: []string{
				".env", "authorized_keys", "id_rsa", "passwd", "secure", "shadow",
			},
			description: "Should handle mixed pattern types correctly",
		},
		{
			name:                       "empty_patterns",
			outputCriticalPathPatterns: []string{},
			expectedPatterns:           []string{},
			description:                "Should return empty list for empty input",
		},
		{
			name: "duplicate_basenames",
			outputCriticalPathPatterns: []string{
				"/etc/passwd", "/usr/local/etc/passwd", // both resolve to "passwd"
				"/home/user/.bashrc", "/root/.bashrc", // both resolve to ".bashrc"
				"id_rsa", "/root/.ssh/id_rsa", // duplicate "id_rsa"
			},
			expectedPatterns: []string{
				".bashrc", "id_rsa", "passwd",
			},
			description: "Should deduplicate patterns with same basename",
		},
		{
			name: "nested_paths",
			outputCriticalPathPatterns: []string{
				"/etc/systemd/system/important.service",
				"/home/user/.config/app/config.yaml",
				"/var/lib/app/secrets/key.pem",
			},
			expectedPatterns: []string{
				"config.yaml", "important.service", "key.pem",
			},
			description: "Should extract basenames from deeply nested paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Always create config with test-specific patterns
			config := &Config{
				OutputCriticalPathPatterns: tt.outputCriticalPathPatterns,
			}

			result := config.GetSuspiciousFilePatterns()

			// Handle comparison between empty slice and nil, or use reflect.DeepEqual for other cases
			if len(result) != len(tt.expectedPatterns) || (len(result) > 0 && !reflect.DeepEqual(result, tt.expectedPatterns)) {
				t.Errorf("GetSuspiciousFilePatterns() = %v, want %v", result, tt.expectedPatterns)
				t.Errorf("Description: %s", tt.description)
			}

			// Verify result is sorted
			for i := 1; i < len(result); i++ {
				if result[i-1] > result[i] {
					t.Errorf("Result is not sorted: %v", result)
					break
				}
			}
		})
	}
}

func TestConfig_GetSuspiciousFilePatterns_Invariants(t *testing.T) {
	// Test with a known set of patterns to verify algorithm behavior
	testConfig := &Config{
		OutputCriticalPathPatterns: []string{
			"/etc/passwd", "/etc/shadow", "/etc/sudoers",
			"authorized_keys", "id_rsa", "id_ed25519",
			".ssh/", ".gnupg/", // directories should be excluded
			"/var/log/secure", "/usr/bin/vim",
			"duplicate_file", "/path/to/duplicate_file", // test deduplication
		},
	}

	patterns := testConfig.GetSuspiciousFilePatterns()

	// Verify expected core patterns are present
	expectedPatterns := []string{"passwd", "shadow", "authorized_keys", "id_rsa"}
	for _, expected := range expectedPatterns {
		found := false
		for _, pattern := range patterns {
			if pattern == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected pattern %q not found in suspicious patterns: %v", expected, patterns)
		}
	}

	// Verify patterns are not empty
	assert.NotEmpty(t, patterns, "GetSuspiciousFilePatterns() returned empty list")

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, pattern := range patterns {
		assert.False(t, seen[pattern], "Duplicate pattern found: %q", pattern)
		seen[pattern] = true
	}

	// Verify result is sorted
	for i := 1; i < len(patterns); i++ {
		if patterns[i-1] > patterns[i] {
			assert.Fail(t, "Result is not sorted", "patterns: %v", patterns)
			break
		}
	}

	// Verify directories are excluded
	for _, pattern := range patterns {
		assert.NotContains(t, []string{".ssh", ".gnupg"}, pattern, "Directory pattern %q should have been excluded", pattern)
	}

	// Verify deduplication works
	duplicateCount := 0
	for _, pattern := range patterns {
		if pattern == "duplicate_file" {
			duplicateCount++
		}
	}
	if duplicateCount != 1 {
		t.Errorf("Expected exactly 1 occurrence of 'duplicate_file', got %d", duplicateCount)
	}
}
