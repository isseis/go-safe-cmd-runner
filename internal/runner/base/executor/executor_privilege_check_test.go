//go:build test

package executor

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	e.Execute(context.Background(), nil, cmd, nil, nil) //nolint:errcheck

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
	_, err := e.Execute(context.Background(), nil, cmd, nil, nil)

	assert.NoError(t, err)
	assert.False(t, exitCalled, "osExit should NOT be called when identity is clean")
}

// TestApplyCredential_SetsCredentialFields verifies that applyCredential -- the
// helper executeCommandWithPath uses to wire SysProcAttr.Credential for run-as
// execution -- copies the resolved run-as identity's Uid/Gid/Groups onto the
// resulting exec.Cmd exactly, since the kernel relies on these fields (not the
// resolvedIdent struct) at execve time.
func TestApplyCredential_SetsCredentialFields(t *testing.T) {
	resolvedIdent := struct {
		UID    uint32
		GID    uint32
		Groups []uint32
	}{UID: 1000, GID: 2000, Groups: []uint32{2000, 3000}}

	cred := &syscall.Credential{
		Uid:         resolvedIdent.UID,
		Gid:         resolvedIdent.GID,
		Groups:      resolvedIdent.Groups,
		NoSetGroups: false,
	}

	execCmd := exec.Command("/bin/echo", "hello") //nolint:gosec // fixed test binary
	applyCredential(execCmd, cred)

	require.NotNil(t, execCmd.SysProcAttr, "SysProcAttr must be set when cred is non-nil")
	require.NotNil(t, execCmd.SysProcAttr.Credential, "Credential must be set when cred is non-nil")
	assert.Equal(t, resolvedIdent.UID, execCmd.SysProcAttr.Credential.Uid, "Uid must match resolved run-as identity")
	assert.Equal(t, resolvedIdent.GID, execCmd.SysProcAttr.Credential.Gid, "Gid must match resolved run-as identity")
	assert.Equal(t, resolvedIdent.Groups, execCmd.SysProcAttr.Credential.Groups, "Groups must match resolved run-as identity")
	assert.False(t, execCmd.SysProcAttr.Credential.NoSetGroups, "NoSetGroups must be false so supplementary groups are reset")
}

// TestApplyCredential_NilCredIsNoop verifies that normal (non-run-as) execution,
// where cred is nil, leaves SysProcAttr untouched instead of attaching a
// zero-value Credential (which would incorrectly force uid/gid to 0).
func TestApplyCredential_NilCredIsNoop(t *testing.T) {
	execCmd := exec.Command("/bin/echo", "hello") //nolint:gosec // fixed test binary
	applyCredential(execCmd, nil)

	assert.Nil(t, execCmd.SysProcAttr, "SysProcAttr must stay nil for normal execution")
}

// TestPrepareExecCommand_CredentialWiring exercises the same call sequence
// executeCommandWithPath uses in production (prepareExecCommand followed by
// applyCredential) end to end, and asserts the resulting exec.Cmd carries the
// resolved run-as identity's Uid/Gid/Groups.
func TestPrepareExecCommand_CredentialWiring(t *testing.T) {
	e := NewDefaultExecutor().(*DefaultExecutor)

	cred := &syscall.Credential{
		Uid:         1500,
		Gid:         1600,
		Groups:      []uint32{1600, 1700},
		NoSetGroups: false,
	}

	execCmd, cleanup, err := e.prepareExecCommand(context.Background(), nil, "/bin/echo", []string{"hello"}, cred)
	require.NoError(t, err)
	defer cleanup()

	applyCredential(execCmd, cred)

	require.NotNil(t, execCmd.SysProcAttr)
	require.NotNil(t, execCmd.SysProcAttr.Credential)
	assert.Equal(t, cred.Uid, execCmd.SysProcAttr.Credential.Uid)
	assert.Equal(t, cred.Gid, execCmd.SysProcAttr.Credential.Gid)
	assert.Equal(t, cred.Groups, execCmd.SysProcAttr.Credential.Groups)
}
