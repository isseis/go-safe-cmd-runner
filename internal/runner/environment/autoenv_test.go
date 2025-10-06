package environment

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		// Format: YYYYMMDDHHMM.msec (e.g., "202510051430.123")
		pattern := `^\d{12}\.\d{3}$`
		matched, err := regexp.MatchString(pattern, datetime)
		require.NoError(t, err)
		assert.True(t, matched, "DATETIME should match pattern YYYYMMDDHHMM.msec, got: %s", datetime)
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
			expectedDate: "202510051430.123",
		},
		{
			name:         "zero milliseconds",
			fixedTime:    time.Date(2025, 10, 5, 14, 30, 22, 0, time.UTC),
			expectedDate: "202510051430.000",
		},
		{
			name:         "year end boundary",
			fixedTime:    time.Date(2025, 12, 31, 23, 59, 59, 999000000, time.UTC),
			expectedDate: "202512312359.999",
		},
		{
			name:         "year start boundary",
			fixedTime:    time.Date(2025, 1, 1, 0, 0, 0, 1000000, time.UTC),
			expectedDate: "202501010000.001",
		},
		{
			name:         "nanosecond precision truncation",
			fixedTime:    time.Date(2025, 10, 5, 14, 30, 22, 123456789, time.UTC),
			expectedDate: "202510051430.123",
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
	expected := "202510051430.123"
	assert.Equal(t, expected, datetime, "should convert to UTC")
}

func TestNewAutoEnvProvider_NilClock(t *testing.T) {
	provider := NewAutoEnvProvider(nil)
	result := provider.Generate()

	// Should use time.Now() as default
	assert.NotEmpty(t, result[AutoEnvPrefix+AutoEnvKeyDatetime])
	assert.NotEmpty(t, result[AutoEnvPrefix+AutoEnvKeyPID])
}
