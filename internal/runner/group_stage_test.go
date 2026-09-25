package runner

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errGroupStageCause is the sentinel wrapped by the stage-error tests.
var errGroupStageCause = errors.New("group stage cause")

// TestGroupStageTableHasARowForEveryStage pins that the definition table
// covers every declared stage: a stage added without a row would leave a zero
// row and report an empty error_type.
func TestGroupStageTableHasARowForEveryStage(t *testing.T) {
	generic := groupStageDefinitions[GroupStageUnknown]
	for stage := range groupStageCount {
		def, ok := groupStageDefinitionFor(stage)
		require.True(t, ok, "the lookup must accept the declared stage %s", stage)
		assert.NotEmpty(t, def.errorType, "stage %s has no error_type", stage)
		assert.NotEmpty(t, def.message, "stage %s has no summary", stage)
		if stage != GroupStageUnknown {
			assert.NotEqual(t, generic, def, "stage %s must not reuse the generic row", stage)
		}
	}

	// The lookup must reject the closing sentinel and negative values, so an
	// out-of-range stage cannot silently read a real row.
	_, ok := groupStageDefinitionFor(groupStageCount)
	assert.False(t, ok, "the closing sentinel must not resolve to a stage")
	_, ok = groupStageDefinitionFor(GroupStage(-1))
	assert.False(t, ok, "a negative stage must not resolve to a row")
}

// TestGroupStagePreExecutionErrorMapping pins the error_type, summary, scope
// and cause that the conversion produces for every stage.
func TestGroupStagePreExecutionErrorMapping(t *testing.T) {
	tests := []struct {
		name      string
		stage     GroupStage
		group     string
		command   string
		errorType logging.ErrorType
		message   string
		ctx       common.NotificationContext
	}{
		{
			name:      "unknown uses the generic row",
			stage:     GroupStageUnknown,
			group:     "backup",
			errorType: logging.ErrorTypeGroupPreExecution,
			message:   "Group pre-execution failed",
			ctx:       common.GroupScope("backup"),
		},
		{
			name:      "group preparation is group-scoped",
			stage:     GroupStageGroupPreparation,
			group:     "backup",
			errorType: logging.ErrorTypeGroupPreparation,
			message:   "Group preparation failed",
			ctx:       common.GroupScope("backup"),
		},
		{
			name:      "command preparation is command-scoped",
			stage:     GroupStageCommandPreparation,
			group:     "backup",
			command:   "dump",
			errorType: logging.ErrorTypeGroupPreparation,
			message:   "Command preparation failed",
			ctx:       common.CommandScope("backup", "dump"),
		},
		{
			name:      "directory permission audit is group-scoped",
			stage:     GroupStageDirPermissionAudit,
			group:     "backup",
			errorType: logging.ErrorTypeGroupDirPermissionViolation,
			message:   "Group directory permission audit failed",
			ctx:       common.GroupScope("backup"),
		},
		{
			name:      "file verification is group-scoped",
			stage:     GroupStageFileVerification,
			group:     "backup",
			errorType: logging.ErrorTypeGroupFileVerification,
			message:   "Group file verification failed",
			ctx:       common.GroupScope("backup"),
		},
		{
			name:      "command verification is command-scoped",
			stage:     GroupStageCommandVerification,
			group:     "backup",
			command:   "dump",
			errorType: logging.ErrorTypeCommandVerification,
			message:   "Command verification failed",
			ctx:       common.CommandScope("backup", "dump"),
		},
	}

	cause := fmt.Errorf("wrapped: %w", errGroupStageCause)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stageErr *GroupStageError
			if tt.command != "" {
				stageErr = newCommandStageError(tt.stage, tt.group, tt.command, cause)
			} else {
				stageErr = newGroupStageError(tt.stage, tt.group, cause)
			}
			assert.Equal(t, tt.command, stageErr.CommandName(),
				"the command name must match the stage level")

			got := groupStagePreExecutionError(stageErr, "run-mapping")

			assert.Equal(t, tt.errorType, got.Type)
			assert.Equal(t, tt.message, got.Message)
			assert.Equal(t, string(resource.ComponentRunner), got.Component)
			assert.Equal(t, "run-mapping", got.RunID)
			assert.Equal(t, tt.ctx, got.NotificationContext)
			assert.Equal(t, cause, got.Err)
			assert.Nil(t, got.FailedFilePaths, "these failures carry no target list")
		})
	}
}

// TestGroupStageUnknownAndOutOfRangeUseGenericRow pins the fail-secure path:
// an undeclared or out-of-range stage still notifies, under the generic row
// and a valid group scope.
func TestGroupStageUnknownAndOutOfRangeUseGenericRow(t *testing.T) {
	generic := groupStageDefinitions[GroupStageUnknown]
	cause := fmt.Errorf("wrapped: %w", errGroupStageCause)

	tests := []struct {
		name     string
		stageErr *GroupStageError
	}{
		{
			name:     "undeclared stage",
			stageErr: newGroupStageError(GroupStageUnknown, "backup", cause),
		},
		{
			// An out-of-range stage cannot go through a constructor, so the
			// test builds it directly; this is the only way to exercise the
			// fallback for a value the constructor rejects.
			name:     "out-of-range stage",
			stageErr: &GroupStageError{stage: GroupStage(99), group: "backup", err: cause},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupStagePreExecutionError(tt.stageErr, "run-fallback")
			assert.Equal(t, generic.errorType, got.Type)
			assert.Equal(t, generic.message, got.Message)
			assert.Equal(t, common.GroupScope("backup"), got.NotificationContext)
		})
	}
}

// TestGroupStageErrorZeroValueDoesNotPanic pins the robustness of the zero
// value: Error() must not panic, and the conversion must fall back to the
// generic row.
func TestGroupStageErrorZeroValueDoesNotPanic(t *testing.T) {
	generic := groupStageDefinitions[GroupStageUnknown]

	var zero GroupStageError
	var got string
	require.NotPanics(t, func() { got = zero.Error() })
	assert.Equal(t, "group pre-execution failed", got)

	preExecErr := groupStagePreExecutionError(&zero, "run-zero")
	assert.Equal(t, generic.errorType, preExecErr.Type)
	assert.Equal(t, generic.message, preExecErr.Message)
	// The zero value has no group name, so the scope is a group scope with an
	// empty name; the display boundary reports that as an invalid scope.
	assert.Equal(t, common.GroupScope(""), preExecErr.NotificationContext)
}

// TestGroupStagePreExecutionErrorUsesDeclaredStageNotReasonText pins that the
// error_type follows the declared stage, never words in the cause.
func TestGroupStagePreExecutionErrorUsesDeclaredStageNotReasonText(t *testing.T) {
	cause := fmt.Errorf("verification permission token failed: %w", errGroupStageCause)
	stageErr := newGroupStageError(GroupStageGroupPreparation, "backup", cause)

	got := groupStagePreExecutionError(stageErr, "run-declared")

	assert.Equal(t, logging.ErrorTypeGroupPreparation, got.Type,
		"the declared stage must decide the error_type, not the cause text")
	assert.Equal(t, "Group preparation failed", got.Message)
	assert.Equal(t, common.GroupScope("backup"), got.NotificationContext)
}

// TestGroupStageConstructorsPanicOnInvalidInput pins that the constructors
// reject a stage whose level contradicts the constructor, an out-of-range
// stage, an empty name and a nil cause.
func TestGroupStageConstructorsPanicOnInvalidInput(t *testing.T) {
	cause := errGroupStageCause

	tests := []struct {
		name string
		call func()
	}{
		{
			name: "group constructor rejects a command-level stage",
			call: func() { newGroupStageError(GroupStageCommandPreparation, "backup", cause) },
		},
		{
			name: "group constructor rejects an out-of-range stage",
			call: func() { newGroupStageError(GroupStage(99), "backup", cause) },
		},
		{
			name: "group constructor rejects the closing sentinel",
			call: func() { newGroupStageError(groupStageCount, "backup", cause) },
		},
		{
			name: "group constructor rejects an empty group name",
			call: func() { newGroupStageError(GroupStageGroupPreparation, "", cause) },
		},
		{
			name: "group constructor rejects a nil cause",
			call: func() { newGroupStageError(GroupStageGroupPreparation, "backup", nil) },
		},
		{
			name: "command constructor rejects a group-level stage",
			call: func() { newCommandStageError(GroupStageGroupPreparation, "backup", "dump", cause) },
		},
		{
			name: "command constructor rejects the unknown stage",
			call: func() { newCommandStageError(GroupStageUnknown, "backup", "dump", cause) },
		},
		{
			name: "command constructor rejects an out-of-range stage",
			call: func() { newCommandStageError(GroupStage(99), "backup", "dump", cause) },
		},
		{
			name: "command constructor rejects the closing sentinel",
			call: func() { newCommandStageError(groupStageCount, "backup", "dump", cause) },
		},
		{
			name: "command constructor rejects an empty group name",
			call: func() { newCommandStageError(GroupStageCommandPreparation, "", "dump", cause) },
		},
		{
			name: "command constructor rejects an empty command name",
			call: func() { newCommandStageError(GroupStageCommandPreparation, "backup", "", cause) },
		},
		{
			name: "command constructor rejects a nil cause",
			call: func() { newCommandStageError(GroupStageCommandPreparation, "backup", "dump", nil) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, tt.call)
		})
	}
}

// TestGroupStageNotificationContextPanicsOnUnknownScope pins that a definition
// whose scope is neither group nor command fails loudly instead of silently
// reporting a group scope.
func TestGroupStageNotificationContextPanicsOnUnknownScope(t *testing.T) {
	assert.Panics(t, func() {
		groupStageNotificationContext(common.ScopeGlobal, "backup", "dump")
	})
}

// TestGroupStageErrorUnwrapsCause pins that the stage error reports the
// cause's message and preserves the errors.Is / errors.As chain.
func TestGroupStageErrorUnwrapsCause(t *testing.T) {
	cause := fmt.Errorf("wrapped: %w", errGroupStageCause)
	stageErr := newGroupStageError(GroupStageGroupPreparation, "backup", cause)

	assert.Equal(t, cause.Error(), stageErr.Error())
	assert.ErrorIs(t, stageErr, errGroupStageCause)
	assert.ErrorIs(t, stageErr, cause)

	outer := fmt.Errorf("outer: %w", stageErr)
	got, ok := errors.AsType[*GroupStageError](outer)
	require.True(t, ok, "the stage error must survive wrapping")
	assert.Equal(t, GroupStageGroupPreparation, got.Stage())
	assert.Equal(t, "backup", got.GroupName())
}

// TestGroupStageStringCoversEveryStage pins that each declared stage has a
// distinct, non-empty name and that a value outside the range reads
// "unknown".
func TestGroupStageStringCoversEveryStage(t *testing.T) {
	tests := []struct {
		stage GroupStage
		want  string
	}{
		{GroupStageUnknown, "GroupStageUnknown"},
		{GroupStageGroupPreparation, "GroupStageGroupPreparation"},
		{GroupStageCommandPreparation, "GroupStageCommandPreparation"},
		{GroupStageDirPermissionAudit, "GroupStageDirPermissionAudit"},
		{GroupStageFileVerification, "GroupStageFileVerification"},
		{GroupStageCommandVerification, "GroupStageCommandVerification"},
	}
	require.Len(t, tests, int(groupStageCount),
		"every declared stage needs an expected name")

	seen := make(map[string]GroupStage, groupStageCount)
	for _, tt := range tests {
		got := tt.stage.String()
		assert.Equal(t, tt.want, got)
		if previous, dup := seen[got]; dup {
			t.Errorf("stage %d duplicates the name %q of stage %d", tt.stage, got, previous)
		}
		seen[got] = tt.stage
	}

	for _, out := range []GroupStage{-1, groupStageCount, 99} {
		assert.Equal(t, "unknown", out.String(), "stage %d is out of range", out)
	}
}

const (
	// groupStageErrorFile is the file that declares GroupStageError and its
	// constructors.
	groupStageErrorFile = "internal/runner/group_stage.go"
	// groupStageErrorConstructors names the functions allowed to build a
	// GroupStageError, for the violation messages.
	groupStageErrorConstructors = "newGroupStageError or newCommandStageError"
	// runnerPackageDir is the package whose unqualified GroupStageError
	// identifier names the type, and where the private fields are assignable.
	runnerPackageDir = "internal/runner"
	// runnerImportPath resolves the qualified runner.GroupStageError form.
	runnerImportPath = "github.com/isseis/go-safe-cmd-runner/internal/runner"
)

// groupStageErrorConstructorNames are the functions allowed to build a
// GroupStageError.
var groupStageErrorConstructorNames = map[string]struct{}{
	"newGroupStageError":   {},
	"newCommandStageError": {},
}

// groupStageErrorFields are the private fields that only the constructors may
// set.
var groupStageErrorFields = []string{"stage", "group", "command", "err"}

// TestProductionGroupStageErrorLiteralsUseConstructors fixes the construction
// forms of GroupStageError and the field mutation that could bypass them. The
// fields are unexported, so a same-package production file could still build
// the struct or assign its fields directly; this go/ast guard is what keeps
// construction in the two constructors.
//
//   - A composite literal naming GroupStageError -- the qualified
//     runner.GroupStageError{...} form anywhere, and the unqualified
//     GroupStageError{...} form inside internal/runner -- must appear only
//     inside newGroupStageError or newCommandStageError.
//   - Inside internal/runner, a selector assignment or increment of .stage,
//     .group, .command or .err is reported outside those two constructors. The
//     scan is limited to this package because the unqualified type name is
//     unambiguous there; scanning wider would match unrelated selector
//     assignments such as the base executor's pc.stage and *w.err.
//   - The field-name match carries no type information, so a future struct
//     added directly under internal/runner with a field of the same name would
//     be a false positive. That is the known maintenance obligation of this
//     incomplete (by design) scan, as is a mutation made through a type alias.
//   - Test files are not scanned: they legitimately build out-of-range values
//     that the constructors reject.
//   - A scan that finds no literal fails rather than passing vacuously.
func TestProductionGroupStageErrorLiteralsUseConstructors(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	var (
		literals             int
		literalViolations    []string
		assignmentViolations []string
	)
	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		found, lv, av := checkGroupStageErrorConstruction(t, file, src)
		literals += found
		literalViolations = append(literalViolations, lv...)
		assignmentViolations = append(assignmentViolations, av...)
	}

	require.NotZero(t, literals,
		"the scan found no GroupStageError literal; the scan is broken")
	assert.Empty(t, literalViolations,
		"GroupStageError literals may only be built in %s:\n%s",
		groupStageErrorConstructors, strings.Join(literalViolations, "\n"))
	assert.Empty(t, assignmentViolations,
		"GroupStageError fields may only be set in %s:\n%s",
		groupStageErrorConstructors, strings.Join(assignmentViolations, "\n"))
}

// checkGroupStageErrorConstruction scans one production file and returns the
// number of GroupStageError literals found (value, pointer and elided forms)
// together with the positions of those built outside the two constructors and
// of any assignment or increment of one of its private fields outside them.
func checkGroupStageErrorConstruction(t *testing.T, filename, src string) (found int, literalViolations, assignmentViolations []string) {
	t.Helper()

	fset, file := identitymutationguard.ParseSource(t, filename, src)
	qualifiers := identitymutationguard.ResolveLocalImports(t, filename, file, func(importPath string) bool {
		return importPath == runnerImportPath
	})
	inPackage := path.Dir(filename) == runnerPackageDir
	isGroupStageError := func(expr ast.Expr) bool {
		return identitymutationguard.IsNamedType(expr, qualifiers, runnerImportPath, "GroupStageError", inPackage)
	}

	for _, decl := range file.Decls {
		funcName := ""
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcName = fn.Name.Name
		}
		_, isConstructor := groupStageErrorConstructorNames[funcName]
		allowed := filename == groupStageErrorFile && isConstructor

		ast.Inspect(decl, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				literals := make([]*ast.CompositeLit, 0, 1)
				if isGroupStageError(node.Type) {
					literals = append(literals, node)
				}
				literals = append(literals, identitymutationguard.ElidedCompositeLiterals(node, isGroupStageError)...)
				for _, literal := range literals {
					found++
					if !allowed {
						literalViolations = append(literalViolations, fmt.Sprintf(
							"%s: GroupStageError literal built outside %s", fset.Position(literal.Pos()), groupStageErrorConstructors))
					}
				}
			case *ast.AssignStmt:
				if !inPackage || allowed {
					return true
				}
				for _, lhs := range node.Lhs {
					reportGroupStageFieldMutation(fset, lhs, "assigned", &assignmentViolations)
				}
			case *ast.IncDecStmt:
				if !inPackage || allowed {
					return true
				}
				reportGroupStageFieldMutation(fset, node.X, "modified", &assignmentViolations)
			}
			return true
		})
	}
	return found, literalViolations, assignmentViolations
}

// reportGroupStageFieldMutation appends a violation when expr is a selector for
// one of GroupStageError's private fields.
func reportGroupStageFieldMutation(fset *token.FileSet, expr ast.Expr, verb string, violations *[]string) {
	sel, ok := identitymutationguard.UnwrapParen(expr).(*ast.SelectorExpr)
	if !ok || !slices.Contains(groupStageErrorFields, sel.Sel.Name) {
		return
	}
	*violations = append(*violations, fmt.Sprintf(
		"%s: GroupStageError.%s %s outside %s", fset.Position(expr.Pos()), sel.Sel.Name, verb, groupStageErrorConstructors))
}

// TestGroupStageConstructionCheckRecognizesForms pins the forms the guard must
// recognize, so the guard cannot become a no-op that still passes the
// repository scan.
func TestGroupStageConstructionCheckRecognizesForms(t *testing.T) {
	const header = "package x\n\nimport r \"github.com/isseis/go-safe-cmd-runner/internal/runner\"\n\nvar _ = r.GroupStageUnknown\n\n"

	literalTests := []struct {
		name              string
		path              string
		src               string
		wantLiterals      int
		wantLiteralMisses int
	}{
		{
			name:              "qualified pointer literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var e = &r.GroupStageError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "value literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var e = r.GroupStageError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "elided pointer literal outside the package is reported",
			path:              "internal/x/x.go",
			src:               header + "var es = []*r.GroupStageError{{}}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "unqualified literal inside the package is reported",
			path:              "internal/runner/x.go",
			src:               "package runner\n\nvar e = &GroupStageError{}\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:              "literal in a constructor is accepted",
			path:              groupStageErrorFile,
			src:               "package runner\n\nfunc newGroupStageError() *GroupStageError { return &GroupStageError{} }\n",
			wantLiterals:      1,
			wantLiteralMisses: 0,
		},
		{
			name:              "literal in another function of the constructor file is reported",
			path:              groupStageErrorFile,
			src:               "package runner\n\nfunc helper() *GroupStageError { return &GroupStageError{} }\n",
			wantLiterals:      1,
			wantLiteralMisses: 1,
		},
		{
			name:         "a same-named type in another package is not reported",
			path:         "internal/x/x.go",
			src:          "package x\n\ntype GroupStageError struct{}\n\nvar e = &GroupStageError{}\n",
			wantLiterals: 0,
		},
		{
			name:         "a different type inside the package is not reported",
			path:         "internal/runner/x.go",
			src:          "package runner\n\nvar e = &OtherError{}\n",
			wantLiterals: 0,
		},
	}

	for _, tt := range literalTests {
		t.Run(tt.name, func(t *testing.T) {
			found, literalMisses, _ := checkGroupStageErrorConstruction(t, tt.path, tt.src)
			assert.Equal(t, tt.wantLiterals, found)
			assert.Len(t, literalMisses, tt.wantLiteralMisses, "literal violations: %v", literalMisses)
		})
	}

	assignmentTests := []struct {
		name string
		path string
		src  string
		want int
	}{
		{
			name: "stage assignment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupStageError) { e.stage = GroupStageUnknown }\n",
			want: 1,
		},
		{
			name: "group assignment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupStageError) { e.group = \"x\" }\n",
			want: 1,
		},
		{
			name: "command assignment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupStageError) { e.command = \"x\" }\n",
			want: 1,
		},
		{
			name: "err assignment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupStageError) { e.err = nil }\n",
			want: 1,
		},
		{
			name: "stage increment outside the constructors is reported",
			path: "internal/runner/x.go",
			src:  "package runner\n\nfunc helper(e *GroupStageError) { e.stage++ }\n",
			want: 1,
		},
		{
			name: "assignment in a constructor is accepted",
			path: groupStageErrorFile,
			src:  "package runner\n\nfunc newGroupStageError(e *GroupStageError) { e.err = nil }\n",
			want: 0,
		},
		{
			name: "unrelated field assignment in another package is not reported",
			path: "internal/x/x.go",
			src:  "package x\n\nfunc helper(r *result) { r.err = nil }\n",
			want: 0,
		},
	}

	for _, tt := range assignmentTests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, assignmentMisses := checkGroupStageErrorConstruction(t, tt.path, tt.src)
			assert.Len(t, assignmentMisses, tt.want, "assignment violations: %v", assignmentMisses)
		})
	}
}
