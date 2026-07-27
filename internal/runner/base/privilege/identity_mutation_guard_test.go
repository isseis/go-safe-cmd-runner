//go:build !windows

package privilege

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// identityMutationFuncNames lists the syscall/unix functions that change the
// calling process's identity (UID, GID, or supplementary groups). A call to
// any of these outside the two allowed call sites checked below would mean
// this package mutates identity somewhere other than the root-escalation and
// root-restoration paths that WithPrivileges relies on (see unix.go).
var identityMutationFuncNames = map[string]struct{}{
	"Seteuid":   {},
	"Setegid":   {},
	"Setuid":    {},
	"Setgid":    {},
	"Setreuid":  {},
	"Setregid":  {},
	"Setresuid": {},
	"Setresgid": {},
	"Setgroups": {},
	"Setfsuid":  {},
	"Setfsgid":  {},
}

// isTrackedIdentityMutationImportPath reports whether importPath is "syscall"
// or golang.org/x/sys/unix (matched by suffix, like the sibling guard
// forbiddenLiveIdentityPackage in internal/runner/base/risk, so a vendored or
// differently rooted path still resolves).
func isTrackedIdentityMutationImportPath(importPath string) bool {
	if importPath == "syscall" {
		return true
	}
	return importPath == "unix" || strings.HasSuffix(importPath, "/unix")
}

// allowedIdentityMutationCall identifies one permitted call site: the
// enclosing function and the exact argument expression, rendered by
// go/types.ExprString, that the call must use.
type allowedIdentityMutationCall struct {
	funcName string
	arg      string
}

// allowedIdentityMutationCalls is the exhaustive list of identity-mutation
// calls this package may make: escalating to root, and restoring the
// original UID afterwards. No other combination is permitted.
var allowedIdentityMutationCalls = []allowedIdentityMutationCall{
	{funcName: "escalatePrivileges", arg: "0"},
	{funcName: "restorePrivileges", arg: "m.originalUID"},
}

// identityMutationCallSite records one identity-mutation call found by
// parsing a source file.
type identityMutationCallSite struct {
	funcName string
	callExpr string // rendered "pkg.Func(args)" for failure messages
	arg      string
}

// TestNoUnexpectedIdentityMutationSyscalls statically verifies that
// syscall/unix identity-mutation functions (Seteuid, Setegid, ...) are only
// called from escalatePrivileges(0) and restorePrivileges(m.originalUID).
// It replaces a dynamic check that non-privileged CI cannot perform: a
// demotion syscall failing with EPERM leaves identity unchanged, so it can't
// be distinguished from one that was never called.
//
// Parsing notes:
//   - Uses go/parser.ParseFile per file, not the deprecated ParseDir
//     (SA1019, not exempted for _test.go in this repo's staticcheck config).
//   - Does not interpret build tags, so identity_linux.go and
//     identity_other.go are both parsed regardless of GOOS; neither calls an
//     identity-mutation function, so the check applies uniformly across
//     platforms.
//   - Resolves each call's local package identifier to its import path via
//     the file's imports (same technique as
//     internal/runner/base/risk/live_identity_guard_test.go's
//     liveIdentityRefsIn), so an aliased import can't bypass the guard and a
//     same-named local identifier can't produce a false match.
//
// Known gap: only recognizes qualified calls (`syscall.Seteuid(...)`), not a
// function-value reference (`syscallSeteuid: syscall.Seteuid`) or an
// indirect call through an injected field (`m.syscallSetegid(gid)`).
// Widening to all selector expressions is deferred because
// UnixPrivilegeManager.syscallSeteuid/syscallSetegid are still legitimately
// initialized that way (see newPlatformManager, changeUserGroupInternal in
// unix.go); a later step removes those fields once their demotion path is
// deleted and extends this test to reject bare function-value references too.
func TestNoUnexpectedIdentityMutationSyscalls(t *testing.T) {
	// Control 1: an aliased import is still resolved to "syscall" (matching is by
	// import path, not local name), and the disallowed argument is flagged.
	aliasSrc := "package p\n" +
		"import sc \"syscall\"\n" +
		"func escalatePrivileges() error { return sc.Seteuid(1) }\n"
	aliasSites := identityMutationCallSitesInSource(t, "alias.go", aliasSrc)
	require.Len(t, aliasSites, 1, "an aliased syscall import must still be recognized")
	assert.Equal(t, "escalatePrivileges", aliasSites[0].funcName)
	assert.Equal(t, "1", aliasSites[0].arg)

	// Control 2: a call via a local identifier that happens to be named "syscall" but
	// is not an import of the syscall package (here, a local variable) is not
	// resolved to the real syscall package and so is not flagged. This exercises the
	// import-path resolution rather than literal identifier matching.
	shadowSrc := "package p\n" +
		"type syscall struct{}\n" +
		"func (syscall) Seteuid(int) error { return nil }\n" +
		"func f() error { var syscall syscall; return syscall.Seteuid(1) }\n"
	shadowSites := identityMutationCallSitesInSource(t, "shadow.go", shadowSrc)
	assert.Empty(t, shadowSites, "a local identifier that merely shares the package's name must not be treated as the syscall package")

	// Control 3: golang.org/x/sys/unix is matched by suffix.
	unixSrc := "package p\n" +
		"import \"golang.org/x/sys/unix\"\n" +
		"func restorePrivileges() error { return unix.Seteuid(0) }\n"
	unixSites := identityMutationCallSitesInSource(t, "unix.go", unixSrc)
	require.Len(t, unixSites, 1)
	assert.Equal(t, "restorePrivileges", unixSites[0].funcName)

	sites := findIdentityMutationCallSites(t, ".")

	seen := make(map[allowedIdentityMutationCall]bool, len(allowedIdentityMutationCalls))
	for _, site := range sites {
		got := allowedIdentityMutationCall{funcName: site.funcName, arg: site.arg}
		if !isAllowedIdentityMutationCall(got) {
			t.Errorf("unexpected identity-mutation call %s in function %s: only escalatePrivileges(0) and restorePrivileges(m.originalUID) are permitted",
				site.callExpr, site.funcName)
			continue
		}
		seen[got] = true
	}

	for _, allowed := range allowedIdentityMutationCalls {
		if !seen[allowed] {
			t.Errorf("expected call with argument %s not found in function %s; the check may be vacuously passing",
				allowed.arg, allowed.funcName)
		}
	}
}

func isAllowedIdentityMutationCall(call allowedIdentityMutationCall) bool {
	for _, allowed := range allowedIdentityMutationCalls {
		if allowed == call {
			return true
		}
	}
	return false
}

// findIdentityMutationCallSites parses every non-test .go file in dir and
// returns every call to a syscall/unix identity-mutation function, recording
// the name of the enclosing top-level function or method.
//
// Besides *_test.go, this also skips test_helpers.go / test_helpers_*.go:
// per this repo's convention (docs/dev/developer_guide/test_organization.md,
// "Classification B"), those always carry a `//go:build test` constraint and
// so are never part of the production build, even though their name doesn't
// end in "_test.go".
func findIdentityMutationCallSites(t *testing.T, dir string) []identityMutationCallSite {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoErrorf(t, err, "failed to read directory %s", dir)

	var sites []identityMutationCallSite
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || isTestHelpersFileName(name) {
			continue
		}

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "failed to read %s", path)

		sites = append(sites, identityMutationCallSitesInSource(t, path, string(data))...)
	}

	return sites
}

// isTestHelpersFileName reports whether name matches this repo's
// test-helper file naming convention: test_helpers.go or
// test_helpers_<category>.go.
func isTestHelpersFileName(name string) bool {
	base := strings.TrimSuffix(name, ".go")
	return base == "test_helpers" || strings.HasPrefix(base, "test_helpers_")
}

func TestIsTestHelpersFileName(t *testing.T) {
	tests := map[string]bool{
		"test_helpers.go":       true,
		"test_helpers_group.go": true,
		"test_helpersgroup.go":  false, // no separator: not the "_<category>" form
		"testhelpers.go":        false,
		"helpers_test.go":       false, // this is a _test.go file, not a test_helpers one
		"manager.go":            false,
		"manager_test.go":       false,
	}
	for name, want := range tests {
		assert.Equalf(t, want, isTestHelpersFileName(name), "isTestHelpersFileName(%q)", name)
	}
}

// identityMutationCallSitesInSource parses src (Go source for a single file
// named filename) and returns every call to a syscall/unix identity-mutation
// function, with the local package identifier of each call resolved to its
// import path via the file's import declarations.
func identityMutationCallSitesInSource(t *testing.T, filename, src string) []identityMutationCallSite {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	require.NoErrorf(t, err, "failed to parse %s", filename)

	// localToImportPath maps the local package identifier (alias if present, else
	// the path's last element) to the full import path, so an aliased import
	// cannot bypass the check by literal-name matching.
	localToImportPath := make(map[string]string)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		require.NoErrorf(t, err, "failed to unquote import path %s in %s", imp.Path.Value, filename)
		if imp.Name != nil && imp.Name.Name == "." {
			require.Falsef(t, isTrackedIdentityMutationImportPath(path),
				"dot-import of identity-mutation package is forbidden: %s in %s", path, filename)
			continue
		}
		local := path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			local = path[idx+1:]
		}
		if imp.Name != nil {
			local = imp.Name.Name
		}
		localToImportPath[local] = path
	}

	var sites []identityMutationCallSite
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}

		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			importPath, isImported := localToImportPath[pkgIdent.Name]
			if !isImported {
				// Not a resolved import (e.g. a local variable or type that merely
				// shares the package's name): never treat this as a match, unlike
				// falling back to the bare identifier.
				return true
			}
			if !isTrackedIdentityMutationImportPath(importPath) {
				return true
			}
			if _, isTrackedFunc := identityMutationFuncNames[sel.Sel.Name]; !isTrackedFunc {
				return true
			}

			args := make([]string, len(call.Args))
			for i, arg := range call.Args {
				args[i] = types.ExprString(arg)
			}
			arg := strings.Join(args, ", ")

			sites = append(sites, identityMutationCallSite{
				funcName: funcDecl.Name.Name,
				callExpr: pkgIdent.Name + "." + sel.Sel.Name + "(" + arg + ")",
				arg:      arg,
			})
			return true
		})
	}

	return sites
}
