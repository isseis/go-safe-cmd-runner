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

func TestSubstitutionHashEscape_Encode_RelativePathError(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

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
			name:        "empty path should fail",
			input:       "",
			expectError: true,
		},
		{
			name:        "root path should succeed",
			input:       "/",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.Encode(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
				var relativePathErr ErrInvalidPath
				assert.ErrorAs(t, err, &relativePathErr)
				assert.Equal(t, tt.input, relativePathErr.Path)
			} else {
				assert.NoError(t, err)
				// For empty input, result should also be empty
				if tt.input == "" {
					assert.Empty(t, result)
				} else {
					assert.NotEmpty(t, result)
				}
			}
		})
	}
}

func TestSubstitutionHashEscape_Encode_PathNormalizationError(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name          string
		input         string
		expectError   bool
		expectedClean string
	}{
		{
			name:        "clean absolute path should succeed",
			input:       "/usr/bin/python3",
			expectError: false,
		},
		{
			name:          "path with .. should fail",
			input:         "/foo/bar/../baz",
			expectError:   true,
			expectedClean: "/foo/baz",
		},
		{
			name:          "path with double slash should fail",
			input:         "/foo//bar",
			expectError:   true,
			expectedClean: "/foo/bar",
		},
		{
			name:          "path with . should fail",
			input:         "/foo/./bar",
			expectError:   true,
			expectedClean: "/foo/bar",
		},
		{
			name:          "path with multiple .. should fail",
			input:         "/foo/bar/../../baz",
			expectError:   true,
			expectedClean: "/baz",
		},
		{
			name:          "path resolving to root should fail",
			input:         "/foo/../..",
			expectError:   true,
			expectedClean: "/",
		},
		{
			name:        "root path should succeed",
			input:       "/",
			expectError: false,
		},
		{
			name:        "clean nested path should succeed",
			input:       "/foo/bar/baz",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.Encode(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
				var pathNotCleanErr ErrInvalidPath
				assert.ErrorAs(t, err, &pathNotCleanErr)
				assert.Equal(t, tt.input, pathNotCleanErr.Path)
			} else {
				assert.NoError(t, err)
				// For root path, result should be "~"
				if tt.input == "/" {
					assert.Equal(t, "~", result)
				} else {
					assert.NotEmpty(t, result)
				}
			}
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
			encoded, err := encoder.Encode(originalPath)
			require.NoError(t, err)

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
