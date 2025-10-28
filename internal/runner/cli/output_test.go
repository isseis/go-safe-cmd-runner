package cli

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/stretchr/testify/assert"
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
			assert.NoError(t, err, "ParseDryRunDetailLevel(%q) should not error", tt.input)
			assert.Equal(t, tt.want, got, "ParseDryRunDetailLevel(%q) should equal %v", tt.input, tt.want)
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
			assert.Error(t, err, "ParseDryRunDetailLevel(%q) should error", tt.input)
			assert.True(t, errors.Is(err, ErrInvalidDetailLevel), "ParseDryRunDetailLevel(%q) error should be ErrInvalidDetailLevel", tt.input)
			assert.Equal(t, resource.DetailLevelSummary, got, "ParseDryRunDetailLevel(%q) should return default", tt.input)
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
			assert.NoError(t, err, "ParseDryRunOutputFormat(%q) should not error", tt.input)
			assert.Equal(t, tt.want, got, "ParseDryRunOutputFormat(%q) should equal %v", tt.input, tt.want)
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
			assert.Error(t, err, "ParseDryRunOutputFormat(%q) should error", tt.input)
			assert.True(t, errors.Is(err, ErrInvalidOutputFormat), "ParseDryRunOutputFormat(%q) error should be ErrInvalidOutputFormat", tt.input)
			assert.Equal(t, resource.OutputFormatText, got, "ParseDryRunOutputFormat(%q) should return default", tt.input)
		})
	}
}
