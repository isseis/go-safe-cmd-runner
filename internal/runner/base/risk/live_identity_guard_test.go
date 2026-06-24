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

// forbiddenLiveIdentityPackage reports whether importPath is one of the packages
// whose live-identity / environment readers are forbidden in the zoning code.
// golang.org/x/sys/unix is matched by suffix so a vendored or differently-rooted
// path still resolves.
func forbiddenLiveIdentityPackage(importPath string) bool {
	switch importPath {
	case "os", "syscall", "os/user":
		return true
	}
	return importPath == "unix" || strings.HasSuffix(importPath, "/unix")
}

// forbiddenLiveIdentityCall reports whether importPath.fn is a live-identity or
// ambient-environment read that the axis-2 zoning code must never call: the judgment
// consumes only the precomputed RunAsIdent injected at construction, so reading the
// live process identity or the environment ($HOME and friends) would make the verdict
// depend on live euid / env and diverge between dry-run and runtime. The set covers
// the os/syscall/unix uid/gid/euid/egid/groups getters, the environment readers
// (Getenv/LookupEnv/Environ and the os.User*Dir / ExpandEnv helpers), and the os/user
// database lookups (Current and the Lookup* family). Matching is by resolved import
// path, not local identifier, so an aliased import cannot bypass it. It is a
// non-exhaustive regression guardrail, not a completeness proof.
func forbiddenLiveIdentityCall(importPath, fn string) bool {
	switch {
	case importPath == "os":
		switch fn {
		case "Geteuid", "Getuid", "Getgid", "Getegid", "Getgroups",
			"Getenv", "LookupEnv", "Environ", "ExpandEnv",
			"UserHomeDir", "UserConfigDir", "UserCacheDir":
			return true
		}
	case importPath == "syscall" || importPath == "unix" || strings.HasSuffix(importPath, "/unix"):
		switch fn {
		case "Geteuid", "Getuid", "Getgid", "Getegid", "Getgroups", "Environ", "Getenv":
			return true
		}
	case importPath == "os/user":
		return fn == "Current" || strings.HasPrefix(fn, "Lookup")
	}
	return false
}

// liveIdentityCallsIn parses Go source and returns "file:line: detail" for each
// forbidden call (or forbidden dot-import) it contains. It inspects the AST's call
// expressions only, so forbidden names appearing in comments or string literals are
// ignored, and formatting / line splits do not matter. Each selector's local package
// identifier is resolved to its import path via the file's import declarations, so an
// aliased import (import myos "os"; myos.Getenv()) cannot bypass the guard. A dot
// import of a forbidden package is itself reported, since it would make the calls
// unqualified and defeat selector-based detection.
func liveIdentityCallsIn(t *testing.T, name, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	require.NoErrorf(t, err, "parse %s", name)

	var hits []string
	// localToImportPath maps the local package identifier (alias if present, else the
	// path's last element) to the full import path.
	localToImportPath := make(map[string]string)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		local := path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			local = path[idx+1:]
		}
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "." {
			// A dot import makes the package's functions callable unqualified, which
			// selector inspection cannot see; for a forbidden package that is a bypass.
			if forbiddenLiveIdentityPackage(path) {
				hits = append(hits, fmt.Sprintf("%s:%d: dot-import of %q defeats the guard", name, fset.Position(imp.Pos()).Line, path))
			}
			continue
		}
		localToImportPath[local] = path
	}

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
		importPath, ok := localToImportPath[pkg.Name]
		if !ok {
			importPath = pkg.Name // not an imported package; fall back to the identifier
		}
		if forbiddenLiveIdentityCall(importPath, sel.Sel.Name) {
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
	// Control 1: an aliased forbidden import is still detected (matching is by import
	// path, not local name), while the same text in a comment and a string literal is
	// ignored and a non-forbidden call (strings) is not flagged.
	aliasCtl := "package p\n" + // 1
		"import (\n" + // 2
		"\tmyos \"os\"\n" + // 3
		"\t\"strings\"\n" + // 4
		")\n" + // 5
		"// myos.Geteuid() in a comment must be ignored\n" + // 6
		"func f() string { _ = \"myos.Getenv() in a string\"; _ = myos.Geteuid(); return strings.TrimSpace(\"x\") }\n" // 7
	assert.Equal(t, []string{"alias.go:7: myos.Geteuid"}, liveIdentityCallsIn(t, "alias.go", aliasCtl),
		"the AST check must resolve the alias to os and flag the call only, ignoring the comment, the string, and strings.TrimSpace")

	// Control 2: a dot import of a forbidden package is reported (it would make the
	// calls unqualified and bypass selector detection).
	dotCtl := "package p\n" +
		"import . \"os\"\n" +
		"func g() { _ = Geteuid() }\n"
	dotHits := liveIdentityCallsIn(t, "dot.go", dotCtl)
	require.Len(t, dotHits, 1)
	assert.Contains(t, dotHits[0], "dot-import", "a dot import of a forbidden package must be flagged")

	for _, path := range zoningGuardedFiles {
		src, err := os.ReadFile(path)
		require.NoErrorf(t, err, "guarded file must exist (a move/rename must not silently void this guard): %s", path)
		require.NotEmptyf(t, src, "guarded file must be non-empty: %s", path)

		hits := liveIdentityCallsIn(t, path, string(src))
		assert.Emptyf(t, hits, "axis-2 zoning code must not read live identity or environment:\n%s", strings.Join(hits, "\n"))
	}
}
