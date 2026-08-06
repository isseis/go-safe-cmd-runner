//go:build test

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartupPrivilegeDropOrder statically verifies the startup privilege drop
// of cmd/runner. The order it checks leaves no runtime trace when it is
// correct -- a drop that happens in the right order simply succeeds -- so an
// execution test cannot observe it; the guard reads the source instead.
//
// It asserts four things:
//
//  1. dropStartupPrivileges calls setegid before seteuid, so the effective UID
//     is never surrendered while a privileged group is still held.
//  2. dropStartupPrivileges is the first statement of main's body, and in
//     particular precedes flag.Parse, so no input is processed with the
//     privileges the process was started with. Ordering against flag.Parse
//     alone would not catch a statement inserted above the drop.
//  3. The only identity-mutation calls in this package's production code are
//     those two, and none of those functions is referenced as a value.
//  4. This package still declares exactly one init function, since every init
//     runs before main and therefore before the drop.
func TestStartupPrivilegeDropOrder(t *testing.T) {
	t.Run("setegid precedes seteuid in dropStartupPrivileges", func(t *testing.T) {
		sites, _ := identitymutationguard.FindRefs(t, ".")

		setegid := onlyCallSite(t, sites, "dropStartupPrivileges", "Setegid")
		seteuid := onlyCallSite(t, sites, "dropStartupPrivileges", "Seteuid")

		require.Equal(t, setegid.File, seteuid.File,
			"positions are only comparable within one file")
		assert.Less(t, setegid.Pos, seteuid.Pos,
			"setegid must be called before seteuid in dropStartupPrivileges")
	})

	t.Run("privilege drop precedes flag parsing in main", func(t *testing.T) {
		sites, _ := identitymutationguard.FindRefsWithOptions(t, ".", startupOrderOptions())

		drop := onlyCallSite(t, sites, "main", "dropStartupPrivileges")
		parse := onlyCallSite(t, sites, "main", "Parse")

		require.Equal(t, drop.File, parse.File,
			"positions are only comparable within one file")
		assert.Less(t, drop.Pos, parse.Pos,
			"main must drop privileges before parsing flags")
	})

	t.Run("privilege drop is main's first statement", func(t *testing.T) {
		body := mainFuncBody(t, "main.go")
		require.NotEmpty(t, body.List, "main must have a body")

		assert.True(t, callsDropStartupPrivileges(body.List[0]),
			"the privilege drop must be main's very first statement: anything placed above it runs with the privileges the process was started with, and ordering it against flag.Parse alone would not detect that")
	})

	t.Run("identity mutation is confined to dropStartupPrivileges", func(t *testing.T) {
		sites, valueRefs := identitymutationguard.FindRefs(t, ".")

		for _, ref := range valueRefs {
			t.Errorf("identity-mutation function %s referenced as a value in function %s; such a reference can be invoked later through the variable or field",
				ref.Expr, ref.FuncName)
		}

		allowed := map[string]struct{}{"Setegid": {}, "Seteuid": {}}
		for _, site := range sites {
			if site.FuncName != "dropStartupPrivileges" {
				t.Errorf("identity-mutation call %s outside dropStartupPrivileges, in function %s", site.CallExpr, site.FuncName)
				continue
			}
			if _, ok := allowed[site.SyscallName]; !ok {
				t.Errorf("unexpected identity-mutation call %s in dropStartupPrivileges", site.CallExpr)
			}
		}
		assert.Len(t, sites, len(allowed), "dropStartupPrivileges must contain exactly the allowed calls")
	})

	t.Run("package declares exactly one init function", func(t *testing.T) {
		count := 0
		for _, path := range identitymutationguard.ProductionGoFiles(t, ".") {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			require.NoErrorf(t, err, "failed to parse %s", path)
			for _, decl := range file.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Recv == nil && funcDecl.Name.Name == "init" {
					count++
				}
			}
		}
		assert.Equal(t, 1, count,
			"every init function runs before main, and so before the privilege drop; adding one widens what executes while privileged. Package-level variable initializers run just as early: they are not counted here, but the previous subtest does reject an identity-mutation call in one")
	})

	t.Run("control: the order assertions fail on reversed source", func(t *testing.T) {
		reversed := "package main\n" +
			"import (\n\t\"flag\"\n\t\"syscall\"\n)\n" +
			"func dropStartupPrivileges(uid, gid int) error {\n" +
			"\tif err := syscall.Seteuid(uid); err != nil { return err }\n" +
			"\treturn syscall.Setegid(gid)\n" +
			"}\n" +
			"func main() {\n" +
			"\tflag.Parse()\n" +
			"\t_ = dropStartupPrivileges(syscall.Getuid(), syscall.Getgid())\n" +
			"}\n"

		sites, _ := identitymutationguard.RefsInSourceWithOptions(t, "reversed.go", reversed, startupOrderOptions())

		setegid := onlyCallSite(t, sites, "dropStartupPrivileges", "Setegid")
		seteuid := onlyCallSite(t, sites, "dropStartupPrivileges", "Seteuid")
		assert.Greater(t, setegid.Pos, seteuid.Pos,
			"the scan must see the reversed order inside the function body, not merely find both calls")

		drop := onlyCallSite(t, sites, "main", "dropStartupPrivileges")
		parse := onlyCallSite(t, sites, "main", "Parse")
		assert.Greater(t, drop.Pos, parse.Pos,
			"the scan must see the reversed order inside main's body")
	})
}

// mainFuncBody returns the body of the main function declared in path.
func mainFuncBody(t *testing.T, path string) *ast.BlockStmt {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoErrorf(t, err, "failed to parse %s", path)
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Recv == nil && funcDecl.Name.Name == "main" {
			return funcDecl.Body
		}
	}
	t.Fatalf("no main function declared in %s", path)
	return nil
}

// callsDropStartupPrivileges reports whether stmt contains a call to
// dropStartupPrivileges anywhere within it, so the guard accepts the call
// wrapped in an if-statement initializer as main writes it today, but not a
// call moved below some other statement.
func callsDropStartupPrivileges(stmt ast.Stmt) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "dropStartupPrivileges" {
			found = true
			return false
		}
		return true
	})
	return found
}

// startupOrderOptions tracks the two calls whose order within main matters and
// that the default syscall/unix scan does not cover: flag.Parse (qualified) and
// dropStartupPrivileges (unqualified, declared in this package).
func startupOrderOptions() identitymutationguard.Options {
	return identitymutationguard.Options{
		Extra: []identitymutationguard.ExtraTrackedFunc{
			{ImportPath: "flag", FuncName: "Parse"},
			{ImportPath: "", FuncName: "dropStartupPrivileges"},
		},
	}
}

// onlyCallSite returns the single call to funcName's tracked callee named
// calleeName, failing the test if there is not exactly one. Requiring exactly
// one guards against an order assertion that passes vacuously because the scan
// found nothing.
func onlyCallSite(t *testing.T, sites []identitymutationguard.CallSite, funcName, calleeName string) identitymutationguard.CallSite {
	t.Helper()

	var matches []identitymutationguard.CallSite
	for _, site := range sites {
		if site.FuncName == funcName && site.SyscallName == calleeName {
			matches = append(matches, site)
		}
	}
	require.Lenf(t, matches, 1, "expected exactly one call to %s in %s, got %d", calleeName, funcName, len(matches))
	return matches[0]
}
