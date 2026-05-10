//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewUnsetTimeout(t *testing.T) {
	timeout := NewUnsetTimeout()

	assert.False(t, timeout.IsSet(), "NewUnsetTimeout() should create an unset timeout")
	assert.False(t, timeout.IsUnlimited(), "NewUnsetTimeout() should not be unlimited")
	// Value() should not be called on unset timeout - it should panic
}

func unlimitedTimeout() Timeout {
	return Timeout{NewOptionalValue[int32](0)}
}

func timeoutFromSeconds(seconds int32) Timeout {
	return Timeout{NewOptionalValue(seconds)}
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
			timeout: unlimitedTimeout(),
			want:    true,
		},
		{
			name: "positive timeout",
			timeout: func() Timeout {
				return timeoutFromSeconds(60)
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
			timeout: unlimitedTimeout(),
			want:    true,
		},
		{
			name: "positive timeout",
			timeout: func() Timeout {
				return timeoutFromSeconds(60)
			}(),
			want: false,
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
		want    int32
	}{
		{
			name:    "unlimited timeout",
			timeout: unlimitedTimeout(),
			want:    0,
		},
		{
			name: "positive timeout",
			timeout: func() Timeout {
				return timeoutFromSeconds(60)
			}(),
			want: 60,
		},
		{
			name: "large timeout",
			timeout: func() Timeout {
				return timeoutFromSeconds(3600)
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
		ptr       *int32
		wantSet   bool
		wantUnlim bool
		wantValue int32
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
			ptr:       ptr(int32(0)),
			wantSet:   true,
			wantUnlim: true,
			wantValue: 0,
		},
		{
			name:      "positive pointer creates timeout",
			ptr:       ptr(int32(120)),
			wantSet:   true,
			wantUnlim: false,
			wantValue: 120,
		},
		{
			name:      "max timeout pointer",
			ptr:       ptr(int32(MaxTimeout)),
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
