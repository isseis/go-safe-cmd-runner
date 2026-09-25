package runner

import (
	"errors"
	"fmt"
	"go/ast"
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
	require.Equal(t, int(groupStageCount), len(groupStageDefinitions),
		"the table must have one row per stage")

	generic := groupStageDefinitions[GroupStageUnknown]
	for stage := range groupStageCount {
		def, ok := groupStageDefinitionFor(stage)
		require.True(t, ok, "stage %s has no row", stage)
		assert.NotEmpty(t, def.errorType, "stage %s has no error_type", stage)
		assert.NotEmpty(t, def.message, "stage %s has no summary", stage)
		if stage != GroupStageUnknown {
			assert.NotEqual(t, generic, def, "stage %s must not reuse the generic row", stage)
		}
	}
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
	seen := make(map[string]GroupStage, groupStageCount)
	for stage := range groupStageCount {
		name := stage.String()
		assert.NotEmpty(t, name, "stage %d has no name", stage)
		if previous, dup := seen[name]; dup {
			t.Errorf("stage %d duplicates the name %q of stage %d", stage, name, previous)
		}
		seen[name] = stage
	}

	assert.Equal(t, "unknown", GroupStage(-1).String())
	assert.Equal(t, "unknown", GroupStage(99).String())
}

const (
	// groupStageErrorFile is the file that declares GroupStageError and the
	// constructors allowed to build it.
	groupStageErrorFile = "internal/runner/group_stage.go"
	// runnerPackageDir is the package whose unqualified GroupStageError
	// identifier names the type, and where the private fields are assignable.
	runnerPackageDir = "internal/runner"
	// runnerImportPath resolves the qualified runner.GroupStageError form.
	runnerImportPath = "github.com/isseis/go-safe-cmd-runner/internal/runner"
)

// groupStageErrorFields are the private fields that only the constructors may
// set.
var groupStageErrorFields = []string{"stage", "group", "command", "err"}

// TestProductionGroupStageErrorLiteralsUseConstructors fixes the construction
// forms of GroupStageError and the field assignment that could bypass them.
// The fields are unexported, so a same-package production file could still
// build the struct or assign its fields directly; this go/ast guard is what
// keeps construction in the two constructors.
//
//   - A composite literal naming GroupStageError -- the qualified
//     runner.GroupStageError{...} form anywhere, and the unqualified
//     GroupStageError{...} form inside internal/runner -- must appear only in
//     group_stage.go.
//   - Inside internal/runner, a selector assignment to .stage, .group,
//     .command or .err must not appear outside group_stage.go. The scan is
//     limited to this package because the unqualified type name is
//     unambiguous there; scanning wider would match unrelated selector
//     assignments such as the base executor's pc.stage and *w.err.
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
		groupStageErrorFile, strings.Join(literalViolations, "\n"))
	assert.Empty(t, assignmentViolations,
		"GroupStageError fields may only be set in %s:\n%s",
		groupStageErrorFile, strings.Join(assignmentViolations, "\n"))
}

// checkGroupStageErrorConstruction scans one production file and returns the
// number of GroupStageError literals found (value, pointer and elided forms)
// together with the positions of those built outside group_stage.go and of any
// assignment to one of its private fields outside it.
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
	allowed := filename == groupStageErrorFile

	ast.Inspect(file, func(n ast.Node) bool {
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
						"%s: GroupStageError literal built outside %s", fset.Position(literal.Pos()), groupStageErrorFile))
				}
			}
		case *ast.AssignStmt:
			if !inPackage || allowed {
				return true
			}
			for _, lhs := range node.Lhs {
				sel, ok := identitymutationguard.UnwrapParen(lhs).(*ast.SelectorExpr)
				if !ok || !slices.Contains(groupStageErrorFields, sel.Sel.Name) {
					continue
				}
				assignmentViolations = append(assignmentViolations, fmt.Sprintf(
					"%s: GroupStageError.%s assigned outside %s", fset.Position(lhs.Pos()), sel.Sel.Name, groupStageErrorFile))
			}
		}
		return true
	})
	return found, literalViolations, assignmentViolations
}
