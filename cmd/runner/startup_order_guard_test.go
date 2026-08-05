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
//  2. main calls dropStartupPrivileges before flag.Parse, so no user input is
//     processed with the privileges the process was started with.
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
			"every init function runs before main, and so before the privilege drop; adding one widens what executes while privileged")
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
