//go:build test

package errmsg

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The guards type-check the packages they read with go/types and take names,
// constants, types and method selections from the type checker rather than
// inferring them from syntax. They look for mistakes in ordinary code; code
// written to evade them is out of scope (03 §9.0).

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
// is a file and a function in it, never a function name alone. The positions
// must also be in scope (see inScope), which removes the functions excluded
// from the expansion file.
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
// signature holds an errmsg.Part must be one of them.
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

// supportedBuilds are the builds the project ships (release.yml) and tests
// (ci.yml runs with cgo on and off). A file whose selection differs among them
// is a build variant (03 §9.0).
var supportedBuilds = func() []build.Context {
	var ctxs []build.Context
	for _, target := range [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "arm64"}} {
		for _, cgo := range []bool{false, true} {
			ctx := build.Default
			ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = target[0], target[1], cgo
			ctxs = append(ctxs, ctx)
		}
	}
	return ctxs
}()

// guardFset holds every position the checks report. It is shared with
// sourceImporter so the standard library is type-checked once per test binary.
var guardFset = token.NewFileSet()

// sourceImporter loads, from source, every package a check does not type-check
// itself.
var sourceImporter = sync.OnceValue(func() types.ImporterFrom {
	return importer.ForCompiler(guardFset, "source", nil).(types.ImporterFrom)
})

// guardFile is one Go source file given to the checks.
type guardFile struct {
	path string // slash-separated, relative to the repository root
	src  string
}

// sourceFile is a parsed guardFile.
type sourceFile struct {
	guardFile
	file *ast.File
	// variant reports whether the file's selection differs among
	// supportedBuilds.
	variant bool
}

// typedPackage is one package type-checked from the files the current build
// selects.
type typedPackage struct {
	dir   string
	pkg   *types.Package
	info  *types.Info
	files []*sourceFile
}

// guardSet is what the checks read.
type guardSet struct {
	files    []*sourceFile   // every given file, of every build
	pkgs     []*typedPackage // sorted by directory
	errmsg   *types.Package
	variants map[string]struct{} // paths of the build-variant files
}

// loadGuard parses files, marks the build variants among them and
// type-checks, per directory, the files the current build selects.
func loadGuard(t *testing.T, files []guardFile) *guardSet {
	t.Helper()
	set := &guardSet{variants: map[string]struct{}{}}
	groups := map[string][]*sourceFile{}
	for _, f := range files {
		file, err := parser.ParseFile(guardFset, f.path, f.src, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "failed to parse %s", f.path)
		sf := &sourceFile{guardFile: f, file: file}
		selected := map[bool]struct{}{}
		for _, ctx := range supportedBuilds {
			selected[selectedBy(t, ctx, sf)] = struct{}{}
		}
		if len(selected) > 1 {
			sf.variant = true
			set.variants[f.path] = struct{}{}
		}
		set.files = append(set.files, sf)
		if selectedBy(t, build.Default, sf) {
			importPath := modulePath + "/" + path.Dir(f.path)
			groups[importPath] = append(groups[importPath], sf)
		}
	}
	im := &guardImporter{t: t, groups: groups, checked: map[string]*typedPackage{}}
	for _, importPath := range slices.Sorted(maps.Keys(groups)) {
		set.pkgs = append(set.pkgs, im.check(importPath))
	}
	errmsgPkg, err := im.Import(errmsgImportPath)
	require.NoError(t, err)
	set.errmsg = errmsgPkg
	return set
}

// selectedBy reports whether ctx builds f.
func selectedBy(t *testing.T, ctx build.Context, f *sourceFile) bool {
	t.Helper()
	ctx.OpenFile = func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(f.src)), nil }
	ok, err := ctx.MatchFile(path.Dir(f.path), path.Base(f.path))
	require.NoErrorf(t, err, "failed to match the build constraints of %s", f.path)
	// MatchFile does not read imports, and a file importing "C" is built only
	// with cgo.
	return ok && (ctx.CgoEnabled || !slices.ContainsFunc(f.file.Imports, func(imp *ast.ImportSpec) bool {
		return imp.Path.Value == `"C"`
	}))
}

// guardImporter type-checks the given packages from their files, so they share
// one errmsg package, and loads every other import from source.
type guardImporter struct {
	t       *testing.T
	groups  map[string][]*sourceFile
	checked map[string]*typedPackage
}

func (im *guardImporter) Import(importPath string) (*types.Package, error) {
	return im.ImportFrom(importPath, "", 0)
}

func (im *guardImporter) ImportFrom(importPath, _ string, mode types.ImportMode) (*types.Package, error) {
	if _, ok := im.groups[importPath]; ok {
		return im.check(importPath).pkg, nil
	}
	// The file positions are repository-relative, so resolve from the root.
	return sourceImporter().ImportFrom(importPath, identitymutationguard.RepositoryRoot(im.t), mode)
}

func (im *guardImporter) check(importPath string) *typedPackage {
	if p, ok := im.checked[importPath]; ok {
		return p
	}
	files := im.groups[importPath]
	asts := make([]*ast.File, len(files))
	for i, f := range files {
		asts[i] = f.file
	}
	info := &types.Info{
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	conf := types.Config{Importer: im, FakeImportC: true}
	pkg, err := conf.Check(importPath, guardFset, asts, info)
	require.NoErrorf(im.t, err, "failed to type-check %s; the guards take names and types from the type checker", importPath)
	p := &typedPackage{dir: strings.TrimPrefix(importPath, modulePath+"/"), pkg: pkg, info: info, files: files}
	im.checked[importPath] = p
	return p
}

// productionGuardFiles returns the production files, of every build, of errmsg
// and of every package whose imports reach it (03 §9.0).
func productionGuardFiles(t *testing.T) []guardFile {
	t.Helper()
	byDir := map[string][]guardFile{}
	imports := map[string]map[string]struct{}{}
	for _, p := range identitymutationguard.ProductionGoFilesInRepo(t) {
		src := identitymutationguard.ReadProductionSource(t, p)
		dir := path.Dir(p)
		byDir[dir] = append(byDir[dir], guardFile{path: p, src: src})
		file, err := parser.ParseFile(token.NewFileSet(), p, src, parser.ImportsOnly)
		require.NoErrorf(t, err, "failed to parse the imports of %s", p)
		if imports[dir] == nil {
			imports[dir] = map[string]struct{}{}
		}
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			require.NoError(t, err)
			if dep, ok := strings.CutPrefix(importPath, modulePath+"/"); ok {
				imports[dir][dep] = struct{}{}
			}
		}
	}
	reach := map[string]struct{}{errmsgDir: {}}
	for changed := true; changed; {
		changed = false
		for dir, deps := range imports {
			if _, done := reach[dir]; done {
				continue
			}
			for dep := range deps {
				if _, ok := reach[dep]; ok {
					reach[dir] = struct{}{}
					changed = true
					break
				}
			}
		}
	}
	var files []guardFile
	for _, dir := range slices.Sorted(maps.Keys(reach)) {
		files = append(files, byDir[dir]...)
	}
	return files
}

var (
	productionGuardOnce sync.Once
	productionGuard     *guardSet
)

// productionGuardSet loads the production files once for every production
// check.
func productionGuardSet(t *testing.T) *guardSet {
	t.Helper()
	productionGuardOnce.Do(func() { productionGuard = loadGuard(t, productionGuardFiles(t)) })
	require.NotNil(t, productionGuard, "loading the production packages failed in an earlier test")
	return productionGuard
}

// syntheticGuard loads files for a self-test, together with errmsg's own
// production files unless files replace errmsg.go.
func syntheticGuard(t *testing.T, files ...guardFile) *guardSet {
	t.Helper()
	const errmsgFile = errmsgDir + "/errmsg.go"
	if !slices.ContainsFunc(files, func(f guardFile) bool { return f.path == errmsgFile }) {
		for _, p := range identitymutationguard.ProductionGoFiles(t, ".") {
			rel := errmsgDir + "/" + path.Base(p)
			files = append(files, guardFile{path: rel, src: identitymutationguard.ReadProductionSource(t, rel)})
		}
	}
	return loadGuard(t, files)
}

func position(pos token.Pos) string {
	p := guardFset.Position(pos)
	return fmt.Sprintf("%s:%d", p.Filename, p.Line)
}

// inVariantFile reports whether pos is in a build-variant file.
func (s *guardSet) inVariantFile(pos token.Pos) bool {
	_, ok := s.variants[guardFset.Position(pos).Filename]
	return ok
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

// errmsgRef is one use of a function the guard tracks.
type errmsgRef struct {
	name  string
	ident *ast.Ident
	call  *ast.CallExpr // nil when the function is used as a value
	fn    *ast.FuncDecl // enclosing function, nil at package level
	file  *sourceFile
}

// fnKey returns the enclosing function's key, or "" at package level.
func (r errmsgRef) fnKey() string {
	if r.fn == nil {
		return ""
	}
	return funcKey(r.fn)
}

// errmsgRefs returns every use in p of one of errmsg's package-level
// functions in names.
func (s *guardSet) errmsgRefs(p *typedPackage, names []string) []errmsgRef {
	return funcRefs(p, func(fn *types.Func) bool {
		return fn.Pkg() == s.errmsg && fn.Signature().Recv() == nil && slices.Contains(names, fn.Name())
	})
}

// funcRefs returns every use in p of a function or method that match accepts.
func funcRefs(p *typedPackage, match func(*types.Func) bool) []errmsgRef {
	var refs []errmsgRef
	for _, f := range p.files {
		for _, decl := range f.file.Decls {
			fn, _ := decl.(*ast.FuncDecl)
			calls := map[*ast.Ident]*ast.CallExpr{}
			ast.Inspect(decl, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					switch fun := identitymutationguard.UnwrapParen(call.Fun).(type) {
					case *ast.Ident:
						calls[fun] = call
					case *ast.SelectorExpr:
						calls[fun.Sel] = call
					}
				}
				return true
			})
			ast.Inspect(decl, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				obj, ok := p.info.Uses[id].(*types.Func)
				if ok && match(obj) {
					refs = append(refs, errmsgRef{name: obj.Name(), ident: id, call: calls[id], fn: fn, file: f})
				}
				return true
			})
		}
	}
	return refs
}

// constFuncNames are the constructors of RoleConstant parts.
var constFuncNames = []string{"Const", "ConstSummary"}

// checkConstCalls reports every call of errmsg.Const or errmsg.ConstSummary
// whose argument is not a constant expression, and every use of them that is
// not a call. It returns the number of calls it checked.
func checkConstCalls(s *guardSet) (checked int, violations []string) {
	for _, p := range s.pkgs {
		for _, ref := range s.errmsgRefs(p, constFuncNames) {
			if ref.call == nil {
				violations = append(violations, fmt.Sprintf("%s: errmsg.%s is referenced as a value; call it with a constant expression", position(ref.ident.Pos()), ref.name))
				continue
			}
			checked++
			if len(ref.call.Args) != 1 || !s.isConstantStringExpr(p, ref.call.Args[0]) {
				violations = append(violations, fmt.Sprintf("%s: errmsg.%s takes a string literal, a constant of the same package, or a + of them", position(ref.call.Pos()), ref.name))
			}
		}
	}
	return checked, violations
}

// isConstantStringExpr reports whether expr is a string literal, a name the
// type checker resolves to a constant of p declared outside the build-variant
// files, or a + of such expressions.
func (s *guardSet) isConstantStringExpr(p *typedPackage, expr ast.Expr) bool {
	switch e := identitymutationguard.UnwrapParen(expr).(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.BinaryExpr:
		return e.Op == token.ADD && s.isConstantStringExpr(p, e.X) && s.isConstantStringExpr(p, e.Y)
	case *ast.Ident:
		c, ok := p.info.Uses[e].(*types.Const)
		// A variant file may declare the name differently in another build.
		return ok && c.Pkg() == p.pkg && !s.inVariantFile(c.Pos())
	default:
		return false
	}
}

// TestProductionConstCallsUseConstantExpressions pins that a RoleConstant
// segment only ever holds bytes from a constant expression, so a value from
// the configuration or the environment can never escape the Text redaction by
// being passed as a constant.
func TestProductionConstCallsUseConstantExpressions(t *testing.T) {
	checked, violations := checkConstCalls(productionGuardSet(t))
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
		{
			name:  "a struct field named like a package constant does not shadow it",
			files: []guardFile{{"internal/x/x.go", src("", "const prefix = \"p\"\n\nfunc f() { _ = struct{ prefix string }{}; _ = errmsg.Const(prefix) }")}},
		},
		{name: "aliased import is followed", files: []guardFile{{"internal/x/x.go", src("em ", "func f(s string) { _ = em.Const(s) }")}}, want: 1},
		{name: "parameter", files: []guardFile{{"internal/x/x.go", src("", "func f(s string) { _ = errmsg.Const(s) }")}}, want: 1},
		{
			name:  "local variable shadowing a package constant",
			files: []guardFile{{"internal/x/x.go", src("", "const c = \"x\"\n\nfunc f(v string) { c := v; _ = errmsg.Const(c) }")}},
			want:  1,
		},
		{
			name:  "package-level closure parameter shadowing a package constant",
			files: []guardFile{{"internal/x/x.go", src("", "const c = \"x\"\n\nvar f = func(c string) errmsg.Part { return errmsg.Const(c) }")}},
			want:  1,
		},
		{
			name:  "nested local constant does not vouch for a package variable",
			files: []guardFile{{"internal/x/x.go", src("", "var c = g()\n\nfunc g() string { return \"\" }\n\nfunc f() { if false { const c = \"x\"; _ = c }; _ = errmsg.Const(c) }")}},
			want:  1,
		},
		{
			// Whichever variant the current build selects, the name is rejected.
			name: "a constant declared in a build-variant file",
			files: []guardFile{
				{"internal/x/x.go", src("", "func f() { _ = errmsg.Const(prefix) }")},
				{"internal/x/prefix_linux.go", "package x\n\nconst prefix = \"failed\"\n"},
				{"internal/x/prefix_other.go", "//go:build !linux\n\npackage x\n\nvar prefix = g()\n\nfunc g() string { return \"\" }\n"},
			},
			want: 1,
		},
		{name: "package variable", files: []guardFile{{"internal/x/x.go", src("", "var c = \"x\"\n\nfunc f() { _ = errmsg.Const(c) }")}}, want: 1},
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
			_, violations := checkConstCalls(syntheticGuard(t, tt.files...))
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}

// exemptRoleFuncNames are the errmsg functions whose call positions are
// checked by checkExemptRoleCalls.
var exemptRoleFuncNames = []string{"Ident", "Path", "PathErrorCause"}

// errmsgRoleChoosers are, for each errmsg function that chooses a segment's
// role or a cause's kind, the only functions of errmsg (keyed as funcKey)
// allowed to call it. errmsg calls the functions of exemptRoleFuncNames
// nowhere.
var errmsgRoleChoosers = map[string][]string{
	"rolePart":  {"Const", "Ident", "Path", "Text", "ConstSummary", "TextSummary", "Message.Freeze"},
	"causePart": {"Cause", "PathErrorCause", "IndentedCause"},
}

// partHelperCallerPositions are the positions, besides those allowed to
// declare an exempt role, allowed to call a helper whose signature holds an
// errmsg.Part.
var partHelperCallerPositions = positions{
	// ContextString renders the context parts that ReportMessage also uses.
	"internal/logging/execution_error.go": {"ExecutionError.ContextString"},
}

// declaresExemptRole reports whether the function keyed fn in file may
// declare an Identifier or Path segment.
func declaresExemptRole(file, fn string) bool {
	return exemptRolePositions.covers(file, fn) && inScope(file, fn)
}

// checkExemptRoleCalls reports calls of errmsg.Ident and errmsg.Path outside
// exemptRolePositions, calls of errmsg.PathErrorCause outside
// pathErrorCausePositions, calls of a package's own helpers whose signature
// holds an errmsg.Part outside those positions and partHelperCallerPositions,
// and inside errmsg calls of the role-choosing functions outside
// errmsgRoleChoosers. Every use of any of them that is not a call is reported.
func checkExemptRoleCalls(s *guardSet) []string {
	var violations []string
	for _, p := range s.pkgs {
		if p.dir == errmsgDir {
			names := append(slices.Clone(exemptRoleFuncNames), slices.Collect(maps.Keys(errmsgRoleChoosers))...)
			for _, ref := range s.errmsgRefs(p, names) {
				switch {
				case ref.call == nil:
					violations = append(violations, fmt.Sprintf("%s: %s is referenced as a value inside errmsg; only its fixed callers may call it", position(ref.ident.Pos()), ref.name))
				case !slices.Contains(errmsgRoleChoosers[ref.name], ref.fnKey()):
					violations = append(violations, fmt.Sprintf("%s: %s is called inside errmsg outside the constructors fixed for it", position(ref.call.Pos()), ref.name))
				}
			}
			continue
		}
		for _, ref := range s.errmsgRefs(p, exemptRoleFuncNames) {
			if ref.call == nil {
				violations = append(violations, fmt.Sprintf("%s: errmsg.%s is referenced as a value; only a call in an allowed position may declare a role", position(ref.ident.Pos()), ref.name))
				continue
			}
			switch ref.name {
			case "Ident", "Path":
				if !declaresExemptRole(ref.file.path, ref.fnKey()) {
					violations = append(violations, fmt.Sprintf("%s: errmsg.%s is called outside the positions allowed to declare an exempt role", position(ref.call.Pos()), ref.name))
				}
			default:
				if !pathErrorCausePositions.covers(ref.file.path, ref.fnKey()) {
					violations = append(violations, fmt.Sprintf("%s: errmsg.PathErrorCause is called outside the temporary directory creation", position(ref.call.Pos())))
				}
			}
		}
		// A helper returning or taking Parts can carry an exempt role to a
		// caller that may not declare one.
		helpers := map[*types.Func]struct{}{}
		for _, f := range p.files {
			for _, decl := range f.file.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok {
					if fn, ok := p.info.Defs[fd.Name].(*types.Func); ok && s.holdsPart(fn) {
						helpers[fn] = struct{}{}
					}
				}
			}
		}
		for _, ref := range funcRefs(p, func(fn *types.Func) bool { _, ok := helpers[fn.Origin()]; return ok }) {
			switch {
			case ref.call == nil:
				violations = append(violations, fmt.Sprintf("%s: %s holds an errmsg.Part and is referenced as a value; only a call in an allowed position may use it", position(ref.ident.Pos()), ref.name))
			case !declaresExemptRole(ref.file.path, ref.fnKey()) && !partHelperCallerPositions.covers(ref.file.path, ref.fnKey()):
				violations = append(violations, fmt.Sprintf("%s: %s holds an errmsg.Part and is called outside the positions allowed to declare an exempt role", position(ref.call.Pos()), ref.name))
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
	s := productionGuardSet(t)
	violations := checkExemptRoleCalls(s)
	assert.Empty(t, violations, strings.Join(violations, "\n"))

	// A renamed errmsg function would otherwise drop out of the table silently.
	declared := map[string]struct{}{}
	for _, p := range s.pkgs {
		if p.dir != errmsgDir {
			continue
		}
		for _, f := range p.files {
			for _, decl := range f.file.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok {
					declared[funcKey(fd)] = struct{}{}
				}
			}
		}
	}
	for chooser, callers := range errmsgRoleChoosers {
		for _, key := range append([]string{chooser}, callers...) {
			assert.Containsf(t, declared, key, "errmsgRoleChoosers names %s, which errmsg does not declare", key)
		}
	}
}

func TestExemptRoleCallCheckRecognizesForms(t *testing.T) {
	const header = "package executor\n\nimport %s\"" + errmsgImportPath + "\"\n\ntype DefaultExecutor struct{}\n\ntype DefaultTempDirManager struct{}\n\n"
	src := func(alias, body string) string { return fmt.Sprintf(header, alias) + body }
	const executorFile = "internal/runner/base/executor/executor.go"
	const tempdirFile = "internal/runner/base/executor/tempdir_manager.go"
	const stageFile = "internal/runner/group_stage.go"
	const stageSrc = "package runner\n\nimport \"" + errmsgImportPath + "\"\n\n"
	const identHelper = "func ident(s string) errmsg.Part { return errmsg.Ident(s) }\n"
	const runnerFile = "internal/runner/runner.go"
	const runnerSrc = stageSrc + "type Runner struct{}\n\n"
	tests := []struct {
		name  string
		files []guardFile
		want  int
	}{
		{name: "Ident in an allowed method", files: []guardFile{{executorFile, src("", `func (e *DefaultExecutor) Validate() { _ = errmsg.Ident("x") }`)}}},
		{name: "Path in a whole-file position", files: []guardFile{{"internal/runner/group_stage.go", "package runner\n\nimport \"" + errmsgImportPath + "\"\n\nvar _ = errmsg.Path(\"/p\")\n"}}},
		{name: "Ident in another method of the same file", files: []guardFile{{executorFile, src("", `func (e *DefaultExecutor) Other() { _ = errmsg.Ident("x") }`)}}, want: 1},
		{name: "allowed method name in another file", files: []guardFile{{"internal/logging/slack_handler.go", src("", `func (e *DefaultExecutor) Validate() { _ = errmsg.Ident("x") }`)}}, want: 1},
		{name: "allowed name as a function rather than a method", files: []guardFile{{executorFile, src("", `func Validate() { _ = errmsg.Path("/p") }`)}}, want: 1},
		{name: "package-level call in a function-scoped file", files: []guardFile{{executorFile, src("", `var _ = errmsg.Path("/p")`)}}, want: 1},
		{name: "dot-import in an allowed method", files: []guardFile{{executorFile, src(". ", `func (e *DefaultExecutor) executeNormal() { _ = Ident("x") }`)}}},
		{name: "function value", files: []guardFile{{executorFile, src("", `func (e *DefaultExecutor) Validate() { f := errmsg.Ident; _ = f }`)}}, want: 1},
		{name: "PathErrorCause in Create", files: []guardFile{{tempdirFile, src("", `func (m *DefaultTempDirManager) Create(err error) { _ = errmsg.PathErrorCause(err) }`)}}},
		{name: "PathErrorCause elsewhere", files: []guardFile{{executorFile, src("", `func (e *DefaultExecutor) Validate(err error) { _ = errmsg.PathErrorCause(err) }`)}}, want: 1},
		{
			name:  "Ident in the expansion file",
			files: []guardFile{{inScopeExpansionFile, "package config\n\nimport \"" + errmsgImportPath + "\"\n\nfunc expandVars() { _ = errmsg.Ident(\"v\") }\n"}},
		},
		{
			name:  "Ident in a function excluded from the expansion file",
			files: []guardFile{{inScopeExpansionFile, "package config\n\nimport \"" + errmsgImportPath + "\"\n\nfunc ProcessEnvImport() { _ = errmsg.Ident(\"v\") }\n"}},
			want:  1,
		},
		{name: "errmsg's own calls of its role-choosing functions", files: nil},
		{name: "Path called inside errmsg", files: []guardFile{{errmsgDir + "/y.go", "package errmsg\n\nfunc QuotedPath(s string) Part { return Path(s) }\n"}}, want: 1},
		{name: "rolePart called outside its fixed callers", files: []guardFile{{errmsgDir + "/y.go", "package errmsg\n\nfunc MakeIdent(s string) Part { return rolePart(RoleIdentifier, s) }\n"}}, want: 1},
		{name: "causePart called outside its fixed callers", files: []guardFile{{errmsgDir + "/y.go", "package errmsg\n\nfunc PathCause(err error) Part { return causePart(causePathError, err) }\n"}}, want: 1},
		{name: "rolePart referenced as a value inside errmsg", files: []guardFile{{errmsgDir + "/y.go", "package errmsg\n\nvar makePart = rolePart\n"}}, want: 1},
		{
			name:  "Part helper called outside the scope",
			files: []guardFile{{stageFile, stageSrc + identHelper}, {runnerFile, runnerSrc + "func (r *Runner) Other(s string) errmsg.Message { return errmsg.NewMessage(ident(s)) }\n"}},
			want:  1,
		},
		{
			name:  "Part helper called in scope but outside the exempt-role positions",
			files: []guardFile{{stageFile, stageSrc + identHelper}, {runnerFile, runnerSrc + "func (r *Runner) Execute(s string) errmsg.Message { return errmsg.NewMessage(ident(s)) }\n"}},
			want:  1,
		},
		{
			name: "Part helper method of a generic type called outside the scope",
			files: []guardFile{
				{stageFile, stageSrc + "type box[T any] struct{}\n\nfunc (box[T]) ident(s string) errmsg.Part { return errmsg.Ident(s) }\n"},
				{runnerFile, runnerSrc + "func (r *Runner) Other(s string) errmsg.Message { return errmsg.NewMessage(box[int]{}.ident(s)) }\n"},
			},
			want: 1,
		},
		{
			name:  "Part helper called from an exempt-role position",
			files: []guardFile{{stageFile, stageSrc + identHelper}, {"internal/runner/group_executor.go", "package runner\n\nimport \"" + errmsgImportPath + "\"\n\nfunc report(s string) errmsg.Message { return errmsg.NewMessage(ident(s)) }\n"}},
		},
		{name: "Part helper referenced as a value", files: []guardFile{{stageFile, stageSrc + identHelper + "\nvar f = ident\n"}}, want: 1},
		{
			name: "context parts rendered by ContextString",
			files: []guardFile{{"internal/logging/execution_error.go", "package logging\n\nimport \"" + errmsgImportPath + "\"\n\ntype ExecutionError struct{}\n\n" +
				"func (e *ExecutionError) contextParts() []errmsg.Part { return nil }\n\n" +
				"func (e *ExecutionError) ContextString() string { return errmsg.NewMessage(e.contextParts()...).String() }\n"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := checkExemptRoleCalls(syntheticGuard(t, tt.files...))
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}

// containsPart reports whether t holds an errmsg.Part: directly, as an
// element, a field, a function parameter or result, an interface method's
// parameter or result, a type argument, a type parameter's constraint, or
// through a named type's definition. errmsg's other types, such as Message, are
// opaque.
func (s *guardSet) containsPart(t types.Type, visiting map[*types.TypeName]struct{}) bool {
	if t == nil {
		return false
	}
	if types.Identical(t, s.errmsg.Scope().Lookup("Part").Type()) {
		return true
	}
	switch t := types.Unalias(t).(type) {
	case *types.Pointer:
		return s.containsPart(t.Elem(), visiting)
	case *types.Slice:
		return s.containsPart(t.Elem(), visiting)
	case *types.Array:
		return s.containsPart(t.Elem(), visiting)
	case *types.Chan:
		return s.containsPart(t.Elem(), visiting)
	case *types.Map:
		return s.containsPart(t.Key(), visiting) || s.containsPart(t.Elem(), visiting)
	case *types.Struct:
		for field := range t.Fields() {
			if s.containsPart(field.Type(), visiting) {
				return true
			}
		}
	case *types.Tuple:
		for v := range t.Variables() {
			if s.containsPart(v.Type(), visiting) {
				return true
			}
		}
	case *types.Signature:
		// A parameter can hand a Part out as well as a result: a channel, a
		// callback or a pointer to fill.
		return s.containsPart(t.Params(), visiting) || s.containsPart(t.Results(), visiting)
	case *types.Interface:
		for method := range t.Methods() {
			if s.containsPart(method.Type(), visiting) {
				return true
			}
		}
		for etyp := range t.EmbeddedTypes() {
			if s.containsPart(etyp, visiting) {
				return true
			}
		}
	case *types.Union:
		for term := range t.Terms() {
			if s.containsPart(term.Type(), visiting) {
				return true
			}
		}
	case *types.TypeParam:
		return s.containsPart(t.Constraint(), visiting)
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() == s.errmsg {
			return false
		}
		if _, seen := visiting[obj]; seen {
			return false
		}
		visiting[obj] = struct{}{}
		defer delete(visiting, obj)
		if args := t.TypeArgs(); args != nil {
			for t := range args.Types() {
				if s.containsPart(t, visiting) {
					return true
				}
			}
		}
		return s.containsPart(t.Underlying(), visiting)
	}
	return false
}

// holdsPart reports whether fn's parameters or results hold an errmsg.Part.
func (s *guardSet) holdsPart(fn *types.Func) bool {
	return s.containsPart(fn.Signature(), map[*types.TypeName]struct{}{})
}

// checkPartConstruction reports any exported or embedded field of
// errmsg.Part, and, outside errmsg, every composite literal of type
// errmsg.Part, every exported function, method or package-level variable whose
// type holds an errmsg.Part, and every unexported one outside the in-scope
// positions. It returns whether the Part struct was found.
func checkPartConstruction(s *guardSet) (partFound bool, violations []string) {
	partName, _ := s.errmsg.Scope().Lookup("Part").(*types.TypeName)
	if partName == nil {
		return false, nil
	}
	part := partName.Type()
	if st, ok := part.Underlying().(*types.Struct); ok {
		partFound = true
		for field := range st.Fields() {
			switch f := field; {
			case f.Embedded():
				violations = append(violations, fmt.Sprintf("%s: errmsg.Part must not embed a field", position(f.Pos())))
			case f.Exported():
				violations = append(violations, fmt.Sprintf("%s: errmsg.Part field %s must be unexported", position(f.Pos()), f.Name()))
			}
		}
	}

	for _, p := range s.pkgs {
		if p.dir == errmsgDir {
			continue
		}
		// report applies to a declared name whose type holds a Part the rule
		// that only in-scope unexported code may hand a Part out. scopeKey is
		// the key inScope checks: the function's, or "" for a package-level
		// variable.
		report := func(file *sourceFile, name *ast.Ident, scopeKey string) {
			switch {
			case name.IsExported():
				violations = append(violations, fmt.Sprintf("%s: exported %s holds an errmsg.Part; only in-scope unexported code may", position(name.Pos()), name.Name))
			case !inScope(file.path, scopeKey):
				violations = append(violations, fmt.Sprintf("%s: %s holds an errmsg.Part outside the in-scope positions", position(name.Pos()), name.Name))
			}
		}
		for _, f := range p.files {
			ast.Inspect(f.file, func(n ast.Node) bool {
				if lit, ok := n.(*ast.CompositeLit); ok && types.Identical(p.info.Types[lit].Type, part) {
					violations = append(violations, fmt.Sprintf("%s: errmsg.Part is built outside errmsg; use its constructors", position(lit.Pos())))
				}
				return true
			})
			for _, decl := range f.file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					if fn, ok := p.info.Defs[d.Name].(*types.Func); ok && s.holdsPart(fn) {
						report(f, d.Name, funcKey(d))
					}
				case *ast.GenDecl:
					if d.Tok != token.VAR {
						continue
					}
					for _, spec := range d.Specs {
						for _, name := range spec.(*ast.ValueSpec).Names {
							// The blank identifier cannot hand anything out.
							if v, ok := p.info.Defs[name].(*types.Var); ok && name.Name != "_" && s.containsPart(v.Type(), map[*types.TypeName]struct{}{}) {
								report(f, name, "")
							}
						}
					}
				}
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
	partFound, violations := checkPartConstruction(productionGuardSet(t))
	require.True(t, partFound, "the errmsg.Part struct was not found; the scan is broken")
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestPartCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport \"" + errmsgImportPath + "\"\n\n"
	const stageHeader = "package runner\n\nimport \"" + errmsgImportPath + "\"\n\n"
	const stageFile = "internal/runner/group_stage.go"
	tests := []struct {
		name   string
		errmsg string // replaces errmsg.go when set
		file   guardFile
		want   int
	}{
		{name: "value literal", file: guardFile{"internal/x/x.go", header + "var _ = errmsg.Part{}\n"}, want: 1},
		{name: "elided slice element", file: guardFile{"internal/x/x.go", header + "var _ = []errmsg.Part{{}, {}}\n"}, want: 2},
		{name: "elided map value", file: guardFile{"internal/x/x.go", header + "var _ = map[string]errmsg.Part{\"a\": {}}\n"}, want: 1},
		{name: "exported Parts method", file: guardFile{"internal/x/x.go", header + "type T struct{}\n\nfunc (T) Parts() []errmsg.Part { return nil }\n"}, want: 1},
		{
			name: "exported function returning a struct holding a Part",
			file: guardFile{"internal/x/x.go", header + "type holder struct{ p errmsg.Part }\n\nfunc Make() *holder { return nil }\n"},
			want: 1,
		},
		{name: "exported function returning an alias of a Part slice", file: guardFile{"internal/x/x.go", header + "type parts = []errmsg.Part\n\nfunc Make() parts { return nil }\n"}, want: 1},
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
		{name: "interface result with a Part method", file: guardFile{"internal/x/x.go", header + "func Make() interface{ Parts() []errmsg.Part } { return nil }\n"}, want: 1},
		{
			name: "named interface result embedding a Part method",
			file: guardFile{"internal/x/x.go", header + "type p interface{ Parts() []errmsg.Part }\n\ntype P interface{ p }\n\nfunc Make() P { return nil }\n"},
			want: 1,
		},
		{
			name: "result type parameter constrained to a Part",
			file: guardFile{stageFile, stageHeader + "func Make[T interface{ errmsg.Part }](s string) T { return any(errmsg.Ident(s)).(T) }\n"},
			want: 1,
		},
		{
			name: "exported function variable returning a Part in a whole-file position",
			file: guardFile{stageFile, stageHeader + "var MakeIdentifier = func(s string) errmsg.Part { return errmsg.Ident(s) }\n"},
			want: 1,
		},
		{
			name: "exported variable holding an in-scope helper that returns a Part",
			file: guardFile{stageFile, stageHeader + "func part(s string) errmsg.Part { return errmsg.Ident(s) }\n\nvar MakePart = part\n"},
			want: 1,
		},
		{name: "exported function sending a Part on a channel parameter", file: guardFile{stageFile, stageHeader + "func Emit(s string, out chan<- errmsg.Part) { out <- errmsg.Ident(s) }\n"}, want: 1},
		{name: "exported function passing a Part to a callback parameter", file: guardFile{stageFile, stageHeader + "func Visit(s string, emit func(errmsg.Part)) { emit(errmsg.Ident(s)) }\n"}, want: 1},
		{name: "exported function filling a Part through a pointer parameter", file: guardFile{stageFile, stageHeader + "func Fill(s string, dst *errmsg.Part) { *dst = errmsg.Ident(s) }\n"}, want: 1},
		{name: "exported variable of a function type taking a Part", file: guardFile{stageFile, stageHeader + "var Hook func(errmsg.Part)\n"}, want: 1},
		{name: "unexported in-scope function taking and returning Parts", file: guardFile{stageFile, stageHeader + "func appendParts(dst []errmsg.Part) []errmsg.Part { return dst }\n"}},
		{name: "unexported Part variable outside the scope", file: guardFile{"internal/x/x.go", header + "var parts []errmsg.Part\n"}, want: 1},
		{name: "exported variable of inferred Part type in a whole-file position", file: guardFile{stageFile, stageHeader + "var Exposed = errmsg.Ident(\"x\")\n"}, want: 1},
		{name: "unexported Part variable in a whole-file position", file: guardFile{stageFile, stageHeader + "var exposed = errmsg.Ident(\"x\")\n"}},
		{
			name: "unexported Part variable in a function-scoped file",
			file: guardFile{"internal/runner/config/errors.go", "package config\n\nimport \"" + errmsgImportPath + "\"\n\nvar exposed = errmsg.Ident(\"x\")\n"},
			want: 1,
		},
		{name: "dot-import literal", file: guardFile{"internal/x/x.go", "package x\n\nimport . \"" + errmsgImportPath + "\"\n\nvar _ = Part{}\n"}, want: 1},
		{name: "returning errmsg.Message is not a Part", file: guardFile{"internal/x/x.go", header + "func Make() errmsg.Message { return errmsg.Message{} }\n"}},
		{
			name: "a Part-like type of another package is not tracked",
			file: guardFile{"internal/x/x.go", "package x\n\ntype Part struct{ Role int }\n\nvar _ = Part{Role: 1}\n\nfunc Parts() []Part { return nil }\n"},
		},
		{name: "exported Part field", errmsg: "package errmsg\n\ntype Part struct {\n\tRole int\n}\n", file: guardFile{"internal/x/x.go", "package x\n"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := []guardFile{tt.file}
			if tt.errmsg != "" {
				files = append(files, guardFile{errmsgDir + "/errmsg.go", tt.errmsg})
			}
			partFound, violations := checkPartConstruction(syntheticGuard(t, files...))
			require.True(t, partFound)
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}

// checkStructuredErrorRendering reports every type of the checked packages
// that has a StructuredMessage returning errmsg.Message, declared or promoted,
// and whose Error is neither declared on the type as exactly
// `return <receiver>.StructuredMessage().String()` nor selected through the
// same embedding path as StructuredMessage. It returns the number of Error
// declarations it accepted.
func checkStructuredErrorRendering(s *guardSet) (accepted int, violations []string) {
	message := s.errmsg.Scope().Lookup("Message").Type()
	for _, p := range s.pkgs {
		decls := map[*types.Func]*ast.FuncDecl{}
		for _, f := range p.files {
			for _, decl := range f.file.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok {
					if fn, ok := p.info.Defs[fd.Name].(*types.Func); ok {
						decls[fn] = fd
					}
				}
			}
		}
		for _, name := range p.pkg.Scope().Names() {
			tn, ok := p.pkg.Scope().Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			switch tn.Type().Underlying().(type) {
			case *types.Interface, *types.Pointer:
				// An interface has no implementation to check, and a defined
				// pointer type has no methods.
				continue
			}
			recv := types.NewPointer(tn.Type())
			smObj, smPath, _ := types.LookupFieldOrMethod(recv, false, p.pkg, "StructuredMessage")
			sm, ok := smObj.(*types.Func)
			if !ok || sm.Signature().Results().Len() != 1 || !types.Identical(sm.Signature().Results().At(0).Type(), message) {
				continue
			}
			errObj, errPath, _ := types.LookupFieldOrMethod(recv, false, p.pkg, "Error")
			errFn, _ := errObj.(*types.Func)
			switch {
			case errFn != nil && len(errPath) == 1:
				if fd := decls[errFn.Origin()]; fd != nil && rendersFromStructuredMessage(fd) {
					accepted++
				} else {
					violations = append(violations, fmt.Sprintf("%s: %s.Error must be exactly `return <receiver>.StructuredMessage().String()`", position(errFn.Pos()), name))
				}
			case errFn != nil && slices.Equal(errPath[:len(errPath)-1], smPath[:len(smPath)-1]):
				// Both methods come from the same embedded value.
			case errFn == nil && len(smPath) == 1:
				violations = append(violations, fmt.Sprintf("%s: %s declares StructuredMessage but not Error; Error must render StructuredMessage", position(tn.Pos()), name))
			case errFn == nil:
				// A promoted StructuredMessage without Error is not an error.
			default:
				violations = append(violations, fmt.Sprintf("%s: %s gets Error through another embedding path than its StructuredMessage", position(tn.Pos()), name))
			}
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
	accepted, violations := checkStructuredErrorRendering(productionGuardSet(t))
	require.GreaterOrEqual(t, accepted, 2, "errmsg.Error and errmsg.JoinedError were not both found; the scan is broken")
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestStructuredErrorRenderCheckRecognizesForms(t *testing.T) {
	const imp = "package x\n\nimport \"" + errmsgImportPath + "\"\n\n"
	const header = imp + "type T struct{ msg errmsg.Message }\n\nfunc (t *T) StructuredMessage() errmsg.Message { return t.msg }\n\n"
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
			name: "embedding *errmsg.Error through a local type and declaring Error",
			src:  imp + "type inner struct{ *errmsg.Error }\n\ntype W struct{ inner }\n\nfunc (w *W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "embedding *errmsg.Error through a dot-import",
			src:  "package x\n\nimport . \"" + errmsgImportPath + "\"\n\ntype inner struct{ *Error }\n\ntype W struct{ inner }\n\nfunc (W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "embedding errmsg.Structured and declaring Error",
			src:  imp + "type W struct{ errmsg.Structured }\n\nfunc (W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "embedding a local interface that declares StructuredMessage",
			src:  imp + "type sm interface{ StructuredMessage() errmsg.Message }\n\ntype W struct{ sm }\n\nfunc (W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "embedding a generic type that embeds *errmsg.Error",
			src:  imp + "type inner[T any] struct{ *errmsg.Error }\n\ntype W struct{ inner[int] }\n\nfunc (W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "defined type over a struct embedding *errmsg.Error",
			src:  imp + "type inner struct{ *errmsg.Error }\n\ntype base struct{ inner }\n\ntype W base\n\nfunc (W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "defined type does not get the methods declared on its underlying type",
			src: imp + "type base struct{ msg errmsg.Message }\n\n" +
				"func (b base) StructuredMessage() errmsg.Message { return b.msg }\n\n" +
				"func (b base) Error() string { return b.StructuredMessage().String() }\n\n" +
				"type W base\n\nfunc (W) Error() string { return \"x\" }\n",
		},
		{
			name: "embedding an alias of errmsg.Error and declaring Error",
			src:  imp + "type inner = errmsg.Error\n\ntype W struct{ *inner }\n\nfunc (w W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "embedding an alias of an unnamed struct and declaring Error",
			src:  imp + "type inner = struct{ *errmsg.Error }\n\ntype W struct{ inner }\n\nfunc (W) Error() string { return \"x\" }\n",
			want: 1,
		},
		{
			name: "StructuredMessage and Error promoted through different fields",
			src: imp + "type sm interface{ StructuredMessage() errmsg.Message }\n\n" +
				"type badError struct{}\n\nfunc (badError) Error() string { return \"x\" }\n\ntype W struct {\n\tsm\n\tbadError\n}\n",
			want: 1,
		},
		{
			// The shallower StructuredMessage of A shadows G's, while Error
			// still comes from G.
			name: "a shallower StructuredMessage shadows the one paired with Error",
			src: imp + "type A interface{ StructuredMessage() errmsg.Message }\n\n" +
				"type G struct{ msg errmsg.Message }\n\nfunc (g G) StructuredMessage() errmsg.Message { return g.msg }\n\n" +
				"func (g G) Error() string { return g.StructuredMessage().String() }\n\n" +
				"type B struct{ G }\n\ntype W struct {\n\tA\n\tB\n}\n",
			want: 1,
		},
		{name: "an embedded structured error promoting both methods", src: imp + "type W struct{ *errmsg.Error }\n"},
		{
			name: "an interface embedding error and listing StructuredMessage",
			src:  imp + "type S interface {\n\terror\n\tStructuredMessage() errmsg.Message\n}\n",
		},
		{name: "an unrelated error type", src: "package x\n\ntype E struct{}\n\nfunc (E) Error() string { return \"x\" }\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accepted, violations := checkStructuredErrorRendering(syntheticGuard(t, guardFile{"internal/x/x.go", tt.src}))
			assert.Positive(t, accepted, "errmsg's own Error must be accepted")
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}

// checkBuildVariants reports every build-variant file that imports errmsg,
// declares a type, or declares a method named Error or StructuredMessage: the
// other checks type-check one build, so these must be the same in every
// supported build (03 §9.0).
func checkBuildVariants(s *guardSet) []string {
	var violations []string
	for _, f := range s.files {
		if !f.variant {
			continue
		}
		for _, imp := range f.file.Imports {
			if imp.Path.Value == strconv.Quote(errmsgImportPath) {
				violations = append(violations, fmt.Sprintf("%s: a build-variant file must not import errmsg", position(imp.Pos())))
			}
		}
		for _, decl := range f.file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					ts := spec.(*ast.TypeSpec)
					violations = append(violations, fmt.Sprintf("%s: a build-variant file must not declare type %s", position(ts.Pos()), ts.Name.Name))
				}
			case *ast.FuncDecl:
				if d.Recv != nil && (d.Name.Name == "Error" || d.Name.Name == "StructuredMessage") {
					violations = append(violations, fmt.Sprintf("%s: a build-variant file must not declare %s", position(d.Pos()), funcKey(d)))
				}
			}
		}
	}
	return violations
}

// TestProductionBuildVariantFilesDoNotDeclareStructure pins that the files the
// other checks may not see in the current build declare nothing those checks
// read.
func TestProductionBuildVariantFilesDoNotDeclareStructure(t *testing.T) {
	violations := checkBuildVariants(productionGuardSet(t))
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestBuildVariantCheckRecognizesForms(t *testing.T) {
	const imp = "import \"" + errmsgImportPath + "\"\n\n"
	const common = "package x\n\ntype T struct{}\n"
	tests := []struct {
		name  string
		files []guardFile
		want  int
	}{
		{
			name:  "a variant file importing errmsg",
			files: []guardFile{{"internal/x/f_linux.go", "package x\n\n" + imp + "var _ = errmsg.Text(\"x\")\n"}, {"internal/x/f_other.go", "//go:build !linux\n\npackage x\n"}},
			want:  1,
		},
		{
			name:  "a variant file declaring a type",
			files: []guardFile{{"internal/x/t_darwin.go", "package x\n\ntype T struct{}\n"}},
			want:  1,
		},
		{
			name: "a variant file declaring Error",
			files: []guardFile{
				{"internal/x/x.go", common},
				{"internal/x/e_linux.go", "package x\n\nfunc (T) Error() string { return \"\" }\n"},
				{"internal/x/e_other.go", "//go:build !linux\n\npackage x\n\nfunc (T) Error() string { return \"\" }\n"},
			},
			want: 2,
		},
		{
			name:  "a cgo variant declaring a type",
			files: []guardFile{{"internal/x/c.go", "//go:build cgo\n\npackage x\n\ntype C struct{}\n"}},
			want:  1,
		},
		{
			name: "a file in every supported build is not a variant",
			files: []guardFile{{"internal/x/x.go", "//go:build !windows\n\npackage x\n\n" + imp + "type T struct{ msg errmsg.Message }\n\n" +
				"func (t T) StructuredMessage() errmsg.Message { return t.msg }\n\nfunc (t T) Error() string { return t.StructuredMessage().String() }\n"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := checkBuildVariants(syntheticGuard(t, tt.files...))
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}
