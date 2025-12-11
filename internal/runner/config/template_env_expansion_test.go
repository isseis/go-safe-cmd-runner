//go:build test

package config

import (
	"errors"
	"testing"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandTemplateEnv(tt.env, tt.params, tt.templateName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}

				// Verify error type
				switch tt.errType.(type) {
				case *ErrTemplateInvalidEnvFormat:
					var target *ErrTemplateInvalidEnvFormat
					if !errors.As(err, &target) {
						t.Errorf("expected ErrTemplateInvalidEnvFormat, got %T: %v", err, err)
					}
				case *ErrPlaceholderInEnvKey:
					var target *ErrPlaceholderInEnvKey
					if !errors.As(err, &target) {
						t.Errorf("expected ErrPlaceholderInEnvKey, got %T: %v", err, err)
					}
				case *ErrArrayInMixedContext:
					var target *ErrArrayInMixedContext
					if !errors.As(err, &target) {
						t.Errorf("expected ErrArrayInMixedContext, got %T: %v", err, err)
					}
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.want) {
				t.Errorf("result length = %d, want %d\nresult: %v\nwant: %v",
					len(result), len(tt.want), result, tt.want)
				return
			}

			for i, got := range result {
				if got != tt.want[i] {
					t.Errorf("result[%d] = %q, want %q", i, got, tt.want[i])
				}
			}
		})
	}
}
