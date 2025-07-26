package privilege

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestManager_GetHealthStatus(t *testing.T) {
	tests := []struct {
		name            string
		expectSupported bool
		expectError     bool
	}{
		{
			name:            "non-privileged manager",
			expectSupported: false,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a real manager using the public interface
			logger := slog.Default()
			manager := NewManager(logger)

			ctx := context.Background()
			status := manager.GetHealthStatus(ctx)

			assert.Equal(t, tt.expectSupported, status.IsSupported)
			assert.NotZero(t, status.LastCheck)

			if tt.expectError {
				assert.NotEmpty(t, status.Error)
				assert.False(t, status.CanElevate)
			} else {
				assert.Empty(t, status.Error)
				assert.True(t, status.CanElevate)
			}

			// Check that duration is reasonable
			assert.True(t, status.CheckDuration < 1*time.Second)
		})
	}
}
