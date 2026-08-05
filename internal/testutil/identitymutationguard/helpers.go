//go:build test

// Package identitymutationguard provides a reusable go/ast scan that finds
// calls to, and value-references of, process-identity-mutating syscall/unix
// functions (Seteuid, Setgroups, raw Syscall entry points, ...) in a
// package's production source files.
//
// It backs the identity-mutation guard tests of packages that must never
// change process identity (e.g. internal/runner/resource,
// internal/runner/base/risktypes): each such package's guard test calls
// FindRefs with its own directory and interprets the result under its own
// allow-list policy (the privilege package's guard, which does need to
// escalate/restore, keeps its own copy with a two-entry allow-list rather
// than depending on this package, since its policy check is inherently
// package-specific).
package identitymutationguard

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

	"github.com/stretchr/testify/require"
)

// FuncNames is the set of syscall/unix functions that change the calling
// process's identity (UID, GID, or supplementary groups), or reach one
// indirectly. Seteuid/Setegid/Setuid/Setgid/Setreuid/Setregid/Setresuid/
// Setresgid/Setgroups/Setfsuid/Setfsgid change identity directly. Raw
// syscall entry points (Syscall/Syscall6/RawSyscall/RawSyscall6) can reach
// any of them without naming it, e.g.
// syscall.Syscall(syscall.SYS_SETRESUID, ...). Capset and Prctl alter the
// process's privilege state as materially as a set*id call.
var FuncNames = map[string]struct{}{
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

// isTrackedImportPath reports whether importPath is "syscall", exactly
// "unix", or ends in "/unix" (e.g. golang.org/x/sys/unix), so a vendored or
// differently rooted path still resolves.
func isTrackedImportPath(importPath string) bool {
	return importPath == "syscall" || importPath == "unix" || strings.HasSuffix(importPath, "/unix")
}

// CallSite records one tracked call found while scanning source.
type CallSite struct {
	FuncName    string // enclosing top-level function or method, receiver-qualified
	SyscallName string // e.g. "Seteuid"
	CallExpr    string // rendered "Func(...)" for failure messages
	File        string // path of the file the call was found in
	// Pos is the position of the call expression. Positions are only
	// comparable between call sites found in the same File: every file is
	// parsed with its own token.FileSet, so offsets restart per file and a
	// cross-file comparison is meaningless. Check File equality before
	// ordering two Pos values.
	Pos token.Pos
}

// ExtraTrackedFunc names one additional function to track beyond FuncNames.
// A non-empty ImportPath matches a qualified call such as flag.Parse(),
// resolved through the file's imports. An empty ImportPath matches an
// unqualified call to a function of the package being scanned, e.g.
// dropStartupPrivileges(...).
type ExtraTrackedFunc struct {
	ImportPath string
	FuncName   string
}

// Options customizes a scan. The zero value scans for FuncNames alone, which
// is what FindRefs and RefsInSource use.
type Options struct {
	Extra []ExtraTrackedFunc
}

// ValueRef records one identity-mutation function referenced as a value
// rather than called outright, e.g. a struct field initialized to
// syscall.Seteuid. Such a reference can be invoked later through that
// variable or field, which a call-site scan alone cannot follow.
type ValueRef struct {
	FuncName string
	Expr     string // rendered "pkg.Func" for failure messages
}

// FindRefs parses every production .go file in dir -- skipping *_test.go
// (the toolchain's own naming rule) and any file whose //go:build
// constraint positively requires the "test" tag, i.e. the constraint
// cannot be satisfied without it -- and returns every call to, and every
// function-value reference of, a tracked identity-mutation function.
//
// Only a positive requirement excludes a file. A constraint that merely
// evaluates false without the tag on the current platform (e.g.
// "!linux && !windows") says nothing about test-only-ness and is still
// scanned, which is the safe direction for a security guard.
func FindRefs(t *testing.T, dir string) ([]CallSite, []ValueRef) {
	t.Helper()
	return FindRefsWithOptions(t, dir, Options{})
}

// FindRefsWithOptions is FindRefs with the tracked function set extended by
// opts.
func FindRefsWithOptions(t *testing.T, dir string, opts Options) ([]CallSite, []ValueRef) {
	t.Helper()

	var sites []CallSite
	var valueRefs []ValueRef
	for _, path := range ProductionGoFiles(t, dir) {
		fileSites, fileValueRefs := RefsInSourceWithOptions(t, path, readGoFile(t, path), opts)
		sites = append(sites, fileSites...)
		valueRefs = append(valueRefs, fileValueRefs...)
	}

	return sites, valueRefs
}

// ProductionGoFiles returns the path of every production .go file in dir,
// applying the same exclusions as FindRefs: *_test.go, and any file whose
// //go:build constraint positively requires the "test" tag. Callers that need
// to run their own analysis over exactly the files a guard scans (e.g.
// counting init functions) use this so the definition of "production file"
// stays in one place.
func ProductionGoFiles(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoErrorf(t, err, "failed to read directory %s", dir)

	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		if isTestOnlyBuildConstrained(t, path, readGoFile(t, path)) {
			continue
		}
		paths = append(paths, path)
	}

	return paths
}

// readGoFile reads a .go file found by directory traversal.
func readGoFile(t *testing.T, path string) string {
	t.Helper()

	// #nosec G304 -- path is built from an os.ReadDir result filtered to *.go,
	// not from external/attacker-controlled input.
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "failed to read %s", path)
	return string(data)
}

// isTestOnlyBuildConstrained reports whether src's //go:build constraint
// positively requires the "test" tag -- that is, whether the constraint is
// unsatisfiable unless the tag is set.
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
// build tag, i.e. whether "expr is satisfied" implies "tag is set".
//
//   - `A && B` requires the tag if either side does.
//   - `A || B` requires the tag only if both sides do.
//
// negated carries the polarity of the surrounding context (De Morgan swaps
// the operators under negation) and must be false at the top-level call.
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

// CallSitesInSource returns only the calls found by RefsInSource, for
// assertions that concern call sites alone.
func CallSitesInSource(t *testing.T, filename, src string) []CallSite {
	t.Helper()
	calls, _ := RefsInSource(t, filename, src)
	return calls
}

// RefsInSource parses src (Go source for a single file named filename) and
// returns every tracked identity-mutation function that is called, and
// every one that is merely referenced as a value. The local package
// identifier is resolved to its import path via the file's import
// declarations in both cases, so an aliased import cannot bypass the check
// and a same-named local identifier cannot produce a false match.
func RefsInSource(t *testing.T, filename, src string) ([]CallSite, []ValueRef) {
	t.Helper()
	return RefsInSourceWithOptions(t, filename, src, Options{})
}

// RefsInSourceWithOptions is RefsInSource with the tracked function set
// extended by opts. Unqualified entries of opts.Extra (empty ImportPath) are
// matched at call sites only: within a single package a bare identifier is
// read as a value in too many benign ways for a value-reference report to
// carry the meaning it has for a syscall.
func RefsInSourceWithOptions(t *testing.T, filename, src string, opts Options) ([]CallSite, []ValueRef) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	require.NoErrorf(t, err, "failed to parse %s", filename)

	sc := &scanner{
		filename:          filename,
		opts:              opts,
		localToImportPath: resolveLocalImports(t, filename, file),
		calledSelectors:   make(map[*ast.SelectorExpr]struct{}),
	}
	for _, decl := range file.Decls {
		sc.scanDecl(decl)
	}
	return sc.sites, sc.valueRefs
}

// resolveLocalImports maps each import's local identifier (alias if
// present, else the path's last element) to its full import path, so a
// selector's package qualifier can be resolved regardless of aliasing. A
// dot-import of a tracked identity-mutation package is rejected outright,
// since it would make references unqualified and defeat selector-based
// detection.
func resolveLocalImports(t *testing.T, filename string, file *ast.File) map[string]string {
	t.Helper()

	localToImportPath := make(map[string]string)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		require.NoErrorf(t, err, "failed to unquote import path %s in %s", imp.Path.Value, filename)
		if imp.Name != nil && imp.Name.Name == "." {
			require.Falsef(t, isTrackedImportPath(path),
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
	return localToImportPath
}

// scanner accumulates identity-mutation call sites and value references
// while walking a single file's declarations.
type scanner struct {
	filename          string
	opts              Options
	localToImportPath map[string]string
	// calledSelectors marks the selectors that appear in callee position, so
	// the value-reference pass can skip them. ast.Inspect visits a CallExpr
	// before its children, so a selector is always marked before it is
	// reached on its own.
	calledSelectors map[*ast.SelectorExpr]struct{}
	sites           []CallSite
	valueRefs       []ValueRef
}

// trackedSelector reports whether expr is a qualified reference to a tracked
// function: one of the identity-mutation functions of a syscall/unix package
// (e.g. syscall.Seteuid), or an Options.Extra entry naming an import path and
// a function (e.g. flag.Parse). The qualifier is resolved through the file's
// imports in both cases.
func (sc *scanner) trackedSelector(expr ast.Expr) (*ast.SelectorExpr, bool) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	importPath, isImported := sc.localToImportPath[pkgIdent.Name]
	if !isImported {
		// Not a resolved import (e.g. a local variable or type that merely
		// shares the package's name): never treat this as a match.
		return nil, false
	}
	if isTrackedImportPath(importPath) {
		if _, isTrackedFunc := FuncNames[sel.Sel.Name]; isTrackedFunc {
			return sel, true
		}
	}
	for _, extra := range sc.opts.Extra {
		if extra.ImportPath != "" && extra.ImportPath == importPath && extra.FuncName == sel.Sel.Name {
			return sel, true
		}
	}
	return nil, false
}

// trackedUnqualified reports whether name is an Options.Extra entry with an
// empty ImportPath, i.e. a function of the package being scanned that is
// called without a qualifier.
func (sc *scanner) trackedUnqualified(name string) bool {
	for _, extra := range sc.opts.Extra {
		if extra.ImportPath == "" && extra.FuncName == name {
			return true
		}
	}
	return false
}

// scanDecl walks one top-level declaration, labeling any sites found within
// it by the enclosing function/method name (or "package-level" for a
// var/const initializer outside any function body).
func (sc *scanner) scanDecl(decl ast.Decl) {
	funcName := "package-level"
	node := ast.Node(decl)
	if funcDecl, ok := decl.(*ast.FuncDecl); ok {
		if funcDecl.Body == nil {
			return
		}
		funcName = declaredFuncName(funcDecl)
		node = funcDecl.Body
	} else if _, ok := decl.(*ast.GenDecl); !ok {
		return
	}

	ast.Inspect(node, func(n ast.Node) bool {
		sc.visit(n, funcName)
		return true
	})
}

// visit records n as a call site or a value reference if it is a tracked
// identity-mutation function, and is a no-op otherwise.
func (sc *scanner) visit(n ast.Node, funcName string) {
	switch n := n.(type) {
	case *ast.CallExpr:
		// Unwrap parens so a parenthesized callee like (syscall.Seteuid)(0)
		// is still recognized.
		fun := n.Fun
		for {
			paren, ok := fun.(*ast.ParenExpr)
			if !ok {
				break
			}
			fun = paren.X
		}

		sel, ok := sc.trackedSelector(fun)
		if !ok {
			if ident, isIdent := fun.(*ast.Ident); isIdent && sc.trackedUnqualified(ident.Name) {
				sc.sites = append(sc.sites, CallSite{
					FuncName:    funcName,
					SyscallName: ident.Name,
					CallExpr:    ident.Name + "(...)",
					File:        sc.filename,
					Pos:         n.Pos(),
				})
			}
			return
		}
		sc.calledSelectors[sel] = struct{}{}

		sc.sites = append(sc.sites, CallSite{
			FuncName:    funcName,
			SyscallName: sel.Sel.Name,
			CallExpr:    pkgIdentName(sel) + "." + sel.Sel.Name + "(...)",
			File:        sc.filename,
			Pos:         n.Pos(),
		})

	case *ast.SelectorExpr:
		// Reached on its own rather than in callee position: a
		// function-value reference that could be invoked indirectly later.
		if _, isCallee := sc.calledSelectors[n]; isCallee {
			return
		}
		if _, ok := sc.trackedSelector(n); !ok {
			return
		}
		sc.valueRefs = append(sc.valueRefs, ValueRef{
			FuncName: funcName,
			Expr:     pkgIdentName(n) + "." + n.Sel.Name,
		})
	}
}

// declaredFuncName renders a function declaration's name, qualified by its
// receiver type for methods: "(*Manager).escalate" rather than "escalate".
// Without the qualifier, a free function or a method on an unrelated type
// could take the name of an allowed call site and inherit its permission.
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
