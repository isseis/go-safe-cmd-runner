package logging

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRunID_AcceptsAllowedCharacters(t *testing.T) {
	validValues := []string{
		"my-custom-run-001",
		"gh-12345678",
		"A_b-9",
	}

	for _, value := range validValues {
		t.Run(value, func(t *testing.T) {
			assert.NoError(t, ValidateRunID(value))
		})
	}
}

func TestValidateRunID_LengthBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		accept bool
	}{
		{
			name:   "minimum length",
			value:  strings.Repeat("a", 1),
			accept: true,
		},
		{
			name:   "maximum length",
			value:  strings.Repeat("a", MaxRunIDLength),
			accept: true,
		},
		{
			name:   "over maximum length",
			value:  strings.Repeat("a", MaxRunIDLength+1),
			accept: false,
		},
		{
			name:   "empty",
			value:  "",
			accept: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRunID(tt.value)
			if tt.accept {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, ErrInvalidRunID))
			}
		})
	}
}

func TestValidateRunID_RejectsNonAllowlistedValues(t *testing.T) {
	rejectedValues := []string{
		"../../etc/cron.d/evil",
		"/tmp/evil",
		"..",
		"a.b",
		"a b",
		"line1\nline2",
		"a\x00b",
		"a\x1bb",
		"ラン",
		"a%b",
	}

	for _, value := range rejectedValues {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			err := ValidateRunID(value)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidRunID))
		})
	}
}

func TestValidateRunID_ErrorOmitsRejectedValue(t *testing.T) {
	rejectedValues := []string{
		"../../etc/cron.d/evil",
		"/tmp/evil",
		"..",
		"a.b",
		"a b",
		"line1\nline2",
		"a\x00b",
		"a\x1bb",
		"ラン",
		"a%b",
	}

	for _, value := range rejectedValues {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			err := ValidateRunID(value)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), value)

			// The whole-value assertion above would still pass if the error
			// echoed a prefix such as "../..", so pin the contract itself:
			// everything the error reproduces from the rejected value is the
			// single %q-rendered byte after the constant template.
			_, rendered, found := strings.Cut(err.Error(), " has value ")
			require.True(t, found, "unexpected error shape: %q", err.Error())
			unquoted, unquoteErr := strconv.Unquote(rendered)
			require.NoError(t, unquoteErr, "rendered byte is not %%q-quoted: %q", rendered)
			assert.Len(t, unquoted, 1, "error reproduces more than one byte of the rejected value")
		})
	}
}

func TestValidateRunID_ErrorIdentifiesFirstViolatingByte(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantIndex string
		wantByte  string
	}{
		{
			name:      "path separator",
			value:     "a/b",
			wantIndex: "index 1",
			wantByte:  `"/"`,
		},
		{
			name:      "dot",
			value:     "a.b",
			wantIndex: "index 1",
			wantByte:  `"."`,
		},
		{
			name:      "NUL byte",
			value:     "a\x00b",
			wantIndex: "index 1",
			wantByte:  `"\x00"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRunID(tt.value)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantIndex)
			assert.Contains(t, err.Error(), tt.wantByte)
		})
	}
}

func TestRunIDFormatDescription_ReflectsMaxRunIDLength(t *testing.T) {
	expected := "1-" + strconv.Itoa(MaxRunIDLength) + " characters, each of A-Z a-z 0-9 '_' '-'"
	assert.Equal(t, expected, RunIDFormatDescription())
}

func TestErrorTypeInvalidRunID_Token(t *testing.T) {
	// The token is the programmatically-discriminable marker for run ID
	// rejection (architecture §4.1); pin it so a typo fails here.
	assert.Equal(t, ErrorType("invalid_run_id"), ErrorTypeInvalidRunID)
}

func TestGenerateRunID_Uniqueness(t *testing.T) {
	// Generate multiple IDs and verify they are unique
	ids := make(map[string]bool)
	iterations := 100

	for range iterations {
		id := GenerateRunID()

		assert.NotEmpty(t, id, "GenerateRunID() returned empty string")
		assert.False(t, ids[id], "GenerateRunID() generated duplicate ID: %s", id)

		ids[id] = true
	}

	assert.Equal(t, iterations, len(ids))
}

func TestGenerateRunID_Format(t *testing.T) {
	id := GenerateRunID()

	// ULID should be 26 characters
	assert.Equal(t, 26, len(id))

	// ULID should only contain specific characters (Crockford's Base32)
	validChars := "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for _, c := range id {
		assert.True(t, strings.ContainsRune(validChars, c), "GenerateRunID() returned ID with invalid character: %c", c)
	}
}

func TestGenerateRunID_SatisfiesValidateRunID(t *testing.T) {
	for range 100 {
		id := GenerateRunID()
		assert.NoError(t, ValidateRunID(id))
	}
}
