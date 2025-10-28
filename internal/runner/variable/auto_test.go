// package variable provides automatic variable generation for TOML configuration files.
package variable

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// datetimePattern matches the datetime format YYYYMMDDHHmmSS.msec
var datetimePattern = regexp.MustCompile(`^\d{14}\.\d{3}$`)

func TestAutoEnvProvider_Generate(t *testing.T) {
	t.Run("contains all required keys - lowercase format", func(t *testing.T) {
		provider := NewAutoVarProvider()
		result := provider.Generate()

		assert.Contains(t, result, AutoVarPrefix+AutoVarKeyDatetime)
		assert.Contains(t, result, AutoVarPrefix+AutoVarKeyPID)
	})

	t.Run("datetime has correct format - lowercase", func(t *testing.T) {
		provider := NewAutoVarProvider()
		result := provider.Generate()

		datetime := result[AutoVarPrefix+AutoVarKeyDatetime]
		// Format: YYYYMMDDHHmmSS.msec (e.g., "20251005143025.123")
		assert.True(t, datetimePattern.MatchString(datetime), "datetime should match pattern YYYYMMDDHHmmSS.msec, got: %s", datetime)
	})

	t.Run("pid is valid - lowercase", func(t *testing.T) {
		provider := NewAutoVarProvider()
		result := provider.Generate()

		pid := result[AutoVarPrefix+AutoVarKeyPID]
		expectedPID := strconv.Itoa(os.Getpid())
		assert.Equal(t, expectedPID, pid)
	})
}

func TestAutoEnvProvider_WithFixedClock(t *testing.T) {
	tests := []struct {
		name             string
		fixedTime        time.Time
		expectedDatetime string // both uppercase and lowercase use the same format
	}{
		{
			name:             "normal time with milliseconds",
			fixedTime:        time.Date(2025, 10, 5, 14, 30, 22, 123000000, time.UTC),
			expectedDatetime: "20251005143022.123",
		},
		{
			name:             "zero milliseconds",
			fixedTime:        time.Date(2025, 10, 5, 14, 30, 22, 0, time.UTC),
			expectedDatetime: "20251005143022.000",
		},
		{
			name:             "year end boundary",
			fixedTime:        time.Date(2025, 12, 31, 23, 59, 59, 999000000, time.UTC),
			expectedDatetime: "20251231235959.999",
		},
		{
			name:             "year start boundary",
			fixedTime:        time.Date(2025, 1, 1, 0, 0, 0, 1000000, time.UTC),
			expectedDatetime: "20250101000000.001",
		},
		{
			name:             "nanosecond precision truncation",
			fixedTime:        time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC),
			expectedDatetime: "20251005143022.123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" - lowercase", func(t *testing.T) {
			clock := func() time.Time { return tt.fixedTime }
			provider := NewAutoVarProviderWithClock(clock)

			result := provider.Generate()
			varDatetime := result[AutoVarPrefix+AutoVarKeyDatetime]
			assert.Equal(t, tt.expectedDatetime, varDatetime)
		})
	}
}

func TestAutoEnvProvider_UTCTimezone(t *testing.T) {
	// Use non-UTC time to ensure conversion happens
	jst := time.FixedZone("JST", 9*60*60) // UTC+9
	fixedTime := time.Date(2025, 10, 5, 23, 30, 22, 123000000, jst)

	clock := func() time.Time { return fixedTime }
	provider := NewAutoVarProviderWithClock(clock)

	result := provider.Generate()

	t.Run("lowercase format converts to UTC", func(t *testing.T) {
		varDatetime := result[AutoVarPrefix+AutoVarKeyDatetime]
		// JST 23:30 = UTC 14:30
		expected := "20251005143022.123"
		assert.Equal(t, expected, varDatetime, "should convert to UTC")
	})
}

func TestNewAutoEnvProvider_NilClock(t *testing.T) {
	provider := NewAutoVarProvider()
	result := provider.Generate()

	// Should use time.Now() as default
	assert.NotEmpty(t, result[AutoVarPrefix+AutoVarKeyDatetime])
	assert.NotEmpty(t, result[AutoVarPrefix+AutoVarKeyPID])
}
