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

// liveIdentityAPIs matches the live-identity calls that the axis-2 zoning code must
// never make: the judgment consumes only the precomputed RunAsIdent injected at
// construction, so reading the live process identity would make the verdict depend
// on the live euid / $HOME and diverge between dry-run and runtime. The pattern is a
// non-exhaustive denylist of the concrete getters (os/syscall/unix uid/gid/euid/egid
// /groups) and the os/user database lookups -- a regression guardrail, not a
// completeness proof.
var liveIdentityAPIs = regexp.MustCompile(
	`os\.Get(euid|uid|gid|egid|groups)|user\.(Current|Lookup)|syscall\.Get(euid|uid|gid|egid|groups)|unix\.Get(euid|uid|gid|egid|groups)`,
)

// zoningGuardedFiles are the axis-2 classification sources that must stay free of
// live-identity reads. Paths are relative to this package directory (go test runs in
// the package directory).
var zoningGuardedFiles = []string{
	"../security/destination_zoning.go",
	"../security/operand_path_resolver.go",
}

// TestNoLiveIdentityInZoning is the static guard for the identity-purity contract:
// the axis-2 classification code reads no live process identity. A positive control
// asserts the pattern actually matches known-bad calls, so a silently-broken pattern
// -- which would pass vacuously (a fail-open) -- is itself caught. The guarded files
// are required to exist and be non-empty so a rename cannot void the guard silently.
func TestNoLiveIdentityInZoning(t *testing.T) {
	// Positive control: the pattern must match each known-bad form.
	for _, bad := range []string{
		"os.Geteuid()", "os.Getuid()", "os.Getgid()", "os.Getgroups()",
		"syscall.Getegid()", "unix.Getgroups()", "user.Current()", "user.LookupGroup(name)",
	} {
		assert.Regexp(t, liveIdentityAPIs, bad, "positive control: pattern must match %q", bad)
	}
	// Negative control: a legitimate precomputed-identity reference must not match.
	assert.NotRegexp(t, liveIdentityAPIs, "input.RunAsIdent.UID", "the precomputed identity reference is allowed")

	for _, path := range zoningGuardedFiles {
		src, err := os.ReadFile(path)
		require.NoErrorf(t, err, "guarded file must exist (a move/rename must not silently void this guard): %s", path)
		require.NotEmptyf(t, src, "guarded file must be non-empty: %s", path)

		var hits []string
		for i, line := range strings.Split(string(src), "\n") {
			if liveIdentityAPIs.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
			}
		}
		assert.Emptyf(t, hits, "axis-2 zoning code must not read live identity:\n%s", strings.Join(hits, "\n"))
	}
}
