//go:build test

package logging

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// commonImportPath is the package whose NotificationContext values may only
	// be built through its constructors.
	commonImportPath = "github.com/isseis/go-safe-cmd-runner/internal/common"
	// loggingImportPath is the package that declares PreExecutionError.
	loggingImportPath = "github.com/isseis/go-safe-cmd-runner/internal/logging"
	// notificationContextFile holds the NotificationContext type and its
	// constructors, so it is the one file with exempt functions.
	notificationContextFile = "internal/common/notification_context.go"
	// loggingPackageDir holds the PreExecutionError type, whose literals may
	// name it without a qualifier.
	loggingPackageDir = "internal/logging"
)

// TestProductionPreExecutionErrorLiteralsCarryNotificationContext verifies that
// every PreExecutionError literal in production code sets NotificationContext.
// A literal that omits it still compiles, because the zero value is a valid
// global scope, so without this guard the omission is invisible until a
// group-scoped report silently loses its scope.
func TestProductionPreExecutionErrorLiteralsCarryNotificationContext(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var (
		literals   int
		violations []string
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, missing := checkPreExecutionErrorLiterals(t, file, src)
		literals += found
		violations = append(violations, missing...)
	}

	// The scan is the point of this test, so a scan that finds nothing must
	// fail loudly rather than pass as if every literal were compliant.
	require.NotZero(t, literals,
		"the scan found no production PreExecutionError literals; the import or file scan is broken")
	assert.Empty(t, violations,
		"every production PreExecutionError literal must set NotificationContext explicitly; missing in:\n%s",
		strings.Join(violations, "\n"))
}

// TestNotificationContextBuiltOnlyByConstructors verifies that production code
// cannot create a NotificationContext without going through GlobalScope,
// GroupScope or CommandScope. The fields are unexported, so outside the package
// only a zero composite literal is expressible; inside the package the same
// rule is what keeps a future helper from bypassing the constructors. Values
// that are only read (struct fields, parameters, constructor results) are not
// constructions and are not reported.
//
// The check is deliberately incomplete: Go offers more ways to obtain a zero
// value than an AST scan can enumerate, so this guards against bypassing the
// constructors by accident rather than against every possible intent. That the
// firing points build the scope they mean is verified at run time instead, by
// the scope assertions of the firing-point tests.
func TestNotificationContextBuiltOnlyByConstructors(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var (
		references int
		violations []string
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		builds, refs := checkNotificationContextBuilds(t, file, src)
		references += refs
		violations = append(violations, builds...)
	}

	// If import resolution or the filename convention breaks, every file
	// reports nothing and the guard passes vacuously. The type is named by its
	// own declaration, so a healthy scan always sees at least one reference.
	require.NotZero(t, references,
		"the scan saw no NotificationContext reference; the import or file scan is broken")
	assert.Empty(t, violations,
		"NotificationContext values may only be built by GlobalScope, GroupScope and CommandScope:\n%s",
		strings.Join(violations, "\n"))
}

// notificationContextExemptFunctions are the type's own machinery: the three
// constructors and the decoder, which rebuilds a value from the encoded record
// and validates it. They are exempt by function, not by file, so a helper
// added to the same file is still checked.
var notificationContextExemptFunctions = map[string]struct{}{
	"GlobalScope":               {},
	"GroupScope":                {},
	"CommandScope":              {},
	"DecodeNotificationContext": {},
}

// checkPreExecutionErrorLiterals returns the number of PreExecutionError
// composite literals in src together with the positions of those that do not
// set the NotificationContext field.
func checkPreExecutionErrorLiterals(t *testing.T, filename, src string) (found int, missing []string) {
	t.Helper()

	fset, file := parseSource(t, filename, src)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == loggingImportPath
	})
	inLogging := path.Dir(filename) == loggingPackageDir

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isNamedType(lit.Type, qualifiers, loggingImportPath, "PreExecutionError", inLogging) {
			return true
		}
		found++
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "NotificationContext" {
				return true
			}
		}
		missing = append(missing, fset.Position(lit.Pos()).String())
		return true
	})
	return found, missing
}

// checkNotificationContextBuilds parses one production file and returns the
// positions of the constructs that build a NotificationContext without a
// constructor, plus the number of references to the type anywhere in the file.
func checkNotificationContextBuilds(t *testing.T, filename, src string) (violations []string, references int) {
	t.Helper()

	fset, file := parseSource(t, filename, src)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == commonImportPath
	})
	inCommon := path.Dir(filename) == "internal/common"
	isType := func(expr ast.Expr) bool {
		return isNamedType(expr, qualifiers, commonImportPath, "NotificationContext", inCommon)
	}
	exemptFile := filename == notificationContextFile

	report := func(pos token.Pos, form string) {
		violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(pos), form))
	}

	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && exemptFile {
			if _, exempt := notificationContextExemptFunctions[fn.Name.Name]; exempt {
				continue
			}
		}

		ast.Inspect(decl, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				if isType(n.Type) {
					report(n.Pos(), "composite literal builds a NotificationContext outside the constructors")
				}
			case *ast.ValueSpec:
				if n.Type != nil && len(n.Values) == 0 && isType(n.Type) {
					report(n.Pos(), "var declaration leaves a NotificationContext at its zero value")
				}
			case *ast.CallExpr:
				if ident, ok := n.Fun.(*ast.Ident); ok && ident.Name == "new" && len(n.Args) == 1 && isType(n.Args[0]) {
					report(n.Pos(), "new builds a zero-valued NotificationContext")
				}
			case *ast.FuncType:
				if n.Results == nil {
					break
				}
				for _, field := range n.Results.List {
					if len(field.Names) == 0 || !isType(field.Type) {
						continue
					}
					for _, name := range field.Names {
						report(name.Pos(), "named result "+name.Name+" starts at the type's zero value")
					}
				}
			case *ast.Ident:
				if isType(n) {
					references++
				}
			case *ast.SelectorExpr:
				if isType(n) {
					references++
				}
			}
			return true
		})
	}
	return violations, references
}

// isNamedType reports whether expr names the identifier name of the package at
// importPath: qualified through an import resolved in qualifiers, or
// unqualified because the file belongs to that package (inPackage).
func isNamedType(expr ast.Expr, qualifiers map[string]string, importPath, name string, inPackage bool) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return inPackage && e.Name == name
	case *ast.SelectorExpr:
		pkgIdent, ok := e.X.(*ast.Ident)
		if !ok {
			return false
		}
		return e.Sel.Name == name && qualifiers[pkgIdent.Name] == importPath
	default:
		return false
	}
}

// parseSource parses one file's source for the guard checks.
func parseSource(t *testing.T, filename, src string) (*token.FileSet, *ast.File) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	require.NoErrorf(t, err, "failed to parse %s", filename)
	return fset, file
}

// TestPreExecutionErrorLiteralCheckRecognizesForms pins the syntax the guard
// must recognize, including the aliased import that defeats a plain string
// search for the qualified type name.
func TestPreExecutionErrorLiteralCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport l \"github.com/isseis/go-safe-cmd-runner/internal/logging\"\n\nvar _ = l.ErrorTypeSystemError\n\n"

	tests := []struct {
		name     string
		body     string
		want     int // expected literals missing NotificationContext
		wantHits int // expected PreExecutionError literals found
	}{
		{
			name:     "literal with the field is accepted",
			body:     "var e = &l.PreExecutionError{Type: l.ErrorTypeSystemError, NotificationContext: commonScope()}",
			want:     0,
			wantHits: 1,
		},
		{
			name:     "literal without the field is reported",
			body:     "var e = &l.PreExecutionError{Type: l.ErrorTypeSystemError}",
			want:     1,
			wantHits: 1,
		},
		{
			name: "a same-named type in another package is not reported",
			body: "type PreExecutionError struct{ Type l.ErrorType }\n\nvar e = &PreExecutionError{}",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, missing := checkPreExecutionErrorLiterals(t, "internal/x/x.go", header+tt.body+"\n")
			assert.Equal(t, tt.wantHits, found)
			assert.Len(t, missing, tt.want)
		})
	}

	t.Run("unqualified literal in the declaring package is checked", func(t *testing.T) {
		src := "package logging\n\nvar e = &PreExecutionError{}\n"
		found, missing := checkPreExecutionErrorLiterals(t, "internal/logging/x.go", src)
		assert.Equal(t, 1, found)
		assert.Len(t, missing, 1)
	})
}

// TestNotificationContextBuildCheckRecognizesForms pins each construction form
// the guard must reject and each legitimate form it must leave alone. The
// excluded functions are part of the type's own implementation; they are
// matched by function name, so a helper added to the same file is still
// reported.
func TestNotificationContextBuildCheckRecognizesForms(t *testing.T) {
	const importedHeader = "package x\n\nimport c \"github.com/isseis/go-safe-cmd-runner/internal/common\"\n\nvar _ = c.GlobalScope()\n\n"

	tests := []struct {
		name string
		path string
		src  string
		want int
	}{
		{
			name: "composite literal",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctx = c.NotificationContext{}\n",
			want: 1,
		},
		{
			name: "uninitialized var declaration",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctx c.NotificationContext\n",
			want: 1,
		},
		{
			name: "new call",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctx = new(c.NotificationContext)\n",
			want: 1,
		},
		{
			name: "named result",
			path: "internal/x/x.go",
			src:  importedHeader + "func f() (ctx c.NotificationContext) { return }\n",
			want: 1,
		},
		{
			name: "named result of a closure",
			path: "internal/x/x.go",
			src:  importedHeader + "var f = func() (ctx c.NotificationContext) { return }\n",
			want: 1,
		},
		{
			name: "unqualified creation inside the declaring package",
			path: "internal/common/x.go",
			src:  "package common\n\nvar ctx = NotificationContext{}\n",
			want: 1,
		},
		{
			name: "unqualified uninitialized declaration inside the declaring package",
			path: "internal/common/x.go",
			src:  "package common\n\nvar ctx NotificationContext\n",
			want: 1,
		},
		{
			name: "constructor calls are accepted",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctx = c.GroupScope(\"backup\")\n",
			want: 0,
		},
		{
			name: "reading a parameter and returning it is accepted",
			path: "internal/x/x.go",
			src:  importedHeader + "func f(m c.NotificationContext) c.NotificationContext { return m }\n",
			want: 0,
		},
		{
			name: "field and parameter types are accepted",
			path: "internal/x/x.go",
			src:  importedHeader + "type s struct{ ctx c.NotificationContext }\n\nfunc f(m c.NotificationContext) {}\n",
			want: 0,
		},
		{
			name: "the decoder is exempt",
			path: notificationContextFile,
			src:  "package common\n\nfunc DecodeNotificationContext() (NotificationContext, error) { return NotificationContext{}, nil }\n",
			want: 0,
		},
		{
			name: "a helper beside the decoder is not exempt",
			path: notificationContextFile,
			src:  "package common\n\nfunc helper() NotificationContext { return NotificationContext{} }\n",
			want: 1,
		},
		{
			name: "a same-named type in another package is not reported",
			path: "internal/x/x.go",
			src:  "package x\n\ntype NotificationContext struct{}\n\nvar ctx = NotificationContext{}\n",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, _ := checkNotificationContextBuilds(t, tt.path, tt.src)
			assert.Len(t, violations, tt.want, "violations: %v", violations)
		})
	}
}
