package logging

import (
	"testing"
)

func TestNewMessageTemplates(t *testing.T) {
	mt := NewMessageTemplates()
	if mt == nil {
		t.Error("NewMessageTemplates should return a non-nil instance")
	}
}

func TestMessageTemplates_FormatCommandMessage(t *testing.T) {
	mt := NewMessageTemplates()

	tests := []struct {
		name        string
		template    string
		commandName string
		attrs       map[string]interface{}
		expected    string
	}{
		{
			name:        "basic command message",
			template:    CommandStartTemplate,
			commandName: "test-cmd",
			attrs:       nil,
			expected:    "Starting command execution command=test-cmd",
		},
		{
			name:        "command complete message",
			template:    CommandCompleteTemplate,
			commandName: "build",
			attrs:       map[string]interface{}{"duration": "5s"},
			expected:    "Command execution completed command=build",
		},
		{
			name:        "empty command name",
			template:    CommandFailedTemplate,
			commandName: "",
			attrs:       nil,
			expected:    "Command execution failed command=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mt.FormatCommandMessage(tt.template, tt.commandName, tt.attrs)
			if result != tt.expected {
				t.Errorf("FormatCommandMessage() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestMessageTemplates_FormatSecurityMessage(t *testing.T) {
	mt := NewMessageTemplates()

	tests := []struct {
		name      string
		template  string
		operation string
		severity  string
		expected  string
	}{
		{
			name:      "security check message",
			template:  SecurityCheckTemplate,
			operation: "file_access",
			severity:  "high",
			expected:  "Performing security validation operation=file_access severity=high",
		},
		{
			name:      "security denied message",
			template:  SecurityDeniedTemplate,
			operation: "command_exec",
			severity:  "critical",
			expected:  "Security check denied access operation=command_exec severity=critical",
		},
		{
			name:      "security warning message",
			template:  SecurityWarningTemplate,
			operation: "path_traversal",
			severity:  "medium",
			expected:  "Security warning detected operation=path_traversal severity=medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mt.FormatSecurityMessage(tt.template, tt.operation, tt.severity)
			if result != tt.expected {
				t.Errorf("FormatSecurityMessage() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestMessageTemplates_FormatSystemMessage(t *testing.T) {
	mt := NewMessageTemplates()

	tests := []struct {
		name      string
		template  string
		component string
		expected  string
	}{
		{
			name:      "system start message",
			template:  SystemStartTemplate,
			component: "runner",
			expected:  "System initialization started component=runner",
		},
		{
			name:      "system ready message",
			template:  SystemReadyTemplate,
			component: "executor",
			expected:  "System ready for operation component=executor",
		},
		{
			name:      "system shutdown message",
			template:  SystemShutdownTemplate,
			component: "logger",
			expected:  "System shutdown initiated component=logger",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mt.FormatSystemMessage(tt.template, tt.component)
			if result != tt.expected {
				t.Errorf("FormatSystemMessage() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestMessageTemplateConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"CommandStartTemplate", CommandStartTemplate, "Starting command execution"},
		{"CommandCompleteTemplate", CommandCompleteTemplate, "Command execution completed"},
		{"CommandFailedTemplate", CommandFailedTemplate, "Command execution failed"},
		{"EnvLoadTemplate", EnvLoadTemplate, "Loading environment variables"},
		{"EnvProcessTemplate", EnvProcessTemplate, "Processing environment configuration"},
		{"EnvValidateTemplate", EnvValidateTemplate, "Validating environment variables"},
		{"FileReadTemplate", FileReadTemplate, "Reading file"},
		{"FileWriteTemplate", FileWriteTemplate, "Writing file"},
		{"FileValidateTemplate", FileValidateTemplate, "Validating file integrity"},
		{"SecurityCheckTemplate", SecurityCheckTemplate, "Performing security validation"},
		{"SecurityDeniedTemplate", SecurityDeniedTemplate, "Security check denied access"},
		{"SecurityWarningTemplate", SecurityWarningTemplate, "Security warning detected"},
		{"SystemStartTemplate", SystemStartTemplate, "System initialization started"},
		{"SystemReadyTemplate", SystemReadyTemplate, "System ready for operation"},
		{"SystemShutdownTemplate", SystemShutdownTemplate, "System shutdown initiated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestLogFileHintTemplates(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"LogFileHintPrefix", LogFileHintPrefix, "Check log file around line"},
		{"LogFileHintSuffix", LogFileHintSuffix, "for more details"},
		{"LogFileHintFullFormat", LogFileHintFullFormat, "Check log file around line %d for more details"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}
