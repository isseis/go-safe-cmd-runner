//go:build test

package debuginfo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestCollectInheritanceAnalysis(t *testing.T) {
	tests := []struct {
		name             string
		runtimeGlobal    *runnertypes.RuntimeGlobal
		runtimeGroup     *runnertypes.RuntimeGroup
		detailLevel      resource.DryRunDetailLevel
		expectedAnalysis *resource.InheritanceAnalysis
	}{
		{
			name: "DetailLevelSummary returns nil",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{
					EnvImport:  []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				Spec:                        &runnertypes.GroupSpec{},
				EnvAllowlistInheritanceMode: runnertypes.InheritanceModeInherit,
			},
			detailLevel:      resource.DetailLevelSummary,
			expectedAnalysis: nil,
		},
		{
			name: "DetailLevelDetailed - basic fields only",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{
					EnvImport:  []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH", "HOME"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				Spec:                        &runnertypes.GroupSpec{},
				EnvAllowlistInheritanceMode: runnertypes.InheritanceModeInherit,
			},
			detailLevel: resource.DetailLevelDetailed,
			expectedAnalysis: &resource.InheritanceAnalysis{
				GlobalEnvImport: []string{"db_host=DB_HOST"},
				GlobalAllowlist: []string{"PATH", "HOME"},
				GroupEnvImport:  []string{},
				GroupAllowlist:  []string{},
				InheritanceMode: runnertypes.InheritanceModeInherit,
				// Difference fields should be nil (not populated for DetailLevelDetailed)
				InheritedVariables:            nil,
				RemovedAllowlistVariables:     nil,
				UnavailableEnvImportVariables: nil,
			},
		},
		{
			name: "DetailLevelFull - all fields including differences (InheritanceModeExplicit)",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{
					EnvImport:  []string{"db_host=DB_HOST", "api_key=API_KEY"},
					EnvAllowed: []string{"PATH", "HOME", "USER"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				Spec: &runnertypes.GroupSpec{
					EnvImport:  []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH"},
				},
				EnvAllowlistInheritanceMode: runnertypes.InheritanceModeExplicit,
			},
			detailLevel: resource.DetailLevelFull,
			expectedAnalysis: &resource.InheritanceAnalysis{
				GlobalEnvImport:               []string{"db_host=DB_HOST", "api_key=API_KEY"},
				GlobalAllowlist:               []string{"PATH", "HOME", "USER"},
				GroupEnvImport:                []string{"db_host=DB_HOST"},
				GroupAllowlist:                []string{"PATH"},
				InheritanceMode:               runnertypes.InheritanceModeExplicit,
				InheritedVariables:            []string{}, // Empty because mode is Explicit
				RemovedAllowlistVariables:     []string{"HOME", "USER"},
				UnavailableEnvImportVariables: []string{"api_key"},
			},
		},
		{
			name: "DetailLevelFull - InheritanceModeInherit",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{
					EnvImport:  []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH", "HOME"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				Spec:                        &runnertypes.GroupSpec{},
				EnvAllowlistInheritanceMode: runnertypes.InheritanceModeInherit,
			},
			detailLevel: resource.DetailLevelFull,
			expectedAnalysis: &resource.InheritanceAnalysis{
				GlobalEnvImport:               []string{"db_host=DB_HOST"},
				GlobalAllowlist:               []string{"PATH", "HOME"},
				GroupEnvImport:                []string{},
				GroupAllowlist:                []string{},
				InheritanceMode:               runnertypes.InheritanceModeInherit,
				InheritedVariables:            []string{"PATH", "HOME"}, // Inherits from global
				RemovedAllowlistVariables:     []string{},
				UnavailableEnvImportVariables: []string{},
			},
		},
		{
			name: "DetailLevelFull - InheritanceModeReject",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{
					EnvImport:  []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH", "HOME"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				Spec: &runnertypes.GroupSpec{
					EnvAllowed: []string{}, // Empty slice means reject
				},
				EnvAllowlistInheritanceMode: runnertypes.InheritanceModeReject,
			},
			detailLevel: resource.DetailLevelFull,
			expectedAnalysis: &resource.InheritanceAnalysis{
				GlobalEnvImport:               []string{"db_host=DB_HOST"},
				GlobalAllowlist:               []string{"PATH", "HOME"},
				GroupEnvImport:                []string{},
				GroupAllowlist:                []string{},
				InheritanceMode:               runnertypes.InheritanceModeReject,
				InheritedVariables:            []string{}, // Empty because mode is Reject
				RemovedAllowlistVariables:     []string{"HOME", "PATH"},
				UnavailableEnvImportVariables: []string{},
			},
		},
		{
			name: "nil Spec is handled safely",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{
					EnvImport:  []string{"db_host=DB_HOST"},
					EnvAllowed: []string{"PATH"},
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				Spec:                        nil, // nil Spec
				EnvAllowlistInheritanceMode: runnertypes.InheritanceModeInherit,
			},
			detailLevel: resource.DetailLevelFull,
			expectedAnalysis: &resource.InheritanceAnalysis{
				GlobalEnvImport:               []string{"db_host=DB_HOST"},
				GlobalAllowlist:               []string{"PATH"},
				GroupEnvImport:                []string{},
				GroupAllowlist:                []string{},
				InheritanceMode:               runnertypes.InheritanceModeInherit,
				InheritedVariables:            []string{"PATH"},
				RemovedAllowlistVariables:     []string{},
				UnavailableEnvImportVariables: []string{},
			},
		},
		{
			name: "nil slices are converted to empty slices",
			runtimeGlobal: &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{
					EnvImport:  nil,
					EnvAllowed: nil,
				},
			},
			runtimeGroup: &runnertypes.RuntimeGroup{
				Spec: &runnertypes.GroupSpec{
					EnvImport:  nil,
					EnvAllowed: nil,
				},
				EnvAllowlistInheritanceMode: runnertypes.InheritanceModeInherit,
			},
			detailLevel: resource.DetailLevelDetailed,
			expectedAnalysis: &resource.InheritanceAnalysis{
				GlobalEnvImport:               []string{},
				GlobalAllowlist:               []string{},
				GroupEnvImport:                []string{},
				GroupAllowlist:                []string{},
				InheritanceMode:               runnertypes.InheritanceModeInherit,
				InheritedVariables:            nil,
				RemovedAllowlistVariables:     nil,
				UnavailableEnvImportVariables: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CollectInheritanceAnalysis(
				tt.runtimeGlobal,
				tt.runtimeGroup,
				tt.detailLevel,
			)

			if tt.expectedAnalysis == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedAnalysis.GlobalEnvImport, result.GlobalEnvImport)
				assert.Equal(t, tt.expectedAnalysis.GlobalAllowlist, result.GlobalAllowlist)
				assert.Equal(t, tt.expectedAnalysis.GroupEnvImport, result.GroupEnvImport)
				assert.Equal(t, tt.expectedAnalysis.GroupAllowlist, result.GroupAllowlist)
				assert.Equal(t, tt.expectedAnalysis.InheritanceMode, result.InheritanceMode)
				assert.Equal(t, tt.expectedAnalysis.InheritedVariables, result.InheritedVariables)
				assert.Equal(t, tt.expectedAnalysis.RemovedAllowlistVariables, result.RemovedAllowlistVariables)
				assert.Equal(t, tt.expectedAnalysis.UnavailableEnvImportVariables, result.UnavailableEnvImportVariables)
			}
		})
	}
}

func TestCollectFinalEnvironment(t *testing.T) {
	tests := []struct {
		name          string
		envMap        map[string]executor.EnvVar
		detailLevel   resource.DryRunDetailLevel
		showSensitive bool
		expectedEnv   *resource.FinalEnvironment
	}{
		{
			name: "DetailLevelSummary returns nil",
			envMap: map[string]executor.EnvVar{
				"PATH": {Value: "/usr/bin", Origin: "system"},
			},
			detailLevel:   resource.DetailLevelSummary,
			showSensitive: false,
			expectedEnv:   nil,
		},
		{
			name: "DetailLevelDetailed returns nil",
			envMap: map[string]executor.EnvVar{
				"PATH": {Value: "/usr/bin", Origin: "system"},
			},
			detailLevel:   resource.DetailLevelDetailed,
			showSensitive: false,
			expectedEnv:   nil,
		},
		{
			name: "DetailLevelFull with showSensitive=true",
			envMap: map[string]executor.EnvVar{
				"PATH":    {Value: "/usr/bin", Origin: "system"},
				"API_KEY": {Value: "secret123", Origin: "vars"},
			},
			detailLevel:   resource.DetailLevelFull,
			showSensitive: true,
			expectedEnv: &resource.FinalEnvironment{
				Variables: map[string]resource.EnvironmentVariable{
					"PATH": {
						Value:  "/usr/bin",
						Source: "system",
						Masked: false,
					},
					"API_KEY": {
						Value:  "secret123",
						Source: "vars",
						Masked: false,
					},
				},
			},
		},
		{
			name: "DetailLevelFull with showSensitive=false masks sensitive vars",
			envMap: map[string]executor.EnvVar{
				"PATH":    {Value: "/usr/bin", Origin: "system"},
				"API_KEY": {Value: "secret123", Origin: "vars"},
			},
			detailLevel:   resource.DetailLevelFull,
			showSensitive: false,
			expectedEnv: &resource.FinalEnvironment{
				Variables: map[string]resource.EnvironmentVariable{
					"PATH": {
						Value:  "/usr/bin",
						Source: "system",
						Masked: false,
					},
					"API_KEY": {
						Value:  "",
						Source: "vars",
						Masked: true,
					},
				},
			},
		},
		{
			name: "Various source types are mapped correctly",
			envMap: map[string]executor.EnvVar{
				"VAR1": {Value: "value1", Origin: "system"},
				"VAR2": {Value: "value2", Origin: "vars"},
				"VAR3": {Value: "value3", Origin: "vars"},
				"VAR4": {Value: "value4", Origin: "command"},
			},
			detailLevel:   resource.DetailLevelFull,
			showSensitive: true,
			expectedEnv: &resource.FinalEnvironment{
				Variables: map[string]resource.EnvironmentVariable{
					"VAR1": {Value: "value1", Source: "system", Masked: false},
					"VAR2": {Value: "value2", Source: "vars", Masked: false},
					"VAR3": {Value: "value3", Source: "vars", Masked: false},
					"VAR4": {Value: "value4", Source: "command", Masked: false},
				},
			},
		},
		{
			name:          "Empty environment map",
			envMap:        map[string]executor.EnvVar{},
			detailLevel:   resource.DetailLevelFull,
			showSensitive: true,
			expectedEnv: &resource.FinalEnvironment{
				Variables: map[string]resource.EnvironmentVariable{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CollectFinalEnvironment(
				tt.envMap,
				tt.detailLevel,
				tt.showSensitive,
			)

			if tt.expectedEnv == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, len(tt.expectedEnv.Variables), len(result.Variables))
				for name, expectedVar := range tt.expectedEnv.Variables {
					actualVar, ok := result.Variables[name]
					assert.True(t, ok, "Variable %s should exist", name)
					assert.Equal(t, expectedVar.Value, actualVar.Value, "Variable %s value mismatch", name)
					assert.Equal(t, expectedVar.Source, actualVar.Source, "Variable %s source mismatch", name)
					assert.Equal(t, expectedVar.Masked, actualVar.Masked, "Variable %s masked mismatch", name)
				}
			}
		})
	}
}
