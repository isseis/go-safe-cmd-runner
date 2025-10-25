// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// InheritanceMode represents how environment allowlist inheritance works
type InheritanceMode int

const (
	// InheritanceModeInherit indicates the group inherits from global allowlist
	// This occurs when env_allowlist field is not defined (nil slice)
	InheritanceModeInherit InheritanceMode = iota

	// InheritanceModeExplicit indicates the group uses only its explicit allowlist
	// This occurs when env_allowlist field has values: ["VAR1", "VAR2"]
	InheritanceModeExplicit

	// InheritanceModeReject indicates the group rejects all environment variables
	// This occurs when env_allowlist field is explicitly empty: []
	InheritanceModeReject
)

// LogLevel represents the logging level for the application.
// Valid values: debug, info, warn, error
type LogLevel string

const (
	// LogLevelDebug enables debug-level logging
	LogLevelDebug LogLevel = "debug"

	// LogLevelInfo enables info-level logging (default)
	LogLevelInfo LogLevel = "info"

	// LogLevelWarn enables warning-level logging
	LogLevelWarn LogLevel = "warn"

	// LogLevelError enables error-level logging only
	LogLevelError LogLevel = "error"
)

// ErrInvalidLogLevel is returned when an invalid log level is provided
var ErrInvalidLogLevel = errors.New("invalid log level")

// UnmarshalText implements the encoding.TextUnmarshaler interface.
// This enables validation during TOML parsing.
func (l *LogLevel) UnmarshalText(text []byte) error {
	s := strings.ToLower(string(text))
	switch LogLevel(s) {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		*l = LogLevel(s)
		return nil
	case "":
		// Empty string defaults to info level
		*l = LogLevelInfo
		return nil
	default:
		return fmt.Errorf("%w: %q (must be one of: debug, info, warn, error)", ErrInvalidLogLevel, string(text))
	}
}

// ToSlogLevel converts LogLevel to slog.Level for use with the slog package.
func (l LogLevel) ToSlogLevel() (slog.Level, error) {
	// Handle empty string as default to info (same as UnmarshalText)
	if l == "" {
		return slog.LevelInfo, nil
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(l)); err != nil {
		return slog.Level(0), fmt.Errorf("failed to convert log level %q to slog.Level: %w", l, err)
	}
	return level, nil
}

// String returns the string representation of LogLevel.
func (l LogLevel) String() string {
	return string(l)
}

// RiskLevel represents the security risk level of a command
type RiskLevel int

const (
	// RiskLevelUnknown indicates commands whose risk level cannot be determined
	RiskLevelUnknown RiskLevel = iota

	// RiskLevelLow indicates commands with minimal security risk
	RiskLevelLow

	// RiskLevelMedium indicates commands with moderate security risk
	RiskLevelMedium

	// RiskLevelHigh indicates commands with high security risk
	RiskLevelHigh

	// RiskLevelCritical indicates commands that should be blocked (e.g., privilege escalation)
	RiskLevelCritical
)

// Risk level string constants used for string representation and parsing.
const (
	// UnknownRiskLevelString represents an unknown risk level.
	UnknownRiskLevelString = "unknown"
	// LowRiskLevelString represents a low risk level.
	LowRiskLevelString = "low"
	// MediumRiskLevelString represents a medium risk level.
	MediumRiskLevelString = "medium"
	// HighRiskLevelString represents a high risk level.
	HighRiskLevelString = "high"
	// CriticalRiskLevelString represents a critical risk level that blocks execution.
	CriticalRiskLevelString = "critical"
)

// String returns a string representation of RiskLevel
func (r RiskLevel) String() string {
	switch r {
	case RiskLevelUnknown:
		return UnknownRiskLevelString
	case RiskLevelLow:
		return LowRiskLevelString
	case RiskLevelMedium:
		return MediumRiskLevelString
	case RiskLevelHigh:
		return HighRiskLevelString
	case RiskLevelCritical:
		return CriticalRiskLevelString
	default:
		return UnknownRiskLevelString
	}
}

// ParseRiskLevel converts a string to RiskLevel for user configuration
// Critical level is prohibited in user configuration and reserved for internal use
func ParseRiskLevel(s string) (RiskLevel, error) {
	switch s {
	case UnknownRiskLevelString:
		return RiskLevelUnknown, nil
	case LowRiskLevelString:
		return RiskLevelLow, nil
	case MediumRiskLevelString:
		return RiskLevelMedium, nil
	case HighRiskLevelString:
		return RiskLevelHigh, nil
	case CriticalRiskLevelString:
		return RiskLevelUnknown, fmt.Errorf("%w: critical risk level cannot be set in configuration (reserved for internal use only)", ErrInvalidRiskLevel)
	case "":
		return RiskLevelLow, nil // Default to low risk for empty strings
	default:
		return RiskLevelUnknown, fmt.Errorf("%w: %s (supported: low, medium, high)", ErrInvalidRiskLevel, s)
	}
}

// String returns a string representation of InheritanceMode for logging
func (m InheritanceMode) String() string {
	switch m {
	case InheritanceModeInherit:
		return "inherit"
	case InheritanceModeExplicit:
		return "explicit"
	case InheritanceModeReject:
		return "reject"
	default:
		return "unknown"
	}
}

// AllowlistResolution contains resolved allowlist information for debugging and logging
type AllowlistResolution struct {
	Mode      InheritanceMode
	GroupName string // For logging context

	// groupAllowlistSet and globalAllowlistSet are internal maps for O(1) lookup performance.
	// These are populated from GroupAllowlist and GlobalAllowlist during resolution.
	groupAllowlistSet  map[string]struct{}
	globalAllowlistSet map[string]struct{}

	// Pre-computed effective set for optimization
	effectiveSet map[string]struct{} // Pre-computed effective allowlist set

	// Lazy evaluation caches for getter methods
	groupAllowlistOnce  sync.Once // Ensures groupAllowlistCache is initialized only once
	groupAllowlistCache []string  // Cache for GetGroupAllowlist()

	globalAllowlistOnce  sync.Once // Ensures globalAllowlistCache is initialized only once
	globalAllowlistCache []string  // Cache for GetGlobalAllowlist()

	effectiveListOnce  sync.Once // Ensures effectiveListCache is initialized only once
	effectiveListCache []string  // Cache for GetEffectiveList()
}

// IsAllowed checks if a variable is allowed in the effective allowlist.
// This is the most frequently called method and is optimized for O(1) performance.
//
// Uses pre-computed effectiveSet for optimal performance:
// - computeEffectiveSet() pre-computes Mode-based effective allowlist
// - IsAllowed() only needs to check effectiveSet (faster than mode switching)
//
// Parameters:
//   - variable: environment variable name to check
//
// Returns: true if the variable is allowed, false otherwise
//
// Panics:
//   - if receiver is nil (programming error - caller must check before calling)
//   - if effectiveSet is nil (invariant violation - object not properly initialized)
func (r *AllowlistResolution) IsAllowed(variable string) bool {
	// empty variable name is input validation error - return false
	if variable == "" {
		return false
	}

	// INVARIANT: effectiveSet must be set during initialization
	// If this is nil, it's an invariant violation, so panic
	if r.effectiveSet == nil {
		panic("AllowlistResolution: effectiveSet is nil - object not properly initialized")
	}

	_, allowed := r.effectiveSet[variable]
	return allowed
}

// SetGroupAllowlistSet sets the internal group allowlist map for O(1) lookups.
// This is called during allowlist resolution to populate the lookup map.
func (r *AllowlistResolution) SetGroupAllowlistSet(allowlistSet map[string]struct{}) {
	r.groupAllowlistSet = allowlistSet
}

// SetGlobalAllowlistSet sets the internal global allowlist map for O(1) lookups.
// This is called during allowlist resolution to populate the lookup map.
func (r *AllowlistResolution) SetGlobalAllowlistSet(allowlistSet map[string]struct{}) {
	r.globalAllowlistSet = allowlistSet
}

// GetEffectiveList returns effective allowlist with lazy evaluation for performance.
// Uses cached slice generated from effectiveSet on first access.
// Thread-safe: Uses sync.Once to ensure cache is initialized only once.
func (r *AllowlistResolution) GetEffectiveList() []string {
	// INVARIANT: effectiveSet must be set during initialization
	if r.effectiveSet == nil {
		panic("AllowlistResolution.GetEffectiveList: effectiveSet is nil - object not properly initialized via NewAllowlistResolution")
	}

	r.effectiveListOnce.Do(func() {
		r.effectiveListCache = r.setToSortedSlice(r.effectiveSet)
	})

	return r.effectiveListCache
}

// GetEffectiveSize returns the number of effective allowlist entries.
// Uses effectiveSet directly for O(1) size query.
func (r *AllowlistResolution) GetEffectiveSize() int {
	// INVARIANT: effectiveSet must be set during initialization
	if r.effectiveSet == nil {
		panic("AllowlistResolution.GetEffectiveSize: effectiveSet is nil - object not properly initialized via NewAllowlistResolution")
	}

	return len(r.effectiveSet)
}

// GetGroupAllowlist returns group allowlist with lazy evaluation.
// Uses cached slice generated from groupAllowlistSet on first access.
func (r *AllowlistResolution) GetGroupAllowlist() []string {
	// Thread-safe lazy evaluation and caching using sync.Once
	r.groupAllowlistOnce.Do(func() {
		r.groupAllowlistCache = r.setToSortedSlice(r.groupAllowlistSet)
	})

	return r.groupAllowlistCache
}

// GetGlobalAllowlist returns global allowlist with lazy evaluation.
// Uses cached slice generated from globalAllowlistSet on first access.
func (r *AllowlistResolution) GetGlobalAllowlist() []string {
	// Thread-safe lazy evaluation and caching using sync.Once
	r.globalAllowlistOnce.Do(func() {
		r.globalAllowlistCache = r.setToSortedSlice(r.globalAllowlistSet)
	})

	return r.globalAllowlistCache
}

// GetMode returns the inheritance mode used for this resolution.
func (r *AllowlistResolution) GetMode() InheritanceMode {
	return r.Mode
}

// GetGroupName returns the name of the group this resolution is for.
func (r *AllowlistResolution) GetGroupName() string {
	return r.GroupName
}

// newAllowlistResolution creates a new AllowlistResolution with pre-computed effective set.
// This constructor optimizes performance by pre-computing the effective allowlist.
// This is a private method - external callers should use NewAllowlistResolutionBuilder instead.
//
// Parameters:
//   - mode: inheritance mode (Inherit/Explicit/Reject)
//   - groupName: name of the group this resolution is for
//   - groupSet: group allowlist as a map for O(1) lookups (must not be nil)
//   - globalSet: global allowlist as a map for O(1) lookups (must not be nil)
//
// Returns:
//   - *AllowlistResolution: new instance with effectiveSet pre-computed
//
// Panics:
//   - if groupSet is nil
//   - if globalSet is nil
func newAllowlistResolution(
	mode InheritanceMode,
	groupName string,
	groupSet map[string]struct{},
	globalSet map[string]struct{},
) *AllowlistResolution {
	// Input validation
	if groupSet == nil {
		panic("newAllowlistResolution: groupSet cannot be nil")
	}
	if globalSet == nil {
		panic("newAllowlistResolution: globalSet cannot be nil")
	}

	r := &AllowlistResolution{
		Mode:               mode,
		GroupName:          groupName,
		groupAllowlistSet:  groupSet,
		globalAllowlistSet: globalSet,
	}

	// Pre-compute effectiveSet for optimization
	r.computeEffectiveSet()

	return r
}

// computeEffectiveSet calculates the effective allowlist based on inheritance mode.
// This method establishes the invariant that effectiveSet is never nil after calling.
//
// Invariants established:
// - After calling, effectiveSet is guaranteed to be non-nil
// - groupAllowlistSet and globalAllowlistSet must be non-nil (precondition)
//
// Panics:
//   - if groupAllowlistSet is nil
//   - if globalAllowlistSet is nil
func (r *AllowlistResolution) computeEffectiveSet() {
	// Invariant preconditions: groupAllowlistSet and globalAllowlistSet must not be nil
	if r.groupAllowlistSet == nil {
		panic("AllowlistResolution: groupAllowlistSet is nil - cannot compute effective set")
	}
	if r.globalAllowlistSet == nil {
		panic("AllowlistResolution: globalAllowlistSet is nil - cannot compute effective set")
	}

	switch r.Mode {
	case InheritanceModeInherit:
		// Use global allowlist directly (zero-copy reference)
		r.effectiveSet = r.globalAllowlistSet

	case InheritanceModeExplicit:
		// Use group allowlist directly (zero-copy reference)
		r.effectiveSet = r.groupAllowlistSet

	case InheritanceModeReject:
		// Empty set (not nil, but empty map)
		r.effectiveSet = make(map[string]struct{})

	default:
		// Default to inherit mode
		r.effectiveSet = r.globalAllowlistSet
	}

	// POST-CONDITION: effectiveSet must not be nil
	if r.effectiveSet == nil {
		panic("AllowlistResolution: internal error - effectiveSet is still nil after computeEffectiveSet()")
	}
}

// setToSortedSlice converts a set (map[string]struct{}) to a sorted string slice.
// This helper method is used by getter methods for consistent ordering.
//
// Parameters:
//   - set: the map to convert to a slice
//
// Returns:
//   - []string: sorted slice of keys from the map
func (r *AllowlistResolution) setToSortedSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return []string{}
	}

	slice := make([]string, 0, len(set))
	for variable := range set {
		slice = append(slice, variable)
	}

	sort.Strings(slice)
	return slice
}

// Operation represents different types of privileged operations
type Operation string

// Supported privileged operations
const (
	OperationFileHashCalculation Operation = "file_hash_calculation"
	OperationCommandExecution    Operation = "command_execution"
	OperationUserGroupExecution  Operation = "user_group_execution"
	OperationUserGroupDryRun     Operation = "user_group_dry_run"
	OperationFileAccess          Operation = "file_access"
	OperationFileValidation      Operation = "file_validation" // For file integrity validation
	OperationHealthCheck         Operation = "health_check"
)

// ElevationContext contains context information for privilege elevation
type ElevationContext struct {
	Operation   Operation
	CommandName string
	FilePath    string
	OriginalUID int
	TargetUID   int
	// User/group privilege change fields
	RunAsUser  string
	RunAsGroup string
}

// Standard privilege errors
var (
	ErrPrivilegedExecutionNotAvailable  = fmt.Errorf("privileged execution not available: binary lacks required SUID bit or running as non-root user")
	ErrInvalidRiskLevel                 = errors.New("invalid risk level")
	ErrPrivilegeEscalationBlocked       = errors.New("privilege escalation command blocked for security")
	ErrCriticalRiskBlocked              = errors.New("critical risk command execution blocked")
	ErrCommandSecurityViolation         = errors.New("command security violation: risk level too high")
	ErrInvalidEnvironmentVariableFormat = errors.New("invalid environment variable format")
	ErrDuplicateEnvironmentVariable     = errors.New("duplicate environment variable")
)

// PrivilegeManager interface defines methods for privilege management
type PrivilegeManager interface {
	IsPrivilegedExecutionSupported() bool
	WithPrivileges(elevationCtx ElevationContext, fn func() error) error

	// Enhanced privilege management for user/group specification
	WithUserGroup(user, group string, fn func() error) error
	IsUserGroupSupported() bool
}

// AllowlistResolutionBuilder provides a fluent interface for creating AllowlistResolution.
//
// The builder supports two input formats:
//   - Slice-based: WithGroupVariables for []string input
//   - Set-based: WithGlobalVariablesSet for map[string]struct{} input
//
// Set-based methods are more efficient when the caller already has data in map form,
// avoiding unnecessary map -> slice -> map conversions.
//
// Example usage:
//
//	resolution := NewAllowlistResolutionBuilder().
//	    WithMode(InheritanceModeExplicit).
//	    WithGroupName("build").
//	    WithGroupVariables([]string{"PATH", "HOME"}).
//	    WithGlobalVariablesSet(globalSet).
//	    Build()
type AllowlistResolutionBuilder struct {
	mode      InheritanceMode
	groupName string
	groupVars []string
	globalSet map[string]struct{}
}

// NewAllowlistResolutionBuilder creates a new builder with default values.
// Default mode is InheritanceModeInherit.
func NewAllowlistResolutionBuilder() *AllowlistResolutionBuilder {
	return &AllowlistResolutionBuilder{
		mode: InheritanceModeInherit, // default to inherit mode
	}
}

// WithMode sets the inheritance mode for the resolution.
// Returns the builder for method chaining.
func (b *AllowlistResolutionBuilder) WithMode(mode InheritanceMode) *AllowlistResolutionBuilder {
	b.mode = mode
	return b
}

// WithGroupName sets the group name for the resolution.
// Returns the builder for method chaining.
func (b *AllowlistResolutionBuilder) WithGroupName(name string) *AllowlistResolutionBuilder {
	b.groupName = name
	return b
}

// WithGroupVariables sets the group-specific variables for the resolution.
// Returns the builder for method chaining.
func (b *AllowlistResolutionBuilder) WithGroupVariables(vars []string) *AllowlistResolutionBuilder {
	b.groupVars = vars
	return b
}

// WithGlobalVariablesSet sets the global variables using a pre-built set.
// Returns the builder for method chaining.
func (b *AllowlistResolutionBuilder) WithGlobalVariablesSet(globalSet map[string]struct{}) *AllowlistResolutionBuilder {
	b.globalSet = globalSet
	return b
}

// Build creates the AllowlistResolution with the configured settings.
//
// Returns:
//   - *AllowlistResolution: newly created resolution with pre-computed effective set
//
// Panics:
//   - if newAllowlistResolution panics (e.g., nil sets passed)
func (b *AllowlistResolutionBuilder) Build() *AllowlistResolution {
	// Convert group variables slice to set
	groupSet := common.SliceToSet(b.groupVars)

	// Use global set if provided, otherwise create empty set
	var globalSet map[string]struct{}
	if b.globalSet != nil {
		globalSet = b.globalSet
	} else {
		globalSet = make(map[string]struct{})
	}

	return newAllowlistResolution(b.mode, b.groupName, groupSet, globalSet)
}
