package config

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// TestValidateEnvList tests environment variable list validation
func TestValidateEnvList(t *testing.T) {
	tests := []struct {
		name    string
		envList []string
		context string
		wantErr bool
		errType error
	}{
		{
			name:    "no duplicates",
			envList: []string{"VAR1=value1", "VAR2=value2", "VAR3=value3"},
			context: "global.env",
			wantErr: false,
		},
		{
			name:    "duplicate keys",
			envList: []string{"VAR1=value1", "VAR2=value2", "VAR1=value3"},
			context: "global.env",
			wantErr: true,
			errType: ErrDuplicateEnvVariable,
		},
		{
			name:    "different case keys are allowed (case-sensitive)",
			envList: []string{"PATH=/usr/bin", "path=/usr/local/bin"},
			context: "group.env",
			wantErr: false,
		},
		{
			name:    "empty list",
			envList: []string{},
			context: "global.env",
			wantErr: false,
		},
		{
			name:    "nil list",
			envList: nil,
			context: "global.env",
			wantErr: false,
		},
		{
			name:    "invalid format without equals",
			envList: []string{"VAR1=value1", "INVALID_NO_EQUALS"},
			context: "global.env",
			wantErr: true,
			errType: ErrMalformedEnvVariable,
		},
		{
			name:    "empty key",
			envList: []string{"=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrMalformedEnvVariable,
		},
		{
			name:    "invalid key starting with number",
			envList: []string{"123VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrInvalidEnvKey,
		},
		{
			name:    "invalid key with hyphen",
			envList: []string{"MY-VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrInvalidEnvKey,
		},
		{
			name:    "invalid key with dot",
			envList: []string{"MY.VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrInvalidEnvKey,
		},
		{
			name:    "invalid key with space",
			envList: []string{"MY VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrInvalidEnvKey,
		},
		{
			name:    "reserved prefix __RUNNER_",
			envList: []string{"__RUNNER_VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: &runnertypes.ReservedEnvPrefixError{},
		},
		{
			name:    "lowercase __runner_ is allowed (case-sensitive check)",
			envList: []string{"__runner_var=value"},
			context: "global.env",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnvList(tt.envList, tt.context)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					// If tt.errType is a pointer to a struct error (like
					// *runnertypes.ReservedEnvPrefixError), use errors.As to
					// check the error chain for that concrete type. For
					// sentinel errors (plain error values), fall back to
					// errors.Is.
					switch tt.errType.(type) {
					case *runnertypes.ReservedEnvPrefixError:
						var target *runnertypes.ReservedEnvPrefixError
						assert.True(t, errors.As(err, &target))
					default:
						assert.True(t, errors.Is(err, tt.errType))
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
