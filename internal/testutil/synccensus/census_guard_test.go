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
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scanRoots are the directories the census covers, relative to this file.
// Paths are reported relative to the repository root, so repoRootPrefix is
// stripped from every scanned path before it reaches the expectation table.
//
// Production Go files outside these two trees (scripts/verification/) are out
// of scope on purpose: the census covers what the runner binaries build.
//
// filepath.WalkDir does not follow directory symlinks, so a package reached
// only through a symlinked directory would be skipped silently. No such
// directory exists here. An unreadable directory does fail loudly, since
// WalkDir reports the error to the callback, which returns it.
var scanRoots = []string{"../../../internal", "../../../cmd"}

const repoRootPrefix = "../../../"

// declaration is one synchronization primitive found by the scan, identified
// by the file it lives in and the name it is declared under.
type declaration struct {
	file string
	name string
}

func (d declaration) String() string {
	return d.file + ": " + d.name
}

// describeCount renders one mismatch line. The counts are only spelled out
// when the name is declared more than once on either side, so the common
// one-for-one case stays readable.
func (d declaration) describeCount(delta, found, expected int, reason string) string {
	line := d.String()
	if found > 1 || expected > 1 {
		line += fmt.Sprintf(" (found %d, table has %d: %d unaccounted for)", found, expected, delta)
	}
	if reason != "" {
		line += " (" + reason + ")"
	}
	return line
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

// expectedDeclarations is the census: one row per synchronization primitive
// left in production code after task 0170. Rows are matched by multiplicity,
// so a file that declares two primitives under the same name (two functions
// each with a local "mu", say) needs two rows; one row would otherwise cover
// both and hide the second addition.
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
	foundCounts := make(map[declaration]int)
	for _, decl := range scanProductionDeclarations(t) {
		foundCounts[decl]++
	}

	expectedCounts := make(map[declaration]int, len(expectedDeclarations))
	reasons := make(map[declaration]string, len(expectedDeclarations))
	for _, e := range expectedDeclarations {
		decl := declaration{file: e.file, name: e.name}
		expectedCounts[decl]++
		reasons[decl] = e.reason
	}

	var undeclared, missing []string
	for decl, count := range foundCounts {
		if surplus := count - expectedCounts[decl]; surplus > 0 {
			undeclared = append(undeclared, decl.describeCount(surplus, count, expectedCounts[decl], ""))
		}
	}
	for decl, count := range expectedCounts {
		if shortfall := count - foundCounts[decl]; shortfall > 0 {
			missing = append(missing, decl.describeCount(shortfall, foundCounts[decl], count, reasons[decl]))
		}
	}
	sort.Strings(undeclared)
	sort.Strings(missing)

	// Both directions are reported: a rename removes one declaration and adds
	// another, and seeing only half of that costs the reader a second run.
	assert.Emptyf(t, undeclared,
		"found in production code but not in the expectation table:\n%s\n"+
			"Either document why the concurrent access is real and add a row, or remove the declaration.",
		strings.Join(undeclared, "\n"))
	assert.Emptyf(t, missing,
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
	// Passing a nil source lets go/parser read the file itself, so this scan
	// never opens a path of its own.
	return declarationsInSource(t, path, nil)
}

// declarationsInSource is declarationsInFile over source held in memory, so
// the recognized declaration forms can be tested against synthetic snippets
// rather than only against whatever the repository happens to contain today.
// A nil src makes go/parser read filename instead.
func declarationsInSource(t *testing.T, filename string, src any) []declaration {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	require.NoErrorf(t, err, "failed to parse %s", filename)

	sc := &fileScanner{
		file: strings.TrimPrefix(filepath.ToSlash(filename), repoRootPrefix),
		localToImportPath: identitymutationguard.ResolveLocalImports(t, filename, file,
			func(importPath string) bool { return importPath == "sync" || importPath == "sync/atomic" }),
		syncTypeNames: make(map[string]struct{}),
	}
	sc.collectSyncTypeNames(file)
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
	// syncTypeNames holds the file's own type names that stand for a sync or
	// sync/atomic type, so "type mutex = sync.Mutex" cannot hide a lock behind
	// a local spelling. Only same-file names are resolved: a name declared in
	// another file or package still needs type information the scan does not
	// have (see the census limitations in 02_architecture.md §4.6).
	syncTypeNames map[string]struct{}
	found         []declaration
}

// collectSyncTypeNames records the file's type names that resolve to a sync or
// sync/atomic type, following chains ("type a = sync.Mutex; type b = a") by
// iterating until nothing new is learned. Declaration order does not matter,
// which matters because Go's is not the order the scan reads them in.
func (sc *fileScanner) collectSyncTypeNames(file *ast.File) {
	for {
		grew := false
		ast.Inspect(file, func(n ast.Node) bool {
			spec, isType := n.(*ast.TypeSpec)
			if !isType {
				return true
			}
			if _, known := sc.syncTypeNames[spec.Name.Name]; known {
				return true
			}
			if sc.matchesType(spec.Type) {
				sc.syncTypeNames[spec.Name.Name] = struct{}{}
				grew = true
			}
			return true
		})
		if !grew {
			return
		}
	}
}

func (sc *fileScanner) visit(n ast.Node) {
	switch n := n.(type) {
	case *ast.StructType:
		// Only a struct field can be embedded, so this is the one place an
		// unnamed *ast.Field means "the type name is the field name". Reading
		// every *ast.Field that way would also catch unnamed parameters and
		// results -- func() []sync.Mutex has both an unnamed field and no name
		// to report -- and name the declaration after its syntax rather than
		// after anything a reader could act on.
		for _, field := range n.Fields.List {
			if !sc.matchesType(field.Type) {
				continue
			}
			if len(field.Names) == 0 {
				sc.record(embeddedFieldName(field.Type))
				continue
			}
			for _, name := range field.Names {
				sc.record(name.Name)
			}
		}

	case *ast.FuncType:
		// Named parameters and results: a parameter typed *sync.Mutex is
		// over-reported rather than missed, which is the safe direction for a
		// guard. Unnamed ones declare nothing, so there is nothing to report.
		for _, list := range []*ast.FieldList{n.Params, n.Results} {
			if list == nil {
				continue
			}
			for _, field := range list.List {
				if !sc.matchesType(field.Type) {
					continue
				}
				for _, name := range field.Names {
					sc.record(name.Name)
				}
			}
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

// matchesType reports whether expr is a type expression from sync or
// sync/atomic, after peeling off the layers a declaration can wrap one in:
// pointer, slice/array, map value, channel element, variadic element, and the
// index of a generic instantiation such as atomic.Pointer[T]. A bare
// identifier matches when the file declares it as a name for such a type
// (see collectSyncTypeNames).
//
// Every exported type of either package is a synchronization primitive, and a
// type position holds nothing else, so neither package needs a hand-kept list
// of type names to stay in step with -- a list is exactly how sync.Pool would
// slip through.
func (sc *fileScanner) matchesType(expr ast.Expr) bool {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.ArrayType:
			expr = e.Elt
		case *ast.MapType:
			expr = e.Value
		case *ast.ChanType:
			expr = e.Value
		case *ast.Ellipsis:
			expr = e.Elt
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		default:
			if ident, isIdent := expr.(*ast.Ident); isIdent {
				_, isSyncName := sc.syncTypeNames[ident.Name]
				return isSyncName
			}
			pkg, ok := sc.qualifierImportPath(expr)
			if !ok {
				return false
			}
			return pkg == "sync" || pkg == "sync/atomic"
		}
	}
}

// matchesInitializer reports whether expr is an initializer that declares a
// synchronization primitive without naming its type: a composite literal such
// as sync.Mutex{}, or a call to a sync constructor such as sync.OnceValue or
// sync.NewCond.
func (sc *fileScanner) matchesInitializer(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		// &sync.Mutex{}
		return sc.matchesInitializer(e.X)
	case *ast.CompositeLit:
		return sc.matchesType(e.Type)
	case *ast.CallExpr:
		// new(sync.Mutex), the sibling form of &sync.Mutex{}.
		if ident, isIdent := e.Fun.(*ast.Ident); isIdent && ident.Name == "new" && len(e.Args) == 1 {
			return sc.matchesType(e.Args[0])
		}
		// An explicit instantiation, sync.OnceValue[bool](f), puts the
		// selector under an index expression.
		fun := e.Fun
		for {
			switch indexed := fun.(type) {
			case *ast.IndexExpr:
				fun = indexed.X
				continue
			case *ast.IndexListExpr:
				fun = indexed.X
				continue
			}
			break
		}
		// Any call into sync counts, for the reason matchesType keeps no list
		// of type names: a function position holds nothing but package
		// members, and every function sync exports (OnceFunc, OnceValue,
		// OnceValues, NewCond) returns a synchronization primitive. A
		// hand-kept list of constructor names is exactly how sync.NewCond
		// would slip through.
		//
		// sync/atomic is deliberately not included here: its functions return
		// plain values (atomic.AddInt64 yields an int64), so a call to one
		// declares no primitive.
		pkg, ok := sc.qualifierImportPath(fun)
		return ok && pkg == "sync"
	default:
		return false
	}
}

// qualifierImportPath resolves the qualifier of a selector expression such as
// sync.Mutex to its import path, so an aliased import still matches and a local
// identifier that merely shares a package's name does not. The selected name is
// not reported: neither caller looks at it, since every member of sync and
// sync/atomic counts.
func (sc *fileScanner) qualifierImportPath(expr ast.Expr) (importPath string, ok bool) {
	sel, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return "", false
	}
	ident, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return "", false
	}
	path, isImported := sc.localToImportPath[ident.Name]
	if !isImported {
		return "", false
	}
	return path, true
}

// embeddedFieldName renders the field name an embedded type contributes, e.g.
// "Mutex" for an embedded sync.Mutex.
func embeddedFieldName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			// An embedded generic type, atomic.Pointer[int], contributes the
			// name of the generic type itself.
			expr = e.X
		case *ast.IndexListExpr:
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

// TestDeclarationsInSourceRecognizesDeclarationForms pins the syntax the scan
// must recognize. The census itself only exercises the handful of forms this
// repository happens to use today, which leaves most of the scan untested and
// lets a form the repository does not use yet -- atomic.Pointer[T], say --
// pass unnoticed the day someone adds it.
func TestDeclarationsInSourceRecognizesDeclarationForms(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "struct field",
			body: "type s struct{ mu sync.Mutex }",
			want: []string{"mu"},
		},
		{
			name: "embedded field takes the type name",
			body: "type s struct{ sync.Mutex }",
			want: []string{"Mutex"},
		},
		{
			name: "pointer slice and map wrappers are peeled",
			body: "type s struct{\na *sync.Mutex\nb []sync.RWMutex\nc map[string]*sync.Mutex\n}",
			want: []string{"a", "b", "c"},
		},
		{
			name: "channel and variadic element types are peeled",
			body: "type s struct{ ch chan sync.Mutex }\nfunc f(mus ...sync.Mutex) {}",
			want: []string{"ch", "mus"},
		},
		{
			name: "generic instantiation is peeled",
			body: "type s struct{\np atomic.Pointer[int]\nv atomic.Value\n}",
			want: []string{"p", "v"},
		},
		{
			name: "sync.Pool is a primitive like any other",
			body: "var pool sync.Pool",
			want: []string{"pool"},
		},
		{
			name: "top level var",
			body: "var mu sync.Mutex",
			want: []string{"mu"},
		},
		{
			name: "function local var",
			body: "func f() {\nvar wg sync.WaitGroup\n_ = wg\n}",
			want: []string{"wg"},
		},
		{
			name: "two declarations sharing a name are recorded separately",
			body: "func f() {\nvar mu sync.Mutex\n_ = mu\n}\n\nfunc g() {\nvar mu sync.Mutex\n_ = mu\n}",
			want: []string{"mu", "mu"},
		},
		{
			name: "short variable declaration",
			body: "func f() {\nmu := sync.Mutex{}\n_ = mu\n}",
			want: []string{"mu"},
		},
		{
			name: "address-of composite literal",
			body: "var mu = &sync.Mutex{}",
			want: []string{"mu"},
		},
		{
			name: "new call",
			body: "var mu = new(sync.RWMutex)",
			want: []string{"mu"},
		},
		{
			name: "type-less OnceValue initializer",
			body: "var probe = sync.OnceValue(func() bool { return true })",
			want: []string{"probe"},
		},
		{
			name: "explicitly instantiated OnceValue initializer",
			body: "var probe = sync.OnceValue[bool](func() bool { return true })",
			want: []string{"probe"},
		},
		{
			name: "NewCond initializer",
			body: "var mu sync.Mutex\n\nvar cond = sync.NewCond(&mu)",
			want: []string{"mu", "cond"},
		},
		{
			name: "NewCond short variable declaration",
			body: "func f() {\nmu := sync.Mutex{}\ncond := sync.NewCond(&mu)\n_ = cond\n}",
			want: []string{"mu", "cond"},
		},
		{
			name: "a local name for a sync type does not hide a declaration",
			body: "type mutex = sync.Mutex\n\nvar mu mutex",
			want: []string{"mu"},
		},
		{
			name: "a chain of local names is followed",
			body: "type mutex = sync.Mutex\n\ntype guard = mutex\n\nvar mu guard",
			want: []string{"mu"},
		},
		{
			name: "a local name declared after its use still resolves",
			body: "var mu mutex\n\ntype mutex = sync.Mutex",
			want: []string{"mu"},
		},
		{
			name: "a defined type over a sync type is a primitive",
			body: "type mutex sync.Mutex\n\nvar mu mutex",
			want: []string{"mu"},
		},
		{
			name: "an unnamed result type reports no declaration",
			body: "func newMus() []sync.Mutex { return nil }",
			want: nil,
		},
		{
			name: "an unnamed function parameter reports no declaration",
			body: "type s struct{ fn func(*sync.Mutex) }",
			want: nil,
		},
		{
			name: "a named function parameter is reported",
			body: "func f(mu *sync.Mutex) { _ = mu }",
			want: []string{"mu"},
		},
		{
			name: "a plain integer used with atomic operations is out of scope",
			body: "var counter int64\n\nfunc f() { atomic.AddInt64(&counter, 1) }",
			want: nil,
		},
		{
			name: "a qualifier that is not an import does not match",
			body: "type notSync struct{ Mutex int }\n\nvar shadow notSync\n\nvar mu = shadow.Mutex",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "package p\n\nimport (\n\"sync\"\n\"sync/atomic\"\n)\n\nvar _, _ = sync.Mutex{}, atomic.Int64{}\n\n" + tt.body + "\n"
			var got []string
			for _, decl := range declarationsInSource(t, "synthetic.go", src) {
				got = append(got, decl.name)
			}
			// The header declares one sync and one atomic value so that both
			// imports are used; those two are anonymous (_) and are dropped
			// here so each case asserts on its own body alone.
			got = withoutBlanks(got)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDeclarationsInSourceResolvesAliasedImports checks that the qualifier is
// resolved through the file's imports rather than matched by spelling, in both
// directions: an alias for sync still matches, and another package aliased to
// "sync" does not.
func TestDeclarationsInSourceResolvesAliasedImports(t *testing.T) {
	aliased := declarationsInSource(t, "aliased.go",
		"package p\n\nimport s \"sync\"\n\ntype t struct{ mu s.Mutex }\n")
	assert.Equal(t, []declaration{{file: "aliased.go", name: "mu"}}, aliased)

	decoy := declarationsInSource(t, "decoy.go",
		"package p\n\nimport sync \"errors\"\n\nvar _ = sync.New\n\ntype t struct{ mu sync.Mutex }\n")
	assert.Empty(t, decoy, "a package aliased to sync must not be mistaken for sync")
}

func withoutBlanks(names []string) []string {
	var kept []string
	for _, name := range names {
		if name != "_" {
			kept = append(kept, name)
		}
	}
	return kept
}
