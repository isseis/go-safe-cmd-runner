//go:build test

package executor

import (
	"cmp"
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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file statically checks which calls are reachable from the three
// privilege windows -- regions where the process runs with effective uid 0,
// so every call inside one runs as root. The per-window allowlist is a
// security decision (02_architecture.md section 7.2) that loses its meaning
// if it drifts from the code; this check makes the build enforce it.
//
// Four premises fix the scope. They are stated here because each is a place
// where the check deliberately sees less than everything:
//
//  1. What is tracked: everything under this module's path, every interface
//     method whatever its package, and the stdlib packages that can act
//     outside the process (trackedExternalPackagePaths /
//     trackedReceiverTypes). The rest of the stdlib is not tracked.
//     First-party code is tracked wholesale because a helper of this
//     repository can open a file at euid 0 just as os.OpenFile can, and
//     internal/safefileio exists so people reach for it instead of os --
//     exempting it would hide the most natural way to add a file operation.
//     An interface method has no single implementation to inspect, so
//     allowlisting one asserts that every implementation reachable at that
//     point is acceptable at euid 0. The allowlist is thus a declaration of
//     what the window touches, not a transcript of the implementation: pure
//     computation may be added inside a window without editing it.
//  2. How far reachability is followed: from the literal handed to
//     WithPrivileges, through functions declared in THIS package only. A
//     call leaving the package is a leaf matched against the allowlist by
//     name: "the window may call os.MkdirTemp", not "the window may do
//     whatever os.MkdirTemp does". A literal written inside a reachable
//     function counts as running in the window even if it is merely stored
//     for later (stageFromFD's cleanup closure), erring towards reporting
//     more. A call through a function value cannot be resolved here at all
//     and must be named in privilegeWindowIndirections; an unnamed one fails
//     the check.
//  3. How the indirection table is kept honest: naming the literal a
//     function value carries is a claim, and a wrong claim would walk one
//     function while the window runs another -- failing open. The claim is
//     therefore checked (verifyIndirection).
//  4. How receiver types are resolved: (*os.File).Stat and
//     (*os.Process).Kill cannot be told apart from any other Stat or Kill by
//     name, so the package is type-checked with go/types. golang.org/x/tools
//     would be a shorter route but is not a dependency of this module, and
//     this check does not justify adding one.
//
// Logging is prohibited in all three windows by the allowlist naming no
// logging method at all: a slog handler may open a file, and inside a window
// that open happens at euid 0. (*os.File).WriteString is nonetheless allowed
// -- the hazard is opening a path, not writing: writing to os.Stderr is a
// write(2) on a descriptor the invoking user opened before any privilege was
// gained, and io.Copy is already on the list.
//
// Assignments are not calls and are not checked: `pc.execCmd.Path =
// stagedPath` is out of scope by construction.

// Window names. Each names one region where the effective uid is 0.
const (
	windowStart   = "start_window"
	windowKill    = "kill_window"
	windowCleanup = "cleanup_window"
)

// firstPartyModulePrefix is this module's path; everything under it is
// tracked wholesale (premise 1).
const firstPartyModulePrefix = "github.com/isseis/go-safe-cmd-runner/"

// trackedExternalPackagePaths are the packages outside this module whose
// functions can act on something outside this process. path/filepath is
// tracked although most of its use here is pure string work -- Walk, Glob
// and EvalSymlinks are not -- so its pure members are allowlisted per window
// instead. golang.org/x/sys/unix is tracked for the same reason as syscall:
// internal/runner/base/privilege already reaches for it to change the
// process identity.
var trackedExternalPackagePaths = map[string]struct{}{
	"os":            {},
	"os/exec":       {},
	"os/user":       {},
	"syscall":       {},
	"io":            {},
	"net":           {},
	"path/filepath": {},
	"log/slog":      {},

	"golang.org/x/sys/unix": {},
}

// trackedReceiverTypes are the types whose methods are tracked for the same
// reason as trackedExternalPackagePaths, keyed by the receiver's package PATH
// and type name so a package named "os" from another import path cannot pass
// itself off as the standard one. First-party receivers need no entry: their
// package path carries firstPartyModulePrefix.
var trackedReceiverTypes = map[string]struct{}{
	"os.File":         {},
	"os.Process":      {},
	"os/exec.Cmd":     {},
	"log/slog.Logger": {},
}

// allowedWindowCalls is the allowlist, transcribing the table in
// 02_architecture.md section 7.2; no Logger method appears (see the header).
//
// io.NewSectionReader is on the list although the design table does not name
// it: tracking is by package, and stageFromFD reads the verified descriptor
// through a section reader. Like os.NewFile, which the table does name, it
// opens nothing -- it wraps a descriptor that is already open.
var allowedWindowCalls = map[string]map[string]struct{}{
	windowStart: {
		"(*exec.Cmd).Start":          {},
		"os.MkdirTemp":               {},
		"syscall.Dup":                {},
		"os.NewFile":                 {},
		"os.OpenFile":                {},
		"io.Copy":                    {},
		"io.NewSectionReader":        {},
		"filepath.Base":              {},
		"filepath.Join":              {},
		"(fs.FileInfo).Size":         {},
		"(*risktypes.VerifiedFD).Fd": {},
		"os.Chmod":                   {},
		"os.Chown":                   {},
		"os.RemoveAll":               {},
		"(*os.File).Stat":            {},
		"(*os.File).Close":           {},
		"syscall.Close":              {},
		"(*os.File).WriteString":     {},
	},
	windowKill: {
		"(*os.Process).Kill": {},
	},
	windowCleanup: {
		"os.RemoveAll":           {},
		"(*os.File).WriteString": {},
	},
}

// privilegeWindowRoots maps the function whose body contains a WithPrivileges
// call to the window it opens. The check requires this map to match the
// package's call sites exactly, so a fourth window added anywhere in the
// package fails until it is declared here with an allowlist of its own.
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
// assignedTo and passedTo is set.
type funcLiteralSite struct {
	enclosing  string
	assignedTo string
	passedTo   string
}

// privilegeWindowIndirections resolves the calls the analysis cannot follow
// on its own, because the callee is a function value rather than a declared
// function. Every such call reached from a window must appear here; an
// unnamed one fails the check, since skipping it would hide everything the
// value points at.
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
// type-check it under, and how to resolve its windows and calls through
// function values.
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
// itself still rejects what it exists to reject.
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

	for _, window := range slices.Sorted(maps.Keys(allowedWindowCalls)) {
		allowed := allowedWindowCalls[window]
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
			// The reverse direction: an entry that is no longer reached
			// means the analysis has stopped seeing the window, and the
			// check above would pass while looking at nothing.
			assert.ElementsMatch(t, slices.Sorted(maps.Keys(allowed)), slices.Sorted(maps.Keys(seen)),
				"the %s's allowlist and what is actually reachable from it have diverged; an entry that is never reached means this check may be passing vacuously", window)
		})
	}

	// The negative self-tests below are what keeps the rest honest. The
	// prohibition on logging in particular is expressed as an absence from
	// the allowlist, and an absence stays green when the analysis breaks.
	t.Run("rejects_unlisted_call", func(t *testing.T) {
		unlisted := unlistedCalls(t, windowGuardConfig{
			dir:     filepath.Join("testdata", "bad_unlisted_call"),
			pkgPath: "badunlistedcall",
			roots:   map[string]string{"(*runner).startWindowHolder": windowStart},
		})
		require.Len(t, unlisted, 1)
		assert.Equal(t, "os.Remove", unlisted[0].name)
	})

	t.Run("rejects_logging_in_window", func(t *testing.T) {
		unlisted := unlistedCalls(t, windowGuardConfig{
			dir:     filepath.Join("testdata", "bad_logging_in_window"),
			pkgPath: "badlogginginwindow",
			roots:   map[string]string{"(*runner).startWindowHolder": windowStart},
			indirections: map[indirectCallSite]funcLiteralSite{
				{enclosing: "(*runner).report", callee: "warn"}: {
					enclosing:  "(*runner).report",
					assignedTo: "warn",
				},
			},
		})
		require.Len(t, unlisted, 1)
		assert.Equal(t, "(*slog.Logger).Warn", unlisted[0].name)
	})

	// The indirection table is a claim about which literal a function value
	// carries; a claim nothing checks is where the analysis would fail open.
	// This fixture binds the field through a composite literal -- the form
	// the check originally missed.
	t.Run("rejects_rebound_field", func(t *testing.T) {
		_, problems := analyzePrivilegeWindows(t, windowGuardConfig{
			dir:     filepath.Join("testdata", "bad_rebound_field"),
			pkgPath: "badreboundfield",
			roots:   map[string]string{"(*runner).startWindowHolder": windowStart},
			indirections: map[indirectCallSite]funcLiteralSite{
				{enclosing: "(*prepared).runCleanup", callee: "p.cleanup"}: {
					enclosing:  "(*runner).stage",
					assignedTo: "cleanup",
				},
			},
		})
		require.Len(t, problems, 1)
		assert.Contains(t, problems[0], "cleanup is bound to")
	})

	// A positional composite literal binds the field while naming no field to
	// match; the fixture's keyed nil binding keeps the stale-entry fallback
	// from masking the miss, so what is exercised is the positional branch.
	t.Run("rejects_positional_field", func(t *testing.T) {
		_, problems := analyzePrivilegeWindows(t, windowGuardConfig{
			dir:     filepath.Join("testdata", "bad_positional_field"),
			pkgPath: "badpositionalfield",
			roots:   map[string]string{"(*runner).startWindowHolder": windowStart},
			indirections: map[indirectCallSite]funcLiteralSite{
				{enclosing: "(*prepared).runCleanup", callee: "p.cleanup"}: {
					enclosing:  "(*runner).stage",
					assignedTo: "cleanup",
				},
			},
		})
		require.Len(t, problems, 1)
		assert.Contains(t, problems[0], "positional")
	})
}

// unlistedCalls runs the guard over one of the negative-test packages in
// testdata and returns the calls it rejects. The fixtures put the offending
// call a hop away from the window literal, so the whole analysis -- root
// discovery, reachability, the indirection table, the allowlist match -- is
// exercised, not just the allowlist.
func unlistedCalls(t *testing.T, cfg windowGuardConfig) []trackedCall {
	t.Helper()

	calls, problems := analyzePrivilegeWindows(t, cfg)
	require.Emptyf(t, problems, "%s: the negative test data must be analyzable", cfg.dir)

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
	files        []*ast.File
	pkg          *types.Package
	info         *types.Info
	decls        map[string]*ast.FuncDecl // by receiver-qualified name
	funcBodies   map[*types.Func]*ast.FuncDecl
	indirections map[indirectCallSite]resolvedIndirection
	verified     map[indirectCallSite]struct{}
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
	// that has disappeared means this check is looking for a window that is
	// no longer there.
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
	// A literal can be reached twice -- once as part of the body it is
	// written in, once through the indirection table that names it.
	return slices.CompactFunc(slices.SortedFunc(slices.Values(calls), compareTrackedCalls), func(a, b trackedCall) bool {
		return compareTrackedCalls(a, b) == 0
	}), g.problems
}

// compareTrackedCalls orders calls by window, then name, then source position,
// so that duplicates land next to each other and reports are stable.
func compareTrackedCalls(a, b trackedCall) int {
	return cmp.Or(
		cmp.Compare(a.window, b.window),
		cmp.Compare(a.name, b.name),
		cmp.Compare(a.pos, b.pos),
		cmp.Compare(a.enclosing, b.enclosing),
	)
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
// in the indirection table. visited breaks cycles and is per window, so a
// function reached from two windows is analyzed for each.
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

		// An interface method has no single body to follow, so it is never
		// descended into and is always tracked, whichever package it comes
		// from; dropping such calls would hide the mechanism by which most
		// side effects enter this codebase.
		isInterfaceMethod := false
		if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
			_, isInterfaceMethod = sig.Recv().Type().Underlying().(*types.Interface)
		}

		if !isInterfaceMethod && fn.Pkg() == g.pkg {
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

		if name, tracked := trackedCallName(fn); tracked || isInterfaceMethod {
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
// must have been named in the indirection table: silently ignoring it would
// leave everything it points at unexamined.
func (g *windowGuard) followIndirect(window string, call *ast.CallExpr, fun ast.Expr, enclosing string, visited map[ast.Node]struct{}, calls *[]trackedCall) {
	site := indirectCallSite{enclosing: enclosing, callee: types.ExprString(fun)}
	resolved, ok := g.indirections[site]
	if !ok {
		g.problems = append(g.problems,
			fmt.Sprintf("%s: the %s reaches a call through the function value %q in %s, which privilegeWindowIndirections does not resolve; the window cannot be analyzed until that table names the literal the value carries",
				g.position(call), window, site.callee, enclosing))
		return
	}
	lit := resolved.lit
	// The claim is checked once per entry, however many windows reach it.
	if _, done := g.verified[site]; !done {
		g.verified[site] = struct{}{}
		g.verifyIndirection(site, resolved, g.calleeObject(fun))
	}
	litEnclosing, _, ok := g.enclosingDeclOf(lit)
	if !ok {
		g.problems = append(g.problems,
			fmt.Sprintf("%s: the literal privilegeWindowIndirections gives for %q is not inside any declared function of this package",
				g.position(call), site.callee))
		return
	}
	g.walk(window, lit.Body, litEnclosing, visited, calls)
}

// enclosingDeclOf returns the receiver-qualified name of the declared function
// whose body contains n, together with that declaration, so that what is found
// inside a literal is reported against the function it is written in. A node
// written at package level -- a var initializer -- belongs to no declaration
// and reports false.
func (g *windowGuard) enclosingDeclOf(n ast.Node) (string, *ast.FuncDecl, bool) {
	for _, name := range slices.Sorted(maps.Keys(g.decls)) {
		decl := g.decls[name]
		if n.Pos() >= decl.Pos() && n.End() <= decl.End() {
			return name, decl, true
		}
	}
	return "", nil, false
}

// resolveIndirections turns each entry of the indirection table into the
// function literal it names, failing loudly when the literal cannot be found
// or is ambiguous rather than leaving the call unresolved.
func (g *windowGuard) resolveIndirections(table map[indirectCallSite]funcLiteralSite) {
	g.indirections = make(map[indirectCallSite]resolvedIndirection, len(table))
	g.verified = make(map[indirectCallSite]struct{}, len(table))
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
		g.indirections[site] = resolvedIndirection{lit: lits[0], target: target}
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

// resolvedIndirection is one entry of the indirection table after the literal
// it names has been located.
type resolvedIndirection struct {
	lit    *ast.FuncLit
	target funcLiteralSite
}

// verifyIndirection checks the claim an indirection-table entry makes
// (premise 3). A field: every assignment to it in the package must put the
// named literal there, directly or through a local whose only assignment is
// a call to the function that declares the literal, or clear it with nil.
// Anything else (a parameter, as with startWindowFn): the function holding
// the call must at least reach the function that declares the literal, so
// that the value plausibly travels between them.
func (g *windowGuard) verifyIndirection(site indirectCallSite, resolved resolvedIndirection, callee types.Object) {
	if v, ok := callee.(*types.Var); ok {
		if v.IsField() {
			g.verifyFieldCarries(site, v, resolved)
			return
		}
		// A local's claim can be checked outright from its assignments; a
		// parameter has none here and falls through to the structural check.
		if decl, ok := g.decls[site.enclosing]; ok {
			if assignments, carries := g.localBinding(decl, v, resolved); assignments > 0 {
				if !carries {
					g.problems = append(g.problems,
						fmt.Sprintf("privilegeWindowIndirections says %q in %s carries the literal declared in %s, but that local is assigned something else there",
							site.callee, site.enclosing, resolved.target.enclosing))
				}
				return
			}
		}
	}
	if _, ok := g.decls[resolved.target.enclosing]; !ok {
		return // already reported by resolveIndirections
	}
	if !g.calls(site.enclosing, resolved.target.enclosing) {
		g.problems = append(g.problems,
			fmt.Sprintf("privilegeWindowIndirections says %s's %q carries a literal declared in %s, but %s never calls %s, so the value cannot travel between them",
				site.enclosing, site.callee, resolved.target.enclosing, site.enclosing, resolved.target.enclosing))
	}
}

// localBinding returns how many times obj is assigned in decl and whether
// every one of those assignments is the literal the table names.
func (g *windowGuard) localBinding(decl *ast.FuncDecl, obj types.Object, resolved resolvedIndirection) (int, bool) {
	assignments := 0
	carries := true
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || (g.info.Defs[ident] != obj && g.info.Uses[ident] != obj) {
				continue
			}
			assignments++
			if i >= len(assign.Rhs) || assign.Rhs[i] != ast.Expr(resolved.lit) {
				carries = false
			}
		}
		return true
	})
	return assignments, carries
}

// verifyFieldCarries reports every binding of field that does not put the
// literal the table names into it. Whole files are walked, not function
// bodies, because a binding this pass does not see is a binding taken on
// faith: a composite literal (`&preparedCommand{stagingCleanup: fn}` or the
// positional `&preparedCommand{fn}`) binds the field just as an assignment
// does, and a package-level var initializer can hold one outside any
// declaration.
func (g *windowGuard) verifyFieldCarries(site indirectCallSite, field *types.Var, resolved resolvedIndirection) {
	bindings := 0
	check := func(where ast.Node, written string, value ast.Expr) {
		bindings++
		name, decl, _ := g.enclosingDeclOf(where)
		if name == "" {
			name = "the package's declarations"
		}
		if g.carriesLiteral(decl, value, resolved) {
			return
		}
		g.problems = append(g.problems,
			fmt.Sprintf("%s: %s is bound to %s in %s, but privilegeWindowIndirections says it carries the literal declared in %s; the window would run something this analysis never looked at",
				g.position(where), written, types.ExprString(value), name, resolved.target.enclosing))
	}
	for _, file := range g.files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || g.info.Uses[sel.Sel] != field {
						continue
					}
					if len(n.Rhs) != len(n.Lhs) {
						bindings++
						name, _, _ := g.enclosingDeclOf(n)
						g.problems = append(g.problems,
							fmt.Sprintf("%s: %s is assigned from a multi-value expression in %s, which this check cannot trace to the literal privilegeWindowIndirections names for %q",
								g.position(n), types.ExprString(sel), name, site.callee))
						continue
					}
					check(n, types.ExprString(sel), n.Rhs[i])
				}
			case *ast.CompositeLit:
				unkeyed := false
				for _, elt := range n.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						unkeyed = true
						continue
					}
					key, ok := kv.Key.(*ast.Ident)
					if !ok || g.info.Uses[key] != field {
						continue
					}
					check(kv, key.Name, kv.Value)
				}
				// A positional literal names no field to match, but binds the
				// field just as a keyed one does; fail closed, as the
				// multi-value-assignment branch above does.
				if unkeyed && g.literalBindsField(n, field) {
					bindings++
					name, _, _ := g.enclosingDeclOf(n)
					g.problems = append(g.problems,
						fmt.Sprintf("%s: a positional %s literal in %s binds %s, a binding this check cannot trace to the literal privilegeWindowIndirections names for %q",
							g.position(n), types.ExprString(n.Type), name, field.Name(), site.callee))
				}
			}
			return true
		})
	}
	if bindings == 0 {
		g.problems = append(g.problems,
			fmt.Sprintf("privilegeWindowIndirections resolves %q in %s, but nothing in this package binds that field; the entry is stale",
				site.callee, site.enclosing))
	}
}

// literalBindsField reports whether lit constructs the struct field belongs
// to.
func (g *windowGuard) literalBindsField(lit *ast.CompositeLit, field *types.Var) bool {
	typ := g.info.TypeOf(lit.Type)
	if typ == nil {
		return false
	}
	strct, ok := types.Unalias(typ).Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for f := range strct.Fields() {
		if f == field {
			return true
		}
	}
	return false
}

// carriesLiteral reports whether expr, written in decl, puts the named
// literal into the field: nil, the literal itself, or a local assigned
// exactly once from a call to the function that declares the literal.
//
// The last form checks which function the value came from, not which of its
// results it is, so a function returning two closures could hand over the
// other one unnoticed. Telling them apart needs dataflow this analysis does
// not do (premise 3); no such function exists here today, and the allowlist
// still bounds what either closure could call.
func (g *windowGuard) carriesLiteral(decl *ast.FuncDecl, expr ast.Expr, resolved resolvedIndirection) bool {
	if ident, ok := expr.(*ast.Ident); ok && ident.Name == "nil" {
		return true
	}
	if lit, ok := expr.(*ast.FuncLit); ok {
		return lit == resolved.lit
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	// A binding written at package level has no body to trace the name back
	// through, so only the two forms above can be accepted there.
	if decl == nil || decl.Body == nil {
		return false
	}
	obj := g.info.Uses[ident]
	if obj == nil {
		return false
	}
	sources := 0
	fromDeclaringFunc := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			target, ok := lhs.(*ast.Ident)
			if !ok || (g.info.Defs[target] != obj && g.info.Uses[target] != obj) {
				continue
			}
			sources++
			call, ok := singleCall(assign, i)
			if !ok {
				continue
			}
			if fn, ok := g.calleeFunc(call); ok {
				if body, ok := g.funcBodies[fn]; ok && declaredFuncName(body) == resolved.target.enclosing {
					fromDeclaringFunc = true
				}
			}
		}
		return true
	})
	return sources == 1 && fromDeclaringFunc
}

// singleCall returns the call an assignment's right-hand side consists of,
// whether the assignment binds one name or spreads the call's results over
// several.
func singleCall(assign *ast.AssignStmt, i int) (*ast.CallExpr, bool) {
	if len(assign.Rhs) == 1 {
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		return call, ok
	}
	if i >= len(assign.Rhs) {
		return nil, false
	}
	call, ok := assign.Rhs[i].(*ast.CallExpr)
	return call, ok
}

// calls reports whether the function named caller contains a call to the
// function named callee.
func (g *windowGuard) calls(caller, callee string) bool {
	decl, ok := g.decls[caller]
	if !ok {
		return false
	}
	found := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := g.calleeFunc(call); ok {
			if body, ok := g.funcBodies[fn]; ok && declaredFuncName(body) == callee {
				found = true
			}
		}
		return true
	})
	return found
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
		return name, isTrackedReceiver(recv.Type())
	}
	if fn.Pkg() == nil {
		return "", false
	}
	return fn.Pkg().Name() + "." + fn.Name(), isTrackedPackagePath(fn.Pkg().Path())
}

// isTrackedPackagePath reports whether calls into the package at path are
// matched against the allowlist: every package of this module, and the
// standard-library packages that can act outside the process.
func isTrackedPackagePath(path string) bool {
	if strings.HasPrefix(path, firstPartyModulePrefix) {
		return true
	}
	_, tracked := trackedExternalPackagePaths[path]
	return tracked
}

// isTrackedReceiver reports whether methods on typ are matched against the
// allowlist, resolving the receiver to its named type so that a pointer
// receiver and a value receiver answer alike.
func isTrackedReceiver(typ types.Type) bool {
	if ptr, ok := types.Unalias(typ).(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	path := named.Obj().Pkg().Path()
	if strings.HasPrefix(path, firstPartyModulePrefix) {
		return true
	}
	_, tracked := trackedReceiverTypes[path+"."+named.Obj().Name()]
	return tracked
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
		files:      files,
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

// TestPrivilegeWindowGuardLiteralResolution covers the two "the analysis
// cannot go on" branches -- a call carrying more than one function literal,
// and a binding that matches no literal or several -- which the package's
// own shape happens not to exercise.
func TestPrivilegeWindowGuardLiteralResolution(t *testing.T) {
	src := "package p\n" +
		"func twice() {\n" +
		"\trun(func() {})\n" +
		"\trun(func() {})\n" +
		"}\n" +
		"func pair() {\n" +
		"\trun(func() {}, func() {})\n" +
		"}\n" +
		"func rebound() {\n" +
		"\tcleanup := func() {}\n" +
		"\tcleanup = func() {}\n" +
		"\t_ = cleanup\n" +
		"}\n"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "snippet.go", src, parser.SkipObjectResolution)
	require.NoError(t, err)

	decls := make(map[string]*ast.FuncDecl)
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			decls[funcDecl.Name.Name] = funcDecl
		}
	}

	assert.Len(t, findFuncLits(decls["twice"], funcLiteralSite{passedTo: "run"}), 2,
		"two calls to the same callee are ambiguous and must not resolve to one literal")
	assert.Empty(t, findFuncLits(decls["twice"], funcLiteralSite{passedTo: "absent"}),
		"a callee that is never called must resolve to no literal")
	assert.Len(t, findFuncLits(decls["rebound"], funcLiteralSite{assignedTo: "cleanup"}), 2,
		"a local assigned a literal twice is ambiguous")
	assert.Empty(t, findFuncLits(decls["rebound"], funcLiteralSite{assignedTo: "other"}),
		"a name nothing is assigned to must resolve to no literal")

	var pairCall *ast.CallExpr
	ast.Inspect(decls["pair"].Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && pairCall == nil {
			pairCall = call
		}
		return true
	})
	require.NotNil(t, pairCall)
	_, ok := soleFuncLitArg(pairCall)
	assert.False(t, ok, "a call carrying two function literals has no sole one, so the window body is ambiguous")
}
