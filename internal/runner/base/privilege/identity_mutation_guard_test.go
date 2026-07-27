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

// isTrackedIdentityMutationImportPath reports whether importPath is "syscall",
// exactly "unix", or ends in "/unix" (e.g. golang.org/x/sys/unix), matching by
// suffix like the sibling guard forbiddenLiveIdentityPackage in
// internal/runner/base/risk, so a vendored or differently rooted path still
// resolves.
func isTrackedIdentityMutationImportPath(importPath string) bool {
	return importPath == "syscall" || importPath == "unix" || strings.HasSuffix(importPath, "/unix")
}

// allowedIdentityMutationCall identifies one permitted call site: the
// enclosing function, the specific syscall/unix function name (e.g.
// "Seteuid"), and the exact argument expression, rendered by
// go/types.ExprString, that the call must use.
type allowedIdentityMutationCall struct {
	funcName    string
	syscallName string
	arg         string
}

// allowedIdentityMutationCalls is the exhaustive list of identity-mutation
// calls this package may make: escalating to root, and restoring the
// original UID afterwards. No other combination is permitted.
var allowedIdentityMutationCalls = []allowedIdentityMutationCall{
	{funcName: "escalatePrivileges", syscallName: "Seteuid", arg: "0"},
	{funcName: "restorePrivileges", syscallName: "Seteuid", arg: "m.originalUID"},
}

// identityMutationCallSite records one identity-mutation call found by
// parsing a source file.
type identityMutationCallSite struct {
	funcName    string
	syscallName string
	callExpr    string // rendered "pkg.Func(args)" for failure messages
	arg         string
}

// identityMutationValueRef records one identity-mutation function referenced as
// a value rather than called outright, e.g. a struct field initialized to
// syscall.Seteuid. Such a reference can be invoked later through that variable
// or field, which the call-site scan cannot follow, so none are permitted.
type identityMutationValueRef struct {
	funcName string
	expr     string // rendered "pkg.Func" for failure messages
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
// Beyond direct calls, it also rejects referencing any of these functions as a
// value, e.g. a struct field initialized to syscall.Seteuid. Such a reference
// could be invoked later through that variable or field -- an indirect call
// this AST scan cannot follow to its target -- so no value reference is
// permitted anywhere in the package. Together the two checks mean an
// identity-mutation function can only be reached through the two allowed call
// sites above.
//
// Remaining gap: a call reaching these syscalls without naming them here, e.g.
// via reflection or a helper in another package, is still invisible to this
// scan.
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

	// Control 4: a call sharing the allowed (funcName, arg) pair but using a
	// different identity-mutation syscall must not be treated as allowed.
	// This guards against the allowlist matching on funcName/arg alone and
	// missing a swap like Setuid(0) in place of the required Seteuid(0).
	wrongSyscallSrc := "package p\n" +
		"import \"syscall\"\n" +
		"func escalatePrivileges() error { return syscall.Setuid(0) }\n"
	wrongSyscallSites := identityMutationCallSitesInSource(t, "wrong_syscall.go", wrongSyscallSrc)
	require.Len(t, wrongSyscallSites, 1)
	got := allowedIdentityMutationCall{
		funcName:    wrongSyscallSites[0].funcName,
		syscallName: wrongSyscallSites[0].syscallName,
		arg:         wrongSyscallSites[0].arg,
	}
	assert.False(t, isAllowedIdentityMutationCall(got),
		"Setuid(0) in escalatePrivileges must not be allowed even though escalatePrivileges/Seteuid(0) is; only the specific syscall/arg combination is permitted")

	// Control 5: a call inside a package-level var initializer, outside any function
	// body, must still be found; the decl loop must not only walk *ast.FuncDecl.
	packageLevelSrc := "package p\n" +
		"import \"syscall\"\n" +
		"var _ = syscall.Seteuid(0)\n"
	packageLevelSites := identityMutationCallSitesInSource(t, "package_level.go", packageLevelSrc)
	require.Len(t, packageLevelSites, 1, "a call in a package-level var initializer must be detected")
	assert.Equal(t, "package-level", packageLevelSites[0].funcName)

	// Control 6: a parenthesized callee must resolve the same as an unparenthesized
	// one, since ast.CallExpr.Fun can be wrapped in *ast.ParenExpr.
	parenSrc := "package p\n" +
		"import \"syscall\"\n" +
		"func escalatePrivileges() error { return (syscall.Seteuid)(0) }\n"
	parenSites := identityMutationCallSitesInSource(t, "paren.go", parenSrc)
	require.Len(t, parenSites, 1, "a parenthesized callee like (syscall.Seteuid)(0) must be detected")
	assert.Equal(t, "escalatePrivileges", parenSites[0].funcName)
	assert.Equal(t, "0", parenSites[0].arg)

	// Control 7: a function-value reference is reported as a value ref, not a call.
	// This is the form that lets an identity-mutation function be invoked later
	// through a variable or field, which the call-site scan cannot follow.
	valueRefSrc := "package p\n" +
		"import \"syscall\"\n" +
		"func newPlatformManager() func(int) error { return syscall.Seteuid }\n"
	valueRefCalls, valueRefSites := identityMutationRefsInSource(t, "value_ref.go", valueRefSrc)
	assert.Empty(t, valueRefCalls, "a bare function-value reference must not be counted as a call")
	require.Len(t, valueRefSites, 1, "a bare function-value reference must be detected")
	assert.Equal(t, "newPlatformManager", valueRefSites[0].funcName)
	assert.Equal(t, "syscall.Seteuid", valueRefSites[0].expr)

	// Control 8: an outright call is not additionally reported as a value reference,
	// so the two allowed call sites below do not themselves trip the value-ref check.
	calledNotValueRefs := identityMutationValueRefsInSource(t, "called.go",
		"package p\n"+
			"import \"syscall\"\n"+
			"func escalatePrivileges() error { return syscall.Seteuid(0) }\n")
	assert.Empty(t, calledNotValueRefs, "a called selector must not also be reported as a function-value reference")

	sites, valueRefs := findIdentityMutationRefs(t, ".")

	for _, ref := range valueRefs {
		t.Errorf("identity-mutation function %s referenced as a value in function %s; it could be invoked indirectly through a variable or field, which this check cannot follow, so no value reference is permitted",
			ref.expr, ref.funcName)
	}

	seen := make(map[allowedIdentityMutationCall]bool, len(allowedIdentityMutationCalls))
	for _, site := range sites {
		got := allowedIdentityMutationCall{funcName: site.funcName, syscallName: site.syscallName, arg: site.arg}
		if !isAllowedIdentityMutationCall(got) {
			t.Errorf("unexpected identity-mutation call %s in function %s: only escalatePrivileges(0) and restorePrivileges(m.originalUID) are permitted",
				site.callExpr, site.funcName)
			continue
		}
		seen[got] = true
	}

	for _, allowed := range allowedIdentityMutationCalls {
		if !seen[allowed] {
			t.Errorf("expected call %s(%s) not found in function %s; the check may be vacuously passing",
				allowed.syscallName, allowed.arg, allowed.funcName)
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

// findIdentityMutationRefs parses every non-test .go file in dir and returns
// every call to, and every function-value reference of, a syscall/unix
// identity-mutation function, recording the name of the enclosing top-level
// function or method.
//
// Besides *_test.go, this also skips test_helpers.go / test_helpers_*.go:
// per this repo's convention (docs/dev/developer_guide/test_organization.md,
// "Classification B"), those always carry a `//go:build test` constraint and
// so are never part of the production build, even though their name doesn't
// end in "_test.go".
func findIdentityMutationRefs(t *testing.T, dir string) ([]identityMutationCallSite, []identityMutationValueRef) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoErrorf(t, err, "failed to read directory %s", dir)

	var sites []identityMutationCallSite
	var valueRefs []identityMutationValueRef
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || isTestHelpersFileName(name) {
			continue
		}

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "failed to read %s", path)

		fileSites, fileValueRefs := identityMutationRefsInSource(t, path, string(data))
		sites = append(sites, fileSites...)
		valueRefs = append(valueRefs, fileValueRefs...)
	}

	return sites, valueRefs
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

// identityMutationCallSitesInSource returns only the calls found by
// identityMutationRefsInSource, for assertions that concern call sites alone.
func identityMutationCallSitesInSource(t *testing.T, filename, src string) []identityMutationCallSite {
	t.Helper()

	calls, _ := identityMutationRefsInSource(t, filename, src)
	return calls
}

// identityMutationValueRefsInSource returns only the value references found by
// identityMutationRefsInSource.
func identityMutationValueRefsInSource(t *testing.T, filename, src string) []identityMutationValueRef {
	t.Helper()

	_, valueRefs := identityMutationRefsInSource(t, filename, src)
	return valueRefs
}

// identityMutationRefsInSource parses src (Go source for a single file named
// filename) and returns every syscall/unix identity-mutation function that is
// called, and every one that is merely referenced as a value. The local package
// identifier is resolved to its import path via the file's import declarations
// in both cases.
func identityMutationRefsInSource(t *testing.T, filename, src string) ([]identityMutationCallSite, []identityMutationValueRef) {
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

	// trackedSelector reports whether expr is a qualified reference to one of the
	// identity-mutation functions, e.g. syscall.Seteuid, resolving the qualifier
	// through the file's imports.
	trackedSelector := func(expr ast.Expr) (*ast.SelectorExpr, bool) {
		sel, ok := expr.(*ast.SelectorExpr)
		if !ok {
			return nil, false
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return nil, false
		}
		importPath, isImported := localToImportPath[pkgIdent.Name]
		if !isImported {
			// Not a resolved import (e.g. a local variable or type that merely
			// shares the package's name): never treat this as a match, unlike
			// falling back to the bare identifier.
			return nil, false
		}
		if !isTrackedIdentityMutationImportPath(importPath) {
			return nil, false
		}
		if _, isTrackedFunc := identityMutationFuncNames[sel.Sel.Name]; !isTrackedFunc {
			return nil, false
		}
		return sel, true
	}

	var sites []identityMutationCallSite
	var valueRefs []identityMutationValueRef
	// calledSelectors marks the selectors that appear in callee position, so the
	// value-reference pass can skip them. ast.Inspect visits a CallExpr before its
	// children, so a selector is always marked before it is reached on its own.
	calledSelectors := make(map[*ast.SelectorExpr]struct{})
	for _, decl := range file.Decls {
		// funcName labels reported sites; package-level var/const initializers have no
		// enclosing function, so this fallback keeps them from being silently skipped.
		funcName := "package-level"
		node := ast.Node(decl)
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if funcDecl.Body == nil {
				continue
			}
			funcName = funcDecl.Name.Name
			node = funcDecl.Body
		} else if _, ok := decl.(*ast.GenDecl); !ok {
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				// Unwrap parens so a parenthesized callee like (syscall.Seteuid)(0) is still recognized.
				fun := n.Fun
				for {
					paren, ok := fun.(*ast.ParenExpr)
					if !ok {
						break
					}
					fun = paren.X
				}

				sel, ok := trackedSelector(fun)
				if !ok {
					return true
				}
				calledSelectors[sel] = struct{}{}

				args := make([]string, len(n.Args))
				for i, arg := range n.Args {
					args[i] = types.ExprString(arg)
				}
				arg := strings.Join(args, ", ")

				sites = append(sites, identityMutationCallSite{
					funcName:    funcName,
					syscallName: sel.Sel.Name,
					callExpr:    types.ExprString(sel) + "(" + arg + ")",
					arg:         arg,
				})

			case *ast.SelectorExpr:
				// Reached on its own rather than in callee position: a function-value
				// reference that could be invoked indirectly later.
				if _, isCallee := calledSelectors[n]; isCallee {
					return true
				}
				if _, ok := trackedSelector(n); !ok {
					return true
				}
				valueRefs = append(valueRefs, identityMutationValueRef{
					funcName: funcName,
					expr:     types.ExprString(n),
				})
			}
			return true
		})
	}

	return sites, valueRefs
}
