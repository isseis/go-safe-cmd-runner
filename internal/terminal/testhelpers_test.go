package terminal

import (
	"testing"
)

// setupCleanEnv sets up a clean environment for terminal tests by explicitly controlling
// all color-related environment variables and setting only the specified ones. This ensures
// tests are not affected by the actual environment while remaining safe for parallel execution.
func setupCleanEnv(t *testing.T, envVars map[string]string) {
	t.Helper()

	// Variables that use os.LookupEnv() - these need special handling as empty != unset
	existenceCheckedVars := []string{"NO_COLOR"}

	// Variables that use os.Getenv() - empty value is treated as unset
	valueCheckedVars := []string{
		"CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR",
		"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION",
		// CI environment variables
		"CI", "GITHUB_ACTIONS", "JENKINS_URL", "BUILD_NUMBER",
		"CONTINUOUS_INTEGRATION", "TRAVIS", "CIRCLECI", "APPVEYOR", "GITLAB_CI",
	}

	// For variables that check existence (os.LookupEnv), only set if explicitly specified
	for _, v := range existenceCheckedVars {
		if value, specified := envVars[v]; specified {
			t.Setenv(v, value)
		}
		// If not specified, we leave it unset (don't call t.Setenv at all)
	}

	// For variables that check value (os.Getenv), set to empty if not specified
	for _, v := range valueCheckedVars {
		if value, specified := envVars[v]; specified {
			t.Setenv(v, value)
		} else {
			t.Setenv(v, "") // Empty is treated as unset for these variables
		}
	}
}
