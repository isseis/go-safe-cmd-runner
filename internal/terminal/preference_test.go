package terminal

import (
	"testing"
)

func TestUserPreference_CLICOLORForce(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		options      PreferenceOptions
		wantColor    bool
		wantExplicit bool
	}{
		{
			name:         "CLICOLOR_FORCE=1 overrides everything",
			envVars:      map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": "1"},
			wantColor:    true,
			wantExplicit: true,
		},
		{
			name:         "CLICOLOR_FORCE=0 allows other checks",
			envVars:      map[string]string{"CLICOLOR_FORCE": "0", "CLICOLOR": "1"},
			wantColor:    false, // CLICOLOR is now handled in capabilities, not preference
			wantExplicit: false, // CLICOLOR is no longer explicit
		},
		{
			name:         "NO_COLOR disables color",
			envVars:      map[string]string{"NO_COLOR": "1"},
			wantColor:    false,
			wantExplicit: true,
		},
		{
			name:         "CLICOLOR=0 disables color",
			envVars:      map[string]string{"CLICOLOR": "0"},
			wantColor:    false, // CLICOLOR is now handled in capabilities, not preference
			wantExplicit: false, // CLICOLOR is no longer explicit
		},
		{
			name:         "CLICOLOR=1 enables color",
			envVars:      map[string]string{"CLICOLOR": "1"},
			wantColor:    false, // CLICOLOR is now handled in capabilities, not preference
			wantExplicit: false, // CLICOLOR is no longer explicit
		},
		{
			name:         "force color option overrides env",
			envVars:      map[string]string{"NO_COLOR": "1"},
			options:      PreferenceOptions{ForceColor: true},
			wantColor:    true,
			wantExplicit: true,
		},
		{
			name:         "disable color option overrides env",
			envVars:      map[string]string{"CLICOLOR": "1"},
			options:      PreferenceOptions{DisableColor: true},
			wantColor:    false,
			wantExplicit: true,
		},
		{
			name:         "no preferences set",
			envVars:      map[string]string{}, // Empty map - NO_COLOR not set, other vars set to ""
			wantColor:    false,
			wantExplicit: false, // No explicit preferences when NO_COLOR is not set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Always set up clean environment for testing to ensure isolation from real environment
			setupCleanEnv(t, tt.envVars)

			pref := NewUserPreference(tt.options)

			if got := pref.SupportsColor(); got != tt.wantColor {
				t.Errorf("SupportsColor() = %v, want %v", got, tt.wantColor)
			}

			if got := pref.HasExplicitPreference(); got != tt.wantExplicit {
				t.Errorf("HasExplicitPreference() = %v, want %v", got, tt.wantExplicit)
			}
		})
	}
}

func TestUserPreference_PriorityLogic(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		options     PreferenceOptions
		wantColor   bool
		description string
	}{
		{
			name:        "Command line force has highest priority",
			envVars:     map[string]string{"NO_COLOR": "1", "CLICOLOR_FORCE": "0"},
			options:     PreferenceOptions{ForceColor: true},
			wantColor:   true,
			description: "Command line --force-color should override all other settings",
		},
		{
			name:        "Command line disable has highest priority",
			envVars:     map[string]string{"CLICOLOR_FORCE": "1"},
			options:     PreferenceOptions{DisableColor: true},
			wantColor:   false,
			description: "Command line --disable-color should override all other settings",
		},
		{
			name:        "CLICOLOR_FORCE=1 second highest priority",
			envVars:     map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": "1", "CLICOLOR": "0"},
			wantColor:   true,
			description: "CLICOLOR_FORCE=1 should override NO_COLOR and CLICOLOR",
		},
		{
			name:        "NO_COLOR third priority",
			envVars:     map[string]string{"NO_COLOR": "1", "CLICOLOR": "1"},
			wantColor:   false,
			description: "NO_COLOR should override CLICOLOR",
		},
		{
			name:        "CLICOLOR no longer handled in preference",
			envVars:     map[string]string{"CLICOLOR": "1"},
			wantColor:   false,
			description: "CLICOLOR is now handled in capabilities, not preference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Always set up clean environment for testing to ensure isolation from real environment
			setupCleanEnv(t, tt.envVars)

			pref := NewUserPreference(tt.options)

			if got := pref.SupportsColor(); got != tt.wantColor {
				t.Errorf("SupportsColor() = %v, want %v. %s", got, tt.wantColor, tt.description)
			}
		})
	}
}

func TestUserPreference_EnvVarParsing(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		wantColor    bool
		wantExplicit bool
	}{
		{
			name:         "CLICOLOR_FORCE=1",
			envVars:      map[string]string{"CLICOLOR_FORCE": "1"},
			wantColor:    true,
			wantExplicit: true,
		},
		{
			name:         "CLICOLOR_FORCE=true",
			envVars:      map[string]string{"CLICOLOR_FORCE": "true"},
			wantColor:    true,
			wantExplicit: true,
		},
		{
			name:         "CLICOLOR_FORCE=yes",
			envVars:      map[string]string{"CLICOLOR_FORCE": "yes"},
			wantColor:    true,
			wantExplicit: true,
		},
		{
			name:         "CLICOLOR_FORCE=0",
			envVars:      map[string]string{"CLICOLOR_FORCE": "0"},
			wantColor:    false,
			wantExplicit: false, // CLICOLOR_FORCE=0 is not explicit, NO_COLOR not set
		},
		{
			name:         "CLICOLOR_FORCE=false",
			envVars:      map[string]string{"CLICOLOR_FORCE": "false"},
			wantColor:    false,
			wantExplicit: false, // CLICOLOR_FORCE=false is not explicit, NO_COLOR not set
		},
		{
			name:         "CLICOLOR_FORCE=invalid",
			envVars:      map[string]string{"CLICOLOR_FORCE": "invalid"},
			wantColor:    false,
			wantExplicit: false, // CLICOLOR_FORCE=invalid is not explicit, NO_COLOR not set
		},
		{
			name:         "NO_COLOR set",
			envVars:      map[string]string{"NO_COLOR": "1"},
			wantColor:    false,
			wantExplicit: true,
		},
		{
			name:         "NO_COLOR empty",
			envVars:      map[string]string{"NO_COLOR": ""},
			wantColor:    false,
			wantExplicit: true,
		},
		{
			name:         "CLICOLOR=1",
			envVars:      map[string]string{"CLICOLOR": "1"},
			wantColor:    false, // CLICOLOR is now handled in capabilities, not preference
			wantExplicit: false, // CLICOLOR is no longer explicit - it only applies in interactive mode
		},
		{
			name:         "CLICOLOR=0",
			envVars:      map[string]string{"CLICOLOR": "0"},
			wantColor:    false, // CLICOLOR is now handled in capabilities, not preference
			wantExplicit: false, // CLICOLOR is no longer explicit - it only applies in interactive mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Always set up clean environment for testing to ensure isolation from real environment
			setupCleanEnv(t, tt.envVars)

			pref := NewUserPreference(PreferenceOptions{})

			if got := pref.SupportsColor(); got != tt.wantColor {
				t.Errorf("SupportsColor() = %v, want %v", got, tt.wantColor)
			}

			if got := pref.HasExplicitPreference(); got != tt.wantExplicit {
				t.Errorf("HasExplicitPreference() = %v, want %v", got, tt.wantExplicit)
			}
		})
	}
}
