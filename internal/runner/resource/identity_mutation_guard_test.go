//go:build test

package resource

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This guard protects the resource package, a separate boundary from the
// existing identity-mutation guard in internal/runner/base/privilege
// (identity_mutation_guard_test.go). That guard allows two specific call
// sites (root escalation and restoration); this one allows none, because
// validateRunAsIdentity (dryrun_manager.go) only ever reads identity
// (risktypes.ResolveRunAsIdentStrict / OriginalExecutionIdentity) and must
// never change it. A single hit here is a failure with no exceptions.

// identityMutationFuncNames lists the syscall/unix functions that change the
// calling process's identity (UID, GID, or supplementary groups). See the
// same list in internal/runner/base/privilege/identity_mutation_guard_test.go
// for the rationale behind each entry.
var identityMutationFuncNames = map[string]struct{}{
	"Seteuid":     {},
	"Setegid":     {},
	"Setuid":      {},
	"Setgid":      {},
	"Setreuid":    {},
	"Setregid":    {},
	"Setresuid":   {},
	"Setresgid":   {},
	"Setgroups":   {},
	"Setfsuid":    {},
	"Setfsgid":    {},
	"Syscall":     {},
	"Syscall6":    {},
	"RawSyscall":  {},
	"RawSyscall6": {},
	"Capset":      {},
	"Prctl":       {},
}

// isTrackedIdentityMutationImportPath reports whether importPath is "syscall",
// exactly "unix", or ends in "/unix" (e.g. golang.org/x/sys/unix).
func isTrackedIdentityMutationImportPath(importPath string) bool {
	return importPath == "syscall" || importPath == "unix" || strings.HasSuffix(importPath, "/unix")
}

// identityMutationCallSite records one identity-mutation call found by
// parsing a source file.
type identityMutationCallSite struct {
	funcName    string
	syscallName string
	callExpr    string
}

// identityMutationValueRef records one identity-mutation function referenced
// as a value rather than called outright (e.g. assigned to a variable or
// field), which could be invoked later through that reference.
type identityMutationValueRef struct {
	funcName string
	expr     string
}

// TestResourcePackageDoesNotMutateIdentity statically verifies that the
// resource package never calls, or references as a value, a syscall/unix
// identity-mutation function. Unlike the privilege package's guard, the
// allow-list here is empty: the resource package's job is to preview and
// validate, not to change what the process is running as.
func TestResourcePackageDoesNotMutateIdentity(t *testing.T) {
	// Control 1: an aliased import is still resolved to "syscall".
	aliasSrc := "package p\n" +
		"import sc \"syscall\"\n" +
		"func f() error { return sc.Seteuid(0) }\n"
	aliasSites := identityMutationCallSitesInSource(t, "alias.go", aliasSrc)
	require.Len(t, aliasSites, 1, "an aliased syscall import must still be recognized")
	assert.Equal(t, "Seteuid", aliasSites[0].syscallName)

	// Control 2: golang.org/x/sys/unix is matched by suffix.
	unixSrc := "package p\n" +
		"import \"golang.org/x/sys/unix\"\n" +
		"func f() error { return unix.Setgroups(nil) }\n"
	unixSites := identityMutationCallSitesInSource(t, "unix.go", unixSrc)
	require.Len(t, unixSites, 1)
	assert.Equal(t, "Setgroups", unixSites[0].syscallName)

	// Control 3: a raw syscall entry point is tracked in its own right.
	rawSrc := "package p\n" +
		"import \"syscall\"\n" +
		"func f() { syscall.RawSyscall(syscall.SYS_SETRESUID, 0, 0, 0) }\n"
	rawSites := identityMutationCallSitesInSource(t, "raw.go", rawSrc)
	require.Len(t, rawSites, 1, "a raw syscall entry point must be detected")

	// Control 4: a function-value reference (not a direct call) is flagged
	// too, since it could be invoked later through the variable/field.
	valueRefSrc := "package p\n" +
		"import \"syscall\"\n" +
		"func f() func(int) error { return syscall.Seteuid }\n"
	valueRefCalls, valueRefSites := identityMutationRefsInSource(t, "value_ref.go", valueRefSrc)
	assert.Empty(t, valueRefCalls, "a bare function-value reference must not be counted as a call")
	require.Len(t, valueRefSites, 1, "a bare function-value reference must be detected")

	// Control 5: a local identifier that merely shares the package's name
	// (not a real import) is not resolved to the tracked package.
	shadowSrc := "package p\n" +
		"type syscall struct{}\n" +
		"func (syscall) Seteuid(int) error { return nil }\n" +
		"func f() error { var syscall syscall; return syscall.Seteuid(1) }\n"
	shadowSites := identityMutationCallSitesInSource(t, "shadow.go", shadowSrc)
	assert.Empty(t, shadowSites, "a local identifier that merely shares the package's name must not be treated as the syscall package")

	sites, valueRefs := findIdentityMutationRefs(t, ".")

	for _, ref := range valueRefs {
		t.Errorf("identity-mutation function %s referenced as a value in function %s; the resource package must never reference it, called or not",
			ref.expr, ref.funcName)
	}
	for _, site := range sites {
		t.Errorf("unexpected identity-mutation call %s in function %s: the resource package must never change process identity",
			site.callExpr, site.funcName)
	}
}

// findIdentityMutationRefs parses every production .go file in dir (skipping
// *_test.go and any file whose //go:build constraint positively requires the
// "test" tag) and returns every call to, and every function-value reference
// of, a syscall/unix identity-mutation function.
func findIdentityMutationRefs(t *testing.T, dir string) ([]identityMutationCallSite, []identityMutationValueRef) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoErrorf(t, err, "failed to read directory %s", dir)

	var sites []identityMutationCallSite
	var valueRefs []identityMutationValueRef
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "failed to read %s", path)

		src := string(data)
		if isTestOnlyBuildConstrained(t, path, src) {
			continue
		}

		fileSites, fileValueRefs := identityMutationRefsInSource(t, path, src)
		sites = append(sites, fileSites...)
		valueRefs = append(valueRefs, fileValueRefs...)
	}

	return sites, valueRefs
}

// isTestOnlyBuildConstrained reports whether src's //go:build constraint
// positively requires the "test" tag. See the sibling guard in
// internal/runner/base/privilege/identity_mutation_guard_test.go for the
// full rationale (evaluation cannot distinguish "false on this platform"
// from "test-only").
func isTestOnlyBuildConstrained(t *testing.T, filename, src string) bool {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.PackageClauseOnly|parser.ParseComments)
	require.NoErrorf(t, err, "failed to parse build constraints of %s", filename)

	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, comment := range group.List {
			if !strings.HasPrefix(comment.Text, "//go:build") {
				continue
			}
			expr, err := constraint.Parse(comment.Text)
			require.NoErrorf(t, err, "failed to parse build constraint %q in %s", comment.Text, filename)
			return constraintRequiresTag(expr, "test", false)
		}
	}
	return false
}

// constraintRequiresTag reports whether expr positively requires the named
// build tag. See the sibling guard for the full derivation.
func constraintRequiresTag(expr constraint.Expr, tag string, negated bool) bool {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		return !negated && e.Tag == tag
	case *constraint.NotExpr:
		return constraintRequiresTag(e.X, tag, !negated)
	case *constraint.AndExpr:
		if negated {
			return constraintRequiresTag(e.X, tag, true) && constraintRequiresTag(e.Y, tag, true)
		}
		return constraintRequiresTag(e.X, tag, false) || constraintRequiresTag(e.Y, tag, false)
	case *constraint.OrExpr:
		if negated {
			return constraintRequiresTag(e.X, tag, true) || constraintRequiresTag(e.Y, tag, true)
		}
		return constraintRequiresTag(e.X, tag, false) && constraintRequiresTag(e.Y, tag, false)
	default:
		return false
	}
}

// identityMutationCallSitesInSource returns only the calls found by
// identityMutationRefsInSource.
func identityMutationCallSitesInSource(t *testing.T, filename, src string) []identityMutationCallSite {
	t.Helper()
	calls, _ := identityMutationRefsInSource(t, filename, src)
	return calls
}

// identityMutationRefsInSource parses src and returns every syscall/unix
// identity-mutation function that is called, and every one that is merely
// referenced as a value. The local package identifier is resolved to its
// import path via the file's import declarations in both cases.
func identityMutationRefsInSource(t *testing.T, filename, src string) ([]identityMutationCallSite, []identityMutationValueRef) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	require.NoErrorf(t, err, "failed to parse %s", filename)

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
	calledSelectors := make(map[*ast.SelectorExpr]struct{})
	for _, decl := range file.Decls {
		funcName := "package-level"
		node := ast.Node(decl)
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if funcDecl.Body == nil {
				continue
			}
			funcName = declaredFuncName(funcDecl)
			node = funcDecl.Body
		} else if _, ok := decl.(*ast.GenDecl); !ok {
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
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
				for i := range n.Args {
					args[i] = "..."
				}

				sites = append(sites, identityMutationCallSite{
					funcName:    funcName,
					syscallName: sel.Sel.Name,
					callExpr:    sel.Sel.Name + "(" + strings.Join(args, ", ") + ")",
				})

			case *ast.SelectorExpr:
				if _, isCallee := calledSelectors[n]; isCallee {
					return true
				}
				if _, ok := trackedSelector(n); !ok {
					return true
				}
				valueRefs = append(valueRefs, identityMutationValueRef{
					funcName: funcName,
					expr:     pkgIdentName(n) + "." + n.Sel.Name,
				})
			}
			return true
		})
	}

	return sites, valueRefs
}

// declaredFuncName renders a function declaration's name, qualified by its
// receiver type for methods.
func declaredFuncName(funcDecl *ast.FuncDecl) string {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
		return funcDecl.Name.Name
	}
	if starExpr, ok := funcDecl.Recv.List[0].Type.(*ast.StarExpr); ok {
		if ident, ok := starExpr.X.(*ast.Ident); ok {
			return "(*" + ident.Name + ")." + funcDecl.Name.Name
		}
	}
	if ident, ok := funcDecl.Recv.List[0].Type.(*ast.Ident); ok {
		return "(" + ident.Name + ")." + funcDecl.Name.Name
	}
	return funcDecl.Name.Name
}

// pkgIdentName renders the package-qualifier identifier of a selector
// expression, e.g. "syscall" in syscall.Seteuid.
func pkgIdentName(sel *ast.SelectorExpr) string {
	if ident, ok := sel.X.(*ast.Ident); ok {
		return ident.Name
	}
	return "?"
}
