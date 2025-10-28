package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInteractiveDetector_IsInteractive(t *testing.T) {
	tests := []struct {
		name            string
		envVars         map[string]string
		options         DetectorOptions
		wantInteractive bool
	}{
		{
			name:            "CI environment detected - GITHUB_ACTIONS",
			envVars:         map[string]string{"GITHUB_ACTIONS": "true"},
			wantInteractive: false,
		},
		{
			name:            "CI environment detected - CI=true",
			envVars:         map[string]string{"CI": "true"},
			wantInteractive: false,
		},
		{
			name:            "CI environment detected - JENKINS_URL",
			envVars:         map[string]string{"JENKINS_URL": "http://jenkins.example.com"},
			wantInteractive: false,
		},
		{
			name:            "CI environment detected - BUILD_NUMBER",
			envVars:         map[string]string{"BUILD_NUMBER": "123"},
			wantInteractive: false,
		},
		{
			name:            "CI environment detected - CONTINUOUS_INTEGRATION",
			envVars:         map[string]string{"CONTINUOUS_INTEGRATION": "true"},
			wantInteractive: false,
		},
		{
			name:            "Force interactive mode overrides CI",
			envVars:         map[string]string{"CI": "true"},
			options:         DetectorOptions{ForceInteractive: true},
			wantInteractive: true,
		},
		{
			name:            "Force non-interactive mode",
			envVars:         map[string]string{},
			options:         DetectorOptions{ForceNonInteractive: true},
			wantInteractive: false,
		},
		{
			name:            "TERM environment variable indicates terminal",
			envVars:         map[string]string{"TERM": "xterm-256color"},
			wantInteractive: true,
		},
		{
			name:            "TERM=dumb should not be interactive",
			envVars:         map[string]string{"TERM": "dumb"},
			wantInteractive: false,
		},
		{
			name:            "Empty TERM should not be interactive",
			envVars:         map[string]string{"TERM": ""},
			wantInteractive: false,
		},
		{
			name:            "No CI environment - depends on terminal check",
			envVars:         map[string]string{},
			wantInteractive: false, // In tests, stdout/stderr are not terminals
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up clean environment for testing
			setupCleanEnv(t, tt.envVars)

			detector := NewInteractiveDetector(tt.options)

			got := detector.IsInteractive()
			assert.Equal(t, tt.wantInteractive, got)
		})
	}
}

func TestInteractiveDetector_IsCIEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantCI  bool
	}{
		{
			name:    "GITHUB_ACTIONS",
			envVars: map[string]string{"GITHUB_ACTIONS": "true"},
			wantCI:  true,
		},
		{
			name:    "CI=true",
			envVars: map[string]string{"CI": "true"},
			wantCI:  true,
		},
		{
			name:    "CI=1",
			envVars: map[string]string{"CI": "1"},
			wantCI:  true,
		},
		{
			name:    "JENKINS_URL set",
			envVars: map[string]string{"JENKINS_URL": "http://jenkins.example.com"},
			wantCI:  true,
		},
		{
			name:    "BUILD_NUMBER set",
			envVars: map[string]string{"BUILD_NUMBER": "123"},
			wantCI:  true,
		},
		{
			name:    "CONTINUOUS_INTEGRATION=true",
			envVars: map[string]string{"CONTINUOUS_INTEGRATION": "true"},
			wantCI:  true,
		},
		{
			name:    "TRAVIS=true",
			envVars: map[string]string{"TRAVIS": "true"},
			wantCI:  true,
		},
		{
			name:    "CIRCLECI=true",
			envVars: map[string]string{"CIRCLECI": "true"},
			wantCI:  true,
		},
		{
			name:    "APPVEYOR=True",
			envVars: map[string]string{"APPVEYOR": "True"},
			wantCI:  true,
		},
		{
			name:    "GITLAB_CI=true",
			envVars: map[string]string{"GITLAB_CI": "true"},
			wantCI:  true,
		},
		{
			name:    "CI=false",
			envVars: map[string]string{"CI": "false"},
			wantCI:  false,
		},
		{
			name:    "No CI variables",
			envVars: map[string]string{},
			wantCI:  false,
		},
		{
			name:    "Multiple CI indicators",
			envVars: map[string]string{"CI": "true", "GITHUB_ACTIONS": "true"},
			wantCI:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up clean environment for testing
			setupCleanEnv(t, tt.envVars)

			detector := NewInteractiveDetector(DetectorOptions{})

			got := detector.IsCIEnvironment()
			assert.Equal(t, tt.wantCI, got)
		})
	}
}

func TestInteractiveDetector_IsTerminal(t *testing.T) {
	// Note: IsTerminal() checks if stdout/stderr are connected to actual terminals
	// using term.IsTerminal(). In test environments, this typically returns false
	// since tests run with pipes/redirected output.

	detector := NewInteractiveDetector(DetectorOptions{})

	// Test basic functionality - should return consistent boolean value
	result1 := detector.IsTerminal()
	result2 := detector.IsTerminal()

	assert.Equal(t, result1, result2, "IsTerminal should return consistent results")

	// In most test environments, stdout/stderr are not terminals
	// This is expected behavior, but we test for consistency rather than specific value
	t.Logf("IsTerminal() returned %v in test environment", result1)

	// Test that the method exists and is callable (regression test)
	result3 := detector.IsTerminal()
	assert.Equal(t, result1, result3, "IsTerminal should return same value on subsequent call")
}

func TestInteractiveDetector_PriorityLogic(t *testing.T) {
	tests := []struct {
		name            string
		envVars         map[string]string
		options         DetectorOptions
		wantInteractive bool
		description     string
	}{
		{
			name:            "Force interactive has highest priority",
			envVars:         map[string]string{"CI": "true"},
			options:         DetectorOptions{ForceInteractive: true},
			wantInteractive: true,
			description:     "ForceInteractive should override CI environment",
		},
		{
			name:            "Force non-interactive has highest priority",
			envVars:         map[string]string{}, // Terminal might be detected in some environments
			options:         DetectorOptions{ForceNonInteractive: true},
			wantInteractive: false,
			description:     "ForceNonInteractive should override terminal detection",
		},
		{
			name:            "CI environment second priority",
			envVars:         map[string]string{"CI": "true"},
			options:         DetectorOptions{},
			wantInteractive: false,
			description:     "CI environment should disable interactive mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up clean environment for testing
			setupCleanEnv(t, tt.envVars)

			detector := NewInteractiveDetector(tt.options)

			got := detector.IsInteractive()
			assert.Equal(t, tt.wantInteractive, got, tt.description)
		})
	}
}
