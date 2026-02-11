package pathencoding_test

import (
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator/pathencoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubstitutionHashEscape_Encode(t *testing.T) {
	encoder := pathencoding.NewSubstitutionHashEscape()

	tests := []struct {
		name        string
		input       string
		expected    string
		expectedErr bool
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
			name:        "empty path",
			input:       "",
			expectedErr: true,
		},
		{
			name:     "root path",
			input:    "/",
			expected: "~",
		},
		{
			name:     "long path without special chars",
			input:    "/usr/local/share/applications/very/long/directory/structure/file.txt",
			expected: "~usr~local~share~applications~very~long~directory~structure~file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.Encode(tt.input)
			if tt.expectedErr {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSubstitutionHashEscape_Encode_ErrorCases(t *testing.T) {
	encoder := pathencoding.NewSubstitutionHashEscape()

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "absolute path should succeed",
			input:       "/usr/bin/python3",
			expectError: false,
		},
		{
			name:        "relative path should fail",
			input:       "usr/bin/python3",
			expectError: true,
		},
		{
			name:        "relative path with dot should fail",
			input:       "./local/file",
			expectError: true,
		},
		{
			name:        "relative path with double dot should fail",
			input:       "../parent/file",
			expectError: true,
		},
		{
			name:        "path with dot component should fail",
			input:       "/path/./file",
			expectError: true,
		},
		{
			name:        "path with double dot component should fail",
			input:       "/path/../file",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.Encode(tt.input)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
			}
		})
	}
}

func TestSubstitutionHashEscape_Decode(t *testing.T) {
	encoder := pathencoding.NewSubstitutionHashEscape()

	tests := []struct {
		name     string
		encoded  string
		expected string
	}{
		{
			name:     "simple encoded path",
			encoded:  "~usr~bin~python3",
			expected: "/usr/bin/python3",
		},
		{
			name:     "path with escaped hash",
			encoded:  "~home~user#1test~file",
			expected: "/home/user#test/file",
		},
		{
			name:     "path with escaped tilde",
			encoded:  "~home~##user~file",
			expected: "/home/~user/file",
		},
		{
			name:     "complex encoded path",
			encoded:  "~path~with#1hash~and##tilde~file",
			expected: "/path/with#hash/and~tilde/file",
		},
		{
			name:     "root path",
			encoded:  "~",
			expected: "/",
		},
		{
			name:     "empty string",
			encoded:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.Decode(tt.encoded)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubstitutionHashEscape_Decode_FallbackError(t *testing.T) {
	encoder := pathencoding.NewSubstitutionHashEscape()

	// Test fallback encoded strings (those that don't start with ~)
	fallbackStrings := []string{
		"AbCdEf123456.json",
		"hash123.json",
		"notstartwith~.json",
		"regularfilename",
	}

	for _, encoded := range fallbackStrings {
		t.Run("fallback_"+encoded, func(t *testing.T) {
			result, err := encoder.Decode(encoded)
			assert.Error(t, err)
			assert.Empty(t, result)
			assert.Contains(t, err.Error(), "fallback")
		})
	}
}

func TestSubstitutionHashEscape_EncodeDecode_Roundtrip(t *testing.T) {
	encoder := pathencoding.NewSubstitutionHashEscape()

	testPaths := []string{
		"/usr/bin/python3",
		"/home/user/file.txt",
		"/path/with#hash/chars",
		"/path/with~tilde/chars",
		"/complex/path#with~mixed/special#chars",
		"/",
		"/single",
		"/very/long/path/with/many/components/file.extension",
	}

	for _, path := range testPaths {
		t.Run("roundtrip_"+strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			// Encode
			encoded, err := encoder.Encode(path)
			require.NoError(t, err)
			require.NotEmpty(t, encoded)

			// Decode
			decoded, err := encoder.Decode(encoded)
			require.NoError(t, err)
			assert.Equal(t, path, decoded)
		})
	}
}

func TestSubstitutionHashEscape_EdgeCases(t *testing.T) {
	encoder := pathencoding.NewSubstitutionHashEscape()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "consecutive hash characters",
			input:    "/path/with##double/hash",
			expected: "~path~with#1#1double~hash",
		},
		{
			name:     "consecutive tilde characters",
			input:    "/path/with~~double/tilde",
			expected: "~path~with####double~tilde",
		},
		{
			name:     "mixed consecutive special chars",
			input:    "/path/with#~mixed/chars",
			expected: "~path~with#1##mixed~chars",
		},
		{
			name:     "path ending with special chars",
			input:    "/path/ending#",
			expected: "~path~ending#1",
		},
		{
			name:     "path ending with tilde",
			input:    "/path/ending~",
			expected: "~path~ending##",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test encoding
			encoded, err := encoder.Encode(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, encoded)

			// Test round trip
			decoded, err := encoder.Decode(encoded)
			require.NoError(t, err)
			assert.Equal(t, tt.input, decoded)
		})
	}
}
