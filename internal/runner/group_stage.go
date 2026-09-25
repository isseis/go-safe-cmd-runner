package runner

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// GroupStage declares which pre-execution step of a group failed. The zero
// value GroupStageUnknown is reported under the generic error type.
type GroupStage int

const (
	// GroupStageUnknown is the zero value: the stage was not declared.
	GroupStageUnknown GroupStage = iota
	// GroupStageGroupPreparation covers group expansion and the group working
	// directory resolution.
	GroupStageGroupPreparation
	// GroupStageCommandPreparation covers command expansion and the command
	// working directory resolution.
	GroupStageCommandPreparation
	// GroupStageDirPermissionAudit covers the group directory permission audit.
	GroupStageDirPermissionAudit
	// GroupStageFileVerification covers group file verification.
	GroupStageFileVerification
	// GroupStageCommandVerification covers command path re-resolution and
	// dependency verification.
	GroupStageCommandVerification
	// groupStageCount closes the range of declared stages. It is not a stage.
	groupStageCount
)

// String returns the Go name of the stage, or "unknown" for a value outside
// the declared range.
func (s GroupStage) String() string {
	switch s {
	case GroupStageUnknown:
		return "GroupStageUnknown"
	case GroupStageGroupPreparation:
		return "GroupStageGroupPreparation"
	case GroupStageCommandPreparation:
		return "GroupStageCommandPreparation"
	case GroupStageDirPermissionAudit:
		return "GroupStageDirPermissionAudit"
	case GroupStageFileVerification:
		return "GroupStageFileVerification"
	case GroupStageCommandVerification:
		return "GroupStageCommandVerification"
	default:
		return "unknown"
	}
}

// groupStageDefinition is one row of the stage definition table: the
// error_type, the scope level and the summary line a stage reports.
type groupStageDefinition struct {
	errorType logging.ErrorType
	scope     common.NotificationScope
	message   string
}

// groupStageDefinitions is the single source of the per-stage notification
// definition. It is keyed by the stage constants and sized by groupStageCount,
// so a stage added without a row leaves a zero row that the table test catches.
var groupStageDefinitions = [groupStageCount]groupStageDefinition{
	GroupStageUnknown: {
		errorType: logging.ErrorTypeGroupPreExecution,
		scope:     common.ScopeGroup,
		message:   "Group pre-execution failed",
	},
	GroupStageGroupPreparation: {
		errorType: logging.ErrorTypeGroupPreparation,
		scope:     common.ScopeGroup,
		message:   "Group preparation failed",
	},
	GroupStageCommandPreparation: {
		errorType: logging.ErrorTypeGroupPreparation,
		scope:     common.ScopeCommand,
		message:   "Command preparation failed",
	},
	GroupStageDirPermissionAudit: {
		errorType: logging.ErrorTypeGroupDirPermissionViolation,
		scope:     common.ScopeGroup,
		message:   "Group directory permission audit failed",
	},
	GroupStageFileVerification: {
		errorType: logging.ErrorTypeGroupFileVerification,
		scope:     common.ScopeGroup,
		message:   "Group file verification failed",
	},
	GroupStageCommandVerification: {
		errorType: logging.ErrorTypeCommandVerification,
		scope:     common.ScopeCommand,
		message:   "Command verification failed",
	},
}

// groupStageDefinitionFor returns the table row of a declared stage. ok is
// false for a value outside the declared range, for which the caller must fall
// back to the generic row.
func groupStageDefinitionFor(stage GroupStage) (groupStageDefinition, bool) {
	if stage < GroupStageUnknown || stage >= groupStageCount {
		return groupStageDefinition{}, false
	}
	return groupStageDefinitions[stage], true
}

// GroupStageError reports that a group failed before its first command ran,
// and in which stage. Its fields are unexported so a stage is declared only
// through newGroupStageError or newCommandStageError.
type GroupStageError struct {
	stage   GroupStage
	group   string
	command string // set only for command-level stages
	err     error
}

// Stage returns the declared stage.
func (e *GroupStageError) Stage() GroupStage {
	return e.stage
}

// GroupName returns the group the failure belongs to.
func (e *GroupStageError) GroupName() string {
	return e.group
}

// CommandName returns the command the failure belongs to, or an empty string
// for a group-level stage.
func (e *GroupStageError) CommandName() string {
	return e.command
}

// Error returns the cause's message, or a fixed line when the receiver is the
// zero value, so a zero GroupStageError never panics while being reported.
func (e *GroupStageError) Error() string {
	if e.err == nil {
		return "group pre-execution failed"
	}
	return e.err.Error()
}

// Unwrap returns the cause so errors.Is and errors.As can traverse the chain.
func (e *GroupStageError) Unwrap() error {
	return e.err
}

// newGroupStageError builds a group-level stage error. It panics on a stage
// that the table does not define as group-level, an out-of-range stage, an
// empty group name or a nil cause: those are caller mistakes, not runtime
// inputs.
func newGroupStageError(stage GroupStage, group string, err error) *GroupStageError {
	def, ok := groupStageDefinitionFor(stage)
	if !ok {
		panic(fmt.Sprintf("newGroupStageError: stage %d is out of range", stage))
	}
	if def.scope != common.ScopeGroup {
		panic(fmt.Sprintf("newGroupStageError: stage %s is not group-level", stage))
	}
	if group == "" {
		panic("newGroupStageError: group name must not be empty")
	}
	if err == nil {
		panic("newGroupStageError: cause must not be nil")
	}
	return &GroupStageError{stage: stage, group: group, err: err}
}

// newCommandStageError builds a command-level stage error. It panics on a
// stage that the table does not define as command-level, an out-of-range
// stage, an empty group or command name, or a nil cause: those are caller
// mistakes, not runtime inputs.
func newCommandStageError(stage GroupStage, group, command string, err error) *GroupStageError {
	def, ok := groupStageDefinitionFor(stage)
	if !ok {
		panic(fmt.Sprintf("newCommandStageError: stage %d is out of range", stage))
	}
	if def.scope != common.ScopeCommand {
		panic(fmt.Sprintf("newCommandStageError: stage %s is not command-level", stage))
	}
	if group == "" {
		panic("newCommandStageError: group name must not be empty")
	}
	if command == "" {
		panic("newCommandStageError: command name must not be empty")
	}
	if err == nil {
		panic("newCommandStageError: cause must not be nil")
	}
	return &GroupStageError{stage: stage, group: group, command: command, err: err}
}

// groupStagePreExecutionError converts a stage error into the notification
// record the logging layer records. The stage selects the error_type, the
// scope level and the summary; FailedFilePaths stays nil because these
// failures carry no target list.
func groupStagePreExecutionError(stageErr *GroupStageError, runID string) *logging.PreExecutionError {
	def, ok := groupStageDefinitionFor(stageErr.Stage())
	if !ok {
		def = groupStageDefinitions[GroupStageUnknown]
	}
	return &logging.PreExecutionError{
		Type:                def.errorType,
		Message:             def.message,
		Component:           string(resource.ComponentRunner),
		RunID:               runID,
		NotificationContext: groupStageNotificationContext(def.scope, stageErr.group, stageErr.command),
		Err:                 stageErr.Unwrap(),
	}
}

// groupStageNotificationContext builds the scope a stage's definition
// declares: a command scope at command level, a group scope otherwise.
func groupStageNotificationContext(scope common.NotificationScope, group, command string) common.NotificationContext {
	if scope == common.ScopeCommand {
		return common.CommandScope(group, command)
	}
	return common.GroupScope(group)
}
