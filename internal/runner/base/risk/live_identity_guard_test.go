//go:build test

package risk

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveIdentityAPIs matches the live-identity and ambient-environment calls that the
// axis-2 zoning code must never make: the judgment consumes only the precomputed
// RunAsIdent injected at construction, so reading the live process identity or the
// environment ($HOME and friends) would make the verdict depend on live euid / env
// and diverge between dry-run and runtime. The denylist covers the concrete getters
// (os/syscall/unix uid/gid/euid/egid/groups), the os/user database lookups, and the
// environment readers (os/syscall/unix Getenv/LookupEnv/Environ and the os.User*Dir
// / ExpandEnv helpers). The trailing `\s*\(` requires an actual call, so a mention
// in a comment or string (e.g. documenting that an API is forbidden) is not a false
// positive, while still catching a call split across lines. It is a non-exhaustive
// regression guardrail, not a completeness proof.
var liveIdentityAPIs = regexp.MustCompile(
	`(os\.Get(euid|uid|gid|egid|groups)|user\.(Current|Lookup\w*)|(syscall|unix)\.Get(euid|uid|gid|egid|groups)|os\.(Getenv|LookupEnv|UserHomeDir|Environ|ExpandEnv|UserConfigDir|UserCacheDir)|(syscall|unix)\.(Environ|Getenv))\s*\(`,
)

// zoningGuardedFiles are the axis-2 classification sources that must stay free of
// live-identity reads. Paths are relative to this package directory (go test runs in
// the package directory). destination_zoning_spec.go is included because its command
// specs, operand extractors, and operation floors are core classification logic.
var zoningGuardedFiles = []string{
	"../security/destination_zoning.go",
	"../security/destination_zoning_spec.go",
	"../security/operand_path_resolver.go",
}

// TestNoLiveIdentityInZoning is the static guard for the identity-purity contract:
// the axis-2 classification code reads no live process identity. A positive control
// asserts the pattern actually matches known-bad calls, so a silently-broken pattern
// -- which would pass vacuously (a fail-open) -- is itself caught. The guarded files
// are required to exist and be non-empty so a rename cannot void the guard silently.
func TestNoLiveIdentityInZoning(t *testing.T) {
	// Positive control: the pattern must match each known-bad call form.
	for _, bad := range []string{
		"os.Geteuid()", "os.Getuid()", "os.Getgid()", "os.Getgroups()",
		"syscall.Getegid()", "unix.Getgroups()", "user.Current()", "user.LookupGroup(name)",
		"os.Getenv(\"HOME\")", "os.UserHomeDir()", "os.LookupEnv(\"HOME\")",
		"os.Environ()", "os.ExpandEnv(\"$HOME\")", "os.UserConfigDir()", "os.UserCacheDir()",
		"syscall.Environ()", "unix.Environ()", "syscall.Getenv(\"HOME\")", "unix.Getenv(\"HOME\")",
	} {
		assert.Regexp(t, liveIdentityAPIs, bad, "positive control: pattern must match %q", bad)
	}
	// Negative controls: a precomputed-identity reference must not match, and a mention
	// of a forbidden API without a call (e.g. in a comment) must not be a false positive.
	assert.NotRegexp(t, liveIdentityAPIs, "input.RunAsIdent.UID", "the precomputed identity reference is allowed")
	assert.NotRegexp(t, liveIdentityAPIs, "// never call os.Getenv here", "a mention without a call is not a violation")

	for _, path := range zoningGuardedFiles {
		src, err := os.ReadFile(path)
		require.NoErrorf(t, err, "guarded file must exist (a move/rename must not silently void this guard): %s", path)
		require.NotEmptyf(t, src, "guarded file must be non-empty: %s", path)

		// The pass/fail decision scans the whole file so a call split across lines
		// cannot evade the guard; the per-line loop only collects readable locations.
		if !liveIdentityAPIs.MatchString(string(src)) {
			continue
		}
		var hits []string
		for i, line := range strings.Split(string(src), "\n") {
			if liveIdentityAPIs.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
			}
		}
		if len(hits) == 0 {
			hits = append(hits, fmt.Sprintf("%s: matched on whole-file scan (the call may span lines)", path))
		}
		assert.Failf(t, "axis-2 zoning code must not read live identity or environment",
			"%s", strings.Join(hits, "\n"))
	}
}
