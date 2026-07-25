package environment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsForbiddenEnvVar_Prefix(t *testing.T) {
	cases := []string{
		"LD_PRELOAD",
		"LD_LIBRARY_PATH",
		"LD_AUDIT",
		"DYLD_INSERT_LIBRARIES",
		"DYLD_LIBRARY_PATH",
		"BASH_FUNC_foo%%",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			assert.True(t, IsForbiddenEnvVar(name))
		})
	}
}

func TestIsForbiddenEnvVar_Exact(t *testing.T) {
	// Range over the package's own exact-match list so that adding an entry there
	// automatically extends this test's coverage.
	for name := range forbiddenEnvVarExact {
		t.Run(name, func(t *testing.T) {
			assert.True(t, IsForbiddenEnvVar(name))
		})
	}
}

func TestIsForbiddenEnvVar_NonMatch(t *testing.T) {
	cases := []string{
		"",
		"PATH",
		"HOME",
		"USER",
		"LANG",
		"TZ",
		"TERM",
		"LANGUAGE",
		"LDFLAGS",       // looks like it has the LD_ prefix but does not
		"LD",            // bare prefix name, must not match via a buggy HasPrefix(name, "LD")
		"DYLDFOO",       // near-miss of the DYLD_ prefix
		"DYLD",          // bare prefix name
		"BASH_FUNCTION", // near-miss of the BASH_FUNC_ prefix
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			assert.False(t, IsForbiddenEnvVar(name))
		})
	}
}

func TestIsForbiddenEnvVar_CaseSensitive(t *testing.T) {
	assert.False(t, IsForbiddenEnvVar("ld_preload"))
	assert.True(t, IsForbiddenEnvVar("LD_PRELOAD"))
	assert.False(t, IsForbiddenEnvVar("glibc_tunables"))
	assert.True(t, IsForbiddenEnvVar("GLIBC_TUNABLES"))
}
