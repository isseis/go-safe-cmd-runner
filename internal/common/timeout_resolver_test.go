//nolint:revive // "common" is an appropriate name for shared utilities package
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveTimeoutWithContext(t *testing.T) {
	tests := []struct {
		name            string
		cmdTimeout      *int
		groupTimeout    *int
		globalTimeout   *int
		commandName     string
		groupName       string
		expectedTimeout int
		expectedLevel   string
	}{
		{
			name:            "all nil - use default",
			cmdTimeout:      nil,
			groupTimeout:    nil,
			globalTimeout:   nil,
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: DefaultTimeout,
			expectedLevel:   "default",
		},
		{
			name:            "command timeout takes precedence",
			cmdTimeout:      IntPtr(120),
			groupTimeout:    IntPtr(90),
			globalTimeout:   IntPtr(60),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 120,
			expectedLevel:   "command",
		},
		{
			name:            "group timeout when command is nil",
			cmdTimeout:      nil,
			groupTimeout:    IntPtr(90),
			globalTimeout:   IntPtr(60),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 90,
			expectedLevel:   "group",
		},
		{
			name:            "global timeout when cmd and group are nil",
			cmdTimeout:      nil,
			groupTimeout:    nil,
			globalTimeout:   IntPtr(45),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 45,
			expectedLevel:   "global",
		},
		{
			name:            "command timeout 0 (unlimited)",
			cmdTimeout:      IntPtr(0),
			groupTimeout:    IntPtr(90),
			globalTimeout:   IntPtr(60),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 0,
			expectedLevel:   "command",
		},
		{
			name:            "group timeout 0 (unlimited)",
			cmdTimeout:      nil,
			groupTimeout:    IntPtr(0),
			globalTimeout:   IntPtr(60),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 0,
			expectedLevel:   "group",
		},
		{
			name:            "global timeout 0 (unlimited)",
			cmdTimeout:      nil,
			groupTimeout:    nil,
			globalTimeout:   IntPtr(0),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 0,
			expectedLevel:   "global",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout, context := ResolveTimeout(
				tt.cmdTimeout,
				tt.groupTimeout,
				tt.globalTimeout,
				tt.commandName,
				tt.groupName,
			)

			assert.Equal(t, tt.expectedTimeout, timeout)
			assert.Equal(t, tt.expectedLevel, context.Level)
			assert.Equal(t, tt.commandName, context.CommandName)
			assert.Equal(t, tt.groupName, context.GroupName)
		})
	}
}

func TestIsUnlimitedTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  int
		expected bool
	}{
		{
			name:     "zero timeout is unlimited",
			timeout:  0,
			expected: true,
		},
		{
			name:     "positive timeout is not unlimited",
			timeout:  60,
			expected: false,
		},
		{
			name:     "large timeout is not unlimited",
			timeout:  3600,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsUnlimitedTimeout(tt.timeout)
			assert.Equal(t, tt.expected, result)
		})
	}
}
