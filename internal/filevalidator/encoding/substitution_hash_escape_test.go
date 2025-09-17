package encoding

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubstitutionHashEscape_Encode(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple path",
			input:    "/usr/bin/python3",
			expected: "~usr~bin~python3",
		},
		{
			name:     "path with hash character",
			input:    "/home/user#test/file",
			expected: "~home~user#1test~file",
		},
		{
			name:     "path with tilde character",
			input:    "/home/~user/file",
			expected: "~home~##user~file",
		},
		{
			name:     "complex path",
			input:    "/path/with#hash/and~tilde/file",
			expected: "~path~with#1hash~and##tilde~file",
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "~",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encoder.Encode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubstitutionHashEscape_Decode(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple encoded path",
			input:    "~usr~bin~python3",
			expected: "/usr/bin/python3",
			wantErr:  false,
		},
		{
			name:     "fallback format",
			input:    "AbCdEf123456.json",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "complex encoded path",
			input:    "~path~with#1hash~and##tilde~file",
			expected: "/path/with#hash/and~tilde/file",
			wantErr:  false,
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.Decode(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSubstitutionHashEscape_RoundTrip(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	// Property-based test: encode then decode should return original
	testPaths := []string{
		"/usr/bin/python3",
		"/home/user_name/project_files",
		"/path/with#special/chars~here",
		"/very/deep/nested/directory/structure/file.txt",
		"/",
		"/single",
	}

	for _, originalPath := range testPaths {
		t.Run(originalPath, func(t *testing.T) {
			// Encode
			encoded := encoder.Encode(originalPath)

			// Decode
			decoded, err := encoder.Decode(encoded)

			// Verify round-trip
			require.NoError(t, err)
			assert.Equal(t, originalPath, decoded, "Round-trip failed for path: %s", originalPath)
		})
	}
}

func TestSubstitutionHashEscape_NameMaxFallback(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name         string
		path         string
		wantFallback bool
	}{
		{
			name:         "short path uses normal encoding",
			path:         "/usr/bin/python3",
			wantFallback: false,
		},
		{
			name:         "very long path uses fallback",
			path:         "/" + strings.Repeat("very-long-directory-name", 15) + "/file.txt",
			wantFallback: true,
		},
		{
			name:         "edge case near limit",
			path:         "/" + strings.Repeat("a", 248) + "/f", // Should encode to 251 chars
			wantFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encoder.EncodeWithFallback(tt.path)

			assert.Equal(t, tt.wantFallback, result.IsFallback)

			if result.IsFallback {
				// Fallback should not start with `~`
				assert.NotEqual(t, byte('~'), result.EncodedName[0])
				// Fallback should be within length limits
				assert.LessOrEqual(t, len(result.EncodedName), MaxFilenameLength)
			} else {
				// Normal encoding should start with `~` (for full paths)
				assert.Equal(t, byte('~'), result.EncodedName[0])
				// Should be reversible
				decoded, err := encoder.Decode(result.EncodedName)
				assert.NoError(t, err)
				assert.Equal(t, tt.path, decoded)
			}
		})
	}
}

func TestSubstitutionHashEscape_AnalyzeEncoding(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name              string
		input             string
		expectFallback    bool
		expectNonZeroFreq bool
	}{
		{
			name:              "simple path analysis",
			input:             "/usr/bin/python3",
			expectFallback:    false,
			expectNonZeroFreq: true,
		},
		{
			name:              "empty path analysis",
			input:             "",
			expectFallback:    false,
			expectNonZeroFreq: false,
		},
		{
			name:              "long path fallback analysis",
			input:             "/" + strings.Repeat("very-long-directory-name", 15) + "/file.txt",
			expectFallback:    true,
			expectNonZeroFreq: false, // No frequency analysis for fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := encoder.AnalyzeEncoding(tt.input)

			assert.Equal(t, tt.input, analysis.OriginalPath)
			assert.Equal(t, tt.expectFallback, analysis.IsFallback)
			assert.Equal(t, len(tt.input), analysis.OriginalLength)
			assert.Equal(t, len(analysis.EncodedName), analysis.EncodedLength)

			if tt.input != "" {
				assert.GreaterOrEqual(t, analysis.ExpansionRatio, 0.0)
			} else {
				assert.Equal(t, 0.0, analysis.ExpansionRatio)
			}

			if tt.expectNonZeroFreq {
				assert.NotNil(t, analysis.CharFrequency)
				assert.Greater(t, len(analysis.CharFrequency), 0)
			}
		})
	}
}

func TestSubstitutionHashEscape_IsNormalEncoding(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "normal encoding starts with tilde",
			input:    "~usr~bin~python3",
			expected: true,
		},
		{
			name:     "fallback encoding doesn't start with tilde",
			input:    "AbCdEf123456.json",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encoder.IsNormalEncoding(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
