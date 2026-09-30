//go:build test

package runner

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
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
}

// inScopeFunctions are the functions and methods that are in scope for the guard
// but live in files that also hold out-of-scope code. runner.go, resource and
// executor are named per function for that reason.
var inScopeFunctions = []scopedFunc{
	{"internal/runner/runner.go", "(*Runner).Execute"},
	{"internal/runner/runner.go", "(*Runner).ExecuteGroup"},
	{"internal/runner/runner.go", "(*Runner).executeGroups"},
	{"internal/runner/config/errors.go", "(*ErrUndefinedVariableDetail).StructuredMessage"},
	{"internal/runner/config/errors.go", "(Level).parts"},
	{"internal/runner/config/errors.go", "(Field).parts"},
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

// expansionExcludedFunctions are the functions in expansion.go that build the
// errors of issue #1197, out of the task's scope. The whole-file scope of
// expansion.go excludes their bodies. The guard confirms each name still
// exists, so a rename cannot quietly drop the exclusion (or the function).
var expansionExcludedFunctions = []string{
	"ProcessEnvImport",
	"ProcessEnv",
	"resolveAndPrepareCommandSpec",
	"ApplyTemplateInheritance",
	"expandTemplateToSpec",
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
// from DetailMessage/ReportMessage, and the config *...Detail types are out of
// scope (issue #1197).
var unwrapWithoutStructuredExceptions = []typeRef{
	{"internal/logging/pre_execution_error.go", "PreExecutionError"},
	{"internal/logging/execution_error.go", "ExecutionError"},
}

// wrapScope is the scan's scope: whole-file units (with per-file excluded
// function keys) and function-unit entries.
type wrapScope struct {
	wholeFiles map[string]map[string]bool
	functions  map[string]map[string]bool
}

// buildWrapScope turns the scope tables into the form the scan consumes.
func buildWrapScope() wrapScope {
	scope := wrapScope{
		wholeFiles: make(map[string]map[string]bool),
		functions:  make(map[string]map[string]bool),
	}
	for _, file := range inScopeWholeFiles {
		scope.wholeFiles[file] = make(map[string]bool)
	}
	for _, fn := range expansionExcludedFunctions {
		scope.wholeFiles["internal/runner/config/expansion.go"][fn] = true
	}
	for _, sf := range inScopeFunctions {
		if scope.functions[sf.file] == nil {
			scope.functions[sf.file] = make(map[string]bool)
		}
		scope.functions[sf.file][sf.fn] = true
	}
	return scope
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

// topLevelConstNames returns the names of the package-level const identifiers
// declared in the file.
func topLevelConstNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range valueSpec.Names {
				names[name.Name] = true
			}
		}
	}
	return names
}

// declaredInFunc returns the const and non-const names declared inside fn, so
// the errors.New check can reject an argument that only shadows a package-level
// constant. A function-local const name is still a constant expression and
// stays in consts; parameters, short variable declarations, range variables
// and var declarations go to vars.
func declaredInFunc(fn *ast.FuncDecl) (consts, vars map[string]bool) {
	consts = make(map[string]bool)
	vars = make(map[string]bool)
	addParams := func(ft *ast.FuncType) {
		if ft == nil {
			return
		}
		for _, fields := range []*ast.FieldList{ft.Params, ft.Results} {
			if fields == nil {
				continue
			}
			for _, field := range fields.List {
				for _, name := range field.Names {
					vars[name.Name] = true
				}
			}
		}
	}
	addParams(fn.Type)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range valueSpec.Names {
					if node.Tok == token.CONST {
						consts[name.Name] = true
					} else {
						vars[name.Name] = true
					}
				}
			}
		case *ast.AssignStmt:
			if node.Tok == token.DEFINE {
				for _, lhs := range node.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						vars[ident.Name] = true
					}
				}
			}
		case *ast.RangeStmt:
			if node.Tok == token.DEFINE {
				if ident, ok := node.Key.(*ast.Ident); ok {
					vars[ident.Name] = true
				}
				if ident, ok := node.Value.(*ast.Ident); ok {
					vars[ident.Name] = true
				}
			}
		case *ast.FuncLit:
			addParams(node.Type)
		}
		return true
	})
	return consts, vars
}

// isConstStringExpr reports whether expr is a constant string expression
// without type information: a string literal, a name that is not shadowed by a
// non-const declaration, or a "+" concatenation of those.
func isConstStringExpr(expr ast.Expr, consts, vars map[string]bool) bool {
	switch e := identitymutationguard.UnwrapParen(expr).(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.Ident:
		return consts[e.Name] && !vars[e.Name]
	case *ast.BinaryExpr:
		return e.Op == token.ADD && isConstStringExpr(e.X, consts, vars) && isConstStringExpr(e.Y, consts, vars)
	default:
		return false
	}
}

// checkFileWraps parses src and returns, for each call inside the scope of
// filename, the number of in-scope declarations scanned and the positions of
// fmt.Errorf, errors.Join and errors.New (with a non-constant argument) calls.
func checkFileWraps(t *testing.T, filename, src string, scope wrapScope) (scanned int, violations []string) {
	t.Helper()

	fset, file := identitymutationguard.ParseSource(t, filename, src)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == "fmt" || importPath == "errors"
	})
	topConsts := topLevelConstNames(file)
	wholeExcluded, whole := scope.wholeFiles[filename]
	funcs := scope.functions[filename]

	scan := func(decl ast.Decl, allowed bool, consts, vars map[string]bool) {
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
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case qualifiers[pkg.Name] == "fmt" && sel.Sel.Name == "Errorf":
				violations = append(violations, fmt.Sprintf(
					"%s: fmt.Errorf outside errmsg on an in-scope path", fset.Position(call.Pos())))
			case qualifiers[pkg.Name] == "errors" && sel.Sel.Name == "Join":
				violations = append(violations, fmt.Sprintf(
					"%s: errors.Join outside errmsg.Join on an in-scope path", fset.Position(call.Pos())))
			case qualifiers[pkg.Name] == "errors" && sel.Sel.Name == "New":
				if len(call.Args) == 1 && isConstStringExpr(call.Args[0], consts, vars) {
					return true
				}
				violations = append(violations, fmt.Sprintf(
					"%s: errors.New with a non-constant argument on an in-scope path", fset.Position(call.Pos())))
			}
			return true
		})
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			key := funcKey(d)
			localConsts, localVars := declaredInFunc(d)
			consts := make(map[string]bool, len(topConsts)+len(localConsts))
			for name := range topConsts {
				consts[name] = true
			}
			for name := range localConsts {
				consts[name] = true
			}
			switch {
			case whole:
				scan(decl, !wholeExcluded[key], consts, localVars)
			case funcs != nil:
				scan(decl, funcs[key], consts, localVars)
			}
		case *ast.GenDecl:
			// Package-level declarations (for example a sentinel var built
			// with errors.New) belong to the whole-file scope only.
			if whole {
				scan(decl, true, topConsts, nil)
			}
		}
	}
	return scanned, violations
}

// collectReceiverMethods maps each receiver type declared in file to the set
// of method names it declares.
func collectReceiverMethods(file *ast.File) map[string]map[string]bool {
	methods := make(map[string]map[string]bool)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		recv := receiverTypeName(fn.Recv.List[0].Type)
		if recv == "" {
			continue
		}
		if methods[recv] == nil {
			methods[recv] = make(map[string]bool)
		}
		methods[recv][fn.Name.Name] = true
	}
	return methods
}

// missingStructuredMessage returns the type names that declare Unwrap but not
// StructuredMessage, skipping the exception types.
func missingStructuredMessage(methods map[string]map[string]bool, exceptions map[string]bool) []string {
	var missing []string
	for recv, names := range methods {
		if !names["Unwrap"] || names["StructuredMessage"] || exceptions[recv] {
			continue
		}
		missing = append(missing, recv)
	}
	return missing
}

// TestInScopeWrapsUseStructuredErrors is the wrap guard: no in-scope wrap may
// use fmt.Errorf, errors.Join or a non-constant errors.New, so every wrap on
// the two records' paths goes through errmsg and keeps the cause's structure.
func TestInScopeWrapsUseStructuredErrors(t *testing.T) {
	scope := buildWrapScope()
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	scannedFiles := 0
	for _, file := range files {
		if scope.wholeFiles[file] == nil && scope.functions[file] == nil {
			continue
		}
		src := identitymutationguard.ReadProductionSource(t, file)
		scanned, violations := checkFileWraps(t, file, src, scope)
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

// TestInScopeErrorTypesDeclareStructuredMessage pins that the types wrapping
// causes on an in-scope path declare StructuredMessage, both for the explicit
// list (function-unit files) and for every Unwrap-declaring type in a
// whole-file scope file.
func TestInScopeErrorTypesDeclareStructuredMessage(t *testing.T) {
	exceptions := make(map[string]bool)
	for _, ref := range unwrapWithoutStructuredExceptions {
		exceptions[ref.name] = true
	}

	// Explicit list: each named type must exist and declare StructuredMessage.
	byFile := make(map[string][]string)
	for _, ref := range inScopeErrorTypes {
		byFile[ref.file] = append(byFile[ref.file], ref.name)
	}
	for file, names := range byFile {
		src := identitymutationguard.ReadProductionSource(t, file)
		_, parsed := identitymutationguard.ParseSource(t, file, src)
		methods := collectReceiverMethods(parsed)
		for _, name := range names {
			assert.Truef(t, methods[name]["StructuredMessage"],
				"%s: type %s wraps an in-scope cause but does not declare StructuredMessage", file, name)
		}
	}

	// Whole-file scope: any Unwrap declaration must come with StructuredMessage.
	for _, file := range inScopeWholeFiles {
		src := identitymutationguard.ReadProductionSource(t, file)
		_, parsed := identitymutationguard.ParseSource(t, file, src)
		missing := missingStructuredMessage(collectReceiverMethods(parsed), exceptions)
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

	for _, file := range inScopeWholeFiles {
		assert.Truef(t, present[file], "in-scope file %s is missing; the scope is stale", file)
	}

	byFile := make(map[string][]string)
	for _, sf := range inScopeFunctions {
		byFile[sf.file] = append(byFile[sf.file], sf.fn)
	}
	byFile["internal/runner/config/expansion.go"] = append(byFile["internal/runner/config/expansion.go"], expansionExcludedFunctions...)

	for file, keys := range byFile {
		require.Truef(t, present[file], "scope file %s is missing; the scope is stale", file)
		src := identitymutationguard.ReadProductionSource(t, file)
		_, parsed := identitymutationguard.ParseSource(t, file, src)
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

	// The exception list must keep naming types that still declare Unwrap, or
	// an exception silently stops applying to the type it was written for.
	for _, ref := range unwrapWithoutStructuredExceptions {
		require.Truef(t, present[ref.file], "exception file %s is missing; the exception is stale", ref.file)
		src := identitymutationguard.ReadProductionSource(t, ref.file)
		_, parsed := identitymutationguard.ParseSource(t, ref.file, src)
		methods := collectReceiverMethods(parsed)
		assert.Truef(t, methods[ref.name]["Unwrap"],
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
	scope := wrapScope{
		wholeFiles: map[string]map[string]bool{
			runnerFile: {"excluded": true},
		},
		functions: map[string]map[string]bool{
			configFile: {"scanned": true, "(*T).method": true},
		},
	}
	const runnerHeader = "package runner\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\n"
	const configHeader = "package config\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\n"

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
			_, violations := checkFileWraps(t, tt.file, tt.src, scope)
			assert.Len(t, violations, tt.want, "violations: %v", violations)
		})
	}
}

// TestUnwrapStructuredCheckRecognizesForms pins that the whole-file Unwrap
// check reports a missing StructuredMessage and honours the exception list.
func TestUnwrapStructuredCheckRecognizesForms(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		exceptions map[string]bool
		want       int
	}{
		{
			name: "a pointer receiver with Unwrap and no StructuredMessage is reported",
			src:  "package x\n\nfunc (e *T) Unwrap() error { return nil }\n",
			want: 1,
		},
		{
			name: "a value receiver with Unwrap and no StructuredMessage is reported",
			src:  "package x\n\nfunc (e T) Unwrap() error { return nil }\n",
			want: 1,
		},
		{
			name: "a type declaring both is accepted",
			src:  "package x\n\nfunc (e *T) Unwrap() error { return nil }\n\nfunc (e *T) StructuredMessage() M { return M{} }\n",
			want: 0,
		},
		{
			name: "a type without Unwrap is not reported",
			src:  "package x\n\nfunc (e *T) Error() string { return \"\" }\n",
			want: 0,
		},
		{
			name:       "an exception type is accepted",
			src:        "package x\n\nfunc (e *T) Unwrap() error { return nil }\n",
			exceptions: map[string]bool{"T": true},
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, file := identitymutationguard.ParseSource(t, "internal/x/x.go", tt.src)
			missing := missingStructuredMessage(collectReceiverMethods(file), tt.exceptions)
			assert.Len(t, missing, tt.want, "missing: %v", missing)
		})
	}
}
