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
		})
	}
}

func TestRunIDFormatDescription_ReflectsMaxRunIDLength(t *testing.T) {
	assert.True(t, strings.Contains(RunIDFormatDescription, strconv.Itoa(MaxRunIDLength)))
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
