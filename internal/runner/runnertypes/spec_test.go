package runnertypes

import (
	"reflect"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalSpec_UnmarshalTOML_NewFieldNames(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		want    GlobalSpec
		wantErr bool
	}{
		{
			name: "verify_standard_paths = true",
			toml: `
verify_standard_paths = true
env_vars = ["LANG=en_US.UTF-8"]
env_allowed = ["PATH", "HOME"]
env_import = ["user=USER"]
output_size_limit = 1048576
`,
			want: GlobalSpec{
				VerifyStandardPaths: commontesting.BoolPtr(true),
				EnvVars:             []string{"LANG=en_US.UTF-8"},
				EnvAllowed:          []string{"PATH", "HOME"},
				EnvImport:           []string{"user=USER"},
				OutputSizeLimit:     commontesting.Int64Ptr(1048576),
			},
		},
		{
			name: "verify_standard_paths = false",
			toml: `
verify_standard_paths = false
env_vars = []
env_allowed = []
env_import = []
output_size_limit = 0
`,
			want: GlobalSpec{
				VerifyStandardPaths: commontesting.BoolPtr(false),
				EnvVars:             []string{},
				EnvAllowed:          []string{},
				EnvImport:           []string{},
				OutputSizeLimit:     commontesting.Int64Ptr(0),
			},
		},
		{
			name: "verify_standard_paths omitted (nil)",
			toml: `
env_vars = ["DEBUG=1"]
env_allowed = ["DEBUG"]
env_import = []
output_size_limit = 2097152
`,
			want: GlobalSpec{
				VerifyStandardPaths: nil, // Should remain nil
				EnvVars:             []string{"DEBUG=1"},
				EnvAllowed:          []string{"DEBUG"},
				EnvImport:           []string{},
				OutputSizeLimit:     commontesting.Int64Ptr(2097152),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var spec GlobalSpec
			err := toml.Unmarshal([]byte(tt.toml), &spec)

			assert.Equal(t, tt.wantErr, err != nil, "toml.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			if err == nil {
				require.True(t, reflect.DeepEqual(spec, tt.want), "GlobalSpec unmarshal result mismatch")

				// Detailed comparison for VerifyStandardPaths
				switch {
				case spec.VerifyStandardPaths == nil && tt.want.VerifyStandardPaths != nil:
					assert.Fail(t, "VerifyStandardPaths: got nil, want %v", *tt.want.VerifyStandardPaths)
				case spec.VerifyStandardPaths != nil && tt.want.VerifyStandardPaths == nil:
					assert.Fail(t, "VerifyStandardPaths: got %v, want nil", *spec.VerifyStandardPaths)
				case spec.VerifyStandardPaths != nil && tt.want.VerifyStandardPaths != nil:
					if *spec.VerifyStandardPaths != *tt.want.VerifyStandardPaths {
						assert.Equal(t, *tt.want.VerifyStandardPaths, *spec.VerifyStandardPaths, "VerifyStandardPaths mismatch")
					}
				}
			}
		})
	}
}

func TestGroupSpec_UnmarshalTOML_NewFieldNames(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		want    GroupSpec
		wantErr bool
	}{
		{
			name: "all new field names",
			toml: `
name = "test-group"
env_vars = ["GROUP_VAR=value"]
env_allowed = ["GROUP_VAR", "PATH"]
env_import = ["group_user=USER"]
`,
			want: GroupSpec{
				Name:       "test-group",
				EnvVars:    []string{"GROUP_VAR=value"},
				EnvAllowed: []string{"GROUP_VAR", "PATH"},
				EnvImport:  []string{"group_user=USER"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var spec GroupSpec
			err := toml.Unmarshal([]byte(tt.toml), &spec)

			assert.Equal(t, tt.wantErr, err != nil, "toml.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			if err == nil {
				require.True(t, reflect.DeepEqual(spec, tt.want), "GroupSpec unmarshal result mismatch")
			}
		})
	}
}

func TestCommandSpec_UnmarshalTOML_NewFieldNames(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		want    CommandSpec
		wantErr bool
	}{
		{
			name: "all new field names",
			toml: `
name = "test-command"
cmd = "/bin/echo"
env_vars = ["CMD_VAR=test"]
env_import = ["cmd_user=USER"]
risk_level = "low"
output_file = "/tmp/output.log"
`,
			want: CommandSpec{
				Name:       "test-command",
				Cmd:        "/bin/echo",
				EnvVars:    []string{"CMD_VAR=test"},
				EnvImport:  []string{"cmd_user=USER"},
				RiskLevel:  "low",
				OutputFile: "/tmp/output.log",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var spec CommandSpec
			err := toml.Unmarshal([]byte(tt.toml), &spec)

			assert.Equal(t, tt.wantErr, err != nil, "toml.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			if err == nil {
				require.True(t, reflect.DeepEqual(spec, tt.want), "CommandSpec unmarshal result mismatch")
			}
		})
	}
}

func TestConfigSpec_Parse(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		want    *ConfigSpec
		wantErr bool
	}{
		{
			name: "valid basic config",
			toml: `
version = "1.0"

[global]
timeout = 300

[[groups]]
name = "test"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
args = ["hello"]
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global: GlobalSpec{
					Timeout: commontesting.Int32Ptr(300),
				},
				Groups: []GroupSpec{
					{
						Name: "test",
						Commands: []CommandSpec{
							{
								Name: "hello",
								Cmd:  "/bin/echo",
								Args: []string{"hello"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with all global fields",
			toml: `
version = "1.0"

[global]
timeout = 300
verify_standard_paths = false
output_size_limit = 1048576
verify_files = ["/usr/bin/python3", "/usr/bin/gcc"]
env_allowed = ["PATH", "HOME"]
env_vars = ["PATH=/usr/bin:/bin", "HOME=/root"]
env_import = ["user=USER", "shell=SHELL"]

[global.vars]
PREFIX = "/opt"
VERSION = "1.0"

[[groups]]
name = "test"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global: GlobalSpec{
					Timeout:             commontesting.Int32Ptr(300),
					VerifyStandardPaths: commontesting.BoolPtr(false),
					OutputSizeLimit:     commontesting.Int64Ptr(1048576),
					VerifyFiles:         []string{"/usr/bin/python3", "/usr/bin/gcc"},
					EnvAllowed:          []string{"PATH", "HOME"},
					EnvVars:             []string{"PATH=/usr/bin:/bin", "HOME=/root"},
					EnvImport:           []string{"user=USER", "shell=SHELL"},
					Vars:                map[string]any{"PREFIX": "/opt", "VERSION": "1.0"},
				},
				Groups: []GroupSpec{
					{
						Name: "test",
						Commands: []CommandSpec{
							{
								Name: "hello",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with all group fields",
			toml: `
version = "1.0"

[global]
timeout = 300

[[groups]]
name = "build"
description = "Build tasks"
workdir = "/tmp/build"
verify_files = ["/usr/bin/make"]
env_allowed = ["PATH", "CC"]
env_vars = ["CC=gcc"]
env_import = ["home=HOME"]

[groups.vars]
BUILD_TYPE = "release"

[[groups.commands]]
name = "compile"
cmd = "/usr/bin/make"
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global: GlobalSpec{
					Timeout: commontesting.Int32Ptr(300),
				},
				Groups: []GroupSpec{
					{
						Name:        "build",
						Description: "Build tasks",
						WorkDir:     "/tmp/build",
						VerifyFiles: []string{"/usr/bin/make"},
						EnvAllowed:  []string{"PATH", "CC"},
						EnvVars:     []string{"CC=gcc"},
						EnvImport:   []string{"home=HOME"},
						Vars:        map[string]any{"BUILD_TYPE": "release"},
						Commands: []CommandSpec{
							{
								Name: "compile",
								Cmd:  "/usr/bin/make",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with all command fields",
			toml: `
version = "1.0"

[global]
timeout = 300

[[groups]]
name = "test"

[[groups.commands]]
name = "mycommand"
description = "Test command"
cmd = "/usr/bin/python3"
args = ["-m", "pytest"]
workdir = "/tmp/test"
timeout = 60
run_as_user = "testuser"
run_as_group = "testgroup"
risk_level = "medium"
output_file = "/tmp/output.log"
env_vars = ["PYTHONPATH=/opt/lib"]
env_import = ["path=PATH"]

[groups.commands.vars]
TEST_VAR = "value"
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global: GlobalSpec{
					Timeout: commontesting.Int32Ptr(300),
				},
				Groups: []GroupSpec{
					{
						Name: "test",
						Commands: []CommandSpec{
							{
								Name:        "mycommand",
								Description: "Test command",
								Cmd:         "/usr/bin/python3",
								Args:        []string{"-m", "pytest"},
								WorkDir:     "/tmp/test",
								Timeout:     commontesting.Int32Ptr(60),
								RunAsUser:   "testuser",
								RunAsGroup:  "testgroup",
								RiskLevel:   "medium",
								OutputFile:  "/tmp/output.log",
								EnvVars:     []string{"PYTHONPATH=/opt/lib"},
								EnvImport:   []string{"path=PATH"},
								Vars:        map[string]any{"TEST_VAR": "value"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty fields use default values",
			toml: `
version = "1.0"

[global]

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd"
cmd = "/bin/true"
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global:  GlobalSpec{},
				Groups: []GroupSpec{
					{
						Name: "test",
						Commands: []CommandSpec{
							{
								Name: "cmd",
								Cmd:  "/bin/true",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple groups and commands",
			toml: `
version = "1.0"

[global]
timeout = 300

[[groups]]
name = "group1"

[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"
args = ["hello"]

[[groups.commands]]
name = "cmd2"
cmd = "/bin/echo"
args = ["world"]

[[groups]]
name = "group2"

[[groups.commands]]
name = "cmd3"
cmd = "/bin/date"
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global: GlobalSpec{
					Timeout: commontesting.Int32Ptr(300),
				},
				Groups: []GroupSpec{
					{
						Name: "group1",
						Commands: []CommandSpec{
							{
								Name: "cmd1",
								Cmd:  "/bin/echo",
								Args: []string{"hello"},
							},
							{
								Name: "cmd2",
								Cmd:  "/bin/echo",
								Args: []string{"world"},
							},
						},
					},
					{
						Name: "group2",
						Commands: []CommandSpec{
							{
								Name: "cmd3",
								Cmd:  "/bin/date",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid TOML syntax",
			toml:    `invalid toml [[[`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ConfigSpec
			err := toml.Unmarshal([]byte(tt.toml), &got)
			assert.Equal(t, tt.wantErr, err != nil, "Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			if !tt.wantErr {
				require.True(t, reflect.DeepEqual(&got, tt.want), "Unmarshal() got:\n%+v\nwant:\n%+v", &got, tt.want)
			}
		})
	}
}

func TestCommandSpec_GetRiskLevel(t *testing.T) {
	tests := []struct {
		name      string
		riskLevel string
		want      RiskLevel
		wantErr   bool
	}{
		{
			name:      "low risk level",
			riskLevel: "low",
			want:      RiskLevelLow,
			wantErr:   false,
		},
		{
			name:      "medium risk level",
			riskLevel: "medium",
			want:      RiskLevelMedium,
			wantErr:   false,
		},
		{
			name:      "high risk level",
			riskLevel: "high",
			want:      RiskLevelHigh,
			wantErr:   false,
		},
		{
			name:      "empty string defaults to low",
			riskLevel: "",
			want:      RiskLevelLow,
			wantErr:   false,
		},
		{
			name:      "unknown risk level",
			riskLevel: "unknown",
			want:      RiskLevelUnknown,
			wantErr:   false,
		},
		{
			name:      "invalid risk level",
			riskLevel: "invalid",
			want:      RiskLevelUnknown,
			wantErr:   true,
		},
		{
			name:      "critical risk level is prohibited",
			riskLevel: "critical",
			want:      RiskLevelUnknown,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &CommandSpec{
				RiskLevel: tt.riskLevel,
			}
			got, err := spec.GetRiskLevel()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCommandSpec_HasUserGroupSpecification(t *testing.T) {
	tests := []struct {
		name       string
		runAsUser  string
		runAsGroup string
		want       bool
	}{
		{
			name:       "both user and group specified",
			runAsUser:  "testuser",
			runAsGroup: "testgroup",
			want:       true,
		},
		{
			name:       "only user specified",
			runAsUser:  "testuser",
			runAsGroup: "",
			want:       true,
		},
		{
			name:       "only group specified",
			runAsUser:  "",
			runAsGroup: "testgroup",
			want:       true,
		},
		{
			name:       "neither specified",
			runAsUser:  "",
			runAsGroup: "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &CommandSpec{
				RunAsUser:  tt.runAsUser,
				RunAsGroup: tt.runAsGroup,
			}
			got := spec.HasUserGroupSpecification()
			assert.Equal(t, tt.want, got, "HasUserGroupSpecification() = %v, want %v", got, tt.want)
		})
	}
}
