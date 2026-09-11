//go:build test

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartupPrivilegeDropOrder statically verifies the startup privilege drop
// of cmd/runner. The order it checks leaves no runtime trace when it is
// correct -- a drop that happens in the right order simply succeeds -- so an
// execution test cannot observe it; the guard reads the source instead.
//
// It asserts four things:
//
//  1. dropStartupPrivileges calls setegid before seteuid, so the effective UID
//     is never surrendered while a privileged group is still held.
//  2. dropStartupPrivileges is the first statement of main's body, and in
//     particular precedes flag.Parse, so no input is processed with the
//     privileges the process was started with. Ordering against flag.Parse
//     alone would not catch a statement inserted above the drop.
//  3. The only identity-mutation calls in this package's production code are
//     those two, and none of those functions is referenced as a value.
//  4. This package still declares exactly one init function, since every init
//     runs before main and therefore before the drop.
func TestStartupPrivilegeDropOrder(t *testing.T) {
	t.Run("setegid precedes seteuid in dropStartupPrivileges", func(t *testing.T) {
		sites, _ := identitymutationguard.FindRefs(t, ".")

		setegid := onlyCallSite(t, sites, "dropStartupPrivileges", "Setegid")
		seteuid := onlyCallSite(t, sites, "dropStartupPrivileges", "Seteuid")

		require.Equal(t, setegid.File, seteuid.File,
			"positions are only comparable within one file")
		assert.Less(t, setegid.Pos, seteuid.Pos,
			"setegid must be called before seteuid in dropStartupPrivileges")
	})

	t.Run("privilege drop precedes flag parsing in main", func(t *testing.T) {
		sites, _ := identitymutationguard.FindRefsWithOptions(t, ".", startupOrderOptions())

		drop := onlyCallSite(t, sites, "main", "dropStartupPrivileges")
		parse := onlyCallSite(t, sites, "main", "Parse")

		require.Equal(t, drop.File, parse.File,
			"positions are only comparable within one file")
		assert.Less(t, drop.Pos, parse.Pos,
			"main must drop privileges before parsing flags")
	})

	t.Run("privilege drop is main's first statement", func(t *testing.T) {
		body := mainFuncBody(t, "main.go")
		require.NotEmpty(t, body.List, "main must have a body")

		assert.True(t, callsDropStartupPrivileges(body.List[0]),
			"the privilege drop must be main's very first statement: anything placed above it runs with the privileges the process was started with, and ordering it against flag.Parse alone would not detect that")
	})

	t.Run("identity mutation is confined to dropStartupPrivileges", func(t *testing.T) {
		sites, valueRefs := identitymutationguard.FindRefs(t, ".")

		for _, ref := range valueRefs {
			t.Errorf("identity-mutation function %s referenced as a value in function %s; such a reference can be invoked later through the variable or field",
				ref.Expr, ref.FuncName)
		}

		allowed := map[string]struct{}{"Setegid": {}, "Seteuid": {}}
		for _, site := range sites {
			if site.FuncName != "dropStartupPrivileges" {
				t.Errorf("identity-mutation call %s outside dropStartupPrivileges, in function %s", site.CallExpr, site.FuncName)
				continue
			}
			if _, ok := allowed[site.SyscallName]; !ok {
				t.Errorf("unexpected identity-mutation call %s in dropStartupPrivileges", site.CallExpr)
			}
		}
		assert.Len(t, sites, len(allowed), "dropStartupPrivileges must contain exactly the allowed calls")
	})

	t.Run("package declares exactly one init function", func(t *testing.T) {
		count := 0
		for _, path := range identitymutationguard.ProductionGoFiles(t, ".") {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			require.NoErrorf(t, err, "failed to parse %s", path)
			for _, decl := range file.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Recv == nil && funcDecl.Name.Name == "init" {
					count++
				}
			}
		}
		assert.Equal(t, 1, count,
			"every init function runs before main, and so before the privilege drop; adding one widens what executes while privileged. Package-level variable initializers run just as early: they are not counted here, but the previous subtest does reject an identity-mutation call in one")
	})

	t.Run("control: the order assertions fail on reversed source", func(t *testing.T) {
		reversed := "package main\n" +
			"import (\n\t\"flag\"\n\t\"syscall\"\n)\n" +
			"func dropStartupPrivileges(uid, gid int) error {\n" +
			"\tif err := syscall.Seteuid(uid); err != nil { return err }\n" +
			"\treturn syscall.Setegid(gid)\n" +
			"}\n" +
			"func main() {\n" +
			"\tflag.Parse()\n" +
			"\t_ = dropStartupPrivileges(syscall.Getuid(), syscall.Getgid())\n" +
			"}\n"

		sites, _ := identitymutationguard.RefsInSourceWithOptions(t, "reversed.go", reversed, startupOrderOptions())

		setegid := onlyCallSite(t, sites, "dropStartupPrivileges", "Setegid")
		seteuid := onlyCallSite(t, sites, "dropStartupPrivileges", "Seteuid")
		assert.Greater(t, setegid.Pos, seteuid.Pos,
			"the scan must see the reversed order inside the function body, not merely find both calls")

		drop := onlyCallSite(t, sites, "main", "dropStartupPrivileges")
		parse := onlyCallSite(t, sites, "main", "Parse")
		assert.Greater(t, drop.Pos, parse.Pos,
			"the scan must see the reversed order inside main's body")
	})
}

// TestEnumerationEnvironmentPrecomputeOrder statically verifies where run
// settles the enumeration-completeness classification. Both neighbours matter
// and neither leaves a runtime trace on a host this build can enumerate
// exhaustively, so the guard reads the source: settling before logging is set
// up would send the warning nowhere, and settling after the first verification
// would let a denial precede the warning that explains it. Both
// verification-manager constructors are tracked, since which one runs depends
// on --dry-run.
//
// The guard compares source positions, so it sees where the call is written
// rather than whether it executes: a change that wrapped the call in a
// condition, or returned above it, would keep this green. Nothing does so
// today -- the call is a statement of run's own body.
func TestEnumerationEnvironmentPrecomputeOrder(t *testing.T) {
	t.Run("run settles the classification after logging and before verification", func(t *testing.T) {
		sites, _ := identitymutationguard.FindRefsWithOptions(t, ".", enumerationOrderOptions())

		setupLogging := onlyCallSite(t, sites, "run", "SetupLogging")
		precompute := onlyCallSite(t, sites, "run", "PrecomputeEnumerationEnvironment")
		newVerification := onlyCallSite(t, sites, "run", "NewVerificationManager")
		newDryRunVerification := onlyCallSite(t, sites, "run", "NewManagerForDryRun")

		for _, later := range []identitymutationguard.CallSite{precompute, newVerification, newDryRunVerification} {
			require.Equal(t, setupLogging.File, later.File,
				"positions are only comparable within one file")
		}
		assert.Less(t, setupLogging.Pos, precompute.Pos,
			"run must set logging up before settling the classification, or the warning reaches no destination")
		assert.Less(t, precompute.Pos, newVerification.Pos,
			"run must settle the classification before the first verification, or a denial precedes the warning that explains it")
		assert.Less(t, precompute.Pos, newDryRunVerification.Pos,
			"the same must hold on the --dry-run path, which builds its verification manager elsewhere")
	})

	t.Run("control: the order assertions fail on reordered source", func(t *testing.T) {
		reordered := "package main\n" +
			"import (\n" +
			"\t\"github.com/isseis/go-safe-cmd-runner/internal/groupmembership\"\n" +
			"\t\"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap\"\n" +
			"\t\"github.com/isseis/go-safe-cmd-runner/internal/verification\"\n" +
			")\n" +
			"func run(runID string) error {\n" +
			"\t_, _ = bootstrap.NewVerificationManager()\n" +
			"\t_, _ = verification.NewManagerForDryRun()\n" +
			"\tgroupmembership.PrecomputeEnumerationEnvironment()\n" +
			"\t_ = bootstrap.SetupLogging(bootstrap.SetupLoggingOptions{})\n" +
			"\treturn nil\n" +
			"}\n"

		sites, _ := identitymutationguard.RefsInSourceWithOptions(t, "reordered.go", reordered, enumerationOrderOptions())

		setupLogging := onlyCallSite(t, sites, "run", "SetupLogging")
		precompute := onlyCallSite(t, sites, "run", "PrecomputeEnumerationEnvironment")
		newVerification := onlyCallSite(t, sites, "run", "NewVerificationManager")

		assert.Greater(t, setupLogging.Pos, precompute.Pos,
			"the scan must see the reordered source inside run's body, not merely find the calls")
		assert.Greater(t, precompute.Pos, newVerification.Pos,
			"the scan must see the reordered source inside run's body, not merely find the calls")
		assert.Greater(t, precompute.Pos, onlyCallSite(t, sites, "run", "NewManagerForDryRun").Pos,
			"the scan must see the reordered source inside run's body, not merely find the calls")
	})
}

// mainFuncBody returns the body of the main function declared in path.
func mainFuncBody(t *testing.T, path string) *ast.BlockStmt {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoErrorf(t, err, "failed to parse %s", path)
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Recv == nil && funcDecl.Name.Name == "main" {
			return funcDecl.Body
		}
	}
	t.Fatalf("no main function declared in %s", path)
	return nil
}

// callsDropStartupPrivileges reports whether stmt contains a call to
// dropStartupPrivileges anywhere within it, so the guard accepts the call
// wrapped in an if-statement initializer as main writes it today, but not a
// call moved below some other statement.
func callsDropStartupPrivileges(stmt ast.Stmt) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "dropStartupPrivileges" {
			found = true
			return false
		}
		return true
	})
	return found
}

// startupOrderOptions tracks the two calls whose order within main matters and
// that the default syscall/unix scan does not cover: flag.Parse (qualified) and
// dropStartupPrivileges (unqualified, declared in this package).
func startupOrderOptions() identitymutationguard.Options {
	return identitymutationguard.Options{
		Extra: []identitymutationguard.ExtraTrackedFunc{
			{ImportPath: "flag", FuncName: "Parse"},
			{ImportPath: "", FuncName: "dropStartupPrivileges"},
		},
	}
}

// enumerationOrderOptions tracks the three calls of run whose order decides
// whether the enumeration-completeness warning precedes the first denial.
func enumerationOrderOptions() identitymutationguard.Options {
	return identitymutationguard.Options{
		Extra: []identitymutationguard.ExtraTrackedFunc{
			{ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap", FuncName: "SetupLogging"},
			{ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/groupmembership", FuncName: "PrecomputeEnumerationEnvironment"},
			{ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap", FuncName: "NewVerificationManager"},
			{ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/verification", FuncName: "NewManagerForDryRun"},
		},
	}
}

// identifierRedactionBootstrapPath is the import path of the package that
// exposes both SetupSlackLogging and ValidateIdentifierRedaction.
const identifierRedactionBootstrapPath = "github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"

// identifierRedactionConfigPath is the import path of the package whose
// ExpandGlobal must run after the identifier check.
const identifierRedactionConfigPath = "github.com/isseis/go-safe-cmd-runner/internal/runner/config"

// TestIdentifierRedactionWiring statically verifies that run validates
// identifiers with the redaction Config SetupSlackLogging built.
//
// The check's own tests pass a Config explicitly, so none of them fails if the
// production run passes redaction.DefaultConfig(), rebuilds a Config with
// NewConfig, or reassigns the variable between the two calls. Those are the
// mistakes that would let a rejected identifier reach a notification as
// [REDACTED], and no input/output test can tell a rebuilt Config apart from the
// production one while AddSlackHandlers passes only WithWebhookHost. The guard
// therefore reads the source: the call must come after SetupSlackLogging, its
// second argument must be the variable that received SetupSlackLogging's first
// return value, and that variable must not be rebound in between.
//
// The control subtests below run the same analysis over synthetic source so the
// guard itself is exercised: a version that stopped checking the argument or
// the rebinding would leave a control passing and fail the corresponding
// subtest.
func TestIdentifierRedactionWiring(t *testing.T) {
	t.Run("production source passes the checks", func(t *testing.T) {
		assert.Empty(t, identifierRedactionWiringProblems(t, "main.go"))
	})

	t.Run("control: correct wiring passes", func(t *testing.T) {
		assert.Empty(t, identifierRedactionWiringProblemsInSource(t, "valid.go", validIdentifierRedactionWiring))
	})

	t.Run("control: wiring mistakes are rejected for their stated reason", func(t *testing.T) {
		for name, tt := range map[string]struct {
			src         string
			wantProblem string
		}{
			"reassignment": {
				src:         reassignsIdentifierRedactionConfig,
				wantProblem: "is assigned between",
			},
			"rebuilt config": {
				src:         rebuildsIdentifierRedactionConfig,
				wantProblem: "must receive the variable SetupSlackLogging returned",
			},
			"check before setup": {
				src:         checksBeforeSetup,
				wantProblem: "must be called after SetupSlackLogging",
			},
			"inner redeclaration": {
				src:         shadowsIdentifierRedactionConfig,
				wantProblem: "is redeclared between",
			},
			"missing check": {
				src:         missingIdentifierRedactionCheck,
				wantProblem: "expected exactly one ValidateIdentifierRedaction call",
			},
			"deferred check": {
				src:         defersIdentifierRedactionCheck,
				wantProblem: "must be assigned, not deferred",
			},
			"check after expansion": {
				src:         checksAfterExpansion,
				wantProblem: "must be called before config.ExpandGlobal",
			},
		} {
			t.Run(name, func(t *testing.T) {
				problems := identifierRedactionWiringProblemsInSource(t, name+".go", tt.src)
				require.NotEmpty(t, problems)
				assert.Contains(t, strings.Join(problems, "\n"), tt.wantProblem)
			})
		}
	})
}

// identifierRedactionWiringProblems parses the file at path and returns one
// message per way run fails to pass the Config SetupSlackLogging built to
// ValidateIdentifierRedaction.
func identifierRedactionWiringProblems(t *testing.T, path string) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoErrorf(t, err, "failed to parse %s", path)
	return identifierRedactionWiringFileProblems(t, path, file)
}

// identifierRedactionWiringProblemsInSource is identifierRedactionWiringProblems
// over in-memory source, so the control subtests can run the analysis on
// deliberately broken wiring.
func identifierRedactionWiringProblemsInSource(t *testing.T, filename, src string) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	require.NoErrorf(t, err, "failed to parse %s", filename)
	return identifierRedactionWiringFileProblems(t, filename, file)
}

// identifierRedactionWiringFileProblems is the analysis shared by the
// production and control invocations. It resolves the bootstrap import so an
// aliased import still matches, and rejects a dot-import that would make the
// calls unqualified.
func identifierRedactionWiringFileProblems(t *testing.T, filename string, file *ast.File) []string {
	t.Helper()

	localToImport := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == identifierRedactionBootstrapPath || importPath == identifierRedactionConfigPath
	})
	qualifier, configQualifier := "", ""
	for local, importPath := range localToImport {
		switch importPath {
		case identifierRedactionBootstrapPath:
			qualifier = local
		case identifierRedactionConfigPath:
			configQualifier = local
		}
	}
	if qualifier == "" {
		return []string{"the file does not import the bootstrap package"}
	}

	run := funcDeclByName(file, "run")
	if run == nil {
		return []string{"no run function declared"}
	}

	var setupCalls, checkCalls []*ast.CallExpr
	setupIdx, checkIdx := -1, -1
	for i, stmt := range run.Body.List {
		ast.Inspect(stmt, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch qualifiedCallName(call, qualifier) {
			case "SetupSlackLogging":
				setupCalls = append(setupCalls, call)
				setupIdx = i
			case "ValidateIdentifierRedaction":
				checkCalls = append(checkCalls, call)
				checkIdx = i
			}
			return true
		})
	}

	var problems []string
	if len(setupCalls) != 1 {
		problems = append(problems, fmt.Sprintf("expected exactly one SetupSlackLogging call in run, found %d", len(setupCalls)))
	}
	if len(checkCalls) != 1 {
		problems = append(problems, fmt.Sprintf("expected exactly one ValidateIdentifierRedaction call in run, found %d", len(checkCalls)))
	}
	if len(problems) > 0 {
		return problems
	}

	setupCall, checkCall := setupCalls[0], checkCalls[0]

	setupVar := ""
	if assign, ok := run.Body.List[setupIdx].(*ast.AssignStmt); ok &&
		len(assign.Lhs) == 2 && len(assign.Rhs) == 1 && assign.Rhs[0] == setupCall {
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			setupVar = ident.Name
		}
	}
	if setupVar == "" {
		problems = append(problems, "SetupSlackLogging's first return value is not bound to a single variable")
	}

	if setupCall.Pos() >= checkCall.Pos() {
		problems = append(problems, "ValidateIdentifierRedaction must be called after SetupSlackLogging")
	}

	if len(checkCall.Args) != 2 {
		problems = append(problems, fmt.Sprintf("ValidateIdentifierRedaction must take 2 arguments, found %d", len(checkCall.Args)))
	} else if ident, ok := checkCall.Args[1].(*ast.Ident); !ok || ident.Name != setupVar {
		problems = append(problems, "ValidateIdentifierRedaction must receive the variable SetupSlackLogging returned")
	}

	// The check must precede the first global expansion: a name it rejects must
	// not be used by any earlier step.
	if configQualifier == "" {
		problems = append(problems, "the file does not import the config package")
	} else if firstExpansion, found := firstCallByName(run.Body, configQualifier, "ExpandGlobal"); found && firstExpansion.Pos() < checkCall.Pos() {
		problems = append(problems, "ValidateIdentifierRedaction must be called before config.ExpandGlobal")
	}

	parents := parentMap(file)
	problems = append(problems, checkCallPlacement(parents, checkCall, run.Body)...)
	if setupIdx < checkIdx {
		problems = append(problems, rebindingProblems(run.Body.List[setupIdx+1:checkIdx], setupVar)...)
	}

	return problems
}

// checkCallPlacement rejects a check call that is not assigned in run's body or
// in the initializer of a top-level if statement. A call nested in a block or
// function literal can receive a shadowing variable that happens to share the
// name, which the argument-name comparison above cannot see; a deferred or bare
// call discards the error the check exists to report.
func checkCallPlacement(parents map[ast.Node]ast.Node, call *ast.CallExpr, body *ast.BlockStmt) []string {
	parent, ok := parents[call]
	if !ok {
		return []string{"ValidateIdentifierRedaction call has no parent"}
	}
	stmt, ok := parent.(ast.Stmt)
	if !ok {
		return []string{"ValidateIdentifierRedaction must be called in a statement of run"}
	}
	assign, isAssign := stmt.(*ast.AssignStmt)
	if !isAssign {
		return []string{"ValidateIdentifierRedaction must be assigned, not deferred or called for effect"}
	}
	if len(assign.Rhs) != 1 || assign.Rhs[0] != call {
		return []string{"ValidateIdentifierRedaction must be the whole value of its assignment"}
	}
	stmtParent := parents[stmt]
	if stmtParent == body {
		return nil
	}
	// Allow the if-initializer form: if err := ...; err != nil { ... }.
	if ifStmt, ok := stmtParent.(*ast.IfStmt); ok && ifStmt.Init == stmt && parents[ifStmt] == body {
		return nil
	}
	return []string{"ValidateIdentifierRedaction must be a direct statement of run, not nested in a block or function literal"}
}

// firstCallByName returns the earliest call in body whose selector name is
// funcName and whose qualifier resolves to the given local import name.
func firstCallByName(body *ast.BlockStmt, qualifier, funcName string) (*ast.CallExpr, bool) {
	var first *ast.CallExpr
	for _, stmt := range body.List {
		ast.Inspect(stmt, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if qualifiedCallName(call, qualifier) != funcName {
				return true
			}
			if first == nil || call.Pos() < first.Pos() {
				first = call
			}
			return true
		})
	}
	return first, first != nil
}

// rebindingProblems reports every declaration or assignment of name inside the
// statements between the SetupSlackLogging assignment and the check call. Both
// assignments in the same scope and redeclarations in an inner scope count:
// this guard is written against rebinding, not against a list of statement
// shapes.
func rebindingProblems(stmts []ast.Stmt, name string) []string {
	if name == "" {
		return nil
	}

	var problems []string
	report := func(kind string) {
		problems = append(problems, fmt.Sprintf("%s is %s between SetupSlackLogging and ValidateIdentifierRedaction", name, kind))
	}

	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range node.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok && ident.Name == name {
						if node.Tok == token.DEFINE {
							report("redeclared")
						} else {
							report("assigned")
						}
					}
				}
			case *ast.ValueSpec:
				for _, ident := range node.Names {
					if ident.Name == name {
						report("redeclared")
					}
				}
			case *ast.RangeStmt:
				if ident, ok := node.Key.(*ast.Ident); ok && ident.Name == name {
					report("redeclared")
				}
				if ident, ok := node.Value.(*ast.Ident); ok && ident.Name == name {
					report("redeclared")
				}
			case *ast.FuncLit:
				for _, field := range node.Type.Params.List {
					for _, ident := range field.Names {
						if ident.Name == name {
							report("shadowed as a parameter")
						}
					}
				}
			}
			return true
		})
	}
	return problems
}

// parentMap records each AST node's parent, so the placement check can see
// through any nesting between the check call and run's body. It always returns
// true from the inspector so ast.Inspect reports the matching nil after every
// node and the stack stays balanced.
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

// qualifiedCallName returns the selector name of a call qualified by the
// resolved local name of an imported package, or "" for anything else.
func qualifiedCallName(call *ast.CallExpr, qualifier string) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || pkgIdent.Name != qualifier {
		return ""
	}
	return sel.Sel.Name
}

// funcDeclByName returns the free function with the given name, or nil.
func funcDeclByName(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Recv == nil && funcDecl.Name.Name == name {
			return funcDecl
		}
	}
	return nil
}

// validIdentifierRedactionWiring is the shape the guard accepts and the
// production code must keep: the check receives the variable that received
// SetupSlackLogging's Config, after SetupSlackLogging returns.
const validIdentifierRedactionWiring = `package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
)

func run(cfg any) error {
	redactionConfig, err := bootstrap.SetupSlackLogging(cfg)
	if err != nil {
		return err
	}
	if err := bootstrap.ValidateIdentifierRedaction(cfg, redactionConfig); err != nil {
		return err
	}
	_, _ = config.ExpandGlobal(cfg)
	return nil
}
`

const reassignsIdentifierRedactionConfig = `package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
)

func run(cfg any) error {
	redactionConfig, err := bootstrap.SetupSlackLogging(cfg)
	if err != nil {
		return err
	}
	redactionConfig = redaction.DefaultConfig()
	if err := bootstrap.ValidateIdentifierRedaction(cfg, redactionConfig); err != nil {
		return err
	}
	return nil
}
`

const rebuildsIdentifierRedactionConfig = `package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
)

func run(cfg any) error {
	if _, err := bootstrap.SetupSlackLogging(cfg); err != nil {
		return err
	}
	if err := bootstrap.ValidateIdentifierRedaction(cfg, redaction.NewConfig()); err != nil {
		return err
	}
	return nil
}
`

const checksBeforeSetup = `package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
)

func run(cfg any) error {
	if err := bootstrap.ValidateIdentifierRedaction(cfg, redaction.DefaultConfig()); err != nil {
		return err
	}
	redactionConfig, err := bootstrap.SetupSlackLogging(cfg)
	if err != nil {
		return err
	}
	_ = redactionConfig
	return nil
}
`

const shadowsIdentifierRedactionConfig = `package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
)

func run(cfg any) error {
	redactionConfig, err := bootstrap.SetupSlackLogging(cfg)
	if err != nil {
		return err
	}
	{
		redactionConfig := redaction.DefaultConfig()
		_ = redactionConfig
	}
	if err := bootstrap.ValidateIdentifierRedaction(cfg, redactionConfig); err != nil {
		return err
	}
	return nil
}
`

const missingIdentifierRedactionCheck = `package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
)

func run(cfg any) error {
	redactionConfig, err := bootstrap.SetupSlackLogging(cfg)
	if err != nil {
		return err
	}
	_, _ = config.ExpandGlobal(cfg)
	_ = redactionConfig
	return nil
}
`

const defersIdentifierRedactionCheck = `package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
)

func run(cfg any) error {
	redactionConfig, err := bootstrap.SetupSlackLogging(cfg)
	if err != nil {
		return err
	}
	defer bootstrap.ValidateIdentifierRedaction(cfg, redactionConfig)
	_, _ = config.ExpandGlobal(cfg)
	return nil
}
`

const checksAfterExpansion = `package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
)

func run(cfg any) error {
	redactionConfig, err := bootstrap.SetupSlackLogging(cfg)
	if err != nil {
		return err
	}
	_, _ = config.ExpandGlobal(cfg)
	if err := bootstrap.ValidateIdentifierRedaction(cfg, redactionConfig); err != nil {
		return err
	}
	return nil
}
`

// onlyCallSite returns the single call to funcName's tracked callee named
// calleeName, failing the test if there is not exactly one. Requiring exactly
// one guards against an order assertion that passes vacuously because the scan
// found nothing.
func onlyCallSite(t *testing.T, sites []identitymutationguard.CallSite, funcName, calleeName string) identitymutationguard.CallSite {
	t.Helper()

	var matches []identitymutationguard.CallSite
	for _, site := range sites {
		if site.FuncName == funcName && site.SyscallName == calleeName {
			matches = append(matches, site)
		}
	}
	require.Lenf(t, matches, 1, "expected exactly one call to %s in %s, got %d", calleeName, funcName, len(matches))
	return matches[0]
}
