//go:build test

package runner

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
	"strings"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scopedFunc names one function or method the wrap guard scans, by its
// repository-relative file and its receiver-qualified key.
type scopedFunc struct {
	file string
	fn   string
}

// typeRef names one type declaration by file and bare type name.
type typeRef struct {
	file string
	name string
}

// inScopeWholeFiles are the files whose every top-level function is in scope
// for the wrap guard: the group execution and expansion paths keep their wrappers in
// one file, so a whole-file rule cannot fall out of scope silently when a
// function is split or added.
var inScopeWholeFiles = []string{
	"internal/runner/group_executor.go",
	"internal/runner/group_stage.go",
	"internal/runner/group_errors.go",
	"internal/runner/config/expansion.go",
	"internal/runner/config/errors.go",
	"internal/runner/config/template_errors.go",
	"internal/runner/config/template_expansion.go",
	"internal/runner/cli/filter.go",
}

// inScopeFunctions are the functions and methods that are in scope for the guard
// but live in files that also hold out-of-scope code. runner.go, resource and
// executor are named per function for that reason.
var inScopeFunctions = []scopedFunc{
	{"internal/runner/runner.go", "(*Runner).Execute"},
	{"internal/runner/runner.go", "(*Runner).ExecuteGroup"},
	{"internal/runner/runner.go", "(*Runner).executeGroups"},
	{"internal/runner/resource/normal_manager.go", "(*NormalResourceManager).ExecuteCommand"},
	{"internal/runner/resource/normal_manager.go", "(*NormalResourceManager).executeCommandWithOutput"},
	{"internal/runner/resource/normal_manager.go", "(*NormalResourceManager).ValidateOutputPath"},
	{"internal/runner/resource/dryrun_manager.go", "(*DryRunResourceManager).ExecuteCommand"},
	{"internal/runner/resource/dryrun_manager.go", "(*DryRunResourceManager).evaluateCommandRisk"},
	{"internal/runner/resource/dryrun_manager.go", "(*DryRunResourceManager).ValidateOutputPath"},
	{"internal/runner/base/output/manager.go", "(*DefaultOutputCaptureManager).ValidateOutputPath"},
	{"internal/runner/base/output/manager.go", "(*DefaultOutputCaptureManager).validateAndResolvePath"},
	{"internal/runner/base/output/path.go", "(*DefaultPathValidator).ValidateAndResolvePath"},
	{"internal/runner/base/output/path.go", "(*DefaultPathValidator).validateRelativePath"},
	{"internal/runner/base/output/path.go", "validatePathSecurity"},
	{"internal/runner/base/executor/tempdir_manager.go", "(*DefaultTempDirManager).Create"},
	{"internal/runner/base/executor/executor.go", "(*DefaultExecutor).Validate"},
	{"internal/runner/base/executor/executor.go", "(*DefaultExecutor).validatePrivilegedCommand"},
	{"internal/runner/base/executor/executor.go", "(*DefaultExecutor).executeNormal"},
	{"internal/runner/base/executor/executor.go", "(*DefaultExecutor).executeWithUserGroup"},
	{"internal/runner/base/executor/command_lifecycle.go", "(*DefaultExecutor).runCommand"},
	{"internal/runner/base/executor/command_lifecycle.go", "(*DefaultExecutor).reportStartFailure"},
	{"internal/runner/base/executor/command_lifecycle.go", "(*DefaultExecutor).superviseCommand"},
	{"internal/runner/base/executor/command_lifecycle.go", "(*DefaultExecutor).killChild"},
	{"internal/runner/base/executor/command_lifecycle.go", "killOutcome"},
	{"internal/runner/base/privilege/errors.go", "(*Error).StructuredMessage"},
	{"internal/runner/base/privilege/unix.go", "(*UnixPrivilegeManager).performElevation"},
	{"internal/logging/pre_execution_error.go", "(*PreExecutionError).DetailMessage"},
	{"internal/logging/execution_error.go", "(*ExecutionError).ReportMessage"},
	{"internal/logging/execution_error.go", "(*ExecutionError).contextParts"},
}

// inScopeErrorTypes are the types whose StructuredMessage declaration the
// guard pins explicitly. They wrap causes reachable from the in-scope
// functions but are declared in files that are only partly in scope, so the
// whole-file type check does not see them.
var inScopeErrorTypes = []typeRef{
	{"internal/runner/runner.go", "cancelledRunError"},
	{"internal/runner/base/executor/command_lifecycle.go", "killAfterCancelError"},
	{"internal/runner/base/privilege/errors.go", "Error"},
}

// unwrapWithoutStructuredExceptions are the types allowed to declare Unwrap
// without StructuredMessage: the two logging report types build their message
// from DetailMessage/ReportMessage instead.
var unwrapWithoutStructuredExceptions = []typeRef{
	{"internal/logging/pre_execution_error.go", "PreExecutionError"},
	{"internal/logging/execution_error.go", "ExecutionError"},
}

// wrapGuardModulePath and wrapGuardErrmsgImport identify the errmsg package by
// import path, so the checks recognize errmsg.Message through the type checker
// without importing the package themselves.
const (
	wrapGuardModulePath   = "github.com/isseis/go-safe-cmd-runner"
	wrapGuardErrmsgImport = wrapGuardModulePath + "/internal/errmsg"
)

// wrapGuardFile is one Go source file given to the wrap guard's type checker.
type wrapGuardFile struct {
	path string
	src  string
}

// wrapSourceFile is a parsed wrapGuardFile.
type wrapSourceFile struct {
	wrapGuardFile
	file *ast.File
}

// wrapTypedPackage is one package type-checked from the files the current build
// selects.
type wrapTypedPackage struct {
	dir   string
	pkg   *types.Package
	info  *types.Info
	files []*wrapSourceFile
}

// file returns the parsed file at filePath, or nil when the package does not
// hold it in the current build.
func (p *wrapTypedPackage) file(filePath string) *ast.File {
	for _, f := range p.files {
		if f.path == filePath {
			return f.file
		}
	}
	return nil
}

// wrapGuardSet is the type-checked view the wrap guard reads: every relevant
// package, keyed by its repository-relative directory.
type wrapGuardSet struct {
	pkgs map[string]*wrapTypedPackage
}

// packageForFile returns the package holding the repository-relative file path.
func (s *wrapGuardSet) packageForFile(filePath string) *wrapTypedPackage {
	return s.pkgs[path.Dir(filePath)]
}

// wrapGuardFset and wrapSourceImporter are shared with the importer so the
// standard library is type-checked once per test binary.
var (
	wrapGuardFset      = token.NewFileSet()
	wrapSourceImporter = sync.OnceValue(func() types.ImporterFrom {
		return importer.ForCompiler(wrapGuardFset, "source", nil).(types.ImporterFrom)
	})
)

// wrapSelectedBy reports whether the current build selects f, applying its
// //go:build constraints and skipping a file that imports "C" without cgo.
func wrapSelectedBy(t *testing.T, f *wrapSourceFile) bool {
	t.Helper()
	ctx := build.Default
	ctx.OpenFile = func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(f.src)), nil }
	ok, err := ctx.MatchFile(path.Dir(f.path), path.Base(f.path))
	require.NoErrorf(t, err, "failed to match the build constraints of %s", f.path)
	// MatchFile does not read imports, and a file importing "C" is built only
	// with cgo.
	return ok && (ctx.CgoEnabled || !slices.ContainsFunc(f.file.Imports, func(imp *ast.ImportSpec) bool {
		return imp.Path.Value == `"C"`
	}))
}

// wrapImporter type-checks the requested in-repo packages from their files, so
// they share one errmsg package, and loads every other import from source.
type wrapImporter struct {
	t       *testing.T
	groups  map[string][]*wrapSourceFile
	checked map[string]*wrapTypedPackage
}

func (im *wrapImporter) Import(importPath string) (*types.Package, error) {
	return im.ImportFrom(importPath, "", 0)
}

func (im *wrapImporter) ImportFrom(importPath, _ string, mode types.ImportMode) (*types.Package, error) {
	if _, ok := im.groups[importPath]; ok {
		return im.check(importPath).pkg, nil
	}
	// The file positions are repository-relative, so resolve from the root.
	return wrapSourceImporter().ImportFrom(importPath, identitymutationguard.RepositoryRoot(im.t), mode)
}

func (im *wrapImporter) check(importPath string) *wrapTypedPackage {
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
	pkg, err := conf.Check(importPath, wrapGuardFset, asts, info)
	require.NoErrorf(im.t, err, "failed to type-check %s; the wrap guard resolves names and types with the type checker", importPath)
	p := &wrapTypedPackage{dir: strings.TrimPrefix(importPath, wrapGuardModulePath+"/"), pkg: pkg, info: info, files: files}
	im.checked[importPath] = p
	return p
}

// loadWrapGuard parses files, type-checks the packages the current build
// selects, and returns the ones named by wantDirs (all of them when wantDirs is
// nil). An in-repo import is type-checked from the given files; every other
// import is loaded from source.
func loadWrapGuard(t *testing.T, files []wrapGuardFile, wantDirs map[string]bool) *wrapGuardSet {
	t.Helper()
	groups := map[string][]*wrapSourceFile{}
	for _, f := range files {
		file, err := parser.ParseFile(wrapGuardFset, f.path, f.src, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "failed to parse %s", f.path)
		sf := &wrapSourceFile{wrapGuardFile: f, file: file}
		if !wrapSelectedBy(t, sf) {
			continue
		}
		importPath := wrapGuardModulePath + "/" + path.Dir(f.path)
		groups[importPath] = append(groups[importPath], sf)
	}
	im := &wrapImporter{t: t, groups: groups, checked: map[string]*wrapTypedPackage{}}
	set := &wrapGuardSet{pkgs: map[string]*wrapTypedPackage{}}
	for _, importPath := range slices.Sorted(maps.Keys(groups)) {
		dir := strings.TrimPrefix(importPath, wrapGuardModulePath+"/")
		if wantDirs != nil && !wantDirs[dir] {
			continue
		}
		set.pkgs[dir] = im.check(importPath)
	}
	return set
}

// loadWrapGuardFromMap is loadWrapGuard for a set of synthetic files.
func loadWrapGuardFromMap(t *testing.T, srcs map[string]string) *wrapGuardSet {
	t.Helper()
	files := make([]wrapGuardFile, 0, len(srcs))
	for p, src := range srcs {
		files = append(files, wrapGuardFile{path: p, src: src})
	}
	return loadWrapGuard(t, files, nil)
}

// syntheticWrapPackage type-checks one synthetic file and returns its package.
func syntheticWrapPackage(t *testing.T, filePath, src string) *wrapTypedPackage {
	t.Helper()
	return syntheticWrapPackageFiles(t, map[string]string{filePath: src}, filePath)
}

// syntheticWrapPackageFiles type-checks several synthetic files and returns the
// package holding want.
func syntheticWrapPackageFiles(t *testing.T, srcs map[string]string, want string) *wrapTypedPackage {
	t.Helper()
	set := loadWrapGuardFromMap(t, srcs)
	pkg := set.packageForFile(want)
	require.NotNilf(t, pkg, "no type-checked package for %s", want)
	return pkg
}

// relevantPackageDirs returns the directories of the scope tables and the
// exception/type lists: the only packages whose type-checked view the checks
// read directly.
func relevantPackageDirs() map[string]bool {
	dirs := make(map[string]bool)
	for _, file := range inScopeWholeFiles {
		dirs[path.Dir(file)] = true
	}
	for _, sf := range inScopeFunctions {
		dirs[path.Dir(sf.file)] = true
	}
	for _, ref := range inScopeErrorTypes {
		dirs[path.Dir(ref.file)] = true
	}
	for _, ref := range unwrapWithoutStructuredExceptions {
		dirs[path.Dir(ref.file)] = true
	}
	return dirs
}

// wrapScope is the scan's scope: whole-file units (with per-file excluded
// function keys), function-unit entries, and the type-checked package set the
// checks resolve names and method signatures against.
type wrapScope struct {
	wholeFiles map[string]map[string]bool
	functions  map[string]map[string]bool
	set        *wrapGuardSet
}

// buildWrapScope turns the scope tables into the form the scan consumes, over
// the type-checked packages of set.
func buildWrapScope(set *wrapGuardSet) wrapScope {
	scope := wrapScope{
		wholeFiles: make(map[string]map[string]bool),
		functions:  make(map[string]map[string]bool),
		set:        set,
	}
	for _, file := range inScopeWholeFiles {
		scope.wholeFiles[file] = make(map[string]bool)
	}
	for _, sf := range inScopeFunctions {
		if scope.functions[sf.file] == nil {
			scope.functions[sf.file] = make(map[string]bool)
		}
		scope.functions[sf.file][sf.fn] = true
	}
	return scope
}

var (
	wrapProductionOnce sync.Once
	wrapProductionSet  *wrapGuardSet
)

// wrapProductionGuardSet type-checks the repository's production files once for
// every check.
func wrapProductionGuardSet(t *testing.T) *wrapGuardSet {
	t.Helper()
	wrapProductionOnce.Do(func() {
		var files []wrapGuardFile
		for _, p := range identitymutationguard.ProductionGoFilesInRepo(t) {
			files = append(files, wrapGuardFile{path: p, src: identitymutationguard.ReadProductionSource(t, p)})
		}
		wrapProductionSet = loadWrapGuard(t, files, relevantPackageDirs())
	})
	require.NotNil(t, wrapProductionSet, "loading the production packages failed in an earlier test")
	return wrapProductionSet
}

// funcKey renders a function or method declaration's receiver-qualified key:
// "(*Runner).Execute", "(Level).parts" or "validatePathSecurity".
func funcKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recvType := fn.Recv.List[0].Type
	recv := receiverTypeName(recvType)
	if recv == "" {
		return fn.Name.Name
	}
	if _, isPointer := identitymutationguard.UnwrapParen(recvType).(*ast.StarExpr); isPointer {
		return "(*" + recv + ")." + fn.Name.Name
	}
	return "(" + recv + ")." + fn.Name.Name
}

// receiverTypeName returns the bare type name of a method receiver
// declaration, unwrapping a pointer and generic type parameters.
func receiverTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return receiverTypeName(e.X)
	case *ast.IndexExpr:
		return receiverTypeName(e.X)
	case *ast.IndexListExpr:
		return receiverTypeName(e.X)
	}
	return ""
}

// exceptionsForFile returns the exception type names that apply to file. An
// exception is keyed by both its declaring file and its type name, so a type
// that merely shares a name with an exception type but is declared elsewhere
// is not silently exempted.
func exceptionsForFile(refs []typeRef, file string) map[string]bool {
	exceptions := make(map[string]bool)
	for _, ref := range refs {
		if ref.file == file {
			exceptions[ref.name] = true
		}
	}
	return exceptions
}

// isConstantStringExpr reports whether expr is a constant string expression,
// resolving every name through the type checker: a string literal, a constant
// (declared in any lexical scope or a sibling file), or a "+" concatenation of
// those. A parameter or variable that shadows a constant resolves to a
// *types.Var, so it is rejected.
func isConstantStringExpr(info *types.Info, expr ast.Expr) bool {
	switch e := identitymutationguard.UnwrapParen(expr).(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.Ident:
		_, ok := info.Uses[e].(*types.Const)
		return ok
	case *ast.BinaryExpr:
		return e.Op == token.ADD && isConstantStringExpr(info, e.X) && isConstantStringExpr(info, e.Y)
	default:
		return false
	}
}

// checkFileWraps returns, for each call inside the scope of filename, the
// number of in-scope declarations scanned and the positions of fmt.Errorf,
// errors.Join and errors.New (with a non-constant argument) calls. Names are
// resolved with the type checker, so an errors.New argument counts as constant
// only where it actually resolves to a constant, whatever shadows it by name.
func checkFileWraps(t *testing.T, filename string, scope wrapScope) (scanned int, violations []string) {
	t.Helper()
	require.NotNilf(t, scope.set, "%s: the scope carries no type-checked packages", filename)
	wp := scope.set.packageForFile(filename)
	require.NotNilf(t, wp, "%s: no type-checked package for its directory; the scope is broken", filename)
	file := wp.file(filename)
	require.NotNilf(t, file, "%s: the file is in scope but was not type-checked; the scope is broken", filename)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == "fmt" || importPath == "errors"
	})
	wholeExcluded, whole := scope.wholeFiles[filename]
	funcs := scope.functions[filename]

	scan := func(decl ast.Decl, allowed bool) {
		if !allowed {
			return
		}
		scanned++
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case qualifiers[pkgIdent.Name] == "fmt" && sel.Sel.Name == "Errorf":
				violations = append(violations, fmt.Sprintf(
					"%s: fmt.Errorf outside errmsg on an in-scope path", wrapGuardFset.Position(call.Pos())))
			case qualifiers[pkgIdent.Name] == "errors" && sel.Sel.Name == "Join":
				violations = append(violations, fmt.Sprintf(
					"%s: errors.Join outside errmsg.Join on an in-scope path", wrapGuardFset.Position(call.Pos())))
			case qualifiers[pkgIdent.Name] == "errors" && sel.Sel.Name == "New":
				if len(call.Args) == 1 && isConstantStringExpr(wp.info, call.Args[0]) {
					return true
				}
				violations = append(violations, fmt.Sprintf(
					"%s: errors.New with a non-constant argument on an in-scope path", wrapGuardFset.Position(call.Pos())))
			}
			return true
		})
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			key := funcKey(d)
			switch {
			case whole:
				scan(decl, !wholeExcluded[key])
			case funcs != nil:
				scan(decl, funcs[key])
			}
		case *ast.GenDecl:
			// Package-level declarations (for example a sentinel var built
			// with errors.New) belong to the whole-file scope only.
			if whole {
				scan(decl, true)
			}
		}
	}
	return scanned, violations
}

// hasMethod reports whether the type named by tn has a method of the given
// name, declared or promoted anywhere in its package.
func hasMethod(pkg *wrapTypedPackage, tn *types.TypeName, name string) bool {
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(tn.Type()), false, pkg.pkg, name)
	_, ok := obj.(*types.Func)
	return ok
}

// declaresStructuredMessage reports whether the type named by tn has a
// StructuredMessage method with the structured contract func() errmsg.Message,
// declared or promoted. The result type is compared through the type checker,
// so a same-named method with a parameter or a different result type is not
// the structured contract.
func declaresStructuredMessage(pkg *wrapTypedPackage, tn *types.TypeName) bool {
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(tn.Type()), false, pkg.pkg, "StructuredMessage")
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig := fn.Signature()
	return sig.Params().Len() == 0 && sig.Results().Len() == 1 && isErrmsgMessage(sig.Results().At(0).Type())
}

// isErrmsgMessage reports whether t is errmsg.Message.
func isErrmsgMessage(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == wrapGuardErrmsgImport && obj.Name() == "Message"
}

// receiverTypeNameObj returns the type name of a method declaration's receiver,
// or nil when the type checker cannot resolve it.
func receiverTypeNameObj(pkg *wrapTypedPackage, fn *ast.FuncDecl) *types.TypeName {
	method, ok := pkg.info.Defs[fn.Name].(*types.Func)
	if !ok {
		return nil
	}
	recv := method.Signature().Recv()
	if recv == nil {
		return nil
	}
	t := types.Unalias(recv.Type())
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	return named.Obj()
}

// missingStructuredMessage returns the receiver type names of the Unwrap
// methods declared in the package's file filePath that have no
// StructuredMessage of the structured contract, skipping the exception names
// keyed to that file. The StructuredMessage lookup is package-wide, so a
// StructuredMessage declared in a sibling file counts.
func missingStructuredMessage(pkg *wrapTypedPackage, filePath string, exceptions map[string]bool) []string {
	file := pkg.file(filePath)
	if file == nil {
		return nil
	}
	var missing []string
	seen := map[*types.TypeName]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "Unwrap" {
			continue
		}
		tn := receiverTypeNameObj(pkg, fn)
		if tn == nil || seen[tn] {
			continue
		}
		seen[tn] = true
		if exceptions[tn.Name()] {
			continue
		}
		if declaresStructuredMessage(pkg, tn) {
			continue
		}
		missing = append(missing, tn.Name())
	}
	return missing
}

// TestInScopeWrapsUseStructuredErrors is the wrap guard: no in-scope wrap may
// use fmt.Errorf, errors.Join or a non-constant errors.New, so every wrap on
// the two records' paths goes through errmsg and keeps the cause's structure.
func TestInScopeWrapsUseStructuredErrors(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")
	scope := buildWrapScope(wrapProductionGuardSet(t))

	scannedFiles := 0
	for _, file := range files {
		if scope.wholeFiles[file] == nil && scope.functions[file] == nil {
			continue
		}
		scanned, violations := checkFileWraps(t, file, scope)
		require.NotZero(t, scanned, "file %s is in scope but no declaration was scanned; the scope is broken", file)
		scannedFiles++
		assert.Empty(t, violations,
			"an in-scope wrap must use errmsg, not fmt.Errorf/errors.Join/errors.New:\n%s",
			strings.Join(violations, "\n"))
	}
	assert.Len(t, scope.wholeFiles, len(inScopeWholeFiles),
		"every whole-file scope entry must be scanned")
	assert.Positive(t, scannedFiles, "no file was scanned; the guard is a no-op")
}

// TestInScopeErrorTypesDeclareStructuredMessage pins, through the type checker,
// that the types wrapping causes on an in-scope path declare a
// StructuredMessage of the structured contract func() errmsg.Message, both for
// the explicit list (function-unit files) and for every Unwrap-declaring type
// in a whole-file scope file.
func TestInScopeErrorTypesDeclareStructuredMessage(t *testing.T) {
	set := wrapProductionGuardSet(t)

	// Explicit list: each named type must be declared in the named file and
	// declare a properly signed StructuredMessage somewhere in its package.
	for _, ref := range inScopeErrorTypes {
		wp := set.pkgs[path.Dir(ref.file)]
		require.NotNilf(t, wp, "%s: no type-checked package for %s", ref.file, path.Dir(ref.file))
		tn, _ := wp.pkg.Scope().Lookup(ref.name).(*types.TypeName)
		assert.Truef(t, tn != nil && wrapGuardFset.Position(tn.Pos()).Filename == ref.file,
			"%s: type %s is not declared there; the scope is stale", ref.file, ref.name)
		assert.Truef(t, tn != nil && declaresStructuredMessage(wp, tn),
			"%s: type %s wraps an in-scope cause but does not declare a StructuredMessage of the structured contract", ref.file, ref.name)
	}

	// Whole-file scope: any Unwrap declaration must come with a properly
	// signed StructuredMessage anywhere in the package. An exception applies
	// only in its own declaring file, so a same-named type elsewhere is not
	// silently exempted.
	for _, file := range inScopeWholeFiles {
		wp := set.pkgs[path.Dir(file)]
		require.NotNilf(t, wp, "%s: no type-checked package for %s", file, path.Dir(file))
		exceptions := exceptionsForFile(unwrapWithoutStructuredExceptions, file)
		missing := missingStructuredMessage(wp, file, exceptions)
		assert.Empty(t, missing,
			"%s: types declaring Unwrap must also declare StructuredMessage: %v", file, missing)
	}
}

// TestScopeCatalogNamesExist pins that every file, function and excluded
// function named in the scope tables still exists in the code, so a rename or
// deletion cannot silently shrink the guarded set.
func TestScopeCatalogNamesExist(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	present := make(map[string]bool, len(files))
	for _, file := range files {
		present[file] = true
	}
	set := wrapProductionGuardSet(t)

	for _, file := range inScopeWholeFiles {
		assert.Truef(t, present[file], "in-scope file %s is missing; the scope is stale", file)
	}

	byFile := make(map[string][]string)
	for _, sf := range inScopeFunctions {
		byFile[sf.file] = append(byFile[sf.file], sf.fn)
	}

	for file, keys := range byFile {
		require.Truef(t, present[file], "scope file %s is missing; the scope is stale", file)
		wp := set.pkgs[path.Dir(file)]
		require.NotNilf(t, wp, "%s: no type-checked package for %s", file, path.Dir(file))
		parsed := wp.file(file)
		require.NotNilf(t, parsed, "%s: the file was not type-checked; the scope is broken", file)
		declared := make(map[string]bool)
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				declared[funcKey(fn)] = true
			}
		}
		for _, key := range keys {
			assert.Truef(t, declared[key], "%s: scope entry %s was not found; the scope is stale", file, key)
		}
	}

	// The exception list must keep naming types that still declare Unwrap
	// somewhere in their package, or an exception silently stops applying to
	// the type it was written for.
	for _, ref := range unwrapWithoutStructuredExceptions {
		require.Truef(t, present[ref.file], "exception file %s is missing; the exception is stale", ref.file)
		wp := set.pkgs[path.Dir(ref.file)]
		require.NotNilf(t, wp, "%s: no type-checked package for %s", ref.file, path.Dir(ref.file))
		tn, _ := wp.pkg.Scope().Lookup(ref.name).(*types.TypeName)
		assert.Truef(t, tn != nil && hasMethod(wp, tn, "Unwrap"),
			"%s: exception %s no longer declares Unwrap; the exception is stale", ref.file, ref.name)
	}
}

// TestWrapCheckRecognizesForms pins that checkFileWraps reports the forms it
// must, so it cannot pass the repository scan as a no-op.
func TestWrapCheckRecognizesForms(t *testing.T) {
	const (
		runnerFile = "internal/runner/guard_sample.go"
		configFile = "internal/runner/config/guard_sample.go"
	)
	const runnerHeader = "package runner\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\nvar _ = []any{fmt.Sprintf, errors.ErrUnsupported}\n\n"
	const configHeader = "package config\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\nvar _ = []any{fmt.Sprintf, errors.ErrUnsupported}\n\n"

	scopeFor := func(t *testing.T, file wrapGuardFile) wrapScope {
		t.Helper()
		return wrapScope{
			wholeFiles: map[string]map[string]bool{
				runnerFile: {"excluded": true},
			},
			functions: map[string]map[string]bool{
				configFile: {"scanned": true, "(*T).method": true},
			},
			set: loadWrapGuard(t, []wrapGuardFile{file}, nil),
		}
	}

	tests := []struct {
		name string
		file string
		src  string
		want int
	}{
		{
			name: "errors.Join in a whole-file scope is reported",
			file: runnerFile,
			src:  runnerHeader + "func f(a, b error) error { return errors.Join(a, b) }\n",
			want: 1,
		},
		{
			name: "fmt.Errorf in a whole-file scope is reported",
			file: runnerFile,
			src:  runnerHeader + "func f(err error) error { return fmt.Errorf(\"x: %w\", err) }\n",
			want: 1,
		},
		{
			name: "aliased fmt import is reported",
			file: runnerFile,
			src:  "package runner\n\nimport f \"fmt\"\n\nfunc g(err error) error { return f.Errorf(\"x: %v\", err) }\n",
			want: 1,
		},
		{
			name: "errors.New with a non-constant argument is reported",
			file: runnerFile,
			src:  runnerHeader + "func h(msg string) error { return errors.New(msg) }\n",
			want: 1,
		},
		{
			name: "errors.New with a string literal is accepted",
			file: runnerFile,
			src:  runnerHeader + "var errSentinel = errors.New(\"sentinel\")\n",
			want: 0,
		},
		{
			name: "errors.New with a named const is accepted",
			file: runnerFile,
			src:  runnerHeader + "const msg = \"sentinel\"\n\nvar errSentinel = errors.New(msg)\n",
			want: 0,
		},
		{
			name: "errors.New with a concatenated const is accepted",
			file: runnerFile,
			src:  runnerHeader + "const part = \"sentinel\"\n\nvar errSentinel = errors.New(part + \" rest\")\n",
			want: 0,
		},
		{
			name: "errors.New with a parameter shadowing a const is reported",
			file: runnerFile,
			src:  runnerHeader + "const msg = \"sentinel\"\n\nfunc h(msg string) error { return errors.New(msg) }\n",
			want: 1,
		},
		{
			name: "errors.New with a local variable shadowing a const is reported",
			file: runnerFile,
			src:  runnerHeader + "const msg = \"sentinel\"\n\nfunc h() error { msg := \"x\"; return errors.New(msg) }\n",
			want: 1,
		},
		{
			// The block-local const does not reach a use of the mutable
			// package variable of the same name outside the block.
			name: "a nested const does not vouch for the package variable it shadows",
			file: runnerFile,
			src:  runnerHeader + "var msg = buildMessage()\n\nfunc buildMessage() string { return \"x\" }\n\nfunc h() error { if false { const msg = \"sentinel\"; _ = msg }; return errors.New(msg) }\n",
			want: 1,
		},
		{
			name: "errors.New with a function-local const is accepted",
			file: runnerFile,
			src:  runnerHeader + "func h() error { const msg = \"sentinel\"; return errors.New(msg) }\n",
			want: 0,
		},
		{
			name: "an excluded function is not scanned",
			file: runnerFile,
			src:  runnerHeader + "func excluded(err error) error { return fmt.Errorf(\"x: %w\", err) }\n",
			want: 0,
		},
		{
			name: "an in-scope function unit is scanned",
			file: configFile,
			src:  configHeader + "func scanned(err error) error { return fmt.Errorf(\"x: %w\", err) }\n",
			want: 1,
		},
		{
			name: "an in-scope method is scanned",
			file: configFile,
			src:  configHeader + "type T struct{}\n\nfunc (*T) method(err error) error { return errors.Join(err) }\n",
			want: 1,
		},
		{
			name: "a function outside the function unit is not reported",
			file: configFile,
			src:  configHeader + "func other(err error) error { return fmt.Errorf(\"x: %w\", err) }\n",
			want: 0,
		},
		{
			name: "a package-level errors.New is checked in a whole-file scope",
			file: runnerFile,
			src:  runnerHeader + "var errSentinel = errors.New(buildMessage())\n\nfunc buildMessage() string { return \"x\" }\n",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, violations := checkFileWraps(t, tt.file, scopeFor(t, wrapGuardFile{path: tt.file, src: tt.src}))
			assert.Len(t, violations, tt.want, "violations: %v", violations)
		})
	}
}

// TestWrapCheckResolvesPackageWideConsts pins that an errors.New argument
// which is a package-level constant declared in a sibling file of the same
// package is accepted, while a same-named non-constant of the checked file is
// still reported.
func TestWrapCheckResolvesPackageWideConsts(t *testing.T) {
	const (
		file   = "internal/runner/guard_sample.go"
		header = "package runner\n\nimport \"errors\"\n\n"
	)
	scope := func(t *testing.T, source string) wrapScope {
		t.Helper()
		return wrapScope{
			wholeFiles: map[string]map[string]bool{file: {}},
			set: loadWrapGuardFromMap(t, map[string]string{
				file:                         source,
				"internal/runner/sibling.go": "package runner\n\nconst errText = \"sibling sentinel\"\n",
			}),
		}
	}

	set := scope(t, header+"var errSentinel = errors.New(errText)\n")
	_, violations := checkFileWraps(t, file, set)
	assert.Emptyf(t, violations, "a sibling-file constant must be accepted: %v", violations)

	set = scope(t, header+"func f(errText string) error { return errors.New(errText) }\n")
	_, violations = checkFileWraps(t, file, set)
	assert.Lenf(t, violations, 1, "a parameter shadowing the sibling constant must be reported: %v", violations)
}

// TestUnwrapStructuredCheckRecognizesForms pins that the whole-file Unwrap
// check reports a missing or wrongly signed StructuredMessage and honours the
// exception list.
func TestUnwrapStructuredCheckRecognizesForms(t *testing.T) {
	const (
		filePath = "internal/x/x.go"
		header   = "package x\n\ntype T struct{}\n\n"
		imp      = "package x\n\nimport \"" + wrapGuardErrmsgImport + "\"\n\ntype T struct{}\n\n"
	)
	tests := []struct {
		name       string
		src        string
		exceptions map[string]bool
		want       int
	}{
		{
			name: "a pointer receiver with Unwrap and no StructuredMessage is reported",
			src:  header + "func (e *T) Unwrap() error { return nil }\n",
			want: 1,
		},
		{
			name: "a value receiver with Unwrap and no StructuredMessage is reported",
			src:  header + "func (e T) Unwrap() error { return nil }\n",
			want: 1,
		},
		{
			name: "a type declaring both with the structured contract is accepted",
			src:  imp + "func (e *T) Unwrap() error { return nil }\n\nfunc (e *T) StructuredMessage() errmsg.Message { return errmsg.Message{} }\n",
			want: 0,
		},
		{
			name: "a StructuredMessage taking a parameter is not the structured contract",
			src:  imp + "func (e *T) Unwrap() error { return nil }\n\nfunc (e *T) StructuredMessage(int) errmsg.Message { return errmsg.Message{} }\n",
			want: 1,
		},
		{
			name: "a StructuredMessage returning another type is not the structured contract",
			src:  header + "func (e *T) Unwrap() error { return nil }\n\nfunc (e *T) StructuredMessage() string { return \"\" }\n",
			want: 1,
		},
		{
			name: "a type without Unwrap is not reported",
			src:  header + "func (e *T) Error() string { return \"\" }\n",
			want: 0,
		},
		{
			name:       "an exception type is accepted",
			src:        header + "func (e *T) Unwrap() error { return nil }\n",
			exceptions: map[string]bool{"T": true},
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := syntheticWrapPackage(t, filePath, tt.src)
			missing := missingStructuredMessage(pkg, filePath, tt.exceptions)
			assert.Len(t, missing, tt.want, "missing: %v", missing)
		})
	}
}

// TestMissingStructuredMessageResolvesPackageWideMethods pins that a
// StructuredMessage declared in a sibling file satisfies the whole-file Unwrap
// check, and that an exception applies only in its own declaring file.
func TestMissingStructuredMessageResolvesPackageWideMethods(t *testing.T) {
	const guardFile = "internal/runner/guard_sample.go"
	pkg := syntheticWrapPackageFiles(t, map[string]string{
		guardFile:                    "package runner\n\ntype T struct{}\n\nfunc (e *T) Unwrap() error { return nil }\n",
		"internal/runner/sibling.go": "package runner\n\nimport \"" + wrapGuardErrmsgImport + "\"\n\nfunc (e *T) StructuredMessage() errmsg.Message { return errmsg.Message{} }\n",
	}, guardFile)
	assert.Empty(t, missingStructuredMessage(pkg, guardFile, nil),
		"a StructuredMessage in a sibling file must satisfy the check")

	pkg = syntheticWrapPackage(t, guardFile, "package runner\n\ntype T struct{}\n\nfunc (e *T) Unwrap() error { return nil }\n")
	assert.Len(t, missingStructuredMessage(pkg, guardFile, nil), 1,
		"a package with no StructuredMessage anywhere must be reported")

	pkg = syntheticWrapPackage(t, guardFile,
		"package runner\n\ntype ExecutionError struct{}\n\nfunc (e *ExecutionError) Unwrap() error { return nil }\n")
	assert.Len(t, missingStructuredMessage(pkg, guardFile,
		exceptionsForFile(unwrapWithoutStructuredExceptions, guardFile)), 1,
		"an exception keyed to another file must not exempt a same-named type")

	const loggingFile = "internal/logging/execution_error.go"
	pkg = syntheticWrapPackage(t, loggingFile,
		"package logging\n\ntype ExecutionError struct{}\n\nfunc (e *ExecutionError) Unwrap() error { return nil }\n")
	assert.Empty(t, missingStructuredMessage(pkg, loggingFile,
		exceptionsForFile(unwrapWithoutStructuredExceptions, loggingFile)),
		"the exception must apply in its declaring file")
}
