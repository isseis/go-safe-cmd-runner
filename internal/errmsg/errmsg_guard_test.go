//go:build test

package errmsg

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
	// modulePath is the module prefix of every in-repository import path.
	modulePath = "github.com/isseis/go-safe-cmd-runner"
	// errmsgDir is this package's directory relative to the repository root.
	errmsgDir = "internal/errmsg"
	// errmsgImportPath is this package's import path.
	errmsgImportPath = modulePath + "/" + errmsgDir
)

// wholeFile marks a position table entry that covers every function of a
// file, and code outside any function.
const wholeFile = "*"

// positions maps a repository-relative file path to the keys (see funcKey) of
// the functions it covers, or to wholeFile.
type positions map[string][]string

// covers reports whether the function keyed fn in file is covered.
func (p positions) covers(file, fn string) bool {
	keys := p[file]
	return slices.Contains(keys, wholeFile) || (fn != "" && slices.Contains(keys, fn))
}

// exemptRolePositions are the only positions allowed to declare an
// Identifier or Path segment through errmsg.Ident or errmsg.Path. A position
// is a file and a function in it, never a function name alone.
var exemptRolePositions = positions{
	"internal/runner/group_executor.go":          {wholeFile},
	"internal/runner/group_stage.go":             {wholeFile},
	"internal/runner/group_errors.go":            {wholeFile},
	"internal/runner/config/expansion.go":        {wholeFile},
	"internal/runner/config/errors.go":           {"ErrUndefinedVariableDetail.StructuredMessage", "Level.parts", "Field.parts"},
	"internal/logging/pre_execution_error.go":    {"PreExecutionError.DetailMessage"},
	"internal/logging/execution_error.go":        {"ExecutionError.ReportMessage", "ExecutionError.contextParts"},
	"internal/runner/base/privilege/errors.go":   {"Error.StructuredMessage"},
	"internal/runner/base/executor/executor.go":  {"DefaultExecutor.Validate", "DefaultExecutor.validatePrivilegedCommand", "DefaultExecutor.executeNormal", "DefaultExecutor.executeWithUserGroup"},
	"internal/runner/resource/normal_manager.go": {"NormalResourceManager.ExecuteCommand"},
	"internal/runner/resource/dryrun_manager.go": {"DryRunResourceManager.evaluateCommandRisk"},
}

// pathErrorCausePositions are the only positions allowed to split a
// *fs.PathError through errmsg.PathErrorCause.
var pathErrorCausePositions = positions{
	"internal/runner/base/executor/tempdir_manager.go": {"DefaultTempDirManager.Create"},
}

// inScopePositions are the paths whose error wrapping is in scope for
// structured messages. An unexported function or method outside errmsg whose
// result contains an errmsg.Part must be one of them.
var inScopePositions = positions{
	"internal/runner/group_executor.go":                  {wholeFile},
	"internal/runner/group_stage.go":                     {wholeFile},
	"internal/runner/group_errors.go":                    {wholeFile},
	"internal/runner/runner.go":                          {"Runner.Execute", "Runner.ExecuteGroup", "Runner.executeGroups"},
	"internal/runner/config/errors.go":                   {"ErrUndefinedVariableDetail.StructuredMessage", "Level.parts", "Field.parts"},
	"internal/runner/resource/normal_manager.go":         {"NormalResourceManager.ExecuteCommand", "NormalResourceManager.executeCommandWithOutput"},
	"internal/runner/resource/dryrun_manager.go":         {"DryRunResourceManager.ExecuteCommand", "DryRunResourceManager.evaluateCommandRisk"},
	"internal/runner/base/executor/tempdir_manager.go":   {"DefaultTempDirManager.Create"},
	"internal/runner/base/executor/executor.go":          {"DefaultExecutor.Validate", "DefaultExecutor.validatePrivilegedCommand", "DefaultExecutor.executeNormal", "DefaultExecutor.executeWithUserGroup"},
	"internal/runner/base/executor/command_lifecycle.go": {"DefaultExecutor.runCommand", "DefaultExecutor.reportStartFailure", "DefaultExecutor.superviseCommand", "DefaultExecutor.killChild", "killOutcome"},
	"internal/runner/base/privilege/errors.go":           {"Error.StructuredMessage"},
	"internal/runner/base/privilege/unix.go":             {"UnixPrivilegeManager.performElevation"},
	"internal/logging/pre_execution_error.go":            {"PreExecutionError.DetailMessage"},
	"internal/logging/execution_error.go":                {"ExecutionError.ReportMessage", "ExecutionError.contextParts"},
}

// inScopeExpansionFile is covered as a whole file except for the functions in
// inScopeExpansionExclusions.
const inScopeExpansionFile = "internal/runner/config/expansion.go"

// inScopeExpansionExclusions are the functions of inScopeExpansionFile that
// build errors outside the scope of structured messages.
var inScopeExpansionExclusions = []string{
	"ProcessEnvImport", "ProcessEnv", "resolveAndPrepareCommandSpec", "ApplyTemplateInheritance", "expandTemplateToSpec",
}

// inScope reports whether the function keyed fn in file is in scope.
func inScope(file, fn string) bool {
	if file == inScopeExpansionFile {
		return !slices.Contains(inScopeExpansionExclusions, fn)
	}
	return inScopePositions.covers(file, fn)
}

// guardFile is one Go source file given to a guard check.
type guardFile struct {
	path string // slash-separated, relative to the repository root
	src  string
}

// parsedFile is a parsed guardFile with its imports resolved.
type parsedFile struct {
	path    string
	dir     string
	fset    *token.FileSet
	file    *ast.File
	imports map[string]string // local package name -> import path
	// unqualified reports whether errmsg's exported names are reachable
	// without a qualifier: in errmsg itself, or through a dot-import.
	unqualified bool
}

// productionGuardFiles returns every production Go file of the repository.
func productionGuardFiles(t *testing.T) []guardFile {
	t.Helper()
	paths := identitymutationguard.ProductionGoFilesInRepo(t)
	files := make([]guardFile, 0, len(paths))
	for _, p := range paths {
		files = append(files, guardFile{path: p, src: identitymutationguard.ReadProductionSource(t, p)})
	}
	return files
}

// parseGuardFiles parses files. A dot-import of errmsg is resolved rather than
// rejected: the file's unqualified names are then matched as errmsg's.
func parseGuardFiles(t *testing.T, files []guardFile) []*parsedFile {
	t.Helper()
	noDotImportRejected := func(string) bool { return false }
	parsed := make([]*parsedFile, 0, len(files))
	for _, f := range files {
		fset, file := identitymutationguard.ParseSource(t, f.path, f.src)
		dir := path.Dir(f.path)
		pf := &parsedFile{
			path:        f.path,
			dir:         dir,
			fset:        fset,
			file:        file,
			imports:     identitymutationguard.ResolveLocalImports(t, f.path, file, noDotImportRejected),
			unqualified: dir == errmsgDir,
		}
		for _, imp := range file.Imports {
			if imp.Name != nil && imp.Name.Name == "." && strings.Trim(imp.Path.Value, `"`) == errmsgImportPath {
				pf.unqualified = true
			}
		}
		parsed = append(parsed, pf)
	}
	return parsed
}

// dirOfImportPath returns the repository-relative directory of an
// in-repository import path.
func dirOfImportPath(importPath string) (string, bool) {
	return strings.CutPrefix(importPath, modulePath+"/")
}

func (pf *parsedFile) position(node ast.Node) string {
	pos := pf.fset.Position(node.Pos())
	return fmt.Sprintf("%s:%d", pf.path, pos.Line)
}

// isErrmsgName reports whether expr names errmsg's exported identifier name.
func (pf *parsedFile) isErrmsgName(expr ast.Expr, name string) bool {
	return identitymutationguard.IsNamedType(expr, pf.imports, errmsgImportPath, name, pf.unqualified)
}

// funcKey returns "Type.Method" for a method (the receiver's pointer and type
// parameters dropped) and the name alone for a function.
func funcKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverTypeName(fn) + "." + fn.Name.Name
}

// receiverTypeName returns the receiver's type name, or "" for a function.
func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := identitymutationguard.UnwrapParen(fn.Recv.List[0].Type)
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = identitymutationguard.UnwrapParen(star.X)
	}
	switch e := expr.(type) {
	case *ast.IndexExpr:
		expr = e.X
	case *ast.IndexListExpr:
		expr = e.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// errmsgRef is one reference to an errmsg function.
type errmsgRef struct {
	name string
	node ast.Node
	call *ast.CallExpr // nil when the function is referenced as a value
	fn   *ast.FuncDecl // enclosing function, nil at package level
}

// fnKey returns the enclosing function's key, or "" at package level.
func (r errmsgRef) fnKey() string {
	if r.fn == nil {
		return ""
	}
	return funcKey(r.fn)
}

// findErrmsgRefs returns every reference in pf to one of errmsg's functions
// in names. A qualified reference is resolved through the file's imports, so
// an aliased import is followed; unqualified names are matched only where
// they resolve to errmsg (see parsedFile.unqualified).
func findErrmsgRefs(pf *parsedFile, names []string) []errmsgRef {
	var refs []errmsgRef
	for _, decl := range pf.file.Decls {
		fn, _ := decl.(*ast.FuncDecl)
		callees := map[ast.Expr]*ast.CallExpr{}
		// Identifiers that are not references: selector members, keys of
		// composite literal elements and the declared function's own name.
		notRefs := map[*ast.Ident]struct{}{}
		if fn != nil {
			notRefs[fn.Name] = struct{}{}
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.CallExpr:
				callees[identitymutationguard.UnwrapParen(e.Fun)] = e
			case *ast.SelectorExpr:
				notRefs[e.Sel] = struct{}{}
			case *ast.KeyValueExpr:
				if key, ok := e.Key.(*ast.Ident); ok {
					notRefs[key] = struct{}{}
				}
			}
			return true
		})
		ast.Inspect(decl, func(n ast.Node) bool {
			var name string
			switch e := n.(type) {
			case *ast.SelectorExpr:
				pkg, ok := e.X.(*ast.Ident)
				if !ok || pf.imports[pkg.Name] != errmsgImportPath {
					return true
				}
				name = e.Sel.Name
			case *ast.Ident:
				if _, skip := notRefs[e]; skip || !pf.unqualified {
					return true
				}
				name = e.Name
			default:
				return true
			}
			if slices.Contains(names, name) {
				expr, _ := n.(ast.Expr)
				refs = append(refs, errmsgRef{name: name, node: n, call: callees[expr], fn: fn})
			}
			return true
		})
	}
	return refs
}

// constFuncNames are the constructors of RoleConstant parts.
var constFuncNames = []string{"Const", "ConstSummary"}

// checkConstCalls reports every call of errmsg.Const or errmsg.ConstSummary
// whose argument is not a constant expression, and every reference to them
// that is not a call. It returns the number of calls it checked.
func checkConstCalls(files []*parsedFile) (checked int, violations []string) {
	packageConsts := map[string]map[string]struct{}{}
	for _, pf := range files {
		consts := packageConsts[pf.dir]
		if consts == nil {
			consts = map[string]struct{}{}
			packageConsts[pf.dir] = consts
		}
		for _, decl := range pf.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					consts[name.Name] = struct{}{}
				}
			}
		}
	}

	for _, pf := range files {
		for _, ref := range findErrmsgRefs(pf, constFuncNames) {
			if ref.call == nil {
				violations = append(violations, fmt.Sprintf("%s: errmsg.%s is referenced as a value; call it with a constant expression", pf.position(ref.node), ref.name))
				continue
			}
			checked++
			if len(ref.call.Args) != 1 || !isConstantStringExpr(ref.call.Args[0], ref.fn, packageConsts[pf.dir]) {
				violations = append(violations, fmt.Sprintf("%s: errmsg.%s takes a string literal, a constant of the same package, or a + of them", pf.position(ref.call), ref.name))
			}
		}
	}
	return checked, violations
}

// isConstantStringExpr reports whether expr is a string literal, a name that
// resolves to a constant (declared in fn or at package level in the same
// package), or a + of such expressions. A name the check cannot resolve to a
// constant is rejected.
func isConstantStringExpr(expr ast.Expr, fn *ast.FuncDecl, packageConsts map[string]struct{}) bool {
	switch e := identitymutationguard.UnwrapParen(expr).(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.BinaryExpr:
		return e.Op == token.ADD &&
			isConstantStringExpr(e.X, fn, packageConsts) && isConstantStringExpr(e.Y, fn, packageConsts)
	case *ast.Ident:
		switch localDeclaration(fn, e.Name) {
		case declaredConst:
			return true
		case declaredOther:
			return false
		default:
			_, ok := packageConsts[e.Name]
			return ok
		}
	default:
		return false
	}
}

// declarationKind classifies how a name is declared inside a function.
type declarationKind int

const (
	notDeclared declarationKind = iota
	declaredConst
	declaredOther
)

// localDeclaration reports how fn declares name anywhere in its receiver,
// signature or body. A name declared both as a constant and as anything else
// (in different scopes) is reported as declaredOther, since the check does
// not resolve scopes.
func localDeclaration(fn *ast.FuncDecl, name string) declarationKind {
	if fn == nil {
		return notDeclared
	}
	kind := notDeclared
	mark := func(ident ast.Expr, k declarationKind) {
		if id, ok := ident.(*ast.Ident); ok && id.Name == name && kind != declaredOther {
			kind = k
		}
	}
	ast.Inspect(fn, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.Field:
			for _, id := range e.Names {
				mark(id, declaredOther)
			}
		case *ast.AssignStmt:
			if e.Tok == token.DEFINE {
				for _, lhs := range e.Lhs {
					mark(lhs, declaredOther)
				}
			}
		case *ast.RangeStmt:
			if e.Tok == token.DEFINE {
				mark(e.Key, declaredOther)
				mark(e.Value, declaredOther)
			}
		case *ast.GenDecl:
			k := declaredOther
			if e.Tok == token.CONST {
				k = declaredConst
			}
			for _, spec := range e.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, id := range vs.Names {
						mark(id, k)
					}
				}
			}
		}
		return true
	})
	return kind
}

// TestProductionConstCallsUseConstantExpressions pins that a RoleConstant
// segment only ever holds bytes from a constant expression, so a value from
// the configuration or the environment can never escape the Text redaction by
// being passed as a constant.
func TestProductionConstCallsUseConstantExpressions(t *testing.T) {
	checked, violations := checkConstCalls(parseGuardFiles(t, productionGuardFiles(t)))
	require.Positive(t, checked, "no errmsg.Const call was found; the scan is broken")
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestConstCallCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport %s\"" + errmsgImportPath + "\"\n\n"
	src := func(alias, body string) string { return fmt.Sprintf(header, alias) + body }
	tests := []struct {
		name  string
		files []guardFile
		want  int
	}{
		{name: "string literal", files: []guardFile{{"internal/x/x.go", src("", `var _ = errmsg.Const("fixed")`)}}},
		{name: "concatenated literals", files: []guardFile{{"internal/x/x.go", src("", `var _ = errmsg.ConstSummary("a" + ("b" + "c"))`)}}},
		{
			name: "package constant declared in another file",
			files: []guardFile{
				{"internal/x/x.go", src("", "func f() { _ = errmsg.Const(prefix + \": \") }")},
				{"internal/x/consts.go", "package x\n\nconst prefix = \"failed\"\n"},
			},
		},
		{name: "local constant", files: []guardFile{{"internal/x/x.go", src("", "func f() { const c = \"x\"; _ = errmsg.Const(c) }")}}},
		{name: "aliased import is followed", files: []guardFile{{"internal/x/x.go", src("em ", "func f(s string) { _ = em.Const(s) }")}}, want: 1},
		{name: "parameter", files: []guardFile{{"internal/x/x.go", src("", "func f(s string) { _ = errmsg.Const(s) }")}}, want: 1},
		{
			name: "local variable shadowing a package constant",
			files: []guardFile{
				{"internal/x/x.go", src("", "const c = \"x\"\n\nfunc f(v string) { c := v; _ = errmsg.Const(c) }")},
			},
			want: 1,
		},
		{name: "unresolved name", files: []guardFile{{"internal/x/x.go", src("", "func f() { _ = errmsg.Const(unknown) }")}}, want: 1},
		{
			name:  "constant of another package",
			files: []guardFile{{"internal/x/x.go", src("", "import \"os\"\n\nfunc f() { _ = errmsg.Const(os.DevNull) }")}},
			want:  1,
		},
		{name: "function call", files: []guardFile{{"internal/x/x.go", src("", "func f() { _ = errmsg.Const(g()) }\n\nfunc g() string { return \"\" }")}}, want: 1},
		{name: "literal plus variable", files: []guardFile{{"internal/x/x.go", src("", "func f(s string) { _ = errmsg.ConstSummary(\"a\" + s) }")}}, want: 1},
		{name: "referenced as a value", files: []guardFile{{"internal/x/x.go", src("", "var f = errmsg.Const")}}, want: 1},
		{name: "dot-import is resolved", files: []guardFile{{"internal/x/x.go", src(". ", "func f(s string) { _ = Const(s) }")}}, want: 1},
		{
			name:  "unqualified call inside errmsg",
			files: []guardFile{{"internal/errmsg/y.go", "package errmsg\n\nfunc f(s string) Part { return Const(s) }\n"}},
			want:  1,
		},
		{
			name:  "a same-named function of another package is not tracked",
			files: []guardFile{{"internal/x/x.go", "package x\n\nfunc Const(s string) string { return s }\n\nfunc f(s string) { _ = Const(s) }\n"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, violations := checkConstCalls(parseGuardFiles(t, tt.files))
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}

// exemptRoleFuncNames are the errmsg functions whose call positions are
// checked by checkExemptRoleCalls.
var exemptRoleFuncNames = []string{"Ident", "Path", "PathErrorCause", "IndentedCause"}

// checkExemptRoleCalls reports calls of errmsg.Ident and errmsg.Path outside
// exemptRolePositions, calls of errmsg.PathErrorCause outside
// pathErrorCausePositions, calls of errmsg.IndentedCause with other than one
// argument, and references to any of them that are not calls. errmsg's own
// files are not checked: they implement the constructors.
func checkExemptRoleCalls(files []*parsedFile) []string {
	var violations []string
	for _, pf := range files {
		if pf.dir == errmsgDir {
			continue
		}
		for _, ref := range findErrmsgRefs(pf, exemptRoleFuncNames) {
			if ref.call == nil {
				violations = append(violations, fmt.Sprintf("%s: errmsg.%s is referenced as a value; only a call in an allowed position may declare a role", pf.position(ref.node), ref.name))
				continue
			}
			switch ref.name {
			case "Ident", "Path":
				if !exemptRolePositions.covers(pf.path, ref.fnKey()) {
					violations = append(violations, fmt.Sprintf("%s: errmsg.%s is called outside the positions allowed to declare an exempt role", pf.position(ref.call), ref.name))
				}
			case "PathErrorCause":
				if !pathErrorCausePositions.covers(pf.path, ref.fnKey()) {
					violations = append(violations, fmt.Sprintf("%s: errmsg.PathErrorCause is called outside the temporary directory creation", pf.position(ref.call)))
				}
			default:
				if len(ref.call.Args) != 1 {
					violations = append(violations, fmt.Sprintf("%s: errmsg.IndentedCause takes the cause alone", pf.position(ref.call)))
				}
			}
		}
	}
	return violations
}

// TestProductionExemptRoleCallsAreInAllowedPositions pins where Identifier
// and Path segments may be declared: only at the positions of
// exemptRolePositions, decided by file and function, so a new caller cannot
// exempt text from whole-value replacement without extending the table.
func TestProductionExemptRoleCallsAreInAllowedPositions(t *testing.T) {
	violations := checkExemptRoleCalls(parseGuardFiles(t, productionGuardFiles(t)))
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestExemptRoleCallCheckRecognizesForms(t *testing.T) {
	const header = "package executor\n\nimport %s\"" + errmsgImportPath + "\"\n\ntype DefaultExecutor struct{}\n\ntype DefaultTempDirManager struct{}\n\n"
	src := func(alias, body string) string { return fmt.Sprintf(header, alias) + body }
	const executorFile = "internal/runner/base/executor/executor.go"
	const tempdirFile = "internal/runner/base/executor/tempdir_manager.go"
	tests := []struct {
		name string
		file guardFile
		want int
	}{
		{name: "Ident in an allowed method", file: guardFile{executorFile, src("", `func (e *DefaultExecutor) Validate() { _ = errmsg.Ident("x") }`)}},
		{name: "Path in a whole-file position", file: guardFile{"internal/runner/group_stage.go", "package runner\n\nimport \"" + errmsgImportPath + "\"\n\nvar _ = errmsg.Path(\"/p\")\n"}},
		{name: "Ident in another method of the same file", file: guardFile{executorFile, src("", `func (e *DefaultExecutor) Other() { _ = errmsg.Ident("x") }`)}, want: 1},
		{name: "allowed method name in another file", file: guardFile{"internal/logging/slack_handler.go", src("", `func (e *DefaultExecutor) Validate() { _ = errmsg.Ident("x") }`)}, want: 1},
		{name: "allowed name as a function rather than a method", file: guardFile{executorFile, src("", `func Validate() { _ = errmsg.Path("/p") }`)}, want: 1},
		{name: "package-level call in a function-scoped file", file: guardFile{executorFile, src("", `var _ = errmsg.Path("/p")`)}, want: 1},
		{name: "aliased import", file: guardFile{"internal/logging/slack_handler.go", src("em ", `func f() { _ = em.Ident("x") }`)}, want: 1},
		{name: "dot-import", file: guardFile{"internal/logging/slack_handler.go", src(". ", `func f() { _ = Path("/p") }`)}, want: 1},
		{name: "dot-import in an allowed method", file: guardFile{executorFile, src(". ", `func (e *DefaultExecutor) executeNormal() { _ = Ident("x") }`)}},
		{name: "function value", file: guardFile{executorFile, src("", `func (e *DefaultExecutor) Validate() { f := errmsg.Ident; _ = f }`)}, want: 1},
		{name: "PathErrorCause in Create", file: guardFile{tempdirFile, src("", `func (m *DefaultTempDirManager) Create(err error) { _ = errmsg.PathErrorCause(err) }`)}},
		{name: "PathErrorCause elsewhere", file: guardFile{executorFile, src("", `func (e *DefaultExecutor) Validate(err error) { _ = errmsg.PathErrorCause(err) }`)}, want: 1},
		{name: "IndentedCause with one argument", file: guardFile{"internal/x/x.go", src("", `func f(err error) { _ = errmsg.IndentedCause(err) }`)}},
		{name: "IndentedCause with two arguments", file: guardFile{"internal/x/x.go", src("", `func f(err error) { _ = errmsg.IndentedCause(err, "  ") }`)}, want: 1},
		{name: "a same-named local function is not tracked", file: guardFile{"internal/x/x.go", "package x\n\nfunc Ident(s string) string { return s }\n\nvar _ = Ident(\"x\")\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := checkExemptRoleCalls(parseGuardFiles(t, []guardFile{tt.file}))
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}

// typeDecl is one type declaration with the file it appears in, so names in
// its definition resolve through that file's imports.
type typeDecl struct {
	spec *ast.TypeSpec
	pf   *parsedFile
}

// typeIndex maps a repository-relative package directory to its type
// declarations by name.
type typeIndex map[string]map[string]typeDecl

func newTypeIndex(files []*parsedFile) typeIndex {
	index := typeIndex{}
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts := spec.(*ast.TypeSpec)
				if index[pf.dir] == nil {
					index[pf.dir] = map[string]typeDecl{}
				}
				index[pf.dir][ts.Name.Name] = typeDecl{spec: ts, pf: pf}
			}
		}
	}
	return index
}

// lookup resolves a type name used in pf, qualified or not, to its
// declaration when the declaration is among the indexed files.
func (idx typeIndex) lookup(pf *parsedFile, expr ast.Expr) (typeDecl, bool) {
	switch e := identitymutationguard.UnwrapParen(expr).(type) {
	case *ast.Ident:
		d, ok := idx[pf.dir][e.Name]
		return d, ok
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		if !ok {
			return typeDecl{}, false
		}
		dir, ok := dirOfImportPath(pf.imports[pkg.Name])
		if !ok {
			return typeDecl{}, false
		}
		d, ok := idx[dir][e.Sel.Name]
		return d, ok
	default:
		return typeDecl{}, false
	}
}

// containsPart reports whether the type expr, used in pf, holds an
// errmsg.Part: directly, as an element, a field, a function result, a type
// argument, or through the definition of a named type.
func (idx typeIndex) containsPart(pf *parsedFile, expr ast.Expr, visiting map[*ast.TypeSpec]struct{}) bool {
	if expr == nil {
		return false
	}
	if pf.dir != errmsgDir && pf.isErrmsgName(expr, "Part") {
		return true
	}
	switch e := identitymutationguard.UnwrapParen(expr).(type) {
	case *ast.StarExpr:
		return idx.containsPart(pf, e.X, visiting)
	case *ast.ArrayType:
		return idx.containsPart(pf, e.Elt, visiting)
	case *ast.Ellipsis:
		return idx.containsPart(pf, e.Elt, visiting)
	case *ast.MapType:
		return idx.containsPart(pf, e.Key, visiting) || idx.containsPart(pf, e.Value, visiting)
	case *ast.ChanType:
		return idx.containsPart(pf, e.Value, visiting)
	case *ast.StructType:
		return idx.fieldsContainPart(pf, e.Fields, visiting)
	case *ast.FuncType:
		return idx.fieldsContainPart(pf, e.Results, visiting)
	case *ast.IndexExpr:
		return idx.containsPart(pf, e.X, visiting) || idx.containsPart(pf, e.Index, visiting)
	case *ast.IndexListExpr:
		return idx.containsPart(pf, e.X, visiting) || slices.ContainsFunc(e.Indices, func(i ast.Expr) bool {
			return idx.containsPart(pf, i, visiting)
		})
	case *ast.Ident, *ast.SelectorExpr:
		d, ok := idx.lookup(pf, e)
		if !ok || d.pf.dir == errmsgDir {
			// errmsg's own types, such as Message, are opaque outside it.
			return false
		}
		if _, seen := visiting[d.spec]; seen {
			return false
		}
		visiting[d.spec] = struct{}{}
		defer delete(visiting, d.spec)
		return idx.containsPart(d.pf, d.spec.Type, visiting)
	default:
		return false
	}
}

func (idx typeIndex) fieldsContainPart(pf *parsedFile, fields *ast.FieldList, visiting map[*ast.TypeSpec]struct{}) bool {
	if fields == nil {
		return false
	}
	return slices.ContainsFunc(fields.List, func(f *ast.Field) bool {
		return idx.containsPart(pf, f.Type, visiting)
	})
}

// checkPartConstruction reports, outside errmsg, every composite literal that
// builds an errmsg.Part (including the elided []errmsg.Part{{...}} form), every
// exported function or method whose result contains an errmsg.Part, and every
// unexported one outside the in-scope positions. It also reports any exported
// or embedded field of errmsg's Part struct, and returns whether that struct
// was found.
func checkPartConstruction(files []*parsedFile) (partFound bool, violations []string) {
	idx := newTypeIndex(files)
	if d, ok := idx[errmsgDir]["Part"]; ok {
		if st, ok := d.spec.Type.(*ast.StructType); ok {
			partFound = true
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					violations = append(violations, fmt.Sprintf("%s: errmsg.Part must not embed a field", d.pf.position(field)))
				}
				for _, name := range field.Names {
					if name.IsExported() {
						violations = append(violations, fmt.Sprintf("%s: errmsg.Part field %s must be unexported", d.pf.position(name), name.Name))
					}
				}
			}
		}
	}

	for _, pf := range files {
		if pf.dir == errmsgDir {
			continue
		}
		isPart := func(expr ast.Expr) bool { return pf.isErrmsgName(expr, "Part") }
		ast.Inspect(pf.file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if lit.Type != nil && isPart(lit.Type) {
				violations = append(violations, fmt.Sprintf("%s: errmsg.Part is built outside errmsg; use its constructors", pf.position(lit)))
			}
			for _, inner := range identitymutationguard.ElidedCompositeLiterals(lit, isPart) {
				violations = append(violations, fmt.Sprintf("%s: errmsg.Part is built outside errmsg through an elided literal", pf.position(inner)))
			}
			return true
		})

		for _, decl := range pf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !idx.fieldsContainPart(pf, fn.Type.Results, map[*ast.TypeSpec]struct{}{}) {
				continue
			}
			switch {
			case fn.Name.IsExported():
				violations = append(violations, fmt.Sprintf("%s: exported %s returns an errmsg.Part; only in-scope unexported code may", pf.position(fn), funcKey(fn)))
			case !inScope(pf.path, funcKey(fn)):
				violations = append(violations, fmt.Sprintf("%s: %s returns an errmsg.Part outside the in-scope positions", pf.position(fn), funcKey(fn)))
			}
		}
	}
	return partFound, violations
}

// TestProductionPartFieldsAreUnexportedAndUnbuiltOutsideErrmsg pins that a
// role is only ever chosen by errmsg's constructors: Part's fields are
// unexported, no code outside errmsg builds a Part literal, and a Part can only
// be handed out by unexported in-scope code.
func TestProductionPartFieldsAreUnexportedAndUnbuiltOutsideErrmsg(t *testing.T) {
	partFound, violations := checkPartConstruction(parseGuardFiles(t, productionGuardFiles(t)))
	require.True(t, partFound, "the errmsg.Part struct was not found; the scan is broken")
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestPartCheckRecognizesForms(t *testing.T) {
	const errmsgFile = "internal/errmsg/errmsg.go"
	const errmsgSrc = "package errmsg\n\ntype Part struct {\n\trole int\n\ttext string\n}\n"
	const header = "package x\n\nimport \"" + errmsgImportPath + "\"\n\n"
	tests := []struct {
		name      string
		errmsgSrc string
		file      guardFile
		want      int
	}{
		{name: "value literal", file: guardFile{"internal/x/x.go", header + "var _ = errmsg.Part{}\n"}, want: 1},
		{name: "pointer literal", file: guardFile{"internal/x/x.go", header + "var _ = &errmsg.Part{}\n"}, want: 1},
		{name: "elided slice element", file: guardFile{"internal/x/x.go", header + "var _ = []errmsg.Part{{}, {}}\n"}, want: 2},
		{name: "elided map value", file: guardFile{"internal/x/x.go", header + "var _ = map[string]errmsg.Part{\"a\": {}}\n"}, want: 1},
		{name: "exported Parts method", file: guardFile{"internal/x/x.go", header + "type T struct{}\n\nfunc (T) Parts() []errmsg.Part { return nil }\n"}, want: 1},
		{
			name: "exported function returning a struct holding a Part",
			file: guardFile{"internal/x/x.go", header + "type holder struct{ p errmsg.Part }\n\nfunc Make() *holder { return nil }\n"},
			want: 1,
		},
		{name: "unexported function outside the scope", file: guardFile{"internal/x/x.go", header + "func parts() []errmsg.Part { return nil }\n"}, want: 1},
		{
			name: "unexported method in scope",
			file: guardFile{"internal/runner/config/errors.go", "package config\n\nimport \"" + errmsgImportPath + "\"\n\ntype Level struct{}\n\nfunc (l Level) parts() []errmsg.Part { return nil }\n"},
		},
		{
			name: "unexported function of the expansion file in scope",
			file: guardFile{inScopeExpansionFile, "package config\n\nimport \"" + errmsgImportPath + "\"\n\nfunc expandParts() (p []errmsg.Part) { return }\n"},
		},
		{
			name: "function excluded from the expansion file",
			file: guardFile{inScopeExpansionFile, "package config\n\nimport \"" + errmsgImportPath + "\"\n\nfunc resolveAndPrepareCommandSpec() (p []errmsg.Part) { return }\n"},
			want: 1,
		},
		{name: "returning errmsg.Message is not a Part", file: guardFile{"internal/x/x.go", header + "func Make() errmsg.Message { return errmsg.Message{} }\n"}},
		{
			name: "a Part-like type of another package is not tracked",
			file: guardFile{"internal/x/x.go", "package x\n\ntype Part struct{ Role int }\n\nvar _ = Part{Role: 1}\n\nfunc Parts() []Part { return nil }\n"},
		},
		{name: "exported Part field", errmsgSrc: "package errmsg\n\ntype Part struct {\n\tRole int\n}\n", file: guardFile{"internal/x/x.go", "package x\n"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := errmsgSrc
			if tt.errmsgSrc != "" {
				src = tt.errmsgSrc
			}
			partFound, violations := checkPartConstruction(parseGuardFiles(t, []guardFile{{errmsgFile, src}, tt.file}))
			require.True(t, partFound)
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}

// checkStructuredErrorRendering reports every type that declares
// StructuredMessage without an Error() whose body is exactly
// `return <receiver>.StructuredMessage().String()`, and every type that
// declares Error() while it gets StructuredMessage by embedding (such as a
// type embedding *errmsg.Error). It returns the number of Error methods it
// accepted.
func checkStructuredErrorRendering(files []*parsedFile) (accepted int, violations []string) {
	idx := newTypeIndex(files)
	type methodSet map[string]*ast.FuncDecl
	methods := map[typeDecl]methodSet{}
	methodFile := map[*ast.FuncDecl]*parsedFile{}
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			d, ok := idx[pf.dir][receiverTypeName(fn)]
			if !ok {
				continue
			}
			if methods[d] == nil {
				methods[d] = methodSet{}
			}
			methods[d][fn.Name.Name] = fn
			methodFile[fn] = pf
		}
	}

	// hasStructuredMessage reports whether d declares StructuredMessage or
	// gets it from an embedded field.
	var hasStructuredMessage func(d typeDecl, visiting map[*ast.TypeSpec]struct{}) bool
	hasStructuredMessage = func(d typeDecl, visiting map[*ast.TypeSpec]struct{}) bool {
		if _, ok := methods[d]["StructuredMessage"]; ok {
			return true
		}
		st, ok := d.spec.Type.(*ast.StructType)
		if !ok {
			return false
		}
		if _, seen := visiting[d.spec]; seen {
			return false
		}
		visiting[d.spec] = struct{}{}
		defer delete(visiting, d.spec)
		for _, field := range st.Fields.List {
			if len(field.Names) != 0 {
				continue
			}
			embedded := identitymutationguard.UnwrapParen(field.Type)
			if star, ok := embedded.(*ast.StarExpr); ok {
				embedded = star.X
			}
			if inner, ok := idx.lookup(d.pf, embedded); ok && hasStructuredMessage(inner, visiting) {
				return true
			}
		}
		return false
	}

	for d, ms := range methods {
		errorMethod, declaresError := ms["Error"]
		_, declaresStructured := ms["StructuredMessage"]
		switch {
		case declaresStructured && !declaresError:
			violations = append(violations, fmt.Sprintf("%s: %s declares StructuredMessage but not Error; Error must render StructuredMessage", d.pf.position(d.spec), d.spec.Name.Name))
		case declaresError && !declaresStructured && hasStructuredMessage(d, map[*ast.TypeSpec]struct{}{}):
			violations = append(violations, fmt.Sprintf("%s: %s.Error overrides the Error of an embedded structured error", methodFile[errorMethod].position(errorMethod), d.spec.Name.Name))
		case declaresStructured && !rendersFromStructuredMessage(errorMethod):
			violations = append(violations, fmt.Sprintf("%s: %s.Error must be exactly `return <receiver>.StructuredMessage().String()`", methodFile[errorMethod].position(errorMethod), d.spec.Name.Name))
		case declaresStructured:
			accepted++
		}
	}
	slices.Sort(violations)
	return accepted, violations
}

// rendersFromStructuredMessage reports whether fn's body is exactly
// `return <receiver>.StructuredMessage().String()`.
func rendersFromStructuredMessage(fn *ast.FuncDecl) bool {
	if len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
		return false
	}
	receiver := fn.Recv.List[0].Names[0].Name
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return false
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	stringCall, ok := ret.Results[0].(*ast.CallExpr)
	if !ok || len(stringCall.Args) != 0 {
		return false
	}
	stringSel, ok := stringCall.Fun.(*ast.SelectorExpr)
	if !ok || stringSel.Sel.Name != "String" {
		return false
	}
	messageCall, ok := stringSel.X.(*ast.CallExpr)
	if !ok || len(messageCall.Args) != 0 {
		return false
	}
	messageSel, ok := messageCall.Fun.(*ast.SelectorExpr)
	if !ok || messageSel.Sel.Name != "StructuredMessage" {
		return false
	}
	recv, ok := messageSel.X.(*ast.Ident)
	return ok && recv.Name == receiver && receiver != "_"
}

// TestProductionStructuredErrorsRenderFromTheirMessage pins that an error with
// a structured message builds its Error() text from that message, so the
// unredacted rendering and Error() can never drift apart.
func TestProductionStructuredErrorsRenderFromTheirMessage(t *testing.T) {
	accepted, violations := checkStructuredErrorRendering(parseGuardFiles(t, productionGuardFiles(t)))
	require.GreaterOrEqual(t, accepted, 2, "errmsg.Error and errmsg.JoinedError were not both found; the scan is broken")
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestStructuredErrorRenderCheckRecognizesForms(t *testing.T) {
	const errmsgFile = "internal/errmsg/errmsg.go"
	const errmsgSrc = "package errmsg\n\ntype Message struct{}\n\nfunc (Message) String() string { return \"\" }\n\n" +
		"type Error struct{ msg Message }\n\nfunc (e *Error) Error() string { return e.StructuredMessage().String() }\n\n" +
		"func (e *Error) StructuredMessage() Message { return e.msg }\n"
	const header = "package x\n\nimport \"" + errmsgImportPath + "\"\n\ntype T struct{ msg errmsg.Message }\n\n" +
		"func (t *T) StructuredMessage() errmsg.Message { return t.msg }\n\n"
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "the one-statement form", src: header + "func (t *T) Error() string { return t.StructuredMessage().String() }\n"},
		{name: "two statements", src: header + "func (t *T) Error() string { s := t.StructuredMessage().String(); return s }\n", want: 1},
		{name: "a different expression", src: header + "func (t *T) Error() string { return t.msg.String() }\n", want: 1},
		{name: "a literal", src: header + "func (t *T) Error() string { return \"failed\" }\n", want: 1},
		{name: "another value's message", src: header + "var other T\n\nfunc (t *T) Error() string { return other.StructuredMessage().String() }\n", want: 1},
		{name: "StructuredMessage without Error", src: header, want: 1},
		{
			name: "embedding *errmsg.Error and declaring Error",
			src:  "package x\n\nimport \"" + errmsgImportPath + "\"\n\ntype W struct{ *errmsg.Error }\n\nfunc (w W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "embedding through a local type",
			src: "package x\n\nimport \"" + errmsgImportPath + "\"\n\ntype inner struct{ errmsg.Error }\n\n" +
				"type W struct{ inner }\n\nfunc (w *W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{name: "an unrelated error type", src: "package x\n\ntype E struct{}\n\nfunc (E) Error() string { return \"x\" }\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accepted, violations := checkStructuredErrorRendering(parseGuardFiles(t, []guardFile{{errmsgFile, errmsgSrc}, {"internal/x/x.go", tt.src}}))
			assert.Positive(t, accepted, "the synthetic errmsg.Error must be accepted")
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}
