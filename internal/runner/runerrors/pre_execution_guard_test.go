//go:build test

package runerrors

import (
	"go/ast"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunerrorsExportsOnlyTheSharedConstructor pins the package's public
// surface: the shared constructor is the only exported top-level declaration.
// The removed classification API had no production caller, and keeping a
// single export stops a future caller from reaching for a dead symbol.
func TestRunerrorsExportsOnlyTheSharedConstructor(t *testing.T) {
	files := identitymutationguard.ProductionGoFiles(t, ".")
	require.NotEmpty(t, files, "the package scan returned no production files")

	const constructor = "NewVerificationPreExecutionError"
	var exported []string
	for _, path := range files {
		// #nosec G304 -- path comes from a directory listing of the package
		// under test, not from external input.
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "failed to read %s", path)
		_, file := identitymutationguard.ParseSource(t, path, string(data))
		exported = append(exported, exportedDeclarations(file)...)
	}

	require.Contains(t, exported, constructor,
		"the scan must see the shared constructor; the scan or the constructor is missing")
	assert.ElementsMatch(t, []string{constructor}, exported,
		"runerrors may export only the shared constructor")
}

// exportedDeclarations returns the names of the exported top-level
// declarations in file.
func exportedDeclarations(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.IsExported() {
				names = append(names, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						names = append(names, s.Name.Name)
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.IsExported() {
							names = append(names, name.Name)
						}
					}
				}
			}
		}
	}
	return names
}
