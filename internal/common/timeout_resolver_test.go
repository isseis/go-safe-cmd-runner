//nolint:revive // "common" is an appropriate name for shared utilities package
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveTimeout(t *testing.T) {
	tests := []struct {
		name           string
		cmdTimeout     *int
		groupTimeout   *int
		globalTimeout  *int
		expectedResult int
	}{
		{
			name:           "all nil - use default",
			cmdTimeout:     nil,
			groupTimeout:   nil,
			globalTimeout:  nil,
			expectedResult: DefaultTimeout,
		},
		{
			name:           "command timeout takes precedence",
			cmdTimeout:     IntPtr(120),
			groupTimeout:   IntPtr(90),
			globalTimeout:  IntPtr(60),
			expectedResult: 120,
		},
		{
			name:           "group timeout when command is nil",
			cmdTimeout:     nil,
			groupTimeout:   IntPtr(90),
			globalTimeout:  IntPtr(60),
			expectedResult: 90,
		},
		{
			name:           "global timeout when cmd and group are nil",
			cmdTimeout:     nil,
			groupTimeout:   nil,
			globalTimeout:  IntPtr(45),
			expectedResult: 45,
		},
		{
			name:           "command timeout 0 (unlimited)",
			cmdTimeout:     IntPtr(0),
			groupTimeout:   IntPtr(90),
			globalTimeout:  IntPtr(60),
			expectedResult: 0,
		},
		{
			name:           "group timeout 0 (unlimited)",
			cmdTimeout:     nil,
			groupTimeout:   IntPtr(0),
			globalTimeout:  IntPtr(60),
			expectedResult: 0,
		},
		{
			name:           "global timeout 0 (unlimited)",
			cmdTimeout:     nil,
			groupTimeout:   nil,
			globalTimeout:  IntPtr(0),
			expectedResult: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveTimeout(tt.cmdTimeout, tt.groupTimeout, tt.globalTimeout)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

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
			name:            "command level resolution",
			cmdTimeout:      IntPtr(120),
			groupTimeout:    IntPtr(90),
			globalTimeout:   IntPtr(60),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 120,
			expectedLevel:   "command",
		},
		{
			name:            "group level resolution",
			cmdTimeout:      nil,
			groupTimeout:    IntPtr(90),
			globalTimeout:   IntPtr(60),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 90,
			expectedLevel:   "group",
		},
		{
			name:            "global level resolution",
			cmdTimeout:      nil,
			groupTimeout:    nil,
			globalTimeout:   IntPtr(30),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 30,
			expectedLevel:   "global",
		},
		{
			name:            "default level resolution",
			cmdTimeout:      nil,
			groupTimeout:    nil,
			globalTimeout:   nil,
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: DefaultTimeout,
			expectedLevel:   "default",
		},
		{
			name:            "command unlimited takes precedence",
			cmdTimeout:      IntPtr(0),
			groupTimeout:    IntPtr(90),
			globalTimeout:   IntPtr(60),
			commandName:     "unlimited-cmd",
			groupName:       "test-group",
			expectedTimeout: 0,
			expectedLevel:   "command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout, context := ResolveTimeoutWithContext(
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
