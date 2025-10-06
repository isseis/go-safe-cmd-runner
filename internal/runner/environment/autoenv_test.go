package environment

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// datetimePattern matches the datetime format YYYYMMDDHHMMSS.msec
var datetimePattern = regexp.MustCompile(`^\d{14}\.\d{3}$`)

func TestAutoEnvProvider_Generate(t *testing.T) {
	t.Run("contains all required keys", func(t *testing.T) {
		provider := NewAutoEnvProvider(nil)
		result := provider.Generate()

		assert.Contains(t, result, AutoEnvPrefix+AutoEnvKeyDatetime)
		assert.Contains(t, result, AutoEnvPrefix+AutoEnvKeyPID)
	})

	t.Run("DATETIME has correct format", func(t *testing.T) {
		provider := NewAutoEnvProvider(nil)
		result := provider.Generate()

		datetime := result[AutoEnvPrefix+AutoEnvKeyDatetime]
		// Format: YYYYMMDDHHMMSS.msec (e.g., "20251005143025.123")
		assert.True(t, datetimePattern.MatchString(datetime), "DATETIME should match pattern YYYYMMDDHHMMSS.msec, got: %s", datetime)
	})

	t.Run("PID is valid", func(t *testing.T) {
		provider := NewAutoEnvProvider(nil)
		result := provider.Generate()

		pid := result[AutoEnvPrefix+AutoEnvKeyPID]
		expectedPID := strconv.Itoa(os.Getpid())
		assert.Equal(t, expectedPID, pid)
	})
}

func TestAutoEnvProvider_WithFixedClock(t *testing.T) {
	tests := []struct {
		name         string
		fixedTime    time.Time
		expectedDate string
	}{
		{
			name:         "normal time with milliseconds",
			fixedTime:    time.Date(2025, 10, 5, 14, 30, 22, 123000000, time.UTC),
			expectedDate: "20251005143022.123",
		},
		{
			name:         "zero milliseconds",
			fixedTime:    time.Date(2025, 10, 5, 14, 30, 22, 0, time.UTC),
			expectedDate: "20251005143022.000",
		},
		{
			name:         "year end boundary",
			fixedTime:    time.Date(2025, 12, 31, 23, 59, 59, 999000000, time.UTC),
			expectedDate: "20251231235959.999",
		},
		{
			name:         "year start boundary",
			fixedTime:    time.Date(2025, 1, 1, 0, 0, 0, 1000000, time.UTC),
			expectedDate: "20250101000000.001",
		},
		{
			name:         "nanosecond precision truncation",
			fixedTime:    time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC),
			expectedDate: "20251005143022.123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := func() time.Time { return tt.fixedTime }
			provider := NewAutoEnvProvider(clock)

			result := provider.Generate()
			datetime := result[AutoEnvPrefix+AutoEnvKeyDatetime]
			assert.Equal(t, tt.expectedDate, datetime)
		})
	}
}

func TestAutoEnvProvider_UTCTimezone(t *testing.T) {
	// Use non-UTC time to ensure conversion happens
	jst := time.FixedZone("JST", 9*60*60) // UTC+9
	fixedTime := time.Date(2025, 10, 5, 23, 30, 22, 123000000, jst)

	clock := func() time.Time { return fixedTime }
	provider := NewAutoEnvProvider(clock)

	result := provider.Generate()
	datetime := result[AutoEnvPrefix+AutoEnvKeyDatetime]

	// JST 23:30 = UTC 14:30
	expected := "20251005143022.123"
	assert.Equal(t, expected, datetime, "should convert to UTC")
}

func TestNewAutoEnvProvider_NilClock(t *testing.T) {
	provider := NewAutoEnvProvider(nil)
	result := provider.Generate()

	// Should use time.Now() as default
	assert.NotEmpty(t, result[AutoEnvPrefix+AutoEnvKeyDatetime])
	assert.NotEmpty(t, result[AutoEnvPrefix+AutoEnvKeyPID])
}
