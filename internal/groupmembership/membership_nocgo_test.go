//go:build !cgo

package groupmembership

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseGroupLine is specific to the no-CGO implementation
func TestParseGroupLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		expected    *groupEntry
		shouldError bool
	}{
		{
			name: "normal group with members",
			line: "adm:x:4:syslog,issei",
			expected: &groupEntry{
				name:    "adm",
				gid:     4,
				members: "syslog,issei",
			},
			shouldError: false,
		},
		{
			name: "group with no members",
			line: "root:x:0:",
			expected: &groupEntry{
				name:    "root",
				gid:     0,
				members: "",
			},
			shouldError: false,
		},
		{
			name:        "invalid line format",
			line:        "invalid:line",
			expected:    nil,
			shouldError: true,
		},
		{
			name:        "invalid GID",
			line:        "group:x:notanumber:members",
			expected:    nil,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseGroupLine(tt.line)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
