package runner

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
	// groupErrorsFile is the file that declares GroupError, GroupErrors and
	// their constructors.
	groupErrorsFile = "internal/runner/group_errors.go"
	// groupErrorsConstructors names the functions allowed to build a GroupError
	// or GroupErrors, for the violation messages.
	groupErrorsConstructors = "newGroupError or newGroupErrors"
)

// groupErrorsConstructorNames are the functions allowed to build a GroupError
// or GroupErrors.
var groupErrorsConstructorNames = map[string]struct{}{
	"newGroupError":  {},
	"newGroupErrors": {},
}

// groupErrorsFields are the private fields that only the constructors may set.
// errs belongs to GroupErrors; group, command and err belong to GroupError.
var groupErrorsFields = []string{"errs", "group", "command", "err"}

// TestProductionGroupErrorLiteralsUseConstructors fixes the construction forms
// of GroupError and GroupErrors and the field mutation that could bypass them.
// The fields are unexported, so a same-package production file could still
// build either struct or assign its fields directly; this go/ast guard is what
// keeps construction in the two constructors.
//
//   - A composite literal naming GroupError or GroupErrors -- the qualified
//     runner.<Type>{...} form anywhere, and the unqualified form inside
//     internal/runner -- must appear only inside newGroupError or
//     newGroupErrors.
//   - Inside internal/runner, a selector assignment or increment of .errs,
//     .group, .command or .err is reported outside those two constructors. The
//     scan is limited to this package because the unqualified type name is
//     unambiguous there; scanning wider would match unrelated selector
//     assignments such as the base executor's pc.stage and *w.err.
//   - The field-name match carries no type information, so a future struct
//     added directly under internal/runner with a field of the same name would
//     be a false positive. That is the known maintenance obligation of this
//     incomplete (by design) scan.
//   - Test files are not scanned: they legitimately build values through the
//     test constructors.
//   - A scan that finds no literal fails rather than passing vacuously.
func TestProductionGroupErrorLiteralsUseConstructors(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var (
		literals             int
		literalViolations    []string
		assignmentViolations []string
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, lv, av := checkGroupErrorsConstruction(t, file, src)
		literals += found
		literalViolations = append(literalViolations, lv...)
		assignmentViolations = append(assignmentViolations, av...)
	}

	require.NotZero(t, literals,
		"the scan found no group error literal; the scan is broken")
	assert.Empty(t, literalViolations,
		"group error literals may only be built in %s:\n%s",
		groupErrorsConstructors, strings.Join(literalViolations, "\n"))
	assert.Empty(t, assignmentViolations,
		"group error fields may only be set in %s:\n%s",
		groupErrorsConstructors, strings.Join(assignmentViolations, "\n"))
}

// checkGroupErrorsConstruction scans one production file and returns the
// number of GroupError and GroupErrors literals found (value, pointer and
// elided forms) together with the positions of those built outside the two
// constructors and of any assignment or increment of one of their private
// fields outside them.
func checkGroupErrorsConstruction(t *testing.T, filename, src string) (found int, literalViolations, assignmentViolations []string) {
	t.Helper()

	fset, file := identitymutationguard.ParseSource(t, filename, src)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == runnerImportPath
	})
	inPackage := path.Dir(filename) == runnerPackageDir
	typeChecks := []func(ast.Expr) bool{
		func(expr ast.Expr) bool {
			return identitymutationguard.IsNamedType(expr, qualifiers, runnerImportPath, "GroupError", inPackage)
		},
		func(expr ast.Expr) bool {
			return identitymutationguard.IsNamedType(expr, qualifiers, runnerImportPath, "GroupErrors", inPackage)
		},
	}

	for _, decl := range file.Decls {
		funcName := ""
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcName = fn.Name.Name
		}
		_, isConstructor := groupErrorsConstructorNames[funcName]
		allowed := filename == groupErrorsFile && isConstructor

		ast.Inspect(decl, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				for _, isType := range typeChecks {
					literals := make([]*ast.CompositeLit, 0, 1)
					if isType(node.Type) {
						literals = append(literals, node)
					}
					literals = append(literals, identitymutationguard.ElidedCompositeLiterals(node, isType)...)
					for _, literal := range literals {
						found++
						if !allowed {
							literalViolations = append(literalViolations, fmt.Sprintf(
								"%s: group error literal built outside %s", fset.Position(literal.Pos()), groupErrorsConstructors))
						}
					}
				}
			case *ast.AssignStmt:
				if !inPackage || allowed {
					return true
				}
				for _, lhs := range node.Lhs {
					reportGroupErrorFieldMutation(fset, lhs, "assigned", &assignmentViolations)
				}
			case *ast.IncDecStmt:
				if !inPackage || allowed {
					return true
				}
				reportGroupErrorFieldMutation(fset, node.X, "modified", &assignmentViolations)
			}
			return true
		})
	}
	return found, literalViolations, assignmentViolations
}

// reportGroupErrorFieldMutation appends a violation when expr is a selector for
// one of the group error types' private fields.
func reportGroupErrorFieldMutation(fset *token.FileSet, expr ast.Expr, verb string, violations *[]string) {
	sel, ok := identitymutationguard.UnwrapParen(expr).(*ast.SelectorExpr)
	if !ok || !slices.Contains(groupErrorsFields, sel.Sel.Name) {
		return
	}
	*violations = append(*violations, fmt.Sprintf(
		"%s: group error field .%s %s outside %s", fset.Position(expr.Pos()), sel.Sel.Name, verb, groupErrorsConstructors))
}

// TestGroupErrorConstructionCheckRecognizesForms pins the forms the guard must
// recognize, so the guard cannot become a no-op that still passes the
// repository scan.
func TestGroupErrorConstructionCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport r \"github.com/isseis/go-safe-cmd-runner/internal/runner\"\n\n"

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
			src:               header + "var e = &r.GroupError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "value literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var e = r.GroupError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "elided pointer literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var es = []*r.GroupError{{}}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "qualified GroupErrors literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var e = &r.GroupErrors{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "unqualified literal inside the package is reported",
			path:              "internal/runner/x.go",
			src:               "package runner\n\nvar e = &GroupError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "literal in a constructor is accepted",
			path:              groupErrorsFile,
			src:               "package runner\n\nfunc newGroupError() *GroupError { return &GroupError{} }\n",
			wantLiterals:      1,
			wantLiteralMisses: 0,
		},
		{
			name:              "GroupErrors literal in a constructor is accepted",
			path:              groupErrorsFile,
			src:               "package runner\n\nfunc newGroupErrors() *GroupErrors { return &GroupErrors{} }\n",
			wantLiterals:      1,
			wantLiteralMisses: 0,
		},
		{
			name:              "literal in another function of the constructor file is reported",
			path:              groupErrorsFile,
			src:               "package runner\n\nfunc helper() *GroupError { return &GroupError{} }\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:         "a same-named type in another package is not reported",
			path:         "internal/x/x.go",
			src:          "package x\n\ntype GroupError struct{}\n\nvar e = &GroupError{}\n",
			wantLiterals: 0,
		},
		{
			name:         "a different type inside the package is not reported",
			path:         "internal/runner/x.go",
			src:          "package runner\n\nvar e = &OtherError{}\n",
			wantLiterals: 0,
		},
	}

	for _, tt := range literalTests {
		t.Run(tt.name, func(t *testing.T) {
			found, literalMisses, _ := checkGroupErrorsConstruction(t, tt.path, tt.src)
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
			name: "group assignment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupError) { e.group = \"x\" }\n",
			want: 1,
		},
		{
			name: "command assignment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupError) { e.command = \"x\" }\n",
			want: 1,
		},
		{
			name: "err assignment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupError) { e.err = nil }\n",
			want: 1,
		},
		{
			name: "errs assignment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupErrors) { e.errs = nil }\n",
			want: 1,
		},
		{
			name: "errs increment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupErrors) { e.errs++ }\n",
			want: 1,
		},
		{
			name: "assignment in a constructor is accepted",
			path: groupErrorsFile,
			src:  "package runner\n\nfunc newGroupError(e *GroupError) { e.err = nil }\n",
			want: 0,
		},
		{
			name: "unrelated field assignment in another package is not reported",
			path: "internal/x/x.go",
			src:  "package x\n\nfunc helper(r *result) { r.err = nil }\n",
			want: 0,
		},
	}

	for _, tt := range assignmentTests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, assignmentMisses := checkGroupErrorsConstruction(t, tt.path, tt.src)
			assert.Len(t, assignmentMisses, tt.want, "assignment violations: %v", assignmentMisses)
		})
	}
}
