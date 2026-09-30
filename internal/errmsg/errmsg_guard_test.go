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
// is a file and a function in it, never a function name alone.
// inScopeExpansionExclusions are removed from the expansion file.
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
	"internal/runner/base/output/path.go":        {"validatePathSecurity", "DefaultPathValidator.validateRelativePath"},
	"internal/runner/base/output/manager.go":     {"DefaultOutputCaptureManager.validateAndResolvePath"},
	"internal/runner/resource/normal_manager.go": {"NormalResourceManager.ExecuteCommand", "NormalResourceManager.ValidateOutputPath"},
	"internal/runner/resource/dryrun_manager.go": {"DryRunResourceManager.evaluateCommandRisk", "DryRunResourceManager.ValidateOutputPath"},
	"internal/dynlib/errors.go":                  {"ErrRecursionDepthExceeded.StructuredMessage", "ErrLibraryHashMismatch.StructuredMessage", "ErrEmptyLibraryPath.StructuredMessage", "ErrDynLibDepsRequired.StructuredMessage"},
	"internal/verification/errors.go":            {"ErrDynLibDepsResolutionChanged.StructuredMessage", "ErrInterpreterRecordNotFound.StructuredMessage", "ErrInterpreterSymlinkRedirected.StructuredMessage", "ErrInterpreterPathMismatch.StructuredMessage"},
}

// pathErrorCausePositions are the only positions allowed to split a
// *fs.PathError through errmsg.PathErrorCause.
var pathErrorCausePositions = positions{
	"internal/runner/base/executor/tempdir_manager.go": {"DefaultTempDirManager.Create"},
}

// inScopeExpansionFile is an exempt-role position as a whole file except for
// the functions in inScopeExpansionExclusions.
const inScopeExpansionFile = "internal/runner/config/expansion.go"

// inScopeExpansionExclusions are the functions of inScopeExpansionFile that
// build errors outside the scope of structured messages.
var inScopeExpansionExclusions = []string{
	"ProcessEnvImport", "ProcessEnv", "resolveAndPrepareCommandSpec", "ApplyTemplateInheritance", "expandTemplateToSpec",
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
	// errmsgDirect holds the directories that import errmsg in at least one
	// build. Only such a package can declare errmsg structure; a package that
	// reaches errmsg only through the packages it imports cannot, so its
	// build-variant type declarations are not part of the structure checks.
	errmsgDirect map[string]struct{}
}

// loadGuard parses files, marks the build variants among them and
// type-checks, per directory, the files the current build selects.
func loadGuard(t *testing.T, files []guardFile) *guardSet {
	t.Helper()
	set := &guardSet{variants: map[string]struct{}{}, errmsgDirect: map[string]struct{}{}}
	// errmsg does not import itself, so seed its own directory: it can declare
	// errmsg structure and its build-variant files must not declare types.
	set.errmsgDirect[errmsgDir] = struct{}{}
	groups := map[string][]*sourceFile{}
	for _, f := range files {
		file, err := parser.ParseFile(guardFset, f.path, f.src, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "failed to parse %s", f.path)
		sf := &sourceFile{guardFile: f, file: file}
		for _, imp := range file.Imports {
			if imp.Path.Value == strconv.Quote(errmsgImportPath) {
				set.errmsgDirect[path.Dir(f.path)] = struct{}{}
				break
			}
		}
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

// TestProductionGuardScopeIncludesDynLibAndVerification pins that the two
// packages whose errors now carry structured messages are inside the guard's
// production scan. Dropping their errmsg import would otherwise silently remove
// them from the Const, part-flow and exempt-role checks.
func TestProductionGuardScopeIncludesDynLibAndVerification(t *testing.T) {
	dirs := map[string]struct{}{}
	for _, f := range productionGuardFiles(t) {
		dirs[path.Dir(f.path)] = struct{}{}
	}
	assert.Contains(t, dirs, "internal/dynlib")
	assert.Contains(t, dirs, "internal/verification")
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

// declaresExemptRole reports whether the function keyed fn in file may
// declare an Identifier or Path segment.
func declaresExemptRole(file, fn string) bool {
	return exemptRolePositions.covers(file, fn) && (file != inScopeExpansionFile || !slices.Contains(inScopeExpansionExclusions, fn))
}

// checkExemptRoleCalls reports calls of errmsg.Ident and errmsg.Path outside
// exemptRolePositions, calls of errmsg.PathErrorCause outside
// pathErrorCausePositions, and inside errmsg calls of the role-choosing
// functions outside errmsgRoleChoosers. Every use of any of them that is not a
// call is reported.
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
		{name: "Ident in a dynlib error StructuredMessage", files: []guardFile{{"internal/dynlib/errors.go", "package dynlib\n\nimport \"" + errmsgImportPath + "\"\n\ntype ErrEmptyLibraryPath struct{ SOName string }\n\nfunc (e *ErrEmptyLibraryPath) StructuredMessage() errmsg.Message { return errmsg.NewMessage(errmsg.Ident(e.SOName)) }\n"}}},
		{name: "Ident in another dynlib function", files: []guardFile{{"internal/dynlib/errors.go", "package dynlib\n\nimport \"" + errmsgImportPath + "\"\n\nfunc other(s string) errmsg.Part { return errmsg.Ident(s) }\n"}}, want: 1},
		{name: "Path in a verification error StructuredMessage", files: []guardFile{{"internal/verification/errors.go", "package verification\n\nimport \"" + errmsgImportPath + "\"\n\ntype ErrInterpreterRecordNotFound struct{ Path string }\n\nfunc (e *ErrInterpreterRecordNotFound) StructuredMessage() errmsg.Message { return errmsg.NewMessage(errmsg.Path(e.Path)) }\n"}}},
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

// checkPartConstruction reports any exported or embedded field of
// errmsg.Part, every composite literal of type errmsg.Part outside
// errmsgPartBuilders, and inside errmsg every assignment to a field of Part. It
// returns whether the Part struct was found.
func checkPartConstruction(s *guardSet) (partFound bool, violations []string) {
	partName, _ := s.errmsg.Scope().Lookup("Part").(*types.TypeName)
	if partName == nil {
		return false, nil
	}
	part := partName.Type()
	partFields := map[*types.Var]struct{}{}
	if st, ok := part.Underlying().(*types.Struct); ok {
		partFound = true
		for field := range st.Fields() {
			partFields[field] = struct{}{}
		}
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
		for _, f := range p.files {
			for _, decl := range f.file.Decls {
				key := ""
				if fd, ok := decl.(*ast.FuncDecl); ok {
					key = funcKey(fd)
				}
				builder := p.dir == errmsgDir && slices.Contains(errmsgPartBuilders, key)
				ast.Inspect(decl, func(n ast.Node) bool {
					if lit, ok := n.(*ast.CompositeLit); ok && !builder && types.Identical(p.info.Types[lit].Type, part) {
						violations = append(violations, fmt.Sprintf("%s: errmsg.Part is built outside errmsg's part builders; use its constructors", position(lit.Pos())))
					}
					// Fields are unexported, so only errmsg can write one.
					for _, target := range writeTargets(n) {
						if sel, ok := identitymutationguard.UnwrapParen(target).(*ast.SelectorExpr); ok {
							if _, isPart := partFields[p.info.Uses[sel.Sel].(*types.Var)]; isPart {
								violations = append(violations, fmt.Sprintf("%s: a field of errmsg.Part is written; a role is chosen only by errmsg's part builders", position(target.Pos())))
							}
						}
					}
					return true
				})
			}
		}
	}
	return partFound, violations
}

// errmsgPartBuilders are the only functions of errmsg (keyed as funcKey)
// allowed to build a Part literal; errmsgRoleChoosers fixes their callers.
var errmsgPartBuilders = []string{"rolePart", "causePart"}

// writeTargets returns the expressions n may write through: assigned,
// incremented, ranged into, or addressed.
func writeTargets(n ast.Node) []ast.Expr {
	switch n := n.(type) {
	case *ast.AssignStmt:
		return n.Lhs
	case *ast.IncDecStmt:
		return []ast.Expr{n.X}
	case *ast.RangeStmt:
		if n.Tok == token.ASSIGN {
			return []ast.Expr{n.Key, n.Value}
		}
	case *ast.UnaryExpr:
		if n.Op == token.AND {
			return []ast.Expr{n.X}
		}
	}
	return nil
}

// TestProductionPartFieldsAreUnexportedAndUnbuiltOutsideErrmsg pins that a
// role is only ever chosen by errmsg's constructors: Part's fields are
// unexported and no code outside errmsg builds a Part literal.
func TestProductionPartFieldsAreUnexportedAndUnbuiltOutsideErrmsg(t *testing.T) {
	partFound, violations := checkPartConstruction(productionGuardSet(t))
	require.True(t, partFound, "the errmsg.Part struct was not found; the scan is broken")
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestPartCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport \"" + errmsgImportPath + "\"\n\n"
	tests := []struct {
		name   string
		errmsg string // replaces errmsg.go when set
		file   guardFile
		want   int
	}{
		{name: "value literal", file: guardFile{"internal/x/x.go", header + "var _ = errmsg.Part{}\n"}, want: 1},
		{name: "elided slice element", file: guardFile{"internal/x/x.go", header + "var _ = []errmsg.Part{{}, {}}\n"}, want: 2},
		{name: "elided map value", file: guardFile{"internal/x/x.go", header + "var _ = map[string]errmsg.Part{\"a\": {}}\n"}, want: 1},
		{name: "dot-import literal", file: guardFile{"internal/x/x.go", "package x\n\nimport . \"" + errmsgImportPath + "\"\n\nvar _ = Part{}\n"}, want: 1},
		{
			name: "a Part-like type of another package is not tracked",
			file: guardFile{"internal/x/x.go", "package x\n\ntype Part struct{ Role int }\n\nvar _ = Part{Role: 1}\n\nfunc Parts() []Part { return nil }\n"},
		},
		{name: "Part literal in a new errmsg function", file: guardFile{errmsgDir + "/y.go", "package errmsg\n\nfunc MakeIdent(s string) Part { return Part{role: RoleIdentifier, text: s} }\n"}, want: 1},
		{name: "Part field written through an embedding struct", file: guardFile{errmsgDir + "/y.go", "package errmsg\n\ntype w struct{ Part }\n\nfunc setRole(p *w) { p.role = RoleIdentifier }\n"}, want: 1},
		{
			name: "Part field incremented, addressed and ranged into",
			file: guardFile{errmsgDir + "/y.go", "package errmsg\n\nfunc bump(p *Part) {\n\tp.role++\n\tr := &p.role\n\t*r = RoleIdentifier\n\tfor _, p.role = range []Role{RolePath} {\n\t}\n}\n"},
			want: 3,
		},
		{name: "Part field written inside errmsg", file: guardFile{errmsgDir + "/y.go", "package errmsg\n\nfunc setRole(p *Part) { p.role = RoleIdentifier }\n"}, want: 1},
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

// partUsePositions are the positions, besides those allowed to declare an
// exempt role, where Parts may flow in any form. They still cannot call
// errmsg.Ident or errmsg.Path.
var partUsePositions = positions{
	// ContextString renders the context parts that ReportMessage also uses.
	"internal/logging/execution_error.go": {"ExecutionError.ContextString"},
}

// checkPartFlow reports, outside errmsg and outside the positions that may
// declare an exempt role or are in partUsePositions, every expression whose
// type holds an errmsg.Part and is not one of the listed forms (03 §9.8). The
// forms are an allowlist, so a form not listed is rejected: a Part can then
// only come from errmsg's constructors or be moved around locally, never be
// obtained from code that could have given it an exempt role.
func checkPartFlow(s *guardSet) []string {
	var violations []string
	for _, p := range s.pkgs {
		if p.dir == errmsgDir {
			continue
		}
		holdsPart := func(e ast.Expr) bool {
			tv, ok := p.info.Types[e]
			return ok && !tv.IsType() && s.containsPart(tv.Type, map[*types.TypeName]struct{}{})
		}
		var visit func(n ast.Node) bool
		// visitCallee checks what a call's callee expression evaluates besides
		// the named function itself: a method's receiver, or a function literal
		// or call producing the function.
		var visitCallee func(fun ast.Expr)
		visitCallee = func(fun ast.Expr) {
			switch f := identitymutationguard.UnwrapParen(fun).(type) {
			case *ast.Ident:
			case *ast.IndexExpr:
				visitCallee(f.X)
				// A map or slice index; type arguments are types and hold nothing.
				ast.Inspect(f.Index, visit)
			case *ast.IndexListExpr:
				visitCallee(f.X)
			case *ast.SelectorExpr:
				if _, isPkg := p.info.Uses[identOf(f.X)].(*types.PkgName); !isPkg {
					ast.Inspect(f.X, visit)
				}
			default:
				ast.Inspect(f, visit)
			}
		}
		visit = func(n ast.Node) bool {
			if cc, ok := n.(*ast.CaseClause); ok {
				// A type switch case naming a Part takes it out of an interface.
				for _, e := range cc.List {
					if tv := p.info.Types[e]; tv.IsType() && s.containsPart(tv.Type, map[*types.TypeName]struct{}{}) {
						violations = append(violations, fmt.Sprintf("%s: a type switch may not take an errmsg.Part out of an interface here", position(e.Pos())))
					}
				}
				return true
			}
			e, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			if call, ok := e.(*ast.CallExpr); ok && !p.info.Types[call.Fun].IsType() {
				if !s.movesPartsOnly(p, call.Fun) && s.handsPartOut(p, call) {
					violations = append(violations, fmt.Sprintf("%s: this call can hand an errmsg.Part back from a function that is not errmsg's, a Part mover's or a builtin", position(call.Pos())))
					return false
				}
				visitCallee(call.Fun)
				for _, arg := range call.Args {
					ast.Inspect(arg, visit)
				}
				return false
			}
			if sel, ok := e.(*ast.SelectorExpr); ok {
				// A method value keeps its receiver's address, so the method can
				// write a Part into the caller's variable later.
				if fn, ok := p.info.Uses[sel.Sel].(*types.Func); ok && fn.Signature().Recv() != nil &&
					s.reachesCaller(fn.Signature().Recv().Type(), map[*types.TypeName]struct{}{}) {
					violations = append(violations, fmt.Sprintf("%s: a method value whose receiver can hold an errmsg.Part may not be taken here", position(sel.Pos())))
					return false
				}
			}
			if !holdsPart(e) || isAllowedPartForm(p, e) {
				return true
			}
			violations = append(violations, fmt.Sprintf("%s: an errmsg.Part may not be taken from %T here; build it with errmsg's constructors or keep it in a local variable", position(e.Pos()), e))
			return false
		}
		for _, f := range p.files {
			for _, decl := range f.file.Decls {
				key := ""
				if fd, ok := decl.(*ast.FuncDecl); ok {
					key = funcKey(fd)
				}
				if declaresExemptRole(f.path, key) || partUsePositions.covers(f.path, key) {
					continue
				}
				ast.Inspect(decl, visit)
			}
		}
	}
	return violations
}

// identOf returns expr as an identifier, or nil.
func identOf(expr ast.Expr) *ast.Ident {
	id, _ := identitymutationguard.UnwrapParen(expr).(*ast.Ident)
	return id
}

// partMoverPackages are the packages, besides errmsg, whose functions may take
// and return Parts anywhere: they only move the values their caller gives
// them.
var partMoverPackages = map[string]struct{}{"slices": {}, "maps": {}}

// movesPartsOnly reports whether the callee fun cannot give a Part an exempt
// role on the caller's behalf: a builtin, a function or method of errmsg or of
// partMoverPackages, or a function literal, whose body is checked in place.
// errmsg's exempt-role constructors are held to their positions by
// checkExemptRoleCalls.
func (s *guardSet) movesPartsOnly(p *typedPackage, fun ast.Expr) bool {
	fun = identitymutationguard.UnwrapParen(fun)
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = f.X
	case *ast.IndexListExpr:
		fun = f.X
	}
	var id *ast.Ident
	switch f := identitymutationguard.UnwrapParen(fun).(type) {
	case *ast.FuncLit:
		return true
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	}
	switch obj := p.info.Uses[id].(type) {
	case *types.Builtin:
		return true
	case *types.Func:
		if obj.Pkg() == nil {
			// A universe method, such as error's Error, knows nothing of Parts.
			return true
		}
		_, mover := partMoverPackages[obj.Pkg().Path()]
		return obj.Pkg() == s.errmsg || mover
	default:
		return false
	}
}

// handsPartOut reports whether call can give its caller a Part: through a
// result, or through a receiver or parameter that reaches the caller's memory. A Part
// passed by value, alone, in a struct or array, or as a fresh variadic list,
// only goes in.
func (s *guardSet) handsPartOut(p *typedPackage, call *ast.CallExpr) bool {
	sig, ok := p.info.Types[call.Fun].Type.Underlying().(*types.Signature)
	if !ok {
		return false
	}
	if s.containsPart(sig.Results(), map[*types.TypeName]struct{}{}) {
		return true
	}
	// A method value's signature omits the receiver, which a pointer
	// receiver lets the method write through.
	if sel, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.SelectorExpr); ok {
		if fn, ok := p.info.Uses[sel.Sel].(*types.Func); ok && fn.Signature().Recv() != nil &&
			s.reachesCaller(fn.Signature().Recv().Type(), map[*types.TypeName]struct{}{}) {
			return true
		}
	}
	for i, param := range slices.Collect(sig.Params().Variables()) {
		t := param.Type()
		if sig.Variadic() && i == sig.Params().Len()-1 && !call.Ellipsis.IsValid() {
			if len(call.Args) < sig.Params().Len() {
				// No variadic argument: the callee gets an empty list.
				continue
			}
			t = t.(*types.Slice).Elem()
		}
		if s.reachesCaller(t, map[*types.TypeName]struct{}{}) {
			return true
		}
	}
	return false
}

// reachesCaller reports whether a parameter of type t can carry a Part back to
// the caller: a Part is reached through a pointer, slice, map, channel,
// function or interface, not through a copied value.
func (s *guardSet) reachesCaller(t types.Type, visiting map[*types.TypeName]struct{}) bool {
	if types.Identical(t, s.errmsg.Scope().Lookup("Part").Type()) {
		return false
	}
	switch u := types.Unalias(t).(type) {
	case *types.Struct:
		for field := range u.Fields() {
			if s.reachesCaller(field.Type(), visiting) {
				return true
			}
		}
		return false
	case *types.Array:
		return s.reachesCaller(u.Elem(), visiting)
	case *types.Named:
		if _, isStruct := u.Underlying().(*types.Struct); !isStruct {
			if _, isArray := u.Underlying().(*types.Array); !isArray {
				return s.containsPart(u, visiting)
			}
		}
		if u.Obj().Pkg() == s.errmsg {
			return false
		}
		if _, seen := visiting[u.Obj()]; seen {
			return false
		}
		visiting[u.Obj()] = struct{}{}
		defer delete(visiting, u.Obj())
		return s.reachesCaller(u.Underlying(), visiting)
	default:
		return s.containsPart(t, visiting)
	}
}

// isAllowedPartForm reports whether e is a form through which a Part may flow
// outside the positions that may declare an exempt role: a composite or
// function literal, a variable or parameter declared inside a function, or a
// parenthesized, indexed, sliced, dereferenced or received expression, whose
// operand is checked on its own. Taking an address is not allowed: the pointer
// would let code elsewhere write a Part into the caller's variable.
func isAllowedPartForm(p *typedPackage, e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.CompositeLit, *ast.FuncLit, *ast.ParenExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.SliceExpr, *ast.StarExpr:
		return true
	case *ast.UnaryExpr:
		return e.Op != token.AND
	case *ast.Ident:
		v, ok := p.info.Uses[e].(*types.Var)
		// Compare with the variable's own package: a dot-import makes another
		// package's variables unqualified.
		return ok && !v.IsField() && v.Parent() != v.Pkg().Scope()
	default:
		return false
	}
}

// TestProductionPartsFlowOnlyInAllowedForms pins that code which may not
// declare an exempt role cannot obtain a Part from code that may.
func TestProductionPartsFlowOnlyInAllowedForms(t *testing.T) {
	violations := checkPartFlow(productionGuardSet(t))
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestPartFlowCheckRecognizesForms(t *testing.T) {
	const imp = "import \"" + errmsgImportPath + "\"\n\n"
	const stageFile = "internal/runner/group_stage.go"
	const stageSrc = "package runner\n\n" + imp
	const identHelper = "func ident(s string) errmsg.Part { return errmsg.Ident(s) }\n"
	const runnerFile = "internal/runner/runner.go"
	const runnerSrc = stageSrc + "type Runner struct{}\n\n"
	stage := func(body string) guardFile { return guardFile{stageFile, stageSrc + body} }
	runner := func(body string) guardFile { return guardFile{runnerFile, runnerSrc + body} }
	tests := []struct {
		name  string
		files []guardFile
		want  int
	}{
		{
			name:  "Part helper called outside the scope",
			files: []guardFile{stage(identHelper), runner("func (r *Runner) Other(s string) errmsg.Message { return errmsg.NewMessage(ident(s)) }\n")},
			want:  1,
		},
		{
			name:  "Part helper called in scope but outside the exempt-role positions",
			files: []guardFile{stage(identHelper), runner("func (r *Runner) Execute(s string) errmsg.Message { return errmsg.NewMessage(ident(s)) }\n")},
			want:  1,
		},
		{
			name: "Part helper method of a generic type called outside the scope",
			files: []guardFile{
				stage("type box[T any] struct{}\n\nfunc (box[T]) ident(s string) errmsg.Part { return errmsg.Ident(s) }\n"),
				runner("func (r *Runner) Other(s string) errmsg.Message { return errmsg.NewMessage(box[int]{}.ident(s)) }\n"),
			},
			want: 1,
		},
		{
			name: "explicitly instantiated Part helper called outside the scope",
			files: []guardFile{
				stage("func part[T ~string](s T) errmsg.Part { return errmsg.Ident(string(s)) }\n"),
				runner("func (r *Runner) Other(s string) errmsg.Message { return errmsg.NewMessage(part[string](s)) }\n"),
			},
			want: 1,
		},
		{
			name: "exported Part helper of another package",
			files: []guardFile{
				stage("func IdentPart(s string) errmsg.Part { return errmsg.Ident(s) }\n"),
				{"internal/x/x.go", "package x\n\nimport (\n\t\"" + errmsgImportPath + "\"\n\t\"" + modulePath + "/internal/runner\"\n)\n\n" +
					"func F(s string) errmsg.Message { return errmsg.NewMessage(runner.IdentPart(s)) }\n"},
			},
			want: 1,
		},
		{
			name:  "Part helper called inside the receiver of a method call",
			files: []guardFile{stage(identHelper), runner("func (r *Runner) Other(s string) string { return errmsg.NewMessage(ident(s)).String() }\n")},
			want:  1,
		},
		{
			name:  "Part helper called inside an immediately invoked function literal",
			files: []guardFile{stage(identHelper), runner("func (r *Runner) Other(s string) string { return func() string { return errmsg.NewMessage(ident(s)).String() }() }\n")},
			want:  1,
		},
		{
			name:  "dot-imported errmsg constructors",
			files: []guardFile{{runnerFile, "package runner\n\nimport . \"" + errmsgImportPath + "\"\n\nfunc Other(s string) Message { return NewMessage(Const(\"a \"), Text(s)) }\n"}},
		},
		{
			name:  "function variable holding a Part helper",
			files: []guardFile{stage("var identPart = func(s string) errmsg.Part { return errmsg.Ident(s) }\n"), runner("func (r *Runner) Other(s string) errmsg.Message { return errmsg.NewMessage(identPart(s)) }\n")},
			want:  1,
		},
		{
			name:  "callback parameter returning a Part",
			files: []guardFile{runner("func (r *Runner) Other(f func(string) errmsg.Part, s string) errmsg.Message { return errmsg.NewMessage(f(s)) }\n")},
			want:  1,
		},
		{
			name: "helper filling a Part through a pointer",
			files: []guardFile{
				stage("func fill(dst *errmsg.Part, s string) { *dst = errmsg.Ident(s) }\n"),
				runner("func (r *Runner) Other(s string) errmsg.Message { var p errmsg.Part; fill(&p, s); return errmsg.NewMessage(p) }\n"),
			},
			want: 1,
		},
		{
			name:  "package variable holding a Part",
			files: []guardFile{stage("var exposed = errmsg.Ident(\"x\")\n"), runner("func (r *Runner) Other() errmsg.Message { return errmsg.NewMessage(exposed) }\n")},
			want:  1,
		},
		{
			name:  "Part taken back from an interface",
			files: []guardFile{stage("var Exposed any = errmsg.Ident(\"x\")\n"), runner("func (r *Runner) Other() errmsg.Message { return errmsg.NewMessage(Exposed.(errmsg.Part)) }\n")},
			want:  1,
		},
		{
			name:  "Part taken back from an interface by a type switch",
			files: []guardFile{stage("var Exposed any = errmsg.Ident(\"x\")\n"), runner("func (r *Runner) Other() errmsg.Message {\n\tswitch v := Exposed.(type) {\n\tcase errmsg.Part:\n\t\treturn errmsg.NewMessage(v)\n\t}\n\treturn errmsg.Message{}\n}\n")},
			want:  1,
		},
		{
			name: "package variable of another package reached through a dot-import",
			files: []guardFile{
				stage("var Exposed = errmsg.Ident(\"x\")\n"),
				{"internal/x/x.go", "package x\n\nimport (\n\t\"" + errmsgImportPath + "\"\n\t. \"" + modulePath + "/internal/runner\"\n)\n\nfunc F() errmsg.Message { return errmsg.NewMessage(Exposed) }\n"},
			},
			want: 1,
		},
		{
			name:  "Part helper called inside the index of a callee",
			files: []guardFile{stage(identHelper), runner("func (r *Runner) Other(s string, hs map[string]func()) { hs[errmsg.NewMessage(ident(s)).String()]() }\n")},
			want:  1,
		},
		{
			name:  "variadic Part helper given the caller's slice",
			files: []guardFile{stage("func fillAll(ps ...errmsg.Part) {}\n"), runner("func (r *Runner) Other(ps []errmsg.Part) { fillAll(ps...) }\n")},
			want:  1,
		},
		{
			name:  "generic identity helper returning a Part",
			files: []guardFile{stage("func pick[T any](v T) T { return v }\n\nvar _ errmsg.Message\n"), runner("func (r *Runner) Other() errmsg.Message { return errmsg.NewMessage(pick(errmsg.Const(\"x\"))) }\n")},
			want:  1,
		},
		{
			name: "Parts passed into closures and helpers by value",
			files: []guardFile{
				stage("func withPrefix(p errmsg.Part) errmsg.Message { return errmsg.NewMessage(errmsg.Const(\"x: \"), p) }\n\nfunc joinAll(ps ...errmsg.Part) errmsg.Message { return errmsg.NewMessage(ps...) }\n"),
				runner("func (r *Runner) Other(s string) []errmsg.Message {\n\tvar parts []errmsg.Part\n\tadd := func(p errmsg.Part) { parts = append(parts, p) }\n\tadd(errmsg.Const(\"x\"))\n" +
					"\tadd(func() errmsg.Part { return errmsg.Text(s) }())\n\treturn []errmsg.Message{errmsg.NewMessage(parts...), withPrefix(errmsg.Text(s)), joinAll(errmsg.Const(\"a\"), errmsg.Text(s))}\n}\n"),
			},
		},
		{
			name: "Parts passed by value inside a struct or array",
			files: []guardFile{
				stage("type pair struct{ P errmsg.Part }\n\nfunc consume(v pair, a [2]errmsg.Part) errmsg.Message { return errmsg.NewMessage(v.P, a[0]) }\n"),
				runner("func (r *Runner) Other(s string, err error) (errmsg.Message, string) {\n\treturn consume(pair{P: errmsg.Text(s)}, [2]errmsg.Part{errmsg.Const(\"a\"), errmsg.Text(s)}), err.Error()\n}\n"),
			},
		},
		{
			name:  "Part slice inside a struct reaches the caller",
			files: []guardFile{stage("type bag struct{ ps []errmsg.Part }\n\nfunc fillBag(b bag) { b.ps[0] = errmsg.Ident(\"x\") }\n"), runner("func (r *Runner) Other(ps []errmsg.Part) { fillBag(bag{ps: ps}) }\n")},
			want:  1,
		},
		{
			name: "method filling a Part through a pointer receiver",
			files: []guardFile{
				stage("type slot [1]errmsg.Part\n\nfunc (s *slot) fill(v string) { s[0] = errmsg.Ident(v) }\n"),
				runner("func (r *Runner) Other(v string) errmsg.Message { var s slot; s.fill(v); return errmsg.NewMessage(s[0]) }\n"),
			},
			want: 1,
		},
		{
			name: "method value keeping a pointer to a Part-holding variable",
			files: []guardFile{
				stage("type slot [1]errmsg.Part\n\nfunc (s *slot) fill(v string) { s[0] = errmsg.Ident(v) }\n"),
				runner("func (r *Runner) Other(v string) errmsg.Message { var s slot; f := s.fill; f(v); return errmsg.NewMessage(s[0]) }\n"),
			},
			want: 1,
		},
		{
			name: "address of a Part-holding variable converted to an interface",
			files: []guardFile{
				stage("type slot [1]errmsg.Part\n\nfunc (s *slot) Fill(v string) { s[0] = errmsg.Ident(v) }\n"),
				runner("func (r *Runner) Other(v string) errmsg.Message {\n\tvar s slot\n\tvar f interface{ Fill(string) } = &s\n\tf.Fill(v)\n\treturn errmsg.NewMessage(s[0])\n}\n"),
			},
			want: 1,
		},
		{
			name: "method with a value receiver and a variadic helper given no arguments",
			files: []guardFile{
				stage("type slot [1]errmsg.Part\n\nfunc (s slot) show() errmsg.Message { return errmsg.NewMessage(s[0]) }\n\nfunc fillAll(dsts ...*errmsg.Part) {}\n"),
				runner("func (r *Runner) Other() errmsg.Message { fillAll(); return slot{errmsg.Const(\"x\")}.show() }\n"),
			},
		},
		{
			name:  "variadic helper given a pointer to fill",
			files: []guardFile{stage("func fillAll(dsts ...*errmsg.Part) {}\n"), runner("func (r *Runner) Other() { var p errmsg.Part; fillAll(&p) }\n")},
			want:  1,
		},
		{
			name:  "field holding a Part",
			files: []guardFile{runner("type holder struct{ p errmsg.Part }\n\nfunc (r *Runner) Other(h holder) errmsg.Message { return errmsg.NewMessage(h.p) }\n")},
			want:  1,
		},
		{
			name:  "method value returning a Part",
			files: []guardFile{stage("type box struct{}\n\nfunc (box) ident(s string) errmsg.Part { return errmsg.Ident(s) }\n"), runner("func (r *Runner) Other() func(string) errmsg.Part { return box{}.ident }\n")},
			want:  1,
		},
		{
			name: "parts built from errmsg constructors, builtins and the standard library",
			files: []guardFile{{runnerFile, "package runner\n\nimport (\n\t\"slices\"\n\n\t\"" + errmsgImportPath + "\"\n)\n\n" +
				"func Other(s string, err error) error {\n" +
				"\tparts := []errmsg.Part{errmsg.Const(\"a \"), errmsg.Text(s), errmsg.ConstSummary(\"b\").Part()}\n" +
				"\tparts = append(parts, errmsg.Const(\": \"), errmsg.Cause(err))\n" +
				"\tfor i, p := range parts {\n\t\tparts[i] = p\n\t}\n" +
				"\treturn errmsg.NewError(slices.Concat[[]errmsg.Part](parts[:1], parts[1:])...)\n}\n"}},
		},
		{
			name:  "Part helper called from an exempt-role position",
			files: []guardFile{stage(identHelper), {"internal/runner/group_executor.go", "package runner\n\n" + imp + "func report(s string) errmsg.Message { return errmsg.NewMessage(ident(s), part[string](s)) }\n\nfunc part[T ~string](s T) errmsg.Part { return errmsg.Ident(string(s)) }\n"}},
		},
		{
			name: "context parts rendered by ContextString",
			files: []guardFile{{"internal/logging/execution_error.go", "package logging\n\n" + imp + "type ExecutionError struct{}\n\n" +
				"func (e *ExecutionError) contextParts() []errmsg.Part { return nil }\n\n" +
				"func (e *ExecutionError) ContextString() string { return errmsg.NewMessage(e.contextParts()...).String() }\n"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := checkPartFlow(syntheticGuard(t, tt.files...))
			assert.Len(t, violations, tt.want, strings.Join(violations, "\n"))
		})
	}
}

// checkStructuredErrorRendering reports every type of the checked packages,
// including an alias of an unnamed struct, that has a `StructuredMessage()
// errmsg.Message`, declared or promoted, and whose `Error() string` is neither
// declared on the type as exactly
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
		// Every type the package declares, including those inside functions.
		var typeNames []*types.TypeName
		for _, obj := range p.info.Defs {
			if tn, ok := obj.(*types.TypeName); ok && tn.Parent() != nil {
				typeNames = append(typeNames, tn)
			}
		}
		slices.SortFunc(typeNames, func(a, b *types.TypeName) int { return int(a.Pos() - b.Pos()) })
		for _, tn := range typeNames {
			name := tn.Name()
			if _, named := types.Unalias(tn.Type()).(*types.Named); tn.IsAlias() && named {
				// Checked as the named type it denotes.
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
			if !ok || !hasSignature(sm, message) {
				continue
			}
			errObj, errPath, _ := types.LookupFieldOrMethod(recv, false, p.pkg, "Error")
			errFn, _ := errObj.(*types.Func)
			if errFn != nil && !hasSignature(errFn, types.Typ[types.String]) {
				// Not the error interface's method.
				errFn = nil
			}
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

// hasSignature reports whether fn takes no parameters and returns exactly one
// value of type result.
func hasSignature(fn *types.Func, result types.Type) bool {
	sig := fn.Signature()
	return sig.Params().Len() == 0 && sig.Results().Len() == 1 && types.Identical(sig.Results().At(0).Type(), result)
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
	stringCall, ok := identitymutationguard.UnwrapParen(ret.Results[0]).(*ast.CallExpr)
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
		{name: "the one-statement form in parentheses", src: header + "func (t *T) Error() string { return (t.StructuredMessage().String()) }\n"},
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
		{
			name: "an alias of an unnamed struct getting its methods through different fields",
			src: imp + "type sm interface{ StructuredMessage() errmsg.Message }\n\n" +
				"type badError struct{}\n\nfunc (badError) Error() string { return \"x\" }\n\ntype W = struct {\n\tsm\n\tbadError\n}\n",
			want: 1,
		},
		{
			name: "a type declared inside a function getting its methods through different fields",
			src: imp + "type messageOnly interface{ StructuredMessage() errmsg.Message }\n\n" +
				"func f(m messageOnly, err error) error {\n\ttype combined struct {\n\t\tmessageOnly\n\t\terror\n\t}\n\treturn combined{m, err}\n}\n",
			want: 1,
		},
		{
			name: "a StructuredMessage taking a parameter is not the structured contract",
			src:  imp + "type T struct{}\n\nfunc (T) StructuredMessage(int) errmsg.Message { return errmsg.Message{} }\n\nfunc (T) Error() string { return \"x\" }\n",
		},
		{name: "an Error taking a parameter is not Error", src: header + "func (t *T) Error(int) string { return t.StructuredMessage().String() }\n", want: 1},
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
// declares a type in a package that imports errmsg, or declares a method named
// Error or StructuredMessage: the other checks type-check one build, so these
// must be the same in every supported build (03 §9.0).
//
// A type declaration only matters in a package that imports errmsg: structure
// is declared there, and a type declared in a package that merely reaches
// errmsg through its own imports cannot change the role of a segment the checks
// see. Such a type could gain structure only by embedding an errmsg-structured
// type, and that type's own declaration is checked where it is declared.
func checkBuildVariants(s *guardSet) []string {
	var violations []string
	for _, f := range s.files {
		if !f.variant {
			continue
		}
		_, direct := s.errmsgDirect[path.Dir(f.path)]
		for _, imp := range f.file.Imports {
			if imp.Path.Value == strconv.Quote(errmsgImportPath) {
				violations = append(violations, fmt.Sprintf("%s: a build-variant file must not import errmsg", position(imp.Pos())))
			}
		}
		for _, decl := range f.file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.TYPE || !direct {
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
			name: "a variant file declaring a type in a package that imports errmsg",
			files: []guardFile{
				{"internal/x/x.go", "package x\n\nimport \"" + errmsgImportPath + "\"\n\nvar _ = errmsg.Text(\"x\")\n"},
				{"internal/x/t_darwin.go", "package x\n\ntype T struct{}\n"},
			},
			want: 1,
		},
		{
			name:  "a variant file declaring a type in a package that does not import errmsg",
			files: []guardFile{{"internal/x/t_darwin.go", "package x\n\ntype T struct{}\n"}},
		},
		{
			name:  "a variant file declaring a type inside errmsg",
			files: []guardFile{{errmsgDir + "/t_darwin.go", "package errmsg\n\ntype T struct{}\n"}},
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
			name: "a cgo variant declaring a type in a package that imports errmsg",
			files: []guardFile{
				{"internal/x/x.go", "package x\n\nimport \"" + errmsgImportPath + "\"\n\nvar _ = errmsg.Text(\"x\")\n"},
				{"internal/x/c.go", "//go:build cgo\n\npackage x\n\ntype C struct{}\n"},
			},
			want: 1,
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
