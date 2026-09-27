package output

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// captureErrorFile is the file that declares CaptureError and its two
	// constructors.
	captureErrorFile = "internal/runner/base/output/errors.go"
	// captureErrorPackageDir is the package whose unqualified CaptureError
	// form the scan resolves.
	captureErrorPackageDir = "internal/runner/base/output"
	// captureErrorImportPath resolves the qualified output.CaptureError form.
	captureErrorImportPath = "github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	// captureErrorConstructors names the functions allowed to build a
	// CaptureError, for the violation messages.
	captureErrorConstructors = "newSizeLimitError or newFileSystemError"
)

// captureErrorConstructorNames are the functions allowed to build a
// CaptureError.
var captureErrorConstructorNames = map[string]struct{}{
	"newSizeLimitError":  {},
	"newFileSystemError": {},
}

// captureErrorFields are the private fields that only the constructors may set.
var captureErrorFields = []string{"typ", "path", "phase", "cause", "limit"}

// TestProductionCaptureErrorLiteralsUseConstructors fixes the construction
// forms of CaptureError and the field mutation that could bypass them. The
// fields are unexported, so other packages cannot build one (the compiler
// rejects it), but a same-package production file could still build the struct
// or assign its fields directly; this go/ast guard is what keeps construction
// in the two constructors.
//
//   - A composite literal naming CaptureError -- the qualified
//     output.CaptureError{...} form anywhere, and the unqualified form inside
//     internal/runner/base/output -- must appear only inside newSizeLimitError
//     or newFileSystemError. The new(CaptureError) call form is reported the
//     same way.
//   - The qualified form is checked repository-wide: another package can write
//     an empty zero-value literal even though it cannot set the fields, so the
//     compiler alone does not confine construction to this package.
//   - Inside internal/runner/base/output, a selector assignment or increment of
//     .typ, .path, .phase, .cause or .limit is reported outside those two
//     constructors. The scan is limited to this package because the unqualified
//     type name is unambiguous there; scanning wider would match unrelated
//     selector assignments to fields sharing those names.
//   - The field-name match carries no type information, so a future struct
//     added directly under this package with a field of the same name would be
//     a false positive. That is the known maintenance obligation of this
//     incomplete (by design) scan, as is a mutation through a type alias or an
//     indirect left-hand side such as x.limit[i] = ... or *x.limit = ... .
//   - Test files are not scanned: they legitimately build values through the
//     test-only builder and through the production constructors.
//   - A scan that finds no literal fails rather than passing vacuously.
func TestProductionCaptureErrorLiteralsUseConstructors(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var (
		literals             int
		literalViolations    []string
		assignmentViolations []string
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, lv, av := checkCaptureErrorConstruction(t, file, src)
		literals += found
		literalViolations = append(literalViolations, lv...)
		assignmentViolations = append(assignmentViolations, av...)
	}

	require.NotZero(t, literals,
		"the scan found no capture error literal; the scan is broken")
	assert.Empty(t, literalViolations,
		"capture error literals may only be built in %s:\n%s",
		captureErrorConstructors, strings.Join(literalViolations, "\n"))
	assert.Empty(t, assignmentViolations,
		"capture error fields may only be set in %s:\n%s",
		captureErrorConstructors, strings.Join(assignmentViolations, "\n"))
}

// checkCaptureErrorConstruction scans one production file and returns the
// number of CaptureError literals found (value, pointer and elided forms)
// together with the positions of those built outside the two constructors and
// of any assignment or increment of one of its private fields outside them.
func checkCaptureErrorConstruction(t *testing.T, filename, src string) (found int, literalViolations, assignmentViolations []string) {
	t.Helper()

	fset, file := identitymutationguard.ParseSource(t, filename, src)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == captureErrorImportPath
	})
	inPackage := path.Dir(filename) == captureErrorPackageDir
	isType := func(expr ast.Expr) bool {
		return identitymutationguard.IsNamedType(expr, qualifiers, captureErrorImportPath, "CaptureError", inPackage)
	}

	for _, decl := range file.Decls {
		funcName := ""
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcName = fn.Name.Name
		}
		_, isConstructor := captureErrorConstructorNames[funcName]
		allowed := filename == captureErrorFile && isConstructor

		ast.Inspect(decl, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				literals := make([]*ast.CompositeLit, 0, 1)
				if isType(node.Type) {
					literals = append(literals, node)
				}
				literals = append(literals, identitymutationguard.ElidedCompositeLiterals(node, isType)...)
				for _, literal := range literals {
					found++
					if !allowed {
						literalViolations = append(literalViolations, fmt.Sprintf(
							"%s: capture error literal built outside %s", fset.Position(literal.Pos()), captureErrorConstructors))
					}
				}
			case *ast.CallExpr:
				if !isNewCaptureError(node, isType) {
					return true
				}
				found++
				if !allowed {
					literalViolations = append(literalViolations, fmt.Sprintf(
						"%s: capture error built with new outside %s", fset.Position(node.Pos()), captureErrorConstructors))
				}
			case *ast.AssignStmt:
				if !inPackage || allowed {
					return true
				}
				for _, lhs := range node.Lhs {
					reportCaptureErrorFieldMutation(fset, lhs, "assigned", &assignmentViolations)
				}
			case *ast.IncDecStmt:
				if !inPackage || allowed {
					return true
				}
				reportCaptureErrorFieldMutation(fset, node.X, "modified", &assignmentViolations)
			}
			return true
		})
	}
	return found, literalViolations, assignmentViolations
}

// isNewCaptureError reports whether call is new(T) for a T the type check
// resolves to CaptureError.
func isNewCaptureError(call *ast.CallExpr, isType func(ast.Expr) bool) bool {
	ident, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.Ident)
	if !ok || ident.Name != "new" || len(call.Args) != 1 {
		return false
	}
	return isType(call.Args[0])
}

// reportCaptureErrorFieldMutation appends a violation when expr is a selector
// for one of CaptureError's private fields.
func reportCaptureErrorFieldMutation(fset *token.FileSet, expr ast.Expr, verb string, violations *[]string) {
	sel, ok := identitymutationguard.UnwrapParen(expr).(*ast.SelectorExpr)
	if !ok || !slices.Contains(captureErrorFields, sel.Sel.Name) {
		return
	}
	*violations = append(*violations, fmt.Sprintf(
		"%s: capture error field .%s %s outside %s", fset.Position(expr.Pos()), sel.Sel.Name, verb, captureErrorConstructors))
}

// TestCaptureErrorConstructionCheckRecognizesForms pins the forms the guard
// must recognize, so the guard cannot become a no-op that still passes the
// repository scan.
func TestCaptureErrorConstructionCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport o \"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output\"\n\n"

	literalTests := []struct {
		name              string
		path              string
		src               string
		wantLiterals      int
		wantLiteralMisses int
	}{
		{
			name:              "qualified pointer literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var e = &o.CaptureError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "value literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var e = o.CaptureError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "elided pointer literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var es = []*o.CaptureError{{}}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "positional value literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var e = o.CaptureError{0, \"p\", 1, nil, 0}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "unqualified literal inside the package is reported",
			path:              "internal/runner/base/output/x.go",
			src:               "package output\n\nvar e = &CaptureError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "literal in a constructor is accepted",
			path:              captureErrorFile,
			src:               "package output\n\nfunc newSizeLimitError() *CaptureError { return &CaptureError{} }\n",
			wantLiterals:      1,
			wantLiteralMisses: 0,
		},
		{
			name:              "literal in another function of the constructor file is reported",
			path:              captureErrorFile,
			src:               "package output\n\nfunc helper() *CaptureError { return &CaptureError{} }\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "new literal inside the package is reported",
			path:              "internal/runner/base/output/x.go",
			src:               "package output\n\nvar e = new(CaptureError)\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "qualified new literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var e = new(o.CaptureError)\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "new literal in a constructor is accepted",
			path:              captureErrorFile,
			src:               "package output\n\nfunc newSizeLimitError() *CaptureError { return new(CaptureError) }\n",
			wantLiterals:      1,
			wantLiteralMisses: 0,
		},
		{
			name:         "new of another type inside the package is not reported",
			path:         "internal/runner/base/output/x.go",
			src:          "package output\n\nvar e = new(OtherError)\n",
			wantLiterals: 0,
		},
		{
			name:         "a same-named type in another package is not reported",
			path:         "internal/x/x.go",
			src:          "package x\n\ntype CaptureError struct{}\n\nvar e = &CaptureError{}\n",
			wantLiterals: 0,
		},
		{
			name:         "a different type inside the package is not reported",
			path:         "internal/runner/base/output/x.go",
			src:          "package output\n\nvar e = &OtherError{}\n",
			wantLiterals: 0,
		},
	}

	for _, tt := range literalTests {
		t.Run(tt.name, func(t *testing.T) {
			found, literalMisses, _ := checkCaptureErrorConstruction(t, tt.path, tt.src)
			assert.Equal(t, tt.wantLiterals, found)
			assert.Len(t, literalMisses, tt.wantLiteralMisses, "literal violations: %v", literalMisses)
		})
	}

	assignmentTests := []struct {
		name string
		path string
		src  string
		want int
	}{
		{
			name: "typ assignment outside the constructors is reported",
			path: "internal/runner/base/output/x.go",
			src:  "package output\n\nfunc helper(e *CaptureError) { e.typ = 0 }\n",
			want: 1,
		},
		{
			name: "path assignment outside the constructors is reported",
			path: "internal/runner/base/output/x.go",
			src:  "package output\n\nfunc helper(e *CaptureError) { e.path = \"x\" }\n",
			want: 1,
		},
		{
			name: "phase assignment outside the constructors is reported",
			path: "internal/runner/base/output/x.go",
			src:  "package output\n\nfunc helper(e *CaptureError) { e.phase = 0 }\n",
			want: 1,
		},
		{
			name: "cause assignment outside the constructors is reported",
			path: "internal/runner/base/output/x.go",
			src:  "package output\n\nfunc helper(e *CaptureError) { e.cause = nil }\n",
			want: 1,
		},
		{
			name: "limit assignment outside the constructors is reported",
			path: "internal/runner/base/output/x.go",
			src:  "package output\n\nfunc helper(e *CaptureError) { e.limit = 1 }\n",
			want: 1,
		},
		{
			name: "limit increment outside the constructors is reported",
			path: "internal/runner/base/output/x.go",
			src:  "package output\n\nfunc helper(e *CaptureError) { e.limit++ }\n",
			want: 1,
		},
		{
			name: "assignment in a constructor is accepted",
			path: captureErrorFile,
			src:  "package output\n\nfunc newSizeLimitError(e *CaptureError) { e.limit = 1 }\n",
			want: 0,
		},
		{
			name: "unrelated field assignment in another package is not reported",
			path: "internal/x/x.go",
			src:  "package x\n\nfunc helper(r *result) { r.limit = 1 }\n",
			want: 0,
		},
	}

	for _, tt := range assignmentTests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, assignmentMisses := checkCaptureErrorConstruction(t, tt.path, tt.src)
			assert.Len(t, assignmentMisses, tt.want, "assignment violations: %v", assignmentMisses)
		})
	}
}

// TestCaptureErrorHasNoLegacyAccessors pins that GetType and GetPath, removed
// because no production code called them, are not declared on CaptureError
// again. A method can only be declared in the type's own package, so the scan
// looks for either method on a CaptureError receiver there.
func TestCaptureErrorHasNoLegacyAccessors(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var violations []string
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		violations = append(violations, findCaptureErrorAccessors(t, file, src)...)
	}

	assert.Empty(t, violations,
		"CaptureError must not re-declare the removed accessors:\n%s",
		strings.Join(violations, "\n"))
}

// findCaptureErrorAccessors returns the positions of GetType or GetPath method
// declarations on a CaptureError receiver in src.
func findCaptureErrorAccessors(t *testing.T, filename, src string) []string {
	t.Helper()

	if path.Dir(filename) != captureErrorPackageDir {
		return nil
	}
	fset, file := identitymutationguard.ParseSource(t, filename, src)
	var found []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || !isLegacyCaptureAccessor(fn.Name.Name) {
			continue
		}
		for _, field := range fn.Recv.List {
			if receiverBaseName(field.Type) == "CaptureError" {
				found = append(found, fset.Position(fn.Pos()).String())
			}
		}
	}
	return found
}

// TestCaptureErrorAccessorCheckRecognizesForms pins the forms the accessor
// finder must recognize, so it cannot become a no-op that still passes the
// repository scan.
func TestCaptureErrorAccessorCheckRecognizesForms(t *testing.T) {
	tests := []struct {
		name string
		path string
		src  string
		want int
	}{
		{
			name: "value receiver GetType is reported",
			path: captureErrorFile,
			src:  "package output\n\nfunc (e CaptureError) GetType() string { return \"\" }\n",
			want: 1,
		},
		{
			name: "pointer receiver GetPath is reported",
			path: captureErrorFile,
			src:  "package output\n\nfunc (e *CaptureError) GetPath() string { return \"\" }\n",
			want: 1,
		},
		{
			name: "an accessor on another receiver type is not reported",
			path: captureErrorFile,
			src:  "package output\n\nfunc (e Other) GetType() string { return \"\" }\n",
			want: 0,
		},
		{
			name: "a method that is not a legacy accessor is not reported",
			path: captureErrorFile,
			src:  "package output\n\nfunc (e *CaptureError) Error() string { return \"\" }\n",
			want: 0,
		},
		{
			name: "declarations outside the package are not scanned",
			path: "internal/x/x.go",
			src:  "package x\n\nfunc (e CaptureError) GetType() string { return \"\" }\n",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Len(t, findCaptureErrorAccessors(t, tt.path, tt.src), tt.want)
		})
	}
}

// isLegacyCaptureAccessor reports whether name is one of the removed
// CaptureError accessors.
func isLegacyCaptureAccessor(name string) bool {
	return name == "GetType" || name == "GetPath"
}

// receiverBaseName returns the unqualified type name a receiver expression
// names, unwrapping parentheses and a pointer.
func receiverBaseName(expr ast.Expr) string {
	expr = identitymutationguard.UnwrapParen(expr)
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = identitymutationguard.UnwrapParen(star.X)
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
