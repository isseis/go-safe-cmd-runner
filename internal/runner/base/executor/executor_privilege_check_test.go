//go:build test

package executor

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecute_PrivilegeLeakCausesExit(t *testing.T) {
	var capturedCode int
	exitCalled := false

	e := NewDefaultExecutor(
		WithExitFunc(func(code int) {
			capturedCode = code
			exitCalled = true
		}),
		WithIdentityChecker(func() error {
			return fmt.Errorf("%w: EUID=0 UID=1000", ErrPrivilegeLeak)
		}),
	).(*DefaultExecutor)

	cmd := createTestCommand("/bin/echo", []string{"hello"})
	// Execute should detect the simulated privilege leak and call osExit.
	e.Execute(context.Background(), cmd, nil, nil) //nolint:errcheck

	assert.True(t, exitCalled, "osExit should be called when a privilege leak is detected")
	assert.Equal(t, 1, capturedCode, "exit code should be 1 for a privilege leak")
}

func TestExecute_NoPrivilegeLeakDoesNotCallExit(t *testing.T) {
	exitCalled := false

	e := NewDefaultExecutor(
		WithExitFunc(func(_ int) {
			exitCalled = true
		}),
		WithIdentityChecker(func() error {
			return nil // identity is clean
		}),
	).(*DefaultExecutor)

	cmd := createTestCommand("/bin/echo", []string{"hello"})
	_, err := e.Execute(context.Background(), cmd, nil, nil)

	assert.NoError(t, err)
	assert.False(t, exitCalled, "osExit should NOT be called when identity is clean")
}
