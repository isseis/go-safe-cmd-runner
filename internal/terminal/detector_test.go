package terminal

import (
	"os"
	"testing"
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
			name:            "No CI environment - depends on terminal check",
			envVars:         map[string]string{},
			wantInteractive: false, // In tests, stdout/stderr are not terminals
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			detector := NewInteractiveDetector(tt.options)

			if got := detector.IsInteractive(); got != tt.wantInteractive {
				t.Errorf("IsInteractive() = %v, want %v", got, tt.wantInteractive)
			}
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
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			detector := NewInteractiveDetector(DetectorOptions{})

			if got := detector.IsCIEnvironment(); got != tt.wantCI {
				t.Errorf("IsCIEnvironment() = %v, want %v", got, tt.wantCI)
			}
		})
	}
}

func TestInteractiveDetector_IsTerminal(t *testing.T) {
	// Note: In test environment, stdout/stderr are typically not terminals
	// This test verifies the method exists and returns a reasonable value

	detector := NewInteractiveDetector(DetectorOptions{})

	// Should return false in test environment
	if got := detector.IsTerminal(); got != false {
		t.Errorf("IsTerminal() = %v, want %v (in test environment)", got, false)
	}
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
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			detector := NewInteractiveDetector(tt.options)

			if got := detector.IsInteractive(); got != tt.wantInteractive {
				t.Errorf("IsInteractive() = %v, want %v. %s", got, tt.wantInteractive, tt.description)
			}
		})
	}
}
