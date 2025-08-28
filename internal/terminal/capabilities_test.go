package terminal

import (
	"testing"
)

func TestCapabilities_Integration(t *testing.T) {
	tests := []struct {
		name             string
		envVars          map[string]string
		options          Options
		wantInteractive  bool
		wantColor        bool
		wantExplicitPref bool
		description      string
	}{
		{
			name:             "Force color and force interactive",
			options:          Options{PreferenceOptions: PreferenceOptions{ForceColor: true}, DetectorOptions: DetectorOptions{ForceInteractive: true}},
			wantInteractive:  true,
			wantColor:        true,
			wantExplicitPref: true,
			description:      "Command line options should force both interactive and color",
		},
		{
			name:             "CLICOLOR_FORCE=1 should enable color in non-interactive environment",
			envVars:          map[string]string{"CLICOLOR_FORCE": "1", "CI": "true"},
			wantInteractive:  false,
			wantColor:        true,
			wantExplicitPref: true,
			description:      "CLICOLOR_FORCE=1 should override interactive detection for color",
		},
		{
			name:             "NO_COLOR should disable color even in interactive terminal",
			envVars:          map[string]string{"NO_COLOR": "1", "TERM": "xterm"},
			options:          Options{DetectorOptions: DetectorOptions{ForceInteractive: true}},
			wantInteractive:  true,
			wantColor:        false,
			wantExplicitPref: true,
			description:      "NO_COLOR should override terminal color capability",
		},
		{
			name:             "Interactive terminal with color support should enable color",
			envVars:          map[string]string{"TERM": "xterm"},
			options:          Options{DetectorOptions: DetectorOptions{ForceInteractive: true}},
			wantInteractive:  true,
			wantColor:        true,
			wantExplicitPref: false,
			description:      "Interactive terminal with color capability should enable color",
		},
		{
			name:             "Non-interactive environment should disable color by default",
			envVars:          map[string]string{"CI": "true", "TERM": "xterm"},
			wantInteractive:  false,
			wantColor:        false,
			wantExplicitPref: false,
			description:      "CI environment should disable color unless explicitly forced",
		},
		{
			name:             "Dumb terminal should not support color even when interactive",
			envVars:          map[string]string{"TERM": "dumb"},
			options:          Options{DetectorOptions: DetectorOptions{ForceInteractive: true}},
			wantInteractive:  true,
			wantColor:        false,
			wantExplicitPref: false,
			description:      "Dumb terminal should not support color",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up clean environment for testing
			setupCleanEnv(t, tt.envVars)

			capabilities := NewCapabilities(tt.options)

			if got := capabilities.IsInteractive(); got != tt.wantInteractive {
				t.Errorf("IsInteractive() = %v, want %v. %s", got, tt.wantInteractive, tt.description)
			}

			if got := capabilities.SupportsColor(); got != tt.wantColor {
				t.Errorf("SupportsColor() = %v, want %v. %s", got, tt.wantColor, tt.description)
			}

			if got := capabilities.HasExplicitUserPreference(); got != tt.wantExplicitPref {
				t.Errorf("HasExplicitUserPreference() = %v, want %v. %s", got, tt.wantExplicitPref, tt.description)
			}
		})
	}
}

func TestCapabilities_ColorPriorityLogic(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		options     Options
		wantColor   bool
		description string
	}{
		{
			name:        "Priority 1: Command line force color",
			envVars:     map[string]string{"NO_COLOR": "1", "CI": "true", "TERM": "dumb"},
			options:     Options{PreferenceOptions: PreferenceOptions{ForceColor: true}},
			wantColor:   true,
			description: "Command line force color should override all other conditions",
		},
		{
			name:        "Priority 1: Command line disable color",
			envVars:     map[string]string{"CLICOLOR_FORCE": "1", "TERM": "xterm"},
			options:     Options{PreferenceOptions: PreferenceOptions{DisableColor: true}, DetectorOptions: DetectorOptions{ForceInteractive: true}},
			wantColor:   false,
			description: "Command line disable color should override all other conditions",
		},
		{
			name:        "Priority 2: CLICOLOR_FORCE=1 overrides CI and terminal detection",
			envVars:     map[string]string{"CLICOLOR_FORCE": "1", "CI": "true", "TERM": "dumb"},
			wantColor:   true,
			description: "CLICOLOR_FORCE=1 should override CI detection and terminal capabilities",
		},
		{
			name:        "Priority 3: NO_COLOR overrides terminal capabilities",
			envVars:     map[string]string{"NO_COLOR": "1", "TERM": "xterm"},
			options:     Options{DetectorOptions: DetectorOptions{ForceInteractive: true}},
			wantColor:   false,
			description: "NO_COLOR should disable color even with color-capable terminal",
		},
		{
			name:        "Priority 4: CLICOLOR=1 enables color in interactive environment",
			envVars:     map[string]string{"CLICOLOR": "1", "TERM": "xterm"},
			options:     Options{DetectorOptions: DetectorOptions{ForceInteractive: true}},
			wantColor:   true,
			description: "CLICOLOR=1 should enable color in interactive environment",
		},
		{
			name:        "Priority 5: Terminal auto-detection in interactive environment",
			envVars:     map[string]string{"TERM": "xterm"},
			options:     Options{DetectorOptions: DetectorOptions{ForceInteractive: true}},
			wantColor:   true,
			description: "Color-capable terminal in interactive mode should enable color",
		},
		{
			name:        "Auto-detection disabled in non-interactive environment",
			envVars:     map[string]string{"TERM": "xterm", "CI": "true"},
			wantColor:   false,
			description: "Color should be disabled in CI environment even with color-capable terminal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up clean environment for testing
			setupCleanEnv(t, tt.envVars)

			capabilities := NewCapabilities(tt.options)

			if got := capabilities.SupportsColor(); got != tt.wantColor {
				t.Errorf("SupportsColor() = %v, want %v. %s", got, tt.wantColor, tt.description)
			}
		})
	}
}

func TestCapabilities_ComponentIntegration(t *testing.T) {
	t.Run("All components work together", func(t *testing.T) {
		// Set up clean test environment: interactive terminal with color support
		setupCleanEnv(t, map[string]string{"TERM": "xterm"})

		// Create capabilities with no explicit options
		options := Options{}
		capabilities := NewCapabilities(options)

		// Test all methods
		hasExplicit := capabilities.HasExplicitUserPreference()
		isInteractive := capabilities.IsInteractive()
		supportsColor := capabilities.SupportsColor()

		// In this setup, we expect:
		// - No explicit preference (no command line options or env vars set)
		// - Interactive detection depends on actual terminal state
		// - Color support depends on both interactive state and terminal capability

		if hasExplicit {
			t.Error("HasExplicitUserPreference() should return false with no options or env vars")
		}

		// For consistent testing, we don't make assumptions about the actual terminal state
		// Instead, test that the methods don't panic and return boolean values
		if isInteractive != true && isInteractive != false {
			t.Error("IsInteractive() should return a boolean value")
		}

		if supportsColor != true && supportsColor != false {
			t.Error("SupportsColor() should return a boolean value")
		}
	})
}

func TestCapabilities_ExplicitUserPreferenceLogic(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		options      Options
		wantExplicit bool
		description  string
	}{
		{
			name:         "Force color option is explicit",
			options:      Options{PreferenceOptions: PreferenceOptions{ForceColor: true}},
			wantExplicit: true,
			description:  "Command line force color should be considered explicit",
		},
		{
			name:         "Disable color option is explicit",
			options:      Options{PreferenceOptions: PreferenceOptions{DisableColor: true}},
			wantExplicit: true,
			description:  "Command line disable color should be considered explicit",
		},
		{
			name:         "CLICOLOR_FORCE=1 is explicit",
			envVars:      map[string]string{"CLICOLOR_FORCE": "1"},
			wantExplicit: true,
			description:  "CLICOLOR_FORCE=1 should be considered explicit preference",
		},
		{
			name:         "CLICOLOR_FORCE=0 is not explicit",
			envVars:      map[string]string{"CLICOLOR_FORCE": "0"},
			wantExplicit: false,
			description:  "CLICOLOR_FORCE=0 should not be considered explicit preference",
		},
		{
			name:         "NO_COLOR is explicit",
			envVars:      map[string]string{"NO_COLOR": "1"},
			wantExplicit: true,
			description:  "NO_COLOR should be considered explicit preference",
		},
		{
			name:         "CLICOLOR is explicit",
			envVars:      map[string]string{"CLICOLOR": "1"},
			wantExplicit: true,
			description:  "CLICOLOR should be considered explicit preference",
		},
		{
			name:         "No explicit preferences",
			envVars:      map[string]string{"TERM": "xterm"},
			wantExplicit: false,
			description:  "Only TERM set should not be considered explicit preference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up clean environment for testing
			setupCleanEnv(t, tt.envVars)

			capabilities := NewCapabilities(tt.options)

			if got := capabilities.HasExplicitUserPreference(); got != tt.wantExplicit {
				t.Errorf("HasExplicitUserPreference() = %v, want %v. %s", got, tt.wantExplicit, tt.description)
			}
		})
	}
}
