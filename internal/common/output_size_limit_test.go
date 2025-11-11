//go:build test

//nolint:revive // "common" is an appropriate name for shared utilities package
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrInvalidOutputSizeLimit_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     ErrInvalidOutputSizeLimit
		wantErr error
	}{
		{
			name: "negative output size",
			err: ErrInvalidOutputSizeLimit{
				Value:   int64(-1),
				Context: "output size limit cannot be negative",
			},
			wantErr: ErrInvalidOutputSizeLimit{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify error type using errors.As
			assert.ErrorAs(t, tt.err, &tt.wantErr, "error should be of type ErrInvalidOutputSizeLimit")
			// Verify Error() returns non-empty message
			assert.NotEmpty(t, tt.err.Error(), "Error() should return non-empty message")
		})
	}
}

func TestNewUnsetOutputSizeLimit(t *testing.T) {
	limit := NewUnsetOutputSizeLimit()

	assert.False(t, limit.IsSet(), "NewUnsetOutputSizeLimit() should create an unset limit")
	assert.False(t, limit.IsUnlimited(), "NewUnsetOutputSizeLimit() should not be unlimited")
	// Value() should not be called on unset limit - it should panic
}

func TestNewUnlimitedOutputSizeLimit(t *testing.T) {
	limit := NewUnlimitedOutputSizeLimit()

	assert.True(t, limit.IsSet(), "NewUnlimitedOutputSizeLimit() should be set")
	assert.True(t, limit.IsUnlimited(), "NewUnlimitedOutputSizeLimit() should be unlimited")
	assert.Equal(t, int64(0), limit.Value(), "NewUnlimitedOutputSizeLimit().Value() should be 0")
}

func TestNewOutputSizeLimit(t *testing.T) {
	tests := []struct {
		name    string
		bytes   int64
		wantErr bool
	}{
		{
			name:    "valid positive size",
			bytes:   1024,
			wantErr: false,
		},
		{
			name:    "zero size (unlimited)",
			bytes:   0,
			wantErr: false,
		},
		{
			name:    "large size (10MB)",
			bytes:   10 * 1024 * 1024,
			wantErr: false,
		},
		{
			name:    "negative size",
			bytes:   -1,
			wantErr: true,
		},
		{
			name:    "large negative size",
			bytes:   -1024,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, err := NewOutputSizeLimit(tt.bytes)
			if tt.wantErr {
				assert.Error(t, err, "NewOutputSizeLimit(%d) should return error", tt.bytes)
				var invalidErr ErrInvalidOutputSizeLimit
				assert.ErrorAs(t, err, &invalidErr, "error should be ErrInvalidOutputSizeLimit")
				return
			}
			require.NoError(t, err, "NewOutputSizeLimit(%d) should not return error", tt.bytes)
			assert.True(t, limit.IsSet(), "NewOutputSizeLimit() should create a set limit")
			assert.Equal(t, tt.bytes, limit.Value(), "NewOutputSizeLimit(%d).Value() should match", tt.bytes)
			if tt.bytes == 0 {
				assert.True(t, limit.IsUnlimited(), "NewOutputSizeLimit(0) should be unlimited")
			} else {
				assert.False(t, limit.IsUnlimited(), "NewOutputSizeLimit(non-zero) should not be unlimited")
			}
		})
	}
}

func TestOutputSizeLimit_IsSet(t *testing.T) {
	tests := []struct {
		name  string
		limit OutputSizeLimit
		want  bool
	}{
		{
			name:  "unset limit",
			limit: NewUnsetOutputSizeLimit(),
			want:  false,
		},
		{
			name:  "unlimited limit",
			limit: NewUnlimitedOutputSizeLimit(),
			want:  true,
		},
		{
			name: "positive limit",
			limit: func() OutputSizeLimit {
				l, _ := NewOutputSizeLimit(1024)
				return l
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.limit.IsSet(), "OutputSizeLimit.IsSet() should match expected value")
		})
	}
}

func TestOutputSizeLimit_IsUnlimited(t *testing.T) {
	tests := []struct {
		name  string
		limit OutputSizeLimit
		want  bool
	}{
		{
			name:  "unset limit",
			limit: NewUnsetOutputSizeLimit(),
			want:  false,
		},
		{
			name:  "unlimited limit",
			limit: NewUnlimitedOutputSizeLimit(),
			want:  true,
		},
		{
			name: "positive limit",
			limit: func() OutputSizeLimit {
				l, _ := NewOutputSizeLimit(1024)
				return l
			}(),
			want: false,
		},
		{
			name: "zero via NewOutputSizeLimit",
			limit: func() OutputSizeLimit {
				l, _ := NewOutputSizeLimit(0)
				return l
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.limit.IsUnlimited(), "OutputSizeLimit.IsUnlimited() should match expected value")
		})
	}
}

func TestOutputSizeLimit_Value(t *testing.T) {
	tests := []struct {
		name  string
		limit OutputSizeLimit
		want  int64
	}{
		{
			name:  "unlimited limit",
			limit: NewUnlimitedOutputSizeLimit(),
			want:  0,
		},
		{
			name: "positive limit",
			limit: func() OutputSizeLimit {
				l, _ := NewOutputSizeLimit(1024)
				return l
			}(),
			want: 1024,
		},
		{
			name: "large limit (10MB)",
			limit: func() OutputSizeLimit {
				l, _ := NewOutputSizeLimit(10 * 1024 * 1024)
				return l
			}(),
			want: 10 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.limit.Value(), "OutputSizeLimit.Value() should match expected value")
		})
	}
}

func TestOutputSizeLimit_ValuePanicsOnUnset(t *testing.T) {
	limit := NewUnsetOutputSizeLimit()

	assert.Panics(t, func() {
		_ = limit.Value()
	}, "Value() should panic when called on unset OutputSizeLimit")
}

func TestNewOutputSizeLimitFromPtr(t *testing.T) {
	tests := []struct {
		name      string
		ptr       *int64
		wantSet   bool
		wantUnlim bool
		wantValue int64
	}{
		{
			name:      "nil pointer creates unset limit",
			ptr:       nil,
			wantSet:   false,
			wantUnlim: false,
			wantValue: 0, // Value() should not be called, but this is for documentation
		},
		{
			name:      "zero pointer creates unlimited limit",
			ptr:       Int64Ptr(0),
			wantSet:   true,
			wantUnlim: true,
			wantValue: 0,
		},
		{
			name:      "positive pointer creates limit",
			ptr:       Int64Ptr(1024),
			wantSet:   true,
			wantUnlim: false,
			wantValue: 1024,
		},
		{
			name:      "large limit pointer (100MB)",
			ptr:       Int64Ptr(100 * 1024 * 1024),
			wantSet:   true,
			wantUnlim: false,
			wantValue: 100 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := NewOutputSizeLimitFromPtr(tt.ptr)
			assert.Equal(t, tt.wantSet, limit.IsSet(), "IsSet() should match expected value")
			assert.Equal(t, tt.wantUnlim, limit.IsUnlimited(), "IsUnlimited() should match expected value")
			if tt.wantSet {
				assert.Equal(t, tt.wantValue, limit.Value(), "Value() should match expected value")
			}
		})
	}
}
