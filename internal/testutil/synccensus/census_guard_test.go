//go:build test

// Package synccensus holds a guard test that pins the census of
// synchronization primitives declared in this repository's production code.
// Every sync/atomic declaration under internal/ and cmd/ must appear in the
// expectation table below, and every table row must correspond to a real
// declaration. Adding a lock without a matching row fails the test, which is
// the point: the addition becomes visible in review rather than silent.
//
// This file requires the "test" build tag because it imports
// identitymutationguard, which carries the same tag. Naming the package path
// without the tag -- go test ./internal/testutil/synccensus/ -- fails with
// "build constraints exclude all Go files"; run make test instead, which
// always passes -tags test.
package synccensus

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/require"
)

// scanRoots are the directories the census covers, relative to this file.
// Paths are reported relative to the repository root, so repoRootPrefix is
// stripped from every scanned path before it reaches the expectation table.
var scanRoots = []string{"../../../internal", "../../../cmd"}

const repoRootPrefix = "../../../"

// syncPrimitiveNames is the set of sync package types that provide
// concurrency control. It is deliberately wider than the types this task
// removed: a census that only listed the removed types would let a
// newly introduced sync.Map or sync.Cond through unnoticed.
var syncPrimitiveNames = map[string]struct{}{
	"Mutex":      {},
	"RWMutex":    {},
	"Once":       {},
	"OnceValue":  {},
	"OnceFunc":   {},
	"OnceValues": {},
	"WaitGroup":  {},
	"Map":        {},
	"Cond":       {},
	"Locker":     {},
}

// onceInitializerNames is the set of sync functions whose call expression is
// the only syntax a declaration such as
//
//	var fdExecSupported = sync.OnceValue(func() bool { ... })
//
// offers: the declaration has no type expression, so a scan that inspects
// types alone misses it entirely.
var onceInitializerNames = map[string]struct{}{
	"OnceValue":  {},
	"OnceFunc":   {},
	"OnceValues": {},
}

// declaration is one synchronization primitive found by the scan, identified
// by the file it lives in and the name it is declared under.
type declaration struct {
	file string
	name string
}

func (d declaration) String() string {
	return d.file + ": " + d.name
}

// expectation is one row of the census: a declaration that is expected to
// exist, with the short reason it is kept. The detailed rationale lives in
// the doc comment next to the declaration itself; the reason here is only
// what a failure message needs to show, so the two do not drift into
// duplicated documentation.
type expectation struct {
	file   string
	name   string
	reason string
}

// expectedDeclarations is the census. Every synchronization primitive left in
// production code after task 0170 appears here exactly once.
var expectedDeclarations = []expectation{
	{"internal/logging/slack_sender.go", "mu", "guards the dispatcher fields against the send worker goroutine"},
	{"internal/logging/slack_sender.go", "aggregateOnce", "Flush and Close can both reach the aggregate report from different goroutines"},
	{"internal/logging/slack_sender.go", "syncInFlight", "terminate waits here for the goroutines running sendSync"},
	{"internal/logging/slack_sender.go", "submitted", "counter updated by the send worker and by callers"},
	{"internal/logging/slack_sender.go", "enqueued", "counter updated by the send worker and by callers"},
	{"internal/logging/slack_sender.go", "sent", "counter updated by the send worker and by callers"},
	{"internal/logging/slack_sender.go", "failed", "counter updated by the send worker and by callers"},
	{"internal/logging/slack_sender.go", "dropped", "counter updated by the send worker and by callers"},
	{"internal/runner/base/output/capture.go", "mutex", "os/exec starts one goroutine per writer and both wrappers share this Capture"},
	{"internal/runner/bootstrap/logger.go", "wg", "this WaitGroup is what makes the Slack flush concurrent"},
	{"internal/logging/log_line_tracker.go", "lineCounter", "incremented from the os/exec output copy goroutine"},
	{"internal/redaction/error_collector.go", "mu", "reached from the os/exec output copy goroutine through the redacting handler"},
	{"internal/runner/base/executor/fdexec_linux.go", "fdExecSupported", "memoization of the fdexec probe, not mutual exclusion"},
	{"internal/runner/base/risktypes/runas_ident.go", "OriginalExecutionIdentity", "memoizes the identity captured before the first privilege change"},
	{"internal/testutil/handlers.go", "mu", "test log recorder that records from whatever goroutine logs"},
	{"internal/groupmembership/membership_cgo.go", "pwentMutex", "setpwent/getpwent/endpwent share a process-wide cursor in libc"},
}

// TestSyncCensusMatchesExpectation checks the scan and the expectation table
// against each other in both directions.
func TestSyncCensusMatchesExpectation(t *testing.T) {
	found := scanProductionDeclarations(t)

	expected := make(map[declaration]string, len(expectedDeclarations))
	for _, e := range expectedDeclarations {
		decl := declaration{file: e.file, name: e.name}
		_, duplicate := expected[decl]
		require.Falsef(t, duplicate, "duplicate expectation row for %s", decl)
		expected[decl] = e.reason
	}

	var undeclared []string
	for _, decl := range found {
		if _, ok := expected[decl]; !ok {
			undeclared = append(undeclared, decl.String())
		}
	}

	foundSet := make(map[declaration]struct{}, len(found))
	for _, decl := range found {
		foundSet[decl] = struct{}{}
	}
	var missing []string
	for decl, reason := range expected {
		if _, ok := foundSet[decl]; !ok {
			missing = append(missing, decl.String()+" ("+reason+")")
		}
	}
	sort.Strings(missing)

	require.Emptyf(t, undeclared,
		"found in production code but not in the expectation table:\n%s\n"+
			"Either document why the concurrent access is real and add a row, or remove the declaration.",
		strings.Join(undeclared, "\n"))
	require.Emptyf(t, missing,
		"in the expectation table but not found in production code:\n%s\n"+
			"The declaration was removed or renamed; drop or update the row.",
		strings.Join(missing, "\n"))
}

// scanProductionDeclarations parses every production Go file under scanRoots
// and returns the synchronization primitives declared in them, sorted so
// failure messages are stable.
func scanProductionDeclarations(t *testing.T) []declaration {
	t.Helper()

	var found []declaration
	for _, root := range scanRoots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				return nil
			}
			// testdata holds test inputs, not code this repository builds.
			if entry.Name() == "testdata" {
				return fs.SkipDir
			}
			for _, file := range identitymutationguard.ProductionGoFiles(t, path) {
				found = append(found, declarationsInFile(t, file)...)
			}
			return nil
		})
		require.NoErrorf(t, err, "failed to walk %s", root)
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].file != found[j].file {
			return found[i].file < found[j].file
		}
		return found[i].name < found[j].name
	})
	return found
}

// declarationsInFile parses one file and returns every synchronization
// primitive declared in it. Struct fields, var declarations (both top level
// and inside function bodies) and short variable declarations are all
// inspected: a WaitGroup declared as a local var inside a function is as much
// part of the census as a mutex field, and := is the form a future addition
// is most likely to take.
func declarationsInFile(t *testing.T, path string) []declaration {
	t.Helper()

	fset := token.NewFileSet()
	// Passing a nil source lets go/parser read the file itself, so this scan
	// never opens a path of its own.
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoErrorf(t, err, "failed to parse %s", path)

	sc := &fileScanner{
		file:              strings.TrimPrefix(filepath.ToSlash(path), repoRootPrefix),
		localToImportPath: resolveLocalImports(t, path, file),
	}
	ast.Inspect(file, func(n ast.Node) bool {
		sc.visit(n)
		return true
	})
	return sc.found
}

// fileScanner accumulates the declarations found while walking one file.
type fileScanner struct {
	file              string
	localToImportPath map[string]string
	found             []declaration
}

func (sc *fileScanner) visit(n ast.Node) {
	switch n := n.(type) {
	case *ast.Field:
		// Struct fields, and also function parameters and results: a
		// parameter typed *sync.Mutex is over-reported rather than missed,
		// which is the safe direction for a guard.
		if !sc.matchesType(n.Type) {
			return
		}
		if len(n.Names) == 0 {
			// Embedded field: the type name is the field name.
			sc.record(embeddedFieldName(n.Type))
			return
		}
		for _, name := range n.Names {
			sc.record(name.Name)
		}

	case *ast.ValueSpec:
		if n.Type != nil {
			if sc.matchesType(n.Type) {
				for _, name := range n.Names {
					sc.record(name.Name)
				}
			}
			return
		}
		sc.recordFromValues(identNames(n.Names), n.Values)

	case *ast.AssignStmt:
		if n.Tok != token.DEFINE {
			return
		}
		names := make([]string, 0, len(n.Lhs))
		for _, lhs := range n.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				names = append(names, ident.Name)
				continue
			}
			names = append(names, "?")
		}
		sc.recordFromValues(names, n.Rhs)
	}
}

// recordFromValues records the names whose initializer declares a
// synchronization primitive. When the counts line up each name is judged by
// its own initializer; otherwise (a multi-value call, say) a single match
// attributes the declaration to every name, which over-reports rather than
// misses.
func (sc *fileScanner) recordFromValues(names []string, values []ast.Expr) {
	if len(names) == len(values) {
		for i, value := range values {
			if sc.matchesInitializer(value) {
				sc.record(names[i])
			}
		}
		return
	}
	for _, value := range values {
		if !sc.matchesInitializer(value) {
			continue
		}
		for _, name := range names {
			sc.record(name)
		}
		return
	}
}

func (sc *fileScanner) record(name string) {
	sc.found = append(sc.found, declaration{file: sc.file, name: name})
}

// matchesType reports whether expr is a type expression naming a sync
// primitive or any atomic type, after peeling off pointer, slice, array and
// map layers so that *sync.Mutex and map[int]*sync.Mutex both match.
func (sc *fileScanner) matchesType(expr ast.Expr) bool {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.ArrayType:
			expr = e.Elt
		case *ast.MapType:
			expr = e.Value
		default:
			pkg, name, ok := sc.qualifiedName(expr)
			if !ok {
				return false
			}
			switch pkg {
			case "sync":
				_, isPrimitive := syncPrimitiveNames[name]
				return isPrimitive
			case "sync/atomic":
				// Every exported type of sync/atomic is a synchronization
				// primitive, and a type position holds nothing else.
				return true
			default:
				return false
			}
		}
	}
}

// matchesInitializer reports whether expr is an initializer that declares a
// synchronization primitive without naming its type: a composite literal such
// as sync.Mutex{}, or a call to one of the sync.Once* constructors.
func (sc *fileScanner) matchesInitializer(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		// &sync.Mutex{}
		return sc.matchesInitializer(e.X)
	case *ast.CompositeLit:
		return sc.matchesType(e.Type)
	case *ast.CallExpr:
		pkg, name, ok := sc.qualifiedName(e.Fun)
		if !ok || pkg != "sync" {
			return false
		}
		_, isOnce := onceInitializerNames[name]
		return isOnce
	default:
		return false
	}
}

// qualifiedName resolves a selector expression such as sync.Mutex to the
// import path of its qualifier and the selected name, so an aliased import
// still matches and a local identifier that merely shares a package's name
// does not.
func (sc *fileScanner) qualifiedName(expr ast.Expr) (importPath, name string, ok bool) {
	sel, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return "", "", false
	}
	ident, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return "", "", false
	}
	path, isImported := sc.localToImportPath[ident.Name]
	if !isImported {
		return "", "", false
	}
	return path, sel.Sel.Name, true
}

// embeddedFieldName renders the field name an embedded type contributes, e.g.
// "Mutex" for an embedded sync.Mutex.
func embeddedFieldName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.SelectorExpr:
			return e.Sel.Name
		case *ast.Ident:
			return e.Name
		default:
			return fmt.Sprintf("%T", expr)
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

// resolveLocalImports maps each import's local identifier to its import path.
// A dot-import of sync or sync/atomic would make declarations unqualified and
// invisible to this scan, so it is rejected outright.
func resolveLocalImports(t *testing.T, filename string, file *ast.File) map[string]string {
	t.Helper()

	localToImportPath := make(map[string]string)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		require.NoErrorf(t, err, "failed to unquote import path %s in %s", imp.Path.Value, filename)
		if imp.Name != nil && imp.Name.Name == "." {
			require.Falsef(t, path == "sync" || path == "sync/atomic",
				"dot-import of %s hides synchronization declarations from the census: %s", path, filename)
			continue
		}
		local := path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			local = path[idx+1:]
		}
		if imp.Name != nil {
			local = imp.Name.Name
		}
		localToImportPath[local] = path
	}
	return localToImportPath
}
