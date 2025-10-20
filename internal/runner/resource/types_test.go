//go:build test
// +build test

package resource

import (
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestResourceTypeString(t *testing.T) {
	tests := []struct {
		rType    ResourceType
		expected string
	}{
		{ResourceTypeCommand, "command"},
		{ResourceTypeFilesystem, "filesystem"},
		{ResourceTypePrivilege, "privilege"},
		{ResourceTypeNetwork, "network"},
		{ResourceTypeProcess, "process"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.rType.String())
	}
}

func TestResourceOperationString(t *testing.T) {
	tests := []struct {
		op       ResourceOperation
		expected string
	}{
		{OperationCreate, "create"},
		{OperationDelete, "delete"},
		{OperationExecute, "execute"},
		{OperationEscalate, "escalate"},
		{OperationSend, "send"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.op.String())
	}
}

func TestDetailLevelString(t *testing.T) {
	tests := []struct {
		level    DetailLevel
		expected string
	}{
		{DetailLevelSummary, "summary"},
		{DetailLevelDetailed, "detailed"},
		{DetailLevelFull, "full"},
		{DetailLevel(999), "unknown"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.level.String())
	}
}

func TestOutputFormatString(t *testing.T) {
	tests := []struct {
		format   OutputFormat
		expected string
	}{
		{OutputFormatText, "text"},
		{OutputFormatJSON, "json"},
		{OutputFormat(999), "unknown"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.format.String())
	}
}

func TestResourceAnalysis(t *testing.T) {
	timestamp := time.Now()
	analysis := &ResourceAnalysis{
		Type:      ResourceTypeCommand,
		Operation: OperationExecute,
		Target:    "echo test",
		Parameters: map[string]any{
			"timeout": 30,
		},
		Impact: ResourceImpact{
			Reversible:  true,
			Persistent:  false,
			Description: "Test command execution",
		},
		Timestamp: timestamp,
	}

	assert.Equal(t, ResourceTypeCommand, analysis.Type)
	assert.Equal(t, OperationExecute, analysis.Operation)
	assert.Equal(t, "echo test", analysis.Target)
	assert.Equal(t, 30, analysis.Parameters["timeout"])
	assert.True(t, analysis.Impact.Reversible)
	assert.False(t, analysis.Impact.Persistent)
	assert.Equal(t, timestamp, analysis.Timestamp)
}

func TestDryRunOptions(t *testing.T) {
	opts := &DryRunOptions{
		DetailLevel:      DetailLevelFull,
		OutputFormat:     OutputFormatJSON,
		ShowSensitive:    false,
		VerifyFiles:      true,
		ShowTimings:      true,
		ShowDependencies: true,
		MaxDepth:         5,
	}

	assert.Equal(t, DetailLevelFull, opts.DetailLevel)
	assert.Equal(t, OutputFormatJSON, opts.OutputFormat)
	assert.False(t, opts.ShowSensitive)
	assert.True(t, opts.VerifyFiles)
	assert.True(t, opts.ShowTimings)
	assert.True(t, opts.ShowDependencies)
	assert.Equal(t, 5, opts.MaxDepth)
}

func TestRiskLevelString(t *testing.T) {
	tests := []struct {
		level    runnertypes.RiskLevel
		expected string
	}{
		{runnertypes.RiskLevelLow, "low"},
		{runnertypes.RiskLevelMedium, "medium"},
		{runnertypes.RiskLevelHigh, "high"},
		{runnertypes.RiskLevelCritical, "critical"},
		{runnertypes.RiskLevel(999), "unknown"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.level.String())
	}
}

func TestDryRunResult(t *testing.T) {
	result := &DryRunResult{
		Metadata: &ResultMetadata{
			GeneratedAt:     time.Now(),
			RunID:           "test-run-1",
			ConfigPath:      "/path/to/config.toml",
			EnvironmentFile: "/path/to/.env",
			Version:         "1.0.0",
			Duration:        time.Second * 5,
		},
		ResourceAnalyses: []ResourceAnalysis{},
		SecurityAnalysis: &SecurityAnalysis{
			Risks:             []SecurityRisk{},
			PrivilegeChanges:  []PrivilegeChange{},
			EnvironmentAccess: []EnvironmentAccess{},
			FileAccess:        []FileAccess{},
		},
		EnvironmentInfo: &EnvironmentInfo{
			TotalVariables:    0,
			AllowedVariables:  []string{},
			FilteredVariables: []string{},
			VariableUsage:     map[string][]string{},
		},
		Errors:   []DryRunError{},
		Warnings: []DryRunWarning{},
	}

	assert.NotNil(t, result.Metadata)
	assert.NotNil(t, result.SecurityAnalysis)
	assert.NotNil(t, result.EnvironmentInfo)
	assert.Equal(t, "test-run-1", result.Metadata.RunID)
}
