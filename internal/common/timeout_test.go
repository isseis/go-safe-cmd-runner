//nolint:revive // "common" is an appropriate name for shared utilities package
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
