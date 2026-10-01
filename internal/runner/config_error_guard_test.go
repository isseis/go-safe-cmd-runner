//go:build test

package runner

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
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

// configFlatteningAllowedFile and configFlatteningAllowedFormat name the one
// allowed flattening: the "%s.%s" location string validateVariableName passes
// to variable.ValidateVariableNameForScope.
const (
	configFlatteningAllowedFile   = "internal/runner/config/validation.go"
	configFlatteningAllowedFormat = "%s.%s"
	configFlatteningAllowedFunc   = "validateVariableName"
)

// configFlattenedLevelFieldValues reports every position in pkg where a
// config.Level or config.Field value is flattened to a string and loses its
// role: an explicit .String() call, or a fmt formatting call that renders it
// (with %s/%v/%q, or the no-format Print/Sprint family). The only allowed
// position is the location string in validateVariableName's expression.
func configFlattenedLevelFieldValues(pkg *wrapTypedPackage) []string {
	levelType, fieldType, present := configRoleTypes(pkg)
	if !present {
		return nil
	}
	if levelType == nil || fieldType == nil {
		return []string{"config.Level/config.Field not found; the guard is broken"}
	}
	isRoleValue := func(t types.Type) bool {
		return t != nil && (types.Identical(t, levelType) || types.Identical(t, fieldType))
	}

	var violations []string
	for _, file := range pkg.files {
		for _, decl := range file.file.Decls {
			funcName := ""
			if fd, ok := decl.(*ast.FuncDecl); ok {
				funcName = fd.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// A fmt formatting call flattens a Level/Field argument.
				format, fmtArgs, fmtName, ok := fmtFlatteningCall(pkg, call)
				if ok {
					if isAllowedFlattening(file.path, funcName, fmtName, format) {
						return true
					}
					for _, arg := range fmtArgs {
						if isRoleValue(pkg.info.Types[arg].Type) {
							violations = append(violations, fmt.Sprintf(
								"%s: a config.Level/config.Field is rendered through fmt.%s; use its parts() instead",
								wrapGuardFset.Position(call.Pos()), fmtName))
							break
						}
					}
					return true
				}

				// An explicit .String() call on a Level/Field value.
				sel, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "String" {
					return true
				}
				if isRoleValue(pkg.info.Types[sel.X].Type) {
					violations = append(violations, fmt.Sprintf(
						"%s: a config.Level/config.Field is flattened with .String(); use its parts() instead",
						wrapGuardFset.Position(call.Pos())))
				}
				return true
			})
		}
	}
	return violations
}

// fmtFlatteningCall reports whether call is a fmt call that would render its
// value arguments, the arguments that carry values (the format string and the
// writer excluded), the function name, and the constant format string (empty
// for the no-format Print/Sprint family).
func fmtFlatteningCall(pkg *wrapTypedPackage, call *ast.CallExpr) (format string, args []ast.Expr, name string, ok bool) {
	sel, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return "", nil, "", false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", nil, "", false
	}
	pkgName, ok := pkg.info.Uses[pkgIdent].(*types.PkgName)
	if !ok || pkgName.Imported().Path() != "fmt" {
		return "", nil, "", false
	}
	name = sel.Sel.Name
	switch name {
	case "Sprintf", "Errorf", "Printf":
		format, args, ok = fmtFormatArgs(pkg, call.Args, 0)
	case "Fprintf":
		format, args, ok = fmtFormatArgs(pkg, call.Args, 1)
	case "Sprint", "Sprintln", "Print", "Println":
		args, ok = call.Args, true
	case "Fprint", "Fprintln":
		if len(call.Args) < 1 {
			return "", nil, name, false
		}
		args, ok = call.Args[1:], true
	default:
		return "", nil, name, false
	}
	return format, args, name, ok
}

// fmtFormatArgs returns the value arguments of a fmt call whose format string is
// at index formatIdx, when that format string is constant and renders a value
// with %s, %v or %q.
func fmtFormatArgs(pkg *wrapTypedPackage, all []ast.Expr, formatIdx int) (string, []ast.Expr, bool) {
	if len(all) <= formatIdx {
		return "", nil, false
	}
	tv, ok := pkg.info.Types[all[formatIdx]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", nil, false
	}
	format := constant.StringVal(tv.Value)
	if !containsFlatteningVerb(format) {
		return "", nil, false
	}
	return format, all[formatIdx+1:], true
}

// containsFlatteningVerb reports whether format renders a value with a
// flattening verb: %s, %v or %q, with any flags, argument index, width or
// precision between the percent sign and the verb (so %+v, %#v, %10v and
// %[1]v are recognized).
func containsFlatteningVerb(format string) bool {
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		j := i + 1
		if j < len(format) && format[j] == '%' {
			i = j
			continue
		}
		for j < len(format) && strings.ContainsRune("+#- 0", rune(format[j])) {
			j++
		}
		if j < len(format) && format[j] == '[' {
			for j < len(format) && format[j] != ']' {
				j++
			}
			j++
		}
		for j < len(format) && (format[j] == '.' || (format[j] >= '0' && format[j] <= '9')) {
			j++
		}
		if j < len(format) {
			switch format[j] {
			case 's', 'v', 'q':
				return true
			}
		}
		i = j
	}
	return false
}

// isAllowedFlattening reports whether the position is the one expression that
// may render a Level and a Field through fmt: the "%s.%s" call inside
// validateVariableName.
func isAllowedFlattening(file, funcName, fmtName, format string) bool {
	return file == configFlatteningAllowedFile &&
		funcName == configFlatteningAllowedFunc &&
		fmtName == "Sprintf" &&
		format == configFlatteningAllowedFormat
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

	// Self-test: the allowed location string is accepted.
	pkg := syntheticWrapPackage(t, configFlatteningAllowedFile,
		pkgLine+fmtImport+roleTypes+"func validateVariableName(l Level, fld Field) string { return fmt.Sprintf(\"%s.%s\", l, fld) }\n")
	assert.Empty(t, configFlattenedLevelFieldValues(pkg))

	// Self-test: a Level rendered with %s elsewhere is reported.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
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

	// Self-test: an explicit .String() outside the allowed position is reported.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+roleTypes+"func f(l Level) string { return l.String() }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: the same flattening inside validateVariableName is rejected
	// too, because only its "%s.%s" expression is allowed.
	pkg = syntheticWrapPackage(t, configFlatteningAllowedFile,
		pkgLine+fmtImport+roleTypes+"func validateVariableName(l Level) string { return fmt.Sprintf(\"%s\", l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)
}
