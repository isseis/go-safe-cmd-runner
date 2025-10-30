package color

import (
	"strings"
	"testing"
)

func TestNewColor(t *testing.T) {
	testColor := NewColor("\033[31m") // Red
	result := testColor("ERROR")
	expected := "\033[31mERROR\033[0m"

	if result != expected {
		t.Errorf("NewColor() = %q, want %q", result, expected)
	}
}

func TestPredefinedColors(t *testing.T) {
	tests := []struct {
		name      string
		colorFunc Color
		input     string
		expected  string
	}{
		{"Red", Red, "ERROR", "\033[31mERROR\033[0m"},
		{"Green", Green, "INFO", "\033[32mINFO\033[0m"},
		{"Yellow", Yellow, "WARN", "\033[33mWARN\033[0m"},
		{"Gray", Gray, "DEBUG", "\033[90mDEBUG\033[0m"},
		{"Blue", Blue, "BLUE", "\033[34mBLUE\033[0m"},
		{"Purple", Purple, "PURPLE", "\033[35mPURPLE\033[0m"},
		{"Cyan", Cyan, "CYAN", "\033[36mCYAN\033[0m"},
		{"White", White, "WHITE", "\033[37mWHITE\033[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.colorFunc(tt.input)
			if result != tt.expected {
				t.Errorf("%s() = %q, want %q", tt.name, result, tt.expected)
			}
		})
	}
}

func TestColorResetHandling(t *testing.T) {
	// Test that colors properly reset and don't interfere with each other
	redText := Red("ERROR")
	greenText := Green("INFO")

	// Verify both contain reset codes
	if !strings.HasSuffix(redText, resetCode) {
		t.Error("Red text does not end with reset code")
	}
	if !strings.HasSuffix(greenText, resetCode) {
		t.Error("Green text does not end with reset code")
	}

	// Verify colors start with correct codes
	if !strings.HasPrefix(redText, redCode) {
		t.Error("Red text does not start with red code")
	}
	if !strings.HasPrefix(greenText, greenCode) {
		t.Error("Green text does not start with green code")
	}
}
