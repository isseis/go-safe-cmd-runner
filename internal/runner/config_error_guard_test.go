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

// configErrorTypesMissingStructured returns the names of the types declared in
// pkg that have an Error() string method but no StructuredMessage of the
// structured contract. A type alias is checked as the type it points at.
func configErrorTypesMissingStructured(pkg *wrapTypedPackage) []string {
	var missing []string
	for _, name := range pkg.pkg.Scope().Names() {
		tn, ok := pkg.pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		if !declaresErrorString(pkg, tn) {
			continue
		}
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

// configFlatteningAllowedFile is the one file whose single "%s.%s" location
// string may render a Level and a Field through fmt: the position string
// validateVariableName passes to variable.ValidateVariableNameForScope.
const configFlatteningAllowedFile = "internal/runner/config/validation.go"

// configFlatteningAllowedFormat is the only format allowed in that file.
const configFlatteningAllowedFormat = "%s.%s"

// configFlattenedLevelFieldValues reports every position in pkg where a
// config.Level or config.Field value is flattened to a string and loses its
// role: an explicit .String() call, or a fmt.Sprintf/Sprintf/Sprint/Errorf
// argument rendered with %s, %v or %q. The only allowed position is the
// "%s.%s" location string in validation.go.
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
		ast.Inspect(file.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// fmt.Sprintf and friends flatten a Level/Field argument.
			if format, ok := fmtCallFormat(pkg, call); ok {
				if isAllowedFlattening(file.path, format) {
					return true
				}
				if containsFlatteningVerb(format) {
					for _, arg := range call.Args[1:] {
						if isRoleValue(pkg.info.Types[arg].Type) {
							violations = append(violations, fmt.Sprintf(
								"%s: a config.Level/config.Field is rendered through fmt; use its parts() instead",
								wrapGuardFset.Position(call.Pos())))
							break
						}
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
	return violations
}

// fmtCallFormat returns the constant format string of a call to one of fmt's
// formatting functions, and whether the call is such a call.
func fmtCallFormat(pkg *wrapTypedPackage, call *ast.CallExpr) (string, bool) {
	sel, ok := identitymutationguard.UnwrapParen(call.Fun).(*ast.SelectorExpr)
	if !ok || len(call.Args) == 0 {
		return "", false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	pkgName, ok := pkg.info.Uses[pkgIdent].(*types.PkgName)
	if !ok || pkgName.Imported().Path() != "fmt" {
		return "", false
	}
	switch sel.Sel.Name {
	case "Sprintf", "Sprint", "Sprintln", "Errorf":
	default:
		return "", false
	}
	tv, ok := pkg.info.Types[call.Args[0]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// containsFlatteningVerb reports whether format renders a value with %s, %v or
// %q. Index/width/precision modifiers are ignored; ordinary code does not
// combine them with a role value.
func containsFlatteningVerb(format string) bool {
	return strings.Contains(format, "%s") || strings.Contains(format, "%v") || strings.Contains(format, "%q")
}

// isAllowedFlattening reports whether the position is the one location string
// that may render a Level and a Field through fmt.
func isAllowedFlattening(file, format string) bool {
	return file == configFlatteningAllowedFile && format == configFlatteningAllowedFormat
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
	missing := configErrorTypesMissingStructured(configGuardPackage(t))
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
		pkgLine+fmtImport+roleTypes+"func f(l Level, fld Field) string { return fmt.Sprintf(\"%s.%s\", l, fld) }\n")
	assert.Empty(t, configFlattenedLevelFieldValues(pkg))

	// Self-test: a Level rendered with %s elsewhere is reported.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(l Level) string { return fmt.Sprintf(\"%s\", l) }\n")
	assert.Len(t, configFlattenedLevelFieldValues(pkg), 1)

	// Self-test: a Field rendered with %v is reported.
	pkg = syntheticWrapPackage(t, "internal/runner/config/x.go",
		pkgLine+fmtImport+roleTypes+"func f(fld Field) string { return fmt.Sprintf(\"%v\", fld) }\n")
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
