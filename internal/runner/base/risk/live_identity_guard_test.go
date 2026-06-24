//go:build test

package risk

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zoningGuardedFiles are the axis-2 classification sources that must stay free of
// live-identity and ambient-environment reads. Paths are relative to this package
// directory (go test runs in the package directory). destination_zoning_spec.go is
// included because its command specs, operand extractors, and operation floors are
// core classification logic.
var zoningGuardedFiles = []string{
	"../security/destination_zoning.go",
	"../security/destination_zoning_spec.go",
	"../security/operand_path_resolver.go",
}

// forbiddenLiveIdentityCall reports whether a pkg.fn selector is a live-identity or
// ambient-environment read that the axis-2 zoning code must never call: the judgment
// consumes only the precomputed RunAsIdent injected at construction, so reading the
// live process identity or the environment ($HOME and friends) would make the verdict
// depend on live euid / env and diverge between dry-run and runtime. The set covers
// the os/syscall/unix uid/gid/euid/egid/groups getters, the environment readers
// (Getenv/LookupEnv/Environ and the os.User*Dir / ExpandEnv helpers), and the os/user
// database lookups (Current and the Lookup* family). It is a non-exhaustive
// regression guardrail, not a completeness proof.
func forbiddenLiveIdentityCall(pkg, fn string) bool {
	switch pkg {
	case "os":
		switch fn {
		case "Geteuid", "Getuid", "Getgid", "Getegid", "Getgroups",
			"Getenv", "LookupEnv", "Environ", "ExpandEnv",
			"UserHomeDir", "UserConfigDir", "UserCacheDir":
			return true
		}
	case "syscall", "unix":
		switch fn {
		case "Geteuid", "Getuid", "Getgid", "Getegid", "Getgroups", "Environ", "Getenv":
			return true
		}
	case "user":
		return fn == "Current" || strings.HasPrefix(fn, "Lookup")
	}
	return false
}

// liveIdentityCallsIn parses Go source and returns "file:line: pkg.fn" for each
// forbidden call it contains. It inspects the AST's call expressions only, so it is
// immune to the false positives a raw-text scan suffers: forbidden names appearing in
// comments or string literals (e.g. documenting that an API is forbidden) are not
// calls and are ignored, and source formatting / line splits do not matter.
func liveIdentityCallsIn(t *testing.T, name, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	require.NoErrorf(t, err, "parse %s", name)

	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if forbiddenLiveIdentityCall(pkg.Name, sel.Sel.Name) {
			hits = append(hits, fmt.Sprintf("%s:%d: %s.%s", name, fset.Position(call.Pos()).Line, pkg.Name, sel.Sel.Name))
		}
		return true
	})
	return hits
}

// TestNoLiveIdentityInZoning is the static guard for the identity-purity contract:
// the axis-2 classification code reads no live process identity or environment. The
// guarded files are required to exist and be non-empty so a rename cannot void the
// guard silently.
func TestNoLiveIdentityInZoning(t *testing.T) {
	// Control: a real call is flagged, while the same text in a comment and in a
	// string literal is ignored. This proves the AST check inspects calls only -- so
	// it cannot be vacuously defeated (a fail-open) nor false-positive on documentation.
	control := "package p\n" +
		"import \"os\"\n" +
		"// os.Geteuid() in a comment must be ignored\n" +
		"func f() { _ = \"os.Getenv() in a string\"; _ = os.Geteuid() }\n"
	assert.Equal(t, []string{"control.go:4: os.Geteuid"}, liveIdentityCallsIn(t, "control.go", control),
		"the AST check must flag the real call only, ignoring the comment and the string literal")

	for _, path := range zoningGuardedFiles {
		src, err := os.ReadFile(path)
		require.NoErrorf(t, err, "guarded file must exist (a move/rename must not silently void this guard): %s", path)
		require.NotEmptyf(t, src, "guarded file must be non-empty: %s", path)

		hits := liveIdentityCallsIn(t, path, string(src))
		assert.Emptyf(t, hits, "axis-2 zoning code must not read live identity or environment:\n%s", strings.Join(hits, "\n"))
	}
}
