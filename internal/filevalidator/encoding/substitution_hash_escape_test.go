package encoding

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testApplicationPath = "/usr/local/bin/application/module/file.txt"

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
				assert.NotEmpty(t, result)
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
			result, err := encoder.EncodeWithFallback(tt.path)

			assert.NoError(t, err)
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

func TestSubstitutionHashEscape_EncodeWithFallback_ErrorCases(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name        string
		input       string
		expectError bool
		errorType   error
	}{
		{
			name:        "empty path should fail",
			input:       "",
			expectError: true,
			errorType:   ErrEmptyPath,
		},
		{
			name:        "relative path should fail",
			input:       "usr/bin/python3",
			expectError: true,
			errorType:   ErrNotAbsoluteOrNormalized,
		},
		{
			name:        "relative path with dot should fail",
			input:       "./local/file",
			expectError: true,
			errorType:   ErrNotAbsoluteOrNormalized,
		},
		{
			name:        "relative path with double dot should fail",
			input:       "../parent/file",
			expectError: true,
			errorType:   ErrNotAbsoluteOrNormalized,
		},
		{
			name:        "path with double slash should fail",
			input:       "/foo//bar",
			expectError: true,
			errorType:   ErrNotAbsoluteOrNormalized,
		},
		{
			name:        "path with . should fail",
			input:       "/foo/./bar",
			expectError: true,
			errorType:   ErrNotAbsoluteOrNormalized,
		},
		{
			name:        "path with .. should fail",
			input:       "/foo/bar/../baz",
			expectError: true,
			errorType:   ErrNotAbsoluteOrNormalized,
		},
		{
			name:        "absolute clean path should succeed",
			input:       "/usr/bin/python3",
			expectError: false,
		},
		{
			name:        "root path should succeed",
			input:       "/",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.EncodeWithFallback(tt.input)

			if tt.expectError {
				assert.Error(t, err)

				// Check that error is wrapped in ErrInvalidPath
				var invalidPathErr ErrInvalidPath
				assert.ErrorAs(t, err, &invalidPathErr)
				assert.Equal(t, tt.input, invalidPathErr.Path)

				// Check the underlying error type
				assert.ErrorIs(t, invalidPathErr.Err, tt.errorType)

				// For empty path, check the result structure
				if tt.input == "" {
					assert.Equal(t, "", result.EncodedName)
					assert.False(t, result.IsFallback)
					assert.Equal(t, 0, result.OriginalLength)
					assert.Equal(t, 0, result.EncodedLength)
				} else {
					// For other error cases, result should be empty
					assert.Equal(t, Result{}, result)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result.EncodedName)
				assert.Equal(t, len(tt.input), result.OriginalLength)
				assert.Equal(t, len(result.EncodedName), result.EncodedLength)

				// For short paths, should use normal encoding
				if len(result.EncodedName) <= MaxFilenameLength {
					assert.False(t, result.IsFallback)
					assert.Equal(t, byte('~'), result.EncodedName[0]) // Should start with ~
				}
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

func TestSubstitutionHashEscape_IsFallbackEncoding(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "fallback encoding doesn't start with tilde",
			input:    "AbCdEf123456.json",
			expected: true,
		},
		{
			name:     "normal encoding starts with tilde",
			input:    "~usr~bin~python3",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "SHA256 hash-like string",
			input:    "a1b2c3d4e5f6789012345678901234567890abcd",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encoder.IsFallbackEncoding(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubstitutionHashEscape_Decode_FallbackErrorCases(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errorType error
	}{
		{
			name:      "fallback encoding should not be reversible",
			input:     "AbCdEf123456.json",
			wantErr:   true,
			errorType: &ErrFallbackNotReversible{},
		},
		{
			name:      "SHA256-like hash should not be reversible",
			input:     "a1b2c3d4e5f6789012345678901234567890abcd",
			wantErr:   true,
			errorType: &ErrFallbackNotReversible{},
		},
		{
			name:      "normal encoding should be reversible",
			input:     "~usr~bin~python3",
			wantErr:   false,
			errorType: nil,
		},
		{
			name:      "empty string should be reversible",
			input:     "",
			wantErr:   false,
			errorType: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.Decode(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				var fallbackErr ErrFallbackNotReversible
				assert.ErrorAs(t, err, &fallbackErr)
				assert.Equal(t, tt.input, fallbackErr.EncodedName)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSubstitutionHashEscape_EdgeCaseCharacters(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple consecutive hash characters",
			input:    "/path/with###hash/file",
			expected: "~path~with#1#1#1hash~file",
		},
		{
			name:     "multiple consecutive tildes",
			input:    "/path/with~~~tilde/file",
			expected: "~path~with######tilde~file",
		},
		{
			name:     "mixed consecutive special characters",
			input:    "/path/~#~#~/file",
			expected: "~path~###1###1##~file",
		},
		{
			name:     "path ending with special characters",
			input:    "/path/ending~#",
			expected: "~path~ending###1",
		},
		{
			name:     "path starting with special characters after root",
			input:    "/~#path/file",
			expected: "~###1path~file",
		},
		{
			name:     "single character segments",
			input:    "/a/b/c/d",
			expected: "~a~b~c~d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test encoding
			encoded, err := encoder.Encode(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, encoded)

			// Test round-trip
			decoded, err := encoder.Decode(encoded)
			require.NoError(t, err)
			assert.Equal(t, tt.input, decoded)
		})
	}
}

func TestSubstitutionHashEscape_InvalidInputFormatting(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name        string
		input       string
		expectError string
	}{
		{
			name:        "path with newline",
			input:       "/path/with\nnewline",
			expectError: "", // Should succeed - newlines are valid in Linux filenames
		},
		{
			name:        "path with tab character",
			input:       "/path/with\ttab",
			expectError: "", // Should succeed - tabs are valid in Linux filenames
		},
		{
			name:        "path with unicode characters",
			input:       "/path/with/ユニコード/file",
			expectError: "", // Should succeed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encoder.Encode(tt.input)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSubstitutionHashEscape_BoundaryValues(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name              string
		input             string
		expectedFallback  bool
		expectedMaxLength int
	}{
		{
			name:              "exactly at MaxFilenameLength boundary normal encoding",
			input:             "/" + strings.Repeat("a", 249), // Encodes to exactly 250 chars (limit)
			expectedFallback:  false,
			expectedMaxLength: 255,
		},
		{
			name:              "just over MaxFilenameLength boundary",
			input:             "/" + strings.Repeat("a", 250), // Encodes to 251 chars, over limit
			expectedFallback:  true,
			expectedMaxLength: 255,
		},
		{
			name:              "very short path",
			input:             "/a",
			expectedFallback:  false,
			expectedMaxLength: 255,
		},
		{
			name:              "medium length path",
			input:             "/" + strings.Repeat("path/", 10) + "file.txt",
			expectedFallback:  false,
			expectedMaxLength: 255,
		},
		{
			name:              "long path with special characters forcing fallback",
			input:             "/" + strings.Repeat("~#", 130) + "/file",
			expectedFallback:  true,
			expectedMaxLength: 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.EncodeWithFallback(tt.input)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedFallback, result.IsFallback)
			assert.LessOrEqual(t, result.EncodedLength, tt.expectedMaxLength)

			if !result.IsFallback {
				// Test round-trip for normal encoding
				decoded, err := encoder.Decode(result.EncodedName)
				require.NoError(t, err)
				assert.Equal(t, tt.input, decoded)
			}
		})
	}
}

func TestSubstitutionHashEscape_FallbackSHA256Generation(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name                  string
		input                 string
		expectedSHA256Pattern string
	}{
		{
			name:                  "long path generates consistent SHA256",
			input:                 "/" + strings.Repeat("very-long-directory-name", 15) + "/file.txt",
			expectedSHA256Pattern: `^[A-Za-z0-9_-]{12}\.json$`, // Base64URL encoding, 12 chars
		},
		{
			name:                  "different long paths generate different hashes",
			input:                 "/" + strings.Repeat("different-long-directory-name", 15) + "/file.txt",
			expectedSHA256Pattern: `^[A-Za-z0-9_-]{12}\.json$`, // Base64URL encoding, 12 chars
		},
	}

	var previousHash string
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.EncodeWithFallback(tt.input)
			require.NoError(t, err)

			assert.True(t, result.IsFallback)
			assert.Regexp(t, tt.expectedSHA256Pattern, result.EncodedName)

			// Ensure different inputs generate different hashes
			if i > 0 {
				assert.NotEqual(t, previousHash, result.EncodedName)
			}
			previousHash = result.EncodedName
		})
	}
}

func TestSubstitutionHashEscape_PathLengthEdgeCases(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	// Test various path lengths around the boundary
	for length := 250; length <= 255; length++ {
		t.Run(fmt.Sprintf("path_length_%d", length), func(t *testing.T) {
			// Create a path that would encode to approximately the target length
			basePath := "/" + strings.Repeat("a", length-1)

			result, err := encoder.EncodeWithFallback(basePath)
			require.NoError(t, err)

			// Verify that encoded name is within filesystem limits
			assert.LessOrEqual(t, result.EncodedLength, MaxFilenameLength)

			if !result.IsFallback {
				// If normal encoding, verify round-trip
				decoded, err := encoder.Decode(result.EncodedName)
				require.NoError(t, err)
				assert.Equal(t, basePath, decoded)
			}
		})
	}
}

func TestSubstitutionHashEscape_SpecialCharacterDensity(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name              string
		input             string
		expectedExpansion int // How much the encoded version should expand
		expectedFallback  bool
	}{
		{
			name:              "no special characters",
			input:             "/normal/path/file",
			expectedExpansion: 0, // Same length (slashes become tildes)
			expectedFallback:  false,
		},
		{
			name:              "all tildes",
			input:             "/" + strings.Repeat("~", 50),
			expectedExpansion: 50, // Each ~ becomes ##, doubling the length
			expectedFallback:  false,
		},
		{
			name:              "all hashes",
			input:             "/" + strings.Repeat("#", 50),
			expectedExpansion: 50, // Each # becomes #1, doubling the length
			expectedFallback:  false,
		},
		{
			name:              "high density special characters forcing fallback",
			input:             "/" + strings.Repeat("~#", 100), // 200 chars -> 400+ chars encoded
			expectedExpansion: 0,                               // Fallback doesn't expand
			expectedFallback:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encoder.EncodeWithFallback(tt.input)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedFallback, result.IsFallback)

			if !result.IsFallback && tt.expectedExpansion > 0 {
				// Check that expansion is approximately correct
				actualExpansion := result.EncodedLength - result.OriginalLength
				assert.GreaterOrEqual(t, actualExpansion, tt.expectedExpansion)
			}
		})
	}
}

func TestSubstitutionHashEscape_ExtensiveRoundTripTests(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	// Extended property-based testing with many more test cases
	testPaths := []string{
		// Basic paths
		"/usr/bin/python3",
		"/home/user_name/project_files",
		"/",
		"/single",

		// Paths with special characters
		"/path/with#special/chars~here",
		"/path/with###multiple#hashes",
		"/path/with~~~multiple~tildes",
		"/path/with~#~#~#mixed",

		// Complex nesting
		"/very/deep/nested/directory/structure/with/many/levels/file.txt",
		"/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z",

		// Edge cases
		"/path/ending/with#",
		"/path/ending/with~",
		"/#starting/with/hash",
		"/~starting/with/tilde",

		// Real-world patterns
		"/usr/local/bin/node_modules/.bin/webpack",
		"/home/user/.config/applications/custom.desktop",
		"/opt/software/lib/x86_64-linux-gnu/libexample.so.1.0.0",
		"/var/log/application/app.log.2023-12-01",

		// Unicode and international characters
		"/path/with/ユニコード/file",
		"/path/with/émojis/🚀/file",
		"/путь/с/русскими/символами",
		"/路径/与/中文/字符",

		// Boundary cases
		"/a",
		"/ab",
		"/abc",
		"/abcd",
		"/" + strings.Repeat("a", 100),
		"/" + strings.Repeat("x", 200),
	}

	for _, originalPath := range testPaths {
		t.Run(fmt.Sprintf("round_trip_%s", originalPath), func(t *testing.T) {
			// Test direct encode/decode
			encoded, err := encoder.Encode(originalPath)
			require.NoError(t, err, "Failed to encode path: %s", originalPath)

			decoded, err := encoder.Decode(encoded)
			require.NoError(t, err, "Failed to decode path: %s -> %s", originalPath, encoded)
			assert.Equal(t, originalPath, decoded, "Round-trip failed for path: %s", originalPath)

			// Test EncodeWithFallback
			result, err := encoder.EncodeWithFallback(originalPath)
			require.NoError(t, err, "Failed to encode with fallback: %s", originalPath)

			if !result.IsFallback {
				// Only test round-trip for normal encoding (fallback is not reversible)
				decodedFallback, err := encoder.Decode(result.EncodedName)
				require.NoError(t, err, "Failed to decode fallback result: %s -> %s", originalPath, result.EncodedName)
				assert.Equal(t, originalPath, decodedFallback, "Round-trip failed for fallback path: %s", originalPath)
			}

			// Verify encoding properties
			assert.Equal(t, len(originalPath), result.OriginalLength)
			assert.Equal(t, len(result.EncodedName), result.EncodedLength)
			assert.LessOrEqual(t, result.EncodedLength, MaxFilenameLength)
		})
	}
}

func TestSubstitutionHashEscape_ConsistencyTests(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	testPaths := []string{
		"/usr/bin/python3",
		"/path/with#hash/file",
		"/path/with~tilde/file",
		"/" + strings.Repeat("long-directory-name", 20),
	}

	// Test that encoding is deterministic (same input always produces same output)
	for _, path := range testPaths {
		t.Run(fmt.Sprintf("consistency_%s", path), func(t *testing.T) {
			results := make([]Result, 5)

			// Encode the same path multiple times
			for i := range 5 {
				result, err := encoder.EncodeWithFallback(path)
				require.NoError(t, err)
				results[i] = result
			}

			// Verify all results are identical
			for i := 1; i < len(results); i++ {
				assert.Equal(t, results[0], results[i], "Encoding not consistent for path: %s", path)
			}
		})
	}
}

func TestSubstitutionHashEscape_StressTest(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	// Create a seeded random source for reproducible tests
	source := rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()))
	rng := rand.New(source)

	// Generate many paths with random characteristics
	testCases := []struct {
		name      string
		generator func() string
		count     int
	}{
		{
			name: "random_normal_paths",
			generator: func() string {
				depth := 3 + rng.IntN(5) // 3-7 levels deep
				path := ""
				for range depth {
					path += "/dir" + fmt.Sprintf("%d", rng.IntN(100))
				}
				return path + "/file.txt"
			},
			count: 50,
		},
		{
			name: "random_special_character_paths",
			generator: func() string {
				base := "/path/with"
				length := 10 + rng.IntN(20) // 10-29 characters
				specialChars := []string{"#", "~", "@", "&", "%", "!", "*"}
				for range length {
					if rng.Float32() < 0.3 { // 30% chance of special character
						base += specialChars[rng.IntN(len(specialChars))]
					} else {
						base += string(rune('a' + rng.IntN(26))) // random lowercase letter
					}
				}
				return base + "/file"
			},
			count: 30,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for range tc.count {
				path := tc.generator()

				// Ensure valid absolute path
				if !strings.HasPrefix(path, "/") {
					path = "/" + path
				}

				result, err := encoder.EncodeWithFallback(path)
				require.NoError(t, err, "Failed to encode path: %s", path)

				// Verify basic properties
				assert.LessOrEqual(t, result.EncodedLength, MaxFilenameLength)
				assert.Equal(t, len(path), result.OriginalLength)
				assert.Equal(t, len(result.EncodedName), result.EncodedLength)

				// Test round-trip if not fallback
				if !result.IsFallback {
					decoded, err := encoder.Decode(result.EncodedName)
					require.NoError(t, err, "Failed to decode: %s -> %s", path, result.EncodedName)
					assert.Equal(t, path, decoded, "Round-trip failed: %s", path)
				}
			}
		})
	}
}

// Benchmark tests for performance verification

func BenchmarkSubstitutionHashEscape_Encode(b *testing.B) {
	encoder := NewSubstitutionHashEscape()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := encoder.Encode(testApplicationPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSubstitutionHashEscape_Decode(b *testing.B) {
	encoder := NewSubstitutionHashEscape()
	encoded, err := encoder.Encode(testApplicationPath)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := encoder.Decode(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSubstitutionHashEscape_EncodeWithFallback(b *testing.B) {
	encoder := NewSubstitutionHashEscape()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := encoder.EncodeWithFallback(testApplicationPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSubstitutionHashEscape_EncodeWithSpecialChars(b *testing.B) {
	encoder := NewSubstitutionHashEscape()
	testPath := "/path/with#many~special#chars/and~more#special~chars/file"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := encoder.Encode(testPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSubstitutionHashEscape_EncodeLongPathFallback(b *testing.B) {
	encoder := NewSubstitutionHashEscape()
	// Create a path that will trigger fallback
	testPath := "/" + strings.Repeat("very-long-directory-name", 15) + "/file.txt"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := encoder.EncodeWithFallback(testPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSubstitutionHashEscape_EncodeVariousLengths(b *testing.B) {
	encoder := NewSubstitutionHashEscape()

	testCases := []struct {
		name string
		path string
	}{
		{"short_path", "/usr/bin/python3"},
		{"medium_path", "/usr/local/share/applications/software/config/file.txt"},
		{"long_path", "/" + strings.Repeat("directory/", 20) + "file.txt"},
		{"very_long_path", "/" + strings.Repeat("very-long-directory-name/", 10) + "file.txt"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := encoder.EncodeWithFallback(tc.path)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSubstitutionHashEscape_MemoryEfficiency(b *testing.B) {
	encoder := NewSubstitutionHashEscape()

	// Test memory efficiency with 1000 paths (target: <1MB per 1000 paths)
	paths := make([]string, 1000)
	for i := range 1000 {
		paths[i] = fmt.Sprintf("/path/to/file%d/subdir/file.txt", i)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		results := make([]Result, len(paths))
		for j, path := range paths {
			result, err := encoder.EncodeWithFallback(path)
			if err != nil {
				b.Fatal(err)
			}
			results[j] = result
		}
		// Prevent compiler optimization
		_ = results
	}
}

func BenchmarkSubstitutionHashEscape_ThroughputTest(b *testing.B) {
	encoder := NewSubstitutionHashEscape()

	// Test throughput against target: 10,000 paths/sec
	testPaths := []string{
		"/usr/bin/python3",
		"/home/user/documents/project/file.txt",
		"/path/with#special/chars~here",
		"/opt/application/lib/module.so",
		"/var/log/application/app.log",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, path := range testPaths {
			_, err := encoder.EncodeWithFallback(path)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// Performance measurement and regression detection helpers

func TestSubstitutionHashEscape_MemoryUsageMeasurement(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	tests := []struct {
		name      string
		pathCount int
		pathGen   func(int) string
	}{
		{
			name:      "1000_normal_paths",
			pathCount: 1000,
			pathGen:   func(i int) string { return fmt.Sprintf("/path/to/file%d/subdir/file.txt", i) },
		},
		{
			name:      "100_special_char_paths",
			pathCount: 100,
			pathGen:   func(i int) string { return fmt.Sprintf("/path/with#special~chars%d/file", i) },
		},
		{
			name:      "50_long_paths",
			pathCount: 50,
			pathGen: func(i int) string {
				return fmt.Sprintf("/%s%d/file.txt", strings.Repeat("very-long-directory-name/", 10), i)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			results := make([]Result, tt.pathCount)
			for i := range tt.pathCount {
				path := tt.pathGen(i)
				result, err := encoder.EncodeWithFallback(path)
				require.NoError(t, err)
				results[i] = result
			}

			runtime.ReadMemStats(&m2)
			allocatedBytes := m2.TotalAlloc - m1.TotalAlloc

			// Memory efficiency targets from architecture document
			bytesPerPath := float64(allocatedBytes) / float64(tt.pathCount)
			totalMBForThousand := (bytesPerPath * 1000) / (1024 * 1024)

			t.Logf("Test: %s", tt.name)
			t.Logf("  Paths processed: %d", tt.pathCount)
			t.Logf("  Total memory allocated: %d bytes", allocatedBytes)
			t.Logf("  Memory per path: %.2f bytes", bytesPerPath)
			t.Logf("  Projected memory for 1000 paths: %.2f MB", totalMBForThousand)

			// Architecture target: <1MB per 1000 paths
			if tt.name == "1000_normal_paths" {
				assert.Less(t, totalMBForThousand, 1.0, "Memory usage exceeds target of 1MB per 1000 paths")
			}

			// Additional assertions for other test cases to guard against regressions
			if tt.name == "100_special_char_paths" {
				// Special character paths may use more memory due to encoding expansion
				// Allow up to 2MB per 1000 paths for special character heavy scenarios
				assert.Less(t, totalMBForThousand, 2.0, "Memory usage for special char paths exceeds 2MB per 1000 paths")
			}

			if tt.name == "50_long_paths" {
				// Long paths may trigger fallback encoding which uses SHA256 computation
				// Allow up to 3MB per 1000 paths for long path scenarios that may use fallback
				assert.Less(t, totalMBForThousand, 3.0, "Memory usage for long paths exceeds 3MB per 1000 paths")
			}

			// Prevent compiler optimization
			_ = results
		})
	}
}

func TestSubstitutionHashEscape_ThroughputMeasurement(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	testPaths := []string{
		"/usr/bin/python3",
		"/home/user/documents/project/file.txt",
		"/path/with#special/chars~here",
		"/opt/application/lib/module.so",
		"/var/log/application/app.log",
		"/very/deep/nested/directory/structure/with/many/levels/file.txt",
		"/path/with###multiple#hash#characters/file",
		"/path/with~~~multiple~tilde~characters/file",
	}

	// Measure throughput using fixed duration for more stable results across different environments
	testDuration := 100 * time.Millisecond // Run test for 100ms
	start := time.Now()
	iterations := 0

	for time.Since(start) < testDuration {
		path := testPaths[iterations%len(testPaths)]
		_, err := encoder.EncodeWithFallback(path)
		require.NoError(t, err)
		iterations++
	}

	actualDuration := time.Since(start)
	pathsPerSecond := float64(iterations) / actualDuration.Seconds()

	t.Logf("Throughput measurement:")
	t.Logf("  Operations: %d", iterations)
	t.Logf("  Duration: %v", actualDuration)
	t.Logf("  Paths per second: %.0f", pathsPerSecond)

	// Architecture target: 10,000 paths/sec
	assert.GreaterOrEqual(t, pathsPerSecond, 10000.0,
		"Throughput below target of 10,000 paths/sec: got %.0f", pathsPerSecond)
}

func TestSubstitutionHashEscape_PerformanceRegression(t *testing.T) {
	encoder := NewSubstitutionHashEscape()

	// Baseline performance expectations (adjusted based on actual measurements)
	benchmarks := []struct {
		name                string
		operation           func() error
		maxNsPerOp          int64 // Maximum nanoseconds per operation
		maxAllocsBytesPerOp int64 // Maximum allocated bytes per operation
	}{
		{
			name: "encode_simple_path",
			operation: func() error {
				_, err := encoder.Encode("/usr/bin/python3")
				return err
			},
			maxNsPerOp:          5000, // 5μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 200,  // 200 bytes
		},
		{
			name: "encode_with_fallback_normal",
			operation: func() error {
				_, err := encoder.EncodeWithFallback(testApplicationPath)
				return err
			},
			maxNsPerOp:          8000, // 8μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 400,  // 400 bytes
		},
		{
			name: "encode_special_chars",
			operation: func() error {
				_, err := encoder.Encode("/path/with#many~special#chars/file")
				return err
			},
			maxNsPerOp:          6000, // 6μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 300,  // 300 bytes
		},
		{
			name: "decode_normal",
			operation: func() error {
				_, err := encoder.Decode("~usr~bin~python3")
				return err
			},
			maxNsPerOp:          4000, // 4μs (adjusted for CI environment)
			maxAllocsBytesPerOp: 150,  // 150 bytes
		},
	}

	for _, bench := range benchmarks {
		t.Run(bench.name, func(t *testing.T) {
			// Warmup
			for range 100 {
				_ = bench.operation()
			}

			// Measure performance
			iterations := 1000
			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			start := time.Now()
			for range iterations {
				err := bench.operation()
				require.NoError(t, err)
			}
			duration := time.Since(start)

			runtime.ReadMemStats(&m2)
			allocatedBytes := m2.TotalAlloc - m1.TotalAlloc

			nsPerOp := duration.Nanoseconds() / int64(iterations)
			bytesPerOp := int64(allocatedBytes) / int64(iterations)

			t.Logf("Performance regression test: %s", bench.name)
			t.Logf("  Nanoseconds per operation: %d (max: %d)", nsPerOp, bench.maxNsPerOp)
			t.Logf("  Bytes allocated per operation: %d (max: %d)", bytesPerOp, bench.maxAllocsBytesPerOp)

			// Check against regression thresholds
			if nsPerOp > bench.maxNsPerOp {
				t.Errorf("Performance regression detected: %d ns/op > %d ns/op", nsPerOp, bench.maxNsPerOp)
			}

			if bytesPerOp > bench.maxAllocsBytesPerOp {
				t.Errorf("Memory regression detected: %d bytes/op > %d bytes/op", bytesPerOp, bench.maxAllocsBytesPerOp)
			}
		})
	}
}
