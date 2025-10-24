//nolint:revive // "common" is an appropriate name for shared utilities package
package common

import (
	"testing"
)

func TestNewUnsetTimeout(t *testing.T) {
	timeout := NewUnsetTimeout()

	if timeout.IsSet() {
		t.Error("NewUnsetTimeout() should create an unset timeout")
	}
	if timeout.IsUnlimited() {
		t.Error("NewUnsetTimeout() should not be unlimited")
	}
	if timeout.Value() != 0 {
		t.Errorf("NewUnsetTimeout().Value() = %d, want 0", timeout.Value())
	}
}

func TestNewUnlimitedTimeout(t *testing.T) {
	timeout := NewUnlimitedTimeout()

	if !timeout.IsSet() {
		t.Error("NewUnlimitedTimeout() should be set")
	}
	if !timeout.IsUnlimited() {
		t.Error("NewUnlimitedTimeout() should be unlimited")
	}
	if timeout.Value() != 0 {
		t.Errorf("NewUnlimitedTimeout().Value() = %d, want 0", timeout.Value())
	}
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
				if err == nil {
					t.Errorf("NewTimeout(%d) should return error", tt.seconds)
				}
				return
			}
			if err != nil {
				t.Errorf("NewTimeout(%d) unexpected error: %v", tt.seconds, err)
				return
			}
			if !timeout.IsSet() {
				t.Error("NewTimeout() should create a set timeout")
			}
			if timeout.Value() != tt.seconds {
				t.Errorf("NewTimeout(%d).Value() = %d, want %d", tt.seconds, timeout.Value(), tt.seconds)
			}
			if tt.seconds == 0 {
				if !timeout.IsUnlimited() {
					t.Error("NewTimeout(0) should be unlimited")
				}
			} else {
				if timeout.IsUnlimited() {
					t.Error("NewTimeout(non-zero) should not be unlimited")
				}
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
			if got := tt.timeout.IsSet(); got != tt.want {
				t.Errorf("Timeout.IsSet() = %v, want %v", got, tt.want)
			}
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
			if got := tt.timeout.IsUnlimited(); got != tt.want {
				t.Errorf("Timeout.IsUnlimited() = %v, want %v", got, tt.want)
			}
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
			name:    "unset timeout",
			timeout: NewUnsetTimeout(),
			want:    0,
		},
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
			if got := tt.timeout.Value(); got != tt.want {
				t.Errorf("Timeout.Value() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTimeout_UnmarshalTOML(t *testing.T) {
	tests := []struct {
		name      string
		data      interface{}
		wantValue int
		wantSet   bool
		wantErr   bool
	}{
		{
			name:      "nil data",
			data:      nil,
			wantValue: 0,
			wantSet:   false,
			wantErr:   false,
		},
		{
			name:      "int value",
			data:      60,
			wantValue: 60,
			wantSet:   true,
			wantErr:   false,
		},
		{
			name:      "int64 value",
			data:      int64(120),
			wantValue: 120,
			wantSet:   true,
			wantErr:   false,
		},
		{
			name:      "int32 value",
			data:      int32(30),
			wantValue: 30,
			wantSet:   true,
			wantErr:   false,
		},
		{
			name:      "zero value",
			data:      0,
			wantValue: 0,
			wantSet:   true,
			wantErr:   false,
		},
		{
			name:    "negative value",
			data:    -1,
			wantErr: true,
		},
		{
			name:    "exceeds max",
			data:    MaxTimeout + 1,
			wantErr: true,
		},
		{
			name:    "unsupported type",
			data:    "60",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var timeout Timeout
			err := timeout.UnmarshalTOML(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Errorf("UnmarshalTOML() should return error for %v", tt.data)
				}
				return
			}
			if err != nil {
				t.Errorf("UnmarshalTOML() unexpected error: %v", err)
				return
			}
			if timeout.IsSet() != tt.wantSet {
				t.Errorf("UnmarshalTOML() IsSet() = %v, want %v", timeout.IsSet(), tt.wantSet)
			}
			if timeout.Value() != tt.wantValue {
				t.Errorf("UnmarshalTOML() Value() = %d, want %d", timeout.Value(), tt.wantValue)
			}
		})
	}
}
