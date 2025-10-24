package runnertypes

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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

			runtime, err := NewRuntimeCommand(spec, tt.globalTimeout)
			if err != nil {
				t.Fatalf("NewRuntimeCommand() failed: %v", err)
			}

			if runtime.EffectiveTimeout != tt.expectedEffective {
				t.Errorf("EffectiveTimeout = %d, want %d", runtime.EffectiveTimeout, tt.expectedEffective)
			}

			// Verify that the original spec timeout is preserved
			timeout := runtime.Timeout()
			if tt.commandTimeout == nil {
				if timeout.IsSet() {
					t.Errorf("Timeout().IsSet() = true, want false (unset)")
				}
			} else {
				if !timeout.IsSet() {
					t.Errorf("Timeout().IsSet() = false, want true")
				}
				if timeout.Value() != *tt.commandTimeout {
					t.Errorf("Timeout().Value() = %d, want %d", timeout.Value(), *tt.commandTimeout)
				}
			}
		})
	}
}

func TestNewRuntimeCommandLegacy_BackwardCompatibility(t *testing.T) {
	spec := &CommandSpec{
		Name: "test-command",
		Cmd:  "/bin/echo",
		Args: []string{"hello"},
		// No timeout specified
		Timeout: nil,
	}

	runtime, err := NewRuntimeCommandLegacy(spec)
	if err != nil {
		t.Fatalf("NewRuntimeCommandLegacy() failed: %v", err)
	}

	// Should use default timeout since no global timeout is provided
	if runtime.EffectiveTimeout != common.DefaultTimeout {
		t.Errorf("EffectiveTimeout = %d, want %d", runtime.EffectiveTimeout, common.DefaultTimeout)
	}

	// Verify that the spec timeout is unset
	timeout := runtime.Timeout()
	if timeout.IsSet() {
		t.Errorf("Timeout().IsSet() = true, want false (unset)")
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

	runtime, err := NewRuntimeCommand(spec, globalTimeout)
	if err != nil {
		t.Fatalf("NewRuntimeCommand() failed: %v", err)
	}

	// Command timeout should take precedence, resulting in unlimited execution
	if runtime.EffectiveTimeout != 0 {
		t.Errorf("EffectiveTimeout = %d, want 0 (unlimited)", runtime.EffectiveTimeout)
	}

	// Verify that the original command timeout is preserved (0 = unlimited)
	timeout := runtime.Timeout()
	if !timeout.IsSet() {
		t.Errorf("Timeout().IsSet() = false, want true")
	}
	if !timeout.IsUnlimited() {
		t.Errorf("Timeout().IsUnlimited() = false, want true")
	}
	if timeout.Value() != 0 {
		t.Errorf("Timeout().Value() = %d, want 0", timeout.Value())
	}
}

func TestNewRuntimeCommand_GlobalTimeoutZero(t *testing.T) {
	spec := &CommandSpec{
		Name:    "inherit-unlimited-command",
		Cmd:     "/bin/sleep",
		Args:    []string{"999999"},
		Timeout: nil, // Inherit from global
	}

	globalTimeout := common.IntPtr(0) // Unlimited global timeout

	runtime, err := NewRuntimeCommand(spec, globalTimeout)
	if err != nil {
		t.Fatalf("NewRuntimeCommand() failed: %v", err)
	}

	// Should inherit unlimited execution from global timeout
	if runtime.EffectiveTimeout != 0 {
		t.Errorf("EffectiveTimeout = %d, want 0 (unlimited)", runtime.EffectiveTimeout)
	}

	// Verify that the command timeout is still unset (not specified at command level)
	timeout := runtime.Timeout()
	if timeout.IsSet() {
		t.Errorf("Timeout().IsSet() = true, want false (unset at command level)")
	}
}

func TestNewRuntimeCommand_ErrorHandling(t *testing.T) {
	// Test with nil spec
	runtime, err := NewRuntimeCommand(nil, common.IntPtr(60))
	if err != ErrNilSpec {
		t.Errorf("NewRuntimeCommand(nil, ...) error = %v, want %v", err, ErrNilSpec)
	}
	if runtime != nil {
		t.Errorf("NewRuntimeCommand(nil, ...) runtime = %v, want nil", runtime)
	}

	// Test legacy function with nil spec
	runtimeLegacy, err := NewRuntimeCommandLegacy(nil)
	if err != ErrNilSpec {
		t.Errorf("NewRuntimeCommandLegacy(nil) error = %v, want %v", err, ErrNilSpec)
	}
	if runtimeLegacy != nil {
		t.Errorf("NewRuntimeCommandLegacy(nil) runtime = %v, want nil", runtimeLegacy)
	}
}
