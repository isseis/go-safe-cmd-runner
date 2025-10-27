package cli

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

func TestParseDryRunDetailLevel_ValidLevels(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  resource.DryRunDetailLevel
	}{
		{
			name:  "summary level",
			input: "summary",
			want:  resource.DetailLevelSummary,
		},
		{
			name:  "detailed level",
			input: "detailed",
			want:  resource.DetailLevelDetailed,
		},
		{
			name:  "full level",
			input: "full",
			want:  resource.DetailLevelFull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDryRunDetailLevel(tt.input)
			if err != nil {
				t.Errorf("ParseDryRunDetailLevel(%q) error = %v, want nil", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseDryRunDetailLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDryRunDetailLevel_InvalidLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid level",
			input: "invalid",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "wrong case",
			input: "SUMMARY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDryRunDetailLevel(tt.input)
			if err == nil {
				t.Errorf("ParseDryRunDetailLevel(%q) error = nil, want error", tt.input)
			}
			if !errors.Is(err, ErrInvalidDetailLevel) {
				t.Errorf("ParseDryRunDetailLevel(%q) error = %v, want ErrInvalidDetailLevel", tt.input, err)
			}
			// Should return default value on error
			if got != resource.DetailLevelSummary {
				t.Errorf("ParseDryRunDetailLevel(%q) = %v, want %v (default)", tt.input, got, resource.DetailLevelSummary)
			}
		})
	}
}

func TestParseDryRunOutputFormat_ValidFormats(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  resource.OutputFormat
	}{
		{
			name:  "text format",
			input: "text",
			want:  resource.OutputFormatText,
		},
		{
			name:  "json format",
			input: "json",
			want:  resource.OutputFormatJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDryRunOutputFormat(tt.input)
			if err != nil {
				t.Errorf("ParseDryRunOutputFormat(%q) error = %v, want nil", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseDryRunOutputFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDryRunOutputFormat_InvalidFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid format",
			input: "xml",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "wrong case",
			input: "TEXT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDryRunOutputFormat(tt.input)
			if err == nil {
				t.Errorf("ParseDryRunOutputFormat(%q) error = nil, want error", tt.input)
			}
			if !errors.Is(err, ErrInvalidOutputFormat) {
				t.Errorf("ParseDryRunOutputFormat(%q) error = %v, want ErrInvalidOutputFormat", tt.input, err)
			}
			// Should return default value on error
			if got != resource.OutputFormatText {
				t.Errorf("ParseDryRunOutputFormat(%q) = %v, want %v (default)", tt.input, got, resource.OutputFormatText)
			}
		})
	}
}
