package encoding

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncode(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple path",
			input:    "simple",
			expected: "simple",
			wantErr:  false,
		},
		{
			name:     "path with slash",
			input:    "path/to/file",
			expected: "path__slash__to__slash__file",
			wantErr:  false,
		},
		{
			name:     "path with multiple special chars",
			input:    `path\to:file*`,
			expected: "path__backslash__to__colon__file__asterisk__",
			wantErr:  false,
		},
		{
			name:     "path with space",
			input:    "file name.txt",
			expected: "file__space__name.txt",
			wantErr:  false,
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
			wantErr:  true,
		},
		{
			name:    "very long path",
			input:   strings.Repeat("a", 300),
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Encode(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple encoded path",
			input:    "simple",
			expected: "simple",
			wantErr:  false,
		},
		{
			name:     "encoded path with slash",
			input:    "path__slash__to__slash__file",
			expected: "path/to/file",
			wantErr:  false,
		},
		{
			name:     "encoded path with multiple special chars",
			input:    "path__backslash__to__colon__file__asterisk__",
			expected: `path\to:file*`,
			wantErr:  false,
		},
		{
			name:     "encoded path with space",
			input:    "file__space__name.txt",
			expected: "file name.txt",
			wantErr:  false,
		},
		{
			name:     "empty encoded path",
			input:    "",
			expected: "",
			wantErr:  true,
		},
		{
			name:    "fallback encoded path",
			input:   "sha256_abcdef123456",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Decode(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestEncodeWithFallback(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectFallback bool
	}{
		{
			name:           "simple path",
			input:          "simple",
			expectFallback: false,
		},
		{
			name:           "path with special chars",
			input:          "path/to/file",
			expectFallback: false,
		},
		{
			name:           "very long path",
			input:          strings.Repeat("a", 300),
			expectFallback: true,
		},
		{
			name:           "empty path",
			input:          "",
			expectFallback: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := EncodeWithFallback(tc.input)
			assert.Equal(t, tc.expectFallback, result.IsFallback)
			assert.Equal(t, len(tc.input), result.OriginalLength)
			assert.Equal(t, len(result.EncodedName), result.EncodedLength)

			if tc.expectFallback {
				assert.True(t, strings.HasPrefix(result.EncodedName, "sha256_"))
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	testCases := []string{
		"simple",
		"path/to/file",
		`path\to:file*`,
		"file name.txt",
		"file?with<many>special|chars",
	}

	for _, original := range testCases {
		t.Run(original, func(t *testing.T) {
			encoded, err := Encode(original)
			require.NoError(t, err)

			decoded, err := Decode(encoded)
			require.NoError(t, err)

			assert.Equal(t, original, decoded)
		})
	}
}

func TestIsNormalEncoding(t *testing.T) {
	assert.True(t, IsNormalEncoding("normal_path"))
	assert.False(t, IsNormalEncoding("sha256_abcdef123456"))
}

func TestIsFallbackEncoding(t *testing.T) {
	assert.False(t, IsFallbackEncoding("normal_path"))
	assert.True(t, IsFallbackEncoding("sha256_abcdef123456"))
}

func TestResult(t *testing.T) {
	result := Result{
		EncodedName:    "test",
		IsFallback:     false,
		OriginalLength: 10,
		EncodedLength:  4,
	}

	assert.True(t, result.IsNormalEncoding())
	assert.False(t, result.IsFallbackEncoding())

	result.IsFallback = true
	assert.False(t, result.IsNormalEncoding())
	assert.True(t, result.IsFallbackEncoding())
}
