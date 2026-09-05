//go:build test

package executor

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file statically checks what the three privilege windows -- the start
// window, the kill window and the staging cleanup window -- can reach. A
// window is a region where the process's effective uid is 0, so every call
// made inside one runs as root; the list of calls that may appear there is a
// security decision (02_architecture.md) and loses its meaning as soon as it
// drifts from the code. The check below turns that list into something the
// build enforces.
//
// Three premises fix its scope. They are stated here rather than left to be
// inferred from the code, because each of them is a place where the check
// deliberately sees less than everything.
//
// 1. What is tracked. Only calls that can have an effect outside the process
// are tracked: functions of the os, syscall, io, os/exec and log/slog
// packages, and methods on *os.File, *os.Process, *exec.Cmd and *slog.Logger.
// Calls to fmt, path/filepath, errors, strconv and the like are not tracked at
// all. The allowlist is therefore a declaration of what the window touches --
// files, descriptors, processes -- and not a transcript of the implementation;
// pure computation may be added inside a window without editing it, while
// nothing that touches the world outside the process can be.
//
// 2. How far reachability is followed. Starting from the function literal
// handed to WithPrivileges, calls are followed through functions and methods
// declared in THIS package only (startPrepared -> stagePrepared -> stageFromFD
// is the real depth). A call that leaves the package is a leaf: it is matched
// against the allowlist by name and not descended into. So the allowlist says
// "the window may call os.MkdirTemp", not "the window may do whatever
// os.MkdirTemp does". Two consequences are deliberate. A function literal
// written inside a reachable function is treated as running in the window even
// if it is merely stored for later (stageFromFD's cleanup closure is exactly
// that), which errs towards reporting more. And a call made through a function
// value -- a parameter or a struct field -- cannot be resolved by this
// analysis at all, so rather than being skipped it must be named in
// privilegeWindowIndirections below, together with the literal it carries;
// an unnamed one fails the check.
//
// 3. How receiver types are resolved. (*os.File).Stat and (*os.Process).Kill
// cannot be told apart from any other Stat or Kill by name, so the package is
// type-checked with go/types, using go/importer's source importer to resolve
// its dependencies. golang.org/x/tools would offer a shorter route but is not
// a dependency of this module, and this check does not justify adding one.
//
// Logging is prohibited in all three windows, and the prohibition is expressed
// by the allowlist naming no Logger method at all. A slog handler is free to
// open a file, and inside a window that open happens at euid 0 -- the same
// hazard this task removed from the output copy goroutine, with no reason to
// keep for log records. (*os.File).WriteString is nonetheless allowed, because
// the danger being avoided is opening a path, not writing as such: writing to
// os.Stderr is a write(2) on a descriptor the invoking user opened before any
// privilege was gained. io.Copy is already on the list, so admitting
// WriteString adds no new class of capability.
//
// Assignments are not calls and are not checked: the start window's
// `pc.execCmd.Path = stagedPath` is out of scope by construction.
//
// Only the files that go/build selects for a production build on the current
// GOOS are analyzed -- the same set the compiler would use, minus _test.go and
// the //go:build test helpers.

// Window names. Each names one region where the effective uid is 0.
const (
	windowStart   = "start_window"
	windowKill    = "kill_window"
	windowCleanup = "cleanup_window"
)

// trackedPackagePaths are the packages whose functions can act on something
// outside this process, and whose calls are therefore matched against the
// allowlist. See premise 1 above for why the set is this narrow.
var trackedPackagePaths = map[string]struct{}{
	"os":       {},
	"os/exec":  {},
	"syscall":  {},
	"io":       {},
	"log/slog": {},
}

// trackedReceiverTypes are the types whose methods are tracked for the same
// reason as trackedPackagePaths. They are written as go/types renders them, so
// a method is matched by its receiver's type rather than by the name of the
// variable it is called on.
var trackedReceiverTypes = map[string]struct{}{
	"*os.File":     {},
	"*os.Process":  {},
	"*exec.Cmd":    {},
	"*slog.Logger": {},
}

// allowedWindowCalls is the allowlist: for each window, every tracked call
// that may be reached from inside it. It transcribes the table in
// 02_architecture.md section 7.2, and no Logger method appears in it.
//
// io.NewSectionReader is on the list although the design table does not name
// it: tracking is by package, and stageFromFD reads the verified descriptor
// through a section reader. Like os.NewFile, which the table does name, it
// opens nothing -- it wraps a descriptor that is already open.
var allowedWindowCalls = map[string]map[string]struct{}{
	windowStart: {
		"(*exec.Cmd).Start":      {},
		"os.MkdirTemp":           {},
		"syscall.Dup":            {},
		"os.NewFile":             {},
		"os.OpenFile":            {},
		"io.Copy":                {},
		"io.NewSectionReader":    {},
		"os.Chmod":               {},
		"os.Chown":               {},
		"os.RemoveAll":           {},
		"(*os.File).Stat":        {},
		"(*os.File).Close":       {},
		"syscall.Close":          {},
		"(*os.File).WriteString": {},
	},
	windowKill: {
		"(*os.Process).Kill": {},
	},
	windowCleanup: {
		"os.RemoveAll":           {},
		"(*os.File).WriteString": {},
	},
}

// privilegeWindowRoots maps the function that opens a window -- the one whose
// body contains the WithPrivileges call -- to the window it opens. The check
// requires this map to match the package's WithPrivileges call sites exactly,
// so a fourth window added anywhere in the package fails until it is declared
// here with an allowlist of its own.
var privilegeWindowRoots = map[string]string{
	"(*DefaultExecutor).executeWithUserGroup": windowStart,
	"(*DefaultExecutor).killChild":            windowKill,
	"(*DefaultExecutor).removeStagedCopy":     windowCleanup,
}

// indirectCallSite names a call made through a function value: the function
// whose body contains the call, and the callee expression as written.
type indirectCallSite struct {
	enclosing string
	callee    string
}

// funcLiteralSite names one function literal by where it is written and what
// it is bound to, rather than by an ordinal, so that inserting an unrelated
// literal nearby cannot silently change which one is meant. Exactly one of
// assignedTo (the literal is assigned to that local variable) and passedTo
// (the literal is the sole function-literal argument of a call to that callee)
// is set.
type funcLiteralSite struct {
	enclosing  string
	assignedTo string
	passedTo   string
}

// privilegeWindowIndirections resolves the calls the analysis cannot follow on
// its own, because the callee is a function value rather than a declared
// function. Every such call reached from a window must appear here; one that
// does not fails the check rather than being quietly ignored, since skipping
// it would hide everything the value points at.
var privilegeWindowIndirections = map[indirectCallSite]funcLiteralSite{
	// executeWithUserGroup's window body calls the start phase through the
	// startWindowFn parameter that runCommand fills in.
	{enclosing: "(*DefaultExecutor).executeWithUserGroup", callee: "fn"}: {
		enclosing: "(*DefaultExecutor).runCommand",
		passedTo:  "startWindow",
	},
	// stageFromFD's own deferred cleanup on an error return.
	{enclosing: "(*DefaultExecutor).stageFromFD", callee: "cleanup"}: {
		enclosing:  "(*DefaultExecutor).stageFromFD",
		assignedTo: "cleanup",
	},
	// The staging cleanup travels to its callers as a field holding the same
	// literal, and is run from both the start window and the cleanup window.
	{enclosing: "(*preparedCommand).runStagingCleanup", callee: "pc.stagingCleanup"}: {
		enclosing:  "(*DefaultExecutor).stageFromFD",
		assignedTo: "cleanup",
	},
}

// windowGuardConfig describes one package to check: where it is, the name to
// type-check it under, which of its functions open which window, and how to
// resolve its calls through function values.
type windowGuardConfig struct {
	dir          string
	pkgPath      string
	roots        map[string]string
	indirections map[indirectCallSite]funcLiteralSite
}

// trackedCall is one tracked call reached from a window.
type trackedCall struct {
	window    string
	name      string // "os.MkdirTemp" or "(*os.File).Close"
	enclosing string // the declared function whose body contains the call
	pos       string
}

// TestPrivilegeWindowAllowedCalls checks that nothing outside the allowlist is
// reachable from inside a privilege window, per window, and that the check
// itself still rejects the two things it exists to reject.
func TestPrivilegeWindowAllowedCalls(t *testing.T) {
	calls, problems := analyzePrivilegeWindows(t, windowGuardConfig{
		dir:          ".",
		pkgPath:      "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor",
		roots:        privilegeWindowRoots,
		indirections: privilegeWindowIndirections,
	})
	for _, problem := range problems {
		t.Error(problem)
	}

	for window, allowed := range allowedWindowCalls {
		t.Run(window, func(t *testing.T) {
			seen := make(map[string]struct{})
			for _, call := range calls {
				if call.window != window {
					continue
				}
				seen[call.name] = struct{}{}
				assert.Containsf(t, allowed, call.name,
					"%s: %s is reachable from the %s but is not on its allowlist (called in %s); either the call belongs outside the window or 02_architecture.md section 7.2 and this list have to change together",
					call.pos, call.name, window, call.enclosing)
			}
			// The reverse direction: an allowlist entry that is no longer
			// reached means the reachability analysis has stopped seeing the
			// window, and the check above would pass while looking at nothing.
			assert.ElementsMatch(t, slices.Sorted(maps.Keys(allowed)), slices.Sorted(maps.Keys(seen)),
				"the %s's allowlist and what is actually reachable from it have diverged; an entry that is never reached means this check may be passing vacuously", window)
		})
	}

	// The two negative self-tests below are what keeps the rest honest. The
	// prohibition on logging in particular is expressed as an absence from the
	// allowlist, and an absence stays green when the analysis breaks.
	t.Run("rejects_unlisted_call", func(t *testing.T) {
		unlisted := unlistedCalls(t, "bad_unlisted_call", "badunlistedcall")
		require.Len(t, unlisted, 1)
		assert.Equal(t, "os.Remove", unlisted[0].name)
	})

	t.Run("rejects_logging_in_window", func(t *testing.T) {
		unlisted := unlistedCalls(t, "bad_logging_in_window", "badlogginginwindow")
		require.Len(t, unlisted, 1)
		assert.Equal(t, "(*slog.Logger).Warn", unlisted[0].name)
	})
}

// unlistedCalls runs the guard over one of the negative-test packages in
// testdata and returns the calls it rejects. Those packages declare a
// WithPrivileges of their own, so what is exercised is the whole analysis --
// root discovery, reachability and the allowlist match -- and not just the
// allowlist.
func unlistedCalls(t *testing.T, dirName, pkgPath string) []trackedCall {
	t.Helper()

	calls, problems := analyzePrivilegeWindows(t, windowGuardConfig{
		dir:     filepath.Join("testdata", dirName),
		pkgPath: pkgPath,
		roots:   map[string]string{"(*runner).startWindowHolder": windowStart},
	})
	require.Emptyf(t, problems, "%s: the negative test data must be analyzable", dirName)

	var unlisted []trackedCall
	for _, call := range calls {
		if _, ok := allowedWindowCalls[call.window][call.name]; !ok {
			unlisted = append(unlisted, call)
		}
	}
	return unlisted
}

// windowGuard holds the parsed and type-checked package being analyzed.
type windowGuard struct {
	fset         *token.FileSet
	pkg          *types.Package
	info         *types.Info
	decls        map[string]*ast.FuncDecl // by receiver-qualified name
	funcBodies   map[*types.Func]*ast.FuncDecl
	indirections map[indirectCallSite]*ast.FuncLit
	problems     []string
}

// analyzePrivilegeWindows returns every tracked call reachable from a
// privilege window in cfg's package, and the problems that stopped the
// analysis from following something.
func analyzePrivilegeWindows(t *testing.T, cfg windowGuardConfig) ([]trackedCall, []string) {
	t.Helper()

	g := loadWindowGuard(t, cfg)
	g.resolveIndirections(cfg.indirections)

	roots := g.findWindowRoots()
	foundIn := make([]string, 0, len(roots))
	for _, root := range roots {
		foundIn = append(foundIn, root.enclosing)
	}
	// Declared roots and actual WithPrivileges call sites must agree: an
	// undeclared one would open a window nothing checks, and a declared one
	// that has disappeared means this check is looking for a window that is no
	// longer there.
	assert.ElementsMatch(t, slices.Sorted(maps.Keys(cfg.roots)), foundIn,
		"the declared privilege windows and the package's WithPrivileges call sites have diverged")

	var calls []trackedCall
	for _, root := range roots {
		window, ok := cfg.roots[root.enclosing]
		if !ok {
			continue // already reported by the assertion above
		}
		visited := make(map[ast.Node]struct{})
		g.walk(window, root.body, root.enclosing, visited, &calls)
	}
	return calls, g.problems
}

// windowRoot is one WithPrivileges call site: the function literal it is
// handed, and the declared function whose body contains it.
type windowRoot struct {
	enclosing string
	body      *ast.FuncLit
}

// findWindowRoots returns every WithPrivileges call site in the package, so
// that a window added without being declared is reported rather than skipped.
func (g *windowGuard) findWindowRoots() []windowRoot {
	var roots []windowRoot
	g.forEachDecl(func(name string, decl *ast.FuncDecl) {
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := g.calleeFunc(call)
			if !ok || fn.Name() != "WithPrivileges" {
				return true
			}
			lit, ok := soleFuncLitArg(call)
			if !ok {
				g.problems = append(g.problems,
					fmt.Sprintf("%s: WithPrivileges in %s is not passed exactly one function literal; the window's body cannot be analyzed",
						g.position(call), name))
				return true
			}
			roots = append(roots, windowRoot{enclosing: name, body: lit})
			return true
		})
	})
	return roots
}

// walk records every tracked call reachable from node, descending into
// functions and methods declared in this package and into the literals named
// in the indirection table. visited keeps a cycle from becoming a hang and is
// per window, so a function reached from two windows is analyzed for each.
func (g *windowGuard) walk(window string, node ast.Node, enclosing string, visited map[ast.Node]struct{}, calls *[]trackedCall) {
	if _, seen := visited[node]; seen {
		return
	}
	visited[node] = struct{}{}

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fun := unparen(call.Fun)

		// A conversion such as int(cred.Gid) is not a call to a function.
		if tv, ok := g.info.Types[fun]; ok && tv.IsType() {
			return true
		}
		// An immediately invoked literal needs no resolution: ast.Inspect
		// walks into its body as a child of this node.
		if _, ok := fun.(*ast.FuncLit); ok {
			return true
		}

		obj := g.calleeObject(fun)
		fn, isFunc := obj.(*types.Func)
		if !isFunc {
			if _, isBuiltin := obj.(*types.Builtin); isBuiltin {
				return true
			}
			g.followIndirect(window, call, fun, enclosing, visited, calls)
			return true
		}

		if fn.Pkg() == g.pkg {
			decl, ok := g.funcBodies[fn]
			if !ok || decl.Body == nil {
				g.problems = append(g.problems,
					fmt.Sprintf("%s: no body found for %s, called from %s; the %s cannot be analyzed",
						g.position(call), fn.Name(), enclosing, window))
				return true
			}
			g.walk(window, decl.Body, declaredFuncName(decl), visited, calls)
			return true
		}

		if name, tracked := trackedCallName(fn); tracked {
			*calls = append(*calls, trackedCall{
				window:    window,
				name:      name,
				enclosing: enclosing,
				pos:       g.position(call),
			})
		}
		return true
	})
}

// followIndirect handles a call whose callee is a function value. The value
// has to have been named in the indirection table: silently ignoring it would
// leave everything it points at unexamined, which is how a call could be moved
// into a window without this check noticing.
func (g *windowGuard) followIndirect(window string, call *ast.CallExpr, fun ast.Expr, enclosing string, visited map[ast.Node]struct{}, calls *[]trackedCall) {
	site := indirectCallSite{enclosing: enclosing, callee: types.ExprString(fun)}
	lit, ok := g.indirections[site]
	if !ok {
		g.problems = append(g.problems,
			fmt.Sprintf("%s: the %s reaches a call through the function value %q in %s, which privilegeWindowIndirections does not resolve; the window cannot be analyzed until that table names the literal the value carries",
				g.position(call), window, site.callee, enclosing))
		return
	}
	g.walk(window, lit.Body, g.enclosingDeclOf(lit), visited, calls)
}

// enclosingDeclOf returns the receiver-qualified name of the declared function
// whose body contains lit, so that calls found inside the literal are reported
// against the function they are written in.
func (g *windowGuard) enclosingDeclOf(lit *ast.FuncLit) string {
	found := ""
	g.forEachDecl(func(name string, decl *ast.FuncDecl) {
		if lit.Pos() >= decl.Pos() && lit.End() <= decl.End() {
			found = name
		}
	})
	return found
}

// resolveIndirections turns each entry of the indirection table into the
// function literal it names, failing loudly when the literal cannot be found
// or is ambiguous rather than leaving the call unresolved.
func (g *windowGuard) resolveIndirections(table map[indirectCallSite]funcLiteralSite) {
	g.indirections = make(map[indirectCallSite]*ast.FuncLit, len(table))
	for site, target := range table {
		decl, ok := g.decls[target.enclosing]
		if !ok {
			g.problems = append(g.problems,
				fmt.Sprintf("privilegeWindowIndirections names %s, which this package does not declare", target.enclosing))
			continue
		}
		lits := findFuncLits(decl, target)
		if len(lits) != 1 {
			g.problems = append(g.problems,
				fmt.Sprintf("privilegeWindowIndirections: %d function literals in %s match %+v, want exactly 1", len(lits), target.enclosing, target))
			continue
		}
		g.indirections[site] = lits[0]
	}
}

// findFuncLits returns the function literals in decl that match target's
// binding: assigned to the named local variable, or passed as the sole
// function-literal argument to the named callee.
func findFuncLits(decl *ast.FuncDecl, target funcLiteralSite) []*ast.FuncLit {
	var lits []*ast.FuncLit
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if target.assignedTo == "" {
				return true
			}
			for i, rhs := range n.Rhs {
				lit, ok := rhs.(*ast.FuncLit)
				if !ok || i >= len(n.Lhs) {
					continue
				}
				if ident, ok := n.Lhs[i].(*ast.Ident); ok && ident.Name == target.assignedTo {
					lits = append(lits, lit)
				}
			}
		case *ast.CallExpr:
			if target.passedTo == "" || types.ExprString(unparen(n.Fun)) != target.passedTo {
				return true
			}
			if lit, ok := soleFuncLitArg(n); ok {
				lits = append(lits, lit)
			}
		}
		return true
	})
	return lits
}

// soleFuncLitArg returns call's only function-literal argument.
func soleFuncLitArg(call *ast.CallExpr) (*ast.FuncLit, bool) {
	var found *ast.FuncLit
	for _, arg := range call.Args {
		lit, ok := arg.(*ast.FuncLit)
		if !ok {
			continue
		}
		if found != nil {
			return nil, false
		}
		found = lit
	}
	return found, found != nil
}

// trackedCallName renders fn as the allowlist writes it -- "os.MkdirTemp" for
// a package function, "(*os.File).Close" for a method -- and reports whether
// it is tracked at all.
func trackedCallName(fn *types.Func) (string, bool) {
	qualifier := func(p *types.Package) string { return p.Name() }
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return "", false
	}
	if recv := sig.Recv(); recv != nil {
		name := "(" + types.TypeString(recv.Type(), qualifier) + ")." + fn.Name()
		recvType := types.TypeString(recv.Type(), qualifier)
		_, tracked := trackedReceiverTypes[recvType]
		return name, tracked
	}
	if fn.Pkg() == nil {
		return "", false
	}
	_, tracked := trackedPackagePaths[fn.Pkg().Path()]
	return fn.Pkg().Name() + "." + fn.Name(), tracked
}

// calleeFunc resolves call's callee to a declared function or method.
func (g *windowGuard) calleeFunc(call *ast.CallExpr) (*types.Func, bool) {
	fn, ok := g.calleeObject(unparen(call.Fun)).(*types.Func)
	return fn, ok
}

// calleeObject resolves a callee expression to the object it names, which is a
// *types.Func for a declared function or method and something else -- a
// variable, a field -- for a call through a function value.
func (g *windowGuard) calleeObject(fun ast.Expr) types.Object {
	switch fun := fun.(type) {
	case *ast.Ident:
		return g.info.Uses[fun]
	case *ast.SelectorExpr:
		return g.info.Uses[fun.Sel]
	default:
		return nil
	}
}

func (g *windowGuard) position(n ast.Node) string {
	pos := g.fset.Position(n.Pos())
	return filepath.Base(pos.Filename) + ":" + fmt.Sprint(pos.Line)
}

// forEachDecl calls fn for every function and method declared in the package,
// in a stable order.
func (g *windowGuard) forEachDecl(fn func(name string, decl *ast.FuncDecl)) {
	for _, name := range slices.Sorted(maps.Keys(g.decls)) {
		fn(name, g.decls[name])
	}
}

// loadWindowGuard parses and type-checks cfg's package. Only the files
// go/build selects for a production build on the current GOOS are read: the
// //go:build test helpers and _test.go files are not part of what runs with
// privilege.
func loadWindowGuard(t *testing.T, cfg windowGuardConfig) *windowGuard {
	t.Helper()

	buildPkg, err := build.ImportDir(cfg.dir, 0)
	require.NoErrorf(t, err, "failed to list the Go files of %s", cfg.dir)
	require.NotEmptyf(t, buildPkg.GoFiles, "%s has no production Go files to analyze", cfg.dir)

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(buildPkg.GoFiles))
	for _, name := range buildPkg.GoFiles {
		file, err := parser.ParseFile(fset, filepath.Join(cfg.dir, name), nil, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "failed to parse %s", name)
		files = append(files, file)
	}

	info := &types.Info{
		Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{},
		// Types is read to tell a conversion, T(x), from a call.
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check(cfg.pkgPath, fset, files, info)
	require.NoErrorf(t, err, "failed to type-check %s; without type information a method cannot be attributed to its receiver's type", cfg.dir)

	g := &windowGuard{
		fset:       fset,
		pkg:        pkg,
		info:       info,
		decls:      make(map[string]*ast.FuncDecl),
		funcBodies: make(map[*types.Func]*ast.FuncDecl),
	}
	for _, file := range files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}
			g.decls[declaredFuncName(funcDecl)] = funcDecl
			if fn, ok := info.Defs[funcDecl.Name].(*types.Func); ok {
				g.funcBodies[fn] = funcDecl
			}
		}
	}
	return g
}

// declaredFuncName renders a declaration's name, qualified by its receiver
// type for methods -- "(*DefaultExecutor).killChild" rather than "killChild"
// -- so that a method on another type cannot be mistaken for one this file
// names.
func declaredFuncName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return decl.Name.Name
	}
	return "(" + types.ExprString(decl.Recv.List[0].Type) + ")." + decl.Name.Name
}

// unparen strips the parentheses around a callee, so that (os.Remove)(dir)
// resolves like os.Remove(dir).
func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}
