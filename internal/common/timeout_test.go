//go:build test

//nolint:revive // "common" is an appropriate name for shared utilities package
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrInvalidTimeout_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     ErrInvalidTimeout
		wantMsg string
	}{
		{
			name: "negative timeout",
			err: ErrInvalidTimeout{
				Value:   -1,
				Context: "timeout cannot be negative",
			},
			wantMsg: "invalid timeout value -1 in timeout cannot be negative",
		},
		{
			name: "exceeds max",
			err: ErrInvalidTimeout{
				Value:   100000,
				Context: "timeout exceeds maximum allowed value",
			},
			wantMsg: "invalid timeout value 100000 in timeout exceeds maximum allowed value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			assert.Equal(t, tt.wantMsg, got, "ErrInvalidTimeout.Error() should match expected message")
		})
	}
}

func TestNewUnsetTimeout(t *testing.T) {
	timeout := NewUnsetTimeout()

	assert.False(t, timeout.IsSet(), "NewUnsetTimeout() should create an unset timeout")
	assert.False(t, timeout.IsUnlimited(), "NewUnsetTimeout() should not be unlimited")
	// Value() should not be called on unset timeout - it should panic
}

func TestNewUnlimitedTimeout(t *testing.T) {
	timeout := NewUnlimitedTimeout()

	assert.True(t, timeout.IsSet(), "NewUnlimitedTimeout() should be set")
	assert.True(t, timeout.IsUnlimited(), "NewUnlimitedTimeout() should be unlimited")
	assert.Equal(t, 0, timeout.Value(), "NewUnlimitedTimeout().Value() should be 0")
}

func TestNewTimeout(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		wantErr bool
	}{
		{
			name:    "valid positive timeout",
			seconds: 60,
			wantErr: false,
		},
		{
			name:    "zero timeout",
			seconds: 0,
			wantErr: false,
		},
		{
			name:    "max timeout",
			seconds: MaxTimeout,
			wantErr: false,
		},
		{
			name:    "negative timeout",
			seconds: -1,
			wantErr: true,
		},
		{
			name:    "exceeds max timeout",
			seconds: MaxTimeout + 1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout, err := NewTimeout(tt.seconds)
			if tt.wantErr {
				assert.Error(t, err, "NewTimeout(%d) should return error", tt.seconds)
				return
			}
			require.NoError(t, err, "NewTimeout(%d) should not return error", tt.seconds)
			assert.True(t, timeout.IsSet(), "NewTimeout() should create a set timeout")
			assert.Equal(t, tt.seconds, timeout.Value(), "NewTimeout(%d).Value() should match", tt.seconds)
			if tt.seconds == 0 {
				assert.True(t, timeout.IsUnlimited(), "NewTimeout(0) should be unlimited")
			} else {
				assert.False(t, timeout.IsUnlimited(), "NewTimeout(non-zero) should not be unlimited")
			}
		})
	}
}

func TestTimeout_IsSet(t *testing.T) {
	tests := []struct {
		name    string
		timeout Timeout
		want    bool
	}{
		{
			name:    "unset timeout",
			timeout: NewUnsetTimeout(),
			want:    false,
		},
		{
			name:    "unlimited timeout",
			timeout: NewUnlimitedTimeout(),
			want:    true,
		},
		{
			name: "positive timeout",
			timeout: func() Timeout {
				t, _ := NewTimeout(60)
				return t
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.timeout.IsSet(), "Timeout.IsSet() should match expected value")
		})
	}
}

func TestTimeout_IsUnlimited(t *testing.T) {
	tests := []struct {
		name    string
		timeout Timeout
		want    bool
	}{
		{
			name:    "unset timeout",
			timeout: NewUnsetTimeout(),
			want:    false,
		},
		{
			name:    "unlimited timeout",
			timeout: NewUnlimitedTimeout(),
			want:    true,
		},
		{
			name: "positive timeout",
			timeout: func() Timeout {
				t, _ := NewTimeout(60)
				return t
			}(),
			want: false,
		},
		{
			name: "zero via NewTimeout",
			timeout: func() Timeout {
				t, _ := NewTimeout(0)
				return t
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.timeout.IsUnlimited(), "Timeout.IsUnlimited() should match expected value")
		})
	}
}

func TestTimeout_Value(t *testing.T) {
	tests := []struct {
		name    string
		timeout Timeout
		want    int
	}{
		{
			name:    "unlimited timeout",
			timeout: NewUnlimitedTimeout(),
			want:    0,
		},
		{
			name: "positive timeout",
			timeout: func() Timeout {
				t, _ := NewTimeout(60)
				return t
			}(),
			want: 60,
		},
		{
			name: "large timeout",
			timeout: func() Timeout {
				t, _ := NewTimeout(3600)
				return t
			}(),
			want: 3600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.timeout.Value(), "Timeout.Value() should match expected value")
		})
	}
}

func TestTimeout_ValuePanicsOnUnset(t *testing.T) {
	timeout := NewUnsetTimeout()

	assert.Panics(t, func() {
		_ = timeout.Value()
	}, "Value() should panic when called on unset Timeout")
}

func TestNewFromIntPtr(t *testing.T) {
	tests := []struct {
		name      string
		ptr       *int
		wantSet   bool
		wantUnlim bool
		wantValue int
	}{
		{
			name:      "nil pointer creates unset timeout",
			ptr:       nil,
			wantSet:   false,
			wantUnlim: false,
			wantValue: 0, // Value() should not be called, but this is for documentation
		},
		{
			name:      "zero pointer creates unlimited timeout",
			ptr:       IntPtr(0),
			wantSet:   true,
			wantUnlim: true,
			wantValue: 0,
		},
		{
			name:      "positive pointer creates timeout",
			ptr:       IntPtr(120),
			wantSet:   true,
			wantUnlim: false,
			wantValue: 120,
		},
		{
			name:      "max timeout pointer",
			ptr:       IntPtr(MaxTimeout),
			wantSet:   true,
			wantUnlim: false,
			wantValue: MaxTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := NewFromIntPtr(tt.ptr)
			assert.Equal(t, tt.wantSet, timeout.IsSet(), "IsSet() should match expected value")
			assert.Equal(t, tt.wantUnlim, timeout.IsUnlimited(), "IsUnlimited() should match expected value")
			if tt.wantSet {
				assert.Equal(t, tt.wantValue, timeout.Value(), "Value() should match expected value")
			}
		})
	}
}

func TestResolveEffectiveTimeout(t *testing.T) {
	tests := []struct {
		name              string
		commandTimeoutSec *int // nil for unset, pointer to value otherwise
		globalTimeoutSec  *int // nil for unset, pointer to value otherwise
		want              int
	}{
		{
			name:              "command timeout overrides global",
			commandTimeoutSec: IntPtr(120),
			globalTimeoutSec:  IntPtr(60),
			want:              120,
		},
		{
			name:              "command unlimited overrides global",
			commandTimeoutSec: IntPtr(0),
			globalTimeoutSec:  IntPtr(60),
			want:              0,
		},
		{
			name:              "global timeout when command unset",
			commandTimeoutSec: nil,
			globalTimeoutSec:  IntPtr(90),
			want:              90,
		},
		{
			name:              "global unlimited when command unset",
			commandTimeoutSec: nil,
			globalTimeoutSec:  IntPtr(0),
			want:              0,
		},
		{
			name:              "default timeout when both unset",
			commandTimeoutSec: nil,
			globalTimeoutSec:  nil,
			want:              DefaultTimeout,
		},
		{
			name:              "command zero overrides positive global",
			commandTimeoutSec: IntPtr(0),
			globalTimeoutSec:  IntPtr(300),
			want:              0,
		},
		{
			name:              "command positive overrides global zero",
			commandTimeoutSec: IntPtr(180),
			globalTimeoutSec:  IntPtr(0),
			want:              180,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commandTimeout := NewFromIntPtr(tt.commandTimeoutSec)
			globalTimeout := NewFromIntPtr(tt.globalTimeoutSec)

			result := ResolveEffectiveTimeout(commandTimeout, globalTimeout)
			assert.Equal(t, tt.want, result, "ResolveEffectiveTimeout() should return expected value")
		})
	}
}
