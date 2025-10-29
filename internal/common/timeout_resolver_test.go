//nolint:revive // "common" is an appropriate name for shared utilities package
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveTimeout(t *testing.T) {
	tests := []struct {
		name            string
		cmdTimeout      Timeout
		groupTimeout    Timeout
		globalTimeout   Timeout
		commandName     string
		groupName       string
		expectedTimeout int
		expectedLevel   string
	}{
		// Original TestResolveTimeout test cases merged here
		{
			name:            "all unset - use default",
			cmdTimeout:      NewUnsetTimeout(),
			groupTimeout:    NewUnsetTimeout(),
			globalTimeout:   NewUnsetTimeout(),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: DefaultTimeout,
			expectedLevel:   "default",
		},
		{
			name:            "command timeout takes precedence",
			cmdTimeout:      NewFromIntPtr(IntPtr(120)),
			groupTimeout:    NewFromIntPtr(IntPtr(90)),
			globalTimeout:   NewFromIntPtr(IntPtr(60)),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 120,
			expectedLevel:   "command",
		},
		{
			name:            "group timeout when command is unset",
			cmdTimeout:      NewUnsetTimeout(),
			groupTimeout:    NewFromIntPtr(IntPtr(90)),
			globalTimeout:   NewFromIntPtr(IntPtr(60)),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 90,
			expectedLevel:   "group",
		},
		{
			name:            "global timeout when cmd and group are unset",
			cmdTimeout:      NewUnsetTimeout(),
			groupTimeout:    NewUnsetTimeout(),
			globalTimeout:   NewFromIntPtr(IntPtr(45)),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 45,
			expectedLevel:   "global",
		},
		{
			name:            "command timeout 0 (unlimited)",
			cmdTimeout:      NewFromIntPtr(IntPtr(0)),
			groupTimeout:    NewFromIntPtr(IntPtr(90)),
			globalTimeout:   NewFromIntPtr(IntPtr(60)),
			commandName:     "unlimited-cmd",
			groupName:       "test-group",
			expectedTimeout: 0,
			expectedLevel:   "command",
		},
		{
			name:            "group timeout 0 (unlimited)",
			cmdTimeout:      NewUnsetTimeout(),
			groupTimeout:    NewFromIntPtr(IntPtr(0)),
			globalTimeout:   NewFromIntPtr(IntPtr(60)),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 0,
			expectedLevel:   "group",
		},
		{
			name:            "global timeout 0 (unlimited)",
			cmdTimeout:      NewUnsetTimeout(),
			groupTimeout:    NewUnsetTimeout(),
			globalTimeout:   NewFromIntPtr(IntPtr(0)),
			commandName:     "test-cmd",
			groupName:       "test-group",
			expectedTimeout: 0,
			expectedLevel:   "global",
		},
		// Original TestResolveTimeoutWithContext test cases
		{
			name:            "command level resolution with context",
			cmdTimeout:      NewFromIntPtr(IntPtr(30)),
			groupTimeout:    NewUnsetTimeout(),
			globalTimeout:   NewFromIntPtr(IntPtr(60)),
			commandName:     "test-command",
			groupName:       "test-group",
			expectedTimeout: 30,
			expectedLevel:   "command",
		},
		{
			name:            "global level resolution with context",
			cmdTimeout:      NewUnsetTimeout(),
			groupTimeout:    NewUnsetTimeout(),
			globalTimeout:   NewFromIntPtr(IntPtr(60)),
			commandName:     "test-command",
			groupName:       "test-group",
			expectedTimeout: 60,
			expectedLevel:   "global",
		},
		{
			name:            "default timeout with context",
			cmdTimeout:      NewUnsetTimeout(),
			groupTimeout:    NewUnsetTimeout(),
			globalTimeout:   NewUnsetTimeout(),
			commandName:     "test-command",
			groupName:       "test-group",
			expectedTimeout: DefaultTimeout,
			expectedLevel:   "default",
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
