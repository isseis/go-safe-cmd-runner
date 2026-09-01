//go:build test

// Package sourceorder finds where named references appear inside one
// function's body, so that a guard test can assert the order of two startup
// steps whose correct order leaves no runtime trace. A startup step that runs
// in the right order simply succeeds, so an execution test cannot observe the
// ordering; these helpers read the source instead.
//
// It complements identitymutationguard, whose scan resolves qualified calls
// through a file's imports. That scan cannot see a reference such as
// pkg.New().Method, where the qualifier is a call rather than a package name,
// nor a method value that is called later through a variable. Matching is by
// identifier name alone, which over-reports rather than under-reports; OnlyRef
// therefore requires exactly one match so that an order assertion cannot pass
// because the scan found nothing.
package sourceorder

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Body returns the body of the function named funcName declared in the file
// at path.
func Body(t *testing.T, path, funcName string) *ast.BlockStmt {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoErrorf(t, err, "failed to parse %s", path)
	return funcBody(t, file, path, funcName)
}

// BodyInSource is Body for source held in memory, so that a test can check
// its own scan against source it reordered on purpose.
func BodyInSource(t *testing.T, filename, src, funcName string) *ast.BlockStmt {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	require.NoErrorf(t, err, "failed to parse %s", filename)
	return funcBody(t, file, filename, funcName)
}

func funcBody(t *testing.T, file *ast.File, filename, funcName string) *ast.BlockStmt {
	t.Helper()

	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Recv == nil && funcDecl.Name.Name == funcName {
			require.NotNilf(t, funcDecl.Body, "%s has no body in %s", funcName, filename)
			return funcDecl.Body
		}
	}
	t.Fatalf("no function %s declared in %s", funcName, filename)
	return nil
}

// OnlyRef returns the position of the single identifier named name within
// body, failing the test if there is not exactly one. A selector's field
// counts, so both pkg.Name and value.Name are found. Positions are comparable
// only between references taken from the same body, since every parse uses its
// own token.FileSet.
func OnlyRef(t *testing.T, body *ast.BlockStmt, name string) token.Pos {
	t.Helper()

	var positions []token.Pos
	ast.Inspect(body, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == name {
			positions = append(positions, ident.Pos())
		}
		return true
	})
	require.Lenf(t, positions, 1, "expected exactly one reference to %s, got %d", name, len(positions))
	return positions[0]
}

// AssertUIDResolvedBeforeCheck verifies that run, declared in the file at
// mainPath, resolves the permission check UID before it calls checkFuncName.
//
// EnsurePermissionCheckUID is also what settles the group-enumeration
// completeness classification for the process, and that classification is
// settled at startup or not at all: an enumeration reached before it denies.
// The order therefore decides whether the warning about a host this build
// cannot enumerate precedes the first denial over it. Getting it right leaves
// no runtime trace, so this checks the source rather than an execution test.
//
// It also runs a control subtest against source it reorders on purpose, so
// that the order assertion is shown to fail when the order is actually
// wrong.
func AssertUIDResolvedBeforeCheck(t *testing.T, mainPath, checkFuncName string) {
	t.Helper()

	t.Run("run resolves the permission check UID before checking directories", func(t *testing.T) {
		body := Body(t, mainPath, "run")

		ensure := OnlyRef(t, body, "EnsurePermissionCheckUID")
		check := OnlyRef(t, body, checkFuncName)

		assert.Less(t, ensure, check,
			"run must resolve the permission check UID, and with it settle the enumeration completeness classification, before the first decision that depends on it")
	})

	t.Run("control: the order assertion fails on reordered source", func(t *testing.T) {
		reordered := fmt.Sprintf(
			"package main\n"+
				"func run() int {\n"+
				"\t_, _ = %s(nil, deps{}, nil)\n"+
				"\tensureUID := groupmembership.New().EnsurePermissionCheckUID\n"+
				"\t_ = ensureUID()\n"+
				"\treturn 0\n"+
				"}\n", checkFuncName)

		body := BodyInSource(t, "reordered.go", reordered, "run")

		ensure := OnlyRef(t, body, "EnsurePermissionCheckUID")
		check := OnlyRef(t, body, checkFuncName)
		assert.Greater(t, ensure, check,
			"the scan must see the reordered source inside run's body, not merely find both references")
	})
}
