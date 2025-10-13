// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// Config represents the root configuration structure
type Config struct {
	Version string         `toml:"version"`
	Global  GlobalConfig   `toml:"global"`
	Groups  []CommandGroup `toml:"groups"`
}

// GlobalConfig contains global configuration options
type GlobalConfig struct {
	Timeout           int      `toml:"timeout"`             // Global timeout in seconds
	WorkDir           string   `toml:"workdir"`             // Working directory
	LogLevel          string   `toml:"log_level"`           // Log level (debug, info, warn, error)
	VerifyFiles       []string `toml:"verify_files"`        // Files to verify at global level
	SkipStandardPaths bool     `toml:"skip_standard_paths"` // Skip verification for standard system paths
	EnvAllowlist      []string `toml:"env_allowlist"`       // Global environment variable allowlist
	MaxOutputSize     int64    `toml:"max_output_size"`     // Default output size limit in bytes
	Env               []string `toml:"env"`                 // Global environment variables (KEY=VALUE format)

	// ExpandedVerifyFiles contains the verify_files paths with environment variable substitutions applied.
	// It is the expanded version of the VerifyFiles field, populated during configuration loading
	// and used during file verification to avoid re-expanding for each verification.
	// The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedVerifyFiles []string `toml:"-"`

	// ExpandedEnv contains the global environment variables with all variable substitutions applied.
	// It is the expanded version of the Env field, populated during configuration loading
	// and used during command execution to avoid re-expanding Global.Env for each command.
	// The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedEnv map[string]string `toml:"-"`
}

// CommandGroup represents a group of related commands with a name
type CommandGroup struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Priority    int    `toml:"priority"`

	// Fields for resource management
	TempDir bool   `toml:"temp_dir"` // Auto-generate temporary directory
	WorkDir string `toml:"workdir"`  // Working directory

	Commands     []Command `toml:"commands"`
	VerifyFiles  []string  `toml:"verify_files"`  // Files to verify for this group
	EnvAllowlist []string  `toml:"env_allowlist"` // Group-level environment variable allowlist
	Env          []string  `toml:"env"`           // Group-level environment variables (KEY=VALUE format)

	// ExpandedVerifyFiles contains the verify_files paths with environment variable substitutions applied.
	// It is the expanded version of the VerifyFiles field, populated during configuration loading
	// and used during file verification to avoid re-expanding for each verification.
	// The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedVerifyFiles []string `toml:"-"`

	// ExpandedEnv contains the group environment variables with all variable substitutions applied.
	// It is the expanded version of the Env field, populated during configuration loading
	// and used during command execution to avoid re-expanding Group.Env for each command.
	// The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedEnv map[string]string `toml:"-"`
}

// Command represents a single command to be executed
type Command struct {
	Name         string   `toml:"name"`
	Description  string   `toml:"description"`
	Cmd          string   `toml:"cmd"`
	Args         []string `toml:"args"`
	Env          []string `toml:"env"`
	Dir          string   `toml:"dir"`
	Timeout      int      `toml:"timeout"`        // Command-specific timeout (overrides global)
	RunAsUser    string   `toml:"run_as_user"`    // User to execute command as (using seteuid)
	RunAsGroup   string   `toml:"run_as_group"`   // Group to execute command as (using setegid)
	MaxRiskLevel string   `toml:"max_risk_level"` // Maximum allowed risk level (low, medium, high)
	Output       string   `toml:"output"`         // Standard output file path for capture

	// ExpandedCmd contains the command path with environment variable substitutions applied.
	// It is the expanded version of the Cmd field, populated during configuration loading
	// (Phase 1) and used during command execution (Phase 2) to avoid re-expanding Command.Cmd
	// for each execution. The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedCmd string `toml:"-"`

	// ExpandedArgs contains the command arguments with environment variable substitutions applied.
	// It is the expanded version of the Args field, populated during configuration loading
	// (Phase 1) and used during command execution (Phase 2) to avoid re-expanding Command.Args
	// for each execution. The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedArgs []string `toml:"-"`

	// ExpandedEnv contains the environment variables with all variable substitutions applied.
	// It is the expanded version of the Env field, populated during configuration loading
	// (Phase 1) and used during command execution (Phase 2) to avoid re-expanding Command.Env
	// for each execution. The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedEnv map[string]string `toml:"-"`
}

// GetMaxRiskLevel returns the parsed maximum risk level for this command
func (c *Command) GetMaxRiskLevel() (RiskLevel, error) {
	return ParseRiskLevel(c.MaxRiskLevel)
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified
func (c *Command) HasUserGroupSpecification() bool {
	return c.RunAsUser != "" || c.RunAsGroup != ""
}

// BuildEnvironmentMap builds a map of environment variables from the command's Env slice.
// This is used for variable expansion processing.
func (c *Command) BuildEnvironmentMap() (map[string]string, error) {
	env := make(map[string]string)

	for _, envVar := range c.Env {
		key, value, ok := common.ParseEnvVariable(envVar)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrInvalidEnvironmentVariableFormat, envVar)
		}
		if _, exists := env[key]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateEnvironmentVariable, key)
		}
		env[key] = value
	}

	return env, nil
}

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
	Mode            InheritanceMode
	GroupAllowlist  []string
	GlobalAllowlist []string
	EffectiveList   []string // The actual allowlist being used
	GroupName       string   // For logging context

	// groupAllowlistSet and globalAllowlistSet are internal maps for O(1) lookup performance.
	// These are populated from GroupAllowlist and GlobalAllowlist during resolution.
	groupAllowlistSet  map[string]struct{}
	globalAllowlistSet map[string]struct{}

	// Phase 2 additions: Pre-computed effective set for optimization
	effectiveSet map[string]struct{} // Pre-computed effective allowlist set

	// Phase 2 additions: Lazy evaluation caches for getter methods
	groupAllowlistCache  []string // Cache for GetGroupAllowlist()
	globalAllowlistCache []string // Cache for GetGlobalAllowlist()
	effectiveListCache   []string // Cache for GetEffectiveList()

	// Thread-safe lazy initialization
	groupAllowlistOnce  sync.Once // Ensures groupAllowlistCache is initialized only once
	globalAllowlistOnce sync.Once // Ensures globalAllowlistCache is initialized only once
	effectiveListOnce   sync.Once // Ensures effectiveListCache is initialized only once
}

// IsAllowed checks if a variable is allowed in the effective allowlist.
// This is the most frequently called method and is optimized for O(1) performance.
//
// Phase 2 implementation: Uses pre-computed effectiveSet for optimal performance
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
// Phase 2 implementation: Uses cached slice generated from effectiveSet on first access.
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
// Phase 2 optimization: Uses effectiveSet directly for O(1) size query.
func (r *AllowlistResolution) GetEffectiveSize() int {
	// INVARIANT: effectiveSet must be set during initialization
	if r.effectiveSet == nil {
		panic("AllowlistResolution.GetEffectiveSize: effectiveSet is nil - object not properly initialized via NewAllowlistResolution")
	}

	return len(r.effectiveSet)
}

// GetGroupAllowlist returns group allowlist with lazy evaluation.
// Phase 2 implementation: Uses cached slice generated from groupAllowlistSet on first access.
func (r *AllowlistResolution) GetGroupAllowlist() []string {
	// Thread-safe lazy evaluation and caching using sync.Once
	r.groupAllowlistOnce.Do(func() {
		r.groupAllowlistCache = r.setToSortedSlice(r.groupAllowlistSet)
	})

	return r.groupAllowlistCache
}

// GetGlobalAllowlist returns global allowlist with lazy evaluation.
// Phase 2 implementation: Uses cached slice generated from globalAllowlistSet on first access.
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
// Phase 2 constructor that optimizes performance by pre-computing the effective allowlist.
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

		// Phase 2: slice fields will be lazily generated
		GroupAllowlist:  []string{},
		GlobalAllowlist: []string{},
		EffectiveList:   []string{},
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
// Available in Phase 2 and later.
//
// The builder supports two input formats:
//   - Slice-based: WithGroupVariables/WithGlobalVariables for []string input
//   - Set-based: WithGroupVariablesSet/WithGlobalVariablesSet for map[string]struct{} input
//
// Set-based methods are more efficient when the caller already has data in map form,
// avoiding unnecessary map -> slice -> map conversions.
//
// Example usage with slices:
//
//	resolution := NewAllowlistResolutionBuilder().
//	    WithMode(InheritanceModeExplicit).
//	    WithGroupName("build").
//	    WithGroupVariables([]string{"PATH", "HOME"}).
//	    WithGlobalVariables([]string{"USER", "SHELL"}).
//	    Build()
//
// Example usage with sets (more efficient):
//
//	resolution := NewAllowlistResolutionBuilder().
//	    WithMode(InheritanceModeExplicit).
//	    WithGroupName("build").
//	    WithGroupVariablesSet(groupSet).
//	    WithGlobalVariablesSet(globalSet).
//	    Build()
type AllowlistResolutionBuilder struct {
	mode       InheritanceMode
	groupName  string
	groupVars  []string
	globalVars []string
	groupSet   map[string]struct{}
	globalSet  map[string]struct{}
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

// WithGlobalVariables sets the global variables for the resolution.
// Returns the builder for method chaining.
func (b *AllowlistResolutionBuilder) WithGlobalVariables(vars []string) *AllowlistResolutionBuilder {
	b.globalVars = vars
	return b
}

// WithGroupVariablesSet sets the group-specific variables using a pre-built set.
// This is more efficient than WithGroupVariables when the caller already has a map.
// If both WithGroupVariables and WithGroupVariablesSet are called, the set takes precedence.
// Returns the builder for method chaining.
func (b *AllowlistResolutionBuilder) WithGroupVariablesSet(set map[string]struct{}) *AllowlistResolutionBuilder {
	b.groupSet = set
	return b
}

// WithGlobalVariablesSet sets the global variables using a pre-built set.
// This is more efficient than WithGlobalVariables when the caller already has a map.
// If both WithGlobalVariables and WithGlobalVariablesSet are called, the set takes precedence.
// Returns the builder for method chaining.
func (b *AllowlistResolutionBuilder) WithGlobalVariablesSet(set map[string]struct{}) *AllowlistResolutionBuilder {
	b.globalSet = set
	return b
}

// Build creates the AllowlistResolution with the configured settings.
// The builder accepts either slice-based or set-based inputs, but not both for the same field.
// If both are provided for the same field, this method panics to catch programming errors early.
//
// Returns:
//   - *AllowlistResolution: newly created resolution with pre-computed effective set
//
// Panics:
//   - if both WithGroupVariables and WithGroupVariablesSet were called (programming error)
//   - if both WithGlobalVariables and WithGlobalVariablesSet were called (programming error)
//   - if newAllowlistResolution panics (e.g., nil sets passed)
func (b *AllowlistResolutionBuilder) Build() *AllowlistResolution {
	// Detect conflicting configurations (both slice and set provided for same field)
	if b.groupVars != nil && b.groupSet != nil {
		panic("AllowlistResolutionBuilder: both WithGroupVariables and WithGroupVariablesSet were called - use only one")
	}
	if b.globalVars != nil && b.globalSet != nil {
		panic("AllowlistResolutionBuilder: both WithGlobalVariables and WithGlobalVariablesSet were called - use only one")
	}

	// Use provided set if available, otherwise convert slice to set
	var groupSet map[string]struct{}
	if b.groupSet != nil {
		groupSet = b.groupSet
	} else {
		groupSet = common.SliceToSet(b.groupVars)
	}

	var globalSet map[string]struct{}
	if b.globalSet != nil {
		globalSet = b.globalSet
	} else {
		globalSet = common.SliceToSet(b.globalVars)
	}

	return newAllowlistResolution(b.mode, b.groupName, groupSet, globalSet)
}

// NewTestAllowlistResolutionSimple creates a simple AllowlistResolution for basic testing.
// Uses InheritanceModeInherit by default with "test-group" as the group name.
//
// Parameters:
//   - globalVars: global environment variables for the allowlist
//   - groupVars: group-specific environment variables for the allowlist
//
// Returns: *AllowlistResolution configured with Inherit mode and "test-group" name
func NewTestAllowlistResolutionSimple(
	globalVars []string,
	groupVars []string,
) *AllowlistResolution {
	return NewAllowlistResolutionBuilder().
		WithMode(InheritanceModeInherit).
		WithGroupName("test-group").
		WithGlobalVariables(globalVars).
		WithGroupVariables(groupVars).
		Build()
}

// NewTestAllowlistResolutionWithMode creates AllowlistResolution with specific inheritance mode.
// Supports all current inheritance modes: Inherit, Explicit, Reject.
// Uses "test-group" as the group name.
//
// Parameters:
//   - mode: the inheritance mode to use
//   - globalVars: global environment variables for the allowlist
//   - groupVars: group-specific environment variables for the allowlist
//
// Returns: *AllowlistResolution configured with the specified mode and "test-group" name
func NewTestAllowlistResolutionWithMode(
	mode InheritanceMode,
	globalVars []string,
	groupVars []string,
) *AllowlistResolution {
	return NewAllowlistResolutionBuilder().
		WithMode(mode).
		WithGroupName("test-group").
		WithGlobalVariables(globalVars).
		WithGroupVariables(groupVars).
		Build()
}
