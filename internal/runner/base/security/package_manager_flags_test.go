package security

import (
	"os"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSystemModification_DpkgFlags fixes dpkg flag-style detection: install/
// remove/purge (and their long forms) are modifying, while query/list options
// and concatenated-argument or single-dash long forms are not. Short-flag
// matching is case-sensitive on the first character (-i differs from -I).
func TestSystemModification_DpkgFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		// Modifying short options (first character in "irP").
		{"install short", []string{"-i", "pkg.deb"}, true},
		{"remove short", []string{"-r", "nginx"}, true},
		{"purge short", []string{"-P", "nginx"}, true},
		// Modifying long forms.
		{"install long", []string{"--install", "pkg.deb"}, true},
		{"remove long", []string{"--remove", "nginx"}, true},
		{"purge long", []string{"--purge", "nginx"}, true},
		{"unpack long", []string{"--unpack", "pkg.deb"}, true},
		{"configure long", []string{"--configure", "nginx"}, true},
		{"configure all", []string{"--configure", "-a"}, true},
		// Query / read-only short options are not modifying.
		{"list short", []string{"-l"}, false},
		{"listfiles short", []string{"-L", "nginx"}, false},
		{"status short", []string{"-s", "nginx"}, false},
		{"search short", []string{"-S", "/bin/ls"}, false},
		{"print-avail short", []string{"-p", "nginx"}, false},
		{"info short", []string{"-I", "pkg.deb"}, false},
		{"contents short", []string{"-c", "pkg.deb"}, false},
		// Query long forms are not modifying.
		{"info long", []string{"--info", "pkg.deb"}, false},
		{"list long", []string{"--list"}, false},
		{"status long", []string{"--status", "nginx"}, false},
		{"get-selections long", []string{"--get-selections"}, false},
		{"print-avail long", []string{"--print-avail", "nginx"}, false},
		// Boundary: concatenated-argument and single-dash long forms whose first
		// character is not in "irP".
		{"debug level not modifying", []string{"-D2"}, false},
		{"single-dash long list not modifying", []string{"-list"}, false},
		// Case sensitivity: -i (install) is modifying, -I (info) is not.
		{"install lowercase i", []string{"-i", "pkg.deb"}, true},
		{"info uppercase I", []string{"-I", "pkg.deb"}, false},
		// No arguments.
		{"no args", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSystemModification("dpkg", tt.args))
		})
	}
}

// TestSystemModification_RpmFlags fixes rpm flag-style detection: install/
// upgrade/freshen/erase short modes and their long forms are modifying, with
// modifier flags (-v/-h/--nodeps/--force) having no effect; query/verify and
// other read-only long forms are not modifying.
func TestSystemModification_RpmFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		// Modifying short modes (first character in "iUFe").
		{"install short", []string{"-i", "pkg.rpm"}, true},
		{"upgrade short", []string{"-U", "pkg.rpm"}, true},
		{"freshen short", []string{"-F", "pkg.rpm"}, true},
		{"erase short", []string{"-e", "nginx"}, true},
		// Modifier flags concatenated or appended do not change the mode.
		{"install verbose hash", []string{"-ivh", "pkg.rpm"}, true},
		{"upgrade verbose hash", []string{"-Uvh", "pkg.rpm"}, true},
		{"erase nodeps", []string{"-e", "--nodeps", "nginx"}, true},
		{"erase verbose", []string{"-e", "--verbose", "nginx"}, true},
		{"upgrade force", []string{"-U", "--force", "pkg.rpm"}, true},
		// Modifying long forms.
		{"install long", []string{"--install", "pkg.rpm"}, true},
		{"upgrade long", []string{"--upgrade", "pkg.rpm"}, true},
		{"freshen long", []string{"--freshen", "pkg.rpm"}, true},
		{"erase long", []string{"--erase", "nginx"}, true},
		{"reinstall long", []string{"--reinstall", "pkg.rpm"}, true},
		{"import long", []string{"--import", "KEY"}, true},
		{"initdb long", []string{"--initdb"}, true},
		{"rebuilddb long", []string{"--rebuilddb"}, true},
		{"setperms long", []string{"--setperms", "nginx"}, true},
		{"setugids long", []string{"--setugids", "nginx"}, true},
		// Query / verify modes are not modifying.
		{"query info", []string{"-qi", "nginx"}, false},
		{"query all", []string{"-qa"}, false},
		{"query package info", []string{"-qpi", "pkg.rpm"}, false},
		{"query list", []string{"-ql", "nginx"}, false},
		{"verify short", []string{"-V", "nginx"}, false},
		{"query all verbose", []string{"-qa", "--verbose"}, false},
		{"query long", []string{"--query", "nginx"}, false},
		{"verify long", []string{"--verify", "nginx"}, false},
		// Read-only long forms that are not exact matches of a modifying form.
		{"eval long", []string{"--eval", "%{_libdir}"}, false},
		{"querytags long", []string{"--querytags"}, false},
		// Boundary: concatenated-argument short options whose first character is
		// not in "iUFe".
		{"eval macro", []string{"-E%{_libdir}"}, false},
		{"define macro", []string{"-D'enable_foo 1'"}, false},
		// No arguments.
		{"no args", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSystemModification("rpm", tt.args))
		})
	}
}

// TestSystemModification_RpmExcludePriority fixes that any query/verify token
// suppresses detection even when a modifying flag is also present, in both
// short and long forms (least-privilege fail-open for contradictory input).
func TestSystemModification_RpmExcludePriority(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"query then install short", []string{"-q", "-i", "nginx"}},
		{"install then query short", []string{"-i", "-q", "nginx"}},
		{"erase then query short", []string{"-e", "-q", "nginx"}},
		{"install then query long", []string{"--install", "--query", "pkg.rpm"}},
		{"erase then verify long", []string{"--erase", "--verify", "nginx"}},
		{"install then verify short", []string{"-i", "-V", "nginx"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, isSystemModification("rpm", tt.args),
				"a query/verify token must suppress detection")
		})
	}
}

// TestSystemModification_DegenerateTokens fixes that degenerate tokens (a lone
// "-", a lone "--", or an empty string) are neither modifying nor exclude flags
// and never panic. When a degenerate token is the only candidate the command is
// non-modifying; a degenerate token alongside a valid modifying flag does not
// suppress detection (fail-safe is preserved).
func TestSystemModification_DegenerateTokens(t *testing.T) {
	// Sole candidate is a degenerate token -> non-modifying.
	soleDegenerate := []struct {
		name string
		cmd  string
		args []string
	}{
		{"dpkg lone dash", "dpkg", []string{"-"}},
		{"dpkg lone double dash", "dpkg", []string{"--"}},
		{"dpkg empty string", "dpkg", []string{""}},
		{"rpm lone dash", "rpm", []string{"-"}},
		{"pacman lone dash", "pacman", []string{"-"}},
	}
	for _, tt := range soleDegenerate {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, isSystemModification(tt.cmd, tt.args))
		})
	}

	// A degenerate token must not suppress a valid modifying flag.
	withModifying := []struct {
		name string
		cmd  string
		args []string
	}{
		{"dpkg install then empty", "dpkg", []string{"-i", ""}},
		{"dpkg empty then install", "dpkg", []string{"", "-i"}},
		{"rpm upgrade then empty", "rpm", []string{"-U", ""}},
	}
	for _, tt := range withModifying {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, isSystemModification(tt.cmd, tt.args))
		})
	}
}

// TestSystemModification_PacmanFlags fixes pacman regression after the move to
// first-character matching: -S/-R/-U (and combinations) and the long forms are
// modifying, the existing -Ss/-Si over-detection is preserved, queries are not
// modifying, and the only behavioral difference is the non-idiomatic -yS (now
// not detected), which is within the accepted tolerance.
func TestSystemModification_PacmanFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"sync", []string{"-S", "nginx"}, true},
		{"remove", []string{"-R", "nginx"}, true},
		{"upgrade", []string{"-U", "pkg.tar.zst"}, true},
		{"sync refresh upgrade", []string{"-Syu"}, true},
		{"remove recursive", []string{"-Rns", "nginx"}, true},
		{"sync long", []string{"--sync", "nginx"}, true},
		{"remove long", []string{"--remove", "nginx"}, true},
		{"upgrade long", []string{"--upgrade", "pkg.tar.zst"}, true},
		// Existing over-detection: search/info start with S and are detected.
		{"sync search over-detect", []string{"-Ss", "nginx"}, true},
		{"sync info over-detect", []string{"-Si", "nginx"}, true},
		// Queries are not modifying.
		{"query", []string{"-Q"}, false},
		{"query info", []string{"-Qi", "nginx"}, false},
		// Accepted difference: a non-idiomatic ordering is no longer detected.
		{"non-idiomatic ordering", []string{"-yS"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSystemModification("pacman", tt.args))
		})
	}
}

// TestSystemModification_AbsolutePathAndSymlink verifies that flag-style
// detection works on a resolved absolute path and through a symlink alias, since
// classification is performed over the resolved name set (basename + symlink
// targets), not the literal invocation string.
func TestSystemModification_AbsolutePathAndSymlink(t *testing.T) {
	t.Run("absolute path", func(t *testing.T) {
		assert.True(t, isSystemModification("/usr/bin/dpkg", []string{"-i", "pkg.deb"}))
		assert.True(t, isSystemModification("/usr/bin/rpm", []string{"-U", "pkg.rpm"}))
	})

	t.Run("symlink alias", func(t *testing.T) {
		tmpDir := tu.SafeTempDir(t)

		// A symlink alias whose resolved target basename is dpkg/rpm must be
		// detected, since the resolved name set includes the target basename.
		for _, mgr := range []struct {
			name string
			args []string
		}{
			{"dpkg", []string{"-i", "pkg.deb"}},
			{"rpm", []string{"-U", "pkg.rpm"}},
		} {
			realPath := tmpDir + "/" + mgr.name
			f, err := os.Create(realPath)
			require.NoError(t, err)
			f.Close()

			alias := tmpDir + "/" + mgr.name + "-alias"
			require.NoError(t, os.Symlink(realPath, alias))

			assert.Truef(t, isSystemModification(alias, mgr.args),
				"symlink alias to %s should be detected", mgr.name)
		}
	})
}
