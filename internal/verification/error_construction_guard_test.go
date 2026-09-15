//go:build test

package verification

import (
	"fmt"
	"go/ast"
	"path"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// verificationImportPath is the package whose Error type may only be built
	// by the private constructor.
	verificationImportPath = "github.com/isseis/go-safe-cmd-runner/internal/verification"
	// verificationPackageDir is the directory of this package, where the
	// unqualified Error identifier also names verification.Error.
	verificationPackageDir = "internal/verification"
	// errorConstructorFile and errorConstructorFunc locate the one function
	// allowed to build an Error.
	errorConstructorFile = "internal/verification/manager.go"
	errorConstructorFunc = "newVerificationError"
)

// TestVerificationErrorLiteralsOnlyInConstructor fixes the three construction
// forms of verification.Error and the field assignment that could bypass them.
// The type's fields are exported (the task deliberately does not change the
// type), so the compiler cannot enforce a single construction point; this
// go/ast guard does.
//
//   - (a) A composite literal naming verification.Error -- the qualified
//     verification.Error{...} form anywhere, and the unqualified Error{...}
//     form inside internal/verification -- must appear only inside
//     newVerificationError. Unqualified Error in other packages names a
//     different type (e.g. privilege.Error in internal/runner/base/privilege),
//     so scanning for it repository-wide would be a false positive.
//   - (b) Inside internal/verification, a selector assignment to .Details must
//     not appear outside newVerificationError. This check is deliberately
//     incomplete in the same way as TestNotificationContextBuiltOnlyByConstructors:
//     an AST scan without type information would match an unrelated type's
//     .Details field, so it is limited to this package where the unqualified
//     Error identifier is unambiguous.
//   - (c) A scan that finds no literal fails rather than passing vacuously.
func TestVerificationErrorLiteralsOnlyInConstructor(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var (
		literals             int
		literalViolations    []string
		assignmentViolations []string
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, literalMisses, assignmentMisses := checkVerificationErrorConstruction(t, file, src)
		literals += found
		literalViolations = append(literalViolations, literalMisses...)
		assignmentViolations = append(assignmentViolations, assignmentMisses...)
	}

	require.NotZero(t, literals,
		"the scan found no verification.Error literal; the scan is broken")
	assert.Empty(t, literalViolations,
		"verification.Error literals may only be built in %s:\n%s",
		errorConstructorFunc, strings.Join(literalViolations, "\n"))
	assert.Empty(t, assignmentViolations,
		"Error.Details may only be set in %s:\n%s",
		errorConstructorFunc, strings.Join(assignmentViolations, "\n"))
}

// checkVerificationErrorConstruction scans one production file and returns the
// number of verification.Error literals found (value, pointer and elided
// forms) together with the positions of those built outside the constructor
// and of any assignment to an Error.Details selector outside it.
func checkVerificationErrorConstruction(t *testing.T, filename, src string) (found int, literalViolations, assignmentViolations []string) {
	t.Helper()

	fset, file := identitymutationguard.ParseSource(t, filename, src)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == verificationImportPath
	})
	inPackage := path.Dir(filename) == verificationPackageDir
	isError := func(expr ast.Expr) bool {
		return identitymutationguard.IsNamedType(expr, qualifiers, verificationImportPath, "Error", inPackage)
	}

	for _, decl := range file.Decls {
		funcName := ""
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcName = fn.Name.Name
		}
		allowed := filename == errorConstructorFile && funcName == errorConstructorFunc

		ast.Inspect(decl, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				literals := make([]*ast.CompositeLit, 0, 1)
				if isError(node.Type) {
					literals = append(literals, node)
				}
				literals = append(literals, identitymutationguard.ElidedCompositeLiterals(node, isError)...)
				for _, literal := range literals {
					found++
					if !allowed {
						literalViolations = append(literalViolations, fmt.Sprintf(
							"%s: verification.Error literal built outside %s", fset.Position(literal.Pos()), errorConstructorFunc))
					}
				}
			case *ast.AssignStmt:
				if !inPackage || allowed {
					return true
				}
				for _, lhs := range node.Lhs {
					sel, ok := identitymutationguard.UnwrapParen(lhs).(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "Details" {
						continue
					}
					assignmentViolations = append(assignmentViolations, fmt.Sprintf(
						"%s: Error.Details assigned outside %s", fset.Position(lhs.Pos()), errorConstructorFunc))
				}
			}
			return true
		})
	}
	return found, literalViolations, assignmentViolations
}

// TestVerificationErrorConstructionCheckRecognizesForms pins the forms the
// guard must recognize: the qualified literal, the unqualified literal inside
// the package, the pointer and elided forms, and the Detail assignment, plus
// the forms it must leave alone.
func TestVerificationErrorConstructionCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport v \"github.com/isseis/go-safe-cmd-runner/internal/verification\"\n\nvar _ = v.ErrConfigNil\n\n"

	tests := []struct {
		name           string
		path           string
		src            string
		wantLiterals   int
		wantViolations int
	}{
		{
			name:           "qualified literal outside the package is reported",
			path:           "internal/x/x.go",
			src:            header + "var e = &v.Error{Op: \"x\"}\n",
			wantLiterals:   1,
			wantViolations: 1,
		},
		{
			name:           "value literal outside the package is reported",
			path:           "internal/x/x.go",
			src:            header + "var e = v.Error{Op: \"x\"}\n",
			wantLiterals:   1,
			wantViolations: 1,
		},
		{
			name:           "elided pointer literal outside the package is reported",
			path:           "internal/x/x.go",
			src:            header + "var es = []*v.Error{{Op: \"x\"}}\n",
			wantLiterals:   1,
			wantViolations: 1,
		},
		{
			name:           "unqualified literal inside the package is reported",
			path:           "internal/verification/x.go",
			src:            "package verification\n\nvar e = &Error{Op: \"x\"}\n",
			wantLiterals:   1,
			wantViolations: 1,
		},
		{
			name:           "literal in the constructor is accepted",
			path:           errorConstructorFile,
			src:            "package verification\n\nfunc newVerificationError() *Error { return &Error{Op: \"x\"} }\n",
			wantLiterals:   1,
			wantViolations: 0,
		},
		{
			name:           "literal in another function of the constructor file is reported",
			path:           errorConstructorFile,
			src:            "package verification\n\nfunc helper() *Error { return &Error{Op: \"x\"} }\n",
			wantLiterals:   1,
			wantViolations: 1,
		},
		{
			name:           "a same-named type in another package is not reported",
			path:           "internal/x/x.go",
			src:            "package x\n\ntype Error struct{ Op string }\n\nvar e = &Error{Op: \"x\"}\n",
			wantLiterals:   0,
			wantViolations: 0,
		},
		{
			name:           "a different type inside the package is not reported",
			path:           "internal/verification/x.go",
			src:            "package verification\n\nvar e = &OpError{Op: \"x\"}\n",
			wantLiterals:   0,
			wantViolations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, literalViolations, _ := checkVerificationErrorConstruction(t, tt.path, tt.src)
			assert.Equal(t, tt.wantLiterals, found)
			assert.Len(t, literalViolations, tt.wantViolations, "violations: %v", literalViolations)
		})
	}

	t.Run("Details assignment inside the package outside the constructor is reported", func(t *testing.T) {
		src := "package verification\n\nfunc helper(e *Error) { e.Details = []string{\"x\"} }\n"
		_, literalViolations, assignmentViolations := checkVerificationErrorConstruction(t, "internal/verification/x.go", src)
		assert.Empty(t, literalViolations)
		require.Len(t, assignmentViolations, 1, "violations: %v", assignmentViolations)
		assert.Contains(t, assignmentViolations[0], "Error.Details assigned")
	})

	t.Run("Details assignment in the constructor is accepted", func(t *testing.T) {
		src := "package verification\n\nfunc newVerificationError(e *Error) { e.Details = []string{\"x\"} }\n"
		_, _, assignmentViolations := checkVerificationErrorConstruction(t, errorConstructorFile, src)
		assert.Empty(t, assignmentViolations)
	})

	t.Run("Details assignment in another package is not reported", func(t *testing.T) {
		src := "package x\n\nfunc helper(r *result) { r.Details = []string{\"x\"} }\n"
		_, _, assignmentViolations := checkVerificationErrorConstruction(t, "internal/x/x.go", src)
		assert.Empty(t, assignmentViolations)
	})
}
