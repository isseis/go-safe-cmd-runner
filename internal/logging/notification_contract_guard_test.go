//go:build test

package logging

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"
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
		references          int
		qualifiedReferences int
		violations          []string
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		builds, refs, qualifiedRefs := checkNotificationContextBuilds(t, file, src)
		references += refs
		qualifiedReferences += qualifiedRefs
		violations = append(violations, builds...)
	}

	// If the filename convention or import resolution breaks, the guard would
	// pass vacuously: unqualified references in internal/common alone keep
	// `references` above zero, so the qualified counter is the one that proves
	// the resolver still reaches out-of-package uses.
	require.Contains(t, files, notificationContextFile,
		"the repository scan must use root-relative slash paths")
	require.NotZero(t, references,
		"the scan saw no NotificationContext reference; the file scan is broken")
	require.NotZero(t, qualifiedReferences,
		"the scan resolved no qualified NotificationContext reference; import resolution is broken")
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
	isPreExecutionError := func(expr ast.Expr) bool {
		return isNamedType(expr, qualifiers, loggingImportPath, "PreExecutionError", inLogging)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		literals := make([]*ast.CompositeLit, 0, 1)
		if isPreExecutionError(lit.Type) {
			literals = append(literals, lit)
		}
		literals = append(literals, elidedCompositeLiterals(lit, isPreExecutionError)...)
		for _, literal := range literals {
			found++
			if !setsNotificationContext(literal) {
				missing = append(missing, fset.Position(literal.Pos()).String())
			}
		}
		return true
	})
	return found, missing
}

// setsNotificationContext reports whether the literal sets the
// NotificationContext field with a keyed element.
func setsNotificationContext(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "NotificationContext" {
			return true
		}
	}
	return false
}

// checkNotificationContextBuilds parses one production file and returns the
// positions of the constructs that build a NotificationContext without a
// constructor, plus the number of references to the type anywhere in the file
// and the number of those references that were resolved through an import.
func checkNotificationContextBuilds(t *testing.T, filename, src string) (violations []string, references, qualifiedReferences int) {
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
				for _, elided := range elidedCompositeLiterals(n, isType) {
					report(elided.Pos(), "composite literal with an elided type builds a NotificationContext outside the constructors")
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
					qualifiedReferences++
				}
			}
			return true
		})
	}
	return violations, references, qualifiedReferences
}

// elidedCompositeLiterals returns the nested composite literals that leave
// their type implicit when the enclosing literal's element type matches
// isType. []T{{...}} and map[K]T{k: {...}} create a T value without ever
// spelling the type name, so a type check on the nested literal alone (whose
// Type is nil) would miss them. The same elision is allowed for a pointer
// element type: []*T{{...}} builds &T{...}, so *T is unwrapped to T.
func elidedCompositeLiterals(lit *ast.CompositeLit, isType func(ast.Expr) bool) []*ast.CompositeLit {
	element := compositeElementType(lit.Type)
	if element == nil || !isType(element) {
		return nil
	}
	var elided []*ast.CompositeLit
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}
		if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
			elided = append(elided, inner)
		}
	}
	return elided
}

// compositeElementType returns the element type of a slice, array or map type,
// unwrapping parentheses and one level of pointer: an element of type *T may
// elide &T in the literal, which still constructs a T.
func compositeElementType(expr ast.Expr) ast.Expr {
	var element ast.Expr
	switch e := unwrapParen(expr).(type) {
	case *ast.ArrayType:
		element = e.Elt
	case *ast.MapType:
		element = e.Value
	default:
		return nil
	}
	if star, ok := unwrapParen(element).(*ast.StarExpr); ok {
		return star.X
	}
	return element
}

// unwrapParen peels parenthesized type expressions.
func unwrapParen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

// isNamedType reports whether expr names the identifier name of the package at
// importPath: qualified through an import resolved in qualifiers, or
// unqualified because the file belongs to that package (inPackage).
func isNamedType(expr ast.Expr, qualifiers map[string]string, importPath, name string, inPackage bool) bool {
	switch e := unwrapParen(expr).(type) {
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
			name:     "elided slice element without the field is reported",
			body:     "var es = []l.PreExecutionError{{Type: l.ErrorTypeSystemError}}",
			want:     1,
			wantHits: 1,
		},
		{
			name:     "elided slice element with the field is accepted",
			body:     "var es = []l.PreExecutionError{{NotificationContext: commonScope()}}",
			want:     0,
			wantHits: 1,
		},
		{
			name:     "elided pointer slice element without the field is reported",
			body:     "var es = []*l.PreExecutionError{{Type: l.ErrorTypeSystemError}}",
			want:     1,
			wantHits: 1,
		},
		{
			name:     "elided pointer map value without the field is reported",
			body:     "var es = map[string]*l.PreExecutionError{\"k\": {Type: l.ErrorTypeSystemError}}",
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
			name: "parenthesized var type",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctx (c.NotificationContext)\n",
			want: 1,
		},
		{
			name: "elided slice element",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctxs = []c.NotificationContext{{}}\n",
			want: 1,
		},
		{
			name: "elided map value",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctxs = map[string]c.NotificationContext{\"k\": {}}\n",
			want: 1,
		},
		{
			name: "elided pointer slice element",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctxs = []*c.NotificationContext{{}}\n",
			want: 1,
		},
		{
			name: "elided pointer map value",
			path: "internal/x/x.go",
			src:  importedHeader + "var ctxs = map[string]*c.NotificationContext{\"k\": {}}\n",
			want: 1,
		},
		{
			name: "named result",
			path: "internal/x/x.go",
			src:  importedHeader + "func f() (ctx c.NotificationContext) { return }\n",
			want: 1,
		},
		{
			name: "named result with a parenthesized type",
			path: "internal/x/x.go",
			src:  importedHeader + "func f() (ctx (c.NotificationContext)) { return }\n",
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
			violations, _, _ := checkNotificationContextBuilds(t, tt.path, tt.src)
			assert.Len(t, violations, tt.want, "violations: %v", violations)
		})
	}
}

// --- Notification token and type-name guards ---

// notificationFile holds the registry: the only file allowed to mention a
// registered type name, the token variables, or notificationDefinitions.
const notificationFile = "internal/logging/notification.go"

// notificationRegistrySyntax is the token-variable and accessor-name set of
// notification.go. Go does not keep identifiers at run time, so the guard
// reads them from the syntax instead of from notificationDefinitions.
type notificationRegistrySyntax struct {
	tokenVars     map[string]struct{}
	accessorNames map[string]struct{}
}

// collectNotificationRegistrySyntax reads notification.go and returns the
// package-level variables initialized by registerNotification and the
// functions that return one of them. If either set is empty the declaration
// shape changed and the guard must fail rather than pass vacuously.
func collectNotificationRegistrySyntax(t *testing.T) notificationRegistrySyntax {
	t.Helper()

	src := identitymutationguard.ReadProductionSource(t, notificationFile)
	_, file := parseSource(t, notificationFile, src)

	syntax := notificationRegistrySyntax{
		tokenVars:     map[string]struct{}{},
		accessorNames: map[string]struct{}{},
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, value := range valueSpec.Values {
				if i >= len(valueSpec.Names) || !isRegisterNotificationCall(value) {
					continue
				}
				syntax.tokenVars[valueSpec.Names[i].Name] = struct{}{}
			}
		}
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || len(fn.Body.List) != 1 {
			continue
		}
		ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		ident, ok := ret.Results[0].(*ast.Ident)
		if !ok {
			continue
		}
		if _, isToken := syntax.tokenVars[ident.Name]; isToken {
			syntax.accessorNames[fn.Name.Name] = struct{}{}
		}
	}

	require.NotEmpty(t, syntax.tokenVars,
		"no registerNotification-initialized variables found in %s; the registry declaration shape changed", notificationFile)
	require.NotEmpty(t, syntax.accessorNames,
		"no accessor functions returning a token found in %s; the registry declaration shape changed", notificationFile)
	return syntax
}

// isRegisterNotificationCall reports whether expr is a call to
// registerNotification.
func isRegisterNotificationCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	return ok && ident.Name == "registerNotification"
}

// selectorOrIdentName returns the identifier a callee expression names: the
// selector name for pkg.Fn and the identifier itself for Fn.
func selectorOrIdentName(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name, true
	case *ast.SelectorExpr:
		return e.Sel.Name, true
	default:
		return "", false
	}
}

// stringLiteralValue returns the unquoted value of a string literal argument.
func stringLiteralValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// isStaticallyFalse reports whether expr is the predeclared false identifier,
// the only value the slack_notify guard accepts without further analysis.
func isStaticallyFalse(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "false"
}

// notificationAttrsFunctionRange returns the body of the NotificationAttrs
// function in notification.go, the one place a production slack_notify=true may
// be built. It returns nil when the function or file is not being scanned.
func notificationAttrsFunctionRange(filename string, file *ast.File) ast.Node {
	if filename != notificationFile {
		return nil
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "NotificationAttrs" {
			return fn.Body
		}
	}
	return nil
}

// checkNotificationAttributeConstructions reports constructions of
// slack_notify=true outside NotificationAttrs and constructions of a
// message_type string literal outside internal/logging. It returns the number
// of slack_notify constructions seen, so a scan that sees none can fail
// loudly.
func checkNotificationAttributeConstructions(t *testing.T, filename, src string) (violations []string, slackNotifyConstructions int) {
	t.Helper()

	fset, file := parseSource(t, filename, src)
	inLoggingPackage := path.Dir(filename) == loggingPackageDir
	allowed := notificationAttrsFunctionRange(filename, file)

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := selectorOrIdentName(call.Fun)
		if !ok || len(call.Args) < 2 {
			return true
		}
		key, ok := stringLiteralValue(call.Args[0])
		if !ok {
			return true
		}

		switch key {
		case slackNotifyAttrKey:
			// The known construction form is slog.Bool(slack_notify, value).
			if callee != "Bool" {
				return true
			}
			slackNotifyConstructions++
			if isStaticallyFalse(call.Args[1]) {
				return true
			}
			if allowed != nil && call.Pos() >= allowed.Pos() && call.End() <= allowed.End() {
				return true
			}
			violations = append(violations, fmt.Sprintf(
				"%s: slack_notify=true built outside NotificationAttrs", fset.Position(call.Pos())))
		case msgTypeAttrKey:
			if inLoggingPackage {
				return true
			}
			if _, isLiteral := stringLiteralValue(call.Args[1]); isLiteral {
				violations = append(violations, fmt.Sprintf(
					"%s: message_type is built from a string literal outside internal/logging", fset.Position(call.Pos())))
			}
		}
		return true
	})

	return violations, slackNotifyConstructions
}

// checkNotificationAttributeUsage reports any production use of a registered
// token other than as the first argument of NotificationAttrs: a call to the
// accessor in another position, a reference to a private token variable
// outside notification.go, or a reference to notificationDefinitions outside
// it. It returns the number of accessor calls seen.
func checkNotificationAttributeUsage(t *testing.T, filename, src string, syntax notificationRegistrySyntax) (violations []string, accessorCalls int) {
	t.Helper()

	fset, file := parseSource(t, filename, src)
	inNotificationFile := filename == notificationFile

	allowedAccessorCalls := map[ast.Node]struct{}{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := selectorOrIdentName(call.Fun)
		if !ok || name != "NotificationAttrs" {
			return true
		}
		if len(call.Args) == 0 {
			violations = append(violations, fmt.Sprintf(
				"%s: NotificationAttrs called without arguments", fset.Position(call.Pos())))
			return true
		}
		first := call.Args[0]
		accessorCall, ok := first.(*ast.CallExpr)
		if !ok {
			violations = append(violations, fmt.Sprintf(
				"%s: NotificationAttrs first argument is not a registered accessor call", fset.Position(first.Pos())))
			return true
		}
		accessorName, ok := selectorOrIdentName(accessorCall.Fun)
		if !ok {
			violations = append(violations, fmt.Sprintf(
				"%s: NotificationAttrs first argument does not name an accessor", fset.Position(accessorCall.Pos())))
			return true
		}
		if _, registered := syntax.accessorNames[accessorName]; !registered {
			violations = append(violations, fmt.Sprintf(
				"%s: NotificationAttrs first argument %s is not a registered accessor", fset.Position(accessorCall.Pos()), accessorName))
			return true
		}
		allowedAccessorCalls[accessorCall] = struct{}{}
		return true
	})

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			name, ok := selectorOrIdentName(node.Fun)
			if !ok {
				return true
			}
			if _, isAccessor := syntax.accessorNames[name]; !isAccessor {
				return true
			}
			accessorCalls++
			if _, allowed := allowedAccessorCalls[node]; !allowed {
				violations = append(violations, fmt.Sprintf(
					"%s: accessor %s called outside the NotificationAttrs first argument", fset.Position(node.Pos()), name))
			}
		case *ast.Ident:
			if inNotificationFile {
				return true
			}
			if _, isToken := syntax.tokenVars[node.Name]; isToken {
				violations = append(violations, fmt.Sprintf(
					"%s: reference to private notification token %s", fset.Position(node.Pos()), node.Name))
			}
			if node.Name == "notificationDefinitions" {
				violations = append(violations, fmt.Sprintf(
					"%s: reference to notificationDefinitions", fset.Position(node.Pos())))
			}
		}
		return true
	})

	return violations, accessorCalls
}

// checkRegisteredTypeNameLiterals reports string literals equal to a
// registered type name. The expected names are passed in from the live
// registry so the check carries no copy of them.
func checkRegisteredTypeNameLiterals(t *testing.T, filename, src string, registered map[string]struct{}) (violations []string, literals int) {
	t.Helper()

	fset, file := parseSource(t, filename, src)
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		literals++
		if _, isRegistered := registered[value]; isRegistered {
			violations = append(violations, fmt.Sprintf(
				"%s: registered type name %q appears outside the registry", fset.Position(lit.Pos()), value))
		}
		return true
	})
	return violations, literals
}

// TestProductionCodeSetsSlackNotifyOnlyInNotificationAttrs verifies no
// production file other than NotificationAttrs sets slack_notify true.
// slack_notify=false constructions are allowed: they opt a record out of
// Slack delivery and cannot trigger a notification.
func TestProductionCodeSetsSlackNotifyOnlyInNotificationAttrs(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var (
		violations    []string
		constructions int
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, count := checkNotificationAttributeConstructions(t, file, src)
		violations = append(violations, found...)
		constructions += count
	}

	require.NotZero(t, constructions, "the scan saw no slack_notify construction; the checker is broken")
	assert.Empty(t, violations, "slack_notify=true may only be built in NotificationAttrs:\n%s", strings.Join(violations, "\n"))
}

// TestProductionCodeDoesNotBuildMessageTypeLiteralsOutsideLogging verifies
// that a firing point outside internal/logging cannot name a type with a
// copied string; it has to go through a registered accessor.
func TestProductionCodeDoesNotBuildMessageTypeLiteralsOutsideLogging(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var violations []string
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, _ := checkNotificationAttributeConstructions(t, file, src)
		violations = append(violations, found...)
	}
	assert.Empty(t, violations, "message_type must not be built from a string literal outside internal/logging:\n%s", strings.Join(violations, "\n"))
}

// TestProductionCodeUsesNotificationTokensOnlyInNotificationAttrs verifies the
// route to a Notification value is one: the first argument of
// NotificationAttrs. Accessor calls elsewhere, private token variables and the
// registry itself are all rejected outside notification.go.
func TestProductionCodeUsesNotificationTokensOnlyInNotificationAttrs(t *testing.T) {
	syntax := collectNotificationRegistrySyntax(t)
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var (
		violations    []string
		accessorCalls int
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, calls := checkNotificationAttributeUsage(t, file, src, syntax)
		violations = append(violations, found...)
		accessorCalls += calls
	}

	require.NotZero(t, accessorCalls, "the scan saw no accessor call; the checker is broken")
	assert.Empty(t, violations, "Notification values may only come from NotificationAttrs's first argument:\n%s", strings.Join(violations, "\n"))
}

// TestRegisteredTypeNamesAppearOnlyInNotificationRegistry verifies no
// production file outside notification.go repeats a registered type name as a
// string literal. The expected names come from the live registry.
func TestRegisteredTypeNamesAppearOnlyInNotificationRegistry(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	registered := make(map[string]struct{}, len(notificationDefinitions))
	require.NotEmpty(t, notificationDefinitions)
	for _, definition := range notificationDefinitions {
		registered[definition.messageType] = struct{}{}
	}

	var (
		violations []string
		literals   int
	)
	for _, file := range files {
		if file == notificationFile {
			continue
		}
		src := identitymutationguard.ReadProductionSource(t, file)
		found, count := checkRegisteredTypeNameLiterals(t, file, src, registered)
		violations = append(violations, found...)
		literals += count
	}

	require.NotZero(t, literals, "the scan saw no string literal; the checker is broken")
	assert.Empty(t, violations, "registered type names may only appear in the registry:\n%s", strings.Join(violations, "\n"))
}

// TestSlackNotifyConstructionCheckRecognizesForms pins the constructions the
// guard must reject and the ones it must leave alone.
func TestSlackNotifyConstructionCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport \"log/slog\"\n\n"

	tests := []struct {
		name string
		path string
		src  string
		want int
	}{
		{
			name: "true outside the notification file is rejected",
			path: "internal/x/x.go",
			src:  header + "var _ = slog.Bool(\"slack_notify\", true)\n",
			want: 1,
		},
		{
			name: "false outside the notification file is accepted",
			path: "internal/x/x.go",
			src:  header + "var _ = slog.Bool(\"slack_notify\", false)\n",
			want: 0,
		},
		{
			name: "a variable value is treated as true",
			path: "internal/x/x.go",
			src:  header + "func f(enabled bool) { _ = slog.Bool(\"slack_notify\", enabled) }\n",
			want: 1,
		},
		{
			name: "true inside NotificationAttrs is accepted",
			path: notificationFile,
			src:  header + "func NotificationAttrs() { _ = slog.Bool(\"slack_notify\", true) }\n",
			want: 0,
		},
		{
			name: "true in another function of the notification file is rejected",
			path: notificationFile,
			src:  header + "func helper() { _ = slog.Bool(\"slack_notify\", true) }\n",
			want: 1,
		},
		{
			name: "a message_type literal outside internal/logging is rejected",
			path: "internal/x/x.go",
			src:  header + "var _ = slog.String(\"message_type\", \"command_group_summary\")\n",
			want: 1,
		},
		{
			name: "a message_type literal inside internal/logging is accepted",
			path: "internal/logging/x.go",
			src:  header + "var _ = slog.String(\"message_type\", \"execution_error\")\n",
			want: 0,
		},
		{
			name: "a message_type variable outside internal/logging is accepted",
			path: "internal/x/x.go",
			src:  header + "var _ = slog.String(\"message_type\", dynamicType)\n",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, _ := checkNotificationAttributeConstructions(t, tt.path, tt.src)
			assert.Len(t, violations, tt.want, "violations: %v", violations)
		})
	}
}

// TestNotificationAttributeUsageCheckRecognizesForms pins the accessor uses
// the guard must reject and the ones it must leave alone.
func TestNotificationAttributeUsageCheckRecognizesForms(t *testing.T) {
	syntax := notificationRegistrySyntax{
		tokenVars:     map[string]struct{}{"summaryToken": {}},
		accessorNames: map[string]struct{}{"SummaryNotification": {}},
	}
	const header = "package x\n\n"

	tests := []struct {
		name string
		path string
		src  string
		want int
	}{
		{
			name: "accessor as the first argument is accepted",
			path: "internal/x/x.go",
			src:  header + "var a = NotificationAttrs(SummaryNotification(), ctx)\n",
			want: 0,
		},
		{
			name: "another call as the first argument is rejected",
			path: "internal/x/x.go",
			src:  header + "var a = NotificationAttrs(other(), ctx)\n",
			want: 1,
		},
		{
			name: "accessor called outside the argument position is rejected",
			path: "internal/x/x.go",
			src:  header + "var token = SummaryNotification()\n\nvar a = NotificationAttrs(token, ctx)\n",
			want: 2, // the call and the non-call first argument
		},
		{
			name: "token variable outside the notification file is rejected",
			path: "internal/x/x.go",
			src:  header + "var token = summaryToken\n",
			want: 1,
		},
		{
			name: "registry reference outside the notification file is rejected",
			path: "internal/x/x.go",
			src:  header + "func f() int { return len(notificationDefinitions) }\n",
			want: 1,
		},
		{
			name: "token and registry inside the notification file are accepted",
			path: notificationFile,
			src:  header + "var token = summaryToken\n\nvar _ = notificationDefinitions\n",
			want: 0,
		},
		{
			name: "accessor called outside the argument position in the notification file is rejected",
			path: notificationFile,
			src:  header + "var token = SummaryNotification()\n",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, _ := checkNotificationAttributeUsage(t, tt.path, tt.src, syntax)
			assert.Len(t, violations, tt.want, "violations: %v", violations)
		})
	}
}

// TestRegisteredTypeNameLiteralCheckRecognizesForms pins the type-name literal
// check: only an exact registered name is rejected.
func TestRegisteredTypeNameLiteralCheckRecognizesForms(t *testing.T) {
	registered := map[string]struct{}{"command_group_summary": {}}

	src := "package x\n\nvar a = \"command_group_summary\"\nvar b = \"command_group_summary_extra\"\n"
	violations, literals := checkRegisteredTypeNameLiterals(t, "internal/x/x.go", src, registered)
	assert.Len(t, violations, 1, "violations: %v", violations)
	assert.Equal(t, 2, literals)
}
