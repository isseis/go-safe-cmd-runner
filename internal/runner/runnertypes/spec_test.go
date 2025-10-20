package runnertypes

import (
	"reflect"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

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
					Timeout: 300,
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
log_level = "debug"
skip_standard_paths = true
max_output_size = 1048576
verify_files = ["/usr/bin/python3", "/usr/bin/gcc"]
env_allowlist = ["PATH", "HOME"]
env = ["PATH=/usr/bin:/bin", "HOME=/root"]
from_env = ["user=USER", "shell=SHELL"]
vars = ["PREFIX=/opt", "VERSION=1.0"]

[[groups]]
name = "test"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global: GlobalSpec{
					Timeout:           300,
					LogLevel:          "debug",
					SkipStandardPaths: true,
					MaxOutputSize:     1048576,
					VerifyFiles:       []string{"/usr/bin/python3", "/usr/bin/gcc"},
					EnvAllowlist:      []string{"PATH", "HOME"},
					Env:               []string{"PATH=/usr/bin:/bin", "HOME=/root"},
					FromEnv:           []string{"user=USER", "shell=SHELL"},
					Vars:              []string{"PREFIX=/opt", "VERSION=1.0"},
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
priority = 1
workdir = "/tmp/build"
verify_files = ["/usr/bin/make"]
env_allowlist = ["PATH", "CC"]
env = ["CC=gcc"]
from_env = ["home=HOME"]
vars = ["BUILD_TYPE=release"]

[[groups.commands]]
name = "compile"
cmd = "/usr/bin/make"
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global: GlobalSpec{
					Timeout: 300,
				},
				Groups: []GroupSpec{
					{
						Name:         "build",
						Description:  "Build tasks",
						Priority:     1,
						WorkDir:      "/tmp/build",
						VerifyFiles:  []string{"/usr/bin/make"},
						EnvAllowlist: []string{"PATH", "CC"},
						Env:          []string{"CC=gcc"},
						FromEnv:      []string{"home=HOME"},
						Vars:         []string{"BUILD_TYPE=release"},
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
max_risk_level = "medium"
output = "/tmp/output.log"
env = ["PYTHONPATH=/opt/lib"]
from_env = ["path=PATH"]
vars = ["TEST_VAR=value"]
`,
			want: &ConfigSpec{
				Version: "1.0",
				Global: GlobalSpec{
					Timeout: 300,
				},
				Groups: []GroupSpec{
					{
						Name: "test",
						Commands: []CommandSpec{
							{
								Name:         "mycommand",
								Description:  "Test command",
								Cmd:          "/usr/bin/python3",
								Args:         []string{"-m", "pytest"},
								WorkDir:      "/tmp/test",
								Timeout:      60,
								RunAsUser:    "testuser",
								RunAsGroup:   "testgroup",
								MaxRiskLevel: "medium",
								Output:       "/tmp/output.log",
								Env:          []string{"PYTHONPATH=/opt/lib"},
								FromEnv:      []string{"path=PATH"},
								Vars:         []string{"TEST_VAR=value"},
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
					Timeout: 300,
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
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !reflect.DeepEqual(&got, tt.want) {
					t.Errorf("Unmarshal() got:\n%+v\nwant:\n%+v", &got, tt.want)
				}
			}
		})
	}
}

func TestCommandSpec_GetMaxRiskLevel(t *testing.T) {
	tests := []struct {
		name         string
		maxRiskLevel string
		want         RiskLevel
		wantErr      bool
	}{
		{
			name:         "low risk level",
			maxRiskLevel: "low",
			want:         RiskLevelLow,
			wantErr:      false,
		},
		{
			name:         "medium risk level",
			maxRiskLevel: "medium",
			want:         RiskLevelMedium,
			wantErr:      false,
		},
		{
			name:         "high risk level",
			maxRiskLevel: "high",
			want:         RiskLevelHigh,
			wantErr:      false,
		},
		{
			name:         "empty string defaults to low",
			maxRiskLevel: "",
			want:         RiskLevelLow,
			wantErr:      false,
		},
		{
			name:         "unknown risk level",
			maxRiskLevel: "unknown",
			want:         RiskLevelUnknown,
			wantErr:      false,
		},
		{
			name:         "invalid risk level",
			maxRiskLevel: "invalid",
			want:         RiskLevelUnknown,
			wantErr:      true,
		},
		{
			name:         "critical risk level is prohibited",
			maxRiskLevel: "critical",
			want:         RiskLevelUnknown,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &CommandSpec{
				MaxRiskLevel: tt.maxRiskLevel,
			}
			got, err := spec.GetMaxRiskLevel()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMaxRiskLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetMaxRiskLevel() = %v, want %v", got, tt.want)
			}
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
			if got != tt.want {
				t.Errorf("HasUserGroupSpecification() = %v, want %v", got, tt.want)
			}
		})
	}
}
