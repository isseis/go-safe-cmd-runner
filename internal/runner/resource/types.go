package resource

import (
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ResourceAnalysis captures analysis of a resource operation
// nolint:revive // ResourceAnalysis is intentionally named to be clear about its purpose
type ResourceAnalysis struct {
	Type       ResourceType      `json:"type"`
	Operation  ResourceOperation `json:"operation"`
	Target     string            `json:"target"`
	Parameters map[string]any    `json:"parameters"`
	Impact     ResourceImpact    `json:"impact"`
	Timestamp  time.Time         `json:"timestamp"`
}

// ResourceType represents the type of resource being operated on
// nolint:revive // ResourceType is intentionally named to be clear about its purpose
type ResourceType string

const (
	// ResourceTypeCommand represents command execution
	ResourceTypeCommand ResourceType = "command"
	// ResourceTypeFilesystem represents filesystem operations
	ResourceTypeFilesystem ResourceType = "filesystem"
	// ResourceTypePrivilege represents privilege management
	ResourceTypePrivilege ResourceType = "privilege"
	// ResourceTypeNetwork represents network operations
	ResourceTypeNetwork ResourceType = "network"
	// ResourceTypeProcess represents process management
	ResourceTypeProcess ResourceType = "process"
)

// String returns the string representation of ResourceType
func (r ResourceType) String() string {
	return string(r)
}

// ResourceOperation represents the operation being performed
// nolint:revive // ResourceOperation is intentionally named to be clear about its purpose
type ResourceOperation string

const (
	// OperationCreate represents a create operation
	OperationCreate ResourceOperation = "create"
	// OperationDelete represents a delete operation
	OperationDelete ResourceOperation = "delete"
	// OperationExecute represents an execute operation
	OperationExecute ResourceOperation = "execute"
	// OperationEscalate represents a privilege escalation operation
	OperationEscalate ResourceOperation = "escalate"
	// OperationSend represents a send operation (e.g., notifications)
	OperationSend ResourceOperation = "send"
)

// String returns the string representation of ResourceOperation
func (r ResourceOperation) String() string {
	return string(r)
}

// ResourceImpact describes the impact of a resource operation
// nolint:revive // ResourceImpact is intentionally named to be clear about its purpose
type ResourceImpact struct {
	Reversible   bool   `json:"reversible"`
	Persistent   bool   `json:"persistent"`
	SecurityRisk string `json:"security_risk,omitempty"`
	Description  string `json:"description"`
}

// DryRunOptions holds options for dry-run execution
type DryRunOptions struct {
	DetailLevel      DetailLevel  `json:"detail_level"`
	OutputFormat     OutputFormat `json:"output_format"`
	ShowSensitive    bool         `json:"show_sensitive"`
	VerifyFiles      bool         `json:"verify_files"`
	ShowTimings      bool         `json:"show_timings"`
	ShowDependencies bool         `json:"show_dependencies"`
	MaxDepth         int          `json:"max_depth"` // Maximum depth for variable resolution

	// Security analysis configuration
	VerifyStandardPaths bool   `json:"verify_standard_paths"` // Perform hash validation for standard system paths
	HashDir             string `json:"hash_dir"`              // Directory containing hash files
}

// DetailLevel represents the level of detail in output
type DetailLevel int

const (
	// DetailLevelSummary shows only summary information
	DetailLevelSummary DetailLevel = iota
	// DetailLevelDetailed shows detailed information
	DetailLevelDetailed
	// DetailLevelFull shows full information including debug details
	DetailLevelFull
)

// String returns the string representation of DetailLevel
func (d DetailLevel) String() string {
	switch d {
	case DetailLevelSummary:
		return "summary"
	case DetailLevelDetailed:
		return "detailed"
	case DetailLevelFull:
		return "full"
	default:
		return unknownString
	}
}

// OutputFormat represents the output format
type OutputFormat int

const (
	// OutputFormatText represents plain text output
	OutputFormatText OutputFormat = iota
	// OutputFormatJSON represents JSON output
	OutputFormatJSON
)

// String returns the string representation of OutputFormat
func (o OutputFormat) String() string {
	switch o {
	case OutputFormatText:
		return "text"
	case OutputFormatJSON:
		return "json"
	default:
		return unknownString
	}
}

// DryRunResult represents the complete result of a dry-run analysis
type DryRunResult struct {
	Metadata         *ResultMetadata    `json:"metadata"`
	ResourceAnalyses []ResourceAnalysis `json:"resource_analyses"`
	SecurityAnalysis *SecurityAnalysis  `json:"security_analysis"`
	EnvironmentInfo  *EnvironmentInfo   `json:"environment_info"`
	Errors           []DryRunError      `json:"errors"`
	Warnings         []DryRunWarning    `json:"warnings"`
}

// ResultMetadata contains metadata about the dry-run result
type ResultMetadata struct {
	GeneratedAt     time.Time     `json:"generated_at"`
	RunID           string        `json:"run_id"`
	ConfigPath      string        `json:"config_path"`
	EnvironmentFile string        `json:"environment_file"`
	Version         string        `json:"version"`
	Duration        time.Duration `json:"duration"`
}

// ResolvedCommand represents a fully resolved command
type ResolvedCommand struct {
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	CommandLine     string         `json:"command_line"`     // After variable expansion
	OriginalCommand string         `json:"original_command"` // Original template
	WorkingDir      string         `json:"working_dir"`
	Timeout         time.Duration  `json:"timeout"`
	RequiredUser    string         `json:"required_user"` // root, current, etc.
	PrivilegeInfo   PrivilegeInfo  `json:"privilege_info"`
	OutputSettings  OutputSettings `json:"output_settings"`
}

// PrivilegeInfo contains information about privilege requirements
type PrivilegeInfo struct {
	RequiresPrivilege bool     `json:"requires_privilege"`
	TargetUser        string   `json:"target_user"`
	Capabilities      []string `json:"capabilities"`
	Risks             []string `json:"risks"`
}

// OutputSettings contains command output settings
type OutputSettings struct {
	CaptureStdout bool `json:"capture_stdout"`
	CaptureStderr bool `json:"capture_stderr"`
}

// SecurityAnalysis contains security analysis results
type SecurityAnalysis struct {
	Risks             []SecurityRisk      `json:"risks"`
	PrivilegeChanges  []PrivilegeChange   `json:"privilege_changes"`
	EnvironmentAccess []EnvironmentAccess `json:"environment_access"`
	FileAccess        []FileAccess        `json:"file_access"`
}

// SecurityRisk represents a security risk
type SecurityRisk struct {
	Level       runnertypes.RiskLevel `json:"level"`
	Type        RiskType              `json:"type"`
	Description string                `json:"description"`
	Command     string                `json:"command"`
	Group       string                `json:"group"`
	Mitigation  string                `json:"mitigation"`
}

// RiskType represents the type of security risk
type RiskType string

const (
	// RiskTypePrivilegeEscalation represents privilege escalation risks
	RiskTypePrivilegeEscalation RiskType = "privilege_escalation"
	// RiskTypeDangerousCommand represents dangerous command risks
	RiskTypeDangerousCommand RiskType = "dangerous_command"
	// RiskTypeDataExposure represents data exposure risks
	RiskTypeDataExposure RiskType = "data_exposure"
)

// PrivilegeChange represents a privilege change
type PrivilegeChange struct {
	Group     string `json:"group"`
	Command   string `json:"command"`
	FromUser  string `json:"from_user"`
	ToUser    string `json:"to_user"`
	Mechanism string `json:"mechanism"`
}

// EnvironmentAccess represents environment variable access
type EnvironmentAccess struct {
	Variable   string   `json:"variable"`
	AccessType string   `json:"access_type"` // read, write
	Commands   []string `json:"commands"`
	Groups     []string `json:"groups"`
	Sensitive  bool     `json:"sensitive"`
}

// FileAccess represents file access
type FileAccess struct {
	Path       string   `json:"path"`
	AccessType string   `json:"access_type"` // read, write, execute
	Commands   []string `json:"commands"`
	Groups     []string `json:"groups"`
}

// EnvironmentInfo contains information about the environment
type EnvironmentInfo struct {
	TotalVariables    int                 `json:"total_variables"`
	AllowedVariables  []string            `json:"allowed_variables"`
	FilteredVariables []string            `json:"filtered_variables"`
	VariableUsage     map[string][]string `json:"variable_usage"` // variable -> commands
}

// DryRunError represents an error that occurred during dry-run
type DryRunError struct {
	Type        ErrorType   `json:"type"`
	Code        string      `json:"code"`
	Message     string      `json:"message"`
	Component   string      `json:"component"`
	Group       string      `json:"group,omitempty"`
	Command     string      `json:"command,omitempty"`
	Details     interface{} `json:"details,omitempty"`
	Recoverable bool        `json:"recoverable"`
}

// ErrorType represents the type of error
type ErrorType string

const (
	// ErrorTypeConfigurationError represents configuration errors
	ErrorTypeConfigurationError ErrorType = "configuration_error"
	// ErrorTypeVerificationError represents verification errors
	ErrorTypeVerificationError ErrorType = "verification_error"
	// ErrorTypeVariableError represents variable resolution errors
	ErrorTypeVariableError ErrorType = "variable_error"
	// ErrorTypeSecurityError represents security errors
	ErrorTypeSecurityError ErrorType = "security_error"
	// ErrorTypeSystemError represents system errors
	ErrorTypeSystemError ErrorType = "system_error"
)

// DryRunWarning represents a warning that occurred during dry-run
type DryRunWarning struct {
	Type      WarningType `json:"type"`
	Message   string      `json:"message"`
	Component string      `json:"component"`
	Group     string      `json:"group,omitempty"`
	Command   string      `json:"command,omitempty"`
}

// WarningType represents the type of warning
type WarningType string

const (
	// WarningTypeDeprecatedFeature represents deprecated feature warnings
	WarningTypeDeprecatedFeature WarningType = "deprecated_feature"
	// WarningTypeSecurityConcern represents security concern warnings
	WarningTypeSecurityConcern WarningType = "security_concern"
	// WarningTypePerformanceConcern represents performance concern warnings
	WarningTypePerformanceConcern WarningType = "performance_concern"
	// WarningTypeCompatibility represents compatibility warnings
	WarningTypeCompatibility WarningType = "compatibility"
)
