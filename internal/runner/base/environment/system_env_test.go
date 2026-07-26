package environment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSystemEnvironment(t *testing.T) {
	t.Setenv("TEST_VAR1", "value1")
	t.Setenv("TEST_VAR2", "value2")
	t.Setenv("EMPTY_VAR", "")

	result := ParseSystemEnvironment()

	assert.Contains(t, result, "TEST_VAR1")
	assert.Equal(t, "value1", result["TEST_VAR1"])
	assert.Contains(t, result, "TEST_VAR2")
	assert.Equal(t, "value2", result["TEST_VAR2"])
	assert.Contains(t, result, "EMPTY_VAR")
	assert.Equal(t, "", result["EMPTY_VAR"])
}

func TestParseSystemEnvironment_EmptyEnvironment(t *testing.T) {
	result := ParseSystemEnvironment()

	assert.NotNil(t, result)
}

func TestParseSystemEnvironment_SpecialCharactersInValue(t *testing.T) {
	t.Setenv("VAR_WITH_NEWLINE", "line1\nline2")
	t.Setenv("VAR_WITH_TAB", "value\twith\ttabs")
	t.Setenv("VAR_WITH_QUOTE", "value\"with'quotes")

	result := ParseSystemEnvironment()

	assert.Equal(t, "line1\nline2", result["VAR_WITH_NEWLINE"])
	assert.Equal(t, "value\twith\ttabs", result["VAR_WITH_TAB"])
	assert.Equal(t, "value\"with'quotes", result["VAR_WITH_QUOTE"])
}

func TestParseSystemEnvironment_EmptyValue(t *testing.T) {
	t.Setenv("EMPTY_VAR", "")

	result := ParseSystemEnvironment()

	assert.Contains(t, result, "EMPTY_VAR")
	assert.Equal(t, "", result["EMPTY_VAR"])
}

func TestParseSystemEnvironment_NoEmptyKey(t *testing.T) {
	t.Setenv("VALID_VAR", "value")

	result := ParseSystemEnvironment()

	assert.NotContains(t, result, "")
	assert.Contains(t, result, "VALID_VAR")
}
