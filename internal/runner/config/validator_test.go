package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTimeouts(t *testing.T) {
	tests := []struct {
		name        string
		config      *runnertypes.ConfigSpec
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid - no timeout specified",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test_group",
						Commands: []runnertypes.CommandSpec{
							{
								Name: "test_cmd",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid - positive global timeout",
			config: &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: common.Int32Ptr(30),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test_group",
						Commands: []runnertypes.CommandSpec{
							{
								Name: "test_cmd",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid - zero global timeout",
			config: &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: common.Int32Ptr(0),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test_group",
						Commands: []runnertypes.CommandSpec{
							{
								Name: "test_cmd",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid - negative global timeout",
			config: &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: common.Int32Ptr(-10),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test_group",
						Commands: []runnertypes.CommandSpec{
							{
								Name: "test_cmd",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "timeout must not be negative: global timeout got -10",
		},
		{
			name: "valid - positive command timeout",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test_group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:    "test_cmd",
								Cmd:     "/bin/echo",
								Timeout: common.Int32Ptr(60),
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid - zero command timeout",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test_group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:    "test_cmd",
								Cmd:     "/bin/echo",
								Timeout: common.Int32Ptr(0),
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid - negative command timeout",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test_group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:    "test_cmd",
								Cmd:     "/bin/echo",
								Timeout: common.Int32Ptr(-5),
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "timeout must not be negative: command 'test_cmd' in group 'test_group' (groups[0].commands[0]) got -5",
		},
		{
			name: "invalid - multiple negative command timeouts",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test_group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:    "cmd1",
								Cmd:     "/bin/echo",
								Timeout: common.Int32Ptr(-1),
							},
							{
								Name:    "cmd2",
								Cmd:     "/bin/echo",
								Timeout: common.Int32Ptr(-2),
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "timeout must not be negative: command 'cmd1' in group 'test_group' (groups[0].commands[0]) got -1",
		},
		{
			name: "invalid - negative timeout in second group",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{
						Name: "group1",
						Commands: []runnertypes.CommandSpec{
							{
								Name:    "cmd1",
								Cmd:     "/bin/echo",
								Timeout: common.Int32Ptr(30),
							},
						},
					},
					{
						Name: "group2",
						Commands: []runnertypes.CommandSpec{
							{
								Name:    "cmd2",
								Cmd:     "/bin/echo",
								Timeout: common.Int32Ptr(-15),
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "timeout must not be negative: command 'cmd2' in group 'group2' (groups[1].commands[0]) got -15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeouts(tt.config)

			if tt.expectError {
				require.Error(t, err, "expected error but got none")
				assert.Contains(t, err.Error(), tt.errorMsg, "error message mismatch")
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}
