//go:build test

package runner

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configErrorTypeNames returns the names of the types declared in pkg that have
// an Error() string method. A type alias is skipped; it is checked as the type
// it points at.
func configErrorTypeNames(pkg *wrapTypedPackage) []string {
	var names []string
	for _, name := range pkg.pkg.Scope().Names() {
		tn, ok := pkg.pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		if declaresErrorString(pkg, tn) {
			names = append(names, name)
		}
	}
	return names
}

// configErrorTypesMissingStructured returns the names among configErrorTypeNames
// that have no StructuredMessage of the structured contract.
func configErrorTypesMissingStructured(pkg *wrapTypedPackage) []string {
	var missing []string
	for _, name := range configErrorTypeNames(pkg) {
		tn, _ := pkg.pkg.Scope().Lookup(name).(*types.TypeName)
		if !declaresStructuredMessage(pkg, tn) {
			missing = append(missing, name)
		}
	}
	return missing
}

// declaresErrorString reports whether the type named by tn declares or promotes
// a method func() string named Error.
func declaresErrorString(pkg *wrapTypedPackage, tn *types.TypeName) bool {
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(tn.Type()), false, pkg.pkg, "Error")
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig := fn.Signature()
	return sig.Params().Len() == 0 && sig.Results().Len() == 1 &&
		types.Identical(sig.Results().At(0).Type(), types.Typ[types.String])
}

// configLevelFieldTypesWrong returns "<Type>.<Field>" for every error type field
// that names a level or field slot but is not typed config.Level/config.Field.
// The name rule is deliberately narrow: the exact names "Level" and "Field",
// and the "...Level" rule already in use (EnvImportLevel, VarsLevel). A raw
// string field whose name merely ends in "Field" (an unvalidated name that
// belongs to Text) is not a level/field slot and is not rejected.
func configLevelFieldTypesWrong(pkg *wrapTypedPackage) []string {
	levelType, fieldType, present := configRoleTypes(pkg)
	if !present {
		return nil
	}
	if levelType == nil || fieldType == nil {
		return []string{"config.Level/config.Field not found; the guard is broken"}
	}

	var wrong []string
	for _, name := range pkg.pkg.Scope().Names() {
		tn, ok := pkg.pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() || !declaresErrorString(pkg, tn) {
			continue
		}
		st, ok := tn.Type().Underlying().(*types.Struct)
		if !ok {
			continue
		}
		for field := range st.Fields() {
			switch {
			case field.Name() == "Field":
				if !types.Identical(field.Type(), fieldType) {
					wrong = append(wrong, name+"."+field.Name())
				}
			case field.Name() == "Level", strings.HasSuffix(field.Name(), "Level"):
				if !types.Identical(field.Type(), levelType) {
					wrong = append(wrong, name+"."+field.Name())
				}
			}
		}
	}
	return wrong
}

// configImportPath is the config package's import path, where Level and Field
// are declared.
const configImportPath = wrapGuardModulePath + "/internal/runner/config"

// configRoleTypes returns the config.Level and config.Field types as seen from
// pkg, and whether pkg can use them at all. pkg is config itself, or a package
// that imports it; a package that does not import config has no Level/Field
// values to flatten, so there is nothing for the guard to check.
func configRoleTypes(pkg *wrapTypedPackage) (levelType, fieldType types.Type, present bool) {
	scope := pkg.pkg
	if pkg.pkg.Path() != configImportPath {
		scope = nil
		for _, imp := range pkg.pkg.Imports() {
			if imp.Path() == configImportPath {
				scope = imp
				break
			}
		}
	}
	if scope == nil {
		return nil, nil, false
	}
	if tn, _ := scope.Scope().Lookup("Level").(*types.TypeName); tn != nil {
		levelType = tn.Type()
	}
	if tn, _ := scope.Scope().Lookup("Field").(*types.TypeName); tn != nil {
		fieldType = tn.Type()
	}
	return levelType, fieldType, true
}

// configFlatteningAllowedFile is where the one allowed flattening lives: the
// location string validateVariableName passes to
// variable.ValidateVariableNameForScope.
const configFlatteningAllowedFile = "internal/runner/config/validation.go"

// configFlattenedLevelFieldValues reports every position in pkg where a
// config.Level or config.Field value is flattened to a string and loses its
// role: an explicit .String() call, or a fmt formatting call that renders it
// (with %s/%v/%q, or the no-format Print/Sprint family). The only allowed
// position is the location string expression validateVariableName passes to
// variable.ValidateVariableNameForScope.
func configFlattenedLevelFieldValues(pkg *wrapTypedPackage) []string {
	levelType, fieldType, present := configRoleTypes(pkg)
	if !present {
		return nil
	}
	if levelType == nil || fieldType == nil {
		return []string{"config.Level/config.Field not found; the guard is broken"}
	}
	// A role value is a Level or a Field, seen directly or through any number
	// of pointers.
	isRoleValue := func(t types.Type) bool {
		if t == nil {
			return false
		}
		t = types.Unalias(t)
		for {
			ptr, ok := t.(*types.Pointer)
			if !ok {
				break
			}
			t = types.Unalias(ptr.Elem())
		}
		return types.Identical(t, levelType) || types.Identical(t, fieldType)
	}
	// isRoleExpr reports whether expr's type is a Level or a Field, however it
	// is parenthesized.
	isRoleExpr := func(e ast.Expr) bool {
		return isRoleValue(pkg.info.Types[identitymutationguard.UnwrapParen(e)].Type)
	}
	allowed := configFlatteningAllowedNodes(pkg)

	var violations []string
	for _, file := range pkg.files {
		ast.Inspect(file.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// A fmt formatting call flattens the arguments it renders with a
			// flattening verb.
			fmtArgs, fmtName, ok := fmtFlatteningCall(pkg, call)
			if ok {
				if allowed[call] {
					return true
				}
				if slices.ContainsFunc(fmtArgs, isRoleExpr) {
					violations = append(violations, fmt.Sprintf(
						"%s: a config.Level/config.Field is rendered through fmt.%s; use its parts() instead",
						wrapGuardFset.Position(call.Pos()), fmtName))
				}
				return true
			}

			// An explicit .String() call on a Level/Field value.
			sel, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "String" {
				return true
			}
			if isRoleExpr(sel.X) {
				violations = append(violations, fmt.Sprintf(
					"%s: a config.Level/config.Field is flattened with .String(); use its parts() instead",
					wrapGuardFset.Position(call.Pos())))
			}
			return true
		})
	}
	return violations
}

// configFlatteningAllowedNodes returns the identities of the expressions in the
// config package that may flatten Level/Field values: the location argument of
// variable.ValidateVariableNameForScope, whether written directly as the
// flattening call or assigned to the local variable it names.
func configFlatteningAllowedNodes(pkg *wrapTypedPackage) map[*ast.CallExpr]bool {
	allowed := map[*ast.CallExpr]bool{}
	if pkg.pkg.Path() != configImportPath {
		return allowed
	}
	for _, file := range pkg.files {
		ast.Inspect(file.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isValidateVariableNameForScope(pkg, call) || len(call.Args) < 3 {
				return true
			}
			if node, ok := assignedCall(pkg, call.Args[2]).(*ast.CallExpr); ok {
				allowed[node] = true
			}
			return true
		})
	}
	return allowed
}

// isValidateVariableNameForScope reports whether call resolves to
// variable.ValidateVariableNameForScope.
func isValidateVariableNameForScope(pkg *wrapTypedPackage, call *ast.CallExpr) bool {
	sel, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "ValidateVariableNameForScope" {
		return false
	}
	fn, ok := pkg.info.Uses[sel.Sel].(*types.Func)
	return ok && fn.Pkg() != nil &&
		fn.Pkg().Path() == wrapGuardModulePath+"/internal/runner/base/variable"
}

// assignedCall returns the call expression that produces the value of expr: the
// call itself, or the call bound to the variable expr names by an assignment
// (location := ...) or a var declaration (var location = ...).
func assignedCall(pkg *wrapTypedPackage, expr ast.Expr) ast.Expr {
	expr = identitymutationguard.UnwrapParen(expr)
	if _, ok := expr.(*ast.CallExpr); ok {
		return expr
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return nil
	}
	obj := pkg.info.Uses[ident]
	if obj == nil {
		obj = pkg.info.Defs[ident]
	}
	if obj == nil {
		return nil
	}
	for _, file := range pkg.files {
		var found ast.Expr
		ast.Inspect(file.file, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			var names, values []ast.Expr
			switch decl := n.(type) {
			case *ast.AssignStmt:
				names, values = decl.Lhs, decl.Rhs
			case *ast.ValueSpec:
				for _, name := range decl.Names {
					names = append(names, name)
				}
				values = decl.Values
			default:
				return true
			}
			if len(values) != 1 {
				return true
			}
			rhs := identitymutationguard.UnwrapParen(values[0])
			if _, ok := rhs.(*ast.CallExpr); !ok {
				return true
			}
			for _, name := range names {
				nameIdent, ok := identitymutationguard.UnwrapParen(name).(*ast.Ident)
				if !ok {
					continue
				}
				def := pkg.info.Defs[nameIdent]
				if def == nil {
					def = pkg.info.Uses[nameIdent]
				}
				if def == obj {
					found = rhs
					return false
				}
			}
			return true
		})
		if found != nil {
			return found
		}
	}
	return nil
}

// fmtFlatteningCall reports whether call is a fmt call that renders at least one
// value argument with a flattening verb, the arguments it so renders, and the
// function name.
func fmtFlatteningCall(pkg *wrapTypedPackage, call *ast.CallExpr) (args []ast.Expr, name string, ok bool) {
	sel, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil, "", false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, "", false
	}
	pkgName, ok := pkg.info.Uses[pkgIdent].(*types.PkgName)
	if !ok || pkgName.Imported().Path() != "fmt" {
		return nil, "", false
	}
	name = sel.Sel.Name
	switch name {
	case "Sprintf", "Errorf", "Printf":
		args = fmtFlatteningArgs(pkg, call.Args, 0)
	case "Fprintf", "Appendf":
		args = fmtFlatteningArgs(pkg, call.Args, 1)
	case "Sprint", "Sprintln", "Print", "Println":
		args = call.Args
	case "Fprint", "Fprintln", "Append", "Appendln":
		if len(call.Args) < 1 {
			return nil, name, false
		}
		args = call.Args[1:]
	default:
		return nil, name, false
	}
	return args, name, len(args) > 0
}

// fmtFlatteningArgs returns the value arguments a format string renders with a
// flattening verb. The format string is at index formatIdx and the value
// arguments follow it. When the format argument has no constant string value,
// every value argument may be rendered by a flattening verb, so all of them are
// returned: the check fails closed rather than missing a flattening.
func fmtFlatteningArgs(pkg *wrapTypedPackage, all []ast.Expr, formatIdx int) []ast.Expr {
	if len(all) <= formatIdx {
		return nil
	}
	values := all[formatIdx+1:]
	if len(values) == 0 {
		return nil
	}
	tv, ok := pkg.info.Types[all[formatIdx]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return values
	}
	indexes := flatteningVerbArgIndexes(constant.StringVal(tv.Value))
	if len(indexes) == 0 {
		return nil
	}
	var args []ast.Expr
	for i, arg := range values {
		if indexes[i+1] {
			args = append(args, arg)
		}
	}
	return args
}

// flatteningVerbArgIndexes returns the 1-based positions of the value arguments
// that a constant format string renders with a flattening verb (%s, %v or %q).
// It follows fmt's argument indexing: an explicit [n] index, the implicit
// sequential counter, and the arguments consumed by a * width or precision. A
// [n] index may follow a * width or precision, so "%[2]*[1]s" renders argument
// 1 with %s while argument 2 supplies the width. Non-flattening verbs still
// advance the counter, so a value used only with one of them is never
// attributed to a flattening verb.
func flatteningVerbArgIndexes(format string) map[int]bool {
	out := map[int]bool{}
	next := 1
	for i := 0; i < len(format); {
		if format[i] != '%' {
			i++
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			i++
			continue
		}
		// Optional explicit argument index.
		if n, ni, ok := parseArgIndex(format, i); ok {
			next, i = n, ni
		}
		// Flags.
		for i < len(format) && strings.ContainsRune("+#- 0", rune(format[i])) {
			i++
		}
		// Optional explicit argument index after the flags.
		if n, ni, ok := parseArgIndex(format, i); ok {
			next, i = n, ni
		}
		// Width: digits, or a * that consumes an argument and may be followed
		// by an explicit argument index.
		if i < len(format) && format[i] == '*' {
			i++
			next++
			if n, ni, ok := parseArgIndex(format, i); ok {
				next, i = n, ni
			}
		} else {
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		// Precision: digits, or a * that consumes an argument and may be
		// followed by an explicit argument index.
		if i < len(format) && format[i] == '.' {
			i++
			if n, ni, ok := parseArgIndex(format, i); ok {
				next, i = n, ni
			}
			if i < len(format) && format[i] == '*' {
				i++
				next++
				if n, ni, ok := parseArgIndex(format, i); ok {
					next, i = n, ni
				}
			} else {
				for i < len(format) && format[i] >= '0' && format[i] <= '9' {
					i++
				}
			}
		}
		// Optional explicit argument index before the verb.
		if n, ni, ok := parseArgIndex(format, i); ok {
			next, i = n, ni
		}
		if i >= len(format) {
			break
		}
		verb := format[i]
		i++
		switch verb {
		case 's', 'v', 'q':
			out[next] = true
		}
		next++
	}
	return out
}

// parseArgIndex reads an explicit fmt argument index "[n]" at format[i]. It
// reports whether one was present and returns the 1-based index and the index
// just past the closing bracket.
func parseArgIndex(format string, i int) (n, next int, ok bool) {
	if i >= len(format) || format[i] != '[' {
		return 0, i, false
	}
	j := i + 1
	for j < len(format) && format[j] != ']' {
		j++
	}
	if j >= len(format) {
		return 0, i, false
	}
	v, err := strconv.Atoi(format[i+1 : j])
	if err != nil || v <= 0 {
		return 0, i, false
	}
	return v, j + 1, true
}

// configGuardPackage returns the type-checked config package the guards read.
func configGuardPackage(t *testing.T) *wrapTypedPackage {
	t.Helper()
	pkg := wrapProductionGuardSet(t).packageForFile("internal/runner/config/errors.go")
	require.NotNil(t, pkg, "no type-checked package for internal/runner/config; the guard is broken")
	return pkg
}

// TestConfigErrorTypesDeclareStructuredMessage pins that every type declared in
// internal/runner/config with an Error() string method also declares a
// StructuredMessage of the structured contract. The type list is not
// maintained: a new error type is checked automatically.
func TestConfigErrorTypesDeclareStructuredMessage(t *testing.T) {
	pkgReal := configGuardPackage(t)
	require.Positive(t, len(configErrorTypeNames(pkgReal)),
		"the config package must declare error types, or the scan is vacuous")
	missing := configErrorTypesMissingStructured(pkgReal)
	assert.Empty(t, missing,
		"config types with Error() must declare StructuredMessage: %v", missing)

	// Self-test: a type with Error() and no StructuredMessage is reported; a
	// type with the structured contract is accepted.
	pkg := syntheticWrapPackage(t, "internal/runner/config/x.go",
		"package config\n\ntype onlyError struct{}\n\nfunc (onlyError) Error() string { return \"\" }\n")
	assert.Equal(t, []string{"onlyError"}, configErrorTypesMissingStructured(pkg))

	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		"package config\n\nimport \""+wrapGuardErrmsgImport+"\"\n\ntype both struct{}\n\nfunc (both) Error() string { return \"\" }\n\nfunc (both) StructuredMessage() errmsg.Message { return errmsg.Message{} }\n")
	assert.Empty(t, configErrorTypesMissingStructured(pkg))
}

// TestConfigErrorLevelAndFieldTypesAreTyped pins that the level and field slots
// of the config error types are typed config.Level/config.Field, and that a raw
// string field whose name merely ends in "Field" is not treated as a slot.
func TestConfigErrorLevelAndFieldTypesAreTyped(t *testing.T) {
	wrong := configLevelFieldTypesWrong(configGuardPackage(t))
	assert.Empty(t, wrong, "level/field slots must be typed: %v", wrong)

	const header = "package config\n\ntype Level struct{}\n\ntype Field struct{}\n\n"

	// Self-test: a string Level slot is reported.
	pkg := syntheticWrapPackage(t, "internal/runner/config/x.go",
		header+"type e struct{ Level string }\n\nfunc (e) Error() string { return \"\" }\n")
	assert.Equal(t, []string{"e.Level"}, configLevelFieldTypesWrong(pkg))

	// Self-test: a "...Level" slot is reported.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		header+"type e struct{ VarsLevel string }\n\nfunc (e) Error() string { return \"\" }\n")
	assert.Equal(t, []string{"e.VarsLevel"}, configLevelFieldTypesWrong(pkg))

	// Self-test: typed slots are accepted.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		header+"type e struct{ Level Level; Field Field; EnvImportLevel Level }\n\nfunc (e) Error() string { return \"\" }\n")
	assert.Empty(t, configLevelFieldTypesWrong(pkg))

	// Self-test: a raw string field ending in "Field" is not a slot.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		header+"type e struct{ UnknownField string }\n\nfunc (e) Error() string { return \"\" }\n")
	assert.Empty(t, configLevelFieldTypesWrong(pkg))
}

// TestConfigProductionDoesNotFlattenLevelOrField pins that no production code
// flattens a Level or Field to a string, except the one location string in
// validateVariableName.
func TestConfigProductionDoesNotFlattenLevelOrField(t *testing.T) {
	// internal/runner/cli is in scope because it builds config errors; it
	// currently imports config only through runnertypes, so configRoleTypes
	// reports it as having no Level/Field values and the check is trivially
	// satisfied there. The config package must be non-vacuous.
	levelType, _, present := configRoleTypes(configGuardPackage(t))
	require.True(t, present && levelType != nil, "the config package must expose Level; the guard is broken")

	for _, dir := range []string{"internal/runner/config", "internal/runner/cli"} {
		pkg := wrapProductionGuardSet(t).pkgs[dir]
		require.NotNilf(t, pkg, "no type-checked package for %s; the guard is broken", dir)
		violations := configFlattenedLevelFieldValues(pkg)
		assert.Empty(t, violations, strings.Join(violations, "\n"))
	}

	const pkgLine = "package config\n\n"
	const fmtImport = "import \"fmt\"\n\n"
	const roleTypes = "type Level struct{}\n\nfunc (Level) String() string { return \"\" }\n\ntype Field struct{}\n\nfunc (Field) String() string { return \"\" }\n\n"

	// The allowed-position self-tests call variable.ValidateVariableNameForScope,
	// so they type-check a synthetic variable package alongside the config one.
	const variablePath = wrapGuardModulePath + "/internal/runner/base/variable"
	const variableFile = "internal/runner/base/variable/scope.go"
	const variableSrc = "package variable\n\ntype Scope int\n\nconst (\n\tScopeLocal Scope = iota\n\tScopeGlobal\n)\n\nfunc ValidateVariableNameForScope(name string, expectedScope Scope, location string) error { return nil }\n"
	configWithLocation := func(body string) string {
		return pkgLine +
			"import (\n\t\"fmt\"\n\tvariable \"" + variablePath + "\"\n)\n\n" +
			roleTypes + body
	}
	syntheticAllowed := func(body string) *wrapTypedPackage {
		return syntheticWrapPackageFiles(t,
			map[string]string{variableFile: variableSrc, configFlatteningAllowedFile: configWithLocation(body)},
			configFlatteningAllowedFile)
	}

	// Self-test: the location string passed to ValidateVariableNameForScope is
	// accepted, even when built in a temporary variable.
	allowedBody := "func validateVariableName(l Level, fld Field) string {\n\tlocation := fmt.Sprintf(\"%s.%s\", l, fld)\n\t_ = variable.ValidateVariableNameForScope(\"x\", variable.ScopeLocal, location)\n\treturn location\n}\n"
	assert.Empty(t, configFlattenedLevelFieldValues(syntheticAllowed(allowedBody)))

	// Self-test: a second "%s.%s" call in the same function is not the location
	// expression, so it is reported.
	secondBody := "func validateVariableName(l Level, fld Field) string {\n\tlocation := fmt.Sprintf(\"%s.%s\", l, fld)\n\t_ = variable.ValidateVariableNameForScope(\"x\", variable.ScopeLocal, location)\n\treturn fmt.Sprintf(\"%s.%s\", l, fld)\n}\n"
	assert.Len(t, configFlattenedLevelFieldValues(syntheticAllowed(secondBody)), 1)

	// Self-test: a var-declared location expression is accepted too.
	varAllowedBody := "func validateVariableName(l Level, fld Field) string {\n\tvar location = fmt.Sprintf(\"%s.%s\", l, fld)\n\t_ = variable.ValidateVariableNameForScope(\"x\", variable.ScopeLocal, location)\n\treturn location\n}\n"
	assert.Empty(t, configFlattenedLevelFieldValues(syntheticAllowed(varAllowedBody)))

	// Self-test: a wrong-position "%s.%s" next to a var-declared location
	// expression is reported.
	varSecondBody := "func validateVariableName(l Level, fld Field) string {\n\tvar location = fmt.Sprintf(\"%s.%s\", l, fld)\n\t_ = variable.ValidateVariableNameForScope(\"x\", variable.ScopeLocal, location)\n\treturn fmt.Sprintf(\"%s.%s\", l, fld)\n}\n"
	assert.Len(t, configFlattenedLevelFieldValues(syntheticAllowed(varSecondBody)), 1)

	// Self-test: a Level rendered with %s elsewhere is reported.
	pkg := syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level) string { return fmt.Sprintf(\"%s\", l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: a Field rendered with %v and a Level with %q are reported.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(fld Field) string { return fmt.Sprintf(\"%v\", fld) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level) string { return fmt.Sprintf(\"%q\", l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: a flagged verb (%+v) and the no-format Sprint family are
	// reported too.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level) string { return fmt.Sprintf(\"%+v\", l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level, fld Field) string { return fmt.Sprint(l, fld) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: the fmt Append family renders its value arguments too.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level) []byte { return fmt.Appendf(nil, \"%s\", l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level) []byte { return fmt.Append(nil, l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: a nonconstant format string fails closed, so a Level that may
	// be rendered by a flattening verb is reported.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level, format string) string { return fmt.Sprintf(format, l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: an explicit index may follow a * width, so %[2]*[1]s renders
	// argument 1 (the Level) and argument 2 supplies the width.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(level Level, width int) string { return fmt.Sprintf(\"%[2]*[1]s\", level, width) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: a flattening verb binds only the argument it consumes. Here
	// %T consumes the Level and %s the string, so there is no violation; when
	// %s consumes the Level, there is one.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level, detail string) string { return fmt.Sprintf(\"type %T: %s\", l, detail) }\n")
	assert.Empty(t, configFlattenedLevelFieldValues(pkg))

	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level, detail string) string { return fmt.Sprintf(\"type %s: %T\", l, detail) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: a pointer to a Level or Field flattens the value it points at.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level) string { return fmt.Sprintf(\"%s\", &l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+roleTypes+"func f(l Level) string { return (&l).String() }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: an explicit .String() outside the allowed position is reported.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+roleTypes+"func f(l Level) string { return l.String() }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: a flattening call inside validateVariableName that is not the
	// location expression is rejected.
	pkg = syntheticWrapPackage(t, configFlatteningAllowedFile,
		pkgLine+fmtImport+roleTypes+"func validateVariableName(l Level) string { return fmt.Sprintf(\"%s\", l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)
}
