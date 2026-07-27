//go:build !windows

package privilege

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// identityMutationPkgNames lists the package identifiers (as they appear in
// source, not import paths) whose identity-mutation functions this test
// tracks.
var identityMutationPkgNames = map[string]struct{}{
	"syscall": {},
	"unix":    {}, // golang.org/x/sys/unix
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
// parsing the package's source files.
type identityMutationCallSite struct {
	funcName string
	callExpr string // rendered "pkg.Func(args)" for failure messages
	arg      string
}

// TestNoUnexpectedIdentityMutationSyscalls statically verifies that
// syscall/unix identity-mutation functions (Seteuid, Setegid, ...) are only
// called from escalatePrivileges (elevate to root) and restorePrivileges
// (revert to the original UID), each with the exact argument expected. This
// is the static replacement for a dynamic check that non-privileged CI
// cannot perform, since a demotion syscall that fails with EPERM leaves
// identity unchanged and so cannot be distinguished from one that was never
// called.
//
// This test parses every .go file in this package directory with
// go/parser.ParseFile rather than go/parser.ParseDir: ParseDir is deprecated
// as of Go 1.22, and this repository's staticcheck configuration does not
// exempt _test.go files from SA1019. It deliberately does not interpret
// build tags, so identity_linux.go and identity_other.go are both parsed
// regardless of GOOS. Neither calls an identity-mutation function, so this
// is safe, and it means the check applies uniformly across platforms rather
// than only the one running the test.
//
// What this test cannot detect: a function value reference to an
// identity-mutation function (e.g. a struct literal field initialized as
// `syscallSeteuid: syscall.Seteuid`) or an indirect call through an injected
// function field (e.g. `m.syscallSetegid(gid)`). It only recognizes
// qualified call expressions of the form `syscall.Seteuid(...)` or
// `unix.Seteuid(...)`. Both gaps currently exist in this package
// (UnixPrivilegeManager.syscallSeteuid / syscallSetegid, and their use in
// changeUserGroupInternal) and are closed in a later step that removes
// those injected fields entirely and extends this test to also reject bare
// function-value references to identity-mutation functions.
func TestNoUnexpectedIdentityMutationSyscalls(t *testing.T) {
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
			t.Errorf("expected call %s(%s) not found in function %s; the check may be vacuously passing",
				"<identity-mutation func>", allowed.arg, allowed.funcName)
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
func findIdentityMutationCallSites(t *testing.T, dir string) []identityMutationCallSite {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory %s: %v", dir, err)
	}

	var sites []identityMutationCallSite
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", path, err)
		}

		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			sites = append(sites, findIdentityMutationCallsInFunc(funcDecl)...)
		}
	}

	return sites
}

// findIdentityMutationCallsInFunc walks funcDecl's body and returns every
// call expression of the form `pkg.Func(args)` where pkg is a tracked
// package identifier and Func is a tracked identity-mutation function name.
func findIdentityMutationCallsInFunc(funcDecl *ast.FuncDecl) []identityMutationCallSite {
	if funcDecl.Body == nil {
		return nil
	}

	var sites []identityMutationCallSite
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
		if _, isTrackedPkg := identityMutationPkgNames[pkgIdent.Name]; !isTrackedPkg {
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

	return sites
}
