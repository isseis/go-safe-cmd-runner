//go:build test

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrDuplicateTemplateName_ErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      *ErrDuplicateTemplateName
		expected string
	}{
		{
			name: "single location",
			err: &ErrDuplicateTemplateName{
				Name:      "backup",
				Locations: []string{"/tmp/template1.toml"},
			},
			expected: `duplicate template name "backup"`,
		},
		{
			name: "no locations",
			err: &ErrDuplicateTemplateName{
				Name:      "backup",
				Locations: []string{},
			},
			expected: `duplicate template name "backup"`,
		},
		{
			name: "multiple locations",
			err: &ErrDuplicateTemplateName{
				Name: "backup",
				Locations: []string{
					"/tmp/template1.toml",
					"/tmp/template2.toml",
					"/tmp/template3.toml",
				},
			},
			expected: `duplicate command template name "backup"
  Defined in:
    - /tmp/template1.toml
    - /tmp/template2.toml
    - /tmp/template3.toml`,
		},
		{
			name: "two locations",
			err: &ErrDuplicateTemplateName{
				Name: "restore",
				Locations: []string{
					"/etc/config.toml",
					"/home/user/.config/config.toml",
				},
			},
			expected: `duplicate command template name "restore"
  Defined in:
    - /etc/config.toml
    - /home/user/.config/config.toml`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.err.Error()
			assert.Equal(t, tt.expected, actual)
		})
	}
}
