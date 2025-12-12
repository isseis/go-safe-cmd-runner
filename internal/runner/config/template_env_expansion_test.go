//go:build test

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandTemplateEnv(t *testing.T) {
	tests := []struct {
		name         string
		env          []string
		params       map[string]interface{}
		templateName string
		want         []string
		wantErr      bool
		errType      error
	}{
		// Basic expansion
		{
			name:         "simple string parameter",
			env:          []string{"PATH=${path}"},
			params:       map[string]interface{}{"path": "/usr/bin"},
			templateName: "test",
			want:         []string{"PATH=/usr/bin"},
		},
		{
			name:         "multiple env entries",
			env:          []string{"PATH=${path}", "HOME=${home}"},
			params:       map[string]interface{}{"path": "/usr/bin", "home": "/home/user"},
			templateName: "test",
			want:         []string{"PATH=/usr/bin", "HOME=/home/user"},
		},

		// Array expansion (element-level)
		{
			name:         "pure array placeholder - single element",
			env:          []string{"REQUIRED=foo", "${@opts}"},
			params:       map[string]interface{}{"opts": []string{"DEBUG=1"}},
			templateName: "test",
			want:         []string{"REQUIRED=foo", "DEBUG=1"},
		},
		{
			name:         "pure array placeholder - multiple elements",
			env:          []string{"REQUIRED=foo", "${@opts}"},
			params:       map[string]interface{}{"opts": []string{"DEBUG=1", "VERBOSE=1"}},
			templateName: "test",
			want:         []string{"REQUIRED=foo", "DEBUG=1", "VERBOSE=1"},
		},
		{
			name:         "pure array placeholder - empty array",
			env:          []string{"REQUIRED=foo", "${@opts}"},
			params:       map[string]interface{}{"opts": []string{}},
			templateName: "test",
			want:         []string{"REQUIRED=foo"},
		},
		{
			name:         "pure array placeholder - not provided",
			env:          []string{"REQUIRED=foo", "${@opts}"},
			params:       map[string]interface{}{},
			templateName: "test",
			want:         []string{"REQUIRED=foo"},
		},
		{
			name: "multiple array placeholders",
			env:  []string{"${@common}", "${@app_specific}"},
			params: map[string]interface{}{
				"common":       []string{"PATH=/usr/bin", "HOME=/home/user"},
				"app_specific": []string{"DEBUG=1"},
			},
			templateName: "test",
			want:         []string{"PATH=/usr/bin", "HOME=/home/user", "DEBUG=1"},
		},

		// Optional placeholder
		{
			name:         "optional parameter - VALUE part",
			env:          []string{"PATH=${?path}"},
			params:       map[string]interface{}{"path": "/usr/bin"},
			templateName: "test",
			want:         []string{"PATH=/usr/bin"},
		},
		{
			name:         "optional parameter - VALUE part - empty",
			env:          []string{"REQUIRED=foo", "PATH=${?path}"},
			params:       map[string]interface{}{"path": ""},
			templateName: "test",
			want:         []string{"REQUIRED=foo"},
		},
		{
			name:         "optional parameter - VALUE part - not provided",
			env:          []string{"REQUIRED=foo", "PATH=${?path}"},
			params:       map[string]interface{}{},
			templateName: "test",
			want:         []string{"REQUIRED=foo"},
		},

		// Mixed scenarios
		{
			name: "combined string and array parameters",
			env:  []string{"STATIC=value", "DYNAMIC=${param}", "${@opts}"},
			params: map[string]interface{}{
				"param": "/usr/bin",
				"opts":  []string{"DEBUG=1", "VERBOSE=1"},
			},
			templateName: "test",
			want:         []string{"STATIC=value", "DYNAMIC=/usr/bin", "DEBUG=1", "VERBOSE=1"},
		},

		// Error cases - invalid format
		{
			name:         "invalid format - no equals sign",
			env:          []string{"INVALID"},
			params:       map[string]interface{}{},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrTemplateInvalidEnvFormat{},
		},
		{
			name:         "invalid format after expansion",
			env:          []string{"${entry}"},
			params:       map[string]interface{}{"entry": "INVALID"},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrTemplateInvalidEnvFormat{},
		},
		{
			name:         "invalid format in array element",
			env:          []string{"${@opts}"},
			params:       map[string]interface{}{"opts": []string{"DEBUG=1", "INVALID"}},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrTemplateInvalidEnvFormat{},
		},

		// Error cases - placeholder in KEY
		{
			name:         "placeholder in KEY part - required",
			env:          []string{"${key}=value"},
			params:       map[string]interface{}{"key": "PATH"},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrPlaceholderInEnvKey{},
		},
		{
			name:         "placeholder in KEY part - optional",
			env:          []string{"${?key}=value"},
			params:       map[string]interface{}{"key": "PATH"},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrPlaceholderInEnvKey{},
		},
		{
			name:         "placeholder in KEY part - array",
			env:          []string{"${@keys}=value"},
			params:       map[string]interface{}{"keys": []string{"PATH"}},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrPlaceholderInEnvKey{},
		},
		{
			name:         "placeholder in middle of KEY",
			env:          []string{"PREFIX_${key}_SUFFIX=value"},
			params:       map[string]interface{}{"key": "PATH"},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrPlaceholderInEnvKey{},
		},

		// Error cases - array in VALUE part (mixed context)
		{
			name:         "array in VALUE part",
			env:          []string{"PATH=${@paths}"},
			params:       map[string]interface{}{"paths": []string{"/usr/bin", "/bin"}},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrArrayInMixedContext{},
		},

		// Error cases - duplicate keys
		{
			name:         "duplicate key in template",
			env:          []string{"PATH=/usr/bin", "PATH=/bin"},
			params:       map[string]interface{}{},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrDuplicateEnvVariableDetail{},
		},
		{
			name:         "duplicate key from array expansion",
			env:          []string{"REQUIRED=value", "${@optional_env}"},
			params:       map[string]interface{}{"optional_env": []string{"REQUIRED=foo"}},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrDuplicateEnvVariableDetail{},
		},
		{
			name: "duplicate key from multiple array placeholders",
			env:  []string{"${@common}", "${@app_specific}"},
			params: map[string]interface{}{
				"common":       []string{"PATH=/usr/bin", "HOME=/home/user"},
				"app_specific": []string{"PATH=/usr/local/bin"},
			},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrDuplicateEnvVariableDetail{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandTemplateEnv(tt.env, tt.params, tt.templateName)

			if tt.wantErr {
				assert.Error(t, err, "expected error, got nil")

				// Verify error type
				switch tt.errType.(type) {
				case *ErrTemplateInvalidEnvFormat:
					var target *ErrTemplateInvalidEnvFormat
					assert.ErrorAs(t, err, &target, "expected ErrTemplateInvalidEnvFormat")
				case *ErrPlaceholderInEnvKey:
					var target *ErrPlaceholderInEnvKey
					assert.ErrorAs(t, err, &target, "expected ErrPlaceholderInEnvKey")
				case *ErrArrayInMixedContext:
					var target *ErrArrayInMixedContext
					assert.ErrorAs(t, err, &target, "expected ErrArrayInMixedContext")
				case *ErrDuplicateEnvVariableDetail:
					var target *ErrDuplicateEnvVariableDetail
					assert.ErrorAs(t, err, &target, "expected ErrDuplicateEnvVariableDetail")
				}
				return
			}

			assert.NoError(t, err, "unexpected error")

			assert.Equal(t, len(tt.want), len(result), "result length mismatch")
			assert.Equal(t, tt.want, result, "result content mismatch")
		})
	}
}
