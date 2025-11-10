package runnertypes

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestNewRuntimeCommand_TimeoutResolution(t *testing.T) {
	tests := []struct {
		name              string
		commandTimeout    *int
		globalTimeout     *int
		expectedEffective int
	}{
		{
			name:              "command timeout takes precedence",
			commandTimeout:    common.IntPtr(120),
			globalTimeout:     common.IntPtr(60),
			expectedEffective: 120,
		},
		{
			name:              "global timeout when command is nil",
			commandTimeout:    nil,
			globalTimeout:     common.IntPtr(90),
			expectedEffective: 90,
		},
		{
			name:              "default timeout when both are nil",
			commandTimeout:    nil,
			globalTimeout:     nil,
			expectedEffective: common.DefaultTimeout,
		},
		{
			name:              "command unlimited timeout (0)",
			commandTimeout:    common.IntPtr(0),
			globalTimeout:     common.IntPtr(60),
			expectedEffective: 0,
		},
		{
			name:              "global unlimited timeout (0)",
			commandTimeout:    nil,
			globalTimeout:     common.IntPtr(0),
			expectedEffective: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &CommandSpec{
				Name:    "test-command",
				Cmd:     "/bin/echo",
				Args:    []string{"hello"},
				Timeout: tt.commandTimeout,
			}

			runtime, err := NewRuntimeCommand(spec, common.NewFromIntPtr(tt.globalTimeout), common.NewUnsetOutputSizeLimit(), "test-group")
			assert.NoError(t, err, "NewRuntimeCommand() should not fail")

			assert.Equal(t, tt.expectedEffective, runtime.EffectiveTimeout,
				"EffectiveTimeout should match expected value")

			// Verify that the original spec timeout is preserved
			timeout := runtime.Timeout()
			if tt.commandTimeout == nil {
				assert.False(t, timeout.IsSet(), "Timeout should be unset when command timeout is nil")
			} else {
				assert.True(t, timeout.IsSet(), "Timeout should be set when command timeout is specified")
				assert.Equal(t, *tt.commandTimeout, timeout.Value(),
					"Timeout value should match command timeout")
			}
		})
	}
}

func TestNewRuntimeCommand_CommandTimeoutZero(t *testing.T) {
	spec := &CommandSpec{
		Name:    "unlimited-command",
		Cmd:     "/bin/sleep",
		Args:    []string{"999999"},
		Timeout: common.IntPtr(0), // Unlimited execution
	}

	globalTimeout := common.IntPtr(60) // 60 seconds global timeout

	runtime, err := NewRuntimeCommand(spec, common.NewFromIntPtr(globalTimeout), common.NewUnsetOutputSizeLimit(), "test-group")
	assert.NoError(t, err, "NewRuntimeCommand() should not fail")

	// Command timeout should take precedence, resulting in unlimited execution
	assert.Equal(t, 0, runtime.EffectiveTimeout,
		"Command timeout should take precedence, resulting in unlimited execution")

	// Verify that the original command timeout is preserved (0 = unlimited)
	timeout := runtime.Timeout()
	assert.True(t, timeout.IsSet(), "Timeout should be set when explicitly set to 0")
	assert.True(t, timeout.IsUnlimited(), "Timeout should be unlimited when set to 0")
	assert.Equal(t, 0, timeout.Value(), "Timeout value should be 0")
}

func TestNewRuntimeCommand_GlobalTimeoutZero(t *testing.T) {
	spec := &CommandSpec{
		Name:    "inherit-unlimited-command",
		Cmd:     "/bin/sleep",
		Args:    []string{"999999"},
		Timeout: nil, // Inherit from global
	}

	globalTimeout := common.IntPtr(0) // Unlimited global timeout

	runtime, err := NewRuntimeCommand(spec, common.NewFromIntPtr(globalTimeout), common.NewUnsetOutputSizeLimit(), "test-group")
	assert.NoError(t, err, "NewRuntimeCommand() should not fail")

	// Should inherit unlimited execution from global timeout
	assert.Equal(t, 0, runtime.EffectiveTimeout,
		"Should inherit unlimited execution from global timeout")

	// Verify that the command timeout is still unset (not specified at command level)
	timeout := runtime.Timeout()
	assert.False(t, timeout.IsSet(), "Timeout should be unset at command level when not specified")
}

func TestNewRuntimeCommand_ErrorHandling(t *testing.T) {
	// Test with nil spec
	runtime, err := NewRuntimeCommand(nil, common.NewFromIntPtr(common.IntPtr(60)), common.NewUnsetOutputSizeLimit(), "test-group")
	assert.ErrorIs(t, err, ErrNilSpec, "NewRuntimeCommand(nil, ...) should return ErrNilSpec")
	assert.Nil(t, runtime, "NewRuntimeCommand(nil, ...) should return nil runtime")
}

func TestNewRuntimeCommand_TimeoutResolutionContext(t *testing.T) {
	tests := []struct {
		name          string
		cmdTimeout    *int
		globalTimeout *int
		commandName   string
		groupName     string
		wantValue     int
		wantLevel     string
	}{
		{
			name:          "command level resolution",
			cmdTimeout:    common.IntPtr(30),
			globalTimeout: common.IntPtr(60),
			commandName:   "test-cmd",
			groupName:     "test-group",
			wantValue:     30,
			wantLevel:     "command",
		},
		{
			name:          "global level resolution",
			cmdTimeout:    nil,
			globalTimeout: common.IntPtr(60),
			commandName:   "test-cmd",
			groupName:     "test-group",
			wantValue:     60,
			wantLevel:     "global",
		},
		{
			name:          "default level resolution",
			cmdTimeout:    nil,
			globalTimeout: nil,
			commandName:   "test-cmd",
			groupName:     "test-group",
			wantValue:     common.DefaultTimeout,
			wantLevel:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &CommandSpec{
				Name:    tt.commandName,
				Cmd:     "/bin/echo",
				Timeout: tt.cmdTimeout,
			}

			runtime, err := NewRuntimeCommand(
				spec,
				common.NewFromIntPtr(tt.globalTimeout),
				common.NewUnsetOutputSizeLimit(),
				tt.groupName,
			)

			assert.NoError(t, err, "NewRuntimeCommand should not fail")
			assert.Equal(t, tt.wantValue, runtime.EffectiveTimeout, "effective timeout should match")
			assert.Equal(t, tt.wantLevel, runtime.TimeoutResolution.Level, "resolution level should match")
			assert.Equal(t, tt.commandName, runtime.TimeoutResolution.CommandName, "command name should match")
			assert.Equal(t, tt.groupName, runtime.TimeoutResolution.GroupName, "group name should match")
		})
	}
}
