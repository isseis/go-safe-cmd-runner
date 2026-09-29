//go:build test

package redaction

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errmsgPath is the import path of the package that owns errmsg.Role.
const errmsgPath = "github.com/isseis/go-safe-cmd-runner/internal/errmsg"

// roleGuardFset holds every file the guard parses; the source importer is
// bound to it, and sharing both means the standard library and errmsg are
// type-checked once per test binary rather than once per case.
var roleGuardFset = token.NewFileSet()

var roleGuardImporter = sync.OnceValue(func() types.ImporterFrom {
	return importer.ForCompiler(roleGuardFset, "source", nil).(types.ImporterFrom)
})

// typeCheckForRoleGuard parses and type-checks one package from srcs (file
// name to source), resolving imports from source relative to the repository
// root. Names, constants and fields are then taken from the type checker, so
// an aliased or dot import of errmsg is resolved like any other.
func typeCheckForRoleGuard(t *testing.T, srcs map[string]string) (*token.FileSet, []*ast.File, *types.Info) {
	t.Helper()
	fset := roleGuardFset
	files := make([]*ast.File, 0, len(srcs))
	for name, src := range srcs {
		f, err := parser.ParseFile(fset, name, src, 0)
		require.NoErrorf(t, err, "failed to parse %s", name)
		files = append(files, f)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	root := identitymutationguard.RepositoryRoot(t)
	conf := types.Config{Importer: sourceImporterAt{
		from: roleGuardImporter(),
		dir:  root,
	}}
	_, err := conf.Check("guardtarget", fset, files, info)
	require.NoError(t, err, "the guard takes names and types from the type checker")
	return fset, files, info
}

// sourceImporterAt resolves every import from dir, so module packages are
// found no matter which directory the parsed files claim to be in.
type sourceImporterAt struct {
	from types.ImporterFrom
	dir  string
}

func (s sourceImporterAt) Import(path string) (*types.Package, error) {
	return s.from.ImportFrom(path, s.dir, 0)
}

// isErrmsgType reports whether t is the named type errmsg.<name>.
func isErrmsgType(t types.Type, name string) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == errmsgPath && obj.Name() == name
}

// checkRoleConstruction reports every place in files that chooses an
// errmsg.Role value rather than reading one: a conversion to errmsg.Role, a
// Role constant used anywhere but a case clause or an ==/!= comparison, an
// assignment to Segment.Role, and a Segment composite literal setting Role.
// reads counts the Role constants accepted as reads, so a caller can tell the
// scan saw them.
func checkRoleConstruction(fset *token.FileSet, files []*ast.File, info *types.Info) (reads int, violations []string) {
	report := func(n ast.Node, what string) {
		violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(n.Pos()), what))
	}
	isRoleField := func(e ast.Expr) bool {
		sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
		if !ok {
			return false
		}
		s, ok := info.Selections[sel]
		return ok && s.Kind() == types.FieldVal && sel.Sel.Name == "Role" && isErrmsgType(derefType(s.Recv()), "Segment")
	}

	for _, f := range files {
		// allowed holds the constant references in read positions.
		allowed := map[ast.Node]struct{}{}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CaseClause:
				for _, e := range n.List {
					allowed[ast.Unparen(e)] = struct{}{}
				}
			case *ast.BinaryExpr:
				if n.Op == token.EQL || n.Op == token.NEQ {
					allowed[ast.Unparen(n.X)] = struct{}{}
					allowed[ast.Unparen(n.Y)] = struct{}{}
				}
			}
			return true
		})

		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if tv, ok := info.Types[n.Fun]; ok && tv.IsType() && isErrmsgType(tv.Type, "Role") {
					report(n, "conversion to errmsg.Role")
				}
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					if isRoleField(lhs) {
						report(lhs, "assignment to errmsg.Segment.Role")
					}
				}
			case *ast.IncDecStmt:
				if isRoleField(n.X) {
					report(n, "assignment to errmsg.Segment.Role")
				}
			case *ast.CompositeLit:
				if tv, ok := info.Types[n]; ok && isErrmsgType(tv.Type, "Segment") {
					for _, elt := range n.Elts {
						kv, keyed := elt.(*ast.KeyValueExpr)
						if !keyed {
							report(elt, "errmsg.Segment literal setting Role by position")
							break
						}
						if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Role" {
							report(elt, "errmsg.Segment literal setting Role")
						}
					}
				}
			case *ast.SelectorExpr, *ast.Ident:
				id := identOfRef(n)
				c, ok := info.Uses[id].(*types.Const)
				if !ok || !isErrmsgType(c.Type(), "Role") {
					return true
				}
				if _, read := allowed[n]; read {
					reads++
				} else {
					report(n, "use of errmsg."+c.Name()+" outside a case clause or comparison")
				}
				// Do not visit the selector's identifier again.
				return false
			}
			return true
		})
	}
	return reads, violations
}

// identOfRef returns the identifier naming the object a reference refers to.
func identOfRef(n ast.Node) *ast.Ident {
	if sel, ok := n.(*ast.SelectorExpr); ok {
		return sel.Sel
	}
	return n.(*ast.Ident)
}

func derefType(t types.Type) types.Type {
	if p, ok := t.(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

// TestProductionRedactionDoesNotConstructRoles fixes that redaction only reads
// the role of each segment: the role is declared where the error is built, and
// redaction never chooses one.
func TestProductionRedactionDoesNotConstructRoles(t *testing.T) {
	dir := filepath.Join(identitymutationguard.RepositoryRoot(t), "internal", "redaction")
	srcs := map[string]string{}
	for _, path := range identitymutationguard.ProductionGoFiles(t, dir) {
		srcs[filepath.Base(path)] = identitymutationguard.ReadProductionSource(t, path)
	}

	reads, violations := checkRoleConstruction(typeCheckForRoleGuard(t, srcs))

	assert.Empty(t, violations)
	// redactSegment dispatches on the role, so the scan must see those reads;
	// zero would mean it is no longer looking at the right code.
	assert.Positive(t, reads)
}

func TestRoleConstructionCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport \"github.com/isseis/go-safe-cmd-runner/internal/errmsg\"\n\n"
	tests := []struct {
		name      string
		src       string
		wantReads int
		wantViol  int
	}{
		{
			name:     "conversion to Role is reported",
			src:      header + "var _ = errmsg.Role(99)\n",
			wantViol: 1,
		},
		{
			name:     "a Role constant as a value is reported",
			src:      header + "var r = errmsg.RoleText\n",
			wantViol: 1,
		},
		{
			name:     "a Role constant passed as an argument is reported",
			src:      header + "func f(errmsg.Role) {}\n\nfunc g() { f(errmsg.RolePath) }\n",
			wantViol: 1,
		},
		{
			name:     "assignment to Segment.Role is reported",
			src:      header + "func f(s *errmsg.Segment, r errmsg.Role) { s.Role = r }\n",
			wantViol: 1,
		},
		{
			name:     "a Segment literal setting Role is reported",
			src:      header + "func f(r errmsg.Role) errmsg.Segment { return errmsg.Segment{Role: r, Text: \"x\"} }\n",
			wantViol: 1,
		},
		{
			name:     "a positional Segment literal is reported",
			src:      header + "func f(r errmsg.Role) errmsg.Segment { return errmsg.Segment{r, \"x\"} }\n",
			wantViol: 1,
		},
		{
			name:     "an aliased import is resolved",
			src:      "package x\n\nimport em \"github.com/isseis/go-safe-cmd-runner/internal/errmsg\"\n\nvar r = em.RoleIdentifier\n",
			wantViol: 1,
		},
		{
			name:      "a case clause reads the role",
			src:       header + "func f(s errmsg.Segment) bool {\n\tswitch s.Role {\n\tcase errmsg.RolePath, errmsg.RoleConstant:\n\t\treturn true\n\t}\n\treturn false\n}\n",
			wantReads: 2,
		},
		{
			name:      "a comparison reads the role",
			src:       header + "func f(s errmsg.Segment) bool { return s.Role == errmsg.RoleIdentifier || errmsg.RoleText != s.Role }\n",
			wantReads: 2,
		},
		{
			name: "a Segment literal without Role is not reported",
			src:  header + "var _ = errmsg.Segment{Text: \"x\"}\n",
		},
		{
			name: "reading Segment.Role is not reported",
			src:  header + "func f(s errmsg.Segment) errmsg.Role { return s.Role }\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reads, violations := checkRoleConstruction(typeCheckForRoleGuard(t, map[string]string{"x.go": tt.src}))
			assert.Equal(t, tt.wantReads, reads)
			assert.Len(t, violations, tt.wantViol, "violations: %v", violations)
		})
	}
}
