//go:build test

package runerrors

import (
	"fmt"
	"go/ast"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// runerrorsImportPath declares the shared constructor.
	runerrorsImportPath = "github.com/isseis/go-safe-cmd-runner/internal/runner/runerrors"
	// runerrorsPackageDir is the one directory whose production files may
	// build a PreExecutionError with FailedFilePaths.
	runerrorsPackageDir = "internal/runner/runerrors"
	// loggingImportPath declares PreExecutionError.
	loggingImportPath = "github.com/isseis/go-safe-cmd-runner/internal/logging"
	// loggingPackageDir holds the PreExecutionError type, whose literals may
	// name it without a qualifier.
	loggingPackageDir = "internal/logging"
	// sharedConstructor is the only function allowed to build a
	// PreExecutionError from a verification failure.
	sharedConstructor = "NewVerificationPreExecutionError"
)

// verificationFiringPoints are the two report boundaries that must call the
// shared constructor exactly once each: the group boundary in the runner and
// the global boundary in main.
var verificationFiringPoints = []string{
	"cmd/runner/main.go",
	"internal/runner/runner.go",
}

// TestFiringPointsUseSharedVerificationConstructor fixes that the global and
// group report boundaries build their verification PreExecutionError through
// NewVerificationPreExecutionError rather than by hand. PreExecutionError's
// fields are exported, so the compiler cannot enforce this; the guard
// enumerates the construction forms instead:
//
//   - (a) Each firing point calls the constructor exactly once and no other
//     production file calls it, so a boundary cannot silently stop using it
//     or gain a second, divergent report path.
//   - (b) Outside internal/runner/runerrors, no PreExecutionError literal
//     (value, pointer or elided form) sets FailedFilePaths. A hand-built
//     report that still passed the behavior tests would need this field, so
//     this is the one prohibition that detects a duplicate of the constructor.
//   - (c) Outside internal/runner/runerrors, no selector assignment sets
//     .FailedFilePaths, which would bypass the literal check.
//   - (d) A scan that finds no call fails rather than passing vacuously.
func TestFiringPointsUseSharedVerificationConstructor(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")
	for _, firingPoint := range verificationFiringPoints {
		require.Contains(t, files, firingPoint, "the repository scan must reach the firing point")
	}

	calls := make(map[string]int)
	var violations []string
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, misses := checkVerificationConstructorUse(t, file, src)
		if found > 0 {
			calls[file] = found
		}
		violations = append(violations, misses...)
	}

	require.NotEmpty(t, calls,
		"the scan found no call to %s; the import or file scan is broken", sharedConstructor)
	want := make(map[string]int, len(verificationFiringPoints))
	for _, firingPoint := range verificationFiringPoints {
		want[firingPoint] = 1
	}
	assert.Equal(t, want, calls,
		"each firing point must call %s exactly once and no other production file may call it", sharedConstructor)
	assert.Empty(t, violations,
		"FailedFilePaths may only be set inside %s:\n%s", runerrorsPackageDir, strings.Join(violations, "\n"))
}

// checkVerificationConstructorUse scans one production file and returns the
// number of calls to the shared constructor together with the positions of
// the PreExecutionError literals that set FailedFilePaths and of the selector
// assignments to .FailedFilePaths, both outside internal/runner/runerrors.
func checkVerificationConstructorUse(t *testing.T, filename, src string) (calls int, violations []string) {
	t.Helper()

	fset, file := identitymutationguard.ParseSource(t, filename, src)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == runerrorsImportPath || importPath == loggingImportPath
	})
	dir := path.Dir(filename)
	inRunerrors := dir == runerrorsPackageDir
	inLogging := dir == loggingPackageDir
	isPreExecutionError := func(expr ast.Expr) bool {
		return identitymutationguard.IsNamedType(expr, qualifiers, loggingImportPath, "PreExecutionError", inLogging)
	}

	report := func(pos ast.Node, form string) {
		violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(pos.Pos()), form))
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if identitymutationguard.IsNamedType(node.Fun, qualifiers, runerrorsImportPath, sharedConstructor, inRunerrors) {
				calls++
			}
		case *ast.CompositeLit:
			if inRunerrors {
				return true
			}
			literals := make([]*ast.CompositeLit, 0, 1)
			if isPreExecutionError(node.Type) {
				literals = append(literals, node)
			}
			literals = append(literals, identitymutationguard.ElidedCompositeLiterals(node, isPreExecutionError)...)
			for _, literal := range literals {
				if setsFailedFilePaths(literal) {
					report(literal, "PreExecutionError literal sets FailedFilePaths outside the shared constructor")
				}
			}
		case *ast.AssignStmt:
			if inRunerrors {
				return true
			}
			for _, lhs := range node.Lhs {
				sel, ok := identitymutationguard.UnwrapParen(lhs).(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "FailedFilePaths" {
					report(lhs, "FailedFilePaths assigned outside the shared constructor")
				}
			}
		}
		return true
	})
	return calls, violations
}

// setsFailedFilePaths reports whether the literal sets the FailedFilePaths
// field with a keyed element.
func setsFailedFilePaths(lit *ast.CompositeLit) bool {
	_, ok := identitymutationguard.KeyedElementValue(lit, "FailedFilePaths")
	return ok
}

// TestVerificationConstructorUseCheckRecognizesForms pins the forms the guard
// must count or reject and those it must leave alone, including the aliased
// imports that defeat a plain string search.
func TestVerificationConstructorUseCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport (\n\tl \"github.com/isseis/go-safe-cmd-runner/internal/logging\"\n\tre \"github.com/isseis/go-safe-cmd-runner/internal/runner/runerrors\"\n)\n\nvar _ = l.ErrorTypeSystemError\nvar _ = re.NewVerificationPreExecutionError\n\n"

	tests := []struct {
		name           string
		path           string
		src            string
		wantCalls      int
		wantViolations int
	}{
		{
			name:      "qualified call through an aliased import is counted",
			path:      "internal/x/x.go",
			src:       header + "var e = re.NewVerificationPreExecutionError(nil, l.ErrorTypeSystemError, scope(), \"id\")\n",
			wantCalls: 1,
		},
		{
			name:      "two calls in one file are both counted",
			path:      "internal/x/x.go",
			src:       header + "var a = re.NewVerificationPreExecutionError(nil, l.ErrorTypeSystemError, scope(), \"id\")\nvar b = re.NewVerificationPreExecutionError(nil, l.ErrorTypeSystemError, scope(), \"id\")\n",
			wantCalls: 2,
		},
		{
			name: "a same-named function in another package is not counted",
			path: "internal/x/x.go",
			src:  header + "var e = other.NewVerificationPreExecutionError()\n",
		},
		{
			name:           "pointer literal with FailedFilePaths is reported",
			path:           "internal/x/x.go",
			src:            header + "var e = &l.PreExecutionError{FailedFilePaths: []string{\"/a\"}}\n",
			wantViolations: 1,
		},
		{
			name:           "value literal with FailedFilePaths is reported",
			path:           "internal/x/x.go",
			src:            header + "var e = l.PreExecutionError{FailedFilePaths: nil}\n",
			wantViolations: 1,
		},
		{
			name:           "elided pointer slice element with FailedFilePaths is reported",
			path:           "internal/x/x.go",
			src:            header + "var es = []*l.PreExecutionError{{FailedFilePaths: nil}}\n",
			wantViolations: 1,
		},
		{
			name: "literal without FailedFilePaths is accepted",
			path: "internal/x/x.go",
			src:  header + "var e = &l.PreExecutionError{Type: l.ErrorTypeSystemError}\n",
		},
		{
			name:           "selector assignment is reported",
			path:           "internal/x/x.go",
			src:            header + "func f(pe *l.PreExecutionError) { pe.FailedFilePaths = nil }\n",
			wantViolations: 1,
		},
		{
			name: "literal with FailedFilePaths inside runerrors is accepted",
			path: "internal/runner/runerrors/x.go",
			src:  header + "var e = &l.PreExecutionError{FailedFilePaths: nil}\n",
		},
		{
			name: "selector assignment inside runerrors is accepted",
			path: "internal/runner/runerrors/x.go",
			src:  header + "func f(pe *l.PreExecutionError) { pe.FailedFilePaths = nil }\n",
		},
		{
			name:           "unqualified literal in the declaring package is checked",
			path:           "internal/logging/x.go",
			src:            "package logging\n\nvar e = &PreExecutionError{FailedFilePaths: nil}\n",
			wantViolations: 1,
		},
		{
			name: "a same-named type in another package is not reported",
			path: "internal/x/x.go",
			src:  "package x\n\ntype PreExecutionError struct{ FailedFilePaths []string }\n\nvar e = &PreExecutionError{FailedFilePaths: nil}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, violations := checkVerificationConstructorUse(t, tt.path, tt.src)
			assert.Equal(t, tt.wantCalls, calls)
			assert.Len(t, violations, tt.wantViolations, "violations: %v", violations)
		})
	}
}

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

// TestExportedDeclarationsRecognizesForms pins the recognizer the guard depends
// on: exported functions, types, consts and vars are reported, while
// unexported ones and methods are not. Without this, the guard's non-function
// branches could break and a re-added exported type (the dead classification
// API) would pass unnoticed.
func TestExportedDeclarationsRecognizesForms(t *testing.T) {
	const src = `package p

import "fmt"

func Exported() {}
func unexported() {}

func (r *T) Method() {}

type T struct{}
type hidden struct{}

const ExportedConst = 1
const hiddenConst = 2

var ExportedVar = fmt.Sprint()
var hiddenVar = 1
`
	_, file := identitymutationguard.ParseSource(t, "internal/x/x.go", src)
	assert.ElementsMatch(t,
		[]string{"Exported", "T", "ExportedConst", "ExportedVar"},
		exportedDeclarations(file),
		"only exported top-level funcs, types, consts and vars are reported; methods are excluded")
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
