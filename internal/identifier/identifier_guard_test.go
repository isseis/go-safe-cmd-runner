//go:build test

package identifier

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// identifierImportPath is the package the scanned calls must resolve to. The
// guard also passes it to ResolveLocalImports so a dot import cannot make the
// calls unqualified and invisible to the scan.
const identifierImportPath = "github.com/isseis/go-safe-cmd-runner/internal/identifier"

// NewIdentifierName is the tracked constructor. Only calls are legitimate; a
// value reference can be invoked through a variable and would otherwise exempt
// free text (see TestIdentifierDeclarationCatalog_Control).
const NewIdentifierName = "NewIdentifier"

// Site shape labels.
const (
	useDirect = "direct" // the value is the attribute value or the callee's argument
	useBoxed  = "boxed"  // the value is an element of a []any-style literal
	noKey     = "(no key)"
)

var identifierNewIdentifierOptions = identitymutationguard.Options{
	Extra: []identitymutationguard.ExtraTrackedFunc{
		{ImportPath: identifierImportPath, FuncName: NewIdentifierName},
	},
}

// siteCatalogEntry is one entry of the declaration-site catalog. The tuple
// identifies the site so that a declaration moved to another call (same file,
// function and argument expression) still fails the comparison, and Count
// catches a removal that leaves other identical sites in place.
type siteCatalogEntry struct {
	File    string
	Func    string
	Context string
	Key     string
	ArgExpr string
	Use     string
	Count   int
}

// declarationSiteCatalog is the allowlist of production sites that declare an
// identifier. It is the declaration-site table of
// docs/tasks/0173_identifier_redaction_exemption/02_architecture.md section
// 3.4 (12 files, 43 sites), transcribed by hand: the guard is only as
// trustworthy as this list is independent of the scan. Context is the
// enclosing log call (or statement) as the scan renders it; Key is the
// attribute key expression when one sits at the site, and noKey for a value
// passed to a helper that names the attribute itself.
var declarationSiteCatalog = []siteCatalogEntry{
	// internal/common
	{"internal/common/notification_context.go", "(NotificationContext).LogValue", "slog.GroupValue", "NotificationContextAttrs.Group", "c.group", useDirect, 1},
	{"internal/common/notification_context.go", "(NotificationContext).LogValue", "append", "NotificationContextAttrs.Command", "c.command", useDirect, 1},
	{"internal/common/logschema.go", "(CommandResult).LogValue", "slog.GroupValue", "LogFieldName", "c.Name", useDirect, 1},
	{"internal/common/logschema.go", "(CommandResults).LogValue", "append", "LogFieldName", "cmd.Name", useDirect, 1},

	// internal/runner
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).ExecuteGroup", `slog.Info("Executing group")`, `"name"`, "groupSpec.Name", useDirect, 2},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).ExecuteGroup", `slog.Info("Group completed successfully")`, `"name"`, "groupSpec.Name", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).executeAllCommands", `slog.Info("Executing command")`, `"command"`, "runtimeCmd.Spec.Name", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).verifyGroupFiles", `slog.Info("Group file verification completed")`, `"group"`, "groupName", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).verifyGroupFiles", `slog.Error("Command dependency verification failed")`, `"group"`, "groupName", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).outputDryRunDebugInfo", `slog.Warn("Failed to record group analysis")`, `"group"`, "groupSpec.Name", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).executeCommandInGroup", `slog.Debug("Built process environment variables")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).executeCommandInGroup", `slog.Debug("Built process environment variables")`, `"group"`, "groupSpec.Name", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).executeCommandInGroup", `slog.Warn("Failed to update command debug info")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).createCommandContext", "ge.securityLogger.LogUnlimitedExecution", noKey, "cmd.Name()", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).createCommandContext", `slog.Debug("Command timeout configured")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).executeSingleCommand", "ge.securityLogger.LogTimeoutExceeded", noKey, "cmd.Name()", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).executeSingleCommand", `slog.Error("Command failed")`, `"command"`, "cmd.Name()", useBoxed, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).executeSingleCommand", "buildCommandDebugLogArgs", noKey, "cmd.Name()", useDirect, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).executeSingleCommand", `slog.Error("Command failed with non-zero exit code")`, `"command"`, "cmd.Name()", useBoxed, 1},
	{"internal/runner/group_executor.go", "(*DefaultGroupExecutor).resolveGroupWorkDir", `slog.Info("Using group workdir")`, `"group"`, "runtimeGroup.Spec.Name", useDirect, 1},
	{"internal/runner/runner.go", "(*Runner).logGroupExecutionSummary", "slog.LogAttrs", "common.GroupSummaryAttrs.Group", "groupSpec.Name", useDirect, 1},
	{"internal/runner/config/expansion.go", "resolveAndPrepareCommandSpec", `slog.Warn("Template parameter warning")`, `"command"`, "spec.Name", useDirect, 1},

	// internal/runner/base
	{"internal/runner/base/executor/executor.go", "(*DefaultExecutor).Execute", `e.Logger.Error("CRITICAL SECURITY FAILURE: privilege leak detected after command execution")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/base/executor/executor.go", "(*DefaultExecutor).executeWithUserGroup", `e.Logger.Debug("Calling WithPrivileges for user/group execution")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/base/executor/tempdir_manager.go", "(*DefaultTempDirManager).Create", `slog.Info("[DRY-RUN] Would create temporary directory")`, `"group"`, "m.groupName", useDirect, 1},
	{"internal/runner/base/executor/tempdir_manager.go", "(*DefaultTempDirManager).Create", `slog.Info("Created temporary directory")`, `"group"`, "m.groupName", useDirect, 1},
	{"internal/runner/base/privilege/unix.go", "(*UnixPrivilegeManager).WithPrivileges", `m.logger.Debug("Entering privileged operation callback")`, `"command"`, "execCtx.elevationCtx.CommandName", useDirect, 1},
	{"internal/runner/base/privilege/unix.go", "(*UnixPrivilegeManager).WithPrivileges", `m.logger.Debug("Privileged operation callback completed")`, `"command"`, "execCtx.elevationCtx.CommandName", useDirect, 1},
	{"internal/runner/base/privilege/unix.go", "(*UnixPrivilegeManager).logElevationOutcome", `m.logger.Info("Native root execution - no privilege escalation needed")`, `"command"`, "execCtx.elevationCtx.CommandName", useDirect, 1},
	{"internal/runner/base/privilege/unix.go", "(*UnixPrivilegeManager).logElevationOutcome", `m.logger.Info("Privileges elevated")`, `"command"`, "execCtx.elevationCtx.CommandName", useDirect, 1},
	{"internal/runner/base/audit/logger.go", "(*Logger).LogUserGroupExecution", "l.logger.LogAttrs", "common.UserGroupCommandFailureAttrs.CommandName", "cmd.Name()", useDirect, 1},
	{"internal/runner/base/audit/logger.go", "(*Logger).LogRiskProfile", "l.logger.LogAttrs", `"command_name"`, "entry.CommandName", useDirect, 1},

	// internal/runner/resource
	{"internal/runner/resource/normal_manager.go", "(*NormalResourceManager).ExecuteCommand", `n.logger.Warn("Failed to close verified command plan")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/resource/normal_manager.go", "(*NormalResourceManager).ExecuteCommand", `n.logger.Error("Command execution rejected due to risk level violation")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/resource/normal_manager.go", "(*NormalResourceManager).ExecuteCommand", `n.logger.Error("Command execution rejected due to risk level violation")`, `"command_path"`, "group.Name", useDirect, 1},
	{"internal/runner/resource/dryrun_manager.go", "(*DryRunResourceManager).validateRunAsIdentity", `d.logger.Warn("Dry-run run-as identity resolution failed")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/resource/dryrun_manager.go", "(*DryRunResourceManager).validateRunAsIdentity", `d.logger.Warn("Dry-run run-as identity resolution failed")`, `"group"`, "groupDisplayName", useDirect, 1},
	{"internal/runner/resource/dryrun_manager.go", "(*DryRunResourceManager).validateRunAsIdentity", `d.logger.Info("Dry-run run-as identity resolved")`, `"command"`, "cmd.Name()", useDirect, 1},
	{"internal/runner/resource/dryrun_manager.go", "(*DryRunResourceManager).validateRunAsIdentity", `d.logger.Info("Dry-run run-as identity resolved")`, `"group"`, "groupDisplayName", useDirect, 1},
	{"internal/runner/resource/dryrun_manager.go", "(*DryRunResourceManager).evaluateCommandRisk", `slog.Warn("Failed to close dry-run command plan")`, `"command"`, "cmd.Name()", useDirect, 1},

	// internal/verification
	{"internal/verification/manager.go", "(*Manager).VerifyGroupFiles", `slog.Error("Group file verification failed")`, `"group"`, "groupName", useDirect, 1},
	{"internal/verification/manager.go", "(*Manager).collectVerificationFiles", `slog.Warn("Failed to resolve command path")`, `"group"`, "input.Name", useDirect, 1},
}

// identifierDeclarationAllowlist is the fixed top-level declaration face of the
// internal/identifier package. The check renders declarations without function
// bodies, so the bodies stay review-visible instead of frozen here. Adding a
// producer (a forwarding wrapper, a void helper, a type alias, a defined type,
// a package-level initializer) needs a new top-level declaration and is
// rejected by name.
var identifierDeclarationAllowlist = map[string]string{
	"log/slog":              `"log/slog"`,
	"Identifier":            `type Identifier struct { name string }`,
	NewIdentifierName:       `func NewIdentifier(name string) Identifier`,
	"(Identifier).Name":     `func (Identifier) Name() string`,
	"(Identifier).String":   `func (Identifier) String() string`,
	"(Identifier).LogValue": `func (Identifier) LogValue() slog.Value`,
	"_":                     `var _ slog.LogValuer = Identifier{}`,
}

// TestIdentifierDeclarationCatalog pins the identifier exemption boundary:
// the production calls to identifier.NewIdentifier must match the catalog
// exactly, and the internal/identifier package must expose only the allowed
// declarations.
func TestIdentifierDeclarationCatalog(t *testing.T) {
	t.Run("declaration sites", func(t *testing.T) {
		files := identitymutationguard.ProductionGoFilesInRepo(t)
		got, err := scanDeclarationSites(t, files)
		require.NoError(t, err)
		require.NoError(t, compareDeclarationSites(got, declarationSiteCatalog))
	})

	t.Run("declaration face", func(t *testing.T) {
		require.NoError(t, checkDeclarationFace(t, "."))
	})
}

// TestIdentifierDeclarationCatalog_Control verifies the guard fails for the
// reason it claims to: one mutation at a time, on sources built for the check.
func TestIdentifierDeclarationCatalog_Control(t *testing.T) {
	t.Run("declaration sites", testDeclarationSiteControls)
	t.Run("declaration face", testDeclarationFaceControls)
}

// scannedSite is one decoded declaration site.
type scannedSite struct {
	File    string
	Func    string
	Context string
	Key     string
	ArgExpr string
	Use     string
}

// scanDeclarationSites decodes every identifier.NewIdentifier call in files.
// It reuses identitymutationguard for the raw scan and parses the file again to
// recover the argument expression, the enclosing call and the result use.
func scanDeclarationSites(t *testing.T, files []string) ([]scannedSite, error) {
	t.Helper()

	var sites []scannedSite
	var valueRefs []string
	for _, path := range files {
		src := identitymutationguard.ReadProductionSource(t, path)
		fileSites, fileRefs, err := scanOneFile(t, path, src)
		if err != nil {
			return nil, err
		}
		sites = append(sites, fileSites...)
		valueRefs = append(valueRefs, fileRefs...)
	}
	if len(valueRefs) > 0 {
		return nil, fmt.Errorf(
			"identifier.NewIdentifier must be called at the declaration site, not bound as a value (an alias can be invoked with free text): %s",
			strings.Join(valueRefs, ", "))
	}
	return sites, nil
}

// scanOneFile decodes the sites of a single already-read source file.
func scanOneFile(t *testing.T, path, src string) ([]scannedSite, []string, error) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Reject dot imports before anything else: an unqualified call would be
	// invisible to the qualified scan below. The predicate is also passed to
	// ResolveLocalImports so the import-resolution step keeps the same
	// invariant even if this pre-check is ever lost.
	if err := rejectIdentifierDotImports(path, file); err != nil {
		return nil, nil, err
	}
	imports := identitymutationguard.ResolveLocalImports(t, path, file, rejectScanDotImports)

	scannerSites, scannerRefs := identitymutationguard.RefsInSourceWithOptions(t, path, src, identifierNewIdentifierOptions)
	funcByOffset := make(map[int]string)
	for _, site := range scannerSites {
		if site.SyscallName == NewIdentifierName {
			funcByOffset[posOffset(site.Pos)] = site.FuncName
		}
	}
	var valueRefs []string
	for _, ref := range scannerRefs {
		if strings.HasSuffix(ref.Expr, "."+NewIdentifierName) {
			valueRefs = append(valueRefs, fmt.Sprintf("%s in %s", ref.Expr, ref.FuncName))
		}
	}

	var calls []*ast.CallExpr
	for _, decl := range file.Decls {
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isNewIdentifierCall(call, imports) {
				return true
			}
			calls = append(calls, call)
			return true
		})
	}

	parents := parentMap(file)
	var sites []scannedSite
	for _, call := range calls {
		offset := fset.Position(call.Pos()).Offset
		funcName, ok := funcByOffset[offset]
		if !ok {
			return nil, nil, fmt.Errorf("%s: the guard did not decode the NewIdentifier call at offset %d", path, offset)
		}
		delete(funcByOffset, offset)

		argExpr, err := singleArgSource(call, src, fset)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		context, key, use, err := classifySite(src, fset, parents, call)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %s: %w", path, argExpr, err)
		}
		sites = append(sites, scannedSite{
			File:    path,
			Func:    funcName,
			Context: context,
			Key:     key,
			ArgExpr: argExpr,
			Use:     use,
		})
	}
	if len(funcByOffset) > 0 {
		// Every scanned call must have been decoded: a call the guard cannot
		// see is a declaration the catalog cannot bound, so fail closed.
		var missed []string
		for _, funcName := range funcByOffset {
			missed = append(missed, funcName)
		}
		slices.Sort(missed)
		return nil, nil, fmt.Errorf("%s: the scanner reported NewIdentifier calls the guard did not decode in %s", path, strings.Join(missed, ", "))
	}
	return sites, valueRefs, nil
}

// rejectIdentifierDotImports fails when a file dot-imports a package the scan
// depends on seeing qualified. syscall/unix are included because the shared
// scanner tracks them as well.
func rejectIdentifierDotImports(path string, file *ast.File) error {
	for _, imp := range file.Imports {
		if imp.Name == nil || imp.Name.Name != "." {
			continue
		}
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if rejectScanDotImports(importPath) {
			return fmt.Errorf("%s: dot-import of %s makes NewIdentifier calls unqualified and defeats the declaration-site scan", path, importPath)
		}
	}
	return nil
}

func rejectScanDotImports(importPath string) bool {
	return importPath == identifierImportPath || identitymutationguard.IsTrackedImportPath(importPath)
}

// isNewIdentifierCall reports whether call is a qualified identifier.NewIdentifier
// call, resolving import aliases. A dot-imported or locally defined NewIdentifier
// is deliberately not a match.
func isNewIdentifierCall(call *ast.CallExpr, imports map[string]string) bool {
	fun := call.Fun
	for {
		paren, ok := fun.(*ast.ParenExpr)
		if !ok {
			break
		}
		fun = paren.X
	}
	selector, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return imports[qualifier.Name] == identifierImportPath && selector.Sel.Name == NewIdentifierName
}

// classifySite returns the enclosing call label, the attribute key expression
// and the result shape for one declaration site. It fails when the result is
// used in any way other than being passed on directly (or as a literal
// element), which covers identifier.NewIdentifier(x).Name() and type
// conversions such as string(identifier.NewIdentifier(x)).
func classifySite(src string, fset *token.FileSet, parents map[ast.Node]ast.Node, call *ast.CallExpr) (context, key, use string, err error) {
	key = ""
	use = useDirect
	node := ast.Node(call)

	for {
		parent, ok := parents[node]
		if !ok {
			return "", "", "", errors.New("the NewIdentifier call is not part of a consuming statement")
		}
		switch p := parent.(type) {
		case *ast.ParenExpr:
			node = p
		case *ast.CallExpr:
			index := nodeIndex(p.Args, node)
			if index < 0 {
				return "", "", "", fmt.Errorf("the NewIdentifier call is not consumed as an argument of %s", exprSource(src, fset, p))
			}
			if key == "" && index > 0 {
				key = exprSource(src, fset, p.Args[index-1])
			}
			if isSlogAttrConstructor(p) {
				node = p
				continue
			}
			if isTypeConversion(p) {
				return "", "", "", errors.New("the NewIdentifier result is converted instead of being passed on")
			}
			return callContext(src, fset, p), withNoKey(key), use, nil
		case *ast.CompositeLit:
			index := nodeIndex(p.Elts, node)
			if index < 0 {
				return "", "", "", errors.New("the NewIdentifier call appears in an unsupported composite literal position")
			}
			if node == ast.Node(call) {
				use = useBoxed
			}
			if key == "" && index > 0 {
				key = exprSource(src, fset, p.Elts[index-1])
			}
			node = p
		case *ast.AssignStmt:
			name, ok := assignedName(p, node)
			if !ok {
				return "", "", "", errors.New("the NewIdentifier result must be assigned to a single local variable")
			}
			block := enclosingBlock(parents, p)
			if block == nil {
				return "", "", "", errors.New("the NewIdentifier result is not assigned inside a block")
			}
			consumer := findEllipsisConsumer(block, p, name)
			if consumer == nil {
				return "", "", "", fmt.Errorf("no call consumes the %s argument list", name)
			}
			return callContext(src, fset, consumer), withNoKey(key), use, nil
		default:
			return "", "", "", fmt.Errorf("the NewIdentifier result is used in an unsupported way (%T)", parent)
		}
	}
}

// callContext labels the consuming call for the catalog. A string-literal
// message is appended so two calls in one function stay distinguishable.
func callContext(src string, fset *token.FileSet, call *ast.CallExpr) string {
	callee := exprSource(src, fset, call.Fun)
	if len(call.Args) > 0 {
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			return callee + "(" + lit.Value + ")"
		}
	}
	return callee
}

// slogAttrConstructors are the log/slog helpers that build an attribute from a
// key and a value. The consuming call is the log call that takes the built
// attribute, so the classification step walks past these.
var slogAttrConstructors = map[string]struct{}{
	"Any": {}, "Bool": {}, "Duration": {}, "Float64": {}, "Group": {},
	"Int": {}, "Int64": {}, "String": {}, "Time": {}, "Uint64": {},
}

func isSlogAttrConstructor(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok || qualifier.Name != "slog" {
		return false
	}
	_, ok = slogAttrConstructors[selector.Sel.Name]
	return ok
}

// builtinTypeNames are the Go type names a conversion can use as a callee.
var builtinTypeNames = map[string]struct{}{
	"string": {}, "bool": {}, "byte": {}, "rune": {},
	"int": {}, "int8": {}, "int16": {}, "int32": {}, "int64": {},
	"uint": {}, "uint8": {}, "uint16": {}, "uint32": {}, "uint64": {}, "uintptr": {},
	"float32": {}, "float64": {}, "complex64": {}, "complex128": {},
	"any": {},
}

func isTypeConversion(call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = builtinTypeNames[ident.Name]
	return ok
}

// nodeIndex returns the position of node in exprs, or -1.
func nodeIndex(exprs []ast.Expr, node ast.Node) int {
	for i, expr := range exprs {
		if ast.Node(expr) == node {
			return i
		}
	}
	return -1
}

// assignedName returns the single variable name an assignment binds, when the
// composite literal is its single right-hand side.
func assignedName(assign *ast.AssignStmt, node ast.Node) (string, bool) {
	if len(assign.Rhs) != 1 || len(assign.Lhs) != 1 {
		return "", false
	}
	if ast.Node(assign.Rhs[0]) != node {
		return "", false
	}
	ident, ok := assign.Lhs[0].(*ast.Ident)
	if !ok {
		return "", false
	}
	return ident.Name, true
}

// enclosingBlock returns the nearest block statement containing node.
func enclosingBlock(parents map[ast.Node]ast.Node, node ast.Node) *ast.BlockStmt {
	for {
		parent, ok := parents[node]
		if !ok {
			return nil
		}
		if block, ok := parent.(*ast.BlockStmt); ok {
			return block
		}
		node = parent
	}
}

// findEllipsisConsumer returns the first call after stmt that forwards name as
// an expanded argument, which is how a []any declaration site reaches its log
// call.
func findEllipsisConsumer(block *ast.BlockStmt, stmt ast.Stmt, name string) *ast.CallExpr {
	reached := false
	for _, candidate := range block.List {
		if candidate == stmt {
			reached = true
			continue
		}
		if !reached {
			continue
		}
		var consumer *ast.CallExpr
		ast.Inspect(candidate, func(n ast.Node) bool {
			if consumer != nil {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if call.Ellipsis.IsValid() && len(call.Args) > 0 {
				if ident, ok := call.Args[len(call.Args)-1].(*ast.Ident); ok && ident.Name == name {
					consumer = call
					return false
				}
			}
			return true
		})
		if consumer != nil {
			return consumer
		}
	}
	return nil
}

// singleArgSource returns the source text of the constructor's single argument.
func singleArgSource(call *ast.CallExpr, src string, fset *token.FileSet) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("identifier.NewIdentifier takes exactly one argument")
	}
	return exprSource(src, fset, call.Args[0]), nil
}

// exprSource returns the exact source text of node.
func exprSource(src string, fset *token.FileSet, node ast.Node) string {
	start := fset.Position(node.Pos()).Offset
	end := fset.Position(node.End()).Offset
	return src[start:end]
}

func withNoKey(key string) string {
	if key == "" {
		return noKey
	}
	return key
}

// posOffset turns a token.Pos from a fresh single-file FileSet into a byte
// offset. The first file of a FileSet is based at 1, and both the scanner and
// this guard parse one file into a fresh FileSet, so the offsets agree.
func posOffset(pos token.Pos) int {
	return int(pos) - 1
}

// parentMap records the parent of every node in file.
func parentMap(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}

// siteKey is the comparable identity of a catalog entry.
type siteKey struct {
	File    string
	Func    string
	Context string
	Key     string
	ArgExpr string
	Use     string
}

// compareDeclarationSites requires a bidirectional match between the scan and
// the catalog.
func compareDeclarationSites(got []scannedSite, catalog []siteCatalogEntry) error {
	gotCounts := make(map[siteKey]int, len(got))
	for _, site := range got {
		gotCounts[siteKey(site)]++
	}

	var problems []string
	wantCounts := make(map[siteKey]int, len(catalog))
	for _, entry := range catalog {
		key := siteKey{
			File:    entry.File,
			Func:    entry.Func,
			Context: entry.Context,
			Key:     entry.Key,
			ArgExpr: entry.ArgExpr,
			Use:     entry.Use,
		}
		if _, duplicate := wantCounts[key]; duplicate {
			problems = append(problems, fmt.Sprintf("catalog lists %v twice", key))
		}
		wantCounts[key] = entry.Count
	}

	for _, key := range sortedSiteKeys(wantCounts) {
		if got := gotCounts[key]; got != wantCounts[key] {
			problems = append(problems, fmt.Sprintf("declaration %v: catalog expects %d, scan found %d", key, wantCounts[key], got))
		}
		delete(gotCounts, key)
	}
	for _, key := range sortedSiteKeys(gotCounts) {
		problems = append(problems, fmt.Sprintf("declaration %v: scan found %d, not in catalog", key, gotCounts[key]))
	}
	return joinProblems(problems)
}

// sortedSiteKeys returns the map's keys in a deterministic order.
func sortedSiteKeys(counts map[siteKey]int) []siteKey {
	keys := slices.Collect(maps.Keys(counts))
	slices.SortFunc(keys, func(a, b siteKey) int {
		return strings.Compare(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
	})
	return keys
}

// checkDeclarationFace compares the top-level declarations of every production
// file in dir with the allowlist.
func checkDeclarationFace(t *testing.T, dir string) error {
	t.Helper()

	fset := token.NewFileSet()
	actual := make(map[string][]string)
	for _, path := range identitymutationguard.ProductionGoFiles(t, dir) {
		// #nosec G304 -- path comes from a directory listing of the package
		// under test, not from external input.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		file, err := parser.ParseFile(fset, path, string(data), 0)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		collectDeclarationFace(t, fset, file, actual)
	}
	return compareDeclarationFace(actual)
}

// collectDeclarationFace records each top-level declaration rendered without
// its function body, keyed by declaration name.
func collectDeclarationFace(t *testing.T, fset *token.FileSet, file *ast.File, actual map[string][]string) {
	t.Helper()

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.ImportSpec:
					importPath, err := strconv.Unquote(s.Path.Value)
					require.NoError(t, err)
					actual[importPath] = append(actual[importPath], renderNode(t, fset, s))
				case *ast.TypeSpec:
					actual[s.Name.Name] = append(actual[s.Name.Name], "type "+renderNode(t, fset, s))
				case *ast.ValueSpec:
					actual[strings.Join(identNames(s.Names), ", ")] = append(
						actual[strings.Join(identNames(s.Names), ", ")],
						d.Tok.String()+" "+renderNode(t, fset, s))
				}
			}
		case *ast.FuncDecl:
			name := declarationName(t, fset, d)
			actual[name] = append(actual[name], renderFuncSignature(t, fset, d))
		}
	}
}

func identNames(idents []*ast.Ident) []string {
	names := make([]string, 0, len(idents))
	for _, ident := range idents {
		names = append(names, ident.Name)
	}
	return names
}

// declarationName renders a function or method name for diagnostics.
func declarationName(t *testing.T, fset *token.FileSet, decl *ast.FuncDecl) string {
	t.Helper()

	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return decl.Name.Name
	}
	return "(" + renderNode(t, fset, decl.Recv.List[0].Type) + ")." + decl.Name.Name
}

// renderFuncSignature renders a function declaration's signature, receiver
// type included and body excluded.
func renderFuncSignature(t *testing.T, fset *token.FileSet, decl *ast.FuncDecl) string {
	t.Helper()

	receiver := ""
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		receiver = "(" + renderNode(t, fset, decl.Recv.List[0].Type) + ") "
	}
	typ := renderNode(t, fset, decl.Type)
	return "func " + receiver + decl.Name.Name + strings.TrimPrefix(typ, "func")
}

// renderNode renders node with gofmt and collapses whitespace so a declaration
// is one comparable line.
func renderNode(t *testing.T, fset *token.FileSet, node ast.Node) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, format.Node(&buf, fset, node))
	return strings.Join(strings.Fields(buf.String()), " ")
}

// compareDeclarationFace reports every declaration that is missing, changed or
// not on the allowlist, naming each one.
func compareDeclarationFace(actual map[string][]string) error {
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(actual)) {
		got := actual[name]
		want, ok := identifierDeclarationAllowlist[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("unexpected declaration %q: %s", name, strings.Join(got, "; ")))
			continue
		}
		if len(got) != 1 || got[0] != want {
			problems = append(problems, fmt.Sprintf("declaration %q changed: want %q, got %q", name, want, strings.Join(got, "; ")))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(identifierDeclarationAllowlist)) {
		if _, ok := actual[name]; !ok {
			problems = append(problems, fmt.Sprintf("missing declaration %q: %s", name, identifierDeclarationAllowlist[name]))
		}
	}
	return joinProblems(problems)
}

func joinProblems(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	slices.Sort(problems)
	return errors.New(strings.Join(problems, "\n"))
}

// validSiteSource is a minimal production-shaped file for the positive control.
const validSiteSource = `package synthetic

import (
	"log/slog"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/identifier"
)

func Run(cmd *command, group *groupSpec) {
	_ = syscall.Seteuid(0)
	slog.Info("Executing", slog.Any("command", identifier.NewIdentifier(cmd.Name())))
}
`

func testDeclarationSiteControls(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic.go")
	writeSyntheticFile(t, path, validSiteSource)

	catalog := []siteCatalogEntry{{
		File:    path,
		Func:    "Run",
		Context: `slog.Info("Executing")`,
		Key:     `"command"`,
		ArgExpr: "cmd.Name()",
		Use:     useDirect,
		Count:   1,
	}}

	t.Run("positive control passes and ignores the syscall site", func(t *testing.T) {
		got, err := scanDeclarationSites(t, []string{path})
		require.NoError(t, err)
		require.Len(t, got, 1, "only the NewIdentifier site may be decoded")
		require.NoError(t, compareDeclarationSites(got, catalog))
	})

	t.Run("extra declaration fails", func(t *testing.T) {
		extra := strings.Replace(validSiteSource,
			"\t_ = syscall.Seteuid(0)\n",
			"\t_ = syscall.Seteuid(0)\n\tslog.Info(\"Other\", slog.Any(\"group\", identifier.NewIdentifier(group.Name())))\n", 1)
		require.NotEqual(t, validSiteSource, extra, "the control source must actually change")
		writeSyntheticFile(t, path, extra)

		got, err := scanDeclarationSites(t, []string{path})
		require.NoError(t, err)
		err = compareDeclarationSites(got, catalog)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in catalog")
		assert.Contains(t, err.Error(), `"group"`)
	})

	t.Run("missing declaration fails", func(t *testing.T) {
		writeSyntheticFile(t, path, validSiteSource)

		got, err := scanDeclarationSites(t, []string{path})
		require.NoError(t, err)
		err = compareDeclarationSites(got, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scan found 1, not in catalog")
	})

	t.Run("moved to another log call fails", func(t *testing.T) {
		moved := strings.Replace(validSiteSource, `slog.Info("Executing"`, `slog.Warn("Executing"`, 1)
		require.NotEqual(t, validSiteSource, moved, "the control source must actually change")
		writeSyntheticFile(t, path, moved)

		got, err := scanDeclarationSites(t, []string{path})
		require.NoError(t, err)
		err = compareDeclarationSites(got, catalog)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `slog.Warn("Executing")`)
	})

	t.Run("derived result use fails", func(t *testing.T) {
		derived := `package synthetic

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/identifier"
)

func Run(entry *entrySpec) {
	slog.String("command_name", identifier.NewIdentifier(entry.CommandName).Name())
}
`
		writeSyntheticFile(t, path, derived)

		_, err := scanDeclarationSites(t, []string{path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported way")
	})

	t.Run("bound value reference fails", func(t *testing.T) {
		bound := `package synthetic

import "github.com/isseis/go-safe-cmd-runner/internal/identifier"

var makeID = identifier.NewIdentifier
`
		writeSyntheticFile(t, path, bound)

		_, err := scanDeclarationSites(t, []string{path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not bound as a value")
		assert.Contains(t, err.Error(), "identifier.NewIdentifier")
	})

	t.Run("same-named function in another package is not a site", func(t *testing.T) {
		local := `package synthetic

import "log/slog"

func NewIdentifier(s string) string { return s }

func Run(cmd *command) {
	slog.Info("Executing", slog.String("command", NewIdentifier(cmd.Name())))
}
`
		writeSyntheticFile(t, path, local)

		got, err := scanDeclarationSites(t, []string{path})
		require.NoError(t, err)
		assert.Empty(t, got, "an unqualified local NewIdentifier must not be tracked")
	})

	t.Run("dot import fails", func(t *testing.T) {
		dotImported := `package synthetic

import (
	"log/slog"

	. "github.com/isseis/go-safe-cmd-runner/internal/identifier"
)

func Run(cmd *command) {
	slog.Info("Executing", slog.Any("command", NewIdentifier(cmd.Name())))
}
`
		writeSyntheticFile(t, path, dotImported)

		_, err := scanDeclarationSites(t, []string{path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dot-import")
	})

	t.Run("aliased import is resolved", func(t *testing.T) {
		aliased := `package synthetic

import (
	"log/slog"

	id "github.com/isseis/go-safe-cmd-runner/internal/identifier"
)

func Run(cmd *command) {
	slog.Info("Executing", slog.Any("command", id.NewIdentifier(cmd.Name())))
}
`
		writeSyntheticFile(t, path, aliased)

		got, err := scanDeclarationSites(t, []string{path})
		require.NoError(t, err)
		require.Len(t, got, 1, "an aliased call must still be decoded")
		err = compareDeclarationSites(got, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in catalog")
	})

	t.Run("composite literal value position fails", func(t *testing.T) {
		valuePosition := `package synthetic

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/identifier"
)

func Run(cmd *command) {
	slog.Info("Executing", slog.Any("command", map[string]any{"id": identifier.NewIdentifier(cmd.Name())}))
}
`
		writeSyntheticFile(t, path, valuePosition)

		_, err := scanDeclarationSites(t, []string{path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported way")
	})

	t.Run("unconsumed argument list fails", func(t *testing.T) {
		unconsumed := `package synthetic

import "github.com/isseis/go-safe-cmd-runner/internal/identifier"

func Run(cmd *command) {
	args := []any{"command", identifier.NewIdentifier(cmd.Name())}
	_ = args
}
`
		writeSyntheticFile(t, path, unconsumed)

		_, err := scanDeclarationSites(t, []string{path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no call consumes")
	})

	t.Run("wrong arity fails", func(t *testing.T) {
		arity := `package synthetic

import "github.com/isseis/go-safe-cmd-runner/internal/identifier"

func Run() {
	_ = identifier.NewIdentifier()
}
`
		writeSyntheticFile(t, path, arity)

		_, err := scanDeclarationSites(t, []string{path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one argument")
	})
}

// validDeclarationSource is a synthetic package whose top-level declarations
// match identifierDeclarationAllowlist exactly.
const validDeclarationSource = `package identifier

import "log/slog"

type Identifier struct {
	name string
}

var _ slog.LogValuer = Identifier{}

func NewIdentifier(name string) Identifier {
	return Identifier{name: name}
}

func (i Identifier) Name() string {
	return i.name
}

func (i Identifier) String() string {
	return i.name
}

func (i Identifier) LogValue() slog.Value {
	return slog.StringValue(i.name)
}
`

func testDeclarationFaceControls(t *testing.T) {
	t.Helper()

	t.Run("positive control passes", func(t *testing.T) {
		dir := t.TempDir()
		writeSyntheticFile(t, filepath.Join(dir, "identifier.go"), validDeclarationSource)
		require.NoError(t, checkDeclarationFace(t, dir))
	})

	controls := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "missing declaration is rejected by name",
			source: strings.Replace(validDeclarationSource,
				"func NewIdentifier(name string) Identifier {\n\treturn Identifier{name: name}\n}\n\n", "", 1),
			expected: NewIdentifierName,
		},
		{
			name: "forwarding wrapper is rejected by name",
			source: validDeclarationSource + `
func FromString(s string) Identifier {
	return NewIdentifier(s)
}
`,
			expected: "FromString",
		},
		{
			name: "void helper is rejected by name",
			source: validDeclarationSource + `
func LogFreeText(s string) {
	slog.Info("x", "stdout", NewIdentifier(s))
}
`,
			expected: "LogFreeText",
		},
		{
			name:     "type alias is rejected by name",
			source:   validDeclarationSource + "\ntype Alias = Identifier\n",
			expected: "Alias",
		},
		{
			name:     "defined type is rejected by name",
			source:   validDeclarationSource + "\ntype Alias Identifier\n",
			expected: "Alias",
		},
		{
			name:     "package-level initializer is rejected by name",
			source:   validDeclarationSource + "\nvar defaultIdentifier = Identifier{name: \"monkey\"}\n",
			expected: "defaultIdentifier",
		},
		{
			name: "changed signature is rejected by name",
			source: strings.Replace(validDeclarationSource,
				"func NewIdentifier(name string) Identifier", "func NewIdentifier(name any) Identifier", 1),
			expected: NewIdentifierName,
		},
		{
			name: "changed struct is rejected by name",
			source: strings.Replace(validDeclarationSource,
				"type Identifier struct {\n\tname string\n}", "type Identifier struct {\n\tname string\n\tkind string\n}", 1),
			expected: "Identifier",
		},
		{
			name: "removed import is rejected by name",
			source: strings.Replace(validDeclarationSource,
				"import \"log/slog\"\n\n", "", 1),
			expected: "log/slog",
		},
		{
			name: "added import is rejected by name",
			source: strings.Replace(validDeclarationSource,
				"import \"log/slog\"", "import (\n\t\"fmt\"\n\t\"log/slog\"\n)", 1),
			expected: "fmt",
		},
	}

	for _, tt := range controls {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEqual(t, validDeclarationSource, tt.source, "the control source must actually change")
			dir := t.TempDir()
			writeSyntheticFile(t, filepath.Join(dir, "identifier.go"), tt.source)

			err := checkDeclarationFace(t, dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expected)
			assert.NotContains(t, err.Error(), "\n", "one mutation must produce one diagnosable problem")
		})
	}

	t.Run("platform-tagged production file is enumerated", func(t *testing.T) {
		dir := t.TempDir()
		writeSyntheticFile(t, filepath.Join(dir, "identifier.go"), validDeclarationSource)
		const windowsFile = `//go:build windows

package identifier

func FromWindowsName(s string) Identifier {
	return Identifier{name: s}
}
`
		writeSyntheticFile(t, filepath.Join(dir, "identifier_windows.go"), windowsFile)

		require.Contains(t, identitymutationguard.ProductionGoFiles(t, dir), filepath.Join(dir, "identifier_windows.go"),
			"a non-host platform file must still be enumerated")

		err := checkDeclarationFace(t, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "FromWindowsName")
		assert.NotContains(t, err.Error(), "\n", "one mutation must produce one diagnosable problem")
	})
}

func writeSyntheticFile(t *testing.T, path, src string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
}
