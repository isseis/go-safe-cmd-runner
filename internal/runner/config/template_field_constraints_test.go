//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// TestTemplateFieldConstraints validates all field-specific parameter usage constraints
// in a single table-driven test for better visibility of the complete constraint matrix.
//
// This test ensures that:
// - cmd, workdir: Only allow ${param} and ${?param}, reject ${@param}
// - args: Allow all placeholder types (${param}, ${?param}, ${@param})
// - env: Allow all types at element level, but reject placeholders in KEY part
func TestTemplateFieldConstraints(t *testing.T) {
	tests := []struct {
		name        string
		field       string // Field being tested
		placeholder string // Placeholder content (e.g., "${@param}")
		inEnvKey    bool   // For env: is the placeholder in the KEY part?
		wantErr     bool   // Should this produce an error?
		errType     error  // Expected error type
		description string // Human-readable explanation
	}{
		// ========== cmd field ==========
		{
			name:        "cmd: ${param} allowed",
			field:       "cmd",
			placeholder: "${binary}",
			wantErr:     false,
			description: "Required placeholder in cmd is valid",
		},
		{
			name:        "cmd: ${?param} allowed",
			field:       "cmd",
			placeholder: "${?binary}",
			wantErr:     false,
			description: "Optional placeholder in cmd is valid",
		},
		// Note: cmd with ${@param} is tested in integration tests (template_integration_test.go)
		// because expandSingleArg allows it, but the validation happens at a higher level

		// ========== args field (VALUE part )==========
		{
			name:        "args VALUE: ${param} allowed",
			field:       "args",
			placeholder: "${file}.txt",
			wantErr:     false,
			description: "Required placeholder in args is valid",
		},
		{
			name:        "args VALUE: ${?param} allowed",
			field:       "args",
			placeholder: "test${?version}.log",
			wantErr:     false,
			description: "Optional placeholder in args is valid",
		},
		{
			name:        "args VALUE: ${@param} in mixed context rejected",
			field:       "args",
			placeholder: "foo-${@flags}",
			wantErr:     true,
			errType:     &ErrArrayInMixedContext{},
			description: "Array placeholder mixed with other text in args element is invalid",
		},

		// ========== args field (element level)==========
		{
			name:        "args: ${param} allowed",
			field:       "args",
			placeholder: "${file}",
			wantErr:     false,
			description: "Required placeholder in args is valid",
		},
		{
			name:        "args: ${?param} allowed",
			field:       "args",
			placeholder: "${?verbose}",
			wantErr:     false,
			description: "Optional placeholder in args is valid",
		},
		{
			name:        "args: ${@param} allowed",
			field:       "args",
			placeholder: "${@flags}",
			wantErr:     false,
			description: "Array placeholder in args is valid (element-level expansion)",
		},

		// ========== env field (VALUE part) ==========
		{
			name:        "env VALUE: ${param} allowed",
			field:       "env",
			placeholder: "PATH=${path}",
			inEnvKey:    false,
			wantErr:     false,
			description: "Required placeholder in env VALUE is valid",
		},
		{
			name:        "env VALUE: ${?param} allowed",
			field:       "env",
			placeholder: "PATH=${?path}",
			inEnvKey:    false,
			wantErr:     false,
			description: "Optional placeholder in env VALUE is valid",
		},
		{
			name:        "env VALUE: ${@param} rejected (mixed context)",
			field:       "env",
			placeholder: "PATH=${@paths}",
			inEnvKey:    false,
			wantErr:     true,
			errType:     &ErrArrayInMixedContext{},
			description: "Array placeholder in env VALUE part must be rejected",
		},

		// ========== env field (element level) ==========
		{
			name:        "env element: ${param} allowed",
			field:       "env_element",
			placeholder: "${env_var}",
			inEnvKey:    false,
			wantErr:     false,
			description: "Required placeholder as entire env element is valid",
		},
		{
			name:        "env element: ${?param} allowed",
			field:       "env_element",
			placeholder: "${?env_var}",
			inEnvKey:    false,
			wantErr:     false,
			description: "Optional placeholder as entire env element is valid",
		},
		{
			name:        "env element: ${@param} allowed",
			field:       "env_element",
			placeholder: "${@env_vars}",
			inEnvKey:    false,
			wantErr:     false,
			description: "Array placeholder as entire env element is valid (element-level expansion)",
		},

		// ========== env field (KEY part - all rejected) ==========
		{
			name:        "env KEY: ${param} rejected",
			field:       "env",
			placeholder: "${key}=value",
			inEnvKey:    true,
			wantErr:     true,
			errType:     &ErrPlaceholderInEnvKey{},
			description: "Required placeholder in env KEY must be rejected (security)",
		},
		{
			name:        "env KEY: ${?param} rejected",
			field:       "env",
			placeholder: "${?key}=value",
			inEnvKey:    true,
			wantErr:     true,
			errType:     &ErrPlaceholderInEnvKey{},
			description: "Optional placeholder in env KEY must be rejected (security)",
		},
		{
			name:        "env KEY: ${@param} rejected",
			field:       "env",
			placeholder: "${@keys}=value",
			inEnvKey:    true,
			wantErr:     true,
			errType:     &ErrPlaceholderInEnvKey{},
			description: "Array placeholder in env KEY must be rejected (security)",
		},
		{
			name:        "env KEY: placeholder in middle rejected",
			field:       "env",
			placeholder: "PREFIX_${key}_SUFFIX=value",
			inEnvKey:    true,
			wantErr:     true,
			errType:     &ErrPlaceholderInEnvKey{},
			description: "Placeholder anywhere in env KEY must be rejected (security)",
		},

		// ========== workdir field ==========
		{
			name:        "workdir: ${param} allowed",
			field:       "workdir",
			placeholder: "${dir}",
			wantErr:     false,
			description: "Required placeholder in workdir is valid",
		},
		{
			name:        "workdir: ${?param} allowed",
			field:       "workdir",
			placeholder: "${?dir}",
			wantErr:     false,
			description: "Optional placeholder in workdir is valid",
		},
		{
			name:        "workdir: ${@param} rejected",
			field:       "workdir",
			placeholder: "${@dirs}",
			wantErr:     true,
			errType:     &ErrArrayInMixedContext{},
			description: "Array placeholder in workdir must be rejected (must be single path)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test template based on field
			var template *runnertypes.CommandTemplate
			params := map[string]any{}

			switch tt.field {
			case "cmd":
				template = &runnertypes.CommandTemplate{
					Cmd: tt.placeholder,
				}
				// Setup params based on placeholder type
				setupParams(params, tt.placeholder)

			case "args":
				template = &runnertypes.CommandTemplate{
					Cmd:  "test",
					Args: []string{tt.placeholder},
				}
				setupParams(params, tt.placeholder)

			case "env":
				template = &runnertypes.CommandTemplate{
					Cmd:     "test",
					EnvVars: []string{tt.placeholder},
				}
				setupParams(params, tt.placeholder)

			case "env_element":
				template = &runnertypes.CommandTemplate{
					Cmd:     "test",
					EnvVars: []string{tt.placeholder},
				}
				setupParamsForEnvElement(params, tt.placeholder)

			case "workdir":
				template = &runnertypes.CommandTemplate{
					Cmd:     "test",
					WorkDir: runnertypes.StringPtr(tt.placeholder),
				}
				setupParams(params, tt.placeholder)
			}

			// Test expansion
			var err error
			switch tt.field {
			case "cmd":
				_, err = expandSingleArg(template.Cmd, params, "test_template", "cmd")
			case "args":
				_, err = ExpandTemplateArgs(template.Args, params, "test_template")
			case "env", "env_element":
				_, err = ExpandTemplateEnv(template.EnvVars, params, "test_template")
			case "workdir":
				if template.WorkDir != nil {
					_, err = expandSingleArg(*template.WorkDir, params, "test_template", "workdir")
				}
			}

			// Verify result
			if tt.wantErr {
				assert.Error(t, err, "%s: expected error but got nil\nDescription: %s",
					tt.name, tt.description)

				// Verify error type
				if tt.errType != nil {
					switch tt.errType.(type) {
					case *ErrTemplateCmdNotSingleValue:
						var target *ErrTemplateCmdNotSingleValue
						assert.ErrorAs(t, err, &target, "%s: expected ErrTemplateCmdNotSingleValue",
							tt.name)
					case *ErrArrayInMixedContext:
						var target *ErrArrayInMixedContext
						assert.ErrorAs(t, err, &target, "%s: expected ErrArrayInMixedContext",
							tt.name)
					case *ErrPlaceholderInEnvKey:
						var target *ErrPlaceholderInEnvKey
						assert.ErrorAs(t, err, &target, "%s: expected ErrPlaceholderInEnvKey",
							tt.name)
					}
				}
			} else {
				assert.NoError(t, err, "%s: unexpected error: %v\nDescription: %s",
					tt.name, err, tt.description)
			}
		})
	}
}

// setupParams creates appropriate parameter values based on placeholder content
func setupParams(params map[string]any, placeholder string) {
	// Extract parameter names from placeholder
	placeholders, _ := parsePlaceholders(placeholder)

	for _, ph := range placeholders {
		switch ph.ptype {
		case placeholderRequired, placeholderOptional:
			params[ph.name] = "/test/value"
		case placeholderArray:
			params[ph.name] = []string{"/test/value1", "/test/value2"}
		}
	}
}

// setupParamsForEnvElement creates env-appropriate parameter values (KEY=VALUE format)
func setupParamsForEnvElement(params map[string]any, placeholder string) {
	// Extract parameter names from placeholder
	placeholders, _ := parsePlaceholders(placeholder)

	for _, ph := range placeholders {
		switch ph.ptype {
		case placeholderRequired, placeholderOptional:
			params[ph.name] = "TEST_KEY=test_value"
		case placeholderArray:
			params[ph.name] = []string{"KEY1=value1", "KEY2=value2"}
		}
	}
}
