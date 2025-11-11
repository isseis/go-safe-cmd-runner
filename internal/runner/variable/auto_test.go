//go:build test

package variable

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDatetimeKey(t *testing.T) {
	assert.Equal(t, "__runner_datetime", DatetimeKey())
}

func TestPIDKey(t *testing.T) {
	assert.Equal(t, "__runner_pid", PIDKey())
}

func TestWorkDirKey(t *testing.T) {
	assert.Equal(t, "__runner_workdir", WorkDirKey())
}

func TestGenerateAutoVars(t *testing.T) {
	// Test with fixed time for reproducibility
	fixedTime := time.Date(2025, 10, 5, 14, 30, 25, 123000000, time.UTC)
	clock := func() time.Time {
		return fixedTime
	}

	autoVars := GenerateAutoVars(clock)

	// Check that both variables are present
	assert.Contains(t, autoVars, DatetimeKey())
	assert.Contains(t, autoVars, PIDKey())

	// Check datetime format: YYYYMMDDHHmmSS.msec
	datetime := autoVars[DatetimeKey()]
	assert.Equal(t, "20251005143025.123", datetime)

	// Check PID format: should be numeric
	pid := autoVars[PIDKey()]
	assert.Equal(t, fmt.Sprintf("%d", os.Getpid()), pid)
	matched, err := regexp.MatchString(`^\d+$`, pid)
	assert.NoError(t, err)
	assert.True(t, matched, "PID should be numeric")
}

func TestGenerateAutoVars_NilClock(t *testing.T) {
	// Test with nil clock (should use time.Now)
	autoVars := GenerateAutoVars(nil)

	// Check that both variables are present
	assert.Contains(t, autoVars, DatetimeKey())
	assert.Contains(t, autoVars, PIDKey())

	// Check datetime format
	datetime := autoVars[DatetimeKey()]
	// Format should match: YYYYMMDDHHmmSS.msec
	matched, err := regexp.MatchString(`^\d{14}\.\d{3}$`, datetime)
	assert.NoError(t, err)
	assert.True(t, matched, "Datetime should match format YYYYMMDDHHmmSS.msec")

	// Check PID
	pid := autoVars[PIDKey()]
	assert.Equal(t, fmt.Sprintf("%d", os.Getpid()), pid)
}

func TestGenerateAutoVars_DatetimeFormat(t *testing.T) {
	// Test various times to ensure format is consistent
	testCases := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{
			name:     "midnight",
			time:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: "20250101000000.000",
		},
		{
			name:     "with milliseconds",
			time:     time.Date(2025, 12, 31, 23, 59, 59, 999000000, time.UTC),
			expected: "20251231235959.999",
		},
		{
			name:     "single digit month and day",
			time:     time.Date(2025, 3, 5, 9, 8, 7, 123000000, time.UTC),
			expected: "20250305090807.123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clock := func() time.Time {
				return tc.time
			}
			autoVars := GenerateAutoVars(clock)
			assert.Equal(t, tc.expected, autoVars[DatetimeKey()])
		})
	}
}

func TestGenerateAutoVars_Consistency(t *testing.T) {
	// Test that calling GenerateAutoVars multiple times with the same clock
	// produces consistent results
	fixedTime := time.Date(2025, 10, 5, 14, 30, 25, 123000000, time.UTC)
	clock := func() time.Time {
		return fixedTime
	}

	autoVars1 := GenerateAutoVars(clock)
	autoVars2 := GenerateAutoVars(clock)

	assert.Equal(t, autoVars1[DatetimeKey()], autoVars2[DatetimeKey()])
	assert.Equal(t, autoVars1[PIDKey()], autoVars2[PIDKey()])
}
