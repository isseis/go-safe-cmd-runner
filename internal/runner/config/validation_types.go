package config

import "time"

// ValidationResult contains the results of configuration validation
type ValidationResult struct {
	Valid     bool                `json:"valid"`
	Errors    []ValidationError   `json:"errors"`
	Warnings  []ValidationWarning `json:"warnings"`
	Summary   ValidationSummary   `json:"summary"`
	Timestamp time.Time           `json:"timestamp"`
}

// ValidationError represents a configuration error that prevents operation
type ValidationError struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Location string `json:"location"` // e.g., "groups[0].env_allowlist"
	Severity string `json:"severity"`
}

// ValidationWarning represents a configuration issue that might cause problems
type ValidationWarning struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	Location   string `json:"location"`
	Suggestion string `json:"suggestion"`
}

// ValidationSummary provides a high-level overview of validation results
type ValidationSummary struct {
	TotalGroups         int `json:"total_groups"`
	GroupsWithAllowlist int `json:"groups_with_allowlist"`
	GlobalAllowlistSize int `json:"global_allowlist_size"`
	TotalCommands       int `json:"total_commands"`
	CommandsWithEnv     int `json:"commands_with_env"`
}
