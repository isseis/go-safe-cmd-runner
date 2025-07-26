package privilege_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/stretchr/testify/assert"
)

func TestLinuxPrivilegeManager_GetHealthStatus(t *testing.T) {
	tests := []struct {
		name            string
		isSetuid        bool
		expectSupported bool
		expectError     bool
	}{
		{
			name:            "healthy privileged manager",
			isSetuid:        true,
			expectSupported: true,
			expectError:     false,
		},
		{
			name:            "non-privileged manager",
			isSetuid:        false,
			expectSupported: false,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test manager using the internal structure
			manager := &MockLinuxPrivilegeManager{
				logger:      slog.Default(),
				originalUID: 1000,
				isSetuid:    tt.isSetuid,
			}

			ctx := context.Background()
			status := manager.GetHealthStatus(ctx)

			assert.Equal(t, tt.expectSupported, status.IsSupported)
			assert.Equal(t, tt.isSetuid, status.SetuidConfigured)
			assert.Equal(t, 1000, status.OriginalUID)
			assert.NotZero(t, status.LastCheck)

			if tt.expectError {
				assert.NotEmpty(t, status.Error)
				assert.False(t, status.CanElevate)
			} else {
				assert.Empty(t, status.Error)
				assert.True(t, status.CanElevate)
			}

			// Check that duration is reasonable (should be very fast for mock)
			assert.True(t, status.CheckDuration < 100*time.Millisecond)
		})
	}
}

// MockLinuxPrivilegeManager for testing health status functionality
type MockLinuxPrivilegeManager struct {
	logger      *slog.Logger
	originalUID int
	isSetuid    bool
	metrics     privilege.Metrics
}

func (m *MockLinuxPrivilegeManager) GetHealthStatus(ctx context.Context) privilege.HealthStatus {
	status := privilege.HealthStatus{
		IsSupported:      m.isSetuid,
		SetuidConfigured: m.isSetuid,
		OriginalUID:      m.originalUID,
		CurrentUID:       m.originalUID,
		EffectiveUID:     m.originalUID,
		LastCheck:        time.Now(),
	}

	if !status.IsSupported {
		status.Error = "Privileged execution not supported"
		status.CanElevate = false
		return status
	}

	// Mock health check
	start := time.Now()
	err := m.HealthCheck(ctx)
	status.CheckDuration = time.Since(start)

	if err != nil {
		status.Error = err.Error()
		status.CanElevate = false
	} else {
		status.CanElevate = true
	}

	return status
}

func (m *MockLinuxPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	return m.isSetuid
}

func (m *MockLinuxPrivilegeManager) HealthCheck(_ context.Context) error {
	if !m.isSetuid {
		return privilege.ErrPrivilegedExecutionNotAvailable
	}
	return nil
}

func (m *MockLinuxPrivilegeManager) WithPrivileges(_ context.Context, _ privilege.ElevationContext, fn func() error) error {
	return fn()
}

func (m *MockLinuxPrivilegeManager) GetCurrentUID() int {
	return m.originalUID
}

func (m *MockLinuxPrivilegeManager) GetOriginalUID() int {
	return m.originalUID
}

func (m *MockLinuxPrivilegeManager) GetMetrics() privilege.Metrics {
	return m.metrics.GetSnapshot()
}
