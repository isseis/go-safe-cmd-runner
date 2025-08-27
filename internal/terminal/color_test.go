package terminal

import (
	"os"
	"testing"
)

func TestColorDetector_SupportsColor(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantColor   bool
		description string
	}{
		{
			name:        "xterm supports color",
			envVars:     map[string]string{"TERM": "xterm"},
			wantColor:   true,
			description: "xterm is a common terminal that supports color",
		},
		{
			name:        "xterm-256color supports color",
			envVars:     map[string]string{"TERM": "xterm-256color"},
			wantColor:   true,
			description: "xterm-256color explicitly supports color",
		},
		{
			name:        "screen supports color",
			envVars:     map[string]string{"TERM": "screen"},
			wantColor:   true,
			description: "screen terminal supports color",
		},
		{
			name:        "dumb terminal does not support color",
			envVars:     map[string]string{"TERM": "dumb"},
			wantColor:   false,
			description: "dumb terminal explicitly does not support color",
		},
		{
			name:        "empty TERM does not support color",
			envVars:     map[string]string{"TERM": ""},
			wantColor:   false,
			description: "empty TERM variable means no color support",
		},
		{
			name:        "no TERM variable does not support color",
			envVars:     map[string]string{},
			wantColor:   false,
			description: "missing TERM variable means no color support",
		},
		{
			name:        "vt100 supports basic color",
			envVars:     map[string]string{"TERM": "vt100"},
			wantColor:   true,
			description: "vt100 supports basic color capabilities",
		},
		{
			name:        "ansi supports color",
			envVars:     map[string]string{"TERM": "ansi"},
			wantColor:   true,
			description: "ansi terminal supports color",
		},
		{
			name:        "linux supports color",
			envVars:     map[string]string{"TERM": "linux"},
			wantColor:   true,
			description: "linux console supports color",
		},
		{
			name:        "unknown terminal defaults to no color",
			envVars:     map[string]string{"TERM": "unknown-terminal"},
			wantColor:   false,
			description: "unknown terminal types default to no color for safety",
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

			detector := NewColorDetector()

			if got := detector.SupportsColor(); got != tt.wantColor {
				t.Errorf("SupportsColor() = %v, want %v. %s", got, tt.wantColor, tt.description)
			}
		})
	}
}

func TestColorDetector_CommonTerminals(t *testing.T) {
	// Test common terminal types found in the wild
	supportedTerminals := []string{
		"xterm",
		"xterm-color",
		"xterm-256color",
		"screen",
		"screen-256color",
		"tmux",
		"tmux-256color",
		"rxvt",
		"rxvt-unicode",
		"rxvt-unicode-256color",
		"vt100",
		"vt220",
		"ansi",
		"linux",
		"cygwin",
		"putty",
	}

	for _, terminal := range supportedTerminals {
		t.Run(terminal, func(t *testing.T) {
			os.Clearenv()
			os.Setenv("TERM", terminal)

			detector := NewColorDetector()

			if !detector.SupportsColor() {
				t.Errorf("Terminal %s should support color but doesn't", terminal)
			}
		})
	}
}

func TestColorDetector_NonColorTerminals(t *testing.T) {
	// Test terminals that should not support color
	tests := []struct {
		name     string
		terminal string
	}{
		{"dumb terminal", "dumb"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			os.Setenv("TERM", tt.terminal)

			detector := NewColorDetector()

			if detector.SupportsColor() {
				t.Errorf("Terminal '%s' should not support color but does", tt.terminal)
			}
		})
	}
}

func TestColorDetector_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name      string
		termValue string
		wantColor bool
	}{
		{"lowercase xterm", "xterm", true},
		{"uppercase XTERM", "XTERM", true},
		{"mixed case XTerm", "XTerm", true},
		{"lowercase dumb", "dumb", false},
		{"uppercase DUMB", "DUMB", false},
		{"mixed case Dumb", "Dumb", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			os.Setenv("TERM", tt.termValue)

			detector := NewColorDetector()

			if got := detector.SupportsColor(); got != tt.wantColor {
				t.Errorf("SupportsColor() with TERM=%s = %v, want %v", tt.termValue, got, tt.wantColor)
			}
		})
	}
}
