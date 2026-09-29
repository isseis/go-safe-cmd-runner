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

// isRoleField reports whether e selects the Role field of errmsg.Segment,
// directly or through an embedding struct. The field is identified by its
// object, not by the name of the receiver type.
func isRoleField(info *types.Info, e ast.Expr) bool {
	sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	s, ok := info.Selections[sel]
	if !ok || s.Kind() != types.FieldVal {
		return false
	}
	v, ok := s.Obj().(*types.Var)
	return ok && v.IsField() && v.Pkg() != nil && v.Pkg().Path() == errmsgPath && v.Name() == "Role"
}

// isRoleTyped reports whether e has type errmsg.Role.
func isRoleTyped(info *types.Info, e ast.Expr) bool {
	tv, ok := info.Types[e]
	return ok && tv.Type != nil && isErrmsgType(tv.Type, "Role")
}

// checkRoleConstruction reports every place in files that chooses an
// errmsg.Role value rather than reading one:
//   - a constant of type errmsg.Role (a Role constant, or an untyped constant
//     converted to Role) anywhere but a case clause or an ==/!= operand;
//   - a conversion to errmsg.Role or errmsg.Segment;
//   - arithmetic, ++ or -- on a Role value;
//   - an assignment to Segment.Role, taking its address, or a Segment literal
//     setting Role.
//
// reads counts the Role constants accepted in read positions, so a caller can
// tell the scan saw them.
func checkRoleConstruction(fset *token.FileSet, files []*ast.File, info *types.Info) (reads int, violations []string) {
	report := func(n ast.Node, what string) {
		violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(n.Pos()), what))
	}

	for _, f := range files {
		// allowed holds the operands in read positions.
		allowed := map[ast.Expr]struct{}{}
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
			if e, ok := n.(ast.Expr); ok {
				if tv, typed := info.Types[e]; typed && tv.Value != nil && isRoleTyped(info, e) {
					if _, read := allowed[ast.Unparen(e)]; read {
						reads++
					} else {
						report(e, "constant errmsg.Role value outside a case clause or comparison")
					}
					// One report per constant expression, not one per operand.
					return false
				}
			}
			switch n := n.(type) {
			case *ast.CallExpr:
				if tv, ok := info.Types[n.Fun]; ok && tv.IsType() {
					switch {
					case isErrmsgType(tv.Type, "Role"):
						report(n, "conversion to errmsg.Role")
					case isErrmsgType(tv.Type, "Segment"):
						report(n, "conversion to errmsg.Segment")
					}
				}
			case *ast.BinaryExpr:
				if n.Op != token.EQL && n.Op != token.NEQ && isRoleTyped(info, n) {
					report(n, "arithmetic on errmsg.Role")
				}
			case *ast.UnaryExpr:
				if n.Op == token.AND && isRoleField(info, n.X) {
					report(n, "address of errmsg.Segment.Role")
				}
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					if isRoleField(info, lhs) {
						report(lhs, "assignment to errmsg.Segment.Role")
					}
				}
			case *ast.IncDecStmt:
				if isRoleTyped(info, n.X) {
					report(n, "increment or decrement of errmsg.Role")
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
			}
			return true
		})
	}
	return reads, violations
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
			name:     "assignment through an embedding struct is reported",
			src:      header + "type w struct{ errmsg.Segment }\n\nfunc f(x *w, r errmsg.Role) { x.Role = r }\n",
			wantViol: 1,
		},
		{
			name:     "taking the address of Segment.Role is reported",
			src:      header + "func f(s *errmsg.Segment) *errmsg.Role { return &s.Role }\n",
			wantViol: 1,
		},
		{
			name:     "an untyped constant given type Role is reported",
			src:      header + "var r errmsg.Role = 2\n\nfunc f() errmsg.Role { return 3 }\n",
			wantViol: 2,
		},
		{
			name:     "arithmetic on a role is reported",
			src:      header + "func f(s errmsg.Segment) errmsg.Role { return s.Role + s.Role }\n",
			wantViol: 1,
		},
		{
			name:     "incrementing a role is reported",
			src:      header + "func f(r errmsg.Role) errmsg.Role { r++; return r }\n",
			wantViol: 1,
		},
		{
			name:     "conversion to Segment is reported",
			src:      header + "type seg struct {\n\tRole errmsg.Role\n\tText string\n}\n\nfunc f(s seg) errmsg.Segment { return errmsg.Segment(s) }\n",
			wantViol: 1,
		},
		{
			name:      "an untyped constant compared with a role is a read",
			src:       header + "func f(s errmsg.Segment) bool { return s.Role == 3 }\n",
			wantReads: 1,
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
